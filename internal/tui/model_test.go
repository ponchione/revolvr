package tui

import (
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
		"State: not initialized",
	}
	if !reflect.DeepEqual(lines[:len(want)], want) {
		t.Fatalf("view lines = %#v, want prefix %#v", lines, want)
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
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	lines := normalizedViewLines(updated.View())
	want := []string{
		"Revolvr",
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
		"> run-new  failed  verification failed",
		"  run-old  completed  committed change",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("view lines = %#v, want %#v", lines, want)
	}
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

	afterKey, cmd := updateStatusModel(t, model, keyRunes("r"))
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
		"> run-new  failed  new summary",
	} {
		if !containsLine(lines, want) {
			t.Fatalf("refreshed view missing %q: %#v", want, lines)
		}
	}
}

func TestStatusModelOpenActionRendersSelectedRunDetails(t *testing.T) {
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

	afterDown, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("selection move cmd = %v, want nil", cmd)
	}
	if !containsLine(normalizedViewLines(afterDown.View()), "> run-old  failed  old summary") {
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
		"Run Details",
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
