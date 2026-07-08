package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/taskqueue"
)

const (
	stateDirName           = ".revolvr"
	defaultRecentRunsLimit = 20
)

type Config struct {
	WorkDir         string
	RecentRunsLimit int
}

type AddTaskInput struct {
	Task    string
	Summary string
}

type StatusResult struct {
	Initialized  bool
	Tasks        []taskqueue.Task
	RecentRuns   []ledger.Run
	LatestEvents []ledger.Event
}

func AddTask(ctx context.Context, cfg Config, input AddTaskInput) (taskqueue.Task, error) {
	taskText := strings.TrimSpace(input.Task)
	if taskText == "" {
		return taskqueue.Task{}, errors.New("task add: task text is required")
	}

	tasks, err := openTaskStore(ctx, cfg)
	if err != nil {
		return taskqueue.Task{}, err
	}
	defer tasks.Close()

	return tasks.AddTask(ctx, taskqueue.TaskSpec{
		Task:    taskText,
		Summary: strings.TrimSpace(input.Summary),
	})
}

func ListTasks(ctx context.Context, cfg Config) ([]taskqueue.Task, error) {
	tasks, err := openTaskStore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer tasks.Close()

	return tasks.ListTasks(ctx)
}

func RetryTask(ctx context.Context, cfg Config, taskID string) (taskqueue.Task, error) {
	return unblockBlockedTask(ctx, cfg, taskID, "task retry")
}

func UnblockTask(ctx context.Context, cfg Config, taskID string) (taskqueue.Task, error) {
	return unblockBlockedTask(ctx, cfg, taskID, "task unblock")
}

func Status(ctx context.Context, cfg Config) (StatusResult, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return StatusResult{}, err
	}
	initialized, err := stateInitialized(paths)
	if err != nil {
		return StatusResult{}, err
	}
	if !initialized {
		return StatusResult{Initialized: false}, nil
	}

	tasks, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		return StatusResult{}, err
	}
	defer tasks.Close()

	taskList, err := tasks.ListTasks(ctx)
	if err != nil {
		return StatusResult{}, err
	}

	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		return StatusResult{}, err
	}
	defer runs.Close()

	limit := cfg.RecentRunsLimit
	if limit <= 0 {
		limit = defaultRecentRunsLimit
	}
	recentRuns, err := runs.ListRecentRuns(ctx, limit)
	if err != nil {
		return StatusResult{}, err
	}

	var latestEvents []ledger.Event
	if len(recentRuns) > 0 {
		latestHistory, ok, err := runs.GetRunWithEvents(ctx, recentRuns[0].ID)
		if err != nil {
			return StatusResult{}, err
		}
		if ok {
			latestEvents = latestHistory.Events
		}
	}

	return StatusResult{
		Initialized:  true,
		Tasks:        taskList,
		RecentRuns:   recentRuns,
		LatestEvents: latestEvents,
	}, nil
}

func ShowRun(ctx context.Context, cfg Config, runID string) (ledger.RunWithEvents, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ledger.RunWithEvents{}, errors.New("show run: run id is required")
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return ledger.RunWithEvents{}, err
	}
	initialized, err := ledgerInitialized(paths)
	if err != nil {
		return ledger.RunWithEvents{}, err
	}
	if !initialized {
		return ledger.RunWithEvents{}, errors.New("state is not initialized; run `revolvr init` first")
	}

	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		return ledger.RunWithEvents{}, err
	}
	defer runs.Close()

	history, ok, err := runs.GetRunWithEvents(ctx, runID)
	if err != nil {
		return ledger.RunWithEvents{}, err
	}
	if !ok {
		return ledger.RunWithEvents{}, fmt.Errorf("run %q not found", runID)
	}
	return history, nil
}

func ValidateReceipt(ctx context.Context, cfg Config, runID string) (receipt.ValidationResult, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return receipt.ValidationResult{}, errors.New("receipt validate: run id is required")
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return receipt.ValidationResult{}, err
	}
	initialized, err := ledgerInitialized(paths)
	if err != nil {
		return receipt.ValidationResult{}, err
	}
	if !initialized {
		return receipt.ValidationResult{}, errors.New("state is not initialized; run `revolvr init` first")
	}

	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		return receipt.ValidationResult{}, err
	}
	defer runs.Close()

	history, ok, err := runs.GetRunWithEvents(ctx, runID)
	if err != nil {
		return receipt.ValidationResult{}, err
	}
	if !ok {
		return receipt.ValidationResult{}, fmt.Errorf("run %q not found", runID)
	}

	return receipt.ValidateRunReceipt(receipt.ValidationInput{
		WorkDir: paths.WorkDir,
		History: history,
	})
}

func unblockBlockedTask(ctx context.Context, cfg Config, rawTaskID string, operation string) (taskqueue.Task, error) {
	taskID := strings.TrimSpace(rawTaskID)
	if taskID == "" {
		return taskqueue.Task{}, fmt.Errorf("%s: task id is required", operation)
	}

	tasks, err := openTaskStore(ctx, cfg)
	if err != nil {
		return taskqueue.Task{}, err
	}
	defer tasks.Close()

	task, changed, err := tasks.UnblockTask(ctx, taskID)
	if err != nil {
		return taskqueue.Task{}, err
	}
	if !changed {
		if task.ID == "" {
			return taskqueue.Task{}, fmt.Errorf("task %q not found", taskID)
		}
		return taskqueue.Task{}, fmt.Errorf("task %q is not blocked (status: %s)", taskID, task.Status)
	}
	return task, nil
}

type statePaths struct {
	WorkDir      string
	StateDir     string
	TaskDBPath   string
	LedgerDBPath string
}

func resolveStatePaths(workDir string) (statePaths, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return statePaths{}, fmt.Errorf("resolve working directory: %w", err)
		}
		workDir = wd
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return statePaths{}, fmt.Errorf("resolve working directory: %w", err)
	}

	stateDir := filepath.Join(absWorkDir, stateDirName)
	return statePaths{
		WorkDir:      absWorkDir,
		StateDir:     stateDir,
		TaskDBPath:   filepath.Join(stateDir, "tasks.sqlite"),
		LedgerDBPath: filepath.Join(stateDir, "ledger.sqlite"),
	}, nil
}

func openTaskStore(ctx context.Context, cfg Config) (*taskqueue.Store, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	return taskqueue.Open(ctx, paths.TaskDBPath)
}

func stateInitialized(paths statePaths) (bool, error) {
	return pathsInitialized(paths, paths.TaskDBPath, paths.LedgerDBPath)
}

func ledgerInitialized(paths statePaths) (bool, error) {
	return pathsInitialized(paths, paths.LedgerDBPath)
}

func pathsInitialized(paths statePaths, storePaths ...string) (bool, error) {
	stateDir, err := existingDir(paths.StateDir)
	if err != nil || !stateDir {
		return stateDir, err
	}
	for _, storePath := range storePaths {
		exists, err := existingFile(storePath)
		if err != nil || !exists {
			return exists, err
		}
	}
	return true, nil
}

func existingDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	return info.IsDir(), nil
}

func existingFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	return !info.IsDir(), nil
}
