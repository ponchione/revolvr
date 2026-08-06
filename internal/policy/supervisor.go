// Package policy contains trusted, deterministic host policy. It returns
// routing proposals only; it never mutates lifecycle or database state.
package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"revolvr/internal/model"
	"revolvr/internal/tasklifecycle"
)

const SupervisorPolicyVersion = "revolvr-supervisor-host-policy-v1"

type Action string

const (
	ActionPlan       Action = "plan"
	ActionImplement  Action = "implement"
	ActionCorrect    Action = "correct"
	ActionDocument   Action = "document"
	ActionSimplify   Action = "simplify"
	ActionComplete   Action = "complete"
	ActionBlock      Action = "block"
	ActionNeedsInput Action = "needs_input"
)

var admittedActions = []Action{
	ActionPlan,
	ActionImplement,
	ActionCorrect,
	ActionDocument,
	ActionSimplify,
	ActionComplete,
	ActionBlock,
	ActionNeedsInput,
}

type Identity struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type policyRule struct {
	Action       Action                     `json:"action"`
	Lifecycles   []tasklifecycle.TaskStatus `json:"lifecycles"`
	RouteKind    RouteKind                  `json:"route_kind"`
	WorkerRole   string                     `json:"worker_role"`
	ProposedNext tasklifecycle.TaskStatus   `json:"proposed_next"`
}

var supervisorRules = []policyRule{
	{ActionPlan, []tasklifecycle.TaskStatus{tasklifecycle.TaskAdmitted}, RouteWorkerRequest, "planner", tasklifecycle.TaskPlanning},
	{ActionImplement, []tasklifecycle.TaskStatus{tasklifecycle.TaskReady}, RouteWorkerRequest, "implementer", tasklifecycle.TaskWorking},
	{ActionCorrect, []tasklifecycle.TaskStatus{tasklifecycle.TaskVerifying, tasklifecycle.TaskAuditing}, RouteWorkerRequest, "corrector", tasklifecycle.TaskCorrecting},
	{ActionDocument, []tasklifecycle.TaskStatus{tasklifecycle.TaskAuditing}, RouteWorkerRequest, "documentor", tasklifecycle.TaskDocumenting},
	{ActionSimplify, []tasklifecycle.TaskStatus{tasklifecycle.TaskAuditing}, RouteWorkerRequest, "simplifier", tasklifecycle.TaskSimplifying},
	{ActionComplete, []tasklifecycle.TaskStatus{tasklifecycle.TaskAuditing}, RouteCompletionPreflight, "", tasklifecycle.TaskFinalizing},
}

type policyProjection struct {
	Version         string       `json:"version"`
	Actions         []Action     `json:"actions"`
	Rules           []policyRule `json:"rules"`
	AdvisoryActions []Action     `json:"advisory_actions"`
	ScopeRule       string       `json:"scope_rule"`
	BudgetRule      string       `json:"budget_rule"`
	CompletionRule  string       `json:"completion_rule"`
}

func CurrentIdentity() Identity {
	raw, err := json.Marshal(policyProjection{
		Version:         SupervisorPolicyVersion,
		Actions:         append([]Action(nil), admittedActions...),
		Rules:           cloneRules(supervisorRules),
		AdvisoryActions: []Action{ActionBlock, ActionNeedsInput},
		ScopeRule:       "clean-relative-path-within-allowed-and-outside-excluded-v1",
		BudgetRule:      "charge-supervisor-before-worker-routing-v1",
		CompletionRule:  "proposal-to-completion-preflight-only-v1",
	})
	if err != nil {
		panic(err)
	}
	return Identity{Version: SupervisorPolicyVersion, SHA256: model.SHA256(raw)}
}

func ValidateIdentity(identity Identity) error {
	want := CurrentIdentity()
	if identity != want {
		return fmt.Errorf("host policy identity mismatch: got %q/%q, want %q/%q", identity.Version, identity.SHA256, want.Version, want.SHA256)
	}
	return nil
}

type RouteKind string

const (
	RouteWorkerRequest       RouteKind = "worker_request"
	RouteCompletionPreflight RouteKind = "completion_preflight_proposal"
	RouteBlockAdvisory       RouteKind = "block_advisory"
	RouteNeedsInputAdvisory  RouteKind = "needs_input_advisory"
)

type Budget struct {
	IdentityID              string `json:"identity_id"`
	ModelCallsRemaining     int64  `json:"model_calls_remaining"`
	WorkerAttemptsRemaining int64  `json:"worker_attempts_remaining"`
	TokensRemaining         int64  `json:"tokens_remaining"`
}

type Scope struct {
	AllowedPaths  []string `json:"allowed_paths"`
	ExcludedPaths []string `json:"excluded_paths"`
}

type PlanGate struct {
	ID        string         `json:"id"`
	Completed bool           `json:"completed"`
	Steps     []PlanStepGate `json:"steps"`
}

type PlanStepGate struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type CriterionGate struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type VerificationGate struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	SourceRevision   string `json:"source_revision"`
	Final            bool   `json:"final"`
	EvidenceComplete bool   `json:"evidence_complete"`
}

type AuditGate struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	SourceRevision   string `json:"source_revision"`
	VerificationID   string `json:"verification_id"`
	Independent      bool   `json:"independent"`
	EvidenceComplete bool   `json:"evidence_complete"`
}

type FindingGate struct {
	ID   string `json:"id"`
	Open bool   `json:"open"`
}

type CompletionEvidence struct {
	PlanID             string   `json:"plan_id"`
	CriterionIDs       []string `json:"criterion_ids"`
	VerificationID     string   `json:"verification_id"`
	AuditID            string   `json:"audit_id"`
	ArtifactManifestID string   `json:"artifact_manifest_id"`
}

type CorrectionAuthority struct {
	Kind       string   `json:"kind"`
	FindingIDs []string `json:"finding_ids"`
}

type Proposal struct {
	DecisionID string
	Action     Action
	Scope      []string
	Correction *CorrectionAuthority
	Completion *CompletionEvidence
}

type Input struct {
	TaskID                   string
	Lifecycle                tasklifecycle.TaskStatus
	SourceRevision           string
	SourceSafe               bool
	Budget                   Budget
	SupervisorUsageTokens    int64
	Scope                    Scope
	Plan                     *PlanGate
	Criteria                 []CriterionGate
	Verification             *VerificationGate
	Audit                    *AuditGate
	Findings                 []FindingGate
	WorkspaceReconciled      bool
	ArtifactManifestID       string
	ArtifactManifestComplete bool
	Proposal                 Proposal
}

type Route struct {
	Kind           RouteKind                `json:"kind"`
	TaskID         string                   `json:"task_id"`
	DecisionID     string                   `json:"decision_id"`
	Action         Action                   `json:"action"`
	WorkerRole     string                   `json:"worker_role,omitempty"`
	ProposedStatus tasklifecycle.TaskStatus `json:"proposed_status"`
}

func RouteSupervisor(in Input) (Route, error) {
	if err := validateInput(in); err != nil {
		return Route{}, err
	}
	if !slices.Contains(admittedActions, in.Proposal.Action) {
		return Route{}, fmt.Errorf("unknown supervisor action %q", in.Proposal.Action)
	}
	if in.Budget.ModelCallsRemaining <= 0 || in.Budget.TokensRemaining <= 0 {
		return Route{}, errors.New("supervisor budget was exhausted before this decision")
	}
	if in.SupervisorUsageTokens < 0 || in.SupervisorUsageTokens > in.Budget.TokensRemaining {
		return Route{}, errors.New("supervisor token usage exceeds the pinned budget")
	}

	remainingModelCalls := in.Budget.ModelCallsRemaining - 1
	remainingTokens := in.Budget.TokensRemaining - in.SupervisorUsageTokens
	if isWorkerAction(in.Proposal.Action) && (remainingModelCalls <= 0 || in.Budget.WorkerAttemptsRemaining <= 0 || remainingTokens <= 0) {
		return Route{}, fmt.Errorf("action %q is denied because the relevant worker budget is exhausted", in.Proposal.Action)
	}
	if err := validateScope(in.Scope, in.Proposal.Action, in.Proposal.Scope); err != nil {
		return Route{}, err
	}

	if in.Proposal.Action == ActionBlock || in.Proposal.Action == ActionNeedsInput {
		if !activeLifecycle(in.Lifecycle) {
			return Route{}, fmt.Errorf("lifecycle %q does not admit advisory action %q", in.Lifecycle, in.Proposal.Action)
		}
		kind := RouteBlockAdvisory
		next := tasklifecycle.TaskBlocked
		if in.Proposal.Action == ActionNeedsInput {
			kind = RouteNeedsInputAdvisory
			next = tasklifecycle.TaskNeedsInput
		}
		if err := tasklifecycle.ValidateTaskTransition(in.Lifecycle, next, tasklifecycle.AuthoritySupervisorPolicy); err != nil {
			return Route{}, fmt.Errorf("lifecycle %q rejects action %q: %w", in.Lifecycle, in.Proposal.Action, err)
		}
		return Route{Kind: kind, TaskID: in.TaskID, DecisionID: in.Proposal.DecisionID, Action: in.Proposal.Action, ProposedStatus: next}, nil
	}

	rule, ok := ruleFor(in.Proposal.Action)
	if !ok || !slices.Contains(rule.Lifecycles, in.Lifecycle) {
		return Route{}, fmt.Errorf("lifecycle %q does not admit action %q", in.Lifecycle, in.Proposal.Action)
	}
	if err := tasklifecycle.ValidateTaskTransition(in.Lifecycle, rule.ProposedNext, transitionAuthority(in.Proposal.Action)); err != nil {
		return Route{}, fmt.Errorf("lifecycle %q rejects action %q: %w", in.Lifecycle, in.Proposal.Action, err)
	}
	if err := validateActionGates(in); err != nil {
		return Route{}, err
	}
	return Route{Kind: rule.RouteKind, TaskID: in.TaskID, DecisionID: in.Proposal.DecisionID, Action: in.Proposal.Action, WorkerRole: rule.WorkerRole, ProposedStatus: rule.ProposedNext}, nil
}

func validateInput(in Input) error {
	if strings.TrimSpace(in.TaskID) == "" || strings.TrimSpace(in.Proposal.DecisionID) == "" {
		return errors.New("policy requires normalized task and decision identities")
	}
	if strings.TrimSpace(in.SourceRevision) == "" || strings.TrimSpace(in.Budget.IdentityID) == "" {
		return errors.New("policy requires source and budget identities")
	}
	if in.Budget.ModelCallsRemaining < 0 || in.Budget.WorkerAttemptsRemaining < 0 || in.Budget.TokensRemaining < 0 {
		return errors.New("policy budget counters must be nonnegative")
	}
	if in.Proposal.Action != ActionBlock && in.Proposal.Action != ActionNeedsInput && !in.SourceSafe {
		return fmt.Errorf("action %q requires safe pinned source", in.Proposal.Action)
	}
	return nil
}

func validateActionGates(in Input) error {
	switch in.Proposal.Action {
	case ActionPlan:
		return nil
	case ActionImplement:
		if in.Plan == nil || in.Plan.Completed {
			return errors.New("implement requires a current incomplete plan")
		}
		for _, step := range in.Plan.Steps {
			if step.Status == "pending" || step.Status == "in_progress" {
				return nil
			}
		}
		return errors.New("implement requires a pending or in-progress plan step")
	case ActionCorrect:
		return validateCorrection(in)
	case ActionDocument, ActionSimplify:
		return requireFreshCleanAudit(in)
	case ActionComplete:
		return validateCompletion(in)
	default:
		return fmt.Errorf("unsupported routed action %q", in.Proposal.Action)
	}
}

func validateCorrection(in Input) error {
	if in.Proposal.Correction == nil {
		return errors.New("correct requires exact correction authority")
	}
	switch in.Proposal.Correction.Kind {
	case "verification_failure":
		if in.Lifecycle != tasklifecycle.TaskVerifying || len(in.Proposal.Correction.FindingIDs) != 0 || in.Verification == nil || in.Verification.Status != "failed" || in.Verification.SourceRevision != in.SourceRevision {
			return errors.New("verification correction does not match the current failed verification")
		}
	case "audit_findings":
		if in.Lifecycle != tasklifecycle.TaskAuditing || in.Audit == nil || in.Audit.Status != "changes_required" || in.Audit.SourceRevision != in.SourceRevision || len(in.Proposal.Correction.FindingIDs) == 0 {
			return errors.New("audit correction does not match the current changes-required audit")
		}
		open := make(map[string]bool, len(in.Findings))
		for _, finding := range in.Findings {
			open[finding.ID] = finding.Open
		}
		seen := make(map[string]struct{}, len(in.Proposal.Correction.FindingIDs))
		for _, id := range in.Proposal.Correction.FindingIDs {
			if !open[id] {
				return fmt.Errorf("audit correction finding %q is not currently open", id)
			}
			if _, duplicate := seen[id]; duplicate {
				return fmt.Errorf("audit correction repeats finding %q", id)
			}
			seen[id] = struct{}{}
		}
	default:
		return fmt.Errorf("unknown correction authority %q", in.Proposal.Correction.Kind)
	}
	return nil
}

func validateCompletion(in Input) error {
	if in.Proposal.Completion == nil {
		return errors.New("complete requires a typed completion-preflight proposal")
	}
	if in.Plan == nil || !in.Plan.Completed || in.Proposal.Completion.PlanID != in.Plan.ID {
		return errors.New("complete requires the exact completed plan")
	}
	for _, step := range in.Plan.Steps {
		if step.Status != "completed" && step.Status != "skipped" {
			return fmt.Errorf("complete is blocked by nonterminal plan step %q", step.ID)
		}
	}
	if len(in.Criteria) == 0 {
		return errors.New("complete requires acceptance criteria")
	}
	wantCriteria := make([]string, 0, len(in.Criteria))
	for _, criterion := range in.Criteria {
		if criterion.Status != "passed" && criterion.Status != "waived" && criterion.Status != "not_applicable" {
			return fmt.Errorf("complete is blocked by criterion %q in status %q", criterion.ID, criterion.Status)
		}
		wantCriteria = append(wantCriteria, criterion.ID)
	}
	if !slices.Equal(in.Proposal.Completion.CriterionIDs, wantCriteria) {
		return errors.New("complete criterion identity does not match canonical acceptance evidence")
	}
	if err := requireFreshCleanAudit(in); err != nil {
		return err
	}
	if in.Proposal.Completion.VerificationID != in.Verification.ID || in.Proposal.Completion.AuditID != in.Audit.ID {
		return errors.New("complete verification or audit identity is stale")
	}
	for _, finding := range in.Findings {
		if finding.Open {
			return fmt.Errorf("complete is blocked by open finding %q", finding.ID)
		}
	}
	if !in.WorkspaceReconciled {
		return errors.New("complete requires a reconciled workspace")
	}
	if !in.ArtifactManifestComplete || in.ArtifactManifestID == "" || in.Proposal.Completion.ArtifactManifestID != in.ArtifactManifestID {
		return errors.New("complete requires the exact complete artifact manifest")
	}
	return nil
}

func requireFreshCleanAudit(in Input) error {
	if in.Verification == nil || in.Verification.Status != "passed" || !in.Verification.Final || !in.Verification.EvidenceComplete || in.Verification.SourceRevision != in.SourceRevision {
		return errors.New("a fresh passed final verification with complete evidence is required")
	}
	if in.Audit == nil || in.Audit.Status != "clean" || !in.Audit.Independent || !in.Audit.EvidenceComplete || in.Audit.SourceRevision != in.SourceRevision || in.Audit.VerificationID != in.Verification.ID {
		return errors.New("a fresh independent clean audit linked to verification is required")
	}
	return nil
}

func validateScope(authority Scope, action Action, proposed []string) error {
	sourceAction := action == ActionImplement || action == ActionCorrect || action == ActionDocument || action == ActionSimplify
	if !sourceAction {
		if len(proposed) != 0 {
			return fmt.Errorf("action %q forbids source scope", action)
		}
		return nil
	}
	if len(proposed) == 0 {
		return fmt.Errorf("action %q requires bounded source scope", action)
	}
	seen := make(map[string]struct{}, len(proposed))
	for _, candidate := range proposed {
		if !cleanRelative(candidate) {
			return fmt.Errorf("proposed scope %q is not a clean repository-relative path", candidate)
		}
		if _, duplicate := seen[candidate]; duplicate {
			return fmt.Errorf("proposed scope repeats %q", candidate)
		}
		seen[candidate] = struct{}{}
		if !covered(candidate, authority.AllowedPaths) || covered(candidate, authority.ExcludedPaths) {
			return fmt.Errorf("proposed scope %q broadens the accepted task scope", candidate)
		}
	}
	return nil
}

func cleanRelative(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && path.Clean(value) == value && value != "." && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../")
}

func covered(candidate string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if cleanRelative(prefix) && (candidate == prefix || strings.HasPrefix(candidate, prefix+"/")) {
			return true
		}
	}
	return false
}

func ruleFor(action Action) (policyRule, bool) {
	for _, rule := range supervisorRules {
		if rule.Action == action {
			return rule, true
		}
	}
	return policyRule{}, false
}

func transitionAuthority(action Action) tasklifecycle.Authority {
	if action == ActionImplement {
		return tasklifecycle.AuthoritySupervisorPolicy
	}
	return tasklifecycle.AuthorityPolicy
}

func isWorkerAction(action Action) bool {
	return action == ActionPlan || action == ActionImplement || action == ActionCorrect || action == ActionDocument || action == ActionSimplify
}

func activeLifecycle(status tasklifecycle.TaskStatus) bool {
	switch status {
	case tasklifecycle.TaskDraft, tasklifecycle.TaskCompiled, tasklifecycle.TaskAwaitingApproval, tasklifecycle.TaskPending,
		tasklifecycle.TaskAdmitted, tasklifecycle.TaskPlanning, tasklifecycle.TaskReady, tasklifecycle.TaskWorking,
		tasklifecycle.TaskVerifying, tasklifecycle.TaskAuditing, tasklifecycle.TaskCorrecting,
		tasklifecycle.TaskDocumenting, tasklifecycle.TaskSimplifying, tasklifecycle.TaskFinalizing:
		return true
	default:
		return false
	}
}

func cloneRules(values []policyRule) []policyRule {
	out := make([]policyRule, len(values))
	copy(out, values)
	for i := range out {
		out[i].Lifecycles = append([]tasklifecycle.TaskStatus(nil), out[i].Lifecycles...)
	}
	return out
}
