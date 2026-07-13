package ledger

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	modernsqlite "modernc.org/sqlite"
)

func TestReadSnapshotPreservesExactOrderingAndPayloadBytes(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 18, 0, 0, 0, time.UTC)
	store := openTestStore(t, func() time.Time { return base })
	defer store.Close()

	runZ, err := store.CreateRun(ctx, RunSpec{ID: "run-z", TaskID: "task", Task: "z", StartedAt: base})
	if err != nil {
		t.Fatal(err)
	}
	runA, err := store.CreateRun(ctx, RunSpec{ID: "run-a", TaskID: "task", Task: "a", StartedAt: base})
	if err != nil {
		t.Fatal(err)
	}
	runOld, err := store.CreateRun(ctx, RunSpec{ID: "run-old", TaskID: "task", Task: "old", StartedAt: base.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	runEmpty, err := store.CreateRun(ctx, RunSpec{ID: "run-empty", TaskID: "task", Task: "empty", StartedAt: base.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		runID     string
		eventType EventType
		payload   any
		createdAt time.Time
	}{
		{runID: runZ.ID, eventType: EventRunStarted, payload: ` {"sequence": 1} `, createdAt: base.Add(time.Second)},
		{runID: runOld.ID, eventType: EventCodexJSONEvent, payload: `{malformed`, createdAt: base.Add(2 * time.Second)},
		{runID: runZ.ID, eventType: EventRunCompleted, payload: `{"sequence":3}`, createdAt: base.Add(3 * time.Second)},
		{runID: runA.ID, eventType: EventContextBuilt, payload: nil, createdAt: base.Add(4 * time.Second)},
	}
	for _, fixture := range fixtures {
		if _, err := store.db.ExecContext(ctx, `
INSERT INTO events (run_id, event_type, event_data, created_at)
VALUES (?, ?, ?, ?)`, fixture.runID, fixture.eventType, fixture.payload, formatTime(fixture.createdAt)); err != nil {
			t.Fatalf("insert event for %s: %v", fixture.runID, err)
		}
	}

	got, err := store.ReadSnapshot(ctx)
	if err != nil {
		t.Fatalf("ReadSnapshot() error = %v", err)
	}
	want := Snapshot{
		Runs: []RunWithEvents{
			{Run: runOld, Events: []Event{{ID: 2, RunID: runOld.ID, Type: EventCodexJSONEvent, Payload: []byte(`{malformed`), CreatedAt: base.Add(2 * time.Second)}}},
			{Run: runA, Events: []Event{{ID: 4, RunID: runA.ID, Type: EventContextBuilt, CreatedAt: base.Add(4 * time.Second)}}},
			{Run: runZ, Events: []Event{
				{ID: 1, RunID: runZ.ID, Type: EventRunStarted, Payload: []byte(` {"sequence": 1} `), CreatedAt: base.Add(time.Second)},
				{ID: 3, RunID: runZ.ID, Type: EventRunCompleted, Payload: []byte(`{"sequence":3}`), CreatedAt: base.Add(3 * time.Second)},
			}},
			{Run: runEmpty},
		},
		MaxEventID: 4,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot mismatch:\ngot  = %#v\nwant = %#v", got, want)
	}
}

func TestSnapshotQueriesStayConstantBeyondSQLiteVariableLimit(t *testing.T) {
	ctx := context.Background()
	store, counter := openQueryCountingStore(t)
	base := time.Date(2026, 7, 13, 19, 0, 0, 0, time.UTC)
	maxEventID := insertSnapshotFixtures(t, store, 0, 1, base)

	counter.queries.Store(0)
	small, err := store.ReadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.queries.Load(); got != 2 {
		t.Fatalf("one-run snapshot query count = %d, want 2", got)
	}
	if len(small.Runs) != 1 || small.MaxEventID != maxEventID {
		t.Fatalf("one-run snapshot = %#v", small)
	}

	maxEventID = insertSnapshotFixtures(t, store, 1, 1104, base)
	counter.queries.Store(0)
	large, err := store.ReadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.queries.Load(); got != 2 {
		t.Fatalf("1105-run snapshot query count = %d, want 2", got)
	}
	if len(large.Runs) != 1105 || large.MaxEventID != maxEventID {
		t.Fatalf("large snapshot runs=%d max_event_id=%d, want 1105/%d", len(large.Runs), large.MaxEventID, maxEventID)
	}
	if large.Runs[0].Events != nil {
		t.Fatalf("run without events = %#v, want nil events", large.Runs[0].Events)
	}
	if got := string(large.Runs[1000].Events[0].Payload); got != `{malformed` {
		t.Fatalf("malformed payload = %q", got)
	}
	counter.queries.Store(0)
	repeated, err := store.ReadSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.queries.Load(); got != 2 {
		t.Fatalf("repeated large snapshot query count = %d, want 2", got)
	}
	if !reflect.DeepEqual(repeated, large) {
		t.Fatal("repeated large snapshot changed deterministic ordering or bytes")
	}

	for _, limit := range []int{1, 1001, 2000} {
		counter.queries.Store(0)
		history, err := store.ListRecentRunsForTaskWithEvents(ctx, "task-large", limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if got := counter.queries.Load(); got != 2 {
			t.Fatalf("limit %d query count = %d, want 2", limit, got)
		}
		want := limit
		if want > 1105 {
			want = 1105
		}
		if len(history) != want {
			t.Fatalf("limit %d returned %d runs, want %d", limit, len(history), want)
		}
	}
}

func TestSnapshotReadersReturnNoPartialResultAfterCancellation(t *testing.T) {
	store, counter := openQueryCountingStore(t)
	insertSnapshotFixtures(t, store, 0, 3, time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC))
	started := make(chan struct{})
	counter.queries.Store(0)
	counter.beforeQuery = func(ctx context.Context, queryNumber int64) error {
		if queryNumber != 2 {
			return nil
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		snapshot Snapshot
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		snapshot, err := store.ReadSnapshot(ctx)
		resultCh <- result{snapshot: snapshot, err: err}
	}()
	<-started
	cancel()
	got := <-resultCh
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("ReadSnapshot() error = %v, want context canceled", got.err)
	}
	if !reflect.DeepEqual(got.snapshot, Snapshot{}) {
		t.Fatalf("canceled snapshot = %#v, want zero result", got.snapshot)
	}

	canceled, cancelImmediately := context.WithCancel(context.Background())
	cancelImmediately()
	if history, err := store.ListRecentRunsForTaskWithEvents(canceled, "task-large", 2); !errors.Is(err, context.Canceled) || history != nil {
		t.Fatalf("pre-canceled task history = %#v, %v", history, err)
	}
}

func BenchmarkReadSnapshot(b *testing.B) {
	for _, size := range []int{10, 2000} {
		b.Run(fmt.Sprintf("runs_%d", size), func(b *testing.B) {
			ctx := context.Background()
			store, err := OpenWithClock(ctx, filepath.Join(b.TempDir(), "ledger.sqlite"), nil)
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			insertBenchmarkSnapshotFixtures(b, store, size, 3)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				snapshot, err := store.ReadSnapshot(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if len(snapshot.Runs) != size {
					b.Fatalf("snapshot runs = %d, want %d", len(snapshot.Runs), size)
				}
			}
		})
	}
}

type queryCountingDriver struct {
	inner       *modernsqlite.Driver
	queries     atomic.Int64
	beforeQuery func(context.Context, int64) error
}

type queryCountingConn struct {
	driver.Conn
	counter *queryCountingDriver
}

var queryCountingDriverSequence atomic.Uint64

func openQueryCountingStore(t *testing.T) (*Store, *queryCountingDriver) {
	t.Helper()
	counter := &queryCountingDriver{inner: &modernsqlite.Driver{}}
	driverName := fmt.Sprintf("sqlite-aud13-query-count-%d", queryCountingDriverSequence.Add(1))
	sql.Register(driverName, counter)
	db, err := sql.Open(driverName, filepath.Join(t.TempDir(), "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := StoreWithClock(context.Background(), db, nil)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, counter
}

func (d *queryCountingDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &queryCountingConn{Conn: conn, counter: d}, nil
}

func (c *queryCountingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryNumber := c.counter.queries.Add(1)
	if c.counter.beforeQuery != nil {
		if err := c.counter.beforeQuery(ctx, queryNumber); err != nil {
			return nil, err
		}
	}
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, query, args)
}

func (c *queryCountingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, query, args)
}

func (c *queryCountingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		if opts.Isolation != driver.IsolationLevel(sql.LevelDefault) || opts.ReadOnly {
			return nil, errors.New("wrapped driver does not support transaction options")
		}
		return c.Conn.Begin()
	}
	return beginner.BeginTx(ctx, opts)
}

func (c *queryCountingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if preparer, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return preparer.PrepareContext(ctx, query)
	}
	return c.Conn.Prepare(query)
}

func (c *queryCountingConn) Ping(ctx context.Context) error {
	if pinger, ok := c.Conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

func insertSnapshotFixtures(t *testing.T, store *Store, start, count int, base time.Time) int64 {
	t.Helper()
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	runStatement, err := tx.Prepare(`
INSERT INTO runs (id, task_id, task, status, started_at, duration_seconds)
VALUES (?, ?, ?, ?, ?, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	defer runStatement.Close()
	eventStatement, err := tx.Prepare(`
INSERT INTO events (run_id, event_type, event_data, created_at)
VALUES (?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer eventStatement.Close()
	var maxEventID int64
	for i := start; i < start+count; i++ {
		runID := fmt.Sprintf("run-%04d", i)
		startedAt := base.Add(time.Duration(i) * time.Second)
		if _, err := runStatement.Exec(runID, "task-large", "large history", StatusRunning, formatTime(startedAt)); err != nil {
			t.Fatal(err)
		}
		if i%3 != 1 {
			continue
		}
		payload := fmt.Sprintf(`{"index":%d}`, i)
		if i == 1000 {
			payload = `{malformed`
		}
		result, err := eventStatement.Exec(runID, EventContextBuilt, payload, formatTime(startedAt.Add(time.Millisecond)))
		if err != nil {
			t.Fatal(err)
		}
		maxEventID, err = result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if start > 0 && maxEventID == 0 {
		if err := store.db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM events`).Scan(&maxEventID); err != nil {
			t.Fatal(err)
		}
	}
	return maxEventID
}

func insertBenchmarkSnapshotFixtures(b *testing.B, store *Store, runCount, eventsPerRun int) {
	b.Helper()
	tx, err := store.db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	runStatement, err := tx.Prepare(`INSERT INTO runs (id, task_id, task, status, started_at, duration_seconds) VALUES (?, ?, ?, ?, ?, 0)`)
	if err != nil {
		b.Fatal(err)
	}
	defer runStatement.Close()
	eventStatement, err := tx.Prepare(`INSERT INTO events (run_id, event_type, event_data, created_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer eventStatement.Close()
	base := time.Date(2026, 7, 13, 21, 0, 0, 0, time.UTC)
	for i := 0; i < runCount; i++ {
		runID := fmt.Sprintf("run-%06d", i)
		createdAt := formatTime(base.Add(time.Duration(i) * time.Second))
		if _, err := runStatement.Exec(runID, "task-benchmark", "benchmark", StatusCompleted, createdAt); err != nil {
			b.Fatal(err)
		}
		for eventIndex := 0; eventIndex < eventsPerRun; eventIndex++ {
			payload := fmt.Sprintf(`{"run":%d,"event":%d}`, i, eventIndex)
			if _, err := eventStatement.Exec(runID, EventContextBuilt, payload, createdAt); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}
