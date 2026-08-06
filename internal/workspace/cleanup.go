package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"revolvr/internal/storage/postgres"
)

type cleanupEffect struct {
	WorkspacePath            string `json:"workspace_path"`
	BranchRef                string `json:"branch_ref"`
	BranchCommit             string `json:"branch_commit"`
	OriginalIdentitySHA256   string `json:"original_identity_sha256"`
	WorktreeRegistrationGone bool   `json:"worktree_registration_gone"`
	WorkspacePathGone        bool   `json:"workspace_path_gone"`
}

func (m *Manager) cleanup(ctx context.Context, row postgres.CoreWorkspace, operationID string) (postgres.CoreWorkspace, error) {
	if row.Status == string(StatusCleaned) {
		if err := m.verifyCleaned(ctx, row, operationID); err != nil {
			return postgres.CoreWorkspace{}, err
		}
		return row, nil
	}
	if row.Status != string(StatusCompleted) && row.Status != string(StatusCancelled) && row.Status != string(StatusFailed) {
		return postgres.CoreWorkspace{}, fmt.Errorf("%w: cleanup requires terminal state, got %s", ErrIllegalTransition, row.Status)
	}
	branchCommit := row.SourceCommit
	if row.CandidateCommit.Valid {
		branchCommit = row.CandidateCommit.String
	}
	material := cleanupMaterial(row, branchCommit)
	operation, existed, err := m.ensureOperation(ctx, row, operationID, "worktree_cleanup", material)
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	registrations, err := m.worktreeRegistrations(ctx, row.ManagedRepositoryPath)
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	registration, exact, collision := findRegistration(registrations, row.WorkspacePath, row.BranchRef)
	if collision {
		return postgres.CoreWorkspace{}, &ConflictError{Effect: "worktree cleanup", Detail: "path or branch registration is divergent"}
	}
	info, statErr := os.Lstat(row.WorkspacePath)
	pathExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return postgres.CoreWorkspace{}, statErr
	}
	if !existed && row.WorkspaceDevice.Valid && (!exact || !pathExists) {
		return postgres.CoreWorkspace{}, &ConflictError{Effect: "worktree cleanup", Detail: "admitted worktree disappeared before cleanup operation admission"}
	}
	if operation.Status == "applied" && (exact || pathExists) {
		return postgres.CoreWorkspace{}, &ConflictError{Effect: "worktree cleanup", Detail: "cleaned path or registration reappeared"}
	}
	if exact {
		if registration.Head != branchCommit {
			return postgres.CoreWorkspace{}, &ConflictError{Effect: "worktree cleanup", Detail: "registered HEAD differs from retained branch authority"}
		}
		if !pathExists || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return postgres.CoreWorkspace{}, &ConflictError{Effect: "worktree cleanup", Detail: "registered path is not the admitted directory"}
		}
		directory, found, err := m.workspaceRoot.OpenDir(row.WorkspacePath, false)
		if err != nil || !found {
			return postgres.CoreWorkspace{}, errors.Join(err, &ConflictError{Effect: "worktree cleanup", Detail: "cannot bind admitted path"})
		}
		device, inode, identityErr := directory.Identity()
		_ = directory.Close()
		if identityErr != nil || (row.WorkspaceDevice.Valid && (uint64(row.WorkspaceDevice.Int64) != device || uint64(row.WorkspaceInode.Int64) != inode)) {
			return postgres.CoreWorkspace{}, errors.Join(identityErr, &ConflictError{Effect: "worktree cleanup", Detail: "path device/inode identity changed"})
		}
		if _, err := m.git(ctx, row.ManagedRepositoryPath, "worktree", "remove", "--force", "--", row.WorkspacePath); err != nil {
			return postgres.CoreWorkspace{}, err
		}
	} else if pathExists {
		return postgres.CoreWorkspace{}, &ConflictError{Effect: "worktree cleanup", Detail: "unregistered path exists; recursive deletion refused"}
	}
	effect, err := m.verifyCleanupEffect(ctx, row, branchCommit)
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	if _, err := m.completeOperation(ctx, operation, effect); err != nil {
		return postgres.CoreWorkspace{}, err
	}
	return m.markCleaned(ctx, row, operationID, effect)
}

func cleanupMaterial(row postgres.CoreWorkspace, branchCommit string) string {
	material, _ := stableHash(struct {
		WorkspaceID, Path, BranchRef, BranchCommit, TerminalStatus string
		Device, Inode                                              int64
	}{
		uuidString(row.ID), row.WorkspacePath, row.BranchRef, branchCommit, row.TerminalStatus.String,
		row.WorkspaceDevice.Int64, row.WorkspaceInode.Int64,
	})
	return material
}

func (m *Manager) verifyCleaned(ctx context.Context, row postgres.CoreWorkspace, operationID string) error {
	branchCommit := row.SourceCommit
	if row.CandidateCommit.Valid {
		branchCommit = row.CandidateCommit.String
	}
	operation, err := postgres.New(m.config.Pool).GetWorkspaceOperation(ctx, operationID)
	if err != nil || operation.WorkspaceID != row.ID || operation.OperationKind != "worktree_cleanup" ||
		operation.MaterialSha256 != cleanupMaterial(row, branchCommit) || operation.Status != "applied" {
		return errors.Join(err, &ConflictError{Effect: "worktree cleanup", Detail: "cleaned workspace has no matching applied operation"})
	}
	effect, err := m.verifyCleanupEffect(ctx, row, branchCommit)
	if err != nil {
		return err
	}
	return m.compareCleanupEffect(operation, effect)
}

func (m *Manager) verifyCleanupEffect(ctx context.Context, row postgres.CoreWorkspace, branchCommit string) (cleanupEffect, error) {
	registrations, err := m.worktreeRegistrations(ctx, row.ManagedRepositoryPath)
	if err != nil {
		return cleanupEffect{}, err
	}
	_, exact, collision := findRegistration(registrations, row.WorkspacePath, row.BranchRef)
	if exact || collision {
		return cleanupEffect{}, &ConflictError{Effect: "worktree cleanup", Detail: "Git registration remains after removal"}
	}
	if _, err := os.Lstat(row.WorkspacePath); !errors.Is(err, os.ErrNotExist) {
		return cleanupEffect{}, &ConflictError{Effect: "worktree cleanup", Detail: "workspace path remains after removal"}
	}
	branchOID, found, err := m.refOID(ctx, row.ManagedRepositoryPath, row.BranchRef)
	if err != nil || !found || branchOID != branchCommit {
		return cleanupEffect{}, errors.Join(err, &ConflictError{Effect: "worktree cleanup", Detail: "retained evidence branch changed"})
	}
	_, originalRaw, err := m.captureCheckoutIdentity(ctx, row.OriginalCheckoutPath)
	if err != nil || !sameCheckoutIdentity(originalRaw, row.OriginalIdentityBefore) {
		return cleanupEffect{}, errors.Join(err, &ConflictError{Effect: "original checkout", Detail: "identity changed before cleanup completion"})
	}
	return cleanupEffect{
		WorkspacePath: row.WorkspacePath, BranchRef: row.BranchRef, BranchCommit: branchCommit,
		OriginalIdentitySHA256: hexString(originalRaw), WorktreeRegistrationGone: true, WorkspacePathGone: true,
	}, nil
}

func (m *Manager) compareCleanupEffect(operation postgres.CoreWorkspaceOperation, effect cleanupEffect) error {
	var recorded cleanupEffect
	if err := json.Unmarshal(operation.Effect, &recorded); err != nil || recorded != effect {
		return errors.Join(err, &ConflictError{Effect: "worktree cleanup", Detail: "applied cleanup evidence differs"})
	}
	return nil
}

func (m *Manager) markCleaned(ctx context.Context, expected postgres.CoreWorkspace, operationID string, effect cleanupEffect) (postgres.CoreWorkspace, error) {
	var updated postgres.CoreWorkspace
	err := pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		current, err := queries.GetWorkspaceForUpdate(ctx, expected.ID)
		if err != nil {
			return err
		}
		if current.Status == string(StatusCleaned) {
			updated = current
			return nil
		}
		if current.Status != expected.Status || current.AggregateVersion != expected.AggregateVersion ||
			current.TerminalStatus.String != current.Status {
			return &ConflictError{Effect: "worktree cleanup", Detail: "canonical terminal state advanced"}
		}
		updated, err = queries.MarkWorkspaceCleaned(ctx, postgres.MarkWorkspaceCleanedParams{
			CleanupCompletedAt: timestamp(m.now()), WorkspaceID: current.ID,
			ExpectedTerminalStatus: current.Status, ExpectedAggregateVersion: current.AggregateVersion,
		})
		if err != nil {
			return err
		}
		detailRaw, _ := json.Marshal(effect)
		return appendWorkspaceEvent(ctx, queries, updated, "workspace.cleaned", operationID, map[string]any{"effect": json.RawMessage(detailRaw)})
	})
	if err != nil {
		return postgres.CoreWorkspace{}, fmt.Errorf("mark workspace cleaned: %w", err)
	}
	return updated, nil
}
