package autonomousarchive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

func TestArchiveStorageRejectsUnsafeFinalComponents(t *testing.T) {
	tests := []struct {
		name  string
		setup func(string, string, []byte) error
	}{
		{
			name: "symlink",
			setup: func(path, outside string, _ []byte) error {
				return os.Symlink(filepath.Join(outside, "sentinel"), path)
			},
		},
		{
			name: "hard link",
			setup: func(path, outside string, _ []byte) error {
				return os.Link(filepath.Join(outside, "sentinel"), path)
			},
		},
		{
			name: "unsafe mode",
			setup: func(path, _ string, raw []byte) error {
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					return err
				}
				return os.Chmod(path, 0o666)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), archiveOutside(t)
			raw := []byte("trusted archive evidence\n")
			if err := os.WriteFile(filepath.Join(outside, "sentinel"), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			rel := ".agent/archive/2026/07/task/final.md"
			path := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := test.setup(path, outside, raw); err != nil {
				t.Fatal(err)
			}
			before := archiveTreeSnapshot(t, outside)
			storage, err := bindArchiveStorage(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := storage.readArtifact(artifact(rel, raw)); !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("readArtifact error = %v, want runtimepath.ErrUnsafe", err)
			}
			assertArchiveOutsideUnchanged(t, outside, before)
		})
	}
}

func TestArchiveStorageRejectsSubstitutionAtEveryMutationBoundary(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		point      FailurePoint
		failBefore bool
	}{
		{name: "immutable directory open", operation: "immutable", point: FailureAfterStorageDirectoryOpen},
		{name: "immutable temporary open", operation: "immutable", point: FailureAfterStorageOpen},
		{name: "immutable file sync", operation: "immutable", point: FailureBeforeStorageFileSync},
		{name: "immutable publication", operation: "immutable", point: FailureBeforeStoragePublish},
		{name: "immutable after publication", operation: "immutable", point: FailureAfterStoragePublish},
		{name: "immutable directory sync", operation: "immutable", point: FailureBeforeStorageDirectorySync},
		{name: "immutable readback", operation: "immutable", point: FailureBeforeStorageReadback},
		{name: "immutable opened readback", operation: "immutable", point: FailureAfterStorageReadOpen},
		{name: "immutable cleanup", operation: "immutable", point: FailureBeforeStorageCleanup, failBefore: true},
		{name: "mutable directory open", operation: "mutable", point: FailureAfterStorageDirectoryOpen},
		{name: "mutable temporary open", operation: "mutable", point: FailureAfterStorageOpen},
		{name: "mutable file sync", operation: "mutable", point: FailureBeforeStorageFileSync},
		{name: "mutable publication", operation: "mutable", point: FailureBeforeStoragePublish},
		{name: "mutable after publication", operation: "mutable", point: FailureAfterStoragePublish},
		{name: "mutable directory sync", operation: "mutable", point: FailureBeforeStorageDirectorySync},
		{name: "mutable readback", operation: "mutable", point: FailureBeforeStorageReadback},
		{name: "mutable opened readback", operation: "mutable", point: FailureAfterStorageReadOpen},
		{name: "mutable cleanup", operation: "mutable", point: FailureBeforeStorageCleanup, failBefore: true},
		{name: "remove directory open", operation: "remove", point: FailureAfterStorageDirectoryOpen},
		{name: "remove file open", operation: "remove", point: FailureAfterStorageReadOpen},
		{name: "before remove", operation: "remove", point: FailureBeforeStorageRemove},
		{name: "after remove", operation: "remove", point: FailureAfterStorageRemove},
		{name: "remove directory sync", operation: "remove", point: FailureBeforeStorageDirectorySync},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), archiveOutside(t)
			rel := filepath.ToSlash(filepath.Join(".agent", "archive-path-test", strings.ReplaceAll(test.name, " ", "-"), "artifact.json"))
			path := filepath.Join(root, filepath.FromSlash(rel))
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			storage, err := bindArchiveStorage(root)
			if err != nil {
				t.Fatal(err)
			}
			oldRaw := []byte("old journal\n")
			newRaw := []byte("new archive bytes\n")
			if test.operation == "mutable" {
				if err := storage.writeMutable(rel, oldRaw); err != nil {
					t.Fatal(err)
				}
			} else if test.operation == "remove" {
				if err := os.WriteFile(path, oldRaw, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			injectedFailure := errors.New("injected pre-cleanup failure")
			fired := false
			var outsideBefore []string
			storage.inject = func(point FailurePoint) error {
				if test.failBefore && point == FailureBeforeStorageFileSync {
					return injectedFailure
				}
				if fired || point != test.point {
					return nil
				}
				fired = true
				var err error
				outsideBefore, err = substituteArchiveDirectory(t, dir, outside, filepath.Base(path))
				return err
			}

			var operationErr error
			switch test.operation {
			case "immutable":
				operationErr = storage.writeImmutable(artifact(rel, newRaw), newRaw)
			case "mutable":
				operationErr = storage.writeMutable(rel, newRaw)
			case "remove":
				operationErr = storage.removeExact(artifact(rel, oldRaw))
			default:
				t.Fatalf("unknown operation %q", test.operation)
			}
			if !fired || outsideBefore == nil {
				t.Fatal("substitution hook did not run")
			}
			if !errors.Is(operationErr, runtimepath.ErrUnsafe) {
				t.Fatalf("operation error = %v, want runtimepath.ErrUnsafe", operationErr)
			}
			if test.failBefore && !errors.Is(operationErr, injectedFailure) {
				t.Fatalf("operation error = %v, want injected cleanup precursor", operationErr)
			}
			assertArchiveOutsideUnchanged(t, outside, outsideBefore)
		})
	}
}

func TestArchiveStorageChecksHeldLeaseBeforePublication(t *testing.T) {
	root := t.TempDir()
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := acquireFileLock(context.Background(), boundary, ".revolvr/locks/archive-path-test.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	rel := ".agent/archive/2026/07/lease-task/task.md"
	raw := []byte("leased archive bytes\n")
	fired := false
	storage := newArchiveStorage(boundary, func(point FailurePoint) error {
		if fired || point != FailureBeforeStoragePublish {
			return nil
		}
		fired = true
		if err := os.Rename(lease.Path(), lease.Path()+".held"); err != nil {
			return err
		}
		return os.WriteFile(lease.Path(), []byte("replacement lease\n"), 0o600)
	}, lease)
	if err := storage.writeImmutable(artifact(rel, raw), raw); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("writeImmutable error = %v, want runtimepath.ErrUnsafe", err)
	}
	if !fired {
		t.Fatal("lease substitution hook did not run")
	}
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("artifact published after lease substitution: %v", err)
	}
}

func TestArchiveStorageEnumerationRejectsNamespaceSubstitution(t *testing.T) {
	root, outside := t.TempDir(), archiveOutside(t)
	archiveDir := filepath.Join(root, filepath.FromSlash(ArchiveRoot))
	if err := os.MkdirAll(filepath.Join(archiveDir, "2026", "07", "task"), 0o755); err != nil {
		t.Fatal(err)
	}
	storage, err := bindArchiveStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	fired := false
	var before []string
	storage.inject = func(point FailurePoint) error {
		if fired || point != FailureBeforeStorageEnumeration {
			return nil
		}
		fired = true
		var err error
		before, err = substituteArchiveDirectory(t, archiveDir, outside, "archive.json")
		return err
	}
	if _, err := storage.list(); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("list error = %v, want runtimepath.ErrUnsafe", err)
	}
	if !fired || before == nil {
		t.Fatal("enumeration substitution hook did not run")
	}
	assertArchiveOutsideUnchanged(t, outside, before)
}

func TestArchiveRejectsPersistenceSubstitutionEndToEnd(t *testing.T) {
	root, ledgerStore := terminalRepo(t, DispositionCancelled)
	defer ledgerStore.Close()
	headBefore := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD"))
	dir := filepath.Join(root, ".revolvr", "autonomous", "tasks", "terminal-task", "archive", "history")
	outside := archiveOutside(t)
	fired := false
	var before []string
	cfg := Config{RepositoryRoot: root, Ledger: ledgerStore, FailureInjector: func(point FailurePoint) error {
		if fired || point != FailureBeforeStoragePublish {
			return nil
		}
		fired = true
		var err error
		before, err = substituteArchiveDirectory(t, dir, outside, "attacker-history.json")
		return err
	}}
	request := ArchiveRequest{TaskID: "terminal-task", OperationID: "archive-path-substitution", ArchiveRunID: "archive-run-path-substitution", Authority: authority(DispositionCancelled), ArchivedAt: archiveTestTime}
	if _, err := Archive(context.Background(), cfg, request); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("Archive error = %v, want runtimepath.ErrUnsafe", err)
	}
	if !fired || before == nil {
		t.Fatal("archive persistence substitution hook did not run")
	}
	assertArchiveOutsideUnchanged(t, outside, before)
	if got := strings.TrimSpace(runGitTest(t, root, "rev-parse", "HEAD")); got != headBefore {
		t.Fatalf("archive advanced HEAD from %s to %s", headBefore, got)
	}
	if _, found, err := taskfile.FindByID(root, "terminal-task"); err != nil || !found {
		t.Fatalf("archive changed active task: found=%t error=%v", found, err)
	}
}

func archiveOutside(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "authority-sentinel"), []byte("outside authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return outside
}

func substituteArchiveDirectory(t *testing.T, dir, outside, finalName string) ([]string, error) {
	t.Helper()
	moved := dir + ".moved"
	if err := os.Rename(dir, moved); err != nil {
		return nil, err
	}
	if err := filepath.WalkDir(moved, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".archive.tmp-") && !strings.HasPrefix(entry.Name(), ".journal.tmp-") {
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
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outside, finalName), []byte("attacker final\n"), 0o600); err != nil {
		return nil, err
	}
	if err := os.Symlink(outside, dir); err != nil {
		return nil, err
	}
	return archiveTreeSnapshot(t, outside), nil
}

func archiveTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		nlink := uint64(0)
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			nlink = uint64(stat.Nlink)
		}
		value := fmt.Sprintf("%s|%s|%04o|%d", filepath.ToSlash(rel), info.Mode().Type(), info.Mode().Perm(), nlink)
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += "|->" + target
		} else if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += "|" + string(raw)
		}
		result = append(result, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertArchiveOutsideUnchanged(t *testing.T, outside string, before []string) {
	t.Helper()
	if after := archiveTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
		t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
	}
}
