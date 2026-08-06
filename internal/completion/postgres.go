package completion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/evidence"
	"revolvr/internal/id"
	"revolvr/internal/storage/postgres"
)

type PersistenceFailurePoint string

const (
	PersistenceFailureBeforeTerminalState PersistenceFailurePoint = "before_terminal_state"
	PersistenceFailureBeforeEvents        PersistenceFailurePoint = "before_terminal_events"
)

type PersistenceFailureInjector func(PersistenceFailurePoint) error

type SupplementalSource interface {
	LoadCompletionSupplement(context.Context, *postgres.Queries, Key) (Supplement, error)
}

type PostgresStore struct {
	pool         *pgxpool.Pool
	failure      PersistenceFailureInjector
	supplemental SupplementalSource
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("completion PostgreSQL store requires a pool")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) SetFailureInjector(failure PersistenceFailureInjector) {
	s.failure = failure
}

func (s *PostgresStore) SetSupplementalSource(source SupplementalSource) {
	s.supplemental = source
}

func (s *PostgresStore) ReadCompletionSnapshot(ctx context.Context, key Key) (Snapshot, error) {
	if s.supplemental == nil {
		return Snapshot{}, errors.New("completion PostgreSQL reader requires a supplemental evidence source")
	}
	taskID, err := parseUUID(key.Identity.TaskID)
	if err != nil {
		return Snapshot{}, err
	}
	runID, err := parseUUID(key.Identity.RunID)
	if err != nil {
		return Snapshot{}, err
	}
	workspaceID, err := parseUUID(key.Identity.WorkspaceID)
	if err != nil {
		return Snapshot{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := postgres.New(tx)
	core, err := queries.GetCompletionReadAuthority(ctx, postgres.GetCompletionReadAuthorityParams{ID: taskID, ID_2: runID, ID_3: workspaceID})
	if err != nil {
		return Snapshot{}, err
	}
	if uuidString(core.ProjectID) != key.Identity.ProjectID || uuidString(core.TaskID) != key.Identity.TaskID ||
		uuidString(core.TaskVersionID) != key.Identity.TaskVersionID || uuidString(core.RunID) != key.Identity.RunID ||
		uuidString(core.WorkspaceID) != key.Identity.WorkspaceID {
		return Snapshot{}, ErrStalePreflight
	}
	steps, err := queries.ListPlanSteps(ctx, core.AcceptedPlanVersionID)
	if err != nil {
		return Snapshot{}, err
	}
	criteria, err := queries.ListCompletionCriteria(ctx, taskID)
	if err != nil {
		return Snapshot{}, err
	}
	verificationID := pgtype.UUID{}
	// The supplemental audit/claim layer does not choose verification. Select the
	// immutable latest final occurrence for this exact completing run/source.
	if err := tx.QueryRow(ctx, `SELECT id FROM core.verification_runs
		WHERE run_id=$1 AND workspace_id=$2 AND purpose='final'
		ORDER BY completed_at DESC,id DESC LIMIT 1`, runID, workspaceID).Scan(&verificationID); err != nil {
		return Snapshot{}, err
	}
	verificationRun, err := queries.GetCompletionVerificationAuthority(ctx, verificationID)
	if err != nil {
		return Snapshot{}, err
	}
	checks, err := queries.ListVerificationChecks(ctx, verificationID)
	if err != nil {
		return Snapshot{}, err
	}
	supplement, err := s.supplemental.LoadCompletionSupplement(ctx, queries, key)
	if err != nil {
		return Snapshot{}, err
	}
	for _, artifact := range supplement.Artifacts {
		artifactID, err := parseUUID(artifact.ID)
		if err != nil {
			return Snapshot{}, err
		}
		row, err := queries.GetArtifactByID(ctx, artifactID)
		if err != nil || row.Sha256 != artifact.SHA256 || row.SizeBytes != artifact.SizeBytes ||
			row.StoragePath != artifact.StoragePath || row.MediaType != artifact.MediaType || row.LogicalKind != artifact.Kind {
			return Snapshot{}, errors.Join(ErrStalePreflight, err)
		}
	}
	planSteps := make([]PlanStep, 0, len(steps))
	for _, step := range steps {
		planSteps = append(planSteps, PlanStep{ID: step.StepID, Status: step.Status})
	}
	criterionEvidence := make(map[string]string)
	for _, criterion := range supplement.Claims {
		if criterion.CriterionID == "" {
			continue
		}
		for _, link := range criterion.Evidence {
			if link.Kind == "verification_check" {
				criterionEvidence[criterion.CriterionID] = link.ID
				break
			}
		}
	}
	completionCriteria := make([]Criterion, 0, len(criteria))
	for _, criterion := range criteria {
		id := uuidString(criterion.ID)
		completionCriteria = append(completionCriteria, Criterion{ID: id, Status: criterion.Status, VerificationCheckID: criterionEvidence[id]})
	}
	verificationChecks := make([]VerificationCheck, 0, len(checks))
	for _, check := range checks {
		verificationChecks = append(verificationChecks, VerificationCheck{
			ID: uuidString(check.ID), Tier: check.Tier, Outcome: check.Outcome,
			ExecutionFingerprint: check.ExecutionFingerprint,
			ReusedFromCheckID:    uuidString(check.ReusedFromCheckID), ImageDigest: check.ImageDigest,
			Profile: check.SandboxProfile, ProfileSHA256: check.SandboxProfileSha256,
		})
	}
	if len(verificationChecks) == 0 {
		return Snapshot{}, ErrStalePreflight
	}
	artifacts := append([]evidence.ArtifactReference(nil), supplement.Artifacts...)
	manifestHash, err := evidence.ArtifactManifestHash(artifacts)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Identity:      key.Identity, TaskStatus: core.TaskStatus, RunStatus: core.RunStatus,
		Aggregates: Aggregates{Task: core.TaskAggregateVersion, Run: core.RunAggregateVersion, Workspace: core.WorkspaceAggregateVersion, Plan: core.PlanAggregateVersion, Lease: core.LeaseAggregateVersion},
		Source:     Source{BeforeCommit: core.BeforeCommit, BeforeTree: core.BeforeTree, AfterCommit: core.CandidateCommit.String, AfterTree: core.CandidateTree.String, DiffSHA256: core.DiffSha256.String, FrozenAt: core.WorkspaceUpdatedAt.Time.UTC()},
		Plan:       &Plan{ID: uuidString(core.PlanID), VersionID: uuidString(core.AcceptedPlanVersionID), SHA256: core.AcceptedPlanContentSha256, Steps: planSteps},
		Criteria:   completionCriteria,
		Verification: &Verification{
			ID: uuidString(verificationRun.ID), Purpose: verificationRun.Purpose, Status: verificationRun.Status,
			SourceCommit: verificationRun.CandidateCommit, SourceTree: verificationRun.CandidateTree,
			ImageDigest: verificationChecks[0].ImageDigest, Profile: verificationChecks[0].Profile,
			ProfileSHA256: verificationChecks[0].ProfileSHA256,
			CompletedAt:   verificationRun.CompletedAt.Time.UTC(), Checks: verificationChecks,
		},
		Audit: supplement.Audit, Findings: append([]Finding(nil), supplement.Findings...), Budget: supplement.Budget,
		Workspace:   Workspace{Status: core.WorkspaceStatus, Reconciled: core.CandidateCommit.Valid && core.CandidateTree.Valid && core.DiffArtifactID.Valid && core.DiffSha256.Valid, CandidateCommit: core.CandidateCommit.String, CandidateTree: core.CandidateTree.String, DiffSHA256: core.DiffSha256.String},
		Lease:       Lease{Name: core.LeaseName, RunID: uuidString(core.LeaseRunID), Held: core.LeaseRunID.Valid},
		Invocations: append([]Invocation(nil), supplement.Invocations...), Artifacts: artifacts,
		ArtifactManifestSHA256: manifestHash, OperatorInputs: append([]OperatorInput(nil), supplement.OperatorInputs...),
		Trajectory: supplement.Trajectory, HarnessAssets: supplement.HarnessAssets,
		Claims: append([]evidence.Claim(nil), supplement.Claims...),
	}
	if err := tx.Commit(ctx); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *PostgresStore) LookupCompletion(ctx context.Context, key Key) (TerminalResult, bool, error) {
	row, err := postgres.New(s.pool).GetCompletionByOperationID(ctx, key.OperationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return TerminalResult{}, false, nil
	}
	if err != nil {
		return TerminalResult{}, false, err
	}
	if uuidString(row.ProjectID) != key.Identity.ProjectID || uuidString(row.TaskID) != key.Identity.TaskID ||
		uuidString(row.TaskVersionID) != key.Identity.TaskVersionID || uuidString(row.RunID) != key.Identity.RunID ||
		uuidString(row.WorkspaceID) != key.Identity.WorkspaceID {
		return TerminalResult{}, false, ErrAlreadyCompleted
	}
	return TerminalResult{CompletionID: uuidString(row.ID), Replay: true}, true, nil
}

func (s *PostgresStore) CommitCompletion(ctx context.Context, command TerminalCommand) (TerminalResult, error) {
	var result TerminalResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		preflight, err := BuildPreflight(command.Preflight.Snapshot)
		if err != nil || !preflight.Accepted() || preflight.SHA256 != command.Preflight.SHA256 {
			return ErrStalePreflight
		}
		if err := validateMaterializedCommand(command); err != nil {
			return err
		}
		existing, err := queries.GetCompletionByOperationID(ctx, command.OperationID)
		if err == nil {
			if err := compareExisting(existing, command); err != nil {
				return err
			}
			result = TerminalResult{CompletionID: uuidString(existing.ID), Replay: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		ids, err := parseTerminalIDs(command)
		if err != nil {
			return err
		}
		authority, err := queries.GetCompletionPersistenceAuthority(ctx, postgres.GetCompletionPersistenceAuthorityParams{
			ID: ids.task, ID_2: ids.run, ID_3: ids.workspace,
		})
		if err != nil {
			return errors.Join(ErrStalePreflight, err)
		}
		if err := validatePersistenceAuthority(authority, command.Preflight.Snapshot); err != nil {
			return err
		}
		verificationAuthority, err := queries.GetCompletionVerificationAuthority(ctx, ids.verification)
		if err != nil || !validVerificationAuthority(verificationAuthority, command.Preflight.Snapshot) {
			return errors.Join(ErrStalePreflight, err)
		}
		nonterminal, err := queries.CountCompletionNonterminalPlanSteps(ctx, authority.AcceptedPlanVersionID)
		if err != nil || nonterminal != 0 {
			return errors.Join(ErrStalePreflight, err)
		}
		unsatisfied, err := queries.CountCompletionUnsatisfiedCriteria(ctx, ids.task)
		if err != nil || unsatisfied != 0 {
			return errors.Join(ErrStalePreflight, err)
		}
		planSteps, err := queries.ListPlanSteps(ctx, authority.AcceptedPlanVersionID)
		if err != nil || !samePlanSteps(planSteps, command.Preflight.Snapshot.Plan.Steps) {
			return errors.Join(ErrStalePreflight, err)
		}
		criteria, err := queries.ListCompletionCriteria(ctx, ids.task)
		if err != nil || !sameCriteria(criteria, command.Preflight.Snapshot.Criteria) {
			return errors.Join(ErrStalePreflight, err)
		}
		checks, err := queries.ListVerificationChecks(ctx, ids.verification)
		if err != nil || !sameVerificationChecks(checks, command.Preflight.Snapshot.Verification.Checks) {
			return errors.Join(ErrStalePreflight, err)
		}

		completionArtifacts := []struct {
			artifact evidence.Artifact
			role     string
		}{
			{command.Materialized.EvidenceJSON, "evidence_json"},
			{command.Materialized.Markdown, "human_markdown"},
			{command.Materialized.Manifest, "manifest"},
		}
		registered := make([]pgtype.UUID, 0, len(completionArtifacts))
		for _, item := range completionArtifacts {
			artifactID, err := registerCompletionArtifact(ctx, queries, item.artifact, command.CompletedAt)
			if err != nil {
				return err
			}
			registered = append(registered, artifactID)
			if _, err := queries.InsertArtifactProvenance(ctx, postgres.InsertArtifactProvenanceParams{
				ID: newUUID(), ArtifactID: artifactID, ProjectID: ids.project, TaskID: ids.task,
				TaskVersionID: ids.taskVersion, RunID: ids.run, WorkspaceID: ids.workspace,
				ProducerRole:         item.artifact.Provenance.ProducerRole,
				ProducingOperationID: command.OperationID,
				SourceCommit:         command.Preflight.Snapshot.Source.AfterCommit,
				SourceTree:           command.Preflight.Snapshot.Source.AfterTree,
				CreatedAt:            timestamp(command.CompletedAt),
			}); err != nil {
				return err
			}
		}
		trajectoryRaw, err := json.Marshal(command.Preflight.Snapshot.Trajectory)
		if err != nil {
			return err
		}
		trajectoryHash, err := evidence.Hash(command.Preflight.Snapshot.Trajectory)
		if err != nil {
			return err
		}
		harnessRaw, err := json.Marshal(command.Preflight.Snapshot.HarnessAssets)
		if err != nil {
			return err
		}
		if _, err := queries.InsertCompletion(ctx, postgres.InsertCompletionParams{
			ID: ids.completion, OperationID: command.OperationID, ProjectID: ids.project,
			TaskID: ids.task, TaskVersionID: ids.taskVersion, RunID: ids.run,
			WorkspaceID: ids.workspace, VerificationRunID: ids.verification,
			PreflightSha256:    command.Preflight.SHA256,
			EvidenceArtifactID: registered[0], EvidenceSha256: command.Materialized.EvidenceJSON.SHA256,
			MarkdownArtifactID: registered[1], MarkdownSha256: command.Materialized.Markdown.SHA256,
			ManifestArtifactID: registered[2], ManifestSha256: command.Materialized.Manifest.SHA256,
			TrajectoryEnvelope: trajectoryRaw, TrajectorySha256: trajectoryHash,
			HarnessAssetSetManifest: harnessRaw,
			HarnessAssetSetSha256:   command.Preflight.Snapshot.HarnessAssets.ManifestSHA256,
			CompletedAt:             timestamp(command.CompletedAt), CreatedAt: timestamp(command.CompletedAt),
		}); err != nil {
			return err
		}
		for index, item := range completionArtifacts {
			if _, err := queries.InsertCompletionArtifact(ctx, postgres.InsertCompletionArtifactParams{
				CompletionID: ids.completion, Ordinal: int32(index + 1), ArtifactID: registered[index],
				ArtifactSha256: item.artifact.SHA256, ArtifactRole: item.role,
			}); err != nil {
				return err
			}
		}
		for index, artifact := range command.Preflight.Snapshot.Artifacts {
			artifactID, err := parseUUID(artifact.ID)
			if err != nil {
				return err
			}
			row, err := queries.GetArtifactByID(ctx, artifactID)
			if err != nil || row.Sha256 != artifact.SHA256 || row.SizeBytes != artifact.SizeBytes ||
				row.MediaType != artifact.MediaType || row.LogicalKind != artifact.Kind || row.StoragePath != artifact.StoragePath {
				return errors.Join(ErrStalePreflight, err)
			}
			if _, err := queries.InsertCompletionArtifact(ctx, postgres.InsertCompletionArtifactParams{
				CompletionID: ids.completion, Ordinal: int32(index + 4), ArtifactID: artifactID,
				ArtifactSha256: artifact.SHA256, ArtifactRole: "supporting",
			}); err != nil {
				return err
			}
		}
		if err := persistClaims(ctx, queries, ids, command); err != nil {
			return err
		}
		if err := injectPersistence(s.failure, PersistenceFailureBeforeTerminalState); err != nil {
			return err
		}
		task, err := queries.CompleteTask(ctx, postgres.CompleteTaskParams{
			ID: ids.task, AggregateVersion: command.Preflight.Snapshot.Aggregates.Task,
			UpdatedAt: timestamp(command.CompletedAt),
		})
		if err != nil {
			return errors.Join(ErrStalePreflight, err)
		}
		run, err := queries.CompleteRun(ctx, postgres.CompleteRunParams{
			ID: ids.run, AggregateVersion: command.Preflight.Snapshot.Aggregates.Run,
			ReleasedAt: timestamp(command.CompletedAt),
		})
		if err != nil {
			return errors.Join(ErrStalePreflight, err)
		}
		workspace, err := queries.CompleteWorkspace(ctx, postgres.CompleteWorkspaceParams{
			ID: ids.workspace, AggregateVersion: command.Preflight.Snapshot.Aggregates.Workspace,
			TerminalReason: pgtype.Text{String: "completion evidence accepted", Valid: true},
			UpdatedAt:      timestamp(command.CompletedAt),
		})
		if err != nil {
			return errors.Join(ErrStalePreflight, err)
		}
		lease, err := queries.ReleaseCompletionLease(ctx, postgres.ReleaseCompletionLeaseParams{
			RunID: ids.run, AggregateVersion: command.Preflight.Snapshot.Aggregates.Lease,
		})
		if err != nil {
			return errors.Join(ErrStalePreflight, err)
		}
		if err := injectPersistence(s.failure, PersistenceFailureBeforeEvents); err != nil {
			return err
		}
		if err := appendCompletionEvents(ctx, queries, ids, command, task.AggregateVersion, run.AggregateVersion, workspace.AggregateVersion, lease.AggregateVersion); err != nil {
			return err
		}
		result = TerminalResult{CompletionID: command.CompletionID}
		return nil
	})
	if err != nil {
		return TerminalResult{}, errors.Join(ErrPersistence, err)
	}
	return result, nil
}

func validateMaterializedCommand(command TerminalCommand) error {
	payloads, err := buildCapsulePayloads(command.Preflight)
	if err != nil {
		return err
	}
	wantProvenance := evidence.Provenance{
		SchemaVersion: evidence.ArtifactProvenanceSchemaVersion,
		ProjectID:     command.Preflight.Snapshot.Identity.ProjectID,
		TaskID:        command.Preflight.Snapshot.Identity.TaskID,
		TaskVersionID: command.Preflight.Snapshot.Identity.TaskVersionID,
		RunID:         command.Preflight.Snapshot.Identity.RunID,
		WorkspaceID:   command.Preflight.Snapshot.Identity.WorkspaceID,
		ProducerRole:  "host", ProducingOperationID: command.OperationID,
		SourceCommit: command.Preflight.Snapshot.Source.AfterCommit,
		SourceTree:   command.Preflight.Snapshot.Source.AfterTree,
	}
	tests := []struct {
		artifact  evidence.Artifact
		kind      string
		mediaType string
		content   []byte
	}{
		{command.Materialized.EvidenceJSON, "completion_evidence", "application/json", payloads.evidenceJSON},
		{command.Materialized.Markdown, "completion_markdown", "text/markdown", payloads.markdown},
		{command.Materialized.Manifest, "completion_manifest", "application/json", payloads.manifest},
	}
	for _, test := range tests {
		artifact := test.artifact
		hash := evidence.HashBytes(test.content)
		if artifact.ID == "" || artifact.Kind != test.kind || artifact.MediaType != test.mediaType ||
			artifact.SHA256 != hash || artifact.SizeBytes != int64(len(test.content)) ||
			!artifact.Resolved || !bytes.Equal(artifact.Content, test.content) ||
			artifact.Provenance != wantProvenance || !contentAddressedPath(artifact.StoragePath, hash) {
			return evidence.ErrArtifactDivergence
		}
	}
	return nil
}

func contentAddressedPath(path, hash string) bool {
	if len(hash) != 64 || filepath.Base(path) != hash {
		return false
	}
	second := filepath.Dir(path)
	first := filepath.Dir(second)
	return filepath.Base(second) == hash[2:4] && filepath.Base(first) == hash[:2] &&
		filepath.Base(filepath.Dir(first)) == "sha256"
}

func samePlanSteps(rows []postgres.CorePlanStep, steps []PlanStep) bool {
	if len(rows) != len(steps) {
		return false
	}
	for index, row := range rows {
		if row.StepID != steps[index].ID || row.Status != steps[index].Status {
			return false
		}
	}
	return true
}

func sameCriteria(rows []postgres.ListCompletionCriteriaRow, criteria []Criterion) bool {
	if len(rows) != len(criteria) {
		return false
	}
	states := make(map[string]string, len(rows))
	for _, row := range rows {
		states[uuidString(row.ID)] = row.Status
	}
	for _, criterion := range criteria {
		if states[criterion.ID] != criterion.Status {
			return false
		}
	}
	return true
}

func sameVerificationChecks(rows []postgres.CoreVerificationCheck, checks []VerificationCheck) bool {
	if len(rows) != len(checks) {
		return false
	}
	for index, row := range rows {
		check := checks[index]
		if uuidString(row.ID) != check.ID || row.Tier != check.Tier || row.Outcome != check.Outcome ||
			row.ExecutionFingerprint != check.ExecutionFingerprint || uuidString(row.ReusedFromCheckID) != check.ReusedFromCheckID ||
			row.ImageDigest != check.ImageDigest || row.SandboxProfile != check.Profile ||
			row.SandboxProfileSha256 != check.ProfileSHA256 {
			return false
		}
	}
	return true
}

type terminalIDs struct {
	completion, project, task, taskVersion, run, workspace, verification pgtype.UUID
}

func parseTerminalIDs(command TerminalCommand) (terminalIDs, error) {
	values := []string{
		command.CompletionID, command.Preflight.Snapshot.Identity.ProjectID,
		command.Preflight.Snapshot.Identity.TaskID, command.Preflight.Snapshot.Identity.TaskVersionID,
		command.Preflight.Snapshot.Identity.RunID, command.Preflight.Snapshot.Identity.WorkspaceID,
		command.Preflight.Snapshot.Verification.ID,
	}
	parsed := make([]pgtype.UUID, len(values))
	for index, value := range values {
		var err error
		parsed[index], err = parseUUID(value)
		if err != nil {
			return terminalIDs{}, err
		}
	}
	return terminalIDs{parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], parsed[5], parsed[6]}, nil
}

func validatePersistenceAuthority(row postgres.GetCompletionPersistenceAuthorityRow, snapshot Snapshot) error {
	if uuidString(row.ProjectID) != snapshot.Identity.ProjectID || uuidString(row.TaskID) != snapshot.Identity.TaskID ||
		!row.AcceptedVersionID.Valid || uuidString(row.AcceptedVersionID) != snapshot.Identity.TaskVersionID ||
		row.TaskStatus != "finalizing" || row.TaskAggregateVersion != snapshot.Aggregates.Task ||
		uuidString(row.RunID) != snapshot.Identity.RunID || uuidString(row.TaskVersionID) != snapshot.Identity.TaskVersionID ||
		row.RunStatus != "active" || row.RunAggregateVersion != snapshot.Aggregates.Run ||
		uuidString(row.WorkspaceID) != snapshot.Identity.WorkspaceID || row.WorkspaceStatus != "frozen" ||
		row.WorkspaceAggregateVersion != snapshot.Aggregates.Workspace || !row.CandidateCommit.Valid ||
		row.CandidateCommit.String != snapshot.Source.AfterCommit || !row.CandidateTree.Valid ||
		row.CandidateTree.String != snapshot.Source.AfterTree || !row.DiffSha256.Valid ||
		row.DiffSha256.String != snapshot.Source.DiffSHA256 || snapshot.Plan == nil ||
		uuidString(row.PlanID) != snapshot.Plan.ID || !row.AcceptedPlanVersionID.Valid ||
		uuidString(row.AcceptedPlanVersionID) != snapshot.Plan.VersionID ||
		row.AcceptedPlanContentSha256 != snapshot.Plan.SHA256 || row.PlanAggregateVersion != snapshot.Aggregates.Plan ||
		row.LeaseName != "global-source-mutation-v1" || !row.LeaseRunID.Valid ||
		uuidString(row.LeaseRunID) != snapshot.Identity.RunID || row.LeaseAggregateVersion != snapshot.Aggregates.Lease {
		return ErrStalePreflight
	}
	return nil
}

func validVerificationAuthority(row postgres.GetCompletionVerificationAuthorityRow, snapshot Snapshot) bool {
	verification := snapshot.Verification
	return verification != nil && uuidString(row.ID) == verification.ID &&
		uuidString(row.ProjectID) == snapshot.Identity.ProjectID && uuidString(row.TaskID) == snapshot.Identity.TaskID &&
		uuidString(row.TaskVersionID) == snapshot.Identity.TaskVersionID && uuidString(row.RunID) == snapshot.Identity.RunID &&
		uuidString(row.WorkspaceID) == snapshot.Identity.WorkspaceID && row.Purpose == "final" && row.Status == "passed" &&
		row.CandidateCommit == snapshot.Source.AfterCommit && row.CandidateTree == snapshot.Source.AfterTree &&
		row.CompletedAt.Valid && row.CompletedAt.Time.UTC().Equal(verification.CompletedAt.UTC()) && row.CheckCount > 0 &&
		row.FreshFinalCheckCount > 0 && row.NonfreshOrNonpassingCheckCount == 0
}

func registerCompletionArtifact(ctx context.Context, queries *postgres.Queries, artifact evidence.Artifact, createdAt time.Time) (pgtype.UUID, error) {
	if artifact.SHA256 != evidence.HashBytes(artifact.Content) || int64(len(artifact.Content)) != artifact.SizeBytes ||
		artifact.Kind == "" || artifact.MediaType == "" || artifact.StoragePath == "" || !artifact.Resolved {
		return pgtype.UUID{}, evidence.ErrInvalidEvidence
	}
	existing, err := queries.GetArtifactBySHA256(ctx, artifact.SHA256)
	if err == nil {
		if existing.SizeBytes != artifact.SizeBytes {
			return pgtype.UUID{}, evidence.ErrArtifactDivergence
		}
		return existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, err
	}
	artifactID, err := parseUUID(artifact.ID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	row, err := queries.InsertArtifact(ctx, postgres.InsertArtifactParams{
		ID: artifactID, Sha256: artifact.SHA256, SizeBytes: artifact.SizeBytes,
		MediaType: artifact.MediaType, LogicalKind: artifact.Kind,
		StoragePath: artifact.StoragePath, CreatedAt: timestamp(createdAt),
	})
	return row.ID, err
}

func persistClaims(ctx context.Context, queries *postgres.Queries, ids terminalIDs, command TerminalCommand) error {
	for _, claim := range command.Preflight.Snapshot.Claims {
		claimID, err := parseUUID(claim.ID)
		if err != nil {
			return err
		}
		criterionID := pgtype.UUID{}
		if claim.CriterionID != "" {
			criterionID, err = parseUUID(claim.CriterionID)
			if err != nil {
				return err
			}
		}
		if _, err := queries.InsertClaim(ctx, postgres.InsertClaimParams{
			ID: claimID, ProjectID: ids.project, TaskID: ids.task, TaskVersionID: ids.taskVersion,
			RunID: ids.run, CriterionID: criterionID, ClaimKey: claim.Key,
			Statement: claim.Statement, StatementSha256: claim.StatementSHA256,
			CreatedAt: timestamp(command.CompletedAt),
		}); err != nil {
			return err
		}
		for index, link := range claim.Evidence {
			artifactID, checkID := pgtype.UUID{}, pgtype.UUID{}
			if link.Kind == "artifact" {
				artifactID, err = parseUUID(link.ID)
			} else {
				checkID, err = parseUUID(link.ID)
			}
			if err != nil {
				return err
			}
			if _, err := queries.InsertClaimEvidence(ctx, postgres.InsertClaimEvidenceParams{
				ClaimID: claimID, ProjectID: ids.project, TaskID: ids.task, TaskVersionID: ids.taskVersion,
				RunID: ids.run, Ordinal: int32(index + 1), EvidenceKind: link.Kind,
				ArtifactID: artifactID, VerificationCheckID: checkID, EvidenceSha256: link.SHA256,
				CreatedAt: timestamp(command.CompletedAt),
			}); err != nil {
				return err
			}
		}
		if err := queries.InsertCompletionClaim(ctx, postgres.InsertCompletionClaimParams{
			CompletionID: ids.completion, ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.taskVersion, RunID: ids.run, ClaimID: claimID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func appendCompletionEvents(
	ctx context.Context,
	queries *postgres.Queries,
	ids terminalIDs,
	command TerminalCommand,
	taskVersion, runVersion, workspaceVersion, leaseVersion int64,
) error {
	payload, err := json.Marshal(struct {
		SchemaVersion   string `json:"schema_version"`
		CompletionID    string `json:"completion_id"`
		OperationID     string `json:"operation_id"`
		PreflightSHA256 string `json:"preflight_sha256"`
		CapsuleSHA256   string `json:"capsule_sha256"`
		ManifestSHA256  string `json:"manifest_sha256"`
		LeaseVersion    int64  `json:"released_lease_version"`
	}{
		SchemaVersion: "revolvr-completion-result-event-v1", CompletionID: command.CompletionID,
		OperationID: command.OperationID, PreflightSHA256: command.Preflight.SHA256,
		CapsuleSHA256:  command.Materialized.EvidenceJSON.SHA256,
		ManifestSHA256: command.Materialized.Manifest.SHA256, LeaseVersion: leaseVersion,
	})
	if err != nil {
		return err
	}
	events := []postgres.AppendEventParams{
		{ID: newUUID(), ProjectID: ids.project, TaskID: ids.task, RunID: ids.run, EventType: "completion.completed", AggregateType: "completion", AggregateID: ids.completion, AggregateVersion: 1, Payload: payload, CreatedAt: timestamp(command.CompletedAt)},
		{ID: newUUID(), ProjectID: ids.project, TaskID: ids.task, RunID: ids.run, EventType: "task.completed", AggregateType: "task", AggregateID: ids.task, AggregateVersion: taskVersion, Payload: payload, CreatedAt: timestamp(command.CompletedAt)},
		{ID: newUUID(), ProjectID: ids.project, TaskID: ids.task, RunID: ids.run, EventType: "run.completed", AggregateType: "run", AggregateID: ids.run, AggregateVersion: runVersion, Payload: payload, CreatedAt: timestamp(command.CompletedAt)},
		{ID: newUUID(), ProjectID: ids.project, TaskID: ids.task, RunID: ids.run, EventType: "workspace.completed", AggregateType: "workspace", AggregateID: ids.workspace, AggregateVersion: workspaceVersion, Payload: payload, CreatedAt: timestamp(command.CompletedAt)},
	}
	for _, event := range events {
		if _, err := queries.AppendEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func compareExisting(existing postgres.CoreCompletion, command TerminalCommand) error {
	trajectoryHash, _ := evidence.Hash(command.Preflight.Snapshot.Trajectory)
	if uuidString(existing.ID) != command.CompletionID || existing.OperationID != command.OperationID ||
		uuidString(existing.ProjectID) != command.Preflight.Snapshot.Identity.ProjectID ||
		uuidString(existing.TaskID) != command.Preflight.Snapshot.Identity.TaskID ||
		uuidString(existing.TaskVersionID) != command.Preflight.Snapshot.Identity.TaskVersionID ||
		uuidString(existing.RunID) != command.Preflight.Snapshot.Identity.RunID ||
		uuidString(existing.WorkspaceID) != command.Preflight.Snapshot.Identity.WorkspaceID ||
		existing.PreflightSha256 != command.Preflight.SHA256 || existing.EvidenceSha256 != command.Materialized.EvidenceJSON.SHA256 ||
		existing.MarkdownSha256 != command.Materialized.Markdown.SHA256 || existing.ManifestSha256 != command.Materialized.Manifest.SHA256 ||
		existing.TrajectorySha256 != trajectoryHash || existing.HarnessAssetSetSha256 != command.Preflight.Snapshot.HarnessAssets.ManifestSHA256 {
		return ErrAlreadyCompleted
	}
	return nil
}

func injectPersistence(failure PersistenceFailureInjector, point PersistenceFailurePoint) error {
	if failure == nil {
		return nil
	}
	return failure(point)
}

func parseUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID %q: %w", value, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func newUUID() pgtype.UUID {
	parsed := uuid.MustParse(id.New())
	return pgtype.UUID{Bytes: parsed, Valid: true}
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
