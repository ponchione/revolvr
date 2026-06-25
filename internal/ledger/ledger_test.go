package ledger

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCreateRun(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 123, time.UTC)
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()

	run, err := store.CreateRun(ctx, RunSpec{
		TaskID: "task-1",
		Task:   "add the run ledger",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	if run.ID == "" {
		t.Fatal("run id is empty")
	}
	if got, want := run.Status, StatusRunning; got != want {
		t.Fatalf("run status = %q, want %q", got, want)
	}
	if !run.StartedAt.Equal(now) {
		t.Fatalf("started at = %s, want %s", run.StartedAt, now)
	}

	got, ok, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if !ok {
		t.Fatal("get run returned not found")
	}
	if got.ID != run.ID || got.TaskID != run.TaskID || got.Task != run.Task || got.Status != run.Status {
		t.Fatalf("stored run = %+v, want %+v", got, run)
	}
}

func TestAppendEventRecordsJSONPayload(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC)
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()
	run := mustCreateRun(t, store, "run-events", now)

	event, err := store.AppendEvent(ctx, run.ID, EventPromptBuilt, map[string]any{
		"prompt_path": "prompts/task-1.md",
		"tokens":      42,
	})
	if err != nil {
		t.Fatalf("append event: %v", err)
	}

	if event.ID == 0 {
		t.Fatal("event id is zero")
	}
	if got, want := event.RunID, run.ID; got != want {
		t.Fatalf("event run id = %q, want %q", got, want)
	}
	if got, want := event.Type, EventPromptBuilt; got != want {
		t.Fatalf("event type = %q, want %q", got, want)
	}
	if !event.CreatedAt.Equal(now) {
		t.Fatalf("created at = %s, want %s", event.CreatedAt, now)
	}
	assertJSONEqual(t, event.Payload, `{"prompt_path":"prompts/task-1.md","tokens":42}`)
}

func TestListRecentRuns(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store := openTestStore(t, func() time.Time { return base })
	defer store.Close()

	for _, spec := range []RunSpec{
		{ID: "run-old", TaskID: "task-old", Task: "old task", StartedAt: base},
		{ID: "run-middle", TaskID: "task-middle", Task: "middle task", StartedAt: base.Add(time.Hour)},
		{ID: "run-new", TaskID: "task-new", Task: "new task", StartedAt: base.Add(2 * time.Hour)},
	} {
		if _, err := store.CreateRun(ctx, spec); err != nil {
			t.Fatalf("create %s: %v", spec.ID, err)
		}
	}

	runs, err := store.ListRecentRuns(ctx, 2)
	if err != nil {
		t.Fatalf("list recent runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	if got, want := []string{runs[0].ID, runs[1].ID}, []string{"run-new", "run-middle"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent run order = %v, want %v", got, want)
	}
}

func TestGetRunWithEventsReturnsHistory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()
	run := mustCreateRun(t, store, "run-history", now)

	if _, err := store.AppendEvent(ctx, run.ID, EventRunStarted, json.RawMessage(`{"pid":123}`)); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if _, err := store.AppendEvent(ctx, run.ID, EventCodexCompleted, map[string]any{"exit_code": 0}); err != nil {
		t.Fatalf("append second event: %v", err)
	}

	history, ok, err := store.GetRunWithEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with events: %v", err)
	}
	if !ok {
		t.Fatal("get run with events returned not found")
	}
	if got, want := history.Run.ID, run.ID; got != want {
		t.Fatalf("history run id = %q, want %q", got, want)
	}
	if len(history.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(history.Events))
	}
	if got, want := []EventType{history.Events[0].Type, history.Events[1].Type}, []EventType{EventRunStarted, EventCodexCompleted}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event types = %v, want %v", got, want)
	}
	assertJSONEqual(t, history.Events[0].Payload, `{"pid":123}`)
	assertJSONEqual(t, history.Events[1].Payload, `{"exit_code":0}`)
}

func TestPersistenceAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	startedAt := time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(90 * time.Second)
	exitCode := 0

	store, err := OpenWithClock(ctx, path, func() time.Time { return startedAt })
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	run, err := store.CreateRun(ctx, RunSpec{
		ID:                 "run-persisted",
		TaskID:             "task-persisted",
		Task:               "persist this run",
		Status:             StatusCompleted,
		Summary:            "ledger survived reopen",
		StartedAt:          startedAt,
		CompletedAt:        &completedAt,
		DurationSeconds:    90,
		CodexExitCode:      &exitCode,
		VerificationStatus: "passed",
		CommitSHA:          "abc123",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := store.AppendEvent(ctx, run.ID, EventRunCompleted, map[string]any{"summary": run.Summary}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()

	history, ok, err := reopened.GetRunWithEvents(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with events after reopen: %v", err)
	}
	if !ok {
		t.Fatal("persisted run was not found after reopen")
	}
	if got := history.Run; got.Status != StatusCompleted || got.Summary != "ledger survived reopen" || got.VerificationStatus != "passed" || got.CommitSHA != "abc123" {
		t.Fatalf("persisted run fields = %+v", got)
	}
	if history.Run.CompletedAt == nil || !history.Run.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed at = %v, want %s", history.Run.CompletedAt, completedAt)
	}
	if history.Run.CodexExitCode == nil || *history.Run.CodexExitCode != exitCode {
		t.Fatalf("codex exit code = %v, want %d", history.Run.CodexExitCode, exitCode)
	}
	if len(history.Events) != 1 {
		t.Fatalf("got %d persisted events, want 1", len(history.Events))
	}
	assertJSONEqual(t, history.Events[0].Payload, `{"summary":"ledger survived reopen"}`)
}

func openTestStore(t *testing.T, clk func() time.Time) *Store {
	t.Helper()
	store, err := OpenWithClock(context.Background(), filepath.Join(t.TempDir(), "ledger.sqlite"), clk)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	return store
}

func mustCreateRun(t *testing.T, store *Store, runID string, startedAt time.Time) Run {
	t.Helper()
	run, err := store.CreateRun(context.Background(), RunSpec{
		ID:        runID,
		TaskID:    "task-" + runID,
		Task:      "task for " + runID,
		StartedAt: startedAt,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func assertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal got JSON %q: %v", string(got), err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("unmarshal want JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON payload = %#v, want %#v", gotValue, wantValue)
	}
}
