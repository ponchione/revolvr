package autonomousscheduler

import (
	"context"
	"fmt"

	"revolvr/internal/autonomouschildpublication"
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
	publications := make(map[string]autonomouschildpublication.Projection)
	for _, task := range tasks {
		item := ActiveTask{Task: task}
		if task.Workflow == taskfile.WorkflowAutonomousV1 {
			hasChildMetadata := task.ParentTaskID != "" || task.ChildProposalID != "" || task.ChildDecisionID != "" || task.ChildRunID != "" || len(task.ChildEvidence) != 0 || task.ParentBehavior != ""
			snapshot, found, loadErr := store.Load(ctx, task.ID)
			if loadErr != nil && loadErr != autonomousstate.ErrStateMissing {
				return nil, fmt.Errorf("scheduler: load state for %q: %w", task.ID, loadErr)
			}
			if found {
				item.Lifecycle = string(snapshot.State.Lifecycle)
				item.StateSHA256 = snapshot.SHA256
				item.StateByteSize = snapshot.ByteSize
				if snapshot.State.ChildOf != nil {
					operationID := snapshot.State.ChildOf.OperationID
					publication, loaded := publications[operationID]
					if !loaded {
						var publicationFound bool
						var publicationErr error
						publication, publicationFound, publicationErr = autonomouschildpublication.Load(repositoryRoot, operationID)
						if publicationErr != nil {
							return nil, fmt.Errorf("scheduler: child task %q publication authority is incomplete: %w", task.ID, publicationErr)
						}
						if !publicationFound {
							return nil, fmt.Errorf("scheduler: child task %q publication authority is missing", task.ID)
						}
						publications[operationID] = publication
					}
					if validationErr := publication.ValidateActiveChild(task, snapshot); validationErr != nil {
						return nil, fmt.Errorf("scheduler: child task %q publication authority is incomplete or malformed: %w", task.ID, validationErr)
					}
				} else if hasChildMetadata {
					return nil, fmt.Errorf("scheduler: child task %q has no immutable child lineage", task.ID)
				}
			} else if requireState || hasChildMetadata {
				return nil, fmt.Errorf("scheduler: autonomous task %q has no canonical state", task.ID)
			}
		}
		result = append(result, item)
	}
	return result, nil
}
