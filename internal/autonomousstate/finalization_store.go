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

type FinalizationCommitRequest struct {
	TaskID        string
	Expected      ExpectedState
	PreviousState autonomous.ExecutionState
	NextState     autonomous.ExecutionState
	History       FinalizationHistoryRecord
}

type FinalizationCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     FinalizationHistorySnapshot
}

func (s *Store) CommitFinalization(ctx context.Context, request FinalizationCommitRequest) (FinalizationCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return FinalizationCommitResult{}, errors.New("commit finalization transition: exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := request.NextState.Validate(); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return FinalizationCommitResult{}, err
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID || request.History.TaskID != request.TaskID {
		return FinalizationCommitResult{}, errors.New("commit finalization transition: task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return FinalizationCommitResult{}, err
	}
	previousRaw, _ := MarshalState(request.PreviousState)
	nextRaw, _ := MarshalState(request.NextState)
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousRaw)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextRaw)
	if previousIdentity.SHA256 != request.Expected.SHA256 || previousIdentity.ByteSize != request.Expected.ByteSize || request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return FinalizationCommitResult{}, errors.New("commit finalization transition: state identities do not match exact canonical bytes")
	}
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return FinalizationCommitResult{}, err
	}
	lockFile, err := s.openLock(filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return FinalizationCommitResult{}, err
	}
	defer lockFile.Close()
	if err := flockContext(ctx, lockFile); err != nil {
		return FinalizationCommitResult{}, err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	existing, foundHistory, err := s.readFinalizationOperation(task, request.History.OperationID, request.History.Stage)
	if err != nil {
		return FinalizationCommitResult{}, err
	}
	current, foundState, err := s.readCurrent(task)
	if err != nil {
		return FinalizationCommitResult{}, err
	}
	if foundHistory {
		if err := sameFinalizationOperation(existing.Record, request.History); err != nil {
			return FinalizationCommitResult{}, err
		}
		if foundState && current.SHA256 == request.History.ResultingState.SHA256 && current.ByteSize == request.History.ResultingState.ByteSize {
			return FinalizationCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, foundState); err != nil {
		return FinalizationCommitResult{}, err
	}
	history := existing
	if !foundHistory {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return FinalizationCommitResult{}, err
		}
		path := finalizationHistoryPath(request.TaskID, request.History.Stage, request.History.OperationID)
		raw, err := MarshalFinalizationHistory(request.History)
		if err != nil {
			return FinalizationCommitResult{}, err
		}
		created, err := s.writeImmutable(path, raw, "finalization history", FailureDuringHistoryWrite)
		if err != nil {
			return FinalizationCommitResult{}, err
		}
		if !created {
			return FinalizationCommitResult{}, fmt.Errorf("%w: finalization history appeared concurrently", ErrOperationConflict)
		}
		if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(path)))); err != nil {
			return FinalizationCommitResult{}, err
		}
		history = FinalizationHistorySnapshot{Record: request.History, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: path}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return FinalizationCommitResult{}, err
	}
	statePath, err := s.safePath(task.AutonomousStatePath)
	if err != nil {
		return FinalizationCommitResult{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(statePath), ".state.json.tmp-*")
	if err != nil {
		return FinalizationCommitResult{}, err
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
		return FinalizationCommitResult{}, err
	}
	if err := s.fail(FailureDuringStateWrite); err != nil {
		return FinalizationCommitResult{}, err
	}
	if _, err := temp.Write(nextRaw); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := s.fail(FailureStateFileSync); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := temp.Sync(); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := temp.Close(); err != nil {
		closed = true
		return FinalizationCommitResult{}, err
	}
	closed = true
	latest, found, err := s.readCurrent(task)
	if err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := compareExpected(request.Expected, latest, found); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := s.fail(FailureBeforeStateRename); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := s.fail(FailureStateRename); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := os.Rename(tempPath, statePath); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := s.fail(FailureAfterStateRename); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := syncDirectory(filepath.Dir(statePath)); err != nil {
		return FinalizationCommitResult{}, err
	}
	if err := s.fail(FailureStateReadback); err != nil {
		return FinalizationCommitResult{}, err
	}
	readback, ok, err := s.readCurrent(task)
	if err != nil || !ok || readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize {
		return FinalizationCommitResult{}, errors.Join(err, errors.New("commit finalization transition: state readback mismatch"))
	}
	return FinalizationCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readFinalizationOperation(task taskfile.Task, operationID string, stage autonomous.FinalizationStage) (FinalizationHistorySnapshot, bool, error) {
	records, err := s.readAllFinalizationHistory(task)
	if err != nil {
		return FinalizationHistorySnapshot{}, false, err
	}
	var matches []FinalizationHistorySnapshot
	for _, record := range records {
		if record.Record.OperationID == operationID && record.Record.Stage == stage {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return FinalizationHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return FinalizationHistorySnapshot{}, false, fmt.Errorf("%w: duplicate finalization operation", ErrOperationConflict)
	}
	return matches[0], true, nil
}

func (s *Store) readAllFinalizationHistory(task taskfile.Task) ([]FinalizationHistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "finalization"))
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
	var records []FinalizationHistorySnapshot
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
		record, err := DecodeFinalizationHistory(raw)
		if err != nil {
			return nil, err
		}
		if record.TaskID != task.ID {
			return nil, errors.New("finalization history task association mismatch")
		}
		records = append(records, FinalizationHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	return records, nil
}

func finalizationHistoryPath(taskID string, stage autonomous.FinalizationStage, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "finalization", string(stage)+"-"+operationHash(operationID)+".json"))
}
