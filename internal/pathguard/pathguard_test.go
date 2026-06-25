package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRejectsEmptyPath(t *testing.T) {
	_, err := Resolve(t.TempDir(), " \t ")
	if !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("error = %v, want %v", err, ErrEmptyPath)
	}
}

func TestResolveRejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()

	_, err := Resolve(root, filepath.Join(root, "file.txt"))
	if !errors.Is(err, ErrAbsolutePath) {
		t.Fatalf("error = %v, want %v", err, ErrAbsolutePath)
	}
}

func TestResolveRejectsDotDotEscape(t *testing.T) {
	for _, rel := range []string{"../outside.txt", "nested/../../outside.txt"} {
		t.Run(rel, func(t *testing.T) {
			_, err := Resolve(t.TempDir(), rel)
			if !errors.Is(err, ErrEscapesRoot) {
				t.Fatalf("error = %v, want %v", err, ErrEscapesRoot)
			}
		})
	}
}

func TestResolveAllowsNormalNestedPath(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "tasks", "open"))
	mustWriteFile(t, filepath.Join(root, "tasks", "open", "task.md"), []byte("task"))

	got, err := Resolve(root, "tasks/open/task.md")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := filepath.Join(root, "tasks", "open", "task.md"); got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestResolveAllowsMissingLeafUnderValidRoot(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "receipts"))

	got, err := Resolve(root, "receipts/run-1.md")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := filepath.Join(root, "receipts", "run-1.md"); got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestResolveAllowsSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "actual")
	mustMkdirAll(t, target)
	link := filepath.Join(root, "link")
	mustSymlink(t, target, link)

	got, err := Resolve(root, "link/missing-leaf.txt")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := filepath.Join(root, "link", "missing-leaf.txt"); got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
}

func TestResolveRejectsSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	mustSymlink(t, outside, link)

	_, err := Resolve(root, "escape/missing-leaf.txt")
	if !errors.Is(err, ErrEscapesRoot) {
		t.Fatalf("error = %v, want %v", err, ErrEscapesRoot)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("symlink %s -> %s: %v", newname, oldname, err)
	}
}
