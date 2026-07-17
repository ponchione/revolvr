package codexexec

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

const (
	DefaultExecutable      = "codex"
	DefaultModel           = "gpt-5.6-sol"
	DefaultReasoningEffort = "xhigh"
	SessionModeEphemeral   = "ephemeral"
)

type InvocationConfig struct {
	Executable             string
	WorkingDir             string
	ArtifactRoot           string
	Model                  string
	ReasoningEffort        string
	Ephemeral              bool
	Sandbox                string
	ApprovalPolicy         string
	BypassApprovalsSandbox bool
	Artifacts              ArtifactPaths
	OutputSchema           string
	CodexVersion           string
	EffectiveConfigSchema  string
	EffectiveConfigSHA256  string
	SafetyPolicySHA256     string
	CodexIdentity          CodexExecutableIdentity
	GitIdentity            ExecutableIdentity
}

type InvocationProvenance struct {
	Executable            string                   `json:"executable"`
	Version               string                   `json:"version"`
	Model                 string                   `json:"model"`
	ReasoningEffort       string                   `json:"reasoning_effort"`
	Ephemeral             bool                     `json:"ephemeral"`
	SessionMode           string                   `json:"session_mode"`
	EffectiveConfigSchema string                   `json:"effective_config_schema"`
	EffectiveConfigSHA256 string                   `json:"effective_config_sha256"`
	SafetyPolicySHA256    string                   `json:"safety_policy_sha256,omitempty"`
	Argv                  []string                 `json:"argv"`
	WorkingDir            string                   `json:"working_dir"`
	CodexIdentity         *CodexExecutableIdentity `json:"codex_identity,omitempty"`
	GitIdentity           *ExecutableIdentity      `json:"git_identity,omitempty"`
}

func PrepareInvocation(cfg InvocationConfig) (InvocationProvenance, ArtifactPaths, error) {
	executable := strings.TrimSpace(cfg.Executable)
	if executable == "" {
		executable = DefaultExecutable
	}
	workDir, err := absoluteWorkingDir(cfg.WorkingDir)
	if err != nil {
		return InvocationProvenance{}, ArtifactPaths{}, err
	}
	artifactRoot := workDir
	if strings.TrimSpace(cfg.ArtifactRoot) != "" {
		artifactRoot, err = absoluteWorkingDir(cfg.ArtifactRoot)
		if err != nil {
			return InvocationProvenance{}, ArtifactPaths{}, fmt.Errorf("resolve Codex artifact root: %w", err)
		}
	}
	model, err := NormalizeModel(cfg.Model)
	if err != nil {
		return InvocationProvenance{}, ArtifactPaths{}, err
	}
	effort, err := NormalizeReasoningEffort(cfg.ReasoningEffort)
	if err != nil {
		return InvocationProvenance{}, ArtifactPaths{}, err
	}
	if !cfg.Ephemeral {
		return InvocationProvenance{}, ArtifactPaths{}, errors.New("prepare Codex invocation: only ephemeral sessions are supported")
	}
	artifacts, err := resolveArtifacts(artifactRoot, cfg.Artifacts)
	if err != nil {
		return InvocationProvenance{}, ArtifactPaths{}, err
	}
	outputSchema, err := resolveOutputSchema(artifactRoot, cfg.OutputSchema)
	if err != nil {
		return InvocationProvenance{}, ArtifactPaths{}, err
	}

	ephemeral := true
	normalized := Config{
		Model:                     model,
		ReasoningEffort:           effort,
		Ephemeral:                 &ephemeral,
		Sandbox:                   strings.TrimSpace(cfg.Sandbox),
		ApprovalPolicy:            strings.TrimSpace(cfg.ApprovalPolicy),
		BypassApprovalsAndSandbox: cfg.BypassApprovalsSandbox,
	}
	invocationArtifacts := artifacts
	if invocationArtifacts.LastMessage != "" {
		invocationArtifacts.LastMessage = lastMessageRawPath(invocationArtifacts.LastMessage)
	}
	argv := buildArgs(workDir, normalized, invocationArtifacts, outputSchema)
	return InvocationProvenance{
		Executable:            executable,
		Version:               strings.TrimSpace(cfg.CodexVersion),
		Model:                 model,
		ReasoningEffort:       effort,
		Ephemeral:             true,
		SessionMode:           SessionModeEphemeral,
		EffectiveConfigSchema: strings.TrimSpace(cfg.EffectiveConfigSchema),
		EffectiveConfigSHA256: strings.TrimSpace(cfg.EffectiveConfigSHA256),
		SafetyPolicySHA256:    strings.TrimSpace(cfg.SafetyPolicySHA256),
		Argv:                  argv,
		WorkingDir:            workDir,
		CodexIdentity:         codexIdentityPointer(cfg.CodexIdentity),
		GitIdentity:           executableIdentityPointer(cfg.GitIdentity),
	}, artifacts, nil
}

func resolveOutputSchema(workDir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	resolved, err := resolveArtifactPath(workDir, path)
	if err != nil {
		return "", fmt.Errorf("resolve output schema: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect output schema: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("inspect output schema: path must name a regular file")
	}
	if info.Size() == 0 {
		return "", errors.New("inspect output schema: file is empty")
	}
	return resolved, nil
}

func NormalizeModel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("Codex model is required")
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("invalid Codex model %q: whitespace and control characters are not allowed", value)
	}
	return value, nil
}

func NormalizeReasoningEffort(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("Codex reasoning effort is required")
	}
	switch value {
	case "minimal", "low", "medium", "high", "xhigh":
		return value, nil
	default:
		return "", fmt.Errorf("invalid Codex reasoning effort %q (want minimal, low, medium, high, or xhigh)", value)
	}
}

func (p InvocationProvenance) Validate() error {
	if strings.TrimSpace(p.Executable) == "" {
		return errors.New("Codex invocation provenance: executable is required")
	}
	if strings.TrimSpace(p.Version) == "" {
		return errors.New("Codex invocation provenance: version is required")
	}
	if p.CodexIdentity != nil {
		if err := p.CodexIdentity.Validate(); err != nil {
			return fmt.Errorf("Codex invocation provenance: %w", err)
		}
		if p.Version != p.CodexIdentity.Version || p.Executable != p.CodexIdentity.Executable.Configured {
			return errors.New("Codex invocation provenance: executable/version do not match admitted Codex identity")
		}
	}
	if p.GitIdentity != nil {
		if err := p.GitIdentity.Validate(); err != nil {
			return fmt.Errorf("Codex invocation provenance: Git %w", err)
		}
	}
	model, err := NormalizeModel(p.Model)
	if err != nil {
		return fmt.Errorf("Codex invocation provenance: %w", err)
	}
	effort, err := NormalizeReasoningEffort(p.ReasoningEffort)
	if err != nil {
		return fmt.Errorf("Codex invocation provenance: %w", err)
	}
	if model != p.Model || effort != p.ReasoningEffort {
		return errors.New("Codex invocation provenance: model and reasoning effort must be normalized")
	}
	if !p.Ephemeral || p.SessionMode != SessionModeEphemeral {
		return errors.New("Codex invocation provenance: only ephemeral sessions are supported")
	}
	if strings.TrimSpace(p.EffectiveConfigSchema) == "" || strings.TrimSpace(p.EffectiveConfigSHA256) == "" {
		return errors.New("Codex invocation provenance: effective config schema and SHA-256 are required")
	}
	if decoded, err := hex.DecodeString(p.EffectiveConfigSHA256); err != nil || len(decoded) != sha256.Size {
		return errors.New("Codex invocation provenance: effective config SHA-256 must be 64 hexadecimal characters")
	}
	if p.SafetyPolicySHA256 != "" {
		if decoded, err := hex.DecodeString(p.SafetyPolicySHA256); err != nil || len(decoded) != sha256.Size {
			return errors.New("Codex invocation provenance: safety policy SHA-256 must be 64 hexadecimal characters")
		}
	}
	if len(p.Argv) == 0 || !slices.Contains(p.Argv, "exec") || slices.Contains(p.Argv, "resume") {
		return errors.New("Codex invocation provenance: argv must contain exec and must not contain resume")
	}
	if strings.TrimSpace(p.WorkingDir) == "" || !filepath.IsAbs(p.WorkingDir) {
		return errors.New("Codex invocation provenance: absolute working directory is required")
	}
	return nil
}

func sameInvocation(got, want InvocationProvenance) bool {
	return got.Executable == want.Executable &&
		got.Version == want.Version &&
		got.Model == want.Model &&
		got.ReasoningEffort == want.ReasoningEffort &&
		got.Ephemeral == want.Ephemeral &&
		got.SessionMode == want.SessionMode &&
		got.EffectiveConfigSchema == want.EffectiveConfigSchema &&
		got.EffectiveConfigSHA256 == want.EffectiveConfigSHA256 &&
		got.SafetyPolicySHA256 == want.SafetyPolicySHA256 &&
		slices.Equal(got.Argv, want.Argv) &&
		got.WorkingDir == want.WorkingDir &&
		reflect.DeepEqual(got.CodexIdentity, want.CodexIdentity) &&
		reflect.DeepEqual(got.GitIdentity, want.GitIdentity)
}

func codexIdentityPointer(identity CodexExecutableIdentity) *CodexExecutableIdentity {
	if identity == (CodexExecutableIdentity{}) {
		return nil
	}
	copy := identity
	return &copy
}

func executableIdentityPointer(identity ExecutableIdentity) *ExecutableIdentity {
	if identity == (ExecutableIdentity{}) {
		return nil
	}
	copy := identity
	return &copy
}

func absoluteWorkingDir(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("prepare Codex invocation: working directory is required")
	}
	workDir, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return workDir, nil
}
