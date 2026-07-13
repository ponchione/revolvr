package autonomousexec

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

func TestLeaseExcludesDirectAndQueueDriversAndCancelsPromptly(t *testing.T) {
	root := t.TempDir()
	release, err := Acquire(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryAcquire(root); !errors.Is(err, ErrActive) {
		t.Fatalf("nonblocking contention err=%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, root); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended acquire err=%v", err)
	}
	release()
	second, err := Acquire(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	second()
	probe, err := TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	probe()
}

func TestLeaseRejectsEverySymlinkedAncestorWithoutOutsideMutation(t *testing.T) {
	for _, component := range []string{".revolvr", ".revolvr/locks"} {
		t.Run(component, func(t *testing.T) {
			root, outside := t.TempDir(), executionOutside(t)
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			before := executionOutsideSnapshot(t, outside)
			release, err := Acquire(context.Background(), root)
			if release != nil {
				release()
			}
			assertExecutionUnsafe(t, err, component, outside, before)
		})
	}
}

func TestLeaseRejectsUnsafeFinalComponentsWithoutOutsideMutation(t *testing.T) {
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
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), executionOutside(t)
			dir := filepath.Join(root, ".revolvr", "locks")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "autonomous-execution.lock")
			test.setup(t, path, outside)
			before := executionOutsideSnapshot(t, outside)
			release, err := Acquire(context.Background(), root)
			if release != nil {
				release()
			}
			assertExecutionUnsafe(t, err, ".revolvr/locks/autonomous-execution.lock", outside, before)
		})
	}
}

func executionOutside(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return outside
}

func executionOutsideSnapshot(t *testing.T, outside string) string {
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

func assertExecutionUnsafe(t *testing.T, err error, component, outside, before string) {
	t.Helper()
	if !errors.Is(err, runtimepath.ErrUnsafe) || !strings.Contains(err.Error(), component) {
		t.Fatalf("error = %v, want unsafe component %q", err, component)
	}
	if after := executionOutsideSnapshot(t, outside); after != before {
		t.Fatalf("outside sentinel changed\nbefore: %s\nafter:  %s", before, after)
	}
}
