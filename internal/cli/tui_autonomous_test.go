package cli

import (
	"context"
	"testing"

	"revolvr/internal/app"
	"revolvr/internal/autonomousqueue"
	tuiapp "revolvr/internal/tui"
)

func TestTUIRunnerReceivesAutonomousEvidenceInputTaskAndQueueCallbacks(t *testing.T) {
	queueCalled := false
	root := NewRootCommand(Options{
		WorkDir: t.TempDir(),
		RunQueue: func(_ context.Context, _ app.Config, input app.QueueInput) (autonomousqueue.Result, error) {
			queueCalled = true
			if input.MaxTasks != 100 || input.MaxCycles != 50 || input.MaximumWorkers != 1 || input.Progress == nil {
				t.Fatalf("queue input = %#v", input)
			}
			return autonomousqueue.Result{OperationID: "queue-tui", StopReason: autonomousqueue.StopDrained}, nil
		},
		TUIRunner: func(_ context.Context, _ app.StatusResult, opts tuiapp.RunOptions) error {
			if opts.ListAutonomous == nil || opts.LoadAutonomous == nil || opts.AnswerInput == nil || opts.RunTask == nil || opts.RunQueue == nil || opts.Preflight == nil || opts.RefreshStatus == nil {
				t.Fatalf("missing autonomous TUI callback: %#v", opts)
			}
			_, err := opts.RunQueue(context.Background(), 100, 50, func(autonomousqueue.Operation) {})
			return err
		},
	})
	root.SetArgs([]string{"tui"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !queueCalled {
		t.Fatal("injected queue runner was not called")
	}
}
