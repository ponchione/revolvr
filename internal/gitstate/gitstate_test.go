package gitstate

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"revolvr/internal/runner"
)

func TestParseShortStatusParsesEntriesAndQuotedPaths(t *testing.T) {
	status := strings.Join([]string{
		" M internal/runner/runner.go",
		"M  go.mod",
		"A  new.go",
		"D  old.go",
		"R  old/name.go -> new/name.go",
		`R  "old dir/quoted\tname.go" -> "new dir/quoted\"name.go"`,
		`C  "source -> tricky.go" -> "copy path.go"`,
		`?? "untracked file.go"`,
		`?? " leading and trailing "`,
		" M internal/runner/runner.go",
		"",
	}, "\n")

	got := ParseShortStatus(status)
	want := []Entry{
		{Status: " M", Kind: KindModified, Path: "internal/runner/runner.go"},
		{Status: "M ", Kind: KindModified, Path: "go.mod"},
		{Status: "A ", Kind: KindAdded, Path: "new.go"},
		{Status: "D ", Kind: KindDeleted, Path: "old.go"},
		{Status: "R ", Kind: KindRenamed, OldPath: "old/name.go", Path: "new/name.go"},
		{Status: "R ", Kind: KindRenamed, OldPath: "old dir/quoted\tname.go", Path: "new dir/quoted\"name.go"},
		{Status: "C ", Kind: KindCopied, OldPath: "source -> tricky.go", Path: "copy path.go"},
		{Status: "??", Kind: KindUntracked, Path: "untracked file.go"},
		{Status: "??", Kind: KindUntracked, Path: " leading and trailing "},
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
		"internal/runner/runner.go",
		"new dir/quoted\"name.go",
		"new.go",
		"new/name.go",
		"old dir/quoted\tname.go",
		"old.go",
		"old/name.go",
		"source -> tricky.go",
		"untracked file.go",
	)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("paths = %#v, want %#v", gotPaths, wantPaths)
	}
}

func TestParseShortStatusSkipsBranchAndInvalidLines(t *testing.T) {
	got := ParseShortStatus("## main...origin/main\n M valid.go\nx\n")
	want := []Entry{{Status: " M", Kind: KindModified, Path: "valid.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %#v, want %#v", got, want)
	}
}

func TestCaptureDirtyWorktreeInvokesGitStatus(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	var gotCommand runner.Command
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		gotCommand = command
		return runner.Result{
			ExitCode:             0,
			Stdout:               " M z.go\n?? a.go\n",
			Stderr:               "status note\n",
			StdoutTruncatedBytes: 1,
			StderrTruncatedBytes: 2,
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
	if wantArgs := []string{"status", "--short", "--untracked-files=all"}; !reflect.DeepEqual(gotCommand.Args, wantArgs) {
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
	if capture.RawStatus != " M z.go\n?? a.go\n" {
		t.Fatalf("raw status = %q", capture.RawStatus)
	}
	if capture.CaptureError != "" {
		t.Fatalf("capture error = %q, want empty", capture.CaptureError)
	}
	if capture.Stderr != "status note\n" || capture.StdoutTruncatedBytes != 1 || capture.StderrTruncatedBytes != 2 {
		t.Fatalf("capture output metadata = %+v", capture)
	}
}

func TestCaptureChangedFilesRecordsGitFailures(t *testing.T) {
	ctx := context.Background()
	fakeRunner := func(_ context.Context, _ runner.Command) runner.Result {
		return runner.Result{
			ExitCode: 128,
			Stdout:   " M parsed.go\n",
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
	if got, want := capture.ChangedFiles, []string{"parsed.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed files = %#v, want %#v", got, want)
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
