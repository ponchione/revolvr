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
	checkpoint, checkpointFound, err := readOperation(root, filepath.Join(dir, "operation.json"), operationID)
	if err != nil {
		return Operation{}, false, err
	}
	history, err := readHistory(root, filepath.Join(dir, "history"), operationID)
	if err != nil {
		return Operation{}, false, err
	}
	if len(history) == 0 {
		if checkpointFound {
			return Operation{}, false, errors.New("autonomous queue: checkpoint is not backed by history")
		}
		return Operation{}, false, nil
	}
	if checkpointFound {
		if checkpoint.Sequence >= int64(len(history)) || !canonicalEqual(checkpoint, history[checkpoint.Sequence]) {
			return Operation{}, false, errors.New("autonomous queue: checkpoint is ahead of or conflicts with history")
		}
	}
	return history[len(history)-1], true, nil
}

func readHistory(root, historyDir, operationID string) ([]Operation, error) {
	entries, _, err := runtimepath.ReadDir(root, historyDir, true)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	history := make([]Operation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil, errors.New("autonomous queue: foreign history entry")
		}
		op, found, err := readOperation(root, filepath.Join(historyDir, entry.Name()), operationID)
		if err != nil {
			return nil, err
		}
		if !found || entry.Name() != historyName(op) || op.Sequence != int64(len(history)) {
			return nil, errors.New("autonomous queue: history filename or sequence is not canonical and contiguous")
		}
		if len(history) == 0 {
			if err := validateInitialOperation(op); err != nil {
				return nil, err
			}
		} else if err := validateTransition(history[len(history)-1], op); err != nil {
			return nil, err
		}
		history = append(history, op)
	}
	return history, nil
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
	if err := next.Validate(); err != nil {
		return errors.Join(err, errors.New("autonomous queue: invalid transition successor"))
	}
	dir := queueDir(root, next.OperationID)
	historyDir := filepath.Join(dir, "history")
	if err := runtimepath.EnsureDir(root, historyDir, 0o700); err != nil {
		return err
	}
	history, err := readHistory(root, historyDir, next.OperationID)
	if err != nil {
		return err
	}
	if previous.OperationID == "" {
		if len(history) != 0 {
			return errors.New("autonomous queue: initial transition conflicts with existing history")
		}
		if err := validateInitialOperation(next); err != nil {
			return err
		}
	} else {
		if len(history) == 0 {
			return errors.New("autonomous queue: transition predecessor is not backed by history")
		}
		current := history[len(history)-1]
		if previous.OperationID != current.OperationID || previous.Sequence != current.Sequence || previous.Stage != current.Stage {
			return errors.New("autonomous queue: transition predecessor is not current history")
		}
		if err := validateTransition(current, next); err != nil {
			return err
		}
	}
	raw, err := canonical(next)
	if err != nil {
		return err
	}
	historyPath := filepath.Join(historyDir, historyName(next))
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
	if err != nil || !found || !canonicalEqual(readback, next) {
		return errors.Join(err, errors.New("autonomous queue: strict checkpoint readback failed"))
	}
	return nil
}

func historyName(op Operation) string {
	return fmt.Sprintf("%020d-%s.json", op.Sequence, op.Stage)
}

func validateInitialOperation(op Operation) error {
	if err := op.Validate(); err != nil {
		return errors.Join(err, errors.New("autonomous queue: invalid initial transition"))
	}
	if op.Sequence != 0 || op.Stage != "admitted" || !op.UpdatedAt.Equal(op.StartedAt) ||
		op.CompletedAt != nil || op.LastFingerprint != "" || op.InFlight != nil || len(op.Slots) != 0 ||
		op.SequentialFallback != "" || len(op.Outcomes) != 0 || len(op.Exclusions) != 0 ||
		op.Statistics.Selections != 0 || op.Statistics.TasksRun != 0 || op.Statistics.Batches != 0 ||
		op.Statistics.PeakActiveWorkers != 0 || op.Statistics.SequentialFallbacks != 0 || len(op.Statistics.Outcomes) != 0 ||
		len(op.RemainingReady) != 0 || len(op.RemainingWaiting) != 0 || op.StopReason != "" || op.StopDetail != "" {
		return errors.New("autonomous queue: initial history entry is not a pristine admission")
	}
	return nil
}

func validateTransition(previous, next Operation) error {
	if err := previous.Validate(); err != nil {
		return errors.Join(err, errors.New("autonomous queue: invalid transition predecessor"))
	}
	if err := next.Validate(); err != nil {
		return errors.Join(err, errors.New("autonomous queue: invalid transition successor"))
	}
	if next.Sequence != previous.Sequence+1 || next.UpdatedAt.Before(previous.UpdatedAt) || !sameOperationMaterial(previous, next) || previous.Stage == "terminal" {
		return errors.New("autonomous queue: transition sequence, time, material, or terminal boundary is invalid")
	}
	if previous.SchemaVersion != next.SchemaVersion {
		return validateLegacyMigration(previous, next)
	}
	switch previous.Stage {
	case "admitted":
		if next.Stage == "selected" {
			return validateSelectionTransition(previous, next)
		}
		if next.Stage == "terminal" {
			return validateTerminalTransition(previous, next)
		}
	case "selected":
		if next.Stage == "task_stopped" {
			return validateTaskStoppedTransition(previous, next)
		}
	case "task_stopped":
		switch next.Stage {
		case "selected":
			return validateSelectionTransition(previous, next)
		case "task_stopped":
			return validateTaskStoppedTransition(previous, next)
		case "terminal":
			return validateTerminalTransition(previous, next)
		}
	}
	return fmt.Errorf("autonomous queue: illegal stage transition %s -> %s", previous.Stage, next.Stage)
}

func sameOperationMaterial(previous, next Operation) bool {
	return previous.OperationID == next.OperationID &&
		previous.Mode == next.Mode &&
		previous.ConfigSchema == next.ConfigSchema &&
		previous.ConfigSHA256 == next.ConfigSHA256 &&
		previous.SafetyIdentity == next.SafetyIdentity &&
		previous.MaxTasks == next.MaxTasks &&
		effectiveMaximumWorkers(previous) == effectiveMaximumWorkers(next) &&
		previous.StartedAt.Equal(next.StartedAt) &&
		previous.Sweep == next.Sweep &&
		previous.DaemonWakeCount == next.DaemonWakeCount &&
		previous.DaemonWakeFingerprint == next.DaemonWakeFingerprint
}

func effectiveMaximumWorkers(op Operation) int {
	if op.SchemaVersion == LegacyOperationSchemaVersion {
		return 1
	}
	return op.MaximumWorkers
}

func validateLegacyMigration(previous, next Operation) error {
	if previous.SchemaVersion != LegacyOperationSchemaVersion || next.SchemaVersion != OperationSchemaVersion || previous.Stage != next.Stage || next.MaximumWorkers != 1 {
		return errors.New("autonomous queue: illegal operation schema transition")
	}
	before, after := previous, next
	before.SchemaVersion, after.SchemaVersion = "", ""
	before.MaximumWorkers, after.MaximumWorkers = 0, 0
	before.InFlight, after.InFlight = nil, nil
	before.Slots, after.Slots = nil, nil
	before.Sequence, after.Sequence = 0, 0
	before.UpdatedAt, after.UpdatedAt = time.Time{}, time.Time{}
	if !canonicalEqual(before, after) {
		return errors.New("autonomous queue: legacy migration changed operation evidence")
	}
	if previous.InFlight == nil {
		if len(next.Slots) != 0 {
			return errors.New("autonomous queue: legacy migration invented worker slots")
		}
		return nil
	}
	if len(next.Slots) != 1 || next.Slots[0].State != SlotAdmitted || next.Slots[0].Outcome != nil {
		return errors.New("autonomous queue: legacy migration did not preserve its in-flight selection")
	}
	want, got := *previous.InFlight, next.Slots[0].Selection
	want.Sequence = previous.Statistics.Selections + 1
	want.Batch = previous.Statistics.Selections + 1
	want.Slot = 1
	if !canonicalEqual(want, got) {
		return errors.New("autonomous queue: legacy migration changed its in-flight selection")
	}
	return nil
}

func validateSelectionTransition(previous, next Operation) error {
	if !equalOutcomes(previous.Outcomes, next.Outcomes) || previous.Statistics.Selections != next.Statistics.Selections || previous.Statistics.TasksRun != next.Statistics.TasksRun || !equalOutcomeCounts(previous.Statistics.Outcomes, next.Statistics.Outcomes) {
		return errors.New("autonomous queue: selection transition changed completed outcomes")
	}
	if next.SchemaVersion == LegacyOperationSchemaVersion {
		if next.InFlight == nil {
			return errors.New("autonomous queue: legacy selection has no in-flight task")
		}
		return nil
	}
	if previous.SchemaVersion != OperationSchemaVersion || next.Statistics.Batches != previous.Statistics.Batches+1 || len(next.Slots) == 0 {
		return errors.New("autonomous queue: selection transition has invalid batch evidence")
	}
	wantPeak := previous.Statistics.PeakActiveWorkers
	if len(next.Slots) > wantPeak {
		wantPeak = len(next.Slots)
	}
	wantFallbacks := previous.Statistics.SequentialFallbacks
	if next.SequentialFallback != "" {
		wantFallbacks++
	}
	if next.Statistics.PeakActiveWorkers != wantPeak || next.Statistics.SequentialFallbacks != wantFallbacks {
		return errors.New("autonomous queue: selection transition has conflicting concurrency statistics")
	}
	for i, slot := range next.Slots {
		if slot.State != SlotAdmitted || slot.Selection.Sequence != previous.Statistics.Selections+int64(i)+1 || slot.Selection.Batch != next.Statistics.Batches {
			return errors.New("autonomous queue: selection transition has noncanonical worker slots")
		}
	}
	return nil
}

func validateTaskStoppedTransition(previous, next Operation) error {
	if len(next.Outcomes) != len(previous.Outcomes)+1 || !equalOutcomes(previous.Outcomes, next.Outcomes[:len(previous.Outcomes)]) {
		return errors.New("autonomous queue: task-stop transition did not append one outcome")
	}
	wantStatistics := previous.Statistics
	wantStatistics.Outcomes = append([]OutcomeCount(nil), previous.Statistics.Outcomes...)
	wantStatistics.add(next.Outcomes[len(next.Outcomes)-1].StopReason)
	if !canonicalEqual(wantStatistics, next.Statistics) {
		return errors.New("autonomous queue: task-stop transition has conflicting statistics")
	}
	if next.SchemaVersion == LegacyOperationSchemaVersion {
		if previous.Stage != "selected" || previous.InFlight == nil || next.InFlight != nil {
			return errors.New("autonomous queue: illegal legacy task-stop transition")
		}
		outcome := next.Outcomes[len(next.Outcomes)-1]
		if outcome.TaskID != previous.InFlight.TaskID || outcome.TaskOperationID != previous.InFlight.TaskOperationID {
			return errors.New("autonomous queue: legacy task-stop outcome conflicts with its selection")
		}
		return nil
	}
	if previous.SchemaVersion != OperationSchemaVersion || len(previous.Slots) != len(next.Slots) {
		return errors.New("autonomous queue: task-stop transition changed worker-slot cardinality")
	}
	changed := -1
	for i := range previous.Slots {
		if previous.Slots[i].State == SlotAdmitted && next.Slots[i].State == SlotTerminal && canonicalEqual(previous.Slots[i].Selection, next.Slots[i].Selection) {
			if changed >= 0 {
				return errors.New("autonomous queue: task-stop transition completed multiple worker slots")
			}
			changed = i
			continue
		}
		if !canonicalEqual(previous.Slots[i], next.Slots[i]) {
			return errors.New("autonomous queue: task-stop transition rewrote worker-slot evidence")
		}
	}
	if changed < 0 || !canonicalEqual(next.Slots[changed].Outcome, &next.Outcomes[len(next.Outcomes)-1]) {
		return errors.New("autonomous queue: task-stop transition lacks one matching terminal slot")
	}
	return nil
}

func validateTerminalTransition(previous, next Operation) error {
	if next.CompletedAt == nil || !next.CompletedAt.Equal(next.UpdatedAt) || !equalOutcomes(previous.Outcomes, next.Outcomes) || !canonicalEqual(previous.Statistics, next.Statistics) || !equalWorkerSlots(previous.Slots, next.Slots) {
		return errors.New("autonomous queue: terminal transition changed completed evidence")
	}
	return nil
}

func canonicalEqual(left, right any) bool {
	leftRaw, leftErr := canonical(left)
	rightRaw, rightErr := canonical(right)
	return leftErr == nil && rightErr == nil && string(leftRaw) == string(rightRaw)
}

func equalOutcomes(left, right []TaskOutcome) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !canonicalEqual(left[i], right[i]) {
			return false
		}
	}
	return true
}

func equalOutcomeCounts(left, right []OutcomeCount) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalWorkerSlots(left, right []WorkerSlot) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !canonicalEqual(left[i], right[i]) {
			return false
		}
	}
	return true
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
