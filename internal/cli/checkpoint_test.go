package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/operatorcheckpoint"
)

func TestCheckpointFulfillCommandAndReadSurfaces(t *testing.T) {
	workDir := t.TempDir()
	taskID := "manual-acceptance"
	receiptPath, receiptSHA := writeCLICheckpointFixture(t, workDir, taskID)
	writeCLITaskFile(t, workDir, "release.md", "---\nid: release\nstatus: pending\ndepends_on: "+taskID+"\n---\n# Release\n")

	listBefore, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listBefore, taskID+"\tpending\toperator-checkpoint-v1") || !strings.Contains(listBefore, "\tawaiting\t"+receiptPath+"\n") {
		t.Fatalf("awaiting task list projection missing:\n%s", listBefore)
	}

	out, err := executeCLI(t, workDir, "checkpoint", "fulfill", taskID, "--receipt", receiptPath, "--operator", "operator@example.test")
	if err != nil {
		t.Fatalf("fulfill checkpoint: %v", err)
	}
	want := fmt.Sprintf("Checkpoint %s fulfilled: receipt=%s sha256=%s\n", taskID, receiptPath, receiptSHA)
	if out != want {
		t.Fatalf("fulfillment output = %q, want %q", out, want)
	}
	assertNoCLIStateDir(t, workDir)

	replay, err := executeCLI(t, workDir, "checkpoint", "fulfill", taskID, "--receipt", receiptPath, "--operator", "operator@example.test")
	if err != nil {
		t.Fatalf("replay checkpoint: %v", err)
	}
	if want := fmt.Sprintf("Checkpoint %s already fulfilled: receipt=%s sha256=%s\n", taskID, receiptPath, receiptSHA); replay != want {
		t.Fatalf("replay output = %q, want %q", replay, want)
	}

	listAfter, err := executeCLI(t, workDir, "task", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listAfter, taskID+"\tcompleted\toperator-checkpoint-v1") || !strings.Contains(listAfter, "\tfulfilled\t"+receiptPath+"\n") || !strings.Contains(listAfter, "release\tpending\tmixed-pass-v1\timplement\timplementer\taudit\tmixed-pass-v1\tready") {
		t.Fatalf("fulfilled task list projection missing:\n%s", listAfter)
	}

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatal(err)
	}
	status, err := executeCLI(t, workDir, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Operator checkpoint: " + taskID + " state=fulfilled receipt=" + receiptPath + " sha256=" + receiptSHA,
		"Next task: release - Release",
	} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status missing %q:\n%s", expected, status)
		}
	}
}

func TestCheckpointFulfillHelpAndRequiredAuthority(t *testing.T) {
	workDir := t.TempDir()
	out, err := executeCLI(t, workDir, "checkpoint", "fulfill", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fulfill <task-id>", "--receipt", "--operator"} {
		if !strings.Contains(out, want) {
			t.Fatalf("checkpoint help missing %q:\n%s", want, out)
		}
	}

	_, err = executeCLI(t, workDir, "checkpoint", "fulfill", "manual-acceptance")
	if err == nil || !strings.Contains(err.Error(), "--receipt is required") {
		t.Fatalf("missing authority error = %v", err)
	}
	assertNoCLIStateDir(t, workDir)
}

func writeCLICheckpointFixture(t *testing.T, root, taskID string) (string, string) {
	t.Helper()
	receiptPath := operatorcheckpoint.ExpectedReceiptPath(taskID)
	writeCLITaskFile(t, root, taskID+".md", fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: operator-checkpoint-v1\ncheckpoint_receipt_path: %s\n---\n# Manual acceptance\n", taskID, receiptPath))
	receipt := operatorcheckpoint.Receipt{
		SchemaVersion: operatorcheckpoint.ReceiptSchemaVersion,
		TaskID:        taskID,
		Outcome:       operatorcheckpoint.OutcomeAccepted,
		Operator:      "operator@example.test",
		Provenance:    "manual repository review",
		AcceptedAt:    time.Date(2026, 7, 14, 20, 0, 0, 0, time.UTC),
		Subject:       "Manual acceptance",
		Decision:      "Accepted.",
		Evidence: []operatorcheckpoint.EvidenceReference{{
			Kind: operatorcheckpoint.EvidenceArtifact, Path: "artifacts/manual-review.txt", SHA256: strings.Repeat("b", 64),
		}},
	}
	raw, err := operatorcheckpoint.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	absPath := filepath.Join(root, filepath.FromSlash(receiptPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return receiptPath, fmt.Sprintf("%x", sum)
}
