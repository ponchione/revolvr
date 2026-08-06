// Package workspace owns PostgreSQL-backed managed Git workspaces for admitted
// scheduler runs. Git and filesystem administration stays on the trusted host;
// sandboxes receive only the symbolic managed source returned by SandboxBinding.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/runner"
	"revolvr/internal/sandbox"
)

const (
	StatusPlanned     Status = "planned"
	StatusCreating    Status = "creating"
	StatusReady       Status = "ready"
	StatusActive      Status = "active"
	StatusFrozen      Status = "frozen"
	StatusReconciling Status = "reconciling"
	StatusCompleted   Status = "completed"
	StatusCancelled   Status = "cancelled"
	StatusFailed      Status = "failed"
	StatusCleaned     Status = "cleaned"
)

var (
	ErrConflict            = errors.New("workspace effect conflicts with durable authority")
	ErrUnsafePath          = errors.New("unsafe managed workspace path")
	ErrWrongSourceRevision = errors.New("managed repository does not contain the scheduler-pinned source revision")
	ErrIllegalTransition   = errors.New("illegal workspace lifecycle transition")
	ErrCleanupFailed       = errors.New("managed workspace cleanup failed")
	ErrNoChanges           = errors.New("workspace has no changes to capture")
	ErrInjectedCrash       = errors.New("injected workspace crash")
)

type Status string

type FailurePoint string

const (
	FailureAfterBranch   FailurePoint = "after_branch_creation"
	FailureAfterWorktree FailurePoint = "after_worktree_creation"
	FailureAfterCommit   FailurePoint = "after_candidate_commit"
)

type FailureInjector func(FailurePoint) error

type CommandRunner func(context.Context, runner.Command) runner.Result

// Config contains only trusted host authority. WorkspaceRoot and ArtifactRoot
// must already exist and are never derived from model or sandbox input.
type Config struct {
	Pool            *pgxpool.Pool
	WorkspaceRoot   string
	ArtifactRoot    string
	GitExecutable   string
	Timeout         time.Duration
	StdoutCap       int
	StderrCap       int
	Clock           func() time.Time
	CommandRunner   CommandRunner
	FailureInjector FailureInjector
}

type CreateRequest struct {
	WorkspaceID      string
	RunID            string
	OperationID      string
	SymbolicSourceID string
}

type TransitionRequest struct {
	WorkspaceID string
	OperationID string
	Reason      string
}

type CommitRequest struct {
	WorkspaceID string
	OperationID string
	Summary     string
}

type CheckoutIdentity struct {
	SchemaVersion  string `json:"schema_version"`
	CanonicalPath  string `json:"canonical_path"`
	Device         uint64 `json:"device"`
	Inode          uint64 `json:"inode"`
	HeadCommit     string `json:"head_commit"`
	HeadTree       string `json:"head_tree"`
	Branch         string `json:"branch,omitempty"`
	StatusSHA256   string `json:"status_sha256"`
	StatusBytes    int    `json:"status_bytes"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
	IndexSHA256    string `json:"index_sha256"`
	WorktreeSHA256 string `json:"worktree_sha256"`
}

type Change struct {
	Status  string `json:"status"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
}

type Workspace struct {
	ID                    string
	RunID                 string
	ProjectID             string
	ProjectSourceID       string
	TaskID                string
	CreationOperationID   string
	SymbolicSourceID      string
	Status                Status
	TerminalStatus        Status
	AggregateVersion      int64
	OriginalCheckoutPath  string
	ManagedRepositoryPath string
	WorkspaceRoot         string
	Path                  string
	BranchRef             string
	SourceCommit          string
	SourceTree            string
	Device                uint64
	Inode                 uint64
	OriginalBefore        CheckoutIdentity
	OriginalAfter         *CheckoutIdentity
	GitStatus             []byte
	ChangedManifest       []Change
	DiffArtifactID        string
	DiffArtifactPath      string
	DiffSHA256            string
	CandidateCommit       string
	CandidateTree         string
	TerminalReason        string
	CleanupCompletedAt    *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Replayed              bool
}

type CommitEvidence struct {
	Workspace        Workspace
	GitStatus        []byte
	ChangedManifest  []Change
	DiffArtifactID   string
	DiffArtifactPath string
	DiffSHA256       string
	CandidateCommit  string
	CandidateTree    string
}

type SandboxMountBinding struct {
	Source             sandbox.ManagedSource
	Mount              sandbox.Mount
	ForbiddenHostPaths []string
}

func (w Workspace) SandboxBinding() (SandboxMountBinding, error) {
	if w.Status != StatusReady && w.Status != StatusActive && w.Status != StatusFrozen {
		return SandboxMountBinding{}, fmt.Errorf("%w: workspace status %q cannot be mounted", ErrIllegalTransition, w.Status)
	}
	if w.Device == 0 || w.Inode == 0 || w.SymbolicSourceID == "" {
		return SandboxMountBinding{}, fmt.Errorf("%w: workspace mount identity is incomplete", ErrConflict)
	}
	return SandboxMountBinding{
		Source: sandbox.ManagedSource{
			ID: w.SymbolicSourceID, Root: w.WorkspaceRoot,
			RelativePath: filepath.Base(w.Path), Kind: sandbox.SourceWorkspace,
			Type: sandbox.SourceDirectory, Target: "/workspace",
		},
		Mount:              sandbox.Mount{SourceID: w.SymbolicSourceID, Target: "/workspace", Mode: sandbox.MountReadWrite},
		ForbiddenHostPaths: []string{w.OriginalCheckoutPath, w.ManagedRepositoryPath},
	}, nil
}

type ConflictError struct {
	Effect string
	Detail string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s: %s", e.Effect, e.Detail)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }
