package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"revolvr/internal/app"
	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomousview"
	"revolvr/internal/taskfile"
)

func TestAutonomousWorkflowLoadsProjectionAndRejectsStaleResponse(t *testing.T) {
	view := tuiAutonomousView("task-one", "ready")
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
			return []app.AutonomousTaskSelector{{Selector: "task-one", TaskID: "task-one", SourceKind: autonomousview.SourceActive, Status: "pending", Title: "One"}}, nil
		},
		LoadAutonomous: func(selector string) (autonomousview.View, error) {
			if selector != "task-one" {
				t.Fatalf("selector = %q", selector)
			}
			return view, nil
		},
	})
	model.width, model.height = 180, 220
	model.resizeViewport()
	model.updateViewportContent()
	model, cmd := updateStatusModel(t, model, keyRunes("6"))
	if cmd == nil || !model.autonomous.LoadingList {
		t.Fatalf("loading=%t cmd=%v", model.autonomous.LoadingList, cmd)
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd == nil || !model.autonomous.LoadingView {
		t.Fatalf("view loading=%t cmd=%v", model.autonomous.LoadingView, cmd)
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd != nil || model.autonomous.View == nil {
		t.Fatalf("view=%#v cmd=%v", model.autonomous.View, cmd)
	}
	requireLines(t, normalizedViewLines(model.View()),
		"Autonomous Workflow",
		"Status: pending | lifecycle: ready | phase: ready",
		"Scheduler readiness: ready",
		"Next supervisor action: undetermined_requires_supervisor",
		"Budget retries: limited limit=3 consumed=1 remaining=2 exhausted=false unit=attempts",
		"Verification: state=available status=passed purpose=final final_gate=passed run=verify-run occurrence=verify-occurrence source=source-one",
	)

	stale := tuiAutonomousView("stale-task", "blocked")
	updated, _ := model.Update(autonomousViewMsg{token: model.autonomous.Request - 1, selector: "task-one", view: stale})
	model = updated.(StatusModel)
	if model.autonomous.View.Identity.TaskID != "task-one" {
		t.Fatalf("stale response replaced view: %#v", model.autonomous.View.Identity)
	}
}

func TestAutonomousWorkflowPreservesTaskIdentityAcrossActiveToArchiveRefresh(t *testing.T) {
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{LoadAutonomous: func(string) (autonomousview.View, error) { return tuiAutonomousView("task-one", "completed"), nil }})
	model.view = viewAutonomous
	model.autonomous = autonomousState{TaskID: "task-one", Selector: "task-one", Request: 7, Selectors: []app.AutonomousTaskSelector{{Selector: "task-one", TaskID: "task-one", SourceKind: autonomousview.SourceActive}}}
	archive := app.AutonomousTaskSelector{Selector: "archive-one", TaskID: "task-one", SourceKind: autonomousview.SourceArchive, Status: "completed", ArchiveID: "archive-one", Disposition: "completed"}
	updated, cmd := model.Update(autonomousSelectorsMsg{token: 7, selectors: []app.AutonomousTaskSelector{archive}})
	model = updated.(StatusModel)
	if model.autonomous.TaskID != "task-one" || model.autonomous.Selector != "archive-one" || cmd == nil {
		t.Fatalf("selection=%#v cmd=%v", model.autonomous, cmd)
	}
}

func TestAutonomousWorkflowRenderingIsPlainNarrowAndScrollableAcrossLifecycles(t *testing.T) {
	for _, lifecycle := range []string{"pending", "ready", "planning", "working", "verifying", "auditing", "correcting", "needs_input", "blocked", "finalizing", "completed", "cancelled", "superseded", "abandoned"} {
		t.Run(lifecycle, func(t *testing.T) {
			view := tuiAutonomousView("task-"+strings.ReplaceAll(lifecycle, "_", "-"), lifecycle)
			model := NewStatusModel(app.StatusResult{Initialized: true})
			model.view = viewAutonomous
			model.autonomous.View = &view
			model.autonomous.Selector = view.Identity.TaskID
			model.autonomous.Selectors = []app.AutonomousTaskSelector{{Selector: view.Identity.TaskID, TaskID: view.Identity.TaskID, SourceKind: autonomousview.SourceActive, Status: "pending"}}
			model.width, model.height = 44, 15
			model.resizeViewport()
			model.updateViewportContent()
			lines := normalizedViewLines(model.View())
			assertMaxLineWidth(t, lines, 44)
			if !containsLine(lines, "Autonomous Workflow") {
				t.Fatalf("missing workflow title: %#v", lines)
			}
			before := model.viewport.YOffset
			model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyPgDown})
			if model.viewport.YOffset <= before {
				t.Fatalf("lifecycle %s did not scroll: before=%d after=%d", lifecycle, before, model.viewport.YOffset)
			}
		})
	}
}

func TestAutonomousAnswerRequiresExplicitChoiceAndDoubleConfirmation(t *testing.T) {
	view := tuiAutonomousView("input-task", "needs_input")
	view.Input = autonomousview.OperatorInput{State: "waiting", QuestionID: "deployment-mode", Revision: 2, ContentSHA256: strings.Repeat("c", 64), Question: "Choose a mode.", BlockingReason: "The task is ambiguous.", Options: []autonomousview.InputOption{{ID: "change", Meaning: "Change behavior."}, {ID: "keep", Meaning: "Keep behavior."}}, RecommendationOption: "keep", RecommendationRationale: "Compatibility."}
	called := 0
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		AnswerInput: func(request app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
			called++
			if request.OptionID != "change" || request.QuestionID != "deployment-mode" || request.Operator != "tui-operator" {
				t.Fatalf("request = %#v", request)
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
	model.updateViewportContent()

	model, cmd := updateStatusModel(t, model, keyRunes("a"))
	if cmd != nil || model.autonomous.Answer.Selected != -1 {
		t.Fatalf("recommendation was preselected: %#v", model.autonomous.Answer)
	}
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || called != 0 {
		t.Fatalf("unselected enter submitted: cmd=%v calls=%d", cmd, called)
	}
	model, _ = updateStatusModel(t, model, keyRunes("j"))
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !model.autonomous.Answer.Confirming || called != 0 {
		t.Fatalf("first confirmation state=%#v cmd=%v calls=%d", model.autonomous.Answer, cmd, called)
	}
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmed answer command is nil")
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if called != 1 || cmd == nil || !model.autonomous.Answer.Result.AnswerPersisted {
		t.Fatalf("calls=%d answer=%#v reload=%v", called, model.autonomous.Answer, cmd)
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd != nil || model.autonomous.View.Input.State != "none" {
		t.Fatalf("reloaded input=%#v cmd=%v", model.autonomous.View.Input, cmd)
	}
}

func TestAutonomousQueueProgressAndCancellationUseOneActiveRun(t *testing.T) {
	refreshCalled := false
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunQueue: func(ctx context.Context, maxTasks, maxCycles int64, progress autonomousqueue.Progress) (autonomousqueue.Result, error) {
			if maxTasks != 100 || maxCycles != 50 {
				t.Fatalf("bounds=%d/%d", maxTasks, maxCycles)
			}
			progress(autonomousqueue.Operation{OperationID: "queue-one", Stage: "selected", Statistics: autonomousqueue.Statistics{Selections: 1}, InFlight: &autonomousqueue.Selection{TaskID: "task-one"}})
			<-ctx.Done()
			return autonomousqueue.Result{OperationID: "queue-one", StopReason: autonomousqueue.StopCancelled, Statistics: autonomousqueue.Statistics{Selections: 1}}, ctx.Err()
		},
		RefreshStatus: func() (app.StatusResult, error) {
			refreshCalled = true
			return app.StatusResult{Initialized: true}, nil
		},
	})
	model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
	model, cmd := updateStatusModel(t, model, keyRunes("Q"))
	if cmd == nil {
		t.Fatal("queue start command is nil")
	}
	model, wait := runStatusModelCmd(t, model, cmd)
	if wait == nil || model.runOnce.RunID != "queue-one" {
		t.Fatalf("progress state=%#v wait=%v", model.runOnce, wait)
	}
	model, duplicate := updateStatusModel(t, model, keyRunes("Q"))
	if duplicate != nil || !strings.Contains(model.message, "active") {
		t.Fatalf("overlap cmd=%v message=%q", duplicate, model.message)
	}
	model, _ = updateStatusModel(t, model, keyRunes("c"))
	if !model.runOnce.CancelRequested {
		t.Fatal("queue cancellation was not requested")
	}
	model = drainStatusModelCmds(t, model, wait)
	if !refreshCalled || model.runOnce.Active || model.runOnce.Outcome != "cancelled" || !strings.Contains(model.runOnce.Err, "canceled") {
		t.Fatalf("final queue state=%#v refresh=%t", model.runOnce, refreshCalled)
	}
}

func tuiAutonomousView(taskID, lifecycle string) autonomousview.View {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	return autonomousview.View{
		SchemaVersion: autonomousview.SchemaVersion,
		Identity:      autonomousview.Identity{SourceKind: autonomousview.SourceActive, TaskID: taskID, Title: "Autonomous task", TaskPath: ".agent/tasks/" + taskID + ".md", TaskSHA256: strings.Repeat("a", 64), TaskByteSize: 120, Workflow: taskfile.WorkflowAutonomousV1, TaskStatus: "pending", Lifecycle: lifecycle, StatePath: ".revolvr/autonomous/tasks/" + taskID + "/state.json", StateSHA256: strings.Repeat("b", 64), StateByteSize: 500, StateSchema: autonomous.ExecutionStateSchemaVersion},
		Summary:       autonomousview.Summary{Phase: lifecycle, Plan: autonomousview.Progress{Completed: 1, Total: 2}, Acceptance: autonomousview.Progress{Completed: 1, Total: 2}, OpenBlockingFindings: 1, TotalAttempts: 1},
		Why:           autonomousview.Why{LatestDecision: "implement", CurrentlyAdmittedAction: "none", SchedulerReadiness: "ready", NextSupervisorAction: "undetermined_requires_supervisor", Reasons: []autonomousview.WhyReason{{Code: "verification_gate", Text: "Final verification remains required."}}},
		Plan:          &autonomousview.Plan{ID: "plan-one", Revision: 2, Steps: []autonomousview.PlanStep{{ID: "step-one", Status: "completed", Description: "Implement the bounded change."}, {ID: "step-two", Status: "pending", Description: "Verify the complete acceptance matrix."}}},
		Acceptance:    []autonomousview.Acceptance{{ID: "criterion-one", Description: "The feature works.", Status: "satisfied"}, {ID: "criterion-two", Description: "Compatibility is retained.", Status: "waived", Rationale: "Operator-approved exception."}},
		Findings:      []autonomousview.Finding{{ID: "finding-one", Significance: "blocking", Summary: "A blocking issue remains.", RequiredCorrection: "Correct the issue.", Status: "open", IntroducedBy: autonomousview.AuditIdentity{Revision: 1, RunID: "audit-one"}, CurrentAudit: autonomousview.AuditIdentity{Revision: 2, RunID: "audit-two"}}},
		Attempts:      autonomousview.Attempts{Total: 1, PerAction: []autonomousview.ActionAttempts{{Action: "implement", Attempts: 1}}, Budgets: []autonomousview.Budget{{Name: "retries", Mode: "limited", Limit: 3, Consumed: 1, Remaining: 2, Unit: "attempts"}, {Name: "elapsed", Mode: "unlimited", Unit: "nanoseconds"}}, Events: []autonomousview.AttemptReference{{Sequence: 1, AttemptID: "attempt-one", Kind: "completed", Action: "implement", Outcome: "passed", RunID: "worker-one", CreatedAt: now}}},
		Input:         autonomousview.OperatorInput{State: "none"},
		Verification:  autonomousview.Verification{State: "available", RunID: "verify-run", OccurrenceID: "verify-occurrence", SourceRevision: "source-one", Status: "passed", Purpose: "final", FinalGate: "passed"},
		Audit:         autonomousview.Audit{State: "available", Revision: 2, RunID: "audit-two", SourceRevision: "source-one", Disposition: "changes_required", FindingCount: 1},
		Workspace:     autonomousview.Workspace{State: "available", WorkspaceID: "workspace-one", Status: "ready", ExecutionRoot: "/tmp/worktree", BranchRef: "refs/heads/revolvr/tasks/one", SourceRevision: "source-one", CheckpointSequence: 2, CheckpointCommit: "abc123"},
		Terminal:      autonomousview.Terminal{State: "active"},
		Provenance:    autonomousview.Provenance{WorkerRunIDs: []string{"worker-one"}, VerificationRunIDs: []string{"verify-run"}, AuditRunIDs: []string{"audit-two"}, References: []autonomousview.Reference{{Kind: "task", Path: ".agent/tasks/" + taskID + ".md", SHA256: strings.Repeat("a", 64), ByteSize: 120, Detail: "Canonical task."}}},
		Diagnostics:   []autonomousview.Diagnostic{{Code: "optional_history_missing", Section: "audit", Detail: "Optional legacy history was unavailable.", Reference: "history"}},
	}
}
