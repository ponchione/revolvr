package autonomousstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"revolvr/internal/autonomous"
	"revolvr/internal/taskfile"
)

type AttemptHistorySnapshot struct {
	Record     AttemptHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}

type AttemptCommitRequest struct {
	TaskID        string
	Expected      ExpectedState
	PreviousState autonomous.ExecutionState
	NextState     autonomous.ExecutionState
	History       AttemptHistoryRecord
}

type AttemptCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     AttemptHistorySnapshot
}

func (s *Store) LoadAttemptOperation(ctx context.Context, taskID, operationID string) (AttemptHistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return AttemptHistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return AttemptHistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return AttemptHistorySnapshot{}, false, err
	}
	return s.readAttemptOperation(task, operationID)
}

func (s *Store) CommitAttempt(ctx context.Context, request AttemptCommitRequest) (AttemptCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return AttemptCommitResult{}, errors.New("commit attempt transition: exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: previous state: %w", err)
	}
	if err := request.NextState.Validate(); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: next state: %w", err)
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: %w", err)
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID {
		return AttemptCommitResult{}, errors.New("commit attempt transition: state task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: invalid history: %w", err)
	}
	if request.History.TaskID != request.TaskID {
		return AttemptCommitResult{}, errors.New("commit attempt transition: history task identity mismatch")
	}
	previousBytes, err := MarshalState(request.PreviousState)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	nextBytes, err := MarshalState(request.NextState)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousBytes)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextBytes)
	if previousIdentity.SHA256 != request.Expected.SHA256 || previousIdentity.ByteSize != request.Expected.ByteSize {
		return AttemptCommitResult{}, errors.New("commit attempt transition: previous state does not match expected canonical identity")
	}
	if request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return AttemptCommitResult{}, errors.New("commit attempt transition: history state identities do not match canonical state bytes")
	}

	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return AttemptCommitResult{}, err
	}
	lockFile, err := s.openLock(filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return AttemptCommitResult{}, err
	}
	defer lockFile.Close()
	if err := flockContext(ctx, lockFile); err != nil {
		return AttemptCommitResult{}, err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	existing, historyFound, err := s.readAttemptOperation(task, request.History.OperationID)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	if historyFound {
		if err := sameAttemptOperation(existing.Record, request.History); err != nil {
			return AttemptCommitResult{}, err
		}
		if currentFound && current.SHA256 == request.History.ResultingState.SHA256 && current.ByteSize == request.History.ResultingState.ByteSize {
			return AttemptCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return AttemptCommitResult{}, err
	}
	history := existing
	if !historyFound {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return AttemptCommitResult{}, err
		}
		historyPath := attemptHistoryPath(request.TaskID, request.History.Sequence, request.History.Kind, request.History.OperationID)
		historyBytes, err := MarshalAttemptHistory(request.History)
		if err != nil {
			return AttemptCommitResult{}, err
		}
		created, err := s.writeImmutable(historyPath, historyBytes, "attempt history", FailureDuringHistoryWrite)
		if err != nil {
			return AttemptCommitResult{}, err
		}
		if !created {
			return AttemptCommitResult{}, fmt.Errorf("%w: attempt history appeared concurrently", ErrOperationConflict)
		}
		if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(historyPath)))); err != nil {
			return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: sync history directory: %w", err)
		}
		history = AttemptHistorySnapshot{Record: request.History, SHA256: hashBytes(historyBytes), ByteSize: len(historyBytes), SourcePath: historyPath}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return AttemptCommitResult{}, err
	}

	statePath, err := s.safePath(task.AutonomousStatePath)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(statePath), ".state.json.tmp-*")
	if err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: create state temporary file: %w", err)
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o644); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := s.fail(FailureDuringStateWrite); err != nil {
		return AttemptCommitResult{}, err
	}
	if _, err := temp.Write(nextBytes); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: write state temporary file: %w", err)
	}
	if err := s.fail(FailureStateFileSync); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := temp.Sync(); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: sync state temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		closed = true
		return AttemptCommitResult{}, err
	}
	closed = true
	latest, latestFound, err := s.readCurrent(task)
	if err != nil {
		return AttemptCommitResult{}, err
	}
	if err := compareExpected(request.Expected, latest, latestFound); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := s.fail(FailureBeforeStateRename); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := s.fail(FailureStateRename); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := os.Rename(tempPath, statePath); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: atomically replace state: %w", err)
	}
	if err := s.fail(FailureAfterStateRename); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := s.fail(FailureStateDirectorySync); err != nil {
		return AttemptCommitResult{}, err
	}
	if err := syncDirectory(filepath.Dir(statePath)); err != nil {
		return AttemptCommitResult{}, fmt.Errorf("commit attempt transition: sync state directory: %w", err)
	}
	if err := s.fail(FailureStateReadback); err != nil {
		return AttemptCommitResult{}, err
	}
	readback, found, err := s.readCurrent(task)
	if err != nil || !found {
		return AttemptCommitResult{}, errors.Join(err, errors.New("commit attempt transition: state readback missing"))
	}
	if readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize || !reflectStateEqual(readback.State, request.NextState) {
		return AttemptCommitResult{}, errors.New("commit attempt transition: state readback mismatch")
	}
	return AttemptCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readAttemptOperation(task taskfile.Task, operationID string) (AttemptHistorySnapshot, bool, error) {
	records, err := s.readAllAttemptHistory(task)
	if err != nil {
		return AttemptHistorySnapshot{}, false, err
	}
	var matches []AttemptHistorySnapshot
	for _, record := range records {
		if record.Record.OperationID == operationID {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return AttemptHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return AttemptHistorySnapshot{}, false, fmt.Errorf("%w: attempt operation %q has multiple history records", ErrOperationConflict, operationID)
	}
	return matches[0], true, nil
}

func (s *Store) readAllAttemptHistory(task taskfile.Task) ([]AttemptHistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "attempts"))
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
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	result := make([]AttemptHistorySnapshot, 0, len(names))
	for _, name := range names {
		rel := filepath.ToSlash(filepath.Join(dirRel, name))
		raw, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		record, err := DecodeAttemptHistory(raw)
		if err != nil {
			return nil, err
		}
		if record.TaskID != task.ID {
			return nil, errors.New("attempt history task association mismatch")
		}
		result = append(result, AttemptHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	return result, nil
}

func attemptHistoryPath(taskID string, sequence int64, kind AttemptTransitionKind, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "attempts", fmt.Sprintf("%020d-%s-%s.json", sequence, kind, operationHash(operationID))))
}
