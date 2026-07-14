package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/operatorcheckpoint"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
)

func TestFulfillCheckpointPublishesAuthorityAndUnlocksDependent(t *testing.T) {
	root := t.TempDir()
	taskID := "manual-acceptance"
	receiptPath := writeAppCheckpointFixture(t, root, taskID, validAppCheckpointReceipt(taskID))
	writeAppTaskFile(t, root, "release.md", "---\nid: release\nstatus: pending\ndepends_on: "+taskID+"\n---\n# Release\n")

	before, err := ListTasks(context.Background(), Config{WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := appTaskByID(t, before, taskID)
	if checkpoint.CheckpointState != taskmodel.CheckpointStateAwaiting || checkpoint.ReadinessReason != "awaiting_operator" {
		t.Fatalf("awaiting checkpoint projection = %+v", checkpoint)
	}
	originalTaskBytes := append([]byte(nil), checkpoint.Task...)

	result, err := FulfillCheckpoint(context.Background(), Config{WorkDir: root}, FulfillCheckpointInput{
		TaskID: taskID, ReceiptPath: receiptPath, Operator: "operator@example.test",
	})
	if err != nil {
		t.Fatalf("fulfill checkpoint: %v", err)
	}
	if result.Replayed || result.Task.CheckpointState != taskmodel.CheckpointStateFulfilled || result.Task.ReadinessReason != "completed" {
		t.Fatalf("fulfillment result = %+v", result)
	}
	selected, found := result.Schedule.SelectedForWorkflow("mixed-pass-v1")
	if !found || selected.TaskID != "release" {
		t.Fatalf("dependent selection found=%t selected=%+v", found, selected)
	}
	if !strings.Contains(result.Task.Task, "status: completed") || !strings.Contains(result.Task.Task, "checkpoint_receipt_sha256: "+result.ReceiptSHA256) {
		t.Fatalf("fulfilled task bytes = %q", result.Task.Task)
	}

	fulfilledBytes := []byte(result.Task.Task)
	replayed, err := FulfillCheckpoint(context.Background(), Config{WorkDir: root}, FulfillCheckpointInput{
		TaskID: taskID, ReceiptPath: receiptPath, Operator: "operator@example.test",
	})
	if err != nil {
		t.Fatalf("replay checkpoint: %v", err)
	}
	if !replayed.Replayed || !bytes.Equal([]byte(replayed.Task.Task), fulfilledBytes) {
		t.Fatalf("replay result = %+v", replayed)
	}
	if bytes.Equal(originalTaskBytes, fulfilledBytes) {
		t.Fatal("fulfillment did not change task authority")
	}

	after, err := ListTasks(context.Background(), Config{WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := appTaskByID(t, after, taskID).CheckpointState; got != taskmodel.CheckpointStateFulfilled {
		t.Fatalf("fulfilled checkpoint state = %q", got)
	}
	if dependent := appTaskByID(t, after, "release"); !dependent.NextRunnable || dependent.ReadinessReason != "ready" {
		t.Fatalf("dependent projection = %+v", dependent)
	}
}

func TestFulfillCheckpointRejectsInvalidAuthorityWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		input   func(string, string) FulfillCheckpointInput
		prepare func(*testing.T, string, string)
		want    string
	}{
		{name: "wrong path", input: func(taskID, _ string) FulfillCheckpointInput {
			return FulfillCheckpointInput{TaskID: taskID, ReceiptPath: ".agent/checkpoints/other/receipt.json", Operator: "operator@example.test"}
		}, want: "does not match canonical checkpoint path"},
		{name: "wrong operator", input: func(taskID, path string) FulfillCheckpointInput {
			return FulfillCheckpointInput{TaskID: taskID, ReceiptPath: path, Operator: "other@example.test"}
		}, want: "does not match receipt operator"},
		{name: "wrong task", input: func(_ string, path string) FulfillCheckpointInput {
			return FulfillCheckpointInput{TaskID: "other", ReceiptPath: path, Operator: "operator@example.test"}
		}, want: `task "other" not found`},
		{name: "mismatched receipt task", input: validAppCheckpointInput, prepare: func(t *testing.T, root, taskID string) {
			writeAppCheckpointReceiptAt(t, root, taskID, validAppCheckpointReceipt("other"))
		}, want: "does not match checkpoint"},
		{name: "malformed receipt", input: validAppCheckpointInput, prepare: func(t *testing.T, root, taskID string) {
			writeAppCheckpointReceiptRaw(t, root, taskID, []byte(`{"schema_version":`))
		}, want: "decode operator checkpoint receipt"},
		{name: "invalid graph", input: validAppCheckpointInput, prepare: func(t *testing.T, root, _ string) {
			writeAppTaskFile(t, root, "invalid.md", "---\nid: invalid\nstatus: pending\ndepends_on: missing\n---\n# Invalid\n")
		}, want: "task graph is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			taskID := "manual-acceptance"
			receiptPath := writeAppCheckpointFixture(t, root, taskID, validAppCheckpointReceipt(taskID))
			if test.prepare != nil {
				test.prepare(t, root, taskID)
			}
			taskPath := filepath.Join(root, taskfile.TasksDir, taskID+".md")
			before, err := os.ReadFile(taskPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = FulfillCheckpoint(context.Background(), Config{WorkDir: root}, test.input(taskID, receiptPath))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("fulfillment error = %v, want %q", err, test.want)
			}
			after, err := os.ReadFile(taskPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("invalid fulfillment mutated task bytes")
			}
		})
	}
}

func TestFulfillCheckpointRejectsReplacedReceiptAsConflictingReplay(t *testing.T) {
	root := t.TempDir()
	taskID := "manual-acceptance"
	receiptPath := writeAppCheckpointFixture(t, root, taskID, validAppCheckpointReceipt(taskID))
	input := validAppCheckpointInput(taskID, receiptPath)
	if _, err := FulfillCheckpoint(context.Background(), Config{WorkDir: root}, input); err != nil {
		t.Fatal(err)
	}
	taskPath := filepath.Join(root, taskfile.TasksDir, taskID+".md")
	boundBytes, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	replaced := validAppCheckpointReceipt(taskID)
	replaced.Decision = "Accepted after replacement review."
	writeAppCheckpointReceiptAt(t, root, taskID, replaced)
	if _, err := FulfillCheckpoint(context.Background(), Config{WorkDir: root}, input); err == nil || !strings.Contains(err.Error(), "conflicting replay") {
		t.Fatalf("replaced receipt error = %v", err)
	}
	current, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(boundBytes, current) {
		t.Fatal("conflicting replay mutated task bytes")
	}
}

func validAppCheckpointInput(taskID, receiptPath string) FulfillCheckpointInput {
	return FulfillCheckpointInput{TaskID: taskID, ReceiptPath: receiptPath, Operator: "operator@example.test"}
}

func writeAppCheckpointFixture(t *testing.T, root, taskID string, receipt operatorcheckpoint.Receipt) string {
	t.Helper()
	receiptPath := operatorcheckpoint.ExpectedReceiptPath(taskID)
	writeAppTaskFile(t, root, taskID+".md", fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: operator-checkpoint-v1\ncheckpoint_receipt_path: %s\n---\n# Manual acceptance\n", taskID, receiptPath))
	writeAppCheckpointReceiptAt(t, root, taskID, receipt)
	return receiptPath
}

func writeAppCheckpointReceiptAt(t *testing.T, root, pathTaskID string, receipt operatorcheckpoint.Receipt) {
	t.Helper()
	raw, err := operatorcheckpoint.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	writeAppCheckpointReceiptRaw(t, root, pathTaskID, raw)
}

func writeAppCheckpointReceiptRaw(t *testing.T, root, taskID string, raw []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(operatorcheckpoint.ExpectedReceiptPath(taskID)))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func validAppCheckpointReceipt(taskID string) operatorcheckpoint.Receipt {
	return operatorcheckpoint.Receipt{
		SchemaVersion: operatorcheckpoint.ReceiptSchemaVersion,
		TaskID:        taskID,
		Outcome:       operatorcheckpoint.OutcomeAccepted,
		Operator:      "operator@example.test",
		Provenance:    "manual repository review",
		AcceptedAt:    time.Date(2026, 7, 14, 19, 0, 0, 0, time.UTC),
		Subject:       "Manual acceptance",
		Decision:      "Accepted.",
		Evidence: []operatorcheckpoint.EvidenceReference{{
			Kind: operatorcheckpoint.EvidenceArtifact, Path: "artifacts/manual-review.txt", SHA256: appCheckpointHash("evidence"),
		}},
	}
}

func appCheckpointHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func appTaskByID(t *testing.T, tasks []taskmodel.Task, taskID string) taskmodel.Task {
	t.Helper()
	for _, task := range tasks {
		if task.ID == taskID {
			return task
		}
	}
	t.Fatalf("task %q not found in %+v", taskID, tasks)
	return taskmodel.Task{}
}
