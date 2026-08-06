// Package tool implements the trusted, role-scoped broker between a fresh
// model invocation and one already-admitted sandbox workspace.
package tool

import (
	"context"
	"encoding/json"
	"time"

	"revolvr/internal/sandbox"
)

const (
	CallSchemaVersion     = "revolvr-tool-call-v1"
	RegistryVersion       = "revolvr-tool-registry-v1"
	HostPolicyVersion     = "revolvr-tool-host-policy-v1"
	EvidenceSchemaVersion = "revolvr-tool-execution-evidence-v2"

	ToolFileRead   = "file_read"
	ToolTextSearch = "text_search"
	ToolSourceEdit = "source_edit"
	ToolCommand    = "command"
)

// RuntimeKind is a closed host-owned execution discriminator. Adding another
// value is an architecture change; a model cannot select or extend it.
type RuntimeKind string

const RuntimeDirectToolsV1 RuntimeKind = "direct_tools_v1"

type SequenceRequest struct {
	RuntimeKind   RuntimeKind `json:"runtime_kind"`
	RunID         string      `json:"run_id"`
	RequestSHA256 string      `json:"request_sha256"`
}

type SequenceGrant struct {
	Sequence      int64       `json:"sequence"`
	RuntimeKind   RuntimeKind `json:"runtime_kind"`
	RunID         string      `json:"run_id"`
	RequestSHA256 string      `json:"request_sha256"`
	Trusted       bool        `json:"trusted"`
}

// TrajectorySequencer is supplied by the trusted host. Runtime handlers and
// model calls never assign canonical ordering.
type TrajectorySequencer interface {
	Next(context.Context, SequenceRequest) (SequenceGrant, error)
}

type Capability string

const (
	CapabilityRead    Capability = "read_source"
	CapabilitySearch  Capability = "search_source"
	CapabilityWrite   Capability = "write_source"
	CapabilityCommand Capability = "run_command"
)

type Definition struct {
	Name          string          `json:"name"`
	SchemaVersion string          `json:"schema_version"`
	Capability    Capability      `json:"capability"`
	MutatesSource bool            `json:"mutates_source"`
	InputSchema   json.RawMessage `json:"input_schema"`
}

type Registry struct {
	Version     string       `json:"version"`
	SHA256      string       `json:"sha256"`
	Role        sandbox.Role `json:"role"`
	Definitions []Definition `json:"definitions"`
}

type Authority struct {
	ProjectID         string   `json:"project_id"`
	TaskID            string   `json:"task_id"`
	TaskVersionID     string   `json:"task_version_id"`
	RunID             string   `json:"run_id"`
	SourceRevision    string   `json:"source_revision"`
	SourceCommit      string   `json:"source_commit"`
	SourceTree        string   `json:"source_tree"`
	PlanID            string   `json:"plan_id"`
	PlanVersionID     string   `json:"plan_version_id"`
	PlanRevision      int      `json:"plan_revision"`
	StepBatchSHA256   string   `json:"step_batch_sha256"`
	StepIDs           []string `json:"step_ids"`
	WorkspaceID       string   `json:"workspace_id"`
	SandboxID         string   `json:"sandbox_id"`
	SandboxSHA256     string   `json:"sandbox_sha256"`
	RegistryVersion   string   `json:"registry_version"`
	RegistrySHA256    string   `json:"registry_sha256"`
	HostPolicyVersion string   `json:"host_policy_version"`
	HostPolicySHA256  string   `json:"host_policy_sha256"`
}

type Call struct {
	SchemaVersion string          `json:"schema_version"`
	CallID        string          `json:"call_id"`
	Tool          string          `json:"tool"`
	Authority     Authority       `json:"authority"`
	Arguments     json.RawMessage `json:"arguments"`
}

type FileReadArguments struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset"`
	MaxBytes int64  `json:"max_bytes"`
}

type TextSearchArguments struct {
	Query          string   `json:"query"`
	Paths          []string `json:"paths"`
	MaximumResults int      `json:"maximum_results"`
	OutputCapBytes int64    `json:"output_cap_bytes"`
}

type SourceEditArguments struct {
	Path           string `json:"path"`
	ExpectedSHA256 string `json:"expected_sha256"`
	Content        string `json:"content"`
}

type CommandArguments struct {
	Argv                []string               `json:"argv"`
	WorkingDirectory    string                 `json:"working_directory"`
	EnvironmentNames    []string               `json:"environment_names"`
	Network             sandbox.NetworkProfile `json:"network"`
	TimeoutMilliseconds int64                  `json:"timeout_milliseconds"`
	CPUs                int64                  `json:"cpus"`
	MemoryBytes         int64                  `json:"memory_bytes"`
	PIDs                int64                  `json:"pids"`
	TmpfsBytes          int64                  `json:"tmpfs_bytes"`
	StdoutCapBytes      int64                  `json:"stdout_cap_bytes"`
	StderrCapBytes      int64                  `json:"stderr_cap_bytes"`
}

type Operation struct {
	Tool       string
	FileRead   *FileReadArguments
	TextSearch *TextSearchArguments
	SourceEdit *SourceEditArguments
	Command    *CommandArguments
}

type SourceChange struct {
	Path         string `json:"path"`
	BeforeSHA256 string `json:"before_sha256,omitempty"`
	AfterSHA256  string `json:"after_sha256,omitempty"`
}

type EffectProof struct {
	Proven       bool   `json:"proven"`
	Kind         string `json:"kind,omitempty"`
	Identity     string `json:"identity,omitempty"`
	BeforeSHA256 string `json:"before_sha256,omitempty"`
	AfterSHA256  string `json:"after_sha256,omitempty"`
}

type ExecutionResult struct {
	ExitCode             int            `json:"exit_code"`
	Stdout               []byte         `json:"-"`
	Stderr               []byte         `json:"-"`
	StdoutTruncatedBytes int64          `json:"stdout_truncated_bytes"`
	StderrTruncatedBytes int64          `json:"stderr_truncated_bytes"`
	TimedOut             bool           `json:"timed_out"`
	Cancelled            bool           `json:"cancelled"`
	SourceChanges        []SourceChange `json:"source_changes,omitempty"`
	Effect               EffectProof    `json:"effect"`
}

// SandboxExecutor has no host-policy or credential authority. It receives
// only a normalized sandbox specification and an operation already admitted
// by Broker.
type SandboxExecutor interface {
	Execute(context.Context, sandbox.Specification, Operation) (ExecutionResult, error)
	Cancel(context.Context, string) error
}

// RuntimeExecutionRequest is the narrow host-to-handler boundary. It carries
// only already-validated authority, sandbox, policy, and operation data.
type RuntimeExecutionRequest struct {
	RuntimeKind        RuntimeKind           `json:"runtime_kind"`
	TrajectorySequence int64                 `json:"trajectory_sequence"`
	RequestSHA256      string                `json:"request_sha256"`
	Call               Call                  `json:"call"`
	Operation          Operation             `json:"-"`
	Sandbox            sandbox.Specification `json:"sandbox"`
	HostPolicyVersion  string                `json:"host_policy_version"`
	HostPolicySHA256   string                `json:"host_policy_sha256"`
}

// RuntimeHandler has no lifecycle, registry, model, PostgreSQL, or credential
// authority. Broker validation and evidence normalization surround every
// handler implementation.
type RuntimeHandler interface {
	Kind() RuntimeKind
	Execute(context.Context, RuntimeExecutionRequest) (ExecutionResult, error)
	Cancel(context.Context, string) error
}

type Artifact struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type,omitempty"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type ResultRepresentationKind string

const (
	ResultRepresentationInline   ResultRepresentationKind = "inline"
	ResultRepresentationArtifact ResultRepresentationKind = "artifacts"
)

type InlineResult struct {
	MediaType string `json:"media_type"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type ArtifactReference struct {
	Artifact  Artifact `json:"artifact"`
	Immutable bool     `json:"immutable"`
}

// ResultRepresentation is an exclusive union: exactly one bounded inline
// value or one-or-more immutable content-addressed artifacts.
type ResultRepresentation struct {
	Kind           ResultRepresentationKind `json:"kind"`
	Inline         *InlineResult            `json:"inline,omitempty"`
	Artifacts      []ArtifactReference      `json:"artifacts,omitempty"`
	MediaType      string                   `json:"media_type"`
	SHA256         string                   `json:"sha256"`
	SizeBytes      int64                    `json:"size_bytes"`
	Truncated      bool                     `json:"truncated"`
	TruncatedBytes int64                    `json:"truncated_bytes"`
	Resolution     string                   `json:"resolution"`
}

type RuntimeEvidence struct {
	Image             sandbox.Image          `json:"image"`
	Profile           sandbox.RuntimeProfile `json:"profile"`
	Network           sandbox.NetworkProfile `json:"network"`
	Resources         sandbox.Resources      `json:"resources"`
	SandboxSHA256     string                 `json:"sandbox_sha256"`
	HostPolicyVersion string                 `json:"host_policy_version"`
	HostPolicySHA256  string                 `json:"host_policy_sha256"`
}

type CancellationEvidence struct {
	Requested     bool   `json:"requested"`
	StopAttempted bool   `json:"stop_attempted"`
	StopSucceeded bool   `json:"stop_succeeded"`
	Error         string `json:"error,omitempty"`
}

type Evidence struct {
	SchemaVersion        string               `json:"schema_version"`
	RuntimeKind          RuntimeKind          `json:"runtime_kind"`
	TrajectorySequence   int64                `json:"trajectory_sequence"`
	CallID               string               `json:"call_id"`
	Tool                 string               `json:"tool,omitempty"`
	Authority            Authority            `json:"authority"`
	Runtime              RuntimeEvidence      `json:"runtime"`
	RequestSHA256        string               `json:"request_sha256"`
	ResultSHA256         string               `json:"result_sha256"`
	ResultRepresentation ResultRepresentation `json:"result_representation"`
	Input                Artifact             `json:"input"`
	Result               Artifact             `json:"result"`
	Stdout               Artifact             `json:"stdout"`
	Stderr               Artifact             `json:"stderr"`
	StartedAt            time.Time            `json:"started_at"`
	FinishedAt           time.Time            `json:"finished_at"`
	Duration             time.Duration        `json:"duration_ns"`
	Disposition          string               `json:"disposition"`
	DenialCode           string               `json:"denial_code,omitempty"`
	Detail               string               `json:"detail,omitempty"`
	ExitCode             int                  `json:"exit_code"`
	TimedOut             bool                 `json:"timed_out"`
	Cancelled            bool                 `json:"cancelled"`
	Replayed             bool                 `json:"replayed"`
	Truncated            bool                 `json:"truncated"`
	StdoutTruncatedBytes int64                `json:"stdout_truncated_bytes"`
	StderrTruncatedBytes int64                `json:"stderr_truncated_bytes"`
	SourceChanges        []SourceChange       `json:"source_changes,omitempty"`
	Effect               EffectProof          `json:"effect"`
	Cancellation         CancellationEvidence `json:"cancellation"`
}

type Outcome struct {
	Evidence             Evidence `json:"evidence"`
	TrajectorySequence   int64    `json:"trajectory_sequence"`
	ReplayedFromSequence int64    `json:"replayed_from_sequence,omitempty"`
	Stdout               string   `json:"stdout,omitempty"`
	Stderr               string   `json:"stderr,omitempty"`
}
