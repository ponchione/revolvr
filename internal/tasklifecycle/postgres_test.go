package tasklifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/storage/postgres"
	"revolvr/internal/taskintake"
)

func TestPostgresReviewGateAndApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := lifecycleTestPool(t, ctx)
	fixture := importLifecycleTask(t, ctx, pool, "review-gate")

	initial := taskState(t, ctx, pool, fixture.taskID)
	if initial.status != TaskDraft || initial.aggregateVersion != 1 || initial.acceptedVersionID.Valid {
		t.Fatalf("imported task state = %#v, want unaccepted draft version 1", initial)
	}
	versionBefore := taskVersionBytes(t, ctx, pool, fixture.versionID)

	compiled, err := Transition(ctx, pool, TransitionCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 1, NextStatus: TaskCompiled, Authority: AuthorityHostValidator,
	})
	if err != nil {
		t.Fatal(err)
	}
	awaiting, err := Transition(ctx, pool, TransitionCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 2, NextStatus: TaskAwaitingApproval, Authority: AuthorityHost,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeEmptyOperator := taskState(t, ctx, pool, fixture.taskID)
	if _, err := Approve(ctx, pool, ApprovalCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 3, OperatorIdentity: "   ",
	}); err == nil {
		t.Fatal("Approve() accepted an empty operator identity")
	}
	if after := taskState(t, ctx, pool, fixture.taskID); after != beforeEmptyOperator {
		t.Fatalf("empty-operator rejection changed state from %#v to %#v", beforeEmptyOperator, after)
	}

	approved, err := Approve(ctx, pool, ApprovalCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 3, OperatorIdentity: " operator@example.test ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if compiled.AggregateVersion != 2 || awaiting.AggregateVersion != 3 || approved.AggregateVersion != 4 ||
		approved.Status != TaskPending || approved.PreviousStatus != TaskAwaitingApproval {
		t.Fatalf("transition results = compiled %#v awaiting %#v approved %#v", compiled, awaiting, approved)
	}
	if approved.UpdatedAt.Location() != time.UTC {
		t.Fatalf("approval timestamp location = %v, want UTC", approved.UpdatedAt.Location())
	}

	selected, err := postgres.New(pool).GetApprovedTaskWithSelectedVersion(ctx, postgres.GetApprovedTaskWithSelectedVersionParams{
		ProjectID: fixture.projectID, TaskID: fixture.taskID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Status != "pending" || selected.AggregateVersion != 4 ||
		selected.AcceptedVersionID != fixture.versionID || selected.SelectedVersionID != fixture.versionID ||
		selected.SelectedVersionNumber != 1 || selected.SelectedTitle != "Lifecycle task" {
		t.Fatalf("selected approved task = %#v", selected)
	}
	if versionAfter := taskVersionBytes(t, ctx, pool, fixture.versionID); !bytes.Equal(versionAfter, versionBefore) {
		t.Fatalf("immutable task version changed through approval:\nbefore %s\nafter  %s", versionBefore, versionAfter)
	}

	events := lifecycleEvents(t, ctx, pool, fixture.taskID)
	if len(events) != 3 || events[0].eventType != "task.compiled" || events[0].aggregateVersion != 2 ||
		events[1].eventType != "task.awaiting_approval" || events[1].aggregateVersion != 3 ||
		events[2].eventType != "task.approved" || events[2].aggregateVersion != 4 {
		t.Fatalf("lifecycle events = %#v", events)
	}
	var payload eventPayload
	if err := json.Unmarshal(events[2].payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ProjectID != fixture.project || payload.TaskID != fixture.task ||
		payload.TaskVersionID != fixture.version || payload.TaskVersionNumber != 1 ||
		payload.TaskVersionSourceArtifactID != fixture.sourceArtifact ||
		payload.OperatorIdentity != "operator@example.test" || payload.Authority != string(AuthorityOperator) ||
		payload.PreviousStatus != "awaiting_approval" || payload.NewStatus != "pending" ||
		payload.ExpectedAggregateVersion != 3 || payload.AggregateVersion != 4 {
		t.Fatalf("task.approved payload = %#v", payload)
	}
}

func TestPostgresLifecycleRejectsStaleOwnershipAuthorityAndFutureTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := lifecycleTestPool(t, ctx)
	one := importLifecycleTask(t, ctx, pool, "closed-one")
	two := importLifecycleTask(t, ctx, pool, "closed-two")
	otherProject := lifecycleProject(t, ctx, pool, "other-project")

	assertUnchanged := func(f lifecycleFixture, wantErr error, run func() error) {
		t.Helper()
		before := taskState(t, ctx, pool, f.taskID)
		beforeEvents := len(lifecycleEvents(t, ctx, pool, f.taskID))
		if err := run(); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if after := taskState(t, ctx, pool, f.taskID); after != before {
			t.Fatalf("rejection changed state from %#v to %#v", before, after)
		}
		if afterEvents := len(lifecycleEvents(t, ctx, pool, f.taskID)); afterEvents != beforeEvents {
			t.Fatalf("rejection changed event count from %d to %d", beforeEvents, afterEvents)
		}
	}

	assertUnchanged(one, ErrConflict, func() error {
		_, err := Transition(ctx, pool, TransitionCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 2, NextStatus: TaskCompiled, Authority: AuthorityHostValidator,
		})
		return err
	})
	assertUnchanged(one, ErrUnauthorized, func() error {
		_, err := Transition(ctx, pool, TransitionCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 1, NextStatus: TaskCompiled, Authority: AuthorityModel,
		})
		return err
	})
	assertUnchanged(one, ErrIllegalTransition, func() error {
		_, err := Transition(ctx, pool, TransitionCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 1, NextStatus: TaskPending, Authority: AuthorityOperator,
		})
		return err
	})
	assertUnchanged(one, ErrNotFound, func() error {
		_, err := Transition(ctx, pool, TransitionCommand{
			ProjectID: otherProject.string, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 1, NextStatus: TaskCompiled, Authority: AuthorityHostValidator,
		})
		return err
	})
	assertUnchanged(one, ErrNotFound, func() error {
		_, err := Transition(ctx, pool, TransitionCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: two.version,
			ExpectedAggregateVersion: 1, NextStatus: TaskCompiled, Authority: AuthorityHostValidator,
		})
		return err
	})

	advanceToAwaiting(t, ctx, pool, one)
	assertUnchanged(one, ErrNotFound, func() error {
		_, err := Approve(ctx, pool, ApprovalCommand{
			ProjectID: otherProject.string, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 3, OperatorIdentity: "operator",
		})
		return err
	})
	assertUnchanged(two, ErrNotFound, func() error {
		_, err := Approve(ctx, pool, ApprovalCommand{
			ProjectID: one.project, TaskID: two.task, TaskVersionID: two.version,
			ExpectedAggregateVersion: 1, OperatorIdentity: "operator",
		})
		return err
	})
	assertUnchanged(one, ErrNotFound, func() error {
		_, err := Approve(ctx, pool, ApprovalCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: two.version,
			ExpectedAggregateVersion: 3, OperatorIdentity: "operator",
		})
		return err
	})
	if _, err := Approve(ctx, pool, ApprovalCommand{
		ProjectID: one.project, TaskID: one.task, TaskVersionID: one.version,
		ExpectedAggregateVersion: 3, OperatorIdentity: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	assertUnchanged(one, ErrUnavailable, func() error {
		_, err := Transition(ctx, pool, TransitionCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 4, NextStatus: TaskAdmitted, Authority: AuthorityScheduler,
		})
		return err
	})
	assertUnchanged(one, ErrAlreadyApproved, func() error {
		_, err := Approve(ctx, pool, ApprovalCommand{
			ProjectID: one.project, TaskID: one.task, TaskVersionID: one.version,
			ExpectedAggregateVersion: 4, OperatorIdentity: "operator",
		})
		return err
	})
}

func TestPostgresConcurrentTransitionHasOneWinnerAndOneEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := lifecycleTestPool(t, ctx)
	fixture := importLifecycleTask(t, ctx, pool, "concurrent")
	command := TransitionCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 1, NextStatus: TaskCompiled, Authority: AuthorityHostValidator,
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			_, err := Transition(ctx, pool, command)
			errs <- err
		}()
	}
	ready.Wait()
	close(start)

	var successes, conflicts int
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Transition() error = %v", err)
		}
	}
	state := taskState(t, ctx, pool, fixture.taskID)
	if successes != 1 || conflicts != 1 || state.status != TaskCompiled || state.aggregateVersion != 2 {
		t.Fatalf("success/conflict/state = %d/%d/%#v", successes, conflicts, state)
	}
	if events := lifecycleEvents(t, ctx, pool, fixture.taskID); len(events) != 1 || events[0].eventType != "task.compiled" {
		t.Fatalf("events = %#v, want one task.compiled", events)
	}
}

func TestPostgresApprovalEventFailureRollsBackAndRetrySucceedsOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := lifecycleTestPool(t, ctx)
	fixture := importLifecycleTask(t, ctx, pool, "rollback")
	advanceToAwaiting(t, ctx, pool, fixture)
	before := taskState(t, ctx, pool, fixture.taskID)
	beforeEvents := lifecycleEvents(t, ctx, pool, fixture.taskID)
	enableApprovalEventFailure(t, ctx, pool)

	command := ApprovalCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 3, OperatorIdentity: "rollback-operator",
	}
	if _, err := Approve(ctx, pool, command); err == nil {
		t.Fatal("Approve() succeeded with forced event failure")
	}
	if after := taskState(t, ctx, pool, fixture.taskID); after != before {
		t.Fatalf("failed approval changed state from %#v to %#v", before, after)
	}
	if afterEvents := lifecycleEvents(t, ctx, pool, fixture.taskID); len(afterEvents) != len(beforeEvents) {
		t.Fatalf("failed approval changed events from %#v to %#v", beforeEvents, afterEvents)
	}

	disableApprovalEventFailure(t, ctx, pool)
	if _, err := Approve(ctx, pool, command); err != nil {
		t.Fatal(err)
	}
	if _, err := Approve(ctx, pool, ApprovalCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 4, OperatorIdentity: "rollback-operator",
	}); !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("repeated Approve() error = %v, want %v", err, ErrAlreadyApproved)
	}
	events := lifecycleEvents(t, ctx, pool, fixture.taskID)
	approved := 0
	for _, event := range events {
		if event.eventType == "task.approved" {
			approved++
		}
	}
	if approved != 1 || len(events) != len(beforeEvents)+1 {
		t.Fatalf("events after retry = %#v, want one approval", events)
	}
}

type lifecycleFixture struct {
	projectID      pgtype.UUID
	taskID         pgtype.UUID
	versionID      pgtype.UUID
	project        string
	task           string
	version        string
	sourceArtifact string
}

type projectIdentity struct {
	uuid   pgtype.UUID
	string string
}

func importLifecycleTask(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) lifecycleFixture {
	t.Helper()
	project := lifecycleProject(t, ctx, pool, name)
	externalID := name + "-" + id.New()[:8]
	source := []byte(fmt.Sprintf(`---
schema: revolvr-task-v1
id: %s
priority: 100
mutation_class: bounded_source
risk: low
network: none
depends_on: []
conflicts: []
expected_paths:
  - internal/tasklifecycle/**
budget:
  max_cycles: 2
  max_model_tokens: 1000
  max_wall_time: 5m
---

# Lifecycle task

## Goal

Exercise the PostgreSQL lifecycle review gate.

## Scope

- Lifecycle state.

## Excluded Scope

- Scheduler execution.

## Acceptance

### AC-1

The lifecycle transition is stored atomically.

Verification:

`+"```text\ntrue\n```\n", externalID))
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := taskintake.Import(ctx, pool, project.string, artifactRoot, externalID+".md", "text/markdown", source)
	if err != nil {
		t.Fatal(err)
	}
	taskID := parseTestUUID(t, result.TaskID)
	var versionID, sourceArtifactID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id, source_artifact_id FROM core.task_versions WHERE task_id = $1", taskID).Scan(&versionID, &sourceArtifactID); err != nil {
		t.Fatal(err)
	}
	return lifecycleFixture{
		projectID: project.uuid, taskID: taskID, versionID: versionID,
		project: project.string, task: uuidString(taskID), version: uuidString(versionID),
		sourceArtifact: uuidString(sourceArtifactID),
	}
}

func lifecycleProject(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) projectIdentity {
	t.Helper()
	projectID := newUUID()
	now := timestamp(time.Now().UTC().Truncate(time.Microsecond))
	if _, err := postgres.New(pool).InsertProject(ctx, postgres.InsertProjectParams{
		ID: projectID, Name: name + "-" + uuidString(projectID), Status: "registered",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupLifecycleProject(t, pool, projectID) })
	return projectIdentity{uuid: projectID, string: uuidString(projectID)}
}

func advanceToAwaiting(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fixture lifecycleFixture) {
	t.Helper()
	if _, err := Transition(ctx, pool, TransitionCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 1, NextStatus: TaskCompiled, Authority: AuthorityHostValidator,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Transition(ctx, pool, TransitionCommand{
		ProjectID: fixture.project, TaskID: fixture.task, TaskVersionID: fixture.version,
		ExpectedAggregateVersion: 2, NextStatus: TaskAwaitingApproval, Authority: AuthorityHost,
	}); err != nil {
		t.Fatal(err)
	}
}

type storedTaskState struct {
	status            TaskStatus
	acceptedVersionID pgtype.UUID
	aggregateVersion  int64
	createdAt         time.Time
	updatedAt         time.Time
}

func taskState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, taskID pgtype.UUID) storedTaskState {
	t.Helper()
	var state storedTaskState
	if err := pool.QueryRow(ctx, `SELECT status, accepted_version_id, aggregate_version, created_at, updated_at
        FROM core.tasks WHERE id = $1`, taskID).Scan(
		&state.status, &state.acceptedVersionID, &state.aggregateVersion, &state.createdAt, &state.updatedAt,
	); err != nil {
		t.Fatal(err)
	}
	return state
}

func taskVersionBytes(t *testing.T, ctx context.Context, pool *pgxpool.Pool, versionID pgtype.UUID) []byte {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx, "SELECT to_jsonb(tv) FROM core.task_versions AS tv WHERE id = $1", versionID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

type storedEvent struct {
	eventType        string
	aggregateVersion int64
	payload          []byte
}

func lifecycleEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, taskID pgtype.UUID) []storedEvent {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT event_type, aggregate_version, payload
        FROM core.events WHERE aggregate_type = 'task' AND aggregate_id = $1
        ORDER BY aggregate_version`, taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var events []storedEvent
	for rows.Next() {
		var event storedEvent
		if err := rows.Scan(&event.eventType, &event.aggregateVersion, &event.payload); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func lifecycleTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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
	return pool
}

func parseTestUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func enableApprovalEventFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	disableApprovalEventFailure(t, ctx, pool)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION core.fail_task_approval_test() RETURNS trigger
        LANGUAGE plpgsql AS $$
        BEGIN
            IF NEW.event_type = 'task.approved' THEN
                RAISE EXCEPTION 'forced task approval failure';
            END IF;
            RETURN NEW;
        END
        $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER fail_task_approval_test
        BEFORE INSERT ON core.events FOR EACH ROW EXECUTE FUNCTION core.fail_task_approval_test()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disableApprovalEventFailure(t, context.Background(), pool) })
}

func disableApprovalEventFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "DROP TRIGGER IF EXISTS fail_task_approval_test ON core.events"); err != nil {
		t.Error(err)
	}
	if _, err := pool.Exec(ctx, "DROP FUNCTION IF EXISTS core.fail_task_approval_test()"); err != nil {
		t.Error(err)
	}
}

func cleanupLifecycleProject(t *testing.T, pool *pgxpool.Pool, projectID pgtype.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT source_artifact_id FROM core.task_imports WHERE project_id = $1
        UNION SELECT v.source_artifact_id FROM core.task_versions AS v
        JOIN core.tasks AS t ON t.id = v.task_id WHERE t.project_id = $1`, projectID)
	if err != nil {
		t.Error(err)
		return
	}
	var artifacts []pgtype.UUID
	for rows.Next() {
		var artifactID pgtype.UUID
		if err := rows.Scan(&artifactID); err != nil {
			t.Error(err)
			rows.Close()
			return
		}
		artifacts = append(artifacts, artifactID)
	}
	rows.Close()
	statements := []string{
		"DELETE FROM core.task_imports WHERE project_id = $1",
		"DELETE FROM core.task_acceptance_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.task_acceptance_criteria WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.task_conflicts WHERE project_id = $1",
		"DELETE FROM core.task_dependencies WHERE project_id = $1",
		"UPDATE core.tasks SET accepted_version_id = NULL, status = 'draft' WHERE project_id = $1",
		"DELETE FROM core.task_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id = $1)",
		"DELETE FROM core.tasks WHERE project_id = $1",
		"DELETE FROM core.events WHERE project_id = $1",
		"DELETE FROM core.projects WHERE id = $1",
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, projectID); err != nil {
			t.Error(err)
			return
		}
	}
	for _, artifactID := range artifacts {
		if _, err := pool.Exec(ctx, "DELETE FROM core.artifacts WHERE id = $1", artifactID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			t.Error(err)
		}
	}
}
