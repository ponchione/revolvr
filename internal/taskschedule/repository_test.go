package taskschedule

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/operatorcheckpoint"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

func TestLoadProjectsCanonicalDependencySelection(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "010-dependent.md", `---
id: task-dependent
status: pending
priority: 1
depends_on: task-prerequisite
---
# Dependent
`)
	writeTask(t, root, "020-prerequisite.md", `---
id: task-prerequisite
status: pending
priority: 50
---
# Prerequisite
`)

	snapshot, err := Load(context.Background(), Config{
		RepositoryRoot:    root,
		SelectionWorkflow: taskscheduler.WorkflowMixedPassV1,
	})
	if err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	selected, found := snapshot.Result.SelectedForWorkflow(taskscheduler.WorkflowMixedPassV1)
	if !found || selected.TaskID != "task-prerequisite" {
		t.Fatalf("selected found=%t task=%+v, want prerequisite", found, selected)
	}
	dependent, found := snapshot.Task("task-dependent")
	if !found {
		t.Fatal("dependent canonical task not found")
	}
	readiness, found := snapshot.Readiness(dependent)
	if !found || readiness.Reason != taskscheduler.ReasonWaitingDependency || !reflect.DeepEqual(readiness.UnmetDependencyIDs, []string{"task-prerequisite"}) {
		t.Fatalf("dependent readiness found=%t value=%+v", found, readiness)
	}
}

func TestLoadRetainsInvalidGraphDiagnosticsForReadSurfaces(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "010-first.md", "---\nid: duplicate\nstatus: pending\n---\n# First\n")
	writeTask(t, root, "020-second.md", "---\nid: duplicate\nstatus: pending\n---\n# Second\n")

	snapshot, err := Load(context.Background(), Config{
		RepositoryRoot:    root,
		SelectionWorkflow: taskscheduler.WorkflowMixedPassV1,
	})
	if err != nil {
		t.Fatalf("load invalid schedule: %v", err)
	}
	if snapshot.Result.Valid() || snapshot.Result.SelectedNext != nil || len(snapshot.Result.InvalidGraph) != 1 {
		t.Fatalf("invalid result = %+v", snapshot.Result)
	}
	if got := snapshot.Result.InvalidGraph[0].Code; got != taskscheduler.DiagnosticDuplicateTaskID {
		t.Fatalf("diagnostic code = %q, want duplicate_task_id", got)
	}
	for _, task := range snapshot.Tasks {
		readiness, found := snapshot.Readiness(task)
		if !found || readiness.Reason != taskscheduler.ReasonInvalidGraph {
			t.Fatalf("task %s readiness found=%t value=%+v", task.SourcePath, found, readiness)
		}
	}
}

func TestLoadClassifiesAwaitingCheckpointAndNeverSelectsIt(t *testing.T) {
	root := t.TempDir()
	writeCheckpointTask(t, root, "manual-acceptance", taskfile.StatusPending, "")
	writeTask(t, root, "release.md", `---
id: release
status: pending
depends_on: manual-acceptance
---
# Release
`)

	snapshot, err := Load(context.Background(), Config{RepositoryRoot: root, SelectionWorkflow: taskscheduler.WorkflowMixedPassV1})
	if err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	checkpoint, found := snapshot.Task("manual-acceptance")
	if !found {
		t.Fatal("checkpoint not found")
	}
	readiness, found := snapshot.Readiness(checkpoint)
	if !found || readiness.State != taskscheduler.StateAwaitingOperator || readiness.Reason != taskscheduler.ReasonAwaitingOperator {
		t.Fatalf("checkpoint readiness found=%t value=%+v", found, readiness)
	}
	dependent, found := snapshot.Task("release")
	if !found {
		t.Fatal("dependent not found")
	}
	readiness, found = snapshot.Readiness(dependent)
	if !found || readiness.Reason != taskscheduler.ReasonWaitingDependency || len(readiness.DependencyIssues) != 1 || readiness.DependencyIssues[0].Reason != taskscheduler.ReasonAwaitingOperatorDependency {
		t.Fatalf("dependent readiness found=%t value=%+v", found, readiness)
	}
	if snapshot.Result.SelectedNext != nil || len(snapshot.Result.Ready) != 0 {
		t.Fatalf("awaiting checkpoint was executable: %+v", snapshot.Result)
	}
}

func TestLoadVerifiedCheckpointUnlocksDependent(t *testing.T) {
	root := t.TempDir()
	raw := writeCheckpointReceipt(t, root, validCheckpointReceipt("manual-acceptance"))
	writeCheckpointTask(t, root, "manual-acceptance", taskfile.StatusCompleted, checkpointHash(raw))
	writeTask(t, root, "release.md", `---
id: release
status: pending
depends_on: manual-acceptance
---
# Release
`)

	snapshot, err := Load(context.Background(), Config{RepositoryRoot: root, SelectionWorkflow: taskscheduler.WorkflowMixedPassV1})
	if err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	if !snapshot.Result.Valid() || snapshot.Result.SelectedNext == nil || snapshot.Result.SelectedNext.TaskID != "release" {
		t.Fatalf("fulfilled schedule = %+v", snapshot.Result)
	}
	checkpoint, _ := snapshot.Task("manual-acceptance")
	readiness, _ := snapshot.Readiness(checkpoint)
	if readiness.Reason != taskscheduler.ReasonCompleted {
		t.Fatalf("checkpoint readiness = %+v", readiness)
	}
	for _, ready := range snapshot.Result.Ready {
		if ready.TaskID == checkpoint.ID {
			t.Fatal("checkpoint entered executable ready set")
		}
	}
}

func TestLoadReevaluationUnlocksDependentAfterAtomicCheckpointFulfillment(t *testing.T) {
	root := t.TempDir()
	raw := writeCheckpointReceipt(t, root, validCheckpointReceipt("manual-acceptance"))
	writeCheckpointTask(t, root, "manual-acceptance", taskfile.StatusPending, "")
	writeTask(t, root, "release.md", "---\nid: release\nstatus: pending\ndepends_on: manual-acceptance\n---\n# Release\n")

	before, err := Load(context.Background(), Config{RepositoryRoot: root, SelectionWorkflow: taskscheduler.WorkflowMixedPassV1})
	if err != nil {
		t.Fatal(err)
	}
	if before.Result.SelectedNext != nil {
		t.Fatalf("dependent selected before fulfillment: %+v", before.Result.SelectedNext)
	}
	checkpoint, found := before.Task("manual-acceptance")
	if !found {
		t.Fatal("checkpoint not found")
	}
	if _, changed, err := taskfile.FulfillOperatorCheckpoint(root, checkpoint, checkpointHash(raw)); err != nil || !changed {
		t.Fatalf("fulfill checkpoint changed=%t err=%v", changed, err)
	}

	after, err := Load(context.Background(), Config{RepositoryRoot: root, SelectionWorkflow: taskscheduler.WorkflowMixedPassV1})
	if err != nil {
		t.Fatal(err)
	}
	if after.Result.SelectedNext == nil || after.Result.SelectedNext.TaskID != "release" {
		t.Fatalf("dependent not selected after scheduler re-evaluation: %+v", after.Result)
	}
}

func TestLoadInvalidCompletedCheckpointFailsGraphClosed(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string) string
		want    string
	}{
		{name: "missing receipt", prepare: func(t *testing.T, root string) string { return strings.Repeat("a", 64) }, want: os.ErrNotExist.Error()},
		{name: "malformed receipt", prepare: func(t *testing.T, root string) string {
			raw := []byte(`{"schema_version":`)
			writeCheckpointReceiptRaw(t, root, "manual-acceptance", raw)
			return checkpointHash(raw)
		}, want: "decode operator checkpoint receipt"},
		{name: "mismatched task", prepare: func(t *testing.T, root string) string {
			raw := writeCheckpointReceiptAt(t, root, "manual-acceptance", validCheckpointReceipt("other-checkpoint"))
			return checkpointHash(raw)
		}, want: "does not match checkpoint"},
		{name: "changed identity", prepare: func(t *testing.T, root string) string {
			original, err := operatorcheckpoint.Marshal(validCheckpointReceipt("manual-acceptance"))
			if err != nil {
				t.Fatal(err)
			}
			changed := validCheckpointReceipt("manual-acceptance")
			changed.Decision = "Accepted after a second review."
			writeCheckpointReceipt(t, root, changed)
			return checkpointHash(original)
		}, want: "does not match current receipt identity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			boundSHA := test.prepare(t, root)
			writeCheckpointTask(t, root, "manual-acceptance", taskfile.StatusCompleted, boundSHA)
			snapshot, err := Load(context.Background(), Config{RepositoryRoot: root, SelectionWorkflow: taskscheduler.WorkflowMixedPassV1})
			if err != nil {
				t.Fatalf("load schedule: %v", err)
			}
			if snapshot.Result.Valid() || snapshot.Result.SelectedNext != nil || len(snapshot.Result.InvalidGraph) != 1 || snapshot.Result.InvalidGraph[0].Code != taskscheduler.DiagnosticInvalidTask || !strings.Contains(snapshot.Result.InvalidGraph[0].Detail, test.want) {
				t.Fatalf("invalid schedule = %+v, want detail %q", snapshot.Result, test.want)
			}
		})
	}
}

func TestLoadActiveStrictMissingStateNamesMigrationRecoveryCommand(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "autonomous.md", "---\nid: autonomous\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/autonomous/state.json\n---\n# Autonomous\n")
	_, err := LoadActiveStrict(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "revolvr task migrate --to autonomous-v1 autonomous") {
		t.Fatalf("LoadActiveStrict error = %v", err)
	}
}

func writeTask(t *testing.T, root string, name string, content string) {
	t.Helper()
	dir := filepath.Join(root, taskfile.TasksDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create task directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write task %s: %v", name, err)
	}
}

func writeCheckpointTask(t *testing.T, root, taskID, status, receiptSHA string) {
	t.Helper()
	identity := ""
	if receiptSHA != "" {
		identity = "checkpoint_receipt_sha256: " + receiptSHA + "\n"
	}
	writeTask(t, root, taskID+".md", fmt.Sprintf("---\nid: %s\nstatus: %s\nworkflow: operator-checkpoint-v1\ncheckpoint_receipt_path: %s\n%s---\n# Manual acceptance\n", taskID, status, operatorcheckpoint.ExpectedReceiptPath(taskID), identity))
}

func writeCheckpointReceipt(t *testing.T, root string, receipt operatorcheckpoint.Receipt) []byte {
	t.Helper()
	return writeCheckpointReceiptAt(t, root, receipt.TaskID, receipt)
}

func writeCheckpointReceiptAt(t *testing.T, root, pathTaskID string, receipt operatorcheckpoint.Receipt) []byte {
	t.Helper()
	raw, err := operatorcheckpoint.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal checkpoint receipt: %v", err)
	}
	writeCheckpointReceiptRaw(t, root, pathTaskID, raw)
	return raw
}

func writeCheckpointReceiptRaw(t *testing.T, root, taskID string, raw []byte) {
	t.Helper()
	absPath := filepath.Join(root, filepath.FromSlash(operatorcheckpoint.ExpectedReceiptPath(taskID)))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("create checkpoint receipt directory: %v", err)
	}
	if err := os.WriteFile(absPath, raw, 0o644); err != nil {
		t.Fatalf("write checkpoint receipt: %v", err)
	}
}

func validCheckpointReceipt(taskID string) operatorcheckpoint.Receipt {
	return operatorcheckpoint.Receipt{
		SchemaVersion: operatorcheckpoint.ReceiptSchemaVersion,
		TaskID:        taskID,
		Outcome:       operatorcheckpoint.OutcomeAccepted,
		Operator:      "operator@example.test",
		Provenance:    "manual acceptance review",
		AcceptedAt:    time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC),
		Subject:       "Manual acceptance",
		Decision:      "Accepted.",
		Evidence: []operatorcheckpoint.EvidenceReference{{
			Kind: operatorcheckpoint.EvidenceArtifact, Path: "artifacts/manual-review.txt", SHA256: strings.Repeat("b", 64),
		}},
	}
}

func checkpointHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}
