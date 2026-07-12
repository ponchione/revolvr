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
)

type FailurePoint string

const (
	FailureBeforeHistory FailurePoint = "before_history"
	FailureAfterHistory  FailurePoint = "after_history"
	FailureBeforeRename  FailurePoint = "before_rename"
	FailureAfterRename   FailurePoint = "after_rename"
)

type FailureInjector func(FailurePoint) error

func Inspect(repositoryRoot, operationID string) (Operation, bool, error) {
	root, err := canonicalRoot(repositoryRoot)
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
	var candidates []Operation
	if op, found, err := readOperation(filepath.Join(dir, "operation.json"), operationID); err != nil {
		return Operation{}, false, err
	} else if found {
		candidates = append(candidates, op)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "history"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Operation{}, false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return Operation{}, false, errors.New("autonomous queue: foreign history entry")
		}
		op, found, readErr := readOperation(filepath.Join(dir, "history", entry.Name()), operationID)
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

func readOperation(path, operationID string) (Operation, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Operation{}, false, nil
	}
	if err != nil {
		return Operation{}, false, err
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
	if err := ensureNoSymlinkParents(root, historyDir); err != nil {
		return err
	}
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
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
	if prior, readErr := os.ReadFile(historyPath); readErr == nil {
		if string(prior) != string(raw) {
			return errors.New("autonomous queue: immutable history conflict")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	} else if err := writeExclusive(historyPath, raw); err != nil {
		return err
	}
	if err := injectAt(inject, FailureAfterHistory); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".operation-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
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
	if err := os.Rename(name, filepath.Join(dir, "operation.json")); err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	if err := injectAt(inject, FailureAfterRename); err != nil {
		return err
	}
	readback, found, err := readOperation(filepath.Join(dir, "operation.json"), next.OperationID)
	if err != nil || !found || readback.Sequence != next.Sequence || readback.Stage != next.Stage {
		return errors.Join(err, errors.New("autonomous queue: strict checkpoint readback failed"))
	}
	return nil
}

func lockOperation(ctx context.Context, root, operationID string) (func(), error) {
	dir := queueDir(root, operationID)
	if err := ensureNoSymlinkParents(root, dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "operation.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
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

func canonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func queueDir(root, operationID string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "queues", operationID)
}

func ensureNoSymlinkParents(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || filepath.IsAbs(rel) {
		return errors.New("autonomous queue: unsafe runtime path")
	}
	current := root
	for _, part := range splitPath(rel) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("autonomous queue: symlinked runtime namespace")
		}
	}
	return nil
}

func splitPath(path string) []string {
	var result []string
	for path != "." && path != "" {
		dir, base := filepath.Split(path)
		result = append([]string{base}, result...)
		path = filepath.Clean(dir)
	}
	return result
}

func writeExclusive(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
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
	return syncDir(filepath.Dir(path))
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func injectAt(inject FailureInjector, point FailurePoint) error {
	if inject == nil {
		return nil
	}
	return inject(point)
}
