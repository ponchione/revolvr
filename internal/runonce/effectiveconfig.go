package runonce

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"revolvr/internal/artifactretention"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousqueue"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/verification"
)

const (
	EffectiveConfigSchema   = "revolvr-effective-run-config-v8"
	OperationalBoundsSchema = "revolvr-attended-operational-bounds-v1"
)

const (
	DefaultTaskAttemptLimit       int64 = 16
	DefaultActionAttemptLimit     int64 = 4
	DefaultModelTokenLimit        int64 = 1_000_000
	DefaultTaskCycleLimit         int64 = 50
	DefaultAttendedProcessTimeout       = 30 * time.Minute
	DefaultAttendedElapsedLimit         = 4 * time.Hour
)

var attendedBoundedActions = []string{"audit", "correct", "document", "implement", "plan", "simplify"}

type ActionAttemptBound struct {
	Action   string `json:"action"`
	Attempts int64  `json:"attempts"`
}

// OperationalBounds is the finite Level-1 authority recorded in effective
// configuration and exact task-run evidence. Later readiness levels may
// require operators to configure these values explicitly.
type OperationalBounds struct {
	SchemaVersion        string               `json:"schema_version"`
	TaskAttempts         int64                `json:"task_attempts"`
	ActionAttempts       []ActionAttemptBound `json:"action_attempts"`
	Elapsed              time.Duration        `json:"elapsed"`
	ModelTokens          int64                `json:"model_tokens"`
	CyclesPerTask        int64                `json:"cycles_per_task"`
	ProcessDuration      time.Duration        `json:"process_duration"`
	OutputBytesPerStream int                  `json:"output_bytes_per_stream"`
	RetainedDiskBytes    int64                `json:"retained_disk_bytes"`
	NotificationAttempts int                  `json:"notification_attempts"`
}

func (b OperationalBounds) Validate(notificationsEnabled bool) error {
	if b.SchemaVersion != OperationalBoundsSchema || b.TaskAttempts <= 0 || b.Elapsed <= 0 || b.ModelTokens <= 0 || b.CyclesPerTask <= 0 || b.ProcessDuration <= 0 || b.OutputBytesPerStream <= 0 || b.RetainedDiskBytes <= 0 {
		return errors.New("operational bounds require the attended schema and finite positive task, elapsed, token, cycle, process, output, and disk limits")
	}
	if len(b.ActionAttempts) != len(attendedBoundedActions) {
		return errors.New("operational bounds require every attended action")
	}
	for i, action := range b.ActionAttempts {
		if action.Action != attendedBoundedActions[i] || strings.TrimSpace(action.Action) != action.Action || action.Attempts <= 0 {
			return errors.New("operational action bounds are incomplete or not canonical")
		}
	}
	if notificationsEnabled && b.NotificationAttempts <= 0 {
		return errors.New("enabled notifications require a finite positive attempt bound")
	}
	if !notificationsEnabled && b.NotificationAttempts != 0 {
		return errors.New("disabled notifications cannot retain attempt authority")
	}
	return nil
}

type EffectiveConfigFingerprint struct {
	Schema     string
	Projection EffectiveConfigProjection
	JSON       []byte
	SHA256     string
}

type EffectiveConfigProjection struct {
	Schema            string                        `json:"schema"`
	WorkingDir        string                        `json:"working_dir"`
	Codex             EffectiveCodexConfig          `json:"codex"`
	Git               EffectiveGitConfig            `json:"git"`
	Verification      EffectiveVerificationConfig   `json:"verification"`
	Commit            EffectiveCommitConfig         `json:"commit"`
	SourceWriterLock  EffectiveSourceWriterLock     `json:"source_writer_lock"`
	Autonomy          autonomoussafety.Declaration  `json:"autonomy"`
	Retention         artifactretention.Policy      `json:"retention"`
	Notifications     autonomousnotification.Policy `json:"notifications"`
	Queue             autonomousqueue.Policy        `json:"queue"`
	OperationalBounds OperationalBounds             `json:"operational_bounds"`
}

type EffectiveCodexConfig struct {
	Executable                string                            `json:"executable"`
	Model                     string                            `json:"model"`
	ReasoningEffort           string                            `json:"reasoning_effort"`
	Ephemeral                 bool                              `json:"ephemeral"`
	Sandbox                   string                            `json:"sandbox"`
	ApprovalPolicy            string                            `json:"approval_policy"`
	BypassApprovalsAndSandbox bool                              `json:"bypass_approvals_and_sandbox"`
	Timeout                   time.Duration                     `json:"timeout"`
	StdoutCap                 int                               `json:"stdout_cap"`
	StderrCap                 int                               `json:"stderr_cap"`
	Identity                  codexexec.CodexExecutableIdentity `json:"identity"`
}

type EffectiveGitConfig struct {
	Executable string                       `json:"executable"`
	Timeout    time.Duration                `json:"timeout"`
	StdoutCap  int                          `json:"stdout_cap"`
	StderrCap  int                          `json:"stderr_cap"`
	Identity   codexexec.ExecutableIdentity `json:"identity"`
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
			Identity:                  normalized.CodexIdentity,
		},
		Git: EffectiveGitConfig{
			Executable: normalized.GitExecutable,
			Timeout:    normalized.GitTimeout,
			StdoutCap:  normalized.GitStdoutCap,
			StderrCap:  normalized.GitStderrCap,
			Identity:   normalized.GitIdentity,
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
		Autonomy:          normalized.SafetyDeclaration,
		Retention:         normalized.RetentionPolicy,
		Notifications:     normalized.NotificationPolicy,
		Queue:             normalized.QueuePolicy,
		OperationalBounds: cloneOperationalBounds(normalized.OperationalBounds),
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

func cloneOperationalBounds(bounds OperationalBounds) OperationalBounds {
	bounds.ActionAttempts = append([]ActionAttemptBound(nil), bounds.ActionAttempts...)
	return bounds
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
