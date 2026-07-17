package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousworkspace"
	"revolvr/internal/codexexec"
	commitpkg "revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/runner"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func TestExternalGitContainmentMatrix(t *testing.T) {
	t.Run("dirty worktree refuses before publication", func(t *testing.T) {
		fixture := newExternalGitContainmentFixture(t, "sha1", false)
		writeExternalGitFile(t, filepath.Join(fixture.root, "source.txt"), "operator dirty bytes\n", 0o644)
		assertExternalGitAdmissionRefused(t, fixture, "worktree clean")
	})

	t.Run("staged index refuses before publication", func(t *testing.T) {
		fixture := newExternalGitContainmentFixture(t, "sha1", false)
		writeExternalGitFile(t, filepath.Join(fixture.root, "source.txt"), "operator staged bytes\n", 0o644)
		runSchedulingGit(t, fixture.root, "add", "--", "source.txt")
		assertExternalGitAdmissionRefused(t, fixture, "worktree clean")
	})

	t.Run("ignored source refuses before workspace publication", func(t *testing.T) {
		fixture := newExternalGitContainmentFixture(t, "sha1", false)
		writeExternalGitFile(t, filepath.Join(fixture.root, "ignored-cache", "value.txt"), "ignored operator bytes\n", 0o640)

		effective, err := admitExternalMode(context.Background(), fixture.root, PreflightModeAttendedTask, 1, nil, false)
		if err != nil {
			t.Fatalf("clean Git admission with ignored source: %v", err)
		}
		before := captureExternalGitContainment(t, fixture.root)
		beforeRefs := externalGitRefs(t, fixture.root)
		unrelatedBefore := captureExternalGitContainment(t, fixture.unrelated)
		outsideBefore := snapshotExternalTree(t, fixture.outside)

		_, _, err = prepareTaskWorkspace(context.Background(), fixture.root, fixture.taskID, "ignored-operation", effective, fixedExternalGitClock)
		if !errors.Is(err, gitstate.ErrPolicyRelevantIgnored) {
			t.Fatalf("prepare task workspace error = %v, want policy-relevant ignored refusal", err)
		}
		assertExternalGitContainmentEqual(t, "control worktree", captureExternalGitContainment(t, fixture.root), before)
		assertExternalGitRefsEqual(t, externalGitRefs(t, fixture.root), beforeRefs)
		assertExternalGitContainmentEqual(t, "unrelated worktree", captureExternalGitContainment(t, fixture.unrelated), unrelatedBefore)
		if after := snapshotExternalTree(t, fixture.outside); !reflect.DeepEqual(after, outsideBefore) {
			t.Fatalf("outside sentinel changed\nbefore=%v\nafter=%v", outsideBefore, after)
		}
		if _, err := os.Lstat(productionHappyWorkspaceRoot(t, fixture.root, fixture.taskID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ignored source published a task workspace: %v", err)
		}
	})

	t.Run("active submodule refuses before publication", func(t *testing.T) {
		fixture := newExternalGitContainmentFixture(t, "sha1", false)
		child := t.TempDir()
		initializeExternalGitRepository(t, child, "sha1")
		writeExternalGitFile(t, filepath.Join(child, "child.txt"), "child source\n", 0o644)
		runSchedulingGit(t, child, "add", "child.txt")
		runSchedulingGit(t, child, "commit", "-q", "-m", "Child baseline")
		runSchedulingGit(t, fixture.root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", child, "vendor/child")
		runSchedulingGit(t, fixture.root, "commit", "-q", "-am", "Add active submodule")
		assertExternalGitAdmissionRefused(t, fixture, "active submodules")
	})

	for _, test := range []struct {
		name             string
		objectFormat     string
		linkedControl    bool
		externalCommit   bool
		wantObjectIDSize int
	}{
		{name: "SHA-1 task branch", objectFormat: "sha1", wantObjectIDSize: 40},
		{name: "SHA-256 task branch", objectFormat: "sha256", wantObjectIDSize: 64},
		{name: "linked control worktree", objectFormat: "sha1", linkedControl: true, wantObjectIDSize: 40},
		{name: "concurrent external commit", objectFormat: "sha1", externalCommit: true, wantObjectIDSize: 40},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newExternalGitContainmentFixture(t, test.objectFormat, test.linkedControl)
			assertExternalGitPublishedDelta(t, fixture, test.wantObjectIDSize, test.externalCommit)
		})
	}
}

type externalGitContainmentFixture struct {
	root      string
	primary   string
	unrelated string
	outside   string
	taskID    string
	baseline  string
}

func newExternalGitContainmentFixture(t *testing.T, objectFormat string, linkedControl bool) externalGitContainmentFixture {
	t.Helper()
	const taskID = "external-git-containment"
	primary := t.TempDir()
	initializeExternalGitRepository(t, primary, objectFormat)
	writeExternalGitFile(t, filepath.Join(primary, ".git", "info", "exclude"), "/.revolvr/\n", 0o644)
	writeExternalGitFile(t, filepath.Join(primary, ".gitignore"), "ignored-cache/\n", 0o644)
	writeExternalGitFile(t, filepath.Join(primary, "source.txt"), "baseline source\n", 0o644)
	createExternalGitSentinelTree(t, filepath.Join(primary, "containment-sentinel"))

	task, err := taskfile.ProjectAutonomousTask(primary, taskfile.AutonomousCreateInput{
		ID: taskID, Title: "External Git containment", Body: "Exercise the exact external Git containment boundary.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taskfile.PublishAutonomousTask(primary, task); err != nil {
		t.Fatal(err)
	}
	runSchedulingGit(t, primary, "add", ".agent", ".gitignore", "source.txt", "containment-sentinel")
	runSchedulingGit(t, primary, "commit", "-q", "-m", "External Git containment baseline")
	baseline := runSchedulingGit(t, primary, "rev-parse", "HEAD")

	root := primary
	if linkedControl {
		root = filepath.Join(t.TempDir(), "linked-control")
		runSchedulingGit(t, primary, "worktree", "add", "-q", "-b", "ext11-linked-control", root, baseline)
		hardenExternalGitAgentTree(t, filepath.Join(root, ".agent"))
	}
	writeExternalGitFile(t, filepath.Join(root, ".revolvr", "config.yaml"), "verification:\n  commands: [{name: go}]\n", 0o644)
	writeExternalGitState(t, root, taskID)
	createAppPreflightState(t, root)

	unrelated := filepath.Join(t.TempDir(), "unrelated-worktree")
	runSchedulingGit(t, primary, "worktree", "add", "-q", "-b", "ext11-unrelated", unrelated, baseline)
	createExternalGitSentinelTree(t, filepath.Join(unrelated, "unrelated-sentinel"))

	outside := filepath.Join(t.TempDir(), "outside-sentinel")
	createExternalGitSentinelTree(t, outside)
	return externalGitContainmentFixture{root: root, primary: primary, unrelated: unrelated, outside: outside, taskID: taskID, baseline: baseline}
}

func initializeExternalGitRepository(t *testing.T, root, objectFormat string) {
	t.Helper()
	command := exec.Command("git", "init", "-q", "--object-format="+objectFormat)
	command.Dir = root
	if raw, err := command.CombinedOutput(); err != nil {
		if objectFormat == "sha256" {
			t.Skipf("installed Git does not support SHA-256 repositories: %v: %s", err, strings.TrimSpace(string(raw)))
		}
		t.Fatalf("git init --object-format=%s: %v: %s", objectFormat, err, raw)
	}
	runSchedulingGit(t, root, "config", "user.name", "Revolvr Test")
	runSchedulingGit(t, root, "config", "user.email", "revolvr@example.invalid")
}

func writeExternalGitState(t *testing.T, root, taskID string) {
	t.Helper()
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        taskID,
		Lifecycle:     autonomous.LifecycleStatePending,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	writeExternalGitFile(t, filepath.Join(root, ".revolvr", "autonomous", "tasks", taskID, "state.json"), string(raw), 0o644)
}

func hardenExternalGitAgentTree(t *testing.T, root string) {
	t.Helper()
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		mode := os.FileMode(0o644)
		if info.IsDir() {
			mode = 0o755
		}
		return os.Chmod(path, mode)
	}); err != nil {
		t.Fatalf("harden linked-worktree agent tree: %v", err)
	}
}

func createExternalGitSentinelTree(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o750); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(root, "regular.txt")
	writeExternalGitFile(t, regular, "sentinel regular bytes\n", 0o640)
	if err := os.Link(regular, filepath.Join(root, "hardlink.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("regular.txt", filepath.Join(root, "symlink.txt")); err != nil {
		t.Fatal(err)
	}
	writeExternalGitFile(t, filepath.Join(root, "executable.sh"), "#!/bin/sh\nexit 0\n", 0o750)
}

func writeExternalGitFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

type externalGitContainmentSnapshot struct {
	Head      string
	Branch    string
	BranchOID string
	Status    string
	Index     string
	Sentinel  map[string]string
}

func captureExternalGitContainment(t *testing.T, root string) externalGitContainmentSnapshot {
	t.Helper()
	branch := runSchedulingGit(t, root, "symbolic-ref", "HEAD")
	indexPath := runSchedulingGit(t, root, "rev-parse", "--git-path", "index")
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(root, indexPath)
	}
	sentinel := filepath.Join(root, "containment-sentinel")
	if _, err := os.Lstat(filepath.Join(root, "unrelated-sentinel")); err == nil {
		sentinel = filepath.Join(root, "unrelated-sentinel")
	}
	return externalGitContainmentSnapshot{
		Head:      runSchedulingGit(t, root, "rev-parse", "HEAD"),
		Branch:    branch,
		BranchOID: runSchedulingGit(t, root, "rev-parse", branch),
		Status:    runExternalGitRaw(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"),
		Index:     externalGitIndexAuthority(t, indexPath),
		Sentinel:  snapshotExternalTree(t, sentinel),
	}
}

func externalGitIndexAuthority(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("index stat identity unavailable for %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("type=%s perm=%04o size=%d links=%d sha256=%x", info.Mode().Type(), info.Mode().Perm(), info.Size(), uint64(stat.Nlink), sha256.Sum256(raw))
}

func assertExternalGitContainmentEqual(t *testing.T, name string, got, want externalGitContainmentSnapshot) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s authority changed\ngot=%+v\nwant=%+v", name, got, want)
	}
}

func externalGitRefs(t *testing.T, root string) map[string]string {
	t.Helper()
	raw := runExternalGitRaw(t, root, "for-each-ref", "--sort=refname", "--format=%(refname) %(objectname)", "refs/heads")
	refs := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("parse Git ref record %q", line)
		}
		refs[fields[0]] = fields[1]
	}
	return refs
}

func assertExternalGitRefsEqual(t *testing.T, got, want map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Git refs changed\ngot=%v\nwant=%v", got, want)
	}
}

func runExternalGitRaw(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	raw, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(raw)
}

func assertExternalGitAdmissionRefused(t *testing.T, fixture externalGitContainmentFixture, want string) {
	t.Helper()
	before := captureExternalGitContainment(t, fixture.root)
	beforeRefs := externalGitRefs(t, fixture.root)
	unrelatedBefore := captureExternalGitContainment(t, fixture.unrelated)
	outsideBefore := snapshotExternalTree(t, fixture.outside)
	steps := 0
	_, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: fixture.root}, TaskRunInput{
		OperationID: "external-git-refusal",
		TaskID:      fixture.taskID,
		MaxCycles:   1,
		Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			steps++
			return autonomoustaskrun.StepResult{}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), want) || steps != 0 {
		t.Fatalf("refusal error=%v steps=%d, want %q before any task step", err, steps, want)
	}
	assertExternalGitContainmentEqual(t, "control worktree", captureExternalGitContainment(t, fixture.root), before)
	assertExternalGitRefsEqual(t, externalGitRefs(t, fixture.root), beforeRefs)
	assertExternalGitContainmentEqual(t, "unrelated worktree", captureExternalGitContainment(t, fixture.unrelated), unrelatedBefore)
	if after := snapshotExternalTree(t, fixture.outside); !reflect.DeepEqual(after, outsideBefore) {
		t.Fatalf("outside sentinel changed\nbefore=%v\nafter=%v", outsideBefore, after)
	}
}

func assertExternalGitPublishedDelta(t *testing.T, fixture externalGitContainmentFixture, objectIDSize int, concurrentExternalCommit bool) {
	t.Helper()
	ctx := context.Background()
	effective, err := admitExternalMode(ctx, fixture.root, PreflightModeAttendedTask, 1, nil, false)
	if err != nil {
		t.Fatalf("external Git fixture admission: %v", err)
	}
	controlBefore := captureExternalGitContainment(t, fixture.root)
	refsBefore := externalGitRefs(t, fixture.root)
	unrelatedBefore := captureExternalGitContainment(t, fixture.unrelated)
	outsideBefore := snapshotExternalTree(t, fixture.outside)

	workspace, _, err := prepareTaskWorkspace(ctx, fixture.root, fixture.taskID, "published-operation", effective, fixedExternalGitClock)
	if err != nil {
		t.Fatalf("prepare exact task workspace: %v", err)
	}
	if len(workspace.BaselineSHA) != objectIDSize || workspace.BaselineSHA != fixture.baseline {
		t.Fatalf("workspace baseline = %q, want %d-byte exact baseline %q", workspace.BaselineSHA, objectIDSize, fixture.baseline)
	}
	assertExternalGitContainmentEqual(t, "control worktree after workspace preparation", captureExternalGitContainment(t, fixture.root), controlBefore)
	assertExternalGitContainmentEqual(t, "unrelated worktree after workspace preparation", captureExternalGitContainment(t, fixture.unrelated), unrelatedBefore)
	if after := snapshotExternalTree(t, fixture.outside); !reflect.DeepEqual(after, outsideBefore) {
		t.Fatalf("workspace preparation changed outside sentinel\nbefore=%v\nafter=%v", outsideBefore, after)
	}

	workspaceSentinelBefore := snapshotExternalTree(t, filepath.Join(workspace.ExecutionRoot, "containment-sentinel"))
	preRun, err := gitstate.CaptureDirtyWorktree(ctx, gitstate.Config{WorkingDir: workspace.ExecutionRoot, GitExecutable: effective.GitExecutable})
	if err != nil || preRun.CaptureError != "" || len(preRun.DirtyFiles) != 0 {
		t.Fatalf("capture clean task workspace: capture=%+v err=%v", preRun, err)
	}
	writeExternalGitFile(t, filepath.Join(workspace.ExecutionRoot, "result.txt"), "exact task result\n", 0o644)
	postRun, err := gitstate.CaptureChangedFiles(ctx, gitstate.Config{WorkingDir: workspace.ExecutionRoot, GitExecutable: effective.GitExecutable})
	if err != nil || postRun.CaptureError != "" || !reflect.DeepEqual(postRun.ChangedFiles, []string{"result.txt"}) {
		t.Fatalf("capture exact task delta: capture=%+v err=%v", postRun, err)
	}

	var externalHead string
	var externalAuthority externalGitContainmentSnapshot
	externalCommitStarted := false
	commandRunner := func(commandCtx context.Context, command runner.Command) runner.Result {
		if concurrentExternalCommit && !externalCommitStarted && command.Dir == workspace.ExecutionRoot && externalGitSubcommand(command.Args) == "add" {
			externalCommitStarted = true
			writeExternalGitFile(t, filepath.Join(fixture.root, "external.txt"), "concurrent operator commit\n", 0o644)
			runSchedulingGit(t, fixture.root, "add", "--", "external.txt")
			runSchedulingGit(t, fixture.root, "commit", "-q", "-m", "Concurrent operator commit")
			externalHead = runSchedulingGit(t, fixture.root, "rev-parse", "HEAD")
			externalAuthority = captureExternalGitContainment(t, fixture.root)
		}
		return runner.Run(commandCtx, command)
	}
	commitResult, err := commitpkg.Run(ctx, commitpkg.Config{
		WorkingDir:  workspace.ExecutionRoot,
		RunID:       "external-git-worker",
		TaskID:      fixture.taskID,
		TaskSummary: "Publish exact external Git containment delta",
		CodexResult: &codexexec.Result{ExitCode: 0},
		VerificationResult: &verification.Result{
			Status: verification.StatusPassed, Passed: true, FailedCommandIndex: -1,
			Commands: []verification.CommandResult{{Index: 0, Command: "test exact task result", Name: "test", Status: verification.StatusPassed, Passed: true, ExitCode: 0}},
		},
		PreRunDirty:    &preRun,
		PostRunChanged: &postRun,
		GitExecutable:  effective.GitExecutable,
		Timeout:        effective.GitTimeout,
		StdoutCap:      effective.GitStdoutCap,
		StderrCap:      effective.GitStderrCap,
		CommandRunner:  commandRunner,
	})
	if err != nil || commitResult.Status != commitpkg.StatusCommitted {
		t.Fatalf("publish exact task delta: result=%+v err=%v", commitResult, err)
	}
	if concurrentExternalCommit && (!externalCommitStarted || externalHead == "") {
		t.Fatal("concurrent external commit was not injected before task staging")
	}
	if len(commitResult.CommitSHA) != objectIDSize {
		t.Fatalf("task commit ID size = %d, want %d: %q", len(commitResult.CommitSHA), objectIDSize, commitResult.CommitSHA)
	}

	reconciled, err := autonomousworkspace.ReconcileCommit(ctx, autonomousworkspace.Config{
		ControlRoot: fixture.root, TaskID: fixture.taskID, OperationID: "published-reconcile", BaselineSHA: fixture.baseline,
		GitExecutable: effective.GitExecutable, Timeout: effective.GitTimeout, StdoutCap: effective.GitStdoutCap, StderrCap: effective.GitStderrCap, Clock: fixedExternalGitClock,
	}, workspace, commitResult.CommitSHA)
	if err != nil || reconciled.Workspace.HeadSHA != commitResult.CommitSHA || reconciled.Workspace.Status != autonomous.WorkspaceStatusReady {
		t.Fatalf("reconcile exact task commit: workspace=%+v err=%v", reconciled.Workspace, err)
	}

	wantControl := controlBefore
	if concurrentExternalCommit {
		wantControl = externalAuthority
	}
	assertExternalGitContainmentEqual(t, "control worktree after task publication", captureExternalGitContainment(t, fixture.root), wantControl)
	assertExternalGitContainmentEqual(t, "unrelated worktree after task publication", captureExternalGitContainment(t, fixture.unrelated), unrelatedBefore)
	if after := snapshotExternalTree(t, fixture.outside); !reflect.DeepEqual(after, outsideBefore) {
		t.Fatalf("task publication changed outside sentinel\nbefore=%v\nafter=%v", outsideBefore, after)
	}
	if after := snapshotExternalTree(t, filepath.Join(workspace.ExecutionRoot, "containment-sentinel")); !reflect.DeepEqual(after, workspaceSentinelBefore) {
		t.Fatalf("task publication changed workspace sentinel\nbefore=%v\nafter=%v", workspaceSentinelBefore, after)
	}

	wantRefs := cloneExternalGitRefs(refsBefore)
	if concurrentExternalCommit {
		wantRefs[controlBefore.Branch] = externalHead
	}
	wantRefs[workspace.BranchRef] = commitResult.CommitSHA
	assertExternalGitRefsEqual(t, externalGitRefs(t, fixture.root), wantRefs)
	if got := runSchedulingGit(t, workspace.ExecutionRoot, "symbolic-ref", "HEAD"); got != workspace.BranchRef {
		t.Fatalf("task workspace branch = %q, want %q", got, workspace.BranchRef)
	}
	if got := runSchedulingGit(t, workspace.ExecutionRoot, "rev-parse", "HEAD^"); got != fixture.baseline {
		t.Fatalf("task commit parent = %q, want exact admitted baseline %q", got, fixture.baseline)
	}
	if got := runSchedulingGit(t, workspace.ExecutionRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); got != "result.txt" {
		t.Fatalf("task commit delta = %q, want only result.txt", got)
	}
	if got := runSchedulingGit(t, workspace.ExecutionRoot, "show", "HEAD:result.txt"); got != "exact task result" {
		t.Fatalf("task commit result bytes = %q", got)
	}
	if got := runSchedulingGit(t, workspace.ExecutionRoot, "status", "--porcelain=v1", "--untracked-files=all"); got != "" {
		t.Fatalf("task workspace status after commit = %q, want clean", got)
	}
	if indexTree, headTree := runSchedulingGit(t, workspace.ExecutionRoot, "write-tree"), runSchedulingGit(t, workspace.ExecutionRoot, "rev-parse", "HEAD^{tree}"); indexTree != headTree {
		t.Fatalf("task workspace index tree = %q, want committed tree %q", indexTree, headTree)
	}
	if concurrentExternalCommit {
		assertExternalGitPathAbsent(t, workspace.ExecutionRoot, "HEAD:external.txt")
		assertExternalGitPathAbsent(t, fixture.root, "HEAD:result.txt")
	}
}

func cloneExternalGitRefs(input map[string]string) map[string]string {
	result := make(map[string]string, len(input)+1)
	for ref, oid := range input {
		result[ref] = oid
	}
	return result
}

func externalGitSubcommand(args []string) string {
	for _, arg := range args {
		if arg == "--literal-pathspecs" {
			continue
		}
		return arg
	}
	return ""
}

func assertExternalGitPathAbsent(t *testing.T, root, object string) {
	t.Helper()
	command := exec.Command("git", "cat-file", "-e", object)
	command.Dir = root
	if err := command.Run(); err == nil {
		t.Fatalf("Git object path %q unexpectedly exists in %s", object, root)
	}
}

func fixedExternalGitClock() time.Time {
	return time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC)
}

func (s externalGitContainmentSnapshot) String() string {
	return fmt.Sprintf("head=%s branch=%s branch_oid=%s status=%q index=%v sentinel=%v", s.Head, s.Branch, s.BranchOID, s.Status, s.Index, s.Sentinel)
}
