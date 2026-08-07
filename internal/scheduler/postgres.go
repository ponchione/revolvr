package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/storage/postgres"
)

const GlobalLeaseName = "global-source-mutation-v1"

var (
	ErrNoReady     = errors.New("no ready task")
	ErrWaiting     = errors.New("tasks are waiting")
	ErrUnsafeGraph = errors.New("unsafe task graph")
	ErrLeaseBusy   = errors.New("global execution lease is busy")
	ErrConflict    = errors.New("scheduler authority conflict")
)

type Candidate struct {
	ProjectID                string
	ProjectSourceID          string
	TaskID                   string
	TaskVersionID            string
	ExternalTaskID           string
	ExpectedAggregateVersion int64
	Priority                 int32
	CreatedAt                time.Time
	SourceCommit             string
	SourceTree               string
}

type AdmissionCommand struct {
	RunID               string
	CoordinatorIdentity string
	Candidate           Candidate
}

type Admission struct {
	RunID                string
	Candidate            Candidate
	TaskAggregateVersion int64
	LeaseVersion         int64
	RunEventID           string
	TaskEventID          string
	AdmittedAt           time.Time
	Replayed             bool
}

type ReleaseResult struct {
	RunID        string
	LeaseVersion int64
	RunVersion   int64
	EventID      string
	ReleasedAt   time.Time
	Replayed     bool
}

type Reconciliation struct {
	State   string
	RunID   string
	Changed bool
}

// QueueState classifies a valid graph only after Select reports no ready task.
// It does not select, reorder, or admit work; Select remains the sole selector.
type QueueState struct {
	WaitingDependencies []WaitingTask
	WaitingInput        []WaitingTask
	Blocked             []WaitingTask
}

type UnsafeGraphError struct {
	Diagnostics []Diagnostic
}

func (e *UnsafeGraphError) Error() string {
	if len(e.Diagnostics) == 0 {
		return ErrUnsafeGraph.Error()
	}
	return fmt.Sprintf("%s: %s", ErrUnsafeGraph, e.Diagnostics[0].Detail)
}

func (e *UnsafeGraphError) Unwrap() error { return ErrUnsafeGraph }

type WaitingError struct {
	Tasks []WaitingTask
}

func (e *WaitingError) Error() string {
	if len(e.Tasks) == 0 {
		return ErrWaiting.Error()
	}
	return fmt.Sprintf("%s: %s is %s", ErrWaiting, e.Tasks[0].TaskID, e.Tasks[0].Reason)
}

func (e *WaitingError) Unwrap() error { return ErrWaiting }

type LeaseBusyError struct {
	RunID               string
	CoordinatorIdentity string
}

func (e *LeaseBusyError) Error() string {
	return fmt.Sprintf("%s: run %s held by %s", ErrLeaseBusy, e.RunID, e.CoordinatorIdentity)
}

func (e *LeaseBusyError) Unwrap() error { return ErrLeaseBusy }

func Select(ctx context.Context, pool *pgxpool.Pool) (Candidate, error) {
	if pool == nil {
		return Candidate{}, errors.New("scheduler: PostgreSQL pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Candidate{}, fmt.Errorf("scheduler selection: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := postgres.New(tx)
	lease, err := queries.GetGlobalExecutionLease(ctx)
	if err != nil {
		return Candidate{}, fmt.Errorf("scheduler selection: load global lease: %w", err)
	}
	if lease.RunID.Valid {
		return Candidate{}, leaseBusy(lease)
	}
	active, err := queries.ListActiveRuns(ctx)
	if err != nil {
		return Candidate{}, fmt.Errorf("scheduler selection: list active runs: %w", err)
	}
	if len(active) != 0 {
		return Candidate{}, activeRunWithoutLease(active)
	}
	candidate, err := selectCandidate(ctx, queries)
	if err != nil {
		return Candidate{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Candidate{}, fmt.Errorf("scheduler selection: %w", err)
	}
	return candidate, nil
}

func InspectQueueState(ctx context.Context, pool *pgxpool.Pool) (QueueState, error) {
	if pool == nil {
		return QueueState{}, errors.New("scheduler queue inspection: PostgreSQL pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return QueueState{}, fmt.Errorf("scheduler queue inspection: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := postgres.New(tx)
	lease, err := queries.GetGlobalExecutionLease(ctx)
	if err != nil {
		return QueueState{}, fmt.Errorf("scheduler queue inspection: load global lease: %w", err)
	}
	if lease.RunID.Valid {
		return QueueState{}, leaseBusy(lease)
	}
	active, err := queries.ListActiveRuns(ctx)
	if err != nil {
		return QueueState{}, fmt.Errorf("scheduler queue inspection: list active runs: %w", err)
	}
	if len(active) != 0 {
		return QueueState{}, activeRunWithoutLease(active)
	}
	tasks, err := loadGraph(ctx, queries)
	if err != nil {
		return QueueState{}, fmt.Errorf("scheduler queue inspection: %w", err)
	}
	projection := evaluateGraph(tasks)
	if len(projection.diagnostics) != 0 {
		return QueueState{}, &UnsafeGraphError{Diagnostics: projection.diagnostics}
	}
	if projection.candidate != nil {
		return QueueState{}, fmt.Errorf("%w: ready work exists", ErrConflict)
	}
	byID := make(map[string]graphTask, len(tasks))
	for _, task := range tasks {
		byID[task.taskID] = task
	}
	state := QueueState{}
	for _, task := range tasks {
		switch task.status {
		case "needs_input":
			state.WaitingInput = append(state.WaitingInput, WaitingTask{TaskID: task.taskID, Reason: "task_needs_input"})
		case "blocked":
			state.Blocked = append(state.Blocked, WaitingTask{TaskID: task.taskID, Reason: "task_blocked"})
		case "pending":
			reason := waitingReason(task, byID)
			if task.awaitingOperatorCheckpoint || dependencyHasStatus(task, byID, "needs_input") {
				state.WaitingInput = append(state.WaitingInput, WaitingTask{TaskID: task.taskID, Reason: reason})
			} else if reason != "" {
				state.WaitingDependencies = append(state.WaitingDependencies, WaitingTask{TaskID: task.taskID, Reason: reason})
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return QueueState{}, fmt.Errorf("scheduler queue inspection: %w", err)
	}
	return state, nil
}

func dependencyHasStatus(task graphTask, byID map[string]graphTask, status string) bool {
	for _, edge := range task.dependencies {
		if byID[edge.targetID].status == status {
			return true
		}
	}
	return false
}

func Admit(ctx context.Context, pool *pgxpool.Pool, command AdmissionCommand) (Admission, error) {
	if pool == nil {
		return Admission{}, errors.New("scheduler admission: PostgreSQL pool is required")
	}
	runID, err := parseUUIDv7("run id", command.RunID)
	if err != nil {
		return Admission{}, err
	}
	coordinator := strings.TrimSpace(command.CoordinatorIdentity)
	if coordinator == "" {
		return Admission{}, errors.New("scheduler admission: coordinator identity is required")
	}
	ids, err := parseCandidate(command.Candidate)
	if err != nil {
		return Admission{}, err
	}

	var result Admission
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		lease, err := queries.GetGlobalExecutionLeaseForUpdate(ctx)
		if err != nil {
			return err
		}
		if lease.RunID.Valid {
			if lease.RunID == runID {
				result, err = replayAdmission(ctx, queries, lease, command, ids)
				return err
			}
			return leaseBusy(lease)
		}
		active, err := queries.ListActiveRuns(ctx)
		if err != nil {
			return err
		}
		if len(active) != 0 {
			return activeRunWithoutLease(active)
		}
		if _, err := queries.GetRun(ctx, runID); err == nil {
			return fmt.Errorf("%w: run id was already used", ErrConflict)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		selected, err := selectCandidate(ctx, queries)
		if err != nil {
			return err
		}
		if !sameCandidate(selected, command.Candidate) {
			return fmt.Errorf("%w: selected task or pinned authority changed", ErrConflict)
		}

		now := time.Now().UTC().Truncate(time.Microsecond)
		if _, err := queries.InsertRun(ctx, postgres.InsertRunParams{
			ID: runID, ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.version, ProjectSourceID: ids.source,
			AdmittedTaskAggregateVersion: command.Candidate.ExpectedAggregateVersion + 1,
			SourceCommit:                 command.Candidate.SourceCommit, SourceTree: command.Candidate.SourceTree,
			CoordinatorIdentity: coordinator, CreatedAt: timestamp(now),
		}); err != nil {
			if uniqueViolation(err) {
				return &LeaseBusyError{}
			}
			return err
		}
		acquired, err := queries.AcquireGlobalExecutionLease(ctx, postgres.AcquireGlobalExecutionLeaseParams{
			RunID: runID, CoordinatorIdentity: text(coordinator), AcquiredAt: timestamp(now),
			ExpectedAggregateVersion: lease.AggregateVersion,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrLeaseBusy
		}
		if err != nil {
			return err
		}
		admitted, err := queries.AdmitSchedulerTask(ctx, postgres.AdmitSchedulerTaskParams{
			UpdatedAt: timestamp(now), ProjectID: ids.project, TaskID: ids.task,
			TaskVersionID: ids.version, ExpectedAggregateVersion: command.Candidate.ExpectedAggregateVersion,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		runEventID, taskEventID, err := appendAdmissionEvents(ctx, queries, runID, ids, command, admitted.AggregateVersion, acquired.AggregateVersion, now)
		if err != nil {
			return err
		}
		result = Admission{
			RunID: command.RunID, Candidate: command.Candidate,
			TaskAggregateVersion: admitted.AggregateVersion, LeaseVersion: acquired.AggregateVersion,
			RunEventID: uuidString(runEventID), TaskEventID: uuidString(taskEventID), AdmittedAt: now,
		}
		return nil
	})
	if err != nil {
		return Admission{}, fmt.Errorf("scheduler admission: %w", err)
	}
	return result, nil
}

func Release(ctx context.Context, pool *pgxpool.Pool, runID, coordinatorIdentity string) (ReleaseResult, error) {
	if pool == nil {
		return ReleaseResult{}, errors.New("scheduler release: PostgreSQL pool is required")
	}
	id, err := parseUUID("run id", runID)
	if err != nil {
		return ReleaseResult{}, err
	}
	coordinator := strings.TrimSpace(coordinatorIdentity)
	if coordinator == "" {
		return ReleaseResult{}, errors.New("scheduler release: coordinator identity is required")
	}
	var result ReleaseResult
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		lease, err := queries.GetGlobalExecutionLeaseForUpdate(ctx)
		if err != nil {
			return err
		}
		run, err := queries.GetRun(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: run does not exist", ErrConflict)
		}
		if err != nil {
			return err
		}
		if run.CoordinatorIdentity != coordinator {
			return fmt.Errorf("%w: coordinator identity changed", ErrConflict)
		}
		if run.Status == "released" && !lease.RunID.Valid {
			result = ReleaseResult{RunID: runID, LeaseVersion: lease.AggregateVersion, RunVersion: run.AggregateVersion, ReleasedAt: run.ReleasedAt.Time, Replayed: true}
			return nil
		}
		if !lease.RunID.Valid || lease.RunID != id || lease.CoordinatorIdentity.String != coordinator {
			if lease.RunID.Valid {
				return leaseBusy(lease)
			}
			return fmt.Errorf("%w: run and lease are not paired", ErrConflict)
		}
		result, err = releaseResolvedRun(ctx, queries, lease, run)
		return err
	})
	if err != nil {
		return ReleaseResult{}, fmt.Errorf("scheduler release: %w", err)
	}
	return result, nil
}

func Reconcile(ctx context.Context, pool *pgxpool.Pool, coordinatorIdentity string) (Reconciliation, error) {
	if pool == nil {
		return Reconciliation{}, errors.New("scheduler reconciliation: PostgreSQL pool is required")
	}
	coordinator := strings.TrimSpace(coordinatorIdentity)
	if coordinator == "" {
		return Reconciliation{}, errors.New("scheduler reconciliation: coordinator identity is required")
	}
	var result Reconciliation
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		queries := postgres.New(tx)
		lease, err := queries.GetGlobalExecutionLeaseForUpdate(ctx)
		if err != nil {
			return err
		}
		active, err := queries.ListActiveRuns(ctx)
		if err != nil {
			return err
		}
		if !lease.RunID.Valid {
			if len(active) != 0 {
				return activeRunWithoutLease(active)
			}
			result.State = "idle"
			return nil
		}
		if lease.CoordinatorIdentity.String != coordinator {
			return leaseBusy(lease)
		}
		run, err := queries.GetRun(ctx, lease.RunID)
		if err != nil {
			return fmt.Errorf("%w: lease references no authoritative run", ErrConflict)
		}
		result.RunID = uuidString(run.ID)
		if run.Status == "released" {
			if len(active) != 0 {
				return fmt.Errorf("%w: released lease coexists with active run", ErrConflict)
			}
			if _, err := validateRunAuthority(ctx, queries, run); err != nil {
				return err
			}
			if _, err := queries.ReleaseGlobalExecutionLease(ctx, postgres.ReleaseGlobalExecutionLeaseParams{
				RunID: run.ID, CoordinatorIdentity: text(coordinator), ExpectedAggregateVersion: lease.AggregateVersion,
			}); err != nil {
				return err
			}
			result.State, result.Changed = "released", true
			return nil
		}
		if len(active) != 1 || active[0].ID != run.ID {
			return fmt.Errorf("%w: lease and active run set disagree", ErrConflict)
		}
		task, err := validateRunAuthority(ctx, queries, run)
		if err != nil {
			return err
		}
		if releasableTaskStatus(task.Status) {
			if _, err := releaseResolvedRun(ctx, queries, lease, run); err != nil {
				return err
			}
			result.State, result.Changed = "released", true
			return nil
		}
		if !activeTaskStatus(task.Status) || task.AggregateVersion < run.AdmittedTaskAggregateVersion {
			return fmt.Errorf("%w: active run task state is inconsistent", ErrConflict)
		}
		result.State = "active"
		return nil
	})
	if err != nil {
		return Reconciliation{}, fmt.Errorf("scheduler reconciliation: %w", err)
	}
	return result, nil
}

func selectCandidate(ctx context.Context, queries *postgres.Queries) (Candidate, error) {
	tasks, err := loadGraph(ctx, queries)
	if err != nil {
		return Candidate{}, err
	}
	projection := evaluateGraph(tasks)
	return selectProjection(projection)
}

func selectProjection(projection graphResult) (Candidate, error) {
	if len(projection.diagnostics) != 0 {
		return Candidate{}, &UnsafeGraphError{Diagnostics: projection.diagnostics}
	}
	if projection.candidate != nil {
		return *projection.candidate, nil
	}
	if len(projection.waiting) != 0 {
		return Candidate{}, &WaitingError{Tasks: projection.waiting}
	}
	return Candidate{}, ErrNoReady
}

func loadGraph(ctx context.Context, queries *postgres.Queries) ([]graphTask, error) {
	rows, err := queries.ListSchedulerTasks(ctx)
	if err != nil {
		return nil, err
	}
	sources, err := queries.ListSchedulerProjectSources(ctx)
	if err != nil {
		return nil, err
	}
	dependencies, err := queries.ListSchedulerDependencies(ctx)
	if err != nil {
		return nil, err
	}
	conflicts, err := queries.ListSchedulerConflicts(ctx)
	if err != nil {
		return nil, err
	}

	byProject := make(map[string][]projectSource)
	for _, source := range sources {
		projectID := uuidString(source.ProjectID)
		byProject[projectID] = append(byProject[projectID], projectSource{id: uuidString(source.ID), commit: source.CurrentCommit, tree: source.CurrentTree})
	}
	tasks := make([]graphTask, 0, len(rows))
	byID := make(map[string]int, len(rows))
	for _, row := range rows {
		taskID := uuidString(row.ID)
		projectID := uuidString(row.ProjectID)
		tasks = append(tasks, graphTask{
			projectID: projectID, projectStatus: row.ProjectStatus,
			projectSources: append([]projectSource(nil), byProject[projectID]...),
			taskID:         taskID, externalTaskID: row.ExternalTaskID, status: row.Status,
			acceptedVersionID: uuidString(row.AcceptedVersionID), taskVersionID: uuidString(row.TaskVersionID),
			aggregateVersion: row.AggregateVersion, priority: row.TaskPriority.Int32,
			createdAt: row.CreatedAt.Time, awaitingOperatorCheckpoint: row.AwaitingOperatorCheckpoint,
		})
		byID[taskID] = len(tasks) - 1
	}
	for _, row := range dependencies {
		if index, ok := byID[uuidString(row.TaskID)]; ok {
			tasks[index].dependencies = append(tasks[index].dependencies, graphEdge{versionID: uuidString(row.TaskVersionID), targetID: uuidString(row.DependencyTaskID)})
		}
	}
	for _, row := range conflicts {
		if index, ok := byID[uuidString(row.TaskID)]; ok {
			tasks[index].conflicts = append(tasks[index].conflicts, graphEdge{versionID: uuidString(row.TaskVersionID), targetID: uuidString(row.ConflictingTaskID)})
		}
	}
	return tasks, nil
}

type candidateIDs struct {
	project pgtype.UUID
	source  pgtype.UUID
	task    pgtype.UUID
	version pgtype.UUID
}

func parseCandidate(candidate Candidate) (candidateIDs, error) {
	projectID, err := parseUUID("project id", candidate.ProjectID)
	if err != nil {
		return candidateIDs{}, err
	}
	sourceID, err := parseUUID("project source id", candidate.ProjectSourceID)
	if err != nil {
		return candidateIDs{}, err
	}
	taskID, err := parseUUID("task id", candidate.TaskID)
	if err != nil {
		return candidateIDs{}, err
	}
	versionID, err := parseUUID("task version id", candidate.TaskVersionID)
	if err != nil {
		return candidateIDs{}, err
	}
	if candidate.ExpectedAggregateVersion < 1 {
		return candidateIDs{}, errors.New("scheduler admission: expected task aggregate version must be positive")
	}
	return candidateIDs{project: projectID, source: sourceID, task: taskID, version: versionID}, nil
}

func replayAdmission(ctx context.Context, queries *postgres.Queries, lease postgres.CoreExecutionLease, command AdmissionCommand, ids candidateIDs) (Admission, error) {
	run, err := queries.GetRun(ctx, lease.RunID)
	if err != nil {
		return Admission{}, fmt.Errorf("%w: lease references no run", ErrConflict)
	}
	if lease.CoordinatorIdentity.String != strings.TrimSpace(command.CoordinatorIdentity) ||
		run.Status != "active" ||
		run.ProjectID != ids.project || run.ProjectSourceID != ids.source || run.TaskID != ids.task ||
		run.TaskVersionID != ids.version || run.SourceCommit != command.Candidate.SourceCommit ||
		run.SourceTree != command.Candidate.SourceTree ||
		run.AdmittedTaskAggregateVersion != command.Candidate.ExpectedAggregateVersion+1 {
		return Admission{}, fmt.Errorf("%w: replay authority changed", ErrConflict)
	}
	if _, err := validateRunAuthority(ctx, queries, run); err != nil {
		return Admission{}, err
	}
	return Admission{
		RunID: command.RunID, Candidate: command.Candidate,
		TaskAggregateVersion: run.AdmittedTaskAggregateVersion, LeaseVersion: lease.AggregateVersion,
		AdmittedAt: run.CreatedAt.Time, Replayed: true,
	}, nil
}

func appendAdmissionEvents(ctx context.Context, queries *postgres.Queries, runID pgtype.UUID, ids candidateIDs, command AdmissionCommand, taskVersion, leaseVersion int64, createdAt time.Time) (pgtype.UUID, pgtype.UUID, error) {
	payload, err := json.Marshal(map[string]any{
		"run_id": command.RunID, "project_id": command.Candidate.ProjectID,
		"project_source_id": command.Candidate.ProjectSourceID,
		"task_id":           command.Candidate.TaskID, "task_version_id": command.Candidate.TaskVersionID,
		"source_commit": command.Candidate.SourceCommit, "source_tree": command.Candidate.SourceTree,
		"coordinator_identity": strings.TrimSpace(command.CoordinatorIdentity),
		"lease_name":           GlobalLeaseName, "lease_version": leaseVersion,
		"expected_task_aggregate_version": command.Candidate.ExpectedAggregateVersion,
		"task_aggregate_version":          taskVersion,
	})
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	runEventID := newUUID()
	if _, err := queries.AppendEvent(ctx, postgres.AppendEventParams{
		ID: runEventID, ProjectID: ids.project, TaskID: ids.task, RunID: runID,
		EventType: "run.admitted", AggregateType: "run", AggregateID: runID,
		AggregateVersion: 1, Payload: payload, CreatedAt: timestamp(createdAt),
	}); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	taskEventID := newUUID()
	if _, err := queries.AppendEvent(ctx, postgres.AppendEventParams{
		ID: taskEventID, ProjectID: ids.project, TaskID: ids.task, RunID: runID,
		EventType: "task.admitted", AggregateType: "task", AggregateID: ids.task,
		AggregateVersion: taskVersion, Payload: payload, CreatedAt: timestamp(createdAt),
	}); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}
	return runEventID, taskEventID, nil
}

func releaseResolvedRun(ctx context.Context, queries *postgres.Queries, lease postgres.CoreExecutionLease, run postgres.CoreRun) (ReleaseResult, error) {
	task, err := validateRunAuthority(ctx, queries, run)
	if err != nil || !releasableTaskStatus(task.Status) {
		return ReleaseResult{}, fmt.Errorf("%w: run still owns unresolved task work", ErrConflict)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	released, err := queries.ReleaseRun(ctx, postgres.ReleaseRunParams{
		ReleasedAt: timestamp(now), RunID: run.ID, ExpectedAggregateVersion: run.AggregateVersion,
	})
	if err != nil {
		return ReleaseResult{}, err
	}
	releasedLease, err := queries.ReleaseGlobalExecutionLease(ctx, postgres.ReleaseGlobalExecutionLeaseParams{
		RunID: run.ID, CoordinatorIdentity: text(run.CoordinatorIdentity), ExpectedAggregateVersion: lease.AggregateVersion,
	})
	if err != nil {
		return ReleaseResult{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"run_id": uuidString(run.ID), "task_id": uuidString(run.TaskID),
		"task_status": task.Status, "lease_name": GlobalLeaseName,
		"lease_version": releasedLease.AggregateVersion,
	})
	if err != nil {
		return ReleaseResult{}, err
	}
	eventID := newUUID()
	if _, err := queries.AppendEvent(ctx, postgres.AppendEventParams{
		ID: eventID, ProjectID: run.ProjectID, TaskID: run.TaskID, RunID: run.ID,
		EventType: "run.released", AggregateType: "run", AggregateID: run.ID,
		AggregateVersion: released.AggregateVersion, Payload: payload, CreatedAt: timestamp(now),
	}); err != nil {
		return ReleaseResult{}, err
	}
	return ReleaseResult{
		RunID: uuidString(run.ID), LeaseVersion: releasedLease.AggregateVersion,
		RunVersion: released.AggregateVersion, EventID: uuidString(eventID), ReleasedAt: now,
	}, nil
}

func validateRunAuthority(ctx context.Context, queries *postgres.Queries, run postgres.CoreRun) (postgres.GetTaskAndVersionRow, error) {
	task, err := queries.GetTaskAndVersion(ctx, postgres.GetTaskAndVersionParams{
		ProjectID: run.ProjectID, TaskID: run.TaskID, TaskVersionID: run.TaskVersionID,
	})
	if err != nil || task.AcceptedVersionID != run.TaskVersionID || task.AggregateVersion < run.AdmittedTaskAggregateVersion {
		return postgres.GetTaskAndVersionRow{}, fmt.Errorf("%w: run task authority changed", ErrConflict)
	}
	source, err := queries.GetSchedulerProjectSource(ctx, postgres.GetSchedulerProjectSourceParams{
		ProjectSourceID: run.ProjectSourceID, ProjectID: run.ProjectID,
	})
	if err != nil || source.CurrentCommit != run.SourceCommit || source.CurrentTree != run.SourceTree {
		return postgres.GetTaskAndVersionRow{}, fmt.Errorf("%w: run source authority changed", ErrConflict)
	}
	events, err := queries.CountRunAdmissionEvents(ctx, postgres.CountRunAdmissionEventsParams{
		RunID: run.ID, TaskID: run.TaskID, TaskAggregateVersion: run.AdmittedTaskAggregateVersion,
	})
	if err != nil || events != 2 {
		return postgres.GetTaskAndVersionRow{}, fmt.Errorf("%w: run admission evidence is incomplete", ErrConflict)
	}
	return task, nil
}

func sameCandidate(left, right Candidate) bool {
	return left.ProjectID == right.ProjectID && left.ProjectSourceID == right.ProjectSourceID &&
		left.TaskID == right.TaskID && left.TaskVersionID == right.TaskVersionID &&
		left.ExternalTaskID == right.ExternalTaskID && left.ExpectedAggregateVersion == right.ExpectedAggregateVersion &&
		left.Priority == right.Priority && left.CreatedAt.Equal(right.CreatedAt) &&
		left.SourceCommit == right.SourceCommit && left.SourceTree == right.SourceTree
}

func activeTaskStatus(status string) bool {
	return oneOf(status, "admitted", "planning", "ready", "working", "verifying", "auditing", "correcting", "documenting", "simplifying", "finalizing")
}

func releasableTaskStatus(status string) bool {
	return oneOf(status, "needs_input", "blocked", "completed", "cancelled", "budget_exhausted", "unsafe", "superseded", "abandoned")
}

func activeRunWithoutLease(runs []postgres.CoreRun) error {
	diagnostics := make([]Diagnostic, 0, len(runs))
	for _, run := range runs {
		diagnostics = append(diagnostics, diagnostic("active_run_without_lease", uuidString(run.TaskID), "", fmt.Sprintf("active run %s has no global lease", uuidString(run.ID))))
	}
	return &UnsafeGraphError{Diagnostics: diagnostics}
}

func leaseBusy(lease postgres.CoreExecutionLease) error {
	return &LeaseBusyError{RunID: uuidString(lease.RunID), CoordinatorIdentity: lease.CoordinatorIdentity.String}
}

func parseUUID(name, value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("scheduler: invalid %s: %w", name, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func parseUUIDv7(name, value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Version() != 7 {
		return pgtype.UUID{}, fmt.Errorf("scheduler: %s must be UUIDv7", name)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func newUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func text(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
