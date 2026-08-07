package evaluation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"revolvr/internal/audit"
	"revolvr/internal/completion"
	"revolvr/internal/correction"
	"revolvr/internal/evidence"
	"revolvr/internal/sandbox"
	"revolvr/internal/verification"
)

func TestArchitecture017VerificationAuthorityAndExactReuse(t *testing.T) {
	pinned := evaluationPinnedPlan(t, verification.TierFocused)
	store := &evaluationVerificationStore{reusable: map[string]verification.ReusableCheck{}}
	executor := &evaluationGateExecutor{exitCode: 1}
	engine := evaluationVerificationEngine(t, pinned, store, executor)
	first, err := engine.Run(context.Background(), verification.Request{Pinned: pinned, Purpose: verification.PurposeCandidate})
	if err != nil {
		t.Fatal(err)
	}
	if executor.calls != 1 || len(first.Checks) != 1 || first.Checks[0].Outcome != verification.OutcomeFailed {
		t.Fatalf("initial verification = %+v calls=%d", first, executor.calls)
	}
	second, err := engine.Run(context.Background(), verification.Request{Pinned: pinned, Purpose: verification.PurposeCandidate})
	if err != nil {
		t.Fatal(err)
	}
	if executor.calls != 1 || second.Checks[0].Outcome != verification.OutcomeUnchangedFailureReused || second.Checks[0].ReusedFromCheckID == "" {
		t.Fatalf("exact failure reuse = %+v calls=%d", second, executor.calls)
	}

	finalPinned := evaluationPinnedPlan(t, verification.TierFinal)
	finalFingerprint, err := verification.ExecutionFingerprint(finalPinned, finalPinned.Plan.Gates[0])
	if err != nil {
		t.Fatal(err)
	}
	finalStore := &evaluationVerificationStore{reusable: map[string]verification.ReusableCheck{finalFingerprint: evaluationReusable(verification.OutcomePassed)}}
	finalExecutor := &evaluationGateExecutor{exitCode: 0}
	finalEngine := evaluationVerificationEngine(t, finalPinned, finalStore, finalExecutor)
	finalResult, err := finalEngine.Run(context.Background(), verification.Request{Pinned: finalPinned, Purpose: verification.PurposeFinal})
	if err != nil {
		t.Fatal(err)
	}
	if finalExecutor.calls != 1 || finalResult.Checks[0].Outcome != verification.OutcomePassed || finalResult.Checks[0].ReusedFromCheckID != "" {
		t.Fatalf("fresh final verification = %+v calls=%d", finalResult, finalExecutor.calls)
	}
}

func TestArchitecture018CompletionAuthorityAcceptsOnlyCompleteEvidence(t *testing.T) {
	snapshot := evaluationCompletionSnapshot(t)
	accepted, err := completion.BuildPreflight(snapshot)
	if err != nil || !accepted.Accepted() {
		t.Fatalf("accepted completion preflight = %+v err=%v", accepted, err)
	}
	if accepted.Snapshot.Trajectory.RuntimeKind != evidence.DirectToolsRuntimeKind || accepted.Snapshot.Trajectory.Used || accepted.Snapshot.HarnessAssets.RuntimeKind != evidence.DirectToolsRuntimeKind || accepted.Snapshot.HarnessAssets.Used {
		t.Fatalf("direct-tools trajectory/harness authority = %+v / %+v", accepted.Snapshot.Trajectory, accepted.Snapshot.HarnessAssets)
	}

	falseCompletion := snapshot
	falseCompletion.Criteria[0].Status = "pending"
	falseCompletion.Verification.Checks[0].Outcome = string(verification.OutcomePassedReused)
	falseCompletion.Verification.Checks[0].ReusedFromCheckID = "prior-check"
	falseCompletion.Audit.Disposition = string(audit.DispositionChangesRequired)
	falseCompletion.Findings = []completion.Finding{{ID: "finding-1", Significance: "blocking", Status: "open"}}
	rejected, err := completion.BuildPreflight(falseCompletion)
	if err != nil || rejected.Accepted() {
		t.Fatalf("false completion preflight = %+v err=%v", rejected, err)
	}
	want := map[completion.Reason]bool{
		completion.ReasonCriterionNonterminal: false,
		completion.ReasonVerificationStale:    false,
		completion.ReasonAuditChangesRequired: false,
		completion.ReasonBlockingFindingOpen:  false,
	}
	for _, rejection := range rejected.Rejections {
		if _, ok := want[rejection.Reason]; ok {
			want[rejection.Reason] = true
		}
	}
	for reason, found := range want {
		if !found {
			t.Fatalf("missing completion rejection %q in %+v", reason, rejected.Rejections)
		}
	}
}

func TestArchitecture019FailureAndStrategyAuthorityIsSemanticAndBounded(t *testing.T) {
	source := audit.Source{Revision: strings.Repeat("d", 64), Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40), DiffSHA256: strings.Repeat("c", 64)}
	failure, err := correction.NormalizeFailure(correction.Authority{
		Kind: correction.AuthorityVerification, Source: source,
		Verification: &correction.VerificationFailure{
			VerificationRunID: "verification-1", CheckID: "check-1", GateID: "go-test", Outcome: verification.OutcomeFailed,
			ExitCode: 1, FailedTestIDs: []string{"TestRun"}, StableExcerpts: []string{"expected ready"},
			Component: "fixture", AffectedFiles: []string{"result.txt"},
		},
	})
	if err != nil || failure.SHA256 == "" {
		t.Fatalf("failure signature = %+v err=%v", failure, err)
	}
	first := correction.Strategy{
		SchemaVersion: correction.StrategySchemaVersion, Approach: "Repair the exact fixture result.",
		Techniques: []string{"edit exact output", "run focused test"}, TargetFiles: []string{"result.txt"},
		TargetSymbols: []string{"Run"}, ExpectedEvidence: []string{"fresh final verification"},
	}
	second := correction.Strategy{
		SchemaVersion: correction.StrategySchemaVersion, Approach: "  repair   the exact fixture RESULT. ",
		Techniques: []string{"run focused test", "edit exact output"}, TargetFiles: []string{"result.txt"},
		TargetSymbols: []string{"run"}, ExpectedEvidence: []string{"FRESH final verification"},
	}
	firstFingerprint, _, err := correction.StrategyFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, _, err := correction.StrategyFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint != secondFingerprint {
		t.Fatalf("semantic strategy fingerprints differ: %s / %s", firstFingerprint, secondFingerprint)
	}
	if _, _, err := correction.StrategyFingerprint(correction.Strategy{SchemaVersion: correction.StrategySchemaVersion, Approach: "broaden scope"}); err == nil {
		t.Fatal("incomplete correction strategy was admitted")
	}
}

type evaluationVerificationStore struct {
	reusable map[string]verification.ReusableCheck
	runs     []verification.PersistedRun
}

func (s *evaluationVerificationStore) FindReusable(_ context.Context, fingerprint string) (verification.ReusableCheck, bool, error) {
	value, found := s.reusable[fingerprint]
	return value, found, nil
}

func (s *evaluationVerificationStore) Persist(_ context.Context, run verification.PersistedRun) error {
	s.runs = append(s.runs, run)
	for _, check := range run.Checks {
		if check.Outcome == verification.OutcomePassed || check.Outcome == verification.OutcomeFailed {
			s.reusable[check.ExecutionFingerprint] = verification.ReusableCheck{
				ID: check.ID, Outcome: check.Outcome, ExitCode: check.ExitCode, Stdout: check.Stdout, Stderr: check.Stderr,
				ParsedResult: check.ParsedResult, SandboxEvidence: check.SandboxEvidence,
				FailureSignatures: check.FailureSignatures, SandboxSpecificationSHA256: check.SandboxSpecificationSHA256,
				OriginalExecutedAt: check.OriginalExecutedAt,
			}
		}
	}
	return nil
}

type evaluationGateExecutor struct {
	exitCode int
	calls    int
}

func (e *evaluationGateExecutor) Execute(context.Context, verification.GateExecution) (verification.ExecutionResult, error) {
	e.calls++
	started := time.Date(2026, 8, 7, 12, 0, e.calls, 0, time.UTC)
	return verification.ExecutionResult{
		SandboxSpecificationSHA256: strings.Repeat("6", 64), ExitCode: e.exitCode,
		Stdout: []byte("fixture verification\n"), Evidence: json.RawMessage(`{"runtime":"deterministic-fake"}`),
		StartedAt: started, CompletedAt: started.Add(time.Second),
	}, nil
}

type evaluationAuthorityObserver struct {
	pinned verification.PinnedPlan
}

func (o evaluationAuthorityObserver) Observe(_ context.Context, gate verification.Gate) (verification.AuthoritySnapshot, error) {
	return verification.AuthoritySnapshot{Source: gate.Source, ProjectEnvironmentSHA256: o.pinned.ProjectEnvironment.SHA256, AuthorityInputs: append([]verification.MaterialInput(nil), gate.AuthorityInputs...)}, nil
}

type evaluationArtifactWriter struct {
	next int
}

func (a *evaluationArtifactWriter) Materialize(_ context.Context, kind, media string, raw []byte) (verification.Artifact, error) {
	a.next++
	return verification.Artifact{ID: "artifact-" + string(rune('0'+a.next)), SHA256: hashBytes(raw), SizeBytes: int64(len(raw)), MediaType: media, LogicalKind: kind, StoragePath: "fixture/artifact"}, nil
}

func evaluationVerificationEngine(t *testing.T, pinned verification.PinnedPlan, store verification.Store, executor verification.GateExecutor) *verification.Engine {
	t.Helper()
	now := time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	nextID := 0
	engine, err := verification.NewEngine(verification.EngineConfig{
		Store: store, Executor: executor, Artifacts: &evaluationArtifactWriter{}, Observer: evaluationAuthorityObserver{pinned: pinned},
		Clock: func() time.Time { now = now.Add(time.Second); return now },
		NewID: func() string {
			nextID++
			return "00000000-0000-5000-8000-" + strings.Repeat("0", 11) + string(rune('0'+nextID))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func evaluationPinnedPlan(t *testing.T, tier verification.Tier) verification.PinnedPlan {
	t.Helper()
	source := verification.SourceIdentity{Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40)}
	pinned, err := verification.Pin(verification.PinnedPlan{
		Plan: verification.Plan{
			SchemaVersion: verification.PlanSchemaVersion, Version: "plan-v1", VerificationPlanVersion: "evaluation-v1",
			VerificationPlanSHA256: strings.Repeat("c", 64), AuthorityChangePolicy: verification.AuthorityReject,
			AllowReuse: true, AllowMissingBaseline: true, RequireFreshFinal: tier == verification.TierFinal,
			Gates: []verification.Gate{{
				ID: "evaluation", Tier: tier, Source: source, Argv: []string{"go", "test", "./..."}, WorkingDirectory: "/workspace",
				Image:          sandbox.Image{Reference: "example.invalid/revolvr/verifier", Digest: "sha256:" + strings.Repeat("d", 64)},
				SandboxProfile: sandbox.ProfileStrict, SandboxProfileSHA256: strings.Repeat("e", 64),
				Resources:       sandbox.Resources{CPUs: 1, MemoryBytes: 1 << 30, PIDs: 128, TimeoutSeconds: 60, TmpfsBytes: 64 << 20},
				Parser:          verification.Parser{Kind: verification.ParserNone, Version: verification.DefaultStructuredParserVersion},
				AuthorityInputs: []verification.MaterialInput{{Kind: "fixture", Path: "main_test.go", SHA256: strings.Repeat("f", 64), SizeBytes: 100}},
				OutputPolicy:    verification.OutputPolicy{Version: verification.DefaultOutputPolicyVersion, StdoutMaxBytes: 1 << 20, StderrMaxBytes: 1 << 20},
			}},
		},
		ProjectID: "00000000-0000-5000-8000-000000000001", TaskID: "00000000-0000-5000-8000-000000000002",
		TaskVersionID: "00000000-0000-5000-8000-000000000003", RunID: "00000000-0000-5000-8000-000000000004",
		WorkspaceID: "00000000-0000-5000-8000-000000000005", Candidate: source,
		ProjectEnvironment: verification.ProjectEnvironment{SchemaVersion: verification.DefaultProjectEnvironmentVersion, Contract: json.RawMessage(`{"go":"1.26.5","network":"none"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return pinned
}

func evaluationReusable(outcome verification.Outcome) verification.ReusableCheck {
	exitCode := 0
	return verification.ReusableCheck{
		ID: "reusable-check", Outcome: outcome, ExitCode: &exitCode,
		Stdout: verification.Artifact{ID: "stdout", SHA256: hashBytes(nil)}, Stderr: verification.Artifact{ID: "stderr", SHA256: hashBytes(nil)},
		ParsedResult: json.RawMessage(`{}`), SandboxEvidence: json.RawMessage(`{}`),
		SandboxSpecificationSHA256: strings.Repeat("6", 64), OriginalExecutedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	}
}

func evaluationCompletionSnapshot(t *testing.T) completion.Snapshot {
	t.Helper()
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	identity := completion.Identity{
		ProjectID: "00000000-0000-5000-8000-000000000001", TaskID: "00000000-0000-5000-8000-000000000002",
		TaskVersionID: "00000000-0000-5000-8000-000000000003", RunID: "00000000-0000-5000-8000-000000000004",
		WorkspaceID: "00000000-0000-5000-8000-000000000005",
	}
	commit, tree := strings.Repeat("a", 40), strings.Repeat("b", 40)
	provenance := evidence.Provenance{
		SchemaVersion: evidence.ArtifactProvenanceSchemaVersion, ProjectID: identity.ProjectID, TaskID: identity.TaskID,
		TaskVersionID: identity.TaskVersionID, RunID: identity.RunID, WorkspaceID: identity.WorkspaceID,
		ProducerRole: "host", ProducingOperationID: "evaluation", SourceCommit: commit, SourceTree: tree,
	}
	artifacts := []evidence.ArtifactReference{
		{ID: "task-artifact", Kind: "task", MediaType: "text/markdown", SHA256: strings.Repeat("1", 64), SizeBytes: 1, StoragePath: "fixture/task", Resolved: true, Required: true, Provenance: provenance},
		{ID: "audit-artifact", Kind: "audit", MediaType: "application/json", SHA256: strings.Repeat("2", 64), SizeBytes: 1, StoragePath: "fixture/audit", Resolved: true, Required: true, Provenance: provenance},
	}
	manifest, err := evidence.ArtifactManifestHash(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	checkID, criterionID := "verification-check", "ac-1"
	claim, err := evidence.NewClaim("claim-1", criterionID, "acceptance", "The deterministic result is verified.", []evidence.EvidenceLink{{Kind: "verification_check", ID: checkID, SHA256: strings.Repeat("9", 64), Resolved: true}})
	if err != nil {
		t.Fatal(err)
	}
	budget := completion.Budget{SchemaVersion: "revolvr-budget-v1", Limit: 10, Consumed: 1}
	budget.SHA256, err = evidence.Hash(budget)
	if err != nil {
		t.Fatal(err)
	}
	return completion.Snapshot{
		SchemaVersion: completion.SnapshotSchemaVersion, Identity: identity, TaskStatus: "finalizing", RunStatus: "active",
		Aggregates: completion.Aggregates{Task: 1, Run: 1, Workspace: 1, Plan: 1, Lease: 1},
		Source:     completion.Source{BeforeCommit: strings.Repeat("c", 40), BeforeTree: strings.Repeat("d", 40), AfterCommit: commit, AfterTree: tree, DiffSHA256: strings.Repeat("e", 64), FrozenAt: now},
		Plan:       &completion.Plan{ID: "plan", VersionID: "plan-v1", SHA256: strings.Repeat("f", 64), Steps: []completion.PlanStep{{ID: "implement", Status: "completed"}}},
		Criteria:   []completion.Criterion{{ID: criterionID, Status: "passed", VerificationCheckID: checkID}},
		Verification: &completion.Verification{
			ID: "verification", Purpose: "final", Status: "passed", SourceCommit: commit, SourceTree: tree,
			ImageDigest: "sha256:" + strings.Repeat("3", 64), Profile: "strict", ProfileSHA256: strings.Repeat("4", 64), CompletedAt: now.Add(time.Minute),
			Checks: []completion.VerificationCheck{{ID: checkID, Tier: 4, Outcome: "passed", ExecutionFingerprint: strings.Repeat("9", 64), ImageDigest: "sha256:" + strings.Repeat("3", 64), Profile: "strict", ProfileSHA256: strings.Repeat("4", 64)}},
		},
		Audit:    &completion.Audit{SchemaVersion: completion.AuditSchemaVersion, ID: "audit", RunID: identity.RunID, Role: "auditor", Independent: true, Disposition: string(audit.DispositionClean), SourceCommit: commit, SourceTree: tree, ReportArtifactID: "audit-artifact", ReportSHA256: strings.Repeat("2", 64), CompletedAt: now.Add(2 * time.Minute)},
		Findings: []completion.Finding{}, Budget: budget,
		Workspace:   completion.Workspace{Status: "frozen", Reconciled: true, CandidateCommit: commit, CandidateTree: tree, DiffSHA256: strings.Repeat("e", 64)},
		Lease:       completion.Lease{Name: "global-source-mutation-v1", RunID: identity.RunID, Held: true},
		Invocations: []completion.Invocation{{Role: "implementer", Model: "deterministic-fake", PromptVersion: "evaluation-v1", PromptSHA256: strings.Repeat("5", 64), DossierSHA256: strings.Repeat("6", 64), ImageDigest: "sha256:" + strings.Repeat("7", 64), Profile: "strict"}},
		Artifacts:   artifacts, ArtifactManifestSHA256: manifest, OperatorInputs: []completion.OperatorInput{},
		Trajectory: evidence.DirectToolsTrajectoryEnvelope(), HarnessAssets: evidence.DirectToolsHarnessAssetSet(), Claims: []evidence.Claim{claim},
	}
}
