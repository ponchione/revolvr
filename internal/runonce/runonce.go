package runonce

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/id"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/taskqueue"
	"revolvr/internal/verification"
)

const (
	defaultCodexSandbox        = "workspace-write"
	defaultCodexApprovalPolicy = "never"
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
type DirtyCapture func(context.Context, gitstate.Config) (gitstate.Capture, error)
type ChangedCapture func(context.Context, gitstate.Config) (gitstate.Capture, error)
type VerificationRunner func(context.Context, verification.Config) (verification.Result, error)
type CommitRunner func(context.Context, commit.Config) (commit.Result, error)

type Config struct {
	WorkingDir string

	TaskStore   *taskqueue.Store
	LedgerStore *ledger.Store
	TaskDBPath  string
	LedgerPath  string

	CodexExecutable                string
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

	CommandRunner      CommandRunner
	CodexRunner        CodexRunner
	DirtyCapture       DirtyCapture
	ChangedCapture     ChangedCapture
	VerificationRunner VerificationRunner
	CommitRunner       CommitRunner
	Clock              func() time.Time
	CodexProgress      func(codexexec.ProgressEvent)
}

type Result struct {
	Outcome            Outcome
	Message            string
	WorkingDir         string
	NoTask             bool
	Task               taskqueue.Task
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
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	cfg, workDir, err := normalizeConfig(cfg)
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

	taskStore, closeTask, err := openTaskStore(ctx, cfg, workDir)
	if err != nil {
		return result, err
	}
	defer closeTask()

	ledgerStore, closeLedger, err := openLedgerStore(ctx, cfg, workDir)
	if err != nil {
		return result, err
	}
	defer closeLedger()

	task, ok, err := taskStore.SelectNext(ctx)
	if err != nil {
		return result, err
	}
	if !ok {
		result.Outcome = OutcomeNoTask
		result.NoTask = true
		result.Message = "no pending runnable tasks"
		return result, nil
	}
	result.Task = task

	run, err := ledgerStore.CreateRun(ctx, ledger.RunSpec{
		ID:     runID,
		TaskID: task.ID,
		Task:   task.Task,
	})
	if err != nil {
		return result, err
	}
	result.Run = run
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventRunStarted, map[string]any{
		"run_id":  run.ID,
		"task_id": task.ID,
	})
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventTaskSelected, map[string]any{
		"task_id": task.ID,
		"task":    task.Task,
		"summary": task.Summary,
	})

	paths := newRunPaths(run.ID)
	result.ReceiptRelPath = paths.receiptRel
	result.ReceiptPath = filepath.Join(workDir, paths.receiptRel)
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventRunArtifacts, ledger.RunArtifacts{
		PromptPath:           paths.promptRel,
		CodexStdoutJSONLPath: paths.stdoutRel,
		CodexStderrPath:      paths.stderrRel,
		LastMessagePath:      paths.lastMessageRel,
		ReceiptPath:          paths.receiptRel,
	})

	preRunDirty, err := cfg.DirtyCapture(ctx, gitConfig(cfg, workDir))
	if err != nil {
		result.PreRunDirty = gitstate.Capture{Kind: gitstate.CaptureKindDirty, CaptureError: err.Error()}
		result.Message = "capture pre-run dirty state failed: " + err.Error()
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	result.PreRunDirty = preRunDirty
	if !cfg.AllowPreExistingDirty && hasPreExistingDirty(result.PreRunDirty) {
		appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventChangedFilesCaptured, map[string]any{
			"pre_run_dirty_files": dirtyFileList(result.PreRunDirty),
			"changed_files":       []string{},
			"capture_error":       result.PreRunDirty.CaptureError,
		})
		result.Message = "pre-existing dirty files are present"
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}

	promptText, err := prompt.Build(prompt.Input{
		RunID:          run.ID,
		PassID:         run.ID,
		TaskID:         task.ID,
		Task:           task.Task,
		RepositoryRoot: workDir,
		ReceiptPath:    paths.receiptRel,
		ArtifactPaths: []prompt.ArtifactPath{
			{Label: "prompt", Path: paths.promptRel},
			{Label: "codex stdout jsonl", Path: paths.stdoutRel},
			{Label: "codex stderr", Path: paths.stderrRel},
			{Label: "codex last message", Path: paths.lastMessageRel},
		},
	})
	if err != nil {
		result.Message = "build prompt failed: " + err.Error()
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	if err := writeTextFile(filepath.Join(workDir, paths.promptRel), promptText, 0o644); err != nil {
		result.Message = "write prompt artifact failed: " + err.Error()
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeBlocked, receipt.VerdictBlocked, "not_run", "")
	}
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventPromptBuilt, map[string]any{
		"prompt_path":  paths.promptRel,
		"receipt_path": paths.receiptRel,
		"bytes":        len(promptText),
	})

	codexResult, err := cfg.CodexRunner(ctx, codexexec.Config{
		Executable:                cfg.CodexExecutable,
		WorkingDir:                workDir,
		Prompt:                    promptText,
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
	})
	if err != nil {
		if codexResult.ExitCode == 0 {
			codexResult.ExitCode = -1
		}
		codexResult.Err = err
	}
	result.Codex = codexResult

	postRunChanged, err := cfg.ChangedCapture(ctx, gitConfig(cfg, workDir))
	if err != nil {
		result.PostRunChanged = gitstate.Capture{Kind: gitstate.CaptureKindChanged, CaptureError: err.Error()}
	} else {
		result.PostRunChanged = postRunChanged
	}
	appendEvent(ctx, &result, ledgerStore, run.ID, ledger.EventChangedFilesCaptured, map[string]any{
		"pre_run_dirty_files": result.PreRunDirty.DirtyFiles,
		"changed_files":       result.PostRunChanged.ChangedFiles,
		"capture_error":       result.PostRunChanged.CaptureError,
	})

	ensureRunReceipt(ctx, cfg, ledgerStore, &result, receipt.VerdictCompletedWithConcerns, "not_run", nil, "", codexResult.FinalMessage)

	if !codexSucceeded(result.Codex) {
		result.Message = codexFailureMessage(result.Codex)
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeCodexFailed, receipt.VerdictCodexFailed, "not_run", "")
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
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeVerificationFailed, receipt.VerdictVerificationFailed, "failed", "")
	}
	result.Verification = verificationResult
	if verificationResult.LedgerError != nil {
		setLedgerError(&result, verificationResult.LedgerError)
	}

	verificationStatus := verificationStatus(verificationResult)
	if !verificationResult.Passed || verificationResult.Status != verification.StatusPassed {
		result.Message = verificationFailureMessage(verificationResult)
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeVerificationFailed, receipt.VerdictVerificationFailed, verificationStatus, "")
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
	if err != nil {
		result.Message = "auto-commit gate failed: " + err.Error()
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeCommitFailed, receipt.VerdictBlocked, verificationStatus, "")
	}
	result.Commit = commitResult
	if commitResult.LedgerError != nil {
		setLedgerError(&result, commitResult.LedgerError)
	}

	switch commitResult.Status {
	case commit.StatusCommitted:
		result.Message = "committed " + commitResult.CommitSHA
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeCommitted, receipt.VerdictCompleted, verificationStatus, commitResult.CommitSHA)
	case commit.StatusRefused:
		outcome, verdict := commitRefusalOutcome(commitResult.RefusalReason)
		result.Message = commitRefusalMessage(commitResult)
		return finish(ctx, cfg, taskStore, ledgerStore, &result, outcome, verdict, verificationStatus, "")
	default:
		result.Message = nonEmpty(commitResult.Message, "auto-commit failed")
		return finish(ctx, cfg, taskStore, ledgerStore, &result, OutcomeCommitFailed, receipt.VerdictBlocked, verificationStatus, "")
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
	if cfg.VerificationCommands == nil {
		cfg.VerificationCommands = defaultVerificationCommands(workDir)
	}
	if cfg.CodexRunner == nil {
		cfg.CodexRunner = codexexec.Run
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

func openTaskStore(ctx context.Context, cfg Config, workDir string) (*taskqueue.Store, func(), error) {
	if cfg.TaskStore != nil {
		return cfg.TaskStore, func() {}, nil
	}
	path := strings.TrimSpace(cfg.TaskDBPath)
	if path == "" {
		path = filepath.Join(workDir, ".revolvr", "tasks.sqlite")
	}
	store, err := taskqueue.Open(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return store, func() { _ = store.Close() }, nil
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

func finish(ctx context.Context, cfg Config, tasks *taskqueue.Store, runs *ledger.Store, result *Result, outcome Outcome, verdict receipt.Verdict, verificationStatus string, commitSHA string) (Result, error) {
	result.Outcome = outcome
	if strings.TrimSpace(result.Message) == "" {
		result.Message = string(outcome)
	}
	parsedReceipt := result.Receipt
	receiptWasSynthesized := result.ReceiptSynthesized
	entries := verificationEntries(result.Verification)
	files := changedFiles(result.PostRunChanged)
	completedAt := cfg.Clock()
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
	updatedRun, ok, err := runs.CompleteRun(ctx, result.Run.ID, ledger.RunCompletion{
		Status:             runStatus,
		Summary:            result.Message,
		CompletedAt:        completedAt,
		CodexExitCode:      &exitCode,
		VerificationStatus: verificationStatus,
		CommitSHA:          commitSHA,
	})
	if err != nil {
		return *result, err
	}
	if ok {
		result.Run = updatedRun
	}

	if outcome == OutcomeCommitted {
		updatedTask, ok, err := tasks.CompleteTask(ctx, result.Task.ID, result.Message)
		if err != nil {
			return *result, err
		}
		if ok {
			result.Task = updatedTask
		}
	} else {
		updatedTask, ok, err := tasks.BlockTask(ctx, result.Task.ID, result.Message)
		if err != nil {
			return *result, err
		}
		if ok {
			result.Task = updatedTask
		}
	}

	appendEvent(ctx, result, runs, result.Run.ID, eventType, map[string]any{
		"outcome":             outcome,
		"message":             result.Message,
		"task_status":         result.Task.Status,
		"run_status":          result.Run.Status,
		"codex_exit_code":     result.Codex.ExitCode,
		"verification_status": verificationStatus,
		"commit_sha":          commitSHA,
		"commit_status":       result.Commit.Status,
		"commit_refusal":      result.Commit.RefusalReason,
		"commit_message":      result.Commit.Message,
	})
	return *result, nil
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
	runDirRel      string
	promptRel      string
	stdoutRel      string
	stderrRel      string
	lastMessageRel string
	receiptRel     string
}

func newRunPaths(runID string) runPaths {
	runDir := filepath.Join(".revolvr", "runs", runID)
	return runPaths{
		runDirRel:      runDir,
		promptRel:      filepath.Join(runDir, "prompt.md"),
		stdoutRel:      filepath.Join(runDir, "codex.jsonl"),
		stderrRel:      filepath.Join(runDir, "codex.stderr"),
		lastMessageRel: filepath.Join(runDir, "last-message.txt"),
		receiptRel:     filepath.Join(".revolvr", "receipts", runID+".md"),
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

func taskSummary(task taskqueue.Task) string {
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
