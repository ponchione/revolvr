package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"revolvr/internal/id"
)

type Engine struct {
	store     Store
	executor  GateExecutor
	artifacts ArtifactWriter
	observer  AuthorityObserver
	clock     func() time.Time
	newID     func() string
}

func NewEngine(config EngineConfig) (*Engine, error) {
	if config.Store == nil || config.Executor == nil || config.Artifacts == nil || config.Observer == nil {
		return nil, errors.New("verification engine requires store, executor, artifacts, and authority observer")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.NewID == nil {
		config.NewID = id.New
	}
	return &Engine{
		store: config.Store, executor: config.Executor, artifacts: config.Artifacts,
		observer: config.Observer, clock: config.Clock, newID: config.NewID,
	}, nil
}

func (e *Engine) Run(ctx context.Context, request Request) (EngineResult, error) {
	pinned, err := Pin(request.Pinned)
	if err != nil {
		return EngineResult{}, err
	}
	if request.Purpose != PurposeBaseline && request.Purpose != PurposeCandidate && request.Purpose != PurposeFinal {
		return EngineResult{}, invalidPlan("purpose %q is invalid", request.Purpose)
	}
	if request.Purpose == PurposeFinal {
		foundFinal := false
		for _, gate := range pinned.Plan.Gates {
			foundFinal = foundFinal || gate.Tier == TierFinal
		}
		if !foundFinal {
			return EngineResult{}, invalidPlan("final verification requires a Tier 4 gate")
		}
	}
	started := e.now()
	persisted := PersistedRun{
		ID: e.newID(), EventID: e.newID(), Pinned: pinned, Purpose: request.Purpose,
		StartedAt: started,
	}
	result := EngineResult{VerificationRunID: persisted.ID, AuthorityAction: pinned.Plan.AuthorityChangePolicy}
	for index, gate := range pinned.Plan.Gates {
		check, directive, runErr := e.runGate(ctx, pinned, request.Purpose, index+1, gate)
		if runErr != nil {
			if errors.Is(runErr, ErrArtifact) {
				check.Outcome = OutcomeArtifactFailed
				result.Checks = append(result.Checks, check)
				result.Status = RunInfrastructureFailed
			}
			return result, runErr
		}
		persisted.Checks = append(persisted.Checks, check)
		if directive != "" {
			result.DualRunRequired = directive == AuthorityDualRun
			result.EscalationRequired = directive == AuthorityEscalate
		}
		if directive != "" || stopAfter(check.Outcome) {
			break
		}
	}
	baseline := append([]PersistedCheck(nil), request.Baseline...)
	var candidate []PersistedCheck
	for _, check := range persisted.Checks {
		if check.Gate.Tier == TierAdmissionBaseline {
			baseline = append(baseline, check)
		} else {
			candidate = append(candidate, check)
		}
	}
	persisted.Differential = ClassifyDifferential(baseline, candidate)
	persisted.Status = aggregateStatus(persisted.Checks)
	persisted.CompletedAt = e.now()
	if persisted.CompletedAt.Before(persisted.StartedAt) {
		persisted.CompletedAt = persisted.StartedAt
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := e.store.Persist(persistCtx, persisted); err != nil {
		return EngineResult{}, fmt.Errorf("%w: %v", ErrPersistence, err)
	}
	result.Status = persisted.Status
	result.Checks = persisted.Checks
	result.Differential = persisted.Differential
	return result, nil
}

func (e *Engine) runGate(ctx context.Context, pinned PinnedPlan, purpose Purpose, ordinal int, gate Gate) (PersistedCheck, AuthorityChangePolicy, error) {
	fingerprint, err := ExecutionFingerprint(pinned, gate)
	if err != nil {
		return PersistedCheck{}, "", err
	}
	occurred := e.now()
	check := PersistedCheck{
		ID: e.newID(), Ordinal: ordinal, Gate: gate, ExecutionFingerprint: fingerprint,
		VerifierProtocolVersion:       pinned.VerifierProtocol,
		VerifierImplementationVersion: pinned.VerifierImplementation,
		OriginalExecutedAt:            occurred, OccurredAt: occurred, StartedAt: occurred, CompletedAt: occurred,
	}
	snapshot, observeErr := e.observer.Observe(ctx, gate)
	if outcome, authorityErr := compareAuthority(ctx, pinned, gate, snapshot, observeErr); outcome != "" {
		check.Outcome = outcome
		check.SandboxSpecificationSHA256 = notExecutedSHA(fingerprint, check.Outcome)
		check.SandboxEvidence = failureEvidence(check.Outcome, authorityErr)
		if err := e.attachArtifacts(ctx, &check, nil, []byte(authorityErr.Error())); err != nil {
			return check, "", err
		}
		return check, authorityDirective(pinned.Plan.AuthorityChangePolicy, check.Outcome), nil
	}
	forceFresh := gate.Tier == TierFinal && (purpose == PurposeFinal || pinned.Plan.RequireFreshFinal)
	if pinned.Plan.AllowReuse && !forceFresh {
		reusable, found, lookupErr := e.store.FindReusable(ctx, fingerprint)
		if lookupErr != nil {
			return check, "", fmt.Errorf("%w: lookup reusable verification: %v", ErrPersistence, lookupErr)
		}
		if found {
			postSnapshot, postErr := e.observer.Observe(ctx, gate)
			if outcome, authorityErr := compareAuthority(ctx, pinned, gate, postSnapshot, postErr); outcome != "" {
				check.Outcome = outcome
				check.SandboxSpecificationSHA256 = notExecutedSHA(fingerprint, check.Outcome)
				check.SandboxEvidence = failureEvidence(check.Outcome, authorityErr)
				if err := e.attachArtifacts(ctx, &check, nil, []byte(authorityErr.Error())); err != nil {
					return check, "", err
				}
				return check, authorityDirective(pinned.Plan.AuthorityChangePolicy, check.Outcome), nil
			}
			check.ReusedFromCheckID = reusable.ID
			check.OriginalExecutedAt = reusable.OriginalExecutedAt
			check.ExitCode = reusable.ExitCode
			check.Stdout = reusable.Stdout
			check.Stderr = reusable.Stderr
			check.ParsedResult = append(json.RawMessage(nil), reusable.ParsedResult...)
			check.SandboxEvidence = append(json.RawMessage(nil), reusable.SandboxEvidence...)
			check.FailureSignatures = append([]string(nil), reusable.FailureSignatures...)
			check.SandboxSpecificationSHA256 = reusable.SandboxSpecificationSHA256
			switch reusable.Outcome {
			case OutcomePassed:
				check.Outcome = OutcomePassedReused
			case OutcomeFailed:
				check.Outcome = OutcomeUnchangedFailureReused
			default:
				return check, "", fmt.Errorf("%w: store returned nonreusable outcome %q", ErrPersistence, reusable.Outcome)
			}
			return check, "", nil
		}
	}
	execution, executionErr := e.executor.Execute(ctx, GateExecution{SandboxID: e.newID(), Pinned: pinned, Gate: gate})
	check.SandboxSpecificationSHA256 = execution.SandboxSpecificationSHA256
	check.StartedAt = normalizeTime(execution.StartedAt, occurred)
	check.CompletedAt = normalizeTime(execution.CompletedAt, check.StartedAt)
	check.OriginalExecutedAt = check.CompletedAt
	check.OccurredAt = check.CompletedAt
	check.ExitCode = &execution.ExitCode
	check.TimedOut = execution.TimedOut
	check.Cancelled = execution.Cancelled
	check.SandboxEvidence = canonicalEvidence(execution.Evidence)
	if errors.Is(executionErr, ErrArtifact) {
		check.Outcome = OutcomeArtifactFailed
		return check, "", executionErr
	}
	postSnapshot, postErr := e.observer.Observe(ctx, gate)
	if outcome, authorityErr := compareAuthority(ctx, pinned, gate, postSnapshot, postErr); outcome != "" {
		check.Outcome = outcome
		check.TimedOut = false
		check.Cancelled = outcome == OutcomeCancelled
		check.SandboxEvidence = postExecutionAuthorityEvidence(check.SandboxEvidence, check.Outcome, authorityErr)
		check.ParsedResult = json.RawMessage(`{}`)
		if err := e.attachArtifacts(ctx, &check, execution.Stdout, execution.Stderr); err != nil {
			return check, "", err
		}
		return check, authorityDirective(pinned.Plan.AuthorityChangePolicy, check.Outcome), nil
	}
	switch {
	case execution.Cancelled || errors.Is(ctx.Err(), context.Canceled):
		check.Outcome = OutcomeCancelled
	case execution.TimedOut:
		check.Outcome = OutcomeTimedOut
	case execution.MissingCommand || execution.ExitCode == 127:
		check.Outcome = OutcomeMissingCommand
	case executionErr != nil:
		check.Outcome = OutcomeInfrastructureFailed
	case execution.StdoutTruncatedBytes > 0 || execution.StderrTruncatedBytes > 0 || int64(len(execution.Stdout)) > gate.OutputPolicy.StdoutMaxBytes || int64(len(execution.Stderr)) > gate.OutputPolicy.StderrMaxBytes:
		check.Outcome = OutcomeAmbiguous
	default:
		parsed, failures, parseErr := parseOutput(gate, execution.Stdout, execution.ExitCode)
		check.ParsedResult = parsed
		check.FailureSignatures = failures
		if parseErr != nil {
			check.Outcome = OutcomeMalformedOutput
			check.ParsedResult = json.RawMessage(`{}`)
			check.FailureSignatures = []string{"malformed-output:" + gate.ID}
		} else if execution.ExitCode == 0 {
			check.Outcome = OutcomePassed
		} else {
			check.Outcome = OutcomeFailed
			if len(check.FailureSignatures) == 0 {
				check.FailureSignatures = []string{fmt.Sprintf("gate:%s:exit:%d", gate.ID, execution.ExitCode)}
			}
		}
	}
	if len(check.ParsedResult) == 0 {
		check.ParsedResult = json.RawMessage(`{}`)
	}
	if len(check.SandboxEvidence) == 0 {
		check.SandboxEvidence = failureEvidence(check.Outcome, executionErr)
	}
	check.TimedOut = check.Outcome == OutcomeTimedOut
	check.Cancelled = check.Outcome == OutcomeCancelled
	if err := e.attachArtifacts(ctx, &check, execution.Stdout, execution.Stderr); err != nil {
		return check, "", err
	}
	return check, "", nil
}

func (e *Engine) attachArtifacts(ctx context.Context, check *PersistedCheck, stdout, stderr []byte) error {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	stdoutArtifact, err := e.artifacts.Materialize(recordCtx, "verification-stdout", "application/octet-stream", stdout)
	if err != nil {
		return fmt.Errorf("%w: stdout: %v", ErrArtifact, err)
	}
	stderrArtifact, err := e.artifacts.Materialize(recordCtx, "verification-stderr", "application/octet-stream", stderr)
	if err != nil {
		return fmt.Errorf("%w: stderr: %v", ErrArtifact, err)
	}
	check.Stdout = stdoutArtifact
	check.Stderr = stderrArtifact
	return nil
}

func (e *Engine) now() time.Time {
	return e.clock().UTC().Truncate(time.Microsecond)
}

func observedFailure(ctx context.Context, err error) Outcome {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return OutcomeCancelled
	case errors.Is(err, ErrStaleEnvironment):
		return OutcomeStaleEnvironment
	case errors.Is(err, ErrAuthorityChanged):
		return OutcomeAuthorityTampered
	case errors.Is(err, ErrStaleSource):
		return OutcomeStaleSource
	default:
		return OutcomeInfrastructureFailed
	}
}

func compareAuthority(ctx context.Context, pinned PinnedPlan, gate Gate, snapshot AuthoritySnapshot, observeErr error) (Outcome, error) {
	if observeErr != nil {
		return observedFailure(ctx, observeErr), observeErr
	}
	if snapshot.Source != gate.Source {
		err := fmt.Errorf("%w: expected %s/%s, observed %s/%s", ErrStaleSource, gate.Source.Commit, gate.Source.Tree, snapshot.Source.Commit, snapshot.Source.Tree)
		return OutcomeStaleSource, err
	}
	if snapshot.ProjectEnvironmentSHA256 != pinned.ProjectEnvironment.SHA256 {
		err := fmt.Errorf("%w: expected %s, observed %s", ErrStaleEnvironment, pinned.ProjectEnvironment.SHA256, snapshot.ProjectEnvironmentSHA256)
		return OutcomeStaleEnvironment, err
	}
	if !reflect.DeepEqual(snapshot.AuthorityInputs, gate.AuthorityInputs) {
		err := fmt.Errorf("%w: gate %s material inputs differ", ErrAuthorityChanged, gate.ID)
		return OutcomeAuthorityTampered, err
	}
	return "", nil
}

func authorityDirective(policy AuthorityChangePolicy, outcome Outcome) AuthorityChangePolicy {
	if outcome != OutcomeAuthorityTampered {
		return ""
	}
	return policy
}

func stopAfter(outcome Outcome) bool {
	switch outcome {
	case OutcomeCancelled, OutcomeTimedOut, OutcomeIncomplete, OutcomeInfrastructureFailed,
		OutcomeAmbiguous, OutcomeStaleSource, OutcomeStaleEnvironment, OutcomeAuthorityTampered:
		return true
	default:
		return false
	}
}

func aggregateStatus(checks []PersistedCheck) RunStatus {
	status := RunPassed
	for _, check := range checks {
		switch check.Outcome {
		case OutcomeCancelled:
			return RunCancelled
		case OutcomeInfrastructureFailed, OutcomeArtifactFailed:
			status = RunInfrastructureFailed
		case OutcomeAmbiguous:
			if status != RunInfrastructureFailed {
				status = RunAmbiguous
			}
		case OutcomeTimedOut, OutcomeIncomplete, OutcomeMissingCommand:
			if status != RunInfrastructureFailed && status != RunAmbiguous {
				status = RunIncomplete
			}
		default:
			if check.Outcome.Failed() && status == RunPassed {
				status = RunFailed
			}
		}
	}
	return status
}

func normalizeTime(value, fallback time.Time) time.Time {
	if value.IsZero() || value.Before(fallback) {
		return fallback
	}
	return value.UTC().Truncate(time.Microsecond)
}

func notExecutedSHA(fingerprint string, outcome Outcome) string {
	return hashBytes([]byte("revolvr-not-executed-v1\x00" + fingerprint + "\x00" + string(outcome)))
}

func failureEvidence(outcome Outcome, err error) json.RawMessage {
	message := ""
	if err != nil {
		message = err.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	raw, _ := json.Marshal(map[string]any{"schema_version": "revolvr-verification-failure-evidence-v1", "outcome": outcome, "error": message})
	return raw
}

func postExecutionAuthorityEvidence(execution json.RawMessage, outcome Outcome, err error) json.RawMessage {
	message := err.Error()
	if len(message) > 4096 {
		message = message[:4096]
	}
	raw, _ := json.Marshal(map[string]any{
		"schema_version": "revolvr-post-execution-authority-failure-v1",
		"outcome":        outcome, "error": message, "sandbox_execution": execution,
	})
	return raw
}

func canonicalEvidence(raw json.RawMessage) json.RawMessage {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return nil
	}
	return canonical
}
