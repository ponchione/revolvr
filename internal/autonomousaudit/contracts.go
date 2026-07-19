// Package autonomousaudit owns the structured auditor-output contract and
// pure audit/finding lifecycle transitions. It performs no repository I/O.
package autonomousaudit

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousverification"
)

const AuditOutputSchemaVersion = "autonomous-audit-output-v1"

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

type AuditProvenance struct {
	Action               autonomous.Action                     `json:"action"`
	WorkerProfile        autonomous.WorkerProfile              `json:"worker_profile"`
	WorkerRunID          string                                `json:"worker_run_id"`
	Decision             autonomous.DecisionReference          `json:"decision_reference"`
	Dossier              DossierIdentity                       `json:"dossier"`
	Profile              ProfileIdentity                       `json:"profile"`
	RawOutputPath        string                                `json:"raw_output_path"`
	SourceRevision       string                                `json:"source_revision"`
	Verification         autonomouspolicy.VerificationEvidence `json:"verification"`
	LatestSourceMutation *SourceMutationIdentity               `json:"latest_source_mutation,omitempty"`
}

type SourceMutationIdentity struct {
	TaskID            string            `json:"task_id"`
	RunID             string            `json:"run_id"`
	DecisionID        string            `json:"decision_id,omitempty"`
	Action            autonomous.Action `json:"action"`
	ResultingRevision string            `json:"resulting_revision"`
}

// modelAuditProvenance is the exact closed provenance projection copied by the
// model. The full tiered verification result is trusted host evidence, so the
// model emits null for verification.summary.tiered and the apply boundary
// reattaches the already-validated host value after comparing this projection.
type modelAuditProvenance struct {
	Action               autonomous.Action            `json:"action"`
	WorkerProfile        autonomous.WorkerProfile     `json:"worker_profile"`
	WorkerRunID          string                       `json:"worker_run_id"`
	Decision             autonomous.DecisionReference `json:"decision_reference"`
	Dossier              DossierIdentity              `json:"dossier"`
	Profile              ProfileIdentity              `json:"profile"`
	RawOutputPath        string                       `json:"raw_output_path"`
	SourceRevision       string                       `json:"source_revision"`
	Verification         modelVerificationEvidence    `json:"verification"`
	LatestSourceMutation *modelSourceMutation         `json:"latest_source_mutation"`
}

type modelVerificationEvidence struct {
	Summary        modelVerificationSummary             `json:"summary"`
	SourceRevision string                               `json:"source_revision"`
	Tiered         *autonomousverification.GateEvidence `json:"tiered"`
}

type modelVerificationSummary struct {
	TaskID       string                         `json:"task_id"`
	Status       autonomous.VerificationStatus  `json:"status"`
	Command      *string                        `json:"command"`
	Summary      string                         `json:"summary"`
	RunID        string                         `json:"run_id"`
	OccurrenceID string                         `json:"occurrence_id"`
	Evidence     []autonomous.EvidenceReference `json:"evidence"`
	Tiered       *struct{}                      `json:"tiered"`
}

type modelSourceMutation struct {
	TaskID            string            `json:"task_id"`
	RunID             string            `json:"run_id"`
	DecisionID        *string           `json:"decision_id"`
	Action            autonomous.Action `json:"action"`
	ResultingRevision string            `json:"resulting_revision"`
}

// ModelVerificationProjection removes the trusted full tiered result from the
// model-authored envelope while retaining the exact closed final-gate value.
func ModelVerificationProjection(value autonomouspolicy.VerificationEvidence) autonomouspolicy.VerificationEvidence {
	projected := value
	projected.Summary.Tiered = nil
	return projected
}

// ModelProvenanceProjection returns the schema-shaped provenance value included
// in the auditor prompt, including explicit nulls for every semantic optional.
func ModelProvenanceProjection(value AuditProvenance) modelAuditProvenance {
	verification := ModelVerificationProjection(value.Verification)
	projected := modelAuditProvenance{
		Action:         value.Action,
		WorkerProfile:  value.WorkerProfile,
		WorkerRunID:    value.WorkerRunID,
		Decision:       value.Decision,
		Dossier:        value.Dossier,
		Profile:        value.Profile,
		RawOutputPath:  value.RawOutputPath,
		SourceRevision: value.SourceRevision,
		Verification: modelVerificationEvidence{
			Summary: modelVerificationSummary{
				TaskID:       verification.Summary.TaskID,
				Status:       verification.Summary.Status,
				Command:      optionalModelString(verification.Summary.Command),
				Summary:      verification.Summary.Summary,
				RunID:        verification.Summary.RunID,
				OccurrenceID: verification.Summary.OccurrenceID,
				Evidence:     verification.Summary.Evidence,
			},
			SourceRevision: verification.SourceRevision,
			Tiered:         verification.Tiered,
		},
	}
	if value.LatestSourceMutation != nil {
		projected.LatestSourceMutation = &modelSourceMutation{
			TaskID:            value.LatestSourceMutation.TaskID,
			RunID:             value.LatestSourceMutation.RunID,
			DecisionID:        optionalModelString(value.LatestSourceMutation.DecisionID),
			Action:            value.LatestSourceMutation.Action,
			ResultingRevision: value.LatestSourceMutation.ResultingRevision,
		}
	}
	return projected
}

func optionalModelString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func SourceMutationFromPolicy(value *autonomouspolicy.SourceMutation) *SourceMutationIdentity {
	if value == nil {
		return nil
	}
	return &SourceMutationIdentity{TaskID: value.TaskID, RunID: value.RunID, DecisionID: value.DecisionID, Action: value.Action, ResultingRevision: value.ResultingRevision}
}

func (s *SourceMutationIdentity) Policy() *autonomouspolicy.SourceMutation {
	if s == nil {
		return nil
	}
	return &autonomouspolicy.SourceMutation{TaskID: s.TaskID, RunID: s.RunID, DecisionID: s.DecisionID, Action: s.Action, ResultingRevision: s.ResultingRevision}
}

type AuditOutput struct {
	SchemaVersion string                 `json:"schema_version"`
	TaskID        string                 `json:"task_id"`
	Report        autonomous.AuditReport `json:"report"`
	Provenance    AuditProvenance        `json:"provenance"`
}

func (o AuditOutput) Validate() error {
	if o.SchemaVersion != AuditOutputSchemaVersion {
		return fmt.Errorf("validate audit output: unsupported schema_version %q (want %q)", o.SchemaVersion, AuditOutputSchemaVersion)
	}
	if err := validateIdentity("task_id", o.TaskID); err != nil {
		return fmt.Errorf("validate audit output: %w", err)
	}
	if err := o.Report.Validate(); err != nil {
		return fmt.Errorf("validate audit output: report: %w", err)
	}
	if o.Report.TaskID != o.TaskID {
		return fmt.Errorf("validate audit output: report task_id %q does not match output task_id %q", o.Report.TaskID, o.TaskID)
	}
	if err := o.Provenance.Validate(); err != nil {
		return fmt.Errorf("validate audit output: provenance: %w", err)
	}
	if o.Provenance.Decision.TaskID != o.TaskID || o.Provenance.Dossier.TaskID != o.TaskID || o.Provenance.Verification.Summary.TaskID != o.TaskID {
		return errors.New("validate audit output: provenance task identity does not match output task_id")
	}
	for _, evidence := range o.Provenance.Verification.Summary.Evidence {
		if !containsEvidence(o.Report.Inputs, evidence) {
			return errors.New("validate audit output: report inputs must cite every exact current verification evidence reference")
		}
	}
	return nil
}

func (p AuditProvenance) Validate() error {
	if p.Action != autonomous.ActionAudit || p.WorkerProfile != autonomous.WorkerProfileAuditor {
		return fmt.Errorf("authorized route must be %q -> %q (got %q -> %q)", autonomous.ActionAudit, autonomous.WorkerProfileAuditor, p.Action, p.WorkerProfile)
	}
	if err := validateIdentity("worker_run_id", p.WorkerRunID); err != nil {
		return err
	}
	if err := p.Decision.Validate(); err != nil {
		return fmt.Errorf("decision_reference: %w", err)
	}
	if p.Decision.Action != p.Action || p.Decision.WorkerProfile != p.WorkerProfile {
		return errors.New("decision reference does not match authorized auditor route")
	}
	if p.Decision.RunID == p.WorkerRunID {
		return errors.New("supervisor and auditor worker run IDs must differ")
	}
	if err := p.Dossier.Validate(); err != nil {
		return fmt.Errorf("dossier: %w", err)
	}
	if err := p.Profile.Validate(); err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	if err := validatePath("raw_output_path", p.RawOutputPath); err != nil {
		return err
	}
	if !validSHA256(p.SourceRevision) {
		return fmt.Errorf("source_revision %q is invalid", p.SourceRevision)
	}
	if err := autonomouspolicy.ValidateEvidence(p.Decision.TaskID, autonomouspolicy.SourceEvidence{Revision: p.SourceRevision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: p.LatestSourceMutation.Policy()}, &p.Verification, nil); err != nil {
		return fmt.Errorf("verification/source evidence: %w", err)
	}
	if p.Verification.SourceRevision != p.SourceRevision || p.Verification.Summary.Status != autonomous.VerificationStatusPassed {
		return errors.New("verification must be passed and tied to the audited source revision")
	}
	if p.Verification.Tiered != nil && !p.Verification.Tiered.FinalSatisfied {
		return errors.New("tiered verification must satisfy the final gate before audit")
	}
	if p.WorkerRunID == p.Verification.Summary.RunID {
		return errors.New("auditor worker and verification run IDs must differ")
	}
	if p.LatestSourceMutation != nil && p.WorkerRunID == p.LatestSourceMutation.RunID {
		return errors.New("auditor worker and latest source-mutating run IDs must differ")
	}
	return nil
}

func (d DossierIdentity) Validate() error {
	if strings.TrimSpace(d.SchemaVersion) == "" || d.ByteSize <= 0 || !validSHA256(d.SHA256) {
		return errors.New("schema_version, valid SHA-256, and positive byte_size are required")
	}
	return validateIdentity("task_id", d.TaskID)
}

func (p ProfileIdentity) Validate() error {
	if p.Name != autonomous.WorkerProfileAuditor {
		return fmt.Errorf("name must be %q (got %q)", autonomous.WorkerProfileAuditor, p.Name)
	}
	if err := validatePath("path", p.Path); err != nil {
		return err
	}
	if !validSHA256(p.SHA256) || p.ByteSize <= 0 {
		return errors.New("valid profile SHA-256 and positive byte_size are required")
	}
	return nil
}

type AuditChange struct {
	State            autonomous.ExecutionState
	NewFindingIDs    []string
	BlockingCount    int
	NonBlockingCount int
}

// ApplyReport applies one validated report while enforcing durable finding
// identity. PriorReports must contain committed reports only.
func ApplyReport(previous autonomous.ExecutionState, output AuditOutput, decision autonomous.SupervisorDecision, priorReports []autonomous.AuditReport) (AuditChange, error) {
	if err := previous.Validate(); err != nil {
		return AuditChange{}, fmt.Errorf("apply audit report: previous state: %w", err)
	}
	if previous.Lifecycle != autonomous.LifecycleStateReady {
		return AuditChange{}, fmt.Errorf("apply audit report: ready lifecycle is required (got %q)", previous.Lifecycle)
	}
	if err := output.Validate(); err != nil {
		return AuditChange{}, fmt.Errorf("apply audit report: %w", err)
	}
	if err := decision.Validate(); err != nil || decision.Action != autonomous.ActionAudit || decision.WorkerProfile != autonomous.WorkerProfileAuditor {
		return AuditChange{}, errors.New("apply audit report: supervisor decision is not an authorized audit -> auditor route")
	}
	if previous.TaskID != output.TaskID || decision.TaskID != output.TaskID {
		return AuditChange{}, errors.New("apply audit report: task or decision identity mismatch")
	}

	definitions := make(map[string]autonomous.AuditFinding)
	fingerprints := make(map[string]string)
	for _, report := range priorReports {
		if err := report.Validate(); err != nil || report.TaskID != previous.TaskID {
			return AuditChange{}, errors.New("apply audit report: prior committed report is invalid or belongs to another task")
		}
		for _, finding := range report.Findings {
			if prior, ok := definitions[finding.ID]; ok {
				if err := validateRepeatedFinding(prior, finding); err != nil {
					return AuditChange{}, err
				}
			}
			definitions[finding.ID] = finding
			fingerprints[findingFingerprint(finding)] = finding.ID
		}
	}
	statusByID := make(map[string]autonomous.FindingResolutionStatus, len(previous.FindingResolutions))
	for _, resolution := range previous.FindingResolutions {
		statusByID[resolution.FindingID] = resolution.Status
	}
	reported := make(map[string]struct{}, len(output.Report.Findings))
	for _, finding := range output.Report.Findings {
		reported[finding.ID] = struct{}{}
		if prior, ok := definitions[finding.ID]; ok {
			if statusByID[finding.ID] != autonomous.FindingResolutionStatusOpen {
				return AuditChange{}, fmt.Errorf("apply audit report: terminal finding %q cannot be reused or reopened", finding.ID)
			}
			if err := validateRepeatedFinding(prior, finding); err != nil {
				return AuditChange{}, err
			}
		} else if priorID, ok := fingerprints[findingFingerprint(finding)]; ok && statusByID[priorID] == autonomous.FindingResolutionStatusOpen {
			return AuditChange{}, fmt.Errorf("apply audit report: finding %q is a probable rename of open finding %q without explicit supersession", finding.ID, priorID)
		}
	}
	missingOpenFindingIDs := make([]string, 0)
	for id, status := range statusByID {
		if status == autonomous.FindingResolutionStatusOpen {
			if _, ok := reported[id]; !ok {
				missingOpenFindingIDs = append(missingOpenFindingIDs, id)
			}
		}
	}
	sort.Strings(missingOpenFindingIDs)
	if len(missingOpenFindingIDs) > 0 {
		return AuditChange{}, fmt.Errorf("apply audit report: open finding %q disappeared from the current report", missingOpenFindingIDs[0])
	}

	next, err := cloneState(previous)
	if err != nil {
		return AuditChange{}, err
	}
	change := AuditChange{}
	for _, finding := range output.Report.Findings {
		if _, exists := statusByID[finding.ID]; !exists {
			next.FindingResolutions = append(next.FindingResolutions, autonomous.FindingResolution{FindingID: finding.ID, Status: autonomous.FindingResolutionStatusOpen})
			change.NewFindingIDs = append(change.NewFindingIDs, finding.ID)
		}
		if finding.Significance == autonomous.FindingSignificanceBlocking {
			change.BlockingCount++
		} else {
			change.NonBlockingCount++
		}
	}
	reference := output.Provenance.Decision
	next.LatestDecision = &reference
	next.Lifecycle = autonomous.LifecycleStateReady
	if err := autonomous.ValidateExecutionStateTransition(previous, next); err != nil {
		return AuditChange{}, fmt.Errorf("apply audit report: %w", err)
	}
	change.State = next
	return change, nil
}

type ResolutionRequest struct {
	FindingID               string
	Status                  autonomous.FindingResolutionStatus
	Evidence                []autonomous.EvidenceReference
	Rationale               string
	SupersedingFindingID    string
	CorrectionDecision      *autonomous.SupervisorDecision
	DecisionReference       *autonomous.DecisionReference
	Verification            *autonomouspolicy.VerificationEvidence
	ResultingSourceRevision string
}

func ApplyResolution(previous autonomous.ExecutionState, request ResolutionRequest, findings []autonomous.AuditFinding) (autonomous.ExecutionState, autonomous.FindingResolution, error) {
	if err := previous.Validate(); err != nil {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, err
	}
	if previous.Lifecycle != autonomous.LifecycleStateReady {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: ready lifecycle is required")
	}
	known := make(map[string]autonomous.AuditFinding, len(findings))
	for _, finding := range findings {
		known[finding.ID] = finding
	}
	if _, ok := known[request.FindingID]; !ok {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: unknown finding %q", request.FindingID)
	}
	index := -1
	for i, current := range previous.FindingResolutions {
		if current.FindingID == request.FindingID {
			index = i
			if current.Status != autonomous.FindingResolutionStatusOpen {
				return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: finding %q is already terminal as %q", request.FindingID, current.Status)
			}
			break
		}
	}
	if index < 0 {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: finding %q has no open durable resolution", request.FindingID)
	}
	if request.Status == autonomous.FindingResolutionStatusOpen {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: target status must be terminal")
	}
	if len(request.Evidence) == 0 {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: %s requires typed evidence", request.Status)
	}
	resolution := autonomous.FindingResolution{FindingID: request.FindingID, Status: request.Status, Evidence: append([]autonomous.EvidenceReference(nil), request.Evidence...), Rationale: request.Rationale, SupersedingFindingID: request.SupersedingFindingID}
	if request.DecisionReference != nil {
		ref := *request.DecisionReference
		resolution.Resolution = &ref
	}
	switch request.Status {
	case autonomous.FindingResolutionStatusResolved:
		if request.CorrectionDecision != nil {
			if request.DecisionReference == nil || request.CorrectionDecision.Action != autonomous.ActionCorrect || !containsString(request.CorrectionDecision.FindingIDs, request.FindingID) || request.DecisionReference.Action != autonomous.ActionCorrect || request.DecisionReference.DecisionID == "" {
				return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: correction decision/reference must exactly cite the resolved finding")
			}
			if err := request.CorrectionDecision.Validate(); err != nil {
				return autonomous.ExecutionState{}, autonomous.FindingResolution{}, err
			}
		}
		if request.Verification != nil {
			if request.Verification.Summary.Status != autonomous.VerificationStatusPassed || request.Verification.SourceRevision == "" || request.Verification.SourceRevision != request.ResultingSourceRevision {
				return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: supplied verification must be passed and current for the resulting source")
			}
			for _, evidence := range request.Verification.Summary.Evidence {
				if !containsEvidence(request.Evidence, evidence) {
					return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: resolution evidence must retain exact supplied verification evidence")
				}
			}
		}
	case autonomous.FindingResolutionStatusWaived, autonomous.FindingResolutionStatusInvalid:
		if strings.TrimSpace(request.Rationale) == "" {
			return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: %s requires a concrete rationale", request.Status)
		}
	case autonomous.FindingResolutionStatusSuperseded:
		if request.SupersedingFindingID == "" || request.SupersedingFindingID == request.FindingID {
			return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: superseded requires a different target finding")
		}
		if _, ok := known[request.SupersedingFindingID]; !ok {
			return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: unknown superseding finding %q", request.SupersedingFindingID)
		}
		status, ok := resolutionStatus(previous, request.SupersedingFindingID)
		if !ok || status == autonomous.FindingResolutionStatusInvalid || status == autonomous.FindingResolutionStatusSuperseded {
			return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: superseding target is absent or contradictory")
		}
		if supersessionReaches(previous, request.SupersedingFindingID, request.FindingID) {
			return autonomous.ExecutionState{}, autonomous.FindingResolution{}, errors.New("apply finding resolution: supersession cycle detected")
		}
	default:
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, fmt.Errorf("apply finding resolution: unsupported terminal status %q", request.Status)
	}
	if err := resolution.Validate(); err != nil {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, err
	}
	next, err := cloneState(previous)
	if err != nil {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, err
	}
	next.FindingResolutions[index] = resolution
	if request.DecisionReference != nil {
		ref := *request.DecisionReference
		next.LatestDecision = &ref
	}
	next.Lifecycle = autonomous.LifecycleStateReady
	if err := autonomous.ValidateExecutionStateTransition(previous, next); err != nil {
		return autonomous.ExecutionState{}, autonomous.FindingResolution{}, err
	}
	return next, resolution, nil
}

func validateRepeatedFinding(prior, next autonomous.AuditFinding) error {
	if prior.Significance != next.Significance {
		return fmt.Errorf("apply audit report: reused finding %q changed significance", next.ID)
	}
	if normalizeMeaning(prior.Summary) != normalizeMeaning(next.Summary) {
		return fmt.Errorf("apply audit report: reused finding %q changed summary meaning", next.ID)
	}
	if normalizeMeaning(prior.RequiredCorrection) != normalizeMeaning(next.RequiredCorrection) {
		return fmt.Errorf("apply audit report: reused finding %q changed required correction meaning", next.ID)
	}
	if len(next.Evidence) < len(prior.Evidence) || !reflect.DeepEqual(next.Evidence[:len(prior.Evidence)], prior.Evidence) {
		return fmt.Errorf("apply audit report: reused finding %q removed or reordered prior evidence", next.ID)
	}
	return nil
}

func findingFingerprint(f autonomous.AuditFinding) string {
	return normalizeMeaning(f.Summary) + "\x00" + normalizeMeaning(f.RequiredCorrection)
}
func normalizeMeaning(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
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
func resolutionStatus(state autonomous.ExecutionState, id string) (autonomous.FindingResolutionStatus, bool) {
	for _, value := range state.FindingResolutions {
		if value.FindingID == id {
			return value.Status, true
		}
	}
	return "", false
}
func supersessionReaches(state autonomous.ExecutionState, start, target string) bool {
	seen := map[string]bool{}
	for start != "" && !seen[start] {
		if start == target {
			return true
		}
		seen[start] = true
		next := ""
		for _, value := range state.FindingResolutions {
			if value.FindingID == start && value.Status == autonomous.FindingResolutionStatusSuperseded {
				next = value.SupersedingFindingID
				break
			}
		}
		start = next
	}
	return false
}
func cloneState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	var result autonomous.ExecutionState
	err = json.Unmarshal(raw, &result)
	return result, err
}
