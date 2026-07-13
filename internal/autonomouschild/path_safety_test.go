package autonomouschild

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

	"revolvr/internal/autonomouschildpublication"
	"revolvr/internal/runtimepath"
)

func TestChildPersistenceRejectsEveryUnsafeAncestor(t *testing.T) {
	const operationID = "child-path"
	for _, component := range []string{
		".revolvr",
		".revolvr/autonomous",
		".revolvr/autonomous/child-publications",
		".revolvr/autonomous/child-publications/history",
	} {
		t.Run("symlink "+component, func(t *testing.T) {
			root, outside := t.TempDir(), childOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			childSymlink(t, outside, link)
			before := childOutsideSnapshot(t, outside)
			err := persist(root, Journal{}, childPathJournal(operationID), nil)
			assertChildUnsafe(t, err, component, outside, before)
		})
	}

	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{"wrong type", func(t *testing.T, path string) { childWrite(t, path, []byte("file"), 0o600) }},
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
			root, outside := t.TempDir(), childOutside(t)
			path := filepath.Join(root, ".revolvr", "autonomous")
			test.setup(t, path)
			before := childOutsideSnapshot(t, outside)
			err := persist(root, Journal{}, childPathJournal(operationID), nil)
			assertChildUnsafe(t, err, ".revolvr/autonomous", outside, before)
		})
	}
}

func TestChildPersistenceRejectsUnsafeFinalFiles(t *testing.T) {
	journal := childPathJournal("child-final")
	historyName := autonomouschildpublication.HistoryFilename(journal.OperationID, journal.Sequence)
	tests := []struct {
		name  string
		final string
		setup func(*testing.T, string, string)
	}{
		{"history symlink", "history/" + historyName, func(t *testing.T, path, outside string) { childSymlink(t, filepath.Join(outside, "sentinel"), path) }},
		{"history hard link", "history/" + historyName, func(t *testing.T, path, outside string) {
			if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint symlink", journal.OperationID + ".json", func(t *testing.T, path, outside string) { childSymlink(t, filepath.Join(outside, "sentinel"), path) }},
		{"checkpoint directory", journal.OperationID + ".json", func(t *testing.T, path, _ string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint fifo", journal.OperationID + ".json", func(t *testing.T, path, _ string) {
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint hard link", journal.OperationID + ".json", func(t *testing.T, path, outside string) {
			if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkpoint unsafe mode", journal.OperationID + ".json", func(t *testing.T, path, _ string) {
			childWrite(t, path, []byte("unsafe"), 0o600)
			if err := os.Chmod(path, 0o666); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), childOutside(t)
			base := filepath.Join(root, ".revolvr", "autonomous", "child-publications")
			if err := os.MkdirAll(filepath.Join(base, "history"), 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(base, filepath.FromSlash(test.final))
			test.setup(t, path, outside)
			before := childOutsideSnapshot(t, outside)
			err := persist(root, Journal{}, journal, nil)
			component := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "child-publications", filepath.FromSlash(test.final)))
			assertChildUnsafe(t, err, component, outside, before)
		})
	}
}

func TestChildMutableCheckpointRevalidatesBeforeRename(t *testing.T) {
	root, outside := t.TempDir(), childOutside(t)
	path := filepath.Join(root, ".revolvr", "autonomous", "child-publications", "rename.json")
	if err := writeMutable(root, path, []byte("first\n"), nil); err != nil {
		t.Fatal(err)
	}
	before := childOutsideSnapshot(t, outside)
	fired := false
	err := writeMutable(root, path, []byte("second\n"), func(point FailurePoint) error {
		if point != FailureBeforeRename || fired {
			return nil
		}
		fired = true
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Symlink(filepath.Join(outside, "sentinel"), path)
	})
	if !fired {
		t.Fatal("checkpoint substitution did not run")
	}
	assertChildUnsafe(t, err, ".revolvr/autonomous/child-publications/rename.json", outside, before)
	assertNoChildTemp(t, filepath.Dir(path), ".child-journal-")
}

func TestChildImmutablePublicationRevalidatesBeforeLink(t *testing.T) {
	root, outside := t.TempDir(), childOutside(t)
	rel := ".revolvr/autonomous/tasks/child-one/state.json"
	path := filepath.Join(root, filepath.FromSlash(rel))
	raw := []byte("exact child state\n")
	before := childOutsideSnapshot(t, outside)
	fired := false
	err := writeImmutable(root, rel, raw, hash(raw), func(point FailurePoint) error {
		if point != FailureBeforeLink || fired {
			return nil
		}
		fired = true
		return os.Symlink(filepath.Join(outside, "sentinel"), path)
	})
	if !fired {
		t.Fatal("immutable target substitution did not run")
	}
	assertChildUnsafe(t, err, filepath.ToSlash(rel), outside, before)
	assertNoChildTemp(t, filepath.Dir(path), ".child-immutable-")
}

func TestChildTempCleanupDoesNotFollowSubstitutedParent(t *testing.T) {
	root, outside := t.TempDir(), childOutside(t)
	dir := filepath.Join(root, ".revolvr", "autonomous", "child-publications")
	path := filepath.Join(dir, "parent.json")
	if err := writeMutable(root, path, []byte("first\n"), nil); err != nil {
		t.Fatal(err)
	}
	moved := dir + ".moved"
	var before string
	fired := false
	err := writeMutable(root, path, []byte("second\n"), func(point FailurePoint) error {
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
			if strings.HasPrefix(entry.Name(), ".child-journal-") {
				temp = entry.Name()
				break
			}
		}
		if temp == "" {
			return errors.New("child temporary checkpoint not found")
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
		before = childOutsideSnapshot(t, outside)
		return nil
	})
	if !fired || before == "" {
		t.Fatal("parent substitution did not run")
	}
	assertChildUnsafe(t, err, ".revolvr/autonomous/child-publications", outside, before)
}

func TestChildLockRejectsUnsafePathsAndOpenedFileSubstitution(t *testing.T) {
	for _, component := range []string{".revolvr", ".revolvr/locks"} {
		t.Run("ancestor "+component, func(t *testing.T) {
			root, outside := t.TempDir(), childOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			childSymlink(t, outside, link)
			before := childOutsideSnapshot(t, outside)
			release, err := lock(context.Background(), root, nil)
			if release != nil {
				release()
			}
			assertChildUnsafe(t, err, component, outside, before)
		})
	}
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{"symlink", func(t *testing.T, path, outside string) { childSymlink(t, filepath.Join(outside, "sentinel"), path) }},
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
			childWrite(t, path, nil, 0o600)
			if err := os.Chmod(path, 0o666); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run("final "+test.name, func(t *testing.T) {
			root, outside := t.TempDir(), childOutside(t)
			dir := filepath.Join(root, ".revolvr", "locks")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "child-publication.lock")
			test.setup(t, path, outside)
			before := childOutsideSnapshot(t, outside)
			release, err := lock(context.Background(), root, nil)
			if release != nil {
				release()
			}
			assertChildUnsafe(t, err, ".revolvr/locks/child-publication.lock", outside, before)
		})
	}

	t.Run("opened file substitution", func(t *testing.T) {
		root, outside := t.TempDir(), childOutside(t)
		path := filepath.Join(root, ".revolvr", "locks", "child-publication.lock")
		before := childOutsideSnapshot(t, outside)
		fired := false
		release, err := lock(context.Background(), root, func(point FailurePoint) error {
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
			t.Fatal("opened lock substitution did not run")
		}
		assertChildUnsafe(t, err, ".revolvr/locks/child-publication.lock", outside, before)
	})
}

func TestChildAuthorityReadsRejectUnsafeCheckpointAndHistory(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
	}{
		{"checkpoint symlink", "checkpoint"},
		{"history hard link", "history"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), childOutside(t)
			journal := childPathJournal("child-read")
			if err := persist(root, Journal{}, journal, nil); err != nil {
				t.Fatal(err)
			}
			var path string
			if test.target == "checkpoint" {
				path = filepath.Join(root, ".revolvr", "autonomous", "child-publications", journal.OperationID+".json")
			} else {
				path = filepath.Join(root, ".revolvr", "autonomous", "child-publications", "history", autonomouschildpublication.HistoryFilename(journal.OperationID, 1))
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if test.target == "checkpoint" {
				childSymlink(t, filepath.Join(outside, "sentinel"), path)
			} else if err := os.Link(filepath.Join(outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
			before := childOutsideSnapshot(t, outside)
			_, _, err := autonomouschildpublication.Load(root, journal.OperationID)
			assertChildUnsafe(t, err, filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), outside, before)
		})
	}
}

func childPathJournal(operationID string) Journal {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	journal := Journal{SchemaVersion: JournalSchemaVersion, OperationID: operationID, ParentTaskID: "parent", DecisionID: "decision-one", ProposalID: "proposal-one", MaterialSHA256: strings.Repeat("a", 64), Stage: StageAdmitted, Sequence: 1, CreatedAt: now}
	taskID := autonomouschildpublication.ChildTaskID(journal.ParentTaskID, journal.DecisionID, journal.ProposalID, "child-one")
	journal.Children = []ChildRecord{{TaskID: taskID, ProposalKey: "child-one", TaskPath: filepath.ToSlash(filepath.Join(".agent", "tasks", taskID+".md")), TaskSHA256: strings.Repeat("b", 64), StatePath: filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json")), StateSHA256: strings.Repeat("c", 64)}}
	return journal
}

func childOutside(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	childWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
	return outside
}

func childOutsideSnapshot(t *testing.T, outside string) string {
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
		links = stat.Nlink
	}
	return fmt.Sprintf("%v|%04o|%d|%x", names, info.Mode().Perm(), links, raw)
}

func assertChildUnsafe(t *testing.T, err error, component, outside, before string) {
	t.Helper()
	if !errors.Is(err, runtimepath.ErrUnsafe) || !strings.Contains(err.Error(), component) {
		t.Fatalf("error = %v, want unsafe component %q", err, component)
	}
	if after := childOutsideSnapshot(t, outside); after != before {
		t.Fatalf("outside sentinel changed\nbefore: %s\nafter:  %s", before, after)
	}
}

func childWrite(t *testing.T, path string, raw []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatal(err)
	}
}

func childSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func assertNoChildTemp(t *testing.T, dir, prefix string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			t.Fatalf("child temporary survived rejected substitution: %s", entry.Name())
		}
	}
}
