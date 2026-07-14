package autonomoustaskrun

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

func TestTaskRunPersistenceRejectsEverySymlinkedAncestor(t *testing.T) {
	const operationID = "path-state"
	components := []string{
		".revolvr",
		".revolvr/autonomous",
		".revolvr/autonomous/task-runs",
		".revolvr/autonomous/task-runs/" + operationID,
		".revolvr/autonomous/task-runs/" + operationID + "/history",
	}
	for _, component := range components {
		t.Run(component, func(t *testing.T) {
			root, outside := t.TempDir(), taskRunOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			before := taskRunOutsideSnapshot(t, outside)
			err := persist(root, Operation{}, pathSafetyOperation(operationID))
			assertTaskRunUnsafe(t, err, component, outside, before)
		})
	}
}

func TestTaskRunPersistenceRejectsUnsafeFinalFiles(t *testing.T) {
	tests := []struct {
		name  string
		final string
		setup func(*testing.T, string, string)
	}{
		{
			name: "history symlink", final: "history/00000000000000000000-admitted.json",
			setup: func(t *testing.T, path, outside string) {
				if err := os.Symlink(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checkpoint symlink", final: "operation.json",
			setup: func(t *testing.T, path, outside string) {
				if err := os.Symlink(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checkpoint directory", final: "operation.json",
			setup: func(t *testing.T, path, _ string) {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checkpoint fifo", final: "operation.json",
			setup: func(t *testing.T, path, _ string) {
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checkpoint hard link", final: "operation.json",
			setup: func(t *testing.T, path, outside string) {
				if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checkpoint unsafe mode", final: "operation.json",
			setup: func(t *testing.T, path, _ string) {
				if err := os.WriteFile(path, []byte("unsafe"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), taskRunOutside(t)
			dir := operationDir(root, "unsafe-final")
			if err := os.MkdirAll(filepath.Join(dir, "history"), 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, filepath.FromSlash(test.final))
			test.setup(t, path, outside)
			before := taskRunOutsideSnapshot(t, outside)
			err := persist(root, Operation{}, pathSafetyOperation("unsafe-final"))
			component := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "task-runs", "unsafe-final", filepath.FromSlash(test.final)))
			assertTaskRunUnsafe(t, err, component, outside, before)
		})
	}
}

func TestTaskRunPersistenceRevalidatesCheckpointBeforeRename(t *testing.T) {
	root, outside := t.TempDir(), taskRunOutside(t)
	op := pathSafetyOperation("rename-substitution")
	if err := persist(root, Operation{}, op); err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(operationDir(root, op.OperationID), "operation.json")
	before := taskRunOutsideSnapshot(t, outside)
	fired := false
	err := persist(root, op, op, func(point FailurePoint) error {
		if point != FailureBeforeOperationRename || fired {
			return nil
		}
		fired = true
		if err := os.Remove(checkpoint); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(outside, "sentinel"), checkpoint)
	})
	if !fired {
		t.Fatal("rename substitution did not run")
	}
	assertTaskRunUnsafe(t, err, ".revolvr/autonomous/task-runs/rename-substitution/operation.json", outside, before)
	entries, readErr := os.ReadDir(operationDir(root, op.OperationID))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".operation-") {
			t.Fatalf("temporary checkpoint survived rejected substitution: %s", entry.Name())
		}
	}
}

func TestTaskRunPersistenceDoesNotFollowSubstitutedParentDuringTempCleanup(t *testing.T) {
	root, outside := t.TempDir(), taskRunOutside(t)
	op := pathSafetyOperation("parent-substitution")
	if err := persist(root, Operation{}, op); err != nil {
		t.Fatal(err)
	}
	dir := operationDir(root, op.OperationID)
	moved := dir + ".moved"
	var before string
	fired := false
	err := persist(root, op, op, func(point FailurePoint) error {
		if point != FailureBeforeOperationRename || fired {
			return nil
		}
		fired = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		var tempName string
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".operation-") {
				tempName = entry.Name()
				break
			}
		}
		if tempName == "" {
			return errors.New("temporary checkpoint not found")
		}
		if err := os.WriteFile(filepath.Join(outside, tempName), []byte("outside-bait\n"), 0o600); err != nil {
			return err
		}
		if err := os.Rename(dir, moved); err != nil {
			return err
		}
		if err := os.Symlink(outside, dir); err != nil {
			return err
		}
		before = taskRunOutsideSnapshot(t, outside)
		return nil
	})
	if !fired || before == "" {
		t.Fatal("parent substitution did not run")
	}
	assertTaskRunUnsafe(t, err, ".revolvr/autonomous/task-runs/parent-substitution", outside, before)
}

func TestTaskRunLockRejectsEverySymlinkedAncestorAndUnsafeFinal(t *testing.T) {
	const operationID = "path-lock"
	components := []string{
		".revolvr",
		".revolvr/autonomous",
		".revolvr/autonomous/task-runs",
		".revolvr/autonomous/task-runs/" + operationID,
	}
	for _, component := range components {
		t.Run("ancestor "+component, func(t *testing.T) {
			root, outside := t.TempDir(), taskRunOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			before := taskRunOutsideSnapshot(t, outside)
			release, err := lockOperation(context.Background(), root, operationID)
			if release != nil {
				release()
			}
			assertTaskRunUnsafe(t, err, component, outside, before)
		})
	}

	for _, test := range []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, path, outside string) {
				if err := os.Symlink(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, path, _ string) {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hard link",
			setup: func(t *testing.T, path, outside string) {
				if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unsafe mode",
			setup: func(t *testing.T, path, _ string) {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run("final "+test.name, func(t *testing.T) {
			root, outside := t.TempDir(), taskRunOutside(t)
			dir := operationDir(root, operationID)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "operation.lock")
			test.setup(t, path, outside)
			before := taskRunOutsideSnapshot(t, outside)
			release, err := lockOperation(context.Background(), root, operationID)
			if release != nil {
				release()
			}
			assertTaskRunUnsafe(t, err, ".revolvr/autonomous/task-runs/path-lock/operation.lock", outside, before)
		})
	}
}

func pathSafetyOperation(id string) Operation {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	return Operation{
		SchemaVersion: OperationSchemaVersion,
		OperationID:   id,
		TaskID:        "task-one",
		Task:          Identity{Path: ".agent/tasks/task-one.md", SHA256: strings.Repeat("a", 64), ByteSize: 1},
		State:         Identity{Path: ".revolvr/autonomous/tasks/task-one/state.json", SHA256: strings.Repeat("b", 64), ByteSize: 1},
		ConfigSHA256:  strings.Repeat("c", 64),
		MaxCycles:     Limited(1),
		StartedAt:     now,
		UpdatedAt:     now,
		Stage:         "admitted",
	}
}

func taskRunOutside(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return outside
}

func taskRunOutsideSnapshot(t *testing.T, outside string) string {
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

func assertTaskRunUnsafe(t *testing.T, err error, component, outside, before string) {
	t.Helper()
	if !errors.Is(err, runtimepath.ErrUnsafe) || !strings.Contains(err.Error(), component) {
		t.Fatalf("error = %v, want unsafe component %q", err, component)
	}
	if after := taskRunOutsideSnapshot(t, outside); after != before {
		t.Fatalf("outside sentinel changed\nbefore: %s\nafter:  %s", before, after)
	}
}
