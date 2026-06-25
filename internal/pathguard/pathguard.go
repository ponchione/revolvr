package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrEmptyPath    = errors.New("path is empty")
	ErrAbsolutePath = errors.New("absolute path is not allowed")
	ErrEscapesRoot  = errors.New("path escapes root")
)

func Resolve(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", ErrEmptyPath
	}
	if filepath.IsAbs(rel) {
		return "", ErrAbsolutePath
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve root symlinks: %w", err)
	}

	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrEscapesRoot
	}

	target := filepath.Join(absRoot, clean)
	if !WithinRoot(absRoot, target) {
		return "", ErrEscapesRoot
	}

	nearest, err := nearestExistingPath(absRoot, target)
	if err != nil {
		return "", err
	}
	resolvedNearest, err := filepath.EvalSymlinks(nearest)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks: %w", err)
	}
	if !WithinRoot(resolvedRoot, resolvedNearest) {
		return "", ErrEscapesRoot
	}

	return target, nil
}

func nearestExistingPath(root, target string) (string, error) {
	current := target
	for {
		if !WithinRoot(root, current) {
			return "", ErrEscapesRoot
		}
		if _, err := os.Stat(current); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat path: %w", err)
		}

		if current == root {
			return "", fmt.Errorf("root does not exist: %s", root)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ErrEscapesRoot
		}
		current = parent
	}
}

func WithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
