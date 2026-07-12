package autonomous

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Action string

const (
	ActionPlan      Action = "plan"
	ActionImplement Action = "implement"
	ActionAudit     Action = "audit"
	ActionCorrect   Action = "correct"
	ActionDocument  Action = "document"
	ActionSimplify  Action = "simplify"
	ActionComplete  Action = "complete"
	ActionBlock     Action = "block"
)

type WorkerProfile string

const (
	WorkerProfilePlanner     WorkerProfile = "planner"
	WorkerProfileImplementer WorkerProfile = "implementer"
	WorkerProfileAuditor     WorkerProfile = "auditor"
	WorkerProfileCorrector   WorkerProfile = "corrector"
	WorkerProfileDocumentor  WorkerProfile = "documentor"
	WorkerProfileSimplifier  WorkerProfile = "simplifier"
)

type EvidenceKind string

const (
	EvidenceKindTask         EvidenceKind = "task"
	EvidenceKindPlan         EvidenceKind = "plan"
	EvidenceKindLedger       EvidenceKind = "ledger"
	EvidenceKindReceipt      EvidenceKind = "receipt"
	EvidenceKindVerification EvidenceKind = "verification"
	EvidenceKindGit          EvidenceKind = "git"
	EvidenceKindAudit        EvidenceKind = "audit"
	EvidenceKindRepository   EvidenceKind = "repository"
	EvidenceKindFile         EvidenceKind = "file"
)

type AuditDisposition string

const (
	AuditDispositionClean           AuditDisposition = "clean"
	AuditDispositionChangesRequired AuditDisposition = "changes_required"
)

type FindingSignificance string

const (
	FindingSignificanceBlocking    FindingSignificance = "blocking"
	FindingSignificanceNonBlocking FindingSignificance = "non_blocking"
)

type EvidenceReference struct {
	Kind      EvidenceKind `json:"kind"`
	Reference string       `json:"reference"`
	Detail    string       `json:"detail"`
}

type SupervisorDecision struct {
	TaskID              string                     `json:"task_id"`
	Action              Action                     `json:"action"`
	WorkerProfile       WorkerProfile              `json:"worker_profile,omitempty"`
	Rationale           string                     `json:"rationale"`
	SuccessCriteria     []string                   `json:"success_criteria,omitempty"`
	Inputs              []EvidenceReference        `json:"inputs"`
	FindingIDs          []string                   `json:"finding_ids,omitempty"`
	VerificationFailure *VerificationFailureTarget `json:"verification_failure,omitempty"`
	Strategy            *Strategy                  `json:"strategy,omitempty"`
}

// Strategy is validated structured retry material. Runtime accounting hashes
// its normalized meaning; decision IDs, timestamps, and rationale prose are not
// strategy authority.
type Strategy struct {
	Approach   string              `json:"approach"`
	Techniques []string            `json:"techniques,omitempty"`
	Targets    []EvidenceReference `json:"targets,omitempty"`
}

type AuditReport struct {
	TaskID      string              `json:"task_id"`
	Disposition AuditDisposition    `json:"disposition"`
	Rationale   string              `json:"rationale"`
	Inputs      []EvidenceReference `json:"inputs"`
	Findings    []AuditFinding      `json:"findings,omitempty"`
}

type AuditFinding struct {
	ID                 string              `json:"id"`
	Significance       FindingSignificance `json:"significance"`
	Summary            string              `json:"summary"`
	Evidence           []EvidenceReference `json:"evidence"`
	RequiredCorrection string              `json:"required_correction"`
}

func (d SupervisorDecision) Validate() error {
	if strings.TrimSpace(d.TaskID) == "" {
		return errors.New("validate supervisor decision: task_id is required")
	}
	if !validAction(d.Action) {
		return fmt.Errorf("validate supervisor decision: unknown action %q", d.Action)
	}
	if strings.TrimSpace(d.Rationale) == "" {
		return errors.New("validate supervisor decision: rationale is required")
	}
	if err := validateEvidenceReferences("validate supervisor decision: inputs", d.Inputs); err != nil {
		return err
	}
	if d.Strategy != nil {
		if err := d.Strategy.Validate(); err != nil {
			return fmt.Errorf("validate supervisor decision: strategy: %w", err)
		}
	}

	expectedProfile, workerAction := workerProfileForAction(d.Action)
	if workerAction {
		if WorkerProfile(strings.TrimSpace(string(d.WorkerProfile))) != expectedProfile {
			return fmt.Errorf("validate supervisor decision: action %q requires compatible worker_profile %q (got %q)", d.Action, expectedProfile, d.WorkerProfile)
		}
		if err := validateSuccessCriteria(d.SuccessCriteria); err != nil {
			return err
		}
	} else {
		if d.Strategy != nil {
			return fmt.Errorf("validate supervisor decision: terminal action %q must not include strategy", d.Action)
		}
		if strings.TrimSpace(string(d.WorkerProfile)) != "" {
			return fmt.Errorf("validate supervisor decision: terminal action %q must not select worker_profile %q", d.Action, d.WorkerProfile)
		}
		if err := validateOptionalSuccessCriteria(d.SuccessCriteria); err != nil {
			return err
		}
	}

	if d.Action == ActionCorrect {
		if len(d.FindingIDs) == 0 && d.VerificationFailure == nil {
			return errors.New("validate supervisor decision: correct action requires at least one finding_id or one verification_failure")
		}
		if len(d.FindingIDs) != 0 && d.VerificationFailure != nil {
			return errors.New("validate supervisor decision: correct action requires exactly one audit-finding or verification-failure authority")
		}
		if len(d.FindingIDs) != 0 {
			if err := validateFindingIDs("validate supervisor decision: finding_ids", d.FindingIDs); err != nil {
				return err
			}
		} else if err := d.VerificationFailure.Validate(); err != nil {
			return fmt.Errorf("validate supervisor decision: verification_failure: %w", err)
		}
	} else if len(d.FindingIDs) != 0 {
		return fmt.Errorf("validate supervisor decision: finding_ids are only valid for action %q", ActionCorrect)
	} else if d.VerificationFailure != nil {
		return fmt.Errorf("validate supervisor decision: verification_failure is only valid for action %q", ActionCorrect)
	}

	return nil
}

func (s Strategy) Validate() error {
	if strings.TrimSpace(s.Approach) == "" {
		return errors.New("approach is required")
	}
	seen := make(map[string]struct{}, len(s.Techniques))
	for i, technique := range s.Techniques {
		normalized := strings.ToLower(strings.Join(strings.Fields(technique), " "))
		if normalized == "" {
			return fmt.Errorf("techniques[%d] is empty", i)
		}
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("techniques[%d] duplicates materially identical strategy technique", i)
		}
		seen[normalized] = struct{}{}
	}
	if err := validateOptionalEvidenceReferences("strategy targets", s.Targets); err != nil {
		return err
	}
	return nil
}

func (r AuditReport) Validate() error {
	if strings.TrimSpace(r.TaskID) == "" {
		return errors.New("validate audit report: task_id is required")
	}
	if !validAuditDisposition(r.Disposition) {
		return fmt.Errorf("validate audit report: unknown disposition %q", r.Disposition)
	}
	if strings.TrimSpace(r.Rationale) == "" {
		return errors.New("validate audit report: rationale is required")
	}
	if err := validateEvidenceReferences("validate audit report: inputs", r.Inputs); err != nil {
		return err
	}

	switch r.Disposition {
	case AuditDispositionClean:
		if len(r.Findings) != 0 {
			return errors.New("validate audit report: clean disposition must not include findings")
		}
	case AuditDispositionChangesRequired:
		if len(r.Findings) == 0 {
			return errors.New("validate audit report: changes_required disposition requires at least one finding")
		}
	}

	seen := make(map[string]struct{}, len(r.Findings))
	for i, finding := range r.Findings {
		if err := finding.Validate(); err != nil {
			return fmt.Errorf("validate audit report: findings[%d]: %w", i, err)
		}
		if _, exists := seen[finding.ID]; exists {
			return fmt.Errorf("validate audit report: duplicate finding id %q", finding.ID)
		}
		seen[finding.ID] = struct{}{}
	}
	return nil
}

func (f AuditFinding) Validate() error {
	if !validFindingID(f.ID) {
		return fmt.Errorf("invalid finding id %q (want lower-case kebab-case beginning with a letter)", f.ID)
	}
	if !validFindingSignificance(f.Significance) {
		return fmt.Errorf("unknown significance %q", f.Significance)
	}
	if strings.TrimSpace(f.Summary) == "" {
		return errors.New("summary is required")
	}
	if err := validateEvidenceReferences("evidence", f.Evidence); err != nil {
		return err
	}
	if strings.TrimSpace(f.RequiredCorrection) == "" {
		return errors.New("required_correction is required")
	}
	return nil
}

func ValidateCorrectionDecision(decision SupervisorDecision, report AuditReport) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	if decision.Action != ActionCorrect {
		return fmt.Errorf("validate correction decision: action must be %q (got %q)", ActionCorrect, decision.Action)
	}
	if err := report.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(decision.TaskID) != strings.TrimSpace(report.TaskID) {
		return fmt.Errorf("validate correction decision: task_id %q does not match audit task_id %q", decision.TaskID, report.TaskID)
	}
	if report.Disposition != AuditDispositionChangesRequired {
		return fmt.Errorf("validate correction decision: audit disposition must be %q", AuditDispositionChangesRequired)
	}
	if decision.VerificationFailure != nil {
		return errors.New("validate correction decision: audit-finding correction must not cite verification_failure")
	}

	known := make(map[string]struct{}, len(report.Findings))
	for _, finding := range report.Findings {
		known[finding.ID] = struct{}{}
	}
	for _, findingID := range decision.FindingIDs {
		if _, exists := known[findingID]; !exists {
			return fmt.Errorf("validate correction decision: finding_id %q does not reference an audit finding", findingID)
		}
	}
	return nil
}

func ValidateVerificationCorrectionDecision(decision SupervisorDecision, target VerificationFailureTarget) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	if decision.Action != ActionCorrect || decision.VerificationFailure == nil || len(decision.FindingIDs) != 0 {
		return errors.New("validate verification correction decision: exact verification-failure authority is required")
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("validate verification correction decision: target: %w", err)
	}
	if !reflect.DeepEqual(*decision.VerificationFailure, target) {
		return errors.New("validate verification correction decision: cited verification failure does not exactly match durable authority")
	}
	return nil
}

func validAction(action Action) bool {
	switch action {
	case ActionPlan, ActionImplement, ActionAudit, ActionCorrect, ActionDocument, ActionSimplify, ActionComplete, ActionBlock:
		return true
	default:
		return false
	}
}

func workerProfileForAction(action Action) (WorkerProfile, bool) {
	switch action {
	case ActionPlan:
		return WorkerProfilePlanner, true
	case ActionImplement:
		return WorkerProfileImplementer, true
	case ActionAudit:
		return WorkerProfileAuditor, true
	case ActionCorrect:
		return WorkerProfileCorrector, true
	case ActionDocument:
		return WorkerProfileDocumentor, true
	case ActionSimplify:
		return WorkerProfileSimplifier, true
	default:
		return "", false
	}
}

func validEvidenceKind(kind EvidenceKind) bool {
	switch kind {
	case EvidenceKindTask, EvidenceKindPlan, EvidenceKindLedger, EvidenceKindReceipt, EvidenceKindVerification, EvidenceKindGit, EvidenceKindAudit, EvidenceKindRepository, EvidenceKindFile:
		return true
	default:
		return false
	}
}

func validAuditDisposition(disposition AuditDisposition) bool {
	switch disposition {
	case AuditDispositionClean, AuditDispositionChangesRequired:
		return true
	default:
		return false
	}
}

func validFindingSignificance(significance FindingSignificance) bool {
	switch significance {
	case FindingSignificanceBlocking, FindingSignificanceNonBlocking:
		return true
	default:
		return false
	}
}

func validateEvidenceReferences(prefix string, references []EvidenceReference) error {
	if len(references) == 0 {
		return fmt.Errorf("%s requires at least one evidence reference", prefix)
	}
	for i, reference := range references {
		if !validEvidenceKind(reference.Kind) {
			return fmt.Errorf("%s[%d]: unknown kind %q", prefix, i, reference.Kind)
		}
		if strings.TrimSpace(reference.Reference) == "" {
			return fmt.Errorf("%s[%d]: reference is required", prefix, i)
		}
		if strings.TrimSpace(reference.Detail) == "" {
			return fmt.Errorf("%s[%d]: detail is required", prefix, i)
		}
	}
	return nil
}

func validateSuccessCriteria(criteria []string) error {
	if len(criteria) == 0 {
		return errors.New("validate supervisor decision: worker action requires at least one success criterion")
	}
	return validateOptionalSuccessCriteria(criteria)
}

func validateOptionalSuccessCriteria(criteria []string) error {
	for i, criterion := range criteria {
		if strings.TrimSpace(criterion) == "" {
			return fmt.Errorf("validate supervisor decision: success_criteria[%d] is empty", i)
		}
	}
	return nil
}

func validateFindingIDs(prefix string, ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for i, id := range ids {
		if !validFindingID(id) {
			return fmt.Errorf("%s[%d]: invalid finding id %q (want lower-case kebab-case beginning with a letter)", prefix, i, id)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%s[%d]: duplicate finding id %q", prefix, i, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validFindingID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) {
		return false
	}
	for i, r := range id {
		switch {
		case i == 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && r == '-' && id[i-1] != '-' && i < len(id)-1:
		default:
			return false
		}
	}
	return true
}
