package gitstate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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
	KindIgnored   ChangeKind = "ignored"
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

// ParsePorcelainV1Z parses `git status --porcelain=v1 -z` output. Paths are
// retained as arbitrary non-NUL byte strings; in rename/copy records Git emits
// the destination first and the source in the following NUL-delimited field.
func ParsePorcelainV1Z(status string) ([]Entry, error) {
	if status == "" {
		return nil, nil
	}
	entries := make([]Entry, 0, strings.Count(status, "\x00"))
	for offset := 0; offset < len(status); {
		record, next, err := nextStatusField(status, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		if len(record) < 4 || record[2] != ' ' {
			return nil, errors.New("git status porcelain v1 -z: malformed status record")
		}
		statusCode := record[:2]
		if !validStatusCode(statusCode) {
			return nil, fmt.Errorf("git status porcelain v1 -z: invalid status code %q", statusCode)
		}
		entry := Entry{Status: statusCode, Kind: classifyStatus(statusCode), Path: record[3:]}
		if entry.Path == "" {
			return nil, errors.New("git status porcelain v1 -z: empty path")
		}
		if entry.Kind == KindRenamed || entry.Kind == KindCopied {
			oldPath, renameNext, err := nextStatusField(status, offset)
			if err != nil {
				return nil, errors.New("git status porcelain v1 -z: incomplete rename/copy record")
			}
			if oldPath == "" {
				return nil, errors.New("git status porcelain v1 -z: empty rename/copy source path")
			}
			entry.OldPath = oldPath
			offset = renameNext
		}
		entries = append(entries, entry)
	}
	return entries, nil
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
		Args:        []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		Dir:         workDir,
		Timeout:     cfg.Timeout,
		StdoutLimit: cfg.StdoutCap,
		StderrLimit: cfg.StderrCap,
	})

	capture := Capture{
		Kind:                 kind,
		RawStatus:            result.Stdout,
		ExitCode:             result.ExitCode,
		TimedOut:             result.TimedOut,
		Stderr:               result.Stderr,
		StdoutTruncatedBytes: result.StdoutTruncatedBytes,
		StderrTruncatedBytes: result.StderrTruncatedBytes,
	}
	capture.CaptureError = captureError(result)
	if capture.CaptureError != "" {
		return capture, nil
	}
	entries, err := ParsePorcelainV1Z(result.Stdout)
	if err != nil {
		capture.CaptureError = err.Error()
		return capture, nil
	}
	capture.Entries = entries
	capture.Paths = PathsFromEntries(entries)
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
	parts := make([]string, 0, 4)
	if result.Err != nil {
		parts = append(parts, result.Err.Error())
	}
	if result.TimedOut {
		parts = append(parts, "git status timed out")
	}
	if result.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("git status exited with code %d", result.ExitCode))
	}
	if result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
		parts = append(parts, fmt.Sprintf(
			"git status output was truncated (stdout=%d bytes, stderr=%d bytes)",
			result.StdoutTruncatedBytes,
			result.StderrTruncatedBytes,
		))
	}
	return strings.Join(parts, "; ")
}

func nextStatusField(status string, offset int) (string, int, error) {
	if offset >= len(status) {
		return "", offset, errors.New("git status porcelain v1 -z: missing NUL-delimited field")
	}
	relative := strings.IndexByte(status[offset:], 0)
	if relative < 0 {
		return "", offset, errors.New("git status porcelain v1 -z: unterminated field")
	}
	end := offset + relative
	return status[offset:end], end + 1, nil
}

func validStatusCode(status string) bool {
	if len(status) != 2 || status == "  " {
		return false
	}
	for _, code := range []byte(status) {
		if !strings.ContainsRune(" MTADRCU?!", rune(code)) {
			return false
		}
	}
	if strings.ContainsAny(status, "?!") {
		return status == "??" || status == "!!"
	}
	return true
}

func classifyStatus(status string) ChangeKind {
	switch {
	case strings.Contains(status, "?"):
		return KindUntracked
	case strings.Contains(status, "!"):
		return KindIgnored
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

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}
