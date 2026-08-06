package completion

import (
	"strings"
	"testing"

	"revolvr/internal/evidence"
)

func TestPreflightAcceptsExactSyntheticEvidence(t *testing.T) {
	snapshot := validSnapshot(t)
	first, err := BuildPreflight(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPreflight(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Accepted() || first.SHA256 != second.SHA256 || !sha256Pattern.MatchString(first.SHA256) {
		t.Fatalf("preflight = %#v, second hash = %q", first, second.SHA256)
	}
	t.Logf("deterministic preflight sha256: %s", first.SHA256)
	if first.Snapshot.Trajectory.State != evidence.TrajectoryInactive || first.Snapshot.HarnessAssets.State != evidence.HarnessAssetsInactive || first.Snapshot.HarnessAssets.ManifestSHA256 == "" {
		t.Fatalf("direct-tools provenance = %#v / %#v", first.Snapshot.Trajectory, first.Snapshot.HarnessAssets)
	}
}

func TestPreflightRejectsFalseCompletionMatrix(t *testing.T) {
	tests := []struct {
		name   string
		reason Reason
		mutate func(*Snapshot)
	}{
		{"task authority changed", ReasonTaskAuthorityChanged, func(s *Snapshot) { s.TaskStatus = "auditing" }},
		{"plan missing", ReasonPlanMissing, func(s *Snapshot) { s.Plan = nil }},
		{"plan step nonterminal", ReasonPlanStepNonterminal, func(s *Snapshot) { s.Plan.Steps[0].Status = "pending" }},
		{"criterion nonterminal", ReasonCriterionNonterminal, func(s *Snapshot) { s.Criteria[0].Status = "pending" }},
		{"criterion missing", ReasonCriterionNonterminal, func(s *Snapshot) { s.Criteria = nil }},
		{"criterion unsatisfied", ReasonCriterionUnsatisfied, func(s *Snapshot) { s.Criteria[0].Status = "failed" }},
		{"verification stale", ReasonVerificationStale, func(s *Snapshot) { s.Verification.CompletedAt = s.Source.FrozenAt.Add(-1) }},
		{"verification failed", ReasonVerificationFailed, func(s *Snapshot) { s.Verification.Status = "failed" }},
		{"verification reused final", ReasonVerificationStale, func(s *Snapshot) {
			s.Verification.Checks[0].Outcome = "passed_reused"
			s.Verification.Checks[0].ReusedFromCheckID = "old"
		}},
		{"audit missing", ReasonAuditMissing, func(s *Snapshot) { s.Audit = nil }},
		{"audit stale", ReasonAuditStale, func(s *Snapshot) { s.Audit.SourceTree = strings.Repeat("0", 40) }},
		{"audit changes required", ReasonAuditChangesRequired, func(s *Snapshot) { s.Audit.Disposition = "changes_required" }},
		{"blocking finding", ReasonBlockingFindingOpen, func(s *Snapshot) { s.Findings = []Finding{{ID: "finding", Significance: "blocking", Status: "open"}} }},
		{"source changed", ReasonSourceRevisionChanged, func(s *Snapshot) { s.Workspace.CandidateTree = strings.Repeat("0", 40) }},
		{"budget invalid", ReasonBudgetInvalid, func(s *Snapshot) { s.Budget.Consumed = s.Budget.Limit + 1 }},
		{"workspace unreconciled", ReasonWorkspaceUnreconciled, func(s *Snapshot) { s.Workspace.Reconciled = false }},
		{"artifact incomplete", ReasonArtifactManifestIncomplete, func(s *Snapshot) { s.Artifacts[0].Resolved = false }},
		{"commit invalid", ReasonCommitInvalid, func(s *Snapshot) { s.Source.AfterCommit = "bad" }},
		{"lease unreconciled", ReasonLeaseUnreconciled, func(s *Snapshot) { s.Lease.Held = false }},
		{"prompt model missing", ReasonPromptModelAuthorityMissing, func(s *Snapshot) { s.Invocations = nil }},
		{"trajectory missing", ReasonTrajectoryProvenanceInvalid, func(s *Snapshot) { s.Trajectory.Used = true }},
		{"harness missing", ReasonHarnessAssetsInvalid, func(s *Snapshot) {
			s.HarnessAssets.Used = true
			s.HarnessAssets.ManifestSHA256, _ = s.HarnessAssets.MaterialHash()
		}},
		{"operator input invalid", ReasonOperatorInputInvalid, func(s *Snapshot) {
			s.OperatorInputs = []OperatorInput{{ID: "answer", Version: 1, SHA256: strings.Repeat("1", 64)}}
		}},
		{"claim evidence invalid", ReasonClaimEvidenceInvalid, func(s *Snapshot) { s.Claims[0].Evidence[0].SHA256 = strings.Repeat("0", 64) }},
		{"claim missing", ReasonClaimEvidenceInvalid, func(s *Snapshot) { s.Claims = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := validSnapshot(t)
			test.mutate(&snapshot)
			preflight, err := BuildPreflight(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if preflight.Accepted() || !hasReason(preflight, test.reason) {
				t.Fatalf("rejections = %#v, want %q", preflight.Rejections, test.reason)
			}
		})
	}
}

func TestPreflightHashBindsTrajectoryHarnessAndAggregateVersions(t *testing.T) {
	baseline, err := BuildPreflight(validSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []func(*Snapshot){
		func(s *Snapshot) { s.Aggregates.Task++ },
		func(s *Snapshot) { s.Trajectory.State = "changed" },
		func(s *Snapshot) { s.HarnessAssets.ManifestSHA256 = strings.Repeat("0", 64) },
	}
	for _, mutate := range tests {
		snapshot := validSnapshot(t)
		mutate(&snapshot)
		changed, err := BuildPreflight(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if changed.SHA256 == baseline.SHA256 {
			t.Fatal("material authority change did not alter preflight hash")
		}
	}
}
