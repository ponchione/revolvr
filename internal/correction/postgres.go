package correction

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/audit"
	"revolvr/internal/evidence"
	"revolvr/internal/id"
	storage "revolvr/internal/storage/postgres"
)

type PostgresStore struct {
	pool  *pgxpool.Pool
	newID func() string
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("correction PostgreSQL store requires a pool")
	}
	return &PostgresStore{pool: pool, newID: id.New}, nil
}

func (s *PostgresStore) SetIDGenerator(generator func() string) {
	if generator != nil {
		s.newID = generator
	}
}

func (s *PostgresStore) HasFailedStrategy(ctx context.Context, taskID, failureSHA, fingerprint string) (bool, error) {
	parsedTask, err := correctionUUID(taskID)
	if err != nil {
		return false, err
	}
	_, err = storage.New(s.pool).FindFailedStrategy(ctx, storage.FindFailedStrategyParams{TaskID: parsedTask, SignatureSha256: failureSHA, StrategyFingerprint: fingerprint})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *PostgresStore) Begin(ctx context.Context, identity audit.Identity, record AttemptRecord) error {
	if err := validateAttemptRecord(record); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		existing, err := queries.GetStrategyByOperationID(ctx, record.OperationID)
		if err == nil {
			var failureSHA string
			if err := tx.QueryRow(ctx, `SELECT signature_sha256 FROM core.failure_signatures WHERE id=$1`, existing.FailureSignatureID).Scan(&failureSHA); err != nil {
				return err
			}
			if uuidString(existing.ID) != record.StrategyID || existing.StrategyFingerprint != record.StrategyFingerprint || existing.DossierSha256 != record.DossierSHA256 || existing.CorrectorInvocationID != record.CorrectorInvocationID || existing.SandboxSpecificationSha256 != record.SandboxSpecificationSHA256 || failureSHA != record.Failure.SHA256 {
				return errors.New("correction strategy replay material changed")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		ids, err := parseCorrectionIdentity(identity)
		if err != nil {
			return err
		}
		if err := validateFailureAuthority(ctx, tx, identity, record.Failure); err != nil {
			return err
		}
		verificationID, checkID, auditID := pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}
		if record.Failure.VerificationRunID != "" {
			verificationID, err = correctionUUID(record.Failure.VerificationRunID)
			if err != nil {
				return err
			}
		}
		if record.Failure.VerificationCheckID != "" {
			checkID, err = correctionUUID(record.Failure.VerificationCheckID)
			if err != nil {
				return err
			}
		}
		if record.Failure.AuditRunID != "" {
			auditID, err = correctionUUID(record.Failure.AuditRunID)
			if err != nil {
				return err
			}
		}
		findingKeys, _ := json.Marshal(record.Failure.FindingIDs)
		signatureID, err := correctionUUID(s.newID())
		if err != nil {
			return err
		}
		if _, err := queries.InsertFailureSignature(ctx, storage.InsertFailureSignatureParams{
			ID: signatureID, OperationID: record.OperationID + ".failure", ProjectID: ids.project,
			TaskID: ids.task, TaskVersionID: ids.taskVersion, RunID: ids.run, WorkspaceID: ids.workspace,
			AuthorityKind: string(record.Failure.AuthorityKind), VerificationRunID: verificationID,
			VerificationCheckID: checkID, AuditRunID: auditID, FindingKeys: findingKeys,
			SourceCommit: record.Failure.Source.Commit, SourceTree: record.Failure.Source.Tree,
			NormalizedMaterial: record.Failure.NormalizedMaterial, SignatureSha256: record.Failure.SHA256,
			CreatedAt: correctionTimestamp(record.StartedAt),
		}); err != nil {
			return err
		}
		strategyRaw, _ := json.Marshal(record.Strategy)
		strategyID, err := correctionUUID(record.StrategyID)
		if err != nil {
			return err
		}
		if _, err := queries.InsertStrategy(ctx, storage.InsertStrategyParams{
			ID: strategyID, OperationID: record.OperationID, ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.taskVersion, RunID: ids.run, WorkspaceID: ids.workspace,
			FailureSignatureID: signatureID, SourceCommit: record.Failure.Source.Commit,
			SourceTree: record.Failure.Source.Tree, DossierSha256: record.DossierSHA256,
			StrategyFingerprint: record.StrategyFingerprint, NormalizedStrategy: strategyRaw,
			CorrectorInvocationID:      record.CorrectorInvocationID,
			SandboxSpecificationSha256: record.SandboxSpecificationSHA256,
			CreatedAt:                  correctionTimestamp(record.StartedAt),
		}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"schema_version": StrategySchemaVersion, "strategy_id": record.StrategyID,
			"failure_sha256": record.Failure.SHA256, "strategy_fingerprint": record.StrategyFingerprint,
			"dossier_sha256": record.DossierSHA256,
		})
		eventID, err := correctionUUID(s.newID())
		if err != nil {
			return err
		}
		_, err = queries.AppendEvent(ctx, storage.AppendEventParams{
			ID: eventID, ProjectID: ids.project, TaskID: ids.task, RunID: ids.run,
			EventType: "correction.strategy_admitted", AggregateType: "strategy",
			AggregateID: strategyID, AggregateVersion: 1, Payload: payload,
			CreatedAt: correctionTimestamp(record.StartedAt),
		})
		return err
	})
}

func (s *PostgresStore) Complete(ctx context.Context, identity audit.Identity, record OutcomeRecord) error {
	if err := validateOutcomeRecord(record); err != nil {
		return err
	}
	recordSHA := outcomeRecordHash(record)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		strategyID, err := correctionUUID(record.StrategyID)
		if err != nil {
			return err
		}
		existing, err := queries.GetStrategyOutcome(ctx, strategyID)
		if err == nil {
			if uuidString(existing.ID) != record.ID || existing.RecordSha256 != recordSHA {
				return errors.New("strategy outcome replay material changed")
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		ids, err := parseCorrectionIdentity(identity)
		if err != nil {
			return err
		}
		var ownerProject, ownerTask, ownerVersion, ownerRun, ownerWorkspace pgtype.UUID
		var strategyCreated time.Time
		if err := tx.QueryRow(ctx, `SELECT project_id,task_id,task_version_id,run_id,workspace_id,created_at FROM core.strategies WHERE id=$1 FOR SHARE`, strategyID).Scan(&ownerProject, &ownerTask, &ownerVersion, &ownerRun, &ownerWorkspace, &strategyCreated); err != nil {
			return err
		}
		if ownerProject != ids.project || ownerTask != ids.task || ownerVersion != ids.taskVersion || ownerRun != ids.run || ownerWorkspace != ids.workspace || record.CompletedAt.Before(strategyCreated) {
			return errors.New("strategy outcome task authority changed")
		}
		verificationID, auditID := pgtype.UUID{}, pgtype.UUID{}
		if record.VerificationRunID != "" {
			verificationID, err = correctionUUID(record.VerificationRunID)
			if err != nil {
				return err
			}
		}
		if record.AuditRunID != "" {
			auditID, err = correctionUUID(record.AuditRunID)
			if err != nil {
				return err
			}
		}
		if err := validateOutcomeAuthority(ctx, tx, identity, record, verificationID, auditID); err != nil {
			return err
		}
		commit, tree, diff := optionalText(record.ResultingSource.Commit), optionalText(record.ResultingSource.Tree), optionalText(record.ResultingSource.DiffSHA256)
		evidenceValues := record.Evidence
		if evidenceValues == nil {
			evidenceValues = []audit.DispositionEvidence{}
		}
		evidenceRaw, _ := json.Marshal(evidenceValues)
		outcomeID, err := correctionUUID(record.ID)
		if err != nil {
			return err
		}
		if _, err := queries.InsertStrategyOutcome(ctx, storage.InsertStrategyOutcomeParams{
			ID: outcomeID, StrategyID: strategyID, TaskID: ids.task, Outcome: record.Outcome,
			ResultingSourceCommit: commit, ResultingSourceTree: tree, DiffSha256: diff,
			VerificationRunID: verificationID, AuditRunID: auditID, Evidence: evidenceRaw,
			RecordSha256: recordSHA, CompletedAt: correctionTimestamp(record.CompletedAt), CreatedAt: correctionTimestamp(record.CompletedAt),
		}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"schema_version": OutcomeSchemaVersion, "strategy_id": record.StrategyID,
			"outcome": record.Outcome, "source_commit": record.ResultingSource.Commit,
			"source_tree": record.ResultingSource.Tree, "diff_sha256": record.ResultingSource.DiffSHA256,
			"verification_run_id": record.VerificationRunID, "audit_run_id": record.AuditRunID,
		})
		eventID, err := correctionUUID(s.newID())
		if err != nil {
			return err
		}
		_, err = queries.AppendEvent(ctx, storage.AppendEventParams{
			ID: eventID, ProjectID: ids.project, TaskID: ids.task, RunID: ids.run,
			EventType: "correction.strategy_completed", AggregateType: "strategy",
			AggregateID: strategyID, AggregateVersion: 2, Payload: payload,
			CreatedAt: correctionTimestamp(record.CompletedAt),
		})
		return err
	})
}

func validateFailureAuthority(ctx context.Context, tx pgx.Tx, identity audit.Identity, failure FailureSignature) error {
	material, err := decodeFailureMaterial(failure)
	if err != nil {
		return ErrInvalidAuthority
	}
	switch failure.AuthorityKind {
	case AuthorityVerification:
		var taskID, versionID, runID, workspaceID pgtype.UUID
		var outcome, gateID, commit, tree string
		var exitCode pgtype.Int4
		if err := tx.QueryRow(ctx, `SELECT v.task_id,v.task_version_id,v.run_id,v.workspace_id,c.outcome,c.gate_id,c.exit_code,c.source_commit,c.source_tree
			FROM core.verification_checks c JOIN core.verification_runs v ON v.id=c.verification_run_id
			WHERE v.id=$1 AND c.id=$2`, failure.VerificationRunID, failure.VerificationCheckID).Scan(&taskID, &versionID, &runID, &workspaceID, &outcome, &gateID, &exitCode, &commit, &tree); err != nil {
			return err
		}
		if uuidString(taskID) != identity.TaskID || uuidString(versionID) != identity.TaskVersionID || uuidString(runID) != identity.RunID || uuidString(workspaceID) != identity.WorkspaceID || (outcome == "passed" || outcome == "passed_reused") || commit != failure.Source.Commit || tree != failure.Source.Tree || material.GateID != gateID || string(material.Outcome) != outcome || !exitCode.Valid || material.ExitCode != int(exitCode.Int32) {
			return ErrInvalidAuthority
		}
	case AuthorityFindings:
		var taskID, versionID, runID, workspaceID pgtype.UUID
		var disposition, commit, tree string
		if err := tx.QueryRow(ctx, `SELECT task_id,task_version_id,run_id,workspace_id,disposition,source_commit,source_tree FROM core.audit_runs WHERE id=$1`, failure.AuditRunID).Scan(&taskID, &versionID, &runID, &workspaceID, &disposition, &commit, &tree); err != nil {
			return err
		}
		if uuidString(taskID) != identity.TaskID || uuidString(versionID) != identity.TaskVersionID || uuidString(runID) != identity.RunID || uuidString(workspaceID) != identity.WorkspaceID || disposition != "changes_required" || commit != failure.Source.Commit || tree != failure.Source.Tree {
			return ErrInvalidAuthority
		}
		for _, findingID := range failure.FindingIDs {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM core.audit_findings f
				JOIN core.audit_finding_occurrences o ON o.finding_id=f.id
				LEFT JOIN core.finding_dispositions d ON d.finding_id=f.id
				WHERE o.audit_run_id=$1 AND f.finding_key=$2 AND d.id IS NULL`, failure.AuditRunID, findingID).Scan(&count); err != nil || count != 1 {
				return errors.Join(ErrInvalidAuthority, err)
			}
		}
		definitions := make([]string, 0, len(failure.FindingIDs))
		rows, err := tx.Query(ctx, `SELECT f.definition_sha256 FROM core.audit_findings f
			JOIN core.audit_finding_occurrences o ON o.finding_id=f.id
			WHERE o.audit_run_id=$1 AND f.finding_key = ANY($2::text[])`, failure.AuditRunID, failure.FindingIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var definition string
			if err := rows.Scan(&definition); err != nil {
				return err
			}
			definitions = append(definitions, definition)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		slices.Sort(definitions)
		if !slices.Equal(definitions, material.FindingDefinitions) || !slices.Equal(failure.FindingIDs, material.FindingIDs) {
			return ErrInvalidAuthority
		}
	default:
		return ErrInvalidAuthority
	}
	return nil
}

func validateAttemptRecord(record AttemptRecord) error {
	if !correctionToken(record.OperationID) || !correctionToken(record.CorrectorInvocationID) || record.StartedAt.IsZero() || !validCorrectionSHA(record.DossierSHA256) || !validCorrectionSHA(record.SandboxSpecificationSHA256) {
		return errors.New("correction strategy occurrence identity, time, dossier, or sandbox authority is invalid")
	}
	if _, err := correctionUUID(record.StrategyID); err != nil {
		return err
	}
	material, err := decodeFailureMaterial(record.Failure)
	if err != nil || material.Kind != record.Failure.AuthorityKind || material.Source != record.Failure.Source {
		return ErrInvalidAuthority
	}
	fingerprint, normalized, err := StrategyFingerprint(record.Strategy)
	if err != nil || fingerprint != record.StrategyFingerprint || !reflect.DeepEqual(normalized, record.Strategy) {
		return errors.New("correction strategy fingerprint or normalized material is divergent")
	}
	return nil
}

func decodeFailureMaterial(failure FailureSignature) (failureMaterial, error) {
	if failure.SchemaVersion != FailureSchemaVersion || failure.SHA256 != modelHash(failure.NormalizedMaterial) || !validCorrectionSHA(failure.Source.Revision) || !validCorrectionSHA(failure.Source.DiffSHA256) || !validCorrectionGitOID(failure.Source.Commit) || !validCorrectionGitOID(failure.Source.Tree) {
		return failureMaterial{}, ErrInvalidAuthority
	}
	var material failureMaterial
	if err := json.Unmarshal(failure.NormalizedMaterial, &material); err != nil {
		return failureMaterial{}, ErrInvalidAuthority
	}
	canonical, err := json.Marshal(material)
	if err != nil || !bytes.Equal(canonical, failure.NormalizedMaterial) || material.Kind != failure.AuthorityKind || material.Source != failure.Source {
		return failureMaterial{}, ErrInvalidAuthority
	}
	if failure.AuthorityKind == AuthorityVerification {
		if failure.VerificationRunID == "" || failure.VerificationCheckID == "" || failure.AuditRunID != "" || len(failure.FindingIDs) != 0 || len(material.FindingIDs) != 0 || material.GateID == "" || !material.Outcome.Failed() {
			return failureMaterial{}, ErrInvalidAuthority
		}
	} else if failure.AuthorityKind == AuthorityFindings {
		if failure.VerificationRunID != "" || failure.VerificationCheckID != "" || failure.AuditRunID == "" || len(failure.FindingIDs) == 0 || !slices.IsSorted(failure.FindingIDs) || !slices.Equal(failure.FindingIDs, material.FindingIDs) || !slices.IsSorted(material.FindingDefinitions) || len(material.FindingDefinitions) != len(material.FindingIDs) {
			return failureMaterial{}, ErrInvalidAuthority
		}
	} else {
		return failureMaterial{}, ErrInvalidAuthority
	}
	return material, nil
}

func validateOutcomeRecord(record OutcomeRecord) error {
	if _, err := correctionUUID(record.ID); err != nil {
		return err
	}
	if _, err := correctionUUID(record.StrategyID); err != nil {
		return err
	}
	if record.CompletedAt.IsZero() || !slices.Contains([]string{"succeeded", "failed", "no_progress", "cancelled", "budget_exhausted", "blocked"}, record.Outcome) {
		return errors.New("strategy outcome identity, status, or completion time is invalid")
	}
	sourcePresent := record.ResultingSource != (audit.Source{})
	if sourcePresent && (!validCorrectionSHA(record.ResultingSource.Revision) || !validCorrectionSHA(record.ResultingSource.DiffSHA256) || !validCorrectionGitOID(record.ResultingSource.Commit) || !validCorrectionGitOID(record.ResultingSource.Tree)) {
		return errors.New("strategy outcome source identity is malformed")
	}
	for _, item := range record.Evidence {
		if !correctionToken(item.Kind) || !correctionToken(item.ID) || !validCorrectionSHA(item.SHA256) || strings.TrimSpace(item.Reference) == "" {
			return errors.New("strategy outcome evidence is malformed")
		}
	}
	if record.Outcome == "succeeded" && (!sourcePresent || record.VerificationRunID == "" || record.AuditRunID == "" || len(record.Evidence) == 0) {
		return errors.New("successful correction outcome lacks source, verification, audit, or evidence")
	}
	return nil
}

func validateOutcomeAuthority(ctx context.Context, tx pgx.Tx, identity audit.Identity, record OutcomeRecord, verificationID, auditID pgtype.UUID) error {
	var verificationStatus, verificationPurpose, verificationCommit, verificationTree string
	var verificationCompleted time.Time
	if verificationID.Valid {
		var projectID, taskID, versionID, runID, workspaceID pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT project_id,task_id,task_version_id,run_id,workspace_id,status,purpose,candidate_commit,candidate_tree,completed_at FROM core.verification_runs WHERE id=$1`, verificationID).Scan(&projectID, &taskID, &versionID, &runID, &workspaceID, &verificationStatus, &verificationPurpose, &verificationCommit, &verificationTree, &verificationCompleted); err != nil {
			return err
		}
		if uuidString(projectID) != identity.ProjectID || uuidString(taskID) != identity.TaskID || uuidString(versionID) != identity.TaskVersionID || uuidString(runID) != identity.RunID || uuidString(workspaceID) != identity.WorkspaceID {
			return errors.New("strategy outcome verification ownership is stale")
		}
	}
	var auditDisposition, auditCommit, auditTree string
	var auditVerificationID pgtype.UUID
	var auditCompleted time.Time
	if auditID.Valid {
		var projectID, taskID, versionID, runID, workspaceID pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT project_id,task_id,task_version_id,run_id,workspace_id,verification_run_id,disposition,source_commit,source_tree,completed_at FROM core.audit_runs WHERE id=$1`, auditID).Scan(&projectID, &taskID, &versionID, &runID, &workspaceID, &auditVerificationID, &auditDisposition, &auditCommit, &auditTree, &auditCompleted); err != nil {
			return err
		}
		if uuidString(projectID) != identity.ProjectID || uuidString(taskID) != identity.TaskID || uuidString(versionID) != identity.TaskVersionID || uuidString(runID) != identity.RunID || uuidString(workspaceID) != identity.WorkspaceID {
			return errors.New("strategy outcome audit ownership is stale")
		}
	}
	if record.ResultingSource != (audit.Source{}) {
		if verificationID.Valid && (verificationCommit != record.ResultingSource.Commit || verificationTree != record.ResultingSource.Tree) || auditID.Valid && (auditCommit != record.ResultingSource.Commit || auditTree != record.ResultingSource.Tree) {
			return errors.New("strategy outcome evidence is bound to a different source")
		}
	}
	if record.Outcome == "succeeded" {
		if verificationStatus != "passed" || verificationPurpose != "final" || auditDisposition != "clean" || auditVerificationID != verificationID || auditCompleted.Before(verificationCompleted) || record.CompletedAt.Before(auditCompleted) {
			return errors.New("successful correction lacks exact fresh final verification and clean re-audit")
		}
	}
	return nil
}

func correctionToken(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, " \t\r\n")
}

func validCorrectionSHA(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}

func validCorrectionGitOID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && (len(raw) == 20 || len(raw) == 32) && value == strings.ToLower(value)
}

type correctionIDs struct{ project, task, taskVersion, run, workspace pgtype.UUID }

func parseCorrectionIdentity(identity audit.Identity) (correctionIDs, error) {
	values := []string{identity.ProjectID, identity.TaskID, identity.TaskVersionID, identity.RunID, identity.WorkspaceID}
	parsed := make([]pgtype.UUID, len(values))
	for i, value := range values {
		var err error
		parsed[i], err = correctionUUID(value)
		if err != nil {
			return correctionIDs{}, err
		}
	}
	return correctionIDs{parsed[0], parsed[1], parsed[2], parsed[3], parsed[4]}, nil
}

func correctionUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID %q: %w", value, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
func correctionTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func optionalText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func modelHash(raw []byte) string           { return evidence.HashBytes(raw) }
