package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/runonce"
	"revolvr/internal/verification"
)

const DefaultConfigFile = "config.yaml"

const (
	DefaultCodexExecutable                = codexexec.DefaultExecutable
	DefaultCodexModel                     = codexexec.DefaultModel
	DefaultCodexReasoningEffort           = codexexec.DefaultReasoningEffort
	DefaultCodexEphemeral                 = true
	DefaultCodexBypassApprovalsAndSandbox = true
	DefaultGitExecutable                  = "git"
	DefaultGitTimeout                     = 30 * time.Second
	DefaultCommitTimeout                  = 30 * time.Second
)

type RunConfigCheckResult struct {
	Path                  string
	Found                 bool
	Effective             runonce.Config
	EffectiveConfigSchema string
	EffectiveConfigSHA256 string
}

type fileConfig struct {
	Codex        codexConfig        `yaml:"codex"`
	Git          gitConfig          `yaml:"git"`
	Verification verificationConfig `yaml:"verification"`
	Commit       commitConfig       `yaml:"commit"`
	Output       outputConfig       `yaml:"output"`
}

type codexConfig struct {
	Executable                           string  `yaml:"executable"`
	Model                                *string `yaml:"model"`
	ReasoningEffort                      *string `yaml:"reasoning_effort"`
	Ephemeral                            *bool   `yaml:"ephemeral"`
	Sandbox                              string  `yaml:"sandbox"`
	ApprovalPolicy                       string  `yaml:"approval_policy"`
	DangerouslyBypassApprovalsAndSandbox *bool   `yaml:"dangerously_bypass_approvals_and_sandbox"`
	Yolo                                 *bool   `yaml:"yolo"`
	TimeoutSeconds                       int     `yaml:"timeout_seconds"`
}

type gitConfig struct {
	Executable     string `yaml:"executable"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type verificationConfig struct {
	MissingPolicy string              `yaml:"missing_policy"`
	Commands      *[]verificationItem `yaml:"commands"`
	Tiers         *[]verificationTier `yaml:"tiers"`
}

type verificationItem struct {
	Name           string   `yaml:"name"`
	Args           []string `yaml:"args"`
	Dir            string   `yaml:"dir"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
	Env            []string `yaml:"env"`
	StdoutCapBytes int      `yaml:"stdout_cap_bytes"`
	StderrCapBytes int      `yaml:"stderr_cap_bytes"`
}

type verificationTier struct {
	ID               string             `yaml:"id"`
	Kind             string             `yaml:"kind"`
	RequiredForFinal bool               `yaml:"required_for_final"`
	RunForFast       bool               `yaml:"run_for_fast"`
	RunForFinal      bool               `yaml:"run_for_final"`
	RerunPolicy      string             `yaml:"rerun_policy"`
	Commands         []verificationItem `yaml:"commands"`
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

func CheckRunConfig(workDir string) (RunConfigCheckResult, error) {
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		return RunConfigCheckResult{}, err
	}
	path := filepath.Join(paths.StateDir, DefaultConfigFile)
	found, err := existingFile(path)
	if err != nil {
		return RunConfigCheckResult{}, err
	}

	cfg, err := LoadRunOnceConfig(workDir, DefaultRunOnceConfig(workDir))
	if err != nil {
		return RunConfigCheckResult{}, err
	}
	effective, err := runonce.EffectiveConfig(cfg)
	if err != nil {
		return RunConfigCheckResult{}, err
	}
	fingerprint, err := runonce.FingerprintEffectiveConfig(effective)
	if err != nil {
		return RunConfigCheckResult{}, err
	}
	return RunConfigCheckResult{
		Path:                  path,
		Found:                 found,
		Effective:             effective,
		EffectiveConfigSchema: fingerprint.Schema,
		EffectiveConfigSHA256: fingerprint.SHA256,
	}, nil
}

func DefaultRunOnceConfig(workDir string) runonce.Config {
	return runonce.Config{
		WorkingDir:                     workDir,
		CodexExecutable:                DefaultCodexExecutable,
		CodexModel:                     DefaultCodexModel,
		CodexReasoningEffort:           DefaultCodexReasoningEffort,
		CodexEphemeral:                 DefaultCodexEphemeral,
		CodexBypassApprovalsAndSandbox: DefaultCodexBypassApprovalsAndSandbox,
	}
}

func LoadRunOnceConfig(workDir string, base runonce.Config) (runonce.Config, error) {
	paths, err := resolveStatePaths(workDir)
	if err != nil {
		return runonce.Config{}, err
	}
	path := filepath.Join(paths.StateDir, DefaultConfigFile)
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
	if cfg.Codex.Model != nil {
		value := strings.TrimSpace(*cfg.Codex.Model)
		if value == "" {
			return runonce.Config{}, errors.New("codex model must not be empty")
		}
		normalized, err := codexexec.NormalizeModel(value)
		if err != nil {
			return runonce.Config{}, err
		}
		base.CodexModel = normalized
	}
	if cfg.Codex.ReasoningEffort != nil {
		value := strings.TrimSpace(*cfg.Codex.ReasoningEffort)
		if value == "" {
			return runonce.Config{}, errors.New("codex reasoning_effort must not be empty")
		}
		normalized, err := codexexec.NormalizeReasoningEffort(value)
		if err != nil {
			return runonce.Config{}, err
		}
		base.CodexReasoningEffort = normalized
	}
	if cfg.Codex.Ephemeral != nil {
		if !*cfg.Codex.Ephemeral {
			return runonce.Config{}, errors.New("codex ephemeral must be true; persistent or resumed sessions are not supported")
		}
		base.CodexEphemeral = true
	}
	if value := strings.TrimSpace(cfg.Codex.Sandbox); value != "" {
		base.CodexSandbox = value
	}
	if value := strings.TrimSpace(cfg.Codex.ApprovalPolicy); value != "" {
		base.CodexApprovalPolicy = value
	}
	if cfg.Codex.DangerouslyBypassApprovalsAndSandbox != nil && cfg.Codex.Yolo != nil {
		return runonce.Config{}, errors.New("codex dangerously_bypass_approvals_and_sandbox and yolo cannot both be set")
	}
	if cfg.Codex.DangerouslyBypassApprovalsAndSandbox != nil {
		base.CodexBypassApprovalsAndSandbox = *cfg.Codex.DangerouslyBypassApprovalsAndSandbox
	}
	if cfg.Codex.Yolo != nil {
		base.CodexBypassApprovalsAndSandbox = *cfg.Codex.Yolo
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
	if cfg.Verification.Commands != nil && cfg.Verification.Tiers != nil {
		return runonce.Config{}, errors.New("verification commands and tiers cannot both be set")
	}
	if cfg.Verification.Commands != nil && len(*cfg.Verification.Commands) > 0 {
		commands := make([]verification.Command, 0, len(*cfg.Verification.Commands))
		for i, command := range *cfg.Verification.Commands {
			name := strings.TrimSpace(command.Name)
			if name == "" {
				return runonce.Config{}, fmt.Errorf("verification.commands[%d].name is required", i)
			}
			item := verification.Command{
				Name:      name,
				Args:      append([]string(nil), command.Args...),
				Dir:       strings.TrimSpace(command.Dir),
				Env:       append([]string(nil), command.Env...),
				StdoutCap: command.StdoutCapBytes,
				StderrCap: command.StderrCapBytes,
			}
			if command.TimeoutSeconds > 0 {
				item.Timeout = seconds(command.TimeoutSeconds)
			}
			commands = append(commands, item)
		}
		base.VerificationCommands = commands
	}
	if cfg.Verification.Tiers != nil {
		tiers := make([]autonomousverification.Tier, 0, len(*cfg.Verification.Tiers))
		for i, configured := range *cfg.Verification.Tiers {
			commands := make([]verification.Command, 0, len(configured.Commands))
			for j, command := range configured.Commands {
				name := strings.TrimSpace(command.Name)
				if name == "" {
					return runonce.Config{}, fmt.Errorf("verification.tiers[%d].commands[%d].name is required", i, j)
				}
				item := verification.Command{Name: name, Args: append([]string(nil), command.Args...), Dir: strings.TrimSpace(command.Dir), Env: append([]string(nil), command.Env...), StdoutCap: command.StdoutCapBytes, StderrCap: command.StderrCapBytes}
				if command.TimeoutSeconds > 0 {
					item.Timeout = seconds(command.TimeoutSeconds)
				}
				commands = append(commands, item)
			}
			rerunPolicy := autonomousverification.RerunPolicy(strings.TrimSpace(configured.RerunPolicy))
			if rerunPolicy == "" {
				rerunPolicy = autonomousverification.RerunNever
			}
			tiers = append(tiers, autonomousverification.Tier{ID: strings.TrimSpace(configured.ID), Kind: autonomousverification.TierKind(strings.TrimSpace(configured.Kind)), RequiredForFinal: configured.RequiredForFinal, RunForFast: configured.RunForFast, RunForFinal: configured.RunForFinal, Commands: commands, RerunPolicy: rerunPolicy})
		}
		plan := autonomousverification.Plan{SchemaVersion: autonomousverification.PlanSchemaVersion, Tiers: tiers}
		if err := plan.Validate(); err != nil {
			return runonce.Config{}, err
		}
		base.VerificationCommands = nil
		base.VerificationPlan = &plan
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
