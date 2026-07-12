package autonomousfinalization

import (
	"encoding/json"
	"fmt"
	"strings"

	"revolvr/internal/autonomous"
)

func RenderCapsule(e FrozenEvidence) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# Completion Capsule: %s\n\n", clean(e.Task.Title))
	fmt.Fprintf(&out, "- Task: `%s`\n- Outcome: completed\n- Finalization operation: `%s`\n- Finalization run: `%s`\n\n", e.Task.TaskID, e.OperationID, e.FinalizationRunID)
	out.WriteString("## Task and Specification\n\n")
	fmt.Fprintf(&out, "- Source: `%s` (`%s`, %d bytes)\n- Workflow: `%s`\n- Frozen state: `%s` (%d bytes)\n\n", e.Task.Path, e.Task.SHA256, e.Task.ByteSize, e.Task.Workflow, e.StateIdentity.SHA256, e.StateIdentity.ByteSize)
	out.WriteString("## Final Source and Workspace\n\n")
	fmt.Fprintf(&out, "- Workspace: `%s` (%s)\n- Branch: `%s`\n- HEAD: `%s`\n- Tree: `%s`\n- Source revision: `%s`\n- Checkpoint: %d / `%s`\n\n", e.Workspace.WorkspaceID, e.Workspace.Status, e.Workspace.BranchRef, e.Workspace.HeadSHA, e.Workspace.TreeSHA, e.Workspace.SourceRevision, e.Workspace.Checkpoint.Sequence, e.Workspace.Checkpoint.OperationID)
	out.WriteString("## Completion Decision\n\n")
	fmt.Fprintf(&out, "- Decision: `%s` from run `%s`\n- Action: `%s`\n- Decision artifact: `%s`\n", e.DecisionReference.DecisionID, e.DecisionReference.RunID, e.Decision.Action, e.DecisionReference.Artifact.Reference)
	for _, item := range e.Decision.Inputs {
		fmt.Fprintf(&out, "- Evidence: %s `%s`\n", item.Kind, item.Reference)
	}
	out.WriteString("\n## Completed Plan\n\n")
	fmt.Fprintf(&out, "Plan `%s`, revision %d, completed=%t.\n\n", e.State.Plan.ID, e.State.Plan.Revision, e.State.Plan.Completed)
	for i, step := range e.State.Plan.Steps {
		fmt.Fprintf(&out, "%d. `%s` — %s — **%s**\n", i+1, step.ID, clean(step.Description), step.Status)
	}
	out.WriteString("\n## Acceptance Matrix\n\n| Criterion | Status | Requirement | Evidence / rationale |\n|---|---|---|---|\n")
	for _, c := range e.State.AcceptanceCriteria {
		fmt.Fprintf(&out, "| `%s` | %s | %s | %s |\n", c.ID, c.Status, cell(c.Requirement), cell(evidenceText(c.Evidence, c.Rationale)))
	}
	out.WriteString("\n## Final Verification\n\n")
	v := e.Verification
	fmt.Fprintf(&out, "- Run / occurrence: `%s` / `%s`\n- Source: `%s`\n- Status: **%s**\n", v.Summary.RunID, v.Summary.OccurrenceID, v.SourceRevision, v.Summary.Status)
	if v.Tiered != nil {
		fmt.Fprintf(&out, "- Purpose: `%s`; final gate satisfied: `%t`\n- Required tiers: %s\n", v.Tiered.Purpose, v.Tiered.FinalSatisfied, codeList(v.Tiered.RequiredFinalTiers))
		for _, tier := range v.Tiered.RequiredOutcomes {
			fmt.Fprintf(&out, "- Tier `%s`: %s\n", tier.TierID, tier.Outcome)
		}
	}
	out.WriteString("\n## Independent Audit and Findings\n\n")
	fmt.Fprintf(&out, "- Audit run: `%s`\n- Auditor: `%s`\n- Disposition: **%s**\n- Consumed verification: `%s` / `%s`\n", e.Audit.RunID, e.Audit.AuditorProfile, e.Audit.Report.Disposition, e.Audit.VerificationRunID, e.Audit.VerificationOccurrenceID)
	if len(e.State.FindingResolutions) == 0 {
		out.WriteString("- Findings: none recorded.\n")
	}
	for _, finding := range e.State.FindingResolutions {
		fmt.Fprintf(&out, "- Finding `%s`: **%s** — %s\n", finding.FindingID, finding.Status, clean(evidenceText(finding.Evidence, finding.Rationale)))
	}
	out.WriteString("\n## Optional Roles\n\n")
	if len(e.State.OptionalRoles) == 0 {
		out.WriteString("- Documentor: omitted; no occurrence was required.\n- Simplifier: omitted; no occurrence was required.\n")
	}
	for _, role := range e.State.OptionalRoles {
		fmt.Fprintf(&out, "- `%s` occurrence %d: **%s**; decision `%s`; source `%s` → `%s`", role.Role, role.Sequence, role.Outcome, role.Decision.DecisionID, role.SourceBefore, role.SourceAfter)
		if role.CommitSHA != "" {
			fmt.Fprintf(&out, "; commit `%s`", role.CommitSHA)
		}
		out.WriteString(".\n")
	}
	out.WriteString("\n## Attempts, Runs, and Commits\n\n")
	fmt.Fprintf(&out, "- Attempts charged: %d; elapsed: %s; tokens: %d.\n", e.State.Attempts.TotalAttempts, e.State.Attempts.ElapsedTimeBudget.Consumed, e.State.Attempts.TokenBudget.Consumed)
	for _, run := range e.Runs {
		fmt.Fprintf(&out, "- Run %d `%s` (%s): %s; artifact `%s`.\n", run.Sequence, run.RunID, run.Kind, run.Outcome, run.Artifact.Reference)
	}
	if len(e.Commits) == 0 {
		out.WriteString("- Commits: none; final HEAD equals the workspace baseline.\n")
	}
	for _, commit := range e.Commits {
		fmt.Fprintf(&out, "- Commit %d `%s` from run `%s` (%s", commit.Sequence, commit.SHA, commit.RunID, commit.Outcome)
		if commit.Reconciled {
			out.WriteString(", reconciled")
		}
		out.WriteString(").\n")
	}
	out.WriteString("\n## Safety and Reproducibility\n\n")
	fmt.Fprintf(&out, "- Safety policy: `%s` (%s)\n- Ready preflight observed: `%s`\n- Effective config: `%s` / `%s`\n- Model: `%s`; reasoning: `%s`; ephemeral: `%t`\n", e.SafetyPolicy.PolicySHA256, e.SafetyPolicy.Mode, e.SafetyPreflight.ObservedAt.UTC().Format(timeLayout), e.EffectiveConfigSchema, e.EffectiveConfigSHA256, e.SafetyPolicy.Codex.Model, e.SafetyPolicy.Codex.ReasoningEffort, e.SafetyPolicy.Codex.Ephemeral)
	out.WriteString("\n## Waivers, Not Applicable, Omissions, and Warnings\n\n")
	any := false
	for _, c := range e.State.AcceptanceCriteria {
		if c.Status == "waived" || c.Status == "not_applicable" {
			any = true
			fmt.Fprintf(&out, "- Acceptance `%s`: **%s** — %s\n", c.ID, c.Status, clean(evidenceText(c.Evidence, c.Rationale)))
		}
	}
	for _, f := range e.State.FindingResolutions {
		if f.Status != "resolved" {
			any = true
			fmt.Fprintf(&out, "- Finding `%s`: **%s** — %s\n", f.FindingID, f.Status, clean(evidenceText(f.Evidence, f.Rationale)))
		}
	}
	if !any {
		out.WriteString("- No acceptance waivers/not-applicable dispositions or non-resolved finding dispositions.\n")
	}
	out.WriteString("- Optional role absence is explicit above and is not completion authority.\n- Capsule prose is informational; typed frozen evidence and manifest identities are authoritative.\n")
	out.WriteString("\n## Finalization\n\n")
	fmt.Fprintf(&out, "- Admitted at: `%s`\n- Terminal time: `%s`\n- Frozen evidence: `completion-evidence.json` (identity recorded by the manifest)\n- Capsule identity: recorded by `completion-manifest.json`\n", e.AdmittedAt.UTC().Format(timeLayout), e.TerminalAt.UTC().Format(timeLayout))
	return []byte(out.String()), nil
}

const timeLayout = "2006-01-02T15:04:05.000000000Z"

func BuildManifest(e FrozenEvidence, frozen, capsule autonomous.FinalizationArtifact) (Manifest, error) {
	sources := []SourceRecord{
		sourceRecord("task", e.Task.Path, e.Task), sourceRecord("state", e.StateIdentity.Path, e.State), sourceRecord("decision", e.DecisionReference.Artifact.Reference, e.Decision), sourceRecord("workspace", e.Workspace.WorkspaceID, e.Workspace), sourceRecord("verification", e.Verification.Summary.RunID+"/"+e.Verification.Summary.OccurrenceID, e.Verification), sourceRecord("audit", e.Audit.RunID, e.Audit), sourceRecord("safety", e.SafetyPolicy.PolicySHA256, e.SafetyPolicy), sourceRecord("config", e.EffectiveConfigSchema, struct {
			SHA256 string `json:"sha256"`
		}{e.EffectiveConfigSHA256}), sourceRecord("commits", "ordered-commit-evidence", e.Commits), sourceRecord("runs", "ordered-run-evidence", e.Runs),
	}
	omissions := []Omission{}
	if len(e.State.OptionalRoles) == 0 {
		omissions = append(omissions, Omission{Kind: "optional_roles", Reason: "no documentor or simplifier occurrence was required"})
	}
	if len(e.State.FindingResolutions) == 0 {
		omissions = append(omissions, Omission{Kind: "finding_resolutions", Reason: "no audit finding required a lifecycle disposition"})
	}
	m := Manifest{SchemaVersion: ManifestSchemaVersion, TaskID: e.Task.TaskID, OperationID: e.OperationID, FrozenEvidence: frozen, Capsule: capsule, Sources: sources, Omissions: omissions}
	return m, m.Validate()
}

func sourceRecord(kind, reference string, value any) SourceRecord {
	raw, _ := json.Marshal(value)
	return SourceRecord{Kind: kind, Reference: reference, SHA256: hash(raw), ByteSize: len(raw)}
}
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }
func cell(s string) string  { return strings.ReplaceAll(clean(s), "|", "\\|") }
func evidenceText(e []autonomous.EvidenceReference, rationale string) string {
	parts := []string{}
	for _, item := range e {
		parts = append(parts, fmt.Sprintf("%s %s", item.Kind, item.Reference))
	}
	if rationale != "" {
		parts = append(parts, rationale)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "; ")
}
func codeList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = "`" + v + "`"
	}
	return strings.Join(out, ", ")
}
