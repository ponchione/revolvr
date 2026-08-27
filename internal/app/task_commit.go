package app

import (
	"context"
	"fmt"
	"strings"

	"revolvr/internal/gitstate"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
)

// AddTaskAndCommit creates one task and commits only its canonical task file.
func AddTaskAndCommit(ctx context.Context, cfg Config, input AddTaskInput, commandRunner PreflightCommandRunner) (taskmodel.Task, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return taskmodel.Task{}, err
	}
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return taskmodel.Task{}, err
	}
	runCfg, err = runonce.EffectiveConfig(runCfg)
	if err != nil {
		return taskmodel.Task{}, err
	}
	gitCfg := gitstate.Config{
		WorkingDir: paths.WorkDir, GitExecutable: runCfg.GitExecutable,
		Timeout: runCfg.GitTimeout, StdoutCap: runCfg.GitStdoutCap, StderrCap: runCfg.GitStderrCap,
	}
	if commandRunner == nil {
		commandRunner = runner.Run
	}
	gitCfg.CommandRunner = gitstate.CommandRunner(commandRunner)
	dirty, err := gitstate.CaptureDirtyWorktree(ctx, gitCfg)
	if err != nil {
		return taskmodel.Task{}, fmt.Errorf("task add: capture worktree: %w", err)
	}
	if dirty.CaptureError != "" {
		return taskmodel.Task{}, fmt.Errorf("task add: capture worktree: %s", dirty.CaptureError)
	}
	if len(dirty.Paths) != 0 {
		return taskmodel.Task{}, fmt.Errorf("task add: worktree must be clean before TUI task creation: %s", strings.Join(dirty.Paths, ", "))
	}
	runGit := func(args ...string) runner.Result {
		return commandRunner(ctx, runner.Command{
			Name: runCfg.GitExecutable, Args: args, Dir: paths.WorkDir,
			Timeout: runCfg.GitTimeout, StdoutLimit: runCfg.GitStdoutCap, StderrLimit: runCfg.GitStderrCap,
		})
	}
	passed := func(result runner.Result) bool {
		return result.Err == nil && !result.TimedOut && result.ExitCode == 0
	}
	before := runGit("rev-parse", "--verify", "HEAD")
	if !passed(before) || strings.TrimSpace(before.Stdout) == "" {
		return taskmodel.Task{}, fmt.Errorf("task add: resolve HEAD before task creation: %s", runnerFailure(before))
	}

	task, err := AddTask(ctx, cfg, input)
	if err != nil {
		return taskmodel.Task{}, err
	}
	created, ok, err := taskfile.FindByID(paths.WorkDir, task.ID)
	if err != nil {
		return taskmodel.Task{}, fmt.Errorf("task add: reload created task %q: %w", task.ID, err)
	}
	if !ok {
		return taskmodel.Task{}, fmt.Errorf("task add: reload created task %q: not found", task.ID)
	}
	stage := runGit("--literal-pathspecs", "add", "--", created.SourcePath)
	if !passed(stage) {
		return taskmodel.Task{}, fmt.Errorf("task add: stage %s: %s", created.SourcePath, runnerFailure(stage))
	}
	commit := runGit("--literal-pathspecs", "commit", "--only", "-m", "Add task "+task.ID, "--", created.SourcePath)
	after := runGit("rev-parse", "--verify", "HEAD")
	if !passed(after) || strings.TrimSpace(after.Stdout) == "" {
		return taskmodel.Task{}, fmt.Errorf("task add: commit outcome is indeterminate: %s", runnerFailure(after))
	}
	if strings.TrimSpace(after.Stdout) == strings.TrimSpace(before.Stdout) {
		return taskmodel.Task{}, fmt.Errorf("task add: commit task: %s", runnerFailure(commit))
	}

	dirty, err = gitstate.CaptureDirtyWorktree(ctx, gitCfg)
	if err != nil || dirty.CaptureError != "" || len(dirty.Paths) != 0 {
		return taskmodel.Task{}, fmt.Errorf("task add: task committed but worktree is not ready for preflight")
	}
	return task, nil
}

func runnerFailure(result runner.Result) string {
	if detail := strings.TrimSpace(result.Stderr); detail != "" {
		return detail
	}
	if result.Err != nil {
		return result.Err.Error()
	}
	if result.TimedOut {
		return "command timed out"
	}
	return fmt.Sprintf("exit code %d", result.ExitCode)
}
