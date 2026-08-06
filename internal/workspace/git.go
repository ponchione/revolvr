package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"revolvr/internal/runner"
	"revolvr/internal/storage/postgres"
)

var gitConfigPrefix = []string{
	"-c", "core.hooksPath=/dev/null",
	"-c", "protocol.file.allow=never",
	"-c", "credential.helper=",
}

type branchEffect struct {
	BranchRef string `json:"branch_ref"`
	Commit    string `json:"commit"`
	Tree      string `json:"tree"`
}

type worktreeEffect struct {
	Path      string `json:"path"`
	BranchRef string `json:"branch_ref"`
	Commit    string `json:"commit"`
	Tree      string `json:"tree"`
	Device    uint64 `json:"device"`
	Inode     uint64 `json:"inode"`
}

func (m *Manager) ensureBranch(ctx context.Context, workspace postgres.CoreWorkspace, operationID string) error {
	material, _ := stableHash(branchEffect{BranchRef: workspace.BranchRef, Commit: workspace.SourceCommit, Tree: workspace.SourceTree})
	operation, existed, err := m.ensureOperation(ctx, workspace, operationID, "branch_create", material)
	if err != nil {
		return err
	}
	oid, found, err := m.refOID(ctx, workspace.ManagedRepositoryPath, workspace.BranchRef)
	if err != nil {
		return err
	}
	if !existed && found {
		return &ConflictError{Effect: "branch creation", Detail: "branch existed before its operation was admitted"}
	}
	if operation.Status == "applied" && !found {
		return &ConflictError{Effect: "branch creation", Detail: "applied branch is missing"}
	}
	if found && oid != workspace.SourceCommit {
		return &ConflictError{Effect: "branch creation", Detail: "branch points at a divergent commit"}
	}
	if !found {
		zero := strings.Repeat("0", len(workspace.SourceCommit))
		if _, err := m.git(ctx, workspace.ManagedRepositoryPath, "update-ref", workspace.BranchRef, workspace.SourceCommit, zero); err != nil {
			return err
		}
	}
	effect, err := m.verifyBranch(ctx, workspace)
	if err != nil {
		return err
	}
	if err := m.inject(FailureAfterBranch); err != nil {
		return err
	}
	_, err = m.completeOperation(ctx, operation, effect)
	return err
}

func (m *Manager) verifyBranch(ctx context.Context, workspace postgres.CoreWorkspace) (branchEffect, error) {
	oid, found, err := m.refOID(ctx, workspace.ManagedRepositoryPath, workspace.BranchRef)
	if err != nil || !found || oid != workspace.SourceCommit {
		return branchEffect{}, errors.Join(err, &ConflictError{Effect: "branch creation", Detail: "branch identity is missing or divergent"})
	}
	tree, err := m.git(ctx, workspace.ManagedRepositoryPath, "rev-parse", "--verify", oid+"^{tree}")
	if err != nil || strings.TrimSpace(tree) != workspace.SourceTree {
		return branchEffect{}, errors.Join(err, &ConflictError{Effect: "branch creation", Detail: "branch tree differs from scheduler authority"})
	}
	return branchEffect{BranchRef: workspace.BranchRef, Commit: oid, Tree: strings.TrimSpace(tree)}, nil
}

func (m *Manager) ensureWorktree(ctx context.Context, workspace postgres.CoreWorkspace, operationID string) (uint64, uint64, error) {
	requested := struct {
		Path, BranchRef, Commit, Tree string
	}{workspace.WorkspacePath, workspace.BranchRef, workspace.SourceCommit, workspace.SourceTree}
	material, _ := stableHash(requested)
	operation, existed, err := m.ensureOperation(ctx, workspace, operationID, "worktree_create", material)
	if err != nil {
		return 0, 0, err
	}
	registrations, err := m.worktreeRegistrations(ctx, workspace.ManagedRepositoryPath)
	if err != nil {
		return 0, 0, err
	}
	registration, exact, collision := findRegistration(registrations, workspace.WorkspacePath, workspace.BranchRef)
	pathInfo, pathErr := os.Lstat(workspace.WorkspacePath)
	pathExists := pathErr == nil
	if pathErr != nil && !errors.Is(pathErr, os.ErrNotExist) {
		return 0, 0, pathErr
	}
	if !existed && (pathExists || exact || collision) {
		return 0, 0, &ConflictError{Effect: "worktree creation", Detail: "path or Git registration existed before its operation was admitted"}
	}
	if collision {
		return 0, 0, &ConflictError{Effect: "worktree creation", Detail: "path or branch is registered with divergent authority"}
	}
	if operation.Status == "applied" && (!exact || !pathExists) {
		return 0, 0, &ConflictError{Effect: "worktree creation", Detail: "applied worktree effect is missing"}
	}
	if pathExists && (!pathInfo.IsDir() || pathInfo.Mode()&os.ModeSymlink != 0 || !exact) {
		return 0, 0, &ConflictError{Effect: "worktree creation", Detail: "workspace path is not the exact registered directory"}
	}
	if exact && registration.Head != workspace.SourceCommit {
		return 0, 0, &ConflictError{Effect: "worktree creation", Detail: "registered worktree HEAD is divergent"}
	}
	if !exact {
		if operation.Status == "applied" {
			return 0, 0, &ConflictError{Effect: "worktree creation", Detail: "applied registration disappeared"}
		}
		shortBranch := strings.TrimPrefix(workspace.BranchRef, "refs/heads/")
		if _, err := m.git(ctx, workspace.ManagedRepositoryPath, "worktree", "add", "--", workspace.WorkspacePath, shortBranch); err != nil {
			return 0, 0, err
		}
	}
	if operation.Status == "planned" {
		if err := hardenWorktreeMode(workspace.WorkspacePath); err != nil {
			return 0, 0, err
		}
	}
	effect, err := m.verifyWorktree(ctx, workspace, workspace.SourceCommit, workspace.SourceTree)
	if err != nil {
		return 0, 0, err
	}
	if operation.Status == "applied" {
		var recorded worktreeEffect
		if err := json.Unmarshal(operation.Effect, &recorded); err != nil || recorded != effect {
			return 0, 0, errors.Join(err, &ConflictError{Effect: "worktree creation", Detail: "filesystem identity differs from applied effect"})
		}
	}
	if err := m.inject(FailureAfterWorktree); err != nil {
		return 0, 0, err
	}
	if _, err := m.completeOperation(ctx, operation, effect); err != nil {
		return 0, 0, err
	}
	return effect.Device, effect.Inode, nil
}

func hardenWorktreeMode(path string) error {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return errors.Join(err, &ConflictError{Effect: "worktree creation", Detail: "Git-created path is not a real directory"})
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("%w: open Git-created worktree: %v", ErrUnsafePath, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("%w: open Git-created worktree descriptor", ErrUnsafePath)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return errors.Join(err, &ConflictError{Effect: "worktree creation", Detail: "path changed while opening"})
	}
	if err := unix.Fchmod(fd, 0o750); err != nil {
		return err
	}
	after, err := os.Lstat(path)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() || !os.SameFile(opened, after) {
		return errors.Join(err, &ConflictError{Effect: "worktree creation", Detail: "path changed while hardening permissions"})
	}
	return nil
}

func (m *Manager) verifyWorktree(ctx context.Context, workspace postgres.CoreWorkspace, expectedCommit, expectedTree string) (worktreeEffect, error) {
	registrations, err := m.worktreeRegistrations(ctx, workspace.ManagedRepositoryPath)
	if err != nil {
		return worktreeEffect{}, err
	}
	registration, exact, collision := findRegistration(registrations, workspace.WorkspacePath, workspace.BranchRef)
	if !exact || collision || registration.Head != expectedCommit {
		return worktreeEffect{}, &ConflictError{Effect: "worktree", Detail: "exact Git registration is missing or divergent"}
	}
	directory, found, err := m.workspaceRoot.OpenDir(workspace.WorkspacePath, false)
	if err != nil || !found {
		return worktreeEffect{}, errors.Join(err, &ConflictError{Effect: "worktree", Detail: "workspace path is not a protected directory"})
	}
	device, inode, identityErr := directory.Identity()
	closeErr := directory.Close()
	if identityErr != nil || closeErr != nil {
		return worktreeEffect{}, errors.Join(identityErr, closeErr)
	}
	head, err := m.git(ctx, workspace.WorkspacePath, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != expectedCommit {
		return worktreeEffect{}, errors.Join(err, &ConflictError{Effect: "worktree", Detail: "HEAD differs from expected commit"})
	}
	branch, err := m.git(ctx, workspace.WorkspacePath, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || strings.TrimSpace(branch) != workspace.BranchRef {
		return worktreeEffect{}, errors.Join(err, &ConflictError{Effect: "worktree", Detail: "symbolic branch differs from admitted ref"})
	}
	tree, err := m.git(ctx, workspace.WorkspacePath, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil || strings.TrimSpace(tree) != expectedTree {
		return worktreeEffect{}, errors.Join(err, &ConflictError{Effect: "worktree", Detail: "tree differs from expected tree"})
	}
	return worktreeEffect{
		Path: workspace.WorkspacePath, BranchRef: workspace.BranchRef,
		Commit: expectedCommit, Tree: expectedTree, Device: device, Inode: inode,
	}, nil
}

func (m *Manager) revalidateWorkspace(ctx context.Context, workspace postgres.CoreWorkspace) error {
	expectedCommit, expectedTree := workspace.SourceCommit, workspace.SourceTree
	if workspace.CandidateCommit.Valid {
		expectedCommit, expectedTree = workspace.CandidateCommit.String, workspace.CandidateTree.String
	}
	effect, err := m.verifyWorktree(ctx, workspace, expectedCommit, expectedTree)
	if err != nil {
		return err
	}
	if workspace.WorkspaceDevice.Valid && (uint64(workspace.WorkspaceDevice.Int64) != effect.Device || uint64(workspace.WorkspaceInode.Int64) != effect.Inode) {
		return &ConflictError{Effect: "worktree", Detail: "filesystem device/inode identity changed"}
	}
	return nil
}

type worktreeRegistration struct {
	Path, Head, Branch string
}

func (m *Manager) worktreeRegistrations(ctx context.Context, repository string) ([]worktreeRegistration, error) {
	raw, err := m.git(ctx, repository, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(raw, "\x00")
	var result []worktreeRegistration
	var current *worktreeRegistration
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "worktree "):
			result = append(result, worktreeRegistration{Path: filepath.Clean(strings.TrimPrefix(field, "worktree "))})
			current = &result[len(result)-1]
		case current != nil && strings.HasPrefix(field, "HEAD "):
			current.Head = strings.TrimPrefix(field, "HEAD ")
		case current != nil && strings.HasPrefix(field, "branch "):
			current.Branch = strings.TrimPrefix(field, "branch ")
		}
	}
	return result, nil
}

func findRegistration(registrations []worktreeRegistration, path, branch string) (worktreeRegistration, bool, bool) {
	var exact worktreeRegistration
	found := false
	collision := false
	for _, registration := range registrations {
		pathMatches := filepath.Clean(registration.Path) == filepath.Clean(path)
		branchMatches := registration.Branch == branch
		if pathMatches && branchMatches {
			if found {
				collision = true
			}
			exact, found = registration, true
		} else if pathMatches || branchMatches {
			collision = true
		}
	}
	return exact, found, collision
}

func (m *Manager) refOID(ctx context.Context, repository, ref string) (string, bool, error) {
	result := m.runGit(ctx, repository, []string{"show-ref", "--verify", "--quiet", ref}, nil)
	if result.ExitCode == 1 && result.Err == nil && !result.TimedOut && result.StdoutTruncatedBytes == 0 && result.StderrTruncatedBytes == 0 {
		return "", false, nil
	}
	if err := gitResultError(result); err != nil {
		return "", false, err
	}
	oidRaw, err := m.git(ctx, repository, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", false, err
	}
	oid := strings.TrimSpace(oidRaw)
	if oid == "" || strings.ContainsAny(oid, "\r\n ") {
		return "", false, &ConflictError{Effect: "Git ref", Detail: "object id output is malformed"}
	}
	return oid, true, nil
}

func (m *Manager) git(ctx context.Context, directory string, args ...string) (string, error) {
	result := m.runGit(ctx, directory, args, nil)
	if err := gitResultError(result); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return result.Stdout, nil
}

func (m *Manager) runGit(ctx context.Context, directory string, args []string, stdin []byte) runner.Result {
	command := runner.Command{
		Name: m.config.GitExecutable, Args: append(append([]string(nil), gitConfigPrefix...), args...),
		Dir: directory, Env: m.gitEnvironment(), ReplaceEnv: true,
		Timeout: m.config.Timeout, StdoutLimit: m.config.StdoutCap, StderrLimit: m.config.StderrCap,
	}
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	return m.config.CommandRunner(ctx, command)
}

func (m *Manager) safeCommandRunner(ctx context.Context, command runner.Command) runner.Result {
	command.Args = append(append([]string(nil), gitConfigPrefix...), command.Args...)
	command.Env = m.gitEnvironment()
	command.ReplaceEnv = true
	return m.config.CommandRunner(ctx, command)
}

func (m *Manager) gitEnvironment() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "HOME=/nonexistent",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0",
		"GIT_AUTHOR_NAME=Revolvr", "GIT_AUTHOR_EMAIL=revolvr@invalid.local",
		"GIT_COMMITTER_NAME=Revolvr", "GIT_COMMITTER_EMAIL=revolvr@invalid.local",
	}
}

func gitResultError(result runner.Result) error {
	if result.Err == nil && !result.TimedOut && result.ExitCode == 0 && result.StdoutTruncatedBytes == 0 && result.StderrTruncatedBytes == 0 {
		return nil
	}
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = fmt.Sprintf("exit code %d", result.ExitCode)
	}
	if result.Err != nil {
		detail = result.Err.Error()
	}
	if result.TimedOut {
		detail = "command timed out: " + detail
	}
	if result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
		detail = "command output exceeded the configured limit"
	}
	return errors.New(detail)
}

func (m *Manager) inject(point FailurePoint) error {
	if m.config.FailureInjector == nil {
		return nil
	}
	if err := m.config.FailureInjector(point); err != nil {
		return errors.Join(ErrInjectedCrash, err)
	}
	return nil
}
