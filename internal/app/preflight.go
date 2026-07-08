package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"revolvr/internal/gitstate"
	"revolvr/internal/runner"
)

type PreflightCommandRunner func(context.Context, runner.Command) runner.Result
type ExecutableLookPath func(string) (string, error)

type PreflightCheckStatus string

const (
	PreflightOK   PreflightCheckStatus = "OK"
	PreflightFail PreflightCheckStatus = "FAIL"
)

type PreflightCheck struct {
	Status PreflightCheckStatus
	Name   string
	Detail string
}

type PreflightResult struct {
	Checks []PreflightCheck
	Ready  bool
}

type PreflightInput struct {
	CommandRunner PreflightCommandRunner
	LookPath      ExecutableLookPath
}

func Preflight(ctx context.Context, cfg Config, input PreflightInput) (PreflightResult, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return PreflightResult{}, err
	}

	commandRunner := input.CommandRunner
	if commandRunner == nil {
		commandRunner = runner.Run
	}
	lookPath := input.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	result := PreflightResult{Ready: true}
	addCheck := func(status PreflightCheckStatus, name string, detail string) {
		if status == PreflightFail {
			result.Ready = false
		}
		result.Checks = append(result.Checks, PreflightCheck{
			Status: status,
			Name:   name,
			Detail: strings.TrimSpace(detail),
		})
	}

	initialized, err := stateInitialized(paths)
	switch {
	case err != nil:
		addCheck(PreflightFail, "state", err.Error())
	case initialized:
		addCheck(PreflightOK, "state", "initialized at "+paths.StateDir)
	default:
		addCheck(PreflightFail, "state", "not initialized; run `revolvr init`")
	}

	configResult, err := CheckRunConfig(paths.WorkDir)
	if err != nil {
		addCheck(PreflightFail, "config", err.Error())
		return result, nil
	}
	if configResult.Found {
		addCheck(PreflightOK, "config", "loaded "+configResult.Path)
	} else {
		addCheck(PreflightOK, "config", "defaults used")
	}
	runCfg := configResult.Effective

	codexExecutable := preflightEffectiveString(runCfg.CodexExecutable, DefaultCodexExecutable)
	addExecutableCheck(addCheck, lookPath, "codex executable", codexExecutable)

	gitExecutable := preflightEffectiveString(runCfg.GitExecutable, DefaultGitExecutable)
	addExecutableCheck(addCheck, lookPath, "git executable", gitExecutable)

	addGitIdentityCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	addWorktreeCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	addRuntimeIgnoreCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	addVerificationCheck(addCheck, len(runCfg.VerificationCommands), runCfg.AllowMissingVerification)

	return result, nil
}

func addExecutableCheck(addCheck func(PreflightCheckStatus, string, string), lookPath ExecutableLookPath, name string, executable string) {
	resolved, err := lookPath(executable)
	if err != nil {
		addCheck(PreflightFail, name, fmt.Sprintf("%q not found: %v", executable, err))
		return
	}
	addCheck(PreflightOK, name, resolved)
}

func addGitIdentityCheck(ctx context.Context, addCheck func(PreflightCheckStatus, string, string), commandRunner PreflightCommandRunner, workDir string, gitExecutable string, timeout time.Duration, stdoutCap int, stderrCap int) {
	nameResult := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"config", "--get", "user.name"}, timeout, stdoutCap, stderrCap)
	emailResult := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"config", "--get", "user.email"}, timeout, stdoutCap, stderrCap)
	name := strings.TrimSpace(nameResult.Stdout)
	email := strings.TrimSpace(emailResult.Stdout)

	missing := []string{}
	if !preflightCommandPassed(nameResult) || name == "" {
		missing = append(missing, "user.name")
	}
	if !preflightCommandPassed(emailResult) || email == "" {
		missing = append(missing, "user.email")
	}
	if len(missing) > 0 {
		addCheck(PreflightFail, "git identity", "missing "+strings.Join(missing, " and "))
		return
	}
	addCheck(PreflightOK, "git identity", fmt.Sprintf("%s <%s>", preflightOneLine(name), preflightOneLine(email)))
}

func addWorktreeCheck(ctx context.Context, addCheck func(PreflightCheckStatus, string, string), commandRunner PreflightCommandRunner, workDir string, gitExecutable string, timeout time.Duration, stdoutCap int, stderrCap int) {
	result := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"status", "--short", "--untracked-files=all"}, timeout, stdoutCap, stderrCap)
	if !preflightCommandPassed(result) {
		addCheck(PreflightFail, "worktree clean", preflightCommandFailure(result))
		return
	}
	entries := gitstate.ParseShortStatus(result.Stdout)
	paths := gitstate.PathsFromEntries(entries)
	if len(paths) > 0 {
		addCheck(PreflightFail, "worktree clean", "dirty files: "+strings.Join(paths, ", "))
		return
	}
	addCheck(PreflightOK, "worktree clean", "no changes")
}

func addRuntimeIgnoreCheck(ctx context.Context, addCheck func(PreflightCheckStatus, string, string), commandRunner PreflightCommandRunner, workDir string, gitExecutable string, timeout time.Duration, stdoutCap int, stderrCap int) {
	result := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"check-ignore", "--quiet", stateDirName + "/"}, timeout, stdoutCap, stderrCap)
	switch {
	case preflightCommandPassed(result):
		addCheck(PreflightOK, "runtime state ignored", stateDirName+"/ ignored by Git")
	case result.Err == nil && !result.TimedOut && result.ExitCode == 1:
		addCheck(PreflightFail, "runtime state ignored", stateDirName+"/ is not ignored; run `revolvr init`")
	default:
		addCheck(PreflightFail, "runtime state ignored", preflightCommandFailure(result))
	}
}

func addVerificationCheck(addCheck func(PreflightCheckStatus, string, string), commandCount int, allowMissing bool) {
	if commandCount > 0 {
		label := "commands"
		if commandCount == 1 {
			label = "command"
		}
		addCheck(PreflightOK, "verification commands", fmt.Sprintf("%d %s configured", commandCount, label))
		return
	}
	if allowMissing {
		addCheck(PreflightOK, "verification commands", "missing verification allowed by config")
		return
	}
	addCheck(PreflightFail, "verification commands", "no verification commands configured")
}

func runPreflightGit(ctx context.Context, commandRunner PreflightCommandRunner, workDir string, gitExecutable string, args []string, timeout time.Duration, stdoutCap int, stderrCap int) runner.Result {
	return commandRunner(ctx, runner.Command{
		Name:        gitExecutable,
		Args:        append([]string(nil), args...),
		Dir:         workDir,
		Timeout:     timeout,
		StdoutLimit: stdoutCap,
		StderrLimit: stderrCap,
	})
}

func preflightCommandPassed(result runner.Result) bool {
	return result.Err == nil && !result.TimedOut && result.ExitCode == 0
}

func preflightCommandFailure(result runner.Result) string {
	switch {
	case result.TimedOut:
		return "command timed out"
	case result.Err != nil:
		return result.Err.Error()
	case result.ExitCode != 0:
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		if detail == "" {
			return fmt.Sprintf("command exited with code %d", result.ExitCode)
		}
		return fmt.Sprintf("command exited with code %d: %s", result.ExitCode, preflightOneLine(detail))
	default:
		return "command failed"
	}
}

func preflightEffectiveString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func preflightOneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
