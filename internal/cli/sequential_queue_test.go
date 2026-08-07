package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"revolvr/internal/app"
	"revolvr/internal/queue"
)

const testQueueOperationID = "019c3a64-6c00-7000-8000-000000000023"

func TestSequentialQueueStartStatusAndCancelCommands(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{
		Out: &out, WorkDir: t.TempDir(),
		StartSequentialQueue: func(_ context.Context, _ app.Config, input app.SequentialQueueStartInput) (queue.Result, error) {
			if input.OperationID != testQueueOperationID || input.CoordinatorIdentity != "queue-cli:"+testQueueOperationID ||
				input.Limits.MaximumTasks != 2 || input.Limits.MaximumCyclesPerTask != 3 ||
				input.Limits.MaximumTotalCycles != 4 || input.Limits.MaximumRemoteTokens != 5 ||
				input.Limits.MaximumCostMicrousd != 6 || input.Limits.MaximumDuration != 7*time.Minute ||
				input.QualityGateStatus != queue.QualityGateDeterministicOnly {
				t.Fatalf("input = %#v", input)
			}
			return queue.Result{
				OperationID: testQueueOperationID, StopReason: queue.StopBudgetExhausted,
				TasksStarted: 1, CyclesConsumed: 3, RemoteTokensConsumed: 5,
				CostMicrousdConsumed: 6, PeakSourceMutatingWorkers: 1,
				TerminalMarkerSHA256: strings.Repeat("a", 64),
				QualityGateStatus:    queue.QualityGateDeterministicOnly,
			}, nil
		},
	})
	root.SetArgs([]string{"queue", "start", "--operation-id", testQueueOperationID,
		"--max-tasks", "2", "--max-cycles-per-task", "3", "--max-total-cycles", "4",
		"--max-remote-tokens", "5", "--max-cost-microusd", "6", "--max-duration", "7m"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "stop=budget_exhausted tasks=1 cycles=3 tokens=5 cost_microusd=6 peak_workers=1") ||
		!strings.Contains(got, "gate=deterministic_evaluation_only") {
		t.Fatalf("start output = %q", got)
	}

	out.Reset()
	root = NewRootCommand(Options{Out: &out, WorkDir: t.TempDir(), SequentialQueueStatus: func(_ context.Context, _ app.Config, databaseURL, operationID string) (queue.Status, error) {
		if databaseURL != "" || operationID != testQueueOperationID {
			t.Fatalf("status arguments = %q/%q", databaseURL, operationID)
		}
		return queue.Status{
			OperationID: operationID, Status: "terminal", StopReason: queue.StopDrained,
			TerminalMarkerSHA256: strings.Repeat("b", 64),
			Configuration: queue.PinnedConfiguration{
				WorkerMode: queue.WorkerModeDirectTools, MaximumWorkers: 1,
				QualityGateStatus: queue.QualityGateDeterministicOnly,
				Limits: queue.Limits{MaximumTasks: 2, MaximumCyclesPerTask: 3,
					MaximumTotalCycles: 4, MaximumRemoteTokens: 5,
					MaximumCostMicrousd: 6, MaximumDuration: 7 * time.Minute},
			},
		}, nil
	}})
	root.SetArgs([]string{"queue", "status", testQueueOperationID})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "status=terminal stop=drained") ||
		!strings.Contains(got, "workers=1 mode=direct_tools_v1") {
		t.Fatalf("status output = %q", got)
	}

	out.Reset()
	root = NewRootCommand(Options{Out: &out, WorkDir: t.TempDir(), CancelSequentialQueue: func(_ context.Context, _ app.Config, databaseURL, operationID string) (queue.Status, bool, error) {
		return queue.Status{OperationID: operationID, Status: "active"}, false, nil
	}})
	root.SetArgs([]string{"queue", "cancel", testQueueOperationID})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Queue cancellation: operation="+testQueueOperationID+" replayed=false status=active\n" {
		t.Fatalf("cancel output = %q", got)
	}
}

func TestSequentialQueueStartRequiresStableFiniteAuthority(t *testing.T) {
	for _, args := range [][]string{
		{"queue", "start"},
		{"queue", "start", "--operation-id", testQueueOperationID, "--max-tasks", "0"},
		{"queue", "start", "--operation-id", testQueueOperationID, "--max-cycles-per-task", "5", "--max-total-cycles", "4"},
		{"queue", "start", "--operation-id", testQueueOperationID, "--max-duration", "0"},
	} {
		root := NewRootCommand(Options{Out: &bytes.Buffer{}, WorkDir: t.TempDir(), StartSequentialQueue: func(context.Context, app.Config, app.SequentialQueueStartInput) (queue.Result, error) {
			t.Fatal("runner called with invalid authority")
			return queue.Result{}, nil
		}})
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("args %v unexpectedly succeeded", args)
		}
	}
}

func TestSequentialQueueHelpDocumentsOnlyOneWorker(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Out: &out})
	root.SetArgs([]string{"queue", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"start", "status", "cancel", "bounded sequential"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("queue help missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "worker") || strings.Contains(out.String(), "daemon") {
		t.Fatalf("canonical queue help exposes parallel or daemon controls:\n%s", out.String())
	}
}
