package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"revolvr/internal/runtimepath"
)

const defaultFlockPollInterval = 10 * time.Millisecond

var (
	ErrFlockContended   = errors.New("file lock is contended")
	ErrFlockUnsupported = errors.New("file locking is unsupported on this platform")
)

type FlockMode uint8

const (
	FlockShared FlockMode = iota + 1
	FlockExclusive
)

type FlockConfig struct {
	RelativePath  string
	Mode          FlockMode
	Wait          bool
	Create        bool
	DirectoryMode os.FileMode
	FileMode      os.FileMode
	PollInterval  time.Duration
	PollIntervals []time.Duration
	AfterOpen     func(root, path string) error
}

// Flock is an identity-checked advisory lock below one canonical repository
// root. Check must be called immediately before using the descriptor for a
// destructive metadata operation.
type Flock struct {
	root  string
	path  string
	file  *os.File
	mu    sync.Mutex
	close error
}

// AcquireFlock opens and locks one protected runtime file. Existing and newly
// created ancestors, the named final component, and the opened descriptor are
// validated before the lock is attempted and again after it succeeds.
func AcquireFlock(ctx context.Context, repositoryRoot string, cfg FlockConfig) (*Flock, error) {
	root, err := runtimepath.CanonicalRoot(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("acquire file lock: canonical root: %w", err)
	}
	cfg, rel, err := normalizeFlockConfig(cfg)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, rel)
	if cfg.Create {
		if err := runtimepath.EnsureDir(root, filepath.Dir(path), cfg.DirectoryMode); err != nil {
			return nil, fmt.Errorf("acquire file lock %q: prepare directory: %w", filepath.ToSlash(rel), err)
		}
	} else if err := runtimepath.CheckDir(root, filepath.Dir(path), true); err != nil {
		return nil, fmt.Errorf("acquire file lock %q: validate directory: %w", filepath.ToSlash(rel), err)
	}
	if err := runtimepath.CheckFile(root, path, true); err != nil {
		return nil, fmt.Errorf("acquire file lock %q: validate final component: %w", filepath.ToSlash(rel), err)
	}
	file, err := openFlockFile(path, cfg.Create, cfg.FileMode)
	if noFollowSymlinkError(err) {
		return nil, fmt.Errorf("%w: %q became a symlink during lock open", runtimepath.ErrUnsafe, filepath.ToSlash(rel))
	}
	if err != nil {
		return nil, fmt.Errorf("acquire file lock %q: open: %w", filepath.ToSlash(rel), err)
	}
	closeFile := true
	defer func() {
		if closeFile {
			_ = file.Close()
		}
	}()
	if err := runtimepath.CheckOpenedFile(root, path, file); err != nil {
		return nil, fmt.Errorf("acquire file lock %q: validate opened file: %w", filepath.ToSlash(rel), err)
	}
	if cfg.AfterOpen != nil {
		if err := cfg.AfterOpen(root, path); err != nil {
			return nil, err
		}
	}
	attempt := 0
	for {
		err = tryFlock(file, cfg.Mode)
		if err == nil {
			if err := runtimepath.CheckOpenedFile(root, path, file); err != nil {
				_ = unlockFlock(file)
				return nil, fmt.Errorf("acquire file lock %q: validate locked file: %w", filepath.ToSlash(rel), err)
			}
			closeFile = false
			return &Flock{root: root, path: path, file: file}, nil
		}
		if !flockWouldBlock(err) {
			return nil, fmt.Errorf("acquire file lock %q: flock: %w", filepath.ToSlash(rel), err)
		}
		if !cfg.Wait {
			return nil, fmt.Errorf("acquire file lock %q: %w: %w", filepath.ToSlash(rel), ErrFlockContended, err)
		}
		pollInterval := cfg.PollInterval
		if len(cfg.PollIntervals) > 0 {
			index := min(attempt, len(cfg.PollIntervals)-1)
			pollInterval = cfg.PollIntervals[index]
		}
		attempt++
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func normalizeFlockConfig(cfg FlockConfig) (FlockConfig, string, error) {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(cfg.RelativePath)))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return FlockConfig{}, "", errors.New("acquire file lock: safe repository-relative path is required")
	}
	if cfg.Mode != FlockShared && cfg.Mode != FlockExclusive {
		return FlockConfig{}, "", errors.New("acquire file lock: shared or exclusive mode is required")
	}
	if cfg.DirectoryMode == 0 {
		cfg.DirectoryMode = 0o700
	}
	if cfg.FileMode == 0 {
		cfg.FileMode = 0o600
	}
	if cfg.DirectoryMode.Perm()&0o022 != 0 || cfg.FileMode.Perm()&0o022 != 0 {
		return FlockConfig{}, "", errors.New("acquire file lock: directory and file modes must not be group/world writable")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultFlockPollInterval
	}
	if len(cfg.PollIntervals) > 0 {
		cfg.PollIntervals = append([]time.Duration(nil), cfg.PollIntervals...)
		for _, interval := range cfg.PollIntervals {
			if interval <= 0 {
				return FlockConfig{}, "", errors.New("acquire file lock: poll intervals must be positive")
			}
		}
	}
	return cfg, rel, nil
}

// Check proves that the held descriptor still identifies its protected named
// component. Callers use it immediately before truncation or replacement.
func (l *Flock) Check() error {
	if l == nil {
		return errors.New("file lock is nil")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return errors.New("file lock is closed")
	}
	return runtimepath.CheckOpenedFile(l.root, l.path, l.file)
}

func (l *Flock) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Flock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return l.close
	}
	l.close = errors.Join(unlockFlock(l.file), l.file.Close())
	l.file = nil
	return l.close
}
