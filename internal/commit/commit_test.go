package commit

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/runner"
	"revolvr/internal/verification"
)

func TestRunCommitsChangedFilesAndRecordsSHA(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer store.Close()
	run, err := store.CreateRun(ctx, ledger.RunSpec{
		ID:     "run-commit",
		TaskID: "task-commit",
		Task:   "add auto commit",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	var calls []runner.Command
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		calls = append(calls, command)
		switch {
		case reflect.DeepEqual(command.Args, []string{"add", "--", "a.go", "b.go"}):
			return runner.Result{ExitCode: 0}
		case reflect.DeepEqual(command.Args, []string{"commit", "-m", "Add auto commit gate", "-m", "Run-ID: run-commit\nTask-ID: task-commit\nVerification: passed"}):
			return runner.Result{ExitCode: 0, Stdout: "[main abc123] Add auto commit gate\n"}
		case reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "HEAD"}):
			return runner.Result{ExitCode: 0, Stdout: "abc123def456\n"}
		default:
			t.Fatalf("unexpected git command: %#v", command.Args)
			return runner.Result{ExitCode: 2}
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir:         workDir,
		RunID:              run.ID,
		TaskID:             "task-commit",
		TaskSummary:        "Add auto commit gate",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &gitstate.Capture{ChangedFiles: []string{"b.go", "a.go", "a.go"}},
		GitExecutable:      "git-test",
		Timeout:            2 * time.Second,
		StdoutCap:          123,
		StderrCap:          45,
		Ledger:             store,
		CommandRunner:      fakeRunner,
	})
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}

	if result.Status != StatusCommitted {
		t.Fatalf("status = %s, want committed", result.Status)
	}
	if result.CommitSHA != "abc123def456" {
		t.Fatalf("commit sha = %q, want abc123def456", result.CommitSHA)
	}
	if got, want := result.ChangedFiles, []string{"a.go", "b.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed files = %#v, want %#v", got, want)
	}
	if len(calls) != 3 {
		t.Fatalf("git call count = %d, want 3", len(calls))
	}
	absWorkDir := mustAbs(t, workDir)
	for _, call := range calls {
		if call.Name != "git-test" {
			t.Fatalf("git executable = %q, want git-test", call.Name)
		}
		if call.Dir != absWorkDir {
			t.Fatalf("git dir = %q, want %q", call.Dir, absWorkDir)
		}
		if call.Timeout != 2*time.Second || call.StdoutLimit != 123 || call.StderrLimit != 45 {
			t.Fatalf("git limits = timeout %s stdout %d stderr %d", call.Timeout, call.StdoutLimit, call.StderrLimit)
		}
	}
	commitArgs := calls[1].Args
	if !strings.Contains(commitArgs[4], "Run-ID: run-commit") || !strings.Contains(commitArgs[4], "Task-ID: task-commit") {
		t.Fatalf("commit body = %q, want run id and task id", commitArgs[4])
	}

	stored, ok, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if !ok {
		t.Fatal("stored run not found")
	}
	if stored.CommitSHA != "abc123def456" {
		t.Fatalf("stored commit sha = %q, want abc123def456", stored.CommitSHA)
	}

	history, ok, err := store.GetRunWithEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	gotTypes := eventTypes(history.Events)
	wantTypes := []ledger.EventType{ledger.EventCommitStarted, ledger.EventCommitCreated}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("ledger event types = %#v, want %#v", gotTypes, wantTypes)
	}
	var created map[string]any
	if err := json.Unmarshal(history.Events[1].Payload, &created); err != nil {
		t.Fatalf("unmarshal commit_created payload: %v", err)
	}
	if created["commit_sha"] != "abc123def456" {
		t.Fatalf("commit_created payload = %#v", created)
	}
}

func TestRunRefusesWhenVerificationFails(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.VerificationResult = &verification.Result{
			Status:             verification.StatusFailed,
			Passed:             false,
			FailedCommandIndex: 0,
			Commands: []verification.CommandResult{
				{Index: 0, Command: "go test ./...", Status: verification.StatusFailed, Passed: false, ExitCode: 1},
			},
		}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonVerificationFailed {
		t.Fatalf("result = %+v, want verification refusal", result)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRefusesWhenNoChanges(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.PostRunChanged = &gitstate.Capture{}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonNoChanges {
		t.Fatalf("result = %+v, want no-changes refusal", result)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRefusesWhenCodexFails(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.CodexResult = &codexexec.Result{ExitCode: 1, Err: errors.New("codex failed")}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonCodexFailed {
		t.Fatalf("result = %+v, want codex refusal", result)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRefusesPreExistingDirtyFilesByDefault(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.PreRunDirty = &gitstate.Capture{DirtyFiles: []string{"manual.go"}}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonPreExistingDirty {
		t.Fatalf("result = %+v, want pre-existing dirty refusal", result)
	}
	if got, want := result.PreExistingDirtyFiles, []string{"manual.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-existing files = %#v, want %#v", got, want)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRequiresInputs(t *testing.T) {
	_, err := Run(context.Background(), Config{
		WorkingDir: t.TempDir(),
		RunID:      "run-1",
		TaskID:     "task-1",
	})
	if err == nil || !strings.Contains(err.Error(), "task summary is required") {
		t.Fatalf("error = %v, want task summary requirement", err)
	}
}

func baseConfig(t *testing.T, fakeRunner CommandRunner, mutate func(*Config)) Config {
	t.Helper()
	cfg := Config{
		WorkingDir:         t.TempDir(),
		RunID:              "run-1",
		TaskID:             "task-1",
		TaskSummary:        "Do the task",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &gitstate.Capture{ChangedFiles: []string{"changed.go"}},
		CommandRunner:      fakeRunner,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func passedVerification() *verification.Result {
	return &verification.Result{
		Status:             verification.StatusPassed,
		Passed:             true,
		FailedCommandIndex: -1,
		Commands: []verification.CommandResult{
			{
				Index:    0,
				Command:  "go test ./...",
				Name:     "go",
				Args:     []string{"test", "./..."},
				Status:   verification.StatusPassed,
				Passed:   true,
				ExitCode: 0,
			},
		},
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve absolute path: %v", err)
	}
	return abs
}

func eventTypes(events []ledger.Event) []ledger.EventType {
	out := make([]ledger.EventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}
