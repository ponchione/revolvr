package workspace

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
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"revolvr/internal/gitstate"
	"revolvr/internal/storage/postgres"
)

func (m *Manager) Get(ctx context.Context, workspaceID string) (Workspace, error) {
	id, err := parseUUID("workspace id", workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	row, err := postgres.New(m.config.Pool).GetWorkspace(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	return m.workspaceFromRow(ctx, row)
}

func (m *Manager) Activate(ctx context.Context, request TransitionRequest) (Workspace, error) {
	return m.simpleTransition(ctx, request, StatusReady, StatusActive, "workspace.active")
}

func (m *Manager) Freeze(ctx context.Context, request TransitionRequest) (Workspace, error) {
	return m.simpleTransition(ctx, request, StatusActive, StatusFrozen, "workspace.frozen")
}

func (m *Manager) simpleTransition(ctx context.Context, request TransitionRequest, expected, next Status, eventType string) (Workspace, error) {
	if err := validateOperationID(request.OperationID, 512); err != nil {
		return Workspace{}, err
	}
	id, err := parseUUID("workspace id", request.WorkspaceID)
	if err != nil {
		return Workspace{}, err
	}
	row, err := postgres.New(m.config.Pool).GetWorkspace(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	if row.Status == string(next) {
		workspace, err := m.workspaceFromRow(ctx, row)
		workspace.Replayed = true
		return workspace, err
	}
	if row.Status != string(expected) {
		return Workspace{}, fmt.Errorf("%w: cannot move %s to %s", ErrIllegalTransition, row.Status, next)
	}
	if err := m.revalidateWorkspace(ctx, row); err != nil {
		return Workspace{}, err
	}
	row, err = m.advanceStatus(ctx, row, next, eventType, request.OperationID, nil)
	if err != nil {
		return Workspace{}, err
	}
	return m.workspaceFromRow(ctx, row)
}

// Commit captures host-observed status, an ordered changed-file manifest, and
// a content-addressed binary diff before creating one operation-identified
// candidate commit. Completion immediately cleans only the admitted worktree.
func (m *Manager) Commit(ctx context.Context, request CommitRequest) (CommitEvidence, error) {
	if err := validateOperationID(request.OperationID, 470); err != nil {
		return CommitEvidence{}, err
	}
	request.Summary = strings.TrimSpace(request.Summary)
	if request.Summary == "" || len(request.Summary) > 1024 || strings.ContainsAny(request.Summary, "\r\n") {
		return CommitEvidence{}, errors.New("workspace commit: summary is empty, multiline, or oversized")
	}
	id, err := parseUUID("workspace id", request.WorkspaceID)
	if err != nil {
		return CommitEvidence{}, err
	}
	row, err := postgres.New(m.config.Pool).GetWorkspace(ctx, id)
	if err != nil {
		return CommitEvidence{}, err
	}
	if row.Status == string(StatusCleaned) || row.Status == string(StatusCompleted) {
		return m.replayCommit(ctx, row, request)
	}
	if row.Status != string(StatusFrozen) && row.Status != string(StatusReconciling) {
		return CommitEvidence{}, fmt.Errorf("%w: commit requires frozen or reconciling, got %s", ErrIllegalTransition, row.Status)
	}
	if err := m.revalidateOriginal(ctx, row); err != nil {
		return CommitEvidence{}, err
	}
	if row.Status == string(StatusFrozen) {
		row, err = m.captureForCommit(ctx, row, request.OperationID+":capture")
		if err != nil {
			return m.failCommit(row, request.OperationID, err)
		}
	}
	_, recoveryErr := postgres.New(m.config.Pool).GetWorkspaceOperation(ctx, request.OperationID+":commit")
	recoveringCommit := recoveryErr == nil
	commit, tree, err := m.createCandidate(ctx, row, request)
	if err != nil {
		if errors.Is(err, ErrInjectedCrash) {
			return CommitEvidence{}, err
		}
		return m.failCommit(row, request.OperationID, err)
	}
	row, err = m.persistCandidate(ctx, row, request, commit, tree)
	if err != nil {
		return CommitEvidence{}, err
	}
	evidence, err := m.commitEvidence(ctx, row)
	if err != nil {
		return CommitEvidence{}, err
	}
	cleaned, cleanupErr := m.cleanup(ctx, row, request.OperationID+":cleanup")
	if cleanupErr != nil {
		return evidence, errors.Join(ErrCleanupFailed, cleanupErr)
	}
	evidence.Workspace, err = m.workspaceFromRow(ctx, cleaned)
	evidence.Workspace.Replayed = recoveringCommit
	return evidence, err
}

func (m *Manager) replayCommit(ctx context.Context, row postgres.CoreWorkspace, request CommitRequest) (CommitEvidence, error) {
	operation, err := postgres.New(m.config.Pool).GetWorkspaceOperation(ctx, request.OperationID+":commit")
	if err != nil || operation.WorkspaceID != row.ID || operation.OperationKind != "commit" || operation.Status != "applied" {
		return CommitEvidence{}, errors.Join(err, &ConflictError{Effect: "candidate commit", Detail: "completion has no matching applied operation"})
	}
	evidence, err := m.commitEvidence(ctx, row)
	if err != nil {
		return CommitEvidence{}, err
	}
	cleaned, cleanupErr := m.cleanup(ctx, row, request.OperationID+":cleanup")
	if cleanupErr != nil {
		return evidence, errors.Join(ErrCleanupFailed, cleanupErr)
	}
	evidence.Workspace, err = m.workspaceFromRow(ctx, cleaned)
	evidence.Workspace.Replayed = true
	return evidence, err
}

func (m *Manager) Cancel(ctx context.Context, request TransitionRequest) (Workspace, error) {
	return m.terminate(ctx, request, StatusCancelled)
}

func (m *Manager) Fail(ctx context.Context, request TransitionRequest) (Workspace, error) {
	return m.terminate(ctx, request, StatusFailed)
}

func (m *Manager) terminate(ctx context.Context, request TransitionRequest, terminal Status) (Workspace, error) {
	if err := validateOperationID(request.OperationID, 470); err != nil {
		return Workspace{}, err
	}
	id, err := parseUUID("workspace id", request.WorkspaceID)
	if err != nil {
		return Workspace{}, err
	}
	internal, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	row, err := postgres.New(m.config.Pool).GetWorkspace(internal, id)
	if err != nil {
		return Workspace{}, err
	}
	if row.Status == string(StatusCleaned) {
		if row.TerminalStatus.String != string(terminal) {
			return Workspace{}, &ConflictError{Effect: "workspace termination", Detail: "cleaned workspace has a different terminal outcome"}
		}
		if row.TerminalReason.String != strings.TrimSpace(request.Reason) {
			return Workspace{}, &ConflictError{Effect: "workspace termination", Detail: "cleaned workspace has a different terminal reason"}
		}
		row, err = m.cleanup(internal, row, request.OperationID+":cleanup")
		if err != nil {
			return Workspace{}, errors.Join(ErrCleanupFailed, err)
		}
		workspace, err := m.workspaceFromRow(internal, row)
		workspace.Replayed = true
		return workspace, err
	}
	var evidenceErr error
	if row.Status == string(StatusActive) {
		var updated postgres.CoreWorkspace
		updated, evidenceErr = m.advanceStatus(internal, row, StatusFrozen, "workspace.frozen", request.OperationID+":freeze", map[string]any{"terminal_capture": true})
		if evidenceErr == nil {
			row = updated
		}
	}
	if evidenceErr == nil && row.Status == string(StatusFrozen) {
		var captured postgres.CoreWorkspace
		captured, evidenceErr = m.captureForCommit(internal, row, request.OperationID+":capture")
		if errors.Is(evidenceErr, ErrNoChanges) {
			evidenceErr = nil
		} else if evidenceErr == nil {
			row = captured
		}
	}
	if row.Status != string(terminal) {
		row, err = m.markTerminal(internal, row, terminal, request.Reason, request.OperationID+":terminal")
		if err != nil {
			return Workspace{}, errors.Join(evidenceErr, err)
		}
	}
	cleaned, err := m.cleanup(internal, row, request.OperationID+":cleanup")
	if err != nil {
		workspace, conversionErr := m.workspaceFromRow(internal, row)
		return workspace, errors.Join(evidenceErr, ErrCleanupFailed, err, conversionErr)
	}
	workspace, conversionErr := m.workspaceFromRow(internal, cleaned)
	return workspace, errors.Join(evidenceErr, conversionErr)
}

func (m *Manager) Cleanup(ctx context.Context, request TransitionRequest) (Workspace, error) {
	id, err := parseUUID("workspace id", request.WorkspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if err := validateOperationID(request.OperationID, 500); err != nil {
		return Workspace{}, err
	}
	row, err := postgres.New(m.config.Pool).GetWorkspace(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	cleaned, err := m.cleanup(ctx, row, request.OperationID)
	if err != nil {
		return Workspace{}, errors.Join(ErrCleanupFailed, err)
	}
	return m.workspaceFromRow(ctx, cleaned)
}

func (m *Manager) failCommit(row postgres.CoreWorkspace, operationID string, cause error) (CommitEvidence, error) {
	internal, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	current, err := postgres.New(m.config.Pool).GetWorkspace(internal, row.ID)
	if err == nil && current.Status != string(StatusFailed) && current.Status != string(StatusCleaned) {
		current, err = m.markTerminal(internal, current, StatusFailed, cause.Error(), operationID+":failed")
	}
	if err == nil && current.Status == string(StatusFailed) {
		_, err = m.cleanup(internal, current, operationID+":cleanup")
	}
	return CommitEvidence{}, errors.Join(cause, err)
}

func (m *Manager) revalidateOriginal(ctx context.Context, row postgres.CoreWorkspace) error {
	_, raw, err := m.captureCheckoutIdentity(ctx, row.OriginalCheckoutPath)
	if err != nil {
		return err
	}
	if !sameCheckoutIdentity(raw, row.OriginalIdentityBefore) {
		return &ConflictError{Effect: "original checkout", Detail: "source or filesystem identity changed after workspace admission"}
	}
	return nil
}

func (m *Manager) captureForCommit(ctx context.Context, row postgres.CoreWorkspace, operationID string) (postgres.CoreWorkspace, error) {
	if err := m.revalidateWorkspace(ctx, row); err != nil {
		return postgres.CoreWorkspace{}, err
	}
	capture, err := gitstate.CaptureChangedFiles(ctx, gitstate.Config{
		WorkingDir: row.WorkspacePath, GitExecutable: m.config.GitExecutable,
		Timeout: m.config.Timeout, StdoutCap: m.config.StdoutCap, StderrCap: m.config.StderrCap,
		CommandRunner: gitstate.CommandRunner(m.safeCommandRunner),
	})
	if err != nil || capture.CaptureError != "" {
		return postgres.CoreWorkspace{}, errors.Join(err, errors.New(capture.CaptureError))
	}
	if len(capture.Paths) == 0 {
		return postgres.CoreWorkspace{}, ErrNoChanges
	}
	manifest, err := manifestFromEntries(capture.Entries)
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	paths := manifestPaths(manifest)
	manifestRaw, _ := json.Marshal(manifest)
	material, _ := stableHash(struct {
		WorkspaceID, OperationID, SourceCommit string
		ChangedPaths                           []string
	}{uuidString(row.ID), operationID, row.SourceCommit, paths})
	operation, _, err := m.ensureOperation(ctx, row, operationID, "capture", material)
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	args := append([]string{"--literal-pathspecs", "add", "--all", "--"}, paths...)
	if _, err := m.git(ctx, row.WorkspacePath, args...); err != nil {
		return postgres.CoreWorkspace{}, err
	}
	diff, err := m.git(ctx, row.WorkspacePath, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", row.SourceCommit, "--")
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	diffSum := sha256.Sum256([]byte(diff))
	diffHash := hex.EncodeToString(diffSum[:])
	artifactPath, err := m.materializeArtifact(diffHash, []byte(diff))
	if err != nil {
		return postgres.CoreWorkspace{}, err
	}
	return m.persistCapture(ctx, row, operation, capture.RawStatus, manifestRaw, diffHash, artifactPath, int64(len(diff)))
}

func manifestFromEntries(entries []gitstate.Entry) ([]Change, error) {
	manifest := make([]Change, 0, len(entries))
	for _, entry := range entries {
		for _, candidate := range []string{entry.Path, entry.OldPath} {
			if candidate == "" {
				continue
			}
			clean := filepath.Clean(candidate)
			if filepath.IsAbs(candidate) || clean != candidate || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) ||
				clean == ".git" || strings.HasPrefix(clean, ".git"+string(filepath.Separator)) {
				return nil, fmt.Errorf("%w: changed path %q is unsafe", ErrUnsafePath, candidate)
			}
		}
		manifest = append(manifest, Change{Status: entry.Status, Kind: string(entry.Kind), Path: entry.Path, OldPath: entry.OldPath})
	}
	sort.Slice(manifest, func(i, j int) bool {
		return manifest[i].Path+"\x00"+manifest[i].OldPath+"\x00"+manifest[i].Status < manifest[j].Path+"\x00"+manifest[j].OldPath+"\x00"+manifest[j].Status
	})
	return manifest, nil
}

func manifestPaths(manifest []Change) []string {
	seen := make(map[string]bool, len(manifest)*2)
	for _, change := range manifest {
		seen[change.Path] = true
		if change.OldPath != "" {
			seen[change.OldPath] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (m *Manager) persistCapture(ctx context.Context, expected postgres.CoreWorkspace, operation postgres.CoreWorkspaceOperation, status string, manifest []byte, diffHash, artifactPath string, size int64) (postgres.CoreWorkspace, error) {
	var updated postgres.CoreWorkspace
	err := pgx.BeginFunc(ctx, m.config.Pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		current, err := queries.GetWorkspaceForUpdate(ctx, expected.ID)
		if err != nil {
			return err
		}
		if current.Status == string(StatusReconciling) {
			if !bytes.Equal(current.GitStatus, []byte(status)) || !sameJSON(current.ChangedManifest, manifest) || current.DiffSha256.String != diffHash {
				return &ConflictError{Effect: "workspace capture", Detail: "canonical capture differs"}
			}
			updated = current
			return nil
		}
		if current.Status != string(StatusFrozen) || current.AggregateVersion != expected.AggregateVersion {
			return &ConflictError{Effect: "workspace capture", Detail: "canonical state is not the expected frozen version"}
		}
		artifact, err := queries.GetArtifactBySHA256(ctx, diffHash)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			artifact, err = queries.InsertArtifact(ctx, postgres.InsertArtifactParams{
				ID: newUUID(), Sha256: diffHash, SizeBytes: size, MediaType: "text/x-diff",
				LogicalKind: "workspace_diff", StoragePath: artifactPath, CreatedAt: timestamp(m.now()),
			})
		case err == nil:
			if artifact.SizeBytes != size || artifact.MediaType != "text/x-diff" || artifact.LogicalKind != "workspace_diff" || artifact.StoragePath != artifactPath {
				return &ConflictError{Effect: "workspace diff artifact", Detail: "content-addressed metadata differs"}
			}
		}
		if err != nil {
			return err
		}
		effectRaw, _ := json.Marshal(struct {
			StatusSHA256                         string          `json:"status_sha256"`
			Manifest                             json.RawMessage `json:"manifest"`
			ArtifactID, ArtifactPath, DiffSHA256 string
		}{hexString([]byte(status)), manifest, uuidString(artifact.ID), artifactPath, diffHash})
		if operation.Status == "applied" {
			if !sameJSON(operation.Effect, effectRaw) {
				return &ConflictError{Effect: "workspace capture", Detail: "applied effect differs"}
			}
		} else if _, err := queries.CompleteWorkspaceOperation(ctx, postgres.CompleteWorkspaceOperationParams{
			Effect: effectRaw, AppliedAt: timestamp(m.now()), OperationID: operation.OperationID,
			WorkspaceID: operation.WorkspaceID, OperationKind: operation.OperationKind, MaterialSha256: operation.MaterialSha256,
		}); err != nil {
			return err
		}
		updated, err = queries.RecordWorkspaceCapture(ctx, postgres.RecordWorkspaceCaptureParams{
			GitStatus: []byte(status), ChangedManifest: manifest, DiffArtifactID: artifact.ID,
			DiffSha256: textValue(diffHash), UpdatedAt: timestamp(m.now()), WorkspaceID: current.ID,
			ExpectedAggregateVersion: current.AggregateVersion,
		})
		if err != nil {
			return err
		}
		return appendWorkspaceEvent(ctx, queries, updated, "workspace.reconciling", operation.OperationID, map[string]any{
			"changed_manifest": json.RawMessage(manifest), "diff_artifact_id": uuidString(artifact.ID), "diff_sha256": diffHash,
		})
	})
	if err != nil {
		return postgres.CoreWorkspace{}, fmt.Errorf("persist workspace capture: %w", err)
	}
	return updated, nil
}

func (m *Manager) materializeArtifact(hash string, content []byte) (string, error) {
	if len(hash) != 64 || hexString(content) != hash {
		return "", errors.New("workspace artifact hash does not match content")
	}
	parent := filepath.Join(m.artifactRoot.Root(), "sha256", hash[:2], hash[2:4])
	if err := m.artifactRoot.EnsureDir(parent, 0o755); err != nil {
		return "", fmt.Errorf("%w: create artifact path: %v", ErrUnsafePath, err)
	}
	directory, found, err := m.artifactRoot.OpenDir(parent, false)
	if err != nil || !found {
		return "", errors.Join(err, os.ErrNotExist)
	}
	defer directory.Close()
	file, err := directory.OpenFile(hash, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if n, writeErr := file.Write(content); writeErr != nil || n != len(content) {
			_ = file.Close()
			return "", errors.Join(writeErr, errors.New("short artifact write"))
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Chmod(0o444); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		if err := directory.Sync(); err != nil {
			return "", err
		}
	} else if errors.Is(err, os.ErrExist) {
		existing, found, readErr := directory.ReadFileLimit(hash, false, int64(len(content)+1))
		if readErr != nil || !found || !bytes.Equal(existing, content) {
			return "", errors.Join(readErr, &ConflictError{Effect: "workspace artifact", Detail: "content-addressed path contains different bytes"})
		}
	} else {
		return "", err
	}
	return filepath.Join(parent, hash), nil
}

func hexString(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
