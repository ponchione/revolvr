package codexexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"revolvr/internal/runner"
)

const (
	DefaultVersionTimeout   = 10 * time.Second
	DefaultVersionOutputCap = 16 * 1024
)

type VersionConfig struct {
	Executable    string
	WorkingDir    string
	Timeout       time.Duration
	StdoutCap     int
	StderrCap     int
	CommandRunner CommandRunner
}

func DiscoverVersion(ctx context.Context, cfg VersionConfig) (string, error) {
	executable := strings.TrimSpace(cfg.Executable)
	if executable == "" {
		executable = DefaultExecutable
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultVersionTimeout
	}
	if cfg.StdoutCap <= 0 {
		cfg.StdoutCap = DefaultVersionOutputCap
	}
	if cfg.StderrCap <= 0 {
		cfg.StderrCap = DefaultVersionOutputCap
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}

	result := cfg.CommandRunner(ctx, runner.Command{
		Name:        executable,
		Args:        []string{"--version"},
		Dir:         strings.TrimSpace(cfg.WorkingDir),
		Timeout:     cfg.Timeout,
		StdoutLimit: cfg.StdoutCap,
		StderrLimit: cfg.StderrCap,
	})
	switch {
	case result.TimedOut:
		return "", fmt.Errorf("discover Codex version with %q: command timed out after %s", executable, cfg.Timeout)
	case result.Err != nil:
		if errors.Is(result.Err, context.DeadlineExceeded) {
			return "", fmt.Errorf("discover Codex version with %q: command timed out after %s", executable, cfg.Timeout)
		}
		return "", fmt.Errorf("discover Codex version with %q: execution failed: %w", executable, result.Err)
	case result.ExitCode != 0:
		detail := firstNonemptyLine(result.Stderr, result.Stdout)
		if detail == "" {
			return "", fmt.Errorf("discover Codex version with %q: command exited with code %d", executable, result.ExitCode)
		}
		return "", fmt.Errorf("discover Codex version with %q: command exited with code %d: %s", executable, result.ExitCode, detail)
	case result.StdoutTruncatedBytes > 0 || result.StderrTruncatedBytes > 0:
		return "", fmt.Errorf("discover Codex version with %q: output was truncated (stdout=%d bytes, stderr=%d bytes)", executable, result.StdoutTruncatedBytes, result.StderrTruncatedBytes)
	}

	version := strings.TrimSpace(result.Stdout)
	if version == "" {
		return "", fmt.Errorf("discover Codex version with %q: version output is empty", executable)
	}
	if strings.ContainsAny(version, "\r\n") || strings.IndexFunc(version, func(r rune) bool {
		return unicode.IsControl(r) && r != '\t'
	}) >= 0 {
		return "", fmt.Errorf("discover Codex version with %q: version output must be one well-formed line", executable)
	}
	return version, nil
}

func firstNonemptyLine(values ...string) string {
	for _, value := range values {
		for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return line
			}
		}
	}
	return ""
}
