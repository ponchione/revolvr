package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"revolvr/internal/commit"
	"revolvr/internal/ledger"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
)

func TestNewRootCommandConstructsExpectedCommands(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})

	for _, args := range [][]string{
		{"init"},
		{"task"},
		{"task", "add"},
		{"task", "list"},
		{"run"},
		{"status"},
		{"show"},
	} {
		cmd, remaining, err := root.Find(args)
		if err != nil {
			t.Fatalf("find %q: %v", strings.Join(args, " "), err)
		}
		if len(remaining) != 0 {
			t.Fatalf("find %q left remaining args %v", strings.Join(args, " "), remaining)
		}
		if cmd == root {
			t.Fatalf("find %q returned root command", strings.Join(args, " "))
		}
	}
}

func TestRootHelpWorks(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out})
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}

	help := out.String()
	for _, want := range []string{"Run bounded Codex harness passes", "init", "task", "run", "status", "show"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestVersionOutputWorks(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test-version", Out: &out})
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}

	if got, want := out.String(), "revolvr test-version\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestPlaceholderCommandOutput(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out})
	root.SetArgs([]string{"task"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute task: %v", err)
	}

	if got, want := out.String(), "revolvr task is not implemented yet.\n"; got != want {
		t.Fatalf("placeholder output = %q, want %q", got, want)
	}
}

func TestStatusUninitializedSucceedsWithoutCreatingState(t *testing.T) {
	workDir := t.TempDir()
	out, err := executeCLI(t, workDir, "status")
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if got, want := out, "Not initialized. Run `revolvr init` first.\n"; got != want {
		t.Fatalf("status output = %q, want %q", got, want)
	}

	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	if _, err := os.Stat(paths.StateDir); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}
}

func TestStatusEmptyInitializedState(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	out, err := executeCLI(t, workDir, "status")
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	want := "Total tasks: 0\n" +
		"Pending tasks: 0\n" +
		"Blocked tasks: 0\n" +
		"Completed tasks: 0\n" +
		"Recent runs: 0\n" +
		"Latest run: none\n"
	if out != want {
		t.Fatalf("status output = %q, want %q", out, want)
	}
}

func TestStatusShowsTaskCountsAndRecentRuns(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	base := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
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

	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
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
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	out, err := executeCLI(t, workDir, "status")
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	want := "Total tasks: 3\n" +
		"Pending tasks: 1\n" +
		"Blocked tasks: 1\n" +
		"Completed tasks: 1\n" +
		"Recent runs: 2\n" +
		"Latest run: run-new (failed)\n"
	if out != want {
		t.Fatalf("status output = %q, want %q", out, want)
	}
}

func TestInitCreatesStoresAndIsIdempotent(t *testing.T) {
	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	firstOut, err := executeCLI(t, workDir, "init")
	if err != nil {
		t.Fatalf("execute init: %v", err)
	}
	for _, want := range []string{
		"Initialized revolvr state:",
		paths.StateDir,
		paths.TaskDBPath,
		paths.LedgerDBPath,
		paths.RunsDir,
		paths.ReceiptsDir,
		paths.LocksDir,
	} {
		if !strings.Contains(firstOut, want) {
			t.Fatalf("init output missing %q:\n%s", want, firstOut)
		}
	}
	for _, dir := range []string{paths.StateDir, paths.RunsDir, paths.ReceiptsDir, paths.LocksDir} {
		assertDirExists(t, dir)
	}
	assertTaskStoreOpens(t, paths.TaskDBPath)
	assertLedgerStoreOpens(t, paths.LedgerDBPath)

	secondOut, err := executeCLI(t, workDir, "init")
	if err != nil {
		t.Fatalf("execute init second time: %v", err)
	}
	if secondOut != firstOut {
		t.Fatalf("second init output = %q, want %q", secondOut, firstOut)
	}
	assertTaskStoreOpens(t, paths.TaskDBPath)
	assertLedgerStoreOpens(t, paths.LedgerDBPath)
}

func TestTaskAddPersistsTask(t *testing.T) {
	workDir := t.TempDir()
	out, err := executeCLI(t, workDir, "task", "add", "Implement the CLI slice")
	if err != nil {
		t.Fatalf("execute task add: %v", err)
	}
	if !strings.Contains(out, "Added task ") || !strings.Contains(out, "Implement the CLI slice") {
		t.Fatalf("task add output = %q, want confirmation", out)
	}

	tasks := readTasks(t, workDir)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	task := tasks[0]
	if task.ID == "" {
		t.Fatal("task id is empty")
	}
	if !strings.Contains(out, task.ID) {
		t.Fatalf("task add output %q missing created id %q", out, task.ID)
	}
	if got, want := task.Status, taskqueue.StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	if got, want := task.Task, "Implement the CLI slice"; got != want {
		t.Fatalf("task text = %q, want %q", got, want)
	}
	if got := task.Summary; got != "" {
		t.Fatalf("task summary = %q, want empty", got)
	}
}

func TestTaskAddSummaryPersistsSummary(t *testing.T) {
	workDir := t.TempDir()
	out, err := executeCLI(t, workDir, "task", "add", "--summary", "CLI bootstrap", "Implement init and task commands")
	if err != nil {
		t.Fatalf("execute task add --summary: %v", err)
	}
	if !strings.Contains(out, "summary: CLI bootstrap") {
		t.Fatalf("task add output = %q, want summary confirmation", out)
	}

	tasks := readTasks(t, workDir)
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if got, want := tasks[0].Task, "Implement init and task commands"; got != want {
		t.Fatalf("task text = %q, want %q", got, want)
	}
	if got, want := tasks[0].Summary, "CLI bootstrap"; got != want {
		t.Fatalf("task summary = %q, want %q", got, want)
	}
}

func TestTaskAddRejectsEmptyInput(t *testing.T) {
	_, err := executeCLI(t, t.TempDir(), "task", "add", "   ")
	if err == nil {
		t.Fatal("task add empty input succeeded, want error")
	}
	if !strings.Contains(err.Error(), "task text is required") {
		t.Fatalf("task add empty error = %v, want task text required", err)
	}
}

func TestTaskListShowsPersistedTasks(t *testing.T) {
	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	ctx := context.Background()
	store, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	base := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	for _, spec := range []taskqueue.TaskSpec{
		{ID: "task-b", Task: "Second task", Summary: "second", CreatedAt: base.Add(time.Minute)},
		{ID: "task-a", Task: "First task", Summary: "first", CreatedAt: base},
	} {
		if _, err := store.AddTask(ctx, spec); err != nil {
			t.Fatalf("add %s: %v", spec.ID, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	out, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatalf("execute task list: %v", err)
	}
	want := "ID\tSTATUS\tTASK\tSUMMARY\n" +
		"task-a\tpending\tFirst task\tfirst\n" +
		"task-b\tpending\tSecond task\tsecond\n"
	if out != want {
		t.Fatalf("task list output = %q, want %q", out, want)
	}
}

func TestTaskListHandlesEmptyQueue(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	out, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatalf("execute task list: %v", err)
	}
	if got, want := out, "No tasks.\n"; got != want {
		t.Fatalf("empty task list output = %q, want %q", got, want)
	}
}

func TestRunOnceInvokesRunnerAndPrintsSummary(t *testing.T) {
	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: "/repo",
		RunOnce: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
			called = true
			if cfg.WorkingDir != "/repo" {
				t.Fatalf("working dir = %q, want /repo", cfg.WorkingDir)
			}
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-1"},
				Task:    taskqueue.Task{ID: "task-1"},
				Commit:  commit.Result{CommitSHA: "abc123"},
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}
	if !called {
		t.Fatal("run once runner was not called")
	}

	if got, want := out.String(), "Run run-1 completed task task-1; commit abc123.\n"; got != want {
		t.Fatalf("run once output = %q, want %q", got, want)
	}
}

func TestRunOncePrintsNoTaskSummary(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}

	if got, want := out.String(), "No pending runnable tasks.\n"; got != want {
		t.Fatalf("run once output = %q, want %q", got, want)
	}
}

func TestShowRunPrintsPersistedRunAndEvents(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(2 * time.Minute)
	exitCode := 0
	eventTime := startedAt.Add(time.Second)
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:                 "run-show",
		TaskID:             "task-show",
		Task:               "Implement show command",
		Status:             ledger.StatusCompleted,
		Summary:            "completed cleanly",
		StartedAt:          startedAt,
		CompletedAt:        &completedAt,
		DurationSeconds:    120,
		CodexExitCode:      &exitCode,
		VerificationStatus: "passed",
		CommitSHA:          "abc123",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-show", ledger.EventRunStarted, map[string]any{"run_id": "run-show"}); err != nil {
		t.Fatalf("append start event: %v", err)
	}
	eventTime = completedAt
	if _, err := runs.AppendEvent(ctx, "run-show", ledger.EventRunCompleted, map[string]any{"summary": "completed cleanly"}); err != nil {
		t.Fatalf("append completed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	out, err := executeCLI(t, workDir, "show", "run-show")
	if err != nil {
		t.Fatalf("execute show: %v", err)
	}
	want := "Run ID: run-show\n" +
		"Task ID: task-show\n" +
		"Task: Implement show command\n" +
		"Status: completed\n" +
		"Summary: completed cleanly\n" +
		"Started at: 2026-06-26T12:00:00Z\n" +
		"Completed at: 2026-06-26T12:02:00Z\n" +
		"Codex exit code: 0\n" +
		"Verification status: passed\n" +
		"Commit SHA: abc123\n" +
		"Events:\n" +
		"ID\tTYPE\tTIMESTAMP\n" +
		"1\trun_started\t2026-06-26T12:00:01Z\n" +
		"2\trun_completed\t2026-06-26T12:02:00Z\n"
	if out != want {
		t.Fatalf("show output = %q, want %q", out, want)
	}
}

func TestShowRunNotFoundReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	_, err := executeCLI(t, workDir, "show", "missing-run")
	if err == nil {
		t.Fatal("show missing run succeeded, want error")
	}
	if !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("show missing run error = %v, want not found", err)
	}
}

func TestShowUninitializedReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	_, err := executeCLI(t, workDir, "show", "run-missing-state")
	if err == nil {
		t.Fatal("show uninitialized state succeeded, want error")
	}
	if !strings.Contains(err.Error(), "state is not initialized") {
		t.Fatalf("show uninitialized error = %v, want state not initialized", err)
	}

	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	if _, err := os.Stat(paths.StateDir); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}
}

func TestShowRequiresRunID(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})
	root.SetArgs([]string{"show"})

	if err := root.Execute(); err == nil {
		t.Fatal("execute show without run id succeeded, want error")
	}
}

func executeCLI(t *testing.T, workDir string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out, WorkDir: workDir})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}

func assertTaskStoreOpens(t *testing.T, path string) {
	t.Helper()
	assertFileExists(t, path)
	store, err := taskqueue.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open task store %s: %v", path, err)
	}
	defer store.Close()
	tasks, err := store.ListTasks(context.Background())
	if err != nil {
		t.Fatalf("list tasks from %s: %v", path, err)
	}
	if tasks == nil {
		return
	}
}

func assertLedgerStoreOpens(t *testing.T, path string) {
	t.Helper()
	assertFileExists(t, path)
	store, err := ledger.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open ledger store %s: %v", path, err)
	}
	defer store.Close()
	runs, err := store.ListRecentRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("list runs from %s: %v", path, err)
	}
	if runs == nil {
		return
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory, want file", path)
	}
}

func readTasks(t *testing.T, workDir string) []taskqueue.Task {
	t.Helper()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	store, err := taskqueue.Open(context.Background(), paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	defer store.Close()
	tasks, err := store.ListTasks(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	return tasks
}
