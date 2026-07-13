package runtimepath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func TestEnsureDirRejectsEverySymlinkedComponentWithoutOutsideMutation(t *testing.T) {
	components := []string{
		".revolvr",
		".revolvr/autonomous",
		".revolvr/autonomous/task-runs",
		".revolvr/autonomous/task-runs/run-one",
		".revolvr/autonomous/task-runs/run-one/history",
	}
	for _, component := range components {
		t.Run(component, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			mustRuntimeWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
			target := filepath.Join(root, filepath.FromSlash(".revolvr/autonomous/task-runs/run-one/history"))
			link := filepath.Join(root, filepath.FromSlash(component))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			before := runtimeTreeSnapshot(t, outside)
			err := EnsureDir(root, target, 0o700)
			if !errors.Is(err, ErrUnsafe) || !strings.Contains(err.Error(), component) {
				t.Fatalf("error = %v, want unsafe component %q", err, component)
			}
			if after := runtimeTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
				t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
			}
		})
	}
}

func TestRuntimePathRejectsWrongTypesLinksAndModes(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, string, string)
		check  func(string, string) error
		detail string
	}{
		{
			name: "regular ancestor", detail: ".revolvr/autonomous",
			setup: func(t *testing.T, root, _ string) {
				mustRuntimeWrite(t, filepath.Join(root, ".revolvr", "autonomous"), []byte("not-directory"), 0o600)
			},
			check: func(root, _ string) error {
				return EnsureDir(root, filepath.Join(root, ".revolvr", "autonomous", "task-runs"), 0o700)
			},
		},
		{
			name: "unsafe directory mode", detail: ".revolvr/autonomous",
			setup: func(t *testing.T, root, _ string) {
				path := filepath.Join(root, ".revolvr", "autonomous")
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o777); err != nil {
					t.Fatal(err)
				}
			},
			check: func(root, _ string) error {
				return EnsureDir(root, filepath.Join(root, ".revolvr", "autonomous", "task-runs"), 0o700)
			},
		},
		{
			name: "final symlink", detail: "protected",
			setup: func(t *testing.T, root, outside string) {
				mustRuntimeParents(t, root)
				if err := os.Symlink(filepath.Join(outside, "sentinel"), filepath.Join(root, ".revolvr", "protected")); err != nil {
					t.Fatal(err)
				}
			},
			check: func(root, _ string) error {
				return CheckFile(root, filepath.Join(root, ".revolvr", "protected"), false)
			},
		},
		{
			name: "final directory", detail: "protected",
			setup: func(t *testing.T, root, _ string) {
				mustRuntimeParents(t, root)
				if err := os.Mkdir(filepath.Join(root, ".revolvr", "protected"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			check: func(root, _ string) error {
				return CheckFile(root, filepath.Join(root, ".revolvr", "protected"), false)
			},
		},
		{
			name: "final fifo", detail: "protected",
			setup: func(t *testing.T, root, _ string) {
				mustRuntimeParents(t, root)
				if err := syscall.Mkfifo(filepath.Join(root, ".revolvr", "protected"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			check: func(root, _ string) error {
				return CheckFile(root, filepath.Join(root, ".revolvr", "protected"), false)
			},
		},
		{
			name: "final hard link", detail: "protected",
			setup: func(t *testing.T, root, outside string) {
				mustRuntimeParents(t, root)
				if err := os.Link(filepath.Join(outside, "sentinel"), filepath.Join(root, ".revolvr", "protected")); err != nil {
					t.Fatal(err)
				}
			},
			check: func(root, _ string) error {
				return CheckFile(root, filepath.Join(root, ".revolvr", "protected"), false)
			},
		},
		{
			name: "unsafe final mode", detail: "protected",
			setup: func(t *testing.T, root, _ string) {
				mustRuntimeParents(t, root)
				path := filepath.Join(root, ".revolvr", "protected")
				mustRuntimeWrite(t, path, []byte("unsafe"), 0o600)
				if err := os.Chmod(path, 0o666); err != nil {
					t.Fatal(err)
				}
			},
			check: func(root, _ string) error {
				return CheckFile(root, filepath.Join(root, ".revolvr", "protected"), false)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			mustRuntimeWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
			test.setup(t, root, outside)
			before := runtimeTreeSnapshot(t, outside)
			err := test.check(root, outside)
			if !errors.Is(err, ErrUnsafe) || !strings.Contains(err.Error(), test.detail) {
				t.Fatalf("error = %v, want unsafe component containing %q", err, test.detail)
			}
			if after := runtimeTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
				t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
			}
		})
	}
}

func TestCheckOpenedFileRejectsFinalSubstitutionBeforeUse(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mustRuntimeParents(t, root)
	mustRuntimeWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
	path := filepath.Join(root, ".revolvr", "protected")
	mustRuntimeWrite(t, path, []byte("inside"), 0o600)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "sentinel"), path); err != nil {
		t.Fatal(err)
	}
	before := runtimeTreeSnapshot(t, outside)
	if err := CheckOpenedFile(root, path, file); !errors.Is(err, ErrUnsafe) || !strings.Contains(err.Error(), ".revolvr/protected") {
		t.Fatalf("error = %v, want substituted final component", err)
	}
	if after := runtimeTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
		t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestCheckOpenedDirRejectsFinalSubstitutionBeforeUse(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	mustRuntimeParents(t, root)
	mustRuntimeWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
	path := filepath.Join(root, ".revolvr", "protected")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	before := runtimeTreeSnapshot(t, outside)
	if err := CheckOpenedDir(root, path, dir); !errors.Is(err, ErrUnsafe) || !strings.Contains(err.Error(), ".revolvr/protected") {
		t.Fatalf("error = %v, want substituted directory", err)
	}
	if after := runtimeTreeSnapshot(t, outside); !reflect.DeepEqual(after, before) {
		t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestProtectedReadHelpersUseNamedOpenedIdentities(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".revolvr", "protected")
	if err := EnsureDir(root, dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	want := []byte("protected bytes\n")
	mustRuntimeWrite(t, path, want, 0o600)
	raw, found, err := ReadFile(root, path, false)
	if err != nil || !found || !reflect.DeepEqual(raw, want) {
		t.Fatalf("ReadFile = %q found=%t err=%v", raw, found, err)
	}
	entries, found, err := ReadDir(root, dir, false)
	if err != nil || !found || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("ReadDir = %+v found=%t err=%v", entries, found, err)
	}
	if _, found, err := ReadFile(root, filepath.Join(dir, "missing"), true); err != nil || found {
		t.Fatalf("missing ReadFile found=%t err=%v", found, err)
	}
	if _, found, err := ReadDir(root, filepath.Join(dir, "missing"), true); err != nil || found {
		t.Fatalf("missing ReadDir found=%t err=%v", found, err)
	}
	if err := SyncDir(root, dir); err != nil {
		t.Fatalf("SyncDir: %v", err)
	}
}

func TestRuntimePathAllowsOrdinaryCreateAndReopen(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".revolvr", "autonomous", "task-runs", "run-one")
	if err := EnsureDir(root, dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "operation.lock")
	mustRuntimeWrite(t, path, []byte(""), 0o600)
	if err := CheckDir(root, dir, false); err != nil {
		t.Fatal(err)
	}
	if err := CheckFile(root, path, false); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := CheckOpenedFile(root, path, file); err != nil {
		t.Fatal(err)
	}
}

func mustRuntimeParents(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".revolvr"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustRuntimeWrite(t *testing.T, path string, raw []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatal(err)
	}
}

func runtimeTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
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
		value := fmt.Sprintf("%s|%s|%04o", filepath.ToSlash(rel), info.Mode().Type(), info.Mode().Perm())
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
	return result
}
