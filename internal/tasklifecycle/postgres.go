package tasklifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/storage/postgres"
)

var (
	ErrUnavailable     = errors.New("lifecycle transition is not available in this slice")
	ErrConflict        = errors.New("task aggregate version conflict")
	ErrNotFound        = errors.New("task or task version not found for project")
	ErrAlreadyApproved = errors.New("task already has an accepted version")
)

type TransitionCommand struct {
	ProjectID                string
	TaskID                   string
	TaskVersionID            string
	ExpectedAggregateVersion int64
	NextStatus               TaskStatus
	Authority                Authority
}

type ApprovalCommand struct {
	ProjectID                string
	TaskID                   string
	TaskVersionID            string
	ExpectedAggregateVersion int64
	OperatorIdentity         string
}

type Result struct {
	ProjectID        string
	TaskID           string
	TaskVersionID    string
	PreviousStatus   TaskStatus
	Status           TaskStatus
	AggregateVersion int64
	EventID          string
	UpdatedAt        time.Time
}

type eventPayload struct {
	ProjectID                   string `json:"project_id"`
	TaskID                      string `json:"task_id"`
	TaskVersionID               string `json:"task_version_id"`
	TaskVersionNumber           int32  `json:"task_version_number"`
	TaskVersionSourceArtifactID string `json:"task_version_source_artifact_id"`
	Authority                   string `json:"authority"`
	OperatorIdentity            string `json:"operator_identity"`
	PreviousStatus              string `json:"previous_status"`
	NewStatus                   string `json:"new_status"`
	ExpectedAggregateVersion    int64  `json:"expected_aggregate_version"`
	AggregateVersion            int64  `json:"aggregate_version"`
}

func Transition(ctx context.Context, pool *pgxpool.Pool, command TransitionCommand) (Result, error) {
	if pool == nil {
		return Result{}, errors.New("task lifecycle: PostgreSQL pool is required")
	}
	ids, err := parseIDs(command.ProjectID, command.TaskID, command.TaskVersionID)
	if err != nil {
		return Result{}, err
	}
	if command.ExpectedAggregateVersion < 1 {
		return Result{}, errors.New("task lifecycle: expected aggregate version must be positive")
	}

	var result Result
	queries := postgres.New(pool)
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		current, err := getTaskAndVersion(ctx, qtx, ids)
		if err != nil {
			return err
		}
		if current.AggregateVersion != command.ExpectedAggregateVersion {
			return ErrConflict
		}
		previous := TaskStatus(current.Status)
		if err := ValidateTaskTransition(previous, command.NextStatus, command.Authority); err != nil {
			return err
		}

		eventType := ""
		switch {
		case previous == TaskDraft && command.NextStatus == TaskCompiled:
			eventType = "task.compiled"
		case previous == TaskCompiled && command.NextStatus == TaskAwaitingApproval:
			eventType = "task.awaiting_approval"
		default:
			return ErrUnavailable
		}

		updatedAt := time.Now().UTC().Truncate(time.Microsecond)
		updated, err := qtx.CompareAndUpdateTaskState(ctx, postgres.CompareAndUpdateTaskStateParams{
			NewStatus: string(command.NextStatus), UpdatedAt: timestamp(updatedAt),
			ProjectID: ids.project, TaskID: ids.task, ExpectedStatus: current.Status,
			ExpectedAggregateVersion: command.ExpectedAggregateVersion,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		eventID, err := appendEvent(ctx, qtx, ids, current, updated.AggregateVersion,
			eventType, command.Authority, "", previous, command.NextStatus,
			command.ExpectedAggregateVersion, updatedAt)
		if err != nil {
			return err
		}
		result = lifecycleResult(ids, previous, command.NextStatus, updated.AggregateVersion, eventID, updatedAt)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("task lifecycle transition: %w", err)
	}
	return result, nil
}

func Approve(ctx context.Context, pool *pgxpool.Pool, command ApprovalCommand) (Result, error) {
	if pool == nil {
		return Result{}, errors.New("task lifecycle: PostgreSQL pool is required")
	}
	operator := strings.TrimSpace(command.OperatorIdentity)
	if operator == "" {
		return Result{}, errors.New("task lifecycle: operator identity is required")
	}
	ids, err := parseIDs(command.ProjectID, command.TaskID, command.TaskVersionID)
	if err != nil {
		return Result{}, err
	}
	if command.ExpectedAggregateVersion < 1 {
		return Result{}, errors.New("task lifecycle: expected aggregate version must be positive")
	}

	var result Result
	queries := postgres.New(pool)
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := queries.WithTx(tx)
		current, err := getTaskAndVersion(ctx, qtx, ids)
		if err != nil {
			return err
		}
		if current.AggregateVersion != command.ExpectedAggregateVersion {
			return ErrConflict
		}
		if current.AcceptedVersionID.Valid {
			return ErrAlreadyApproved
		}
		previous := TaskStatus(current.Status)
		if err := ValidateTaskTransition(previous, TaskPending, AuthorityOperator); err != nil {
			return err
		}

		updatedAt := time.Now().UTC().Truncate(time.Microsecond)
		updated, err := qtx.ApproveTaskVersion(ctx, postgres.ApproveTaskVersionParams{
			TaskVersionID: ids.version, UpdatedAt: timestamp(updatedAt), ProjectID: ids.project,
			TaskID: ids.task, ExpectedAggregateVersion: command.ExpectedAggregateVersion,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		eventID, err := appendEvent(ctx, qtx, ids, current, updated.AggregateVersion,
			"task.approved", AuthorityOperator, operator, previous, TaskPending,
			command.ExpectedAggregateVersion, updatedAt)
		if err != nil {
			return err
		}
		result = lifecycleResult(ids, previous, TaskPending, updated.AggregateVersion, eventID, updatedAt)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("task approval: %w", err)
	}
	return result, nil
}

type identifiers struct {
	project pgtype.UUID
	task    pgtype.UUID
	version pgtype.UUID
}

func parseIDs(projectID, taskID, versionID string) (identifiers, error) {
	parse := func(name, value string) (pgtype.UUID, error) {
		parsed, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("task lifecycle: invalid %s: %w", name, err)
		}
		return pgtype.UUID{Bytes: parsed, Valid: true}, nil
	}
	project, err := parse("project id", projectID)
	if err != nil {
		return identifiers{}, err
	}
	task, err := parse("task id", taskID)
	if err != nil {
		return identifiers{}, err
	}
	version, err := parse("task version id", versionID)
	if err != nil {
		return identifiers{}, err
	}
	return identifiers{project: project, task: task, version: version}, nil
}

func getTaskAndVersion(ctx context.Context, queries *postgres.Queries, ids identifiers) (postgres.GetTaskAndVersionRow, error) {
	row, err := queries.GetTaskAndVersion(ctx, postgres.GetTaskAndVersionParams{
		ProjectID: ids.project, TaskID: ids.task, TaskVersionID: ids.version,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.GetTaskAndVersionRow{}, ErrNotFound
	}
	return row, err
}

func appendEvent(
	ctx context.Context,
	queries *postgres.Queries,
	ids identifiers,
	current postgres.GetTaskAndVersionRow,
	aggregateVersion int64,
	eventType string,
	authority Authority,
	operator string,
	previous, next TaskStatus,
	expectedVersion int64,
	createdAt time.Time,
) (pgtype.UUID, error) {
	payload, err := json.Marshal(eventPayload{
		ProjectID: uuidString(ids.project), TaskID: uuidString(ids.task),
		TaskVersionID: uuidString(ids.version), TaskVersionNumber: current.TaskVersionNumber,
		TaskVersionSourceArtifactID: uuidString(current.TaskVersionSourceArtifactID),
		Authority:                   string(authority), OperatorIdentity: operator,
		PreviousStatus: string(previous), NewStatus: string(next),
		ExpectedAggregateVersion: expectedVersion, AggregateVersion: aggregateVersion,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	eventID := newUUID()
	_, err = queries.AppendEvent(ctx, postgres.AppendEventParams{
		ID: eventID, ProjectID: ids.project, TaskID: ids.task, EventType: eventType,
		AggregateType: "task", AggregateID: ids.task, AggregateVersion: aggregateVersion,
		Payload: payload, CreatedAt: timestamp(createdAt),
	})
	return eventID, err
}

func lifecycleResult(ids identifiers, previous, next TaskStatus, aggregateVersion int64, eventID pgtype.UUID, updatedAt time.Time) Result {
	return Result{
		ProjectID: uuidString(ids.project), TaskID: uuidString(ids.task),
		TaskVersionID: uuidString(ids.version), PreviousStatus: previous, Status: next,
		AggregateVersion: aggregateVersion, EventID: uuidString(eventID), UpdatedAt: updatedAt,
	}
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
