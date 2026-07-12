package autonomousplanapply

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
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/supervisor"
)

func TestApplyPlanningResultPersistsInitialPlanReopensAndAuthorizesImplement(t *testing.T) {
	fixture := newApplyFixture(t, nil, "one")
	result, err := ApplyPlanningResult(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != DispositionCreated || result.State.Lifecycle != autonomous.LifecycleStateReady || result.CurrentPlan != (autonomousstate.PlanIdentity{ID: "plan-one", Revision: 1}) {
		t.Fatalf("apply result = %+v", result)
	}
	if result.State.LatestDecision == nil || result.State.LatestDecision.DecisionID != fixture.decisionReference.DecisionID || result.Acceptance.Pending != 1 {
		t.Fatalf("state decision/acceptance = %+v / %+v", result.State.LatestDecision, result.Acceptance)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
	if err != nil {
		t.Fatal(err)
	}
	reopened, found, err := store.Load(context.Background(), "task-1")
	if err != nil || !found || reopened.SHA256 != result.Current.SHA256 || !reflect.DeepEqual(reopened.State, result.State) {
		t.Fatalf("reopened state = %+v, %t, %v", reopened, found, err)
	}
	rawOutput, err := os.ReadFile(filepath.Join(fixture.repo, filepath.FromSlash(result.Planner.RawOutput.Path)))
	if err != nil {
		t.Fatal(err)
	}
	canonicalOutput, err := os.ReadFile(filepath.Join(fixture.repo, filepath.FromSlash(result.Planner.CanonicalOutput.Path)))
	if err != nil || result.Planner.RawOutput.Path == result.Planner.CanonicalOutput.Path || len(rawOutput) == 0 || len(canonicalOutput) == 0 || !strings.HasSuffix(result.Planner.CanonicalOutput.Path, "planner-output.canonical.json") {
		t.Fatalf("raw/canonical evidence not distinct: raw=%q canonical=%q error=%v", result.Planner.RawOutput.Path, result.Planner.CanonicalOutput.Path, err)
	}

	implementDecision, implementReference := applyPolicyDecision(autonomous.ActionImplement)
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{
		TaskID: "task-1", Decision: implementDecision, Reference: implementReference,
		State:  reopened.State,
		Source: autonomouspolicy.SourceEvidence{Revision: strings.Repeat("9", 64), Safety: autonomouspolicy.SourceSafetySafe},
	})
	if err != nil || route.Kind != autonomouspolicy.RouteKindWorker || route.WorkerProfile != autonomous.WorkerProfileImplementer {
		t.Fatalf("reopened implement route = %+v, %v", route, err)
	}
}

func TestApplyPlanningResultIsIdempotentAndRecoversOrphanedHistory(t *testing.T) {
	t.Run("committed replay", func(t *testing.T) {
		fixture := newApplyFixture(t, nil, "one")
		first, err := ApplyPlanningResult(context.Background(), fixture.cfg)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := ApplyPlanningResult(context.Background(), fixture.cfg)
		if err != nil || replay.Disposition != DispositionReplayed || replay.History.SourcePath != first.History.SourcePath || replay.Current.SHA256 != first.Current.SHA256 {
			t.Fatalf("replay = %+v, %v", replay, err)
		}
		fixture.output.Plan.Steps[0].Description = "Materially different work."
		fixture.rewriteOutput(t)
		conflict, err := ApplyPlanningResult(context.Background(), fixture.cfg)
		if err == nil || conflict.Failure == nil || conflict.Failure.Stage != "operation_conflict" {
			t.Fatalf("operation conflict = %+v, %v", conflict, err)
		}
	})

	t.Run("orphaned history", func(t *testing.T) {
		fixture := newApplyFixture(t, nil, "one")
		failed := false
		store, err := autonomousstate.New(autonomousstate.Config{
			RepositoryRoot: fixture.repo,
			FailureInjector: func(point autonomousstate.FailurePoint) error {
				if !failed && point == autonomousstate.FailureAfterHistoryWrite {
					failed = true
					return fmt.Errorf("crash")
				}
				return nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		fixture.cfg.Store = store
		if _, err := ApplyPlanningResult(context.Background(), fixture.cfg); err == nil || !failed {
			t.Fatalf("first apply error=%v failed=%t", err, failed)
		}
		fixture.cfg.Store = nil
		recovered, err := ApplyPlanningResult(context.Background(), fixture.cfg)
		if err != nil || recovered.Disposition != DispositionCreated {
			t.Fatalf("recovered apply = %+v, %v", recovered, err)
		}
	})
}

func TestApplyPlanningResultPersistsDeliberateRevisionAndPreservesEvidence(t *testing.T) {
	initial := newApplyFixture(t, nil, "one")
	first, err := ApplyPlanningResult(context.Background(), initial.cfg)
	if err != nil {
		t.Fatal(err)
	}
	revisionState := first.State
	revisionState.Plan.Steps[0].Status = autonomous.PlanStepStatusCompleted
	revisionState.Plan.Steps[0].Evidence = []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindVerification, "verification-one")}
	stateBytes, err := autonomousstate.MarshalState(revisionState)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(initial.repo, filepath.FromSlash(first.StatePath)), stateBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	store, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: initial.repo})
	snapshot, found, err := store.Load(context.Background(), "task-1")
	if err != nil || !found {
		t.Fatal(err)
	}
	revision := newApplyFixture(t, &applyFixtureSeed{repo: initial.repo, state: snapshot.State, expected: snapshot.Expected()}, "two")
	revision.output.Plan.ID = "plan-two"
	revision.output.Plan.Revision = 2
	revision.output.Plan.SupersedesPlanID = "plan-one"
	revision.output.Plan.Steps = []autonomous.PlanStep{
		snapshot.State.Plan.Steps[0],
		{ID: "step-two", Description: "Run repository verification.", Status: autonomous.PlanStepStatusPending},
	}
	revision.output.AcceptanceCriteria = cloneApplyCriteria(snapshot.State.AcceptanceCriteria)
	revision.rewriteOutput(t)
	updated, err := ApplyPlanningResult(context.Background(), revision.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Disposition != DispositionUpdated || updated.CurrentPlan.ID != "plan-two" || updated.PreviousPlan == nil || updated.PreviousPlan.ID != "plan-one" {
		t.Fatalf("revision result = %+v", updated)
	}
	if !reflect.DeepEqual(updated.State.Plan.Steps[0], snapshot.State.Plan.Steps[0]) || !reflect.DeepEqual(updated.State.AcceptanceCriteria, snapshot.State.AcceptanceCriteria) {
		t.Fatal("completed work or acceptance evidence disappeared")
	}
	if updated.History.Record.PreviousPlan == nil || updated.History.Record.ResultingPlan.SupersedesPlanID != "plan-one" {
		t.Fatalf("history predecessor = %+v", updated.History.Record)
	}
}

func TestApplyPlanningResultRejectsInvalidCycleOrOutputWithoutStateMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*applyFixture)
		want   string
	}{
		{name: "worker failure", mutate: func(f *applyFixture) {
			f.cfg.Cycle.Outcome = autonomouscycle.OutcomeWorkerFailed
			f.cfg.Cycle.Failure = &autonomouscycle.Failure{Stage: "worker", Reason: "failed"}
		}, want: "cycle_evidence"},
		{name: "source mutation", mutate: func(f *applyFixture) { f.cfg.Cycle.Source.WorkerDifference.Changed = true }, want: "cycle_evidence"},
		{name: "non-plan route", mutate: func(f *applyFixture) { f.cfg.Cycle.Route.Action = autonomous.ActionImplement }, want: "cycle_evidence"},
		{name: "terminal route", mutate: func(f *applyFixture) { f.cfg.Cycle.Route.Kind = autonomouspolicy.RouteKindComplete }, want: "cycle_evidence"},
		{name: "wrong profile", mutate: func(f *applyFixture) { f.cfg.Cycle.Worker.Profile.Name = "auditor" }, want: "cycle_evidence"},
		{name: "same runs", mutate: func(f *applyFixture) { f.cfg.Cycle.Worker.RunID = f.cfg.Cycle.Supervisor.RunID }, want: "cycle_evidence"},
		{name: "missing output", mutate: func(f *applyFixture) {
			_ = os.Remove(filepath.Join(f.repo, filepath.FromSlash(f.cfg.Cycle.Worker.Artifacts.Output.Path)))
		}, want: "planner_raw_output"},
		{name: "malformed output", mutate: func(f *applyFixture) { f.replaceRaw(t, []byte(`{"schema_version":`)) }, want: "planner_output"},
		{name: "wrong task output", mutate: func(f *applyFixture) {
			f.output.TaskID = "other"
			f.output.Plan.TaskID = "other"
			f.output.Provenance.Decision.TaskID = "other"
			f.output.Provenance.Dossier.TaskID = "other"
			raw, _ := autonomousplanning.MarshalPlanningOutput(f.output)
			f.replaceRaw(t, raw)
		}, want: "planner_output_identity"},
		{name: "wrong decision", mutate: func(f *applyFixture) {
			f.output.Provenance.Decision.DecisionID = "decision-other"
			raw, _ := autonomousplanning.MarshalPlanningOutput(f.output)
			f.replaceRaw(t, raw)
		}, want: "planner_output_identity"},
		{name: "wrong worker", mutate: func(f *applyFixture) {
			f.output.Provenance.WorkerRunID = "worker-other"
			raw, _ := autonomousplanning.MarshalPlanningOutput(f.output)
			f.replaceRaw(t, raw)
		}, want: "planner_output_identity"},
		{name: "wrong raw artifact hash", mutate: func(f *applyFixture) { f.cfg.Cycle.Worker.Artifacts.Output.SHA256 = strings.Repeat("f", 64) }, want: "planner_raw_output"},
		{name: "wrong dossier state", mutate: func(f *applyFixture) { f.cfg.Cycle.DossierManifest.Sources[1].SHA256 = strings.Repeat("f", 64) }, want: "dossier_identity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newApplyFixture(t, nil, "one")
			tt.mutate(fixture)
			result, err := ApplyPlanningResult(context.Background(), fixture.cfg)
			if err == nil || result.Failure == nil || !strings.Contains(result.Failure.Stage, tt.want) {
				t.Fatalf("result=%+v error=%v, want stage %q", result, err, tt.want)
			}
			if _, statErr := os.Stat(filepath.Join(fixture.repo, ".revolvr", "autonomous", "tasks", "task-1", "state.json")); !os.IsNotExist(statErr) {
				t.Fatalf("rejected apply created state: %v", statErr)
			}
		})
	}
}

func TestReopenedMatrixRetainsRationaleAndCompletionPolicyRemainsPure(t *testing.T) {
	fixture := newApplyFixture(t, nil, "one")
	source := *fixture.output.AcceptanceCriteria[0].Source
	fixture.output.AcceptanceCriteria = []autonomous.AcceptanceCriterion{
		{ID: "criterion-satisfied", Requirement: "Behavior works.", Status: autonomous.AcceptanceStatusSatisfied, Evidence: []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindVerification, "verification-current")}, Source: &source},
		{ID: "criterion-waived", Requirement: "Optional behavior may be waived.", Status: autonomous.AcceptanceStatusWaived, Rationale: "The operator explicitly waived this criterion.", Source: &source},
		{ID: "criterion-na", Requirement: "No unrelated behavior changes.", Status: autonomous.AcceptanceStatusNotApplicable, Rationale: "The unrelated surface is absent.", Source: &source},
	}
	fixture.rewriteOutput(t)
	result, err := ApplyPlanningResult(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.State.AcceptanceCriteria[0].Evidence) == 0 || result.State.AcceptanceCriteria[1].Rationale == "" || result.State.AcceptanceCriteria[2].Rationale == "" {
		t.Fatal("waived/not-applicable rationale was not retained")
	}

	completionState := result.State
	completionState.Plan.Steps[0].Status = autonomous.PlanStepStatusCompleted
	completionState.Plan.Steps[0].Evidence = []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindVerification, "verification-current")}
	completionState.Plan.Completed = true
	decision, reference := applyPolicyDecision(autonomous.ActionComplete)
	verification := applyVerificationEvidence()
	audit := applyCleanAuditEvidence()
	before := mustApplyJSON(t, completionState)
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{
		TaskID: "task-1", Decision: decision, Reference: reference, State: completionState,
		Source:       autonomouspolicy.SourceEvidence{Revision: strings.Repeat("9", 64), Safety: autonomouspolicy.SourceSafetySafe},
		Verification: &verification, Audit: &audit,
	})
	if err != nil || route.Kind != autonomouspolicy.RouteKindComplete {
		t.Fatalf("completion route = %+v, %v", route, err)
	}
	if after := mustApplyJSON(t, completionState); !reflect.DeepEqual(after, before) {
		t.Fatal("policy mutated reopened state")
	}
	pending := completionState
	pending.AcceptanceCriteria = []autonomous.AcceptanceCriterion{{ID: "criterion-pending", Requirement: "pending", Status: autonomous.AcceptanceStatusPending}}
	if _, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: "task-1", Decision: decision, Reference: reference, State: pending, Source: autonomouspolicy.SourceEvidence{Revision: strings.Repeat("9", 64), Safety: autonomouspolicy.SourceSafetySafe}, Verification: &verification, Audit: &audit}); err == nil || !strings.Contains(err.Error(), "pending") {
		t.Fatalf("pending completion error = %v", err)
	}
}

type applyFixtureSeed struct {
	repo     string
	state    autonomous.ExecutionState
	expected autonomousstate.ExpectedState
}

type applyFixture struct {
	repo              string
	taskRaw           []byte
	state             autonomous.ExecutionState
	decision          autonomous.SupervisorDecision
	decisionReference autonomous.DecisionReference
	output            autonomousplanning.PlanningOutput
	cfg               Config
}

func newApplyFixture(t *testing.T, seed *applyFixtureSeed, suffix string) *applyFixture {
	t.Helper()
	repo := ""
	state := applyPendingState()
	expected := autonomousstate.ExpectedState{}
	if seed == nil {
		repo = t.TempDir()
	} else {
		repo, state, expected = seed.repo, seed.state, seed.expected
	}
	taskPath := filepath.Join(repo, ".agent", "tasks", "task-1.md")
	if seed == nil {
		taskRaw := []byte("---\nid: task-1\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/task-1/state.json\n---\n# Task\n\nBehavior works.\n\nOptional behavior may be waived.\n\nNo unrelated behavior changes.\n")
		writeApplyFile(t, taskPath, taskRaw)
		writeApplyFile(t, filepath.Join(repo, ".agent", "profiles", "planner.md"), []byte("planner fixture"))
	}
	taskRaw, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	profileRaw, err := os.ReadFile(filepath.Join(repo, ".agent", "profiles", "planner.md"))
	if err != nil {
		t.Fatal(err)
	}
	supervisorRun := "supervisor-" + suffix
	workerRun := "worker-" + suffix
	decisionPath := filepath.ToSlash(filepath.Join(".revolvr", "runs", supervisorRun, "supervisor-decision.json"))
	decisionArtifact := applyEvidence(autonomous.EvidenceKindFile, decisionPath)
	reference := autonomous.DecisionReference{
		DecisionID: "decision-" + suffix, RunID: supervisorRun, TaskID: "task-1", Action: autonomous.ActionPlan,
		WorkerProfile: autonomous.WorkerProfilePlanner, Artifact: decisionArtifact,
		CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	}
	decision := autonomous.SupervisorDecision{
		TaskID: "task-1", Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner,
		Rationale: "Planning is required.", SuccessCriteria: []string{"Supervisor refinement is retained."},
		Inputs: []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindTask, ".agent/tasks/task-1.md")},
	}
	decisionRaw, _ := json.MarshalIndent(decision, "", "  ")
	decisionRaw = append(decisionRaw, '\n')
	writeApplyFile(t, filepath.Join(repo, filepath.FromSlash(decisionPath)), decisionRaw)
	taskOrigin := autonomousplanning.CanonicalTaskOrigin(".agent/tasks/task-1.md", hashBytes(taskRaw))
	source := taskOrigin
	rawOutputPath := filepath.ToSlash(filepath.Join(".revolvr", "runs", workerRun, "planner-output.raw.json"))
	dossierHash := strings.Repeat("a", 64)
	sourceRevision := strings.Repeat("b", 64)
	output := autonomousplanning.PlanningOutput{
		SchemaVersion: autonomousplanning.PlanningOutputSchemaVersion, TaskID: "task-1",
		Plan: autonomous.TaskPlan{
			TaskID: "task-1", ID: "plan-one", Revision: 1,
			Provenance: []autonomous.EvidenceReference{taskOrigin, decisionArtifact},
			Steps:      []autonomous.PlanStep{{ID: "step-one", Description: "Implement behavior.", Status: autonomous.PlanStepStatusPending}},
		},
		AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "Behavior works.", Status: autonomous.AcceptanceStatusPending, Source: &source}},
		Inputs:             []autonomous.EvidenceReference{taskOrigin, decisionArtifact},
		Provenance: autonomousplanning.PlanningProvenance{
			Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner, WorkerRunID: workerRun, Decision: reference,
			Dossier:       autonomousplanning.DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", SHA256: dossierHash, ByteSize: 123},
			Profile:       autonomousplanning.ProfileIdentity{Name: autonomous.WorkerProfilePlanner, Path: ".agent/profiles/planner.md", SHA256: hashBytes(profileRaw), ByteSize: len(profileRaw)},
			RawOutputPath: rawOutputPath, SourceRevision: sourceRevision,
		},
	}
	fixture := &applyFixture{repo: repo, taskRaw: taskRaw, state: state, decision: decision, decisionReference: reference, output: output}
	fixture.cfg = Config{
		RepositoryRoot: repo, TaskID: "task-1", OperationID: "planning-operation-" + suffix,
		Expected: expected, CreatedAt: time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
	}
	if seed == nil {
		initial := state
		fixture.cfg.InitialState = &initial
	}
	fixture.buildCycle(t)
	return fixture
}

func (f *applyFixture) buildCycle(t *testing.T) {
	t.Helper()
	raw, err := autonomousplanning.MarshalPlanningOutput(f.output)
	if err != nil {
		t.Fatal(err)
	}
	writeApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(f.output.Provenance.RawOutputPath)), raw)
	schema, err := autonomousplanning.PlanningOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.ToSlash(filepath.Join(".revolvr", "runs", f.output.Provenance.WorkerRunID, "planner-output-schema.json"))
	writeApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(schemaPath)), schema)
	snapshot := applySourceSnapshot()
	stateRaw, _ := json.Marshal(f.state)
	dossier := autonomous.TaskDossierManifest{
		SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1",
		DossierSHA256: f.output.Provenance.Dossier.SHA256, DossierByteSize: f.output.Provenance.Dossier.ByteSize,
		Sources: []autonomous.DossierSourceRecord{
			{Kind: autonomous.DossierSourceKindTaskSpec, ID: "task-spec:task-1", Path: ".agent/tasks/task-1.md", SHA256: hashBytes(f.taskRaw), ByteSize: len(f.taskRaw)},
			{Kind: autonomous.DossierSourceKindExecutionState, ID: "execution-state", SHA256: hashBytes(stateRaw), ByteSize: len(stateRaw)},
		},
	}
	decisionRaw, _ := os.ReadFile(filepath.Join(f.repo, filepath.FromSlash(f.decisionReference.Artifact.Reference)))
	profileRaw, _ := os.ReadFile(filepath.Join(f.repo, ".agent", "profiles", "planner.md"))
	rawArtifact := autonomouscycle.Artifact{Path: f.output.Provenance.RawOutputPath, SHA256: hashBytes(raw), ByteSize: len(raw)}
	schemaArtifact := autonomouscycle.Artifact{Path: schemaPath, SHA256: hashBytes(schema), ByteSize: len(schema)}
	schemaAbs, _ := filepath.Abs(filepath.Join(f.repo, filepath.FromSlash(schemaPath)))
	f.cfg.Cycle = autonomouscycle.Result{
		TaskID: "task-1", Outcome: autonomouscycle.OutcomeReadOnlyCompleted, DossierManifest: dossier,
		Supervisor: supervisor.Result{
			RunID: f.decisionReference.RunID, Decision: &f.decision, DecisionReference: &f.decisionReference,
			Artifacts: supervisor.Artifacts{Decision: supervisor.Artifact{Path: f.decisionReference.Artifact.Reference, SHA256: hashBytes(decisionRaw), ByteSize: len(decisionRaw)}},
			Dossier:   supervisor.DossierProvenance{SchemaVersion: dossier.SchemaVersion, TaskID: "task-1", SHA256: dossier.DossierSHA256, ByteSize: dossier.DossierByteSize},
		},
		Route: &autonomouspolicy.Route{Kind: autonomouspolicy.RouteKindWorker, TaskID: "task-1", DecisionID: f.decisionReference.DecisionID, Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner, SourceRevision: f.output.Provenance.SourceRevision},
		Worker: autonomouscycle.WorkerEvidence{
			Started: true, RunID: f.output.Provenance.WorkerRunID,
			Run:     ledger.Run{ID: f.output.Provenance.WorkerRunID, TaskID: "task-1", Status: ledger.StatusCompleted},
			Action:  autonomous.ActionPlan,
			Profile: autonomouscycle.ProfileEvidence{Name: "planner", Path: filepath.Join(".agent", "profiles", "planner.md"), SHA256: hashBytes(profileRaw), ByteSize: len(profileRaw)},
			Invocation: codexexec.InvocationProvenance{
				Executable: "fake-codex", Version: "codex-cli test", Model: codexexec.DefaultModel, ReasoningEffort: codexexec.DefaultReasoningEffort,
				Ephemeral: true, SessionMode: codexexec.SessionModeEphemeral,
				EffectiveConfigSchema: "revolvr-effective-run-config-v1", EffectiveConfigSHA256: strings.Repeat("c", 64),
				Argv: []string{"exec", "--json", "--ephemeral", "--output-schema", schemaAbs, "-"}, WorkingDir: f.repo,
			},
			Artifacts: autonomouscycle.WorkerArtifacts{OutputSchema: &schemaArtifact, Output: rawArtifact},
			Codex:     codexexec.Result{ExitCode: 0, FinalMessage: string(raw)}, RawOutput: raw,
		},
		Source: autonomouscycle.SourceEvidence{
			Admission: &snapshot, WorkerAfter: &snapshot,
			WorkerDifference:  gitstate.CompareSourceSnapshots(snapshot, snapshot),
			AdmissionRevision: f.output.Provenance.SourceRevision, WorkerRevision: f.output.Provenance.SourceRevision,
		},
	}
}

func (f *applyFixture) rewriteOutput(t *testing.T) {
	t.Helper()
	f.output.Provenance.Dossier = autonomousplanning.DossierIdentity{
		SchemaVersion: f.cfg.Cycle.DossierManifest.SchemaVersion, TaskID: "task-1",
		SHA256: f.cfg.Cycle.DossierManifest.DossierSHA256, ByteSize: f.cfg.Cycle.DossierManifest.DossierByteSize,
	}
	f.output.Provenance.Decision = f.decisionReference
	f.output.Provenance.WorkerRunID = f.cfg.Cycle.Worker.RunID
	f.output.Provenance.Profile = autonomousplanning.ProfileIdentity{Name: autonomous.WorkerProfilePlanner, Path: ".agent/profiles/planner.md", SHA256: f.cfg.Cycle.Worker.Profile.SHA256, ByteSize: f.cfg.Cycle.Worker.Profile.ByteSize}
	f.output.Provenance.RawOutputPath = f.cfg.Cycle.Worker.Artifacts.Output.Path
	f.output.Provenance.SourceRevision = f.cfg.Cycle.Source.AdmissionRevision
	raw, err := autonomousplanning.MarshalPlanningOutput(f.output)
	if err != nil {
		t.Fatal(err)
	}
	f.replaceRaw(t, raw)
}

func (f *applyFixture) replaceRaw(t *testing.T, raw []byte) {
	t.Helper()
	path := f.cfg.Cycle.Worker.Artifacts.Output.Path
	writeApplyFile(t, filepath.Join(f.repo, filepath.FromSlash(path)), raw)
	f.cfg.Cycle.Worker.Artifacts.Output.SHA256 = hashBytes(raw)
	f.cfg.Cycle.Worker.Artifacts.Output.ByteSize = len(raw)
	f.cfg.Cycle.Worker.RawOutput = append([]byte(nil), raw...)
	f.cfg.Cycle.Worker.Codex.FinalMessage = string(raw)
}

func applyPendingState() autonomous.ExecutionState {
	return autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-1", Lifecycle: autonomous.LifecycleStatePending,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
}

func applyPolicyDecision(action autonomous.Action) (autonomous.SupervisorDecision, autonomous.DecisionReference) {
	profile := autonomous.WorkerProfile("")
	if action == autonomous.ActionImplement {
		profile = autonomous.WorkerProfileImplementer
	}
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: action, WorkerProfile: profile, Rationale: "Current evidence supports this route.", Inputs: []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindTask, "task")}}
	if profile != "" {
		decision.SuccessCriteria = []string{"Complete the action."}
	}
	reference := autonomous.DecisionReference{DecisionID: "decision-policy-" + string(action), RunID: "run-policy-" + string(action), TaskID: "task-1", Action: action, WorkerProfile: profile, Artifact: applyEvidence(autonomous.EvidenceKindFile, "decision-policy"), CreatedAt: time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)}
	return decision, reference
}

func applyVerificationEvidence() autonomouspolicy.VerificationEvidence {
	return autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: "task-1", Status: autonomous.VerificationStatusPassed, Summary: "passed", RunID: "run-verification", OccurrenceID: "verification-current", Evidence: []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindVerification, "verification-current")}}, SourceRevision: strings.Repeat("9", 64)}
}

func applyCleanAuditEvidence() autonomouspolicy.AuditEvidence {
	return autonomouspolicy.AuditEvidence{Report: autonomous.AuditReport{TaskID: "task-1", Disposition: autonomous.AuditDispositionClean, Rationale: "clean", Inputs: []autonomous.EvidenceReference{applyEvidence(autonomous.EvidenceKindVerification, "verification-current")}}, RunID: "run-audit", AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: strings.Repeat("9", 64), VerificationRunID: "run-verification", VerificationOccurrenceID: "verification-current"}
}

func applySourceSnapshot() gitstate.SourceSnapshot {
	entries := []gitstate.SourceEntry{}
	indexSum := sha256.Sum256(nil)
	worktreeRaw, _ := json.Marshal(entries)
	worktreeSum := sha256.Sum256(worktreeRaw)
	snapshot := gitstate.SourceSnapshot{SchemaVersion: gitstate.SourceSnapshotSchemaVersion, Head: "head-one", IndexSHA256: fmt.Sprintf("%x", indexSum), WorktreeSHA256: fmt.Sprintf("%x", worktreeSum), Entries: entries}
	raw, _ := json.Marshal(struct {
		SchemaVersion  string                 `json:"schema_version"`
		Head           string                 `json:"head"`
		IndexSHA256    string                 `json:"index_sha256"`
		WorktreeSHA256 string                 `json:"worktree_sha256"`
		Entries        []gitstate.SourceEntry `json:"entries"`
	}{snapshot.SchemaVersion, snapshot.Head, snapshot.IndexSHA256, snapshot.WorktreeSHA256, snapshot.Entries})
	sum := sha256.Sum256(raw)
	snapshot.SnapshotSHA256 = fmt.Sprintf("%x", sum)
	return snapshot
}

func applyEvidence(kind autonomous.EvidenceKind, reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: "Exact apply-test evidence."}
}

func cloneApplyCriteria(criteria []autonomous.AcceptanceCriterion) []autonomous.AcceptanceCriterion {
	raw, _ := json.Marshal(criteria)
	var result []autonomous.AcceptanceCriterion
	_ = json.Unmarshal(raw, &result)
	return result
}

func writeApplyFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustApplyJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
