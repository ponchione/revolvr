package planner

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"revolvr/internal/model"
)

const MaximumDossierBytes = 256 << 10

type Omission struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

type Dossier struct {
	SchemaVersion            string          `json:"schema_version"`
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
	AcceptanceRequirements   []Criterion     `json:"acceptance_requirements"`
	BaselineVerification     []Criterion     `json:"baseline_verification"`
	Omissions                []Omission      `json:"omissions"`
}

type DossierArtifact struct {
	Version  string          `json:"version"`
	SHA256   string          `json:"sha256"`
	ByteSize int             `json:"byte_size"`
	Content  json.RawMessage `json:"content"`
}

func BuildDossier(state CanonicalState) (DossierArtifact, error) {
	if err := validateCanonicalState(state); err != nil {
		return DossierArtifact{}, fmt.Errorf("build planner dossier: %w", err)
	}
	state = cloneAndSortState(state)
	omissions := []Omission{
		{Section: "semantic_retrieval", Reason: "not available before architecture task 021"},
		{Section: "conversation_history", Reason: "fresh planner invocation uses durable canonical state only"},
		{Section: "broad_raw_source", Reason: "bounded project map replaces unbounded source content"},
	}
	if len(state.ModuleRelationships) == 0 {
		omissions = append(omissions, Omission{"module_relationships", "no canonical relationships were supplied"})
	}
	if len(state.Conventions) == 0 {
		omissions = append(omissions, Omission{"conventions", "no additional canonical conventions were supplied"})
	}
	if len(state.PriorDecisions) == 0 {
		omissions = append(omissions, Omission{"prior_decisions", "no applicable prior decisions were supplied"})
	}
	if state.PriorPlan == nil {
		omissions = append(omissions, Omission{"prior_plan", "this is the initial plan revision"})
	}
	dossier := Dossier{
		SchemaVersion: DossierVersion, Task: state.Task, Run: state.Run, Source: state.Source,
		SupervisorDecisionID: state.SupervisorDecisionID, SupervisorDecisionSHA256: state.SupervisorDecisionSHA256,
		ArchitectureConstraints: state.ArchitectureConstraints, ProjectMap: state.ProjectMap,
		ModuleRelationships: state.ModuleRelationships, Conventions: state.Conventions,
		PriorDecisions: state.PriorDecisions, PriorPlan: state.PriorPlan,
		AcceptanceRequirements: append([]Criterion(nil), state.Task.Criteria...),
		BaselineVerification:   append([]Criterion(nil), state.Task.Criteria...), Omissions: omissions,
	}
	raw, err := json.Marshal(dossier)
	if err != nil {
		return DossierArtifact{}, err
	}
	if len(raw) > MaximumDossierBytes {
		return DossierArtifact{}, fmt.Errorf("dossier is %d bytes, maximum is %d", len(raw), MaximumDossierBytes)
	}
	return DossierArtifact{Version: DossierVersion, SHA256: model.SHA256(raw), ByteSize: len(raw), Content: raw}, nil
}

func validateCanonicalState(state CanonicalState) error {
	for label, value := range map[string]string{
		"task id": state.Task.TaskID, "task version id": state.Task.TaskVersionID,
		"run id": state.Run.RunID, "project id": state.Run.ProjectID,
		"project source id": state.Run.ProjectSourceID, "supervisor decision id": state.SupervisorDecisionID,
	} {
		if !token(value) {
			return fmt.Errorf("%s is missing or malformed", label)
		}
	}
	if state.Task.VersionNumber <= 0 || !validSHA(state.Source.Revision) || !validGitOID(state.Source.Commit) || !validGitOID(state.Source.Tree) || !validSHA(state.SupervisorDecisionSHA256) {
		return errors.New("task version, source, or supervisor decision identity is malformed")
	}
	if err := validateText("task title", state.Task.Title); err != nil {
		return err
	}
	if err := validateText("task goal", state.Task.Goal); err != nil {
		return err
	}
	if err := validateStringList("task scope", state.Task.Scope, true, false); err != nil {
		return err
	}
	if err := validateStringList("task excluded scope", state.Task.ExcludedScope, false, false); err != nil {
		return err
	}
	if err := validateStringList("task dependencies", state.Task.Dependencies, false, true); err != nil {
		return err
	}
	if err := validateStringList("task expected paths", state.Task.ExpectedPaths, true, false); err != nil {
		return err
	}
	for _, value := range state.Task.ExpectedPaths {
		if !cleanRelative(value) {
			return fmt.Errorf("task expected path %q is unsafe", value)
		}
	}
	if len(state.Task.Criteria) == 0 || len(state.Task.Criteria) > 128 {
		return errors.New("task criteria are empty or unbounded")
	}
	seenCriteria := map[string]struct{}{}
	for _, criterion := range state.Task.Criteria {
		if !stableID.MatchString(criterion.ID) {
			return fmt.Errorf("criterion identity %q is malformed", criterion.ID)
		}
		if _, duplicate := seenCriteria[criterion.ID]; duplicate {
			return fmt.Errorf("duplicate criterion %q", criterion.ID)
		}
		seenCriteria[criterion.ID] = struct{}{}
		if err := validateText("criterion requirement", criterion.Requirement); err != nil {
			return err
		}
		if criterion.VerificationMethod != "command" && criterion.VerificationMethod != "operator_checkpoint" {
			return fmt.Errorf("criterion %q uses unsupported verification %q", criterion.ID, criterion.VerificationMethod)
		}
		if strings.TrimSpace(criterion.VerificationReference) == "" || placeholder.MatchString(criterion.VerificationReference) {
			return fmt.Errorf("criterion %q verification is unresolved", criterion.ID)
		}
	}
	if len(state.ArchitectureConstraints) == 0 || len(state.ProjectMap) == 0 {
		return errors.New("architecture constraints and bounded project map are required")
	}
	for label, values := range map[string][]EvidenceItem{"architecture constraint": state.ArchitectureConstraints, "module relationship": state.ModuleRelationships, "convention": state.Conventions, "prior decision": state.PriorDecisions} {
		if err := validateEvidenceItems(label, values); err != nil {
			return err
		}
	}
	seenPaths := map[string]struct{}{}
	for _, item := range state.ProjectMap {
		if !cleanRelative(item.Path) || strings.TrimSpace(item.Component) == "" || strings.TrimSpace(item.Kind) == "" {
			return fmt.Errorf("project map item %q is malformed", item.Path)
		}
		if _, duplicate := seenPaths[item.Path]; duplicate {
			return fmt.Errorf("duplicate project map path %q", item.Path)
		}
		seenPaths[item.Path] = struct{}{}
	}
	if state.PriorPlan != nil {
		if !token(state.PriorPlan.PlanID) || !token(state.PriorPlan.PlanVersionID) || state.PriorPlan.Revision <= 0 || len(state.PriorPlan.Steps) == 0 {
			return errors.New("prior plan identity is malformed")
		}
		for i, step := range state.PriorPlan.Steps {
			if step.Ordinal != i+1 || !stableID.MatchString(step.ID) {
				return errors.New("prior plan steps are missing, duplicated, or reordered")
			}
		}
	}
	return nil
}

func validateEvidenceItems(label string, values []EvidenceItem) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !token(value.ID) || strings.TrimSpace(value.Kind) == "" || strings.TrimSpace(value.Summary) == "" {
			return fmt.Errorf("%s %q is malformed", label, value.ID)
		}
		if value.SHA256 != "" && !validSHA(value.SHA256) {
			return fmt.Errorf("%s %q has malformed hash", label, value.ID)
		}
		if _, duplicate := seen[value.ID]; duplicate {
			return fmt.Errorf("duplicate %s %q", label, value.ID)
		}
		seen[value.ID] = struct{}{}
	}
	return nil
}

func cloneAndSortState(state CanonicalState) CanonicalState {
	state.Task.Scope = append([]string(nil), state.Task.Scope...)
	state.Task.ExcludedScope = append([]string(nil), state.Task.ExcludedScope...)
	state.Task.Dependencies = append([]string(nil), state.Task.Dependencies...)
	state.Task.ExpectedPaths = append([]string(nil), state.Task.ExpectedPaths...)
	state.Task.Criteria = append([]Criterion(nil), state.Task.Criteria...)
	state.ArchitectureConstraints = sortedEvidence(state.ArchitectureConstraints)
	state.ModuleRelationships = sortedEvidence(state.ModuleRelationships)
	state.Conventions = sortedEvidence(state.Conventions)
	state.PriorDecisions = sortedEvidence(state.PriorDecisions)
	state.ProjectMap = append([]ProjectPath(nil), state.ProjectMap...)
	sort.Slice(state.ProjectMap, func(i, j int) bool { return state.ProjectMap[i].Path < state.ProjectMap[j].Path })
	if state.PriorPlan != nil {
		value := *state.PriorPlan
		value.Steps = append([]Step(nil), value.Steps...)
		state.PriorPlan = &value
	}
	return state
}

func dossierEvidence(state CanonicalState) map[string]struct{} {
	values := []string{"task:" + state.Task.TaskVersionID, "run:" + state.Run.RunID, "source:" + state.Source.Revision, "supervisor_decision:" + state.SupervisorDecisionID}
	for _, criterion := range state.Task.Criteria {
		values = append(values, "criterion:"+criterion.ID, "verification:"+criterion.ID)
	}
	for _, item := range state.ArchitectureConstraints {
		values = append(values, "architecture:"+item.ID)
	}
	for _, item := range state.ProjectMap {
		values = append(values, "project_path:"+item.Path)
	}
	for _, item := range state.ModuleRelationships {
		values = append(values, "relationship:"+item.ID)
	}
	for _, item := range state.Conventions {
		values = append(values, "convention:"+item.ID)
	}
	for _, item := range state.PriorDecisions {
		values = append(values, "decision:"+item.ID)
	}
	if state.PriorPlan != nil {
		values = append(values, "prior_plan:"+state.PriorPlan.PlanVersionID)
		for _, step := range state.PriorPlan.Steps {
			values = append(values, "prior_step:"+step.ID)
		}
	}
	slices.Sort(values)
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
