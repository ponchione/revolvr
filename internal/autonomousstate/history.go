package autonomousstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousplanning"
)

const PlanningHistorySchemaVersion = "autonomous-planning-transition-v1"

type PlanningChange string

const (
	PlanningChangeCreated PlanningChange = "created"
	PlanningChangeRevised PlanningChange = "revised"
)

type ArtifactIdentity struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type StateIdentity struct {
	Path      string `json:"path"`
	Persisted bool   `json:"persisted"`
	SHA256    string `json:"sha256"`
	ByteSize  int    `json:"byte_size"`
}

type PlanIdentity struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type AcceptanceCounts struct {
	Total         int `json:"total"`
	Pending       int `json:"pending"`
	Satisfied     int `json:"satisfied"`
	Waived        int `json:"waived"`
	NotApplicable int `json:"not_applicable"`
}

type PlanningHistoryRecord struct {
	SchemaVersion     string         `json:"schema_version"`
	TaskID            string         `json:"task_id"`
	OperationID       string         `json:"operation_id"`
	ApplicationSHA256 string         `json:"application_sha256"`
	Change            PlanningChange `json:"change"`
	CreatedAt         time.Time      `json:"created_at"`

	Decision           autonomous.DecisionReference       `json:"decision_reference"`
	SupervisorDecision ArtifactIdentity                   `json:"supervisor_decision_artifact"`
	WorkerRunID        string                             `json:"worker_run_id"`
	Profile            autonomousplanning.ProfileIdentity `json:"profile"`
	Dossier            autonomousplanning.DossierIdentity `json:"dossier"`
	SourceRevision     string                             `json:"source_revision"`
	TaskSource         ArtifactIdentity                   `json:"task_source"`
	RawOutput          ArtifactIdentity                   `json:"raw_output"`
	CanonicalOutput    ArtifactIdentity                   `json:"canonical_output"`

	PreviousState         StateIdentity                    `json:"previous_state"`
	ResultingState        StateIdentity                    `json:"resulting_state"`
	PreviousPlan          *autonomous.TaskPlan             `json:"previous_plan,omitempty"`
	ResultingPlan         autonomous.TaskPlan              `json:"resulting_plan"`
	PreviousAcceptance    []autonomous.AcceptanceCriterion `json:"previous_acceptance"`
	ResultingAcceptance   []autonomous.AcceptanceCriterion `json:"resulting_acceptance"`
	PreviousPlanIdentity  *PlanIdentity                    `json:"previous_plan_identity,omitempty"`
	ResultingPlanIdentity PlanIdentity                     `json:"resulting_plan_identity"`
	Acceptance            AcceptanceCounts                 `json:"acceptance_counts"`
}

func (r PlanningHistoryRecord) Validate() error {
	if r.SchemaVersion != PlanningHistorySchemaVersion {
		return fmt.Errorf("validate planning history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return fmt.Errorf("validate planning history: %w", err)
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil {
		return fmt.Errorf("validate planning history: %w", err)
	}
	if !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate planning history: application_sha256 is invalid")
	}
	if r.Change != PlanningChangeCreated && r.Change != PlanningChangeRevised {
		return fmt.Errorf("validate planning history: unknown change %q", r.Change)
	}
	if r.CreatedAt.IsZero() {
		return errors.New("validate planning history: created_at is required")
	}
	if err := r.Decision.Validate(); err != nil {
		return fmt.Errorf("validate planning history: decision_reference: %w", err)
	}
	if r.Decision.TaskID != r.TaskID || r.Decision.Action != autonomous.ActionPlan || r.Decision.WorkerProfile != autonomous.WorkerProfilePlanner {
		return errors.New("validate planning history: decision reference is not the exact task plan -> planner decision")
	}
	if err := r.SupervisorDecision.Validate(); err != nil {
		return fmt.Errorf("validate planning history: supervisor decision artifact: %w", err)
	}
	if r.SupervisorDecision.Path != r.Decision.Artifact.Reference {
		return errors.New("validate planning history: supervisor artifact path does not match decision reference")
	}
	if err := validateIdentity("worker_run_id", r.WorkerRunID); err != nil {
		return fmt.Errorf("validate planning history: %w", err)
	}
	if r.WorkerRunID == r.Decision.RunID {
		return errors.New("validate planning history: supervisor and worker run IDs must differ")
	}
	if err := r.Profile.Validate(); err != nil {
		return fmt.Errorf("validate planning history: profile: %w", err)
	}
	if err := r.Dossier.Validate(); err != nil {
		return fmt.Errorf("validate planning history: dossier: %w", err)
	}
	if r.Dossier.TaskID != r.TaskID {
		return errors.New("validate planning history: dossier task identity mismatch")
	}
	if !validSHA256(r.SourceRevision) {
		return errors.New("validate planning history: source_revision is invalid")
	}
	artifacts := []struct {
		label    string
		artifact ArtifactIdentity
	}{
		{label: "task source", artifact: r.TaskSource},
		{label: "raw output", artifact: r.RawOutput},
		{label: "canonical output", artifact: r.CanonicalOutput},
	}
	for _, item := range artifacts {
		if err := item.artifact.Validate(); err != nil {
			return fmt.Errorf("validate planning history: %s: %w", item.label, err)
		}
	}
	if err := r.PreviousState.Validate(); err != nil {
		return fmt.Errorf("validate planning history: previous_state: %w", err)
	}
	if err := r.ResultingState.Validate(); err != nil {
		return fmt.Errorf("validate planning history: resulting_state: %w", err)
	}
	if !r.ResultingState.Persisted {
		return errors.New("validate planning history: resulting_state must be persisted")
	}
	if err := r.ResultingPlan.Validate(); err != nil {
		return fmt.Errorf("validate planning history: resulting_plan: %w", err)
	}
	if r.ResultingPlan.TaskID != r.TaskID || r.ResultingPlanIdentity != planIdentity(r.ResultingPlan) {
		return errors.New("validate planning history: resulting plan identity mismatch")
	}
	if r.PreviousPlan == nil {
		if r.PreviousPlanIdentity != nil || r.Change != PlanningChangeCreated {
			return errors.New("validate planning history: created transition has contradictory previous plan identity")
		}
	} else {
		if err := r.PreviousPlan.Validate(); err != nil {
			return fmt.Errorf("validate planning history: previous_plan: %w", err)
		}
		if r.PreviousPlanIdentity == nil || *r.PreviousPlanIdentity != planIdentity(*r.PreviousPlan) || r.Change != PlanningChangeRevised {
			return errors.New("validate planning history: revised transition has contradictory previous plan identity")
		}
	}
	if err := validateAcceptanceSlice(r.TaskID, r.PreviousAcceptance); err != nil {
		return fmt.Errorf("validate planning history: previous_acceptance: %w", err)
	}
	if err := validateAcceptanceSlice(r.TaskID, r.ResultingAcceptance); err != nil {
		return fmt.Errorf("validate planning history: resulting_acceptance: %w", err)
	}
	if got := CountAcceptance(r.ResultingAcceptance); got != r.Acceptance {
		return fmt.Errorf("validate planning history: acceptance_counts = %+v, want %+v", r.Acceptance, got)
	}
	return nil
}

func (a ArtifactIdentity) Validate() error {
	if err := validatePath(a.Path); err != nil {
		return err
	}
	if !validSHA256(a.SHA256) {
		return errors.New("SHA-256 is invalid")
	}
	if a.ByteSize < 0 {
		return fmt.Errorf("byte_size cannot be negative (got %d)", a.ByteSize)
	}
	return nil
}

func (s StateIdentity) Validate() error {
	if err := validatePath(s.Path); err != nil {
		return err
	}
	if !validSHA256(s.SHA256) {
		return errors.New("SHA-256 is invalid")
	}
	if s.ByteSize <= 0 {
		return fmt.Errorf("byte_size must be positive (got %d)", s.ByteSize)
	}
	return nil
}

func CountAcceptance(criteria []autonomous.AcceptanceCriterion) AcceptanceCounts {
	counts := AcceptanceCounts{Total: len(criteria)}
	for _, criterion := range criteria {
		switch criterion.Status {
		case autonomous.AcceptanceStatusPending:
			counts.Pending++
		case autonomous.AcceptanceStatusSatisfied:
			counts.Satisfied++
		case autonomous.AcceptanceStatusWaived:
			counts.Waived++
		case autonomous.AcceptanceStatusNotApplicable:
			counts.NotApplicable++
		}
	}
	return counts
}

func MarshalPlanningHistory(record PlanningHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal planning history: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodePlanningHistory(raw []byte) (PlanningHistoryRecord, error) {
	var record PlanningHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return PlanningHistoryRecord{}, fmt.Errorf("decode planning history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return PlanningHistoryRecord{}, err
	}
	canonical, err := MarshalPlanningHistory(record)
	if err != nil {
		return PlanningHistoryRecord{}, err
	}
	if !reflect.DeepEqual(raw, canonical) {
		return PlanningHistoryRecord{}, errors.New("decode planning history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func PlanIdentityFor(plan *autonomous.TaskPlan) *PlanIdentity {
	if plan == nil {
		return nil
	}
	identity := planIdentity(*plan)
	return &identity
}

func planIdentity(plan autonomous.TaskPlan) PlanIdentity {
	return PlanIdentity{ID: plan.ID, Revision: plan.Revision}
}

func validateAcceptanceSlice(taskID string, criteria []autonomous.AcceptanceCriterion) error {
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        taskID, Lifecycle: autonomous.LifecycleStatePending,
		AcceptanceCriteria: criteria,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	return state.Validate()
}

func validateIdentity(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s %q is empty or malformed", label, value)
	}
	return nil
}

func validatePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.HasPrefix(value, "/") {
		return fmt.Errorf("path %q is empty, absolute, or malformed", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("path %q is not normalized and repository-relative", value)
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
