package autonomous

import (
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
