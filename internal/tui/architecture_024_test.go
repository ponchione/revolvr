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

func TestTranscriptNavigatesCanonicalChangeSummaryAndEvidenceAtNarrowWidth(t *testing.T) {
	started := time.Date(2026, 8, 27, 15, 0, 0, 0, time.UTC)
	status := app.StatusResult{
		Initialized: true,
		Tasks:       []taskmodel.Task{{ID: "next-task", Status: taskmodel.StatusPending, NextRunnable: true}},
		RecentRuns: []ledger.Run{{
			ID:                 "run-ui",
			TaskID:             "architecture-024-ui",
			Status:             ledger.StatusRunning,
			VerificationStatus: "running",
			StartedAt:          started,
		}},
		LatestEvents: []ledger.Event{
			{ID: 1, RunID: "run-ui", Type: ledger.EventRunStarted, Payload: jsonPayload(t, map[string]any{"run_id": "run-ui", "task_id": "architecture-024-ui"}), CreatedAt: started},
			{ID: 2, RunID: "run-ui", Type: ledger.EventTaskSelected, Payload: jsonPayload(t, map[string]any{"task_id": "architecture-024-ui", "summary": strings.Repeat("verbose task instructions ", 20), "workflow": "mixed-pass-v1", "phase": "audit", "profile_name": "auditor"}), CreatedAt: started.Add(time.Second)},
			{ID: 3, RunID: "run-ui", Type: ledger.EventCodexJSONEvent, Payload: jsonPayload(t, map[string]any{"type": "item.started", "item_type": "command_execution"}), CreatedAt: started.Add(2 * time.Second)},
			{ID: 4, RunID: "run-ui", Type: ledger.EventCodexJSONEvent, Payload: jsonPayload(t, map[string]any{"type": "item.completed", "item_type": "command_execution"}), CreatedAt: started.Add(3 * time.Second)},
			{ID: 5, RunID: "run-ui", Type: ledger.EventCodexJSONEvent, Payload: jsonPayload(t, map[string]any{"message": "Checking the operator console."}), CreatedAt: started.Add(4 * time.Second)},
			{ID: 6, RunID: "run-ui", Type: ledger.EventChangedFilesCaptured, Payload: jsonPayload(t, map[string]any{"changed_files": []string{"internal/tui/model.go"}}), CreatedAt: started.Add(5 * time.Second)},
			{ID: 7, RunID: "run-ui", Type: ledger.EventRunArtifacts, Payload: jsonPayload(t, map[string]any{"receipt_path": ".revolvr/receipts/run-ui.md"}), CreatedAt: started.Add(6 * time.Second)},
			{ID: 8, RunID: "run-ui", Type: ledger.EventReceiptParsed, Payload: jsonPayload(t, map[string]any{"receipt_path": ".revolvr/receipts/run-ui.md", "verdict": "completed"}), CreatedAt: started.Add(7 * time.Second)},
			{ID: 9, RunID: "run-ui", Type: ledger.EventCommitStarted, Payload: jsonPayload(t, map[string]any{"changed_files": []string{"internal/tui/model.go"}}), CreatedAt: started.Add(8 * time.Second)},
			{ID: 10, RunID: "run-ui", Type: ledger.EventReceiptWarning, Payload: jsonPayload(t, map[string]any{"warning_type": "changed_files_mismatch", "message": "receipt changed files differ from harness captured changed files", "receipt_path": ".revolvr/receipts/run-ui.md"}), CreatedAt: started.Add(9 * time.Second)},
		},
	}
	model := NewStatusModel(status)
	model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 48, Height: 80})
	requireLines(t, normalizedViewLines(model.View()),
		"• Run running · verification running",
		"› 15:00 codex — Checking the operator console.",
		"✓ 15:00 changes captured — 1 changed file",
		"• 15:00 receipt parsed — completed",
		"• 15:00 commit started — 1 changed file",
		"! 15:00 receipt — changed files differ from",
		"  captured files",
		"• --:-- verification running",
		"› / for commands",
	)
	dashboardLines := normalizedViewLines(model.View())
	for _, noise := range []string{"item.started", "item.completed", "command_execution", "verbose task instructions"} {
		for _, line := range dashboardLines {
			if strings.Contains(line, noise) {
				t.Fatalf("dashboard contains noisy transcript detail %q: %#v", noise, dashboardLines)
			}
		}
	}
	for _, duplicate := range []string{"Dashboard", "Transcript", "Activity", "State: initialized", "Tasks", "Latest Run", "Recent Runs", "Events", "Task architecture-024-ui", "Run run-ui"} {
		requireNoLine(t, dashboardLines, duplicate)
	}

	model, cmd := updateStatusModel(t, model, keyRunes("d"))
	if cmd != nil || model.view != viewDiff {
		t.Fatalf("diff navigation view=%v cmd=%v", model.view, cmd)
	}
	requireLines(t, normalizedViewLines(model.View()), "Change Summary", "Changed Files", "internal/tui/model.go", "6 changed_files_captured 2026-08-27T15:00:05Z")

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

func TestFocusedRunRefreshReloadsCanonicalHistory(t *testing.T) {
	started := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	run := ledger.Run{ID: "run-refresh", TaskID: "run-task", Status: ledger.StatusRunning, StartedAt: started}
	history := ledger.RunWithEvents{Run: run, Events: []ledger.Event{{ID: 1, RunID: run.ID, Type: ledger.EventRunStarted, CreatedAt: started}}}
	refreshed := history
	refreshed.Events = append(refreshed.Events, ledger.Event{ID: 2, RunID: run.ID, Type: ledger.EventChangedFilesCaptured, CreatedAt: started.Add(time.Second)})
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{run}}, StatusActions{
		RefreshStatus: func() (app.StatusResult, error) {
			return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{run}}, nil
		},
		OpenRun: func(runID string) (ledger.RunWithEvents, error) {
			if runID != run.ID {
				t.Fatalf("opened run = %q", runID)
			}
			return refreshed, nil
		},
	})
	model.view = viewRunDetail
	model.runDetails = &history
	model.openFocusedView(viewEvidence)

	model, cmd := updateStatusModel(t, model, keyRunes("r"))
	if cmd == nil {
		t.Fatal("refresh command is nil")
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd == nil {
		t.Fatal("focused run reload command is nil")
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd != nil || model.view != viewEvidence || model.runDetails == nil || len(model.runDetails.Events) != 2 {
		t.Fatalf("focused refresh state: view=%v details=%#v cmd=%v", model.view, model.runDetails, cmd)
	}
	requireLines(t, normalizedViewLines(model.View()), "2  changed_files_captured  2026-08-27T16:00:01Z")
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
	model, _ = updateStatusModel(t, model, keyRunes("answer"))
	model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeySpace})
	model, _ = updateStatusModel(t, model, keyRunes("keep"))
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
