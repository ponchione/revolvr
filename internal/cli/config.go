package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/app"
	"revolvr/internal/verification"
)

const (
	defaultCodexExecutable                = app.DefaultCodexExecutable
	defaultCodexBypassApprovalsAndSandbox = app.DefaultCodexBypassApprovalsAndSandbox
	defaultGitExecutable                  = app.DefaultGitExecutable
	defaultGitTimeout                     = app.DefaultGitTimeout
	defaultCommitTimeout                  = app.DefaultCommitTimeout
)

type configCheckResult = app.RunConfigCheckResult

func checkRunConfig(workDir string) (configCheckResult, error) {
	return app.CheckRunConfig(workDir)
}

func writeConfigCheck(out io.Writer, result configCheckResult) error {
	cfg := result.Effective
	defaults := "merged"
	if !result.Found {
		defaults = "used"
	}
	lines := []string{
		fmt.Sprintf("Config path: %s", result.Path),
		fmt.Sprintf("Config found: %t", result.Found),
		fmt.Sprintf("Defaults: %s", defaults),
		fmt.Sprintf("Codex executable: %s", effectiveString(cfg.CodexExecutable, defaultCodexExecutable)),
		fmt.Sprintf("Codex dangerously bypass approvals and sandbox: %t", cfg.CodexBypassApprovalsAndSandbox),
		fmt.Sprintf("Codex sandbox: %s", cfg.CodexSandbox),
		fmt.Sprintf("Codex approval policy: %s", cfg.CodexApprovalPolicy),
		fmt.Sprintf("Codex timeout: %s", cfg.CodexTimeout),
		fmt.Sprintf("Git executable: %s", effectiveString(cfg.GitExecutable, defaultGitExecutable)),
		fmt.Sprintf("Git timeout: %s", effectiveDuration(cfg.GitTimeout, defaultGitTimeout)),
		fmt.Sprintf("Verification missing policy: %s", cfg.MissingVerificationPolicy),
		fmt.Sprintf("Verification command count: %d", len(cfg.VerificationCommands)),
	}
	for i, command := range cfg.VerificationCommands {
		lines = append(lines, formatVerificationCommand(i, command))
	}
	lines = append(lines,
		fmt.Sprintf("Commit allow pre-existing dirty: %t", cfg.AllowPreExistingDirty),
		fmt.Sprintf("Commit allow missing verification: %t", cfg.AllowMissingVerification),
		fmt.Sprintf("Commit timeout: %s", effectiveDuration(cfg.CommitTimeout, defaultCommitTimeout)),
		fmt.Sprintf("Output caps bytes: codex_stdout=%d codex_stderr=%d git_stdout=%d git_stderr=%d verification_stdout=%d verification_stderr=%d commit_stdout=%d commit_stderr=%d",
			cfg.CodexStdoutCap,
			cfg.CodexStderrCap,
			cfg.GitStdoutCap,
			cfg.GitStderrCap,
			cfg.VerificationStdoutCap,
			cfg.VerificationStderrCap,
			cfg.CommitStdoutCap,
			cfg.CommitStderrCap,
		),
	)
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func formatVerificationCommand(index int, command verification.Command) string {
	parts := []string{
		fmt.Sprintf("Verification command %d: name=%s", index, command.Name),
		fmt.Sprintf("args=%s", formatVerificationArgs(command.Args)),
	}
	if dir := strings.TrimSpace(command.Dir); dir != "" {
		parts = append(parts, fmt.Sprintf("dir=%s", dir))
	}
	if command.Timeout > 0 {
		parts = append(parts, fmt.Sprintf("timeout=%s", command.Timeout))
	}
	return strings.Join(parts, " ")
}

func formatVerificationArgs(args []string) string {
	if len(args) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func effectiveString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func effectiveDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
