package autonomousqueue

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
)

type Snapshot struct {
	Fingerprint string
	Nodes       []autonomousscheduler.Node
}

type Loader func(context.Context) (Snapshot, error)

type RunTaskInput struct {
	TaskID, OperationID string
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
	} else {
		now := n.Clock().UTC()
		op = Operation{SchemaVersion: OperationSchemaVersion, OperationID: n.OperationID, Mode: n.Mode, ConfigSchema: n.ConfigSchema, ConfigSHA256: n.ConfigSHA256, SafetyIdentity: n.SafetyIdentity, MaxTasks: n.MaxTasks, StartedAt: now, UpdatedAt: now, Sweep: n.Sweep, DaemonWakeCount: n.DaemonWakeCount, DaemonWakeFingerprint: n.DaemonWakeFingerprint, Stage: "admitted"}
		if err := persist(n.root, Operation{}, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
	}
	if err := admitQueueLedger(ctx, n, op); err != nil {
		return Result{}, err
	}

	for {
		if err := ctx.Err(); err != nil {
			result, stopErr := terminate(n, op, StopCancelled, err.Error(), nil)
			return result, errors.Join(err, stopErr)
		}
		if op.InFlight != nil {
			result, runErr := n.Runner(ctx, RunTaskInput{TaskID: op.InFlight.TaskID, OperationID: op.InFlight.TaskOperationID})
			if err := reconcileTask(n, &op, result, runErr); err != nil {
				return resultOf(op, false), err
			}
			if op.StopReason.Valid() {
				return resultOf(op, false), runErr
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
			reason := classifyEmpty(snapshot.Nodes, op.Outcomes)
			return terminate(n, op, reason, stopDetail(reason), waiting)
		}
		if op.Statistics.TasksRun >= n.MaxTasks {
			remaining := taskIDs(ready)
			return terminate(n, op, StopBudgetExhausted, "queue maximum task bound reached with ready work remaining", append(remaining, waiting...))
		}
		node := ready[0]
		authority := nodeAuthority(node)
		selection := Selection{TaskID: node.Task.ID, TaskOperationID: deriveTaskOperationID(op.OperationID, op.Statistics.Selections+1, node.Task.ID, snapshot.Fingerprint), Fingerprint: snapshot.Fingerprint, Authority: authority}
		before := op
		op.Sequence++
		op.Stage = "selected"
		op.UpdatedAt = n.Clock().UTC()
		op.LastFingerprint = snapshot.Fingerprint
		op.InFlight = &selection
		if err := persist(n.root, before, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
		if err := recordQueueEvent(ctx, n, op, ledger.EventQueueSelection); err != nil {
			return Result{}, err
		}
		emit(n.Progress, op)
	}
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
	if !safeID(cfg.OperationID) || (cfg.Mode != ModeUntilExhausted && cfg.Mode != ModeDaemon) || strings.TrimSpace(cfg.ConfigSchema) == "" || !validHash(cfg.ConfigSHA256) || strings.TrimSpace(cfg.SafetyIdentity) == "" || cfg.MaxTasks <= 0 || cfg.Sweep <= 0 || cfg.DaemonWakeCount < 0 || cfg.DaemonWakeFingerprint != "" && !validHash(cfg.DaemonWakeFingerprint) || (cfg.DaemonWakeCount == 0) != (cfg.DaemonWakeFingerprint == "") || cfg.Clock == nil || cfg.Loader == nil || cfg.Runner == nil {
		return normalized{}, errors.New("autonomous queue: exact identity, mode, configuration, safety, bounds, clock, loader, and runner are required")
	}
	return normalized{Config: cfg, root: root}, nil
}

func compatible(op Operation, cfg Config) error {
	if op.OperationID != cfg.OperationID || op.Mode != cfg.Mode || op.ConfigSchema != cfg.ConfigSchema || op.ConfigSHA256 != cfg.ConfigSHA256 || op.SafetyIdentity != cfg.SafetyIdentity || op.MaxTasks != cfg.MaxTasks || op.Sweep != cfg.Sweep || op.DaemonWakeCount != cfg.DaemonWakeCount || op.DaemonWakeFingerprint != cfg.DaemonWakeFingerprint {
		return errors.New("autonomous queue: operation identity was reused with different material")
	}
	return nil
}

func reconcileTask(n normalized, op *Operation, result autonomoustaskrun.Result, runErr error) error {
	selection := *op.InFlight
	if result.TaskID != selection.TaskID || result.OperationID != selection.TaskOperationID || !result.StopReason.Valid() {
		_, err := terminate(n, *op, StopUnsafeAmbiguous, "pinned task runner returned conflicting or incomplete terminal evidence", nil)
		if err == nil {
			loaded, _, _ := load(n.root, op.OperationID)
			*op = loaded
		}
		return errors.Join(runErr, err, errors.New("autonomous queue: ambiguous task result"))
	}
	after, loadErr := n.Loader(context.Background())
	if loadErr != nil || !validHash(after.Fingerprint) {
		_, stopErr := terminate(n, *op, StopUnsafeAmbiguous, errors.Join(loadErr, errors.New("failed to load post-task scheduling authority")).Error(), nil)
		if stopErr == nil {
			loaded, _, _ := load(n.root, op.OperationID)
			*op = loaded
		}
		return errors.Join(loadErr, stopErr)
	}
	outcome := TaskOutcome{TaskID: result.TaskID, TaskOperationID: result.OperationID, StopReason: result.StopReason, StopDetail: redact(n.Redact, result.StopDetail), BeforeFingerprint: selection.Fingerprint, AfterFingerprint: after.Fingerprint, Authority: selection.Authority, Statistics: result.Statistics, Evidence: append([]string(nil), result.Evidence...), Replayed: result.Replayed}
	before := *op
	op.Sequence++
	op.Stage = "task_stopped"
	op.UpdatedAt = n.Clock().UTC()
	op.InFlight = nil
	op.Outcomes = append(op.Outcomes, outcome)
	op.Statistics.add(result.StopReason)
	op.LastFingerprint = after.Fingerprint
	if safeYield(result.StopReason) {
		op.Exclusions = addExclusion(op.Exclusions, Exclusion{TaskID: result.TaskID, Authority: selection.Authority})
	}
	if result.StopReason == autonomoustaskrun.StopOperationCancelled || errors.Is(runErr, context.Canceled) {
		now := op.UpdatedAt
		op.CompletedAt, op.Stage, op.StopReason, op.StopDetail = &now, "terminal", StopCancelled, outcome.StopDetail
	} else if result.StopReason == autonomoustaskrun.StopSafety {
		now := op.UpdatedAt
		op.CompletedAt, op.Stage, op.StopReason, op.StopDetail = &now, "terminal", StopSafety, outcome.StopDetail
	} else if result.StopReason == autonomoustaskrun.StopUnsafeAmbiguous || result.StopReason == autonomoustaskrun.StopNoTask || runErr != nil {
		now := op.UpdatedAt
		op.CompletedAt, op.Stage, op.StopReason, op.StopDetail = &now, "terminal", StopUnsafeAmbiguous, redact(n.Redact, errors.Join(runErr, errors.New(outcome.StopDetail)).Error())
	}
	if err := persist(n.root, before, *op, n.FailureInjector); err != nil {
		return err
	}
	if err := recordQueueEvent(context.Background(), n, *op, ledger.EventQueueTaskStopped); err != nil {
		return err
	}
	if op.StopReason.Valid() {
		if err := completeQueueLedger(context.Background(), n, *op); err != nil {
			return err
		}
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

func classifyEmpty(nodes []autonomousscheduler.Node, outcomes []TaskOutcome) StopReason {
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

func deriveTaskOperationID(queueID string, selection int64, taskID, fingerprint string) string {
	return "task-run-" + hashStrings("autonomous-queue-task-v1", queueID, fmt.Sprint(selection), taskID, fingerprint)[:24]
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
