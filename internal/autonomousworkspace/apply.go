package autonomousworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
)

type ApplyConfig struct {
	TaskID        string
	OperationID   string
	Kind          autonomousstate.WorkspaceTransitionKind
	Expected      autonomousstate.ExpectedState
	PreviousState autonomous.ExecutionState
	Workspace     autonomous.TaskWorkspace
	CreatedAt     time.Time
	Store         *autonomousstate.Store
}

// Apply persists one already-observed Git workspace transition under the
// canonical per-task state lock. Git administration is completed before this
// call; immutable history is written before canonical state replacement.
func Apply(ctx context.Context, cfg ApplyConfig) (autonomousstate.WorkspaceCommitResult, error) {
	if cfg.Store == nil || cfg.TaskID == "" || cfg.OperationID == "" || cfg.CreatedAt.IsZero() {
		return autonomousstate.WorkspaceCommitResult{}, errors.New("apply workspace transition: task, operation, time, and store are required")
	}
	if err := cfg.PreviousState.Validate(); err != nil {
		return autonomousstate.WorkspaceCommitResult{}, err
	}
	if cfg.PreviousState.TaskID != cfg.TaskID || cfg.Workspace.TaskID != cfg.TaskID {
		return autonomousstate.WorkspaceCommitResult{}, errors.New("apply workspace transition: task identity mismatch")
	}
	next := cfg.PreviousState
	workspace := cfg.Workspace
	next.Workspace = &workspace
	if err := next.Validate(); err != nil {
		return autonomousstate.WorkspaceCommitResult{}, err
	}
	previousRaw, err := autonomousstate.MarshalState(cfg.PreviousState)
	if err != nil {
		return autonomousstate.WorkspaceCommitResult{}, err
	}
	nextRaw, err := autonomousstate.MarshalState(next)
	if err != nil {
		return autonomousstate.WorkspaceCommitResult{}, err
	}
	statePath := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", cfg.TaskID, "state.json"))
	previousIdentity := applyStateIdentity(statePath, previousRaw)
	resultingIdentity := applyStateIdentity(statePath, nextRaw)
	applicationRaw, _ := json.Marshal(struct {
		TaskID, OperationID string
		Kind                autonomousstate.WorkspaceTransitionKind
		Workspace           autonomous.TaskWorkspace
	}{cfg.TaskID, cfg.OperationID, cfg.Kind, workspace})
	applicationSum := sha256.Sum256(applicationRaw)
	history := autonomousstate.WorkspaceHistoryRecord{SchemaVersion: autonomousstate.WorkspaceHistorySchemaVersion, TaskID: cfg.TaskID, OperationID: cfg.OperationID, ApplicationSHA256: hex.EncodeToString(applicationSum[:]), Kind: cfg.Kind, CreatedAt: cfg.CreatedAt.UTC(), PreviousWorkspace: cfg.PreviousState.Workspace, ResultingWorkspace: workspace, PreviousState: previousIdentity, ResultingState: resultingIdentity}
	return cfg.Store.CommitWorkspace(ctx, autonomousstate.WorkspaceCommitRequest{TaskID: cfg.TaskID, Expected: cfg.Expected, PreviousState: cfg.PreviousState, NextState: next, History: history})
}

func applyStateIdentity(path string, raw []byte) autonomousstate.StateIdentity {
	sum := sha256.Sum256(raw)
	return autonomousstate.StateIdentity{Path: path, Persisted: true, SHA256: hex.EncodeToString(sum[:]), ByteSize: len(raw)}
}
