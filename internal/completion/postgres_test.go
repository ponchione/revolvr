package completion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/evidence"
	storage "revolvr/internal/storage/postgres"
)

func TestPostgresCompletionIsAtomicImmutableAndReplaySafe(t *testing.T) {
	fixture := newCompletionPostgresFixture(t)
	store, err := NewPostgresStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	store.SetSupplementalSource(staticSupplement{snapshot: fixture.snapshot})
	key := Key{OperationID: "read-" + uuid.NewString(), Identity: fixture.snapshot.Identity}
	databaseSnapshot, err := store.ReadCompletionSnapshot(fixture.ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	fixture.snapshot = databaseSnapshot
	preflight, err := BuildPreflight(databaseSnapshot)
	if err != nil || !preflight.Accepted() {
		t.Fatalf("preflight = %#v, %v", preflight.Rejections, err)
	}
	artifactRoot := filepath.Join(t.TempDir(), "completion-artifacts")
	if err := os.Mkdir(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	artifactStore, err := evidence.NewStore(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	operationID := "completion-" + uuid.NewString()
	materialized, err := MaterializeCapsule(
		fixture.ctx, artifactStore, preflight, completionProvenance(fixture.snapshot, operationID), nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	completedAt := fixture.now.Add(5 * time.Minute)
	command := TerminalCommand{
		CompletionID: uuid.NewString(), OperationID: operationID, Preflight: preflight,
		Materialized: materialized, CompletedAt: completedAt,
	}
	forgedCapsule := command
	forgedCapsule.Materialized.Markdown.Content = append([]byte(nil), command.Materialized.Markdown.Content...)
	forgedCapsule.Materialized.Markdown.Content[0] ^= 1
	if _, err := store.CommitCompletion(fixture.ctx, forgedCapsule); !errors.Is(err, evidence.ErrArtifactDivergence) {
		t.Fatalf("forged capsule error = %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO core.plan_steps (
		plan_version_id,plan_id,step_id,ordinal,status,description,criterion_ids,
		depends_on_step_ids,expected_paths,components,test_strategy,risks,assumptions,evidence_refs,lineage
	) SELECT plan_version_id,plan_id,'unexpected-terminal-step',2,'completed',description,criterion_ids,
		depends_on_step_ids,expected_paths,components,test_strategy,risks,assumptions,evidence_refs,lineage
		FROM core.plan_steps WHERE plan_version_id=$1 AND step_id='implement'`, fixture.snapshot.Plan.VersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitCompletion(fixture.ctx, command); !errors.Is(err, ErrStalePreflight) {
		t.Fatalf("changed canonical plan error = %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `DELETE FROM core.plan_steps WHERE plan_version_id=$1 AND step_id='unexpected-terminal-step'`, fixture.snapshot.Plan.VersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.task_acceptance_criteria SET status='waived' WHERE id=$1`, fixture.snapshot.Criteria[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitCompletion(fixture.ctx, command); !errors.Is(err, ErrStalePreflight) {
		t.Fatalf("changed canonical criterion error = %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.task_acceptance_criteria SET status='passed' WHERE id=$1`, fixture.snapshot.Criteria[0].ID); err != nil {
		t.Fatal(err)
	}
	forgedSnapshot := fixture.snapshot
	forgedVerification := *fixture.snapshot.Verification
	forgedVerification.Checks = append([]VerificationCheck(nil), fixture.snapshot.Verification.Checks...)
	forgedVerification.Checks[0].ExecutionFingerprint = strings.Repeat("8", 64)
	forgedSnapshot.Verification = &forgedVerification
	forgedSnapshot.Claims = append([]evidence.Claim(nil), fixture.snapshot.Claims...)
	forgedSnapshot.Claims[0].Evidence = append([]evidence.EvidenceLink(nil), fixture.snapshot.Claims[0].Evidence...)
	forgedSnapshot.Claims[0].Evidence[0].SHA256 = forgedVerification.Checks[0].ExecutionFingerprint
	forgedPreflight, err := BuildPreflight(forgedSnapshot)
	if err != nil || !forgedPreflight.Accepted() {
		t.Fatalf("forged verification preflight = %#v, %v", forgedPreflight.Rejections, err)
	}
	forgedVerificationArtifacts, err := MaterializeCapsule(
		fixture.ctx, artifactStore, forgedPreflight, completionProvenance(forgedSnapshot, operationID), nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	forgedVerificationCommand := command
	forgedVerificationCommand.Preflight = forgedPreflight
	forgedVerificationCommand.Materialized = forgedVerificationArtifacts
	if _, err := store.CommitCompletion(fixture.ctx, forgedVerificationCommand); !errors.Is(err, ErrStalePreflight) {
		t.Fatalf("forged verification authority error = %v", err)
	}
	fixture.assertState(t, "finalizing", "active", "frozen", true, 0, 0)
	forced := errors.New("forced terminal event failure")
	store.SetFailureInjector(func(point PersistenceFailurePoint) error {
		if point == PersistenceFailureBeforeEvents {
			return forced
		}
		return nil
	})
	if _, err := store.CommitCompletion(fixture.ctx, command); !errors.Is(err, forced) {
		t.Fatalf("forced failure error = %v", err)
	}
	fixture.assertState(t, "finalizing", "active", "frozen", true, 0, 0)
	store.SetFailureInjector(nil)
	result, err := store.CommitCompletion(fixture.ctx, command)
	if err != nil || result.Replay || result.CompletionID != command.CompletionID {
		t.Fatalf("completion = %#v, %v", result, err)
	}
	fixture.assertState(t, "completed", "released", "completed", false, 1, 4)
	replay, err := store.CommitCompletion(fixture.ctx, command)
	if err != nil || !replay.Replay || replay.CompletionID != command.CompletionID {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	lookup, found, err := store.LookupCompletion(fixture.ctx, Key{OperationID: operationID, Identity: fixture.snapshot.Identity})
	if err != nil || !found || !lookup.Replay || lookup.CompletionID != command.CompletionID {
		t.Fatalf("lookup = %#v, %v, %v", lookup, found, err)
	}
	divergent := command
	divergent.Materialized.Manifest.SHA256 = strings.Repeat("0", 64)
	if _, err := store.CommitCompletion(fixture.ctx, divergent); !errors.Is(err, evidence.ErrArtifactDivergence) {
		t.Fatalf("divergent replay error = %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.completions SET preflight_sha256=$1 WHERE id=$2`, strings.Repeat("0", 64), command.CompletionID); err == nil {
		t.Fatal("immutable completion accepted an update")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.verification_runs SET status='failed' WHERE id=$1`, fixture.snapshot.Verification.ID); err == nil {
		t.Fatal("architecture-017 verification authority accepted completion-side mutation")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO core.completion_artifacts(completion_id,ordinal,artifact_id,artifact_sha256,artifact_role) VALUES($1,999,$2,$3,'supporting')`, command.CompletionID, fixture.snapshot.Artifacts[0].ID, strings.Repeat("0", 64)); err == nil {
		t.Fatal("completion artifact accepted a divergent hash")
	}
}

type staticSupplement struct{ snapshot Snapshot }

func (s staticSupplement) LoadCompletionSupplement(_ context.Context, _ *storage.Queries, _ Key) (Supplement, error) {
	return Supplement{
		Audit: s.snapshot.Audit, Findings: s.snapshot.Findings, Budget: s.snapshot.Budget,
		Invocations: s.snapshot.Invocations, Artifacts: s.snapshot.Artifacts,
		OperatorInputs: s.snapshot.OperatorInputs, Trajectory: s.snapshot.Trajectory,
		HarnessAssets: s.snapshot.HarnessAssets, Claims: s.snapshot.Claims,
	}, nil
}

type completionPostgresFixture struct {
	ctx              context.Context
	pool             *pgxpool.Pool
	snapshot         Snapshot
	now              time.Time
	sourceID         string
	sourceArtifactID string
	stdoutID         string
	stderrID         string
	artifactPrefix   string
}

func newCompletionPostgresFixture(t *testing.T) *completionPostgresFixture {
	t.Helper()
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := storage.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &completionPostgresFixture{ctx: ctx, pool: pool, snapshot: validSnapshot(t), now: time.Now().UTC().Truncate(time.Microsecond), sourceID: uuid.NewString(), sourceArtifactID: uuid.NewString(), stdoutID: uuid.NewString(), stderrID: uuid.NewString(), artifactPrefix: "completion-fixture/" + uuid.NewString()}
	f.snapshot.Source.FrozenAt = f.now
	f.snapshot.Verification.CompletedAt = f.now.Add(time.Minute)
	f.snapshot.Audit.CompletedAt = f.now.Add(2 * time.Minute)
	f.snapshot.Aggregates = Aggregates{Task: 7, Run: 3, Workspace: 5, Plan: 2, Lease: 1}
	for index := range f.snapshot.Artifacts {
		f.snapshot.Artifacts[index].StoragePath = f.artifactPrefix + "/" + f.snapshot.Artifacts[index].Kind
	}
	f.snapshot.ArtifactManifestSHA256, err = evidence.ArtifactManifestHash(f.snapshot.Artifacts)
	if err != nil {
		t.Fatal(err)
	}
	queries := storage.New(pool)
	projectID := mustUUID(t, f.snapshot.Identity.ProjectID)
	taskID := mustUUID(t, f.snapshot.Identity.TaskID)
	taskVersionID := mustUUID(t, f.snapshot.Identity.TaskVersionID)
	runID := mustUUID(t, f.snapshot.Identity.RunID)
	workspaceID := mustUUID(t, f.snapshot.Identity.WorkspaceID)
	sourceID := mustUUID(t, f.sourceID)
	sourceArtifactID := mustUUID(t, f.sourceArtifactID)
	if _, err := queries.InsertProject(ctx, storage.InsertProjectParams{ID: projectID, Name: "completion-" + uuid.NewString(), Status: "active", CreatedAt: timestamp(f.now), UpdatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertProjectSource(ctx, storage.InsertProjectSourceParams{ID: sourceID, ProjectID: projectID, CanonicalSourcePath: "/fixture/source/" + uuid.NewString(), ManagedRepositoryPath: "/fixture/managed/" + uuid.NewString(), CurrentCommit: f.snapshot.Source.BeforeCommit, CurrentTree: f.snapshot.Source.BeforeTree, DirtyState: []byte(`{}`), Remotes: []byte(`[]`)}); err != nil {
		t.Fatal(err)
	}
	insertArtifact(t, queries, sourceArtifactID, strings.Repeat("a", 64), 1, "task_source", f.artifactPrefix+"/task", f.now)
	if _, err := queries.InsertTask(ctx, storage.InsertTaskParams{ID: taskID, ProjectID: projectID, ExternalTaskID: "completion-" + strings.ToLower(uuid.NewString()), Status: "draft", CreatedAt: timestamp(f.now), UpdatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTaskVersion(ctx, storage.InsertTaskVersionParams{ID: taskVersionID, TaskID: taskID, VersionNumber: 1, SourceArtifactID: sourceArtifactID, Title: "Completion fixture", Goal: "Complete exactly once", RiskClass: "high", MutationClass: "architecture_change", NetworkProfile: "none", Priority: 1, Scope: []byte(`[]`), ExcludedScope: []byte(`[]`), VerificationPlan: []byte(`[]`), Budget: []byte(`{}`), SecretRequirements: []byte(`[]`), ExpectedPaths: []byte(`[]`), OperatorCheckpoints: []byte(`[]`), CreatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	criterionID := mustUUID(t, f.snapshot.Criteria[0].ID)
	if _, err := queries.InsertTaskAcceptanceCriterion(ctx, storage.InsertTaskAcceptanceCriterionParams{ID: criterionID, TaskID: taskID, ExternalCriterionID: "AC-1", Status: "passed", CreatedAt: timestamp(f.now), UpdatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTaskAcceptanceVersion(ctx, storage.InsertTaskAcceptanceVersionParams{ID: mustUUID(t, uuid.NewString()), CriterionID: criterionID, TaskID: taskID, TaskVersionID: taskVersionID, VersionNumber: 1, Requirement: "Completion is exact", VerificationMethod: "command", VerificationReference: pgtype.Text{String: "tier-4", Valid: true}, CreatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE core.tasks SET accepted_version_id=$1,status='finalizing',aggregate_version=7,updated_at=$2 WHERE id=$3`, f.snapshot.Identity.TaskVersionID, f.now, f.snapshot.Identity.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertRun(ctx, storage.InsertRunParams{ID: runID, ProjectID: projectID, TaskID: taskID, TaskVersionID: taskVersionID, ProjectSourceID: sourceID, AdmittedTaskAggregateVersion: 7, SourceCommit: f.snapshot.Source.BeforeCommit, SourceTree: f.snapshot.Source.BeforeTree, CoordinatorIdentity: "completion-fixture", CreatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE core.runs SET aggregate_version=3 WHERE id=$1`, f.snapshot.Identity.RunID); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range f.snapshot.Artifacts {
		insertArtifact(t, queries, mustUUID(t, artifact.ID), artifact.SHA256, artifact.SizeBytes, artifact.Kind, artifact.StoragePath, f.now)
	}
	stdoutHash, stderrHash := strings.Repeat("6", 64), strings.Repeat("7", 64)
	insertArtifact(t, queries, mustUUID(t, f.stdoutID), stdoutHash, 1, "verification_stdout", f.artifactPrefix+"/stdout", f.now)
	insertArtifact(t, queries, mustUUID(t, f.stderrID), stderrHash, 1, "verification_stderr", f.artifactPrefix+"/stderr", f.now)
	identityJSON := []byte(`{"fixture":true}`)
	if _, err := pool.Exec(ctx, `INSERT INTO core.workspaces (
		id,run_id,project_id,project_source_id,task_id,creation_operation_id,symbolic_source_id,status,aggregate_version,
		original_checkout_path,managed_repository_path,workspace_root,workspace_path,branch_ref,
		source_commit,source_tree,workspace_device,workspace_inode,original_identity_before,original_identity_after,
		diff_artifact_id,diff_sha256,candidate_commit,candidate_tree,created_at,updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,'frozen',5,$8,$9,$10,$11,$12,$13,$14,1,1,$15,$15,$16,$17,$18,$19,$20,$20)`,
		f.snapshot.Identity.WorkspaceID, f.snapshot.Identity.RunID, f.snapshot.Identity.ProjectID, f.sourceID, f.snapshot.Identity.TaskID,
		"create-"+uuid.NewString(), "completion-"+strings.ToLower(uuid.NewString()), "/fixture/original/"+uuid.NewString(),
		"/fixture/managed/"+uuid.NewString(), "/fixture/root/"+uuid.NewString(), "/fixture/workspace/"+uuid.NewString(),
		"refs/heads/revolvr/workspaces/completion-"+strings.ToLower(uuid.NewString()), f.snapshot.Source.BeforeCommit, f.snapshot.Source.BeforeTree,
		identityJSON, f.snapshot.Artifacts[2].ID, f.snapshot.Source.DiffSHA256, f.snapshot.Source.AfterCommit, f.snapshot.Source.AfterTree, f.now); err != nil {
		t.Fatal(err)
	}
	planID := mustUUID(t, f.snapshot.Plan.ID)
	planVersionID := mustUUID(t, f.snapshot.Plan.VersionID)
	if _, err := queries.InsertPlan(ctx, storage.InsertPlanParams{ID: planID, ProjectID: projectID, TaskID: taskID, TaskVersionID: taskVersionID, RunID: runID, ProjectSourceID: sourceID, SourceRevision: strings.Repeat("8", 64), SourceCommit: f.snapshot.Source.AfterCommit, SourceTree: f.snapshot.Source.AfterTree, CreatedAt: timestamp(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertPlanVersion(ctx, fixturePlanVersion(planVersionID, planID, taskID, taskVersionID, runID, sourceID, f.snapshot.Plan.SHA256, f.now)); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertPlanStep(ctx, storage.InsertPlanStepParams{PlanVersionID: planVersionID, PlanID: planID, StepID: "implement", Ordinal: 1, Status: "completed", Description: "Implement completion", CriterionIds: []byte(`["AC-1"]`), DependsOnStepIds: []byte(`[]`), ExpectedPaths: []byte(`["internal/completion"]`), Components: []byte(`["completion"]`), TestStrategy: []byte(`[{"command":"go test"}]`), Risks: []byte(`[]`), Assumptions: []byte(`[]`), EvidenceRefs: []byte(`["task"]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE core.plans SET accepted_version_id=$1,accepted_operation_id=$2,accepted_by='trusted_host',accepted_at=$3,aggregate_version=2,updated_at=$3 WHERE id=$4`, f.snapshot.Plan.VersionID, "accept-"+uuid.NewString(), f.now, f.snapshot.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertVerificationRun(ctx, storage.InsertVerificationRunParams{ID: mustUUID(t, f.snapshot.Verification.ID), ProjectID: projectID, TaskID: taskID, TaskVersionID: taskVersionID, RunID: runID, WorkspaceID: workspaceID, Purpose: "final", Status: "passed", PlanSchemaVersion: "v1", PlanVersion: "v1", PlanSha256: strings.Repeat("1", 64), PinnedPlan: []byte(`{}`), CandidateCommit: f.snapshot.Source.AfterCommit, CandidateTree: f.snapshot.Source.AfterTree, ProjectEnvironmentSha256: strings.Repeat("2", 64), ProjectEnvironment: []byte(`{}`), Differential: []byte(`{}`), StartedAt: timestamp(f.now), CompletedAt: timestamp(f.snapshot.Verification.CompletedAt), CreatedAt: timestamp(f.snapshot.Verification.CompletedAt)}); err != nil {
		t.Fatal(err)
	}
	check := f.snapshot.Verification.Checks[0]
	if _, err := queries.InsertVerificationCheck(ctx, storage.InsertVerificationCheckParams{ID: mustUUID(t, check.ID), VerificationRunID: mustUUID(t, f.snapshot.Verification.ID), RunID: runID, Ordinal: 1, GateID: "tier-4", Tier: 4, Outcome: "passed", ExecutionFingerprint: check.ExecutionFingerprint, VerifierProtocolVersion: "v1", VerifierImplementationVersion: "v1", ParserKind: "none", ParserVersion: "v1", SourceCommit: f.snapshot.Source.AfterCommit, SourceTree: f.snapshot.Source.AfterTree, CommandArgv: []byte(`["go","test","./..."]`), WorkingDirectory: "/workspace", Environment: []byte(`[]`), ImageReference: "fixture@" + f.snapshot.Verification.ImageDigest, ImageDigest: f.snapshot.Verification.ImageDigest, SandboxProfile: "strict", SandboxProfileSha256: f.snapshot.Verification.ProfileSHA256, SandboxSpecificationSha256: strings.Repeat("3", 64), AuthorityInputs: []byte(`[]`), OutputPolicy: []byte(`{}`), ExitCode: pgtype.Int4{Int32: 0, Valid: true}, StdoutArtifactID: mustUUID(t, f.stdoutID), StderrArtifactID: mustUUID(t, f.stderrID), ParsedResult: []byte(`{}`), SandboxEvidence: []byte(`{}`), FailureSignatures: []byte(`[]`), OriginalExecutedAt: timestamp(f.snapshot.Verification.CompletedAt), OccurredAt: timestamp(f.snapshot.Verification.CompletedAt), StartedAt: timestamp(f.now), CompletedAt: timestamp(f.snapshot.Verification.CompletedAt), CreatedAt: timestamp(f.snapshot.Verification.CompletedAt)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE core.execution_leases SET run_id=$1,coordinator_identity='completion-fixture',acquired_at=$2,aggregate_version=1 WHERE lease_name='global-source-mutation-v1'`, f.snapshot.Identity.RunID, f.now); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

func fixturePlanVersion(id, planID, taskID, taskVersionID, runID, sourceID pgtype.UUID, contentSHA string, at time.Time) storage.InsertPlanVersionParams {
	return storage.InsertPlanVersionParams{ID: id, PlanID: planID, TaskID: taskID, TaskVersionID: taskVersionID, RunID: runID, ProjectSourceID: sourceID, RevisionNumber: 1, CandidateSha256: strings.Repeat("4", 64), ContentSha256: contentSHA, ChangeExplanation: "initial exact plan", SourceRevision: strings.Repeat("8", 64), SupervisorDecisionID: "decision", SupervisorDecisionSha256: strings.Repeat("5", 64), DossierVersion: "v1", DossierSha256: strings.Repeat("6", 64), DossierContent: []byte(`{}`), PromptVersion: "v1", PromptSha256: strings.Repeat("7", 64), PromptContent: []byte("prompt"), ResponseSchemaVersion: "v1", ResponseSchemaSha256: strings.Repeat("8", 64), ResponseSchema: []byte(`{}`), ModelPolicyVersion: "v1", ModelPolicySha256: strings.Repeat("9", 64), ModelPolicy: []byte(`{}`), HostPolicyVersion: "v1", HostPolicySha256: strings.Repeat("a", 64), HostPolicy: []byte(`{}`), ExpectedRequest: []byte(`{}`), ModelResult: []byte(`{}`), RawOutput: []byte(`{}`), CanonicalOutput: []byte(`{}`), CreatedAt: timestamp(at)}
}

func insertArtifact(t *testing.T, queries *storage.Queries, artifactID pgtype.UUID, sha string, size int64, kind, path string, at time.Time) {
	t.Helper()
	if _, err := queries.InsertArtifact(context.Background(), storage.InsertArtifactParams{ID: artifactID, Sha256: sha, SizeBytes: size, MediaType: "application/octet-stream", LogicalKind: kind, StoragePath: path, CreatedAt: timestamp(at)}); err != nil {
		t.Fatal(err)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func (f *completionPostgresFixture) assertState(t *testing.T, task, run, workspace string, leaseHeld bool, completions, events int) {
	t.Helper()
	var gotTask, gotRun, gotWorkspace string
	var leaseRun pgtype.UUID
	var gotCompletions, gotEvents int
	if err := f.pool.QueryRow(f.ctx, `SELECT status FROM core.tasks WHERE id=$1`, f.snapshot.Identity.TaskID).Scan(&gotTask); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT status FROM core.runs WHERE id=$1`, f.snapshot.Identity.RunID).Scan(&gotRun); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT status FROM core.workspaces WHERE id=$1`, f.snapshot.Identity.WorkspaceID).Scan(&gotWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT run_id FROM core.execution_leases WHERE lease_name='global-source-mutation-v1'`).Scan(&leaseRun); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM core.completions WHERE task_id=$1`, f.snapshot.Identity.TaskID).Scan(&gotCompletions); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM core.events WHERE aggregate_type IN ('completion','task','run','workspace') AND task_id=$1`, f.snapshot.Identity.TaskID).Scan(&gotEvents); err != nil {
		t.Fatal(err)
	}
	if gotTask != task || gotRun != run || gotWorkspace != workspace || leaseRun.Valid != leaseHeld || gotCompletions != completions || gotEvents != events {
		t.Fatalf("state = task=%s run=%s workspace=%s lease=%v completions=%d events=%d", gotTask, gotRun, gotWorkspace, leaseRun.Valid, gotCompletions, gotEvents)
	}
}

func (f *completionPostgresFixture) cleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	statements := []string{
		`DELETE FROM core.events WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.completion_claims WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.completion_artifacts WHERE completion_id IN (SELECT id FROM core.completions WHERE task_id='` + f.snapshot.Identity.TaskID + `')`,
		`DELETE FROM core.completions WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.claim_evidence WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.claims WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`WITH owned AS (DELETE FROM core.artifact_provenance WHERE task_id='` + f.snapshot.Identity.TaskID + `' RETURNING artifact_id) DELETE FROM core.artifacts WHERE id IN (SELECT artifact_id FROM owned)`,
		`DELETE FROM core.verification_checks WHERE run_id='` + f.snapshot.Identity.RunID + `'`,
		`DELETE FROM core.verification_runs WHERE run_id='` + f.snapshot.Identity.RunID + `'`,
		`DELETE FROM core.plan_steps WHERE plan_id='` + f.snapshot.Plan.ID + `'`,
		`UPDATE core.plans SET accepted_version_id=NULL,accepted_operation_id=NULL,accepted_by=NULL,accepted_at=NULL WHERE id='` + f.snapshot.Plan.ID + `'`,
		`DELETE FROM core.plan_versions WHERE plan_id='` + f.snapshot.Plan.ID + `'`,
		`DELETE FROM core.plans WHERE id='` + f.snapshot.Plan.ID + `'`,
		`DELETE FROM core.workspaces WHERE run_id='` + f.snapshot.Identity.RunID + `'`,
		`UPDATE core.execution_leases SET run_id=NULL,coordinator_identity=NULL,acquired_at=NULL WHERE run_id='` + f.snapshot.Identity.RunID + `'`,
		`DELETE FROM core.runs WHERE id='` + f.snapshot.Identity.RunID + `'`,
		`UPDATE core.tasks SET accepted_version_id=NULL,status='draft' WHERE id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.task_acceptance_versions WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.task_acceptance_criteria WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.task_versions WHERE task_id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.tasks WHERE id='` + f.snapshot.Identity.TaskID + `'`,
		`DELETE FROM core.artifacts WHERE storage_path LIKE '` + f.artifactPrefix + `/%'`,
		`DELETE FROM core.project_sources WHERE id='` + f.sourceID + `'`,
		`DELETE FROM core.projects WHERE id='` + f.snapshot.Identity.ProjectID + `'`,
	}
	for _, statement := range statements {
		if _, err := f.pool.Exec(ctx, statement); err != nil {
			t.Errorf("cleanup %q: %v", statement, err)
		}
	}
}

func TestCompletionMigrationHasReversibleDownSection(t *testing.T) {
	raw, err := os.ReadFile("../../db/migrations/00010_evidence_completion.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"-- +goose Up", "-- +goose Down", "DROP TABLE core.completions", "DROP TABLE core.claims", "DROP TABLE core.artifact_provenance"} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
}
