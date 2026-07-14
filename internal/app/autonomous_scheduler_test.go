package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
)

func TestExplicitAutonomousRunRejectsWaitingDependencyBeforeRunner(t *testing.T) {
	repo := t.TempDir()
	createSchedulingTask(t, repo, "parent", nil)
	createSchedulingTask(t, repo, "child", []string{"parent"})
	calls := 0
	_, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repo}, TaskRunInput{OperationID: "explicit-waiting", TaskID: "child", MaxCycles: 1, Clock: time.Now, Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
		calls++
		return autonomoustaskrun.StepResult{}, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "waiting_dependency") || calls != 0 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".revolvr", "autonomous", "task-runs", "explicit-waiting")); !os.IsNotExist(statErr) {
		t.Fatalf("operation state exists: %v", statErr)
	}
}

func TestAutonomousRunRejectsInvalidSharedGraphBeforeRunner(t *testing.T) {
	repo := t.TempDir()
	createSchedulingTask(t, repo, "invalid-child", []string{"missing-dependency"})
	calls := 0
	_, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repo}, TaskRunInput{OperationID: "invalid-graph", MaxCycles: 1, Clock: time.Now, Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
		calls++
		return autonomoustaskrun.StepResult{}, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "missing_dependency") || calls != 0 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".revolvr", "autonomous", "task-runs", "invalid-graph")); !os.IsNotExist(statErr) {
		t.Fatalf("operation state exists: %v", statErr)
	}
}

func TestQueueRunsIndependentTasksInSchedulerOrderWithoutLiveCodex(t *testing.T) {
	repo := t.TempDir()
	createSchedulingTask(t, repo, "second", nil)
	createSchedulingTask(t, repo, "first", nil)
	// Canonical source paths break equal-priority ties.
	var calls []string
	result, err := RunQueue(context.Background(), Config{WorkDir: repo}, QueueInput{OperationID: "app-queue", MaxTasks: 10, MaxCycles: 1, Clock: time.Now, TaskRunner: func(_ context.Context, in autonomousqueue.RunTaskInput) (autonomoustaskrun.Result, error) {
		calls = append(calls, in.TaskID)
		return autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, OperationID: in.OperationID, TaskID: in.TaskID, StopReason: autonomoustaskrun.StopMaxCycles, StopDetail: "bounded fake"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "first,second" {
		t.Fatalf("calls=%s", got)
	}
	if result.StopReason != autonomousqueue.StopWaitingDependency || len(result.Outcomes) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(repo, ".revolvr", "autonomous", "queues", "app-queue", "operation.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulingFingerprintExcludesQueueAndLedgerRuntimeChanges(t *testing.T) {
	repo := t.TempDir()
	createSchedulingTask(t, repo, "task", nil)
	cfg, err := runonce.EffectiveConfig(DefaultRunOnceConfig(repo))
	if err != nil {
		t.Fatal(err)
	}
	before, err := loadQueueSnapshot(context.Background(), repo, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr", "autonomous", "queues", "self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".revolvr", "autonomous", "queues", "self", "operation.json"), []byte("self-generated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".revolvr", "unrelated-ledger-note"), []byte("event"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := loadQueueSnapshot(context.Background(), repo, cfg)
	if err != nil || before.Fingerprint != after.Fingerprint {
		t.Fatalf("before=%s after=%s err=%v", before.Fingerprint, after.Fingerprint, err)
	}
}

func TestDaemonRefusesOperatorAttendedConfiguration(t *testing.T) {
	_, err := RunDaemon(context.Background(), Config{WorkDir: t.TempDir()}, DaemonInput{OperationID: "daemon-attended", MaxSweeps: 1, Poll: time.Millisecond, Debounce: time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "fully-unattended") {
		t.Fatalf("err=%v", err)
	}
}

func TestTaskSafetyNotificationFailureIsIsolatedReleasedAndReplayed(t *testing.T) {
	repo := t.TempDir()
	createSchedulingTask(t, repo, "notify-task", nil)
	hook := filepath.Join(repo, "notify-hook")
	if err := os.WriteFile(hook, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "notification-test-secret"
	t.Setenv("HOOK_TOKEN", secret)
	writeAppTestFile(t, filepath.Join(repo, ".revolvr", "config.yaml"), `
autonomy:
  redaction:
    environment_variables: [HOOK_TOKEN]
notifications:
  enabled: true
  events: [safety_stop]
  executable: notify-hook
  directory: repository_root
  environment_names: [HOOK_TOKEN]
  timeout_seconds: 1
  stdout_cap_bytes: 128
  stderr_cap_bytes: 128
  maximum_attempts: 1
  retry_delay_seconds: 0
`)
	taskPath := filepath.Join(repo, ".agent", "tasks", "notify-task.md")
	before, _ := os.ReadFile(taskPath)
	steps, hooks := 0, 0
	var observations []struct {
		result autonomousnotification.Result
		err    error
	}
	runtime := NotificationRuntime{LookPath: func(string) (string, error) { return hook, nil }, LookupEnv: func(name string) (string, bool) { return secret, name == "HOOK_TOKEN" }, Runner: func(ctx context.Context, command runner.Command) runner.Result {
		hooks++
		unlock, err := autonomousexec.Acquire(ctx, repo)
		if err != nil {
			t.Fatalf("notification overlapped autonomous execution lease: %v", err)
		}
		unlock()
		raw, _ := io.ReadAll(command.Stdin)
		if !strings.Contains(string(raw), `"event":"safety_stop"`) || strings.Contains(string(raw), secret) || !command.ReplaceEnv || !reflect.DeepEqual(command.Env, []string{"HOOK_TOKEN=" + secret}) {
			t.Fatalf("notification command=%+v payload=%s", command, raw)
		}
		return runner.Result{ExitCode: 9, Stderr: secret}
	}}
	input := TaskRunInput{OperationID: "notify-safety", TaskID: "notify-task", MaxCycles: 1, Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
		steps++
		return autonomoustaskrun.StepResult{StopReason: autonomoustaskrun.StopSafety, StopDetail: "unsafe " + secret}, nil
	}, NotificationRuntime: runtime, Notification: func(result autonomousnotification.Result, err error) {
		observations = append(observations, struct {
			result autonomousnotification.Result
			err    error
		}{result, err})
	}}
	result, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repo}, input)
	if err != nil || result.StopReason != autonomoustaskrun.StopSafety || steps != 1 || hooks != 1 || len(observations) != 1 || observations[0].err == nil {
		t.Fatalf("result=%+v steps=%d hooks=%d observations=%+v err=%v", result, steps, hooks, observations, err)
	}
	if strings.Contains(observations[0].err.Error(), secret) {
		t.Fatalf("secret in notification error: %v", observations[0].err)
	}
	after, _ := os.ReadFile(taskPath)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("hook failure changed canonical task")
	}
	observations = nil
	replay, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repo}, input)
	if err != nil || !replay.Replayed || steps != 1 || hooks != 1 || len(observations) != 1 || !observations[0].result.Replayed {
		t.Fatalf("replay=%+v steps=%d hooks=%d observations=%+v err=%v", replay, steps, hooks, observations, err)
	}
}

func createSchedulingTask(t *testing.T, repo, id string, dependencies []string) {
	t.Helper()
	task, err := taskfile.ProjectAutonomousTask(repo, taskfile.AutonomousCreateInput{ID: id, Title: id, Body: "Bounded work.", DependsOn: dependencies})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = taskfile.PublishAutonomousTask(repo, task); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: id, Lifecycle: autonomous.LifecycleStatePending, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".revolvr", "autonomous", "tasks", id, "state.json")
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
