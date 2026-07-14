package autonomousstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/taskfile"
)

type WorkspaceCommitRequest struct {
	TaskID                   string
	Expected                 ExpectedState
	PreviousState, NextState autonomous.ExecutionState
	History                  WorkspaceHistoryRecord
}

type WorkspaceCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     WorkspaceHistorySnapshot
}

func (s *Store) LoadWorkspaceOperation(ctx context.Context, taskID, operationID string) (WorkspaceHistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return WorkspaceHistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return WorkspaceHistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return WorkspaceHistorySnapshot{}, false, err
	}
	return s.readWorkspaceOperation(task, operationID)
}

func (s *Store) CommitWorkspace(ctx context.Context, request WorkspaceCommitRequest) (WorkspaceCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return WorkspaceCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return WorkspaceCommitResult{}, errors.New("commit workspace transition: exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return WorkspaceCommitResult{}, err
	}
	if err := request.NextState.Validate(); err != nil {
		return WorkspaceCommitResult{}, err
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return WorkspaceCommitResult{}, err
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID || request.History.TaskID != request.TaskID {
		return WorkspaceCommitResult{}, errors.New("commit workspace transition: task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return WorkspaceCommitResult{}, err
	}
	if err := validateWorkspaceDelta(request.PreviousState, request.NextState, request.History); err != nil {
		return WorkspaceCommitResult{}, err
	}
	previousRaw, _ := MarshalState(request.PreviousState)
	nextRaw, _ := MarshalState(request.NextState)
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousRaw)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextRaw)
	if previousIdentity.SHA256 != request.Expected.SHA256 || previousIdentity.ByteSize != request.Expected.ByteSize || request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return WorkspaceCommitResult{}, errors.New("commit workspace transition: state identities do not match exact canonical bytes")
	}
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return WorkspaceCommitResult{}, err
	}
	lockLease, err := s.acquireLock(ctx, filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return WorkspaceCommitResult{}, err
	}
	defer lockLease.Close()
	existing, historyFound, err := s.readWorkspaceOperation(task, request.History.OperationID)
	if err != nil {
		return WorkspaceCommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return WorkspaceCommitResult{}, err
	}
	if historyFound {
		if err := sameWorkspaceOperation(existing.Record, request.History); err != nil {
			return WorkspaceCommitResult{}, err
		}
		if currentFound && current.SHA256 == request.History.ResultingState.SHA256 && current.ByteSize == request.History.ResultingState.ByteSize {
			return WorkspaceCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return WorkspaceCommitResult{}, err
	}
	history := existing
	if !historyFound {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return WorkspaceCommitResult{}, err
		}
		path := workspaceHistoryPath(request.TaskID, request.History.Kind, request.History.OperationID)
		raw, err := MarshalWorkspaceHistory(request.History)
		if err != nil {
			return WorkspaceCommitResult{}, err
		}
		created, err := s.writeImmutable(path, raw, "workspace history", FailureDuringHistoryWrite)
		if err != nil {
			return WorkspaceCommitResult{}, err
		}
		if !created {
			return WorkspaceCommitResult{}, fmt.Errorf("%w: workspace history appeared concurrently", ErrOperationConflict)
		}
		if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(path)))); err != nil {
			return WorkspaceCommitResult{}, err
		}
		history = WorkspaceHistorySnapshot{Record: request.History, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: path}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return WorkspaceCommitResult{}, err
	}
	readback, found, err := s.replaceState(task, request.Expected, nextRaw)
	if err != nil {
		return WorkspaceCommitResult{}, err
	}
	if !found || readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize {
		return WorkspaceCommitResult{}, errors.New("commit workspace transition: state readback mismatch")
	}
	return WorkspaceCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readWorkspaceOperation(task taskfile.Task, operationID string) (WorkspaceHistorySnapshot, bool, error) {
	records, err := s.readAllWorkspaceHistory(task)
	if err != nil {
		return WorkspaceHistorySnapshot{}, false, err
	}
	var matches []WorkspaceHistorySnapshot
	for _, record := range records {
		if record.Record.OperationID == operationID {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return WorkspaceHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return WorkspaceHistorySnapshot{}, false, fmt.Errorf("%w: duplicate workspace operation %q", ErrOperationConflict, operationID)
	}
	return matches[0], true, nil
}

func (s *Store) readAllWorkspaceHistory(task taskfile.Task) ([]WorkspaceHistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "workspace"))
	dir, err := s.safePath(dirRel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result []WorkspaceHistorySnapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(dirRel, entry.Name()))
		abs, err := s.safePath(rel)
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		record, err := DecodeWorkspaceHistory(raw)
		if err != nil {
			return nil, err
		}
		if record.TaskID != task.ID {
			return nil, errors.New("workspace history task association mismatch")
		}
		result = append(result, WorkspaceHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	return result, nil
}

func workspaceHistoryPath(taskID string, kind WorkspaceTransitionKind, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "workspace", fmt.Sprintf("%s-%s.json", kind, operationHash(operationID))))
}

func validateWorkspaceDelta(previous, next autonomous.ExecutionState, record WorkspaceHistoryRecord) error {
	previousCopy, nextCopy := previous, next
	previousCopy.Workspace, nextCopy.Workspace = nil, nil
	if !reflect.DeepEqual(previousCopy, nextCopy) {
		return errors.New("commit workspace transition: transition changed non-workspace state")
	}
	if !reflect.DeepEqual(previous.Workspace, record.PreviousWorkspace) || next.Workspace == nil || !reflect.DeepEqual(*next.Workspace, record.ResultingWorkspace) {
		return errors.New("commit workspace transition: history workspace payload does not match state delta")
	}
	return nil
}
