// Package autonomousview owns the pure, versioned operator projection for one
// autonomous task. It performs no repository, ledger, clock, command, or model
// I/O; callers supply already validated evidence snapshots.
package autonomousview

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"revolvr/internal/autonomous"
)

const SchemaVersion = "autonomous-task-view-v1"

type SourceKind string

const (
	SourceActive  SourceKind = "active"
	SourceArchive SourceKind = "archive"
)

type Identity struct {
	SourceKind         SourceKind `json:"source_kind"`
	TaskID             string     `json:"task_id"`
	Title              string     `json:"title"`
	TaskPath           string     `json:"task_path"`
	TaskSHA256         string     `json:"task_sha256"`
	TaskByteSize       int        `json:"task_byte_size"`
	Workflow           string     `json:"workflow"`
	TaskStatus         string     `json:"task_status"`
	Lifecycle          string     `json:"lifecycle,omitempty"`
	StatePath          string     `json:"state_path,omitempty"`
	StateSHA256        string     `json:"state_sha256,omitempty"`
	StateByteSize      int        `json:"state_byte_size,omitempty"`
	StateSchema        string     `json:"state_schema,omitempty"`
	ArchiveID          string     `json:"archive_id,omitempty"`
	ArchiveDisposition string     `json:"archive_disposition,omitempty"`
}

type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

type Summary struct {
	Phase                   string   `json:"phase"`
	Plan                    Progress `json:"plan"`
	Acceptance              Progress `json:"acceptance"`
	OpenBlockingFindings    int      `json:"open_blocking_findings"`
	OpenNonBlockingFindings int      `json:"open_non_blocking_findings"`
	TotalAttempts           int64    `json:"total_attempts"`
	ConsecutiveFailures     int64    `json:"consecutive_failures"`
	NeedsInput              bool     `json:"needs_input"`
	Blocked                 bool     `json:"blocked"`
	Terminal                bool     `json:"terminal"`
}

type WhyReason struct {
	Code     string                         `json:"code"`
	Text     string                         `json:"text"`
	Evidence []autonomous.EvidenceReference `json:"evidence,omitempty"`
}

type Why struct {
	LatestDecision          string                        `json:"latest_decision"`
	LatestDecisionReference *autonomous.DecisionReference `json:"latest_decision_reference,omitempty"`
	CurrentlyAdmittedAction string                        `json:"currently_admitted_action"`
	SchedulerReadiness      string                        `json:"scheduler_readiness"`
	NextSupervisorAction    string                        `json:"next_supervisor_action"`
	Reasons                 []WhyReason                   `json:"reasons"`
}

type Plan struct {
	ID               string                         `json:"id"`
	Revision         int64                          `json:"revision"`
	SupersedesPlanID string                         `json:"supersedes_plan_id,omitempty"`
	Completed        bool                           `json:"completed"`
	Provenance       []autonomous.EvidenceReference `json:"provenance"`
	Steps            []PlanStep                     `json:"steps"`
}

type PlanStep struct {
	ID          string                         `json:"id"`
	Status      string                         `json:"status"`
	Description string                         `json:"description"`
	Rationale   string                         `json:"rationale,omitempty"`
	Evidence    []autonomous.EvidenceReference `json:"evidence,omitempty"`
}

type Acceptance struct {
	ID          string                         `json:"id"`
	Description string                         `json:"description"`
	Status      string                         `json:"status"`
	Rationale   string                         `json:"rationale,omitempty"`
	Evidence    []autonomous.EvidenceReference `json:"evidence,omitempty"`
	Source      *autonomous.EvidenceReference  `json:"source,omitempty"`
}

type AuditIdentity struct {
	Revision       int64  `json:"revision"`
	RunID          string `json:"run_id"`
	SourceRevision string `json:"source_revision"`
	Disposition    string `json:"disposition"`
	ArtifactPath   string `json:"artifact_path,omitempty"`
}

type Finding struct {
	ID                   string                         `json:"id"`
	Significance         string                         `json:"significance"`
	Summary              string                         `json:"summary"`
	RequiredCorrection   string                         `json:"required_correction"`
	IntroducedBy         AuditIdentity                  `json:"introduced_by"`
	CurrentAudit         AuditIdentity                  `json:"current_audit"`
	Status               string                         `json:"status"`
	Evidence             []autonomous.EvidenceReference `json:"evidence,omitempty"`
	ResolutionEvidence   []autonomous.EvidenceReference `json:"resolution_evidence,omitempty"`
	ResolutionRationale  string                         `json:"resolution_rationale,omitempty"`
	ResolutionAuthority  *autonomous.DecisionReference  `json:"resolution_authority,omitempty"`
	SupersedingFindingID string                         `json:"superseding_finding_id,omitempty"`
}

type Budget struct {
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	Limit     int64  `json:"limit,omitempty"`
	Consumed  int64  `json:"consumed"`
	Remaining int64  `json:"remaining,omitempty"`
	Exhausted bool   `json:"exhausted,omitempty"`
	Unit      string `json:"unit"`
}

type ActionAttempts struct {
	Action   string `json:"action"`
	Attempts int64  `json:"attempts"`
}

type AttemptReference struct {
	Sequence     int64                          `json:"sequence"`
	AttemptID    string                         `json:"attempt_id"`
	Kind         string                         `json:"kind"`
	Action       string                         `json:"action"`
	Outcome      string                         `json:"outcome,omitempty"`
	RunID        string                         `json:"run_id,omitempty"`
	OccurrenceID string                         `json:"occurrence_id,omitempty"`
	CreatedAt    time.Time                      `json:"created_at"`
	Evidence     []autonomous.EvidenceReference `json:"evidence,omitempty"`
}

type Attempts struct {
	Total               int64              `json:"total"`
	ConsecutiveFailures int64              `json:"consecutive_failures"`
	PerAction           []ActionAttempts   `json:"per_action"`
	Budgets             []Budget           `json:"budgets"`
	Events              []AttemptReference `json:"events"`
	Stops               []string           `json:"stops"`
}

type InputOption struct {
	ID      string `json:"id"`
	Meaning string `json:"meaning"`
}

type OperatorInput struct {
	State                   string        `json:"state"`
	QuestionID              string        `json:"question_id,omitempty"`
	Revision                int64         `json:"revision,omitempty"`
	ContentSHA256           string        `json:"content_sha256,omitempty"`
	Question                string        `json:"question,omitempty"`
	BlockingReason          string        `json:"blocking_reason,omitempty"`
	Options                 []InputOption `json:"options,omitempty"`
	RecommendationOption    string        `json:"recommendation_option,omitempty"`
	RecommendationRationale string        `json:"recommendation_rationale,omitempty"`
	AnswerID                string        `json:"answer_id,omitempty"`
	AnswerOptionID          string        `json:"answer_option_id,omitempty"`
	AnswerActor             string        `json:"answer_actor,omitempty"`
}

type Verification struct {
	State          string                         `json:"state"`
	RunID          string                         `json:"run_id,omitempty"`
	OccurrenceID   string                         `json:"occurrence_id,omitempty"`
	SourceRevision string                         `json:"source_revision,omitempty"`
	Status         string                         `json:"status,omitempty"`
	Purpose        string                         `json:"purpose,omitempty"`
	FinalGate      string                         `json:"final_gate,omitempty"`
	Evidence       []autonomous.EvidenceReference `json:"evidence,omitempty"`
}

type Audit struct {
	State          string `json:"state"`
	Revision       int64  `json:"revision,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`
	Disposition    string `json:"disposition,omitempty"`
	FindingCount   int    `json:"finding_count"`
	ArtifactPath   string `json:"artifact_path,omitempty"`
}

type Workspace struct {
	State              string `json:"state"`
	WorkspaceID        string `json:"workspace_id,omitempty"`
	Status             string `json:"status,omitempty"`
	ExecutionRoot      string `json:"execution_root,omitempty"`
	BranchRef          string `json:"branch_ref,omitempty"`
	SourceRevision     string `json:"source_revision,omitempty"`
	CheckpointSequence int64  `json:"checkpoint_sequence,omitempty"`
	CheckpointCommit   string `json:"checkpoint_commit,omitempty"`
}

type Terminal struct {
	State             string    `json:"state"`
	Reason            string    `json:"reason,omitempty"`
	FinalizationStage string    `json:"finalization_stage,omitempty"`
	ArchiveID         string    `json:"archive_id,omitempty"`
	Disposition       string    `json:"disposition,omitempty"`
	ArchivedAt        time.Time `json:"archived_at,omitempty"`
	VerifiedNow       bool      `json:"verified_now"`
}

type Provenance struct {
	Decision           *autonomous.DecisionReference `json:"decision,omitempty"`
	WorkerRunIDs       []string                      `json:"worker_run_ids"`
	VerificationRunIDs []string                      `json:"verification_run_ids"`
	AuditRunIDs        []string                      `json:"audit_run_ids"`
	References         []Reference                   `json:"references"`
}

type Reference struct {
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	ByteSize int    `json:"byte_size,omitempty"`
	Detail   string `json:"detail"`
}

type Diagnostic struct {
	Code      string `json:"code"`
	Section   string `json:"section"`
	Detail    string `json:"detail"`
	Reference string `json:"reference,omitempty"`
}

type View struct {
	SchemaVersion string        `json:"schema_version"`
	Identity      Identity      `json:"identity"`
	Summary       Summary       `json:"summary"`
	Why           Why           `json:"why"`
	Plan          *Plan         `json:"plan,omitempty"`
	Acceptance    []Acceptance  `json:"acceptance"`
	Findings      []Finding     `json:"findings"`
	Attempts      Attempts      `json:"attempts"`
	Input         OperatorInput `json:"operator_input"`
	Verification  Verification  `json:"verification"`
	Audit         Audit         `json:"audit"`
	Workspace     Workspace     `json:"workspace"`
	Terminal      Terminal      `json:"terminal"`
	Provenance    Provenance    `json:"provenance"`
	Diagnostics   []Diagnostic  `json:"diagnostics"`
}

func (v View) Validate() error {
	if v.SchemaVersion != SchemaVersion {
		return fmt.Errorf("autonomous view: unsupported schema_version %q", v.SchemaVersion)
	}
	if v.Identity.SourceKind != SourceActive && v.Identity.SourceKind != SourceArchive {
		return fmt.Errorf("autonomous view: unknown source kind %q", v.Identity.SourceKind)
	}
	if strings.TrimSpace(v.Identity.TaskID) == "" || strings.TrimSpace(v.Identity.TaskPath) == "" || strings.TrimSpace(v.Identity.Workflow) == "" || !validSHA256(v.Identity.TaskSHA256) || v.Identity.TaskByteSize <= 0 {
		return errors.New("autonomous view: task identity is incomplete")
	}
	if v.Identity.StatePath != "" {
		if !validSHA256(v.Identity.StateSHA256) || v.Identity.StateByteSize <= 0 || v.Identity.StateSchema == "" || v.Identity.Lifecycle == "" {
			return errors.New("autonomous view: state identity is incomplete")
		}
	} else if v.Identity.StateSHA256 != "" || v.Identity.StateByteSize != 0 || v.Identity.StateSchema != "" || v.Identity.Lifecycle != "" {
		return errors.New("autonomous view: absent state must not claim state identity")
	}
	if v.Identity.TaskByteSize < 0 || v.Identity.StateByteSize < 0 || v.Summary.Plan.Completed < 0 || v.Summary.Plan.Total < v.Summary.Plan.Completed || v.Summary.Acceptance.Completed < 0 || v.Summary.Acceptance.Total < v.Summary.Acceptance.Completed {
		return errors.New("autonomous view: counts are invalid")
	}
	if v.Identity.SourceKind == SourceArchive && (v.Identity.ArchiveID == "" || v.Identity.ArchiveDisposition == "") {
		return errors.New("autonomous view: archive identity is incomplete")
	}
	if v.Why.LatestDecision == "" || v.Why.CurrentlyAdmittedAction == "" || v.Why.SchedulerReadiness == "" || v.Why.NextSupervisorAction == "" {
		return errors.New("autonomous view: why projection is incomplete")
	}
	for i, reason := range v.Why.Reasons {
		if reason.Code == "" || strings.TrimSpace(reason.Text) == "" {
			return fmt.Errorf("autonomous view: why reason %d is incomplete", i)
		}
	}
	if v.Plan != nil {
		seenSteps := map[string]bool{}
		for _, step := range v.Plan.Steps {
			if step.ID == "" || seenSteps[step.ID] || !oneOf(step.Status, "pending", "in_progress", "completed", "skipped") {
				return errors.New("autonomous view: plan steps are malformed or duplicated")
			}
			seenSteps[step.ID] = true
		}
	}
	seenAcceptance := map[string]bool{}
	for _, item := range v.Acceptance {
		if item.ID == "" || seenAcceptance[item.ID] || !oneOf(item.Status, "pending", "satisfied", "waived", "not_applicable") {
			return errors.New("autonomous view: acceptance entries are malformed or duplicated")
		}
		seenAcceptance[item.ID] = true
	}
	seenFindings := map[string]bool{}
	for _, item := range v.Findings {
		if item.ID == "" || seenFindings[item.ID] || !oneOf(item.Status, "open", "resolved", "waived", "superseded", "invalid") || !oneOf(item.Significance, "blocking", "non_blocking", "not_available") {
			return errors.New("autonomous view: finding entries are malformed or duplicated")
		}
		seenFindings[item.ID] = true
	}
	seenBudgets := map[string]bool{}
	for _, item := range v.Attempts.Budgets {
		if item.Name == "" || seenBudgets[item.Name] || !oneOf(item.Mode, "unset", "limited", "unlimited") || item.Consumed < 0 || item.Limit < 0 || item.Remaining < 0 {
			return errors.New("autonomous view: budget entries are malformed or duplicated")
		}
		seenBudgets[item.Name] = true
	}
	seen := map[string]bool{}
	for _, item := range v.Diagnostics {
		if item.Code == "" || item.Section == "" || item.Detail == "" {
			return errors.New("autonomous view: diagnostic is incomplete")
		}
		key := item.Code + "\x00" + item.Section + "\x00" + item.Reference
		if seen[key] {
			return fmt.Errorf("autonomous view: duplicate diagnostic %q", item.Code)
		}
		seen[key] = true
	}
	return nil
}

func validSHA256(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func Marshal(v View) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func Decode(raw []byte) (View, error) {
	var v View
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&v); err != nil {
		return v, fmt.Errorf("decode autonomous view: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return v, errors.New("decode autonomous view: expected exactly one JSON value")
	}
	if err := v.Validate(); err != nil {
		return v, err
	}
	canonical, _ := Marshal(v)
	if !bytes.Equal(raw, canonical) {
		return v, errors.New("decode autonomous view: bytes are not canonical deterministic JSON")
	}
	return v, nil
}
