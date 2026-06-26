package runonce

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/commit"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/taskqueue"
	"revolvr/internal/verification"
)

func TestRunCommitsVerifiedCodexChanges(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{
		ID:      "task-1",
		Task:    "Implement the selected task",
		Summary: "Implement selected task",
	}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	state := &fakeCommandState{
		t:                 t,
		workDir:           env.workDir,
		writeReceipt:      true,
		postStatus:        " M internal/feature.go\n",
		verificationExit:  0,
		commitSHA:         "abc123def456",
		expectedCommitAdd: []string{"add", "--", "internal/feature.go"},
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		TaskStore:            env.tasks,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if result.Commit.CommitSHA != "abc123def456" {
		t.Fatalf("commit sha = %q, want abc123def456", result.Commit.CommitSHA)
	}
	if result.Task.Status != taskqueue.StatusCompleted {
		t.Fatalf("task status = %q, want completed", result.Task.Status)
	}
	if result.Run.Status != ledger.StatusCompleted {
		t.Fatalf("run status = %q, want completed", result.Run.Status)
	}
	if result.Run.CommitSHA != "abc123def456" {
		t.Fatalf("ledger commit sha = %q, want abc123def456", result.Run.CommitSHA)
	}
	if result.ReceiptSynthesized {
		t.Fatal("receipt was synthesized, want parsed Codex receipt")
	}
	if got, want := result.Receipt.Metrics, (receipt.Metrics{InputTokens: 7, OutputTokens: 3, DurationSeconds: 1}); got != want {
		t.Fatalf("receipt metrics = %#v, want %#v", got, want)
	}
	if containsArg(state.codexArgs, "resume") {
		t.Fatalf("codex args include resume: %#v", state.codexArgs)
	}
	if got, want := state.gitCommands, [][]string{
		{"status", "--short", "--untracked-files=all"},
		{"status", "--short", "--untracked-files=all"},
		{"add", "--", "internal/feature.go"},
		{"commit", "-m", "Implement selected task", "-m", "Run-ID: " + result.Run.ID + "\nTask-ID: task-1\nVerification: passed"},
		{"rev-parse", "--verify", "HEAD"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("git commands = %#v, want %#v", got, want)
	}
	assertRunEvents(t, env.ledger, result.Run.ID, []ledger.EventType{
		ledger.EventRunStarted,
		ledger.EventTaskSelected,
		ledger.EventPromptBuilt,
		ledger.EventCodexStarted,
		ledger.EventCodexJSONEvent,
		ledger.EventCodexCompleted,
		ledger.EventChangedFilesCaptured,
		ledger.EventReceiptParsed,
		ledger.EventVerificationStarted,
		ledger.EventVerificationCompleted,
		ledger.EventCommitStarted,
		ledger.EventCommitCreated,
		ledger.EventRunCompleted,
	})
}

func TestRunBlocksWhenVerificationFailsAndSkipsCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-verify", Task: "Break verification"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		postStatus:       " M internal/feature.go\n",
		verificationExit: 1,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		TaskStore:            env.tasks,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeVerificationFailed {
		t.Fatalf("outcome = %s, want verification_failed", result.Outcome)
	}
	if result.Task.Status != taskqueue.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if result.Run.Status != ledger.StatusFailed {
		t.Fatalf("run status = %q, want failed", result.Run.Status)
	}
	if result.Commit.Status != "" {
		t.Fatalf("commit result = %+v, want zero value", result.Commit)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	if !result.ReceiptSynthesized {
		t.Fatal("receipt synthesized = false, want fallback receipt")
	}
	if result.Receipt.Verdict != receipt.VerdictVerificationFailed {
		t.Fatalf("fallback verdict = %q, want verification_failed", result.Receipt.Verdict)
	}
}

func TestRunBlocksWhenNoChangesAfterSuccessfulVerification(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-no-change", Task: "Make no changes"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		postStatus:       "",
		verificationExit: 0,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		TaskStore:            env.tasks,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeNoChanges {
		t.Fatalf("outcome = %s, want no_changes", result.Outcome)
	}
	if result.Commit.Status != commit.StatusRefused || result.Commit.RefusalReason != commit.ReasonNoChanges {
		t.Fatalf("commit result = %+v, want no changes refusal", result.Commit)
	}
	if result.Task.Status != taskqueue.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	if result.Receipt.Verdict != receipt.VerdictNoChanges {
		t.Fatalf("fallback verdict = %q, want no_changes", result.Receipt.Verdict)
	}
}

func TestRunBlocksWhenCodexFailsAndSkipsVerificationAndCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-codex", Task: "Codex fails"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	state := &fakeCommandState{
		t:          t,
		workDir:    env.workDir,
		codexExit:  2,
		postStatus: " M internal/partial.go\n",
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		TaskStore:            env.tasks,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if result.Task.Status != taskqueue.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if state.verificationCalls != 0 {
		t.Fatalf("verification calls = %d, want 0", state.verificationCalls)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	if result.Receipt.Verdict != receipt.VerdictCodexFailed {
		t.Fatalf("fallback verdict = %q, want codex_failed", result.Receipt.Verdict)
	}
}

func TestRunReturnsNoTaskWhenQueueIsEmpty(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	state := &fakeCommandState{t: t, workDir: env.workDir}

	result, err := Run(ctx, Config{
		WorkingDir:    env.workDir,
		TaskStore:     env.tasks,
		LedgerStore:   env.ledger,
		CommandRunner: state.run,
		Clock:         env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !result.NoTask || result.Outcome != OutcomeNoTask {
		t.Fatalf("result = %+v, want no task", result)
	}
	if len(state.commands) != 0 {
		t.Fatalf("commands = %#v, want none", state.commands)
	}
}

type testEnv struct {
	workDir string
	tasks   *taskqueue.Store
	ledger  *ledger.Store
	now     time.Time
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	ctx := context.Background()
	workDir := t.TempDir()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	tasks, err := taskqueue.OpenWithClock(ctx, filepath.Join(workDir, "tasks.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open task store: %v", err)
	}
	t.Cleanup(func() { _ = tasks.Close() })
	runs, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	return testEnv{workDir: workDir, tasks: tasks, ledger: runs, now: now}
}

func (e testEnv) clock() time.Time {
	return e.now.Add(2 * time.Minute)
}

type fakeCommandState struct {
	t                   *testing.T
	workDir             string
	writeReceipt        bool
	postStatus          string
	codexExit           int
	verificationExit    int
	commitSHA           string
	expectedCommitAdd   []string
	commands            []runner.Command
	codexArgs           []string
	gitCommands         [][]string
	gitStatusCalls      int
	gitAddOrCommitCalls int
	verificationCalls   int
}

func (s *fakeCommandState) run(_ context.Context, command runner.Command) runner.Result {
	s.commands = append(s.commands, command)
	switch command.Name {
	case "codex-test", "codex":
		return s.runCodex(command)
	case "git-test", "git":
		return s.runGit(command)
	case "go":
		s.verificationCalls++
		if s.verificationExit != 0 {
			return runner.Result{ExitCode: s.verificationExit, Stderr: "verification failed\n"}
		}
		return runner.Result{ExitCode: 0, Stdout: "ok\n"}
	default:
		s.t.Fatalf("unexpected command %s %#v", command.Name, command.Args)
		return runner.Result{ExitCode: 127}
	}
}

func (s *fakeCommandState) runCodex(command runner.Command) runner.Result {
	s.codexArgs = append([]string(nil), command.Args...)
	promptText := readPrompt(s.t, command.Stdin)
	receiptRel := promptValue(s.t, promptText, "Receipt path")
	runID := promptValue(s.t, promptText, "Run ID")
	taskID := promptValue(s.t, promptText, "Task ID")
	if s.writeReceipt {
		content := validReceipt(runID, taskID, "Implement the selected task")
		if err := writeTestFile(filepath.Join(command.Dir, receiptRel), content); err != nil {
			s.t.Fatalf("write receipt: %v", err)
		}
	}
	if lastMessagePath := argAfter(command.Args, "--output-last-message"); lastMessagePath != "" {
		if err := writeTestFile(lastMessagePath, "final message\n"); err != nil {
			s.t.Fatalf("write last message: %v", err)
		}
	}
	line := `{"type":"turn.completed","final_message":"done","usage":{"input_tokens":7,"output_tokens":3,"duration_seconds":1}}`
	if command.OnStdoutLine != nil {
		command.OnStdoutLine(line)
	}
	exitCode := s.codexExit
	return runner.Result{ExitCode: exitCode, Stdout: line + "\n"}
}

func (s *fakeCommandState) runGit(command runner.Command) runner.Result {
	s.gitCommands = append(s.gitCommands, append([]string(nil), command.Args...))
	if reflect.DeepEqual(command.Args, []string{"status", "--short", "--untracked-files=all"}) {
		s.gitStatusCalls++
		if s.gitStatusCalls == 1 {
			return runner.Result{ExitCode: 0}
		}
		return runner.Result{ExitCode: 0, Stdout: s.postStatus}
	}
	if len(command.Args) > 0 && (command.Args[0] == "add" || command.Args[0] == "commit") {
		s.gitAddOrCommitCalls++
	}
	if len(s.expectedCommitAdd) > 0 && command.Args[0] == "add" && !reflect.DeepEqual(command.Args, s.expectedCommitAdd) {
		s.t.Fatalf("git add args = %#v, want %#v", command.Args, s.expectedCommitAdd)
	}
	switch command.Args[0] {
	case "add", "commit":
		return runner.Result{ExitCode: 0}
	case "rev-parse":
		sha := s.commitSHA
		if sha == "" {
			sha = "abc123"
		}
		return runner.Result{ExitCode: 0, Stdout: sha + "\n"}
	default:
		s.t.Fatalf("unexpected git command %#v", command.Args)
		return runner.Result{ExitCode: 2}
	}
}

func validReceipt(runID string, taskID string, task string) string {
	return fmt.Sprintf(`---
schema_version: revolvr.receipt.v1
run_id: %s
pass_id: %s
task_id: %s
task: %q
verdict: completed
timestamp: 2026-06-26T12:00:00Z
codex_exit_code: 0
verification_status: not_run
commit_sha: ""
changed_files:
  - internal/feature.go
verification: []
metrics:
  input_tokens: 0
  output_tokens: 0
  duration_seconds: 0
---
## Summary
Implemented the selected task.

## Changed Files
- internal/feature.go

## Verification
- Not run yet.

## Concerns
None.

## Next Steps
None.
`, runID, runID, taskID, task)
}

func readPrompt(t *testing.T, reader io.Reader) string {
	t.Helper()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	return string(content)
}

func promptValue(t *testing.T, promptText string, label string) string {
	t.Helper()
	prefix := "- " + label + ": `"
	for _, line := range strings.Split(promptText, "\n") {
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimPrefix(line, prefix)
			value = strings.TrimSuffix(value, "`")
			return value
		}
	}
	t.Fatalf("prompt missing %s:\n%s", label, promptText)
	return ""
}

func writeTestFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
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

func assertRunEvents(t *testing.T, store *ledger.Store, runID string, want []ledger.EventType) {
	t.Helper()
	history, ok, err := store.GetRunWithEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	got := make([]ledger.EventType, 0, len(history.Events))
	for _, event := range history.Events {
		got = append(got, event.Type)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}
