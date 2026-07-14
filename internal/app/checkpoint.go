package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"revolvr/internal/autonomousarchive"
	"revolvr/internal/operatorcheckpoint"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskmodel"
	"revolvr/internal/taskschedule"
	"revolvr/internal/taskscheduler"
)

type FulfillCheckpointInput struct {
	TaskID      string
	ReceiptPath string
	Operator    string
}

type FulfillCheckpointResult struct {
	Task          taskmodel.Task
	ReceiptPath   string
	ReceiptSHA256 string
	Replayed      bool
	Schedule      taskscheduler.Result
}

// FulfillCheckpoint validates operator, receipt, canonical task, and graph
// authority before publishing one atomic task-metadata transition. It never
// invokes a worker or creates a Git commit.
func FulfillCheckpoint(ctx context.Context, cfg Config, input FulfillCheckpointInput) (FulfillCheckpointResult, error) {
	input.TaskID = strings.TrimSpace(input.TaskID)
	receiptPath := strings.TrimSpace(input.ReceiptPath)
	operator := strings.TrimSpace(input.Operator)
	if input.TaskID == "" {
		return FulfillCheckpointResult{}, errors.New("checkpoint fulfill: task id is required")
	}
	if receiptPath == "" {
		return FulfillCheckpointResult{}, errors.New("checkpoint fulfill: --receipt is required")
	}
	if input.ReceiptPath != receiptPath {
		return FulfillCheckpointResult{}, errors.New("checkpoint fulfill: --receipt must not have surrounding whitespace")
	}
	input.ReceiptPath = receiptPath
	if operator == "" {
		return FulfillCheckpointResult{}, errors.New("checkpoint fulfill: --operator is required")
	}
	if input.Operator != operator {
		return FulfillCheckpointResult{}, errors.New("checkpoint fulfill: --operator must not have surrounding whitespace")
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return FulfillCheckpointResult{}, err
	}
	runs, closeRuns, err := openSchedulingLedger(ctx, paths)
	if err != nil {
		return FulfillCheckpointResult{}, err
	}
	defer closeRuns()
	snapshot, err := loadTaskSchedule(ctx, paths.WorkDir, runs)
	if err != nil {
		return FulfillCheckpointResult{}, err
	}
	task, found := snapshot.Task(input.TaskID)
	if !found {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: task %q not found", input.TaskID)
	}
	if task.Workflow != taskfile.WorkflowOperatorCheckpointV1 {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: task %q uses workflow %q, not %q", task.ID, task.Workflow, taskfile.WorkflowOperatorCheckpointV1)
	}
	if input.ReceiptPath != task.CheckpointReceiptPath {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: --receipt %q does not match canonical checkpoint path %q", input.ReceiptPath, task.CheckpointReceiptPath)
	}
	receipt, err := operatorcheckpoint.Load(paths.WorkDir, input.ReceiptPath, task.ID)
	if err != nil {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: %w", err)
	}
	if receipt.Receipt.Operator != operator {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: --operator %q does not match receipt operator %q", operator, receipt.Receipt.Operator)
	}
	if task.Status == taskfile.StatusCompleted && task.CheckpointReceiptSHA256 != receipt.SHA256 {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: conflicting replay for task %q: bound receipt identity %s does not match current receipt identity %s", task.ID, task.CheckpointReceiptSHA256, receipt.SHA256)
	}
	if !snapshot.Result.Valid() {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: task graph is invalid: %s", schedulingDiagnosticSummary(snapshot.Result.InvalidGraph))
	}

	// Re-read immediately before publication so ordinary receipt replacement is
	// detected without mutating the canonical task bytes.
	currentReceipt, err := operatorcheckpoint.Load(paths.WorkDir, input.ReceiptPath, task.ID)
	if err != nil {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: revalidate receipt: %w", err)
	}
	if currentReceipt.SHA256 != receipt.SHA256 {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: receipt changed during validation from %s to %s", receipt.SHA256, currentReceipt.SHA256)
	}
	updated, changed, err := taskfile.FulfillOperatorCheckpoint(paths.WorkDir, task, receipt.SHA256)
	if err != nil {
		return FulfillCheckpointResult{}, err
	}

	refreshed, err := loadTaskSchedule(ctx, paths.WorkDir, runs)
	if err != nil {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: reload schedule: %w", err)
	}
	if !refreshed.Result.Valid() {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: completed checkpoint failed schedule revalidation: %s", schedulingDiagnosticSummary(refreshed.Result.InvalidGraph))
	}
	readiness, found := refreshed.Readiness(updated)
	if !found || readiness.Reason != taskscheduler.ReasonCompleted {
		return FulfillCheckpointResult{}, fmt.Errorf("checkpoint fulfill: completed checkpoint %q was not verified by scheduler re-evaluation", task.ID)
	}
	projected, err := taskFromFileTask(updated)
	if err != nil {
		return FulfillCheckpointResult{}, err
	}
	projected.Readiness = readiness.Reason
	projected.ReadinessReason = string(readiness.Reason)
	projected.CheckpointState = taskmodel.CheckpointStateFulfilled
	return FulfillCheckpointResult{
		Task:          projected,
		ReceiptPath:   receipt.Path,
		ReceiptSHA256: receipt.SHA256,
		Replayed:      !changed,
		Schedule:      refreshed.Result,
	}, nil
}

func schedulingDiagnosticSummary(diagnostics []taskscheduler.Diagnostic) string {
	values := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		values = append(values, fmt.Sprintf("%s: %s", diagnostic.Code, strings.Join(strings.Fields(diagnostic.Detail), " ")))
	}
	if len(values) == 0 {
		return "unknown invalid graph"
	}
	return strings.Join(values, "; ")
}

func loadTaskSchedule(ctx context.Context, workDir string, runs autonomousarchive.Ledger) (taskschedule.Snapshot, error) {
	runCfg, err := LoadRunOnceConfig(workDir, DefaultRunOnceConfig(workDir))
	if err != nil {
		return taskschedule.Snapshot{}, err
	}
	return taskschedule.Load(ctx, taskschedule.Config{
		RepositoryRoot:    workDir,
		SelectionWorkflow: taskscheduler.WorkflowMixedPassV1,
		Ledger:            runs,
		GitExecutable:     runCfg.GitExecutable,
		GitTimeout:        runCfg.GitTimeout,
		ForbiddenValues:   archiveSecretValues(runCfg),
	})
}
