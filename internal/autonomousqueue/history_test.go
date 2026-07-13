package autonomousqueue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueueHistoryRejectsIncompleteOrDivergentChains(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string, Operation)
	}{
		{
			name: "checkpoint_without_history",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueCheckpoint(t, root, initial)
			},
		},
		{
			name: "history_without_initial_admission",
			setup: func(t *testing.T, root string, initial Operation) {
				terminal := queueTerminalOperation(initial)
				writeQueueHistory(t, root, historyName(terminal), terminal)
			},
		},
		{
			name: "sequence_gap",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueHistory(t, root, historyName(initial), initial)
				terminal := queueTerminalOperation(initial)
				terminal.Sequence = 2
				writeQueueHistory(t, root, historyName(terminal), terminal)
			},
		},
		{
			name: "duplicate_sequence",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueHistory(t, root, historyName(initial), initial)
				terminal := queueTerminalOperation(initial)
				terminal.Sequence = 0
				writeQueueHistory(t, root, historyName(terminal), terminal)
			},
		},
		{
			name: "illegal_stage_jump",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueHistory(t, root, historyName(initial), initial)
				stopped := initial
				stopped.Sequence = 1
				stopped.Stage = "task_stopped"
				stopped.UpdatedAt = stopped.UpdatedAt.Add(time.Second)
				writeQueueHistory(t, root, historyName(stopped), stopped)
			},
		},
		{
			name: "changed_immutable_material",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueHistory(t, root, historyName(initial), initial)
				terminal := queueTerminalOperation(initial)
				terminal.ConfigSHA256 = strings.Repeat("c", 64)
				writeQueueHistory(t, root, historyName(terminal), terminal)
			},
		},
		{
			name: "foreign_canonical_json",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueHistory(t, root, historyName(initial), initial)
				writeQueueHistory(t, root, "foreign.json", queueTerminalOperation(initial))
			},
		},
		{
			name: "filename_content_mismatch",
			setup: func(t *testing.T, root string, initial Operation) {
				writeQueueHistory(t, root, historyName(initial), initial)
				terminal := queueTerminalOperation(initial)
				writeQueueHistory(t, root, "00000000000000000001-selected.json", terminal)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			initial := queuePathOperation("queue-history")
			test.setup(t, root, initial)
			if op, found, err := Inspect(root, initial.OperationID); err == nil {
				t.Fatalf("Inspect() = %+v, %v, nil; want corrupt history error", op, found)
			}
		})
	}
}

func TestQueueHistoryIsAuthorityWhenCheckpointIsMissingOrBehind(t *testing.T) {
	for _, test := range []struct {
		name       string
		checkpoint string
	}{
		{name: "missing", checkpoint: "missing"},
		{name: "behind", checkpoint: "behind"},
		{name: "equal", checkpoint: "equal"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			initial := queuePathOperation("queue-recovery")
			if err := persist(root, Operation{}, initial, nil); err != nil {
				t.Fatal(err)
			}
			terminal := queueTerminalOperation(initial)
			if err := persist(root, initial, terminal, nil); err != nil {
				t.Fatal(err)
			}
			switch test.checkpoint {
			case "missing":
				if err := os.Remove(queueCheckpointPath(root, initial.OperationID)); err != nil {
					t.Fatal(err)
				}
			case "behind":
				writeQueueCheckpoint(t, root, initial)
			}
			op, found, err := Inspect(root, initial.OperationID)
			if err != nil || !found || !canonicalEqual(op, terminal) {
				t.Fatalf("Inspect() = %+v, %v, %v; want latest history", op, found, err)
			}
		})
	}
}

func TestQueueHistoryRejectsAheadOrConflictingCheckpoint(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string, Operation)
	}{
		{
			name: "ahead",
			setup: func(t *testing.T, root string, initial Operation) {
				if err := persist(root, Operation{}, initial, nil); err != nil {
					t.Fatal(err)
				}
				writeQueueCheckpoint(t, root, queueTerminalOperation(initial))
			},
		},
		{
			name: "conflicting_behind",
			setup: func(t *testing.T, root string, initial Operation) {
				if err := persist(root, Operation{}, initial, nil); err != nil {
					t.Fatal(err)
				}
				terminal := queueTerminalOperation(initial)
				if err := persist(root, initial, terminal, nil); err != nil {
					t.Fatal(err)
				}
				initial.UpdatedAt = initial.UpdatedAt.Add(time.Second)
				writeQueueCheckpoint(t, root, initial)
			},
		},
		{
			name: "conflicting_equal",
			setup: func(t *testing.T, root string, initial Operation) {
				if err := persist(root, Operation{}, initial, nil); err != nil {
					t.Fatal(err)
				}
				terminal := queueTerminalOperation(initial)
				if err := persist(root, initial, terminal, nil); err != nil {
					t.Fatal(err)
				}
				terminal.StopDetail = "conflicting cache detail"
				writeQueueCheckpoint(t, root, terminal)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			initial := queuePathOperation("queue-checkpoint")
			test.setup(t, root, initial)
			if op, found, err := Inspect(root, initial.OperationID); err == nil {
				t.Fatalf("Inspect() = %+v, %v, nil; want checkpoint authority error", op, found)
			}
		})
	}
}

func TestQueuePersistenceRequiresNextLegalHistoryTransition(t *testing.T) {
	root := t.TempDir()
	initial := queuePathOperation("queue-persist")
	if err := persist(root, Operation{}, initial, nil); err != nil {
		t.Fatal(err)
	}
	if err := persist(root, initial, initial, nil); err == nil {
		t.Fatal("persist accepted an equal-sequence transition")
	}
	jump := initial
	jump.Sequence++
	jump.Stage = "task_stopped"
	jump.UpdatedAt = jump.UpdatedAt.Add(time.Second)
	if err := persist(root, initial, jump, nil); err == nil {
		t.Fatal("persist accepted an illegal stage jump")
	}
	terminal := queueTerminalOperation(initial)
	terminal.SafetyIdentity = strings.Repeat("d", 64)
	if err := persist(root, initial, terminal, nil); err == nil {
		t.Fatal("persist accepted changed immutable operation material")
	}
}

func TestQueueHistoryAcceptsOneWayLegacyMigration(t *testing.T) {
	root := t.TempDir()
	legacy := queuePathOperation("queue-migration")
	legacy.SchemaVersion = LegacyOperationSchemaVersion
	legacy.MaximumWorkers = 0
	if err := persist(root, Operation{}, legacy, nil); err != nil {
		t.Fatal(err)
	}
	selected := legacy
	selected.Sequence++
	selected.Stage = "selected"
	selected.UpdatedAt = selected.UpdatedAt.Add(time.Second)
	selected.InFlight = &Selection{TaskID: "task", TaskOperationID: "task-run", Fingerprint: strings.Repeat("e", 64), Authority: strings.Repeat("f", 64)}
	if err := persist(root, legacy, selected, nil); err != nil {
		t.Fatal(err)
	}
	migrated := selected
	migrated.SchemaVersion = OperationSchemaVersion
	migrated.MaximumWorkers = 1
	migrated.Sequence++
	migrated.UpdatedAt = migrated.UpdatedAt.Add(time.Second)
	migrated.Slots = []WorkerSlot{{Selection: Selection{Sequence: 1, Batch: 1, Slot: 1, TaskID: "task", TaskOperationID: "task-run", Fingerprint: strings.Repeat("e", 64), Authority: strings.Repeat("f", 64)}, State: SlotAdmitted}}
	migrated.InFlight = nil
	if err := persist(root, selected, migrated, nil); err != nil {
		t.Fatal(err)
	}
	op, found, err := Inspect(root, legacy.OperationID)
	if err != nil || !found || !canonicalEqual(op, migrated) {
		t.Fatalf("Inspect() = %+v, %v, %v; want migrated history authority", op, found, err)
	}
}

func writeQueueHistory(t *testing.T, root, name string, op Operation) {
	t.Helper()
	dir := filepath.Join(queueDir(root, op.OperationID), "history")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeQueueOperation(t, filepath.Join(dir, name), op)
}

func writeQueueCheckpoint(t *testing.T, root string, op Operation) {
	t.Helper()
	dir := queueDir(root, op.OperationID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeQueueOperation(t, queueCheckpointPath(root, op.OperationID), op)
}

func writeQueueOperation(t *testing.T, path string, op Operation) {
	t.Helper()
	raw, err := canonical(op)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func queueCheckpointPath(root, operationID string) string {
	return filepath.Join(queueDir(root, operationID), "operation.json")
}
