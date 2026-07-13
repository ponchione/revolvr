package app

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
	TimeoutSeconds                       *int64  `yaml:"timeout_seconds"`
}

type gitConfig struct {
	Executable     string `yaml:"executable"`
	TimeoutSeconds *int64 `yaml:"timeout_seconds"`
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
	TimeoutSeconds *int64   `yaml:"timeout_seconds"`
	Env            []string `yaml:"env"`
	StdoutCapBytes *int     `yaml:"stdout_cap_bytes"`
	StderrCapBytes *int     `yaml:"stderr_cap_bytes"`
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
	AllowPreExistingDirty    *bool  `yaml:"allow_pre_existing_dirty"`
	AllowMissingVerification *bool  `yaml:"allow_missing_verification"`
	TimeoutSeconds           *int64 `yaml:"timeout_seconds"`
}

type outputConfig struct {
	CodexStdoutCapBytes        *int `yaml:"codex_stdout_cap_bytes"`
	CodexStderrCapBytes        *int `yaml:"codex_stderr_cap_bytes"`
	GitStdoutCapBytes          *int `yaml:"git_stdout_cap_bytes"`
	GitStderrCapBytes          *int `yaml:"git_stderr_cap_bytes"`
	VerificationStdoutCapBytes *int `yaml:"verification_stdout_cap_bytes"`
	VerificationStderrCapBytes *int `yaml:"verification_stderr_cap_bytes"`
	CommitStdoutCapBytes       *int `yaml:"commit_stdout_cap_bytes"`
	CommitStderrCapBytes       *int `yaml:"commit_stderr_cap_bytes"`
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
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return fileConfig{}, fmt.Errorf("decode YAML: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err == nil {
		return fileConfig{}, errors.New("decode YAML: exactly one document is required")
	} else if !errors.Is(err, io.EOF) {
		return fileConfig{}, fmt.Errorf("decode trailing YAML: %w", err)
	}
	if field := nullNumericField(&document, ""); field != "" {
		return fileConfig{}, fmt.Errorf("decode YAML field %s: null is not an integer", field)
	}

	var cfg fileConfig
	decoder = yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if field := yamlErrorField(document, err); field != "" {
			return fileConfig{}, fmt.Errorf("decode YAML field %s: %w", field, err)
		}
		return fileConfig{}, fmt.Errorf("decode YAML: %w", err)
	}
	return cfg, nil
}

func nullNumericField(node *yaml.Node, prefix string) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if field := nullNumericField(child, prefix); field != "" {
				return field
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			field := key.Value
			if prefix != "" {
				field = prefix + "." + field
			}
			if value.Kind == yaml.ScalarNode && value.Tag == "!!null" && isNumericConfigName(key.Value) {
				return field
			}
			if nested := nullNumericField(value, field); nested != "" {
				return nested
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if field := nullNumericField(child, fmt.Sprintf("%s[%d]", prefix, i)); field != "" {
				return field
			}
		}
	}
	return ""
}

func isNumericConfigName(name string) bool {
	switch name {
	case "maximum_workers",
		"timeout_seconds", "stdout_cap_bytes", "stderr_cap_bytes", "maximum_attempts", "retry_delay_seconds",
		"recent_run_count", "compress_after_seconds", "prune_after_seconds", "minimum_compress_bytes",
		"max_files_per_operation", "max_bytes_per_operation", "decompression_cap_bytes",
		"codex_stdout_cap_bytes", "codex_stderr_cap_bytes", "git_stdout_cap_bytes", "git_stderr_cap_bytes",
		"verification_stdout_cap_bytes", "verification_stderr_cap_bytes", "commit_stdout_cap_bytes", "commit_stderr_cap_bytes":
		return true
	default:
		return false
	}
}

func yamlErrorField(document yaml.Node, err error) string {
	var typeError *yaml.TypeError
	if !errors.As(err, &typeError) || len(typeError.Errors) == 0 {
		return ""
	}
	var line int
	if _, scanErr := fmt.Sscanf(typeError.Errors[0], "line %d:", &line); scanErr != nil {
		return ""
	}
	return yamlFieldAtLine(&document, line, "")
}

func yamlFieldAtLine(node *yaml.Node, line int, prefix string) string {
	if node == nil {
		return ""
	}
	best := ""
	if node.Line == line {
		best = prefix
	}
	choose := func(candidate string) {
		if len(candidate) > len(best) {
			best = candidate
		}
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			choose(yamlFieldAtLine(child, line, prefix))
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			field := key.Value
			if prefix != "" {
				field = prefix + "." + field
			}
			if key.Line == line {
				choose(field)
			}
			choose(yamlFieldAtLine(value, line, field))
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			choose(yamlFieldAtLine(child, line, fmt.Sprintf("%s[%d]", prefix, i)))
		}
	}
	return best
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
	if err := applyPositiveDuration("codex.timeout_seconds", &base.CodexTimeout, cfg.Codex.TimeoutSeconds); err != nil {
		return runonce.Config{}, err
	}

	if value := strings.TrimSpace(cfg.Git.Executable); value != "" {
		base.GitExecutable = value
	}
	if err := applyPositiveDuration("git.timeout_seconds", &base.GitTimeout, cfg.Git.TimeoutSeconds); err != nil {
		return runonce.Config{}, err
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
			item, err := command.apply(fmt.Sprintf("verification.commands[%d]", i))
			if err != nil {
				return runonce.Config{}, err
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
				item, err := command.apply(fmt.Sprintf("verification.tiers[%d].commands[%d]", i, j))
				if err != nil {
					return runonce.Config{}, err
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
	if err := applyPositiveDuration("commit.timeout_seconds", &base.CommitTimeout, cfg.Commit.TimeoutSeconds); err != nil {
		return runonce.Config{}, err
	}

	for _, configured := range []struct {
		field  string
		target *int
		value  *int
	}{
		{"output.codex_stdout_cap_bytes", &base.CodexStdoutCap, cfg.Output.CodexStdoutCapBytes},
		{"output.codex_stderr_cap_bytes", &base.CodexStderrCap, cfg.Output.CodexStderrCapBytes},
		{"output.git_stdout_cap_bytes", &base.GitStdoutCap, cfg.Output.GitStdoutCapBytes},
		{"output.git_stderr_cap_bytes", &base.GitStderrCap, cfg.Output.GitStderrCapBytes},
		{"output.verification_stdout_cap_bytes", &base.VerificationStdoutCap, cfg.Output.VerificationStdoutCapBytes},
		{"output.verification_stderr_cap_bytes", &base.VerificationStderrCap, cfg.Output.VerificationStderrCapBytes},
		{"output.commit_stdout_cap_bytes", &base.CommitStdoutCap, cfg.Output.CommitStdoutCapBytes},
		{"output.commit_stderr_cap_bytes", &base.CommitStderrCap, cfg.Output.CommitStderrCapBytes},
	} {
		if err := applyPositiveInt(configured.field, configured.target, configured.value); err != nil {
			return runonce.Config{}, err
		}
	}

	return base, nil
}

func (command verificationItem) apply(field string) (verification.Command, error) {
	name := strings.TrimSpace(command.Name)
	if name == "" {
		return verification.Command{}, fmt.Errorf("%s.name is required", field)
	}
	item := verification.Command{
		Name: name,
		Args: append([]string(nil), command.Args...),
		Dir:  strings.TrimSpace(command.Dir),
		Env:  append([]string(nil), command.Env...),
	}
	if err := applyPositiveDuration(field+".timeout_seconds", &item.Timeout, command.TimeoutSeconds); err != nil {
		return verification.Command{}, err
	}
	if err := applyPositiveInt(field+".stdout_cap_bytes", &item.StdoutCap, command.StdoutCapBytes); err != nil {
		return verification.Command{}, err
	}
	if err := applyPositiveInt(field+".stderr_cap_bytes", &item.StderrCap, command.StderrCapBytes); err != nil {
		return verification.Command{}, err
	}
	return item, nil
}

func (cfg queueConfig) apply(base autonomousqueue.Policy) (autonomousqueue.Policy, error) {
	if base.SchemaVersion == "" {
		base = autonomousqueue.DefaultPolicy()
	}
	if value := strings.TrimSpace(cfg.SchemaVersion); value != "" {
		base.SchemaVersion = value
	}
	if cfg.MaximumWorkers != nil {
		if *cfg.MaximumWorkers <= 0 || *cfg.MaximumWorkers > autonomousqueue.MaximumWorkerLimit {
			return autonomousqueue.Policy{}, fmt.Errorf("queue.maximum_workers must be between 1 and %d", autonomousqueue.MaximumWorkerLimit)
		}
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
	if err := applyPositiveDuration("notifications.timeout_seconds", &base.Timeout, cfg.TimeoutSeconds); err != nil {
		return autonomousnotification.Policy{}, err
	}
	if cfg.TimeoutSeconds != nil && base.Timeout > autonomousnotification.MaxTimeout {
		return autonomousnotification.Policy{}, fmt.Errorf("notifications.timeout_seconds must be at most %d", int64(autonomousnotification.MaxTimeout/time.Second))
	}
	if cfg.StdoutCapBytes != nil {
		if *cfg.StdoutCapBytes <= 0 || *cfg.StdoutCapBytes > autonomousnotification.MaxOutputCap {
			return autonomousnotification.Policy{}, fmt.Errorf("notifications.stdout_cap_bytes must be between 1 and %d", autonomousnotification.MaxOutputCap)
		}
		base.StdoutCap = *cfg.StdoutCapBytes
	}
	if cfg.StderrCapBytes != nil {
		if *cfg.StderrCapBytes <= 0 || *cfg.StderrCapBytes > autonomousnotification.MaxOutputCap {
			return autonomousnotification.Policy{}, fmt.Errorf("notifications.stderr_cap_bytes must be between 1 and %d", autonomousnotification.MaxOutputCap)
		}
		base.StderrCap = *cfg.StderrCapBytes
	}
	if cfg.MaximumAttempts != nil {
		if *cfg.MaximumAttempts <= 0 || *cfg.MaximumAttempts > autonomousnotification.MaxAttempts {
			return autonomousnotification.Policy{}, fmt.Errorf("notifications.maximum_attempts must be between 1 and %d", autonomousnotification.MaxAttempts)
		}
		base.MaximumAttempts = *cfg.MaximumAttempts
	}
	if err := applyNonNegativeDuration("notifications.retry_delay_seconds", &base.RetryDelay, cfg.RetryDelaySeconds); err != nil {
		return autonomousnotification.Policy{}, err
	}
	if cfg.RetryDelaySeconds != nil && base.RetryDelay > autonomousnotification.MaxRetryDelay {
		return autonomousnotification.Policy{}, fmt.Errorf("notifications.retry_delay_seconds must be at most %d", int64(autonomousnotification.MaxRetryDelay/time.Second))
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
		if *cfg.RecentRunCount < 0 {
			return artifactretention.Policy{}, errors.New("retention.recent_run_count must be nonnegative")
		}
		base.RecentRunCount = *cfg.RecentRunCount
	}
	if err := applyNonNegativeDuration("retention.compress_after_seconds", &base.CompressAfter, cfg.CompressAfterSeconds); err != nil {
		return artifactretention.Policy{}, err
	}
	if err := applyNonNegativeDuration("retention.prune_after_seconds", &base.PruneAfter, cfg.PruneAfterSeconds); err != nil {
		return artifactretention.Policy{}, err
	}
	if cfg.MinimumCompressBytes != nil {
		if *cfg.MinimumCompressBytes < 0 {
			return artifactretention.Policy{}, errors.New("retention.minimum_compress_bytes must be nonnegative")
		}
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
		if *cfg.MaxFilesPerOperation <= 0 {
			return artifactretention.Policy{}, errors.New("retention.max_files_per_operation must be positive")
		}
		base.MaxFilesPerOperation = *cfg.MaxFilesPerOperation
	}
	if cfg.MaxBytesPerOperation != nil {
		if *cfg.MaxBytesPerOperation <= 0 {
			return artifactretention.Policy{}, errors.New("retention.max_bytes_per_operation must be positive")
		}
		base.MaxBytesPerOperation = *cfg.MaxBytesPerOperation
	}
	if cfg.DecompressionCapBytes != nil {
		if *cfg.DecompressionCapBytes <= 0 {
			return artifactretention.Policy{}, errors.New("retention.decompression_cap_bytes must be positive")
		}
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

const maximumDurationSeconds int64 = int64((1<<63 - 1) / time.Second)

func applyPositiveDuration(field string, target *time.Duration, value *int64) error {
	if value == nil {
		return nil
	}
	if *value <= 0 {
		return fmt.Errorf("%s must be positive", field)
	}
	if *value > maximumDurationSeconds {
		return fmt.Errorf("%s overflows time.Duration", field)
	}
	*target = time.Duration(*value) * time.Second
	return nil
}

func applyNonNegativeDuration(field string, target *time.Duration, value *int64) error {
	if value == nil {
		return nil
	}
	if *value < 0 {
		return fmt.Errorf("%s must be nonnegative", field)
	}
	if *value > maximumDurationSeconds {
		return fmt.Errorf("%s overflows time.Duration", field)
	}
	*target = time.Duration(*value) * time.Second
	return nil
}

func applyPositiveInt(field string, target *int, value *int) error {
	if value == nil {
		return nil
	}
	if *value <= 0 {
		return fmt.Errorf("%s must be positive", field)
	}
	*target = *value
	return nil
}
