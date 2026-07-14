package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousattempt"
	"revolvr/internal/autonomousauditapply"
	"revolvr/internal/autonomousblock"
	"revolvr/internal/autonomouschild"
	"revolvr/internal/autonomouscorrection"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomousdaemon"
	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomousinput"
	"revolvr/internal/autonomousoptional"
	"revolvr/internal/autonomousplanapply"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/autonomousworkspace"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/id"
	"revolvr/internal/ledger"
	"revolvr/internal/redact"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

type TaskRunInput struct {
	OperationID, TaskID string
	MaxCycles           int64
	Unlimited           bool
	Runner              autonomoustaskrun.StepRunner
	Clock               func() time.Time
	Progress            autonomoustaskrun.Progress
	RunConfig           *runonce.Config
	Notification        NotificationObserver
	NotificationRuntime NotificationRuntime
}

func RunTaskUntilTerminal(ctx context.Context, cfg Config, input TaskRunInput) (autonomoustaskrun.Result, error) {
	unlock, err := autonomousexec.Acquire(ctx, cfg.WorkDir)
	if err != nil {
		return autonomoustaskrun.Result{}, err
	}
	result, runErr := runTaskUntilTerminal(ctx, cfg, input)
	unlock()
	dispatchTaskOutcome(ctx, cfg.WorkDir, result, input.NotificationRuntime, input.Notification)
	return result, runErr
}

func runTaskUntilTerminal(ctx context.Context, cfg Config, input TaskRunInput) (autonomoustaskrun.Result, error) {
	runCfg, err := LoadRunOnceConfig(cfg.WorkDir, DefaultRunOnceConfig(cfg.WorkDir))
	if input.RunConfig != nil {
		runCfg = *input.RunConfig
		runCfg.WorkingDir = cfg.WorkDir
		err = nil
	}
	if err != nil {
		return autonomoustaskrun.Result{}, err
	}
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		return autonomoustaskrun.Result{}, err
	}
	fingerprint, err := runonce.FingerprintEffectiveConfig(effective)
	if err != nil {
		return autonomoustaskrun.Result{}, err
	}
	operationID := input.OperationID
	if operationID == "" {
		operationID = "task-run-" + id.New()
	}
	maxCycles := input.MaxCycles
	if maxCycles == 0 {
		maxCycles = 50
	}
	max := autonomoustaskrun.Limited(maxCycles)
	if input.Unlimited {
		max = autonomoustaskrun.Unlimited()
	}
	clock := input.Clock
	if clock == nil {
		clock = time.Now
	}
	redactor, _, err := redact.New(effective.SafetyDeclaration.Redaction, os.LookupEnv)
	if err != nil {
		return autonomoustaskrun.Result{}, err
	}
	taskID := input.TaskID
	var archiveEvidence []autonomousscheduler.ArchiveEvidence
	restarting := false
	if existing, found, inspectErr := autonomoustaskrun.Inspect(cfg.WorkDir, operationID); inspectErr != nil {
		return autonomoustaskrun.Result{}, inspectErr
	} else if found {
		restarting = true
		taskID = existing.TaskID
	}
	if !restarting {
		archiveEvidence, err = verifiedSchedulingArchives(ctx, cfg.WorkDir, effective)
		if err != nil {
			return autonomoustaskrun.Result{}, err
		}
	}
	if !restarting {
		active, loadErr := autonomousscheduler.LoadActiveStrict(ctx, cfg.WorkDir)
		if loadErr != nil {
			return autonomoustaskrun.Result{}, loadErr
		}
		graph, buildErr := autonomousscheduler.BuildSnapshot(active, archiveEvidence)
		if buildErr != nil {
			return autonomoustaskrun.Result{}, buildErr
		}
		if taskID == "" {
			if selected := autonomousscheduler.SelectNextReady(graph, nil); selected.Found {
				taskID = selected.Task.ID
			}
		} else {
			node, classifyErr := autonomousscheduler.ClassifyTask(graph, taskID, nil)
			if classifyErr != nil {
				return autonomoustaskrun.Result{}, classifyErr
			}
			if node.Reason != taskscheduler.ReasonReady {
				return autonomoustaskrun.Result{}, fmt.Errorf("autonomous task %q is not ready: %s (dependencies=%v conflicts=%v)", taskID, node.Reason, node.WaitingOn, node.Conflicts)
			}
		}
	}
	step := input.Runner
	var closeLedger func() error
	var loopLedger autonomoustaskrun.Ledger
	if step == nil && taskID != "" {
		paths, pathErr := resolveStatePaths(cfg.WorkDir)
		if pathErr != nil {
			return autonomoustaskrun.Result{}, pathErr
		}
		store, openErr := ledger.OpenWithClock(ctx, paths.LedgerDBPath, clock)
		if openErr != nil {
			return autonomoustaskrun.Result{}, openErr
		}
		closeLedger = store.Close
		loopLedger = store
		workspace, stateStore, prepErr := prepareTaskWorkspace(ctx, paths.WorkDir, taskID, operationID, effective, clock)
		if prepErr != nil {
			_ = closeLedger()
			return autonomoustaskrun.Result{}, prepErr
		}
		version, versionErr := codexexec.DiscoverVersion(ctx, codexexec.VersionConfig{Executable: effective.CodexExecutable, WorkingDir: workspace.ExecutionRoot, Timeout: effective.GitTimeout, StdoutCap: effective.CodexStdoutCap, StderrCap: effective.CodexStderrCap, CommandRunner: codexexec.CommandRunner(commandRunner(effective))})
		if versionErr != nil {
			_ = closeLedger()
			return autonomoustaskrun.Result{}, versionErr
		}
		step = productionStepRunner(productionStepConfig{root: paths.WorkDir, taskID: taskID, operationID: operationID, run: effective, configSchema: fingerprint.Schema, configSHA: fingerprint.SHA256, codexVersion: version, workspace: workspace, stateStore: stateStore, ledger: store, ledgerPath: paths.LedgerDBPath, redactor: redactor, clock: clock})
	}
	if closeLedger != nil {
		defer closeLedger()
	}
	return autonomoustaskrun.RunTaskUntilTerminal(ctx, autonomoustaskrun.Config{RepositoryRoot: cfg.WorkDir, OperationID: operationID, TaskID: taskID, ConfigSHA256: fingerprint.SHA256, MaxCycles: max, Clock: clock, Runner: step, Progress: input.Progress, Redact: redactor.String, Ledger: loopLedger, ArchiveEvidence: archiveEvidence})
}

type QueueInput struct {
	OperationID           string
	MaxTasks              int64
	MaxCycles             int64
	MaximumWorkers        int
	Mode                  autonomousqueue.Mode
	Sweep                 int64
	Clock                 func() time.Time
	Progress              autonomousqueue.Progress
	RunConfig             *runonce.Config
	TaskRunner            autonomousqueue.TaskRunner
	DaemonWakeCount       int64
	DaemonWakeFingerprint string
	Notification          NotificationObserver
	NotificationRuntime   NotificationRuntime
}

func RunQueue(ctx context.Context, cfg Config, input QueueInput) (autonomousqueue.Result, error) {
	unlock, err := autonomousexec.Acquire(ctx, cfg.WorkDir)
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	result, runErr := runQueue(ctx, cfg, input)
	unlock()
	dispatchQueueOutcome(ctx, cfg.WorkDir, result, input.NotificationRuntime, input.Notification)
	return result, runErr
}

func runQueue(ctx context.Context, cfg Config, input QueueInput) (autonomousqueue.Result, error) {
	runCfg, err := LoadRunOnceConfig(cfg.WorkDir, DefaultRunOnceConfig(cfg.WorkDir))
	if input.RunConfig != nil {
		runCfg = *input.RunConfig
		runCfg.WorkingDir = cfg.WorkDir
		err = nil
	}
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	if input.MaximumWorkers != 0 {
		runCfg.QueuePolicy.MaximumWorkers = input.MaximumWorkers
	}
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	fingerprint, err := runonce.FingerprintEffectiveConfig(effective)
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	redactor, _, err := redact.New(effective.SafetyDeclaration.Redaction, os.LookupEnv)
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	operationID := input.OperationID
	if operationID == "" {
		operationID = "queue-" + id.New()
	}
	maxTasks := input.MaxTasks
	if maxTasks == 0 {
		maxTasks = 100
	}
	maxCycles := input.MaxCycles
	if maxCycles == 0 {
		maxCycles = 50
	}
	mode := input.Mode
	if mode == "" {
		mode = autonomousqueue.ModeUntilExhausted
	}
	sweep := input.Sweep
	if sweep == 0 {
		sweep = 1
	}
	clock := input.Clock
	if clock == nil {
		clock = time.Now
	}
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	queueLedger, err := ledger.OpenWithClock(ctx, paths.LedgerDBPath, clock)
	if err != nil {
		return autonomousqueue.Result{}, err
	}
	defer queueLedger.Close()
	loader := func(loadCtx context.Context) (autonomousqueue.Snapshot, error) {
		return loadQueueSnapshot(loadCtx, cfg.WorkDir, effective)
	}
	taskRunner := input.TaskRunner
	if taskRunner == nil {
		taskRunner = func(runCtx context.Context, task autonomousqueue.RunTaskInput) (autonomoustaskrun.Result, error) {
			return runTaskUntilTerminal(runCtx, cfg, TaskRunInput{OperationID: task.OperationID, TaskID: task.TaskID, MaxCycles: maxCycles, Clock: clock, RunConfig: &effective})
		}
	}
	return autonomousqueue.RunUntilExhausted(ctx, autonomousqueue.Config{RepositoryRoot: cfg.WorkDir, OperationID: operationID, Mode: mode, ConfigSchema: fingerprint.Schema, ConfigSHA256: fingerprint.SHA256, SafetyIdentity: safetyDeclarationIdentity(effective), MaxTasks: maxTasks, MaximumWorkers: effective.QueuePolicy.MaximumWorkers, Sweep: sweep, DaemonWakeCount: input.DaemonWakeCount, DaemonWakeFingerprint: input.DaemonWakeFingerprint, Clock: clock, Loader: loader, Runner: taskRunner, Progress: input.Progress, Redact: redactor.String, Ledger: queueLedger})
}

type DaemonInput struct {
	OperationID         string
	MaxTasks            int64
	MaxCycles           int64
	MaximumWorkers      int
	MaxSweeps           int64
	Poll                time.Duration
	Debounce            time.Duration
	Clock               func() time.Time
	Wait                autonomousdaemon.Wait
	RunConfig           *runonce.Config
	Notification        NotificationObserver
	NotificationRuntime NotificationRuntime
}

func RunDaemon(ctx context.Context, cfg Config, input DaemonInput) (autonomousdaemon.Result, error) {
	runCfg, err := LoadRunOnceConfig(cfg.WorkDir, DefaultRunOnceConfig(cfg.WorkDir))
	if input.RunConfig != nil {
		runCfg = *input.RunConfig
		runCfg.WorkingDir = cfg.WorkDir
		err = nil
	}
	if err != nil {
		return autonomousdaemon.Result{}, err
	}
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		return autonomousdaemon.Result{}, err
	}
	baseID := input.OperationID
	if baseID == "" {
		baseID = "daemon-" + id.New()
	}
	maxSweeps := input.MaxSweeps
	if maxSweeps == 0 {
		maxSweeps = 1000
	}
	poll := input.Poll
	if poll == 0 {
		poll = time.Second
	}
	debounce := input.Debounce
	if debounce == 0 {
		debounce = 500 * time.Millisecond
	}
	var lastWake autonomousdaemon.Wake
	var wakeCount int64
	result, daemonErr := autonomousdaemon.RunDaemon(ctx, autonomousdaemon.Config{
		FullyUnattended: effective.SafetyDeclaration.Mode == autonomoussafety.ModeFullyUnattended,
		PollInterval:    poll,
		Debounce:        debounce,
		MaxSweeps:       maxSweeps,
		Wait:            input.Wait,
		OnWake: func(wake autonomousdaemon.Wake) {
			lastWake = wake
			wakeCount++
		},
		Fingerprint: func(fpCtx context.Context) (string, error) {
			snapshot, loadErr := loadQueueSnapshot(fpCtx, cfg.WorkDir, effective)
			return snapshot.Fingerprint, loadErr
		},
		Sweep: func(sweepCtx context.Context, generation int64) (autonomousqueue.Result, error) {
			sweepID := "queue-" + hashText("autonomous-daemon-sweep-v1", baseID, fmt.Sprint(generation))[:24]
			return RunQueue(sweepCtx, cfg, QueueInput{OperationID: sweepID, MaxTasks: input.MaxTasks, MaxCycles: input.MaxCycles, MaximumWorkers: input.MaximumWorkers, Mode: autonomousqueue.ModeDaemon, Sweep: generation, DaemonWakeCount: wakeCount, DaemonWakeFingerprint: lastWake.Fingerprint, Clock: input.Clock, RunConfig: &effective, Notification: input.Notification, NotificationRuntime: input.NotificationRuntime})
		},
	})
	dispatchDaemonFailure(ctx, cfg.WorkDir, baseID, result, daemonErr, input.NotificationRuntime, input.Notification)
	return result, daemonErr
}

func loadQueueSnapshot(ctx context.Context, root string, cfg runonce.Config) (autonomousqueue.Snapshot, error) {
	archives, err := verifiedSchedulingArchives(ctx, root, cfg)
	if err != nil {
		return autonomousqueue.Snapshot{}, err
	}
	active, err := autonomousscheduler.LoadActiveStrict(ctx, root)
	if err != nil {
		return autonomousqueue.Snapshot{}, err
	}
	graph, err := autonomousscheduler.BuildSnapshot(active, archives)
	if err != nil {
		return autonomousqueue.Snapshot{}, err
	}
	nodes := autonomousscheduler.ClassifyAll(graph, nil)
	type authorityNode struct {
		TaskID, TaskSHA256, StateSHA256, Lifecycle, Status, Reason string
		StateByteSize                                              int
		WaitingOn, Conflicts                                       []string
	}
	projection := make([]authorityNode, len(nodes))
	for i, node := range nodes {
		projection[i] = authorityNode{TaskID: node.Task.ID, TaskSHA256: node.Task.SourceSHA256(), StateSHA256: node.StateSHA256, StateByteSize: node.StateByteSize, Lifecycle: node.Lifecycle, Status: node.Task.Status, Reason: string(node.Reason), WaitingOn: append([]string(nil), node.WaitingOn...), Conflicts: append([]string(nil), node.Conflicts...)}
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return autonomousqueue.Snapshot{}, err
	}
	return autonomousqueue.Snapshot{Fingerprint: hashText(string(raw)), Nodes: nodes, Classify: func(occupied []string) ([]autonomousscheduler.Node, error) {
		return autonomousscheduler.ClassifyAll(graph, occupied), nil
	}}, nil
}

func safetyDeclarationIdentity(cfg runonce.Config) string {
	raw, _ := json.Marshal(cfg.SafetyDeclaration)
	return hashText("autonomous-safety-declaration-v1", string(raw))
}

func hashText(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func verifiedSchedulingArchives(ctx context.Context, root string, cfg runonce.Config) ([]autonomousscheduler.ArchiveEvidence, error) {
	entries, err := autonomousarchive.List(root)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	paths, err := resolveStatePaths(root)
	if err != nil {
		return nil, err
	}
	store, err := ledger.OpenLiveReadOnly(ctx, paths.LedgerDBPath)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	result := make([]autonomousscheduler.ArchiveEvidence, 0, len(entries))
	for _, entry := range entries {
		report, verifyErr := autonomousarchive.Verify(ctx, autonomousarchive.VerifyConfig{RepositoryRoot: root, Ledger: store, GitExecutable: cfg.GitExecutable, GitTimeout: cfg.GitTimeout, ForbiddenValues: archiveSecretValues(cfg)}, entry.Manifest.ArchiveID)
		if verifyErr != nil {
			return nil, verifyErr
		}
		result = append(result, autonomousscheduler.ArchiveEvidence{
			TaskID:      entry.Manifest.TaskID,
			ArchiveID:   entry.Manifest.ArchiveID,
			Disposition: string(entry.Manifest.Disposition),
			Reason:      entry.Manifest.Reason,
			Verified:    report.Passed,
			Reconciled:  report.Passed,
		})
	}
	return result, nil
}

func commandRunner(cfg runonce.Config) func(context.Context, runner.Command) runner.Result {
	if cfg.CommandRunner != nil {
		return func(ctx context.Context, c runner.Command) runner.Result { return cfg.CommandRunner(ctx, c) }
	}
	return runner.Run
}

func prepareTaskWorkspace(ctx context.Context, root, taskID, operationID string, cfg runonce.Config, clock func() time.Time) (autonomous.TaskWorkspace, *autonomousstate.Store, error) {
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		return autonomous.TaskWorkspace{}, nil, err
	}
	snap, found, err := store.Load(ctx, taskID)
	if err != nil || !found {
		return autonomous.TaskWorkspace{}, nil, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	wcfg := autonomousworkspace.Config{ControlRoot: root, TaskID: taskID, OperationID: operationID + "-workspace", GitExecutable: cfg.GitExecutable, Timeout: cfg.GitTimeout, StdoutCap: cfg.GitStdoutCap, StderrCap: cfg.GitStderrCap, Clock: clock, CommandRunner: autonomousworkspace.CommandRunner(commandRunner(cfg))}
	if snap.State.Workspace != nil {
		wcfg.BaselineSHA = snap.State.Workspace.BaselineSHA
		if _, err := autonomousworkspace.Reopen(ctx, wcfg, *snap.State.Workspace); err != nil {
			return autonomous.TaskWorkspace{}, nil, err
		}
		return *snap.State.Workspace, store, nil
	}
	source, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{WorkingDir: root, GitExecutable: cfg.GitExecutable, Timeout: cfg.GitTimeout, StdoutCap: cfg.GitStdoutCap, StderrCap: cfg.GitStderrCap, AllowHarnessRuntime: true, CommandRunner: gitstate.CommandRunner(commandRunner(cfg))})
	if err != nil {
		return autonomous.TaskWorkspace{}, nil, err
	}
	wcfg.BaselineSHA = source.Head
	prepared, err := autonomousworkspace.Prepare(ctx, wcfg)
	if err != nil {
		return autonomous.TaskWorkspace{}, nil, err
	}
	applied, err := autonomousworkspace.Apply(ctx, autonomousworkspace.ApplyConfig{TaskID: taskID, OperationID: operationID + "-workspace-state", Kind: autonomousstate.WorkspaceTransitionCreated, Expected: snap.Expected(), PreviousState: snap.State, Workspace: prepared.Workspace, CreatedAt: clock().UTC(), Store: store})
	if err != nil {
		return autonomous.TaskWorkspace{}, nil, err
	}
	return *applied.Current.State.Workspace, store, nil
}

type productionStepConfig struct {
	root, taskID, operationID, configSchema, configSHA, codexVersion string
	run                                                              runonce.Config
	workspace                                                        autonomous.TaskWorkspace
	stateStore                                                       *autonomousstate.Store
	ledger                                                           *ledger.Store
	ledgerPath                                                       string
	redactor                                                         *redact.Redactor
	clock                                                            func() time.Time
}

func productionStepRunner(p productionStepConfig) autonomoustaskrun.StepRunner {
	return func(ctx context.Context, in autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
		snapshot, found, err := p.stateStore.Load(ctx, p.taskID)
		if err != nil || !found {
			return autonomoustaskrun.StepResult{}, errors.Join(err, autonomousstate.ErrStateMissing)
		}
		workspace := *snapshot.State.Workspace
		cycleCfg := autonomouscycle.Config{RepositoryRoot: p.root, Workspace: &workspace, TaskID: p.taskID, State: snapshot.State, SafetyDeclaration: p.run.SafetyDeclaration, SourceSafety: autonomouspolicy.SourceSafetySafe, LatestMutation: in.Operation.LatestMutation, Verification: in.Operation.Verification, Audit: in.Operation.Audit, LedgerPath: p.run.LedgerPath, Ledger: p.ledger, CodexExecutable: p.run.CodexExecutable, CodexModel: p.run.CodexModel, CodexReasoningEffort: p.run.CodexReasoningEffort, CodexEphemeral: p.run.CodexEphemeral, CodexSandbox: p.run.CodexSandbox, CodexApprovalPolicy: p.run.CodexApprovalPolicy, CodexBypassApprovalsAndSandbox: p.run.CodexBypassApprovalsAndSandbox, CodexVersion: p.codexVersion, EffectiveConfigSchema: p.configSchema, EffectiveConfigSHA256: p.configSHA, CodexTimeout: p.run.CodexTimeout, CodexStdoutCap: p.run.CodexStdoutCap, CodexStderrCap: p.run.CodexStderrCap, GitExecutable: p.run.GitExecutable, GitTimeout: p.run.GitTimeout, GitStdoutCap: p.run.GitStdoutCap, GitStderrCap: p.run.GitStderrCap, VerificationCommands: p.run.VerificationCommands, VerificationPlan: p.run.VerificationPlan, MissingVerificationPolicy: p.run.MissingVerificationPolicy, VerificationTimeout: p.run.VerificationTimeout, VerificationStdoutCap: p.run.VerificationStdoutCap, VerificationStderrCap: p.run.VerificationStderrCap, AllowPreExistingDirty: false, CommitTimeout: p.run.CommitTimeout, CommitStdoutCap: p.run.CommitStdoutCap, CommitStderrCap: p.run.CommitStderrCap, SourceWriterLockTimeout: p.run.SourceWriterLockTimeout, SourceWriterLockHeartbeatInterval: p.run.SourceWriterLockHeartbeatInterval, SourceWriterLockPID: p.run.SourceWriterLockPID, IDGenerator: id.New, Clock: p.clock, CommandRunner: autonomouscycle.CommandRunner(commandRunner(p.run))}
		attemptID := "attempt-" + id.New()
		admissionOp := p.operationID + "-admit-" + id.New()
		completionOp := p.operationID + "-complete-" + id.New()
		limits := defaultAttemptLimits()
		admission := autonomousattempt.AdmissionConfig{OperationID: admissionOp, AttemptID: attemptID, Expected: snapshot.Expected(), Limits: limits, Store: p.stateStore}
		admitted := false
		var admissionResult autonomousattempt.Result
		var admissionReason autonomous.BreakerReason
		cycleCfg.BeforeWorker = func(admitCtx context.Context, input autonomouscycle.WorkerAdmissionInput) error {
			strategy := autonomousattempt.Strategy{Approach: "Execute the exact fresh supervisor decision.", Techniques: []string{string(input.Decision.Action)}, Targets: input.Decision.Inputs}
			if input.Decision.Strategy != nil {
				strategy = autonomousattempt.Strategy{Approach: input.Decision.Strategy.Approach, Techniques: input.Decision.Strategy.Techniques, Targets: input.Decision.Strategy.Targets}
			}
			admission.TaskID = input.TaskID
			admission.Action = input.Decision.Action
			admission.Decision = input.Decision
			admission.Reference = input.Reference
			admission.Strategy = strategy
			admission.SourceRevision = input.SourceRevision
			admission.SourceSafety = autonomouspolicy.SourceSafetySafe
			admission.CreatedAt = p.clock().UTC()
			res, admitErr := autonomousattempt.Admit(admitCtx, admission)
			admissionResult = res
			admissionReason = res.Reason
			if admitErr != nil {
				return admitErr
			}
			if res.Disposition == autonomousattempt.DispositionBlocked {
				admissionReason = res.Reason
				return errors.New("attempt budget exhausted")
			}
			admitted = true
			return nil
		}
		started := p.clock().UTC()
		cycle, cycleErr := autonomouscycle.Run(ctx, cycleCfg)
		step := autonomoustaskrun.StepResult{RunID: cycle.Supervisor.RunID, LatestMutation: in.Operation.LatestMutation, Verification: in.Operation.Verification, Audit: in.Operation.Audit}
		if cycle.Supervisor.Decision != nil {
			step.Action = string(cycle.Supervisor.Decision.Action)
		}
		if cycle.Supervisor.DecisionReference != nil {
			step.DecisionID = cycle.Supervisor.DecisionReference.DecisionID
		}
		if cycle.Worker.Started {
			step.RunID = cycle.Worker.RunID
		}
		if admissionReason != "" {
			step.StopReason = stopForBreaker(admissionReason)
			step.StopDetail = string(admissionReason)
			return step, nil
		}
		coordinated := cycle.Route != nil && cycle.Route.Kind == autonomouspolicy.RouteKindWorker && (cycle.Route.Action == autonomous.ActionCorrect || cycle.Route.Action == autonomous.ActionDocument || cycle.Route.Action == autonomous.ActionSimplify)
		if cycle.Route != nil && cycle.Route.Kind == autonomouspolicy.RouteKindWorker && admitted && !coordinated {
			obs := autonomousattempt.ObserveCycle(cycle, cycleErr)
			current, ok, loadErr := p.stateStore.Load(context.WithoutCancel(ctx), p.taskID)
			if loadErr != nil || !ok {
				return step, errors.Join(loadErr, autonomousstate.ErrStateMissing)
			}
			completed, completeErr := autonomousattempt.Complete(context.WithoutCancel(ctx), autonomousattempt.CompletionConfig{TaskID: p.taskID, OperationID: completionOp, AttemptID: attemptID, Expected: current.Expected(), RunID: obs.RunID, OccurrenceID: obs.OccurrenceID, SourceAfter: obs.SourceAfter, Outcome: obs.Outcome, Duration: p.clock().UTC().Sub(started), Tokens: obs.Tokens, Evidence: obs.Evidence, Signatures: obs.Signatures, StopReason: obs.StopReason, CreatedAt: p.clock().UTC(), Store: p.stateStore})
			if completeErr != nil {
				return step, completeErr
			}
			step.Statistics.AttemptsAdmitted = 1
			step.Statistics.AttemptsCompleted = 1
			step.Statistics.Actions = []autonomoustaskrun.ActionCount{{Action: step.Action, Count: 1}}
			snapshot = completed.Current
		}
		if cycleErr != nil {
			return classifyCycleStop(step, cycle), cycleErr
		}
		if cycle.Supervisor.Decision != nil && cycle.Supervisor.Decision.ChildTasks != nil {
			parentTask, found, loadErr := taskfile.FindByID(p.root, p.taskID)
			if loadErr != nil || !found {
				return step, errors.Join(loadErr, errors.New("child publication parent task missing"))
			}
			archives, archiveErr := verifiedSchedulingArchives(context.WithoutCancel(ctx), p.root, p.run)
			if archiveErr != nil {
				return step, archiveErr
			}
			_, childErr := autonomouschild.Apply(context.WithoutCancel(ctx), autonomouschild.Input{RepositoryRoot: p.root, OperationID: p.operationID + "-children-" + cycle.Supervisor.DecisionReference.DecisionID, Decision: *cycle.Supervisor.Decision, Reference: *cycle.Supervisor.DecisionReference, ExpectedParentTaskSHA256: parentTask.SourceSHA256(), ExpectedParentStateSHA256: snapshot.SHA256, ArchiveEvidence: archives, ForbiddenValues: archiveSecretValues(p.run), Ledger: p.ledger, CreatedAt: cycle.Supervisor.DecisionReference.CreatedAt})
			if childErr != nil {
				return step, childErr
			}
		}
		if coordinated && admitted {
			switch cycle.Route.Action {
			case autonomous.ActionCorrect:
				return finishCorrectionStep(ctx, p, in, snapshot, workspace, cycleCfg, cycle, admission, admissionResult, completionOp, attemptID, started, step)
			case autonomous.ActionDocument, autonomous.ActionSimplify:
				return finishOptionalStep(ctx, p, in, snapshot, workspace, cycleCfg, cycle, admission, admissionResult, completionOp, attemptID, started, step)
			}
		}
		switch cycle.Outcome {
		case autonomouscycle.OutcomeReadOnlyCompleted:
			if cycle.Route.Action == autonomous.ActionPlan {
				_, applyErr := autonomousplanapply.ApplyPlanningResult(context.WithoutCancel(ctx), autonomousplanapply.Config{RepositoryRoot: p.root, TaskID: p.taskID, OperationID: p.operationID + "-plan-" + id.New(), Expected: snapshot.Expected(), Cycle: cycle, CreatedAt: p.clock().UTC(), Store: p.stateStore})
				if applyErr != nil {
					return step, applyErr
				}
			}
			if cycle.Route.Action == autonomous.ActionAudit {
				if step.Verification == nil {
					return step, errors.New("audit route has no current verification evidence")
				}
				applied, applyErr := autonomousauditapply.ApplyAuditResult(context.WithoutCancel(ctx), autonomousauditapply.ApplyConfig{RepositoryRoot: p.root, TaskID: p.taskID, OperationID: p.operationID + "-audit-" + id.New(), Expected: snapshot.Expected(), Cycle: cycle, Verification: *step.Verification, LatestMutation: step.LatestMutation, CreatedAt: p.clock().UTC(), Store: p.stateStore})
				if applyErr != nil {
					return step, applyErr
				}
				step.Audit = &applied.PolicyEvidence
				step.Statistics.Audits++
			}
		case autonomouscycle.OutcomeVerifiedChangesCommitted:
			if cycle.Worker.Verification.Policy != nil {
				v := *cycle.Worker.Verification.Policy
				step.Verification = &v
			}
			step.LatestMutation = &autonomouspolicy.SourceMutation{TaskID: p.taskID, RunID: cycle.Worker.RunID, DecisionID: step.DecisionID, Action: cycle.Route.Action, ResultingRevision: cycle.Source.FinalRevision}
			step.Statistics.VerificationRuns++
			step.Statistics.SourceCommits++
			advanced, advanceErr := advanceWorkspace(context.WithoutCancel(ctx), p, workspace, cycle)
			if advanceErr != nil {
				return step, advanceErr
			}
			p.workspace = advanced
			step.Statistics.CheckpointAdvances++
		case autonomouscycle.OutcomeNeedsInputAuthorized:
			recorded, recordErr := autonomousinput.RecordQuestion(context.WithoutCancel(ctx), autonomousinput.QuestionRequest{RepositoryRoot: p.root, TaskID: p.taskID, OperationID: p.operationID + "-input-" + id.New(), Expected: snapshot.Expected(), Decision: *cycle.Supervisor.Decision, Reference: *cycle.Supervisor.DecisionReference, SourceRevision: cycle.Source.FinalRevision, SourceSafety: autonomouspolicy.SourceSafetySafe, RecordedAt: p.clock().UTC()})
			if recordErr != nil {
				return step, recordErr
			}
			step.StopReason = autonomoustaskrun.StopNeedsInput
			step.StopDetail = string(recorded.Yield.Reason)
		case autonomouscycle.OutcomeBlockAuthorized:
			blocked, blockErr := autonomousblock.Apply(context.WithoutCancel(ctx), autonomousblock.Config{RepositoryRoot: p.root, TaskID: p.taskID, OperationID: p.operationID + "-block-" + id.New(), Expected: snapshot.Expected(), Decision: *cycle.Supervisor.Decision, Reference: *cycle.Supervisor.DecisionReference, Source: autonomouspolicy.SourceEvidence{Revision: cycle.Source.FinalRevision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: step.LatestMutation}, Verification: step.Verification, Audit: step.Audit, CreatedAt: p.clock().UTC(), Store: p.stateStore})
			if blockErr != nil {
				return step, blockErr
			}
			step.StopReason = autonomoustaskrun.StopBlocked
			step.StopDetail = blocked.Current.State.Terminal.Reason
			step.Evidence = append(step.Evidence, blocked.History.SourcePath)
		case autonomouscycle.OutcomeCompleteAuthorized:
			frozen, freezeErr := buildFrozenEvidence(context.WithoutCancel(ctx), p, snapshot, cycle, step)
			if freezeErr != nil {
				return step, freezeErr
			}
			finalized, finalErr := autonomousfinalization.Finalize(context.WithoutCancel(ctx), autonomousfinalization.Config{RepositoryRoot: p.root, Evidence: frozen, StateStore: p.stateStore, Ledger: p.ledger, Redactor: p.redactor, RevalidateEvidence: func(checkCtx context.Context, e autonomousfinalization.FrozenEvidence) error {
				return revalidateFrozen(checkCtx, p, e)
			}})
			if finalErr != nil {
				return step, finalErr
			}
			step.StopReason = autonomoustaskrun.StopCompleted
			step.StopDetail = "AW-20 finalization reached ledger completion"
			step.Evidence = append(step.Evidence, finalized.Manifest.Path)
			return step, nil
		case autonomouscycle.OutcomeSafetyPreflightFailed, autonomouscycle.OutcomeSourceChanged, autonomouscycle.OutcomeSourceChangedDuringDossier, autonomouscycle.OutcomeReadOnlyMutation:
			step.StopReason = autonomoustaskrun.StopSafety
			step.StopDetail = cycleFailure(cycle)
		default:
			if cycle.Outcome != autonomouscycle.OutcomeWorkerNoChanges {
				step.StopReason = autonomoustaskrun.StopUnsafeAmbiguous
				step.StopDetail = cycleFailure(cycle)
			}
		}
		return step, nil
	}
}

func defaultAttemptLimits() autonomousattempt.Limits {
	actions := []autonomous.Action{autonomous.ActionPlan, autonomous.ActionImplement, autonomous.ActionAudit, autonomous.ActionCorrect, autonomous.ActionDocument, autonomous.ActionSimplify}
	budgets := make([]autonomous.ActionBudget, 0, len(actions))
	for _, a := range actions {
		budgets = append(budgets, autonomous.ActionBudget{Action: a, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4}})
	}
	return autonomousattempt.Limits{TaskAttempts: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 16}, ActionAttempts: budgets, Elapsed: autonomous.DurationBudget{Mode: autonomous.BudgetModeLimited, Limit: 4 * time.Hour}, Tokens: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}, RepeatedSignatureLimit: 3}
}

func finishCorrectionStep(ctx context.Context, p productionStepConfig, in autonomoustaskrun.StepInput, _ autonomousstate.Snapshot, workspace autonomous.TaskWorkspace, cycleCfg autonomouscycle.Config, cycle autonomouscycle.Result, admissionCfg autonomousattempt.AdmissionConfig, admission autonomousattempt.Result, completionOp, attemptID string, started time.Time, step autonomoustaskrun.StepResult) (autonomoustaskrun.StepResult, error) {
	current, found, err := p.stateStore.Load(context.WithoutCancel(ctx), p.taskID)
	if err != nil || !found {
		return step, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	authority := autonomouscorrection.Authority{Kind: autonomouscorrection.AuthorityAudit, FindingIDs: append([]string(nil), cycle.Supervisor.Decision.FindingIDs...)}
	if cycle.Supervisor.Decision.VerificationFailure != nil {
		if in.Operation.Verification == nil {
			return step, errors.New("correction decision cites a verification failure without durable current verification evidence")
		}
		authority = autonomouscorrection.Authority{Kind: autonomouscorrection.AuthorityVerification, Verification: in.Operation.Verification}
	}
	plan := autonomousverification.AdaptLegacy(p.run.VerificationCommands)
	if p.run.VerificationPlan != nil {
		plan = autonomousverification.ClonePlan(*p.run.VerificationPlan)
	}
	correctionCfg := cycleCfg
	correctionCfg.BeforeWorker = nil
	preserved := cycle.Source.Admission
	if preserved == nil {
		return step, errors.New("correction cycle is missing its exact admission snapshot")
	}
	baseSnapshotter := correctionCfg.SourceSnapshotter
	if baseSnapshotter == nil {
		baseSnapshotter = gitstate.CaptureSourceSnapshot
	}
	usedPreserved := false
	correctionCfg.SourceSnapshotter = func(checkCtx context.Context, cfg gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error) {
		if !usedPreserved {
			usedPreserved = true
			return *preserved, nil
		}
		return baseSnapshotter(checkCtx, cfg)
	}
	auditCfg := cycleCfg
	auditCfg.BeforeWorker = nil
	auditCfg.CorrectionFailure = nil
	cycleCalls := 0
	coordinated, correctionErr := autonomouscorrection.Run(ctx, autonomouscorrection.Config{
		RepositoryRoot: p.root, Workspace: &workspace, TaskID: p.taskID, Expected: current.Expected(), Authority: authority, Store: p.stateStore,
		CorrectionCycle: correctionCfg, AuditCycle: auditCfg, FinalPlan: plan,
		FinalTimeout: p.run.VerificationTimeout, FinalStdoutCap: p.run.VerificationStdoutCap, FinalStderrCap: p.run.VerificationStderrCap,
		IDGenerator: id.New, Clock: p.clock,
		CycleRunner: func(runCtx context.Context, cfg autonomouscycle.Config) (autonomouscycle.Result, error) {
			cycleCalls++
			if cycleCalls == 1 {
				return cycle, nil
			}
			return autonomouscycle.RunInWorkspace(runCtx, cfg)
		},
	})
	observation := autonomousattempt.ObserveCorrection(coordinated, correctionErr)
	completed, completeErr := completeCoordinatedAttempt(ctx, p, admissionCfg, admission, completionOp, attemptID, started, observation)
	if completeErr != nil {
		return step, completeErr
	}
	step.Statistics.AttemptsAdmitted, step.Statistics.AttemptsCompleted = 1, 1
	step.Statistics.Actions = []autonomoustaskrun.ActionCount{{Action: string(autonomous.ActionCorrect), Count: 1}}
	step.Statistics.Corrections = 1
	if completed.Reason != "" {
		step.StopReason, step.StopDetail = stopForBreaker(completed.Reason), string(completed.Reason)
		return step, nil
	}
	if correctionErr != nil {
		if errors.Is(correctionErr, context.Canceled) || ctx.Err() != nil {
			step.StopReason, step.StopDetail = autonomoustaskrun.StopOperationCancelled, correctionErr.Error()
		} else if coordinated.Outcome == autonomouscorrection.OutcomeSafetyStopped {
			step.StopReason, step.StopDetail = autonomoustaskrun.StopSafety, correctionErr.Error()
		} else {
			step.Evidence = append(step.Evidence, "correction:"+string(coordinated.Outcome))
		}
		return step, nil
	}
	if coordinated.Outcome != autonomouscorrection.OutcomeReturnedToSupervisor {
		return step, fmt.Errorf("correction coordinator returned unsupported outcome %q", coordinated.Outcome)
	}
	verification := coordinated.AuditApplication.History.Record.Verification
	audit := coordinated.AuditApplication.PolicyEvidence
	step.Verification, step.Audit = &verification, &audit
	step.LatestMutation = &autonomouspolicy.SourceMutation{TaskID: p.taskID, RunID: cycle.Worker.RunID, DecisionID: cycle.Route.DecisionID, Action: autonomous.ActionCorrect, ResultingRevision: cycle.Source.FinalRevision}
	step.Statistics.VerificationRuns = 2
	step.Statistics.Audits = 1
	step.Statistics.SourceCommits = 1
	advanced, err := advanceWorkspace(context.WithoutCancel(ctx), p, workspace, cycle)
	if err != nil {
		return step, err
	}
	p.workspace = advanced
	step.Statistics.CheckpointAdvances = 1
	return step, nil
}

func finishOptionalStep(ctx context.Context, p productionStepConfig, in autonomoustaskrun.StepInput, _ autonomousstate.Snapshot, workspace autonomous.TaskWorkspace, cycleCfg autonomouscycle.Config, cycle autonomouscycle.Result, admissionCfg autonomousattempt.AdmissionConfig, admission autonomousattempt.Result, completionOp, attemptID string, started time.Time, step autonomoustaskrun.StepResult) (autonomoustaskrun.StepResult, error) {
	current, found, err := p.stateStore.Load(context.WithoutCancel(ctx), p.taskID)
	if err != nil || !found {
		return step, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	assessment, err := buildOptionalAssessment(p, current, cycle, in.Operation.Verification, in.Operation.Audit)
	if err != nil {
		return step, err
	}
	roleCfg := cycleCfg
	roleCfg.BeforeWorker = nil
	auditCfg := cycleCfg
	auditCfg.BeforeWorker = nil
	optional, optionalErr := autonomousoptional.Continue(ctx, autonomousoptional.Config{
		RepositoryRoot: p.root, Workspace: &workspace, TaskID: p.taskID, Expected: current.Expected(), Assessment: assessment, Store: p.stateStore, Ledger: p.ledger,
		Admission: admissionCfg, CompletionOperationID: completionOp, DispositionOperationID: p.operationID + "-optional-" + id.New(), AuditOperationID: p.operationID + "-optional-audit-" + id.New(),
		RoleCycle: roleCfg, AuditCycle: auditCfg, Clock: p.clock,
	}, admission, cycle, started)
	step.Statistics.AttemptsAdmitted, step.Statistics.AttemptsCompleted = 1, 1
	step.Statistics.Actions = []autonomoustaskrun.ActionCount{{Action: string(cycle.Route.Action), Count: 1}}
	step.Statistics.OptionalRoles = 1
	if optional.Completion.Reason != "" {
		step.StopReason, step.StopDetail = stopForBreaker(optional.Completion.Reason), string(optional.Completion.Reason)
		return step, nil
	}
	if optionalErr != nil {
		if errors.Is(optionalErr, context.Canceled) || ctx.Err() != nil {
			step.StopReason, step.StopDetail = autonomoustaskrun.StopOperationCancelled, optionalErr.Error()
		} else {
			step.Evidence = append(step.Evidence, "optional-role:"+string(optional.Outcome))
		}
		return step, nil
	}
	if optional.Outcome == autonomousoptional.OutcomeSourceChanged {
		verification := optional.AuditApplication.History.Record.Verification
		audit := optional.AuditApplication.PolicyEvidence
		step.Verification, step.Audit = &verification, &audit
		step.LatestMutation = &autonomouspolicy.SourceMutation{TaskID: p.taskID, RunID: cycle.Worker.RunID, DecisionID: cycle.Route.DecisionID, Action: cycle.Route.Action, ResultingRevision: cycle.Source.FinalRevision}
		step.Statistics.VerificationRuns, step.Statistics.Audits, step.Statistics.SourceCommits = 1, 1, 1
		advanced, err := advanceWorkspace(context.WithoutCancel(ctx), p, workspace, cycle)
		if err != nil {
			return step, err
		}
		p.workspace = advanced
		step.Statistics.CheckpointAdvances = 1
	}
	return step, nil
}

func completeCoordinatedAttempt(ctx context.Context, p productionStepConfig, cfg autonomousattempt.AdmissionConfig, admission autonomousattempt.Result, completionOp, attemptID string, started time.Time, observation autonomousattempt.Observation) (autonomousattempt.Result, error) {
	current, found, err := p.stateStore.Load(context.WithoutCancel(ctx), p.taskID)
	if err != nil || !found {
		return autonomousattempt.Result{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if observation.RunID == "" {
		observation.RunID = "attempt-" + attemptID
	}
	if observation.SourceAfter == "" {
		observation.SourceAfter = cfg.SourceRevision
	}
	return autonomousattempt.Complete(context.WithoutCancel(ctx), autonomousattempt.CompletionConfig{TaskID: p.taskID, OperationID: completionOp, AttemptID: attemptID, Expected: current.Expected(), RunID: observation.RunID, OccurrenceID: observation.OccurrenceID, SourceAfter: observation.SourceAfter, Outcome: observation.Outcome, Duration: p.clock().UTC().Sub(started), Tokens: observation.Tokens, Evidence: observation.Evidence, Signatures: observation.Signatures, StopReason: observation.StopReason, CreatedAt: p.clock().UTC(), Store: p.stateStore})
}

func buildOptionalAssessment(p productionStepConfig, state autonomousstate.Snapshot, cycle autonomouscycle.Result, verification *autonomouspolicy.VerificationEvidence, audit *autonomouspolicy.AuditEvidence) (autonomous.OptionalRoleAssessment, error) {
	if verification == nil || audit == nil || cycle.Supervisor.Decision == nil || cycle.Supervisor.DecisionReference == nil || cycle.Route == nil {
		return autonomous.OptionalRoleAssessment{}, errors.New("optional-role route requires exact current verification, audit, and decision evidence")
	}
	task, found, err := taskfile.FindByID(p.root, p.taskID)
	if err != nil || !found {
		return autonomous.OptionalRoleAssessment{}, errors.Join(err, autonomousstate.ErrTaskMissing)
	}
	role := cycle.Route.WorkerProfile
	kind := autonomous.OptionalRoleEvidenceUserFacingChange
	if role == autonomous.WorkerProfileSimplifier {
		kind = autonomous.OptionalRoleEvidenceMaintainabilityTarget
	}
	var evidence []autonomous.OptionalRoleEvidence
	for _, input := range cycle.Supervisor.Decision.Inputs {
		target := cleanOptionalTarget(input.Reference)
		if target == "" {
			continue
		}
		idValue := fmt.Sprintf("target-%02d", len(evidence)+1)
		evidence = append(evidence, autonomous.OptionalRoleEvidence{ID: idValue, Role: role, Kind: kind, Reference: input, SourceRevision: cycle.Source.AdmissionRevision, TargetPath: target})
	}
	if len(evidence) == 0 {
		return autonomous.OptionalRoleAssessment{}, errors.New("optional-role decision has no exact repository-contained target evidence")
	}
	selected := make([]string, len(evidence))
	for i := range evidence {
		selected[i] = evidence[i].ID
	}
	assessment := autonomous.OptionalRoleAssessment{SchemaVersion: autonomous.OptionalRoleAssessmentSchemaVersion, TaskID: p.taskID, Role: role, Disposition: autonomous.OptionalRoleDispositionRun, Decision: *cycle.Supervisor.Decision, DecisionReference: *cycle.Supervisor.DecisionReference, TaskSource: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindTask, Reference: task.SourcePath, Detail: "Exact canonical task source for optional-role admission."}, StateSHA256: state.SHA256, SourceRevision: cycle.Source.AdmissionRevision, VerificationRunID: verification.Summary.RunID, VerificationID: verification.Summary.OccurrenceID, AuditRunID: audit.RunID, AuditSourceRevision: audit.SourceRevision, Evidence: evidence, SelectedEvidenceIDs: selected, Rationale: "Exact supervisor input artifacts identify the bounded optional-role targets."}
	return assessment, assessment.Validate()
}

func cleanOptionalTarget(value string) string {
	value = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(value))))
	if value == "." || value == ".." || filepath.IsAbs(filepath.FromSlash(value)) || strings.HasPrefix(value, "../") || strings.HasPrefix(value, ".agent/") || strings.HasPrefix(value, ".revolvr/") || strings.HasPrefix(value, ".git/") {
		return ""
	}
	return value
}
func classifyCycleStop(step autonomoustaskrun.StepResult, c autonomouscycle.Result) autonomoustaskrun.StepResult {
	if c.Outcome == autonomouscycle.OutcomeSafetyPreflightFailed || c.Outcome == autonomouscycle.OutcomeSourceChanged || c.Outcome == autonomouscycle.OutcomeSourceChangedDuringDossier {
		step.StopReason = autonomoustaskrun.StopSafety
	} else {
		step.StopReason = autonomoustaskrun.StopUnsafeAmbiguous
	}
	step.StopDetail = cycleFailure(c)
	return step
}
func stopForBreaker(reason autonomous.BreakerReason) autonomoustaskrun.StopReason {
	switch reason {
	case autonomous.BreakerTaskAttemptsExhausted, autonomous.BreakerActionAttemptsExhausted, autonomous.BreakerElapsedExhausted, autonomous.BreakerTokenExhausted:
		return autonomoustaskrun.StopBudgetExhausted
	case autonomous.BreakerRepeatedSignature, autonomous.BreakerUnchangedSource, autonomous.BreakerIdenticalStrategy:
		return autonomoustaskrun.StopNoProgress
	case autonomous.BreakerCancellation:
		return autonomoustaskrun.StopOperationCancelled
	default:
		return autonomoustaskrun.StopSafety
	}
}
func cycleFailure(c autonomouscycle.Result) string {
	if c.Failure != nil {
		return c.Failure.Stage + ": " + c.Failure.Reason
	}
	return string(c.Outcome)
}
func advanceWorkspace(ctx context.Context, p productionStepConfig, current autonomous.TaskWorkspace, cycle autonomouscycle.Result) (autonomous.TaskWorkspace, error) {
	cfg := autonomousworkspace.Config{ControlRoot: p.root, TaskID: p.taskID, OperationID: p.operationID + "-checkpoint-" + id.New(), BaselineSHA: current.BaselineSHA, GitExecutable: p.run.GitExecutable, Timeout: p.run.GitTimeout, StdoutCap: p.run.GitStdoutCap, StderrCap: p.run.GitStderrCap, Clock: p.clock, CommandRunner: autonomousworkspace.CommandRunner(commandRunner(p.run))}
	advanced, err := autonomousworkspace.AdvanceCheckpoint(ctx, cfg, current, "verified autonomous cycle "+cycle.Worker.RunID)
	if err != nil {
		return current, err
	}
	snap, ok, err := p.stateStore.Load(ctx, p.taskID)
	if err != nil || !ok {
		return current, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	applied, err := autonomousworkspace.Apply(ctx, autonomousworkspace.ApplyConfig{TaskID: p.taskID, OperationID: cfg.OperationID + "-state", Kind: autonomousstate.WorkspaceTransitionCheckpoint, Expected: snap.Expected(), PreviousState: snap.State, Workspace: advanced.Workspace, CreatedAt: p.clock().UTC(), Store: p.stateStore})
	if err != nil {
		return current, err
	}
	return *applied.Current.State.Workspace, nil
}

func buildFrozenEvidence(ctx context.Context, p productionStepConfig, snapshot autonomousstate.Snapshot, cycle autonomouscycle.Result, step autonomoustaskrun.StepResult) (autonomousfinalization.FrozenEvidence, error) {
	if step.Verification == nil || step.Audit == nil || cycle.Route == nil || cycle.SafetyPolicy == nil || cycle.SafetyPreflight == nil {
		return autonomousfinalization.FrozenEvidence{}, errors.New("complete decision is missing verification, audit, route, or safety authority")
	}
	task, found, err := taskfile.FindByID(p.root, p.taskID)
	if err != nil || !found {
		return autonomousfinalization.FrozenEvidence{}, errors.Join(err, autonomousstate.ErrTaskMissing)
	}
	projected, err := taskfile.ProjectMetadataFromSnapshot(p.root, task, taskfile.MetadataUpdate{Status: taskfile.StatusCompleted})
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, err
	}
	stateID, err := autonomousstate.StateIdentityFor(snapshot.SourcePath, true, snapshot.State)
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, err
	}
	history, err := p.ledger.ListRecentRunsForTaskWithEvents(ctx, p.taskID, 100)
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, err
	}
	runs := make([]autonomousfinalization.RunEvidence, 0, len(history))
	commits := []autonomousfinalization.CommitEvidence{}
	for i := len(history) - 1; i >= 0; i-- {
		r := history[i].Run
		if r.CompletedAt == nil {
			continue
		}
		runs = append(runs, autonomousfinalization.RunEvidence{Sequence: int64(len(runs) + 1), RunID: r.ID, Kind: "autonomous", Outcome: r.Status, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindLedger, Reference: "ledger:" + r.ID, Detail: "Exact completed autonomous ledger run."}, StartedAt: r.StartedAt, CompletedAt: *r.CompletedAt})
		if r.CommitSHA != "" {
			parent := ""
			if len(commits) > 0 {
				parent = commits[len(commits)-1].SHA
			}
			commits = append(commits, autonomousfinalization.CommitEvidence{Sequence: int64(len(commits) + 1), SHA: r.CommitSHA, ParentSHA: parent, RunID: r.ID, Action: autonomous.ActionImplement, Outcome: "reconciled", Reconciled: true, CreatedAt: *r.CompletedAt})
		}
	}
	now := p.clock().UTC()
	source := autonomouspolicy.SourceEvidence{Revision: cycle.Source.FinalRevision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: step.LatestMutation}
	if source.LatestMutation == nil {
		source.LatestMutation = inferMutation(p.taskID, history, source.Revision)
	}
	e := autonomousfinalization.FrozenEvidence{SchemaVersion: autonomousfinalization.FrozenEvidenceSchemaVersion, OperationID: p.operationID + "-finalization", FinalizationRunID: "finalization-" + id.New(), Task: autonomousfinalization.TaskSource{TaskID: task.ID, Title: task.Title, Path: task.SourcePath, SHA256: task.SourceSHA256(), ByteSize: task.SourceByteSize(), Workflow: task.Workflow, StatePath: task.AutonomousStatePath, CompletedSHA256: projected.SourceSHA256(), CompletedByteSize: projected.SourceByteSize()}, State: snapshot.State, StateIdentity: stateID, Decision: *cycle.Supervisor.Decision, DecisionReference: *cycle.Supervisor.DecisionReference, Route: *cycle.Route, Source: source, Verification: *step.Verification, Audit: *step.Audit, Workspace: *snapshot.State.Workspace, SafetyPolicy: *cycle.SafetyPolicy, SafetyPreflight: *cycle.SafetyPreflight, EffectiveConfigSchema: p.configSchema, EffectiveConfigSHA256: p.configSHA, Commits: commits, Runs: runs, Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: task.SourcePath, Detail: "Exact canonical task source at completion admission."}, {Kind: autonomous.EvidenceKindLedger, Reference: "ledger:" + cycle.Supervisor.RunID, Detail: "Fresh complete supervisor decision run."}}, AdmittedAt: now, TerminalAt: now}
	if err := e.Validate(); err != nil {
		return autonomousfinalization.FrozenEvidence{}, err
	}
	return e, nil
}

func inferMutation(taskID string, history []ledger.RunWithEvents, revision string) *autonomouspolicy.SourceMutation {
	for _, item := range history {
		if item.Run.CommitSHA != "" {
			return &autonomouspolicy.SourceMutation{TaskID: taskID, RunID: item.Run.ID, DecisionID: "decision-" + item.Run.ID, Action: autonomous.ActionImplement, ResultingRevision: revision}
		}
	}
	return nil
}

func revalidateFrozen(ctx context.Context, p productionStepConfig, e autonomousfinalization.FrozenEvidence) error {
	task, found, err := taskfile.FindByID(p.root, p.taskID)
	if err != nil || !found {
		return errors.Join(err, autonomousstate.ErrTaskMissing)
	}
	if task.SourceSHA256() != e.Task.SHA256 || task.SourceByteSize() != e.Task.ByteSize {
		return errors.New("completion task source changed")
	}
	snapshot, found, err := p.stateStore.Load(ctx, p.taskID)
	if err != nil || !found {
		return errors.Join(err, autonomousstate.ErrStateMissing)
	}
	identity, err := autonomousstate.StateIdentityFor(snapshot.SourcePath, true, snapshot.State)
	if err != nil || identity != e.StateIdentity {
		return errors.Join(err, errors.New("completion state changed"))
	}
	source, err := gitstate.CaptureSourceSnapshot(ctx, gitstate.SourceSnapshotConfig{WorkingDir: e.Workspace.ExecutionRoot, GitExecutable: p.run.GitExecutable, Timeout: p.run.GitTimeout, StdoutCap: p.run.GitStdoutCap, StderrCap: p.run.GitStderrCap, CommandRunner: gitstate.CommandRunner(commandRunner(p.run))})
	if err != nil {
		return err
	}
	revision, err := gitstate.PolicySourceRevision(source)
	if err != nil {
		return err
	}
	if revision != e.Source.Revision || source.Head != e.Workspace.HeadSHA {
		return errors.New("completion workspace source changed")
	}
	return nil
}

func bytesSHA(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
