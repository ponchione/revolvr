package autonomousoptional

import (
	"context"
	"errors"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousattempt"
	"revolvr/internal/autonomousauditapply"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/supervisor"
)

func TestRunNotApplicableRolesAreIndependentAndStartNoWork(t *testing.T) {
	for _, role := range []autonomous.WorkerProfile{autonomous.WorkerProfileDocumentor, autonomous.WorkerProfileSimplifier} {
		t.Run(string(role), func(t *testing.T) {
			fixture := newOptionalFixture(t, role, autonomous.OptionalRoleDispositionNotApplicable)
			cycleCalls, auditApplyCalls := 0, 0
			fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
				cycleCalls++
				return autonomouscycle.Result{}, nil
			}
			fixture.cfg.AuditApplier = func(context.Context, autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error) {
				auditApplyCalls++
				return autonomousauditapply.Result{}, nil
			}
			result, err := Run(context.Background(), fixture.cfg)
			if err != nil || result.Outcome != OutcomeNotApplicable || cycleCalls != 0 || auditApplyCalls != 0 {
				t.Fatalf("result=%+v err=%v calls=%d/%d", result, err, cycleCalls, auditApplyCalls)
			}
			state := result.Application.Current.State
			if state.Attempts.TotalAttempts != 0 || len(state.Attempts.Events) != 0 || len(state.OptionalRoles) != 1 || state.OptionalRoles[0].Outcome != autonomous.OptionalRoleOutcomeNotApplicable || fixture.ledger.events != 1 {
				t.Fatalf("state=%+v ledger=%+v", state, fixture.ledger)
			}
		})
	}
}

func TestRunNoChangeAccountsOneAttemptWithoutVerificationAuditOrCommit(t *testing.T) {
	for _, role := range []autonomous.WorkerProfile{autonomous.WorkerProfileDocumentor, autonomous.WorkerProfileSimplifier} {
		t.Run(string(role), func(t *testing.T) {
			fixture := newOptionalFixture(t, role, autonomous.OptionalRoleDispositionRun)
			roleResult := fixture.roleResult(autonomouscycle.OutcomeWorkerNoChanges, fixture.source)
			calls := 0
			fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
				calls++
				return roleResult, nil
			}
			fixture.cfg.AuditApplier = func(context.Context, autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error) {
				t.Fatal("no-op started audit persistence")
				return autonomousauditapply.Result{}, nil
			}

			result, err := Run(context.Background(), fixture.cfg)
			if err != nil || result.Outcome != OutcomeNoChange || calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
			}
			state := result.Application.Current.State
			if state.Attempts.TotalAttempts != 1 || len(state.Attempts.Events) != 2 || state.Attempts.Events[1].Outcome != autonomous.AttemptOutcomeNoProgress || len(state.OptionalRoles) != 1 {
				t.Fatalf("state=%+v", state)
			}
			o := state.OptionalRoles[0]
			if o.Outcome != autonomous.OptionalRoleOutcomeNoChange || o.SourceBefore != o.SourceAfter || o.CommitSHA != "" || len(o.ChangedPaths) != 0 || o.Worker == nil || o.Worker.Receipt.Reference == "" || o.Worker.Ledger.Reference == "" {
				t.Fatalf("occurrence=%+v", o)
			}
			replay, err := Run(context.Background(), fixture.cfg)
			if err != nil || replay.Outcome != OutcomeNoChange || replay.Application.Disposition != autonomousstate.CommitReplayed || calls != 1 || fixture.ledger.events != 1 {
				t.Fatalf("replay=%+v err=%v calls=%d events=%d", replay, err, calls, fixture.ledger.events)
			}
		})
	}
}

func TestRunSourceChangingRolesRequireFinalVerificationCommitAndFreshAudit(t *testing.T) {
	for _, role := range []autonomous.WorkerProfile{autonomous.WorkerProfileDocumentor, autonomous.WorkerProfileSimplifier} {
		t.Run(string(role), func(t *testing.T) {
			fixture := newOptionalFixture(t, role, autonomous.OptionalRoleDispositionRun)
			changed := testHash("source-changed-" + string(role))
			roleResult := fixture.roleResult(autonomouscycle.OutcomeVerifiedChangesCommitted, changed)
			auditResult := fixture.auditResult(roleResult)
			cycleCalls, applyCalls := 0, 0
			fixture.cfg.CycleRunner = func(_ context.Context, cfg autonomouscycle.Config) (autonomouscycle.Result, error) {
				cycleCalls++
				if cycleCalls == 1 {
					return roleResult, nil
				}
				if cfg.LatestMutation == nil || cfg.LatestMutation.RunID != roleResult.Worker.RunID || cfg.Verification == nil || cfg.Verification.SourceRevision != changed {
					t.Fatalf("audit config=%+v", cfg)
				}
				return auditResult, nil
			}
			fixture.cfg.AuditApplier = func(_ context.Context, cfg autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error) {
				applyCalls++
				current, found, err := fixture.store.Load(context.Background(), fixture.taskID)
				if err != nil || !found || cfg.Expected != current.Expected() {
					t.Fatalf("audit apply current=%+v found=%t err=%v cfg=%+v", current, found, err, cfg)
				}
				return autonomousauditapply.Result{Current: current, AuditRevision: 2, Report: auditResultReport(fixture.taskID), State: current.State}, nil
			}
			if role == autonomous.WorkerProfileSimplifier {
				fixture.cfg.BehaviorPreservation = []autonomous.EvidenceReference{testEvidence("behavior-preservation-tests")}
			}

			result, err := Run(context.Background(), fixture.cfg)
			if err != nil || result.Outcome != OutcomeSourceChanged || cycleCalls != 2 || applyCalls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d/%d", result, err, cycleCalls, applyCalls)
			}
			o := result.Application.Current.State.OptionalRoles[0]
			if o.Outcome != autonomous.OptionalRoleOutcomeSourceChanged || o.SourceAfter != changed || o.CommitSHA != "commit-role" || o.Gate.AuditWorkerRunID != auditResult.Worker.RunID || o.Gate.AuditRevision != 2 || result.Completion.Current.State.Attempts.Events[1].Outcome != autonomous.AttemptOutcomeSucceeded {
				t.Fatalf("occurrence=%+v completion=%+v", o, result.Completion)
			}
			if role == autonomous.WorkerProfileSimplifier && len(o.BehaviorPreservation) == 0 {
				t.Fatal("simplification behavior evidence disappeared")
			}
		})
	}
}

func TestRunStoppedStagesNeverStartLaterWorkOrPersistSuccess(t *testing.T) {
	tests := []struct {
		name    string
		outcome autonomouscycle.Outcome
	}{
		{"failed verification", autonomouscycle.OutcomeVerificationFailed},
		{"commit refusal", autonomouscycle.OutcomeCommitFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOptionalFixture(t, autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionRun)
			roleResult := fixture.roleResult(test.outcome, testHash("failed-source"))
			roleResult.Failure = &autonomouscycle.Failure{Stage: "role", Reason: test.name}
			cycleCalls, applyCalls := 0, 0
			fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
				cycleCalls++
				return roleResult, errors.New(test.name)
			}
			fixture.cfg.AuditApplier = func(context.Context, autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error) {
				applyCalls++
				return autonomousauditapply.Result{}, nil
			}
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != OutcomeRoleStopped || cycleCalls != 1 || applyCalls != 0 {
				t.Fatalf("result=%+v err=%v calls=%d/%d", result, err, cycleCalls, applyCalls)
			}
			current, found, loadErr := fixture.store.Load(context.Background(), fixture.taskID)
			if loadErr != nil || !found || len(current.State.OptionalRoles) != 0 || len(current.State.Attempts.Events) != 2 || current.State.Attempts.Events[1].Outcome != autonomous.AttemptOutcomeFailed {
				t.Fatalf("state=%+v found=%t err=%v", current.State, found, loadErr)
			}
		})
	}
}

func TestContinueUsesPreAdmittedRoleWithoutStartingItAgain(t *testing.T) {
	f := newOptionalFixture(t, autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionRun)
	admitCfg := f.cfg.Admission
	admitCfg.TaskID, admitCfg.Expected, admitCfg.Action = f.taskID, f.snapshot.Expected(), f.assessment.Decision.Action
	admitCfg.Decision, admitCfg.Reference = f.assessment.Decision, f.assessment.DecisionReference
	admitCfg.SourceRevision, admitCfg.SourceSafety, admitCfg.Store = f.source, autonomouspolicy.SourceSafetySafe, f.store
	admission, err := autonomousattempt.Admit(context.Background(), admitCfg)
	if err != nil {
		t.Fatal(err)
	}
	f.cfg.Admission = admitCfg
	f.cfg.Expected = admission.Current.Expected()
	f.cfg.Assessment.StateSHA256 = admission.Current.SHA256
	role := f.roleResult(autonomouscycle.OutcomeWorkerNoChanges, f.source)
	called := false
	f.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
		called = true
		return autonomouscycle.Result{}, errors.New("unexpected role or audit cycle")
	}
	result, err := Continue(context.Background(), f.cfg, admission, role, testTime.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if called || result.Outcome != OutcomeNoChange || result.Completion.Disposition != autonomousattempt.DispositionCompleted || len(result.Application.Current.State.OptionalRoles) != 1 {
		t.Fatalf("called=%v result=%+v", called, result)
	}
}

func TestRunFailedOrNonIndependentAuditCannotValidateSourceChange(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*autonomouscycle.Result)
		runErr error
	}{
		{"failed audit", nil, errors.New("audit failed")},
		{"non-independent audit", func(result *autonomouscycle.Result) {
			result.Worker.RunID = "worker-role"
			result.Worker.Run.ID = "worker-role"
		}, nil},
		{"stale audit source", func(result *autonomouscycle.Result) { result.Source.AdmissionRevision = testHash("stale-audit") }, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOptionalFixture(t, autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionRun)
			role := fixture.roleResult(autonomouscycle.OutcomeVerifiedChangesCommitted, testHash("changed-for-audit"))
			audit := fixture.auditResult(role)
			if test.mutate != nil {
				test.mutate(&audit)
			}
			calls, applied := 0, 0
			fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
				calls++
				if calls == 1 {
					return role, nil
				}
				return audit, test.runErr
			}
			fixture.cfg.AuditApplier = func(context.Context, autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error) {
				applied++
				return autonomousauditapply.Result{}, nil
			}
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != OutcomeAuditStopped || applied != 0 {
				t.Fatalf("result=%+v err=%v applied=%d", result, err, applied)
			}
			current, _, _ := fixture.store.Load(context.Background(), fixture.taskID)
			if len(current.State.OptionalRoles) != 0 || current.State.Attempts.Events[1].Outcome != autonomous.AttemptOutcomeFailed {
				t.Fatalf("state=%+v", current.State)
			}
		})
	}
}

func TestRunRejectsSourceChangeOutsideSelectedRoleTarget(t *testing.T) {
	fixture := newOptionalFixture(t, autonomous.WorkerProfileSimplifier, autonomous.OptionalRoleDispositionRun)
	role := fixture.roleResult(autonomouscycle.OutcomeVerifiedChangesCommitted, testHash("unrelated-change"))
	role.Source.ChangedFiles = []string{"internal/unrelated_feature.go"}
	calls := 0
	fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
		calls++
		return role, nil
	}
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeRoleStopped || calls != 1 || len(result.Completion.Current.State.OptionalRoles) != 0 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
	}
}

func TestRunRejectsStaleAuditBeforeAdmission(t *testing.T) {
	fixture := newOptionalFixture(t, autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionRun)
	fixture.audit.PolicyEvidence.SourceRevision = testHash("stale")
	cycleCalls := 0
	fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
		cycleCalls++
		return autonomouscycle.Result{}, nil
	}
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || cycleCalls != 0 || result.Admission.Current.State.TaskID != "" {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, cycleCalls)
	}
	current, _, _ := fixture.store.Load(context.Background(), fixture.taskID)
	if current.State.Attempts.TotalAttempts != 0 || len(current.State.OptionalRoles) != 0 {
		t.Fatalf("stale audit changed state: %+v", current.State)
	}
}

func TestRunRetainsAW15BudgetAndCancellationAdmissionStops(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*optionalFixture) context.Context
		want   autonomous.BreakerReason
	}{
		{"task attempt budget", func(f *optionalFixture) context.Context {
			f.cfg.Admission.Limits.TaskAttempts.Limit = 0
			return context.Background()
		}, autonomous.BreakerTaskAttemptsExhausted},
		{"action attempt budget", func(f *optionalFixture) context.Context {
			for i := range f.cfg.Admission.Limits.ActionAttempts {
				if f.cfg.Admission.Limits.ActionAttempts[i].Action == f.assessment.Decision.Action {
					f.cfg.Admission.Limits.ActionAttempts[i].Budget.Limit = 0
				}
			}
			return context.Background()
		}, autonomous.BreakerActionAttemptsExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOptionalFixture(t, autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionRun)
			ctx := test.mutate(fixture)
			calls := 0
			fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
				calls++
				return autonomouscycle.Result{}, nil
			}
			result, err := Run(ctx, fixture.cfg)
			if err != nil || result.Outcome != OutcomeAdmissionStopped || result.Admission.Reason != test.want || calls != 0 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
			}
		})
	}

	fixture := newOptionalFixture(t, autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionRun)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.cfg.CycleRunner = func(context.Context, autonomouscycle.Config) (autonomouscycle.Result, error) {
		cancel()
		return fixture.roleResult(autonomouscycle.OutcomeWorkerFailed, fixture.source), context.Canceled
	}
	result, err := Run(ctx, fixture.cfg)
	if !errors.Is(err, context.Canceled) || result.Outcome != OutcomeRoleStopped || result.Completion.Reason != autonomous.BreakerCancellation || result.Completion.Current.State.Attempts.TotalAttempts != 1 {
		t.Fatalf("cancel result=%+v err=%v", result, err)
	}
}

type optionalFixture struct {
	t          *testing.T
	taskID     string
	source     string
	store      *autonomousstate.Store
	snapshot   autonomousstate.Snapshot
	audit      autonomousstate.AuditSnapshot
	ledger     *optionalLedger
	cfg        Config
	assessment autonomous.OptionalRoleAssessment
	now        time.Time
	clockTick  int
}

func newOptionalFixture(t *testing.T, role autonomous.WorkerProfile, disposition autonomous.OptionalRoleDisposition) *optionalFixture {
	t.Helper()
	repo := t.TempDir()
	store, snapshot := optionalStore(t, repo, "task-optional")
	source := testHash("optional-source")
	assessment := testAssessment("task-optional", role, disposition, snapshot.SHA256, source, "decision-role")
	if disposition == autonomous.OptionalRoleDispositionRun {
		assessment.Evidence[0].Kind = autonomous.OptionalRoleEvidenceTaskDocumentation
		assessment.Evidence[0].TargetPath = "README.md"
		if role == autonomous.WorkerProfileSimplifier {
			assessment.Evidence[0].Kind = autonomous.OptionalRoleEvidenceComplexityTarget
			assessment.Evidence[0].TargetPath = "internal/parser.go"
		}
	}
	verification := currentOptionalVerification("task-optional", source)
	auditEvidence := currentOptionalAudit("task-optional", source, verification)
	assessment.VerificationRunID = verification.Summary.RunID
	assessment.VerificationID = verification.Summary.OccurrenceID
	assessment.AuditRunID = auditEvidence.RunID
	assessment.AuditSourceRevision = auditEvidence.SourceRevision
	auditDecision := autonomous.DecisionReference{DecisionID: "decision-audit-current", RunID: "audit-supervisor-current", TaskID: "task-optional", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: testEvidence("audit-decision-artifact"), CreatedAt: testTime.Add(-time.Hour)}
	audit := autonomousstate.AuditSnapshot{Revision: 1, PolicyEvidence: auditEvidence, History: autonomousstate.AuditHistorySnapshot{Record: autonomousstate.AuditHistoryRecord{Decision: auditDecision}}}
	f := &optionalFixture{t: t, taskID: "task-optional", source: source, store: store, snapshot: snapshot, audit: audit, ledger: &optionalLedger{}, assessment: assessment, now: testTime}
	strategy := autonomousattempt.Strategy{Approach: assessment.Decision.Strategy.Approach, Techniques: assessment.Decision.Strategy.Techniques, Targets: assessment.Decision.Strategy.Targets}
	f.cfg = Config{RepositoryRoot: repo, TaskID: f.taskID, Expected: snapshot.Expected(), Assessment: assessment, Store: store, Ledger: f.ledger, Admission: autonomousattempt.AdmissionConfig{OperationID: "admit-role", AttemptID: "attempt-role", Strategy: strategy, Limits: optionalLimits(), CreatedAt: testTime, Store: store}, CompletionOperationID: "complete-role", DispositionOperationID: "disposition-role", AuditOperationID: "audit-role", RoleCycle: autonomouscycle.Config{Verification: &verification, Audit: &auditEvidence}, AuditCycle: autonomouscycle.Config{}, Clock: f.clock, AuditLoader: func(context.Context, string) (autonomousstate.AuditSnapshot, bool, error) { return f.audit, true, nil }}
	return f
}

func (f *optionalFixture) clock() time.Time {
	f.clockTick++
	return f.now.Add(time.Duration(f.clockTick) * time.Minute)
}

func (f *optionalFixture) roleResult(outcome autonomouscycle.Outcome, final string) autonomouscycle.Result {
	action := f.assessment.Decision.Action
	run := ledger.Run{ID: "worker-role", TaskID: f.taskID, Status: ledger.StatusCompleted, StartedAt: f.now.Add(2 * time.Minute)}
	completed := f.now.Add(3 * time.Minute)
	run.CompletedAt = &completed
	changedPath := "README.md"
	if f.assessment.Role == autonomous.WorkerProfileSimplifier {
		changedPath = "internal/parser.go"
	}
	result := autonomouscycle.Result{TaskID: f.taskID, Outcome: outcome, DossierManifest: autonomous.TaskDossierManifest{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: f.taskID, DossierSHA256: testHash("dossier"), DossierByteSize: 100}, Route: &autonomouspolicy.Route{Kind: autonomouspolicy.RouteKindWorker, TaskID: f.taskID, DecisionID: f.assessment.DecisionReference.DecisionID, Action: action, WorkerProfile: f.assessment.Role, SourceRevision: f.source}, Source: autonomouscycle.SourceEvidence{AdmissionRevision: f.source, WorkerRevision: final, FinalRevision: final, ChangedFiles: []string{changedPath}}}
	decision, reference := f.assessment.Decision, f.assessment.DecisionReference
	result.Supervisor.RunID, result.Supervisor.Decision, result.Supervisor.DecisionReference = reference.RunID, &decision, &reference
	result.Supervisor.Codex = codexexec.Result{UsageFound: true, Usage: receipt.Metrics{InputTokens: 2, OutputTokens: 1}}
	result.Worker = autonomouscycle.WorkerEvidence{Started: true, RunID: run.ID, Run: run, Action: action, Profile: autonomouscycle.ProfileEvidence{Name: string(f.assessment.Role), Path: ".agent/profiles/" + string(f.assessment.Role) + ".md", SHA256: testHash("profile"), ByteSize: 50}, Codex: codexexec.Result{UsageFound: true, Usage: receipt.Metrics{InputTokens: 3, OutputTokens: 1}}, Receipt: autonomouscycle.ReceiptEvidence{Path: ".revolvr/runs/worker-role/receipt.md"}}
	if outcome == autonomouscycle.OutcomeWorkerNoChanges {
		result.Source.WorkerRevision, result.Source.FinalRevision, result.Source.ChangedFiles = f.source, f.source, nil
	} else if outcome == autonomouscycle.OutcomeVerifiedChangesCommitted {
		verification := currentOptionalVerification(f.taskID, final)
		verification.Summary.RunID, verification.Summary.OccurrenceID = "worker-role", "role-verification"
		result.Worker.Verification = autonomouscycle.VerificationEvidence{OccurrenceID: verification.Summary.OccurrenceID, SourceRevision: final, Policy: &verification}
		result.Worker.Commit = commit.Result{Status: commit.StatusCommitted, CommitSHA: "commit-role"}
	}
	return result
}

func (f *optionalFixture) auditResult(role autonomouscycle.Result) autonomouscycle.Result {
	completed := f.now.Add(6 * time.Minute)
	auditDecision := autonomous.SupervisorDecision{TaskID: f.taskID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Rationale: "Fresh independent audit.", SuccessCriteria: []string{"Return structured audit evidence."}, Inputs: []autonomous.EvidenceReference{testEvidence("audit-input")}, Strategy: &autonomous.Strategy{Approach: "audit independently"}}
	reference := autonomous.DecisionReference{DecisionID: "decision-audit-new", RunID: "audit-supervisor-new", TaskID: f.taskID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: testEvidence("audit-decision-new"), CreatedAt: f.now.Add(4 * time.Minute)}
	return autonomouscycle.Result{TaskID: f.taskID, Outcome: autonomouscycle.OutcomeReadOnlyCompleted, Route: &autonomouspolicy.Route{Kind: autonomouspolicy.RouteKindWorker, TaskID: f.taskID, DecisionID: reference.DecisionID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, SourceRevision: role.Source.FinalRevision}, Supervisor: supervisorResult(reference.RunID, auditDecision, reference), Worker: autonomouscycle.WorkerEvidence{Started: true, RunID: "audit-worker-new", Run: ledger.Run{ID: "audit-worker-new", TaskID: f.taskID, Status: ledger.StatusCompleted, CompletedAt: &completed}, Action: autonomous.ActionAudit}, Source: autonomouscycle.SourceEvidence{AdmissionRevision: role.Source.FinalRevision, WorkerRevision: role.Source.FinalRevision, FinalRevision: role.Source.FinalRevision}}
}

func supervisorResult(runID string, decision autonomous.SupervisorDecision, reference autonomous.DecisionReference) supervisor.Result {
	return supervisor.Result{RunID: runID, Decision: &decision, DecisionReference: &reference}
}

func currentOptionalVerification(taskID, source string) autonomouspolicy.VerificationEvidence {
	return autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: taskID, Status: autonomous.VerificationStatusPassed, Summary: "passed", RunID: "verification-current", OccurrenceID: "verification-occurrence", Evidence: []autonomous.EvidenceReference{testEvidence("verification-current")}}, SourceRevision: source}
}

func currentOptionalAudit(taskID, source string, verification autonomouspolicy.VerificationEvidence) autonomouspolicy.AuditEvidence {
	return autonomouspolicy.AuditEvidence{Report: auditResultReport(taskID), RunID: "audit-current", AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: source, VerificationRunID: verification.Summary.RunID, VerificationOccurrenceID: verification.Summary.OccurrenceID}
}

func auditResultReport(taskID string) autonomous.AuditReport {
	return autonomous.AuditReport{TaskID: taskID, Disposition: autonomous.AuditDispositionClean, Rationale: "Independent audit is clean.", Inputs: []autonomous.EvidenceReference{testEvidence("verification-current")}}
}

func optionalLimits() autonomousattempt.Limits {
	return autonomousattempt.Limits{TaskAttempts: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4}, ActionAttempts: []autonomous.ActionBudget{{Action: autonomous.ActionDocument, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 2}}, {Action: autonomous.ActionSimplify, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 2}}}, Elapsed: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, Tokens: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}, RepeatedSignatureLimit: 3}
}

type optionalLedger struct{ events int64 }

func (l *optionalLedger) AppendEvent(_ context.Context, runID string, eventType ledger.EventType, _ any) (ledger.Event, error) {
	if eventType != ledger.EventOptionalRoleDisposition {
		return ledger.Event{}, errors.New("unexpected event")
	}
	l.events++
	return ledger.Event{ID: l.events, RunID: runID, Type: eventType, CreatedAt: testTime}, nil
}
