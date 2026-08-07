package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/scheduler"
	"revolvr/internal/storage/postgres"
)

type Config struct {
	OperationID         string
	CoordinatorIdentity string
	Limits              Limits
	QualityGateStatus   QualityGateStatus
	Clock               func() time.Time
	Executor            TaskExecutor
	FailureInjector     FailureInjector
	CancellationPoll    time.Duration
}

func Run(ctx context.Context, pool *pgxpool.Pool, cfg Config) (Result, error) {
	if _, err := validateOperationID(cfg.OperationID); err != nil {
		return Result{}, err
	}
	coordinator := strings.TrimSpace(cfg.CoordinatorIdentity)
	if coordinator == "" || len(coordinator) > 1024 {
		return Result{}, errors.New("sequential queue: coordinator identity is required and bounded")
	}
	if cfg.Executor == nil {
		return Result{}, errors.New("sequential queue: canonical task executor is required")
	}
	configuration, configSHA, rawConfig, err := NewPinnedConfiguration(cfg.Limits, cfg.QualityGateStatus)
	if err != nil {
		return Result{}, err
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	poll := cfg.CancellationPoll
	if poll == 0 {
		poll = 50 * time.Millisecond
	}
	if poll < time.Millisecond || poll > time.Second {
		return Result{}, errors.New("sequential queue: cancellation poll must be between 1ms and 1s")
	}
	store, err := NewStore(pool)
	if err != nil {
		return Result{}, err
	}
	unlock, err := store.acquire(ctx, cfg.OperationID)
	if err != nil {
		return Result{}, err
	}
	defer unlock()

	now := clock().UTC().Truncate(time.Microsecond)
	operation, replayed, err := store.createOrLoad(ctx, cfg.OperationID, configuration, configSHA, rawConfig, now)
	if err != nil {
		return Result{}, err
	}
	if operation.Status == "terminal" {
		result, err := resultFromStatus(ctx, store, cfg.OperationID, true)
		return result, err
	}

	for {
		operation, err = store.getOperation(ctx, cfg.OperationID)
		if err != nil {
			return Result{}, err
		}
		if operation.Status == "terminal" {
			return resultFromStatus(ctx, store, cfg.OperationID, true)
		}
		if !clock().Before(operation.DeadlineAt.Time) && !operation.ActiveOccurrenceID.Valid && !operation.SelectionIntentID.Valid {
			return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, StopBudgetExhausted, "maximum queue duration reached", clock(), replayed)
		}
		if ctx.Err() != nil && !operation.CancelRequestedAt.Valid {
			_, _, _ = store.requestCancel(context.WithoutCancel(ctx), cfg.OperationID, clock())
			operation.CancelRequestedAt = timestamp(clock())
		}

		occurrence, found, err := store.openOccurrence(context.WithoutCancel(ctx), cfg.OperationID)
		if err != nil {
			return Result{}, err
		}
		if found {
			stop, processErr := processOccurrence(ctx, store, pool, cfg, configuration, operation, occurrence, clock, poll)
			if processErr != nil {
				return Result{}, processErr
			}
			if stop != "" {
				return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, stop, stopDetail(stop), clock(), replayed)
			}
			continue
		}

		if operation.CancelRequestedAt.Valid {
			return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, StopCancelled, "queue cancellation requested", clock(), replayed)
		}
		intent, err := store.beginSelection(ctx, cfg.OperationID, id.New(), id.New(), clock())
		if err != nil {
			return Result{}, err
		}
		if err := inject(cfg.FailureInjector, FailureBeforeSelection); err != nil {
			return Result{}, err
		}
		candidate, selectErr := scheduler.Select(ctx, pool)
		if selectErr != nil {
			switch {
			case errors.Is(selectErr, scheduler.ErrNoReady), errors.Is(selectErr, scheduler.ErrWaiting):
				if err := store.clearSelection(context.WithoutCancel(ctx), cfg.OperationID, intent, "no_ready_task", clock()); err != nil {
					return Result{}, err
				}
				reason, detail, classifyErr := classifyEmpty(ctx, pool)
				if classifyErr != nil {
					return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, StopUnsafe, classifyErr.Error(), clock(), replayed)
				}
				return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, reason, detail, clock(), replayed)
			case errors.Is(selectErr, scheduler.ErrUnsafeGraph):
				if err := store.clearSelection(context.WithoutCancel(ctx), cfg.OperationID, intent, "unsafe_graph", clock()); err != nil {
					return Result{}, err
				}
				return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, StopUnsafe, selectErr.Error(), clock(), replayed)
			default:
				return Result{}, selectErr
			}
		}
		operation, err = store.getOperation(ctx, cfg.OperationID)
		if err != nil {
			return Result{}, err
		}
		if budgetFull(operation) {
			if err := store.clearSelection(context.WithoutCancel(ctx), cfg.OperationID, intent, "ready_work_exceeds_queue_budget", clock()); err != nil {
				return Result{}, err
			}
			return terminateAndResult(context.WithoutCancel(ctx), store, cfg.OperationID, StopBudgetExhausted, "a ready task remains but an exact queue budget is exhausted", clock(), replayed)
		}
		occurrence, err = store.persistSelectedOccurrence(ctx, cfg.OperationID, coordinator, intent, candidate, clock())
		if err != nil {
			return Result{}, err
		}
		if err := inject(cfg.FailureInjector, FailureAfterSelection); err != nil {
			return Result{}, err
		}
	}
}

func processOccurrence(ctx context.Context, store *Store, pool *pgxpool.Pool, cfg Config, configuration PinnedConfiguration, operation postgres.CoreQueueOperation, occurrence postgres.CoreQueueTaskOccurrence, clock func() time.Time, poll time.Duration) (StopReason, error) {
	candidate, err := candidateFromOccurrence(occurrence)
	if err != nil {
		return "", err
	}
	switch occurrence.State {
	case "selected":
		_, err := scheduler.Admit(ctx, pool, scheduler.AdmissionCommand{
			RunID:               uuidString(occurrence.SchedulerRunID),
			CoordinatorIdentity: occurrence.CoordinatorIdentity, Candidate: candidate,
		})
		if err != nil {
			return "", err
		}
		occurrence, err = store.markAdmitted(context.WithoutCancel(ctx), uuidString(occurrence.ID), clock())
		if err != nil {
			return "", err
		}
		fallthrough
	case "admitted":
		remainingCycles := operation.MaxTotalCycles - operation.CyclesConsumed
		remainingTokens := operation.MaxRemoteTokens - operation.RemoteTokensConsumed
		remainingCost := operation.MaxCostMicrousd - operation.CostMicrousdConsumed
		effects := &effectRecorder{store: store, operationID: uuidString(operation.ID), occurrenceID: uuidString(occurrence.ID), clock: clock, injector: cfg.FailureInjector}
		request := TaskRequest{
			QueueOperationID: uuidString(operation.ID), OccurrenceID: uuidString(occurrence.ID),
			OccurrenceSequence:  occurrence.OccurrenceSequence,
			SchedulerRunID:      uuidString(occurrence.SchedulerRunID),
			CoordinatorIdentity: occurrence.CoordinatorIdentity, Candidate: candidate,
			MaximumCycles:   min(configuration.Limits.MaximumCyclesPerTask, remainingCycles),
			RemainingCycles: remainingCycles, RemainingTokens: remainingTokens,
			RemainingCostMicrousd: remainingCost, Effects: effects,
		}
		result, runErr := runActiveChild(ctx, store, cfg.OperationID, operation.DeadlineAt.Time, poll, clock, cfg.Executor, request)
		if validateErr := validateResult(result, configuration.Limits); validateErr != nil {
			return "", errors.Join(runErr, validateErr)
		}
		if result.CyclesConsumed > remainingCycles || result.RemoteTokensConsumed > remainingTokens || result.CostMicrousdConsumed > remainingCost {
			return "", errors.New("sequential queue: task result exceeds remaining queue budget")
		}
		effectRows, err := store.listEffects(context.WithoutCancel(ctx), occurrence.ID)
		if err != nil {
			return "", err
		}
		if err := validateEffectHistory(effectRows, result.Outcome); err != nil {
			return "", err
		}
		occurrence, _, err = store.persistRunnerTerminal(context.WithoutCancel(ctx), uuidString(occurrence.ID), result, clock())
		if err != nil {
			return "", err
		}
		if !admittedTerminalError(runErr, result.Outcome) {
			return "", runErr
		}
		fallthrough
	case "runner_terminal":
		if _, err := scheduler.Release(context.WithoutCancel(ctx), pool, uuidString(occurrence.SchedulerRunID), occurrence.CoordinatorIdentity); err != nil {
			return "", err
		}
		if err := inject(cfg.FailureInjector, FailureBeforeCheckpoint); err != nil {
			return "", err
		}
		if _, _, err := store.checkpoint(context.WithoutCancel(ctx), uuidString(occurrence.ID), clock()); err != nil {
			return "", err
		}
		if err := inject(cfg.FailureInjector, FailureAfterCheckpoint); err != nil {
			return "", err
		}
		if !clock().Before(operation.DeadlineAt.Time) {
			return StopBudgetExhausted, nil
		}
		outcome := TaskOutcome(occurrence.Outcome.String)
		if stop := outcome.queueStop(); stop != "" {
			return stop, nil
		}
		return "", nil
	default:
		return "", ErrOperationConflict
	}
}

func runActiveChild(ctx context.Context, store *Store, operationID string, deadline time.Time, poll time.Duration, clock func() time.Time, executor TaskExecutor, request TaskRequest) (TaskResult, error) {
	child, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	type execution struct {
		result TaskResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		result, err := executor(child, request)
		done <- execution{result: result, err: err}
	}()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case completed := <-done:
			return completed.result, completed.err
		case <-ticker.C:
			requested, err := store.cancellationRequested(context.WithoutCancel(ctx), operationID)
			if err != nil {
				cancel()
				completed := <-done
				return completed.result, errors.Join(completed.err, err)
			}
			if requested {
				cancel()
			}
		case <-child.Done():
			cancel()
			var cancelErr error
			if ctx.Err() != nil {
				_, _, cancelErr = store.requestCancel(context.WithoutCancel(ctx), operationID, clock().UTC().Truncate(time.Microsecond))
			}
			completed := <-done
			return completed.result, errors.Join(completed.err, child.Err(), cancelErr)
		}
	}
}

func admittedTerminalError(err error, outcome TaskOutcome) bool {
	if err == nil || outcome == OutcomeUnsafe || outcome == OutcomeSystemFailure {
		return true
	}
	if outcome == OutcomeCancelled && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return true
	}
	return outcome == OutcomeTaskBudgetExhausted && errors.Is(err, context.DeadlineExceeded)
}

type effectRecorder struct {
	store                     *Store
	operationID, occurrenceID string
	clock                     func() time.Time
	injector                  FailureInjector
}

func (r *effectRecorder) PersistIntent(ctx context.Context, kind EffectKind, identity, material string) (EffectAdmission, error) {
	result, err := r.store.persistEffectIntent(ctx, r.operationID, r.occurrenceID, kind, identity, material, r.clock())
	if err != nil {
		return EffectAdmission{}, err
	}
	switch kind {
	case EffectWorker:
		err = inject(r.injector, FailureBeforeWorkerEffect)
	case EffectCompletion:
		err = inject(r.injector, FailureBeforeCompletion)
	}
	return result, err
}

func (r *effectRecorder) PersistCompletion(ctx context.Context, kind EffectKind, identity, material, evidence string) (EffectAdmission, error) {
	result, err := r.store.persistEffectCompletion(ctx, r.operationID, r.occurrenceID, kind, identity, material, evidence, r.clock())
	if err != nil {
		return EffectAdmission{}, err
	}
	switch kind {
	case EffectWorker:
		err = inject(r.injector, FailureAfterWorkerEffect)
	case EffectCompletion:
		err = inject(r.injector, FailureAfterCompletion)
	}
	return result, err
}

func validateEffectHistory(effects []postgres.CoreQueueTaskEffect, outcome TaskOutcome) error {
	if len(effects) == 0 || EffectKind(effects[0].EffectKind) != EffectSupervisor {
		return errors.New("sequential queue: every admitted task requires a persisted supervisor effect")
	}
	kinds := make([]EffectKind, 0, len(effects))
	for i, effect := range effects {
		if effect.EffectSequence != int64(i+1) || effect.Status != "completed" ||
			!EffectKind(effect.EffectKind).valid() || !effect.EvidenceSha256.Valid {
			return errors.New("sequential queue: task effect history is incomplete or noncontiguous")
		}
		kinds = append(kinds, EffectKind(effect.EffectKind))
	}
	if outcome != OutcomeCompleted {
		return nil
	}
	position := 0
	for _, required := range []EffectKind{EffectSupervisor, EffectWorker, EffectVerification, EffectAudit, EffectCompletion} {
		found := false
		for position < len(kinds) {
			if kinds[position] == required {
				found = true
				position++
				break
			}
			position++
		}
		if !found {
			return fmt.Errorf("sequential queue: completed task lacks ordered %s effect", required)
		}
	}
	if kinds[len(kinds)-1] != EffectCompletion {
		return errors.New("sequential queue: completion must be the final external effect")
	}
	return nil
}

func candidateFromOccurrence(occurrence postgres.CoreQueueTaskOccurrence) (scheduler.Candidate, error) {
	var candidate scheduler.Candidate
	if err := decodeStrict(occurrence.Selection, &candidate); err != nil {
		return scheduler.Candidate{}, ErrOperationConflict
	}
	_, selectionSHA, err := canonicalJSON(candidate)
	if err != nil || !occurrence.SelectionSha256.Valid || selectionSHA != occurrence.SelectionSha256.String {
		return scheduler.Candidate{}, ErrOperationConflict
	}
	if candidate.ProjectID != uuidString(occurrence.ProjectID) ||
		candidate.ProjectSourceID != uuidString(occurrence.ProjectSourceID) ||
		candidate.TaskID != uuidString(occurrence.TaskID) ||
		candidate.TaskVersionID != uuidString(occurrence.TaskVersionID) ||
		candidate.ExternalTaskID != occurrence.ExternalTaskID.String ||
		candidate.ExpectedAggregateVersion != occurrence.ExpectedTaskAggregateVersion.Int64 ||
		candidate.Priority != occurrence.TaskPriority.Int32 ||
		!candidate.CreatedAt.Equal(occurrence.TaskCreatedAt.Time) ||
		candidate.SourceCommit != occurrence.SourceCommit.String ||
		candidate.SourceTree != occurrence.SourceTree.String {
		return scheduler.Candidate{}, ErrOperationConflict
	}
	return candidate, nil
}

func classifyEmpty(ctx context.Context, pool *pgxpool.Pool) (StopReason, string, error) {
	state, err := scheduler.InspectQueueState(ctx, pool)
	if err != nil {
		return "", "", err
	}
	if len(state.WaitingInput) != 0 {
		return StopWaitingInput, fmt.Sprintf("%d task(s) are waiting on operator input", len(state.WaitingInput)), nil
	}
	if len(state.WaitingDependencies) != 0 {
		return StopWaitingDependencies, fmt.Sprintf("%d task(s) are waiting on dependencies", len(state.WaitingDependencies)), nil
	}
	if len(state.Blocked) != 0 {
		return StopAllRemainingBlocked, fmt.Sprintf("%d remaining task(s) are blocked", len(state.Blocked)), nil
	}
	return StopDrained, "no ready or waiting canonical tasks remain", nil
}

func budgetFull(operation postgres.CoreQueueOperation) bool {
	return operation.TasksStarted >= operation.MaxTasks ||
		operation.CyclesConsumed >= operation.MaxTotalCycles ||
		operation.RemoteTokensConsumed >= operation.MaxRemoteTokens ||
		operation.CostMicrousdConsumed >= operation.MaxCostMicrousd
}

func terminateAndResult(ctx context.Context, store *Store, operationID string, reason StopReason, detail string, now time.Time, replayed bool) (Result, error) {
	_, terminalReplay, err := store.terminate(ctx, operationID, reason, detail, now.UTC().Truncate(time.Microsecond))
	if err != nil {
		return Result{}, err
	}
	result, err := resultFromStatus(ctx, store, operationID, replayed || terminalReplay)
	return result, err
}

func resultFromStatus(ctx context.Context, store *Store, operationID string, replayed bool) (Result, error) {
	status, err := store.status(ctx, operationID)
	if err != nil {
		return Result{}, err
	}
	return Result{
		SchemaVersion: ResultSchemaVersion, OperationID: operationID,
		ConfigSHA256: status.ConfigSHA256, QualityGateStatus: status.Configuration.QualityGateStatus,
		StopReason: status.StopReason, StopDetail: status.StopDetail,
		TerminalMarkerSHA256: status.TerminalMarkerSHA256,
		TasksStarted:         status.TasksStarted, CyclesConsumed: status.CyclesConsumed,
		RemoteTokensConsumed:      status.RemoteTokensConsumed,
		CostMicrousdConsumed:      status.CostMicrousdConsumed,
		PeakSourceMutatingWorkers: status.PeakSourceMutatingWorkers,
		Outcomes:                  append([]Outcome(nil), status.Outcomes...), Replayed: replayed,
	}, nil
}

func StatusOperation(ctx context.Context, pool *pgxpool.Pool, operationID string) (Status, error) {
	if _, err := validateOperationID(operationID); err != nil {
		return Status{}, err
	}
	store, err := NewStore(pool)
	if err != nil {
		return Status{}, err
	}
	return store.status(ctx, operationID)
}

func Cancel(ctx context.Context, pool *pgxpool.Pool, operationID string, clock func() time.Time) (Status, bool, error) {
	if _, err := validateOperationID(operationID); err != nil {
		return Status{}, false, err
	}
	if clock == nil {
		clock = time.Now
	}
	store, err := NewStore(pool)
	if err != nil {
		return Status{}, false, err
	}
	_, replayed, err := store.requestCancel(ctx, operationID, clock().UTC().Truncate(time.Microsecond))
	if err != nil {
		return Status{}, false, err
	}
	status, err := store.status(ctx, operationID)
	return status, replayed, err
}

func stopDetail(reason StopReason) string {
	switch reason {
	case StopCancelled:
		return "active child stopped and queue cancellation reconciled"
	case StopUnsafe:
		return "task returned an unsafe outcome"
	case StopSystemFailure:
		return "task returned a system-failure outcome"
	case StopBudgetExhausted:
		return "an exact queue budget is exhausted"
	default:
		return string(reason)
	}
}

func inject(injector FailureInjector, point FailurePoint) error {
	if injector == nil {
		return nil
	}
	return injector(point)
}
