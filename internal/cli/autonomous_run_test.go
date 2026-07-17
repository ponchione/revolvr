package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/app"
	"revolvr/internal/autonomousdaemon"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoustaskrun"
)

func TestRunUntilTerminalFlagsAndDeterministicSummary(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Out: &out, WorkDir: t.TempDir(), RunTaskUntilTerminal: func(_ context.Context, _ app.Config, input app.TaskRunInput) (autonomoustaskrun.Result, error) {
		if input.TaskID != "task-one" || input.OperationID != "operation-one" || input.MaxCycles != 3 {
			t.Fatalf("input=%+v", input)
		}
		return autonomoustaskrun.Result{OperationID: "operation-one", TaskID: "task-one", StopReason: autonomoustaskrun.StopNeedsInput, StopDetail: "choose an API", LastAction: "needs_input", LastDecisionID: "decision-2", LastRunID: "supervisor-2", Statistics: autonomoustaskrun.Statistics{CyclesStarted: 2, SupervisorStarted: 2, SupervisorCompleted: 2, AttemptsAdmitted: 1, AttemptsCompleted: 1, VerificationRuns: 1, Audits: 1, Actions: []autonomoustaskrun.ActionCount{{Action: "implement", Count: 1}}}}, nil
	}})
	root.SetArgs([]string{"run", "--until-terminal", "--task", "task-one", "--operation-id", "operation-one", "--max-cycles", "3"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "Task run: task=task-one operation=operation-one cycles=2/3 stop=needs_input replayed=false\nLast: action=needs_input decision=decision-2 run=supervisor-2\nStats: supervisors=2/2 attempts=1/1 verification=1 audits=1 corrections=0 optional=0 commits=0 checkpoints=0 actions=implement:1\nDetail: choose an API\n"
	if out.String() != want {
		t.Fatalf("output=%q want=%q", out.String(), want)
	}
}

func TestRunModesAreMutuallyExclusive(t *testing.T) {
	root := NewRootCommand(Options{Out: &bytes.Buffer{}, WorkDir: t.TempDir()})
	root.SetArgs([]string{"run", "--once", "--until-terminal"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunAutonomousFlagsRequireModeAndPositiveLimit(t *testing.T) {
	for _, args := range [][]string{{"run", "--task", "task-one"}, {"run", "--operation-id", "operation-one"}, {"run", "--max-cycles", "2"}} {
		root := NewRootCommand(Options{Out: &bytes.Buffer{}, WorkDir: t.TempDir()})
		root.SetArgs(args)
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "require --until-terminal, --queue, or --daemon") {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
	root := NewRootCommand(Options{Out: &bytes.Buffer{}, WorkDir: t.TempDir()})
	root.SetArgs([]string{"run", "--until-terminal", "--max-cycles", "0"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunQueueFlagsAndSummary(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Out: &out, WorkDir: t.TempDir(), RunQueue: func(_ context.Context, _ app.Config, input app.QueueInput) (autonomousqueue.Result, error) {
		if input.OperationID != "queue-one" || input.MaxTasks != 2 || input.MaxCycles != 3 || input.MaximumWorkers != 2 {
			t.Fatalf("input=%+v", input)
		}
		return autonomousqueue.Result{OperationID: "queue-one", Mode: autonomousqueue.ModeUntilExhausted, StopReason: autonomousqueue.StopWaitingInput, StopDetail: "operator answer required", MaximumWorkers: 2, Outcomes: []autonomousqueue.TaskOutcome{{SelectionSequence: 1, Batch: 1, Slot: 1, TaskID: "a", TaskOperationID: "task-run-a", StopReason: autonomoustaskrun.StopCompleted}, {SelectionSequence: 2, Batch: 1, Slot: 2, TaskID: "b", TaskOperationID: "task-run-b", StopReason: autonomoustaskrun.StopNeedsInput, StopDetail: "choose"}}, Statistics: autonomousqueue.Statistics{Selections: 2, TasksRun: 2, Batches: 1, PeakActiveWorkers: 2}, RemainingWaiting: []string{"b:needs_input"}}, nil
	}})
	root.SetArgs([]string{"run", "--queue", "--operation-id", "queue-one", "--max-tasks", "2", "--max-cycles", "3", "--workers", "2"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "Queue: operation=queue-one mode=until_exhausted stop=waiting_input replayed=false tasks=2 selections=2 workers=2 peak=2 batches=1 fallbacks=0\nTask: selection=1 batch=1 slot=1 id=a operation=task-run-a stop=completed replayed=false detail=\nTask: selection=2 batch=1 slot=2 id=b operation=task-run-b stop=needs_input replayed=false detail=choose\nRemaining: ready= waiting=b:needs_input\nDetail: operator answer required\n"
	if out.String() != want {
		t.Fatalf("output=%q want=%q", out.String(), want)
	}
}

func TestRunDaemonFlagsValidationAndSummary(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Out: &out, WorkDir: t.TempDir(), RunDaemon: func(_ context.Context, _ app.Config, input app.DaemonInput) (autonomousdaemon.Result, error) {
		if input.OperationID != "daemon-one" || input.MaximumWorkers != 3 || input.MaxSweeps != 4 || input.Poll != 2*time.Second || input.Debounce != time.Second {
			t.Fatalf("input=%+v", input)
		}
		return autonomousdaemon.Result{StopReason: autonomousdaemon.StopCancelled, StopDetail: "signal", Sweeps: 2, Wakes: []autonomousdaemon.Wake{{Generation: 2, Fingerprint: "abc"}}, LastFingerprint: "abc"}, nil
	}})
	root.SetArgs([]string{"run", "--daemon", "--operation-id", "daemon-one", "--workers", "3", "--max-sweeps", "4", "--daemon-poll", "2s", "--daemon-debounce", "1s"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Daemon: stop=cancelled sweeps=2 wakes=1 fingerprint=abc detail=signal\n" {
		t.Fatalf("output=%q", got)
	}

	for _, args := range [][]string{{"run", "--queue", "--daemon"}, {"run", "--queue", "--task", "x"}, {"run", "--queue", "--daemon-poll", "1s"}, {"run", "--daemon", "--max-sweeps", "0"}, {"run", "--queue", "--workers", "0"}, {"run", "--queue", "--workers", "5"}, {"run", "--queue", "--workers", "1", "--workers", "2"}} {
		cmd := NewRootCommand(Options{Out: &bytes.Buffer{}, WorkDir: t.TempDir()})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("args=%v unexpectedly succeeded", args)
		}
	}
}

func TestNotificationWarningRenderingAndReadOnlyInspection(t *testing.T) {
	workDir := t.TempDir()
	var out bytes.Buffer
	root := NewRootCommand(Options{Out: &out, WorkDir: workDir, RunTaskUntilTerminal: func(_ context.Context, _ app.Config, input app.TaskRunInput) (autonomoustaskrun.Result, error) {
		input.Notification(autonomousnotification.Result{DeliveryID: "delivery-one", Event: autonomousnotification.EventSafetyStop, Stage: autonomousnotification.StageFailed, Attempts: 2, Detail: "hook delivery failed"}, errors.New("bounded failure"))
		return autonomoustaskrun.Result{OperationID: "op", TaskID: "task", StopReason: autonomoustaskrun.StopSafety}, nil
	}})
	root.SetArgs([]string{"run", "--until-terminal", "--task", "task"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Notification warning: delivery=delivery-one event=safety_stop stage=failed attempts=2 detail=hook delivery failed error=bounded failure\n") {
		t.Fatalf("output=%q", out.String())
	}
	oldWorkDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkDir) })
	out.Reset()
	root = NewRootCommand(Options{Out: &out})
	root.SetArgs([]string{"notification", "list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "DELIVERY ID\tEVENT\tSTAGE\tATTEMPTS\tUPDATED AT\n" {
		t.Fatalf("list=%q", out.String())
	}
	if _, err := os.Stat(filepath.Join(workDir, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("read-only list created state: %v", err)
	}
	root = NewRootCommand(Options{Out: &out})
	root.SetArgs([]string{"notification", "show", "missing-delivery"})
	if err := root.Execute(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("show error=%v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("read-only show created state: %v", err)
	}
}
