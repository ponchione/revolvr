package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"revolvr/internal/storage/postgres"
)

type candidateEffect struct {
	Commit      string `json:"commit"`
	Tree        string `json:"tree"`
	Parent      string `json:"parent"`
	BranchRef   string `json:"branch_ref"`
	DiffSHA256  string `json:"diff_sha256"`
	OperationID string `json:"operation_id"`
}

func (m *Manager) createCandidate(ctx context.Context, row postgres.CoreWorkspace, request CommitRequest) (string, string, error) {
	material, err := candidateOperationMaterial(row, request)
	if err != nil {
		return "", "", err
	}
	operation, existed, err := m.ensureOperation(ctx, row, request.OperationID+":commit", "commit", material)
	if err != nil {
		return "", "", err
	}
	head, found, err := m.refOID(ctx, row.ManagedRepositoryPath, row.BranchRef)
	if err != nil || !found {
		return "", "", errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "workspace branch is missing"})
	}
	if !existed && head != row.SourceCommit {
		return "", "", &ConflictError{Effect: "candidate commit", Detail: "branch advanced before commit operation admission"}
	}
	if operation.Status == "applied" {
		var effect candidateEffect
		if err := json.Unmarshal(operation.Effect, &effect); err != nil {
			return "", "", err
		}
		verified, err := m.verifyCandidate(ctx, row, request, effect.Commit)
		if err != nil || verified != effect {
			return "", "", errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "applied commit effect is divergent"})
		}
		return effect.Commit, effect.Tree, nil
	}
	if head != row.SourceCommit {
		effect, err := m.verifyCandidate(ctx, row, request, head)
		if err != nil {
			return "", "", err
		}
		if err := m.inject(FailureAfterCommit); err != nil {
			return "", "", err
		}
		return effect.Commit, effect.Tree, nil
	}

	subject, body := candidateMessage(row, request)
	result := m.runGit(ctx, row.WorkspacePath, []string{
		"commit", "--no-verify", "--no-gpg-sign", "-m", subject, "-m", body,
	}, nil)
	postHead, postFound, inspectErr := m.refOID(context.WithoutCancel(ctx), row.ManagedRepositoryPath, row.BranchRef)
	if inspectErr != nil || !postFound {
		return "", "", errors.Join(gitResultError(result), inspectErr, &ConflictError{Effect: "candidate commit", Detail: "post-commit branch outcome is indeterminate"})
	}
	if postHead == row.SourceCommit {
		return "", "", gitResultError(result)
	}
	effect, err := m.verifyCandidate(context.WithoutCancel(ctx), row, request, postHead)
	if err != nil {
		return "", "", err
	}
	if err := m.inject(FailureAfterCommit); err != nil {
		return "", "", err
	}
	return effect.Commit, effect.Tree, nil
}

func candidateOperationMaterial(row postgres.CoreWorkspace, request CommitRequest) (string, error) {
	return stableHash(struct {
		WorkspaceID, RunID, TaskID, OperationID, Summary string
		SourceCommit, SourceTree, BranchRef, DiffSHA256  string
	}{
		uuidString(row.ID), uuidString(row.RunID), uuidString(row.TaskID), request.OperationID,
		request.Summary, row.SourceCommit, row.SourceTree, row.BranchRef, row.DiffSha256.String,
	})
}

func candidateMessage(row postgres.CoreWorkspace, request CommitRequest) (string, string) {
	return request.Summary, strings.Join([]string{
		"Run-ID: " + uuidString(row.RunID),
		"Task-ID: " + uuidString(row.TaskID),
		"Workspace-ID: " + uuidString(row.ID),
		"Workspace-Operation: " + request.OperationID,
	}, "\n")
}

func (m *Manager) verifyCandidate(ctx context.Context, row postgres.CoreWorkspace, request CommitRequest, commit string) (candidateEffect, error) {
	parents, err := m.git(ctx, row.ManagedRepositoryPath, "rev-list", "--parents", "-n", "1", commit)
	if err != nil {
		return candidateEffect{}, err
	}
	fields := strings.Fields(parents)
	if len(fields) != 2 || fields[0] != commit || fields[1] != row.SourceCommit {
		return candidateEffect{}, &ConflictError{Effect: "candidate commit", Detail: "commit parent is not the pinned source"}
	}
	tree, err := m.git(ctx, row.ManagedRepositoryPath, "rev-parse", "--verify", commit+"^{tree}")
	if err != nil {
		return candidateEffect{}, err
	}
	tree = strings.TrimSpace(tree)
	message, err := m.git(ctx, row.ManagedRepositoryPath, "show", "-s", "--format=%B", commit)
	if err != nil {
		return candidateEffect{}, err
	}
	subject, body := candidateMessage(row, request)
	if strings.TrimRight(message, "\n") != subject+"\n\n"+body {
		return candidateEffect{}, &ConflictError{Effect: "candidate commit", Detail: "commit message does not bind the exact operation"}
	}
	diff, err := m.git(ctx, row.ManagedRepositoryPath, "diff", "--binary", "--full-index", "--no-ext-diff", row.SourceCommit, commit, "--")
	if err != nil || hexString([]byte(diff)) != row.DiffSha256.String {
		return candidateEffect{}, errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "commit diff does not match captured artifact"})
	}
	pathsRaw, err := m.git(ctx, row.ManagedRepositoryPath, "diff", "--name-only", "--no-renames", "-z", row.SourceCommit, commit, "--")
	if err != nil {
		return candidateEffect{}, err
	}
	actualPaths := splitNUL(pathsRaw)
	var manifest []Change
	if err := json.Unmarshal(row.ChangedManifest, &manifest); err != nil {
		return candidateEffect{}, err
	}
	wantPaths := manifestPaths(manifest)
	sort.Strings(actualPaths)
	if !equalStrings(actualPaths, wantPaths) {
		return candidateEffect{}, &ConflictError{Effect: "candidate commit", Detail: "committed paths differ from the host manifest"}
	}
	branchHead, found, err := m.refOID(ctx, row.ManagedRepositoryPath, row.BranchRef)
	if err != nil || !found || branchHead != commit {
		return candidateEffect{}, errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "workspace branch does not name candidate"})
	}
	status, err := m.git(ctx, row.WorkspacePath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil || status != "" {
		return candidateEffect{}, errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "worktree is not clean after commit"})
	}
	return candidateEffect{
		Commit: commit, Tree: tree, Parent: row.SourceCommit, BranchRef: row.BranchRef,
		DiffSHA256: row.DiffSha256.String, OperationID: request.OperationID,
	}, nil
}

func (m *Manager) persistCandidate(ctx context.Context, expected postgres.CoreWorkspace, request CommitRequest, commit, tree string) (postgres.CoreWorkspace, error) {
	material, _ := candidateOperationMaterial(expected, request)
	operation, err := postgres.New(m.config.Pool).GetWorkspaceOperation(ctx, request.OperationID+":commit")
	if err != nil || operation.WorkspaceID != expected.ID || operation.OperationKind != "commit" || operation.MaterialSha256 != material {
		return postgres.CoreWorkspace{}, errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "operation journal identity differs"})
	}
	effect, err := m.verifyCandidate(ctx, expected, request, commit)
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	effectRaw, _ := json.Marshal(effect)
	var updated postgres.CoreWorkspace
	err = pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		current, err := queries.GetWorkspaceForUpdate(ctx, expected.ID)
		if err != nil {
			return err
		}
		if current.Status == string(StatusCompleted) {
			if current.CandidateCommit.String != commit || current.CandidateTree.String != tree {
				return &ConflictError{Effect: "candidate commit", Detail: "canonical candidate differs"}
			}
			updated = current
			return nil
		}
		if current.Status != string(StatusReconciling) || current.AggregateVersion != expected.AggregateVersion {
			return &ConflictError{Effect: "candidate commit", Detail: "canonical state is not the expected reconciling version"}
		}
		if operation.Status == "applied" {
			if !sameJSON(operation.Effect, effectRaw) {
				return &ConflictError{Effect: "candidate commit", Detail: "applied operation effect differs"}
			}
		} else if _, err := queries.CompleteWorkspaceOperation(ctx, postgres.CompleteWorkspaceOperationParams{
			Effect: effectRaw, AppliedAt: timestamp(m.now()), OperationID: operation.OperationID,
			WorkspaceID: operation.WorkspaceID, OperationKind: operation.OperationKind, MaterialSha256: operation.MaterialSha256,
		}); err != nil {
			return err
		}
		updated, err = queries.RecordWorkspaceCandidate(ctx, postgres.RecordWorkspaceCandidateParams{
			CandidateCommit: textValue(commit), CandidateTree: textValue(tree), UpdatedAt: timestamp(m.now()),
			WorkspaceID: current.ID, ExpectedAggregateVersion: current.AggregateVersion,
		})
		if err != nil {
			return err
		}
		return appendWorkspaceEvent(ctx, queries, updated, "workspace.completed", operation.OperationID, map[string]any{
			"candidate_commit": commit, "candidate_tree": tree, "diff_sha256": current.DiffSha256.String,
		})
	})
	if err != nil {
		return postgres.CoreWorkspace{}, fmt.Errorf("persist candidate commit: %w", err)
	}
	return updated, nil
}

func (m *Manager) commitEvidence(ctx context.Context, row postgres.CoreWorkspace) (CommitEvidence, error) {
	workspace, err := m.workspaceFromRow(ctx, row)
	if err != nil {
		return CommitEvidence{}, err
	}
	return CommitEvidence{
		Workspace: workspace, GitStatus: workspace.GitStatus,
		ChangedManifest: append([]Change(nil), workspace.ChangedManifest...),
		DiffArtifactID:  workspace.DiffArtifactID, DiffArtifactPath: workspace.DiffArtifactPath,
		DiffSHA256: workspace.DiffSHA256, CandidateCommit: workspace.CandidateCommit,
		CandidateTree: workspace.CandidateTree,
	}, nil
}

func splitNUL(raw string) []string {
	fields := strings.Split(raw, "\x00")
	if len(fields) != 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	return fields
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
