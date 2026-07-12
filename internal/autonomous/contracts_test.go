package autonomous

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSupervisorDecisionValidateSupportsEveryAction(t *testing.T) {
	tests := []struct {
		name       string
		action     Action
		profile    WorkerProfile
		criteria   []string
		findingIDs []string
	}{
		{name: "plan", action: ActionPlan, profile: WorkerProfilePlanner, criteria: []string{"A durable implementation plan is recorded."}},
		{name: "implement", action: ActionImplement, profile: WorkerProfileImplementer, criteria: []string{"The requested behavior is implemented."}},
		{name: "audit", action: ActionAudit, profile: WorkerProfileAuditor, criteria: []string{"An independent audit report is recorded."}},
		{name: "correct", action: ActionCorrect, profile: WorkerProfileCorrector, criteria: []string{"The referenced finding is resolved."}, findingIDs: []string{"finding-001"}},
		{name: "document", action: ActionDocument, profile: WorkerProfileDocumentor, criteria: []string{"Affected operator guidance is accurate."}},
		{name: "simplify", action: ActionSimplify, profile: WorkerProfileSimplifier, criteria: []string{"Unnecessary complexity is removed without behavior changes."}},
		{name: "complete", action: ActionComplete},
		{name: "block", action: ActionBlock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := validDecision(tt.action, tt.profile)
			decision.SuccessCriteria = tt.criteria
			decision.FindingIDs = tt.findingIDs
			if err := decision.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestSupervisorDecisionValidateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SupervisorDecision)
		wantErr string
	}{
		{
			name: "missing task identity",
			mutate: func(decision *SupervisorDecision) {
				decision.TaskID = " "
			},
			wantErr: "task_id is required",
		},
		{
			name: "unknown action",
			mutate: func(decision *SupervisorDecision) {
				decision.Action = "review"
			},
			wantErr: `unknown action "review"`,
		},
		{
			name: "missing rationale",
			mutate: func(decision *SupervisorDecision) {
				decision.Rationale = "\t"
			},
			wantErr: "rationale is required",
		},
		{
			name: "missing worker profile",
			mutate: func(decision *SupervisorDecision) {
				decision.WorkerProfile = ""
			},
			wantErr: `requires compatible worker_profile "implementer"`,
		},
		{
			name: "incompatible worker profile",
			mutate: func(decision *SupervisorDecision) {
				decision.WorkerProfile = WorkerProfileAuditor
			},
			wantErr: `requires compatible worker_profile "implementer"`,
		},
		{
			name: "missing success criteria",
			mutate: func(decision *SupervisorDecision) {
				decision.SuccessCriteria = nil
			},
			wantErr: "worker action requires at least one success criterion",
		},
		{
			name: "empty success criterion",
			mutate: func(decision *SupervisorDecision) {
				decision.SuccessCriteria = []string{" "}
			},
			wantErr: "success_criteria[0] is empty",
		},
		{
			name: "missing inputs",
			mutate: func(decision *SupervisorDecision) {
				decision.Inputs = nil
			},
			wantErr: "inputs requires at least one evidence reference",
		},
		{
			name: "unknown evidence kind",
			mutate: func(decision *SupervisorDecision) {
				decision.Inputs[0].Kind = "chat"
			},
			wantErr: `inputs[0]: unknown kind "chat"`,
		},
		{
			name: "missing evidence reference",
			mutate: func(decision *SupervisorDecision) {
				decision.Inputs[0].Reference = ""
			},
			wantErr: "inputs[0]: reference is required",
		},
		{
			name: "missing evidence detail",
			mutate: func(decision *SupervisorDecision) {
				decision.Inputs[0].Detail = " "
			},
			wantErr: "inputs[0]: detail is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := validDecision(ActionImplement, WorkerProfileImplementer)
			tt.mutate(&decision)
			assertErrorContains(t, decision.Validate(), tt.wantErr)
		})
	}
}

func TestSupervisorDecisionValidateRejectsInvalidTerminalProfiles(t *testing.T) {
	for _, action := range []Action{ActionComplete, ActionBlock} {
		t.Run(string(action), func(t *testing.T) {
			decision := validDecision(action, WorkerProfileImplementer)
			decision.SuccessCriteria = nil
			assertErrorContains(t, decision.Validate(), `terminal action "`+string(action)+`" must not select worker_profile`)
		})
	}
}

func TestSupervisorDecisionValidateCorrectionFindingIDs(t *testing.T) {
	tests := []struct {
		name       string
		findingIDs []string
		wantErr    string
	}{
		{name: "missing", wantErr: "correct action requires at least one finding_id"},
		{name: "malformed", findingIDs: []string{"Finding_1"}, wantErr: `invalid finding id "Finding_1"`},
		{name: "duplicate", findingIDs: []string{"finding-1", "finding-1"}, wantErr: `duplicate finding id "finding-1"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := validDecision(ActionCorrect, WorkerProfileCorrector)
			decision.FindingIDs = tt.findingIDs
			assertErrorContains(t, decision.Validate(), tt.wantErr)
		})
	}

	decision := validDecision(ActionImplement, WorkerProfileImplementer)
	decision.FindingIDs = []string{"finding-1"}
	assertErrorContains(t, decision.Validate(), `finding_ids are only valid for action "correct"`)
}

func TestSupervisorDecisionVerificationFailureAuthority(t *testing.T) {
	target := VerificationFailureTarget{TaskID: "task-1", RunID: "verify-run", OccurrenceID: "verify-occurrence", SourceRevision: strings.Repeat("a", 64), Status: VerificationStatusFailed, Evidence: []EvidenceReference{{Kind: EvidenceKindVerification, Reference: ".revolvr/runs/verify-run/verification.json", Detail: "Exact failed occurrence."}}}
	decision := validDecision(ActionCorrect, WorkerProfileCorrector)
	decision.VerificationFailure = &target
	if err := ValidateVerificationCorrectionDecision(decision, target); err != nil {
		t.Fatal(err)
	}
	wrong := target
	wrong.OccurrenceID = "other"
	if err := ValidateVerificationCorrectionDecision(decision, wrong); err == nil || !strings.Contains(err.Error(), "does not exactly match") {
		t.Fatalf("wrong target error=%v", err)
	}
	decision.FindingIDs = []string{"finding-one"}
	if err := decision.Validate(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("mixed authority error=%v", err)
	}
}

func TestAuditReportValidateSupportsEveryDisposition(t *testing.T) {
	tests := []struct {
		name        string
		disposition AuditDisposition
		findings    []AuditFinding
	}{
		{name: "clean", disposition: AuditDispositionClean},
		{name: "changes required", disposition: AuditDispositionChangesRequired, findings: []AuditFinding{validFinding("finding-001")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := validAuditReport(tt.disposition, tt.findings...)
			if err := report.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestAuditReportValidateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AuditReport)
		wantErr string
	}{
		{
			name: "missing task identity",
			mutate: func(report *AuditReport) {
				report.TaskID = ""
			},
			wantErr: "task_id is required",
		},
		{
			name: "unknown disposition",
			mutate: func(report *AuditReport) {
				report.Disposition = "concerns"
			},
			wantErr: `unknown disposition "concerns"`,
		},
		{
			name: "missing rationale",
			mutate: func(report *AuditReport) {
				report.Rationale = " "
			},
			wantErr: "rationale is required",
		},
		{
			name: "missing inputs",
			mutate: func(report *AuditReport) {
				report.Inputs = nil
			},
			wantErr: "inputs requires at least one evidence reference",
		},
		{
			name: "changes required without findings",
			mutate: func(report *AuditReport) {
				report.Findings = nil
			},
			wantErr: "changes_required disposition requires at least one finding",
		},
		{
			name: "clean with findings",
			mutate: func(report *AuditReport) {
				report.Disposition = AuditDispositionClean
			},
			wantErr: "clean disposition must not include findings",
		},
		{
			name: "duplicate finding ids",
			mutate: func(report *AuditReport) {
				report.Findings = append(report.Findings, validFinding("finding-001"))
			},
			wantErr: `duplicate finding id "finding-001"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := validAuditReport(AuditDispositionChangesRequired, validFinding("finding-001"))
			tt.mutate(&report)
			assertErrorContains(t, report.Validate(), tt.wantErr)
		})
	}
}

func TestAuditFindingValidate(t *testing.T) {
	for _, significance := range []FindingSignificance{FindingSignificanceBlocking, FindingSignificanceNonBlocking} {
		t.Run(string(significance), func(t *testing.T) {
			finding := validFinding("finding-001")
			finding.Significance = significance
			if err := finding.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	tests := []struct {
		name    string
		mutate  func(*AuditFinding)
		wantErr string
	}{
		{name: "empty id", mutate: func(finding *AuditFinding) { finding.ID = "" }, wantErr: "invalid finding id"},
		{name: "upper-case id", mutate: func(finding *AuditFinding) { finding.ID = "Finding-1" }, wantErr: "invalid finding id"},
		{name: "underscore id", mutate: func(finding *AuditFinding) { finding.ID = "finding_1" }, wantErr: "invalid finding id"},
		{name: "repeated hyphen", mutate: func(finding *AuditFinding) { finding.ID = "finding--1" }, wantErr: "invalid finding id"},
		{name: "trailing hyphen", mutate: func(finding *AuditFinding) { finding.ID = "finding-" }, wantErr: "invalid finding id"},
		{name: "missing significance", mutate: func(finding *AuditFinding) { finding.Significance = "" }, wantErr: `unknown significance ""`},
		{name: "unknown significance", mutate: func(finding *AuditFinding) { finding.Significance = "critical" }, wantErr: `unknown significance "critical"`},
		{name: "missing summary", mutate: func(finding *AuditFinding) { finding.Summary = " " }, wantErr: "summary is required"},
		{name: "missing evidence", mutate: func(finding *AuditFinding) { finding.Evidence = nil }, wantErr: "evidence requires at least one evidence reference"},
		{name: "evidence without reference", mutate: func(finding *AuditFinding) { finding.Evidence[0].Reference = "" }, wantErr: "evidence[0]: reference is required"},
		{name: "evidence without detail", mutate: func(finding *AuditFinding) { finding.Evidence[0].Detail = "" }, wantErr: "evidence[0]: detail is required"},
		{name: "missing correction", mutate: func(finding *AuditFinding) { finding.RequiredCorrection = "\n" }, wantErr: "required_correction is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := validFinding("finding-001")
			tt.mutate(&finding)
			assertErrorContains(t, finding.Validate(), tt.wantErr)
		})
	}
}

func TestEvidenceReferenceKindsAreValid(t *testing.T) {
	kinds := []EvidenceKind{
		EvidenceKindTask,
		EvidenceKindPlan,
		EvidenceKindLedger,
		EvidenceKindReceipt,
		EvidenceKindVerification,
		EvidenceKindGit,
		EvidenceKindAudit,
		EvidenceKindRepository,
		EvidenceKindFile,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			decision := validDecision(ActionImplement, WorkerProfileImplementer)
			decision.Inputs[0].Kind = kind
			if err := decision.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateCorrectionDecisionReferencesAuditFindings(t *testing.T) {
	report := validAuditReport(
		AuditDispositionChangesRequired,
		validFinding("finding-001"),
		validFinding("finding-002"),
	)
	decision := validDecision(ActionCorrect, WorkerProfileCorrector)
	decision.FindingIDs = []string{"finding-002"}

	if err := ValidateCorrectionDecision(decision, report); err != nil {
		t.Fatalf("ValidateCorrectionDecision() error = %v", err)
	}

	t.Run("unknown finding", func(t *testing.T) {
		invalid := decision
		invalid.FindingIDs = []string{"finding-003"}
		assertErrorContains(t, ValidateCorrectionDecision(invalid, report), `finding_id "finding-003" does not reference an audit finding`)
	})

	t.Run("task mismatch", func(t *testing.T) {
		invalid := decision
		invalid.TaskID = "task-2"
		assertErrorContains(t, ValidateCorrectionDecision(invalid, report), "does not match audit task_id")
	})

	t.Run("non-correction action", func(t *testing.T) {
		invalid := validDecision(ActionImplement, WorkerProfileImplementer)
		assertErrorContains(t, ValidateCorrectionDecision(invalid, report), `action must be "correct"`)
	})

	t.Run("clean audit", func(t *testing.T) {
		clean := validAuditReport(AuditDispositionClean)
		assertErrorContains(t, ValidateCorrectionDecision(decision, clean), `audit disposition must be "changes_required"`)
	})
}

func TestContractsJSONRoundTrip(t *testing.T) {
	t.Run("supervisor decision", func(t *testing.T) {
		want := validDecision(ActionCorrect, WorkerProfileCorrector)
		want.FindingIDs = []string{"finding-001"}
		assertJSONRoundTrip(t, want, func(got SupervisorDecision) error { return got.Validate() })
	})

	t.Run("audit report", func(t *testing.T) {
		want := validAuditReport(AuditDispositionChangesRequired, validFinding("finding-001"))
		assertJSONRoundTrip(t, want, func(got AuditReport) error { return got.Validate() })
	})
}

func validDecision(action Action, profile WorkerProfile) SupervisorDecision {
	return SupervisorDecision{
		TaskID:          "task-1",
		Action:          action,
		WorkerProfile:   profile,
		Rationale:       "Repository evidence identifies this as the next legal action.",
		SuccessCriteria: []string{"The requested outcome has durable evidence."},
		Inputs: []EvidenceReference{
			{
				Kind:      EvidenceKindTask,
				Reference: ".agent/tasks/task-1.md",
				Detail:    "The task remains pending with unmet acceptance criteria.",
			},
		},
	}
}

func validAuditReport(disposition AuditDisposition, findings ...AuditFinding) AuditReport {
	return AuditReport{
		TaskID:      "task-1",
		Disposition: disposition,
		Rationale:   "The independent review reached this disposition from durable evidence.",
		Inputs: []EvidenceReference{
			{
				Kind:      EvidenceKindVerification,
				Reference: "run-1:verification_completed",
				Detail:    "The recorded verification command passed.",
			},
		},
		Findings: findings,
	}
}

func validFinding(id string) AuditFinding {
	return AuditFinding{
		ID:           id,
		Significance: FindingSignificanceBlocking,
		Summary:      "A required behavior is not covered.",
		Evidence: []EvidenceReference{
			{
				Kind:      EvidenceKindFile,
				Reference: "internal/example/example.go:42",
				Detail:    "The branch returns before applying the required update.",
			},
		},
		RequiredCorrection: "Apply the update before returning and add regression coverage.",
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want substring %q", err, want)
	}
}

func assertJSONRoundTrip[T any](t *testing.T, want T, validate func(T) error) {
	t.Helper()
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	repeated, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("second json.Marshal() error = %v", err)
	}
	if string(raw) != string(repeated) {
		t.Fatalf("JSON is not deterministic:\nfirst:  %s\nsecond: %s", raw, repeated)
	}

	var got T
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
	if err := validate(got); err != nil {
		t.Fatalf("round-trip validation error = %v", err)
	}
}
