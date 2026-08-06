// Package implementer owns the fresh, bounded implementer loop. Its output is
// advisory evidence: it cannot advance a task, plan, criterion, verification,
// or completion state.
package implementer

import (
	"context"
	"encoding/json"
	"time"

	"revolvr/internal/gitstate"
	"revolvr/internal/model"
	"revolvr/internal/planner"
	"revolvr/internal/tool"
)

const (
	AdmissionSchemaVersion = "revolvr-implementer-admission-v1"
	PromptVersion          = "revolvr-implementer-prompt-v1"
	SummarySchemaVersion   = "revolvr-implementer-summary-v1"
	SummarySchemaName      = "revolvr_implementer_summary_v1"
	ModelPolicyVersion     = "revolvr-implementer-model-policy-v1"
	InvocationVersion      = "revolvr-implementer-invocation-v1"
	EvidenceVersion        = "revolvr-implementer-evidence-v1"

	MaximumBatchSteps = 4
)

type Admission struct {
	SchemaVersion      string         `json:"schema_version"`
	Accepted           bool           `json:"accepted"`
	AcceptanceID       string         `json:"acceptance_id"`
	ProjectID          string         `json:"project_id"`
	TaskID             string         `json:"task_id"`
	TaskVersionID      string         `json:"task_version_id"`
	RunID              string         `json:"run_id"`
	ProjectSourceID    string         `json:"project_source_id"`
	SourceRevision     string         `json:"source_revision"`
	SourceCommit       string         `json:"source_commit"`
	SourceTree         string         `json:"source_tree"`
	PlanID             string         `json:"plan_id"`
	PlanVersionID      string         `json:"plan_version_id"`
	PlanRevision       int            `json:"plan_revision"`
	StepBatchSHA256    string         `json:"step_batch_sha256"`
	PlanAccepted       bool           `json:"plan_accepted"`
	PlanAcceptanceID   string         `json:"plan_acceptance_id"`
	ActiveSteps        []planner.Step `json:"active_steps"`
	WorkspaceID        string         `json:"workspace_id"`
	WorkspaceRoot      string         `json:"workspace_root"`
	WorkspaceDevice    uint64         `json:"workspace_device"`
	WorkspaceInode     uint64         `json:"workspace_inode"`
	WorkspaceStatus    string         `json:"workspace_status"`
	SandboxID          string         `json:"sandbox_id"`
	SandboxSHA256      string         `json:"sandbox_sha256"`
	HostPolicyVersion  string         `json:"host_policy_version"`
	HostPolicySHA256   string         `json:"host_policy_sha256"`
	RegistryVersion    string         `json:"registry_version"`
	RegistrySHA256     string         `json:"registry_sha256"`
	ModelPolicyVersion string         `json:"model_policy_version"`
	ModelPolicySHA256  string         `json:"model_policy_sha256"`
	ExpectedPaths      []string       `json:"expected_paths"`
	AdjacentPaths      []string       `json:"adjacent_paths"`
	ProtectedPaths     []string       `json:"protected_paths"`
	DependencyPaths    []string       `json:"dependency_paths"`
	VerificationPaths  []string       `json:"verification_authority_paths"`
}

type ModelPolicy struct {
	Version           string `json:"version"`
	SHA256            string `json:"sha256"`
	Model             string `json:"model"`
	MaximumIterations int    `json:"maximum_iterations"`
	MaximumToolCalls  int    `json:"maximum_tool_calls"`
	MaximumFinalBytes int    `json:"maximum_final_bytes"`
	FreshSession      bool   `json:"fresh_session"`
	HiddenState       bool   `json:"hidden_state"`
}

type SummaryIdentity struct {
	ProjectID            string `json:"project_id"`
	TaskID               string `json:"task_id"`
	TaskVersionID        string `json:"task_version_id"`
	RunID                string `json:"run_id"`
	SourceRevision       string `json:"source_revision"`
	SourceCommit         string `json:"source_commit"`
	SourceTree           string `json:"source_tree"`
	PlanID               string `json:"plan_id"`
	PlanVersionID        string `json:"plan_version_id"`
	PlanRevision         int    `json:"plan_revision"`
	StepBatchSHA256      string `json:"step_batch_sha256"`
	WorkspaceID          string `json:"workspace_id"`
	SandboxID            string `json:"sandbox_id"`
	SandboxSHA256        string `json:"sandbox_sha256"`
	PromptVersion        string `json:"prompt_version"`
	PromptSHA256         string `json:"prompt_sha256"`
	SummarySchemaVersion string `json:"summary_schema_version"`
	SummarySchemaSHA256  string `json:"summary_schema_sha256"`
	RegistryVersion      string `json:"registry_version"`
	RegistrySHA256       string `json:"registry_sha256"`
	HostPolicyVersion    string `json:"host_policy_version"`
	HostPolicySHA256     string `json:"host_policy_sha256"`
	ModelPolicyVersion   string `json:"model_policy_version"`
	ModelPolicySHA256    string `json:"model_policy_sha256"`
}

type VoluntaryTest struct {
	ToolCallID string `json:"tool_call_id"`
	Outcome    string `json:"outcome"`
}

type PlanProgress struct {
	StepID          string   `json:"step_id"`
	Status          string   `json:"status"`
	EvidenceCallIDs []string `json:"evidence_call_ids"`
}

type Summary struct {
	SchemaVersion         string          `json:"schema_version"`
	Identity              SummaryIdentity `json:"identity"`
	Summary               string          `json:"summary"`
	ClaimedFiles          []string        `json:"claimed_files"`
	VoluntaryTests        []VoluntaryTest `json:"voluntary_tests"`
	Concerns              []string        `json:"concerns"`
	CandidatePlanProgress []PlanProgress  `json:"candidate_plan_progress"`
	CandidateFollowUpWork []string        `json:"candidate_follow_up_work"`
	Partial               bool            `json:"partial"`
}

type HistoryItem struct {
	Kind        string          `json:"kind"`
	Iteration   int             `json:"iteration"`
	ToolCall    json.RawMessage `json:"tool_call,omitempty"`
	ToolOutcome *tool.Outcome   `json:"tool_outcome,omitempty"`
}

type ModelRequest struct {
	SchemaVersion string          `json:"schema_version"`
	InvocationID  string          `json:"invocation_id"`
	Iteration     int             `json:"iteration"`
	FreshSession  bool            `json:"fresh_session"`
	PromptVersion string          `json:"prompt_version"`
	PromptSHA256  string          `json:"prompt_sha256"`
	Prompt        string          `json:"prompt"`
	SummarySchema json.RawMessage `json:"summary_schema"`
	Registry      tool.Registry   `json:"registry"`
	Authority     tool.Authority  `json:"authority"`
	History       []HistoryItem   `json:"history"`
}

type ModelTurn struct {
	ToolCalls   []json.RawMessage   `json:"tool_calls,omitempty"`
	FinalOutput json.RawMessage     `json:"final_output,omitempty"`
	Refusal     string              `json:"refusal,omitempty"`
	Usage       model.UsageEvidence `json:"usage"`
}

type Model interface {
	Next(context.Context, ModelRequest) (ModelTurn, error)
}

type ModelIterationEvidence struct {
	Iteration  int                 `json:"iteration"`
	StartedAt  time.Time           `json:"started_at"`
	FinishedAt time.Time           `json:"finished_at"`
	Duration   time.Duration       `json:"duration_ns"`
	Request    tool.Artifact       `json:"request"`
	Response   tool.Artifact       `json:"response"`
	Usage      model.UsageEvidence `json:"usage"`
	Error      string              `json:"error,omitempty"`
}

type Change struct {
	Status  string `json:"status"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
}

type WorkspaceObservation struct {
	SourceSnapshot  gitstate.SourceSnapshot `json:"source_snapshot"`
	SourceRevision  string                  `json:"source_revision"`
	HeadCommit      string                  `json:"head_commit"`
	HeadTree        string                  `json:"head_tree"`
	RawStatus       []byte                  `json:"-"`
	ChangedManifest []Change                `json:"changed_manifest"`
	Diff            []byte                  `json:"-"`
	DiffSHA256      string                  `json:"diff_sha256"`
}

type Observer interface {
	Capture(context.Context, string) (WorkspaceObservation, error)
}

type SignalKind string

const (
	SignalAdjacentChange                SignalKind = "adjacent_change"
	SignalUnexpectedChange              SignalKind = "unexpected_change"
	SignalProtectedChange               SignalKind = "protected_change"
	SignalDependencyChange              SignalKind = "dependency_change"
	SignalVerificationAuthorityMutation SignalKind = "verification_authority_mutation"
	SignalClaimedActualMismatch         SignalKind = "claimed_actual_mismatch"
	SignalToolActualMismatch            SignalKind = "tool_actual_mismatch"
	SignalNoSourceChange                SignalKind = "no_source_change"
	SignalPartialWork                   SignalKind = "partial_work"
	SignalCancellation                  SignalKind = "cancellation"
)

type PolicySignal struct {
	Kind   SignalKind `json:"kind"`
	Paths  []string   `json:"paths,omitempty"`
	Detail string     `json:"detail"`
}

type SourceEvidence struct {
	BeforeSnapshot tool.Artifact `json:"before_snapshot"`
	AfterSnapshot  tool.Artifact `json:"after_snapshot"`
	Status         tool.Artifact `json:"status"`
	Manifest       tool.Artifact `json:"manifest"`
	Diff           tool.Artifact `json:"diff"`
	DiffSHA256     string        `json:"diff_sha256"`
}

type Result struct {
	SchemaVersion        string                    `json:"schema_version"`
	InvocationID         string                    `json:"invocation_id"`
	Disposition          string                    `json:"disposition"`
	Admission            Admission                 `json:"admission"`
	PromptVersion        string                    `json:"prompt_version"`
	PromptSHA256         string                    `json:"prompt_sha256"`
	SummarySchemaVersion string                    `json:"summary_schema_version"`
	SummarySchemaSHA256  string                    `json:"summary_schema_sha256"`
	ModelPolicy          ModelPolicy               `json:"model_policy"`
	ModelIterations      []ModelIterationEvidence  `json:"model_iterations"`
	ToolExecutions       []tool.Evidence           `json:"tool_executions"`
	Summary              *Summary                  `json:"summary,omitempty"`
	RawSummary           tool.Artifact             `json:"raw_summary"`
	Source               SourceEvidence            `json:"source"`
	Before               WorkspaceObservation      `json:"before"`
	After                WorkspaceObservation      `json:"after"`
	Signals              []PolicySignal            `json:"policy_signals"`
	Cancellation         tool.CancellationEvidence `json:"cancellation"`
	Error                string                    `json:"error,omitempty"`
	Replayed             bool                      `json:"replayed"`
}

type Config struct {
	Admission    Admission
	ModelPolicy  ModelPolicy
	Model        Model
	Broker       *tool.Broker
	Observer     Observer
	EvidenceRoot string
	Clock        func() time.Time
}
