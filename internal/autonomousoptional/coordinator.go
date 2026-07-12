package autonomousoptional

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousattempt"
	"revolvr/internal/autonomousauditapply"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/ledger"
)

type CycleRunner func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error)
type AuditApplier func(context.Context, autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error)
type AuditLoader func(context.Context, string) (autonomousstate.AuditSnapshot, bool, error)

type Ledger interface {
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
}

type Config struct {
	RepositoryRoot string
	TaskID         string
	Expected       autonomousstate.ExpectedState
	Assessment     autonomous.OptionalRoleAssessment
	Store          *autonomousstate.Store
	Ledger         Ledger

	Admission              autonomousattempt.AdmissionConfig
	CompletionOperationID  string
	DispositionOperationID string
	AuditOperationID       string
	RoleCycle              autonomouscycle.Config
	AuditCycle             autonomouscycle.Config
	BehaviorPreservation   []autonomous.EvidenceReference

	Clock        func() time.Time
	CycleRunner  CycleRunner
	AuditApplier AuditApplier
	AuditLoader  AuditLoader
}

type Outcome string

const (
	OutcomeNotApplicable      Outcome = "not_applicable"
	OutcomeNoChange           Outcome = "no_change"
	OutcomeSourceChanged      Outcome = "source_changed"
	OutcomeAdmissionStopped   Outcome = "admission_stopped"
	OutcomeRoleStopped        Outcome = "role_stopped"
	OutcomeAuditStopped       Outcome = "audit_stopped"
	OutcomePersistenceStopped Outcome = "persistence_stopped"
)

type Result struct {
	TaskID           string
	Outcome          Outcome
	Admission        autonomousattempt.Result
	RoleCycle        autonomouscycle.Result
	AuditCycle       autonomouscycle.Result
	AuditApplication autonomousauditapply.Result
	Completion       autonomousattempt.Result
	Application      ApplyResult
	Failure          error
}

// Run performs one conditional documentation or simplification operation. A
// run assessment admits exactly one AW-15 attempt; the nested audit is a stage
// of that attempt, not another charged action.
func Run(ctx context.Context, cfg Config) (Result, error) {
	result := Result{TaskID: cfg.TaskID}
	if replay, ok, err := replayResult(ctx, cfg); err != nil {
		return stopped(result, OutcomePersistenceStopped, err)
	} else if ok {
		return replay, nil
	}
	n, snapshot, currentAudit, err := normalize(ctx, cfg)
	if err != nil {
		return stopped(result, OutcomePersistenceStopped, err)
	}
	gate := gateFromAudit(currentAudit)
	if n.Assessment.Disposition == autonomous.OptionalRoleDispositionNotApplicable {
		occurrence, err := skippedOccurrence(n, snapshot, gate)
		if err != nil {
			return stopped(result, OutcomePersistenceStopped, err)
		}
		applied, err := recordAndApply(ctx, n, snapshot.Expected(), occurrence)
		result.Application = applied
		if err != nil {
			return stopped(result, OutcomePersistenceStopped, err)
		}
		result.Outcome = OutcomeNotApplicable
		return result, nil
	}

	admissionCfg := n.Admission
	admissionCfg.TaskID, admissionCfg.Expected, admissionCfg.Action = n.TaskID, snapshot.Expected(), n.Assessment.Decision.Action
	admissionCfg.Decision, admissionCfg.Reference = n.Assessment.Decision, n.Assessment.DecisionReference
	admissionCfg.SourceRevision, admissionCfg.SourceSafety, admissionCfg.Store = n.Assessment.SourceRevision, autonomouspolicy.SourceSafetySafe, n.Store
	admission, err := autonomousattempt.Admit(ctx, admissionCfg)
	result.Admission = admission
	if err != nil || admission.Disposition == autonomousattempt.DispositionBlocked {
		if err != nil {
			return stopped(result, OutcomeAdmissionStopped, err)
		}
		result.Outcome = OutcomeAdmissionStopped
		return result, nil
	}
	started := n.Clock().UTC()
	roleCfg := n.RoleCycle
	roleCfg.RepositoryRoot, roleCfg.TaskID, roleCfg.State = n.RepositoryRoot, n.TaskID, admission.Current.State
	roleCfg.SourceSafety = autonomouspolicy.SourceSafetySafe
	roleCfg.Verification = cloneVerification(n.RoleCycle.Verification)
	roleCfg.Audit = cloneAudit(n.RoleCycle.Audit)
	role, roleErr := n.CycleRunner(ctx, roleCfg)
	result.RoleCycle = role
	if roleErr != nil || validateRoleCycle(n.Assessment, role) != nil {
		if roleErr == nil {
			roleErr = validateRoleCycle(n.Assessment, role)
		}
		completion, completeErr := completeFailure(n, admission.Current.Expected(), role, nil, started, roleErr)
		result.Completion = completion
		return stopped(result, OutcomeRoleStopped, errors.Join(roleErr, completeErr))
	}
	if role.Outcome == autonomouscycle.OutcomeWorkerNoChanges {
		observation := autonomousattempt.ObserveCycle(role, nil)
		completion, err := complete(n, admission.Current.Expected(), observation, started)
		result.Completion = completion
		if err != nil {
			return stopped(result, OutcomePersistenceStopped, err)
		}
		occurrence, err := workerOccurrence(n, completion.Current.State, role, gate, autonomous.OptionalRoleOutcomeNoChange, "No source change was observed from the authorized optional role.")
		if err != nil {
			return stopped(result, OutcomePersistenceStopped, err)
		}
		applied, err := recordAndApply(ctx, n, completion.Current.Expected(), occurrence)
		result.Application = applied
		if err != nil {
			return stopped(result, OutcomePersistenceStopped, err)
		}
		result.Outcome = OutcomeNoChange
		return result, nil
	}
	if role.Outcome != autonomouscycle.OutcomeVerifiedChangesCommitted {
		err := fmt.Errorf("optional-role cycle returned unsupported outcome %q", role.Outcome)
		completion, completeErr := completeFailure(n, admission.Current.Expected(), role, nil, started, err)
		result.Completion = completion
		return stopped(result, OutcomeRoleStopped, errors.Join(err, completeErr))
	}

	verification, err := roleVerification(role)
	if err != nil {
		completion, completeErr := completeFailure(n, admission.Current.Expected(), role, nil, started, err)
		result.Completion = completion
		return stopped(result, OutcomeRoleStopped, errors.Join(err, completeErr))
	}
	auditCfg := n.AuditCycle
	auditCfg.RepositoryRoot, auditCfg.TaskID, auditCfg.State = n.RepositoryRoot, n.TaskID, admission.Current.State
	auditCfg.SourceSafety, auditCfg.Verification, auditCfg.Audit = autonomouspolicy.SourceSafetySafe, &verification, nil
	auditCfg.LatestMutation = &autonomouspolicy.SourceMutation{TaskID: n.TaskID, RunID: role.Worker.RunID, DecisionID: role.Route.DecisionID, Action: role.Route.Action, ResultingRevision: role.Source.FinalRevision}
	auditResult, auditErr := n.CycleRunner(ctx, auditCfg)
	result.AuditCycle = auditResult
	if auditErr != nil || validateAuditCycle(role, auditResult, verification) != nil {
		if auditErr == nil {
			auditErr = validateAuditCycle(role, auditResult, verification)
		}
		completion, completeErr := completeFailure(n, admission.Current.Expected(), role, &auditResult, started, auditErr)
		result.Completion = completion
		return stopped(result, OutcomeAuditStopped, errors.Join(auditErr, completeErr))
	}
	appliedAudit, auditErr := n.AuditApplier(context.WithoutCancel(ctx), autonomousauditapply.ApplyConfig{RepositoryRoot: n.RepositoryRoot, TaskID: n.TaskID, OperationID: n.AuditOperationID, Expected: admission.Current.Expected(), Cycle: auditResult, Verification: verification, LatestMutation: auditCfg.LatestMutation, CreatedAt: n.Clock().UTC(), Store: n.Store})
	result.AuditApplication = appliedAudit
	if auditErr != nil {
		latest, found, loadErr := n.Store.Load(context.WithoutCancel(ctx), n.TaskID)
		expected := admission.Current.Expected()
		if loadErr == nil && found {
			expected = latest.Expected()
		}
		completion, completeErr := completeFailure(n, expected, role, &auditResult, started, auditErr)
		result.Completion = completion
		return stopped(result, OutcomeAuditStopped, errors.Join(auditErr, loadErr, completeErr))
	}
	observation := combinedObservation(role, auditResult)
	completion, err := complete(n, appliedAudit.Current.Expected(), observation, started)
	result.Completion = completion
	if err != nil {
		return stopped(result, OutcomePersistenceStopped, err)
	}
	freshGate := autonomous.OptionalRoleGate{SourceRevision: role.Source.FinalRevision, VerificationRunID: verification.Summary.RunID, VerificationOccurrenceID: verification.Summary.OccurrenceID, AuditSupervisorRunID: auditResult.Supervisor.RunID, AuditWorkerRunID: auditResult.Worker.RunID, AuditRevision: appliedAudit.AuditRevision}
	occurrence, err := workerOccurrence(n, completion.Current.State, role, freshGate, autonomous.OptionalRoleOutcomeSourceChanged, "The authorized optional role changed source, passed final verification, committed exact changes, and received a fresh independent audit.")
	if err != nil {
		return stopped(result, OutcomePersistenceStopped, err)
	}
	applied, err := recordAndApply(ctx, n, completion.Current.Expected(), occurrence)
	result.Application = applied
	if err != nil {
		return stopped(result, OutcomePersistenceStopped, err)
	}
	result.Outcome = OutcomeSourceChanged
	return result, nil
}

func replayResult(ctx context.Context, cfg Config) (Result, bool, error) {
	if cfg.Store == nil || strings.TrimSpace(cfg.TaskID) == "" || strings.TrimSpace(cfg.DispositionOperationID) == "" {
		return Result{}, false, nil
	}
	history, found, err := cfg.Store.LoadOptionalRoleOperation(ctx, cfg.TaskID, cfg.DispositionOperationID)
	if err != nil || !found {
		return Result{}, false, err
	}
	assessmentSHA, err := cfg.Assessment.Identity()
	if err != nil {
		return Result{}, true, err
	}
	if assessmentSHA != history.Record.Occurrence.AssessmentSHA256 {
		return Result{}, true, fmt.Errorf("%w: optional-role assessment differs from recorded disposition", autonomousstate.ErrOperationConflict)
	}
	current, stateFound, err := cfg.Store.Load(ctx, cfg.TaskID)
	if err != nil || !stateFound {
		return Result{}, true, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if !occurrencePresent(current.State, history.Record.Occurrence) {
		return Result{}, true, autonomousstate.ErrStaleWrite
	}
	outcome := OutcomeNotApplicable
	if history.Record.Occurrence.Outcome == autonomous.OptionalRoleOutcomeNoChange {
		outcome = OutcomeNoChange
	} else if history.Record.Occurrence.Outcome == autonomous.OptionalRoleOutcomeSourceChanged {
		outcome = OutcomeSourceChanged
	}
	return Result{TaskID: cfg.TaskID, Outcome: outcome, Application: ApplyResult{Disposition: autonomousstate.CommitReplayed, Current: current, History: history}}, true, nil
}

type normalized struct{ Config }

func normalize(ctx context.Context, cfg Config) (normalized, autonomousstate.Snapshot, autonomousstate.AuditSnapshot, error) {
	if strings.TrimSpace(cfg.RepositoryRoot) == "" || strings.TrimSpace(cfg.TaskID) == "" || cfg.Store == nil || cfg.Ledger == nil || cfg.Clock == nil || strings.TrimSpace(cfg.DispositionOperationID) == "" {
		return normalized{}, autonomousstate.Snapshot{}, autonomousstate.AuditSnapshot{}, errors.New("optional-role coordinator requires repository, task, store, ledger, clock, and disposition operation")
	}
	if err := cfg.Assessment.Validate(); err != nil || cfg.Assessment.TaskID != cfg.TaskID {
		return normalized{}, autonomousstate.Snapshot{}, autonomousstate.AuditSnapshot{}, errors.Join(err, errors.New("optional-role assessment has wrong task identity"))
	}
	if err := cfg.Expected.Validate(); err != nil || !cfg.Expected.Exists {
		return normalized{}, autonomousstate.Snapshot{}, autonomousstate.AuditSnapshot{}, errors.New("exact existing state expectation is required")
	}
	snapshot, found, err := cfg.Store.Load(ctx, cfg.TaskID)
	if err != nil || !found {
		return normalized{}, snapshot, autonomousstate.AuditSnapshot{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if snapshot.SHA256 != cfg.Expected.SHA256 || snapshot.ByteSize != cfg.Expected.ByteSize || snapshot.SHA256 != cfg.Assessment.StateSHA256 {
		return normalized{}, snapshot, autonomousstate.AuditSnapshot{}, autonomousstate.ErrStaleWrite
	}
	if cfg.CycleRunner == nil {
		cfg.CycleRunner = autonomouscycle.Run
	}
	if cfg.AuditApplier == nil {
		cfg.AuditApplier = autonomousauditapply.ApplyAuditResult
	}
	if cfg.AuditLoader == nil {
		cfg.AuditLoader = cfg.Store.LoadCurrentAudit
	}
	audit, found, err := cfg.AuditLoader(ctx, cfg.TaskID)
	if err != nil || !found {
		return normalized{}, snapshot, audit, errors.Join(err, errors.New("current committed audit evidence is required"))
	}
	if err := validateCurrentAuthority(cfg, snapshot, audit); err != nil {
		return normalized{}, snapshot, audit, err
	}
	if cfg.Assessment.Disposition == autonomous.OptionalRoleDispositionRun {
		if strings.TrimSpace(cfg.Admission.OperationID) == "" || strings.TrimSpace(cfg.Admission.AttemptID) == "" || strings.TrimSpace(cfg.CompletionOperationID) == "" || strings.TrimSpace(cfg.AuditOperationID) == "" {
			return normalized{}, snapshot, audit, errors.New("run assessment requires admission, completion, and audit operation identities")
		}
		if cfg.Assessment.Decision.Strategy == nil {
			return normalized{}, snapshot, audit, errors.New("run assessment requires exact structured supervisor strategy")
		}
	}
	return normalized{cfg}, snapshot, audit, nil
}

func validateCurrentAuthority(cfg Config, snapshot autonomousstate.Snapshot, audit autonomousstate.AuditSnapshot) error {
	verification := cfg.RoleCycle.Verification
	policyAudit := cfg.RoleCycle.Audit
	if verification == nil || policyAudit == nil || verification.Summary.RunID != cfg.Assessment.VerificationRunID || verification.Summary.OccurrenceID != cfg.Assessment.VerificationID || verification.SourceRevision != cfg.Assessment.SourceRevision || policyAudit.RunID != cfg.Assessment.AuditRunID || policyAudit.SourceRevision != cfg.Assessment.AuditSourceRevision {
		return errors.New("optional-role assessment does not match exact current verification/audit evidence")
	}
	if audit.PolicyEvidence.RunID != policyAudit.RunID || audit.PolicyEvidence.SourceRevision != cfg.Assessment.SourceRevision || audit.History.Record.Decision.RunID == audit.PolicyEvidence.RunID {
		return errors.New("optional-role audit authority is stale or non-independent")
	}
	_, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: cfg.TaskID, Decision: cfg.Assessment.Decision, Reference: cfg.Assessment.DecisionReference, State: snapshot.State, Source: autonomouspolicy.SourceEvidence{Revision: cfg.Assessment.SourceRevision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: cfg.RoleCycle.LatestMutation}, Verification: verification, Audit: policyAudit})
	return err
}

func gateFromAudit(a autonomousstate.AuditSnapshot) autonomous.OptionalRoleGate {
	return autonomous.OptionalRoleGate{SourceRevision: a.PolicyEvidence.SourceRevision, VerificationRunID: a.PolicyEvidence.VerificationRunID, VerificationOccurrenceID: a.PolicyEvidence.VerificationOccurrenceID, AuditSupervisorRunID: a.History.Record.Decision.RunID, AuditWorkerRunID: a.PolicyEvidence.RunID, AuditRevision: a.Revision}
}

func skippedOccurrence(n normalized, snapshot autonomousstate.Snapshot, gate autonomous.OptionalRoleGate) (autonomous.OptionalRoleOccurrence, error) {
	sha, err := n.Assessment.Identity()
	if err != nil {
		return autonomous.OptionalRoleOccurrence{}, err
	}
	return autonomous.OptionalRoleOccurrence{SchemaVersion: autonomous.OptionalRoleOccurrenceSchemaVersion, Sequence: int64(len(snapshot.State.OptionalRoles) + 1), TaskID: n.TaskID, Role: n.Assessment.Role, Outcome: autonomous.OptionalRoleOutcomeNotApplicable, Decision: n.Assessment.DecisionReference, AssessmentSHA256: sha, SourceBefore: n.Assessment.SourceRevision, SourceAfter: n.Assessment.SourceRevision, Gate: gate, Evidence: assessmentEvidence(n.Assessment), Rationale: n.Assessment.Rationale, CreatedAt: n.Clock().UTC()}, nil
}

func workerOccurrence(n normalized, state autonomous.ExecutionState, cycle autonomouscycle.Result, gate autonomous.OptionalRoleGate, outcome autonomous.OptionalRoleOutcome, rationale string) (autonomous.OptionalRoleOccurrence, error) {
	sha, err := n.Assessment.Identity()
	if err != nil {
		return autonomous.OptionalRoleOccurrence{}, err
	}
	worker := &autonomous.OptionalRoleWorkerEvidence{AttemptID: n.Admission.AttemptID, RunID: cycle.Worker.RunID, DossierSHA256: cycle.DossierManifest.DossierSHA256, DossierByteSize: cycle.DossierManifest.DossierByteSize, ProfilePath: cycle.Worker.Profile.Path, ProfileSHA256: cycle.Worker.Profile.SHA256, ProfileByteSize: cycle.Worker.Profile.ByteSize, Receipt: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindReceipt, Reference: cycle.Worker.Receipt.Path, Detail: "Harness-finalized optional-role receipt."}, Ledger: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindLedger, Reference: "ledger:" + cycle.Worker.RunID, Detail: "Completed optional-role worker ledger run."}}
	occurrence := autonomous.OptionalRoleOccurrence{SchemaVersion: autonomous.OptionalRoleOccurrenceSchemaVersion, Sequence: int64(len(state.OptionalRoles) + 1), TaskID: n.TaskID, Role: n.Assessment.Role, Outcome: outcome, Decision: n.Assessment.DecisionReference, AssessmentSHA256: sha, SourceBefore: n.Assessment.SourceRevision, SourceAfter: cycle.Source.FinalRevision, Gate: gate, Worker: worker, Evidence: append(assessmentEvidence(n.Assessment), worker.Receipt, worker.Ledger), Rationale: rationale, CreatedAt: n.Clock().UTC()}
	if outcome == autonomous.OptionalRoleOutcomeSourceChanged {
		occurrence.ChangedPaths = append([]string(nil), cycle.Source.ChangedFiles...)
		sort.Strings(occurrence.ChangedPaths)
		occurrence.CommitSHA = cycle.Worker.Commit.CommitSHA
		occurrence.BehaviorPreservation = append([]autonomous.EvidenceReference(nil), n.BehaviorPreservation...)
		if cycle.Worker.Verification.Policy != nil {
			occurrence.Evidence = append(occurrence.Evidence, cycle.Worker.Verification.Policy.Summary.Evidence...)
		}
	}
	return occurrence, occurrence.Validate()
}

func recordAndApply(ctx context.Context, n normalized, expected autonomousstate.ExpectedState, occurrence autonomous.OptionalRoleOccurrence) (ApplyResult, error) {
	if history, found, err := n.Store.LoadOptionalRoleOperation(ctx, n.TaskID, n.DispositionOperationID); err != nil {
		return ApplyResult{}, err
	} else if found {
		occurrence = history.Record.Occurrence
	} else {
		runID := n.Assessment.DecisionReference.RunID
		if occurrence.Worker != nil {
			runID = occurrence.Worker.RunID
		}
		event, err := n.Ledger.AppendEvent(ctx, runID, ledger.EventOptionalRoleDisposition, occurrence)
		if err != nil {
			return ApplyResult{}, err
		}
		occurrence.Evidence = append(occurrence.Evidence, autonomous.EvidenceReference{Kind: autonomous.EvidenceKindLedger, Reference: fmt.Sprintf("ledger:%s:event:%d", event.RunID, event.ID), Detail: "Durable optional-role disposition ledger event."})
	}
	return Apply(context.WithoutCancel(ctx), ApplyConfig{TaskID: n.TaskID, OperationID: n.DispositionOperationID, Expected: expected, Assessment: n.Assessment, Occurrence: occurrence, CreatedAt: n.Clock().UTC(), Store: n.Store})
}

func complete(n normalized, expected autonomousstate.ExpectedState, observation autonomousattempt.Observation, started time.Time) (autonomousattempt.Result, error) {
	finished := n.Clock().UTC()
	if finished.Before(started) {
		return autonomousattempt.Result{}, errors.New("optional-role trusted clock moved backwards")
	}
	return autonomousattempt.Complete(context.Background(), autonomousattempt.CompletionConfig{TaskID: n.TaskID, OperationID: n.CompletionOperationID, AttemptID: n.Admission.AttemptID, Expected: expected, RunID: observation.RunID, OccurrenceID: observation.OccurrenceID, SourceAfter: observation.SourceAfter, Outcome: observation.Outcome, Duration: finished.Sub(started), Tokens: observation.Tokens, Evidence: observation.Evidence, Signatures: observation.Signatures, StopReason: observation.StopReason, CreatedAt: finished, Store: n.Store})
}

func completeFailure(n normalized, expected autonomousstate.ExpectedState, role autonomouscycle.Result, audit *autonomouscycle.Result, started time.Time, cause error) (autonomousattempt.Result, error) {
	observation := autonomousattempt.ObserveCycle(role, cause)
	observation.Outcome = autonomous.AttemptOutcomeFailed
	if errors.Is(cause, context.Canceled) {
		observation.Outcome = autonomous.AttemptOutcomeCancelled
	}
	if audit != nil {
		other := autonomousattempt.ObserveCycle(*audit, cause)
		observation.Evidence = append(observation.Evidence, other.Evidence...)
		observation.Tokens = sumTokens(observation.Tokens, other.Tokens)
	}
	if len(observation.Evidence) == 0 {
		observation.Evidence = []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "optional-role:" + n.Admission.AttemptID, Detail: cause.Error()}}
	}
	return complete(n, expected, observation, started)
}

func combinedObservation(role, audit autonomouscycle.Result) autonomousattempt.Observation {
	left, right := autonomousattempt.ObserveCycle(role, nil), autonomousattempt.ObserveCycle(audit, nil)
	left.Outcome = autonomous.AttemptOutcomeSucceeded
	left.Evidence = append(left.Evidence, right.Evidence...)
	left.Tokens = sumTokens(left.Tokens, right.Tokens)
	return left
}

func sumTokens(left, right *int64) *int64 {
	if left == nil || right == nil || *left < 0 || *right < 0 || *left > math.MaxInt64-*right {
		return nil
	}
	total := *left + *right
	return &total
}

func validateRoleCycle(a autonomous.OptionalRoleAssessment, r autonomouscycle.Result) error {
	if r.Route == nil || r.Route.Action != a.Decision.Action || r.Route.WorkerProfile != a.Role || r.Outcome != autonomouscycle.OutcomeWorkerNoChanges && r.Outcome != autonomouscycle.OutcomeVerifiedChangesCommitted || r.Failure != nil || r.Supervisor.Decision == nil || r.Supervisor.DecisionReference == nil || !reflect.DeepEqual(*r.Supervisor.Decision, a.Decision) || *r.Supervisor.DecisionReference != a.DecisionReference || r.Source.AdmissionRevision != a.SourceRevision || r.Worker.RunID == "" || r.Worker.Action != a.Decision.Action || r.Worker.Profile.Name != string(a.Role) {
		return errors.New("optional-role cycle does not preserve exact assessment, decision, profile, source, and successful outcome")
	}
	if r.Outcome == autonomouscycle.OutcomeWorkerNoChanges && (r.Source.FinalRevision != a.SourceRevision || r.Worker.Verification.OccurrenceID != "" || r.Worker.Commit.CommitSHA != "") {
		return errors.New("optional-role no-op synthesized source, verification, or commit evidence")
	}
	if r.Outcome == autonomouscycle.OutcomeVerifiedChangesCommitted {
		if err := validateChangedPathAuthority(a, r.Source.ChangedFiles); err != nil {
			return err
		}
	}
	return nil
}

func validateChangedPathAuthority(a autonomous.OptionalRoleAssessment, changed []string) error {
	selected := make(map[string]struct{}, len(a.SelectedEvidenceIDs))
	for _, id := range a.SelectedEvidenceIDs {
		selected[id] = struct{}{}
	}
	var targets []string
	for _, evidence := range a.Evidence {
		if _, ok := selected[evidence.ID]; ok && evidence.TargetPath != "" {
			targets = append(targets, strings.TrimSuffix(evidence.TargetPath, "/"))
		}
	}
	if len(targets) == 0 || len(changed) == 0 {
		return errors.New("source-changing optional role requires selected target paths and observed changed paths")
	}
	for _, path := range changed {
		authorized := false
		for _, target := range targets {
			if path == target || strings.HasPrefix(path, target+"/") {
				authorized = true
				break
			}
		}
		if !authorized {
			return fmt.Errorf("optional-role changed path %q is outside selected target authority %q", path, targets)
		}
	}
	return nil
}

func roleVerification(r autonomouscycle.Result) (autonomouspolicy.VerificationEvidence, error) {
	if r.Worker.Verification.Policy == nil || r.Worker.Verification.Policy.Summary.Status != autonomous.VerificationStatusPassed || r.Worker.Verification.Policy.SourceRevision != r.Source.FinalRevision {
		return autonomouspolicy.VerificationEvidence{}, errors.New("source-changing optional role lacks current passed verification")
	}
	if r.Worker.Verification.Policy.Tiered != nil && (r.Worker.Verification.Policy.Tiered.Purpose != autonomousverification.PurposeFinal || !r.Worker.Verification.Policy.Tiered.FinalSatisfied) {
		return autonomouspolicy.VerificationEvidence{}, errors.New("source-changing optional role lacks a satisfied final verification gate")
	}
	if strings.TrimSpace(r.Worker.Commit.CommitSHA) == "" {
		return autonomouspolicy.VerificationEvidence{}, errors.New("source-changing optional role lacks an exact commit")
	}
	return *cloneVerification(r.Worker.Verification.Policy), nil
}

func validateAuditCycle(role, audit autonomouscycle.Result, verification autonomouspolicy.VerificationEvidence) error {
	if audit.Outcome != autonomouscycle.OutcomeReadOnlyCompleted || audit.Route == nil || audit.Route.Action != autonomous.ActionAudit || audit.Worker.Action != autonomous.ActionAudit || audit.Source.AdmissionRevision != role.Source.FinalRevision || audit.Source.FinalRevision != role.Source.FinalRevision || audit.Worker.Run.CompletedAt == nil || role.Worker.Run.CompletedAt == nil || !audit.Worker.Run.CompletedAt.After(*role.Worker.Run.CompletedAt) {
		return errors.New("optional-role fresh audit did not complete read-only after the source-changing role")
	}
	if role.Supervisor.RunID == role.Worker.RunID || audit.Supervisor.RunID == audit.Worker.RunID {
		return errors.New("optional-role and audit supervisor/worker identities are not independent")
	}
	for _, auditID := range []string{audit.Supervisor.RunID, audit.Worker.RunID} {
		for _, priorID := range []string{role.Supervisor.RunID, role.Worker.RunID, verification.Summary.RunID} {
			if auditID == priorID {
				return errors.New("optional-role auditor identities are not independent from role and verification runs")
			}
		}
	}
	return nil
}

func assessmentEvidence(a autonomous.OptionalRoleAssessment) []autonomous.EvidenceReference {
	byID := make(map[string]autonomous.EvidenceReference, len(a.Evidence))
	for _, evidence := range a.Evidence {
		byID[evidence.ID] = evidence.Reference
	}
	result := []autonomous.EvidenceReference{a.TaskSource, a.DecisionReference.Artifact}
	for _, id := range a.SelectedEvidenceIDs {
		result = append(result, byID[id])
	}
	return result
}

func cloneVerification(value *autonomouspolicy.VerificationEvidence) *autonomouspolicy.VerificationEvidence {
	if value == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	var result autonomouspolicy.VerificationEvidence
	_ = json.Unmarshal(raw, &result)
	return &result
}

func cloneAudit(value *autonomouspolicy.AuditEvidence) *autonomouspolicy.AuditEvidence {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func stopped(result Result, outcome Outcome, err error) (Result, error) {
	result.Outcome, result.Failure = outcome, err
	return result, err
}
