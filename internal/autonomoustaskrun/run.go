package autonomoustaskrun

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

type StepInput struct {
	Operation Operation
	Cycle     int64
}
type StepResult struct {
	StopReason     StopReason
	StopDetail     string
	Action         string
	RunID          string
	DecisionID     string
	Evidence       []string
	Statistics     Statistics
	LatestMutation *autonomouspolicy.SourceMutation
	Verification   *autonomouspolicy.VerificationEvidence
	Audit          *autonomouspolicy.AuditEvidence
	Metrics        *MetricsEvidence
}
type StepRunner func(context.Context, StepInput) (StepResult, error)
type Progress func(Operation)

type Ledger interface {
	CreateRun(context.Context, ledger.RunSpec) (ledger.Run, error)
	GetRunWithEvents(context.Context, string) (ledger.RunWithEvents, bool, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
	CompleteRun(context.Context, string, ledger.RunCompletion) (ledger.Run, bool, error)
}

type Config struct {
	RepositoryRoot  string
	OperationID     string
	TaskID          string
	ConfigSHA256    string
	MaxCycles       MaxCycles
	Clock           func() time.Time
	Runner          StepRunner
	Progress        Progress
	Redact          func(string) string
	Ledger          Ledger
	FailureInjector FailureInjector
	ArchiveEvidence []autonomousscheduler.ArchiveEvidence
	OccupiedTaskIDs []string
}

func RunTaskUntilTerminal(ctx context.Context, cfg Config) (Result, error) {
	n, err := normalize(cfg)
	if err != nil {
		return Result{}, err
	}
	unlock, err := lockOperation(ctx, n.root, n.OperationID)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	op, found, err := loadOperation(n.root, n.OperationID)
	if err != nil {
		return Result{}, err
	}
	if found {
		if err := compatible(op, n); err != nil {
			return Result{}, err
		}
		if op.StopReason.Valid() {
			if err := admitLoopLedger(ctx, n, op); err != nil {
				return Result{}, err
			}
			if err := completeLoopLedger(context.WithoutCancel(ctx), n, op); err != nil {
				return resultOf(op, true), err
			}
			return resultOf(op, true), nil
		}
		if op.InFlight {
			return stop(n, op, StopUnsafeAmbiguous, "restart found an in-flight cycle without exact reconciliation evidence")
		}
	} else {
		if err := ctx.Err(); err != nil {
			return Result{SchemaVersion: ResultSchemaVersion, StopReason: StopOperationCancelled, StopDetail: err.Error()}, err
		}
		task, ok, err := resolveTaskScheduled(ctx, n.root, n.TaskID, n.ArchiveEvidence, n.OccupiedTaskIDs)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			return Result{SchemaVersion: ResultSchemaVersion, StopReason: StopNoTask}, nil
		}
		op, err = admit(n, task)
		if err != nil {
			return Result{}, err
		}
		if err := persist(n.root, Operation{}, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
	}
	if err := admitLoopLedger(ctx, n, op); err != nil {
		return Result{}, err
	}
	if found {
		if err := recordLoopEvent(ctx, n, op, ledger.EventTaskRunRestarted); err != nil {
			return Result{}, err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			result, stopErr := stop(n, op, StopOperationCancelled, err.Error())
			return result, errors.Join(err, stopErr)
		}
		terminal, detail, err := canonicalTerminal(n.root, op.TaskID)
		if err != nil {
			return stop(n, op, StopUnsafeAmbiguous, err.Error())
		}
		if terminal != "" {
			return stop(n, op, terminal, detail)
		}
		if n.MaxCycles.Mode == "limited" && op.Statistics.CyclesStarted >= n.MaxCycles.Limit {
			return stop(n, op, StopMaxCycles, "caller-owned maximum cycle limit reached")
		}
		before := op
		op.Sequence++
		op.Stage = "cycle_started"
		op.InFlight = true
		op.UpdatedAt = n.Clock().UTC()
		op.Statistics.CyclesStarted++
		op.Statistics.SupervisorStarted++
		if err := persist(n.root, before, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
		if err := recordLoopEvent(ctx, n, op, ledger.EventTaskRunCycleStarted); err != nil {
			return Result{}, err
		}
		emit(n.Progress, op)
		if n.Runner == nil {
			return stop(n, op, StopUnsafeAmbiguous, "production autonomous step runner is unavailable")
		}
		step, runErr := n.Runner(ctx, StepInput{Operation: op, Cycle: op.Statistics.CyclesStarted})
		step.StopDetail = redactText(n.Redact, step.StopDetail)
		step.Action = redactText(n.Redact, step.Action)
		step.RunID = redactText(n.Redact, step.RunID)
		step.DecisionID = redactText(n.Redact, step.DecisionID)
		for i := range step.Evidence {
			step.Evidence[i] = redactText(n.Redact, step.Evidence[i])
		}
		before = op
		op.InFlight = false
		op.Stage = "cycle_completed"
		op.UpdatedAt = n.Clock().UTC()
		op.Statistics.CyclesCompleted++
		op.Statistics.SupervisorCompleted++
		op.Statistics.Add(step.Statistics)
		op.LastAction = strings.TrimSpace(step.Action)
		op.LastRunID = strings.TrimSpace(step.RunID)
		op.LastDecisionID = strings.TrimSpace(step.DecisionID)
		op.Evidence = append([]string(nil), step.Evidence...)
		op.LatestMutation = step.LatestMutation
		op.Verification = step.Verification
		op.Audit = step.Audit
		op.Metrics = step.Metrics
		if authority, authorityErr := currentAuthority(n.root, op.TaskID); authorityErr == nil {
			op.State = authority.State
			op.WorkspaceID = authority.WorkspaceID
			op.CheckpointSHA = authority.CheckpointSHA
			op.Metrics = authority.Metrics
		} else if !errors.Is(authorityErr, autonomousstate.ErrTaskMissing) {
			runErr = errors.Join(runErr, fmt.Errorf("reload pinned authority: %w", authorityErr))
		}
		if runErr != nil {
			if errors.Is(runErr, context.Canceled) || ctx.Err() != nil {
				op.StopReason = StopOperationCancelled
			} else {
				op.StopReason = StopUnsafeAmbiguous
			}
			op.StopDetail = redactText(n.Redact, runErr.Error())
		} else if step.StopReason != "" {
			if !step.StopReason.Valid() {
				return Result{}, fmt.Errorf("task run: invalid step stop reason %q", step.StopReason)
			}
			op.StopReason, op.StopDetail = step.StopReason, strings.TrimSpace(step.StopDetail)
		}
		if op.StopReason.Valid() {
			now := n.Clock().UTC()
			op.CompletedAt = &now
			op.Stage = "terminal"
		}
		if err := persist(n.root, before, op, n.FailureInjector); err != nil {
			return Result{}, err
		}
		if err := recordLoopEvent(context.WithoutCancel(ctx), n, op, ledger.EventTaskRunCycleCompleted); err != nil {
			return resultOf(op, false), err
		}
		if op.StopReason.Valid() {
			if err := completeLoopLedger(context.WithoutCancel(ctx), n, op); err != nil {
				return resultOf(op, false), err
			}
		}
		emit(n.Progress, op)
		if op.StopReason.Valid() {
			if runErr != nil {
				return resultOf(op, false), runErr
			}
			return resultOf(op, false), nil
		}
	}
}

type authoritySnapshot struct {
	State                      Identity
	WorkspaceID, CheckpointSHA string
	Metrics                    *MetricsEvidence
}

func currentAuthority(root, taskID string) (authoritySnapshot, error) {
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		return authoritySnapshot{}, err
	}
	snap, found, err := store.Load(context.Background(), taskID)
	if err != nil {
		return authoritySnapshot{}, err
	}
	if !found {
		return authoritySnapshot{}, autonomousstate.ErrStateMissing
	}
	raw, err := canonical(snap.State)
	if err != nil {
		return authoritySnapshot{}, err
	}
	identity := hashBytes(raw)
	identity.Path = snap.SourcePath
	result := authoritySnapshot{State: identity, Metrics: metricsEvidence(snap.State)}
	if snap.State.Workspace != nil {
		result.WorkspaceID = snap.State.Workspace.WorkspaceID
		result.CheckpointSHA = snap.State.Workspace.Checkpoint.CommitSHA
	}
	return result, nil
}

type normalized struct {
	Config
	root string
}

func normalize(cfg Config) (normalized, error) {
	root, err := runtimepath.CanonicalRoot(strings.TrimSpace(cfg.RepositoryRoot))
	if err != nil {
		return normalized{}, err
	}
	if !safeID(cfg.OperationID) {
		return normalized{}, errors.New("task run: safe operation ID is required")
	}
	if cfg.TaskID != "" && !safeID(cfg.TaskID) {
		return normalized{}, errors.New("task run: task ID is malformed")
	}
	if len(cfg.ConfigSHA256) != 64 {
		return normalized{}, errors.New("task run: effective configuration SHA-256 is required")
	}
	if err := cfg.MaxCycles.Validate(); err != nil {
		return normalized{}, err
	}
	if cfg.Clock == nil {
		return normalized{}, errors.New("task run: clock is required")
	}
	return normalized{Config: cfg, root: root}, nil
}
func resolveTask(root, id string) (taskfile.Task, bool, error) {
	return resolveTaskScheduled(context.Background(), root, id, nil, nil)
}

func resolveTaskScheduled(ctx context.Context, root, id string, archives []autonomousscheduler.ArchiveEvidence, occupied []string) (taskfile.Task, bool, error) {
	active, err := autonomousscheduler.LoadActiveStrict(ctx, root)
	if err != nil {
		return taskfile.Task{}, false, err
	}
	graph, err := autonomousscheduler.BuildSnapshot(active, archives)
	if err != nil {
		return taskfile.Task{}, false, err
	}
	if id == "" {
		selection := autonomousscheduler.SelectNextReady(graph, occupied)
		if !selection.Found {
			return taskfile.Task{}, false, nil
		}
		return selection.Task, true, nil
	}
	node, err := autonomousscheduler.ClassifyTask(graph, id, occupied)
	if err != nil {
		return taskfile.Task{}, false, err
	}
	if node.Reason != taskscheduler.ReasonReady {
		return taskfile.Task{}, false, fmt.Errorf("task run: explicit task %q is not ready: %s (dependencies=%v conflicts=%v)", id, node.Reason, node.WaitingOn, node.Conflicts)
	}
	return node.Task, true, nil
}
func admit(n normalized, task taskfile.Task) (Operation, error) {
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: n.root})
	if err != nil {
		return Operation{}, err
	}
	snap, ok, err := store.Load(context.Background(), task.ID)
	if err != nil || !ok {
		if err == nil {
			err = errors.New("task run: autonomous state is missing")
		}
		return Operation{}, err
	}
	stateRaw, err := canonical(snap.State)
	if err != nil {
		return Operation{}, err
	}
	now := n.Clock().UTC()
	taskID := hashBytes(task.SourceBytes)
	taskID.Path = task.SourcePath
	stateID := hashBytes(stateRaw)
	stateID.Path = snap.SourcePath
	op := Operation{SchemaVersion: OperationSchemaVersion, OperationID: n.OperationID, TaskID: task.ID, Task: taskID, State: stateID, ConfigSHA256: n.ConfigSHA256, MaxCycles: n.MaxCycles, StartedAt: now, UpdatedAt: now, Stage: "admitted"}
	op.Metrics = metricsEvidence(snap.State)
	if snap.State.Workspace != nil {
		op.WorkspaceID = snap.State.Workspace.WorkspaceID
		op.CheckpointSHA = snap.State.Workspace.Checkpoint.CommitSHA
	}
	return op, nil
}

func metricsEvidence(state autonomous.ExecutionState) *MetricsEvidence {
	result := &MetricsEvidence{
		Attempts:           state.Attempts,
		FindingResolutions: append([]autonomous.FindingResolution(nil), state.FindingResolutions...),
	}
	if state.CircuitBreaker != nil {
		value := *state.CircuitBreaker
		result.CircuitBreaker = &value
	}
	if state.Finalization != nil {
		value := *state.Finalization
		result.Finalization = &value
	}
	return result
}
func canonicalTerminal(root, taskID string) (StopReason, string, error) {
	t, ok, err := taskfile.FindByID(root, taskID)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return StopUnsafeAmbiguous, "pinned active task disappeared; archive/finalization reconciliation is required", nil
	}
	switch t.Status {
	case taskfile.StatusCompleted:
		return StopCompleted, "canonical task is completed", nil
	case taskfile.StatusBlocked:
		return StopBlocked, "canonical task is blocked", nil
	case taskfile.StatusCancelled:
		return StopTaskCancelled, "canonical task is cancelled", nil
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		return "", "", err
	}
	s, ok, err := store.Load(context.Background(), taskID)
	if err != nil || !ok {
		return "", "", err
	}
	if s.State.Lifecycle == autonomous.LifecycleStateNeedsInput {
		detail := "canonical task needs input"
		if s.State.NeedsInput != nil {
			detail = s.State.NeedsInput.Reason
		}
		return StopNeedsInput, detail, nil
	}
	if s.State.Lifecycle == autonomous.LifecycleStateCompleted {
		return StopCompleted, "canonical autonomous state is completed", nil
	}
	if s.State.Lifecycle == autonomous.LifecycleStateBlocked {
		var reason autonomous.BreakerReason
		if s.State.CircuitBreaker != nil {
			reason = s.State.CircuitBreaker.Reason
		} else if len(s.State.Attempts.ActionStops) > 0 {
			reason = s.State.Attempts.ActionStops[len(s.State.Attempts.ActionStops)-1].Reason
		}
		if reason != "" {
			switch reason {
			case autonomous.BreakerTaskAttemptsExhausted, autonomous.BreakerActionAttemptsExhausted, autonomous.BreakerElapsedExhausted, autonomous.BreakerTokenExhausted:
				return StopBudgetExhausted, string(reason), nil
			case autonomous.BreakerUnchangedSource, autonomous.BreakerRepeatedSignature, autonomous.BreakerIdenticalStrategy:
				return StopNoProgress, string(reason), nil
			case autonomous.BreakerUnsafeSource, autonomous.BreakerStaleEvidence, autonomous.BreakerAccountingSafety:
				return StopSafety, string(reason), nil
			}
		}
		return StopBlocked, "canonical autonomous state is blocked", nil
	}
	if s.State.Lifecycle == autonomous.LifecycleStateCancelled {
		return StopTaskCancelled, "canonical autonomous state is cancelled", nil
	}
	return "", "", nil
}
func compatible(op Operation, n normalized) error {
	if op.SchemaVersion != OperationSchemaVersion || op.OperationID != n.OperationID || (n.TaskID != "" && op.TaskID != n.TaskID) || op.ConfigSHA256 != n.ConfigSHA256 || op.MaxCycles != n.MaxCycles {
		return errors.New("task run: durable operation conflicts with requested task, configuration, or cycle limit")
	}
	return nil
}
func stop(n normalized, op Operation, reason StopReason, detail string) (Result, error) {
	before := op
	op.StopReason, op.StopDetail = reason, redactText(n.Redact, detail)
	op.Stage = "terminal"
	op.InFlight = false
	op.UpdatedAt = n.Clock().UTC()
	now := op.UpdatedAt
	op.CompletedAt = &now
	if err := persist(n.root, before, op, n.FailureInjector); err != nil {
		return Result{}, err
	}
	if err := completeLoopLedger(context.Background(), n, op); err != nil {
		return resultOf(op, false), err
	}
	emit(n.Progress, op)
	return resultOf(op, false), nil
}
func redactText(redact func(string) string, value string) string {
	if redact == nil {
		return value
	}
	return redact(value)
}
func resultOf(op Operation, replay bool) Result {
	return Result{SchemaVersion: ResultSchemaVersion, OperationID: op.OperationID, TaskID: op.TaskID, StopReason: op.StopReason, StopDetail: op.StopDetail, Statistics: op.Statistics, LastAction: op.LastAction, LastRunID: op.LastRunID, LastDecisionID: op.LastDecisionID, Evidence: append([]string(nil), op.Evidence...), Replayed: replay}
}
func emit(progress Progress, op Operation) {
	if progress != nil {
		defer func() { _ = recover() }()
		progress(op)
	}
}
func lockOperation(ctx context.Context, root, id string) (func(), error) {
	rel, err := filepath.Rel(root, filepath.Join(operationDir(root, id), "operation.lock"))
	if err != nil {
		return nil, err
	}
	lease, err := lock.AcquireFlock(ctx, root, lock.FlockConfig{
		RelativePath: filepath.ToSlash(rel),
		Mode:         lock.FlockExclusive,
		Wait:         true,
		Create:       true,
	})
	if err != nil {
		return nil, err
	}
	return func() { _ = lease.Close() }, nil
}
