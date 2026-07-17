package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/ledger"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
)

func TestRecoverAutonomousTaskRequiresExactReconciliation(t *testing.T) {
	root, taskID, operationID, now := recoveryAppFixture(t)
	readOnlyBefore := productionTerminalTree(t, filepath.Join(root, ".revolvr"))
	inspected, err := RecoverAutonomousTask(context.Background(), Config{WorkDir: root}, RecoverAutonomousTaskInput{
		TaskID: taskID, OperationID: operationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inspected.Ready || !inspected.ReconcileEligible || inspected.Reconciled || len(inspected.Checks) != 7 {
		t.Fatalf("read-only recovery = %+v", inspected)
	}
	wantChecks := []string{"task", "state", "workspace", "git", "ledger", "receipt", "artifacts"}
	for i, check := range inspected.Checks {
		if check.Name != wantChecks[i] || !check.Passed || strings.TrimSpace(check.Detail) == "" {
			t.Fatalf("check[%d] = %+v, want passing %s authority", i, check, wantChecks[i])
		}
	}
	if after := productionTerminalTree(t, filepath.Join(root, ".revolvr")); !reflect.DeepEqual(after, readOnlyBefore) {
		t.Fatalf("read-only recovery changed runtime evidence\nbefore=%v\nafter=%v", readOnlyBefore, after)
	}

	taskRuns := filepath.Join(root, ".revolvr", "autonomous", "task-runs")
	beforeRefusal := productionTerminalTree(t, taskRuns)
	if _, err := RecoverAutonomousTask(context.Background(), Config{WorkDir: root}, RecoverAutonomousTaskInput{
		TaskID: taskID, OperationID: operationID, Reconcile: true, ConfirmOperation: "different-operation",
	}); err == nil || !strings.Contains(err.Error(), "exactly match") {
		t.Fatalf("confirmation error = %v", err)
	}
	if after := productionTerminalTree(t, taskRuns); !reflect.DeepEqual(after, beforeRefusal) {
		t.Fatalf("confirmation refusal created recovery evidence\nbefore=%v\nafter=%v", beforeRefusal, after)
	}

	oldBefore := productionTerminalTree(t, filepath.Join(taskRuns, operationID))
	reconciled, err := RecoverAutonomousTask(context.Background(), Config{WorkDir: root}, RecoverAutonomousTaskInput{
		TaskID: taskID, OperationID: operationID, Reconcile: true, ConfirmOperation: operationID,
		Clock: func() time.Time { return now.Add(time.Hour) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reconciled.Reconciled || reconciled.Replayed || reconciled.NewOperationID == "" || reconciled.NewOperationID == operationID {
		t.Fatalf("reconciled recovery = %+v", reconciled)
	}
	if oldAfter := productionTerminalTree(t, filepath.Join(taskRuns, operationID)); !reflect.DeepEqual(oldAfter, oldBefore) {
		t.Fatalf("reconciliation changed old operation\nbefore=%v\nafter=%v", oldBefore, oldAfter)
	}
	newOperation, found, err := autonomoustaskrun.Inspect(root, reconciled.NewOperationID)
	if err != nil || !found {
		t.Fatalf("inspect new operation: found=%t err=%v", found, err)
	}
	if newOperation.Stage != "admitted" || newOperation.InFlight || newOperation.StopReason != "" ||
		!reflect.DeepEqual(newOperation.Evidence, []string{"reconciled_from_operation:" + operationID, "recovery_authority_sha256:" + reconciled.AuthoritySHA256}) {
		t.Fatalf("new operation = %+v", newOperation)
	}

	replayed, err := RecoverAutonomousTask(context.Background(), Config{WorkDir: root}, RecoverAutonomousTaskInput{
		TaskID: taskID, OperationID: operationID, Reconcile: true, ConfirmOperation: operationID,
		Clock: func() time.Time { return now.Add(2 * time.Hour) },
	})
	if err != nil || !replayed.Reconciled || !replayed.Replayed || replayed.NewOperationID != reconciled.NewOperationID {
		t.Fatalf("replayed reconciliation = %+v err=%v", replayed, err)
	}

	task, found, err := taskfile.FindByID(root, taskID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(task.SourcePath)), append(task.SourceBytes, []byte("\nchanged authority\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeDriftRefusal := productionTerminalTree(t, taskRuns)
	drifted, err := RecoverAutonomousTask(context.Background(), Config{WorkDir: root}, RecoverAutonomousTaskInput{
		TaskID: taskID, OperationID: operationID, Reconcile: true, ConfirmOperation: operationID,
	})
	if err == nil || drifted.Ready || !strings.Contains(err.Error(), "agreement from every authority") {
		t.Fatalf("drift reconciliation = %+v err=%v", drifted, err)
	}
	if after := productionTerminalTree(t, taskRuns); !reflect.DeepEqual(after, beforeDriftRefusal) {
		t.Fatalf("authority refusal changed operation evidence\nbefore=%v\nafter=%v", beforeDriftRefusal, after)
	}
	if _, err := RetryTask(context.Background(), Config{WorkDir: root}, taskID); err == nil {
		t.Fatal("generic retry unexpectedly admitted a non-blocked recovery task")
	}

	stateStore, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	state, found, err := stateStore.Load(context.Background(), taskID)
	if err != nil || !found || state.State.Workspace == nil {
		t.Fatalf("load recovery workspace: found=%t state=%+v err=%v", found, state, err)
	}
	writeProductionHappyFile(t, filepath.Join(state.State.Workspace.ExecutionRoot, "advanced-head.txt"), "unreconciled source evidence\n")
	runSchedulingGit(t, state.State.Workspace.ExecutionRoot, "add", "advanced-head.txt")
	runSchedulingGit(t, state.State.Workspace.ExecutionRoot, "commit", "-q", "-m", "Advance recovery workspace")
	refsBeforeDriftInspection := runSchedulingGit(t, root, "for-each-ref", "--sort=refname", "--format=%(refname) %(objectname)")
	repositoryBeforeDriftInspection := snapshotExternalTree(t, root)
	driftInspection, err := RecoverAutonomousTask(context.Background(), Config{WorkDir: root}, RecoverAutonomousTaskInput{
		TaskID: taskID, OperationID: operationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if driftInspection.Ready || driftInspection.ReconcileEligible || driftInspection.Checks[3].Name != "git" || driftInspection.Checks[3].Passed || !strings.Contains(driftInspection.Checks[3].Detail, "observed_head=") {
		t.Fatalf("advanced-HEAD recovery inspection = %+v", driftInspection)
	}
	if refsAfter := runSchedulingGit(t, root, "for-each-ref", "--sort=refname", "--format=%(refname) %(objectname)"); refsAfter != refsBeforeDriftInspection {
		t.Fatalf("read-only drift inspection changed refs\nbefore=%s\nafter=%s", refsBeforeDriftInspection, refsAfter)
	}
	if repositoryAfter := snapshotExternalTree(t, root); !reflect.DeepEqual(repositoryAfter, repositoryBeforeDriftInspection) {
		t.Fatalf("read-only drift inspection changed repository evidence\nbefore=%v\nafter=%v", repositoryBeforeDriftInspection, repositoryAfter)
	}
}

func recoveryAppFixture(t *testing.T) (string, string, string, time.Time) {
	t.Helper()
	root := t.TempDir()
	initializeSchedulingGitRepository(t, root)
	taskID, operationID := "recovery-task", "recovery-old-operation"
	task, err := taskfile.ProjectAutonomousTask(root, taskfile.AutonomousCreateInput{ID: taskID, Title: "Recover exact autonomous authority", Body: "Exercise explicit Level-1 recovery."})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = taskfile.PublishAutonomousTask(root, task); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        taskID,
		Lifecycle:     autonomous.LifecycleStateReady,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	writeProductionHappyFile(t, filepath.Join(root, filepath.FromSlash(task.AutonomousStatePath)), string(raw))
	runSchedulingGit(t, root, "add", ".agent")
	runSchedulingGit(t, root, "commit", "-q", "-m", "Seed recovery task")

	now := time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)
	clockTicks := int64(-1)
	clock := func() time.Time {
		clockTicks++
		return now.Add(time.Duration(clockTicks) * time.Second)
	}
	runCfg := DefaultRunOnceConfig(root)
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runonce.FingerprintEffectiveConfig(effective)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareTaskWorkspace(context.Background(), root, taskID, operationID, effective, clock); err != nil {
		t.Fatal(err)
	}
	paths, err := resolveStatePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := ledger.OpenWithClock(context.Background(), paths.LedgerDBPath, clock)
	if err != nil {
		t.Fatal(err)
	}
	persistions := 0
	injected := errors.New("injected interruption after in-flight history")
	base := autonomoustaskrun.Config{
		RepositoryRoot: root, OperationID: operationID, TaskID: taskID, ConfigSHA256: fingerprint.SHA256,
		MaxCycles: autonomoustaskrun.Limited(1), Clock: clock, Ledger: runs,
		Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			t.Fatal("interrupted operation unexpectedly reached its runner")
			return autonomoustaskrun.StepResult{}, nil
		},
	}
	interrupted := base
	interrupted.FailureInjector = func(point autonomoustaskrun.FailurePoint) error {
		if point == autonomoustaskrun.FailureAfterOperationHistory {
			persistions++
			if persistions == 2 {
				return injected
			}
		}
		return nil
	}
	if _, err := autonomoustaskrun.RunTaskUntilTerminal(context.Background(), interrupted); !errors.Is(err, injected) {
		t.Fatalf("interruption error = %v", err)
	}
	result, err := autonomoustaskrun.RunTaskUntilTerminal(context.Background(), base)
	if err != nil || result.StopReason != autonomoustaskrun.StopUnsafeAmbiguous {
		t.Fatalf("unsafe terminalization = %+v err=%v", result, err)
	}
	if err := runs.Close(); err != nil {
		t.Fatal(err)
	}
	return root, taskID, operationID, now
}
