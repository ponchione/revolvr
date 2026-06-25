package receipt

import "time"

const SchemaVersion = "revolvr.receipt.v1"

type Verdict string

const (
	VerdictCompleted             Verdict = "completed"
	VerdictCompletedWithConcerns Verdict = "completed_with_concerns"
	VerdictBlocked               Verdict = "blocked"
	VerdictVerificationFailed    Verdict = "verification_failed"
	VerdictCodexFailed           Verdict = "codex_failed"
	VerdictSafetyLimit           Verdict = "safety_limit"
	VerdictNoChanges             Verdict = "no_changes"
)

type Receipt struct {
	SchemaVersion      string              `yaml:"schema_version"`
	RunID              string              `yaml:"run_id"`
	PassID             string              `yaml:"pass_id"`
	TaskID             string              `yaml:"task_id"`
	Task               string              `yaml:"task"`
	Verdict            Verdict             `yaml:"verdict"`
	Timestamp          time.Time           `yaml:"timestamp"`
	CodexExitCode      int                 `yaml:"codex_exit_code"`
	VerificationStatus string              `yaml:"verification_status"`
	CommitSHA          string              `yaml:"commit_sha"`
	ChangedFiles       []string            `yaml:"changed_files"`
	Verification       []VerificationEntry `yaml:"verification"`
	Metrics            Metrics             `yaml:"metrics"`
	RawBody            string              `yaml:"-"`
	ChangedFileClaims  []string            `yaml:"-"`
	VerificationClaims []VerificationClaim `yaml:"-"`
}

type VerificationEntry struct {
	Command  string `yaml:"command"`
	ExitCode int    `yaml:"exit_code"`
	Status   string `yaml:"status"`
}

type Metrics struct {
	InputTokens     int `yaml:"input_tokens"`
	OutputTokens    int `yaml:"output_tokens"`
	DurationSeconds int `yaml:"duration_seconds"`
}

type VerificationClaim struct {
	Command     string
	ExitCode    int
	HasExitCode bool
	Status      string
}
