// Package autonomousworkspace owns harness-created per-task Git worktrees.
// Canonical task/runtime state stays in the control root; every source and Git
// command that can mutate task content runs in the validated execution root.
package autonomousworkspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/gitstate"
	"revolvr/internal/lock"
	"revolvr/internal/runner"
)

var (
	ErrGitConflict       = errors.New("task workspace Git ownership conflict")
	ErrUnsafeOwnership   = errors.New("unsafe task workspace ownership")
	ErrCleanupRefused    = errors.New("task workspace cleanup refused")
	ErrOperationConflict = errors.New("task workspace operation conflict")
	ErrWorkspaceBusy     = errors.New("task workspace source writer is active")
)

const ownerSchema = "revolvr-workspace-owner-v1"

type CommandRunner func(context.Context, runner.Command) runner.Result

type Config struct {
	ControlRoot   string
	TaskID        string
	OperationID   string
	BaselineSHA   string
	GitExecutable string
	Timeout       time.Duration
	StdoutCap     int
	StderrCap     int
	Clock         func() time.Time
	CommandRunner CommandRunner
}

type CommandEvidence struct {
	Args                 []string  `json:"args"`
	Directory            string    `json:"directory"`
	ExitCode             int       `json:"exit_code"`
	TimedOut             bool      `json:"timed_out,omitempty"`
	Error                string    `json:"error,omitempty"`
	Stdout               string    `json:"stdout,omitempty"`
	Stderr               string    `json:"stderr,omitempty"`
	StdoutTruncatedBytes int64     `json:"stdout_truncated_bytes,omitempty"`
	StderrTruncatedBytes int64     `json:"stderr_truncated_bytes,omitempty"`
	StartedAt            time.Time `json:"started_at"`
	EndedAt              time.Time `json:"ended_at"`
}

type Result struct {
	Workspace    autonomous.TaskWorkspace `json:"workspace"`
	PrimaryDirty []string                 `json:"primary_dirty,omitempty"`
	Commands     []CommandEvidence        `json:"commands,omitempty"`
	Reopened     bool                     `json:"reopened"`
}

type ownerMarker struct {
	SchemaVersion       string    `json:"schema_version"`
	TaskID              string    `json:"task_id"`
	WorkspaceID         string    `json:"workspace_id"`
	ControlRoot         string    `json:"control_root"`
	ExecutionRoot       string    `json:"execution_root"`
	GitCommonDir        string    `json:"git_common_dir"`
	BranchRef           string    `json:"branch_ref"`
	BaselineSHA         string    `json:"baseline_sha"`
	CreationOperationID string    `json:"creation_operation_id"`
	MaterialSHA256      string    `json:"material_sha256"`
	CreatedAt           time.Time `json:"created_at"`
}

type normalized struct {
	Config
	root, common, workspaceID, execution, branchRef, markerPath string
	commands                                                    []CommandEvidence
}

func Prepare(ctx context.Context, cfg Config) (Result, error) {
	n, err := normalize(ctx, cfg)
	if err != nil {
		return Result{}, err
	}
	unlock, err := acquireAdminLock(ctx, n.root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()

	dirty, err := gitstate.CaptureDirtyWorktree(ctx, gitstate.Config{WorkingDir: n.root, GitExecutable: n.GitExecutable, Timeout: n.Timeout, StdoutCap: n.StdoutCap, StderrCap: n.StderrCap, CommandRunner: gitstate.CommandRunner(n.CommandRunner)})
	if err != nil || dirty.CaptureError != "" {
		return Result{}, errors.Join(err, fmt.Errorf("capture primary worktree evidence: %s", dirty.CaptureError))
	}

	marker, found, err := readMarker(n.markerPath)
	if err != nil {
		return Result{}, err
	}
	if found {
		if err := validateMarker(marker, n); err != nil {
			return Result{}, err
		}
		if err := recoverOwnedCreation(ctx, n); err != nil {
			return Result{}, err
		}
		workspace, err := reopen(ctx, n, marker, nil)
		return Result{Workspace: workspace, PrimaryDirty: sourcePaths(dirty.Paths), Commands: n.commands, Reopened: true}, err
	}
	if err := ensureAbsentSafePath(n.root, n.execution); err != nil {
		return Result{}, err
	}
	if _, exists, err := refOID(ctx, n, n.branchRef); err != nil {
		return Result{}, err
	} else if exists {
		return Result{}, fmt.Errorf("%w: branch %s already exists without the exact ownership marker", ErrGitConflict, n.branchRef)
	}
	marker = ownerMarker{SchemaVersion: ownerSchema, TaskID: n.TaskID, WorkspaceID: n.workspaceID, ControlRoot: n.root, ExecutionRoot: n.execution, GitCommonDir: n.common, BranchRef: n.branchRef, BaselineSHA: n.BaselineSHA, CreationOperationID: n.OperationID, CreatedAt: n.Clock().UTC()}
	marker.MaterialSHA256 = markerMaterial(marker)
	if err := writeMarker(n.markerPath, marker); err != nil {
		return Result{}, err
	}
	if _, err := n.git(ctx, n.root, "update-ref", n.branchRef, n.BaselineSHA, strings.Repeat("0", len(n.BaselineSHA))); err != nil {
		return Result{}, err
	}
	shortBranch := strings.TrimPrefix(n.branchRef, "refs/heads/")
	if _, err := n.git(ctx, n.root, "worktree", "add", n.execution, shortBranch); err != nil {
		return Result{}, err
	}
	workspace, err := reopen(ctx, n, marker, nil)
	return Result{Workspace: workspace, PrimaryDirty: sourcePaths(dirty.Paths), Commands: n.commands}, err
}

func recoverOwnedCreation(ctx context.Context, n *normalized) error {
	_, refExists, err := refOID(ctx, n, n.branchRef)
	if err != nil {
		return err
	}
	if !refExists {
		if _, err := n.git(ctx, n.root, "update-ref", n.branchRef, n.BaselineSHA, strings.Repeat("0", len(n.BaselineSHA))); err != nil {
			return err
		}
	}
	registry, err := n.git(ctx, n.root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	if registered(registry, n.execution, n.branchRef) {
		return nil
	}
	if _, err := os.Lstat(n.execution); err == nil {
		return fmt.Errorf("%w: unregistered execution path exists during owned creation recovery", ErrGitConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ensureAbsentSafePath(n.root, n.execution); err != nil {
		return err
	}
	shortBranch := strings.TrimPrefix(n.branchRef, "refs/heads/")
	_, err = n.git(ctx, n.root, "worktree", "add", n.execution, shortBranch)
	return err
}

// Reopen verifies marker, common-dir, registry, .git link, branch, HEAD, tree,
// source identity, and cleanliness. It never guesses ownership from names.
func Reopen(ctx context.Context, cfg Config, expected autonomous.TaskWorkspace) (Result, error) {
	n, err := normalize(ctx, cfg)
	if err != nil {
		return Result{}, err
	}
	if err := expected.Validate(); err != nil {
		return Result{}, err
	}
	unlock, err := acquireAdminLock(ctx, n.root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if expected.TaskID != n.TaskID || expected.WorkspaceID != n.workspaceID || expected.ControlRoot != n.root || expected.ExecutionRoot != n.execution || expected.GitCommonDir != n.common || expected.BranchRef != n.branchRef || expected.BaselineSHA != n.BaselineSHA {
		return Result{}, fmt.Errorf("%w: expected workspace identity does not match deterministic authority", ErrUnsafeOwnership)
	}
	marker, found, err := readMarker(n.markerPath)
	if err != nil || !found {
		return Result{}, errors.Join(err, fmt.Errorf("%w: ownership marker is missing", ErrUnsafeOwnership))
	}
	workspace, err := reopen(ctx, n, marker, &expected)
	return Result{Workspace: workspace, Commands: n.commands, Reopened: true}, err
}

// ReconcileCommit classifies only an exact caller-observed post-commit HEAD.
// It is the bridge for the existing pre/post-HEAD commit reconciliation rule;
// an unavailable or different HEAD remains ambiguous and inspectable.
func ReconcileCommit(ctx context.Context, cfg Config, expected autonomous.TaskWorkspace, observedHead string) (Result, error) {
	n, err := normalize(ctx, cfg)
	if err != nil {
		return Result{}, err
	}
	if err := expected.Validate(); err != nil {
		return Result{}, err
	}
	unlock, err := acquireAdminLock(ctx, n.root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if !validOID(observedHead) || observedHead == expected.HeadSHA {
		return Result{}, errors.New("reconcile workspace commit: a distinct full observed HEAD is required")
	}
	marker, found, err := readMarker(n.markerPath)
	if err != nil || !found {
		return Result{}, errors.Join(err, fmt.Errorf("%w: ownership marker is missing", ErrUnsafeOwnership))
	}
	trusted := expected
	trusted.HeadSHA = observedHead
	workspace, err := reopen(ctx, n, marker, &trusted)
	if err != nil {
		return Result{}, err
	}
	if workspace.HeadSHA != observedHead {
		return Result{}, fmt.Errorf("%w: actual HEAD differs from the observed commit", ErrGitConflict)
	}
	workspace.Status = autonomous.WorkspaceStatusReady
	workspace.UpdatedAt = n.Clock().UTC()
	return Result{Workspace: workspace, Commands: n.commands, Reopened: true}, nil
}

func AdvanceCheckpoint(ctx context.Context, cfg Config, expected autonomous.TaskWorkspace, provenance string) (Result, error) {
	result, err := Reopen(ctx, cfg, expected)
	if err != nil {
		return result, err
	}
	n, _ := normalize(ctx, cfg)
	if result.Workspace.Status == autonomous.WorkspaceStatusAmbiguous || len(result.Workspace.RetainedRefs) != len(expected.RetainedRefs) {
		return result, fmt.Errorf("%w: workspace has an unclassified or newly ambiguous commit", ErrGitConflict)
	}
	dirty, err := captureDirty(ctx, n, n.execution)
	if err != nil || len(dirty) != 0 {
		return result, errors.Join(err, fmt.Errorf("%w: checkpoint requires a clean execution worktree", ErrGitConflict))
	}
	result.Workspace.Checkpoint = autonomous.WorkspaceCheckpoint{Sequence: expected.Checkpoint.Sequence + 1, CommitSHA: result.Workspace.HeadSHA, TreeSHA: result.Workspace.TreeSHA, SourceRevision: result.Workspace.SourceRevision, OperationID: cfg.OperationID, Provenance: strings.TrimSpace(provenance), CreatedAt: n.Clock().UTC()}
	result.Workspace.UpdatedAt = n.Clock().UTC()
	if err := result.Workspace.Validate(); err != nil {
		return result, err
	}
	return result, nil
}

// Restore retains an immutable failed-state commit/ref before resetting only
// the revalidated harness worktree to its exact durable checkpoint.
func Restore(ctx context.Context, cfg Config, expected autonomous.TaskWorkspace, reason string) (Result, error) {
	if strings.TrimSpace(reason) == "" {
		return Result{}, errors.New("restore workspace: failed-state reason is required")
	}
	result, err := Reopen(ctx, cfg, expected)
	if err != nil {
		return result, err
	}
	n, _ := normalize(ctx, cfg)
	unlock, err := acquireAdminLock(ctx, n.root)
	if err != nil {
		return result, err
	}
	defer unlock()
	if err := refuseActiveWriter(ctx, n); err != nil {
		return result, err
	}
	marker, _, err := readMarker(n.markerPath)
	if err != nil {
		return result, err
	}
	revalidated, err := reopen(ctx, n, marker, &expected)
	if err != nil {
		return result, err
	}
	result.Workspace = revalidated
	from := result.Workspace.HeadSHA
	dirty, err := captureDirty(ctx, n, n.execution)
	if err != nil {
		return result, err
	}
	ignored, err := ignoredPaths(ctx, n)
	if err != nil {
		return result, err
	}
	if len(ignored) != 0 {
		return result, fmt.Errorf("%w: ignored files cannot be retained safely: %v", ErrUnsafeOwnership, ignored)
	}
	if from != expected.Checkpoint.CommitSHA || len(dirty) != 0 {
		retained, err := retainCurrent(ctx, n, result.Workspace, reason)
		if err != nil {
			return result, err
		}
		result.Workspace.RetainedRefs = append(result.Workspace.RetainedRefs, retained)
	}
	if _, err := n.git(ctx, n.execution, "reset", "--hard", expected.Checkpoint.CommitSHA); err != nil {
		return result, err
	}
	if _, err := n.git(ctx, n.execution, "clean", "-fd"); err != nil {
		return result, err
	}
	refreshed, err := reopen(ctx, n, mustMarker(n.markerPath), &expected)
	if err != nil {
		return result, err
	}
	refreshed.RetainedRefs = result.Workspace.RetainedRefs
	refreshed.Status = autonomous.WorkspaceStatusRestored
	refreshed.LastRecovery = &autonomous.WorkspaceRecovery{OperationID: n.OperationID, Kind: "failed_attempt_restored", FromCommitSHA: from, ToCommitSHA: expected.Checkpoint.CommitSHA, SourceRevision: refreshed.SourceRevision, CreatedAt: n.Clock().UTC()}
	if len(refreshed.RetainedRefs) > 0 {
		refreshed.LastRecovery.RetainedRef = refreshed.RetainedRefs[len(refreshed.RetainedRefs)-1].Ref
	}
	if refreshed.HeadSHA != expected.Checkpoint.CommitSHA || refreshed.TreeSHA != expected.Checkpoint.TreeSHA || refreshed.SourceRevision != expected.Checkpoint.SourceRevision {
		return result, fmt.Errorf("%w: post-restore identity does not match the durable checkpoint", ErrUnsafeOwnership)
	}
	result.Workspace, result.Commands = refreshed, n.commands
	return result, nil
}

func Cleanup(ctx context.Context, cfg Config, expected autonomous.TaskWorkspace) (Result, error) {
	result, err := Reopen(ctx, cfg, expected)
	if err != nil {
		if errors.Is(err, gitstate.ErrPolicyRelevantIgnored) {
			return result, errors.Join(ErrCleanupRefused, err)
		}
		return result, err
	}
	n, _ := normalize(ctx, cfg)
	unlock, err := acquireAdminLock(ctx, n.root)
	if err != nil {
		return result, err
	}
	defer unlock()
	if err := refuseActiveWriter(ctx, n); err != nil {
		return result, err
	}
	marker, _, err := readMarker(n.markerPath)
	if err != nil {
		return result, err
	}
	revalidated, err := reopen(ctx, n, marker, &expected)
	if err != nil {
		return result, err
	}
	result.Workspace = revalidated
	dirty, err := captureDirty(ctx, n, n.execution)
	if err != nil || len(dirty) != 0 || result.Workspace.HeadSHA != expected.Checkpoint.CommitSHA || len(expected.RetainedRefs) != 0 || expected.Status == autonomous.WorkspaceStatusAmbiguous {
		return result, errors.Join(err, fmt.Errorf("%w: workspace is dirty, advanced, ambiguous, or retains required evidence", ErrCleanupRefused))
	}
	ignored, err := ignoredPaths(ctx, n)
	if err != nil {
		return result, err
	}
	if len(ignored) != 0 {
		return result, fmt.Errorf("%w: ignored files are present: %v", ErrCleanupRefused, ignored)
	}
	registry, err := n.git(ctx, n.root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return result, err
	}
	if registrationLocked(registry, n.execution) {
		return result, fmt.Errorf("%w: Git worktree is locked", ErrCleanupRefused)
	}
	if _, err := n.git(ctx, n.root, "worktree", "remove", n.execution); err != nil {
		return result, err
	}
	if _, err := n.git(ctx, n.root, "update-ref", "-d", n.branchRef, result.Workspace.HeadSHA); err != nil {
		return result, err
	}
	result.Workspace.Status = autonomous.WorkspaceStatusCleaned
	result.Workspace.UpdatedAt = n.Clock().UTC()
	result.Commands = n.commands
	return result, nil
}

func refuseActiveWriter(ctx context.Context, n *normalized) error {
	metadata, found, err := lock.ReadWorkspaceSourceWriter(ctx, n.root, n.workspaceID)
	if err != nil {
		return err
	}
	if found && metadata.ExpiresAt.After(n.Clock().UTC()) {
		return fmt.Errorf("%w: source writer %s still owns workspace %s", ErrWorkspaceBusy, metadata.RunID, n.workspaceID)
	}
	return nil
}

func normalize(ctx context.Context, cfg Config) (*normalized, error) {
	root, err := canonicalDirectory(cfg.ControlRoot)
	if err != nil {
		return nil, fmt.Errorf("workspace control root: %w", err)
	}
	if strings.TrimSpace(cfg.TaskID) == "" || cfg.TaskID != strings.TrimSpace(cfg.TaskID) || strings.ContainsAny(cfg.TaskID, "\r\n/") {
		return nil, errors.New("workspace task ID is empty or malformed")
	}
	if strings.TrimSpace(cfg.OperationID) == "" || cfg.OperationID != strings.TrimSpace(cfg.OperationID) || strings.ContainsAny(cfg.OperationID, "\r\n") {
		return nil, errors.New("workspace operation ID is empty or malformed")
	}
	if cfg.GitExecutable == "" {
		cfg.GitExecutable = "git"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.StdoutCap <= 0 {
		cfg.StdoutCap = 1 << 20
	}
	if cfg.StderrCap <= 0 {
		cfg.StderrCap = 1 << 20
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}
	n := &normalized{Config: cfg, root: root}
	commonRaw, err := n.git(ctx, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	common := strings.TrimSpace(commonRaw)
	if !filepath.IsAbs(common) {
		common = filepath.Join(root, common)
	}
	common, err = filepath.EvalSymlinks(filepath.Clean(common))
	if err != nil {
		return nil, fmt.Errorf("resolve Git common directory: %w", err)
	}
	n.common = common
	base := strings.TrimSpace(cfg.BaselineSHA)
	if base == "" {
		base = "HEAD"
	}
	baseline, err := n.git(ctx, root, "rev-parse", "--verify", base+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("resolve exact workspace baseline: %w", err)
	}
	n.BaselineSHA = strings.TrimSpace(baseline)
	identity := sha256.Sum256([]byte(root + "\x00" + common + "\x00" + cfg.TaskID))
	n.workspaceID = hex.EncodeToString(identity[:16])
	n.execution = filepath.Join(root, ".revolvr", "autonomous", "worktrees", n.workspaceID)
	taskSlug := slug(cfg.TaskID)
	n.branchRef = "refs/heads/revolvr/tasks/" + taskSlug + "-" + n.workspaceID[:12]
	n.markerPath = filepath.Join(root, ".revolvr", "autonomous", "tasks", cfg.TaskID, "workspace-owner.json")
	return n, nil
}

func reopen(ctx context.Context, n *normalized, marker ownerMarker, trusted *autonomous.TaskWorkspace) (autonomous.TaskWorkspace, error) {
	if err := validateMarker(marker, n); err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	branchOID, exists, err := refOID(ctx, n, n.branchRef)
	if err != nil || !exists {
		return autonomous.TaskWorkspace{}, errors.Join(err, fmt.Errorf("%w: owned branch is missing", ErrGitConflict))
	}
	registry, err := n.git(ctx, n.root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	if !registered(registry, n.execution, n.branchRef) {
		return autonomous.TaskWorkspace{}, fmt.Errorf("%w: exact worktree registration is missing or mismatched", ErrUnsafeOwnership)
	}
	gitLink, err := os.ReadFile(filepath.Join(n.execution, ".git"))
	if err != nil || !strings.HasPrefix(string(gitLink), "gitdir: ") {
		return autonomous.TaskWorkspace{}, errors.Join(err, fmt.Errorf("%w: linked-worktree .git file is invalid", ErrUnsafeOwnership))
	}
	actualCommonRaw, err := n.git(ctx, n.execution, "rev-parse", "--git-common-dir")
	if err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	actualCommon := strings.TrimSpace(actualCommonRaw)
	if !filepath.IsAbs(actualCommon) {
		actualCommon = filepath.Join(n.execution, actualCommon)
	}
	actualCommon, err = filepath.EvalSymlinks(filepath.Clean(actualCommon))
	if err != nil || actualCommon != n.common {
		return autonomous.TaskWorkspace{}, errors.Join(err, fmt.Errorf("%w: Git common directory mismatch", ErrUnsafeOwnership))
	}
	head, err := n.git(ctx, n.execution, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	head = strings.TrimSpace(head)
	if head != branchOID {
		return autonomous.TaskWorkspace{}, fmt.Errorf("%w: branch and worktree HEAD differ", ErrGitConflict)
	}
	tree, err := n.git(ctx, n.execution, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	snapshot, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{WorkingDir: n.execution, GitExecutable: n.GitExecutable, Timeout: n.Timeout, StdoutCap: n.StdoutCap, StderrCap: n.StderrCap, CommandRunner: gitstate.CommandRunner(n.CommandRunner)})
	if err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	revision, err := gitstate.PolicySourceRevision(snapshot)
	if err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	now := n.Clock().UTC()
	w := autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: n.TaskID, WorkspaceID: n.workspaceID, ControlRoot: n.root, ExecutionRoot: n.execution, GitCommonDir: n.common, BranchRef: n.branchRef, OwnerMarker: n.markerPath, BaselineSHA: n.BaselineSHA, HeadSHA: head, TreeSHA: strings.TrimSpace(tree), SourceRevision: revision, Status: autonomous.WorkspaceStatusReady, CreatedAt: marker.CreatedAt, UpdatedAt: now}
	w.Checkpoint = autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: n.BaselineSHA, TreeSHA: w.TreeSHA, SourceRevision: revision, OperationID: marker.CreationOperationID, Provenance: "exact captured control-repository commit", CreatedAt: marker.CreatedAt}
	if trusted != nil {
		w.Checkpoint = trusted.Checkpoint
		w.RetainedRefs = append([]autonomous.WorkspaceRetainedRef(nil), trusted.RetainedRefs...)
		w.LastRecovery = trusted.LastRecovery
		w.Status = trusted.Status
		if w.Status == autonomous.WorkspaceStatusCleaned {
			return autonomous.TaskWorkspace{}, fmt.Errorf("%w: cleaned workspace is unexpectedly registered", ErrUnsafeOwnership)
		}
	}
	if head != n.BaselineSHA && (trusted == nil || head != trusted.HeadSHA) {
		w.Status = autonomous.WorkspaceStatusAmbiguous
		retainedRef := retainedRef(n.workspaceID, "ambiguous", head)
		if _, err := n.git(ctx, n.root, "update-ref", retainedRef, head); err != nil {
			return autonomous.TaskWorkspace{}, err
		}
		already := false
		for _, retained := range w.RetainedRefs {
			if retained.Ref == retainedRef && retained.CommitSHA == head {
				already = true
			}
		}
		if !already {
			w.RetainedRefs = append(w.RetainedRefs, autonomous.WorkspaceRetainedRef{Ref: retainedRef, CommitSHA: head, TreeSHA: w.TreeSHA, SourceRevision: revision, Reason: "branch advanced before durable checkpoint reconciliation", OperationID: n.OperationID, CreatedAt: now})
		}
	}
	if err := w.Validate(); err != nil {
		return autonomous.TaskWorkspace{}, err
	}
	return w, nil
}

func retainCurrent(ctx context.Context, n *normalized, w autonomous.TaskWorkspace, reason string) (autonomous.WorkspaceRetainedRef, error) {
	if _, err := n.git(ctx, n.execution, "add", "-A"); err != nil {
		return autonomous.WorkspaceRetainedRef{}, err
	}
	tree, err := n.git(ctx, n.execution, "write-tree")
	if err != nil {
		return autonomous.WorkspaceRetainedRef{}, err
	}
	tree = strings.TrimSpace(tree)
	commitResult := n.run(ctx, n.execution, []string{"commit-tree", tree, "-p", w.HeadSHA}, []byte("Revolvr retained failed workspace state\n"))
	if err := commandError(commitResult); err != nil {
		return autonomous.WorkspaceRetainedRef{}, err
	}
	commit := strings.TrimSpace(commitResult.Stdout)
	ref := retainedRef(n.workspaceID, "failed", commit)
	if _, err := n.git(ctx, n.root, "update-ref", ref, commit); err != nil {
		return autonomous.WorkspaceRetainedRef{}, err
	}
	snapshot, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{WorkingDir: n.execution, GitExecutable: n.GitExecutable, Timeout: n.Timeout, StdoutCap: n.StdoutCap, StderrCap: n.StderrCap, CommandRunner: gitstate.CommandRunner(n.CommandRunner)})
	if err != nil {
		return autonomous.WorkspaceRetainedRef{}, err
	}
	revision, err := gitstate.PolicySourceRevision(snapshot)
	if err != nil {
		return autonomous.WorkspaceRetainedRef{}, err
	}
	return autonomous.WorkspaceRetainedRef{Ref: ref, CommitSHA: commit, TreeSHA: tree, SourceRevision: revision, Reason: strings.TrimSpace(reason), OperationID: n.OperationID, CreatedAt: n.Clock().UTC()}, nil
}

func captureDirty(ctx context.Context, n *normalized, dir string) ([]string, error) {
	c, err := gitstate.CaptureDirtyWorktree(ctx, gitstate.Config{WorkingDir: dir, GitExecutable: n.GitExecutable, Timeout: n.Timeout, StdoutCap: n.StdoutCap, StderrCap: n.StderrCap, CommandRunner: gitstate.CommandRunner(n.CommandRunner)})
	if err != nil || c.CaptureError != "" {
		return nil, errors.Join(err, errors.New(c.CaptureError))
	}
	return c.Paths, nil
}

func ignoredPaths(ctx context.Context, n *normalized) ([]string, error) {
	raw, err := n.git(ctx, n.execution, "status", "--porcelain=v1", "-z", "--ignored", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	entries, err := gitstate.ParsePorcelainV1Z(raw)
	if err != nil {
		return nil, fmt.Errorf("parse ignored worktree status: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Kind == gitstate.KindIgnored {
			paths = append(paths, entry.Path)
		}
	}
	return paths, nil
}

func refOID(ctx context.Context, n *normalized, ref string) (string, bool, error) {
	r := n.run(ctx, n.root, []string{"show-ref", "--verify", "--quiet", ref}, nil)
	if r.ExitCode == 1 && r.Err == nil && !r.TimedOut {
		return "", false, nil
	}
	if err := commandError(r); err != nil {
		return "", false, err
	}
	oid, err := n.git(ctx, n.root, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(oid), true, nil
}

func (n *normalized) git(ctx context.Context, dir string, args ...string) (string, error) {
	r := n.run(ctx, dir, args, nil)
	if err := commandError(r); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return r.Stdout, nil
}

func (n *normalized) run(ctx context.Context, dir string, args []string, stdin []byte) runner.Result {
	cmd := runner.Command{Name: n.GitExecutable, Args: append([]string(nil), args...), Dir: dir, Timeout: n.Timeout, StdoutLimit: n.StdoutCap, StderrLimit: n.StderrCap}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	started := n.Clock().UTC()
	r := n.CommandRunner(ctx, cmd)
	ended := n.Clock().UTC()
	n.commands = append(n.commands, CommandEvidence{Args: append([]string(nil), args...), Directory: dir, ExitCode: r.ExitCode, TimedOut: r.TimedOut, Error: errorText(r.Err), Stdout: r.Stdout, Stderr: r.Stderr, StdoutTruncatedBytes: r.StdoutTruncatedBytes, StderrTruncatedBytes: r.StderrTruncatedBytes, StartedAt: started, EndedAt: ended})
	return r
}

func commandError(r runner.Result) error {
	if r.Err != nil || r.TimedOut || r.ExitCode != 0 || r.StdoutTruncatedBytes != 0 || r.StderrTruncatedBytes != 0 {
		return fmt.Errorf("command failed (exit %d, timeout %t, error %v, stdout truncated %d, stderr truncated %d): %s", r.ExitCode, r.TimedOut, r.Err, r.StdoutTruncatedBytes, r.StderrTruncatedBytes, strings.TrimSpace(r.Stderr))
	}
	return nil
}

func readMarker(path string) (ownerMarker, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ownerMarker{}, false, nil
	}
	if err != nil {
		return ownerMarker{}, false, err
	}
	var marker ownerMarker
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&marker); err != nil {
		return marker, false, fmt.Errorf("decode workspace ownership marker: %w", err)
	}
	canonical, _ := marshalMarker(marker)
	if !bytes.Equal(raw, canonical) {
		return marker, false, fmt.Errorf("%w: ownership marker is not canonical", ErrUnsafeOwnership)
	}
	return marker, true, nil
}

func writeMarker(path string, marker ownerMarker) error {
	if err := ensureSafeParents(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path)))), filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := marshalMarker(marker)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = syncDir(filepath.Dir(path))
	}
	return err
}

func marshalMarker(marker ownerMarker) ([]byte, error) {
	raw, err := json.MarshalIndent(marker, "", "  ")
	return append(raw, '\n'), err
}
func markerMaterial(marker ownerMarker) string {
	marker.MaterialSHA256 = ""
	raw, _ := json.Marshal(marker)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validateMarker(marker ownerMarker, n *normalized) error {
	if marker.SchemaVersion != ownerSchema || marker.TaskID != n.TaskID || marker.WorkspaceID != n.workspaceID || marker.ControlRoot != n.root || marker.ExecutionRoot != n.execution || marker.GitCommonDir != n.common || marker.BranchRef != n.branchRef || marker.BaselineSHA != n.BaselineSHA || marker.MaterialSHA256 != markerMaterial(marker) || marker.CreatedAt.IsZero() {
		return fmt.Errorf("%w: ownership marker material mismatch", ErrUnsafeOwnership)
	}
	if marker.CreationOperationID == n.OperationID && marker.BaselineSHA != n.BaselineSHA {
		return ErrOperationConflict
	}
	return nil
}

func mustMarker(path string) ownerMarker { marker, _, _ := readMarker(path); return marker }

func registered(raw, execution, branchRef string) bool {
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); {
		if fields[i] == "" {
			i++
			continue
		}
		if !strings.HasPrefix(fields[i], "worktree ") {
			i++
			continue
		}
		path := strings.TrimPrefix(fields[i], "worktree ")
		i++
		branch := ""
		for i < len(fields) && !strings.HasPrefix(fields[i], "worktree ") {
			if strings.HasPrefix(fields[i], "branch ") {
				branch = strings.TrimPrefix(fields[i], "branch ")
			}
			i++
		}
		if filepath.Clean(path) == execution && branch == branchRef {
			return true
		}
	}
	return false
}

func registrationLocked(raw, execution string) bool {
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); {
		if !strings.HasPrefix(fields[i], "worktree ") {
			i++
			continue
		}
		path := strings.TrimPrefix(fields[i], "worktree ")
		i++
		locked := false
		for i < len(fields) && !strings.HasPrefix(fields[i], "worktree ") {
			if fields[i] == "locked" || strings.HasPrefix(fields[i], "locked ") {
				locked = true
			}
			i++
		}
		if filepath.Clean(path) == execution {
			return locked
		}
	}
	return false
}

func ensureAbsentSafePath(root, path string) error {
	if !within(root, path) {
		return fmt.Errorf("%w: execution path escapes control root", ErrUnsafeOwnership)
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%w: execution path already exists", ErrGitConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return ensureSafeParents(root, filepath.Dir(path))
}
func ensureSafeParents(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnsafeOwnership
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink component %s", ErrUnsafeOwnership, current)
		}
	}
	return nil
}
func canonicalDirectory(value string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.Join(err, errors.New("not a directory"))
	}
	return filepath.Clean(resolved), nil
}
func within(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func slug(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "task"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}
func retainedRef(workspaceID, kind, commit string) string {
	return "refs/revolvr/retained/" + workspaceID + "/" + kind + "-" + commit[:12]
}
func validOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sourcePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, value := range paths {
		clean := filepath.ToSlash(filepath.Clean(value))
		if clean == ".revolvr" || strings.HasPrefix(clean, ".revolvr/") {
			continue
		}
		out = append(out, value)
	}
	return out
}

func acquireAdminLock(ctx context.Context, root string, afterOpen ...func(root, path string) error) (func(), error) {
	var hook func(root, path string) error
	if len(afterOpen) > 0 {
		hook = afterOpen[0]
	}
	lease, err := lock.AcquireFlock(ctx, root, lock.FlockConfig{
		RelativePath: ".revolvr/locks/git-admin.lock",
		Mode:         lock.FlockExclusive,
		Wait:         true,
		Create:       true,
		AfterOpen:    hook,
	})
	if err != nil {
		return nil, err
	}
	return func() { _ = lease.Close() }, nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// DeterministicJSON is used by persistence and dossier tests.
func DeterministicJSON(workspace autonomous.TaskWorkspace) ([]byte, error) {
	if err := workspace.Validate(); err != nil {
		return nil, err
	}
	copyValue := workspace
	copyValue.RetainedRefs = append([]autonomous.WorkspaceRetainedRef(nil), workspace.RetainedRefs...)
	raw, err := json.MarshalIndent(copyValue, "", "  ")
	return append(raw, '\n'), err
}
