package autonomous

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestAttemptTransitionRejectsCounterBudgetAndEventRewrites(t *testing.T) {
	previous := attemptTransitionState()
	tests := []struct {
		name   string
		mutate func(*ExecutionState)
	}{
		{"total decrease", func(next *ExecutionState) { next.Attempts.TotalAttempts-- }},
		{"task consumption decrease", func(next *ExecutionState) { next.Attempts.RetryBudget.Consumed-- }},
		{"elapsed consumption decrease", func(next *ExecutionState) { next.Attempts.ElapsedTimeBudget.Consumed-- }},
		{"token consumption decrease", func(next *ExecutionState) { next.Attempts.TokenBudget.Consumed-- }},
		{"limit widened", func(next *ExecutionState) { next.Attempts.RetryBudget.Limit++ }},
		{"limited switched unlimited", func(next *ExecutionState) {
			next.Attempts.RetryBudget.Mode, next.Attempts.RetryBudget.Limit = BudgetModeUnlimited, 0
		}},
		{"action consumption decrease", func(next *ExecutionState) { next.Attempts.ActionBudgets[0].Budget.Consumed-- }},
		{"event rewritten", func(next *ExecutionState) { next.Attempts.Events[0].AttemptID = "attempt-rewritten" }},
		{"last failure erased", func(next *ExecutionState) { next.Attempts.LastFailure = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := cloneAttemptTransitionState(previous)
			tt.mutate(&next)
			if err := ValidateExecutionStateTransition(previous, next); err == nil {
				t.Fatal("materially non-monotonic transition unexpectedly passed")
			}
		})
	}
}

func TestDossierRendersDurableAttemptAndBreakerAuthority(t *testing.T) {
	state := attemptTransitionState()
	evidence := EvidenceReference{Kind: EvidenceKindLedger, Reference: "attempt-control:stop", Detail: "Exact breaker evidence."}
	signature := CanonicalSignature{Kind: SignatureKindOperationFailure, SHA256: strings.Repeat("4", 64), Evidence: []EvidenceReference{evidence}}
	state.Lifecycle = LifecycleStateBlocked
	state.Terminal = &TerminalDetail{Reason: string(BreakerRepeatedSignature), Evidence: []EvidenceReference{evidence}}
	state.CircuitBreaker = &CircuitBreakerDetail{Reason: BreakerRepeatedSignature, TriggerAttemptIDs: []string{"attempt-one"}, TriggerSignature: &signature, Budget: BudgetSnapshot{TaskAttempts: state.Attempts.RetryBudget, Action: ActionImplement, ActionBudget: state.Attempts.ActionBudgets[0].Budget, Elapsed: state.Attempts.ElapsedTimeBudget, Tokens: state.Attempts.TokenBudget}, Evidence: []EvidenceReference{evidence}}
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	writeExecutionState(&out, state)
	for _, want := range []string{"Durable attempt events", "attempt=attempt-one", "Trigger signature", "### Circuit Breaker", "repeated_signature", "Task-attempt budget"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dossier projection missing %q:\n%s", want, out.String())
		}
	}
}

func TestSupervisorStrategyContractIsValidatedWithoutBreakingLegacyWorkerDecisions(t *testing.T) {
	decision := SupervisorDecision{TaskID: "task-1", Action: ActionImplement, WorkerProfile: WorkerProfileImplementer, Rationale: "work", SuccessCriteria: []string{"passes"}, Inputs: []EvidenceReference{{Kind: EvidenceKindTask, Reference: "task", Detail: "Exact task."}}}
	if err := decision.Validate(); err != nil {
		t.Fatalf("legacy decision: %v", err)
	}
	decision.Strategy = &Strategy{Approach: "change parser state", Techniques: []string{"table driven"}, Targets: []EvidenceReference{{Kind: EvidenceKindFile, Reference: "parser.go", Detail: "Target parser."}}}
	if err := decision.Validate(); err != nil {
		t.Fatalf("structured strategy: %v", err)
	}
	decision.Strategy.Techniques = []string{"Table   driven", "table driven"}
	if err := decision.Validate(); err == nil {
		t.Fatal("materially duplicate techniques passed")
	}
	terminal := SupervisorDecision{TaskID: "task-1", Action: ActionBlock, Rationale: "blocked", Inputs: []EvidenceReference{{Kind: EvidenceKindTask, Reference: "task", Detail: "Exact task."}}, Strategy: &Strategy{Approach: "pretend retry"}}
	if err := terminal.Validate(); err == nil {
		t.Fatal("terminal decision accepted worker strategy")
	}
}

func attemptTransitionState() ExecutionState {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	evidence := EvidenceReference{Kind: EvidenceKindLedger, Reference: "attempt-one", Detail: "Exact evidence."}
	reference := DecisionReference{DecisionID: "decision-one", RunID: "supervisor-one", TaskID: "task-1", Action: ActionImplement, WorkerProfile: WorkerProfileImplementer, Artifact: evidence, CreatedAt: now}
	tokens := int64(5)
	return ExecutionState{SchemaVersion: ExecutionStateSchemaVersion, TaskID: "task-1", Lifecycle: LifecycleStateReady, Attempts: AttemptState{
		TotalAttempts: 1, ActionAttempts: []ActionAttempt{{Action: ActionImplement, Attempts: 1}}, ActionBudgets: []ActionBudget{{Action: ActionImplement, Budget: CountBudget{Mode: BudgetModeLimited, Limit: 3, Consumed: 1}}}, ConsecutiveFailures: 1,
		RetryBudget: CountBudget{Mode: BudgetModeLimited, Limit: 3, Consumed: 1}, ElapsedTimeBudget: DurationBudget{Mode: BudgetModeLimited, Limit: time.Minute, Consumed: time.Second}, TokenBudget: CountBudget{Mode: BudgetModeLimited, Limit: 100, Consumed: 5}, RepeatedSignatureLimit: 2,
		TransitionSequence: 2,
		Events: []AttemptEvent{
			{Sequence: 1, Kind: AttemptEventAdmitted, AttemptID: "attempt-one", Action: ActionImplement, Decision: reference, StrategySHA256: strings.Repeat("1", 64), SourceBefore: strings.Repeat("2", 64), Evidence: []EvidenceReference{evidence}, CreatedAt: now},
			{Sequence: 2, Kind: AttemptEventCompleted, AttemptID: "attempt-one", Action: ActionImplement, Decision: reference, StrategySHA256: strings.Repeat("1", 64), RunID: "worker-one", SourceBefore: strings.Repeat("2", 64), SourceAfter: strings.Repeat("3", 64), SourceAfterKnown: true, Outcome: AttemptOutcomeFailed, Duration: time.Second, Tokens: &tokens, Evidence: []EvidenceReference{evidence}, CreatedAt: now.Add(time.Second)},
		}, LastFailure: &evidence,
	}}
}

func cloneAttemptTransitionState(state ExecutionState) ExecutionState {
	copy := state
	copy.Attempts.ActionAttempts = append([]ActionAttempt(nil), state.Attempts.ActionAttempts...)
	copy.Attempts.ActionBudgets = append([]ActionBudget(nil), state.Attempts.ActionBudgets...)
	copy.Attempts.Events = append([]AttemptEvent(nil), state.Attempts.Events...)
	return copy
}
