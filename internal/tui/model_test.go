package tui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/ledger"
	"revolvr/internal/taskqueue"
)

func TestStatusModelRendersUninitializedSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{})

	lines := normalizedViewLines(model.View())
	want := []string{
		"Revolvr",
		"Views: [Dashboard] | Tasks | Runs | Run Detail | Help",
		"State: not initialized",
		"",
		"Dashboard",
		"State: not initialized",
		"",
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | ? Help | a Add Task",
		"      r Refresh | q Quit",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("view lines = %#v, want %#v", lines, want)
	}
}

func TestStatusModelRendersStaticStatusSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{
		Initialized: true,
		Tasks: []taskqueue.Task{
			{ID: "task-pending", Status: taskqueue.StatusPending},
			{ID: "task-blocked", Status: taskqueue.StatusBlocked},
			{ID: "task-completed", Status: taskqueue.StatusCompleted},
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
		"Views: [Dashboard] | Tasks | Runs | Run Detail | Help",
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
		"Keys: 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | ? Help | a Add Task | r Refresh | q Quit",
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
		"Views: Dashboard | [Tasks] | Runs | Run Detail | Help",
		"Tasks",
		"Total: 0",
		"Pending: 0",
		"Blocked: 0",
		"Completed: 0",
		"Task List",
		"None",
		"Task Detail",
		"No task selected.",
		"Keys: j/k Select | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | ? Help | a Add Task | r Refresh | q Quit",
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
		"> task-pending  pending  write focused tests",
		"  task-blocked  ! blocked  blocked task",
		"  task-completed  completed  finished task",
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
		"> task-blocked  ! blocked  blocked task",
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
		"> task-completed  completed  finished task",
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

func TestStatusModelTaskEntryRejectsEmptyTaskTextInline(t *testing.T) {
	addCalled := false
	refreshCalled := false
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		AddTask: func(app.AddTaskInput) (taskqueue.Task, error) {
			addCalled = true
			return taskqueue.Task{}, nil
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
		AddTask: func(app.AddTaskInput) (taskqueue.Task, error) {
			addCalled = true
			return taskqueue.Task{}, nil
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
		"Views: Dashboard | Tasks | [Runs] | Run Detail | Help",
		"> run-one  completed  none  none  done",
	)
}

func TestStatusModelTaskEntrySubmitAddsRefreshesAndSelectsNewTask(t *testing.T) {
	base := time.Date(2026, 7, 8, 11, 0, 0, 0, time.UTC)
	existing := taskqueue.Task{
		ID:        "task-old",
		Status:    taskqueue.StatusPending,
		Task:      "existing task",
		CreatedAt: base,
		UpdatedAt: base,
	}
	added := taskqueue.Task{
		ID:        "task-new",
		Status:    taskqueue.StatusPending,
		Task:      "Implement add flow",
		Summary:   "TUI add",
		CreatedAt: base.Add(time.Minute),
		UpdatedAt: base.Add(time.Minute),
	}
	var input app.AddTaskInput
	var calls []string
	model := NewStatusModelWithActions(app.StatusResult{
		Initialized: true,
		Tasks:       []taskqueue.Task{existing},
	}, StatusActions{
		AddTask: func(got app.AddTaskInput) (taskqueue.Task, error) {
			calls = append(calls, "add")
			input = got
			return added, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			calls = append(calls, "refresh")
			return app.StatusResult{
				Initialized: true,
				Tasks:       []taskqueue.Task{existing, added},
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
		"> task-new  pending  TUI add",
		"ID: task-new",
		"Status: pending",
		"Task: Implement add flow",
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
				Tasks: []taskqueue.Task{
					{ID: "task-1", Status: taskqueue.StatusPending},
					{ID: "task-2", Status: taskqueue.StatusCompleted},
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
		"Views: Dashboard | Tasks | [Runs] | Run Detail | Help",
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
					PromptPath:  ".revolvr/runs/run-artifacts/prompt.md",
					ReceiptPath: ".revolvr/receipts/run-artifacts.md",
				}),
				CreatedAt: startedAt,
			},
		},
	}
	view := runDetailView(t, history, 140, 60)

	requireLines(t, normalizedViewLines(view.View()),
		"Artifacts",
		"prompt: .revolvr/runs/run-artifacts/prompt.md",
		"codex stdout jsonl: missing",
		"codex stderr: missing",
		"last message: missing",
		"receipt: .revolvr/receipts/run-artifacts.md",
	)
}

func TestStatusModelRunDetailScrollsLongEventOutput(t *testing.T) {
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
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
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
		"Views: Dashboard | Tasks | Runs | [Run Detail] | Help",
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
		"1  run_started  2026-07-08T12:00:00Z",
		"2  run_artifacts  2026-07-08T12:02:00Z",
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

	runsView, cmd := updateStatusModel(t, model, keyRunes("3"))
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}
	runsLines := normalizedViewLines(runsView.View())
	for _, want := range []string{
		"Views: Dashboard | Tasks | [Runs] | Run Detail | Help",
		"Keys: j/k Select | enter Open | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail",
		"      ? Help | a Add Task | r Refresh | q Quit",
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
		"Views: Dashboard | Tasks | Runs | Run Detail | [Help]",
		"Help",
		"1  Dashboard",
		"enter or o  Open selected run",
		"Keys: esc Back | 1 Dashboard | 2 Tasks | 3 Runs | 4 Detail | ? Help | a Add Task",
		"      r Refresh | q Quit",
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
		"      3 Runs | 4 Detail | ? Help",
		"      a Add Task | r Refresh",
		"      q Quit",
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

func normalizedViewLines(view string) []string {
	rawLines := strings.Split(view, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
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

func jsonPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func sampleTasks() []taskqueue.Task {
	createdPending := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	createdBlocked := createdPending.Add(time.Minute)
	blockedAt := createdPending.Add(2 * time.Minute)
	completedCreated := createdPending.Add(3 * time.Minute)
	completedAt := createdPending.Add(4 * time.Minute)

	return []taskqueue.Task{
		{
			ID:        "task-pending",
			Status:    taskqueue.StatusPending,
			Summary:   "write focused tests",
			Task:      "Add focused task view tests",
			CreatedAt: createdPending,
			UpdatedAt: createdPending,
		},
		{
			ID:        "task-blocked",
			Status:    taskqueue.StatusBlocked,
			Task:      "blocked task",
			Blocker:   "waiting on access",
			CreatedAt: createdBlocked,
			UpdatedAt: blockedAt,
			BlockedAt: &blockedAt,
		},
		{
			ID:          "task-completed",
			Status:      taskqueue.StatusCompleted,
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
