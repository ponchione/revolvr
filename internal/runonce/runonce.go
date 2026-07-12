package runonce

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/id"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/passpolicy"
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/verification"
)

const (
	defaultCodexSandbox        = "workspace-write"
	defaultCodexApprovalPolicy = "never"
	defaultGitExecutable       = "git"
	defaultOutputCap           = 256 * 1024
	defaultLockTimeout         = 5 * time.Minute
	defaultLockReleaseTimeout  = 5 * time.Second
)

type Outcome string

const (
	OutcomeNoTask             Outcome = "no_task"
	OutcomeCommitted          Outcome = "committed"
	OutcomeCodexFailed        Outcome = "codex_failed"
	OutcomeVerificationFailed Outcome = "verification_failed"
	OutcomeNoChanges          Outcome = "no_changes"
	OutcomeBlocked            Outcome = "blocked"
	OutcomeCommitFailed       Outcome = "commit_failed"
)

type CommandRunner func(context.Context, runner.Command) runner.Result

type CodexRunner func(context.Context, codexexec.Config) (codexexec.Result, error)
type CodexVersionDiscoverer func(context.Context, codexexec.VersionConfig) (string, error)
type DirtyCapture func(context.Context, gitstate.Config) (gitstate.Capture, error)
type ChangedCapture func(context.Context, gitstate.Config) (gitstate.Capture, error)
type VerificationRunner func(context.Context, verification.Config) (verification.Result, error)
type CommitRunner func(context.Context, commit.Config) (commit.Result, error)

type Config struct {
	WorkingDir string

	LedgerStore *ledger.Store
	LedgerPath  string

	CodexExecutable                string
	CodexModel                     string
	CodexReasoningEffort           string
	CodexEphemeral                 bool
	CodexSandbox                   string
	CodexApprovalPolicy            string
	CodexBypassApprovalsAndSandbox bool
	CodexTimeout                   time.Duration
	CodexStdoutCap                 int
	CodexStderrCap                 int

	GitExecutable string
	GitTimeout    time.Duration
	GitStdoutCap  int
	GitStderrCap  int

	VerificationCommands      []verification.Command
	VerificationPlan          *autonomousverification.Plan
	MissingVerificationPolicy verification.MissingCommandsPolicy
	VerificationTimeout       time.Duration
	VerificationStdoutCap     int
	VerificationStderrCap     int

	AllowPreExistingDirty    bool
	AllowMissingVerification bool
	CommitTimeout            time.Duration
	CommitStdoutCap          int
	CommitStderrCap          int

	SourceWriterLockTimeout           time.Duration
	SourceWriterLockHeartbeatInterval time.Duration
	SourceWriterLockPID               int

	CommandRunner          CommandRunner
	CodexRunner            CodexRunner
	CodexVersionDiscoverer CodexVersionDiscoverer
	DirtyCapture           DirtyCapture
	ChangedCapture         ChangedCapture
	VerificationRunner     VerificationRunner
	CommitRunner           CommitRunner
	Clock                  func() time.Time
	CodexProgress          func(codexexec.ProgressEvent)
}

type Result struct {
	Outcome            Outcome
	Message            string
	WorkingDir         string
	NoTask             bool
	Task               taskmodel.Task
	FileTask           taskfile.Task
	Run                ledger.Run
	Receipt            receipt.Receipt
	ReceiptPath        string
	ReceiptRelPath     string
	ReceiptSynthesized bool
	ReceiptError       string
	PreRunDirty        gitstate.Capture
	PostRunChanged     gitstate.Capture
	Codex              codexexec.Result
	Verification       verification.Result
	Commit             commit.Result
	ReceiptWarnings    []ReceiptWarning
	LedgerError        error

	phasePolicy            passpolicy.Policy
	phaseTransitionApplied bool
	selectedFileTask       taskfile.Task
	changedCaptureError    string
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	cfg, workDir, err := normalizeConfig(cfg)
	if err != nil {
		return Result{}, err
	}
	if cfg.VerificationPlan != nil {
		return Result{}, errors.New("run once: tiered autonomous verification is not supported by mixed-pass-v1")
	}
	effectiveConfig, err := FingerprintEffectiveConfig(cfg)
	if err != nil {
		return Result{}, err
	}
	result := Result{WorkingDir: workDir}
	runID := id.New()

	sourceLock, err := lock.AcquireSourceWriter(ctx, lock.Config{
		WorkingDir: workDir,
		RunID:      runID,
		PID:        cfg.SourceWriterLockPID,
		Timeout:    cfg.SourceWriterLockTimeout,
		Clock:      cfg.Clock,
	})
	if err != nil {
		result.Message = "source-writer lock unavailable: " + err.Error()
		return result, err
	}
	stopHeartbeat := startLockHeartbeat(ctx, sourceLock, cfg.SourceWriterLockHeartbeatInterval)
	defer func() {
		stopHeartbeat()
		releaseCtx, cancel := context.WithTimeout(context.Background(), defaultLockReleaseTimeout)
		defer cancel()
		_ = sourceLock.Release(releaseCtx)
	}()

	ledgerStore, closeLedger, err := openLedgerStore(ctx, cfg, workDir)
	if err != nil {
		return result, err
	}
	defer closeLedger()

	fileTask, ok, err := taskfile.SelectNextForWorkflow(workDir, taskfile.WorkflowMixedPassV1)
	if err != nil {
		return result, err
	}
	if !ok {
		result.Outcome = OutcomeNoTask
		result.NoTask = true
		result.Message = "no pending runnable tasks"
		return result, nil
	}
	task := taskFromFileTask(fileTask)
	result.Task = task
	result.FileTask = fileTask
	result.selectedFileTask = fileTask

	policy, err := passpolicy.Lookup(fileTask.Workflow, fileTask.Phase)
	if err != nil {
		result.Message = "lookup pass policy failed: " + err.Error()
		return result, err
	}
	result.phasePolicy = policy

	run, err := ledgerStore.CreateRun(ctx, ledger.RunSpec{
		ID:     runID,
		TaskID: fileTask.ID,
		Task:   fileTask.ContextBody,
	})
	if err != nil {
		return result, err
	}
	result.Run = run
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventRunStarted, map[string]any{
		"run_id":  run.ID,
		"task_id": fileTask.ID,
	})
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventTaskSelected, map[string]any{
		"task_id":      fileTask.ID,
		"task":         fileTask.ContextBody,
		"summary":      fileTask.Title,
		"workflow":     fileTask.Workflow,
		"phase":        fileTask.Phase,
		"profile_name": policy.ProfileName,
	})

	paths := newRunPaths(run.ID)
	result.ReceiptRelPath = paths.receiptRel
	result.ReceiptPath = filepath.Join(workDir, paths.receiptRel)
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventRunArtifacts, ledger.RunArtifacts{
		ContextPayloadPath:   paths.contextPayloadRel,
		ContextManifestPath:  paths.contextManifestRel,
		CodexStdoutJSONLPath: paths.stdoutRel,
		CodexStderrPath:      paths.stderrRel,
		LastMessagePath:      paths.lastMessageRel,
		ReceiptPath:          paths.receiptRel,
	})

	preRunDirty, err := cfg.DirtyCapture(ctx, gitConfig(cfg, workDir))
	if err != nil {
		result.PreRunDirty = gitstate.Capture{Kind: gitstate.CaptureKindDirty, CaptureError: err.Error()}
		result.Message = "capture pre-run dirty state failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	result.PreRunDirty = preRunDirty
	if result.PreRunDirty.CaptureError != "" {
		appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventChangedFilesCaptured, map[string]any{
			"pre_run_dirty_files": dirtyFileList(result.PreRunDirty),
			"changed_files":       []string{},
			"capture_error":       result.PreRunDirty.CaptureError,
		})
		result.Message = "capture pre-run dirty state failed: " + result.PreRunDirty.CaptureError
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	if !cfg.AllowPreExistingDirty && hasPreExistingDirty(result.PreRunDirty) {
		appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventChangedFilesCaptured, map[string]any{
			"pre_run_dirty_files": dirtyFileList(result.PreRunDirty),
			"changed_files":       []string{},
			"capture_error":       result.PreRunDirty.CaptureError,
		})
		result.Message = "pre-existing dirty files are present"
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}

	runProfile, err := prompt.LoadRunProfile(workDir, policy.ProfileName)
	if err != nil {
		result.Message = "load run profile failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	versionTimeout := cfg.CodexTimeout
	if versionTimeout <= 0 {
		versionTimeout = codexexec.DefaultVersionTimeout
	}
	codexVersion, err := cfg.CodexVersionDiscoverer(ctx, codexexec.VersionConfig{
		Executable:    cfg.CodexExecutable,
		WorkingDir:    workDir,
		Timeout:       versionTimeout,
		StdoutCap:     cfg.CodexStdoutCap,
		StderrCap:     cfg.CodexStderrCap,
		CommandRunner: codexCommandRunner(cfg.CommandRunner),
	})
	if err != nil {
		result.Message = "discover Codex version failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	invocation, _, err := codexexec.PrepareInvocation(codexexec.InvocationConfig{
		Executable:             cfg.CodexExecutable,
		WorkingDir:             workDir,
		Model:                  cfg.CodexModel,
		ReasoningEffort:        cfg.CodexReasoningEffort,
		Ephemeral:              cfg.CodexEphemeral,
		Sandbox:                cfg.CodexSandbox,
		ApprovalPolicy:         cfg.CodexApprovalPolicy,
		BypassApprovalsSandbox: cfg.CodexBypassApprovalsAndSandbox,
		Artifacts: codexexec.ArtifactPaths{
			StdoutJSONL: paths.stdoutRel,
			Stderr:      paths.stderrRel,
			LastMessage: paths.lastMessageRel,
		},
		CodexVersion:          codexVersion,
		EffectiveConfigSchema: effectiveConfig.Schema,
		EffectiveConfigSHA256: effectiveConfig.SHA256,
	})
	if err != nil {
		result.Message = "prepare Codex invocation failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}

	contextInput := prompt.Input{
		RunID:          run.ID,
		PassID:         run.ID,
		TaskID:         fileTask.ID,
		Task:           fileTask.ContextBody,
		TaskSource:     prompt.SourceContent{Path: fileTask.SourcePath, Content: fileTask.SourceBytes},
		RunProfile:     runProfile,
		RepositoryRoot: workDir,
		ReceiptPath:    paths.receiptRel,
		ArtifactPaths: []prompt.ArtifactPath{
			{Label: "context payload", Path: paths.contextPayloadRel},
			{Label: "context manifest", Path: paths.contextManifestRel},
			{Label: "codex stdout jsonl", Path: paths.stdoutRel},
			{Label: "codex stderr", Path: paths.stderrRel},
			{Label: "codex last message", Path: paths.lastMessageRel},
		},
	}
	contextPayload, err := prompt.BuildContextPayload(contextInput)
	if err != nil {
		result.Message = "build context payload failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	contextManifest, err := prompt.BuildContextManifest(prompt.ContextManifestInput{
		Input:              contextInput,
		ContextPayload:     []byte(contextPayload),
		ContextPayloadPath: paths.contextPayloadRel,
		GeneratedAt:        cfg.Clock(),
		Invocation:         invocation,
	})
	if err != nil {
		result.Message = "build context manifest failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	contextManifestJSON, err := prompt.MarshalContextManifest(contextManifest)
	if err != nil {
		result.Message = "marshal context manifest failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	if err := writeTextFile(filepath.Join(workDir, paths.contextPayloadRel), contextPayload, 0o644); err != nil {
		result.Message = "write context payload artifact failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	if err := writeTextFile(filepath.Join(workDir, paths.contextManifestRel), string(contextManifestJSON), 0o644); err != nil {
		result.Message = "write context manifest artifact failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventContextBuilt, map[string]any{
		"context_payload_path":      paths.contextPayloadRel,
		"context_manifest_path":     paths.contextManifestRel,
		"context_payload_sha256":    contextManifest.ContextPayloadSHA256,
		"context_payload_byte_size": contextManifest.ContextPayloadByteSize,
		"receipt_path":              paths.receiptRel,
		"invocation":                invocation,
	})

	codexResult, err := cfg.CodexRunner(ctx, codexexec.Config{
		Executable:                cfg.CodexExecutable,
		WorkingDir:                workDir,
		Prompt:                    contextPayload,
		Model:                     cfg.CodexModel,
		ReasoningEffort:           cfg.CodexReasoningEffort,
		Ephemeral:                 &cfg.CodexEphemeral,
		Timeout:                   cfg.CodexTimeout,
		StdoutCap:                 cfg.CodexStdoutCap,
		StderrCap:                 cfg.CodexStderrCap,
		Sandbox:                   cfg.CodexSandbox,
		ApprovalPolicy:            cfg.CodexApprovalPolicy,
		BypassApprovalsAndSandbox: cfg.CodexBypassApprovalsAndSandbox,
		Artifacts: codexexec.ArtifactPaths{
			StdoutJSONL: paths.stdoutRel,
			Stderr:      paths.stderrRel,
			LastMessage: paths.lastMessageRel,
		},
		RunID:         run.ID,
		Ledger:        ledgerStore,
		CommandRunner: codexCommandRunner(cfg.CommandRunner),
		OnProgress:    cfg.CodexProgress,
		Provenance:    invocation,
	})
	if err != nil {
		if codexResult.ExitCode == 0 {
			codexResult.ExitCode = -1
		}
		codexResult.Err = err
	}
	result.Codex = codexResult

	captureAndRecordChangedFiles(ctx, cfg, ledgerStore, &result, workDir)

	ensureRunReceipt(ctx, cfg, ledgerStore, &result, receipt.VerdictCompletedWithConcerns, "not_run", nil, "", codexResult.FinalMessage)

	if !codexSucceeded(result.Codex) {
		result.Message = codexFailureMessage(result.Codex)
		return finish(ctx, cfg, ledgerStore, &result, OutcomeCodexFailed, receipt.VerdictCodexFailed, "not_run", "")
	}
	if !receiptMatches(result.Receipt, result.Run.ID, result.Task.ID) {
		result.Message = "prepare run receipt failed"
		if result.ReceiptError != "" {
			result.Message += ": " + result.ReceiptError
		}
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}

	verificationResult, err := cfg.VerificationRunner(ctx, verification.Config{
		WorkingDir:            workDir,
		Commands:              cfg.VerificationCommands,
		MissingCommandsPolicy: cfg.MissingVerificationPolicy,
		Timeout:               cfg.VerificationTimeout,
		StdoutCap:             cfg.VerificationStdoutCap,
		StderrCap:             cfg.VerificationStderrCap,
		RunID:                 run.ID,
		Ledger:                ledgerStore,
		CommandRunner:         verificationCommandRunner(cfg.CommandRunner),
	})
	if err != nil {
		result.Message = "verification runner failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeVerificationFailed, receipt.VerdictVerificationFailed, "failed", "")
	}
	result.Verification = verificationResult
	if verificationResult.LedgerError != nil {
		setLedgerError(&result, verificationResult.LedgerError)
	}

	verificationStatus := verificationStatus(verificationResult)
	if !verificationResult.Passed || verificationResult.Status != verification.StatusPassed {
		result.Message = verificationFailureMessage(verificationResult)
		return finish(ctx, cfg, ledgerStore, &result, OutcomeVerificationFailed, receipt.VerdictVerificationFailed, verificationStatus, "")
	}

	if unchanged, snapshotErr := selectedTaskSnapshotUnchanged(workDir, result.selectedFileTask); !unchanged {
		result.Message = "selected task changed during the pass"
		if snapshotErr != nil {
			result.Message += ": " + snapshotErr.Error()
		}
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, verificationStatus, "")
	}
	if result.PostRunChanged.CaptureError != "" {
		result.Commit = gitStateCaptureRefusal(result.PostRunChanged.CaptureError)
		result.Message = result.Commit.Message
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, verificationStatus, "")
	}
	if !policy.AllowNoChangeSuccess && !hasMeaningfulPreTransitionChanges(result.PreRunDirty, result.PostRunChanged, result.selectedFileTask.SourcePath) {
		result.Commit = commit.Result{
			Status:        commit.StatusRefused,
			RefusalReason: commit.ReasonNoChanges,
			Message:       "there are no meaningful changes to commit",
		}
		result.Message = result.Commit.Message
		return finish(ctx, cfg, ledgerStore, &result, OutcomeNoChanges, receipt.VerdictNoChanges, verificationStatus, "")
	}

	updatedFileTask, err := applyPolicyTransition(workDir, result.selectedFileTask, policy)
	if err != nil {
		result.Message = "update task workflow metadata before commit failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, verificationStatus, "")
	}
	result.FileTask = updatedFileTask
	result.Task = taskFromFileTask(updatedFileTask)
	result.phaseTransitionApplied = true
	captureAndRecordChangedFiles(ctx, cfg, ledgerStore, &result, workDir)
	if result.PostRunChanged.CaptureError != "" {
		result.Commit = gitStateCaptureRefusal(result.PostRunChanged.CaptureError)
		result.Message = result.Commit.Message
		return finish(ctx, cfg, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, verificationStatus, "")
	}

	commitResult, err := cfg.CommitRunner(ctx, commit.Config{
		WorkingDir:               workDir,
		RunID:                    run.ID,
		TaskID:                   task.ID,
		TaskSummary:              taskSummary(task),
		CodexResult:              &result.Codex,
		VerificationResult:       &result.Verification,
		PreRunDirty:              &result.PreRunDirty,
		PostRunChanged:           &result.PostRunChanged,
		AllowPreExistingDirty:    cfg.AllowPreExistingDirty,
		AllowMissingVerification: cfg.AllowMissingVerification,
		GitExecutable:            cfg.GitExecutable,
		Timeout:                  cfg.CommitTimeout,
		StdoutCap:                cfg.CommitStdoutCap,
		StderrCap:                cfg.CommitStderrCap,
		Ledger:                   ledgerStore,
		CommandRunner:            commitCommandRunner(cfg.CommandRunner),
	})
	result.Commit = commitResult
	if err != nil {
		result.Message = "auto-commit gate failed: " + err.Error()
		return finish(ctx, cfg, ledgerStore, &result, OutcomeCommitFailed, receipt.VerdictBlocked, verificationStatus, "")
	}
	if commitResult.LedgerError != nil {
		setLedgerError(&result, commitResult.LedgerError)
	}

	switch commitResult.Status {
	case commit.StatusCommitted:
		result.Message = "committed " + commitResult.CommitSHA
		return finish(ctx, cfg, ledgerStore, &result, OutcomeCommitted, receipt.VerdictCompleted, verificationStatus, commitResult.CommitSHA)
	case commit.StatusRefused:
		outcome, verdict := commitRefusalOutcome(commitResult.RefusalReason)
		result.Message = commitRefusalMessage(commitResult)
		return finish(ctx, cfg, ledgerStore, &result, outcome, verdict, verificationStatus, "")
	case commit.StatusIndeterminate:
		result.Message = nonEmpty(commitResult.Message, "auto-commit outcome is indeterminate")
		return finish(ctx, cfg, ledgerStore, &result, OutcomeCommitFailed, receipt.VerdictBlocked, verificationStatus, "")
	default:
		result.Message = nonEmpty(commitResult.Message, "auto-commit failed")
		return finish(ctx, cfg, ledgerStore, &result, OutcomeCommitFailed, receipt.VerdictBlocked, verificationStatus, "")
	}
}

func normalizeConfig(cfg Config) (Config, string, error) {
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Config{}, "", fmt.Errorf("resolve working directory: %w", err)
		}
		cfg.WorkingDir = wd
	}
	workDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve working directory: %w", err)
	}
	cfg.WorkingDir = workDir
	cfg.CodexExecutable = strings.TrimSpace(cfg.CodexExecutable)
	if cfg.CodexExecutable == "" {
		cfg.CodexExecutable = codexexec.DefaultExecutable
	}
	if strings.TrimSpace(cfg.CodexModel) == "" {
		cfg.CodexModel = codexexec.DefaultModel
	}
	if strings.TrimSpace(cfg.CodexReasoningEffort) == "" {
		cfg.CodexReasoningEffort = codexexec.DefaultReasoningEffort
	}
	if !cfg.CodexEphemeral {
		cfg.CodexEphemeral = true
	}
	model, err := codexexec.NormalizeModel(cfg.CodexModel)
	if err != nil {
		return Config{}, "", fmt.Errorf("run once: %w", err)
	}
	effort, err := codexexec.NormalizeReasoningEffort(cfg.CodexReasoningEffort)
	if err != nil {
		return Config{}, "", fmt.Errorf("run once: %w", err)
	}
	cfg.CodexModel = model
	cfg.CodexReasoningEffort = effort
	cfg.CodexSandbox = strings.TrimSpace(cfg.CodexSandbox)
	if cfg.CodexSandbox == "" {
		cfg.CodexSandbox = defaultCodexSandbox
	}
	cfg.CodexApprovalPolicy = strings.TrimSpace(cfg.CodexApprovalPolicy)
	if cfg.CodexApprovalPolicy == "" {
		cfg.CodexApprovalPolicy = defaultCodexApprovalPolicy
	}
	if cfg.CodexStdoutCap <= 0 {
		cfg.CodexStdoutCap = defaultOutputCap
	}
	if cfg.CodexStderrCap <= 0 {
		cfg.CodexStderrCap = defaultOutputCap
	}
	if cfg.GitStdoutCap <= 0 {
		cfg.GitStdoutCap = defaultOutputCap
	}
	if cfg.GitStderrCap <= 0 {
		cfg.GitStderrCap = defaultOutputCap
	}
	if cfg.VerificationStdoutCap <= 0 {
		cfg.VerificationStdoutCap = defaultOutputCap
	}
	if cfg.VerificationStderrCap <= 0 {
		cfg.VerificationStderrCap = defaultOutputCap
	}
	if cfg.CommitStdoutCap <= 0 {
		cfg.CommitStdoutCap = defaultOutputCap
	}
	if cfg.CommitStderrCap <= 0 {
		cfg.CommitStderrCap = defaultOutputCap
	}
	cfg.GitExecutable = strings.TrimSpace(cfg.GitExecutable)
	if cfg.GitExecutable == "" {
		cfg.GitExecutable = defaultGitExecutable
	}
	if cfg.SourceWriterLockTimeout <= 0 {
		cfg.SourceWriterLockTimeout = defaultLockTimeout
	}
	if cfg.SourceWriterLockHeartbeatInterval <= 0 {
		cfg.SourceWriterLockHeartbeatInterval = defaultHeartbeatInterval(cfg.SourceWriterLockTimeout)
	}
	if cfg.SourceWriterLockHeartbeatInterval >= cfg.SourceWriterLockTimeout {
		cfg.SourceWriterLockHeartbeatInterval = defaultHeartbeatInterval(cfg.SourceWriterLockTimeout)
	}
	if cfg.MissingVerificationPolicy == "" {
		cfg.MissingVerificationPolicy = verification.MissingCommandsFail
	}
	switch cfg.MissingVerificationPolicy {
	case verification.MissingCommandsFail, verification.MissingCommandsPass:
	default:
		return Config{}, "", fmt.Errorf("run once: invalid missing verification policy %q", cfg.MissingVerificationPolicy)
	}
	if cfg.VerificationCommands == nil && cfg.VerificationPlan == nil {
		cfg.VerificationCommands = defaultVerificationCommands(workDir)
	}
	if cfg.VerificationPlan != nil {
		if len(cfg.VerificationCommands) > 0 {
			return Config{}, "", errors.New("run once: flat verification commands and a tiered verification plan cannot both be configured")
		}
		if err := cfg.VerificationPlan.Validate(); err != nil {
			return Config{}, "", fmt.Errorf("run once: %w", err)
		}
		plan := autonomousverification.ClonePlan(*cfg.VerificationPlan)
		cfg.VerificationPlan = &plan
	}
	if cfg.CodexRunner == nil {
		cfg.CodexRunner = codexexec.Run
	}
	if cfg.CodexVersionDiscoverer == nil {
		cfg.CodexVersionDiscoverer = codexexec.DiscoverVersion
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
	if cfg.CommitRunner == nil {
		cfg.CommitRunner = commit.Run
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return cfg, workDir, nil
}

func EffectiveConfig(cfg Config) (Config, error) {
	normalized, _, err := normalizeConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	return normalized, nil
}

func defaultHeartbeatInterval(timeout time.Duration) time.Duration {
	interval := timeout / 3
	if interval < time.Second {
		interval = time.Second
	}
	if interval >= timeout {
		interval = timeout / 2
	}
	if interval <= 0 {
		interval = time.Nanosecond
	}
	return interval
}

func startLockHeartbeat(ctx context.Context, sourceLock *lock.SourceWriter, interval time.Duration) func() {
	if sourceLock == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = time.Second
	}

	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				_ = sourceLock.Heartbeat(heartbeatCtx)
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func openLedgerStore(ctx context.Context, cfg Config, workDir string) (*ledger.Store, func(), error) {
	if cfg.LedgerStore != nil {
		return cfg.LedgerStore, func() {}, nil
	}
	path := strings.TrimSpace(cfg.LedgerPath)
	if path == "" {
		path = filepath.Join(workDir, ".revolvr", "ledger.sqlite")
	}
	store, err := ledger.Open(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return store, func() { _ = store.Close() }, nil
}

func captureAndRecordChangedFiles(ctx context.Context, cfg Config, runs *ledger.Store, result *Result, workDir string) {
	postRunChanged, err := cfg.ChangedCapture(ctx, gitConfig(cfg, workDir))
	if err != nil {
		result.PostRunChanged = gitstate.Capture{Kind: gitstate.CaptureKindChanged, CaptureError: err.Error()}
	} else {
		result.PostRunChanged = postRunChanged
	}
	if result.PostRunChanged.CaptureError != "" && result.changedCaptureError == "" {
		result.changedCaptureError = result.PostRunChanged.CaptureError
	}
	appendEvent(ctx, result, runs, result.Run.ID, ledger.EventChangedFilesCaptured, map[string]any{
		"pre_run_dirty_files": dirtyFileList(result.PreRunDirty),
		"changed_files":       result.PostRunChanged.ChangedFiles,
		"capture_error":       result.PostRunChanged.CaptureError,
	})
}

func selectedTaskSnapshotUnchanged(repositoryRoot string, snapshot taskfile.Task) (bool, error) {
	current, ok, err := taskfile.FindByID(repositoryRoot, snapshot.ID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("selected task %q no longer exists", snapshot.ID)
	}
	if filepath.Clean(current.SourcePath) != filepath.Clean(snapshot.SourcePath) {
		return false, fmt.Errorf("selected task %q moved from %s to %s", snapshot.ID, snapshot.SourcePath, current.SourcePath)
	}
	return bytes.Equal(current.SourceBytes, snapshot.SourceBytes), nil
}

func hasMeaningfulPreTransitionChanges(preRun gitstate.Capture, postRun gitstate.Capture, selectedTaskPath string) bool {
	selectedTaskPath = filepath.Clean(selectedTaskPath)
	preExisting := make(map[string]struct{})
	for _, path := range dirtyFileList(preRun) {
		preExisting[filepath.Clean(path)] = struct{}{}
	}
	for _, path := range changedFiles(postRun) {
		path = filepath.Clean(path)
		if path == selectedTaskPath {
			continue
		}
		if _, existedBeforeRun := preExisting[path]; !existedBeforeRun {
			return true
		}
	}
	return false
}

func gitStateCaptureRefusal(captureError string) commit.Result {
	message := "git state capture failed"
	if strings.TrimSpace(captureError) != "" {
		message += ": " + captureError
	}
	return commit.Result{
		Status:        commit.StatusRefused,
		RefusalReason: commit.ReasonGitStateCaptureFailed,
		Message:       message,
	}
}

func applyPolicyTransition(repositoryRoot string, task taskfile.Task, policy passpolicy.Policy) (taskfile.Task, error) {
	update := taskfile.MetadataUpdate{Status: taskfile.StatusPending}
	if policy.CompletesTask {
		update.Status = taskfile.StatusCompleted
	} else {
		if strings.TrimSpace(policy.NextPhase) == "" {
			return taskfile.Task{}, fmt.Errorf("policy phase %q does not define a next phase", policy.Phase)
		}
		update.Phase = policy.NextPhase
	}
	return taskfile.UpdateMetadataFromSnapshot(repositoryRoot, task, update)
}

func finish(ctx context.Context, cfg Config, runs *ledger.Store, result *Result, outcome Outcome, verdict receipt.Verdict, verificationStatus string, commitSHA string) (Result, error) {
	result.Outcome = outcome
	if strings.TrimSpace(result.Message) == "" {
		result.Message = string(outcome)
	}
	completedAt := cfg.Clock()
	var finishErr error
	var taskUpdateError string
	var taskRestageError string
	taskRestageApplied := false
	if result.selectedFileTask.SourcePath != "" && outcome != OutcomeCommitted {
		updatedFileTask, err := blockTaskAfterFailedRun(cfg.WorkingDir, result)
		if err != nil {
			taskUpdateError = err.Error()
			finishErr = errors.Join(finishErr, fmt.Errorf("update task after failed run: %w", err))
			result.Message += "; update task after failed run failed: " + err.Error()
			if current, loadErr := taskfile.Load(cfg.WorkingDir, result.selectedFileTask.SourcePath); loadErr == nil {
				result.FileTask = current
			}
		} else {
			result.FileTask = updatedFileTask
			if commitStagedChanges(result.Commit) {
				if restageErr := stageRestoredTask(ctx, cfg, result.selectedFileTask.SourcePath); restageErr != nil {
					taskRestageError = restageErr.Error()
					finishErr = errors.Join(finishErr, fmt.Errorf("stage restored task after commit failure: %w", restageErr))
					result.Message += "; stage restored task after commit failure failed: " + restageErr.Error()
				} else {
					taskRestageApplied = true
				}
			}
		}
		captureAndRecordChangedFiles(ctx, cfg, runs, result, cfg.WorkingDir)
	}

	if result.FileTask.SourcePath != "" {
		result.Task = taskFromFileTask(result.FileTask)
	}
	if result.FileTask.Status == taskfile.StatusBlocked {
		result.Task.Blocker = result.Message
		blockedAt := completedAt
		result.Task.BlockedAt = &blockedAt
	} else if result.FileTask.Status == taskfile.StatusCompleted {
		taskCompletedAt := completedAt
		result.Task.CompletedAt = &taskCompletedAt
	}
	result.Task.UpdatedAt = completedAt

	parsedReceipt := result.Receipt
	receiptWasSynthesized := result.ReceiptSynthesized
	entries := verificationEntries(result.Verification)
	files := changedFiles(result.PostRunChanged)
	finalizeRunReceipt(ctx, cfg, runs, result, verdict, verificationStatus, entries, commitSHA, result.Message, completedAt)
	if !receiptWasSynthesized && !result.ReceiptSynthesized {
		recordReceiptWarnings(ctx, runs, result, parsedReceipt, verdict, verificationStatus, entries, files)
	}

	runStatus := ledger.StatusFailed
	eventType := ledger.EventRunFailed
	if outcome == OutcomeCommitted {
		runStatus = ledger.StatusCompleted
		eventType = ledger.EventRunCompleted
	}
	exitCode := result.Codex.ExitCode
	updatedRun, ok, completionErr := runs.CompleteRun(ctx, result.Run.ID, ledger.RunCompletion{
		Status:             runStatus,
		Summary:            result.Message,
		CompletedAt:        completedAt,
		CodexExitCode:      &exitCode,
		VerificationStatus: verificationStatus,
		CommitSHA:          commitSHA,
	})
	if completionErr != nil {
		setLedgerError(result, completionErr)
		finishErr = errors.Join(finishErr, completionErr)
	} else if !ok {
		completionErr = fmt.Errorf("complete run: run %q not found", result.Run.ID)
		setLedgerError(result, completionErr)
		finishErr = errors.Join(finishErr, completionErr)
	} else {
		result.Run = updatedRun
	}

	payload := map[string]any{
		"outcome":                outcome,
		"message":                result.Message,
		"task_status":            result.Task.Status,
		"task_phase":             result.FileTask.Phase,
		"run_status":             result.Run.Status,
		"codex_exit_code":        result.Codex.ExitCode,
		"verification_status":    verificationStatus,
		"commit_sha":             commitSHA,
		"commit_pre_head":        result.Commit.PreCommitSHA,
		"commit_post_head":       result.Commit.PostCommitSHA,
		"commit_head_retry":      result.Commit.HEADLookupRetried,
		"commit_status":          result.Commit.Status,
		"commit_refusal":         result.Commit.RefusalReason,
		"commit_message":         result.Commit.Message,
		"changed_files":          files,
		"capture_error":          result.changedCaptureError,
		"pre_run_capture_error":  result.PreRunDirty.CaptureError,
		"receipt_verdict":        verdict,
		"receipt_actual_verdict": result.Receipt.Verdict,
		"receipt_synthesized":    result.ReceiptSynthesized,
		"receipt_error":          result.ReceiptError,
		"task_update_error":      taskUpdateError,
		"task_restage_applied":   taskRestageApplied,
		"task_restage_error":     taskRestageError,
	}
	if completionErr != nil {
		payload["run_completion_error"] = completionErr.Error()
	}
	if result.phasePolicy.Workflow != "" {
		payload["workflow"] = result.phasePolicy.Workflow
		payload["phase"] = result.phasePolicy.Phase
		payload["next_phase"] = result.phasePolicy.NextPhase
		payload["completes_task"] = result.phasePolicy.CompletesTask
		payload["allow_no_change_success"] = result.phasePolicy.AllowNoChangeSuccess
		payload["phase_transition_applied"] = result.phaseTransitionApplied
	}
	appendEvent(ctx, result, runs, result.Run.ID, eventType, payload)
	return *result, finishErr
}

func blockTaskAfterFailedRun(repositoryRoot string, result *Result) (taskfile.Task, error) {
	snapshot := result.selectedFileTask
	if result.Commit.Status == commit.StatusIndeterminate && result.phaseTransitionApplied {
		snapshot = result.FileTask
	}
	return taskfile.UpdateMetadataFromSnapshot(repositoryRoot, snapshot, taskfile.MetadataUpdate{
		Status: taskfile.StatusBlocked,
	})
}

func commitStagedChanges(result commit.Result) bool {
	for _, command := range result.Commands {
		if len(command.Args) == 0 || command.Args[0] != "add" {
			continue
		}
		return command.Error == "" && !command.TimedOut && command.ExitCode == 0
	}
	return false
}

func stageRestoredTask(ctx context.Context, cfg Config, sourcePath string) error {
	commandRunner := cfg.CommandRunner
	if commandRunner == nil {
		commandRunner = runner.Run
	}
	executable := strings.TrimSpace(cfg.GitExecutable)
	if executable == "" {
		executable = "git"
	}
	timeout := cfg.GitTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	result := commandRunner(ctx, runner.Command{
		Name:        executable,
		Args:        []string{"add", "--", sourcePath},
		Dir:         cfg.WorkingDir,
		Timeout:     timeout,
		StdoutLimit: cfg.GitStdoutCap,
		StderrLimit: cfg.GitStderrCap,
	})
	if result.Err != nil {
		return result.Err
	}
	if result.TimedOut {
		return errors.New("git add restored task timed out")
	}
	if result.ExitCode != 0 {
		message := fmt.Sprintf("git add restored task exited with code %d", result.ExitCode)
		if stderr := strings.TrimSpace(result.Stderr); stderr != "" {
			message += ": " + stderr
		}
		return errors.New(message)
	}
	return nil
}

func ensureRunReceipt(ctx context.Context, cfg Config, runs *ledger.Store, result *Result, verdict receipt.Verdict, verificationStatus string, verificationEntries []receipt.VerificationEntry, commitSHA string, finalText string) {
	if result.ReceiptPath == "" {
		return
	}
	if !result.ReceiptSynthesized {
		parsed, parseErr := parseReceiptFile(result.ReceiptPath, result.Codex.Artifacts.StdoutJSONL)
		if parseErr == nil && receiptMatches(parsed, result.Run.ID, result.Task.ID) {
			result.Receipt = parsed
			appendEvent(ctx, result, runs, result.Run.ID, ledger.EventReceiptParsed, map[string]any{
				"receipt_path": result.ReceiptRelPath,
				"verdict":      parsed.Verdict,
			})
			return
		}
		if parseErr != nil && !errors.Is(parseErr, os.ErrNotExist) {
			result.ReceiptError = parseErr.Error()
		}
		if parseErr == nil {
			result.ReceiptError = "receipt identifiers did not match the selected run"
		}
	}

	writeFallbackReceipt(ctx, runs, result, verdict, verificationStatus, verificationEntries, commitSHA, finalText, cfg.Clock())
}

func finalizeRunReceipt(ctx context.Context, cfg Config, runs *ledger.Store, result *Result, verdict receipt.Verdict, verificationStatus string, verificationEntries []receipt.VerificationEntry, commitSHA string, finalText string, timestamp time.Time) {
	if result.ReceiptPath == "" {
		return
	}
	metrics := result.Receipt.Metrics
	if result.Codex.UsageFound {
		metrics = result.Codex.Usage
	}

	content, err := os.ReadFile(result.ReceiptPath)
	if err == nil {
		updated, parsed, changed, rewriteErr := receipt.RewriteHarnessFields(content, receipt.HarnessFields{
			Timestamp:          timestamp,
			Verdict:            verdict,
			CodexExitCode:      result.Codex.ExitCode,
			VerificationStatus: verificationStatus,
			CommitSHA:          commitSHA,
			ChangedFiles:       changedFiles(result.PostRunChanged),
			Verification:       verificationEntries,
			Metrics:            metrics,
		})
		if rewriteErr == nil && receiptMatches(parsed, result.Run.ID, result.Task.ID) {
			if changed {
				if writeErr := writeTextFile(result.ReceiptPath, string(updated), 0o644); writeErr != nil {
					result.ReceiptError = writeErr.Error()
					return
				}
			}
			result.Receipt = parsed
			return
		}
		if rewriteErr != nil {
			result.ReceiptError = rewriteErr.Error()
		} else {
			result.ReceiptError = "receipt identifiers did not match the selected run"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		result.ReceiptError = err.Error()
	}

	writeFallbackReceipt(ctx, runs, result, verdict, verificationStatus, verificationEntries, commitSHA, finalText, timestamp)
}

func writeFallbackReceipt(ctx context.Context, runs *ledger.Store, result *Result, verdict receipt.Verdict, verificationStatus string, verificationEntries []receipt.VerificationEntry, commitSHA string, finalText string, timestamp time.Time) {
	content, parsed := receipt.FormatFallbackReceipt(receipt.FallbackInput{
		RunID:              result.Run.ID,
		PassID:             result.Run.ID,
		TaskID:             result.Task.ID,
		Task:               result.Task.Task,
		Verdict:            verdict,
		Timestamp:          timestamp,
		CodexExitCode:      result.Codex.ExitCode,
		VerificationStatus: verificationStatus,
		CommitSHA:          commitSHA,
		ChangedFiles:       changedFiles(result.PostRunChanged),
		Verification:       verificationEntries,
		Metrics:            result.Codex.Usage,
		FinalText:          finalText,
	})
	if err := writeTextFile(result.ReceiptPath, content, 0o644); err != nil {
		result.ReceiptError = err.Error()
		return
	}
	result.Receipt = parsed
	result.ReceiptSynthesized = true
	appendEvent(ctx, result, runs, result.Run.ID, ledger.EventReceiptSynthesized, map[string]any{
		"receipt_path": result.ReceiptRelPath,
		"verdict":      parsed.Verdict,
		"reason":       result.ReceiptError,
	})
}

func parseReceiptFile(path string, codexJSONLPath string) (receipt.Receipt, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return receipt.Receipt{}, err
	}
	parsed, err := receipt.Parse(content)
	if err != nil {
		return receipt.Receipt{}, err
	}
	if strings.TrimSpace(codexJSONLPath) == "" {
		return parsed, nil
	}
	jsonl, err := os.ReadFile(codexJSONLPath)
	if err != nil {
		return parsed, nil
	}
	updated, reparsed, changed, err := receipt.RewriteMetricsFromCodexJSONL(content, jsonl)
	if err != nil {
		return parsed, nil
	}
	if changed {
		if writeErr := writeTextFile(path, string(updated), 0o644); writeErr != nil {
			return receipt.Receipt{}, writeErr
		}
	}
	return reparsed, nil
}

func appendEvent(ctx context.Context, result *Result, runs *ledger.Store, runID string, eventType ledger.EventType, payload any) {
	if runs == nil || strings.TrimSpace(runID) == "" {
		return
	}
	_, err := runs.AppendEvent(ctx, runID, eventType, payload)
	setLedgerError(result, err)
}

func setLedgerError(result *Result, err error) {
	if err != nil && result.LedgerError == nil {
		result.LedgerError = err
	}
}

type runPaths struct {
	runDirRel          string
	contextPayloadRel  string
	contextManifestRel string
	stdoutRel          string
	stderrRel          string
	lastMessageRel     string
	receiptRel         string
}

func newRunPaths(runID string) runPaths {
	runDir := filepath.Join(".revolvr", "runs", runID)
	return runPaths{
		runDirRel:          runDir,
		contextPayloadRel:  filepath.Join(runDir, "context.md"),
		contextManifestRel: filepath.Join(runDir, "context.json"),
		stdoutRel:          filepath.Join(runDir, "codex.jsonl"),
		stderrRel:          filepath.Join(runDir, "codex.stderr"),
		lastMessageRel:     filepath.Join(runDir, "last-message.txt"),
		receiptRel:         filepath.Join(".revolvr", "receipts", runID+".md"),
	}
}

func gitConfig(cfg Config, workDir string) gitstate.Config {
	return gitstate.Config{
		WorkingDir:    workDir,
		GitExecutable: cfg.GitExecutable,
		Timeout:       cfg.GitTimeout,
		StdoutCap:     cfg.GitStdoutCap,
		StderrCap:     cfg.GitStderrCap,
		CommandRunner: gitCommandRunner(cfg.CommandRunner),
	}
}

func defaultVerificationCommands(workDir string) []verification.Command {
	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); err == nil {
		return []verification.Command{{Name: "go", Args: []string{"test", "./..."}}}
	}
	return nil
}

func codexSucceeded(result codexexec.Result) bool {
	return result.Err == nil && !result.TimedOut && result.ExitCode == 0
}

func codexFailureMessage(result codexexec.Result) string {
	switch {
	case result.TimedOut:
		return "Codex timed out"
	case result.Err != nil:
		return "Codex failed: " + result.Err.Error()
	case result.ExitCode != 0:
		return fmt.Sprintf("Codex exited with code %d", result.ExitCode)
	default:
		return "Codex did not complete successfully"
	}
}

func verificationStatus(result verification.Result) string {
	if result.MissingCommands {
		return "missing"
	}
	if result.Status == verification.StatusPassed && result.Passed {
		return "passed"
	}
	return "failed"
}

func verificationFailureMessage(result verification.Result) string {
	if result.MissingCommands {
		return "verification commands are missing"
	}
	if result.Message != "" {
		return result.Message
	}
	return "verification failed"
}

func commitRefusalOutcome(reason commit.RefusalReason) (Outcome, receipt.Verdict) {
	switch reason {
	case commit.ReasonNoChanges:
		return OutcomeNoChanges, receipt.VerdictNoChanges
	case commit.ReasonVerificationFailed, commit.ReasonVerificationCommandsMissing:
		return OutcomeVerificationFailed, receipt.VerdictVerificationFailed
	default:
		return OutcomeBlocked, receipt.VerdictBlocked
	}
}

func commitRefusalMessage(result commit.Result) string {
	if result.Message != "" {
		return result.Message
	}
	if result.RefusalReason != "" {
		return "auto-commit refused: " + string(result.RefusalReason)
	}
	return "auto-commit refused"
}

func taskFromFileTask(task taskfile.Task) taskmodel.Task {
	return taskmodel.Task{
		ID:      task.ID,
		Task:    task.ContextBody,
		Status:  task.Status,
		Summary: task.Title,
	}
}

func taskSummary(task taskmodel.Task) string {
	if summary := singleLine(task.Summary); summary != "" {
		return summary
	}
	return truncateRunes(singleLine(task.Task), 72)
}

func receiptMatches(parsed receipt.Receipt, runID string, taskID string) bool {
	return parsed.RunID == runID && parsed.PassID == runID && parsed.TaskID == taskID
}

func verificationEntries(result verification.Result) []receipt.VerificationEntry {
	entries := make([]receipt.VerificationEntry, 0, len(result.Commands))
	for _, command := range result.Commands {
		entries = append(entries, receipt.VerificationEntry{
			Command:  command.Command,
			ExitCode: command.ExitCode,
			Status:   string(command.Status),
		})
	}
	return entries
}

func changedFiles(capture gitstate.Capture) []string {
	if len(capture.ChangedFiles) > 0 {
		return append([]string(nil), capture.ChangedFiles...)
	}
	return append([]string(nil), capture.Paths...)
}

func hasPreExistingDirty(capture gitstate.Capture) bool {
	return len(capture.DirtyFiles) > 0 || len(capture.Paths) > 0
}

func dirtyFileList(capture gitstate.Capture) []string {
	if len(capture.DirtyFiles) > 0 {
		return append([]string(nil), capture.DirtyFiles...)
	}
	return append([]string(nil), capture.Paths...)
}

func writeTextFile(path string, content string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), perm)
}

func nonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func codexCommandRunner(fn CommandRunner) codexexec.CommandRunner {
	if fn == nil {
		return nil
	}
	return codexexec.CommandRunner(fn)
}

func gitCommandRunner(fn CommandRunner) gitstate.CommandRunner {
	if fn == nil {
		return nil
	}
	return gitstate.CommandRunner(fn)
}

func verificationCommandRunner(fn CommandRunner) verification.CommandRunner {
	if fn == nil {
		return nil
	}
	return verification.CommandRunner(fn)
}

func commitCommandRunner(fn CommandRunner) commit.CommandRunner {
	if fn == nil {
		return nil
	}
	return commit.CommandRunner(fn)
}
