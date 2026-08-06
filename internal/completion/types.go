package completion

import (
	"context"
	"errors"
	"time"

	"revolvr/internal/evidence"
)

const (
	SnapshotSchemaVersion  = "revolvr-completion-preflight-snapshot-v1"
	PreflightSchemaVersion = "revolvr-completion-preflight-v1"
	AuditSchemaVersion     = "revolvr-audit-run-v1"
)

var (
	ErrRejected         = errors.New("completion preflight rejected")
	ErrStalePreflight   = errors.New("completion preflight is stale")
	ErrPersistence      = errors.New("completion persistence failed")
	ErrAlreadyCompleted = errors.New("completion operation conflicts with terminal evidence")
)

type Reason string

const (
	ReasonTaskAuthorityChanged        Reason = "task_authority_changed"
	ReasonPlanMissing                 Reason = "plan_missing"
	ReasonPlanStepNonterminal         Reason = "plan_step_nonterminal"
	ReasonCriterionNonterminal        Reason = "criterion_nonterminal"
	ReasonCriterionUnsatisfied        Reason = "criterion_unsatisfied"
	ReasonVerificationStale           Reason = "verification_stale"
	ReasonVerificationFailed          Reason = "verification_failed"
	ReasonAuditMissing                Reason = "audit_missing"
	ReasonAuditStale                  Reason = "audit_stale"
	ReasonAuditChangesRequired        Reason = "audit_changes_required"
	ReasonBlockingFindingOpen         Reason = "blocking_finding_open"
	ReasonSourceRevisionChanged       Reason = "source_revision_changed"
	ReasonBudgetInvalid               Reason = "budget_invalid"
	ReasonWorkspaceUnreconciled       Reason = "workspace_unreconciled"
	ReasonArtifactManifestIncomplete  Reason = "artifact_manifest_incomplete"
	ReasonCommitInvalid               Reason = "commit_invalid"
	ReasonLeaseUnreconciled           Reason = "lease_unreconciled"
	ReasonPromptModelAuthorityMissing Reason = "prompt_model_authority_missing"
	ReasonTrajectoryProvenanceInvalid Reason = "trajectory_provenance_invalid"
	ReasonHarnessAssetsInvalid        Reason = "harness_asset_provenance_invalid"
	ReasonOperatorInputInvalid        Reason = "operator_input_invalid"
	ReasonClaimEvidenceInvalid        Reason = "claim_evidence_invalid"
)

type Identity struct {
	ProjectID     string `json:"project_id"`
	TaskID        string `json:"task_id"`
	TaskVersionID string `json:"task_version_id"`
	RunID         string `json:"run_id"`
	WorkspaceID   string `json:"workspace_id"`
}

type Aggregates struct {
	Task      int64 `json:"task"`
	Run       int64 `json:"run"`
	Workspace int64 `json:"workspace"`
	Plan      int64 `json:"plan"`
	Lease     int64 `json:"lease"`
}

type Source struct {
	BeforeCommit string    `json:"before_commit"`
	BeforeTree   string    `json:"before_tree"`
	AfterCommit  string    `json:"after_commit"`
	AfterTree    string    `json:"after_tree"`
	DiffSHA256   string    `json:"diff_sha256"`
	FrozenAt     time.Time `json:"frozen_at"`
}

type PlanStep struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Plan struct {
	ID        string     `json:"id"`
	VersionID string     `json:"version_id"`
	SHA256    string     `json:"sha256"`
	Steps     []PlanStep `json:"steps"`
}

type Criterion struct {
	ID                  string `json:"id"`
	Status              string `json:"status"`
	VerificationCheckID string `json:"verification_check_id,omitempty"`
}

type VerificationCheck struct {
	ID                   string `json:"id"`
	Tier                 int16  `json:"tier"`
	Outcome              string `json:"outcome"`
	ExecutionFingerprint string `json:"execution_fingerprint"`
	ReusedFromCheckID    string `json:"reused_from_check_id,omitempty"`
	ImageDigest          string `json:"image_digest"`
	Profile              string `json:"profile"`
	ProfileSHA256        string `json:"profile_sha256"`
}

type Verification struct {
	ID            string              `json:"id"`
	Purpose       string              `json:"purpose"`
	Status        string              `json:"status"`
	SourceCommit  string              `json:"source_commit"`
	SourceTree    string              `json:"source_tree"`
	ImageDigest   string              `json:"image_digest"`
	Profile       string              `json:"profile"`
	ProfileSHA256 string              `json:"profile_sha256"`
	CompletedAt   time.Time           `json:"completed_at"`
	Checks        []VerificationCheck `json:"checks"`
}

type Audit struct {
	SchemaVersion    string    `json:"schema_version"`
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	Role             string    `json:"role"`
	Independent      bool      `json:"independent"`
	Disposition      string    `json:"disposition"`
	SourceCommit     string    `json:"source_commit"`
	SourceTree       string    `json:"source_tree"`
	ReportArtifactID string    `json:"report_artifact_id"`
	ReportSHA256     string    `json:"report_sha256"`
	CompletedAt      time.Time `json:"completed_at"`
}

type Finding struct {
	ID           string `json:"id"`
	Significance string `json:"significance"`
	Status       string `json:"status"`
	EvidenceID   string `json:"evidence_id,omitempty"`
}

type Budget struct {
	SchemaVersion string `json:"schema_version"`
	SHA256        string `json:"sha256"`
	Limit         int64  `json:"limit"`
	Consumed      int64  `json:"consumed"`
	InFlight      int64  `json:"in_flight"`
}

type Workspace struct {
	Status          string `json:"status"`
	Reconciled      bool   `json:"reconciled"`
	CandidateCommit string `json:"candidate_commit"`
	CandidateTree   string `json:"candidate_tree"`
	DiffSHA256      string `json:"diff_sha256"`
}

type Lease struct {
	Name  string `json:"name"`
	RunID string `json:"run_id"`
	Held  bool   `json:"held"`
}

type Invocation struct {
	Role          string `json:"role"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`
	PromptSHA256  string `json:"prompt_sha256"`
	DossierSHA256 string `json:"dossier_sha256"`
	ImageDigest   string `json:"image_digest"`
	Profile       string `json:"profile"`
}

type OperatorInput struct {
	ID         string `json:"id"`
	Version    int64  `json:"version"`
	SHA256     string `json:"sha256"`
	ArtifactID string `json:"artifact_id"`
	Resolved   bool   `json:"resolved"`
}

type Snapshot struct {
	SchemaVersion          string                       `json:"schema_version"`
	Identity               Identity                     `json:"identity"`
	TaskStatus             string                       `json:"task_status"`
	RunStatus              string                       `json:"run_status"`
	Aggregates             Aggregates                   `json:"aggregate_versions"`
	Source                 Source                       `json:"source"`
	Plan                   *Plan                        `json:"plan"`
	Criteria               []Criterion                  `json:"criteria"`
	Verification           *Verification                `json:"verification"`
	Audit                  *Audit                       `json:"audit"`
	Findings               []Finding                    `json:"findings"`
	Budget                 Budget                       `json:"budget"`
	Workspace              Workspace                    `json:"workspace"`
	Lease                  Lease                        `json:"lease"`
	Invocations            []Invocation                 `json:"prompt_model_authority"`
	Artifacts              []evidence.ArtifactReference `json:"artifacts"`
	ArtifactManifestSHA256 string                       `json:"artifact_manifest_sha256"`
	OperatorInputs         []OperatorInput              `json:"operator_inputs"`
	Trajectory             evidence.TrajectoryEnvelope  `json:"trajectory"`
	HarnessAssets          evidence.HarnessAssetSet     `json:"harness_asset_set"`
	Claims                 []evidence.Claim             `json:"claims"`
}

type Rejection struct {
	Reason Reason `json:"reason"`
	Detail string `json:"detail"`
}

type Preflight struct {
	SchemaVersion string      `json:"schema_version"`
	SHA256        string      `json:"sha256"`
	Snapshot      Snapshot    `json:"snapshot"`
	Rejections    []Rejection `json:"rejections"`
}

func (p Preflight) Accepted() bool { return len(p.Rejections) == 0 }

type Key struct {
	OperationID string
	Identity    Identity
}

type Materialized struct {
	EvidenceJSON evidence.Artifact
	Markdown     evidence.Artifact
	Manifest     evidence.Artifact
}

type TerminalCommand struct {
	CompletionID string
	OperationID  string
	Preflight    Preflight
	Materialized Materialized
	CompletedAt  time.Time
}

type TerminalResult struct {
	CompletionID string
	Replay       bool
}

type Result struct {
	Preflight    Preflight
	Materialized Materialized
	Terminal     TerminalResult
}

type Supplement struct {
	Audit          *Audit
	Findings       []Finding
	Budget         Budget
	Invocations    []Invocation
	Artifacts      []evidence.ArtifactReference
	OperatorInputs []OperatorInput
	Trajectory     evidence.TrajectoryEnvelope
	HarnessAssets  evidence.HarnessAssetSet
	Claims         []evidence.Claim
}

type AuthorityReader interface {
	ReadCompletionSnapshot(context.Context, Key) (Snapshot, error)
}

type TerminalStore interface {
	CommitCompletion(context.Context, TerminalCommand) (TerminalResult, error)
}

type TerminalLookup interface {
	LookupCompletion(context.Context, Key) (TerminalResult, bool, error)
}

type FailurePoint string

const (
	FailureAfterEvidenceJSON FailurePoint = "after_evidence_json"
	FailureAfterMarkdown     FailurePoint = "after_markdown"
	FailureBeforeTerminal    FailurePoint = "before_terminal_transaction"
)

type FailureInjector func(FailurePoint) error
