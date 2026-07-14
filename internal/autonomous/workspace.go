package autonomous

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/gitoid"
)

const WorkspaceSchemaVersion = "autonomous-task-workspace-v1"

type WorkspaceStatus string

const (
	WorkspaceStatusPreparing WorkspaceStatus = "preparing"
	WorkspaceStatusReady     WorkspaceStatus = "ready"
	WorkspaceStatusRestored  WorkspaceStatus = "restored"
	WorkspaceStatusAmbiguous WorkspaceStatus = "ambiguous"
	WorkspaceStatusCleaned   WorkspaceStatus = "cleaned"
)

type WorkspaceCheckpoint struct {
	Sequence       int64     `json:"sequence"`
	CommitSHA      string    `json:"commit_sha"`
	TreeSHA        string    `json:"tree_sha"`
	SourceRevision string    `json:"source_revision"`
	OperationID    string    `json:"operation_id"`
	Provenance     string    `json:"provenance"`
	CreatedAt      time.Time `json:"created_at"`
}

type WorkspaceRetainedRef struct {
	Ref            string    `json:"ref"`
	CommitSHA      string    `json:"commit_sha"`
	TreeSHA        string    `json:"tree_sha"`
	SourceRevision string    `json:"source_revision"`
	Reason         string    `json:"reason"`
	OperationID    string    `json:"operation_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type WorkspaceRecovery struct {
	OperationID    string    `json:"operation_id"`
	Kind           string    `json:"kind"`
	FromCommitSHA  string    `json:"from_commit_sha"`
	ToCommitSHA    string    `json:"to_commit_sha"`
	RetainedRef    string    `json:"retained_ref,omitempty"`
	SourceRevision string    `json:"source_revision"`
	CreatedAt      time.Time `json:"created_at"`
}

// TaskWorkspace is the exact authority separating canonical control state
// from task source execution. Paths are canonical absolute paths because a
// relative path could be interpreted against either root after restart.
type TaskWorkspace struct {
	SchemaVersion  string                 `json:"schema_version"`
	TaskID         string                 `json:"task_id"`
	WorkspaceID    string                 `json:"workspace_id"`
	ControlRoot    string                 `json:"control_root"`
	ExecutionRoot  string                 `json:"execution_root"`
	GitCommonDir   string                 `json:"git_common_dir"`
	BranchRef      string                 `json:"branch_ref"`
	OwnerMarker    string                 `json:"owner_marker"`
	BaselineSHA    string                 `json:"baseline_sha"`
	HeadSHA        string                 `json:"head_sha"`
	TreeSHA        string                 `json:"tree_sha"`
	SourceRevision string                 `json:"source_revision"`
	Checkpoint     WorkspaceCheckpoint    `json:"checkpoint"`
	RetainedRefs   []WorkspaceRetainedRef `json:"retained_refs,omitempty"`
	LastRecovery   *WorkspaceRecovery     `json:"last_recovery,omitempty"`
	Status         WorkspaceStatus        `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

func (w TaskWorkspace) Validate() error {
	if w.SchemaVersion != WorkspaceSchemaVersion {
		return fmt.Errorf("validate task workspace: unsupported schema_version %q", w.SchemaVersion)
	}
	if strings.TrimSpace(w.TaskID) == "" || strings.TrimSpace(w.WorkspaceID) == "" || strings.ContainsAny(w.TaskID+w.WorkspaceID, "\r\n") {
		return errors.New("validate task workspace: task_id and workspace_id are required")
	}
	for _, item := range []struct{ label, value string }{{"control_root", w.ControlRoot}, {"execution_root", w.ExecutionRoot}, {"git_common_dir", w.GitCommonDir}, {"owner_marker", w.OwnerMarker}} {
		if item.value == "" || !filepath.IsAbs(item.value) || filepath.Clean(item.value) != item.value {
			return fmt.Errorf("validate task workspace: %s must be a canonical absolute path", item.label)
		}
	}
	if w.ExecutionRoot == w.ControlRoot || !pathWithin(w.ControlRoot, w.ExecutionRoot) || !pathWithin(w.ControlRoot, w.OwnerMarker) {
		return errors.New("validate task workspace: execution root and owner marker must be distinct paths under the control root")
	}
	if !strings.HasPrefix(w.BranchRef, "refs/heads/revolvr/tasks/") || strings.ContainsAny(w.BranchRef, " \t\r\n~^:?*[\\") {
		return errors.New("validate task workspace: branch_ref is not a canonical harness-owned ref")
	}
	for _, item := range []struct{ label, value string }{{"baseline_sha", w.BaselineSHA}, {"head_sha", w.HeadSHA}, {"tree_sha", w.TreeSHA}} {
		if !validGitOID(item.value) {
			return fmt.Errorf("validate task workspace: %s is not a full Git object identity", item.label)
		}
	}
	if !validStateSHA256(w.SourceRevision) {
		return errors.New("validate task workspace: source_revision is not a SHA-256")
	}
	if err := w.Checkpoint.Validate(); err != nil {
		return fmt.Errorf("validate task workspace: checkpoint: %w", err)
	}
	if w.Checkpoint.CommitSHA == "" || w.Checkpoint.Sequence < 1 {
		return errors.New("validate task workspace: an initial checkpoint is required")
	}
	seen := map[string]struct{}{}
	for i, retained := range w.RetainedRefs {
		if err := retained.Validate(); err != nil {
			return fmt.Errorf("validate task workspace: retained_refs[%d]: %w", i, err)
		}
		if _, ok := seen[retained.Ref]; ok {
			return fmt.Errorf("validate task workspace: duplicate retained ref %q", retained.Ref)
		}
		seen[retained.Ref] = struct{}{}
	}
	if w.LastRecovery != nil {
		if err := w.LastRecovery.Validate(); err != nil {
			return fmt.Errorf("validate task workspace: last_recovery: %w", err)
		}
	}
	switch w.Status {
	case WorkspaceStatusPreparing, WorkspaceStatusReady, WorkspaceStatusRestored, WorkspaceStatusAmbiguous, WorkspaceStatusCleaned:
	default:
		return fmt.Errorf("validate task workspace: unknown status %q", w.Status)
	}
	if w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() || w.UpdatedAt.Before(w.CreatedAt) {
		return errors.New("validate task workspace: valid creation/update times are required")
	}
	return nil
}

func (c WorkspaceCheckpoint) Validate() error {
	if c.Sequence < 1 || !validGitOID(c.CommitSHA) || !validGitOID(c.TreeSHA) || !validStateSHA256(c.SourceRevision) || strings.TrimSpace(c.OperationID) == "" || strings.TrimSpace(c.Provenance) == "" || c.CreatedAt.IsZero() {
		return errors.New("checkpoint requires sequence, exact commit/tree/source, operation provenance, and time")
	}
	return nil
}

func (r WorkspaceRetainedRef) Validate() error {
	if !strings.HasPrefix(r.Ref, "refs/revolvr/retained/") || !validGitOID(r.CommitSHA) || !validGitOID(r.TreeSHA) || !validStateSHA256(r.SourceRevision) || strings.TrimSpace(r.Reason) == "" || strings.TrimSpace(r.OperationID) == "" || r.CreatedAt.IsZero() {
		return errors.New("retained ref requires a harness ref, exact identities, reason, operation, and time")
	}
	return nil
}

func (r WorkspaceRecovery) Validate() error {
	if strings.TrimSpace(r.OperationID) == "" || strings.TrimSpace(r.Kind) == "" || !validGitOID(r.FromCommitSHA) || !validGitOID(r.ToCommitSHA) || !validStateSHA256(r.SourceRevision) || r.CreatedAt.IsZero() {
		return errors.New("recovery requires operation, kind, exact commits/source, and time")
	}
	return nil
}

func validGitOID(value string) bool {
	return gitoid.Valid(value)
}

func pathWithin(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
