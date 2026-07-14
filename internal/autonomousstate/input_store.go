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

type InputCommitRequest struct {
	TaskID                   string
	Expected                 ExpectedState
	PreviousState, NextState autonomous.ExecutionState
	History                  InputHistoryRecord
}
type InputCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     InputHistorySnapshot
}

func (s *Store) LoadInputOperation(ctx context.Context, taskID, operationID string) (InputHistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return InputHistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return InputHistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return InputHistorySnapshot{}, false, err
	}
	return s.readInputOperation(task, operationID)
}

func (s *Store) CommitInput(ctx context.Context, request InputCommitRequest) (InputCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return InputCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return InputCommitResult{}, errors.New("commit input transition: exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return InputCommitResult{}, err
	}
	if err := request.NextState.Validate(); err != nil {
		return InputCommitResult{}, err
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return InputCommitResult{}, err
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID || request.History.TaskID != request.TaskID {
		return InputCommitResult{}, errors.New("commit input transition: task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return InputCommitResult{}, err
	}
	if err := validateInputDelta(request.PreviousState.Input, request.NextState.Input, request.History); err != nil {
		return InputCommitResult{}, fmt.Errorf("commit input transition: %w", err)
	}
	previousRaw, _ := MarshalState(request.PreviousState)
	nextRaw, _ := MarshalState(request.NextState)
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousRaw)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextRaw)
	if previousIdentity.SHA256 != request.Expected.SHA256 || previousIdentity.ByteSize != request.Expected.ByteSize || request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return InputCommitResult{}, errors.New("commit input transition: state identities do not match exact canonical bytes")
	}
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return InputCommitResult{}, err
	}
	lockLease, err := s.acquireLock(ctx, filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return InputCommitResult{}, err
	}
	defer lockLease.Close()
	existing, historyFound, err := s.readInputOperation(task, request.History.OperationID)
	if err != nil {
		return InputCommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return InputCommitResult{}, err
	}
	if historyFound {
		if err := sameInputOperation(existing.Record, request.History); err != nil {
			return InputCommitResult{}, err
		}
		if currentFound && inputTransitionApplied(current.State, request.History) {
			return InputCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return InputCommitResult{}, err
	}
	history := existing
	if !historyFound {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return InputCommitResult{}, err
		}
		path := inputHistoryPath(request.TaskID, request.History.Sequence, request.History.Kind, request.History.OperationID)
		raw, err := MarshalInputHistory(request.History)
		if err != nil {
			return InputCommitResult{}, err
		}
		created, err := s.writeImmutable(path, raw, "input history", FailureDuringHistoryWrite)
		if err != nil {
			return InputCommitResult{}, err
		}
		if !created {
			return InputCommitResult{}, fmt.Errorf("%w: input history appeared concurrently", ErrOperationConflict)
		}
		if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(path)))); err != nil {
			return InputCommitResult{}, err
		}
		history = InputHistorySnapshot{Record: request.History, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: path}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return InputCommitResult{}, err
	}
	readback, found, err := s.replaceState(task, request.Expected, nextRaw)
	if err != nil {
		return InputCommitResult{}, err
	}
	if !found || readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize || !inputTransitionApplied(readback.State, request.History) {
		return InputCommitResult{}, errors.New("commit input transition: state readback mismatch")
	}
	return InputCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readInputOperation(task taskfile.Task, operationID string) (InputHistorySnapshot, bool, error) {
	records, err := s.readAllInputHistory(task)
	if err != nil {
		return InputHistorySnapshot{}, false, err
	}
	var matches []InputHistorySnapshot
	for _, r := range records {
		if r.Record.OperationID == operationID {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return InputHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return InputHistorySnapshot{}, false, fmt.Errorf("%w: duplicate input operation %q", ErrOperationConflict, operationID)
	}
	return matches[0], true, nil
}

func (s *Store) readAllInputHistory(task taskfile.Task) ([]InputHistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "input"))
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
	var result []InputHistorySnapshot
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
		record, err := DecodeInputHistory(raw)
		if err != nil {
			return nil, err
		}
		if record.TaskID != task.ID {
			return nil, errors.New("input history task association mismatch")
		}
		result = append(result, InputHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	return result, nil
}

func inputHistoryPath(taskID string, sequence int64, kind InputTransitionKind, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "input", fmt.Sprintf("%020d-%s-%s.json", sequence, kind, operationHash(operationID))))
}

func validateInputDelta(previous, next autonomous.InputState, record InputHistoryRecord) error {
	if next.TransitionSequence != previous.TransitionSequence+1 || record.Sequence != next.TransitionSequence {
		return errors.New("input transition must append exactly one next sequence")
	}
	switch record.Kind {
	case InputTransitionQuestion:
		if len(next.Questions) != len(previous.Questions)+1 || len(next.Answers) != len(previous.Answers) || len(next.Resumes) != len(previous.Resumes) || !reflect.DeepEqual(next.Questions[len(next.Questions)-1], *record.Question) {
			return errors.New("question transition changed unrelated input evidence")
		}
	case InputTransitionAnswer:
		if len(next.Questions) != len(previous.Questions) || len(next.Answers) != len(previous.Answers)+1 || len(next.Resumes) != len(previous.Resumes) || !reflect.DeepEqual(next.Answers[len(next.Answers)-1], *record.Answer) {
			return errors.New("answer transition changed unrelated input evidence")
		}
	case InputTransitionResume:
		if len(next.Questions) != len(previous.Questions) || len(next.Answers) != len(previous.Answers) || len(next.Resumes) != len(previous.Resumes)+1 || !reflect.DeepEqual(next.Resumes[len(next.Resumes)-1], *record.Resume) {
			return errors.New("resume transition changed unrelated input evidence")
		}
	}
	return nil
}
func inputTransitionApplied(state autonomous.ExecutionState, record InputHistoryRecord) bool {
	switch record.Kind {
	case InputTransitionQuestion:
		for _, v := range state.Input.Questions {
			if v.Sequence == record.Sequence {
				return reflect.DeepEqual(v, *record.Question)
			}
		}
	case InputTransitionAnswer:
		for _, v := range state.Input.Answers {
			if v.Sequence == record.Sequence {
				return reflect.DeepEqual(v, *record.Answer)
			}
		}
	case InputTransitionResume:
		for _, v := range state.Input.Resumes {
			if v.Sequence == record.Sequence {
				return reflect.DeepEqual(v, *record.Resume)
			}
		}
	}
	return false
}
