package autonomousverification

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/runner"
	"revolvr/internal/verification"
)

func TestExecutePassesAllSelectedTiersInOrder(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "checks"))
	var calls []runner.Command
	result, err := Execute(context.Background(), executionConfig(root, allTierPlan(), PurposeFinal, func(_ context.Context, c runner.Command) runner.Result {
		calls = append(calls, c)
		return runner.Result{ExitCode: 0, Stdout: "ok", StdoutTruncatedBytes: 2}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomePassed || !result.Gate.FinalSatisfied || len(result.Tiers) != 7 || len(calls) != 7 {
		t.Fatalf("result=%+v calls=%d", result, len(calls))
	}
	for i, c := range calls {
		if c.Args[0] != allTierPlan().Tiers[i].ID {
			t.Fatalf("call %d=%v", i, c.Args)
		}
	}
	id := result.Tiers[0].Commands[0].Identity
	if id.Name != "check" || id.Args[0] != "structural" || id.Dir != filepath.Join(root, "checks") || id.Env[0] != "TIER=structural" || id.Timeout != time.Minute || id.StdoutCap != 100 || id.StderrCap != 101 || id.Purpose != PurposeFinal || id.PlanSHA256 != result.Plan.SHA256 || len(id.SHA256) != 64 {
		t.Fatalf("identity=%+v", id)
	}
	if result.Tiers[0].Commands[0].Attempts[0].Stdout.TruncatedBytes != 2 {
		t.Fatal("truncation evidence missing")
	}
	raw, _ := MarshalResult(result)
	decoded, err := DecodeResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Gate.FinalSatisfied != result.Gate.FinalSatisfied {
		t.Fatal("result round trip differs")
	}
}

func TestExecuteFastNeverSatisfiesFinalGate(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "checks"))
	result, err := Execute(context.Background(), executionConfig(root, allTierPlan(), PurposeFast, passRunner))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomePassed || result.Gate.FinalSatisfied || !reflect.DeepEqual(result.Gate.MissingRequired, []string{"full-suite"}) {
		t.Fatalf("gate=%+v", result.Gate)
	}
}

func TestExecuteFailFastAndExplicitFailureKinds(t *testing.T) {
	tests := []struct {
		name    string
		rr      runner.Result
		outcome Outcome
	}{
		{"ordinary", runner.Result{ExitCode: 2, Stderr: "no"}, OutcomeFailed},
		{"timeout", runner.Result{ExitCode: -1, TimedOut: true, Err: context.DeadlineExceeded}, OutcomeTimedOut},
		{"cancelled", runner.Result{ExitCode: -1, Err: context.Canceled}, OutcomeCancelled},
		{"runner", runner.Result{ExitCode: -1, Err: errors.New("setup")}, OutcomeRunnerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mustMkdir(t, filepath.Join(root, "checks"))
			calls := 0
			cfg := executionConfig(root, allTierPlan(), PurposeFinal, func(context.Context, runner.Command) runner.Result { calls++; return tt.rr })
			result, err := Execute(context.Background(), cfg)
			if err == nil {
				t.Fatal("expected classified error")
			}
			if result.Outcome != tt.outcome || calls != 1 || len(result.Tiers) != 1 || result.Gate.FinalSatisfied {
				t.Fatalf("result=%+v calls=%d", result, calls)
			}
			a := result.Tiers[0].Commands[0].Attempts[0]
			if tt.outcome == OutcomeTimedOut && !a.TimedOut {
				t.Fatal("timeout fact missing")
			}
			if tt.outcome == OutcomeCancelled && !a.Cancelled {
				t.Fatal("cancel fact missing")
			}
		})
	}
}

func TestExecuteMissingCommandsAndCancellationBeforeStart(t *testing.T) {
	root := t.TempDir()
	plan := AdaptLegacy(nil)
	cfg := executionConfig(root, plan, PurposeFinal, passRunner)
	result, err := Execute(context.Background(), cfg)
	if err == nil || result.Outcome != OutcomeMissing || len(result.Tiers) != 1 || result.Tiers[0].Outcome != OutcomeMissing {
		t.Fatalf("missing result=%+v err=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	cfg = executionConfig(root, AdaptLegacy([]verification.Command{{Name: "x"}}), PurposeFinal, func(context.Context, runner.Command) runner.Result { calls++; return runner.Result{} })
	result, err = Execute(ctx, cfg)
	if err == nil || result.Outcome != OutcomeCancelled || calls != 0 {
		t.Fatalf("cancel result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestExecuteControlledRerunClassifiesFlakyAndFailFail(t *testing.T) {
	for _, tt := range []struct {
		name   string
		second runner.Result
		want   Outcome
	}{{"fail-pass", runner.Result{ExitCode: 0}, OutcomeFlaky}, {"fail-fail", runner.Result{ExitCode: 3}, OutcomeFailed}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			plan := AdaptLegacy([]verification.Command{{Name: "check"}})
			plan.Tiers[0].RerunPolicy = RerunOnceToClassifyFlaky
			calls := 0
			cfg := executionConfig(root, plan, PurposeFinal, func(context.Context, runner.Command) runner.Result {
				calls++
				if calls == 1 {
					return runner.Result{ExitCode: 2, Stderr: "first"}
				}
				return tt.second
			})
			result, err := Execute(context.Background(), cfg)
			if err == nil || result.Outcome != tt.want || calls != 2 {
				t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
			}
			cr := result.Tiers[0].Commands[0]
			if len(cr.Attempts) != 2 || cr.Attempts[0].Passed || cr.Attempts[1].Command.SHA256 != cr.Attempts[0].Command.SHA256 || cr.Attempts[1].Number != 2 || result.Gate.FinalSatisfied {
				t.Fatalf("command=%+v gate=%+v", cr, result.Gate)
			}
			if result.Aggregate.Passed || result.Aggregate.Status != verification.StatusFailed || len(result.Aggregate.Commands) != 2 {
				t.Fatalf("aggregate=%+v", result.Aggregate)
			}
		})
	}
}

func TestExecuteNeverRerunsIneligibleOrDisabled(t *testing.T) {
	for _, tt := range []struct {
		name   string
		policy RerunPolicy
		rr     runner.Result
	}{{"disabled", RerunNever, runner.Result{ExitCode: 2}}, {"timeout", RerunOnceToClassifyFlaky, runner.Result{TimedOut: true, Err: context.DeadlineExceeded}}, {"cancel", RerunOnceToClassifyFlaky, runner.Result{Err: context.Canceled}}, {"runner", RerunOnceToClassifyFlaky, runner.Result{Err: errors.New("x")}}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			plan := AdaptLegacy([]verification.Command{{Name: "check"}})
			plan.Tiers[0].RerunPolicy = tt.policy
			calls := 0
			cfg := executionConfig(root, plan, PurposeFinal, func(context.Context, runner.Command) runner.Result { calls++; return tt.rr })
			result, _ := Execute(context.Background(), cfg)
			if calls != 1 || len(result.Tiers[0].Commands[0].Attempts) != 1 {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
		})
	}
}

func TestExecuteArtifactLedgerAndFailures(t *testing.T) {
	root := t.TempDir()
	store := &memoryLedger{}
	cfg := executionConfig(root, AdaptLegacy([]verification.Command{{Name: "check"}}), PurposeFinal, passRunner)
	cfg.Ledger = store
	cfg.ArtifactPath = filepath.Join(".revolvr", "runs", "worker", "verification.json")
	result, err := Execute(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact == nil {
		t.Fatal("artifact missing")
	}
	raw, err := os.ReadFile(filepath.Join(root, cfg.ArtifactPath))
	if err != nil {
		t.Fatal(err)
	}
	var decoded Result
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Outcome != OutcomePassed {
		t.Fatalf("artifact decode=%v %+v", err, decoded)
	}
	want := []ledger.EventType{ledger.EventVerificationStarted, ledger.EventVerificationTierStarted, ledger.EventVerificationTierCompleted, ledger.EventVerificationCompleted}
	if !reflect.DeepEqual(store.types, want) {
		t.Fatalf("events=%v want %v", store.types, want)
	}
	cfg = executionConfig(root, AdaptLegacy([]verification.Command{{Name: "check"}}), PurposeFinal, passRunner)
	cfg.ArtifactPath = "verification.json"
	cfg.ArtifactWriter = func(string, string, []byte) error { return errors.New("disk full") }
	result, err = Execute(context.Background(), cfg)
	if err == nil || result.Outcome != OutcomeArtifactError || result.FailureStage != "artifact_write" {
		t.Fatalf("artifact failure=%+v err=%v", result, err)
	}
	store = &memoryLedger{failAt: 1}
	cfg = executionConfig(root, AdaptLegacy([]verification.Command{{Name: "check"}}), PurposeFinal, passRunner)
	cfg.Ledger = store
	result, err = Execute(context.Background(), cfg)
	if err == nil || result.Outcome != OutcomeLedgerError {
		t.Fatalf("ledger failure=%+v err=%v", result, err)
	}
}

type memoryLedger struct {
	types  []ledger.EventType
	failAt int
}

func (m *memoryLedger) AppendEvent(_ context.Context, _ string, kind ledger.EventType, _ any) (ledger.Event, error) {
	m.types = append(m.types, kind)
	if m.failAt > 0 && len(m.types) == m.failAt {
		return ledger.Event{}, errors.New("ledger unavailable")
	}
	return ledger.Event{Type: kind}, nil
}

func executionConfig(root string, plan Plan, purpose Purpose, run CommandRunner) Config {
	tick := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	seq := 0
	return Config{RepositoryRoot: root, TaskID: "task-1", RunID: "worker-run", OccurrenceID: "verification-one", SourceRevision: strings.Repeat("a", 64), Plan: plan, Purpose: purpose, Timeout: time.Minute, StdoutCap: 100, StderrCap: 101, Clock: func() time.Time { tick = tick.Add(time.Second); return tick }, AttemptID: func() string { seq++; return "attempt-" + string(rune('0'+seq)) }, CommandRunner: run}
}
func passRunner(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 0} }
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}
