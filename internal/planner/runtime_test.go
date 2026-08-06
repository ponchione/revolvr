package planner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"revolvr/internal/model"
	"revolvr/internal/policy"
	"revolvr/internal/supervisor"
)

type fakeStateReader struct {
	states []CanonicalState
	reads  int
}

func (f *fakeStateReader) ReadPlannerState(context.Context, string, string) (CanonicalState, error) {
	i := f.reads
	if i >= len(f.states) {
		i = len(f.states) - 1
	}
	f.reads++
	return f.states[i], nil
}

type fakeModel struct {
	result model.Result
	err    error
	calls  int
}

func (f *fakeModel) Invoke(context.Context, model.PreparedRequest) (model.Result, error) {
	f.calls++
	return f.result, f.err
}

func TestGenerateValidPlanBindsEveryIdentityAndCriterion(t *testing.T) {
	cfg, state, prepared := testPrepared(t)
	output := validOutput(prepared, state)
	fake := cfg.Model.(*fakeModel)
	fake.result = successfulResult(prepared, output, cfg.ModelPolicy.Model)
	candidate, err := Generate(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || cfg.StateReader.(*fakeStateReader).reads != 2 {
		t.Fatalf("calls/reads = %d/%d, want 1/2", fake.calls, cfg.StateReader.(*fakeStateReader).reads)
	}
	if candidate.CandidateSHA256 != candidateHash(candidate) || candidate.Output.Revision.DossierSHA256 != candidate.Dossier.SHA256 || candidate.Output.Revision.PromptSHA256 != candidate.Prompt.SHA256 || candidate.Output.Revision.ResponseSchemaSHA256 != candidate.ResponseSchema.SHA256 || candidate.Output.Revision.ModelPolicySHA256 != candidate.ModelPolicy.SHA256 || candidate.Output.Revision.HostPolicySHA256 != candidate.HostPolicy.SHA256 {
		t.Fatalf("candidate identities are not exact: %#v", candidate.Output.Revision)
	}
	if got := []string{candidate.Output.Steps[0].CriterionIDs[0], candidate.Output.Steps[1].CriterionIDs[0]}; got[0] != "AC-1" || got[1] != "AC-2" {
		t.Fatalf("criterion mapping = %v", got)
	}
	var dossier Dossier
	if err := json.Unmarshal(candidate.Dossier.Content, &dossier); err != nil {
		t.Fatal(err)
	}
	if len(dossier.Omissions) < 4 || dossier.Omissions[0].Section != "semantic_retrieval" {
		t.Fatalf("dossier omissions = %#v", dossier.Omissions)
	}
	if strings.Contains(candidate.Prompt.Content, "OPENAI_API_KEY") {
		t.Fatal("planner prompt contains an API credential name")
	}
}

func TestPlannerProductionBoundaryHasNoAmbientCredentialToolNetworkOrMutationAccess(t *testing.T) {
	for _, name := range []string{"contracts.go", "dossier.go", "runtime.go", "schema.go", "store.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, forbidden := range []string{"OPENAI_API_KEY", "os.Getenv", "net/http", "ToolCall", "WriteFile", "taskintake.Import"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains forbidden production capability %q", name, forbidden)
			}
		}
	}
}

func TestGenerateRejectsMalformedRefusedAndUntrustedPlans(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Output)
		raw     func([]byte) []byte
		outcome model.Outcome
	}{
		{name: "duplicate steps", mutate: func(o *Output) { o.Steps[1].ID = o.Steps[0].ID }},
		{name: "reordered steps", mutate: func(o *Output) { o.Steps[0], o.Steps[1] = o.Steps[1], o.Steps[0] }},
		{name: "missing criterion", mutate: func(o *Output) { o.Steps = o.Steps[:1] }},
		{name: "invented dependency", mutate: func(o *Output) { o.TaskDependencyIDs = []string{"invented"} }},
		{name: "placeholder", mutate: func(o *Output) { o.Steps[0].Description = "TODO later" }},
		{name: "unsupported test", mutate: func(o *Output) { o.Steps[0].TestStrategy[0].Reference = "go test ./..." }},
		{name: "stale identity", mutate: func(o *Output) { o.Revision.DossierSHA256 = strings.Repeat("f", 64) }},
		{name: "scope expansion", mutate: func(o *Output) { o.Steps[0].ExpectedPaths = []string{"cmd/revolvr"} }},
		{name: "unknown field", raw: func(raw []byte) []byte {
			return []byte(strings.Replace(string(raw), `"schema_version":`, `"unknown":true,"schema_version":`, 1))
		}},
		{name: "duplicate field", raw: func(raw []byte) []byte {
			return []byte(strings.Replace(string(raw), `"schema_version":`, `"schema_version":"revolvr-planner-output-v1","schema_version":`, 1))
		}},
		{name: "malformed", raw: func([]byte) []byte { return []byte(`{"broken"`) }},
		{name: "refusal", outcome: model.OutcomeSafetyRefusal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, state, prepared := testPrepared(t)
			output := validOutput(prepared, state)
			if tt.mutate != nil {
				tt.mutate(&output)
			}
			raw, _ := json.Marshal(output)
			if tt.raw != nil {
				raw = tt.raw(raw)
			}
			result := successfulResult(prepared, output, cfg.ModelPolicy.Model)
			result.StructuredOutput = raw
			if tt.outcome != "" {
				result.Outcome = tt.outcome
			}
			cfg.Model.(*fakeModel).result = result
			candidate, err := Generate(context.Background(), cfg)
			if !errors.Is(err, ErrRejected) {
				t.Fatalf("error = %v, want ErrRejected", err)
			}
			if candidate.PlanID != "" {
				t.Fatalf("rejected candidate persisted in result: %#v", candidate)
			}
		})
	}
}

func TestRevisionCannotRegressCompletedStepOrHideLineage(t *testing.T) {
	cfg, state, prepared := testPrepared(t)
	priorOutput := validOutput(prepared, state)
	priorOutput.Steps[0].Status = "completed"
	state.PriorPlan = &PriorPlan{PlanID: cfg.PlanID, PlanVersionID: uuid.NewString(), Revision: 1, Steps: priorOutput.Steps}
	cfg.PlanVersionID = uuid.NewString()
	cfg.ExpectedPlanAggregateVersion = 2
	cfg.StateReader = &fakeStateReader{states: []CanonicalState{state, state}}
	prepared, err := Prepare(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	output := validOutput(prepared, state)
	output.Steps[0].Status = "pending"
	output.Steps[0].Lineage = &StepLineage{PriorPlanVersionID: state.PriorPlan.PlanVersionID, PriorStepID: output.Steps[0].ID, PriorStatus: "completed"}
	output.Steps[1].Lineage = &StepLineage{PriorPlanVersionID: state.PriorPlan.PlanVersionID, PriorStepID: output.Steps[1].ID, PriorStatus: "pending"}
	cfg.Model.(*fakeModel).result = successfulResult(prepared, output, cfg.ModelPolicy.Model)
	if _, err := Generate(context.Background(), cfg); !errors.Is(err, ErrRejected) {
		t.Fatalf("completed regression error = %v", err)
	}

	cfg.StateReader = &fakeStateReader{states: []CanonicalState{state, state}}
	output = validOutput(prepared, state)
	output.Steps[0].Status = "completed"
	output.Steps[0].Lineage = &StepLineage{PriorPlanVersionID: state.PriorPlan.PlanVersionID, PriorStepID: output.Steps[0].ID, PriorStatus: "completed"}
	output.Steps[1].Status = "in_progress"
	output.Steps[1].Lineage = &StepLineage{PriorPlanVersionID: state.PriorPlan.PlanVersionID, PriorStepID: output.Steps[1].ID, PriorStatus: "pending", TransitionEvidence: "criterion:AC-2"}
	cfg.Model.(*fakeModel).result = successfulResult(prepared, output, cfg.ModelPolicy.Model)
	if _, err := Generate(context.Background(), cfg); err != nil {
		t.Fatalf("valid monotonic lineage was rejected: %v", err)
	}
}

func TestGenerateRejectsCanonicalIdentityDriftDuringInvocation(t *testing.T) {
	for _, name := range []string{"task", "source", "supervisor decision", "dossier"} {
		t.Run(name, func(t *testing.T) {
			cfg, state, prepared := testPrepared(t)
			changed := state
			switch name {
			case "task":
				changed.Task.VersionNumber++
			case "source":
				changed.Source.Tree = strings.Repeat("f", 40)
			case "supervisor decision":
				changed.SupervisorDecisionID = "decision-stale"
			case "dossier":
				changed.ArchitectureConstraints = append([]EvidenceItem(nil), state.ArchitectureConstraints...)
				changed.ArchitectureConstraints[0].Summary = "Changed while the model ran"
			}
			cfg.StateReader = &fakeStateReader{states: []CanonicalState{state, changed}}
			cfg.Model.(*fakeModel).result = successfulResult(prepared, validOutput(prepared, state), cfg.ModelPolicy.Model)
			if _, err := Generate(context.Background(), cfg); !errors.Is(err, ErrRejected) {
				t.Fatalf("drift error = %v, want ErrRejected", err)
			}
		})
	}
}

func testPrepared(t *testing.T) (Config, CanonicalState, Prepared) {
	t.Helper()
	taskID := uuid.NewString()
	taskVersion := uuid.NewString()
	runID := uuid.NewString()
	sourceID := uuid.NewString()
	decisionID := "decision-" + uuid.NewString()
	decision := &supervisor.Decision{Action: policy.ActionPlan, Identity: supervisor.DecisionIdentity{DecisionID: decisionID}}
	hash := strings.Repeat("d", 64)
	decision.Identity.ContentSHA256 = &hash
	admission := supervisor.DecisionRecord{Disposition: supervisor.DecisionAccepted, DecisionID: decisionID, TaskID: taskID, TaskVersionID: taskVersion, TaskVersion: 1, RunID: runID, SourceRevision: strings.Repeat("a", 64), HostPolicy: policy.CurrentIdentity(), Decision: decision, Route: &policy.Route{Kind: policy.RouteWorkerRequest, TaskID: taskID, DecisionID: decisionID, Action: policy.ActionPlan, WorkerRole: "planner"}}
	state := CanonicalState{Task: TaskContract{TaskID: taskID, TaskVersionID: taskVersion, VersionNumber: 1, Title: "Add planner", Goal: "Create bounded plans", Scope: []string{"Planner only"}, ExcludedScope: []string{"Worker execution"}, Dependencies: []string{"architecture-014-supervisor"}, ExpectedPaths: []string{"internal/planner", "db"}, Criteria: []Criterion{{ID: "AC-1", Requirement: "Planner validates output", VerificationMethod: "command", VerificationReference: "go test ./internal/planner"}, {ID: "AC-2", Requirement: "Database stores plan", VerificationMethod: "command", VerificationReference: "go test ./..."}}}, Run: RunAuthority{RunID: runID, ProjectID: uuid.NewString(), ProjectSourceID: sourceID}, Source: SourceAuthority{Revision: strings.Repeat("a", 64), Commit: strings.Repeat("b", 40), Tree: strings.Repeat("c", 40)}, SupervisorDecisionID: decisionID, SupervisorDecisionSHA256: hash, ArchitectureConstraints: []EvidenceItem{{ID: "adr-023", Kind: "architecture", Summary: "Prompts are immutable", SHA256: strings.Repeat("e", 64)}}, ProjectMap: []ProjectPath{{Path: "internal/planner", Component: "planner", Kind: "package"}, {Path: "db", Component: "storage", Kind: "directory"}}}
	modelPolicy, err := PinModelPolicy(ModelPolicySettings{Model: "gpt-test", ReasoningEffort: "high", MaxOutputTokens: 4096, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{TaskID: taskID, TaskVersionID: taskVersion, TaskVersionNumber: 1, RunID: runID, ProjectSourceID: sourceID, SourceRevision: state.Source.Revision, SourceCommit: state.Source.Commit, SourceTree: state.Source.Tree, PlanID: uuid.NewString(), PlanVersionID: uuid.NewString(), Admission: admission, ModelPolicy: modelPolicy, HostPolicy: CurrentHostPolicy(), StateReader: &fakeStateReader{states: []CanonicalState{state, state}}, Model: &fakeModel{}}
	prepared, err := Prepare(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, state, prepared
}

func validOutput(prepared Prepared, state CanonicalState) Output {
	revision := prepared.ExpectedRevision
	return Output{RevolvrIdentity: prepared.ExpectedOutput, SchemaVersion: OutputSchemaVersion, Revision: revision, ChangeExplanation: "Initial bounded plan from the accepted task contract.", TaskDependencyIDs: append([]string(nil), state.Task.Dependencies...), Steps: []Step{{ID: "step-1", Ordinal: 1, Status: "pending", Description: "Implement the validated planner boundary.", CriterionIDs: []string{"AC-1"}, DependsOnStepIDs: []string{}, ExpectedPaths: []string{"internal/planner"}, Components: []string{"planner"}, TestStrategy: []TestStrategy{{CriterionID: "AC-1", Method: "command", Reference: "go test ./internal/planner"}}, Risks: []string{"Schema drift"}, Assumptions: []string{"Canonical task is pinned"}, EvidenceRefs: []string{"task:" + state.Task.TaskVersionID, "criterion:AC-1"}}, {ID: "step-2", Ordinal: 2, Status: "pending", Description: "Persist the accepted plan revision.", CriterionIDs: []string{"AC-2"}, DependsOnStepIDs: []string{"step-1"}, ExpectedPaths: []string{"db"}, Components: []string{"storage"}, TestStrategy: []TestStrategy{{CriterionID: "AC-2", Method: "command", Reference: "go test ./..."}}, Risks: []string{}, Assumptions: []string{}, EvidenceRefs: []string{"task:" + state.Task.TaskVersionID, "criterion:AC-2"}}}, Risks: []string{"Concurrent acceptance"}, Assumptions: []string{"PostgreSQL is canonical"}, EvidenceRefs: []string{"task:" + state.Task.TaskVersionID, "supervisor_decision:" + state.SupervisorDecisionID}}
}

func successfulResult(prepared Prepared, output Output, modelName string) model.Result {
	raw, _ := json.Marshal(output)
	return model.Result{Outcome: model.OutcomeSuccess, Request: prepared.ExpectedRequest, StructuredOutput: raw, CompletedResponse: []byte(`{"id":"response"}`), Usage: model.UsageEvidence{Available: true, InputTokens: 10, OutputTokens: 20, TotalTokens: 30}, Service: model.ServiceEvidence{Model: modelName}, Cost: model.CostEvidence{Source: "not_reported_by_responses_api"}}
}
