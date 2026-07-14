package gitstate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"revolvr/internal/runner"
)

func TestCaptureSourceEntryRejectsFinalSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeSourceTestFile(t, path, []byte("source bytes\n"))
	writeSourceTestFile(t, outside, []byte("outside bytes\n"))
	fired := false

	_, err := captureSourceEntryWithHook(root, "source.txt", "", func(point sourceEntryCapturePoint, _ string) error {
		if point != sourceEntryAfterInitialLstat || fired {
			return nil
		}
		fired = true
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Symlink(outside, path)
	})
	if err == nil || !fired {
		t.Fatalf("capture error/fired = %v/%t, want rejected final symlink replacement", err, fired)
	}
	raw, readErr := os.ReadFile(outside)
	if readErr != nil || string(raw) != "outside bytes\n" {
		t.Fatalf("outside bytes/error = %q/%v", raw, readErr)
	}
}

func TestCaptureSourceEntryRejectsRegularInodeReplacementAfterOpen(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	replacement := filepath.Join(root, "replacement.txt")
	writeSourceTestFile(t, path, []byte("original"))
	writeSourceTestFile(t, replacement, []byte("replaced"))
	fired := false

	_, err := captureSourceEntryWithHook(root, "source.txt", "", func(point sourceEntryCapturePoint, _ string) error {
		if point != sourceEntryAfterRegularOpen || fired {
			return nil
		}
		fired = true
		return os.Rename(replacement, path)
	})
	if err == nil || !strings.Contains(err.Error(), "regular file changed while it was being captured") || !fired {
		t.Fatalf("capture error/fired = %v/%t, want inode-replacement rejection", err, fired)
	}
}

func TestCaptureSourceEntryRejectsRegularABAOpenSubstitution(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	held := filepath.Join(root, "source-held.txt")
	replacement := filepath.Join(root, "replacement.txt")
	writeSourceTestFile(t, path, []byte("AAAA"))
	writeSourceTestFile(t, replacement, []byte("BBBB"))
	matchSourceMetadata(t, path, replacement)
	var replaced, restored bool

	_, err := captureSourceEntryWithHook(root, "source.txt", "", func(point sourceEntryCapturePoint, _ string) error {
		switch point {
		case sourceEntryAfterInitialLstat:
			if err := os.Rename(path, held); err != nil {
				return err
			}
			if err := os.Rename(replacement, path); err != nil {
				return err
			}
			replaced = true
		case sourceEntryAfterRegularOpen:
			if err := os.Rename(held, path); err != nil {
				return err
			}
			restored = true
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "regular file changed while it was being captured") || !replaced || !restored {
		t.Fatalf("capture error/replaced/restored = %v/%t/%t, want ABA rejection", err, replaced, restored)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil || string(raw) != "AAAA" {
		t.Fatalf("restored bytes/error = %q/%v", raw, readErr)
	}
}

func TestCaptureSourceEntryRejectsRegularMutationAfterRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.txt")
	writeSourceTestFile(t, path, []byte("original"))
	fired := false

	_, err := captureSourceEntryWithHook(root, "source.txt", "", func(point sourceEntryCapturePoint, _ string) error {
		if point != sourceEntryBeforeRegularDescriptorRecheck || fired {
			return nil
		}
		fired = true
		return os.WriteFile(path, []byte("modified after read"), 0o644)
	})
	if err == nil || !strings.Contains(err.Error(), "regular file changed while it was being captured") || !fired {
		t.Fatalf("capture error/fired = %v/%t, want descriptor mutation rejection", err, fired)
	}
}

func TestCaptureSourceEntryRejectsSymlinkABAOpenSubstitution(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source-link")
	held := filepath.Join(root, "source-held")
	replacement := filepath.Join(root, "replacement-link")
	if err := os.Symlink("target-a", path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-b", replacement); err != nil {
		t.Fatal(err)
	}
	matchSourceSymlinkMetadata(t, path, replacement)
	var replaced, restored bool

	_, err := captureSourceEntryWithHook(root, "source-link", "", func(point sourceEntryCapturePoint, _ string) error {
		switch point {
		case sourceEntryAfterInitialLstat:
			if err := os.Rename(path, held); err != nil {
				return err
			}
			if err := os.Rename(replacement, path); err != nil {
				return err
			}
			replaced = true
		case sourceEntryAfterSymlinkOpen:
			if err := os.Rename(held, path); err != nil {
				return err
			}
			restored = true
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "symlink changed while it was being captured") || !replaced || !restored {
		t.Fatalf("capture error/replaced/restored = %v/%t/%t, want symlink ABA rejection", err, replaced, restored)
	}
}

func TestCaptureSourceEntryReadsSymlinkThroughOpenedIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source-link")
	held := filepath.Join(root, "source-held")
	replacement := filepath.Join(root, "replacement-link")
	if err := os.Symlink("target-a", path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-b", replacement); err != nil {
		t.Fatal(err)
	}
	matchSourceSymlinkMetadata(t, path, replacement)
	var replaced, restored bool

	entry, err := captureSourceEntryWithHook(root, "source-link", "", func(point sourceEntryCapturePoint, _ string) error {
		switch point {
		case sourceEntryAfterSymlinkOpen:
			if err := os.Rename(path, held); err != nil {
				return err
			}
			if err := os.Rename(replacement, path); err != nil {
				return err
			}
			replaced = true
		case sourceEntryAfterSymlinkRead:
			if err := os.Rename(held, path); err != nil {
				return err
			}
			restored = true
		}
		return nil
	})
	if err != nil || !replaced || !restored {
		t.Fatalf("capture error/replaced/restored = %v/%t/%t", err, replaced, restored)
	}
	wantHash := sha256.Sum256([]byte("target-a"))
	if entry.FileType != "symlink" || entry.ByteSize != int64(len("target-a")) || entry.SHA256 != fmt.Sprintf("%x", wantHash) {
		t.Fatalf("entry = %+v, want opened target-a identity", entry)
	}
}

func TestCaptureSourceEntryRejectsSymlinkReplacementAfterRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source-link")
	replacement := filepath.Join(root, "replacement-link")
	if err := os.Symlink("target-a", path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-b", replacement); err != nil {
		t.Fatal(err)
	}
	matchSourceSymlinkMetadata(t, path, replacement)
	fired := false

	_, err := captureSourceEntryWithHook(root, "source-link", "", func(point sourceEntryCapturePoint, _ string) error {
		if point != sourceEntryAfterSymlinkRead || fired {
			return nil
		}
		fired = true
		return os.Rename(replacement, path)
	})
	if err == nil || !strings.Contains(err.Error(), "symlink changed while it was being captured") || !fired {
		t.Fatalf("capture error/fired = %v/%t, want post-read replacement rejection", err, fired)
	}
}

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
	cfg := SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true}
	before, err := CaptureSourceSnapshot(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".revolvr", "runs", "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".revolvr", "runs", "run-1", "artifact"), []byte("runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := CaptureSourceSnapshot(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if difference := CompareSourceSnapshots(before, after); difference.Changed {
		t.Fatalf("runtime-only difference = %+v", difference)
	}
}

func TestSourceSnapshotRejectsPolicyRelevantIgnoredState(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
		make     func(*testing.T, string)
		want     string
	}{
		{name: "regular file", path: "local.env", wantPath: "local.env", make: func(t *testing.T, path string) {
			writeSourceTestFile(t, path, []byte("do-not-log-this-secret\n"))
		}, want: "regular"},
		{name: "ignored directory", path: "cache/value", wantPath: "cache", make: func(t *testing.T, path string) {
			writeSourceTestFile(t, path, []byte("cache bytes\n"))
		}, want: "directory"},
		{name: "empty ignored directory", path: "empty-cache", wantPath: "empty-cache", make: func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}, want: "directory"},
		{name: "symlink", path: "secret-link.env", wantPath: "secret-link.env", make: func(t *testing.T, path string) {
			if err := os.Symlink("/outside/secret-target", path); err != nil {
				t.Fatal(err)
			}
		}, want: "symlink"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sourceTestRepository(t)
			writeSourceTestFile(t, filepath.Join(root, ".gitignore"), []byte(".revolvr/\n*.env\ncache/\nempty-cache/\n"))
			runGitTest(t, root, "add", ".gitignore")
			runGitTest(t, root, "commit", "-qm", "ignore policy inputs")
			tt.make(t, filepath.Join(root, filepath.FromSlash(tt.path)))

			_, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true})
			if err == nil || !strings.Contains(err.Error(), tt.wantPath) || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "classification=policy_relevant") {
				t.Fatalf("error = %v, want safe path/type/classification", err)
			}
			for _, forbidden := range []string{"do-not-log-this-secret", "/outside/secret-target", "cache bytes"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("error exposed ignored content or symlink target: %v", err)
				}
			}
		})
	}
}

func TestSourceSnapshotHarnessRuntimeAllowanceIsExplicit(t *testing.T) {
	root := sourceTestRepository(t)
	writeSourceTestFile(t, filepath.Join(root, ".revolvr", "runs", "run-1", "artifact"), []byte("runtime"))

	_, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root})
	if err == nil || !strings.Contains(err.Error(), `".revolvr"`) || !strings.Contains(err.Error(), "policy_relevant") {
		t.Fatalf("default error = %v, want policy-relevant runtime refusal", err)
	}
	first, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true})
	if err != nil {
		t.Fatal(err)
	}
	writeSourceTestFile(t, filepath.Join(root, ".revolvr", "runs", "run-2", "artifact"), []byte("more runtime"))
	second, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true})
	if err != nil {
		t.Fatal(err)
	}
	if difference := CompareSourceSnapshots(first, second); difference.Changed {
		t.Fatalf("allowlisted runtime changed source evidence: %+v", difference)
	}
}

func TestSourceSnapshotRejectsNestedAndHostileIgnoredPaths(t *testing.T) {
	root := sourceTestRepository(t)
	writeSourceTestFile(t, filepath.Join(root, "nested", ".gitignore"), []byte("generated/\n*.env\n"))
	runGitTest(t, root, "add", "nested/.gitignore")
	runGitTest(t, root, "commit", "-qm", "nested ignore policy")
	writeSourceTestFile(t, filepath.Join(root, "nested", "generated", "value"), []byte("nested secret bytes\n"))
	writeSourceTestFile(t, filepath.Join(root, "nested", "line\nbreak.env"), []byte("hostile secret bytes\n"))

	_, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true})
	if err == nil || !strings.Contains(err.Error(), "nested/generated") || !strings.Contains(err.Error(), `line\nbreak.env`) {
		t.Fatalf("error = %v, want nested and escaped hostile paths", err)
	}
	for _, forbidden := range []string{"nested secret bytes", "hostile secret bytes"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("error exposed ignored content: %v", err)
		}
	}
}

func TestSourceSnapshotIncludesTrackedAndUnignoredRevolvrPaths(t *testing.T) {
	root := sourceTestRepository(t)
	writeSourceTestFile(t, filepath.Join(root, ".gitignore"), nil)
	writeSourceTestFile(t, filepath.Join(root, ".revolvr", "tracked.conf"), []byte("tracked\n"))
	writeSourceTestFile(t, filepath.Join(root, ".revolvr", "untracked.conf"), []byte("untracked\n"))
	runGitTest(t, root, "add", ".gitignore", ".revolvr/tracked.conf")
	runGitTest(t, root, "commit", "-qm", "track revolvr input")

	snapshot, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		got = append(got, entry.Path)
	}
	for _, want := range []string{".revolvr/tracked.conf", ".revolvr/untracked.conf"} {
		if !containsSourceTestString(got, want) {
			t.Fatalf("snapshot paths = %v, missing %q", got, want)
		}
	}
}

func TestSourceSnapshotIgnoredInputCannotEnterCleanCheckoutEvidence(t *testing.T) {
	root := sourceTestRepository(t)
	writeSourceTestFile(t, filepath.Join(root, ".gitignore"), []byte(".revolvr/\n*.env\n"))
	runGitTest(t, root, "add", ".gitignore")
	runGitTest(t, root, "commit", "-qm", "ignore environment")
	writeSourceTestFile(t, filepath.Join(root, "tracked.txt"), []byte("verified only with local input\n"))
	writeSourceTestFile(t, filepath.Join(root, "test.env"), []byte("hidden input\n"))

	if _, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true}); err == nil {
		t.Fatal("snapshot accepted an ignored verification input")
	}
	if err := os.Remove(filepath.Join(root, "test.env")); err != nil {
		t.Fatal(err)
	}
	accepted, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: root, AllowHarnessRuntime: true})
	if err != nil {
		t.Fatal(err)
	}
	acceptedRevision, err := PolicySourceRevision(accepted)
	if err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "tracked.txt")
	runGitTest(t, root, "commit", "-qm", "reproducible change")
	clone := filepath.Join(t.TempDir(), "clone")
	runGitTest(t, filepath.Dir(clone), "clone", "-q", root, clone)
	replayed, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: clone})
	if err != nil {
		t.Fatal(err)
	}
	replayedRevision, err := PolicySourceRevision(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if replayedRevision != acceptedRevision {
		t.Fatalf("clean replay revision = %s, want %s\naccepted=%+v\nreplayed=%+v", replayedRevision, acceptedRevision, accepted.Entries, replayed.Entries)
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

func TestSourceSnapshotValidationReportsFirstInvalidHashDeterministically(t *testing.T) {
	snapshot, err := CaptureSourceSnapshot(context.Background(), SourceSnapshotConfig{WorkingDir: sourceTestRepository(t)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.IndexSHA256 = "invalid-index"
	snapshot.WorktreeSHA256 = "invalid-worktree"
	snapshot.SnapshotSHA256 = "invalid-snapshot"
	const want = "validate source snapshot: index SHA-256 is invalid"
	for i := 0; i < 100; i++ {
		if err := snapshot.Validate(); err == nil || err.Error() != want {
			t.Fatalf("Validate() error = %v, want %q (run %d)", err, want, i)
		}
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
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".revolvr/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "tracked.txt", ".gitignore")
	runGitTest(t, root, "commit", "-qm", "baseline")
	return root
}

func writeSourceTestFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func matchSourceMetadata(t *testing.T, source, target string) {
	t.Helper()
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func matchSourceSymlinkMetadata(t *testing.T, source, target string) {
	t.Helper()
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := unix.NsecToTimespec(info.ModTime().UnixNano())
	if err := unix.UtimesNanoAt(unix.AT_FDCWD, target, []unix.Timespec{timestamp, timestamp}, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		t.Fatal(err)
	}
}

func containsSourceTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func runGitTest(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
