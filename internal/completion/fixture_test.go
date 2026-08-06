package completion

import (
	"strings"
	"testing"
	"time"

	"revolvr/internal/evidence"
)

func validSnapshot(t *testing.T) Snapshot {
	t.Helper()
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	identity := Identity{
		ProjectID: "00000000-0000-0000-0000-000000000001", TaskID: "00000000-0000-0000-0000-000000000002",
		TaskVersionID: "00000000-0000-0000-0000-000000000003", RunID: "00000000-0000-0000-0000-000000000004",
		WorkspaceID: "00000000-0000-0000-0000-000000000005",
	}
	commit, tree := strings.Repeat("a", 40), strings.Repeat("b", 40)
	provenance := evidence.Provenance{
		SchemaVersion: evidence.ArtifactProvenanceSchemaVersion,
		ProjectID:     identity.ProjectID, TaskID: identity.TaskID, TaskVersionID: identity.TaskVersionID,
		RunID: identity.RunID, WorkspaceID: identity.WorkspaceID, ProducerRole: "host",
		ProducingOperationID: "fixture-source", SourceCommit: commit, SourceTree: tree,
	}
	artifactKinds := []string{"task_source", "plan_source", "diff", "verification_report", "audit_report"}
	artifacts := make([]evidence.ArtifactReference, 0, len(artifactKinds))
	for index, kind := range artifactKinds {
		artifacts = append(artifacts, evidence.ArtifactReference{
			ID: "00000000-0000-0000-0000-00000000010" + string(rune('1'+index)), Kind: kind, MediaType: "application/octet-stream",
			SHA256: strings.Repeat(string(rune('1'+index)), 64), SizeBytes: int64(index + 1),
			StoragePath: "fixture/" + kind, Resolved: true, Required: true, Provenance: provenance,
		})
	}
	verificationID := "00000000-0000-0000-0000-000000000201"
	checkID := "00000000-0000-0000-0000-000000000202"
	criterionID := "00000000-0000-0000-0000-000000000301"
	claim, err := evidence.NewClaim("00000000-0000-0000-0000-000000000401", criterionID, "acceptance", "The accepted behavior is verified.", []evidence.EvidenceLink{{
		Kind: "verification_check", ID: checkID, SHA256: strings.Repeat("9", 64), Resolved: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	budget := Budget{SchemaVersion: "revolvr-budget-v1", Limit: 100, Consumed: 20}
	budget.SHA256, err = evidence.Hash(budget)
	if err != nil {
		t.Fatal(err)
	}
	manifestHash, err := evidence.ArtifactManifestHash(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	return Snapshot{
		SchemaVersion: SnapshotSchemaVersion, Identity: identity, TaskStatus: "finalizing", RunStatus: "active",
		Aggregates: Aggregates{Task: 7, Run: 3, Workspace: 5, Plan: 2, Lease: 1},
		Source: Source{
			BeforeCommit: strings.Repeat("c", 40), BeforeTree: strings.Repeat("d", 40),
			AfterCommit: commit, AfterTree: tree, DiffSHA256: strings.Repeat("e", 64), FrozenAt: now,
		},
		Plan:     &Plan{ID: "00000000-0000-0000-0000-000000000501", VersionID: "00000000-0000-0000-0000-000000000502", SHA256: strings.Repeat("f", 64), Steps: []PlanStep{{ID: "implement", Status: "completed"}}},
		Criteria: []Criterion{{ID: criterionID, Status: "passed", VerificationCheckID: checkID}},
		Verification: &Verification{
			ID: verificationID, Purpose: "final", Status: "passed", SourceCommit: commit, SourceTree: tree,
			ImageDigest: "sha256:" + strings.Repeat("1", 64), Profile: "strict", ProfileSHA256: strings.Repeat("2", 64),
			CompletedAt: now.Add(time.Minute), Checks: []VerificationCheck{{
				ID: checkID, Tier: 4, Outcome: "passed", ExecutionFingerprint: strings.Repeat("9", 64),
				ImageDigest: "sha256:" + strings.Repeat("1", 64), Profile: "strict", ProfileSHA256: strings.Repeat("2", 64),
			}},
		},
		Audit: &Audit{
			SchemaVersion: AuditSchemaVersion, ID: "00000000-0000-0000-0000-000000000601", RunID: identity.RunID, Role: "auditor",
			Independent: true, Disposition: "clean", SourceCommit: commit, SourceTree: tree,
			ReportArtifactID: artifacts[4].ID, ReportSHA256: artifacts[4].SHA256, CompletedAt: now.Add(2 * time.Minute),
		},
		Findings: []Finding{}, Budget: budget,
		Workspace: Workspace{Status: "frozen", Reconciled: true, CandidateCommit: commit, CandidateTree: tree, DiffSHA256: strings.Repeat("e", 64)},
		Lease:     Lease{Name: "global-source-mutation-v1", RunID: identity.RunID, Held: true},
		Invocations: []Invocation{{
			Role: "implementer", Model: "gpt-fixture", PromptVersion: "implementer-v1",
			PromptSHA256: strings.Repeat("3", 64), DossierSHA256: strings.Repeat("4", 64),
			ImageDigest: "sha256:" + strings.Repeat("5", 64), Profile: "strict",
		}},
		Artifacts: artifacts, ArtifactManifestSHA256: manifestHash, OperatorInputs: []OperatorInput{},
		Trajectory: evidence.DirectToolsTrajectoryEnvelope(), HarnessAssets: evidence.DirectToolsHarnessAssetSet(),
		Claims: []evidence.Claim{claim},
	}
}

func hasReason(preflight Preflight, reason Reason) bool {
	for _, rejection := range preflight.Rejections {
		if rejection.Reason == reason {
			return true
		}
	}
	return false
}
