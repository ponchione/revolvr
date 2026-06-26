package taskqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"revolvr/internal/id"
)

const (
	driverName = "sqlite"

	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	task TEXT NOT NULL,
	status TEXT NOT NULL,
	summary TEXT,
	blocker TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	completed_at TEXT,
	blocked_at TEXT,
	CHECK (status IN ('pending', 'completed', 'blocked'))
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_order ON tasks(status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at, id);
`

type Store struct {
	db    *sql.DB
	clock func() time.Time
}

type TaskSpec struct {
	ID        string
	Task      string
	Summary   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Task struct {
	ID          string
	Task        string
	Status      string
	Summary     string
	Blocker     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	BlockedAt   *time.Time
}

func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithClock(ctx, path, nil)
}

func OpenWithClock(ctx context.Context, path string, clk func() time.Time) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("open task queue: path is required")
	}
	if err := ensureDBDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("open task queue: %w", err)
	}
	db.SetMaxOpenConns(1)

	store, err := StoreWithClock(ctx, db, clk)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func NewStore(ctx context.Context, db *sql.DB) (*Store, error) {
	return StoreWithClock(ctx, db, nil)
}

func StoreWithClock(ctx context.Context, db *sql.DB, clk func() time.Time) (*Store, error) {
	if db == nil {
		return nil, errors.New("new task queue store: db is nil")
	}
	db.SetMaxOpenConns(1)
	if clk == nil {
		clk = time.Now
	}
	s := &Store{db: db, clock: clk}
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) AddTask(ctx context.Context, spec TaskSpec) (Task, error) {
	if err := s.ready(); err != nil {
		return Task{}, err
	}
	task, err := s.normalizeTaskSpec(spec)
	if err != nil {
		return Task{}, err
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO tasks (
	id,
	task,
	status,
	summary,
	blocker,
	created_at,
	updated_at,
	completed_at,
	blocked_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID,
		task.Task,
		task.Status,
		nullableString(task.Summary),
		nullableString(task.Blocker),
		formatTime(task.CreatedAt),
		formatTime(task.UpdatedAt),
		nullableTime(task.CompletedAt),
		nullableTime(task.BlockedAt),
	)
	if err != nil {
		return Task{}, fmt.Errorf("add task: %w", err)
	}
	return task, nil
}

func (s *Store) ListTasks(ctx context.Context) ([]Task, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT
	id,
	task,
	status,
	summary,
	blocker,
	created_at,
	updated_at,
	completed_at,
	blocked_at
FROM tasks
ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("list tasks: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

func (s *Store) SelectNext(ctx context.Context) (Task, bool, error) {
	if err := s.ready(); err != nil {
		return Task{}, false, err
	}

	task, err := scanTask(s.db.QueryRowContext(ctx, `
SELECT
	id,
	task,
	status,
	summary,
	blocker,
	created_at,
	updated_at,
	completed_at,
	blocked_at
FROM tasks
WHERE status = ? AND (blocker IS NULL OR blocker = '')
ORDER BY created_at ASC, id ASC
LIMIT 1`, StatusPending))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, fmt.Errorf("select next task: %w", err)
	}
	return task, true, nil
}

func (s *Store) GetTask(ctx context.Context, taskID string) (Task, bool, error) {
	if err := s.ready(); err != nil {
		return Task{}, false, err
	}
	if strings.TrimSpace(taskID) == "" {
		return Task{}, false, errors.New("get task: task id is required")
	}

	task, err := scanTask(s.db.QueryRowContext(ctx, `
SELECT
	id,
	task,
	status,
	summary,
	blocker,
	created_at,
	updated_at,
	completed_at,
	blocked_at
FROM tasks
WHERE id = ?`, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, false, nil
	}
	if err != nil {
		return Task{}, false, fmt.Errorf("get task: %w", err)
	}
	return task, true, nil
}

func (s *Store) CompleteTask(ctx context.Context, taskID string, summary string) (Task, bool, error) {
	if err := s.ready(); err != nil {
		return Task{}, false, err
	}
	if strings.TrimSpace(taskID) == "" {
		return Task{}, false, errors.New("complete task: task id is required")
	}

	now := s.clock().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET
	status = ?,
	summary = ?,
	blocker = NULL,
	updated_at = ?,
	completed_at = ?,
	blocked_at = NULL
WHERE id = ?`,
		StatusCompleted,
		nullableString(summary),
		formatTime(now),
		formatTime(now),
		taskID,
	)
	if err != nil {
		return Task{}, false, fmt.Errorf("complete task: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Task{}, false, fmt.Errorf("complete task: read affected rows: %w", err)
	}
	if changed == 0 {
		return Task{}, false, nil
	}
	return s.getExistingTask(ctx, taskID, "complete task")
}

func (s *Store) BlockTask(ctx context.Context, taskID string, blocker string) (Task, bool, error) {
	if err := s.ready(); err != nil {
		return Task{}, false, err
	}
	if strings.TrimSpace(taskID) == "" {
		return Task{}, false, errors.New("block task: task id is required")
	}
	if strings.TrimSpace(blocker) == "" {
		return Task{}, false, errors.New("block task: blocker is required")
	}

	now := s.clock().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET
	status = ?,
	blocker = ?,
	updated_at = ?,
	completed_at = NULL,
	blocked_at = ?
WHERE id = ?`,
		StatusBlocked,
		blocker,
		formatTime(now),
		formatTime(now),
		taskID,
	)
	if err != nil {
		return Task{}, false, fmt.Errorf("block task: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Task{}, false, fmt.Errorf("block task: read affected rows: %w", err)
	}
	if changed == 0 {
		return Task{}, false, nil
	}
	return s.getExistingTask(ctx, taskID, "block task")
}

func (s *Store) UnblockTask(ctx context.Context, taskID string) (Task, bool, error) {
	if err := s.ready(); err != nil {
		return Task{}, false, err
	}
	if strings.TrimSpace(taskID) == "" {
		return Task{}, false, errors.New("unblock task: task id is required")
	}

	now := s.clock().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET
	status = ?,
	blocker = NULL,
	updated_at = ?,
	completed_at = NULL,
	blocked_at = NULL
WHERE id = ? AND status = ?`,
		StatusPending,
		formatTime(now),
		taskID,
		StatusBlocked,
	)
	if err != nil {
		return Task{}, false, fmt.Errorf("unblock task: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Task{}, false, fmt.Errorf("unblock task: read affected rows: %w", err)
	}
	if changed == 0 {
		task, ok, err := s.GetTask(ctx, taskID)
		if err != nil {
			return Task{}, false, fmt.Errorf("unblock task: %w", err)
		}
		if !ok {
			return Task{}, false, nil
		}
		return task, false, nil
	}
	return s.getExistingTask(ctx, taskID, "unblock task")
}

func (s *Store) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("init task queue: enable foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("init task queue: set busy timeout: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("init task queue schema: %w", err)
	}
	return nil
}

func (s *Store) ready() error {
	if s == nil || s.db == nil {
		return errors.New("task queue store is nil")
	}
	return nil
}

func (s *Store) normalizeTaskSpec(spec TaskSpec) (Task, error) {
	taskID := spec.ID
	if taskID == "" {
		taskID = id.New()
	}
	if strings.TrimSpace(taskID) == "" {
		return Task{}, errors.New("add task: id is required")
	}
	if strings.TrimSpace(spec.Task) == "" {
		return Task{}, errors.New("add task: task is required")
	}

	createdAt := spec.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.clock()
	}
	updatedAt := spec.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	return Task{
		ID:        taskID,
		Task:      spec.Task,
		Status:    StatusPending,
		Summary:   spec.Summary,
		CreatedAt: createdAt.UTC(),
		UpdatedAt: updatedAt.UTC(),
	}, nil
}

func (s *Store) getExistingTask(ctx context.Context, taskID string, operation string) (Task, bool, error) {
	task, ok, err := s.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !ok {
		return Task{}, false, fmt.Errorf("%s: updated task was not found", operation)
	}
	return task, true, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (Task, error) {
	var task Task
	var summary sql.NullString
	var blocker sql.NullString
	var createdAt string
	var updatedAt string
	var completedAt sql.NullString
	var blockedAt sql.NullString
	if err := row.Scan(
		&task.ID,
		&task.Task,
		&task.Status,
		&summary,
		&blocker,
		&createdAt,
		&updatedAt,
		&completedAt,
		&blockedAt,
	); err != nil {
		return Task{}, err
	}

	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse created_at: %w", err)
	}
	parsedUpdatedAt, err := parseTime(updatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse updated_at: %w", err)
	}
	task.CreatedAt = parsedCreatedAt
	task.UpdatedAt = parsedUpdatedAt
	task.Summary = stringFromNull(summary)
	task.Blocker = stringFromNull(blocker)
	task.CompletedAt, err = timePtrFromNull(completedAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse completed_at: %w", err)
	}
	task.BlockedAt, err = timePtrFromNull(blockedAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse blocked_at: %w", err)
	}
	return task, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func stringFromNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*value), Valid: true}
}

func timePtrFromNull(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	t, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func ensureDBDir(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("open task queue: create db directory: %w", err)
	}
	return nil
}
