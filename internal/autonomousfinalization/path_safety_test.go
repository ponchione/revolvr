package autonomousfinalization

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

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

func TestFinalizationStorageRejectsUnsafeFinalComponents(t *testing.T) {
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
			root, outside := t.TempDir(), finalizationOutside(t)
			raw := []byte("trusted completion evidence\n")
			if err := os.WriteFile(filepath.Join(outside, "sentinel"), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			rel := ".revolvr/autonomous/tasks/final-task/completion/final.md"
			path := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := test.setup(path, outside, raw); err != nil {
				t.Fatal(err)
			}
			before := finalizationTreeSnapshot(t, outside)
			storage, err := bindFinalizationStorage(root, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := storage.verifyArtifact(artifact(rel, raw), raw); !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("verifyArtifact error = %v, want runtimepath.ErrUnsafe", err)
			}
			assertFinalizationOutsideUnchanged(t, outside, before)
		})
	}
}

func TestFinalizationStorageRejectsAncestorSubstitutionAtEveryBoundary(t *testing.T) {
	tests := []struct {
		name       string
		point      FailurePoint
		failBefore bool
		published  bool
	}{
		{name: "before directory open", point: FailureBeforeArtifactDirectoryOpen},
		{name: "after directory open", point: FailureAfterArtifactDirectoryOpen},
		{name: "after temporary open", point: FailureAfterArtifactTemporaryOpen},
		{name: "before file sync", point: FailureBeforeArtifactFileSync},
		{name: "before publication", point: FailureBeforeArtifactPublish},
		{name: "after publication", point: FailureAfterArtifactPublish, published: true},
		{name: "before directory sync", point: FailureBeforeArtifactDirectorySync, published: true},
		{name: "before readback", point: FailureBeforeArtifactReadback, published: true},
		{name: "after readback open", point: FailureAfterArtifactReadOpen, published: true},
		{name: "cleanup", point: FailureBeforeArtifactCleanup, failBefore: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), finalizationOutside(t)
			rel := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", "final-task", "completion", strings.ReplaceAll(test.name, " ", "-")+".json"))
			path := filepath.Join(root, filepath.FromSlash(rel))
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			raw := []byte("completion artifact\n")
			injectedFailure := errors.New("injected pre-cleanup failure")
			fired := false
			var outsideBefore []string
			storage, err := bindFinalizationStorage(root, func(point FailurePoint) error {
				if test.failBefore && point == FailureBeforeArtifactFileSync {
					return injectedFailure
				}
				if fired || point != test.point {
					return nil
				}
				fired = true
				var err error
				outsideBefore, err = substituteFinalizationDirectory(t, dir, outside, filepath.Base(path))
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			operationErr := storage.writeImmutable(artifact(rel, raw), raw)
			if !fired || outsideBefore == nil {
				t.Fatal("substitution hook did not run")
			}
			if !errors.Is(operationErr, runtimepath.ErrUnsafe) {
				t.Fatalf("writeImmutable error = %v, want runtimepath.ErrUnsafe", operationErr)
			}
			if test.failBefore && !errors.Is(operationErr, injectedFailure) {
				t.Fatalf("writeImmutable error = %v, want injected cleanup precursor", operationErr)
			}
			movedArtifact := filepath.Join(dir+".moved", filepath.Base(path))
			_, statErr := os.Lstat(movedArtifact)
			if test.published && statErr != nil {
				t.Fatalf("published artifact missing from retained directory: %v", statErr)
			}
			if !test.published && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("artifact published before authority was lost: %v", statErr)
			}
			assertFinalizationOutsideUnchanged(t, outside, outsideBefore)
		})
	}
}

func TestFinalizationStorageRejectsUnsafeDestinationBeforePublication(t *testing.T) {
	tests := []struct {
		name  string
		setup func(string, string) error
	}{
		{
			name: "symlink",
			setup: func(path, outside string) error {
				return os.Symlink(filepath.Join(outside, "authority-sentinel"), path)
			},
		},
		{
			name: "hard link",
			setup: func(path, outside string) error {
				return os.Link(filepath.Join(outside, "authority-sentinel"), path)
			},
		},
		{
			name: "unsafe mode",
			setup: func(path, _ string) error {
				if err := os.WriteFile(path, []byte("attacker final\n"), 0o644); err != nil {
					return err
				}
				return os.Chmod(path, 0o666)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), finalizationOutside(t)
			rel := ".revolvr/autonomous/tasks/final-task/completion/artifact.json"
			path := filepath.Join(root, filepath.FromSlash(rel))
			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			fired := false
			var outsideBefore []string
			storage, err := bindFinalizationStorage(root, func(point FailurePoint) error {
				if fired || point != FailureBeforeArtifactPublish {
					return nil
				}
				fired = true
				if err := test.setup(path, outside); err != nil {
					return err
				}
				outsideBefore = finalizationTreeSnapshot(t, outside)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			raw := []byte("trusted completion artifact\n")
			if err := storage.writeImmutable(artifact(rel, raw), raw); !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("writeImmutable error = %v, want runtimepath.ErrUnsafe", err)
			}
			if !fired || outsideBefore == nil {
				t.Fatal("destination substitution hook did not run")
			}
			assertFinalizationOutsideUnchanged(t, outside, outsideBefore)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".completion.tmp-") {
					t.Fatalf("temporary artifact survived stable-parent cleanup: %s", entry.Name())
				}
			}
		})
	}
}

func TestFinalizeRejectsCompletionNamespaceSubstitutionEndToEnd(t *testing.T) {
	root := t.TempDir()
	e := fixtureEvidence(t, root, func(raw []byte) { writeFixture(t, root, raw) })
	stateStore, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(root, ".revolvr", "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	completionDir := filepath.Join(root, ".revolvr", "autonomous", "tasks", e.Task.TaskID, "completion")
	outside := finalizationOutside(t)
	fired := false
	var outsideBefore []string
	cfg := Config{RepositoryRoot: root, Evidence: e, StateStore: stateStore, Ledger: ledgerStore, RevalidateEvidence: func(context.Context, FrozenEvidence) error { return nil }, FailureInjector: func(point FailurePoint) error {
		if fired || point != FailureBeforeArtifactPublish {
			return nil
		}
		fired = true
		var err error
		outsideBefore, err = substituteFinalizationDirectory(t, completionDir, outside, "completion-evidence.json")
		return err
	}}
	if _, err := Finalize(context.Background(), cfg); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("Finalize error = %v, want runtimepath.ErrUnsafe", err)
	}
	if !fired || outsideBefore == nil {
		t.Fatal("finalization substitution hook did not run")
	}
	assertFinalizationOutsideUnchanged(t, outside, outsideBefore)
	snapshot, found, err := stateStore.Load(context.Background(), e.Task.TaskID)
	if err != nil || !found {
		t.Fatalf("state found=%t error=%v", found, err)
	}
	if snapshot.State.Lifecycle != autonomous.LifecycleStateReady || snapshot.State.Finalization != nil {
		t.Fatalf("finalization state advanced after rejected publication: %+v", snapshot.State.Finalization)
	}
	task, found, err := taskfile.FindByID(root, e.Task.TaskID)
	if err != nil || !found || task.Status != taskfile.StatusPending {
		t.Fatalf("task found=%t status=%q error=%v", found, task.Status, err)
	}
	if _, found, err := ledgerStore.GetRunWithEvents(context.Background(), e.FinalizationRunID); err != nil || found {
		t.Fatalf("ledger run found=%t error=%v", found, err)
	}
}

func finalizationOutside(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "authority-sentinel"), []byte("outside authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return outside
}

func substituteFinalizationDirectory(t *testing.T, dir, outside, finalName string) ([]string, error) {
	t.Helper()
	moved := dir + ".moved"
	if err := os.Rename(dir, moved); err != nil {
		return nil, err
	}
	if err := filepath.WalkDir(moved, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".completion.tmp-") {
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
	return finalizationTreeSnapshot(t, outside), nil
}

func finalizationTreeSnapshot(t *testing.T, root string) []string {
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

func assertFinalizationOutsideUnchanged(t *testing.T, outside string, before []string) {
	t.Helper()
	if after := finalizationTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
		t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
	}
}
