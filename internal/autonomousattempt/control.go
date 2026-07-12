package autonomousattempt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
)

type Disposition string

const (
	DispositionAdmitted  Disposition = "admitted"
	DispositionCompleted Disposition = "completed"
	DispositionBlocked   Disposition = "blocked"
	DispositionReplayed  Disposition = "replayed"
)

type Result struct {
	Disposition Disposition
	Reason      autonomous.BreakerReason
	Current     autonomousstate.Snapshot
	History     autonomousstate.AttemptHistorySnapshot
}

type AdmissionConfig struct {
	TaskID         string
	OperationID    string
	AttemptID      string
	Expected       autonomousstate.ExpectedState
	Action         autonomous.Action
	Decision       autonomous.SupervisorDecision
	Reference      autonomous.DecisionReference
	Strategy       Strategy
	SourceRevision string
	SourceSafety   autonomouspolicy.SourceSafety
	Limits         Limits
	CreatedAt      time.Time
	Store          *autonomousstate.Store
}

type CompletionConfig struct {
	TaskID       string
	OperationID  string
	AttemptID    string
	Expected     autonomousstate.ExpectedState
	RunID        string
	OccurrenceID string
	SourceAfter  string
	Outcome      autonomous.AttemptOutcome
	Duration     time.Duration
	Tokens       *int64
	Evidence     []autonomous.EvidenceReference
	Signatures   []autonomous.CanonicalSignature
	StopReason   autonomous.BreakerReason
	CreatedAt    time.Time
	Store        *autonomousstate.Store
}

func Admit(ctx context.Context, cfg AdmissionConfig) (Result, error) {
	applicationSHA, strategySHA, err := validateAdmission(cfg)
	if err != nil {
		return Result{}, err
	}
	if replay, ok, err := replay(ctx, cfg.Store, cfg.TaskID, cfg.OperationID, applicationSHA); err != nil || ok {
		return replay, err
	}
	snapshot, found, err := cfg.Store.Load(ctx, cfg.TaskID)
	if err != nil {
		return Result{}, err
	}
	if err := exactExpected(cfg.Expected, snapshot, found); err != nil {
		return Result{}, err
	}
	state, err := cloneState(snapshot.State)
	if err != nil {
		return Result{}, err
	}
	if state.Lifecycle != autonomous.LifecycleStatePending && state.Lifecycle != autonomous.LifecycleStateReady {
		return Result{}, fmt.Errorf("attempt admission: lifecycle %q does not admit a new action", state.Lifecycle)
	}
	if err := bindOrVerifyLimits(&state.Attempts, cfg.Limits); err != nil {
		return persistBreaker(ctx, cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, autonomous.BreakerAccountingSafety, nil, strategySHA, cfg.CreatedAt, err)
	}
	if cfg.SourceSafety != autonomouspolicy.SourceSafetySafe {
		return persistBreaker(ctx, cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, autonomous.BreakerUnsafeSource, nil, strategySHA, cfg.CreatedAt, fmt.Errorf("source safety is %q", cfg.SourceSafety))
	}
	if state.Attempts.RequiredStrategyChangeFrom != "" && state.Attempts.RequiredStrategyChangeFrom == strategySHA {
		return persistBreaker(ctx, cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, autonomous.BreakerIdenticalStrategy, attemptIDsForStrategy(state.Attempts.Events, strategySHA), strategySHA, cfg.CreatedAt, errors.New("a materially changed strategy is required before retry"))
	}
	if reason, err := exhaustedReason(state.Attempts, cfg.Action); reason != "" {
		if reason == autonomous.BreakerActionAttemptsExhausted {
			for _, stop := range state.Attempts.ActionStops {
				if stop.Budget.Action == cfg.Action {
					return Result{Disposition: DispositionBlocked, Reason: reason, Current: snapshot}, nil
				}
			}
			return persistActionStop(ctx, cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, cfg.CreatedAt, err)
		}
		return persistBreaker(ctx, cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, reason, nil, strategySHA, cfg.CreatedAt, err)
	}
	if ctx.Err() != nil {
		return persistBreaker(context.WithoutCancel(ctx), cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, autonomous.BreakerCancellation, nil, strategySHA, cfg.CreatedAt, ctx.Err())
	}

	next, err := cloneState(state)
	if err != nil {
		return Result{}, err
	}
	if next.Attempts.TotalAttempts == math.MaxInt64 || next.Attempts.RetryBudget.Consumed == math.MaxInt64 || next.Attempts.TransitionSequence == math.MaxInt64 || !actionIncrementSafe(next.Attempts, cfg.Action) {
		return persistBreaker(ctx, cfg.Store, snapshot, state, cfg.OperationID, applicationSHA, cfg.Action, autonomous.BreakerAccountingSafety, nil, strategySHA, cfg.CreatedAt, errors.New("attempt accounting overflow"))
	}
	next.Attempts.TotalAttempts++
	next.Attempts.RetryBudget.Consumed++
	incrementAction(&next.Attempts, cfg.Action)
	next.Attempts.TransitionSequence++
	event := autonomous.AttemptEvent{
		Sequence: next.Attempts.TransitionSequence, Kind: autonomous.AttemptEventAdmitted,
		AttemptID: cfg.AttemptID, Action: cfg.Action, Decision: cfg.Reference,
		StrategySHA256: strategySHA, SourceBefore: cfg.SourceRevision, CreatedAt: cfg.CreatedAt.UTC(),
		Evidence: []autonomous.EvidenceReference{cfg.Reference.Artifact},
	}
	next.Attempts.Events = append(next.Attempts.Events, event)
	return commit(ctx, cfg.Store, snapshot, next, cfg.OperationID, applicationSHA, autonomousstate.AttemptTransitionAdmitted, &event, nil, cfg.CreatedAt)
}

func Complete(ctx context.Context, cfg CompletionConfig) (Result, error) {
	// Once admission is durable, cancellation must not discard its accounting.
	ctx = context.WithoutCancel(ctx)
	applicationSHA, err := validateCompletion(cfg)
	if err != nil {
		return Result{}, err
	}
	if replayResult, ok, err := replay(ctx, cfg.Store, cfg.TaskID, cfg.OperationID, applicationSHA); err != nil || ok {
		return replayResult, err
	}
	snapshot, found, err := cfg.Store.Load(ctx, cfg.TaskID)
	if err != nil {
		return Result{}, err
	}
	if err := exactExpected(cfg.Expected, snapshot, found); err != nil {
		return Result{}, err
	}
	admission, ok := unmatchedAdmission(snapshot.State.Attempts.Events, cfg.AttemptID)
	if !ok {
		return Result{}, fmt.Errorf("attempt completion: no unmatched admission %q", cfg.AttemptID)
	}
	if cfg.CreatedAt.Before(admission.CreatedAt) {
		return Result{}, errors.New("attempt completion: created_at precedes admission")
	}
	if snapshot.State.Attempts.TransitionSequence == math.MaxInt64 {
		return Result{}, errors.New("attempt completion: transition sequence overflow")
	}
	evidence := append([]autonomous.EvidenceReference(nil), cfg.Evidence...)
	var malformedEvidence error
	if len(evidence) > 0 {
		probe := autonomous.SupervisorDecision{TaskID: cfg.TaskID, Action: autonomous.ActionBlock, Rationale: "validate attempt evidence", Inputs: evidence}
		if err := probe.Validate(); err != nil {
			malformedEvidence = err
			evidence = nil
		}
	}
	if len(evidence) == 0 {
		evidence = []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "attempt:" + cfg.AttemptID, Detail: "Harness-owned attempt completion evidence."}}
	}
	outcome := cfg.Outcome
	sourceKnown := validSHA(cfg.SourceAfter)
	unchanged := sourceKnown && sourceWriting(admission.Action) && cfg.SourceAfter == admission.SourceBefore
	if outcome == autonomous.AttemptOutcomeSucceeded && unchanged {
		outcome = autonomous.AttemptOutcomeNoProgress
	}
	malformedTokens := cfg.Tokens != nil && *cfg.Tokens < 0
	var tokens *int64
	if !malformedTokens {
		tokens = cloneInt64(cfg.Tokens)
	}
	signatures, signatureErr := canonicalSignatures(cfg.Signatures)
	malformedEvidence = errors.Join(malformedEvidence, signatureErr)
	event := autonomous.AttemptEvent{
		Sequence: snapshot.State.Attempts.TransitionSequence + 1, Kind: autonomous.AttemptEventCompleted,
		AttemptID: admission.AttemptID, Action: admission.Action, Decision: admission.Decision,
		StrategySHA256: admission.StrategySHA256, RunID: cfg.RunID, OccurrenceID: cfg.OccurrenceID,
		SourceBefore: admission.SourceBefore, SourceAfter: cfg.SourceAfter, Outcome: outcome,
		SourceAfterKnown: sourceKnown,
		Duration:         cfg.Duration, Tokens: tokens, Evidence: evidence,
		Signatures: signatures, CreatedAt: cfg.CreatedAt.UTC(),
	}
	next, err := cloneState(snapshot.State)
	if err != nil {
		return Result{}, err
	}
	next.Attempts.Events = append(next.Attempts.Events, event)
	next.Attempts.TransitionSequence = event.Sequence
	reason, trigger, accountingErr := applyCompletionAccounting(&next.Attempts, event)
	if accountingErr != nil {
		reason = autonomous.BreakerAccountingSafety
	}
	if reason == "" && malformedEvidence != nil {
		reason = autonomous.BreakerAccountingSafety
		accountingErr = malformedEvidence
	}
	if reason == "" && malformedTokens {
		reason = autonomous.BreakerAccountingSafety
		if next.Attempts.TokenBudget.Mode == autonomous.BudgetModeLimited {
			reason = autonomous.BreakerMissingTrustedMetrics
		}
		accountingErr = errors.New("trusted token metrics were negative or malformed")
	}
	if reason == "" && cfg.Tokens == nil && next.Attempts.TokenBudget.Mode == autonomous.BudgetModeLimited {
		reason = autonomous.BreakerMissingTrustedMetrics
		accountingErr = errors.New("limited token budget requires unambiguous trusted token metrics")
	}
	if reason == "" {
		reason, trigger = progressBreaker(next.Attempts, event)
	}
	if reason == "" && outcome == autonomous.AttemptOutcomeCancelled {
		reason = autonomous.BreakerCancellation
	}
	if reason == "" && outcome == autonomous.AttemptOutcomeSafetyStopped {
		reason = cfg.StopReason
		if reason == "" {
			reason = autonomous.BreakerStaleEvidence
		}
	}
	var breaker *autonomous.CircuitBreakerDetail
	if reason != "" {
		breaker = buildBreaker(next.Attempts, admission.Action, reason, triggerAttemptIDs(next.Attempts.Events, trigger), trigger, admission.StrategySHA256, cfg.OperationID, accountingErr)
		applyBreaker(&next, breaker)
	} else if outcome == autonomous.AttemptOutcomeNoProgress {
		next.Attempts.RequiredStrategyChangeFrom = admission.StrategySHA256
	} else if outcome == autonomous.AttemptOutcomeSucceeded {
		next.Attempts.RequiredStrategyChangeFrom = ""
	}
	return commit(ctx, cfg.Store, snapshot, next, cfg.OperationID, applicationSHA, autonomousstate.AttemptTransitionCompleted, &event, breaker, cfg.CreatedAt)
}

func validateAdmission(cfg AdmissionConfig) (string, string, error) {
	if cfg.Store == nil || strings.TrimSpace(cfg.TaskID) == "" || strings.TrimSpace(cfg.OperationID) == "" || strings.TrimSpace(cfg.AttemptID) == "" || cfg.CreatedAt.IsZero() || !validSHA(cfg.SourceRevision) {
		return "", "", errors.New("attempt admission: store, identities, time, and source revision are required")
	}
	if err := cfg.Expected.Validate(); err != nil || !cfg.Expected.Exists {
		return "", "", errors.New("attempt admission: exact existing state expectation is required")
	}
	if err := cfg.Decision.Validate(); err != nil {
		return "", "", err
	}
	if err := cfg.Reference.Validate(); err != nil {
		return "", "", err
	}
	if cfg.Decision.TaskID != cfg.TaskID || cfg.Reference.TaskID != cfg.TaskID || cfg.Decision.Action != cfg.Action || cfg.Reference.Action != cfg.Action || cfg.Decision.WorkerProfile != cfg.Reference.WorkerProfile {
		return "", "", errors.New("attempt admission: decision/reference/task/action identity mismatch")
	}
	if err := cfg.Limits.Validate(); err != nil {
		return "", "", err
	}
	strategySHA, err := cfg.Strategy.Signature()
	if err != nil {
		return "", "", err
	}
	if sourceWriting(cfg.Action) || cfg.Action == autonomous.ActionPlan || cfg.Action == autonomous.ActionAudit {
		if cfg.Decision.Strategy == nil {
			return "", "", errors.New("attempt admission: worker decision requires validated structured strategy material")
		}
		decisionStrategy := Strategy{Approach: cfg.Decision.Strategy.Approach, Techniques: cfg.Decision.Strategy.Techniques, Targets: cfg.Decision.Strategy.Targets}
		decisionStrategySHA, err := decisionStrategy.Signature()
		if err != nil || decisionStrategySHA != strategySHA {
			return "", "", errors.New("attempt admission: caller strategy does not match the validated supervisor strategy")
		}
	}
	applicationSHA, err := hashCanonical(struct {
		Kind                           string `json:"kind"`
		TaskID, OperationID, AttemptID string
		Expected                       autonomousstate.ExpectedState
		Action                         autonomous.Action
		Decision                       autonomous.SupervisorDecision
		Reference                      autonomous.DecisionReference
		StrategySHA, Source            string
		SourceSafety                   autonomouspolicy.SourceSafety
		Limits                         Limits
	}{"admit", cfg.TaskID, cfg.OperationID, cfg.AttemptID, cfg.Expected, cfg.Action, cfg.Decision, cfg.Reference, strategySHA, cfg.SourceRevision, cfg.SourceSafety, cfg.Limits})
	return applicationSHA, strategySHA, err
}

func validateCompletion(cfg CompletionConfig) (string, error) {
	if cfg.Store == nil || strings.TrimSpace(cfg.TaskID) == "" || strings.TrimSpace(cfg.OperationID) == "" || strings.TrimSpace(cfg.AttemptID) == "" || strings.TrimSpace(cfg.RunID) == "" || cfg.CreatedAt.IsZero() || cfg.Duration < 0 {
		return "", errors.New("attempt completion: store, identities, time, nonnegative duration, and source revision are required")
	}
	if !validSHA(cfg.SourceAfter) && (cfg.SourceAfter != "" || (cfg.Outcome != autonomous.AttemptOutcomeCancelled && cfg.Outcome != autonomous.AttemptOutcomeSafetyStopped)) {
		return "", errors.New("attempt completion: source_after is invalid or unavailable for a non-safety outcome")
	}
	if err := cfg.Expected.Validate(); err != nil || !cfg.Expected.Exists {
		return "", errors.New("attempt completion: exact existing state expectation is required")
	}
	if cfg.StopReason != "" && !knownBreakerReason(cfg.StopReason) {
		return "", fmt.Errorf("attempt completion: unknown stop reason %q", cfg.StopReason)
	}
	if !validOutcome(cfg.Outcome) {
		return "", fmt.Errorf("attempt completion: unknown outcome %q", cfg.Outcome)
	}
	return hashCanonical(struct {
		Kind, TaskID, OperationID, AttemptID, RunID, OccurrenceID, SourceAfter string
		Expected                                                               autonomousstate.ExpectedState
		Outcome                                                                autonomous.AttemptOutcome
		Duration                                                               time.Duration
		Tokens                                                                 *int64
		Evidence                                                               []autonomous.EvidenceReference
		Signatures                                                             []autonomous.CanonicalSignature
		StopReason                                                             autonomous.BreakerReason
	}{"complete", cfg.TaskID, cfg.OperationID, cfg.AttemptID, cfg.RunID, cfg.OccurrenceID, cfg.SourceAfter, cfg.Expected, cfg.Outcome, cfg.Duration, cfg.Tokens, cfg.Evidence, cfg.Signatures, cfg.StopReason})
}

func bindOrVerifyLimits(attempts *autonomous.AttemptState, limits Limits) error {
	canonical := append([]autonomous.ActionBudget(nil), limits.ActionAttempts...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Action < canonical[j].Action })
	if attempts.TotalAttempts == 0 && len(attempts.Events) == 0 && attempts.RetryBudget.Mode == autonomous.BudgetModeUnset && attempts.ElapsedTimeBudget.Mode == autonomous.BudgetModeUnset && attempts.TokenBudget.Mode == autonomous.BudgetModeUnset && len(attempts.ActionBudgets) == 0 && attempts.RepeatedSignatureLimit == 0 {
		attempts.RetryBudget = limits.TaskAttempts
		attempts.ElapsedTimeBudget = limits.Elapsed
		attempts.TokenBudget = limits.Tokens
		attempts.ActionBudgets = canonical
		attempts.RepeatedSignatureLimit = limits.RepeatedSignatureLimit
		return nil
	}
	if attempts.RetryBudget.Consumed != attempts.TotalAttempts {
		return errors.New("persisted task attempt accounting is inconsistent")
	}
	counts := make(map[autonomous.Action]int64, len(attempts.ActionAttempts))
	for _, counter := range attempts.ActionAttempts {
		counts[counter.Action] = counter.Attempts
	}
	for _, budget := range attempts.ActionBudgets {
		if budget.Budget.Consumed != counts[budget.Action] {
			return fmt.Errorf("persisted action %q attempt accounting is inconsistent", budget.Action)
		}
	}
	wantTask, wantElapsed, wantTokens := limits.TaskAttempts, limits.Elapsed, limits.Tokens
	wantTask.Consumed, wantElapsed.Consumed, wantTokens.Consumed = attempts.RetryBudget.Consumed, attempts.ElapsedTimeBudget.Consumed, attempts.TokenBudget.Consumed
	if attempts.RetryBudget != wantTask || attempts.ElapsedTimeBudget != wantElapsed || attempts.TokenBudget != wantTokens || attempts.RepeatedSignatureLimit != limits.RepeatedSignatureLimit {
		return errors.New("caller attempted to replace authoritative task budget limits")
	}
	if len(canonical) != len(attempts.ActionBudgets) {
		return errors.New("caller attempted to replace authoritative action budget limits")
	}
	for i := range canonical {
		want := canonical[i].Budget
		want.Consumed = attempts.ActionBudgets[i].Budget.Consumed
		if canonical[i].Action != attempts.ActionBudgets[i].Action || want != attempts.ActionBudgets[i].Budget {
			return errors.New("caller attempted to replace authoritative action budget limits")
		}
	}
	return nil
}

func exhaustedReason(a autonomous.AttemptState, action autonomous.Action) (autonomous.BreakerReason, error) {
	if exhaustedCount(a.RetryBudget) {
		return autonomous.BreakerTaskAttemptsExhausted, errors.New("task attempt budget exhausted")
	}
	actionBudget := actionBudget(a, action)
	if exhaustedCount(actionBudget) {
		return autonomous.BreakerActionAttemptsExhausted, errors.New("action attempt budget exhausted")
	}
	if exhaustedDuration(a.ElapsedTimeBudget) {
		return autonomous.BreakerElapsedExhausted, errors.New("elapsed budget exhausted")
	}
	if exhaustedCount(a.TokenBudget) {
		return autonomous.BreakerTokenExhausted, errors.New("token budget exhausted")
	}
	return "", nil
}

func incrementAction(a *autonomous.AttemptState, action autonomous.Action) {
	for i := range a.ActionAttempts {
		if a.ActionAttempts[i].Action == action {
			a.ActionAttempts[i].Attempts++
			goto budget
		}
	}
	a.ActionAttempts = append(a.ActionAttempts, autonomous.ActionAttempt{Action: action, Attempts: 1})
	sort.Slice(a.ActionAttempts, func(i, j int) bool { return a.ActionAttempts[i].Action < a.ActionAttempts[j].Action })
budget:
	for i := range a.ActionBudgets {
		if a.ActionBudgets[i].Action == action {
			a.ActionBudgets[i].Budget.Consumed++
			return
		}
	}
}

func actionIncrementSafe(a autonomous.AttemptState, action autonomous.Action) bool {
	for _, counter := range a.ActionAttempts {
		if counter.Action == action && counter.Attempts == math.MaxInt64 {
			return false
		}
	}
	for _, budget := range a.ActionBudgets {
		if budget.Action == action && budget.Budget.Consumed == math.MaxInt64 {
			return false
		}
	}
	return true
}

func applyCompletionAccounting(a *autonomous.AttemptState, event autonomous.AttemptEvent) (autonomous.BreakerReason, *autonomous.CanonicalSignature, error) {
	if event.Duration > time.Duration(math.MaxInt64-int64(a.ElapsedTimeBudget.Consumed)) {
		return autonomous.BreakerAccountingSafety, nil, errors.New("elapsed accounting overflow")
	}
	a.ElapsedTimeBudget.Consumed += event.Duration
	if event.Tokens != nil {
		if *event.Tokens > math.MaxInt64-a.TokenBudget.Consumed {
			return autonomous.BreakerAccountingSafety, nil, errors.New("token accounting overflow")
		}
		a.TokenBudget.Consumed += *event.Tokens
	}
	if event.Outcome == autonomous.AttemptOutcomeSucceeded {
		a.ConsecutiveFailures = 0
	} else {
		if a.ConsecutiveFailures == math.MaxInt64 {
			return autonomous.BreakerAccountingSafety, nil, errors.New("failure streak overflow")
		}
		a.ConsecutiveFailures++
		failure := event.Evidence[0]
		a.LastFailure = &failure
	}
	return "", nil, nil
}

func progressBreaker(a autonomous.AttemptState, event autonomous.AttemptEvent) (autonomous.BreakerReason, *autonomous.CanonicalSignature) {
	for i := range event.Signatures {
		signature := event.Signatures[i]
		count := 0
		for _, prior := range a.Events {
			if prior.Kind != autonomous.AttemptEventCompleted || (signature.Kind != autonomous.SignatureKindDecision && prior.Outcome == autonomous.AttemptOutcomeSucceeded) {
				continue
			}
			for _, candidate := range prior.Signatures {
				if candidate.Kind == signature.Kind && candidate.SHA256 == signature.SHA256 {
					count++
					break
				}
			}
		}
		if int64(count) >= a.RepeatedSignatureLimit {
			return autonomous.BreakerRepeatedSignature, &signature
		}
	}
	if event.Outcome == autonomous.AttemptOutcomeNoProgress {
		count := 0
		for _, prior := range a.Events {
			if prior.Kind == autonomous.AttemptEventCompleted && prior.Outcome == autonomous.AttemptOutcomeNoProgress && prior.SourceAfterKnown && prior.SourceBefore == prior.SourceAfter {
				count++
			}
		}
		if int64(count) >= a.RepeatedSignatureLimit {
			return autonomous.BreakerUnchangedSource, nil
		}
	}
	return "", nil
}

func persistBreaker(ctx context.Context, store *autonomousstate.Store, snapshot autonomousstate.Snapshot, next autonomous.ExecutionState, operationID, applicationSHA string, action autonomous.Action, reason autonomous.BreakerReason, attempts []string, strategy string, created time.Time, cause error) (Result, error) {
	breaker := buildBreaker(next.Attempts, action, reason, attempts, nil, strategy, operationID, cause)
	next.Attempts.TransitionSequence++
	applyBreaker(&next, breaker)
	return commit(ctx, store, snapshot, next, operationID, applicationSHA, autonomousstate.AttemptTransitionBreaker, nil, breaker, created)
}

func persistActionStop(ctx context.Context, store *autonomousstate.Store, snapshot autonomousstate.Snapshot, next autonomous.ExecutionState, operationID, applicationSHA string, action autonomous.Action, created time.Time, cause error) (Result, error) {
	breaker := buildBreaker(next.Attempts, action, autonomous.BreakerActionAttemptsExhausted, nil, nil, "", operationID, cause)
	next.Attempts.TransitionSequence++
	next.Attempts.ActionStops = append(next.Attempts.ActionStops, *breaker)
	return commit(ctx, store, snapshot, next, operationID, applicationSHA, autonomousstate.AttemptTransitionBreaker, nil, breaker, created)
}

func buildBreaker(a autonomous.AttemptState, action autonomous.Action, reason autonomous.BreakerReason, ids []string, signature *autonomous.CanonicalSignature, strategy, operationID string, cause error) *autonomous.CircuitBreakerDetail {
	detail := "Authoritative autonomous attempt circuit breaker " + string(reason) + "."
	if cause != nil {
		detail += " " + cause.Error()
	}
	evidence := []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "attempt-control:" + operationID, Detail: detail}}
	requiredStrategy := ""
	if reason == autonomous.BreakerIdenticalStrategy || reason == autonomous.BreakerUnchangedSource {
		requiredStrategy = strategy
	}
	return &autonomous.CircuitBreakerDetail{Reason: reason, TriggerAttemptIDs: ids, TriggerSignature: signature, RequiredStrategy: requiredStrategy, Budget: autonomous.BudgetSnapshot{TaskAttempts: a.RetryBudget, Action: action, ActionBudget: actionBudget(a, action), Elapsed: a.ElapsedTimeBudget, Tokens: a.TokenBudget}, Evidence: evidence}
}

func applyBreaker(state *autonomous.ExecutionState, breaker *autonomous.CircuitBreakerDetail) {
	state.Lifecycle = autonomous.LifecycleStateBlocked
	state.CircuitBreaker = breaker
	state.NeedsInput = nil
	state.Terminal = &autonomous.TerminalDetail{Reason: string(breaker.Reason), Evidence: append([]autonomous.EvidenceReference(nil), breaker.Evidence...)}
}

func commit(ctx context.Context, store *autonomousstate.Store, previous autonomousstate.Snapshot, next autonomous.ExecutionState, operationID, applicationSHA string, kind autonomousstate.AttemptTransitionKind, event *autonomous.AttemptEvent, breaker *autonomous.CircuitBreakerDetail, created time.Time) (Result, error) {
	previousIdentity, err := autonomousstate.StateIdentityFor(previous.SourcePath, true, previous.State)
	if err != nil {
		return Result{}, err
	}
	nextIdentity, err := autonomousstate.StateIdentityFor(previous.SourcePath, true, next)
	if err != nil {
		return Result{}, err
	}
	sequence := next.Attempts.TransitionSequence
	record := autonomousstate.AttemptHistoryRecord{SchemaVersion: autonomousstate.AttemptHistorySchemaVersion, TaskID: previous.State.TaskID, OperationID: operationID, ApplicationSHA256: applicationSHA, Sequence: sequence, Kind: kind, CreatedAt: created.UTC(), Event: event, Breaker: breaker, PreviousState: previousIdentity, ResultingState: nextIdentity}
	result, err := store.CommitAttempt(ctx, autonomousstate.AttemptCommitRequest{TaskID: previous.State.TaskID, Expected: previous.Expected(), PreviousState: previous.State, NextState: next, History: record})
	if err != nil {
		return Result{}, err
	}
	disposition := DispositionCompleted
	if kind == autonomousstate.AttemptTransitionAdmitted {
		disposition = DispositionAdmitted
	}
	if breaker != nil {
		disposition = DispositionBlocked
	}
	if result.Disposition == autonomousstate.CommitReplayed {
		disposition = DispositionReplayed
	}
	reason := autonomous.BreakerReason("")
	if breaker != nil {
		reason = breaker.Reason
	}
	return Result{Disposition: disposition, Reason: reason, Current: result.Current, History: result.History}, nil
}

func replay(ctx context.Context, store *autonomousstate.Store, taskID, operationID, applicationSHA string) (Result, bool, error) {
	history, found, err := store.LoadAttemptOperation(ctx, taskID, operationID)
	if err != nil || !found {
		return Result{}, false, err
	}
	if history.Record.ApplicationSHA256 != applicationSHA {
		return Result{}, true, fmt.Errorf("%w: attempt operation evidence differs", autonomousstate.ErrOperationConflict)
	}
	current, stateFound, err := store.Load(ctx, taskID)
	if err != nil {
		return Result{}, true, err
	}
	if !stateFound || (current.SHA256 != history.Record.ResultingState.SHA256 || current.ByteSize != history.Record.ResultingState.ByteSize) && !historyApplied(current.State, history.Record) {
		if stateFound && current.SHA256 == history.Record.PreviousState.SHA256 && current.ByteSize == history.Record.PreviousState.ByteSize {
			return Result{}, false, nil
		}
		return Result{}, true, fmt.Errorf("%w: attempt operation is orphaned or stale", autonomousstate.ErrStaleWrite)
	}
	reason := autonomous.BreakerReason("")
	if history.Record.Breaker != nil {
		reason = history.Record.Breaker.Reason
	}
	return Result{Disposition: DispositionReplayed, Reason: reason, Current: current, History: history}, true, nil
}

func exactExpected(expected autonomousstate.ExpectedState, current autonomousstate.Snapshot, found bool) error {
	if !found || !expected.Exists || current.SHA256 != expected.SHA256 || current.ByteSize != expected.ByteSize {
		return fmt.Errorf("%w: expected state does not match current state", autonomousstate.ErrStaleWrite)
	}
	return nil
}

func historyApplied(state autonomous.ExecutionState, record autonomousstate.AttemptHistoryRecord) bool {
	if record.Event != nil {
		for _, event := range state.Attempts.Events {
			if reflect.DeepEqual(event, *record.Event) {
				return true
			}
		}
	}
	if record.Breaker == nil {
		return false
	}
	if state.CircuitBreaker != nil && reflect.DeepEqual(*record.Breaker, *state.CircuitBreaker) {
		return true
	}
	for _, stop := range state.Attempts.ActionStops {
		if reflect.DeepEqual(*record.Breaker, stop) {
			return true
		}
	}
	return false
}

func cloneState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	var cloned autonomous.ExecutionState
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return autonomous.ExecutionState{}, err
	}
	return cloned, nil
}

func exhaustedCount(b autonomous.CountBudget) bool {
	return b.Mode == autonomous.BudgetModeLimited && b.Consumed >= b.Limit
}
func exhaustedDuration(b autonomous.DurationBudget) bool {
	return b.Mode == autonomous.BudgetModeLimited && b.Consumed >= b.Limit
}
func actionBudget(a autonomous.AttemptState, action autonomous.Action) autonomous.CountBudget {
	for _, b := range a.ActionBudgets {
		if b.Action == action {
			return b.Budget
		}
	}
	return autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}
}
func unmatchedAdmission(events []autonomous.AttemptEvent, id string) (autonomous.AttemptEvent, bool) {
	var found autonomous.AttemptEvent
	ok := false
	for _, e := range events {
		if e.AttemptID != id {
			continue
		}
		if e.Kind == autonomous.AttemptEventAdmitted {
			found, ok = e, true
		} else if e.Kind == autonomous.AttemptEventCompleted {
			ok = false
		}
	}
	return found, ok
}
func attemptIDsForStrategy(events []autonomous.AttemptEvent, strategy string) []string {
	var result []string
	for _, e := range events {
		if e.Kind == autonomous.AttemptEventCompleted && e.StrategySHA256 == strategy {
			result = append(result, e.AttemptID)
		}
	}
	return uniqueStrings(result)
}
func triggerAttemptIDs(events []autonomous.AttemptEvent, signature *autonomous.CanonicalSignature) []string {
	if signature == nil {
		return nil
	}
	var ids []string
	for _, e := range events {
		for _, s := range e.Signatures {
			if s.Kind == signature.Kind && s.SHA256 == signature.SHA256 {
				ids = append(ids, e.AttemptID)
			}
		}
	}
	return uniqueStrings(ids)
}
func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range values {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func sourceWriting(action autonomous.Action) bool {
	return action == autonomous.ActionImplement || action == autonomous.ActionCorrect || action == autonomous.ActionDocument || action == autonomous.ActionSimplify
}
func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
func validOutcome(value autonomous.AttemptOutcome) bool {
	return value == autonomous.AttemptOutcomeSucceeded || value == autonomous.AttemptOutcomeFailed || value == autonomous.AttemptOutcomeNoProgress || value == autonomous.AttemptOutcomeCancelled || value == autonomous.AttemptOutcomeSafetyStopped
}

func knownBreakerReason(value autonomous.BreakerReason) bool {
	switch value {
	case autonomous.BreakerTaskAttemptsExhausted, autonomous.BreakerActionAttemptsExhausted, autonomous.BreakerElapsedExhausted, autonomous.BreakerTokenExhausted, autonomous.BreakerRepeatedSignature, autonomous.BreakerUnchangedSource, autonomous.BreakerIdenticalStrategy, autonomous.BreakerMissingTrustedMetrics, autonomous.BreakerStaleEvidence, autonomous.BreakerCancellation, autonomous.BreakerUnsafeSource, autonomous.BreakerAccountingSafety:
		return true
	default:
		return false
	}
}

func canonicalSignatures(values []autonomous.CanonicalSignature) ([]autonomous.CanonicalSignature, error) {
	result := append([]autonomous.CanonicalSignature(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].SHA256 < result[j].SHA256
	})
	for i, signature := range result {
		if err := signature.Validate(); err != nil {
			return nil, fmt.Errorf("signature %d: %w", i, err)
		}
		if i > 0 && result[i-1].Kind == signature.Kind {
			return nil, fmt.Errorf("signature %d duplicates kind %q", i, signature.Kind)
		}
	}
	return result, nil
}
