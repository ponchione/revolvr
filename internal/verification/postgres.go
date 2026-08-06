package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/storage/postgres"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("verification PostgreSQL store requires a pool")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) FindReusable(ctx context.Context, fingerprint string) (ReusableCheck, bool, error) {
	if !hexSHA256.MatchString(fingerprint) {
		return ReusableCheck{}, false, invalidPlan("execution fingerprint is invalid")
	}
	queries := postgres.New(s.pool)
	row, err := queries.FindReusableVerificationCheck(ctx, fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReusableCheck{}, false, nil
	}
	if err != nil {
		return ReusableCheck{}, false, err
	}
	stdout, err := queries.GetArtifactByID(ctx, row.StdoutArtifactID)
	if err != nil {
		return ReusableCheck{}, false, err
	}
	stderr, err := queries.GetArtifactByID(ctx, row.StderrArtifactID)
	if err != nil {
		return ReusableCheck{}, false, err
	}
	var failures []string
	if json.Unmarshal(row.FailureSignatures, &failures) != nil {
		return ReusableCheck{}, false, errors.New("reusable verification failure signatures are malformed")
	}
	result := ReusableCheck{
		ID: uuidString(row.ID), Outcome: Outcome(row.Outcome),
		Stdout: artifactFromRow(stdout), Stderr: artifactFromRow(stderr),
		ParsedResult:      append(json.RawMessage(nil), row.ParsedResult...),
		SandboxEvidence:   append(json.RawMessage(nil), row.SandboxEvidence...),
		FailureSignatures: failures, SandboxSpecificationSHA256: row.SandboxSpecificationSha256,
		OriginalExecutedAt: row.OriginalExecutedAt.Time.UTC(),
	}
	if row.ExitCode.Valid {
		exitCode := int(row.ExitCode.Int32)
		result.ExitCode = &exitCode
	}
	return result, true, nil
}

func (s *PostgresStore) Persist(ctx context.Context, run PersistedRun) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		ids, err := persistenceIDs(run)
		if err != nil {
			return err
		}
		authority, err := queries.GetVerificationPersistenceAuthority(ctx, postgres.GetVerificationPersistenceAuthorityParams{RunID: ids.run, WorkspaceID: ids.workspace})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStaleSource
		}
		if err != nil {
			return err
		}
		if authority.RunStatus != "active" || authority.WorkspaceStatus != "frozen" ||
			uuidString(authority.ProjectID) != run.Pinned.ProjectID || uuidString(authority.TaskID) != run.Pinned.TaskID ||
			uuidString(authority.TaskVersionID) != run.Pinned.TaskVersionID || !authority.AcceptedVersionID.Valid ||
			uuidString(authority.AcceptedVersionID) != run.Pinned.TaskVersionID || uuidString(authority.WorkspaceRunID) != run.Pinned.RunID ||
			uuidString(authority.WorkspaceProjectID) != run.Pinned.ProjectID || uuidString(authority.WorkspaceTaskID) != run.Pinned.TaskID ||
			!authority.CandidateCommit.Valid || authority.CandidateCommit.String != run.Pinned.Candidate.Commit ||
			!authority.CandidateTree.Valid || authority.CandidateTree.String != run.Pinned.Candidate.Tree {
			return ErrStaleSource
		}
		acceptedPlan, err := canonicalAnyJSON(authority.AcceptedVerificationPlan)
		if err != nil || hashBytes(acceptedPlan) != run.Pinned.Plan.VerificationPlanSHA256 {
			return ErrAuthorityChanged
		}
		pinnedRaw, err := json.Marshal(run.Pinned)
		if err != nil {
			return err
		}
		differentialRaw, err := json.Marshal(run.Differential)
		if err != nil {
			return err
		}
		if _, err := queries.InsertVerificationRun(ctx, postgres.InsertVerificationRunParams{
			ID: ids.verificationRun, ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.taskVersion, RunID: ids.run, WorkspaceID: ids.workspace,
			Purpose: string(run.Purpose), Status: string(run.Status),
			PlanSchemaVersion: run.Pinned.Plan.SchemaVersion, PlanVersion: run.Pinned.Plan.Version,
			PlanSha256: run.Pinned.PlanSHA256, PinnedPlan: pinnedRaw,
			CandidateCommit: run.Pinned.Candidate.Commit, CandidateTree: run.Pinned.Candidate.Tree,
			ProjectEnvironmentSha256: run.Pinned.ProjectEnvironment.SHA256,
			ProjectEnvironment:       run.Pinned.ProjectEnvironment.Contract, Differential: differentialRaw,
			StartedAt: timestamp(run.StartedAt), CompletedAt: timestamp(run.CompletedAt), CreatedAt: timestamp(run.CompletedAt),
		}); err != nil {
			return err
		}
		for _, check := range run.Checks {
			stdoutID, err := registerArtifact(ctx, queries, check.Stdout, run.CompletedAt)
			if err != nil {
				return err
			}
			stderrID, err := registerArtifact(ctx, queries, check.Stderr, run.CompletedAt)
			if err != nil {
				return err
			}
			checkID, err := parseUUID(check.ID)
			if err != nil {
				return err
			}
			reusedID := pgtype.UUID{}
			if check.ReusedFromCheckID != "" {
				reusedID, err = parseUUID(check.ReusedFromCheckID)
				if err != nil {
					return err
				}
			}
			argv, _ := json.Marshal(check.Gate.Argv)
			environment, _ := json.Marshal(nonNilEnvironment(check.Gate.Environment))
			authorityInputs, _ := json.Marshal(nonNilInputs(check.Gate.AuthorityInputs))
			outputPolicy, _ := json.Marshal(check.Gate.OutputPolicy)
			failureSignatures, _ := json.Marshal(nonNilStrings(check.FailureSignatures))
			parsed := nonemptyObject(check.ParsedResult)
			evidence := nonemptyObject(check.SandboxEvidence)
			exitCode := pgtype.Int4{}
			if check.ExitCode != nil {
				exitCode = pgtype.Int4{Int32: int32(*check.ExitCode), Valid: true}
			}
			if _, err := queries.InsertVerificationCheck(ctx, postgres.InsertVerificationCheckParams{
				ID: checkID, VerificationRunID: ids.verificationRun, RunID: ids.run,
				Ordinal: int32(check.Ordinal), GateID: check.Gate.ID, Tier: int16(check.Gate.Tier), Outcome: string(check.Outcome),
				ExecutionFingerprint:          check.ExecutionFingerprint,
				VerifierProtocolVersion:       check.VerifierProtocolVersion,
				VerifierImplementationVersion: check.VerifierImplementationVersion,
				ParserKind:                    string(check.Gate.Parser.Kind), ParserVersion: check.Gate.Parser.Version,
				SourceCommit: check.Gate.Source.Commit, SourceTree: check.Gate.Source.Tree,
				CommandArgv: argv, WorkingDirectory: check.Gate.WorkingDirectory, Environment: environment,
				ImageReference: check.Gate.Image.Reference, ImageDigest: check.Gate.Image.Digest,
				SandboxProfile: string(check.Gate.SandboxProfile), SandboxProfileSha256: check.Gate.SandboxProfileSHA256,
				SandboxSpecificationSha256: check.SandboxSpecificationSHA256,
				AuthorityInputs:            authorityInputs, OutputPolicy: outputPolicy, ExitCode: exitCode,
				TimedOut: check.TimedOut, Cancelled: check.Cancelled,
				StdoutArtifactID: stdoutID, StderrArtifactID: stderrID,
				ParsedResult: parsed, SandboxEvidence: evidence, FailureSignatures: failureSignatures,
				ReusedFromCheckID: reusedID, OriginalExecutedAt: timestamp(check.OriginalExecutedAt),
				OccurredAt: timestamp(check.OccurredAt), StartedAt: timestamp(check.StartedAt),
				CompletedAt: timestamp(check.CompletedAt), CreatedAt: timestamp(run.CompletedAt),
			}); err != nil {
				return err
			}
		}
		payload, err := json.Marshal(map[string]any{
			"schema_version":      "revolvr-verification-result-event-v1",
			"verification_run_id": run.ID, "purpose": run.Purpose, "status": run.Status,
			"plan_sha256": run.Pinned.PlanSHA256, "candidate_commit": run.Pinned.Candidate.Commit,
			"candidate_tree": run.Pinned.Candidate.Tree, "check_count": len(run.Checks),
			"differential": run.Differential,
		})
		if err != nil {
			return err
		}
		_, err = queries.AppendEvent(ctx, postgres.AppendEventParams{
			ID: ids.event, ProjectID: ids.project, TaskID: ids.task, RunID: ids.run,
			EventType: "verification.result_recorded", AggregateType: "verification_run",
			AggregateID: ids.verificationRun, AggregateVersion: 1, Payload: payload,
			CreatedAt: timestamp(run.CompletedAt),
		})
		return err
	})
}

type parsedPersistenceIDs struct {
	verificationRun pgtype.UUID
	event           pgtype.UUID
	project         pgtype.UUID
	task            pgtype.UUID
	taskVersion     pgtype.UUID
	run             pgtype.UUID
	workspace       pgtype.UUID
}

func persistenceIDs(run PersistedRun) (parsedPersistenceIDs, error) {
	values := []string{run.ID, run.EventID, run.Pinned.ProjectID, run.Pinned.TaskID, run.Pinned.TaskVersionID, run.Pinned.RunID, run.Pinned.WorkspaceID}
	parsed := make([]pgtype.UUID, len(values))
	for index, value := range values {
		var err error
		parsed[index], err = parseUUID(value)
		if err != nil {
			return parsedPersistenceIDs{}, err
		}
	}
	return parsedPersistenceIDs{parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], parsed[5], parsed[6]}, nil
}

func registerArtifact(ctx context.Context, queries *postgres.Queries, artifact Artifact, createdAt time.Time) (pgtype.UUID, error) {
	if !hexSHA256.MatchString(artifact.SHA256) || artifact.SizeBytes < 0 || artifact.MediaType == "" || artifact.LogicalKind == "" || artifact.StoragePath == "" {
		return pgtype.UUID{}, fmt.Errorf("%w: artifact metadata is invalid", ErrArtifact)
	}
	existing, err := queries.GetArtifactBySHA256(ctx, artifact.SHA256)
	if err == nil {
		if existing.SizeBytes != artifact.SizeBytes {
			return pgtype.UUID{}, fmt.Errorf("%w: content address has conflicting size", ErrArtifact)
		}
		return existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, err
	}
	if len(artifact.Content) == 0 && artifact.SizeBytes != 0 {
		return pgtype.UUID{}, fmt.Errorf("%w: new artifact content is unavailable", ErrArtifact)
	}
	if int64(len(artifact.Content)) != artifact.SizeBytes || hashBytes(artifact.Content) != artifact.SHA256 {
		return pgtype.UUID{}, fmt.Errorf("%w: artifact content identity is invalid", ErrArtifact)
	}
	id, err := parseUUID(artifact.ID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	row, err := queries.InsertArtifact(ctx, postgres.InsertArtifactParams{
		ID: id, Sha256: artifact.SHA256, SizeBytes: artifact.SizeBytes,
		MediaType: artifact.MediaType, LogicalKind: artifact.LogicalKind,
		StoragePath: artifact.StoragePath, CreatedAt: timestamp(createdAt),
	})
	return row.ID, err
}

func artifactFromRow(row postgres.CoreArtifact) Artifact {
	return Artifact{
		ID: uuidString(row.ID), SHA256: row.Sha256, SizeBytes: row.SizeBytes,
		MediaType: row.MediaType, LogicalKind: row.LogicalKind, StoragePath: row.StoragePath,
	}
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

func nonNilInputs(values []MaterialInput) []MaterialInput {
	if values == nil {
		return []MaterialInput{}
	}
	return values
}

func nonNilEnvironment(values []EnvironmentVariable) []EnvironmentVariable {
	if values == nil {
		return []EnvironmentVariable{}
	}
	return values
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonemptyObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
