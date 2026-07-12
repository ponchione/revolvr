package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"revolvr/internal/autonomousarchive"
)

func TestArchiveListAndShowAppProjection(t *testing.T) {
	root := t.TempDir()
	writeAppArchiveFixture(t, root, "task-two", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "archive-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	writeAppArchiveFixture(t, root, "task-one", time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC), "archive-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	entries, err := ListArchives(context.Background(), Config{WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Manifest.TaskID != "task-one" || entries[1].Manifest.TaskID != "task-two" {
		t.Fatalf("entries = %+v", entries)
	}
	shown, err := ShowArchive(context.Background(), Config{WorkDir: root}, "task-two")
	if err != nil || shown.Manifest.ArchiveID != "archive-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("shown = %+v err=%v", shown, err)
	}
}

func writeAppArchiveFixture(t *testing.T, root, taskID string, archivedAt time.Time, archiveID string) {
	t.Helper()
	base := filepath.ToSlash(filepath.Join(autonomousarchive.ArchiveRoot, archivedAt.Format("2006"), archivedAt.Format("01"), taskID))
	taskBytes := []byte(fmt.Sprintf("---\nid: %s\nstatus: cancelled\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\n---\n# %s\n", taskID, taskID, taskID))
	taskHash := fmt.Sprintf("%x", sha256.Sum256(taskBytes))
	stateBytes := []byte("state")
	stateHash := fmt.Sprintf("%x", sha256.Sum256(stateBytes))
	manifest := autonomousarchive.Manifest{SchemaVersion: autonomousarchive.ManifestSchemaVersion, ArchiveID: archiveID, OperationID: "operation-" + taskID, ArchiveRunID: "run-" + taskID, TaskID: taskID, Disposition: autonomousarchive.DispositionCancelled, Reason: "operator cancelled", Provenance: "operator:test", TerminalAt: archivedAt.Add(-time.Minute), ArchivedAt: archivedAt, OriginalTask: autonomousarchive.Artifact{Path: ".agent/tasks/" + taskID + ".md", SHA256: taskHash, ByteSize: len(taskBytes)}, ArchivedTask: autonomousarchive.Artifact{Path: base + "/task.md", SHA256: taskHash, ByteSize: len(taskBytes)}, Workflow: "autonomous-v1", State: autonomousarchive.Artifact{Path: ".revolvr/autonomous/tasks/" + taskID + "/state.json", SHA256: stateHash, ByteSize: len(stateBytes)}, ExpectedPaths: []string{base + "/task.md", base + "/archive.json"}, Omissions: []string{"completion capsule omitted because disposition is not completed"}}
	raw, err := autonomousarchive.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, filepath.FromSlash(base))
	if err := os.MkdirAll(abs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(abs, "task.md"), taskBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(abs, "archive.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
