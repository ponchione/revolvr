package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"

	"revolvr/internal/id"
)

const (
	driverName         = "sqlite"
	defaultRecentLimit = 20
	liveBusySlice      = 25 * time.Millisecond
	liveBusyLimit      = 5 * time.Second

	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

var ErrReadOnly = errors.New("ledger store is read-only")

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
	db           *sql.DB
	clock        func() time.Time
	liveReadOnly bool
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

// Snapshot is an exact, deterministic read model for export and retention.
// Runs are ordered by start time then ID and events retain their global IDs.
type Snapshot struct {
	Runs       []RunWithEvents
	MaxEventID int64
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

// OpenLiveReadOnly opens a live ledger without creating parent directories,
// initializing schema, or applying migrations. It uses ordinary SQLite
// locking and change detection; immutable mode is unsafe for a live database.
// Short driver-level busy waits are retried up to the existing five-second
// bound so caller cancellation remains prompt.
func OpenLiveReadOnly(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("open ledger read-only: path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open ledger read-only: resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("open ledger read-only: inspect %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("open ledger read-only: %s is a directory", path)
	}

	dsnURL := &url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}
	query := dsnURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", liveBusySlice.Milliseconds()))
	query.Add("_pragma", "query_only(1)")
	dsnURL.RawQuery = query.Encode()
	db, err := sql.Open(driverName, dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("open ledger read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open ledger read-only: %w", err)
	}
	return &Store{db: db, liveReadOnly: true}, nil
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
	if err := s.writable(); err != nil {
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
	if err := s.writable(); err != nil {
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
	if err := s.writable(); err != nil {
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
	if err := s.writable(); err != nil {
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
	return retryLiveRead(ctx, s.liveReadOnly, func() ([]Run, error) {
		return s.listRecentRunsOnce(ctx, limit)
	})
}

func (s *Store) listRecentRunsOnce(ctx context.Context, limit int) ([]Run, error) {
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

// ListRecentRunsForTaskWithEvents filters by exact task identity before
// applying the explicit limit. Zero returns no history; negative limits are
// rejected. Runs are newest-first with ascending run ID as the stable
// timestamp tie-breaker, and events are ordered by ledger event ID.
func (s *Store) ListRecentRunsForTaskWithEvents(ctx context.Context, taskID string, limit int) ([]RunWithEvents, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, errors.New("list recent task runs: task id is required")
	}
	if limit < 0 {
		return nil, fmt.Errorf("list recent task runs: limit cannot be negative (got %d; zero selects no runs)", limit)
	}
	if limit == 0 {
		return nil, nil
	}
	return retryLiveRead(ctx, s.liveReadOnly, func() ([]RunWithEvents, error) {
		return s.listRecentRunsForTaskWithEventsOnce(ctx, taskID, limit)
	})
}

func (s *Store) listRecentRunsForTaskWithEventsOnce(ctx context.Context, taskID string, limit int) ([]RunWithEvents, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("list recent task runs: begin snapshot: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
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
WHERE task_id = ?
ORDER BY started_at DESC, id ASC
LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent task runs: %w", err)
	}
	defer rows.Close()

	var runs []Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list recent task runs: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("list recent task runs: close runs: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recent task runs: %w", err)
	}

	history := make([]RunWithEvents, 0, len(runs))
	for _, run := range runs {
		history = append(history, RunWithEvents{Run: run})
	}
	_, err = appendEvidenceEvents(ctx, tx, history, `
SELECT events.id, events.run_id, events.event_type, events.event_data, events.created_at
FROM events
JOIN (
	SELECT id, started_at
	FROM runs
	WHERE task_id = ?
	ORDER BY started_at DESC, id ASC
	LIMIT ?
) AS selected_runs ON selected_runs.id = events.run_id
ORDER BY selected_runs.started_at DESC, selected_runs.id ASC, events.id ASC`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent task runs: events: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("list recent task runs: commit snapshot: %w", err)
	}
	return history, nil
}

func (s *Store) GetRun(ctx context.Context, runID string) (Run, bool, error) {
	if err := s.ready(); err != nil {
		return Run{}, false, err
	}
	if strings.TrimSpace(runID) == "" {
		return Run{}, false, errors.New("get run: run id is required")
	}

	result, err := retryLiveRead(ctx, s.liveReadOnly, func() (runLookup, error) {
		run, queryErr := getRun(ctx, s.db, runID)
		if errors.Is(queryErr, sql.ErrNoRows) {
			return runLookup{}, nil
		}
		return runLookup{run: run, found: queryErr == nil}, queryErr
	})
	if err != nil {
		return Run{}, false, fmt.Errorf("get run: %w", err)
	}
	return result.run, result.found, nil
}

type runLookup struct {
	run   Run
	found bool
}

func getRun(ctx context.Context, q queryer, runID string) (Run, error) {
	return scanRun(q.QueryRowContext(ctx, `
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
}

func (s *Store) GetRunWithEvents(ctx context.Context, runID string) (RunWithEvents, bool, error) {
	if err := s.ready(); err != nil {
		return RunWithEvents{}, false, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return RunWithEvents{}, false, errors.New("get run: run id is required")
	}
	result, err := retryLiveRead(ctx, s.liveReadOnly, func() (runWithEventsLookup, error) {
		return s.getRunWithEventsOnce(ctx, runID)
	})
	if err != nil {
		return RunWithEvents{}, false, err
	}
	return result.history, result.found, nil
}

type runWithEventsLookup struct {
	history RunWithEvents
	found   bool
}

func (s *Store) getRunWithEventsOnce(ctx context.Context, runID string) (runWithEventsLookup, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return runWithEventsLookup{}, fmt.Errorf("get run with events: begin snapshot: %w", err)
	}
	defer tx.Rollback()
	run, err := getRun(ctx, tx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return runWithEventsLookup{}, nil
	}
	if err != nil {
		return runWithEventsLookup{}, fmt.Errorf("get run with events: get run: %w", err)
	}

	events, err := listEvents(ctx, tx, runID)
	if err != nil {
		return runWithEventsLookup{}, err
	}
	if err := tx.Commit(); err != nil {
		return runWithEventsLookup{}, fmt.Errorf("get run with events: commit snapshot: %w", err)
	}
	return runWithEventsLookup{history: RunWithEvents{Run: run, Events: events}, found: true}, nil
}

// ReadSnapshot reads all runs and their raw event payloads from one SQLite
// read transaction. It never initializes or mutates the ledger.
func (s *Store) ReadSnapshot(ctx context.Context) (Snapshot, error) {
	if err := s.ready(); err != nil {
		return Snapshot{}, err
	}
	return retryLiveRead(ctx, s.liveReadOnly, func() (Snapshot, error) {
		return s.readSnapshotOnce(ctx)
	})
}

func (s *Store) readSnapshotOnce(ctx context.Context) (Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, fmt.Errorf("read ledger snapshot: begin: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
SELECT id, task_id, task, status, summary, started_at, completed_at,
       duration_seconds, codex_exit_code, verification_status, commit_sha
FROM runs ORDER BY started_at ASC, id ASC`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read ledger snapshot: runs: %w", err)
	}
	history := make([]RunWithEvents, 0)
	for rows.Next() {
		run, scanErr := scanRun(rows)
		if scanErr != nil {
			rows.Close()
			return Snapshot{}, fmt.Errorf("read ledger snapshot: run: %w", scanErr)
		}
		history = append(history, RunWithEvents{Run: run})
	}
	if err := rows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("read ledger snapshot: close runs: %w", err)
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("read ledger snapshot: runs: %w", err)
	}

	out := Snapshot{Runs: history}
	out.MaxEventID, err = appendEvidenceEvents(ctx, tx, out.Runs, `
SELECT events.id, events.run_id, events.event_type, events.event_data, events.created_at
FROM events
JOIN runs ON runs.id = events.run_id
ORDER BY events.id ASC`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read ledger snapshot: events: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("read ledger snapshot: commit read transaction: %w", err)
	}
	return out, nil
}

// appendEvidenceEvents preserves malformed JSON payload bytes while merging
// query-ordered events directly into the already ordered run result.
func appendEvidenceEvents(ctx context.Context, q queryer, history []RunWithEvents, query string, args ...any) (int64, error) {
	if len(history) == 0 {
		return 0, nil
	}
	runIndexes := make(map[string]int, len(history))
	for i := range history {
		runIndexes[history[i].Run.ID] = i
	}

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	var maxEventID int64
	for rows.Next() {
		event, scanErr := scanEvidenceEvent(rows)
		if scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("scan event: %w", scanErr)
		}
		runIndex, ok := runIndexes[event.RunID]
		if !ok {
			rows.Close()
			return 0, fmt.Errorf("event %d references unselected run %q", event.ID, event.RunID)
		}
		history[runIndex].Events = append(history[runIndex].Events, event)
		if event.ID > maxEventID {
			maxEventID = event.ID
		}
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close events: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return maxEventID, nil
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

func (s *Store) writable() error {
	if err := s.ready(); err != nil {
		return err
	}
	if s.liveReadOnly {
		return ErrReadOnly
	}
	return nil
}

func retryLiveRead[T any](ctx context.Context, enabled bool, operation func() (T, error)) (T, error) {
	if !enabled {
		return operation()
	}
	deadline := time.Now().Add(liveBusyLimit)
	for {
		value, err := operation()
		if err == nil || !isSQLiteBusy(err) {
			return value, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			var zero T
			return zero, errors.Join(ctxErr, err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return value, err
		}
		pause := liveBusySlice
		if remaining < pause {
			pause = remaining
		}
		timer := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			var zero T
			return zero, errors.Join(ctx.Err(), err)
		case <-timer.C:
		}
	}
}

func isSQLiteBusy(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	const (
		sqliteBusy   = 5
		sqliteLocked = 6
	)
	return sqliteErr.Code()&0xff == sqliteBusy || sqliteErr.Code()&0xff == sqliteLocked
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

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func listEvents(ctx context.Context, q queryer, runID string) ([]Event, error) {
	rows, err := q.QueryContext(ctx, `
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

func scanEvidenceEvent(row scanner) (Event, error) {
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
