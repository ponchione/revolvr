package autonomousqueue

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
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
		{name: "dependency", nodes: []autonomousscheduler.Node{waitingNode("child", autonomousscheduler.ReasonWaitingDependency)}, want: StopWaitingDependency},
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
							nodes[i].Task.Status, nodes[i].Lifecycle, nodes[i].Reason = taskfile.StatusCompleted, "completed", autonomousscheduler.ReasonNotPending
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
				item.Task.Status, item.Lifecycle, item.Reason = taskfile.StatusCompleted, "completed", autonomousscheduler.ReasonNotPending
			} else {
				for _, dep := range spec.deps {
					if !completed[dep] {
						item.Reason = autonomousscheduler.ReasonWaitingDependency
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
		nodes[0].Task.Status, nodes[0].Lifecycle, nodes[0].Reason = taskfile.StatusCompleted, "completed", autonomousscheduler.ReasonNotPending
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
	if err != nil || !found || op.InFlight == nil {
		t.Fatalf("inspect=%+v found=%v err=%v", op, found, err)
	}
	pinned := op.InFlight.TaskOperationID
	cfg.FailureInjector = nil
	result, err := RunUntilExhausted(context.Background(), cfg)
	if err != nil || len(result.Outcomes) != 1 || result.Outcomes[0].TaskOperationID != pinned {
		t.Fatalf("recovery=%+v err=%v pinned=%s", result, err, pinned)
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
		nodes[0].Task.Status, nodes[0].Lifecycle, nodes[0].Reason = taskfile.StatusCompleted, "completed", autonomousscheduler.ReasonNotPending
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
}

func testConfig(root, id string, clock func() time.Time, loader Loader, runner TaskRunner) Config {
	return Config{RepositoryRoot: root, OperationID: id, Mode: ModeUntilExhausted, ConfigSchema: "config-v1", ConfigSHA256: strings.Repeat("a", 64), SafetyIdentity: strings.Repeat("b", 64), MaxTasks: 20, Sweep: 1, Clock: clock, Loader: loader, Runner: runner}
}

func node(id string, priority int, lifecycle string) autonomousscheduler.Node {
	status := taskfile.StatusPending
	reason := autonomousscheduler.ReasonReady
	if lifecycle == "completed" {
		status, reason = taskfile.StatusCompleted, autonomousscheduler.ReasonNotPending
	}
	return autonomousscheduler.Node{Task: taskfile.Task{ID: id, Status: status, Workflow: taskfile.WorkflowAutonomousV1, HasPriority: true, Priority: priority, SourcePath: filepath.ToSlash(filepath.Join(".agent", "tasks", id+".md")), SourceBytes: []byte(id)}, Lifecycle: lifecycle, StateSHA256: strings.Repeat("c", 64), StateByteSize: len(id), Reason: reason}
}

func lifecycleNode(id, lifecycle string) autonomousscheduler.Node {
	result := node(id, 0, "pending")
	result.Lifecycle = lifecycle
	return result
}

func waitingNode(id string, reason autonomousscheduler.Reason) autonomousscheduler.Node {
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
