package cli

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"revolvr/internal/gitstate"
	"revolvr/internal/runner"
)

type DoctorCommandRunner func(context.Context, runner.Command) runner.Result
type ExecutableLookPath func(string) (string, error)

type doctorStatus string

const (
	doctorOK   doctorStatus = "OK"
	doctorFail doctorStatus = "FAIL"
)

type doctorCheck struct {
	Status doctorStatus
	Name   string
	Detail string
}

type doctorResult struct {
	Checks []doctorCheck
	Ready  bool
}

type doctorConfig struct {
	WorkDir       string
	CommandRunner DoctorCommandRunner
	LookPath      ExecutableLookPath
}

type doctorFailedError struct{}

func (doctorFailedError) Error() string {
	return "doctor: preflight failed"
}

func newDoctorCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check readiness for dogfooding",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := runDoctor(cmd.Context(), doctorConfig{
				WorkDir:       opts.WorkDir,
				CommandRunner: opts.DoctorCommandRunner,
				LookPath:      opts.ExecutableLookPath,
			})
			if err != nil {
				return err
			}
			if err := writeDoctor(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			if !result.Ready {
				return doctorFailedError{}
			}
			return nil
		},
	}
}

func runDoctor(ctx context.Context, cfg doctorConfig) (doctorResult, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return doctorResult{}, err
	}

	commandRunner := cfg.CommandRunner
	if commandRunner == nil {
		commandRunner = runner.Run
	}
	lookPath := cfg.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	result := doctorResult{Ready: true}
	addCheck := func(status doctorStatus, name string, detail string) {
		if status == doctorFail {
			result.Ready = false
		}
		result.Checks = append(result.Checks, doctorCheck{
			Status: status,
			Name:   name,
			Detail: strings.TrimSpace(detail),
		})
	}

	initialized, err := stateInitialized(paths)
	switch {
	case err != nil:
		addCheck(doctorFail, "state", err.Error())
	case initialized:
		addCheck(doctorOK, "state", "initialized at "+paths.StateDir)
	default:
		addCheck(doctorFail, "state", "not initialized; run `revolvr init`")
	}

	configResult, err := checkRunConfig(paths.WorkDir)
	if err != nil {
		addCheck(doctorFail, "config", err.Error())
		return result, nil
	}
	if configResult.Found {
		addCheck(doctorOK, "config", "loaded "+configResult.Path)
	} else {
		addCheck(doctorOK, "config", "defaults used")
	}
	runCfg := configResult.Effective

	codexExecutable := effectiveString(runCfg.CodexExecutable, defaultCodexExecutable)
	addExecutableCheck(addCheck, lookPath, "codex executable", codexExecutable)

	gitExecutable := effectiveString(runCfg.GitExecutable, defaultGitExecutable)
	addExecutableCheck(addCheck, lookPath, "git executable", gitExecutable)

	addGitIdentityCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	addWorktreeCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	addRuntimeIgnoreCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	addVerificationCheck(addCheck, len(runCfg.VerificationCommands), runCfg.AllowMissingVerification)

	return result, nil
}

func writeDoctor(out io.Writer, result doctorResult) error {
	if _, err := fmt.Fprintln(out, "Dogfood preflight:"); err != nil {
		return err
	}
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(out, "%s %s: %s\n", check.Status, check.Name, check.Detail); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "Ready: %t\n", result.Ready)
	return err
}

func addExecutableCheck(addCheck func(doctorStatus, string, string), lookPath ExecutableLookPath, name string, executable string) {
	resolved, err := lookPath(executable)
	if err != nil {
		addCheck(doctorFail, name, fmt.Sprintf("%q not found: %v", executable, err))
		return
	}
	addCheck(doctorOK, name, resolved)
}

func addGitIdentityCheck(ctx context.Context, addCheck func(doctorStatus, string, string), commandRunner DoctorCommandRunner, workDir string, gitExecutable string, timeout time.Duration, stdoutCap int, stderrCap int) {
	nameResult := runDoctorGit(ctx, commandRunner, workDir, gitExecutable, []string{"config", "--get", "user.name"}, timeout, stdoutCap, stderrCap)
	emailResult := runDoctorGit(ctx, commandRunner, workDir, gitExecutable, []string{"config", "--get", "user.email"}, timeout, stdoutCap, stderrCap)
	name := strings.TrimSpace(nameResult.Stdout)
	email := strings.TrimSpace(emailResult.Stdout)

	missing := []string{}
	if !doctorCommandPassed(nameResult) || name == "" {
		missing = append(missing, "user.name")
	}
	if !doctorCommandPassed(emailResult) || email == "" {
		missing = append(missing, "user.email")
	}
	if len(missing) > 0 {
		addCheck(doctorFail, "git identity", "missing "+strings.Join(missing, " and "))
		return
	}
	addCheck(doctorOK, "git identity", fmt.Sprintf("%s <%s>", oneLine(name), oneLine(email)))
}

func addWorktreeCheck(ctx context.Context, addCheck func(doctorStatus, string, string), commandRunner DoctorCommandRunner, workDir string, gitExecutable string, timeout time.Duration, stdoutCap int, stderrCap int) {
	result := runDoctorGit(ctx, commandRunner, workDir, gitExecutable, []string{"status", "--short", "--untracked-files=all"}, timeout, stdoutCap, stderrCap)
	if !doctorCommandPassed(result) {
		addCheck(doctorFail, "worktree clean", doctorCommandFailure(result))
		return
	}
	entries := gitstate.ParseShortStatus(result.Stdout)
	paths := gitstate.PathsFromEntries(entries)
	if len(paths) > 0 {
		addCheck(doctorFail, "worktree clean", "dirty files: "+strings.Join(paths, ", "))
		return
	}
	addCheck(doctorOK, "worktree clean", "no changes")
}

func addRuntimeIgnoreCheck(ctx context.Context, addCheck func(doctorStatus, string, string), commandRunner DoctorCommandRunner, workDir string, gitExecutable string, timeout time.Duration, stdoutCap int, stderrCap int) {
	result := runDoctorGit(ctx, commandRunner, workDir, gitExecutable, []string{"check-ignore", "--quiet", revolvrStateDir + "/"}, timeout, stdoutCap, stderrCap)
	switch {
	case doctorCommandPassed(result):
		addCheck(doctorOK, "runtime state ignored", revolvrStateDir+"/ ignored by Git")
	case result.Err == nil && !result.TimedOut && result.ExitCode == 1:
		addCheck(doctorFail, "runtime state ignored", revolvrStateDir+"/ is not ignored; run `revolvr init`")
	default:
		addCheck(doctorFail, "runtime state ignored", doctorCommandFailure(result))
	}
}

func addVerificationCheck(addCheck func(doctorStatus, string, string), commandCount int, allowMissing bool) {
	if commandCount > 0 {
		label := "commands"
		if commandCount == 1 {
			label = "command"
		}
		addCheck(doctorOK, "verification commands", fmt.Sprintf("%d %s configured", commandCount, label))
		return
	}
	if allowMissing {
		addCheck(doctorOK, "verification commands", "missing verification allowed by config")
		return
	}
	addCheck(doctorFail, "verification commands", "no verification commands configured")
}

func runDoctorGit(ctx context.Context, commandRunner DoctorCommandRunner, workDir string, gitExecutable string, args []string, timeout time.Duration, stdoutCap int, stderrCap int) runner.Result {
	return commandRunner(ctx, runner.Command{
		Name:        gitExecutable,
		Args:        append([]string(nil), args...),
		Dir:         workDir,
		Timeout:     timeout,
		StdoutLimit: stdoutCap,
		StderrLimit: stderrCap,
	})
}

func doctorCommandPassed(result runner.Result) bool {
	return result.Err == nil && !result.TimedOut && result.ExitCode == 0
}

func doctorCommandFailure(result runner.Result) string {
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
		return fmt.Sprintf("command exited with code %d: %s", result.ExitCode, oneLine(detail))
	default:
		return "command failed"
	}
}
