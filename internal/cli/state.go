package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/ledger"
	"revolvr/internal/taskqueue"
)

const revolvrStateDir = ".revolvr"
const revolvrGitExcludePattern = "/.revolvr/"

type statePaths struct {
	WorkDir      string
	StateDir     string
	TaskDBPath   string
	LedgerDBPath string
	RunsDir      string
	ReceiptsDir  string
	LocksDir     string
}

func resolveStatePaths(workDir string) (statePaths, error) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return statePaths{}, fmt.Errorf("resolve working directory: %w", err)
		}
		workDir = wd
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return statePaths{}, fmt.Errorf("resolve working directory: %w", err)
	}

	stateDir := filepath.Join(absWorkDir, revolvrStateDir)
	return statePaths{
		WorkDir:      absWorkDir,
		StateDir:     stateDir,
		TaskDBPath:   filepath.Join(stateDir, "tasks.sqlite"),
		LedgerDBPath: filepath.Join(stateDir, "ledger.sqlite"),
		RunsDir:      filepath.Join(stateDir, "runs"),
		ReceiptsDir:  filepath.Join(stateDir, "receipts"),
		LocksDir:     filepath.Join(stateDir, "locks"),
	}, nil
}

func initializeState(ctx context.Context, paths statePaths) error {
	for _, dir := range []string{paths.StateDir, paths.RunsDir, paths.ReceiptsDir, paths.LocksDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("initialize state: create %s: %w", dir, err)
		}
	}

	tasks, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		return err
	}
	if err := tasks.Close(); err != nil {
		return fmt.Errorf("close task queue: %w", err)
	}

	runs, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		return err
	}
	if err := runs.Close(); err != nil {
		return fmt.Errorf("close ledger: %w", err)
	}
	if err := ensureStateIgnoredByGit(paths.WorkDir); err != nil {
		return err
	}
	return nil
}

func ensureStateIgnoredByGit(workDir string) error {
	excludePath, ok, err := gitExcludePath(workDir)
	if err != nil {
		return fmt.Errorf("initialize state: resolve git exclude: %w", err)
	}
	if !ok {
		return nil
	}
	if err := ensureExcludePattern(excludePath, revolvrGitExcludePattern); err != nil {
		return fmt.Errorf("initialize state: update git exclude: %w", err)
	}
	return nil
}

func gitExcludePath(workDir string) (string, bool, error) {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect %s: %w", gitPath, err)
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "info", "exclude"), true, nil
	}

	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", gitPath, err)
	}
	firstLine := strings.TrimSpace(strings.SplitN(string(content), "\n", 2)[0])
	const prefix = "gitdir:"
	if !strings.HasPrefix(firstLine, prefix) {
		return "", false, fmt.Errorf("%s is not a Git directory or worktree gitdir file", gitPath)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(firstLine, prefix))
	if gitDir == "" {
		return "", false, fmt.Errorf("%s has an empty gitdir", gitPath)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	return filepath.Join(filepath.Clean(gitDir), "info", "exclude"), true, nil
}

func ensureExcludePattern(path string, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if excludeContentHasPattern(string(content), pattern) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	updated := string(content)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += pattern + "\n"
	return os.WriteFile(path, []byte(updated), 0o644)
}

func excludeContentHasPattern(content string, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == pattern {
			return true
		}
	}
	return false
}

func openTaskStore(ctx context.Context, opts Options) (*taskqueue.Store, func(), error) {
	paths, err := resolveStatePaths(opts.WorkDir)
	if err != nil {
		return nil, nil, err
	}
	store, err := taskqueue.Open(ctx, paths.TaskDBPath)
	if err != nil {
		return nil, nil, err
	}
	return store, func() { _ = store.Close() }, nil
}

func openLedgerStore(ctx context.Context, opts Options) (*ledger.Store, func(), error) {
	paths, err := resolveStatePaths(opts.WorkDir)
	if err != nil {
		return nil, nil, err
	}
	store, err := ledger.Open(ctx, paths.LedgerDBPath)
	if err != nil {
		return nil, nil, err
	}
	return store, func() { _ = store.Close() }, nil
}

func stateInitialized(paths statePaths) (bool, error) {
	return pathsInitialized(paths, paths.TaskDBPath, paths.LedgerDBPath)
}

func ledgerInitialized(paths statePaths) (bool, error) {
	return pathsInitialized(paths, paths.LedgerDBPath)
}

func pathsInitialized(paths statePaths, storePaths ...string) (bool, error) {
	stateDir, err := existingDir(paths.StateDir)
	if err != nil || !stateDir {
		return stateDir, err
	}
	for _, storePath := range storePaths {
		exists, err := existingFile(storePath)
		if err != nil || !exists {
			return exists, err
		}
	}
	return true, nil
}

func existingDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	return info.IsDir(), nil
}

func existingFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	return !info.IsDir(), nil
}
