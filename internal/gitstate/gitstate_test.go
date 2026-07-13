package gitstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"revolvr/internal/runner"
)

func TestParsePorcelainV1ZPreservesArbitraryPathsAndRenameOrder(t *testing.T) {
	status := " M internal/runner/runner.go\x00" +
		"M  go.mod\x00" +
		"A  new.go\x00" +
		"D  old.go\x00" +
		"R  new/name.go\x00old/name.go\x00" +
		"R  new dir/quoted\"name.go\x00old dir/quoted\tname.go\x00" +
		"C  copy path.go\x00source -> tricky.go\x00" +
		"?? untracked\nfile.go\x00" +
		"??  leading and trailing \x00" +
		"!! ignored\nfile.go\x00" +
		" M internal/runner/runner.go\x00"

	got, err := ParsePorcelainV1Z(status)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Status: " M", Kind: KindModified, Path: "internal/runner/runner.go"},
		{Status: "M ", Kind: KindModified, Path: "go.mod"},
		{Status: "A ", Kind: KindAdded, Path: "new.go"},
		{Status: "D ", Kind: KindDeleted, Path: "old.go"},
		{Status: "R ", Kind: KindRenamed, OldPath: "old/name.go", Path: "new/name.go"},
		{Status: "R ", Kind: KindRenamed, OldPath: "old dir/quoted\tname.go", Path: "new dir/quoted\"name.go"},
		{Status: "C ", Kind: KindCopied, OldPath: "source -> tricky.go", Path: "copy path.go"},
		{Status: "??", Kind: KindUntracked, Path: "untracked\nfile.go"},
		{Status: "??", Kind: KindUntracked, Path: " leading and trailing "},
		{Status: "!!", Kind: KindIgnored, Path: "ignored\nfile.go"},
		{Status: " M", Kind: KindModified, Path: "internal/runner/runner.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %#v, want %#v", got, want)
	}

	gotPaths := PathsFromEntries(got)
	wantPaths := sortedStrings(
		" leading and trailing ",
		"copy path.go",
		"go.mod",
		"ignored\nfile.go",
		"internal/runner/runner.go",
		"new dir/quoted\"name.go",
		"new.go",
		"new/name.go",
		"old dir/quoted\tname.go",
		"old.go",
		"old/name.go",
		"source -> tricky.go",
		"untracked\nfile.go",
	)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("paths = %#v, want %#v", gotPaths, wantPaths)
	}
}

func TestParsePorcelainV1ZRejectsMalformedOrIncompleteRecords(t *testing.T) {
	tests := map[string]string{
		"unterminated":        " M valid.go",
		"short":               "M\x00",
		"separator":           " Mxvalid.go\x00",
		"empty path":          " M \x00",
		"invalid status":      "X  valid.go\x00",
		"mixed special":       "?  valid.go\x00",
		"missing rename from": "R  new.go\x00",
		"empty rename from":   "R  new.go\x00\x00",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if entries, err := ParsePorcelainV1Z(raw); err == nil {
				t.Fatalf("entries = %#v, want malformed-status error", entries)
			}
		})
	}
}

func TestCaptureDirtyWorktreeInvokesGitStatus(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	var gotCommand runner.Command
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		gotCommand = command
		return runner.Result{
			ExitCode: 0,
			Stdout:   " M z.go\x00?? a.go\x00",
			Stderr:   "status note\n",
		}
	}

	capture, err := CaptureDirtyWorktree(ctx, Config{
		WorkingDir:    workDir,
		GitExecutable: "git-test",
		Timeout:       2 * time.Second,
		StdoutCap:     123,
		StderrCap:     45,
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("capture dirty worktree: %v", err)
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		t.Fatalf("resolve work dir: %v", err)
	}
	if gotCommand.Name != "git-test" {
		t.Fatalf("command name = %q, want git-test", gotCommand.Name)
	}
	if gotCommand.Dir != absWorkDir {
		t.Fatalf("command dir = %q, want %q", gotCommand.Dir, absWorkDir)
	}
	if wantArgs := []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}; !reflect.DeepEqual(gotCommand.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotCommand.Args, wantArgs)
	}
	if gotCommand.Timeout != 2*time.Second || gotCommand.StdoutLimit != 123 || gotCommand.StderrLimit != 45 {
		t.Fatalf("command limits = timeout %s stdout %d stderr %d", gotCommand.Timeout, gotCommand.StdoutLimit, gotCommand.StderrLimit)
	}

	if capture.Kind != CaptureKindDirty {
		t.Fatalf("capture kind = %q, want dirty", capture.Kind)
	}
	if got, want := capture.DirtyFiles, []string{"a.go", "z.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dirty files = %#v, want %#v", got, want)
	}
	if len(capture.ChangedFiles) != 0 {
		t.Fatalf("changed files = %#v, want empty", capture.ChangedFiles)
	}
	if got, want := capture.Paths, []string{"a.go", "z.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if capture.RawStatus != " M z.go\x00?? a.go\x00" {
		t.Fatalf("raw status = %q", capture.RawStatus)
	}
	if capture.CaptureError != "" {
		t.Fatalf("capture error = %q, want empty", capture.CaptureError)
	}
	if capture.Stderr != "status note\n" || capture.StdoutTruncatedBytes != 0 || capture.StderrTruncatedBytes != 0 {
		t.Fatalf("capture output metadata = %+v", capture)
	}
}

func TestCaptureStatusRejectsTruncationWithoutPublishingPartialPaths(t *testing.T) {
	for _, test := range []struct {
		name            string
		stdoutTruncated int64
		stderrTruncated int64
	}{
		{name: "stdout", stdoutTruncated: 9},
		{name: "stderr", stderrTruncated: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture, err := CaptureChangedFiles(context.Background(), Config{
				WorkingDir: t.TempDir(),
				CommandRunner: func(context.Context, runner.Command) runner.Result {
					return runner.Result{
						ExitCode:             0,
						Stdout:               " M partial.go\x00",
						StdoutTruncatedBytes: test.stdoutTruncated,
						StderrTruncatedBytes: test.stderrTruncated,
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(capture.CaptureError, "output was truncated") {
				t.Fatalf("capture error = %q", capture.CaptureError)
			}
			if len(capture.Paths) != 0 || len(capture.Entries) != 0 || len(capture.ChangedFiles) != 0 {
				t.Fatalf("truncated capture published partial paths: %+v", capture)
			}
			if capture.RawStatus != " M partial.go\x00" {
				t.Fatalf("bounded diagnostic preview = %q", capture.RawStatus)
			}
		})
	}
}

func TestCaptureChangedFilesRecordsGitFailures(t *testing.T) {
	ctx := context.Background()
	fakeRunner := func(_ context.Context, _ runner.Command) runner.Result {
		return runner.Result{
			ExitCode: 128,
			Stdout:   " M parsed.go\x00",
			Stderr:   "fatal: not a git repository\n",
		}
	}

	capture, err := CaptureChangedFiles(ctx, Config{
		WorkingDir:    t.TempDir(),
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("capture changed files: %v", err)
	}
	if capture.Kind != CaptureKindChanged {
		t.Fatalf("capture kind = %q, want changed", capture.Kind)
	}
	if len(capture.ChangedFiles) != 0 || len(capture.Paths) != 0 || len(capture.Entries) != 0 {
		t.Fatalf("failed command published partial paths: %+v", capture)
	}
	if !strings.Contains(capture.CaptureError, "128") {
		t.Fatalf("capture error = %q, want exit code", capture.CaptureError)
	}
	if capture.Stderr != "fatal: not a git repository\n" {
		t.Fatalf("stderr = %q", capture.Stderr)
	}
}

func TestCaptureChangedFilesRecordsRunnerErrors(t *testing.T) {
	runErr := errors.New("start command: git not found")
	fakeRunner := func(_ context.Context, _ runner.Command) runner.Result {
		return runner.Result{ExitCode: -1, Err: runErr}
	}

	capture, err := CaptureChangedFiles(context.Background(), Config{
		WorkingDir:    t.TempDir(),
		CommandRunner: fakeRunner,
	})
	if err != nil {
		t.Fatalf("capture changed files: %v", err)
	}
	if !strings.Contains(capture.CaptureError, runErr.Error()) {
		t.Fatalf("capture error = %q, want runner error", capture.CaptureError)
	}
}

func TestCaptureChangedFilesRecordsMalformedPorcelainWithoutPaths(t *testing.T) {
	capture, err := CaptureChangedFiles(context.Background(), Config{
		WorkingDir: t.TempDir(),
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			return runner.Result{ExitCode: 0, Stdout: " M unterminated.go"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if capture.CaptureError == "" || !strings.Contains(capture.CaptureError, "unterminated") {
		t.Fatalf("capture error = %q", capture.CaptureError)
	}
	if len(capture.Paths) != 0 || len(capture.Entries) != 0 || len(capture.ChangedFiles) != 0 {
		t.Fatalf("malformed capture published paths: %+v", capture)
	}
}

func TestCaptureChangedFilesRealGitPreservesRenameDeleteUntrackedAndHostileNames(t *testing.T) {
	root := initGitStateRepository(t)
	modified := "modified.txt"
	deleted := "deleted\nname.txt"
	oldName := "old\nname.txt"
	newName := "new\nname.txt"
	untracked := "untracked\nname.txt"
	tracked := []string{modified, deleted, oldName}
	for _, path := range tracked {
		if err := os.WriteFile(filepath.Join(root, path), []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitStateTest(t, root, append([]string{"add", "--"}, tracked...)...)
	runGitStateTest(t, root, "commit", "-q", "-m", "initial")
	if err := os.WriteFile(filepath.Join(root, modified), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, deleted)); err != nil {
		t.Fatal(err)
	}
	runGitStateTest(t, root, "mv", "--", oldName, newName)
	if err := os.WriteFile(filepath.Join(root, untracked), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{modified, deleted, oldName, newName, untracked}
	invalidUTF8 := ""
	if runtime.GOOS != "windows" {
		invalidUTF8 = string([]byte("non-utf8-\xff.txt"))
		if err := os.WriteFile(filepath.Join(root, invalidUTF8), []byte("raw name\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		wantPaths = append(wantPaths, invalidUTF8)
	}
	sort.Strings(wantPaths)

	capture, err := CaptureChangedFiles(context.Background(), Config{WorkingDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if capture.CaptureError != "" {
		t.Fatalf("capture error = %q", capture.CaptureError)
	}
	if !reflect.DeepEqual(capture.ChangedFiles, wantPaths) {
		t.Fatalf("changed paths = %#v, want %#v", capture.ChangedFiles, wantPaths)
	}
	entries := make(map[string]Entry, len(capture.Entries))
	for _, entry := range capture.Entries {
		entries[entry.Path] = entry
	}
	if entries[modified].Kind != KindModified || entries[deleted].Kind != KindDeleted {
		t.Fatalf("modified/deleted entries = %+v / %+v", entries[modified], entries[deleted])
	}
	if got := entries[newName]; got.Kind != KindRenamed || got.OldPath != oldName {
		t.Fatalf("rename entry = %+v", got)
	}
	if entries[untracked].Kind != KindUntracked {
		t.Fatalf("untracked entry = %+v", entries[untracked])
	}
	if invalidUTF8 != "" && entries[invalidUTF8].Kind != KindUntracked {
		t.Fatalf("non-UTF-8 entry = %+v", entries[invalidUTF8])
	}
}

func TestCaptureChangedFilesLargeRepositoryIsCompleteOrFailsWithoutPartialPaths(t *testing.T) {
	root := initGitStateRepository(t)
	const pathCount = 4000
	if err := os.MkdirAll(filepath.Join(root, "bulk"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantPaths := make([]string, 0, pathCount)
	for i := 0; i < pathCount; i++ {
		name := fmt.Sprintf("%05d-%s.txt", i, strings.Repeat("x", 64))
		if err := os.WriteFile(filepath.Join(root, "bulk", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		wantPaths = append(wantPaths, filepath.ToSlash(filepath.Join("bulk", name)))
	}

	bounded, err := CaptureChangedFiles(context.Background(), Config{WorkingDir: root, StdoutCap: 256 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.StdoutTruncatedBytes == 0 || !strings.Contains(bounded.CaptureError, "truncated") {
		t.Fatalf("bounded capture truncated=%d error=%q", bounded.StdoutTruncatedBytes, bounded.CaptureError)
	}
	if len(bounded.Paths) != 0 || len(bounded.Entries) != 0 || len(bounded.ChangedFiles) != 0 {
		t.Fatalf("bounded capture published a partial set: paths=%d entries=%d", len(bounded.Paths), len(bounded.Entries))
	}

	complete, err := CaptureChangedFiles(context.Background(), Config{WorkingDir: root, StdoutCap: 2 * 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if complete.CaptureError != "" || complete.StdoutTruncatedBytes != 0 {
		t.Fatalf("complete capture error = %q truncated=%d", complete.CaptureError, complete.StdoutTruncatedBytes)
	}
	if len(complete.RawStatus) <= 256*1024 {
		t.Fatalf("complete raw status = %d bytes, want beyond former cap", len(complete.RawStatus))
	}
	if !reflect.DeepEqual(complete.ChangedFiles, wantPaths) {
		t.Fatalf("complete changed paths = %d entries, want exact deterministic %d-entry set", len(complete.ChangedFiles), len(wantPaths))
	}
	repeated, err := CaptureChangedFiles(context.Background(), Config{WorkingDir: root, StdoutCap: 2 * 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.CaptureError != "" || !reflect.DeepEqual(repeated.ChangedFiles, complete.ChangedFiles) {
		t.Fatalf("repeated capture error=%q paths_equal=%t", repeated.CaptureError, reflect.DeepEqual(repeated.ChangedFiles, complete.ChangedFiles))
	}
}

func TestCaptureRequiresWorkingDirectory(t *testing.T) {
	_, err := CaptureDirtyWorktree(context.Background(), Config{})
	if err == nil || !strings.Contains(err.Error(), "working directory is required") {
		t.Fatalf("error = %v, want working directory requirement", err)
	}
}

func sortedStrings(values ...string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func initGitStateRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitStateTest(t, root, "init", "-q")
	runGitStateTest(t, root, "config", "user.name", "Revolvr Test")
	runGitStateTest(t, root, "config", "user.email", "revolvr@example.invalid")
	return root
}

func runGitStateTest(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, output)
	}
	return string(output)
}
