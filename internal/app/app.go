package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/taskimport"
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

type TaskImport struct {
	Task    string
	Summary string
}

type ImportTasksInput struct {
	Tasks  []TaskImport
	DryRun bool
}

type ImportTasksFromMarkdownInput struct {
	Markdown []byte
	DryRun   bool
}

type ImportTasksResult struct {
	DryRun bool
	Tasks  []ImportedTask
}

type ImportedTask struct {
	ID      string
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
	validated, err := validateTaskInput(input.Task, input.Summary, "task add")
	if err != nil {
		return taskqueue.Task{}, err
	}

	tasks, err := openTaskStore(ctx, cfg)
	if err != nil {
		return taskqueue.Task{}, err
	}
	defer tasks.Close()

	return tasks.AddTask(ctx, taskqueue.TaskSpec{
		Task:    validated.Task,
		Summary: validated.Summary,
	})
}

func ParseTaskImport(markdown []byte) ([]TaskImport, error) {
	specs, err := taskimport.Parse(markdown)
	if err != nil {
		return nil, fmt.Errorf("task import: parse: %w", err)
	}

	tasks := make([]TaskImport, 0, len(specs))
	for _, spec := range specs {
		tasks = append(tasks, TaskImport{
			Task:    spec.Task,
			Summary: spec.Summary,
		})
	}
	return tasks, nil
}

func ImportTasksFromMarkdown(ctx context.Context, cfg Config, input ImportTasksFromMarkdownInput) (ImportTasksResult, error) {
	tasks, err := ParseTaskImport(input.Markdown)
	if err != nil {
		return ImportTasksResult{}, err
	}
	return ImportTasks(ctx, cfg, ImportTasksInput{
		Tasks:  tasks,
		DryRun: input.DryRun,
	})
}

func ImportTasks(ctx context.Context, cfg Config, input ImportTasksInput) (ImportTasksResult, error) {
	tasks, err := validateTaskImports(input.Tasks)
	if err != nil {
		return ImportTasksResult{}, err
	}

	result := ImportTasksResult{
		DryRun: input.DryRun,
		Tasks:  make([]ImportedTask, 0, len(tasks)),
	}
	for _, task := range tasks {
		result.Tasks = append(result.Tasks, ImportedTask{
			Task:    task.Task,
			Summary: task.Summary,
		})
	}
	if input.DryRun || len(tasks) == 0 {
		return result, nil
	}

	store, err := openTaskStore(ctx, cfg)
	if err != nil {
		return ImportTasksResult{}, err
	}
	defer store.Close()

	baseCreatedAt := time.Now().UTC()
	for i, task := range tasks {
		created, err := store.AddTask(ctx, taskqueue.TaskSpec{
			Task:      task.Task,
			Summary:   task.Summary,
			CreatedAt: baseCreatedAt.Add(time.Duration(i) * time.Nanosecond),
		})
		if err != nil {
			return ImportTasksResult{}, fmt.Errorf("task import: create task %d: %w", i+1, err)
		}
		result.Tasks[i].ID = created.ID
	}
	return result, nil
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

func validateTaskImports(tasks []TaskImport) ([]AddTaskInput, error) {
	validated := make([]AddTaskInput, 0, len(tasks))
	for i, task := range tasks {
		input, err := validateTaskInput(task.Task, task.Summary, fmt.Sprintf("task import: task %d", i+1))
		if err != nil {
			return nil, err
		}
		validated = append(validated, input)
	}
	return validated, nil
}

func validateTaskInput(task string, summary string, operation string) (AddTaskInput, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return AddTaskInput{}, fmt.Errorf("%s: task text is required", operation)
	}
	return AddTaskInput{
		Task:    task,
		Summary: strings.TrimSpace(summary),
	}, nil
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
