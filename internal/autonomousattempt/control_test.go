package autonomousattempt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/codexexec"
	"revolvr/internal/receipt"
)

func TestTransientFailureCanRecoverWithoutErasingAccounting(t *testing.T) {
	fixture := newAttemptFixture(t, "task-1", baseAttemptState("task-1"))
	signature, err := OperationFailureSignature(OperationFailureMaterial{TaskID: "task-1", Action: autonomous.ActionImplement, Stage: "worker", Classification: "transient", Evidence: []autonomous.EvidenceReference{evidence("failure-one")}})
	if err != nil {
		t.Fatal(err)
	}
	first := fixture.admit(t, "attempt-one", strategy("edit parser"))
	failed := fixture.complete(t, first, "attempt-one", autonomous.AttemptOutcomeFailed, hash("source-two"), int64ptr(10), []autonomous.CanonicalSignature{signature})
	if failed.Current.State.Attempts.ConsecutiveFailures != 1 || failed.Current.State.Attempts.LastFailure == nil {
		t.Fatalf("failed accounting = %+v", failed.Current.State.Attempts)
	}
	fixture.snapshot = failed.Current
	second := fixture.admit(t, "attempt-two", strategy("edit parser"))
	passed := fixture.complete(t, second, "attempt-two", autonomous.AttemptOutcomeSucceeded, hash("source-three"), int64ptr(12), nil)
	a := passed.Current.State.Attempts
	if a.TotalAttempts != 2 || a.RetryBudget.Consumed != 2 || a.TokenBudget.Consumed != 22 || a.ElapsedTimeBudget.Consumed != 4*time.Second || a.ConsecutiveFailures != 0 || a.LastFailure == nil || len(a.Events) != 4 {
		t.Fatalf("recovered accounting = %+v", a)
	}
	if a.Events[1].Outcome != autonomous.AttemptOutcomeFailed || a.Events[3].Outcome != autonomous.AttemptOutcomeSucceeded {
		t.Fatalf("attempt events were overwritten: %+v", a.Events)
	}
}

func TestTaskAttemptBoundariesAndStaleWriter(t *testing.T) {
	tests := []struct {
		name                   string
		limit, consumed, total int64
		wantBlocked            bool
	}{
		{"below", 2, 1, 1, false}, {"at", 1, 1, 1, true}, {"above", 1, 2, 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := configuredState("task-1", tt.limit, tt.consumed, tt.total)
			fixture := newAttemptFixture(t, "task-1", state)
			fixture.limits.TaskAttempts.Limit = tt.limit
			result := fixture.admit(t, "attempt-boundary", strategy("bounded work"))
			if got := result.Disposition == DispositionBlocked; got != tt.wantBlocked {
				t.Fatalf("blocked=%t want %t result=%+v", got, tt.wantBlocked, result)
			}
			if tt.wantBlocked && result.Reason != autonomous.BreakerTaskAttemptsExhausted {
				t.Fatalf("reason=%q", result.Reason)
			}
		})
	}

	fixture := newAttemptFixture(t, "task-stale", baseAttemptState("task-stale"))
	stale := fixture.snapshot.Expected()
	first := fixture.admit(t, "attempt-one", strategy("first"))
	cfg := fixture.admission("attempt-stale", strategy("stale"))
	cfg.Expected = stale
	_, err := Admit(context.Background(), cfg)
	if !errors.Is(err, autonomousstate.ErrStaleWrite) {
		t.Fatalf("stale error=%v", err)
	}
	loaded, _, err := fixture.store.Load(context.Background(), fixture.taskID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State.Attempts.TotalAttempts != 1 || loaded.SHA256 != first.Current.SHA256 {
		t.Fatalf("stale writer changed accounting: %+v", loaded.State.Attempts)
	}
}

func TestBudgetModesRemainExplicit(t *testing.T) {
	for _, mode := range []autonomous.BudgetMode{autonomous.BudgetModeUnset, autonomous.BudgetModeUnlimited} {
		t.Run(string(mode), func(t *testing.T) {
			fixture := newAttemptFixture(t, "task-"+string(mode), baseAttemptState("task-"+string(mode)))
			fixture.limits = Limits{TaskAttempts: autonomous.CountBudget{Mode: mode}, ActionAttempts: []autonomous.ActionBudget{{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: mode}}}, Elapsed: autonomous.DurationBudget{Mode: mode}, Tokens: autonomous.CountBudget{Mode: mode}, RepeatedSignatureLimit: 2}
			if result := fixture.admit(t, "attempt-one", strategy("explicit mode")); result.Disposition != DispositionAdmitted {
				t.Fatalf("mode %s result=%+v", mode, result)
			}
		})
	}
	limited := newAttemptFixture(t, "task-zero", baseAttemptState("task-zero"))
	limited.limits.TaskAttempts = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 0}
	if result := limited.admit(t, "attempt-one", strategy("zero limit")); result.Reason != autonomous.BreakerTaskAttemptsExhausted {
		t.Fatalf("zero limited budget=%+v", result)
	}
	maximum := int64(^uint64(0) >> 1)
	overflowState := baseAttemptState("task-overflow")
	overflowState.Attempts.TotalAttempts = maximum
	overflowState.Attempts.RetryBudget = autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited, Consumed: maximum}
	overflowState.Attempts.ActionAttempts = []autonomous.ActionAttempt{{Action: autonomous.ActionImplement, Attempts: maximum}}
	overflowState.Attempts.ActionBudgets = []autonomous.ActionBudget{{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited, Consumed: maximum}}}
	overflowState.Attempts.ElapsedTimeBudget = autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}
	overflowState.Attempts.TokenBudget = autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}
	overflowState.Attempts.RepeatedSignatureLimit = 2
	overflow := newAttemptFixture(t, "task-overflow", overflowState)
	overflow.limits = Limits{TaskAttempts: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}, ActionAttempts: []autonomous.ActionBudget{{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}}}, Elapsed: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, Tokens: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}, RepeatedSignatureLimit: 2}
	if result := overflow.admit(t, "attempt-one", strategy("overflow")); result.Reason != autonomous.BreakerAccountingSafety {
		t.Fatalf("overflow did not fail closed: %+v", result)
	}
}

func TestPerActionExhaustionDoesNotConsumeOtherAction(t *testing.T) {
	state := configuredState("task-1", 4, 1, 1)
	state.Attempts.ActionAttempts = []autonomous.ActionAttempt{{Action: autonomous.ActionImplement, Attempts: 1}}
	state.Attempts.ActionBudgets = []autonomous.ActionBudget{
		{Action: autonomous.ActionAudit, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 2}},
		{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 1, Consumed: 1}},
	}
	fixture := newAttemptFixture(t, "task-1", state)
	fixture.limits.ActionAttempts = []autonomous.ActionBudget{
		{Action: autonomous.ActionAudit, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 2}},
		{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 1}},
	}
	blocked := fixture.admit(t, "attempt-implement", strategy("implement again"))
	if blocked.Reason != autonomous.BreakerActionAttemptsExhausted || blocked.Current.State.Lifecycle != autonomous.LifecycleStateReady || blocked.Current.State.Attempts.TotalAttempts != 1 {
		t.Fatalf("action stop=%+v", blocked)
	}
	if len(blocked.Current.State.Attempts.ActionStops) != 1 {
		t.Fatalf("action stop was not durable: %+v", blocked.Current.State.Attempts)
	}
	fixture.snapshot = blocked.Current
	fixture.action = autonomous.ActionAudit
	fixture.profile = autonomous.WorkerProfileAuditor
	allowed := fixture.admit(t, "attempt-audit", strategy("audit instead"))
	if allowed.Disposition != DispositionAdmitted || allowed.Current.State.Attempts.TotalAttempts != 2 {
		t.Fatalf("unrelated action admission=%+v", allowed)
	}
}

func TestNoProgressRequiresChangedStrategyAndRepeatedSignaturesTerminate(t *testing.T) {
	fixture := newAttemptFixture(t, "task-1", baseAttemptState("task-1"))
	fixture.limits.RepeatedSignatureLimit = 3
	first := fixture.admit(t, "attempt-one", strategy("same plan"))
	noProgress := fixture.complete(t, first, "attempt-one", autonomous.AttemptOutcomeSucceeded, fixture.source, int64ptr(5), nil)
	if noProgress.Current.State.Attempts.RequiredStrategyChangeFrom == "" || noProgress.Current.State.Lifecycle != autonomous.LifecycleStateReady {
		t.Fatalf("no-progress state=%+v", noProgress.Current.State)
	}
	fixture.snapshot = noProgress.Current
	identical := fixture.admit(t, "attempt-two", strategy("  SAME   plan "))
	if identical.Reason != autonomous.BreakerIdenticalStrategy || identical.Current.State.CircuitBreaker == nil {
		t.Fatalf("identical strategy result=%+v", identical)
	}

	changedFixture := newAttemptFixture(t, "task-2", baseAttemptState("task-2"))
	changedFixture.limits.RepeatedSignatureLimit = 3
	first = changedFixture.admit(t, "attempt-one", strategy("same plan"))
	noProgress = changedFixture.complete(t, first, "attempt-one", autonomous.AttemptOutcomeSucceeded, changedFixture.source, int64ptr(5), nil)
	changedFixture.snapshot = noProgress.Current
	changed := changedFixture.admit(t, "attempt-two", strategy("different typed plan"))
	if changed.Disposition != DispositionAdmitted {
		t.Fatalf("changed strategy rejected: %+v", changed)
	}
	passed := changedFixture.complete(t, changed, "attempt-two", autonomous.AttemptOutcomeSucceeded, hash("changed-source"), int64ptr(5), nil)
	if passed.Current.State.Attempts.RequiredStrategyChangeFrom != "" {
		t.Fatalf("successful changed strategy did not clear requirement")
	}

	repeat := newAttemptFixture(t, "task-3", baseAttemptState("task-3"))
	sig, err := DecisionSignature(repeat.decision())
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		admission := repeat.admit(t, fmt.Sprintf("attempt-%d", i), strategy("loop"))
		completion := repeat.complete(t, admission, fmt.Sprintf("attempt-%d", i), autonomous.AttemptOutcomeFailed, hash(fmt.Sprintf("failed-%d", i)), int64ptr(4), []autonomous.CanonicalSignature{sig})
		if i == 1 {
			if completion.Disposition == DispositionBlocked {
				t.Fatal("one transient signature tripped breaker")
			}
			repeat.snapshot = completion.Current
		} else if completion.Reason != autonomous.BreakerRepeatedSignature {
			t.Fatalf("repeated signature result=%+v", completion)
		}
	}
}

func TestEveryExactFailureSignatureKindTerminatesAtConfiguredThreshold(t *testing.T) {
	tests := []struct {
		name string
		make func(string) autonomous.CanonicalSignature
	}{
		{"decision", func(task string) autonomous.CanonicalSignature {
			decision := autonomous.SupervisorDecision{TaskID: task, Action: autonomous.ActionImplement, WorkerProfile: autonomous.WorkerProfileImplementer, Rationale: "incidental prose", SuccessCriteria: []string{"works"}, Inputs: []autonomous.EvidenceReference{evidence("input")}}
			signature, err := DecisionSignature(decision)
			if err != nil {
				t.Fatal(err)
			}
			return signature
		}},
		{"verification", func(task string) autonomous.CanonicalSignature {
			target := autonomous.VerificationFailureTarget{TaskID: task, RunID: "verify-run", OccurrenceID: "verify-occurrence", SourceRevision: hash("verify-source"), Status: autonomous.VerificationStatusFailed, Evidence: []autonomous.EvidenceReference{evidence("verify")}}
			signature, err := VerificationFailureSignature(VerificationFailureMaterial{Target: target, Classification: "failed", TierID: "focused", CommandSHA256: hash("command")})
			if err != nil {
				t.Fatal(err)
			}
			return signature
		}},
		{"open-findings", func(task string) autonomous.CanonicalSignature {
			signature, err := OpenFindingSignature(OpenFindingMaterial{TaskID: task, AuditRunID: "audit-occurrence", ReportSHA256: hash("report"), ReportBytes: 12, Findings: []FindingIdentity{{ID: "finding-one", ReportSHA256: hash("report"), ReportBytes: 12, FindingSHA256: hash("finding")}}})
			if err != nil {
				t.Fatal(err)
			}
			return signature
		}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := fmt.Sprintf("task-signature-%d", i)
			fixture := newAttemptFixture(t, task, baseAttemptState(task))
			signature := tt.make(task)
			for attempt := 1; attempt <= 2; attempt++ {
				id := fmt.Sprintf("attempt-%d", attempt)
				admitted := fixture.admit(t, id, strategy("repeat strategy"))
				completed := fixture.complete(t, admitted, id, autonomous.AttemptOutcomeFailed, hash(fmt.Sprintf("source-%d", attempt)), int64ptr(1), []autonomous.CanonicalSignature{signature})
				if attempt == 1 {
					fixture.snapshot = completed.Current
					continue
				}
				if completed.Reason != autonomous.BreakerRepeatedSignature || completed.Current.State.CircuitBreaker == nil || completed.Current.State.CircuitBreaker.TriggerSignature == nil {
					t.Fatalf("second %s signature did not terminate: %+v", tt.name, completed)
				}
			}
		})
	}

	first, err := OpenFindingSignature(OpenFindingMaterial{TaskID: "task", AuditRunID: "audit-one", ReportSHA256: hash("report"), ReportBytes: 12, Findings: []FindingIdentity{{ID: "finding-one", ReportSHA256: hash("report"), ReportBytes: 12, FindingSHA256: hash("finding")}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenFindingSignature(OpenFindingMaterial{TaskID: "task", AuditRunID: "audit-two", ReportSHA256: hash("report"), ReportBytes: 12, Findings: []FindingIdentity{{ID: "finding-one", ReportSHA256: hash("report"), ReportBytes: 12, FindingSHA256: hash("finding")}}})
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 == second.SHA256 {
		t.Fatal("independent newer audit occurrence was not distinct")
	}
}

func TestTokenElapsedRestartReplayAndLimitAuthority(t *testing.T) {
	fixture := newAttemptFixture(t, "task-1", baseAttemptState("task-1"))
	fixture.limits.Elapsed = autonomous.DurationBudget{Mode: autonomous.BudgetModeLimited, Limit: 2 * time.Second}
	fixture.limits.Tokens = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 10}
	admission := fixture.admit(t, "attempt-one", strategy("measured"))
	completion := fixture.completeDuration(t, admission, "attempt-one", autonomous.AttemptOutcomeFailed, hash("failed"), int64ptr(10), nil, 2*time.Second)
	fixture.snapshot = completion.Current

	reopened, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
	if err != nil {
		t.Fatal(err)
	}
	loaded, found, err := reopened.Load(context.Background(), "task-1")
	if err != nil || !found {
		t.Fatalf("reopen=%t err=%v", found, err)
	}
	if loaded.State.Attempts.TokenBudget.Consumed != 10 || loaded.State.Attempts.ElapsedTimeBudget.Consumed != 2*time.Second || len(loaded.State.Attempts.Events) != 2 {
		t.Fatalf("reopened state=%+v", loaded.State.Attempts)
	}
	fixture.store, fixture.snapshot = reopened, loaded
	exhausted := fixture.admit(t, "attempt-two", strategy("retry"))
	if exhausted.Reason != autonomous.BreakerElapsedExhausted {
		t.Fatalf("boundary reason=%q", exhausted.Reason)
	}

	replay, err := Complete(context.Background(), fixture.completion(admission, "attempt-one", autonomous.AttemptOutcomeFailed, hash("failed"), int64ptr(10), nil, 2*time.Second))
	if err != nil || replay.Disposition != DispositionReplayed {
		t.Fatalf("completion replay=%+v err=%v", replay, err)
	}

	missing := newAttemptFixture(t, "task-missing", baseAttemptState("task-missing"))
	missing.limits.Tokens = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 10}
	a := missing.admit(t, "attempt-one", strategy("limited"))
	stopped := missing.complete(t, a, "attempt-one", autonomous.AttemptOutcomeFailed, hash("failed"), nil, nil)
	if stopped.Reason != autonomous.BreakerMissingTrustedMetrics || stopped.Current.State.Attempts.TokenBudget.Consumed != 0 {
		t.Fatalf("missing metrics stop=%+v", stopped)
	}
	malformed := newAttemptFixture(t, "task-malformed", baseAttemptState("task-malformed"))
	malformed.limits.Tokens = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 10}
	a = malformed.admit(t, "attempt-one", strategy("limited"))
	stopped = malformed.complete(t, a, "attempt-one", autonomous.AttemptOutcomeFailed, hash("failed"), int64ptr(-1), nil)
	if stopped.Reason != autonomous.BreakerMissingTrustedMetrics || stopped.Current.State.Attempts.Events[1].Tokens != nil {
		t.Fatalf("malformed metrics stop=%+v", stopped)
	}
	badEvidence := newAttemptFixture(t, "task-bad-evidence", baseAttemptState("task-bad-evidence"))
	a = badEvidence.admit(t, "attempt-one", strategy("validate evidence"))
	stopped = badEvidence.complete(t, a, "attempt-one", autonomous.AttemptOutcomeFailed, hash("failed"), int64ptr(1), []autonomous.CanonicalSignature{{Kind: autonomous.SignatureKindOperationFailure, SHA256: "bad"}})
	if stopped.Reason != autonomous.BreakerAccountingSafety || stopped.Current.State.Attempts.ElapsedTimeBudget.Consumed != 2*time.Second {
		t.Fatalf("malformed evidence stop=%+v", stopped)
	}

	authority := newAttemptFixture(t, "task-authority", baseAttemptState("task-authority"))
	a = authority.admit(t, "attempt-one", strategy("authority"))
	authority.snapshot = a.Current
	authority.limits.TaskAttempts.Limit++
	reset := authority.admit(t, "attempt-two", strategy("authority two"))
	if reset.Reason != autonomous.BreakerAccountingSafety || reset.Current.State.Attempts.RetryBudget.Limit == authority.limits.TaskAttempts.Limit {
		t.Fatalf("limit reset was accepted: %+v", reset)
	}
	strategyAuthority := newAttemptFixture(t, "task-strategy-authority", baseAttemptState("task-strategy-authority"))
	cfg := strategyAuthority.admission("attempt-one", strategy("validated strategy"))
	cfg.Strategy = strategy("unvalidated substituted strategy")
	if _, err := Admit(context.Background(), cfg); err == nil {
		t.Fatal("caller substituted strategy not present in validated supervisor decision")
	}
}

func TestOrphanHistoryRetryRecoversWithoutDoubleCharge(t *testing.T) {
	fixture := newAttemptFixture(t, "task-crash", baseAttemptState("task-crash"))
	injected := false
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo, FailureInjector: func(point autonomousstate.FailurePoint) error {
		if point == autonomousstate.FailureAfterHistoryWrite && !injected {
			injected = true
			return errors.New("crash")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.store = store
	cfg := fixture.admission("attempt-one", strategy("recover operation"))
	if _, err := Admit(context.Background(), cfg); err == nil {
		t.Fatal("injected pre-state crash unexpectedly succeeded")
	}
	cleanStore, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Store = cleanStore
	recovered, err := Admit(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Current.State.Attempts.TotalAttempts != 1 || len(recovered.Current.State.Attempts.Events) != 1 {
		t.Fatalf("recovered accounting=%+v", recovered.Current.State.Attempts)
	}
	history, found, err := cleanStore.LoadAttemptOperation(context.Background(), fixture.taskID, cfg.OperationID)
	if err != nil || !found || history.Record.Event.AttemptID != "attempt-one" {
		t.Fatalf("history found=%t record=%+v err=%v", found, history.Record, err)
	}
}

func TestTaskIsolationAndSignatureCanonicalization(t *testing.T) {
	repo := t.TempDir()
	first := newAttemptFixtureAt(t, repo, "task-a", configuredState("task-a", 1, 1, 1))
	first.limits.TaskAttempts.Limit = 1
	second := newAttemptFixtureAt(t, repo, "task-b", baseAttemptState("task-b"))
	blocked := first.admit(t, "attempt-a", strategy("exhausted"))
	if blocked.Reason != autonomous.BreakerTaskAttemptsExhausted {
		t.Fatalf("task-a=%+v", blocked)
	}
	allowed := second.admit(t, "attempt-b", strategy("clean"))
	if allowed.Disposition != DispositionAdmitted || allowed.Current.State.TaskID != "task-b" {
		t.Fatalf("task-b=%+v", allowed)
	}

	one, err := strategy(" Parse   AST with tokens ").Signature()
	if err != nil {
		t.Fatal(err)
	}
	two, err := (Strategy{Approach: "parse ast with TOKENS"}).Signature()
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatalf("format-only strategy changed signature: %s != %s", one, two)
	}
	three, _ := strategy("rewrite parser state machine").Signature()
	if one == three {
		t.Fatal("materially changed strategy retained signature")
	}

	target := autonomous.VerificationFailureTarget{TaskID: "task-b", RunID: "run", OccurrenceID: "occurrence", SourceRevision: hash("source"), Status: autonomous.VerificationStatusFailed, Evidence: []autonomous.EvidenceReference{evidence("verification")}}
	verification, err := VerificationFailureSignature(VerificationFailureMaterial{Target: target, Classification: "failed", TierID: "focused", CommandSHA256: hash("command")})
	if err != nil || verification.Kind != autonomous.SignatureKindVerificationFailure {
		t.Fatalf("verification signature=%+v err=%v", verification, err)
	}
	finding, err := OpenFindingSignature(OpenFindingMaterial{TaskID: "task-b", AuditRunID: "audit-run", ReportSHA256: hash("report"), ReportBytes: 10, Findings: []FindingIdentity{{ID: "finding-one", ReportSHA256: hash("report"), ReportBytes: 10, FindingSHA256: hash("finding")}}})
	if err != nil || finding.Kind != autonomous.SignatureKindOpenFindings {
		t.Fatalf("finding signature=%+v err=%v", finding, err)
	}
}

func TestExecuteUsesTrustedTimingAndNeverRunsAfterAdmissionStop(t *testing.T) {
	fixture := newAttemptFixture(t, "task-execute", baseAttemptState("task-execute"))
	fixture.limits.Elapsed.Limit = 10 * time.Minute
	admission := fixture.admission("attempt-one", strategy("one bounded run"))
	times := []time.Time{fixture.now.Add(10 * time.Minute), fixture.now.Add(13 * time.Minute)}
	clockCalls, runnerCalls := 0, 0
	result, err := Execute(context.Background(), ExecuteConfig{
		Admission: admission, CompletionOperationID: "complete-attempt-one",
		Clock: func() time.Time { value := times[clockCalls]; clockCalls++; return value },
		Runner: func(context.Context) (Observation, error) {
			runnerCalls++
			return Observation{RunID: "worker-one", SourceAfter: hash("execute-source"), Outcome: autonomous.AttemptOutcomeSucceeded, Tokens: int64ptr(7), Evidence: []autonomous.EvidenceReference{evidence("execute")}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runnerCalls != 1 || result.Completion.Current.State.Attempts.ElapsedTimeBudget.Consumed != 3*time.Minute {
		t.Fatalf("execution result=%+v runner=%d", result, runnerCalls)
	}

	blockedFixture := newAttemptFixture(t, "task-blocked-runner", configuredState("task-blocked-runner", 1, 1, 1))
	blockedFixture.limits.TaskAttempts.Limit = 1
	runnerCalls = 0
	blockedAdmission := blockedFixture.admission("attempt-two", strategy("must not run"))
	blockedResult, err := Execute(context.Background(), ExecuteConfig{Admission: blockedAdmission, CompletionOperationID: "complete-attempt-two", Clock: time.Now, Runner: func(context.Context) (Observation, error) { runnerCalls++; return Observation{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if runnerCalls != 0 || blockedResult.Admission.Reason != autonomous.BreakerTaskAttemptsExhausted {
		t.Fatalf("blocked execution=%+v calls=%d", blockedResult, runnerCalls)
	}
	unsafeFixture := newAttemptFixture(t, "task-unsafe-runner", baseAttemptState("task-unsafe-runner"))
	unsafeAdmission := unsafeFixture.admission("attempt-unsafe", strategy("must not write"))
	unsafeAdmission.SourceSafety = autonomouspolicy.SourceSafetyUnsafe
	unsafeResult, err := Execute(context.Background(), ExecuteConfig{Admission: unsafeAdmission, CompletionOperationID: "complete-attempt-unsafe", Clock: time.Now, Runner: func(context.Context) (Observation, error) { runnerCalls++; return Observation{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if runnerCalls != 0 || unsafeResult.Admission.Reason != autonomous.BreakerUnsafeSource {
		t.Fatalf("unsafe execution=%+v calls=%d", unsafeResult, runnerCalls)
	}
}

func TestExecuteCancellationRetainsAdmittedAccounting(t *testing.T) {
	fixture := newAttemptFixture(t, "task-cancel", baseAttemptState("task-cancel"))
	ctx, cancel := context.WithCancel(context.Background())
	times := []time.Time{fixture.now.Add(time.Hour), fixture.now.Add(time.Hour + time.Second)}
	clockCalls := 0
	result, err := Execute(ctx, ExecuteConfig{
		Admission: fixture.admission("attempt-one", strategy("cancelled work")), CompletionOperationID: "complete-attempt-one",
		Clock: func() time.Time { value := times[clockCalls]; clockCalls++; return value },
		Runner: func(context.Context) (Observation, error) {
			cancel()
			return Observation{RunID: "worker-cancel", SourceAfter: hash("cancel-source"), Tokens: int64ptr(3), Evidence: []autonomous.EvidenceReference{evidence("cancel")}}, context.Canceled
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error=%v", err)
	}
	if result.Completion.Reason != autonomous.BreakerCancellation || result.Completion.Current.State.Attempts.TotalAttempts != 1 || len(result.Completion.Current.State.Attempts.Events) != 2 {
		t.Fatalf("cancel accounting=%+v", result)
	}
}

func TestBreakerPreservesPriorPlanFindingAndDecisionEvidence(t *testing.T) {
	state := baseAttemptState("task-preserve")
	state.Plan = &autonomous.TaskPlan{TaskID: "task-preserve", ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{evidence("plan")}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "work", Status: autonomous.PlanStepStatusPending}}}
	state.FindingResolutions = []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}}
	priorPlan := *state.Plan
	priorFindings := append([]autonomous.FindingResolution(nil), state.FindingResolutions...)
	fixture := newAttemptFixture(t, "task-preserve", state)
	fixture.limits.RepeatedSignatureLimit = 1
	signature, err := OperationFailureSignature(OperationFailureMaterial{TaskID: "task-preserve", Action: autonomous.ActionImplement, Stage: "commit", Classification: "failed", Evidence: []autonomous.EvidenceReference{evidence("commit-failure")}})
	if err != nil {
		t.Fatal(err)
	}
	admitted := fixture.admit(t, "attempt-one", strategy("preserve history"))
	blocked := fixture.complete(t, admitted, "attempt-one", autonomous.AttemptOutcomeFailed, hash("failed-source"), int64ptr(2), []autonomous.CanonicalSignature{signature})
	if blocked.Reason != autonomous.BreakerRepeatedSignature || !reflect.DeepEqual(*blocked.Current.State.Plan, priorPlan) || !reflect.DeepEqual(blocked.Current.State.FindingResolutions, priorFindings) || blocked.Current.State.LatestDecision != state.LatestDecision {
		t.Fatalf("breaker rewrote prior state: %+v", blocked.Current.State)
	}
}

func TestCycleObservationUsesTrustedExecutionMetricsAndTypedStops(t *testing.T) {
	cycle := autonomouscycle.Result{TaskID: "task", Outcome: autonomouscycle.OutcomeWorkerNoChanges, Source: autonomouscycle.SourceEvidence{AdmissionRevision: hash("before"), FinalRevision: hash("after")}}
	cycle.Supervisor.RunID = "supervisor"
	cycle.Supervisor.Codex = codexexec.Result{UsageFound: true, Usage: receipt.Metrics{InputTokens: 3, OutputTokens: 2}}
	cycle.Worker.Started, cycle.Worker.RunID = true, "worker"
	cycle.Worker.Codex = codexexec.Result{UsageFound: true, Usage: receipt.Metrics{InputTokens: 4, OutputTokens: 1}}
	observation := ObserveCycle(cycle, nil)
	if observation.Outcome != autonomous.AttemptOutcomeNoProgress || observation.Tokens == nil || *observation.Tokens != 10 {
		t.Fatalf("cycle observation=%+v", observation)
	}
	cycle.Worker.Codex.UsageFound = false
	if got := ObserveCycle(cycle, nil).Tokens; got != nil {
		t.Fatalf("ambiguous usage=%d, want nil", *got)
	}
	cycle.Outcome = autonomouscycle.OutcomeSourceChanged
	unsafe := ObserveCycle(cycle, errors.New("source changed"))
	if unsafe.Outcome != autonomous.AttemptOutcomeSafetyStopped || unsafe.StopReason != autonomous.BreakerUnsafeSource {
		t.Fatalf("unsafe observation=%+v", unsafe)
	}
}

type attemptFixture struct {
	t            *testing.T
	repo, taskID string
	store        *autonomousstate.Store
	snapshot     autonomousstate.Snapshot
	action       autonomous.Action
	profile      autonomous.WorkerProfile
	limits       Limits
	source       string
	now          time.Time
	sequence     int
}

func newAttemptFixture(t *testing.T, taskID string, state autonomous.ExecutionState) *attemptFixture {
	return newAttemptFixtureAt(t, t.TempDir(), taskID, state)
}

func newAttemptFixtureAt(t *testing.T, repo, taskID string, state autonomous.ExecutionState) *attemptFixture {
	t.Helper()
	taskRaw := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\n---\n# Task\n\nTest.\n", taskID, taskID))
	taskPath := filepath.Join(repo, ".agent", "tasks", taskID+".md")
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath, taskRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(repo, ".revolvr", "autonomous", "tasks", taskID, "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("load=%t err=%v", found, err)
	}
	return &attemptFixture{t: t, repo: repo, taskID: taskID, store: store, snapshot: snapshot, action: autonomous.ActionImplement, profile: autonomous.WorkerProfileImplementer, limits: defaultLimits(), source: hash("source-one"), now: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)}
}

func (f *attemptFixture) decision() autonomous.SupervisorDecision {
	return autonomous.SupervisorDecision{TaskID: f.taskID, Action: f.action, WorkerProfile: f.profile, Rationale: "test", SuccessCriteria: []string{"works"}, Inputs: []autonomous.EvidenceReference{evidence("input")}}
}
func (f *attemptFixture) reference(id string) autonomous.DecisionReference {
	return autonomous.DecisionReference{DecisionID: "decision-" + id, RunID: "supervisor-" + id, TaskID: f.taskID, Action: f.action, WorkerProfile: f.profile, Artifact: evidence("decision-" + id), CreatedAt: f.now}
}
func (f *attemptFixture) admission(id string, s Strategy) AdmissionConfig {
	f.sequence++
	decision := f.decision()
	decision.Strategy = &autonomous.Strategy{Approach: s.Approach, Techniques: append([]string(nil), s.Techniques...), Targets: append([]autonomous.EvidenceReference(nil), s.Targets...)}
	return AdmissionConfig{TaskID: f.taskID, OperationID: "admit-" + id, AttemptID: id, Expected: f.snapshot.Expected(), Action: f.action, Decision: decision, Reference: f.reference(id), Strategy: s, SourceRevision: f.source, SourceSafety: autonomouspolicy.SourceSafetySafe, Limits: f.limits, CreatedAt: f.now.Add(time.Duration(f.sequence) * time.Minute), Store: f.store}
}
func (f *attemptFixture) admit(t *testing.T, id string, s Strategy) Result {
	t.Helper()
	result, err := Admit(context.Background(), f.admission(id, s))
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func (f *attemptFixture) completion(admission Result, id string, outcome autonomous.AttemptOutcome, source string, tokens *int64, signatures []autonomous.CanonicalSignature, duration time.Duration) CompletionConfig {
	return CompletionConfig{TaskID: f.taskID, OperationID: "complete-" + id, AttemptID: id, Expected: admission.Current.Expected(), RunID: "worker-" + id, OccurrenceID: "occurrence-" + id, SourceAfter: source, Outcome: outcome, Duration: duration, Tokens: tokens, Evidence: []autonomous.EvidenceReference{evidence("result-" + id)}, Signatures: signatures, CreatedAt: f.now.Add(time.Duration(f.sequence)*time.Minute + duration), Store: f.store}
}
func (f *attemptFixture) complete(t *testing.T, admission Result, id string, outcome autonomous.AttemptOutcome, source string, tokens *int64, signatures []autonomous.CanonicalSignature) Result {
	return f.completeDuration(t, admission, id, outcome, source, tokens, signatures, 2*time.Second)
}
func (f *attemptFixture) completeDuration(t *testing.T, admission Result, id string, outcome autonomous.AttemptOutcome, source string, tokens *int64, signatures []autonomous.CanonicalSignature, duration time.Duration) Result {
	t.Helper()
	result, err := Complete(context.Background(), f.completion(admission, id, outcome, source, tokens, signatures, duration))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func baseAttemptState(task string) autonomous.ExecutionState {
	return autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: task, Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
}
func configuredState(task string, limit, consumed, total int64) autonomous.ExecutionState {
	state := baseAttemptState(task)
	state.Attempts.TotalAttempts = total
	state.Attempts.RetryBudget = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: limit, Consumed: consumed}
	state.Attempts.ActionAttempts = []autonomous.ActionAttempt{{Action: autonomous.ActionImplement, Attempts: total}}
	state.Attempts.ActionBudgets = []autonomous.ActionBudget{{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4, Consumed: total}}}
	state.Attempts.ElapsedTimeBudget = autonomous.DurationBudget{Mode: autonomous.BudgetModeLimited, Limit: time.Minute}
	state.Attempts.TokenBudget = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 100}
	state.Attempts.RepeatedSignatureLimit = 2
	return state
}
func defaultLimits() Limits {
	return Limits{TaskAttempts: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4}, ActionAttempts: []autonomous.ActionBudget{{Action: autonomous.ActionImplement, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4}}}, Elapsed: autonomous.DurationBudget{Mode: autonomous.BudgetModeLimited, Limit: time.Minute}, Tokens: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 100}, RepeatedSignatureLimit: 2}
}
func strategy(value string) Strategy { return Strategy{Approach: value} }
func evidence(value string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: autonomous.EvidenceKindLedger, Reference: value, Detail: "Exact test evidence."}
}
func hash(value string) string    { return fmt.Sprintf("%064x", []byte(value))[:64] }
func int64ptr(value int64) *int64 { return &value }
