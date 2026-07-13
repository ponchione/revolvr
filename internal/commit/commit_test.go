package commit

import (
	"context"
	"encoding/json"
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

	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/runner"
	"revolvr/internal/verification"
)

func TestRunCommitsChangedFilesAndRecordsSHA(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(ctx, filepath.Join(workDir, "ledger.sqlite"), func() time.Time { return now })
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer store.Close()
	run, err := store.CreateRun(ctx, ledger.RunSpec{
		ID:     "run-commit",
		TaskID: "task-commit",
		Task:   "add auto commit",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	var calls []runner.Command
	headCalls := 0
	fakeRunner := func(_ context.Context, command runner.Command) runner.Result {
		calls = append(calls, command)
		switch {
		case reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}):
			headCalls++
			if headCalls == 1 {
				return runner.Result{ExitCode: 0, Stdout: "parent123\n"}
			}
			return runner.Result{ExitCode: 0, Stdout: "abc123def456\n"}
		case reflect.DeepEqual(command.Args, []string{"--literal-pathspecs", "add", "--", "a.go", "b.go"}):
			return runner.Result{ExitCode: 0}
		case reflect.DeepEqual(command.Args, []string{"commit", "-m", "Add auto commit gate", "-m", "Run-ID: run-commit\nTask-ID: task-commit\nVerification: passed"}):
			return runner.Result{ExitCode: 0, Stdout: "[main abc123] Add auto commit gate\n"}
		default:
			t.Fatalf("unexpected git command: %#v", command.Args)
			return runner.Result{ExitCode: 2}
		}
	}

	result, err := Run(ctx, Config{
		WorkingDir:         workDir,
		RunID:              run.ID,
		TaskID:             "task-commit",
		TaskSummary:        "Add auto commit gate",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &gitstate.Capture{ChangedFiles: []string{"b.go", "a.go", "a.go"}},
		GitExecutable:      "git-test",
		Timeout:            2 * time.Second,
		StdoutCap:          123,
		StderrCap:          45,
		Ledger:             store,
		CommandRunner:      fakeRunner,
	})
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}

	if result.Status != StatusCommitted {
		t.Fatalf("status = %s, want committed", result.Status)
	}
	if result.CommitSHA != "abc123def456" {
		t.Fatalf("commit sha = %q, want abc123def456", result.CommitSHA)
	}
	if result.PreCommitSHA != "parent123" || result.PostCommitSHA != "abc123def456" || result.HEADLookupRetried {
		t.Fatalf("HEAD evidence = pre %q post %q retried=%v", result.PreCommitSHA, result.PostCommitSHA, result.HEADLookupRetried)
	}
	if got, want := result.ChangedFiles, []string{"a.go", "b.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed files = %#v, want %#v", got, want)
	}
	if len(calls) != 4 {
		t.Fatalf("git call count = %d, want 4", len(calls))
	}
	absWorkDir := mustAbs(t, workDir)
	for _, call := range calls {
		if call.Name != "git-test" {
			t.Fatalf("git executable = %q, want git-test", call.Name)
		}
		if call.Dir != absWorkDir {
			t.Fatalf("git dir = %q, want %q", call.Dir, absWorkDir)
		}
		if call.Timeout != 2*time.Second || call.StdoutLimit != 123 || call.StderrLimit != 45 {
			t.Fatalf("git limits = timeout %s stdout %d stderr %d", call.Timeout, call.StdoutLimit, call.StderrLimit)
		}
	}
	commitArgs := calls[2].Args
	if !strings.Contains(commitArgs[4], "Run-ID: run-commit") || !strings.Contains(commitArgs[4], "Task-ID: task-commit") {
		t.Fatalf("commit body = %q, want run id and task id", commitArgs[4])
	}

	stored, ok, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if !ok {
		t.Fatal("stored run not found")
	}
	if stored.CommitSHA != "abc123def456" {
		t.Fatalf("stored commit sha = %q, want abc123def456", stored.CommitSHA)
	}

	history, ok, err := store.GetRunWithEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("run history not found")
	}
	gotTypes := eventTypes(history.Events)
	wantTypes := []ledger.EventType{ledger.EventCommitStarted, ledger.EventCommitCreated}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("ledger event types = %#v, want %#v", gotTypes, wantTypes)
	}
	var created map[string]any
	if err := json.Unmarshal(history.Events[1].Payload, &created); err != nil {
		t.Fatalf("unmarshal commit_created payload: %v", err)
	}
	if created["commit_sha"] != "abc123def456" {
		t.Fatalf("commit_created payload = %#v", created)
	}
	if created["pre_commit_sha"] != "parent123" || created["head_lookup_retried"] != false {
		t.Fatalf("commit_created HEAD evidence = %#v", created)
	}
}

func TestRunRecoversCreatedCommitAfterTransientHEADLookupFailure(t *testing.T) {
	headCalls := 0
	result, err := Run(context.Background(), baseConfig(t, func(_ context.Context, command runner.Command) runner.Result {
		switch {
		case reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}):
			headCalls++
			switch headCalls {
			case 1:
				return runner.Result{ExitCode: 0, Stdout: "parent123\n"}
			case 2:
				return runner.Result{ExitCode: 2, Err: errors.New("temporary HEAD lookup failure")}
			default:
				return runner.Result{ExitCode: 0, Stdout: "created456\n"}
			}
		case commitTestGitSubcommand(command.Args) == "add":
			return runner.Result{ExitCode: 0}
		case len(command.Args) > 0 && command.Args[0] == "commit":
			return runner.Result{ExitCode: 0}
		default:
			t.Fatalf("unexpected git command: %#v", command.Args)
			return runner.Result{ExitCode: 2}
		}
	}, nil))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusCommitted || result.CommitSHA != "created456" {
		t.Fatalf("result = %+v, want reconciled created commit", result)
	}
	if !result.HEADLookupRetried || result.Message != "commit created after reconciling HEAD" {
		t.Fatalf("reconciliation evidence = retried=%v message=%q", result.HEADLookupRetried, result.Message)
	}
	if len(result.Commands) != 5 || result.Commands[3].Error != "temporary HEAD lookup failure" {
		t.Fatalf("commands = %+v, want failed lookup followed by successful retry", result.Commands)
	}
}

func TestRunReconcilesRealCommitAfterTransientHEADLookupFailure(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	runCommitTestGit(t, workDir, "init", "-q")
	runCommitTestGit(t, workDir, "config", "user.name", "Revolvr Test")
	runCommitTestGit(t, workDir, "config", "user.email", "revolvr-test@example.invalid")
	path := filepath.Join(workDir, "feature.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatalf("write initial feature: %v", err)
	}
	runCommitTestGit(t, workDir, "add", "feature.txt")
	runCommitTestGit(t, workDir, "commit", "-q", "-m", "Initial commit")
	preCommitSHA := strings.TrimSpace(runCommitTestGit(t, workDir, "rev-parse", "HEAD"))
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatalf("update feature: %v", err)
	}

	commitAttempted := false
	postLookupFailed := false
	commandRunner := func(ctx context.Context, command runner.Command) runner.Result {
		if len(command.Args) > 0 && command.Args[0] == "commit" {
			result := runner.Run(ctx, command)
			commitAttempted = true
			return result
		}
		if commitAttempted && !postLookupFailed && reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}) {
			postLookupFailed = true
			return runner.Result{ExitCode: 2, Err: errors.New("injected post-commit HEAD lookup failure")}
		}
		return runner.Run(ctx, command)
	}

	result, err := Run(ctx, Config{
		WorkingDir:         workDir,
		RunID:              "run-real-reconcile",
		TaskID:             "task-real-reconcile",
		TaskSummary:        "Reconcile real commit",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &gitstate.Capture{ChangedFiles: []string{"feature.txt"}},
		CommandRunner:      commandRunner,
	})
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusCommitted || !result.HEADLookupRetried || result.PreCommitSHA != preCommitSHA {
		t.Fatalf("result = %+v, want reconciled real commit after %s", result, preCommitSHA)
	}
	postCommitSHA := strings.TrimSpace(runCommitTestGit(t, workDir, "rev-parse", "HEAD"))
	if result.CommitSHA != postCommitSHA || result.PostCommitSHA != postCommitSHA || postCommitSHA == preCommitSHA {
		t.Fatalf("HEAD evidence = result %q post %q pre %q", result.CommitSHA, postCommitSHA, preCommitSHA)
	}
}

func TestRunStagesLiteralPathsAndCommitsExactTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows filesystems do not support every legal Git filename exercised here")
	}

	ctx := context.Background()
	workDir := t.TempDir()
	runCommitTestGit(t, workDir, "init", "-q")
	runCommitTestGit(t, workDir, "config", "user.name", "Revolvr Test")
	runCommitTestGit(t, workDir, "config", "user.email", "revolvr-test@example.invalid")

	deletedPath := "delete\nme.txt"
	renameOldPath := "rename\told.txt"
	for path, content := range map[string]string{
		"keep.txt":     "keep\n",
		deletedPath:    "delete\n",
		renameOldPath:  "rename\n",
		"modified.txt": "before\n",
	} {
		if err := os.WriteFile(filepath.Join(workDir, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write baseline path %q: %v", path, err)
		}
	}
	runCommitTestGit(t, workDir, "add", "-A")
	runCommitTestGit(t, workDir, "commit", "-q", "-m", "Initial commit")

	renameNewPath := ":renamed [*]?.txt"
	newPaths := []string{
		":name",
		":(glob)decoy*.txt",
		" leading and trailing ",
		"space name.txt",
		"tab\tname.txt",
		"line\nname.txt",
		"wild*.txt",
		"-leading.txt",
	}
	if err := os.Remove(filepath.Join(workDir, deletedPath)); err != nil {
		t.Fatalf("delete path %q: %v", deletedPath, err)
	}
	if err := os.Rename(filepath.Join(workDir, renameOldPath), filepath.Join(workDir, renameNewPath)); err != nil {
		t.Fatalf("rename %q to %q: %v", renameOldPath, renameNewPath, err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "modified.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatalf("modify tracked path: %v", err)
	}
	for _, path := range newPaths {
		if err := os.WriteFile(filepath.Join(workDir, path), []byte(path), 0o644); err != nil {
			t.Fatalf("write changed path %q: %v", path, err)
		}
	}

	capture, err := gitstate.CaptureChangedFiles(ctx, gitstate.Config{WorkingDir: workDir})
	if err != nil {
		t.Fatalf("capture changed files: %v", err)
	}
	expectedChanged := append([]string{deletedPath, renameOldPath, renameNewPath, "modified.txt"}, newPaths...)
	sort.Strings(expectedChanged)
	if !reflect.DeepEqual(capture.ChangedFiles, expectedChanged) {
		t.Fatalf("captured changed files = %#v, want %#v", capture.ChangedFiles, expectedChanged)
	}

	// These files appear after the capture and match pathspec-looking filenames in
	// expectedChanged. Literal staging must not pull either one into the index.
	decoys := []string{"decoy-unintended.txt", "wild-unintended.txt"}
	for _, path := range decoys {
		if err := os.WriteFile(filepath.Join(workDir, path), []byte("unstaged\n"), 0o644); err != nil {
			t.Fatalf("write decoy path %q: %v", path, err)
		}
	}

	var addArgs, stagedChanges, stagedTree []string
	commandRunner := func(ctx context.Context, command runner.Command) runner.Result {
		result := runner.Run(ctx, command)
		if commitTestGitSubcommand(command.Args) == "add" && result.Err == nil && !result.TimedOut && result.ExitCode == 0 {
			addArgs = append([]string(nil), command.Args...)
			stagedChanges = commitTestNULPaths(runCommitTestGit(t, workDir, "diff", "--cached", "--no-renames", "--name-only", "-z"))
			stagedTree = commitTestNULPaths(runCommitTestGit(t, workDir, "ls-files", "-z"))
		}
		return result
	}

	result, err := Run(ctx, Config{
		WorkingDir:         workDir,
		RunID:              "run-literal-paths",
		TaskID:             "task-literal-paths",
		TaskSummary:        "Stage literal paths",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &capture,
		CommandRunner:      commandRunner,
	})
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusCommitted {
		t.Fatalf("result = %+v, want committed", result)
	}

	wantAddArgs := append([]string{"--literal-pathspecs", "add", "--"}, expectedChanged...)
	if !reflect.DeepEqual(addArgs, wantAddArgs) {
		t.Fatalf("git add args = %#v, want %#v", addArgs, wantAddArgs)
	}
	if !reflect.DeepEqual(stagedChanges, expectedChanged) {
		t.Fatalf("staged changes = %#v, want %#v", stagedChanges, expectedChanged)
	}
	expectedTree := append([]string{"keep.txt", "modified.txt", renameNewPath}, newPaths...)
	sort.Strings(expectedTree)
	if !reflect.DeepEqual(stagedTree, expectedTree) {
		t.Fatalf("staged tree = %#v, want %#v", stagedTree, expectedTree)
	}
	committedTree := commitTestNULPaths(runCommitTestGit(t, workDir, "ls-tree", "-r", "--name-only", "-z", "HEAD"))
	if !reflect.DeepEqual(committedTree, expectedTree) {
		t.Fatalf("committed tree = %#v, want %#v", committedTree, expectedTree)
	}
	for _, path := range decoys {
		if strings.Contains(string(runCommitTestGit(t, workDir, "ls-files", "-z")), path+"\x00") {
			t.Fatalf("decoy path %q entered the index", path)
		}
	}
}

func TestRunRecordsCommitWhenCommandFailsButHEADAdvanced(t *testing.T) {
	headCalls := 0
	result, err := Run(context.Background(), baseConfig(t, func(_ context.Context, command runner.Command) runner.Result {
		switch {
		case reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}):
			headCalls++
			if headCalls == 1 {
				return runner.Result{ExitCode: 0, Stdout: "parent123\n"}
			}
			return runner.Result{ExitCode: 0, Stdout: "created456\n"}
		case commitTestGitSubcommand(command.Args) == "add":
			return runner.Result{ExitCode: 0}
		case len(command.Args) > 0 && command.Args[0] == "commit":
			return runner.Result{ExitCode: 1, Stderr: "hook reported failure after creating commit"}
		default:
			t.Fatalf("unexpected git command: %#v", command.Args)
			return runner.Result{ExitCode: 2}
		}
	}, nil))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusCommitted || result.CommitSHA != "created456" {
		t.Fatalf("result = %+v, want advanced HEAD recorded as committed", result)
	}
	if result.Message != "commit created despite git commit command failure" {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestRunReportsIndeterminateWhenPostCommitHEADCannotBeResolved(t *testing.T) {
	headCalls := 0
	result, err := Run(context.Background(), baseConfig(t, func(_ context.Context, command runner.Command) runner.Result {
		switch {
		case reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}):
			headCalls++
			if headCalls == 1 {
				return runner.Result{ExitCode: 0, Stdout: "parent123\n"}
			}
			return runner.Result{ExitCode: 2, Err: errors.New("HEAD unavailable")}
		case commitTestGitSubcommand(command.Args) == "add":
			return runner.Result{ExitCode: 0}
		case len(command.Args) > 0 && command.Args[0] == "commit":
			return runner.Result{ExitCode: 0}
		default:
			t.Fatalf("unexpected git command: %#v", command.Args)
			return runner.Result{ExitCode: 2}
		}
	}, nil))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusIndeterminate || result.CommitSHA != "" {
		t.Fatalf("result = %+v, want indeterminate without an invented SHA", result)
	}
	if result.PreCommitSHA != "parent123" || result.PostCommitSHA != "" || !result.HEADLookupRetried {
		t.Fatalf("HEAD evidence = pre %q post %q retried=%v", result.PreCommitSHA, result.PostCommitSHA, result.HEADLookupRetried)
	}
	if !strings.Contains(result.Message, "indeterminate") || len(result.Commands) != 5 {
		t.Fatalf("result evidence = %+v", result)
	}
}

func TestRunSupportsInitialCommitWithUnbornHEAD(t *testing.T) {
	headCalls := 0
	result, err := Run(context.Background(), baseConfig(t, func(_ context.Context, command runner.Command) runner.Result {
		switch {
		case reflect.DeepEqual(command.Args, []string{"rev-parse", "--verify", "--quiet", "HEAD"}):
			headCalls++
			if headCalls == 1 {
				return runner.Result{ExitCode: 1}
			}
			return runner.Result{ExitCode: 0, Stdout: "initial123\n"}
		case commitTestGitSubcommand(command.Args) == "add":
			return runner.Result{ExitCode: 0}
		case len(command.Args) > 0 && command.Args[0] == "commit":
			return runner.Result{ExitCode: 0}
		default:
			t.Fatalf("unexpected git command: %#v", command.Args)
			return runner.Result{ExitCode: 2}
		}
	}, nil))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusCommitted || result.PreCommitSHA != "" || result.CommitSHA != "initial123" {
		t.Fatalf("result = %+v, want successful initial commit", result)
	}
}

func TestRunRefusesWhenVerificationFails(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.VerificationResult = &verification.Result{
			Status:             verification.StatusFailed,
			Passed:             false,
			FailedCommandIndex: 0,
			Commands: []verification.CommandResult{
				{Index: 0, Command: "go test ./...", Status: verification.StatusFailed, Passed: false, ExitCode: 1},
			},
		}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonVerificationFailed {
		t.Fatalf("result = %+v, want verification refusal", result)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRefusesWhenNoChanges(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.PostRunChanged = &gitstate.Capture{}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonNoChanges {
		t.Fatalf("result = %+v, want no-changes refusal", result)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRefusesTruncatedGitStateBeforeAnyGitMutation(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.PostRunChanged = &gitstate.Capture{
			CaptureError:         "git status output was truncated (stdout=42 bytes, stderr=0 bytes)",
			Paths:                []string{"partial.go"},
			ChangedFiles:         []string{"partial.go"},
			StdoutTruncatedBytes: 42,
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonGitStateCaptureFailed {
		t.Fatalf("result = %+v, want Git-state refusal", result)
	}
	if calls != 0 || len(result.Commands) != 0 {
		t.Fatalf("truncated capture ran Git commands: calls=%d commands=%+v", calls, result.Commands)
	}
	if len(result.ChangedFiles) != 0 {
		t.Fatalf("refusal published partial changed paths: %#v", result.ChangedFiles)
	}
}

func TestRunRefusesRealLargeTruncatedStatusWithoutStaging(t *testing.T) {
	workDir := t.TempDir()
	runCommitTestGit(t, workDir, "init", "-q")
	runCommitTestGit(t, workDir, "config", "user.name", "Revolvr Test")
	runCommitTestGit(t, workDir, "config", "user.email", "revolvr-test@example.invalid")
	if err := os.WriteFile(filepath.Join(workDir, "baseline.txt"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommitTestGit(t, workDir, "add", "--", "baseline.txt")
	runCommitTestGit(t, workDir, "commit", "-q", "-m", "initial")
	headBefore := strings.TrimSpace(runCommitTestGit(t, workDir, "rev-parse", "HEAD"))
	if err := os.MkdirAll(filepath.Join(workDir, "bulk"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4000; i++ {
		name := fmt.Sprintf("%05d-%s.txt", i, strings.Repeat("x", 64))
		if err := os.WriteFile(filepath.Join(workDir, "bulk", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	capture, err := gitstate.CaptureChangedFiles(context.Background(), gitstate.Config{WorkingDir: workDir, StdoutCap: 256 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if capture.StdoutTruncatedBytes == 0 || !strings.Contains(capture.CaptureError, "truncated") {
		t.Fatalf("capture truncated=%d error=%q, want real truncation", capture.StdoutTruncatedBytes, capture.CaptureError)
	}

	gitCalls := 0
	result, err := Run(context.Background(), Config{
		WorkingDir:         workDir,
		RunID:              "run-large-status",
		TaskID:             "task-large-status",
		TaskSummary:        "Refuse incomplete changed set",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &capture,
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			gitCalls++
			return runner.Result{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonGitStateCaptureFailed || gitCalls != 0 {
		t.Fatalf("result=%+v git calls=%d, want refusal before Git", result, gitCalls)
	}
	if got := strings.TrimSpace(runCommitTestGit(t, workDir, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("HEAD advanced from %s to %s", headBefore, got)
	}
	if staged := strings.TrimSpace(runCommitTestGit(t, workDir, "diff", "--cached", "--name-only")); staged != "" {
		t.Fatalf("paths were staged after incomplete status: %q", staged)
	}
}

func TestRunRefusesWhenCodexFails(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.CodexResult = &codexexec.Result{ExitCode: 1, Err: errors.New("codex failed")}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonCodexFailed {
		t.Fatalf("result = %+v, want codex refusal", result)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRefusesPreExistingDirtyFilesByDefault(t *testing.T) {
	calls := 0
	result, err := Run(context.Background(), baseConfig(t, func(context.Context, runner.Command) runner.Result {
		calls++
		return runner.Result{ExitCode: 0}
	}, func(cfg *Config) {
		cfg.PreRunDirty = &gitstate.Capture{DirtyFiles: []string{"manual.go"}}
	}))
	if err != nil {
		t.Fatalf("run auto-commit: %v", err)
	}
	if result.Status != StatusRefused || result.RefusalReason != ReasonPreExistingDirty {
		t.Fatalf("result = %+v, want pre-existing dirty refusal", result)
	}
	if got, want := result.PreExistingDirtyFiles, []string{"manual.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-existing files = %#v, want %#v", got, want)
	}
	if calls != 0 {
		t.Fatalf("git calls = %d, want 0", calls)
	}
}

func TestRunRequiresInputs(t *testing.T) {
	_, err := Run(context.Background(), Config{
		WorkingDir: t.TempDir(),
		RunID:      "run-1",
		TaskID:     "task-1",
	})
	if err == nil || !strings.Contains(err.Error(), "task summary is required") {
		t.Fatalf("error = %v, want task summary requirement", err)
	}
}

func baseConfig(t *testing.T, fakeRunner CommandRunner, mutate func(*Config)) Config {
	t.Helper()
	cfg := Config{
		WorkingDir:         t.TempDir(),
		RunID:              "run-1",
		TaskID:             "task-1",
		TaskSummary:        "Do the task",
		CodexResult:        &codexexec.Result{ExitCode: 0},
		VerificationResult: passedVerification(),
		PreRunDirty:        &gitstate.Capture{},
		PostRunChanged:     &gitstate.Capture{ChangedFiles: []string{"changed.go"}},
		CommandRunner:      fakeRunner,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

func passedVerification() *verification.Result {
	return &verification.Result{
		Status:             verification.StatusPassed,
		Passed:             true,
		FailedCommandIndex: -1,
		Commands: []verification.CommandResult{
			{
				Index:    0,
				Command:  "go test ./...",
				Name:     "go",
				Args:     []string{"test", "./..."},
				Status:   verification.StatusPassed,
				Passed:   true,
				ExitCode: 0,
			},
		},
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve absolute path: %v", err)
	}
	return abs
}

func eventTypes(events []ledger.Event) []ledger.EventType {
	out := make([]ledger.EventType, 0, len(events))
	for _, event := range events {
		out = append(out, event.Type)
	}
	return out
}

func runCommitTestGit(t *testing.T, workDir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = workDir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func commitTestGitSubcommand(args []string) string {
	if len(args) > 1 && args[0] == "--literal-pathspecs" {
		return args[1]
	}
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func commitTestNULPaths(output string) []string {
	output = strings.TrimSuffix(output, "\x00")
	if output == "" {
		return nil
	}
	paths := strings.Split(output, "\x00")
	sort.Strings(paths)
	return paths
}
