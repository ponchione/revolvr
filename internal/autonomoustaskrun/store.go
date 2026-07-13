package autonomoustaskrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"revolvr/internal/runtimepath"
)

type FailurePoint string

const (
	FailureBeforeOperationHistory FailurePoint = "before_operation_history"
	FailureAfterOperationHistory  FailurePoint = "after_operation_history"
	FailureBeforeOperationRename  FailurePoint = "before_operation_rename"
	FailureAfterOperationRename   FailurePoint = "after_operation_rename"
)

type FailureInjector func(FailurePoint) error

func operationDir(root, id string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "task-runs", id)
}

// Inspect loads one durable operation without acquiring its execution lease.
// Callers may use it only to recover the pinned identity before Run acquires
// the lease and performs the authoritative compatibility checks.
func Inspect(repositoryRoot, operationID string) (Operation, bool, error) {
	if !safeID(operationID) {
		return Operation{}, false, errors.New("task run: safe operation ID is required")
	}
	root, err := runtimepath.CanonicalRoot(repositoryRoot)
	if err != nil {
		return Operation{}, false, err
	}
	return loadOperation(root, operationID)
}
func loadOperation(root, id string) (Operation, bool, error) {
	dir := operationDir(root, id)
	if err := runtimepath.CheckDir(root, dir, true); err != nil {
		return Operation{}, false, err
	}
	var candidates []Operation
	checkpoint := filepath.Join(dir, "operation.json")
	if op, found, err := readOperationFile(root, checkpoint, id); err != nil {
		return Operation{}, false, err
	} else if found {
		candidates = append(candidates, op)
	}
	historyDir := filepath.Join(dir, "history")
	if err := runtimepath.CheckDir(root, historyDir, true); err != nil {
		return Operation{}, false, err
	}
	entries, err := os.ReadDir(historyDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Operation{}, false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		op, found, err := readOperationFile(root, filepath.Join(historyDir, entry.Name()), id)
		if err != nil {
			return Operation{}, false, err
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
		return operationStageOrder(candidates[i].Stage) > operationStageOrder(candidates[j].Stage)
	})
	return candidates[0], true, nil
}
func readOperationFile(root, path, operationID string) (Operation, bool, error) {
	if err := runtimepath.CheckFile(root, path, true); err != nil {
		return Operation{}, false, err
	}
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
	canonicalRaw, err := canonical(op)
	if err != nil || string(raw) != string(canonicalRaw) || op.OperationID != operationID || op.Validate() != nil {
		return Operation{}, false, errors.New("task run: operation checkpoint/history is not canonical or has wrong identity")
	}
	return op, true, nil
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

func persist(root string, previous, next Operation, injectors ...FailureInjector) error {
	if err := next.Validate(); err != nil || next.Sequence < previous.Sequence {
		return errors.New("task run: invalid operation transition")
	}
	dir := operationDir(root, next.OperationID)
	history := filepath.Join(dir, "history")
	if err := runtimepath.EnsureDir(root, history, 0o700); err != nil {
		return err
	}
	raw, err := canonical(next)
	if err != nil {
		return err
	}
	historyPath := filepath.Join(history, fmt.Sprintf("%020d-%s.json", next.Sequence, next.Stage))
	if err := injectFailure(injectors, FailureBeforeOperationHistory); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, historyPath, true); err != nil {
		return err
	}
	if prior, err := os.ReadFile(historyPath); err == nil {
		if string(prior) != string(raw) {
			return errors.New("task run: immutable history conflict")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err := writeExclusive(root, historyPath, raw); err != nil {
		return err
	}
	if err := injectFailure(injectors, FailureAfterOperationHistory); err != nil {
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
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, name, tmp); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := injectFailure(injectors, FailureBeforeOperationRename); err != nil {
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
	if err := injectFailure(injectors, FailureAfterOperationRename); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, checkpoint, false); err != nil {
		return err
	}
	if err := runtimepath.CheckDir(root, dir, false); err != nil {
		return err
	}
	return syncDir(dir)
}

func injectFailure(injectors []FailureInjector, point FailurePoint) error {
	if len(injectors) == 0 || injectors[0] == nil {
		return nil
	}
	return injectors[0](point)
}
func writeExclusive(root, path string, raw []byte) error {
	if err := runtimepath.CheckFile(root, path, true); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, path, f); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, path, false); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func removeProtectedTemp(root, path string) {
	if err := runtimepath.CheckFile(root, path, false); err == nil {
		_ = os.Remove(path)
	}
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
