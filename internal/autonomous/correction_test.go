package autonomous

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCorrectionOutputExactAuthorityAndPartialRepair(t *testing.T) {
	output := CorrectionOutput{SchemaVersion: CorrectionOutputSchemaVersion, TaskID: "task-1", WorkerRunID: "corrector-run", DecisionID: "decision-correct", FindingIDs: []string{"finding-one", "finding-two"}, Outcome: CorrectionOutcomePartial, ResolvedFindingIDs: []string{"finding-one"}, RemainingFindingIDs: []string{"finding-two"}, Evidence: []EvidenceReference{{Kind: EvidenceKindFile, Reference: "internal/fix.go", Detail: "New correction evidence."}}}
	raw, err := MarshalCorrectionOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCorrectionOutput(raw)
	if err != nil || !reflect.DeepEqual(parsed, output) {
		t.Fatalf("parsed=%+v err=%v", parsed, err)
	}
	outside := output
	outside.ResolvedFindingIDs = []string{"finding-three"}
	if err := outside.Validate(); err == nil || (!strings.Contains(err.Error(), "outside correction authority") && !strings.Contains(err.Error(), "partitioned exactly once")) {
		t.Fatalf("outside error=%v", err)
	}
}

func TestCorrectionOutputVerificationRepairCannotClaimFindings(t *testing.T) {
	target := VerificationFailureTarget{TaskID: "task-1", RunID: "verify-run", OccurrenceID: "occurrence-one", SourceRevision: strings.Repeat("a", 64), Status: VerificationStatusFailed, Evidence: []EvidenceReference{{Kind: EvidenceKindVerification, Reference: "verification.json", Detail: "Exact failure."}}}
	output := CorrectionOutput{SchemaVersion: CorrectionOutputSchemaVersion, TaskID: "task-1", WorkerRunID: "corrector-run", DecisionID: "decision-correct", VerificationFailure: &target, Outcome: CorrectionOutcomeCorrected, VerificationFailureAddressed: true, Evidence: []EvidenceReference{{Kind: EvidenceKindFile, Reference: "internal/fix.go", Detail: "New repair evidence."}}}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
	output.ResolvedFindingIDs = []string{"unrelated"}
	if err := output.Validate(); err == nil || !strings.Contains(err.Error(), "cannot claim audit findings") {
		t.Fatalf("claim error=%v", err)
	}
}

func TestParseCorrectionOutputRequiredNullAndEmptyAuthorityRepresentations(t *testing.T) {
	evidence := []any{map[string]any{"kind": "file", "reference": "internal/fix.go", "detail": "New correction evidence."}}
	base := func() map[string]any {
		return map[string]any{
			"schema_version": CorrectionOutputSchemaVersion, "task_id": "task-1", "worker_run_id": "corrector-run", "decision_id": "decision-correct",
			"finding_ids": []any{}, "verification_failure": nil, "outcome": "corrected", "resolved_finding_ids": []any{}, "remaining_finding_ids": []any{},
			"verification_failure_addressed": false, "evidence": evidence,
		}
	}

	t.Run("audit finding", func(t *testing.T) {
		wire := base()
		wire["finding_ids"] = []any{"finding-one"}
		wire["resolved_finding_ids"] = []any{"finding-one"}
		parsed := parseCorrectionWire(t, wire)
		if parsed.VerificationFailure != nil || !reflect.DeepEqual(parsed.FindingIDs, []string{"finding-one"}) || len(parsed.RemainingFindingIDs) != 0 {
			t.Fatalf("decoded audit-finding correction = %+v", parsed)
		}
	})

	t.Run("verification failure", func(t *testing.T) {
		wire := base()
		wire["verification_failure"] = map[string]any{
			"task_id": "task-1", "run_id": "verify-run", "occurrence_id": "occurrence-one", "source_revision": strings.Repeat("a", 64), "status": "failed",
			"evidence": []any{map[string]any{"kind": "verification", "reference": "verification.json", "detail": "Exact failure."}},
		}
		wire["verification_failure_addressed"] = true
		parsed := parseCorrectionWire(t, wire)
		if parsed.VerificationFailure == nil || len(parsed.FindingIDs) != 0 || len(parsed.ResolvedFindingIDs) != 0 || len(parsed.RemainingFindingIDs) != 0 || !parsed.VerificationFailureAddressed {
			t.Fatalf("decoded verification-failure correction = %+v", parsed)
		}
	})

	t.Run("exclusive authority remains enforced", func(t *testing.T) {
		wire := base()
		wire["finding_ids"] = []any{"finding-one"}
		wire["resolved_finding_ids"] = []any{"finding-one"}
		wire["verification_failure"] = map[string]any{
			"task_id": "task-1", "run_id": "verify-run", "occurrence_id": "occurrence-one", "source_revision": strings.Repeat("a", 64), "status": "failed",
			"evidence": []any{map[string]any{"kind": "verification", "reference": "verification.json", "detail": "Exact failure."}},
		}
		raw, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCorrectionOutput(raw); err == nil || !strings.Contains(err.Error(), "exactly one authority kind") {
			t.Fatalf("ParseCorrectionOutput() error = %v, want exclusive authority rejection", err)
		}
	})
}

func TestParseCorrectionOutputRejectsDuplicateFindingIdentities(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CorrectionOutput)
	}{
		{name: "finding_ids", mutate: func(output *CorrectionOutput) { output.FindingIDs = []string{"finding-one", "finding-one"} }},
		{name: "resolved_finding_ids", mutate: func(output *CorrectionOutput) { output.ResolvedFindingIDs = []string{"finding-one", "finding-one"} }},
		{name: "remaining_finding_ids", mutate: func(output *CorrectionOutput) { output.RemainingFindingIDs = []string{"finding-two", "finding-two"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := CorrectionOutput{
				SchemaVersion: CorrectionOutputSchemaVersion, TaskID: "task-1", WorkerRunID: "corrector-run", DecisionID: "decision-correct",
				FindingIDs: []string{"finding-one", "finding-two"}, Outcome: CorrectionOutcomePartial,
				ResolvedFindingIDs: []string{"finding-one"}, RemainingFindingIDs: []string{"finding-two"},
				Evidence: []EvidenceReference{{Kind: EvidenceKindFile, Reference: "internal/fix.go", Detail: "New correction evidence."}},
			}
			tt.mutate(&output)
			raw, err := json.Marshal(output)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseCorrectionOutput(raw); err == nil || !strings.Contains(err.Error(), "duplicate finding id") {
				t.Fatalf("ParseCorrectionOutput() error = %v, want duplicate finding identity rejection", err)
			}
		})
	}
}

func parseCorrectionWire(t *testing.T, wire map[string]any) CorrectionOutput {
	t.Helper()
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCorrectionOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
