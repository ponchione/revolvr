package supervisor

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"revolvr/internal/model"
	"revolvr/internal/policy"
	"revolvr/internal/tasklifecycle"
)

const (
	SupervisorDossierVersion  = "revolvr-supervisor-dossier-v1"
	MaxSupervisorDossierBytes = 96 << 10
	maxDossierItems           = 128
	maxDossierTextBytes       = 4096
)

type TaskContext struct {
	TaskID          string   `json:"task_id"`
	TaskVersionID   string   `json:"task_version_id"`
	Version         int64    `json:"version"`
	ContractSummary string   `json:"contract_summary"`
	AllowedPaths    []string `json:"allowed_paths"`
	ExcludedPaths   []string `json:"excluded_paths"`
}

type RunContext struct {
	RunID string `json:"run_id"`
}

type SourceContext struct {
	Revision string `json:"revision"`
	Commit   string `json:"commit"`
	Tree     string `json:"tree"`
	Safe     bool   `json:"safe"`
}

type PlanContext struct {
	ID        string                `json:"id"`
	Completed bool                  `json:"completed"`
	Steps     []policy.PlanStepGate `json:"steps"`
}

type CriterionContext struct {
	ID          string `json:"id"`
	Requirement string `json:"requirement"`
	Status      string `json:"status"`
}

type VerificationContext struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	SourceRevision   string `json:"source_revision"`
	Final            bool   `json:"final"`
	EvidenceComplete bool   `json:"evidence_complete"`
	Summary          string `json:"summary"`
}

type AuditContext struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	SourceRevision   string `json:"source_revision"`
	VerificationID   string `json:"verification_id"`
	Independent      bool   `json:"independent"`
	EvidenceComplete bool   `json:"evidence_complete"`
	Summary          string `json:"summary"`
}

type FindingContext struct {
	ID      string `json:"id"`
	Open    bool   `json:"open"`
	Summary string `json:"summary"`
}

type AttemptContext struct {
	ID      string        `json:"id"`
	Action  policy.Action `json:"action"`
	Outcome string        `json:"outcome"`
}

type StrategyContext struct {
	ID       string   `json:"id"`
	Approach string   `json:"approach"`
	Targets  []string `json:"targets"`
}

type AuthorityDecision struct {
	ID        string `json:"id"`
	Authority string `json:"authority"`
	Decision  string `json:"decision"`
	SHA256    string `json:"sha256"`
}

type ArtifactManifestContext struct {
	ID       string `json:"id"`
	Complete bool   `json:"complete"`
}

type Omission struct {
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

// CanonicalState is a caller-supplied, trusted projection of the scheduler-
// pinned task/run and canonical workflow evidence. A StateReader must return a
// fresh projection before and after the model call.
type CanonicalState struct {
	Task                   TaskContext              `json:"task"`
	Run                    RunContext               `json:"run"`
	Source                 SourceContext            `json:"source"`
	Lifecycle              tasklifecycle.TaskStatus `json:"lifecycle"`
	Plan                   *PlanContext             `json:"plan"`
	Criteria               []CriterionContext       `json:"criteria"`
	LatestVerification     *VerificationContext     `json:"latest_verification"`
	LatestAudit            *AuditContext            `json:"latest_audit"`
	Findings               []FindingContext         `json:"findings"`
	Attempts               []AttemptContext         `json:"attempts"`
	Strategies             []StrategyContext        `json:"strategies"`
	Budget                 policy.Budget            `json:"budget"`
	HighAuthorityDecisions []AuthorityDecision      `json:"high_authority_decisions"`
	WorkspaceReconciled    bool                     `json:"workspace_reconciled"`
	ArtifactManifest       ArtifactManifestContext  `json:"artifact_manifest"`
}

type Dossier struct {
	SchemaVersion          string                   `json:"schema_version"`
	Task                   TaskContext              `json:"task"`
	Run                    RunContext               `json:"run"`
	Source                 SourceContext            `json:"source"`
	Lifecycle              tasklifecycle.TaskStatus `json:"lifecycle"`
	Plan                   *PlanContext             `json:"plan"`
	Criteria               []CriterionContext       `json:"criteria"`
	LatestVerification     *VerificationContext     `json:"latest_verification"`
	LatestAudit            *AuditContext            `json:"latest_audit"`
	OpenFindings           []FindingContext         `json:"open_findings"`
	Attempts               []AttemptContext         `json:"attempts"`
	Strategies             []StrategyContext        `json:"strategies"`
	Budget                 policy.Budget            `json:"budget"`
	HighAuthorityDecisions []AuthorityDecision      `json:"high_authority_decisions"`
	WorkspaceReconciled    bool                     `json:"workspace_reconciled"`
	ArtifactManifest       ArtifactManifestContext  `json:"artifact_manifest"`
	Omissions              []Omission               `json:"omissions"`
}

type DossierArtifact struct {
	Version  string          `json:"version"`
	SHA256   string          `json:"sha256"`
	ByteSize int             `json:"byte_size"`
	Content  json.RawMessage `json:"content"`
}

func BuildSupervisorDossier(state CanonicalState) (DossierArtifact, error) {
	if err := validateCanonicalState(state); err != nil {
		return DossierArtifact{}, fmt.Errorf("build supervisor dossier: %w", err)
	}
	openFindings := make([]FindingContext, 0, len(state.Findings))
	for _, finding := range state.Findings {
		if finding.Open {
			openFindings = append(openFindings, finding)
		}
	}
	dossier := Dossier{
		SchemaVersion:          SupervisorDossierVersion,
		Task:                   cloneTaskContext(state.Task),
		Run:                    state.Run,
		Source:                 state.Source,
		Lifecycle:              state.Lifecycle,
		Plan:                   clonePlanContext(state.Plan),
		Criteria:               append([]CriterionContext(nil), state.Criteria...),
		LatestVerification:     cloneVerification(state.LatestVerification),
		LatestAudit:            cloneAudit(state.LatestAudit),
		OpenFindings:           append([]FindingContext(nil), openFindings...),
		Attempts:               append([]AttemptContext(nil), state.Attempts...),
		Strategies:             cloneStrategies(state.Strategies),
		Budget:                 state.Budget,
		HighAuthorityDecisions: append([]AuthorityDecision(nil), state.HighAuthorityDecisions...),
		WorkspaceReconciled:    state.WorkspaceReconciled,
		ArtifactManifest:       state.ArtifactManifest,
		Omissions:              dossierOmissions(state),
	}
	raw, err := json.Marshal(dossier)
	if err != nil {
		return DossierArtifact{}, fmt.Errorf("encode: %w", err)
	}
	if len(raw) > MaxSupervisorDossierBytes {
		return DossierArtifact{}, fmt.Errorf("encoded dossier is %d bytes, limit is %d", len(raw), MaxSupervisorDossierBytes)
	}
	return DossierArtifact{Version: SupervisorDossierVersion, SHA256: model.SHA256(raw), ByteSize: len(raw), Content: append(json.RawMessage(nil), raw...)}, nil
}

func validateCanonicalState(state CanonicalState) error {
	identities := []struct {
		label string
		value string
	}{
		{"task_id", state.Task.TaskID}, {"task_version_id", state.Task.TaskVersionID}, {"run_id", state.Run.RunID},
		{"source_revision", state.Source.Revision}, {"source_commit", state.Source.Commit}, {"source_tree", state.Source.Tree},
		{"budget identity", state.Budget.IdentityID}, {"artifact manifest identity", state.ArtifactManifest.ID},
	}
	for _, identity := range identities {
		if !stableToken(identity.value) {
			return fmt.Errorf("%s is missing or not normalized", identity.label)
		}
	}
	if state.Task.Version <= 0 {
		return errors.New("task version must be positive")
	}
	if !validSHA256(state.Source.Revision) || !validGitOID(state.Source.Commit) || !validGitOID(state.Source.Tree) {
		return errors.New("source revision, commit, or tree identity is malformed")
	}
	if err := boundedText("task contract summary", state.Task.ContractSummary); err != nil {
		return err
	}
	if len(state.Task.AllowedPaths) == 0 {
		return errors.New("task scope must contain at least one allowed path")
	}
	if state.Budget.ModelCallsRemaining < 0 || state.Budget.WorkerAttemptsRemaining < 0 || state.Budget.TokensRemaining < 0 {
		return errors.New("budget counters must be nonnegative")
	}
	if err := boundedCount("criteria", len(state.Criteria)); err != nil {
		return err
	}
	for _, count := range []struct {
		name string
		n    int
	}{{"findings", len(state.Findings)}, {"attempts", len(state.Attempts)}, {"strategies", len(state.Strategies)}, {"high-authority decisions", len(state.HighAuthorityDecisions)}} {
		if err := boundedCount(count.name, count.n); err != nil {
			return err
		}
	}
	if err := uniqueCanonicalIdentities(state); err != nil {
		return err
	}
	return nil
}

func uniqueCanonicalIdentities(state CanonicalState) error {
	groups := []struct {
		label string
		ids   []string
	}{
		{"criterion", collectCriterionIDs(state.Criteria)},
		{"finding", collectFindingIDs(state.Findings)},
		{"attempt", collectAttemptIDs(state.Attempts)},
		{"strategy", collectStrategyIDs(state.Strategies)},
		{"authority decision", collectAuthorityIDs(state.HighAuthorityDecisions)},
	}
	for _, group := range groups {
		seen := make(map[string]struct{}, len(group.ids))
		for _, id := range group.ids {
			if !stableToken(id) {
				return fmt.Errorf("%s identity %q is missing or not normalized", group.label, id)
			}
			if _, duplicate := seen[id]; duplicate {
				return fmt.Errorf("duplicate %s identity %q", group.label, id)
			}
			seen[id] = struct{}{}
		}
	}
	for _, value := range state.Criteria {
		if err := boundedText("criterion requirement", value.Requirement); err != nil {
			return err
		}
	}
	for _, value := range state.Findings {
		if err := boundedText("finding summary", value.Summary); err != nil {
			return err
		}
	}
	for _, value := range state.Strategies {
		if err := boundedText("strategy approach", value.Approach); err != nil {
			return err
		}
	}
	for _, value := range state.HighAuthorityDecisions {
		if err := boundedText("authority decision", value.Decision); err != nil {
			return err
		}
		if !validSHA256(value.SHA256) {
			return fmt.Errorf("authority decision %q has malformed SHA-256", value.ID)
		}
	}
	return nil
}

func dossierOmissions(state CanonicalState) []Omission {
	omissions := []Omission{
		{Section: "broad_raw_source", Reason: "excluded by supervisor context policy"},
		{Section: "unrelated_code", Reason: "excluded by supervisor context policy"},
		{Section: "conversation_history", Reason: "fresh decision uses canonical durable state only"},
	}
	if state.Plan == nil {
		omissions = append(omissions, Omission{Section: "plan", Reason: "no canonical plan exists"})
	}
	if state.LatestVerification == nil {
		omissions = append(omissions, Omission{Section: "latest_verification", Reason: "no canonical verification exists"})
	}
	if state.LatestAudit == nil {
		omissions = append(omissions, Omission{Section: "latest_audit", Reason: "no canonical audit exists"})
	}
	if len(state.Findings) == 0 {
		omissions = append(omissions, Omission{Section: "open_findings", Reason: "no canonical findings exist"})
	}
	if len(state.Attempts) == 0 {
		omissions = append(omissions, Omission{Section: "attempt_history", Reason: "no canonical attempts exist"})
	}
	if len(state.Strategies) == 0 {
		omissions = append(omissions, Omission{Section: "strategy_history", Reason: "no canonical strategies exist"})
	}
	if len(state.HighAuthorityDecisions) == 0 {
		omissions = append(omissions, Omission{Section: "high_authority_decisions", Reason: "no applicable canonical decisions exist"})
	}
	return omissions
}

func evidenceIDs(state CanonicalState) map[string]struct{} {
	values := []string{
		"task:" + state.Task.TaskVersionID,
		"run:" + state.Run.RunID,
		"source:" + state.Source.Revision,
		"budget:" + state.Budget.IdentityID,
		"artifact_manifest:" + state.ArtifactManifest.ID,
	}
	if state.Plan != nil {
		values = append(values, "plan:"+state.Plan.ID)
	}
	if state.LatestVerification != nil {
		values = append(values, "verification:"+state.LatestVerification.ID)
	}
	if state.LatestAudit != nil {
		values = append(values, "audit:"+state.LatestAudit.ID)
	}
	for _, value := range state.Criteria {
		values = append(values, "criterion:"+value.ID)
	}
	for _, value := range state.Findings {
		values = append(values, "finding:"+value.ID)
	}
	for _, value := range state.Attempts {
		values = append(values, "attempt:"+value.ID)
	}
	for _, value := range state.Strategies {
		values = append(values, "strategy:"+value.ID)
	}
	for _, value := range state.HighAuthorityDecisions {
		values = append(values, "decision:"+value.ID)
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func stableToken(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && strings.IndexFunc(value, func(r rune) bool { return r <= ' ' || r == 0x7f }) < 0
}

func validSHA256(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && strings.ToLower(value) == value
}

func validGitOID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && (len(raw) == 20 || len(raw) == 32) && strings.ToLower(value) == value
}

func boundedText(label, value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxDossierTextBytes {
		return fmt.Errorf("%s must be nonblank and at most %d bytes", label, maxDossierTextBytes)
	}
	return nil
}

func boundedCount(label string, count int) error {
	if count > maxDossierItems {
		return fmt.Errorf("%s has %d items, limit is %d", label, count, maxDossierItems)
	}
	return nil
}

func collectCriterionIDs(values []CriterionContext) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].ID
	}
	return out
}
func collectFindingIDs(values []FindingContext) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].ID
	}
	return out
}
func collectAttemptIDs(values []AttemptContext) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].ID
	}
	return out
}
func collectStrategyIDs(values []StrategyContext) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].ID
	}
	return out
}
func collectAuthorityIDs(values []AuthorityDecision) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = values[i].ID
	}
	return out
}

func cloneTaskContext(value TaskContext) TaskContext {
	value.AllowedPaths = append([]string(nil), value.AllowedPaths...)
	value.ExcludedPaths = append([]string(nil), value.ExcludedPaths...)
	return value
}

func clonePlanContext(value *PlanContext) *PlanContext {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Steps = append([]policy.PlanStepGate(nil), value.Steps...)
	return &cloned
}

func cloneVerification(value *VerificationContext) *VerificationContext {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
func cloneAudit(value *AuditContext) *AuditContext {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStrategies(values []StrategyContext) []StrategyContext {
	out := append([]StrategyContext(nil), values...)
	for i := range out {
		out[i].Targets = append([]string(nil), out[i].Targets...)
	}
	return out
}
