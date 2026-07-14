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

func TestTaskRunPersistenceRejectsSubstitutionAtEveryMetadataBoundary(t *testing.T) {
	tests := []struct {
		name       string
		point      FailurePoint
		occurrence int
		failBefore bool
	}{
		{name: "before history read", point: FailureBeforeOperationHistory, occurrence: 1},
		{name: "after history open", point: FailureAfterOperationHistoryOpen, occurrence: 1},
		{name: "before history publication", point: FailureBeforeOperationHistoryPublish, occurrence: 1},
		{name: "after history publication", point: FailureAfterOperationHistoryPublish, occurrence: 1},
		{name: "history directory sync", point: FailureBeforeOperationDirectorySync, occurrence: 1},
		{name: "after checkpoint open", point: FailureAfterOperationCheckpointOpen, occurrence: 1},
		{name: "before checkpoint replacement", point: FailureBeforeOperationRename, occurrence: 1},
		{name: "after checkpoint replacement", point: FailureAfterOperationRename, occurrence: 1},
		{name: "checkpoint directory sync", point: FailureBeforeOperationDirectorySync, occurrence: 2},
		{name: "before checkpoint readback", point: FailureBeforeOperationReadback, occurrence: 1},
		{name: "history cleanup", point: FailureBeforeOperationCleanup, occurrence: 1, failBefore: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), taskRunOutside(t)
			op := pathSafetyOperation("metadata-" + strings.ReplaceAll(test.name, " ", "-"))
			dir := operationDir(root, op.OperationID)
			injectedFailure := errors.New("injected pre-cleanup failure")
			counts := make(map[FailurePoint]int)
			var before string
			inject := func(point FailurePoint) error {
				if test.failBefore && point == FailureBeforeOperationFileSync {
					return injectedFailure
				}
				if point != test.point {
					return nil
				}
				counts[point]++
				if counts[point] != test.occurrence {
					return nil
				}
				var err error
				before, err = substituteTaskRunDirectory(t, dir, outside)
				return err
			}

			err := persist(root, Operation{}, op, inject)
			if before == "" {
				t.Fatal("substitution hook did not run")
			}
			assertTaskRunUnsafe(t, err, filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "task-runs", op.OperationID)), outside, before)
			if test.failBefore && !errors.Is(err, injectedFailure) {
				t.Fatalf("error = %v, want injected pre-cleanup failure", err)
			}
		})
	}
}

func TestTaskRunPersistenceChecksHeldLeaseBeforeCheckpointReplacement(t *testing.T) {
	root := t.TempDir()
	op := pathSafetyOperation("lease-substitution")
	dir := operationDir(root, op.OperationID)
	fired := false
	err := persist(root, Operation{}, op, func(point FailurePoint) error {
		if fired || point != FailureBeforeOperationRename {
			return nil
		}
		fired = true
		lockPath := filepath.Join(dir, "operation.lock")
		if err := os.Rename(lockPath, lockPath+".held"); err != nil {
			return err
		}
		return os.WriteFile(lockPath, []byte("replacement lock\n"), 0o600)
	})
	if !fired {
		t.Fatal("lease substitution hook did not run")
	}
	if !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("persist error = %v, want runtimepath.ErrUnsafe", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "operation.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkpoint was replaced after lease substitution: %v", err)
	}
	recovered, found, err := Inspect(root, op.OperationID)
	if err != nil || !found || recovered.Sequence != op.Sequence || recovered.Stage != op.Stage {
		t.Fatalf("recovered = %+v found=%t error=%v", recovered, found, err)
	}
}

func TestTaskRunInspectRejectsUnsafeEvidenceFiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir, outside string)
	}{
		{
			name: "checkpoint symlink",
			setup: func(t *testing.T, dir, outside string) {
				t.Helper()
				path := filepath.Join(dir, "operation.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "history hard link",
			setup: func(t *testing.T, dir, outside string) {
				t.Helper()
				path := filepath.Join(dir, "history", "00000000000000000000-admitted.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "history unsafe mode",
			setup: func(t *testing.T, dir, _ string) {
				t.Helper()
				path := filepath.Join(dir, "history", "00000000000000000000-admitted.json")
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), taskRunOutside(t)
			op := pathSafetyOperation("inspect-" + strings.ReplaceAll(test.name, " ", "-"))
			if err := persist(root, Operation{}, op); err != nil {
				t.Fatal(err)
			}
			dir := operationDir(root, op.OperationID)
			test.setup(t, dir, outside)
			before := taskRunOutsideSnapshot(t, outside)
			if _, _, err := Inspect(root, op.OperationID); !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("Inspect error = %v, want runtimepath.ErrUnsafe", err)
			}
			if after := taskRunOutsideSnapshot(t, outside); after != before {
				t.Fatalf("outside tree changed\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
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
			boundary, bindErr := runtimepath.Bind(root)
			if bindErr != nil {
				t.Fatal(bindErr)
			}
			lease, err := lockOperation(context.Background(), boundary, operationID)
			if lease != nil {
				_ = lease.Close()
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
			boundary, bindErr := runtimepath.Bind(root)
			if bindErr != nil {
				t.Fatal(bindErr)
			}
			lease, err := lockOperation(context.Background(), boundary, operationID)
			if lease != nil {
				_ = lease.Close()
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
	var result []string
	err := filepath.WalkDir(outside, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(outside, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		links := uint64(0)
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			links = uint64(stat.Nlink)
		}
		value := fmt.Sprintf("%s|%s|%04o|%d", filepath.ToSlash(rel), info.Mode().Type(), info.Mode().Perm(), links)
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += "|" + string(raw)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += "|" + target
		}
		result = append(result, value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(result, "\n")
}

func substituteTaskRunDirectory(t *testing.T, dir, outside string) (string, error) {
	t.Helper()
	moved := dir + ".moved"
	if err := os.Rename(dir, moved); err != nil {
		return "", err
	}
	if err := filepath.WalkDir(moved, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".history-") && !strings.HasPrefix(entry.Name(), ".operation-") {
			return nil
		}
		rel, err := filepath.Rel(moved, path)
		if err != nil {
			return err
		}
		attackerPath := filepath.Join(outside, rel)
		if err := os.MkdirAll(filepath.Dir(attackerPath), 0o700); err != nil {
			return err
		}
		return os.WriteFile(attackerPath, []byte("attacker temporary\n"), 0o600)
	}); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(outside, "operation.lock"), []byte("attacker lock\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Symlink(outside, dir); err != nil {
		return "", err
	}
	return taskRunOutsideSnapshot(t, outside), nil
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
