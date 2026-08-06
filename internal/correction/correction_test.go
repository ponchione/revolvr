package correction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/audit"
	"revolvr/internal/completion"
	"revolvr/internal/evidence"
	"revolvr/internal/model"
	"revolvr/internal/sandbox"
	"revolvr/internal/tool"
	"revolvr/internal/verification"
)

func TestBoundedCorrectionReverifiesReauditsAndSatisfiesCompletionGate(t *testing.T) {
	cfg := correctionFixture(t)
	result, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeCorrectedClean || len(result.Dispositions) != 1 || !cfg.Store.(*memoryStore).completed {
		t.Fatalf("result = %#v", result)
	}
	if result.Verification.Purpose != verification.PurposeFinal || result.Audit.Disposition != audit.DispositionClean || result.Audit.AuditorInvocationID == result.Worker.InvocationID {
		t.Fatal("correction did not preserve independent final verification/re-audit identities")
	}
	snapshot := completionSnapshot(t, cfg, result)
	preflight, err := completion.BuildPreflight(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.Accepted() {
		t.Fatalf("completion rejections = %#v", preflight.Rejections)
	}
}

func TestCorrectionStopsWithTypedOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		want   Outcome
		mutate func(*Config)
	}{
		{"repeated-strategy", OutcomeRepeatedStrategy, func(c *Config) { c.Store.(*memoryStore).repeated = true }},
		{"identical-diff", OutcomeIdenticalDiff, func(c *Config) {
			c.DossierInput.PriorStrategies = []StrategyRecord{{ID: "prior", FailureSHA256: strings.Repeat("d", 64), Fingerprint: strings.Repeat("e", 64), Outcome: "no_progress", DiffSHA256: strings.Repeat("f", 64)}}
		}},
		{"no-changes", OutcomeNoChanges, func(c *Config) {
			c.Worker = fakeWorker{result: WorkerResult{InvocationID: correctionInvocation, Outcome: WorkerSucceeded, Strategy: c.Strategy, Source: c.DossierInput.CurrentSource}}
		}},
		{"no-evidence", OutcomeNoEvidence, func(c *Config) { w := c.Worker.(fakeWorker); w.result.Evidence = nil; c.Worker = w }},
		{"budget", OutcomeBudgetExhausted, func(c *Config) { c.Budget.ConsumedCycles = c.Budget.MaximumCycles }},
		{"cancelled", OutcomeCancelled, func(c *Config) { w := c.Worker.(fakeWorker); w.result.Outcome = WorkerCancelled; c.Worker = w }},
		{"correction-failed", OutcomeCorrectionFailed, func(c *Config) {
			w := c.Worker.(fakeWorker)
			w.result.Outcome = WorkerFailed
			w.err = errors.New("failed")
			c.Worker = w
		}},
		{"repeated-failure", OutcomeRepeatedFailure, func(c *Config) {
			failure := &VerificationFailure{
				VerificationRunID: verificationRunID,
				CheckID:           verificationCheckID,
				GateID:            "go-test",
				Outcome:           verification.OutcomeFailed,
				ExitCode:          1,
				FailedTestIDs:     []string{"TestBoundary"},
				StableExcerpts:    []string{"boundary validation failed"},
				Component:         "service",
				AffectedFiles:     []string{"internal/service.go"},
			}
			c.DossierInput.Authority = Authority{Kind: AuthorityVerification, Source: c.DossierInput.CurrentSource, Verification: failure, Findings: []audit.Finding{}}
			worker := c.Worker.(fakeWorker)
			c.Verifier = fakeVerifier{value: failedVerification(worker.result.Source, failure.CheckID, failure.Outcome)}
		}},
		{"changes-required", OutcomeChangesRequired, func(c *Config) {
			a := c.Auditor.(fakeAuditor)
			a.value.Disposition = audit.DispositionChangesRequired
			a.value.Findings = []audit.Finding{fixtureFinding()}
			c.Auditor = a
		}},
		{"malformed-clean-reaudit", OutcomeAuditFailed, func(c *Config) {
			a := c.Auditor.(fakeAuditor)
			a.value.Findings = []audit.Finding{fixtureFinding()}
			c.Auditor = a
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := correctionFixture(t)
			test.mutate(&cfg)
			result, err := Run(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != test.want {
				t.Fatalf("outcome = %s, want %s (%s)", result.Outcome, test.want, result.Reason)
			}
		})
	}
}

func TestFailureAndStrategyNormalizationRejectsMaterialRepeatAndScopeExpansion(t *testing.T) {
	cfg := correctionFixture(t)
	first, err := NormalizeFailure(cfg.DossierInput.Authority)
	if err != nil {
		t.Fatal(err)
	}
	reordered := cfg.DossierInput.Authority
	reordered.Findings = append([]audit.Finding(nil), reordered.Findings...)
	second, err := NormalizeFailure(reordered)
	if err != nil || first.SHA256 != second.SHA256 {
		t.Fatalf("failure hashes = %s/%s, %v", first.SHA256, second.SHA256, err)
	}
	fingerprint, normalized, err := StrategyFingerprint(cfg.Strategy)
	if err != nil {
		t.Fatal(err)
	}
	variant := cfg.Strategy
	variant.Approach = "  " + cfg.Strategy.Approach + "  "
	got, gotNormalized, err := StrategyFingerprint(variant)
	if err != nil || got != fingerprint || !reflect.DeepEqual(gotNormalized, normalized) {
		t.Fatalf("whitespace-normalized strategy = %s/%s %#v/%#v, %v", got, fingerprint, gotNormalized, normalized, err)
	}
	variant = cfg.Strategy
	variant.Approach = strings.ToUpper(cfg.Strategy.Approach)
	got, gotNormalized, err = StrategyFingerprint(variant)
	if err != nil || got != fingerprint || !reflect.DeepEqual(gotNormalized, normalized) {
		t.Fatalf("semantic strategy = %s/%s %#v/%#v, %v", got, fingerprint, gotNormalized, normalized, err)
	}
	cfg.Strategy.TargetFiles = []string{"internal/unrelated.go"}
	if _, err := Run(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "outside cited") {
		t.Fatalf("scope expansion error = %v", err)
	}
	cfg = correctionFixture(t)
	worker := cfg.Worker.(fakeWorker)
	worker.result.ChangedSymbols = []string{"UnrelatedSymbol"}
	cfg.Worker = worker
	result, err := Run(context.Background(), cfg)
	if err != nil || result.Outcome != OutcomeCorrectionFailed || !strings.Contains(result.Reason, "undeclared symbol") {
		t.Fatalf("symbol scope expansion = %#v, %v", result, err)
	}
}

const (
	projectID               = "10000000-0000-0000-0000-000000000001"
	taskID                  = "10000000-0000-0000-0000-000000000002"
	versionID               = "10000000-0000-0000-0000-000000000003"
	runID                   = "10000000-0000-0000-0000-000000000004"
	workspaceID             = "10000000-0000-0000-0000-000000000005"
	auditRunID              = "10000000-0000-0000-0000-000000000006"
	verificationRunID       = "10000000-0000-0000-0000-000000000007"
	verificationCheckID     = "10000000-0000-0000-0000-000000000008"
	correctionInvocation    = "10000000-0000-0000-0000-000000000009"
	strategyID              = "10000000-0000-0000-0000-000000000010"
	outcomeID               = "10000000-0000-0000-0000-000000000011"
	finalVerificationID     = "10000000-0000-0000-0000-000000000012"
	finalCheckID            = "10000000-0000-0000-0000-000000000013"
	reauditID               = "10000000-0000-0000-0000-000000000014"
	reauditorID             = "10000000-0000-0000-0000-000000000015"
	correctionFixtureSource = "package service\n\nfunc Serve() {\n\treadInput()\n\tvalidateInput()\n\twriteOutput()\n}\n\nfunc validateInput() {\n\tcheckBoundary()\n\trecordEvidence()\n}\n"
)

func correctionFixture(t *testing.T) Config {
	t.Helper()
	before := audit.Source{Revision: strings.Repeat("a", 64), Commit: strings.Repeat("b", 40), Tree: strings.Repeat("c", 40), DiffSHA256: strings.Repeat("d", 64)}
	after := audit.Source{Revision: strings.Repeat("e", 64), Commit: strings.Repeat("f", 40), Tree: strings.Repeat("1", 40), DiffSHA256: strings.Repeat("f", 64)}
	finding := fixtureFinding()
	identity := audit.Identity{ProjectID: projectID, TaskID: taskID, TaskVersionID: versionID, RunID: runID, WorkspaceID: workspaceID}
	authority := Authority{Kind: AuthorityFindings, Source: before, AuditRunID: auditRunID, Findings: []audit.Finding{finding}}
	input := DossierInput{Identity: identity, Authority: authority, CurrentSource: before, RelevantSource: []audit.SourceFile{{Path: "internal/service.go", SHA256: model.SHA256([]byte(correctionFixtureSource)), SizeBytes: int64(len(correctionFixtureSource)), Symbols: []string{"Serve"}, ArtifactID: "source-service", Content: correctionFixtureSource}}, RelevantTests: []RelevantTest{{ID: "service-test", Argv: []string{"go", "test", "./internal/service"}, Paths: []string{"internal/service/service_test.go"}}}, PriorStrategies: []StrategyRecord{}}
	strategy := Strategy{SchemaVersion: StrategySchemaVersion, Approach: "Validate the cited boundary before serving.", Techniques: []string{"table driven regression"}, TargetFiles: []string{"internal/service.go"}, TargetSymbols: []string{"Serve"}, ExpectedEvidence: []string{"focused test passes", "final verification passes"}}
	spec := correctorSandbox(t, identity, correctionInvocation)
	registry, err := tool.RegistryForRole(sandbox.RoleCorrector)
	if err != nil {
		t.Fatal(err)
	}
	worker := fakeWorker{result: WorkerResult{InvocationID: correctionInvocation, Outcome: WorkerSucceeded, Strategy: strategy, Source: after, ChangedFiles: []string{"internal/service.go"}, ChangedSymbols: []string{"Serve"}, ResolvedFindingIDs: []string{finding.ID}, Evidence: []audit.DispositionEvidence{{Kind: "artifact", ID: "correction-report", SHA256: strings.Repeat("3", 64), Reference: "correction/report.json"}}}}
	verificationValue := passedVerification(after)
	reaudit := AuditResult{AuditID: reauditID, AuditorInvocationID: reauditorID, Disposition: audit.DispositionClean, Source: after, VerificationRunID: finalVerificationID, Findings: []audit.Finding{}, CompletedAt: time.Date(2026, 8, 6, 12, 3, 0, 0, time.UTC)}
	return Config{OperationID: "correction-operation", StrategyID: strategyID, OutcomeID: outcomeID, Identity: identity, DossierInput: input, Strategy: strategy, Budget: Budget{MaximumCycles: 2, MaximumAttempts: 2}, CorrectorInvocationID: correctionInvocation, Sandbox: spec, ToolRegistry: registry, Worker: worker, Verifier: fakeVerifier{value: verificationValue}, Auditor: fakeAuditor{value: reaudit}, Store: &memoryStore{}, Dispositioner: &memoryDispositioner{}, DispositionID: func(string) string { return "10000000-0000-0000-0000-000000000016" }, Clock: func() time.Time { return time.Date(2026, 8, 6, 12, 1, 0, 0, time.UTC) }}
}

func fixtureFinding() audit.Finding {
	return audit.Finding{ID: "missing-boundary-check", Significance: audit.SignificanceBlocking, Summary: "The cited service boundary is unchecked.", RequiredCorrection: "Validate the cited input before serving.", SourceEvidence: []audit.Citation{{ArtifactID: "source-service", Path: "internal/service.go", SHA256: model.SHA256([]byte(correctionFixtureSource)), StartLine: 10, EndLine: 12}}, AffectedFiles: []string{"internal/service.go"}, AffectedSymbols: []string{"Serve"}, CriterionImpact: []audit.CriterionImpact{{CriterionID: "AC-1", Impact: audit.ImpactViolated, Detail: "The boundary criterion is not satisfied."}}}
}

func correctorSandbox(t *testing.T, identity audit.Identity, invocationID string) sandbox.Specification {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"workspace", "cache"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "context.json"), []byte("{}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	image := sandbox.Image{Reference: "revolvr/corrector", Digest: "sha256:" + strings.Repeat("a", 64)}
	resources := sandbox.Resources{CPUs: 2, MemoryBytes: 1 << 30, PIDs: 128, TimeoutSeconds: 300, TmpfsBytes: 1 << 20}
	policy := sandbox.Policy{ProjectID: identity.ProjectID, TaskID: identity.TaskID, RunID: invocationID, Role: sandbox.RoleCorrector, ApprovedImages: []sandbox.Image{image}, AllowedProfiles: []sandbox.RuntimeProfile{sandbox.ProfileStrict}, AllowedNetworks: []sandbox.NetworkProfile{sandbox.NetworkNone}, AllowedEnvironmentNames: []string{"ROLE"}, ManagedSources: []sandbox.ManagedSource{{ID: "context", Root: root, RelativePath: "context.json", Kind: sandbox.SourceContext, Type: sandbox.SourceFile, Target: "/context"}, {ID: "workspace", Root: root, RelativePath: "workspace", Kind: sandbox.SourceWorkspace, Type: sandbox.SourceDirectory, Target: "/workspace"}, {ID: "cache", Root: root, RelativePath: "cache", Kind: sandbox.SourceCache, Type: sandbox.SourceDirectory, Target: "/cache/go"}}, MaximumResources: resources}
	request := sandbox.Request{SchemaVersion: sandbox.RequestSchemaVersion, SandboxID: "corrector-sandbox", ProjectID: identity.ProjectID, TaskID: identity.TaskID, RunID: invocationID, Role: sandbox.RoleCorrector, Image: image, RuntimeProfile: sandbox.ProfileStrict, Command: []string{"/usr/local/bin/revolvr-worker", "--correct"}, WorkingDirectory: "/workspace", Mounts: []sandbox.Mount{{SourceID: "context", Target: "/context", Mode: sandbox.MountReadOnly}, {SourceID: "workspace", Target: "/workspace", Mode: sandbox.MountReadWrite}, {SourceID: "cache", Target: "/cache/go", Mode: sandbox.MountReadOnly}}, Network: sandbox.NetworkNone, Resources: resources, Environment: map[string]string{"ROLE": "corrector"}}
	spec, err := sandbox.Validate(request, policy)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func passedVerification(source audit.Source) audit.VerificationEvidence {
	return audit.VerificationEvidence{ID: finalVerificationID, Purpose: verification.PurposeFinal, Status: verification.RunPassed, Source: verification.SourceIdentity{Commit: source.Commit, Tree: source.Tree}, CompletedAt: time.Date(2026, 8, 6, 12, 2, 0, 0, time.UTC), Checks: []audit.VerificationCheck{{ID: finalCheckID, Tier: verification.TierFinal, Outcome: verification.OutcomePassed, ExecutionFingerprint: strings.Repeat("4", 64)}}}
}
func failedVerification(source audit.Source, checkID string, outcome verification.Outcome) audit.VerificationEvidence {
	return audit.VerificationEvidence{ID: finalVerificationID, Purpose: verification.PurposeFinal, Status: verification.RunFailed, Source: verification.SourceIdentity{Commit: source.Commit, Tree: source.Tree}, CompletedAt: time.Date(2026, 8, 6, 12, 2, 0, 0, time.UTC), Checks: []audit.VerificationCheck{{ID: checkID, Tier: verification.TierFinal, Outcome: outcome, ExecutionFingerprint: strings.Repeat("4", 64)}}}
}

type fakeWorker struct {
	result WorkerResult
	err    error
}

func (f fakeWorker) Run(context.Context, WorkerRequest) (WorkerResult, error) { return f.result, f.err }

type fakeVerifier struct {
	value audit.VerificationEvidence
	err   error
}

func (f fakeVerifier) Verify(context.Context, VerificationRequest) (audit.VerificationEvidence, error) {
	return f.value, f.err
}

type fakeAuditor struct {
	value AuditResult
	err   error
}

func (f fakeAuditor) Audit(context.Context, AuditRequest) (AuditResult, error) { return f.value, f.err }

type memoryStore struct{ repeated, begun, completed bool }

func (m *memoryStore) HasFailedStrategy(context.Context, string, string, string) (bool, error) {
	return m.repeated, nil
}
func (m *memoryStore) Begin(context.Context, audit.Identity, AttemptRecord) error {
	m.begun = true
	return nil
}
func (m *memoryStore) Complete(context.Context, audit.Identity, OutcomeRecord) error {
	m.completed = true
	return nil
}

type memoryDispositioner struct{ calls int }

func (m *memoryDispositioner) DispositionMany(_ context.Context, commands []audit.DispositionCommand) ([]audit.DispositionResult, error) {
	m.calls += len(commands)
	results := make([]audit.DispositionResult, len(commands))
	for index := range commands {
		results[index] = audit.DispositionResult{ID: "resolution"}
	}
	return results, nil
}

func completionSnapshot(t *testing.T, cfg Config, result Result) completion.Snapshot {
	t.Helper()
	now := time.Date(2026, 8, 6, 12, 4, 0, 0, time.UTC)
	source := result.Worker.Source
	provenance := evidence.Provenance{SchemaVersion: evidence.ArtifactProvenanceSchemaVersion, ProjectID: projectID, TaskID: taskID, TaskVersionID: versionID, RunID: runID, WorkspaceID: workspaceID, ProducerRole: "host", ProducingOperationID: "fixture", SourceCommit: source.Commit, SourceTree: source.Tree}
	artifacts := []evidence.ArtifactReference{{ID: "task-artifact", Kind: "task_source", MediaType: "application/json", SHA256: strings.Repeat("5", 64), SizeBytes: 1, StoragePath: "completion/task", Resolved: true, Required: true, Provenance: provenance}, {ID: "plan-artifact", Kind: "plan_source", MediaType: "application/json", SHA256: strings.Repeat("6", 64), SizeBytes: 1, StoragePath: "completion/plan", Resolved: true, Required: true, Provenance: provenance}, {ID: "diff-artifact", Kind: "diff", MediaType: "text/plain", SHA256: source.DiffSHA256, SizeBytes: 1, StoragePath: "completion/diff", Resolved: true, Required: true, Provenance: provenance}, {ID: "verification-artifact", Kind: "verification_report", MediaType: "application/json", SHA256: strings.Repeat("7", 64), SizeBytes: 1, StoragePath: "completion/verification", Resolved: true, Required: true, Provenance: provenance}, {ID: "audit-artifact", Kind: "audit_report", MediaType: "application/json", SHA256: strings.Repeat("8", 64), SizeBytes: 1, StoragePath: "completion/audit", Resolved: true, Required: true, Provenance: provenance}}
	manifest, _ := evidence.ArtifactManifestHash(artifacts)
	criterionID := "10000000-0000-0000-0000-000000000020"
	claim, err := evidence.NewClaim("10000000-0000-0000-0000-000000000021", criterionID, "acceptance", "The correction passed exact verification.", []evidence.EvidenceLink{{Kind: "verification_check", ID: finalCheckID, SHA256: strings.Repeat("4", 64), Resolved: true}})
	if err != nil {
		t.Fatal(err)
	}
	budget := completion.Budget{SchemaVersion: "revolvr-budget-v1", Limit: 2, Consumed: 1}
	budget.SHA256, _ = evidence.Hash(budget)
	now = time.Date(2026, 8, 6, 12, 1, 0, 0, time.UTC)
	return completion.Snapshot{SchemaVersion: completion.SnapshotSchemaVersion, Identity: completion.Identity{ProjectID: projectID, TaskID: taskID, TaskVersionID: versionID, RunID: runID, WorkspaceID: workspaceID}, TaskStatus: "finalizing", RunStatus: "active", Aggregates: completion.Aggregates{Task: 1, Run: 1, Workspace: 1, Plan: 1, Lease: 1}, Source: completion.Source{BeforeCommit: strings.Repeat("b", 40), BeforeTree: strings.Repeat("c", 40), AfterCommit: source.Commit, AfterTree: source.Tree, DiffSHA256: source.DiffSHA256, FrozenAt: now}, Plan: &completion.Plan{ID: "plan", VersionID: "plan-version", SHA256: strings.Repeat("9", 64), Steps: []completion.PlanStep{{ID: "correct", Status: "completed"}}}, Criteria: []completion.Criterion{{ID: criterionID, Status: "passed", VerificationCheckID: finalCheckID}}, Verification: &completion.Verification{ID: finalVerificationID, Purpose: "final", Status: "passed", SourceCommit: source.Commit, SourceTree: source.Tree, ImageDigest: "sha256:" + strings.Repeat("a", 64), Profile: "strict", ProfileSHA256: strings.Repeat("b", 64), CompletedAt: result.Verification.CompletedAt, Checks: []completion.VerificationCheck{{ID: finalCheckID, Tier: 4, Outcome: "passed", ExecutionFingerprint: strings.Repeat("4", 64), ImageDigest: "sha256:" + strings.Repeat("a", 64), Profile: "strict", ProfileSHA256: strings.Repeat("b", 64)}}}, Audit: &completion.Audit{SchemaVersion: completion.AuditSchemaVersion, ID: reauditID, RunID: runID, Role: "auditor", Independent: true, Disposition: "clean", SourceCommit: source.Commit, SourceTree: source.Tree, ReportArtifactID: "audit-artifact", ReportSHA256: strings.Repeat("8", 64), CompletedAt: result.Audit.CompletedAt}, Findings: []completion.Finding{{ID: fixtureFinding().ID, Significance: "blocking", Status: "resolved", EvidenceID: result.Dispositions[0].ID}}, Budget: budget, Workspace: completion.Workspace{Status: "frozen", Reconciled: true, CandidateCommit: source.Commit, CandidateTree: source.Tree, DiffSHA256: source.DiffSHA256}, Lease: completion.Lease{Name: "global-source-mutation-v1", RunID: runID, Held: true}, Invocations: []completion.Invocation{{Role: "corrector", Model: "gpt-fixture", PromptVersion: "corrector-v1", PromptSHA256: strings.Repeat("c", 64), DossierSHA256: strings.Repeat("d", 64), ImageDigest: "sha256:" + strings.Repeat("a", 64), Profile: "strict"}}, Artifacts: artifacts, ArtifactManifestSHA256: manifest, OperatorInputs: []completion.OperatorInput{}, Trajectory: evidence.DirectToolsTrajectoryEnvelope(), HarnessAssets: evidence.DirectToolsHarnessAssetSet(), Claims: []evidence.Claim{claim}}
}
