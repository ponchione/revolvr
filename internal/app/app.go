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
	"revolvr/internal/taskscheduler"
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
	Task      string
	Summary   string
	DependsOn []string
	Tags      []string
	Conflicts []string
}

type TaskImport struct {
	Task      string
	Summary   string
	DependsOn []string
	Tags      []string
	Conflicts []string
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
	ID        string
	Task      string
	Summary   string
	DependsOn []string
	Tags      []string
	Conflicts []string
}

type StatusResult struct {
	Initialized  bool
	Tasks        []taskmodel.Task
	Schedule     taskscheduler.Result
	RecentRuns   []ledger.Run
	LatestEvents []ledger.Event
}

func AddTask(ctx context.Context, cfg Config, input AddTaskInput) (taskmodel.Task, error) {
	validated, err := validateTaskInput(input.Task, input.Summary, "task add")
	if err != nil {
		return taskmodel.Task{}, err
	}
	if err := taskfile.ValidateSchedulingMetadata(input.DependsOn, input.Tags, input.Conflicts); err != nil {
		return taskmodel.Task{}, fmt.Errorf("task add scheduling metadata: %w", err)
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return taskmodel.Task{}, err
	}

	task, err := taskfile.Create(paths.WorkDir, taskfile.CreateInput{
		Title:     taskFileTitle(validated),
		Body:      validated.Task,
		DependsOn: input.DependsOn, Tags: input.Tags, Conflicts: input.Conflicts,
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
			Task:      spec.Task,
			Summary:   spec.Summary,
			DependsOn: append([]string(nil), spec.DependsOn...), Tags: append([]string(nil), spec.Tags...), Conflicts: append([]string(nil), spec.Conflicts...),
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
			Task:      task.Task,
			Summary:   task.Summary,
			DependsOn: append([]string(nil), task.DependsOn...), Tags: append([]string(nil), task.Tags...), Conflicts: append([]string(nil), task.Conflicts...),
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
			Title:     taskFileTitle(task),
			Body:      task.Task,
			DependsOn: task.DependsOn, Tags: task.Tags, Conflicts: task.Conflicts,
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
	runs, closeRuns, err := openSchedulingLedger(ctx, paths)
	if err != nil {
		return nil, err
	}
	defer closeRuns()
	tasks, _, err := listTaskFilesAsTasks(ctx, paths.WorkDir, runs)
	return tasks, err
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

	runs, err := openReadOnlyLedger(ctx, paths)
	if err != nil {
		return StatusResult{}, err
	}
	defer runs.Close()
	taskList, schedule, err := listTaskFilesAsTasks(ctx, paths.WorkDir, runs)
	if err != nil {
		return StatusResult{}, err
	}

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
		Schedule:     schedule,
		RecentRuns:   recentRuns,
		LatestEvents: latestEvents,
	}, nil
}

func listTaskFilesAsTasks(ctx context.Context, workDir string, runs *ledger.Store) ([]taskmodel.Task, taskscheduler.Result, error) {
	snapshot, err := loadTaskSchedule(ctx, workDir, runs)
	if err != nil {
		return nil, taskscheduler.Result{}, err
	}
	mixedSelected, hasMixedSelected := snapshot.Result.SelectedForWorkflow(taskscheduler.WorkflowMixedPassV1)
	autonomousSelected, hasAutonomousSelected := snapshot.Result.SelectedForWorkflow(taskscheduler.WorkflowAutonomousV1)
	result := make([]taskmodel.Task, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		adapted, err := taskFromFileTask(task)
		if err != nil {
			return nil, taskscheduler.Result{}, err
		}
		readiness, found := snapshot.Readiness(task)
		if !found {
			return nil, taskscheduler.Result{}, fmt.Errorf("task schedule omitted canonical task %q at %s", task.ID, task.SourcePath)
		}
		adapted.Readiness = readiness.Reason
		adapted.ReadinessReason = string(readiness.Reason)
		if task.Workflow == taskfile.WorkflowOperatorCheckpointV1 {
			switch {
			case task.Status == taskfile.StatusPending:
				adapted.CheckpointState = taskmodel.CheckpointStateAwaiting
			case readiness.Reason == taskscheduler.ReasonCompleted:
				adapted.CheckpointState = taskmodel.CheckpointStateFulfilled
			default:
				adapted.CheckpointState = taskmodel.CheckpointStateInvalid
			}
		}
		adapted.WaitingDependencyIDs = append([]string(nil), readiness.UnmetDependencyIDs...)
		adapted.DependencyIssues = append([]taskscheduler.DependencyIssue(nil), readiness.DependencyIssues...)
		adapted.ConflictBlockers = append([]string(nil), readiness.ConflictingTaskOrKeys...)
		adapted.SchedulingDiagnostics = cloneSchedulingDiagnostics(snapshot.Result.InvalidGraph)
		adapted.NextRunnable = hasMixedSelected && readiness.TaskID == mixedSelected.TaskID && readiness.SourcePath == mixedSelected.SourcePath
		adapted.NextAutonomous = hasAutonomousSelected && readiness.TaskID == autonomousSelected.TaskID && readiness.SourcePath == autonomousSelected.SourcePath
		adapted.SelectedNext = snapshot.Result.SelectedNext != nil && readiness.TaskID == snapshot.Result.SelectedNext.TaskID && readiness.SourcePath == snapshot.Result.SelectedNext.SourcePath
		adapted.AutonomousReady = task.Workflow == taskfile.WorkflowAutonomousV1 && readiness.Reason == taskscheduler.ReasonReady
		result = append(result, adapted)
	}
	return result, snapshot.Result, nil
}

func openSchedulingLedger(ctx context.Context, paths statePaths) (*ledger.Store, func(), error) {
	initialized, err := ledgerInitialized(paths)
	if err != nil || !initialized {
		return nil, func() {}, err
	}
	runs, err := openReadOnlyLedger(ctx, paths)
	if err != nil {
		return nil, nil, err
	}
	return runs, func() { _ = runs.Close() }, nil
}

func openReadOnlyLedger(ctx context.Context, paths statePaths) (*ledger.Store, error) {
	return ledger.OpenLiveReadOnly(ctx, paths.LedgerDBPath)
}

func cloneSchedulingDiagnostics(input []taskscheduler.Diagnostic) []taskscheduler.Diagnostic {
	result := make([]taskscheduler.Diagnostic, len(input))
	for i, diagnostic := range input {
		diagnostic.Cycle = append([]string(nil), diagnostic.Cycle...)
		result[i] = diagnostic
	}
	return result
}

func taskFromFileTask(task taskfile.Task) (taskmodel.Task, error) {
	if task.Workflow == taskfile.WorkflowAutonomousV1 || task.Workflow == taskfile.WorkflowOperatorCheckpointV1 {
		return taskmodel.Task{
			ID:                    task.ID,
			Task:                  task.ContextBody,
			Status:                task.Status,
			Summary:               task.Title,
			Workflow:              task.Workflow,
			DependsOn:             append([]string(nil), task.DependsOn...),
			Tags:                  append([]string(nil), task.Tags...),
			Conflicts:             append([]string(nil), task.Conflicts...),
			ParentTaskID:          task.ParentTaskID,
			CheckpointReceiptPath: task.CheckpointReceiptPath,
			CheckpointReceiptSHA:  task.CheckpointReceiptSHA256,
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
		ID:           task.ID,
		Task:         task.ContextBody,
		Status:       task.Status,
		Summary:      task.Title,
		Workflow:     policy.Workflow,
		Phase:        policy.Phase,
		RunProfile:   policy.ProfileName,
		NextState:    nextState,
		DependsOn:    append([]string(nil), task.DependsOn...),
		Tags:         append([]string(nil), task.Tags...),
		Conflicts:    append([]string(nil), task.Conflicts...),
		ParentTaskID: task.ParentTaskID,
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

	runs, err := openReadOnlyLedger(ctx, paths)
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

	runs, err := openReadOnlyLedger(ctx, paths)
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
		if err := taskfile.ValidateSchedulingMetadata(task.DependsOn, task.Tags, task.Conflicts); err != nil {
			return nil, fmt.Errorf("task import: task %d scheduling metadata: %w", i+1, err)
		}
		input.DependsOn = append([]string(nil), task.DependsOn...)
		input.Tags = append([]string(nil), task.Tags...)
		input.Conflicts = append([]string(nil), task.Conflicts...)
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
