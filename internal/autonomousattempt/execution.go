package autonomousattempt

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouscorrection"
	"revolvr/internal/autonomouscycle"
)

type Observation struct {
	RunID        string
	OccurrenceID string
	SourceAfter  string
	Outcome      autonomous.AttemptOutcome
	Tokens       *int64
	Evidence     []autonomous.EvidenceReference
	Signatures   []autonomous.CanonicalSignature
	StopReason   autonomous.BreakerReason
}

type Runner func(context.Context) (Observation, error)

type ExecuteConfig struct {
	Admission             AdmissionConfig
	CompletionOperationID string
	Clock                 func() time.Time
	Runner                Runner
}

type ExecutionResult struct {
	Admission   Result
	Completion  Result
	Observation Observation
	RunError    error
}

func CycleOperation(cfg autonomouscycle.Config) Runner {
	return func(ctx context.Context) (Observation, error) {
		result, err := autonomouscycle.Run(ctx, cfg)
		return ObserveCycle(result, err), err
	}
}

func CorrectionOperation(cfg autonomouscorrection.Config) Runner {
	return func(ctx context.Context) (Observation, error) {
		result, err := autonomouscorrection.Run(ctx, cfg)
		return ObserveCorrection(result, err), err
	}
}

// Execute admits, invokes, and accounts for exactly one operation. It is not a
// retry loop and never calls the runner after a rejected admission.
func Execute(ctx context.Context, cfg ExecuteConfig) (ExecutionResult, error) {
	if cfg.Clock == nil || cfg.Runner == nil || strings.TrimSpace(cfg.CompletionOperationID) == "" {
		return ExecutionResult{}, errors.New("execute autonomous attempt: clock, runner, and completion operation ID are required")
	}
	admission, err := Admit(ctx, cfg.Admission)
	result := ExecutionResult{Admission: admission}
	if err != nil || admission.Disposition == DispositionBlocked {
		return result, err
	}
	started := cfg.Clock().UTC()
	observation, runErr := cfg.Runner(ctx)
	finished := cfg.Clock().UTC()
	result.Observation, result.RunError = observation, runErr
	if finished.Before(started) {
		return result, errors.New("execute autonomous attempt: trusted clock moved backwards")
	}
	if observation.Outcome == "" {
		observation.Outcome = autonomous.AttemptOutcomeFailed
	}
	if ctx.Err() != nil {
		observation.Outcome = autonomous.AttemptOutcomeCancelled
	}
	if strings.TrimSpace(observation.RunID) == "" {
		observation.RunID = "attempt-" + cfg.Admission.AttemptID
	}
	if !validSHA(observation.SourceAfter) {
		observation.SourceAfter = ""
		if observation.Outcome != autonomous.AttemptOutcomeCancelled {
			observation.Outcome = autonomous.AttemptOutcomeSafetyStopped
		}
		observation.Evidence = append(observation.Evidence, autonomous.EvidenceReference{Kind: autonomous.EvidenceKindGit, Reference: "attempt:" + cfg.Admission.AttemptID + ":source-after", Detail: "Trusted post-operation source identity was unavailable."})
	}
	if runErr != nil && len(observation.Evidence) == 0 {
		observation.Evidence = []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "attempt:" + cfg.Admission.AttemptID, Detail: runErr.Error()}}
	}
	completion, completeErr := Complete(context.WithoutCancel(ctx), CompletionConfig{
		TaskID: cfg.Admission.TaskID, OperationID: cfg.CompletionOperationID,
		AttemptID: cfg.Admission.AttemptID, Expected: admission.Current.Expected(),
		RunID: observation.RunID, OccurrenceID: observation.OccurrenceID,
		SourceAfter: observation.SourceAfter, Outcome: observation.Outcome,
		Duration: finished.Sub(started), Tokens: observation.Tokens,
		Evidence: observation.Evidence, Signatures: observation.Signatures,
		StopReason: observation.StopReason,
		CreatedAt:  finished, Store: cfg.Admission.Store,
	})
	result.Completion = completion
	if completeErr != nil {
		return result, completeErr
	}
	return result, runErr
}

func ObserveCycle(result autonomouscycle.Result, runErr error) Observation {
	observation := Observation{RunID: result.Supervisor.RunID, SourceAfter: result.Source.FinalRevision}
	if result.Worker.Started {
		observation.RunID = result.Worker.RunID
		observation.OccurrenceID = result.Worker.Verification.OccurrenceID
	}
	if observation.SourceAfter == "" {
		observation.SourceAfter = result.Source.WorkerRevision
	}
	if observation.SourceAfter == "" {
		observation.SourceAfter = result.Source.AdmissionRevision
	}
	switch result.Outcome {
	case autonomouscycle.OutcomeVerifiedChangesCommitted, autonomouscycle.OutcomeReadOnlyCompleted, autonomouscycle.OutcomeCompleteAuthorized, autonomouscycle.OutcomeBlockAuthorized, autonomouscycle.OutcomeNeedsInputAuthorized:
		observation.Outcome = autonomous.AttemptOutcomeSucceeded
	case autonomouscycle.OutcomeWorkerNoChanges:
		observation.Outcome = autonomous.AttemptOutcomeNoProgress
	default:
		observation.Outcome = autonomous.AttemptOutcomeFailed
	}
	switch result.Outcome {
	case autonomouscycle.OutcomeSourceChangedDuringDossier, autonomouscycle.OutcomeSourceChanged, autonomouscycle.OutcomeReadOnlyMutation:
		observation.Outcome = autonomous.AttemptOutcomeSafetyStopped
		observation.StopReason = autonomous.BreakerUnsafeSource
	case autonomouscycle.OutcomeInvalidConfiguration, autonomouscycle.OutcomeSafetyPreflightFailed, autonomouscycle.OutcomeNoTaskState, autonomouscycle.OutcomeDossierFailed, autonomouscycle.OutcomeSupervisorFailed, autonomouscycle.OutcomePolicyRejected:
		observation.Outcome = autonomous.AttemptOutcomeSafetyStopped
		observation.StopReason = autonomous.BreakerStaleEvidence
	}
	if result.Supervisor.Decision != nil {
		if signature, err := DecisionSignature(*result.Supervisor.Decision); err == nil {
			observation.Signatures = append(observation.Signatures, signature)
		}
	}
	if result.Failure != nil {
		material := OperationFailureMaterial{TaskID: result.TaskID, Action: actionFromCycle(result), Stage: result.Failure.Stage, Classification: string(result.Outcome), Evidence: cycleEvidence(result)}
		if signature, err := OperationFailureSignature(material); err == nil {
			observation.Signatures = append(observation.Signatures, signature)
		}
	}
	observation.Evidence = cycleEvidence(result)
	observation.Tokens = cycleTokens(result)
	if runErr != nil && errors.Is(runErr, context.Canceled) {
		observation.Outcome = autonomous.AttemptOutcomeCancelled
	}
	return observation
}

func ObserveCorrection(result autonomouscorrection.Result, runErr error) Observation {
	observation := Observation{RunID: result.Correction.Worker.RunID, SourceAfter: result.Correction.Source.FinalRevision, Outcome: autonomous.AttemptOutcomeFailed}
	if result.Audit.Worker.Started {
		observation.RunID = result.Audit.Worker.RunID
		observation.SourceAfter = result.Audit.Source.FinalRevision
	}
	if result.Outcome == autonomouscorrection.OutcomeReturnedToSupervisor {
		observation.Outcome = autonomous.AttemptOutcomeSucceeded
	}
	if result.Outcome == autonomouscorrection.OutcomeSafetyStopped {
		observation.Outcome = autonomous.AttemptOutcomeSafetyStopped
		observation.StopReason = autonomous.BreakerUnsafeSource
	}
	if runErr != nil && errors.Is(runErr, context.Canceled) {
		observation.Outcome = autonomous.AttemptOutcomeCancelled
	}
	observation.Evidence = append(cycleEvidence(result.Correction), cycleEvidence(result.Audit)...)
	if result.Failure != nil {
		signature, err := OperationFailureSignature(OperationFailureMaterial{TaskID: result.TaskID, Action: autonomous.ActionCorrect, Stage: result.Failure.Stage, Classification: string(result.Outcome), Evidence: observation.Evidence})
		if err == nil {
			observation.Signatures = append(observation.Signatures, signature)
		}
	}
	left, leftOK := tokenValue(result.Correction)
	right, rightOK := tokenValue(result.Audit)
	if leftOK && (!result.Audit.Worker.Started || rightOK) && left <= math.MaxInt64-right {
		total := left + right
		observation.Tokens = &total
	}
	return observation
}

func cycleTokens(result autonomouscycle.Result) *int64 {
	value, ok := tokenValue(result)
	if !ok {
		return nil
	}
	return &value
}

func tokenValue(result autonomouscycle.Result) (int64, bool) {
	var total int64
	if result.Supervisor.RunID != "" {
		if !result.Supervisor.Codex.UsageFound {
			return 0, false
		}
		value := int64(result.Supervisor.Codex.Usage.InputTokens) + int64(result.Supervisor.Codex.Usage.OutputTokens)
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	if result.Worker.Started {
		if !result.Worker.Codex.UsageFound {
			return 0, false
		}
		value := int64(result.Worker.Codex.Usage.InputTokens) + int64(result.Worker.Codex.Usage.OutputTokens)
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func cycleEvidence(result autonomouscycle.Result) []autonomous.EvidenceReference {
	var evidence []autonomous.EvidenceReference
	if result.Supervisor.DecisionReference != nil {
		evidence = append(evidence, result.Supervisor.DecisionReference.Artifact)
	}
	if result.Worker.Receipt.Path != "" {
		evidence = append(evidence, autonomous.EvidenceReference{Kind: autonomous.EvidenceKindReceipt, Reference: result.Worker.Receipt.Path, Detail: "Harness-finalized worker receipt."})
	}
	if result.Worker.Verification.Policy != nil {
		evidence = append(evidence, result.Worker.Verification.Policy.Summary.Evidence...)
	}
	return evidence
}

func actionFromCycle(result autonomouscycle.Result) autonomous.Action {
	if result.Route != nil {
		return result.Route.Action
	}
	if result.Supervisor.Decision != nil {
		return result.Supervisor.Decision.Action
	}
	return autonomous.ActionBlock
}
