package autonomousarchive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/ledger"
	"revolvr/internal/redact"
	"revolvr/internal/taskfile"
)

var archiveTestTime = time.Date(2026, 7, 12, 23, 59, 59, 0, time.UTC)

func TestArchiveRefusesWhileQueueCoordinatorLeaseIsActive(t *testing.T) {
	root := t.TempDir()
	release, err := autonomousexec.Acquire(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = Archive(context.Background(), Config{RepositoryRoot: root}, ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-race", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime})
	if !errors.Is(err, autonomousexec.ErrActive) {
		t.Fatalf("archive contention err=%v", err)
	}
}

func TestArchiveCompletedCopiesAndVerifiesAW20Capsule(t *testing.T) {
	root, store := completedRepo(t)
	defer store.Close()
	reason := "Completion gates validated and completion capsule materialized."
	result, err := Archive(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ArchiveRequest{TaskID: "completed-task", OperationID: "archive-completed", ArchiveRunID: "archive-run-completed", Authority: TerminalAuthority{SchemaVersion: AuthoritySchemaVersion, Disposition: DispositionCompleted, Reason: reason, Provenance: "aw20:finalization", TerminalAt: archiveTestTime.Add(-2*time.Minute + time.Second)}, ArchivedAt: archiveTestTime})
	if err != nil {
		t.Fatal(err)
	}
	if result.Entry.Manifest.CompletionCapsule == nil || result.Entry.Manifest.Finalization == nil || result.Entry.Manifest.TerminalLedger == nil {
		t.Fatalf("completed manifest = %+v", result.Entry.Manifest)
	}
	archivedCapsule, err := readArtifactBytes(root, *result.Entry.Manifest.CompletionCapsule)
	if err != nil {
		t.Fatal(err)
	}
	activeCapsule, err := os.ReadFile(filepath.Join(root, ".revolvr", "autonomous", "tasks", "completed-task", "completion", "completion.md"))
	if err != nil || string(archivedCapsule) != string(activeCapsule) {
		t.Fatalf("capsule copy mismatch err=%v", err)
	}
	report, err := Verify(context.Background(), VerifyConfig{RepositoryRoot: root, Ledger: store}, result.Entry.Manifest.ArchiveID)
	if err != nil || !report.Passed {
		t.Fatalf("verify err=%v report=%+v", err, report)
	}
}

func TestArchiveEveryNonCompletedDispositionAndVerify(t *testing.T) {
	for _, disposition := range []Disposition{DispositionCancelled, DispositionSuperseded, DispositionAbandoned} {
		t.Run(string(disposition), func(t *testing.T) {
			root, store := terminalRepo(t, disposition)
			defer store.Close()
			result, err := Archive(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-" + string(disposition), ArchiveRunID: "archive-run-" + string(disposition), Authority: authority(disposition), ArchivedAt: archiveTestTime})
			if err != nil {
				t.Fatal(err)
			}
			if result.Journal.Stage != StageLedgerComplete || !validOID(result.CommitSHA) {
				t.Fatalf("result = %+v", result)
			}
			wantBase := filepath.ToSlash(filepath.Join(ArchiveRoot, "2026", "07", "terminal-task"))
			if result.Entry.Manifest.ArchivedTask.Path != wantBase+"/task.md" || result.Entry.Manifest.CompletionCapsule != nil {
				t.Fatalf("manifest = %+v", result.Entry.Manifest)
			}
			if _, found, err := taskfile.FindByID(root, "terminal-task"); err != nil || found {
				t.Fatalf("active task found=%t err=%v", found, err)
			}
			report, err := Verify(context.Background(), VerifyConfig{RepositoryRoot: root, Ledger: store}, result.Entry.Manifest.ArchiveID)
			if err != nil || !report.Passed {
				t.Fatalf("verify err=%v report=%+v", err, report)
			}
		})
	}
}

func TestArchiveCrashAfterActiveRemovalRollsForwardExactlyOnce(t *testing.T) {
	root, store := terminalRepo(t, DispositionCancelled)
	defer store.Close()
	failed := false
	cfg := Config{RepositoryRoot: root, Ledger: store, FailureInjector: func(point FailurePoint) error {
		if point == FailureAfterActiveRemoval && !failed {
			failed = true
			return fmt.Errorf("stop")
		}
		return nil
	}}
	request := ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-crash", ArchiveRunID: "archive-run-crash", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime}
	if _, err := Archive(context.Background(), cfg, request); err == nil || !strings.Contains(err.Error(), "after_active_task_removal") {
		t.Fatalf("first archive err = %v", err)
	}
	result, err := Archive(context.Background(), cfg, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replayed || result.Journal.Stage != StageLedgerComplete {
		t.Fatalf("recovery result = %+v", result)
	}
	history, found, err := store.GetRunWithEvents(context.Background(), request.ArchiveRunID)
	if err != nil || !found {
		t.Fatalf("ledger found=%t err=%v", found, err)
	}
	counts := map[ledger.EventType]int{}
	for _, event := range history.Events {
		counts[event.Type]++
	}
	for _, kind := range []ledger.EventType{ledger.EventArchivePrepared, ledger.EventArchiveFilesPublished, ledger.EventArchiveActiveRemoved, ledger.EventArchiveCommitReconciled, ledger.EventArchiveCompleted} {
		if counts[kind] != 1 {
			t.Fatalf("event %s count = %d", kind, counts[kind])
		}
	}
	if _, err := Archive(context.Background(), cfg, ArchiveRequest{TaskID: request.TaskID, OperationID: request.OperationID, ArchiveRunID: request.ArchiveRunID, Authority: authority(DispositionAbandoned), ArchivedAt: request.ArchivedAt}); err == nil {
		t.Fatal("conflicting operation reuse succeeded")
	}
}

func TestArchiveRejectsBlockedAndReopenCreatesNewLifecycle(t *testing.T) {
	root, store := terminalRepo(t, DispositionCancelled)
	defer store.Close()
	archived, err := Archive(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-reopen", ArchiveRunID: "archive-run-reopen", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime})
	if err != nil {
		t.Fatal(err)
	}
	reopenedAt := archiveTestTime.Add(time.Minute)
	reopened, err := Reopen(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ReopenRequest{Selector: archived.Entry.Manifest.ArchiveID, OperationID: "reopen-one", NewTaskID: "terminal-task-reopened", Authority: "operator:test", Reason: "new requirements", ReopenedAt: reopenedAt})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Task.Status != taskfile.StatusPending || reopened.State.State.Lifecycle != autonomous.LifecycleStatePending || reopened.State.State.ReopenedFrom == nil || reopened.State.State.ReopenedFrom.ArchiveID != archived.Entry.Manifest.ArchiveID {
		t.Fatalf("reopened = %+v", reopened)
	}
	if !strings.Contains(string(reopened.Task.SourceBytes), "unknown: preserved\n") || !strings.HasSuffix(string(reopened.Task.SourceBytes), "Exact spec without final newline") {
		t.Fatalf("reopened task did not preserve unknown metadata/spec bytes: %q", reopened.Task.SourceBytes)
	}
	next, found, err := taskfile.SelectNextForWorkflow(root, taskfile.WorkflowAutonomousV1)
	if err != nil || !found || next.ID != "terminal-task-reopened" {
		t.Fatalf("next = %+v found=%t err=%v", next, found, err)
	}
	replay, err := Reopen(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ReopenRequest{Selector: archived.Entry.Manifest.ArchiveID, OperationID: "reopen-one", NewTaskID: "terminal-task-reopened", Authority: "operator:test", Reason: "new requirements", ReopenedAt: reopenedAt})
	if err != nil || !replay.Replayed || replay.Record.CommitSHA != reopened.Record.CommitSHA {
		t.Fatalf("replay = %+v err=%v", replay, err)
	}
	if _, err := Reopen(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ReopenRequest{Selector: archived.Entry.Manifest.ArchiveID, OperationID: "reopen-two", NewTaskID: "another-task", Authority: "operator:test", Reason: "different lifecycle", ReopenedAt: reopenedAt.Add(time.Minute)}); err == nil {
		t.Fatal("second reopen succeeded")
	}
	report, err := Verify(context.Background(), VerifyConfig{RepositoryRoot: root, Ledger: store}, archived.Entry.Manifest.ArchiveID)
	if err != nil || !report.Passed {
		t.Fatalf("verify after reopen err=%v report=%+v", err, report)
	}

	blockedRoot, blockedStore := lifecycleRepo(t, autonomous.LifecycleStateBlocked, taskfile.StatusBlocked, "blocked")
	defer blockedStore.Close()
	_, err = Archive(context.Background(), Config{RepositoryRoot: blockedRoot, Ledger: blockedStore}, ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-blocked", Authority: TerminalAuthority{SchemaVersion: AuthoritySchemaVersion, Disposition: DispositionCancelled, Reason: "blocked", Provenance: "operator:test", TerminalAt: archiveTestTime.Add(-time.Minute)}, ArchivedAt: archiveTestTime})
	if err == nil {
		t.Fatal("blocked task archive succeeded")
	}
}

func TestReopenRecoversStateAndTaskPublishedBeforeCommit(t *testing.T) {
	root, store := terminalRepo(t, DispositionCancelled)
	defer store.Close()
	archived, err := Archive(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-partial-reopen", ArchiveRunID: "archive-run-partial-reopen", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime})
	if err != nil {
		t.Fatal(err)
	}
	m := archived.Entry.Manifest
	request := ReopenRequest{Selector: m.ArchiveID, OperationID: "reopen-partial", NewTaskID: "partial-reopen-task", Authority: "operator:test", Reason: "continue with revised scope", ReopenedAt: archiveTestTime.Add(time.Minute)}
	archivedBytes, err := readArtifactBytes(root, m.ArchivedTask)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := taskfile.ProjectReopenedTask(root, taskfile.ReopenInput{OriginalSourcePath: m.OriginalTask.Path, ArchivedSourceBytes: archivedBytes, NewTaskID: request.NewTaskID})
	if err != nil {
		t.Fatal(err)
	}
	lineage := autonomous.ReopenLineage{SchemaVersion: autonomous.ReopenLineageSchemaVersion, OperationID: request.OperationID, ArchiveID: m.ArchiveID, ArchivedTaskID: m.TaskID, ArchivedTaskSHA256: m.ArchivedTask.SHA256, ArchivedTaskSize: m.ArchivedTask.ByteSize, Disposition: string(m.Disposition), ArchiveCommitSHA: archived.CommitSHA, Authority: request.Authority, Reason: request.Reason, ReopenedAt: request.ReopenedAt}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: request.NewTaskID, Lifecycle: autonomous.LifecycleStatePending, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}, ReopenedFrom: &lineage}
	stateBytes, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeImmutable(root, artifact(projected.AutonomousStatePath, stateBytes), stateBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := taskfile.PublishReopenedTask(root, projected); err != nil {
		t.Fatal(err)
	}
	result, err := Reopen(context.Background(), Config{RepositoryRoot: root, Ledger: store}, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || !validOID(result.Record.CommitSHA) {
		t.Fatalf("recovered result = %+v", result)
	}
}

func TestVerifyIsReadOnlyAndReportsTrackedTaskTampering(t *testing.T) {
	root, store := terminalRepo(t, DispositionCancelled)
	defer store.Close()
	archived, err := Archive(context.Background(), Config{RepositoryRoot: root, Ledger: store}, ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-verify-readonly", ArchiveRunID: "archive-run-verify-readonly", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime})
	if err != nil {
		t.Fatal(err)
	}
	headBefore := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	statusBefore := runGitTest(t, root, "status", "--porcelain=v1", "--untracked-files=all")
	historyBefore, _, err := store.GetRunWithEvents(context.Background(), archived.Entry.Manifest.ArchiveRunID)
	if err != nil {
		t.Fatal(err)
	}
	manifestInfoBefore, err := os.Stat(filepath.Join(root, filepath.FromSlash(archived.Entry.ManifestPath)))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), VerifyConfig{RepositoryRoot: root, Ledger: store}, archived.Entry.Manifest.ArchiveID)
	if err != nil || !report.Passed {
		t.Fatalf("verify err=%v report=%+v", err, report)
	}
	historyAfter, _, _ := store.GetRunWithEvents(context.Background(), archived.Entry.Manifest.ArchiveRunID)
	manifestInfoAfter, _ := os.Stat(filepath.Join(root, filepath.FromSlash(archived.Entry.ManifestPath)))
	if got := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD")); got != headBefore || runGitTest(t, root, "status", "--porcelain=v1", "--untracked-files=all") != statusBefore || len(historyAfter.Events) != len(historyBefore.Events) || !manifestInfoAfter.ModTime().Equal(manifestInfoBefore.ModTime()) {
		t.Fatal("read-only verification changed Git, ledger, or manifest metadata")
	}
	taskPath := filepath.Join(root, filepath.FromSlash(archived.Entry.Manifest.ArchivedTask.Path))
	if err := os.WriteFile(taskPath, append([]byte(nil), []byte("tampered\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = Verify(context.Background(), VerifyConfig{RepositoryRoot: root, Ledger: store}, archived.Entry.Manifest.ArchiveID)
	if err != nil || report.Passed || checkPassed(report, "task_bytes") {
		t.Fatalf("tampered verify err=%v report=%+v", err, report)
	}
}

func TestArchiveRejectsSecretAndSymlinkBeforeAdmission(t *testing.T) {
	root, store := terminalRepo(t, DispositionCancelled)
	defer store.Close()
	request := ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-secret", ArchiveRunID: "archive-run-secret", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime}
	if _, err := Archive(context.Background(), Config{RepositoryRoot: root, Ledger: store, ForbiddenValues: []string{"Exact spec"}}, request); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret admission err = %v", err)
	}
	if _, found, err := taskfile.FindByID(root, "terminal-task"); err != nil || !found {
		t.Fatalf("task preserved found=%t err=%v", found, err)
	}
	outside := t.TempDir()
	year := filepath.Join(root, ".agent", "archive", "2026")
	if err := os.MkdirAll(filepath.Dir(year), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, year); err != nil {
		t.Fatal(err)
	}
	if _, err := List(root); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink list err = %v", err)
	}
}

func checkPassed(report VerificationReport, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return check.Passed
		}
	}
	return false
}

func terminalRepo(t *testing.T, disposition Disposition) (string, *ledger.Store) {
	t.Helper()
	lifecycle := map[Disposition]autonomous.LifecycleState{DispositionCancelled: autonomous.LifecycleStateCancelled, DispositionSuperseded: autonomous.LifecycleStateSuperseded, DispositionAbandoned: autonomous.LifecycleStateAbandoned}[disposition]
	return lifecycleRepo(t, lifecycle, string(disposition), "terminal "+string(disposition))
}

func lifecycleRepo(t *testing.T, lifecycle autonomous.LifecycleState, status, reason string) (string, *ledger.Store) {
	t.Helper()
	root := t.TempDir()
	runGitTest(t, root, "init", "-q")
	runGitTest(t, root, "config", "user.name", "Revolvr Test")
	runGitTest(t, root, "config", "user.email", "revolvr@example.test")
	if err := os.WriteFile(filepath.Join(root, ".git", "info", "exclude"), []byte("/.revolvr/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := ".revolvr/autonomous/tasks/terminal-task/state.json"
	taskRaw := []byte("---\nid: terminal-task\nstatus: " + status + "\nworkflow: autonomous-v1\nautonomous_state_path: " + statePath + "\nunknown: preserved\n---\n# Terminal task\r\n\r\nExact spec without final newline")
	taskPath := filepath.Join(root, ".agent", "tasks", "terminal-task.md")
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath, taskRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "terminal-task", Lifecycle: lifecycle, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}, Terminal: &autonomous.TerminalDetail{Reason: reason}}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	stateAbs := filepath.Join(root, filepath.FromSlash(statePath))
	if err := os.MkdirAll(filepath.Dir(stateAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateAbs, stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "--", ".agent/tasks/terminal-task.md")
	runGitTest(t, root, "commit", "-q", "-m", "seed terminal task")
	store, err := ledger.Open(context.Background(), filepath.Join(root, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	return root, store
}

func completedRepo(t *testing.T) (string, *ledger.Store) {
	t.Helper()
	root := t.TempDir()
	runGitTest(t, root, "init", "-q")
	runGitTest(t, root, "config", "user.name", "Revolvr Test")
	runGitTest(t, root, "config", "user.email", "revolvr@example.test")
	if err := os.WriteFile(filepath.Join(root, ".git", "info", "exclude"), []byte("/.revolvr/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	taskID := "completed-task"
	statePath := ".revolvr/autonomous/tasks/completed-task/state.json"
	taskPath := ".agent/tasks/completed-task.md"
	taskRaw := []byte("---\nid: completed-task\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: " + statePath + "\n---\n# Completed task\n\nFinish it.\n")
	if err := os.MkdirAll(filepath.Join(root, ".agent", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(taskPath)), taskRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "--", taskPath)
	runGitTest(t, root, "commit", "-q", "-m", "seed completed task")
	head := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD^{tree}"))
	now := archiveTestTime.Add(-2 * time.Minute)
	source := strings.Repeat("b", 64)
	workspace := autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: taskID, WorkspaceID: "workspace-one", ControlRoot: root, ExecutionRoot: filepath.Join(root, ".revolvr", "autonomous", "worktrees", "workspace-one"), GitCommonDir: filepath.Join(root, ".git"), BranchRef: "refs/heads/revolvr/tasks/completed-task-one", OwnerMarker: filepath.Join(root, ".revolvr", "autonomous", "workspaces", "workspace-one.json"), BaselineSHA: head, HeadSHA: head, TreeSHA: tree, SourceRevision: source, Checkpoint: autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: head, TreeSHA: tree, SourceRevision: source, OperationID: "workspace-create", Provenance: "baseline", CreatedAt: now.Add(-time.Hour)}, Status: autonomous.WorkspaceStatusReady, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute)}
	verificationEvidence := archiveEvidence(autonomous.EvidenceKindVerification, "verification-run/verification-final")
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStateReady, Plan: &autonomous.TaskPlan{TaskID: taskID, ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{archiveEvidence(autonomous.EvidenceKindTask, taskPath)}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "Implement and verify the task.", Status: autonomous.PlanStepStatusCompleted, Evidence: []autonomous.EvidenceReference{verificationEvidence}}}, Completed: true}, AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "The task is verified.", Status: autonomous.AcceptanceStatusSatisfied, Evidence: []autonomous.EvidenceReference{verificationEvidence}}}, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}, Workspace: &workspace}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	stateAbs := filepath.Join(root, filepath.FromSlash(statePath))
	if err := os.MkdirAll(filepath.Dir(stateAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateAbs, stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	stateIdentity, err := autonomousstate.StateIdentityFor(statePath, true, state)
	if err != nil {
		t.Fatal(err)
	}
	decision := autonomous.SupervisorDecision{TaskID: taskID, Action: autonomous.ActionComplete, Rationale: "All exact gates are satisfied.", Inputs: []autonomous.EvidenceReference{verificationEvidence}}
	reference := autonomous.DecisionReference{DecisionID: "decision-complete", RunID: "supervisor-complete", TaskID: taskID, Action: autonomous.ActionComplete, Artifact: archiveEvidence(autonomous.EvidenceKindFile, ".revolvr/runs/supervisor-complete/decision.json"), CreatedAt: now.Add(-time.Minute)}
	planIdentity := autonomousverification.PlanIdentity{SchemaVersion: autonomousverification.PlanSchemaVersion, SHA256: strings.Repeat("d", 64), ByteSize: 12}
	gate := autonomousverification.GateEvidence{SchemaVersion: autonomousverification.GateSchemaVersion, Plan: planIdentity, Purpose: autonomousverification.PurposeFinal, RequiredFinalTiers: []string{"full-suite"}, SelectedTiers: []string{"full-suite"}, ExecutedTiers: []string{"full-suite"}, RequiredOutcomes: []autonomousverification.TierGate{{TierID: "full-suite", Outcome: autonomousverification.OutcomePassed}}, OverallOutcome: autonomousverification.OutcomePassed, FinalSatisfied: true}
	verification := autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: taskID, Status: autonomous.VerificationStatusPassed, Summary: "All final tiers passed.", RunID: "verification-run", OccurrenceID: "verification-final", Evidence: []autonomous.EvidenceReference{verificationEvidence}}, SourceRevision: source, Tiered: &gate}
	audit := autonomouspolicy.AuditEvidence{Report: autonomous.AuditReport{TaskID: taskID, Disposition: autonomous.AuditDispositionClean, Rationale: "Independent review is clean.", Inputs: []autonomous.EvidenceReference{verificationEvidence}}, RunID: "audit-run", AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: source, VerificationRunID: "verification-run", VerificationOccurrenceID: "verification-final"}
	sourceEvidence := autonomouspolicy.SourceEvidence{Revision: source, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: &autonomouspolicy.SourceMutation{TaskID: taskID, RunID: "worker-run", DecisionID: "decision-worker", Action: autonomous.ActionImplement, ResultingRevision: source}}
	configHash := strings.Repeat("e", 64)
	policy := autonomoussafety.Policy{SchemaVersion: autonomoussafety.PolicySchemaVersion, TaskID: taskID, Workspace: workspace, Mode: autonomoussafety.ModeOperatorAttended, Codex: autonomoussafety.CodexPolicy{Sandbox: "danger-full-access", ApprovalPolicy: "never", DangerousBypass: true, Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", Ephemeral: true}, ExternalIsolation: autonomoussafety.ExternalIsolation{Expectation: autonomoussafety.IsolationNone, Enforcement: autonomoussafety.EnforcementNone}, Network: autonomoussafety.NetworkPolicy{Access: autonomoussafety.NetworkUnknown, Enforcement: autonomoussafety.EnforcementNone}, Hooks: autonomoussafety.HookTrust{Policy: autonomoussafety.HooksOperatorAttended}, Environment: autonomoussafety.EnvironmentPolicy{InheritHost: true}, Redaction: redact.Policy{SchemaVersion: redact.PolicySchemaVersion}, RedactionPolicyHash: strings.Repeat("f", 64), ConfigPath: ".revolvr/config.yaml", ConfigSHA256: configHash, WorktreeNotice: "Git worktree isolation is source/Git isolation, not a security sandbox."}
	policy, err = autonomoussafety.FinalizePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	preflight := autonomoussafety.PreflightResult{SchemaVersion: autonomoussafety.PreflightSchemaVersion, TaskID: taskID, WorkspaceID: workspace.WorkspaceID, SourceRevision: source, PolicySHA256: policy.PolicySHA256, ConfigSHA256: configHash, ObservedAt: now.Add(-30 * time.Second), Ready: true, Checks: []autonomoussafety.Check{{Name: "ready", Status: autonomoussafety.CheckOK, Detail: "exact authority ready"}}}
	completedRaw := []byte(strings.Replace(string(taskRaw), "status: pending", "status: completed", 1))
	frozen := autonomousfinalization.FrozenEvidence{SchemaVersion: autonomousfinalization.FrozenEvidenceSchemaVersion, OperationID: "finalize-one", FinalizationRunID: "finalization-run", Task: autonomousfinalization.TaskSource{TaskID: taskID, Title: "Completed task", Path: taskPath, SHA256: artifact(taskPath, taskRaw).SHA256, ByteSize: len(taskRaw), Workflow: taskfile.WorkflowAutonomousV1, StatePath: statePath, CompletedSHA256: artifact(taskPath, completedRaw).SHA256, CompletedByteSize: len(completedRaw)}, State: state, StateIdentity: stateIdentity, Decision: decision, DecisionReference: reference, Source: sourceEvidence, Verification: verification, Audit: audit, Workspace: workspace, SafetyPolicy: policy, SafetyPreflight: preflight, EffectiveConfigSchema: "revolvr-effective-run-config-v2", EffectiveConfigSHA256: configHash, Runs: []autonomousfinalization.RunEvidence{{Sequence: 1, RunID: "supervisor-complete", Kind: "supervisor", Outcome: "completed", Artifact: reference.Artifact, StartedAt: now.Add(-2 * time.Minute), CompletedAt: now.Add(-time.Minute)}, {Sequence: 2, RunID: "verification-run", Kind: "verification", Outcome: "passed", Artifact: verificationEvidence, StartedAt: now.Add(-time.Minute), CompletedAt: now.Add(-45 * time.Second)}, {Sequence: 3, RunID: "audit-run", Kind: "audit", Outcome: "clean", Artifact: archiveEvidence(autonomous.EvidenceKindAudit, "audit-run/report"), StartedAt: now.Add(-40 * time.Second), CompletedAt: now.Add(-30 * time.Second)}}, Provenance: []autonomous.EvidenceReference{archiveEvidence(autonomous.EvidenceKindTask, taskPath)}, AdmittedAt: now, TerminalAt: now.Add(time.Second)}
	frozen.Route, err = autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: taskID, Decision: decision, Reference: reference, State: state, Source: sourceEvidence, Verification: &verification, Audit: &audit})
	if err != nil {
		t.Fatal(err)
	}
	store, err := ledger.Open(context.Background(), filepath.Join(root, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	stateStore, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := autonomousfinalization.Finalize(context.Background(), autonomousfinalization.Config{RepositoryRoot: root, Evidence: frozen, StateStore: stateStore, Ledger: store, RevalidateEvidence: func(context.Context, autonomousfinalization.FrozenEvidence) error { return nil }}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return root, store
}

func archiveEvidence(kind autonomous.EvidenceKind, reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: "trusted harness evidence"}
}

func authority(disposition Disposition) TerminalAuthority {
	return TerminalAuthority{SchemaVersion: AuthoritySchemaVersion, Disposition: disposition, Reason: "terminal " + string(disposition), Provenance: "operator:test", TerminalAt: archiveTestTime.Add(-time.Minute)}
}

func runGitTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
