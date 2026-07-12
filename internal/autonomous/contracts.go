package autonomous

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Action string

const (
	ActionPlan       Action = "plan"
	ActionImplement  Action = "implement"
	ActionAudit      Action = "audit"
	ActionCorrect    Action = "correct"
	ActionDocument   Action = "document"
	ActionSimplify   Action = "simplify"
	ActionComplete   Action = "complete"
	ActionBlock      Action = "block"
	ActionNeedsInput Action = "needs_input"
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
	NeedsInput          *NeedsInputQuestion        `json:"needs_input,omitempty"`
	ChildTasks          *ChildTaskProposalSet      `json:"child_tasks,omitempty"`
}

const MaxChildTaskProposals = 4

type ChildParentBehavior string

const (
	ChildDependsOnParent ChildParentBehavior = "depends_on_parent"
	ChildIndependent     ChildParentBehavior = "independent"
)

type ChildTaskProposalSet struct {
	ParentTaskID string              `json:"parent_task_id"`
	ProposalID   string              `json:"proposal_id"`
	Children     []ChildTaskProposal `json:"children"`
}

type ChildTaskProposal struct {
	Key             string              `json:"key"`
	Title           string              `json:"title"`
	Scope           string              `json:"scope"`
	SuccessCriteria []string            `json:"success_criteria"`
	DependsOn       []string            `json:"depends_on,omitempty"`
	Tags            []string            `json:"tags,omitempty"`
	Conflicts       []string            `json:"conflicts,omitempty"`
	ParentBehavior  ChildParentBehavior `json:"parent_behavior"`
	Evidence        []EvidenceReference `json:"evidence"`
}

type InputSourceEffect string

const InputSourceEffectReadOnly InputSourceEffect = "read_only"

type NeedsInputOption struct {
	ID      string `json:"id"`
	Meaning string `json:"meaning"`
}

type NeedsInputRecommendation struct {
	OptionID  string `json:"option_id"`
	Rationale string `json:"rationale"`
}

// IndependentWorkDeclaration is deliberately narrower than a route. AW-17
// permits only explicitly option-independent read-only work to be projected;
// it never starts that work while recording or answering a question.
type IndependentWorkDeclaration struct {
	ID                     string              `json:"id"`
	Action                 Action              `json:"action"`
	WorkerProfile          WorkerProfile       `json:"worker_profile"`
	Description            string              `json:"description"`
	SourceEffect           InputSourceEffect   `json:"source_effect"`
	IndependentOfOptionIDs []string            `json:"independent_of_option_ids"`
	Inputs                 []EvidenceReference `json:"inputs"`
}

type NeedsInputQuestion struct {
	TaskID          string                       `json:"task_id"`
	QuestionID      string                       `json:"question_id"`
	Revision        int64                        `json:"revision"`
	ContentSHA256   string                       `json:"content_sha256"`
	Question        string                       `json:"question"`
	BlockingReason  string                       `json:"blocking_reason"`
	Options         []NeedsInputOption           `json:"options"`
	Recommendation  NeedsInputRecommendation     `json:"recommendation"`
	Evidence        []EvidenceReference          `json:"evidence"`
	IndependentWork []IndependentWorkDeclaration `json:"independent_work,omitempty"`
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
		if d.NeedsInput != nil {
			return fmt.Errorf("validate supervisor decision: needs_input is only valid for action %q", ActionNeedsInput)
		}
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
	if d.Action == ActionNeedsInput {
		if d.NeedsInput == nil {
			return errors.New("validate supervisor decision: needs_input action requires structured needs_input outcome")
		}
		if len(d.SuccessCriteria) != 0 {
			return errors.New("validate supervisor decision: needs_input action must not include success_criteria")
		}
		if err := d.NeedsInput.Validate(); err != nil {
			return fmt.Errorf("validate supervisor decision: needs_input: %w", err)
		}
		if d.NeedsInput.TaskID != d.TaskID {
			return errors.New("validate supervisor decision: needs_input task identity mismatch")
		}
	} else if d.NeedsInput != nil {
		return fmt.Errorf("validate supervisor decision: needs_input is only valid for action %q", ActionNeedsInput)
	}
	if d.ChildTasks != nil {
		if d.Action != ActionBlock && d.Action != ActionNeedsInput {
			return errors.New("validate supervisor decision: child_tasks are allowed only with block or needs_input")
		}
		if err := d.ChildTasks.validate(d); err != nil {
			return fmt.Errorf("validate supervisor decision: child_tasks: %w", err)
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

func (p ChildTaskProposalSet) validate(decision SupervisorDecision) error {
	if p.ParentTaskID != decision.TaskID || !validFindingID(p.ProposalID) {
		return errors.New("parent_task_id must match the decision and proposal_id must be stable lower-case kebab-case")
	}
	if len(p.Children) == 0 || len(p.Children) > MaxChildTaskProposals {
		return fmt.Errorf("children count must be between 1 and %d", MaxChildTaskProposals)
	}
	if raw, err := json.Marshal(p); err != nil || len(raw) > 32768 {
		return errors.New("proposal set exceeds the 32768-byte canonical bound")
	}
	keys, scopes := map[string]struct{}{}, map[string]struct{}{}
	for i, child := range p.Children {
		if !validFindingID(child.Key) || strings.TrimSpace(child.Title) == "" || strings.TrimSpace(child.Scope) == "" || len(child.Title) > 160 || len(child.Scope) > 8192 {
			return fmt.Errorf("children[%d] requires a stable key and bounded non-empty title/scope", i)
		}
		if _, ok := keys[child.Key]; ok {
			return fmt.Errorf("children[%d] duplicates key %q", i, child.Key)
		}
		keys[child.Key] = struct{}{}
		scopeID := strings.ToLower(strings.Join(strings.Fields(child.Scope), " "))
		if _, ok := scopes[scopeID]; ok {
			return fmt.Errorf("children[%d] duplicates equivalent scope", i)
		}
		scopes[scopeID] = struct{}{}
		if err := validateSuccessCriteria(child.SuccessCriteria); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
		if len(child.SuccessCriteria) > 16 || len(child.DependsOn) > 32 || len(child.Tags) > 32 || len(child.Conflicts) > 32 || len(child.Evidence) > 16 {
			return fmt.Errorf("children[%d] exceeds a bounded list limit", i)
		}
		for _, criterion := range child.SuccessCriteria {
			if len(criterion) > 512 {
				return fmt.Errorf("children[%d] success criterion exceeds 512 bytes", i)
			}
		}
		if err := validateEvidenceReferences(fmt.Sprintf("children[%d] evidence", i), child.Evidence); err != nil {
			return err
		}
		for _, evidence := range child.Evidence {
			if !containsEvidence(decision.Inputs, evidence) {
				return fmt.Errorf("children[%d] cites evidence outside the accepted supervisor inputs", i)
			}
		}
		if err := validateStableUniqueStrings("depends_on", child.DependsOn); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
		if err := validateStableUniqueStrings("tags", child.Tags); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
		if err := validateStableUniqueStrings("conflicts", child.Conflicts); err != nil {
			return fmt.Errorf("children[%d]: %w", i, err)
		}
		hasParent := false
		for _, id := range child.DependsOn {
			hasParent = hasParent || id == decision.TaskID
		}
		switch child.ParentBehavior {
		case ChildDependsOnParent:
			if !hasParent {
				return fmt.Errorf("children[%d] depends_on_parent must depend on the parent", i)
			}
		case ChildIndependent:
			if hasParent {
				return fmt.Errorf("children[%d] independent work must not depend on the parent", i)
			}
			if decision.Action != ActionBlock && decision.Action != ActionNeedsInput {
				return fmt.Errorf("children[%d] independent behavior is invalid for this route", i)
			}
		default:
			return fmt.Errorf("children[%d] has invalid parent_behavior %q", i, child.ParentBehavior)
		}
		lower := strings.ToLower(child.Scope)
		for _, forbidden := range []string{"chmod ", "sudo ", "network access", "environment variable", "sandbox bypass", "roadmap"} {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("children[%d] contains forbidden authority-bearing scope", i)
			}
		}
	}
	return nil
}

func containsEvidence(values []EvidenceReference, want EvidenceReference) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, want) {
			return true
		}
	}
	return false
}

func validateStableUniqueStrings(label string, values []string) error {
	seen := map[string]struct{}{}
	for i, value := range values {
		if !validTaskIdentity(value) {
			return fmt.Errorf("%s[%d] is malformed", label, i)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s contains duplicate %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validTaskIdentity(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func (q NeedsInputQuestion) Validate() error {
	if strings.TrimSpace(q.TaskID) == "" || q.TaskID != strings.TrimSpace(q.TaskID) {
		return errors.New("task_id is required and normalized")
	}
	if !validFindingID(q.QuestionID) {
		return fmt.Errorf("invalid question_id %q", q.QuestionID)
	}
	if q.Revision < 1 {
		return errors.New("revision must be positive")
	}
	if strings.TrimSpace(q.Question) == "" || strings.TrimSpace(q.BlockingReason) == "" {
		return errors.New("question and blocking_reason are required")
	}
	if len(q.Options) < 2 {
		return errors.New("at least two mutually exclusive options are required")
	}
	optionIDs := make(map[string]struct{}, len(q.Options))
	meanings := make(map[string]struct{}, len(q.Options))
	for i, option := range q.Options {
		if !validFindingID(option.ID) {
			return fmt.Errorf("options[%d]: invalid option id %q", i, option.ID)
		}
		if _, ok := optionIDs[option.ID]; ok {
			return fmt.Errorf("options[%d]: duplicate option id %q", i, option.ID)
		}
		optionIDs[option.ID] = struct{}{}
		meaning := strings.ToLower(strings.Join(strings.Fields(option.Meaning), " "))
		if meaning == "" {
			return fmt.Errorf("options[%d]: meaning is required", i)
		}
		if _, ok := meanings[meaning]; ok {
			return fmt.Errorf("options[%d]: materially ambiguous duplicate meaning", i)
		}
		meanings[meaning] = struct{}{}
	}
	if _, ok := optionIDs[q.Recommendation.OptionID]; !ok {
		return errors.New("recommendation must reference exactly one offered option_id")
	}
	if strings.TrimSpace(q.Recommendation.Rationale) == "" {
		return errors.New("recommendation rationale is required")
	}
	if err := validateEvidenceReferences("needs-input evidence", q.Evidence); err != nil {
		return err
	}
	workIDs := make(map[string]struct{}, len(q.IndependentWork))
	for i, work := range q.IndependentWork {
		if err := work.validate(q.Options); err != nil {
			return fmt.Errorf("independent_work[%d]: %w", i, err)
		}
		if _, ok := workIDs[work.ID]; ok {
			return fmt.Errorf("independent_work[%d]: duplicate id %q", i, work.ID)
		}
		workIDs[work.ID] = struct{}{}
	}
	want, err := QuestionContentSHA256(q)
	if err != nil {
		return err
	}
	if q.ContentSHA256 != want {
		return fmt.Errorf("content_sha256 %q does not match deterministic question identity %q", q.ContentSHA256, want)
	}
	return nil
}

func (w IndependentWorkDeclaration) validate(options []NeedsInputOption) error {
	if !validFindingID(w.ID) || strings.TrimSpace(w.Description) == "" {
		return errors.New("stable id and description are required")
	}
	expected, ok := workerProfileForAction(w.Action)
	if !ok || (w.Action != ActionPlan && w.Action != ActionAudit) || w.WorkerProfile != expected {
		return errors.New("only compatible plan/planner or audit/auditor work may be declared independent")
	}
	if w.SourceEffect != InputSourceEffectReadOnly {
		return errors.New("independent work must have read_only source_effect")
	}
	if len(w.IndependentOfOptionIDs) != len(options) {
		return errors.New("independent_of_option_ids must name every offered option in canonical order")
	}
	for i := range options {
		if w.IndependentOfOptionIDs[i] != options[i].ID {
			return errors.New("independent_of_option_ids must name every offered option in canonical order")
		}
	}
	return validateEvidenceReferences("independent work inputs", w.Inputs)
}

// QuestionContentSHA256 binds all control-relevant question material while
// excluding only the hash field itself.
func QuestionContentSHA256(q NeedsInputQuestion) (string, error) {
	projection := struct {
		TaskID          string                       `json:"task_id"`
		QuestionID      string                       `json:"question_id"`
		Revision        int64                        `json:"revision"`
		Question        string                       `json:"question"`
		BlockingReason  string                       `json:"blocking_reason"`
		Options         []NeedsInputOption           `json:"options"`
		Recommendation  NeedsInputRecommendation     `json:"recommendation"`
		Evidence        []EvidenceReference          `json:"evidence"`
		IndependentWork []IndependentWorkDeclaration `json:"independent_work,omitempty"`
	}{q.TaskID, q.QuestionID, q.Revision, q.Question, q.BlockingReason, q.Options, q.Recommendation, q.Evidence, q.IndependentWork}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("canonicalize needs-input question: %w", err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
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
	case ActionPlan, ActionImplement, ActionAudit, ActionCorrect, ActionDocument, ActionSimplify, ActionComplete, ActionBlock, ActionNeedsInput:
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
