package autonomous

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	OptionalRoleAssessmentSchemaVersion = "autonomous-optional-role-assessment-v1"
	OptionalRoleOccurrenceSchemaVersion = "autonomous-optional-role-occurrence-v1"
)

type OptionalRoleDisposition string

const (
	OptionalRoleDispositionRun           OptionalRoleDisposition = "run"
	OptionalRoleDispositionNotApplicable OptionalRoleDisposition = "not_applicable"
)

type OptionalRoleOutcome string

const (
	OptionalRoleOutcomeNotApplicable OptionalRoleOutcome = "not_applicable"
	OptionalRoleOutcomeNoChange      OptionalRoleOutcome = "no_change"
	OptionalRoleOutcomeSourceChanged OptionalRoleOutcome = "source_changed"
)

type OptionalRoleEvidenceKind string

const (
	OptionalRoleEvidenceTaskDocumentation       OptionalRoleEvidenceKind = "task_documentation_requirement"
	OptionalRoleEvidenceAcceptanceDocumentation OptionalRoleEvidenceKind = "acceptance_documentation_requirement"
	OptionalRoleEvidencePlanDocumentation       OptionalRoleEvidenceKind = "plan_documentation_requirement"
	OptionalRoleEvidenceAuditDocumentation      OptionalRoleEvidenceKind = "audit_documentation_finding"
	OptionalRoleEvidenceUserFacingChange        OptionalRoleEvidenceKind = "user_facing_change"
	OptionalRoleEvidenceComplexityTarget        OptionalRoleEvidenceKind = "complexity_target"
	OptionalRoleEvidenceDuplicationTarget       OptionalRoleEvidenceKind = "duplication_target"
	OptionalRoleEvidenceMaintainabilityTarget   OptionalRoleEvidenceKind = "maintainability_target"
	OptionalRoleEvidenceNoRelevantWork          OptionalRoleEvidenceKind = "no_relevant_work"
)

// OptionalRoleEvidence is trusted harness input derived from an exact current
// task, state, source, or audit projection. Supervisor prose cannot create one.
type OptionalRoleEvidence struct {
	ID             string                   `json:"id"`
	Role           WorkerProfile            `json:"role"`
	Kind           OptionalRoleEvidenceKind `json:"kind"`
	Reference      EvidenceReference        `json:"reference"`
	SourceRevision string                   `json:"source_revision"`
	TargetPath     string                   `json:"target_path,omitempty"`
}

type OptionalRoleAssessment struct {
	SchemaVersion       string                  `json:"schema_version"`
	TaskID              string                  `json:"task_id"`
	Role                WorkerProfile           `json:"role"`
	Disposition         OptionalRoleDisposition `json:"disposition"`
	Decision            SupervisorDecision      `json:"decision"`
	DecisionReference   DecisionReference       `json:"decision_reference"`
	TaskSource          EvidenceReference       `json:"task_source"`
	StateSHA256         string                  `json:"state_sha256"`
	SourceRevision      string                  `json:"source_revision"`
	VerificationRunID   string                  `json:"verification_run_id"`
	VerificationID      string                  `json:"verification_occurrence_id"`
	AuditRunID          string                  `json:"audit_run_id"`
	AuditSourceRevision string                  `json:"audit_source_revision"`
	Evidence            []OptionalRoleEvidence  `json:"evidence"`
	SelectedEvidenceIDs []string                `json:"selected_evidence_ids"`
	Rationale           string                  `json:"rationale"`
}

type OptionalRoleGate struct {
	SourceRevision           string `json:"source_revision"`
	VerificationRunID        string `json:"verification_run_id"`
	VerificationOccurrenceID string `json:"verification_occurrence_id"`
	AuditSupervisorRunID     string `json:"audit_supervisor_run_id"`
	AuditWorkerRunID         string `json:"audit_worker_run_id"`
	AuditRevision            int64  `json:"audit_revision"`
}

type OptionalRoleWorkerEvidence struct {
	AttemptID       string            `json:"attempt_id"`
	RunID           string            `json:"run_id"`
	DossierSHA256   string            `json:"dossier_sha256"`
	DossierByteSize int               `json:"dossier_byte_size"`
	ProfilePath     string            `json:"profile_path"`
	ProfileSHA256   string            `json:"profile_sha256"`
	ProfileByteSize int               `json:"profile_byte_size"`
	Receipt         EvidenceReference `json:"receipt"`
	Ledger          EvidenceReference `json:"ledger"`
}

// OptionalRoleOccurrence is append-only task evidence. It is deliberately a
// compact identity projection; exact worker, verification, commit, and audit
// material remains in their existing artifacts and immutable histories.
type OptionalRoleOccurrence struct {
	SchemaVersion        string                      `json:"schema_version"`
	Sequence             int64                       `json:"sequence"`
	TaskID               string                      `json:"task_id"`
	Role                 WorkerProfile               `json:"role"`
	Outcome              OptionalRoleOutcome         `json:"outcome"`
	Decision             DecisionReference           `json:"decision_reference"`
	AssessmentSHA256     string                      `json:"assessment_sha256"`
	SourceBefore         string                      `json:"source_before"`
	SourceAfter          string                      `json:"source_after"`
	Gate                 OptionalRoleGate            `json:"gate"`
	Worker               *OptionalRoleWorkerEvidence `json:"worker,omitempty"`
	ChangedPaths         []string                    `json:"changed_paths,omitempty"`
	CommitSHA            string                      `json:"commit_sha,omitempty"`
	BehaviorPreservation []EvidenceReference         `json:"behavior_preservation,omitempty"`
	Evidence             []EvidenceReference         `json:"evidence"`
	Rationale            string                      `json:"rationale"`
	CreatedAt            time.Time                   `json:"created_at"`
}

func (e OptionalRoleEvidence) Validate(source string) error {
	if !validFindingID(e.ID) {
		return fmt.Errorf("invalid evidence id %q", e.ID)
	}
	if !optionalRole(e.Role) {
		return fmt.Errorf("unknown optional role %q", e.Role)
	}
	if !optionalEvidenceKind(e.Kind) {
		return fmt.Errorf("unknown evidence kind %q", e.Kind)
	}
	if err := validateEvidenceReferences("optional-role evidence reference", []EvidenceReference{e.Reference}); err != nil {
		return err
	}
	if !validStateSHA256(e.SourceRevision) || e.SourceRevision != source {
		return errors.New("optional-role evidence is stale for the assessed source")
	}
	needsPath := e.Kind != OptionalRoleEvidenceNoRelevantWork
	if needsPath && (strings.TrimSpace(e.TargetPath) == "" || e.TargetPath != strings.TrimSpace(e.TargetPath)) {
		return fmt.Errorf("evidence kind %q requires a normalized target_path", e.Kind)
	}
	if needsPath {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(e.TargetPath)))
		if filepath.IsAbs(filepath.FromSlash(e.TargetPath)) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != e.TargetPath {
			return fmt.Errorf("evidence kind %q target_path must be repository-relative and clean", e.Kind)
		}
	}
	if !needsPath && strings.TrimSpace(e.TargetPath) != "" {
		return fmt.Errorf("evidence kind %q must not set target_path", e.Kind)
	}
	return nil
}

func (a OptionalRoleAssessment) Validate() error {
	if a.SchemaVersion != OptionalRoleAssessmentSchemaVersion {
		return fmt.Errorf("validate optional-role assessment: unsupported schema_version %q", a.SchemaVersion)
	}
	if strings.TrimSpace(a.TaskID) == "" || !optionalRole(a.Role) || !validStateSHA256(a.StateSHA256) || !validStateSHA256(a.SourceRevision) {
		return errors.New("validate optional-role assessment: task, role, state, and source identities are required")
	}
	if err := a.Decision.Validate(); err != nil {
		return fmt.Errorf("validate optional-role assessment: decision: %w", err)
	}
	if err := a.DecisionReference.Validate(); err != nil {
		return fmt.Errorf("validate optional-role assessment: decision reference: %w", err)
	}
	wantAction, wantProfile := optionalRoleAction(a.Role)
	if a.Decision.TaskID != a.TaskID || a.Decision.Action != wantAction || a.Decision.WorkerProfile != wantProfile || a.DecisionReference.TaskID != a.TaskID || a.DecisionReference.Action != wantAction || a.DecisionReference.WorkerProfile != wantProfile {
		return errors.New("validate optional-role assessment: decision does not exactly select the assessed role")
	}
	if err := validateEvidenceReferences("validate optional-role assessment: task_source", []EvidenceReference{a.TaskSource}); err != nil || a.TaskSource.Kind != EvidenceKindTask {
		return errors.New("validate optional-role assessment: exact task source evidence is required")
	}
	if strings.TrimSpace(a.VerificationRunID) == "" || strings.TrimSpace(a.VerificationID) == "" || strings.TrimSpace(a.AuditRunID) == "" || a.AuditSourceRevision != a.SourceRevision {
		return errors.New("validate optional-role assessment: exact current verification and audit identities are required")
	}
	if len(a.Evidence) == 0 || len(a.SelectedEvidenceIDs) == 0 || strings.TrimSpace(a.Rationale) == "" {
		return errors.New("validate optional-role assessment: structured evidence, selection, and rationale are required")
	}
	byID := make(map[string]OptionalRoleEvidence, len(a.Evidence))
	relevant := false
	for i, evidence := range a.Evidence {
		if err := evidence.Validate(a.SourceRevision); err != nil {
			return fmt.Errorf("validate optional-role assessment: evidence[%d]: %w", i, err)
		}
		if evidence.Role != a.Role {
			return fmt.Errorf("validate optional-role assessment: evidence[%d] has role %q, want %q", i, evidence.Role, a.Role)
		}
		if _, ok := byID[evidence.ID]; ok {
			return fmt.Errorf("validate optional-role assessment: duplicate evidence id %q", evidence.ID)
		}
		byID[evidence.ID] = evidence
		if optionalEvidenceRelevant(a.Role, evidence.Kind) {
			relevant = true
		}
	}
	selectedRelevant, selectedNoWork := false, false
	seen := make(map[string]struct{}, len(a.SelectedEvidenceIDs))
	for i, id := range a.SelectedEvidenceIDs {
		evidence, ok := byID[id]
		if !ok {
			return fmt.Errorf("validate optional-role assessment: selected_evidence_ids[%d] %q is unknown", i, id)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("validate optional-role assessment: duplicate selected evidence id %q", id)
		}
		seen[id] = struct{}{}
		if !containsEvidenceReference(a.Decision.Inputs, evidence.Reference) {
			return fmt.Errorf("validate optional-role assessment: selected evidence %q is absent from supervisor inputs", id)
		}
		selectedRelevant = selectedRelevant || optionalEvidenceRelevant(a.Role, evidence.Kind)
		selectedNoWork = selectedNoWork || evidence.Kind == OptionalRoleEvidenceNoRelevantWork
	}
	switch a.Disposition {
	case OptionalRoleDispositionRun:
		if !selectedRelevant {
			return errors.New("validate optional-role assessment: run requires a concrete selected role target")
		}
	case OptionalRoleDispositionNotApplicable:
		if relevant {
			return errors.New("validate optional-role assessment: not_applicable cannot waive a current role obligation or target")
		}
		if !selectedNoWork {
			return errors.New("validate optional-role assessment: not_applicable requires exact no_relevant_work evidence")
		}
	default:
		return fmt.Errorf("validate optional-role assessment: unknown disposition %q", a.Disposition)
	}
	return nil
}

func (a OptionalRoleAssessment) Identity() (string, error) {
	if err := a.Validate(); err != nil {
		return "", err
	}
	evidence := append([]OptionalRoleEvidence(nil), a.Evidence...)
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].ID < evidence[j].ID })
	selected := append([]string(nil), a.SelectedEvidenceIDs...)
	sort.Strings(selected)
	raw, err := json.Marshal(struct {
		SchemaVersion       string                  `json:"schema_version"`
		TaskID              string                  `json:"task_id"`
		Role                WorkerProfile           `json:"role"`
		Disposition         OptionalRoleDisposition `json:"disposition"`
		TaskSource          EvidenceReference       `json:"task_source"`
		StateSHA256         string                  `json:"state_sha256"`
		SourceRevision      string                  `json:"source_revision"`
		VerificationRunID   string                  `json:"verification_run_id"`
		VerificationID      string                  `json:"verification_occurrence_id"`
		AuditRunID          string                  `json:"audit_run_id"`
		AuditSourceRevision string                  `json:"audit_source_revision"`
		Evidence            []OptionalRoleEvidence  `json:"evidence"`
		SelectedEvidenceIDs []string                `json:"selected_evidence_ids"`
	}{a.SchemaVersion, a.TaskID, a.Role, a.Disposition, a.TaskSource, a.StateSHA256, a.SourceRevision, a.VerificationRunID, a.VerificationID, a.AuditRunID, a.AuditSourceRevision, evidence, selected})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

func (g OptionalRoleGate) Validate() error {
	if !validStateSHA256(g.SourceRevision) || strings.TrimSpace(g.VerificationRunID) == "" || strings.TrimSpace(g.VerificationOccurrenceID) == "" || strings.TrimSpace(g.AuditSupervisorRunID) == "" || strings.TrimSpace(g.AuditWorkerRunID) == "" || g.AuditRevision < 1 {
		return errors.New("optional-role gate requires exact source, verification, and audit identities")
	}
	ids := []string{g.VerificationRunID, g.AuditSupervisorRunID, g.AuditWorkerRunID}
	sort.Strings(ids)
	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			return errors.New("optional-role gate requires independent verification and audit runs")
		}
	}
	return nil
}

func (w OptionalRoleWorkerEvidence) Validate() error {
	if strings.TrimSpace(w.AttemptID) == "" || strings.TrimSpace(w.RunID) == "" || !validStateSHA256(w.DossierSHA256) || w.DossierByteSize <= 0 || strings.TrimSpace(w.ProfilePath) == "" || !validStateSHA256(w.ProfileSHA256) || w.ProfileByteSize <= 0 {
		return errors.New("optional-role worker evidence is incomplete")
	}
	if err := validateEvidenceReferences("optional-role worker receipt", []EvidenceReference{w.Receipt}); err != nil || w.Receipt.Kind != EvidenceKindReceipt {
		return errors.New("optional-role worker receipt evidence is invalid")
	}
	if err := validateEvidenceReferences("optional-role worker ledger", []EvidenceReference{w.Ledger}); err != nil || w.Ledger.Kind != EvidenceKindLedger {
		return errors.New("optional-role worker ledger evidence is invalid")
	}
	return nil
}

func (o OptionalRoleOccurrence) Validate() error {
	if o.SchemaVersion != OptionalRoleOccurrenceSchemaVersion || o.Sequence < 1 || strings.TrimSpace(o.TaskID) == "" || !optionalRole(o.Role) || !validStateSHA256(o.AssessmentSHA256) || !validStateSHA256(o.SourceBefore) || !validStateSHA256(o.SourceAfter) || o.CreatedAt.IsZero() {
		return errors.New("validate optional-role occurrence: schema, sequence, task, role, source, assessment, and time are required")
	}
	if err := o.Decision.Validate(); err != nil || o.Decision.TaskID != o.TaskID {
		return errors.New("validate optional-role occurrence: decision reference is invalid")
	}
	wantAction, wantProfile := optionalRoleAction(o.Role)
	if o.Decision.Action != wantAction || o.Decision.WorkerProfile != wantProfile {
		return errors.New("validate optional-role occurrence: decision does not match role")
	}
	if err := o.Gate.Validate(); err != nil || o.Gate.SourceRevision != o.SourceAfter {
		return errors.New("validate optional-role occurrence: current gate is invalid or stale")
	}
	if err := validateEvidenceReferences("validate optional-role occurrence: evidence", o.Evidence); err != nil || strings.TrimSpace(o.Rationale) == "" {
		return errors.New("validate optional-role occurrence: evidence and rationale are required")
	}
	if err := validateOptionalEvidenceReferences("validate optional-role occurrence: behavior preservation", o.BehaviorPreservation); err != nil {
		return err
	}
	switch o.Outcome {
	case OptionalRoleOutcomeNotApplicable:
		if o.SourceBefore != o.SourceAfter || o.Worker != nil || len(o.ChangedPaths) != 0 || o.CommitSHA != "" || len(o.BehaviorPreservation) != 0 {
			return errors.New("validate optional-role occurrence: not_applicable contains execution or source-change evidence")
		}
	case OptionalRoleOutcomeNoChange:
		if o.SourceBefore != o.SourceAfter || o.Worker == nil || len(o.ChangedPaths) != 0 || o.CommitSHA != "" || len(o.BehaviorPreservation) != 0 {
			return errors.New("validate optional-role occurrence: no_change requires one worker and unchanged source")
		}
	case OptionalRoleOutcomeSourceChanged:
		if o.SourceBefore == o.SourceAfter || o.Worker == nil || len(o.ChangedPaths) == 0 || strings.TrimSpace(o.CommitSHA) == "" {
			return errors.New("validate optional-role occurrence: source_changed requires worker, paths, commit, and distinct source")
		}
		seen := map[string]struct{}{}
		for i, path := range o.ChangedPaths {
			if strings.TrimSpace(path) == "" || path != strings.TrimSpace(path) {
				return fmt.Errorf("validate optional-role occurrence: changed_paths[%d] is malformed", i)
			}
			if _, ok := seen[path]; ok {
				return fmt.Errorf("validate optional-role occurrence: duplicate changed path %q", path)
			}
			seen[path] = struct{}{}
		}
		if o.Role == WorkerProfileSimplifier && len(o.BehaviorPreservation) == 0 {
			return errors.New("validate optional-role occurrence: source-changing simplification requires behavior-preservation evidence")
		}
	default:
		return fmt.Errorf("validate optional-role occurrence: unknown outcome %q", o.Outcome)
	}
	if o.Worker != nil {
		if err := o.Worker.Validate(); err != nil {
			return fmt.Errorf("validate optional-role occurrence: %w", err)
		}
		for _, id := range []string{o.Gate.AuditSupervisorRunID, o.Gate.AuditWorkerRunID} {
			if o.Outcome == OptionalRoleOutcomeSourceChanged && o.Worker.RunID == id {
				return errors.New("validate optional-role occurrence: worker, verification, and audit runs must be independent")
			}
		}
	}
	return nil
}

func optionalRole(role WorkerProfile) bool {
	return role == WorkerProfileDocumentor || role == WorkerProfileSimplifier
}

func optionalRoleAction(role WorkerProfile) (Action, WorkerProfile) {
	if role == WorkerProfileDocumentor {
		return ActionDocument, WorkerProfileDocumentor
	}
	return ActionSimplify, WorkerProfileSimplifier
}

func optionalEvidenceKind(kind OptionalRoleEvidenceKind) bool {
	switch kind {
	case OptionalRoleEvidenceTaskDocumentation, OptionalRoleEvidenceAcceptanceDocumentation, OptionalRoleEvidencePlanDocumentation, OptionalRoleEvidenceAuditDocumentation, OptionalRoleEvidenceUserFacingChange, OptionalRoleEvidenceComplexityTarget, OptionalRoleEvidenceDuplicationTarget, OptionalRoleEvidenceMaintainabilityTarget, OptionalRoleEvidenceNoRelevantWork:
		return true
	default:
		return false
	}
}

func optionalEvidenceRelevant(role WorkerProfile, kind OptionalRoleEvidenceKind) bool {
	if role == WorkerProfileDocumentor {
		switch kind {
		case OptionalRoleEvidenceTaskDocumentation, OptionalRoleEvidenceAcceptanceDocumentation, OptionalRoleEvidencePlanDocumentation, OptionalRoleEvidenceAuditDocumentation, OptionalRoleEvidenceUserFacingChange:
			return true
		}
		return false
	}
	switch kind {
	case OptionalRoleEvidenceComplexityTarget, OptionalRoleEvidenceDuplicationTarget, OptionalRoleEvidenceMaintainabilityTarget:
		return true
	default:
		return false
	}
}

func containsEvidenceReference(values []EvidenceReference, target EvidenceReference) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
