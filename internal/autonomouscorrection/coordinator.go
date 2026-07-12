// Package autonomouscorrection composes one bounded correction, final
// verification, and independent re-audit. It owns no retry loop.
package autonomouscorrection

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousauditapply"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
)

type AuthorityKind string

const (
	AuthorityVerification AuthorityKind = "verification_failure"
	AuthorityAudit        AuthorityKind = "audit_findings"
)

type Authority struct {
	Kind         AuthorityKind
	Verification *autonomouspolicy.VerificationEvidence
	FindingIDs   []string
}

type CycleRunner func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error)
type VerificationRunner func(context.Context, autonomousverification.Config) (autonomousverification.Result, error)
type AuditApplier func(context.Context, autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error)
type ResolutionApplier func(context.Context, autonomousauditapply.ResolutionConfig) (autonomousauditapply.Result, error)
type AuditLoader func(context.Context, string) (autonomousstate.AuditSnapshot, bool, error)

type Config struct {
	RepositoryRoot string
	TaskID         string
	Expected       autonomousstate.ExpectedState
	Authority      Authority
	Store          *autonomousstate.Store

	CorrectionCycle autonomouscycle.Config
	AuditCycle      autonomouscycle.Config
	FinalPlan       autonomousverification.Plan
	FinalTimeout    time.Duration
	FinalStdoutCap  int
	FinalStderrCap  int

	IDGenerator func() string
	Clock       func() time.Time

	CycleRunner        CycleRunner
	VerificationRunner VerificationRunner
	AuditApplier       AuditApplier
	ResolutionApplier  ResolutionApplier
	AuditLoader        AuditLoader
}

type Outcome string

const (
	OutcomeReturnedToSupervisor     Outcome = "returned_to_supervisor"
	OutcomeCorrectionStopped        Outcome = "correction_stopped"
	OutcomeFinalVerificationStopped Outcome = "final_verification_stopped"
	OutcomeAuditStopped             Outcome = "audit_stopped"
	OutcomeSafetyStopped            Outcome = "safety_stopped"
)

type Failure struct{ Stage, Reason string }

type Result struct {
	TaskID            string
	Outcome           Outcome
	Authority         Authority
	Checkpoint        gitstate.SourceSnapshot
	Correction        autonomouscycle.Result
	CorrectionOutput  autonomous.CorrectionOutput
	FinalVerification autonomousverification.Result
	Resolutions       []autonomousauditapply.Result
	Audit             autonomouscycle.Result
	AuditApplication  autonomousauditapply.Result
	State             autonomous.ExecutionState
	Failure           *Failure
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	result := Result{TaskID: cfg.TaskID, Authority: cloneAuthority(cfg.Authority)}
	n, snapshot, err := normalize(ctx, cfg)
	if err != nil {
		return stop(result, OutcomeSafetyStopped, "admission", err)
	}
	result.State = snapshot.State

	checkpoint, err := captureSource(ctx, n.CorrectionCycle)
	if err != nil {
		return stop(result, OutcomeSafetyStopped, "checkpoint", err)
	}
	result.Checkpoint = checkpoint
	revision, err := gitstate.PolicySourceRevision(checkpoint)
	if err != nil {
		return stop(result, OutcomeSafetyStopped, "checkpoint", err)
	}
	if dirty, err := captureDirty(ctx, n.CorrectionCycle); err != nil || dirty.CaptureError != "" || len(capturePaths(dirty)) != 0 {
		if err == nil {
			err = errors.New(strings.TrimSpace(dirty.CaptureError))
			if err.Error() == "" {
				err = fmt.Errorf("unsafe pre-correction dirty paths: %q", capturePaths(dirty))
			}
		}
		return stop(result, OutcomeSafetyStopped, "dirty_state", err)
	}

	audit, verification, failureTarget, auditRevision, err := admitAuthority(ctx, n, snapshot, revision)
	if err != nil {
		return stop(result, OutcomeSafetyStopped, "authority", err)
	}
	correctionCfg := n.CorrectionCycle
	correctionCfg.RepositoryRoot, correctionCfg.TaskID, correctionCfg.State = n.RepositoryRoot, n.TaskID, snapshot.State
	correctionCfg.SourceSafety = autonomouspolicy.SourceSafetySafe
	correctionCfg.Verification, correctionCfg.Audit, correctionCfg.CorrectionFailure = verification, audit, failureTarget
	correctionCfg.AllowPreExistingDirty = false
	correction, correctionErr := n.CycleRunner(ctx, correctionCfg)
	result.Correction = correction
	if correctionErr != nil {
		return stop(result, OutcomeCorrectionStopped, "correction_cycle", correctionErr)
	}
	if err := validateCorrection(correction, n.Authority, failureTarget, revision); err != nil {
		return stop(result, OutcomeCorrectionStopped, "correction_evidence", err)
	}
	output, err := autonomous.ParseCorrectionOutput(correction.Worker.RawOutput)
	if err != nil {
		return stop(result, OutcomeCorrectionStopped, "correction_output", err)
	}
	if err := validateCorrectionOutput(output, correction, n.Authority, failureTarget); err != nil {
		return stop(result, OutcomeCorrectionStopped, "correction_output", err)
	}
	result.CorrectionOutput = output
	if output.Outcome == autonomous.CorrectionOutcomeFailed {
		return stop(result, OutcomeCorrectionStopped, "correction_reported_failure", errors.New("corrector reported failure"))
	}
	if err := ctx.Err(); err != nil {
		return stop(result, OutcomeCorrectionStopped, "cancellation", err)
	}

	beforeFinal, err := captureSource(ctx, correctionCfg)
	if err != nil {
		return stop(result, OutcomeFinalVerificationStopped, "final_source_before", err)
	}
	beforeRevision, err := gitstate.PolicySourceRevision(beforeFinal)
	if err != nil || beforeRevision != correction.Source.FinalRevision {
		if err == nil {
			err = errors.New("corrected source revision changed before final verification")
		}
		return stop(result, OutcomeFinalVerificationStopped, "final_source_before", err)
	}
	finalRunID, finalOccurrenceID, err := nextTwoIDs(n.IDGenerator)
	if err != nil {
		return stop(result, OutcomeFinalVerificationStopped, "final_identity", err)
	}
	verificationRun, err := correctionCfg.Ledger.CreateRun(ctx, ledger.RunSpec{ID: finalRunID, TaskID: n.TaskID, Task: "final verification after correction", StartedAt: n.Clock().UTC()})
	if err != nil {
		return stop(result, OutcomeFinalVerificationStopped, "final_ledger", err)
	}
	finalResult, finalErr := n.VerificationRunner(ctx, autonomousverification.Config{RepositoryRoot: n.RepositoryRoot, TaskID: n.TaskID, RunID: finalRunID, OccurrenceID: finalOccurrenceID, SourceRevision: beforeRevision, Plan: n.FinalPlan, Purpose: autonomousverification.PurposeFinal, Timeout: n.FinalTimeout, StdoutCap: n.FinalStdoutCap, StderrCap: n.FinalStderrCap, Clock: n.Clock, AttemptID: n.IDGenerator, CommandRunner: autonomousverification.CommandRunner(correctionCfg.CommandRunner), Ledger: correctionCfg.Ledger, ArtifactPath: filepath.ToSlash(filepath.Join(".revolvr", "runs", finalRunID, "verification.json")), ArtifactWriter: correctionCfg.VerificationArtifactWriter})
	result.FinalVerification = finalResult
	if finalErr == nil {
		finalErr = finalResult.Validate()
	}
	completionStatus := ledger.StatusCompleted
	if finalErr != nil || finalResult.Outcome != autonomousverification.OutcomePassed || !finalResult.Gate.FinalSatisfied {
		completionStatus = ledger.StatusFailed
	}
	_, foundRun, completeErr := correctionCfg.Ledger.CompleteRun(ctx, verificationRun.ID, ledger.RunCompletion{Status: completionStatus, Summary: fmt.Sprintf("final verification %s", finalResult.Outcome), CompletedAt: n.Clock().UTC(), VerificationStatus: string(finalResult.Outcome)})
	if completeErr != nil || !foundRun {
		if completeErr == nil {
			completeErr = errors.New("final verification ledger run disappeared")
		}
		return stop(result, OutcomeFinalVerificationStopped, "final_ledger", completeErr)
	}
	if finalErr != nil || finalResult.Outcome != autonomousverification.OutcomePassed || !finalResult.Gate.FinalSatisfied {
		if finalErr == nil {
			finalErr = errors.New("final verification did not pass every required tier")
		}
		return stop(result, OutcomeFinalVerificationStopped, "final_verification", finalErr)
	}
	if correction.Worker.Run.CompletedAt == nil || !finalResult.StartedAt.After(*correction.Worker.Run.CompletedAt) {
		return stop(result, OutcomeFinalVerificationStopped, "final_freshness", errors.New("final verification is not newer than the completed correction"))
	}
	afterFinal, err := captureSource(ctx, correctionCfg)
	if err != nil {
		return stop(result, OutcomeFinalVerificationStopped, "final_source_after", err)
	}
	if gitstate.CompareSourceSnapshots(beforeFinal, afterFinal).Changed {
		return stop(result, OutcomeFinalVerificationStopped, "final_source_mutation", errors.New("final verification changed source"))
	}
	if err := ctx.Err(); err != nil {
		return stop(result, OutcomeFinalVerificationStopped, "cancellation", err)
	}
	finalEvidence := policyVerification(finalResult)

	current := snapshot
	if n.Authority.Kind == AuthorityAudit {
		for i, findingID := range output.ResolvedFindingIDs {
			evidence := append([]autonomous.EvidenceReference(nil), output.Evidence...)
			evidence = append(evidence, autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: correction.Worker.Artifacts.Output.Path, Detail: fmt.Sprintf("Exact corrector output SHA-256 %s (%d bytes).", correction.Worker.Artifacts.Output.SHA256, correction.Worker.Artifacts.Output.ByteSize)})
			evidence = append(evidence, finalEvidence.Summary.Evidence...)
			application, applyErr := n.ResolutionApplier(ctx, autonomousauditapply.ResolutionConfig{RepositoryRoot: n.RepositoryRoot, TaskID: n.TaskID, OperationID: fmt.Sprintf("%s-resolution-%02d", correction.Worker.RunID, i+1), Expected: current.Expected(), AuditRevision: auditRevision, Request: autonomousaudit.ResolutionRequest{FindingID: findingID, Status: autonomous.FindingResolutionStatusResolved, Evidence: evidence, CorrectionDecision: correction.Supervisor.Decision, DecisionReference: correction.Supervisor.DecisionReference, Verification: &finalEvidence, ResultingSourceRevision: beforeRevision}, CreatedAt: n.Clock().UTC(), Store: n.Store})
			if applyErr != nil {
				return stop(result, OutcomeCorrectionStopped, "finding_resolution", applyErr)
			}
			result.Resolutions = append(result.Resolutions, application)
			current = application.Current
		}
	}
	result.State = current.State
	if err := ctx.Err(); err != nil {
		return stop(result, OutcomeFinalVerificationStopped, "cancellation", err)
	}

	auditCfg := n.AuditCycle
	auditCfg.RepositoryRoot, auditCfg.TaskID, auditCfg.State = n.RepositoryRoot, n.TaskID, current.State
	auditCfg.SourceSafety, auditCfg.Verification, auditCfg.Audit, auditCfg.CorrectionFailure = autonomouspolicy.SourceSafetySafe, &finalEvidence, nil, nil
	auditCfg.LatestMutation = &autonomouspolicy.SourceMutation{TaskID: n.TaskID, RunID: correction.Worker.RunID, DecisionID: correction.Route.DecisionID, Action: autonomous.ActionCorrect, ResultingRevision: beforeRevision}
	auditResult, auditErr := n.CycleRunner(ctx, auditCfg)
	result.Audit = auditResult
	if auditErr != nil {
		return stop(result, OutcomeAuditStopped, "audit_cycle", auditErr)
	}
	if err := validateAudit(auditResult, correction, finalResult, beforeRevision); err != nil {
		return stop(result, OutcomeAuditStopped, "audit_evidence", err)
	}
	applied, err := n.AuditApplier(ctx, autonomousauditapply.ApplyConfig{RepositoryRoot: n.RepositoryRoot, TaskID: n.TaskID, OperationID: "reaudit-" + auditResult.Worker.RunID, Expected: current.Expected(), Cycle: auditResult, Verification: finalEvidence, LatestMutation: auditCfg.LatestMutation, CreatedAt: n.Clock().UTC(), Store: n.Store})
	if err != nil {
		return stop(result, OutcomeAuditStopped, "audit_persistence", err)
	}
	if err := applied.State.Validate(); err != nil || applied.State.TaskID != n.TaskID {
		if err == nil {
			err = errors.New("re-audit persistence returned wrong task state")
		}
		return stop(result, OutcomeAuditStopped, "audit_persistence", err)
	}
	result.AuditApplication, result.State, result.Outcome = applied, applied.State, OutcomeReturnedToSupervisor
	return result, nil
}

type normalized struct {
	Config
	Store *autonomousstate.Store
}

func normalize(ctx context.Context, cfg Config) (normalized, autonomousstate.Snapshot, error) {
	if strings.TrimSpace(cfg.RepositoryRoot) == "" || strings.TrimSpace(cfg.TaskID) == "" || cfg.IDGenerator == nil || cfg.Clock == nil {
		return normalized{}, autonomousstate.Snapshot{}, errors.New("repository_root, task_id, ID generator, and clock are required")
	}
	if err := cfg.Expected.Validate(); err != nil || !cfg.Expected.Exists {
		return normalized{}, autonomousstate.Snapshot{}, errors.New("exact existing expected state is required")
	}
	if err := cfg.FinalPlan.Validate(); err != nil {
		return normalized{}, autonomousstate.Snapshot{}, err
	}
	if cfg.FinalTimeout <= 0 || cfg.FinalStdoutCap <= 0 || cfg.FinalStderrCap <= 0 {
		return normalized{}, autonomousstate.Snapshot{}, errors.New("positive final verification bounds are required")
	}
	if cfg.CorrectionCycle.Ledger == nil {
		return normalized{}, autonomousstate.Snapshot{}, errors.New("writable ledger is required for distinct final verification evidence")
	}
	store := cfg.Store
	var err error
	if store == nil {
		store, err = autonomousstate.New(autonomousstate.Config{RepositoryRoot: cfg.RepositoryRoot})
		if err != nil {
			return normalized{}, autonomousstate.Snapshot{}, err
		}
	}
	snapshot, found, err := store.Load(ctx, cfg.TaskID)
	if err != nil || !found {
		return normalized{}, snapshot, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if snapshot.SHA256 != cfg.Expected.SHA256 || snapshot.ByteSize != cfg.Expected.ByteSize {
		return normalized{}, snapshot, autonomousstate.ErrStaleWrite
	}
	if snapshot.State.Lifecycle != autonomous.LifecycleStateReady {
		return normalized{}, snapshot, errors.New("ready lifecycle is required")
	}
	if cfg.CycleRunner == nil {
		cfg.CycleRunner = autonomouscycle.Run
	}
	if cfg.VerificationRunner == nil {
		cfg.VerificationRunner = autonomousverification.Execute
	}
	if cfg.AuditApplier == nil {
		cfg.AuditApplier = autonomousauditapply.ApplyAuditResult
	}
	if cfg.ResolutionApplier == nil {
		cfg.ResolutionApplier = autonomousauditapply.ApplyFindingResolution
	}
	if cfg.AuditLoader == nil {
		cfg.AuditLoader = store.LoadCurrentAudit
	}
	cfg.Store = store
	return normalized{Config: cfg, Store: store}, snapshot, nil
}

func admitAuthority(ctx context.Context, n normalized, s autonomousstate.Snapshot, revision string) (*autonomouspolicy.AuditEvidence, *autonomouspolicy.VerificationEvidence, *autonomous.VerificationFailureTarget, int64, error) {
	switch n.Authority.Kind {
	case AuthorityVerification:
		if n.Authority.Verification == nil || len(n.Authority.FindingIDs) != 0 {
			return nil, nil, nil, 0, errors.New("verification authority is incomplete")
		}
		v := cloneVerification(n.Authority.Verification)
		if v.Summary.Status != autonomous.VerificationStatusFailed || v.SourceRevision != revision {
			return nil, nil, nil, 0, errors.New("verification authority is not the exact current failed occurrence")
		}
		t := &autonomous.VerificationFailureTarget{TaskID: n.TaskID, RunID: v.Summary.RunID, OccurrenceID: v.Summary.OccurrenceID, SourceRevision: v.SourceRevision, Status: v.Summary.Status, Evidence: append([]autonomous.EvidenceReference(nil), v.Summary.Evidence...)}
		if err := t.Validate(); err != nil {
			return nil, nil, nil, 0, err
		}
		return nil, v, t, 0, nil
	case AuthorityAudit:
		if n.Authority.Verification != nil || len(n.Authority.FindingIDs) == 0 {
			return nil, nil, nil, 0, errors.New("audit authority is incomplete")
		}
		current, found, err := n.AuditLoader(ctx, n.TaskID)
		if err != nil || !found {
			return nil, nil, nil, 0, errors.Join(err, errors.New("current committed audit authority is missing"))
		}
		if current.PolicyEvidence.SourceRevision != revision {
			return nil, nil, nil, 0, errors.New("audit authority is stale for checkpoint")
		}
		ids := append([]string(nil), n.Authority.FindingIDs...)
		if hasDuplicate(ids) {
			return nil, nil, nil, 0, errors.New("audit authority contains duplicate finding IDs")
		}
		known := map[string]autonomous.AuditFinding{}
		for _, f := range current.Report.Findings {
			known[f.ID] = f
		}
		for _, id := range ids {
			f, ok := known[id]
			if !ok || f.Significance != autonomous.FindingSignificanceBlocking || resolutionStatus(s.State, id) != autonomous.FindingResolutionStatusOpen {
				return nil, nil, nil, 0, fmt.Errorf("finding %q is not an open blocking current-audit finding", id)
			}
		}
		v := cloneVerification(&current.History.Record.Verification)
		a := current.PolicyEvidence
		return &a, v, nil, current.Revision, nil
	default:
		return nil, nil, nil, 0, fmt.Errorf("unknown correction authority %q", n.Authority.Kind)
	}
}

func validateCorrection(r autonomouscycle.Result, a Authority, failure *autonomous.VerificationFailureTarget, revision string) error {
	if r.Outcome != autonomouscycle.OutcomeVerifiedChangesCommitted || r.Failure != nil || r.Route == nil || r.Route.Action != autonomous.ActionCorrect || r.Worker.Action != autonomous.ActionCorrect || r.Source.AdmissionRevision != revision || r.Source.FinalRevision == revision {
		return errors.New("correction did not produce committed source changes from exact checkpoint")
	}
	if r.Worker.Verification.Tiered == nil || r.Worker.Verification.Tiered.Validate() != nil || r.Worker.Verification.Tiered.Purpose != autonomousverification.PurposeFast || r.Worker.Verification.Tiered.Outcome != autonomousverification.OutcomePassed || r.Worker.Verification.Tiered.SourceRevision != r.Source.FinalRevision || r.Worker.Verification.Tiered.RunID != r.Worker.RunID {
		return errors.New("correction lacks ordinary passed fast verification")
	}
	if r.Supervisor.Decision == nil {
		return errors.New("correction decision is missing")
	}
	if strings.TrimSpace(r.Worker.Artifacts.Output.Path) == "" || r.Worker.Artifacts.Output.SHA256 == "" || r.Worker.Artifacts.Output.ByteSize <= 0 {
		return errors.New("correction output artifact identity is missing")
	}
	if a.Kind == AuthorityAudit {
		if !reflect.DeepEqual(r.Supervisor.Decision.FindingIDs, a.FindingIDs) || r.Supervisor.Decision.VerificationFailure != nil {
			return errors.New("correction decision does not preserve exact cited finding authority")
		}
	}
	if a.Kind == AuthorityVerification {
		if err := autonomous.ValidateVerificationCorrectionDecision(*r.Supervisor.Decision, *failure); err != nil {
			return err
		}
	}
	return nil
}

func validateCorrectionOutput(o autonomous.CorrectionOutput, r autonomouscycle.Result, a Authority, f *autonomous.VerificationFailureTarget) error {
	if o.TaskID != r.TaskID || o.WorkerRunID != r.Worker.RunID || o.DecisionID != r.Route.DecisionID {
		return errors.New("correction output identity mismatch")
	}
	if a.Kind == AuthorityAudit && !reflect.DeepEqual(o.FindingIDs, a.FindingIDs) {
		return errors.New("correction output changed cited finding authority")
	}
	if a.Kind == AuthorityVerification && (o.VerificationFailure == nil || !reflect.DeepEqual(*o.VerificationFailure, *f)) {
		return errors.New("correction output changed verification-failure authority")
	}
	return nil
}

func validateAudit(a, c autonomouscycle.Result, v autonomousverification.Result, revision string) error {
	if a.Outcome != autonomouscycle.OutcomeReadOnlyCompleted || a.Route == nil || a.Route.Action != autonomous.ActionAudit || a.Worker.Action != autonomous.ActionAudit {
		return errors.New("re-audit did not complete independently")
	}
	ids := []string{a.Supervisor.RunID, a.Worker.RunID, v.RunID, c.Supervisor.RunID, c.Worker.RunID}
	if hasDuplicate(ids) {
		return errors.New("supervisor, corrector, final verification, and auditor runs must be distinct")
	}
	if a.Source.AdmissionRevision != revision || a.Source.WorkerRevision != revision {
		return errors.New("re-audit source is stale")
	}
	if !a.Worker.Run.StartedAt.After(v.EndedAt) {
		return errors.New("re-audit is not newer than final verification")
	}
	return nil
}

func policyVerification(r autonomousverification.Result) autonomouspolicy.VerificationEvidence {
	status := autonomous.VerificationStatusFailed
	if r.Outcome == autonomousverification.OutcomePassed && r.Gate.FinalSatisfied {
		status = autonomous.VerificationStatusPassed
	}
	e := []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: fmt.Sprintf("ledger:%s:verification:%s", r.RunID, r.OccurrenceID), Detail: "Distinct final verification for corrected source."}}
	if r.Artifact != nil {
		e = append(e, autonomous.EvidenceReference{Kind: autonomous.EvidenceKindVerification, Reference: r.Artifact.Path, Detail: fmt.Sprintf("Final verification artifact SHA-256 %s (%d bytes).", r.Artifact.SHA256, r.Artifact.ByteSize)})
	}
	copy := r
	gate := r.Gate
	return autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: r.TaskID, Status: status, Command: "tiered final verification", Summary: fmt.Sprintf("Final verification classified %s.", r.Outcome), RunID: r.RunID, OccurrenceID: r.OccurrenceID, Evidence: e, Tiered: &copy}, SourceRevision: r.SourceRevision, Tiered: &gate}
}

func captureSource(ctx context.Context, c autonomouscycle.Config) (gitstate.SourceSnapshot, error) {
	fn := c.SourceSnapshotter
	if fn == nil {
		fn = gitstate.CaptureSourceSnapshot
	}
	return fn(ctx, gitstate.SourceSnapshotConfig{WorkingDir: c.RepositoryRoot, GitExecutable: c.GitExecutable, Timeout: c.GitTimeout, StdoutCap: c.GitStdoutCap, StderrCap: c.GitStderrCap, CommandRunner: gitstate.CommandRunner(c.CommandRunner)})
}
func captureDirty(ctx context.Context, c autonomouscycle.Config) (gitstate.Capture, error) {
	fn := c.DirtyCapture
	if fn == nil {
		fn = gitstate.CaptureDirtyWorktree
	}
	return fn(ctx, gitstate.Config{WorkingDir: c.RepositoryRoot, GitExecutable: c.GitExecutable, Timeout: c.GitTimeout, StdoutCap: c.GitStdoutCap, StderrCap: c.GitStderrCap, CommandRunner: gitstate.CommandRunner(c.CommandRunner)})
}
func capturePaths(c gitstate.Capture) []string {
	v := c.Paths
	if len(c.DirtyFiles) != 0 {
		v = c.DirtyFiles
	}
	out := append([]string(nil), v...)
	sort.Strings(out)
	return out
}
func nextTwoIDs(fn func() string) (string, string, error) {
	a, b := strings.TrimSpace(fn()), strings.TrimSpace(fn())
	if a == "" || b == "" || a == b {
		return "", "", errors.New("distinct final verification run and occurrence IDs are required")
	}
	return a, b, nil
}
func resolutionStatus(s autonomous.ExecutionState, id string) autonomous.FindingResolutionStatus {
	for _, r := range s.FindingResolutions {
		if r.FindingID == id {
			return r.Status
		}
	}
	return ""
}
func hasDuplicate(v []string) bool {
	m := map[string]bool{}
	for _, s := range v {
		if s == "" || m[s] {
			return true
		}
		m[s] = true
	}
	return false
}
func cloneVerification(v *autonomouspolicy.VerificationEvidence) *autonomouspolicy.VerificationEvidence {
	if v == nil {
		return nil
	}
	out := *v
	out.Summary.Evidence = append([]autonomous.EvidenceReference(nil), v.Summary.Evidence...)
	return &out
}
func cloneAuthority(a Authority) Authority {
	a.FindingIDs = append([]string(nil), a.FindingIDs...)
	a.Verification = cloneVerification(a.Verification)
	return a
}
func stop(r Result, o Outcome, stage string, err error) (Result, error) {
	if err == nil {
		err = errors.New("unknown failure")
	}
	r.Outcome = o
	r.Failure = &Failure{Stage: stage, Reason: err.Error()}
	return r, fmt.Errorf("bounded autonomous correction %s: %w", stage, err)
}
