package autonomousstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"

	"revolvr/internal/autonomous"
	"revolvr/internal/taskfile"
)

type BlockCommitRequest struct {
	TaskID                   string
	Expected                 ExpectedState
	PreviousState, NextState autonomous.ExecutionState
	History                  BlockHistoryRecord
}

type BlockCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     BlockHistorySnapshot
}

func (s *Store) LoadBlockOperation(ctx context.Context, taskID, operationID string) (BlockHistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return BlockHistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return BlockHistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return BlockHistorySnapshot{}, false, err
	}
	return s.readBlockOperation(task, operationID)
}

func (s *Store) CommitBlock(ctx context.Context, request BlockCommitRequest) (BlockCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return BlockCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return BlockCommitResult{}, errors.New("commit block transition: exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return BlockCommitResult{}, err
	}
	if err := request.NextState.Validate(); err != nil {
		return BlockCommitResult{}, err
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return BlockCommitResult{}, err
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID || request.History.TaskID != request.TaskID {
		return BlockCommitResult{}, errors.New("commit block transition: task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return BlockCommitResult{}, err
	}
	if err := validateBlockDelta(request.PreviousState, request.NextState, request.History); err != nil {
		return BlockCommitResult{}, fmt.Errorf("commit block transition: %w", err)
	}
	previousRaw, err := MarshalState(request.PreviousState)
	if err != nil {
		return BlockCommitResult{}, err
	}
	nextRaw, err := MarshalState(request.NextState)
	if err != nil {
		return BlockCommitResult{}, err
	}
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousRaw)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextRaw)
	if previousIdentity.SHA256 != request.Expected.SHA256 || previousIdentity.ByteSize != request.Expected.ByteSize || request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return BlockCommitResult{}, errors.New("commit block transition: state identities do not match exact canonical bytes")
	}
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return BlockCommitResult{}, err
	}
	lockFile, err := s.openLock(filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return BlockCommitResult{}, err
	}
	defer lockFile.Close()
	if err := flockContext(ctx, lockFile); err != nil {
		return BlockCommitResult{}, err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	existing, historyFound, err := s.readBlockOperation(task, request.History.OperationID)
	if err != nil {
		return BlockCommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return BlockCommitResult{}, err
	}
	if historyFound {
		if err := sameBlockOperation(existing.Record, request.History); err != nil {
			return BlockCommitResult{}, err
		}
		if currentFound && blockTransitionApplied(current.State, request.History) {
			return BlockCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return BlockCommitResult{}, err
	}
	history := existing
	if !historyFound {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return BlockCommitResult{}, err
		}
		path := blockHistoryPath(request.TaskID, request.History.OperationID)
		raw, err := MarshalBlockHistory(request.History)
		if err != nil {
			return BlockCommitResult{}, err
		}
		created, err := s.writeImmutable(path, raw, "block history", FailureDuringHistoryWrite)
		if err != nil {
			return BlockCommitResult{}, err
		}
		if !created {
			return BlockCommitResult{}, fmt.Errorf("%w: block history appeared concurrently", ErrOperationConflict)
		}
		if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(path)))); err != nil {
			return BlockCommitResult{}, err
		}
		history = BlockHistorySnapshot{Record: request.History, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: path}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return BlockCommitResult{}, err
	}
	statePath, err := s.safePath(task.AutonomousStatePath)
	if err != nil {
		return BlockCommitResult{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(statePath), ".state.json.tmp-*")
	if err != nil {
		return BlockCommitResult{}, err
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
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureDuringStateWrite); err != nil {
		return BlockCommitResult{}, err
	}
	if _, err := temp.Write(nextRaw); err != nil {
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureStateFileSync); err != nil {
		return BlockCommitResult{}, err
	}
	if err := temp.Sync(); err != nil {
		return BlockCommitResult{}, err
	}
	if err := temp.Close(); err != nil {
		closed = true
		return BlockCommitResult{}, err
	}
	closed = true
	latest, found, err := s.readCurrent(task)
	if err != nil {
		return BlockCommitResult{}, err
	}
	if err := compareExpected(request.Expected, latest, found); err != nil {
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureBeforeStateRename); err != nil {
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureStateRename); err != nil {
		return BlockCommitResult{}, err
	}
	if err := os.Rename(tempPath, statePath); err != nil {
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureAfterStateRename); err != nil {
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureStateDirectorySync); err != nil {
		return BlockCommitResult{}, err
	}
	if err := syncDirectory(filepath.Dir(statePath)); err != nil {
		return BlockCommitResult{}, err
	}
	if err := s.fail(FailureStateReadback); err != nil {
		return BlockCommitResult{}, err
	}
	readback, found, err := s.readCurrent(task)
	if err != nil || !found || readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize || !blockTransitionApplied(readback.State, request.History) {
		return BlockCommitResult{}, errors.Join(err, errors.New("commit block transition: state readback mismatch"))
	}
	return BlockCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readBlockOperation(task taskfile.Task, operationID string) (BlockHistorySnapshot, bool, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "block"))
	dir, err := s.safePath(dirRel)
	if err != nil {
		return BlockHistorySnapshot{}, false, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return BlockHistorySnapshot{}, false, nil
	}
	if err != nil {
		return BlockHistorySnapshot{}, false, err
	}
	var matches []BlockHistorySnapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(dirRel, entry.Name()))
		abs, err := s.safePath(rel)
		if err != nil {
			return BlockHistorySnapshot{}, false, err
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			return BlockHistorySnapshot{}, false, err
		}
		record, err := DecodeBlockHistory(raw)
		if err != nil {
			return BlockHistorySnapshot{}, false, err
		}
		if record.TaskID != task.ID {
			return BlockHistorySnapshot{}, false, errors.New("block history task association mismatch")
		}
		if record.OperationID == operationID {
			matches = append(matches, BlockHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
		}
	}
	if len(matches) == 0 {
		return BlockHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return BlockHistorySnapshot{}, false, fmt.Errorf("%w: duplicate block operation %q", ErrOperationConflict, operationID)
	}
	return matches[0], true, nil
}

func blockHistoryPath(taskID, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "block", operationHash(operationID)+".json"))
}

func validateBlockDelta(previous, next autonomous.ExecutionState, record BlockHistoryRecord) error {
	want := previous
	want.Lifecycle = autonomous.LifecycleStateBlocked
	want.LatestDecision = &record.Decision
	want.Terminal = &autonomous.TerminalDetail{Reason: record.Reason, Evidence: append([]autonomous.EvidenceReference(nil), record.Evidence...)}
	if !reflect.DeepEqual(want, next) {
		return errors.New("explicit block transition changed unrelated state")
	}
	return nil
}

func blockTransitionApplied(state autonomous.ExecutionState, record BlockHistoryRecord) bool {
	return state.Lifecycle == autonomous.LifecycleStateBlocked && state.LatestDecision != nil && *state.LatestDecision == record.Decision && state.Terminal != nil && state.Terminal.Reason == record.Reason && reflect.DeepEqual(state.Terminal.Evidence, record.Evidence)
}
