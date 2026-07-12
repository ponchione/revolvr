package autonomoustaskrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
)

func TestRunPinsOneTaskAndNeverStartsNext(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	writeTask(t, repo, "two", 2)
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	calls := 0
	result, err := RunTaskUntilTerminal(context.Background(), Config{
		RepositoryRoot: repo, OperationID: "run-one", ConfigSHA256: sha(), MaxCycles: Limited(5),
		Clock: func() time.Time { now = now.Add(time.Second); return now },
		Runner: func(_ context.Context, in StepInput) (StepResult, error) {
			calls++
			if in.Operation.TaskID != "one" {
				t.Fatalf("runner task = %q", in.Operation.TaskID)
			}
			if calls == 2 {
				return StepResult{StopReason: StopBlocked, StopDetail: "exact blocker", Action: "audit", Statistics: Statistics{Actions: []ActionCount{{Action: "audit", Count: 1}}}}, nil
			}
			return StepResult{Action: "implement", Statistics: Statistics{AttemptsAdmitted: 1, AttemptsCompleted: 1, Actions: []ActionCount{{Action: "implement", Count: 1}}}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "one" || result.StopReason != StopBlocked || calls != 2 {
		t.Fatalf("result=%+v calls=%d", result, calls)
	}
	if got := result.Statistics.Actions; len(got) != 2 || got[0].Action != "audit" || got[1].Action != "implement" {
		t.Fatalf("actions=%+v", got)
	}
	if _, err := os.Stat(filepath.Join(repo, ".revolvr", "autonomous", "task-runs", "run-one", "operation.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunMaxCyclesAndExactReplay(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	calls := 0
	cfg := Config{RepositoryRoot: repo, OperationID: "run-max", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(1), Clock: time.Now, Runner: func(context.Context, StepInput) (StepResult, error) { calls++; return StepResult{}, nil }}
	first, err := RunTaskUntilTerminal(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.StopReason != StopMaxCycles || calls != 1 {
		t.Fatalf("first=%+v calls=%d", first, calls)
	}
	second, err := RunTaskUntilTerminal(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || calls != 1 {
		t.Fatalf("second=%+v calls=%d", second, calls)
	}
	cfg.MaxCycles = Limited(2)
	if _, err := RunTaskUntilTerminal(context.Background(), cfg); err == nil {
		t.Fatal("changed limit succeeded")
	}
}

func TestRunUsesDependencyReadySelectionAndRejectsExplicitWaitingTask(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "dependency", 9)
	writeTask(t, repo, "waiting", 0)
	path := filepath.Join(repo, ".agent", "tasks", "waiting.md")
	raw, _ := os.ReadFile(path)
	raw = []byte(strings.Replace(string(raw), "priority: 0\n", "priority: 0\ndepends_on: dependency\n", 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "dependency-select", ConfigSHA256: sha(), MaxCycles: Limited(1), Clock: time.Now, Runner: func(_ context.Context, input StepInput) (StepResult, error) {
		calls++
		if input.Operation.TaskID != "dependency" {
			t.Fatalf("selected %q", input.Operation.TaskID)
		}
		return StepResult{}, nil
	}})
	if err != nil || result.TaskID != "dependency" || calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
	_, err = RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "dependency-explicit", TaskID: "waiting", ConfigSHA256: sha(), MaxCycles: Limited(1), Clock: time.Now, Runner: func(context.Context, StepInput) (StepResult, error) {
		t.Fatal("runner called")
		return StepResult{}, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "waiting_dependency") {
		t.Fatalf("explicit error=%v", err)
	}
}

func TestRunCancellationBeforeAdmissionDoesNotCreateOperation(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := RunTaskUntilTerminal(ctx, Config{RepositoryRoot: repo, OperationID: "cancelled", ConfigSHA256: sha(), MaxCycles: Unlimited(), Clock: time.Now, Runner: func(context.Context, StepInput) (StepResult, error) {
		t.Fatal("runner called")
		return StepResult{}, nil
	}})
	if err == nil || result.StopReason != StopOperationCancelled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestRunNoTaskNeedsNoStepRunner(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agent", "tasks"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "no-task", ConfigSHA256: sha(), MaxCycles: Limited(2), Clock: time.Now})
	if err != nil || result.StopReason != StopNoTask || result.OperationID != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestRunRedactsPersistentStopDetail(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	secret := "secret-value"
	result, err := RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "redacted", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(2), Clock: time.Now, Redact: func(v string) string { return strings.ReplaceAll(v, secret, "[REDACTED]") }, Runner: func(context.Context, StepInput) (StepResult, error) {
		return StepResult{StopReason: StopBlocked, StopDetail: "blocked by " + secret}, nil
	}})
	if err != nil || strings.Contains(result.StopDetail, secret) || !strings.Contains(result.StopDetail, "[REDACTED]") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".revolvr", "autonomous", "task-runs", "redacted", "operation.json"))
	if err != nil || strings.Contains(string(raw), secret) {
		t.Fatalf("checkpoint=%q err=%v", raw, err)
	}
}

func TestRunLedgerSummaryIsIdempotentAcrossReplay(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(context.Background(), filepath.Join(repo, ".revolvr", "ledger.db"), func() time.Time { now = now.Add(time.Second); return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := Config{RepositoryRoot: repo, OperationID: "ledger-run", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(2), Clock: func() time.Time { now = now.Add(time.Second); return now }, Ledger: store, Runner: func(context.Context, StepInput) (StepResult, error) {
		return StepResult{Action: "block", DecisionID: "decision-block", RunID: "supervisor-block", StopReason: StopBlocked, StopDetail: "exact blocker"}, nil
	}}
	if _, err := RunTaskUntilTerminal(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := RunTaskUntilTerminal(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	entry, found, err := store.GetRunWithEvents(context.Background(), loopLedgerRunID("ledger-run"))
	if err != nil || !found {
		t.Fatalf("ledger found=%v err=%v", found, err)
	}
	counts := map[ledger.EventType]int{}
	for _, event := range entry.Events {
		counts[event.Type]++
	}
	for _, eventType := range []ledger.EventType{ledger.EventTaskRunAdmitted, ledger.EventTaskRunCycleStarted, ledger.EventTaskRunCycleCompleted, ledger.EventTaskRunStopped} {
		if counts[eventType] != 1 {
			t.Fatalf("event %s count=%d events=%+v", eventType, counts[eventType], entry.Events)
		}
	}
	if entry.Run.Status != ledger.StatusCompleted || entry.Run.Summary != "task one stopped: blocked: exact blocker" {
		t.Fatalf("run=%+v", entry.Run)
	}
}

func TestFakeIntegrationTerminalMatrix(t *testing.T) {
	tests := []struct {
		name  string
		steps []StepResult
		stop  StopReason
	}{
		{name: "completion", stop: StopCompleted, steps: []StepResult{{Action: "plan", Statistics: Statistics{AttemptsAdmitted: 1, AttemptsCompleted: 1, Actions: []ActionCount{{Action: "plan", Count: 1}}}}, {Action: "implement", Statistics: Statistics{AttemptsAdmitted: 1, AttemptsCompleted: 1, VerificationRuns: 1, SourceCommits: 1, CheckpointAdvances: 1, Actions: []ActionCount{{Action: "implement", Count: 1}}}}, {Action: "audit", Statistics: Statistics{AttemptsAdmitted: 1, AttemptsCompleted: 1, Audits: 1, Actions: []ActionCount{{Action: "audit", Count: 1}}}}, {Action: "complete", StopReason: StopCompleted, StopDetail: "finalized"}}},
		{name: "correction", stop: StopCompleted, steps: []StepResult{{Action: "implement", Statistics: Statistics{AttemptsAdmitted: 1, AttemptsCompleted: 1, Actions: []ActionCount{{Action: "implement", Count: 1}}}}, {Action: "correct", Statistics: Statistics{AttemptsAdmitted: 1, AttemptsCompleted: 1, Corrections: 1, VerificationRuns: 2, Audits: 1, SourceCommits: 1, CheckpointAdvances: 1, Actions: []ActionCount{{Action: "correct", Count: 1}}}}, {Action: "complete", StopReason: StopCompleted}}},
		{name: "needs input", stop: StopNeedsInput, steps: []StepResult{{Action: "needs_input", StopReason: StopNeedsInput, StopDetail: "choose exact option"}}},
		{name: "explicit blocker", stop: StopBlocked, steps: []StepResult{{Action: "block", StopReason: StopBlocked, StopDetail: "external authority missing"}}},
		{name: "budget", stop: StopBudgetExhausted, steps: []StepResult{{Action: "implement", StopReason: StopBudgetExhausted, StopDetail: "task_attempts_exhausted"}}},
		{name: "safety", stop: StopSafety, steps: []StepResult{{Action: "implement", StopReason: StopSafety, StopDetail: "protected path drift"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			writeTask(t, repo, "one", 1)
			writeTask(t, repo, "two", 2)
			secondTask := filepath.Join(repo, ".agent", "tasks", "two.md")
			secondState := filepath.Join(repo, ".revolvr", "autonomous", "tasks", "two", "state.json")
			beforeTask, _ := os.ReadFile(secondTask)
			beforeState, _ := os.ReadFile(secondState)
			calls := 0
			result, err := RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "matrix-" + strings.ReplaceAll(test.name, " ", "-"), TaskID: "one", ConfigSHA256: sha(), MaxCycles: Unlimited(), Clock: time.Now, Runner: func(_ context.Context, input StepInput) (StepResult, error) {
				if input.Operation.TaskID != "one" || calls >= len(test.steps) {
					t.Fatalf("unexpected runner input=%+v calls=%d", input, calls)
				}
				step := test.steps[calls]
				calls++
				step.RunID = fmt.Sprintf("run-%d", calls)
				step.DecisionID = fmt.Sprintf("decision-%d", calls)
				return step, nil
			}})
			if err != nil || result.StopReason != test.stop || calls != len(test.steps) || result.Statistics.CyclesStarted != int64(len(test.steps)) || result.Statistics.CyclesCompleted != int64(len(test.steps)) {
				t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
			}
			afterTask, _ := os.ReadFile(secondTask)
			afterState, _ := os.ReadFile(secondState)
			if string(afterTask) != string(beforeTask) || string(afterState) != string(beforeState) {
				t.Fatal("second pending task changed")
			}
		})
	}
}

func TestRunCancellationDuringCyclePersistsOperationStop(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	ctx, cancel := context.WithCancel(context.Background())
	result, err := RunTaskUntilTerminal(ctx, Config{RepositoryRoot: repo, OperationID: "cancel-during", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Unlimited(), Clock: time.Now, Runner: func(context.Context, StepInput) (StepResult, error) {
		cancel()
		return StepResult{Action: "implement"}, context.Canceled
	}})
	if !errors.Is(err, context.Canceled) || result.StopReason != StopOperationCancelled || result.Statistics.CyclesStarted != 1 || result.Statistics.CyclesCompleted != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	replayed, replayErr := RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "cancel-during", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Unlimited(), Clock: time.Now})
	if replayErr != nil || !replayed.Replayed || replayed.StopReason != StopOperationCancelled {
		t.Fatalf("replayed=%+v err=%v", replayed, replayErr)
	}
}

func TestRunRestartsNonterminalCheckpointWithoutReselecting(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	writeTask(t, repo, "two", 2)
	cfg := Config{RepositoryRoot: repo, OperationID: "restart", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(3), Clock: time.Now}
	n, err := normalize(cfg)
	if err != nil {
		t.Fatal(err)
	}
	task, found, err := resolveTask(n.root, "one")
	if err != nil || !found {
		t.Fatal(err)
	}
	op, err := admit(n, task)
	if err != nil {
		t.Fatal(err)
	}
	op.Sequence, op.Stage = 1, "cycle_completed"
	op.Statistics = Statistics{CyclesStarted: 1, CyclesCompleted: 1, SupervisorStarted: 1, SupervisorCompleted: 1}
	if err := persist(n.root, Operation{}, op); err != nil {
		t.Fatal(err)
	}
	calls := 0
	cfg.Runner = func(_ context.Context, input StepInput) (StepResult, error) {
		calls++
		if input.Operation.TaskID != "one" || input.Cycle != 2 {
			t.Fatalf("input=%+v", input)
		}
		return StepResult{Action: "block", StopReason: StopBlocked}, nil
	}
	result, err := RunTaskUntilTerminal(context.Background(), cfg)
	if err != nil || result.TaskID != "one" || result.StopReason != StopBlocked || calls != 1 || result.Statistics.CyclesStarted != 2 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestRunRecoversAdmissionCrashFromImmutableHistory(t *testing.T) {
	for _, point := range []FailurePoint{FailureAfterOperationHistory, FailureBeforeOperationRename, FailureAfterOperationRename} {
		t.Run(string(point), func(t *testing.T) {
			repo := t.TempDir()
			writeTask(t, repo, "one", 1)
			fired, calls := false, 0
			cfg := Config{RepositoryRoot: repo, OperationID: "crash-" + string(point), TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(2), Clock: time.Now, FailureInjector: func(got FailurePoint) error {
				if !fired && got == point {
					fired = true
					return errors.New("injected crash")
				}
				return nil
			}, Runner: func(context.Context, StepInput) (StepResult, error) {
				calls++
				return StepResult{Action: "block", StopReason: StopBlocked}, nil
			}}
			if _, err := RunTaskUntilTerminal(context.Background(), cfg); err == nil || calls != 0 {
				t.Fatalf("first err=%v calls=%d", err, calls)
			}
			result, err := RunTaskUntilTerminal(context.Background(), cfg)
			if err != nil || result.StopReason != StopBlocked || calls != 1 {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestRunFailClosesAmbiguousInFlightCycleWithoutStartingWork(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	cfg := Config{RepositoryRoot: repo, OperationID: "ambiguous", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(2), Clock: time.Now}
	n, err := normalize(cfg)
	if err != nil {
		t.Fatal(err)
	}
	task, _, err := resolveTask(n.root, "one")
	if err != nil {
		t.Fatal(err)
	}
	op, err := admit(n, task)
	if err != nil {
		t.Fatal(err)
	}
	op.Sequence, op.Stage, op.InFlight = 1, "cycle_started", true
	op.Statistics = Statistics{CyclesStarted: 1, SupervisorStarted: 1}
	if err := persist(n.root, Operation{}, op); err != nil {
		t.Fatal(err)
	}
	cfg.Runner = func(context.Context, StepInput) (StepResult, error) {
		t.Fatal("ambiguous restart started external work")
		return StepResult{}, nil
	}
	result, err := RunTaskUntilTerminal(context.Background(), cfg)
	if err != nil || result.StopReason != StopUnsafeAmbiguous || result.Statistics.CyclesCompleted != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestProgressPanicCannotChangeControlFlow(t *testing.T) {
	repo := t.TempDir()
	writeTask(t, repo, "one", 1)
	result, err := RunTaskUntilTerminal(context.Background(), Config{RepositoryRoot: repo, OperationID: "progress-panic", TaskID: "one", ConfigSHA256: sha(), MaxCycles: Limited(1), Clock: time.Now, Progress: func(Operation) { panic("renderer failed") }, Runner: func(context.Context, StepInput) (StepResult, error) {
		return StepResult{StopReason: StopBlocked}, nil
	}})
	if err != nil || result.StopReason != StopBlocked {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func writeTask(t *testing.T, repo, id string, priority int) {
	t.Helper()
	task := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\npriority: %d\n---\n# %s\n", id, id, priority, id))
	path := filepath.Join(repo, ".agent", "tasks", id+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, task, 0o644); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: id, Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(repo, ".revolvr", "autonomous", "tasks", id, "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha() string { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }
