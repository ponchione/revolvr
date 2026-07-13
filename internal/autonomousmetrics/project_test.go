package autonomousmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/ledger"
	"revolvr/internal/ledgerexport"
	"revolvr/internal/runner"
	"revolvr/internal/verification"
)

var fixed = time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)

func TestProjectAllTaskOutcomesCanonicalAndCallerOwned(t *testing.T) {
	reasons := []autonomoustaskrun.StopReason{autonomoustaskrun.StopCompleted, autonomoustaskrun.StopBlocked, autonomoustaskrun.StopNeedsInput, autonomoustaskrun.StopSafety, autonomoustaskrun.StopBudgetExhausted, autonomoustaskrun.StopNoProgress, autonomoustaskrun.StopTaskCancelled, autonomoustaskrun.StopOperationCancelled, autonomoustaskrun.StopMaxCycles, autonomoustaskrun.StopUnsafeAmbiguous}
	var runs []ledger.RunWithEvents
	for i, reason := range reasons {
		runs = append(runs, taskRun(t, i+1, "task-"+string(rune('a'+i)), "op-"+string(rune('a'+i)), reason, nil, ""))
	}
	snapshot := ledger.Snapshot{Runs: runs, MaxEventID: int64(len(runs))}
	before, _ := json.Marshal(snapshot)
	p, err := Project(snapshot, LogicalSource(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if p.TaskOutcomes.Total != 10 || p.TaskOutcomes.SuccessNumerator != 1 || p.TaskOutcomes.SuccessDenominator != 10 {
		t.Fatalf("outcomes=%+v", p.TaskOutcomes)
	}
	want := []string{"blocked", "budget", "cancelled", "completed", "max_cycle", "needs_input", "no_progress", "safety", "unsafe"}
	for i, name := range want {
		if p.TaskOutcomes.Counts[i].Name != name {
			t.Fatalf("counts=%+v", p.TaskOutcomes.Counts)
		}
	}
	after, _ := json.Marshal(snapshot)
	if string(before) != string(after) {
		t.Fatal("projection mutated caller snapshot")
	}
	raw, err := Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(raw)
	if err != nil || decoded.SchemaVersion != SchemaVersion {
		t.Fatalf("decode=%+v err=%v", decoded, err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	bad, _ := json.MarshalIndent(object, "", "  ")
	bad = append(bad, '\n')
	if _, err := Decode(bad); err == nil {
		t.Fatal("unknown field accepted")
	}
	conflict := snapshot
	conflicting := taskEvent(t, "task-a", "op-a", 99, "block", autonomoustaskrun.StopBlocked)
	conflict.Runs = append(append([]ledger.RunWithEvents(nil), snapshot.Runs...), history(t, 99, "run-conflict", ledger.EventTaskRunStopped, conflicting))
	conflict.MaxEventID = 99
	if _, err := Project(conflict, LogicalSource(conflict)); err == nil {
		t.Fatal("conflicting logical operation accepted")
	}
	unknown := taskEvent(t, "task-z", "op-z", 1, "complete", autonomoustaskrun.StopCompleted)
	unknown.SchemaVersion = "autonomous-task-run-event-v99"
	unknownSnapshot := ledger.Snapshot{Runs: []ledger.RunWithEvents{history(t, 1, "run-unknown", ledger.EventTaskRunStopped, unknown)}, MaxEventID: 1}
	if _, err := Project(unknownSnapshot, LogicalSource(unknownSnapshot)); err == nil {
		t.Fatal("unknown relevant evidence schema accepted")
	}
}

func TestProjectAttemptsAuditsVerificationArchiveAndQueue(t *testing.T) {
	tokens := int64(17)
	attempts := autonomous.AttemptState{Events: []autonomous.AttemptEvent{{Sequence: 1, Kind: autonomous.AttemptEventAdmitted, AttemptID: "attempt-one", Action: autonomous.ActionCorrect, Decision: decision(), StrategySHA256: hash("strategy"), SourceBefore: hash("source"), CreatedAt: fixed}, {Sequence: 2, Kind: autonomous.AttemptEventCompleted, AttemptID: "attempt-one", Action: autonomous.ActionCorrect, Decision: decision(), StrategySHA256: hash("strategy"), RunID: "worker-one", OccurrenceID: "occurrence-one", SourceBefore: hash("source"), SourceAfter: hash("source-two"), SourceAfterKnown: true, Outcome: autonomous.AttemptOutcomeSucceeded, Duration: 3 * time.Second, Tokens: &tokens, CreatedAt: fixed.Add(3 * time.Second)}}}
	cycle := taskEvent(t, "task-one", "op-one", 1, "correct", "")
	cycle.Statistics.Corrections = 1
	cycle.Statistics.AttemptsAdmitted = 1
	cycle.Statistics.AttemptsCompleted = 1
	cycle.Metrics = &autonomoustaskrun.MetricsEvidence{Attempts: attempts, FindingResolutions: []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusResolved, Evidence: []autonomous.EvidenceReference{evidence("resolution")}}}}
	cycle.Audit = auditRaw(t, false)
	cycleRun := history(t, 1, "run-task", ledger.EventTaskRunCycleCompleted, cycle)
	terminal := taskEvent(t, "task-one", "op-one", 2, "complete", autonomoustaskrun.StopCompleted)
	terminal.Statistics = cycle.Statistics
	terminal.Metrics = cycle.Metrics
	terminalRun := history(t, 2, "run-task-terminal", ledger.EventTaskRunStopped, terminal)
	verification := verificationEvent()
	verificationRun := history(t, 3, "verification-run", ledger.EventVerificationCompleted, verification)
	archiveAt := fixed.Add(2 * time.Minute)
	archive := autonomousarchive.LedgerEvent{SchemaVersion: autonomousarchive.LedgerEventSchemaVersion, ArchiveID: strings.Repeat("a", 64), OperationID: "archive-one", TaskID: "task-one", Disposition: autonomousarchive.DispositionCompleted, Stage: autonomousarchive.StageLedgerComplete, Manifest: autonomousarchive.Artifact{Path: ".agent/archive/2026/07/task-one/archive.json", SHA256: hash("manifest"), ByteSize: 1}, CommitSHA: strings.Repeat("b", 40), TerminalAt: fixed.Add(time.Minute), ArchivedAt: archiveAt}
	archiveRun := history(t, 4, "archive-run", ledger.EventArchiveCompleted, archive)
	finalization := autonomousfinalization.LedgerEvent{SchemaVersion: autonomousfinalization.LedgerEventSchemaVersion, TaskID: "task-one", OperationID: "finalize-one", Stage: autonomous.FinalizationStageLedgerCompleted, SourceRevision: hash("source"), PolicySHA256: hash("policy"), AdmittedAt: fixed.Add(50 * time.Second), TerminalAt: fixed.Add(time.Minute)}
	finalizationRun := history(t, 6, "finalization-run", ledger.EventFinalizationCompleted, finalization)
	queueDone := fixed.Add(10 * time.Second)
	queue := autonomousqueue.LedgerEvent{SchemaVersion: autonomousqueue.LedgerEventSchemaVersion, OperationID: "queue-one", Mode: autonomousqueue.ModeUntilExhausted, Sequence: 2, Sweep: 1, Stage: "terminal", StopReason: autonomousqueue.StopDrained, StartedAt: fixed, UpdatedAt: queueDone, CompletedAt: &queueDone, MaximumWorkers: 2, Statistics: autonomousqueue.Statistics{Selections: 1, TasksRun: 1, Batches: 1, PeakActiveWorkers: 1}}
	queueRun := history(t, 5, "queue-run", ledger.EventQueueStopped, queue)
	snapshot := ledger.Snapshot{Runs: []ledger.RunWithEvents{cycleRun, terminalRun, verificationRun, archiveRun, queueRun, finalizationRun}, MaxEventID: 6}
	p, err := Project(snapshot, LogicalSource(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if p.Attempts.Admitted != 1 || p.Attempts.Completed != 1 || p.Attempts.CorrectionCycles != 1 || p.Usage.RecordedTokens != 17 || p.Usage.AttemptDurationNanoseconds != int64(3*time.Second) {
		t.Fatalf("attempts=%+v usage=%+v", p.Attempts, p.Usage)
	}
	if p.Audits.Performed != 1 || p.Audits.ChangesRequired != 1 || p.Audits.BlockingFindings != 1 || p.Audits.Findings[0].Disposition != "resolved" {
		t.Fatalf("audits=%+v", p.Audits)
	}
	if p.Verification.Occurrences != 1 || p.Verification.OrdinaryPasses != 0 {
		t.Fatalf("verification=%+v", p.Verification)
	}
	if p.Archives.LatencyNanoseconds != int64(time.Minute) || len(p.Archives.TerminalCompletions) != 1 || p.Queues.TasksRun != 1 || p.Queues.Drained != 1 {
		t.Fatalf("archive=%+v queue=%+v", p.Archives, p.Queues)
	}
	if p.Queues.MaximumConfiguredWorkers != 2 || p.Queues.PeakActiveWorkers != 1 || p.Queues.ParallelSweeps != 0 || p.Queues.Facts[0].Batches != 1 {
		t.Fatalf("queue concurrency=%+v", p.Queues)
	}
}

func TestLiveAndVerifiedExportProjectionBytesEqualAndCorruptionRefused(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr"), 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ledger.OpenWithClock(context.Background(), filepath.Join(repo, ".revolvr", "ledger.sqlite"), func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateRun(context.Background(), ledger.RunSpec{ID: "task-run", TaskID: "task-one", Task: "run exact autonomous task until terminal", StartedAt: fixed}); err != nil {
		t.Fatal(err)
	}
	event := taskEvent(t, "task-one", "op-one", 1, "complete", autonomoustaskrun.StopCompleted)
	if _, err = store.AppendEvent(context.Background(), "task-run", ledger.EventTaskRunStopped, event); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.CompleteRun(context.Background(), "task-run", ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: "done", CompletedAt: fixed.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	live, err := store.ReadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	exported, err := ledgerexport.Export(context.Background(), ledgerexport.ExportInput{RepositoryRoot: repo, OperationID: "export-one", ExportedAt: fixed.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ledgerexport.ReplaySnapshot(context.Background(), repo, exported.Manifest.ExportID, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Project(live, LogicalSource(live))
	b, _ := Project(replayed, LogicalSource(replayed))
	ar, _ := Marshal(a)
	br, _ := Marshal(b)
	if string(ar) != string(br) {
		t.Fatalf("live/export differ\nlive=%s\nexport=%s", ar, br)
	}
	records := filepath.Join(repo, filepath.FromSlash(exported.Manifest.Records.Path))
	raw, _ := os.ReadFile(records)
	raw[0] = 'x'
	if err := os.WriteFile(records, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerexport.ReplaySnapshot(context.Background(), repo, exported.Manifest.ExportID, nil); err == nil {
		t.Fatal("corrupted export accepted")
	}
}

func TestProjectRetainsFlakyFailurePassAndRerun(t *testing.T) {
	repo := t.TempDir()
	store, err := ledger.OpenWithClock(context.Background(), filepath.Join(repo, "ledger.sqlite"), func() time.Time { return fixed })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateRun(context.Background(), ledger.RunSpec{ID: "verification-run", TaskID: "task-one", Task: "tiered verification", StartedAt: fixed}); err != nil {
		t.Fatal(err)
	}
	clockValue := fixed
	clock := func() time.Time { clockValue = clockValue.Add(time.Second); return clockValue }
	calls, ids := 0, 0
	plan := autonomousverification.Plan{SchemaVersion: autonomousverification.PlanSchemaVersion, Tiers: []autonomousverification.Tier{{
		ID: "full-suite", Kind: autonomousverification.TierFullSuite, RequiredForFinal: true, RunForFinal: true,
		RerunPolicy: autonomousverification.RerunOnceToClassifyFlaky,
		Commands:    []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
	}}}
	_, _ = autonomousverification.Execute(context.Background(), autonomousverification.Config{RepositoryRoot: repo, TaskID: "task-one", RunID: "verification-run", OccurrenceID: "verification-one", SourceRevision: hash("source"), Plan: plan, Purpose: autonomousverification.PurposeFinal, Timeout: time.Minute, StdoutCap: 1024, StderrCap: 1024, Clock: clock, AttemptID: func() string { ids++; return fmt.Sprintf("attempt-%d", ids) }, CommandRunner: func(context.Context, runner.Command) runner.Result {
		calls++
		if calls == 1 {
			return runner.Result{ExitCode: 1, Stderr: "first ordinary failure"}
		}
		return runner.Result{ExitCode: 0, Stdout: "rerun passed"}
	}, Ledger: store})
	snapshot, err := store.ReadSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Project(snapshot, LogicalSource(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if p.Verification.FlakyClassifications != 1 || p.Verification.Reruns != 1 || p.Verification.OrdinaryFailures != 1 || p.Verification.OrdinaryPasses != 1 {
		t.Fatalf("verification=%+v", p.Verification)
	}
}

func taskRun(t *testing.T, id int, task, op string, reason autonomoustaskrun.StopReason, metrics *autonomoustaskrun.MetricsEvidence, action string) ledger.RunWithEvents {
	e := taskEvent(t, task, op, int64(id), action, reason)
	e.Metrics = metrics
	return history(t, int64(id), "run-"+op, ledger.EventTaskRunStopped, e)
}
func taskEvent(t *testing.T, task, op string, sequence int64, action string, reason autonomoustaskrun.StopReason) autonomoustaskrun.LedgerEvent {
	done := fixed.Add(time.Duration(sequence) * time.Second)
	return autonomoustaskrun.LedgerEvent{SchemaVersion: autonomoustaskrun.LedgerEventSchemaVersion, OperationID: op, TaskID: task, Sequence: sequence, Stage: map[bool]string{true: "terminal", false: "cycle_completed"}[reason != ""], Cycle: sequence, Action: action, RunID: "worker-" + op, StopReason: reason, StartedAt: fixed, UpdatedAt: done, CompletedAt: func() *time.Time {
		if reason == "" {
			return nil
		}
		return &done
	}(), Statistics: autonomoustaskrun.Statistics{CyclesStarted: sequence, CyclesCompleted: sequence}}
}
func history(t *testing.T, id int64, run string, kind ledger.EventType, payload any) ledger.RunWithEvents {
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	done := fixed.Add(time.Duration(id) * time.Second)
	return ledger.RunWithEvents{Run: ledger.Run{ID: run, TaskID: "task-one", Task: "fixture", Status: ledger.StatusCompleted, StartedAt: fixed, CompletedAt: &done, DurationSeconds: int(done.Sub(fixed).Seconds())}, Events: []ledger.Event{{ID: id, RunID: run, Type: kind, Payload: raw, CreatedAt: done}}}
}
func decision() autonomous.DecisionReference {
	return autonomous.DecisionReference{DecisionID: "decision-one", RunID: "supervisor-one", TaskID: "task-one", Action: autonomous.ActionCorrect, WorkerProfile: autonomous.WorkerProfileCorrector, Artifact: evidence("decision"), CreatedAt: fixed}
}
func evidence(ref string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: autonomous.EvidenceKindTask, Reference: ref, Detail: "deterministic fixture evidence"}
}
func hash(value string) string { _ = value; return strings.Repeat("a", 64) }
func auditRaw(t *testing.T, clean bool) json.RawMessage {
	return auditRawRun(t, "task-one", "audit-one", clean)
}
func auditRawRun(t *testing.T, taskID, runID string, clean bool) json.RawMessage {
	disposition := autonomous.AuditDispositionChangesRequired
	var findings []autonomous.AuditFinding
	if clean {
		disposition = autonomous.AuditDispositionClean
	} else {
		findings = []autonomous.AuditFinding{{ID: "finding-one", Significance: autonomous.FindingSignificanceBlocking, Summary: "deterministic finding", Evidence: []autonomous.EvidenceReference{evidence("finding")}, RequiredCorrection: "repair the exact fixture"}}
	}
	value := autonomouspolicy.AuditEvidence{RunID: runID, Report: autonomous.AuditReport{TaskID: taskID, Disposition: disposition, Rationale: "deterministic audit result", Inputs: []autonomous.EvidenceReference{evidence("audit-input")}, Findings: findings}}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func verificationEvent() autonomousverification.CompletedLedgerEvent {
	return verificationEventTask("task-one")
}
func verificationEventTask(taskID string) autonomousverification.CompletedLedgerEvent {
	plan := autonomousverification.PlanIdentity{SchemaVersion: autonomousverification.PlanSchemaVersion, SHA256: hash("plan"), ByteSize: 1}
	gate := autonomousverification.GateEvidence{SchemaVersion: autonomousverification.GateSchemaVersion, Plan: plan, Purpose: autonomousverification.PurposeFinal, RequiredFinalTiers: []string{}, SelectedTiers: []string{}, ExecutedTiers: []string{}, RequiredOutcomes: []autonomousverification.TierGate{}, MissingRequired: []string{}, OverallOutcome: autonomousverification.OutcomePassed, FinalSatisfied: true}
	return autonomousverification.CompletedLedgerEvent{SchemaVersion: autonomousverification.LedgerEventSchemaVersion, Status: "passed", Passed: true, TaskID: taskID, OccurrenceID: "verify-one", SourceRevision: hash("source"), Plan: plan, Purpose: autonomousverification.PurposeFinal, Outcome: autonomousverification.OutcomePassed, Gate: gate, Tiers: []autonomousverification.TierResult{}}
}
