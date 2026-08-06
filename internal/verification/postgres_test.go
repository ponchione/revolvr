package verification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	storage "revolvr/internal/storage/postgres"
)

func TestPostgresVerificationAtomicPersistenceReuseAndRollback(t *testing.T) {
	fixture := newVerificationPostgresFixture(t)
	store, err := NewPostgresStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.persistedRun(t, OutcomePassed, time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC))
	if _, err := fixture.pool.Exec(fixture.ctx, `INSERT INTO core.events (
		id, project_id, task_id, run_id, event_type, aggregate_type, aggregate_id,
		aggregate_version, payload, created_at
	) VALUES ($1,$2,$3,$4,'verification.rollback_fixture','verification_run',$5,1,'{}',$6)`,
		first.EventID, fixture.projectID, fixture.taskID, fixture.runID, first.ID, first.CompletedAt); err != nil {
		t.Fatal(err)
	}
	rollbackSHA := first.Checks[0].Stdout.SHA256
	if err := store.Persist(fixture.ctx, first); err == nil {
		t.Fatal("Persist rollback fixture error = nil")
	}
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM core.verification_runs WHERE id=$1`, first.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back verification runs = %d, %v", count, err)
	}
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM core.artifacts WHERE sha256=$1`, rollbackSHA).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back verification artifacts = %d, %v", count, err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `DELETE FROM core.events WHERE aggregate_type='verification_run' AND aggregate_id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Persist(fixture.ctx, first); err != nil {
		t.Fatal(err)
	}
	reusable, found, err := store.FindReusable(fixture.ctx, first.Checks[0].ExecutionFingerprint)
	if err != nil || !found || reusable.ID != first.Checks[0].ID || reusable.Outcome != OutcomePassed {
		t.Fatalf("FindReusable = %#v, %v, %v", reusable, found, err)
	}

	reuseAt := first.CompletedAt.Add(time.Hour)
	reuse := fixture.persistedRun(t, OutcomePassedReused, reuseAt)
	reuse.Checks[0].ReusedFromCheckID = reusable.ID
	reuse.Checks[0].OriginalExecutedAt = reusable.OriginalExecutedAt
	reuse.Checks[0].Stdout = reusable.Stdout
	reuse.Checks[0].Stderr = reusable.Stderr
	reuse.Checks[0].ParsedResult = reusable.ParsedResult
	reuse.Checks[0].SandboxEvidence = reusable.SandboxEvidence
	reuse.Checks[0].SandboxSpecificationSHA256 = reusable.SandboxSpecificationSHA256
	forged := reuse
	forged.ID, forged.EventID = uuid.NewString(), uuid.NewString()
	forged.Checks = append([]PersistedCheck(nil), reuse.Checks...)
	forged.Checks[0].ID = uuid.NewString()
	forged.Checks[0].ExitCode = pointer(99)
	if err := store.Persist(fixture.ctx, forged); err == nil {
		t.Fatal("forged reuse was accepted")
	}
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM core.verification_runs WHERE id=$1`, forged.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("forged reuse rollback count = %d, %v", count, err)
	}
	if err := store.Persist(fixture.ctx, reuse); err != nil {
		t.Fatal(err)
	}
	var reusedFrom pgtype.UUID
	var originalExecutedAt, occurredAt time.Time
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT reused_from_check_id, original_executed_at, occurred_at FROM core.verification_checks WHERE id=$1`, reuse.Checks[0].ID).Scan(&reusedFrom, &originalExecutedAt, &occurredAt); err != nil {
		t.Fatal(err)
	}
	if uuidString(reusedFrom) != reusable.ID || !originalExecutedAt.Equal(reusable.OriginalExecutedAt) || !occurredAt.Equal(reuseAt) || !occurredAt.After(originalExecutedAt) {
		t.Fatalf("reuse linkage/timestamps = %s %s %s", uuidString(reusedFrom), originalExecutedAt, occurredAt)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.verification_checks SET occurred_at=occurred_at + interval '1 second' WHERE id=$1`, reuse.Checks[0].ID); err == nil {
		t.Fatal("immutable reuse occurrence accepted an update")
	}

	nonreusableOutcomes := []struct {
		outcome Outcome
		status  RunStatus
	}{
		{OutcomeCancelled, RunCancelled},
		{OutcomeIncomplete, RunIncomplete},
		{OutcomeInfrastructureFailed, RunInfrastructureFailed},
		{OutcomeAmbiguous, RunAmbiguous},
	}
	for index, excluded := range nonreusableOutcomes {
		nonreusable := fixture.persistedRun(t, excluded.outcome, reuseAt.Add(time.Duration(index+1)*time.Hour))
		nonreusable.Status = excluded.status
		nonreusable.Checks[0].Cancelled = excluded.outcome == OutcomeCancelled
		if err := store.Persist(fixture.ctx, nonreusable); err != nil {
			t.Fatal(err)
		}
		got, found, err := store.FindReusable(fixture.ctx, first.Checks[0].ExecutionFingerprint)
		if err != nil || !found || got.ID != first.Checks[0].ID {
			t.Fatalf("%s terminal result displaced reusable original = %#v, %v, %v", excluded.outcome, got, found, err)
		}
	}
	changed := fixture.pinned
	changed.Plan.Gates = append([]Gate(nil), fixture.pinned.Plan.Gates...)
	changed.PlanSHA256 = ""
	changed.Plan.Gates[0].Parser.Version = "parser-v2"
	changed, err = Pin(changed)
	if err != nil {
		t.Fatal(err)
	}
	changedFingerprint, _ := ExecutionFingerprint(changed, changed.Plan.Gates[0])
	if _, found, err := store.FindReusable(fixture.ctx, changedFingerprint); err != nil || found {
		t.Fatalf("materially changed fingerprint reuse = %v, %v", found, err)
	}

	failure := fixture.persistedRun(t, OutcomeFailed, reuseAt.Add(5*time.Hour))
	failure.Status = RunFailed
	failure.Checks[0].ExitCode = pointer(1)
	if err := store.Persist(fixture.ctx, failure); err != nil {
		t.Fatal(err)
	}
	reusableFailure, found, err := store.FindReusable(fixture.ctx, failure.Checks[0].ExecutionFingerprint)
	if err != nil || !found || reusableFailure.ID != failure.Checks[0].ID || reusableFailure.Outcome != OutcomeFailed {
		t.Fatalf("reusable failure = %#v, %v, %v", reusableFailure, found, err)
	}
	failureReuseAt := reuseAt.Add(6 * time.Hour)
	failureReuse := fixture.persistedRun(t, OutcomeUnchangedFailureReused, failureReuseAt)
	failureReuse.Status = RunFailed
	failureReuse.Checks[0].ReusedFromCheckID = reusableFailure.ID
	failureReuse.Checks[0].OriginalExecutedAt = reusableFailure.OriginalExecutedAt
	failureReuse.Checks[0].ExitCode = reusableFailure.ExitCode
	failureReuse.Checks[0].Stdout = reusableFailure.Stdout
	failureReuse.Checks[0].Stderr = reusableFailure.Stderr
	failureReuse.Checks[0].ParsedResult = reusableFailure.ParsedResult
	failureReuse.Checks[0].SandboxEvidence = reusableFailure.SandboxEvidence
	failureReuse.Checks[0].FailureSignatures = reusableFailure.FailureSignatures
	if err := store.Persist(fixture.ctx, failureReuse); err != nil {
		t.Fatal(err)
	}
	var failureOutcome, failureRunStatus string
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT c.outcome,r.status FROM core.verification_checks c JOIN core.verification_runs r ON r.id=c.verification_run_id WHERE c.id=$1`, failureReuse.Checks[0].ID).Scan(&failureOutcome, &failureRunStatus); err != nil || failureOutcome != string(OutcomeUnchangedFailureReused) || failureRunStatus != string(RunFailed) {
		t.Fatalf("reused failure persisted as %q/%q, %v", failureOutcome, failureRunStatus, err)
	}
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT count(*) FROM core.events WHERE aggregate_type='verification_run' AND run_id=$1`, fixture.runID).Scan(&count); err != nil || count != 8 {
		t.Fatalf("verification result events = %d, %v", count, err)
	}
}

type verificationPostgresFixture struct {
	ctx              context.Context
	pool             *pgxpool.Pool
	projectID        string
	sourceID         string
	taskID           string
	taskVersionID    string
	runID            string
	workspaceID      string
	sourceArtifactID string
	candidate        SourceIdentity
	pinned           PinnedPlan
	artifactPrefix   string
}

func newVerificationPostgresFixture(t *testing.T) *verificationPostgresFixture {
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
	f := &verificationPostgresFixture{
		ctx: ctx, pool: pool, projectID: uuid.NewString(), sourceID: uuid.NewString(),
		taskID: uuid.NewString(), taskVersionID: uuid.NewString(), runID: uuid.NewString(),
		workspaceID: uuid.NewString(), sourceArtifactID: uuid.NewString(),
		candidate:      SourceIdentity{Commit: strings.Repeat("c", 40), Tree: strings.Repeat("d", 40)},
		artifactPrefix: "verification-test/" + uuid.NewString(),
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	queries := storage.New(pool)
	projectID, _ := parseUUID(f.projectID)
	sourceID, _ := parseUUID(f.sourceID)
	taskID, _ := parseUUID(f.taskID)
	taskVersionID, _ := parseUUID(f.taskVersionID)
	runID, _ := parseUUID(f.runID)
	artifactID, _ := parseUUID(f.sourceArtifactID)
	if _, err := queries.InsertProject(ctx, storage.InsertProjectParams{ID: projectID, Name: "verification-" + uuid.NewString(), Status: "active", CreatedAt: timestamp(now), UpdatedAt: timestamp(now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertProjectSource(ctx, storage.InsertProjectSourceParams{ID: sourceID, ProjectID: projectID, CanonicalSourcePath: "/fixture/source/" + uuid.NewString(), ManagedRepositoryPath: "/fixture/managed/" + uuid.NewString(), CurrentCommit: strings.Repeat("a", 40), CurrentTree: strings.Repeat("b", 40), DirtyState: []byte(`{}`), Remotes: []byte(`[]`)}); err != nil {
		t.Fatal(err)
	}
	sourceBytes := []byte(uuid.NewString())
	if _, err := queries.InsertArtifact(ctx, storage.InsertArtifactParams{ID: artifactID, Sha256: hashBytes(sourceBytes), SizeBytes: int64(len(sourceBytes)), MediaType: "application/json", LogicalKind: "verification_test_task", StoragePath: f.artifactPrefix + "/task", CreatedAt: timestamp(now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTask(ctx, storage.InsertTaskParams{ID: taskID, ProjectID: projectID, ExternalTaskID: "verification-" + strings.ToLower(uuid.NewString()), Status: "draft", CreatedAt: timestamp(now), UpdatedAt: timestamp(now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTaskVersion(ctx, storage.InsertTaskVersionParams{ID: taskVersionID, TaskID: taskID, VersionNumber: 1, SourceArtifactID: artifactID, Title: "Verification persistence", Goal: "Exercise atomic verifier persistence", RiskClass: "high", MutationClass: "architecture_change", NetworkProfile: "none", Priority: 1, Scope: []byte(`[]`), ExcludedScope: []byte(`[]`), VerificationPlan: []byte(`[]`), Budget: []byte(`{}`), SecretRequirements: []byte(`[]`), ExpectedPaths: []byte(`[]`), OperatorCheckpoints: []byte(`[]`), CreatedAt: timestamp(now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE core.tasks SET accepted_version_id=$1,status='working',aggregate_version=2,updated_at=$2 WHERE id=$3`, f.taskVersionID, now, f.taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertRun(ctx, storage.InsertRunParams{ID: runID, ProjectID: projectID, TaskID: taskID, TaskVersionID: taskVersionID, ProjectSourceID: sourceID, AdmittedTaskAggregateVersion: 2, SourceCommit: strings.Repeat("a", 40), SourceTree: strings.Repeat("b", 40), CoordinatorIdentity: "verification-test", CreatedAt: timestamp(now)}); err != nil {
		t.Fatal(err)
	}
	identity := `{"fixture":true}`
	if _, err := pool.Exec(ctx, `INSERT INTO core.workspaces (
		id,run_id,project_id,project_source_id,task_id,creation_operation_id,symbolic_source_id,status,
		original_checkout_path,managed_repository_path,workspace_root,workspace_path,branch_ref,
		source_commit,source_tree,workspace_device,workspace_inode,original_identity_before,
		original_identity_after,candidate_commit,candidate_tree,created_at,updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,'frozen',$8,$9,$10,$11,$12,$13,$14,1,1,$15,$15,$16,$17,$18,$18)`,
		f.workspaceID, f.runID, f.projectID, f.sourceID, f.taskID, "verification-create-"+uuid.NewString(),
		"verification-"+strings.ToLower(uuid.NewString()), "/fixture/original/"+uuid.NewString(),
		"/fixture/managed/"+uuid.NewString(), "/fixture/workspaces/"+uuid.NewString(),
		"/fixture/workspaces/"+uuid.NewString(), "refs/heads/revolvr/workspaces/verification-"+strings.ToLower(uuid.NewString()),
		strings.Repeat("a", 40), strings.Repeat("b", 40), identity, f.candidate.Commit, f.candidate.Tree, now); err != nil {
		t.Fatal(err)
	}
	f.pinned = fixturePinned(t, TierFocused)
	f.pinned.ProjectID, f.pinned.TaskID, f.pinned.TaskVersionID = f.projectID, f.taskID, f.taskVersionID
	f.pinned.RunID, f.pinned.WorkspaceID, f.pinned.Candidate = f.runID, f.workspaceID, f.candidate
	f.pinned.Plan.Gates[0].Source = f.candidate
	f.pinned.Plan.VerificationPlanSHA256 = hashBytes([]byte(`[]`))
	f.pinned.PlanSHA256 = ""
	f.pinned, err = Pin(f.pinned)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

func (f *verificationPostgresFixture) persistedRun(t *testing.T, outcome Outcome, occurred time.Time) PersistedRun {
	t.Helper()
	fingerprint, err := ExecutionFingerprint(f.pinned, f.pinned.Plan.Gates[0])
	if err != nil {
		t.Fatal(err)
	}
	exit := 0
	stdout := []byte("verification stdout " + uuid.NewString())
	stderr := []byte("verification stderr " + uuid.NewString())
	check := PersistedCheck{
		ID: uuid.NewString(), Ordinal: 1, Gate: f.pinned.Plan.Gates[0], Outcome: outcome,
		ExecutionFingerprint: fingerprint, VerifierProtocolVersion: f.pinned.VerifierProtocol,
		VerifierImplementationVersion: f.pinned.VerifierImplementation,
		SandboxSpecificationSHA256:    strings.Repeat("7", 64), ExitCode: &exit,
		Stdout:             Artifact{ID: uuid.NewString(), SHA256: hashBytes(stdout), SizeBytes: int64(len(stdout)), MediaType: "application/octet-stream", LogicalKind: "verification-stdout", StoragePath: f.artifactPrefix + "/" + uuid.NewString(), Content: stdout},
		Stderr:             Artifact{ID: uuid.NewString(), SHA256: hashBytes(stderr), SizeBytes: int64(len(stderr)), MediaType: "application/octet-stream", LogicalKind: "verification-stderr", StoragePath: f.artifactPrefix + "/" + uuid.NewString(), Content: stderr},
		ParsedResult:       parsedFixture("gate:focused", map[bool]string{true: "failed", false: "passed"}[outcome.Failed()]),
		SandboxEvidence:    json.RawMessage(`{"runtime":"postgres-fixture"}`),
		OriginalExecutedAt: occurred, OccurredAt: occurred, StartedAt: occurred.Add(-time.Second), CompletedAt: occurred,
	}
	if outcome.Failed() {
		check.FailureSignatures = []string{"gate:focused"}
	}
	return PersistedRun{ID: uuid.NewString(), EventID: uuid.NewString(), Pinned: f.pinned, Purpose: PurposeCandidate, Status: aggregateStatus([]PersistedCheck{check}), Checks: []PersistedCheck{check}, Differential: Differential{}, StartedAt: occurred.Add(-2 * time.Second), CompletedAt: occurred}
}

func (f *verificationPostgresFixture) cleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	statements := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM core.events WHERE project_id=$1`, []any{f.projectID}},
		{`DELETE FROM core.verification_checks WHERE run_id=$1`, []any{f.runID}},
		{`DELETE FROM core.verification_runs WHERE run_id=$1`, []any{f.runID}},
		{`DELETE FROM core.workspaces WHERE run_id=$1`, []any{f.runID}},
		{`DELETE FROM core.runs WHERE id=$1`, []any{f.runID}},
		{`UPDATE core.tasks SET accepted_version_id=NULL,status='draft' WHERE id=$1`, []any{f.taskID}},
		{`DELETE FROM core.task_versions WHERE task_id=$1`, []any{f.taskID}},
		{`DELETE FROM core.tasks WHERE id=$1`, []any{f.taskID}},
		{`DELETE FROM core.artifacts WHERE storage_path LIKE $1`, []any{f.artifactPrefix + "%"}},
		{`DELETE FROM core.project_sources WHERE id=$1`, []any{f.sourceID}},
		{`DELETE FROM core.projects WHERE id=$1`, []any{f.projectID}},
	}
	for _, statement := range statements {
		if _, err := f.pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Errorf("cleanup %q: %v", statement.query, err)
		}
	}
}

func TestVerificationMigrationHasReversibleDownSection(t *testing.T) {
	raw, err := os.ReadFile("../../db/migrations/00009_verification.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"-- +goose Up", "-- +goose Down", "DROP TABLE core.verification_checks", "DROP TABLE core.verification_runs"} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
}

func TestArtifactHashFixtureUsesSHA256(t *testing.T) {
	raw := []byte("verification")
	sum := sha256.Sum256(raw)
	if hashBytes(raw) != hex.EncodeToString(sum[:]) {
		t.Fatal("artifact hash is not SHA-256")
	}
}
