package codexexec

import (
	"bufio"
	"bytes"
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

	"revolvr/internal/jsonl"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/redact"
	"revolvr/internal/runner"
)

func TestRunInvokesCodexExecAndCapturesArtifactsAndLedger(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	canonicalLastMessage := filepath.Join(workDir, ".revolvr/runs/run-1/last-message.txt")
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
			if err := writeCommandStdout(command, line+"\n"); err != nil {
				t.Fatalf("write fake stdout: %v", err)
			}
		}
		command.OnStderrLine("codex progress")
		lastMessagePath := argAfter(command.Args, "--output-last-message")
		if lastMessagePath == "" {
			t.Fatal("missing --output-last-message argument")
		}
		if lastMessagePath != lastMessageRawPath(canonicalLastMessage) {
			t.Fatalf("last-message argument = %q, want raw temporary %q", lastMessagePath, lastMessageRawPath(canonicalLastMessage))
		}
		if _, err := os.Stat(canonicalLastMessage); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canonical last-message exists while child is running: %v", err)
		}
		assertFileMode(t, lastMessagePath, 0o600)
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
		"--output-last-message", lastMessageRawPath(canonicalLastMessage),
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
	assertFile(t, canonicalLastMessage, "final from file\n")
	assertFileMode(t, canonicalLastMessage, 0o644)
	assertPathAbsent(t, lastMessageRawPath(canonicalLastMessage))
	assertPathAbsent(t, lastMessageRedactedPath(canonicalLastMessage))

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
	canonicalLastMessage := filepath.Join(workDir, "run/final.json")
	redactor, _, err := redact.New(redact.Policy{SchemaVersion: redact.PolicySchemaVersion, EnvironmentVariables: []string{"TOKEN"}}, func(name string) (string, bool) { return secret, name == "TOKEN" })
	if err != nil {
		t.Fatal(err)
	}
	result, err := Run(context.Background(), Config{
		WorkingDir: workDir, Prompt: "safe prompt", Redactor: redactor,
		Artifacts: ArtifactPaths{StdoutJSONL: "run/codex.jsonl", Stderr: "run/codex.stderr", LastMessage: "run/final.json"},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			rawLastMessage := argAfter(command.Args, "--output-last-message")
			if rawLastMessage != lastMessageRawPath(canonicalLastMessage) {
				t.Fatalf("last-message argument = %q, want %q", rawLastMessage, lastMessageRawPath(canonicalLastMessage))
			}
			assertPathAbsent(t, canonicalLastMessage)
			assertFileMode(t, rawLastMessage, 0o600)
			line := `{"type":"turn.completed","final_message":"` + secret + `"}`
			if err := writeCommandStdout(command, line+"\n"); err != nil {
				t.Fatal(err)
			}
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
	assertFileMode(t, canonicalLastMessage, 0o644)
	assertPathAbsent(t, lastMessageRawPath(canonicalLastMessage))
	assertPathAbsent(t, lastMessageRedactedPath(canonicalLastMessage))
	if result.LastMessageRedaction.MatchCount != 1 {
		t.Fatalf("last-message redaction facts = %#v, want one match", result.LastMessageRedaction)
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

func TestReleaseCodexAllowlist(t *testing.T) {
	manifest, err := CurrentReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != ReleaseManifestSchema || len(manifest.Codex) != 1 {
		t.Fatalf("release manifest = %+v", manifest)
	}
	build := manifest.Codex[0]
	identity := CodexExecutableIdentity{Version: build.Version, Executable: ExecutableIdentity{Configured: "codex", Resolved: "/opt/revolvr/codex", SHA256: build.SHA256}}
	if err := manifest.Authorize(identity); err != nil {
		t.Fatalf("authorize exact release identity: %v", err)
	}

	for _, test := range []struct {
		name     string
		identity CodexExecutableIdentity
	}{
		{name: "unlisted version", identity: CodexExecutableIdentity{Version: "codex-cli 999.0.0", Executable: identity.Executable}},
		{name: "different bytes", identity: CodexExecutableIdentity{Version: build.Version, Executable: ExecutableIdentity{Configured: "codex", Resolved: "/opt/revolvr/codex", SHA256: strings.Repeat("f", 64)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := manifest.Authorize(test.identity); err == nil || !strings.Contains(err.Error(), "not release-authorized") {
				t.Fatalf("Authorize(%+v) error = %v", test.identity, err)
			}
		})
	}

	ranged := manifest
	ranged.Codex = append([]ReleaseCodexBuild(nil), manifest.Codex...)
	ranged.Codex[0].Version = "codex-cli >=0.144.4"
	if err := ranged.Validate(); err == nil || !strings.Contains(err.Error(), "exact Codex CLI version") {
		t.Fatalf("semantic range manifest error = %v", err)
	}
	duplicated := manifest
	duplicated.Codex = append(duplicated.Codex, duplicated.Codex[0])
	if err := duplicated.Validate(); err == nil || !strings.Contains(err.Error(), "exactly one Codex build") {
		t.Fatalf("multi-build first manifest error = %v", err)
	}
}

func TestRunReportsInvalidJSONAndProcessState(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		if err := writeCommandStdout(command, "{\"type\":\"thread.started\"}\nnot-json\n"); err != nil {
			t.Fatal(err)
		}
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

func TestRunPreservesLargeJSONLRecordsAcrossArbitraryChunks(t *testing.T) {
	workDir := t.TempDir()
	secret := "token-super-secret"
	redactor, _, err := redact.New(redact.Policy{SchemaVersion: redact.PolicySchemaVersion, EnvironmentVariables: []string{"TOKEN"}}, func(name string) (string, bool) {
		return secret, name == "TOKEN"
	})
	if err != nil {
		t.Fatal(err)
	}
	below := sizedJSONRecord(t, 64*1024-1, "below")
	above := sizedJSONRecord(t, 64*1024+1, secret)
	large := sizedJSONRecord(t, 80*1024, "café")
	stream := below + "\n" + above + "\n" + large
	streamBytes := []byte(stream)
	firstNewline := bytes.IndexByte(streamBytes, '\n')
	secretStart := bytes.Index(streamBytes, []byte(secret))
	secondNewline := bytes.IndexByte(streamBytes[firstNewline+1:], '\n') + firstNewline + 1
	runeStart := bytes.Index(streamBytes, []byte("é"))
	cuts := []int{1, 17, firstNewline, firstNewline + 1, secretStart + 2, secretStart + len(secret) - 1, secondNewline, secondNewline + 1, runeStart + 1}

	result, err := Run(context.Background(), Config{
		WorkingDir: workDir,
		Prompt:     "safe prompt",
		Redactor:   redactor,
		StdoutCap:  32,
		Artifacts:  ArtifactPaths{StdoutJSONL: "run/codex.jsonl"},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			start := 0
			for _, cut := range cuts {
				if err := writeCommandStdout(command, string(streamBytes[start:cut])); err != nil {
					t.Fatalf("write stdout chunk ending at %d: %v", cut, err)
				}
				start = cut
			}
			if err := writeCommandStdout(command, string(streamBytes[start:])); err != nil {
				t.Fatalf("write final stdout chunk: %v", err)
			}
			return runner.Result{
				ExitCode:             0,
				Stdout:               stream[:32],
				StdoutTruncatedBytes: int64(len(stream) - 32),
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Err != nil || result.ArtifactError != nil || len(result.JSONParseErrors) != 0 {
		t.Fatalf("unexpected result errors: err=%v artifact=%v parse=%#v", result.Err, result.ArtifactError, result.JSONParseErrors)
	}
	if result.JSONEvents != 3 {
		t.Fatalf("JSON events = %d, want 3", result.JSONEvents)
	}
	if len(result.Stdout.Content) != 32 || result.Stdout.TruncatedBytes != int64(len(stream)-32) {
		t.Fatalf("capped stdout = %d bytes, truncated = %d", len(result.Stdout.Content), result.Stdout.TruncatedBytes)
	}

	artifact, err := os.Open(result.Artifacts.StdoutJSONL)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	scanner := bufio.NewScanner(artifact)
	scanner.Buffer(make([]byte, 1024), jsonl.MaxRecordBytes+1)
	var payloads []string
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("artifact record %d is invalid JSON: %v", len(payloads)+1, err)
		}
		payloads = append(payloads, event["payload"].(string))
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 3 {
		t.Fatalf("artifact records = %d, want 3", len(payloads))
	}
	if len(payloads[0])+len(`{"type":"item.completed","payload":""}`) != 64*1024-1 {
		t.Fatalf("below-limit record payload size = %d", len(payloads[0]))
	}
	if strings.Contains(payloads[1], secret) || !strings.HasSuffix(payloads[1], redact.Replacement) {
		t.Fatalf("split secret was not redacted: suffix %q", payloads[1][len(payloads[1])-len(redact.Replacement):])
	}
	if !strings.HasSuffix(payloads[2], "café") {
		t.Fatalf("split UTF-8 payload suffix = %q", payloads[2][len(payloads[2])-8:])
	}
}

func TestRunRejectsOversizedJSONLRecordWithoutPersistingPartialJSON(t *testing.T) {
	workDir := t.TempDir()
	valid := `{"type":"thread.started"}`
	oversized := `{"payload":"` + strings.Repeat("x", jsonl.MaxRecordBytes) + `"}`
	result, err := Run(context.Background(), Config{
		WorkingDir: workDir,
		Prompt:     "safe prompt",
		Artifacts:  ArtifactPaths{StdoutJSONL: "run/codex.jsonl"},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			writeErr := writeCommandStdout(command, valid+"\n", oversized)
			return runner.Result{ExitCode: -1, Err: writeErr}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(result.Err, jsonl.ErrRecordTooLarge) {
		t.Fatalf("result error = %v, want ErrRecordTooLarge", result.Err)
	}
	if !errors.Is(result.ArtifactError, jsonl.ErrRecordTooLarge) {
		t.Fatalf("artifact error = %v, want ErrRecordTooLarge", result.ArtifactError)
	}
	var sizeErr *jsonl.RecordTooLargeError
	if !errors.As(result.Err, &sizeErr) || sizeErr.Record != 2 || sizeErr.Limit != jsonl.MaxRecordBytes {
		t.Fatalf("record size error = %#v", result.Err)
	}
	assertFile(t, result.Artifacts.StdoutJSONL, valid+"\n")
	if result.JSONEvents != 1 {
		t.Fatalf("JSON events = %d, want 1", result.JSONEvents)
	}
}

func TestRunClassifiesUnreadableMetricsSourceAsArtifactFailure(t *testing.T) {
	workDir := t.TempDir()
	artifactPath := filepath.Join(workDir, "run", "codex.jsonl")
	result, err := Run(context.Background(), Config{
		WorkingDir: workDir,
		Prompt:     "safe prompt",
		Artifacts:  ArtifactPaths{StdoutJSONL: "run/codex.jsonl"},
		CommandRunner: func(_ context.Context, _ runner.Command) runner.Result {
			if err := os.Remove(artifactPath); err != nil {
				t.Fatalf("remove open artifact name: %v", err)
			}
			if err := os.Mkdir(artifactPath, 0o755); err != nil {
				t.Fatalf("replace artifact with unreadable directory: %v", err)
			}
			return runner.Result{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ParseError != nil {
		t.Fatalf("parse error = %v, want source failure kept separate", result.ParseError)
	}
	if !errors.Is(result.ArtifactError, receipt.ErrCodexJSONLSource) {
		t.Fatalf("artifact error = %v, want Codex JSONL source failure", result.ArtifactError)
	}
}

func TestRunClassifiesCanceledMetricsParsingWithoutCorruptingArtifact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	workDir := t.TempDir()
	line := `{"type":"turn.completed","usage":{"input_tokens":1}}`
	result, err := Run(ctx, Config{
		WorkingDir: workDir,
		Prompt:     "safe prompt",
		Artifacts:  ArtifactPaths{StdoutJSONL: "run/codex.jsonl"},
		CommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			if err := writeCommandStdout(command, line+"\n"); err != nil {
				t.Fatal(err)
			}
			cancel()
			return runner.Result{ExitCode: 0, Stdout: line + "\n"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(result.ParseError, context.Canceled) {
		t.Fatalf("parse error = %v, want context cancellation", result.ParseError)
	}
	if result.ArtifactError != nil || result.UsageFound {
		t.Fatalf("artifact error/usage = %v/%t", result.ArtifactError, result.UsageFound)
	}
	assertFile(t, result.Artifacts.StdoutJSONL, line+"\n")
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

func writeCommandStdout(command runner.Command, chunks ...string) error {
	if command.StdoutWriter == nil {
		return errors.New("runner command has no authoritative stdout writer")
	}
	for _, chunk := range chunks {
		written, err := io.WriteString(command.StdoutWriter, chunk)
		if err != nil {
			return err
		}
		if written != len(chunk) {
			return io.ErrShortWrite
		}
	}
	return nil
}

func sizedJSONRecord(t *testing.T, size int, payloadSuffix string) string {
	t.Helper()
	prefix := `{"type":"item.completed","payload":"`
	suffix := `"}`
	fillerBytes := size - len(prefix) - len(payloadSuffix) - len(suffix)
	if fillerBytes < 0 {
		t.Fatalf("JSONL record size %d is too small", size)
	}
	record := prefix + strings.Repeat("x", fillerBytes) + payloadSuffix + suffix
	if len(record) != size {
		t.Fatalf("JSONL record size = %d, want %d", len(record), size)
	}
	return record
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

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s mode = %s, want regular non-symlink file", path, info.Mode())
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %s exists or cannot be inspected: %v", path, err)
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
