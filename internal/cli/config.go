package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/app"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/verification"
)

const (
	defaultCodexExecutable                = app.DefaultCodexExecutable
	defaultCodexModel                     = app.DefaultCodexModel
	defaultCodexReasoningEffort           = app.DefaultCodexReasoningEffort
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
		fmt.Sprintf("Codex model: %s", effectiveString(cfg.CodexModel, defaultCodexModel)),
		fmt.Sprintf("Codex reasoning effort: %s", effectiveString(cfg.CodexReasoningEffort, defaultCodexReasoningEffort)),
		fmt.Sprintf("Codex session mode: ephemeral (ephemeral=%t)", cfg.CodexEphemeral),
		fmt.Sprintf("Codex dangerously bypass approvals and sandbox: %t", cfg.CodexBypassApprovalsAndSandbox),
		fmt.Sprintf("Codex sandbox: %s", cfg.CodexSandbox),
		fmt.Sprintf("Codex approval policy: %s", cfg.CodexApprovalPolicy),
		fmt.Sprintf("Codex timeout: %s", cfg.CodexTimeout),
		fmt.Sprintf("Effective config schema: %s", result.EffectiveConfigSchema),
		fmt.Sprintf("Effective config SHA-256: %s", result.EffectiveConfigSHA256),
		fmt.Sprintf("Autonomy safety schema: %s", cfg.SafetyDeclaration.SchemaVersion),
		fmt.Sprintf("Autonomy mode: %s", cfg.SafetyDeclaration.Mode),
		fmt.Sprintf("Worktree isolation: Git/source isolation only; not a security sandbox"),
		fmt.Sprintf("External isolation: expectation=%s enforcement=%s attestation=%t", cfg.SafetyDeclaration.ExternalIsolation.Expectation, cfg.SafetyDeclaration.ExternalIsolation.Enforcement, cfg.SafetyDeclaration.ExternalIsolation.Attestation != nil),
		fmt.Sprintf("Network policy: access=%s enforcement=%s attestation=%t", cfg.SafetyDeclaration.Network.Access, cfg.SafetyDeclaration.Network.Enforcement, cfg.SafetyDeclaration.Network.Attestation != nil),
		fmt.Sprintf("Git hooks policy: %s trusted=%d", cfg.SafetyDeclaration.Hooks.Policy, len(cfg.SafetyDeclaration.Hooks.Trusted)),
		fmt.Sprintf("Environment policy: inherit_host=%t allow=%s", cfg.SafetyDeclaration.Environment.InheritHost, formatVerificationArgs(cfg.SafetyDeclaration.Environment.Allow)),
		fmt.Sprintf("Secret redaction sources: environment_variables=%s", formatVerificationArgs(cfg.SafetyDeclaration.Redaction.EnvironmentVariables)),
		fmt.Sprintf("Fully unattended acknowledgement present: %t", strings.TrimSpace(cfg.SafetyDeclaration.Acknowledgement) != ""),
		fmt.Sprintf("Retention policy schema: %s", cfg.RetentionPolicy.SchemaVersion),
		fmt.Sprintf("Retention mutation enabled: %t", cfg.RetentionPolicy.MutationEnabled),
		fmt.Sprintf("Retention recent run count: %d", cfg.RetentionPolicy.RecentRunCount),
		fmt.Sprintf("Retention ages: compress=%s prune=%s", cfg.RetentionPolicy.CompressAfter, cfg.RetentionPolicy.PruneAfter),
		fmt.Sprintf("Retention classes: codex_jsonl=%t codex_stderr=%t prune_compressed=%t", cfg.RetentionPolicy.CompressCodexJSONL, cfg.RetentionPolicy.CompressCodexStderr, cfg.RetentionPolicy.PruneCompressedStreams),
		fmt.Sprintf("Retention verified export required: %t", cfg.RetentionPolicy.RequireVerifiedExport),
		fmt.Sprintf("Retention operation bounds: files=%d bytes=%d", cfg.RetentionPolicy.MaxFilesPerOperation, cfg.RetentionPolicy.MaxBytesPerOperation),
		fmt.Sprintf("Notification policy schema: %s", cfg.NotificationPolicy.SchemaVersion),
		fmt.Sprintf("Notifications enabled: %t", cfg.NotificationPolicy.Enabled),
		fmt.Sprintf("Notification events: %s", formatNotificationEvents(cfg.NotificationPolicy.Events)),
		fmt.Sprintf("Notification executable: %s", cfg.NotificationPolicy.Executable),
		fmt.Sprintf("Notification argument count: %d", len(cfg.NotificationPolicy.Args)),
		fmt.Sprintf("Notification directory: %s", cfg.NotificationPolicy.Directory),
		fmt.Sprintf("Notification environment names: %s", formatVerificationArgs(cfg.NotificationPolicy.EnvironmentNames)),
		fmt.Sprintf("Notification bounds: timeout=%s stdout=%d stderr=%d attempts=%d retry_delay=%s", cfg.NotificationPolicy.Timeout, cfg.NotificationPolicy.StdoutCap, cfg.NotificationPolicy.StderrCap, cfg.NotificationPolicy.MaximumAttempts, cfg.NotificationPolicy.RetryDelay),
		fmt.Sprintf("Git executable: %s", effectiveString(cfg.GitExecutable, defaultGitExecutable)),
		fmt.Sprintf("Git timeout: %s", effectiveDuration(cfg.GitTimeout, defaultGitTimeout)),
		fmt.Sprintf("Verification missing policy: %s", cfg.MissingVerificationPolicy),
		fmt.Sprintf("Verification command count: %d", len(cfg.VerificationCommands)),
	}
	for i, command := range cfg.VerificationCommands {
		lines = append(lines, formatVerificationCommand(i, command))
	}
	if cfg.VerificationPlan != nil {
		lines = append(lines, fmt.Sprintf("Verification tiered plan: %s", cfg.VerificationPlan.SchemaVersion), fmt.Sprintf("Verification tier count: %d", len(cfg.VerificationPlan.Tiers)))
		for i, tier := range cfg.VerificationPlan.Tiers {
			lines = append(lines, fmt.Sprintf("Verification tier %d: id=%s kind=%s required_for_final=%t run_for_fast=%t run_for_final=%t rerun_policy=%s command_count=%d", i, tier.ID, tier.Kind, tier.RequiredForFinal, tier.RunForFast, tier.RunForFinal, tier.RerunPolicy, len(tier.Commands)))
		}
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

func formatNotificationEvents(events []autonomousnotification.Event) string {
	values := make([]string, len(events))
	for i, event := range events {
		values[i] = string(event)
	}
	return formatVerificationArgs(values)
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
