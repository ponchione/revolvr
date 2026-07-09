package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"revolvr/internal/ledger"
	"revolvr/internal/prompt"
	"revolvr/internal/taskfile"
)

const revolvrStateDir = ".revolvr"
const revolvrGitExcludePattern = "/.revolvr/"

type statePaths struct {
	WorkDir      string
	StateDir     string
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
	if err := seedDefaultRunProfiles(paths.WorkDir); err != nil {
		return err
	}
	if err := ensureTaskFilesDir(paths.WorkDir); err != nil {
		return err
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

func ensureTaskFilesDir(workDir string) error {
	tasksDir := filepath.Join(workDir, taskfile.TasksDir)
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return fmt.Errorf("initialize state: create %s: %w", tasksDir, err)
	}
	return nil
}

func seedDefaultRunProfiles(workDir string) error {
	profilesDir := filepath.Join(workDir, ".agent", "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		return fmt.Errorf("initialize state: create %s: %w", profilesDir, err)
	}
	for _, template := range prompt.DefaultRunProfileTemplates() {
		path := filepath.Join(workDir, prompt.RunProfileSourcePath(template.Name))
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				return fmt.Errorf("initialize state: profile path %s is a directory", path)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("initialize state: inspect profile %s: %w", path, err)
		}

		content := strings.TrimRight(template.Content, "\n") + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("initialize state: write profile %s: %w", path, err)
		}
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
	return pathsInitialized(paths, paths.LedgerDBPath)
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
