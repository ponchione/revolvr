package autonomouscorrection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousauditapply"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/supervisor"
	"revolvr/internal/verification"
)

func TestVerificationRepairOrdersFastFinalAndIndependentAudit(t *testing.T) {
	f := newFixture(t, AuthorityVerification, nil)
	result, err := Run(context.Background(), f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeReturnedToSupervisor || !reflect.DeepEqual(f.stages, []string{"correction", "final", "audit", "audit-apply"}) {
		t.Fatalf("result=%+v stages=%v", result, f.stages)
	}
	if result.FinalVerification.Purpose != autonomousverification.PurposeFinal || !result.FinalVerification.Gate.FinalSatisfied {
		t.Fatalf("final=%+v", result.FinalVerification)
	}
	if result.Audit.Worker.RunID == result.Correction.Worker.RunID || result.Audit.Worker.RunID == result.FinalVerification.RunID {
		t.Fatal("auditor identity is not independent")
	}
	if result.Audit.Worker.Run.StartedAt.Before(result.FinalVerification.EndedAt) {
		t.Fatal("re-audit is older than final verification")
	}
}

func TestAuditRepairResolvesOnlyExactClaimsAndRetainsPartialFinding(t *testing.T) {
	all := newFixture(t, AuthorityAudit, []string{"finding-one", "finding-two"})
	allResult, err := Run(context.Background(), all.cfg)
	if err != nil || !reflect.DeepEqual(all.resolved, []string{"finding-one", "finding-two"}) || resolutionStatus(allResult.State, "finding-two") != autonomous.FindingResolutionStatusResolved {
		t.Fatalf("multi-finding result=%+v resolved=%v err=%v", allResult, all.resolved, err)
	}

	f := newFixture(t, AuthorityAudit, []string{"finding-one", "finding-two"})
	f.output.Outcome = autonomous.CorrectionOutcomePartial
	f.output.ResolvedFindingIDs = []string{"finding-one"}
	f.output.RemainingFindingIDs = []string{"finding-two"}
	f.rebuildCorrection()
	result, err := Run(context.Background(), f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resolutions) != 1 || f.resolved[0] != "finding-one" || resolutionStatus(result.State, "finding-two") != autonomous.FindingResolutionStatusOpen {
		t.Fatalf("resolutions=%v state=%+v", f.resolved, result.State.FindingResolutions)
	}
	if result.Outcome != OutcomeReturnedToSupervisor {
		t.Fatalf("outcome=%q", result.Outcome)
	}

	uncited := newFixture(t, AuthorityAudit, []string{"finding-one"})
	uncited.output.ResolvedFindingIDs = []string{"finding-two"}
	uncited.output.RemainingFindingIDs = nil
	uncited.rebuildCorrection()
	result, err = Run(context.Background(), uncited.cfg)
	if err == nil || result.Failure.Stage != "correction_output" || len(uncited.resolved) != 0 {
		t.Fatalf("uncited result=%+v err=%v", result, err)
	}
}

func TestCorrectionStopsBeforeLaterStagesForFailuresRegressionAndCancellation(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*fixture, *context.Context)
		want    Outcome
	}{
		{"corrector failure", func(f *fixture, _ *context.Context) { f.correctionErr = errors.New("codex failed") }, OutcomeCorrectionStopped},
		{"corrector timeout", func(f *fixture, _ *context.Context) { f.correctionErr = context.DeadlineExceeded }, OutcomeCorrectionStopped},
		{"corrector cancellation", func(f *fixture, _ *context.Context) { f.correctionErr = context.Canceled }, OutcomeCorrectionStopped},
		{"no changes", func(f *fixture, _ *context.Context) { f.correction.Outcome = autonomouscycle.OutcomeWorkerNoChanges }, OutcomeCorrectionStopped},
		{"commit failure", func(f *fixture, _ *context.Context) { f.correction.Outcome = autonomouscycle.OutcomeCommitFailed }, OutcomeCorrectionStopped},
		{"malformed output", func(f *fixture, _ *context.Context) { f.correction.Worker.RawOutput = []byte("not-json") }, OutcomeCorrectionStopped},
		{"regression", func(f *fixture, _ *context.Context) { f.finalOutcome = autonomousverification.OutcomeFailed }, OutcomeFinalVerificationStopped},
		{"missing final", func(f *fixture, _ *context.Context) { f.finalOutcome = autonomousverification.OutcomeMissing }, OutcomeFinalVerificationStopped},
		{"timed out final", func(f *fixture, _ *context.Context) { f.finalOutcome = autonomousverification.OutcomeTimedOut }, OutcomeFinalVerificationStopped},
		{"flaky final", func(f *fixture, _ *context.Context) { f.finalOutcome = autonomousverification.OutcomeFlaky }, OutcomeFinalVerificationStopped},
		{"source mutating final", func(f *fixture, _ *context.Context) { f.snapshots[2] = f.base }, OutcomeFinalVerificationStopped},
		{"cancelled final", func(f *fixture, _ *context.Context) { f.finalErr = context.Canceled }, OutcomeFinalVerificationStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, AuthorityVerification, nil)
			ctx := context.Background()
			tt.prepare(f, &ctx)
			result, err := Run(ctx, f.cfg)
			if err == nil || result.Outcome != tt.want {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if tt.want == OutcomeCorrectionStopped && contains(f.stages, "final") {
				t.Fatalf("later stage ran: %v", f.stages)
			}
			if tt.want == OutcomeFinalVerificationStopped && contains(f.stages, "audit") {
				t.Fatalf("audit ran: %v", f.stages)
			}
		})
	}
}

func TestUnsafeDirtyStateBlocksBeforeSourceWriter(t *testing.T) {
	f := newFixture(t, AuthorityVerification, nil)
	f.cfg.CorrectionCycle.DirtyCapture = func(context.Context, gitstate.Config) (gitstate.Capture, error) {
		return gitstate.Capture{Kind: gitstate.CaptureKindDirty, Paths: []string{"user-work.go"}, DirtyFiles: []string{"user-work.go"}}, nil
	}
	result, err := Run(context.Background(), f.cfg)
	if err == nil || result.Outcome != OutcomeSafetyStopped || len(f.stages) != 0 {
		t.Fatalf("result=%+v err=%v stages=%v", result, err, f.stages)
	}
}

type fixture struct {
	t                *testing.T
	root             string
	now              time.Time
	cfg              Config
	state            autonomous.ExecutionState
	base, corrected  gitstate.SourceSnapshot
	snapshots        []gitstate.SourceSnapshot
	snapshotIndex    int
	plan             autonomousverification.Plan
	failure          autonomouspolicy.VerificationEvidence
	audit            autonomousstate.AuditSnapshot
	correction       autonomouscycle.Result
	output           autonomous.CorrectionOutput
	correctionErr    error
	finalOutcome     autonomousverification.Outcome
	finalErr         error
	stages, resolved []string
	ids              []string
	idIndex          int
}

func newFixture(t *testing.T, kind AuthorityKind, findingIDs []string) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), now: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC), finalOutcome: autonomousverification.OutcomePassed, ids: []string{"final-run", "final-occurrence", "attempt-one"}}
	f.state = autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-1", Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	for _, id := range findingIDs {
		f.state.FindingResolutions = append(f.state.FindingResolutions, autonomous.FindingResolution{FindingID: id, Status: autonomous.FindingResolutionStatusOpen})
	}
	task := []byte("---\nid: task-1\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/task-1/state.json\n---\n# Task\n")
	write(t, filepath.Join(f.root, ".agent/tasks/task-1.md"), task)
	raw, _ := autonomousstate.MarshalState(f.state)
	write(t, filepath.Join(f.root, ".revolvr/autonomous/tasks/task-1/state.json"), raw)
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: f.root})
	if err != nil {
		t.Fatal(err)
	}
	snap, found, err := store.Load(context.Background(), "task-1")
	if err != nil || !found {
		t.Fatal(err)
	}
	f.base = testSnapshot("head-one", nil)
	f.corrected = testSnapshot("head-two", []gitstate.SourceEntry{{Path: "fixed.go", FileType: "regular", Mode: 0o644, ByteSize: 4, SHA256: hash([]byte("fix\n"))}})
	baseRev, _ := gitstate.PolicySourceRevision(f.base)
	correctedRev, _ := gitstate.PolicySourceRevision(f.corrected)
	f.snapshots = []gitstate.SourceSnapshot{f.base, f.corrected, f.corrected}
	f.plan = autonomousverification.Plan{SchemaVersion: autonomousverification.PlanSchemaVersion, Tiers: []autonomousverification.Tier{{ID: "structural", Kind: autonomousverification.TierStructural, RequiredForFinal: true, RunForFast: true, RunForFinal: true, Commands: []verification.Command{{Name: "fake"}}, RerunPolicy: autonomousverification.RerunNever}}}
	f.failure = autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: "task-1", Status: autonomous.VerificationStatusFailed, Summary: "failed", RunID: "failed-run", OccurrenceID: "failed-occurrence", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: ".revolvr/runs/failed-run/verification.json", Detail: "Exact failed verification artifact."}}}, SourceRevision: baseRev}
	target := autonomous.VerificationFailureTarget{TaskID: "task-1", RunID: "failed-run", OccurrenceID: "failed-occurrence", SourceRevision: baseRev, Status: autonomous.VerificationStatusFailed, Evidence: append([]autonomous.EvidenceReference(nil), f.failure.Summary.Evidence...)}
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: autonomous.ActionCorrect, WorkerProfile: autonomous.WorkerProfileCorrector, Rationale: "correct exact authority", SuccessCriteria: []string{"repair"}, Inputs: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task", Detail: "task"}}}
	if kind == AuthorityAudit {
		decision.FindingIDs = append([]string(nil), findingIDs...)
	} else {
		decision.VerificationFailure = &target
	}
	reference := autonomous.DecisionReference{DecisionID: "decision-correct", RunID: "correct-supervisor", TaskID: "task-1", Action: autonomous.ActionCorrect, WorkerProfile: autonomous.WorkerProfileCorrector, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: "decision.json", Detail: "decision"}, CreatedAt: f.now}
	fast := testVerification(t, f.plan, autonomousverification.PurposeFast, "corrector-run", "fast-occurrence", correctedRev, autonomousverification.OutcomePassed, f.now.Add(time.Minute))
	correctionCompleted := f.now.Add(time.Minute)
	f.output = autonomous.CorrectionOutput{SchemaVersion: autonomous.CorrectionOutputSchemaVersion, TaskID: "task-1", WorkerRunID: "corrector-run", DecisionID: "decision-correct", Outcome: autonomous.CorrectionOutcomeCorrected, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: "fixed.go", Detail: "New correction evidence."}}}
	if kind == AuthorityAudit {
		f.output.FindingIDs = append([]string(nil), findingIDs...)
		f.output.ResolvedFindingIDs = append([]string(nil), findingIDs...)
	} else {
		copy := target
		f.output.VerificationFailure = &copy
		f.output.VerificationFailureAddressed = true
	}
	f.correction = autonomouscycle.Result{TaskID: "task-1", Outcome: autonomouscycle.OutcomeVerifiedChangesCommitted, Supervisor: supervisor.Result{RunID: "correct-supervisor", Decision: &decision, DecisionReference: &reference}, Route: &autonomouspolicy.Route{Kind: autonomouspolicy.RouteKindWorker, TaskID: "task-1", DecisionID: "decision-correct", Action: autonomous.ActionCorrect, WorkerProfile: autonomous.WorkerProfileCorrector, SourceRevision: baseRev}, Worker: autonomouscycle.WorkerEvidence{Started: true, RunID: "corrector-run", Run: ledger.Run{ID: "corrector-run", TaskID: "task-1", Status: ledger.StatusCompleted, StartedAt: f.now, CompletedAt: &correctionCompleted}, Action: autonomous.ActionCorrect, Verification: autonomouscycle.VerificationEvidence{Tiered: &fast}}, Source: autonomouscycle.SourceEvidence{AdmissionRevision: baseRev, FinalRevision: correctedRev}}
	f.rebuildCorrection()
	if kind == AuthorityAudit {
		report := autonomous.AuditReport{TaskID: "task-1", Disposition: autonomous.AuditDispositionChangesRequired, Rationale: "findings", Inputs: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "prior", Detail: "prior"}}}
		for _, id := range findingIDs {
			report.Findings = append(report.Findings, autonomous.AuditFinding{ID: id, Significance: autonomous.FindingSignificanceBlocking, Summary: "defect " + id, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: id + ".go", Detail: "defect"}}, RequiredCorrection: "fix " + id})
		}
		prior := policyPassed("prior-final", "prior-occurrence", baseRev)
		f.audit = autonomousstate.AuditSnapshot{State: snap, Revision: 1, Report: report, PolicyEvidence: autonomouspolicy.AuditEvidence{Report: report, RunID: "prior-auditor", AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: baseRev, VerificationRunID: prior.Summary.RunID, VerificationOccurrenceID: prior.Summary.OccurrenceID}, History: autonomousstate.AuditHistorySnapshot{Record: autonomousstate.AuditHistoryRecord{Verification: prior}}}
	}
	f.cfg = Config{RepositoryRoot: f.root, TaskID: "task-1", Expected: snap.Expected(), Authority: Authority{Kind: kind, FindingIDs: append([]string(nil), findingIDs...)}, Store: store, CorrectionCycle: autonomouscycle.Config{RepositoryRoot: f.root, Ledger: newTestLedger(), GitExecutable: "fake", GitTimeout: time.Second, GitStdoutCap: 1024, GitStderrCap: 1024, VerificationPlan: &f.plan, SourceSnapshotter: f.capture, DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
		return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
	}}, AuditCycle: autonomouscycle.Config{}, FinalPlan: f.plan, FinalTimeout: time.Second, FinalStdoutCap: 1024, FinalStderrCap: 1024, IDGenerator: f.nextID, Clock: func() time.Time { return f.now.Add(2 * time.Minute) }, CycleRunner: f.runCycle, VerificationRunner: f.runFinal, ResolutionApplier: f.applyResolution, AuditApplier: f.applyAudit, AuditLoader: func(context.Context, string) (autonomousstate.AuditSnapshot, bool, error) { return f.audit, true, nil }}
	if kind == AuthorityVerification {
		f.cfg.Authority.Verification = &f.failure
	}
	return f
}

func (f *fixture) rebuildCorrection() {
	raw, err := autonomous.MarshalCorrectionOutput(f.output)
	if err != nil {
		raw = []byte("invalid")
	}
	f.correction.Worker.RawOutput = raw
	f.correction.Worker.Artifacts.Output = autonomouscycle.Artifact{Path: ".revolvr/runs/corrector-run/corrector-output.raw.json", SHA256: hash(raw), ByteSize: len(raw)}
}
func (f *fixture) nextID() string { v := f.ids[f.idIndex]; f.idIndex++; return v }
func (f *fixture) capture(context.Context, gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error) {
	v := f.snapshots[f.snapshotIndex]
	f.snapshotIndex++
	return v, nil
}
func (f *fixture) runCycle(_ context.Context, cfg autonomouscycle.Config) (autonomouscycle.Result, error) {
	if len(f.stages) == 0 {
		f.stages = append(f.stages, "correction")
		return f.correction, f.correctionErr
	}
	f.stages = append(f.stages, "audit")
	v := cfg.Verification
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Rationale: "audit", SuccessCriteria: []string{"report"}, Inputs: v.Summary.Evidence}
	ref := autonomous.DecisionReference{DecisionID: "decision-audit", RunID: "audit-supervisor", TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: "audit-decision", Detail: "audit"}, CreatedAt: f.now.Add(4 * time.Minute)}
	rev := v.SourceRevision
	return autonomouscycle.Result{TaskID: "task-1", Outcome: autonomouscycle.OutcomeReadOnlyCompleted, Supervisor: supervisor.Result{RunID: "audit-supervisor", Decision: &decision, DecisionReference: &ref}, Route: &autonomouspolicy.Route{Kind: autonomouspolicy.RouteKindWorker, TaskID: "task-1", DecisionID: "decision-audit", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, SourceRevision: rev}, Worker: autonomouscycle.WorkerEvidence{Started: true, RunID: "auditor-run", Run: ledger.Run{ID: "auditor-run", TaskID: "task-1", Status: ledger.StatusCompleted, StartedAt: f.now.Add(5 * time.Minute)}, Action: autonomous.ActionAudit}, Source: autonomouscycle.SourceEvidence{AdmissionRevision: rev, WorkerRevision: rev}}, nil
}
func (f *fixture) runFinal(_ context.Context, cfg autonomousverification.Config) (autonomousverification.Result, error) {
	f.stages = append(f.stages, "final")
	if f.finalErr != nil {
		return autonomousverification.Result{}, f.finalErr
	}
	return testVerification(f.t, cfg.Plan, autonomousverification.PurposeFinal, cfg.RunID, cfg.OccurrenceID, cfg.SourceRevision, f.finalOutcome, f.now.Add(2*time.Minute)), nil
}
func (f *fixture) applyResolution(_ context.Context, cfg autonomousauditapply.ResolutionConfig) (autonomousauditapply.Result, error) {
	f.resolved = append(f.resolved, cfg.Request.FindingID)
	next := f.state
	for i := range next.FindingResolutions {
		if next.FindingResolutions[i].FindingID == cfg.Request.FindingID {
			next.FindingResolutions[i].Status = autonomous.FindingResolutionStatusResolved
			next.FindingResolutions[i].Evidence = cfg.Request.Evidence
			next.FindingResolutions[i].Resolution = cfg.Request.DecisionReference
		}
	}
	f.state = next
	raw, _ := autonomousstate.MarshalState(next)
	snap := autonomousstate.Snapshot{State: next, SHA256: hash(raw), ByteSize: len(raw), SourcePath: "state.json"}
	return autonomousauditapply.Result{Current: snap, State: next}, nil
}
func (f *fixture) applyAudit(_ context.Context, _ autonomousauditapply.ApplyConfig) (autonomousauditapply.Result, error) {
	f.stages = append(f.stages, "audit-apply")
	return autonomousauditapply.Result{State: f.state}, nil
}

func testVerification(t *testing.T, plan autonomousverification.Plan, purpose autonomousverification.Purpose, run, occ, rev string, outcome autonomousverification.Outcome, at time.Time) autonomousverification.Result {
	t.Helper()
	id, _ := autonomousverification.Identity(plan)
	gate := autonomousverification.GateEvidence{SchemaVersion: autonomousverification.GateSchemaVersion, Plan: id, Purpose: purpose, RequiredFinalTiers: []string{"structural"}, SelectedTiers: []string{"structural"}, ExecutedTiers: []string{"structural"}, RequiredOutcomes: []autonomousverification.TierGate{{TierID: "structural", Outcome: outcome}}, OverallOutcome: outcome, FinalSatisfied: purpose == autonomousverification.PurposeFinal && outcome == autonomousverification.OutcomePassed}
	r := autonomousverification.Result{SchemaVersion: autonomousverification.ResultSchemaVersion, TaskID: "task-1", RunID: run, OccurrenceID: occ, SourceRevision: rev, Plan: id, Purpose: purpose, Outcome: outcome, Gate: gate, Tiers: []autonomousverification.TierResult{{ID: "structural", Kind: autonomousverification.TierStructural, RequiredForFinal: true, Outcome: outcome, StartedAt: at, EndedAt: at}}, StartedAt: at, EndedAt: at}
	return r
}
func policyPassed(run, occ, rev string) autonomouspolicy.VerificationEvidence {
	return autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: "task-1", Status: autonomous.VerificationStatusPassed, Summary: "passed", RunID: run, OccurrenceID: occ, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: run + ":" + occ, Detail: "passed"}}}, SourceRevision: rev}
}
func testSnapshot(head string, entries []gitstate.SourceEntry) gitstate.SourceSnapshot {
	if entries == nil {
		entries = []gitstate.SourceEntry{}
	}
	index := sha256.Sum256(nil)
	workRaw, _ := json.Marshal(entries)
	work := sha256.Sum256(workRaw)
	s := gitstate.SourceSnapshot{SchemaVersion: gitstate.SourceSnapshotSchemaVersion, Head: head, IndexSHA256: fmt.Sprintf("%x", index), WorktreeSHA256: fmt.Sprintf("%x", work), Entries: entries}
	raw, _ := json.Marshal(struct {
		SchemaVersion  string                 `json:"schema_version"`
		Head           string                 `json:"head"`
		IndexSHA256    string                 `json:"index_sha256"`
		WorktreeSHA256 string                 `json:"worktree_sha256"`
		Entries        []gitstate.SourceEntry `json:"entries"`
	}{s.SchemaVersion, s.Head, s.IndexSHA256, s.WorktreeSHA256, s.Entries})
	s.SnapshotSHA256 = hash(raw)
	return s
}
func hash(raw []byte) string { s := sha256.Sum256(raw); return fmt.Sprintf("%x", s) }
func write(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
}
func contains(v []string, w string) bool {
	for _, s := range v {
		if s == w {
			return true
		}
	}
	return false
}

type testLedger struct {
	runs map[string]ledger.Run
	next int64
}

func newTestLedger() *testLedger { return &testLedger{runs: map[string]ledger.Run{}} }
func (l *testLedger) CreateRun(_ context.Context, s ledger.RunSpec) (ledger.Run, error) {
	r := ledger.Run{ID: s.ID, TaskID: s.TaskID, Task: s.Task, Status: ledger.StatusRunning, StartedAt: s.StartedAt}
	l.runs[r.ID] = r
	return r, nil
}
func (l *testLedger) AppendEvent(_ context.Context, runID string, eventType ledger.EventType, payload any) (ledger.Event, error) {
	l.next++
	raw, _ := json.Marshal(payload)
	return ledger.Event{ID: l.next, RunID: runID, Type: eventType, Payload: raw}, nil
}
func (l *testLedger) CompleteRun(_ context.Context, id string, c ledger.RunCompletion) (ledger.Run, bool, error) {
	r, ok := l.runs[id]
	if !ok {
		return ledger.Run{}, false, nil
	}
	r.Status, r.CompletedAt, r.Summary, r.VerificationStatus = c.Status, &c.CompletedAt, c.Summary, c.VerificationStatus
	l.runs[id] = r
	return r, true, nil
}
func (l *testLedger) RecordCommitSHA(context.Context, string, string) error { return nil }
