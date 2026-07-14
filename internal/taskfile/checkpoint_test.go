package taskfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"revolvr/internal/operatorcheckpoint"
)

func TestLoadOperatorCheckpointMetadata(t *testing.T) {
	root := t.TempDir()
	taskID := "license-acceptance"
	receiptPath := operatorcheckpoint.ExpectedReceiptPath(taskID)
	path := writeTaskFile(t, root, "license.md", fmt.Sprintf(`---
id: %s
status: pending
workflow: operator-checkpoint-v1
checkpoint_receipt_path: %s
depends_on: asset-review
---
# Accept asset license
`, taskID, receiptPath))
	task, err := Load(root, path)
	if err != nil {
		t.Fatalf("load pending checkpoint: %v", err)
	}
	if task.Workflow != WorkflowOperatorCheckpointV1 || task.Status != StatusPending || task.CheckpointReceiptPath != receiptPath || task.CheckpointReceiptSHA256 != "" {
		t.Fatalf("checkpoint task = %#v", task)
	}

	sha := strings.Repeat("a", 64)
	completedPath := writeTaskFile(t, root, "hardware.md", fmt.Sprintf(`---
id: hardware-acceptance
status: completed
workflow: operator-checkpoint-v1
checkpoint_receipt_path: %s
checkpoint_receipt_sha256: %s
---
# Accept hardware run
`, operatorcheckpoint.ExpectedReceiptPath("hardware-acceptance"), sha))
	completed, err := Load(root, completedPath)
	if err != nil {
		t.Fatalf("load completed checkpoint: %v", err)
	}
	if completed.CheckpointReceiptSHA256 != sha {
		t.Fatalf("completed receipt identity = %q", completed.CheckpointReceiptSHA256)
	}
}

func TestFulfillOperatorCheckpointIsAtomicAndReplaySafe(t *testing.T) {
	root := t.TempDir()
	taskID := "manual-acceptance"
	receiptPath := operatorcheckpoint.ExpectedReceiptPath(taskID)
	original := []byte("---\r\nid: " + taskID + "\r\nstatus: pending\r\nworkflow: operator-checkpoint-v1\r\ncheckpoint_receipt_path: " + receiptPath + "\r\nx-owner: release\r\n---\r\n# Manual acceptance\r\n\r\nKeep exact body bytes.\r\n")
	path := writeTaskFile(t, root, taskID+".md", string(original))
	snapshot, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	receiptSHA := strings.Repeat("a", 64)
	updated, changed, err := FulfillOperatorCheckpoint(root, snapshot, receiptSHA)
	if err != nil {
		t.Fatalf("fulfill checkpoint: %v", err)
	}
	if !changed || updated.Status != StatusCompleted || updated.CheckpointReceiptSHA256 != receiptSHA {
		t.Fatalf("fulfillment changed=%t task=%+v", changed, updated)
	}
	want := bytes.Replace(original, []byte("status: pending\r\n"), []byte("status: completed\r\n"), 1)
	want = bytes.Replace(want, []byte("checkpoint_receipt_path: "+receiptPath+"\r\n"), []byte("checkpoint_receipt_path: "+receiptPath+"\r\ncheckpoint_receipt_sha256: "+receiptSHA+"\r\n"), 1)
	if !bytes.Equal(updated.SourceBytes, want) {
		t.Fatalf("fulfilled bytes changed unrelated content:\ngot  %q\nwant %q", updated.SourceBytes, want)
	}

	replayed, changed, err := FulfillOperatorCheckpoint(root, updated, receiptSHA)
	if err != nil {
		t.Fatalf("replay fulfillment: %v", err)
	}
	if changed || !bytes.Equal(replayed.SourceBytes, want) {
		t.Fatalf("replay changed=%t bytes=%q", changed, replayed.SourceBytes)
	}
	beforeConflict, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := FulfillOperatorCheckpoint(root, replayed, strings.Repeat("b", 64)); err == nil || !strings.Contains(err.Error(), "conflicting replay") {
		t.Fatalf("conflicting replay error = %v", err)
	}
	afterConflict, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeConflict, afterConflict) {
		t.Fatal("conflicting replay mutated task bytes")
	}
}

func TestFulfillOperatorCheckpointAtomicWriteFailureLeavesBytesUnchanged(t *testing.T) {
	root := t.TempDir()
	taskID := "manual-acceptance"
	path := writeTaskFile(t, root, taskID+".md", fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: operator-checkpoint-v1\ncheckpoint_receipt_path: %s\n---\n# Manual acceptance\n", taskID, operatorcheckpoint.ExpectedReceiptPath(taskID)))
	snapshot, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("injected atomic write failure")
	originalWriter := writeCheckpointFileAtomically
	writeCheckpointFileAtomically = func(string, []byte, os.FileMode) error { return sentinel }
	t.Cleanup(func() { writeCheckpointFileAtomically = originalWriter })

	if _, _, err := FulfillOperatorCheckpoint(root, snapshot, strings.Repeat("a", 64)); !errors.Is(err, sentinel) {
		t.Fatalf("fulfillment error = %v, want injected error", err)
	}
	current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, snapshot.SourceBytes) {
		t.Fatal("failed atomic publication changed task bytes")
	}
}

func TestLoadRejectsInvalidOperatorCheckpointMetadata(t *testing.T) {
	root := t.TempDir()
	validPath := operatorcheckpoint.ExpectedReceiptPath("checkpoint")
	tests := []struct {
		name string
		meta string
		want string
	}{
		{name: "missing receipt path", meta: "", want: `frontmatter key "checkpoint_receipt_path" is required`},
		{name: "unsafe receipt path", meta: "checkpoint_receipt_path: ../receipt.json\n", want: "must be"},
		{name: "alternate receipt path", meta: "checkpoint_receipt_path: .agent/checkpoints/other/receipt.json\n", want: "must be"},
		{name: "pending receipt identity", meta: "checkpoint_receipt_path: " + validPath + "\ncheckpoint_receipt_sha256: " + strings.Repeat("a", 64) + "\n", want: `checkpoint_receipt_sha256" is not allowed for pending`},
		{name: "completed missing identity", meta: "status: completed\ncheckpoint_receipt_path: " + validPath + "\n", want: "must be a lowercase SHA-256"},
		{name: "completed malformed identity", meta: "status: completed\ncheckpoint_receipt_path: " + validPath + "\ncheckpoint_receipt_sha256: ABC\n", want: "must be a lowercase SHA-256"},
		{name: "running status", meta: "status: running\ncheckpoint_receipt_path: " + validPath + "\n", want: `invalid status "running"`},
		{name: "phase", meta: "phase: implement\ncheckpoint_receipt_path: " + validPath + "\n", want: "phase, profile, and autonomous_state_path are not allowed"},
		{name: "profile", meta: "profile: implementer\ncheckpoint_receipt_path: " + validPath + "\n", want: "phase, profile, and autonomous_state_path are not allowed"},
		{name: "autonomous state", meta: "autonomous_state_path: .revolvr/autonomous/tasks/checkpoint/state.json\ncheckpoint_receipt_path: " + validPath + "\n", want: "phase, profile, and autonomous_state_path are not allowed"},
		{name: "child lineage", meta: "parent_task_id: parent\nchild_proposal_id: proposal\nchild_decision_id: decision\nchild_run_id: run\nchild_evidence: task:parent\nparent_behavior: independent\ncheckpoint_receipt_path: " + validPath + "\n", want: "autonomous child lineage is not allowed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTaskFile(t, root, "checkpoint.md", "---\nid: checkpoint\nworkflow: operator-checkpoint-v1\n"+test.meta+"---\n# Checkpoint\n")
			_, err := Load(root, path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("load error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestCheckpointReceiptMetadataIsWorkflowScoped(t *testing.T) {
	root := t.TempDir()
	for _, workflow := range []string{WorkflowMixedPassV1, WorkflowAutonomousV1} {
		t.Run(workflow, func(t *testing.T) {
			extra := ""
			if workflow == WorkflowAutonomousV1 {
				extra = "autonomous_state_path: " + autonomousStatePath("task") + "\n"
			}
			path := writeTaskFile(t, root, workflow+".md", fmt.Sprintf("---\nid: task\nworkflow: %s\n%scheckpoint_receipt_path: %s\n---\n# Task\n", workflow, extra, operatorcheckpoint.ExpectedReceiptPath("task")))
			_, err := Load(root, path)
			if err == nil || !strings.Contains(err.Error(), "checkpoint receipt metadata is not allowed") {
				t.Fatalf("load error = %v", err)
			}
		})
	}
}
