package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"revolvr/internal/app"
	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousview"
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
	requireLines(t, normalizedViewLines(model.View()), "Ready", "Next: type a task or use /run", "›", "Enter submit · / commands · ? shortcuts")

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
	if err := RunStatus(context.Background(), RunOptions{
		Input:           input,
		Output:          &output,
		BootstrapStatus: func() (app.StatusResult, error) { return status, nil },
	}); err != nil {
		t.Fatalf("run installed transcript shell: %v", err)
	}
	rendered := output.String()
	for _, line := range []string{"Revolvr", "Project: /work/revolvr", "At start: initialized"} {
		if got := strings.Count(rendered, line); got != 0 {
			t.Fatalf("startup history line %q count = %d, want 0 in %q", line, got, rendered)
		}
	}
	if !strings.Contains(rendered, "Ready") {
		t.Fatalf("launch output missing ready state in %q", rendered)
	}
	if strings.Contains(rendered, "\x1b[2J") {
		t.Fatalf("installed shell cleared terminal history in %q", rendered)
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
	if got, want := len(cells), maxTranscriptEvents+2; got != want {
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

func TestLiveOperationCellRunningModes(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		state    runOnceState
		wantMode string
	}{
		{
			name:     "single pass",
			width:    80,
			state:    runOnceState{Mode: runModeOnce, Task: "Compact durable agent state", Current: "Running go test ./..."},
			wantMode: "Mode: single pass",
		},
		{
			name:     "bounded loop",
			width:    40,
			state:    runOnceState{Mode: runModeLoop, Task: "Compact durable agent state", Current: "Running go test ./...", MaxPasses: 3, Stats: app.RunLoopStats{MaxPasses: 3}},
			wantMode: "Mode: loop · pass 1 of 3",
		},
		{
			name:  "autonomous task",
			width: 40,
			state: runOnceState{
				Mode: runModeTask, Task: "task-017", Current: "implement · cycle started", MaxPasses: 50, Progress: 2,
			},
			wantMode: "Mode: autonomous · cycle 2 of 50",
		},
		{
			name:  "queue",
			width: 40,
			state: runOnceState{
				Mode: runModeQueue, Task: "autonomous task queue", Current: "selected · task task-017", MaxPasses: 100, Progress: 2,
			},
			wantMode: "Mode: queue · tasks 2 of 100",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewStatusModel(app.StatusResult{Initialized: true})
			model.width = test.width
			model.runOnce = test.state
			lines := model.liveOperationLines()
			assertMaxLineWidth(t, lines, test.width)
			requireLines(t, lines,
				"Running: "+test.state.Task,
				test.wantMode,
				"Safety: admitted",
				"Current: "+test.state.Current,
				"Next: wait, or press c or Esc to cancel",
			)
		})
	}
}

func TestLiveOperationCellReplacesAndBoundsProgress(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model.width = 40
	model.runOnce = runOnceState{
		Active: true, Started: true, Mode: runModeLoop, Token: 7,
		Task: "Compact durable agent state", MaxPasses: 3, Stats: app.RunLoopStats{MaxPasses: 3},
	}
	emitted := len(model.emitted)
	for i := range 20 {
		message := fmt.Sprintf("progress %d with enough detail to wrap across more than two physical rows and be replaced", i)
		model, _ = updateStatusModel(t, model, runOnceProgressMsg{token: 7, event: codexexec.ProgressEvent{Source: "codex", Message: message}})
	}

	lines := model.liveOperationLines()
	if len(lines) != 6 {
		t.Fatalf("live cell rows = %d, want 6: %#v", len(lines), lines)
	}
	if len(model.emitted) != emitted {
		t.Fatalf("progress changed emitted transcript identities: before=%d after=%d", emitted, len(model.emitted))
	}
	assertMaxLineWidth(t, lines, 40)
	requireLines(t, lines, "Safety: admitted", "Next: wait, or press c or Esc to cancel")
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Current:") || !strings.Contains(joined, "progress 19") || strings.Contains(joined, "progress 18") || !strings.Contains(joined, "…") {
		t.Fatalf("bounded current detail = %q", joined)
	}
}

func TestLiveOperationCellCancellationAndLifecycleAreExplicit(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model.width = 40
	model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeOnce, Task: "Compact durable agent state", Current: "completed successfully"}
	requireLines(t, model.liveOperationLines(),
		"Running: Compact durable agent state",
		"Current: completed successfully",
		"Next: wait, or press c or Esc to cancel",
	)

	model.runOnce.CancelRequested = true
	want := []string{
		"Cancelling: Compact durable agent state",
		"Current: waiting for the run to stop",
		"Next: wait for settlement",
	}
	if got := model.liveOperationLines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cancelling cell = %#v, want %#v", got, want)
	}
}

func TestLiveOperationCellShowsElapsedTimeWhenItFits(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model.width = 80
	model.runOnce = runOnceState{Mode: runModeOnce, Task: "task-017", Current: "working", StartedAt: time.Now().Add(-2 * time.Second)}
	if mode := model.liveOperationLines()[1]; !strings.HasPrefix(mode, "Mode: single pass · elapsed ") {
		t.Fatalf("mode line = %q, want elapsed time", mode)
	}
}

func TestLiveOperationCellOwnsCancellationHint(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model.width = 80
	model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeOnce, Task: "task-017", Current: "working"}

	if footer := strings.Join(model.footerLines(), "\n"); strings.Contains(footer, "Cancel Run") {
		t.Fatalf("footer duplicated live cancellation hint: %q", footer)
	}
	requireLines(t, model.liveOperationLines(), "Next: wait, or press c or Esc to cancel")
}

func TestLiveOperationCellTerminalVocabulary(t *testing.T) {
	tests := []struct {
		outcome string
		kind    transcriptCellKind
		want    []string
	}{
		{outcome: "completed", kind: transcriptCellResult, want: []string{"Completed: task-017", "Next: /run to continue"}},
		{outcome: "failed", kind: transcriptCellWarning, want: []string{"Failed: task-017", "Reason: verification failed", "Next: /detail to inspect the failure"}},
		{outcome: "cancelled", kind: transcriptCellWarning, want: []string{"Cancelled: task-017", "Result: no completion was recorded", "Next: /run to retry"}},
		{outcome: "blocked", kind: transcriptCellWarning, want: []string{"Blocked: task-017", "Reason: dependency task-016 is pending", "Next: /workflow to inspect the task"}},
		{outcome: "safety_stop", kind: transcriptCellWarning, want: []string{"Safety stop: task-017", "Reason: protected path changed", "Next: /detail to inspect the evidence"}},
		{outcome: "needs_input", kind: transcriptCellQuestion, want: []string{"Needs input: task-017", "Question: Choose the verification scope", "Next: answer the question to continue"}},
	}
	reasons := map[string]string{
		"failed":      "verification failed",
		"blocked":     "dependency task-016 is pending",
		"safety_stop": "protected path changed",
		"needs_input": "Choose the verification scope",
	}

	for _, test := range tests {
		t.Run(test.outcome, func(t *testing.T) {
			cell := terminalTranscriptCell("terminal-1", "task-017", test.outcome, reasons[test.outcome])
			if cell.kind != test.kind || !reflect.DeepEqual(cell.source, test.want) {
				t.Fatalf("terminal cell = %#v, want kind=%q source=%#v", cell, test.kind, test.want)
			}
		})
	}
}

func TestLiveOperationCellReconcilesTerminalResults(t *testing.T) {
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
		{
			name: "blocked loop",
			state: runOnceState{
				Active: true, Started: true, Mode: runModeLoop, Token: 7, RunID: "run-loop-blocked",
				LastResult: runonce.Result{Outcome: runonce.OutcomeBlocked, Message: "dependency task-016 is pending", Run: ledger.Run{ID: "run-loop-blocked", Task: "task-017", Status: ledger.StatusFailed}},
			},
			msg:          runOnceDoneMsg{token: 7, loop: true, lastRunID: "run-loop-blocked", history: ledger.RunWithEvents{Run: ledger.Run{ID: "run-loop-blocked", Task: "task-017", Status: ledger.StatusFailed}}},
			wantIdentity: "run:run-loop-blocked:status:failed",
			wantLine:     "Blocked: task-017",
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
		"Not initialized",
		"Next: run revolvr init in this repository",
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
		"Keys: j/k Select | enter Workflow | a Add Task | r Refresh | esc Close | q Quit",
	)
}

func TestStatusModelRendersNextRunnableTaskStates(t *testing.T) {
	tests := []struct {
		name         string
		tasks        []taskmodel.Task
		readyWant    []string
		tasksWant    []string
		tasksNotWant []string
	}{
		{
			name: "pending",
			tasks: []taskmodel.Task{
				{ID: "task-blocked", Status: taskmodel.StatusBlocked, Summary: "waiting on access"},
				{ID: "task-ready", Status: taskmodel.StatusPending, Summary: "ship change", NextRunnable: true},
				{ID: "task-later", Status: taskmodel.StatusPending, Task: "later task"},
			},
			readyWant: []string{
				"Ready",
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
			readyWant: []string{
				"Ready",
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
			readyWant: []string{
				"Ready",
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
			readyWant: []string{
				"Ready",
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
			readyWant: []string{
				"Ready",
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

			readyLines := normalizedViewLines(model.View())
			requireLines(t, readyLines, tt.readyWant...)

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

	ready := normalizedViewLines(model.View())
	requireLines(t, ready, "Ready", "Next task: none")
	requireNoLine(t, ready, "Next task: task-dependent")

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

	ready := normalizedViewLines(model.View())
	requireLines(t, ready, "Ready", "Next task: none")
	requireNoLine(t, ready, `Scheduling diagnostic: missing_dependency: task-invalid -> task-missing`)
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
		"Keys: j/k Select | enter Workflow | u Retry | a Add Task | r Refresh | esc Close | q Quit",
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
	if entryView.overlay == nil || entryView.overlay.content != viewTaskEntry {
		t.Fatalf("overlay = %#v, want task entry", entryView.overlay)
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
	runsView := openTasksView(t, resized)

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
	if cancelled.overlay == nil || cancelled.overlay.content != viewTasks {
		t.Fatalf("overlay = %#v, want tasks", cancelled.overlay)
	}
	if cancelled.taskEntry.taskText != "" || cancelled.taskEntry.summary != "" {
		t.Fatalf("task entry state = %+v, want cleared", cancelled.taskEntry)
	}
	requireLines(t, normalizedViewLines(cancelled.View()),
		"Tasks",
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
	if afterAdd.overlay == nil || afterAdd.overlay.content != viewTasks {
		t.Fatalf("overlay = %#v, want tasks", afterAdd.overlay)
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
	if cmd != nil || unchanged.composer != model.composer || addCalled || refreshCalled {
		t.Fatalf("empty submission changed state: cmd=%v composer=%#v add=%t refresh=%t", cmd, unchanged.composer, addCalled, refreshCalled)
	}
	unchanged, _ = typeIntoStatusModel(t, unchanged, "   ")
	beforeWhitespace := unchanged
	unchanged, cmd = updateStatusModel(t, unchanged, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || unchanged.composer != beforeWhitespace.composer || unchanged.message != beforeWhitespace.message || addCalled || refreshCalled {
		t.Fatalf("whitespace submission changed state: cmd=%v composer=%#v message=%q add=%t refresh=%t", cmd, unchanged.composer, unchanged.message, addCalled, refreshCalled)
	}

	model = NewStatusModelWithActions(app.StatusResult{Initialized: true}, actions)
	model, _ = typeIntoStatusModel(t, model, "  draft task  ")
	review, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || addCalled || refreshCalled {
		t.Fatalf("draft transition: cmd=%v add=%t refresh=%t", cmd, addCalled, refreshCalled)
	}
	if review.overlay == nil || review.overlay.content != viewTaskEntry || review.taskEntry.taskText != "  draft task  " || review.composer.Text != "" {
		t.Fatalf("draft state: overlay=%#v entry=%#v composer=%#v", review.overlay, review.taskEntry, review.composer)
	}
	requireLines(t, normalizedViewLines(review.View()), "Add Task", "> Task:   draft task", "  Summary:")

	cancelled, cmd := updateStatusModel(t, review, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || addCalled || refreshCalled || cancelled.overlay != nil || !cancelled.composer.Active || cancelled.composer.Text != "" {
		t.Fatalf("cancelled draft: cmd=%v add=%t refresh=%t overlay=%#v composer=%#v", cmd, addCalled, refreshCalled, cancelled.overlay, cancelled.composer)
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
			if cmd != nil || calls != 0 || rejected.composer.Text != "keep this draft" || rejected.message != tt.want {
				t.Fatalf("rejected state: cmd=%v calls=%d composer=%#v message=%q", cmd, calls, rejected.composer, rejected.message)
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
			if cmd == nil || settling.settling == nil || calls != 0 || settling.composer.Text != "keep this draft" {
				t.Fatalf("settlement consumed input: cmd=%v calls=%d settling=%#v composer=%#v", cmd, calls, settling.settling, settling.composer)
			}
			settled, cmd := updateStatusModel(t, settling, transcriptCommittedMsg{token: 41, identity: settling.settling.cell.identity})
			if cmd != nil || settled.runOnce.Started || calls != 0 || settled.composer.Text != "keep this draft" {
				t.Fatalf("settled input changed: cmd=%v calls=%d run=%#v composer=%#v", cmd, calls, settled.runOnce, settled.composer)
			}
			review, cmd := updateStatusModel(t, settled, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || calls != 0 || review.overlay == nil || review.overlay.content != viewTaskEntry || review.taskEntry.taskText != "keep this draft" {
				t.Fatalf("explicit retry: cmd=%v calls=%d overlay=%#v entry=%#v", cmd, calls, review.overlay, review.taskEntry)
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
	question := tuiAutonomousView("input-task", "needs_input")
	question.Input = autonomousview.OperatorInput{State: "waiting", QuestionID: "question-one", Options: []autonomousview.InputOption{{ID: "choice-one", Meaning: "Use the choice."}}}
	model.autonomous.View = &question
	model.autonomous.TaskID = "input-task"
	model.composer.Text = "preserved draft"
	model.openOverlay(viewNeedsInput, 0)
	model.overlay.parent = viewApproval
	model.autonomous.Answer = autonomousAnswerState{Active: true, Selected: -1}

	model, cmd := updateStatusModel(t, model, keyRunes("free-form answer"))
	if cmd != nil || addCalled || model.overlay.composer.Text != "preserved draft" {
		t.Fatalf("typed input runes changed composer: cmd=%v add=%t composer=%#v", cmd, addCalled, model.overlay.composer)
	}
	model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || addCalled || !model.autonomous.Answer.Active || model.overlay.message != "Select an offered option before confirming." {
		t.Fatalf("typed input enter: cmd=%v add=%t answer=%#v message=%q", cmd, addCalled, model.autonomous.Answer, model.overlay.message)
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

func TestPreflightReadyOverlayShowsChecks(t *testing.T) {
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
	if afterPreflight.overlay == nil || afterPreflight.overlay.content != viewPreflight {
		t.Fatalf("overlay=%#v, want Preflight", afterPreflight.overlay)
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
		"Keys: p Check | R Run Once | n Passes 3 | L Run Loop | r Refresh | esc Close | q Quit",
	)
}

func TestPreflightFailedOverlayShowsChecks(t *testing.T) {
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

func TestPreflightOverlay(t *testing.T) {
	t.Run("command entry preserves source state and narrow geometry", func(t *testing.T) {
		calls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, ProjectRoot: "/work/revolvr"}, StatusActions{
			Preflight: func() (app.PreflightResult, error) {
				calls++
				return app.PreflightResult{Ready: true, Checks: []app.PreflightCheck{{Status: app.PreflightOK, Name: "state", Detail: "ready"}}}, nil
			},
		})
		model.message = "underlying notice"
		model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
		committed := slices.Clone(model.committed)
		model, _ = updateStatusModel(t, model, keyRunes("/preflight"))
		model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil || calls != 0 || model.overlay == nil || model.overlay.content != viewPreflight {
			t.Fatalf("opened state: calls=%d overlay=%#v cmd=%v", calls, model.overlay, cmd)
		}
		if model.composer.Active || !reflect.DeepEqual(model.overlay.composer, commandComposerState{Active: true}) {
			t.Fatalf("composer=%#v saved=%#v, want inactive with saved focus", model.composer, model.overlay.composer)
		}
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || calls != 1 {
			t.Fatalf("preflight result: calls=%d cmd=%v", calls, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Status: ready", "OK state: ready")
		for _, width := range []int{80, 40} {
			model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 24})
			assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
		}

		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, commandComposerState{Active: true}) || model.message != "underlying notice" {
			t.Fatalf("restored state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
		}
		if !reflect.DeepEqual(model.committed, committed) {
			t.Fatal("Preflight overlay changed committed transcript cells")
		}
	})

	t.Run("check refresh and loop-pass actions use existing callbacks", func(t *testing.T) {
		preflightCalls := 0
		refreshCalls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			Preflight: func() (app.PreflightResult, error) {
				preflightCalls++
				status := app.PreflightFail
				if preflightCalls == 1 {
					status = app.PreflightOK
				}
				return app.PreflightResult{
					Ready:  preflightCalls == 1,
					Checks: []app.PreflightCheck{{Status: status, Name: "worktree clean", Detail: "dirty files"}},
				}, nil
			},
			RefreshStatus: func() (app.StatusResult, error) {
				refreshCalls++
				return app.StatusResult{Initialized: true}, nil
			},
		})
		model.message = "underlying notice"
		if model.Init() == nil {
			t.Fatal("session append command is nil")
		}
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("5"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || preflightCalls != 1 {
			t.Fatalf("initial check: calls=%d cmd=%v", preflightCalls, cmd)
		}

		model, cmd = updateStatusModel(t, model, keyRunes("p"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || preflightCalls != 2 || model.preflight.Result.Ready {
			t.Fatalf("repeat check: calls=%d state=%#v cmd=%v", preflightCalls, model.preflight, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Preflight failed.", "FAIL worktree clean: dirty files")

		model, cmd = updateStatusModel(t, model, keyRunes("r"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || refreshCalls != 1 || model.overlay == nil {
			t.Fatalf("refresh: calls=%d overlay=%#v cmd=%v", refreshCalls, model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Refreshed.")

		model, cmd = updateStatusModel(t, model, keyRunes("n"))
		if cmd != nil || model.selectedRunLoopPasses() != 5 || model.message != "underlying notice" {
			t.Fatalf("loop passes=%d source message=%q cmd=%v", model.selectedRunLoopPasses(), model.message, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()),
			"Notice: Loop max passes set to 5.",
			"Keys: p Check | R Run Once | n Passes 5 | L Run Loop | r Refresh | esc Close",
			"      q Quit",
		)
	})

	t.Run("run actions keep admission guards and source notice", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			key     string
			mode    string
			actions StatusActions
		}{
			{
				name: "once",
				key:  "R",
				mode: runModeOnce,
				actions: StatusActions{RunOnce: func(context.Context, app.RunProgress) (runonce.Result, error) {
					return runonce.Result{}, nil
				}},
			},
			{
				name: "loop",
				key:  "L",
				mode: runModeLoop,
				actions: StatusActions{RunLoop: func(context.Context, int, app.RunProgress, app.RunPassFunc) (app.RunLoopResult, error) {
					return app.RunLoopResult{}, nil
				}},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.actions.Preflight = func() (app.PreflightResult, error) {
					return app.PreflightResult{Ready: true}, nil
				}
				model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, tc.actions)
				model.message = "underlying notice"
				model.composer.Active = false
				model, cmd := updateStatusModel(t, model, keyRunes("5"))
				model, cmd = runStatusModelCmd(t, model, cmd)
				model, cmd = updateStatusModel(t, model, keyRunes(tc.key))
				if cmd == nil || !model.runOnce.Active || model.runOnce.Mode != tc.mode || model.overlay == nil || model.message != "underlying notice" {
					t.Fatalf("run state=%#v overlay=%#v source message=%q cmd=%v", model.runOnce, model.overlay, model.message, cmd)
				}
				model.runOnce.Cancel()
			})
		}

		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			Preflight: func() (app.PreflightResult, error) { return app.PreflightResult{Ready: false}, nil },
			RunOnce:   func(context.Context, app.RunProgress) (runonce.Result, error) { return runonce.Result{}, nil },
		})
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("5"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		model, cmd = updateStatusModel(t, model, keyRunes("R"))
		if cmd != nil || model.runOnce.Active {
			t.Fatalf("blocked run state=%#v cmd=%v", model.runOnce, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Run blocked: preflight is not ready.")
	})

	t.Run("errors and active-operation refusals stay textual", func(t *testing.T) {
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			Preflight: func() (app.PreflightResult, error) { return app.PreflightResult{}, errors.New("inspection failed") },
		})
		model.message = "underlying notice"
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("5"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || model.overlay == nil || model.message != "underlying notice" {
			t.Fatalf("error state: overlay=%#v source message=%q cmd=%v", model.overlay, model.message, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Preflight error.", "Status: error", "Error: inspection failed")

		unavailable := NewStatusModel(app.StatusResult{Initialized: true})
		unavailable.composer.Active = false
		unavailable, cmd = updateStatusModel(t, unavailable, keyRunes("5"))
		if cmd != nil || unavailable.overlay == nil {
			t.Fatalf("unavailable state: overlay=%#v cmd=%v", unavailable.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(unavailable.View()), "Notice: Preflight is unavailable.", "Status: not run")

		activeCalls := 0
		active := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			Preflight:     func() (app.PreflightResult, error) { activeCalls++; return app.PreflightResult{}, nil },
			RefreshStatus: func() (app.StatusResult, error) { activeCalls++; return app.StatusResult{}, nil },
		})
		active.composer.Active = false
		active.runOnce = runOnceState{Active: true, Started: true, Mode: runModeOnce, Token: 7, RunID: "run-live"}
		active, cmd = updateStatusModel(t, active, keyRunes("5"))
		if cmd != nil || active.overlay == nil || active.runOnce.RunID != "run-live" || activeCalls != 0 {
			t.Fatalf("active open: run=%#v overlay=%#v calls=%d cmd=%v", active.runOnce, active.overlay, activeCalls, cmd)
		}
		requireLines(t, normalizedViewLines(active.View()), "Notice: Run is active; cancel or wait before checking preflight.")
		for _, key := range []string{"p", "r", "R", "n", "L"} {
			active, cmd = updateStatusModel(t, active, keyRunes(key))
			if cmd != nil || activeCalls != 0 || !active.runOnce.Active || active.loopPasses != defaultRunLoopPasses {
				t.Fatalf("active key %q: calls=%d run=%#v passes=%d cmd=%v", key, activeCalls, active.runOnce, active.loopPasses, cmd)
			}
		}
	})

	t.Run("late result cannot replace a newer overlay owner", func(t *testing.T) {
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			Preflight: func() (app.PreflightResult, error) { return app.PreflightResult{Ready: true}, nil },
		})
		model.composer.Active = false
		model, stale := updateStatusModel(t, model, keyRunes("5"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		model, current := updateStatusModel(t, model, keyRunes("5"))
		owner := model.overlay.owner
		model, cmd := runStatusModelCmd(t, model, stale)
		if cmd != nil || model.overlay == nil || model.overlay.owner != owner || model.preflight.Checked {
			t.Fatalf("stale result changed owner: overlay=%#v preflight=%#v cmd=%v", model.overlay, model.preflight, cmd)
		}
		model, cmd = runStatusModelCmd(t, model, current)
		if cmd != nil || !model.preflight.Checked || !model.preflight.Result.Ready {
			t.Fatalf("current result not applied: preflight=%#v cmd=%v", model.preflight, cmd)
		}
	})
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
	if afterAdd.overlay != nil && afterAdd.overlay.content == viewTaskEntry {
		t.Fatal("add task entry opened while run was active")
	}
	requireLines(t, normalizedViewLines(afterAdd.View()),
		"Notice: Run is active; cancel or wait before starting another action.",
		"Running: run",
		"Safety: admitted",
		"Current: starting the run",
		"Next: wait, or press c or Esc to cancel",
		"Enter submit · / commands · ? shortcuts",
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
		"Completed: Run from TUI",
		"Commit: abc123",
		"Next: /run to continue",
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
		"Failed: Run from TUI and fail",
		"Reason: verification failed",
		"Next: /detail to inspect the failure",
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
		"Cancelling: run",
		"Current: waiting for the run to stop",
		"Next: wait for settlement",
	)

	afterCancel := drainStatusModelCmds(t, cancelled, waitCmd)
	requireLines(t, normalizedViewLines(afterCancel.View()),
		"Cancelled: Cancel a TUI run",
		"Result: no completion was recorded",
		"Next: /run to retry",
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
		"Enter submit · / commands · ? shortcuts",
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
		"Completed: Loop task 3",
		"Commit: abc3",
		"Next: /run to continue",
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
		"No pending runnable tasks.",
		"Next: add a task or wait for dependencies",
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
		"Failed: Fail task 2",
		"Reason: run failed",
		"Next: /detail to inspect the failure",
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
		"Blocked: Blocked task",
		"Reason: blocked by preflight",
		"Next: /workflow to inspect the task",
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
		"Cancelling: run",
		"Current: waiting for the run to stop",
		"Next: wait for settlement",
	)

	afterCancel := drainStatusModelCmds(t, cancelView, waitCmd)
	if !cancelled {
		t.Fatal("loop context was not cancelled")
	}
	if !refreshCalled {
		t.Fatal("refresh callback was not called after cancellation")
	}
	requireLines(t, normalizedViewLines(afterCancel.View()),
		"Cancelled: run",
		"Result: no completion was recorded",
		"Next: /run to retry",
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
	if afterOpen.overlay == nil || afterOpen.overlay.content != viewRunDetail {
		t.Fatalf("overlay=%#v, want run detail", afterOpen.overlay)
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
	if afterValidation.overlay == nil || afterValidation.overlay.content != viewRunDetail {
		t.Fatalf("overlay = %#v, want run detail", afterValidation.overlay)
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

	runsView := model
	runsView.openRunsOverlay()

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

	tasksView, _ := updateStatusModel(t, afterOpen, tea.KeyMsg{Type: tea.KeyEsc})
	tasksView, _ = updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyEsc})
	tasksView, cmd = sendShortcut(t, tasksView, "2")
	if cmd != nil {
		t.Fatalf("tasks overlay cmd = %v, want nil", cmd)
	}
	if tasksView.overlay == nil || tasksView.overlay.content != viewTasks {
		t.Fatalf("overlay = %#v, want Tasks", tasksView.overlay)
	}
	if tasksView.runDetails == nil {
		t.Fatal("run details were cleared after switching views")
	}
	if !containsLine(normalizedViewLines(tasksView.View()), "Tasks") {
		t.Fatalf("tasks view missing heading:\n%s", tasksView.View())
	}

	backToDetail, _ := updateStatusModel(t, tasksView, tea.KeyMsg{Type: tea.KeyEsc})
	backToDetail, cmd = sendShortcut(t, backToDetail, "4")
	if cmd != nil {
		t.Fatalf("close tasks overlay cmd = %v, want nil", cmd)
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
	model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 60})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}

	runsView := model
	runsView.composer.Active = false
	runsView, _ = updateStatusModel(t, runsView, keyRunes("3"))
	runsLines := normalizedViewLines(runsView.View())
	for _, want := range []string{
		"Keys: j/k Select | enter Open | r Refresh | esc Close | q Quit",
	} {
		if !containsLine(runsLines, want) {
			t.Fatalf("runs footer/header missing %q: %#v", want, runsLines)
		}
	}

	helpView, _ := updateStatusModel(t, runsView, tea.KeyMsg{Type: tea.KeyEsc})
	helpView, cmd = updateStatusModel(t, helpView, keyRunes("?"))
	if cmd != nil {
		t.Fatalf("help view cmd = %v, want nil", cmd)
	}
	if helpView.overlay == nil || helpView.overlay.content != viewHelp {
		t.Fatalf("help overlay=%#v, want Help", helpView.overlay)
	}
	helpLines := normalizedViewLines(helpView.View())
	for _, want := range []string{
		"Help",
		"Committed results stay in terminal scrollback",
		"Current work replaces one live cell",
		"Active rows use Running, Safety, Current, and Next labels",
		"Results say Completed, Failed, Cancelled, Blocked, Safety stop, or Needs input",
		"? or bare / or /help or /commands  Help",
		"2 or /tasks  Tasks",
		"4 or /detail  Run Detail",
		"a or /answer <option-id>  Answer typed needs-input",
		"Esc or Backspace returns; Run Detail returns to Runs",
		"Typed input returns to Workflow or Approval",
		"n  Cycle loop max passes (current 3)",
		"L or /loop  Run bounded loop",
		"q or /quit or Ctrl-C  Quit after active work settles",
		"Tested: XTerm 390 and tmux 3.4; other terminals are unclaimed",
		"40x24 is the supported minimum; below 40 columns is best effort",
		"Ctrl-Z and external suspend/continue are unsupported",
		"↑/↓ scroll · Esc close",
	} {
		if !containsLine(helpLines, want) {
			t.Fatalf("help view missing %q: %#v", want, helpLines)
		}
	}

	back, cmd := updateStatusModel(t, helpView, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("escape help cmd = %v, want nil", cmd)
	}
	if back.overlay != nil {
		t.Fatalf("state after help escape: overlay=%#v, want none", back.overlay)
	}
}

func TestOverlayShell(t *testing.T) {
	t.Run("retained entries and exact return", func(t *testing.T) {
		entries := []struct {
			name string
			open func(t *testing.T, model StatusModel) StatusModel
			want commandComposerState
		}{
			{
				name: "question mark",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, cmd := updateStatusModel(t, model, keyRunes("?"))
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
			{
				name: "bare slash",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true, Text: "/", DiscoveryOpen: true},
			},
			{
				name: "help command",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/help"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
			{
				name: "commands alias",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/commands"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
		}

		for _, entry := range entries {
			t.Run(entry.name, func(t *testing.T) {
				model := NewStatusModel(app.StatusResult{Initialized: true, ProjectRoot: "/work/revolvr"})
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 80})
				if model.Init() == nil {
					t.Fatal("session append command is nil")
				}
				committed := slices.Clone(model.committed)
				emitted := len(model.emitted)

				model = entry.open(t, model)
				if model.overlay == nil || model.overlay.content != viewHelp {
					t.Fatalf("overlay = %#v, want Help", model.overlay)
				}
				if model.composer.Active || !reflect.DeepEqual(model.overlay.composer, entry.want) {
					t.Fatalf("composer=%#v saved=%#v, want unfocused with %#v", model.composer, model.overlay.composer, entry.want)
				}
				if !reflect.DeepEqual(model.committed, committed) || len(model.emitted) != emitted {
					t.Fatalf("opening overlay changed transcript: committed=%#v emitted=%#v", model.committed, model.emitted)
				}
				requireLines(t, normalizedViewLines(model.View()), "Help", "/refresh /cancel /validate /help /commands /quit")

				focused, cmd := updateStatusModel(t, model, keyRunes("1"))
				if cmd != nil || focused.overlay == nil {
					t.Fatalf("overlay did not retain focus: overlay=%#v cmd=%v", focused.overlay, cmd)
				}
				restored, cmd := updateStatusModel(t, focused, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || restored.overlay != nil || !reflect.DeepEqual(restored.composer, entry.want) {
					t.Fatalf("restored state: overlay=%#v composer=%#v cmd=%v", restored.overlay, restored.composer, cmd)
				}
				if !reflect.DeepEqual(restored.committed, committed) || len(restored.emitted) != emitted {
					t.Fatalf("closing overlay changed transcript: committed=%#v emitted=%#v", restored.committed, restored.emitted)
				}
			})
		}
	})

	t.Run("scroll resize and source state stay bounded", func(t *testing.T) {
		status := app.StatusResult{Initialized: true}
		for i := range 30 {
			status.RecentRuns = append(status.RecentRuns, ledger.Run{ID: fmt.Sprintf("run-%02d", i), Status: ledger.StatusCompleted})
		}
		model := NewStatusModel(status)
		model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = updateStatusModel(t, model, keyRunes("?"))
		for range 20 {
			model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyPgDown})
		}
		if model.overlay == nil || !model.overlay.viewport.AtBottom() || model.overlay.selected != 0 {
			t.Fatalf("overlay scroll state = %#v, want bounded at bottom with no Help selection", model.overlay)
		}

		for _, width := range []int{80, 40} {
			model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 24})
			if model.overlay.viewport.Width != width || model.overlay.viewport.Height < 1 || model.overlay.viewport.Height >= 24 || model.overlay.viewport.YOffset < 0 {
				t.Fatalf("width %d overlay viewport = %#v", width, model.overlay.viewport)
			}
			assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
		}

		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyHome})
		if !model.overlay.viewport.AtTop() {
			t.Fatalf("overlay offset = %d, want top", model.overlay.viewport.YOffset)
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay != nil || !model.composer.Active {
			t.Fatalf("source return: overlay=%#v composer=%#v", model.overlay, model.composer)
		}
	})

	t.Run("active settlement stays behind overlay", func(t *testing.T) {
		model := NewStatusModel(app.StatusResult{Initialized: true})
		if model.Init() == nil {
			t.Fatal("session append command is nil")
		}
		model.runOnce = runOnceState{
			Active: true, Started: true, Mode: runModeTask, Token: 1,
			RunID: "operation-overlay", Task: "task-overlay", Status: "running",
		}
		model.updateViewportContent()
		model, _ = updateStatusModel(t, model, keyRunes("?"))
		if model.overlay == nil || model.runOnce.RunID != "operation-overlay" {
			t.Fatalf("opening Help changed live owner: overlay=%#v run=%#v", model.overlay, model.runOnce)
		}

		model, cmd := updateStatusModel(t, model, runOnceDoneMsg{
			token:   1,
			taskRun: true,
			taskResult: autonomoustaskrun.Result{
				OperationID: "operation-overlay",
				TaskID:      "task-overlay",
				StopReason:  autonomoustaskrun.StopCompleted,
			},
		})
		if cmd == nil || model.overlay == nil || model.settling == nil {
			t.Fatalf("settlement behind overlay: overlay=%#v settling=%#v cmd=%v", model.overlay, model.settling, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Help")
		requireNoLine(t, normalizedViewLines(model.View()), "Completed: task-overlay")

		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if cmd != nil || model.overlay != nil || model.settling == nil {
			t.Fatalf("dismiss during settlement: overlay=%#v settling=%#v cmd=%v", model.overlay, model.settling, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Completed: task-overlay")

		identity := model.settling.cell.identity
		model, cmd = updateStatusModel(t, model, transcriptCommittedMsg{token: 1, identity: identity})
		if cmd != nil || model.settling != nil || model.runOnce.Started || countTranscriptCells(model.committed, identity) != 1 {
			t.Fatalf("acknowledged settlement: run=%#v settling=%#v committed=%#v cmd=%v", model.runOnce, model.settling, model.committed, cmd)
		}
	})
}

func TestWorkflowOverlay(t *testing.T) {
	selector := app.AutonomousTaskSelector{Selector: "task-one", TaskID: "task-one", SourceKind: autonomousview.SourceActive, Status: "pending", Title: "One"}
	projection := tuiAutonomousView("task-one", "working")
	projection.Attempts.Stops = []string{"verification_required"}

	t.Run("key and command entries retain overlay geometry and restore composer state", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) (StatusModel, tea.Cmd)
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) (StatusModel, tea.Cmd) {
					model.composer = commandComposerState{Text: "saved draft"}
					return updateStatusModel(t, model, keyRunes("6"))
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) (StatusModel, tea.Cmd) {
					model, _ = updateStatusModel(t, model, keyRunes("/workflow"))
					return updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				listCalls := 0
				loadCalls := 0
				model := NewStatusModelWithActions(app.StatusResult{Initialized: true, ProjectRoot: "/work/revolvr"}, StatusActions{
					ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
						listCalls++
						return []app.AutonomousTaskSelector{selector}, nil
					},
					LoadAutonomous: func(got string) (autonomousview.View, error) {
						loadCalls++
						if got != selector.Selector {
							t.Fatalf("selector = %q, want %q", got, selector.Selector)
						}
						return projection, nil
					},
				})
				model.message = "underlying notice"
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
				committed := slices.Clone(model.committed)
				model, cmd := entry.open(t, model)
				if cmd == nil || listCalls != 0 || model.overlay == nil || model.overlay.content != viewAutonomous {
					t.Fatalf("opened state: calls=%d overlay=%#v cmd=%v", listCalls, model.overlay, cmd)
				}
				if model.composer.Active || !reflect.DeepEqual(model.overlay.composer, entry.want) {
					t.Fatalf("composer=%#v saved=%#v, want inactive and %#v", model.composer, model.overlay.composer, entry.want)
				}
				model = drainStatusModelCmds(t, model, cmd)
				if listCalls != 1 || loadCalls != 1 || model.autonomous.View == nil {
					t.Fatalf("loads=%d/%d workflow=%#v", listCalls, loadCalls, model.autonomous.View)
				}

				content := normalizedViewLines(model.renderOverlayContent())
				requireLines(t, content,
					"Autonomous Workflow",
					"Status: pending | lifecycle: working | phase: working",
					"Task: task-one | title: Autonomous task",
					"Stops: verification_required",
					"Worker runs: worker-one",
				)
				requireLines(t, normalizedViewLines(model.View()),
					"Keys: j/k Select | enter Reload | a Answer | d Changes | e Evidence | U Run Task",
					"      Q Run Queue | r Refresh | pgup/pgdown Scroll | home/end Jump | esc Close",
				)
				for _, width := range []int{80, 40} {
					model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 24})
					assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
				}
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnd})
				if model.overlay.viewport.YOffset == 0 {
					t.Fatal("Workflow overlay did not scroll")
				}

				model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) || model.message != "underlying notice" {
					t.Fatalf("restored state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
				}
				if !reflect.DeepEqual(model.committed, committed) {
					t.Fatal("Workflow overlay changed committed transcript cells")
				}
			})
		}
	})

	t.Run("selection and refresh retain identity while stale results retain the newer owner", func(t *testing.T) {
		second := app.AutonomousTaskSelector{Selector: "task-two", TaskID: "task-two", SourceKind: autonomousview.SourceActive, Status: "pending", Title: "Two"}
		listCalls := 0
		refreshCalls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
				listCalls++
				if listCalls == 1 {
					return []app.AutonomousTaskSelector{selector, second}, nil
				}
				return []app.AutonomousTaskSelector{second, selector}, nil
			},
			LoadAutonomous: func(got string) (autonomousview.View, error) {
				return tuiAutonomousView(got, "ready"), nil
			},
			RefreshStatus: func() (app.StatusResult, error) {
				refreshCalls++
				return app.StatusResult{Initialized: true}, nil
			},
		})
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("6"))
		model = drainStatusModelCmds(t, model, cmd)
		for _, cell := range model.committed {
			model.emitted[cell.identity] = struct{}{}
		}
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
		if cmd == nil || model.selectedAutonomousPosition() != 1 || model.autonomous.Selected != 0 {
			t.Fatalf("local selection=%d page selection=%d cmd=%v", model.selectedAutonomousPosition(), model.autonomous.Selected, cmd)
		}
		model = drainStatusModelCmds(t, model, cmd)
		model, cmd = updateStatusModel(t, model, keyRunes("r"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd == nil {
			t.Fatalf("refresh returned no Workflow reload: overlay=%#v", model.overlay)
		}
		model = drainStatusModelCmds(t, model, cmd)
		if refreshCalls != 1 || listCalls != 2 || model.autonomous.Selector != "task-two" || model.selectedAutonomousPosition() != 0 {
			t.Fatalf("refresh=%d lists=%d selector=%q selection=%d", refreshCalls, listCalls, model.autonomous.Selector, model.selectedAutonomousPosition())
		}

		staleModel := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
				return []app.AutonomousTaskSelector{selector}, nil
			},
		})
		staleModel.composer.Active = false
		staleModel, staleCmd := updateStatusModel(t, staleModel, keyRunes("6"))
		owner := staleModel.overlay.owner
		staleModel, _ = updateStatusModel(t, staleModel, tea.KeyMsg{Type: tea.KeyEsc})
		staleModel, _ = updateStatusModel(t, staleModel, keyRunes("?"))
		staleModel, next := runStatusModelCmd(t, staleModel, staleCmd)
		if next != nil || staleModel.overlay == nil || staleModel.overlay.content != viewHelp || staleModel.overlay.owner == owner || len(staleModel.autonomous.Selectors) != 0 {
			t.Fatalf("stale result changed owner: overlay=%#v selectors=%#v cmd=%v", staleModel.overlay, staleModel.autonomous.Selectors, next)
		}
	})

	t.Run("needs input keeps the existing typed answer path", func(t *testing.T) {
		question := tuiAutonomousView("task-one", "needs_input")
		question.Input = autonomousview.OperatorInput{
			State:         "waiting",
			QuestionID:    "scope",
			Revision:      2,
			ContentSHA256: strings.Repeat("c", 64),
			Question:      "Choose scope.",
			Options:       []autonomousview.InputOption{{ID: "focused", Meaning: "Run focused tests."}},
		}
		var request app.AnswerAutonomousInputRequest
		answered := false
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) { return []app.AutonomousTaskSelector{selector}, nil },
			LoadAutonomous: func(string) (autonomousview.View, error) {
				if answered {
					resumed := question
					resumed.Input = autonomousview.OperatorInput{State: "none"}
					return resumed, nil
				}
				return question, nil
			},
			AnswerInput: func(got app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
				request = got
				answered = true
				return app.AnswerAutonomousInputResult{TaskID: got.TaskID, QuestionID: got.QuestionID, Revision: got.Revision, OptionID: got.OptionID, AnswerPersisted: true, Resumed: true}, nil
			},
		})
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("6"))
		model = drainStatusModelCmds(t, model, cmd)
		requireLines(t, normalizedViewLines(model.renderOverlayContent()), "State: waiting", "Question: scope | revision: 2 | sha256: "+strings.Repeat("c", 64))
		model, _ = updateStatusModel(t, model, keyRunes("a"))
		model, _ = updateStatusModel(t, model, keyRunes("j"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil || model.overlay == nil || !model.autonomous.Answer.Submitting {
			t.Fatalf("answer state=%#v overlay=%#v cmd=%v", model.autonomous.Answer, model.overlay, cmd)
		}
		model = drainStatusModelCmds(t, model, cmd)
		if request.TaskID != "task-one" || request.QuestionID != "scope" || request.Revision != 2 || request.ContentSHA != strings.Repeat("c", 64) || request.OptionID != "focused" || request.Operator != "tui-operator" {
			t.Fatalf("answer request = %#v", request)
		}
		if model.overlay == nil || model.overlay.content != viewAutonomous || !model.autonomous.Answer.Result.AnswerPersisted || model.autonomous.View.Input.State != "none" {
			t.Fatalf("answer result=%#v overlay=%#v", model.autonomous.Answer, model.overlay)
		}
	})

	t.Run("callback errors and active guards stay with the owning overlay", func(t *testing.T) {
		refreshCalls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) { return []app.AutonomousTaskSelector{selector}, nil },
			LoadAutonomous: func(string) (autonomousview.View, error) {
				return autonomousview.View{}, errors.New("evidence offline")
			},
			RefreshStatus: func() (app.StatusResult, error) {
				refreshCalls++
				return app.StatusResult{}, nil
			},
		})
		model.message = "underlying notice"
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("6"))
		model = drainStatusModelCmds(t, model, cmd)
		requireLines(t, normalizedViewLines(model.View()), "Notice: Workflow evidence load failed.", "Evidence error: evidence offline")

		model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeTask, Cancel: func() {}}
		for key, want := range map[string]string{
			"r": "Notice: Run is active; cancel or wait before refreshing.",
			"U": "Notice: Run already active.",
			"a": "Notice: Run is active; cancel or wait before answering input.",
		} {
			var guarded tea.Cmd
			model, guarded = updateStatusModel(t, model, keyRunes(key))
			if guarded != nil || model.overlay == nil || model.overlay.content != viewAutonomous {
				t.Fatalf("guard %q changed owner: overlay=%#v cmd=%v", key, model.overlay, guarded)
			}
			requireLines(t, normalizedViewLines(model.View()), want)
		}
		if refreshCalls != 0 {
			t.Fatalf("guarded refresh calls = %d, want 0", refreshCalls)
		}
		model.runOnce = runOnceState{}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay != nil || model.message != "underlying notice" {
			t.Fatalf("error return: overlay=%#v message=%q", model.overlay, model.message)
		}
	})

	t.Run("task progress and cancellation stay live behind the overlay", func(t *testing.T) {
		status := app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{{ID: "task-one", Status: taskmodel.StatusPending, Workflow: taskfile.WorkflowAutonomousV1}}}
		model := NewStatusModelWithActions(status, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) { return []app.AutonomousTaskSelector{selector}, nil },
			LoadAutonomous: func(string) (autonomousview.View, error) { return projection, nil },
			RunTask: func(ctx context.Context, taskID string, max int64, progress autonomoustaskrun.Progress) (autonomoustaskrun.Result, error) {
				progress(autonomoustaskrun.Operation{OperationID: "operation-one", Stage: "cycle_started", LastAction: "implement", Statistics: autonomoustaskrun.Statistics{CyclesStarted: 1}})
				<-ctx.Done()
				return autonomoustaskrun.Result{TaskID: taskID, OperationID: "operation-one", StopReason: autonomoustaskrun.StopOperationCancelled}, ctx.Err()
			},
			RefreshStatus: func() (app.StatusResult, error) { return status, nil },
		})
		model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
		model.composer.Active = false
		model, cmd := updateStatusModel(t, model, keyRunes("6"))
		model = drainStatusModelCmds(t, model, cmd)
		before := len(model.committed)
		model, cmd = updateStatusModel(t, model, keyRunes("U"))
		model, wait := runStatusModelCmd(t, model, cmd)
		requireLines(t, normalizedViewLines(model.View()), "Running: task-one", "Safety: admitted", "Current: implement · cycle started")
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		requireLines(t, normalizedViewLines(model.View()), "Cancelling: task-one", "Current: waiting for the run to stop")
		if model.overlay == nil || !model.runOnce.CancelRequested {
			t.Fatalf("Escape dismissed Workflow or missed cancellation: overlay=%#v run=%#v", model.overlay, model.runOnce)
		}
		model, appendCmd := runStatusModelCmd(t, model, wait)
		if appendCmd == nil || model.settling == nil {
			t.Fatalf("terminal result was not awaiting transcript append: settling=%#v cmd=%v", model.settling, appendCmd)
		}
		settling := *model.settling
		model, reload := updateStatusModel(t, model, transcriptCommittedMsg{token: settling.token, identity: settling.cell.identity})
		model = drainStatusModelCmds(t, model, reload)
		if model.overlay == nil || model.overlay.content != viewAutonomous || model.runOnce.Active || model.runOnce.Outcome != "operation_cancelled" || len(model.committed) != before+1 {
			t.Fatalf("settled state: overlay=%#v run=%#v committed=%d, want %d", model.overlay, model.runOnce, len(model.committed), before+1)
		}
		seen := map[string]bool{}
		for _, cell := range model.committed {
			if seen[cell.identity] {
				t.Fatalf("duplicate committed identity %q", cell.identity)
			}
			seen[cell.identity] = true
		}
	})
}

func TestChangeSummaryOverlay(t *testing.T) {
	started := time.Date(2026, 9, 3, 14, 0, 0, 0, time.UTC)
	longPath := "internal/tui/a/very/long/path/that/remains/source-traceable/change-summary-model.go"
	history := ledger.RunWithEvents{
		Run: ledger.Run{ID: "run-changes", CommitSHA: "abc123def456", StartedAt: started},
		Events: []ledger.Event{
			{ID: 11, RunID: "run-changes", Type: ledger.EventChangedFilesCaptured, Payload: jsonPayload(t, map[string]any{"changed_files": []string{longPath, longPath, "internal/tui/model_test.go"}}), CreatedAt: started.Add(time.Second)},
			{ID: 12, RunID: "run-changes", Type: ledger.EventCommitCreated, Payload: jsonPayload(t, map[string]any{"commit_sha": "abc123def456"}), CreatedAt: started.Add(2 * time.Second)},
		},
	}

	t.Run("key and command entries retain canonical renderer geometry and return state", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) StatusModel
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model.composer = commandComposerState{Text: "saved draft"}
					model, cmd := updateStatusModel(t, model, keyRunes("d"))
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/diff"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				model := NewStatusModel(app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}, LatestEvents: history.Events})
				model.message = "underlying notice"
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 10})
				committed := slices.Clone(model.committed)
				model = entry.open(t, model)

				if model.overlay == nil || model.overlay.content != viewDiff {
					t.Fatalf("overlay=%#v, want Change Summary", model.overlay)
				}
				if model.composer.Active || !reflect.DeepEqual(model.overlay.composer, entry.want) {
					t.Fatalf("composer=%#v saved=%#v, want inactive and %#v", model.composer, model.overlay.composer, entry.want)
				}
				overlayContent := model.renderOverlayContent()
				requireLines(t, normalizedViewLines(overlayContent),
					"Change Summary",
					"Run: run-changes",
					"Source: canonical changed-files events and run record",
					"Changed Files",
					longPath,
					"internal/tui/model_test.go",
					"commit: abc123def456",
					"11  changed_files_captured  2026-09-03T14:00:01Z",
					"12  commit_created  2026-09-03T14:00:02Z",
				)
				if strings.Count(overlayContent, longPath) != 1 || strings.Contains(overlayContent, "Exact Diff Artifacts") {
					t.Fatalf("canonical metadata compaction/distinction failed:\n%s", overlayContent)
				}
				requireLines(t, normalizedViewLines(model.View()), "Keys: pgup/pgdown Scroll | home/end Jump | r Refresh | esc Close | q Quit")
				for _, width := range []int{80, 40} {
					model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 10})
					assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
				}
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnd})
				if model.overlay.viewport.YOffset == 0 {
					t.Fatal("Change Summary overlay did not scroll")
				}

				model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) || model.message != "underlying notice" {
					t.Fatalf("restored state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
				}
				if !reflect.DeepEqual(model.committed, committed) {
					t.Fatal("Change Summary overlay changed committed transcript cells")
				}
			})
		}
	})

	t.Run("autonomous projection labels only exact diff artifacts as diffs", func(t *testing.T) {
		projection := tuiAutonomousView("task-changes", "working")
		projection.Provenance.References = append(projection.Provenance.References, autonomousview.Reference{
			Kind: "workspace_diff", Path: ".revolvr/autonomous/tasks/task-changes/artifacts/exact-workspace.diff", SHA256: strings.Repeat("d", 64), ByteSize: 321,
		})
		model := NewStatusModel(app.StatusResult{Initialized: true})
		model.composer = commandComposerState{Text: "workflow draft"}
		model.autonomous.View = &projection
		model.openOverlay(viewApproval, 0)
		model, _ = updateStatusModel(t, model, keyRunes("d"))

		if model.overlay == nil || model.overlay.content != viewDiff || model.overlay.parent != viewApproval || !model.focusedAutonomous() {
			t.Fatalf("autonomous Change Summary state: overlay=%#v", model.overlay)
		}
		content := normalizedViewLines(model.renderOverlayContent())
		requireLines(t, content,
			"Task: task-changes",
			"Workspace: ready",
			"Source revision: source-one",
			"Checkpoint commit: abc123",
			"Exact Diff Artifacts",
			"[workspace_diff] path=.revolvr/autonomous/tasks/task-changes/artifacts/exact-workspace.diff sha256="+strings.Repeat("d", 64)+" bytes=321",
		)
		requireNoLine(t, content, "Changed Files")
		requireNoLine(t, content, "Source: canonical changed-files events and run record")
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay == nil || model.overlay.content != viewApproval || model.composer.Text != "workflow draft" {
			t.Fatalf("autonomous return: overlay=%#v composer=%#v", model.overlay, model.composer)
		}
	})

	t.Run("refresh reloads selected run and guards the active operation", func(t *testing.T) {
		refreshed := history
		refreshed.Events = append(refreshed.Events, ledger.Event{ID: 13, RunID: history.Run.ID, Type: ledger.EventChangedFilesCaptured, Payload: jsonPayload(t, map[string]any{"changed_files": []string{"internal/tui/refreshed.go"}}), CreatedAt: started.Add(3 * time.Second)})
		refreshCalls := 0
		openCalls := 0
		cancelled := false
		refreshFailure := false
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}}, StatusActions{
			RefreshStatus: func() (app.StatusResult, error) {
				refreshCalls++
				if refreshFailure {
					return app.StatusResult{}, errors.New("status offline")
				}
				return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}}, nil
			},
			OpenRun: func(runID string) (ledger.RunWithEvents, error) {
				openCalls++
				if runID != history.Run.ID {
					t.Fatalf("opened run = %q, want %q", runID, history.Run.ID)
				}
				return refreshed, nil
			},
		})
		model.runDetails = &history
		model.openChangeSummaryOverlay()
		for _, cell := range model.committed {
			model.emitted[cell.identity] = struct{}{}
		}
		model, _ = updateStatusModel(t, model, keyRunes("d"))
		owner := model.overlay.owner
		model, cmd := updateStatusModel(t, model, keyRunes("r"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd == nil || refreshCalls != 1 || model.overlay == nil || model.overlay.owner != owner {
			t.Fatalf("refresh state: calls=%d overlay=%#v cmd=%v", refreshCalls, model.overlay, cmd)
		}
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || openCalls != 1 || model.runDetails == nil || len(model.runDetails.Events) != 3 || model.overlay == nil {
			t.Fatalf("reload state: calls=%d details=%#v overlay=%#v cmd=%v", openCalls, model.runDetails, model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.renderOverlayContent()), "internal/tui/refreshed.go", "13  changed_files_captured  2026-09-03T14:00:03Z")
		refreshFailure = true
		model, cmd = updateStatusModel(t, model, keyRunes("r"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || refreshCalls != 2 || model.overlay == nil {
			t.Fatalf("failed refresh: calls=%d overlay=%#v cmd=%v", refreshCalls, model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Refresh failed: status offline")

		model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeOnce, Cancel: func() { cancelled = true }}
		model, cmd = updateStatusModel(t, model, keyRunes("r"))
		if cmd != nil || refreshCalls != 2 || model.overlay == nil {
			t.Fatalf("guarded refresh: calls=%d overlay=%#v cmd=%v", refreshCalls, model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Run is active; cancel or wait before refreshing.")
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if !cancelled || !model.runOnce.CancelRequested || model.overlay == nil {
			t.Fatalf("active Escape: cancelled=%t run=%#v overlay=%#v", cancelled, model.runOnce, model.overlay)
		}
	})

	t.Run("late selected-run reload cannot replace a newer overlay", func(t *testing.T) {
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}}, StatusActions{
			RefreshStatus: func() (app.StatusResult, error) {
				return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}}, nil
			},
			OpenRun: func(string) (ledger.RunWithEvents, error) { return history, nil },
		})
		model.runDetails = &ledger.RunWithEvents{Run: history.Run}
		model.openChangeSummaryOverlay()
		for _, cell := range model.committed {
			model.emitted[cell.identity] = struct{}{}
		}
		model, _ = updateStatusModel(t, model, keyRunes("d"))
		model, cmd := updateStatusModel(t, model, keyRunes("r"))
		model, reload := runStatusModelCmd(t, model, cmd)
		if reload == nil {
			t.Fatal("refresh returned no selected-run reload")
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		model, _ = updateStatusModel(t, model, keyRunes("?"))
		model, cmd = runStatusModelCmd(t, model, reload)
		if cmd != nil || model.overlay == nil || model.overlay.content != viewHelp || len(model.runDetails.Events) != 0 {
			t.Fatalf("stale reload changed newer owner: overlay=%#v details=%#v cmd=%v", model.overlay, model.runDetails, cmd)
		}
	})
}

func TestEvidenceOverlay(t *testing.T) {
	started := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	history := validationDetailHistory("run-evidence")
	history.Run.VerificationStatus = "failed"
	history.Events = append(history.Events,
		ledger.Event{
			ID:        2,
			RunID:     history.Run.ID,
			Type:      ledger.EventVerificationCompleted,
			Payload:   jsonPayload(t, map[string]any{"status": "failed", "failed_command_index": 1, "commands": []map[string]any{{"index": 1, "command": "go test ./...", "status": "failed", "passed": false, "exit_code": 1}}}),
			CreatedAt: started,
		},
		ledger.Event{
			ID:        3,
			RunID:     history.Run.ID,
			Type:      ledger.EventReceiptWarning,
			Payload:   jsonPayload(t, map[string]any{"warning_type": "invalid_receipt", "message": "receipt hash mismatch", "receipt_path": ".revolvr/receipts/run-evidence.md"}),
			CreatedAt: started.Add(time.Second),
		},
	)

	t.Run("key and command entries retain canonical evidence geometry and return state", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) StatusModel
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model.composer = commandComposerState{Text: "saved draft"}
					model, cmd := updateStatusModel(t, model, keyRunes("e"))
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/evidence"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				model := NewStatusModel(app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}, LatestEvents: history.Events})
				model.message = "underlying notice"
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 10})
				committed := slices.Clone(model.committed)
				model = entry.open(t, model)

				if model.overlay == nil || model.overlay.content != viewEvidence {
					t.Fatalf("overlay=%#v, want Evidence", model.overlay)
				}
				if model.composer.Active || !reflect.DeepEqual(model.overlay.composer, entry.want) {
					t.Fatalf("composer=%#v saved=%#v, want inactive and %#v", model.composer, model.overlay.composer, entry.want)
				}
				overlayContent := model.renderOverlayContent()
				requireLines(t, normalizedViewLines(overlayContent),
					"Evidence",
					"Run: run-evidence | status: completed | verification: failed",
					"context payload: .revolvr/runs/run-evidence/context.md",
					"receipt: .revolvr/receipts/run-evidence.md",
					"verification: failed",
					"failed verification: go test ./... (exit_code=1)",
					"warning: invalid_receipt: receipt hash mismatch (.revolvr/receipts/run-evidence.md)",
					"Receipt Validation",
					"Status: not run",
					"Canonical Events",
					"3  receipt_warning  2026-09-03T15:00:01Z",
				)
				requireLines(t, normalizedViewLines(model.View()), "Keys: pgup/pgdown Scroll | home/end Jump | v Validate | r Refresh | esc Close")
				for _, width := range []int{80, 40} {
					model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 10})
					assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
				}
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnd})
				if model.overlay.viewport.YOffset == 0 {
					t.Fatal("Evidence overlay did not scroll")
				}

				model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) || model.message != "underlying notice" {
					t.Fatalf("restored state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
				}
				if !reflect.DeepEqual(model.committed, committed) {
					t.Fatal("Evidence overlay changed committed transcript cells")
				}
			})
		}
	})

	t.Run("autonomous groups keep statuses provenance missing evidence and warnings distinct", func(t *testing.T) {
		projection := tuiAutonomousView("task-evidence", "working")
		projection.Verification.Evidence = []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "verify-run:verify-occurrence", Detail: "Focused verification failed."}}
		projection.Acceptance = []autonomousview.Acceptance{
			{ID: "criterion-satisfied", Description: "Artifact identity is preserved.", Status: "satisfied", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: "internal/tui/model.go", Detail: "Renderer retains the reference."}}},
			{ID: "criterion-missing", Description: "Pending evidence is explicit.", Status: "pending"},
		}
		projection.Findings[0].Status = "invalid"
		projection.Findings[0].Evidence = []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: "internal/tui/stale.go", Detail: "Invalid evidence target."}}
		model := NewStatusModel(app.StatusResult{Initialized: true})
		model.composer = commandComposerState{Text: "workflow draft"}
		model.autonomous.View = &projection
		model.openOverlay(viewApproval, 0)
		model, _ = updateStatusModel(t, model, keyRunes("e"))

		if model.overlay == nil || model.overlay.content != viewEvidence || model.overlay.parent != viewApproval || !model.focusedAutonomous() {
			t.Fatalf("autonomous Evidence state: overlay=%#v", model.overlay)
		}
		content := normalizedViewLines(model.renderOverlayContent())
		requireLines(t, content,
			"Task: task-evidence",
			"[task] path=.agent/tasks/task-evidence.md run=none sha256="+strings.Repeat("a", 64)+" bytes=120 | Canonical task.",
			"Status: state=available result=passed run=verify-run occurrence=verify-occurrence source=source-one",
			"[verification] verify-run:verify-occurrence | Focused verification failed.",
			"[satisfied] criterion-satisfied: Artifact identity is preserved.",
			"[file] internal/tui/model.go | Renderer retains the reference.",
			"[pending] criterion-missing: Pending evidence is explicit.",
			"Evidence: missing",
			"[invalid/blocking] finding-one: A blocking issue remains.",
			"[file] internal/tui/stale.go | Invalid evidence target.",
			"Warning: [optional_history_missing/audit] Optional legacy history was unavailable. | reference=history",
		)
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay == nil || model.overlay.content != viewApproval || model.composer.Text != "workflow draft" {
			t.Fatalf("autonomous return: overlay=%#v composer=%#v", model.overlay, model.composer)
		}
	})

	t.Run("refresh preserves autonomous identity and rejects a stale load", func(t *testing.T) {
		first := app.AutonomousTaskSelector{Selector: "task-one", TaskID: "task-one", SourceKind: autonomousview.SourceActive, Status: "pending"}
		selected := app.AutonomousTaskSelector{Selector: "task-evidence", TaskID: "task-evidence", SourceKind: autonomousview.SourceActive, Status: "pending"}
		projection := tuiAutonomousView(selected.TaskID, "ready")
		listCalls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			RefreshStatus: func() (app.StatusResult, error) { return app.StatusResult{Initialized: true}, nil },
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
				listCalls++
				return []app.AutonomousTaskSelector{selected, first}, nil
			},
			LoadAutonomous: func(selector string) (autonomousview.View, error) {
				if selector != selected.Selector {
					t.Fatalf("loaded selector = %q, want %q", selector, selected.Selector)
				}
				return projection, nil
			},
		})
		model.autonomous = autonomousState{Selectors: []app.AutonomousTaskSelector{first, selected}, Selected: 1, Selector: selected.Selector, TaskID: selected.TaskID, View: &projection}
		model.openOverlay(viewAutonomous, model.autonomous.Selected)
		for _, cell := range model.committed {
			model.emitted[cell.identity] = struct{}{}
		}
		model, _ = updateStatusModel(t, model, keyRunes("e"))
		owner := model.overlay.owner
		model, cmd := updateStatusModel(t, model, keyRunes("r"))
		model = drainStatusModelCmds(t, model, cmd)
		if listCalls != 1 || model.overlay == nil || model.overlay.owner != owner || model.autonomous.Selector != selected.Selector || model.autonomous.Selected != 0 {
			t.Fatalf("refresh: lists=%d overlay=%#v selector=%q selection=%d", listCalls, model.overlay, model.autonomous.Selector, model.autonomous.Selected)
		}

		model, cmd = updateStatusModel(t, model, keyRunes("r"))
		model, staleLoad := runStatusModelCmd(t, model, cmd)
		if staleLoad == nil {
			t.Fatal("refresh returned no autonomous selector load")
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		model, _ = updateStatusModel(t, model, keyRunes("?"))
		model, next := runStatusModelCmd(t, model, staleLoad)
		if next != nil || model.overlay == nil || model.overlay.content != viewHelp || model.overlay.owner == owner || listCalls != 2 {
			t.Fatalf("stale load changed newer owner: overlay=%#v lists=%d cmd=%v", model.overlay, listCalls, next)
		}
	})

	t.Run("canonical receipt validation stays in the owning overlay", func(t *testing.T) {
		validationCalls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{history.Run}, LatestEvents: history.Events}, StatusActions{
			ValidateReceipt: func(runID string) (receipt.ValidationResult, error) {
				validationCalls++
				return receipt.ValidationResult{RunID: runID, ReceiptPath: ".revolvr/receipts/run-evidence.md", Checks: []receipt.ValidationCheck{{Name: receipt.ValidationCheckIdentity, Passed: false, Details: []string{"receipt identity is invalid"}}}}, nil
			},
		})
		model.composer.Active = false
		model, _ = updateStatusModel(t, model, keyRunes("e"))
		model, cmd := updateStatusModel(t, model, keyRunes("v"))
		if cmd == nil || validationCalls != 0 {
			t.Fatalf("validation started: calls=%d target=%q overlay=%#v cmd=%v", validationCalls, model.validationTargetRunID(), model.overlay, cmd)
		}
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || validationCalls != 1 || model.overlay == nil || model.overlay.content != viewEvidence {
			t.Fatalf("validation result: calls=%d overlay=%#v cmd=%v", validationCalls, model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.renderOverlayContent()),
			"Notice: Receipt validation failed.",
			"Status: failed",
			"FAIL identity: failed - receipt identity is invalid",
		)
	})
}

func TestApprovalOverlay(t *testing.T) {
	question := tuiAutonomousView("task-approval", "needs_input")
	question.Input = autonomousview.OperatorInput{
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
	selector := app.AutonomousTaskSelector{Selector: "task-approval", TaskID: "task-approval", SourceKind: autonomousview.SourceActive, Status: "pending"}

	t.Run("key and command entries retain the page renderer and restore source state", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) (StatusModel, tea.Cmd)
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) (StatusModel, tea.Cmd) {
					model.composer = commandComposerState{Text: "saved draft"}
					return updateStatusModel(t, model, keyRunes("A"))
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) (StatusModel, tea.Cmd) {
					model, _ = updateStatusModel(t, model, keyRunes("/approval"))
					return updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				calls := 0
				model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
					AnswerInput: func(app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
						calls++
						return app.AnswerAutonomousInputResult{}, nil
					},
				})
				model.message = "underlying notice"
				model.autonomous = autonomousState{Selectors: []app.AutonomousTaskSelector{selector}, Selector: selector.Selector, TaskID: selector.TaskID, View: &question}
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 10})
				committed := slices.Clone(model.committed)
				model, cmd := entry.open(t, model)
				if cmd != nil || calls != 0 || model.overlay == nil || model.overlay.content != viewApproval {
					t.Fatalf("opened state: calls=%d overlay=%#v cmd=%v", calls, model.overlay, cmd)
				}
				if model.composer.Active || !reflect.DeepEqual(model.overlay.composer, entry.want) {
					t.Fatalf("composer=%#v saved=%#v, want inactive and %#v", model.composer, model.overlay.composer, entry.want)
				}

				requireLines(t, normalizedViewLines(model.renderOverlayContent()),
					"Approval",
					"Task: task-approval",
					"Status: pending | lifecycle: needs_input",
					"Latest decision: implement",
					"[satisfied] criterion-one: The feature works.",
					"State: waiting",
					"Question: deployment-mode | revision: 2 | sha256: "+strings.Repeat("c", 64),
					"  Option change: Change behavior.",
					"  Option keep: Keep behavior.",
					"Recommendation (not selected): keep | Compatibility.",
				)
				requireLines(t, normalizedViewLines(model.View()), "Keys: a Answer | d Changes | e Evidence | pgup/pgdown Scroll | home/end Jump")
				for _, width := range []int{80, 40} {
					model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 10})
					assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
				}
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnd})
				if model.overlay.viewport.YOffset == 0 {
					t.Fatal("Approval overlay did not scroll")
				}

				model, _ = updateStatusModel(t, model, keyRunes("a"))
				if model.overlay == nil || model.overlay.content != viewNeedsInput || model.overlay.parent != viewApproval {
					t.Fatalf("typed child = %#v, want Needs Input over Approval", model.overlay)
				}
				requireLines(t, normalizedViewLines(model.renderOverlayContent()), "Needs Input", "Question: deployment-mode | revision: 2 | sha256: "+strings.Repeat("c", 64), "  change: Change behavior.", "  keep: Keep behavior.")
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
				if calls != 0 || !model.autonomous.Answer.Confirming {
					t.Fatalf("unconfirmed state=%#v calls=%d", model.autonomous.Answer, calls)
				}
				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if model.overlay == nil || model.overlay.content != viewApproval || model.autonomous.Answer.Active || calls != 0 {
					t.Fatalf("cancelled answer: overlay=%#v answer=%#v calls=%d", model.overlay, model.autonomous.Answer, calls)
				}
				model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) || model.message != "underlying notice" {
					t.Fatalf("restored state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
				}
				if !reflect.DeepEqual(model.committed, committed) {
					t.Fatal("Approval overlay changed committed transcript cells")
				}
			})
		}
	})

	t.Run("confirmed decision submits one exact identity and remains visible", func(t *testing.T) {
		calls := 0
		var request app.AnswerAutonomousInputRequest
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			AnswerInput: func(got app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
				calls++
				request = got
				return app.AnswerAutonomousInputResult{TaskID: got.TaskID, QuestionID: got.QuestionID, Revision: got.Revision, OptionID: got.OptionID, AnswerID: "answer-one", AnswerPersisted: true, Resumed: true}, nil
			},
			LoadAutonomous: func(got string) (autonomousview.View, error) {
				if got != selector.Selector {
					t.Fatalf("reloaded selector = %q, want %q", got, selector.Selector)
				}
				resumed := question
				resumed.Input = autonomousview.OperatorInput{State: "none"}
				return resumed, nil
			},
		})
		model.composer.Active = false
		model.autonomous = autonomousState{Selectors: []app.AutonomousTaskSelector{selector}, Selector: selector.Selector, TaskID: selector.TaskID, View: &question}
		model, _ = updateStatusModel(t, model, keyRunes("A"))
		model, _ = updateStatusModel(t, model, keyRunes("a"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
		model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || calls != 0 || !model.autonomous.Answer.Confirming {
			t.Fatalf("first confirmation state=%#v calls=%d cmd=%v", model.autonomous.Answer, calls, cmd)
		}
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil || calls != 0 || !model.autonomous.Answer.Submitting {
			t.Fatalf("submission state=%#v calls=%d cmd=%v", model.autonomous.Answer, calls, cmd)
		}
		model, cmd = runStatusModelCmd(t, model, cmd)
		if calls != 1 || cmd == nil || request.TaskID != "task-approval" || request.QuestionID != "deployment-mode" || request.Revision != 2 || request.ContentSHA != strings.Repeat("c", 64) || request.OptionID != "change" || request.Operator != "tui-operator" {
			t.Fatalf("calls=%d request=%#v reload=%v", calls, request, cmd)
		}
		if model.overlay == nil || model.overlay.content != viewNeedsInput || model.overlay.parent != viewApproval || !model.autonomous.Answer.Result.AnswerPersisted {
			t.Fatalf("answer result=%#v overlay=%#v", model.autonomous.Answer, model.overlay)
		}
		requireLines(t, normalizedViewLines(model.renderOverlayContent()), "Notice: Answer persisted and task resumed.", "Last answer: id=answer-one option=change persisted=true resumed=true")
		model = drainStatusModelCmds(t, model, cmd)
		if model.overlay == nil || model.overlay.content != viewApproval || model.autonomous.View.Input.State != "none" {
			t.Fatalf("reloaded approval state: overlay=%#v input=%#v", model.overlay, model.autonomous.View.Input)
		}
	})

	t.Run("stale rejection and active guards stay with the owning overlay", func(t *testing.T) {
		calls := 0
		refreshCalls := 0
		cancelled := false
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			AnswerInput: func(got app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
				calls++
				return app.AnswerAutonomousInputResult{TaskID: got.TaskID, QuestionID: got.QuestionID, Revision: got.Revision, OptionID: got.OptionID}, errors.New("stale approval request")
			},
			RefreshStatus: func() (app.StatusResult, error) {
				refreshCalls++
				return app.StatusResult{Initialized: true}, nil
			},
			LoadAutonomous: func(string) (autonomousview.View, error) { return question, nil },
		})
		model.composer.Active = false
		model.autonomous = autonomousState{Selectors: []app.AutonomousTaskSelector{selector}, Selector: selector.Selector, TaskID: selector.TaskID, View: &question}
		model, _ = updateStatusModel(t, model, keyRunes("A"))
		model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeTask, Cancel: func() { cancelled = true }}
		model, cmd := updateStatusModel(t, model, keyRunes("a"))
		if cmd != nil || model.autonomous.Answer.Active || calls != 0 {
			t.Fatalf("guarded answer=%#v calls=%d cmd=%v", model.autonomous.Answer, calls, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Run is active; cancel or wait before answering input.")
		model, cmd = updateStatusModel(t, model, keyRunes("r"))
		if cmd != nil || refreshCalls != 0 || model.overlay == nil {
			t.Fatalf("guarded refresh: calls=%d overlay=%#v cmd=%v", refreshCalls, model.overlay, cmd)
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if !cancelled || !model.runOnce.CancelRequested || model.overlay == nil {
			t.Fatalf("active Escape: cancelled=%t run=%#v overlay=%#v", cancelled, model.runOnce, model.overlay)
		}

		model.runOnce = runOnceState{}
		model, _ = updateStatusModel(t, model, keyRunes("a"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		changed := question
		changed.Input.Revision++
		model.autonomous.View = &changed
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || calls != 0 || !model.autonomous.Answer.Active || model.autonomous.Answer.Selected != -1 || model.overlay == nil || model.overlay.content != viewNeedsInput {
			t.Fatalf("stale confirmation: calls=%d answer=%#v overlay=%#v cmd=%v", calls, model.autonomous.Answer, model.overlay, cmd)
		}
		if !strings.Contains(oneLine(model.View()), "Answer not submitted: the selected question changed; review the current options.") {
			t.Fatalf("stale refusal not visible:\n%s", model.View())
		}
		requireNoLine(t, normalizedViewLines(model.View()), "Last answer:")

		model.autonomous.View = &question
		model, _ = updateStatusModel(t, model, keyRunes("a"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, reload := runStatusModelCmd(t, model, cmd)
		if reload == nil || calls != 1 || model.overlay == nil || model.overlay.content != viewNeedsInput || !model.autonomous.Answer.Active || model.autonomous.Answer.Selected != 0 || model.autonomous.Answer.Result.AnswerPersisted {
			t.Fatalf("rejected result: calls=%d answer=%#v overlay=%#v reload=%v", calls, model.autonomous.Answer, model.overlay, reload)
		}
		requireLines(t, normalizedViewLines(model.renderOverlayContent()), "Notice: Answer failed.", "Answer error: stale approval request")
		requireNoLine(t, normalizedViewLines(model.renderOverlayContent()), "Last answer:")
	})

	t.Run("refresh retains exact selection and stale loads cannot replace a newer owner", func(t *testing.T) {
		first := app.AutonomousTaskSelector{Selector: "task-first", TaskID: "task-first", SourceKind: autonomousview.SourceActive, Status: "pending"}
		listCalls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			RefreshStatus: func() (app.StatusResult, error) { return app.StatusResult{Initialized: true}, nil },
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
				listCalls++
				return []app.AutonomousTaskSelector{selector, first}, nil
			},
			LoadAutonomous: func(got string) (autonomousview.View, error) {
				if got != selector.Selector {
					t.Fatalf("loaded selector = %q, want %q", got, selector.Selector)
				}
				return question, nil
			},
		})
		model.composer.Active = false
		model.autonomous = autonomousState{Selectors: []app.AutonomousTaskSelector{first, selector}, Selected: 1, Selector: selector.Selector, TaskID: selector.TaskID, View: &question}
		for _, cell := range model.committed {
			model.emitted[cell.identity] = struct{}{}
		}
		model, _ = updateStatusModel(t, model, keyRunes("A"))
		owner := model.overlay.owner
		model, cmd := updateStatusModel(t, model, keyRunes("r"))
		model = drainStatusModelCmds(t, model, cmd)
		if listCalls != 1 || model.overlay == nil || model.overlay.owner != owner || model.autonomous.Selector != selector.Selector || model.selectedAutonomousPosition() != 0 || model.autonomous.Selected != 1 {
			t.Fatalf("refresh: lists=%d overlay=%#v selector=%q selection=%d page=%d", listCalls, model.overlay, model.autonomous.Selector, model.selectedAutonomousPosition(), model.autonomous.Selected)
		}

		stale := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) { return []app.AutonomousTaskSelector{selector}, nil },
		})
		stale.composer.Active = false
		stale, staleCmd := updateStatusModel(t, stale, keyRunes("A"))
		staleOwner := stale.overlay.owner
		stale, _ = updateStatusModel(t, stale, tea.KeyMsg{Type: tea.KeyEsc})
		stale, _ = updateStatusModel(t, stale, keyRunes("?"))
		stale, next := runStatusModelCmd(t, stale, staleCmd)
		if next != nil || stale.overlay == nil || stale.overlay.content != viewHelp || stale.overlay.owner == staleOwner || len(stale.autonomous.Selectors) != 0 {
			t.Fatalf("stale result changed owner: overlay=%#v selectors=%#v cmd=%v", stale.overlay, stale.autonomous.Selectors, next)
		}
	})
}

func TestNeedsInputChildOverlay(t *testing.T) {
	question := tuiAutonomousView("task-input", "needs_input")
	question.Input = autonomousview.OperatorInput{
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

	for _, parent := range []TUIView{viewAutonomous, viewApproval} {
		t.Run(fmt.Sprintf("parent_%d_preserves_selection_scroll_and_composer", parent), func(t *testing.T) {
			calls := 0
			model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{AnswerInput: func(app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
				calls++
				return app.AnswerAutonomousInputResult{}, nil
			}})
			model.composer = commandComposerState{Active: true, Text: "saved draft"}
			model.autonomous = autonomousState{Selected: 1, Selector: "task-input", TaskID: "task-input", View: &question}
			model.openOverlay(parent, 1)
			model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 10})
			model.overlay.viewport.GotoBottom()
			parentOffset := model.overlay.viewport.YOffset

			model, _ = updateStatusModel(t, model, keyRunes("a"))
			if model.overlay == nil || model.overlay.content != viewNeedsInput || model.overlay.parent != parent || model.overlay.selected != 1 || model.overlay.parentOffset != parentOffset || model.composer.Active {
				t.Fatalf("child state = %#v composer=%#v", model.overlay, model.composer)
			}
			content := normalizedViewLines(model.renderOverlayContent())
			requireLines(t, content, "Needs Input", "Task: task-input", "Question: deployment-mode | revision: 2 | sha256: "+strings.Repeat("c", 64), "  change: Change behavior.", "  keep: Keep behavior.")
			lines := normalizedViewLines(model.View())
			requireLines(t, lines, "Keys: j/k Choose option | enter Confirm", "      esc Back | ctrl+c Quit")
			assertMaxLineWidth(t, lines, 40)

			model, _ = updateStatusModel(t, model, keyRunes("free-form answer"))
			if calls != 0 || !model.autonomous.Answer.Active || model.autonomous.Answer.Selected != -1 {
				t.Fatalf("free-form input changed answer: calls=%d answer=%#v", calls, model.autonomous.Answer)
			}
			model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
			if model.overlay == nil || model.overlay.content != parent || model.overlay.selected != 1 || model.overlay.viewport.YOffset != parentOffset || model.composer.Active {
				t.Fatalf("parent return = %#v composer=%#v", model.overlay, model.composer)
			}
			model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
			if model.overlay != nil || !reflect.DeepEqual(model.composer, commandComposerState{Active: true, Text: "saved draft"}) {
				t.Fatalf("source return overlay=%#v composer=%#v", model.overlay, model.composer)
			}
		})
	}

	t.Run("failure preserves selection and stale result cannot dismiss replacement", func(t *testing.T) {
		calls := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			AnswerInput: func(got app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
				calls++
				return app.AnswerAutonomousInputResult{TaskID: got.TaskID, QuestionID: got.QuestionID, Revision: got.Revision, OptionID: got.OptionID}, errors.New("answer rejected")
			},
			LoadAutonomous: func(string) (autonomousview.View, error) { return question, nil },
		})
		model.autonomous = autonomousState{Selector: "task-input", TaskID: "task-input", View: &question}
		model.openOverlay(viewAutonomous, 0)
		model.beginAutonomousAnswer()
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, reload := runStatusModelCmd(t, model, cmd)
		if calls != 1 || reload == nil || model.overlay == nil || model.overlay.content != viewNeedsInput || !model.autonomous.Answer.Active || model.autonomous.Answer.Selected != 0 {
			t.Fatalf("failure state calls=%d overlay=%#v answer=%#v reload=%v", calls, model.overlay, model.autonomous.Answer, reload)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Answer failed.", "Answer error: answer rejected")

		model.actions.AnswerInput = func(got app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
			calls++
			return app.AnswerAutonomousInputResult{TaskID: got.TaskID, QuestionID: got.QuestionID, Revision: got.Revision, OptionID: got.OptionID, AnswerPersisted: true}, nil
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		replacement := question
		replacement.Input.QuestionID = "replacement"
		replacement.Input.Revision++
		replacement.Input.ContentSHA256 = strings.Repeat("d", 64)
		replacement.Input.Options = []autonomousview.InputOption{{ID: "replacement", Meaning: "Use the replacement."}}
		updated, _ := model.Update(autonomousViewMsg{token: model.autonomous.Request, selector: model.autonomous.Selector, view: replacement})
		model = updated.(StatusModel)
		model, next := runStatusModelCmd(t, model, cmd)
		if next != nil || calls != 2 || model.overlay == nil || model.overlay.content != viewNeedsInput || model.autonomous.Answer.Result.AnswerPersisted || model.autonomous.Answer.Selected != -1 || model.autonomous.View.Input.QuestionID != "replacement" {
			t.Fatalf("stale result state calls=%d overlay=%#v answer=%#v cmd=%v", calls, model.overlay, model.autonomous.Answer, next)
		}
	})
}

func TestTasksOverlay(t *testing.T) {
	t.Run("entries render the task list and restore composer state", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) StatusModel
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model.composer = commandComposerState{Text: "saved draft"}
					model, cmd := updateStatusModel(t, model, keyRunes("2"))
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/tasks"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				model := NewStatusModel(app.StatusResult{Initialized: true, Tasks: sampleTasks()})
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
				committed := slices.Clone(model.committed)
				model = entry.open(t, model)

				if model.overlay == nil || model.overlay.content != viewTasks {
					t.Fatalf("overlay=%#v, want Tasks", model.overlay)
				}
				if !reflect.DeepEqual(model.overlay.composer, entry.want) || model.composer.Active {
					t.Fatalf("composer=%#v saved=%#v, want inactive and %#v", model.composer, model.overlay.composer, entry.want)
				}
				requireLines(t, normalizedViewLines(model.View()), "Tasks", "Task List", "Task Detail", "Keys: j/k Select | enter Workflow | a Add Task | r Refresh | esc Close | q Quit")
				assertMaxLineWidth(t, normalizedViewLines(model.View()), 80)
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 24})
				assertMaxLineWidth(t, normalizedViewLines(model.View()), 40)

				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
				if got := model.selectedTaskID(); got != "task-blocked" {
					t.Fatalf("overlay selection = %q, want task-blocked", got)
				}
				model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) {
					t.Fatalf("restored state: overlay=%#v composer=%#v cmd=%v", model.overlay, model.composer, cmd)
				}
				if !reflect.DeepEqual(model.committed, committed) {
					t.Fatal("Tasks overlay changed committed transcript cells")
				}
			})
		}
	})

	t.Run("refresh and retry preserve canonical selection with fallback", func(t *testing.T) {
		tasks := sampleTasks()
		retried := tasks[1]
		retried.Status = taskmodel.StatusPending
		retried.Blocker = ""
		retried.BlockedAt = nil
		calls := []string{}
		refreshes := 0
		retries := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, Tasks: tasks}, StatusActions{
			RetryTask: func(taskID string) (taskmodel.Task, error) {
				retries++
				calls = append(calls, "retry:"+taskID)
				if retries == 1 {
					return taskmodel.Task{}, errors.New("storage locked")
				}
				return retried, nil
			},
			RefreshStatus: func() (app.StatusResult, error) {
				refreshes++
				calls = append(calls, fmt.Sprintf("refresh:%d", refreshes))
				if refreshes == 1 {
					return app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{tasks[1], tasks[2], tasks[0]}}, nil
				}
				return app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{retried, tasks[2], tasks[0]}}, nil
			},
		})
		model.message = "underlying notice"
		if model.Init() == nil {
			t.Fatal("initial committed cells returned no append command")
		}
		model.composer.Active = false
		model, _ = updateStatusModel(t, model, keyRunes("2"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})

		afterRefresh, cmd := updateStatusModel(t, model, keyRunes("r"))
		if cmd == nil {
			t.Fatal("refresh key returned nil cmd")
		}
		afterRefresh, cmd = runStatusModelCmd(t, afterRefresh, cmd)
		if cmd != nil || afterRefresh.selectedTaskID() != "task-blocked" {
			t.Fatalf("refreshed selection=%q cmd=%v, want task-blocked", afterRefresh.selectedTaskID(), cmd)
		}

		failedRetry, cmd := updateStatusModel(t, afterRefresh, keyRunes("u"))
		if cmd == nil {
			t.Fatal("retry key returned nil cmd")
		}
		failedRetry, cmd = runStatusModelCmd(t, failedRetry, cmd)
		if cmd != nil || failedRetry.overlay == nil || failedRetry.selectedTaskID() != "task-blocked" {
			t.Fatalf("failed retry state: overlay=%#v selection=%q cmd=%v", failedRetry.overlay, failedRetry.selectedTaskID(), cmd)
		}
		requireLines(t, normalizedViewLines(failedRetry.View()), "Notice: Retry failed: storage locked")

		afterRetry, cmd := updateStatusModel(t, failedRetry, keyRunes("u"))
		if cmd == nil {
			t.Fatal("second retry key returned nil cmd")
		}
		afterRetry, cmd = runStatusModelCmd(t, afterRetry, cmd)
		if cmd != nil || afterRetry.overlay == nil || afterRetry.selectedTaskID() != "task-blocked" {
			t.Fatalf("retry state: overlay=%#v selection=%q cmd=%v", afterRetry.overlay, afterRetry.selectedTaskID(), cmd)
		}
		if !reflect.DeepEqual(calls, []string{"refresh:1", "retry:task-blocked", "retry:task-blocked", "refresh:2"}) {
			t.Fatalf("callback order = %#v", calls)
		}
		requireLines(t, normalizedViewLines(afterRetry.View()), "Notice: Retried task task-blocked.", "> - task-blocked  pending  blocked task")
		guarded, cmd := updateStatusModel(t, afterRetry, keyRunes("u"))
		if cmd != nil || len(calls) != 4 || guarded.overlay == nil {
			t.Fatalf("retry guard: calls=%#v overlay=%#v cmd=%v", calls, guarded.overlay, cmd)
		}
		if got := oneLine(guarded.View()); !strings.Contains(got, "Notice: Retry unavailable: selected task task-blocked is not blocked (status: pending).") {
			t.Fatalf("retry guard notice missing from %q", got)
		}

		fallback, cmd := updateStatusModel(t, guarded, refreshStatusMsg{status: app.StatusResult{Initialized: true, Tasks: []taskmodel.Task{tasks[2], tasks[0]}}})
		if cmd != nil || fallback.selectedTaskID() != "task-completed" {
			t.Fatalf("fallback selection=%q cmd=%v, want first task", fallback.selectedTaskID(), cmd)
		}
		failed, cmd := updateStatusModel(t, fallback, refreshStatusMsg{err: errors.New("offline")})
		if cmd != nil || failed.overlay == nil {
			t.Fatalf("failed refresh dismissed overlay: overlay=%#v cmd=%v", failed.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(failed.View()), "Notice: Refresh failed: offline")

		failed, _ = updateStatusModel(t, failed, tea.KeyMsg{Type: tea.KeyEsc})
		if failed.message != "underlying notice" {
			t.Fatalf("underlying notice = %q, want preserved", failed.message)
		}
	})

	t.Run("add and workflow reuse existing action paths", func(t *testing.T) {
		tasks := sampleTasks()
		added := taskmodel.Task{ID: "task-new", Status: taskmodel.StatusPending, Task: "new task", Summary: "new task"}
		calls := []string{}
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, Tasks: tasks}, StatusActions{
			AddTask: func(input app.AddTaskInput) (taskmodel.Task, error) {
				calls = append(calls, "add:"+input.Task)
				return added, nil
			},
			RefreshStatus: func() (app.StatusResult, error) {
				calls = append(calls, "refresh")
				return app.StatusResult{Initialized: true, Tasks: append(tasks, added)}, nil
			},
		})
		model.message = "underlying notice"
		model.composer.Active = false
		model, _ = updateStatusModel(t, model, keyRunes("2"))
		entry, cmd := updateStatusModel(t, model, keyRunes("a"))
		if cmd != nil || entry.overlay == nil || entry.overlay.content != viewTaskEntry {
			t.Fatalf("task entry state: overlay=%#v cmd=%v", entry.overlay, cmd)
		}
		empty, cmd := updateStatusModel(t, entry, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || len(calls) != 0 {
			t.Fatalf("empty add: calls=%#v cmd=%v", calls, cmd)
		}
		requireLines(t, normalizedViewLines(empty.View()), "Error: Task text is required.")

		empty, _ = typeIntoStatusModel(t, empty, "new task")
		submitted, cmd := updateStatusModel(t, empty, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("add submit returned nil cmd")
		}
		addedModel, cmd := runStatusModelCmd(t, submitted, cmd)
		if cmd != nil || addedModel.overlay == nil || addedModel.overlay.content != viewTasks || addedModel.selectedTaskID() != "task-new" {
			t.Fatalf("add result: overlay=%#v selection=%q cmd=%v", addedModel.overlay, addedModel.selectedTaskID(), cmd)
		}
		if !reflect.DeepEqual(calls, []string{"add:new task", "refresh"}) {
			t.Fatalf("add callback order = %#v", calls)
		}
		requireLines(t, normalizedViewLines(addedModel.View()), "Notice: Added and committed task task-new.", "> - task-new  pending  new task")

		addedModel, _ = updateStatusModel(t, addedModel, tea.KeyMsg{Type: tea.KeyEsc})
		if addedModel.message != "underlying notice" || addedModel.overlay != nil {
			t.Fatalf("add return state: message=%q overlay=%#v", addedModel.message, addedModel.overlay)
		}

		workflowCalls := 0
		workflow := NewStatusModelWithActions(app.StatusResult{Initialized: true, Tasks: tasks}, StatusActions{
			ListAutonomous: func() ([]app.AutonomousTaskSelector, error) {
				workflowCalls++
				return nil, nil
			},
		})
		workflow.composer.Active = false
		workflow, _ = updateStatusModel(t, workflow, keyRunes("2"))
		workflow, _ = updateStatusModel(t, workflow, tea.KeyMsg{Type: tea.KeyDown})
		workflow, cmd = updateStatusModel(t, workflow, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil || workflow.overlay == nil || workflow.overlay.content != viewAutonomous || workflow.autonomous.TaskID != "task-blocked" || workflowCalls != 0 {
			t.Fatalf("workflow transition: overlay=%#v task=%q calls=%d cmd=%v", workflow.overlay, workflow.autonomous.TaskID, workflowCalls, cmd)
		}
		workflow, cmd = runStatusModelCmd(t, workflow, cmd)
		if cmd != nil || workflowCalls != 1 || workflow.overlay == nil || workflow.overlay.content != viewAutonomous {
			t.Fatalf("workflow load: overlay=%#v calls=%d cmd=%v", workflow.overlay, workflowCalls, cmd)
		}

		unavailable := NewStatusModel(app.StatusResult{Initialized: true, Tasks: tasks})
		unavailable.composer.Active = false
		unavailable, _ = updateStatusModel(t, unavailable, keyRunes("2"))
		unavailable, cmd = updateStatusModel(t, unavailable, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd != nil || unavailable.overlay == nil || unavailable.overlay.content != viewAutonomous || unavailable.autonomous.Err != "Workflow selector loading is unavailable." {
			t.Fatalf("workflow guard: overlay=%#v error=%q cmd=%v", unavailable.overlay, unavailable.autonomous.Err, cmd)
		}
	})

	t.Run("add failure stays in the owning overlay", func(t *testing.T) {
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
			AddTask: func(app.AddTaskInput) (taskmodel.Task, error) { return taskmodel.Task{}, errors.New("dirty worktree") },
			RefreshStatus: func() (app.StatusResult, error) {
				t.Fatal("refresh ran after add failure")
				return app.StatusResult{}, nil
			},
		})
		model.message = "underlying notice"
		model.composer.Active = false
		model, _ = updateStatusModel(t, model, keyRunes("2"))
		model, _ = updateStatusModel(t, model, keyRunes("a"))
		model, _ = typeIntoStatusModel(t, model, "new task")
		model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || model.overlay == nil || model.overlay.content != viewTaskEntry {
			t.Fatalf("failed add state: overlay=%#v cmd=%v", model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Error: Add failed: dirty worktree")
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay == nil || model.overlay.content != viewTasks {
			t.Fatalf("cancelled add did not return to Tasks overlay: %#v", model.overlay)
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay != nil || model.message != "underlying notice" {
			t.Fatalf("failed add return: overlay=%#v message=%q", model.overlay, model.message)
		}
	})
}

func TestRunsOverlay(t *testing.T) {
	runs := []ledger.Run{
		{ID: "run-a", Status: ledger.StatusCompleted, Summary: "first"},
		{ID: "run-b", Status: ledger.StatusFailed, Summary: "second"},
		{ID: "run-c", Status: ledger.StatusRunning, Summary: "third"},
	}

	t.Run("entries render run history and restore composer state", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) StatusModel
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model.composer = commandComposerState{Text: "saved draft"}
					model, cmd := updateStatusModel(t, model, keyRunes("3"))
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/runs"))
					model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					if cmd != nil {
						t.Fatalf("open command = %v, want nil", cmd)
					}
					return model
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				model := NewStatusModel(app.StatusResult{Initialized: true, RecentRuns: runs})
				model.message = "underlying notice"
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
				committed := slices.Clone(model.committed)
				model = entry.open(t, model)

				if model.overlay == nil || model.overlay.content != viewRuns {
					t.Fatalf("overlay=%#v, want Runs", model.overlay)
				}
				if !reflect.DeepEqual(model.overlay.composer, entry.want) || model.composer.Active {
					t.Fatalf("composer=%#v saved=%#v, want inactive and %#v", model.composer, model.overlay.composer, entry.want)
				}
				requireLines(t, normalizedViewLines(model.View()), "Runs", "Recent Runs", "Keys: j/k Select | enter Open | r Refresh | esc Close | q Quit")
				assertMaxLineWidth(t, normalizedViewLines(model.View()), 80)
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 24})
				assertMaxLineWidth(t, normalizedViewLines(model.View()), 40)

				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
				if model.selectedRunID() != "run-b" {
					t.Fatalf("overlay selection=%q, want run-b", model.selectedRunID())
				}
				model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) || model.message != "underlying notice" {
					t.Fatalf("restored state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
				}
				if !reflect.DeepEqual(model.committed, committed) {
					t.Fatal("Runs overlay changed committed transcript cells")
				}
			})
		}
	})

	t.Run("refresh preserves stable run identity with first-run fallback", func(t *testing.T) {
		refreshes := 0
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: runs}, StatusActions{
			RefreshStatus: func() (app.StatusResult, error) {
				refreshes++
				switch refreshes {
				case 1:
					return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{runs[2], runs[1], runs[0]}}, nil
				case 2:
					return app.StatusResult{Initialized: true, RecentRuns: []ledger.Run{runs[2], runs[0]}}, nil
				default:
					return app.StatusResult{}, errors.New("offline")
				}
			},
		})
		model.message = "underlying notice"
		model.composer.Active = false
		model, _ = updateStatusModel(t, model, keyRunes("3"))
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})

		for _, want := range []string{"run-b", "run-c"} {
			var cmd tea.Cmd
			model, cmd = updateStatusModel(t, model, keyRunes("r"))
			model, cmd = runStatusModelCmd(t, model, cmd)
			model = drainStatusModelCmds(t, model, cmd)
			if model.selectedRunID() != want {
				t.Fatalf("refresh %d selection=%q, want %q", refreshes, model.selectedRunID(), want)
			}
		}
		model, cmd := updateStatusModel(t, model, keyRunes("r"))
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || model.overlay == nil {
			t.Fatalf("failed refresh dismissed overlay: overlay=%#v cmd=%v", model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.View()), "Notice: Refresh failed: offline")
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.message != "underlying notice" {
			t.Fatalf("underlying notice = %q, want preserved", model.message)
		}
	})
}

func TestRunDetailOverlay(t *testing.T) {
	runs := make([]ledger.Run, 24)
	for i := range runs {
		runs[i] = ledger.Run{ID: fmt.Sprintf("run-%02d", i), Status: ledger.StatusCompleted, Summary: "complete"}
	}
	history := ledger.RunWithEvents{
		Run:    ledger.Run{ID: "run-12", TaskID: "task-detail", Task: "Inspect overlay detail", Status: ledger.StatusCompleted, Summary: "complete"},
		Events: []ledger.Event{{ID: 1, RunID: "run-12", Type: ledger.EventRunStarted}},
	}

	t.Run("direct entries construct the parent before the existing empty detail", func(t *testing.T) {
		for _, entry := range []struct {
			name string
			open func(t *testing.T, model StatusModel) StatusModel
			want commandComposerState
		}{
			{
				name: "key",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model.composer = commandComposerState{Text: "saved draft"}
					model, _ = updateStatusModel(t, model, keyRunes("4"))
					return model
				},
				want: commandComposerState{Text: "saved draft"},
			},
			{
				name: "command",
				open: func(t *testing.T, model StatusModel) StatusModel {
					model, _ = updateStatusModel(t, model, keyRunes("/detail"))
					model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
					return model
				},
				want: commandComposerState{Active: true},
			},
		} {
			t.Run(entry.name, func(t *testing.T) {
				model := NewStatusModel(app.StatusResult{Initialized: true, RecentRuns: runs[:2]})
				model.message = "underlying notice"
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
				model = entry.open(t, model)
				if model.overlay == nil || model.overlay.content != viewRunDetail {
					t.Fatalf("overlay=%#v, want direct Run Detail", model.overlay)
				}
				requireLines(t, normalizedViewLines(model.View()), "Run Detail", "No run detail loaded.", "Selected run: run-00", "Keys: up/down Scroll | home/end Jump | enter Reload | v Validate | r Refresh")
				assertMaxLineWidth(t, normalizedViewLines(model.View()), 80)
				model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 24})
				assertMaxLineWidth(t, normalizedViewLines(model.View()), 40)

				model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if model.overlay == nil || model.overlay.content != viewRuns || model.selectedRunID() != "run-00" {
					t.Fatalf("detail back state: overlay=%#v selection=%q", model.overlay, model.selectedRunID())
				}
				model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
				if cmd != nil || model.overlay != nil || !reflect.DeepEqual(model.composer, entry.want) || model.message != "underlying notice" {
					t.Fatalf("root close state: overlay=%#v composer=%#v message=%q cmd=%v", model.overlay, model.composer, model.message, cmd)
				}
			})
		}
	})

	t.Run("selection offset detail evidence and validation survive child back", func(t *testing.T) {
		openedRunID := ""
		validatedRunID := ""
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: runs}, StatusActions{
			OpenRun: func(runID string) (ledger.RunWithEvents, error) {
				openedRunID = runID
				return history, nil
			},
			ValidateReceipt: func(runID string) (receipt.ValidationResult, error) {
				validatedRunID = runID
				return receipt.ValidationResult{RunID: runID, ReceiptPath: ".revolvr/receipts/run-12.md", Checks: []receipt.ValidationCheck{{Name: receipt.ValidationCheckIdentity, Passed: true}}}, nil
			},
		})
		model.composer = commandComposerState{Text: "saved draft"}
		model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
		model, _ = updateStatusModel(t, model, keyRunes("3"))
		for range 12 {
			model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnd})
		parentOffset := model.overlay.viewport.YOffset
		if parentOffset == 0 || model.selectedRunID() != "run-12" {
			t.Fatalf("parent offset=%d selection=%q, want scrolled run-12", parentOffset, model.selectedRunID())
		}

		model, cmd := updateStatusModel(t, model, keyRunes("o"))
		if cmd == nil || openedRunID != "" {
			t.Fatalf("open cmd=%v callback run=%q", cmd, openedRunID)
		}
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || openedRunID != "run-12" || model.overlay == nil || model.overlay.content != viewRunDetail || model.overlay.parentOffset != parentOffset {
			t.Fatalf("opened detail: run=%q overlay=%#v cmd=%v", openedRunID, model.overlay, cmd)
		}
		requireLines(t, normalizedViewLines(model.renderOverlayContent()), "Run Detail", "ID: run-12", "Task: Inspect overlay detail")

		model, cmd = updateStatusModel(t, model, keyRunes("v"))
		if cmd == nil || validatedRunID != "" {
			t.Fatalf("validation cmd=%v callback run=%q", cmd, validatedRunID)
		}
		model, cmd = runStatusModelCmd(t, model, cmd)
		if cmd != nil || validatedRunID != "run-12" || !model.validation.Result.Passed() {
			t.Fatalf("validation run=%q state=%#v cmd=%v", validatedRunID, model.validation, cmd)
		}
		if !strings.Contains(model.renderOverlayContent(), "Notice: Receipt validation passed.") {
			t.Fatalf("validation notice missing from detail:\n%s", model.renderOverlayContent())
		}

		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay == nil || model.overlay.content != viewRuns || model.selectedRunID() != "run-12" || model.overlay.viewport.YOffset != parentOffset {
			t.Fatalf("child back: overlay=%#v selection=%q, want run-12 at offset %d", model.overlay, model.selectedRunID(), parentOffset)
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		if model.overlay != nil || model.composer.Text != "saved draft" {
			t.Fatalf("root return: overlay=%#v composer=%#v", model.overlay, model.composer)
		}
	})

	t.Run("late open result cannot replace a newer overlay owner", func(t *testing.T) {
		model := NewStatusModelWithActions(app.StatusResult{Initialized: true, RecentRuns: runs[:1]}, StatusActions{
			OpenRun: func(string) (ledger.RunWithEvents, error) { return history, nil },
		})
		model.composer.Active = false
		model, _ = updateStatusModel(t, model, keyRunes("3"))
		model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("open returned nil cmd")
		}
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
		model, _ = updateStatusModel(t, model, keyRunes("4"))
		owner := model.overlay.owner
		model, staleCmd := runStatusModelCmd(t, model, cmd)
		if staleCmd != nil || model.overlay == nil || model.overlay.owner != owner || model.overlay.content != viewRunDetail || model.runDetails != nil {
			t.Fatalf("stale result changed owner: overlay=%#v detail=%#v cmd=%v", model.overlay, model.runDetails, staleCmd)
		}
	})
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

func TestStatusModelWideNarrowGeometryMatrix(t *testing.T) {
	status := app.StatusResult{
		Initialized: true,
		ProjectRoot: "/home/alex/source/revolvr",
		Tasks:       []taskmodel.Task{{ID: "task-017", Status: taskmodel.StatusPending, Summary: "Compact durable agent state", NextRunnable: true}},
		RecentRuns:  []ledger.Run{{ID: "run-017", TaskID: "task-017", Task: "Compact durable agent state", Status: ledger.StatusCompleted, VerificationStatus: "passed"}},
	}
	newModel := func() StatusModel {
		model := NewStatusModel(status)
		projection := tuiAutonomousView("task-017", "needs_input")
		projection.Input = autonomousview.OperatorInput{
			State:                   "waiting",
			QuestionID:              "verification-scope",
			Revision:                1,
			ContentSHA256:           strings.Repeat("c", 64),
			Question:                "Choose the verification scope.",
			BlockingReason:          "The task requires an exact scope.",
			Options:                 []autonomousview.InputOption{{ID: "focused", Meaning: "Run package tests and preserve the exact selected option identity."}, {ID: "full", Meaning: "Run all tests."}},
			RecommendationOption:    "focused",
			RecommendationRationale: "Use the smallest sufficient check.",
		}
		model.autonomous.View = &projection
		model.autonomous.TaskID = projection.Identity.TaskID
		model.autonomous.Selector = projection.Identity.TaskID
		model.runDetails = &ledger.RunWithEvents{Run: status.RecentRuns[0]}
		model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
		return model
	}

	states := []struct {
		name  string
		model func() StatusModel
		want  []string
	}{
		{name: "ready", model: func() StatusModel {
			readyStatus := status
			readyStatus.RecentRuns = nil
			return NewStatusModel(readyStatus)
		}, want: []string{"Ready", "Next task: task-017 - Compact durable agent state"}},
		{name: "uninitialized", model: func() StatusModel { return NewStatusModel(app.StatusResult{}) }, want: []string{"Not initialized", "Next: run revolvr init in this repository"}},
		{name: "running", model: func() StatusModel {
			model := newModel()
			model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeLoop, Task: "task-017", MaxPasses: 3, Current: "Running go test ./..."}
			model.updateViewportContent()
			return model
		}, want: []string{"Safety: admitted", "Current: Running go test ./...", "Next: wait, or press c or Esc to cancel"}},
		{name: "cancelling", model: func() StatusModel {
			model := newModel()
			model.runOnce = runOnceState{Active: true, Started: true, CancelRequested: true, Mode: runModeLoop, Task: "task-017"}
			model.updateViewportContent()
			return model
		}, want: []string{"Cancelling: task-017", "Current: waiting for the run to stop", "Next: wait for settlement"}},
		{name: "composer", model: func() StatusModel {
			model := newModel()
			model.composer.Text = "draft task"
			model.updateViewportContent()
			return model
		}, want: []string{"› draft task"}},
		{name: "discovery", model: func() StatusModel {
			model := newModel()
			model.composer = commandComposerState{Active: true, Text: "/", DiscoveryOpen: true, SelectedCommand: len(slashCommands) - 1}
			model.updateViewportContent()
			return model
		}, want: []string{"> /quit — Quit", "› /"}},
	}
	for _, state := range states {
		for _, width := range []int{80, 40} {
			t.Run(fmt.Sprintf("state/%s/%d", state.name, width), func(t *testing.T) {
				model, cmd := updateStatusModel(t, state.model(), tea.WindowSizeMsg{Width: width, Height: 24})
				if cmd != nil {
					t.Fatalf("resize command = %v, want nil", cmd)
				}
				lines := normalizedViewLines(model.View())
				assertMaxLineWidth(t, lines, width)
				visible := strings.Join(strings.Fields(strings.Join(lines, "\n")), " ")
				for _, want := range state.want {
					if !strings.Contains(visible, want) {
						t.Fatalf("view missing %q: %#v", want, lines)
					}
				}
				for _, cell := range model.committed {
					assertMaxLineWidth(t, normalizedViewLines(strings.Join(cell.render(width), "\n")), width)
				}
			})
		}
	}

	terminalReasons := map[string]string{
		"failed": "verification failed", "blocked": "dependency task-016 is pending",
		"safety_stop": "protected path changed", "needs_input": "Choose the verification scope",
	}
	for _, outcome := range []string{"completed", "failed", "cancelled", "blocked", "safety_stop", "needs_input"} {
		for _, width := range []int{80, 40} {
			t.Run(fmt.Sprintf("terminal/%s/%d", outcome, width), func(t *testing.T) {
				cell := terminalTranscriptCell("terminal-1", "task-017", outcome, terminalReasons[outcome])
				lines := normalizedViewLines(strings.Join(cell.render(width), "\n"))
				assertMaxLineWidth(t, lines, width)
				if got, want := strings.Join(strings.Fields(strings.Join(lines, "\n")), " "), strings.Join(strings.Fields(strings.Join(cell.source, "\n")), " "); got != want {
					t.Fatalf("wrapped terminal meaning = %q, want %q", got, want)
				}
			})
		}
	}

	overlays := []struct {
		name  string
		view  TUIView
		want  string
		setup func(*StatusModel)
	}{
		{name: "help", view: viewHelp, want: "Help"},
		{name: "tasks", view: viewTasks, want: "Tasks"},
		{name: "task entry", view: viewTaskEntry, want: "Add Task", setup: func(model *StatusModel) { model.taskEntry.taskText = "Write geometry tests" }},
		{name: "runs", view: viewRuns, want: "Runs"},
		{name: "run detail", view: viewRunDetail, want: "Run Detail"},
		{name: "preflight", view: viewPreflight, want: "Preflight"},
		{name: "workflow", view: viewAutonomous, want: "Autonomous Workflow"},
		{name: "change summary", view: viewDiff, want: "Change Summary"},
		{name: "evidence", view: viewEvidence, want: "Evidence"},
		{name: "approval", view: viewApproval, want: "Approval"},
		{name: "needs input", view: viewNeedsInput, want: "Needs Input", setup: func(model *StatusModel) {
			model.autonomous.Answer = autonomousAnswerState{Active: true, Selected: 0}
		}},
	}
	for _, overlay := range overlays {
		for _, width := range []int{80, 40} {
			t.Run(fmt.Sprintf("overlay/%s/%d", overlay.name, width), func(t *testing.T) {
				model := newModel()
				if model.Init() == nil {
					t.Fatal("startup append command is nil")
				}
				if overlay.setup != nil {
					overlay.setup(&model)
				}
				model.openOverlay(overlay.view, 0)
				model, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 24})
				if cmd != nil {
					t.Fatalf("resize command = %v, want nil", cmd)
				}
				lines := normalizedViewLines(model.View())
				assertMaxLineWidth(t, lines, width)
				requireLines(t, lines, overlay.want)
				if overlay.view == viewNeedsInput {
					visible := strings.Join(strings.Fields(strings.Join(lines, "\n")), " ")
					if !strings.Contains(visible, "> focused: Run package tests and preserve the exact selected option identity.") {
						t.Fatalf("selected typed option was truncated: %#v", lines)
					}
				}
				if countTranscriptCells(model.committed, "session-start") != 1 || model.appendCommitted() != nil {
					t.Fatalf("overlay changed or replayed session history: committed=%#v emitted=%#v", model.committed, model.emitted)
				}
				model.closeOverlay()
				if model.overlay != nil || countTranscriptCells(model.committed, "session-start") != 1 || model.appendCommitted() != nil {
					t.Fatalf("overlay dismissal changed or replayed session history: overlay=%#v committed=%#v emitted=%#v", model.overlay, model.committed, model.emitted)
				}
			})
		}
	}
}

func TestStatusModelResizeRetainsGeometryAndSession(t *testing.T) {
	status := app.StatusResult{
		Initialized: true,
		ProjectRoot: "/home/alex/source/revolvr",
		RecentRuns:  []ledger.Run{{ID: "run-017", Task: "Compact durable agent state", Status: ledger.StatusCompleted, VerificationStatus: "passed"}},
	}
	model := NewStatusModel(status)
	if model.Init() == nil {
		t.Fatal("startup append command is nil")
	}
	wantIdentities := make([]string, len(model.committed))
	for i, cell := range model.committed {
		wantIdentities[i] = cell.identity
	}
	model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeLoop, Task: "Compact durable agent state", MaxPasses: 3, Current: "Running go test ./..."}
	model.composer = commandComposerState{Active: true, Text: "/", DiscoveryOpen: true, SelectedCommand: len(slashCommands) - 1}
	model.updateViewportContent()

	for _, width := range []int{80, 40, 24, 80} {
		var cmd tea.Cmd
		model, cmd = updateStatusModel(t, model, tea.WindowSizeMsg{Width: width, Height: 24})
		if cmd != nil {
			t.Fatalf("resize to %d returned command %v, want nil", width, cmd)
		}
		gotIdentities := make([]string, len(model.committed))
		for i, cell := range model.committed {
			gotIdentities[i] = cell.identity
			rendered := normalizedViewLines(strings.Join(cell.render(width), "\n"))
			assertMaxLineWidth(t, rendered, width)
			if got, want := strings.Join(strings.Fields(strings.Join(rendered, "\n")), ""), strings.Join(strings.Fields(strings.Join(cell.source, "\n")), ""); got != want {
				t.Fatalf("resize to %d changed committed meaning to %q, want %q", width, got, want)
			}
		}
		if !reflect.DeepEqual(gotIdentities, wantIdentities) || countTranscriptCells(model.committed, "session-start") != 1 || model.appendCommitted() != nil {
			t.Fatalf("resize to %d changed or replayed committed history: identities=%#v emitted=%#v", width, gotIdentities, model.emitted)
		}
		if model.runOnce.Current != "Running go test ./..." || model.composer.Text != "/" || model.composer.SelectedCommand != len(slashCommands)-1 {
			t.Fatalf("resize to %d corrupted live/composer state: run=%#v composer=%#v", width, model.runOnce, model.composer)
		}
		assertMaxLineWidth(t, normalizedViewLines(model.View()), width)
	}

	for _, width := range []int{80, 40} {
		t.Run(fmt.Sprintf("lifecycle/%d", width), func(t *testing.T) {
			started := NewStatusModel(status)
			started, cmd := updateStatusModel(t, started, tea.WindowSizeMsg{Width: width, Height: 24})
			if cmd != nil || started.Init() == nil || started.committed[0].identity != "session-start" || countTranscriptCells(started.committed, "session-start") != 1 {
				t.Fatalf("startup history at width %d: committed=%#v cmd=%v", width, started.committed, cmd)
			}
			started, cmd = updateStatusModel(t, started, refreshStatusMsg{status: status})
			if cmd != nil || countTranscriptCells(started.committed, "session-start") != 1 || started.appendCommitted() != nil {
				t.Fatalf("refresh replayed session at width %d: committed=%#v emitted=%#v cmd=%v", width, started.committed, started.emitted, cmd)
			}
			started.openHelpOverlay()
			started.closeOverlay()
			if countTranscriptCells(started.committed, "session-start") != 1 || started.appendCommitted() != nil {
				t.Fatalf("overlay transition replayed session at width %d: committed=%#v emitted=%#v", width, started.committed, started.emitted)
			}

			restarted := NewStatusModel(status)
			restarted, cmd = updateStatusModel(t, restarted, tea.WindowSizeMsg{Width: width, Height: 24})
			if cmd != nil || restarted.Init() == nil || restarted.committed[0].identity != "session-start" || countTranscriptCells(restarted.committed, "session-start") != 1 {
				t.Fatalf("restart history at width %d: committed=%#v cmd=%v", width, restarted.committed, cmd)
			}
		})
	}
}

func TestCommandDiscoveryFindsAndExecutesEveryCommand(t *testing.T) {
	status := app.StatusResult{
		Initialized: true,
		Tasks: []taskmodel.Task{{
			ID:       "task-ready",
			Status:   taskmodel.StatusPending,
			Workflow: taskfile.WorkflowAutonomousV1,
		}},
	}
	actions := StatusActions{
		RefreshStatus: func() (app.StatusResult, error) { return status, nil },
		ValidateReceipt: func(string) (receipt.ValidationResult, error) {
			return receipt.ValidationResult{}, nil
		},
		Preflight: func() (app.PreflightResult, error) {
			return app.PreflightResult{Ready: true}, nil
		},
		RunOnce: func(context.Context, app.RunProgress) (runonce.Result, error) {
			return runonce.Result{}, nil
		},
		RunLoop: func(context.Context, int, app.RunProgress, app.RunPassFunc) (app.RunLoopResult, error) {
			return app.RunLoopResult{}, nil
		},
		RunTask: func(context.Context, string, int64, autonomoustaskrun.Progress) (autonomoustaskrun.Result, error) {
			return autonomoustaskrun.Result{}, nil
		},
		ListAutonomous: func() ([]app.AutonomousTaskSelector, error) { return nil, nil },
		AnswerInput: func(app.AnswerAutonomousInputRequest) (app.AnswerAutonomousInputResult, error) {
			return app.AnswerAutonomousInputResult{}, nil
		},
		RunQueue: func(context.Context, int64, int64, autonomousqueue.Progress) (autonomousqueue.Result, error) {
			return autonomousqueue.Result{}, nil
		},
	}
	question := autonomousview.View{
		Identity: autonomousview.Identity{SourceKind: autonomousview.SourceActive, TaskID: "task-ready"},
		Input: autonomousview.OperatorInput{
			State:         "waiting",
			QuestionID:    "question-one",
			Revision:      1,
			ContentSHA256: strings.Repeat("a", 64),
			Options:       []autonomousview.InputOption{{ID: "option-one", Meaning: "Use option one."}},
		},
	}

	for _, command := range slashCommands {
		t.Run(command.name, func(t *testing.T) {
			model := NewStatusModelWithActions(status, actions)
			model.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
			model.runDetails = &ledger.RunWithEvents{Run: ledger.Run{ID: "run-one"}}
			model.autonomous.View = &question
			model.autonomous.TaskID = "task-ready"
			if command.name == "cancel" {
				model.runOnce = runOnceState{Active: true, Started: true, Mode: runModeOnce, Cancel: func() {}}
			}

			text := "/" + command.name
			if command.name == "answer" {
				text += " option-one"
			}
			model, cmd := updateStatusModel(t, model, keyRunes(text))
			matches := model.filteredSlashCommands()
			if len(matches) == 0 || matches[0].name != command.name {
				t.Fatalf("exact match for %q = %#v", command.name, matches)
			}
			if !strings.Contains(model.renderHelp(), command.usage) {
				t.Fatalf("Help does not include %q", command.usage)
			}

			model, cmd = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
			if model.composer.DiscoveryOpen || strings.HasPrefix(model.message, "Unknown command:") {
				t.Fatalf("command %q did not dispatch: composer=%#v message=%q cmd=%v", command.name, model.composer, model.message, cmd)
			}
			if model.runOnce.Cancel != nil {
				model.runOnce.Cancel()
			}
		})
	}
}

func TestCommandDiscoveryFiltersSelectsAndUsesCommandGuards(t *testing.T) {
	model := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunOnce: func(context.Context, app.RunProgress) (runonce.Result, error) {
			return runonce.Result{}, nil
		},
	})
	model, _ = updateStatusModel(t, model, keyRunes("/ta"))
	lines := normalizedViewLines(model.View())
	requireLines(t, lines,
		"Commands 1-2 of 2 · ↑/↓ select · Esc close",
		"> /tasks — Open Tasks",
		"  /task-run — [disabled] Autonomous task run is unavailable.",
	)

	model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
	requireLines(t, normalizedViewLines(model.View()), "> /task-run — [disabled] Autonomous task run is unavailable.")
	model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyUp})
	model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || model.overlay == nil || model.overlay.content != viewTasks || model.composer.DiscoveryOpen || model.composer.Text != "" {
		t.Fatalf("selected command state: overlay=%#v composer=%#v cmd=%v", model.overlay, model.composer, cmd)
	}

	guarded := NewStatusModelWithActions(app.StatusResult{Initialized: true}, StatusActions{
		RunOnce: func(context.Context, app.RunProgress) (runonce.Result, error) {
			return runonce.Result{}, nil
		},
	})
	guarded, _ = updateStatusModel(t, guarded, keyRunes("/run"))
	requireLines(t, normalizedViewLines(guarded.View()), "> /run — [disabled] Run blocked: preflight is not ready.")
	guarded, cmd = updateStatusModel(t, guarded, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || guarded.composer.Text != "/run" || !guarded.composer.DiscoveryOpen || guarded.message != guarded.runStartBlocker(runModeOnce) {
		t.Fatalf("guarded command state: composer=%#v message=%q cmd=%v", guarded.composer, guarded.message, cmd)
	}

	guarded.preflight = preflightState{Checked: true, Result: app.PreflightResult{Ready: true}}
	requireLines(t, normalizedViewLines(guarded.View()), "> /run — Run once")
	guarded, cmd = updateStatusModel(t, guarded, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !guarded.runOnce.Active || guarded.composer.DiscoveryOpen {
		t.Fatalf("enabled command state: run=%#v composer=%#v cmd=%v", guarded.runOnce, guarded.composer, cmd)
	}
	guarded.runOnce.Cancel()
}

func TestCommandDiscoveryEscapePreservesComposerBufferAndFocus(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model, _ = updateStatusModel(t, model, keyRunes("/ta"))
	model, cmd := updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !model.composer.Active || model.composer.DiscoveryOpen || model.composer.Text != "/ta" {
		t.Fatalf("closed discovery state: composer=%#v cmd=%v", model.composer, cmd)
	}
	requireNoLine(t, normalizedViewLines(model.View()), "Commands 1-2 of 2 · ↑/↓ select · Esc close")

	model, _ = updateStatusModel(t, model, keyRunes("s"))
	if !model.composer.DiscoveryOpen || model.composer.Text != "/tas" {
		t.Fatalf("typing did not reopen discovery: composer=%#v", model.composer)
	}
	requireLines(t, normalizedViewLines(model.View()), "> /tasks — Open Tasks")
}

func TestCommandDiscoveryNarrowWindowKeepsSelectionAndCancellationVisible(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})
	model.runOnce = runOnceState{
		Active:          true,
		Started:         true,
		CancelRequested: true,
		Mode:            runModeLoop,
		Status:          "running",
		MaxPasses:       3,
		Stats:           app.RunLoopStats{MaxPasses: 3},
		Logs:            []string{"system: cancellation requested"},
	}
	model.updateViewportContent()
	model, _ = updateStatusModel(t, model, tea.WindowSizeMsg{Width: 40, Height: 24})
	model, _ = updateStatusModel(t, model, keyRunes("/"))
	for range slashCommands {
		model, _ = updateStatusModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
	}

	if got := len(model.commandDiscoveryLines()); got != maxCommandRows+1 {
		t.Fatalf("popup rows = %d, want %d", got, maxCommandRows+1)
	}
	lines := normalizedViewLines(model.View())
	assertMaxLineWidth(t, lines, 40)
	requireLines(t, lines,
		"Cancelling: none",
		"Current: waiting for the run to stop",
		"Next: wait for settlement",
		"> /quit — Quit",
	)
}

func TestComposerFocusAndEscapeStateTable(t *testing.T) {
	model := NewStatusModel(app.StatusResult{Initialized: true})

	wide, cmd := updateStatusModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Fatalf("window size update cmd = %v, want nil", cmd)
	}
	requireLines(t, normalizedViewLines(wide.View()), "Ready", "Next: type a task or use /run")

	narrow, cmd := updateStatusModel(t, wide, tea.WindowSizeMsg{Width: 40, Height: 24})
	if cmd != nil {
		t.Fatalf("narrow window size update cmd = %v, want nil", cmd)
	}
	if got := len(narrow.footerLines()); got != 3 {
		t.Fatalf("narrow composer rows = %d, want 3", got)
	}
	lines := normalizedViewLines(narrow.View())
	requireLines(t, lines, "›", "Enter submit · / commands", "? shortcuts")
	assertMaxLineWidth(t, lines, 40)

	populated, cmd := updateStatusModel(t, narrow, keyRunes("draft task"))
	if cmd != nil || !populated.composer.Active || populated.composer.Text != "draft task" {
		t.Fatalf("populated composer=%#v cmd=%v", populated.composer, cmd)
	}
	preserved, cmd := updateStatusModel(t, populated, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !preserved.composer.Active || preserved.composer.Text != "draft task" {
		t.Fatalf("plain submit state=%#v cmd=%v", preserved.composer, cmd)
	}
	preserved, cmd = updateStatusModel(t, preserved, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !preserved.composer.Active || preserved.composer.Text != "draft task" {
		t.Fatalf("populated escape state=%#v cmd=%v", preserved.composer, cmd)
	}

	empty := NewStatusModel(app.StatusResult{Initialized: true})
	empty, cmd = updateStatusModel(t, empty, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || empty.composer.Active {
		t.Fatalf("empty escape state=%#v cmd=%v", empty.composer, cmd)
	}
	requireLines(t, normalizedViewLines(empty.View()), "›", "Enter submit · / commands · ? shortcuts")

	command, _ := updateStatusModel(t, empty, keyRunes("/"))
	command, _ = updateStatusModel(t, command, keyRunes("tasks"))
	submitted, cmd := updateStatusModel(t, command, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || submitted.composer.Active || submitted.composer.Text != "" || submitted.overlay == nil || submitted.overlay.content != viewTasks {
		t.Fatalf("submitted command state=%#v overlay=%#v cmd=%v", submitted.composer, submitted.overlay, cmd)
	}

	popup := NewStatusModel(app.StatusResult{Initialized: true})
	popup, _ = updateStatusModel(t, popup, keyRunes("/"))
	popup, cmd = updateStatusModel(t, popup, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || popup.composer.Active || popup.composer.Text != "/" || popup.overlay == nil || popup.overlay.content != viewHelp {
		t.Fatalf("command help state=%#v overlay=%#v cmd=%v", popup.composer, popup.overlay, cmd)
	}
	restored, cmd := updateStatusModel(t, popup, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !restored.composer.Active || restored.composer.Text != "/" || restored.overlay != nil {
		t.Fatalf("command help return=%#v overlay=%#v cmd=%v", restored.composer, restored.overlay, cmd)
	}

	nonComposer := NewStatusModel(app.StatusResult{Initialized: true})
	nonComposer.composer.Text = "saved draft"
	nonComposer.openChangeSummaryOverlay()
	nonComposer, cmd = updateStatusModel(t, nonComposer, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || !nonComposer.composer.Active || nonComposer.composer.Text != "saved draft" {
		t.Fatalf("focused view return=%#v cmd=%v", nonComposer.composer, cmd)
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

func TestStyleAccessibility(t *testing.T) {
	roles := []struct {
		name       string
		style      lipgloss.Style
		foreground lipgloss.TerminalColor
		bold       bool
		faint      bool
	}{
		{name: "section", style: sectionStyle, foreground: lipgloss.NoColor{}, bold: true},
		{name: "selected", style: selectedStyle, foreground: lipgloss.Color("6"), bold: true},
		{name: "success", style: successStyle, foreground: lipgloss.Color("2")},
		{name: "warning", style: warningStyle, foreground: lipgloss.NoColor{}, bold: true},
		{name: "danger", style: dangerStyle, foreground: lipgloss.Color("1")},
		{name: "muted", style: mutedStyle, foreground: lipgloss.NoColor{}, faint: true},
	}
	for _, role := range roles {
		if got := role.style.GetForeground(); !reflect.DeepEqual(got, role.foreground) {
			t.Errorf("%s foreground = %#v, want %#v", role.name, got, role.foreground)
		}
		if got := role.style.GetBackground(); !reflect.DeepEqual(got, lipgloss.NoColor{}) {
			t.Errorf("%s background = %#v, want terminal default", role.name, got)
		}
		if got := role.style.GetBold(); got != role.bold {
			t.Errorf("%s bold = %t, want %t", role.name, got, role.bold)
		}
		if got := role.style.GetFaint(); got != role.faint {
			t.Errorf("%s faint = %t, want %t", role.name, got, role.faint)
		}
	}

	want := []string{
		"> task-017  pending",
		"Status: completed",
		"Status: failed",
		"Warning: evidence incomplete",
		"Safety: admitted",
		"Current: running verification",
		"Cancelled: task-017",
		"Needs input: task-017",
		"! /run — disabled: run already active",
		"› /run",
	}
	styled := []string{
		styleContentLine(want[0]),
		styleContentLine(want[1]),
		styleContentLine(want[2]),
		styleContentLine(want[3]),
		styleContentLine(want[4]),
		styleContentLine(want[5]),
		styleContentLine(want[6]),
		styleContentLine(want[7]),
		styleContentLine(want[8]),
		styleFooterLines(want[9:])[0],
	}
	if got := normalizedViewLines(strings.Join(styled, "\n")); !reflect.DeepEqual(got, want) {
		t.Fatalf("text-only semantic states = %#v, want %#v", got, want)
	}

	if os.Getenv("REVOLVR_TEST_NO_COLOR") == "1" {
		if strings.Contains(strings.Join(styled, "\n"), "\x1b[") {
			t.Fatal("NO_COLOR rendering contains an ANSI style escape")
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStyleAccessibility$")
	cmd.Env = append(os.Environ(), "REVOLVR_TEST_NO_COLOR=1", "NO_COLOR=1", "CLICOLOR_FORCE=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("NO_COLOR subprocess failed: %v\n%s", err, output)
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
	resized.openTasksOverlay()
	return resized
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
	model.runDetails = &history
	model.width = width
	model.height = height
	model.openRunDetailOverlay()
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
