package autonomousqueue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"revolvr/internal/runtimepath"
)

func TestQueuePersistenceRejectsEveryUnsafeAncestor(t *testing.T) {
	const operationID = "queue-path"
	components := []string{
		".revolvr",
		".revolvr/autonomous",
		".revolvr/autonomous/queues",
		".revolvr/autonomous/queues/" + operationID,
		".revolvr/autonomous/queues/" + operationID + "/history",
	}
	for _, component := range components {
		t.Run("symlink "+component, func(t *testing.T) {
			root, outside := t.TempDir(), queueOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			before := queueOutsideSnapshot(t, outside)
			err := persist(root, Operation{}, queuePathOperation(operationID), nil)
			assertQueueUnsafe(t, err, component, outside, before)
		})
	}

	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{"wrong type", func(t *testing.T, path string) { queueWrite(t, path, []byte("file"), 0o600) }},
		{"unsafe mode", func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o777); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), queueOutside(t)
			path := filepath.Join(root, ".revolvr", "autonomous")
			test.setup(t, path)
			before := queueOutsideSnapshot(t, outside)
			err := persist(root, Operation{}, queuePathOperation(operationID), nil)
			assertQueueUnsafe(t, err, ".revolvr/autonomous", outside, before)
		})
	}
}

func TestQueuePersistenceRejectsUnsafeFinalFiles(t *testing.T) {
	tests := []struct {
		name  string
		final string
		setup func(*testing.T, string, string)
	}{
		{"history symlink", "history/00000000000000000000-admitted.json", func(t *testing.T, path, outside string) { queueSymlink(t, filepath.Join(outside, "sentinel"), path) }},
		{"history hard link", "history/00000000000000000000-admitted.json", func(t *testing.T, path, outside string) {
			if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint symlink", "operation.json", func(t *testing.T, path, outside string) { queueSymlink(t, filepath.Join(outside, "sentinel"), path) }},
		{"checkpoint directory", "operation.json", func(t *testing.T, path, _ string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint fifo", "operation.json", func(t *testing.T, path, _ string) {
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint hard link", "operation.json", func(t *testing.T, path, outside string) {
			if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint unsafe mode", "operation.json", func(t *testing.T, path, _ string) {
			queueWrite(t, path, []byte("unsafe"), 0o600)
			if err := os.Chmod(path, 0o666); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), queueOutside(t)
			dir := queueDir(root, "queue-final")
			if err := os.MkdirAll(filepath.Join(dir, "history"), 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, filepath.FromSlash(test.final))
			test.setup(t, path, outside)
			before := queueOutsideSnapshot(t, outside)
			err := persist(root, Operation{}, queuePathOperation("queue-final"), nil)
			component := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "queues", "queue-final", filepath.FromSlash(test.final)))
			assertQueueUnsafe(t, err, component, outside, before)
		})
	}
}

func TestQueuePersistenceRevalidatesCheckpointBeforeRename(t *testing.T) {
	root, outside := t.TempDir(), queueOutside(t)
	op := queuePathOperation("queue-rename")
	if err := persist(root, Operation{}, op, nil); err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(queueDir(root, op.OperationID), "operation.json")
	before := queueOutsideSnapshot(t, outside)
	fired := false
	err := persist(root, op, queueTerminalOperation(op), func(point FailurePoint) error {
		if point != FailureBeforeRename || fired {
			return nil
		}
		fired = true
		if err := os.Remove(checkpoint); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(outside, "sentinel"), checkpoint)
	})
	if !fired {
		t.Fatal("checkpoint substitution did not run")
	}
	assertQueueUnsafe(t, err, ".revolvr/autonomous/queues/queue-rename/operation.json", outside, before)
	assertNoQueueTemp(t, queueDir(root, op.OperationID))
}

func TestQueueTempCleanupDoesNotFollowSubstitutedParent(t *testing.T) {
	root, outside := t.TempDir(), queueOutside(t)
	op := queuePathOperation("queue-parent")
	if err := persist(root, Operation{}, op, nil); err != nil {
		t.Fatal(err)
	}
	dir, moved := queueDir(root, op.OperationID), queueDir(root, op.OperationID)+".moved"
	var before string
	fired := false
	err := persist(root, op, queueTerminalOperation(op), func(point FailurePoint) error {
		if point != FailureBeforeRename || fired {
			return nil
		}
		fired = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		var temp string
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".operation-") {
				temp = entry.Name()
				break
			}
		}
		if temp == "" {
			return errors.New("queue temporary checkpoint not found")
		}
		if err := os.WriteFile(filepath.Join(outside, temp), []byte("outside-bait\n"), 0o600); err != nil {
			return err
		}
		if err := os.Rename(dir, moved); err != nil {
			return err
		}
		if err := os.Symlink(outside, dir); err != nil {
			return err
		}
		before = queueOutsideSnapshot(t, outside)
		return nil
	})
	if !fired || before == "" {
		t.Fatal("parent substitution did not run")
	}
	assertQueueUnsafe(t, err, ".revolvr/autonomous/queues/queue-parent", outside, before)
}

func TestQueueLockRejectsUnsafeAncestorsAndFinalFiles(t *testing.T) {
	const operationID = "queue-lock"
	for _, component := range []string{".revolvr", ".revolvr/autonomous", ".revolvr/autonomous/queues", ".revolvr/autonomous/queues/" + operationID} {
		t.Run("ancestor "+component, func(t *testing.T) {
			root, outside := t.TempDir(), queueOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			queueSymlink(t, outside, link)
			before := queueOutsideSnapshot(t, outside)
			release, err := lockOperation(context.Background(), root, operationID)
			if release != nil {
				release()
			}
			assertQueueUnsafe(t, err, component, outside, before)
		})
	}
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{"symlink", func(t *testing.T, path, outside string) { queueSymlink(t, filepath.Join(outside, "sentinel"), path) }},
		{"directory", func(t *testing.T, path, _ string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"hard link", func(t *testing.T, path, outside string) {
			if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"unsafe mode", func(t *testing.T, path, _ string) {
			queueWrite(t, path, nil, 0o600)
			if err := os.Chmod(path, 0o666); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run("final "+test.name, func(t *testing.T) {
			root, outside := t.TempDir(), queueOutside(t)
			dir := queueDir(root, operationID)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "operation.lock")
			test.setup(t, path, outside)
			before := queueOutsideSnapshot(t, outside)
			release, err := lockOperation(context.Background(), root, operationID)
			if release != nil {
				release()
			}
			assertQueueUnsafe(t, err, ".revolvr/autonomous/queues/queue-lock/operation.lock", outside, before)
		})
	}

	t.Run("opened file substitution", func(t *testing.T) {
		root, outside := t.TempDir(), queueOutside(t)
		path := filepath.Join(queueDir(root, operationID), "operation.lock")
		before := queueOutsideSnapshot(t, outside)
		fired := false
		release, err := lockOperation(context.Background(), root, operationID, func(point FailurePoint) error {
			if point != FailureAfterLockOpen || fired {
				return nil
			}
			fired = true
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(outside, "sentinel"), path)
		})
		if release != nil {
			release()
		}
		if !fired {
			t.Fatal("opened queue lock substitution did not run")
		}
		assertQueueUnsafe(t, err, ".revolvr/autonomous/queues/queue-lock/operation.lock", outside, before)
	})
}

func TestQueueInspectRejectsUnsafeCheckpointAndHistoryReads(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
	}{
		{"checkpoint symlink", "checkpoint"},
		{"history hard link", "history"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), queueOutside(t)
			op := queuePathOperation("queue-read")
			if err := persist(root, Operation{}, op, nil); err != nil {
				t.Fatal(err)
			}
			var path string
			if test.target == "checkpoint" {
				path = filepath.Join(queueDir(root, op.OperationID), "operation.json")
			} else {
				path = filepath.Join(queueDir(root, op.OperationID), "history", "00000000000000000000-admitted.json")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if test.target == "checkpoint" {
				queueSymlink(t, filepath.Join(outside, "sentinel"), path)
			} else if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
			before := queueOutsideSnapshot(t, outside)
			_, _, err := Inspect(root, op.OperationID)
			component := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			assertQueueUnsafe(t, err, component, outside, before)
		})
	}
}

func queuePathOperation(id string) Operation {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	return Operation{SchemaVersion: OperationSchemaVersion, OperationID: id, Mode: ModeUntilExhausted, ConfigSchema: "config-v1", ConfigSHA256: strings.Repeat("a", 64), SafetyIdentity: strings.Repeat("b", 64), MaxTasks: 1, MaximumWorkers: 1, StartedAt: now, UpdatedAt: now, Sequence: 0, Sweep: 1, Stage: "admitted"}
}

func queueTerminalOperation(op Operation) Operation {
	op.Sequence++
	op.Stage = "terminal"
	op.UpdatedAt = op.UpdatedAt.Add(time.Second)
	op.CompletedAt = &op.UpdatedAt
	op.StopReason = StopDrained
	return op
}

func queueOutside(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	queueWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
	return outside
}

func queueOutsideSnapshot(t *testing.T, outside string) string {
	t.Helper()
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	path := filepath.Join(outside, "sentinel")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	links := uint64(0)
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		links = uint64(stat.Nlink)
	}
	return fmt.Sprintf("%v|%04o|%d|%x", names, info.Mode().Perm(), links, raw)
}

func assertQueueUnsafe(t *testing.T, err error, component, outside, before string) {
	t.Helper()
	if !errors.Is(err, runtimepath.ErrUnsafe) || !strings.Contains(err.Error(), component) {
		t.Fatalf("error = %v, want unsafe component %q", err, component)
	}
	if after := queueOutsideSnapshot(t, outside); after != before {
		t.Fatalf("outside sentinel changed\nbefore: %s\nafter:  %s", before, after)
	}
}

func queueWrite(t *testing.T, path string, raw []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatal(err)
	}
}

func queueSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func assertNoQueueTemp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".operation-") {
			t.Fatalf("queue temporary survived rejected substitution: %s", entry.Name())
		}
	}
}
