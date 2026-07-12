package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomousarchive"
)

func TestArchiveCLIListShowAndHelp(t *testing.T) {
	root := t.TempDir()
	out, err := executeCLI(t, root, "archive", "list")
	if err != nil || out != "No archives.\n" {
		t.Fatalf("empty list out=%q err=%v", out, err)
	}
	writeCLIArchiveFixture(t, root)
	out, err = executeCLI(t, root, "archive", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ARCHIVE ID\tTASK ID", "archive-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\ttask-cli\tcancelled", ".agent/archive/2026/07/task-cli/archive.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q:\n%s", want, out)
		}
	}
	out, err = executeCLI(t, root, "archive", "show", "task-cli")
	if err != nil || !strings.Contains(out, "Archive ID: archive-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n") || !strings.Contains(out, "Completion: omitted\n") {
		t.Fatalf("show out=%q err=%v", out, err)
	}
	for _, args := range [][]string{{"archive", "--help"}, {"archive", "create", "--help"}, {"archive", "verify", "--help"}, {"archive", "reopen", "--help"}} {
		if _, err := executeCLI(t, root, args...); err != nil {
			t.Fatalf("help %v: %v", args, err)
		}
	}
}

func TestArchiveCLIRequiresExplicitUTCTime(t *testing.T) {
	root := t.TempDir()
	_, err := executeCLI(t, root, "archive", "create", "task-cli", "--operation-id", "op", "--disposition", "cancelled", "--reason", "cancelled", "--provenance", "operator:test", "--terminal-at", "2026-07-12T12:00:00-04:00", "--archived-at", "2026-07-12T16:00:00Z")
	if err == nil || !strings.Contains(err.Error(), "terminal-at must be UTC") {
		t.Fatalf("error = %v", err)
	}
}

func writeCLIArchiveFixture(t *testing.T, root string) {
	t.Helper()
	taskID := "task-cli"
	archivedAt := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	base := ".agent/archive/2026/07/task-cli"
	taskBytes := []byte("---\nid: task-cli\nstatus: cancelled\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/task-cli/state.json\n---\n# CLI task\n")
	taskHash := fmt.Sprintf("%x", sha256.Sum256(taskBytes))
	stateHash := fmt.Sprintf("%x", sha256.Sum256([]byte("state")))
	manifest := autonomousarchive.Manifest{SchemaVersion: autonomousarchive.ManifestSchemaVersion, ArchiveID: "archive-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OperationID: "archive-cli", ArchiveRunID: "archive-run-cli", TaskID: taskID, Disposition: autonomousarchive.DispositionCancelled, Reason: "operator cancelled", Provenance: "operator:test", TerminalAt: archivedAt.Add(-time.Minute), ArchivedAt: archivedAt, OriginalTask: autonomousarchive.Artifact{Path: ".agent/tasks/task-cli.md", SHA256: taskHash, ByteSize: len(taskBytes)}, ArchivedTask: autonomousarchive.Artifact{Path: base + "/task.md", SHA256: taskHash, ByteSize: len(taskBytes)}, Workflow: "autonomous-v1", State: autonomousarchive.Artifact{Path: ".revolvr/autonomous/tasks/task-cli/state.json", SHA256: stateHash, ByteSize: len("state")}, ExpectedPaths: []string{base + "/task.md", base + "/archive.json"}, Omissions: []string{"completion capsule omitted because disposition is not completed"}}
	raw, err := autonomousarchive.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, filepath.FromSlash(base))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.md"), taskBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "archive.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
