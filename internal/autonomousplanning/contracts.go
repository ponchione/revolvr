// Package autonomousplanning owns the structured planner-output contract and
// the pure rules for applying one complete plan and acceptance-matrix proposal
// to an AW-02 execution state.
package autonomousplanning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"unicode/utf8"

	"revolvr/internal/autonomous"
)

const PlanningOutputSchemaVersion = "autonomous-planning-output-v1"

type DossierIdentity struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	SHA256        string `json:"sha256"`
	ByteSize      int    `json:"byte_size"`
}

type ProfileIdentity struct {
	Name     autonomous.WorkerProfile `json:"name"`
	Path     string                   `json:"path"`
	SHA256   string                   `json:"sha256"`
	ByteSize int                      `json:"byte_size"`
}

type PlanningProvenance struct {
	Action         autonomous.Action            `json:"action"`
	WorkerProfile  autonomous.WorkerProfile     `json:"worker_profile"`
	WorkerRunID    string                       `json:"worker_run_id"`
	Decision       autonomous.DecisionReference `json:"decision_reference"`
	Dossier        DossierIdentity              `json:"dossier"`
	Profile        ProfileIdentity              `json:"profile"`
	RawOutputPath  string                       `json:"raw_output_path"`
	SourceRevision string                       `json:"source_revision"`
}

// PlanningOutput is the complete proposed current plan revision and complete
// current acceptance matrix. It contains no commands or persistence request.
type PlanningOutput struct {
	SchemaVersion      string                           `json:"schema_version"`
	TaskID             string                           `json:"task_id"`
	Plan               autonomous.TaskPlan              `json:"plan"`
	AcceptanceCriteria []autonomous.AcceptanceCriterion `json:"acceptance_criteria"`
	Inputs             []autonomous.EvidenceReference   `json:"inputs"`
	Provenance         PlanningProvenance               `json:"provenance"`
}

type ChangeKind string

const (
	ChangeKindInitial  ChangeKind = "initial"
	ChangeKindRevision ChangeKind = "revision"
)

func (o PlanningOutput) Validate() error {
	if o.SchemaVersion != PlanningOutputSchemaVersion {
		return fmt.Errorf("validate planning output: unsupported schema_version %q (want %q)", o.SchemaVersion, PlanningOutputSchemaVersion)
	}
	if err := validateIdentity("task_id", o.TaskID); err != nil {
		return fmt.Errorf("validate planning output: %w", err)
	}
	if err := o.Plan.Validate(); err != nil {
		return fmt.Errorf("validate planning output: plan: %w", err)
	}
	if o.Plan.TaskID != o.TaskID {
		return fmt.Errorf("validate planning output: plan task_id %q does not match output task_id %q", o.Plan.TaskID, o.TaskID)
	}
	if len(o.AcceptanceCriteria) == 0 {
		return errors.New("validate planning output: acceptance_criteria requires at least one criterion")
	}
	criterionIDs := make(map[string]struct{}, len(o.AcceptanceCriteria))
	requirements := make(map[string]string, len(o.AcceptanceCriteria))
	for i, criterion := range o.AcceptanceCriteria {
		if err := criterion.Validate(); err != nil {
			return fmt.Errorf("validate planning output: acceptance_criteria[%d]: %w", i, err)
		}
		if _, exists := criterionIDs[criterion.ID]; exists {
			return fmt.Errorf("validate planning output: acceptance_criteria[%d]: duplicate criterion id %q", i, criterion.ID)
		}
		criterionIDs[criterion.ID] = struct{}{}
		normalizedRequirement := strings.Join(strings.Fields(criterion.Requirement), " ")
		if priorID, exists := requirements[normalizedRequirement]; exists {
			return fmt.Errorf("validate planning output: criterion ids %q and %q duplicate requirement %q", priorID, criterion.ID, normalizedRequirement)
		}
		requirements[normalizedRequirement] = criterion.ID
	}
	if err := validateEvidence("inputs", o.Inputs, true); err != nil {
		return fmt.Errorf("validate planning output: %w", err)
	}
	if err := o.Provenance.Validate(); err != nil {
		return fmt.Errorf("validate planning output: provenance: %w", err)
	}
	if o.Provenance.Decision.TaskID != o.TaskID || o.Provenance.Dossier.TaskID != o.TaskID {
		return errors.New("validate planning output: provenance task identity does not match output task_id")
	}
	return nil
}

func (p PlanningProvenance) Validate() error {
	if p.Action != autonomous.ActionPlan || p.WorkerProfile != autonomous.WorkerProfilePlanner {
		return fmt.Errorf("authorized route must be %q -> %q (got %q -> %q)", autonomous.ActionPlan, autonomous.WorkerProfilePlanner, p.Action, p.WorkerProfile)
	}
	if err := validateIdentity("worker_run_id", p.WorkerRunID); err != nil {
		return err
	}
	if err := p.Decision.Validate(); err != nil {
		return fmt.Errorf("decision_reference: %w", err)
	}
	if p.Decision.Action != p.Action || p.Decision.WorkerProfile != p.WorkerProfile {
		return errors.New("decision reference does not match authorized planner route")
	}
	if p.Decision.RunID == p.WorkerRunID {
		return errors.New("supervisor and planner worker run IDs must differ")
	}
	if err := p.Dossier.Validate(); err != nil {
		return fmt.Errorf("dossier: %w", err)
	}
	if err := p.Profile.Validate(); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	if err := validateRepositoryPath("raw_output_path", p.RawOutputPath); err != nil {
		return err
	}
	if !validSHA256(p.SourceRevision) {
		return fmt.Errorf("source_revision %q is invalid", p.SourceRevision)
	}
	return nil
}

func (d DossierIdentity) Validate() error {
	if strings.TrimSpace(d.SchemaVersion) == "" {
		return errors.New("schema_version is required")
	}
	if err := validateIdentity("task_id", d.TaskID); err != nil {
		return err
	}
	if !validSHA256(d.SHA256) {
		return fmt.Errorf("SHA-256 %q is invalid", d.SHA256)
	}
	if d.ByteSize <= 0 {
		return fmt.Errorf("byte_size must be positive (got %d)", d.ByteSize)
	}
	return nil
}

func (p ProfileIdentity) Validate() error {
	if p.Name != autonomous.WorkerProfilePlanner {
		return fmt.Errorf("name must be %q (got %q)", autonomous.WorkerProfilePlanner, p.Name)
	}
	if err := validateRepositoryPath("path", p.Path); err != nil {
		return err
	}
	if !validSHA256(p.SHA256) {
		return fmt.Errorf("SHA-256 %q is invalid", p.SHA256)
	}
	if p.ByteSize <= 0 {
		return fmt.Errorf("byte_size must be positive (got %d)", p.ByteSize)
	}
	return nil
}

// CanonicalTaskOrigin is the exact task/specification origin accepted by the
// AW-11 acceptance validator. The source hash prevents a path-only citation
// from silently referring to different task bytes.
func CanonicalTaskOrigin(sourcePath, sourceSHA256 string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{
		Kind:      autonomous.EvidenceKindTask,
		Reference: sourcePath + "#sha256=" + sourceSHA256,
		Detail:    "Exact canonical task/specification bytes used for autonomous planning.",
	}
}

// ApplyProposal validates AW-11-specific creation/revision and origin rules,
// then returns a deep-cloned next AW-02 state. AW-02 transition validation is
// the final authority for general state invariants.
func ApplyProposal(previous autonomous.ExecutionState, output PlanningOutput, decision autonomous.SupervisorDecision, taskOrigin autonomous.EvidenceReference, taskSource []byte) (autonomous.ExecutionState, ChangeKind, error) {
	if err := previous.Validate(); err != nil {
		return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: previous state: %w", err)
	}
	if err := output.Validate(); err != nil {
		return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: %w", err)
	}
	if err := decision.Validate(); err != nil {
		return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: supervisor decision: %w", err)
	}
	if decision.TaskID != previous.TaskID || output.TaskID != previous.TaskID {
		return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: task identity mismatch")
	}
	if decision.Action != autonomous.ActionPlan || decision.WorkerProfile != autonomous.WorkerProfilePlanner {
		return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: supervisor decision is not an authorized plan -> planner route")
	}
	if output.Provenance.Decision.Action != decision.Action || output.Provenance.Decision.WorkerProfile != decision.WorkerProfile {
		return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: output decision provenance does not match supervisor decision")
	}
	if !containsEvidence(output.Inputs, taskOrigin) || !containsEvidence(output.Inputs, output.Provenance.Decision.Artifact) {
		return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: inputs must contain the exact task source and supervisor decision artifact")
	}
	if !containsEvidence(output.Plan.Provenance, taskOrigin) || !containsEvidence(output.Plan.Provenance, output.Provenance.Decision.Artifact) {
		return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: plan provenance must contain the exact task source and supervisor decision artifact")
	}
	if err := validateAcceptanceOrigins(previous.AcceptanceCriteria, output.AcceptanceCriteria, decision, output.Provenance.Decision.Artifact, taskOrigin, taskSource); err != nil {
		return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: acceptance matrix: %w", err)
	}

	kind := ChangeKindInitial
	if previous.Plan == nil {
		if previous.Lifecycle != autonomous.LifecycleStatePending {
			return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: initial plan requires pending lifecycle (got %q)", previous.Lifecycle)
		}
		if output.Plan.Revision != 1 || output.Plan.SupersedesPlanID != "" {
			return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: initial plan must be revision 1 with no predecessor")
		}
		if output.Plan.Completed {
			return autonomous.ExecutionState{}, "", errors.New("apply planning proposal: initial plan must not be completed")
		}
		for _, step := range output.Plan.Steps {
			if step.Status != autonomous.PlanStepStatusPending {
				return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: new initial step %q must be pending (got %q)", step.ID, step.Status)
			}
		}
	} else {
		kind = ChangeKindRevision
		if previous.Lifecycle != autonomous.LifecycleStateReady {
			return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: plan revision requires ready lifecycle (got %q)", previous.Lifecycle)
		}
		if err := validatePlanRevision(*previous.Plan, output.Plan); err != nil {
			return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: plan revision: %w", err)
		}
	}

	next, err := cloneState(previous)
	if err != nil {
		return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: clone state: %w", err)
	}
	plan := output.Plan
	plan.Provenance = append([]autonomous.EvidenceReference(nil), output.Plan.Provenance...)
	plan.Steps = clonePlanSteps(output.Plan.Steps)
	next.Plan = &plan
	next.AcceptanceCriteria = cloneAcceptance(output.AcceptanceCriteria)
	decisionReference := output.Provenance.Decision
	next.LatestDecision = &decisionReference
	next.Lifecycle = autonomous.LifecycleStateReady
	if err := autonomous.ValidateExecutionStateTransition(previous, next); err != nil {
		return autonomous.ExecutionState{}, "", fmt.Errorf("apply planning proposal: %w", err)
	}
	return next, kind, nil
}

func validatePlanRevision(previous, next autonomous.TaskPlan) error {
	if next.ID == previous.ID {
		return errors.New("new revision requires a new plan ID")
	}
	if next.Revision != previous.Revision+1 {
		return fmt.Errorf("revision is %d, want %d", next.Revision, previous.Revision+1)
	}
	if next.SupersedesPlanID != previous.ID {
		return fmt.Errorf("supersedes_plan_id is %q, want %q", next.SupersedesPlanID, previous.ID)
	}
	previousByID := make(map[string]autonomous.PlanStep, len(previous.Steps))
	terminalOrder := make([]string, 0, len(previous.Steps))
	for _, step := range previous.Steps {
		previousByID[step.ID] = step
		if terminalStep(step.Status) {
			terminalOrder = append(terminalOrder, step.ID)
		}
	}
	nextTerminalOrder := make([]string, 0, len(terminalOrder))
	for _, step := range next.Steps {
		prior, exists := previousByID[step.ID]
		if !exists {
			if terminalStep(step.Status) {
				return fmt.Errorf("new step %q cannot begin in terminal status %q", step.ID, step.Status)
			}
			continue
		}
		if prior.Description != step.Description {
			return fmt.Errorf("step ID %q was reused for a materially different description", step.ID)
		}
		if !reflect.DeepEqual(prior, step) {
			return fmt.Errorf("existing step %q changed status, evidence, or rationale during planning", step.ID)
		}
		if terminalStep(prior.Status) {
			nextTerminalOrder = append(nextTerminalOrder, step.ID)
		}
	}
	if !reflect.DeepEqual(nextTerminalOrder, terminalOrder) {
		return fmt.Errorf("terminal step order changed or terminal work disappeared: got %v, want %v", nextTerminalOrder, terminalOrder)
	}
	return nil
}

func validateAcceptanceOrigins(previous, next []autonomous.AcceptanceCriterion, decision autonomous.SupervisorDecision, decisionArtifact autonomous.EvidenceReference, taskOrigin autonomous.EvidenceReference, taskSource []byte) error {
	if len(taskSource) == 0 || !utf8.Valid(taskSource) {
		return errors.New("canonical task source is missing or invalid UTF-8")
	}
	previousByID := make(map[string]autonomous.AcceptanceCriterion, len(previous))
	for _, criterion := range previous {
		previousByID[criterion.ID] = criterion
	}
	for _, criterion := range next {
		if prior, exists := previousByID[criterion.ID]; exists && !reflect.DeepEqual(prior, criterion) {
			return fmt.Errorf("existing criterion %q changed requirement, origin, status, evidence, or rationale", criterion.ID)
		}
		if criterion.Source == nil {
			return fmt.Errorf("criterion %q has no explicit origin", criterion.ID)
		}
		switch {
		case *criterion.Source == taskOrigin:
			if !strings.Contains(string(taskSource), criterion.Requirement) {
				return fmt.Errorf("criterion %q requirement is not an exact statement in the canonical task/specification", criterion.ID)
			}
		case *criterion.Source == decisionArtifact:
			if !containsString(decision.SuccessCriteria, criterion.Requirement) {
				return fmt.Errorf("criterion %q supervisor refinement does not exactly match cited success_criteria", criterion.ID)
			}
		default:
			return fmt.Errorf("criterion %q origin is neither the exact task source nor the current supervisor decision", criterion.ID)
		}
	}
	return nil
}

func MarshalPlanningOutput(output PlanningOutput) ([]byte, error) {
	if err := output.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal planning output: %w", err)
	}
	return append(raw, '\n'), nil
}

func cloneState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	var cloned autonomous.ExecutionState
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return autonomous.ExecutionState{}, err
	}
	return cloned, nil
}

func clonePlanSteps(steps []autonomous.PlanStep) []autonomous.PlanStep {
	result := make([]autonomous.PlanStep, len(steps))
	for i, step := range steps {
		result[i] = step
		result[i].Evidence = append([]autonomous.EvidenceReference(nil), step.Evidence...)
	}
	return result
}

func cloneAcceptance(criteria []autonomous.AcceptanceCriterion) []autonomous.AcceptanceCriterion {
	result := make([]autonomous.AcceptanceCriterion, len(criteria))
	for i, criterion := range criteria {
		result[i] = criterion
		result[i].Evidence = append([]autonomous.EvidenceReference(nil), criterion.Evidence...)
		if criterion.Source != nil {
			source := *criterion.Source
			result[i].Source = &source
		}
	}
	return result
}

func validateEvidence(label string, values []autonomous.EvidenceReference, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("%s requires at least one typed evidence reference", label)
	}
	for i, value := range values {
		if !validEvidenceKind(value.Kind) {
			return fmt.Errorf("%s[%d] has unknown kind %q", label, i, value.Kind)
		}
		if strings.TrimSpace(value.Reference) == "" || strings.TrimSpace(value.Detail) == "" {
			return fmt.Errorf("%s[%d] requires reference and detail", label, i)
		}
	}
	return nil
}

func validEvidenceKind(kind autonomous.EvidenceKind) bool {
	switch kind {
	case autonomous.EvidenceKindTask, autonomous.EvidenceKindPlan, autonomous.EvidenceKindLedger,
		autonomous.EvidenceKindReceipt, autonomous.EvidenceKindVerification, autonomous.EvidenceKindGit,
		autonomous.EvidenceKindAudit, autonomous.EvidenceKindRepository, autonomous.EvidenceKindFile:
		return true
	default:
		return false
	}
}

func validateIdentity(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s %q is empty or malformed", label, value)
	}
	return nil
}

func validateRepositoryPath(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.HasPrefix(value, "/") {
		return fmt.Errorf("%s %q is empty, absolute, or malformed", label, value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("%s %q is not a normalized repository-relative path", label, value)
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func containsEvidence(values []autonomous.EvidenceReference, target autonomous.EvidenceReference) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func terminalStep(status autonomous.PlanStepStatus) bool {
	return status == autonomous.PlanStepStatusCompleted || status == autonomous.PlanStepStatusSkipped
}
