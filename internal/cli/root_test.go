package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
	tuiapp "revolvr/internal/tui"
	"revolvr/internal/verification"
)

func TestNewRootCommandConstructsExpectedCommands(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})

	for _, args := range [][]string{
		{"init"},
		{"task"},
		{"task", "add"},
		{"task", "list"},
		{"task", "retry"},
		{"task", "unblock"},
		{"config"},
		{"config", "check"},
		{"run"},
		{"doctor"},
		{"status"},
		{"tui"},
		{"show"},
		{"receipt"},
		{"receipt", "validate"},
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
	for _, want := range []string{"Run bounded Codex harness passes", "init", "task", "run", "doctor", "status", "tui", "show", "receipt"} {
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

func TestParentCommandHelpOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		wantParts []string
	}{
		{
			name: "task",
			args: []string{"task"},
			wantParts: []string{
				"Manage tasks",
				"Usage:\n  revolvr task [flags]\n  revolvr task [command]",
				"Available Commands:",
				"add",
				"list",
				"retry",
				"unblock",
			},
		},
		{
			name: "config",
			args: []string{"config"},
			wantParts: []string{
				"Inspect run configuration",
				"Usage:\n  revolvr config [flags]\n  revolvr config [command]",
				"Available Commands:",
				"check",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			root := NewRootCommand(Options{Version: "test", Out: &out})
			root.SetArgs(tc.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("execute %s: %v", strings.Join(tc.args, " "), err)
			}

			help := out.String()
			if strings.Contains(help, "is not implemented yet") {
				t.Fatalf("help output contains placeholder:\n%s", help)
			}
			for _, want := range tc.wantParts {
				if !strings.Contains(help, want) {
					t.Fatalf("help output missing %q:\n%s", want, help)
				}
			}
		})
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
		{
			ID:                 "run-new",
			TaskID:             "task-new",
			Task:               "new run",
			Status:             ledger.StatusFailed,
			Summary:            "verification command 0 failed",
			StartedAt:          base.Add(time.Hour),
			VerificationStatus: "failed",
			CommitSHA:          "abc123",
		},
	} {
		if _, err := runs.CreateRun(ctx, spec); err != nil {
			t.Fatalf("create %s: %v", spec.ID, err)
		}
	}
	if _, err := runs.AppendEvent(ctx, "run-new", ledger.EventRunArtifacts, ledger.RunArtifacts{
		PromptPath:  ".revolvr/runs/run-new/prompt.md",
		ReceiptPath: ".revolvr/receipts/run-new.md",
	}); err != nil {
		t.Fatalf("append artifact event: %v", err)
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
		"Latest run: run-new (failed)\n" +
		"Latest summary: verification command 0 failed\n" +
		"Latest verification: failed\n" +
		"Latest commit: abc123\n" +
		"Latest artifacts:\n" +
		"prompt: .revolvr/runs/run-new/prompt.md\n" +
		"receipt: .revolvr/receipts/run-new.md\n"
	if out != want {
		t.Fatalf("status output = %q, want %q", out, want)
	}
}

func TestTUIUninitializedRendersStatusSnapshotWithoutCreatingState(t *testing.T) {
	workDir := t.TempDir()
	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		TUIRunner: func(_ context.Context, status app.StatusResult, opts tuiapp.RunOptions) error {
			called = true
			if status.Initialized {
				t.Fatalf("tui status initialized = true, want false")
			}
			model := tuiapp.NewStatusModel(status)
			updated, cmd := model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			if cmd != nil {
				t.Fatalf("window size update cmd = %v, want nil", cmd)
			}
			_, err := fmt.Fprint(opts.Output, updated.View())
			return err
		},
	})
	root.SetArgs([]string{"tui"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute tui: %v", err)
	}
	if !called {
		t.Fatal("tui runner was not called")
	}
	if !strings.Contains(out.String(), "State: not initialized") {
		t.Fatalf("tui output missing uninitialized state:\n%s", out.String())
	}

	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	if _, err := os.Stat(paths.StateDir); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}
}

func TestTUIRendersTaskCountsLatestRunAndRecentRunsFromAppStatus(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	base := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
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
		{
			ID:        "run-old",
			TaskID:    "task-old",
			Task:      "older run",
			Status:    ledger.StatusCompleted,
			Summary:   "older summary",
			StartedAt: base,
			CommitSHA: "abc123",
		},
		{
			ID:                 "run-new",
			TaskID:             "task-new",
			Task:               "new run",
			Status:             ledger.StatusFailed,
			Summary:            "latest summary",
			StartedAt:          base.Add(time.Hour),
			VerificationStatus: "failed",
		},
	} {
		if _, err := runs.CreateRun(ctx, spec); err != nil {
			t.Fatalf("create %s: %v", spec.ID, err)
		}
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		TUIRunner: func(_ context.Context, status app.StatusResult, opts tuiapp.RunOptions) error {
			called = true
			if !status.Initialized {
				t.Fatalf("tui status initialized = false, want true")
			}
			if got, want := len(status.Tasks), 3; got != want {
				t.Fatalf("tui task count = %d, want %d", got, want)
			}
			if got, want := runIDs(status.RecentRuns), []string{"run-new", "run-old"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("tui recent runs = %#v, want %#v", got, want)
			}
			model := tuiapp.NewStatusModel(status)
			updated, cmd := model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			if cmd != nil {
				t.Fatalf("window size update cmd = %v, want nil", cmd)
			}
			_, err := fmt.Fprint(opts.Output, updated.View())
			return err
		},
	})
	root.SetArgs([]string{"tui"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute tui: %v", err)
	}
	if !called {
		t.Fatal("tui runner was not called")
	}
	for _, want := range []string{
		"Total: 3",
		"Pending: 1",
		"Blocked: 1",
		"Completed: 1",
		"Latest Run",
		"ID: run-new",
		"Summary: latest summary",
		"Recent Runs",
		"run-new  failed  latest summary",
		"run-old  completed  older summary",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("tui output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTUIRunnerReceivesReadOnlyRefreshAndOpenActions(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	base := time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC)
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:        "run-new",
		TaskID:    "task-new",
		Task:      "new run",
		Status:    ledger.StatusCompleted,
		Summary:   "completed summary",
		StartedAt: base,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-new", ledger.EventRunStarted, map[string]any{"run_id": "run-new"}); err != nil {
		t.Fatalf("append run event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			t.Fatal("tui invoked run once hook")
			return runonce.Result{}, nil
		},
		TUIRunner: func(_ context.Context, status app.StatusResult, opts tuiapp.RunOptions) error {
			called = true
			if !status.Initialized {
				t.Fatal("initial tui status initialized = false, want true")
			}
			if opts.RefreshStatus == nil {
				t.Fatal("refresh callback is nil")
			}
			if opts.OpenRun == nil {
				t.Fatal("open run callback is nil")
			}

			refreshed, err := opts.RefreshStatus()
			if err != nil {
				return err
			}
			if got, want := runIDs(refreshed.RecentRuns), []string{"run-new"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("refreshed recent runs = %#v, want %#v", got, want)
			}

			history, err := opts.OpenRun("run-new")
			if err != nil {
				return err
			}
			if history.Run.ID != "run-new" {
				t.Fatalf("opened run id = %q, want run-new", history.Run.ID)
			}
			if got, want := eventTypes(history.Events), []ledger.EventType{ledger.EventRunStarted}; !reflect.DeepEqual(got, want) {
				t.Fatalf("opened event types = %#v, want %#v", got, want)
			}

			_, err = fmt.Fprint(opts.Output, "tui actions ok\n")
			return err
		},
	})
	root.SetArgs([]string{"tui"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute tui: %v", err)
	}
	if !called {
		t.Fatal("tui runner was not called")
	}
	if got, want := out.String(), "tui actions ok\n"; got != want {
		t.Fatalf("tui output = %q, want %q", got, want)
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

func TestInitAddsStateDirToLocalGitExclude(t *testing.T) {
	workDir := t.TempDir()
	excludePath := filepath.Join(workDir, ".git", "info", "exclude")
	writeCLIFile(t, excludePath, "# local excludes\n")

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init second time: %v", err)
	}

	content, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read git exclude: %v", err)
	}
	if got := strings.Count(string(content), revolvrGitExcludePattern); got != 1 {
		t.Fatalf("exclude pattern count = %d, want 1; content:\n%s", got, content)
	}
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

func TestTaskRetryMakesBlockedTaskRunnableForRunOnce(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	base := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	now := base
	store, err := taskqueue.OpenWithClock(ctx, paths.TaskDBPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	original, err := store.AddTask(ctx, taskqueue.TaskSpec{
		ID:        "task-blocked",
		Task:      "retry this task",
		Summary:   "blocked summary",
		CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	now = base.Add(time.Minute)
	if _, ok, err := store.BlockTask(ctx, "task-blocked", "verification failed"); err != nil || !ok {
		t.Fatalf("block task: ok=%v err=%v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	out, err := executeCLI(t, workDir, "task", "retry", "task-blocked")
	if err != nil {
		t.Fatalf("execute task retry: %v", err)
	}
	if got, want := out, "Retried task task-blocked.\n"; got != want {
		t.Fatalf("task retry output = %q, want %q", got, want)
	}
	tasks := readTasks(t, workDir)
	if len(tasks) != 1 {
		t.Fatalf("tasks after retry = %+v, want one task", tasks)
	}
	task := tasks[0]
	if task.ID != original.ID || task.Task != original.Task || task.Summary != original.Summary || !task.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("task after retry = %+v, want same persisted task identity/history as %+v", task, original)
	}
	if task.Status != taskqueue.StatusPending || task.Blocker != "" || task.BlockedAt != nil || task.CompletedAt != nil {
		t.Fatalf("task after retry = %+v, want pending task with blocker state cleared", task)
	}

	var runOut bytes.Buffer
	var selectedID string
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &runOut,
		WorkDir: workDir,
		RunOnce: func(ctx context.Context, cfg runonce.Config) (runonce.Result, error) {
			if cfg.WorkingDir != workDir {
				t.Fatalf("working dir = %q, want %q", cfg.WorkingDir, workDir)
			}
			store, err := taskqueue.Open(ctx, paths.TaskDBPath)
			if err != nil {
				return runonce.Result{}, err
			}
			defer store.Close()
			task, ok, err := store.SelectNext(ctx)
			if err != nil {
				return runonce.Result{}, err
			}
			if !ok {
				return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
			}
			selectedID = task.ID
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-selected"},
				Task:    task,
				Commit:  commit.Result{CommitSHA: "abc123"},
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}
	if selectedID != "task-blocked" {
		t.Fatalf("selected task = %q, want task-blocked", selectedID)
	}
	if got, want := runOut.String(), "Run run-selected completed task task-blocked; commit abc123.\n"; got != want {
		t.Fatalf("run output = %q, want %q", got, want)
	}
}

func TestTaskRetryDoesNotRevertCompletedTask(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	ctx := context.Background()
	store, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	if _, err := store.AddTask(ctx, taskqueue.TaskSpec{ID: "task-done", Task: "already done"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	if _, ok, err := store.CompleteTask(ctx, "task-done", "done"); err != nil || !ok {
		t.Fatalf("complete task: ok=%v err=%v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	_, err = executeCLI(t, workDir, "task", "retry", "task-done")
	if err == nil {
		t.Fatal("task retry completed task succeeded, want error")
	}
	if !strings.Contains(err.Error(), `task "task-done" is not blocked (status: completed)`) {
		t.Fatalf("task retry error = %v, want not blocked", err)
	}
	tasks := readTasks(t, workDir)
	if len(tasks) != 1 || tasks[0].Status != taskqueue.StatusCompleted || tasks[0].CompletedAt == nil {
		t.Fatalf("tasks after completed retry = %+v, want still completed", tasks)
	}
}

func TestTaskRetryMissingTaskReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	_, err := executeCLI(t, workDir, "task", "retry", "missing-task")
	if err == nil {
		t.Fatal("task retry missing task succeeded, want error")
	}
	if !strings.Contains(err.Error(), `task "missing-task" not found`) {
		t.Fatalf("task retry missing error = %v, want not found", err)
	}
}

func TestTaskUnblockDoesNotRevertCompletedTask(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	ctx := context.Background()
	store, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	if _, err := store.AddTask(ctx, taskqueue.TaskSpec{ID: "task-done", Task: "already done"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	if _, ok, err := store.CompleteTask(ctx, "task-done", "done"); err != nil || !ok {
		t.Fatalf("complete task: ok=%v err=%v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close task store: %v", err)
	}

	_, err = executeCLI(t, workDir, "task", "unblock", "task-done")
	if err == nil {
		t.Fatal("task unblock completed task succeeded, want error")
	}
	if !strings.Contains(err.Error(), `task "task-done" is not blocked (status: completed)`) {
		t.Fatalf("task unblock error = %v, want not blocked", err)
	}
	tasks := readTasks(t, workDir)
	if len(tasks) != 1 || tasks[0].Status != taskqueue.StatusCompleted || tasks[0].CompletedAt == nil {
		t.Fatalf("tasks after completed unblock = %+v, want still completed", tasks)
	}
}

func TestTaskUnblockMissingTaskReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	_, err := executeCLI(t, workDir, "task", "unblock", "missing-task")
	if err == nil {
		t.Fatal("task unblock missing task succeeded, want error")
	}
	if !strings.Contains(err.Error(), `task "missing-task" not found`) {
		t.Fatalf("task unblock missing error = %v, want not found", err)
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
			if cfg.CodexExecutable != "" || cfg.GitExecutable != "" || cfg.VerificationCommands != nil || cfg.AllowPreExistingDirty {
				t.Fatalf("run config = %+v, want no config-file overrides", cfg)
			}
			if !cfg.CodexBypassApprovalsAndSandbox {
				t.Fatal("codex bypass approvals and sandbox = false, want default true")
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

func TestRunOncePrintsLiveCodexProgressBeforeSummary(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: "/repo",
		RunOnce: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
			if cfg.CodexProgress == nil {
				t.Fatal("codex progress callback is nil")
			}
			cfg.CodexProgress(codexexec.ProgressEvent{Source: "codex", Message: "message: working"})
			cfg.CodexProgress(codexexec.ProgressEvent{Source: "codex stderr", Message: "checking status"})
			cfg.CodexProgress(codexexec.ProgressEvent{Source: "codex", Message: "   "})
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-progress"},
				Task:    taskqueue.Task{ID: "task-progress"},
				Commit:  commit.Result{CommitSHA: "abc123"},
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}
	want := "codex: message: working\n" +
		"codex stderr: checking status\n" +
		"Run run-progress completed task task-progress; commit abc123.\n"
	if out.String() != want {
		t.Fatalf("run output = %q, want %q", out.String(), want)
	}
}

func TestRunOnceLoadsRepoLocalConfig(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-custom
  sandbox: danger-full-access
  approval_policy: on-request
  dangerously_bypass_approvals_and_sandbox: true
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
	var out bytes.Buffer
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		RunOnce: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
			got = cfg
			return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}

	if got.WorkingDir != workDir {
		t.Fatalf("working dir = %q, want %q", got.WorkingDir, workDir)
	}
	if got.CodexExecutable != "codex-custom" || got.CodexSandbox != "danger-full-access" || got.CodexApprovalPolicy != "on-request" || !got.CodexBypassApprovalsAndSandbox || got.CodexTimeout != 45*time.Second {
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
	if got, want := out.String(), "No pending runnable tasks.\n"; got != want {
		t.Fatalf("run once output = %q, want %q", got, want)
	}
}

func TestRunOnceInvalidConfigReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
verification:
  missing_policy: maybe
`)

	called := false
	root := NewRootCommand(Options{
		Version: "test",
		WorkDir: workDir,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	err := root.Execute()
	if err == nil {
		t.Fatal("execute run --once succeeded, want config error")
	}
	if called {
		t.Fatal("run once runner was called after invalid config")
	}
	if !strings.Contains(err.Error(), "invalid verification missing_policy") || !strings.Contains(err.Error(), "maybe") {
		t.Fatalf("config error = %v, want invalid missing_policy", err)
	}
}

func TestRunOnceUnknownConfigFieldReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  typo: codex
`)

	called := false
	root := NewRootCommand(Options{
		Version: "test",
		WorkDir: workDir,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	err := root.Execute()
	if err == nil {
		t.Fatal("execute run --once succeeded, want config error")
	}
	if called {
		t.Fatal("run once runner was called after invalid config")
	}
	if !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("config error = %v, want unknown field error", err)
	}
}

func TestConfigCheckMissingConfigSucceeds(t *testing.T) {
	workDir := t.TempDir()

	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	want := "Config path: " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"Config found: false\n" +
		"Defaults: used\n" +
		"Codex executable: codex\n" +
		"Codex dangerously bypass approvals and sandbox: true\n" +
		"Codex sandbox: workspace-write\n" +
		"Codex approval policy: never\n" +
		"Codex timeout: 0s\n" +
		"Git executable: git\n" +
		"Git timeout: 30s\n" +
		"Verification missing policy: fail\n" +
		"Verification command count: 0\n" +
		"Commit allow pre-existing dirty: false\n" +
		"Commit allow missing verification: false\n" +
		"Commit timeout: 30s\n" +
		"Output caps bytes: codex_stdout=262144 codex_stderr=262144 git_stdout=262144 git_stderr=262144 verification_stdout=262144 verification_stderr=262144 commit_stdout=262144 commit_stderr=262144\n"
	if out != want {
		t.Fatalf("config check output = %q, want %q", out, want)
	}
}

func TestConfigCheckMissingConfigPrintsDefaultVerificationCommand(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, "go.mod"), "module example.com/revolvrtest\n")

	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	want := "Config path: " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"Config found: false\n" +
		"Defaults: used\n" +
		"Codex executable: codex\n" +
		"Codex dangerously bypass approvals and sandbox: true\n" +
		"Codex sandbox: workspace-write\n" +
		"Codex approval policy: never\n" +
		"Codex timeout: 0s\n" +
		"Git executable: git\n" +
		"Git timeout: 30s\n" +
		"Verification missing policy: fail\n" +
		"Verification command count: 1\n" +
		"Verification command 0: name=go args=[\"test\", \"./...\"]\n" +
		"Commit allow pre-existing dirty: false\n" +
		"Commit allow missing verification: false\n" +
		"Commit timeout: 30s\n" +
		"Output caps bytes: codex_stdout=262144 codex_stderr=262144 git_stdout=262144 git_stderr=262144 verification_stdout=262144 verification_stderr=262144 commit_stdout=262144 commit_stderr=262144\n"
	if out != want {
		t.Fatalf("config check output = %q, want %q", out, want)
	}
}

func TestConfigCheckValidConfigPrintsEffectiveValuesAndDoesNotRunOnce(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-custom
  sandbox: danger-full-access
  approval_policy: on-request
  yolo: true
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

	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	root.SetArgs([]string{"config", "check"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	if called {
		t.Fatal("config check invoked run once hook")
	}

	want := "Config path: " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"Config found: true\n" +
		"Defaults: merged\n" +
		"Codex executable: codex-custom\n" +
		"Codex dangerously bypass approvals and sandbox: true\n" +
		"Codex sandbox: danger-full-access\n" +
		"Codex approval policy: on-request\n" +
		"Codex timeout: 45s\n" +
		"Git executable: git-custom\n" +
		"Git timeout: 12s\n" +
		"Verification missing policy: pass\n" +
		"Verification command count: 1\n" +
		"Verification command 0: name=go args=[\"test\", \"./...\"] dir=internal timeout=9s\n" +
		"Commit allow pre-existing dirty: true\n" +
		"Commit allow missing verification: true\n" +
		"Commit timeout: 30s\n" +
		"Output caps bytes: codex_stdout=101 codex_stderr=102 git_stdout=103 git_stderr=104 verification_stdout=105 verification_stderr=106 commit_stdout=107 commit_stderr=108\n"
	if out.String() != want {
		t.Fatalf("config check output = %q, want %q", out.String(), want)
	}
}

func TestConfigCheckCanDisableCodexBypassDefault(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  yolo: false
`)

	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	if !strings.Contains(out, "Codex dangerously bypass approvals and sandbox: false\n") {
		t.Fatalf("config check output = %q, want yolo false", out)
	}
}

func TestConfigCheckInvalidMissingPolicyReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
verification:
  missing_policy: maybe
`)

	_, err := executeCLI(t, workDir, "config", "check")
	if err == nil {
		t.Fatal("config check succeeded, want config error")
	}
	if !strings.Contains(err.Error(), "invalid verification missing_policy") || !strings.Contains(err.Error(), "maybe") {
		t.Fatalf("config check error = %v, want invalid missing_policy", err)
	}
}

func TestConfigCheckRejectsConflictingCodexBypassAliases(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  dangerously_bypass_approvals_and_sandbox: true
  yolo: true
`)

	_, err := executeCLI(t, workDir, "config", "check")
	if err == nil {
		t.Fatal("config check succeeded, want conflicting bypass aliases error")
	}
	if !strings.Contains(err.Error(), "dangerously_bypass_approvals_and_sandbox and yolo cannot both be set") {
		t.Fatalf("config check error = %v, want conflicting bypass aliases", err)
	}
}

func TestConfigCheckUnknownFieldReturnsClearError(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  typo: codex
`)

	_, err := executeCLI(t, workDir, "config", "check")
	if err == nil {
		t.Fatal("config check succeeded, want config error")
	}
	if !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("config check error = %v, want unknown field error", err)
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

func TestRunOnceReturnsErrorForFailedOutcomesAndPrintsSummary(t *testing.T) {
	for _, outcome := range []runonce.Outcome{
		runonce.OutcomeBlocked,
		runonce.OutcomeCodexFailed,
		runonce.OutcomeVerificationFailed,
		runonce.OutcomeNoChanges,
		runonce.OutcomeCommitFailed,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			var out bytes.Buffer
			root := NewRootCommand(Options{
				Version: "test",
				Out:     &out,
				WorkDir: t.TempDir(),
				RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
					return runonce.Result{
						Outcome: outcome,
						Run:     ledger.Run{ID: "run-" + string(outcome)},
						Task:    taskqueue.Task{ID: "task-1"},
						Message: "stopped",
					}, nil
				},
			})
			root.SetArgs([]string{"run", "--once"})

			err := root.Execute()
			if err == nil {
				t.Fatalf("execute run --once outcome %s succeeded, want error", outcome)
			}
			wantErr := "run run-" + string(outcome) + " stopped with outcome " + string(outcome)
			if err.Error() != wantErr {
				t.Fatalf("run error = %q, want %q", err.Error(), wantErr)
			}
			wantOut := "Run run-" + string(outcome) + " stopped (" + string(outcome) + "): stopped\n"
			if out.String() != wantOut {
				t.Fatalf("run output = %q, want %q", out.String(), wantOut)
			}
		})
	}
}

func TestRunMaxPassesStopsAfterNoTask(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: t.TempDir(),
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			calls++
			if calls == 1 {
				return runonce.Result{
					Outcome: runonce.OutcomeCommitted,
					Run:     ledger.Run{ID: "run-1"},
					Task:    taskqueue.Task{ID: "task-1"},
					Commit:  commit.Result{CommitSHA: "abc123"},
				}, nil
			}
			return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
		},
	})
	root.SetArgs([]string{"run", "--max-passes", "5"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --max-passes: %v", err)
	}
	if calls != 2 {
		t.Fatalf("run calls = %d, want 2", calls)
	}
	want := "Run run-1 completed task task-1; commit abc123.\n" +
		"No pending runnable tasks.\n" +
		"Loop summary: passes=2/5 completed=1 failed_or_blocked=0 no_task=true stop=no_task\n"
	if out.String() != want {
		t.Fatalf("loop output = %q, want %q", out.String(), want)
	}
}

func TestRunMaxPassesStopsAfterRepeatedFailuresWithSummary(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: t.TempDir(),
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			calls++
			return runonce.Result{
				Outcome: runonce.OutcomeVerificationFailed,
				Run:     ledger.Run{ID: "run-failed"},
				Task:    taskqueue.Task{ID: "task-failed"},
				Message: "verification command 0 failed",
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--max-passes", "3"})

	err := root.Execute()
	if err == nil {
		t.Fatal("execute run --max-passes succeeded, want repeated failure guardrail error")
	}
	if calls != 2 {
		t.Fatalf("run calls = %d, want 2", calls)
	}
	if got, want := err.Error(), "run loop stopped after 2 consecutive failed or blocked passes"; got != want {
		t.Fatalf("loop error = %q, want %q", got, want)
	}
	wantOut := "Run run-failed stopped (verification_failed): verification command 0 failed\n" +
		"Run run-failed stopped (verification_failed): verification command 0 failed\n" +
		"Loop summary: passes=2/3 completed=0 failed_or_blocked=2 no_task=false stop=failure_guardrail\n"
	if out.String() != wantOut {
		t.Fatalf("loop output = %q, want %q", out.String(), wantOut)
	}
}

func TestRunMaxPassesStopsAfterBlockedOutcomeWithSummary(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: t.TempDir(),
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			calls++
			return runonce.Result{
				Outcome: runonce.OutcomeBlocked,
				Run:     ledger.Run{ID: fmt.Sprintf("run-blocked-%d", calls)},
				Task:    taskqueue.Task{ID: fmt.Sprintf("task-blocked-%d", calls)},
				Message: "blocked by preflight",
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--max-passes", "5"})

	err := root.Execute()
	if err == nil {
		t.Fatal("execute run --max-passes succeeded, want blocked outcome error")
	}
	if calls != 1 {
		t.Fatalf("run calls = %d, want 1", calls)
	}
	if got, want := err.Error(), "run run-blocked-1 stopped with outcome blocked"; got != want {
		t.Fatalf("loop error = %q, want %q", got, want)
	}
	wantOut := "Run run-blocked-1 stopped (blocked): blocked by preflight\n" +
		"Loop summary: passes=1/5 completed=0 failed_or_blocked=1 no_task=false stop=failed_or_blocked\n"
	if out.String() != wantOut {
		t.Fatalf("loop output = %q, want %q", out.String(), wantOut)
	}
}

func TestRunMaxPassesStopsAfterFailedPassWithChangedFiles(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: t.TempDir(),
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			calls++
			return runonce.Result{
				Outcome:        runonce.OutcomeVerificationFailed,
				Run:            ledger.Run{ID: "run-dirty-failure"},
				Task:           taskqueue.Task{ID: "task-dirty-failure"},
				Message:        "verification command 0 failed",
				PostRunChanged: gitstate.Capture{ChangedFiles: []string{"internal/feature.go"}},
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--max-passes", "4"})

	err := root.Execute()
	if err == nil {
		t.Fatal("execute run --max-passes succeeded, want failed dirty pass error")
	}
	if calls != 1 {
		t.Fatalf("run calls = %d, want 1", calls)
	}
	if got, want := err.Error(), "run run-dirty-failure stopped with outcome verification_failed"; got != want {
		t.Fatalf("loop error = %q, want %q", got, want)
	}
	wantOut := "Run run-dirty-failure stopped (verification_failed): verification command 0 failed\n" +
		"Loop summary: passes=1/4 completed=0 failed_or_blocked=1 no_task=false stop=failed_or_blocked\n"
	if out.String() != wantOut {
		t.Fatalf("loop output = %q, want %q", out.String(), wantOut)
	}
}

func TestRunMaxPassesCapIsHonored(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: t.TempDir(),
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			calls++
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: fmt.Sprintf("run-%d", calls)},
				Task:    taskqueue.Task{ID: fmt.Sprintf("task-%d", calls)},
				Commit:  commit.Result{CommitSHA: fmt.Sprintf("abc%d", calls)},
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--max-passes", "2"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --max-passes: %v", err)
	}
	if calls != 2 {
		t.Fatalf("run calls = %d, want 2", calls)
	}
	want := "Run run-1 completed task task-1; commit abc1.\n" +
		"Run run-2 completed task task-2; commit abc2.\n" +
		"Loop summary: passes=2/2 completed=2 failed_or_blocked=0 no_task=false stop=max_passes\n"
	if out.String() != want {
		t.Fatalf("loop output = %q, want %q", out.String(), want)
	}
}

func TestRunMaxPassesInvalidConfigPrintsConfigErrorSummary(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
verification:
  missing_policy: maybe
`)

	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			called = true
			return runonce.Result{}, nil
		},
	})
	root.SetArgs([]string{"run", "--max-passes", "2"})

	err := root.Execute()
	if err == nil {
		t.Fatal("execute run --max-passes succeeded, want config error")
	}
	if called {
		t.Fatal("run once runner was called after invalid config")
	}
	if !strings.Contains(err.Error(), "invalid verification missing_policy") {
		t.Fatalf("loop error = %v, want invalid missing_policy", err)
	}
	want := "Loop summary: passes=0/2 completed=0 failed_or_blocked=0 no_task=false stop=config_error\n"
	if out.String() != want {
		t.Fatalf("loop output = %q, want %q", out.String(), want)
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

func TestShowRunPrintsPersistedArtifactPaths(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 6, 26, 13, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	exitCode := 1
	eventTime := startedAt.Add(time.Second)
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:                 "run-artifacts",
		TaskID:             "task-artifacts",
		Task:               "Expose artifact paths",
		Status:             ledger.StatusFailed,
		Summary:            "Codex exited with code 1",
		StartedAt:          startedAt,
		CompletedAt:        &completedAt,
		DurationSeconds:    60,
		CodexExitCode:      &exitCode,
		VerificationStatus: "not_run",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-artifacts", ledger.EventRunStarted, map[string]any{"run_id": "run-artifacts"}); err != nil {
		t.Fatalf("append start event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-artifacts", ledger.EventRunArtifacts, ledger.RunArtifacts{
		PromptPath:           ".revolvr/runs/run-artifacts/prompt.md",
		CodexStdoutJSONLPath: ".revolvr/runs/run-artifacts/codex.jsonl",
		CodexStderrPath:      ".revolvr/runs/run-artifacts/codex.stderr",
		LastMessagePath:      ".revolvr/runs/run-artifacts/last-message.txt",
		ReceiptPath:          ".revolvr/receipts/run-artifacts.md",
	}); err != nil {
		t.Fatalf("append artifact event: %v", err)
	}
	eventTime = completedAt
	if _, err := runs.AppendEvent(ctx, "run-artifacts", ledger.EventRunFailed, map[string]any{"summary": "Codex exited with code 1"}); err != nil {
		t.Fatalf("append failed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	out, err := executeCLI(t, workDir, "show", "run-artifacts")
	if err != nil {
		t.Fatalf("execute show: %v", err)
	}
	want := "Run ID: run-artifacts\n" +
		"Task ID: task-artifacts\n" +
		"Task: Expose artifact paths\n" +
		"Status: failed\n" +
		"Summary: Codex exited with code 1\n" +
		"Started at: 2026-06-26T13:00:00Z\n" +
		"Completed at: 2026-06-26T13:01:00Z\n" +
		"Codex exit code: 1\n" +
		"Verification status: not_run\n" +
		"Artifacts:\n" +
		"prompt: .revolvr/runs/run-artifacts/prompt.md\n" +
		"codex stdout jsonl: .revolvr/runs/run-artifacts/codex.jsonl\n" +
		"codex stderr: .revolvr/runs/run-artifacts/codex.stderr\n" +
		"last message: .revolvr/runs/run-artifacts/last-message.txt\n" +
		"receipt: .revolvr/receipts/run-artifacts.md\n" +
		"Events:\n" +
		"ID\tTYPE\tTIMESTAMP\n" +
		"1\trun_started\t2026-06-26T13:00:01Z\n" +
		"2\trun_artifacts\t2026-06-26T13:00:02Z\n" +
		"3\trun_failed\t2026-06-26T13:01:00Z\n"
	if out != want {
		t.Fatalf("show output = %q, want %q", out, want)
	}
}

func TestShowRunPrintsNoneForEmptyArtifactEvent(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC)
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return startedAt })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:        "run-empty-artifacts",
		TaskID:    "task-empty-artifacts",
		Task:      "Empty artifacts",
		Status:    ledger.StatusFailed,
		StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-empty-artifacts", ledger.EventRunArtifacts, ledger.RunArtifacts{}); err != nil {
		t.Fatalf("append artifact event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	out, err := executeCLI(t, workDir, "show", "run-empty-artifacts")
	if err != nil {
		t.Fatalf("execute show: %v", err)
	}
	want := "Run ID: run-empty-artifacts\n" +
		"Task ID: task-empty-artifacts\n" +
		"Task: Empty artifacts\n" +
		"Status: failed\n" +
		"Started at: 2026-06-26T14:00:00Z\n" +
		"Artifacts:\n" +
		"none\n" +
		"Events:\n" +
		"ID\tTYPE\tTIMESTAMP\n" +
		"1\trun_artifacts\t2026-06-26T14:00:00Z\n"
	if out != want {
		t.Fatalf("show output = %q, want %q", out, want)
	}
}

func TestShowRunPrintsDiagnosticsForFailedRun(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 6, 26, 15, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(30 * time.Second)
	exitCode := 0
	eventTime := startedAt
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:                 "run-diagnostics",
		TaskID:             "task-diagnostics",
		Task:               "Surface failure details",
		Status:             ledger.StatusFailed,
		Summary:            "verification command 0 failed",
		StartedAt:          startedAt,
		CompletedAt:        &completedAt,
		DurationSeconds:    30,
		CodexExitCode:      &exitCode,
		VerificationStatus: "failed",
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-diagnostics", ledger.EventRunStarted, map[string]any{"run_id": "run-diagnostics"}); err != nil {
		t.Fatalf("append start event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-diagnostics", ledger.EventCodexCompleted, map[string]any{
		"exit_code": 0,
		"timed_out": false,
		"error":     "",
	}); err != nil {
		t.Fatalf("append codex event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-diagnostics", ledger.EventChangedFilesCaptured, map[string]any{
		"changed_files": []string{"internal/broken.go"},
	}); err != nil {
		t.Fatalf("append changed files event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-diagnostics", ledger.EventVerificationCompleted, map[string]any{
		"status":               "failed",
		"passed":               false,
		"failed_command_index": 0,
		"commands": []map[string]any{{
			"index":     0,
			"command":   "go test ./...",
			"status":    "failed",
			"passed":    false,
			"exit_code": 1,
		}},
	}); err != nil {
		t.Fatalf("append verification event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-diagnostics", ledger.EventReceiptSynthesized, map[string]any{
		"receipt_path": ".revolvr/receipts/run-diagnostics.md",
		"verdict":      "verification_failed",
	}); err != nil {
		t.Fatalf("append receipt event: %v", err)
	}
	eventTime = completedAt
	if _, err := runs.AppendEvent(ctx, "run-diagnostics", ledger.EventRunFailed, map[string]any{
		"outcome": "verification_failed",
		"message": "verification command 0 failed",
	}); err != nil {
		t.Fatalf("append failed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	out, err := executeCLI(t, workDir, "show", "run-diagnostics")
	if err != nil {
		t.Fatalf("execute show: %v", err)
	}
	want := "Run ID: run-diagnostics\n" +
		"Task ID: task-diagnostics\n" +
		"Task: Surface failure details\n" +
		"Status: failed\n" +
		"Summary: verification command 0 failed\n" +
		"Started at: 2026-06-26T15:00:00Z\n" +
		"Completed at: 2026-06-26T15:00:30Z\n" +
		"Codex exit code: 0\n" +
		"Verification status: failed\n" +
		"Artifacts:\n" +
		"receipt: .revolvr/receipts/run-diagnostics.md\n" +
		"Diagnostics:\n" +
		"outcome: verification_failed\n" +
		"message: verification command 0 failed\n" +
		"codex: exit_code=0, timed_out=false\n" +
		"verification: failed\n" +
		"failed verification: go test ./... (exit_code=1)\n" +
		"receipt: verification_failed (.revolvr/receipts/run-diagnostics.md)\n" +
		"changed files: internal/broken.go\n" +
		"Events:\n" +
		"ID\tTYPE\tTIMESTAMP\n" +
		"1\trun_started\t2026-06-26T15:00:00Z\n" +
		"2\tcodex_completed\t2026-06-26T15:00:01Z\n" +
		"3\tchanged_files_captured\t2026-06-26T15:00:02Z\n" +
		"4\tverification_completed\t2026-06-26T15:00:03Z\n" +
		"5\treceipt_synthesized\t2026-06-26T15:00:04Z\n" +
		"6\trun_failed\t2026-06-26T15:00:30Z\n"
	if out != want {
		t.Fatalf("show output = %q, want %q", out, want)
	}
}

func TestShowRunPrintsReceiptWarningsInDiagnostics(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 6, 26, 16, 0, 0, 0, time.UTC)
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return startedAt })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:        "run-warning",
		TaskID:    "task-warning",
		Task:      "Surface receipt warning",
		Status:    ledger.StatusFailed,
		StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-warning", ledger.EventReceiptWarning, map[string]any{
		"warning_type": "changed_files_mismatch",
		"message":      "receipt changed files differ from harness captured changed files",
		"receipt_path": ".revolvr/receipts/run-warning.md",
		"claimed":      []string{"internal/claimed.go"},
		"observed":     []string{"internal/actual.go"},
	}); err != nil {
		t.Fatalf("append warning event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	out, err := executeCLI(t, workDir, "show", "run-warning")
	if err != nil {
		t.Fatalf("execute show: %v", err)
	}
	want := "Run ID: run-warning\n" +
		"Task ID: task-warning\n" +
		"Task: Surface receipt warning\n" +
		"Status: failed\n" +
		"Started at: 2026-06-26T16:00:00Z\n" +
		"Diagnostics:\n" +
		"warning: changed_files_mismatch: receipt changed files differ from harness captured changed files (.revolvr/receipts/run-warning.md)\n" +
		"Events:\n" +
		"ID\tTYPE\tTIMESTAMP\n" +
		"1\treceipt_warning\t2026-06-26T16:00:00Z\n"
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

func TestReceiptValidatePassesForConsistentRun(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	completedAt := time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)
	createValidationRun(t, workDir, validationRunSpec{
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

	out, err := executeCLI(t, workDir, "receipt", "validate", "run-valid-receipt")
	if err != nil {
		t.Fatalf("execute receipt validate: %v\n%s", err, out)
	}
	want := "Receipt validation: passed\n" +
		"Run ID: run-valid-receipt\n" +
		"Receipt: .revolvr/receipts/run-valid-receipt.md\n" +
		"Checks:\n" +
		"identity: ok\n" +
		"completion_time: ok\n" +
		"commit_sha: ok\n" +
		"changed_files: ok\n" +
		"verification_results: ok\n" +
		"artifacts: ok\n"
	if out != want {
		t.Fatalf("receipt validate output = %q, want %q", out, want)
	}
}

func TestReceiptValidateReportsMismatches(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	completedAt := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	createValidationRun(t, workDir, validationRunSpec{
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
		ReceiptTimestamp: completedAt.Add(time.Minute),
		ReceiptChangedFiles: []string{
			"internal/stale.go",
		},
		ReceiptVerificationStatus: "failed",
		ReceiptVerification: []receipt.VerificationEntry{{
			Command:  "go test ./...",
			ExitCode: 1,
			Status:   "failed",
		}},
		WriteArtifacts:          true,
		SkipCodexStderrArtifact: true,
	})

	out, err := executeCLI(t, workDir, "receipt", "validate", "run-invalid-receipt")
	if err == nil {
		t.Fatalf("execute receipt validate succeeded, want validation error\n%s", out)
	}
	if got, want := err.Error(), "receipt validation failed for run run-invalid-receipt (4 failed checks)"; got != want {
		t.Fatalf("receipt validate error = %q, want %q", got, want)
	}
	for _, want := range []string{
		"Receipt validation: failed\n",
		"completion_time: failed - receipt timestamp 2026-07-08T15:01:00Z does not match ledger completed_at 2026-07-08T15:00:00Z\n",
		"changed_files: failed - body changed files got [internal/stale.go], want [internal/actual.go]; frontmatter changed_files got [internal/stale.go], want [internal/actual.go]\n",
		"verification_results: failed - body verification[0] exit_code got 1, want 0; body verification[0] status got \"failed\", want \"passed\"; frontmatter verification[0] got go test ./... (failed, exit 1), want go test ./... (passed, exit 0); receipt verification_status \"failed\" does not match ledger verification_status \"passed\"\n",
		"artifacts: failed - codex stderr artifact does not exist: .revolvr/runs/run-invalid-receipt/codex.stderr\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("receipt validate output missing %q:\n%s", want, out)
		}
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

func writeCLIFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type validationRunSpec struct {
	RunID                     string
	TaskID                    string
	Task                      string
	CompletedAt               time.Time
	CommitSHA                 string
	ChangedFiles              []string
	VerificationStatus        string
	Verification              []receipt.VerificationEntry
	ReceiptTimestamp          time.Time
	ReceiptChangedFiles       []string
	ReceiptVerificationStatus string
	ReceiptVerification       []receipt.VerificationEntry
	WriteArtifacts            bool
	SkipCodexStderrArtifact   bool
}

func createValidationRun(t *testing.T, workDir string, spec validationRunSpec) {
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
	artifactPaths := ledger.RunArtifacts{
		PromptPath:           filepath.Join(".revolvr", "runs", spec.RunID, "prompt.md"),
		CodexStdoutJSONLPath: filepath.Join(".revolvr", "runs", spec.RunID, "codex.jsonl"),
		CodexStderrPath:      filepath.Join(".revolvr", "runs", spec.RunID, "codex.stderr"),
		LastMessagePath:      filepath.Join(".revolvr", "runs", spec.RunID, "last-message.txt"),
		ReceiptPath:          filepath.Join(".revolvr", "receipts", spec.RunID+".md"),
	}
	if _, err := runs.AppendEvent(ctx, spec.RunID, ledger.EventRunArtifacts, artifactPaths); err != nil {
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
		"commands": validationCommandPayloads(spec.Verification),
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
		writeCLIFile(t, filepath.Join(workDir, artifactPaths.PromptPath), "prompt")
		writeCLIFile(t, filepath.Join(workDir, artifactPaths.CodexStdoutJSONLPath), "{}\n")
		if !spec.SkipCodexStderrArtifact {
			writeCLIFile(t, filepath.Join(workDir, artifactPaths.CodexStderrPath), "")
		}
		writeCLIFile(t, filepath.Join(workDir, artifactPaths.LastMessagePath), "done")
	}

	receiptTimestamp := spec.ReceiptTimestamp
	if receiptTimestamp.IsZero() {
		receiptTimestamp = spec.CompletedAt
	}
	receiptChangedFiles := spec.ReceiptChangedFiles
	if receiptChangedFiles == nil {
		receiptChangedFiles = spec.ChangedFiles
	}
	receiptVerificationStatus := spec.ReceiptVerificationStatus
	if receiptVerificationStatus == "" {
		receiptVerificationStatus = spec.VerificationStatus
	}
	receiptVerification := spec.ReceiptVerification
	if receiptVerification == nil {
		receiptVerification = spec.Verification
	}
	content, _ := receipt.FormatFallbackReceipt(receipt.FallbackInput{
		RunID:              spec.RunID,
		PassID:             spec.RunID,
		TaskID:             spec.TaskID,
		Task:               spec.Task,
		Verdict:            receipt.VerdictCompleted,
		Timestamp:          receiptTimestamp,
		CodexExitCode:      0,
		VerificationStatus: receiptVerificationStatus,
		CommitSHA:          spec.CommitSHA,
		ChangedFiles:       receiptChangedFiles,
		Verification:       receiptVerification,
		Metrics:            receipt.Metrics{},
		FinalText:          "completed",
	})
	writeCLIFile(t, filepath.Join(workDir, artifactPaths.ReceiptPath), content)
}

func validationCommandPayloads(entries []receipt.VerificationEntry) []map[string]any {
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
