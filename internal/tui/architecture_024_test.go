package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/autonomousview"
	"revolvr/internal/ledger"
	"revolvr/internal/taskmodel"
)

func TestTranscriptNavigatesCanonicalDiffAndEvidenceAtNarrowWidth(t *testing.T) {
	started := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	status := app.StatusResult{
		Initialized: true,
		Tasks:       []taskmodel.Task{{ID: "architecture-024-ui", Status: taskmodel.StatusPending, NextRunnable: true}},
		RecentRuns: []ledger.Run{{
			ID:                 "run-ui",
			TaskID:             "architecture-024-ui",
			Status:             ledger.StatusRunning,
			VerificationStatus: "running",
			StartedAt:          started,
		}},
		LatestEvents: []ledger.Event{
			{ID: 1, RunID: "run-ui", Type: ledger.EventRunStarted, Payload: jsonPayload(t, map[string]any{"run_id": "run-ui", "task_id": "architecture-024-ui"}), CreatedAt: started},
			{ID: 2, RunID: "run-ui", Type: ledger.EventChangedFilesCaptured, Payload: jsonPayload(t, map[string]any{"changed_files": []string{"internal/tui/model.go"}}), CreatedAt: started.Add(time.Second)},
			{ID: 3, RunID: "run-ui", Type: ledger.EventRunArtifacts, Payload: jsonPayload(t, map[string]any{"receipt_path": ".revolvr/receipts/run-ui.md"}), CreatedAt: started.Add(2 * time.Second)},
		},
	}
	model := NewStatusModel(status)
	model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 48, Height: 80})
	requireLines(t, normalizedViewLines(model.View()),
		"Transcript",
		"Task: architecture-024-ui | Run: run-ui running",
		"  | Safety: verification=running",
		"2 changed_files_captured 2026-08-27T15:00:01Z",
	)

	model, cmd := updateStatusModel(t, model, keyRunes("d"))
	if cmd != nil || model.view != viewDiff {
		t.Fatalf("diff navigation view=%v cmd=%v", model.view, cmd)
	}
	requireLines(t, normalizedViewLines(model.View()), "Diff", "Changed Files", "internal/tui/model.go", "2 changed_files_captured 2026-08-27T15:00:01Z")

	model, cmd = updateStatusModel(t, model, keyRunes("e"))
	if cmd != nil || model.view != viewEvidence {
		t.Fatalf("evidence navigation view=%v cmd=%v", model.view, cmd)
	}
	lines := normalizedViewLines(model.View())
	requireLines(t, lines, "Evidence", "receipt: .revolvr/receipts/run-ui.md", "Canonical Events")
	assertMaxLineWidth(t, lines, 48)

	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || model.view != viewDashboard {
		t.Fatalf("focus return view=%v cmd=%v", model.view, cmd)
	}
}

func TestApprovalComposerSubmitsTypedNeedsInputResponse(t *testing.T) {
	view := tuiAutonomousView("input-task", "needs_input")
	view.Input = autonomousview.OperatorInput{
		State:                   "waiting",
		QuestionID:              "deployment-mode",
		Revision:                2,
		ContentSHA256:           strings.Repeat("c", 64),
		Question:                "Choose a mode.",
		BlockingReason:          "The task is ambiguous.",
		Options:                 []autonomousview.InputOption{{ID: "change", Meaning: "Change behavior."}, {ID: "keep", Meaning: "Keep behavior."}},
		RecommendationOption:    "keep",
		RecommendationRationale: "Compatibility.",
	}
	called := 0
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		AnswerInput: func(request app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
			called++
			if request.TaskID != "input-task" || request.QuestionID != "deployment-mode" || request.OptionID != "keep" || request.Operator != "tui-operator" {
				t.Fatalf("answer request = %#v", request)
			}
			return app.AnswerAutonomousInputResult{TaskID: request.TaskID, QuestionID: request.QuestionID, Revision: request.Revision, OptionID: request.OptionID, AnswerID: "answer-one", AnswerPersisted: true, Resumed: true}, nil
		},
		LoadAutonomous: func(string) (autonomousview.View, error) {
			resumed := view
			resumed.Input = autonomousview.OperatorInput{State: "none"}
			return resumed, nil
		},
	})
	model.view = viewAutonomous
	model.autonomous.View = &view
	model.autonomous.Selector = "input-task"
	model.autonomous.TaskID = "input-task"
	model.autonomous.Selectors = []app.AutonomousTaskSelector{{Selector: "input-task", TaskID: "input-task", SourceKind: autonomousview.SourceActive}}
	model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 44, Height: 60})

	model, _ = updateStatusModel(t, model, keyRunes("/"))
	model, _ = updateStatusModel(t, model, keyRunes("approval"))
	model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || model.view != viewApproval {
		t.Fatalf("approval command view=%v cmd=%v", model.view, cmd)
	}
	requireLines(t, normalizedViewLines(model.View()), "Approval", "Acceptance", "Recommendation (not selected): keep |", "  Compatibility.")

	model, _ = updateStatusModel(t, model, keyRunes("/"))
	model, _ = updateStatusModel(t, model, keyRunes("answer keep"))
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || called != 0 || !model.autonomous.Answer.Active || !model.autonomous.Answer.Confirming {
		t.Fatalf("typed answer state=%#v calls=%d cmd=%v", model.autonomous.Answer, called, cmd)
	}
	requireLines(t, normalizedViewLines(model.View()), "> Option keep: Keep behavior.", "Answer control: confirmation required: press", "  enter")

	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmed typed answer command is nil")
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if called != 1 || cmd == nil || !model.autonomous.Answer.Result.AnswerPersisted {
		t.Fatalf("answer result=%#v calls=%d reload=%v", model.autonomous.Answer, called, cmd)
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd != nil || model.autonomous.View.Input.State != "none" {
		t.Fatalf("reloaded approval input=%#v cmd=%v", model.autonomous.View.Input, cmd)
	}
	assertMaxLineWidth(t, normalizedViewLines(model.View()), 44)
}
