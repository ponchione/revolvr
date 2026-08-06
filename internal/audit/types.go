// Package audit owns the independent, source-bound audit contract. Model
// output is advisory until this package validates and persists it against
// immutable verification and source authority.
package audit

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
	"unicode/utf8"

	"revolvr/internal/model"
	"revolvr/internal/verification"
)

const (
	DossierSchemaVersion = "revolvr-audit-dossier-v1"
	OutputSchemaVersion  = "revolvr-audit-output-v1"
	OutputSchemaName     = "revolvr_audit_output_v1"
	PromptVersion        = "revolvr-auditor-prompt-v1"
	AuditRecordVersion   = "revolvr-audit-run-v1"
	FindingVersion       = "revolvr-audit-finding-v1"
	MaximumDossierBytes  = 4 << 20
	MaximumFindings      = 256
)

var (
	ErrRejected       = errors.New("audit output rejected")
	ErrStaleAuthority = errors.New("audit authority is stale")
	ErrPersistence    = errors.New("audit persistence failed")
	stableIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
)

type Kind string

const (
	KindBase             Kind = "base"
	KindSecurity         Kind = "security"
	KindPerformance      Kind = "performance"
	KindIntegration      Kind = "integration"
	KindMigration        Kind = "migration"
	KindDocumentation    Kind = "documentation"
	KindAPICompatibility Kind = "api_compatibility"
)

type Disposition string

const (
	DispositionClean           Disposition = "clean"
	DispositionChangesRequired Disposition = "changes_required"
	DispositionBlocked         Disposition = "blocked"
)

type Significance string

const (
	SignificanceBlocking    Significance = "blocking"
	SignificanceNonBlocking Significance = "non_blocking"
)

type CriterionImpactKind string

const (
	ImpactViolated   CriterionImpactKind = "violated"
	ImpactAtRisk     CriterionImpactKind = "at_risk"
	ImpactUnverified CriterionImpactKind = "unverified"
)

type Identity struct {
	ProjectID     string `json:"project_id"`
	TaskID        string `json:"task_id"`
	TaskVersionID string `json:"task_version_id"`
	RunID         string `json:"run_id"`
	WorkspaceID   string `json:"workspace_id"`
}

type Source struct {
	Revision   string `json:"revision"`
	Commit     string `json:"commit"`
	Tree       string `json:"tree"`
	DiffSHA256 string `json:"diff_sha256"`
}

type ArtifactIdentity struct {
	ID          string `json:"id"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	MediaType   string `json:"media_type"`
	LogicalKind string `json:"logical_kind"`
	StoragePath string `json:"storage_path"`
}

type TaskEvidence struct {
	ExternalID    string           `json:"external_id"`
	Title         string           `json:"title"`
	Goal          string           `json:"goal"`
	RiskClass     string           `json:"risk_class"`
	MutationClass string           `json:"mutation_class"`
	Scope         []string         `json:"scope"`
	ExcludedScope []string         `json:"excluded_scope"`
	Artifact      ArtifactIdentity `json:"artifact"`
}

type PlanStep struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	Description  string   `json:"description"`
	CriterionIDs []string `json:"criterion_ids"`
}

type PlanEvidence struct {
	ID        string           `json:"id"`
	VersionID string           `json:"version_id"`
	SHA256    string           `json:"sha256"`
	Steps     []PlanStep       `json:"steps"`
	Artifact  ArtifactIdentity `json:"artifact"`
}

type Criterion struct {
	ID                    string `json:"id"`
	Requirement           string `json:"requirement"`
	Status                string `json:"status"`
	VerificationReference string `json:"verification_reference"`
}

type ChangedFile struct {
	Path    string   `json:"path"`
	Status  string   `json:"status"`
	SHA256  string   `json:"sha256"`
	Symbols []string `json:"symbols"`
}

type DiffEvidence struct {
	Artifact ArtifactIdentity `json:"artifact"`
	Patch    string           `json:"patch"`
	Files    []ChangedFile    `json:"files"`
}

type VerificationCheck struct {
	ID                   string               `json:"id"`
	Tier                 verification.Tier    `json:"tier"`
	Outcome              verification.Outcome `json:"outcome"`
	ExecutionFingerprint string               `json:"execution_fingerprint"`
	ReusedFromCheckID    string               `json:"reused_from_check_id,omitempty"`
}

type VerificationEvidence struct {
	ID          string                      `json:"id"`
	Purpose     verification.Purpose        `json:"purpose"`
	Status      verification.RunStatus      `json:"status"`
	Source      verification.SourceIdentity `json:"source"`
	CompletedAt time.Time                   `json:"completed_at"`
	Checks      []VerificationCheck         `json:"checks"`
	Artifact    ArtifactIdentity            `json:"artifact"`
}

type BlastRadiusEdge struct {
	From     string `json:"from"`
	Relation string `json:"relation"`
	To       string `json:"to"`
}

type SourceFile struct {
	Path       string   `json:"path"`
	SHA256     string   `json:"sha256"`
	SizeBytes  int64    `json:"size_bytes"`
	Symbols    []string `json:"symbols"`
	ArtifactID string   `json:"artifact_id"`
	Content    string   `json:"content"`
}

type PriorFinding struct {
	ID               string       `json:"id"`
	Significance     Significance `json:"significance"`
	Status           string       `json:"status"`
	DefinitionSHA256 string       `json:"definition_sha256"`
}

type DossierInput struct {
	Identity       Identity             `json:"identity"`
	Source         Source               `json:"source"`
	Task           TaskEvidence         `json:"task"`
	Plan           PlanEvidence         `json:"plan"`
	Criteria       []Criterion          `json:"acceptance_matrix"`
	Diff           DiffEvidence         `json:"exact_diff"`
	Verification   VerificationEvidence `json:"verification"`
	BlastRadius    []BlastRadiusEdge    `json:"blast_radius"`
	RelevantSource []SourceFile         `json:"relevant_source"`
	PriorFindings  []PriorFinding       `json:"prior_findings"`
}

type Dossier struct {
	SchemaVersion string            `json:"schema_version"`
	AuditKind     Kind              `json:"audit_kind"`
	Input         DossierInput      `json:"input"`
	Omissions     []DossierOmission `json:"omissions"`
}

type DossierOmission struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

type DossierArtifact struct {
	SchemaVersion string          `json:"schema_version"`
	SHA256        string          `json:"sha256"`
	ByteSize      int             `json:"byte_size"`
	Content       json.RawMessage `json:"content"`
}

type Citation struct {
	ArtifactID string `json:"artifact_id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
}

type CriterionImpact struct {
	CriterionID string              `json:"criterion_id"`
	Impact      CriterionImpactKind `json:"impact"`
	Detail      string              `json:"detail"`
}

type Finding struct {
	ID                 string            `json:"id"`
	Significance       Significance      `json:"significance"`
	Summary            string            `json:"summary"`
	RequiredCorrection string            `json:"required_correction"`
	SourceEvidence     []Citation        `json:"source_evidence"`
	AffectedFiles      []string          `json:"affected_files"`
	AffectedSymbols    []string          `json:"affected_symbols"`
	CriterionImpact    []CriterionImpact `json:"criterion_impact"`
}

type OutputAuthority struct {
	AuditID              string `json:"audit_id"`
	AuditKind            Kind   `json:"audit_kind"`
	TaskID               string `json:"task_id"`
	TaskVersionID        string `json:"task_version_id"`
	RunID                string `json:"run_id"`
	SourceRevision       string `json:"source_revision"`
	SourceCommit         string `json:"source_commit"`
	SourceTree           string `json:"source_tree"`
	DiffSHA256           string `json:"diff_sha256"`
	VerificationRunID    string `json:"verification_run_id"`
	DossierSchemaVersion string `json:"dossier_schema_version"`
	DossierSHA256        string `json:"dossier_sha256"`
}

type Output struct {
	RevolvrIdentity model.OutputIdentity `json:"revolvr_identity"`
	SchemaVersion   string               `json:"schema_version"`
	Authority       OutputAuthority      `json:"authority"`
	Disposition     Disposition          `json:"disposition"`
	Rationale       string               `json:"rationale"`
	BlockedReason   string               `json:"blocked_reason"`
	Findings        []Finding            `json:"findings"`
}

func BuildDossier(input DossierInput, kind Kind) (DossierArtifact, error) {
	if err := validateDossierInput(input); err != nil {
		return DossierArtifact{}, fmt.Errorf("build audit dossier: %w", err)
	}
	if !validKind(kind) {
		return DossierArtifact{}, fmt.Errorf("build audit dossier: unknown audit kind %q", kind)
	}
	input = normalizeDossierInput(input)
	dossier := Dossier{
		SchemaVersion: DossierSchemaVersion,
		AuditKind:     kind,
		Input:         input,
		Omissions: []DossierOmission{
			{Section: "conversation_history", Reason: "fresh independent audit uses durable evidence only"},
			{Section: "source_mutation_tools", Reason: "auditor is read-only"},
			{Section: "unrelated_source", Reason: "bounded source and blast-radius evidence only"},
		},
	}
	raw, err := json.Marshal(dossier)
	if err != nil {
		return DossierArtifact{}, err
	}
	if len(raw) > MaximumDossierBytes {
		return DossierArtifact{}, fmt.Errorf("build audit dossier: %d bytes exceeds %d", len(raw), MaximumDossierBytes)
	}
	return DossierArtifact{SchemaVersion: DossierSchemaVersion, SHA256: model.SHA256(raw), ByteSize: len(raw), Content: raw}, nil
}

func ParseOutput(raw []byte) (Output, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Output{}, fmt.Errorf("%w: output is missing", ErrRejected)
	}
	if err := rejectDuplicateJSONFields(raw); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrRejected, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var output Output
	if err := decoder.Decode(&output); err != nil {
		return Output{}, fmt.Errorf("%w: decode exactly one JSON object: %v", ErrRejected, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Output{}, fmt.Errorf("%w: content follows the audit object", ErrRejected)
	}
	return output, nil
}

func ValidateOutput(output Output, dossier DossierArtifact, expected OutputAuthority, identity model.OutputIdentity) error {
	if output.SchemaVersion != OutputSchemaVersion || output.RevolvrIdentity != identity {
		return fmt.Errorf("%w: schema or task-013 output identity is stale", ErrRejected)
	}
	if output.Authority != expected || output.Authority.DossierSchemaVersion != dossier.SchemaVersion || output.Authority.DossierSHA256 != dossier.SHA256 {
		return fmt.Errorf("%w: task, source, verification, dossier, or audit identity is stale", ErrRejected)
	}
	var decoded Dossier
	if err := json.Unmarshal(dossier.Content, &decoded); err != nil || decoded.SchemaVersion != dossier.SchemaVersion || decoded.AuditKind != expected.AuditKind {
		return fmt.Errorf("%w: dossier bytes are malformed or divergent", ErrRejected)
	}
	if strings.TrimSpace(output.Rationale) == "" || output.Rationale != strings.TrimSpace(output.Rationale) {
		return fmt.Errorf("%w: audit rationale is required and normalized", ErrRejected)
	}
	switch output.Disposition {
	case DispositionClean:
		if len(output.Findings) != 0 || output.BlockedReason != "" {
			return fmt.Errorf("%w: clean audit cannot contain findings or a blocked reason", ErrRejected)
		}
	case DispositionChangesRequired:
		if len(output.Findings) == 0 || output.BlockedReason != "" {
			return fmt.Errorf("%w: changes_required needs findings and no blocked reason", ErrRejected)
		}
	case DispositionBlocked:
		if len(output.Findings) != 0 || strings.TrimSpace(output.BlockedReason) == "" || output.BlockedReason != strings.TrimSpace(output.BlockedReason) {
			return fmt.Errorf("%w: blocked audit needs one normalized reason and no findings", ErrRejected)
		}
	default:
		return fmt.Errorf("%w: unknown disposition %q", ErrRejected, output.Disposition)
	}
	if len(output.Findings) > MaximumFindings {
		return fmt.Errorf("%w: finding count exceeds %d", ErrRejected, MaximumFindings)
	}
	allowedCriteria := make(map[string]struct{}, len(decoded.Input.Criteria))
	for _, criterion := range decoded.Input.Criteria {
		allowedCriteria[criterion.ID] = struct{}{}
	}
	allowedFiles := make(map[string]SourceFile, len(decoded.Input.RelevantSource))
	for _, file := range decoded.Input.RelevantSource {
		allowedFiles[file.Path] = file
	}
	seen := make(map[string]struct{}, len(output.Findings))
	for index, finding := range output.Findings {
		if err := validateFinding(finding, allowedFiles, allowedCriteria); err != nil {
			return fmt.Errorf("%w: finding %d: %v", ErrRejected, index+1, err)
		}
		if _, duplicate := seen[finding.ID]; duplicate {
			return fmt.Errorf("%w: duplicate finding ID %q", ErrRejected, finding.ID)
		}
		seen[finding.ID] = struct{}{}
	}
	return nil
}

func FindingDefinitionSHA256(finding Finding) string {
	raw, _ := json.Marshal(normalizeFinding(finding))
	return model.SHA256(raw)
}

func validateFinding(f Finding, files map[string]SourceFile, criteria map[string]struct{}) error {
	if !stableIDPattern.MatchString(f.ID) {
		return fmt.Errorf("finding ID %q is not stable lower-case kebab-case", f.ID)
	}
	if f.Significance != SignificanceBlocking && f.Significance != SignificanceNonBlocking {
		return fmt.Errorf("finding %q has unknown significance %q", f.ID, f.Significance)
	}
	if !normalizedText(f.Summary) || !normalizedText(f.RequiredCorrection) {
		return fmt.Errorf("finding %q lacks a normalized summary or required correction", f.ID)
	}
	if len(f.SourceEvidence) == 0 || len(f.AffectedFiles) == 0 || len(f.CriterionImpact) == 0 {
		return fmt.Errorf("finding %q lacks source evidence, affected files, or criterion impact", f.ID)
	}
	cited := make(map[string]struct{}, len(f.SourceEvidence))
	citedSymbols := map[string]struct{}{}
	for _, citation := range f.SourceEvidence {
		file, ok := files[citation.Path]
		if !ok || citation.ArtifactID == "" || citation.ArtifactID != file.ArtifactID || citation.SHA256 != file.SHA256 || !validSHA256(citation.SHA256) {
			return fmt.Errorf("finding %q cites unknown or divergent source %q", f.ID, citation.Path)
		}
		if citation.StartLine <= 0 || citation.EndLine < citation.StartLine {
			return fmt.Errorf("finding %q has invalid source line evidence", f.ID)
		}
		lineCount := strings.Count(file.Content, "\n") + 1
		if citation.EndLine > lineCount {
			return fmt.Errorf("finding %q cites lines beyond exact source %q", f.ID, citation.Path)
		}
		cited[citation.Path] = struct{}{}
		for _, symbol := range file.Symbols {
			citedSymbols[symbol] = struct{}{}
		}
	}
	if err := validateUniqueStrings("affected files", f.AffectedFiles, true); err != nil {
		return err
	}
	for _, affected := range f.AffectedFiles {
		if _, ok := cited[affected]; !ok {
			return fmt.Errorf("finding %q affected file %q has no exact source citation", f.ID, affected)
		}
	}
	if err := validateUniqueStrings("affected symbols", f.AffectedSymbols, false); err != nil {
		return err
	}
	for _, symbol := range f.AffectedSymbols {
		if _, ok := citedSymbols[symbol]; !ok {
			return fmt.Errorf("finding %q affected symbol %q is absent from cited source evidence", f.ID, symbol)
		}
	}
	seenCriteria := map[string]struct{}{}
	for _, impact := range f.CriterionImpact {
		if _, ok := criteria[impact.CriterionID]; !ok {
			return fmt.Errorf("finding %q cites unknown criterion %q", f.ID, impact.CriterionID)
		}
		if _, duplicate := seenCriteria[impact.CriterionID]; duplicate {
			return fmt.Errorf("finding %q duplicates criterion impact %q", f.ID, impact.CriterionID)
		}
		seenCriteria[impact.CriterionID] = struct{}{}
		if impact.Impact != ImpactViolated && impact.Impact != ImpactAtRisk && impact.Impact != ImpactUnverified || !normalizedText(impact.Detail) {
			return fmt.Errorf("finding %q has malformed criterion impact", f.ID)
		}
	}
	return nil
}

func validateDossierInput(input DossierInput) error {
	for label, value := range map[string]string{
		"project ID": input.Identity.ProjectID, "task ID": input.Identity.TaskID,
		"task version ID": input.Identity.TaskVersionID, "run ID": input.Identity.RunID,
		"workspace ID": input.Identity.WorkspaceID,
	} {
		if !token(value) {
			return fmt.Errorf("%s is missing or malformed", label)
		}
	}
	if !validSHA256(input.Source.Revision) || !validGitOID(input.Source.Commit) || !validGitOID(input.Source.Tree) || !validSHA256(input.Source.DiffSHA256) {
		return errors.New("source revision, commit, tree, or diff identity is malformed")
	}
	if !normalizedText(input.Task.Title) || !normalizedText(input.Task.Goal) || !token(input.Task.ExternalID) || !token(input.Task.RiskClass) || !token(input.Task.MutationClass) || validateArtifact(input.Task.Artifact) != nil {
		return errors.New("task contract or artifact evidence is malformed")
	}
	if err := validateUniqueStrings("task scope", input.Task.Scope, true); err != nil {
		return err
	}
	if err := validateUniqueStrings("task excluded scope", input.Task.ExcludedScope, true); err != nil {
		return err
	}
	if input.Plan.ID == "" || input.Plan.VersionID == "" || !validSHA256(input.Plan.SHA256) || len(input.Plan.Steps) == 0 || validateArtifact(input.Plan.Artifact) != nil {
		return errors.New("accepted plan evidence is incomplete")
	}
	stepIDs := map[string]struct{}{}
	for _, step := range input.Plan.Steps {
		if !token(step.ID) || !normalizedText(step.Description) || len(step.CriterionIDs) == 0 {
			return fmt.Errorf("plan step %q is malformed", step.ID)
		}
		if _, duplicate := stepIDs[step.ID]; duplicate {
			return fmt.Errorf("duplicate plan step %q", step.ID)
		}
		stepIDs[step.ID] = struct{}{}
	}
	if len(input.Criteria) == 0 {
		return errors.New("acceptance matrix is empty")
	}
	criteria := map[string]struct{}{}
	for _, criterion := range input.Criteria {
		if !token(criterion.ID) || !normalizedText(criterion.Requirement) || !token(criterion.Status) || strings.TrimSpace(criterion.VerificationReference) == "" {
			return fmt.Errorf("criterion %q is malformed", criterion.ID)
		}
		if _, duplicate := criteria[criterion.ID]; duplicate {
			return fmt.Errorf("duplicate criterion %q", criterion.ID)
		}
		criteria[criterion.ID] = struct{}{}
	}
	for _, step := range input.Plan.Steps {
		seenCriteria := map[string]struct{}{}
		for _, criterionID := range step.CriterionIDs {
			if _, ok := criteria[criterionID]; !ok {
				return fmt.Errorf("plan step %q cites unknown criterion %q", step.ID, criterionID)
			}
			if _, duplicate := seenCriteria[criterionID]; duplicate {
				return fmt.Errorf("plan step %q repeats criterion %q", step.ID, criterionID)
			}
			seenCriteria[criterionID] = struct{}{}
		}
	}
	if validateArtifact(input.Diff.Artifact) != nil || input.Diff.Artifact.SHA256 != input.Source.DiffSHA256 || !utf8.ValidString(input.Diff.Patch) || input.Diff.Artifact.SHA256 != model.SHA256([]byte(input.Diff.Patch)) || input.Diff.Artifact.SizeBytes != int64(len(input.Diff.Patch)) {
		return errors.New("exact diff artifact is missing or divergent")
	}
	changed := map[string]struct{}{}
	for _, file := range input.Diff.Files {
		if !cleanRelative(file.Path) || !token(file.Status) || !validSHA256(file.SHA256) {
			return fmt.Errorf("changed file %q is malformed", file.Path)
		}
		if _, duplicate := changed[file.Path]; duplicate {
			return fmt.Errorf("duplicate changed file %q", file.Path)
		}
		changed[file.Path] = struct{}{}
		if err := validateUniqueStrings("changed symbols", file.Symbols, false); err != nil {
			return fmt.Errorf("changed file %q: %w", file.Path, err)
		}
	}
	if input.Verification.ID == "" || input.Verification.Purpose != verification.PurposeFinal || input.Verification.Status != verification.RunPassed || input.Verification.Source.Commit != input.Source.Commit || input.Verification.Source.Tree != input.Source.Tree || input.Verification.CompletedAt.IsZero() || len(input.Verification.Checks) == 0 || validateArtifact(input.Verification.Artifact) != nil {
		return errors.New("fresh final architecture-017 verification evidence is missing or stale")
	}
	freshFinal := false
	checkIDs := map[string]struct{}{}
	for _, check := range input.Verification.Checks {
		if check.ID == "" || !validSHA256(check.ExecutionFingerprint) {
			return errors.New("verification check identity is malformed")
		}
		if _, duplicate := checkIDs[check.ID]; duplicate {
			return fmt.Errorf("duplicate verification check %q", check.ID)
		}
		checkIDs[check.ID] = struct{}{}
		freshFinal = freshFinal || check.Tier == verification.TierFinal && check.Outcome == verification.OutcomePassed && check.ReusedFromCheckID == ""
	}
	if !freshFinal {
		return errors.New("audit requires one freshly executed passing Tier 4 check")
	}
	if len(input.RelevantSource) == 0 {
		return errors.New("bounded relevant source is empty")
	}
	relevantPaths := map[string]SourceFile{}
	for _, file := range input.RelevantSource {
		if !cleanRelative(file.Path) || !validSHA256(file.SHA256) || file.SizeBytes < 0 || !token(file.ArtifactID) || !utf8.ValidString(file.Content) || file.SHA256 != model.SHA256([]byte(file.Content)) || file.SizeBytes != int64(len(file.Content)) {
			return fmt.Errorf("relevant source %q is malformed", file.Path)
		}
		if _, duplicate := relevantPaths[file.Path]; duplicate {
			return fmt.Errorf("duplicate relevant source %q", file.Path)
		}
		relevantPaths[file.Path] = file
		if err := validateUniqueStrings("relevant source symbols", file.Symbols, false); err != nil {
			return fmt.Errorf("relevant source %q: %w", file.Path, err)
		}
	}
	for _, file := range input.Diff.Files {
		if relevant, ok := relevantPaths[file.Path]; ok && relevant.SHA256 != file.SHA256 {
			return fmt.Errorf("changed file %q diverges from exact relevant source", file.Path)
		}
	}
	blastEdges := map[string]struct{}{}
	for _, edge := range input.BlastRadius {
		if !normalizedText(edge.From) || !token(edge.Relation) || !normalizedText(edge.To) {
			return errors.New("blast-radius evidence is malformed")
		}
		key := edge.From + "\x00" + edge.Relation + "\x00" + edge.To
		if _, duplicate := blastEdges[key]; duplicate {
			return errors.New("blast-radius evidence contains a duplicate")
		}
		blastEdges[key] = struct{}{}
	}
	priorIDs := map[string]struct{}{}
	for _, finding := range input.PriorFindings {
		if !stableIDPattern.MatchString(finding.ID) || finding.Significance != SignificanceBlocking && finding.Significance != SignificanceNonBlocking || !token(finding.Status) || !validSHA256(finding.DefinitionSHA256) {
			return fmt.Errorf("prior finding %q is malformed", finding.ID)
		}
		if _, duplicate := priorIDs[finding.ID]; duplicate {
			return fmt.Errorf("duplicate prior finding %q", finding.ID)
		}
		priorIDs[finding.ID] = struct{}{}
	}
	return nil
}

func normalizeDossierInput(input DossierInput) DossierInput {
	input.Task.Scope = nonNilStrings(input.Task.Scope)
	input.Task.ExcludedScope = nonNilStrings(input.Task.ExcludedScope)
	input.Plan.Steps = append([]PlanStep(nil), input.Plan.Steps...)
	input.Criteria = append([]Criterion(nil), input.Criteria...)
	input.Diff.Files = append([]ChangedFile(nil), input.Diff.Files...)
	for i := range input.Diff.Files {
		input.Diff.Files[i].Symbols = nonNilStrings(input.Diff.Files[i].Symbols)
		sort.Strings(input.Diff.Files[i].Symbols)
	}
	sort.Slice(input.Diff.Files, func(i, j int) bool { return input.Diff.Files[i].Path < input.Diff.Files[j].Path })
	input.Verification.Checks = append([]VerificationCheck(nil), input.Verification.Checks...)
	input.BlastRadius = append([]BlastRadiusEdge(nil), input.BlastRadius...)
	input.RelevantSource = append([]SourceFile(nil), input.RelevantSource...)
	for i := range input.RelevantSource {
		input.RelevantSource[i].Symbols = nonNilStrings(input.RelevantSource[i].Symbols)
		sort.Strings(input.RelevantSource[i].Symbols)
	}
	input.PriorFindings = append([]PriorFinding(nil), input.PriorFindings...)
	sort.Slice(input.BlastRadius, func(i, j int) bool {
		a, b := input.BlastRadius[i], input.BlastRadius[j]
		return a.From+"\x00"+a.Relation+"\x00"+a.To < b.From+"\x00"+b.Relation+"\x00"+b.To
	})
	sort.Slice(input.RelevantSource, func(i, j int) bool { return input.RelevantSource[i].Path < input.RelevantSource[j].Path })
	sort.Slice(input.PriorFindings, func(i, j int) bool { return input.PriorFindings[i].ID < input.PriorFindings[j].ID })
	return input
}

func normalizeFinding(f Finding) Finding {
	f.SourceEvidence = append([]Citation(nil), f.SourceEvidence...)
	f.AffectedFiles = append([]string(nil), f.AffectedFiles...)
	f.AffectedSymbols = append([]string(nil), f.AffectedSymbols...)
	f.CriterionImpact = append([]CriterionImpact(nil), f.CriterionImpact...)
	sort.Slice(f.SourceEvidence, func(i, j int) bool {
		a, b := f.SourceEvidence[i], f.SourceEvidence[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.StartLine != b.StartLine {
			return a.StartLine < b.StartLine
		}
		return a.EndLine < b.EndLine
	})
	sort.Strings(f.AffectedFiles)
	sort.Strings(f.AffectedSymbols)
	sort.Slice(f.CriterionImpact, func(i, j int) bool { return f.CriterionImpact[i].CriterionID < f.CriterionImpact[j].CriterionID })
	return f
}

func validKind(kind Kind) bool {
	return slices.Contains([]Kind{KindBase, KindSecurity, KindPerformance, KindIntegration, KindMigration, KindDocumentation, KindAPICompatibility}, kind)
}

func validateArtifact(value ArtifactIdentity) error {
	if value.ID == "" || !validSHA256(value.SHA256) || value.SizeBytes < 0 || strings.TrimSpace(value.MediaType) == "" || strings.TrimSpace(value.LogicalKind) == "" || !cleanRelative(value.StoragePath) {
		return errors.New("artifact identity is malformed")
	}
	return nil
}

func validateUniqueStrings(label string, values []string, requirePaths bool) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) || requirePaths && !cleanRelative(value) {
			return fmt.Errorf("%s contains malformed value %q", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s repeats %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func cleanRelative(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func normalizedText(value string) bool {
	return strings.TrimSpace(value) != "" && value == strings.TrimSpace(value) && len(value) <= 65536
}

func token(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, " \t\r\n")
}

func validSHA256(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}

func validGitOID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && (len(raw) == 20 || len(raw) == 32) && value == strings.ToLower(value)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func rejectDuplicateJSONFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		value, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := value.(json.Delim)
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
				if _, duplicate := seen[key]; duplicate {
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
