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

	"revolvr/internal/artifactretention"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoussafety"
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
	Codex         codexConfig        `yaml:"codex"`
	Git           gitConfig          `yaml:"git"`
	Verification  verificationConfig `yaml:"verification"`
	Commit        commitConfig       `yaml:"commit"`
	Output        outputConfig       `yaml:"output"`
	Autonomy      autonomyConfig     `yaml:"autonomy"`
	Retention     retentionConfig    `yaml:"retention"`
	Notifications notificationConfig `yaml:"notifications"`
	Queue         queueConfig        `yaml:"queue"`
}

type queueConfig struct {
	SchemaVersion  string `yaml:"schema_version"`
	MaximumWorkers *int   `yaml:"maximum_workers"`
}

type notificationConfig struct {
	SchemaVersion     string   `yaml:"schema_version"`
	Enabled           *bool    `yaml:"enabled"`
	Events            []string `yaml:"events"`
	Executable        string   `yaml:"executable"`
	Args              []string `yaml:"args"`
	Directory         string   `yaml:"directory"`
	EnvironmentNames  []string `yaml:"environment_names"`
	TimeoutSeconds    *int64   `yaml:"timeout_seconds"`
	StdoutCapBytes    *int     `yaml:"stdout_cap_bytes"`
	StderrCapBytes    *int     `yaml:"stderr_cap_bytes"`
	MaximumAttempts   *int     `yaml:"maximum_attempts"`
	RetryDelaySeconds *int64   `yaml:"retry_delay_seconds"`
}

type retentionConfig struct {
	SchemaVersion          string `yaml:"schema_version"`
	MutationEnabled        *bool  `yaml:"mutation_enabled"`
	RecentRunCount         *int   `yaml:"recent_run_count"`
	CompressAfterSeconds   *int64 `yaml:"compress_after_seconds"`
	PruneAfterSeconds      *int64 `yaml:"prune_after_seconds"`
	MinimumCompressBytes   *int64 `yaml:"minimum_compress_bytes"`
	CompressCodexJSONL     *bool  `yaml:"compress_codex_jsonl"`
	CompressCodexStderr    *bool  `yaml:"compress_codex_stderr"`
	PruneCompressedStreams *bool  `yaml:"prune_compressed_streams"`
	RequireVerifiedExport  *bool  `yaml:"require_verified_export"`
	MaxFilesPerOperation   *int   `yaml:"max_files_per_operation"`
	MaxBytesPerOperation   *int64 `yaml:"max_bytes_per_operation"`
	DecompressionCapBytes  *int64 `yaml:"decompression_cap_bytes"`
}

type autonomyConfig struct {
	SchemaVersion     string                  `yaml:"schema_version"`
	Mode              string                  `yaml:"mode"`
	ExternalIsolation externalIsolationConfig `yaml:"external_isolation"`
	Network           networkPolicyConfig     `yaml:"network"`
	Hooks             hookTrustConfig         `yaml:"hooks"`
	Environment       environmentPolicyConfig `yaml:"environment"`
	Redaction         redactionPolicyConfig   `yaml:"redaction"`
	Acknowledgement   string                  `yaml:"acknowledgement"`
}

type attestationConfig struct {
	Authority string `yaml:"authority"`
	Evidence  string `yaml:"evidence"`
	SHA256    string `yaml:"sha256"`
}
type externalIsolationConfig struct {
	Expectation string             `yaml:"expectation"`
	Enforcement string             `yaml:"enforcement"`
	Attestation *attestationConfig `yaml:"attestation"`
}
type networkPolicyConfig struct {
	Access      string             `yaml:"access"`
	Enforcement string             `yaml:"enforcement"`
	Attestation *attestationConfig `yaml:"attestation"`
}
type trustedHookConfig struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}
type hookTrustConfig struct {
	Policy  string              `yaml:"policy"`
	Trusted []trustedHookConfig `yaml:"trusted"`
}
type environmentPolicyConfig struct {
	InheritHost *bool    `yaml:"inherit_host"`
	Allow       []string `yaml:"allow"`
}
type redactionPolicyConfig struct {
	SchemaVersion        string   `yaml:"schema_version"`
	EnvironmentVariables []string `yaml:"environment_variables"`
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
		SafetyDeclaration:              autonomoussafety.DefaultDeclaration(),
		RetentionPolicy:                artifactretention.DefaultPolicy(),
		NotificationPolicy:             autonomousnotification.DefaultPolicy(),
		QueuePolicy:                    autonomousqueue.DefaultPolicy(),
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
	declaration, err := cfg.Autonomy.apply(base.SafetyDeclaration)
	if err != nil {
		return runonce.Config{}, err
	}
	base.SafetyDeclaration = declaration
	notifications, err := cfg.Notifications.apply(base.NotificationPolicy, declaration.Redaction.EnvironmentVariables)
	if err != nil {
		return runonce.Config{}, err
	}
	base.NotificationPolicy = notifications
	queue, err := cfg.Queue.apply(base.QueuePolicy)
	if err != nil {
		return runonce.Config{}, err
	}
	base.QueuePolicy = queue
	retention, err := cfg.Retention.apply(base.RetentionPolicy)
	if err != nil {
		return runonce.Config{}, err
	}
	base.RetentionPolicy = retention
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

func (cfg queueConfig) apply(base autonomousqueue.Policy) (autonomousqueue.Policy, error) {
	if base.SchemaVersion == "" {
		base = autonomousqueue.DefaultPolicy()
	}
	if value := strings.TrimSpace(cfg.SchemaVersion); value != "" {
		base.SchemaVersion = value
	}
	if cfg.MaximumWorkers != nil {
		base.MaximumWorkers = *cfg.MaximumWorkers
	}
	if err := base.Validate(); err != nil {
		return autonomousqueue.Policy{}, err
	}
	return base, nil
}

func (cfg notificationConfig) apply(base autonomousnotification.Policy, redactionNames []string) (autonomousnotification.Policy, error) {
	if base.SchemaVersion == "" {
		base = autonomousnotification.DefaultPolicy()
	}
	if value := strings.TrimSpace(cfg.SchemaVersion); value != "" {
		base.SchemaVersion = value
	}
	if cfg.Enabled != nil {
		base.Enabled = *cfg.Enabled
	}
	if cfg.Events != nil {
		base.Events = make([]autonomousnotification.Event, len(cfg.Events))
		for i, event := range cfg.Events {
			base.Events[i] = autonomousnotification.Event(strings.TrimSpace(event))
		}
	}
	if cfg.Executable != "" {
		base.Executable = cfg.Executable
	}
	if cfg.Args != nil {
		base.Args = append([]string(nil), cfg.Args...)
	}
	if cfg.Directory != "" {
		base.Directory = cfg.Directory
	}
	if cfg.EnvironmentNames != nil {
		base.EnvironmentNames = append([]string(nil), cfg.EnvironmentNames...)
	}
	if cfg.TimeoutSeconds != nil {
		if *cfg.TimeoutSeconds > int64((time.Duration(1<<63-1))/time.Second) {
			return autonomousnotification.Policy{}, errors.New("notification timeout_seconds overflows duration")
		}
		base.Timeout = time.Duration(*cfg.TimeoutSeconds) * time.Second
	}
	if cfg.StdoutCapBytes != nil {
		base.StdoutCap = *cfg.StdoutCapBytes
	}
	if cfg.StderrCapBytes != nil {
		base.StderrCap = *cfg.StderrCapBytes
	}
	if cfg.MaximumAttempts != nil {
		base.MaximumAttempts = *cfg.MaximumAttempts
	}
	if cfg.RetryDelaySeconds != nil {
		if *cfg.RetryDelaySeconds > int64((time.Duration(1<<63-1))/time.Second) {
			return autonomousnotification.Policy{}, errors.New("notification retry_delay_seconds overflows duration")
		}
		base.RetryDelay = time.Duration(*cfg.RetryDelaySeconds) * time.Second
	}
	return base.Normalize(redactionNames)
}

func (cfg retentionConfig) apply(base artifactretention.Policy) (artifactretention.Policy, error) {
	if base.SchemaVersion == "" {
		base = artifactretention.DefaultPolicy()
	}
	if v := strings.TrimSpace(cfg.SchemaVersion); v != "" {
		base.SchemaVersion = v
	}
	if cfg.MutationEnabled != nil {
		base.MutationEnabled = *cfg.MutationEnabled
	}
	if cfg.RecentRunCount != nil {
		base.RecentRunCount = *cfg.RecentRunCount
	}
	if cfg.CompressAfterSeconds != nil {
		if *cfg.CompressAfterSeconds < 0 {
			return base, errors.New("retention compress_after_seconds cannot be negative")
		}
		if *cfg.CompressAfterSeconds > int64((time.Duration(1<<63-1))/time.Second) {
			return base, errors.New("retention compress_after_seconds overflows duration")
		}
		base.CompressAfter = time.Duration(*cfg.CompressAfterSeconds) * time.Second
	}
	if cfg.PruneAfterSeconds != nil {
		if *cfg.PruneAfterSeconds < 0 {
			return base, errors.New("retention prune_after_seconds cannot be negative")
		}
		if *cfg.PruneAfterSeconds > int64((time.Duration(1<<63-1))/time.Second) {
			return base, errors.New("retention prune_after_seconds overflows duration")
		}
		base.PruneAfter = time.Duration(*cfg.PruneAfterSeconds) * time.Second
	}
	if cfg.MinimumCompressBytes != nil {
		base.MinimumCompressBytes = *cfg.MinimumCompressBytes
	}
	if cfg.CompressCodexJSONL != nil {
		base.CompressCodexJSONL = *cfg.CompressCodexJSONL
	}
	if cfg.CompressCodexStderr != nil {
		base.CompressCodexStderr = *cfg.CompressCodexStderr
	}
	if cfg.PruneCompressedStreams != nil {
		base.PruneCompressedStreams = *cfg.PruneCompressedStreams
	}
	if cfg.RequireVerifiedExport != nil {
		base.RequireVerifiedExport = *cfg.RequireVerifiedExport
	}
	if cfg.MaxFilesPerOperation != nil {
		base.MaxFilesPerOperation = *cfg.MaxFilesPerOperation
	}
	if cfg.MaxBytesPerOperation != nil {
		base.MaxBytesPerOperation = *cfg.MaxBytesPerOperation
	}
	if cfg.DecompressionCapBytes != nil {
		base.DecompressionCapBytes = *cfg.DecompressionCapBytes
	}
	if err := base.Validate(); err != nil {
		return artifactretention.Policy{}, err
	}
	return base, nil
}

func (cfg autonomyConfig) apply(base autonomoussafety.Declaration) (autonomoussafety.Declaration, error) {
	if base.SchemaVersion == "" {
		base = autonomoussafety.DefaultDeclaration()
	}
	if value := strings.TrimSpace(cfg.SchemaVersion); value != "" {
		base.SchemaVersion = value
	}
	if value := strings.TrimSpace(cfg.Mode); value != "" {
		base.Mode = autonomoussafety.Mode(value)
	}
	if value := strings.TrimSpace(cfg.ExternalIsolation.Expectation); value != "" {
		base.ExternalIsolation.Expectation = autonomoussafety.IsolationExpectation(value)
	}
	if value := strings.TrimSpace(cfg.ExternalIsolation.Enforcement); value != "" {
		base.ExternalIsolation.Enforcement = autonomoussafety.Enforcement(value)
	}
	if cfg.ExternalIsolation.Attestation != nil {
		base.ExternalIsolation.Attestation = convertAttestation(cfg.ExternalIsolation.Attestation)
	}
	if value := strings.TrimSpace(cfg.Network.Access); value != "" {
		base.Network.Access = autonomoussafety.NetworkAccess(value)
	}
	if value := strings.TrimSpace(cfg.Network.Enforcement); value != "" {
		base.Network.Enforcement = autonomoussafety.Enforcement(value)
	}
	if cfg.Network.Attestation != nil {
		base.Network.Attestation = convertAttestation(cfg.Network.Attestation)
	}
	if value := strings.TrimSpace(cfg.Hooks.Policy); value != "" {
		base.Hooks.Policy = autonomoussafety.HookPolicy(value)
	}
	if cfg.Hooks.Trusted != nil {
		base.Hooks.Trusted = make([]autonomoussafety.TrustedHook, 0, len(cfg.Hooks.Trusted))
		for _, hook := range cfg.Hooks.Trusted {
			base.Hooks.Trusted = append(base.Hooks.Trusted, autonomoussafety.TrustedHook{Path: strings.TrimSpace(hook.Path), SHA256: strings.TrimSpace(hook.SHA256)})
		}
	}
	if cfg.Environment.InheritHost != nil {
		base.Environment.InheritHost = *cfg.Environment.InheritHost
	}
	if cfg.Environment.Allow != nil {
		base.Environment.Allow = append([]string(nil), cfg.Environment.Allow...)
	}
	if value := strings.TrimSpace(cfg.Redaction.SchemaVersion); value != "" {
		base.Redaction.SchemaVersion = value
	}
	if cfg.Redaction.EnvironmentVariables != nil {
		base.Redaction.EnvironmentVariables = append([]string(nil), cfg.Redaction.EnvironmentVariables...)
	}
	if value := strings.TrimSpace(cfg.Acknowledgement); value != "" {
		base.Acknowledgement = value
	}
	if err := base.Validate(); err != nil {
		return autonomoussafety.Declaration{}, err
	}
	return base, nil
}

func convertAttestation(value *attestationConfig) *autonomoussafety.Attestation {
	if value == nil {
		return nil
	}
	return &autonomoussafety.Attestation{Authority: strings.TrimSpace(value.Authority), Evidence: strings.TrimSpace(value.Evidence), SHA256: strings.TrimSpace(value.SHA256)}
}

func seconds(value int) time.Duration {
	return time.Duration(value) * time.Second
}

func applyPositiveInt(target *int, value int) {
	if value > 0 {
		*target = value
	}
}
