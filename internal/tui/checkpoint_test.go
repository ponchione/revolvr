package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/operatorcheckpoint"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
)

func TestStatusModelRendersAwaitingAndFulfilledOperatorCheckpoints(t *testing.T) {
	awaitingPath := operatorcheckpoint.ExpectedReceiptPath("manual-acceptance")
	fulfilledPath := operatorcheckpoint.ExpectedReceiptPath("license-acceptance")
	status := app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{
		{
			ID: "manual-acceptance", Status: taskmodel.StatusPending, Summary: "Manual acceptance",
			Workflow: taskfile.WorkflowOperatorCheckpointV1, CheckpointState: taskmodel.CheckpointStateAwaiting,
			CheckpointReceiptPath: awaitingPath, ReadinessReason: "awaiting_operator",
		},
		{
			ID: "license-acceptance", Status: taskmodel.StatusCompleted, Summary: "License acceptance",
			Workflow: taskfile.WorkflowOperatorCheckpointV1, CheckpointState: taskmodel.CheckpointStateFulfilled,
			CheckpointReceiptPath: fulfilledPath, CheckpointReceiptSHA: strings.Repeat("a", 64), ReadinessReason: "completed",
		},
	}}
	model := NewStatusModel(status)
	model.width = 240
	requireLines(t, normalizedViewLines(model.View()),
		"Operator checkpoint: manual-acceptance state=awaiting",
		"  receipt="+awaitingPath,
		"Operator checkpoint: license-acceptance state=fulfilled",
		"  receipt="+fulfilledPath,
	)

	tasksView := openTasksView(t, model)
	requireLines(t, normalizedViewLines(tasksView.View()),
		"> - manual-acceptance pending checkpoint=awaiting receipt="+awaitingPath+" Manual",
		"  - license-acceptance completed checkpoint=fulfilled receipt="+fulfilledPath+" License",
		"Workflow: operator-checkpoint-v1",
		"Checkpoint: awaiting",
		"Checkpoint receipt: "+awaitingPath,
		"Checkpoint receipt SHA-256: none",
	)

	fulfilledView, cmd := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("move selection command = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(fulfilledView.View()),
		"Checkpoint: fulfilled",
		"Checkpoint receipt: "+fulfilledPath,
		"Checkpoint receipt SHA-256: "+strings.Repeat("a", 64),
	)
}
