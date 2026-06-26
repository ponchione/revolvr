package verification

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
	"revolvr/internal/runner"
)

const (
	StatusPassed Status = "passed"
	StatusFailed Status = "failed"

	MissingCommandsFail MissingCommandsPolicy = "fail"
	MissingCommandsPass MissingCommandsPolicy = "pass"
)

type Status string

type MissingCommandsPolicy string

type CommandRunner func(context.Context, runner.Command) runner.Result

type Ledger interface {
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
}

type Command struct {
	Name      string
	Args      []string
	Dir       string
	Env       []string
	Timeout   time.Duration
	StdoutCap int
	StderrCap int
}

type Config struct {
	WorkingDir            string
	Commands              []Command
	MissingCommandsPolicy MissingCommandsPolicy
	Timeout               time.Duration
	StdoutCap             int
	StderrCap             int
	RunID                 string
	Ledger                Ledger
	CommandRunner         CommandRunner
}

type CappedOutput struct {
	Content        string
	TruncatedBytes int64
}

type CommandResult struct {
	Index     int
	Command   string
	Name      string
	Args      []string
	Dir       string
	Status    Status
	Passed    bool
	ExitCode  int
	TimedOut  bool
	Err       error
	Error     string
	Timeout   time.Duration
	Stdout    CappedOutput
	Stderr    CappedOutput
	StartedAt time.Time
	EndedAt   time.Time
}

type Result struct {
	Status             Status
	Passed             bool
	MissingCommands    bool
	Message            string
	FailedCommandIndex int
	Commands           []CommandResult
	LedgerError        error
	StartedAt          time.Time
	EndedAt            time.Time
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	cfg, workDir, err := normalizeConfig(cfg)
	if err != nil {
		return Result{}, err
	}

	startedAt := time.Now().UTC()
	result := Result{
		Status:             StatusPassed,
		Passed:             true,
		FailedCommandIndex: -1,
		StartedAt:          startedAt,
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

	appendLedger(ledger.EventVerificationStarted, map[string]any{
		"working_dir":   workDir,
		"command_count": len(cfg.Commands),
		"commands":      commandPayloads(workDir, cfg),
	})

	if len(cfg.Commands) == 0 {
		result.MissingCommands = true
		result.Message = "no verification commands configured"
		if cfg.MissingCommandsPolicy == MissingCommandsFail {
			result.Status = StatusFailed
			result.Passed = false
		}
		result.EndedAt = time.Now().UTC()
		appendLedger(ledger.EventVerificationCompleted, completedPayload(result))
		result.LedgerError = ledgerErr
		return result, nil
	}

	for i, command := range cfg.Commands {
		commandStartedAt := time.Now().UTC()
		commandDir, err := resolveCommandDir(workDir, command.Dir)
		if err != nil {
			return Result{}, fmt.Errorf("run verification: command %d directory: %w", i, err)
		}
		timeout := commandTimeout(cfg, command)
		stdoutCap := commandStdoutCap(cfg, command)
		stderrCap := commandStderrCap(cfg, command)

		runResult := cfg.CommandRunner(ctx, runner.Command{
			Name:        command.Name,
			Args:        append([]string(nil), command.Args...),
			Dir:         commandDir,
			Env:         append([]string(nil), command.Env...),
			Timeout:     timeout,
			StdoutLimit: stdoutCap,
			StderrLimit: stderrCap,
		})
		commandEndedAt := time.Now().UTC()

		passed := commandPassed(runResult)
		commandStatus := StatusPassed
		if !passed {
			commandStatus = StatusFailed
		}
		commandResult := CommandResult{
			Index:    i,
			Command:  commandString(command.Name, command.Args),
			Name:     command.Name,
			Args:     append([]string(nil), command.Args...),
			Dir:      commandDir,
			Status:   commandStatus,
			Passed:   passed,
			ExitCode: runResult.ExitCode,
			TimedOut: runResult.TimedOut,
			Err:      runResult.Err,
			Error:    errorString(runResult.Err),
			Timeout:  timeout,
			Stdout: CappedOutput{
				Content:        runResult.Stdout,
				TruncatedBytes: runResult.StdoutTruncatedBytes,
			},
			Stderr: CappedOutput{
				Content:        runResult.Stderr,
				TruncatedBytes: runResult.StderrTruncatedBytes,
			},
			StartedAt: commandStartedAt,
			EndedAt:   commandEndedAt,
		}
		result.Commands = append(result.Commands, commandResult)

		if !passed {
			result.Status = StatusFailed
			result.Passed = false
			result.FailedCommandIndex = i
			result.Message = fmt.Sprintf("verification command %d failed", i)
			break
		}
	}

	result.EndedAt = time.Now().UTC()
	appendLedger(ledger.EventVerificationCompleted, completedPayload(result))
	result.LedgerError = ledgerErr
	return result, nil
}

func normalizeConfig(cfg Config) (Config, string, error) {
	cfg.WorkingDir = strings.TrimSpace(cfg.WorkingDir)
	if cfg.WorkingDir == "" {
		return Config{}, "", errors.New("run verification: working directory is required")
	}
	if cfg.Ledger != nil && strings.TrimSpace(cfg.RunID) == "" {
		return Config{}, "", errors.New("run verification: run id is required when ledger is configured")
	}
	cfg.RunID = strings.TrimSpace(cfg.RunID)
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}
	commands := make([]Command, 0, len(cfg.Commands))
	for i, command := range cfg.Commands {
		command.Name = strings.TrimSpace(command.Name)
		command.Dir = strings.TrimSpace(command.Dir)
		if command.Name == "" {
			return Config{}, "", fmt.Errorf("run verification: command %d name is required", i)
		}
		command.Args = append([]string(nil), command.Args...)
		command.Env = append([]string(nil), command.Env...)
		commands = append(commands, command)
	}
	cfg.Commands = commands
	if len(cfg.Commands) == 0 {
		switch cfg.MissingCommandsPolicy {
		case MissingCommandsFail, MissingCommandsPass:
		default:
			return Config{}, "", errors.New("run verification: missing commands policy is required when no commands are configured")
		}
	}

	workDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve working directory: %w", err)
	}
	return cfg, workDir, nil
}

func resolveCommandDir(workDir, dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return workDir, nil
	}
	resolved, err := pathguard.Resolve(workDir, dir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func commandTimeout(cfg Config, command Command) time.Duration {
	if command.Timeout > 0 {
		return command.Timeout
	}
	return cfg.Timeout
}

func commandStdoutCap(cfg Config, command Command) int {
	if command.StdoutCap > 0 {
		return command.StdoutCap
	}
	return cfg.StdoutCap
}

func commandStderrCap(cfg Config, command Command) int {
	if command.StderrCap > 0 {
		return command.StderrCap
	}
	return cfg.StderrCap
}

func commandPassed(result runner.Result) bool {
	return result.Err == nil && !result.TimedOut && result.ExitCode == 0
}

func commandPayloads(workDir string, cfg Config) []map[string]any {
	payloads := make([]map[string]any, 0, len(cfg.Commands))
	for i, command := range cfg.Commands {
		commandDir, err := resolveCommandDir(workDir, command.Dir)
		if err != nil {
			commandDir = strings.TrimSpace(command.Dir)
		}
		payloads = append(payloads, map[string]any{
			"index":      i,
			"command":    commandString(command.Name, command.Args),
			"name":       command.Name,
			"args":       append([]string(nil), command.Args...),
			"dir":        commandDir,
			"timeout":    commandTimeout(cfg, command).String(),
			"stdout_cap": commandStdoutCap(cfg, command),
			"stderr_cap": commandStderrCap(cfg, command),
		})
	}
	return payloads
}

func completedPayload(result Result) map[string]any {
	payload := map[string]any{
		"status":               result.Status,
		"passed":               result.Passed,
		"missing_commands":     result.MissingCommands,
		"message":              result.Message,
		"failed_command_index": result.FailedCommandIndex,
		"commands":             commandResultPayloads(result.Commands),
	}
	if !result.StartedAt.IsZero() && !result.EndedAt.IsZero() {
		payload["duration_ms"] = result.EndedAt.Sub(result.StartedAt).Milliseconds()
	}
	return payload
}

func commandResultPayloads(results []CommandResult) []map[string]any {
	payloads := make([]map[string]any, 0, len(results))
	for _, result := range results {
		payload := map[string]any{
			"index":       result.Index,
			"command":     result.Command,
			"name":        result.Name,
			"args":        append([]string(nil), result.Args...),
			"dir":         result.Dir,
			"status":      result.Status,
			"passed":      result.Passed,
			"exit_code":   result.ExitCode,
			"timed_out":   result.TimedOut,
			"error":       result.Error,
			"timeout":     result.Timeout.String(),
			"stdout":      outputPayload(result.Stdout),
			"stderr":      outputPayload(result.Stderr),
			"duration_ms": result.EndedAt.Sub(result.StartedAt).Milliseconds(),
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func outputPayload(output CappedOutput) map[string]any {
	return map[string]any{
		"content":         output.Content,
		"truncated_bytes": output.TruncatedBytes,
	}
}

func commandString(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(name))
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(value string) string {
	if value == "" {
		return strconv.Quote(value)
	}
	if strings.ContainsAny(value, " \t\n\"'\\$`") {
		return strconv.Quote(value)
	}
	return value
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
