package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"revolvr/internal/storage/postgres"
)

func (m *Manager) admitWorkspace(ctx context.Context, params postgres.InsertWorkspaceParams) (postgres.CoreWorkspace, bool, error) {
	queries := postgres.New(m.config.Pool)
	existing, err := queries.GetWorkspace(ctx, params.ID)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return postgres.CoreWorkspace{}, false, fmt.Errorf("workspace admission: %w", err)
	}
	var created postgres.CoreWorkspace
	err = pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		var insertErr error
		created, insertErr = qtx.InsertWorkspace(ctx, params)
		if insertErr != nil {
			return insertErr
		}
		return appendWorkspaceEvent(ctx, qtx, created, "workspace.planned", params.CreationOperationID, map[string]any{
			"symbolic_source_id": created.SymbolicSourceID,
			"source_commit":      created.SourceCommit, "source_tree": created.SourceTree,
			"managed_repository_path": created.ManagedRepositoryPath,
			"workspace_path":          created.WorkspacePath, "branch_ref": created.BranchRef,
		})
	})
	if err == nil {
		return created, false, nil
	}
	if uniqueViolation(err) {
		if existing, getErr := queries.GetWorkspace(ctx, params.ID); getErr == nil {
			return existing, true, nil
		}
		if existing, getErr := queries.GetWorkspaceByRunID(ctx, params.RunID); getErr == nil {
			return existing, true, nil
		}
		return postgres.CoreWorkspace{}, false, &ConflictError{Effect: "workspace admission", Detail: "a workspace, run, operation, path, branch, or symbolic identity is already owned"}
	}
	return postgres.CoreWorkspace{}, false, fmt.Errorf("workspace admission: %w", err)
}

func sameCreation(row postgres.CoreWorkspace, request CreateRequest, authority postgres.GetWorkspaceRunAuthorityRow, workspacePath, branchRef string, before []byte) error {
	if uuidString(row.ID) != request.WorkspaceID || uuidString(row.RunID) != request.RunID ||
		row.ProjectID != authority.ProjectID || row.ProjectSourceID != authority.ProjectSourceID || row.TaskID != authority.TaskID ||
		row.CreationOperationID != request.OperationID || row.SymbolicSourceID != request.SymbolicSourceID ||
		row.OriginalCheckoutPath != authority.CanonicalSourcePath || row.ManagedRepositoryPath != authority.ManagedRepositoryPath ||
		row.WorkspaceRoot == "" || row.WorkspacePath != workspacePath || row.BranchRef != branchRef ||
		row.SourceCommit != authority.SourceCommit || row.SourceTree != authority.SourceTree ||
		!sameCheckoutIdentity(row.OriginalIdentityBefore, before) {
		return &ConflictError{Effect: "workspace admission", Detail: "existing canonical material differs from the requested run"}
	}
	return nil
}

func (m *Manager) advanceStatus(ctx context.Context, expected postgres.CoreWorkspace, next Status, eventType, operationID string, detail map[string]any) (postgres.CoreWorkspace, error) {
	var updated postgres.CoreWorkspace
	err := pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		current, err := queries.GetWorkspaceForUpdate(ctx, expected.ID)
		if err != nil {
			return err
		}
		if current.Status == string(next) {
			updated = current
			return nil
		}
		if current.Status != expected.Status || current.AggregateVersion != expected.AggregateVersion {
			return &ConflictError{Effect: "workspace transition", Detail: "canonical state advanced from the expected version"}
		}
		updated, err = queries.AdvanceWorkspaceStatus(ctx, postgres.AdvanceWorkspaceStatusParams{
			NewStatus: string(next), UpdatedAt: timestamp(m.now()), WorkspaceID: current.ID,
			ExpectedStatus: current.Status, ExpectedAggregateVersion: current.AggregateVersion,
		})
		if err != nil {
			return err
		}
		return appendWorkspaceEvent(ctx, queries, updated, eventType, operationID, detail)
	})
	if err != nil {
		return postgres.CoreWorkspace{}, fmt.Errorf("workspace transition %s -> %s: %w", expected.Status, next, err)
	}
	return updated, nil
}

func (m *Manager) markReady(ctx context.Context, expected postgres.CoreWorkspace, device, inode uint64, after []byte, operationID string) (postgres.CoreWorkspace, error) {
	if device == 0 || inode == 0 || device > uint64(^uint64(0)>>1) || inode > uint64(^uint64(0)>>1) {
		return postgres.CoreWorkspace{}, fmt.Errorf("%w: workspace filesystem identity is invalid", ErrUnsafePath)
	}
	var updated postgres.CoreWorkspace
	err := pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		current, err := queries.GetWorkspaceForUpdate(ctx, expected.ID)
		if err != nil {
			return err
		}
		if current.Status == string(StatusReady) {
			if !current.WorkspaceDevice.Valid || !current.WorkspaceInode.Valid ||
				uint64(current.WorkspaceDevice.Int64) != device || uint64(current.WorkspaceInode.Int64) != inode ||
				!sameCheckoutIdentity(current.OriginalIdentityAfter, after) {
				return &ConflictError{Effect: "workspace ready", Detail: "recorded filesystem or checkout identity differs"}
			}
			updated = current
			return nil
		}
		if current.Status != string(StatusCreating) || current.AggregateVersion != expected.AggregateVersion {
			return &ConflictError{Effect: "workspace ready", Detail: "canonical state is not the expected creating version"}
		}
		updated, err = queries.MarkWorkspaceReady(ctx, postgres.MarkWorkspaceReadyParams{
			WorkspaceDevice:       pgtype.Int8{Int64: int64(device), Valid: true},
			WorkspaceInode:        pgtype.Int8{Int64: int64(inode), Valid: true},
			OriginalIdentityAfter: after, UpdatedAt: timestamp(m.now()),
			WorkspaceID: current.ID, ExpectedAggregateVersion: current.AggregateVersion,
		})
		if err != nil {
			return err
		}
		return appendWorkspaceEvent(ctx, queries, updated, "workspace.ready", operationID, map[string]any{
			"workspace_device": device, "workspace_inode": inode,
			"original_checkout_unchanged": true,
		})
	})
	if err != nil {
		return postgres.CoreWorkspace{}, fmt.Errorf("mark workspace ready: %w", err)
	}
	return updated, nil
}

func (m *Manager) markTerminal(ctx context.Context, expected postgres.CoreWorkspace, terminal Status, reason, operationID string) (postgres.CoreWorkspace, error) {
	if terminal != StatusCancelled && terminal != StatusFailed {
		return postgres.CoreWorkspace{}, fmt.Errorf("%w: unsupported terminal status %q", ErrIllegalTransition, terminal)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 4096 {
		return postgres.CoreWorkspace{}, errors.New("workspace terminal reason is empty or oversized")
	}
	var updated postgres.CoreWorkspace
	err := pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		current, err := queries.GetWorkspaceForUpdate(ctx, expected.ID)
		if err != nil {
			return err
		}
		if current.Status == string(terminal) {
			if current.TerminalReason.String != reason {
				return &ConflictError{Effect: "workspace terminal transition", Detail: "terminal reason differs"}
			}
			updated = current
			return nil
		}
		if current.Status == string(StatusCleaned) && current.TerminalStatus.String == string(terminal) {
			updated = current
			return nil
		}
		if current.Status != expected.Status || current.AggregateVersion != expected.AggregateVersion ||
			current.Status == string(StatusCompleted) {
			return &ConflictError{Effect: "workspace terminal transition", Detail: "canonical state is not the expected non-completed version"}
		}
		updated, err = queries.MarkWorkspaceTerminal(ctx, postgres.MarkWorkspaceTerminalParams{
			TerminalStatus: string(terminal), TerminalReason: textValue(reason), UpdatedAt: timestamp(m.now()),
			WorkspaceID: current.ID, ExpectedStatus: current.Status, ExpectedAggregateVersion: current.AggregateVersion,
		})
		if err != nil {
			return err
		}
		return appendWorkspaceEvent(ctx, queries, updated, "workspace."+string(terminal), operationID, map[string]any{"reason": reason})
	})
	if err != nil {
		return postgres.CoreWorkspace{}, fmt.Errorf("mark workspace %s: %w", terminal, err)
	}
	return updated, nil
}

func appendWorkspaceEvent(ctx context.Context, queries *postgres.Queries, workspace postgres.CoreWorkspace, eventType, operationID string, detail map[string]any) error {
	payload, err := json.Marshal(struct {
		WorkspaceID string         `json:"workspace_id"`
		RunID       string         `json:"run_id"`
		OperationID string         `json:"operation_id"`
		Status      string         `json:"status"`
		Detail      map[string]any `json:"detail,omitempty"`
	}{uuidString(workspace.ID), uuidString(workspace.RunID), operationID, workspace.Status, detail})
	if err != nil {
		return err
	}
	_, err = queries.AppendEvent(ctx, postgres.AppendEventParams{
		ID: newUUID(), ProjectID: workspace.ProjectID, TaskID: workspace.TaskID, RunID: workspace.RunID,
		EventType: eventType, AggregateType: "workspace", AggregateID: workspace.ID,
		AggregateVersion: workspace.AggregateVersion, Payload: payload, CreatedAt: workspace.UpdatedAt,
	})
	return err
}

func (m *Manager) ensureOperation(ctx context.Context, workspace postgres.CoreWorkspace, operationID, kind, material string) (postgres.CoreWorkspaceOperation, bool, error) {
	if err := validateOperationID(operationID, 512); err != nil {
		return postgres.CoreWorkspaceOperation{}, false, err
	}
	queries := postgres.New(m.config.Pool)
	existing, err := queries.GetWorkspaceOperation(ctx, operationID)
	if err == nil {
		if existing.WorkspaceID != workspace.ID || existing.OperationKind != kind || existing.MaterialSha256 != material {
			return postgres.CoreWorkspaceOperation{}, true, &ConflictError{Effect: kind, Detail: "operation id was reused for different material"}
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return postgres.CoreWorkspaceOperation{}, false, err
	}
	created, err := queries.InsertWorkspaceOperation(ctx, postgres.InsertWorkspaceOperationParams{
		OperationID: operationID, WorkspaceID: workspace.ID, OperationKind: kind,
		MaterialSha256: material, CreatedAt: timestamp(m.now()),
	})
	if err == nil {
		return created, false, nil
	}
	if uniqueViolation(err) {
		existing, getErr := queries.GetWorkspaceOperation(ctx, operationID)
		if getErr == nil && existing.WorkspaceID == workspace.ID && existing.OperationKind == kind && existing.MaterialSha256 == material {
			return existing, true, nil
		}
		return postgres.CoreWorkspaceOperation{}, true, &ConflictError{Effect: kind, Detail: "operation id collided"}
	}
	return postgres.CoreWorkspaceOperation{}, false, err
}

func (m *Manager) completeOperation(ctx context.Context, operation postgres.CoreWorkspaceOperation, effect any) (postgres.CoreWorkspaceOperation, error) {
	raw, err := json.Marshal(effect)
	if err != nil {
		return postgres.CoreWorkspaceOperation{}, err
	}
	if operation.Status == "applied" {
		if !sameJSON(operation.Effect, raw) {
			return postgres.CoreWorkspaceOperation{}, &ConflictError{Effect: operation.OperationKind, Detail: "applied effect identity differs"}
		}
		return operation, nil
	}
	completed, err := postgres.New(m.config.Pool).CompleteWorkspaceOperation(ctx, postgres.CompleteWorkspaceOperationParams{
		Effect: raw, AppliedAt: timestamp(m.now()), OperationID: operation.OperationID,
		WorkspaceID: operation.WorkspaceID, OperationKind: operation.OperationKind,
		MaterialSha256: operation.MaterialSha256,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := postgres.New(m.config.Pool).GetWorkspaceOperation(ctx, operation.OperationID)
		if getErr == nil && existing.Status == "applied" && sameJSON(existing.Effect, raw) {
			return existing, nil
		}
		return postgres.CoreWorkspaceOperation{}, &ConflictError{Effect: operation.OperationKind, Detail: "operation changed before completion"}
	}
	return completed, err
}

func (m *Manager) workspaceFromRow(ctx context.Context, row postgres.CoreWorkspace) (Workspace, error) {
	var before CheckoutIdentity
	if err := json.Unmarshal(row.OriginalIdentityBefore, &before); err != nil {
		return Workspace{}, fmt.Errorf("decode original checkout identity: %w", err)
	}
	var after *CheckoutIdentity
	if len(row.OriginalIdentityAfter) != 0 {
		var value CheckoutIdentity
		if err := json.Unmarshal(row.OriginalIdentityAfter, &value); err != nil {
			return Workspace{}, fmt.Errorf("decode final original checkout identity: %w", err)
		}
		after = &value
	}
	var manifest []Change
	if len(row.ChangedManifest) != 0 {
		if err := json.Unmarshal(row.ChangedManifest, &manifest); err != nil {
			return Workspace{}, fmt.Errorf("decode workspace changed manifest: %w", err)
		}
	}
	workspace := Workspace{
		ID: uuidString(row.ID), RunID: uuidString(row.RunID), ProjectID: uuidString(row.ProjectID),
		ProjectSourceID: uuidString(row.ProjectSourceID), TaskID: uuidString(row.TaskID),
		CreationOperationID: row.CreationOperationID, SymbolicSourceID: row.SymbolicSourceID,
		Status: Status(row.Status), AggregateVersion: row.AggregateVersion,
		OriginalCheckoutPath: row.OriginalCheckoutPath, ManagedRepositoryPath: row.ManagedRepositoryPath,
		WorkspaceRoot: row.WorkspaceRoot, Path: row.WorkspacePath, BranchRef: row.BranchRef,
		SourceCommit: row.SourceCommit, SourceTree: row.SourceTree,
		OriginalBefore: before, OriginalAfter: after, GitStatus: append([]byte(nil), row.GitStatus...),
		ChangedManifest: manifest, DiffArtifactID: uuidString(row.DiffArtifactID), DiffSHA256: row.DiffSha256.String,
		CandidateCommit: row.CandidateCommit.String, CandidateTree: row.CandidateTree.String,
		TerminalReason: row.TerminalReason.String, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.TerminalStatus.Valid {
		workspace.TerminalStatus = Status(row.TerminalStatus.String)
	}
	if row.WorkspaceDevice.Valid && row.WorkspaceDevice.Int64 > 0 {
		workspace.Device = uint64(row.WorkspaceDevice.Int64)
	}
	if row.WorkspaceInode.Valid && row.WorkspaceInode.Int64 > 0 {
		workspace.Inode = uint64(row.WorkspaceInode.Int64)
	}
	if row.CleanupCompletedAt.Valid {
		value := row.CleanupCompletedAt.Time
		workspace.CleanupCompletedAt = &value
	}
	if row.DiffSha256.Valid {
		artifact, err := postgres.New(m.config.Pool).GetArtifactBySHA256(ctx, row.DiffSha256.String)
		if err != nil || artifact.ID != row.DiffArtifactID {
			return Workspace{}, errors.Join(err, &ConflictError{Effect: "diff artifact", Detail: "canonical artifact metadata is missing or mismatched"})
		}
		workspace.DiffArtifactPath = artifact.StoragePath
	}
	return workspace, nil
}
