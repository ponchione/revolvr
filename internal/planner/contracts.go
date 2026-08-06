// Package planner owns the trusted, versioned planning boundary. Models may
// propose a plan revision; only the host validates and accepts one.
package planner

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"revolvr/internal/model"
	"revolvr/internal/policy"
	"revolvr/internal/supervisor"
)

const (
	OutputSchemaVersion = "revolvr-planner-output-v1"
	OutputSchemaName    = "revolvr_planner_output_v1"
	PromptVersion       = "revolvr-planner-prompt-v1"
	ModelPolicyVersion  = "revolvr-planner-model-policy-v1"
	HostPolicyVersion   = "revolvr-planner-host-policy-v1"
	DossierVersion      = "revolvr-planner-dossier-v1"
	CandidateVersion    = "revolvr-plan-candidate-v1"
	MaximumSteps        = 64
)

var (
	ErrRejected = errors.New("planner output rejected")
	stableID    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	placeholder = regexp.MustCompile(`(?i)(\bTODO\b|\bTBD\b|\bFIXME\b|\bPLACEHOLDER\b|\?\?|\{\{[^}]*\}\}|<[^>]+>)`)
)

type ModelPolicySettings struct {
	Model              string
	ReasoningEffort    string
	MaxOutputTokens    int
	Timeout            time.Duration
	MaxStreamBytes     int64
	MaxDiagnosticBytes int
}

type ModelPolicy struct {
	Version            string            `json:"version"`
	SHA256             string            `json:"sha256"`
	Model              string            `json:"model"`
	ReasoningEffort    string            `json:"reasoning_effort"`
	MaxOutputTokens    int               `json:"max_output_tokens"`
	Timeout            time.Duration     `json:"timeout_ns"`
	MaxStreamBytes     int64             `json:"max_stream_bytes"`
	MaxDiagnosticBytes int               `json:"max_diagnostic_bytes"`
	ToolMode           string            `json:"tool_mode"`
	FreshSession       bool              `json:"fresh_session"`
	Retry              model.RetryPolicy `json:"retry"`
}

func PinModelPolicy(settings ModelPolicySettings) (ModelPolicy, error) {
	value := ModelPolicy{
		Version: ModelPolicyVersion, Model: settings.Model, ReasoningEffort: settings.ReasoningEffort,
		MaxOutputTokens: settings.MaxOutputTokens, Timeout: settings.Timeout,
		MaxStreamBytes: settings.MaxStreamBytes, MaxDiagnosticBytes: settings.MaxDiagnosticBytes,
		ToolMode: "none", FreshSession: true, Retry: model.RetryPolicy{MaxAttempts: 1},
	}
	if err := validateModelPolicy(value, false); err != nil {
		return ModelPolicy{}, err
	}
	raw, _ := json.Marshal(modelPolicyMaterial(value))
	value.SHA256 = model.SHA256(raw)
	return value, nil
}

func validateModelPolicy(value ModelPolicy, requireHash bool) error {
	if value.Version != ModelPolicyVersion || !token(value.Model) || !token(value.ReasoningEffort) {
		return errors.New("planner model policy version, model, or reasoning effort is invalid")
	}
	if value.MaxOutputTokens <= 0 || value.Timeout <= 0 || value.MaxStreamBytes < 0 || value.MaxDiagnosticBytes < 0 {
		return errors.New("planner model policy limits are invalid")
	}
	if value.ToolMode != "none" || !value.FreshSession || value.Retry.MaxAttempts != 1 {
		return errors.New("planner invocation must be one fresh, tool-free attempt")
	}
	if requireHash {
		raw, _ := json.Marshal(modelPolicyMaterial(value))
		if value.SHA256 != model.SHA256(raw) {
			return errors.New("planner model policy hash is stale")
		}
	}
	return nil
}

func modelPolicyMaterial(value ModelPolicy) any {
	value.SHA256 = ""
	return value
}

type HostPolicy struct {
	Version        string `json:"version"`
	SHA256         string `json:"sha256"`
	MaximumSteps   int    `json:"maximum_steps"`
	PathRule       string `json:"path_rule"`
	CriterionRule  string `json:"criterion_rule"`
	RevisionRule   string `json:"revision_rule"`
	AcceptanceRule string `json:"acceptance_rule"`
}

func CurrentHostPolicy() HostPolicy {
	value := HostPolicy{
		Version: HostPolicyVersion, MaximumSteps: MaximumSteps,
		PathRule:       "task-expected-path-prefix-only-v1",
		CriterionRule:  "canonical-order-exactly-once-v1",
		RevisionRule:   "stable-prefix-monotonic-lineage-v1",
		AcceptanceRule: "trusted-host-operation-only-v1",
	}
	raw, _ := json.Marshal(struct {
		Version, PathRule, CriterionRule, RevisionRule, AcceptanceRule string
		MaximumSteps                                                   int
	}{value.Version, value.PathRule, value.CriterionRule, value.RevisionRule, value.AcceptanceRule, value.MaximumSteps})
	value.SHA256 = model.SHA256(raw)
	return value
}

type Criterion struct {
	ID                    string `json:"id"`
	Requirement           string `json:"requirement"`
	VerificationMethod    string `json:"verification_method"`
	VerificationReference string `json:"verification_reference"`
}

type TaskContract struct {
	TaskID        string      `json:"task_id"`
	TaskVersionID string      `json:"task_version_id"`
	VersionNumber int64       `json:"version_number"`
	Title         string      `json:"title"`
	Goal          string      `json:"goal"`
	Scope         []string    `json:"scope"`
	ExcludedScope []string    `json:"excluded_scope"`
	Dependencies  []string    `json:"dependencies"`
	ExpectedPaths []string    `json:"expected_paths"`
	Criteria      []Criterion `json:"criteria"`
}

type RunAuthority struct {
	RunID           string `json:"run_id"`
	ProjectID       string `json:"project_id"`
	ProjectSourceID string `json:"project_source_id"`
}

type SourceAuthority struct {
	Revision string `json:"revision"`
	Commit   string `json:"commit"`
	Tree     string `json:"tree"`
}

type EvidenceItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	SHA256  string `json:"sha256,omitempty"`
}

type ProjectPath struct {
	Path      string `json:"path"`
	Component string `json:"component"`
	Kind      string `json:"kind"`
}

type PriorPlan struct {
	PlanID        string `json:"plan_id"`
	PlanVersionID string `json:"plan_version_id"`
	Revision      int    `json:"revision"`
	Steps         []Step `json:"steps"`
}

type CanonicalState struct {
	Task                     TaskContract    `json:"task"`
	Run                      RunAuthority    `json:"run"`
	Source                   SourceAuthority `json:"source"`
	SupervisorDecisionID     string          `json:"supervisor_decision_id"`
	SupervisorDecisionSHA256 string          `json:"supervisor_decision_sha256"`
	ArchitectureConstraints  []EvidenceItem  `json:"architecture_constraints"`
	ProjectMap               []ProjectPath   `json:"project_map"`
	ModuleRelationships      []EvidenceItem  `json:"module_relationships"`
	Conventions              []EvidenceItem  `json:"conventions"`
	PriorDecisions           []EvidenceItem  `json:"prior_decisions"`
	PriorPlan                *PriorPlan      `json:"prior_plan"`
}

type TestStrategy struct {
	CriterionID string `json:"criterion_id"`
	Method      string `json:"method"`
	Reference   string `json:"reference"`
}

type StepLineage struct {
	PriorPlanVersionID string `json:"prior_plan_version_id"`
	PriorStepID        string `json:"prior_step_id"`
	PriorStatus        string `json:"prior_status"`
	TransitionEvidence string `json:"transition_evidence"`
}

type Step struct {
	ID               string         `json:"id"`
	Ordinal          int            `json:"ordinal"`
	Status           string         `json:"status"`
	Description      string         `json:"description"`
	CriterionIDs     []string       `json:"criterion_ids"`
	DependsOnStepIDs []string       `json:"depends_on_step_ids"`
	ExpectedPaths    []string       `json:"expected_paths"`
	Components       []string       `json:"components"`
	TestStrategy     []TestStrategy `json:"test_strategy"`
	Risks            []string       `json:"risks"`
	Assumptions      []string       `json:"assumptions"`
	EvidenceRefs     []string       `json:"evidence_refs"`
	Lineage          *StepLineage   `json:"lineage"`
}

type RevisionIdentity struct {
	PlanID                   string  `json:"plan_id"`
	PlanVersionID            string  `json:"plan_version_id"`
	RevisionNumber           int     `json:"revision_number"`
	SupersedesPlanVersionID  *string `json:"supersedes_plan_version_id"`
	TaskID                   string  `json:"task_id"`
	TaskVersionID            string  `json:"task_version_id"`
	TaskVersionNumber        int64   `json:"task_version_number"`
	RunID                    string  `json:"run_id"`
	ProjectSourceID          string  `json:"project_source_id"`
	SourceRevision           string  `json:"source_revision"`
	SupervisorDecisionID     string  `json:"supervisor_decision_id"`
	SupervisorDecisionSHA256 string  `json:"supervisor_decision_sha256"`
	DossierVersion           string  `json:"dossier_version"`
	DossierSHA256            string  `json:"dossier_sha256"`
	PromptVersion            string  `json:"prompt_version"`
	PromptSHA256             string  `json:"prompt_sha256"`
	ResponseSchemaVersion    string  `json:"response_schema_version"`
	ResponseSchemaSHA256     string  `json:"response_schema_sha256"`
	ModelPolicyVersion       string  `json:"model_policy_version"`
	ModelPolicySHA256        string  `json:"model_policy_sha256"`
	HostPolicyVersion        string  `json:"host_policy_version"`
	HostPolicySHA256         string  `json:"host_policy_sha256"`
	ContentSHA256            *string `json:"content_sha256"`
}

type Output struct {
	RevolvrIdentity   model.OutputIdentity `json:"revolvr_identity"`
	SchemaVersion     string               `json:"schema_version"`
	Revision          RevisionIdentity     `json:"revision_identity"`
	ChangeExplanation string               `json:"change_explanation"`
	TaskDependencyIDs []string             `json:"task_dependency_ids"`
	Steps             []Step               `json:"steps"`
	Risks             []string             `json:"risks"`
	Assumptions       []string             `json:"assumptions"`
	EvidenceRefs      []string             `json:"evidence_refs"`
}

func ParseOutput(raw []byte) (Output, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Output{}, errors.New("planner structured output is missing")
	}
	if err := rejectDuplicateJSONFields(raw); err != nil {
		return Output{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var output Output
	if err := decoder.Decode(&output); err != nil {
		return Output{}, fmt.Errorf("decode exactly one planner output: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Output{}, errors.New("content follows planner output")
	}
	want := contentSHA256(output)
	if output.Revision.ContentSHA256 != nil && *output.Revision.ContentSHA256 != want {
		return Output{}, errors.New("planner content SHA-256 is stale")
	}
	output.Revision.ContentSHA256 = &want
	return output, nil
}

func contentSHA256(output Output) string {
	output.Revision.ContentSHA256 = nil
	raw, _ := json.Marshal(output)
	return model.SHA256(raw)
}

func validateOutput(output Output, state CanonicalState, expected RevisionIdentity, outputIdentity model.OutputIdentity, evidence map[string]struct{}) error {
	if output.SchemaVersion != OutputSchemaVersion || output.RevolvrIdentity != outputIdentity {
		return errors.New("planner schema or task-013 output identity is stale")
	}
	got := output.Revision
	content := got.ContentSHA256
	got.ContentSHA256 = nil
	gotSupersedes := got.SupersedesPlanVersionID
	got.SupersedesPlanVersionID = nil
	wantSupersedes := expected.SupersedesPlanVersionID
	expected.ContentSHA256 = nil
	expected.SupersedesPlanVersionID = nil
	supersedesEqual := gotSupersedes == nil && wantSupersedes == nil || gotSupersedes != nil && wantSupersedes != nil && *gotSupersedes == *wantSupersedes
	if got != expected || !supersedesEqual || content == nil || *content != contentSHA256(output) {
		return fmt.Errorf("planner task, run, source, decision, dossier, prompt, policy, plan, or revision identity is stale: got %+v, want %+v", got, expected)
	}
	if !slices.Equal(output.TaskDependencyIDs, state.Task.Dependencies) {
		return errors.New("planner task dependencies are missing, invented, duplicated, or reordered")
	}
	if len(output.Steps) == 0 || len(output.Steps) > MaximumSteps {
		return fmt.Errorf("planner requires between 1 and %d bounded steps", MaximumSteps)
	}
	if err := validateText("change explanation", output.ChangeExplanation); err != nil {
		return err
	}
	if err := validateStringList("plan risks", output.Risks, false, false); err != nil {
		return err
	}
	if err := validateStringList("plan assumptions", output.Assumptions, false, false); err != nil {
		return err
	}
	if err := validateEvidenceRefs("plan", output.EvidenceRefs, evidence); err != nil {
		return err
	}

	criteria := make(map[string]Criterion, len(state.Task.Criteria))
	wantOrder := make([]string, len(state.Task.Criteria))
	for i, criterion := range state.Task.Criteria {
		criteria[criterion.ID] = criterion
		wantOrder[i] = criterion.ID
	}
	seenSteps := map[string]int{}
	mapped := make([]string, 0, len(wantOrder))
	for i, step := range output.Steps {
		if step.Ordinal != i+1 || !stableID.MatchString(step.ID) {
			return fmt.Errorf("step %d has a missing, unstable, or reordered identity", i+1)
		}
		if _, duplicate := seenSteps[step.ID]; duplicate {
			return fmt.Errorf("duplicate step %q", step.ID)
		}
		seenSteps[step.ID] = i
		if err := validateStep(step, state, criteria, seenSteps, evidence); err != nil {
			return fmt.Errorf("step %q: %w", step.ID, err)
		}
		mapped = append(mapped, step.CriterionIDs...)
	}
	if !slices.Equal(mapped, wantOrder) {
		return fmt.Errorf("criterion mapping is missing, duplicated, invented, or reordered: got %v, want %v", mapped, wantOrder)
	}
	return validateRevision(output.Steps, state.PriorPlan, evidence)
}

func validateStep(step Step, state CanonicalState, criteria map[string]Criterion, prior map[string]int, evidence map[string]struct{}) error {
	if !slices.Contains([]string{"pending", "in_progress", "completed", "skipped"}, step.Status) {
		return fmt.Errorf("unsupported status %q", step.Status)
	}
	if err := validateText("description", step.Description); err != nil {
		return err
	}
	if len(step.CriterionIDs) == 0 {
		return errors.New("criterion mapping is empty")
	}
	if err := validateStringList("criterion ids", step.CriterionIDs, true, true); err != nil {
		return err
	}
	for _, id := range step.CriterionIDs {
		if _, ok := criteria[id]; !ok {
			return fmt.Errorf("invented criterion %q", id)
		}
	}
	if err := validateStringList("step dependencies", step.DependsOnStepIDs, false, true); err != nil {
		return err
	}
	for _, id := range step.DependsOnStepIDs {
		pos, ok := prior[id]
		if !ok || pos >= step.Ordinal-1 {
			return fmt.Errorf("dependency %q is invented or not an earlier step", id)
		}
	}
	if err := validateStringList("expected paths", step.ExpectedPaths, true, false); err != nil {
		return err
	}
	for _, candidate := range step.ExpectedPaths {
		if !cleanRelative(candidate) || !covered(candidate, state.Task.ExpectedPaths) {
			return fmt.Errorf("expected path %q expands task scope", candidate)
		}
	}
	if err := validateStringList("components", step.Components, true, false); err != nil {
		return err
	}
	if err := validateStringList("risks", step.Risks, false, false); err != nil {
		return err
	}
	if err := validateStringList("assumptions", step.Assumptions, false, false); err != nil {
		return err
	}
	if err := validateEvidenceRefs("step", step.EvidenceRefs, evidence); err != nil {
		return err
	}
	if len(step.TestStrategy) != len(step.CriterionIDs) {
		return errors.New("test strategy must map every step criterion exactly once")
	}
	for i, test := range step.TestStrategy {
		criterion := criteria[step.CriterionIDs[i]]
		if test.CriterionID != criterion.ID || test.Method != criterion.VerificationMethod || test.Reference != criterion.VerificationReference {
			return fmt.Errorf("unsupported verification for criterion %q", test.CriterionID)
		}
	}
	return nil
}

func validateRevision(steps []Step, prior *PriorPlan, evidence map[string]struct{}) error {
	if prior == nil {
		for _, step := range steps {
			if step.Status != "pending" || step.Lineage != nil {
				return fmt.Errorf("initial step %q must be pending without lineage", step.ID)
			}
		}
		return nil
	}
	if len(steps) < len(prior.Steps) {
		return errors.New("revision removed existing steps")
	}
	for i, old := range prior.Steps {
		next := steps[i]
		if next.ID != old.ID || next.Description != old.Description {
			return fmt.Errorf("revision reordered or repurposed step %q", old.ID)
		}
		if !slices.Equal(next.CriterionIDs, old.CriterionIDs) || !slices.Equal(next.DependsOnStepIDs, old.DependsOnStepIDs) ||
			!slices.Equal(next.ExpectedPaths, old.ExpectedPaths) || !slices.Equal(next.Components, old.Components) ||
			!slices.Equal(next.TestStrategy, old.TestStrategy) || !slices.Equal(next.Risks, old.Risks) ||
			!slices.Equal(next.Assumptions, old.Assumptions) || !slices.Equal(next.EvidenceRefs, old.EvidenceRefs) {
			return fmt.Errorf("revision silently changed existing step %q content", old.ID)
		}
		if next.Lineage == nil || next.Lineage.PriorPlanVersionID != prior.PlanVersionID || next.Lineage.PriorStepID != old.ID || next.Lineage.PriorStatus != old.Status {
			return fmt.Errorf("step %q lacks exact revision lineage", old.ID)
		}
		if !monotonic(old.Status, next.Status) {
			return fmt.Errorf("step %q regressed from %s to %s", old.ID, old.Status, next.Status)
		}
		if old.Status != next.Status && !token(next.Lineage.TransitionEvidence) {
			return fmt.Errorf("step %q transition lacks evidence", old.ID)
		}
		if old.Status != next.Status {
			if _, ok := evidence[next.Lineage.TransitionEvidence]; !ok || !slices.Contains(next.EvidenceRefs, next.Lineage.TransitionEvidence) {
				return fmt.Errorf("step %q transition evidence is outside its dossier-backed evidence", old.ID)
			}
		}
		if old.Status == next.Status && next.Lineage.TransitionEvidence != "" {
			return fmt.Errorf("unchanged step %q invents transition evidence", old.ID)
		}
	}
	for _, step := range steps[len(prior.Steps):] {
		if step.Status != "pending" || step.Lineage != nil {
			return fmt.Errorf("new step %q must be appended pending without lineage", step.ID)
		}
	}
	return nil
}

func monotonic(from, to string) bool {
	if from == to {
		return true
	}
	if from == "pending" {
		return to == "in_progress" || to == "completed" || to == "skipped"
	}
	if from == "in_progress" {
		return to == "completed" || to == "skipped"
	}
	return false
}

func validateText(label, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > 16384 || placeholder.MatchString(value) {
		return fmt.Errorf("%s is blank, oversized, unresolved, or a placeholder", label)
	}
	return nil
}

func validateStringList(label string, values []string, required, identities bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("%s is empty", label)
	}
	seen := map[string]struct{}{}
	for _, value := range values {
		if value != strings.TrimSpace(value) || value == "" || len(value) > 4096 || placeholder.MatchString(value) || identities && !stableID.MatchString(value) {
			return fmt.Errorf("%s contains malformed value %q", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeats %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateEvidenceRefs(label string, values []string, allowed map[string]struct{}) error {
	if err := validateStringList(label+" evidence", values, true, false); err != nil {
		return err
	}
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("%s cites evidence %q outside the dossier", label, value)
		}
	}
	return nil
}

func cleanRelative(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}
func covered(candidate string, roots []string) bool {
	for _, root := range roots {
		if cleanRelative(root) && (candidate == root || strings.HasPrefix(candidate, root+"/")) {
			return true
		}
	}
	return false
}
func token(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, " \t\r\n")
}
func validSHA(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}
func validGitOID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && (len(raw) == 20 || len(raw) == 32) && value == strings.ToLower(value)
}

func rejectDuplicateJSONFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tokenValue, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := tokenValue.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyValue, err := decoder.Token()
				if err != nil {
					return err
				}
				key := keyValue.(string)
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON field %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	return walk()
}

func sortedEvidence(values []EvidenceItem) []EvidenceItem {
	out := append([]EvidenceItem(nil), values...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func validateAdmission(record supervisor.DecisionRecord, taskID, taskVersionID, runID, sourceRevision string) error {
	if record.Disposition != supervisor.DecisionAccepted || record.Decision == nil || record.Route == nil {
		return errors.New("planner requires an accepted supervisor decision and host route")
	}
	if record.TaskID != taskID || record.TaskVersionID != taskVersionID || record.RunID != runID || record.SourceRevision != sourceRevision {
		return errors.New("supervisor decision task, version, run, or source is stale")
	}
	if record.Decision.Action != policy.ActionPlan || record.Route.Kind != policy.RouteWorkerRequest || record.Route.Action != policy.ActionPlan || record.Route.WorkerRole != "planner" || record.Route.DecisionID != record.DecisionID || record.Route.TaskID != taskID {
		return errors.New("supervisor decision is not an admitted planner route")
	}
	if err := policy.ValidateIdentity(record.HostPolicy); err != nil {
		return fmt.Errorf("supervisor host policy is untrusted: %w", err)
	}
	if record.Decision.Identity.ContentSHA256 == nil || !validSHA(*record.Decision.Identity.ContentSHA256) {
		return errors.New("supervisor decision content identity is missing")
	}
	return nil
}
