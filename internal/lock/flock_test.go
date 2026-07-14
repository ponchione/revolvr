package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/runtimepath"
)

func TestAcquireFlockRejectsUnsafePathsWithoutOutsideMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root, outside, sentinel, lockPath string)
	}{
		{
			name: "final symlink",
			setup: func(t *testing.T, root, _, sentinel, lockPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(sentinel, lockPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hard link alias",
			setup: func(t *testing.T, root, _, sentinel, lockPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(sentinel, lockPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked ancestor",
			setup: func(t *testing.T, root, outside, _, _ string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(root, ".revolvr"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, ".revolvr", "locks")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "sentinel.txt")
			const sentinelBytes = "outside sentinel\n"
			if err := os.WriteFile(sentinel, []byte(sentinelBytes), 0o600); err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(root, ".revolvr", "locks", "coordinator.lock")
			tt.setup(t, root, outside, sentinel, lockPath)

			lease, err := AcquireFlock(context.Background(), root, FlockConfig{
				RelativePath: ".revolvr/locks/coordinator.lock",
				Mode:         FlockExclusive,
				Wait:         false,
				Create:       true,
			})
			if err == nil {
				_ = lease.Close()
				t.Fatal("unsafe lock path was acquired")
			}
			if !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("acquire error = %v, want runtimepath.ErrUnsafe", err)
			}
			raw, readErr := os.ReadFile(sentinel)
			if readErr != nil || string(raw) != sentinelBytes {
				t.Fatalf("outside sentinel changed: err=%v bytes=%q", readErr, raw)
			}
			entries, readErr := os.ReadDir(outside)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != "sentinel.txt" {
				t.Fatalf("outside directory changed: err=%v entries=%v", readErr, entries)
			}
		})
	}
}

func TestAcquireFlockRejectsPathSubstitutionBetweenOpenAndFlock(t *testing.T) {
	root := t.TempDir()
	rel := ".revolvr/locks/substituted.lock"
	var originalPath string
	lease, err := AcquireFlock(context.Background(), root, FlockConfig{
		RelativePath: rel,
		Mode:         FlockExclusive,
		Wait:         false,
		Create:       true,
		AfterOpen: func(_, path string) error {
			originalPath = path + ".opened"
			if err := os.Rename(path, originalPath); err != nil {
				return err
			}
			return os.WriteFile(path, []byte("replacement\n"), 0o600)
		},
	})
	if err == nil {
		_ = lease.Close()
		t.Fatal("substituted lock path was acquired")
	}
	if !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("acquire error = %v, want runtimepath.ErrUnsafe", err)
	}
	if raw, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); readErr != nil || string(raw) != "replacement\n" {
		t.Fatalf("replacement path changed: err=%v bytes=%q", readErr, raw)
	}
	if info, statErr := os.Lstat(originalPath); statErr != nil || !info.Mode().IsRegular() {
		t.Fatalf("opened inode was not retained for inspection: err=%v info=%v", statErr, info)
	}
}

func TestFlockCheckRejectsReplacementAfterAcquisition(t *testing.T) {
	root := t.TempDir()
	lease, err := AcquireFlock(context.Background(), root, FlockConfig{
		RelativePath: ".revolvr/locks/held.lock",
		Mode:         FlockExclusive,
		Wait:         false,
		Create:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if info, statErr := os.Lstat(lease.Path()); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("created lock mode = %v, err=%v, want 0600", info, statErr)
	}
	openedPath := lease.Path() + ".opened"
	if err := os.Rename(lease.Path(), openedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lease.Path(), []byte("replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.Check(); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("Check error = %v, want runtimepath.ErrUnsafe", err)
	}
}

func TestAcquireFlockReportsPortableContention(t *testing.T) {
	root := t.TempDir()
	cfg := FlockConfig{
		RelativePath: ".revolvr/locks/contended.lock",
		Mode:         FlockExclusive,
		Wait:         false,
		Create:       true,
	}
	owner, err := AcquireFlock(context.Background(), root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	contender, err := AcquireFlock(context.Background(), root, cfg)
	if contender != nil {
		_ = contender.Close()
		t.Fatal("contended lock was acquired")
	}
	if !errors.Is(err, ErrFlockContended) {
		t.Fatalf("contended acquire error = %v, want ErrFlockContended", err)
	}
}

func TestAcquireFlockRejectsInvalidPollSchedule(t *testing.T) {
	lease, err := AcquireFlock(context.Background(), t.TempDir(), FlockConfig{
		RelativePath:  ".revolvr/locks/invalid-poll.lock",
		Mode:          FlockExclusive,
		Wait:          true,
		Create:        true,
		PollIntervals: []time.Duration{time.Millisecond, 0},
	})
	if lease != nil {
		_ = lease.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "poll intervals") {
		t.Fatalf("invalid poll schedule error = %v", err)
	}
}
