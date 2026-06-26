package taskqueue

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAddTask(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 25, 12, 0, 0, 123, time.UTC)
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()

	task, err := store.AddTask(ctx, TaskSpec{
		ID:      "task-1",
		Task:    "add the task queue",
		Summary: "queue bootstrap",
	})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}

	if got, want := task.ID, "task-1"; got != want {
		t.Fatalf("task id = %q, want %q", got, want)
	}
	if got, want := task.Status, StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	if got, want := task.Task, "add the task queue"; got != want {
		t.Fatalf("task text = %q, want %q", got, want)
	}
	if got, want := task.Summary, "queue bootstrap"; got != want {
		t.Fatalf("task summary = %q, want %q", got, want)
	}
	if !task.CreatedAt.Equal(now) {
		t.Fatalf("created at = %s, want %s", task.CreatedAt, now)
	}
	if !task.UpdatedAt.Equal(now) {
		t.Fatalf("updated at = %s, want %s", task.UpdatedAt, now)
	}
	if task.CompletedAt != nil {
		t.Fatalf("completed at = %s, want nil", task.CompletedAt)
	}
	if task.BlockedAt != nil {
		t.Fatalf("blocked at = %s, want nil", task.BlockedAt)
	}

	got, ok, err := store.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !ok {
		t.Fatal("get task returned not found")
	}
	if got.ID != task.ID || got.Task != task.Task || got.Status != task.Status || got.Summary != task.Summary {
		t.Fatalf("stored task = %+v, want %+v", got, task)
	}
}

func TestListTasksDeterministically(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	store := openTestStore(t, func() time.Time { return base })
	defer store.Close()

	for _, spec := range []TaskSpec{
		{ID: "task-b", Task: "same timestamp b", CreatedAt: base},
		{ID: "task-a", Task: "same timestamp a", CreatedAt: base},
		{ID: "task-c", Task: "later timestamp", CreatedAt: base.Add(time.Minute)},
	} {
		if _, err := store.AddTask(ctx, spec); err != nil {
			t.Fatalf("add %s: %v", spec.ID, err)
		}
	}

	tasks, err := store.ListTasks(ctx)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if got, want := taskIDs(tasks), []string{"task-a", "task-b", "task-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task order = %v, want %v", got, want)
	}
}

func TestSelectNextReturnsFirstPendingUnblockedTask(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	now := base
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()

	for _, spec := range []TaskSpec{
		{ID: "task-blocked", Task: "blocked task", CreatedAt: base},
		{ID: "task-completed", Task: "completed task", CreatedAt: base.Add(time.Minute)},
		{ID: "task-b", Task: "same timestamp b", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "task-a", Task: "same timestamp a", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "task-later", Task: "later pending task", CreatedAt: base.Add(3 * time.Minute)},
	} {
		if _, err := store.AddTask(ctx, spec); err != nil {
			t.Fatalf("add %s: %v", spec.ID, err)
		}
	}
	now = base.Add(10 * time.Minute)
	if _, ok, err := store.BlockTask(ctx, "task-blocked", "needs product decision"); err != nil || !ok {
		t.Fatalf("block task: ok=%v err=%v", ok, err)
	}
	now = base.Add(11 * time.Minute)
	if _, ok, err := store.CompleteTask(ctx, "task-completed", "already done"); err != nil || !ok {
		t.Fatalf("complete task: ok=%v err=%v", ok, err)
	}

	task, ok, err := store.SelectNext(ctx)
	if err != nil {
		t.Fatalf("select next: %v", err)
	}
	if !ok {
		t.Fatal("select next returned not found")
	}
	if got, want := task.ID, "task-a"; got != want {
		t.Fatalf("selected task = %q, want %q", got, want)
	}
}

func TestCompleteTask(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC)
	now := base
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()

	if _, err := store.AddTask(ctx, TaskSpec{ID: "task-complete", Task: "finish this", CreatedAt: base}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	completedAt := base.Add(time.Hour)
	now = completedAt
	task, ok, err := store.CompleteTask(ctx, "task-complete", "finished cleanly")
	if err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if !ok {
		t.Fatal("complete task returned not found")
	}
	if got, want := task.Status, StatusCompleted; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := task.Summary, "finished cleanly"; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if got := task.Blocker; got != "" {
		t.Fatalf("blocker = %q, want empty", got)
	}
	if task.CompletedAt == nil || !task.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed at = %v, want %s", task.CompletedAt, completedAt)
	}
	if !task.UpdatedAt.Equal(completedAt) {
		t.Fatalf("updated at = %s, want %s", task.UpdatedAt, completedAt)
	}
	if task.BlockedAt != nil {
		t.Fatalf("blocked at = %s, want nil", task.BlockedAt)
	}

	if selected, ok, err := store.SelectNext(ctx); err != nil || ok {
		t.Fatalf("select next after complete = %+v ok=%v err=%v, want no task", selected, ok, err)
	}
}

func TestBlockTask(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	now := base
	store := openTestStore(t, func() time.Time { return now })
	defer store.Close()

	if _, err := store.AddTask(ctx, TaskSpec{ID: "task-block", Task: "blocked work", CreatedAt: base}); err != nil {
		t.Fatalf("add task: %v", err)
	}

	blockedAt := base.Add(30 * time.Minute)
	now = blockedAt
	task, ok, err := store.BlockTask(ctx, "task-block", "missing acceptance criteria")
	if err != nil {
		t.Fatalf("block task: %v", err)
	}
	if !ok {
		t.Fatal("block task returned not found")
	}
	if got, want := task.Status, StatusBlocked; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := task.Blocker, "missing acceptance criteria"; got != want {
		t.Fatalf("blocker = %q, want %q", got, want)
	}
	if task.BlockedAt == nil || !task.BlockedAt.Equal(blockedAt) {
		t.Fatalf("blocked at = %v, want %s", task.BlockedAt, blockedAt)
	}
	if !task.UpdatedAt.Equal(blockedAt) {
		t.Fatalf("updated at = %s, want %s", task.UpdatedAt, blockedAt)
	}
	if task.CompletedAt != nil {
		t.Fatalf("completed at = %s, want nil", task.CompletedAt)
	}

	if selected, ok, err := store.SelectNext(ctx); err != nil || ok {
		t.Fatalf("select next after block = %+v ok=%v err=%v, want no task", selected, ok, err)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "revolvr.sqlite")
	base := time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC)
	now := base

	store, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := store.AddTask(ctx, TaskSpec{ID: "task-pending", Task: "still pending", CreatedAt: base}); err != nil {
		t.Fatalf("add pending task: %v", err)
	}
	if _, err := store.AddTask(ctx, TaskSpec{ID: "task-blocked", Task: "blocked task", CreatedAt: base.Add(time.Minute)}); err != nil {
		t.Fatalf("add blocked task: %v", err)
	}
	now = base.Add(2 * time.Minute)
	if _, ok, err := store.BlockTask(ctx, "task-blocked", "waiting on dependency"); err != nil || !ok {
		t.Fatalf("block task: ok=%v err=%v", ok, err)
	}
	if _, err := store.AddTask(ctx, TaskSpec{ID: "task-completed", Task: "completed task", CreatedAt: base.Add(3 * time.Minute)}); err != nil {
		t.Fatalf("add completed task: %v", err)
	}
	now = base.Add(4 * time.Minute)
	if _, ok, err := store.CompleteTask(ctx, "task-completed", "done"); err != nil || !ok {
		t.Fatalf("complete task: ok=%v err=%v", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()

	tasks, err := reopened.ListTasks(ctx)
	if err != nil {
		t.Fatalf("list tasks after reopen: %v", err)
	}
	if got, want := taskIDs(tasks), []string{"task-pending", "task-blocked", "task-completed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task order after reopen = %v, want %v", got, want)
	}

	byID := tasksByID(tasks)
	if got, want := byID["task-pending"].Status, StatusPending; got != want {
		t.Fatalf("pending status = %q, want %q", got, want)
	}
	blocked := byID["task-blocked"]
	if blocked.Status != StatusBlocked || blocked.Blocker != "waiting on dependency" || blocked.BlockedAt == nil {
		t.Fatalf("blocked task after reopen = %+v", blocked)
	}
	completed := byID["task-completed"]
	if completed.Status != StatusCompleted || completed.Summary != "done" || completed.CompletedAt == nil {
		t.Fatalf("completed task after reopen = %+v", completed)
	}

	selected, ok, err := reopened.SelectNext(ctx)
	if err != nil {
		t.Fatalf("select next after reopen: %v", err)
	}
	if !ok {
		t.Fatal("select next after reopen returned not found")
	}
	if got, want := selected.ID, "task-pending"; got != want {
		t.Fatalf("selected task after reopen = %q, want %q", got, want)
	}
}

func openTestStore(t *testing.T, clk func() time.Time) *Store {
	t.Helper()
	store, err := OpenWithClock(context.Background(), filepath.Join(t.TempDir(), "tasks.sqlite"), clk)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	return store
}

func taskIDs(tasks []Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func tasksByID(tasks []Task) map[string]Task {
	byID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	return byID
}
