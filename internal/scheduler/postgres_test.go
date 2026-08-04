package scheduler

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/storage/postgres"
)

func TestPostgresSelectsAndPinsCanonicalTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newSchedulerFixture(t, ctx)
	created := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	completed := fixture.addTask(t, ctx, "completed", "completed", 1, created)
	selectedTask := fixture.addTask(t, ctx, "selected", "pending", 10, created)
	fixture.addDependency(t, ctx, selectedTask, completed)
	fixture.addTask(t, ctx, "later", "pending", 10, created.Add(time.Second))

	selected, err := Select(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	if selected.TaskID != uuidString(selectedTask.id) || selected.TaskVersionID != uuidString(selectedTask.versionID) ||
		selected.ProjectID != uuidString(fixture.projectID) || selected.ProjectSourceID != uuidString(fixture.sourceID) ||
		selected.ExpectedAggregateVersion != 4 || selected.SourceCommit != fixture.commit || selected.SourceTree != fixture.tree {
		t.Fatalf("selected candidate = %#v", selected)
	}

	changed := selected
	changed.TaskID = uuidString(completed.id)
	if _, err := Admit(ctx, fixture.pool, AdmissionCommand{RunID: id.New(), CoordinatorIdentity: "coordinator", Candidate: changed}); !errors.Is(err, ErrConflict) {
		t.Fatalf("substituted admission error = %v, want %v", err, ErrConflict)
	}

	runID := id.New()
	command := AdmissionCommand{RunID: runID, CoordinatorIdentity: "coordinator", Candidate: selected}
	admitted, err := Admit(ctx, fixture.pool, command)
	if err != nil {
		t.Fatal(err)
	}
	if admitted.RunID != runID || admitted.TaskAggregateVersion != 5 || admitted.LeaseVersion < 1 || admitted.RunEventID == "" || admitted.TaskEventID == "" {
		t.Fatalf("admission = %#v", admitted)
	}
	replayed, err := Admit(ctx, fixture.pool, command)
	if err != nil || !replayed.Replayed || replayed.RunID != runID {
		t.Fatalf("replayed admission = %#v, err = %v", replayed, err)
	}
	if _, err := Select(ctx, fixture.pool); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("selection with active lease error = %v, want %v", err, ErrLeaseBusy)
	}

	var taskStatus, runStatus string
	var taskVersion, leaseRun pgtype.UUID
	var taskAggregate, eventCount int64
	if err := fixture.pool.QueryRow(ctx, `SELECT t.status, t.aggregate_version, t.accepted_version_id,
        r.status, l.run_id,
        (SELECT count(*) FROM core.events WHERE run_id = r.id)
        FROM core.tasks AS t
        JOIN core.runs AS r ON r.task_id = t.id
        JOIN core.execution_leases AS l ON l.lease_name = 'global-source-mutation-v1'
        WHERE r.id = $1`, mustUUID(t, runID)).Scan(
		&taskStatus, &taskAggregate, &taskVersion, &runStatus, &leaseRun, &eventCount,
	); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "admitted" || taskAggregate != 5 || taskVersion != selectedTask.versionID ||
		runStatus != "active" || uuidString(leaseRun) != runID || eventCount != 2 {
		t.Fatalf("stored admission = task %s/%d/%s run %s lease %s events %d", taskStatus, taskAggregate, uuidString(taskVersion), runStatus, uuidString(leaseRun), eventCount)
	}
}

func TestPostgresConcurrentAdmissionHasOneWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newSchedulerFixture(t, ctx)
	fixture.addTask(t, ctx, "concurrent", "pending", 1, time.Now().UTC())
	candidate, err := Select(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			_, err := Admit(ctx, fixture.pool, AdmissionCommand{RunID: id.New(), CoordinatorIdentity: "concurrent-coordinator", Candidate: candidate})
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	var successes, busy int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			successes++
		case errors.Is(err, ErrLeaseBusy):
			busy++
		default:
			t.Fatalf("concurrent admission error = %v", err)
		}
	}
	var runs, events int
	if err := fixture.pool.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM core.runs WHERE project_id = $1),
        (SELECT count(*) FROM core.events WHERE project_id = $1 AND event_type IN ('run.admitted', 'task.admitted'))`,
		fixture.projectID).Scan(&runs, &events); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || busy != 1 || runs != 1 || events != 2 {
		t.Fatalf("success/busy/runs/events = %d/%d/%d/%d", successes, busy, runs, events)
	}
}

func TestPostgresAdmissionRollbackAndRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newSchedulerFixture(t, ctx)
	task := fixture.addTask(t, ctx, "rollback", "pending", 1, time.Now().UTC())
	candidate, err := Select(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	enableAdmissionFailure(t, ctx, fixture.pool)
	command := AdmissionCommand{RunID: id.New(), CoordinatorIdentity: "rollback-coordinator", Candidate: candidate}
	if _, err := Admit(ctx, fixture.pool, command); err == nil {
		t.Fatal("Admit() succeeded with forced event failure")
	}
	assertAdmissionCounts(t, ctx, fixture.pool, task.id, 0, 0, "pending", 4, false)

	disableAdmissionFailure(t, ctx, fixture.pool)
	if _, err := Admit(ctx, fixture.pool, command); err != nil {
		t.Fatal(err)
	}
	assertAdmissionCounts(t, ctx, fixture.pool, task.id, 1, 2, "admitted", 5, true)
}

func TestPostgresReleaseAndRestartReconciliation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newSchedulerFixture(t, ctx)
	task := fixture.addTask(t, ctx, "reconcile", "pending", 1, time.Now().UTC())
	candidate, err := Select(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	runID := id.New()
	if _, err := Admit(ctx, fixture.pool, AdmissionCommand{RunID: runID, CoordinatorIdentity: "restart-coordinator", Candidate: candidate}); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconcile(ctx, fixture.pool, "foreign-coordinator"); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("foreign reconciliation error = %v, want %v", err, ErrLeaseBusy)
	}
	active, err := Reconcile(ctx, fixture.pool, "restart-coordinator")
	if err != nil || active.State != "active" || active.Changed || active.RunID != runID {
		t.Fatalf("active reconciliation = %#v, err = %v", active, err)
	}
	if _, err := Release(ctx, fixture.pool, runID, "restart-coordinator"); !errors.Is(err, ErrConflict) {
		t.Fatalf("unresolved release error = %v, want %v", err, ErrConflict)
	}
	fixture.resolveTask(t, ctx, task, "blocked", mustUUID(t, runID))
	reconciled, err := Reconcile(ctx, fixture.pool, "restart-coordinator")
	if err != nil || reconciled.State != "released" || !reconciled.Changed || reconciled.RunID != runID {
		t.Fatalf("resolved reconciliation = %#v, err = %v", reconciled, err)
	}
	idle, err := Reconcile(ctx, fixture.pool, "restart-coordinator")
	if err != nil || idle.State != "idle" || idle.Changed {
		t.Fatalf("idle reconciliation = %#v, err = %v", idle, err)
	}
	replayed, err := Release(ctx, fixture.pool, runID, "restart-coordinator")
	if err != nil || !replayed.Replayed || replayed.RunVersion != 2 {
		t.Fatalf("release replay = %#v, err = %v", replayed, err)
	}
	var runStatus string
	var leaseRun pgtype.UUID
	var releaseEvents int
	if err := fixture.pool.QueryRow(ctx, `SELECT r.status, l.run_id,
        (SELECT count(*) FROM core.events WHERE run_id = r.id AND event_type = 'run.released')
        FROM core.runs AS r CROSS JOIN core.execution_leases AS l
        WHERE r.id = $1 AND l.lease_name = 'global-source-mutation-v1'`, mustUUID(t, runID)).Scan(&runStatus, &leaseRun, &releaseEvents); err != nil {
		t.Fatal(err)
	}
	if runStatus != "released" || leaseRun.Valid || releaseEvents != 1 {
		t.Fatalf("released authority = run %s lease %s events %d", runStatus, uuidString(leaseRun), releaseEvents)
	}

	directTask := fixture.addTask(t, ctx, "direct-release", "pending", 1, time.Now().UTC())
	directCandidate, err := Select(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	directRunID := id.New()
	if _, err := Admit(ctx, fixture.pool, AdmissionCommand{RunID: directRunID, CoordinatorIdentity: "release-coordinator", Candidate: directCandidate}); err != nil {
		t.Fatal(err)
	}
	fixture.resolveTask(t, ctx, directTask, "completed", mustUUID(t, directRunID))
	released, err := Release(ctx, fixture.pool, directRunID, "release-coordinator")
	if err != nil || released.Replayed || released.RunVersion != 2 || released.EventID == "" {
		t.Fatalf("direct release = %#v, err = %v", released, err)
	}
	directReplay, err := Release(ctx, fixture.pool, directRunID, "release-coordinator")
	if err != nil || !directReplay.Replayed || directReplay.EventID != "" {
		t.Fatalf("direct release replay = %#v, err = %v", directReplay, err)
	}
}

type schedulerFixture struct {
	pool      *pgxpool.Pool
	projectID pgtype.UUID
	sourceID  pgtype.UUID
	commit    string
	tree      string
}

type schedulerTask struct {
	id        pgtype.UUID
	versionID pgtype.UUID
	projectID pgtype.UUID
}

func newSchedulerFixture(t *testing.T, ctx context.Context) schedulerFixture {
	t.Helper()
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	projectID := newUUID()
	sourceID := newUUID()
	now := timestamp(time.Now().UTC().Truncate(time.Microsecond))
	queries := postgres.New(pool)
	if _, err := queries.InsertProject(ctx, postgres.InsertProjectParams{
		ID: projectID, Name: "scheduler-" + uuidString(projectID), Status: "registered", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	identity := fmt.Sprintf("%x", sha256.Sum256([]byte(uuidString(projectID))))
	if _, err := queries.InsertProjectSource(ctx, postgres.InsertProjectSourceParams{
		ID: sourceID, ProjectID: projectID,
		CanonicalSourcePath:   "/scheduler/source/" + uuidString(projectID),
		ManagedRepositoryPath: "/scheduler/managed/" + uuidString(projectID) + ".git",
		CurrentCommit:         identity, CurrentTree: identity, DirtyState: []byte(`{"dirty":false}`), Remotes: []byte(`[]`),
	}); err != nil {
		t.Fatal(err)
	}
	fixture := schedulerFixture{pool: pool, projectID: projectID, sourceID: sourceID, commit: identity, tree: identity}
	t.Cleanup(func() { fixture.cleanup(t) })
	return fixture
}

func (f schedulerFixture) addTask(t *testing.T, ctx context.Context, externalID, status string, priority int32, createdAt time.Time) schedulerTask {
	t.Helper()
	queries := postgres.New(f.pool)
	taskID, versionID, artifactID := newUUID(), newUUID(), newUUID()
	createdAt = createdAt.UTC().Truncate(time.Microsecond)
	created := timestamp(createdAt)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(uuidString(artifactID))))
	if _, err := queries.InsertArtifact(ctx, postgres.InsertArtifactParams{
		ID: artifactID, Sha256: hash, SizeBytes: 1, MediaType: "text/markdown",
		LogicalKind: "task-source", StoragePath: "test/" + uuidString(f.projectID) + "/" + hash, CreatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTask(ctx, postgres.InsertTaskParams{
		ID: taskID, ProjectID: f.projectID, ExternalTaskID: externalID + "-" + id.New()[:8],
		Status: "draft", CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTaskVersion(ctx, postgres.InsertTaskVersionParams{
		ID: versionID, TaskID: taskID, VersionNumber: 1, SourceArtifactID: artifactID,
		Title: externalID, Goal: "scheduler fixture", RiskClass: "low", MutationClass: "bounded_source",
		NetworkProfile: "none", Priority: priority, Scope: []byte(`[]`), ExcludedScope: []byte(`[]`),
		VerificationPlan: []byte(`[]`), Budget: []byte(`{}`), SecretRequirements: []byte(`[]`),
		ExpectedPaths: []byte(`[]`), OperatorCheckpoints: []byte(`[]`), CreatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE core.tasks
        SET status = $1, accepted_version_id = $2, aggregate_version = 4
        WHERE id = $3`, status, versionID, taskID); err != nil {
		t.Fatal(err)
	}
	return schedulerTask{id: taskID, versionID: versionID, projectID: f.projectID}
}

func (f schedulerFixture) addDependency(t *testing.T, ctx context.Context, task, dependency schedulerTask) {
	t.Helper()
	if _, err := postgres.New(f.pool).InsertTaskDependency(ctx, postgres.InsertTaskDependencyParams{
		TaskVersionID: task.versionID, TaskID: task.id, ProjectID: f.projectID,
		DependencyTaskID: dependency.id, DependencyType: "requires", CreatedAt: timestamp(time.Now().UTC()),
	}); err != nil {
		t.Fatal(err)
	}
}

func (f schedulerFixture) resolveTask(t *testing.T, ctx context.Context, task schedulerTask, status string, runID pgtype.UUID) {
	t.Helper()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var aggregateVersion int64
	if err := tx.QueryRow(ctx, `UPDATE core.tasks SET status = $1,
        aggregate_version = aggregate_version + 1, updated_at = $2
        WHERE id = $3 RETURNING aggregate_version`, status, time.Now().UTC(), task.id).Scan(&aggregateVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.New(tx).AppendEvent(ctx, postgres.AppendEventParams{
		ID: newUUID(), ProjectID: f.projectID, TaskID: task.id, RunID: runID,
		EventType: "task." + status, AggregateType: "task", AggregateID: task.id,
		AggregateVersion: aggregateVersion, Payload: []byte(`{}`), CreatedAt: timestamp(time.Now().UTC()),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (f schedulerFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	statements := []string{
		`UPDATE core.execution_leases SET run_id = NULL, coordinator_identity = NULL, acquired_at = NULL
         WHERE run_id IN (SELECT id FROM core.runs WHERE project_id = $1)`,
		"DELETE FROM core.events WHERE project_id = $1",
		"DELETE FROM core.runs WHERE project_id = $1",
		"DELETE FROM core.task_acceptance_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.task_acceptance_criteria WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.task_conflicts WHERE project_id = $1",
		"DELETE FROM core.task_dependencies WHERE project_id = $1",
		"UPDATE core.tasks SET accepted_version_id = NULL, status = 'draft' WHERE project_id = $1",
		"DELETE FROM core.task_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.tasks WHERE project_id = $1",
		"DELETE FROM core.project_sources WHERE project_id = $1",
		"DELETE FROM core.projects WHERE id = $1",
		`DELETE FROM core.artifacts
	         WHERE storage_path LIKE 'test/' || $1::text || '/%'`,
	}
	for _, statement := range statements {
		if _, err := f.pool.Exec(ctx, statement, f.projectID); err != nil {
			t.Error(err)
			return
		}
	}
}

func assertAdmissionCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, taskID pgtype.UUID, wantRuns, wantEvents int, wantStatus string, wantAggregate int64, wantLease bool) {
	t.Helper()
	var runs, events int
	var status string
	var aggregate int64
	var leaseRun pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT
        (SELECT count(*) FROM core.runs WHERE task_id = $1),
        (SELECT count(*) FROM core.events WHERE task_id = $1 AND event_type IN ('run.admitted', 'task.admitted')),
        (SELECT status FROM core.tasks WHERE id = $1),
        (SELECT aggregate_version FROM core.tasks WHERE id = $1),
        (SELECT run_id FROM core.execution_leases WHERE lease_name = 'global-source-mutation-v1')`, taskID).Scan(
		&runs, &events, &status, &aggregate, &leaseRun,
	); err != nil {
		t.Fatal(err)
	}
	if runs != wantRuns || events != wantEvents || status != wantStatus || aggregate != wantAggregate || leaseRun.Valid != wantLease {
		t.Fatalf("runs/events/status/aggregate/lease = %d/%d/%s/%d/%v, want %d/%d/%s/%d/%v", runs, events, status, aggregate, leaseRun.Valid, wantRuns, wantEvents, wantStatus, wantAggregate, wantLease)
	}
}

func enableAdmissionFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	disableAdmissionFailure(t, ctx, pool)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION core.fail_scheduler_admission_test() RETURNS trigger
        LANGUAGE plpgsql AS $$
        BEGIN
            IF NEW.event_type = 'task.admitted' THEN
                RAISE EXCEPTION 'forced scheduler admission failure';
            END IF;
            RETURN NEW;
        END
        $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_scheduler_admission_test
        BEFORE INSERT ON core.events FOR EACH ROW EXECUTE FUNCTION core.fail_scheduler_admission_test()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disableAdmissionFailure(t, context.Background(), pool) })
}

func disableAdmissionFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DROP TRIGGER IF EXISTS fail_scheduler_admission_test ON core.events"); err != nil {
		t.Error(err)
	}
	if _, err := pool.Exec(ctx, "DROP FUNCTION IF EXISTS core.fail_scheduler_admission_test()"); err != nil {
		t.Error(err)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := parseUUID("test id", value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
