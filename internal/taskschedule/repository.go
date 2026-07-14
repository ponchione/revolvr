// Package taskschedule assembles one read-only repository scheduling snapshot
// from canonical task, autonomous lifecycle, and verified archive evidence.
// Scheduling policy remains exclusively in the pure taskscheduler package.
package taskschedule

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"revolvr/internal/autonomousarchive"
	"revolvr/internal/operatorcheckpoint"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

type Config struct {
	RepositoryRoot    string
	SelectionWorkflow taskscheduler.Workflow
	Ledger            autonomousarchive.Ledger
	GitExecutable     string
	GitTimeout        time.Duration
	CommandRunner     autonomousarchive.CommandRunner
	ForbiddenValues   []string
}

type Snapshot struct {
	Tasks  []taskfile.Task
	Result taskscheduler.Result
}

func (s Snapshot) Task(taskID string) (taskfile.Task, bool) {
	for _, task := range s.Tasks {
		if task.ID == taskID {
			return task, true
		}
	}
	return taskfile.Task{}, false
}

func (s Snapshot) Readiness(task taskfile.Task) (taskscheduler.TaskReadiness, bool) {
	for _, readiness := range s.Result.Tasks {
		if readiness.TaskID == task.ID && readiness.SourcePath == filepath.ToSlash(task.SourcePath) {
			return readiness, true
		}
	}
	return taskscheduler.TaskReadiness{}, false
}

func Load(ctx context.Context, cfg Config) (Snapshot, error) {
	active, err := LoadActive(ctx, cfg.RepositoryRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load canonical task graph: %w", err)
	}
	fileTasks := make([]taskfile.Task, len(active))
	for i := range active {
		fileTasks[i] = active[i].Task
	}

	input := taskscheduler.Input{
		Tasks:             make([]taskscheduler.Task, 0, len(fileTasks)),
		SelectionWorkflow: cfg.SelectionWorkflow,
	}
	for i, fileTask := range fileTasks {
		state := taskscheduler.State(fileTask.Status)
		if fileTask.Workflow == taskfile.WorkflowAutonomousV1 && active[i].Lifecycle != "" {
			state = taskscheduler.State(active[i].Lifecycle)
		}
		taskInput := taskscheduler.Task{
			ID:          fileTask.ID,
			Workflow:    taskscheduler.Workflow(fileTask.Workflow),
			State:       state,
			SourcePath:  fileTask.SourcePath,
			Priority:    fileTask.Priority,
			HasPriority: fileTask.HasPriority,
			DependsOn:   append([]string(nil), fileTask.DependsOn...),
			Conflicts:   append([]string(nil), fileTask.Conflicts...),
		}
		if fileTask.Workflow == taskfile.WorkflowOperatorCheckpointV1 {
			taskInput.Checkpoint = &taskscheduler.CheckpointAuthority{
				ReceiptPath:   fileTask.CheckpointReceiptPath,
				ReceiptSHA256: fileTask.CheckpointReceiptSHA256,
			}
			if fileTask.Status == taskfile.StatusPending {
				taskInput.State = taskscheduler.StateAwaitingOperator
			} else {
				receipt, receiptErr := operatorcheckpoint.Load(cfg.RepositoryRoot, fileTask.CheckpointReceiptPath, fileTask.ID)
				switch {
				case receiptErr != nil:
					taskInput.Checkpoint.Detail = receiptErr.Error()
				case receipt.SHA256 != fileTask.CheckpointReceiptSHA256:
					taskInput.Checkpoint.Detail = fmt.Sprintf("bound receipt identity %s does not match current receipt identity %s", fileTask.CheckpointReceiptSHA256, receipt.SHA256)
				default:
					taskInput.Checkpoint.Verified = true
				}
			}
		}
		input.Tasks = append(input.Tasks, taskInput)
	}

	archives, err := VerifyArchives(ctx, cfg)
	if err != nil {
		return Snapshot{}, err
	}
	input.Archives = archives
	return Snapshot{Tasks: fileTasks, Result: taskscheduler.Evaluate(input)}, nil
}

func VerifyArchives(ctx context.Context, cfg Config) ([]taskscheduler.Archive, error) {
	entries, err := autonomousarchive.List(cfg.RepositoryRoot)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	if cfg.Ledger == nil {
		return nil, errors.New("verify archived scheduling authority: initialized ledger is required")
	}
	result := make([]taskscheduler.Archive, 0, len(entries))
	for _, entry := range entries {
		report, verifyErr := autonomousarchive.Verify(ctx, autonomousarchive.VerifyConfig{
			RepositoryRoot:  cfg.RepositoryRoot,
			Ledger:          cfg.Ledger,
			GitExecutable:   cfg.GitExecutable,
			GitTimeout:      cfg.GitTimeout,
			CommandRunner:   cfg.CommandRunner,
			ForbiddenValues: append([]string(nil), cfg.ForbiddenValues...),
		}, entry.Manifest.ArchiveID)
		if verifyErr != nil {
			return nil, fmt.Errorf("verify archived scheduling authority %q: %w", entry.Manifest.TaskID, verifyErr)
		}
		result = append(result, taskscheduler.Archive{
			TaskID:      entry.Manifest.TaskID,
			ArchiveID:   entry.Manifest.ArchiveID,
			Disposition: taskscheduler.State(entry.Manifest.Disposition),
			Reason:      entry.Manifest.Reason,
			Verified:    report.Passed,
			Reconciled:  report.Passed,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TaskID < result[j].TaskID })
	return result, nil
}
