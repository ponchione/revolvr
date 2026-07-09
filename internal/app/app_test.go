package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
	"revolvr/internal/verification"
)

func TestStatusUninitializedDoesNotCreateState(t *testing.T) {
	workDir := t.TempDir()

	result, err := Status(context.Background(), Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if result.Initialized {
		t.Fatal("status initialized = true, want false")
	}
	if _, err := os.Stat(filepath.Join(workDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}
}

func TestStatusReturnsTasksRecentRunsAndLatestEvents(t *testing.T) {
	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	base := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	now := base
	tasks, err := taskqueue.OpenWithClock(ctx, paths.TaskDBPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	for _, spec := range []taskqueue.TaskSpec{
		{ID: "task-pending", Task: "pending task", CreatedAt: base},
		{ID: "task-blocked", Task: "blocked task", CreatedAt: base.Add(time.Minute)},
		{ID: "task-completed", Task: "completed task", CreatedAt: base.Add(2 * time.Minute)},
	} {
		if _, err := tasks.AddTask(ctx, spec); err != nil {
			t.Fatalf("add %s: %v", spec.ID, err)
		}
	}
	now = base.Add(3 * time.Minute)
	if _, ok, err := tasks.BlockTask(ctx, "task-blocked", "waiting"); err != nil || !ok {
		t.Fatalf("block task: ok=%v err=%v", ok, err)
	}
	now = base.Add(4 * time.Minute)
	if _, ok, err := tasks.CompleteTask(ctx, "task-completed", "done"); err != nil || !ok {
		t.Fatalf("complete task: ok=%v err=%v", ok, err)
	}
	if err := tasks.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	eventTime := base
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	for _, spec := range []ledger.RunSpec{
		{ID: "run-old", TaskID: "task-old", Task: "old run", Status: ledger.StatusCompleted, StartedAt: base},
		{ID: "run-new", TaskID: "task-new", Task: "new run", Status: ledger.StatusFailed, StartedAt: base.Add(time.Hour)},
	} {
		if _, err := runs.CreateRun(ctx, spec); err != nil {
			t.Fatalf("create %s: %v", spec.ID, err)
		}
	}
	if _, err := runs.AppendEvent(ctx, "run-old", ledger.EventRunStarted, map[string]any{"run_id": "run-old"}); err != nil {
		t.Fatalf("append old event: %v", err)
	}
	eventTime = base.Add(time.Hour)
	if _, err := runs.AppendEvent(ctx, "run-new", ledger.EventRunStarted, map[string]any{"run_id": "run-new"}); err != nil {
		t.Fatalf("append new start event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-new", ledger.EventRunFailed, map[string]any{"message": "failed"}); err != nil {
		t.Fatalf("append new failed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	result, err := Status(ctx, Config{WorkDir: workDir, RecentRunsLimit: 1})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !result.Initialized {
		t.Fatal("status initialized = false, want true")
	}
	if got, want := taskStatuses(result.Tasks), map[string]string{
		"task-pending":   taskqueue.StatusPending,
		"task-blocked":   taskqueue.StatusBlocked,
		"task-completed": taskqueue.StatusCompleted,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task statuses = %#v, want %#v", got, want)
	}
	if got, want := runIDs(result.RecentRuns), []string{"run-new"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent runs = %#v, want %#v", got, want)
	}
	if got, want := eventTypes(result.LatestEvents), []ledger.EventType{ledger.EventRunStarted, ledger.EventRunFailed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("latest event types = %#v, want %#v", got, want)
	}
}

func TestShowRunReturnsPersistedHistory(t *testing.T) {
	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	eventTime := startedAt
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:        "run-show",
		TaskID:    "task-show",
		Task:      "Show one run",
		Status:    ledger.StatusCompleted,
		StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-show", ledger.EventRunStarted, map[string]any{"run_id": "run-show"}); err != nil {
		t.Fatalf("append start event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-show", ledger.EventRunCompleted, map[string]any{"summary": "done"}); err != nil {
		t.Fatalf("append completed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	history, err := ShowRun(ctx, Config{WorkDir: workDir}, " run-show ")
	if err != nil {
		t.Fatalf("show run: %v", err)
	}
	if got, want := history.Run.ID, "run-show"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := eventTypes(history.Events), []ledger.EventType{ledger.EventRunStarted, ledger.EventRunCompleted}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}

func TestShowRunReportsUninitializedAndMissingRun(t *testing.T) {
	ctx := context.Background()
	uninitializedDir := t.TempDir()

	if _, err := ShowRun(ctx, Config{WorkDir: uninitializedDir}, "run-missing-state"); err == nil || !strings.Contains(err.Error(), "state is not initialized") {
		t.Fatalf("show uninitialized error = %v, want state not initialized", err)
	}
	if _, err := os.Stat(filepath.Join(uninitializedDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}

	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	if _, err := ShowRun(ctx, Config{WorkDir: workDir}, "missing-run"); err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("show missing run error = %v, want not found", err)
	}
}

func TestValidateReceiptReturnsPersistedValidationResult(t *testing.T) {
	workDir := t.TempDir()
	completedAt := time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)
	createAppValidationRun(t, workDir, appValidationRunSpec{
		RunID:              "run-valid-receipt",
		TaskID:             "task-valid-receipt",
		Task:               "Validate a receipt",
		CompletedAt:        completedAt,
		CommitSHA:          "abc123def456",
		ChangedFiles:       []string{"internal/feature.go"},
		VerificationStatus: "passed",
		Verification: []receipt.VerificationEntry{{
			Command:  "go test ./...",
			ExitCode: 0,
			Status:   "passed",
		}},
		WriteArtifacts: true,
	})

	result, err := ValidateReceipt(context.Background(), Config{WorkDir: workDir}, " run-valid-receipt ")
	if err != nil {
		t.Fatalf("validate receipt: %v", err)
	}
	if got, want := result.RunID, "run-valid-receipt"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := result.ReceiptPath, filepath.Join(".revolvr", "receipts", "run-valid-receipt.md"); got != want {
		t.Fatalf("receipt path = %q, want %q", got, want)
	}
	if !result.Passed() {
		t.Fatalf("validation passed = false, checks = %#v", result.Checks)
	}
	wantChecks := map[string]string{
		receipt.ValidationCheckIdentity:            "ok",
		receipt.ValidationCheckCompletionTime:      "ok",
		receipt.ValidationCheckCommitSHA:           "ok",
		receipt.ValidationCheckChangedFiles:        "ok",
		receipt.ValidationCheckVerificationResults: "ok",
		receipt.ValidationCheckArtifacts:           "ok",
	}
	if got := validationCheckMessages(result); !reflect.DeepEqual(got, wantChecks) {
		t.Fatalf("check messages = %#v, want %#v", got, wantChecks)
	}
}

func TestValidateReceiptReturnsFailedChecksWithoutCommandError(t *testing.T) {
	workDir := t.TempDir()
	completedAt := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	createAppValidationRun(t, workDir, appValidationRunSpec{
		RunID:              "run-invalid-receipt",
		TaskID:             "task-invalid-receipt",
		Task:               "Validate a stale receipt",
		CompletedAt:        completedAt,
		CommitSHA:          "abc123def456",
		ChangedFiles:       []string{"internal/actual.go"},
		VerificationStatus: "passed",
		Verification: []receipt.VerificationEntry{{
			Command:  "go test ./...",
			ExitCode: 0,
			Status:   "passed",
		}},
		ReceiptTimestamp:    completedAt.Add(time.Minute),
		ReceiptChangedFiles: []string{"internal/stale.go"},
		WriteArtifacts:      true,
	})

	result, err := ValidateReceipt(context.Background(), Config{WorkDir: workDir}, "run-invalid-receipt")
	if err != nil {
		t.Fatalf("validate receipt: %v", err)
	}
	if result.Passed() {
		t.Fatalf("validation passed = true, want failed checks")
	}
	if got, want := failedValidationCheckNames(result), []string{
		receipt.ValidationCheckCompletionTime,
		receipt.ValidationCheckChangedFiles,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failed check names = %#v, want %#v", got, want)
	}
}

func TestValidateReceiptReportsUninitializedMissingAndEmptyRun(t *testing.T) {
	ctx := context.Background()
	uninitializedDir := t.TempDir()

	if _, err := ValidateReceipt(ctx, Config{WorkDir: uninitializedDir}, " "); err == nil || !strings.Contains(err.Error(), "receipt validate: run id is required") {
		t.Fatalf("validate empty run id error = %v, want required run id", err)
	}
	if _, err := ValidateReceipt(ctx, Config{WorkDir: uninitializedDir}, "run-missing-state"); err == nil || !strings.Contains(err.Error(), "state is not initialized") {
		t.Fatalf("validate uninitialized error = %v, want state not initialized", err)
	}
	if _, err := os.Stat(filepath.Join(uninitializedDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}

	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	if _, err := ValidateReceipt(ctx, Config{WorkDir: workDir}, "missing-run"); err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("validate missing run error = %v, want not found", err)
	}
}

func TestPreflightReturnsReadySnapshot(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	createAppPreflightState(t, workDir)
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-test
git:
  executable: git-test
verification:
  commands:
    - name: go
`)

	result, err := Preflight(ctx, Config{WorkDir: workDir}, PreflightInput{
		CommandRunner: readyPreflightCommandRunner(t),
		LookPath: preflightLookPath(map[string]string{
			"codex-test": "/fake/bin/codex-test",
			"git-test":   "/fake/bin/git-test",
		}),
	})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if !result.Ready {
		t.Fatalf("preflight ready = false, checks = %#v", result.Checks)
	}
	want := []PreflightCheck{
		{Status: PreflightOK, Name: "state", Detail: "initialized at " + filepath.Join(workDir, ".revolvr")},
		{Status: PreflightOK, Name: "config", Detail: "loaded " + filepath.Join(workDir, ".revolvr", "config.yaml")},
		{Status: PreflightOK, Name: "codex executable", Detail: "/fake/bin/codex-test"},
		{Status: PreflightOK, Name: "git executable", Detail: "/fake/bin/git-test"},
		{Status: PreflightOK, Name: "git identity", Detail: "Revolvr Doctor <doctor@example.invalid>"},
		{Status: PreflightOK, Name: "worktree clean", Detail: "no changes"},
		{Status: PreflightOK, Name: "runtime state ignored", Detail: ".revolvr/ ignored by Git"},
		{Status: PreflightOK, Name: "verification commands", Detail: "1 command configured"},
	}
	if !reflect.DeepEqual(result.Checks, want) {
		t.Fatalf("preflight checks = %#v, want %#v", result.Checks, want)
	}
}

func TestPreflightReturnsFailedSnapshotWithActionableDetails(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: missing-codex
git:
  executable: git-test
verification:
  missing_policy: fail
`)

	result, err := Preflight(ctx, Config{WorkDir: workDir}, PreflightInput{
		CommandRunner: failedPreflightCommandRunner(t),
		LookPath: preflightLookPath(map[string]string{
			"git-test": "/fake/bin/git-test",
		}),
	})
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if result.Ready {
		t.Fatalf("preflight ready = true, want false")
	}
	want := []PreflightCheck{
		{Status: PreflightFail, Name: "state", Detail: "not initialized; run `revolvr init`"},
		{Status: PreflightOK, Name: "config", Detail: "loaded " + filepath.Join(workDir, ".revolvr", "config.yaml")},
		{Status: PreflightFail, Name: "codex executable", Detail: `"missing-codex" not found: executable missing-codex not found`},
		{Status: PreflightOK, Name: "git executable", Detail: "/fake/bin/git-test"},
		{Status: PreflightFail, Name: "git identity", Detail: "missing user.name and user.email"},
		{Status: PreflightFail, Name: "worktree clean", Detail: "dirty files: internal/app/preflight.go, scratch.txt"},
		{Status: PreflightFail, Name: "runtime state ignored", Detail: ".revolvr/ is not ignored; run `revolvr init`"},
		{Status: PreflightFail, Name: "verification commands", Detail: "no verification commands configured"},
	}
	if !reflect.DeepEqual(result.Checks, want) {
		t.Fatalf("preflight checks = %#v, want %#v", result.Checks, want)
	}
}

func TestTaskAddListAndRetryOperations(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	task, err := AddTask(ctx, Config{WorkDir: workDir}, AddTaskInput{
		Task:    "  Implement app task operations  ",
		Summary: "  app boundary  ",
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	if task.ID == "" {
		t.Fatal("added task id is empty")
	}
	if got, want := task.Task, "Implement app task operations"; got != want {
		t.Fatalf("task text = %q, want %q", got, want)
	}
	if got, want := task.Summary, "app boundary"; got != want {
		t.Fatalf("task summary = %q, want %q", got, want)
	}
	if got, want := task.Status, taskqueue.StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("listed tasks = %+v, want added task %s", tasks, task.ID)
	}

	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	store, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	if _, ok, err := store.BlockTask(ctx, task.ID, "verification failed"); err != nil || !ok {
		t.Fatalf("block task: ok=%v err=%v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	retried, err := RetryTask(ctx, Config{WorkDir: workDir}, " "+task.ID+" ")
	if err != nil {
		t.Fatalf("retry task: %v", err)
	}
	if retried.ID != task.ID || retried.Task != task.Task || retried.Summary != task.Summary {
		t.Fatalf("retried task = %+v, want same identity/text/summary as %+v", retried, task)
	}
	if retried.Status != taskqueue.StatusPending || retried.Blocker != "" || retried.BlockedAt != nil {
		t.Fatalf("retried task = %+v, want pending with blocker state cleared", retried)
	}
}

func TestTaskOperationsReportClearErrors(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	if _, err := AddTask(ctx, Config{WorkDir: workDir}, AddTaskInput{Task: "   "}); err == nil || !strings.Contains(err.Error(), "task add: task text is required") {
		t.Fatalf("add empty task error = %v, want required task text", err)
	}
	if _, err := RetryTask(ctx, Config{WorkDir: workDir}, " "); err == nil || !strings.Contains(err.Error(), "task retry: task id is required") {
		t.Fatalf("retry empty task error = %v, want required task id", err)
	}
	if _, err := UnblockTask(ctx, Config{WorkDir: workDir}, " "); err == nil || !strings.Contains(err.Error(), "task unblock: task id is required") {
		t.Fatalf("unblock empty task error = %v, want required task id", err)
	}
	if _, err := RetryTask(ctx, Config{WorkDir: workDir}, "missing-task"); err == nil || !strings.Contains(err.Error(), `task "missing-task" not found`) {
		t.Fatalf("retry missing task error = %v, want not found", err)
	}

	task, err := AddTask(ctx, Config{WorkDir: workDir}, AddTaskInput{Task: "already done"})
	if err != nil {
		t.Fatalf("add completed task: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	store, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	if _, ok, err := store.CompleteTask(ctx, task.ID, "done"); err != nil || !ok {
		t.Fatalf("complete task: ok=%v err=%v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	if _, err := RetryTask(ctx, Config{WorkDir: workDir}, task.ID); err == nil || !strings.Contains(err.Error(), fmt.Sprintf(`task %q is not blocked (status: completed)`, task.ID)) {
		t.Fatalf("retry completed task error = %v, want not blocked", err)
	}
	if _, err := UnblockTask(ctx, Config{WorkDir: workDir}, task.ID); err == nil || !strings.Contains(err.Error(), fmt.Sprintf(`task %q is not blocked (status: completed)`, task.ID)) {
		t.Fatalf("unblock completed task error = %v, want not blocked", err)
	}
}

func TestTaskImportDryRunReportsTasksWithoutCreatingState(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	result, err := ImportTasksFromMarkdown(ctx, Config{WorkDir: workDir}, ImportTasksFromMarkdownInput{
		DryRun: true,
		Markdown: []byte(`## Task: First import
Create the first task.

### Acceptance
- dry-run reports this task

## Task
Create the second task.

### Summary
Second import
`),
	})
	if err != nil {
		t.Fatalf("import dry-run: %v", err)
	}
	if !result.DryRun {
		t.Fatal("dry-run result flag = false, want true")
	}
	want := []ImportedTask{
		{
			Task:    "Create the first task.\n\n### Acceptance\n- dry-run reports this task",
			Summary: "First import",
		},
		{
			Task:    "Create the second task.",
			Summary: "Second import",
		},
	}
	if !reflect.DeepEqual(result.Tasks, want) {
		t.Fatalf("dry-run tasks = %#v, want %#v", result.Tasks, want)
	}
	assertNoAppStateDir(t, workDir)
}

func TestTaskImportWritePersistsTasksInOrderAndReturnsIDs(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	result, err := ImportTasksFromMarkdown(ctx, Config{WorkDir: workDir}, ImportTasksFromMarkdownInput{
		Markdown: []byte(`## Task: First
Create first.

## Task: Second
Create second.

## Task: Third
Create third.
`),
	})
	if err != nil {
		t.Fatalf("import tasks: %v", err)
	}
	if result.DryRun {
		t.Fatal("dry-run result flag = true, want false")
	}
	if got, want := len(result.Tasks), 3; got != want {
		t.Fatalf("created tasks = %d, want %d", got, want)
	}
	for i, task := range result.Tasks {
		if task.ID == "" {
			t.Fatalf("created task %d id is empty", i+1)
		}
	}
	if got, want := importTaskSummaries(result.Tasks), []string{"First", "Second", "Third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("result task summaries = %#v, want %#v", got, want)
	}

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if got, want := taskIDs(tasks), importedTaskIDs(result.Tasks); !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted task ids = %#v, want result ids %#v", got, want)
	}
	if got, want := taskSummaries(tasks), []string{"First", "Second", "Third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted summaries = %#v, want %#v", got, want)
	}
	if got, want := taskTexts(tasks), []string{"Create first.", "Create second.", "Create third."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted task text = %#v, want %#v", got, want)
	}
}

func TestTaskImportValidationFailureDoesNotPartiallyWrite(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	_, err := ImportTasks(ctx, Config{WorkDir: workDir}, ImportTasksInput{
		Tasks: []TaskImport{
			{Task: "valid first task", Summary: "first"},
			{Task: "   ", Summary: "invalid"},
			{Task: "valid third task", Summary: "third"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "task import: task 2: task text is required") {
		t.Fatalf("validation error = %v, want task 2 required text", err)
	}
	assertNoAppStateDir(t, workDir)
}

func TestTaskImportParseFailureDoesNotPartiallyWrite(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	_, err := ImportTasksFromMarkdown(ctx, Config{WorkDir: workDir}, ImportTasksFromMarkdownInput{
		Markdown: []byte(`## Task: First
Create first.

## Task: Invalid
Create invalid.

### Acceptance
- first acceptance

### Acceptance
- duplicate acceptance
`),
	})
	if err == nil || !strings.Contains(err.Error(), "task import: parse:") || !strings.Contains(err.Error(), "duplicate Acceptance") {
		t.Fatalf("parse error = %v, want duplicate Acceptance parse error", err)
	}
	assertNoAppStateDir(t, workDir)
}

func TestTaskImportEmptyImportDoesNotCreateState(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	result, err := ImportTasks(ctx, Config{WorkDir: workDir}, ImportTasksInput{})
	if err != nil {
		t.Fatalf("empty import: %v", err)
	}
	if result.DryRun {
		t.Fatal("empty import dry-run flag = true, want false")
	}
	if len(result.Tasks) != 0 {
		t.Fatalf("empty import tasks = %#v, want none", result.Tasks)
	}
	assertNoAppStateDir(t, workDir)
}

func TestRunOnceLoadsConfigAndProgressCallback(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-custom
  sandbox: danger-full-access
  approval_policy: on-request
  yolo: false
  timeout_seconds: 45
git:
  executable: git-custom
  timeout_seconds: 12
verification:
  missing_policy: pass
  commands:
    - name: go
      args: ["test", "./..."]
      dir: internal
      timeout_seconds: 9
commit:
  allow_pre_existing_dirty: true
  allow_missing_verification: true
  timeout_seconds: 30
output:
  codex_stdout_cap_bytes: 101
  codex_stderr_cap_bytes: 102
  git_stdout_cap_bytes: 103
  git_stderr_cap_bytes: 104
  verification_stdout_cap_bytes: 105
  verification_stderr_cap_bytes: 106
  commit_stdout_cap_bytes: 107
  commit_stderr_cap_bytes: 108
`)

	var got runonce.Config
	var progress []codexexec.ProgressEvent
	result, err := RunOnce(ctx, Config{WorkDir: workDir}, RunOnceInput{
		Runner: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
			got = cfg
			if cfg.CodexProgress == nil {
				t.Fatal("codex progress callback is nil")
			}
			cfg.CodexProgress(codexexec.ProgressEvent{Source: "codex", Message: "working"})
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-config"},
				Task:    taskqueue.Task{ID: "task-config"},
				Commit:  commit.Result{CommitSHA: "abc123"},
			}, nil
		},
		Progress: func(event codexexec.ProgressEvent) {
			progress = append(progress, event)
		},
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if err := RunOnceOutcomeError(result); err != nil {
		t.Fatalf("run once outcome error = %v, want nil", err)
	}
	if got.WorkingDir != workDir {
		t.Fatalf("working dir = %q, want %q", got.WorkingDir, workDir)
	}
	if got.CodexExecutable != "codex-custom" || got.CodexSandbox != "danger-full-access" || got.CodexApprovalPolicy != "on-request" || got.CodexBypassApprovalsAndSandbox || got.CodexTimeout != 45*time.Second {
		t.Fatalf("codex config = %+v, want config overrides", got)
	}
	if got.GitExecutable != "git-custom" || got.GitTimeout != 12*time.Second {
		t.Fatalf("git config = %+v, want config overrides", got)
	}
	if got.MissingVerificationPolicy != verification.MissingCommandsPass {
		t.Fatalf("missing policy = %q, want pass", got.MissingVerificationPolicy)
	}
	wantCommands := []verification.Command{{
		Name:    "go",
		Args:    []string{"test", "./..."},
		Dir:     "internal",
		Timeout: 9 * time.Second,
	}}
	if !reflect.DeepEqual(got.VerificationCommands, wantCommands) {
		t.Fatalf("verification commands = %#v, want %#v", got.VerificationCommands, wantCommands)
	}
	if !got.AllowPreExistingDirty || !got.AllowMissingVerification || got.CommitTimeout != 30*time.Second {
		t.Fatalf("commit config = %+v, want config overrides", got)
	}
	if got.CodexStdoutCap != 101 || got.CodexStderrCap != 102 ||
		got.GitStdoutCap != 103 || got.GitStderrCap != 104 ||
		got.VerificationStdoutCap != 105 || got.VerificationStderrCap != 106 ||
		got.CommitStdoutCap != 107 || got.CommitStderrCap != 108 {
		t.Fatalf("output caps = %+v, want config overrides", got)
	}
	if len(progress) != 1 || progress[0].Source != "codex" || progress[0].Message != "working" {
		t.Fatalf("progress events = %#v, want codex working event", progress)
	}
}

func TestRunOnceInvalidConfigDoesNotInvokeRunner(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
verification:
  missing_policy: maybe
`)

	called := false
	_, err := RunOnce(ctx, Config{WorkDir: workDir}, RunOnceInput{
		Runner: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid verification missing_policy") {
		t.Fatalf("run once error = %v, want invalid missing_policy", err)
	}
	if called {
		t.Fatal("runner was called after invalid config")
	}
}

func TestRunLoopStopsAfterRepeatedFailuresWithGuardrail(t *testing.T) {
	ctx := context.Background()
	calls := 0
	var passIDs []string

	result, err := RunLoop(ctx, Config{WorkDir: t.TempDir()}, RunLoopInput{
		MaxPasses: 3,
		Runner: func(context.Context, runonce.Config) (runonce.Result, error) {
			calls++
			return runonce.Result{
				Outcome: runonce.OutcomeVerificationFailed,
				Run:     ledger.Run{ID: fmt.Sprintf("run-failed-%d", calls)},
				Task:    taskqueue.Task{ID: "task-failed"},
				Message: "verification command 0 failed",
			}, nil
		},
		OnPass: func(result runonce.Result) error {
			passIDs = append(passIDs, result.Run.ID)
			return nil
		},
	})
	if err == nil || err.Error() != "run loop stopped after 2 consecutive failed or blocked passes" {
		t.Fatalf("run loop error = %v, want guardrail error", err)
	}
	if calls != 2 {
		t.Fatalf("run calls = %d, want 2", calls)
	}
	if got, want := passIDs, []string{"run-failed-1", "run-failed-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pass ids = %#v, want %#v", got, want)
	}
	wantStats := RunLoopStats{
		MaxPasses:                  3,
		Passes:                     2,
		FailedOrBlocked:            2,
		StopReason:                 "failure_guardrail",
		ConsecutiveFailedOrBlocked: 2,
	}
	if !reflect.DeepEqual(result.Stats, wantStats) {
		t.Fatalf("loop stats = %#v, want %#v", result.Stats, wantStats)
	}
}

func TestRunLoopStopsImmediatelyWhenOutcomeNeedsInspection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result runonce.Result
	}{
		{
			name: "blocked",
			result: runonce.Result{
				Outcome: runonce.OutcomeBlocked,
				Run:     ledger.Run{ID: "run-blocked"},
				Task:    taskqueue.Task{ID: "task-blocked"},
				Message: "blocked by preflight",
			},
		},
		{
			name: "dirty verification failure",
			result: runonce.Result{
				Outcome:        runonce.OutcomeVerificationFailed,
				Run:            ledger.Run{ID: "run-dirty-failure"},
				Task:           taskqueue.Task{ID: "task-dirty-failure"},
				Message:        "verification command 0 failed",
				PostRunChanged: gitstate.Capture{ChangedFiles: []string{"internal/feature.go"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			passCalled := false

			result, err := RunLoop(ctx, Config{WorkDir: t.TempDir()}, RunLoopInput{
				MaxPasses: 5,
				Runner: func(context.Context, runonce.Config) (runonce.Result, error) {
					calls++
					return tc.result, nil
				},
				OnPass: func(runonce.Result) error {
					passCalled = true
					return nil
				},
			})
			if err == nil || err.Error() != fmt.Sprintf("run %s stopped with outcome %s", tc.result.Run.ID, tc.result.Outcome) {
				t.Fatalf("run loop error = %v, want outcome error", err)
			}
			if calls != 1 {
				t.Fatalf("run calls = %d, want 1", calls)
			}
			if !passCalled {
				t.Fatal("pass callback was not called")
			}
			wantStats := RunLoopStats{
				MaxPasses:                  5,
				Passes:                     1,
				FailedOrBlocked:            1,
				StopReason:                 "failed_or_blocked",
				ConsecutiveFailedOrBlocked: 1,
			}
			if !reflect.DeepEqual(result.Stats, wantStats) {
				t.Fatalf("loop stats = %#v, want %#v", result.Stats, wantStats)
			}
		})
	}
}

func TestRunLoopConfigErrorStopsBeforePass(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  typo: codex
`)

	called := false
	result, err := RunLoop(ctx, Config{WorkDir: workDir}, RunLoopInput{
		MaxPasses: 2,
		Runner: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("run loop error = %v, want unknown field error", err)
	}
	if called {
		t.Fatal("runner was called after invalid config")
	}
	wantStats := RunLoopStats{
		MaxPasses:  2,
		StopReason: "config_error",
	}
	if !reflect.DeepEqual(result.Stats, wantStats) {
		t.Fatalf("loop stats = %#v, want %#v", result.Stats, wantStats)
	}
}

func taskStatuses(tasks []taskqueue.Task) map[string]string {
	statuses := make(map[string]string, len(tasks))
	for _, task := range tasks {
		statuses[task.ID] = task.Status
	}
	return statuses
}

func taskIDs(tasks []taskqueue.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func taskTexts(tasks []taskqueue.Task) []string {
	texts := make([]string, 0, len(tasks))
	for _, task := range tasks {
		texts = append(texts, task.Task)
	}
	return texts
}

func taskSummaries(tasks []taskqueue.Task) []string {
	summaries := make([]string, 0, len(tasks))
	for _, task := range tasks {
		summaries = append(summaries, task.Summary)
	}
	return summaries
}

func importedTaskIDs(tasks []ImportedTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func importTaskSummaries(tasks []ImportedTask) []string {
	summaries := make([]string, 0, len(tasks))
	for _, task := range tasks {
		summaries = append(summaries, task.Summary)
	}
	return summaries
}

func assertNoAppStateDir(t *testing.T, workDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(workDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}
}

func runIDs(runs []ledger.Run) []string {
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func eventTypes(events []ledger.Event) []ledger.EventType {
	types := make([]ledger.EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

type appValidationRunSpec struct {
	RunID               string
	TaskID              string
	Task                string
	CompletedAt         time.Time
	CommitSHA           string
	ChangedFiles        []string
	VerificationStatus  string
	Verification        []receipt.VerificationEntry
	ReceiptTimestamp    time.Time
	ReceiptChangedFiles []string
	WriteArtifacts      bool
}

func createAppValidationRun(t *testing.T, workDir string, spec appValidationRunSpec) {
	t.Helper()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	ctx := context.Background()
	startedAt := spec.CompletedAt.Add(-time.Minute)
	exitCode := 0
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:                 spec.RunID,
		TaskID:             spec.TaskID,
		Task:               spec.Task,
		Status:             ledger.StatusCompleted,
		Summary:            "completed",
		StartedAt:          startedAt,
		CompletedAt:        &spec.CompletedAt,
		DurationSeconds:    60,
		CodexExitCode:      &exitCode,
		VerificationStatus: spec.VerificationStatus,
		CommitSHA:          spec.CommitSHA,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	artifacts := ledger.RunArtifacts{
		PromptPath:           filepath.Join(".revolvr", "runs", spec.RunID, "prompt.md"),
		CodexStdoutJSONLPath: filepath.Join(".revolvr", "runs", spec.RunID, "codex.jsonl"),
		CodexStderrPath:      filepath.Join(".revolvr", "runs", spec.RunID, "codex.stderr"),
		LastMessagePath:      filepath.Join(".revolvr", "runs", spec.RunID, "last-message.txt"),
		ReceiptPath:          filepath.Join(".revolvr", "receipts", spec.RunID+".md"),
	}
	if _, err := runs.AppendEvent(ctx, spec.RunID, ledger.EventRunArtifacts, artifacts); err != nil {
		t.Fatalf("append artifact event: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, spec.RunID, ledger.EventChangedFilesCaptured, map[string]any{
		"changed_files": spec.ChangedFiles,
	}); err != nil {
		t.Fatalf("append changed files event: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, spec.RunID, ledger.EventVerificationCompleted, map[string]any{
		"status":   spec.VerificationStatus,
		"passed":   spec.VerificationStatus == "passed",
		"commands": appValidationCommandPayloads(spec.Verification),
	}); err != nil {
		t.Fatalf("append verification event: %v", err)
	}
	if spec.CommitSHA != "" {
		if _, err := runs.AppendEvent(ctx, spec.RunID, ledger.EventCommitCreated, map[string]any{
			"commit_sha": spec.CommitSHA,
		}); err != nil {
			t.Fatalf("append commit event: %v", err)
		}
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	if spec.WriteArtifacts {
		writeAppTestFile(t, filepath.Join(workDir, artifacts.PromptPath), "prompt")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.CodexStdoutJSONLPath), "{}\n")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.CodexStderrPath), "")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.LastMessagePath), "done")
	}

	receiptTimestamp := spec.ReceiptTimestamp
	if receiptTimestamp.IsZero() {
		receiptTimestamp = spec.CompletedAt
	}
	receiptChangedFiles := spec.ReceiptChangedFiles
	if receiptChangedFiles == nil {
		receiptChangedFiles = spec.ChangedFiles
	}
	content, _ := receipt.FormatFallbackReceipt(receipt.FallbackInput{
		RunID:              spec.RunID,
		PassID:             spec.RunID,
		TaskID:             spec.TaskID,
		Task:               spec.Task,
		Verdict:            receipt.VerdictCompleted,
		Timestamp:          receiptTimestamp,
		CodexExitCode:      0,
		VerificationStatus: spec.VerificationStatus,
		CommitSHA:          spec.CommitSHA,
		ChangedFiles:       receiptChangedFiles,
		Verification:       spec.Verification,
		Metrics:            receipt.Metrics{},
		FinalText:          "completed",
	})
	writeAppTestFile(t, filepath.Join(workDir, artifacts.ReceiptPath), content)
}

func appValidationCommandPayloads(entries []receipt.VerificationEntry) []map[string]any {
	payloads := make([]map[string]any, 0, len(entries))
	for i, entry := range entries {
		payloads = append(payloads, map[string]any{
			"index":     i,
			"command":   entry.Command,
			"status":    entry.Status,
			"passed":    entry.Status == "passed",
			"exit_code": entry.ExitCode,
		})
	}
	return payloads
}

func writeAppTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func createAppPreflightState(t *testing.T, workDir string) {
	t.Helper()
	ctx := context.Background()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	tasks, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	if err := tasks.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}
}

func preflightLookPath(paths map[string]string) ExecutableLookPath {
	return func(name string) (string, error) {
		if path, ok := paths[name]; ok {
			return path, nil
		}
		return "", fmt.Errorf("executable %s not found", name)
	}
}

func readyPreflightCommandRunner(t *testing.T) PreflightCommandRunner {
	t.Helper()
	return func(_ context.Context, command runner.Command) runner.Result {
		switch strings.Join(command.Args, "\x00") {
		case "config\x00--get\x00user.name":
			return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
		case "config\x00--get\x00user.email":
			return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
		case "status\x00--short\x00--untracked-files=all":
			return runner.Result{ExitCode: 0}
		case "check-ignore\x00--quiet\x00.revolvr/":
			return runner.Result{ExitCode: 0}
		default:
			t.Fatalf("unexpected preflight command: %s %v", command.Name, command.Args)
			return runner.Result{ExitCode: 1}
		}
	}
}

func failedPreflightCommandRunner(t *testing.T) PreflightCommandRunner {
	t.Helper()
	return func(_ context.Context, command runner.Command) runner.Result {
		switch strings.Join(command.Args, "\x00") {
		case "config\x00--get\x00user.name", "config\x00--get\x00user.email":
			return runner.Result{ExitCode: 1}
		case "status\x00--short\x00--untracked-files=all":
			return runner.Result{ExitCode: 0, Stdout: " M internal/app/preflight.go\n?? scratch.txt\n"}
		case "check-ignore\x00--quiet\x00.revolvr/":
			return runner.Result{ExitCode: 1}
		default:
			t.Fatalf("unexpected preflight command: %s %v", command.Name, command.Args)
			return runner.Result{ExitCode: 1}
		}
	}
}

func validationCheckMessages(result receipt.ValidationResult) map[string]string {
	messages := make(map[string]string, len(result.Checks))
	for _, check := range result.Checks {
		messages[check.Name] = check.Message()
	}
	return messages
}

func failedValidationCheckNames(result receipt.ValidationResult) []string {
	failures := result.Failures()
	names := make([]string, 0, len(failures))
	for _, check := range failures {
		names = append(names, check.Name)
	}
	return names
}
