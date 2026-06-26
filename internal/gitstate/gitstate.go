package gitstate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/runner"
)

const (
	defaultGitExecutable = "git"
	defaultTimeout       = 30 * time.Second
)

type CommandRunner func(context.Context, runner.Command) runner.Result

type CaptureKind string

const (
	CaptureKindDirty   CaptureKind = "dirty"
	CaptureKindChanged CaptureKind = "changed"
)

type ChangeKind string

const (
	KindModified  ChangeKind = "modified"
	KindAdded     ChangeKind = "added"
	KindDeleted   ChangeKind = "deleted"
	KindRenamed   ChangeKind = "renamed"
	KindCopied    ChangeKind = "copied"
	KindUntracked ChangeKind = "untracked"
	KindOther     ChangeKind = "other"
)

type Config struct {
	WorkingDir    string
	GitExecutable string
	Timeout       time.Duration
	StdoutCap     int
	StderrCap     int
	CommandRunner CommandRunner
}

type Entry struct {
	Status  string
	Kind    ChangeKind
	Path    string
	OldPath string
}

type Capture struct {
	Kind                 CaptureKind
	DirtyFiles           []string
	ChangedFiles         []string
	Paths                []string
	Entries              []Entry
	RawStatus            string
	ExitCode             int
	TimedOut             bool
	Stderr               string
	StdoutTruncatedBytes int64
	StderrTruncatedBytes int64
	CaptureError         string
}

func CaptureDirtyWorktree(ctx context.Context, cfg Config) (Capture, error) {
	capture, err := captureStatus(ctx, cfg, CaptureKindDirty)
	if err != nil {
		return Capture{}, err
	}
	capture.DirtyFiles = cloneStrings(capture.Paths)
	return capture, nil
}

func CaptureChangedFiles(ctx context.Context, cfg Config) (Capture, error) {
	capture, err := captureStatus(ctx, cfg, CaptureKindChanged)
	if err != nil {
		return Capture{}, err
	}
	capture.ChangedFiles = cloneStrings(capture.Paths)
	return capture, nil
}

func ParseShortStatus(status string) []Entry {
	entries := []Entry{}
	for _, line := range strings.Split(status, "\n") {
		entry, ok := parseShortStatusLine(line)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func PathsFromEntries(entries []Entry) []string {
	seen := map[string]struct{}{}
	add := func(path string) {
		if path == "" {
			return
		}
		seen[path] = struct{}{}
	}

	for _, entry := range entries {
		if entry.OldPath != "" && (entry.Kind == KindRenamed || entry.Kind == KindCopied) {
			add(entry.OldPath)
		}
		add(entry.Path)
	}

	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func captureStatus(ctx context.Context, cfg Config, kind CaptureKind) (Capture, error) {
	cfg, workDir, err := normalizeConfig(cfg)
	if err != nil {
		return Capture{}, err
	}

	result := cfg.CommandRunner(ctx, runner.Command{
		Name:        cfg.GitExecutable,
		Args:        []string{"status", "--short", "--untracked-files=all"},
		Dir:         workDir,
		Timeout:     cfg.Timeout,
		StdoutLimit: cfg.StdoutCap,
		StderrLimit: cfg.StderrCap,
	})

	entries := ParseShortStatus(result.Stdout)
	capture := Capture{
		Kind:                 kind,
		Paths:                PathsFromEntries(entries),
		Entries:              entries,
		RawStatus:            result.Stdout,
		ExitCode:             result.ExitCode,
		TimedOut:             result.TimedOut,
		Stderr:               result.Stderr,
		StdoutTruncatedBytes: result.StdoutTruncatedBytes,
		StderrTruncatedBytes: result.StderrTruncatedBytes,
	}
	capture.CaptureError = captureError(result)
	return capture, nil
}

func normalizeConfig(cfg Config) (Config, string, error) {
	cfg.WorkingDir = strings.TrimSpace(cfg.WorkingDir)
	if cfg.WorkingDir == "" {
		return Config{}, "", errors.New("capture git state: working directory is required")
	}
	cfg.GitExecutable = strings.TrimSpace(cfg.GitExecutable)
	if cfg.GitExecutable == "" {
		cfg.GitExecutable = defaultGitExecutable
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}

	workDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve working directory: %w", err)
	}
	return cfg, workDir, nil
}

func captureError(result runner.Result) string {
	if result.Err != nil {
		if result.ExitCode != 0 {
			return fmt.Sprintf("%v (exit code %d)", result.Err, result.ExitCode)
		}
		return result.Err.Error()
	}
	if result.ExitCode != 0 {
		return fmt.Sprintf("git status exited with code %d", result.ExitCode)
	}
	return ""
}

func parseShortStatusLine(line string) (Entry, bool) {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "## ") {
		return Entry{}, false
	}
	if len(line) < 3 {
		return Entry{}, false
	}

	status := line[:2]
	pathPart := strings.TrimSpace(line[2:])
	if pathPart == "" {
		return Entry{}, false
	}

	kind := classifyStatus(status)
	entry := Entry{
		Status: status,
		Kind:   kind,
	}

	if kind == KindRenamed || kind == KindCopied {
		oldPath, newPath, ok := splitRenamePaths(pathPart)
		if ok {
			entry.OldPath = parseStatusPath(oldPath)
			entry.Path = parseStatusPath(newPath)
			return entry, entry.OldPath != "" || entry.Path != ""
		}
	}

	entry.Path = parseStatusPath(pathPart)
	return entry, entry.Path != ""
}

func classifyStatus(status string) ChangeKind {
	switch {
	case strings.Contains(status, "?"):
		return KindUntracked
	case strings.Contains(status, "R"):
		return KindRenamed
	case strings.Contains(status, "C"):
		return KindCopied
	case strings.Contains(status, "A"):
		return KindAdded
	case strings.Contains(status, "D"):
		return KindDeleted
	case strings.ContainsAny(status, "MT"):
		return KindModified
	default:
		return KindOther
	}
}

func splitRenamePaths(pathPart string) (string, string, bool) {
	inQuote := false
	escaped := false
	for i := 0; i < len(pathPart); i++ {
		switch pathPart[i] {
		case '\\':
			if inQuote {
				escaped = !escaped
				continue
			}
		case '"':
			if inQuote && escaped {
				escaped = false
				continue
			}
			inQuote = !inQuote
		default:
			escaped = false
		}

		if !inQuote && strings.HasPrefix(pathPart[i:], " -> ") {
			return strings.TrimSpace(pathPart[:i]), strings.TrimSpace(pathPart[i+4:]), true
		}
	}
	return "", "", false
}

func parseStatusPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(raw); err == nil {
		return unquoted
	}
	return raw
}

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}
