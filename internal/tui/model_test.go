package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

func TestTranscriptShellProof(t *testing.T) {
	model := newTranscriptShellProofModel(false)
	if model.Init() == nil {
		t.Fatal("initial committed cells returned no append command")
	}
	if got, want := len(model.emitted), len(model.committed); got != want {
		t.Fatalf("emitted identities = %d, want %d", got, want)
	}
	if cmd := model.appendCommitted(); cmd != nil {
		t.Fatal("redraw returned a duplicate append command")
	}

	wantIdle := []string{
		"Ready",
		"Next task: Compact durable agent state",
		"Next: type a task or use /run",
		"›",
		"Enter submit · / commands · ? shortcuts",
	}
	if got := normalizedViewLines(model.View()); !reflect.DeepEqual(got, wantIdle) {
		t.Fatalf("idle managed frame = %#v, want %#v", got, wantIdle)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("composer key command = %v, want nil", cmd)
	}
	model = updated.(*transcriptShellProofModel)
	before := len(model.emitted)
	updated, cmd = model.Update(transcriptShellProofLiveMsg{lines: []string{
		"Running: Compact durable agent state",
		"Mode: loop · pass 1 of 3",
		"Safety: admitted",
		"Current: Running go test ./...",
		"Next: wait, or press c or Esc to cancel",
	}})
	if cmd != nil {
		t.Fatalf("live replacement command = %v, want nil", cmd)
	}
	model = updated.(*transcriptShellProofModel)
	if got := len(model.emitted); got != before {
		t.Fatalf("live replacement changed emitted identities from %d to %d", before, got)
	}
	wantRunning := []string{
		"Running: Compact durable agent state",
		"Mode: loop · pass 1 of 3",
		"Safety: admitted",
		"Current: Running go test ./...",
		"Next: wait, or press c or Esc to cancel",
		"› x",
		"Enter submit · / commands · ? shortcuts",
	}
	lines := normalizedViewLines(model.View())
	if !reflect.DeepEqual(lines, wantRunning) {
		t.Fatalf("running managed frame = %#v, want %#v", lines, wantRunning)
	}
	assertMaxLineWidth(t, lines, 80)

	t.Run("bytes buffer", func(t *testing.T) {
		var output bytes.Buffer
		assertTranscriptShellProofOutput(t, &output, output.String)
	})
	t.Run("strings builder", func(t *testing.T) {
		var output strings.Builder
		assertTranscriptShellProofOutput(t, &output, output.String)
	})
}

func TestStatusModelInstallsTranscriptShell(t *testing.T) {
	status := app.StatusResult{Initialized: true, ProjectRoot: "/work/revolvr"}
	model := NewStatusModelWithActions(status, StatusActions{
		RefreshStatus: func() (app.StatusResult, error) {
			return app.StatusResult{ProjectRoot: "/work/revolvr"}, nil
		},
	})
	if cmd := model.Init(); cmd == nil {
		t.Fatal("session start returned no append command")
	}
	if got := len(model.emitted); got != 1 {
		t.Fatalf("emitted identities = %d, want 1", got)
	}
	if cmd := model.appendCommitted(); cmd != nil {
		t.Fatal("second append command replayed session start")
	}
	requireNoLine(t, normalizedViewLines(model.View()), "Revolvr  Dashboard  initialized")
	requireLines(t, normalizedViewLines(model.View()), "Idle", "No runs recorded.", "›", "Enter submit · / commands · ? shortcuts")

	model, _ = updateStatusModel(t, model, keyRunes("/refresh"))
	model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("refresh command is nil")
	}
	model, cmd = runStatusModelCmd(t, model, cmd)
	if cmd != nil || model.appendCommitted() != nil {
		t.Fatalf("refresh replayed session start: cmd=%v", cmd)
	}
	model, _ = updateStatusModel(t, model, keyRunes("/tasks"))
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || model.appendCommitted() != nil {
		t.Fatalf("navigation replayed session start: cmd=%v", cmd)
	}
	model, cmd = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 24})
	if cmd != nil || model.appendCommitted() != nil {
		t.Fatalf("resize replayed session start: cmd=%v", cmd)
	}

	var output bytes.Buffer
	input, inputWriter := io.Pipe()
	go func() {
		time.Sleep(10 * time.Millisecond)
		_, _ = inputWriter.Write([]byte("/quit\r"))
		_ = inputWriter.Close()
	}()
	if err := RunStatus(context.Background(), status, RunOptions{
		Input:  input,
		Output: &output,
	}); err != nil {
		t.Fatalf("run installed transcript shell: %v", err)
	}
	rendered := output.String()
	for _, line := range []string{"Revolvr", "Project: /work/revolvr", "At start: initialized"} {
		if got := strings.Count(rendered, line); got != 1 {
			t.Fatalf("session line %q count = %d, want 1 in %q", line, got, rendered)
		}
	}
	if session, panel := strings.Index(rendered, "Revolvr"), strings.Index(rendered, "Idle"); session < 0 || panel < 0 || session >= panel {
		t.Fatalf("session start did not precede migration panel in %q", rendered)
	}
	if strings.Contains(rendered, "Revolvr  Dashboard  initialized") {
		t.Fatalf("installed shell retained persistent header in %q", rendered)
	}
}

func TestTranscriptCellKindsRenderDeterministically(t *testing.T) {
	cells := []transcriptCell{
		newSessionTranscriptCell("/home/alex/source/revolvr", true),
		{kind: transcriptCellOperatorAction, identity: "operator-run-1", source: []string{"› /run"}},
		{kind: transcriptCellStatus, identity: "status-ready-1", source: []string{"Ready", "Next: /run"}},
		{kind: transcriptCellProgress, identity: "progress-run-1", source: []string{"Current: Running go test ./..."}},
		{kind: transcriptCellResult, identity: "result-run-1", source: []string{"Completed: Compact durable agent state", "Verification: passed"}},
		{kind: transcriptCellWarning, identity: "warning-run-1", source: []string{"Warning: working tree changed; run stopped before Codex."}},
		{kind: transcriptCellQuestion, identity: "question-task-017", source: []string{"Needs input: task-017", "Question: Choose the verification scope"}},
	}

	for _, cell := range cells {
		t.Run(string(cell.kind), func(t *testing.T) {
			first := cell.render(24)
			second := cell.render(24)
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("render is not deterministic: first=%q second=%q", first, second)
			}
			assertMaxLineWidth(t, first, 24)
			plain := strings.Join(strings.Fields(strings.Join(normalizedViewLines(strings.Join(first, "\n")), " ")), "")
			want := strings.Join(strings.Fields(strings.Join(cell.source, " ")), "")
			if plain != want {
				t.Fatalf("text-only rendering = %q, want %q", plain, want)
			}
		})
	}
}

func TestTranscriptCellUnknownAndMalformedInputRemainVisible(t *testing.T) {
	tests := []transcriptCell{
		{kind: "future-kind", identity: "future-1", source: []string{"opaque future evidence"}},
		{kind: transcriptCellResult, source: []string{"Completed: missing identity"}},
		{kind: transcriptCellWarning, identity: "warning-empty"},
		{kind: transcriptCellSession, identity: "wrong-session", source: []string{"Revolvr"}},
	}
	for i, cell := range tests {
		lines := normalizedViewLines(strings.Join(cell.render(18), "\n"))
		assertMaxLineWidth(t, lines, 18)
		plain := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
		if !strings.HasPrefix(plain, "Warning: unrecognized transcript evidence") {
			t.Fatalf("case %d rendered as %q, want visible warning", i, lines)
		}
		if len(cell.source) > 0 && !strings.Contains(strings.Join(strings.Fields(plain), ""), strings.Join(strings.Fields(cell.source[0]), "")) {
			t.Fatalf("case %d hid source evidence in %q", i, lines)
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "Completed:") {
				t.Fatalf("case %d exposed malformed evidence as success in %q", i, lines)
			}
		}
	}
}

func TestTranscriptCellWrapsByDisplayWidth(t *testing.T) {
	tests := []struct {
		width  int
		source string
	}{
		{width: 8, source: "Status: 世界 ready"},
		{width: 3, source: "世界"},
	}
	for _, test := range tests {
		cell := transcriptCell{
			kind:     transcriptCellStatus,
			identity: "status-wide-1",
			source:   []string{test.source},
		}
		lines := cell.render(test.width)
		assertMaxLineWidth(t, lines, test.width)
		if got := strings.Join(strings.Fields(strings.Join(normalizedViewLines(strings.Join(lines, "\n")), " ")), ""); got != strings.Join(strings.Fields(test.source), "") {
			t.Fatalf("wrapped meaning = %q", got)
		}
	}
}

func TestHistoricalTranscriptProjectsCompletedRun(t *testing.T) {
	status := app.StatusResult{
		Initialized: true,
		ProjectRoot: "/home/alex/source/revolvr",
		RecentRuns: []ledger.Run{{
			ID:                 "run-complete",
			TaskID:             "task-complete",
			Task:               "Compact durable agent state",
			Status:             ledger.StatusCompleted,
			VerificationStatus: "passed",
			CommitSHA:          "ff50d9b5cd07ae91ef7f91ed131dfbb5f5e3e845",
		}},
	}

	first := historicalTranscriptCells(status)
	second := historicalTranscriptCells(status)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("historical projection changed: first=%#v second=%#v", first, second)
	}
	if got, want := len(first), 1; got != want {
		t.Fatalf("historical cells = %d, want %d: %#v", got, want, first)
	}
	wantSource := []string{
		"Completed: Compact durable agent state",
		"Verification: passed",
		"Commit: ff50d9b5cd07",
		"Next: /run to continue",
	}
	if first[0].kind != transcriptCellResult || first[0].identity != "run:run-complete:status:completed" || !reflect.DeepEqual(first[0].source, wantSource) {
		t.Fatalf("completed cell = %#v, want source %#v", first[0], wantSource)
	}

	model := NewStatusModel(status)
	if len(model.committed) != 2 || model.committed[0].identity != "session-start" || model.committed[1].identity != first[0].identity {
		t.Fatalf("startup committed order = %#v", model.committed)
	}
	if model.Init() == nil || len(model.emitted) != len(model.committed) {
		t.Fatalf("startup did not emit each committed identity once: %#v", model.emitted)
	}
	if model.appendCommitted() != nil {
		t.Fatal("startup replayed an already emitted historical cell")
	}
}

func TestHistoricalTranscriptBoundsAndFiltersTimeline(t *testing.T) {
	events := []ledger.Event{
		{ID: 1, RunID: "run-window", Type: ledger.EventRunStarted, Payload: jsonPayload(t, map[string]any{"run_id": "run-window", "task_id": "task-window"})},
		{ID: 2, RunID: "run-window", Type: ledger.EventTaskSelected, Payload: jsonPayload(t, map[string]any{"task_id": "task-window", "summary": "duplicated task body"})},
		{ID: 3, RunID: "run-window", Type: ledger.EventCodexStarted, Payload: jsonPayload(t, map[string]any{"executable": "codex"})},
		{ID: 4, RunID: "run-window", Type: ledger.EventCodexJSONEvent, Payload: jsonPayload(t, map[string]any{"type": "turn.started"})},
	}
	for i := 1; i <= 10; i++ {
		events = append(events, ledger.Event{
			ID:      int64(4 + i),
			RunID:   "run-window",
			Type:    ledger.EventCodexJSONEvent,
			Payload: jsonPayload(t, map[string]any{"message": fmt.Sprintf("operator message %d", i)}),
		})
	}
	status := app.StatusResult{
		Initialized: true,
		RecentRuns: []ledger.Run{{
			ID:     "run-window",
			TaskID: "task-window",
			Task:   "Bound history",
			Status: ledger.StatusCompleted,
		}},
		LatestEvents: events,
	}

	cells := historicalTranscriptCells(status)
	if got, want := len(cells), maxDashboardEvents+2; got != want {
		t.Fatalf("bounded cells = %d, want %d: %#v", got, want, cells)
	}
	if got, want := cells[0].source, []string{"… 3 earlier · 4 Run Detail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window notice = %#v, want %#v", got, want)
	}
	if got := cells[len(cells)-1].identity; got != "run:run-window:status:completed" {
		t.Fatalf("terminal identity = %q", got)
	}
	joined := transcriptCellSource(cells)
	for _, hidden := range []string{"duplicated task body", "turn.started", "operator message 1\n", "operator message 2\n"} {
		if strings.Contains(joined, hidden) {
			t.Fatalf("bounded transcript retained filtered/omitted %q in %q", hidden, joined)
		}
	}
	for i := 3; i <= 10; i++ {
		if want := fmt.Sprintf("operator message %d", i); !strings.Contains(joined, want) {
			t.Fatalf("bounded transcript dropped %q from %q", want, joined)
		}
	}
	if !reflect.DeepEqual(cells, historicalTranscriptCells(status)) {
		t.Fatal("same canonical timeline did not reproduce the same cells")
	}
}

func TestHistoricalTranscriptRefreshAppendsOnlyNewIdentities(t *testing.T) {
	status := app.StatusResult{
		Initialized: true,
		RecentRuns:  []ledger.Run{{ID: "run-refresh", Task: "Refresh history", Status: ledger.StatusCompleted}},
		LatestEvents: []ledger.Event{{
			ID:      1,
			RunID:   "run-refresh",
			Type:    ledger.EventCodexJSONEvent,
			Payload: jsonPayload(t, map[string]any{"message": "first"}),
		}},
	}
	model := NewStatusModel(status)
	if model.Init() == nil {
		t.Fatal("startup returned no committed append")
	}
	before := len(model.emitted)
	status.LatestEvents = append(status.LatestEvents, ledger.Event{
		ID:      2,
		RunID:   "run-refresh",
		Type:    ledger.EventCodexJSONEvent,
		Payload: jsonPayload(t, map[string]any{"message": "second"}),
	})

	model, cmd := updateStatusModel(t, model, refreshStatusMsg{status: status})
	if cmd == nil || len(model.emitted) != before+1 {
		t.Fatalf("refresh append: cmd=%v emitted=%d, want %d", cmd, len(model.emitted), before+1)
	}
	if countTranscriptCells(model.committed, "session-start") != 1 {
		t.Fatalf("refresh session cells = %#v", model.committed)
	}
	for _, cell := range model.committed {
		if _, ok := model.emitted[cell.identity]; !ok {
			t.Fatalf("refresh silently dropped identity %q from emitted set", cell.identity)
		}
	}

	model, cmd = updateStatusModel(t, model, refreshStatusMsg{status: status})
	if cmd != nil || len(model.emitted) != before+1 {
		t.Fatalf("identical refresh replayed history: cmd=%v emitted=%d", cmd, len(model.emitted))
	}
}

func TestLiveTranscriptReconciles(t *testing.T) {
	tests := []struct {
		name         string
		state        runOnceState
		msg          runOnceDoneMsg
		wantIdentity string
		wantLine     string
	}{
		{
			name:  "completed",
			state: runOnceState{Active: true, Started: true, Mode: runModeOnce, Token: 1, Status: "running"},
			msg: runOnceDoneMsg{token: 1, result: runonce.Result{Outcome: runonce.OutcomeCommitted, Run: ledger.Run{
				ID: "run-completed", Task: "Complete the task", Status: ledger.StatusCompleted, VerificationStatus: "passed",
			}}, status: app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{{ID: "run-completed", Task: "Complete the task", Status: ledger.StatusCompleted, VerificationStatus: "passed"}}}},
			wantIdentity: "run:run-completed:status:completed",
			wantLine:     "Completed: Complete the task",
		},
		{
			name:  "failed",
			state: runOnceState{Active: true, Started: true, Mode: runModeOnce, Token: 2, Status: "running"},
			msg: runOnceDoneMsg{token: 2, result: runonce.Result{Outcome: runonce.OutcomeVerificationFailed, Message: "verification failed", Run: ledger.Run{
				ID: "run-failed", Task: "Fail the task", Status: ledger.StatusFailed, Summary: "verification failed",
			}}, status: app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{{ID: "run-failed", Task: "Fail the task", Status: ledger.StatusFailed, Summary: "verification failed"}}}},
			wantIdentity: "run:run-failed:status:failed",
			wantLine:     "Failed: Fail the task",
		},
		{
			name:  "cancelled",
			state: runOnceState{Active: true, Started: true, Mode: runModeOnce, Token: 3, Status: "running"},
			msg: runOnceDoneMsg{token: 3, cancelled: true, result: runonce.Result{Outcome: runonce.OutcomeBlocked, Message: "context canceled", Run: ledger.Run{
				ID: "run-cancelled", Task: "Cancel the task", Status: ledger.StatusFailed,
			}}, status: app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{{ID: "run-cancelled", Task: "Cancel the task", Status: ledger.StatusFailed}}}},
			wantIdentity: "run:run-cancelled:status:failed",
			wantLine:     "Cancelled: Cancel the task",
		},
		{
			name:         "blocked",
			state:        runOnceState{Active: true, Started: true, Mode: runModeTask, Token: 4, RunID: "operation-blocked", Status: "running"},
			msg:          runOnceDoneMsg{token: 4, taskRun: true, taskResult: autonomoustaskrun.Result{OperationID: "operation-blocked", TaskID: "task-017", StopReason: autonomoustaskrun.StopBlocked, StopDetail: "dependency task-016 is pending"}},
			wantIdentity: "task-operation:operation-blocked:stop:blocked",
			wantLine:     "Blocked: task-017",
		},
		{
			name:         "safety stop",
			state:        runOnceState{Active: true, Started: true, Mode: runModeTask, Token: 5, RunID: "operation-safety", Status: "running"},
			msg:          runOnceDoneMsg{token: 5, taskRun: true, taskResult: autonomoustaskrun.Result{OperationID: "operation-safety", TaskID: "task-017", StopReason: autonomoustaskrun.StopSafety, StopDetail: "protected path changed"}},
			wantIdentity: "task-operation:operation-safety:stop:safety_stop",
			wantLine:     "Safety stop: task-017",
		},
		{
			name:         "needs input",
			state:        runOnceState{Active: true, Started: true, Mode: runModeTask, Token: 6, RunID: "operation-input", Status: "running"},
			msg:          runOnceDoneMsg{token: 6, taskRun: true, taskResult: autonomoustaskrun.Result{OperationID: "operation-input", TaskID: "task-017", StopReason: autonomoustaskrun.StopNeedsInput, StopDetail: "Choose the verification scope"}},
			wantIdentity: "task-operation:operation-input:stop:needs_input",
			wantLine:     "Needs input: task-017",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewStatusModel(app.StatusResult{Initialized: true, ProjectRoot: "/work/revolvr"})
			if model.Init() == nil {
				t.Fatal("session append command is nil")
			}
			model.runOnce = test.state
			var cmd tea.Cmd
			if test.name == "completed" {
				model, cmd = updateStatusModel(t, model, refreshStatusMsg{status: test.msg.status})
				if cmd != nil || countTranscriptCells(model.committed, test.wantIdentity) != 1 {
					t.Fatalf("active refresh emitted terminal history: cmd=%v cells=%#v", cmd, model.committed)
				}
				if _, ok := model.emitted[test.wantIdentity]; ok || !model.runOnce.Active {
					t.Fatalf("active refresh replaced live owner: emitted=%#v run=%#v", model.emitted, model.runOnce)
				}
			}
			model, cmd = updateStatusModel(t, model, test.msg)
			if cmd == nil || model.settling == nil || model.settling.cell.identity != test.wantIdentity {
				t.Fatalf("settlement = %#v cmd=%v, want identity %q", model.settling, cmd, test.wantIdentity)
			}
			if got := model.settling.cell.source[0]; got != test.wantLine {
				t.Fatalf("terminal line = %q, want %q", got, test.wantLine)
			}
			if _, ok := model.emitted[test.wantIdentity]; ok || !model.runOnce.Started {
				t.Fatalf("live state cleared before append acknowledgement: emitted=%#v run=%#v", model.emitted, model.runOnce)
			}

			if test.name == "completed" {
				model, cmd = updateStatusModel(t, model, refreshStatusMsg{status: test.msg.status})
				if cmd != nil || countTranscriptCells(model.committed, test.wantIdentity) != 1 {
					t.Fatalf("refresh during settlement duplicated result: cmd=%v cells=%#v", cmd, model.committed)
				}
			}

			model, cmd = updateStatusModel(t, model, transcriptCommittedMsg{token: test.state.Token, identity: test.wantIdentity})
			if cmd != nil || model.settling != nil || model.runOnce.Started || countTranscriptCells(model.committed, test.wantIdentity) != 1 {
				t.Fatalf("acknowledged settlement = %#v cmd=%v committed=%#v", model.runOnce, cmd, model.committed)
			}
			if _, ok := model.emitted[test.wantIdentity]; !ok {
				t.Fatalf("terminal identity %q was not emitted", test.wantIdentity)
			}
			if test.name == "completed" {
				model, cmd = updateStatusModel(t, model, refreshStatusMsg{status: test.msg.status})
				if cmd != nil || countTranscriptCells(model.committed, test.wantIdentity) != 1 {
					t.Fatalf("refresh after settlement replayed result: cmd=%v cells=%#v", cmd, model.committed)
				}
			}
			model, cmd = updateStatusModel(t, model, test.msg)
			if cmd != nil || countTranscriptCells(model.committed, test.wantIdentity) != 1 {
				t.Fatalf("duplicate terminal message replayed result: cmd=%v cells=%#v", cmd, model.committed)
			}
		})
	}
}

func TestLiveTranscriptRejectsStale(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model.runOnce = runOnceState{
		Active:  true,
		Started: true,
		Mode:    runModeTask,
		Token:   8,
		RunID:   "operation-new",
		Status:  "running",
		Logs:    []string{"system: autonomous task run started for task-new"},
	}
	wantLogs := slices.Clone(model.runOnce.Logs)
	staleProgress := taskRunProgressMsg{token: 8, operation: autonomoustaskrun.Operation{OperationID: "operation-old", TaskID: "task-old"}}
	model, cmd := updateStatusModel(t, model, staleProgress)
	if cmd != nil || model.runOnce.RunID != "operation-new" || !reflect.DeepEqual(model.runOnce.Logs, wantLogs) {
		t.Fatalf("stale progress changed live owner: cmd=%v run=%#v", cmd, model.runOnce)
	}

	staleDone := runOnceDoneMsg{token: 8, taskRun: true, taskResult: autonomoustaskrun.Result{
		OperationID: "operation-old", TaskID: "task-old", StopReason: autonomoustaskrun.StopCompleted,
	}}
	model, cmd = updateStatusModel(t, model, staleDone)
	if cmd != nil || model.settling != nil || !model.runOnce.Active || model.runOnce.RunID != "operation-new" {
		t.Fatalf("stale terminal changed live owner: cmd=%v model=%#v", cmd, model)
	}

	model, cmd = updateStatusModel(t, model, runOnceDoneMsg{token: 7, taskRun: true, taskResult: autonomoustaskrun.Result{
		OperationID: "operation-new", TaskID: "task-new", StopReason: autonomoustaskrun.StopCompleted,
	}})
	if cmd != nil || model.settling != nil || !model.runOnce.Active {
		t.Fatalf("stale token changed live owner: cmd=%v model=%#v", cmd, model)
	}
}

func transcriptCellSource(cells []transcriptCell) string {
	var lines []string
	for _, cell := range cells {
		lines = append(lines, cell.source...)
	}
	return strings.Join(lines, "\n")
}

func countTranscriptCells(cells []transcriptCell, identity string) int {
	count := 0
	for _, cell := range cells {
		if cell.identity == identity {
			count++
		}
	}
	return count
}

func TestTranscriptShellResize(t *testing.T) {
	model := newTranscriptShellProofModel(false)
	if model.Init() == nil {
		t.Fatal("initial committed cells returned no append command")
	}
	wantCommitted := make([]transcriptShellProofCell, len(model.committed))
	for i, cell := range model.committed {
		wantCommitted[i] = transcriptShellProofCell{identity: cell.identity, lines: slices.Clone(cell.lines)}
	}

	updated, cmd := model.Update(transcriptShellProofLiveMsg{lines: []string{
		"Running: Compact durable agent state",
		"Mode: loop · pass 1 of 3",
		"Safety: admitted",
		"Current: Running go test ./...",
		"Next: wait, or press c or Esc to cancel",
	}})
	if cmd != nil {
		t.Fatalf("live replacement command = %v, want nil", cmd)
	}
	model = updated.(*transcriptShellProofModel)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Fatalf("composer key command = %v, want nil", cmd)
	}
	model = updated.(*transcriptShellProofModel)
	wantFrameMeaning := strings.Join(strings.Fields(strings.ReplaceAll(strings.Join(slices.Concat(
		model.live,
		[]string{"› x", "Enter submit · / commands · ? shortcuts"},
	), "\n"), "·", "")), " ")

	for _, width := range []int{80, 40, 24, 80} {
		updated, cmd = model.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		if cmd != nil {
			t.Fatalf("resize to %d returned history command %v, want nil", width, cmd)
		}
		model = updated.(*transcriptShellProofModel)
		if !reflect.DeepEqual(model.committed, wantCommitted) {
			t.Fatalf("resize to %d changed committed source to %#v, want %#v", width, model.committed, wantCommitted)
		}
		if got, want := len(model.emitted), len(wantCommitted); got != want {
			t.Fatalf("resize to %d changed emitted identities to %d, want %d", width, got, want)
		}
		if _, ok := model.emitted["session-start"]; !ok {
			t.Fatalf("resize to %d lost session-start identity", width)
		}
		if cmd := model.appendCommitted(); cmd != nil {
			t.Fatalf("resize to %d replayed committed history", width)
		}
		view := model.View()
		assertMaxLineWidth(t, strings.Split(view, "\n"), width)
		lines := normalizedViewLines(view)
		if got := strings.Join(strings.Fields(strings.ReplaceAll(strings.Join(lines, "\n"), "·", "")), " "); got != wantFrameMeaning {
			t.Fatalf("resize to %d changed managed-frame meaning to %q, want %q", width, got, wantFrameMeaning)
		}
		requireLines(t, lines, "› x")
		switch width {
		case 80:
			requireLines(t, lines,
				"Running: Compact durable agent state",
				"Current: Running go test ./...",
				"Enter submit · / commands · ? shortcuts",
			)
		case 40:
			requireLines(t, lines,
				"Next: wait, or press c or Esc to cancel",
				"Enter submit · / commands",
				"? shortcuts",
			)
		}
	}

	updated, cmd = model.Update(transcriptShellProofLiveMsg{lines: []string{
		"Ready",
		"Next task: Compact durable agent state",
		"Next: type a task or use /run",
	}})
	if cmd != nil {
		t.Fatalf("second live replacement command = %v, want nil", cmd)
	}
	model = updated.(*transcriptShellProofModel)
	lines := normalizedViewLines(model.View())
	requireLines(t, lines, "Ready", "› x")
	requireNoLine(t, lines, "Running: Compact durable agent state")
}

func TestTranscriptShellProofInteractive(t *testing.T) {
	if os.Getenv("REVOLVR_TUI_INTERACTIVE_PROOF") != "1" {
		t.Skip("set REVOLVR_TUI_INTERACTIVE_PROOF=1 and press q to run the terminal proof")
	}
	if _, err := tea.NewProgram(
		newTranscriptShellProofModel(false),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	).Run(); err != nil {
		t.Fatalf("run interactive transcript shell proof: %v", err)
	}
}

func TestTranscriptShellSettlementInteractive(t *testing.T) {
	if os.Getenv("REVOLVR_TUI_INTERACTIVE_SETTLEMENT_PROOF") != "1" {
		t.Skip("set REVOLVR_TUI_INTERACTIVE_SETTLEMENT_PROOF=1 to run the terminal settlement proof")
	}
	if _, err := tea.NewProgram(
		newTranscriptShellSettlementProofModel(),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	).Run(); err != nil {
		t.Fatalf("run interactive transcript shell settlement proof: %v", err)
	}
}

func assertTranscriptShellProofOutput(t *testing.T, output io.Writer, contents func() string) {
	t.Helper()
	model := newTranscriptShellProofModel(true)
	final, err := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(output),
		tea.WithoutSignals(),
	).Run()
	if err != nil {
		t.Fatalf("run transcript shell proof: %v", err)
	}
	model = final.(*transcriptShellProofModel)
	if cmd := model.appendCommitted(); cmd != nil {
		t.Fatal("final redraw returned a duplicate append command")
	}

	rendered := contents()
	for _, line := range slices.Concat(model.committed[0].lines, model.committed[1].lines) {
		if got := strings.Count(rendered, line); got != 1 {
			t.Fatalf("committed line %q count = %d, want 1 in %q", line, got, rendered)
		}
	}
	for _, line := range normalizedViewLines(model.View()) {
		if !strings.Contains(rendered, line) {
			t.Fatalf("managed frame line %q missing from %q", line, rendered)
		}
	}
	if session, history := strings.Index(rendered, model.committed[0].lines[0]), strings.Index(rendered, model.committed[1].lines[0]); session < 0 || history < 0 || session >= history {
		t.Fatalf("committed order session=%d history=%d in %q", session, history, rendered)
	}
	if strings.Contains(rendered, "Revolvr  Dashboard  initialized") {
		t.Fatalf("proof output retained dashboard composition: %q", rendered)
	}
}

type transcriptShellProofCell struct {
	identity string
	lines    []string
}

type transcriptShellProofLiveMsg struct {
	lines []string
}

type transcriptShellProofSettledMsg struct {
	token int
	cell  transcriptShellProofCell
}

type transcriptShellProofCommittedMsg struct {
	token    int
	identity string
}

type transcriptShellProofModel struct {
	committed           []transcriptShellProofCell
	emitted             map[string]struct{}
	live                []string
	composer            string
	width               int
	activeToken         int
	settling            *transcriptShellProofCell
	quitAfterSettlement bool
	autoSettlement      *transcriptShellProofSettledMsg
	autoQuit            bool
}

func newTranscriptShellProofModel(autoQuit bool) *transcriptShellProofModel {
	return &transcriptShellProofModel{
		committed: []transcriptShellProofCell{
			{identity: "session-start", lines: []string{
				"Revolvr",
				"Project: /home/alex/source/revolvr",
				"At start: initialized",
			}},
			{identity: "run-ff50d9b5cd07", lines: []string{
				"Completed: Compact durable agent state",
				"Verification: passed",
				"Commit: ff50d9b5cd07",
				"Next: /run to continue",
			}},
		},
		emitted: make(map[string]struct{}),
		live: []string{
			"Ready",
			"Next task: Compact durable agent state",
			"Next: type a task or use /run",
		},
		width:    80,
		autoQuit: autoQuit,
	}
}

func newTranscriptShellSettlementProofModel() *transcriptShellProofModel {
	model := newTranscriptShellProofModel(false)
	model.activeToken = 9
	model.quitAfterSettlement = true
	model.live = []string{"Running: Compact durable agent state"}
	model.autoSettlement = &transcriptShellProofSettledMsg{
		token: 9,
		cell: transcriptShellProofCell{
			identity: "run-cancelled-9",
			lines:    []string{"Cancelled: Compact durable agent state", "Next: /run to retry"},
		},
	}
	return model
}

func (m *transcriptShellProofModel) Init() tea.Cmd {
	cmds := m.pendingCommittedCommands()
	if m.autoSettlement != nil {
		settlement := transcriptShellProofSettledMsg{
			token: m.autoSettlement.token,
			cell: transcriptShellProofCell{
				identity: m.autoSettlement.cell.identity,
				lines:    slices.Clone(m.autoSettlement.cell.lines),
			},
		}
		cmds = append(cmds, func() tea.Msg { return settlement })
	}
	if m.autoQuit {
		cmds = append(cmds, tea.Quit)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

func (m *transcriptShellProofModel) appendCommitted() tea.Cmd {
	cmds := m.pendingCommittedCommands()
	if len(cmds) == 0 {
		return nil
	}
	return tea.Sequence(cmds...)
}

func (m *transcriptShellProofModel) pendingCommittedCommands() []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.committed))
	for _, cell := range m.committed {
		if _, ok := m.emitted[cell.identity]; ok {
			continue
		}
		m.emitted[cell.identity] = struct{}{}
		cmds = append(cmds, tea.Println(strings.Join(cell.lines, "\n")))
	}
	return cmds
}

func (m *transcriptShellProofModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case transcriptShellProofLiveMsg:
		m.live = slices.Clone(msg.lines)
	case transcriptShellProofSettledMsg:
		if msg.token == 0 || msg.token != m.activeToken || m.settling != nil {
			return m, nil
		}
		if _, ok := m.emitted[msg.cell.identity]; ok {
			return m, nil
		}
		cell := transcriptShellProofCell{identity: msg.cell.identity, lines: slices.Clone(msg.cell.lines)}
		m.settling = &cell
		return m, tea.Sequence(
			tea.Println(strings.Join(cell.lines, "\n")),
			func() tea.Msg { return transcriptShellProofCommittedMsg{token: msg.token, identity: cell.identity} },
		)
	case transcriptShellProofCommittedMsg:
		if msg.token != m.activeToken || m.settling == nil || msg.identity != m.settling.identity {
			return m, nil
		}
		m.emitted[msg.identity] = struct{}{}
		m.committed = append(m.committed, *m.settling)
		m.settling = nil
		m.activeToken = 0
		m.live = nil
		if m.quitAfterSettlement {
			m.quitAfterSettlement = false
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 1)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		default:
			m.composer += string(msg.Runes)
		}
	}
	return m, nil
}

func (m *transcriptShellProofModel) View() string {
	lines := wrapPlainLines(m.live, m.width)
	composer := "›"
	if m.composer != "" {
		composer += " " + m.composer
	}
	lines = append(lines, selectedStyle.Render(composer))
	footer := []string{"Enter submit · / commands · ? shortcuts"}
	if m.width <= 40 {
		footer = []string{"Enter submit · / commands", "? shortcuts"}
	}
	for _, line := range wrapPlainLines(footer, m.width) {
		lines = append(lines, mutedStyle.Render(line))
	}
	return strings.Join(lines, "\n")
}

func countProofCells(cells []transcriptShellProofCell, identity string) int {
	count := 0
	for _, cell := range cells {
		if cell.identity == identity {
			count++
		}
	}
	return count
}

func countExact(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func TestStatusModelRendersUninitializedSnapshot(t *testing.T) {
	model := NewStatusModel(app.StatusResult{})

	lines := normalizedViewLines(model.View())
	want := []string{
		"Run `revolvr init` to initialize this repository.",
		"",
		"›",
		"Enter submit · / commands · ? shortcuts",
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
	requireLines(t, lines, "›", "Enter submit · / commands · ? shortcuts")
	requireNoLine(t, lines, "× Run failed · verification failed")
	if got := transcriptCellSource(updated.(StatusModel).committed); !strings.Contains(got, "Failed: run-new") || !strings.Contains(got, "Reason: verification failed") {
		t.Fatalf("committed failure narrative = %q", got)
	}
}

func TestStatusModelTasksViewRendersEmptyTaskState(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	tasksView := openTasksView(t, model)

	lines := normalizedViewLines(tasksView.View())
	requireLines(t, lines,
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
				"Idle",
				"No runs recorded.",
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
				"Idle",
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
				"Idle",
				"No runs recorded.",
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
				"Idle",
				"No runs recorded.",
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
				"Idle",
				"No runs recorded.",
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
	requireLines(t, dashboard, "Idle", "Next task: none")
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

	dashboard := normalizedViewLines(model.View())
	requireLines(t, dashboard, "Idle", "Next task: none")
	requireNoLine(t, dashboard, `Scheduling diagnostic: missing_dependency: task-invalid -> task-missing`)
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
	runsView, cmd := sendShortcut(t, resized, "3")
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
		"Notice: Added and committed task task-new.",
		"> - task-new  pending  TUI add",
		"ID: task-new",
		"Status: pending",
		"Task: --- id: task-new status: pending --- # TUI add Implement add flow",
	)
}

func TestPlainTextComposerOpensReviewedTaskDraft(t *testing.T) {
	addCalled := false
	refreshCalled := false
	addCalls := 0
	refreshCalls := 0
	var addedInput app.AddTaskInput
	actions := StatusActions{
		AddTask: func(input app.AddTaskInput) (taskmodel.Task, error) {
			addCalled = true
			addCalls++
			addedInput = input
			return taskmodel.Task{ID: "task-published"}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) {
			refreshCalled = true
			refreshCalls++
			return app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{{ID: "task-published"}}}, nil
		},
	}
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, actions)

	unchanged, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || unchanged.view != viewDashboard || unchanged.composer != model.composer || addCalled || refreshCalled {
		t.Fatalf("empty submission changed state: cmd=%v view=%v composer=%#v add=%t refresh=%t", cmd, unchanged.view, unchanged.composer, addCalled, refreshCalled)
	}
	unchanged, _ = typeIntoStatusModel(t, unchanged, "   ")
	beforeWhitespace := unchanged
	unchanged, cmd = updateStatusModel(t, unchanged, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || unchanged.view != beforeWhitespace.view || unchanged.composer != beforeWhitespace.composer || unchanged.message != beforeWhitespace.message || addCalled || refreshCalled {
		t.Fatalf("whitespace submission changed state: cmd=%v view=%v composer=%#v message=%q add=%t refresh=%t", cmd, unchanged.view, unchanged.composer, unchanged.message, addCalled, refreshCalled)
	}

	model = NewStatusModelWithActions(app.StatusResult{Initialized: true}, actions)
	model, _ = typeIntoStatusModel(t, model, "  draft task  ")
	review, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || addCalled || refreshCalled {
		t.Fatalf("draft transition: cmd=%v add=%t refresh=%t", cmd, addCalled, refreshCalled)
	}
	if review.view != viewTaskEntry || review.taskEntry.taskText != "  draft task  " || review.composer.Text != "" {
		t.Fatalf("draft state: view=%v entry=%#v composer=%#v", review.view, review.taskEntry, review.composer)
	}
	requireLines(t, normalizedViewLines(review.View()), "Add Task", "> Task:   draft task", "  Summary:")

	cancelled, cmd := updateStatusModel(t, review, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || addCalled || refreshCalled || cancelled.view != viewDashboard || !cancelled.composer.Active || cancelled.composer.Text != "" {
		t.Fatalf("cancelled draft: cmd=%v add=%t refresh=%t view=%v composer=%#v", cmd, addCalled, refreshCalled, cancelled.view, cancelled.composer)
	}

	model = NewStatusModelWithActions(app.StatusResult{Initialized: true}, actions)
	model, _ = typeIntoStatusModel(t, model, "  publish this task  ")
	review, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	submitted, cmd := updateStatusModel(t, review, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || addCalls != 0 || refreshCalls != 0 {
		t.Fatalf("review confirmation: cmd=%v add=%d refresh=%d", cmd, addCalls, refreshCalls)
	}
	_, cmd = runStatusModelCmd(t, submitted, cmd)
	if cmd != nil || addCalls != 1 || refreshCalls != 1 || addedInput.Task != "publish this task" {
		t.Fatalf("published draft: cmd=%v add=%d refresh=%d input=%#v", cmd, addCalls, refreshCalls, addedInput)
	}
}

func TestPlainTextComposerRejectsUnavailableStates(t *testing.T) {
	tests := []struct {
		name             string
		initialized      bool
		addAvailable     bool
		refreshAvailable bool
		run              runOnceState
		want             string
	}{
		{
			name:             "uninitialized",
			addAvailable:     true,
			refreshAvailable: true,
			want:             "Input unavailable: run revolvr init first",
		},
		{
			name:             "active one pass",
			initialized:      true,
			addAvailable:     true,
			refreshAvailable: true,
			run:              runOnceState{Active: true, Started: true, Mode: runModeOnce, Token: 41},
			want:             "Input unavailable: active steering is not supported",
		},
		{
			name:             "active loop",
			initialized:      true,
			addAvailable:     true,
			refreshAvailable: true,
			run:              runOnceState{Active: true, Started: true, Mode: runModeLoop},
			want:             "Input unavailable: queued or deferred input is not supported",
		},
		{
			name:             "active task",
			initialized:      true,
			addAvailable:     true,
			refreshAvailable: true,
			run:              runOnceState{Active: true, Started: true, Mode: runModeTask},
			want:             "Input unavailable: queued or deferred input is not supported",
		},
		{
			name:             "active queue",
			initialized:      true,
			addAvailable:     true,
			refreshAvailable: true,
			run:              runOnceState{Active: true, Started: true, Mode: runModeQueue},
			want:             "Input unavailable: queued or deferred input is not supported",
		},
		{name: "add callback", initialized: true, refreshAvailable: true, want: "Input unavailable: add task is unavailable"},
		{name: "refresh callback", initialized: true, addAvailable: true, want: "Input unavailable: refresh is unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			actions := StatusActions{}
			if tt.addAvailable {
				actions.AddTask = func(app.AddTaskInput) (taskmodel.Task, error) {
					calls++
					return taskmodel.Task{}, nil
				}
			}
			if tt.refreshAvailable {
				actions.RefreshStatus = func() (app.StatusResult, error) {
					calls++
					return app.StatusResult{}, nil
				}
			}
			model := NewStatusModelWithActions(app.StatusResult{Initialized: tt.initialized}, actions)
			model.runOnce = tt.run
			beforeCommitted := slices.Clone(model.committed)
			model, _ = typeIntoStatusModel(t, model, "keep this draft")
			rejected, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || calls != 0 || rejected.view != viewDashboard || rejected.composer.Text != "keep this draft" || rejected.message != tt.want {
				t.Fatalf("rejected state: cmd=%v calls=%d view=%v composer=%#v message=%q", cmd, calls, rejected.view, rejected.composer, rejected.message)
			}
			if !reflect.DeepEqual(rejected.committed, beforeCommitted) || rejected.taskEntry.taskText != "" {
				t.Fatalf("rejection changed durable presentation: committed=%#v entry=%#v", rejected.committed, rejected.taskEntry)
			}
			requireLines(t, normalizedViewLines(rejected.View()), "Notice: "+tt.want, "› keep this draft")

			if tt.name != "active one pass" {
				return
			}
			settling, cmd := updateStatusModel(t, rejected, runOnceDoneMsg{
				token:  41,
				result: runonce.Result{NoTask: true, Outcome: runonce.OutcomeNoTask},
				status: app.StatusResult{Initialized: true},
			})
			if cmd == nil || settling.settling == nil || calls != 0 || settling.composer.Text != "keep this draft" || settling.view != viewDashboard {
				t.Fatalf("settlement consumed input: cmd=%v calls=%d settling=%#v composer=%#v view=%v", cmd, calls, settling.settling, settling.composer, settling.view)
			}
			settled, cmd := updateStatusModel(t, settling, transcriptCommittedMsg{token: 41, identity: settling.settling.cell.identity})
			if cmd != nil || settled.runOnce.Started || calls != 0 || settled.composer.Text != "keep this draft" || settled.view != viewDashboard {
				t.Fatalf("settled input changed: cmd=%v calls=%d run=%#v composer=%#v view=%v", cmd, calls, settled.runOnce, settled.composer, settled.view)
			}
			review, cmd := updateStatusModel(t, settled, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || calls != 0 || review.view != viewTaskEntry || review.taskEntry.taskText != "keep this draft" {
				t.Fatalf("explicit retry: cmd=%v calls=%d view=%v entry=%#v", cmd, calls, review.view, review.taskEntry)
			}
		})
	}
}

func TestPlainTextComposerCannotBypassTypedNeedsInput(t *testing.T) {
	addCalled := false
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		AddTask: func(app.AddTaskInput) (taskmodel.Task, error) {
			addCalled = true
			return taskmodel.Task{}, nil
		},
		RefreshStatus: func() (app.StatusResult, error) { return app.StatusResult{}, nil },
	})
	model.autonomous.Answer = autonomousAnswerState{Active: true, Selected: -1}
	model.composer = commandComposerState{Active: false, Text: "preserved draft"}

	model, cmd := updateStatusModel(t, model, keyRunes("free-form answer"))
	if cmd != nil || addCalled || model.composer.Text != "preserved draft" {
		t.Fatalf("typed input runes changed composer: cmd=%v add=%t composer=%#v", cmd, addCalled, model.composer)
	}
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || addCalled || !model.autonomous.Answer.Active || model.message != "Select an offered option before confirming." {
		t.Fatalf("typed input enter: cmd=%v add=%t answer=%#v message=%q", cmd, addCalled, model.autonomous.Answer, model.message)
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
	model.Init()
	resized, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 40})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	afterKey, cmd := sendShortcut(t, resized, "r")
	if cmd == nil {
		t.Fatal("refresh key returned nil cmd")
	}
	if refreshed {
		t.Fatal("refresh callback ran before command execution")
	}

	afterRefresh, cmd := runStatusModelCmd(t, afterKey, cmd)
	if cmd == nil {
		t.Fatal("refresh message did not append the new run identity")
	}
	afterRefresh, cmd = runStatusModelCmd(t, afterRefresh, cmd)
	if cmd != nil {
		t.Fatalf("historical append command returned %v, want nil", cmd)
	}
	if !refreshed {
		t.Fatal("refresh callback was not called")
	}

	lines := normalizedViewLines(afterRefresh.View())
	for _, want := range []string{"Notice: Refreshed."} {
		if !containsLine(lines, want) {
			t.Fatalf("refreshed view missing %q: %#v", want, lines)
		}
	}
	if got := transcriptCellSource(afterRefresh.committed); !strings.Contains(got, "Failed: run-new") || !strings.Contains(got, "Reason: new summary") {
		t.Fatalf("refreshed committed narrative = %q", got)
	}
	tasksView, cmd := updateStatusModel(t, afterRefresh, keyRunes("2"))
	if cmd != nil {
		t.Fatalf("tasks view cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(tasksView.View()), "Total: 2", "Pending: 1", "Completed: 1")

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

	preflightView, cmd := sendShortcut(t, model, "5")
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

	preflightView, cmd := sendShortcut(t, model, "5")
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

	afterBlocked, cmd := sendShortcut(t, model, "R")
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
	afterFailedPreflight, cmd := sendShortcut(t, afterBlocked, "R")
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
	active, cmd := sendShortcut(t, afterFailedPreflight, "R")
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
		"c Cancel Run | ? Help | q Quit",
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

	afterKey, cmd := sendShortcut(t, model, "R")
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

	afterKey, cmd := sendShortcut(t, model, "R")
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

	afterKey, cmd := sendShortcut(t, model, "R")
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

func TestTranscriptShellSettlement(t *testing.T) {
	t.Run("escape cancels without discarding composer", func(t *testing.T) {
		cancelCalls := 0
		model := NewStatusModel(app.StatusResult{Initialized: true})
		model.composer = commandComposerState{Active: true, Text: "/"}
		model.runOnce = runOnceState{
			Active:  true,
			Started: true,
			Token:   1,
			Cancel:  func() { cancelCalls++ },
		}

		model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if cmd != nil || !model.composer.Active || model.composer.Text != "/" || !model.runOnce.Active || !model.runOnce.CancelRequested || cancelCalls != 1 {
			t.Fatalf("escape state: composer=%#v run=%#v cancel calls=%d cmd=%v", model.composer, model.runOnce, cancelCalls, cmd)
		}
	})

	t.Run("cancel key has one effect", func(t *testing.T) {
		cancelCalls := 0
		model := NewStatusModel(app.StatusResult{Initialized: true})
		model.runOnce = runOnceState{
			Active:  true,
			Started: true,
			Token:   1,
			Cancel:  func() { cancelCalls++ },
			Logs:    []string{"system: run started"},
		}

		model, cmd := updateStatusModel(t, model, keyRunes("c"))
		if cmd != nil || !model.runOnce.CancelRequested || cancelCalls != 1 {
			t.Fatalf("first cancel state=%#v calls=%d cmd=%v", model.runOnce, cancelCalls, cmd)
		}
		model, cmd = updateStatusModel(t, model, keyRunes("c"))
		if cmd != nil || cancelCalls != 1 || countExact(model.runOnce.Logs, "system: cancellation requested") != 1 {
			t.Fatalf("repeated cancel state=%#v calls=%d cmd=%v", model.runOnce, cancelCalls, cmd)
		}
	})

	for _, outcome := range []struct {
		name string
		line string
	}{
		{name: "cancelled", line: "Cancelled: Compact durable agent state"},
		{name: "failed", line: "Failed: Compact durable agent state"},
		{name: "completed", line: "Completed: Compact durable agent state"},
	} {
		t.Run("final cell/"+outcome.name, func(t *testing.T) {
			model := newTranscriptShellProofModel(false)
			if model.Init() == nil {
				t.Fatal("initial committed cells returned no append command")
			}
			model.activeToken = 7
			model.quitAfterSettlement = true
			model.live = []string{"Running: Compact durable agent state"}
			cell := transcriptShellProofCell{
				identity: "run-final-7",
				lines:    []string{outcome.line, "Next: /run to continue"},
			}

			updated, cmd := model.Update(transcriptShellProofSettledMsg{token: 7, cell: cell})
			model = updated.(*transcriptShellProofModel)
			if cmd == nil || model.settling == nil || len(model.live) == 0 {
				t.Fatalf("settlement began without retaining live state: model=%#v cmd=%v", model, cmd)
			}
			if _, ok := model.emitted[cell.identity]; ok {
				t.Fatal("final identity recorded before append acknowledgement")
			}
			updated, duplicateCmd := model.Update(transcriptShellProofSettledMsg{token: 7, cell: cell})
			model = updated.(*transcriptShellProofModel)
			if duplicateCmd != nil {
				t.Fatalf("duplicate settlement command = %v, want nil", duplicateCmd)
			}

			updated, quitCmd := model.Update(transcriptShellProofCommittedMsg{token: 7, identity: cell.identity})
			model = updated.(*transcriptShellProofModel)
			if quitCmd == nil {
				t.Fatal("append acknowledgement did not release delayed quit")
			}
			if _, ok := quitCmd().(tea.QuitMsg); !ok {
				t.Fatal("append acknowledgement command is not tea.Quit")
			}
			if model.activeToken != 0 || model.settling != nil || len(model.live) != 0 {
				t.Fatalf("settled live state = %#v", model)
			}
			if _, ok := model.emitted[cell.identity]; !ok {
				t.Fatal("settled identity was not recorded")
			}
			if countProofCells(model.committed, cell.identity) != 1 || model.appendCommitted() != nil {
				t.Fatalf("final cell was not committed exactly once: %#v", model.committed)
			}

			model.activeToken = 8
			model.live = []string{"Running: newer operation"}
			updated, lateCmd := model.Update(transcriptShellProofSettledMsg{token: 7, cell: cell})
			model = updated.(*transcriptShellProofModel)
			if lateCmd != nil || !reflect.DeepEqual(model.live, []string{"Running: newer operation"}) || model.activeToken != 8 {
				t.Fatalf("late settlement changed newer state: model=%#v cmd=%v", model, lateCmd)
			}
		})
	}

	t.Run("program exits after final append", func(t *testing.T) {
		var output bytes.Buffer
		model := newTranscriptShellSettlementProofModel()
		final, err := tea.NewProgram(
			model,
			tea.WithInput(nil),
			tea.WithOutput(&output),
			tea.WithoutSignals(),
		).Run()
		if err != nil {
			t.Fatalf("run settlement proof: %v", err)
		}
		model = final.(*transcriptShellProofModel)
		if got := strings.Count(output.String(), "Cancelled: Compact durable agent state"); got != 1 {
			t.Fatalf("final output count = %d, want 1 in %q", got, output.String())
		}
		if model.activeToken != 0 || len(model.live) != 0 {
			t.Fatalf("program returned before settlement: %#v", model)
		}
	})

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
		name         string
		msg          tea.KeyMsg
		openComposer bool
	}{
		{name: "q", msg: keyRunes("q")},
		{name: "ctrl-c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}},
		{name: "composer ctrl-c", msg: tea.KeyMsg{Type: tea.KeyCtrlC}, openComposer: true},
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
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
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

				if quitKey.openComposer {
					var composerCmd tea.Cmd
					model, composerCmd = updateStatusModel(t, model, keyRunes("/"))
					if composerCmd != nil || !model.composer.Active {
						t.Fatalf("composer state=%#v cmd=%v", model.composer, composerCmd)
					}
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
				if quitCmd == nil || model.settling == nil || !model.runOnce.QuitAfterSettlement {
					t.Fatalf("matching terminal did not begin final append: cmd=%v model=%#v", quitCmd, model)
				}
				settling := model.settling
				model, quitCmd = updateStatusModel(t, model, transcriptCommittedMsg{token: settling.token, identity: settling.cell.identity})
				if model.runOnce.Active || model.runOnce.Started || model.runOnce.QuitAfterSettlement {
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

	afterFirst, cmd := sendShortcut(t, model, "n")
	if cmd != nil {
		t.Fatalf("first cycle cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(afterFirst.View()),
		"Notice: Loop max passes set to 5.",
		"? Help | R Run | r Refresh | q Quit",
	)
	if afterFirst.selectedRunLoopPasses() != 5 {
		t.Fatalf("loop passes = %d, want 5", afterFirst.selectedRunLoopPasses())
	}

	afterSecond, cmd := updateStatusModel(t, afterFirst, keyRunes("n"))
	if cmd != nil {
		t.Fatalf("second cycle cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(afterSecond.View()),
		"Notice: Loop max passes set to 2.",
	)
	if afterSecond.selectedRunLoopPasses() != 2 {
		t.Fatalf("loop passes = %d, want 2", afterSecond.selectedRunLoopPasses())
	}
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

	afterKey, cmd := sendShortcut(t, model, "L")
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

	afterKey, cmd := sendShortcut(t, model, "L")
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

	afterKey, cmd := sendShortcut(t, model, "L")
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

	afterKey, cmd := sendShortcut(t, model, "L")
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

	afterKey, cmd := sendShortcut(t, model, "L")
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
	m, cmd := sendShortcut(t, m, "U")
	if cmd == nil {
		t.Fatal("start command nil")
	}
	for i := 0; i < 4 && m.runOnce.Active; i++ {
		msg := cmd()
		updated, next := m.Update(msg)
		m = updated.(StatusModel)
		cmd = next
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
	m, cmd := sendShortcut(t, m, "U")
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

	runsView, cmd := sendShortcut(t, model, "3")
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

	runsView, cmd := sendShortcut(t, model, "3")
	if cmd != nil {
		t.Fatalf("runs view cmd = %v, want nil", cmd)
	}
	runsLines := normalizedViewLines(runsView.View())
	for _, want := range []string{
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
	requireLines(t, lines, "›", "Enter submit · / commands · ? shortcuts")
	committed := normalizedViewLines(strings.Join(model.committed[len(model.committed)-1].render(100), "\n"))
	requireLines(t, committed, "Completed: run-success", "Verification: passed", "Commit: abc123", "Next: /run to continue")
	assertMaxLineWidth(t, append(lines, committed...), 100)
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
	requireLines(t, lines, "›", "Enter submit · / commands", "? shortcuts")
	committed := normalizedViewLines(strings.Join(model.committed[len(model.committed)-1].render(40), "\n"))
	if got := strings.Join(committed, "\n"); !strings.Contains(got, "Failed:") || !strings.Contains(got, "Reason: verification failed") || !strings.Contains(got, "Next: /detail to inspect the failure") {
		t.Fatalf("narrow committed narrative = %q", got)
	}
	assertMaxLineWidth(t, append(lines, committed...), 40)
}

func TestComposerFocusAndEscapeStateTable(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})

	wide, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}
	if wide.viewport.Height < 20 {
		t.Fatalf("dashboard viewport height = %d, want at least 20", wide.viewport.Height)
	}

	narrow, cmd := updateStatusModel(t, wide, tea.WindowSizeMsg{Width: 40, Height: 24})
	if cmd != nil {
		t.Fatalf("narrow window size update cmd = %v, want nil", cmd)
	}
	if got := len(narrow.footerLines()); got != 3 {
		t.Fatalf("narrow composer rows = %d, want 3", got)
	}
	lines := normalizedViewLines(narrow.View())
	requireNoLine(t, lines, "Revolvr  Dashboard  initialized")
	requireLines(t, lines, "›", "Enter submit · / commands", "? shortcuts")
	assertMaxLineWidth(t, lines, 40)

	populated, cmd := updateStatusModel(t, narrow, keyRunes("draft task"))
	if cmd != nil || !populated.composer.Active || populated.composer.Text != "draft task" {
		t.Fatalf("populated composer=%#v cmd=%v", populated.composer, cmd)
	}
	preserved, cmd := updateStatusModel(t, populated, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !preserved.composer.Active || preserved.composer.Text != "draft task" || preserved.view != viewDashboard {
		t.Fatalf("plain submit state=%#v view=%v cmd=%v", preserved.composer, preserved.view, cmd)
	}
	preserved, cmd = updateStatusModel(t, preserved, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !preserved.composer.Active || preserved.composer.Text != "draft task" {
		t.Fatalf("populated escape state=%#v cmd=%v", preserved.composer, cmd)
	}

	empty := NewStatusModel(app.StatusResult{Initialized: true})
	empty, cmd = updateStatusModel(t, empty, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || empty.composer.Active || empty.view != viewDashboard {
		t.Fatalf("empty escape state=%#v view=%v cmd=%v", empty.composer, empty.view, cmd)
	}
	requireLines(t, normalizedViewLines(empty.View()), "›", "? Help | R Run | r Refresh | q Quit")

	command, _ := updateStatusModel(t, empty, keyRunes("/"))
	command, _ = updateStatusModel(t, command, keyRunes("tasks"))
	submitted, cmd := updateStatusModel(t, command, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || submitted.composer.Active || submitted.composer.Text != "" || submitted.view != viewTasks {
		t.Fatalf("submitted command state=%#v view=%v cmd=%v", submitted.composer, submitted.view, cmd)
	}

	popup := NewStatusModel(app.StatusResult{Initialized: true})
	popup, _ = updateStatusModel(t, popup, keyRunes("/"))
	popup, cmd = updateStatusModel(t, popup, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || popup.composer.Active || popup.composer.Text != "/" || popup.view != viewHelp {
		t.Fatalf("command help state=%#v view=%v cmd=%v", popup.composer, popup.view, cmd)
	}
	restored, cmd := updateStatusModel(t, popup, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !restored.composer.Active || restored.composer.Text != "/" || restored.view != viewDashboard {
		t.Fatalf("command help return=%#v view=%v cmd=%v", restored.composer, restored.view, cmd)
	}

	nonComposer := NewStatusModel(app.StatusResult{Initialized: true})
	nonComposer.composer.Text = "saved draft"
	nonComposer.openFocusedView(viewDiff)
	nonComposer, cmd = updateStatusModel(t, nonComposer, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !nonComposer.composer.Active || nonComposer.composer.Text != "saved draft" || nonComposer.view != viewDashboard {
		t.Fatalf("focused view return=%#v view=%v cmd=%v", nonComposer.composer, nonComposer.view, cmd)
	}
	nonComposer.autonomous.Answer.Active = true
	nonComposer, cmd = updateStatusModel(t, nonComposer, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || nonComposer.autonomous.Answer.Active || !nonComposer.composer.Active || nonComposer.composer.Text != "saved draft" {
		t.Fatalf("typed question return: answer=%#v composer=%#v cmd=%v", nonComposer.autonomous.Answer, nonComposer.composer, cmd)
	}
}

func TestStatusModelQuitActionReturnsQuitCommand(t *testing.T) {
	model := NewStatusModel(app.StatusResult{})

	model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
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
		if width := ansi.StringWidth(line); width > maxWidth {
			t.Fatalf("line %q has width %d, want <= %d", line, width, maxWidth)
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

func sendShortcut(t *testing.T, model StatusModel, key string) (StatusModel, tea.Cmd) {
	t.Helper()
	if model.composer.Active {
		if model.composer.Text != "" {
			t.Fatalf("shortcut %q cannot take focus from populated composer %#v", key, model.composer)
		}
		var cmd tea.Cmd
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if cmd != nil || model.composer.Active {
			t.Fatalf("shortcut focus state=%#v cmd=%v", model.composer, cmd)
		}
	}
	return updateStatusModel(t, model, keyRunes(key))
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
	resized, _ = updateStatusModel(t, resized, keyRunes("/tasks"))
	tasksView, cmd := updateStatusModel(t, resized, tea.KeyMsg{Type: tea.KeyEnter})
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
	resized, _ = updateStatusModel(t, resized, keyRunes("/runs"))
	runsView, cmd := updateStatusModel(t, resized, tea.KeyMsg{Type: tea.KeyEnter})
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
	model.composer.Active = false
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
	return slices.Contains(lines, want)
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
