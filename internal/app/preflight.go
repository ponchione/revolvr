package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/redact"
	"revolvr/internal/repositorypath"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

type PreflightCommandRunner func(context.Context, runner.Command) runner.Result
type ExecutableLookPath func(string) (string, error)
type ExecutableInspector func(string, codexexec.ExecutableLookPath) (codexexec.ExecutableIdentity, error)
type CodexIdentityInspector func(context.Context, string, string, codexexec.VersionConfig, codexexec.ExecutableLookPath) (codexexec.CodexExecutableIdentity, error)

type PreflightCheckStatus string

type PreflightMode string

const (
	PreflightOK   PreflightCheckStatus = "OK"
	PreflightFail PreflightCheckStatus = "FAIL"

	PreflightModeAttendedTask PreflightMode = "attended-task"
	PreflightModeQueue        PreflightMode = "queue"
	PreflightModeDaemon       PreflightMode = "daemon"
)

type PreflightCheck struct {
	Status PreflightCheckStatus
	Name   string
	Detail string
}

type PreflightResult struct {
	Mode   PreflightMode
	TaskID string
	Checks []PreflightCheck
	Ready  bool
}

type PreflightInput struct {
	CommandRunner          PreflightCommandRunner
	LookPath               ExecutableLookPath
	Mode                   PreflightMode
	TaskID                 string
	Platform               string
	ExecutableInspector    ExecutableInspector
	CodexIdentityInspector CodexIdentityInspector
}

func Preflight(ctx context.Context, cfg Config, input PreflightInput) (PreflightResult, error) {
	mode, taskID, err := normalizePreflightRequest(input.Mode, input.TaskID)
	if err != nil {
		return PreflightResult{}, err
	}
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

	result := PreflightResult{Mode: mode, TaskID: taskID, Ready: true}
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

	authority, pathErr := repositorypath.Inspect(paths.WorkDir, repositorypath.InspectOptions{})
	switch {
	case pathErr != nil:
		addCheck(PreflightFail, "state", pathErr.Error())
		return result, nil
	case authority.Initialized():
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
	graphCfg := runCfg
	graphCfg.CommandRunner = runonce.CommandRunner(commandRunner)
	graph, _, err := loadAutonomousGraph(ctx, paths.WorkDir, graphCfg)
	if err != nil {
		addCheck(PreflightFail, "task graph", err.Error())
		return result, nil
	}
	addModeTaskGraphCheck(addCheck, mode, taskID, graph)
	scope := inspectExternalScope(ctx, externalScopeInput{Mode: mode, Platform: input.Platform, WorkDir: paths.WorkDir, RunConfig: runCfg, CommandRunner: commandRunner, LookPath: lookPath, ExecutableInspector: input.ExecutableInspector, CodexIdentityInspector: input.CodexIdentityInspector})
	gitAuthorityReady := true
	var codexExecutableCheck, codexVersionCheck *PreflightCheck
	for _, check := range scope.Checks {
		if check.Name == "codex executable" {
			copy := check
			codexExecutableCheck = &copy
			continue
		}
		if check.Name == "codex version" {
			copy := check
			codexVersionCheck = &copy
			continue
		}
		addCheck(check.Status, check.Name, check.Detail)
		if (check.Name == "git executable" || check.Name == "repository shape") && check.Status != PreflightOK {
			gitAuthorityReady = false
		}
	}
	addAutonomySafetyCheck(addCheck, runCfg.SafetyDeclaration)
	if err := runCfg.QueuePolicy.Validate(); err != nil {
		addCheck(PreflightFail, "autonomous queue", err.Error())
	} else {
		addCheck(PreflightOK, "autonomous queue", fmt.Sprintf("schema=%s maximum_workers=%d", runCfg.QueuePolicy.SchemaVersion, runCfg.QueuePolicy.MaximumWorkers))
	}
	if err := runCfg.RetentionPolicy.Validate(); err != nil {
		addCheck(PreflightFail, "artifact retention", err.Error())
	} else {
		addCheck(PreflightOK, "artifact retention", fmt.Sprintf("schema=%s mutation_enabled=%t recent_runs=%d", runCfg.RetentionPolicy.SchemaVersion, runCfg.RetentionPolicy.MutationEnabled, runCfg.RetentionPolicy.RecentRunCount))
	}
	if !runCfg.NotificationPolicy.Enabled {
		addCheck(PreflightOK, "notification hooks", "disabled; no executable lookup, environment load, outbox write, or process start")
	} else if _, err := runCfg.NotificationPolicy.Normalize(runCfg.SafetyDeclaration.Redaction.EnvironmentVariables); err != nil {
		addCheck(PreflightFail, "notification hooks", err.Error())
	} else if notificationRedactor, _, err := redact.New(runCfg.SafetyDeclaration.Redaction, os.LookupEnv); err != nil {
		addCheck(PreflightFail, "notification hooks", err.Error())
	} else if resolved, lookupErr := lookPath(runCfg.NotificationPolicy.Executable); lookupErr != nil {
		addCheck(PreflightFail, "notification executable", notificationRedactor.String(fmt.Sprintf("%q not found: %v", runCfg.NotificationPolicy.Executable, lookupErr)))
	} else {
		addCheck(PreflightOK, "notification executable", notificationRedactor.String(resolved))
		addCheck(PreflightOK, "notification hooks", fmt.Sprintf("events=%d arguments=%d directory=%s timeout=%s attempts=%d output_caps=%d/%d replacement_environment_names=%d", len(runCfg.NotificationPolicy.Events), len(runCfg.NotificationPolicy.Args), runCfg.NotificationPolicy.Directory, runCfg.NotificationPolicy.Timeout, runCfg.NotificationPolicy.MaximumAttempts, runCfg.NotificationPolicy.StdoutCap, runCfg.NotificationPolicy.StderrCap, len(runCfg.NotificationPolicy.EnvironmentNames)))
	}

	if codexExecutableCheck != nil {
		addCheck(codexExecutableCheck.Status, codexExecutableCheck.Name, codexExecutableCheck.Detail)
	}
	addCheck(PreflightOK, "codex model", runCfg.CodexModel)
	addCheck(PreflightOK, "codex reasoning effort", runCfg.CodexReasoningEffort)
	addCheck(PreflightOK, "codex session", fmt.Sprintf("%s (ephemeral=%t)", codexexec.SessionModeEphemeral, runCfg.CodexEphemeral))
	if codexVersionCheck != nil {
		addCheck(codexVersionCheck.Status, codexVersionCheck.Name, codexVersionCheck.Detail)
	}
	gitExecutable := preflightEffectiveString(runCfg.GitExecutable, DefaultGitExecutable)
	if gitAuthorityReady {
		addGitIdentityCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
		addRuntimeIgnoreCheck(ctx, addCheck, commandRunner, paths.WorkDir, gitExecutable, runCfg.GitTimeout, runCfg.GitStdoutCap, runCfg.GitStderrCap)
	} else {
		addCheck(PreflightFail, "git identity", "not checked because Git repository authority is unavailable")
		addCheck(PreflightFail, "runtime state ignored", "not checked because Git repository authority is unavailable")
	}

	return result, nil
}

func normalizePreflightRequest(mode PreflightMode, taskID string) (PreflightMode, string, error) {
	if mode == "" {
		mode = PreflightModeAttendedTask
	}
	switch mode {
	case PreflightModeAttendedTask, PreflightModeQueue, PreflightModeDaemon:
	default:
		return "", "", fmt.Errorf("preflight: invalid mode %q (want %s, %s, or %s)", mode, PreflightModeAttendedTask, PreflightModeQueue, PreflightModeDaemon)
	}
	if taskID != "" && !validPreflightTaskID(taskID) {
		return "", "", fmt.Errorf("preflight: invalid exact task selector %q", taskID)
	}
	if taskID != "" && mode != PreflightModeAttendedTask {
		return "", "", fmt.Errorf("preflight: exact task selector requires mode %s", PreflightModeAttendedTask)
	}
	return mode, taskID, nil
}

func validPreflightTaskID(taskID string) bool {
	if taskID == "" || taskID == "." || taskID == ".." {
		return false
	}
	for _, r := range taskID {
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

func addModeTaskGraphCheck(addCheck func(PreflightCheckStatus, string, string), mode PreflightMode, taskID string, graph autonomousscheduler.Graph) {
	autonomousTasks := 0
	for _, node := range graph.Nodes {
		if node.Task.Workflow == taskfile.WorkflowAutonomousV1 {
			autonomousTasks++
		}
	}
	detail := fmt.Sprintf("mode=%s canonical_tasks=%d autonomous_tasks=%d", mode, len(graph.Nodes), autonomousTasks)
	if taskID == "" {
		addCheck(PreflightOK, "task graph", detail)
		return
	}
	node, err := autonomousscheduler.ClassifyTask(graph, taskID, nil)
	if err != nil {
		addCheck(PreflightFail, "task graph", detail+"; "+err.Error())
		return
	}
	detail += fmt.Sprintf(" task=%s readiness=%s", taskID, node.Reason)
	if node.Reason != taskscheduler.ReasonReady {
		detail += fmt.Sprintf(" dependencies=%v conflicts=%v", node.WaitingOn, node.Conflicts)
		addCheck(PreflightFail, "task graph", detail)
		return
	}
	addCheck(PreflightOK, "task graph", detail)
}

func addAutonomySafetyCheck(addCheck func(PreflightCheckStatus, string, string), declaration autonomoussafety.Declaration) {
	if err := declaration.Validate(); err != nil {
		addCheck(PreflightFail, "autonomy safety", err.Error())
		return
	}
	if declaration.Mode == autonomoussafety.ModeFullyUnattended {
		addCheck(PreflightFail, "autonomy safety", "fully unattended execution requires a task/workspace-bound safety preflight; worktree isolation alone is not a security sandbox")
		return
	}
	addCheck(PreflightOK, "autonomy safety", fmt.Sprintf("mode=%s; operator remains responsible for host, network, hooks, and credentials; worktree isolation is Git/source isolation only", declaration.Mode))
}

func addExecutableCheck(addCheck func(PreflightCheckStatus, string, string), lookPath ExecutableLookPath, name string, executable string) bool {
	resolved, err := lookPath(executable)
	if err != nil {
		addCheck(PreflightFail, name, fmt.Sprintf("%q not found: %v", executable, err))
		return false
	}
	addCheck(PreflightOK, name, resolved)
	return true
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
	capture, err := gitstate.CaptureDirtyWorktree(ctx, gitstate.Config{
		WorkingDir:    workDir,
		GitExecutable: gitExecutable,
		Timeout:       timeout,
		StdoutCap:     stdoutCap,
		StderrCap:     stderrCap,
		CommandRunner: gitstate.CommandRunner(commandRunner),
	})
	if err != nil {
		addCheck(PreflightFail, "worktree clean", err.Error())
		return
	}
	if capture.CaptureError != "" {
		addCheck(PreflightFail, "worktree clean", capture.CaptureError)
		return
	}
	if len(capture.Paths) > 0 {
		addCheck(PreflightFail, "worktree clean", "dirty files: "+strings.Join(capture.Paths, ", "))
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
