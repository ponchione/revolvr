// Package runtimepath enforces the filesystem trust boundary for harness-owned
// runtime directories and protected files below a canonical repository root.
package runtimepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	parts, err := relativeParts(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if mkdirErr := os.Mkdir(current, mode); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return mkdirErr
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if err := checkDirectory(root, current, info); err != nil {
			return err
		}
	}
	return CheckDir(root, target, false)
}

// CheckDir validates every existing component through target. When missingOK
// is true, an absent component and its necessarily absent descendants are safe.
func CheckDir(root, target string, missingOK bool) error {
	parts, err := relativeParts(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) && missingOK {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if err := checkDirectory(root, current, info); err != nil {
			return err
		}
	}
	return nil
}

// CheckFile validates directory ancestors and an existing protected regular
// file. A protected file must not be writable by group/other or have aliases.
func CheckFile(root, path string, missingOK bool) error {
	_, _, err := protectedFileInfo(root, path, missingOK)
	return err
}

// CheckOpenedFile proves that file is the same protected regular file named by
// path. Callers use it before locking/writing and again after sensitive opens.
func CheckOpenedFile(root, path string, file *os.File) error {
	pathInfo, found, err := protectedFileInfo(root, path, false)
	if err != nil {
		return err
	}
	if !found || file == nil {
		return unsafe(root, path, "opened file identity is missing")
	}
	openInfo, err := file.Stat()
	if err != nil {
		return err
	}
	if err := checkRegularFile(root, path, openInfo); err != nil {
		return err
	}
	if !os.SameFile(pathInfo, openInfo) {
		return unsafe(root, path, "opened file does not match the named component")
	}
	return nil
}

func protectedFileInfo(root, path string, missingOK bool) (os.FileInfo, bool, error) {
	if err := CheckDir(root, filepath.Dir(path), missingOK); err != nil {
		return nil, false, err
	}
	if _, err := relativeParts(root, path); err != nil {
		return nil, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := checkRegularFile(root, path, info); err != nil {
		return nil, false, err
	}
	return info, true, nil
}

func checkDirectory(root, path string, info os.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return unsafe(root, path, "is a symlink")
	case !info.IsDir():
		return unsafe(root, path, "is not a directory")
	case info.Mode().Perm()&0o022 != 0:
		return unsafe(root, path, fmt.Sprintf("has unsafe directory mode %04o", info.Mode().Perm()))
	default:
		return nil
	}
}

func checkRegularFile(root, path string, info os.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return unsafe(root, path, "is a symlink")
	case !info.Mode().IsRegular():
		return unsafe(root, path, "is not a regular file")
	case info.Mode().Perm()&0o022 != 0:
		return unsafe(root, path, fmt.Sprintf("has unsafe file mode %04o", info.Mode().Perm()))
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return unsafe(root, path, "has an unexpected hard-link count")
	}
	return nil
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
