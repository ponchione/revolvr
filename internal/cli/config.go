package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"revolvr/internal/runonce"
	"revolvr/internal/verification"
)

const defaultConfigFile = "config.yaml"

const (
	defaultCodexExecutable = "codex"
	defaultGitExecutable   = "git"
	defaultGitTimeout      = 30 * time.Second
	defaultCommitTimeout   = 30 * time.Second
)

type fileConfig struct {
	Codex        codexConfig        `yaml:"codex"`
	Git          gitConfig          `yaml:"git"`
	Verification verificationConfig `yaml:"verification"`
	Commit       commitConfig       `yaml:"commit"`
	Output       outputConfig       `yaml:"output"`
}

type codexConfig struct {
	Executable     string `yaml:"executable"`
	Sandbox        string `yaml:"sandbox"`
	ApprovalPolicy string `yaml:"approval_policy"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type gitConfig struct {
	Executable     string `yaml:"executable"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type verificationConfig struct {
	MissingPolicy string             `yaml:"missing_policy"`
	Commands      []verificationItem `yaml:"commands"`
}

type verificationItem struct {
	Name           string   `yaml:"name"`
	Args           []string `yaml:"args"`
	Dir            string   `yaml:"dir"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
}

type commitConfig struct {
	AllowPreExistingDirty    *bool `yaml:"allow_pre_existing_dirty"`
	AllowMissingVerification *bool `yaml:"allow_missing_verification"`
	TimeoutSeconds           int   `yaml:"timeout_seconds"`
}

type outputConfig struct {
	CodexStdoutCapBytes        int `yaml:"codex_stdout_cap_bytes"`
	CodexStderrCapBytes        int `yaml:"codex_stderr_cap_bytes"`
	GitStdoutCapBytes          int `yaml:"git_stdout_cap_bytes"`
	GitStderrCapBytes          int `yaml:"git_stderr_cap_bytes"`
	VerificationStdoutCapBytes int `yaml:"verification_stdout_cap_bytes"`
	VerificationStderrCapBytes int `yaml:"verification_stderr_cap_bytes"`
	CommitStdoutCapBytes       int `yaml:"commit_stdout_cap_bytes"`
	CommitStderrCapBytes       int `yaml:"commit_stderr_cap_bytes"`
}

type configCheckResult struct {
	Path      string
	Found     bool
	Effective runonce.Config
}

func checkRunConfig(workDir string) (configCheckResult, error) {
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		return configCheckResult{}, err
	}
	path := filepath.Join(paths.StateDir, defaultConfigFile)
	found, err := existingFile(path)
	if err != nil {
		return configCheckResult{}, err
	}

	cfg, err := loadRunOnceConfig(workDir, runonce.Config{WorkingDir: workDir})
	if err != nil {
		return configCheckResult{}, err
	}
	effective, err := runonce.EffectiveConfig(cfg)
	if err != nil {
		return configCheckResult{}, err
	}
	return configCheckResult{
		Path:      path,
		Found:     found,
		Effective: effective,
	}, nil
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
		fmt.Sprintf("Codex sandbox: %s", cfg.CodexSandbox),
		fmt.Sprintf("Codex approval policy: %s", cfg.CodexApprovalPolicy),
		fmt.Sprintf("Codex timeout: %s", cfg.CodexTimeout),
		fmt.Sprintf("Git executable: %s", effectiveString(cfg.GitExecutable, defaultGitExecutable)),
		fmt.Sprintf("Git timeout: %s", effectiveDuration(cfg.GitTimeout, defaultGitTimeout)),
		fmt.Sprintf("Verification missing policy: %s", cfg.MissingVerificationPolicy),
		fmt.Sprintf("Verification command count: %d", len(cfg.VerificationCommands)),
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
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func loadRunOnceConfig(workDir string, base runonce.Config) (runonce.Config, error) {
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		return runonce.Config{}, err
	}
	path := filepath.Join(paths.StateDir, defaultConfigFile)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return runonce.Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return base, nil
	}

	parsed, err := parseFileConfig(content)
	if err != nil {
		return runonce.Config{}, fmt.Errorf("load config %s: %w", path, err)
	}
	return parsed.apply(base)
}

func parseFileConfig(content []byte) (fileConfig, error) {
	var cfg fileConfig
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return fileConfig{}, fmt.Errorf("decode YAML: %w", err)
	}
	return cfg, nil
}

func (cfg fileConfig) apply(base runonce.Config) (runonce.Config, error) {
	if value := strings.TrimSpace(cfg.Codex.Executable); value != "" {
		base.CodexExecutable = value
	}
	if value := strings.TrimSpace(cfg.Codex.Sandbox); value != "" {
		base.CodexSandbox = value
	}
	if value := strings.TrimSpace(cfg.Codex.ApprovalPolicy); value != "" {
		base.CodexApprovalPolicy = value
	}
	if cfg.Codex.TimeoutSeconds > 0 {
		base.CodexTimeout = seconds(cfg.Codex.TimeoutSeconds)
	}

	if value := strings.TrimSpace(cfg.Git.Executable); value != "" {
		base.GitExecutable = value
	}
	if cfg.Git.TimeoutSeconds > 0 {
		base.GitTimeout = seconds(cfg.Git.TimeoutSeconds)
	}

	if value := strings.TrimSpace(cfg.Verification.MissingPolicy); value != "" {
		policy := verification.MissingCommandsPolicy(value)
		switch policy {
		case verification.MissingCommandsFail, verification.MissingCommandsPass:
			base.MissingVerificationPolicy = policy
		default:
			return runonce.Config{}, fmt.Errorf("invalid verification missing_policy %q (want %q or %q)", value, verification.MissingCommandsFail, verification.MissingCommandsPass)
		}
	}
	if len(cfg.Verification.Commands) > 0 {
		commands := make([]verification.Command, 0, len(cfg.Verification.Commands))
		for i, command := range cfg.Verification.Commands {
			name := strings.TrimSpace(command.Name)
			if name == "" {
				return runonce.Config{}, fmt.Errorf("verification.commands[%d].name is required", i)
			}
			item := verification.Command{
				Name: name,
				Args: append([]string(nil), command.Args...),
				Dir:  strings.TrimSpace(command.Dir),
			}
			if command.TimeoutSeconds > 0 {
				item.Timeout = seconds(command.TimeoutSeconds)
			}
			commands = append(commands, item)
		}
		base.VerificationCommands = commands
	}

	if cfg.Commit.AllowPreExistingDirty != nil {
		base.AllowPreExistingDirty = *cfg.Commit.AllowPreExistingDirty
	}
	if cfg.Commit.AllowMissingVerification != nil {
		base.AllowMissingVerification = *cfg.Commit.AllowMissingVerification
	}
	if cfg.Commit.TimeoutSeconds > 0 {
		base.CommitTimeout = seconds(cfg.Commit.TimeoutSeconds)
	}

	applyPositiveInt(&base.CodexStdoutCap, cfg.Output.CodexStdoutCapBytes)
	applyPositiveInt(&base.CodexStderrCap, cfg.Output.CodexStderrCapBytes)
	applyPositiveInt(&base.GitStdoutCap, cfg.Output.GitStdoutCapBytes)
	applyPositiveInt(&base.GitStderrCap, cfg.Output.GitStderrCapBytes)
	applyPositiveInt(&base.VerificationStdoutCap, cfg.Output.VerificationStdoutCapBytes)
	applyPositiveInt(&base.VerificationStderrCap, cfg.Output.VerificationStderrCapBytes)
	applyPositiveInt(&base.CommitStdoutCap, cfg.Output.CommitStdoutCapBytes)
	applyPositiveInt(&base.CommitStderrCap, cfg.Output.CommitStderrCapBytes)

	return base, nil
}

func seconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}

func applyPositiveInt(target *int, value int) {
	if value > 0 {
		*target = value
	}
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
