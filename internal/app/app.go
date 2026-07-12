package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/ledger"
	"revolvr/internal/passpolicy"
	"revolvr/internal/receipt"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskimport"
	"revolvr/internal/taskmodel"
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
	Tasks        []taskmodel.Task
	RecentRuns   []ledger.Run
	LatestEvents []ledger.Event
}

func AddTask(ctx context.Context, cfg Config, input AddTaskInput) (taskmodel.Task, error) {
	validated, err := validateTaskInput(input.Task, input.Summary, "task add")
	if err != nil {
		return taskmodel.Task{}, err
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return taskmodel.Task{}, err
	}

	task, err := taskfile.Create(paths.WorkDir, taskfile.CreateInput{
		Title: taskFileTitle(validated),
		Body:  validated.Task,
	})
	if err != nil {
		return taskmodel.Task{}, err
	}
	return taskFromFileTask(task)
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

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return ImportTasksResult{}, err
	}

	for i, task := range tasks {
		created, err := taskfile.Create(paths.WorkDir, taskfile.CreateInput{
			Title: taskFileTitle(task),
			Body:  task.Task,
		})
		if err != nil {
			return ImportTasksResult{}, fmt.Errorf("task import: create task %d: %w", i+1, err)
		}
		result.Tasks[i].ID = created.ID
	}
	return result, nil
}

func ListTasks(ctx context.Context, cfg Config) ([]taskmodel.Task, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	return listTaskFilesAsTasks(paths.WorkDir)
}

func RetryTask(ctx context.Context, cfg Config, taskID string) (taskmodel.Task, error) {
	return unblockBlockedTask(ctx, cfg, taskID, "task retry")
}

func UnblockTask(ctx context.Context, cfg Config, taskID string) (taskmodel.Task, error) {
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

	taskList, err := listTaskFilesAsTasks(paths.WorkDir)
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

func listTaskFilesAsTasks(workDir string) ([]taskmodel.Task, error) {
	tasks, err := taskfile.List(workDir)
	if err != nil {
		return nil, err
	}

	result := make([]taskmodel.Task, 0, len(tasks))
	nextRunnable := nextRunnableTaskIndex(tasks)
	for i, task := range tasks {
		adapted, err := taskFromFileTask(task)
		if err != nil {
			return nil, err
		}
		adapted.NextRunnable = i == nextRunnable
		result = append(result, adapted)
	}
	return result, nil
}

func nextRunnableTaskIndex(tasks []taskfile.Task) int {
	next := -1
	for i, task := range tasks {
		if task.Status != taskfile.StatusPending || task.Workflow != taskfile.WorkflowMixedPassV1 {
			continue
		}
		if next == -1 || taskRunsBefore(task, tasks[next]) {
			next = i
		}
	}
	return next
}

func taskRunsBefore(left taskfile.Task, right taskfile.Task) bool {
	if left.HasPriority && right.HasPriority && left.Priority != right.Priority {
		return left.Priority < right.Priority
	}
	if left.HasPriority != right.HasPriority {
		return left.HasPriority
	}
	return filepath.Base(left.SourcePath) < filepath.Base(right.SourcePath)
}

func taskFromFileTask(task taskfile.Task) (taskmodel.Task, error) {
	if task.Workflow == taskfile.WorkflowAutonomousV1 {
		return taskmodel.Task{
			ID:       task.ID,
			Task:     task.ContextBody,
			Status:   task.Status,
			Summary:  task.Title,
			Workflow: task.Workflow,
		}, nil
	}
	policy, err := passpolicy.Lookup(task.Workflow, task.Phase)
	if err != nil {
		return taskmodel.Task{}, fmt.Errorf("resolve workflow state for task %q: %w", task.ID, err)
	}

	nextState := policy.NextPhase
	if policy.CompletesTask {
		nextState = taskmodel.StatusCompleted
	}
	return taskmodel.Task{
		ID:         task.ID,
		Task:       task.ContextBody,
		Status:     task.Status,
		Summary:    task.Title,
		Workflow:   policy.Workflow,
		Phase:      policy.Phase,
		RunProfile: policy.ProfileName,
		NextState:  nextState,
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

func unblockBlockedTask(ctx context.Context, cfg Config, rawTaskID string, operation string) (taskmodel.Task, error) {
	taskID := strings.TrimSpace(rawTaskID)
	if taskID == "" {
		return taskmodel.Task{}, fmt.Errorf("%s: task id is required", operation)
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return taskmodel.Task{}, err
	}

	task, changed, err := taskfile.UpdateBlockedToPending(paths.WorkDir, taskID)
	if err != nil {
		return taskmodel.Task{}, err
	}
	if !changed {
		if task.ID == "" {
			return taskmodel.Task{}, fmt.Errorf("task %q not found", taskID)
		}
		return taskmodel.Task{}, fmt.Errorf("task %q is not blocked (status: %s)", taskID, task.Status)
	}
	return taskFromFileTask(task)
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

func taskFileTitle(input AddTaskInput) string {
	if input.Summary != "" {
		return input.Summary
	}
	for _, line := range strings.Split(input.Task, "\n") {
		title := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if title != "" {
			return title
		}
	}
	return input.Task
}

type statePaths struct {
	WorkDir      string
	StateDir     string
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
		LedgerDBPath: filepath.Join(stateDir, "ledger.sqlite"),
	}, nil
}

func stateInitialized(paths statePaths) (bool, error) {
	return pathsInitialized(paths, paths.LedgerDBPath)
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
