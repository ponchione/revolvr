package commit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/runner"
	"revolvr/internal/verification"
)

const (
	defaultGitExecutable = "git"
	defaultTimeout       = 30 * time.Second
)

type CommandRunner func(context.Context, runner.Command) runner.Result

type Ledger interface {
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
}

type CommitRecorder interface {
	RecordCommitSHA(context.Context, string, string) error
}

type Status string

const (
	StatusCommitted     Status = "committed"
	StatusRefused       Status = "refused"
	StatusFailed        Status = "failed"
	StatusIndeterminate Status = "indeterminate"
)

type RefusalReason string

const (
	ReasonCodexFailed                 RefusalReason = "codex_failed"
	ReasonVerificationFailed          RefusalReason = "verification_failed"
	ReasonVerificationCommandsMissing RefusalReason = "verification_commands_missing"
	ReasonNoChanges                   RefusalReason = "no_changes"
	ReasonPreExistingDirty            RefusalReason = "pre_existing_dirty"
	ReasonGitStateCaptureFailed       RefusalReason = "git_state_capture_failed"
)

type Config struct {
	WorkingDir               string
	RunID                    string
	TaskID                   string
	TaskSummary              string
	CodexResult              *codexexec.Result
	VerificationResult       *verification.Result
	PreRunDirty              *gitstate.Capture
	PostRunChanged           *gitstate.Capture
	AllowPreExistingDirty    bool
	AllowMissingVerification bool
	GitExecutable            string
	Timeout                  time.Duration
	StdoutCap                int
	StderrCap                int
	Ledger                   Ledger
	CommitRecorder           CommitRecorder
	CommandRunner            CommandRunner
}

type GitCommandResult struct {
	Command   string
	Name      string
	Args      []string
	ExitCode  int
	TimedOut  bool
	Error     string
	Stdout    string
	Stderr    string
	StartedAt time.Time
	EndedAt   time.Time
}

type Result struct {
	Status                Status
	CommitSHA             string
	PreCommitSHA          string
	PostCommitSHA         string
	HEADLookupRetried     bool
	Message               string
	RefusalReason         RefusalReason
	ChangedFiles          []string
	PreExistingDirtyFiles []string
	Commands              []GitCommandResult
	LedgerError           error
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	cfg, workDir, err := normalizeConfig(cfg)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		ChangedFiles:          changedFiles(cfg.PostRunChanged),
		PreExistingDirtyFiles: dirtyFiles(cfg.PreRunDirty),
	}

	if reason, message := refusalReason(cfg, result); reason != "" {
		result.Status = StatusRefused
		result.RefusalReason = reason
		result.Message = message
		return result, nil
	}

	var ledgerErr error
	appendLedger := func(eventType ledger.EventType, payload any) {
		if cfg.Ledger == nil {
			return
		}
		if _, err := cfg.Ledger.AppendEvent(ctx, cfg.RunID, eventType, payload); err != nil && ledgerErr == nil {
			ledgerErr = err
		}
	}

	appendLedger(ledger.EventCommitStarted, map[string]any{
		"run_id":        cfg.RunID,
		"task_id":       cfg.TaskID,
		"task_summary":  cfg.TaskSummary,
		"changed_files": result.ChangedFiles,
	})

	preCommitHEAD, preCommitResult, err := resolveHEAD(ctx, cfg, workDir)
	result.Commands = append(result.Commands, preCommitResult)
	if err != nil {
		result.Status = StatusFailed
		result.Message = "resolve HEAD before commit failed"
		result.LedgerError = ledgerErr
		return result, nil
	}
	result.PreCommitSHA = preCommitHEAD.SHA

	stageResult := runGit(ctx, cfg, workDir, append([]string{"add", "--"}, result.ChangedFiles...))
	result.Commands = append(result.Commands, stageResult)
	if !commandPassed(stageResult) {
		result.Status = StatusFailed
		result.Message = "git add failed"
		result.LedgerError = ledgerErr
		return result, nil
	}

	subject, body := commitMessageParts(cfg.TaskSummary, cfg.RunID, cfg.TaskID)
	commitResult := runGit(ctx, cfg, workDir, []string{"commit", "-m", subject, "-m", body})
	result.Commands = append(result.Commands, commitResult)

	postCommitHEAD, postCommitResults, retried, err := resolveHEADAfterCommit(ctx, cfg, workDir)
	result.Commands = append(result.Commands, postCommitResults...)
	result.HEADLookupRetried = retried
	if err != nil {
		result.Status = StatusIndeterminate
		result.Message = "git commit outcome is indeterminate: resolve HEAD after commit failed"
		result.LedgerError = ledgerErr
		return result, nil
	}
	result.PostCommitSHA = postCommitHEAD.SHA

	if !headAdvanced(preCommitHEAD, postCommitHEAD) {
		result.Status = StatusFailed
		if commandPassed(commitResult) {
			result.Message = "git commit reported success but HEAD did not advance"
		} else {
			result.Message = "git commit failed"
		}
		result.LedgerError = ledgerErr
		return result, nil
	}

	sha := postCommitHEAD.SHA

	result.Status = StatusCommitted
	result.CommitSHA = sha
	result.Message = "commit created"
	if !commandPassed(commitResult) {
		result.Message = "commit created despite git commit command failure"
	} else if retried {
		result.Message = "commit created after reconciling HEAD"
	}

	recorder := cfg.CommitRecorder
	if recorder == nil {
		if ledgerRecorder, ok := cfg.Ledger.(CommitRecorder); ok {
			recorder = ledgerRecorder
		}
	}
	if recorder != nil {
		if err := recorder.RecordCommitSHA(ctx, cfg.RunID, sha); err != nil && ledgerErr == nil {
			ledgerErr = err
		}
	}

	appendLedger(ledger.EventCommitCreated, map[string]any{
		"run_id":              cfg.RunID,
		"task_id":             cfg.TaskID,
		"commit_sha":          sha,
		"pre_commit_sha":      result.PreCommitSHA,
		"head_lookup_retried": result.HEADLookupRetried,
		"changed_files":       result.ChangedFiles,
		"message": map[string]any{
			"subject": subject,
			"body":    body,
		},
	})
	result.LedgerError = ledgerErr
	return result, nil
}

type headState struct {
	SHA    string
	Exists bool
}

func resolveHEAD(ctx context.Context, cfg Config, workDir string) (headState, GitCommandResult, error) {
	result := runGit(ctx, cfg, workDir, []string{"rev-parse", "--verify", "--quiet", "HEAD"})
	if commandPassed(result) {
		sha := strings.TrimSpace(result.Stdout)
		if sha == "" {
			return headState{}, result, errors.New("git rev-parse returned an empty commit SHA")
		}
		return headState{SHA: sha, Exists: true}, result, nil
	}
	if result.Error == "" && !result.TimedOut && result.ExitCode == 1 && strings.TrimSpace(result.Stdout) == "" {
		return headState{}, result, nil
	}
	return headState{}, result, errors.New("git rev-parse failed")
}

func resolveHEADAfterCommit(ctx context.Context, cfg Config, workDir string) (headState, []GitCommandResult, bool, error) {
	head, first, err := resolveHEAD(ctx, cfg, workDir)
	results := []GitCommandResult{first}
	if err == nil {
		return head, results, false, nil
	}

	head, retry, retryErr := resolveHEAD(ctx, cfg, workDir)
	results = append(results, retry)
	return head, results, true, retryErr
}

func headAdvanced(before headState, after headState) bool {
	if !after.Exists {
		return false
	}
	return !before.Exists || before.SHA != after.SHA
}

func normalizeConfig(cfg Config) (Config, string, error) {
	cfg.WorkingDir = strings.TrimSpace(cfg.WorkingDir)
	if cfg.WorkingDir == "" {
		return Config{}, "", errors.New("auto-commit: working directory is required")
	}
	cfg.RunID = strings.TrimSpace(cfg.RunID)
	if cfg.RunID == "" {
		return Config{}, "", errors.New("auto-commit: run id is required")
	}
	cfg.TaskID = strings.TrimSpace(cfg.TaskID)
	if cfg.TaskID == "" {
		return Config{}, "", errors.New("auto-commit: task id is required")
	}
	cfg.TaskSummary = singleLine(strings.TrimSpace(cfg.TaskSummary))
	if cfg.TaskSummary == "" {
		return Config{}, "", errors.New("auto-commit: task summary is required")
	}
	if cfg.CodexResult == nil {
		return Config{}, "", errors.New("auto-commit: codex result is required")
	}
	if cfg.VerificationResult == nil {
		return Config{}, "", errors.New("auto-commit: verification result is required")
	}
	if cfg.PreRunDirty == nil {
		return Config{}, "", errors.New("auto-commit: pre-run dirty capture is required")
	}
	if cfg.PostRunChanged == nil {
		return Config{}, "", errors.New("auto-commit: post-run changed-files capture is required")
	}
	cfg.GitExecutable = strings.TrimSpace(cfg.GitExecutable)
	if cfg.GitExecutable == "" {
		cfg.GitExecutable = defaultGitExecutable
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}

	workDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve working directory: %w", err)
	}
	return cfg, workDir, nil
}

func refusalReason(cfg Config, result Result) (RefusalReason, string) {
	if cfg.CodexResult.Err != nil || cfg.CodexResult.TimedOut || cfg.CodexResult.ExitCode != 0 {
		return ReasonCodexFailed, "Codex did not complete successfully"
	}
	if cfg.VerificationResult.MissingCommands && !cfg.AllowMissingVerification {
		return ReasonVerificationCommandsMissing, "verification commands are missing"
	}
	if !cfg.AllowMissingVerification && len(cfg.VerificationResult.Commands) == 0 {
		return ReasonVerificationCommandsMissing, "verification commands are missing"
	}
	if cfg.VerificationResult.Status != verification.StatusPassed || !cfg.VerificationResult.Passed {
		return ReasonVerificationFailed, "verification did not pass"
	}
	if cfg.PreRunDirty.CaptureError != "" || cfg.PostRunChanged.CaptureError != "" {
		return ReasonGitStateCaptureFailed, "git state capture failed"
	}
	if len(result.PreExistingDirtyFiles) > 0 && !cfg.AllowPreExistingDirty {
		return ReasonPreExistingDirty, "pre-existing dirty files are present"
	}
	if len(result.ChangedFiles) == 0 {
		return ReasonNoChanges, "there are no changes to commit"
	}
	return "", ""
}

func runGit(ctx context.Context, cfg Config, workDir string, args []string) GitCommandResult {
	startedAt := time.Now().UTC()
	runResult := cfg.CommandRunner(ctx, runner.Command{
		Name:        cfg.GitExecutable,
		Args:        append([]string(nil), args...),
		Dir:         workDir,
		Timeout:     cfg.Timeout,
		StdoutLimit: cfg.StdoutCap,
		StderrLimit: cfg.StderrCap,
	})
	endedAt := time.Now().UTC()
	return GitCommandResult{
		Command:   gitCommandString(cfg.GitExecutable, args),
		Name:      cfg.GitExecutable,
		Args:      append([]string(nil), args...),
		ExitCode:  runResult.ExitCode,
		TimedOut:  runResult.TimedOut,
		Error:     errorString(runResult.Err),
		Stdout:    runResult.Stdout,
		Stderr:    runResult.Stderr,
		StartedAt: startedAt,
		EndedAt:   endedAt,
	}
}

func commandPassed(result GitCommandResult) bool {
	return result.Error == "" && !result.TimedOut && result.ExitCode == 0
}

func commitMessageParts(taskSummary, runID, taskID string) (string, string) {
	return taskSummary, strings.Join([]string{
		"Run-ID: " + runID,
		"Task-ID: " + taskID,
		"Verification: passed",
	}, "\n")
}

func dirtyFiles(capture *gitstate.Capture) []string {
	if capture == nil {
		return nil
	}
	if len(capture.DirtyFiles) > 0 {
		return compactSortedStrings(capture.DirtyFiles)
	}
	return compactSortedStrings(capture.Paths)
}

func changedFiles(capture *gitstate.Capture) []string {
	if capture == nil {
		return nil
	}
	if len(capture.ChangedFiles) > 0 {
		return compactSortedStrings(capture.ChangedFiles)
	}
	return compactSortedStrings(capture.Paths)
}

func compactSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func gitCommandString(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(name))
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\n\"'\\$`") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
