// Package runtimepath enforces the filesystem trust boundary for harness-owned
// runtime directories and protected files below a canonical repository root.
package runtimepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsafe = errors.New("unsafe harness runtime path")

// CanonicalRoot resolves the repository root once. Symlinks below the returned
// root remain forbidden by the component checks in this package.
func CanonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("harness runtime path: repository root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", unsafe(resolved, resolved, "repository root is not a directory")
	}
	return resolved, nil
}

// EnsureDir creates missing components one at a time and validates every
// existing and newly created component without following symlinks.
func EnsureDir(root, target string, mode os.FileMode) error {
	boundary, err := Bind(root)
	if err != nil {
		return err
	}
	return boundary.EnsureDir(target, mode)
}

// CheckDir validates every existing component through target. When missingOK
// is true, an absent component and its necessarily absent descendants are safe.
func CheckDir(root, target string, missingOK bool) error {
	boundary, err := Bind(root)
	if err != nil {
		return err
	}
	dir, found, err := boundary.OpenDir(target, missingOK)
	if err != nil {
		return err
	}
	if !found {
		if missingOK {
			return nil
		}
		return os.ErrNotExist
	}
	return dir.Close()
}

// CheckFile validates directory ancestors and an existing protected regular
// file. A protected file must not be writable by group/other or have aliases.
func CheckFile(root, path string, missingOK bool) error {
	boundary, err := Bind(root)
	if err != nil {
		return err
	}
	return boundary.CheckFile(path, missingOK)
}

// CheckOpenedFile proves that file is the same protected regular file named by
// path. Callers use it before locking/writing and again after sensitive opens.
func CheckOpenedFile(root, path string, file *os.File) error {
	boundary, err := Bind(root)
	if err != nil {
		return err
	}
	if file == nil {
		return unsafe(root, path, "opened file identity is missing")
	}
	dir, found, err := boundary.OpenDir(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	if !found {
		return unsafe(root, path, "opened file parent is missing")
	}
	defer dir.Close()
	named, found, err := dir.namedFileStat(filepath.Base(path))
	if err != nil {
		return err
	}
	if !found {
		return unsafe(root, path, "opened file is no longer named")
	}
	opened, err := fstat(file)
	if err != nil {
		return err
	}
	if err := checkRegularFileStat(root, path, named); err != nil {
		return err
	}
	if err := checkRegularFileStat(root, path, opened); err != nil {
		return err
	}
	if identityOf(named) != identityOf(opened) {
		return unsafe(root, path, "opened file does not match the named component")
	}
	return nil
}

// CheckOpenedDir proves that dir is the same protected directory named by
// path. It closes the check/use gap for directory enumeration and sync.
func CheckOpenedDir(root, path string, dir *os.File) error {
	boundary, err := Bind(root)
	if err != nil {
		return err
	}
	if dir == nil {
		return unsafe(root, path, "opened directory identity is missing")
	}
	named, found, err := boundary.OpenDir(path, false)
	if err != nil {
		return err
	}
	if !found {
		return unsafe(root, path, "opened directory is no longer named")
	}
	defer named.Close()
	opened, err := fstat(dir)
	if err != nil {
		return err
	}
	if err := checkDirectoryStat(root, path, opened); err != nil {
		return err
	}
	if named.identity != identityOf(opened) {
		return unsafe(root, path, "opened directory does not match the named component")
	}
	return nil
}

// OpenFile opens one protected regular file without following its final
// component and proves that the opened descriptor matches the named file.
// O_TRUNC is forbidden because truncation would precede the identity check;
// callers that mutate an existing file must do so only after this returns.
func OpenFile(root, path string, flag int, perm os.FileMode) (*os.File, error) {
	boundary, err := Bind(root)
	if err != nil {
		return nil, err
	}
	dir, found, err := boundary.OpenDir(filepath.Dir(path), false)
	if err != nil || !found {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, err
	}
	defer dir.Close()
	protected, err := dir.OpenFile(filepath.Base(path), flag, perm)
	if err != nil {
		return nil, err
	}
	return protected.release(), nil
}

// ReadFile opens and reads one protected regular file only after the opened
// descriptor is proven to match the named component. A substitution during
// the read is detected by the second identity check.
func ReadFile(root, path string, missingOK bool) ([]byte, bool, error) {
	boundary, err := Bind(root)
	if err != nil {
		return nil, false, err
	}
	dir, found, err := boundary.OpenDir(filepath.Dir(path), missingOK)
	if err != nil || !found {
		if err == nil && !missingOK {
			err = os.ErrNotExist
		}
		return nil, false, err
	}
	defer dir.Close()
	raw, found, err := dir.ReadFile(filepath.Base(path), missingOK)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	return raw, true, nil
}

// ReadDir enumerates one protected directory through an identity-checked open
// descriptor. Missing directories can be represented without following an
// attacker-controlled replacement.
func ReadDir(root, path string, missingOK bool) ([]os.DirEntry, bool, error) {
	boundary, err := Bind(root)
	if err != nil {
		return nil, false, err
	}
	dir, found, err := boundary.OpenDir(path, missingOK)
	if err != nil || !found {
		if err == nil && !missingOK {
			err = os.ErrNotExist
		}
		return nil, false, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir()
	if err != nil {
		return nil, false, err
	}
	return entries, true, nil
}

// SyncDir flushes one identity-checked protected directory and verifies that
// its named component did not change across the sync boundary.
func SyncDir(root, path string) error {
	boundary, err := Bind(root)
	if err != nil {
		return err
	}
	dir, found, err := boundary.OpenDir(path, false)
	if err != nil || !found {
		if err == nil {
			err = os.ErrNotExist
		}
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func relativeParts(root, target string) ([]string, error) {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, unsafe(root, target, "escapes the canonical repository root")
	}
	if rel == "." {
		return nil, nil
	}
	return strings.Split(rel, string(filepath.Separator)), nil
}

func unsafe(root, path, detail string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	return fmt.Errorf("%w: %q %s", ErrUnsafe, filepath.ToSlash(rel), detail)
}
