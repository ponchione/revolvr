package autonomousqueue

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

func TestRunMultipleTasksBlockedSkipAndStarvationPrevention(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	var mu sync.Mutex
	states := map[string]string{"high": "pending", "blocked": "pending", "later": "pending"}
	loader := func(context.Context) (Snapshot, error) {
		mu.Lock()
		defer mu.Unlock()
		nodes := []autonomousscheduler.Node{
			node("high", 0, states["high"]),
			node("blocked", 1, states["blocked"]),
			node("later", 2, states["later"]),
		}
		return Snapshot{Fingerprint: fingerprint(states), Nodes: nodes}, nil
	}
	var calls []string
	runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, in.TaskID)
		reason := autonomoustaskrun.StopCompleted
		switch in.TaskID {
		case "high":
			reason = autonomoustaskrun.StopMaxCycles
		case "blocked":
			reason = autonomoustaskrun.StopBlocked
			states[in.TaskID] = "blocked"
		default:
			states[in.TaskID] = "completed"
		}
		return taskResult(in, reason), nil
	}
	result, err := RunUntilExhausted(context.Background(), testConfig(root, "queue-one", clock, loader, runner))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "high,blocked,later" {
		t.Fatalf("calls=%s, want deterministic yield then unrelated tasks", got)
	}
	if result.StopReason != StopWaitingBlocked || len(result.Outcomes) != 3 {
		t.Fatalf("result=%+v", result)
	}
	if result.Outcomes[0].TaskOperationID == result.Outcomes[1].TaskOperationID {
		t.Fatal("derived task operations are not unique")
	}
	replay, err := RunUntilExhausted(context.Background(), testConfig(root, "queue-one", clock, loader, runner))
	if err != nil || !replay.Replayed || len(calls) != 3 {
		t.Fatalf("terminal replay=%+v err=%v calls=%v", replay, err, calls)
	}
}

func TestRunDependencyInputDrainedBudgetSafetyAndCancellation(t *testing.T) {
	tests := []struct {
		name      string
		nodes     []autonomousscheduler.Node
		maxTasks  int64
		runReason autonomoustaskrun.StopReason
		runErr    error
		want      StopReason
		wantCalls int
		wantErr   bool
	}{
		{name: "drained", nodes: []autonomousscheduler.Node{node("done", 0, "completed")}, want: StopDrained},
		{name: "dependency", nodes: []autonomousscheduler.Node{waitingNode("child", taskscheduler.ReasonWaitingDependency)}, want: StopWaitingDependency},
		{name: "input", nodes: []autonomousscheduler.Node{lifecycleNode("input", "needs_input")}, want: StopWaitingInput},
		{name: "safety", nodes: []autonomousscheduler.Node{node("task", 0, "pending")}, runReason: autonomoustaskrun.StopSafety, want: StopSafety, wantCalls: 1},
		{name: "unsafe", nodes: []autonomousscheduler.Node{node("task", 0, "pending")}, runReason: autonomoustaskrun.StopUnsafeAmbiguous, runErr: errors.New("indeterminate"), want: StopUnsafeAmbiguous, wantCalls: 1, wantErr: true},
		{name: "cancelled", nodes: []autonomousscheduler.Node{node("task", 0, "pending")}, runReason: autonomoustaskrun.StopOperationCancelled, runErr: context.Canceled, want: StopCancelled, wantCalls: 1, wantErr: true},
		{name: "queue budget", nodes: []autonomousscheduler.Node{node("one", 0, "pending"), node("two", 1, "pending")}, maxTasks: 1, runReason: autonomoustaskrun.StopCompleted, want: StopBudgetExhausted, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			clock := newClock()
			nodes := append([]autonomousscheduler.Node(nil), tt.nodes...)
			loader := func(context.Context) (Snapshot, error) {
				return Snapshot{Fingerprint: nodesFingerprint(nodes), Nodes: append([]autonomousscheduler.Node(nil), nodes...)}, nil
			}
			calls := 0
			runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
				calls++
				result := taskResult(in, tt.runReason)
				if tt.runReason == autonomoustaskrun.StopCompleted {
					for i := range nodes {
						if nodes[i].Task.ID == in.TaskID {
							nodes[i].Task.Status, nodes[i].Lifecycle, nodes[i].Reason = taskfile.StatusCompleted, "completed", taskscheduler.ReasonCompleted
						}
					}
				}
				return result, tt.runErr
			}
			cfg := testConfig(root, "queue-test", clock, loader, runner)
			if tt.maxTasks > 0 {
				cfg.MaxTasks = tt.maxTasks
			}
			result, err := RunUntilExhausted(context.Background(), cfg)
			if result.StopReason != tt.want || calls != tt.wantCalls || (err != nil) != tt.wantErr {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestQueueEligibilityConsumesSharedArchiveAndWorkflowClassification(t *testing.T) {
	for _, disposition := range []string{taskfile.StatusCompleted, taskfile.StatusCancelled, taskfile.StatusAbandoned, taskfile.StatusSuperseded} {
		t.Run(disposition, func(t *testing.T) {
			dependent := node("dependent", 1, "pending").Task
			dependent.DependsOn = []string{"archived"}
			archive := autonomousscheduler.ArchiveEvidence{TaskID: "archived", ArchiveID: "archive-" + disposition, Disposition: disposition, Verified: true, Reconciled: true}
			if disposition != taskfile.StatusCompleted {
				archive.Reason = "operator recorded " + disposition
			}
			graph, err := autonomousscheduler.BuildSnapshot([]autonomousscheduler.ActiveTask{{Task: dependent, Lifecycle: "pending"}}, []autonomousscheduler.ArchiveEvidence{archive})
			if err != nil {
				t.Fatal(err)
			}
			ready, waiting := eligible(autonomousscheduler.ClassifyAll(graph, nil), nil)
			if disposition == taskfile.StatusCompleted {
				if len(ready) != 1 || ready[0].Task.ID != dependent.ID || !ready[0].SelectedNext || len(waiting) != 0 {
					t.Fatalf("ready=%+v waiting=%v", ready, waiting)
				}
				return
			}
			if len(ready) != 0 || !reflect.DeepEqual(waiting, []string{"dependent:" + string(taskscheduler.ReasonTerminalUnsatisfiedDependency)}) {
				t.Fatalf("ready=%+v waiting=%v", ready, waiting)
			}
		})
	}

	mixed := node("mixed", 0, "pending").Task
	mixed.Workflow = taskfile.WorkflowMixedPassV1
	autonomous := node("autonomous", 1, "pending").Task
	graph, err := autonomousscheduler.BuildSnapshot([]autonomousscheduler.ActiveTask{{Task: mixed}, {Task: autonomous, Lifecycle: "pending"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ready, waiting := eligible(autonomousscheduler.ClassifyAll(graph, nil), nil)
	if len(ready) != 1 || ready[0].Task.ID != autonomous.ID || !ready[0].SelectedNext || len(waiting) != 0 {
		t.Fatalf("cross-workflow ready=%+v waiting=%v", ready, waiting)
	}
}

func TestDependencyDiamondUnlocksAndNewIndependentChildAppearsBetweenSelections(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	completed := map[string]bool{}
	childPublished := false
	loader := func(context.Context) (Snapshot, error) {
		specs := []struct {
			id       string
			priority int
			deps     []string
		}{{"root", 0, nil}, {"left", 1, []string{"root"}}, {"right", 1, []string{"root"}}, {"diamond", 2, []string{"left", "right"}}}
		if childPublished {
			specs = append(specs, struct {
				id       string
				priority int
				deps     []string
			}{"child", -1, nil})
		}
		var nodes []autonomousscheduler.Node
		for _, spec := range specs {
			item := node(spec.id, spec.priority, "pending")
			item.Task.DependsOn = append([]string(nil), spec.deps...)
			if completed[spec.id] {
				item.Task.Status, item.Lifecycle, item.Reason = taskfile.StatusCompleted, "completed", taskscheduler.ReasonCompleted
			} else {
				for _, dep := range spec.deps {
					if !completed[dep] {
						item.Reason = taskscheduler.ReasonWaitingDependency
						item.WaitingOn = append(item.WaitingOn, dep)
					}
				}
			}
			nodes = append(nodes, item)
		}
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].Task.Priority != nodes[j].Task.Priority {
				return nodes[i].Task.Priority < nodes[j].Task.Priority
			}
			return nodes[i].Task.SourcePath < nodes[j].Task.SourcePath
		})
		return Snapshot{Fingerprint: nodesFingerprint(nodes), Nodes: nodes}, nil
	}
	var calls []string
	runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		calls = append(calls, in.TaskID)
		completed[in.TaskID] = true
		if in.TaskID == "root" {
			childPublished = true
		}
		return taskResult(in, autonomoustaskrun.StopCompleted), nil
	}
	result, err := RunUntilExhausted(context.Background(), testConfig(root, "queue-diamond", clock, loader, runner))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "root,child,left,right,diamond" {
		t.Fatalf("calls=%s", got)
	}
	if result.StopReason != StopDrained || len(result.Outcomes) != 5 {
		t.Fatalf("result=%+v", result)
	}
}

func TestCrashAfterSelectionHistoryRecoversSamePinnedTaskOperation(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	nodes := []autonomousscheduler.Node{node("task", 0, "pending")}
	loader := func(context.Context) (Snapshot, error) {
		return Snapshot{Fingerprint: nodesFingerprint(nodes), Nodes: nodes}, nil
	}
	var selectedFailures int
	cfg := testConfig(root, "queue-crash", clock, loader, func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		nodes[0].Task.Status, nodes[0].Lifecycle, nodes[0].Reason = taskfile.StatusCompleted, "completed", taskscheduler.ReasonCompleted
		return taskResult(in, autonomoustaskrun.StopCompleted), nil
	})
	cfg.FailureInjector = func(point FailurePoint) error {
		if point == FailureAfterHistory {
			selectedFailures++
			if selectedFailures == 2 {
				return errors.New("crash after selected history")
			}
		}
		return nil
	}
	if _, err := RunUntilExhausted(context.Background(), cfg); err == nil {
		t.Fatal("injected crash succeeded")
	}
	op, found, err := Inspect(root, "queue-crash")
	if err != nil || !found || len(op.Slots) != 1 {
		t.Fatalf("inspect=%+v found=%v err=%v", op, found, err)
	}
	pinned := op.Slots[0].Selection.TaskOperationID
	cfg.FailureInjector = nil
	result, err := RunUntilExhausted(context.Background(), cfg)
	if err != nil || len(result.Outcomes) != 1 || result.Outcomes[0].TaskOperationID != pinned {
		t.Fatalf("recovery=%+v err=%v pinned=%s", result, err, pinned)
	}
}

func TestLegacySequentialTerminalOperationReplaysAndRejectsParallelChange(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	started := clock().UTC()
	done := clock().UTC()
	op := Operation{SchemaVersion: LegacyOperationSchemaVersion, OperationID: "queue-legacy", Mode: ModeUntilExhausted, ConfigSchema: "config-v1", ConfigSHA256: strings.Repeat("a", 64), SafetyIdentity: strings.Repeat("b", 64), MaxTasks: 20, StartedAt: started, UpdatedAt: started, Sweep: 1, Stage: "admitted"}
	if err := persist(root, Operation{}, op, nil); err != nil {
		t.Fatal(err)
	}
	previous := op
	op.Sequence = 1
	op.Stage = "terminal"
	op.UpdatedAt = done
	op.CompletedAt = &done
	op.StopReason = StopDrained
	op.StopDetail = "legacy drained"
	if err := persist(root, previous, op, nil); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(root, "queue-legacy", clock, func(context.Context) (Snapshot, error) {
		t.Fatal("terminal legacy replay loaded scheduler")
		return Snapshot{}, nil
	}, func(context.Context, RunTaskInput) (autonomoustaskrun.Result, error) {
		t.Fatal("terminal legacy replay started task")
		return autonomoustaskrun.Result{}, nil
	})
	result, err := RunUntilExhausted(context.Background(), cfg)
	if err != nil || !result.Replayed || result.MaximumWorkers != 1 || result.StopReason != StopDrained {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	cfg.MaximumWorkers = 2
	if _, err := RunUntilExhausted(context.Background(), cfg); err == nil {
		t.Fatal("legacy replay accepted changed worker bound")
	}
}

func TestCrashAfterFirstParallelReconciliationReopensExactRemainingSlot(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	var mu sync.Mutex
	states := map[string]string{"one": "pending", "two": "pending"}
	loader := graphLoader(&mu, states, []taskfile.Task{node("one", 0, "pending").Task, node("two", 1, "pending").Task})
	calls := map[string][]string{}
	runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		mu.Lock()
		calls[in.TaskID] = append(calls[in.TaskID], in.OperationID)
		count := len(calls[in.TaskID])
		states[in.TaskID] = "completed"
		mu.Unlock()
		result := taskResult(in, autonomoustaskrun.StopCompleted)
		if count > 1 {
			result.Replayed = true
		}
		return result, nil
	}
	cfg := testConfig(root, "queue-parallel-crash", clock, loader, runner)
	cfg.MaximumWorkers = 2
	histories := 0
	cfg.FailureInjector = func(point FailurePoint) error {
		if point == FailureAfterHistory {
			histories++
			if histories == 3 {
				return errors.New("crash after first slot history")
			}
		}
		return nil
	}
	if _, err := RunUntilExhausted(context.Background(), cfg); err == nil {
		t.Fatal("injected reconciliation crash succeeded")
	}
	cfg.FailureInjector = nil
	result, err := RunUntilExhausted(context.Background(), cfg)
	if err != nil || len(result.Outcomes) != 2 || result.Outcomes[0].TaskID != "one" || result.Outcomes[1].TaskID != "two" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(calls["one"]) != 1 || len(calls["two"]) != 2 || calls["two"][0] != calls["two"][1] {
		t.Fatalf("calls=%v", calls)
	}
}

func TestOperationConflictLeaseCancellationAndRedaction(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	nodes := []autonomousscheduler.Node{node("task", 0, "pending")}
	loader := func(context.Context) (Snapshot, error) {
		return Snapshot{Fingerprint: nodesFingerprint(nodes), Nodes: nodes}, nil
	}
	started := make(chan struct{})
	release := make(chan struct{})
	runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		close(started)
		<-release
		return taskResult(in, autonomoustaskrun.StopSafety), nil
	}
	cfg := testConfig(root, "queue-lock", clock, loader, runner)
	done := make(chan error, 1)
	go func() { _, err := RunUntilExhausted(context.Background(), cfg); done <- err }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := RunUntilExhausted(ctx, cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lease contention err=%v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	conflict := cfg
	conflict.MaxTasks++
	if _, err := RunUntilExhausted(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "different material") {
		t.Fatalf("changed operation err=%v", err)
	}
	conflict = cfg
	conflict.MaximumWorkers = 2
	if _, err := RunUntilExhausted(context.Background(), conflict); err == nil || !strings.Contains(err.Error(), "different material") {
		t.Fatalf("changed worker operation err=%v", err)
	}

	secretNodes := []autonomousscheduler.Node{node("secret", 0, "pending")}
	secretLoader := func(context.Context) (Snapshot, error) {
		return Snapshot{Fingerprint: nodesFingerprint(secretNodes), Nodes: secretNodes}, nil
	}
	redactCfg := testConfig(t.TempDir(), "queue-redact", clock, secretLoader, func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		return taskResultDetail(in, autonomoustaskrun.StopSafety, "token-secret"), nil
	})
	redactCfg.Redact = func(v string) string { return strings.ReplaceAll(v, "token-secret", "[REDACTED]") }
	redacted, _ := RunUntilExhausted(context.Background(), redactCfg)
	raw, _ := canonical(redacted)
	if strings.Contains(string(raw), "token-secret") || !strings.Contains(string(raw), "[REDACTED]") {
		t.Fatalf("redaction=%s", raw)
	}
}

func TestQueueLedgerEffectsAreDeduplicatedOnTerminalReplay(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	store, err := ledger.OpenWithClock(context.Background(), filepath.Join(root, ".revolvr", "ledger.sqlite"), clock)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	nodes := []autonomousscheduler.Node{node("task", 0, "pending")}
	loader := func(context.Context) (Snapshot, error) {
		return Snapshot{Fingerprint: nodesFingerprint(nodes), Nodes: nodes}, nil
	}
	cfg := testConfig(root, "queue-ledger", clock, loader, func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		nodes[0].Task.Status, nodes[0].Lifecycle, nodes[0].Reason = taskfile.StatusCompleted, "completed", taskscheduler.ReasonCompleted
		return taskResult(in, autonomoustaskrun.StopCompleted), nil
	})
	cfg.Ledger = store
	if _, err := RunUntilExhausted(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := RunUntilExhausted(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	history, found, err := store.GetRunWithEvents(context.Background(), queueLedgerRunID("queue-ledger"))
	if err != nil || !found {
		t.Fatalf("history found=%v err=%v", found, err)
	}
	counts := map[ledger.EventType]int{}
	for _, event := range history.Events {
		counts[event.Type]++
	}
	for _, kind := range []ledger.EventType{ledger.EventQueueAdmitted, ledger.EventQueueSelection, ledger.EventQueueTaskStopped, ledger.EventQueueStopped} {
		if counts[kind] != 1 {
			t.Fatalf("event %s count=%d", kind, counts[kind])
		}
	}
	for _, event := range history.Events {
		if event.Type != ledger.EventQueueStopped {
			continue
		}
		decoded, err := DecodeLedgerEvent(event.Payload)
		if err != nil || decoded.MaximumWorkers != 1 || len(decoded.Outcomes) != 1 || decoded.Outcomes[0].SelectionSequence != 1 {
			t.Fatalf("terminal ledger event=%+v err=%v", decoded, err)
		}
	}
}

func TestParallelWorkersRespectBoundAndPublishSelectionOrder(t *testing.T) {
	for _, workers := range []int{1, 2, MaximumWorkerLimit} {
		t.Run(fmt.Sprintf("workers-%d", workers), func(t *testing.T) {
			root := t.TempDir()
			clock := newClock()
			var mu sync.Mutex
			states := map[string]string{}
			var tasks []taskfile.Task
			for i := 0; i < MaximumWorkerLimit; i++ {
				id := fmt.Sprintf("task-%d", i+1)
				states[id] = "pending"
				tasks = append(tasks, node(id, i, "pending").Task)
			}
			loader := graphLoader(&mu, states, tasks)
			active, peak := 0, 0
			runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
				mu.Lock()
				active++
				if active > peak {
					peak = active
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				states[in.TaskID] = "completed"
				active--
				mu.Unlock()
				return taskResult(in, autonomoustaskrun.StopCompleted), nil
			}
			cfg := testConfig(root, "queue-bound", clock, loader, runner)
			cfg.MaximumWorkers = workers
			result, err := RunUntilExhausted(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			wantPeak := workers
			if wantPeak > len(tasks) {
				wantPeak = len(tasks)
			}
			if peak != wantPeak || result.Statistics.PeakActiveWorkers != wantPeak || result.MaximumWorkers != workers {
				t.Fatalf("peak=%d statistics=%+v workers=%d", peak, result.Statistics, result.MaximumWorkers)
			}
			for i, outcome := range result.Outcomes {
				if outcome.TaskID != fmt.Sprintf("task-%d", i+1) || outcome.SelectionSequence != int64(i+1) {
					t.Fatalf("outcomes are not canonical: %+v", result.Outcomes)
				}
			}
		})
	}
}

func TestParallelAdmissionHonorsDependenciesConflictsAndInvertedCompletion(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	var mu sync.Mutex
	states := map[string]string{"first": "pending", "conflict": "pending", "independent": "pending", "dependent": "pending"}
	first := node("first", 0, "pending").Task
	first.Conflicts = []string{"shared"}
	conflict := node("conflict", 1, "pending").Task
	conflict.Conflicts = []string{"shared"}
	independent := node("independent", 2, "pending").Task
	dependent := node("dependent", 3, "pending").Task
	dependent.DependsOn = []string{"first"}
	loader := graphLoader(&mu, states, []taskfile.Task{first, conflict, independent, dependent})
	started := make(chan string, 4)
	releases := map[string]chan struct{}{"first": make(chan struct{}), "independent": make(chan struct{}), "conflict": make(chan struct{}), "dependent": make(chan struct{})}
	runner := func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		started <- in.TaskID
		<-releases[in.TaskID]
		mu.Lock()
		states[in.TaskID] = "completed"
		mu.Unlock()
		return taskResult(in, autonomoustaskrun.StopCompleted), nil
	}
	cfg := testConfig(root, "queue-overlap", clock, loader, runner)
	cfg.MaximumWorkers = 2
	done := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, err := RunUntilExhausted(context.Background(), cfg)
		done <- struct {
			result Result
			err    error
		}{result, err}
	}()
	firstBatch := []string{<-started, <-started}
	sort.Strings(firstBatch)
	if strings.Join(firstBatch, ",") != "first,independent" {
		t.Fatalf("unsafe first batch %v", firstBatch)
	}
	close(releases["independent"])
	close(releases["first"])
	secondBatch := []string{<-started, <-started}
	sort.Strings(secondBatch)
	if strings.Join(secondBatch, ",") != "conflict,dependent" {
		t.Fatalf("second batch=%v", secondBatch)
	}
	close(releases["conflict"])
	close(releases["dependent"])
	finished := <-done
	if finished.err != nil || finished.result.StopReason != StopDrained {
		t.Fatalf("result=%+v err=%v", finished.result, finished.err)
	}
	if got := taskIDsFromOutcomes(finished.result.Outcomes); got != "first,independent,conflict,dependent" {
		t.Fatalf("outcome order=%s", got)
	}
}

func TestParallelCancellationWaitsForEveryWorkerAndFallbackIsDurable(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		root := t.TempDir()
		clock := newClock()
		var mu sync.Mutex
		states := map[string]string{"one": "pending", "two": "pending"}
		loader := graphLoader(&mu, states, []taskfile.Task{node("one", 0, "pending").Task, node("two", 1, "pending").Task})
		started := make(chan struct{}, 2)
		cleaned := make(chan struct{}, 2)
		runner := func(ctx context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
			started <- struct{}{}
			<-ctx.Done()
			cleaned <- struct{}{}
			return taskResult(in, autonomoustaskrun.StopOperationCancelled), ctx.Err()
		}
		ctx, cancel := context.WithCancel(context.Background())
		cfg := testConfig(root, "queue-cancel-all", clock, loader, runner)
		cfg.MaximumWorkers = 2
		done := make(chan Result, 1)
		go func() { result, _ := RunUntilExhausted(ctx, cfg); done <- result }()
		<-started
		<-started
		cancel()
		result := <-done
		if len(cleaned) != 2 || result.StopReason != StopCancelled || len(result.Outcomes) != 2 {
			t.Fatalf("cleanup=%d result=%+v", len(cleaned), result)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		root := t.TempDir()
		clock := newClock()
		var mu sync.Mutex
		states := map[string]string{"one": "pending", "two": "pending"}
		base := graphLoader(&mu, states, []taskfile.Task{node("one", 0, "pending").Task, node("two", 1, "pending").Task})
		loader := func(ctx context.Context) (Snapshot, error) {
			snapshot, err := base(ctx)
			snapshot.Classify = nil
			return snapshot, err
		}
		cfg := testConfig(root, "queue-fallback", clock, loader, func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
			mu.Lock()
			states[in.TaskID] = "completed"
			mu.Unlock()
			return taskResult(in, autonomoustaskrun.StopCompleted), nil
		})
		cfg.MaximumWorkers = 3
		result, err := RunUntilExhausted(context.Background(), cfg)
		if err != nil || result.Statistics.PeakActiveWorkers != 1 || result.Statistics.SequentialFallbacks == 0 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
		op, _, _ := Inspect(root, "queue-fallback")
		if op.Statistics.SequentialFallbacks == 0 {
			t.Fatal("fallback was not durable")
		}
	})
}

func TestParallelWorkerPanicPreservesPeerEvidence(t *testing.T) {
	root := t.TempDir()
	clock := newClock()
	var mu sync.Mutex
	states := map[string]string{"panic": "pending", "peer": "pending"}
	loader := graphLoader(&mu, states, []taskfile.Task{node("panic", 0, "pending").Task, node("peer", 1, "pending").Task})
	cfg := testConfig(root, "queue-panic", clock, loader, func(_ context.Context, in RunTaskInput) (autonomoustaskrun.Result, error) {
		if in.TaskID == "panic" {
			panic("injected")
		}
		mu.Lock()
		states[in.TaskID] = "completed"
		mu.Unlock()
		return taskResult(in, autonomoustaskrun.StopCompleted), nil
	})
	cfg.MaximumWorkers = 2
	result, err := RunUntilExhausted(context.Background(), cfg)
	if err == nil || result.StopReason != StopUnsafeAmbiguous || len(result.Outcomes) != 2 || result.Outcomes[1].StopReason != autonomoustaskrun.StopCompleted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func graphLoader(mu *sync.Mutex, states map[string]string, tasks []taskfile.Task) Loader {
	return func(context.Context) (Snapshot, error) {
		mu.Lock()
		defer mu.Unlock()
		active := make([]autonomousscheduler.ActiveTask, len(tasks))
		for i, task := range tasks {
			copyTask := task
			copyTask.DependsOn = append([]string(nil), task.DependsOn...)
			copyTask.Conflicts = append([]string(nil), task.Conflicts...)
			lifecycle := states[task.ID]
			if lifecycle == "completed" {
				copyTask.Status = taskfile.StatusCompleted
			}
			active[i] = autonomousscheduler.ActiveTask{Task: copyTask, Lifecycle: lifecycle, StateSHA256: strings.Repeat("c", 64), StateByteSize: len(task.ID)}
		}
		graph, err := autonomousscheduler.BuildSnapshot(active, nil)
		if err != nil {
			return Snapshot{}, err
		}
		nodes := autonomousscheduler.ClassifyAll(graph, nil)
		return Snapshot{Fingerprint: nodesFingerprint(nodes), Nodes: nodes, Classify: func(occupied []string) ([]autonomousscheduler.Node, error) {
			return autonomousscheduler.ClassifyAll(graph, occupied), nil
		}}, nil
	}
}

func taskIDsFromOutcomes(outcomes []TaskOutcome) string {
	ids := make([]string, len(outcomes))
	for i, outcome := range outcomes {
		ids[i] = outcome.TaskID
	}
	return strings.Join(ids, ",")
}

func testConfig(root, id string, clock func() time.Time, loader Loader, runner TaskRunner) Config {
	return Config{RepositoryRoot: root, OperationID: id, Mode: ModeUntilExhausted, ConfigSchema: "config-v1", ConfigSHA256: strings.Repeat("a", 64), SafetyIdentity: strings.Repeat("b", 64), MaxTasks: 20, MaximumWorkers: 1, Sweep: 1, Clock: clock, Loader: loader, Runner: runner}
}

func node(id string, priority int, lifecycle string) autonomousscheduler.Node {
	status := taskfile.StatusPending
	reason := taskscheduler.ReasonReady
	switch lifecycle {
	case "completed":
		status, reason = taskfile.StatusCompleted, taskscheduler.ReasonCompleted
	case "blocked":
		reason = taskscheduler.ReasonBlocked
	case "needs_input":
		reason = taskscheduler.ReasonNeedsInput
	case "cancelled":
		status, reason = taskfile.StatusCancelled, taskscheduler.ReasonCancelled
	case "abandoned":
		status, reason = taskfile.StatusAbandoned, taskscheduler.ReasonAbandoned
	case "superseded":
		status, reason = taskfile.StatusSuperseded, taskscheduler.ReasonSuperseded
	}
	return autonomousscheduler.Node{Task: taskfile.Task{ID: id, Status: status, Workflow: taskfile.WorkflowAutonomousV1, HasPriority: true, Priority: priority, SourcePath: filepath.ToSlash(filepath.Join(".agent", "tasks", id+".md")), SourceBytes: []byte(id)}, Lifecycle: lifecycle, StateSHA256: strings.Repeat("c", 64), StateByteSize: len(id), Reason: reason}
}

func lifecycleNode(id, lifecycle string) autonomousscheduler.Node {
	return node(id, 0, lifecycle)
}

func waitingNode(id string, reason taskscheduler.Reason) autonomousscheduler.Node {
	result := node(id, 0, "pending")
	result.Reason = reason
	result.WaitingOn = []string{"dependency"}
	return result
}

func taskResult(in RunTaskInput, reason autonomoustaskrun.StopReason) autonomoustaskrun.Result {
	return taskResultDetail(in, reason, string(reason))
}

func taskResultDetail(in RunTaskInput, reason autonomoustaskrun.StopReason, detail string) autonomoustaskrun.Result {
	return autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, TaskID: in.TaskID, OperationID: in.OperationID, StopReason: reason, StopDetail: detail}
}

func nodesFingerprint(nodes []autonomousscheduler.Node) string {
	var values []string
	for _, item := range nodes {
		values = append(values, item.Task.ID, item.Task.Status, item.Lifecycle, string(item.Reason))
	}
	return hashStrings(values...)
}

func fingerprint(states map[string]string) string {
	return hashStrings(states["high"], states["blocked"], states["later"])
}

func newClock() func() time.Time {
	current := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		current = current.Add(time.Millisecond)
		return current
	}
}
