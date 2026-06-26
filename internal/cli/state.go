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
	return nil
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
