package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"revolvr/internal/runner"
)

func TestSourceSnapshotDetectsContentChangeOnAlreadyDirtyPath(t *testing.T) {
	root := sourceTestRepository(t)
	path := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(path, []byte("dirty before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("dirty after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	difference := CompareSourceSnapshots(before, after)
	if !difference.Changed || !difference.WorktreeChanged || len(difference.PathChanges) != 1 || difference.PathChanges[0].Path != "tracked.txt" {
		t.Fatalf("difference = %+v", difference)
	}
}

func TestSourceSnapshotExcludesRevolvrRuntimeArtifacts(t *testing.T) {
	root := sourceTestRepository(t)
	before, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".revolvr", "runs", "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".revolvr", "runs", "run-1", "artifact"), []byte("runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if difference := CompareSourceSnapshots(before, after); difference.Changed {
		t.Fatalf("runtime-only difference = %+v", difference)
	}
}

func TestSourceSnapshotRejectsTruncatedGitEvidence(t *testing.T) {
	_, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{
		WorkingDir: t.TempDir(),
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			return runner.Result{ExitCode: 0, Stdout: "abc", StdoutTruncatedBytes: 1}
		},
	})
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error = %v, want truncation", err)
	}
}

func TestPolicySourceRevisionTracksContentButSurvivesStageAndCommit(t *testing.T) {
	root := sourceTestRepository(t)
	baseline, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	baselineRevision, err := PolicySourceRevision(baseline)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "added.txt"), []byte("added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	changedRevision, err := PolicySourceRevision(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedRevision == baselineRevision {
		t.Fatal("content change did not change policy source revision")
	}

	runGitTest(t, root, "add", "tracked.txt", "added.txt")
	staged, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	stagedRevision, err := PolicySourceRevision(staged)
	if err != nil {
		t.Fatal(err)
	}
	if stagedRevision != changedRevision {
		t.Fatalf("staging changed policy source revision: got %s want %s", stagedRevision, changedRevision)
	}

	runGitTest(t, root, "commit", "-qm", "content change")
	committed, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	committedRevision, err := PolicySourceRevision(committed)
	if err != nil {
		t.Fatal(err)
	}
	if committedRevision != changedRevision {
		t.Fatalf("commit changed policy source revision: got %s want %s", committedRevision, changedRevision)
	}
}

func TestPolicySourceRevisionDeletionSurvivesStageAndCommit(t *testing.T) {
	root := sourceTestRepository(t)
	if err := os.Remove(filepath.Join(root, "tracked.txt")); err != nil {
		t.Fatal(err)
	}
	deleted, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	deletedRevision, err := PolicySourceRevision(deleted)
	if err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "--", "tracked.txt")
	runGitTest(t, root, "commit", "-qm", "delete tracked")
	committed, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	committedRevision, err := PolicySourceRevision(committed)
	if err != nil {
		t.Fatal(err)
	}
	if committedRevision != deletedRevision {
		t.Fatalf("committed deletion changed policy source revision: got %s want %s", committedRevision, deletedRevision)
	}
}

func sourceTestRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitTest(t, root, "init", "-q")
	runGitTest(t, root, "config", "user.email", "test@example.com")
	runGitTest(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "tracked.txt")
	runGitTest(t, root, "commit", "-qm", "baseline")
	return root
}

func runGitTest(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
