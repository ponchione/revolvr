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
	"revolvr/internal/redact"
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
	var progressEvents []ProgressEvent
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
		OnProgress: func(event ProgressEvent) {
			progressEvents = append(progressEvents, event)
		},
	})
	if err != nil {
		t.Fatalf("run codex exec: %v", err)
	}

	wantArgs := []string{
		"--ask-for-approval", "never",
		"exec", "--json",
		"--model", DefaultModel,
		"-c", "model_reasoning_effort=" + DefaultReasoningEffort,
		"--ephemeral",
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
	if !containsProgress(progressEvents, ProgressEvent{Source: "codex", Message: "thread started: thread-1"}) {
		t.Fatalf("progress events missing thread start: %#v", progressEvents)
	}
	if !containsProgress(progressEvents, ProgressEvent{Source: "codex", Message: "message: intermediate"}) {
		t.Fatalf("progress events missing assistant message: %#v", progressEvents)
	}
	if !containsProgress(progressEvents, ProgressEvent{Source: "codex stderr", Message: "codex progress"}) {
		t.Fatalf("progress events missing stderr line: %#v", progressEvents)
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

func TestRunRedactsPersistentAndReturnedCodexOutput(t *testing.T) {
	workDir := t.TempDir()
	secret := "token-super-secret"
	redactor, _, err := redact.New(redact.Policy{SchemaVersion: redact.PolicySchemaVersion, EnvironmentVariables: []string{"TOKEN"}}, func(name string) (string, bool) { return secret, name == "TOKEN" })
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), Config{
		WorkingDir: workDir, Prompt: "safe prompt", Redactor: redactor,
		Artifacts: ArtifactPaths{StdoutJSONL: "run/codex.jsonl", Stderr: "run/codex.stderr", LastMessage: "run/final.json"},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			line := `{"type":"turn.completed","final_message":"` + secret + `"}`
			command.OnStdoutLine(line)
			command.OnStderrLine("stderr " + secret)
			if err := os.WriteFile(argAfter(command.Args, "--output-last-message"), []byte(`{"value":"`+secret+`"}`), 0o644); err != nil {
				t.Fatal(err)
			}
			return runner.Result{ExitCode: 0, Stdout: line + "\n", Stderr: "stderr " + secret, Err: errors.New("wrapped " + secret)}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for label, value := range map[string]string{"stdout": result.Stdout.Content, "stderr": result.Stderr.Content, "error": result.Err.Error(), "final": result.FinalMessage} {
		if strings.Contains(value, secret) || !strings.Contains(value, redact.Replacement) {
			t.Fatalf("%s not redacted: %q", label, value)
		}
	}
	for _, path := range []string{result.Artifacts.StdoutJSONL, result.Artifacts.Stderr, result.Artifacts.LastMessage} {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(raw), secret) {
			t.Fatalf("artifact %s leaked secret", path)
		}
	}
}

func TestBuildArgsBypassesApprovalsAndSandbox(t *testing.T) {
	args := buildArgs("/repo", Config{
		Model:                     DefaultModel,
		ReasoningEffort:           DefaultReasoningEffort,
		Ephemeral:                 boolPointer(true),
		ApprovalPolicy:            "never",
		Sandbox:                   "workspace-write",
		BypassApprovalsAndSandbox: true,
	}, ArtifactPaths{
		LastMessage: "/repo/.revolvr/runs/run-1/last-message.txt",
	}, "")

	want := []string{
		"exec", "--json",
		"--model", DefaultModel,
		"-c", "model_reasoning_effort=" + DefaultReasoningEffort,
		"--ephemeral",
		"--dangerously-bypass-approvals-and-sandbox",
		"--cd", "/repo",
		"--output-last-message", "/repo/.revolvr/runs/run-1/last-message.txt",
		"-",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	if containsArg(args, "--ask-for-approval") {
		t.Fatalf("args include approval policy despite bypass: %#v", args)
	}
	if containsArg(args, "--sandbox") {
		t.Fatalf("args include sandbox despite bypass: %#v", args)
	}
}

func TestBuildArgsUsesOverridesWithoutLastMessage(t *testing.T) {
	args := buildArgs("/repo", Config{
		Model:                     "gpt-custom",
		ReasoningEffort:           "high",
		Ephemeral:                 boolPointer(true),
		BypassApprovalsAndSandbox: true,
	}, ArtifactPaths{}, "")
	want := []string{
		"exec", "--json",
		"--model", "gpt-custom",
		"-c", "model_reasoning_effort=high",
		"--ephemeral",
		"--dangerously-bypass-approvals-and-sandbox",
		"--cd", "/repo",
		"-",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	if containsArg(args, "resume") || containsArg(args, "--output-last-message") {
		t.Fatalf("args contain forbidden or optional argument: %#v", args)
	}
}

func TestRunAddsTypedOutputSchemaToInvocation(t *testing.T) {
	workDir := t.TempDir()
	schemaRel := "artifacts/schema.json"
	if err := os.MkdirAll(filepath.Join(workDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, schemaRel), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got runner.Command
	_, err := Run(context.Background(), Config{
		WorkingDir:   workDir,
		Prompt:       "return JSON",
		OutputSchema: schemaRel,
		Artifacts: ArtifactPaths{
			StdoutJSONL: "artifacts/codex.jsonl",
			LastMessage: "artifacts/output.json",
		},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			got = command
			if err := os.WriteFile(argAfter(command.Args, "--output-last-message"), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			return runner.Result{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantSchema := filepath.Join(workDir, schemaRel)
	if gotValue := argAfter(got.Args, "--output-schema"); gotValue != wantSchema {
		t.Fatalf("--output-schema = %q, want %q; args=%#v", gotValue, wantSchema, got.Args)
	}
	if countArg(got.Args, "--output-schema") != 1 || countArg(got.Args, "--output-last-message") != 1 {
		t.Fatalf("schema/last-message flags must occur exactly once: %#v", got.Args)
	}
	if containsArg(got.Args, "resume") {
		t.Fatalf("args include resume: %#v", got.Args)
	}
}

func TestRunRejectsUnsafeOrMissingOutputSchemaBeforeCommand(t *testing.T) {
	for _, schemaPath := range []string{"missing-schema.json", "../outside-schema.json", filepath.Join(t.TempDir(), "schema.json")} {
		t.Run(schemaPath, func(t *testing.T) {
			called := false
			_, err := Run(context.Background(), Config{
				WorkingDir:   t.TempDir(),
				Prompt:       "return JSON",
				OutputSchema: schemaPath,
				Artifacts:    ArtifactPaths{StdoutJSONL: "codex.jsonl"},
				CommandRunner: func(context.Context, runner.Command) runner.Result {
					called = true
					return runner.Result{}
				},
			})
			if err == nil {
				t.Fatal("Run() error = nil")
			}
			if called {
				t.Fatal("command runner called after output-schema rejection")
			}
		})
	}
}

func TestDiscoverVersion(t *testing.T) {
	tests := []struct {
		name    string
		result  runner.Result
		want    string
		wantErr string
	}{
		{name: "success", result: runner.Result{ExitCode: 0, Stdout: "codex-cli 1.2.3\n"}, want: "codex-cli 1.2.3"},
		{name: "timeout", result: runner.Result{ExitCode: -1, TimedOut: true, Err: context.DeadlineExceeded}, wantErr: "timed out"},
		{name: "execution failure", result: runner.Result{ExitCode: -1, Err: errors.New("start failed")}, wantErr: "execution failed"},
		{name: "nonzero", result: runner.Result{ExitCode: 2, Stderr: "bad version\n"}, wantErr: "exited with code 2: bad version"},
		{name: "stdout truncation", result: runner.Result{ExitCode: 0, Stdout: "codex-cli", StdoutTruncatedBytes: 4}, wantErr: "output was truncated"},
		{name: "stderr truncation", result: runner.Result{ExitCode: 0, Stdout: "codex-cli", StderrTruncatedBytes: 4}, wantErr: "output was truncated"},
		{name: "empty", result: runner.Result{ExitCode: 0, Stdout: " \n"}, wantErr: "version output is empty"},
		{name: "multiple lines", result: runner.Result{ExitCode: 0, Stdout: "codex-cli 1\nextra\n"}, wantErr: "one well-formed line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var command runner.Command
			got, err := DiscoverVersion(context.Background(), VersionConfig{
				Executable: "codex-test",
				WorkingDir: "/repo",
				Timeout:    2 * time.Second,
				StdoutCap:  101,
				StderrCap:  102,
				CommandRunner: func(_ context.Context, in runner.Command) runner.Result {
					command = in
					return tt.result
				},
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("DiscoverVersion error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil || got != tt.want {
				t.Fatalf("DiscoverVersion = %q, %v, want %q, nil", got, err, tt.want)
			}
			if command.Name != "codex-test" || !reflect.DeepEqual(command.Args, []string{"--version"}) || command.Dir != "/repo" {
				t.Fatalf("version command = %+v", command)
			}
			if command.Timeout != 2*time.Second || command.StdoutLimit != 101 || command.StderrLimit != 102 {
				t.Fatalf("version command bounds = %+v", command)
			}
		})
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

func TestRunRejectsDisabledEphemeralMode(t *testing.T) {
	called := false
	_, err := Run(context.Background(), Config{
		WorkingDir: t.TempDir(),
		Prompt:     "x",
		Ephemeral:  boolPointer(false),
		Artifacts:  ArtifactPaths{StdoutJSONL: "codex.jsonl"},
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			called = true
			return runner.Result{}
		},
	})
	if err == nil || !strings.Contains(err.Error(), "only ephemeral sessions are supported") {
		t.Fatalf("error = %v, want ephemeral requirement", err)
	}
	if called {
		t.Fatal("command runner called after disabled ephemeral mode")
	}
}

func TestRunRejectsEscapingArtifactPaths(t *testing.T) {
	for _, artifactPath := range []string{"../outside/codex.jsonl", filepath.Join(t.TempDir(), "codex.jsonl")} {
		t.Run(artifactPath, func(t *testing.T) {
			called := false
			_, err := Run(context.Background(), Config{
				WorkingDir: t.TempDir(),
				Prompt:     "do one task",
				Artifacts: ArtifactPaths{
					StdoutJSONL: artifactPath,
				},
				CommandRunner: func(context.Context, runner.Command) runner.Result {
					called = true
					return runner.Result{ExitCode: 0}
				},
			})
			if err == nil {
				t.Fatal("run codex exec succeeded, want artifact path error")
			}
			if called {
				t.Fatal("command runner was called after artifact path rejection")
			}
		})
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

func countArg(args []string, value string) int {
	count := 0
	for _, arg := range args {
		if arg == value {
			count++
		}
	}
	return count
}

func containsProgress(events []ProgressEvent, want ProgressEvent) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}

func boolPointer(value bool) *bool {
	return &value
}
