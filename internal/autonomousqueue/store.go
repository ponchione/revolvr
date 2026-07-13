package autonomousqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"revolvr/internal/runtimepath"
)

type FailurePoint string

const (
	FailureBeforeHistory FailurePoint = "before_history"
	FailureAfterHistory  FailurePoint = "after_history"
	FailureBeforeRename  FailurePoint = "before_rename"
	FailureAfterRename   FailurePoint = "after_rename"
	FailureAfterLockOpen FailurePoint = "after_lock_open"
)

type FailureInjector func(FailurePoint) error

func Inspect(repositoryRoot, operationID string) (Operation, bool, error) {
	root, err := runtimepath.CanonicalRoot(repositoryRoot)
	if err != nil {
		return Operation{}, false, err
	}
	if !safeID(operationID) {
		return Operation{}, false, errors.New("autonomous queue: safe operation ID is required")
	}
	return load(root, operationID)
}

func load(root, operationID string) (Operation, bool, error) {
	dir := queueDir(root, operationID)
	if err := runtimepath.CheckDir(root, dir, true); err != nil {
		return Operation{}, false, err
	}
	var candidates []Operation
	if op, found, err := readOperation(root, filepath.Join(dir, "operation.json"), operationID); err != nil {
		return Operation{}, false, err
	} else if found {
		candidates = append(candidates, op)
	}
	historyDir := filepath.Join(dir, "history")
	entries, _, err := runtimepath.ReadDir(root, historyDir, true)
	if err != nil {
		return Operation{}, false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return Operation{}, false, errors.New("autonomous queue: foreign history entry")
		}
		op, found, readErr := readOperation(root, filepath.Join(historyDir, entry.Name()), operationID)
		if readErr != nil {
			return Operation{}, false, readErr
		}
		if found {
			candidates = append(candidates, op)
		}
	}
	if len(candidates) == 0 {
		return Operation{}, false, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Sequence != candidates[j].Sequence {
			return candidates[i].Sequence > candidates[j].Sequence
		}
		return stageOrder(candidates[i].Stage) > stageOrder(candidates[j].Stage)
	})
	return candidates[0], true, nil
}

func readOperation(root, path, operationID string) (Operation, bool, error) {
	raw, found, err := runtimepath.ReadFile(root, path, true)
	if err != nil || !found {
		return Operation{}, found, err
	}
	var op Operation
	if err := json.Unmarshal(raw, &op); err != nil {
		return Operation{}, false, err
	}
	want, err := canonical(op)
	if err != nil || string(raw) != string(want) || op.OperationID != operationID || op.Validate() != nil {
		return Operation{}, false, errors.New("autonomous queue: checkpoint/history is malformed, divergent, or noncanonical")
	}
	return op, true, nil
}

func persist(root string, previous, next Operation, inject FailureInjector) error {
	if err := next.Validate(); err != nil || next.Sequence < previous.Sequence {
		return errors.Join(err, errors.New("autonomous queue: invalid transition"))
	}
	dir := queueDir(root, next.OperationID)
	historyDir := filepath.Join(dir, "history")
	if err := runtimepath.EnsureDir(root, historyDir, 0o700); err != nil {
		return err
	}
	raw, err := canonical(next)
	if err != nil {
		return err
	}
	historyPath := filepath.Join(historyDir, fmt.Sprintf("%020d-%s.json", next.Sequence, next.Stage))
	if err := injectAt(inject, FailureBeforeHistory); err != nil {
		return err
	}
	if prior, found, readErr := runtimepath.ReadFile(root, historyPath, true); readErr == nil && found {
		if string(prior) != string(raw) {
			return errors.New("autonomous queue: immutable history conflict")
		}
	} else if readErr != nil {
		return readErr
	} else if err := writeExclusive(root, historyPath, raw); err != nil {
		return err
	}
	if err := injectAt(inject, FailureAfterHistory); err != nil {
		return err
	}
	checkpoint := filepath.Join(dir, "operation.json")
	if err := runtimepath.CheckFile(root, checkpoint, true); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".operation-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer removeProtectedTemp(root, name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, name, tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := injectAt(inject, FailureBeforeRename); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, checkpoint, true); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, name, false); err != nil {
		return err
	}
	if err := os.Rename(name, checkpoint); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, checkpoint, false); err != nil {
		return err
	}
	if err := runtimepath.SyncDir(root, dir); err != nil {
		return err
	}
	if err := injectAt(inject, FailureAfterRename); err != nil {
		return err
	}
	readback, found, err := readOperation(root, checkpoint, next.OperationID)
	if err != nil || !found || readback.Sequence != next.Sequence || readback.Stage != next.Stage {
		return errors.Join(err, errors.New("autonomous queue: strict checkpoint readback failed"))
	}
	return nil
}

func lockOperation(ctx context.Context, root, operationID string, injectors ...FailureInjector) (func(), error) {
	dir := queueDir(root, operationID)
	if err := runtimepath.EnsureDir(root, dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "operation.lock")
	if err := runtimepath.CheckFile(root, path, true); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	if len(injectors) > 0 {
		if err := injectAt(injectors[0], FailureAfterLockOpen); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	if err := runtimepath.CheckOpenedFile(root, path, f); err != nil {
		_ = f.Close()
		return nil, err
	}
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			if err := runtimepath.CheckOpenedFile(root, path, f); err != nil {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
				return nil, err
			}
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func queueDir(root, operationID string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "queues", operationID)
}

func writeExclusive(root, path string, raw []byte) error {
	if err := runtimepath.CheckFile(root, path, true); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, path, f); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, path, false); err != nil {
		return err
	}
	return runtimepath.SyncDir(root, filepath.Dir(path))
}

func removeProtectedTemp(root, path string) {
	if err := runtimepath.CheckFile(root, path, false); err == nil {
		_ = os.Remove(path)
	}
}

func injectAt(inject FailureInjector, point FailurePoint) error {
	if inject == nil {
		return nil
	}
	return inject(point)
}
