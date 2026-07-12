package autonomous

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const DossierTokenEstimatorSchema = "utf8-bytes-ceil-div-4-v1"

type DossierRole string

const (
	DossierRoleSupervisor  DossierRole = "supervisor"
	DossierRolePlanner     DossierRole = "planner"
	DossierRoleImplementer DossierRole = "implementer"
	DossierRoleAuditor     DossierRole = "auditor"
	DossierRoleCorrector   DossierRole = "corrector"
	DossierRoleDocumentor  DossierRole = "documentor"
	DossierRoleSimplifier  DossierRole = "simplifier"
)

type DossierProjection struct {
	SchemaVersion string      `json:"schema_version"`
	Role          DossierRole `json:"role"`
	Policy        string      `json:"policy"`
}

type DossierSectionFact struct {
	Section       string `json:"section"`
	Included      bool   `json:"included"`
	Reason        string `json:"reason,omitempty"`
	TotalByteSize int    `json:"total_byte_size"`
	IncludedBytes int    `json:"included_byte_size"`
	TotalItems    int    `json:"total_items"`
	IncludedItems int    `json:"included_items"`
	OmittedItems  int    `json:"omitted_items"`
	TokenEstimate int    `json:"token_estimate"`
	Estimator     string `json:"token_estimator"`
}

type DossierTokenEstimate struct {
	SchemaVersion string `json:"schema_version"`
	Estimated     int    `json:"estimated_tokens"`
	Limitation    string `json:"limitation"`
}

type DossierCacheFact struct {
	Key                 string `json:"key"`
	EntryManifestSHA256 string `json:"entry_manifest_sha256"`
	Result              string `json:"result"`
	Diagnostic          string `json:"diagnostic,omitempty"`
}

type projectedSection struct {
	name     string
	raw      []byte
	items    int
	included bool
	reason   string
}

// RoleForAction validates the existing action/profile contract and returns
// the one harness-owned dossier view. Unsupported combinations fail closed.
func RoleForAction(action Action, profile WorkerProfile) (DossierRole, error) {
	expected, worker := workerProfileForAction(action)
	if !worker || expected != profile {
		return "", fmt.Errorf("project dossier: action %q and profile %q are not a supported worker route", action, profile)
	}
	switch profile {
	case WorkerProfilePlanner:
		return DossierRolePlanner, nil
	case WorkerProfileImplementer:
		return DossierRoleImplementer, nil
	case WorkerProfileAuditor:
		return DossierRoleAuditor, nil
	case WorkerProfileCorrector:
		return DossierRoleCorrector, nil
	case WorkerProfileDocumentor:
		return DossierRoleDocumentor, nil
	case WorkerProfileSimplifier:
		return DossierRoleSimplifier, nil
	default:
		return "", fmt.Errorf("project dossier: unsupported profile %q", profile)
	}
}

// ReprojectTaskDossier renders a different role from the exact immutable
// evidence captured when the dossier was built. It performs no I/O.
func ReprojectTaskDossier(dossier TaskDossier, role DossierRole) (TaskDossier, error) {
	if dossier.input == nil {
		return TaskDossier{}, errors.New("project dossier: exact source input is unavailable")
	}
	return ProjectTaskDossier(*dossier.input, role)
}

// ProjectTaskDossier is the pure role projection boundary.
func ProjectTaskDossier(in TaskDossierInput, role DossierRole) (TaskDossier, error) {
	if !validDossierRole(role) {
		return TaskDossier{}, fmt.Errorf("project dossier: unknown role %q", role)
	}
	n, err := validateAndNormalizeDossierInput(in)
	if err != nil {
		return TaskDossier{}, err
	}
	sections := roleSections(n, role)
	var markdown bytes.Buffer
	facts := make([]DossierSectionFact, 0, len(sections))
	for _, section := range sections {
		includedBytes := 0
		includedItems := 0
		if section.included {
			markdown.Write(section.raw)
			includedBytes = len(section.raw)
			includedItems = section.items
		}
		facts = append(facts, DossierSectionFact{
			Section: section.name, Included: section.included, Reason: section.reason,
			TotalByteSize: len(section.raw), IncludedBytes: includedBytes,
			TotalItems: section.items, IncludedItems: includedItems, OmittedItems: section.items - includedItems,
			TokenEstimate: estimateTokens(includedBytes), Estimator: DossierTokenEstimatorSchema,
		})
	}
	writeRoleProjectionFacts(&markdown, facts)
	raw := markdown.Bytes()
	sources, err := buildDossierSourceRecords(n)
	if err != nil {
		return TaskDossier{}, err
	}
	manifest := TaskDossierManifest{
		SchemaVersion: RoleDossierManifestSchemaVersion, TaskID: in.TaskID,
		DossierSHA256: sha256HexBytes(raw), DossierByteSize: len(raw), Sources: sources,
		ProjectionFacts: append([]DossierProjectionFact(nil), n.facts...),
		Projection:      &DossierProjection{SchemaVersion: RoleDossierManifestSchemaVersion, Role: role, Policy: "role-section-matrix-v1"},
		Sections:        facts,
		TokenEstimate:   &DossierTokenEstimate{SchemaVersion: DossierTokenEstimatorSchema, Estimated: estimateTokens(len(raw)), Limitation: "Deterministic UTF-8 byte estimate; not actual model token usage."},
	}
	if in.RepositoryMap != nil && in.RepositoryMap.CacheResult != "" {
		manifest.Cache = &DossierCacheFact{Key: in.RepositoryMap.CacheKey, EntryManifestSHA256: in.RepositoryMap.CacheManifestSHA256, Result: in.RepositoryMap.CacheResult, Diagnostic: in.RepositoryMap.CacheDiagnostic}
	}
	cloned, err := cloneTaskDossierInput(in)
	if err != nil {
		return TaskDossier{}, err
	}
	return TaskDossier{Markdown: append([]byte(nil), raw...), Manifest: manifest, input: &cloned}, nil
}

func validDossierRole(role DossierRole) bool {
	switch role {
	case DossierRoleSupervisor, DossierRolePlanner, DossierRoleImplementer, DossierRoleAuditor, DossierRoleCorrector, DossierRoleDocumentor, DossierRoleSimplifier:
		return true
	default:
		return false
	}
}

func roleSections(n normalizedDossierInput, role DossierRole) []projectedSection {
	section := func(name string, items int, include bool, render func(*bytes.Buffer)) projectedSection {
		var b bytes.Buffer
		render(&b)
		reason := ""
		if !include {
			reason = "not_required_for_role"
		}
		return projectedSection{name: name, raw: b.Bytes(), items: items, included: include, reason: reason}
	}
	allControl := role == DossierRoleSupervisor
	needsVerification := role != DossierRolePlanner
	needsAudit := role == DossierRoleSupervisor || role == DossierRoleAuditor || role == DossierRoleCorrector || role == DossierRoleDocumentor || role == DossierRoleSimplifier
	needsHistory := role == DossierRoleSupervisor || role == DossierRolePlanner
	return []projectedSection{
		section("identity", 1, true, func(b *bytes.Buffer) { writeRoleIdentity(b, n, role) }),
		section("task_spec", 1, true, func(b *bytes.Buffer) { writeTaskSpec(b, n.input.TaskSpec) }),
		section("execution_state", 1, true, func(b *bytes.Buffer) { writeExecutionState(b, n.input.State) }),
		section("current_plan", boolCount(n.input.State.Plan != nil), true, func(b *bytes.Buffer) { writePlan(b, n.input.State.Plan) }),
		section("acceptance", len(n.input.State.AcceptanceCriteria), true, func(b *bytes.Buffer) { writeAcceptanceCriteria(b, n.input.State.AcceptanceCriteria) }),
		section("verification", boolCount(n.input.Verification != nil), needsVerification, func(b *bytes.Buffer) { writeVerification(b, n.input.Verification) }),
		section("audit_findings", len(n.input.State.FindingResolutions)+boolCount(n.input.Audit != nil), needsAudit, func(b *bytes.Buffer) { writeAuditAndResolutions(b, n.input.Audit, n.input.State.FindingResolutions) }),
		section("recent_runs", len(n.includedRuns), needsHistory, func(b *bytes.Buffer) { writeRecentRuns(b, n) }),
		section("git_snapshot", boolCount(n.input.Git != nil), true, func(b *bytes.Buffer) { writeGitSnapshot(b, n.input.Git) }),
		section("repository_guidance", len(n.guidance), true, func(b *bytes.Buffer) { writeGuidance(b, n.guidance) }),
		section("repository_map", boolCount(n.input.RepositoryMap != nil), true, func(b *bytes.Buffer) { writeRepositoryMap(b, n.input.RepositoryMap) }),
		section("legacy_projection_facts", len(n.facts), allControl, func(b *bytes.Buffer) { writeProjectionFacts(b, n.facts) }),
	}
}

func writeRoleIdentity(out *bytes.Buffer, n normalizedDossierInput, role DossierRole) {
	out.WriteString("# Autonomous Task Dossier\n\n## Dossier Identity\n\n")
	writeField(out, "Task ID", n.input.TaskID)
	writeField(out, "Manifest schema", RoleDossierManifestSchemaVersion)
	writeField(out, "Role projection", string(role))
	writeField(out, "Token estimator", DossierTokenEstimatorSchema)
	out.WriteByte('\n')
}

func writeRoleProjectionFacts(out *bytes.Buffer, facts []DossierSectionFact) {
	out.WriteString("## Role Projection and Size Facts\n\n")
	for _, fact := range facts {
		fmt.Fprintf(out, "- %s: included=%t bytes=%d/%d items=%d/%d omitted=%d estimated_tokens=%d", fact.Section, fact.Included, fact.IncludedBytes, fact.TotalByteSize, fact.IncludedItems, fact.TotalItems, fact.OmittedItems, fact.TokenEstimate)
		if fact.Reason != "" {
			fmt.Fprintf(out, " reason=%s", fact.Reason)
		}
		out.WriteByte('\n')
	}
	out.WriteString("\nToken estimates use ")
	out.WriteString(DossierTokenEstimatorSchema)
	out.WriteString(" and are deterministic estimates, not actual Codex usage.\n")
}

func estimateTokens(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func validateRepositoryMapSource(source RepositoryMapSource) error {
	if strings.TrimSpace(source.ID) == "" || source.ID != strings.TrimSpace(source.ID) {
		return errors.New("build task dossier: repository_map.id is required and normalized")
	}
	if len(source.CommitSHA) != 40 || len(source.TreeSHA) != 40 {
		return errors.New("build task dossier: repository_map commit/tree identities must be 40-character Git object IDs")
	}
	if len(source.Content) == 0 || !utf8.Valid(source.Content) {
		return errors.New("build task dossier: repository_map.content must be nonempty UTF-8")
	}
	if source.CacheResult != "" && (len(source.CacheKey) != sha256.Size*2 || len(source.CacheManifestSHA256) != sha256.Size*2) {
		return errors.New("build task dossier: cached repository map requires key and manifest SHA-256 identities")
	}
	return nil
}

func writeRepositoryMap(out *bytes.Buffer, source *RepositoryMapSource) {
	out.WriteString("## Deterministic Repository Map\n\n")
	if source == nil {
		out.WriteString("No committed-tree repository map was supplied.\n\n")
		return
	}
	writeField(out, "Source ID", source.ID)
	writeField(out, "Commit SHA", source.CommitSHA)
	writeField(out, "Tree SHA", source.TreeSHA)
	out.WriteByte('\n')
	writeSourceContent(out, source.Content)
}

func cloneTaskDossierInput(in TaskDossierInput) (TaskDossierInput, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return TaskDossierInput{}, fmt.Errorf("clone dossier input: %w", err)
	}
	var out TaskDossierInput
	if err := json.Unmarshal(raw, &out); err != nil {
		return TaskDossierInput{}, fmt.Errorf("clone dossier input: %w", err)
	}
	out.TaskSpec.Content = append([]byte(nil), in.TaskSpec.Content...)
	out.Guidance = make([]GuidanceSource, len(in.Guidance))
	for i := range in.Guidance {
		out.Guidance[i] = in.Guidance[i]
		out.Guidance[i].Content = append([]byte(nil), in.Guidance[i].Content...)
	}
	out.Receipts = make([]ReceiptSource, len(in.Receipts))
	for i := range in.Receipts {
		out.Receipts[i] = in.Receipts[i]
		out.Receipts[i].Content = append([]byte(nil), in.Receipts[i].Content...)
	}
	if in.RepositoryMap != nil {
		m := *in.RepositoryMap
		m.Content = append([]byte(nil), in.RepositoryMap.Content...)
		out.RepositoryMap = &m
	}
	return out, nil
}
