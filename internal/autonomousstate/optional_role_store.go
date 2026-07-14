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

type OptionalRoleCommitRequest struct {
	TaskID        string
	Expected      ExpectedState
	PreviousState autonomous.ExecutionState
	NextState     autonomous.ExecutionState
	History       OptionalRoleHistoryRecord
}

type OptionalRoleCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     OptionalRoleHistorySnapshot
}

func (s *Store) LoadOptionalRoleOperation(ctx context.Context, taskID, operationID string) (OptionalRoleHistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return OptionalRoleHistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return OptionalRoleHistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return OptionalRoleHistorySnapshot{}, false, err
	}
	return s.readOptionalRoleOperation(task, operationID)
}

func (s *Store) CommitOptionalRole(ctx context.Context, request OptionalRoleCommitRequest) (OptionalRoleCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return OptionalRoleCommitResult{}, errors.New("commit optional-role transition: exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return OptionalRoleCommitResult{}, fmt.Errorf("commit optional-role transition: previous state: %w", err)
	}
	if err := request.NextState.Validate(); err != nil {
		return OptionalRoleCommitResult{}, fmt.Errorf("commit optional-role transition: next state: %w", err)
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return OptionalRoleCommitResult{}, fmt.Errorf("commit optional-role transition: %w", err)
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID || request.History.TaskID != request.TaskID {
		return OptionalRoleCommitResult{}, errors.New("commit optional-role transition: task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return OptionalRoleCommitResult{}, err
	}
	previousBytes, err := MarshalState(request.PreviousState)
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	nextBytes, err := MarshalState(request.NextState)
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousBytes)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextBytes)
	if previousIdentity.SHA256 != request.Expected.SHA256 || previousIdentity.ByteSize != request.Expected.ByteSize || request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return OptionalRoleCommitResult{}, errors.New("commit optional-role transition: state identities do not match exact canonical bytes")
	}

	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return OptionalRoleCommitResult{}, err
	}
	lockLease, err := s.acquireLock(ctx, filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	defer lockLease.Close()

	existing, historyFound, err := s.readOptionalRoleOperation(task, request.History.OperationID)
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	if historyFound {
		if err := sameOptionalRoleOperation(existing.Record, request.History); err != nil {
			return OptionalRoleCommitResult{}, err
		}
		if currentFound && optionalRoleOccurrenceApplied(current.State, request.History.Occurrence) {
			return OptionalRoleCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return OptionalRoleCommitResult{}, err
	}
	history := existing
	if !historyFound {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return OptionalRoleCommitResult{}, err
		}
		historyPath := optionalRoleHistoryPath(request.TaskID, request.History.Sequence, request.History.Occurrence.Role, request.History.OperationID)
		historyBytes, err := MarshalOptionalRoleHistory(request.History)
		if err != nil {
			return OptionalRoleCommitResult{}, err
		}
		created, err := s.writeImmutable(historyPath, historyBytes, "optional-role history", FailureDuringHistoryWrite, lockLease)
		if err != nil {
			return OptionalRoleCommitResult{}, err
		}
		if !created {
			return OptionalRoleCommitResult{}, fmt.Errorf("%w: optional-role history appeared concurrently", ErrOperationConflict)
		}
		if err := s.syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(historyPath)))); err != nil {
			return OptionalRoleCommitResult{}, err
		}
		history = OptionalRoleHistorySnapshot{Record: request.History, SHA256: hashBytes(historyBytes), ByteSize: len(historyBytes), SourcePath: historyPath}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return OptionalRoleCommitResult{}, err
	}

	readback, found, err := s.replaceState(task, request.Expected, nextBytes, lockLease)
	if err != nil {
		return OptionalRoleCommitResult{}, err
	}
	if !found || readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize || !optionalRoleOccurrenceApplied(readback.State, request.History.Occurrence) {
		return OptionalRoleCommitResult{}, errors.New("commit optional-role transition: state readback mismatch")
	}
	return OptionalRoleCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readOptionalRoleOperation(task taskfile.Task, operationID string) (OptionalRoleHistorySnapshot, bool, error) {
	records, err := s.readAllOptionalRoleHistory(task)
	if err != nil {
		return OptionalRoleHistorySnapshot{}, false, err
	}
	var matches []OptionalRoleHistorySnapshot
	for _, record := range records {
		if record.Record.OperationID == operationID {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return OptionalRoleHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return OptionalRoleHistorySnapshot{}, false, fmt.Errorf("%w: optional-role operation %q has multiple history records", ErrOperationConflict, operationID)
	}
	return matches[0], true, nil
}

func (s *Store) readAllOptionalRoleHistory(task taskfile.Task) ([]OptionalRoleHistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "optional_roles"))
	dir, found, err := s.openDir(dirRel, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	defer dir.Close()
	entries, err := dir.ReadDir()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	result := make([]OptionalRoleHistorySnapshot, 0, len(names))
	for _, name := range names {
		rel := filepath.ToSlash(filepath.Join(dirRel, name))
		raw, found, err := dir.ReadFile(name, false)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, os.ErrNotExist
		}
		record, err := DecodeOptionalRoleHistory(raw)
		if err != nil {
			return nil, err
		}
		if record.TaskID != task.ID {
			return nil, errors.New("optional-role history task association mismatch")
		}
		result = append(result, OptionalRoleHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	return result, nil
}

func optionalRoleHistoryPath(taskID string, sequence int64, role autonomous.WorkerProfile, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "optional_roles", fmt.Sprintf("%020d-%s-%s.json", sequence, role, operationHash(operationID))))
}

func optionalRoleOccurrenceApplied(state autonomous.ExecutionState, occurrence autonomous.OptionalRoleOccurrence) bool {
	index := occurrence.Sequence - 1
	return index >= 0 && index < int64(len(state.OptionalRoles)) && reflect.DeepEqual(state.OptionalRoles[index], occurrence)
}
