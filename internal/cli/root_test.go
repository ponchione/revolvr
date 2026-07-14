package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/artifactretention"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskscheduler"
	tuiapp "revolvr/internal/tui"
	"revolvr/internal/verification"
)

func TestNewRootCommandConstructsExpectedCommands(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})

	for _, args := range [][]string{
		{"init"},
		{"task"},
		{"task", "add"},
		{"task", "import"},
		{"task", "migrate"},
		{"task", "list"},
		{"task", "retry"},
		{"task", "unblock"},
		{"checkpoint"},
		{"checkpoint", "fulfill"},
		{"archive"},
		{"archive", "list"},
		{"archive", "show"},
		{"archive", "verify"},
		{"archive", "create"},
		{"archive", "reopen"},
		{"metrics"},
		{"metrics", "show"},
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
	for _, want := range []string{"Run bounded Codex harness passes", "init", "task", "checkpoint", "archive", "run", "doctor", "status", "tui", "show", "receipt"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestRunHelpDocumentsBareOnePassDefault(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out})
	root.SetArgs([]string{"run", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, want := range []string{"Run one harness pass", "--once", "run one selected task (the default mode)", "--max-passes", "--until-terminal", "--queue", "--daemon"} {
		if !strings.Contains(help, want) {
			t.Fatalf("run help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "not implemented") {
		t.Fatalf("run help retains placeholder language:\n%s", help)
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
				"import",
				"list",
				"migrate",
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
		"Next task: none\n" +
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
	writeCLITaskFile(t, workDir, "010-pending.md", cliTaskMarkdown("task-pending", "pending", "Pending File Task", "pending task"))
	writeCLITaskFile(t, workDir, "020-blocked.md", cliTaskMarkdown("task-blocked", "blocked", "Blocked File Task", "blocked task"))
	writeCLITaskFile(t, workDir, "030-completed.md", cliTaskMarkdown("task-completed", "completed", "Completed File Task", "completed task"))

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
		ContextPayloadPath:  ".revolvr/runs/run-new/context.md",
		ContextManifestPath: ".revolvr/runs/run-new/context.json",
		ReceiptPath:         ".revolvr/receipts/run-new.md",
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
		"Next task: task-pending - Pending File Task\n" +
		"Next pass: workflow=mixed-pass-v1 phase=implement profile=implementer next=audit\n" +
		"Recent runs: 2\n" +
		"Latest run: run-new (failed)\n" +
		"Latest summary: verification command 0 failed\n" +
		"Latest verification: failed\n" +
		"Latest commit: abc123\n" +
		"Latest artifacts:\n" +
		"context payload: .revolvr/runs/run-new/context.md\n" +
		"context manifest: .revolvr/runs/run-new/context.json\n" +
		"receipt: .revolvr/receipts/run-new.md\n"
	if out != want {
		t.Fatalf("status output = %q, want %q", out, want)
	}
}

func TestStatusUsesPrioritySelectedNextRunnableMarker(t *testing.T) {
	var out bytes.Buffer
	tasks := []taskmodel.Task{
		{ID: "task-filename-first", Status: taskmodel.StatusPending, Summary: "shown first", Workflow: "mixed-pass-v1", Phase: "implement", RunProfile: "implementer", NextState: "audit"},
		{ID: "task-priority-first", Status: taskmodel.StatusPending, Summary: "runs first", Workflow: "mixed-pass-v1", Phase: "audit", RunProfile: "auditor", NextState: "document", NextRunnable: true},
	}
	if err := writeStatus(&out, tasks, taskscheduler.Result{}, nil, nil); err != nil {
		t.Fatalf("write status: %v", err)
	}
	if !strings.Contains(out.String(), "Next task: task-priority-first - runs first\n") || !strings.Contains(out.String(), "Next pass: workflow=mixed-pass-v1 phase=audit profile=auditor next=document\n") {
		t.Fatalf("status output missing priority-selected task:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Next task: task-filename-first") {
		t.Fatalf("status output used filename-first task:\n%s", out.String())
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
	writeCLITaskFile(t, workDir, "010-pending.md", cliTaskMarkdown("task-pending", "pending", "Pending File Task", "pending task"))
	writeCLITaskFile(t, workDir, "020-blocked.md", cliTaskMarkdown("task-blocked", "blocked", "Blocked File Task", "blocked task"))
	writeCLITaskFile(t, workDir, "030-completed.md", cliTaskMarkdown("task-completed", "completed", "Completed File Task", "completed task"))

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
			tasksView, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
			if cmd != nil {
				t.Fatalf("tasks view update cmd = %v, want nil", cmd)
			}
			for _, want := range []string{
				"Runnable: ready to run",
				"Next task: task-pending - Pending File Task",
				"Workflow: mixed-pass-v1  Phase: implement  Profile: implementer  Next: audit",
				"> next task-pending  pending  phase=implement  profile=implementer  next=audit",
				"  - task-blocked  ! blocked  phase=implement  profile=implementer  next=audit",
				"  - task-completed  completed  phase=implement  profile=implementer  next=audit",
			} {
				if !strings.Contains(tasksView.View(), want) {
					t.Fatalf("tui tasks view missing %q:\n%s", want, tasksView.View())
				}
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
		"Next task: task-pending - Pending File Task",
		"Workflow: mixed-pass-v1  Phase: implement  Profile: implementer  Next: audit",
		"Latest Run",
		"ID: run-new",
		"Summary: latest summary",
		"Recent Runs",
		"run-new  failed  failed  none  latest summary",
		"run-old  completed  none  abc123  older summary",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("tui output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTUIRunnerReceivesRefreshOpenAddAndRetryActions(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-test
git:
  executable: git-test
verification:
  commands:
    - name: go
`)

	base := time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC)
	writeCLITaskFile(t, workDir, "010-blocked.md", cliTaskMarkdown("task-blocked", "blocked", "blocked tui", "Retry from TUI"))
	writeCLITaskFile(t, workDir, "010-visible.md", cliTaskMarkdown("task-visible", "pending", "Visible File Task", "visible file-backed task"))

	createValidationRun(t, workDir, validationRunSpec{
		RunID:              "run-new",
		TaskID:             "task-new",
		Task:               "new run",
		CompletedAt:        base.Add(time.Minute),
		CommitSHA:          "abc123",
		ChangedFiles:       []string{"internal/tui/model.go"},
		VerificationStatus: "passed",
		Verification: []receipt.VerificationEntry{{
			Command:  "go test ./...",
			ExitCode: 0,
			Status:   "passed",
		}},
		WriteArtifacts: true,
	})

	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		RunOnce: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
			if cfg.CodexProgress == nil {
				t.Fatal("tui run once config progress callback is nil")
			}
			cfg.CodexProgress(codexexec.ProgressEvent{Source: "codex", Message: "tui running"})
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-tui"},
				Task:    taskmodel.Task{ID: "task-tui"},
				Commit:  commit.Result{CommitSHA: "abc123"},
			}, nil
		},
		DoctorCommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			if command.Name == "codex-test" && reflect.DeepEqual(command.Args, []string{"--version"}) {
				return runner.Result{ExitCode: 0, Stdout: "codex-test 1.2.3\n"}
			}
			switch strings.Join(command.Args, "\x00") {
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
		},
		ExecutableLookPath: func(name string) (string, error) {
			switch name {
			case "codex-test":
				return "/fake/bin/codex-test", nil
			case "git-test":
				return "/fake/bin/git-test", nil
			default:
				return "", fmt.Errorf("executable %s not found", name)
			}
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
			if opts.AddTask == nil {
				t.Fatal("add task callback is nil")
			}
			if opts.RetryTask == nil {
				t.Fatal("retry task callback is nil")
			}
			if opts.ValidateReceipt == nil {
				t.Fatal("validate receipt callback is nil")
			}
			if opts.Preflight == nil {
				t.Fatal("preflight callback is nil")
			}
			if opts.RunOnce == nil {
				t.Fatal("run once callback is nil")
			}
			if opts.RunLoop == nil {
				t.Fatal("run loop callback is nil")
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
			if got, want := eventTypes(history.Events), []ledger.EventType{
				ledger.EventRunArtifacts,
				ledger.EventChangedFilesCaptured,
				ledger.EventVerificationCompleted,
				ledger.EventCommitCreated,
			}; !reflect.DeepEqual(got, want) {
				t.Fatalf("opened event types = %#v, want %#v", got, want)
			}

			validation, err := opts.ValidateReceipt("run-new")
			if err != nil {
				return err
			}
			if !validation.Passed() {
				t.Fatalf("receipt validation failed: %#v", validation.Failures())
			}

			preflight, err := opts.Preflight()
			if err != nil {
				return err
			}
			if !preflight.Ready {
				t.Fatalf("preflight ready = false, checks = %#v", preflight.Checks)
			}

			var progress []codexexec.ProgressEvent
			runResult, err := opts.RunOnce(context.Background(), func(event codexexec.ProgressEvent) {
				progress = append(progress, event)
			})
			if err != nil {
				return err
			}
			if got, want := runResult.Run.ID, "run-tui"; got != want {
				t.Fatalf("tui run result id = %q, want %q", got, want)
			}
			if len(progress) != 1 || progress[0].Message != "tui running" {
				t.Fatalf("tui run progress = %#v, want one event", progress)
			}

			var loopProgress []codexexec.ProgressEvent
			var passResults []runonce.Result
			loopResult, err := opts.RunLoop(context.Background(), 2, func(event codexexec.ProgressEvent) {
				loopProgress = append(loopProgress, event)
			}, func(result runonce.Result) error {
				passResults = append(passResults, result)
				return nil
			})
			if err != nil {
				return err
			}
			if got, want := loopResult.Stats.StopReason, "max_passes"; got != want {
				t.Fatalf("tui loop stop reason = %q, want %q", got, want)
			}
			if got, want := loopResult.Stats.Passes, 2; got != want {
				t.Fatalf("tui loop passes = %d, want %d", got, want)
			}
			if got, want := loopResult.Stats.Completed, 2; got != want {
				t.Fatalf("tui loop completed = %d, want %d", got, want)
			}
			if len(loopProgress) != 2 || loopProgress[0].Message != "tui running" || loopProgress[1].Message != "tui running" {
				t.Fatalf("tui loop progress = %#v, want two progress events", loopProgress)
			}
			if len(passResults) != 2 || passResults[0].Run.ID != "run-tui" || passResults[1].Run.ID != "run-tui" {
				t.Fatalf("tui loop pass results = %#v, want two run-tui results", passResults)
			}

			retried, err := opts.RetryTask("task-blocked")
			if err != nil {
				return err
			}
			if got, want := retried.Status, taskmodel.StatusPending; got != want {
				t.Fatalf("retried task status = %q, want %q", got, want)
			}
			if retried.Blocker != "" || retried.BlockedAt != nil {
				t.Fatalf("retried task = %+v, want blocker state cleared", retried)
			}

			refreshedAfterRetry, err := opts.RefreshStatus()
			if err != nil {
				return err
			}
			if got, want := taskIDSet(refreshedAfterRetry.Tasks), map[string]bool{"task-blocked": true, "task-visible": true}; !reflect.DeepEqual(got, want) {
				t.Fatalf("refreshed tasks after retry = %#v, want file-backed task ids %#v", got, want)
			}

			added, err := opts.AddTask(app.AddTaskInput{
				Task:    "  Add from TUI  ",
				Summary: "  tui add  ",
			})
			if err != nil {
				return err
			}
			if added.ID == "" {
				t.Fatal("added task id is empty")
			}
			if got, want := added.Task, cliTaskMarkdown(added.ID, "pending", "tui add", "Add from TUI"); got != want {
				t.Fatalf("added task text = %q, want %q", got, want)
			}
			if got, want := added.Summary, "tui add"; got != want {
				t.Fatalf("added task summary = %q, want %q", got, want)
			}

			refreshedAfterAdd, err := opts.RefreshStatus()
			if err != nil {
				return err
			}
			if got := taskIDSet(refreshedAfterAdd.Tasks); !got["task-blocked"] || !got["task-visible"] || !got[added.ID] || len(got) != 3 {
				t.Fatalf("refreshed tasks after add = %#v, want task-blocked, task-visible, and added task %s", got, added.ID)
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
		"Task files: .agent/tasks",
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
	taskFilesDir := filepath.Join(workDir, taskfile.TasksDir)
	assertDirExists(t, taskFilesDir)
	assertDirEntries(t, taskFilesDir, nil)
	seededAutonomousProfiles := make(map[string]bool)
	for _, template := range prompt.DefaultRunProfileTemplates() {
		assertProfileFileContent(t, workDir, template.Name, strings.TrimRight(template.Content, "\n")+"\n")
		seededAutonomousProfiles[template.Name] = true
	}
	for _, name := range []string{"supervisor", "planner", "corrector"} {
		if !seededAutonomousProfiles[name] {
			t.Fatalf("init did not seed autonomous profile %q", name)
		}
	}
	firstProfileContents := readProfileFileContents(t, workDir)
	assertLedgerStoreOpens(t, paths.LedgerDBPath)

	customTask := "# Existing Task\n\n## Goal\nKeep this file.\n"
	writeCLIFile(t, filepath.Join(taskFilesDir, "existing.md"), customTask)
	secondOut, err := executeCLI(t, workDir, "init")
	if err != nil {
		t.Fatalf("execute init second time: %v", err)
	}
	if secondOut != firstOut {
		t.Fatalf("second init output = %q, want %q", secondOut, firstOut)
	}
	if got := readProfileFileContents(t, workDir); !reflect.DeepEqual(got, firstProfileContents) {
		t.Fatalf("profile files changed after repeated init\nfirst:  %#v\nsecond: %#v", firstProfileContents, got)
	}
	assertFileContent(t, filepath.Join(taskFilesDir, "existing.md"), customTask)
	assertLedgerStoreOpens(t, paths.LedgerDBPath)
}

func TestInitDoesNotOverwriteExistingProfileFiles(t *testing.T) {
	workDir := t.TempDir()
	customProfiles := map[string]string{
		prompt.DefaultRunProfileName: "Custom implementer profile.\n",
		"supervisor":                 "Custom supervisor profile.\nKeep this byte-for-byte.\n",
		"planner":                    "Custom planner profile.\n",
		"corrector":                  "Custom corrector profile without a final newline.",
		"simplifier":                 "Custom simplifier profile.\n",
	}
	for name, content := range customProfiles {
		writeCLIFile(t, filepath.Join(workDir, prompt.RunProfileSourcePath(name)), content)
	}

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	for _, template := range prompt.DefaultRunProfileTemplates() {
		want, ok := customProfiles[template.Name]
		if !ok {
			want = strings.TrimRight(template.Content, "\n") + "\n"
		}
		assertProfileFileContent(t, workDir, template.Name, want)
	}
}

func TestInitCreatesOnlyMissingProfileFiles(t *testing.T) {
	workDir := t.TempDir()
	customProfiles := map[string]string{
		"supervisor": "Existing supervisor profile.\n",
		"auditor":    "Existing auditor profile.\r\nPreserve line endings.\r\n",
		"corrector":  "Existing corrector profile without a final newline.",
	}
	for name, content := range customProfiles {
		writeCLIFile(t, filepath.Join(workDir, prompt.RunProfileSourcePath(name)), content)
	}

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	for _, template := range prompt.DefaultRunProfileTemplates() {
		want, exists := customProfiles[template.Name]
		if !exists {
			want = strings.TrimRight(template.Content, "\n") + "\n"
		}
		assertProfileFileContent(t, workDir, template.Name, want)
	}
}

func TestInitAddsStateDirToLocalGitExclude(t *testing.T) {
	workDir := t.TempDir()
	runCLIGitCommand(t, workDir, "init", "-q")
	hardenCLIGitMetadata(t, filepath.Join(workDir, ".git"))
	excludePath := filepath.Join(workDir, ".git", "info", "exclude")
	if err := os.WriteFile(excludePath, []byte("# local excludes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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

func TestInitPreflightRejectsUnsafeComponentsWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{name: "state directory symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.Symlink(outside, filepath.Join(repo, ".revolvr")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "agent directory symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.Symlink(outside, filepath.Join(repo, ".agent")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "profiles directory symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.Mkdir(filepath.Join(repo, ".agent"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(repo, ".agent", "profiles")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tasks directory symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.Mkdir(filepath.Join(repo, ".agent"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(repo, taskfile.TasksDir)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "profile file symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.MkdirAll(filepath.Join(repo, ".agent", "profiles"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "sentinel"), filepath.Join(repo, prompt.RunProfileSourcePath("supervisor"))); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "ledger file symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.Mkdir(filepath.Join(repo, ".revolvr"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "sentinel"), filepath.Join(repo, ".revolvr", "ledger.sqlite")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "git symlink", setup: func(t *testing.T, repo, outside string) {
			if err := os.Symlink(outside, filepath.Join(repo, ".git")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "forged gitdir pointer", setup: func(t *testing.T, repo, outside string) {
			if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+outside+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "git exclude symlink", setup: func(t *testing.T, repo, outside string) {
			runCLIGitCommand(t, repo, "init", "-q")
			exclude := filepath.Join(repo, ".git", "info", "exclude")
			if err := os.Remove(exclude); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "sentinel"), exclude); err != nil {
				t.Fatal(err)
			}
			hardenCLIGitMetadata(t, filepath.Join(repo, ".git"))
		}},
		{name: "state path wrong type", setup: func(t *testing.T, repo, _ string) {
			if err := os.WriteFile(filepath.Join(repo, ".revolvr"), []byte("not-a-directory\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unsafe agent mode", setup: func(t *testing.T, repo, _ string) {
			path := filepath.Join(repo, ".agent")
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o777); err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, outside := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			test.setup(t, repo, outside)
			repoBefore := snapshotCLITree(t, repo)
			outsideBefore := snapshotCLITree(t, outside)

			if _, err := executeCLI(t, repo, "init"); err == nil {
				t.Fatal("execute init succeeded, want unsafe preflight failure")
			}
			if after := snapshotCLITree(t, repo); !reflect.DeepEqual(after, repoBefore) {
				t.Fatalf("repository changed after rejected init\nbefore: %v\nafter:  %v", repoBefore, after)
			}
			if after := snapshotCLITree(t, outside); !reflect.DeepEqual(after, outsideBefore) {
				t.Fatalf("outside tree changed after rejected init\nbefore: %v\nafter:  %v", outsideBefore, after)
			}
		})
	}
}

func TestEnsureExcludePatternRejectsOpenedPathSubstitutionWithoutOutsideMutation(t *testing.T) {
	repo, outside := t.TempDir(), t.TempDir()
	runCLIGitCommand(t, repo, "init", "-q")
	hardenCLIGitMetadata(t, filepath.Join(repo, ".git"))
	target, err := resolveGitExcludeTarget(context.Background(), repo)
	if err != nil || target == nil {
		t.Fatalf("resolve Git exclude target = %+v, %v", target, err)
	}
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("outside-authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideBefore := snapshotCLITree(t, outside)
	moved := target.Path + ".opened"
	err = ensureExcludePattern(target.Root, target.Path, revolvrGitExcludePattern, func() {
		if renameErr := os.Rename(target.Path, moved); renameErr != nil {
			t.Fatal(renameErr)
		}
		if linkErr := os.Symlink(sentinel, target.Path); linkErr != nil {
			t.Fatal(linkErr)
		}
	})
	if !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("exclude update error = %v, want substituted identity rejection", err)
	}
	if after := snapshotCLITree(t, outside); !reflect.DeepEqual(after, outsideBefore) {
		t.Fatalf("outside tree changed after exclude substitution\nbefore: %v\nafter:  %v", outsideBefore, after)
	}
}

func TestInitUsesCommonExcludeForGenuineLinkedWorktree(t *testing.T) {
	parent := t.TempDir()
	mainWorktree := filepath.Join(parent, "main")
	linkedWorktree := filepath.Join(parent, "linked")
	if err := os.Mkdir(mainWorktree, 0o755); err != nil {
		t.Fatal(err)
	}
	runCLIGitCommand(t, mainWorktree, "init", "-q")
	runCLIGitCommand(t, mainWorktree, "config", "user.name", "Revolvr Init")
	runCLIGitCommand(t, mainWorktree, "config", "user.email", "init@example.invalid")
	if err := os.WriteFile(filepath.Join(mainWorktree, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCLIGitCommand(t, mainWorktree, "add", "README.md")
	runCLIGitCommand(t, mainWorktree, "commit", "-q", "-m", "fixture")
	runCLIGitCommand(t, mainWorktree, "worktree", "add", "-q", linkedWorktree)
	hardenCLIGitMetadata(t, filepath.Join(mainWorktree, ".git"))
	if err := os.Chmod(filepath.Join(linkedWorktree, ".git"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := executeCLI(t, linkedWorktree, "init"); err != nil {
		t.Fatalf("execute linked-worktree init: %v", err)
	}
	if _, err := executeCLI(t, linkedWorktree, "init"); err != nil {
		t.Fatalf("execute linked-worktree init again: %v", err)
	}
	commonExclude := filepath.Join(mainWorktree, ".git", "info", "exclude")
	raw, err := os.ReadFile(commonExclude)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), revolvrGitExcludePattern); got != 1 {
		t.Fatalf("common exclude pattern count = %d, want 1; content:\n%s", got, raw)
	}
	gitDir := strings.TrimSpace(runCLIGitCommand(t, linkedWorktree, "rev-parse", "--absolute-git-dir"))
	if _, err := os.Lstat(filepath.Join(gitDir, "info", "exclude")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("per-worktree exclude unexpectedly exists: %v", err)
	}
	assertDirExists(t, filepath.Join(linkedWorktree, ".revolvr"))
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
	if got, want := task.Status, taskmodel.StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	wantTask := cliTaskMarkdown(task.ID, "pending", "Implement the CLI slice", "Implement the CLI slice")
	if got, want := task.Task, wantTask; got != want {
		t.Fatalf("task text = %q, want %q", got, want)
	}
	if got, want := task.Summary, "Implement the CLI slice"; got != want {
		t.Fatalf("task summary = %q, want %q", got, want)
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
	wantTask := cliTaskMarkdown(tasks[0].ID, "pending", "CLI bootstrap", "Implement init and task commands")
	if got, want := tasks[0].Task, wantTask; got != want {
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

func TestTaskImportHelpWorks(t *testing.T) {
	out, err := executeCLI(t, t.TempDir(), "task", "import", "--help")
	if err != nil {
		t.Fatalf("execute task import --help: %v", err)
	}

	for _, want := range []string{
		"Import tasks from a Markdown file",
		"Usage:\n  revolvr task import <path> [flags]",
		"--dry-run",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("task import help missing %q:\n%s", want, out)
		}
	}
}

func TestTaskImportDryRunReportsNumberedRowsWithoutMutatingState(t *testing.T) {
	workDir := t.TempDir()
	importPath := filepath.Join(workDir, "tasks.md")
	writeCLIFile(t, importPath, `
## Task: First import
Draft first task.

### Acceptance
- dry-run reports this row

## Task
Draft second task.

### Summary
Second import
`)

	out, err := executeCLI(t, workDir, "task", "import", "--dry-run", importPath)
	if err != nil {
		t.Fatalf("execute task import --dry-run: %v", err)
	}
	want := "Dry run: 2 task(s) would be imported.\n" +
		"1. First import - Draft first task. ### Acceptance - dry-run reports this row\n" +
		"2. Second import - Draft second task.\n"
	if out != want {
		t.Fatalf("task import dry-run output = %q, want %q", out, want)
	}
	assertNoCLIStateDir(t, workDir)
}

func TestTaskImportCreatesTasksInParsedOrderAndPrintsIDs(t *testing.T) {
	workDir := t.TempDir()
	importPath := filepath.Join(workDir, "tasks.md")
	writeCLIFile(t, importPath, `
## Task: First import
Create first task.

## Task: Second import
Create second task.
`)

	out, err := executeCLI(t, workDir, "task", "import", importPath)
	if err != nil {
		t.Fatalf("execute task import: %v", err)
	}

	tasks := readTasks(t, workDir)
	if got, want := len(tasks), 2; got != want {
		t.Fatalf("imported task count = %d, want %d", got, want)
	}
	if got, want := []string{tasks[0].Task, tasks[1].Task}, []string{
		cliTaskMarkdown(tasks[0].ID, "pending", "First import", "Create first task."),
		cliTaskMarkdown(tasks[1].ID, "pending", "Second import", "Create second task."),
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("imported task text = %#v, want %#v", got, want)
	}
	if got, want := []string{tasks[0].Summary, tasks[1].Summary}, []string{"First import", "Second import"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("imported summaries = %#v, want %#v", got, want)
	}
	want := fmt.Sprintf("Imported 2 task(s).\n1. %s\n2. %s\n", tasks[0].ID, tasks[1].ID)
	if out != want {
		t.Fatalf("task import output = %q, want %q", out, want)
	}
}

func TestTaskImportIgnoresFencedTaskHeading(t *testing.T) {
	workDir := t.TempDir()
	importPath := filepath.Join(workDir, "tasks.md")
	writeCLIFile(t, importPath, strings.Join([]string{
		"## Task: Parser documentation",
		"Document this example:",
		"",
		"```markdown",
		"## Task: not a real task",
		"example body",
		"```",
		"",
	}, "\n"))

	out, err := executeCLI(t, workDir, "task", "import", importPath)
	if err != nil {
		t.Fatalf("execute task import: %v", err)
	}
	tasks := readTasks(t, workDir)
	if got, want := len(tasks), 1; got != want {
		t.Fatalf("imported task count = %d, want %d", got, want)
	}
	if !strings.Contains(tasks[0].Task, "## Task: not a real task") {
		t.Fatalf("imported task lost fenced example:\n%s", tasks[0].Task)
	}
	want := fmt.Sprintf("Imported 1 task(s).\n1. %s\n", tasks[0].ID)
	if out != want {
		t.Fatalf("task import output = %q, want %q", out, want)
	}
}

func TestTaskImportParseErrorReturnsClearErrorWithoutMutatingState(t *testing.T) {
	workDir := t.TempDir()
	importPath := filepath.Join(workDir, "invalid.md")
	writeCLIFile(t, importPath, `
## Task: Invalid import
Create invalid task.

### Acceptance
- first acceptance

### Acceptance
- duplicate acceptance
`)

	_, err := executeCLI(t, workDir, "task", "import", importPath)
	if err == nil {
		t.Fatal("task import parse failure succeeded, want error")
	}
	if !strings.Contains(err.Error(), "task import: parse:") || !strings.Contains(err.Error(), "duplicate Acceptance") {
		t.Fatalf("task import parse error = %v, want duplicate Acceptance parse error", err)
	}
	assertNoCLIStateDir(t, workDir)
}

func TestTaskImportUnreadablePathReturnsClearErrorWithoutMutatingState(t *testing.T) {
	workDir := t.TempDir()
	missingPath := filepath.Join(workDir, "missing.md")

	_, err := executeCLI(t, workDir, "task", "import", missingPath)
	if err == nil {
		t.Fatal("task import unreadable path succeeded, want error")
	}
	if !strings.Contains(err.Error(), "task import: read "+missingPath) {
		t.Fatalf("task import unreadable path error = %v, want read error with path", err)
	}
	assertNoCLIStateDir(t, workDir)
}

func TestTaskListShowsFileBackedTasksInTaskfileOrder(t *testing.T) {
	workDir := t.TempDir()
	second := cliTaskMarkdownWithPhase("020-second", "pending", "Second task", "Second body.", taskfile.PhaseSimplify)
	writeCLITaskFile(t, workDir, "020-second.md", second)
	writeCLITaskFile(t, workDir, "010-first.md", "# First task\n\nFirst body.\n")

	out, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatalf("execute task list: %v", err)
	}
	want := "ID\tSTATUS\tWORKFLOW\tPHASE\tPROFILE\tNEXT\tSELECTED\tREADINESS\tWAITING_ON\tCONFLICT_BLOCKERS\tDIAGNOSTICS\tDEPENDS_ON\tTAGS\tCONFLICTS\tPARENT\tTASK\tSUMMARY\tCHECKPOINT\tCHECKPOINT_RECEIPT\n" +
		"010-first\tpending\tmixed-pass-v1\timplement\timplementer\taudit\tmixed-pass-v1\tready\t\t\t\t\t\t\t\t# First task First body.\tFirst task\t\t\n" +
		"020-second\tpending\tmixed-pass-v1\tsimplify\tsimplifier\tcompleted\t\tready\t\t\t\t\t\t\t\t" + oneLine(second) + "\tSecond task\t\t\n"
	if out != want {
		t.Fatalf("task list output = %q, want %q", out, want)
	}
}

func TestTaskListAndStatusRejectUnsupportedCanonicalFrontmatter(t *testing.T) {
	workDir := t.TempDir()
	writeCLITaskFile(t, workDir, "typo.md", `---
id: typo-task
status: pending
depend_on: prerequisite
---
# Typo Task
`)
	want := `unsupported frontmatter key "depend_on" at .agent/tasks/typo.md:4`

	if _, err := executeCLI(t, workDir, "task", "list"); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("task list error = %v, want %q", err, want)
	}
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	if _, err := executeCLI(t, workDir, "status"); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("status error = %v, want %q", err, want)
	}
}

func TestStatusAndTaskListRenderSharedWaitingSelectionWithoutFallback(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	writeCLITaskFile(t, workDir, "010-dependent.md", `---
id: task-dependent
status: pending
priority: 1
depends_on: task-prerequisite
---
# Dependent
`)
	writeCLITaskFile(t, workDir, "020-prerequisite.md", `---
id: task-prerequisite
status: pending
priority: 50
---
# Prerequisite
`)

	listOut, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatalf("execute task list: %v", err)
	}
	if !strings.Contains(listOut, "task-dependent\tpending\tmixed-pass-v1\timplement\timplementer\taudit\t\twaiting_dependency\ttask-prerequisite\t") {
		t.Fatalf("task list omitted exact waiting projection:\n%s", listOut)
	}
	if !strings.Contains(listOut, "task-prerequisite\tpending\tmixed-pass-v1\timplement\timplementer\taudit\tmixed-pass-v1\tready\t") {
		t.Fatalf("task list omitted selected prerequisite:\n%s", listOut)
	}

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	statusOut, err := executeCLI(t, workDir, "status")
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	for _, want := range []string{
		"Next task: task-prerequisite - Prerequisite\n",
		"Task readiness: task-dependent reason=waiting_dependency waiting_on=task-prerequisite conflict_blockers=none\n",
	} {
		if !strings.Contains(statusOut, want) {
			t.Fatalf("status output missing %q:\n%s", want, statusOut)
		}
	}
	if strings.Contains(statusOut, "Next task: task-dependent") {
		t.Fatalf("status fell back to waiting task:\n%s", statusOut)
	}

	status, err := app.Status(ctx, app.Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("load shared status projection: %v", err)
	}
	tuiOut := tuiapp.NewStatusModel(status).View()
	if !strings.Contains(tuiOut, "Next task: task-prerequisite - Prerequisite") {
		t.Fatalf("TUI omitted status-selected prerequisite:\n%s", tuiOut)
	}
	if strings.Contains(tuiOut, "Next task: task-dependent") {
		t.Fatalf("TUI fell back to waiting task:\n%s", tuiOut)
	}

	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
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
	if runResult.Task.ID != "task-prerequisite" || runResult.FileTask.ID != "task-prerequisite" || codexCalls != 1 {
		t.Fatalf("run selection task=%q file=%q codex_calls=%d, want shared prerequisite", runResult.Task.ID, runResult.FileTask.ID, codexCalls)
	}
}

func TestTaskListAndStatusRenderInvalidGraphDiagnostics(t *testing.T) {
	workDir := t.TempDir()
	writeCLITaskFile(t, workDir, "010-invalid.md", `---
id: task-invalid
status: pending
depends_on: task-missing
---
# Invalid
`)

	listOut, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatalf("execute task list: %v", err)
	}
	if !strings.Contains(listOut, "\tinvalid_graph\t") || !strings.Contains(listOut, `missing_dependency: task "task-invalid" has missing dependency "task-missing"`) {
		t.Fatalf("task list omitted invalid graph projection:\n%s", listOut)
	}
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	statusOut, err := executeCLI(t, workDir, "status")
	if err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if !strings.Contains(statusOut, "Next task: none\n") || !strings.Contains(statusOut, `Scheduling diagnostic: missing_dependency: task "task-invalid" has missing dependency "task-missing"`) {
		t.Fatalf("status omitted fail-closed diagnostic:\n%s", statusOut)
	}
}

func TestTaskListHandlesEmptyTaskList(t *testing.T) {
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
	writeCLITaskFile(t, workDir, "010-blocked.md", cliTaskMarkdown("task-blocked", "blocked", "blocked summary", "retry this task"))

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
	if task.ID != "task-blocked" || task.Task != cliTaskMarkdown("task-blocked", "pending", "blocked summary", "retry this task") || task.Summary != "blocked summary" {
		t.Fatalf("task after retry = %+v, want same file-backed task identity/text/summary", task)
	}
	if task.Status != taskmodel.StatusPending || task.Blocker != "" || task.BlockedAt != nil || task.CompletedAt != nil {
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
			tasks, err := app.ListTasks(ctx, app.Config{WorkDir: cfg.WorkingDir})
			if err != nil {
				return runonce.Result{}, err
			}
			var task taskmodel.Task
			for _, candidate := range tasks {
				if candidate.NextRunnable {
					task = candidate
					break
				}
			}
			if task.ID == "" {
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
	writeCLITaskFile(t, workDir, "010-done.md", cliTaskMarkdown("task-done", "completed", "done", "already done"))

	_, err := executeCLI(t, workDir, "task", "retry", "task-done")
	if err == nil {
		t.Fatal("task retry completed task succeeded, want error")
	}
	if !strings.Contains(err.Error(), `task "task-done" is not blocked (status: completed)`) {
		t.Fatalf("task retry error = %v, want not blocked", err)
	}
	tasks := readTasks(t, workDir)
	if len(tasks) != 1 || tasks[0].Status != taskmodel.StatusCompleted || tasks[0].Task != cliTaskMarkdown("task-done", "completed", "done", "already done") {
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
	writeCLITaskFile(t, workDir, "010-done.md", cliTaskMarkdown("task-done", "completed", "done", "already done"))

	_, err := executeCLI(t, workDir, "task", "unblock", "task-done")
	if err == nil {
		t.Fatal("task unblock completed task succeeded, want error")
	}
	if !strings.Contains(err.Error(), `task "task-done" is not blocked (status: completed)`) {
		t.Fatalf("task unblock error = %v, want not blocked", err)
	}
	tasks := readTasks(t, workDir)
	if len(tasks) != 1 || tasks[0].Status != taskmodel.StatusCompleted || tasks[0].Task != cliTaskMarkdown("task-done", "completed", "done", "already done") {
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
			if cfg.CodexExecutable != "codex" || cfg.CodexModel != "gpt-5.6-sol" || cfg.CodexReasoningEffort != "xhigh" || !cfg.CodexEphemeral || cfg.GitExecutable != "" || cfg.VerificationCommands != nil || cfg.AllowPreExistingDirty {
				t.Fatalf("run config = %+v, want no config-file overrides", cfg)
			}
			if !cfg.CodexBypassApprovalsAndSandbox {
				t.Fatal("codex bypass approvals and sandbox = false, want default true")
			}
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-1"},
				Task:    taskmodel.Task{ID: "task-1"},
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

func TestRunBareDefaultsToOnePassAndPropagatesRunnerFailure(t *testing.T) {
	t.Run("success output", func(t *testing.T) {
		var out bytes.Buffer
		calls := 0
		root := NewRootCommand(Options{
			Version: "test",
			Out:     &out,
			WorkDir: t.TempDir(),
			RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
				calls++
				return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
			},
		})
		root.SetArgs([]string{"run"})
		if err := root.Execute(); err != nil {
			t.Fatalf("bare run: %v", err)
		}
		if calls != 1 || out.String() != "No pending runnable tasks.\n" {
			t.Fatalf("bare run calls=%d output=%q", calls, out.String())
		}
	})

	t.Run("failure exit", func(t *testing.T) {
		var out bytes.Buffer
		root := NewRootCommand(Options{
			Version: "test",
			Out:     &out,
			WorkDir: t.TempDir(),
			RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
				return runonce.Result{}, fmt.Errorf("bare runner failed")
			},
		})
		root.SetArgs([]string{"run"})
		err := root.Execute()
		if err == nil || err.Error() != "bare runner failed" {
			t.Fatalf("bare run error = %v", err)
		}
		if out.Len() != 0 {
			t.Fatalf("failed bare run output = %q", out.String())
		}
	})
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
				Task:    taskmodel.Task{ID: "task-progress"},
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
  model: gpt-custom
  reasoning_effort: high
  ephemeral: true
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
	if got.CodexExecutable != "codex-custom" || got.CodexModel != "gpt-custom" || got.CodexReasoningEffort != "high" || !got.CodexEphemeral || got.CodexSandbox != "danger-full-access" || got.CodexApprovalPolicy != "on-request" || !got.CodexBypassApprovalsAndSandbox || got.CodexTimeout != 45*time.Second {
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
	configResult, configErr := checkRunConfig(workDir)
	if configErr != nil {
		t.Fatalf("check run config: %v", configErr)
	}

	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	want := "Config path: " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"Config found: false\n" +
		"Defaults: used\n" +
		"Codex executable: codex\n" +
		"Codex model: gpt-5.6-sol\n" +
		"Codex reasoning effort: xhigh\n" +
		"Codex session mode: ephemeral (ephemeral=true)\n" +
		"Codex dangerously bypass approvals and sandbox: true\n" +
		"Codex sandbox: workspace-write\n" +
		"Codex approval policy: never\n" +
		"Codex timeout: 0s\n" +
		"Effective config schema: " + configResult.EffectiveConfigSchema + "\n" +
		"Effective config SHA-256: " + configResult.EffectiveConfigSHA256 + "\n" +
		defaultAutonomyConfigOutput() +
		defaultRetentionConfigOutput() +
		defaultNotificationConfigOutput() +
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
	configResult, configErr := checkRunConfig(workDir)
	if configErr != nil {
		t.Fatalf("check run config: %v", configErr)
	}

	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	want := "Config path: " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"Config found: false\n" +
		"Defaults: used\n" +
		"Codex executable: codex\n" +
		"Codex model: gpt-5.6-sol\n" +
		"Codex reasoning effort: xhigh\n" +
		"Codex session mode: ephemeral (ephemeral=true)\n" +
		"Codex dangerously bypass approvals and sandbox: true\n" +
		"Codex sandbox: workspace-write\n" +
		"Codex approval policy: never\n" +
		"Codex timeout: 0s\n" +
		"Effective config schema: " + configResult.EffectiveConfigSchema + "\n" +
		"Effective config SHA-256: " + configResult.EffectiveConfigSHA256 + "\n" +
		defaultAutonomyConfigOutput() +
		defaultRetentionConfigOutput() +
		defaultNotificationConfigOutput() +
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

func TestConfigCheckExplicitEmptyVerificationCommandsSuppressesGoDefault(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, "go.mod"), "module example.com/revolvrtest\n")
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), "verification:\n  commands: []\n")

	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatalf("execute config check: %v", err)
	}
	if !strings.Contains(out, "Config found: true\n") || !strings.Contains(out, "Verification command count: 0\n") {
		t.Fatalf("explicit empty config output = %q", out)
	}
	if strings.Contains(out, "Verification command 0:") {
		t.Fatalf("explicit empty config synthesized a command: %q", out)
	}
}

func TestConfigCheckValidConfigPrintsEffectiveValuesAndDoesNotRunOnce(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-custom
  model: gpt-override
  reasoning_effort: low
  ephemeral: true
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
	configResult, configErr := checkRunConfig(workDir)
	if configErr != nil {
		t.Fatalf("check run config: %v", configErr)
	}

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
		"Codex model: gpt-override\n" +
		"Codex reasoning effort: low\n" +
		"Codex session mode: ephemeral (ephemeral=true)\n" +
		"Codex dangerously bypass approvals and sandbox: true\n" +
		"Codex sandbox: danger-full-access\n" +
		"Codex approval policy: on-request\n" +
		"Codex timeout: 45s\n" +
		"Effective config schema: " + configResult.EffectiveConfigSchema + "\n" +
		"Effective config SHA-256: " + configResult.EffectiveConfigSHA256 + "\n" +
		defaultAutonomyConfigOutput() +
		defaultRetentionConfigOutput() +
		defaultNotificationConfigOutput() +
		"Git executable: git-custom\n" +
		"Git timeout: 12s\n" +
		"Verification missing policy: pass\n" +
		"Verification command count: 1\n" +
		"Verification command 0: name=go args=[\"test\", \"./...\"] dir=internal timeout=9s\n" +
		"Commit allow pre-existing dirty: false\n" +
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

func TestConfigCheckRejectsPreExistingDirtyOption(t *testing.T) {
	workDir := t.TempDir()
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
commit:
  allow_pre_existing_dirty: true
`)

	_, err := executeCLI(t, workDir, "config", "check")
	if err == nil || !strings.Contains(err.Error(), "allow_pre_existing_dirty must be false") || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("config check error = %v, want removed dirty-worktree option error", err)
	}
}

func TestConfigCheckRendersSecretSourcesWithoutValues(t *testing.T) {
	workDir := t.TempDir()
	secret := "never-persist-this-token"
	t.Setenv("REVOLVR_TEST_TOKEN", secret)
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), "autonomy:\n  redaction:\n    environment_variables: [REVOLVR_TEST_TOKEN]\n")
	out, err := executeCLI(t, workDir, "config", "check")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, secret) || !strings.Contains(out, "environment_variables=[\"REVOLVR_TEST_TOKEN\"]") {
		t.Fatalf("config output = %q", out)
	}
}

func defaultAutonomyConfigOutput() string {
	return "Autonomy safety schema: revolvr-autonomous-safety-declaration-v1\n" +
		"Autonomy mode: operator_attended\n" +
		"Worktree isolation: Git/source isolation only; not a security sandbox\n" +
		"External isolation: expectation=none enforcement=none attestation=false\n" +
		"Network policy: access=unknown enforcement=none attestation=false\n" +
		"Git hooks policy: operator_attended trusted=0\n" +
		"Environment policy: inherit_host=true allow=[]\n" +
		"Secret redaction sources: environment_variables=[]\n" +
		"Fully unattended acknowledgement present: false\n" +
		"Queue policy schema: autonomous-queue-policy-v1\n" +
		"Queue maximum workers: 1\n"
}

func defaultRetentionConfigOutput() string {
	return "Retention policy schema: revolvr-artifact-retention-policy-v1\n" +
		"Retention mutation enabled: false\n" +
		"Retention recent run count: 20\n" +
		"Retention ages: compress=168h0m0s prune=2160h0m0s\n" +
		"Retention classes: codex_jsonl=true codex_stderr=true prune_compressed=false\n" +
		"Retention verified export required: true\n" +
		"Retention operation bounds: files=100 bytes=1073741824\n"
}

func defaultNotificationConfigOutput() string {
	return "Notification policy schema: revolvr-notification-policy-v1\n" +
		"Notifications enabled: false\n" +
		"Notification events: []\n" +
		"Notification executable: \n" +
		"Notification argument count: 0\n" +
		"Notification directory: \n" +
		"Notification environment names: []\n" +
		"Notification bounds: timeout=0s stdout=0 stderr=0 attempts=0 retry_delay=0s\n"
}

func TestArtifactGCDryRunIsDefaultAndLedgerExportCommands(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatal(err)
	}
	out, err := executeCLI(t, workDir, "artifact", "gc", "--operation-id", "dry-one", "--planned-at", "2026-07-12T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Artifact GC dry-run\nOperation ID: dry-one\nPlan ID:") || !strings.Contains(out, "mutation_enabled=false") || !strings.Contains(out, "compress=0 prune=0") {
		t.Fatalf("dry-run output=%q", out)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".revolvr", "retention")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created retention state: %v", err)
	}
	exported, err := executeCLI(t, workDir, "ledger", "export", "--operation-id", "export-one", "--exported-at", "2026-07-12T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(exported)
	if len(fields) < 3 {
		t.Fatalf("export output=%q", exported)
	}
	exportID := fields[2]
	verified, err := executeCLI(t, workDir, "ledger", "export", "verify", exportID)
	if err != nil {
		t.Fatalf("verify: %v output=%q", err, verified)
	}
	if !strings.Contains(verified, "Ledger export verification: true") {
		t.Fatalf("verify output=%q", verified)
	}
	replayed, err := executeCLI(t, workDir, "ledger", "export", "replay-validate", exportID)
	if err != nil {
		t.Fatalf("replay: %v output=%q", err, replayed)
	}
	if !strings.Contains(replayed, "passed=true") {
		t.Fatalf("replay output=%q", replayed)
	}
}

func TestArtifactGCApplyRequiresExactPlan(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatal(err)
	}
	_, err := executeCLI(t, workDir, "artifact", "gc", "--operation-id", "apply-one", "--planned-at", "2026-07-12T12:00:00Z", "--apply", "--plan-id", "wrong")
	if err == nil || !strings.Contains(err.Error(), "exact --plan-id") {
		t.Fatalf("apply error=%v", err)
	}
}

func TestArtifactGCApplyAndResumePreserveOperationAndResultWriteErrors(t *testing.T) {
	operationErr := errors.New("artifact GC operation failed")
	writeErr := errors.New("artifact GC result write failed")
	plan := artifactretention.Plan{OperationID: "operation-one", PlanID: "plan-one"}
	result := artifactretention.ApplyResult{Journal: artifactretention.Journal{
		OperationID:    plan.OperationID,
		Stage:          "resumable",
		CompletedPaths: []string{".revolvr/runs/run-one/codex.jsonl"},
	}}

	tests := []struct {
		name         string
		operation    string
		operationErr error
		args         []string
	}{
		{name: "successful apply", operation: "apply", args: []string{"artifact", "gc", "--operation-id", plan.OperationID, "--planned-at", "2026-07-12T12:00:00Z", "--apply", "--plan-id", plan.PlanID}},
		{name: "failed apply", operation: "apply", operationErr: operationErr, args: []string{"artifact", "gc", "--operation-id", plan.OperationID, "--planned-at", "2026-07-12T12:00:00Z", "--apply", "--plan-id", plan.PlanID}},
		{name: "successful resume", operation: "resume", args: []string{"artifact", "gc", "--operation-id", plan.OperationID, "--apply", "--resume"}},
		{name: "failed resume", operation: "resume", operationErr: operationErr, args: []string{"artifact", "gc", "--operation-id", plan.OperationID, "--apply", "--resume"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := &gcResultFailWriter{err: writeErr}
			applyCalls, resumeCalls := 0, 0
			root := NewRootCommand(Options{
				Version: "test",
				Out:     out,
				WorkDir: t.TempDir(),
				PlanArtifactGC: func(context.Context, app.Config, app.GCPlanInput) (artifactretention.Plan, error) {
					return plan, nil
				},
				ApplyArtifactGC: func(context.Context, app.Config, app.GCApplyInput) (artifactretention.ApplyResult, error) {
					applyCalls++
					return result, tc.operationErr
				},
				ResumeArtifactGC: func(context.Context, app.Config, string) (artifactretention.ApplyResult, error) {
					resumeCalls++
					return result, tc.operationErr
				},
			})
			root.SetArgs(tc.args)

			err := root.Execute()
			if !errors.Is(err, writeErr) {
				t.Fatalf("execute error = %v, want result write error", err)
			}
			if tc.operationErr != nil && !errors.Is(err, operationErr) {
				t.Fatalf("execute error = %v, want joined operation error", err)
			}
			if tc.operationErr == nil && errors.Is(err, operationErr) {
				t.Fatalf("execute error = %v, unexpectedly contains operation error", err)
			}
			if tc.operation == "apply" && (applyCalls != 1 || resumeCalls != 0) {
				t.Fatalf("operation calls: apply=%d resume=%d, want apply=1 resume=0", applyCalls, resumeCalls)
			}
			if tc.operation == "resume" && (applyCalls != 0 || resumeCalls != 1) {
				t.Fatalf("operation calls: apply=%d resume=%d, want apply=0 resume=1", applyCalls, resumeCalls)
			}
		})
	}
}

type gcResultFailWriter struct {
	bytes.Buffer
	err error
}

func (w *gcResultFailWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("GC result:")) {
		return 0, w.err
	}
	return w.Buffer.Write(p)
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
						Task:    taskmodel.Task{ID: "task-1"},
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
					Task:    taskmodel.Task{ID: "task-1"},
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
				Task:    taskmodel.Task{ID: "task-failed"},
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
				Task:    taskmodel.Task{ID: fmt.Sprintf("task-blocked-%d", calls)},
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
				Task:           taskmodel.Task{ID: "task-dirty-failure"},
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
				Task:    taskmodel.Task{ID: fmt.Sprintf("task-%d", calls)},
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
		"Timeline:\n" +
		"TIMESTAMP\tPHASE\tSTATUS\tDETAIL\n" +
		"2026-06-26T12:00:01Z\trun\tstarted\trun run-show\n" +
		"2026-06-26T12:02:00Z\trun\tcompleted\trun completed\n" +
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
		ContextPayloadPath:   ".revolvr/runs/run-artifacts/context.md",
		ContextManifestPath:  ".revolvr/runs/run-artifacts/context.json",
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
		"Timeline:\n" +
		"TIMESTAMP\tPHASE\tSTATUS\tDETAIL\n" +
		"2026-06-26T13:00:01Z\trun\tstarted\trun run-artifacts\n" +
		"2026-06-26T13:01:00Z\trun\tfailed\trun failed\n" +
		"Artifacts:\n" +
		"context payload: .revolvr/runs/run-artifacts/context.md\n" +
		"context manifest: .revolvr/runs/run-artifacts/context.json\n" +
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
		"Timeline:\n" +
		"TIMESTAMP\tPHASE\tSTATUS\tDETAIL\n" +
		"2026-06-26T14:00:00Z\trun\tstarted\trun run-empty-artifacts, task task-empty-artifacts\n" +
		"none\trun\tfailed\trun failed\n" +
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
		"Timeline:\n" +
		"TIMESTAMP\tPHASE\tSTATUS\tDETAIL\n" +
		"2026-06-26T15:00:00Z\trun\tstarted\trun run-diagnostics\n" +
		"2026-06-26T15:00:01Z\tcodex\tcompleted\texit_code=0\n" +
		"2026-06-26T15:00:02Z\tchanges\tcaptured\t1 changed file: internal/broken.go\n" +
		"2026-06-26T15:00:03Z\tverification\tfailed\tfailed command 0: go test ./... (exit_code=1)\n" +
		"2026-06-26T15:00:04Z\treceipt\tsynthesized\tverification_failed (.revolvr/receipts/run-diagnostics.md)\n" +
		"2026-06-26T15:00:30Z\trun\tfailed\toutcome=verification_failed: verification command 0 failed\n" +
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
		"Timeline:\n" +
		"TIMESTAMP\tPHASE\tSTATUS\tDETAIL\n" +
		"2026-06-26T16:00:00Z\trun\tstarted\trun run-warning, task task-warning\n" +
		"2026-06-26T16:00:00Z\treceipt\twarning\tchanged_files_mismatch: receipt changed files differ from harness captured changed files (.revolvr/receipts/run-warning.md)\n" +
		"none\trun\tfailed\trun failed\n" +
		"Diagnostics:\n" +
		"warning: changed_files_mismatch: receipt changed files differ from harness captured changed files (.revolvr/receipts/run-warning.md)\n" +
		"Events:\n" +
		"ID\tTYPE\tTIMESTAMP\n" +
		"1\treceipt_warning\t2026-06-26T16:00:00Z\n"
	if out != want {
		t.Fatalf("show output = %q, want %q", out, want)
	}
}

func TestWriteTimelinePrintsNoRows(t *testing.T) {
	var out bytes.Buffer
	if err := writeTimeline(&out, nil); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	want := "Timeline:\nNo timeline rows.\n"
	if got := out.String(); got != want {
		t.Fatalf("timeline output = %q, want %q", got, want)
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
		"verdict: ok\n" +
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

func assertDirEntries(t *testing.T, path string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read dir %s: %v", path, err)
	}
	if want == nil {
		want = []string{}
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dir entries for %s = %#v, want %#v", path, got, want)
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

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := string(content); got != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func assertProfileFileContent(t *testing.T, workDir string, name string, want string) {
	t.Helper()
	path := filepath.Join(workDir, prompt.RunProfileSourcePath(name))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile %s: %v", path, err)
	}
	if got := string(content); got != want {
		t.Fatalf("profile %s content = %q, want %q", name, got, want)
	}
}

func readProfileFileContents(t *testing.T, workDir string) map[string]string {
	t.Helper()
	contents := make(map[string]string)
	for _, template := range prompt.DefaultRunProfileTemplates() {
		path := filepath.Join(workDir, prompt.RunProfileSourcePath(template.Name))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read profile %s: %v", path, err)
		}
		contents[template.Name] = string(raw)
	}
	return contents
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

func runCLIGitCommand(t *testing.T, workDir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	allArgs := append([]string{"-C", workDir}, args...)
	cmd := exec.Command("git", allArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(allArgs, " "), err, output)
	}
	return string(output)
}

func hardenCLIGitMetadata(t *testing.T, gitDir string) {
	t.Helper()
	if err := filepath.WalkDir(gitDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.Chmod(path, info.Mode().Perm()&^0o022)
	}); err != nil {
		t.Fatalf("harden Git metadata: %v", err)
	}
}

func snapshotCLITree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			snapshot[key] = fmt.Sprintf("symlink:%04o:%s", info.Mode().Perm(), target)
		case entry.IsDir():
			snapshot[key] = fmt.Sprintf("dir:%04o", info.Mode().Perm())
		case info.Mode().IsRegular():
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			snapshot[key] = fmt.Sprintf("file:%04o:%d:%x", info.Mode().Perm(), len(raw), sha256.Sum256(raw))
		default:
			snapshot[key] = fmt.Sprintf("other:%s:%04o", info.Mode().Type(), info.Mode().Perm())
		}
		return nil
	}); err != nil {
		t.Fatalf("snapshot tree %s: %v", root, err)
	}
	return snapshot
}

func writeCLITaskFile(t *testing.T, workDir string, name string, content string) {
	t.Helper()
	writeCLIFile(t, filepath.Join(workDir, taskfile.TasksDir, name), content)
}

func cliTaskMarkdown(id string, status string, title string, body string) string {
	return fmt.Sprintf(`---
id: %s
status: %s
---
# %s

%s
`, id, status, title, body)
}

func cliTaskMarkdownWithPhase(id string, status string, title string, body string, phase string) string {
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

func taskIDSet(tasks []taskmodel.Task) map[string]bool {
	ids := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		ids[task.ID] = true
	}
	return ids
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
		ContextPayloadPath:   filepath.Join(".revolvr", "runs", spec.RunID, "context.md"),
		ContextManifestPath:  filepath.Join(".revolvr", "runs", spec.RunID, "context.json"),
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
		writeCLIFile(t, filepath.Join(workDir, artifactPaths.ContextPayloadPath), "context payload")
		writeCLIFile(t, filepath.Join(workDir, artifactPaths.ContextManifestPath), "{}\n")
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

func readTasks(t *testing.T, workDir string) []taskmodel.Task {
	t.Helper()
	tasks, err := app.ListTasks(context.Background(), app.Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	return tasks
}

func assertNoCLIStateDir(t *testing.T, workDir string) {
	t.Helper()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	if _, err := os.Stat(paths.StateDir); !os.IsNotExist(err) {
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
