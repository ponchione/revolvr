package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/prompt"
	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
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
	canonicalWorkDir, err := runtimepath.CanonicalRoot(absWorkDir)
	if err != nil {
		return statePaths{}, fmt.Errorf("resolve working directory identity: %w", err)
	}

	stateDir := filepath.Join(canonicalWorkDir, revolvrStateDir)
	return statePaths{
		WorkDir:      canonicalWorkDir,
		StateDir:     stateDir,
		LedgerDBPath: filepath.Join(stateDir, "ledger.sqlite"),
		RunsDir:      filepath.Join(stateDir, "runs"),
		ReceiptsDir:  filepath.Join(stateDir, "receipts"),
		LocksDir:     filepath.Join(stateDir, "locks"),
	}, nil
}

func initializeState(ctx context.Context, paths statePaths) error {
	root := paths.WorkDir
	if err := preflightStateInitialization(root, paths); err != nil {
		return err
	}
	gitExclude, err := resolveGitExcludeTarget(ctx, root)
	if err != nil {
		return fmt.Errorf("initialize state: resolve git exclude: %w", err)
	}

	for _, dir := range []string{paths.StateDir, paths.RunsDir, paths.ReceiptsDir, paths.LocksDir} {
		if err := runtimepath.EnsureDir(root, dir, 0o755); err != nil {
			return fmt.Errorf("initialize state: create %s: %w", dir, err)
		}
	}
	if err := seedDefaultRunProfiles(root); err != nil {
		return err
	}
	if err := ensureTaskFilesDir(root); err != nil {
		return err
	}
	if err := initializeProtectedLedger(ctx, root, paths.LedgerDBPath); err != nil {
		return err
	}
	if err := ensureStateIgnoredByGit(gitExclude); err != nil {
		return err
	}
	return nil
}

func preflightStateInitialization(root string, paths statePaths) error {
	dirs := []string{
		paths.StateDir,
		paths.RunsDir,
		paths.ReceiptsDir,
		paths.LocksDir,
		filepath.Join(root, ".agent"),
		filepath.Join(root, ".agent", "profiles"),
		filepath.Join(root, taskfile.TasksDir),
	}
	for _, dir := range dirs {
		if err := runtimepath.CheckDir(root, dir, true); err != nil {
			return fmt.Errorf("initialize state: inspect directory %s: %w", dir, err)
		}
	}
	files := []string{paths.LedgerDBPath}
	for _, template := range prompt.DefaultRunProfileTemplates() {
		files = append(files, filepath.Join(root, prompt.RunProfileSourcePath(template.Name)))
	}
	for _, path := range files {
		if err := runtimepath.CheckFile(root, path, true); err != nil {
			return fmt.Errorf("initialize state: inspect file %s: %w", path, err)
		}
	}
	return nil
}

func ensureTaskFilesDir(workDir string) error {
	tasksDir := filepath.Join(workDir, taskfile.TasksDir)
	if err := runtimepath.EnsureDir(workDir, tasksDir, 0o755); err != nil {
		return fmt.Errorf("initialize state: create %s: %w", tasksDir, err)
	}
	return nil
}

func seedDefaultRunProfiles(workDir string) error {
	profilesDir := filepath.Join(workDir, ".agent", "profiles")
	if err := runtimepath.EnsureDir(workDir, profilesDir, 0o755); err != nil {
		return fmt.Errorf("initialize state: create %s: %w", profilesDir, err)
	}
	for _, template := range prompt.DefaultRunProfileTemplates() {
		path := filepath.Join(workDir, prompt.RunProfileSourcePath(template.Name))
		if err := runtimepath.CheckFile(workDir, path, true); err != nil {
			return fmt.Errorf("initialize state: inspect profile %s: %w", path, err)
		}
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("initialize state: inspect profile %s: %w", path, err)
		}

		content := strings.TrimRight(template.Content, "\n") + "\n"
		if err := createProtectedFile(workDir, path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("initialize state: write profile %s: %w", path, err)
		}
	}
	return nil
}

func createProtectedFile(root, path string, content []byte, mode os.FileMode) error {
	file, err := runtimepath.OpenFile(root, path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, path, file); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	return nil
}

func initializeProtectedLedger(ctx context.Context, root, path string) error {
	guard, err := runtimepath.OpenFile(root, path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("initialize state: protect ledger: %w", err)
	}
	defer guard.Close()
	runs, openErr := ledger.Open(ctx, path)
	identityErr := runtimepath.CheckOpenedFile(root, path, guard)
	if openErr != nil {
		return errors.Join(openErr, identityErr)
	}
	closeErr := runs.Close()
	finalIdentityErr := runtimepath.CheckOpenedFile(root, path, guard)
	if err := errors.Join(identityErr, closeErr, finalIdentityErr); err != nil {
		return fmt.Errorf("initialize state: close protected ledger: %w", err)
	}
	return nil
}

type gitExcludeTarget struct {
	Root string
	Path string
}

func ensureStateIgnoredByGit(target *gitExcludeTarget) error {
	if target == nil {
		return nil
	}
	if err := ensureExcludePattern(target.Root, target.Path, revolvrGitExcludePattern, nil); err != nil {
		return fmt.Errorf("initialize state: update git exclude: %w", err)
	}
	return nil
}

func resolveGitExcludeTarget(ctx context.Context, workDir string) (*gitExcludeTarget, error) {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Lstat(gitPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", gitPath, err)
	}
	if info.IsDir() {
		if err := runtimepath.CheckDir(workDir, gitPath, false); err != nil {
			return nil, err
		}
	} else if err := runtimepath.CheckFile(workDir, gitPath, false); err != nil {
		return nil, err
	}

	result := runner.Run(ctx, runner.Command{
		Name:        "git",
		Args:        []string{"rev-parse", "--path-format=absolute", "--show-toplevel", "--absolute-git-dir", "--git-common-dir", "--git-path", "info/exclude"},
		Dir:         workDir,
		Timeout:     10 * time.Second,
		StdoutLimit: 16 * 1024,
		StderrLimit: 16 * 1024,
	})
	if result.Err != nil {
		return nil, fmt.Errorf("inspect Git administrative paths: %w", result.Err)
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = fmt.Sprintf("git exited %d", result.ExitCode)
		}
		return nil, fmt.Errorf("inspect Git administrative paths: %s", detail)
	}
	lines := strings.Split(strings.TrimSuffix(result.Stdout, "\n"), "\n")
	if len(lines) != 4 {
		return nil, fmt.Errorf("inspect Git administrative paths: got %d paths, want 4", len(lines))
	}
	reportedWorktree, err := runtimepath.CanonicalRoot(lines[0])
	if err != nil {
		return nil, fmt.Errorf("resolve Git worktree: %w", err)
	}
	if same, err := sameDirectory(workDir, reportedWorktree); err != nil || !same {
		return nil, errors.Join(err, errors.New("Git worktree identity does not match the requested repository"))
	}
	gitDir, err := runtimepath.CanonicalRoot(lines[1])
	if err != nil {
		return nil, fmt.Errorf("resolve Git directory: %w", err)
	}
	commonDir, err := runtimepath.CanonicalRoot(lines[2])
	if err != nil {
		return nil, fmt.Errorf("resolve Git common directory: %w", err)
	}
	if err := validateProtectedRootDirectory(commonDir, "Git common directory"); err != nil {
		return nil, err
	}
	if !pathWithinDirectory(commonDir, gitDir) {
		return nil, errors.New("Git directory is outside the reported common directory")
	}
	if err := runtimepath.CheckDir(commonDir, gitDir, false); err != nil {
		return nil, fmt.Errorf("validate Git directory: %w", err)
	}
	if info.IsDir() {
		if same, err := sameDirectory(gitPath, gitDir); err != nil || !same {
			return nil, errors.Join(err, errors.New("repository .git directory does not match Git's reported directory"))
		}
	} else if err := validateLinkedWorktreeMarker(workDir, gitPath, gitDir, commonDir); err != nil {
		return nil, err
	}

	excludePath, err := filepath.Abs(strings.TrimSpace(lines[3]))
	if err != nil {
		return nil, fmt.Errorf("resolve Git exclude path: %w", err)
	}
	wantExcludePath := filepath.Join(commonDir, "info", "exclude")
	if filepath.Clean(excludePath) != wantExcludePath {
		return nil, fmt.Errorf("Git exclude path %s does not match common-directory exclude %s", excludePath, wantExcludePath)
	}
	if err := runtimepath.CheckDir(commonDir, filepath.Dir(wantExcludePath), true); err != nil {
		return nil, fmt.Errorf("validate Git exclude directory: %w", err)
	}
	if err := runtimepath.CheckFile(commonDir, wantExcludePath, true); err != nil {
		return nil, fmt.Errorf("validate Git exclude file: %w", err)
	}
	return &gitExcludeTarget{Root: commonDir, Path: wantExcludePath}, nil
}

func validateLinkedWorktreeMarker(workDir, gitPath, gitDir, commonDir string) error {
	raw, _, err := runtimepath.ReadFile(workDir, gitPath, false)
	if err != nil {
		return fmt.Errorf("read worktree .git file: %w", err)
	}
	firstLine := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
	const prefix = "gitdir:"
	if !strings.HasPrefix(firstLine, prefix) {
		return fmt.Errorf("%s is not a linked-worktree gitdir file", gitPath)
	}
	pointer := strings.TrimSpace(strings.TrimPrefix(firstLine, prefix))
	if pointer == "" {
		return fmt.Errorf("%s has an empty gitdir", gitPath)
	}
	if !filepath.IsAbs(pointer) {
		pointer = filepath.Join(workDir, pointer)
	}
	pointerDir, err := runtimepath.CanonicalRoot(pointer)
	if err != nil {
		return fmt.Errorf("resolve worktree gitdir pointer: %w", err)
	}
	if same, err := sameDirectory(pointerDir, gitDir); err != nil || !same {
		return errors.Join(err, errors.New("worktree .git pointer does not match Git's reported directory"))
	}
	if same, err := sameDirectory(gitDir, commonDir); err != nil {
		return err
	} else if same {
		return errors.New("external non-worktree Git directories are not accepted")
	}
	rel, err := filepath.Rel(commonDir, gitDir)
	if err != nil {
		return err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 2 || parts[0] != "worktrees" || parts[1] == "" {
		return errors.New("Git directory is not a canonical linked-worktree administration directory")
	}
	backlinkPath := filepath.Join(gitDir, "gitdir")
	backlink, _, err := runtimepath.ReadFile(commonDir, backlinkPath, false)
	if err != nil {
		return fmt.Errorf("read linked-worktree backlink: %w", err)
	}
	backlinkTarget := strings.TrimSpace(string(backlink))
	if !filepath.IsAbs(backlinkTarget) {
		backlinkTarget = filepath.Join(gitDir, backlinkTarget)
	}
	backlinkInfo, err := os.Lstat(filepath.Clean(backlinkTarget))
	if err != nil {
		return fmt.Errorf("inspect linked-worktree backlink target: %w", err)
	}
	markerInfo, err := os.Lstat(gitPath)
	if err != nil {
		return fmt.Errorf("inspect worktree .git file: %w", err)
	}
	if !os.SameFile(backlinkInfo, markerInfo) {
		return errors.New("linked-worktree backlink does not identify the worktree .git file")
	}
	return nil
}

func sameDirectory(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, err
	}
	return leftInfo.IsDir() && rightInfo.IsDir() && os.SameFile(leftInfo, rightInfo), nil
}

func validateProtectedRootDirectory(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s is not a directory", runtimepath.ErrUnsafe, label)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: %s has unsafe directory mode %04o", runtimepath.ErrUnsafe, label, info.Mode().Perm())
	}
	return nil
}

func pathWithinDirectory(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && (rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func ensureExcludePattern(root, path string, pattern string, afterOpen func()) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	if err := runtimepath.EnsureDir(root, filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := runtimepath.OpenFile(root, path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if afterOpen != nil {
		afterOpen()
	}
	if err := runtimepath.CheckOpenedFile(root, path, file); err != nil {
		return err
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, path, file); err != nil {
		return err
	}
	if excludeContentHasPattern(string(content), pattern) {
		return file.Close()
	}
	suffix := ""
	if len(content) != 0 && content[len(content)-1] != '\n' {
		suffix = "\n"
	}
	suffix += pattern + "\n"
	if _, err := file.WriteString(suffix); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, path, file); err != nil {
		return err
	}
	return file.Close()
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
