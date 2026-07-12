package autonomousauditapply

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/supervisor"
)

func TestApplyAuditResultReopensForDossierAndPolicy(t *testing.T) {
	fixture := newAuditApplyFixture(t, autonomous.AuditDispositionChangesRequired)
	result, err := ApplyAuditResult(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != DispositionUpdated || result.AuditRevision != 1 || result.State.Lifecycle != autonomous.LifecycleStateReady || len(result.NewFindingIDs) != 1 || result.Resolutions.Open != 1 || !reflect.DeepEqual(result.PolicyEvidence.Report, fixture.output.Report) {
		t.Fatalf("result=%+v", result)
	}
	reopenedStore, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
	reopened, found, err := reopenedStore.LoadCurrentAudit(context.Background(), "task-1")
	if err != nil || !found || !reflect.DeepEqual(reopened.PolicyEvidence, result.PolicyEvidence) {
		t.Fatalf("reopened=%+v found=%t err=%v", reopened, found, err)
	}
	dossier, err := autonomous.BuildTaskDossier(autonomous.TaskDossierInput{TaskID: "task-1", TaskSpec: autonomous.TaskSpecSource{ID: "task", Path: ".agent/tasks/task-1.md", Label: "Task", Content: fixture.taskRaw}, State: reopened.State.State, Audit: &reopened.Report, RecentRunLimit: 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"finding-one", "Current resolution: open", "changes_required"} {
		if !strings.Contains(string(dossier.Markdown), want) {
			t.Fatalf("dossier missing %q", want)
		}
	}
	correct, reference := policyDecision(autonomous.ActionCorrect)
	correct.FindingIDs = []string{"finding-one"}
	reference.WorkerProfile = autonomous.WorkerProfileCorrector
	reference.Action = autonomous.ActionCorrect
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: "task-1", Decision: correct, Reference: reference, State: reopened.State.State, Source: autonomouspolicy.SourceEvidence{Revision: fixture.revision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: fixture.mutation}, Verification: &fixture.verification, Audit: &reopened.PolicyEvidence})
	if err != nil || route.Action != autonomous.ActionCorrect {
		t.Fatalf("correction route=%+v err=%v", route, err)
	}
	replay, err := ApplyAuditResult(context.Background(), fixture.cfg)
	if err != nil || replay.Disposition != DispositionReplayed || replay.History.SourcePath != result.History.SourcePath {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}

func TestApplyCleanAuditSupportsCompletionAndOpenFindingCannotDisappear(t *testing.T) {
	fixture := newAuditApplyFixture(t, autonomous.AuditDispositionClean)
	fixture.state.Plan = completedAuditPlan()
	fixture.state.AcceptanceCriteria = []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "Behavior works.", Status: autonomous.AcceptanceStatusSatisfied, Evidence: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindVerification, "verify")}}}
	fixture.rebuild(t)
	result, err := ApplyAuditResult(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	decision, reference := policyDecision(autonomous.ActionComplete)
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: "task-1", Decision: decision, Reference: reference, State: result.State, Source: autonomouspolicy.SourceEvidence{Revision: fixture.revision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: fixture.mutation}, Verification: &fixture.verification, Audit: &result.PolicyEvidence})
	if err != nil || route.Kind != autonomouspolicy.RouteKindComplete {
		t.Fatalf("complete=%+v err=%v", route, err)
	}

	changes := newAuditApplyFixture(t, autonomous.AuditDispositionChangesRequired)
	first, err := ApplyAuditResult(context.Background(), changes.cfg)
	if err != nil {
		t.Fatal(err)
	}
	changes.cfg.Expected = first.Current.Expected()
	changes.state = first.State
	changes.cfg.OperationID = "audit-operation-two"
	changes.output = changesOutput(changes.output, autonomous.AuditDispositionClean)
	changes.rebuild(t)
	if _, err := ApplyAuditResult(context.Background(), changes.cfg); err == nil || !strings.Contains(err.Error(), "disappeared") {
		t.Fatalf("clean with open finding error=%v", err)
	}
}

func TestApplyFindingResolutionPersistsEveryTerminalStatus(t *testing.T) {
	statuses := []autonomous.FindingResolutionStatus{autonomous.FindingResolutionStatusResolved, autonomous.FindingResolutionStatusWaived, autonomous.FindingResolutionStatusInvalid}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			fixture := newAuditApplyFixture(t, autonomous.AuditDispositionChangesRequired)
			auditResult, err := ApplyAuditResult(context.Background(), fixture.cfg)
			if err != nil {
				t.Fatal(err)
			}
			request := autonomousaudit.ResolutionRequest{FindingID: "finding-one", Status: status, Evidence: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindVerification, "resolution-proof")}}
			if status != autonomous.FindingResolutionStatusResolved {
				request.Rationale = "Explicit durable rationale."
			}
			resolutionConfig := ResolutionConfig{RepositoryRoot: fixture.repo, TaskID: "task-1", OperationID: "resolution-operation", Expected: auditResult.Current.Expected(), AuditRevision: auditResult.AuditRevision, Request: request, CreatedAt: fixture.now.Add(time.Hour)}
			result, err := ApplyFindingResolution(context.Background(), resolutionConfig)
			if err != nil {
				t.Fatal(err)
			}
			if result.Resolutions.Open != 0 || result.AuditRevision != 1 || result.State.FindingResolutions[0].Status != status {
				t.Fatalf("result=%+v", result)
			}
			reopened, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
			audit, found, err := reopened.LoadCurrentAudit(context.Background(), "task-1")
			if err != nil || !found || audit.State.State.FindingResolutions[0].Status != status {
				t.Fatalf("reopened=%+v found=%t err=%v", audit, found, err)
			}
			replay, err := ApplyFindingResolution(context.Background(), resolutionConfig)
			if err != nil || replay.Disposition != DispositionReplayed || replay.History.SourcePath != result.History.SourcePath {
				t.Fatalf("resolution replay=%+v err=%v", replay, err)
			}
		})
	}

	t.Run("superseded", func(t *testing.T) {
		fixture := newAuditApplyFixture(t, autonomous.AuditDispositionChangesRequired)
		fixture.output.Report.Findings = append(fixture.output.Report.Findings, auditApplyFinding("finding-two"))
		fixture.rebuild(t)
		auditResult, err := ApplyAuditResult(context.Background(), fixture.cfg)
		if err != nil {
			t.Fatal(err)
		}
		request := autonomousaudit.ResolutionRequest{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusSuperseded, Evidence: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindAudit, "replacement")}, SupersedingFindingID: "finding-two"}
		result, err := ApplyFindingResolution(context.Background(), ResolutionConfig{RepositoryRoot: fixture.repo, TaskID: "task-1", OperationID: "resolution-operation", Expected: auditResult.Current.Expected(), AuditRevision: 1, Request: request, CreatedAt: fixture.now.Add(time.Hour)})
		if err != nil || result.Resolutions.Superseded != 1 || result.Resolutions.Open != 1 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	})
}

func TestApplyAuditRejectsFailedMutatingStaleAndNonIndependentEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*auditApplyFixture)
		want   string
	}{{"worker failure", func(f *auditApplyFixture) { f.cfg.Cycle.Outcome = autonomouscycle.OutcomeWorkerFailed }, "cycle_evidence"}, {"mutation", func(f *auditApplyFixture) { f.cfg.Cycle.Source.WorkerDifference.Changed = true }, "cycle_evidence"}, {"stale verification", func(f *auditApplyFixture) { f.cfg.Verification.SourceRevision = strings.Repeat("9", 64) }, "cycle_evidence"}, {"auditor equals verification", func(f *auditApplyFixture) { f.cfg.Verification.Summary.RunID = f.cfg.Cycle.Worker.RunID }, "cycle_evidence"}, {"auditor equals mutation", func(f *auditApplyFixture) { f.cfg.LatestMutation.RunID = f.cfg.Cycle.Worker.RunID }, "cycle_evidence"}, {"malformed output", func(f *auditApplyFixture) { f.replaceRaw(t, []byte(`{"bad":true}`)) }, "auditor_output"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuditApplyFixture(t, autonomous.AuditDispositionClean)
			tt.mutate(f)
			result, err := ApplyAuditResult(context.Background(), f.cfg)
			if err == nil || result.Failure == nil || !strings.Contains(result.Failure.Stage, tt.want) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestApplyAuditRetryReconcilesIdenticalOrphan(t *testing.T) {
	fixture := newAuditApplyFixture(t, autonomous.AuditDispositionChangesRequired)
	fired := false
	failing, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo, FailureInjector: func(point autonomousstate.FailurePoint) error {
		if !fired && point == autonomousstate.FailureAfterAuditHistory {
			fired = true
			return fmt.Errorf("crash")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.cfg.Store = failing
	if _, err := ApplyAuditResult(context.Background(), fixture.cfg); err == nil || !fired {
		t.Fatalf("failure=%v fired=%t", err, fired)
	}
	reopened, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
	fixture.cfg.Store = reopened
	result, err := ApplyAuditResult(context.Background(), fixture.cfg)
	if err != nil || result.Disposition != DispositionUpdated {
		t.Fatalf("retry=%+v err=%v", result, err)
	}
	history, err := reopened.LoadAuditHistory(context.Background(), "task-1")
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

type auditApplyFixture struct {
	repo         string
	taskRaw      []byte
	state        autonomous.ExecutionState
	output       autonomousaudit.AuditOutput
	verification autonomouspolicy.VerificationEvidence
	mutation     *autonomouspolicy.SourceMutation
	revision     string
	now          time.Time
	cfg          ApplyConfig
}

func newAuditApplyFixture(t *testing.T, disposition autonomous.AuditDisposition) *auditApplyFixture {
	t.Helper()
	f := &auditApplyFixture{repo: t.TempDir(), revision: strings.Repeat("1", 64), now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
	f.taskRaw = []byte("---\nid: task-1\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/task-1/state.json\n---\n# Task\n\nBehavior works.\n")
	writeAuditApplyFile(t, filepath.Join(f.repo, ".agent/tasks/task-1.md"), f.taskRaw)
	writeAuditApplyFile(t, filepath.Join(f.repo, ".agent/profiles/auditor.md"), []byte("auditor fixture\n"))
	f.state = readyAuditState()
	stateRaw, _ := autonomousstate.MarshalState(f.state)
	writeAuditApplyFile(t, filepath.Join(f.repo, ".revolvr/autonomous/tasks/task-1/state.json"), stateRaw)
	f.verification = autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: "task-1", Status: autonomous.VerificationStatusPassed, Summary: "passed", RunID: "verification-run", OccurrenceID: "occurrence-one", Evidence: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindVerification, "verification-run:occurrence-one")}}, SourceRevision: f.revision}
	f.mutation = &autonomouspolicy.SourceMutation{TaskID: "task-1", RunID: "worker-run", DecisionID: "decision-worker", Action: autonomous.ActionImplement, ResultingRevision: f.revision}
	f.output = auditApplyOutput(f, disposition)
	f.cfg = ApplyConfig{RepositoryRoot: f.repo, TaskID: "task-1", OperationID: "audit-operation-one", Expected: autonomousstate.ExpectedState{Exists: true, SHA256: hashBytes(stateRaw), ByteSize: len(stateRaw)}, Verification: f.verification, LatestMutation: f.mutation, CreatedAt: f.now}
	f.rebuild(t)
	return f
}
func (f *auditApplyFixture) rebuild(t *testing.T) {
	t.Helper()
	stateRaw, _ := autonomousstate.MarshalState(f.state)
	writeAuditApplyFile(t, filepath.Join(f.repo, ".revolvr/autonomous/tasks/task-1/state.json"), stateRaw)
	f.cfg.Expected = autonomousstate.ExpectedState{Exists: true, SHA256: hashBytes(stateRaw), ByteSize: len(stateRaw)}
	profileRaw, _ := os.ReadFile(filepath.Join(f.repo, ".agent/profiles/auditor.md"))
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Rationale: "audit", SuccessCriteria: []string{"report"}, Inputs: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindVerification, "verification-run:occurrence-one")}}
	reference := autonomous.DecisionReference{DecisionID: "decision-audit", RunID: "supervisor-run", TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: applyAuditEvidence(autonomous.EvidenceKindFile, ".revolvr/runs/supervisor-run/supervisor-decision.json"), CreatedAt: f.now}
	decisionRaw, _ := json.MarshalIndent(decision, "", "  ")
	decisionRaw = append(decisionRaw, '\n')
	writeAuditApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(reference.Artifact.Reference)), decisionRaw)
	schema, _ := autonomousaudit.AuditOutputSchema()
	schemaPath := ".revolvr/runs/auditor-run/auditor-output-schema.json"
	writeAuditApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(schemaPath)), schema)
	f.output.Provenance.Decision = reference
	f.output.Provenance.Dossier = autonomousaudit.DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", SHA256: strings.Repeat("2", 64), ByteSize: 100}
	f.output.Provenance.Profile = autonomousaudit.ProfileIdentity{Name: autonomous.WorkerProfileAuditor, Path: ".agent/profiles/auditor.md", SHA256: hashBytes(profileRaw), ByteSize: len(profileRaw)}
	f.output.Provenance.Verification = f.cfg.Verification
	f.output.Provenance.LatestSourceMutation = autonomousaudit.SourceMutationFromPolicy(f.cfg.LatestMutation)
	raw, _ := autonomousaudit.MarshalAuditOutput(f.output)
	rawPath := ".revolvr/runs/auditor-run/auditor-output.raw.json"
	writeAuditApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(rawPath)), raw)
	snapshot := auditApplySnapshot()
	stateCompact, _ := json.Marshal(f.state)
	manifest := autonomous.TaskDossierManifest{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", DossierSHA256: strings.Repeat("2", 64), DossierByteSize: 100, Sources: []autonomous.DossierSourceRecord{{Kind: autonomous.DossierSourceKindTaskSpec, Path: ".agent/tasks/task-1.md", SHA256: hashBytes(f.taskRaw), ByteSize: len(f.taskRaw)}, {Kind: autonomous.DossierSourceKindExecutionState, SHA256: hashBytes(stateCompact), ByteSize: len(stateCompact)}}}
	schemaAbs, _ := filepath.Abs(filepath.Join(f.repo, filepath.FromSlash(schemaPath)))
	f.cfg.Cycle = autonomouscycle.Result{TaskID: "task-1", Outcome: autonomouscycle.OutcomeReadOnlyCompleted, DossierManifest: manifest, Supervisor: supervisor.Result{RunID: "supervisor-run", Decision: &decision, DecisionReference: &reference, Artifacts: supervisor.Artifacts{Decision: supervisor.Artifact{Path: reference.Artifact.Reference, SHA256: hashBytes(decisionRaw), ByteSize: len(decisionRaw)}}}, Route: &autonomouspolicy.Route{Kind: autonomouspolicy.RouteKindWorker, TaskID: "task-1", DecisionID: "decision-audit", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, SourceRevision: f.revision}, Worker: autonomouscycle.WorkerEvidence{Started: true, RunID: "auditor-run", Run: ledger.Run{ID: "auditor-run", TaskID: "task-1", Status: ledger.StatusCompleted}, Action: autonomous.ActionAudit, Profile: autonomouscycle.ProfileEvidence{Name: "auditor", Path: ".agent/profiles/auditor.md", SHA256: hashBytes(profileRaw), ByteSize: len(profileRaw)}, Invocation: codexexec.InvocationProvenance{Executable: "fake", Version: "test", Model: codexexec.DefaultModel, ReasoningEffort: codexexec.DefaultReasoningEffort, Ephemeral: true, SessionMode: codexexec.SessionModeEphemeral, EffectiveConfigSchema: "config-v1", EffectiveConfigSHA256: strings.Repeat("3", 64), Argv: []string{"exec", "--output-schema", schemaAbs, "--ephemeral", "-"}, WorkingDir: f.repo}, Artifacts: autonomouscycle.WorkerArtifacts{OutputSchema: &autonomouscycle.Artifact{Path: schemaPath, SHA256: hashBytes(schema), ByteSize: len(schema)}, Output: autonomouscycle.Artifact{Path: rawPath, SHA256: hashBytes(raw), ByteSize: len(raw)}}, Codex: codexexec.Result{ExitCode: 0}, RawOutput: raw}, Source: autonomouscycle.SourceEvidence{Admission: &snapshot, WorkerAfter: &snapshot, AdmissionRevision: f.revision, WorkerRevision: f.revision}}
	f.cfg.Cycle.Supervisor.Dossier = supervisor.DossierProvenance{SchemaVersion: manifest.SchemaVersion, TaskID: "task-1", SHA256: manifest.DossierSHA256, ByteSize: manifest.DossierByteSize}
}
func (f *auditApplyFixture) replaceRaw(t *testing.T, raw []byte) {
	path := f.cfg.Cycle.Worker.Artifacts.Output.Path
	writeAuditApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(path)), raw)
	f.cfg.Cycle.Worker.Artifacts.Output.SHA256 = hashBytes(raw)
	f.cfg.Cycle.Worker.Artifacts.Output.ByteSize = len(raw)
	f.cfg.Cycle.Worker.RawOutput = raw
}
func auditApplyOutput(f *auditApplyFixture, d autonomous.AuditDisposition) autonomousaudit.AuditOutput {
	report := autonomous.AuditReport{TaskID: "task-1", Disposition: d, Rationale: "clean", Inputs: append([]autonomous.EvidenceReference(nil), f.verification.Summary.Evidence...)}
	if d == autonomous.AuditDispositionChangesRequired {
		report.Rationale = "changes"
		report.Findings = []autonomous.AuditFinding{auditApplyFinding("finding-one")}
	}
	return autonomousaudit.AuditOutput{SchemaVersion: autonomousaudit.AuditOutputSchemaVersion, TaskID: "task-1", Report: report, Provenance: autonomousaudit.AuditProvenance{Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, WorkerRunID: "auditor-run", RawOutputPath: ".revolvr/runs/auditor-run/auditor-output.raw.json", SourceRevision: f.revision, Verification: f.verification, LatestSourceMutation: autonomousaudit.SourceMutationFromPolicy(f.mutation)}}
}
func changesOutput(o autonomousaudit.AuditOutput, d autonomous.AuditDisposition) autonomousaudit.AuditOutput {
	o.Report.Disposition = d
	o.Report.Rationale = "clean"
	o.Report.Findings = nil
	return o
}
func auditApplyFinding(id string) autonomous.AuditFinding {
	return autonomous.AuditFinding{ID: id, Significance: autonomous.FindingSignificanceBlocking, Summary: "defect remains", Evidence: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindFile, "example.go")}, RequiredCorrection: "fix defect"}
}
func readyAuditState() autonomous.ExecutionState {
	return autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-1", Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
}
func completedAuditPlan() *autonomous.TaskPlan {
	return &autonomous.TaskPlan{TaskID: "task-1", ID: "plan-one", Revision: 1, Completed: true, Provenance: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindTask, "task")}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "work", Status: autonomous.PlanStepStatusCompleted, Evidence: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindVerification, "verify")}}}}
}
func policyDecision(action autonomous.Action) (autonomous.SupervisorDecision, autonomous.DecisionReference) {
	profile := autonomous.WorkerProfile("")
	if action == autonomous.ActionCorrect {
		profile = autonomous.WorkerProfileCorrector
	}
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: action, WorkerProfile: profile, Rationale: "route", Inputs: []autonomous.EvidenceReference{applyAuditEvidence(autonomous.EvidenceKindTask, "task")}}
	if profile != "" {
		decision.SuccessCriteria = []string{"correct"}
	}
	reference := autonomous.DecisionReference{DecisionID: "decision-policy", RunID: "policy-run", TaskID: "task-1", Action: action, WorkerProfile: profile, Artifact: applyAuditEvidence(autonomous.EvidenceKindFile, "policy-decision"), CreatedAt: time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)}
	return decision, reference
}
func auditApplySnapshot() gitstate.SourceSnapshot {
	entries := []gitstate.SourceEntry{}
	index := sha256.Sum256(nil)
	workRaw, _ := json.Marshal(entries)
	work := sha256.Sum256(workRaw)
	s := gitstate.SourceSnapshot{SchemaVersion: gitstate.SourceSnapshotSchemaVersion, Head: "head", IndexSHA256: fmt.Sprintf("%x", index), WorktreeSHA256: fmt.Sprintf("%x", work), Entries: entries}
	raw, _ := json.Marshal(struct {
		SchemaVersion  string                 `json:"schema_version"`
		Head           string                 `json:"head"`
		IndexSHA256    string                 `json:"index_sha256"`
		WorktreeSHA256 string                 `json:"worktree_sha256"`
		Entries        []gitstate.SourceEntry `json:"entries"`
	}{s.SchemaVersion, s.Head, s.IndexSHA256, s.WorktreeSHA256, s.Entries})
	sum := sha256.Sum256(raw)
	s.SnapshotSHA256 = fmt.Sprintf("%x", sum)
	return s
}
func applyAuditEvidence(k autonomous.EvidenceKind, r string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: k, Reference: r, Detail: "exact test evidence"}
}
func writeAuditApplyFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
