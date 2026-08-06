package verification

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"revolvr/internal/sandbox"
)

const (
	PlanSchemaVersion                = "revolvr-verification-plan-v1"
	VerifierProtocolVersion          = "revolvr-verifier-protocol-v1"
	VerifierImplementationVersion    = "revolvr-verification-engine-v1"
	DefaultStructuredParserVersion   = "revolvr-structured-parser-v1"
	DefaultOutputPolicyVersion       = "revolvr-output-policy-v1"
	DefaultProjectEnvironmentVersion = "revolvr-project-environment-v1"
	MaximumCapturedStreamBytes       = int64(1 << 20)
)

var (
	ErrInvalidPlan      = errors.New("invalid verification plan")
	ErrStaleSource      = errors.New("verification source authority is stale")
	ErrStaleEnvironment = errors.New("verification environment authority is stale")
	ErrAuthorityChanged = errors.New("verification authority changed")
	ErrArtifact         = errors.New("verification artifact failure")
	ErrPersistence      = errors.New("verification persistence failure")
)

type Tier int16

const (
	TierAdmissionBaseline Tier = iota
	TierFocused
	TierProject
	TierRisk
	TierFinal
)

type Purpose string

const (
	PurposeBaseline  Purpose = "baseline"
	PurposeCandidate Purpose = "candidate"
	PurposeFinal     Purpose = "final"
)

type Outcome string

const (
	OutcomePassed                 Outcome = "passed"
	OutcomeFailed                 Outcome = "failed"
	OutcomePassedReused           Outcome = "passed_reused"
	OutcomeUnchangedFailureReused Outcome = "unchanged_failure_reused"
	OutcomeTimedOut               Outcome = "timed_out"
	OutcomeCancelled              Outcome = "cancelled"
	OutcomeIncomplete             Outcome = "incomplete"
	OutcomeInfrastructureFailed   Outcome = "infrastructure_failed"
	OutcomeAmbiguous              Outcome = "ambiguous"
	OutcomeMalformedOutput        Outcome = "malformed_output"
	OutcomeArtifactFailed         Outcome = "artifact_failed"
	OutcomeStaleSource            Outcome = "stale_source"
	OutcomeStaleEnvironment       Outcome = "stale_environment"
	OutcomeMissingCommand         Outcome = "missing_command"
	OutcomeAuthorityTampered      Outcome = "authority_tampered"
)

func (o Outcome) Failed() bool {
	return o != OutcomePassed && o != OutcomePassedReused
}

type RunStatus string

const (
	RunPassed               RunStatus = "passed"
	RunFailed               RunStatus = "failed"
	RunCancelled            RunStatus = "cancelled"
	RunIncomplete           RunStatus = "incomplete"
	RunInfrastructureFailed RunStatus = "infrastructure_failed"
	RunAmbiguous            RunStatus = "ambiguous"
)

type AuthorityChangePolicy string

const (
	AuthorityReject   AuthorityChangePolicy = "reject"
	AuthorityDualRun  AuthorityChangePolicy = "dual_run"
	AuthorityEscalate AuthorityChangePolicy = "escalate"
)

type ParserKind string

const (
	ParserNone       ParserKind = "none"
	ParserGoTestJSON ParserKind = "go_test_json"
	ParserJSON       ParserKind = "json"
	ParserJUnitXML   ParserKind = "junit_xml"
)

type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type MaterialInput struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type SourceIdentity struct {
	Commit string `json:"commit"`
	Tree   string `json:"tree"`
}

type Parser struct {
	Kind    ParserKind `json:"kind"`
	Version string     `json:"version"`
}

type OutputPolicy struct {
	Version        string `json:"version"`
	StdoutMaxBytes int64  `json:"stdout_max_bytes"`
	StderrMaxBytes int64  `json:"stderr_max_bytes"`
}

type Gate struct {
	ID                   string                 `json:"id"`
	Tier                 Tier                   `json:"tier"`
	Source               SourceIdentity         `json:"source"`
	Argv                 []string               `json:"argv"`
	WorkingDirectory     string                 `json:"working_directory"`
	Environment          []EnvironmentVariable  `json:"environment"`
	Image                sandbox.Image          `json:"image"`
	SandboxProfile       sandbox.RuntimeProfile `json:"sandbox_profile"`
	SandboxProfileSHA256 string                 `json:"sandbox_profile_sha256"`
	Resources            sandbox.Resources      `json:"resources"`
	Parser               Parser                 `json:"parser"`
	AuthorityInputs      []MaterialInput        `json:"authority_inputs"`
	OutputPolicy         OutputPolicy           `json:"output_policy"`
}

type Plan struct {
	SchemaVersion           string                `json:"schema_version"`
	Version                 string                `json:"version"`
	VerificationPlanVersion string                `json:"verification_plan_version"`
	VerificationPlanSHA256  string                `json:"verification_plan_sha256"`
	AuthorityChangePolicy   AuthorityChangePolicy `json:"authority_change_policy"`
	AllowReuse              bool                  `json:"allow_reuse"`
	AllowMissingBaseline    bool                  `json:"allow_missing_baseline"`
	RequireFreshFinal       bool                  `json:"require_fresh_final"`
	Gates                   []Gate                `json:"gates"`
}

type ProjectEnvironment struct {
	SchemaVersion string          `json:"schema_version"`
	Contract      json.RawMessage `json:"contract"`
	SHA256        string          `json:"sha256"`
}

type PinnedPlan struct {
	Plan                   Plan               `json:"plan"`
	PlanSHA256             string             `json:"plan_sha256"`
	ProjectID              string             `json:"project_id"`
	TaskID                 string             `json:"task_id"`
	TaskVersionID          string             `json:"task_version_id"`
	RunID                  string             `json:"run_id"`
	WorkspaceID            string             `json:"workspace_id"`
	Candidate              SourceIdentity     `json:"candidate"`
	ProjectEnvironment     ProjectEnvironment `json:"project_environment"`
	VerifierProtocol       string             `json:"verifier_protocol_version"`
	VerifierImplementation string             `json:"verifier_implementation_version"`
}

type AuthoritySnapshot struct {
	Source                   SourceIdentity
	ProjectEnvironmentSHA256 string
	AuthorityInputs          []MaterialInput
}

type AuthorityObserver interface {
	Observe(context.Context, Gate) (AuthoritySnapshot, error)
}

type GateExecution struct {
	SandboxID string
	Pinned    PinnedPlan
	Gate      Gate
}

type ExecutionResult struct {
	SandboxSpecificationSHA256 string
	ExitCode                   int
	TimedOut                   bool
	Cancelled                  bool
	MissingCommand             bool
	Stdout                     []byte
	Stderr                     []byte
	StdoutTruncatedBytes       int64
	StderrTruncatedBytes       int64
	Evidence                   json.RawMessage
	StartedAt                  time.Time
	CompletedAt                time.Time
}

type GateExecutor interface {
	Execute(context.Context, GateExecution) (ExecutionResult, error)
}

type Artifact struct {
	ID          string
	SHA256      string
	SizeBytes   int64
	MediaType   string
	LogicalKind string
	StoragePath string
	Content     []byte
}

type ArtifactWriter interface {
	Materialize(context.Context, string, string, []byte) (Artifact, error)
}

type ReusableCheck struct {
	ID                         string
	Outcome                    Outcome
	ExitCode                   *int
	Stdout                     Artifact
	Stderr                     Artifact
	ParsedResult               json.RawMessage
	SandboxEvidence            json.RawMessage
	FailureSignatures          []string
	SandboxSpecificationSHA256 string
	OriginalExecutedAt         time.Time
}

type PersistedCheck struct {
	ID                            string
	Ordinal                       int
	Gate                          Gate
	Outcome                       Outcome
	ExecutionFingerprint          string
	VerifierProtocolVersion       string
	VerifierImplementationVersion string
	SandboxSpecificationSHA256    string
	ExitCode                      *int
	TimedOut                      bool
	Cancelled                     bool
	Stdout                        Artifact
	Stderr                        Artifact
	ParsedResult                  json.RawMessage
	SandboxEvidence               json.RawMessage
	FailureSignatures             []string
	ReusedFromCheckID             string
	OriginalExecutedAt            time.Time
	OccurredAt                    time.Time
	StartedAt                     time.Time
	CompletedAt                   time.Time
}

type Differential struct {
	New       []string `json:"new"`
	Resolved  []string `json:"resolved"`
	Unchanged []string `json:"unchanged"`
	Flaky     []string `json:"flaky"`
}

type PersistedRun struct {
	ID           string
	EventID      string
	Pinned       PinnedPlan
	Purpose      Purpose
	Status       RunStatus
	Checks       []PersistedCheck
	Differential Differential
	StartedAt    time.Time
	CompletedAt  time.Time
}

type EngineResult struct {
	VerificationRunID  string
	Status             RunStatus
	Checks             []PersistedCheck
	Differential       Differential
	AuthorityAction    AuthorityChangePolicy
	DualRunRequired    bool
	EscalationRequired bool
}

type Store interface {
	FindReusable(context.Context, string) (ReusableCheck, bool, error)
	Persist(context.Context, PersistedRun) error
}

type EngineConfig struct {
	Store     Store
	Executor  GateExecutor
	Artifacts ArtifactWriter
	Observer  AuthorityObserver
	Clock     func() time.Time
	NewID     func() string
}

type Request struct {
	Pinned   PinnedPlan
	Purpose  Purpose
	Baseline []PersistedCheck
}
