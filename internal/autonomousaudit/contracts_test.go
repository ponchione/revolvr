package autonomousaudit

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousverification"
)

func TestAuditOutputStrictCanonicalAndSchema(t *testing.T) {
	for _, disposition := range []autonomous.AuditDisposition{autonomous.AuditDispositionClean, autonomous.AuditDispositionChangesRequired} {
		t.Run(string(disposition), func(t *testing.T) {
			output := auditOutput(disposition)
			raw, err := MarshalAuditOutput(output)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseAuditOutput(raw)
			if err != nil || !reflect.DeepEqual(parsed, output) || raw[len(raw)-1] != '\n' {
				t.Fatalf("round trip = %+v, %v", parsed, err)
			}
			second, _ := MarshalAuditOutput(output)
			if !bytes.Equal(raw, second) {
				t.Fatal("canonical output is nondeterministic")
			}
		})
	}
	schema, err := AuditOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := AuditOutputSchema()
	if !bytes.Equal(schema, second) || !bytes.Contains(schema, []byte(`"additionalProperties": false`)) || !bytes.Contains(schema, []byte(AuditOutputSchemaVersion)) {
		t.Fatal("schema is not strict and deterministic")
	}
}

func TestParseAuditOutputRejectsMissingMalformedMultipleUnknownAndInvalid(t *testing.T) {
	raw, _ := MarshalAuditOutput(auditOutput(autonomous.AuditDispositionClean))
	unknown := bytes.Replace(raw, []byte(`"schema_version":`), []byte(`"unknown":true,"schema_version":`), 1)
	tests := []struct {
		name string
		raw  []byte
		want string
	}{{"missing", nil, "missing or empty"}, {"malformed", []byte(`{"schema_version":`), "decode exactly one"}, {"multiple", append(append([]byte{}, raw...), []byte(` {}`)...), "more than one"}, {"unknown", unknown, "unknown field"}, {"invalid report", bytes.Replace(raw, []byte(`"rationale": "clean"`), []byte(`"rationale": ""`), 1), "rationale is required"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAuditOutput(tt.raw)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}

func TestApplyReportFindingIdentityAndCleanRules(t *testing.T) {
	previous := readyState()
	decision := auditDecision()
	first := auditOutput(autonomous.AuditDispositionChangesRequired)
	change, err := ApplyReport(previous, first, decision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(change.NewFindingIDs) != 1 || change.State.Lifecycle != autonomous.LifecycleStateReady || change.State.LatestDecision.DecisionID != "decision-audit" || change.State.FindingResolutions[0].Status != autonomous.FindingResolutionStatusOpen {
		t.Fatalf("change=%+v", change)
	}
	before := mustAuditJSON(t, previous)
	if !bytes.Equal(before, mustAuditJSON(t, previous)) {
		t.Fatal("caller state mutated")
	}

	tests := []struct {
		name   string
		mutate func(*AuditOutput)
		want   string
	}{
		{"stable repeated", func(*AuditOutput) {}, ""},
		{"significance", func(o *AuditOutput) { o.Report.Findings[0].Significance = autonomous.FindingSignificanceNonBlocking }, "changed significance"},
		{"summary", func(o *AuditOutput) { o.Report.Findings[0].Summary = "Different defect" }, "summary meaning"},
		{"correction", func(o *AuditOutput) { o.Report.Findings[0].RequiredCorrection = "Different repair" }, "required correction"},
		{"evidence removed", func(o *AuditOutput) {
			o.Report.Findings[0].Evidence = []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindFile, "other")}
		}, "removed or reordered"},
		{"renamed", func(o *AuditOutput) { o.Report.Findings[0].ID = "finding-renamed" }, "probable rename"},
		{"open disappears", func(o *AuditOutput) { *o = auditOutput(autonomous.AuditDispositionClean) }, "disappeared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := auditOutput(autonomous.AuditDispositionChangesRequired)
			tt.mutate(&candidate)
			_, err := ApplyReport(change.State, candidate, decision, []autonomous.AuditReport{first.Report})
			if tt.want == "" && err != nil {
				t.Fatal(err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}

func TestApplyResolutionEveryTerminalStatus(t *testing.T) {
	state := readyState()
	state.FindingResolutions = []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}, {FindingID: "finding-two", Status: autonomous.FindingResolutionStatusOpen}}
	findings := []autonomous.AuditFinding{auditFinding("finding-one"), auditFinding("finding-two")}
	tests := []struct {
		status  autonomous.FindingResolutionStatus
		request ResolutionRequest
	}{
		{autonomous.FindingResolutionStatusResolved, ResolutionRequest{Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindVerification, "verify-resolution")}}},
		{autonomous.FindingResolutionStatusWaived, ResolutionRequest{Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindTask, "waiver")}, Rationale: "Explicit authority accepted the risk."}},
		{autonomous.FindingResolutionStatusInvalid, ResolutionRequest{Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindRepository, "invalid-proof")}, Rationale: "The cited path is unreachable."}},
		{autonomous.FindingResolutionStatusSuperseded, ResolutionRequest{Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindAudit, "replacement")}, SupersedingFindingID: "finding-two"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			request := tt.request
			request.FindingID = "finding-one"
			request.Status = tt.status
			next, resolution, err := ApplyResolution(state, request, findings)
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Status != tt.status || !reflect.DeepEqual(next.FindingResolutions[1], state.FindingResolutions[1]) || next.Lifecycle != autonomous.LifecycleStateReady {
				t.Fatalf("next=%+v", next)
			}
		})
	}

	bad := []struct {
		name    string
		request ResolutionRequest
		want    string
	}{{"missing evidence", ResolutionRequest{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusResolved}, "requires typed evidence"}, {"waiver rationale", ResolutionRequest{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusWaived, Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindTask, "x")}}, "rationale"}, {"self supersession", ResolutionRequest{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusSuperseded, Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindAudit, "x")}, SupersedingFindingID: "finding-one"}, "different target"}, {"unknown target", ResolutionRequest{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusSuperseded, Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindAudit, "x")}, SupersedingFindingID: "finding-three"}, "unknown superseding"}, {"unknown finding", ResolutionRequest{FindingID: "finding-three", Status: autonomous.FindingResolutionStatusResolved, Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindVerification, "x")}}, "unknown finding"}}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ApplyResolution(state, tt.request, findings)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestAuditOutputTieredFinalProvenance(t *testing.T) {
	output := auditOutput(autonomous.AuditDispositionClean)
	gate := autonomousverification.GateEvidence{SchemaVersion: autonomousverification.GateSchemaVersion, Plan: autonomousverification.PlanIdentity{SchemaVersion: autonomousverification.PlanSchemaVersion, SHA256: strings.Repeat("a", 64), ByteSize: 12}, Purpose: autonomousverification.PurposeFinal, RequiredFinalTiers: []string{"full-suite"}, SelectedTiers: []string{"full-suite"}, ExecutedTiers: []string{"full-suite"}, RequiredOutcomes: []autonomousverification.TierGate{{TierID: "full-suite", Outcome: autonomousverification.OutcomePassed}}, OverallOutcome: autonomousverification.OutcomePassed, FinalSatisfied: true}
	output.Provenance.Verification.Tiered = &gate
	raw, err := MarshalAuditOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseAuditOutput(raw)
	if err != nil || parsed.Provenance.Verification.Tiered == nil || !parsed.Provenance.Verification.Tiered.FinalSatisfied {
		t.Fatalf("parsed=%+v err=%v", parsed, err)
	}
	fast := gate
	fast.Purpose = autonomousverification.PurposeFast
	fast.SelectedTiers = nil
	fast.ExecutedTiers = nil
	fast.RequiredOutcomes = nil
	fast.MissingRequired = []string{"full-suite"}
	fast.FinalSatisfied = false
	output.Provenance.Verification.Tiered = &fast
	if err := output.Validate(); err == nil || (!strings.Contains(err.Error(), "final gate") && !strings.Contains(err.Error(), "cannot project as passed")) {
		t.Fatalf("fast validation=%v", err)
	}
	flaky := gate
	flaky.OverallOutcome = autonomousverification.OutcomeFlaky
	flaky.RequiredOutcomes[0].Outcome = autonomousverification.OutcomeFlaky
	flaky.FinalSatisfied = false
	output.Provenance.Verification.Tiered = &flaky
	if err := output.Validate(); err == nil || (!strings.Contains(err.Error(), "final gate") && !strings.Contains(err.Error(), "cannot project as passed")) {
		t.Fatalf("flaky validation=%v", err)
	}
}

func auditOutput(disposition autonomous.AuditDisposition) AuditOutput {
	verification := autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: "task-1", Status: autonomous.VerificationStatusPassed, Summary: "passed", RunID: "run-verification", OccurrenceID: "occurrence-one", Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindVerification, "run-verification:occurrence-one")}}, SourceRevision: strings.Repeat("1", 64)}
	report := autonomous.AuditReport{TaskID: "task-1", Disposition: disposition, Rationale: "clean", Inputs: append([]autonomous.EvidenceReference(nil), verification.Summary.Evidence...)}
	if disposition == autonomous.AuditDispositionChangesRequired {
		report.Rationale = "changes required"
		report.Findings = []autonomous.AuditFinding{auditFinding("finding-one")}
	}
	return AuditOutput{SchemaVersion: AuditOutputSchemaVersion, TaskID: "task-1", Report: report, Provenance: AuditProvenance{Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, WorkerRunID: "run-auditor", Decision: auditReference(), Dossier: DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", SHA256: strings.Repeat("2", 64), ByteSize: 100}, Profile: ProfileIdentity{Name: autonomous.WorkerProfileAuditor, Path: ".agent/profiles/auditor.md", SHA256: strings.Repeat("3", 64), ByteSize: 10}, RawOutputPath: ".revolvr/runs/run-auditor/auditor-output.raw.json", SourceRevision: strings.Repeat("1", 64), Verification: verification, LatestSourceMutation: &SourceMutationIdentity{TaskID: "task-1", RunID: "run-worker", DecisionID: "decision-worker", Action: autonomous.ActionImplement, ResultingRevision: strings.Repeat("1", 64)}}}
}
func auditFinding(id string) autonomous.AuditFinding {
	return autonomous.AuditFinding{ID: id, Significance: autonomous.FindingSignificanceBlocking, Summary: "The exact defect remains.", Evidence: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindFile, "internal/example.go:10")}, RequiredCorrection: "Correct the exact defect."}
}
func auditDecision() autonomous.SupervisorDecision {
	return autonomous.SupervisorDecision{TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Rationale: "Audit current source.", SuccessCriteria: []string{"Return an independent report."}, Inputs: []autonomous.EvidenceReference{auditEvidence(autonomous.EvidenceKindVerification, "run-verification:occurrence-one")}}
}
func auditReference() autonomous.DecisionReference {
	return autonomous.DecisionReference{DecisionID: "decision-audit", RunID: "run-supervisor", TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: auditEvidence(autonomous.EvidenceKindFile, ".revolvr/runs/run-supervisor/supervisor-decision.json"), CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
}
func readyState() autonomous.ExecutionState {
	return autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-1", Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
}
func auditEvidence(kind autonomous.EvidenceKind, ref string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: ref, Detail: "Exact durable audit evidence."}
}
func mustAuditJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := jsonMarshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
