package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
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
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskscheduler"
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
	writeAppTaskFile(t, workDir, "010-pending.md", appTaskMarkdown("task-pending", "pending", "Pending File Task", "pending task"))
	writeAppTaskFile(t, workDir, "020-blocked.md", appTaskMarkdownWithPhase("task-blocked", "blocked", "Blocked File Task", "blocked task", taskfile.PhaseAudit))
	writeAppTaskFile(t, workDir, "030-completed.md", appTaskMarkdownWithPhase("task-completed", "completed", "Completed File Task", "completed task", taskfile.PhaseSimplify))
	writeAppTaskFile(t, workDir, "040-running.md", appTaskMarkdownWithPhase("task-running", "running", "Running File Task", "running task", taskfile.PhaseDocument))

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
		"task-pending":   taskmodel.StatusPending,
		"task-blocked":   taskmodel.StatusBlocked,
		"task-completed": taskmodel.StatusCompleted,
		"task-running":   taskfile.StatusRunning,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task statuses = %#v, want %#v", got, want)
	}
	if got, want := taskWorkflowStates(result.Tasks), []taskWorkflowState{
		{ID: "task-pending", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseImplement, RunProfile: "implementer", NextState: taskfile.PhaseAudit},
		{ID: "task-blocked", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseAudit, RunProfile: "auditor", NextState: taskfile.PhaseDocument},
		{ID: "task-completed", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseSimplify, RunProfile: "simplifier", NextState: taskmodel.StatusCompleted},
		{ID: "task-running", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseDocument, RunProfile: "documentor", NextState: taskfile.PhaseSimplify},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task workflow states = %#v, want %#v", got, want)
	}
	if got, want := runIDs(result.RecentRuns), []string{"run-new"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent runs = %#v, want %#v", got, want)
	}
	if got, want := eventTypes(result.LatestEvents), []ledger.EventType{ledger.EventRunStarted, ledger.EventRunFailed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("latest event types = %#v, want %#v", got, want)
	}
}

func TestReadProjectionsLeaveLiveLedgerImmutable(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	completedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	createAppValidationRun(t, workDir, appValidationRunSpec{
		RunID:              "run-read-only-projection",
		TaskID:             "task-read-only-projection",
		Task:               "Inspect immutable live evidence",
		CompletedAt:        completedAt,
		VerificationStatus: "passed",
		WriteArtifacts:     true,
	})
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	if err := os.Chmod(paths.LedgerDBPath, 0o444); err != nil {
		t.Fatalf("make ledger read-only: %v", err)
	}
	if err := os.Chmod(paths.StateDir, 0o555); err != nil {
		t.Fatalf("make ledger parent read-only: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(paths.StateDir, 0o755)
		_ = os.Chmod(paths.LedgerDBPath, 0o644)
	})

	operations := []struct {
		name string
		run  func() error
	}{
		{name: "status", run: func() error {
			_, err := Status(ctx, Config{WorkDir: workDir})
			return err
		}},
		{name: "show run", run: func() error {
			_, err := ShowRun(ctx, Config{WorkDir: workDir}, "run-read-only-projection")
			return err
		}},
		{name: "validate receipt", run: func() error {
			_, err := ValidateReceipt(ctx, Config{WorkDir: workDir}, "run-read-only-projection")
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			before := captureAppLedgerFilesystem(t, paths)
			if err := operation.run(); err != nil {
				t.Fatalf("read projection: %v", err)
			}
			after := captureAppLedgerFilesystem(t, paths)
			assertAppLedgerFilesystemUnchanged(t, before, after)
		})
	}
}

func TestReadProjectionsDoNotInitializeOrMigrateInvalidLedgers(t *testing.T) {
	ctx := context.Background()
	fixtures := []struct {
		name   string
		create func(*testing.T, string)
	}{
		{name: "empty", create: func(t *testing.T, path string) {
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatalf("create empty ledger: %v", err)
			}
		}},
		{name: "old schema", create: createOldAppLedger},
		{name: "malformed", create: func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("not a SQLite database\n"), 0o644); err != nil {
				t.Fatalf("create malformed ledger: %v", err)
			}
		}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			workDir := t.TempDir()
			paths, err := resolveStatePaths(workDir)
			if err != nil {
				t.Fatalf("resolve state paths: %v", err)
			}
			if err := os.MkdirAll(paths.StateDir, 0o755); err != nil {
				t.Fatalf("create state directory: %v", err)
			}
			fixture.create(t, paths.LedgerDBPath)

			operations := []struct {
				name string
				run  func() error
			}{
				{name: "status", run: func() error {
					_, err := Status(ctx, Config{WorkDir: workDir})
					return err
				}},
				{name: "show run", run: func() error {
					_, err := ShowRun(ctx, Config{WorkDir: workDir}, "missing-run")
					return err
				}},
				{name: "validate receipt", run: func() error {
					_, err := ValidateReceipt(ctx, Config{WorkDir: workDir}, "missing-run")
					return err
				}},
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					before := captureAppLedgerFilesystem(t, paths)
					if err := operation.run(); err == nil {
						t.Fatal("read projection error = nil, want invalid-ledger diagnostic")
					}
					after := captureAppLedgerFilesystem(t, paths)
					assertAppLedgerFilesystemUnchanged(t, before, after)
				})
			}
		})
	}
}

func TestListTasksReturnsFileBackedTasksInTaskfileOrder(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	first := appTaskMarkdown("task-first", "pending", "First File Task", "First body.")
	second := appTaskMarkdownWithPhase("task-second", "blocked", "Second File Task", "Second body.", taskfile.PhaseAudit)
	third := appTaskMarkdownWithPhase("task-third", "completed", "Third File Task", "Third body.", taskfile.PhaseSimplify)
	writeAppTaskFile(t, workDir, "020-second.md", second)
	writeAppTaskFile(t, workDir, "010-first.md", first)
	writeAppTaskFile(t, workDir, "030-third.md", third)

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}

	if got, want := taskIDs(tasks), []string{"task-first", "task-second", "task-third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task ids = %#v, want %#v", got, want)
	}
	if got, want := taskStatuses(tasks), map[string]string{
		"task-first":  taskmodel.StatusPending,
		"task-second": taskmodel.StatusBlocked,
		"task-third":  taskmodel.StatusCompleted,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task statuses = %#v, want %#v", got, want)
	}
	if got, want := taskSummaries(tasks), []string{"First File Task", "Second File Task", "Third File Task"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task summaries = %#v, want %#v", got, want)
	}
	if got, want := taskTexts(tasks), []string{first, second, third}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task text = %#v, want %#v", got, want)
	}
	if got, want := taskWorkflowStates(tasks), []taskWorkflowState{
		{ID: "task-first", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseImplement, RunProfile: "implementer", NextState: taskfile.PhaseAudit},
		{ID: "task-second", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseAudit, RunProfile: "auditor", NextState: taskfile.PhaseDocument},
		{ID: "task-third", Workflow: taskfile.WorkflowMixedPassV1, Phase: taskfile.PhaseSimplify, RunProfile: "simplifier", NextState: taskmodel.StatusCompleted},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task workflow states = %#v, want %#v", got, want)
	}
	for _, task := range tasks {
		if !task.CreatedAt.IsZero() || !task.UpdatedAt.IsZero() || task.Blocker != "" || task.BlockedAt != nil || task.CompletedAt != nil {
			t.Fatalf("file-backed task metadata = %+v, want zero timestamps and no blocker/completion metadata", task)
		}
	}
}

func TestListTasksMarksPrioritySelectedTaskWithoutReordering(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTaskFile(t, workDir, "010-first.md", `---
id: task-first
status: pending
priority: 50
---
# First By Filename
`)
	writeAppTaskFile(t, workDir, "020-priority.md", `---
id: task-priority
status: pending
priority: 1
phase: audit
---
# First By Priority
`)

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if got, want := taskIDs(tasks), []string{"task-first", "task-priority"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task order = %#v, want filename order %#v", got, want)
	}
	if tasks[0].NextRunnable || !tasks[1].NextRunnable {
		t.Fatalf("next-runnable flags = %v/%v, want false/true", tasks[0].NextRunnable, tasks[1].NextRunnable)
	}
	if tasks[0].ReadinessReason != "ready" || tasks[1].ReadinessReason != "ready" {
		t.Fatalf("readiness = %q/%q, want ready/ready", tasks[0].ReadinessReason, tasks[1].ReadinessReason)
	}
}

func TestListTasksAndStatusRejectUnsupportedCanonicalFrontmatter(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTaskFile(t, workDir, "typo.md", `---
id: typo-task
status: pending
depend_on: prerequisite
---
# Typo Task
`)
	want := `unsupported frontmatter key "depend_on" at .agent/tasks/typo.md:4`

	if _, err := ListTasks(ctx, Config{WorkDir: workDir}); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("list tasks error = %v, want %q", err, want)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger: %v", err)
	}
	if _, err := Status(ctx, Config{WorkDir: workDir}); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("status error = %v, want %q", err, want)
	}
}

func TestListTasksAndStatusProjectSharedDependencySelection(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".agent", "profiles", "implementer.md"), "Implement the selected task.\n")
	writeAppTaskFile(t, workDir, "010-dependent.md", `---
id: task-dependent
status: pending
priority: 1
depends_on: task-prerequisite
---
# Dependent
`)
	writeAppTaskFile(t, workDir, "020-prerequisite.md", `---
id: task-prerequisite
status: pending
priority: 50
---
# Prerequisite
`)

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if got, want := taskIDs(tasks), []string{"task-dependent", "task-prerequisite"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task order = %#v, want source order %#v", got, want)
	}
	if tasks[0].NextRunnable || tasks[0].Readiness != taskscheduler.ReasonWaitingDependency || !reflect.DeepEqual(tasks[0].WaitingDependencyIDs, []string{"task-prerequisite"}) {
		t.Fatalf("dependent projection = %+v, want exact waiting dependency and no selection", tasks[0])
	}
	if !tasks[1].NextRunnable || tasks[1].Readiness != taskscheduler.ReasonReady {
		t.Fatalf("prerequisite projection = %+v, want selected ready task", tasks[1])
	}

	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("initialize ledger: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close initialized ledger: %v", err)
	}
	status, err := Status(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	selected, found := status.Schedule.SelectedForWorkflow(taskscheduler.WorkflowMixedPassV1)
	if !found || selected.TaskID != "task-prerequisite" || !status.Tasks[1].NextRunnable {
		t.Fatalf("status selection found=%t selected=%+v tasks=%+v", found, selected, status.Tasks)
	}

	runs, err = ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open run ledger: %v", err)
	}
	defer runs.Close()
	codexCalls := 0
	runResult, err := runonce.Run(ctx, runonce.Config{
		WorkingDir:  workDir,
		LedgerStore: runs,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalls++
			return codexexec.Result{ExitCode: 1}, nil
		},
		CodexVersionDiscoverer: func(context.Context, codexexec.VersionConfig) (string, error) {
			return "codex-test 1.2.3", nil
		},
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if runResult.Task.ID != selected.TaskID || runResult.FileTask.ID != selected.TaskID || codexCalls != 1 {
		t.Fatalf("run selection task=%q file=%q codex_calls=%d, want status-selected %q", runResult.Task.ID, runResult.FileTask.ID, codexCalls, selected.TaskID)
	}
}

func TestListAndRetryAutonomousTaskWithoutMixedPassRouting(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTaskFile(t, workDir, "001-autonomous.md", `---
id: task-autonomous
status: pending
workflow: autonomous-v1
autonomous_state_path: .revolvr/autonomous/tasks/task-autonomous/state.json
priority: 1
x-custom: preserved
---
# Autonomous Task

Keep this specification.
`)
	writeAppTaskFile(t, workDir, "010-mixed.md", `---
id: task-mixed
status: pending
priority: 2
---
# Mixed Task
`)

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("listed tasks = %+v, want two", tasks)
	}
	if got := tasks[0]; got.ID != "task-autonomous" || got.Workflow != taskfile.WorkflowAutonomousV1 || got.Phase != "" || got.RunProfile != "" || got.NextState != "" || got.NextRunnable {
		t.Fatalf("autonomous projection = %+v, want lifecycle identity without mixed-pass routing", got)
	}
	if got := tasks[1]; got.ID != "task-mixed" || got.Workflow != taskfile.WorkflowMixedPassV1 || !got.NextRunnable {
		t.Fatalf("mixed-pass projection = %+v, want current runnable task", got)
	}

	path := filepath.Join(taskfile.TasksDir, "001-autonomous.md")
	if _, err := taskfile.UpdateStatus(workDir, path, taskfile.StatusBlocked); err != nil {
		t.Fatalf("block autonomous task: %v", err)
	}
	retried, err := RetryTask(ctx, Config{WorkDir: workDir}, "task-autonomous")
	if err != nil {
		t.Fatalf("retry autonomous task: %v", err)
	}
	if retried.Status != taskmodel.StatusPending || retried.Workflow != taskfile.WorkflowAutonomousV1 || retried.Phase != "" || retried.RunProfile != "" {
		t.Fatalf("retried autonomous projection = %+v", retried)
	}
	fileTask, err := taskfile.Load(workDir, path)
	if err != nil {
		t.Fatalf("load retried autonomous task: %v", err)
	}
	if got, want := fileTask.AutonomousStatePath, ".revolvr/autonomous/tasks/task-autonomous/state.json"; got != want {
		t.Fatalf("state path = %q, want %q", got, want)
	}
	if !strings.Contains(fileTask.ContextBody, "x-custom: preserved") || !strings.Contains(fileTask.ContextBody, "Keep this specification.") {
		t.Fatalf("retried task content = %q", fileTask.ContextBody)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("retry created autonomous runtime state: %v", err)
	}
}

func TestTaskFromFileTaskReportsPolicyLookupErrors(t *testing.T) {
	_, err := taskFromFileTask(taskfile.Task{
		ID:       "task-invalid-policy",
		Workflow: "future-workflow",
		Phase:    taskfile.PhaseImplement,
	})
	if err == nil || !strings.Contains(err.Error(), `resolve workflow state for task "task-invalid-policy": lookup pass policy: unsupported workflow "future-workflow"`) {
		t.Fatalf("task adaptation error = %v, want task and policy context", err)
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
		receipt.ValidationCheckVerdict:             "ok",
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
		CommandRunner:          readyPreflightCommandRunner(t),
		ExecutableInspector:    testPreflightExecutableInspector,
		CodexIdentityInspector: testPreflightCodexIdentityInspector,
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
	manifest, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	want := []PreflightCheck{
		{Status: PreflightOK, Name: "state", Detail: "initialized at " + filepath.Join(workDir, ".revolvr")},
		{Status: PreflightOK, Name: "config", Detail: "loaded " + filepath.Join(workDir, ".revolvr", "config.yaml")},
		{Status: PreflightOK, Name: "task graph", Detail: "mode=attended-task canonical_tasks=0 autonomous_tasks=0"},
		{Status: PreflightOK, Name: "platform", Detail: "mode=attended-task os=linux"},
		{Status: PreflightOK, Name: "operational bounds", Detail: "task_attempts=16 action_attempts=[audit=4,correct=4,document=4,implement=4,plan=4,simplify=4] elapsed=4h0m0s model_tokens=1000000 cycles_per_task=50 process_duration=30m0s output_bytes_per_stream=262144 retained_disk_bytes=1073741824 notification_attempts=0"},
		{Status: PreflightOK, Name: "git executable", Detail: "configured=\"git-test\" resolved=\"/fake/bin/git-test\" sha256=" + strings.Repeat("b", 64)},
		{Status: PreflightOK, Name: "repository shape", Detail: "operator-controlled non-bare Git worktree at " + workDir},
		{Status: PreflightOK, Name: "active submodules", Detail: "none"},
		{Status: PreflightOK, Name: "worktree clean", Detail: "no changes"},
		{Status: PreflightOK, Name: "verification commands", Detail: "1 command configured"},
		{Status: PreflightOK, Name: "autonomy safety", Detail: "mode=operator_attended; operator remains responsible for host, network, hooks, and credentials; worktree isolation is Git/source isolation only"},
		{Status: PreflightOK, Name: "autonomous queue", Detail: "schema=autonomous-queue-policy-v1 maximum_workers=1"},
		{Status: PreflightOK, Name: "artifact retention", Detail: "schema=revolvr-artifact-retention-policy-v1 mutation_enabled=false recent_runs=20"},
		{Status: PreflightOK, Name: "notification hooks", Detail: "disabled; no executable lookup, environment load, outbox write, or process start"},
		{Status: PreflightOK, Name: "codex executable", Detail: "configured=\"codex-test\" resolved=\"/fake/bin/codex-test\" sha256=" + manifest.Codex[0].SHA256},
		{Status: PreflightOK, Name: "codex model", Detail: "gpt-5.6-sol"},
		{Status: PreflightOK, Name: "codex reasoning effort", Detail: "xhigh"},
		{Status: PreflightOK, Name: "codex session", Detail: "ephemeral (ephemeral=true)"},
		{Status: PreflightOK, Name: "codex version", Detail: manifest.Codex[0].Version + " (release-authorized exact identity)"},
		{Status: PreflightOK, Name: "git identity", Detail: "Revolvr Doctor <doctor@example.invalid>"},
		{Status: PreflightOK, Name: "runtime state ignored", Detail: ".revolvr/ ignored by Git"},
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
		CommandRunner:          failedPreflightCommandRunner(t),
		ExecutableInspector:    testPreflightExecutableInspector,
		CodexIdentityInspector: testPreflightCodexIdentityInspector,
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
		{Status: PreflightOK, Name: "task graph", Detail: "mode=attended-task canonical_tasks=0 autonomous_tasks=0"},
		{Status: PreflightOK, Name: "platform", Detail: "mode=attended-task os=linux"},
		{Status: PreflightOK, Name: "operational bounds", Detail: "task_attempts=16 action_attempts=[audit=4,correct=4,document=4,implement=4,plan=4,simplify=4] elapsed=4h0m0s model_tokens=1000000 cycles_per_task=50 process_duration=30m0s output_bytes_per_stream=262144 retained_disk_bytes=1073741824 notification_attempts=0"},
		{Status: PreflightOK, Name: "git executable", Detail: "configured=\"git-test\" resolved=\"/fake/bin/git-test\" sha256=" + strings.Repeat("b", 64)},
		{Status: PreflightOK, Name: "repository shape", Detail: "operator-controlled non-bare Git worktree at " + workDir},
		{Status: PreflightOK, Name: "active submodules", Detail: "none"},
		{Status: PreflightFail, Name: "worktree clean", Detail: "dirty files: internal/app/preflight.go, scratch.txt"},
		{Status: PreflightFail, Name: "verification commands", Detail: "no verification commands configured"},
		{Status: PreflightOK, Name: "autonomy safety", Detail: "mode=operator_attended; operator remains responsible for host, network, hooks, and credentials; worktree isolation is Git/source isolation only"},
		{Status: PreflightOK, Name: "autonomous queue", Detail: "schema=autonomous-queue-policy-v1 maximum_workers=1"},
		{Status: PreflightOK, Name: "artifact retention", Detail: "schema=revolvr-artifact-retention-policy-v1 mutation_enabled=false recent_runs=20"},
		{Status: PreflightOK, Name: "notification hooks", Detail: "disabled; no executable lookup, environment load, outbox write, or process start"},
		{Status: PreflightFail, Name: "codex executable", Detail: `"missing-codex" not found: executable missing-codex not found`},
		{Status: PreflightOK, Name: "codex model", Detail: "gpt-5.6-sol"},
		{Status: PreflightOK, Name: "codex reasoning effort", Detail: "xhigh"},
		{Status: PreflightOK, Name: "codex session", Detail: "ephemeral (ephemeral=true)"},
		{Status: PreflightFail, Name: "codex version", Detail: `"missing-codex" not found: executable missing-codex not found`},
		{Status: PreflightFail, Name: "git identity", Detail: "missing user.name and user.email"},
		{Status: PreflightFail, Name: "runtime state ignored", Detail: ".revolvr/ is not ignored; run `revolvr init`"},
	}
	if !reflect.DeepEqual(result.Checks, want) {
		t.Fatalf("preflight checks = %#v, want %#v", result.Checks, want)
	}
}

func TestPreflightWorktreeCheckFailsClosedOnTruncatedStatus(t *testing.T) {
	var checks []PreflightCheck
	addCheck := func(status PreflightCheckStatus, name, detail string) {
		checks = append(checks, PreflightCheck{Status: status, Name: name, Detail: detail})
	}
	addWorktreeCheck(
		context.Background(),
		addCheck,
		func(context.Context, runner.Command) runner.Result {
			return runner.Result{ExitCode: 0, Stdout: " M partial.go\x00", StdoutTruncatedBytes: 5}
		},
		t.TempDir(),
		"git-test",
		time.Second,
		1024,
		1024,
	)
	if len(checks) != 1 || checks[0].Status != PreflightFail || checks[0].Name != "worktree clean" || !strings.Contains(checks[0].Detail, "truncated") {
		t.Fatalf("worktree checks = %+v, want truncation failure", checks)
	}
}

func TestPreflightDoesNotClaimFullyUnattendedReadinessWithoutTaskWorkspace(t *testing.T) {
	workDir := t.TempDir()
	createAppPreflightState(t, workDir)
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
autonomy:
  mode: fully_unattended
  external_isolation:
    expectation: container
    enforcement: external_attestation
    attestation: {authority: operator, evidence: container, sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}
  network:
    access: denied
    enforcement: external_attestation
    attestation: {authority: operator, evidence: firewall, sha256: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb}
  hooks: {policy: disabled}
  environment: {inherit_host: false}
  acknowledgement: revolvr-fully-unattended-v1:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
verification:
  commands: [{name: go}]
`)
	result, err := Preflight(context.Background(), Config{WorkDir: workDir}, PreflightInput{CommandRunner: readyPreflightCommandRunner(t), LookPath: preflightLookPath(map[string]string{"codex": "/fake/bin/codex", "git": "/fake/bin/git"}), ExecutableInspector: testPreflightExecutableInspector, CodexIdentityInspector: testPreflightCodexIdentityInspector})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready {
		t.Fatal("fully unattended doctor was ready without an admitted task workspace")
	}
	found := false
	for _, check := range result.Checks {
		if check.Name == "autonomy safety" && check.Status == PreflightFail && strings.Contains(check.Detail, "task/workspace-bound") {
			found = true
		}
	}
	if !found {
		t.Fatalf("checks = %+v", result.Checks)
	}
}

func TestPreflightCodexVersionFailuresAreNotReady(t *testing.T) {
	tests := []struct {
		name   string
		result runner.Result
		want   string
	}{
		{name: "timeout", result: runner.Result{TimedOut: true, Err: context.DeadlineExceeded}, want: "timed out"},
		{name: "execution", result: runner.Result{ExitCode: -1, Err: errors.New("start failed")}, want: "execution failed"},
		{name: "nonzero", result: runner.Result{ExitCode: 2, Stderr: "unsupported\n"}, want: "exited with code 2"},
		{name: "truncated", result: runner.Result{ExitCode: 0, Stdout: "codex-test", StdoutTruncatedBytes: 1}, want: "output was truncated"},
		{name: "malformed", result: runner.Result{ExitCode: 0, Stdout: "first\nsecond\n"}, want: "one well-formed line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			createAppPreflightState(t, workDir)
			writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), "verification:\n  commands:\n    - name: go\n")
			result, err := Preflight(context.Background(), Config{WorkDir: workDir}, PreflightInput{
				CommandRunner:          preflightCommandRunnerWithVersion(t, tt.result),
				ExecutableInspector:    testPreflightExecutableInspector,
				CodexIdentityInspector: testPreflightCodexIdentityInspector,
				LookPath: preflightLookPath(map[string]string{
					"codex": "/fake/bin/codex",
					"git":   "/fake/bin/git",
				}),
			})
			if err != nil {
				t.Fatalf("preflight: %v", err)
			}
			if result.Ready {
				t.Fatal("preflight ready = true, want false")
			}
			var detail string
			for _, check := range result.Checks {
				if check.Name == "codex version" {
					detail = check.Detail
				}
			}
			if !strings.Contains(detail, tt.want) {
				t.Fatalf("codex version detail = %q, want %q", detail, tt.want)
			}
		})
	}
}

func TestPreflightInvalidCodexConfigStopsBeforeCommands(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		want   string
	}{
		{name: "effort", config: "codex:\n  reasoning_effort: extreme\n", want: "invalid Codex reasoning effort"},
		{name: "session", config: "codex:\n  ephemeral: false\n", want: "persistent or resumed sessions are not supported"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workDir := t.TempDir()
			writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), tt.config)
			called := false
			result, err := Preflight(context.Background(), Config{WorkDir: workDir}, PreflightInput{
				CommandRunner: func(context.Context, runner.Command) runner.Result {
					called = true
					return runner.Result{}
				},
			})
			if err != nil {
				t.Fatalf("preflight: %v", err)
			}
			if result.Ready || called {
				t.Fatalf("ready=%v command_called=%v, want false/false", result.Ready, called)
			}
			if len(result.Checks) != 2 || result.Checks[1].Name != "config" || !strings.Contains(result.Checks[1].Detail, tt.want) {
				t.Fatalf("checks = %#v", result.Checks)
			}
		})
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
	wantPending := appTaskMarkdown(task.ID, "pending", "app boundary", "Implement app task operations")
	if got, want := task.Task, wantPending; got != want {
		t.Fatalf("task text = %q, want %q", got, want)
	}
	if got, want := task.Summary, "app boundary"; got != want {
		t.Fatalf("task summary = %q, want %q", got, want)
	}
	if got, want := task.Status, taskmodel.StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}

	tasks, err := ListTasks(ctx, Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks after add: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("listed tasks = %+v, want added task %s", tasks, task.ID)
	}
	if got, want := tasks[0].Task, wantPending; got != want {
		t.Fatalf("listed task text = %q, want %q", got, want)
	}

	fileTask, ok, err := taskfile.FindByID(workDir, task.ID)
	if err != nil {
		t.Fatalf("find added task file: %v", err)
	}
	if !ok {
		t.Fatalf("added task file %s not found", task.ID)
	}
	if fileTask.Workflow != taskfile.WorkflowMixedPassV1 || fileTask.Phase != taskfile.PhaseImplement || fileTask.AutonomousStatePath != "" {
		t.Fatalf("added task lifecycle = %+v, want default mixed-pass implement task", fileTask)
	}
	if _, err := taskfile.UpdateStatus(workDir, fileTask.SourcePath, taskfile.StatusBlocked); err != nil {
		t.Fatalf("block task file: %v", err)
	}

	retried, err := RetryTask(ctx, Config{WorkDir: workDir}, " "+task.ID+" ")
	if err != nil {
		t.Fatalf("retry task: %v", err)
	}
	wantRetried := appTaskMarkdown(task.ID, "pending", "app boundary", "Implement app task operations")
	if retried.ID != task.ID || retried.Task != wantRetried || retried.Summary != task.Summary {
		t.Fatalf("retried task = %+v, want same identity and pending markdown %q", retried, wantRetried)
	}
	if retried.Status != taskmodel.StatusPending || retried.Blocker != "" || retried.BlockedAt != nil {
		t.Fatalf("retried task = %+v, want pending with blocker state cleared", retried)
	}
}

func TestRetryTaskPreservesPhaseIdentityBodyAndMetadata(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTaskFile(t, workDir, "retry-audit.md", `---
id: retry-audit
status: blocked
workflow: mixed-pass-v1
phase: audit
profile: ignored-frontmatter-profile
priority: 7
x-custom: preserved
---
# Retry Audit

Keep this body byte-for-byte.
`)
	path := filepath.Join(taskfile.TasksDir, "retry-audit.md")

	retried, err := RetryTask(ctx, Config{WorkDir: workDir}, "retry-audit")
	if err != nil {
		t.Fatalf("retry task: %v", err)
	}
	if retried.ID != "retry-audit" || retried.Status != taskmodel.StatusPending || retried.Phase != taskfile.PhaseAudit || retried.RunProfile != "auditor" {
		t.Fatalf("retried task = %+v, want same audit task pending under auditor policy", retried)
	}
	want := `---
id: retry-audit
status: pending
workflow: mixed-pass-v1
phase: audit
profile: ignored-frontmatter-profile
priority: 7
x-custom: preserved
---
# Retry Audit

Keep this body byte-for-byte.
`
	content, readErr := os.ReadFile(filepath.Join(workDir, path))
	if readErr != nil {
		t.Fatalf("read retried task: %v", readErr)
	}
	if got := string(content); got != want {
		t.Fatalf("retried task content = %q, want only status changed to %q", got, want)
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
	fileTask, ok, err := taskfile.FindByID(workDir, task.ID)
	if err != nil {
		t.Fatalf("find completed task file: %v", err)
	}
	if !ok {
		t.Fatalf("completed task file %s not found", task.ID)
	}
	if _, err := taskfile.UpdateStatus(workDir, fileTask.SourcePath, taskfile.StatusCompleted); err != nil {
		t.Fatalf("complete task file: %v", err)
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
	assertNoAppTasksDir(t, workDir)
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
		t.Fatalf("list imported tasks: %v", err)
	}
	if got, want := taskIDs(tasks), importedTaskIDs(result.Tasks); !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted task ids = %#v, want result ids %#v", got, want)
	}
	if got, want := taskSummaries(tasks), []string{"First", "Second", "Third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted summaries = %#v, want %#v", got, want)
	}
	wantTexts := []string{
		appTaskMarkdown(result.Tasks[0].ID, "pending", "First", "Create first."),
		appTaskMarkdown(result.Tasks[1].ID, "pending", "Second", "Create second."),
		appTaskMarkdown(result.Tasks[2].ID, "pending", "Third", "Create third."),
	}
	if got, want := taskTexts(tasks), wantTexts; !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted task text = %#v, want %#v", got, want)
	}
	for _, task := range tasks {
		if task.Workflow != taskfile.WorkflowMixedPassV1 || task.Phase != taskfile.PhaseImplement || task.RunProfile != "implementer" {
			t.Fatalf("imported task lifecycle = %+v, want default mixed-pass implement task", task)
		}
	}
}

func TestTaskImportPreservesSchedulingMetadata(t *testing.T) {
	workDir := t.TempDir()
	result, err := ImportTasksFromMarkdown(context.Background(), Config{WorkDir: workDir}, ImportTasksFromMarkdownInput{Markdown: []byte("## Task: Scheduled\nDo it.\n\n### Depends On\n- upstream\n\n### Tags\n- api\n\n### Conflicts\n- shared-db\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 {
		t.Fatalf("tasks=%d", len(result.Tasks))
	}
	loaded, found, err := taskfile.FindByID(workDir, result.Tasks[0].ID)
	if err != nil || !found {
		t.Fatalf("load=%v %v", found, err)
	}
	if strings.Join(loaded.DependsOn, ",") != "upstream" || strings.Join(loaded.Tags, ",") != "api" || strings.Join(loaded.Conflicts, ",") != "shared-db" {
		t.Fatalf("loaded=%#v", loaded)
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
	assertNoAppTasksDir(t, workDir)
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
	assertNoAppTasksDir(t, workDir)
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
	assertNoAppTasksDir(t, workDir)
}

func TestRunOnceLoadsConfigAndProgressCallback(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-custom
  model: gpt-custom
  reasoning_effort: high
  ephemeral: true
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
  allow_pre_existing_dirty: false
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
				Task:    taskmodel.Task{ID: "task-config"},
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
	if got.CodexExecutable != "codex-custom" || got.CodexModel != "gpt-custom" || got.CodexReasoningEffort != "high" || !got.CodexEphemeral || got.CodexSandbox != "danger-full-access" || got.CodexApprovalPolicy != "on-request" || got.CodexBypassApprovalsAndSandbox || got.CodexTimeout != 45*time.Second {
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
	if got.AllowPreExistingDirty || !got.AllowMissingVerification || got.CommitTimeout != 30*time.Second {
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

func TestRunOnceRejectsPreExistingDirtyOptionBeforeRunner(t *testing.T) {
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
commit:
  allow_pre_existing_dirty: true
`)
	called := false
	_, err := RunOnce(context.Background(), Config{WorkDir: workDir}, RunOnceInput{
		Runner: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "allow_pre_existing_dirty must be false") || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("RunOnce error = %v, want removed dirty-worktree option error", err)
	}
	if called {
		t.Fatal("runner was called after removed dirty-worktree option")
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
				Task:    taskmodel.Task{ID: "task-failed"},
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
				Task:    taskmodel.Task{ID: "task-blocked"},
				Message: "blocked by preflight",
			},
		},
		{
			name: "dirty verification failure",
			result: runonce.Result{
				Outcome:        runonce.OutcomeVerificationFailed,
				Run:            ledger.Run{ID: "run-dirty-failure"},
				Task:           taskmodel.Task{ID: "task-dirty-failure"},
				Message:        "verification command 0 failed",
				PostRunChanged: gitstate.Capture{ChangedFiles: []string{"internal/feature.go"}},
			},
		},
		{
			name: "durably blocked failed task",
			result: runonce.Result{
				Outcome: runonce.OutcomeVerificationFailed,
				Run:     ledger.Run{ID: "run-blocked-task"},
				Task:    taskmodel.Task{ID: "task-blocked", Status: taskmodel.StatusBlocked},
				Message: "verification failed without product changes",
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

func TestRunLoopClassifiesRunnerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	result, err := RunLoop(ctx, Config{WorkDir: t.TempDir()}, RunLoopInput{
		MaxPasses: 3,
		Runner: func(ctx context.Context, _ runonce.Config) (runonce.Result, error) {
			cancel()
			return runonce.Result{}, ctx.Err()
		},
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("run loop error = %v, want context canceled", err)
	}
	wantStats := RunLoopStats{
		MaxPasses:                  3,
		Passes:                     1,
		FailedOrBlocked:            1,
		StopReason:                 "context_cancelled",
		ConsecutiveFailedOrBlocked: 1,
	}
	if !reflect.DeepEqual(result.Stats, wantStats) {
		t.Fatalf("loop stats = %#v, want %#v", result.Stats, wantStats)
	}
}

func TestRunLoopReevaluatesSharedScheduleAfterPrerequisiteCompletion(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeAppTestFile(t, filepath.Join(workDir, ".agent", "profiles", "simplifier.md"), "Complete the final mixed-pass phase.\n")
	writeAppTaskFile(t, workDir, "010-first.md", `---
id: task-first
status: pending
workflow: mixed-pass-v1
phase: simplify
priority: 50
---
# First

Complete the prerequisite.
`)
	writeAppTaskFile(t, workDir, "020-second.md", `---
id: task-second
status: pending
workflow: mixed-pass-v1
phase: simplify
priority: 1
depends_on: task-first
---
# Second

Run only after the prerequisite.
`)

	runnerCalls := 0
	selected := []string{}
	result, err := RunLoop(ctx, Config{WorkDir: workDir}, RunLoopInput{
		MaxPasses: 5,
		Runner: func(runCtx context.Context, cfg runonce.Config) (runonce.Result, error) {
			runnerCalls++
			selectedPath := ""
			switch runnerCalls {
			case 1:
				selectedPath = filepath.ToSlash(filepath.Join(taskfile.TasksDir, "010-first.md"))
			case 2:
				selectedPath = filepath.ToSlash(filepath.Join(taskfile.TasksDir, "020-second.md"))
			}
			captureCalls := 0
			cfg.DirtyCapture = func(context.Context, gitstate.Config) (gitstate.Capture, error) {
				return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
			}
			cfg.ChangedCapture = func(context.Context, gitstate.Config) (gitstate.Capture, error) {
				captureCalls++
				paths := []string{"internal/feature.go"}
				if captureCalls == 2 {
					paths = append(paths, selectedPath)
				}
				return gitstate.Capture{Kind: gitstate.CaptureKindChanged, ChangedFiles: append([]string(nil), paths...), Paths: paths}, nil
			}
			cfg.CodexRunner = func(context.Context, codexexec.Config) (codexexec.Result, error) {
				return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
			}
			cfg.CodexVersionDiscoverer = func(context.Context, codexexec.VersionConfig) (string, error) {
				return "codex-test 1.2.3", nil
			}
			cfg.VerificationRunner = func(context.Context, verification.Config) (verification.Result, error) {
				return verification.Result{Status: verification.StatusPassed, Passed: true, FailedCommandIndex: -1, Commands: []verification.CommandResult{{Command: "go test ./...", Status: verification.StatusPassed, Passed: true}}}, nil
			}
			cfg.CommitRunner = func(context.Context, commit.Config) (commit.Result, error) {
				return commit.Result{Status: commit.StatusCommitted, CommitSHA: fmt.Sprintf("commit-%d", runnerCalls), ChangedFiles: []string{"internal/feature.go", selectedPath}}, nil
			}
			runResult, runErr := runonce.Run(runCtx, cfg)
			if runResult.Task.ID != "" {
				selected = append(selected, runResult.Task.ID)
			}
			return runResult, runErr
		},
	})
	if err != nil {
		t.Fatalf("run loop: %v", err)
	}
	if got, want := selected, []string{"task-first", "task-second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected tasks = %#v, want %#v", got, want)
	}
	if runnerCalls != 3 || result.Stats.Passes != 3 || result.Stats.Completed != 2 || !result.Stats.NoTask || result.Stats.StopReason != "no_task" {
		t.Fatalf("loop result=%+v runner calls=%d", result, runnerCalls)
	}
	for _, name := range []string{"010-first.md", "020-second.md"} {
		task, loadErr := taskfile.Load(workDir, filepath.Join(taskfile.TasksDir, name))
		if loadErr != nil || task.Status != taskfile.StatusCompleted {
			t.Fatalf("task %s status=%q error=%v", name, task.Status, loadErr)
		}
	}
}

func taskStatuses(tasks []taskmodel.Task) map[string]string {
	statuses := make(map[string]string, len(tasks))
	for _, task := range tasks {
		statuses[task.ID] = task.Status
	}
	return statuses
}

func taskIDs(tasks []taskmodel.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func taskTexts(tasks []taskmodel.Task) []string {
	texts := make([]string, 0, len(tasks))
	for _, task := range tasks {
		texts = append(texts, task.Task)
	}
	return texts
}

func taskSummaries(tasks []taskmodel.Task) []string {
	summaries := make([]string, 0, len(tasks))
	for _, task := range tasks {
		summaries = append(summaries, task.Summary)
	}
	return summaries
}

type taskWorkflowState struct {
	ID         string
	Workflow   string
	Phase      string
	RunProfile string
	NextState  string
}

func taskWorkflowStates(tasks []taskmodel.Task) []taskWorkflowState {
	states := make([]taskWorkflowState, 0, len(tasks))
	for _, task := range tasks {
		states = append(states, taskWorkflowState{
			ID:         task.ID,
			Workflow:   task.Workflow,
			Phase:      task.Phase,
			RunProfile: task.RunProfile,
			NextState:  task.NextState,
		})
	}
	return states
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

func assertNoAppTasksDir(t *testing.T, workDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(workDir, taskfile.TasksDir)); !os.IsNotExist(err) {
		t.Fatalf("task dir stat err = %v, want not exist", err)
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
		ContextPayloadPath:   filepath.Join(".revolvr", "runs", spec.RunID, "context.md"),
		ContextManifestPath:  filepath.Join(".revolvr", "runs", spec.RunID, "context.json"),
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
		writeAppTestFile(t, filepath.Join(workDir, artifacts.ContextPayloadPath), "context payload")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.ContextManifestPath), "{}\n")
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

func createOldAppLedger(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open old-schema ledger: %v", err)
	}
	if _, err := db.Exec(`
CREATE TABLE runs (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	task TEXT NOT NULL,
	status TEXT NOT NULL,
	started_at TEXT NOT NULL
);
CREATE TABLE events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL,
	event_type TEXT NOT NULL,
	created_at TEXT NOT NULL
);`); err != nil {
		_ = db.Close()
		t.Fatalf("create old-schema ledger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close old-schema ledger: %v", err)
	}
}

type appLedgerFileSnapshot struct {
	Exists bool
	Mode   os.FileMode
	Size   int64
	MTime  time.Time
	SHA256 string
}

type appLedgerFilesystemSnapshot struct {
	Entries []string
	Files   map[string]appLedgerFileSnapshot
}

func captureAppLedgerFilesystem(t *testing.T, paths statePaths) appLedgerFilesystemSnapshot {
	t.Helper()
	entries, err := os.ReadDir(paths.StateDir)
	if err != nil {
		t.Fatalf("read state directory: %v", err)
	}
	snapshot := appLedgerFilesystemSnapshot{
		Entries: make([]string, 0, len(entries)),
		Files:   make(map[string]appLedgerFileSnapshot, 4),
	}
	for _, entry := range entries {
		snapshot.Entries = append(snapshot.Entries, entry.Name())
	}
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		path := paths.LedgerDBPath + suffix
		name := filepath.Base(path)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshot.Files[name] = appLedgerFileSnapshot{}
			continue
		}
		if err != nil {
			t.Fatalf("inspect %s: %v", name, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		digest := sha256.Sum256(raw)
		snapshot.Files[name] = appLedgerFileSnapshot{
			Exists: true,
			Mode:   info.Mode(),
			Size:   info.Size(),
			MTime:  info.ModTime(),
			SHA256: fmt.Sprintf("%x", digest),
		}
	}
	return snapshot
}

func assertAppLedgerFilesystemUnchanged(t *testing.T, before, after appLedgerFilesystemSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("read projection mutated ledger filesystem\nbefore=%#v\nafter=%#v", before, after)
	}
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

func writeAppTaskFile(t *testing.T, workDir string, name string, content string) {
	t.Helper()
	writeAppTestFile(t, filepath.Join(workDir, taskfile.TasksDir, name), content)
}

func appTaskMarkdown(id string, status string, title string, body string) string {
	return fmt.Sprintf(`---
id: %s
status: %s
---
# %s

%s
`, id, status, title, body)
}

func appTaskMarkdownWithPhase(id string, status string, title string, body string, phase string) string {
	return fmt.Sprintf(`---
id: %s
status: %s
workflow: %s
phase: %s
---
# %s

%s
`, id, status, taskfile.WorkflowMixedPassV1, phase, title, body)
}

func createAppPreflightState(t *testing.T, workDir string) {
	t.Helper()
	ctx := context.Background()
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
	manifest, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	return preflightCommandRunnerWithVersion(t, runner.Result{ExitCode: 0, Stdout: manifest.Codex[0].Version + "\n"})
}

func testPreflightExecutableInspector(configured string, lookPath codexexec.ExecutableLookPath) (codexexec.ExecutableIdentity, error) {
	path, err := lookPath(configured)
	if err != nil {
		return codexexec.ExecutableIdentity{}, fmt.Errorf("%q not found: %w", configured, err)
	}
	return codexexec.ExecutableIdentity{Configured: configured, Resolved: path, SHA256: strings.Repeat("b", 64)}, nil
}

func testPreflightCodexIdentityInspector(ctx context.Context, configured, workDir string, cfg codexexec.VersionConfig, lookPath codexexec.ExecutableLookPath) (codexexec.CodexExecutableIdentity, error) {
	path, err := lookPath(configured)
	if err != nil {
		return codexexec.CodexExecutableIdentity{}, fmt.Errorf("%q not found: %w", configured, err)
	}
	manifest, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		return codexexec.CodexExecutableIdentity{}, err
	}
	cfg.Executable = path
	cfg.WorkingDir = workDir
	version, err := codexexec.DiscoverVersion(ctx, cfg)
	if err != nil {
		return codexexec.CodexExecutableIdentity{}, err
	}
	identity := codexexec.CodexExecutableIdentity{Version: version, Executable: codexexec.ExecutableIdentity{Configured: configured, Resolved: path, SHA256: manifest.Codex[0].SHA256}}
	if err := manifest.Authorize(identity); err != nil {
		return codexexec.CodexExecutableIdentity{}, err
	}
	return identity, nil
}

func preflightCommandRunnerWithVersion(t *testing.T, version runner.Result) PreflightCommandRunner {
	t.Helper()
	return func(_ context.Context, command runner.Command) runner.Result {
		if reflect.DeepEqual(command.Args, []string{"--version"}) {
			return version
		}
		switch strings.Join(command.Args, "\x00") {
		case "rev-parse\x00--is-bare-repository":
			return runner.Result{ExitCode: 0, Stdout: "false\n"}
		case "rev-parse\x00--show-toplevel":
			return runner.Result{ExitCode: 0, Stdout: command.Dir + "\n"}
		case "submodule\x00status\x00--recursive":
			return runner.Result{ExitCode: 0}
		case "config\x00--get\x00user.name":
			return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
		case "config\x00--get\x00user.email":
			return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
		case "status\x00--porcelain=v1\x00-z\x00--untracked-files=all":
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
		case "rev-parse\x00--is-bare-repository":
			return runner.Result{ExitCode: 0, Stdout: "false\n"}
		case "rev-parse\x00--show-toplevel":
			return runner.Result{ExitCode: 0, Stdout: command.Dir + "\n"}
		case "submodule\x00status\x00--recursive":
			return runner.Result{ExitCode: 0}
		case "config\x00--get\x00user.name", "config\x00--get\x00user.email":
			return runner.Result{ExitCode: 1}
		case "status\x00--porcelain=v1\x00-z\x00--untracked-files=all":
			return runner.Result{ExitCode: 0, Stdout: " M internal/app/preflight.go\x00?? scratch.txt\x00"}
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
