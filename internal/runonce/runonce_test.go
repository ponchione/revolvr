package runonce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
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
	if result.Receipt.CommitSHA != "abc123def456" {
		t.Fatalf("receipt commit sha = %q, want abc123def456", result.Receipt.CommitSHA)
	}
	if result.Receipt.VerificationStatus != "passed" {
		t.Fatalf("receipt verification status = %q, want passed", result.Receipt.VerificationStatus)
	}
	if got, want := result.Receipt.Verification, []receipt.VerificationEntry{{Command: "go test ./...", ExitCode: 0, Status: "passed"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt verification entries = %#v, want %#v", got, want)
	}
	reparsedReceipt, err := receiptFromFile(result.ReceiptPath)
	if err != nil {
		t.Fatalf("parse final receipt: %v", err)
	}
	if reparsedReceipt.CommitSHA != "abc123def456" || reparsedReceipt.VerificationStatus != "passed" {
		t.Fatalf("final receipt = %+v, want commit sha and passed verification", reparsedReceipt)
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
	if _, found, err := lock.ReadSourceWriter(ctx, env.workDir); err != nil || found {
		t.Fatalf("lock after successful run found=%v err=%v, want released", found, err)
	}
	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	artifacts, found := ledger.RunArtifactsFromEvents(history.Events)
	if !found {
		t.Fatal("run artifacts not found in ledger events")
	}
	wantArtifacts := ledger.RunArtifacts{
		PromptPath:           filepath.Join(".revolvr", "runs", result.Run.ID, "prompt.md"),
		CodexStdoutJSONLPath: filepath.Join(".revolvr", "runs", result.Run.ID, "codex.jsonl"),
		CodexStderrPath:      filepath.Join(".revolvr", "runs", result.Run.ID, "codex.stderr"),
		LastMessagePath:      filepath.Join(".revolvr", "runs", result.Run.ID, "last-message.txt"),
		ReceiptPath:          filepath.Join(".revolvr", "receipts", result.Run.ID+".md"),
	}
	if !reflect.DeepEqual(artifacts, wantArtifacts) {
		t.Fatalf("run artifacts = %#v, want %#v", artifacts, wantArtifacts)
	}
	assertRunEvents(t, env.ledger, result.Run.ID, []ledger.EventType{
		ledger.EventRunStarted,
		ledger.EventTaskSelected,
		ledger.EventRunArtifacts,
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

func TestRunRecordsChangedFilesReceiptWarningWithoutBlockingCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-warning-files", Task: "Create a mismatched receipt"}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{ChangedFiles: []string{"internal/claimed.go"}})
		},
		postStatus:        " M internal/actual.go\n",
		verificationExit:  0,
		commitSHA:         "abc123def456",
		expectedCommitAdd: []string{"add", "--", "internal/actual.go"},
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
	if result.Task.Status != taskqueue.StatusCompleted || result.Run.Status != ledger.StatusCompleted {
		t.Fatalf("task/run status = %s/%s, want completed/completed", result.Task.Status, result.Run.Status)
	}

	warnings := receiptWarningEvents(t, env.ledger, result.Run.ID)
	if len(warnings) != 1 {
		t.Fatalf("warning count = %d, want 1", len(warnings))
	}
	var payload struct {
		WarningType string   `json:"warning_type"`
		Message     string   `json:"message"`
		ReceiptPath string   `json:"receipt_path"`
		Claimed     []string `json:"claimed"`
		Observed    []string `json:"observed"`
	}
	decodeEventPayload(t, warnings[0], &payload)
	if payload.WarningType != receiptWarningChangedFiles {
		t.Fatalf("warning type = %q, want %q", payload.WarningType, receiptWarningChangedFiles)
	}
	if payload.Message != changedFilesWarningMessage {
		t.Fatalf("warning message = %q, want %q", payload.Message, changedFilesWarningMessage)
	}
	if got, want := payload.ReceiptPath, filepath.Join(".revolvr", "receipts", result.Run.ID+".md"); got != want {
		t.Fatalf("receipt path = %q, want %q", got, want)
	}
	if got, want := payload.Claimed, []string{"internal/claimed.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("claimed files = %#v, want %#v", got, want)
	}
	if got, want := payload.Observed, []string{"internal/actual.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("observed files = %#v, want %#v", got, want)
	}
}

func TestRunRecordsVerificationReceiptWarningWithoutBlockingCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-warning-verification", Task: "Claim wrong verification"}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{
				ChangedFiles:       []string{"internal/feature.go"},
				VerificationStatus: "failed",
				Verification: []receipt.VerificationEntry{{
					Command:  "go test ./...",
					ExitCode: 1,
					Status:   "failed",
				}},
			})
		},
		postStatus:       " M internal/feature.go\n",
		verificationExit: 0,
		commitSHA:        "abc123def456",
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
	warnings := receiptWarningEvents(t, env.ledger, result.Run.ID)
	if len(warnings) != 1 {
		t.Fatalf("warning count = %d, want 1", len(warnings))
	}
	var payload struct {
		WarningType string            `json:"warning_type"`
		Message     string            `json:"message"`
		Claimed     verificationFacts `json:"claimed"`
		Observed    verificationFacts `json:"observed"`
	}
	decodeEventPayload(t, warnings[0], &payload)
	if payload.WarningType != receiptWarningVerification {
		t.Fatalf("warning type = %q, want %q", payload.WarningType, receiptWarningVerification)
	}
	if payload.Message != verificationWarningMessage {
		t.Fatalf("warning message = %q, want %q", payload.Message, verificationWarningMessage)
	}
	if payload.Claimed.Status != "failed" || payload.Observed.Status != "passed" {
		t.Fatalf("verification payload = %+v / %+v, want failed/passed", payload.Claimed, payload.Observed)
	}
}

func TestRunReceiptWarningsDoNotChangeFailedOutcome(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-warning-failed", Task: "Fail despite receipt"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{
				ChangedFiles:       []string{"internal/feature.go"},
				VerificationStatus: "passed",
				Verification: []receipt.VerificationEntry{{
					Command:  "go test ./...",
					ExitCode: 0,
					Status:   "passed",
				}},
			})
		},
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
	if result.Task.Status != taskqueue.StatusBlocked || result.Run.Status != ledger.StatusFailed {
		t.Fatalf("task/run status = %s/%s, want blocked/failed", result.Task.Status, result.Run.Status)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
	}
	warnings := receiptWarningEvents(t, env.ledger, result.Run.ID)
	if len(warnings) != 2 {
		t.Fatalf("warning count = %d, want verification and verdict warnings", len(warnings))
	}
}

func TestRunDoesNotWarnWhenReceiptFactsMatchHarness(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-no-warning", Task: "Match receipt facts"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	state := &fakeCommandState{
		t:            t,
		workDir:      env.workDir,
		writeReceipt: true,
		receiptContent: func(runID, taskID, task string) string {
			return receiptContent(runID, taskID, task, receiptOptions{
				ChangedFiles:       []string{"internal/feature.go"},
				VerificationStatus: "passed",
				Verification: []receipt.VerificationEntry{{
					Command:  "go test ./...",
					ExitCode: 0,
					Status:   "passed",
				}},
			})
		},
		postStatus:       " M internal/feature.go\n",
		verificationExit: 0,
		commitSHA:        "abc123def456",
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
		t.Fatalf("outcome = %s, want committed", result.Outcome)
	}
	if warnings := receiptWarningEvents(t, env.ledger, result.Run.ID); len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
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
	if warnings := receiptWarningEvents(t, env.ledger, result.Run.ID); len(warnings) != 0 {
		t.Fatalf("fallback warnings = %+v, want none", warnings)
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

func TestRunBlocksPreExistingDirtyBeforePromptCodexVerificationAndCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-dirty", Task: "Avoid dirty worktree"}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	codexCalled := false
	changedCalled := false
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		TaskStore:   env.tasks,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:       gitstate.CaptureKindDirty,
				DirtyFiles: []string{"internal/dirty.go"},
				Paths:      []string{"internal/dirty.go"},
			}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			changedCalled = true
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			verificationCalled = true
			return verification.Result{Status: verification.StatusPassed, Passed: true}, nil
		},
		CommitRunner: func(context.Context, commit.Config) (commit.Result, error) {
			commitCalled = true
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123"}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %s, want blocked", result.Outcome)
	}
	if result.Message != "pre-existing dirty files are present" {
		t.Fatalf("message = %q, want dirty preflight message", result.Message)
	}
	if codexCalled || changedCalled || verificationCalled || commitCalled {
		t.Fatalf("called codex=%v changed=%v verification=%v commit=%v, want all false", codexCalled, changedCalled, verificationCalled, commitCalled)
	}
	if result.Task.Status != taskqueue.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if result.Run.Status != ledger.StatusFailed {
		t.Fatalf("run status = %q, want failed", result.Run.Status)
	}
	if !result.ReceiptSynthesized || result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("receipt synthesized=%v verdict=%q, want synthesized blocked", result.ReceiptSynthesized, result.Receipt.Verdict)
	}
	promptPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "prompt.md")
	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Fatalf("prompt artifact stat err = %v, want not exist", err)
	}
	assertRunEvents(t, env.ledger, result.Run.ID, []ledger.EventType{
		ledger.EventRunStarted,
		ledger.EventTaskSelected,
		ledger.EventRunArtifacts,
		ledger.EventChangedFilesCaptured,
		ledger.EventReceiptSynthesized,
		ledger.EventRunFailed,
	})

	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, err)
	}
	var payload struct {
		PreRunDirtyFiles []string `json:"pre_run_dirty_files"`
		ChangedFiles     []string `json:"changed_files"`
		CaptureError     string   `json:"capture_error"`
	}
	if !decodeTestEventPayload(t, history.Events, ledger.EventChangedFilesCaptured, &payload) {
		t.Fatal("changed-files capture event not found")
	}
	if got, want := payload.PreRunDirtyFiles, []string{"internal/dirty.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-run dirty files = %#v, want %#v", got, want)
	}
	if len(payload.ChangedFiles) != 0 || payload.CaptureError != "" {
		t.Fatalf("changed-files payload = %+v, want no changed files or error", payload)
	}
	if _, found, err := lock.ReadSourceWriter(ctx, env.workDir); err != nil || found {
		t.Fatalf("lock after dirty preflight found=%v err=%v, want released", found, err)
	}
}

func TestRunAllowsPreExistingDirtyWhenConfigured(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-allow-dirty", Task: "Proceed with dirty worktree"}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	codexCalled := false
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:                env.workDir,
		TaskStore:                 env.tasks,
		LedgerStore:               env.ledger,
		AllowPreExistingDirty:     true,
		VerificationCommands:      []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		MissingVerificationPolicy: verification.MissingCommandsFail,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:       gitstate.CaptureKindDirty,
				DirtyFiles: []string{"internal/dirty.go"},
				Paths:      []string{"internal/dirty.go"},
			}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{
				Kind:         gitstate.CaptureKindChanged,
				ChangedFiles: []string{"internal/feature.go"},
				Paths:        []string{"internal/feature.go"},
			}, nil
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			verificationCalled = true
			return verification.Result{
				Status:             verification.StatusPassed,
				Passed:             true,
				FailedCommandIndex: -1,
				Commands: []verification.CommandResult{{
					Command:  "go test ./...",
					Name:     "go",
					Args:     []string{"test", "./..."},
					Status:   verification.StatusPassed,
					Passed:   true,
					ExitCode: 0,
				}},
			}, nil
		},
		CommitRunner: func(_ context.Context, cfg commit.Config) (commit.Result, error) {
			commitCalled = true
			if !cfg.AllowPreExistingDirty {
				t.Fatal("commit config AllowPreExistingDirty = false, want true")
			}
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123", Message: "commit created"}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if !codexCalled || !verificationCalled || !commitCalled {
		t.Fatalf("called codex=%v verification=%v commit=%v, want all true", codexCalled, verificationCalled, commitCalled)
	}
	if result.Run.CommitSHA != "abc123" {
		t.Fatalf("run commit sha = %q, want abc123", result.Run.CommitSHA)
	}
	if result.Task.Status != taskqueue.StatusCompleted {
		t.Fatalf("task status = %q, want completed", result.Task.Status)
	}
}

func TestRunUpdatesParsedReceiptWhenVerificationFails(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-receipt-verify", Task: "Break verification after receipt"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		writeReceipt:     true,
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
	if result.ReceiptSynthesized {
		t.Fatal("receipt was synthesized, want parsed receipt updated by harness")
	}
	if result.Receipt.Verdict != receipt.VerdictVerificationFailed {
		t.Fatalf("receipt verdict = %q, want verification_failed", result.Receipt.Verdict)
	}
	if result.Receipt.VerificationStatus != "failed" {
		t.Fatalf("receipt verification status = %q, want failed", result.Receipt.VerificationStatus)
	}
	if state.gitAddOrCommitCalls != 0 {
		t.Fatalf("git add/commit calls = %d, want 0", state.gitAddOrCommitCalls)
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

func TestRunRefusesLiveSourceWriterLockBeforeStateMutation(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-locked", Task: "Do locked work"}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	existing, err := lock.AcquireSourceWriter(ctx, lock.Config{
		WorkingDir: env.workDir,
		RunID:      "already-running",
		PID:        999,
		Timeout:    time.Hour,
		Clock:      env.clock,
	})
	if err != nil {
		t.Fatalf("acquire existing source-writer lock: %v", err)
	}
	defer existing.Release(ctx)

	state := &fakeCommandState{t: t, workDir: env.workDir}
	result, err := Run(ctx, Config{
		WorkingDir:          env.workDir,
		TaskStore:           env.tasks,
		LedgerStore:         env.ledger,
		CommandRunner:       state.run,
		Clock:               env.clock,
		SourceWriterLockPID: 123,
	})
	if !errors.Is(err, lock.ErrHeld) {
		t.Fatalf("run once error = %v, want source-writer lock held", err)
	}
	if result.Run.ID != "" {
		t.Fatalf("run id = %q, want no ledger run created", result.Run.ID)
	}
	if len(state.commands) != 0 {
		t.Fatalf("commands = %#v, want none", state.commands)
	}
	runs, err := env.ledger.ListRecentRuns(ctx, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %#v, want none", runs)
	}
	task, ok, err := env.tasks.GetTask(ctx, "task-locked")
	if err != nil || !ok {
		t.Fatalf("get task: ok=%v err=%v", ok, err)
	}
	if task.Status != taskqueue.StatusPending {
		t.Fatalf("task status = %q, want pending", task.Status)
	}
}

func TestRunRefreshesSourceWriterLockWhileCodexRuns(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-heartbeat", Task: "Observe heartbeat"}); err != nil {
		t.Fatalf("add task: %v", err)
	}
	clock := &advancingClock{current: time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC), step: time.Second}

	var observed lock.Metadata
	codexRunner := func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
		deadline := time.After(2 * time.Second)
		for {
			metadata, found, err := lock.ReadSourceWriter(ctx, env.workDir)
			if err != nil {
				return codexexec.Result{}, err
			}
			if found && metadata.RunID == cfg.RunID && metadata.HeartbeatAt.After(metadata.AcquiredAt) {
				observed = metadata
				return codexexec.Result{ExitCode: 2}, nil
			}
			select {
			case <-ctx.Done():
				return codexexec.Result{}, ctx.Err()
			case <-deadline:
				return codexexec.Result{}, errors.New("timed out waiting for source-writer heartbeat")
			case <-time.After(time.Millisecond):
			}
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir:                        env.workDir,
		TaskStore:                         env.tasks,
		LedgerStore:                       env.ledger,
		CodexRunner:                       codexRunner,
		DirtyCapture:                      cleanDirtyCapture,
		ChangedCapture:                    emptyChangedCapture,
		Clock:                             clock.now,
		SourceWriterLockPID:               321,
		SourceWriterLockTimeout:           time.Hour,
		SourceWriterLockHeartbeatInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if observed.RunID != result.Run.ID || observed.PID != 321 {
		t.Fatalf("observed metadata = %+v, want run %s pid 321", observed, result.Run.ID)
	}
	if !observed.HeartbeatAt.After(observed.AcquiredAt) {
		t.Fatalf("heartbeat was not refreshed: %+v", observed)
	}
	if _, found, err := lock.ReadSourceWriter(ctx, env.workDir); err != nil || found {
		t.Fatalf("lock after run found=%v err=%v, want released", found, err)
	}
}

func TestRunReleasesSourceWriterLockOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := newTestEnv(t)
	if _, err := env.tasks.AddTask(ctx, taskqueue.TaskSpec{ID: "task-cancel", Task: "Cancel while Codex runs"}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	codexRunner := func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
		cancel()
		err := ctx.Err()
		if err == nil {
			err = context.Canceled
		}
		return codexexec.Result{ExitCode: -1, Err: err}, err
	}

	_, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		TaskStore:   env.tasks,
		LedgerStore: env.ledger,
		CodexRunner: codexRunner,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
		},
		Clock: env.clock,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run once error = %v, want context canceled", err)
	}
	if _, found, readErr := lock.ReadSourceWriter(context.Background(), env.workDir); readErr != nil || found {
		t.Fatalf("lock after cancellation found=%v err=%v, want released", found, readErr)
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

func cleanDirtyCapture(context.Context, gitstate.Config) (gitstate.Capture, error) {
	return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
}

func emptyChangedCapture(context.Context, gitstate.Config) (gitstate.Capture, error) {
	return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
}

type advancingClock struct {
	mu      sync.Mutex
	current time.Time
	step    time.Duration
}

func (c *advancingClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(c.step)
	return c.current
}

type fakeCommandState struct {
	t                   *testing.T
	workDir             string
	writeReceipt        bool
	receiptContent      func(runID string, taskID string, task string) string
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
		if s.receiptContent != nil {
			content = s.receiptContent(runID, taskID, "Implement the selected task")
		}
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

type receiptOptions struct {
	Verdict            receipt.Verdict
	ChangedFiles       []string
	VerificationStatus string
	Verification       []receipt.VerificationEntry
}

func receiptContent(runID string, taskID string, task string, opts receiptOptions) string {
	verdict := opts.Verdict
	if verdict == "" {
		verdict = receipt.VerdictCompleted
	}
	verificationStatus := opts.VerificationStatus
	if verificationStatus == "" {
		verificationStatus = "not_run"
	}

	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "schema_version: %s\n", receipt.SchemaVersion)
	fmt.Fprintf(&out, "run_id: %s\n", runID)
	fmt.Fprintf(&out, "pass_id: %s\n", runID)
	fmt.Fprintf(&out, "task_id: %s\n", taskID)
	fmt.Fprintf(&out, "task: %q\n", task)
	fmt.Fprintf(&out, "verdict: %s\n", verdict)
	out.WriteString("timestamp: 2026-06-26T12:00:00Z\n")
	out.WriteString("codex_exit_code: 0\n")
	fmt.Fprintf(&out, "verification_status: %s\n", verificationStatus)
	out.WriteString("commit_sha: \"\"\n")
	if len(opts.ChangedFiles) == 0 {
		out.WriteString("changed_files: []\n")
	} else {
		out.WriteString("changed_files:\n")
		for _, path := range opts.ChangedFiles {
			fmt.Fprintf(&out, "  - %s\n", path)
		}
	}
	if len(opts.Verification) == 0 {
		out.WriteString("verification: []\n")
	} else {
		out.WriteString("verification:\n")
		for _, entry := range opts.Verification {
			fmt.Fprintf(&out, "  - command: %s\n", entry.Command)
			fmt.Fprintf(&out, "    exit_code: %d\n", entry.ExitCode)
			fmt.Fprintf(&out, "    status: %s\n", entry.Status)
		}
	}
	out.WriteString("metrics:\n")
	out.WriteString("  input_tokens: 0\n")
	out.WriteString("  output_tokens: 0\n")
	out.WriteString("  duration_seconds: 0\n")
	out.WriteString("---\n")
	out.WriteString("## Summary\nImplemented the selected task.\n\n")
	out.WriteString("## Changed Files\n")
	if len(opts.ChangedFiles) == 0 {
		out.WriteString("None.\n")
	} else {
		for _, path := range opts.ChangedFiles {
			fmt.Fprintf(&out, "- %s\n", path)
		}
	}
	out.WriteString("\n## Verification\n")
	if len(opts.Verification) == 0 {
		out.WriteString("- Not run yet.\n")
	} else {
		for _, entry := range opts.Verification {
			fmt.Fprintf(&out, "- `%s` (%s, exit %d)\n", entry.Command, entry.Status, entry.ExitCode)
		}
	}
	out.WriteString("\n## Concerns\nNone.\n\n")
	out.WriteString("## Next Steps\nNone.\n")
	return out.String()
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

func receiptFromFile(path string) (receipt.Receipt, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return receipt.Receipt{}, err
	}
	return receipt.Parse(content)
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

func receiptWarningEvents(t *testing.T, store *ledger.Store, runID string) []ledger.Event {
	t.Helper()
	history, ok, err := store.GetRunWithEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	var warnings []ledger.Event
	for _, event := range history.Events {
		if event.Type == ledger.EventReceiptWarning {
			warnings = append(warnings, event)
		}
	}
	return warnings
}

func decodeEventPayload(t *testing.T, event ledger.Event, target any) {
	t.Helper()
	if err := json.Unmarshal(event.Payload, target); err != nil {
		t.Fatalf("decode %s payload: %v", event.Type, err)
	}
}

func decodeTestEventPayload(t *testing.T, events []ledger.Event, eventType ledger.EventType, target any) bool {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		if err := json.Unmarshal(event.Payload, target); err != nil {
			t.Fatalf("decode %s payload: %v", eventType, err)
		}
		return true
	}
	return false
}
