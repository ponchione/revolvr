package autonomousqueue

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
)

type Snapshot struct {
	Fingerprint string
	Nodes       []autonomousscheduler.Node
	// Classify re-evaluates this exact snapshot against occupied task IDs.
	// A nil classifier is valid sequential authority, but cannot prove overlap.
	Classify func([]string) ([]autonomousscheduler.Node, error)
}

type Loader func(context.Context) (Snapshot, error)

type RunTaskInput struct {
	QueueOperationID  string
	TaskID            string
	OperationID       string
	SelectionSequence int64
	Batch             int64
	Slot              int
}

type TaskRunner func(context.Context, RunTaskInput) (autonomoustaskrun.Result, error)
type Progress func(Operation)

type Ledger interface {
	CreateRun(context.Context, ledger.RunSpec) (ledger.Run, error)
	GetRunWithEvents(context.Context, string) (ledger.RunWithEvents, bool, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
	CompleteRun(context.Context, string, ledger.RunCompletion) (ledger.Run, bool, error)
}

type Config struct {
	RepositoryRoot        string
	OperationID           string
	Mode                  Mode
	ConfigSchema          string
	ConfigSHA256          string
	SafetyIdentity        string
	MaxTasks              int64
	MaximumWorkers        int
	Sweep                 int64
	DaemonWakeCount       int64
	DaemonWakeFingerprint string
	Clock                 func() time.Time
	Loader                Loader
	Runner                TaskRunner
	Progress              Progress
	Redact                func(string) string
	Ledger                Ledger
	FailureInjector       FailureInjector
}

func RunUntilExhausted(ctx context.Context, cfg Config) (Result, error) {
	n, err := normalize(cfg)
	if err != nil {
		return Result{}, err
	}
	unlock, err := lockOperation(ctx, n.root, n.OperationID)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	op, found, err := load(n.root, n.OperationID)
	if err != nil {
		return Result{}, err
	}
	ledgerAdmitted := false
	if found {
		if err := compatible(op, n.Config); err != nil {
			return Result{}, err
		}
		if op.StopReason.Valid() {
			if err := admitQueueLedger(ctx, n, op); err != nil {
				return Result{}, err
			}
			if err := completeQueueLedger(context.WithoutCancel(ctx), n, op); err != nil {
				return resultOf(op, true), err
			}
			return resultOf(op, true), nil
		}
		if op.SchemaVersion == LegacyOperationSchemaVersion {
			if err := admitQueueLedger(ctx, n, op); err != nil {
				return Result{}, err
			}
			ledgerAdmitted = true
			before := op
			op.SchemaVersion = OperationSchemaVersion
			op.MaximumWorkers = 1
			if op.InFlight != nil {
				op.InFlight.Sequence = op.Statistics.Selections + 1
				op.InFlight.Batch = op.Statistics.Selections + 1
				op.InFlight.Slot = 1
				op.Slots = []WorkerSlot{{Selection: *op.InFlight, State: SlotAdmitted}}
				op.InFlight = nil
			}
			op.Sequence++
			op.UpdatedAt = n.Clock().UTC()
			if err := persist(n.root, before, op, n.FailureInjector); err != nil {
				return Result{}, err
			}
		}
	} else {
		now := n.Clock().UTC()
		op = Operation{SchemaVersion: OperationSchemaVersion, OperationID: n.OperationID, Mode: n.Mode, ConfigSchema: n.ConfigSchema, ConfigSHA256: n.ConfigSHA256, SafetyIdentity: n.SafetyIdentity, MaxTasks: n.MaxTasks, MaximumWorkers: n.MaximumWorkers, StartedAt: now, UpdatedAt: now, Sweep: n.Sweep, DaemonWakeCount: n.DaemonWakeCount, DaemonWakeFingerprint: n.DaemonWakeFingerprint, Stage: "admitted"}
		if err := persist(n.root, Operation{}, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
	}
	if !ledgerAdmitted {
		if err := admitQueueLedger(ctx, n, op); err != nil {
			return Result{}, err
		}
	}

	for {
		if err := ctx.Err(); err != nil && !hasAdmittedSlots(op.Slots) {
			result, stopErr := terminate(n, op, StopCancelled, err.Error(), nil)
			return result, errors.Join(err, stopErr)
		}
		if hasAdmittedSlots(op.Slots) {
			results := runBatch(ctx, n, op)
			after, loadErr := n.Loader(context.WithoutCancel(ctx))
			if loadErr != nil || !validHash(after.Fingerprint) {
				detail := errors.Join(loadErr, errors.New("failed to load post-batch scheduling authority")).Error()
				for i := range op.Slots {
					if op.Slots[i].State == SlotTerminal {
						continue
					}
					selection := op.Slots[i].Selection
					result := autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, TaskID: selection.TaskID, OperationID: selection.TaskOperationID, StopReason: autonomoustaskrun.StopUnsafeAmbiguous, StopDetail: detail}
					if err := reconcileSlot(n, &op, i, result, loadErr, selection.Fingerprint); err != nil {
						return resultOf(op, false), err
					}
				}
				result, stopErr := terminate(n, op, StopUnsafeAmbiguous, detail, nil)
				return result, errors.Join(loadErr, stopErr)
			}
			var joined error
			for i := range op.Slots {
				if op.Slots[i].State == SlotTerminal {
					continue
				}
				worker := results[i]
				if err := reconcileSlot(n, &op, i, worker.result, worker.err, after.Fingerprint); err != nil {
					return resultOf(op, false), errors.Join(joined, err)
				}
				joined = errors.Join(joined, worker.err)
			}
			if reason, detail := batchStop(op.Slots, ctx.Err()); reason != "" {
				result, stopErr := terminate(n, op, reason, detail, nil)
				return result, errors.Join(joined, ctx.Err(), stopErr)
			}
			continue
		}
		snapshot, err := n.Loader(ctx)
		if err != nil {
			return terminate(n, op, StopUnsafeAmbiguous, err.Error(), nil)
		}
		if !validHash(snapshot.Fingerprint) {
			return terminate(n, op, StopUnsafeAmbiguous, "scheduler loader returned an invalid exact fingerprint", nil)
		}
		op.Exclusions = liveExclusions(op.Exclusions, snapshot.Nodes)
		ready, waiting := eligible(snapshot.Nodes, op.Exclusions)
		if len(ready) == 0 {
			reason := classifyEmpty(snapshot.Nodes)
			return terminate(n, op, reason, stopDetail(reason), waiting)
		}
		if op.Statistics.TasksRun >= n.MaxTasks {
			remaining := taskIDs(ready)
			return terminate(n, op, StopBudgetExhausted, "queue maximum task bound reached with ready work remaining", append(remaining, waiting...))
		}
		batch, fallback, err := admitBatch(snapshot, ready, op.Exclusions, op.OperationID, op.Statistics.Selections+1, op.Statistics.Batches+1, n.MaximumWorkers, n.MaxTasks-op.Statistics.TasksRun)
		if err != nil {
			return terminate(n, op, StopUnsafeAmbiguous, err.Error(), waiting)
		}
		before := op
		op.Sequence++
		op.Stage = "selected"
		op.UpdatedAt = n.Clock().UTC()
		op.LastFingerprint = snapshot.Fingerprint
		op.Slots = batch
		op.SequentialFallback = fallback
		op.Statistics.Batches++
		if len(batch) > op.Statistics.PeakActiveWorkers {
			op.Statistics.PeakActiveWorkers = len(batch)
		}
		if fallback != "" {
			op.Statistics.SequentialFallbacks++
		}
		if err := persist(n.root, before, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
		if err := recordQueueEvent(ctx, n, op, ledger.EventQueueSelection); err != nil {
			return Result{}, err
		}
		emit(n.Progress, op)
	}
}

type workerResult struct {
	result autonomoustaskrun.Result
	err    error
}

func hasAdmittedSlots(slots []WorkerSlot) bool {
	for _, slot := range slots {
		if slot.State == SlotAdmitted {
			return true
		}
	}
	return false
}

func runBatch(ctx context.Context, n normalized, op Operation) []workerResult {
	results := make([]workerResult, len(op.Slots))
	var wg sync.WaitGroup
	for i := range op.Slots {
		if op.Slots[i].State != SlotAdmitted {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			selection := op.Slots[index].Selection
			child, cancel := context.WithCancel(ctx)
			defer cancel()
			defer func() {
				if recovered := recover(); recovered != nil {
					results[index] = workerResult{
						result: autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, TaskID: selection.TaskID, OperationID: selection.TaskOperationID, StopReason: autonomoustaskrun.StopUnsafeAmbiguous, StopDetail: fmt.Sprintf("queue worker panic: %v", recovered)},
						err:    fmt.Errorf("autonomous queue: worker %d panic: %v", selection.Slot, recovered),
					}
				}
			}()
			result, err := n.Runner(child, RunTaskInput{QueueOperationID: op.OperationID, TaskID: selection.TaskID, OperationID: selection.TaskOperationID, SelectionSequence: selection.Sequence, Batch: selection.Batch, Slot: selection.Slot})
			results[index] = workerResult{result: result, err: err}
		}(i)
	}
	wg.Wait()
	return results
}

func admitBatch(snapshot Snapshot, ready []autonomousscheduler.Node, exclusions []Exclusion, operationID string, firstSequence, batchSequence int64, maximumWorkers int, remainingTasks int64) ([]WorkerSlot, string, error) {
	limit := maximumWorkers
	if int64(limit) > remainingTasks {
		limit = int(remainingTasks)
	}
	if limit <= 0 || len(ready) == 0 {
		return nil, "", errors.New("autonomous queue: batch admission has no capacity or ready task")
	}
	selected := []autonomousscheduler.Node{ready[0]}
	fallback := ""
	if limit > 1 {
		if snapshot.Classify == nil {
			fallback = "overlap_authority_unavailable"
		} else {
			for len(selected) < limit {
				occupied := taskIDs(selected)
				classified, err := snapshot.Classify(occupied)
				if err != nil {
					if len(selected) == 0 {
						return nil, "", err
					}
					fallback = "overlap_authority_unavailable"
					break
				}
				candidates, _ := eligible(classified, exclusions)
				var next *autonomousscheduler.Node
				for i := range candidates {
					if !containsTask(selected, candidates[i].Task.ID) {
						next = &candidates[i]
						break
					}
				}
				if next == nil {
					if len(ready) > len(selected) {
						fallback = "no_additional_safe_candidate"
					}
					break
				}
				selected = append(selected, *next)
			}
		}
	}
	slots := make([]WorkerSlot, len(selected))
	for i, node := range selected {
		sequence := firstSequence + int64(i)
		selection := Selection{Sequence: sequence, Batch: batchSequence, Slot: i + 1, TaskID: node.Task.ID, TaskOperationID: deriveTaskOperationID(operationID, sequence, batchSequence, i+1, node.Task.ID, snapshot.Fingerprint, maximumWorkers), Fingerprint: snapshot.Fingerprint, Authority: nodeAuthority(node)}
		slots[i] = WorkerSlot{Selection: selection, State: SlotAdmitted}
	}
	return slots, fallback, nil
}

func containsTask(nodes []autonomousscheduler.Node, taskID string) bool {
	for _, node := range nodes {
		if node.Task.ID == taskID {
			return true
		}
	}
	return false
}

func batchStop(slots []WorkerSlot, parentErr error) (StopReason, string) {
	if parentErr != nil {
		return StopCancelled, parentErr.Error()
	}
	for _, slot := range slots {
		if slot.Outcome != nil && slot.Outcome.StopReason == autonomoustaskrun.StopSafety {
			return StopSafety, slot.Outcome.StopDetail
		}
	}
	for _, slot := range slots {
		if slot.Outcome == nil {
			continue
		}
		switch slot.Outcome.StopReason {
		case autonomoustaskrun.StopOperationCancelled:
			return StopCancelled, slot.Outcome.StopDetail
		case autonomoustaskrun.StopUnsafeAmbiguous, autonomoustaskrun.StopNoTask:
			return StopUnsafeAmbiguous, slot.Outcome.StopDetail
		}
	}
	return "", ""
}

type normalized struct {
	Config
	root string
}

func normalize(cfg Config) (normalized, error) {
	root, err := canonicalRoot(cfg.RepositoryRoot)
	if err != nil {
		return normalized{}, err
	}
	if cfg.MaximumWorkers == 0 {
		cfg.MaximumWorkers = 1
	}
	if !safeID(cfg.OperationID) || (cfg.Mode != ModeUntilExhausted && cfg.Mode != ModeDaemon) || strings.TrimSpace(cfg.ConfigSchema) == "" || !validHash(cfg.ConfigSHA256) || strings.TrimSpace(cfg.SafetyIdentity) == "" || cfg.MaxTasks <= 0 || cfg.MaximumWorkers <= 0 || cfg.MaximumWorkers > MaximumWorkerLimit || cfg.Sweep <= 0 || cfg.DaemonWakeCount < 0 || cfg.DaemonWakeFingerprint != "" && !validHash(cfg.DaemonWakeFingerprint) || (cfg.DaemonWakeCount == 0) != (cfg.DaemonWakeFingerprint == "") || cfg.Clock == nil || cfg.Loader == nil || cfg.Runner == nil {
		return normalized{}, errors.New("autonomous queue: exact identity, mode, configuration, safety, bounds, clock, loader, and runner are required")
	}
	return normalized{Config: cfg, root: root}, nil
}

func compatible(op Operation, cfg Config) error {
	workers := op.MaximumWorkers
	if op.SchemaVersion == LegacyOperationSchemaVersion {
		workers = 1
	}
	if op.OperationID != cfg.OperationID || op.Mode != cfg.Mode || op.ConfigSchema != cfg.ConfigSchema || op.ConfigSHA256 != cfg.ConfigSHA256 || op.SafetyIdentity != cfg.SafetyIdentity || op.MaxTasks != cfg.MaxTasks || workers != cfg.MaximumWorkers || op.Sweep != cfg.Sweep || op.DaemonWakeCount != cfg.DaemonWakeCount || op.DaemonWakeFingerprint != cfg.DaemonWakeFingerprint {
		return errors.New("autonomous queue: operation identity was reused with different material")
	}
	return nil
}

func reconcileSlot(n normalized, op *Operation, slotIndex int, result autonomoustaskrun.Result, runErr error, afterFingerprint string) error {
	selection := op.Slots[slotIndex].Selection
	if (errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)) && (result.TaskID == "" || !result.StopReason.Valid()) {
		result = autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, TaskID: selection.TaskID, OperationID: selection.TaskOperationID, StopReason: autonomoustaskrun.StopOperationCancelled, StopDetail: runErr.Error()}
	}
	if result.TaskID != selection.TaskID || result.OperationID != selection.TaskOperationID || !result.StopReason.Valid() {
		result = autonomoustaskrun.Result{SchemaVersion: autonomoustaskrun.ResultSchemaVersion, TaskID: selection.TaskID, OperationID: selection.TaskOperationID, StopReason: autonomoustaskrun.StopUnsafeAmbiguous, StopDetail: "pinned task runner returned conflicting or incomplete terminal evidence"}
		runErr = errors.Join(runErr, errors.New("autonomous queue: ambiguous task result"))
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		result.StopReason = autonomoustaskrun.StopUnsafeAmbiguous
		result.StopDetail = errors.Join(runErr, errors.New(result.StopDetail)).Error()
	}
	outcome := TaskOutcome{SelectionSequence: selection.Sequence, Batch: selection.Batch, Slot: selection.Slot, TaskID: result.TaskID, TaskOperationID: result.OperationID, StopReason: result.StopReason, StopDetail: redact(n.Redact, result.StopDetail), BeforeFingerprint: selection.Fingerprint, AfterFingerprint: afterFingerprint, Authority: selection.Authority, Statistics: result.Statistics, Evidence: append([]string(nil), result.Evidence...), Replayed: result.Replayed}
	before := *op
	op.Sequence++
	op.Stage = "task_stopped"
	op.UpdatedAt = n.Clock().UTC()
	op.Slots[slotIndex].State = SlotTerminal
	op.Slots[slotIndex].Outcome = &outcome
	op.Outcomes = append(op.Outcomes, outcome)
	op.Statistics.add(result.StopReason)
	op.LastFingerprint = afterFingerprint
	if safeYield(result.StopReason) {
		op.Exclusions = addExclusion(op.Exclusions, Exclusion{TaskID: result.TaskID, Authority: selection.Authority})
	}
	if err := persist(n.root, before, *op, n.FailureInjector); err != nil {
		return err
	}
	if err := recordQueueEvent(context.Background(), n, *op, ledger.EventQueueTaskStopped); err != nil {
		return err
	}
	emit(n.Progress, *op)
	return nil
}

func terminate(n normalized, op Operation, reason StopReason, detail string, remaining []string) (Result, error) {
	before := op
	op.Sequence++
	op.Stage = "terminal"
	op.InFlight = nil
	op.StopReason = reason
	op.StopDetail = redact(n.Redact, detail)
	op.UpdatedAt = n.Clock().UTC()
	now := op.UpdatedAt
	op.CompletedAt = &now
	op.RemainingWaiting = uniqueSorted(remaining)
	if reason == StopBudgetExhausted {
		op.RemainingReady = uniqueSorted(remaining)
	}
	if err := persist(n.root, before, op, n.FailureInjector); err != nil {
		return Result{}, err
	}
	if err := completeQueueLedger(context.Background(), n, op); err != nil {
		return resultOf(op, false), err
	}
	emit(n.Progress, op)
	return resultOf(op, false), nil
}

func eligible(nodes []autonomousscheduler.Node, exclusions []Exclusion) ([]autonomousscheduler.Node, []string) {
	excluded := make(map[string]string, len(exclusions))
	for _, item := range exclusions {
		excluded[item.TaskID] = item.Authority
	}
	var ready []autonomousscheduler.Node
	var waiting []string
	for _, node := range nodes {
		if node.Task.Workflow == taskfile.WorkflowAutonomousV1 && node.Lifecycle == "needs_input" {
			waiting = append(waiting, node.Task.ID+":"+string(autonomousscheduler.ReasonNeedsInput))
			continue
		}
		if node.Task.Workflow == taskfile.WorkflowAutonomousV1 && (node.Task.Status == taskfile.StatusBlocked || node.Lifecycle == taskfile.StatusBlocked) {
			waiting = append(waiting, node.Task.ID+":blocked")
			continue
		}
		if node.Reason == autonomousscheduler.ReasonReady {
			if excluded[node.Task.ID] != nodeAuthority(node) {
				ready = append(ready, node)
			} else {
				waiting = append(waiting, node.Task.ID+":yielded")
			}
			continue
		}
		if node.Task.Workflow == taskfile.WorkflowAutonomousV1 && node.Task.Status != taskfile.StatusCompleted {
			waiting = append(waiting, node.Task.ID+":"+string(node.Reason))
		}
	}
	return ready, uniqueSorted(waiting)
}

func liveExclusions(exclusions []Exclusion, nodes []autonomousscheduler.Node) []Exclusion {
	byID := make(map[string]autonomousscheduler.Node, len(nodes))
	for _, node := range nodes {
		byID[node.Task.ID] = node
	}
	result := exclusions[:0]
	for _, exclusion := range exclusions {
		if node, ok := byID[exclusion.TaskID]; ok && nodeAuthority(node) == exclusion.Authority {
			result = append(result, exclusion)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TaskID < result[j].TaskID })
	return result
}

func classifyEmpty(nodes []autonomousscheduler.Node) StopReason {
	var pending, input, blocked bool
	for _, node := range nodes {
		if node.Task.Workflow != taskfile.WorkflowAutonomousV1 {
			continue
		}
		if node.Lifecycle == "needs_input" {
			input = true
		}
		if node.Task.Status == taskfile.StatusBlocked || node.Lifecycle == taskfile.StatusBlocked {
			blocked = true
		}
		if node.Task.Status == taskfile.StatusPending && node.Lifecycle != "completed" {
			pending = true
		}
	}
	if input {
		return StopWaitingInput
	}
	if blocked {
		return StopWaitingBlocked
	}
	if pending {
		return StopWaitingDependency
	}
	return StopDrained
}

func stopDetail(reason StopReason) string {
	switch reason {
	case StopDrained:
		return "no active pending autonomous work remains"
	case StopWaitingInput:
		return "no unrelated ready work remains and structured operator input is required"
	case StopWaitingBlocked:
		return "no unrelated ready work remains and active work is blocked"
	default:
		return "no eligible ready work remains"
	}
}

func safeYield(reason autonomoustaskrun.StopReason) bool {
	switch reason {
	case autonomoustaskrun.StopCompleted, autonomoustaskrun.StopBlocked, autonomoustaskrun.StopNeedsInput, autonomoustaskrun.StopBudgetExhausted, autonomoustaskrun.StopNoProgress, autonomoustaskrun.StopTaskCancelled, autonomoustaskrun.StopMaxCycles:
		return true
	default:
		return false
	}
}

func nodeAuthority(node autonomousscheduler.Node) string {
	return hashStrings(node.Task.ID, node.Task.SourceSHA256(), node.StateSHA256, fmt.Sprint(node.StateByteSize), node.Lifecycle, node.Task.Status)
}

func deriveTaskOperationID(queueID string, selection, batch int64, slot int, taskID, fingerprint string, maximumWorkers int) string {
	return "task-run-" + hashStrings("autonomous-queue-task-v2", queueID, fmt.Sprint(selection), fmt.Sprint(batch), fmt.Sprint(slot), taskID, fingerprint, fmt.Sprint(maximumWorkers))[:24]
}

func addExclusion(items []Exclusion, item Exclusion) []Exclusion {
	for i := range items {
		if items[i].TaskID == item.TaskID {
			items[i] = item
			sort.Slice(items, func(i, j int) bool { return items[i].TaskID < items[j].TaskID })
			return items
		}
	}
	items = append(items, item)
	sort.Slice(items, func(i, j int) bool { return items[i].TaskID < items[j].TaskID })
	return items
}

func taskIDs(nodes []autonomousscheduler.Node) []string {
	result := make([]string, len(nodes))
	for i := range nodes {
		result[i] = nodes[i].Task.ID
	}
	return result
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func redact(redactor func(string) string, value string) string {
	if redactor == nil {
		return value
	}
	return redactor(value)
}

func emit(progress Progress, op Operation) {
	if progress != nil {
		defer func() { _ = recover() }()
		progress(op)
	}
}
