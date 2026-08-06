package correction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/audit"
	"revolvr/internal/completion"
	"revolvr/internal/evidence"
	"revolvr/internal/model"
	storage "revolvr/internal/storage/postgres"
	"revolvr/internal/verification"
)

func TestPostgresAuditCorrectionRollbackReplayConcurrencyAndCompletionProjection(t *testing.T) {
	fixture := newAuditCorrectionPostgresFixture(t)
	auditStore, err := audit.NewPostgresStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	findings := []audit.Finding{
		fixture.finding("missing-boundary-check", "Validate the cited boundary."),
		fixture.finding("missing-regression-test", "Add the cited regression test."),
	}
	changes := fixture.auditCommand(t, audit.DispositionChangesRequired, findings, fixture.now.Add(2*time.Minute))
	forgedModel := changes
	forgedModel.Candidate.ModelRequest.ReasoningEffort = "low"
	if _, err := auditStore.Persist(fixture.ctx, forgedModel); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("forged audit model evidence error = %v", err)
	}

	auditStore.SetFailureInjector(func(point audit.PersistenceFailurePoint) error {
		if point == audit.FailureBeforeEvent {
			return errors.New("forced audit event failure")
		}
		return nil
	})
	if _, err := auditStore.Persist(fixture.ctx, changes); !errors.Is(err, audit.ErrPersistence) {
		t.Fatalf("forced audit rollback error = %v", err)
	}
	fixture.assertCount(t, "core.audit_runs", "id", changes.Candidate.AuditID, 0)
	fixture.assertCount(t, "core.artifacts", "sha256", changes.Report.SHA256, 0)

	auditStore.SetFailureInjector(nil)
	result, err := auditStore.Persist(fixture.ctx, changes)
	if err != nil || result.Replay || result.AuditRunID != changes.Candidate.AuditID {
		t.Fatalf("audit persistence = %#v, %v", result, err)
	}
	replay, err := auditStore.Persist(fixture.ctx, changes)
	if err != nil || !replay.Replay || replay.AuditRunID != changes.Candidate.AuditID {
		t.Fatalf("audit replay = %#v, %v", replay, err)
	}
	divergent := changes
	divergent.CompletedAt = divergent.CompletedAt.Add(time.Microsecond)
	if _, err := auditStore.Persist(fixture.ctx, divergent); !errors.Is(err, audit.ErrPersistence) {
		t.Fatalf("divergent audit replay error = %v", err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.audit_runs SET disposition='clean' WHERE id=$1`, changes.Candidate.AuditID); err == nil {
		t.Fatal("immutable audit occurrence accepted an update")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.verification_runs SET status='failed' WHERE id=$1`, fixture.verificationID); err == nil {
		t.Fatal("architecture-017 verification authority accepted audit-side mutation")
	}

	correctionStore, err := NewPostgresStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	authority := Authority{Kind: AuthorityFindings, Source: fixture.source, AuditRunID: changes.Candidate.AuditID, Findings: findings}
	failure, err := NormalizeFailure(authority)
	if err != nil {
		t.Fatal(err)
	}
	strategy := Strategy{
		SchemaVersion: StrategySchemaVersion,
		Approach:      "Validate the cited boundary and add its focused regression test.",
		Techniques:    []string{"table driven regression"},
		TargetFiles:   []string{"internal/service.go"},
		TargetSymbols: []string{"Serve"},
		ExpectedEvidence: []string{
			"focused test passes", "final verification passes", "independent re-audit is clean",
		},
	}
	fingerprint, normalized, err := StrategyFingerprint(strategy)
	if err != nil {
		t.Fatal(err)
	}
	dossier, err := BuildDossier(DossierInput{
		Identity: fixture.identity, Authority: authority, CurrentSource: fixture.source,
		RelevantSource:  []audit.SourceFile{fixture.sourceFile()},
		RelevantTests:   []RelevantTest{{ID: "service-test", Argv: []string{"go", "test", "./internal/service"}, Paths: []string{"internal/service/service_test.go"}}},
		PriorStrategies: []StrategyRecord{},
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := AttemptRecord{
		OperationID: "strategy-" + uuid.NewString(), StrategyID: uuid.NewString(),
		Failure: failure, Strategy: normalized, StrategyFingerprint: fingerprint,
		DossierSHA256: dossier.SHA256, CorrectorInvocationID: "corrector-" + uuid.NewString(),
		SandboxSpecificationSHA256: strings.Repeat("9", 64), StartedAt: fixture.now.Add(3 * time.Minute),
	}
	if err := correctionStore.Begin(fixture.ctx, fixture.identity, attempt); err != nil {
		t.Fatal(err)
	}
	if err := correctionStore.Begin(fixture.ctx, fixture.identity, attempt); err != nil {
		t.Fatalf("strategy replay: %v", err)
	}
	outcome := OutcomeRecord{
		ID: uuid.NewString(), StrategyID: attempt.StrategyID, Outcome: "no_progress",
		ResultingSource: fixture.source,
		Evidence:        []audit.DispositionEvidence{{Kind: "artifact", ID: "correction-attempt", SHA256: strings.Repeat("8", 64), Reference: "correction/attempt.json"}},
		CompletedAt:     fixture.now.Add(4 * time.Minute),
	}
	if err := correctionStore.Complete(fixture.ctx, fixture.identity, outcome); err != nil {
		t.Fatal(err)
	}
	if err := correctionStore.Complete(fixture.ctx, fixture.identity, outcome); err != nil {
		t.Fatalf("outcome replay: %v", err)
	}
	repeated, err := correctionStore.HasFailedStrategy(fixture.ctx, fixture.identity.TaskID, failure.SHA256, fingerprint)
	if err != nil || !repeated {
		t.Fatalf("failed strategy lookup = %v, %v", repeated, err)
	}
	divergentOutcome := outcome
	divergentOutcome.Outcome = "failed"
	if err := correctionStore.Complete(fixture.ctx, fixture.identity, divergentOutcome); err == nil {
		t.Fatal("divergent strategy outcome replay was accepted")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.strategies SET strategy_fingerprint=$1 WHERE id=$2`, strings.Repeat("0", 64), attempt.StrategyID); err == nil {
		t.Fatal("immutable correction strategy accepted an update")
	}

	clean := fixture.auditCommand(t, audit.DispositionClean, nil, fixture.now.Add(5*time.Minute))
	if _, err := auditStore.Persist(fixture.ctx, clean); err != nil {
		t.Fatal(err)
	}
	successStrategy := strategy
	successStrategy.Approach = "Validate the cited boundary with a distinct focused implementation."
	successFingerprint, successNormalized, err := StrategyFingerprint(successStrategy)
	if err != nil {
		t.Fatal(err)
	}
	successAttempt := attempt
	successAttempt.OperationID = "strategy-" + uuid.NewString()
	successAttempt.StrategyID = uuid.NewString()
	successAttempt.Strategy = successNormalized
	successAttempt.StrategyFingerprint = successFingerprint
	successAttempt.CorrectorInvocationID = "corrector-" + uuid.NewString()
	successAttempt.StartedAt = fixture.now.Add(6 * time.Minute)
	if err := correctionStore.Begin(fixture.ctx, fixture.identity, successAttempt); err != nil {
		t.Fatal(err)
	}
	successOutcome := OutcomeRecord{
		ID: uuid.NewString(), StrategyID: successAttempt.StrategyID, Outcome: "succeeded",
		ResultingSource: fixture.source, VerificationRunID: fixture.verificationID,
		AuditRunID:  clean.Candidate.AuditID,
		Evidence:    []audit.DispositionEvidence{{Kind: "artifact", ID: "successful-correction", SHA256: strings.Repeat("5", 64), Reference: "correction/success.json"}},
		CompletedAt: fixture.now.Add(7 * time.Minute),
	}
	if err := correctionStore.Complete(fixture.ctx, fixture.identity, successOutcome); err != nil {
		t.Fatal(err)
	}
	rollbackBatch := []audit.DispositionCommand{
		fixture.resolution(findings[0].ID, clean.Candidate.AuditID),
		fixture.resolution(findings[1].ID, uuid.NewString()),
	}
	if _, err := auditStore.DispositionMany(fixture.ctx, rollbackBatch); !errors.Is(err, audit.ErrPersistence) {
		t.Fatalf("atomic disposition rollback error = %v", err)
	}
	fixture.assertTaskDispositionCount(t, 0)

	commands := []audit.DispositionCommand{
		fixture.resolution(findings[0].ID, clean.Candidate.AuditID),
		fixture.waiver(findings[0].ID),
	}
	type dispositionAttempt struct {
		command audit.DispositionCommand
		result  audit.DispositionResult
		err     error
	}
	ready := make(chan struct{})
	attempts := make(chan dispositionAttempt, len(commands))
	var workers sync.WaitGroup
	for _, command := range commands {
		command := command
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-ready
			value, err := auditStore.Disposition(fixture.ctx, command)
			attempts <- dispositionAttempt{command: command, result: value, err: err}
		}()
	}
	close(ready)
	workers.Wait()
	close(attempts)
	var winner dispositionAttempt
	succeeded := 0
	for attempt := range attempts {
		if attempt.err == nil {
			winner, succeeded = attempt, succeeded+1
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent disposition successes = %d, want 1", succeeded)
	}
	winnerReplay, err := auditStore.Disposition(fixture.ctx, winner.command)
	if err != nil || !winnerReplay.Replay || winnerReplay.ID != winner.result.ID {
		t.Fatalf("disposition replay = %#v, %v", winnerReplay, err)
	}
	second := fixture.resolution(findings[1].ID, clean.Candidate.AuditID)
	if _, err := auditStore.Disposition(fixture.ctx, second); err != nil {
		t.Fatal(err)
	}
	fixture.assertTaskDispositionCount(t, 2)
	if _, err := fixture.pool.Exec(fixture.ctx, `UPDATE core.finding_dispositions SET status='stale' WHERE id=$1`, winner.result.ID); err == nil {
		t.Fatal("immutable finding disposition accepted an update")
	}

	overlay := audit.CompletionOverlay{Base: emptyCompletionSupplement{}}
	supplement, err := overlay.LoadCompletionSupplement(fixture.ctx, storage.New(fixture.pool), completion.Key{Identity: completion.Identity{
		ProjectID: fixture.identity.ProjectID, TaskID: fixture.identity.TaskID,
		TaskVersionID: fixture.identity.TaskVersionID, RunID: fixture.identity.RunID,
		WorkspaceID: fixture.identity.WorkspaceID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if supplement.Audit == nil || supplement.Audit.ID != clean.Candidate.AuditID || supplement.Audit.Disposition != "clean" || len(supplement.Findings) != 2 || len(supplement.Artifacts) != 1 {
		t.Fatalf("canonical completion audit projection = %#v", supplement)
	}
	fixture.assertCount(t, "core.completions", "task_id", fixture.identity.TaskID, 0)
}

type emptyCompletionSupplement struct{}

func (emptyCompletionSupplement) LoadCompletionSupplement(context.Context, *storage.Queries, completion.Key) (completion.Supplement, error) {
	return completion.Supplement{}, nil
}

type auditCorrectionPostgresFixture struct {
	ctx            context.Context
	pool           *pgxpool.Pool
	identity       audit.Identity
	source         audit.Source
	verificationID string
	checkID        string
	diffArtifactID string
	diffContent    string
	sourceContent  string
	now            time.Time
}

func newAuditCorrectionPostgresFixture(t *testing.T) *auditCorrectionPostgresFixture {
	t.Helper()
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := storage.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	diffContent := "diff --git a/internal/service.go b/internal/service.go\n+validate the service boundary " + uuid.NewString() + "\n"
	f := &auditCorrectionPostgresFixture{
		ctx: ctx, pool: pool,
		identity: audit.Identity{ProjectID: uuid.NewString(), TaskID: uuid.NewString(), TaskVersionID: uuid.NewString(), RunID: uuid.NewString(), WorkspaceID: uuid.NewString()},
		source: audit.Source{
			Revision: evidence.HashBytes([]byte(uuid.NewString())), Commit: strings.Repeat("b", 40),
			Tree: strings.Repeat("c", 40), DiffSHA256: model.SHA256([]byte(diffContent)),
		},
		verificationID: uuid.NewString(), checkID: uuid.NewString(), diffArtifactID: uuid.NewString(),
		diffContent: diffContent, sourceContent: correctionFixtureSource,
		now: time.Now().UTC().Truncate(time.Microsecond),
	}
	f.insertAuthority(t)
	return f
}

func (f *auditCorrectionPostgresFixture) insertAuthority(t *testing.T) {
	t.Helper()
	q := storage.New(f.pool)
	projectID, taskID := dbUUID(t, f.identity.ProjectID), dbUUID(t, f.identity.TaskID)
	versionID, runID := dbUUID(t, f.identity.TaskVersionID), dbUUID(t, f.identity.RunID)
	workspaceID, sourceID := dbUUID(t, f.identity.WorkspaceID), dbUUID(t, uuid.NewString())
	taskArtifactID := dbUUID(t, uuid.NewString())
	if _, err := q.InsertProject(f.ctx, storage.InsertProjectParams{ID: projectID, Name: "audit-" + uuid.NewString(), Status: "active", CreatedAt: dbTime(f.now), UpdatedAt: dbTime(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertProjectSource(f.ctx, storage.InsertProjectSourceParams{ID: sourceID, ProjectID: projectID, CanonicalSourcePath: "/audit/source/" + uuid.NewString(), ManagedRepositoryPath: "/audit/managed/" + uuid.NewString(), CurrentCommit: strings.Repeat("1", 40), CurrentTree: strings.Repeat("2", 40), DirtyState: []byte(`{}`), Remotes: []byte(`[]`)}); err != nil {
		t.Fatal(err)
	}
	f.insertArtifact(t, taskArtifactID, evidence.HashBytes([]byte(uuid.NewString())), "task_source", "audit/task/"+uuid.NewString())
	if _, err := q.InsertTask(f.ctx, storage.InsertTaskParams{ID: taskID, ProjectID: projectID, ExternalTaskID: "audit-" + strings.ToLower(uuid.NewString()), Status: "draft", CreatedAt: dbTime(f.now), UpdatedAt: dbTime(f.now)}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertTaskVersion(f.ctx, storage.InsertTaskVersionParams{
		ID: versionID, TaskID: taskID, VersionNumber: 1, SourceArtifactID: taskArtifactID,
		Title: "Audit persistence", Goal: "Persist exact independent audit authority", RiskClass: "high", MutationClass: "architecture_change", NetworkProfile: "none", Priority: 1,
		Scope: []byte(`[]`), ExcludedScope: []byte(`[]`), VerificationPlan: []byte(`[]`), Budget: []byte(`{}`), SecretRequirements: []byte(`[]`), ExpectedPaths: []byte(`[]`), OperatorCheckpoints: []byte(`[]`), CreatedAt: dbTime(f.now),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE core.tasks SET accepted_version_id=$1,status='auditing',aggregate_version=2,updated_at=$2 WHERE id=$3`, f.identity.TaskVersionID, f.now, f.identity.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertRun(f.ctx, storage.InsertRunParams{ID: runID, ProjectID: projectID, TaskID: taskID, TaskVersionID: versionID, ProjectSourceID: sourceID, AdmittedTaskAggregateVersion: 2, SourceCommit: strings.Repeat("1", 40), SourceTree: strings.Repeat("2", 40), CoordinatorIdentity: "audit-postgres-test", CreatedAt: dbTime(f.now)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `UPDATE core.runs SET status='released',released_at=$1,updated_at=$1 WHERE id=$2 AND status='active'`, time.Now().UTC(), f.identity.RunID)
	})
	if _, err := q.InsertWorkspace(f.ctx, storage.InsertWorkspaceParams{ID: workspaceID, RunID: runID, ProjectID: projectID, ProjectSourceID: sourceID, TaskID: taskID, CreationOperationID: "workspace-" + uuid.NewString(), SymbolicSourceID: "audit-" + uuid.NewString(), OriginalCheckoutPath: "/audit/original", ManagedRepositoryPath: "/audit/managed/" + uuid.NewString(), WorkspaceRoot: "/audit/workspaces", WorkspacePath: "/audit/workspaces/" + uuid.NewString(), BranchRef: "refs/heads/revolvr/workspaces/audit-" + uuid.NewString(), SourceCommit: strings.Repeat("1", 40), SourceTree: strings.Repeat("2", 40), OriginalIdentityBefore: []byte(`{}`), CreatedAt: dbTime(f.now)}); err != nil {
		t.Fatal(err)
	}
	f.insertArtifactWithMetadata(t, dbUUID(t, f.diffArtifactID), f.source.DiffSHA256, int64(len(f.diffContent)), "text/x-diff", "diff", "audit/diff/"+uuid.NewString())
	if _, err := f.pool.Exec(f.ctx, `UPDATE core.workspaces SET status='frozen',diff_artifact_id=$1,diff_sha256=$2,candidate_commit=$3,candidate_tree=$4,updated_at=$5 WHERE id=$6`, f.diffArtifactID, f.source.DiffSHA256, f.source.Commit, f.source.Tree, f.now, f.identity.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	stdoutID, stderrID := dbUUID(t, uuid.NewString()), dbUUID(t, uuid.NewString())
	f.insertArtifact(t, stdoutID, evidence.HashBytes([]byte(uuid.NewString())), "verification_stdout", "audit/stdout/"+uuid.NewString())
	f.insertArtifact(t, stderrID, evidence.HashBytes([]byte(uuid.NewString())), "verification_stderr", "audit/stderr/"+uuid.NewString())
	verificationAt := f.now.Add(time.Minute)
	if _, err := q.InsertVerificationRun(f.ctx, storage.InsertVerificationRunParams{ID: dbUUID(t, f.verificationID), ProjectID: projectID, TaskID: taskID, TaskVersionID: versionID, RunID: runID, WorkspaceID: workspaceID, Purpose: "final", Status: "passed", PlanSchemaVersion: "verification-plan-v1", PlanVersion: "1", PlanSha256: strings.Repeat("6", 64), PinnedPlan: []byte(`{}`), CandidateCommit: f.source.Commit, CandidateTree: f.source.Tree, ProjectEnvironmentSha256: strings.Repeat("7", 64), ProjectEnvironment: []byte(`{}`), Differential: []byte(`{}`), StartedAt: dbTime(f.now), CompletedAt: dbTime(verificationAt), CreatedAt: dbTime(verificationAt)}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertVerificationCheck(f.ctx, storage.InsertVerificationCheckParams{ID: dbUUID(t, f.checkID), VerificationRunID: dbUUID(t, f.verificationID), RunID: runID, Ordinal: 1, GateID: "final", Tier: 4, Outcome: "passed", ExecutionFingerprint: strings.Repeat("8", 64), VerifierProtocolVersion: "v1", VerifierImplementationVersion: "v1", ParserKind: "none", ParserVersion: "v1", SourceCommit: f.source.Commit, SourceTree: f.source.Tree, CommandArgv: []byte(`["go","test","./..."]`), WorkingDirectory: "/workspace", Environment: []byte(`[]`), ImageReference: "revolvr/verifier", ImageDigest: "sha256:" + strings.Repeat("9", 64), SandboxProfile: "strict", SandboxProfileSha256: strings.Repeat("a", 64), SandboxSpecificationSha256: strings.Repeat("b", 64), AuthorityInputs: []byte(`[]`), OutputPolicy: []byte(`{}`), ExitCode: pgtype.Int4{Int32: 0, Valid: true}, StdoutArtifactID: stdoutID, StderrArtifactID: stderrID, ParsedResult: []byte(`{}`), SandboxEvidence: []byte(`{}`), FailureSignatures: []byte(`[]`), OriginalExecutedAt: dbTime(verificationAt), OccurredAt: dbTime(verificationAt), StartedAt: dbTime(f.now), CompletedAt: dbTime(verificationAt), CreatedAt: dbTime(verificationAt)}); err != nil {
		t.Fatal(err)
	}
}

func (f *auditCorrectionPostgresFixture) auditCommand(t *testing.T, disposition audit.Disposition, findings []audit.Finding, startedAt time.Time) audit.PersistCommand {
	t.Helper()
	if findings == nil {
		findings = []audit.Finding{}
	}
	input := audit.DossierInput{
		Identity: f.identity, Source: f.source,
		Task:         audit.TaskEvidence{ExternalID: "architecture-019", Title: "Auditor and corrector", Goal: "Audit exact source and correct only cited findings.", RiskClass: "high", MutationClass: "architecture_change", Scope: []string{"internal"}, ExcludedScope: []string{}, Artifact: f.artifactIdentity(uuid.NewString(), strings.Repeat("1", 64), "task_source", "audit/task.json")},
		Plan:         audit.PlanEvidence{ID: "plan", VersionID: "plan-version", SHA256: strings.Repeat("2", 64), Steps: []audit.PlanStep{{ID: "audit", Status: "completed", Description: "Persist and enforce exact audit authority.", CriterionIDs: []string{"AC-1"}}}, Artifact: f.artifactIdentity(uuid.NewString(), strings.Repeat("2", 64), "plan_source", "audit/plan.json")},
		Criteria:     []audit.Criterion{{ID: "AC-1", Requirement: "Audit authority is exact and immutable.", Status: "passed", VerificationReference: "go test ./..."}},
		Diff:         audit.DiffEvidence{Artifact: f.diffArtifactIdentity(), Patch: f.diffContent, Files: []audit.ChangedFile{{Path: "internal/service.go", Status: "modified", SHA256: model.SHA256([]byte(f.sourceContent)), Symbols: []string{"Serve"}}}},
		Verification: audit.VerificationEvidence{ID: f.verificationID, Purpose: verification.PurposeFinal, Status: verification.RunPassed, Source: verification.SourceIdentity{Commit: f.source.Commit, Tree: f.source.Tree}, CompletedAt: f.now.Add(time.Minute), Checks: []audit.VerificationCheck{{ID: f.checkID, Tier: verification.TierFinal, Outcome: verification.OutcomePassed, ExecutionFingerprint: strings.Repeat("8", 64)}}, Artifact: f.artifactIdentity(uuid.NewString(), strings.Repeat("f", 64), "verification_report", "audit/verification.json")},
		BlastRadius:  []audit.BlastRadiusEdge{}, RelevantSource: []audit.SourceFile{f.sourceFile()}, PriorFindings: []audit.PriorFinding{},
	}
	policy, err := audit.PinModelPolicy("gpt-fixture", "high", 4096, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	auditID, auditorID := uuid.NewString(), "auditor-"+uuid.NewString()
	mutatingInvocations := []string{"implementer-" + uuid.NewString()}
	prepared, err := audit.Prepare(audit.Config{AuditID: auditID, AuditorInvocationID: auditorID, SourceMutatingInvocationIDs: mutatingInvocations, Kind: audit.KindBase, Input: input, ModelPolicy: policy, Model: postgresNoopModel{}, StateReader: postgresAuditReader{input: input}})
	if err != nil {
		t.Fatal(err)
	}
	output := audit.Output{RevolvrIdentity: prepared.OutputIdentity, SchemaVersion: audit.OutputSchemaVersion, Authority: prepared.Authority, Disposition: disposition, Rationale: "Independent exact evidence review completed.", Findings: findings}
	canonical, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	modelResult := model.Result{Outcome: model.OutcomeSuccess, Request: prepared.ExpectedRequest, StructuredOutput: append(json.RawMessage(nil), canonical...)}
	candidate := audit.Candidate{AuditID: auditID, AuditorInvocationID: auditorID, SourceMutatingInvocationIDs: mutatingInvocations, Kind: audit.KindBase, Dossier: prepared.Dossier, PromptVersion: audit.PromptVersion, Prompt: prepared.Prompt, PromptSHA256: prepared.PromptSHA256, ResponseSchema: prepared.Schema, ResponseSchemaSHA256: prepared.SchemaSHA256, ModelPolicy: policy, ModelRequest: prepared.ExpectedRequest, ModelResult: modelResult, RawOutput: append(json.RawMessage(nil), canonical...), CanonicalOutput: canonical, Output: output}
	operationID := "audit-" + uuid.NewString()
	report := evidence.Artifact{ID: uuid.NewString(), Kind: "audit_report", MediaType: "application/json", SHA256: evidence.HashBytes(canonical), SizeBytes: int64(len(canonical)), StoragePath: "audit/report/" + uuid.NewString(), Resolved: true, Content: canonical, Provenance: evidence.Provenance{SchemaVersion: evidence.ArtifactProvenanceSchemaVersion, ProjectID: f.identity.ProjectID, TaskID: f.identity.TaskID, TaskVersionID: f.identity.TaskVersionID, RunID: f.identity.RunID, WorkspaceID: f.identity.WorkspaceID, ProducerRole: "auditor", ProducingOperationID: operationID, SourceCommit: f.source.Commit, SourceTree: f.source.Tree}}
	return audit.PersistCommand{OperationID: operationID, Candidate: candidate, Report: report, StartedAt: startedAt, CompletedAt: startedAt.Add(time.Minute)}
}

func (f *auditCorrectionPostgresFixture) finding(id, correction string) audit.Finding {
	return audit.Finding{ID: id, Significance: audit.SignificanceBlocking, Summary: "The cited boundary lacks required evidence.", RequiredCorrection: correction, SourceEvidence: []audit.Citation{{ArtifactID: "source-service", Path: "internal/service.go", SHA256: model.SHA256([]byte(f.sourceContent)), StartLine: 10, EndLine: 12}}, AffectedFiles: []string{"internal/service.go"}, AffectedSymbols: []string{"Serve"}, CriterionImpact: []audit.CriterionImpact{{CriterionID: "AC-1", Impact: audit.ImpactViolated, Detail: "The exact acceptance criterion is not satisfied."}}}
}

func (f *auditCorrectionPostgresFixture) sourceFile() audit.SourceFile {
	return audit.SourceFile{Path: "internal/service.go", SHA256: model.SHA256([]byte(f.sourceContent)), SizeBytes: int64(len(f.sourceContent)), Symbols: []string{"Serve"}, ArtifactID: "source-service", Content: f.sourceContent}
}

func (f *auditCorrectionPostgresFixture) artifactIdentity(id, sha, kind, path string) audit.ArtifactIdentity {
	return audit.ArtifactIdentity{ID: id, SHA256: sha, SizeBytes: 1, MediaType: "application/json", LogicalKind: kind, StoragePath: path}
}

func (f *auditCorrectionPostgresFixture) diffArtifactIdentity() audit.ArtifactIdentity {
	return audit.ArtifactIdentity{ID: f.diffArtifactID, SHA256: f.source.DiffSHA256, SizeBytes: int64(len(f.diffContent)), MediaType: "text/x-diff", LogicalKind: "diff", StoragePath: "audit/diff.patch"}
}

func (f *auditCorrectionPostgresFixture) resolution(findingID, auditID string) audit.DispositionCommand {
	return audit.DispositionCommand{ID: uuid.NewString(), OperationID: "resolve-" + uuid.NewString(), TaskID: f.identity.TaskID, FindingID: findingID, Status: audit.FindingResolved, AuthorityRole: "host", AuthorityID: "corrector-" + uuid.NewString(), ResolutionVerificationRunID: f.verificationID, ResolutionAuditRunID: auditID, SourceCommit: f.source.Commit, SourceTree: f.source.Tree, Evidence: []audit.DispositionEvidence{{Kind: "artifact", ID: "correction-report", SHA256: strings.Repeat("7", 64), Reference: "correction/report.json"}}, CreatedAt: f.now.Add(7 * time.Minute)}
}

func (f *auditCorrectionPostgresFixture) waiver(findingID string) audit.DispositionCommand {
	return audit.DispositionCommand{ID: uuid.NewString(), OperationID: "waive-" + uuid.NewString(), TaskID: f.identity.TaskID, FindingID: findingID, Status: audit.FindingWaived, AuthorityRole: "operator", AuthorityID: "operator-" + uuid.NewString(), SourceCommit: f.source.Commit, SourceTree: f.source.Tree, Evidence: []audit.DispositionEvidence{{Kind: "artifact", ID: "operator-decision", SHA256: strings.Repeat("6", 64), Reference: "operator/decision.json"}}, Rationale: "Operator accepted the documented bounded risk.", CreatedAt: f.now.Add(7 * time.Minute)}
}

func (f *auditCorrectionPostgresFixture) insertArtifact(t *testing.T, id pgtype.UUID, sha, kind, path string) {
	t.Helper()
	f.insertArtifactWithMetadata(t, id, sha, 1, "application/json", kind, path)
}

func (f *auditCorrectionPostgresFixture) insertArtifactWithMetadata(t *testing.T, id pgtype.UUID, sha string, size int64, mediaType, kind, path string) {
	t.Helper()
	if _, err := storage.New(f.pool).InsertArtifact(f.ctx, storage.InsertArtifactParams{ID: id, Sha256: sha, SizeBytes: size, MediaType: mediaType, LogicalKind: kind, StoragePath: path, CreatedAt: dbTime(f.now)}); err != nil {
		t.Fatal(err)
	}
}

func (f *auditCorrectionPostgresFixture) assertCount(t *testing.T, table, column, value string, want int) {
	t.Helper()
	var count int
	query := "SELECT count(*) FROM " + table + " WHERE " + column + "=$1"
	if err := f.pool.QueryRow(f.ctx, query, value).Scan(&count); err != nil || count != want {
		t.Fatalf("%s count = %d, want %d: %v", table, count, want, err)
	}
}

func (f *auditCorrectionPostgresFixture) assertTaskDispositionCount(t *testing.T, want int) {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM core.finding_dispositions WHERE task_id=$1`, f.identity.TaskID).Scan(&count); err != nil || count != want {
		t.Fatalf("finding disposition count = %d, want %d: %v", count, want, err)
	}
}

type postgresNoopModel struct{}

func (postgresNoopModel) Invoke(context.Context, model.PreparedRequest) (model.Result, error) {
	return model.Result{}, errors.New("not invoked")
}

type postgresAuditReader struct{ input audit.DossierInput }

func (r postgresAuditReader) ReadAuditState(context.Context, audit.Identity) (audit.DossierInput, error) {
	return r.input, nil
}

func dbUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func dbTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
