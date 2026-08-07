package queue

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/scheduler"
	"revolvr/internal/storage/postgres"
)

func TestSequentialOrderingDependencyUnlockAndPeakOneWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	first := fixture.addTask(t, ctx, "first", "pending", 20, fixture.now)
	second := fixture.addTask(t, ctx, "second", "pending", 1, fixture.now.Add(time.Second))
	fixture.addDependency(t, ctx, second, first)
	third := fixture.addTask(t, ctx, "third", "pending", 30, fixture.now.Add(2*time.Second))

	executor := fixture.executor(map[string]TaskOutcome{
		uuidString(first.id):  OutcomeCompleted,
		uuidString(second.id): OutcomeCompleted,
		uuidString(third.id):  OutcomeCompleted,
	}, usage{cycles: 2, tokens: 100, cost: 25})
	result, err := Run(ctx, fixture.pool, fixture.config(id.New(), executor))
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopDrained || result.TasksStarted != 3 || result.PeakSourceMutatingWorkers != 1 {
		t.Fatalf("result = %#v", result)
	}
	want := []string{uuidString(first.id), uuidString(second.id), uuidString(third.id)}
	for i, outcome := range result.Outcomes {
		if outcome.TaskID != want[i] || outcome.OccurrenceSequence != int64(i+1) || !outcome.LeaseReconciled {
			t.Fatalf("outcome %d = %#v, want task %s", i, outcome, want[i])
		}
	}
	if peak := fixture.peak.Load(); peak != 1 {
		t.Fatalf("observed peak workers = %d, want 1", peak)
	}
	assertQueueEventOrder(t, ctx, fixture.pool, result.OperationID)
}

func TestYieldedTaskOutcomesPermitUnrelatedProgress(t *testing.T) {
	for _, test := range []struct {
		name     string
		outcome  TaskOutcome
		status   string
		wantStop StopReason
	}{
		{name: "blocked", outcome: OutcomeBlocked, status: "blocked", wantStop: StopAllRemainingBlocked},
		{name: "needs-input", outcome: OutcomeNeedsInput, status: "needs_input", wantStop: StopWaitingInput},
		{name: "dependency-waiting", outcome: OutcomeDependencyWaiting, status: "blocked", wantStop: StopAllRemainingBlocked},
		{name: "task-budget", outcome: OutcomeTaskBudgetExhausted, status: "budget_exhausted", wantStop: StopDrained},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			fixture := newQueueFixture(t, ctx)
			yielded := fixture.addTask(t, ctx, "yielded", "pending", 1, fixture.now)
			unrelated := fixture.addTask(t, ctx, "unrelated", "pending", 20, fixture.now.Add(time.Second))
			executor := fixture.executorWithStatus(map[string]TaskOutcome{
				uuidString(yielded.id): test.outcome, uuidString(unrelated.id): OutcomeCompleted,
			}, map[string]string{uuidString(yielded.id): test.status, uuidString(unrelated.id): "completed"}, usage{cycles: 1})
			result, err := Run(ctx, fixture.pool, fixture.config(id.New(), executor))
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != test.wantStop || len(result.Outcomes) != 2 ||
				result.Outcomes[0].TaskID != uuidString(yielded.id) || result.Outcomes[0].Outcome != test.outcome ||
				result.Outcomes[1].TaskID != uuidString(unrelated.id) || result.Outcomes[1].Outcome != OutcomeCompleted {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestQueueStopClassifiesDependenciesAndInput(t *testing.T) {
	for _, test := range []struct {
		name       string
		dependency string
		want       StopReason
	}{
		{name: "dependency", dependency: "blocked", want: StopWaitingDependencies},
		{name: "input", dependency: "needs_input", want: StopWaitingInput},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			fixture := newQueueFixture(t, ctx)
			dependency := fixture.addTask(t, ctx, "dependency", test.dependency, 1, fixture.now)
			dependent := fixture.addTask(t, ctx, "dependent", "pending", 2, fixture.now.Add(time.Second))
			fixture.addDependency(t, ctx, dependent, dependency)
			result, err := Run(ctx, fixture.pool, fixture.config(id.New(), fixture.executor(nil, usage{})))
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != test.want || result.TasksStarted != 0 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestExactQueueBudgetsStopBeforeNextTask(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits Limits
		usage  usage
	}{
		{name: "tasks", limits: testLimits(1, 10, 10, 100, 100), usage: usage{cycles: 1}},
		{name: "cycles", limits: testLimits(10, 4, 4, 100, 100), usage: usage{cycles: 4}},
		{name: "tokens", limits: testLimits(10, 10, 10, 7, 100), usage: usage{cycles: 1, tokens: 7}},
		{name: "cost", limits: testLimits(10, 10, 10, 100, 9), usage: usage{cycles: 1, cost: 9}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			fixture := newQueueFixture(t, ctx)
			first := fixture.addTask(t, ctx, "first", "pending", 1, fixture.now)
			fixture.addTask(t, ctx, "second", "pending", 2, fixture.now.Add(time.Second))
			cfg := fixture.config(id.New(), fixture.executor(map[string]TaskOutcome{uuidString(first.id): OutcomeCompleted}, test.usage))
			cfg.Limits = test.limits
			result, err := Run(ctx, fixture.pool, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != StopBudgetExhausted || result.TasksStarted != 1 || len(result.Outcomes) != 1 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestQueueDurationBudgetStopsBeforeSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	fixture.addTask(t, ctx, "ready", "pending", 1, fixture.now)
	calls := atomic.Int64{}
	clock := fixture.now
	cfg := fixture.config(id.New(), func(context.Context, TaskRequest) (TaskResult, error) {
		calls.Add(1)
		return TaskResult{}, errors.New("unexpected executor call")
	})
	cfg.Limits.MaximumDuration = time.Millisecond
	cfg.Clock = func() time.Time {
		clock = clock.Add(time.Millisecond)
		return clock
	}
	result, err := Run(ctx, fixture.pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopBudgetExhausted || result.TasksStarted != 0 || calls.Load() != 0 {
		t.Fatalf("result = %#v calls=%d", result, calls.Load())
	}
}

func TestQueueDurationBudgetStopsAndReconcilesActiveChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	task := fixture.addTask(t, ctx, "duration", "pending", 1, fixture.now)
	cfg := fixture.config(id.New(), func(ctx context.Context, request TaskRequest) (TaskResult, error) {
		if err := fixture.effect(ctx, request, EffectSupervisor, "supervisor", func() error { return nil }); err != nil {
			return TaskResult{}, err
		}
		<-ctx.Done()
		clean := context.WithoutCancel(ctx)
		if err := fixture.effect(clean, request, EffectSupervisor, "duration-reconcile", func() error {
			return fixture.resolveTask(clean, request, "budget_exhausted")
		}); err != nil {
			return TaskResult{}, err
		}
		return terminalTaskResult(OutcomeTaskBudgetExhausted, usage{}), nil
	})
	cfg.Clock = time.Now
	cfg.Limits.MaximumDuration = time.Second

	result, err := Run(ctx, fixture.pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopBudgetExhausted || result.TasksStarted != 1 ||
		len(result.Outcomes) != 1 || result.Outcomes[0].Outcome != OutcomeTaskBudgetExhausted ||
		!result.Outcomes[0].LeaseReconciled || !result.Outcomes[0].Reconciliation.Workspace ||
		!result.Outcomes[0].Reconciliation.Evidence {
		t.Fatalf("result = %#v", result)
	}
	assertTaskRunReconciled(t, ctx, fixture.pool, task.id, "budget_exhausted")
}

func TestCorrectionCycleReverifiesBeforeCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	fixture.addTask(t, ctx, "corrected", "pending", 1, fixture.now)
	executor := func(ctx context.Context, request TaskRequest) (TaskResult, error) {
		kinds := []EffectKind{
			EffectSupervisor, EffectWorker, EffectVerification, EffectAudit,
			EffectCorrection, EffectVerification, EffectAudit, EffectCompletion,
		}
		for index, kind := range kinds {
			callback := func() error { return nil }
			if kind == EffectCompletion {
				callback = func() error { return fixture.resolveTask(ctx, request, "completed") }
			}
			if err := fixture.effect(ctx, request, kind, fmt.Sprintf("%02d-%s", index+1, kind), callback); err != nil {
				return TaskResult{}, err
			}
		}
		return terminalTaskResult(OutcomeCompleted, usage{cycles: 2}), nil
	}
	result, err := Run(ctx, fixture.pool, fixture.config(id.New(), executor))
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopDrained || len(result.Outcomes) != 1 {
		t.Fatalf("result = %#v", result)
	}
	rows, err := postgres.New(fixture.pool).ListQueueTaskEffects(ctx, mustUUID(t, result.Outcomes[0].OccurrenceID))
	if err != nil {
		t.Fatal(err)
	}
	want := []EffectKind{
		EffectSupervisor, EffectWorker, EffectVerification, EffectAudit,
		EffectCorrection, EffectVerification, EffectAudit, EffectCompletion,
	}
	if len(rows) != len(want) {
		t.Fatalf("effect count = %d, want %d", len(rows), len(want))
	}
	for index, row := range rows {
		if EffectKind(row.EffectKind) != want[index] || row.EffectSequence != int64(index+1) {
			t.Fatalf("effect %d = %#v, want %s", index, row, want[index])
		}
	}
}

func TestUnsafeAndSystemFailureStopEntireQueue(t *testing.T) {
	for _, test := range []struct {
		outcome TaskOutcome
		stop    StopReason
	}{
		{outcome: OutcomeUnsafe, stop: StopUnsafe},
		{outcome: OutcomeSystemFailure, stop: StopSystemFailure},
	} {
		t.Run(string(test.outcome), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			fixture := newQueueFixture(t, ctx)
			first := fixture.addTask(t, ctx, "first", "pending", 1, fixture.now)
			fixture.addTask(t, ctx, "must-not-start", "pending", 2, fixture.now.Add(time.Second))
			result, err := Run(ctx, fixture.pool, fixture.config(id.New(), fixture.executor(map[string]TaskOutcome{uuidString(first.id): test.outcome}, usage{cycles: 1})))
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != test.stop || result.TasksStarted != 1 || len(result.Outcomes) != 1 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}

func TestGlobalLeaseAdmissionHasExactlyOneWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	fixture.addTask(t, ctx, "race", "pending", 1, fixture.now)
	operationID := id.New()
	crash := errors.New("stop after selection")
	failed := false
	cfg := fixture.config(operationID, fixture.executor(nil, usage{}))
	cfg.FailureInjector = func(point FailurePoint) error {
		if point == FailureAfterSelection && !failed {
			failed = true
			return crash
		}
		return nil
	}
	if _, err := Run(ctx, fixture.pool, cfg); !errors.Is(err, crash) {
		t.Fatalf("queue selection interruption error = %v", err)
	}
	candidate, err := scheduler.Select(ctx, fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	directRun := id.New()
	if _, err := scheduler.Admit(ctx, fixture.pool, scheduler.AdmissionCommand{
		RunID: directRun, CoordinatorIdentity: "direct-run", Candidate: candidate,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, fixture.pool, cfg); !errors.Is(err, scheduler.ErrLeaseBusy) {
		t.Fatalf("queue admission error = %v, want lease busy", err)
	}
	var activeRuns int
	if err := fixture.pool.QueryRow(ctx, `SELECT count(*) FROM core.runs WHERE status='active'`).Scan(&activeRuns); err != nil {
		t.Fatal(err)
	}
	if activeRuns != 1 {
		t.Fatalf("active runs = %d, want exactly one", activeRuns)
	}
}

func TestCrashReplayBoundariesAreIdempotent(t *testing.T) {
	for _, point := range []FailurePoint{
		FailureBeforeSelection, FailureAfterSelection,
		FailureBeforeWorkerEffect, FailureAfterWorkerEffect,
		FailureBeforeCompletion, FailureAfterCompletion,
		FailureBeforeCheckpoint, FailureAfterCheckpoint,
	} {
		t.Run(string(point), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			fixture := newQueueFixture(t, ctx)
			task := fixture.addTask(t, ctx, "recover", "pending", 1, fixture.now)
			operationID := id.New()
			crash := fmt.Errorf("injected %s", point)
			injected := false
			cfg := fixture.config(operationID, fixture.executor(map[string]TaskOutcome{uuidString(task.id): OutcomeCompleted}, usage{cycles: 2, tokens: 3, cost: 4}))
			cfg.FailureInjector = func(observed FailurePoint) error {
				if observed == point && !injected {
					injected = true
					return crash
				}
				return nil
			}
			if _, err := Run(ctx, fixture.pool, cfg); !errors.Is(err, crash) {
				t.Fatalf("first run error = %v, want %v", err, crash)
			}
			result, err := Run(ctx, fixture.pool, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if result.StopReason != StopDrained || result.TasksStarted != 1 || len(result.Outcomes) != 1 {
				t.Fatalf("result = %#v", result)
			}
			fixture.assertExternalEffectsOnce(t)
			assertQueueCounts(t, ctx, fixture.pool, operationID, 1, 1)
			beforeCalls := fixture.calls.Load()
			replayed, err := Run(ctx, fixture.pool, cfg)
			if err != nil || !replayed.Replayed || fixture.calls.Load() != beforeCalls {
				t.Fatalf("terminal replay = %#v err=%v calls=%d/%d", replayed, err, beforeCalls, fixture.calls.Load())
			}
		})
	}
}

func TestCrashAfterExternalWorkerEffectBeforeLocalCompletionReconcilesOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	task := fixture.addTask(t, ctx, "effect-window", "pending", 1, fixture.now)
	operationID := id.New()
	crash := errors.New("worker acted before local completion")
	first := true
	executor := fixture.executor(map[string]TaskOutcome{uuidString(task.id): OutcomeCompleted}, usage{cycles: 1})
	cfg := fixture.config(operationID, func(ctx context.Context, request TaskRequest) (TaskResult, error) {
		if first {
			first = false
			if err := fixture.effect(ctx, request, EffectSupervisor, "supervisor", func() error { return nil }); err != nil {
				return TaskResult{}, err
			}
			identity := request.OccurrenceID + ":worker"
			material := hashBytes([]byte("material:" + identity))
			if _, err := request.Effects.PersistIntent(ctx, EffectWorker, identity, material); err != nil {
				return TaskResult{}, err
			}
			fixture.mu.Lock()
			fixture.effects[identity]++
			fixture.mu.Unlock()
			return TaskResult{}, crash
		}
		return executor(ctx, request)
	})
	if _, err := Run(ctx, fixture.pool, cfg); !errors.Is(err, crash) {
		t.Fatalf("first error = %v, want crash", err)
	}
	result, err := Run(ctx, fixture.pool, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != StopDrained || result.TasksStarted != 1 {
		t.Fatalf("result = %#v", result)
	}
	fixture.assertEffectCount(t, "worker", 1)
}

func TestDivergentEffectReplayFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	task := fixture.addTask(t, ctx, "divergent", "pending", 1, fixture.now)
	operationID := id.New()
	crash := errors.New("after worker")
	injected := false
	executor := fixture.executor(map[string]TaskOutcome{uuidString(task.id): OutcomeCompleted}, usage{cycles: 1})
	cfg := fixture.config(operationID, executor)
	cfg.FailureInjector = func(point FailurePoint) error {
		if point == FailureAfterWorkerEffect && !injected {
			injected = true
			return crash
		}
		return nil
	}
	if _, err := Run(ctx, fixture.pool, cfg); !errors.Is(err, crash) {
		t.Fatalf("first error = %v", err)
	}
	cfg.Executor = fixture.divergentWorkerExecutor(task.id)
	if _, err := Run(ctx, fixture.pool, cfg); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("divergent replay error = %v, want conflict", err)
	}
	fixture.assertEffectCount(t, "worker", 1)
	status, err := StatusOperation(ctx, fixture.pool, operationID)
	if err != nil || status.Status != "active" || status.TasksStarted != 0 {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
}

func TestTerminalEvidenceDivergenceFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	task := fixture.addTask(t, ctx, "terminal", "pending", 1, fixture.now)
	operationID := id.New()
	if _, err := Run(ctx, fixture.pool, fixture.config(operationID, fixture.executor(map[string]TaskOutcome{uuidString(task.id): OutcomeCompleted}, usage{cycles: 1}))); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(ctx, `UPDATE core.queue_task_effects
		SET evidence_sha256=$1 WHERE effect_kind='worker'`, strings.Repeat("f", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := StatusOperation(ctx, fixture.pool, operationID); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("status error = %v, want conflict", err)
	}
	before := fixture.calls.Load()
	if _, err := Run(ctx, fixture.pool, fixture.config(operationID, fixture.executor(nil, usage{}))); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("terminal replay error = %v, want conflict", err)
	}
	if fixture.calls.Load() != before {
		t.Fatalf("terminal divergent replay started new work: calls %d -> %d", before, fixture.calls.Load())
	}
}

func TestSequentialQueueMigrationIsReversibleAndBounded(t *testing.T) {
	raw, err := os.ReadFile("../../db/migrations/00013_sequential_queue.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"-- +goose Up", "-- +goose Down", "CREATE TABLE core.queue_operations",
		"CREATE TABLE core.queue_task_occurrences", "CREATE TABLE core.queue_task_effects",
		"maximum_workers = 1", "worker_mode = 'direct_tools_v1'",
		"DROP TABLE core.queue_task_effects", "DROP TABLE core.queue_task_occurrences",
		"DROP TABLE core.queue_operations",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestCancellationStopsChildAndReconcilesEvidenceAndLease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := newQueueFixture(t, ctx)
	task := fixture.addTask(t, ctx, "cancel", "pending", 1, fixture.now)
	started := make(chan struct{})
	executor := fixture.cancellableExecutor(task.id, started)
	operationID := id.New()
	cfg := fixture.config(operationID, executor)
	resultCh := make(chan Result, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := Run(ctx, fixture.pool, cfg)
		resultCh <- result
		errCh <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	status, replayed, err := Cancel(ctx, fixture.pool, operationID, nil)
	if err != nil || replayed || status.CancelRequestedAt == nil {
		t.Fatalf("cancel status = %#v replayed=%t err=%v", status, replayed, err)
	}
	result := <-resultCh
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if result.StopReason != StopCancelled || result.TasksStarted != 1 || len(result.Outcomes) != 1 ||
		result.Outcomes[0].Outcome != OutcomeCancelled || !result.Outcomes[0].Reconciliation.Workspace ||
		!result.Outcomes[0].Reconciliation.Evidence || !result.Outcomes[0].LeaseReconciled {
		t.Fatalf("result = %#v", result)
	}
	var leaseRun pgtype.UUID
	var taskStatus string
	if err := fixture.pool.QueryRow(ctx, `SELECT l.run_id, t.status FROM core.execution_leases l
		JOIN core.tasks t ON t.id=$1 WHERE l.lease_name='global-source-mutation-v1'`, task.id).Scan(&leaseRun, &taskStatus); err != nil {
		t.Fatal(err)
	}
	if leaseRun.Valid || taskStatus != "cancelled" {
		t.Fatalf("lease/task = %s/%s", uuidString(leaseRun), taskStatus)
	}
}

type queueFixture struct {
	pool      *pgxpool.Pool
	projectID pgtype.UUID
	sourceID  pgtype.UUID
	commit    string
	tree      string
	now       time.Time
	active    atomic.Int64
	peak      atomic.Int64
	calls     atomic.Int64
	mu        sync.Mutex
	effects   map[string]int
}

type queueTask struct {
	id, versionID pgtype.UUID
}

type usage struct{ cycles, tokens, cost int64 }

func newQueueFixture(t *testing.T, ctx context.Context) *queueFixture {
	t.Helper()
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	projectID := mustUUID(t, id.New())
	sourceID := mustUUID(t, id.New())
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	identity := fmt.Sprintf("%x", sha256.Sum256([]byte(uuidString(projectID))))
	queries := postgres.New(pool)
	if _, err := queries.InsertProject(ctx, postgres.InsertProjectParams{
		ID: projectID, Name: "queue-" + uuidString(projectID), Status: "registered",
		CreatedAt: timestamp(now), UpdatedAt: timestamp(now),
	}); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	if _, err := queries.InsertProjectSource(ctx, postgres.InsertProjectSourceParams{
		ID: sourceID, ProjectID: projectID,
		CanonicalSourcePath:   "/queue/source/" + uuidString(projectID),
		ManagedRepositoryPath: "/queue/managed/" + uuidString(projectID) + ".git",
		CurrentCommit:         identity, CurrentTree: identity,
		DirtyState: []byte(`{"dirty":false}`), Remotes: []byte(`[]`),
	}); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	fixture := &queueFixture{pool: pool, projectID: projectID, sourceID: sourceID, commit: identity, tree: identity, now: now, effects: make(map[string]int)}
	t.Cleanup(func() { fixture.cleanup(t); pool.Close() })
	return fixture
}

func (f *queueFixture) addTask(t *testing.T, ctx context.Context, name, status string, priority int32, created time.Time) queueTask {
	t.Helper()
	queries := postgres.New(f.pool)
	taskID, versionID, artifactID := mustUUID(t, id.New()), mustUUID(t, id.New()), mustUUID(t, id.New())
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(uuidString(artifactID))))
	if _, err := queries.InsertArtifact(ctx, postgres.InsertArtifactParams{
		ID: artifactID, Sha256: hash, SizeBytes: 1, MediaType: "text/markdown",
		LogicalKind: "queue-test-task", StoragePath: "queue-test/" + uuidString(f.projectID) + "/" + hash,
		CreatedAt: timestamp(created),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTask(ctx, postgres.InsertTaskParams{
		ID: taskID, ProjectID: f.projectID, ExternalTaskID: name + "-" + id.New()[:8],
		Status: "draft", CreatedAt: timestamp(created), UpdatedAt: timestamp(created),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.InsertTaskVersion(ctx, postgres.InsertTaskVersionParams{
		ID: versionID, TaskID: taskID, VersionNumber: 1, SourceArtifactID: artifactID,
		Title: name, Goal: "queue fixture", RiskClass: "low", MutationClass: "bounded_source",
		NetworkProfile: "none", Priority: priority, Scope: []byte(`[]`), ExcludedScope: []byte(`[]`),
		VerificationPlan: []byte(`[]`), Budget: []byte(`{}`), SecretRequirements: []byte(`[]`),
		ExpectedPaths: []byte(`[]`), OperatorCheckpoints: []byte(`[]`), CreatedAt: timestamp(created),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE core.tasks SET status=$1, accepted_version_id=$2,
		aggregate_version=4 WHERE id=$3`, status, versionID, taskID); err != nil {
		t.Fatal(err)
	}
	return queueTask{id: taskID, versionID: versionID}
}

func (f *queueFixture) addDependency(t *testing.T, ctx context.Context, task, dependency queueTask) {
	t.Helper()
	if _, err := postgres.New(f.pool).InsertTaskDependency(ctx, postgres.InsertTaskDependencyParams{
		TaskVersionID: task.versionID, TaskID: task.id, ProjectID: f.projectID,
		DependencyTaskID: dependency.id, DependencyType: "requires", CreatedAt: timestamp(f.now),
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *queueFixture) config(operationID string, executor TaskExecutor) Config {
	return Config{
		OperationID: operationID, CoordinatorIdentity: "queue-test:" + operationID,
		Limits:            testLimits(20, 10, 100, 10000, 10000),
		QualityGateStatus: QualityGateDeterministicOnly, Clock: func() time.Time { return f.now },
		Executor: executor, CancellationPoll: time.Millisecond,
	}
}

func testLimits(tasks, perTaskCycles, totalCycles, tokens, cost int64) Limits {
	return Limits{
		MaximumTasks: tasks, MaximumCyclesPerTask: perTaskCycles, MaximumTotalCycles: totalCycles,
		MaximumRemoteTokens: tokens, MaximumCostMicrousd: cost, MaximumDuration: time.Hour,
	}
}

func (f *queueFixture) executor(outcomes map[string]TaskOutcome, used usage) TaskExecutor {
	statuses := make(map[string]string, len(outcomes))
	for taskID, outcome := range outcomes {
		statuses[taskID] = statusForOutcome(outcome)
	}
	return f.executorWithStatus(outcomes, statuses, used)
}

func (f *queueFixture) executorWithStatus(outcomes map[string]TaskOutcome, statuses map[string]string, used usage) TaskExecutor {
	return func(ctx context.Context, request TaskRequest) (TaskResult, error) {
		f.calls.Add(1)
		active := f.active.Add(1)
		for {
			peak := f.peak.Load()
			if active <= peak || f.peak.CompareAndSwap(peak, active) {
				break
			}
		}
		defer f.active.Add(-1)
		outcome, ok := outcomes[request.Candidate.TaskID]
		if !ok {
			return TaskResult{}, fmt.Errorf("unexpected task %s", request.Candidate.TaskID)
		}
		terminalStatus := statuses[request.Candidate.TaskID]
		if outcome != OutcomeCompleted {
			err := f.effect(ctx, request, EffectSupervisor, "supervisor", func() error {
				return f.resolveTask(ctx, request, terminalStatus)
			})
			if err != nil {
				return TaskResult{}, err
			}
			return terminalTaskResult(outcome, used), nil
		}
		for _, effect := range []EffectKind{EffectSupervisor, EffectWorker, EffectVerification, EffectAudit, EffectCompletion} {
			callback := func() error { return nil }
			if effect == EffectCompletion {
				callback = func() error { return f.resolveTask(ctx, request, terminalStatus) }
			}
			if err := f.effect(ctx, request, effect, string(effect), callback); err != nil {
				return TaskResult{}, err
			}
		}
		return terminalTaskResult(outcome, used), nil
	}
}

func (f *queueFixture) divergentWorkerExecutor(taskID pgtype.UUID) TaskExecutor {
	return func(ctx context.Context, request TaskRequest) (TaskResult, error) {
		if err := f.effect(ctx, request, EffectSupervisor, "supervisor", func() error { return nil }); err != nil {
			return TaskResult{}, err
		}
		identity := request.OccurrenceID + ":worker"
		_, err := request.Effects.PersistIntent(ctx, EffectWorker, identity, hashBytes([]byte("divergent-material")))
		return TaskResult{}, err
	}
}

func (f *queueFixture) cancellableExecutor(taskID pgtype.UUID, started chan<- struct{}) TaskExecutor {
	return func(ctx context.Context, request TaskRequest) (TaskResult, error) {
		if err := f.effect(ctx, request, EffectSupervisor, "supervisor", func() error { return nil }); err != nil {
			return TaskResult{}, err
		}
		close(started)
		<-ctx.Done()
		if err := f.effect(context.WithoutCancel(ctx), request, EffectSupervisor, "cancel-reconcile", func() error {
			return f.resolveTask(context.WithoutCancel(ctx), request, "cancelled")
		}); err != nil {
			return TaskResult{}, err
		}
		return terminalTaskResult(OutcomeCancelled, usage{}), nil
	}
}

func (f *queueFixture) effect(ctx context.Context, request TaskRequest, kind EffectKind, suffix string, callback func() error) error {
	identity := request.OccurrenceID + ":" + suffix
	material := hashBytes([]byte("material:" + identity))
	evidence := hashBytes([]byte("evidence:" + identity))
	admission, err := request.Effects.PersistIntent(ctx, kind, identity, material)
	if err != nil {
		return err
	}
	if admission.Completed {
		if admission.Evidence != evidence {
			return ErrOperationConflict
		}
		return nil
	}
	if admission.Replayed {
		f.mu.Lock()
		count := f.effects[identity]
		f.mu.Unlock()
		if count > 1 {
			return ErrOperationConflict
		}
		if count == 1 {
			_, err = request.Effects.PersistCompletion(ctx, kind, identity, material, evidence)
			return err
		}
	}
	if err := callback(); err != nil {
		return err
	}
	f.mu.Lock()
	f.effects[identity]++
	f.mu.Unlock()
	_, err = request.Effects.PersistCompletion(ctx, kind, identity, material, evidence)
	return err
}

func (f *queueFixture) resolveTask(ctx context.Context, request TaskRequest, status string) error {
	taskID, err := parseUUID(request.Candidate.TaskID)
	if err != nil {
		return err
	}
	runID, err := parseUUID(request.SchedulerRunID)
	if err != nil {
		return err
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current string
	var aggregate int64
	if err := tx.QueryRow(ctx, `SELECT status,aggregate_version FROM core.tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&current, &aggregate); err != nil {
		return err
	}
	if current == status {
		return tx.Commit(ctx)
	}
	if current != "admitted" {
		return fmt.Errorf("task status = %s, want admitted", current)
	}
	aggregate++
	resolvedAt := f.now.Add(30 * time.Second)
	if _, err := tx.Exec(ctx, `UPDATE core.tasks SET status=$2,aggregate_version=$3,updated_at=$4 WHERE id=$1`, taskID, status, aggregate, resolvedAt); err != nil {
		return err
	}
	_, err = postgres.New(tx).AppendEvent(ctx, postgres.AppendEventParams{
		ID: mustUUIDNoTest(id.New()), ProjectID: f.projectID, TaskID: taskID, RunID: runID,
		EventType: "task." + status, AggregateType: "task", AggregateID: taskID,
		AggregateVersion: aggregate, Payload: []byte(`{}`), CreatedAt: timestamp(resolvedAt),
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func terminalTaskResult(outcome TaskOutcome, used usage) TaskResult {
	return TaskResult{
		Outcome: outcome, CyclesConsumed: used.cycles, RemoteTokensConsumed: used.tokens,
		CostMicrousdConsumed: used.cost,
		Reconciliation:       Reconciliation{Workspace: true, Evidence: true},
	}
}

func statusForOutcome(outcome TaskOutcome) string {
	switch outcome {
	case OutcomeCompleted:
		return "completed"
	case OutcomeBlocked, OutcomeDependencyWaiting:
		return "blocked"
	case OutcomeNeedsInput:
		return "needs_input"
	case OutcomeTaskBudgetExhausted:
		return "budget_exhausted"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeUnsafe:
		return "unsafe"
	default:
		return "unsafe"
	}
}

func (f *queueFixture) assertExternalEffectsOnce(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for identity, count := range f.effects {
		if count != 1 {
			t.Fatalf("effect %s count = %d, want 1", identity, count)
		}
	}
}

func (f *queueFixture) assertEffectCount(t *testing.T, suffix string, want int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for identity, count := range f.effects {
		if len(identity) >= len(suffix) && identity[len(identity)-len(suffix):] == suffix {
			total += count
		}
	}
	if total != want {
		t.Fatalf("effect suffix %s count = %d, want %d", suffix, total, want)
	}
}

func assertTaskRunReconciled(t *testing.T, ctx context.Context, pool *pgxpool.Pool, taskID pgtype.UUID, wantStatus string) {
	t.Helper()
	var leaseRun pgtype.UUID
	var taskStatus string
	if err := pool.QueryRow(ctx, `SELECT l.run_id, t.status FROM core.execution_leases l
		JOIN core.tasks t ON t.id=$1 WHERE l.lease_name='global-source-mutation-v1'`, taskID).Scan(&leaseRun, &taskStatus); err != nil {
		t.Fatal(err)
	}
	if leaseRun.Valid || taskStatus != wantStatus {
		t.Fatalf("lease/task = %s/%s, want empty/%s", uuidString(leaseRun), taskStatus, wantStatus)
	}
}

func (f *queueFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	statements := []string{
		`UPDATE core.execution_leases SET run_id=NULL,coordinator_identity=NULL,acquired_at=NULL
		 WHERE run_id IN (SELECT id FROM core.runs WHERE project_id=$1)`,
		`DELETE FROM core.events WHERE aggregate_type='queue_operation' OR project_id=$1`,
		`UPDATE core.queue_operations SET active_occurrence_id=NULL`,
		`DELETE FROM core.queue_task_effects`,
		`DELETE FROM core.queue_task_occurrences`,
		`DELETE FROM core.queue_operations`,
		`DELETE FROM core.runs WHERE project_id=$1`,
		`DELETE FROM core.task_conflicts WHERE project_id=$1`,
		`DELETE FROM core.task_dependencies WHERE project_id=$1`,
		`UPDATE core.tasks SET accepted_version_id=NULL,status='draft' WHERE project_id=$1`,
		`DELETE FROM core.task_versions WHERE task_id IN (SELECT id FROM core.tasks WHERE project_id=$1)`,
		`DELETE FROM core.tasks WHERE project_id=$1`,
		`DELETE FROM core.project_sources WHERE project_id=$1`,
		`DELETE FROM core.projects WHERE id=$1`,
		`DELETE FROM core.artifacts WHERE storage_path LIKE 'queue-test/' || $1::text || '/%'`,
	}
	for _, statement := range statements {
		var err error
		if strings.Contains(statement, "$1") {
			_, err = f.pool.Exec(ctx, statement, f.projectID)
		} else {
			_, err = f.pool.Exec(ctx, statement)
		}
		if err != nil {
			t.Errorf("cleanup %q: %v", statement, err)
			return
		}
	}
}

func assertQueueCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, operationID string, wantOccurrences, wantTaskRuns int) {
	t.Helper()
	var occurrences, runs int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM core.queue_task_occurrences WHERE queue_operation_id=$1),
		(SELECT count(*) FROM core.runs r JOIN core.queue_task_occurrences o ON o.scheduler_run_id=r.id WHERE o.queue_operation_id=$1)`,
		mustUUID(t, operationID)).Scan(&occurrences, &runs); err != nil {
		t.Fatal(err)
	}
	if occurrences != wantOccurrences || runs != wantTaskRuns {
		t.Fatalf("occurrences/runs = %d/%d, want %d/%d", occurrences, runs, wantOccurrences, wantTaskRuns)
	}
}

func assertQueueEventOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, operationID string) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT aggregate_version,event_type FROM core.events
		WHERE aggregate_type='queue_operation' AND aggregate_id=$1 ORDER BY aggregate_version`, mustUUID(t, operationID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var previous int64
	for rows.Next() {
		var version int64
		var eventType string
		if err := rows.Scan(&version, &eventType); err != nil {
			t.Fatal(err)
		}
		if version != previous+1 {
			t.Fatalf("event %s version = %d after %d", eventType, version, previous)
		}
		previous = version
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func mustUUIDNoTest(value string) pgtype.UUID {
	parsed, err := parseUUID(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
