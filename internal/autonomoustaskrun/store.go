package autonomoustaskrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"revolvr/internal/lock"
	"revolvr/internal/runtimepath"
)

type FailurePoint string

const (
	FailureBeforeOperationHistory        FailurePoint = "before_operation_history"
	FailureAfterOperationHistoryOpen     FailurePoint = "after_operation_history_open"
	FailureBeforeOperationHistoryPublish FailurePoint = "before_operation_history_publish"
	FailureAfterOperationHistoryPublish  FailurePoint = "after_operation_history_publish"
	FailureAfterOperationHistory         FailurePoint = "after_operation_history"
	FailureAfterOperationCheckpointOpen  FailurePoint = "after_operation_checkpoint_open"
	FailureBeforeOperationFileSync       FailurePoint = "before_operation_file_sync"
	FailureBeforeOperationRename         FailurePoint = "before_operation_rename"
	FailureAfterOperationRename          FailurePoint = "after_operation_rename"
	FailureBeforeOperationDirectorySync  FailurePoint = "before_operation_directory_sync"
	FailureBeforeOperationReadback       FailurePoint = "before_operation_readback"
	FailureBeforeOperationCleanup        FailurePoint = "before_operation_cleanup"
)

type FailureInjector func(FailurePoint) error

type taskRunStore struct {
	directory   *runtimepath.Directory
	history     *runtimepath.Directory
	lease       *lock.Flock
	operationID string
	inject      FailureInjector
}

func operationDir(root, id string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "task-runs", id)
}

func openTaskRunStore(boundary runtimepath.Boundary, operationID string, lease *lock.Flock, inject FailureInjector) (*taskRunStore, bool, error) {
	if !safeID(operationID) {
		return nil, false, errors.New("task run: safe operation ID is required")
	}
	directory, found, err := boundary.OpenDir(operationDir(boundary.Root(), operationID), true)
	if err != nil || !found {
		return nil, found, err
	}
	store := &taskRunStore{directory: directory, lease: lease, operationID: operationID, inject: inject}
	if err := store.check(); err != nil {
		_ = directory.Close()
		return nil, false, err
	}
	return store, true, nil
}

func (s *taskRunStore) Close() error {
	if s == nil {
		return nil
	}
	var historyErr, directoryErr error
	if s.history != nil {
		historyErr = s.history.Close()
		s.history = nil
	}
	if s.directory != nil {
		directoryErr = s.directory.Close()
		s.directory = nil
	}
	return errors.Join(historyErr, directoryErr)
}

func (s *taskRunStore) check() error {
	if s == nil || s.directory == nil {
		return errors.New("task run: operation store is closed")
	}
	if err := s.directory.Check(); err != nil {
		return err
	}
	if s.lease != nil {
		if err := s.lease.Check(); err != nil {
			return fmt.Errorf("task run: validate operation lease: %w", err)
		}
	}
	return nil
}

func (s *taskRunStore) requireMutation() error {
	if s.lease == nil {
		return errors.New("task run: persistence requires the operation lease")
	}
	return s.check()
}

func (s *taskRunStore) historyDirectory(create bool) (*runtimepath.Directory, bool, error) {
	if s.history != nil {
		if err := s.history.Check(); err != nil {
			return nil, false, err
		}
		if err := s.check(); err != nil {
			return nil, false, err
		}
		return s.history, true, nil
	}
	if create {
		if err := s.requireMutation(); err != nil {
			return nil, false, err
		}
		history, err := s.directory.EnsureDir("history", 0o700)
		if err != nil {
			return nil, false, err
		}
		s.history = history
		if err := s.requireMutation(); err != nil {
			return nil, false, err
		}
		return history, true, nil
	}
	history, found, err := s.directory.OpenDir("history", true)
	if err != nil || !found {
		return nil, found, err
	}
	s.history = history
	if err := s.check(); err != nil {
		return nil, false, err
	}
	return history, true, nil
}

// Inspect loads one durable operation without acquiring its execution lease.
// Callers may use it only to recover the pinned identity before Run acquires
// the lease and performs the authoritative compatibility checks.
func Inspect(repositoryRoot, operationID string) (Operation, bool, error) {
	if !safeID(operationID) {
		return Operation{}, false, errors.New("task run: safe operation ID is required")
	}
	boundary, err := runtimepath.Bind(repositoryRoot)
	if err != nil {
		return Operation{}, false, err
	}
	store, found, err := openTaskRunStore(boundary, operationID, nil, nil)
	if err != nil || !found {
		return Operation{}, found, err
	}
	defer store.Close()
	return store.load()
}

func (s *taskRunStore) load() (Operation, bool, error) {
	if err := s.check(); err != nil {
		return Operation{}, false, err
	}
	var candidates []Operation
	if op, found, err := s.readOperationFile(s.directory, "operation.json"); err != nil {
		return Operation{}, false, err
	} else if found {
		candidates = append(candidates, op)
	}
	history, found, err := s.historyDirectory(false)
	if err != nil {
		return Operation{}, false, err
	}
	if found {
		entries, err := history.ReadDir()
		if err != nil {
			return Operation{}, false, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			op, found, err := s.readOperationFile(history, entry.Name())
			if err != nil {
				return Operation{}, false, err
			}
			if found {
				candidates = append(candidates, op)
			}
		}
		if err := history.Check(); err != nil {
			return Operation{}, false, err
		}
	}
	if err := s.check(); err != nil {
		return Operation{}, false, err
	}
	if len(candidates) == 0 {
		return Operation{}, false, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Sequence != candidates[j].Sequence {
			return candidates[i].Sequence > candidates[j].Sequence
		}
		return operationStageOrder(candidates[i].Stage) > operationStageOrder(candidates[j].Stage)
	})
	return candidates[0], true, nil
}

func (s *taskRunStore) readOperationFile(directory *runtimepath.Directory, name string) (Operation, bool, error) {
	raw, found, err := s.readRaw(directory, name)
	if err != nil || !found {
		return Operation{}, found, err
	}
	var op Operation
	if err := json.Unmarshal(raw, &op); err != nil {
		return Operation{}, false, err
	}
	canonicalRaw, err := canonical(op)
	if err != nil || string(raw) != string(canonicalRaw) || op.OperationID != s.operationID || op.Validate() != nil {
		return Operation{}, false, errors.New("task run: operation checkpoint/history is not canonical or has wrong identity")
	}
	return op, true, nil
}

func (s *taskRunStore) readRaw(directory *runtimepath.Directory, name string) ([]byte, bool, error) {
	if err := s.check(); err != nil {
		return nil, false, err
	}
	raw, found, err := directory.ReadFile(name, true)
	if err != nil || !found {
		return nil, found, err
	}
	if err := s.check(); err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func operationStageOrder(stage string) int {
	switch stage {
	case "admitted":
		return 0
	case "cycle_started":
		return 1
	case "cycle_completed":
		return 2
	case "terminal":
		return 3
	default:
		return -1
	}
}

// persist remains the package test/setup entry point. Production runs retain
// one store and lease across every transition instead of reacquiring here.
func persist(root string, previous, next Operation, injectors ...FailureInjector) error {
	if err := validateTransition(previous, next); err != nil {
		return err
	}
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return err
	}
	lease, err := lockOperation(context.Background(), boundary, next.OperationID)
	if err != nil {
		return err
	}
	defer lease.Close()
	var inject FailureInjector
	if len(injectors) > 0 {
		inject = injectors[0]
	}
	store, found, err := openTaskRunStore(boundary, next.OperationID, lease, inject)
	if err != nil || !found {
		return errors.Join(err, errors.New("task run: prepared operation directory is missing"))
	}
	defer store.Close()
	return store.persist(previous, next)
}

func (s *taskRunStore) persist(previous, next Operation) error {
	if err := validateTransition(previous, next); err != nil {
		return err
	}
	history, _, err := s.historyDirectory(true)
	if err != nil {
		return err
	}
	raw, err := canonical(next)
	if err != nil {
		return err
	}
	historyName := fmt.Sprintf("%020d-%s.json", next.Sequence, next.Stage)
	if err := s.fail(FailureBeforeOperationHistory); err != nil {
		return err
	}
	if prior, found, err := s.readRaw(history, historyName); err != nil {
		return err
	} else if found {
		if string(prior) != string(raw) {
			return errors.New("task run: immutable history conflict")
		}
	} else if err := s.publishHistory(history, historyName, raw); err != nil {
		return err
	}
	if err := s.fail(FailureAfterOperationHistory); err != nil {
		return err
	}
	return s.replaceCheckpoint(raw)
}

func validateTransition(previous, next Operation) error {
	if err := next.Validate(); err != nil || next.Sequence < previous.Sequence {
		return errors.New("task run: invalid operation transition")
	}
	return nil
}

func (s *taskRunStore) publishHistory(history *runtimepath.Directory, name string, raw []byte) (err error) {
	if err := s.requireMutation(); err != nil {
		return err
	}
	temp, err := history.CreateTemp(".history-", 0o600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(history, temp))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := s.fail(FailureAfterOperationHistoryOpen); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationFileSync); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationHistoryPublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	linkErr := history.Link(temp, name)
	published = temp.IsNamed(name)
	if linkErr != nil {
		return linkErr
	}
	if err := s.fail(FailureAfterOperationHistoryPublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationDirectorySync); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := history.Sync(); err != nil {
		return err
	}
	return s.requireMutation()
}

func (s *taskRunStore) replaceCheckpoint(raw []byte) (err error) {
	const name = "operation.json"
	if err := s.requireMutation(); err != nil {
		return err
	}
	temp, err := s.directory.CreateTemp(".operation-", 0o600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(s.directory, temp))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := s.fail(FailureAfterOperationCheckpointOpen); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationFileSync); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationRename); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	replaceErr := s.directory.Replace(temp, name)
	published = temp.IsNamed(name)
	if replaceErr != nil {
		return replaceErr
	}
	if err := s.fail(FailureAfterOperationRename); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationDirectorySync); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	if err := s.directory.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeOperationReadback); err != nil {
		return err
	}
	if err := s.requireMutation(); err != nil {
		return err
	}
	observed, found, err := s.directory.ReadFile(name, false)
	if err != nil {
		return err
	}
	if !found || string(observed) != string(raw) {
		return errors.New("task run: strict checkpoint readback failed")
	}
	return s.requireMutation()
}

func (s *taskRunStore) cleanupTemp(directory *runtimepath.Directory, temp *runtimepath.File) error {
	faultErr := s.fail(FailureBeforeOperationCleanup)
	if err := temp.Check(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := s.requireMutation(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := directory.Remove(temp); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := s.requireMutation(); err != nil {
		return errors.Join(faultErr, err)
	}
	return errors.Join(faultErr, directory.Sync())
}

func (s *taskRunStore) fail(point FailurePoint) error {
	if s.inject == nil {
		return nil
	}
	return s.inject(point)
}
