package autonomouscycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousassembly"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/lock"
	"revolvr/internal/pathguard"
	"revolvr/internal/prompt"
	"revolvr/internal/redact"
	"revolvr/internal/runner"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

const (
	defaultReleaseTimeout = 5 * time.Second
	workerProfileName     = "supervisor"
)

type normalizedConfig struct {
	Config
	root               string
	executionRoot      string
	state              autonomous.ExecutionState
	supervisorRunID    string
	decisionID         string
	safetyPolicySHA256 string
	redactor           *redact.Redactor
	sourceGuard        *lock.SourceGuard
}

// Run performs one deterministic supervisor decision and at most one worker
// invocation in an exact durable task workspace. It fails closed rather than
// falling back to the operator's primary worktree.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Workspace == nil {
		return failed(Result{TaskID: cfg.TaskID}, OutcomeInvalidConfiguration, "workspace", errors.New("validated task workspace is required"))
	}
	return run(ctx, cfg)
}

func run(ctx context.Context, cfg Config) (result Result, runErr error) {
	n, err := normalizeConfig(cfg)
	if err != nil {
		return failed(Result{TaskID: cfg.TaskID}, OutcomeInvalidConfiguration, "configuration", err)
	}
	result = Result{TaskID: n.TaskID}

	task, found, err := n.TaskLoader(n.root, n.TaskID)
	if err != nil {
		return failed(result, OutcomeInvalidConfiguration, "task", fmt.Errorf("load canonical task: %w", err))
	}
	if !found {
		return failed(result, OutcomeNoTaskState, "task", fmt.Errorf("canonical task %q was not found", n.TaskID))
	}
	if err := validateTask(task, n.TaskID); err != nil {
		return failed(result, OutcomeInvalidConfiguration, "task", err)
	}
	durableStateBefore, err := captureOptionalFile(n.root, task.AutonomousStatePath)
	if err != nil {
		return failed(result, OutcomeInvalidConfiguration, "state_evidence", err)
	}

	dossierBefore, err := captureSource(ctx, n)
	if err != nil {
		return failed(result, OutcomeDossierFailed, "dossier_source_before", err)
	}
	result.Source.DossierBefore = &dossierBefore
	dossierRevision, err := gitstate.PolicySourceRevision(dossierBefore)
	if err != nil {
		return failed(result, OutcomeDossierFailed, "dossier_source_before", err)
	}
	result.Source.DossierRevision = dossierRevision
	preflightSource := autonomouspolicy.SourceEvidence{
		Revision:       dossierRevision,
		Safety:         n.SourceSafety,
		LatestMutation: cloneSourceMutation(n.LatestMutation),
	}
	if err := autonomouspolicy.ValidateEvidence(n.TaskID, preflightSource, n.Verification, n.Audit); err != nil {
		return failed(result, OutcomeInvalidConfiguration, "policy_evidence", err)
	}
	safetyOutput, safetyErr := n.SafetyPreflightRunner(ctx, safetyInput(n, dossierRevision, dossierBefore.Head))
	result.SafetyPolicy = &safetyOutput.Policy
	result.SafetyPreflight = &safetyOutput.Preflight
	if safetyErr != nil {
		return failed(result, OutcomeSafetyPreflightFailed, "safety_preflight", safetyErr)
	}
	if err := validateSafetyOutput(n, dossierRevision, safetyOutput); err != nil {
		return failed(result, OutcomeSafetyPreflightFailed, "safety_preflight", err)
	}
	n.safetyPolicySHA256 = safetyOutput.Policy.PolicySHA256
	n.redactor = safetyOutput.Redactor
	n.CommandRunner = policyCommandRunner(n.CommandRunner, n.redactor, safetyOutput.Policy.Environment, n.SafetyLookupEnv)

	var audit *autonomous.AuditReport
	if n.Audit != nil {
		report := n.Audit.Report
		audit = &report
	}
	dossier, err := n.DossierAssembler(ctx, autonomousassembly.Input{
		RepositoryRoot:      n.root,
		ExecutionRoot:       n.executionRoot,
		TaskID:              n.TaskID,
		State:               n.state,
		Audit:               audit,
		Verification:        verificationSummaryForDossier(n.Verification),
		HistoryPolicy:       n.HistoryPolicy,
		LedgerPath:          n.LedgerPath,
		HistoryReader:       n.HistoryReader,
		GuidancePolicy:      n.GuidancePolicy,
		RepositoryMapPolicy: n.RepositoryMapPolicy,
		Role:                autonomous.DossierRoleSupervisor,
		Git: autonomousassembly.GitOptions{
			Executable:    n.GitExecutable,
			Timeout:       n.GitTimeout,
			StdoutLimit:   n.GitStdoutCap,
			StderrLimit:   n.GitStderrCap,
			CommandRunner: n.CommandRunner,
		},
	})
	if err != nil {
		return failed(result, OutcomeDossierFailed, "dossier_assembly", err)
	}
	if dossier.Manifest.Projection == nil || dossier.Manifest.Projection.Role != autonomous.DossierRoleSupervisor {
		dossier, err = autonomous.ReprojectTaskDossier(dossier, autonomous.DossierRoleSupervisor)
		if err != nil {
			return failed(result, OutcomeDossierFailed, "dossier_projection", err)
		}
	}
	if err := supervisor.ValidateDossier(n.TaskID, dossier); err != nil {
		return failed(result, OutcomeDossierFailed, "dossier_validation", err)
	}
	if err := ensureOptionalFileUnchanged(n.root, task.AutonomousStatePath, durableStateBefore); err != nil {
		return failed(result, OutcomeDossierFailed, "dossier_state_mutation", err)
	}
	result.DossierManifest = dossier.Manifest

	dossierAfter, err := captureSource(ctx, n)
	if err != nil {
		return failed(result, OutcomeDossierFailed, "dossier_source_after", err)
	}
	result.Source.DossierAfter = &dossierAfter
	result.Source.DossierDifference = gitstate.CompareSourceSnapshots(dossierBefore, dossierAfter)
	if result.Source.DossierDifference.Changed {
		return failed(result, OutcomeSourceChangedDuringDossier, "dossier_source_window", sourceChangedError("source changed during dossier assembly", result.Source.DossierDifference))
	}

	supervisorResult, supervisorErr := n.SupervisorRunner(ctx, supervisor.Config{
		RepositoryRoot:                 n.root,
		ExecutionRoot:                  n.executionRoot,
		WorkspaceID:                    workspaceID(n.Workspace),
		TaskID:                         n.TaskID,
		Dossier:                        dossier,
		Audit:                          audit,
		VerificationFailure:            n.CorrectionFailure,
		Ledger:                         n.Ledger,
		RunID:                          n.supervisorRunID,
		DecisionID:                     n.decisionID,
		Clock:                          n.Clock,
		CodexExecutable:                n.CodexExecutable,
		CodexModel:                     n.CodexModel,
		CodexReasoningEffort:           n.CodexReasoningEffort,
		CodexEphemeral:                 n.CodexEphemeral,
		CodexSandbox:                   n.CodexSandbox,
		CodexApprovalPolicy:            n.CodexApprovalPolicy,
		CodexBypassApprovalsAndSandbox: n.CodexBypassApprovalsAndSandbox,
		CodexVersion:                   n.CodexVersion,
		EffectiveConfigSchema:          n.EffectiveConfigSchema,
		EffectiveConfigSHA256:          n.EffectiveConfigSHA256,
		SafetyPolicySHA256:             safetyOutput.Policy.PolicySHA256,
		CodexTimeout:                   n.CodexTimeout,
		CodexStdoutCap:                 n.CodexStdoutCap,
		CodexStderrCap:                 n.CodexStderrCap,
		CodexCommandRunner:             codexexec.CommandRunner(n.CommandRunner),
		GitExecutable:                  n.GitExecutable,
		GitTimeout:                     n.GitTimeout,
		GitStdoutCap:                   n.GitStdoutCap,
		GitStderrCap:                   n.GitStderrCap,
		GitCommandRunner:               gitstate.CommandRunner(n.CommandRunner),
		SourceSnapshotter:              supervisor.SourceSnapshotter(n.SourceSnapshotter),
		SourceLockTimeout:              n.SourceWriterLockTimeout,
		SourceLockPID:                  n.SourceWriterLockPID,
		Redactor:                       safetyOutput.Redactor,
		SafetyPolicy:                   &safetyOutput.Policy,
		SafetyPreflight:                &safetyOutput.Preflight,
	})
	result.Supervisor = supervisorResult
	if supervisorErr != nil {
		return failed(result, OutcomeSupervisorFailed, "supervisor", supervisorErr)
	}
	if err := validateSupervisorResult(n, dossier, dossierAfter, supervisorResult); err != nil {
		return failed(result, OutcomeSupervisorFailed, "supervisor_evidence", err)
	}
	if err := ensureOptionalFileUnchanged(n.root, task.AutonomousStatePath, durableStateBefore); err != nil {
		return failed(result, OutcomeSupervisorFailed, "supervisor_state_mutation", err)
	}

	lockOwner := "cycle-" + n.supervisorRunID
	lockConfig := lock.Config{
		WorkingDir: n.root,
		RunID:      lockOwner,
		PID:        n.SourceWriterLockPID,
		Timeout:    n.SourceWriterLockTimeout,
		Clock:      n.Clock,
	}
	if n.Workspace != nil {
		// The lock file remains control-root state while naming the exact source
		// worktree it protects.
		lockConfig.ControlRoot = n.root
		lockConfig.ExecutionRoot = n.executionRoot
		lockConfig.WorkspaceID = n.Workspace.WorkspaceID
	}
	sourceLock, err := n.LockAcquirer(ctx, lockConfig)
	if err != nil {
		return failed(result, OutcomeSourceChanged, "worker_lock", fmt.Errorf("acquire worker source-writer lock: %w", err))
	}
	sourceGuard := lock.MonitorSourceLease(ctx, sourceLock, n.SourceWriterLockHeartbeatInterval)
	n.sourceGuard = sourceGuard
	ctx = sourceGuard.Context()
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), defaultReleaseTimeout)
		defer cancel()
		if lockErr := sourceGuard.Close(releaseCtx); lockErr != nil {
			if runErr == nil {
				result.Outcome = OutcomeSourceChanged
				result.Failure = &Failure{Stage: "worker_lock", Reason: lockErr.Error()}
			}
			runErr = errors.Join(runErr, lockErr)
		}
	}()

	admission, err := captureSource(ctx, n)
	if err != nil {
		return failed(result, OutcomeSourceChanged, "worker_admission_source", err)
	}
	result.Source.Admission = &admission
	result.Source.AdmissionRevision, err = gitstate.PolicySourceRevision(admission)
	if err != nil {
		return failed(result, OutcomeSourceChanged, "worker_admission_source", err)
	}
	if difference := gitstate.CompareSourceSnapshots(dossierAfter, admission); difference.Changed {
		result.Source.AdmissionDifference = difference
		return failed(result, OutcomeSourceChanged, "worker_admission_source", sourceChangedError("source changed between supervision and worker admission", difference))
	}
	if err := ensureOptionalFileUnchanged(n.root, task.AutonomousStatePath, durableStateBefore); err != nil {
		return failed(result, OutcomeSourceChanged, "worker_admission_state", err)
	}
	if err := sourceOwnershipError(ctx, n); err != nil {
		return failed(result, OutcomeSourceChanged, "worker_lock_admission", err)
	}

	policyInput := autonomouspolicy.Input{
		TaskID:    n.TaskID,
		Decision:  *supervisorResult.Decision,
		Reference: *supervisorResult.DecisionReference,
		State:     n.state,
		Source: autonomouspolicy.SourceEvidence{
			Revision:       result.Source.AdmissionRevision,
			Safety:         n.SourceSafety,
			LatestMutation: cloneSourceMutation(n.LatestMutation),
		},
		Verification:      cloneVerificationEvidence(n.Verification),
		Audit:             cloneAuditEvidence(n.Audit),
		CorrectionFailure: cloneVerificationFailure(n.CorrectionFailure),
	}
	route, err := n.PolicyEvaluator(policyInput)
	if err != nil {
		return failed(result, OutcomePolicyRejected, "policy", err)
	}
	if err := validateRoute(route, policyInput); err != nil {
		return failed(result, OutcomePolicyRejected, "policy_route", err)
	}
	result.Route = &route

	switch route.Kind {
	case autonomouspolicy.RouteKindComplete:
		if err := sourceOwnershipError(ctx, n); err != nil {
			return failed(result, OutcomeSourceChanged, "worker_lock_final", err)
		}
		result.Source.Final = &admission
		result.Source.FinalRevision = result.Source.AdmissionRevision
		result.Outcome = OutcomeCompleteAuthorized
		return result, nil
	case autonomouspolicy.RouteKindBlock:
		if err := sourceOwnershipError(ctx, n); err != nil {
			return failed(result, OutcomeSourceChanged, "worker_lock_final", err)
		}
		result.Source.Final = &admission
		result.Source.FinalRevision = result.Source.AdmissionRevision
		result.Outcome = OutcomeBlockAuthorized
		return result, nil
	case autonomouspolicy.RouteKindNeedsInput:
		if err := sourceOwnershipError(ctx, n); err != nil {
			return failed(result, OutcomeSourceChanged, "worker_lock_final", err)
		}
		result.Source.Final = &admission
		result.Source.FinalRevision = result.Source.AdmissionRevision
		result.Outcome = OutcomeNeedsInputAuthorized
		return result, nil
	case autonomouspolicy.RouteKindWorker:
		if err := sourceOwnershipError(ctx, n); err != nil {
			return failed(result, OutcomeSourceChanged, "worker_lock_before_worker", err)
		}
		if n.BeforeWorker != nil {
			if err := n.BeforeWorker(ctx, WorkerAdmissionInput{
				TaskID: n.TaskID, State: n.state,
				Decision: *supervisorResult.Decision, Reference: *supervisorResult.DecisionReference,
				Route: route, SourceRevision: result.Source.AdmissionRevision,
			}); err != nil {
				return failed(result, OutcomePolicyRejected, "attempt_admission", err)
			}
		}
		return runWorker(ctx, n, task, dossier, supervisorResult, route, admission, result)
	default:
		return failed(result, OutcomePolicyRejected, "policy_route", fmt.Errorf("unknown route kind %q", route.Kind))
	}
}

func normalizeConfig(cfg Config) (normalizedConfig, error) {
	root := strings.TrimSpace(cfg.RepositoryRoot)
	if root == "" {
		return normalizedConfig{}, errors.New("autonomous cycle: repository root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: resolve repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: resolve repository root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: repository root is not a directory: %w", err)
	}
	if cfg.TaskID == "" || cfg.TaskID != strings.TrimSpace(cfg.TaskID) || strings.ContainsAny(cfg.TaskID, "\r\n") {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: task_id %q is empty or malformed", cfg.TaskID)
	}
	if !cfg.RepositoryMapPolicy.Enabled {
		cfg.RepositoryMapPolicy = autonomousassembly.RepositoryMapPolicy{Enabled: true, MaxPaths: 4000, MaxBytes: 512 * 1024}
	}
	executionRoot := resolved
	if cfg.Workspace != nil {
		if err := cfg.Workspace.Validate(); err != nil {
			return normalizedConfig{}, fmt.Errorf("autonomous cycle: workspace: %w", err)
		}
		if cfg.Workspace.TaskID != cfg.TaskID || cfg.Workspace.ControlRoot != resolved {
			return normalizedConfig{}, errors.New("autonomous cycle: workspace task/control identity mismatch")
		}
		if cfg.Workspace.Status != autonomous.WorkspaceStatusReady && cfg.Workspace.Status != autonomous.WorkspaceStatusRestored {
			return normalizedConfig{}, fmt.Errorf("autonomous cycle: workspace status %q is not admitted for execution", cfg.Workspace.Status)
		}
		actual, resolveErr := filepath.EvalSymlinks(cfg.Workspace.ExecutionRoot)
		if resolveErr != nil || actual != cfg.Workspace.ExecutionRoot {
			return normalizedConfig{}, errors.Join(resolveErr, errors.New("autonomous cycle: execution root is missing or noncanonical"))
		}
		executionRoot = actual
	}
	if err := cfg.State.Validate(); err != nil {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: execution state: %w", err)
	}
	if cfg.State.TaskID != cfg.TaskID {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: task_id %q does not match execution state task_id %q", cfg.TaskID, cfg.State.TaskID)
	}
	if cfg.Workspace != nil && (cfg.State.Workspace == nil || !reflect.DeepEqual(*cfg.State.Workspace, *cfg.Workspace)) {
		return normalizedConfig{}, errors.New("autonomous cycle: admitted workspace does not match durable state")
	}
	state, err := cloneExecutionState(cfg.State)
	if err != nil {
		return normalizedConfig{}, err
	}
	if cfg.Ledger == nil {
		return normalizedConfig{}, errors.New("autonomous cycle: writable ledger is required")
	}
	if cfg.IDGenerator == nil {
		return normalizedConfig{}, errors.New("autonomous cycle: ID generator is required")
	}
	if cfg.Clock == nil {
		return normalizedConfig{}, errors.New("autonomous cycle: clock is required")
	}
	if strings.TrimSpace(cfg.CodexExecutable) == "" || strings.TrimSpace(cfg.CodexVersion) == "" {
		return normalizedConfig{}, errors.New("autonomous cycle: explicit Codex executable and version are required")
	}
	if _, err := codexexec.NormalizeModel(cfg.CodexModel); err != nil {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: %w", err)
	}
	if _, err := codexexec.NormalizeReasoningEffort(cfg.CodexReasoningEffort); err != nil {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: %w", err)
	}
	if !cfg.CodexEphemeral {
		return normalizedConfig{}, errors.New("autonomous cycle: only fresh ephemeral Codex sessions are supported")
	}
	if err := cfg.SafetyDeclaration.Validate(); err != nil {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: %w", err)
	}
	if strings.TrimSpace(cfg.CodexSandbox) == "" || strings.TrimSpace(cfg.CodexApprovalPolicy) == "" {
		return normalizedConfig{}, errors.New("autonomous cycle: explicit Codex sandbox and approval policy are required")
	}
	if strings.TrimSpace(cfg.EffectiveConfigSchema) == "" || !validSHA256(cfg.EffectiveConfigSHA256) {
		return normalizedConfig{}, errors.New("autonomous cycle: valid effective-config schema and SHA-256 are required")
	}
	if cfg.CodexTimeout <= 0 || cfg.CodexStdoutCap <= 0 || cfg.CodexStderrCap <= 0 {
		return normalizedConfig{}, errors.New("autonomous cycle: positive Codex timeout and output caps are required")
	}
	if strings.TrimSpace(cfg.GitExecutable) == "" || cfg.GitTimeout <= 0 || cfg.GitStdoutCap <= 0 || cfg.GitStderrCap <= 0 {
		return normalizedConfig{}, errors.New("autonomous cycle: explicit Git executable, timeout, and output caps are required")
	}
	if cfg.SourceWriterLockTimeout <= 0 || cfg.SourceWriterLockHeartbeatInterval <= 0 {
		return normalizedConfig{}, errors.New("autonomous cycle: positive source-lock timeout and heartbeat interval are required")
	}
	minimumSupervisorLock := cfg.CodexTimeout + 2*cfg.GitTimeout + time.Minute
	if cfg.SourceWriterLockTimeout < minimumSupervisorLock {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: source-lock timeout %s is shorter than required supervisor window %s", cfg.SourceWriterLockTimeout, minimumSupervisorLock)
	}
	if cfg.VerificationTimeout <= 0 || cfg.VerificationStdoutCap <= 0 || cfg.VerificationStderrCap <= 0 {
		return normalizedConfig{}, errors.New("autonomous cycle: positive verification timeout and output caps are required")
	}
	if cfg.CommitTimeout <= 0 || cfg.CommitStdoutCap <= 0 || cfg.CommitStderrCap <= 0 {
		return normalizedConfig{}, errors.New("autonomous cycle: positive commit timeout and output caps are required")
	}
	if err := validateVerificationConfig(executionRoot, cfg); err != nil {
		return normalizedConfig{}, err
	}
	if cfg.CorrectionFailure != nil {
		if err := cfg.CorrectionFailure.Validate(); err != nil {
			return normalizedConfig{}, fmt.Errorf("autonomous cycle: correction failure: %w", err)
		}
		if cfg.CorrectionFailure.TaskID != cfg.TaskID {
			return normalizedConfig{}, errors.New("autonomous cycle: correction failure has wrong task identity")
		}
	}

	if cfg.TaskLoader == nil {
		cfg.TaskLoader = taskfile.FindByID
	}
	if cfg.DossierAssembler == nil {
		cfg.DossierAssembler = autonomousassembly.Assemble
	}
	if cfg.SupervisorRunner == nil {
		cfg.SupervisorRunner = supervisor.Run
	}
	if cfg.PolicyEvaluator == nil {
		cfg.PolicyEvaluator = autonomouspolicy.Evaluate
	}
	if cfg.ProfileLoader == nil {
		cfg.ProfileLoader = prompt.LoadRunProfile
	}
	if cfg.CodexRunner == nil {
		cfg.CodexRunner = codexexec.Run
	}
	if cfg.SourceSnapshotter == nil {
		cfg.SourceSnapshotter = gitstate.CaptureSourceSnapshot
	}
	if cfg.DirtyCapture == nil {
		cfg.DirtyCapture = gitstate.CaptureDirtyWorktree
	}
	if cfg.ChangedCapture == nil {
		cfg.ChangedCapture = gitstate.CaptureChangedFiles
	}
	if cfg.VerificationRunner == nil {
		cfg.VerificationRunner = verification.Run
	}
	if cfg.TieredVerificationRunner == nil {
		cfg.TieredVerificationRunner = autonomousverification.Execute
	}
	if cfg.CommitRunner == nil {
		cfg.CommitRunner = commit.Run
	}
	if cfg.LockAcquirer == nil {
		cfg.LockAcquirer = func(ctx context.Context, lockCfg lock.Config) (SourceLock, error) {
			return lock.AcquireSourceWriter(ctx, lockCfg)
		}
	}
	if cfg.SafetyPreflightRunner == nil {
		cfg.SafetyPreflightRunner = autonomoussafety.Run
	}

	supervisorRunID := strings.TrimSpace(cfg.IDGenerator())
	if !safePathID(supervisorRunID) {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: generated unsafe supervisor run ID %q", supervisorRunID)
	}
	decisionID := "decision-" + strings.ToLower(supervisorRunID)
	if !validDecisionID(decisionID) {
		return normalizedConfig{}, fmt.Errorf("autonomous cycle: generated invalid decision ID %q", decisionID)
	}
	cfg.RepositoryRoot = resolved
	cfg.State = state
	cfg.VerificationCommands = cloneVerificationCommands(cfg.VerificationCommands)
	if cfg.VerificationPlan != nil {
		plan := autonomousverification.ClonePlan(*cfg.VerificationPlan)
		cfg.VerificationPlan = &plan
	}
	return normalizedConfig{Config: cfg, root: resolved, executionRoot: executionRoot, state: state, supervisorRunID: supervisorRunID, decisionID: decisionID}, nil
}

func safetyInput(n normalizedConfig, sourceRevision, observedHead string) autonomoussafety.Input {
	commands := []autonomoussafety.CommandSpec{
		{Kind: "codex", Executable: n.CodexExecutable, Args: []string{"exec", "--model", n.CodexModel, "--sandbox", n.CodexSandbox, "--ask-for-approval", n.CodexApprovalPolicy}, WorkingDir: n.executionRoot, Timeout: n.CodexTimeout, StdoutCap: n.CodexStdoutCap, StderrCap: n.CodexStderrCap},
		{Kind: "git", Executable: n.GitExecutable, WorkingDir: n.executionRoot, Timeout: n.GitTimeout, StdoutCap: n.GitStdoutCap, StderrCap: n.GitStderrCap},
	}
	appendVerification := func(kind string, command verification.Command) {
		dir := n.executionRoot
		if strings.TrimSpace(command.Dir) != "" {
			dir = filepath.Join(n.executionRoot, command.Dir)
		}
		timeout := command.Timeout
		if timeout <= 0 {
			timeout = n.VerificationTimeout
		}
		stdoutCap := command.StdoutCap
		if stdoutCap <= 0 {
			stdoutCap = n.VerificationStdoutCap
		}
		stderrCap := command.StderrCap
		if stderrCap <= 0 {
			stderrCap = n.VerificationStderrCap
		}
		commands = append(commands, autonomoussafety.CommandSpec{Kind: kind, Executable: command.Name, Args: append([]string(nil), command.Args...), WorkingDir: dir, Environment: append([]string(nil), command.Env...), Timeout: timeout, StdoutCap: stdoutCap, StderrCap: stderrCap})
	}
	for _, command := range n.VerificationCommands {
		appendVerification("verification", command)
	}
	if n.VerificationPlan != nil {
		for _, tier := range n.VerificationPlan.Tiers {
			for _, command := range tier.Commands {
				appendVerification("verification:"+tier.ID, command)
			}
		}
	}
	return autonomoussafety.Input{
		TaskID:         n.TaskID,
		Workspace:      *n.Workspace,
		SourceRevision: sourceRevision,
		ObservedHead:   observedHead,
		Declaration:    n.SafetyDeclaration,
		Codex: autonomoussafety.CodexPolicy{
			Sandbox: n.CodexSandbox, ApprovalPolicy: n.CodexApprovalPolicy, DangerousBypass: n.CodexBypassApprovalsAndSandbox,
			Model: n.CodexModel, ReasoningEffort: n.CodexReasoningEffort, Ephemeral: n.CodexEphemeral,
		},
		Commands:      commands,
		ConfigPath:    filepath.Join(n.root, ".revolvr", "config.yaml"),
		ConfigSHA256:  n.EffectiveConfigSHA256,
		ObservedAt:    n.Clock().UTC(),
		LookupEnv:     n.SafetyLookupEnv,
		LookPath:      n.SafetyLookPath,
		CommandRunner: n.CommandRunner,
		GitExecutable: n.GitExecutable,
		GitTimeout:    n.GitTimeout,
		GitStdoutCap:  n.GitStdoutCap,
		GitStderrCap:  n.GitStderrCap,
	}
}

func validateSafetyOutput(n normalizedConfig, sourceRevision string, output autonomoussafety.Output) error {
	if !output.Preflight.Ready {
		return errors.New("autonomous safety preflight is not ready")
	}
	if output.Preflight.TaskID != n.TaskID || output.Preflight.WorkspaceID != n.Workspace.WorkspaceID || output.Preflight.SourceRevision != sourceRevision || output.Preflight.ConfigSHA256 != n.EffectiveConfigSHA256 || output.Preflight.PolicySHA256 == "" {
		return errors.New("autonomous safety preflight identity mismatch")
	}
	if output.Policy.TaskID != n.TaskID || output.Policy.Workspace.WorkspaceID != n.Workspace.WorkspaceID || output.Policy.PolicySHA256 != output.Preflight.PolicySHA256 || output.Policy.ConfigSHA256 != n.EffectiveConfigSHA256 {
		return errors.New("autonomous safety policy identity mismatch")
	}
	if err := output.Policy.Validate(); err != nil {
		return err
	}
	return nil
}

func policyCommandRunner(commandRunner CommandRunner, redactor *redact.Redactor, environment autonomoussafety.EnvironmentPolicy, lookup func(string) (string, bool)) CommandRunner {
	if redactor == nil && environment.InheritHost {
		return commandRunner
	}
	if lookup == nil {
		lookup = os.LookupEnv
	}
	return func(ctx context.Context, command runner.Command) runner.Result {
		if !environment.InheritHost {
			env := make([]string, 0, len(environment.Allow)+len(command.Env))
			for _, name := range environment.Allow {
				if value, ok := lookup(name); ok {
					env = append(env, name+"="+value)
				}
			}
			env = append(env, command.Env...)
			command.Env = env
			command.ReplaceEnv = true
		}
		result := commandRunner(ctx, command)
		if redactor != nil {
			result.Stdout = redactor.String(result.Stdout)
			result.Stderr = redactor.String(result.Stderr)
			result.Err = redactor.Error(result.Err)
		}
		return result
	}
}

// RunInWorkspace is the fail-closed autonomous mutation entry point.
func RunInWorkspace(ctx context.Context, cfg Config) (Result, error) {
	return Run(ctx, cfg)
}

func validateTask(task taskfile.Task, taskID string) error {
	if task.ID != taskID {
		return fmt.Errorf("loaded task ID %q does not match requested task ID %q", task.ID, taskID)
	}
	if task.Workflow != taskfile.WorkflowAutonomousV1 {
		return fmt.Errorf("task %q uses workflow %q, want %q", taskID, task.Workflow, taskfile.WorkflowAutonomousV1)
	}
	if task.Status != taskfile.StatusPending {
		return fmt.Errorf("task %q has status %q, want pending", taskID, task.Status)
	}
	if task.Profile != "" || task.Phase != "" {
		return fmt.Errorf("task %q contains mixed-pass profile or phase metadata", taskID)
	}
	if strings.TrimSpace(task.AutonomousStatePath) == "" || len(task.SourceBytes) == 0 || strings.TrimSpace(task.SourcePath) == "" {
		return fmt.Errorf("task %q is missing canonical source or autonomous state metadata", taskID)
	}
	return nil
}

func validateSupervisorResult(n normalizedConfig, dossier autonomous.TaskDossier, source gitstate.SourceSnapshot, result supervisor.Result) error {
	if result.RunID != n.supervisorRunID {
		return fmt.Errorf("supervisor run ID %q does not match requested run ID %q", result.RunID, n.supervisorRunID)
	}
	if result.Decision == nil || result.DecisionReference == nil {
		return errors.New("supervisor returned no validated decision and reference")
	}
	if err := result.Decision.Validate(); err != nil {
		return err
	}
	if err := result.DecisionReference.Validate(); err != nil {
		return err
	}
	if result.Decision.TaskID != n.TaskID || result.DecisionReference.TaskID != n.TaskID {
		return errors.New("supervisor decision evidence has the wrong task identity")
	}
	if result.DecisionReference.DecisionID != n.decisionID || result.DecisionReference.RunID != n.supervisorRunID {
		return errors.New("supervisor decision reference has the wrong run or decision identity")
	}
	if result.Dossier.SchemaVersion != dossier.Manifest.SchemaVersion || result.Dossier.TaskID != n.TaskID || result.Dossier.SHA256 != dossier.Manifest.DossierSHA256 || result.Dossier.ByteSize != dossier.Manifest.DossierByteSize {
		return errors.New("supervisor did not retain the exact dossier identity")
	}
	if result.Profile.Name != workerProfileName {
		return fmt.Errorf("supervisor used profile %q, want %q", result.Profile.Name, workerProfileName)
	}
	if err := result.Invocation.Validate(); err != nil {
		return fmt.Errorf("supervisor invocation: %w", err)
	}
	if !sameCodexIntent(n, result.Invocation) {
		return errors.New("supervisor invocation does not match the explicit Codex configuration")
	}
	if result.SourceBefore == nil || result.SourceAfter == nil {
		return errors.New("supervisor source evidence is incomplete")
	}
	if err := result.SourceBefore.Validate(); err != nil {
		return err
	}
	if err := result.SourceAfter.Validate(); err != nil {
		return err
	}
	if diff := gitstate.CompareSourceSnapshots(source, *result.SourceBefore); diff.Changed {
		return sourceChangedError("supervisor source baseline does not match dossier source", diff)
	}
	if diff := gitstate.CompareSourceSnapshots(*result.SourceBefore, *result.SourceAfter); diff.Changed {
		return sourceChangedError("supervisor changed repository source", diff)
	}
	return nil
}

func validateRoute(route autonomouspolicy.Route, input autonomouspolicy.Input) error {
	if route.TaskID != input.TaskID || route.DecisionID != input.Reference.DecisionID || route.Action != input.Decision.Action || route.WorkerProfile != input.Decision.WorkerProfile || route.SourceRevision != input.Source.Revision {
		return errors.New("policy route does not preserve exact task, decision, action, profile, and source identity")
	}
	return nil
}

func captureSource(ctx context.Context, n normalizedConfig) (gitstate.SourceSnapshot, error) {
	snapshot, err := n.SourceSnapshotter(ctx, gitstate.SourceSnapshotConfig{
		WorkingDir:          n.executionRoot,
		GitExecutable:       n.GitExecutable,
		Timeout:             n.GitTimeout,
		StdoutCap:           n.GitStdoutCap,
		StderrCap:           n.GitStderrCap,
		AllowHarnessRuntime: n.executionRoot == n.root,
		CommandRunner:       gitstate.CommandRunner(n.CommandRunner),
	})
	if err != nil {
		return gitstate.SourceSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return gitstate.SourceSnapshot{}, err
	}
	return snapshot, nil
}

func failed(result Result, outcome Outcome, stage string, cause error) (Result, error) {
	result.Outcome = outcome
	result.Failure = &Failure{Stage: stage, Reason: cause.Error()}
	return result, fmt.Errorf("autonomous cycle task %q %s failed: %w", result.TaskID, stage, cause)
}

func sourceChangedError(message string, difference gitstate.SourceDifference) error {
	paths := make([]string, 0, len(difference.PathChanges))
	for _, change := range difference.PathChanges {
		paths = append(paths, change.Path)
	}
	return fmt.Errorf("%s: before=%q current=%q head_changed=%t index_changed=%t worktree_changed=%t paths=%q", message, difference.BeforeSHA256, difference.AfterSHA256, difference.HeadChanged, difference.IndexChanged, difference.WorktreeChanged, paths)
}

func sourceOwnershipError(ctx context.Context, n normalizedConfig) error {
	if n.sourceGuard == nil {
		return nil
	}
	if failure := n.sourceGuard.Failure(); failure != nil {
		return failure
	}
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, defaultReleaseTimeout)
	defer cancel()
	return n.sourceGuard.Check(checkCtx)
}

func workspaceID(workspace *autonomous.TaskWorkspace) string {
	if workspace == nil {
		return ""
	}
	return workspace.WorkspaceID
}

func validateVerificationConfig(root string, cfg Config) error {
	if cfg.VerificationPlan != nil && len(cfg.VerificationCommands) > 0 {
		return errors.New("autonomous cycle: flat verification commands and a tiered verification plan cannot both be configured")
	}
	if cfg.VerificationPlan != nil {
		if err := cfg.VerificationPlan.Validate(); err != nil {
			return fmt.Errorf("autonomous cycle: %w", err)
		}
		for _, tier := range cfg.VerificationPlan.Tiers {
			for i, command := range tier.Commands {
				if strings.TrimSpace(command.Dir) != "" {
					if _, err := pathguard.Resolve(root, command.Dir); err != nil {
						return fmt.Errorf("autonomous cycle: tier %q command %d directory: %w", tier.ID, i, err)
					}
				}
			}
		}
		return nil
	}
	commands := cfg.VerificationCommands
	if len(commands) == 0 {
		switch cfg.MissingVerificationPolicy {
		case verification.MissingCommandsFail, verification.MissingCommandsPass:
		default:
			return errors.New("autonomous cycle: missing verification policy is required when no commands are configured")
		}
	}
	for i, command := range commands {
		if strings.TrimSpace(command.Name) == "" {
			return fmt.Errorf("autonomous cycle: verification command %d name is required", i)
		}
		if strings.TrimSpace(command.Dir) != "" {
			if _, err := pathguard.Resolve(root, command.Dir); err != nil {
				return fmt.Errorf("autonomous cycle: verification command %d directory: %w", i, err)
			}
		}
	}
	return nil
}

func cloneExecutionState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, fmt.Errorf("autonomous cycle: clone execution state: %w", err)
	}
	var cloned autonomous.ExecutionState
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return autonomous.ExecutionState{}, fmt.Errorf("autonomous cycle: clone execution state: %w", err)
	}
	return cloned, nil
}

func cloneVerificationCommands(commands []verification.Command) []verification.Command {
	cloned := make([]verification.Command, len(commands))
	for i, command := range commands {
		cloned[i] = command
		cloned[i].Args = append([]string(nil), command.Args...)
		cloned[i].Env = append([]string(nil), command.Env...)
	}
	return cloned
}

func cloneSourceMutation(value *autonomouspolicy.SourceMutation) *autonomouspolicy.SourceMutation {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneVerificationEvidence(value *autonomouspolicy.VerificationEvidence) *autonomouspolicy.VerificationEvidence {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Summary.Evidence = append([]autonomous.EvidenceReference(nil), value.Summary.Evidence...)
	if value.Summary.Tiered != nil {
		raw, _ := json.Marshal(value.Summary.Tiered)
		var tiered autonomousverification.Result
		_ = json.Unmarshal(raw, &tiered)
		cloned.Summary.Tiered = &tiered
	}
	if value.Tiered != nil {
		gate := *value.Tiered
		gate.RequiredFinalTiers = append([]string(nil), value.Tiered.RequiredFinalTiers...)
		gate.SelectedTiers = append([]string(nil), value.Tiered.SelectedTiers...)
		gate.ExecutedTiers = append([]string(nil), value.Tiered.ExecutedTiers...)
		gate.RequiredOutcomes = append([]autonomousverification.TierGate(nil), value.Tiered.RequiredOutcomes...)
		gate.MissingRequired = append([]string(nil), value.Tiered.MissingRequired...)
		cloned.Tiered = &gate
	}
	return &cloned
}

func verificationSummaryForDossier(value *autonomouspolicy.VerificationEvidence) *autonomous.VerificationSummary {
	if value == nil {
		return nil
	}
	cloned := cloneVerificationEvidence(value)
	return &cloned.Summary
}

func cloneAuditEvidence(value *autonomouspolicy.AuditEvidence) *autonomouspolicy.AuditEvidence {
	if value == nil {
		return nil
	}
	raw, _ := json.Marshal(value)
	var cloned autonomouspolicy.AuditEvidence
	_ = json.Unmarshal(raw, &cloned)
	return &cloned
}

func cloneVerificationFailure(value *autonomous.VerificationFailureTarget) *autonomous.VerificationFailureTarget {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Evidence = append([]autonomous.EvidenceReference(nil), value.Evidence...)
	return &cloned
}

func sameCodexIntent(n normalizedConfig, invocation codexexec.InvocationProvenance) bool {
	return invocation.Executable == strings.TrimSpace(n.CodexExecutable) &&
		invocation.Model == strings.TrimSpace(n.CodexModel) &&
		invocation.ReasoningEffort == strings.TrimSpace(n.CodexReasoningEffort) &&
		invocation.Ephemeral && invocation.SessionMode == codexexec.SessionModeEphemeral &&
		invocation.Version == strings.TrimSpace(n.CodexVersion) &&
		invocation.EffectiveConfigSchema == strings.TrimSpace(n.EffectiveConfigSchema) &&
		invocation.EffectiveConfigSHA256 == strings.TrimSpace(n.EffectiveConfigSHA256) &&
		invocation.SafetyPolicySHA256 == n.safetyPolicySHA256 &&
		invocation.WorkingDir == n.executionRoot
}

func safePathID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func validDecisionID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for i, r := range value {
		switch {
		case i == 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && r == '-' && value[i-1] != '-' && i < len(value)-1:
		default:
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
