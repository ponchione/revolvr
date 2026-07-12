package autonomousarchive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"revolvr/internal/runner"
)

type CommandRunner func(context.Context, runner.Command) runner.Result

type gitConfig struct {
	root       string
	executable string
	timeout    time.Duration
	runner     CommandRunner
}

type gitResult struct {
	stdout string
	stderr string
	exit   int
	err    error
	timed  bool
}

func (g gitConfig) run(ctx context.Context, args ...string) gitResult {
	result := g.runner(ctx, runner.Command{Name: g.executable, Args: append([]string(nil), args...), Dir: g.root, Timeout: g.timeout, StdoutLimit: 4 << 20, StderrLimit: 1 << 20})
	return gitResult{stdout: result.Stdout, stderr: result.Stderr, exit: result.ExitCode, err: result.Err, timed: result.TimedOut}
}

func (r gitResult) passed() bool { return r.err == nil && !r.timed && r.exit == 0 }

func (g gitConfig) head(ctx context.Context) (string, bool, error) {
	result := g.run(ctx, "rev-parse", "--verify", "--quiet", "HEAD")
	if result.passed() {
		sha := strings.TrimSpace(result.stdout)
		if !validOID(sha) {
			return "", false, errors.New("archive git: HEAD is not a valid object id")
		}
		return sha, true, nil
	}
	if result.err == nil && !result.timed && result.exit == 1 && strings.TrimSpace(result.stdout) == "" {
		return "", false, nil
	}
	return "", false, fmt.Errorf("archive git: resolve HEAD: %s", gitFailure(result))
}

type statusEntry struct {
	index    byte
	worktree byte
	path     string
}

func (g gitConfig) status(ctx context.Context) ([]statusEntry, error) {
	result := g.run(ctx, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if !result.passed() {
		return nil, fmt.Errorf("archive git: status: %s", gitFailure(result))
	}
	parts := strings.Split(result.stdout, "\x00")
	entries := make([]statusEntry, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		value := parts[i]
		if value == "" {
			continue
		}
		if len(value) < 4 || value[2] != ' ' {
			return nil, errors.New("archive git: malformed porcelain status")
		}
		entry := statusEntry{index: value[0], worktree: value[1], path: value[3:]}
		if entry.index == 'R' || entry.index == 'C' || entry.worktree == 'R' || entry.worktree == 'C' {
			i++
			if i >= len(parts) || parts[i] == "" {
				return nil, errors.New("archive git: malformed rename status")
			}
			return nil, errors.New("archive git: rename/copy status is not admissible")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func validateOperationStatus(entries []statusEntry, allowed []string, allowStaged bool) error {
	set := stringSet(allowed)
	for _, entry := range entries {
		if !set[entry.path] {
			return fmt.Errorf("archive git: unrelated dirty path %q", entry.path)
		}
		if !allowStaged && entry.index != ' ' && entry.index != '?' {
			return fmt.Errorf("archive git: pre-existing staged path %q", entry.path)
		}
		if entry.index == 'U' || entry.worktree == 'U' {
			return fmt.Errorf("archive git: conflicted path %q", entry.path)
		}
	}
	return nil
}

func (g gitConfig) stage(ctx context.Context, paths []string) error {
	args := append([]string{"add", "--"}, paths...)
	if result := g.run(ctx, args...); !result.passed() {
		return fmt.Errorf("archive git: stage exact paths: %s", gitFailure(result))
	}
	return nil
}

func (g gitConfig) stagedPaths(ctx context.Context) ([]string, error) {
	result := g.run(ctx, "diff", "--cached", "--no-renames", "--name-only", "-z")
	if !result.passed() {
		return nil, fmt.Errorf("archive git: inspect staged paths: %s", gitFailure(result))
	}
	paths := compactNUL(result.stdout)
	sort.Strings(paths)
	return paths, nil
}

func (g gitConfig) commit(ctx context.Context, subject, body string) gitResult {
	return g.run(ctx, "commit", "-m", subject, "-m", body)
}

func (g gitConfig) commitMessage(ctx context.Context, sha string) (string, error) {
	result := g.run(ctx, "show", "-s", "--format=%B", sha)
	if !result.passed() {
		return "", fmt.Errorf("archive git: read commit message: %s", gitFailure(result))
	}
	return result.stdout, nil
}

func (g gitConfig) commitPaths(ctx context.Context, sha string) ([]string, error) {
	result := g.run(ctx, "diff-tree", "--root", "--no-commit-id", "--name-only", "-r", "-z", sha)
	if !result.passed() {
		return nil, fmt.Errorf("archive git: inspect commit paths: %s", gitFailure(result))
	}
	paths := compactNUL(result.stdout)
	sort.Strings(paths)
	return paths, nil
}

func (g gitConfig) fileAt(ctx context.Context, sha, path string) ([]byte, bool, error) {
	result := g.run(ctx, "show", sha+":"+path)
	if result.passed() {
		return []byte(result.stdout), true, nil
	}
	if result.err == nil && !result.timed && result.exit != 0 {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("archive git: inspect %s:%s: %s", sha, path, gitFailure(result))
}

func verifyCommit(ctx context.Context, g gitConfig, sha string, expectedPaths []string, expectedFiles map[string][]byte, requiredTrailers []string) error {
	if !validOID(sha) {
		return errors.New("archive git: reconciled commit SHA is invalid")
	}
	message, err := g.commitMessage(ctx, sha)
	if err != nil {
		return err
	}
	for _, trailer := range requiredTrailers {
		if !strings.Contains(message, trailer+"\n") && !strings.HasSuffix(strings.TrimSpace(message), trailer) {
			return fmt.Errorf("archive git: commit is missing identity line %q", trailer)
		}
	}
	paths, err := g.commitPaths(ctx, sha)
	if err != nil {
		return err
	}
	want := append([]string(nil), expectedPaths...)
	sort.Strings(want)
	if !equalStrings(paths, want) {
		return fmt.Errorf("archive git: commit paths %v do not match exact operation paths %v", paths, want)
	}
	for path, bytes := range expectedFiles {
		got, found, err := g.fileAt(ctx, sha, path)
		if err != nil || !found {
			return errors.Join(err, fmt.Errorf("archive git: expected committed file %q is missing", path))
		}
		if string(got) != string(bytes) {
			return fmt.Errorf("archive git: committed file %q has different bytes", path)
		}
	}
	return nil
}

func reconcileCommit(ctx context.Context, g gitConfig, before string, beforeExists bool, command gitResult, expectedPaths []string, expectedFiles map[string][]byte, trailers []string) (string, bool, error) {
	after, exists, err := g.head(ctx)
	retried := false
	if err != nil {
		retried = true
		after, exists, err = g.head(ctx)
	}
	if err != nil {
		return "", retried, errors.New("archive git: commit outcome is indeterminate after two HEAD lookups")
	}
	if !exists || (beforeExists && after == before) {
		return "", retried, fmt.Errorf("archive git: commit did not advance HEAD: %s", gitFailure(command))
	}
	if err := verifyCommit(ctx, g, after, expectedPaths, expectedFiles, trailers); err != nil {
		return "", retried, err
	}
	return after, retried, nil
}

func compactNUL(value string) []string {
	parts := strings.Split(value, "\x00")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func gitFailure(result gitResult) string {
	parts := []string{}
	if result.err != nil {
		parts = append(parts, result.err.Error())
	}
	if result.timed {
		parts = append(parts, "timed out")
	}
	if result.exit != 0 {
		parts = append(parts, fmt.Sprintf("exit %d", result.exit))
	}
	if detail := strings.TrimSpace(result.stderr); detail != "" {
		parts = append(parts, detail)
	}
	if len(parts) == 0 {
		return "unknown failure"
	}
	return strings.Join(parts, ": ")
}
