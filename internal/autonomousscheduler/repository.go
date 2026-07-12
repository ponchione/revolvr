package autonomousscheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"revolvr/internal/autonomousstate"
	"revolvr/internal/taskfile"
)

// LoadActive reads one exact duplicate-checked active-task snapshot and the
// matching autonomous lifecycle snapshots. Archive authority is deliberately
// supplied separately by the AW-21 verification owner.
func LoadActive(ctx context.Context, repositoryRoot string) ([]ActiveTask, error) {
	return loadActive(ctx, repositoryRoot, false)
}

// LoadActiveStrict additionally requires canonical state for every active
// autonomous task and is used for execution admission and child publication.
func LoadActiveStrict(ctx context.Context, repositoryRoot string) ([]ActiveTask, error) {
	return loadActive(ctx, repositoryRoot, true)
}

func loadActive(ctx context.Context, repositoryRoot string, requireState bool) ([]ActiveTask, error) {
	tasks, err := taskfile.List(repositoryRoot)
	if err != nil {
		return nil, err
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repositoryRoot})
	if err != nil {
		return nil, err
	}
	result := make([]ActiveTask, 0, len(tasks))
	for _, task := range tasks {
		item := ActiveTask{Task: task}
		if task.Workflow == taskfile.WorkflowAutonomousV1 {
			snapshot, found, loadErr := store.Load(ctx, task.ID)
			if loadErr != nil && loadErr != autonomousstate.ErrStateMissing {
				return nil, fmt.Errorf("scheduler: load state for %q: %w", task.ID, loadErr)
			}
			if found {
				item.Lifecycle = string(snapshot.State.Lifecycle)
				item.StateSHA256 = snapshot.SHA256
				item.StateByteSize = snapshot.ByteSize
				if snapshot.State.ChildOf != nil {
					journalPath := filepath.Join(repositoryRoot, ".revolvr", "autonomous", "child-publications", snapshot.State.ChildOf.OperationID+".json")
					raw, readErr := os.ReadFile(journalPath)
					if readErr != nil {
						return nil, fmt.Errorf("scheduler: child task %q publication authority is incomplete: %w", task.ID, readErr)
					}
					var journal struct {
						SchemaVersion string `json:"schema_version"`
						Stage         string `json:"stage"`
						OperationID   string `json:"operation_id"`
					}
					if decodeErr := json.Unmarshal(raw, &journal); decodeErr != nil || journal.SchemaVersion != "autonomous-child-publication-v1" || journal.OperationID != snapshot.State.ChildOf.OperationID || journal.Stage != "completed" {
						return nil, fmt.Errorf("scheduler: child task %q publication authority is incomplete or malformed", task.ID)
					}
				}
			} else if requireState {
				return nil, fmt.Errorf("scheduler: autonomous task %q has no canonical state", task.ID)
			}
		}
		result = append(result, item)
	}
	return result, nil
}
