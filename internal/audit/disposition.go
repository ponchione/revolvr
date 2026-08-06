package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"revolvr/internal/evidence"
	storage "revolvr/internal/storage/postgres"
)

type FindingStatus string

const (
	FindingResolved   FindingStatus = "resolved"
	FindingWaived     FindingStatus = "waived"
	FindingRejected   FindingStatus = "rejected"
	FindingSuperseded FindingStatus = "superseded"
	FindingStale      FindingStatus = "stale"
)

type DispositionEvidence struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	SHA256    string `json:"sha256"`
	Reference string `json:"reference"`
}

type DispositionCommand struct {
	ID                          string
	OperationID                 string
	TaskID                      string
	FindingID                   string
	Status                      FindingStatus
	AuthorityRole               string
	AuthorityID                 string
	ResolutionVerificationRunID string
	ResolutionAuditRunID        string
	SupersedingFindingID        string
	SourceCommit                string
	SourceTree                  string
	Evidence                    []DispositionEvidence
	Rationale                   string
	CreatedAt                   time.Time
}

type DispositionResult struct {
	ID     string
	Replay bool
}

func (s *PostgresStore) Disposition(ctx context.Context, command DispositionCommand) (DispositionResult, error) {
	results, err := s.DispositionMany(ctx, []DispositionCommand{command})
	if err != nil {
		return DispositionResult{}, err
	}
	return results[0], nil
}

// DispositionMany applies an exact finding set atomically. Either every
// transition is durable or none is, so a failed correction cannot partially
// close its authority and then become ineligible for retry.
func (s *PostgresStore) DispositionMany(ctx context.Context, commands []DispositionCommand) ([]DispositionResult, error) {
	if len(commands) == 0 {
		return nil, errors.New("finding disposition batch is empty")
	}
	recordSHAs := make([]string, len(commands))
	seenFindings := map[string]struct{}{}
	for index, command := range commands {
		sha, err := validateDisposition(command)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seenFindings[command.TaskID+"\x00"+command.FindingID]; duplicate {
			return nil, errors.New("finding disposition batch repeats a finding")
		}
		seenFindings[command.TaskID+"\x00"+command.FindingID] = struct{}{}
		recordSHAs[index] = sha
	}
	results := make([]DispositionResult, len(commands))
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		for index, command := range commands {
			result, err := dispositionInTransaction(ctx, tx, queries, command, recordSHAs[index])
			if err != nil {
				return err
			}
			results[index] = result
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: disposition finding batch: %w", ErrPersistence, err)
	}
	return results, nil
}

func dispositionInTransaction(ctx context.Context, tx pgx.Tx, queries *storage.Queries, command DispositionCommand, recordSHA string) (DispositionResult, error) {
	existing, err := queries.GetFindingDispositionByOperationID(ctx, command.OperationID)
	if err == nil {
		if existing.RecordSha256 != recordSHA || uuidString(existing.ID) != command.ID {
			return DispositionResult{}, errors.New("finding disposition replay material changed")
		}
		return DispositionResult{ID: uuidString(existing.ID), Replay: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DispositionResult{}, err
	}
	taskID, err := parseUUID(command.TaskID)
	if err != nil {
		return DispositionResult{}, err
	}
	finding, err := queries.GetAuditFindingByTaskAndKey(ctx, storage.GetAuditFindingByTaskAndKeyParams{TaskID: taskID, FindingKey: command.FindingID})
	if err != nil {
		return DispositionResult{}, err
	}
	verificationID, auditID, supersedingID := pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}
	if command.ResolutionVerificationRunID != "" {
		verificationID, err = parseUUID(command.ResolutionVerificationRunID)
		if err != nil {
			return DispositionResult{}, err
		}
	}
	if command.ResolutionAuditRunID != "" {
		auditID, err = parseUUID(command.ResolutionAuditRunID)
		if err != nil {
			return DispositionResult{}, err
		}
	}
	if command.SupersedingFindingID != "" {
		superseding, getErr := queries.GetAuditFindingByTaskAndKey(ctx, storage.GetAuditFindingByTaskAndKeyParams{TaskID: taskID, FindingKey: command.SupersedingFindingID})
		if getErr != nil {
			return DispositionResult{}, getErr
		}
		supersedingID = superseding.ID
	}
	if command.Status == FindingResolved {
		var verificationStatus, purpose, commit, tree, auditDisposition, auditCommit, auditTree string
		if err := tx.QueryRow(ctx, `SELECT status,purpose,candidate_commit,candidate_tree FROM core.verification_runs WHERE id=$1`, verificationID).Scan(&verificationStatus, &purpose, &commit, &tree); err != nil {
			return DispositionResult{}, err
		}
		if err := tx.QueryRow(ctx, `SELECT disposition,source_commit,source_tree FROM core.audit_runs WHERE id=$1 AND verification_run_id=$2 AND task_id=$3`, auditID, verificationID, taskID).Scan(&auditDisposition, &auditCommit, &auditTree); err != nil {
			return DispositionResult{}, err
		}
		if verificationStatus != "passed" || purpose != "final" || auditDisposition != "clean" || commit != command.SourceCommit || tree != command.SourceTree || auditCommit != commit || auditTree != tree {
			return DispositionResult{}, errors.New("resolved finding lacks exact fresh final verification and clean re-audit authority")
		}
	}
	if command.Status == FindingStale {
		var workspaceCommit, workspaceTree pgtype.Text
		if err := tx.QueryRow(ctx, `SELECT candidate_commit,candidate_tree FROM core.workspaces WHERE id=$1`, finding.WorkspaceID).Scan(&workspaceCommit, &workspaceTree); err != nil {
			return DispositionResult{}, err
		}
		if !workspaceCommit.Valid || !workspaceTree.Valid || workspaceCommit.String != command.SourceCommit || workspaceTree.String != command.SourceTree || finding.SourceCommit == command.SourceCommit && finding.SourceTree == command.SourceTree {
			return DispositionResult{}, errors.New("stale finding lacks exact changed workspace source authority")
		}
	}
	evidenceRaw, _ := json.Marshal(command.Evidence)
	idValue, err := parseUUID(command.ID)
	if err != nil {
		return DispositionResult{}, err
	}
	row, err := queries.InsertFindingDisposition(ctx, storage.InsertFindingDispositionParams{
		ID: idValue, OperationID: command.OperationID, FindingID: finding.ID, TaskID: taskID,
		Status: string(command.Status), AuthorityRole: command.AuthorityRole, AuthorityID: command.AuthorityID,
		ResolutionVerificationRunID: verificationID, ResolutionAuditRunID: auditID,
		SupersedingFindingID: supersedingID, SourceCommit: command.SourceCommit, SourceTree: command.SourceTree,
		Evidence: evidenceRaw, Rationale: command.Rationale, RecordSha256: recordSHA, CreatedAt: timestamp(command.CreatedAt),
	})
	if err != nil {
		return DispositionResult{}, err
	}
	return DispositionResult{ID: uuidString(row.ID)}, nil
}

func validateDisposition(command DispositionCommand) (string, error) {
	if !token(command.ID) || !token(command.OperationID) || !token(command.TaskID) || !stableIDPattern.MatchString(command.FindingID) || !token(command.AuthorityID) || command.CreatedAt.IsZero() || !validGitOID(command.SourceCommit) || !validGitOID(command.SourceTree) || len(command.Evidence) == 0 {
		return "", errors.New("finding disposition identity, source, time, or evidence is incomplete")
	}
	for _, item := range command.Evidence {
		if !token(item.Kind) || !token(item.ID) || !validSHA256(item.SHA256) || item.Reference == "" {
			return "", errors.New("finding disposition evidence is malformed")
		}
	}
	switch command.Status {
	case FindingResolved:
		if command.AuthorityRole != "host" || command.ResolutionVerificationRunID == "" || command.ResolutionAuditRunID == "" || command.SupersedingFindingID != "" {
			return "", errors.New("resolved finding requires host final-verification and clean re-audit authority")
		}
	case FindingWaived, FindingRejected:
		if command.AuthorityRole != "operator" || !normalizedText(command.Rationale) || command.ResolutionVerificationRunID != "" || command.ResolutionAuditRunID != "" || command.SupersedingFindingID != "" {
			return "", errors.New("waiver/rejection requires exact operator authority and rationale")
		}
	case FindingSuperseded:
		if command.AuthorityRole != "host" && command.AuthorityRole != "operator" || !stableIDPattern.MatchString(command.SupersedingFindingID) || command.SupersedingFindingID == command.FindingID {
			return "", errors.New("supersession requires trusted authority and a different known finding")
		}
	case FindingStale:
		if command.AuthorityRole != "host" || command.ResolutionVerificationRunID != "" || command.ResolutionAuditRunID != "" || command.SupersedingFindingID != "" {
			return "", errors.New("staleness requires host source-change authority")
		}
	default:
		return "", fmt.Errorf("unsupported finding disposition %q", command.Status)
	}
	return evidence.Hash(command)
}
