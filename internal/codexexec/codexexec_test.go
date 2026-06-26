package codexexec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
)

func TestRunInvokesCodexExecAndCapturesArtifactsAndLedger(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer store.Close()
	run, err := store.CreateRun(ctx, ledger.RunSpec{
		ID:     "run-1",
		TaskID: "task-1",
		Task:   "test codex runner",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	stdoutLines := []string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"item.completed","item":{"id":"item-1","type":"assistant_message","role":"assistant","content":[{"type":"output_text","text":"intermediate"}]}}`,
		`{"type":"turn.completed","final_message":"finished from JSON","usage":{"input_tokens":10,"output_tokens":4},"duration_ms":1500}`,
	}
	var gotCommand runner.Command
	var gotPrompt string
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		gotCommand = command
		prompt, err := io.ReadAll(command.Stdin)
		if err != nil {
			t.Fatalf("read stdin prompt: %v", err)
		}
		gotPrompt = string(prompt)
		for _, line := range stdoutLines {
			command.OnStdoutLine(line)
		}
		command.OnStderrLine("codex progress")
		lastMessagePath := argAfter(command.Args, "--output-last-message")
		if lastMessagePath == "" {
			t.Fatal("missing --output-last-message argument")
		}
		if err := os.WriteFile(lastMessagePath, []byte("final from file\n"), 0o644); err != nil {
			t.Fatalf("write fake last message: %v", err)
		}
		return runner.Result{
			ExitCode:             0,
			Stdout:               strings.Join(stdoutLines, "\n") + "\n",
			Stderr:               "codex progress\n",
			StdoutTruncatedBytes: 2,
			StderrTruncatedBytes: 3,
		}
	}

	result, err := Run(ctx, Config{
		Executable:     "codex-test",
		WorkingDir:     workDir,
		Prompt:         "do one task",
		Timeout:        12 * time.Second,
		StdoutCap:      123,
		StderrCap:      45,
		Sandbox:        "workspace-write",
		ApprovalPolicy: "never",
		Artifacts: ArtifactPaths{
			StdoutJSONL: ".revolvr/runs/run-1/codex.jsonl",
			Stderr:      ".revolvr/runs/run-1/codex.stderr",
			LastMessage: ".revolvr/runs/run-1/last-message.txt",
		},
		RunID:         run.ID,
		Ledger:        store,
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("run codex exec: %v", err)
	}

	wantArgs := []string{
		"--ask-for-approval", "never",
		"exec", "--json",
		"--sandbox", "workspace-write",
		"--cd", workDir,
		"--output-last-message", filepath.Join(workDir, ".revolvr/runs/run-1/last-message.txt"),
		"-",
	}
	if gotCommand.Name != "codex-test" {
		t.Fatalf("command name = %q, want codex-test", gotCommand.Name)
	}
	if gotCommand.Dir != workDir {
		t.Fatalf("command dir = %q, want %q", gotCommand.Dir, workDir)
	}
	if !reflect.DeepEqual(gotCommand.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotCommand.Args, wantArgs)
	}
	if containsArg(gotCommand.Args, "resume") {
		t.Fatalf("command args include resume: %#v", gotCommand.Args)
	}
	if gotPrompt != "do one task" {
		t.Fatalf("stdin prompt = %q, want prompt text", gotPrompt)
	}
	if gotCommand.Timeout != 12*time.Second || gotCommand.StdoutLimit != 123 || gotCommand.StderrLimit != 45 {
		t.Fatalf("runner command limits = timeout %s stdout %d stderr %d", gotCommand.Timeout, gotCommand.StdoutLimit, gotCommand.StderrLimit)
	}

	if result.ExitCode != 0 || result.TimedOut || result.Err != nil {
		t.Fatalf("unexpected result state: %+v", result)
	}
	if result.FinalMessage != "final from file" {
		t.Fatalf("final message = %q, want last-message artifact", result.FinalMessage)
	}
	if !result.UsageFound {
		t.Fatal("usage found = false, want true")
	}
	if got, want := result.Usage, (receipt.Metrics{InputTokens: 10, OutputTokens: 4, DurationSeconds: 2}); got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}
	if result.Stdout.TruncatedBytes != 2 || result.Stderr.TruncatedBytes != 3 {
		t.Fatalf("truncated bytes = stdout %d stderr %d", result.Stdout.TruncatedBytes, result.Stderr.TruncatedBytes)
	}

	assertFile(t, result.Artifacts.StdoutJSONL, strings.Join(stdoutLines, "\n")+"\n")
	assertFile(t, result.Artifacts.Stderr, "codex progress\n")

	history, ok, err := store.GetRunWithEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run was not found")
	}
	gotTypes := make([]ledger.EventType, 0, len(history.Events))
	for _, event := range history.Events {
		gotTypes = append(gotTypes, event.Type)
	}
	wantTypes := []ledger.EventType{
		ledger.EventCodexStarted,
		ledger.EventCodexJSONEvent,
		ledger.EventCodexJSONEvent,
		ledger.EventCodexJSONEvent,
		ledger.EventCodexCompleted,
	}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("ledger event types = %#v, want %#v", gotTypes, wantTypes)
	}

	var jsonEventPayload map[string]any
	if err := json.Unmarshal(history.Events[1].Payload, &jsonEventPayload); err != nil {
		t.Fatalf("unmarshal codex json event: %v", err)
	}
	if jsonEventPayload["type"] != "thread.started" || jsonEventPayload["thread_id"] != "thread-1" {
		t.Fatalf("codex json event payload = %#v", jsonEventPayload)
	}
	var completedPayload map[string]any
	if err := json.Unmarshal(history.Events[len(history.Events)-1].Payload, &completedPayload); err != nil {
		t.Fatalf("unmarshal completed event: %v", err)
	}
	if completedPayload["exit_code"] != float64(0) || completedPayload["usage_found"] != true {
		t.Fatalf("completed payload = %#v", completedPayload)
	}
}

func TestRunReportsInvalidJSONAndProcessState(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		command.OnStdoutLine(`{"type":"thread.started"}`)
		command.OnStdoutLine(`not-json`)
		command.OnStderrLine("still running")
		return runner.Result{
			ExitCode: -1,
			Err:      context.DeadlineExceeded,
			TimedOut: true,
			Stdout:   "{\"type\":\"thread.started\"}\nnot-json\n",
			Stderr:   "still running\n",
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir: workDir,
		Prompt:     "do one task",
		Artifacts: ArtifactPaths{
			StdoutJSONL: "artifacts/codex.jsonl",
		},
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("run codex exec: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("timed out = false, want true")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", result.Err)
	}
	if result.JSONEvents != 1 {
		t.Fatalf("json events = %d, want 1", result.JSONEvents)
	}
	if len(result.JSONParseErrors) != 1 || !strings.Contains(result.JSONParseErrors[0], "line 2") {
		t.Fatalf("json parse errors = %#v, want line 2 error", result.JSONParseErrors)
	}
	if result.ParseError == nil {
		t.Fatal("parse error = nil, want usage parse error from invalid JSONL")
	}
	if result.Artifacts.Stderr != "" {
		t.Fatalf("stderr artifact path = %q, want empty", result.Artifacts.Stderr)
	}
}

func TestRunValidatesRequiredConfig(t *testing.T) {
	_, err := Run(context.Background(), Config{
		WorkingDir: t.TempDir(),
		Prompt:     "x",
	})
	if err == nil || !strings.Contains(err.Error(), "stdout JSONL artifact path is required") {
		t.Fatalf("error = %v, want stdout JSONL artifact requirement", err)
	}
}

func assertFile(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, string(got), want)
	}
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
