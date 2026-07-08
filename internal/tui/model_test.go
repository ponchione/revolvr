package tui

import (
	"reflect"
	"strings"
	"testing"

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
		"run-new  failed  verification failed",
		"run-old  completed  committed change",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("view lines = %#v, want %#v", lines, want)
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
