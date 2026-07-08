package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/taskqueue"
)

func TestStatusUninitializedDoesNotCreateState(t *testing.T) {
	workDir := t.TempDir()

	result, err := Status(context.Background(), Config{WorkDir: workDir})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if result.Initialized {
		t.Fatal("status initialized = true, want false")
	}
	if _, err := os.Stat(filepath.Join(workDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}
}

func TestStatusReturnsTasksRecentRunsAndLatestEvents(t *testing.T) {
	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	base := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
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

	eventTime := base
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	for _, spec := range []ledger.RunSpec{
		{ID: "run-old", TaskID: "task-old", Task: "old run", Status: ledger.StatusCompleted, StartedAt: base},
		{ID: "run-new", TaskID: "task-new", Task: "new run", Status: ledger.StatusFailed, StartedAt: base.Add(time.Hour)},
	} {
		if _, err := runs.CreateRun(ctx, spec); err != nil {
			t.Fatalf("create %s: %v", spec.ID, err)
		}
	}
	if _, err := runs.AppendEvent(ctx, "run-old", ledger.EventRunStarted, map[string]any{"run_id": "run-old"}); err != nil {
		t.Fatalf("append old event: %v", err)
	}
	eventTime = base.Add(time.Hour)
	if _, err := runs.AppendEvent(ctx, "run-new", ledger.EventRunStarted, map[string]any{"run_id": "run-new"}); err != nil {
		t.Fatalf("append new start event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-new", ledger.EventRunFailed, map[string]any{"message": "failed"}); err != nil {
		t.Fatalf("append new failed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	result, err := Status(ctx, Config{WorkDir: workDir, RecentRunsLimit: 1})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !result.Initialized {
		t.Fatal("status initialized = false, want true")
	}
	if got, want := taskStatuses(result.Tasks), map[string]string{
		"task-pending":   taskqueue.StatusPending,
		"task-blocked":   taskqueue.StatusBlocked,
		"task-completed": taskqueue.StatusCompleted,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task statuses = %#v, want %#v", got, want)
	}
	if got, want := runIDs(result.RecentRuns), []string{"run-new"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent runs = %#v, want %#v", got, want)
	}
	if got, want := eventTypes(result.LatestEvents), []ledger.EventType{ledger.EventRunStarted, ledger.EventRunFailed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("latest event types = %#v, want %#v", got, want)
	}
}

func TestShowRunReturnsPersistedHistory(t *testing.T) {
	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}

	ctx := context.Background()
	startedAt := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	eventTime := startedAt
	runs, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, func() time.Time { return eventTime })
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if _, err := runs.CreateRun(ctx, ledger.RunSpec{
		ID:        "run-show",
		TaskID:    "task-show",
		Task:      "Show one run",
		Status:    ledger.StatusCompleted,
		StartedAt: startedAt,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runs.AppendEvent(ctx, "run-show", ledger.EventRunStarted, map[string]any{"run_id": "run-show"}); err != nil {
		t.Fatalf("append start event: %v", err)
	}
	eventTime = eventTime.Add(time.Second)
	if _, err := runs.AppendEvent(ctx, "run-show", ledger.EventRunCompleted, map[string]any{"summary": "done"}); err != nil {
		t.Fatalf("append completed event: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	history, err := ShowRun(ctx, Config{WorkDir: workDir}, " run-show ")
	if err != nil {
		t.Fatalf("show run: %v", err)
	}
	if got, want := history.Run.ID, "run-show"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := eventTypes(history.Events), []ledger.EventType{ledger.EventRunStarted, ledger.EventRunCompleted}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %#v, want %#v", got, want)
	}
}

func TestShowRunReportsUninitializedAndMissingRun(t *testing.T) {
	ctx := context.Background()
	uninitializedDir := t.TempDir()

	if _, err := ShowRun(ctx, Config{WorkDir: uninitializedDir}, "run-missing-state"); err == nil || !strings.Contains(err.Error(), "state is not initialized") {
		t.Fatalf("show uninitialized error = %v, want state not initialized", err)
	}
	if _, err := os.Stat(filepath.Join(uninitializedDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}

	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	if _, err := ShowRun(ctx, Config{WorkDir: workDir}, "missing-run"); err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("show missing run error = %v, want not found", err)
	}
}

func TestValidateReceiptReturnsPersistedValidationResult(t *testing.T) {
	workDir := t.TempDir()
	completedAt := time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC)
	createAppValidationRun(t, workDir, appValidationRunSpec{
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

	result, err := ValidateReceipt(context.Background(), Config{WorkDir: workDir}, " run-valid-receipt ")
	if err != nil {
		t.Fatalf("validate receipt: %v", err)
	}
	if got, want := result.RunID, "run-valid-receipt"; got != want {
		t.Fatalf("run id = %q, want %q", got, want)
	}
	if got, want := result.ReceiptPath, filepath.Join(".revolvr", "receipts", "run-valid-receipt.md"); got != want {
		t.Fatalf("receipt path = %q, want %q", got, want)
	}
	if !result.Passed() {
		t.Fatalf("validation passed = false, checks = %#v", result.Checks)
	}
	wantChecks := map[string]string{
		receipt.ValidationCheckIdentity:            "ok",
		receipt.ValidationCheckCompletionTime:      "ok",
		receipt.ValidationCheckCommitSHA:           "ok",
		receipt.ValidationCheckChangedFiles:        "ok",
		receipt.ValidationCheckVerificationResults: "ok",
		receipt.ValidationCheckArtifacts:           "ok",
	}
	if got := validationCheckMessages(result); !reflect.DeepEqual(got, wantChecks) {
		t.Fatalf("check messages = %#v, want %#v", got, wantChecks)
	}
}

func TestValidateReceiptReturnsFailedChecksWithoutCommandError(t *testing.T) {
	workDir := t.TempDir()
	completedAt := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	createAppValidationRun(t, workDir, appValidationRunSpec{
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
		ReceiptTimestamp:    completedAt.Add(time.Minute),
		ReceiptChangedFiles: []string{"internal/stale.go"},
		WriteArtifacts:      true,
	})

	result, err := ValidateReceipt(context.Background(), Config{WorkDir: workDir}, "run-invalid-receipt")
	if err != nil {
		t.Fatalf("validate receipt: %v", err)
	}
	if result.Passed() {
		t.Fatalf("validation passed = true, want failed checks")
	}
	if got, want := failedValidationCheckNames(result), []string{
		receipt.ValidationCheckCompletionTime,
		receipt.ValidationCheckChangedFiles,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failed check names = %#v, want %#v", got, want)
	}
}

func TestValidateReceiptReportsUninitializedMissingAndEmptyRun(t *testing.T) {
	ctx := context.Background()
	uninitializedDir := t.TempDir()

	if _, err := ValidateReceipt(ctx, Config{WorkDir: uninitializedDir}, " "); err == nil || !strings.Contains(err.Error(), "receipt validate: run id is required") {
		t.Fatalf("validate empty run id error = %v, want required run id", err)
	}
	if _, err := ValidateReceipt(ctx, Config{WorkDir: uninitializedDir}, "run-missing-state"); err == nil || !strings.Contains(err.Error(), "state is not initialized") {
		t.Fatalf("validate uninitialized error = %v, want state not initialized", err)
	}
	if _, err := os.Stat(filepath.Join(uninitializedDir, stateDirName)); !os.IsNotExist(err) {
		t.Fatalf("state dir stat err = %v, want not exist", err)
	}

	workDir := t.TempDir()
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		t.Fatalf("resolve state paths: %v", err)
	}
	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		t.Fatalf("open ledger store: %v", err)
	}
	if err := runs.Close(); err != nil {
		t.Fatalf("close ledger store: %v", err)
	}

	if _, err := ValidateReceipt(ctx, Config{WorkDir: workDir}, "missing-run"); err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("validate missing run error = %v, want not found", err)
	}
}

func taskStatuses(tasks []taskqueue.Task) map[string]string {
	statuses := make(map[string]string, len(tasks))
	for _, task := range tasks {
		statuses[task.ID] = task.Status
	}
	return statuses
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

type appValidationRunSpec struct {
	RunID               string
	TaskID              string
	Task                string
	CompletedAt         time.Time
	CommitSHA           string
	ChangedFiles        []string
	VerificationStatus  string
	Verification        []receipt.VerificationEntry
	ReceiptTimestamp    time.Time
	ReceiptChangedFiles []string
	WriteArtifacts      bool
}

func createAppValidationRun(t *testing.T, workDir string, spec appValidationRunSpec) {
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
	artifacts := ledger.RunArtifacts{
		PromptPath:           filepath.Join(".revolvr", "runs", spec.RunID, "prompt.md"),
		CodexStdoutJSONLPath: filepath.Join(".revolvr", "runs", spec.RunID, "codex.jsonl"),
		CodexStderrPath:      filepath.Join(".revolvr", "runs", spec.RunID, "codex.stderr"),
		LastMessagePath:      filepath.Join(".revolvr", "runs", spec.RunID, "last-message.txt"),
		ReceiptPath:          filepath.Join(".revolvr", "receipts", spec.RunID+".md"),
	}
	if _, err := runs.AppendEvent(ctx, spec.RunID, ledger.EventRunArtifacts, artifacts); err != nil {
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
		"commands": appValidationCommandPayloads(spec.Verification),
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
		writeAppTestFile(t, filepath.Join(workDir, artifacts.PromptPath), "prompt")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.CodexStdoutJSONLPath), "{}\n")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.CodexStderrPath), "")
		writeAppTestFile(t, filepath.Join(workDir, artifacts.LastMessagePath), "done")
	}

	receiptTimestamp := spec.ReceiptTimestamp
	if receiptTimestamp.IsZero() {
		receiptTimestamp = spec.CompletedAt
	}
	receiptChangedFiles := spec.ReceiptChangedFiles
	if receiptChangedFiles == nil {
		receiptChangedFiles = spec.ChangedFiles
	}
	content, _ := receipt.FormatFallbackReceipt(receipt.FallbackInput{
		RunID:              spec.RunID,
		PassID:             spec.RunID,
		TaskID:             spec.TaskID,
		Task:               spec.Task,
		Verdict:            receipt.VerdictCompleted,
		Timestamp:          receiptTimestamp,
		CodexExitCode:      0,
		VerificationStatus: spec.VerificationStatus,
		CommitSHA:          spec.CommitSHA,
		ChangedFiles:       receiptChangedFiles,
		Verification:       spec.Verification,
		Metrics:            receipt.Metrics{},
		FinalText:          "completed",
	})
	writeAppTestFile(t, filepath.Join(workDir, artifacts.ReceiptPath), content)
}

func appValidationCommandPayloads(entries []receipt.VerificationEntry) []map[string]any {
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

func writeAppTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func validationCheckMessages(result receipt.ValidationResult) map[string]string {
	messages := make(map[string]string, len(result.Checks))
	for _, check := range result.Checks {
		messages[check.Name] = check.Message()
	}
	return messages
}

func failedValidationCheckNames(result receipt.ValidationResult) []string {
	failures := result.Failures()
	names := make([]string, 0, len(failures))
	for _, check := range failures {
		names = append(names, check.Name)
	}
	return names
}
