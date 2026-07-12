package runonce

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"revolvr/internal/autonomousverification"
	"revolvr/internal/verification"
)

const EffectiveConfigSchema = "revolvr-effective-run-config-v1"

type EffectiveConfigFingerprint struct {
	Schema     string
	Projection EffectiveConfigProjection
	JSON       []byte
	SHA256     string
}

type EffectiveConfigProjection struct {
	Schema           string                      `json:"schema"`
	WorkingDir       string                      `json:"working_dir"`
	Codex            EffectiveCodexConfig        `json:"codex"`
	Git              EffectiveGitConfig          `json:"git"`
	Verification     EffectiveVerificationConfig `json:"verification"`
	Commit           EffectiveCommitConfig       `json:"commit"`
	SourceWriterLock EffectiveSourceWriterLock   `json:"source_writer_lock"`
}

type EffectiveCodexConfig struct {
	Executable                string        `json:"executable"`
	Model                     string        `json:"model"`
	ReasoningEffort           string        `json:"reasoning_effort"`
	Ephemeral                 bool          `json:"ephemeral"`
	Sandbox                   string        `json:"sandbox"`
	ApprovalPolicy            string        `json:"approval_policy"`
	BypassApprovalsAndSandbox bool          `json:"bypass_approvals_and_sandbox"`
	Timeout                   time.Duration `json:"timeout"`
	StdoutCap                 int           `json:"stdout_cap"`
	StderrCap                 int           `json:"stderr_cap"`
}

type EffectiveGitConfig struct {
	Executable string        `json:"executable"`
	Timeout    time.Duration `json:"timeout"`
	StdoutCap  int           `json:"stdout_cap"`
	StderrCap  int           `json:"stderr_cap"`
}

type EffectiveVerificationConfig struct {
	MissingPolicy verification.MissingCommandsPolicy `json:"missing_policy"`
	Timeout       time.Duration                      `json:"timeout"`
	StdoutCap     int                                `json:"stdout_cap"`
	StderrCap     int                                `json:"stderr_cap"`
	Commands      []EffectiveVerificationCommand     `json:"commands"`
	Plan          *autonomousverification.Plan       `json:"tiered_plan,omitempty"`
}

type EffectiveVerificationCommand struct {
	Name      string        `json:"name"`
	Args      []string      `json:"args"`
	Dir       string        `json:"dir"`
	Env       []string      `json:"env"`
	Timeout   time.Duration `json:"timeout"`
	StdoutCap int           `json:"stdout_cap"`
	StderrCap int           `json:"stderr_cap"`
}

type EffectiveCommitConfig struct {
	AllowPreExistingDirty    bool          `json:"allow_pre_existing_dirty"`
	AllowMissingVerification bool          `json:"allow_missing_verification"`
	Timeout                  time.Duration `json:"timeout"`
	StdoutCap                int           `json:"stdout_cap"`
	StderrCap                int           `json:"stderr_cap"`
}

type EffectiveSourceWriterLock struct {
	Timeout           time.Duration `json:"timeout"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

func FingerprintEffectiveConfig(cfg Config) (EffectiveConfigFingerprint, error) {
	normalized, _, err := normalizeConfig(cfg)
	if err != nil {
		return EffectiveConfigFingerprint{}, err
	}
	commands := make([]EffectiveVerificationCommand, 0, len(normalized.VerificationCommands))
	for _, command := range normalized.VerificationCommands {
		commands = append(commands, EffectiveVerificationCommand{
			Name:      command.Name,
			Args:      append([]string(nil), command.Args...),
			Dir:       command.Dir,
			Env:       append([]string(nil), command.Env...),
			Timeout:   command.Timeout,
			StdoutCap: command.StdoutCap,
			StderrCap: command.StderrCap,
		})
	}
	projection := EffectiveConfigProjection{
		Schema:     EffectiveConfigSchema,
		WorkingDir: normalized.WorkingDir,
		Codex: EffectiveCodexConfig{
			Executable:                normalized.CodexExecutable,
			Model:                     normalized.CodexModel,
			ReasoningEffort:           normalized.CodexReasoningEffort,
			Ephemeral:                 normalized.CodexEphemeral,
			Sandbox:                   normalized.CodexSandbox,
			ApprovalPolicy:            normalized.CodexApprovalPolicy,
			BypassApprovalsAndSandbox: normalized.CodexBypassApprovalsAndSandbox,
			Timeout:                   normalized.CodexTimeout,
			StdoutCap:                 normalized.CodexStdoutCap,
			StderrCap:                 normalized.CodexStderrCap,
		},
		Git: EffectiveGitConfig{
			Executable: normalized.GitExecutable,
			Timeout:    normalized.GitTimeout,
			StdoutCap:  normalized.GitStdoutCap,
			StderrCap:  normalized.GitStderrCap,
		},
		Verification: EffectiveVerificationConfig{
			MissingPolicy: normalized.MissingVerificationPolicy,
			Timeout:       normalized.VerificationTimeout,
			StdoutCap:     normalized.VerificationStdoutCap,
			StderrCap:     normalized.VerificationStderrCap,
			Commands:      commands,
			Plan:          cloneEffectivePlan(normalized.VerificationPlan),
		},
		Commit: EffectiveCommitConfig{
			AllowPreExistingDirty:    normalized.AllowPreExistingDirty,
			AllowMissingVerification: normalized.AllowMissingVerification,
			Timeout:                  normalized.CommitTimeout,
			StdoutCap:                normalized.CommitStdoutCap,
			StderrCap:                normalized.CommitStderrCap,
		},
		SourceWriterLock: EffectiveSourceWriterLock{
			Timeout:           normalized.SourceWriterLockTimeout,
			HeartbeatInterval: normalized.SourceWriterLockHeartbeatInterval,
		},
	}
	raw, hash, err := fingerprintProjection(projection)
	if err != nil {
		return EffectiveConfigFingerprint{}, err
	}
	return EffectiveConfigFingerprint{
		Schema:     EffectiveConfigSchema,
		Projection: projection,
		JSON:       append([]byte(nil), raw...),
		SHA256:     hash,
	}, nil
}

func cloneEffectivePlan(plan *autonomousverification.Plan) *autonomousverification.Plan {
	if plan == nil {
		return nil
	}
	cloned := autonomousverification.ClonePlan(*plan)
	return &cloned
}

func fingerprintProjection(projection EffectiveConfigProjection) ([]byte, string, error) {
	raw, err := json.Marshal(projection)
	if err != nil {
		return nil, "", fmt.Errorf("marshal effective run config: %w", err)
	}
	sum := sha256.Sum256(raw)
	return raw, fmt.Sprintf("%x", sum), nil
}
