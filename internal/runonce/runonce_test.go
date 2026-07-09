package runonce

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/verification"
)

func TestRunCommitsVerifiedCodexChanges(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-1", "Implement selected task")

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
	if result.Task.Status != taskmodel.StatusCompleted {
		t.Fatalf("task status = %q, want completed", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("file task status = %q, want completed", got)
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
	if result.Run.CompletedAt == nil {
		t.Fatal("run completed at = nil, want completion time")
	}
	if !result.Receipt.Timestamp.Equal(*result.Run.CompletedAt) {
		t.Fatalf("receipt timestamp = %s, want run completed at %s", result.Receipt.Timestamp, *result.Run.CompletedAt)
	}
	if !reparsedReceipt.Timestamp.Equal(*result.Run.CompletedAt) {
		t.Fatalf("final receipt timestamp = %s, want run completed at %s", reparsedReceipt.Timestamp, *result.Run.CompletedAt)
	}
	if containsArg(state.codexArgs, "resume") {
		t.Fatalf("codex args include resume: %#v", state.codexArgs)
	}
	if got, want := state.gitCommands, [][]string{
		{"status", "--short", "--untracked-files=all"},
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
		ContextPayloadPath:   filepath.Join(".revolvr", "runs", result.Run.ID, "context.md"),
		ContextManifestPath:  filepath.Join(".revolvr", "runs", result.Run.ID, "context.json"),
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
		ledger.EventContextBuilt,
		ledger.EventCodexStarted,
		ledger.EventCodexJSONEvent,
		ledger.EventCodexCompleted,
		ledger.EventChangedFilesCaptured,
		ledger.EventReceiptParsed,
		ledger.EventVerificationStarted,
		ledger.EventVerificationCompleted,
		ledger.EventChangedFilesCaptured,
		ledger.EventCommitStarted,
		ledger.EventCommitCreated,
		ledger.EventRunCompleted,
	})
}

func TestRunSuccessfulCommitChangedFilesIncludeCompletedTaskFile(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-commit-status", "Commit task status")

	var changedCaptureCalls int
	var commitChangedFiles []string
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
		LedgerStore: env.ledger,
		DirtyCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			return gitstate.Capture{Kind: gitstate.CaptureKindDirty}, nil
		},
		ChangedCapture: func(context.Context, gitstate.Config) (gitstate.Capture, error) {
			changedCaptureCalls++
			switch changedCaptureCalls {
			case 1:
				if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusPending {
					t.Fatalf("task status before verification capture = %q, want pending", got)
				}
				return gitstate.Capture{
					Kind:         gitstate.CaptureKindChanged,
					ChangedFiles: []string{"internal/feature.go"},
					Paths:        []string{"internal/feature.go"},
				}, nil
			case 2:
				if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusCompleted {
					t.Fatalf("task status before commit capture = %q, want completed", got)
				}
				return gitstate.Capture{
					Kind:         gitstate.CaptureKindChanged,
					ChangedFiles: []string{"internal/feature.go", selected.SourcePath},
					Paths:        []string{"internal/feature.go", selected.SourcePath},
				}, nil
			default:
				t.Fatalf("changed capture call %d, want exactly 2", changedCaptureCalls)
				return gitstate.Capture{}, nil
			}
		},
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		CommitRunner: func(_ context.Context, cfg commit.Config) (commit.Result, error) {
			commitChangedFiles = changedFiles(*cfg.PostRunChanged)
			return commit.Result{Status: commit.StatusCommitted, CommitSHA: "abc123", ChangedFiles: commitChangedFiles}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeCommitted {
		t.Fatalf("outcome = %s, want committed; message=%s", result.Outcome, result.Message)
	}
	if changedCaptureCalls != 2 {
		t.Fatalf("changed capture calls = %d, want 2", changedCaptureCalls)
	}
	if got, want := commitChangedFiles, []string{"internal/feature.go", selected.SourcePath}; !reflect.DeepEqual(got, want) {
		t.Fatalf("commit changed files = %#v, want %#v", got, want)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("file task status = %q, want completed", got)
	}
}

func TestRunSelectsLowestPriorityPendingTaskFileByPriorityThenFilename(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTaskFile(t, env, "020-later.md", taskFileMarkdown("task-later", "Later Task", taskfile.StatusPending, ptrInt(20)))
	writeRunTaskFile(t, env, "010-beta.md", taskFileMarkdown("task-beta", "Beta Task", taskfile.StatusPending, ptrInt(10)))
	selected := writeRunTaskFile(t, env, "010-alpha.md", taskFileMarkdown("task-alpha", "Alpha Task", taskfile.StatusPending, ptrInt(10)))
	writeRunTaskFile(t, env, "001-completed.md", taskFileMarkdown("task-completed", "Completed Task", taskfile.StatusCompleted, ptrInt(1)))

	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Task.ID != "task-alpha" {
		t.Fatalf("selected task id = %q, want task-alpha", result.Task.ID)
	}
	if result.FileTask.SourcePath != selected.SourcePath {
		t.Fatalf("selected task path = %q, want %q", result.FileTask.SourcePath, selected.SourcePath)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("selected file status = %q, want blocked", got)
	}
	if got := loadRunTask(t, env, filepath.Join(taskfile.TasksDir, "010-beta.md")).Status; got != taskfile.StatusPending {
		t.Fatalf("unselected file status = %q, want pending", got)
	}
}

func TestRunSecondPassAfterCompletionReturnsNoTask(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-once", "Do one task")

	state := &fakeCommandState{
		t:                 t,
		workDir:           env.workDir,
		writeReceipt:      true,
		postStatus:        " M internal/feature.go\n",
		verificationExit:  0,
		commitSHA:         "abc123def456",
		expectedCommitAdd: []string{"add", "--", "internal/feature.go"},
	}
	first, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
		LedgerStore:          env.ledger,
		CodexExecutable:      "codex-test",
		GitExecutable:        "git-test",
		VerificationCommands: []verification.Command{{Name: "go", Args: []string{"test", "./..."}}},
		CommandRunner:        state.run,
		Clock:                env.clock,
	})
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if first.Outcome != OutcomeCommitted {
		t.Fatalf("first outcome = %s, want committed", first.Outcome)
	}

	secondState := &fakeCommandState{t: t, workDir: env.workDir}
	second, err := Run(ctx, Config{
		WorkingDir:    env.workDir,
		LedgerStore:   env.ledger,
		CommandRunner: secondState.run,
		Clock:         env.clock,
	})
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if !second.NoTask || second.Outcome != OutcomeNoTask {
		t.Fatalf("second result = %+v, want no task", second)
	}
	if len(secondState.commands) != 0 {
		t.Fatalf("second commands = %#v, want none", secondState.commands)
	}
}

func TestRunSecondPendingTaskAfterSuccessfulFileTaskCommitStartsClean(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	runTestGit(t, workDir, "init", "-q")
	runTestGit(t, workDir, "config", "user.name", "Revolvr Test")
	runTestGit(t, workDir, "config", "user.email", "revolvr-test@example.invalid")
	if err := os.WriteFile(filepath.Join(workDir, ".git", "info", "exclude"), []byte("/.revolvr/\n"), 0o644); err != nil {
		t.Fatalf("write git exclude: %v", err)
	}

	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	ledgerPath := filepath.Join(t.TempDir(), "ledger.sqlite")
	runs, err := ledger.OpenWithClock(ctx, ledgerPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	env := testEnv{workDir: workDir, ledger: runs, now: now}

	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Test repo\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	writeTestRunProfile(t, workDir, prompt.DefaultRunProfileName, defaultRunProfileTemplateContent(t))
	firstTask := writeRunTaskFile(t, env, "010-first.md", taskFileMarkdown("task-first", "First Task", taskfile.StatusPending, ptrInt(10)))
	secondTask := writeRunTaskFile(t, env, "020-second.md", taskFileMarkdown("task-second", "Second Task", taskfile.StatusPending, ptrInt(20)))
	runTestGit(t, workDir, "add", ".")
	runTestGit(t, workDir, "commit", "-q", "-m", "Initial file tasks")

	var codexCalls int
	cfg := Config{
		WorkingDir:  workDir,
		LedgerStore: runs,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			codexCalls++
			path := filepath.Join(cfg.WorkingDir, fmt.Sprintf("generated-%d.txt", codexCalls))
			if err := os.WriteFile(path, []byte(fmt.Sprintf("generated %d\n", codexCalls)), 0o644); err != nil {
				return codexexec.Result{}, err
			}
			return codexexec.Result{ExitCode: 0, FinalMessage: "done"}, nil
		},
		VerificationRunner: func(context.Context, verification.Config) (verification.Result, error) {
			return passedVerificationResult("go test ./..."), nil
		},
		Clock: func() time.Time { return now.Add(2 * time.Minute) },
	}

	first, err := Run(ctx, cfg)
	if err != nil {
		t.Fatalf("first run once: %v", err)
	}
	if first.Outcome != OutcomeCommitted {
		t.Fatalf("first outcome = %s, want committed; message=%s", first.Outcome, first.Message)
	}
	if got := loadRunTask(t, env, firstTask.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("first file task status = %q, want completed", got)
	}
	assertGitStatusClean(t, workDir)

	second, err := Run(ctx, cfg)
	if err != nil {
		t.Fatalf("second run once: %v", err)
	}
	if second.Outcome != OutcomeCommitted {
		t.Fatalf("second outcome = %s, want committed; message=%s", second.Outcome, second.Message)
	}
	if second.Task.ID != "task-second" {
		t.Fatalf("second selected task = %q, want task-second", second.Task.ID)
	}
	if got := loadRunTask(t, env, secondTask.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("second file task status = %q, want completed", got)
	}
	assertGitStatusClean(t, workDir)
}

func TestRunWritesContextBundleWithDefaultProfile(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	profileContent := "File-backed implementer profile.\n\nUse the markdown from .agent/profiles."
	writeTestRunProfile(t, env.workDir, prompt.DefaultRunProfileName, profileContent)
	selected := writeRunTask(t, env, "task-profile", "Use the default run profile")

	var runnerPayload string
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			runnerPayload = cfg.Prompt
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}

	contextPayloadPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.md")
	payloadBytes, err := os.ReadFile(contextPayloadPath)
	if err != nil {
		t.Fatalf("read context payload artifact: %v", err)
	}
	got := string(payloadBytes)
	if got != runnerPayload {
		t.Fatalf("context payload artifact differs from runner payload\n--- artifact ---\n%s\n--- runner ---\n%s", got, runnerPayload)
	}
	required := []string{
		"## Run Profile",
		profileContent,
		"## Selected Task",
		"Task ID: `task-profile`",
		"## Repository Rules",
		"## Artifact Paths",
		"## Required Receipt Schema",
		"## Stop Condition",
	}
	for _, want := range required {
		if !strings.Contains(got, want) {
			t.Fatalf("context payload artifact missing %q:\n%s", want, got)
		}
	}

	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	manifestBytes, err := os.ReadFile(contextManifestPath)
	if err != nil {
		t.Fatalf("read context manifest artifact: %v", err)
	}
	var manifest prompt.ContextManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal context manifest: %v\n%s", err, manifestBytes)
	}
	if manifest.RunID != result.Run.ID || manifest.TaskID != "task-profile" || manifest.ProfileName != prompt.DefaultRunProfileName {
		t.Fatalf("manifest identity = %+v, want run/task/profile", manifest)
	}
	payloadRel := filepath.Join(".revolvr", "runs", result.Run.ID, "context.md")
	if got, want := manifest.ContextPayloadPath, payloadRel; got != want {
		t.Fatalf("manifest payload path = %q, want %q", got, want)
	}
	if got, want := manifest.ContextPayloadSHA256, sha256HexTest(payloadBytes); got != want {
		t.Fatalf("manifest payload sha256 = %q, want %q", got, want)
	}
	if got, want := manifest.ContextPayloadByteSize, len(payloadBytes); got != want {
		t.Fatalf("manifest payload byte size = %d, want %d", got, want)
	}
	if got, want := manifest.GeneratedAt, env.clock().UTC(); !got.Equal(want) {
		t.Fatalf("manifest generated_at = %s, want %s", got, want)
	}
	selectedTask := manifestSourceByLabel(t, manifest, "selected_task")
	if got, want := selectedTask.Path, selected.SourcePath; got != want {
		t.Fatalf("selected task path = %q, want %q", got, want)
	}
	if got, want := selectedTask.SHA256, sha256HexTest(selected.SourceBytes); got != want {
		t.Fatalf("selected task sha256 = %q, want %q", got, want)
	}
	if got, want := selectedTask.ByteSize, len(selected.SourceBytes); got != want {
		t.Fatalf("selected task byte size = %d, want %d", got, want)
	}
	runProfile := manifestSourceByLabel(t, manifest, "run_profile")
	if got, want := runProfile.Path, filepath.Join(".agent", "profiles", "implementer.md"); got != want {
		t.Fatalf("run profile path = %q, want %q", got, want)
	}
	if got, want := runProfile.SHA256, sha256HexTest([]byte(profileContent)); got != want {
		t.Fatalf("run profile sha256 = %q, want %q", got, want)
	}
	if got, want := runProfile.ByteSize, len([]byte(profileContent)); got != want {
		t.Fatalf("run profile byte size = %d, want %d", got, want)
	}

	history, ok, err := env.ledger.GetRunWithEvents(ctx, result.Run.ID)
	if err != nil || !ok {
		t.Fatalf("get run history ok=%v err=%v", ok, err)
	}
	artifacts, found := ledger.RunArtifactsFromEvents(history.Events)
	if !found {
		t.Fatal("run artifacts not found")
	}
	if artifacts.ContextPayloadPath != payloadRel {
		t.Fatalf("ledger context payload path = %q, want %q", artifacts.ContextPayloadPath, payloadRel)
	}
	manifestRel := filepath.Join(".revolvr", "runs", result.Run.ID, "context.json")
	if artifacts.ContextManifestPath != manifestRel {
		t.Fatalf("ledger context manifest path = %q, want %q", artifacts.ContextManifestPath, manifestRel)
	}
}

func TestRunSmokeScriptTaskFileManifestUsesExactPreRunBytes(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTaskFile(t, env, "001-fake-codex-smoke.md", strings.Join([]string{
		"---",
		"id: smoke-fake-codex",
		"status: pending",
		"priority: 10",
		"---",
		"# Fake Codex run smoke",
		"",
		"Run once success path through strict fake Codex",
		"",
	}, "\n"))
	preRunBytes := append([]byte(nil), selected.SourceBytes...)

	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			return codexexec.Result{ExitCode: 1}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}

	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	manifestBytes, err := os.ReadFile(contextManifestPath)
	if err != nil {
		t.Fatalf("read context manifest artifact: %v", err)
	}
	var manifest prompt.ContextManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal context manifest: %v\n%s", err, manifestBytes)
	}
	selectedTask := manifestSourceByLabel(t, manifest, "selected_task")
	if got, want := selectedTask.Path, selected.SourcePath; got != want {
		t.Fatalf("selected task path = %q, want %q", got, want)
	}
	if got, want := selectedTask.SHA256, sha256HexTest(preRunBytes); got != want {
		t.Fatalf("selected task sha256 = %q, want pre-run %q", got, want)
	}
	if got, want := selectedTask.ByteSize, len(preRunBytes); got != want {
		t.Fatalf("selected task byte size = %d, want pre-run %d", got, want)
	}
}

func TestRunBlocksBeforeCodexWhenDefaultProfileMissing(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	if err := os.Remove(filepath.Join(env.workDir, prompt.RunProfileSourcePath(prompt.DefaultRunProfileName))); err != nil {
		t.Fatalf("remove default profile: %v", err)
	}
	writeRunTask(t, env, "task-missing-profile", "Require a profile file")

	codexCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:     env.workDir,
		LedgerStore:    env.ledger,
		DirtyCapture:   cleanDirtyCapture,
		ChangedCapture: emptyChangedCapture,
		CodexRunner: func(context.Context, codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			return codexexec.Result{ExitCode: 0}, nil
		},
		Clock: env.clock,
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}

	if result.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %s, want blocked", result.Outcome)
	}
	if codexCalled {
		t.Fatal("codex runner was called, want profile load failure before Codex starts")
	}
	for _, want := range []string{
		"load run profile failed",
		filepath.Join(".agent", "profiles", "implementer.md"),
		"run `revolvr init` or create the profile file",
	} {
		if !strings.Contains(result.Message, want) {
			t.Fatalf("message missing %q:\n%s", want, result.Message)
		}
	}
	contextPayloadPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.md")
	if _, err := os.Stat(contextPayloadPath); !os.IsNotExist(err) {
		t.Fatalf("context payload artifact stat err = %v, want not exist", err)
	}
}

func TestRunRecordsChangedFilesReceiptWarningWithoutBlockingCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-warning-files", "Create a mismatched receipt")

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
	if result.Task.Status != taskmodel.StatusCompleted || result.Run.Status != ledger.StatusCompleted {
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
	writeRunTask(t, env, "task-warning-verification", "Claim wrong verification")

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
	writeRunTask(t, env, "task-warning-failed", "Fail despite receipt")
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
	if result.Task.Status != taskmodel.StatusBlocked || result.Run.Status != ledger.StatusFailed {
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
	writeRunTask(t, env, "task-no-warning", "Match receipt facts")
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
	selected := writeRunTask(t, env, "task-verify", "Break verification")
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		postStatus:       " M internal/feature.go\n",
		verificationExit: 1,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
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
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
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
	writeRunTask(t, env, "task-no-change", "Make no changes")
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		postStatus:       "",
		verificationExit: 0,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
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
	if result.Task.Status != taskmodel.StatusBlocked {
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
	selected := writeRunTask(t, env, "task-codex", "Codex fails")
	state := &fakeCommandState{
		t:          t,
		workDir:    env.workDir,
		codexExit:  2,
		postStatus: " M internal/partial.go\n",
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
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
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
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

func TestRunBlocksPreExistingDirtyBeforeContextCodexVerificationAndCommit(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-dirty", "Avoid dirty worktree")

	codexCalled := false
	changedCalled := false
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:  env.workDir,
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
	if result.Task.Status != taskmodel.StatusBlocked {
		t.Fatalf("task status = %q, want blocked", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusBlocked {
		t.Fatalf("file task status = %q, want blocked", got)
	}
	if result.Run.Status != ledger.StatusFailed {
		t.Fatalf("run status = %q, want failed", result.Run.Status)
	}
	if !result.ReceiptSynthesized || result.Receipt.Verdict != receipt.VerdictBlocked {
		t.Fatalf("receipt synthesized=%v verdict=%q, want synthesized blocked", result.ReceiptSynthesized, result.Receipt.Verdict)
	}
	contextPayloadPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.md")
	if _, err := os.Stat(contextPayloadPath); !os.IsNotExist(err) {
		t.Fatalf("context payload artifact stat err = %v, want not exist", err)
	}
	contextManifestPath := filepath.Join(env.workDir, ".revolvr", "runs", result.Run.ID, "context.json")
	if _, err := os.Stat(contextManifestPath); !os.IsNotExist(err) {
		t.Fatalf("context manifest artifact stat err = %v, want not exist", err)
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
	selected := writeRunTask(t, env, "task-allow-dirty", "Proceed with dirty worktree")

	codexCalled := false
	verificationCalled := false
	commitCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:                env.workDir,
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
	if result.Task.Status != taskmodel.StatusCompleted {
		t.Fatalf("task status = %q, want completed", result.Task.Status)
	}
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusCompleted {
		t.Fatalf("file task status = %q, want completed", got)
	}
}

func TestRunUpdatesParsedReceiptWhenVerificationFails(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-receipt-verify", "Break verification after receipt")
	state := &fakeCommandState{
		t:                t,
		workDir:          env.workDir,
		writeReceipt:     true,
		postStatus:       " M internal/feature.go\n",
		verificationExit: 1,
	}

	result, err := Run(ctx, Config{
		WorkingDir:           env.workDir,
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

func TestRunRefusesLiveSourceWriterLockBeforeStateMutation(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	selected := writeRunTask(t, env, "task-locked", "Do locked work")

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
	if got := loadRunTask(t, env, selected.SourcePath).Status; got != taskfile.StatusPending {
		t.Fatalf("file task status = %q, want pending", got)
	}
}

func TestRunRefreshesSourceWriterLockWhileCodexRuns(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-heartbeat", "Observe heartbeat")
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
	writeRunTask(t, env, "task-cancel", "Cancel while Codex runs")

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

func TestRunPassesCodexBypassApprovalsAndSandbox(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	writeRunTask(t, env, "task-yolo", "Run without permission gates")

	codexCalled := false
	result, err := Run(ctx, Config{
		WorkingDir:                     env.workDir,
		LedgerStore:                    env.ledger,
		CodexSandbox:                   "workspace-write",
		CodexApprovalPolicy:            "never",
		CodexBypassApprovalsAndSandbox: true,
		DirtyCapture:                   cleanDirtyCapture,
		ChangedCapture:                 emptyChangedCapture,
		Clock:                          env.clock,
		CodexRunner: func(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
			codexCalled = true
			if !cfg.BypassApprovalsAndSandbox {
				t.Fatal("codex bypass approvals and sandbox = false, want true")
			}
			if cfg.Sandbox != "workspace-write" || cfg.ApprovalPolicy != "never" {
				t.Fatalf("codex config sandbox/policy = %q/%q, want workspace-write/never", cfg.Sandbox, cfg.ApprovalPolicy)
			}
			return codexexec.Result{ExitCode: 1}, nil
		},
	})
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if !codexCalled {
		t.Fatal("codex runner was not called")
	}
	if result.Outcome != OutcomeCodexFailed {
		t.Fatalf("outcome = %s, want codex_failed", result.Outcome)
	}
}

type testEnv struct {
	workDir string
	ledger  *ledger.Store
	now     time.Time
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	ctx := context.Background()
	workDir := t.TempDir()
	writeTestRunProfile(t, workDir, prompt.DefaultRunProfileName, defaultRunProfileTemplateContent(t))
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	runs, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	t.Cleanup(func() { _ = runs.Close() })
	return testEnv{workDir: workDir, ledger: runs, now: now}
}

func (e testEnv) clock() time.Time {
	return e.now.Add(2 * time.Minute)
}

func writeTestRunProfile(t *testing.T, workDir string, name string, content string) {
	t.Helper()
	path := filepath.Join(workDir, prompt.RunProfileSourcePath(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimRight(content, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write profile file: %v", err)
	}
}

func writeRunTask(t *testing.T, env testEnv, id string, title string) taskfile.Task {
	t.Helper()
	return writeRunTaskFile(t, env, id+".md", taskFileMarkdown(id, title, taskfile.StatusPending, nil))
}

func writeRunTaskFile(t *testing.T, env testEnv, name string, content string) taskfile.Task {
	t.Helper()
	path := filepath.Join(env.workDir, taskfile.TasksDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create task dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}
	task, err := taskfile.Load(env.workDir, filepath.Join(taskfile.TasksDir, name))
	if err != nil {
		t.Fatalf("load task file: %v", err)
	}
	return task
}

func loadRunTask(t *testing.T, env testEnv, sourcePath string) taskfile.Task {
	t.Helper()
	task, err := taskfile.Load(env.workDir, sourcePath)
	if err != nil {
		t.Fatalf("load task file %s: %v", sourcePath, err)
	}
	return task
}

func taskFileMarkdown(id string, title string, status string, priority *int) string {
	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "id: %s\n", id)
	fmt.Fprintf(&out, "status: %s\n", status)
	if priority != nil {
		fmt.Fprintf(&out, "priority: %d\n", *priority)
	}
	out.WriteString("---\n")
	fmt.Fprintf(&out, "# %s\n\n", title)
	fmt.Fprintf(&out, "%s\n", title)
	return out.String()
}

func ptrInt(value int) *int {
	return &value
}

func defaultRunProfileTemplateContent(t *testing.T) string {
	t.Helper()
	for _, template := range prompt.DefaultRunProfileTemplates() {
		if template.Name == prompt.DefaultRunProfileName {
			return template.Content
		}
	}
	t.Fatalf("default profile template %q not found", prompt.DefaultRunProfileName)
	return ""
}

func passedVerificationResult(command string) verification.Result {
	return verification.Result{
		Status:             verification.StatusPassed,
		Passed:             true,
		FailedCommandIndex: -1,
		Commands: []verification.CommandResult{{
			Command:  command,
			Status:   verification.StatusPassed,
			Passed:   true,
			ExitCode: 0,
		}},
	}
}

func runTestGit(t *testing.T, workDir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func assertGitStatusClean(t *testing.T, workDir string) {
	t.Helper()
	cmd := exec.Command("git", "status", "--short", "--untracked-files=all")
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status failed: %v\n%s", err, output)
	}
	if len(output) != 0 {
		t.Fatalf("git status = %q, want clean", output)
	}
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
	contextPayload := readContextPayload(s.t, command.Stdin)
	receiptRel := contextPayloadValue(s.t, contextPayload, "Receipt path")
	runID := contextPayloadValue(s.t, contextPayload, "Run ID")
	taskID := contextPayloadValue(s.t, contextPayload, "Task ID")
	taskText := contextPayloadTaskText(s.t, contextPayload)
	if s.writeReceipt {
		content := validReceipt(runID, taskID, taskText)
		if s.receiptContent != nil {
			content = s.receiptContent(runID, taskID, taskText)
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

func readContextPayload(t *testing.T, reader io.Reader) string {
	t.Helper()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read context payload: %v", err)
	}
	return string(content)
}

func contextPayloadValue(t *testing.T, payload string, label string) string {
	t.Helper()
	prefix := "- " + label + ": `"
	for _, line := range strings.Split(payload, "\n") {
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimPrefix(line, prefix)
			value = strings.TrimSuffix(value, "`")
			return value
		}
	}
	t.Fatalf("context payload missing %s:\n%s", label, payload)
	return ""
}

func contextPayloadTaskText(t *testing.T, payload string) string {
	t.Helper()
	lines := strings.Split(payload, "\n")
	for i, line := range lines {
		if line != "- Task text:" {
			continue
		}
		if i+2 >= len(lines) || lines[i+2] != "```text" {
			t.Fatalf("context payload task text block malformed:\n%s", payload)
		}
		start := i + 3
		for end := start; end < len(lines); end++ {
			if lines[end] == "```" {
				return strings.Join(lines[start:end], "\n")
			}
		}
		t.Fatalf("context payload task text block not closed:\n%s", payload)
	}
	t.Fatalf("context payload missing task text:\n%s", payload)
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

func manifestSourceByLabel(t *testing.T, manifest prompt.ContextManifest, label string) prompt.ContextSource {
	t.Helper()
	for _, source := range manifest.Sources {
		if source.Label == label {
			return source
		}
	}
	t.Fatalf("manifest source %q not found: %+v", label, manifest.Sources)
	return prompt.ContextSource{}
}

func sha256HexTest(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
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
