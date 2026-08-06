package audit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"revolvr/internal/model"
	"revolvr/internal/verification"
)

func TestAuditDossierOutputAndSpecialistRouting(t *testing.T) {
	input := auditFixtureInput()
	first, err := BuildDossier(input, KindBase)
	if err != nil {
		t.Fatal(err)
	}
	reordered := input
	reordered.BlastRadius = []BlastRadiusEdge{input.BlastRadius[1], input.BlastRadius[0]}
	reordered.RelevantSource = []SourceFile{input.RelevantSource[1], input.RelevantSource[0]}
	second, err := BuildDossier(reordered, KindBase)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || !reflect.DeepEqual(first.Content, second.Content) {
		t.Fatal("equivalent dossier input did not normalize deterministically")
	}
	routes := RouteSpecialists(input.Task, input.Diff.Files, input.BlastRadius)
	want := []Kind{KindSecurity, KindPerformance, KindIntegration, KindMigration, KindDocumentation, KindAPICompatibility}
	got := make([]Kind, len(routes))
	for i := range routes {
		got[i] = routes[i].Kind
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("specialist routes = %v, want %v", got, want)
	}

	policy, err := PinModelPolicy("gpt-fixture", "high", 4096, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{AuditID: uuidAudit, AuditorInvocationID: uuidAuditor, SourceMutatingInvocationIDs: []string{"implementer-1"}, Kind: KindBase, Input: input, ModelPolicy: policy, StateReader: staticAuditReader{input: input}}
	cfg.Model = rejectingAuditModel{}
	prepared, err := Prepare(cfg)
	if err != nil {
		t.Fatal(err)
	}

	fixtures := []struct {
		name        string
		disposition Disposition
		findings    []Finding
		blocked     string
	}{
		{"clean", DispositionClean, []Finding{}, ""},
		{"changes-required", DispositionChangesRequired, []Finding{auditFixtureFinding()}, ""},
		{"blocked", DispositionBlocked, []Finding{}, "missing operator-owned design authority"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			output := Output{RevolvrIdentity: prepared.OutputIdentity, SchemaVersion: OutputSchemaVersion, Authority: prepared.Authority, Disposition: fixture.disposition, Rationale: "Independent evidence review completed.", BlockedReason: fixture.blocked, Findings: fixture.findings}
			if err := ValidateOutput(output, prepared.Dossier, prepared.Authority, prepared.OutputIdentity); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAuditRejectsMalformedRefusedStaleAndUncitedOutput(t *testing.T) {
	input := auditFixtureInput()
	policy, _ := PinModelPolicy("gpt-fixture", "high", 4096, time.Minute)
	cfg := Config{AuditID: uuidAudit, AuditorInvocationID: uuidAuditor, SourceMutatingInvocationIDs: []string{"implementer-1"}, Kind: KindBase, Input: input, ModelPolicy: policy, StateReader: staticAuditReader{input: input}}
	prepared, err := Prepare(Config{AuditID: cfg.AuditID, AuditorInvocationID: cfg.AuditorInvocationID, SourceMutatingInvocationIDs: cfg.SourceMutatingInvocationIDs, Kind: cfg.Kind, Input: cfg.Input, ModelPolicy: cfg.ModelPolicy, Model: rejectingAuditModel{}, StateReader: cfg.StateReader})
	if err != nil {
		t.Fatal(err)
	}
	valid := Output{RevolvrIdentity: prepared.OutputIdentity, SchemaVersion: OutputSchemaVersion, Authority: prepared.Authority, Disposition: DispositionChangesRequired, Rationale: "Evidence shows a correction is required.", Findings: []Finding{auditFixtureFinding()}}

	cases := []struct {
		name   string
		mutate func(*Output)
	}{
		{"stale-source", func(o *Output) { o.Authority.SourceTree = strings.Repeat("0", 40) }},
		{"unknown-significance", func(o *Output) { o.Findings[0].Significance = "critical" }},
		{"duplicate-id", func(o *Output) { o.Findings = append(o.Findings, o.Findings[0]) }},
		{"uncited-file", func(o *Output) { o.Findings[0].AffectedFiles = []string{"internal/missing.go"} }},
		{"uncited-symbol", func(o *Output) { o.Findings[0].AffectedSymbols = []string{"InventedSymbol"} }},
		{"divergent-citation", func(o *Output) { o.Findings[0].SourceEvidence[0].SHA256 = strings.Repeat("0", 64) }},
		{"unknown-criterion", func(o *Output) { o.Findings[0].CriterionImpact[0].CriterionID = "AC-unknown" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneAuditOutput(t, valid)
			test.mutate(&candidate)
			if err := ValidateOutput(candidate, prepared.Dossier, prepared.Authority, prepared.OutputIdentity); !errors.Is(err, ErrRejected) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	raw, _ := json.Marshal(valid)
	duplicate := strings.Replace(string(raw), `"schema_version":"`+OutputSchemaVersion+`"`, `"schema_version":"`+OutputSchemaVersion+`","schema_version":"`+OutputSchemaVersion+`"`, 1)
	if _, err := ParseOutput([]byte(duplicate)); !errors.Is(err, ErrRejected) {
		t.Fatalf("duplicate JSON error = %v", err)
	}
	if _, err := ParseOutput(append(raw, []byte(` {}`)...)); !errors.Is(err, ErrRejected) {
		t.Fatalf("trailing JSON error = %v", err)
	}

	refused := cfg
	refused.Model = resultAuditModel{err: errors.New("refused")}
	if _, err := Run(context.Background(), refused); !errors.Is(err, ErrRejected) {
		t.Fatalf("refusal error = %v", err)
	}
	selfAudit := cfg
	selfAudit.SourceMutatingInvocationIDs = []string{uuidAuditor}
	selfAudit.Model = rejectingAuditModel{}
	if _, err := Prepare(selfAudit); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("self-audit error = %v", err)
	}
	forgedRequest := prepared.ExpectedRequest
	forgedRequest.ReasoningEffort = "low"
	forged := cfg
	forged.Model = resultAuditModel{result: modelResultFor(t, prepared, valid)}
	forgedResult := forged.Model.(resultAuditModel)
	forgedResult.result.Request = forgedRequest
	forged.Model = forgedResult
	if _, err := Run(context.Background(), forged); !errors.Is(err, ErrRejected) {
		t.Fatalf("forged model provenance error = %v", err)
	}
	staleInput := input
	staleInput.Source.Tree = strings.Repeat("9", 40)
	stale := cfg
	stale.StateReader = staticAuditReader{input: staleInput}
	stale.Model = resultAuditModel{result: modelResultFor(t, prepared, valid)}
	if _, err := Run(context.Background(), stale); !errors.Is(err, ErrStaleAuthority) {
		t.Fatalf("stale error = %v", err)
	}
}

func TestFindingDispositionAuthoritiesAreClosed(t *testing.T) {
	base := DispositionCommand{
		ID:           uuid.NewString(),
		OperationID:  "disposition-" + uuid.NewString(),
		TaskID:       uuidTask,
		FindingID:    "missing-cache-check",
		AuthorityID:  "authority-1",
		SourceCommit: strings.Repeat("a", 40),
		SourceTree:   strings.Repeat("b", 40),
		Evidence: []DispositionEvidence{{
			Kind: "artifact", ID: "finding-evidence", SHA256: strings.Repeat("c", 64), Reference: "audit/evidence.json",
		}},
		CreatedAt: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
	}
	valid := []DispositionCommand{
		withDisposition(base, FindingResolved, "host", uuidVerification, uuidAudit, "", ""),
		withDisposition(base, FindingWaived, "operator", "", "", "", "operator accepted the documented risk"),
		withDisposition(base, FindingRejected, "operator", "", "", "", "operator rejected the finding as inapplicable"),
		withDisposition(base, FindingSuperseded, "host", "", "", "replacement-finding", ""),
		withDisposition(base, FindingStale, "host", "", "", "", ""),
	}
	for _, command := range valid {
		command.ID = uuid.NewString()
		command.OperationID = "disposition-" + uuid.NewString()
		if _, err := validateDisposition(command); err != nil {
			t.Fatalf("valid %s disposition: %v", command.Status, err)
		}
	}
	invalid := []DispositionCommand{
		withDisposition(base, FindingResolved, "auditor", uuidVerification, uuidAudit, "", ""),
		withDisposition(base, FindingWaived, "host", "", "", "", "host waiver"),
		withDisposition(base, FindingRejected, "auditor", "", "", "", "auditor rejection"),
		withDisposition(base, FindingSuperseded, "host", "", "", base.FindingID, ""),
		withDisposition(base, FindingStale, "operator", "", "", "", ""),
	}
	for _, command := range invalid {
		if _, err := validateDisposition(command); err == nil {
			t.Fatalf("invalid %s/%s disposition was accepted", command.Status, command.AuthorityRole)
		}
	}
}

func TestAuditMigrationIsReversible(t *testing.T) {
	raw, err := os.ReadFile("../../db/migrations/00011_audit_correction.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{
		"-- +goose Up", "-- +goose Down", "DROP TABLE core.audit_runs",
		"DROP TABLE core.audit_findings", "DROP TABLE core.finding_dispositions",
		"DROP TABLE core.failure_signatures", "DROP TABLE core.strategies",
		"DROP TABLE core.strategy_outcomes",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
}

func withDisposition(base DispositionCommand, status FindingStatus, role, verificationID, auditID, supersedingID, rationale string) DispositionCommand {
	base.Status = status
	base.AuthorityRole = role
	base.ResolutionVerificationRunID = verificationID
	base.ResolutionAuditRunID = auditID
	base.SupersedingFindingID = supersedingID
	base.Rationale = rationale
	return base
}

const (
	uuidProject                 = "00000000-0000-0000-0000-000000000001"
	uuidTask                    = "00000000-0000-0000-0000-000000000002"
	uuidVersion                 = "00000000-0000-0000-0000-000000000003"
	uuidRun                     = "00000000-0000-0000-0000-000000000004"
	uuidWorkspace               = "00000000-0000-0000-0000-000000000005"
	uuidVerification            = "00000000-0000-0000-0000-000000000006"
	uuidCheck                   = "00000000-0000-0000-0000-000000000007"
	uuidAudit                   = "00000000-0000-0000-0000-000000000008"
	uuidAuditor                 = "00000000-0000-0000-0000-000000000009"
	auditFixtureSecuritySource  = "package api\n\nfunc Authenticate() {\n\tloadToken()\n\tcheckPermission()\n\tcacheLookup()\n}\n\nfunc Cache() {\n\tvalidateEntry()\n\tserveEntry()\n}\n"
	auditFixtureMigrationSource = "CREATE TABLE core.audit_runs (id uuid PRIMARY KEY);\n"
	auditFixtureReadmeSource    = "# Audit and correction\n\nSee the exact evidence contract.\n"
	auditFixturePatch           = "diff --git a/internal/api/security_cache.go b/internal/api/security_cache.go\n+validate cache entry\n"
)

func auditFixtureInput() DossierInput {
	commit, tree := strings.Repeat("a", 40), strings.Repeat("b", 40)
	diffSHA := model.SHA256([]byte(auditFixturePatch))
	artifact := func(id, sha, kind, path string) ArtifactIdentity {
		return ArtifactIdentity{ID: id, SHA256: sha, SizeBytes: 12, MediaType: "application/json", LogicalKind: kind, StoragePath: path}
	}
	return DossierInput{
		Identity: Identity{ProjectID: uuidProject, TaskID: uuidTask, TaskVersionID: uuidVersion, RunID: uuidRun, WorkspaceID: uuidWorkspace},
		Source:   Source{Revision: strings.Repeat("d", 64), Commit: commit, Tree: tree, DiffSHA256: diffSHA},
		Task:     TaskEvidence{ExternalID: "architecture-019", Title: "Auditor", Goal: "Audit exact changes.", RiskClass: "high", MutationClass: "api_schema", Scope: []string{"internal"}, ExcludedScope: []string{"vendor"}, Artifact: artifact("task-artifact", strings.Repeat("1", 64), "task_source", "audit/task.json")},
		Plan:     PlanEvidence{ID: "plan-1", VersionID: "plan-version-1", SHA256: strings.Repeat("2", 64), Steps: []PlanStep{{ID: "implement", Status: "completed", Description: "Implement bounded audit.", CriterionIDs: []string{"AC-1"}}}, Artifact: artifact("plan-artifact", strings.Repeat("2", 64), "plan_source", "audit/plan.json")},
		Criteria: []Criterion{{ID: "AC-1", Requirement: "Audit is exact.", Status: "passed", VerificationReference: "go test ./..."}},
		Diff: DiffEvidence{Artifact: ArtifactIdentity{ID: "diff-artifact", SHA256: diffSHA, SizeBytes: int64(len(auditFixturePatch)), MediaType: "text/x-diff", LogicalKind: "diff", StoragePath: "audit/diff.patch"}, Patch: auditFixturePatch, Files: []ChangedFile{
			{Path: "db/migrations/00011.sql", Status: "added", SHA256: model.SHA256([]byte(auditFixtureMigrationSource)), Symbols: []string{"audit_runs"}},
			{Path: "internal/api/security_cache.go", Status: "modified", SHA256: model.SHA256([]byte(auditFixtureSecuritySource)), Symbols: []string{"Authenticate", "Cache"}},
			{Path: "README.md", Status: "modified", SHA256: model.SHA256([]byte(auditFixtureReadmeSource)), Symbols: []string{}},
		}},
		Verification: VerificationEvidence{ID: uuidVerification, Purpose: verification.PurposeFinal, Status: verification.RunPassed, Source: verification.SourceIdentity{Commit: commit, Tree: tree}, CompletedAt: time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC), Checks: []VerificationCheck{{ID: uuidCheck, Tier: verification.TierFinal, Outcome: verification.OutcomePassed, ExecutionFingerprint: strings.Repeat("6", 64)}}, Artifact: artifact("verification-artifact", strings.Repeat("7", 64), "verification_report", "audit/verification.json")},
		BlastRadius:  []BlastRadiusEdge{{From: "Authenticate", Relation: "calls", To: "Cache"}, {From: "audit_runs", Relation: "consumed_by", To: "completion"}},
		RelevantSource: []SourceFile{
			{Path: "internal/api/security_cache.go", SHA256: model.SHA256([]byte(auditFixtureSecuritySource)), SizeBytes: int64(len(auditFixtureSecuritySource)), Symbols: []string{"Authenticate", "Cache"}, ArtifactID: "source-security", Content: auditFixtureSecuritySource},
			{Path: "db/migrations/00011.sql", SHA256: model.SHA256([]byte(auditFixtureMigrationSource)), SizeBytes: int64(len(auditFixtureMigrationSource)), Symbols: []string{"audit_runs"}, ArtifactID: "source-migration", Content: auditFixtureMigrationSource},
		},
		PriorFindings: []PriorFinding{},
	}
}

func auditFixtureFinding() Finding {
	return Finding{ID: "missing-cache-check", Significance: SignificanceBlocking, Summary: "The authentication cache path is not checked.", RequiredCorrection: "Add the cited cache validation and its focused test.", SourceEvidence: []Citation{{ArtifactID: "source-security", Path: "internal/api/security_cache.go", SHA256: model.SHA256([]byte(auditFixtureSecuritySource)), StartLine: 10, EndLine: 12}}, AffectedFiles: []string{"internal/api/security_cache.go"}, AffectedSymbols: []string{"Authenticate"}, CriterionImpact: []CriterionImpact{{CriterionID: "AC-1", Impact: ImpactViolated, Detail: "The required exact audit behavior is absent."}}}
}

type staticAuditReader struct{ input DossierInput }

func (s staticAuditReader) ReadAuditState(context.Context, Identity) (DossierInput, error) {
	return s.input, nil
}

type rejectingAuditModel struct{}

func (rejectingAuditModel) Invoke(context.Context, model.PreparedRequest) (model.Result, error) {
	return model.Result{}, errors.New("not invoked")
}

type resultAuditModel struct {
	result model.Result
	err    error
}

func (m resultAuditModel) Invoke(context.Context, model.PreparedRequest) (model.Result, error) {
	return m.result, m.err
}

func modelResultFor(t *testing.T, prepared Prepared, output Output) model.Result {
	t.Helper()
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	return model.Result{Outcome: model.OutcomeSuccess, Request: prepared.ExpectedRequest, StructuredOutput: raw}
}
func cloneAuditOutput(t *testing.T, input Output) Output {
	t.Helper()
	raw, _ := json.Marshal(input)
	var out Output
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
