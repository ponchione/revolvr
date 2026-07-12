// Package autonomouspolicy authorizes validated autonomous supervisor routes.
// It consumes explicit current evidence and performs no discovery, execution,
// persistence, or state mutation.
package autonomouspolicy

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousverification"
)

type RouteKind string

const (
	RouteKindWorker     RouteKind = "worker"
	RouteKindComplete   RouteKind = "complete"
	RouteKindBlock      RouteKind = "block"
	RouteKindNeedsInput RouteKind = "needs_input"
)

type SourceSafety string

const (
	SourceSafetySafe    SourceSafety = "safe"
	SourceSafetyUnsafe  SourceSafety = "unsafe"
	SourceSafetyUnknown SourceSafety = "unknown"
)

// SourceMutation identifies the latest source-mutating autonomous worker run,
// when one exists. A nil LatestMutation means no source-mutating worker run is
// present in the caller's current evidence history.
type SourceMutation struct {
	TaskID            string
	RunID             string
	DecisionID        string
	Action            autonomous.Action
	ResultingRevision string
}

// SourceEvidence identifies the exact current repository content and the
// caller's typed safety classification. Revision is a lower-case SHA-256
// produced from the content-sensitive policy source projection. Runtime race
// detection may retain a stricter full snapshot identity separately.
type SourceEvidence struct {
	Revision       string
	Safety         SourceSafety
	LatestMutation *SourceMutation
}

// VerificationEvidence ties an autonomous verification summary to the exact
// source content it verified. Policy evidence requires both run and occurrence
// identities even though dossier summaries may omit them.
type VerificationEvidence struct {
	Summary        autonomous.VerificationSummary       `json:"summary"`
	SourceRevision string                               `json:"source_revision"`
	Tiered         *autonomousverification.GateEvidence `json:"tiered,omitempty"`
}

// AuditEvidence ties an exact audit report to its auditor run, source content,
// and the verification occurrence consumed by the audit.
type AuditEvidence struct {
	Report                   autonomous.AuditReport
	RunID                    string
	AuditorProfile           autonomous.WorkerProfile
	SourceRevision           string
	VerificationRunID        string
	VerificationOccurrenceID string
}

type Input struct {
	TaskID            string
	Decision          autonomous.SupervisorDecision
	Reference         autonomous.DecisionReference
	State             autonomous.ExecutionState
	Source            SourceEvidence
	Verification      *VerificationEvidence
	Audit             *AuditEvidence
	CorrectionFailure *autonomous.VerificationFailureTarget
}

// ValidateEvidence validates the task identity and transient source,
// verification, and audit envelopes consumed by Evaluate without selecting or
// authorizing an action. Runtime orchestrators use it to fail closed before a
// supervisor invocation while leaving all lifecycle and action rules in
// Evaluate.
func ValidateEvidence(taskID string, source SourceEvidence, verification *VerificationEvidence, audit *AuditEvidence) error {
	if err := validateStableIdentity("requested task_id", taskID); err != nil {
		return fmt.Errorf("identity gate: %w", err)
	}
	if err := validateSourceEvidence(source); err != nil {
		return fmt.Errorf("source evidence gate: %w", err)
	}
	if source.LatestMutation != nil && source.LatestMutation.TaskID != taskID {
		return fmt.Errorf("identity gate: latest source mutation task_id %q does not match requested task_id %q", source.LatestMutation.TaskID, taskID)
	}
	if verification != nil {
		if err := validateVerificationEvidence(*verification); err != nil {
			return fmt.Errorf("verification evidence gate: %w", err)
		}
		if verification.Summary.TaskID != taskID {
			return fmt.Errorf("identity gate: verification task_id %q does not match requested task_id %q", verification.Summary.TaskID, taskID)
		}
	}
	if audit != nil {
		if err := validateAuditEvidence(*audit); err != nil {
			return fmt.Errorf("audit evidence gate: %w", err)
		}
		if audit.Report.TaskID != taskID {
			return fmt.Errorf("identity gate: audit task_id %q does not match requested task_id %q", audit.Report.TaskID, taskID)
		}
	}
	return nil
}

// Route is a compact authorization. It contains no command, callback,
// transition, or persistence instruction.
type Route struct {
	Kind           RouteKind
	TaskID         string
	DecisionID     string
	Action         autonomous.Action
	WorkerProfile  autonomous.WorkerProfile
	SourceRevision string
}

// Evaluate deterministically authorizes one supervisor decision without
// mutating input or consulting any ambient state.
func Evaluate(input Input) (Route, error) {
	prefix := fmt.Sprintf("evaluate autonomous route for task %q action %q", input.TaskID, input.Decision.Action)
	if err := validateStableIdentity("requested task_id", input.TaskID); err != nil {
		return Route{}, fmt.Errorf("%s: identity gate: %w", prefix, err)
	}
	if err := input.State.Validate(); err != nil {
		return Route{}, fmt.Errorf("%s: execution state gate: %w", prefix, err)
	}
	if err := input.Decision.Validate(); err != nil {
		return Route{}, fmt.Errorf("%s: supervisor decision gate: %w", prefix, err)
	}
	if err := input.Reference.Validate(); err != nil {
		return Route{}, fmt.Errorf("%s: decision reference gate: %w", prefix, err)
	}
	if input.Decision.TaskID != input.TaskID {
		return Route{}, fmt.Errorf("%s: identity gate: decision task_id %q does not match requested task_id %q", prefix, input.Decision.TaskID, input.TaskID)
	}
	if input.Reference.TaskID != input.TaskID {
		return Route{}, fmt.Errorf("%s: identity gate: decision reference task_id %q does not match requested task_id %q", prefix, input.Reference.TaskID, input.TaskID)
	}
	if input.State.TaskID != input.TaskID {
		return Route{}, fmt.Errorf("%s: identity gate: execution state task_id %q does not match requested task_id %q", prefix, input.State.TaskID, input.TaskID)
	}
	if input.Reference.Action != input.Decision.Action {
		return Route{}, fmt.Errorf("%s: decision provenance gate: reference action %q does not match decision action %q", prefix, input.Reference.Action, input.Decision.Action)
	}
	if input.Reference.WorkerProfile != input.Decision.WorkerProfile {
		return Route{}, fmt.Errorf("%s: decision provenance gate: reference worker_profile %q does not match decision worker_profile %q", prefix, input.Reference.WorkerProfile, input.Decision.WorkerProfile)
	}
	if err := rejectDecisionReplay(input.State, input.Reference); err != nil {
		return Route{}, fmt.Errorf("%s: decision provenance gate: %w", prefix, err)
	}
	if err := ValidateEvidence(input.TaskID, input.Source, input.Verification, input.Audit); err != nil {
		return Route{}, fmt.Errorf("%s: %w", prefix, err)
	}
	if input.CorrectionFailure != nil && (input.Decision.Action != autonomous.ActionCorrect || input.Decision.VerificationFailure == nil) {
		return Route{}, fmt.Errorf("%s: correction gate: verification-failure authority is present for an unrelated decision", prefix)
	}
	if err := admitLifecycle(input.State.Lifecycle, input.Decision.Action); err != nil {
		return Route{}, fmt.Errorf("%s: lifecycle gate: %w", prefix, err)
	}
	if input.Decision.Action != autonomous.ActionBlock && input.Source.Safety != SourceSafetySafe {
		return Route{}, fmt.Errorf("%s: source safety gate: action %q requires safe source evidence (got %q for revision %q)", prefix, input.Decision.Action, input.Source.Safety, input.Source.Revision)
	}

	if err := evaluateAction(input); err != nil {
		return Route{}, fmt.Errorf("%s: %w", prefix, err)
	}

	kind := RouteKindWorker
	if input.Decision.Action == autonomous.ActionComplete {
		kind = RouteKindComplete
	} else if input.Decision.Action == autonomous.ActionBlock {
		kind = RouteKindBlock
	} else if input.Decision.Action == autonomous.ActionNeedsInput {
		kind = RouteKindNeedsInput
	}
	return Route{
		Kind:           kind,
		TaskID:         input.TaskID,
		DecisionID:     input.Reference.DecisionID,
		Action:         input.Decision.Action,
		WorkerProfile:  input.Decision.WorkerProfile,
		SourceRevision: input.Source.Revision,
	}, nil
}

func evaluateAction(input Input) error {
	switch input.Decision.Action {
	case autonomous.ActionPlan:
		if findingID, ok := unresolvedCurrentAuditFinding(input, true); ok {
			return fmt.Errorf("plan gate: current blocking audit finding %q remains unresolved and requires correction", findingID)
		}
		return nil
	case autonomous.ActionImplement:
		if input.State.Plan == nil {
			return errors.New("implementation gate: a current plan is required")
		}
		for _, step := range input.State.Plan.Steps {
			if step.Status == autonomous.PlanStepStatusPending || step.Status == autonomous.PlanStepStatusInProgress {
				if findingID, ok := unresolvedCurrentAuditFinding(input, false); ok {
					return fmt.Errorf("implementation gate: current changes_required audit finding %q remains unresolved and requires correction", findingID)
				}
				return nil
			}
		}
		if input.State.Plan.Completed {
			return fmt.Errorf("implementation gate: plan %q is completed; a deliberate new plan revision is required", input.State.Plan.ID)
		}
		return fmt.Errorf("implementation gate: plan %q has no pending or in_progress step", input.State.Plan.ID)
	case autonomous.ActionAudit:
		return requireFreshPassedVerification(input)
	case autonomous.ActionCorrect:
		if input.Decision.VerificationFailure != nil {
			if input.CorrectionFailure == nil {
				return errors.New("correction gate: exact verification-failure authority is missing")
			}
			if err := autonomous.ValidateVerificationCorrectionDecision(input.Decision, *input.CorrectionFailure); err != nil {
				return fmt.Errorf("correction gate: %w", err)
			}
			if input.Verification == nil || input.Verification.Summary.Status != autonomous.VerificationStatusFailed {
				return errors.New("correction gate: authorizing verification occurrence is not failed")
			}
			if input.Verification.Summary.RunID != input.CorrectionFailure.RunID || input.Verification.Summary.OccurrenceID != input.CorrectionFailure.OccurrenceID || input.Verification.SourceRevision != input.CorrectionFailure.SourceRevision || input.Source.Revision != input.CorrectionFailure.SourceRevision || !reflect.DeepEqual(input.Verification.Summary.Evidence, input.CorrectionFailure.Evidence) {
				return errors.New("correction gate: failed verification evidence does not exactly match correction authority")
			}
			return nil
		}
		if err := requireFreshAudit(input); err != nil {
			return fmt.Errorf("correction gate: %w", err)
		}
		if input.Audit.Report.Disposition != autonomous.AuditDispositionChangesRequired {
			return fmt.Errorf("correction gate: audit run %q disposition is %q, want %q", input.Audit.RunID, input.Audit.Report.Disposition, autonomous.AuditDispositionChangesRequired)
		}
		if err := autonomous.ValidateCorrectionDecision(input.Decision, input.Audit.Report); err != nil {
			return fmt.Errorf("correction gate: %w", err)
		}
		for _, findingID := range input.Decision.FindingIDs {
			if status, found := findingResolution(input.State, findingID); found && status != autonomous.FindingResolutionStatusOpen {
				return fmt.Errorf("correction gate: finding %q is already terminally dispositioned as %q", findingID, status)
			}
		}
		return nil
	case autonomous.ActionDocument:
		if err := requireFreshCleanAudit(input); err != nil {
			return fmt.Errorf("documentation gate: %w", err)
		}
		return nil
	case autonomous.ActionSimplify:
		if err := requireFreshCleanAudit(input); err != nil {
			return fmt.Errorf("simplification gate: %w", err)
		}
		return nil
	case autonomous.ActionComplete:
		return evaluateCompletion(input)
	case autonomous.ActionBlock:
		return nil
	case autonomous.ActionNeedsInput:
		return nil
	default:
		return fmt.Errorf("action gate: unsupported action %q", input.Decision.Action)
	}
}

func evaluateCompletion(input Input) error {
	if input.State.Plan == nil {
		return errors.New("completion gate: a current plan is required")
	}
	for _, step := range input.State.Plan.Steps {
		if step.Status != autonomous.PlanStepStatusCompleted && step.Status != autonomous.PlanStepStatusSkipped {
			return fmt.Errorf("completion gate: plan %q step %q is nonterminal with status %q", input.State.Plan.ID, step.ID, step.Status)
		}
	}
	if !input.State.Plan.Completed {
		return fmt.Errorf("completion gate: plan %q is not marked completed", input.State.Plan.ID)
	}
	if len(input.State.AcceptanceCriteria) == 0 {
		return errors.New("completion gate: at least one acceptance criterion is required")
	}
	for _, criterion := range input.State.AcceptanceCriteria {
		if criterion.Status == autonomous.AcceptanceStatusPending {
			return fmt.Errorf("completion gate: acceptance criterion %q is pending", criterion.ID)
		}
	}
	if err := requireFreshCleanAudit(input); err != nil {
		return fmt.Errorf("completion gate: %w", err)
	}
	for _, resolution := range input.State.FindingResolutions {
		if resolution.Status == autonomous.FindingResolutionStatusOpen {
			return fmt.Errorf("completion gate: finding %q remains open", resolution.FindingID)
		}
	}
	return nil
}

func requireFreshPassedVerification(input Input) error {
	if input.Verification == nil {
		return fmt.Errorf("verification gate: current verification is missing for source revision %q", input.Source.Revision)
	}
	verification := input.Verification
	if verification.Summary.Status != autonomous.VerificationStatusPassed {
		return fmt.Errorf("verification gate: occurrence %q in run %q has status %q, want passed", verification.Summary.OccurrenceID, verification.Summary.RunID, verification.Summary.Status)
	}
	if verification.SourceRevision != input.Source.Revision {
		return fmt.Errorf("verification gate: occurrence %q is stale for source revision %q (verified %q)", verification.Summary.OccurrenceID, input.Source.Revision, verification.SourceRevision)
	}
	if verification.Tiered != nil {
		if err := verification.Tiered.Validate(); err != nil {
			return fmt.Errorf("verification gate: malformed tier evidence: %w", err)
		}
		if verification.Tiered.Purpose != autonomousverification.PurposeFinal {
			return fmt.Errorf("verification gate: occurrence %q is fast-only and cannot authorize audit or completion", verification.Summary.OccurrenceID)
		}
		if !verification.Tiered.FinalSatisfied {
			return fmt.Errorf("verification gate: occurrence %q does not satisfy every required final tier", verification.Summary.OccurrenceID)
		}
	}
	return nil
}

func requireFreshAudit(input Input) error {
	if err := requireFreshPassedVerification(input); err != nil {
		return err
	}
	if input.Audit == nil {
		return fmt.Errorf("audit gate: current audit is missing for source revision %q", input.Source.Revision)
	}
	audit := input.Audit
	if audit.AuditorProfile != autonomous.WorkerProfileAuditor {
		return fmt.Errorf("audit gate: run %q used profile %q, want %q", audit.RunID, audit.AuditorProfile, autonomous.WorkerProfileAuditor)
	}
	if audit.SourceRevision != input.Source.Revision {
		return fmt.Errorf("audit gate: run %q is stale for source revision %q (audited %q)", audit.RunID, input.Source.Revision, audit.SourceRevision)
	}
	if audit.VerificationRunID != input.Verification.Summary.RunID || audit.VerificationOccurrenceID != input.Verification.Summary.OccurrenceID {
		return fmt.Errorf("audit gate: run %q consumed verification %q/%q, want current occurrence %q/%q", audit.RunID, audit.VerificationRunID, audit.VerificationOccurrenceID, input.Verification.Summary.RunID, input.Verification.Summary.OccurrenceID)
	}
	if mutation := input.Source.LatestMutation; mutation != nil && audit.RunID == mutation.RunID {
		return fmt.Errorf("audit independence gate: audit run %q equals latest source-mutating %q run for revision %q", audit.RunID, mutation.Action, mutation.ResultingRevision)
	}
	return nil
}

func requireFreshCleanAudit(input Input) error {
	if err := requireFreshAudit(input); err != nil {
		return err
	}
	if input.Audit.Report.Disposition != autonomous.AuditDispositionClean {
		return fmt.Errorf("audit gate: run %q disposition is %q, want clean", input.Audit.RunID, input.Audit.Report.Disposition)
	}
	return nil
}

func unresolvedCurrentAuditFinding(input Input, blockingOnly bool) (string, bool) {
	if input.Audit == nil ||
		input.Audit.AuditorProfile != autonomous.WorkerProfileAuditor ||
		input.Audit.SourceRevision != input.Source.Revision ||
		input.Audit.Report.Disposition != autonomous.AuditDispositionChangesRequired {
		return "", false
	}
	for _, finding := range input.Audit.Report.Findings {
		if blockingOnly && finding.Significance != autonomous.FindingSignificanceBlocking {
			continue
		}
		if status, found := findingResolution(input.State, finding.ID); !found || status == autonomous.FindingResolutionStatusOpen {
			return finding.ID, true
		}
	}
	return "", false
}

func findingResolution(state autonomous.ExecutionState, findingID string) (autonomous.FindingResolutionStatus, bool) {
	for _, resolution := range state.FindingResolutions {
		if resolution.FindingID == findingID {
			return resolution.Status, true
		}
	}
	return "", false
}

func admitLifecycle(lifecycle autonomous.LifecycleState, action autonomous.Action) error {
	switch lifecycle {
	case autonomous.LifecycleStatePending:
		if action == autonomous.ActionPlan || action == autonomous.ActionBlock || action == autonomous.ActionNeedsInput {
			return nil
		}
		return fmt.Errorf("pending lifecycle admits only %q, %q, or %q, not %q", autonomous.ActionPlan, autonomous.ActionBlock, autonomous.ActionNeedsInput, action)
	case autonomous.LifecycleStateReady:
		return nil
	case autonomous.LifecycleStatePlanning, autonomous.LifecycleStateWorking, autonomous.LifecycleStateVerifying, autonomous.LifecycleStateAuditing, autonomous.LifecycleStateCorrecting:
		return fmt.Errorf("lifecycle %q already has an operation in flight and admits no new routing", lifecycle)
	case autonomous.LifecycleStateNeedsInput:
		return errors.New("needs_input lifecycle admits no routing until an exact durable answer and explicit resume transition")
	case autonomous.LifecycleStateFinalizing:
		return errors.New("finalizing lifecycle admits no new routing")
	case autonomous.LifecycleStateCompleted, autonomous.LifecycleStateBlocked, autonomous.LifecycleStateCancelled:
		return fmt.Errorf("terminal lifecycle %q admits no new routing without an explicit reopen operation", lifecycle)
	default:
		return fmt.Errorf("unknown lifecycle %q", lifecycle)
	}
}

func rejectDecisionReplay(state autonomous.ExecutionState, current autonomous.DecisionReference) error {
	prior := make([]autonomous.DecisionReference, 0, len(state.FindingResolutions)+1)
	for _, resolution := range state.FindingResolutions {
		if resolution.Resolution != nil {
			prior = append(prior, *resolution.Resolution)
		}
	}
	if state.LatestDecision != nil {
		prior = append(prior, *state.LatestDecision)
	}
	for _, reference := range prior {
		if reference.DecisionID != current.DecisionID {
			continue
		}
		if reference != current {
			return fmt.Errorf("decision_id %q reuses materially different task/run/action/profile/artifact/time provenance", current.DecisionID)
		}
		if state.LatestDecision != nil && state.LatestDecision.DecisionID == current.DecisionID {
			return fmt.Errorf("decision_id %q replays the execution state's latest decision", current.DecisionID)
		}
		return fmt.Errorf("decision_id %q replays a prior finding-resolution decision", current.DecisionID)
	}
	return nil
}

func validateSourceEvidence(evidence SourceEvidence) error {
	if !validRevision(evidence.Revision) {
		return fmt.Errorf("current source revision %q is invalid (want 64 lower-case hexadecimal characters)", evidence.Revision)
	}
	switch evidence.Safety {
	case SourceSafetySafe, SourceSafetyUnsafe, SourceSafetyUnknown:
	default:
		return fmt.Errorf("unknown source safety status %q", evidence.Safety)
	}
	if evidence.LatestMutation == nil {
		return nil
	}
	mutation := *evidence.LatestMutation
	if err := validateStableIdentity("latest mutation task_id", mutation.TaskID); err != nil {
		return err
	}
	if err := validateStableIdentity("latest mutation run_id", mutation.RunID); err != nil {
		return err
	}
	if mutation.DecisionID != "" {
		if err := validateStableIdentity("latest mutation decision_id", mutation.DecisionID); err != nil {
			return err
		}
	}
	switch mutation.Action {
	case autonomous.ActionImplement, autonomous.ActionCorrect, autonomous.ActionDocument, autonomous.ActionSimplify:
	default:
		return fmt.Errorf("latest mutation action %q is not source-changing", mutation.Action)
	}
	if !validRevision(mutation.ResultingRevision) {
		return fmt.Errorf("latest mutation resulting source revision %q is invalid", mutation.ResultingRevision)
	}
	return nil
}

func validateVerificationEvidence(evidence VerificationEvidence) error {
	summary := evidence.Summary
	if err := validateStableIdentity("verification task_id", summary.TaskID); err != nil {
		return err
	}
	switch summary.Status {
	case autonomous.VerificationStatusPassed, autonomous.VerificationStatusFailed:
	default:
		return fmt.Errorf("verification has unknown status %q", summary.Status)
	}
	if strings.TrimSpace(summary.Summary) == "" {
		return errors.New("verification summary is required")
	}
	if err := validateStableIdentity("verification run_id", summary.RunID); err != nil {
		return err
	}
	if err := validateStableIdentity("verification occurrence_id", summary.OccurrenceID); err != nil {
		return err
	}
	if err := validateEvidenceReferences("verification evidence", summary.Evidence); err != nil {
		return err
	}
	if !validRevision(evidence.SourceRevision) {
		return fmt.Errorf("verification source revision %q is invalid", evidence.SourceRevision)
	}
	if evidence.Tiered != nil {
		if err := evidence.Tiered.Validate(); err != nil {
			return fmt.Errorf("tiered final-gate evidence: %w", err)
		}
		if evidence.Tiered.OverallOutcome != autonomousverification.OutcomePassed && summary.Status == autonomous.VerificationStatusPassed {
			return errors.New("tiered verification classification cannot project as passed")
		}
	}
	if summary.Tiered != nil {
		if err := summary.Tiered.Validate(); err != nil {
			return fmt.Errorf("tiered verification result: %w", err)
		}
		if summary.Tiered.TaskID != summary.TaskID || summary.Tiered.RunID != summary.RunID || summary.Tiered.OccurrenceID != summary.OccurrenceID || summary.Tiered.SourceRevision != evidence.SourceRevision {
			return errors.New("tiered verification result has wrong task/run/occurrence/source identity")
		}
		if evidence.Tiered == nil || !reflect.DeepEqual(*evidence.Tiered, summary.Tiered.Gate) {
			return errors.New("tiered verification result and final-gate projection disagree")
		}
	}
	return nil
}

func validateAuditEvidence(evidence AuditEvidence) error {
	if err := evidence.Report.Validate(); err != nil {
		return err
	}
	if err := validateStableIdentity("audit run_id", evidence.RunID); err != nil {
		return err
	}
	if !validWorkerProfile(evidence.AuditorProfile) {
		return fmt.Errorf("audit has unknown profile %q", evidence.AuditorProfile)
	}
	if !validRevision(evidence.SourceRevision) {
		return fmt.Errorf("audit source revision %q is invalid", evidence.SourceRevision)
	}
	if err := validateStableIdentity("audit verification_run_id", evidence.VerificationRunID); err != nil {
		return err
	}
	if err := validateStableIdentity("audit verification_occurrence_id", evidence.VerificationOccurrenceID); err != nil {
		return err
	}
	return nil
}

func validateEvidenceReferences(label string, references []autonomous.EvidenceReference) error {
	if len(references) == 0 {
		return fmt.Errorf("%s requires at least one typed reference", label)
	}
	for i, reference := range references {
		if !validEvidenceKind(reference.Kind) {
			return fmt.Errorf("%s[%d] has unknown kind %q", label, i, reference.Kind)
		}
		if strings.TrimSpace(reference.Reference) == "" {
			return fmt.Errorf("%s[%d] reference is required", label, i)
		}
		if strings.TrimSpace(reference.Detail) == "" {
			return fmt.Errorf("%s[%d] detail is required", label, i)
		}
	}
	return nil
}

func validateStableIdentity(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s %q is empty or malformed", label, value)
	}
	return nil
}

func validRevision(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func validWorkerProfile(profile autonomous.WorkerProfile) bool {
	switch profile {
	case autonomous.WorkerProfilePlanner,
		autonomous.WorkerProfileImplementer,
		autonomous.WorkerProfileAuditor,
		autonomous.WorkerProfileCorrector,
		autonomous.WorkerProfileDocumentor,
		autonomous.WorkerProfileSimplifier:
		return true
	default:
		return false
	}
}

func validEvidenceKind(kind autonomous.EvidenceKind) bool {
	switch kind {
	case autonomous.EvidenceKindTask,
		autonomous.EvidenceKindPlan,
		autonomous.EvidenceKindLedger,
		autonomous.EvidenceKindReceipt,
		autonomous.EvidenceKindVerification,
		autonomous.EvidenceKindGit,
		autonomous.EvidenceKindAudit,
		autonomous.EvidenceKindRepository,
		autonomous.EvidenceKindFile:
		return true
	default:
		return false
	}
}

type NeedsInputYieldDisposition string

const (
	NeedsInputYieldClean  NeedsInputYieldDisposition = "clean_yield"
	NeedsInputYieldUnsafe NeedsInputYieldDisposition = "unsafe"
)

type NeedsInputYieldReason string

const (
	NeedsInputYieldReasonClean             NeedsInputYieldReason = "clean"
	NeedsInputYieldReasonTaskMismatch      NeedsInputYieldReason = "task_mismatch"
	NeedsInputYieldReasonMalformedState    NeedsInputYieldReason = "malformed_state"
	NeedsInputYieldReasonNotSuspended      NeedsInputYieldReason = "not_suspended"
	NeedsInputYieldReasonUnsafeSource      NeedsInputYieldReason = "unsafe_source"
	NeedsInputYieldReasonUnknownSource     NeedsInputYieldReason = "unknown_source"
	NeedsInputYieldReasonOperationInFlight NeedsInputYieldReason = "source_operation_in_flight"
	NeedsInputYieldReasonStaleSource       NeedsInputYieldReason = "stale_source"
	NeedsInputYieldReasonMissingProvenance NeedsInputYieldReason = "missing_question_provenance"
)

type NeedsInputYieldInput struct {
	TaskID                  string
	State                   autonomous.ExecutionState
	Source                  SourceEvidence
	SourceOperationInFlight bool
}

type NeedsInputYieldResult struct {
	Disposition NeedsInputYieldDisposition
	Reason      NeedsInputYieldReason
	Question    *autonomous.QuestionIdentity
}

// EvaluateNeedsInputYield is a pure future-scheduler gate. It does not select
// a task or route work; it only proves whether this suspension is safe to leave.
func EvaluateNeedsInputYield(input NeedsInputYieldInput) NeedsInputYieldResult {
	unsafe := func(reason NeedsInputYieldReason) NeedsInputYieldResult {
		return NeedsInputYieldResult{Disposition: NeedsInputYieldUnsafe, Reason: reason}
	}
	if input.TaskID == "" || input.State.TaskID != input.TaskID {
		return unsafe(NeedsInputYieldReasonTaskMismatch)
	}
	if input.State.Lifecycle != autonomous.LifecycleStateNeedsInput || input.State.NeedsInput == nil || input.State.NeedsInput.CurrentQuestion == nil {
		return unsafe(NeedsInputYieldReasonNotSuspended)
	}
	current := *input.State.NeedsInput.CurrentQuestion
	var record *autonomous.InputQuestionRecord
	for i := range input.State.Input.Questions {
		if input.State.Input.Questions[i].Question.Identity() == current {
			record = &input.State.Input.Questions[i]
		}
	}
	if record == nil || record.Decision.Action != autonomous.ActionNeedsInput || record.Decision.Artifact.Reference == "" {
		return unsafe(NeedsInputYieldReasonMissingProvenance)
	}
	if err := input.State.Validate(); err != nil {
		return unsafe(NeedsInputYieldReasonMalformedState)
	}
	if input.SourceOperationInFlight {
		return unsafe(NeedsInputYieldReasonOperationInFlight)
	}
	if err := validateSourceEvidence(input.Source); err != nil {
		return unsafe(NeedsInputYieldReasonUnknownSource)
	}
	if input.Source.Safety == SourceSafetyUnsafe {
		return unsafe(NeedsInputYieldReasonUnsafeSource)
	}
	if input.Source.Safety != SourceSafetySafe {
		return unsafe(NeedsInputYieldReasonUnknownSource)
	}
	if record.SourceRevision != input.Source.Revision {
		return unsafe(NeedsInputYieldReasonStaleSource)
	}
	return NeedsInputYieldResult{Disposition: NeedsInputYieldClean, Reason: NeedsInputYieldReasonClean, Question: &current}
}

type IndependentWorkResult struct {
	Allowed bool
	Work    *autonomous.IndependentWorkDeclaration
	Reason  NeedsInputYieldReason
}

// EvaluateIndependentWork projects only an explicitly declared read-only
// item after the clean-yield proof. It is not a Route and starts nothing.
func EvaluateIndependentWork(input NeedsInputYieldInput, workID string) IndependentWorkResult {
	yield := EvaluateNeedsInputYield(input)
	if yield.Disposition != NeedsInputYieldClean {
		return IndependentWorkResult{Reason: yield.Reason}
	}
	question := input.State.Input.Questions[len(input.State.Input.Questions)-1].Question
	for i := range question.IndependentWork {
		if question.IndependentWork[i].ID == workID {
			work := question.IndependentWork[i]
			return IndependentWorkResult{Allowed: true, Work: &work, Reason: NeedsInputYieldReasonClean}
		}
	}
	return IndependentWorkResult{Reason: NeedsInputYieldReasonMissingProvenance}
}
