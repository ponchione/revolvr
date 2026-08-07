package evaluation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type preparedSource struct {
	root             string
	originalCheckout string
	workspace        string
	baselineCommit   string
	baselineTree     string
	originalBefore   string
}

func prepareFixtureSource(ctx context.Context, repositoryRoot, workRoot string, request ExecutionRequest) (*preparedSource, error) {
	root := filepath.Join(workRoot, request.Scenario.ID)
	if err := os.Mkdir(root, 0o755); err != nil {
		return nil, err
	}
	original := filepath.Join(root, "operator-checkout")
	workspace := filepath.Join(root, "managed-workspace")
	fixture := filepath.Join(repositoryRoot, filepath.FromSlash(request.Authority.Source.FixturePath))
	if err := copyFixture(fixture, original); err != nil {
		return nil, err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, errors.New("evaluation: git executable is required")
	}
	home := filepath.Join(root, "nonexistent-home")
	if _, err := runGit(ctx, git, home, original, "init", "--quiet", "--object-format=sha1"); err != nil {
		return nil, err
	}
	if _, err := runGit(ctx, git, home, original, "add", "--all", ":/"); err != nil {
		return nil, err
	}
	if _, err := runGit(ctx, git, home, original, "commit", "--quiet", "--message", "deterministic evaluation baseline"); err != nil {
		return nil, err
	}
	commit, err := runGit(ctx, git, home, original, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	tree, err := runGit(ctx, git, home, original, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return nil, err
	}
	before, err := checkoutIdentity(ctx, git, home, original)
	if err != nil {
		return nil, err
	}
	if _, err := runGit(ctx, git, home, root, "clone", "--quiet", "--no-local", original, workspace); err != nil {
		return nil, err
	}
	return &preparedSource{root: root, originalCheckout: original, workspace: workspace, baselineCommit: commit, baselineTree: tree, originalBefore: before}, nil
}

func (s *preparedSource) candidate(ctx context.Context, scenario Scenario) (string, error) {
	if !scenarioMutatesManagedSource(scenario.Behavior) {
		return "", nil
	}
	name := "result.txt"
	if scenario.Behavior == "mid_run_source_change" {
		name = "external-change.txt"
	}
	content := "scenario=" + scenario.ID + "\noutcome=" + scenario.ExpectedOutcome + "\n"
	if err := os.WriteFile(filepath.Join(s.workspace, name), []byte(content), 0o644); err != nil {
		return "", err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return "", err
	}
	home := filepath.Join(s.root, "nonexistent-home")
	if _, err := runGit(ctx, git, home, s.workspace, "add", "--", name); err != nil {
		return "", err
	}
	if _, err := runGit(ctx, git, home, s.workspace, "commit", "--quiet", "--message", "deterministic evaluation candidate "+scenario.ID); err != nil {
		return "", err
	}
	return runGit(ctx, git, home, s.workspace, "rev-parse", "HEAD")
}

func (s *preparedSource) originalAfter(ctx context.Context) (string, bool, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", false, err
	}
	after, err := checkoutIdentity(ctx, git, filepath.Join(s.root, "nonexistent-home"), s.originalCheckout)
	return after, err == nil && after == s.originalBefore, err
}

func scenarioMutatesManagedSource(behavior string) bool {
	switch behavior {
	case "straight_success", "compile_correction", "test_correction", "audit_correction", "crash_state", "crash_external", "stale_index", "missing_embeddings", "mid_run_source_change":
		return true
	default:
		return false
	}
}

func copyFixture(source, target string) error {
	if err := os.Mkdir(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(relative), ".git/") || relative == ".git" {
			return errors.New("evaluation: fixture must not contain Git state")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.Mkdir(destination, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("evaluation: fixture entries must be regular files or directories")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, raw, info.Mode().Perm())
	})
}

func runGit(ctx context.Context, executable, home, directory string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = directory
	command.Env = []string{
		"HOME=" + home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"GIT_AUTHOR_NAME=Revolvr Evaluation",
		"GIT_AUTHOR_EMAIL=evaluation@invalid",
		"GIT_AUTHOR_DATE=2001-02-03T04:05:06Z",
		"GIT_COMMITTER_NAME=Revolvr Evaluation",
		"GIT_COMMITTER_EMAIL=evaluation@invalid",
		"GIT_COMMITTER_DATE=2001-02-03T04:05:06Z",
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=core.hooksPath",
		"GIT_CONFIG_VALUE_0=/dev/null",
		"GIT_CONFIG_KEY_1=protocol.file.allow",
		"GIT_CONFIG_VALUE_1=always",
		"GIT_CONFIG_KEY_2=core.autocrlf",
		"GIT_CONFIG_VALUE_2=false",
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("evaluation: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func checkoutIdentity(ctx context.Context, git, home, directory string) (string, error) {
	head, err := runGit(ctx, git, home, directory, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	tree, err := runGit(ctx, git, home, directory, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return "", err
	}
	status, err := runGit(ctx, git, home, directory, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	type material struct {
		Head   string `json:"head"`
		Tree   string `json:"tree"`
		Status string `json:"status"`
		Files  string `json:"files_sha256"`
	}
	files, err := workingFilesIdentity(directory)
	if err != nil {
		return "", err
	}
	return hashValue(material{Head: head, Tree: tree, Status: status, Files: files})
}

func workingFilesIdentity(root string) (string, error) {
	type entry struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	var entries []entry
	err := filepath.WalkDir(root, func(path string, value fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".git" {
			return filepath.SkipDir
		}
		if value.IsDir() {
			return nil
		}
		if value.Type()&os.ModeSymlink != 0 {
			return errors.New("evaluation: checkout contains symlink")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, entry{Path: relative, SHA256: hashBytes(raw)})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	raw, err := Canonical(entries)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}
