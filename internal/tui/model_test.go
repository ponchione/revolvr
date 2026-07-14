package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskscheduler"
)

func TestStatusModelRendersUninitializedSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{})

	lines := normalizedViewLines(model.View())
	want := []string{
		"Revolvr",
		"Views: [Dashboard] | Tasks | Runs | Run Detail | Preflight | Help",
		"State: not initialized",
		"",
		"Dashboard",
		"State: not initialized",
		"Tasks: unavailable",
		"Runnable: unavailable",
		"Runs: unavailable",
		"",
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help",
		"      a Add Task | R Run Once | n Passes 3 | L Run Loop | r Refresh | q Quit",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("view lines = %#v, want %#v", lines, want)
	}
}

func TestStatusModelRendersStaticStatusSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks: []taskmodel.Task{
			{ID: "task-pending", Status: taskmodel.StatusPending, NextRunnable: true},
			{ID: "task-blocked", Status: taskmodel.StatusBlocked},
			{ID: "task-completed", Status: taskmodel.StatusCompleted},
		},
		RecentRuns: []ledger.Run{
			{
				ID:                 "run-new",
				Status:             ledger.StatusFailed,
				Summary:            "verification failed",
				VerificationStatus: "failed",
			},
			{
				ID:        "run-old",
				Status:    ledger.StatusCompleted,
				Summary:   "committed change",
				CommitSHA: "abc123",
			},
		},
	})
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	lines := normalizedViewLines(updated.View())
	want := []string{
		"Revolvr",
		"Views: [Dashboard] | Tasks | Runs | Run Detail | Preflight | Help",
		"State: initialized",
		"",
		"Dashboard",
		"State: initialized",
		"",
		"Tasks",
		"Total: 3",
		"Pending: 1",
		"Blocked: 1",
		"Completed: 1",
		"Runnable: ready to run",
		"Next task: task-pending",
		"",
		"Latest Run",
		"ID: run-new",
		"Status: failed",
		"Summary: verification failed",
		"Verification: failed",
		"Commit: none",
		"",
		"Recent Runs",
		"ID  STATUS  VERIFICATION  COMMIT  SUMMARY",
		"> run-new  failed  failed  none  verification failed",
		"  run-old  completed  none  abc123  committed change",
		"",
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help | a Add Task | R Run Once",
		"      n Passes 3 | L Run Loop | r Refresh | q Quit",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("view lines = %#v, want %#v", lines, want)
	}
}

func TestStatusModelTasksViewRendersEmptyTaskState(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	tasksView := openTasksView(t, model)

	lines := normalizedViewLines(tasksView.View())
	requireLines(t, lines,
		"Views: Dashboard | [Tasks] | Runs | Run Detail | Preflight | Help",
		"Tasks",
		"Total: 0",
		"Pending: 0",
		"Blocked: 0",
		"Completed: 0",
		"Runnable: nothing runnable",
		"Next task: none",
		"Task List",
		"No task files found.",
		"Task Detail",
		"No task selected.",
		"Keys: j/k Select | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help | a Add Task | R Run Once",
		"      n Passes 3 | L Run Loop | r Refresh | q Quit",
	)
}

func TestStatusModelRendersNextRunnableTaskStates(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []taskmodel.Task
		dashboardWant []string
		tasksWant     []string
		tasksNotWant  []string
	}{
		{
			name: "pending",
			tasks: []taskmodel.Task{
				{ID: "task-blocked", Status: taskmodel.StatusBlocked, Summary: "waiting on access"},
				{ID: "task-ready", Status: taskmodel.StatusPending, Summary: "ship change", NextRunnable: true},
				{ID: "task-later", Status: taskmodel.StatusPending, Task: "later task"},
			},
			dashboardWant: []string{
				"Total: 3",
				"Pending: 2",
				"Blocked: 1",
				"Completed: 0",
				"Runnable: ready to run",
				"Next task: task-ready - ship change",
			},
			tasksWant: []string{
				"Runnable: ready to run",
				"Next task: task-ready - ship change",
				"> - task-blocked  ! blocked  waiting on access",
				"  next task-ready  pending  ship change",
				"  - task-later  pending  later task",
			},
			tasksNotWant: []string{
				"Runnable: nothing runnable",
				"Next task: none",
			},
		},
		{
			name: "priority marker overrides display order",
			tasks: []taskmodel.Task{
				{ID: "task-filename-first", Status: taskmodel.StatusPending, Summary: "shown first"},
				{ID: "task-priority-first", Status: taskmodel.StatusPending, Summary: "runs first", NextRunnable: true},
			},
			dashboardWant: []string{
				"Runnable: ready to run",
				"Next task: task-priority-first - runs first",
			},
			tasksWant: []string{
				"> - task-filename-first  pending  shown first",
				"  next task-priority-first  pending  runs first",
			},
			tasksNotWant: []string{
				"Next task: task-filename-first - shown first",
			},
		},
		{
			name: "blocked-only",
			tasks: []taskmodel.Task{
				{ID: "task-blocked", Status: taskmodel.StatusBlocked, Summary: "waiting on access"},
			},
			dashboardWant: []string{
				"Total: 1",
				"Pending: 0",
				"Blocked: 1",
				"Completed: 0",
				"Runnable: nothing runnable",
				"Next task: none",
			},
			tasksWant: []string{
				"Runnable: nothing runnable",
				"Next task: none",
				"> - task-blocked  ! blocked  waiting on access",
			},
			tasksNotWant: []string{
				"Runnable: ready to run",
				"> next task-blocked  ! blocked  waiting on access",
			},
		},
		{
			name: "completed-only",
			tasks: []taskmodel.Task{
				{ID: "task-completed", Status: taskmodel.StatusCompleted, Summary: "done"},
			},
			dashboardWant: []string{
				"Total: 1",
				"Pending: 0",
				"Blocked: 0",
				"Completed: 1",
				"Runnable: nothing runnable",
				"Next task: none",
			},
			tasksWant: []string{
				"Runnable: nothing runnable",
				"Next task: none",
				"> - task-completed  completed  done",
			},
			tasksNotWant: []string{
				"Runnable: ready to run",
				"> next task-completed  completed  done",
			},
		},
		{
			name:  "empty",
			tasks: nil,
			dashboardWant: []string{
				"Total: 0",
				"Pending: 0",
				"Blocked: 0",
				"Completed: 0",
				"Runnable: nothing runnable",
				"Next task: none",
			},
			tasksWant: []string{
				"Runnable: nothing runnable",
				"Next task: none",
				"No task files found.",
				"No task selected.",
			},
			tasksNotWant: []string{
				"Runnable: ready to run",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewStatusModel(app.StatusResult{
				Initialized: true,
				Tasks:       tt.tasks,
			})

			dashboardLines := normalizedViewLines(model.View())
			requireLines(t, dashboardLines, tt.dashboardWant...)

			tasksView := openTasksView(t, model)
			tasksLines := normalizedViewLines(tasksView.View())
			requireLines(t, tasksLines, tt.tasksWant...)
			for _, notWant := range tt.tasksNotWant {
				requireNoLine(t, tasksLines, notWant)
			}
		})
	}
}

func TestStatusModelDoesNotFallbackToPendingWaitingTask(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks: []taskmodel.Task{{
			ID:                   "task-dependent",
			Status:               taskmodel.StatusPending,
			Readiness:            taskscheduler.ReasonWaitingDependency,
			ReadinessReason:      string(taskscheduler.ReasonWaitingDependency),
			WaitingDependencyIDs: []string{"task-prerequisite"},
			DependsOn:            []string{"task-prerequisite"},
		}},
	})

	dashboard := normalizedViewLines(model.View())
	requireLines(t, dashboard, "Runnable: nothing runnable", "Next task: none")
	requireNoLine(t, dashboard, "Next task: task-dependent")

	tasksView := openTasksView(t, model)
	lines := normalizedViewLines(tasksView.View())
	requireLines(t, lines,
		"> - task-dependent  pending",
		"Readiness: waiting_dependency",
		"Waiting on: task-prerequisite",
		"Depends on: task-prerequisite",
	)
	requireNoLine(t, lines, "> next task-dependent  pending")
}

func TestStatusModelRendersSharedInvalidGraphDiagnostics(t *testing.T) {
	diagnostic := taskscheduler.Diagnostic{
		Code:   taskscheduler.DiagnosticMissingDependency,
		Detail: `task-invalid -> task-missing`,
	}
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks: []taskmodel.Task{{
			ID:                    "task-invalid",
			Status:                taskmodel.StatusPending,
			Readiness:             taskscheduler.ReasonInvalidGraph,
			ReadinessReason:       string(taskscheduler.ReasonInvalidGraph),
			SchedulingDiagnostics: []taskscheduler.Diagnostic{diagnostic},
		}},
		Schedule: taskscheduler.Result{InvalidGraph: []taskscheduler.Diagnostic{diagnostic}},
	})

	requireLines(t, normalizedViewLines(model.View()),
		"Runnable: nothing runnable",
		"Next task: none",
		`Scheduling diagnostic: missing_dependency: task-invalid -> task-missing`,
	)
	tasksView := openTasksView(t, model)
	requireLines(t, normalizedViewLines(tasksView.View()),
		"Readiness: invalid_graph",
		`Scheduling diagnostic: missing_dependency: task-invalid -> task-missing`,
	)
}

func TestStatusModelTasksViewRendersPopulatedTaskList(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	})
	tasksView := openTasksView(t, model)

	lines := normalizedViewLines(tasksView.View())
	requireLines(t, lines,
		"Task List",
		"> next task-pending  pending  write focused tests",
		"  - task-blocked  ! blocked  blocked task",
		"  - task-completed  completed  finished task",
	)
}

func TestStatusModelRendersTaskWorkflowState(t *testing.T) {
	tasks := []taskmodel.Task{
		{
			ID:           "task-audit",
			Status:       taskmodel.StatusPending,
			Summary:      "audit task",
			Workflow:     "mixed-pass-v1",
			Phase:        "audit",
			RunProfile:   "auditor",
			NextState:    "document",
			NextRunnable: true,
		},
		{
			ID:         "task-simplify",
			Status:     taskmodel.StatusCompleted,
			Summary:    "simplify task",
			Workflow:   "mixed-pass-v1",
			Phase:      "simplify",
			RunProfile: "simplifier",
			NextState:  taskmodel.StatusCompleted,
		},
	}
	model := NewStatusModel(app.StatusResult{Initialized: true, Tasks: tasks})

	requireLines(t, normalizedViewLines(model.View()),
		"Next task: task-audit - audit task",
		"Workflow: mixed-pass-v1  Phase: audit  Profile: auditor  Next: document",
	)

	tasksView := openTasksView(t, model)
	requireLines(t, normalizedViewLines(tasksView.View()),
		"> next task-audit  pending  phase=audit  profile=auditor  next=document  audit task",
		"  - task-simplify  completed  phase=simplify  profile=simplifier  next=completed  simplify task",
		"Workflow: mixed-pass-v1",
		"Phase: audit",
		"Profile: auditor",
		"Next: document",
	)

	completedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(completedView.View()),
		"Workflow: mixed-pass-v1",
		"Phase: simplify",
		"Profile: simplifier",
		"Next: completed",
	)
}

func TestStatusModelTasksViewRendersPendingTaskDetails(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	})
	tasksView := openTasksView(t, model)

	lines := normalizedViewLines(tasksView.View())
	requireLines(t, lines,
		"Task Detail",
		"ID: task-pending",
		"Status: pending",
		"Summary: write focused tests",
		"Task: Add focused task view tests",
		"Blocker: none",
		"Created: 2026-07-08T10:00:00Z",
		"Updated: 2026-07-08T10:00:00Z",
	)
	requireNoLine(t, lines, "Blocked: 2026-07-08T10:02:00Z")
	requireNoLine(t, lines, "Completed: 2026-07-08T10:04:00Z")
}

func TestStatusModelTasksViewRendersBlockedTaskDetails(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	})
	tasksView := openTasksView(t, model)

	blockedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}

	lines := normalizedViewLines(blockedView.View())
	requireLines(t, lines,
		"> - task-blocked  ! blocked  blocked task",
		"ID: task-blocked",
		"Status: blocked",
		"Summary: none",
		"Task: blocked task",
		"Blocker: waiting on access",
		"Created: 2026-07-08T10:01:00Z",
		"Updated: 2026-07-08T10:02:00Z",
		"Blocked: 2026-07-08T10:02:00Z",
	)
}

func TestStatusModelTasksViewRendersCompletedTaskDetails(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	})
	tasksView := openTasksView(t, model)

	completedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("first move selection cmd = %v, want nil", cmd)
	}
	completedView, cmd = updateStatusModel(t, completedView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("second move selection cmd = %v, want nil", cmd)
	}

	lines := normalizedViewLines(completedView.View())
	requireLines(t, lines,
		"> - task-completed  completed  finished task",
		"ID: task-completed",
		"Status: completed",
		"Summary: finished task",
		"Task: completed task",
		"Blocker: none",
		"Created: 2026-07-08T10:03:00Z",
		"Updated: 2026-07-08T10:04:00Z",
		"Completed: 2026-07-08T10:04:00Z",
	)
}

func TestStatusModelTasksViewRetriesBlockedTaskRefreshesAndSelects(t *testing.T) {
	tasks := sampleTasks()
	retried := tasks[1]
	retried.Status = taskmodel.StatusPending
	retried.Blocker = ""
	retried.BlockedAt = nil
	retried.UpdatedAt = retried.UpdatedAt.Add(time.Minute)

	var calls []string
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		Tasks:       tasks,
	}, StatusActions{
		RetryTask: func(taskID string) (taskmodel.Task, error) {
			calls = append(calls, "retry:"+taskID)
			if taskID != "task-blocked" {
				t.Fatalf("retry task id = %q, want task-blocked", taskID)
			}
			return retried, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{
				Initialized: true,
				Tasks:       []taskmodel.Task{tasks[0], retried, tasks[2]},
			}, nil
		},
	})
	tasksView := openTasksView(t, model)
	blockedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(blockedView.View()),
		"Keys: j/k Select | u Retry | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help | a Add Task | R Run Once",
	)

	afterKey, cmd := updateStatusModel(t, blockedView, keyRunes("u"))
	if cmd == nil {
		t.Fatal("retry key returned nil cmd")
	}
	if len(calls) != 0 {
		t.Fatalf("callbacks ran before command execution: %#v", calls)
	}

	afterRetry, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("retry message cmd = %v, want nil", cmd)
	}
	if !reflect.DeepEqual(calls, []string{"retry:task-blocked", "refresh"}) {
		t.Fatalf("callback order = %#v, want retry then refresh", calls)
	}
	if got, want := afterRetry.selectedTaskID(), "task-blocked"; got != want {
		t.Fatalf("selected task = %q, want %q", got, want)
	}

	lines := normalizedViewLines(afterRetry.View())
	requireLines(t, lines,
		"Notice: Retried task task-blocked.",
		"> - task-blocked  pending  blocked task",
		"ID: task-blocked",
		"Status: pending",
		"Blocker: none",
	)
	requireNoLine(t, lines, "Blocked: 2026-07-08T10:02:00Z")
}

func TestStatusModelTasksViewRejectsNonBlockedRetryWithoutMutation(t *testing.T) {
	calls := 0
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	}, StatusActions{
		RetryTask: func(string) (taskmodel.Task, error) {
			calls++
			return taskmodel.Task{}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls++
			return app.StatusResult{}, nil
		},
	})
	tasksView := openTasksView(t, model)

	afterPending, cmd := updateStatusModel(t, tasksView, keyRunes("u"))
	if cmd != nil {
		t.Fatalf("pending retry cmd = %v, want nil", cmd)
	}
	if calls != 0 {
		t.Fatalf("callback calls after pending retry = %d, want 0", calls)
	}
	requireLines(t, normalizedViewLines(afterPending.View()),
		"Notice: Retry unavailable: selected task task-pending is not blocked (status: pending).",
		"> next task-pending  pending  write focused tests",
	)

	completedView, cmd := updateStatusModel(t, afterPending, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("first move selection cmd = %v, want nil", cmd)
	}
	completedView, cmd = updateStatusModel(t, completedView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("second move selection cmd = %v, want nil", cmd)
	}
	afterCompleted, cmd := updateStatusModel(t, completedView, keyRunes("u"))
	if cmd != nil {
		t.Fatalf("completed retry cmd = %v, want nil", cmd)
	}
	if calls != 0 {
		t.Fatalf("callback calls after completed retry = %d, want 0", calls)
	}
	requireLines(t, normalizedViewLines(afterCompleted.View()),
		"Notice: Retry unavailable: selected task task-completed is not blocked (status: completed).",
		"> - task-completed  completed  finished task",
	)
}

func TestStatusModelTasksViewRetryReportsMissingCallbacks(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	})
	tasksView := openTasksView(t, model)
	blockedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}

	afterMissingRetry, cmd := updateStatusModel(t, blockedView, keyRunes("u"))
	if cmd != nil {
		t.Fatalf("missing retry callback cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(afterMissingRetry.View()),
		"Notice: Retry is unavailable.",
		"> - task-blocked  ! blocked  blocked task",
	)

	called := false
	afterMissingRetry.actions.RetryTask = func(string) (taskmodel.Task, error) {
		called = true
		return taskmodel.Task{}, nil
	}
	afterMissingRefresh, cmd := updateStatusModel(t, afterMissingRetry, keyRunes("u"))
	if cmd != nil {
		t.Fatalf("missing refresh callback cmd = %v, want nil", cmd)
	}
	if called {
		t.Fatal("retry callback ran while refresh callback was missing")
	}
	requireLines(t, normalizedViewLines(afterMissingRefresh.View()),
		"Notice: Retry is unavailable: refresh callback is missing.",
	)
}

func TestStatusModelTasksViewRetryCallbackErrorShowsInlineMessage(t *testing.T) {
	calls := 0
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		Tasks:       sampleTasks(),
	}, StatusActions{
		RetryTask: func(taskID string) (taskmodel.Task, error) {
			calls++
			if taskID != "task-blocked" {
				t.Fatalf("retry task id = %q, want task-blocked", taskID)
			}
			return taskmodel.Task{}, errors.New("storage locked")
		},
		RefreshStatus: func() (app.StatusResult, error) {
			t.Fatal("refresh callback ran after retry error")
			return app.StatusResult{}, nil
		},
	})
	tasksView := openTasksView(t, model)
	blockedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}

	afterKey, cmd := updateStatusModel(t, blockedView, keyRunes("u"))
	if cmd == nil {
		t.Fatal("retry key returned nil cmd")
	}
	afterRetry, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("retry message cmd = %v, want nil", cmd)
	}
	if calls != 1 {
		t.Fatalf("retry calls = %d, want 1", calls)
	}
	requireLines(t, normalizedViewLines(afterRetry.View()),
		"Notice: Retry failed: storage locked",
		"> - task-blocked  ! blocked  blocked task",
		"Status: blocked",
	)
}

func TestStatusModelTasksViewRetryRefreshFailureShowsInlineMessage(t *testing.T) {
	tasks := sampleTasks()
	retried := tasks[1]
	retried.Status = taskmodel.StatusPending
	retried.Blocker = ""
	retried.BlockedAt = nil

	var calls []string
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		Tasks:       tasks,
	}, StatusActions{
		RetryTask: func(taskID string) (taskmodel.Task, error) {
			calls = append(calls, "retry:"+taskID)
			return retried, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{}, errors.New("status database offline")
		},
	})
	tasksView := openTasksView(t, model)
	blockedView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}

	afterKey, cmd := updateStatusModel(t, blockedView, keyRunes("u"))
	if cmd == nil {
		t.Fatal("retry key returned nil cmd")
	}
	afterRetry, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("retry message cmd = %v, want nil", cmd)
	}
	if !reflect.DeepEqual(calls, []string{"retry:task-blocked", "refresh"}) {
		t.Fatalf("callback order = %#v, want retry then refresh", calls)
	}
	requireLines(t, normalizedViewLines(afterRetry.View()),
		"Notice: Retry refresh failed: status database offline",
		"> - task-blocked  ! blocked  blocked task",
		"Status: blocked",
	)
}

func TestStatusModelTaskEntryRejectsEmptyTaskTextInline(t *testing.T) {
	addCalled := false
	refreshCalled := false
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		AddTask: func(app.AddTaskInput) (taskmodel.Task, error) {
			addCalled = true
			return taskmodel.Task{}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			refreshCalled = true
			return app.StatusResult{}, nil
		},
	})
	tasksView := openTasksView(t, model)

	entryView, cmd := updateStatusModel(t, tasksView, keyRunes("a"))
	if cmd != nil {
		t.Fatalf("add key cmd = %v, want nil", cmd)
	}
	if entryView.view != viewTaskEntry {
		t.Fatalf("view = %v, want task entry", entryView.view)
	}

	afterSubmit, cmd := updateStatusModel(t, entryView, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("empty submit cmd = %v, want nil", cmd)
	}
	if addCalled {
		t.Fatal("add callback ran for empty task text")
	}
	if refreshCalled {
		t.Fatal("refresh callback ran for empty task text")
	}

	lines := normalizedViewLines(afterSubmit.View())
	requireLines(t, lines,
		"View: Add Task",
		"Add Task",
		"> Task:",
		"  Summary:",
		"Error: Task text is required.",
		"Keys: tab Field | enter Submit | esc Cancel | ctrl+c Quit",
	)
}

func TestStatusModelTaskEntryCancelReturnsToPreviousViewWithoutWrite(t *testing.T) {
	addCalled := false
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{{
			ID:      "run-one",
			Status:  ledger.StatusCompleted,
			Summary: "done",
		}},
	}, StatusActions{
		AddTask: func(app.AddTaskInput) (taskmodel.Task, error) {
			addCalled = true
			return taskmodel.Task{}, nil
		},
	})
	resized, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}
	runsView, cmd := updateStatusModel(t, resized, keyRunes("3"))
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}

	entryView, cmd := updateStatusModel(t, runsView, keyRunes("a"))
	if cmd != nil {
		t.Fatalf("add key cmd = %v, want nil", cmd)
	}
	entryView, cmd = typeIntoStatusModel(t, entryView, "do not persist")
	if cmd != nil {
		t.Fatalf("typing cmd = %v, want nil", cmd)
	}

	cancelled, cmd := updateStatusModel(t, entryView, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("cancel cmd = %v, want nil", cmd)
	}
	if addCalled {
		t.Fatal("add callback ran after cancel")
	}
	if cancelled.view != viewRuns {
		t.Fatalf("view = %v, want runs", cancelled.view)
	}
	if cancelled.taskEntry.taskText != "" || cancelled.taskEntry.summary != "" {
		t.Fatalf("task entry state = %+v, want cleared", cancelled.taskEntry)
	}
	requireLines(t, normalizedViewLines(cancelled.View()),
		"Views: Dashboard | Tasks | [Runs] | Run Detail | Preflight | Help",
		"> run-one  completed  none  none  done",
	)
}

func TestStatusModelTaskEntrySubmitAddsRefreshesAndSelectsNewTask(t *testing.T) {
	base := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	existing := taskmodel.Task{
		ID:        "task-old",
		Status:    taskmodel.StatusPending,
		Task:      "existing task",
		CreatedAt: base,
		UpdatedAt: base,
	}
	added := taskmodel.Task{
		ID:        "task-new",
		Status:    taskmodel.StatusPending,
		Task:      "---\nid: task-new\nstatus: pending\n---\n# TUI add\n\nImplement add flow\n",
		Summary:   "TUI add",
		CreatedAt: base.Add(time.Minute),
		UpdatedAt: base.Add(time.Minute),
	}
	var input app.AddTaskInput
	var calls []string
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		Tasks:       []taskmodel.Task{existing},
	}, StatusActions{
		AddTask: func(got app.AddTaskInput) (taskmodel.Task, error) {
			calls = append(calls, "add")
			input = got
			return added, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{
				Initialized: true,
				Tasks:       []taskmodel.Task{existing, added},
			}, nil
		},
	})
	tasksView := openTasksView(t, model)
	entryView, cmd := updateStatusModel(t, tasksView, keyRunes("a"))
	if cmd != nil {
		t.Fatalf("add key cmd = %v, want nil", cmd)
	}
	entryView, cmd = typeIntoStatusModel(t, entryView, "  Implement add flow  ")
	if cmd != nil {
		t.Fatalf("task typing cmd = %v, want nil", cmd)
	}
	entryView, cmd = updateStatusModel(t, entryView, tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		t.Fatalf("tab cmd = %v, want nil", cmd)
	}
	entryView, cmd = typeIntoStatusModel(t, entryView, "  TUI add  ")
	if cmd != nil {
		t.Fatalf("summary typing cmd = %v, want nil", cmd)
	}

	afterSubmit, cmd := updateStatusModel(t, entryView, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submit returned nil cmd")
	}
	if len(calls) != 0 {
		t.Fatalf("callbacks ran before command execution: %#v", calls)
	}

	afterAdd, cmd := runStatusModelCmd(t, afterSubmit, cmd)
	if cmd != nil {
		t.Fatalf("add message cmd = %v, want nil", cmd)
	}
	if !reflect.DeepEqual(calls, []string{"add", "refresh"}) {
		t.Fatalf("callback order = %#v, want add then refresh", calls)
	}
	if got, want := input.Task, "Implement add flow"; got != want {
		t.Fatalf("add input task = %q, want %q", got, want)
	}
	if got, want := input.Summary, "TUI add"; got != want {
		t.Fatalf("add input summary = %q, want %q", got, want)
	}
	if afterAdd.view != viewTasks {
		t.Fatalf("view = %v, want tasks", afterAdd.view)
	}
	if got, want := afterAdd.selectedTaskID(), "task-new"; got != want {
		t.Fatalf("selected task = %q, want %q", got, want)
	}

	lines := normalizedViewLines(afterAdd.View())
	requireLines(t, lines,
		"Notice: Added task task-new.",
		"> - task-new  pending  TUI add",
		"ID: task-new",
		"Status: pending",
		"Task: --- id: task-new status: pending --- # TUI add Implement add flow",
	)
}

func TestStatusModelRefreshActionReloadsStatusSnapshot(t *testing.T) {
	refreshed := false
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{{
			ID:      "run-old",
			Status:  ledger.StatusCompleted,
			Summary: "old summary",
		}},
	}, StatusActions{
		RefreshStatus: func() (app.StatusResult, error) {
			refreshed = true
			return app.StatusResult{
				Initialized: true,
				Tasks: []taskmodel.Task{
					{ID: "task-1", Status: taskmodel.StatusPending},
					{ID: "task-2", Status: taskmodel.StatusCompleted},
				},
				RecentRuns: []ledger.Run{{
					ID:      "run-new",
					Status:  ledger.StatusFailed,
					Summary: "new summary",
				}},
			}, nil
		},
	})
	resized, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	afterKey, cmd := updateStatusModel(t, resized, keyRunes("r"))
	if cmd == nil {
		t.Fatal("refresh key returned nil cmd")
	}
	if refreshed {
		t.Fatal("refresh callback ran before command execution")
	}

	afterRefresh, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("refresh message cmd = %v, want nil", cmd)
	}
	if !refreshed {
		t.Fatal("refresh callback was not called")
	}

	lines := normalizedViewLines(afterRefresh.View())
	for _, want := range []string{
		"Notice: Refreshed.",
		"Total: 2",
		"Pending: 1",
		"Completed: 1",
		"ID: run-new",
	} {
		if !containsLine(lines, want) {
			t.Fatalf("refreshed view missing %q: %#v", want, lines)
		}
	}

	runsView, cmd := updateStatusModel(t, afterRefresh, keyRunes("3"))
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}
	if !containsLine(normalizedViewLines(runsView.View()), "> run-new  failed  none  none  new summary") {
		t.Fatalf("refreshed runs view missing run line:\n%s", runsView.View())
	}
}

func TestStatusModelPreflightViewShowsReadyChecks(t *testing.T) {
	called := false
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		Preflight: func() (app.PreflightResult, error) {
			called = true
			return app.PreflightResult{
				Ready: true,
				Checks: []app.PreflightCheck{
					{Status: app.PreflightOK, Name: "state", Detail: "initialized at /work/.revolvr"},
					{Status: app.PreflightOK, Name: "verification commands", Detail: "1 command configured"},
				},
			}, nil
		},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 140, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	preflightView, cmd := updateStatusModel(t, model, keyRunes("5"))
	if cmd == nil {
		t.Fatal("preflight key returned nil cmd")
	}
	if called {
		t.Fatal("preflight callback ran before command execution")
	}
	afterPreflight, cmd := runStatusModelCmd(t, preflightView, cmd)
	if cmd != nil {
		t.Fatalf("preflight message cmd = %v, want nil", cmd)
	}
	if !called {
		t.Fatal("preflight callback was not called")
	}

	lines := normalizedViewLines(afterPreflight.View())
	requireLines(t, lines,
		"Views: Dashboard | Tasks | Runs | Run Detail | [Preflight] | Help",
		"Notice: Preflight ready.",
		"Preflight",
		"Status: ready",
		"Ready: true",
		"Checks",
		"OK state: initialized at /work/.revolvr",
		"OK verification commands: 1 command configured",
		"Keys: p Check | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help | a Add Task | R Run Once | n Passes 3 | L Run Loop",
		"      r Refresh | q Quit",
	)
}

func TestStatusModelPreflightViewShowsFailedChecks(t *testing.T) {
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		Preflight: func() (app.PreflightResult, error) {
			return app.PreflightResult{
				Ready: false,
				Checks: []app.PreflightCheck{
					{Status: app.PreflightFail, Name: "codex executable", Detail: `"codex" not found: executable file not found`},
					{Status: app.PreflightFail, Name: "verification commands", Detail: "no verification commands configured"},
				},
			}, nil
		},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 140, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	preflightView, cmd := updateStatusModel(t, model, keyRunes("5"))
	if cmd == nil {
		t.Fatal("preflight key returned nil cmd")
	}
	afterPreflight, cmd := runStatusModelCmd(t, preflightView, cmd)
	if cmd != nil {
		t.Fatalf("preflight message cmd = %v, want nil", cmd)
	}

	requireLines(t, normalizedViewLines(afterPreflight.View()),
		"Notice: Preflight failed.",
		"Status: failed",
		"Ready: false",
		`FAIL codex executable: "codex" not found: executable file not found`,
		"FAIL verification commands: no verification commands configured",
	)
}

func TestStatusModelRunOnceRequiresReadyPreflightAndRejectsActiveRun(t *testing.T) {
	calls := 0
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunOnce: func(context.Context, app.RunProgress) (runonce.Result, error) {
			calls++
			return runonce.Result{}, nil
		},
	})

	afterBlocked, cmd := updateStatusModel(t, model, keyRunes("R"))
	if cmd != nil {
		t.Fatalf("run without preflight cmd = %v, want nil", cmd)
	}
	if calls != 0 {
		t.Fatalf("run calls = %d, want 0", calls)
	}
	requireLines(t, normalizedViewLines(afterBlocked.View()),
		"Notice: Run blocked: preflight is not ready.",
	)

	afterBlocked.preflight = preflightState{
		Checked: true,
		Result:  app.PreflightResult{Ready: false},
	}
	afterFailedPreflight, cmd := updateStatusModel(t, afterBlocked, keyRunes("R"))
	if cmd != nil {
		t.Fatalf("run with failed preflight cmd = %v, want nil", cmd)
	}
	if calls != 0 {
		t.Fatalf("run calls = %d, want 0", calls)
	}

	afterFailedPreflight.preflight = preflightState{
		Checked: true,
		Result:  app.PreflightResult{Ready: true},
	}
	active, cmd := updateStatusModel(t, afterFailedPreflight, keyRunes("R"))
	if cmd == nil {
		t.Fatal("run with ready preflight returned nil cmd")
	}
	if !active.runOnce.Active {
		t.Fatal("run active = false, want true")
	}
	again, secondCmd := updateStatusModel(t, active, keyRunes("R"))
	if secondCmd != nil {
		t.Fatalf("second run cmd = %v, want nil", secondCmd)
	}
	if calls != 0 {
		t.Fatalf("run calls before command execution = %d, want 0", calls)
	}
	if !again.runOnce.Active {
		t.Fatal("run active after second run key = false, want true")
	}

	afterAdd, addCmd := updateStatusModel(t, again, keyRunes("a"))
	if addCmd != nil {
		t.Fatalf("add while running cmd = %v, want nil", addCmd)
	}
	if afterAdd.view == viewTaskEntry {
		t.Fatal("add task entry opened while run was active")
	}
	requireLines(t, normalizedViewLines(afterAdd.View()),
		"Notice: Run is active; cancel or wait before starting another action.",
		"Run Progress",
		"Status: running",
	)
}

func TestStatusModelRunOnceStreamsProgressAndRefreshesCompletion(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:          "run-success",
			TaskID:      "task-success",
			Task:        "Run from TUI",
			Status:      ledger.StatusCompleted,
			Summary:     "committed",
			StartedAt:   startedAt,
			CompletedAt: &completedAt,
			CommitSHA:   "abc123",
		},
		Events: []ledger.Event{
			{ID: 1, RunID: "run-success", Type: ledger.EventRunStarted, CreatedAt: startedAt},
			{ID: 2, RunID: "run-success", Type: ledger.EventRunCompleted, CreatedAt: completedAt},
		},
	}
	var calls []string
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunOnce: func(_ context.Context, progress app.RunProgress) (runonce.Result, error) {
			calls = append(calls, "run")
			progress(codexexec.ProgressEvent{Source: "codex", Message: "thread started"})
			progress(codexexec.ProgressEvent{Source: "codex stderr", Message: "checking worktree"})
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     history.Run,
				Task:    taskmodel.Task{ID: "task-success"},
			}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{
				Initialized: true,
				RecentRuns:  []ledger.Run{history.Run},
			}, nil
		},
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			calls = append(calls, "open:"+runID)
			return history, nil
		},
	})
	model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 140, Height: 60})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	afterKey, cmd := updateStatusModel(t, model, keyRunes("R"))
	if cmd == nil {
		t.Fatal("run key returned nil cmd")
	}
	if len(calls) != 0 {
		t.Fatalf("run callbacks ran before command execution: %#v", calls)
	}

	afterRun := drainStatusModelCmds(t, afterKey, cmd)
	if !reflect.DeepEqual(calls, []string{"run", "refresh", "open:run-success"}) {
		t.Fatalf("callback order = %#v, want run refresh open", calls)
	}
	if afterRun.runOnce.Active {
		t.Fatal("run active = true after completion")
	}
	if afterRun.runDetails == nil || afterRun.runDetails.Run.ID != "run-success" {
		t.Fatalf("run detail = %+v, want run-success", afterRun.runDetails)
	}
	if got, want := afterRun.selectedRunID(), "run-success"; got != want {
		t.Fatalf("selected run = %q, want %q", got, want)
	}

	requireLines(t, normalizedViewLines(afterRun.View()),
		"Notice: Run completed. run-success.",
		"Run Progress",
		"Status: completed",
		"Run ID: run-success",
		"Outcome: committed",
		"Log",
		"system: run started",
		"codex: thread started",
		"codex stderr: checking worktree",
		"system: terminal state: completed",
		"Latest Run",
		"ID: run-success",
	)
}

func TestStatusModelRunOnceFailureReportsTerminalState(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 15, 10, 0, 0, time.UTC)
	run := ledger.Run{
		ID:        "run-failed",
		TaskID:    "task-failed",
		Task:      "Run from TUI and fail",
		Status:    ledger.StatusFailed,
		Summary:   "verification failed",
		StartedAt: startedAt,
	}
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunOnce: func(_ context.Context, progress app.RunProgress) (runonce.Result, error) {
			progress(codexexec.ProgressEvent{Source: "codex", Message: "message: working"})
			return runonce.Result{
				Outcome: runonce.OutcomeVerificationFailed,
				Run:     run,
				Task:    taskmodel.Task{ID: "task-failed"},
				Message: "verification command 0 failed",
			}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			return app.StatusResult{
				Initialized: true,
				RecentRuns:  []ledger.Run{run},
			}, nil
		},
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			return ledger.RunWithEvents{Run: run}, nil
		},
	})
	model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}

	afterKey, cmd := updateStatusModel(t, model, keyRunes("R"))
	if cmd == nil {
		t.Fatal("run key returned nil cmd")
	}
	afterRun := drainStatusModelCmds(t, afterKey, cmd)

	requireLines(t, normalizedViewLines(afterRun.View()),
		"Notice: Run failed. run-failed.",
		"Run Progress",
		"Status: failed",
		"Run ID: run-failed",
		"Outcome: verification_failed",
		"codex: message: working",
		"system: terminal state: failed",
	)
}

func TestStatusModelRunOnceCancellationReportsTerminalState(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 15, 20, 0, 0, time.UTC)
	run := ledger.Run{
		ID:        "run-cancelled",
		TaskID:    "task-cancelled",
		Task:      "Cancel a TUI run",
		Status:    ledger.StatusFailed,
		StartedAt: startedAt,
	}
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunOnce: func(ctx context.Context, progress app.RunProgress) (runonce.Result, error) {
			progress(codexexec.ProgressEvent{Source: "codex", Message: "started"})
			<-ctx.Done()
			return runonce.Result{
				Outcome: runonce.OutcomeBlocked,
				Run:     run,
				Task:    taskmodel.Task{ID: "task-cancelled"},
				Message: "context canceled",
			}, ctx.Err()
		},
		RefreshStatus: func() (app.StatusResult, error) {
			return app.StatusResult{
				Initialized: true,
				RecentRuns:  []ledger.Run{run},
			}, nil
		},
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			return ledger.RunWithEvents{Run: run}, nil
		},
	})
	model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}

	afterKey, cmd := updateStatusModel(t, model, keyRunes("R"))
	if cmd == nil {
		t.Fatal("run key returned nil cmd")
	}
	afterProgress, waitCmd := runStatusModelCmd(t, afterKey, cmd)
	if waitCmd == nil {
		t.Fatal("progress update returned nil wait command")
	}

	cancelled, cancelCmd := updateStatusModel(t, afterProgress, keyRunes("c"))
	if cancelCmd != nil {
		t.Fatalf("cancel key cmd = %v, want nil", cancelCmd)
	}
	if !cancelled.runOnce.CancelRequested {
		t.Fatal("cancel requested = false, want true")
	}
	requireLines(t, normalizedViewLines(cancelled.View()),
		"Notice: Cancellation requested.",
		"Status: running",
		"Cancellation: requested",
		"system: cancellation requested",
	)

	afterCancel := drainStatusModelCmds(t, cancelled, waitCmd)
	requireLines(t, normalizedViewLines(afterCancel.View()),
		"Notice: Run cancelled. run-cancelled.",
		"Run Progress",
		"Status: cancelled",
		"Run ID: run-cancelled",
		"Outcome: blocked",
		"Error: context canceled",
		"system: terminal state: cancelled",
	)
}

func TestStatusModelActiveQuitWaitsForMatchingTerminalAcrossRunModes(t *testing.T) {
	modes := []struct {
		name string
		key  string
	}{
		{name: "run once", key: "R"},
		{name: "loop", key: "L"},
		{name: "task run", key: "U"},
		{name: "queue", key: "Q"},
	}
	quitKeys := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{name: "q", msg: keyRunes("q")},
		{name: "ctrl-c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}},
	}
	for _, mode := range modes {
		for _, quitKey := range quitKeys {
			t.Run(mode.name+"/"+quitKey.name, func(t *testing.T) {
				status := app.StatusResult{Initialized: true}
				if mode.key == "U" {
					status.Tasks = []taskmodel.Task{{ID: "task-quit", Status: taskmodel.StatusPending, Workflow: taskfile.WorkflowAutonomousV1}}
				}
				started := make(chan struct{})
				cancelObserved := make(chan struct{})
				releaseCleanup := make(chan struct{})
				cleanupReleased := false
				defer func() {
					if !cleanupReleased {
						close(releaseCleanup)
					}
				}()
				cleanupFinished := make(chan struct{})
				refreshed := make(chan struct{})
				waitForCleanup := func(ctx context.Context) error {
					close(started)
					<-ctx.Done()
					close(cancelObserved)
					<-releaseCleanup
					close(cleanupFinished)
					return ctx.Err()
				}
				actions := StatusActions{
					RefreshStatus: func() (app.StatusResult, error) {
						close(refreshed)
						return status, nil
					},
				}
				switch mode.key {
				case "R":
					actions.RunOnce = func(ctx context.Context, _ app.RunProgress) (runonce.Result, error) {
						err := waitForCleanup(ctx)
						return runonce.Result{Outcome: runonce.OutcomeBlocked}, err
					}
				case "L":
					actions.RunLoop = func(ctx context.Context, maxPasses int, _ app.RunProgress, _ app.RunPassFunc) (app.RunLoopResult, error) {
						err := waitForCleanup(ctx)
						return app.RunLoopResult{Stats: app.RunLoopStats{MaxPasses: maxPasses, StopReason: "context_cancelled"}}, err
					}
				case "U":
					actions.RunTask = func(ctx context.Context, taskID string, _ int64, _ autonomoustaskrun.Progress) (autonomoustaskrun.Result, error) {
						err := waitForCleanup(ctx)
						return autonomoustaskrun.Result{TaskID: taskID, StopReason: autonomoustaskrun.StopOperationCancelled}, err
					}
				case "Q":
					actions.RunQueue = func(ctx context.Context, _, _ int64, _ autonomousqueue.Progress) (autonomousqueue.Result, error) {
						err := waitForCleanup(ctx)
						return autonomousqueue.Result{OperationID: "queue-quit", StopReason: autonomousqueue.StopCancelled}, err
					}
				}

				model := NewStatusModelWithActions(status, actions)
				model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
				model, startCmd := updateStatusModel(t, model, keyRunes(mode.key))
				if startCmd == nil {
					t.Fatal("start command is nil")
				}
				terminalMessages := make(chan tea.Msg, 1)
				go func() { terminalMessages <- startCmd() }()
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("run action did not start")
				}

				model, quitCmd := updateStatusModel(t, model, quitKey.msg)
				if quitCmd != nil {
					t.Fatal("active quit returned a command before settlement")
				}
				if !model.runOnce.Active || !model.runOnce.CancelRequested || !model.runOnce.QuitAfterSettlement {
					t.Fatalf("active quit state = %#v", model.runOnce)
				}
				select {
				case <-cancelObserved:
				case <-time.After(time.Second):
					t.Fatal("run action did not observe cancellation")
				}
				stale := runOnceDoneMsg{token: model.runOnce.Token - 1}
				model, staleCmd := updateStatusModel(t, model, stale)
				if staleCmd != nil || !model.runOnce.Active || !model.runOnce.QuitAfterSettlement {
					t.Fatalf("stale terminal released quit: cmd=%v state=%#v", staleCmd, model.runOnce)
				}
				select {
				case terminal := <-terminalMessages:
					t.Fatalf("terminal published before cleanup release: %T", terminal)
				case <-time.After(20 * time.Millisecond):
				}

				close(releaseCleanup)
				cleanupReleased = true
				var terminal tea.Msg
				select {
				case terminal = <-terminalMessages:
				case <-time.After(time.Second):
					t.Fatal("terminal message was not published")
				}
				for name, done := range map[string]<-chan struct{}{"cleanup": cleanupFinished, "refresh": refreshed} {
					select {
					case <-done:
					default:
						t.Fatalf("%s was incomplete before terminal publication", name)
					}
				}
				model, quitCmd = updateStatusModel(t, model, terminal)
				if quitCmd == nil {
					t.Fatal("matching terminal did not release delayed quit")
				}
				if model.runOnce.Active || model.runOnce.QuitAfterSettlement {
					t.Fatalf("settled state = %#v", model.runOnce)
				}
				if _, ok := quitCmd().(tea.QuitMsg); !ok {
					t.Fatal("matching terminal command is not tea.Quit")
				}
			})
		}
	}
}

func TestStatusModelRunLoopCyclesPassCount(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})

	afterFirst, cmd := updateStatusModel(t, model, keyRunes("n"))
	if cmd != nil {
		t.Fatalf("first cycle cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(afterFirst.View()),
		"Notice: Loop max passes set to 5.",
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help",
		"      a Add Task | R Run Once | n Passes 5 | L Run Loop | r Refresh | q Quit",
	)

	afterSecond, cmd := updateStatusModel(t, afterFirst, keyRunes("n"))
	if cmd != nil {
		t.Fatalf("second cycle cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(afterSecond.View()),
		"Notice: Loop max passes set to 2.",
		"      a Add Task | R Run Once | n Passes 2 | L Run Loop | r Refresh | q Quit",
	)
}

func TestStatusModelRunLoopMaxPassCompletionRefreshesAndOpensLatestRun(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 15, 30, 0, 0, time.UTC)
	runs := []ledger.Run{
		{ID: "run-loop-1", TaskID: "task-loop-1", Task: "Loop task 1", Status: ledger.StatusCompleted, StartedAt: startedAt, CommitSHA: "abc1"},
		{ID: "run-loop-2", TaskID: "task-loop-2", Task: "Loop task 2", Status: ledger.StatusCompleted, StartedAt: startedAt.Add(time.Minute), CommitSHA: "abc2"},
		{ID: "run-loop-3", TaskID: "task-loop-3", Task: "Loop task 3", Status: ledger.StatusCompleted, StartedAt: startedAt.Add(2 * time.Minute), CommitSHA: "abc3"},
	}
	var calls []string
	model := runLoopReadyModel(StatusActions{
		RunLoop: func(_ context.Context, maxPasses int, progress app.RunProgress, onPass app.RunPassFunc) (app.RunLoopResult, error) {
			calls = append(calls, fmt.Sprintf("loop:%d", maxPasses))
			if maxPasses != 3 {
				t.Fatalf("max passes = %d, want default 3", maxPasses)
			}
			progress(codexexec.ProgressEvent{Source: "codex", Message: "loop started"})
			for i, run := range runs {
				if err := onPass(runonce.Result{
					Outcome: runonce.OutcomeCommitted,
					Run:     run,
					Task:    taskmodel.Task{ID: run.TaskID},
					Commit:  commit.Result{CommitSHA: run.CommitSHA},
				}); err != nil {
					return app.RunLoopResult{}, err
				}
				calls = append(calls, fmt.Sprintf("pass:%d", i+1))
			}
			return app.RunLoopResult{Stats: app.RunLoopStats{
				MaxPasses:  3,
				Passes:     3,
				Completed:  3,
				StopReason: "max_passes",
			}}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{
				Initialized: true,
				RecentRuns:  []ledger.Run{runs[2], runs[1], runs[0]},
			}, nil
		},
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			calls = append(calls, "open:"+runID)
			if runID != "run-loop-3" {
				t.Fatalf("opened run id = %q, want run-loop-3", runID)
			}
			return ledger.RunWithEvents{Run: runs[2]}, nil
		},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 160, Height: 80})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	afterKey, cmd := updateStatusModel(t, model, keyRunes("L"))
	if cmd == nil {
		t.Fatal("loop key returned nil cmd")
	}
	if len(calls) != 0 {
		t.Fatalf("loop callbacks ran before command execution: %#v", calls)
	}

	afterLoop := drainStatusModelCmds(t, afterKey, cmd)
	if afterLoop.runOnce.Active {
		t.Fatal("loop active = true after completion")
	}
	if afterLoop.runDetails == nil || afterLoop.runDetails.Run.ID != "run-loop-3" {
		t.Fatalf("run detail = %+v, want run-loop-3", afterLoop.runDetails)
	}
	requireLines(t, normalizedViewLines(afterLoop.View()),
		"Notice: Loop completed. Latest run run-loop-3.",
		"Run Progress",
		"Status: completed",
		"Mode: loop",
		"Max passes: 3",
		"Passes: 3/3",
		"Completed: 3",
		"Failed or blocked: 0",
		"No task: false",
		"Stop reason: max_passes",
		"Latest run ID: run-loop-3",
		"codex: loop started",
		"pass 1: run run-loop-1 completed task task-loop-1; commit abc1",
		"pass 2: run run-loop-2 completed task task-loop-2; commit abc2",
		"pass 3: run run-loop-3 completed task task-loop-3; commit abc3",
		"system: terminal state: completed",
	)
}

func TestStatusModelRunLoopNoTaskStopRefreshesStatus(t *testing.T) {
	var calls []string
	model := runLoopReadyModel(StatusActions{
		RunLoop: func(_ context.Context, maxPasses int, _ app.RunProgress, onPass app.RunPassFunc) (app.RunLoopResult, error) {
			calls = append(calls, fmt.Sprintf("loop:%d", maxPasses))
			if err := onPass(runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}); err != nil {
				return app.RunLoopResult{}, err
			}
			return app.RunLoopResult{Stats: app.RunLoopStats{
				MaxPasses:  maxPasses,
				Passes:     1,
				NoTask:     true,
				StopReason: "no_task",
			}}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{Initialized: true}, nil
		},
		OpenRun: func(string) (ledger.RunWithEvents, error) {
			t.Fatal("open run callback should not run without a loop run id")
			return ledger.RunWithEvents{}, nil
		},
	})

	afterKey, cmd := updateStatusModel(t, model, keyRunes("L"))
	if cmd == nil {
		t.Fatal("loop key returned nil cmd")
	}
	afterLoop := drainStatusModelCmds(t, afterKey, cmd)
	if !reflect.DeepEqual(calls, []string{"loop:3", "refresh"}) {
		t.Fatalf("calls = %#v, want loop then refresh", calls)
	}
	requireLines(t, normalizedViewLines(afterLoop.View()),
		"Notice: Loop finished: no pending runnable tasks.",
		"Status: no_task",
		"Passes: 1/3",
		"Completed: 0",
		"Failed or blocked: 0",
		"No task: true",
		"Stop reason: no_task",
		"pass 1: no pending runnable tasks",
		"system: terminal state: no_task",
	)
}

func TestStatusModelRunLoopRepeatedFailureGuardrail(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 15, 40, 0, 0, time.UTC)
	runs := []ledger.Run{
		{ID: "run-failed-1", TaskID: "task-failed-1", Task: "Fail task 1", Status: ledger.StatusFailed, StartedAt: startedAt},
		{ID: "run-failed-2", TaskID: "task-failed-2", Task: "Fail task 2", Status: ledger.StatusFailed, StartedAt: startedAt.Add(time.Minute)},
	}
	model := runLoopReadyModel(StatusActions{
		RunLoop: func(_ context.Context, maxPasses int, _ app.RunProgress, onPass app.RunPassFunc) (app.RunLoopResult, error) {
			for _, run := range runs {
				if err := onPass(runonce.Result{
					Outcome: runonce.OutcomeVerificationFailed,
					Run:     run,
					Task:    taskmodel.Task{ID: run.TaskID},
					Message: "verification command 0 failed",
				}); err != nil {
					return app.RunLoopResult{}, err
				}
			}
			return app.RunLoopResult{Stats: app.RunLoopStats{
				MaxPasses:                  maxPasses,
				Passes:                     2,
				FailedOrBlocked:            2,
				StopReason:                 "failure_guardrail",
				ConsecutiveFailedOrBlocked: 2,
			}}, errors.New("run loop stopped after 2 consecutive failed or blocked passes")
		},
		RefreshStatus: func() (app.StatusResult, error) {
			return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{runs[1], runs[0]}}, nil
		},
		OpenRun: func(string) (ledger.RunWithEvents, error) {
			return ledger.RunWithEvents{Run: runs[1]}, nil
		},
	})

	afterKey, cmd := updateStatusModel(t, model, keyRunes("L"))
	if cmd == nil {
		t.Fatal("loop key returned nil cmd")
	}
	afterLoop := drainStatusModelCmds(t, afterKey, cmd)
	requireLines(t, normalizedViewLines(afterLoop.View()),
		"Notice: Loop failed. Latest run run-failed-2.",
		"Status: failed",
		"Passes: 2/3",
		"Completed: 0",
		"Failed or blocked: 2",
		"Consecutive failed or blocked: 2",
		"Stop reason: failure_guardrail",
		"Latest run ID: run-failed-2",
		"Error: run loop stopped after 2 consecutive failed or blocked passes",
		"pass 1: run run-failed-1 stopped (verification_failed): verification command 0 failed",
		"pass 2: run run-failed-2 stopped (verification_failed): verification command 0 failed",
	)
}

func TestStatusModelRunLoopBlockedStop(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 15, 50, 0, 0, time.UTC)
	run := ledger.Run{ID: "run-blocked", TaskID: "task-blocked", Task: "Blocked task", Status: ledger.StatusFailed, StartedAt: startedAt}
	model := runLoopReadyModel(StatusActions{
		RunLoop: func(_ context.Context, maxPasses int, _ app.RunProgress, onPass app.RunPassFunc) (app.RunLoopResult, error) {
			if err := onPass(runonce.Result{
				Outcome: runonce.OutcomeBlocked,
				Run:     run,
				Task:    taskmodel.Task{ID: "task-blocked"},
				Message: "blocked by preflight",
			}); err != nil {
				return app.RunLoopResult{}, err
			}
			return app.RunLoopResult{Stats: app.RunLoopStats{
				MaxPasses:                  maxPasses,
				Passes:                     1,
				FailedOrBlocked:            1,
				StopReason:                 "failed_or_blocked",
				ConsecutiveFailedOrBlocked: 1,
			}}, errors.New("run run-blocked stopped with outcome blocked")
		},
		RefreshStatus: func() (app.StatusResult, error) {
			return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{run}}, nil
		},
		OpenRun: func(string) (ledger.RunWithEvents, error) {
			return ledger.RunWithEvents{Run: run}, nil
		},
	})

	afterKey, cmd := updateStatusModel(t, model, keyRunes("L"))
	if cmd == nil {
		t.Fatal("loop key returned nil cmd")
	}
	afterLoop := drainStatusModelCmds(t, afterKey, cmd)
	requireLines(t, normalizedViewLines(afterLoop.View()),
		"Notice: Loop failed. Latest run run-blocked.",
		"Status: failed",
		"Passes: 1/3",
		"Failed or blocked: 1",
		"Stop reason: failed_or_blocked",
		"Latest run ID: run-blocked",
		"Error: run run-blocked stopped with outcome blocked",
		"pass 1: run run-blocked stopped (blocked): blocked by preflight",
	)
}

func TestStatusModelRunLoopCancellationReportsTerminalState(t *testing.T) {
	cancelled := false
	refreshCalled := false
	model := runLoopReadyModel(StatusActions{
		RunLoop: func(ctx context.Context, maxPasses int, progress app.RunProgress, _ app.RunPassFunc) (app.RunLoopResult, error) {
			progress(codexexec.ProgressEvent{Source: "codex", Message: "loop started"})
			<-ctx.Done()
			cancelled = true
			return app.RunLoopResult{Stats: app.RunLoopStats{
				MaxPasses:  maxPasses,
				StopReason: "context_cancelled",
			}}, ctx.Err()
		},
		RefreshStatus: func() (app.StatusResult, error) {
			refreshCalled = true
			return app.StatusResult{Initialized: true}, nil
		},
	})

	afterKey, cmd := updateStatusModel(t, model, keyRunes("L"))
	if cmd == nil {
		t.Fatal("loop key returned nil cmd")
	}
	afterProgress, waitCmd := runStatusModelCmd(t, afterKey, cmd)
	if waitCmd == nil {
		t.Fatal("progress update returned nil wait command")
	}

	cancelView, cancelCmd := updateStatusModel(t, afterProgress, keyRunes("c"))
	if cancelCmd != nil {
		t.Fatalf("cancel key cmd = %v, want nil", cancelCmd)
	}
	if !cancelView.runOnce.CancelRequested {
		t.Fatal("cancel requested = false, want true")
	}
	requireLines(t, normalizedViewLines(cancelView.View()),
		"Notice: Cancellation requested.",
		"Status: running",
		"Mode: loop",
		"Cancellation: requested",
		"system: cancellation requested",
	)

	afterCancel := drainStatusModelCmds(t, cancelView, waitCmd)
	if !cancelled {
		t.Fatal("loop context was not cancelled")
	}
	if !refreshCalled {
		t.Fatal("refresh callback was not called after cancellation")
	}
	requireLines(t, normalizedViewLines(afterCancel.View()),
		"Notice: Loop cancelled.",
		"Status: cancelled",
		"Stop reason: context_cancelled",
		"Error: context canceled",
		"system: terminal state: cancelled",
	)
}

func TestStatusModelRunSelectedAutonomousTaskPinsSelection(t *testing.T) {
	status := app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{{ID: "task-one", Status: taskmodel.StatusPending, Workflow: taskfile.WorkflowAutonomousV1}, {ID: "task-two", Status: taskmodel.StatusPending, Workflow: taskfile.WorkflowAutonomousV1}}}
	called := ""
	m := NewStatusModelWithActions(status, StatusActions{RunTask: func(_ context.Context, taskID string, max int64, progress autonomoustaskrun.Progress) (autonomoustaskrun.Result, error) {
		called = taskID
		if max != 50 {
			t.Fatalf("max=%d", max)
		}
		progress(autonomoustaskrun.Operation{Stage: "cycle_started", Statistics: autonomoustaskrun.Statistics{CyclesStarted: 1}, LastAction: "implement"})
		return autonomoustaskrun.Result{TaskID: taskID, OperationID: "operation-one", StopReason: autonomoustaskrun.StopBlocked, Statistics: autonomoustaskrun.Statistics{CyclesStarted: 1}}, nil
	}, RefreshStatus: func() (app.StatusResult, error) { return status, nil }})
	m.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = updated.(StatusModel)
	if cmd == nil {
		t.Fatal("start command nil")
	}
	for i := 0; i < 4 && m.runOnce.Active; i++ {
		msg := cmd()
		updated, cmd = m.Update(msg)
		m = updated.(StatusModel)
	}
	if called != "task-one" || m.runOnce.Active || m.runOnce.Outcome != "blocked" {
		t.Fatalf("called=%q state=%+v", called, m.runOnce)
	}
}

func TestStatusModelRejectsSelectedAutonomousTaskThatIsNotDependencyReady(t *testing.T) {
	called := false
	status := app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{{ID: "waiting", Status: taskmodel.StatusPending, Workflow: taskfile.WorkflowAutonomousV1, ReadinessReason: "waiting_dependency"}}}
	m := NewStatusModelWithActions(status, StatusActions{RunTask: func(context.Context, string, int64, autonomoustaskrun.Progress) (autonomoustaskrun.Result, error) {
		called = true
		return autonomoustaskrun.Result{}, nil
	}})
	m.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	m = updated.(StatusModel)
	if cmd != nil || called || !strings.Contains(m.message, "waiting_dependency") {
		t.Fatalf("cmd=%v called=%v message=%q", cmd, called, m.message)
	}
}

func TestStatusModelRunsViewNavigatesRecentRunsWithMetadata(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{
			{
				ID:                 "run-new",
				Status:             ledger.StatusFailed,
				VerificationStatus: "failed",
				Summary:            "verification failed",
			},
			{
				ID:                 "run-mid",
				Status:             ledger.StatusCompleted,
				VerificationStatus: "passed",
				CommitSHA:          "abc123",
				Summary:            "committed change",
			},
			{
				ID:      "run-old",
				Status:  ledger.StatusRunning,
				Summary: "still running",
			},
		},
	})
	runsView := openRunsView(t, model)

	requireLines(t, normalizedViewLines(runsView.View()),
		"Views: Dashboard | Tasks | [Runs] | Run Detail | Preflight | Help",
		"ID  STATUS  VERIFICATION  COMMIT  SUMMARY",
		"> run-new  failed  failed  none  verification failed",
		"  run-mid  completed  passed  abc123  committed change",
		"  run-old  running  none  none  still running",
	)

	afterDown, cmd := updateStatusModel(t, runsView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(afterDown.View()),
		"  run-new  failed  failed  none  verification failed",
		"> run-mid  completed  passed  abc123  committed change",
	)
}

func TestStatusModelRunsViewOpensSelectedRunDetail(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 12, 30, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	openedRunID := ""
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{
			{ID: "run-new", Status: ledger.StatusCompleted, Summary: "new summary"},
			{ID: "run-old", Status: ledger.StatusFailed, Summary: "old summary"},
		},
	}, StatusActions{
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			openedRunID = runID
			return ledger.RunWithEvents{
				Run: ledger.Run{
					ID:                 runID,
					TaskID:             "task-open",
					Task:               "Open selected run",
					Status:             ledger.StatusFailed,
					Summary:            "opened detail",
					StartedAt:          startedAt,
					CompletedAt:        &completedAt,
					VerificationStatus: "failed",
				},
				Events: []ledger.Event{
					{ID: 1, RunID: runID, Type: ledger.EventRunStarted, CreatedAt: startedAt},
				},
			}, nil
		},
	})
	runsView := openRunsView(t, model)
	selected, cmd := updateStatusModel(t, runsView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection cmd = %v, want nil", cmd)
	}

	afterOpenKey, cmd := updateStatusModel(t, selected, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("open key returned nil cmd")
	}
	if openedRunID != "" {
		t.Fatal("open callback ran before command execution")
	}

	afterOpen, cmd := runStatusModelCmd(t, afterOpenKey, cmd)
	if cmd != nil {
		t.Fatalf("open message cmd = %v, want nil", cmd)
	}
	if openedRunID != "run-old" {
		t.Fatalf("opened run id = %q, want run-old", openedRunID)
	}
	if afterOpen.view != viewRunDetail {
		t.Fatalf("view = %v, want run detail", afterOpen.view)
	}
	requireLines(t, normalizedViewLines(afterOpen.View()),
		"Run Detail",
		"Summary",
		"ID: run-old",
		"Task ID: task-open",
		"Task: Open selected run",
	)
}

func TestStatusModelRunDetailRendersDiagnosticsAndChangedFiles(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 13, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(45 * time.Second)
	exitCode := 0
	failedIndex := 0
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:                 "run-diagnostics",
			TaskID:             "task-diagnostics",
			Task:               "Surface run diagnostics",
			Status:             ledger.StatusFailed,
			Summary:            "verification command 0 failed",
			StartedAt:          startedAt,
			CompletedAt:        &completedAt,
			CodexExitCode:      &exitCode,
			VerificationStatus: "failed",
		},
		Events: []ledger.Event{
			{
				ID:        1,
				RunID:     "run-diagnostics",
				Type:      ledger.EventCodexCompleted,
				Payload:   jsonPayload(t, map[string]any{"exit_code": 0, "timed_out": false}),
				CreatedAt: startedAt.Add(time.Second),
			},
			{
				ID:    2,
				RunID: "run-diagnostics",
				Type:  ledger.EventChangedFilesCaptured,
				Payload: jsonPayload(t, map[string]any{
					"changed_files": []string{"internal/broken.go", "internal/broken.go", " docs/readme.md "},
				}),
				CreatedAt: startedAt.Add(2 * time.Second),
			},
			{
				ID:    3,
				RunID: "run-diagnostics",
				Type:  ledger.EventVerificationCompleted,
				Payload: jsonPayload(t, map[string]any{
					"status":               "failed",
					"failed_command_index": failedIndex,
					"commands": []map[string]any{{
						"index":     0,
						"command":   "go test ./...",
						"status":    "failed",
						"passed":    false,
						"exit_code": 1,
					}},
				}),
				CreatedAt: startedAt.Add(3 * time.Second),
			},
			{
				ID:    4,
				RunID: "run-diagnostics",
				Type:  ledger.EventReceiptSynthesized,
				Payload: jsonPayload(t, map[string]any{
					"receipt_path": ".revolvr/receipts/run-diagnostics.md",
					"verdict":      "verification_failed",
				}),
				CreatedAt: startedAt.Add(4 * time.Second),
			},
			{
				ID:    5,
				RunID: "run-diagnostics",
				Type:  ledger.EventReceiptWarning,
				Payload: jsonPayload(t, map[string]any{
					"warning_type": "changed_files_mismatch",
					"message":      "receipt changed files differ from harness captured changed files",
					"receipt_path": ".revolvr/receipts/run-diagnostics.md",
				}),
				CreatedAt: startedAt.Add(5 * time.Second),
			},
			{
				ID:    6,
				RunID: "run-diagnostics",
				Type:  ledger.EventRunFailed,
				Payload: jsonPayload(t, map[string]any{
					"outcome": "verification_failed",
					"message": "verification command 0 failed",
				}),
				CreatedAt: completedAt,
			},
		},
	}
	view := runDetailView(t, history, 140, 60)

	requireLines(t, normalizedViewLines(view.View()),
		"Diagnostics",
		"outcome: verification_failed",
		"message: verification command 0 failed",
		"codex: exit_code=0, timed_out=false",
		"verification: failed",
		"failed verification: go test ./... (exit_code=1)",
		"receipt: verification_failed (.revolvr/receipts/run-diagnostics.md)",
		"warning: changed_files_mismatch: receipt changed files differ from harness captured changed files (.revolvr/receipts/run-diagnostics.md)",
		"Changed Files",
		"internal/broken.go",
		"docs/readme.md",
	)
}

func TestStatusModelRunDetailRendersTimelineAndRawEvents(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 13, 5, 0, 0, time.UTC)
	completedAt := startedAt.Add(30 * time.Second)
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:          "run-timeline",
			TaskID:      "task-timeline",
			Task:        "Render a timeline",
			Status:      ledger.StatusCompleted,
			Summary:     "committed abc123",
			StartedAt:   startedAt,
			CompletedAt: &completedAt,
			CommitSHA:   "abc123",
		},
		Events: []ledger.Event{
			{
				ID:        1,
				RunID:     "run-timeline",
				Type:      ledger.EventRunStarted,
				Payload:   jsonPayload(t, map[string]any{"run_id": "run-timeline", "task_id": "task-timeline"}),
				CreatedAt: startedAt,
			},
			{
				ID:    2,
				RunID: "run-timeline",
				Type:  ledger.EventTaskSelected,
				Payload: jsonPayload(t, map[string]any{
					"task_id":      "task-timeline",
					"summary":      "Render a timeline",
					"workflow":     "mixed-pass-v1",
					"phase":        "audit",
					"profile_name": "auditor",
				}),
				CreatedAt: startedAt.Add(time.Second),
			},
			{
				ID:    3,
				RunID: "run-timeline",
				Type:  ledger.EventRunCompleted,
				Payload: jsonPayload(t, map[string]any{
					"outcome":             "committed",
					"message":             "committed abc123",
					"verification_status": "passed",
					"commit_sha":          "abc123",
				}),
				CreatedAt: completedAt,
			},
		},
	}
	view := runDetailView(t, history, 160, 80)

	requireLines(t, normalizedViewLines(view.View()),
		"Timeline",
		"TIMESTAMP  PHASE  STATUS  DETAIL",
		"2026-07-08T13:05:00Z  run  started  run run-timeline, task task-timeline",
		"2026-07-08T13:05:01Z  task  selected  task task-timeline: Render a timeline; workflow=mixed-pass-v1; phase=audit; profile=auditor",
		"2026-07-08T13:05:30Z  run  completed  outcome=committed: committed abc123; verification=passed; commit=abc123",
		"Events",
		"1  run_started  2026-07-08T13:05:00Z",
		"2  task_selected  2026-07-08T13:05:01Z",
		"3  run_completed  2026-07-08T13:05:30Z",
	)
}

func TestStatusModelRunDetailRendersNoTimelineRows(t *testing.T) {
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:     "run-empty-timeline",
			TaskID: "task-empty-timeline",
			Task:   "No projected events",
		},
	}
	view := runDetailView(t, history, 140, 60)

	requireLines(t, normalizedViewLines(view.View()),
		"Timeline",
		"No timeline rows.",
		"Events",
		"None",
	)
}

func TestStatusModelRunDetailRendersArtifactsAndMissingArtifacts(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 13, 10, 0, 0, time.UTC)
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:        "run-artifacts",
			TaskID:    "task-artifacts",
			Task:      "Render artifacts",
			Status:    ledger.StatusFailed,
			StartedAt: startedAt,
		},
		Events: []ledger.Event{
			{
				ID:    1,
				RunID: "run-artifacts",
				Type:  ledger.EventRunArtifacts,
				Payload: jsonPayload(t, ledger.RunArtifacts{
					ContextPayloadPath:  ".revolvr/runs/run-artifacts/context.md",
					ContextManifestPath: ".revolvr/runs/run-artifacts/context.json",
					ReceiptPath:         ".revolvr/receipts/run-artifacts.md",
				}),
				CreatedAt: startedAt,
			},
		},
	}
	view := runDetailView(t, history, 140, 60)

	requireLines(t, normalizedViewLines(view.View()),
		"Artifacts",
		"context payload: .revolvr/runs/run-artifacts/context.md",
		"context manifest: .revolvr/runs/run-artifacts/context.json",
		"codex stdout jsonl: missing",
		"codex stderr: missing",
		"last message: missing",
		"receipt: .revolvr/receipts/run-artifacts.md",
	)
}

func TestStatusModelRunDetailValidatesValidReceipt(t *testing.T) {
	history := validationDetailHistory("run-valid-receipt")
	view := runDetailView(t, history, 140, 60)
	calledRunID := ""
	view.actions.ValidateReceipt = func(runID string) (receipt.ValidationResult, error) {
		calledRunID = runID
		return receipt.ValidationResult{
			RunID:       runID,
			ReceiptPath: ".revolvr/receipts/run-valid-receipt.md",
			Checks: []receipt.ValidationCheck{
				{Name: receipt.ValidationCheckIdentity, Passed: true},
				{Name: receipt.ValidationCheckCompletionTime, Passed: true},
				{Name: receipt.ValidationCheckCommitSHA, Passed: true},
				{Name: receipt.ValidationCheckChangedFiles, Passed: true},
				{Name: receipt.ValidationCheckVerificationResults, Passed: true},
				{Name: receipt.ValidationCheckArtifacts, Passed: true},
			},
		}, nil
	}

	afterKey, cmd := updateStatusModel(t, view, keyRunes("v"))
	if cmd == nil {
		t.Fatal("validate key returned nil cmd")
	}
	if calledRunID != "" {
		t.Fatal("validation callback ran before command execution")
	}

	afterValidation, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("validation message cmd = %v, want nil", cmd)
	}
	if calledRunID != "run-valid-receipt" {
		t.Fatalf("validated run id = %q, want run-valid-receipt", calledRunID)
	}
	requireLines(t, normalizedViewLines(afterValidation.View()),
		"Notice: Receipt validation passed.",
		"Receipt Validation",
		"Status: passed",
		"Run ID: run-valid-receipt",
		"Receipt: .revolvr/receipts/run-valid-receipt.md",
		"Checks:",
		"PASS identity: ok",
		"PASS completion_time: ok",
		"PASS commit_sha: ok",
		"PASS changed_files: ok",
		"PASS verification_results: ok",
		"PASS artifacts: ok",
	)
}

func TestStatusModelRunDetailShowsFailedValidationChecks(t *testing.T) {
	history := validationDetailHistory("run-invalid-receipt")
	view := runDetailView(t, history, 140, 60)
	view.actions.ValidateReceipt = func(runID string) (receipt.ValidationResult, error) {
		return receipt.ValidationResult{
			RunID:       runID,
			ReceiptPath: ".revolvr/receipts/run-invalid-receipt.md",
			Checks: []receipt.ValidationCheck{
				{Name: receipt.ValidationCheckIdentity, Passed: true},
				{
					Name:   receipt.ValidationCheckChangedFiles,
					Passed: false,
					Details: []string{
						"frontmatter changed_files got [internal/stale.go], want [internal/actual.go]",
					},
				},
			},
		}, nil
	}

	afterKey, cmd := updateStatusModel(t, view, keyRunes("v"))
	if cmd == nil {
		t.Fatal("validate key returned nil cmd")
	}
	afterValidation, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("validation message cmd = %v, want nil", cmd)
	}

	requireLines(t, normalizedViewLines(afterValidation.View()),
		"Notice: Receipt validation failed.",
		"Receipt Validation",
		"Status: failed",
		"PASS identity: ok",
		"FAIL changed_files: failed - frontmatter changed_files got [internal/stale.go], want [internal/actual.go]",
	)
}

func TestStatusModelRunDetailShowsMissingReceiptValidationError(t *testing.T) {
	history := validationDetailHistory("run-missing-receipt")
	view := runDetailView(t, history, 140, 60)
	view.actions.ValidateReceipt = func(runID string) (receipt.ValidationResult, error) {
		return receipt.ValidationResult{}, errors.New("validate receipt: read .revolvr/receipts/run-missing-receipt.md: no such file or directory")
	}

	afterKey, cmd := updateStatusModel(t, view, keyRunes("v"))
	if cmd == nil {
		t.Fatal("validate key returned nil cmd")
	}
	afterValidation, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("validation message cmd = %v, want nil", cmd)
	}
	if afterValidation.view != viewRunDetail {
		t.Fatalf("view = %v, want run detail", afterValidation.view)
	}
	requireLines(t, normalizedViewLines(afterValidation.View()),
		"Notice: Receipt validation error.",
		"Receipt Validation",
		"Status: error",
		"Error: validate receipt: read .revolvr/receipts/run-missing-receipt.md: no such file or directory",
	)
}

func TestStatusModelRunDetailShowsValidationCallbackErrors(t *testing.T) {
	history := validationDetailHistory("run-validation-error")
	view := runDetailView(t, history, 140, 60)
	view.actions.ValidateReceipt = func(runID string) (receipt.ValidationResult, error) {
		return receipt.ValidationResult{}, errors.New("validation callback failed")
	}

	afterKey, cmd := updateStatusModel(t, view, keyRunes("v"))
	if cmd == nil {
		t.Fatal("validate key returned nil cmd")
	}
	afterValidation, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd != nil {
		t.Fatalf("validation message cmd = %v, want nil", cmd)
	}

	requireLines(t, normalizedViewLines(afterValidation.View()),
		"Notice: Receipt validation error.",
		"Receipt Validation",
		"Status: error",
		"Error: validation callback failed",
	)
}

func TestStatusModelRunDetailScrollsLongTimelineAndEventOutput(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 13, 20, 0, 0, time.UTC)
	events := make([]ledger.Event, 0, 80)
	for i := 1; i <= 80; i++ {
		events = append(events, ledger.Event{
			ID:        int64(i),
			RunID:     "run-long-events",
			Type:      ledger.EventCodexJSONEvent,
			CreatedAt: startedAt.Add(time.Duration(i-1) * time.Second),
		})
	}
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:        "run-long-events",
			TaskID:    "task-long-events",
			Task:      "Render long event output",
			Status:    ledger.StatusRunning,
			StartedAt: startedAt,
		},
		Events: events,
	}
	view := runDetailView(t, history, 160, 12)
	topLines := normalizedViewLines(view.View())
	if containsLine(topLines, "80  codex_json_event  2026-07-08T13:21:19Z") {
		t.Fatalf("top of long detail already showed last event: %#v", topLines)
	}

	timeline := view
	var cmd tea.Cmd
	for i := 0; i < 13; i++ {
		timeline, cmd = updateStatusModel(t, timeline, tea.KeyMsg{Type: tea.KeyDown})
		if cmd != nil {
			t.Fatalf("timeline scroll cmd %d = %v, want nil", i, cmd)
		}
	}
	requireLines(t, normalizedViewLines(timeline.View()),
		"Timeline",
		"TIMESTAMP  PHASE  STATUS  DETAIL",
		"2026-07-08T13:20:00Z  run  started  run run-long-events, task task-long-events",
	)

	bottom, cmd := updateStatusModel(t, view, tea.KeyMsg{Type: tea.KeyEnd})
	if cmd != nil {
		t.Fatalf("end key cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(bottom.View()),
		"76  codex_json_event  2026-07-08T13:21:15Z",
		"80  codex_json_event  2026-07-08T13:21:19Z",
	)
}

func TestStatusModelSwitchesViewsWithoutLosingLoadedRunDetail(t *testing.T) {
	startedAt := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(2 * time.Minute)
	exitCode := 1
	openedRunID := ""

	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{
			{ID: "run-new", Status: ledger.StatusCompleted, Summary: "new summary"},
			{ID: "run-old", Status: ledger.StatusFailed, Summary: "old summary"},
		},
	}, StatusActions{
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			openedRunID = runID
			return ledger.RunWithEvents{
				Run: ledger.Run{
					ID:                 "run-old",
					TaskID:             "task-old",
					Task:               "Inspect selected run",
					Status:             ledger.StatusFailed,
					Summary:            "verification failed",
					StartedAt:          startedAt,
					CompletedAt:        &completedAt,
					CodexExitCode:      &exitCode,
					VerificationStatus: "failed",
					CommitSHA:          "abc123",
				},
				Events: []ledger.Event{
					{ID: 1, RunID: "run-old", Type: ledger.EventRunStarted, CreatedAt: startedAt},
					{ID: 2, RunID: "run-old", Type: ledger.EventRunArtifacts, Payload: []byte(`{"receipt_path":".revolvr/receipts/run-old.md"}`), CreatedAt: completedAt},
				},
			}, nil
		},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 60})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	runsView, cmd := updateStatusModel(t, model, keyRunes("3"))
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}
	if runsView.view != viewRuns {
		t.Fatalf("view = %v, want runs", runsView.view)
	}

	afterDown, cmd := updateStatusModel(t, runsView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("selection move cmd = %v, want nil", cmd)
	}
	if !containsLine(normalizedViewLines(afterDown.View()), "> run-old  failed  none  none  old summary") {
		t.Fatalf("selected run marker missing after down:\n%s", afterDown.View())
	}

	afterEnter, cmd := updateStatusModel(t, afterDown, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("open key returned nil cmd")
	}
	if openedRunID != "" {
		t.Fatal("open callback ran before command execution")
	}

	afterOpen, cmd := runStatusModelCmd(t, afterEnter, cmd)
	if cmd != nil {
		t.Fatalf("open message cmd = %v, want nil", cmd)
	}
	if openedRunID != "run-old" {
		t.Fatalf("opened run id = %q, want run-old", openedRunID)
	}

	lines := normalizedViewLines(afterOpen.View())
	for _, want := range []string{
		"Views: Dashboard | Tasks | Runs | [Run Detail] | Preflight | Help",
		"Run Detail",
		"ID: run-old",
		"Task ID: task-old",
		"Task: Inspect selected run",
		"Status: failed",
		"Summary: verification failed",
		"Started: 2026-07-08T12:00:00Z",
		"Completed: 2026-07-08T12:02:00Z",
		"Codex exit code: 1",
		"Verification: failed",
		"Commit: abc123",
		"receipt: .revolvr/receipts/run-old.md",
		"1 run_started 2026-07-08T12:00:00Z",
		"2 run_artifacts 2026-07-08T12:02:00Z",
	} {
		if !containsLine(lines, want) {
			t.Fatalf("detail view missing %q: %#v", want, lines)
		}
	}

	tasksView, cmd := updateStatusModel(t, afterOpen, keyRunes("2"))
	if cmd != nil {
		t.Fatalf("tasks view cmd = %v, want nil", cmd)
	}
	if tasksView.view != viewTasks {
		t.Fatalf("view = %v, want tasks", tasksView.view)
	}
	if tasksView.runDetails == nil {
		t.Fatal("run details were cleared after switching views")
	}
	if !containsLine(normalizedViewLines(tasksView.View()), "Tasks") {
		t.Fatalf("tasks view missing heading:\n%s", tasksView.View())
	}

	backToDetail, cmd := updateStatusModel(t, tasksView, keyRunes("4"))
	if cmd != nil {
		t.Fatalf("run detail view cmd = %v, want nil", cmd)
	}
	if backToDetail.runDetails == nil {
		t.Fatal("run details were cleared after returning to detail view")
	}
	if !containsLine(normalizedViewLines(backToDetail.View()), "ID: run-old") {
		t.Fatalf("run detail was not preserved:\n%s", backToDetail.View())
	}
}

func TestStatusModelHelpAndFooterRenderingFollowActiveView(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{{
			ID:      "run-one",
			Status:  ledger.StatusCompleted,
			Summary: "done",
		}},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	runsView, cmd := updateStatusModel(t, model, keyRunes("3"))
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}
	runsLines := normalizedViewLines(runsView.View())
	for _, want := range []string{
		"Views: Dashboard | Tasks | [Runs] | Run Detail | Preflight | Help",
		"Keys: j/k Select | enter Open | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail",
		"      5 Preflight | ? Help | a Add Task | R Run Once | n Passes 3 | L Run Loop",
		"      r Refresh | q Quit",
	} {
		if !containsLine(runsLines, want) {
			t.Fatalf("runs footer/header missing %q: %#v", want, runsLines)
		}
	}

	helpView, cmd := updateStatusModel(t, runsView, keyRunes("?"))
	if cmd != nil {
		t.Fatalf("help view cmd = %v, want nil", cmd)
	}
	helpLines := normalizedViewLines(helpView.View())
	for _, want := range []string{
		"Views: Dashboard | Tasks | Runs | Run Detail | Preflight | [Help]",
		"Help",
		"1  Dashboard",
		"n  Cycle loop max passes (current 3)",
		"L  Run loop",
		"enter or o  Open selected run",
		"Keys: esc Back | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight",
		"      ? Help | a Add Task | R Run Once | n Passes 3 | L Run Loop | r Refresh",
		"      q Quit",
	} {
		if !containsLine(helpLines, want) {
			t.Fatalf("help view missing %q: %#v", want, helpLines)
		}
	}

	back, cmd := updateStatusModel(t, helpView, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("escape help cmd = %v, want nil", cmd)
	}
	if back.view != viewRuns {
		t.Fatalf("view after help escape = %v, want runs", back.view)
	}
}

func TestStatusModelWideRenderSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks: []taskmodel.Task{
			{
				ID:           "task-ready",
				Status:       taskmodel.StatusPending,
				Summary:      "write focused TUI polish",
				NextRunnable: true,
			},
			{
				ID:      "task-blocked",
				Status:  taskmodel.StatusBlocked,
				Summary: "blocked task",
			},
		},
		RecentRuns: []ledger.Run{
			{
				ID:                 "run-success",
				Status:             ledger.StatusCompleted,
				VerificationStatus: "passed",
				CommitSHA:          "abc123",
				Summary:            "committed TUI polish",
			},
			{
				ID:                 "run-failed",
				Status:             ledger.StatusFailed,
				VerificationStatus: "failed",
				Summary:            "verification failed",
			},
		},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	lines := normalizedViewLines(model.View())
	want := []string{
		"Revolvr",
		"Views: [Dashboard] | Tasks | Runs | Run Detail | Preflight | Help",
		"State: initialized",
		"",
		"Dashboard",
		"State: initialized",
		"",
		"Tasks",
		"Total: 2",
		"Pending: 1",
		"Blocked: 1",
		"Completed: 0",
		"Runnable: ready to run",
		"Next task: task-ready - write focused TUI polish",
		"",
		"Latest Run",
		"ID: run-success",
		"Status: completed",
		"Summary: committed TUI polish",
		"Verification: passed",
		"Commit: abc123",
		"",
		"Recent Runs",
		"ID  STATUS  VERIFICATION  COMMIT  SUMMARY",
		"> run-success  completed  passed  abc123  committed TUI polish",
		"  run-failed  failed  failed  none  verification failed",
		"",
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | 5 Preflight | ? Help | a Add Task | R Run Once",
		"      n Passes 3 | L Run Loop | r Refresh | q Quit",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("wide view lines = %#v, want %#v", lines, want)
	}
	assertMaxLineWidth(t, lines, 100)
}

func TestStatusModelNarrowRenderSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks: []taskmodel.Task{
			{ID: "task-pending", Status: taskmodel.StatusPending, NextRunnable: true},
			{ID: "task-blocked", Status: taskmodel.StatusBlocked},
		},
		RecentRuns: []ledger.Run{
			{
				ID:                 "019f4415-40b6-7099-9d68-5f87cea67000",
				Status:             ledger.StatusFailed,
				VerificationStatus: "failed",
				Summary:            "verification failed after running a very long command output",
			},
		},
	})
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	lines := normalizedViewLines(model.View())
	want := []string{
		"Revolvr",
		"View: Dashboard",
		"State: initialized",
		"",
		"Dashboard",
		"State: initialized",
		"",
		"Tasks",
		"Total: 2",
		"Pending: 1",
		"Blocked: 1",
		"Completed: 0",
		"Runnable: ready to run",
		"Next task: task-pending",
		"",
		"Latest Run",
		"ID: 019f4415-40b6-7099-9d68-5f87cea67000",
		"Status: failed",
		"Summary: verification failed after",
		"  running a very long command output",
		"Verification: failed",
		"Commit: none",
		"",
		"Recent Runs",
		"ID STATUS SUMMARY",
		"> 019f4415-40b6-7099-9d68-5f87cea67000",
		"  failed verification failed after",
		"  running a very long command output",
		"",
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs",
		"      4 Detail | 5 Preflight | ? Help",
		"      a Add Task | R Run Once",
		"      n Passes 3 | L Run Loop",
		"      r Refresh | q Quit",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("narrow view lines = %#v, want %#v", lines, want)
	}
	assertMaxLineWidth(t, lines, 40)
}

func TestStatusModelResizeUpdatesContentAreaAndWrapsFooter(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})

	resized, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 32, Height: 8})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}
	if resized.viewport.Width != 32 {
		t.Fatalf("viewport width = %d, want 32", resized.viewport.Width)
	}
	if resized.viewport.Height != 1 {
		t.Fatalf("viewport height = %d, want 1", resized.viewport.Height)
	}
	lines := normalizedViewLines(resized.View())
	for _, line := range lines {
		if len(line) > 32 {
			t.Fatalf("line %q has len %d, want <= 32", line, len(line))
		}
	}
	for _, want := range []string{
		"Keys: 1 Dashboard | 2 Tasks",
		"      3 Runs | 4 Detail",
		"      5 Preflight | ? Help",
		"      a Add Task | R Run Once",
		"      n Passes 3 | L Run Loop",
		"      r Refresh | q Quit",
	} {
		if !containsLine(lines, want) {
			t.Fatalf("wrapped footer missing %q: %#v", want, lines)
		}
	}
}

func TestStatusModelQuitActionReturnsQuitCommand(t *testing.T) {
	model := NewStatusModel(app.StatusResult{})

	_, cmd := updateStatusModel(t, model, keyRunes("q"))
	if cmd == nil {
		t.Fatal("quit key returned nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("quit cmd returned %T, want tea.QuitMsg", msg)
	}
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func normalizedViewLines(view string) []string {
	rawLines := strings.Split(view, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = ansiEscapePattern.ReplaceAllString(line, "")
		lines = append(lines, strings.TrimRight(line, " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func assertMaxLineWidth(t *testing.T, lines []string, maxWidth int) {
	t.Helper()
	for _, line := range lines {
		if len([]rune(line)) > maxWidth {
			t.Fatalf("line %q has width %d, want <= %d", line, len([]rune(line)), maxWidth)
		}
	}
}

func updateStatusModel(t *testing.T, model tea.Model, msg tea.Msg) (StatusModel, tea.Cmd) {
	t.Helper()
	updated, cmd := model.Update(msg)
	statusModel, ok := updated.(StatusModel)
	if !ok {
		t.Fatalf("updated model type = %T, want StatusModel", updated)
	}
	return statusModel, cmd
}

func runStatusModelCmd(t *testing.T, model StatusModel, cmd tea.Cmd) (StatusModel, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("cmd is nil")
	}
	return updateStatusModel(t, model, cmd())
}

func drainStatusModelCmds(t *testing.T, model StatusModel, cmd tea.Cmd) StatusModel {
	t.Helper()
	for i := 0; i < 20 && cmd != nil; i++ {
		model, cmd = runStatusModelCmd(t, model, cmd)
	}
	if cmd != nil {
		t.Fatal("command stream did not finish")
	}
	return model
}

func typeIntoStatusModel(t *testing.T, model StatusModel, value string) (StatusModel, tea.Cmd) {
	t.Helper()
	return updateStatusModel(t, model, keyRunes(value))
}

func openTasksView(t *testing.T, model StatusModel) StatusModel {
	t.Helper()
	resized, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}
	tasksView, cmd := updateStatusModel(t, resized, keyRunes("2"))
	if cmd != nil {
		t.Fatalf("tasks view cmd = %v, want nil", cmd)
	}
	return tasksView
}

func openRunsView(t *testing.T, model StatusModel) StatusModel {
	t.Helper()
	resized, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 140, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}
	runsView, cmd := updateStatusModel(t, resized, keyRunes("3"))
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}
	return runsView
}

func runDetailView(t *testing.T, history ledger.RunWithEvents, width int, height int) StatusModel {
	t.Helper()
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		RecentRuns:  []ledger.Run{history.Run},
	})
	model.view = viewRunDetail
	model.previous = viewRuns
	model.runDetails = &history
	model.width = width
	model.height = height
	model.resizeViewport()
	model.updateViewportContent()
	return model
}

func runLoopReadyModel(actions StatusActions) StatusModel {
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, actions)
	model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
	model.width = 160
	model.height = 80
	model.resizeViewport()
	model.updateViewportContent()
	return model
}

func jsonPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func validationDetailHistory(runID string) ledger.RunWithEvents {
	startedAt := time.Date(2026, 7, 8, 13, 30, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	return ledger.RunWithEvents{
		Run: ledger.Run{
			ID:                 runID,
			TaskID:             "task-" + runID,
			Task:               "Validate receipt in TUI",
			Status:             ledger.StatusCompleted,
			Summary:            "completed",
			StartedAt:          startedAt,
			CompletedAt:        &completedAt,
			VerificationStatus: "passed",
			CommitSHA:          "abc123",
		},
		Events: []ledger.Event{
			{
				ID:    1,
				RunID: runID,
				Type:  ledger.EventRunArtifacts,
				Payload: json.RawMessage(`{
					"context_payload_path": ".revolvr/runs/` + runID + `/context.md",
					"context_manifest_path": ".revolvr/runs/` + runID + `/context.json",
					"codex_stdout_jsonl_path": ".revolvr/runs/` + runID + `/codex.jsonl",
					"codex_stderr_path": ".revolvr/runs/` + runID + `/codex.stderr",
					"last_message_path": ".revolvr/runs/` + runID + `/last-message.txt",
					"receipt_path": ".revolvr/receipts/` + runID + `.md"
				}`),
				CreatedAt: completedAt,
			},
		},
	}
}

func sampleTasks() []taskmodel.Task {
	createdPending := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	createdBlocked := createdPending.Add(time.Minute)
	blockedAt := createdPending.Add(2 * time.Minute)
	completedCreated := createdPending.Add(3 * time.Minute)
	completedAt := createdPending.Add(4 * time.Minute)

	return []taskmodel.Task{
		{
			ID:           "task-pending",
			Status:       taskmodel.StatusPending,
			Summary:      "write focused tests",
			Task:         "Add focused task view tests",
			NextRunnable: true,
			CreatedAt:    createdPending,
			UpdatedAt:    createdPending,
		},
		{
			ID:        "task-blocked",
			Status:    taskmodel.StatusBlocked,
			Task:      "blocked task",
			Blocker:   "waiting on access",
			CreatedAt: createdBlocked,
			UpdatedAt: blockedAt,
			BlockedAt: &blockedAt,
		},
		{
			ID:          "task-completed",
			Status:      taskmodel.StatusCompleted,
			Summary:     "finished task",
			Task:        "completed task",
			CreatedAt:   completedCreated,
			UpdatedAt:   completedAt,
			CompletedAt: &completedAt,
		},
	}
}

func keyRunes(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func requireLines(t *testing.T, lines []string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !containsLine(lines, want) {
			t.Fatalf("view missing %q: %#v", want, lines)
		}
	}
}

func requireNoLine(t *testing.T, lines []string, want string) {
	t.Helper()
	if containsLine(lines, want) {
		t.Fatalf("view unexpectedly contained %q: %#v", want, lines)
	}
}
