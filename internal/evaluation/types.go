// Package evaluation provides the deterministic, no-model Architecture 022
// evaluation boundary. It consumes repository-owned fixtures and emits
// canonical evidence without acquiring production lifecycle authority.
package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	SuiteSchemaVersion     = "revolvr-architecture-evaluation-suite-v1"
	ScenarioSchemaVersion  = "revolvr-architecture-evaluation-scenario-v1"
	ResultSchemaVersion    = "revolvr-architecture-evaluation-result-v1"
	BaselineSchemaVersion  = "revolvr-architecture-evaluation-baseline-v1"
	AuthoritySchemaVersion = "revolvr-worker-execution-authority-v1"
	MetricsSchemaVersion   = "revolvr-architecture-evaluation-metrics-v1"
	PolicyVersion          = "revolvr-architecture-evaluation-policy-v1"
)

type WorkerExecutionMode string

const (
	DirectToolsV1           WorkerExecutionMode = "direct_tools_v1"
	ProgrammaticWorkspaceV1 WorkerExecutionMode = "programmatic_workspace_v1"
)

type RetrievalMode string

const (
	RetrievalReady             RetrievalMode = "ready"
	RetrievalStaleIndex        RetrievalMode = "stale_index"
	RetrievalMissingEmbeddings RetrievalMode = "missing_embeddings"
)

type CrashBoundary string

const (
	CrashBeforeSandbox             CrashBoundary = "before_sandbox_creation"
	CrashAfterSandbox              CrashBoundary = "after_sandbox_creation_before_state_update"
	CrashAfterWorker               CrashBoundary = "after_worker_exit_before_artifact_ingestion"
	CrashAfterCandidateCommit      CrashBoundary = "after_candidate_commit_before_state_update"
	CrashAfterCompletionArtifacts  CrashBoundary = "after_completion_artifacts_before_terminal_state"
	CrashAfterExternalBranchExport CrashBoundary = "after_external_branch_export"
)

var allCrashBoundaries = []CrashBoundary{
	CrashBeforeSandbox,
	CrashAfterSandbox,
	CrashAfterWorker,
	CrashAfterCandidateCommit,
	CrashAfterCompletionArtifacts,
	CrashAfterExternalBranchExport,
}

func AllCrashBoundaries() []CrashBoundary {
	return append([]CrashBoundary(nil), allCrashBoundaries...)
}

type Suite struct {
	SchemaVersion     string     `json:"schema_version"`
	FixtureRepository string     `json:"fixture_repository"`
	Policy            Policy     `json:"policy"`
	Scenarios         []Scenario `json:"scenarios"`
}

type Policy struct {
	Version              string   `json:"version"`
	ProtectedPaths       []string `json:"protected_paths"`
	AllowedMutationPaths []string `json:"allowed_mutation_paths"`
	Network              string   `json:"network"`
	MaximumCorrections   int      `json:"maximum_corrections"`
}

type Scenario struct {
	SchemaVersion          string          `json:"schema_version"`
	ID                     string          `json:"id"`
	Description            string          `json:"description"`
	Behavior               string          `json:"behavior"`
	TaskRequirement        string          `json:"task_requirement"`
	AcceptanceRequirement  string          `json:"acceptance_requirement"`
	ExpectedOutcome        string          `json:"expected_outcome"`
	ExpectedStopReason     string          `json:"expected_stop_reason"`
	RetrievalMode          RetrievalMode   `json:"retrieval_mode"`
	DirectToolCount        int             `json:"direct_tool_count"`
	RepeatedReadCount      int             `json:"repeated_read_count"`
	VerificationExecutions int             `json:"verification_executions"`
	VerificationReuses     int             `json:"verification_reuses"`
	CorrectionCycles       int             `json:"correction_cycles"`
	LogicalWallTimeMS      int64           `json:"logical_wall_time_ms"`
	CrashBoundaries        []CrashBoundary `json:"crash_boundaries,omitempty"`
}

type TaskAuthority struct {
	TaskID         string `json:"task_id"`
	TaskVersionID  string `json:"task_version_id"`
	Requirement    string `json:"requirement"`
	RequirementSHA string `json:"requirement_sha256"`
}

type CriterionAuthority struct {
	CriterionID       string `json:"criterion_id"`
	Requirement       string `json:"requirement"`
	RequirementSHA256 string `json:"requirement_sha256"`
}

type SourceAuthority struct {
	FixturePath   string `json:"fixture_path"`
	FixtureSHA256 string `json:"fixture_sha256"`
}

type ExpectedAuthority struct {
	Outcome    string `json:"outcome"`
	StopReason string `json:"stop_reason"`
}

// FrozenAuthority is mode-neutral. WorkerExecutionMode is intentionally not a
// field: every admitted implementation receives these exact immutable bytes.
type FrozenAuthority struct {
	SchemaVersion string               `json:"schema_version"`
	ScenarioID    string               `json:"scenario_id"`
	Task          TaskAuthority        `json:"task"`
	Acceptance    []CriterionAuthority `json:"acceptance"`
	Policy        Policy               `json:"policy"`
	PolicySHA256  string               `json:"policy_sha256"`
	Source        SourceAuthority      `json:"source"`
	Expected      ExpectedAuthority    `json:"expected"`
	SHA256        string               `json:"sha256"`
}

type ExecutionRequest struct {
	Mode      WorkerExecutionMode `json:"worker_execution_mode"`
	Scenario  Scenario            `json:"scenario"`
	Authority FrozenAuthority     `json:"authority"`
}

type StateFact struct {
	Status        string `json:"status"`
	Applicability string `json:"applicability"`
	SHA256        string `json:"sha256"`
}

type CriterionFact struct {
	CriterionID    string `json:"criterion_id"`
	Status         string `json:"status"`
	EvidenceSHA256 string `json:"evidence_sha256,omitempty"`
}

type FindingFact struct {
	FindingID        string `json:"finding_id"`
	Status           string `json:"status"`
	DefinitionSHA256 string `json:"definition_sha256"`
}

type WorkspaceFact struct {
	State                     StateFact `json:"state"`
	BaselineCommit            string    `json:"baseline_commit,omitempty"`
	CandidateCommit           string    `json:"candidate_commit,omitempty"`
	OriginalCheckoutBefore    string    `json:"original_checkout_before"`
	OriginalCheckoutAfter     string    `json:"original_checkout_after"`
	OriginalCheckoutUnchanged bool      `json:"original_checkout_unchanged"`
	Cleaned                   bool      `json:"cleaned"`
}

type SandboxFact struct {
	State          StateFact `json:"state"`
	Profile        string    `json:"profile,omitempty"`
	Network        string    `json:"network,omitempty"`
	AmbientEnv     bool      `json:"ambient_environment_inherited"`
	RuntimeSocket  bool      `json:"runtime_socket_mounted"`
	OriginalSource bool      `json:"original_checkout_mounted"`
}

type VerificationFact struct {
	State       StateFact `json:"state"`
	Executions  int       `json:"executions"`
	ExactReuses int       `json:"exact_reuses"`
	Occurrences int       `json:"occurrences"`
	FreshFinal  bool      `json:"fresh_final"`
}

type AuditFact struct {
	State            StateFact `json:"state"`
	Runs             int       `json:"runs"`
	Independent      bool      `json:"independent"`
	BlockingFindings int       `json:"blocking_findings"`
}

type CompletionFact struct {
	State         StateFact `json:"state"`
	CapsuleSHA256 string    `json:"capsule_sha256,omitempty"`
	Authorized    bool      `json:"authorized"`
}

type EventFact struct {
	Sequence int    `json:"sequence"`
	Type     string `json:"type"`
	SHA256   string `json:"sha256"`
}

type ArtifactFact struct {
	Kind      string `json:"kind"`
	SHA256    string `json:"sha256"`
	SizeBytes int    `json:"size_bytes"`
}

type Omission struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type TokenMetrics struct {
	Input     *int64 `json:"input"`
	Output    *int64 `json:"output"`
	Reasoning *int64 `json:"reasoning"`
	Cached    *int64 `json:"cached"`
}

type Metrics struct {
	SchemaVersion           string              `json:"schema_version"`
	WorkerExecutionMode     WorkerExecutionMode `json:"worker_execution_mode"`
	ContextBytes            int                 `json:"context_bytes"`
	Tokens                  TokenMetrics        `json:"tokens"`
	DirectToolCount         int                 `json:"direct_tool_count"`
	RepeatedReadCount       int                 `json:"repeated_read_count"`
	VerificationExecutions  int                 `json:"verification_executions"`
	VerificationExactReuses int                 `json:"verification_exact_reuses"`
	CorrectionCycles        int                 `json:"correction_cycles"`
	WallTimeNanoseconds     int64               `json:"wall_time_nanoseconds"`
	FinalTypedOutcome       string              `json:"final_typed_outcome"`
	Omissions               []Omission          `json:"omissions"`
}

type RetrievalFact struct {
	Status                  string   `json:"status"`
	CandidateIDs            []string `json:"candidate_ids"`
	LaneStates              []string `json:"lane_states"`
	ContextPackageID        string   `json:"context_package_id"`
	ContextManifestSHA256   string   `json:"context_manifest_sha256"`
	DossierSHA256           string   `json:"dossier_sha256"`
	ExactSourceFirst        bool     `json:"exact_source_first"`
	EmbeddingModelName      string   `json:"embedding_model_name"`
	EmbeddingRevision       string   `json:"embedding_revision"`
	EmbeddingSpaceSHA256    string   `json:"embedding_space_sha256"`
	QueryInstructionSHA256  string   `json:"query_instruction_sha256"`
	DegradedWithoutFallback bool     `json:"degraded_without_fallback"`
}

type CrashReplayFact struct {
	Boundary               CrashBoundary `json:"boundary"`
	EffectSHA256           string        `json:"effect_sha256"`
	ReplayCount            int           `json:"replay_count"`
	ExactReplayIdempotent  bool          `json:"exact_replay_idempotent"`
	DivergentReplayOutcome string        `json:"divergent_replay_outcome"`
}

type SafetyFact struct {
	NoLiveModel            bool `json:"no_live_model"`
	NoPublicNetwork        bool `json:"no_public_network"`
	NoAmbientCredentials   bool `json:"no_ambient_credentials"`
	NoOperatorHomeData     bool `json:"no_operator_home_data"`
	NoRuntimeSocket        bool `json:"no_runtime_socket"`
	OriginalCheckoutIntact bool `json:"original_checkout_intact"`
}

type Result struct {
	SchemaVersion   string              `json:"schema_version"`
	ScenarioID      string              `json:"scenario_id"`
	WorkerMode      WorkerExecutionMode `json:"worker_execution_mode"`
	AuthoritySHA256 string              `json:"authority_sha256"`
	Outcome         string              `json:"outcome"`
	StopReason      string              `json:"stop_reason"`
	Task            StateFact           `json:"task"`
	Run             StateFact           `json:"run"`
	Plan            StateFact           `json:"plan"`
	Criteria        []CriterionFact     `json:"criteria"`
	Findings        []FindingFact       `json:"findings"`
	Workspace       WorkspaceFact       `json:"workspace"`
	Sandbox         SandboxFact         `json:"sandbox"`
	Verification    VerificationFact    `json:"verification"`
	Audit           AuditFact           `json:"audit"`
	Completion      CompletionFact      `json:"completion"`
	Events          []EventFact         `json:"events"`
	Artifacts       []ArtifactFact      `json:"artifacts"`
	Retrieval       RetrievalFact       `json:"retrieval"`
	CrashReplays    []CrashReplayFact   `json:"crash_replays"`
	Metrics         Metrics             `json:"metrics"`
	LeaseAcquired   bool                `json:"lease_acquired"`
	LeaseReleased   bool                `json:"lease_released"`
	Safety          SafetyFact          `json:"safety"`
}

type QualityBaseline struct {
	FixtureCount            int        `json:"fixture_count"`
	RecallAt5               float64    `json:"recall_at_5"`
	RecallAt10              float64    `json:"recall_at_10"`
	MRR                     float64    `json:"mrr"`
	ExactSymbolPreservation float64    `json:"exact_symbol_preservation"`
	Threshold               *float64   `json:"threshold"`
	Omissions               []Omission `json:"omissions"`
}

type Baseline struct {
	SchemaVersion           string                `json:"schema_version"`
	SuiteSHA256             string                `json:"suite_sha256"`
	FixtureRepositorySHA256 string                `json:"fixture_repository_sha256"`
	ImplementationSHA256    string                `json:"implementation_sha256"`
	WorkerModes             []WorkerExecutionMode `json:"worker_modes"`
	ScenarioCount           int                   `json:"scenario_count"`
	Results                 []Result              `json:"results"`
	RetrievalQuality        QualityBaseline       `json:"retrieval_quality"`
	LiveDogfood             string                `json:"live_dogfood"`
	Omissions               []Omission            `json:"omissions"`
}

type ModeRefusalError struct {
	Mode WorkerExecutionMode
	Code string
}

func (e *ModeRefusalError) Error() string {
	return fmt.Sprintf("evaluation: worker execution mode %q: %s", e.Mode, e.Code)
}

func ValidateMode(mode WorkerExecutionMode) error {
	switch mode {
	case DirectToolsV1:
		return nil
	case ProgrammaticWorkspaceV1:
		return &ModeRefusalError{Mode: mode, Code: "not_implemented_not_admitted"}
	default:
		return &ModeRefusalError{Mode: mode, Code: "unknown_not_admitted"}
	}
}

func Canonical(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func hashValue(value any) (string, error) {
	raw, err := Canonical(value)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}

func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("evaluation: trailing JSON value")
		}
		return err
	}
	return nil
}

func (s Suite) Validate() error {
	if s.SchemaVersion != SuiteSchemaVersion || strings.TrimSpace(s.FixtureRepository) == "" {
		return errors.New("evaluation: invalid suite identity")
	}
	if s.Policy.Version != PolicyVersion || s.Policy.Network != "none" || s.Policy.MaximumCorrections < 1 {
		return errors.New("evaluation: invalid deterministic policy")
	}
	if len(s.Scenarios) != 20 {
		return fmt.Errorf("evaluation: scenario count = %d, want 20", len(s.Scenarios))
	}
	seen := map[string]struct{}{}
	coveredCrashes := map[CrashBoundary]struct{}{}
	for _, scenario := range s.Scenarios {
		if err := scenario.Validate(); err != nil {
			return err
		}
		if _, exists := seen[scenario.ID]; exists {
			return fmt.Errorf("evaluation: duplicate scenario %q", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}
		for _, boundary := range scenario.CrashBoundaries {
			coveredCrashes[boundary] = struct{}{}
		}
	}
	for _, id := range []string{
		"straight-success", "compile-failure-correction", "test-failure-correction", "audit-finding-correction",
		"ambiguous-requirement", "missing-dependency", "cyclic-dependency", "scope-violation",
		"protected-path-violation", "repeated-failed-strategy", "no-source-changes", "test-tampering",
		"mid-run-source-change", "cancellation", "crash-during-state-effects", "crash-during-external-effects",
		"stale-retrieval-index", "missing-embedding-service", "sandbox-timeout", "network-denied-dependency-install",
	} {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("evaluation: required scenario %q is missing", id)
		}
	}
	for _, boundary := range allCrashBoundaries {
		if _, exists := coveredCrashes[boundary]; !exists {
			return fmt.Errorf("evaluation: crash boundary %q is not covered", boundary)
		}
	}
	return nil
}

func (s Scenario) Validate() error {
	if s.SchemaVersion != ScenarioSchemaVersion || strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Behavior) == "" || strings.TrimSpace(s.TaskRequirement) == "" || strings.TrimSpace(s.AcceptanceRequirement) == "" || strings.TrimSpace(s.ExpectedOutcome) == "" || strings.TrimSpace(s.ExpectedStopReason) == "" {
		return fmt.Errorf("evaluation: scenario %q is incomplete", s.ID)
	}
	switch s.Behavior {
	case "straight_success", "compile_correction", "test_correction", "audit_correction", "ambiguity",
		"missing_dependency", "cyclic_dependency", "scope_violation", "protected_path", "repeated_strategy",
		"no_changes", "test_tampering", "mid_run_source_change", "cancellation", "crash_state", "crash_external",
		"stale_index", "missing_embeddings", "sandbox_timeout", "network_denied_install":
	default:
		return fmt.Errorf("evaluation: scenario %q has unknown behavior %q", s.ID, s.Behavior)
	}
	switch s.RetrievalMode {
	case RetrievalReady, RetrievalStaleIndex, RetrievalMissingEmbeddings:
	default:
		return fmt.Errorf("evaluation: scenario %q has unknown retrieval mode %q", s.ID, s.RetrievalMode)
	}
	if s.DirectToolCount < 0 || s.RepeatedReadCount < 0 || s.VerificationExecutions < 0 || s.VerificationReuses < 0 || s.CorrectionCycles < 0 || s.LogicalWallTimeMS < 0 {
		return fmt.Errorf("evaluation: scenario %q has negative metrics", s.ID)
	}
	seen := map[CrashBoundary]struct{}{}
	for _, boundary := range s.CrashBoundaries {
		valid := false
		for _, admitted := range allCrashBoundaries {
			valid = valid || boundary == admitted
		}
		if !valid {
			return fmt.Errorf("evaluation: scenario %q has unknown crash boundary %q", s.ID, boundary)
		}
		if _, exists := seen[boundary]; exists {
			return fmt.Errorf("evaluation: scenario %q repeats crash boundary %q", s.ID, boundary)
		}
		seen[boundary] = struct{}{}
	}
	return nil
}
