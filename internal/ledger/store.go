package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
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
	driverName         = "sqlite"
	defaultRecentLimit = 20

	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS runs (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	task TEXT NOT NULL,
	status TEXT NOT NULL,
	summary TEXT,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	duration_seconds INTEGER NOT NULL DEFAULT 0,
	codex_exit_code INTEGER,
	verification_status TEXT,
	commit_sha TEXT
);

CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
	event_type TEXT NOT NULL,
	event_data TEXT,
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_run ON events(run_id, id);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
`

type Store struct {
	db    *sql.DB
	clock func() time.Time
}

type RunSpec struct {
	ID                 string
	TaskID             string
	Task               string
	Status             string
	Summary            string
	StartedAt          time.Time
	CompletedAt        *time.Time
	DurationSeconds    int
	CodexExitCode      *int
	VerificationStatus string
	CommitSHA          string
}

type RunCompletion struct {
	Status             string
	Summary            string
	CompletedAt        time.Time
	CodexExitCode      *int
	VerificationStatus string
	CommitSHA          string
}

type Run struct {
	ID                 string
	TaskID             string
	Task               string
	Status             string
	Summary            string
	StartedAt          time.Time
	CompletedAt        *time.Time
	DurationSeconds    int
	CodexExitCode      *int
	VerificationStatus string
	CommitSHA          string
}

type Event struct {
	ID        int64
	RunID     string
	Type      EventType
	Payload   json.RawMessage
	CreatedAt time.Time
}

type RunWithEvents struct {
	Run    Run
	Events []Event
}

func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithClock(ctx, path, nil)
}

func OpenWithClock(ctx context.Context, path string, clk func() time.Time) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("open ledger: path is required")
	}
	if err := ensureDBDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("open ledger: %w", err)
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
		return nil, errors.New("new ledger store: db is nil")
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

func (s *Store) CreateRun(ctx context.Context, spec RunSpec) (Run, error) {
	if err := s.ready(); err != nil {
		return Run{}, err
	}
	run, err := s.normalizeRunSpec(spec)
	if err != nil {
		return Run{}, err
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO runs (
	id,
	task_id,
	task,
	status,
	summary,
	started_at,
	completed_at,
	duration_seconds,
	codex_exit_code,
	verification_status,
	commit_sha
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.TaskID,
		run.Task,
		run.Status,
		nullableString(run.Summary),
		formatTime(run.StartedAt),
		nullableTime(run.CompletedAt),
		run.DurationSeconds,
		nullableInt(run.CodexExitCode),
		nullableString(run.VerificationStatus),
		nullableString(run.CommitSHA),
	)
	if err != nil {
		return Run{}, fmt.Errorf("create run: %w", err)
	}
	return run, nil
}

func (s *Store) CompleteRun(ctx context.Context, runID string, completion RunCompletion) (Run, bool, error) {
	if err := s.ready(); err != nil {
		return Run{}, false, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Run{}, false, errors.New("complete run: run id is required")
	}

	existing, ok, err := s.GetRun(ctx, runID)
	if err != nil || !ok {
		return Run{}, ok, err
	}

	status := strings.TrimSpace(completion.Status)
	if status == "" {
		status = StatusCompleted
	}
	if !validStatus(status) {
		return Run{}, false, fmt.Errorf("complete run: invalid status %q", status)
	}

	completedAt := completion.CompletedAt
	if completedAt.IsZero() {
		completedAt = s.clock()
	}
	completedAt = completedAt.UTC()
	durationSeconds := int(completedAt.Sub(existing.StartedAt).Seconds())
	if durationSeconds < 0 {
		durationSeconds = 0
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE runs
SET
	status = ?,
	summary = ?,
	completed_at = ?,
	duration_seconds = ?,
	codex_exit_code = ?,
	verification_status = ?,
	commit_sha = ?
WHERE id = ?`,
		status,
		nullableString(completion.Summary),
		formatTime(completedAt),
		durationSeconds,
		nullableInt(completion.CodexExitCode),
		nullableString(completion.VerificationStatus),
		nullableString(completion.CommitSHA),
		runID,
	)
	if err != nil {
		return Run{}, false, fmt.Errorf("complete run: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Run{}, false, fmt.Errorf("complete run: read affected rows: %w", err)
	}
	if rowsAffected == 0 {
		return Run{}, false, nil
	}
	updated, ok, err := s.GetRun(ctx, runID)
	if err != nil || !ok {
		return Run{}, ok, err
	}
	return updated, true, nil
}

func (s *Store) AppendEvent(ctx context.Context, runID string, eventType EventType, payload any) (Event, error) {
	if err := s.ready(); err != nil {
		return Event{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return Event{}, errors.New("append event: run id is required")
	}
	if strings.TrimSpace(string(eventType)) == "" {
		return Event{}, errors.New("append event: event type is required")
	}
	eventData, err := marshalPayload(payload)
	if err != nil {
		return Event{}, err
	}

	createdAt := s.clock().UTC()
	result, err := s.db.ExecContext(ctx, `
INSERT INTO events (run_id, event_type, event_data, created_at)
VALUES (?, ?, ?, ?)`,
		runID,
		string(eventType),
		eventData,
		formatTime(createdAt),
	)
	if err != nil {
		return Event{}, fmt.Errorf("append event: %w", err)
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return Event{}, fmt.Errorf("append event: read inserted id: %w", err)
	}

	var raw json.RawMessage
	if eventData.Valid {
		raw = json.RawMessage(eventData.String)
	}
	return Event{ID: eventID, RunID: runID, Type: eventType, Payload: raw, CreatedAt: createdAt}, nil
}

func (s *Store) RecordCommitSHA(ctx context.Context, runID string, commitSHA string) error {
	if err := s.ready(); err != nil {
		return err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return errors.New("record commit sha: run id is required")
	}
	commitSHA = strings.TrimSpace(commitSHA)
	if commitSHA == "" {
		return errors.New("record commit sha: commit sha is required")
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE runs
SET commit_sha = ?
WHERE id = ?`, commitSHA, runID)
	if err != nil {
		return fmt.Errorf("record commit sha: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record commit sha: read affected rows: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("record commit sha: run %q not found", runID)
	}
	return nil
}

func (s *Store) ListRecentRuns(ctx context.Context, limit int) ([]Run, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultRecentLimit
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT
	id,
	task_id,
	task,
	status,
	summary,
	started_at,
	completed_at,
	duration_seconds,
	codex_exit_code,
	verification_status,
	commit_sha
FROM runs
ORDER BY started_at DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list recent runs: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recent runs: %w", err)
	}
	return runs, nil
}

func (s *Store) GetRun(ctx context.Context, runID string) (Run, bool, error) {
	if err := s.ready(); err != nil {
		return Run{}, false, err
	}
	if strings.TrimSpace(runID) == "" {
		return Run{}, false, errors.New("get run: run id is required")
	}

	run, err := scanRun(s.db.QueryRowContext(ctx, `
SELECT
	id,
	task_id,
	task,
	status,
	summary,
	started_at,
	completed_at,
	duration_seconds,
	codex_exit_code,
	verification_status,
	commit_sha
FROM runs
WHERE id = ?`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, fmt.Errorf("get run: %w", err)
	}
	return run, true, nil
}

func (s *Store) GetRunWithEvents(ctx context.Context, runID string) (RunWithEvents, bool, error) {
	run, ok, err := s.GetRun(ctx, runID)
	if err != nil || !ok {
		return RunWithEvents{}, ok, err
	}

	events, err := s.listEvents(ctx, runID)
	if err != nil {
		return RunWithEvents{}, false, err
	}
	return RunWithEvents{Run: run, Events: events}, true, nil
}

func (s *Store) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("init ledger: enable foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("init ledger: set busy timeout: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("init ledger schema: %w", err)
	}
	return nil
}

func (s *Store) ready() error {
	if s == nil || s.db == nil {
		return errors.New("ledger store is nil")
	}
	return nil
}

func (s *Store) normalizeRunSpec(spec RunSpec) (Run, error) {
	runID := spec.ID
	if runID == "" {
		runID = id.New()
	}
	if strings.TrimSpace(runID) == "" {
		return Run{}, errors.New("create run: id is required")
	}
	if strings.TrimSpace(spec.TaskID) == "" {
		return Run{}, errors.New("create run: task id is required")
	}
	if strings.TrimSpace(spec.Task) == "" {
		return Run{}, errors.New("create run: task is required")
	}
	status := spec.Status
	if status == "" {
		status = StatusRunning
	}
	if strings.TrimSpace(status) == "" {
		return Run{}, errors.New("create run: status is required")
	}
	if spec.DurationSeconds < 0 {
		return Run{}, errors.New("create run: duration seconds cannot be negative")
	}
	startedAt := spec.StartedAt
	if startedAt.IsZero() {
		startedAt = s.clock()
	}

	return Run{
		ID:                 runID,
		TaskID:             spec.TaskID,
		Task:               spec.Task,
		Status:             status,
		Summary:            spec.Summary,
		StartedAt:          startedAt.UTC(),
		CompletedAt:        utcPtr(spec.CompletedAt),
		DurationSeconds:    spec.DurationSeconds,
		CodexExitCode:      cloneInt(spec.CodexExitCode),
		VerificationStatus: spec.VerificationStatus,
		CommitSHA:          spec.CommitSHA,
	}, nil
}

func validStatus(status string) bool {
	switch status {
	case StatusRunning, StatusCompleted, StatusFailed:
		return true
	default:
		return false
	}
}

func (s *Store) listEvents(ctx context.Context, runID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, event_type, event_data, created_at
FROM events
WHERE run_id = ?
ORDER BY id ASC`, runID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("list events: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var summary sql.NullString
	var startedAt string
	var completedAt sql.NullString
	var codexExitCode sql.NullInt64
	var verificationStatus sql.NullString
	var commitSHA sql.NullString
	if err := row.Scan(
		&run.ID,
		&run.TaskID,
		&run.Task,
		&run.Status,
		&summary,
		&startedAt,
		&completedAt,
		&run.DurationSeconds,
		&codexExitCode,
		&verificationStatus,
		&commitSHA,
	); err != nil {
		return Run{}, err
	}

	parsedStartedAt, err := parseTime(startedAt)
	if err != nil {
		return Run{}, fmt.Errorf("parse started_at: %w", err)
	}
	run.StartedAt = parsedStartedAt
	run.Summary = stringFromNull(summary)
	run.CompletedAt, err = timePtrFromNull(completedAt)
	if err != nil {
		return Run{}, fmt.Errorf("parse completed_at: %w", err)
	}
	run.CodexExitCode = intPtrFromNull(codexExitCode)
	run.VerificationStatus = stringFromNull(verificationStatus)
	run.CommitSHA = stringFromNull(commitSHA)
	return run, nil
}

func scanEvent(row scanner) (Event, error) {
	var event Event
	var eventType string
	var eventData sql.NullString
	var createdAt string
	if err := row.Scan(&event.ID, &event.RunID, &eventType, &eventData, &createdAt); err != nil {
		return Event{}, err
	}
	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return Event{}, fmt.Errorf("parse created_at: %w", err)
	}
	event.Type = EventType(eventType)
	event.CreatedAt = parsedCreatedAt
	if eventData.Valid {
		if !json.Valid([]byte(eventData.String)) {
			return Event{}, errors.New("event payload is not valid JSON")
		}
		event.Payload = json.RawMessage(eventData.String)
	}
	return event, nil
}

func marshalPayload(payload any) (sql.NullString, error) {
	if payload == nil {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("append event: marshal payload: %w", err)
	}
	if !json.Valid(b) {
		return sql.NullString{}, errors.New("append event: payload is not valid JSON")
	}
	return sql.NullString{String: string(b), Valid: true}, nil
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

func nullableInt(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func intPtrFromNull(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func utcPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	t := value.UTC()
	return &t
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
		return fmt.Errorf("open ledger: create db directory: %w", err)
	}
	return nil
}
