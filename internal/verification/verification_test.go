package verification

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/runner"
)

func TestRunExecutesCommandsInOrder(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	var calls []runner.Command
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		calls = append(calls, command)
		return runner.Result{
			ExitCode: 0,
			Stdout:   "ok\n",
			Stderr:   "note\n",
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir: workDir,
		Commands: []Command{
			{Name: "go", Args: []string{"test", "./..."}},
			{Name: "npm", Args: []string{"test", "--", "unit suite"}, Dir: "web", Timeout: 5 * time.Second, StdoutCap: 99, StderrCap: 88},
		},
		Timeout:       2 * time.Second,
		StdoutCap:     123,
		StderrCap:     45,
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("run verification: %v", err)
	}

	absWorkDir := mustAbs(t, workDir)
	if result.Status != StatusPassed || !result.Passed {
		t.Fatalf("result status = %s passed=%v, want passed", result.Status, result.Passed)
	}
	if result.FailedCommandIndex != -1 {
		t.Fatalf("failed command index = %d, want -1", result.FailedCommandIndex)
	}
	if len(calls) != 2 {
		t.Fatalf("runner call count = %d, want 2", len(calls))
	}
	if got, want := []string{calls[0].Name, calls[1].Name}, []string{"go", "npm"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runner command order = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(calls[0].Args, []string{"test", "./..."}) {
		t.Fatalf("first args = %#v", calls[0].Args)
	}
	if calls[0].Dir != absWorkDir {
		t.Fatalf("first dir = %q, want %q", calls[0].Dir, absWorkDir)
	}
	if calls[0].Timeout != 2*time.Second || calls[0].StdoutLimit != 123 || calls[0].StderrLimit != 45 {
		t.Fatalf("first limits = timeout %s stdout %d stderr %d", calls[0].Timeout, calls[0].StdoutLimit, calls[0].StderrLimit)
	}
	if calls[1].Dir != filepath.Join(absWorkDir, "web") {
		t.Fatalf("second dir = %q, want web subdir", calls[1].Dir)
	}
	if calls[1].Timeout != 5*time.Second || calls[1].StdoutLimit != 99 || calls[1].StderrLimit != 88 {
		t.Fatalf("second limits = timeout %s stdout %d stderr %d", calls[1].Timeout, calls[1].StdoutLimit, calls[1].StderrLimit)
	}
	if got, want := result.Commands[0].Command, "go test ./..."; got != want {
		t.Fatalf("first command string = %q, want %q", got, want)
	}
	if got, want := result.Commands[1].Command, `npm test -- "unit suite"`; got != want {
		t.Fatalf("second command string = %q, want %q", got, want)
	}
}

func TestRunStopsAfterFirstFailingCommand(t *testing.T) {
	ctx := context.Background()
	var calls []runner.Command
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		calls = append(calls, command)
		if len(calls) == 2 {
			return runner.Result{ExitCode: 7, Stderr: "failed\n"}
		}
		return runner.Result{ExitCode: 0, Stdout: "ok\n"}
	}

	result, err := Run(ctx, Config{
		WorkingDir: t.TempDir(),
		Commands: []Command{
			{Name: "first"},
			{Name: "second"},
			{Name: "third"},
		},
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("run verification: %v", err)
	}

	if result.Status != StatusFailed || result.Passed {
		t.Fatalf("result status = %s passed=%v, want failed", result.Status, result.Passed)
	}
	if result.FailedCommandIndex != 1 {
		t.Fatalf("failed command index = %d, want 1", result.FailedCommandIndex)
	}
	if got, want := len(calls), 2; got != want {
		t.Fatalf("runner call count = %d, want %d", got, want)
	}
	if len(result.Commands) != 2 {
		t.Fatalf("command result count = %d, want 2", len(result.Commands))
	}
	if result.Commands[0].Status != StatusPassed || result.Commands[1].Status != StatusFailed {
		t.Fatalf("command statuses = %s, %s", result.Commands[0].Status, result.Commands[1].Status)
	}
	if result.Commands[1].ExitCode != 7 || result.Commands[1].Stderr.Content != "failed\n" {
		t.Fatalf("failing command result = %+v", result.Commands[1])
	}
}

func TestRunCapturesTimeoutAndOutputMetadata(t *testing.T) {
	ctx := context.Background()
	runErr := context.DeadlineExceeded
	fakeRunner := func(_ context.Context, _ runner.Command) runner.Result {
		return runner.Result{
			ExitCode:             -1,
			Err:                  runErr,
			TimedOut:             true,
			Stdout:               "partial stdout",
			Stderr:               "partial stderr",
			StdoutTruncatedBytes: 10,
			StderrTruncatedBytes: 20,
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir:    t.TempDir(),
		Commands:      []Command{{Name: "slow"}},
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("run verification: %v", err)
	}

	if result.Status != StatusFailed || result.FailedCommandIndex != 0 {
		t.Fatalf("result = %+v, want first command failure", result)
	}
	command := result.Commands[0]
	if !command.TimedOut {
		t.Fatal("timed out = false, want true")
	}
	if !errors.Is(command.Err, context.DeadlineExceeded) || command.Error != context.DeadlineExceeded.Error() {
		t.Fatalf("command error = %v / %q, want deadline exceeded", command.Err, command.Error)
	}
	if command.Stdout.Content != "partial stdout" || command.Stdout.TruncatedBytes != 10 {
		t.Fatalf("stdout capture = %+v", command.Stdout)
	}
	if command.Stderr.Content != "partial stderr" || command.Stderr.TruncatedBytes != 20 {
		t.Fatalf("stderr capture = %+v", command.Stderr)
	}
}

func TestRunWritesVerificationLedgerEvents(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer store.Close()
	run, err := store.CreateRun(ctx, ledger.RunSpec{
		ID:     "run-verify",
		TaskID: "task-verify",
		Task:   "verify the run",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	fakeRunner := func(_ context.Context, _ runner.Command) runner.Result {
		return runner.Result{
			ExitCode:             0,
			Stdout:               "stdout\n",
			Stderr:               "stderr\n",
			StdoutTruncatedBytes: 3,
			StderrTruncatedBytes: 4,
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir: workDir,
		Commands: []Command{
			{Name: "go", Args: []string{"test", "./..."}},
		},
		RunID:         run.ID,
		Ledger:        store,
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("run verification: %v", err)
	}
	if result.LedgerError != nil {
		t.Fatalf("ledger error: %v", result.LedgerError)
	}

	history, ok, err := store.GetRunWithEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run was not found")
	}
	gotTypes := eventTypes(history.Events)
	wantTypes := []ledger.EventType{ledger.EventVerificationStarted, ledger.EventVerificationCompleted}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", gotTypes, wantTypes)
	}

	var started map[string]any
	if err := json.Unmarshal(history.Events[0].Payload, &started); err != nil {
		t.Fatalf("unmarshal started payload: %v", err)
	}
	if started["command_count"] != float64(1) {
		t.Fatalf("started payload = %#v", started)
	}

	var completed map[string]any
	if err := json.Unmarshal(history.Events[1].Payload, &completed); err != nil {
		t.Fatalf("unmarshal completed payload: %v", err)
	}
	if completed["status"] != string(StatusPassed) || completed["passed"] != true {
		t.Fatalf("completed payload = %#v", completed)
	}
	commands, ok := completed["commands"].([]any)
	if !ok || len(commands) != 1 {
		t.Fatalf("completed commands = %#v", completed["commands"])
	}
	command, ok := commands[0].(map[string]any)
	if !ok {
		t.Fatalf("command payload = %#v", commands[0])
	}
	stdout := command["stdout"].(map[string]any)
	stderr := command["stderr"].(map[string]any)
	if stdout["content"] != "stdout\n" || stdout["truncated_bytes"] != float64(3) {
		t.Fatalf("stdout payload = %#v", stdout)
	}
	if stderr["content"] != "stderr\n" || stderr["truncated_bytes"] != float64(4) {
		t.Fatalf("stderr payload = %#v", stderr)
	}
}

func TestRunRequiresExplicitMissingCommandPolicy(t *testing.T) {
	ctx := context.Background()
	_, err := Run(ctx, Config{WorkingDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "missing commands policy is required") {
		t.Fatalf("error = %v, want missing command policy requirement", err)
	}

	called := false
	failResult, err := Run(ctx, Config{
		WorkingDir:            t.TempDir(),
		MissingCommandsPolicy: MissingCommandsFail,
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			called = true
			return runner.Result{}
		},
	})
	if err != nil {
		t.Fatalf("run verification with fail policy: %v", err)
	}
	if called {
		t.Fatal("runner was called for missing commands")
	}
	if failResult.Status != StatusFailed || failResult.Passed || !failResult.MissingCommands {
		t.Fatalf("fail policy result = %+v", failResult)
	}

	passResult, err := Run(ctx, Config{
		WorkingDir:            t.TempDir(),
		MissingCommandsPolicy: MissingCommandsPass,
	})
	if err != nil {
		t.Fatalf("run verification with pass policy: %v", err)
	}
	if passResult.Status != StatusPassed || !passResult.Passed || !passResult.MissingCommands {
		t.Fatalf("pass policy result = %+v", passResult)
	}
}

func TestRunRejectsEscapingCommandDir(t *testing.T) {
	for _, dir := range []string{"../outside", filepath.Join(t.TempDir(), "outside")} {
		t.Run(dir, func(t *testing.T) {
			called := false
			_, err := Run(context.Background(), Config{
				WorkingDir: t.TempDir(),
				Commands: []Command{
					{Name: "go", Args: []string{"test", "./..."}, Dir: dir},
				},
				CommandRunner: func(context.Context, runner.Command) runner.Result {
					called = true
					return runner.Result{ExitCode: 0}
				},
			})
			if err == nil {
				t.Fatal("run verification succeeded, want command dir error")
			}
			if called {
				t.Fatal("command runner was called after command dir rejection")
			}
		})
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
