package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/completion"
	"revolvr/internal/evidence"
	"revolvr/internal/id"
	"revolvr/internal/model"
	storage "revolvr/internal/storage/postgres"
)

type PersistenceFailurePoint string

const (
	FailureBeforeFindings PersistenceFailurePoint = "before_findings"
	FailureBeforeEvent    PersistenceFailurePoint = "before_event"
)

type PersistenceFailureInjector func(PersistenceFailurePoint) error

type PersistCommand struct {
	OperationID string
	Candidate   Candidate
	Report      evidence.Artifact
	StartedAt   time.Time
	CompletedAt time.Time
}

type PersistResult struct {
	AuditRunID string
	Replay     bool
}

type PostgresStore struct {
	pool    *pgxpool.Pool
	newID   func() string
	failure PersistenceFailureInjector
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("audit PostgreSQL store requires a pool")
	}
	return &PostgresStore{pool: pool, newID: id.New}, nil
}

func (s *PostgresStore) SetFailureInjector(injector PersistenceFailureInjector) { s.failure = injector }
func (s *PostgresStore) SetIDGenerator(generator func() string) {
	if generator != nil {
		s.newID = generator
	}
}

func (s *PostgresStore) Persist(ctx context.Context, command PersistCommand) (PersistResult, error) {
	var result PersistResult
	recordSHA, err := validatePersistCommand(command)
	if err != nil {
		return result, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		existing, err := queries.GetAuditRunByOperationID(ctx, command.OperationID)
		if err == nil {
			if existing.RecordSha256 != recordSHA || uuidString(existing.ID) != command.Candidate.AuditID {
				return fmt.Errorf("%w: audit operation replay material changed", ErrPersistence)
			}
			result = PersistResult{AuditRunID: uuidString(existing.ID), Replay: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		verificationID, err := parseUUID(command.Candidate.Output.Authority.VerificationRunID)
		if err != nil {
			return err
		}
		authority, err := queries.GetAuditPersistenceAuthority(ctx, verificationID)
		if err != nil {
			return errors.Join(ErrStaleAuthority, err)
		}
		if err := comparePersistenceAuthority(authority, command); err != nil {
			return err
		}

		reportID, err := registerAuditArtifact(ctx, queries, command.Report, command.CompletedAt)
		if err != nil {
			return err
		}
		ids, err := parseAuditIDs(command)
		if err != nil {
			return err
		}
		provenanceID, err := s.newUUID()
		if err != nil {
			return err
		}
		if _, err := queries.InsertArtifactProvenance(ctx, storage.InsertArtifactProvenanceParams{
			ID: provenanceID, ArtifactID: reportID, ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.taskVersion, RunID: ids.run, WorkspaceID: ids.workspace,
			ProducerRole: "auditor", ProducingOperationID: command.OperationID,
			SourceCommit: command.Candidate.Output.Authority.SourceCommit,
			SourceTree:   command.Candidate.Output.Authority.SourceTree,
			CreatedAt:    timestamp(command.CompletedAt),
		}); err != nil {
			return err
		}
		modelRequest, err := json.Marshal(command.Candidate.ModelRequest)
		if err != nil {
			return err
		}
		modelResult, err := json.Marshal(command.Candidate.ModelResult)
		if err != nil {
			return err
		}
		mutatingInvocations, err := json.Marshal(command.Candidate.SourceMutatingInvocationIDs)
		if err != nil {
			return err
		}
		if _, err := queries.InsertAuditRun(ctx, storage.InsertAuditRunParams{
			ID: ids.audit, OperationID: command.OperationID, ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.taskVersion, RunID: ids.run, WorkspaceID: ids.workspace,
			VerificationRunID: verificationID, AuditKind: string(command.Candidate.Kind),
			Disposition: string(command.Candidate.Output.Disposition), Independent: true,
			AuditorInvocationID:         command.Candidate.AuditorInvocationID,
			SourceMutatingInvocationIds: mutatingInvocations,
			DossierSchemaVersion:        command.Candidate.Dossier.SchemaVersion,
			DossierSha256:               command.Candidate.Dossier.SHA256,
			Dossier:                     command.Candidate.Dossier.Content,
			PromptVersion:               command.Candidate.PromptVersion, PromptSha256: command.Candidate.PromptSHA256,
			Prompt:                command.Candidate.Prompt,
			ResponseSchemaVersion: OutputSchemaVersion,
			ResponseSchemaSha256:  command.Candidate.ResponseSchemaSHA256,
			ResponseSchema:        command.Candidate.ResponseSchema,
			Model:                 command.Candidate.ModelPolicy.Model, ModelRequest: modelRequest,
			ModelResult:      modelResult,
			SourceCommit:     command.Candidate.Output.Authority.SourceCommit,
			SourceTree:       command.Candidate.Output.Authority.SourceTree,
			DiffSha256:       command.Candidate.Output.Authority.DiffSHA256,
			ReportArtifactID: reportID, ReportSha256: command.Report.SHA256, RecordSha256: recordSHA,
			StartedAt: timestamp(command.StartedAt), CompletedAt: timestamp(command.CompletedAt), CreatedAt: timestamp(command.CompletedAt),
		}); err != nil {
			return err
		}
		if s.failure != nil {
			if err := s.failure(FailureBeforeFindings); err != nil {
				return err
			}
		}
		for index, finding := range command.Candidate.Output.Findings {
			findingID, err := s.persistFinding(ctx, queries, ids, command, finding)
			if err != nil {
				return err
			}
			occurrenceSHA, _ := evidence.Hash(struct {
				AuditID, FindingID, DefinitionSHA string
				Ordinal                           int
			}{command.Candidate.AuditID, uuidString(findingID), FindingDefinitionSHA256(finding), index + 1})
			occurrenceID, err := s.newUUID()
			if err != nil {
				return err
			}
			if _, err := queries.InsertAuditFindingOccurrence(ctx, storage.InsertAuditFindingOccurrenceParams{
				ID: occurrenceID, AuditRunID: ids.audit, TaskID: ids.task, FindingID: findingID,
				Ordinal: int32(index + 1), OccurrenceSha256: occurrenceSHA, CreatedAt: timestamp(command.CompletedAt),
			}); err != nil {
				return err
			}
		}
		if s.failure != nil {
			if err := s.failure(FailureBeforeEvent); err != nil {
				return err
			}
		}
		payload, _ := json.Marshal(map[string]any{
			"schema_version": AuditRecordVersion, "audit_run_id": command.Candidate.AuditID,
			"audit_kind": command.Candidate.Kind, "disposition": command.Candidate.Output.Disposition,
			"source_commit":       command.Candidate.Output.Authority.SourceCommit,
			"source_tree":         command.Candidate.Output.Authority.SourceTree,
			"verification_run_id": command.Candidate.Output.Authority.VerificationRunID,
			"dossier_sha256":      command.Candidate.Dossier.SHA256,
			"report_sha256":       command.Report.SHA256, "finding_count": len(command.Candidate.Output.Findings),
		})
		eventID, err := s.newUUID()
		if err != nil {
			return err
		}
		if _, err := queries.AppendEvent(ctx, storage.AppendEventParams{
			ID: eventID, ProjectID: ids.project, TaskID: ids.task, RunID: ids.run,
			EventType: "audit.result_recorded", AggregateType: "audit_run", AggregateID: ids.audit,
			AggregateVersion: 1, Payload: payload, CreatedAt: timestamp(command.CompletedAt),
		}); err != nil {
			return err
		}
		result = PersistResult{AuditRunID: command.Candidate.AuditID}
		return nil
	})
	if err != nil {
		return PersistResult{}, fmt.Errorf("%w: %w", ErrPersistence, err)
	}
	return result, nil
}

type auditIDs struct{ audit, project, task, taskVersion, run, workspace pgtype.UUID }

func parseAuditIDs(command PersistCommand) (auditIDs, error) {
	values := []string{command.Candidate.AuditID, command.Candidate.DossierInput().Identity.ProjectID, command.Candidate.Output.Authority.TaskID, command.Candidate.Output.Authority.TaskVersionID, command.Candidate.Output.Authority.RunID, command.Candidate.DossierInput().Identity.WorkspaceID}
	parsed := make([]pgtype.UUID, len(values))
	for i, value := range values {
		var err error
		parsed[i], err = parseUUID(value)
		if err != nil {
			return auditIDs{}, err
		}
	}
	return auditIDs{parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], parsed[5]}, nil
}

func (c Candidate) DossierInput() DossierInput {
	var dossier Dossier
	_ = json.Unmarshal(c.Dossier.Content, &dossier)
	return dossier.Input
}

func (s *PostgresStore) persistFinding(ctx context.Context, queries *storage.Queries, ids auditIDs, command PersistCommand, finding Finding) (pgtype.UUID, error) {
	definitionSHA := FindingDefinitionSHA256(finding)
	existing, err := queries.GetAuditFindingByTaskAndKey(ctx, storage.GetAuditFindingByTaskAndKeyParams{TaskID: ids.task, FindingKey: finding.ID})
	if err == nil {
		if existing.DefinitionSha256 != definitionSHA || existing.Significance != string(finding.Significance) || existing.Summary != finding.Summary || existing.RequiredCorrection != finding.RequiredCorrection {
			return pgtype.UUID{}, fmt.Errorf("finding %q reused with a materially different definition", finding.ID)
		}
		return existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, err
	}
	sourceEvidence, _ := json.Marshal(finding.SourceEvidence)
	affectedFiles, _ := json.Marshal(finding.AffectedFiles)
	affectedSymbols, _ := json.Marshal(finding.AffectedSymbols)
	criterionImpact, _ := json.Marshal(finding.CriterionImpact)
	findingID, err := s.newUUID()
	if err != nil {
		return pgtype.UUID{}, err
	}
	row, err := queries.InsertAuditFinding(ctx, storage.InsertAuditFindingParams{
		ID: findingID, ProjectID: ids.project, TaskID: ids.task, TaskVersionID: ids.taskVersion,
		RunID: ids.run, WorkspaceID: ids.workspace, IntroducedAuditRunID: ids.audit,
		FindingKey: finding.ID, Significance: string(finding.Significance), Summary: finding.Summary,
		RequiredCorrection: finding.RequiredCorrection, SourceEvidence: sourceEvidence,
		AffectedFiles: affectedFiles, AffectedSymbols: affectedSymbols, CriterionImpact: criterionImpact,
		DefinitionSha256: definitionSHA, SourceCommit: command.Candidate.Output.Authority.SourceCommit,
		SourceTree: command.Candidate.Output.Authority.SourceTree, CreatedAt: timestamp(command.CompletedAt),
	})
	return row.ID, err
}

func validatePersistCommand(command PersistCommand) (string, error) {
	if !token(command.OperationID) || command.Candidate.AuditID == "" || command.Candidate.Output.Authority.AuditID != command.Candidate.AuditID {
		return "", errors.New("persist audit: operation or audit identity is invalid")
	}
	if command.StartedAt.IsZero() || command.CompletedAt.Before(command.StartedAt) {
		return "", errors.New("persist audit: occurrence times are invalid")
	}
	expectedSchema, schemaErr := OutputSchema()
	parsedOutput, outputErr := ParseOutput(command.Candidate.RawOutput)
	canonicalOutput, canonicalErr := json.Marshal(command.Candidate.Output)
	if schemaErr != nil || outputErr != nil || canonicalErr != nil || !reflect.DeepEqual(parsedOutput, command.Candidate.Output) || !bytes.Equal(canonicalOutput, command.Candidate.CanonicalOutput) ||
		command.Candidate.Dossier.ByteSize != len(command.Candidate.Dossier.Content) || command.Candidate.Dossier.SHA256 != model.SHA256(command.Candidate.Dossier.Content) ||
		command.Candidate.PromptVersion != PromptVersion || command.Candidate.Prompt == "" || command.Candidate.PromptSHA256 != model.SHA256([]byte(command.Candidate.Prompt)) ||
		!bytes.Equal(command.Candidate.ResponseSchema, expectedSchema) || command.Candidate.ResponseSchemaSHA256 != schemaIdentity(expectedSchema) ||
		validateModelPolicy(command.Candidate.ModelPolicy, true) != nil || !reflect.DeepEqual(command.Candidate.ModelRequest, command.Candidate.ModelResult.Request) ||
		command.Candidate.ModelResult.Outcome != model.OutcomeSuccess || !bytes.Equal(command.Candidate.ModelResult.StructuredOutput, command.Candidate.RawOutput) ||
		command.Candidate.ModelRequest.Model != command.Candidate.ModelPolicy.Model || command.Candidate.ModelRequest.OutputIdentity != command.Candidate.Output.RevolvrIdentity {
		return "", errors.New("persist audit: dossier, prompt, schema, model, or output provenance is divergent")
	}
	if len(command.Candidate.SourceMutatingInvocationIDs) == 0 {
		return "", errors.New("persist audit: source-mutating invocation authority is missing")
	}
	seenInvocations := map[string]struct{}{}
	for _, invocationID := range command.Candidate.SourceMutatingInvocationIDs {
		if !token(invocationID) || invocationID == command.Candidate.AuditorInvocationID {
			return "", errors.New("persist audit: auditor is not independent from source mutation")
		}
		if _, duplicate := seenInvocations[invocationID]; duplicate {
			return "", errors.New("persist audit: source-mutating invocation authority repeats")
		}
		seenInvocations[invocationID] = struct{}{}
	}
	if err := ValidateOutput(command.Candidate.Output, command.Candidate.Dossier, command.Candidate.Output.Authority, command.Candidate.Output.RevolvrIdentity); err != nil {
		return "", err
	}
	if command.Report.ID == "" || command.Report.Kind != "audit_report" || command.Report.MediaType != "application/json" || !command.Report.Resolved || command.Report.SHA256 != evidence.HashBytes(command.Report.Content) || command.Report.SizeBytes != int64(len(command.Report.Content)) || !slices.Equal(command.Report.Content, command.Candidate.CanonicalOutput) {
		return "", errors.New("persist audit: report artifact bytes or identity are divergent")
	}
	input := command.Candidate.DossierInput()
	provenance := command.Report.Provenance
	if err := provenance.Validate(); err != nil || provenance.ProjectID != input.Identity.ProjectID || provenance.TaskID != input.Identity.TaskID || provenance.TaskVersionID != input.Identity.TaskVersionID || provenance.RunID != input.Identity.RunID || provenance.WorkspaceID != input.Identity.WorkspaceID || provenance.ProducerRole != "auditor" || provenance.ProducingOperationID != command.OperationID || provenance.SourceCommit != input.Source.Commit || provenance.SourceTree != input.Source.Tree {
		return "", errors.New("persist audit: report artifact provenance is stale")
	}
	material := struct {
		OperationID            string
		Candidate              Candidate
		Report                 evidence.Artifact
		StartedAt, CompletedAt time.Time
	}{command.OperationID, command.Candidate, command.Report, command.StartedAt.UTC(), command.CompletedAt.UTC()}
	material.Report.Content = nil
	return evidence.Hash(material)
}

func comparePersistenceAuthority(row storage.GetAuditPersistenceAuthorityRow, command PersistCommand) error {
	input := command.Candidate.DossierInput()
	if uuidString(row.ProjectID) != input.Identity.ProjectID || uuidString(row.TaskID) != input.Identity.TaskID || uuidString(row.TaskVersionID) != input.Identity.TaskVersionID || uuidString(row.RunID) != input.Identity.RunID || uuidString(row.WorkspaceID) != input.Identity.WorkspaceID || !row.AcceptedVersionID.Valid || uuidString(row.AcceptedVersionID) != input.Identity.TaskVersionID {
		return ErrStaleAuthority
	}
	if row.VerificationPurpose != "final" || row.VerificationStatus != "passed" || row.CandidateCommit != input.Source.Commit || row.CandidateTree != input.Source.Tree || row.RunStatus != "active" || row.WorkspaceStatus != "frozen" || !row.WorkspaceCandidateCommit.Valid || row.WorkspaceCandidateCommit.String != input.Source.Commit || !row.WorkspaceCandidateTree.Valid || row.WorkspaceCandidateTree.String != input.Source.Tree || !row.WorkspaceDiffSha256.Valid || row.WorkspaceDiffSha256.String != input.Source.DiffSHA256 {
		return ErrStaleAuthority
	}
	if !command.StartedAt.After(row.VerificationCompletedAt.Time) {
		return errors.New("audit occurrence is not newer than its exact verification")
	}
	return nil
}

func registerAuditArtifact(ctx context.Context, queries *storage.Queries, artifact evidence.Artifact, createdAt time.Time) (pgtype.UUID, error) {
	existing, err := queries.GetArtifactBySHA256(ctx, artifact.SHA256)
	if err == nil {
		if existing.SizeBytes != artifact.SizeBytes || existing.MediaType != artifact.MediaType || existing.LogicalKind != artifact.Kind || existing.StoragePath != artifact.StoragePath {
			return pgtype.UUID{}, errors.New("audit artifact content address has divergent metadata")
		}
		return existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, err
	}
	idValue, err := parseUUID(artifact.ID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	row, err := queries.InsertArtifact(ctx, storage.InsertArtifactParams{
		ID: idValue, Sha256: artifact.SHA256, SizeBytes: artifact.SizeBytes,
		MediaType: artifact.MediaType, LogicalKind: artifact.Kind,
		StoragePath: artifact.StoragePath, CreatedAt: timestamp(createdAt),
	})
	return row.ID, err
}

func (s *PostgresStore) newUUID() (pgtype.UUID, error) {
	parsed, err := parseUUID(s.newID())
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("audit ID generator returned a non-UUID: %w", err)
	}
	return parsed, nil
}

func parseUUID(value string) (pgtype.UUID, error) {
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

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}

// CompletionOverlay replaces architecture-018's fixture audit projection with
// canonical immutable audit/finding rows while preserving the delegate's
// budget, invocation, claim, trajectory, and harness evidence.
type CompletionOverlay struct {
	Base completion.SupplementalSource
}

func (s *CompletionOverlay) LoadCompletionSupplement(ctx context.Context, queries *storage.Queries, key completion.Key) (completion.Supplement, error) {
	if s == nil || s.Base == nil {
		return completion.Supplement{}, errors.New("audit completion overlay requires a base evidence source")
	}
	supplement, err := s.Base.LoadCompletionSupplement(ctx, queries, key)
	if err != nil {
		return completion.Supplement{}, err
	}
	taskID, err := parseUUID(key.Identity.TaskID)
	if err != nil {
		return completion.Supplement{}, err
	}
	runID, err := parseUUID(key.Identity.RunID)
	if err != nil {
		return completion.Supplement{}, err
	}
	workspaceID, err := parseUUID(key.Identity.WorkspaceID)
	if err != nil {
		return completion.Supplement{}, err
	}
	run, err := queries.GetLatestAuditByTaskRunWorkspace(ctx, storage.GetLatestAuditByTaskRunWorkspaceParams{TaskID: taskID, RunID: runID, WorkspaceID: workspaceID})
	if err != nil {
		return completion.Supplement{}, err
	}
	supplement.Audit = &completion.Audit{
		SchemaVersion: completion.AuditSchemaVersion, ID: uuidString(run.ID), RunID: uuidString(run.RunID),
		Role: "auditor", Independent: run.Independent, Disposition: run.Disposition,
		SourceCommit: run.SourceCommit, SourceTree: run.SourceTree,
		ReportArtifactID: uuidString(run.ReportArtifactID), ReportSHA256: run.ReportSha256,
		CompletedAt: run.CompletedAt.Time.UTC(),
	}
	rows, err := queries.ListTaskFindingsWithDisposition(ctx, taskID)
	if err != nil {
		return completion.Supplement{}, err
	}
	supplement.Findings = make([]completion.Finding, 0, len(rows))
	for _, row := range rows {
		supplement.Findings = append(supplement.Findings, completion.Finding{ID: row.FindingKey, Significance: row.Significance, Status: row.DispositionStatus, EvidenceID: uuidString(row.DispositionID)})
	}
	if !slices.ContainsFunc(supplement.Artifacts, func(value evidence.ArtifactReference) bool { return value.ID == uuidString(run.ReportArtifactID) }) {
		artifact, err := queries.GetArtifactByID(ctx, run.ReportArtifactID)
		if err != nil {
			return completion.Supplement{}, err
		}
		provenance, err := queries.GetArtifactProvenanceByArtifactAndRun(ctx, storage.GetArtifactProvenanceByArtifactAndRunParams{ArtifactID: run.ReportArtifactID, RunID: run.RunID})
		if err != nil {
			return completion.Supplement{}, err
		}
		supplement.Artifacts = append(supplement.Artifacts, evidence.ArtifactReference{
			ID: uuidString(artifact.ID), Kind: artifact.LogicalKind, MediaType: artifact.MediaType,
			SHA256: artifact.Sha256, SizeBytes: artifact.SizeBytes, StoragePath: artifact.StoragePath,
			Resolved: true, Required: true,
			Provenance: evidence.Provenance{
				SchemaVersion: evidence.ArtifactProvenanceSchemaVersion,
				ProjectID:     uuidString(provenance.ProjectID), TaskID: uuidString(provenance.TaskID),
				TaskVersionID: uuidString(provenance.TaskVersionID), RunID: uuidString(provenance.RunID),
				WorkspaceID: uuidString(provenance.WorkspaceID), ProducerRole: provenance.ProducerRole,
				ProducingOperationID: provenance.ProducingOperationID,
				SourceCommit:         provenance.SourceCommit, SourceTree: provenance.SourceTree,
			},
		})
	}
	return supplement, nil
}
