package autonomoustaskrun

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

// CreateReconciledOperation publishes one new admitted operation after the
// app recovery boundary has reconciled every external authority. The old
// operation is read and compared but is never locked for mutation or changed.
func CreateReconciledOperation(ctx context.Context, repositoryRoot string, old, next Operation) (Operation, bool, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, false, err
	}
	if err := old.Validate(); err != nil || old.StopReason != StopUnsafeAmbiguous {
		return Operation{}, false, errors.Join(err, errors.New("task recovery: old operation is not terminal unsafe_or_ambiguous evidence"))
	}
	if err := next.Validate(); err != nil {
		return Operation{}, false, fmt.Errorf("task recovery: validate new operation: %w", err)
	}
	if next.Stage != "admitted" || next.Sequence != 0 || next.InFlight || next.StopReason != "" || next.CompletedAt != nil ||
		next.TaskID != old.TaskID || next.OperationID == old.OperationID || next.ConfigSHA256 != old.ConfigSHA256 ||
		!reflect.DeepEqual(next.EffectiveBounds, old.EffectiveBounds) || next.MaxCycles != old.MaxCycles ||
		!containsEvidence(next.Evidence, "reconciled_from_operation:"+old.OperationID) {
		return Operation{}, false, errors.New("task recovery: new operation does not preserve exact old-operation authority")
	}
	currentOld, found, err := Inspect(repositoryRoot, old.OperationID)
	if err != nil || !found || !reflect.DeepEqual(currentOld, old) {
		return Operation{}, false, errors.Join(err, errors.New("task recovery: old operation changed during reconciliation"))
	}
	if err := reconcileCurrentAuthority(repositoryRoot, next); err != nil {
		return Operation{}, false, err
	}

	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return Operation{}, false, err
	}
	lease, err := lockOperation(ctx, boundary, next.OperationID)
	if err != nil {
		return Operation{}, false, err
	}
	defer lease.Close()
	store, found, err := openTaskRunStore(boundary, next.OperationID, lease, nil)
	if err != nil || !found {
		return Operation{}, false, errors.Join(err, errors.New("task recovery: prepared new operation directory is missing"))
	}
	defer store.Close()
	if existing, exists, loadErr := store.load(); loadErr != nil {
		return Operation{}, false, loadErr
	} else if exists {
		if existing.TaskID != next.TaskID || existing.Task != next.Task || existing.State != next.State ||
			existing.WorkspaceID != next.WorkspaceID || existing.CheckpointSHA != next.CheckpointSHA ||
			existing.ConfigSHA256 != next.ConfigSHA256 || !reflect.DeepEqual(existing.EffectiveBounds, next.EffectiveBounds) ||
			existing.MaxCycles != next.MaxCycles || !reflect.DeepEqual(existing.Evidence, next.Evidence) ||
			existing.Stage != "admitted" || existing.Sequence != 0 || existing.InFlight || existing.StopReason != "" {
			return Operation{}, false, errors.New("task recovery: deterministic new operation identity conflicts with existing evidence")
		}
		return existing, true, nil
	}
	currentOld, found, err = Inspect(repositoryRoot, old.OperationID)
	if err != nil || !found || !reflect.DeepEqual(currentOld, old) {
		return Operation{}, false, errors.Join(err, errors.New("task recovery: old operation changed before new-operation publication"))
	}
	if err := reconcileCurrentAuthority(repositoryRoot, next); err != nil {
		return Operation{}, false, err
	}
	if err := store.persist(Operation{}, next); err != nil {
		return Operation{}, false, err
	}
	return next, false, nil
}

func reconcileCurrentAuthority(repositoryRoot string, next Operation) error {
	task, found, err := taskfile.FindByID(repositoryRoot, next.TaskID)
	if err != nil || !found {
		return errors.Join(err, errors.New("task recovery: canonical task disappeared before publication"))
	}
	taskIdentity := hashBytes(task.SourceBytes)
	taskIdentity.Path = task.SourcePath
	if taskIdentity != next.Task {
		return errors.New("task recovery: canonical task changed before publication")
	}
	authority, err := currentAuthority(repositoryRoot, next.TaskID)
	if err != nil {
		return fmt.Errorf("task recovery: reload current state authority: %w", err)
	}
	if authority.State != next.State || authority.WorkspaceID != next.WorkspaceID || authority.CheckpointSHA != next.CheckpointSHA {
		return errors.New("task recovery: state or workspace authority changed before publication")
	}
	return nil
}

func containsEvidence(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
