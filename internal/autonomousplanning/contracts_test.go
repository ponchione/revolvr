package autonomousplanning

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
)

var planningTestTime = time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

func TestPlanningOutputParseMarshalAndSchemaAreStrictAndDeterministic(t *testing.T) {
	output, _, _, _ := validPlanningFixture(t)
	first, err := MarshalPlanningOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalPlanningOutput(output)
	if err != nil || !bytes.Equal(first, second) || first[len(first)-1] != '\n' {
		t.Fatalf("deterministic marshal failed: equal=%t error=%v bytes=%q", bytes.Equal(first, second), err, first)
	}
	parsed, err := ParsePlanningOutput(first)
	if err != nil || !reflect.DeepEqual(parsed, output) {
		t.Fatalf("ParsePlanningOutput() = %+v, %v", parsed, err)
	}
	schemaOne, err := PlanningOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	schemaTwo, err := PlanningOutputSchema()
	if err != nil || !bytes.Equal(schemaOne, schemaTwo) || schemaOne[len(schemaOne)-1] != '\n' {
		t.Fatalf("schema determinism failed: equal=%t error=%v", bytes.Equal(schemaOne, schemaTwo), err)
	}
	for _, want := range []string{`"additionalProperties": false`, PlanningOutputSchemaVersion, `"planner"`, `"acceptance_criterion"`} {
		if !bytes.Contains(schemaOne, []byte(want)) {
			t.Fatalf("schema missing %q", want)
		}
	}
}

func TestParsePlanningOutputRejectsMalformedMissingMultipleAndUnknownFields(t *testing.T) {
	output, _, _, _ := validPlanningFixture(t)
	raw, err := MarshalPlanningOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	unknown := append([]byte(nil), raw...)
	unknown = bytes.Replace(unknown, []byte(`"schema_version":`), []byte(`"unknown":true,"schema_version":`), 1)
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "missing", raw: nil, want: "missing or empty"},
		{name: "malformed", raw: []byte(`{"schema_version":`), want: "decode exactly one"},
		{name: "multiple", raw: append(append([]byte(nil), raw...), []byte(` {}`)...), want: "more than one JSON value"},
		{name: "unknown", raw: unknown, want: "unknown field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePlanningOutput(tt.raw)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPlanningOutputValidateRejectsWrongRouteIdentityAndInvalidMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PlanningOutput)
		want   string
	}{
		{name: "wrong task", mutate: func(o *PlanningOutput) { o.TaskID = "other" }, want: "plan task_id"},
		{name: "wrong action", mutate: func(o *PlanningOutput) { o.Provenance.Action = autonomous.ActionImplement }, want: "authorized route"},
		{name: "wrong profile", mutate: func(o *PlanningOutput) { o.Provenance.WorkerProfile = autonomous.WorkerProfileAuditor }, want: "authorized route"},
		{name: "same run", mutate: func(o *PlanningOutput) { o.Provenance.WorkerRunID = o.Provenance.Decision.RunID }, want: "must differ"},
		{name: "missing criteria", mutate: func(o *PlanningOutput) { o.AcceptanceCriteria = nil }, want: "requires at least one"},
		{name: "duplicate criterion id", mutate: func(o *PlanningOutput) { o.AcceptanceCriteria = append(o.AcceptanceCriteria, o.AcceptanceCriteria[0]) }, want: "duplicate criterion id"},
		{name: "duplicate requirement", mutate: func(o *PlanningOutput) {
			criterion := o.AcceptanceCriteria[0]
			criterion.ID = "criterion-duplicate"
			o.AcceptanceCriteria = append(o.AcceptanceCriteria, criterion)
		}, want: "duplicate requirement"},
		{name: "invalid satisfied", mutate: func(o *PlanningOutput) { o.AcceptanceCriteria[0].Status = autonomous.AcceptanceStatusSatisfied }, want: "satisfied evidence"},
		{name: "invalid waived", mutate: func(o *PlanningOutput) { o.AcceptanceCriteria[0].Status = autonomous.AcceptanceStatusWaived }, want: "waived status requires rationale"},
		{name: "invalid not applicable", mutate: func(o *PlanningOutput) { o.AcceptanceCriteria[0].Status = autonomous.AcceptanceStatusNotApplicable }, want: "not_applicable status requires rationale"},
		{name: "pending with evidence", mutate: func(o *PlanningOutput) {
			o.AcceptanceCriteria[0].Evidence = []autonomous.EvidenceReference{planningEvidence(autonomous.EvidenceKindVerification, "unexpected")}
		}, want: "pending status must not include"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, _, _, _ := validPlanningFixture(t)
			tt.mutate(&output)
			if err := output.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestApplyProposalCreatesReadyInitialStateAndPreservesUnrelatedState(t *testing.T) {
	output, previous, decision, taskRaw := validPlanningFixture(t)
	previous.Attempts.TotalAttempts = 2
	previous.Attempts.ActionAttempts = []autonomous.ActionAttempt{{Action: autonomous.ActionPlan, Attempts: 2}}
	previous.Attempts.RetryBudget = autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4, Consumed: 2}
	previous.FindingResolutions = []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}}
	before := mustPlanningJSON(t, previous)
	next, kind, err := ApplyProposal(previous, output, decision, *output.AcceptanceCriteria[0].Source, taskRaw)
	if err != nil {
		t.Fatal(err)
	}
	if kind != ChangeKindInitial || next.Lifecycle != autonomous.LifecycleStateReady || next.Plan == nil || next.Plan.Revision != 1 || next.LatestDecision == nil || *next.LatestDecision != output.Provenance.Decision {
		t.Fatalf("next state = %+v, kind=%q", next, kind)
	}
	if !reflect.DeepEqual(next.Attempts, previous.Attempts) || !reflect.DeepEqual(next.FindingResolutions, previous.FindingResolutions) {
		t.Fatal("attempts, budgets, or findings changed")
	}
	if got := mustPlanningJSON(t, previous); !bytes.Equal(got, before) {
		t.Fatal("caller previous state mutated")
	}
}

func TestApplyProposalAcceptanceStatusAndOriginRules(t *testing.T) {
	statuses := []autonomous.AcceptanceStatus{
		autonomous.AcceptanceStatusPending,
		autonomous.AcceptanceStatusSatisfied,
		autonomous.AcceptanceStatusWaived,
		autonomous.AcceptanceStatusNotApplicable,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			output, previous, decision, taskRaw := validPlanningFixture(t)
			criterion := output.AcceptanceCriteria[0]
			criterion.Status = status
			switch status {
			case autonomous.AcceptanceStatusSatisfied:
				criterion.Evidence = []autonomous.EvidenceReference{planningEvidence(autonomous.EvidenceKindVerification, "verification-one")}
			case autonomous.AcceptanceStatusWaived:
				criterion.Rationale = "The operator explicitly waived this optional behavior."
			case autonomous.AcceptanceStatusNotApplicable:
				criterion.Rationale = "The cited surface is absent from this task."
			}
			output.AcceptanceCriteria = []autonomous.AcceptanceCriterion{criterion}
			if _, _, err := ApplyProposal(previous, output, decision, *criterion.Source, taskRaw); err != nil {
				t.Fatalf("ApplyProposal() error = %v", err)
			}
		})
	}

	tests := []struct {
		name   string
		mutate func(*PlanningOutput, *autonomous.SupervisorDecision)
		want   string
	}{
		{name: "uncited invention", mutate: func(o *PlanningOutput, _ *autonomous.SupervisorDecision) {
			source := planningEvidence(autonomous.EvidenceKindRepository, "invented")
			o.AcceptanceCriteria[0].Source = &source
		}, want: "neither the exact task source"},
		{name: "task text mismatch", mutate: func(o *PlanningOutput, _ *autonomous.SupervisorDecision) {
			o.AcceptanceCriteria[0].Requirement = "Invented requirement."
		}, want: "not an exact statement"},
		{name: "supervisor refinement mismatch", mutate: func(o *PlanningOutput, _ *autonomous.SupervisorDecision) {
			source := o.Provenance.Decision.Artifact
			o.AcceptanceCriteria[0].Source = &source
			o.AcceptanceCriteria[0].Requirement = "Not a supervisor criterion."
		}, want: "does not exactly match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, previous, decision, taskRaw := validPlanningFixture(t)
			tt.mutate(&output, &decision)
			_, _, err := ApplyProposal(previous, output, decision, CanonicalTaskOrigin(".agent/tasks/task-1.md", strings.Repeat("1", 64)), taskRaw)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("exact supervisor refinement", func(t *testing.T) {
		output, previous, decision, taskRaw := validPlanningFixture(t)
		source := output.Provenance.Decision.Artifact
		output.AcceptanceCriteria[0].Source = &source
		output.AcceptanceCriteria[0].Requirement = decision.SuccessCriteria[0]
		if _, _, err := ApplyProposal(previous, output, decision, CanonicalTaskOrigin(".agent/tasks/task-1.md", strings.Repeat("1", 64)), taskRaw); err != nil {
			t.Fatal(err)
		}
	})
}

func TestApplyProposalRevisionPreservesStableAndTerminalWork(t *testing.T) {
	output, previous, decision, taskRaw := validPlanningFixture(t)
	initial, _, err := ApplyProposal(previous, output, decision, *output.AcceptanceCriteria[0].Source, taskRaw)
	if err != nil {
		t.Fatal(err)
	}
	initial.Plan.Steps[0].Status = autonomous.PlanStepStatusCompleted
	initial.Plan.Steps[0].Evidence = []autonomous.EvidenceReference{planningEvidence(autonomous.EvidenceKindVerification, "verification-one")}
	initial.Plan.Steps = append(initial.Plan.Steps, autonomous.PlanStep{ID: "step-two", Description: "Run focused tests.", Status: autonomous.PlanStepStatusPending})
	initial.Lifecycle = autonomous.LifecycleStateReady

	revision := output
	revision.Plan.ID = "plan-two"
	revision.Plan.Revision = 2
	revision.Plan.SupersedesPlanID = "plan-one"
	revision.Plan.Steps = []autonomous.PlanStep{
		initial.Plan.Steps[0],
		initial.Plan.Steps[1],
		{ID: "step-three", Description: "Run repository verification.", Status: autonomous.PlanStepStatusPending},
	}
	revision.AcceptanceCriteria = cloneAcceptanceForTest(initial.AcceptanceCriteria)
	next, kind, err := ApplyProposal(initial, revision, decision, *output.AcceptanceCriteria[0].Source, taskRaw)
	if err != nil || kind != ChangeKindRevision || next.Plan.ID != "plan-two" {
		t.Fatalf("revision = %+v, %q, %v", next.Plan, kind, err)
	}

	tests := []struct {
		name   string
		mutate func(*PlanningOutput)
		want   string
	}{
		{name: "same plan id", mutate: func(o *PlanningOutput) { o.Plan.ID = "plan-one"; o.Plan.SupersedesPlanID = "plan-other" }, want: "new plan ID"},
		{name: "revision gap", mutate: func(o *PlanningOutput) { o.Plan.Revision = 3 }, want: "want 2"},
		{name: "wrong predecessor", mutate: func(o *PlanningOutput) { o.Plan.SupersedesPlanID = "plan-other" }, want: "want \"plan-one\""},
		{name: "drop completed", mutate: func(o *PlanningOutput) { o.Plan.Steps = o.Plan.Steps[1:] }, want: "terminal step order"},
		{name: "change completed evidence", mutate: func(o *PlanningOutput) { o.Plan.Steps[0].Evidence[0].Reference = "other" }, want: "changed status, evidence"},
		{name: "reuse description", mutate: func(o *PlanningOutput) { o.Plan.Steps[1].Description = "Different work." }, want: "different description"},
		{name: "new completed", mutate: func(o *PlanningOutput) {
			o.Plan.Steps[2].Status = autonomous.PlanStepStatusCompleted
			o.Plan.Steps[2].Evidence = []autonomous.EvidenceReference{planningEvidence(autonomous.EvidenceKindVerification, "new")}
		}, want: "cannot begin in terminal"},
		{name: "criterion disappears", mutate: func(o *PlanningOutput) {
			source := *initial.AcceptanceCriteria[0].Source
			o.AcceptanceCriteria = []autonomous.AcceptanceCriterion{{ID: "criterion-new", Requirement: "Behavior is implemented.", Status: autonomous.AcceptanceStatusPending, Source: &source}}
		}, want: "criterion \"criterion-one\" must not disappear"},
		{name: "criterion evidence changes", mutate: func(o *PlanningOutput) {
			o.AcceptanceCriteria[0].Status = autonomous.AcceptanceStatusSatisfied
			o.AcceptanceCriteria[0].Evidence = []autonomous.EvidenceReference{planningEvidence(autonomous.EvidenceKindVerification, "new")}
		}, want: "existing criterion"},
		{name: "criterion requirement changes", mutate: func(o *PlanningOutput) { o.AcceptanceCriteria[0].Requirement = "Changed requirement." }, want: "existing criterion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := revision
			candidate.Plan.Provenance = append([]autonomous.EvidenceReference(nil), revision.Plan.Provenance...)
			candidate.Plan.Steps = cloneStepsForTest(revision.Plan.Steps)
			candidate.AcceptanceCriteria = cloneAcceptanceForTest(revision.AcceptanceCriteria)
			tt.mutate(&candidate)
			_, _, err := ApplyProposal(initial, candidate, decision, *output.AcceptanceCriteria[0].Source, taskRaw)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestApplyProposalCanReviseCompletedPlanAndAddGroundedCriterion(t *testing.T) {
	output, previous, decision, taskRaw := validPlanningFixture(t)
	initial, _, err := ApplyProposal(previous, output, decision, *output.AcceptanceCriteria[0].Source, taskRaw)
	if err != nil {
		t.Fatal(err)
	}
	initial.Plan.Steps[0].Status = autonomous.PlanStepStatusCompleted
	initial.Plan.Steps[0].Evidence = []autonomous.EvidenceReference{planningEvidence(autonomous.EvidenceKindVerification, "verification-one")}
	initial.Plan.Completed = true
	initial.Lifecycle = autonomous.LifecycleStateReady
	revision := output
	revision.Plan.ID = "plan-two"
	revision.Plan.Revision = 2
	revision.Plan.SupersedesPlanID = "plan-one"
	revision.Plan.Steps = []autonomous.PlanStep{
		initial.Plan.Steps[0],
		{ID: "step-two", Description: "Address newly grounded work.", Status: autonomous.PlanStepStatusPending},
	}
	revision.AcceptanceCriteria = cloneAcceptanceForTest(initial.AcceptanceCriteria)
	taskRaw = append(taskRaw, []byte("New grounded requirement.\n")...)
	taskOrigin := *output.AcceptanceCriteria[0].Source
	newSource := taskOrigin
	revision.AcceptanceCriteria = append(revision.AcceptanceCriteria, autonomous.AcceptanceCriterion{ID: "criterion-two", Requirement: "New grounded requirement.", Status: autonomous.AcceptanceStatusPending, Source: &newSource})
	next, kind, err := ApplyProposal(initial, revision, decision, taskOrigin, taskRaw)
	if err != nil || kind != ChangeKindRevision || next.Plan.Completed || len(next.AcceptanceCriteria) != 2 {
		t.Fatalf("completed-plan revision = %+v, %q, %v", next, kind, err)
	}
}

func validPlanningFixture(t *testing.T) (PlanningOutput, autonomous.ExecutionState, autonomous.SupervisorDecision, []byte) {
	t.Helper()
	taskRaw := []byte("# Task\n\nBehavior is implemented.\n")
	taskOrigin := CanonicalTaskOrigin(".agent/tasks/task-1.md", strings.Repeat("1", 64))
	decisionArtifact := planningEvidence(autonomous.EvidenceKindFile, ".revolvr/runs/supervisor-run/supervisor-decision.json")
	reference := autonomous.DecisionReference{
		DecisionID: "decision-one", RunID: "supervisor-run", TaskID: "task-1",
		Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner,
		Artifact: decisionArtifact, CreatedAt: planningTestTime,
	}
	decision := autonomous.SupervisorDecision{
		TaskID: "task-1", Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner,
		Rationale: "A durable plan is required.", SuccessCriteria: []string{"Supervisor refinement is retained."},
		Inputs: []autonomous.EvidenceReference{taskOrigin},
	}
	source := taskOrigin
	output := PlanningOutput{
		SchemaVersion: PlanningOutputSchemaVersion,
		TaskID:        "task-1",
		Plan: autonomous.TaskPlan{
			TaskID: "task-1", ID: "plan-one", Revision: 1,
			Provenance: []autonomous.EvidenceReference{taskOrigin, decisionArtifact},
			Steps:      []autonomous.PlanStep{{ID: "step-one", Description: "Implement behavior.", Status: autonomous.PlanStepStatusPending}},
		},
		AcceptanceCriteria: []autonomous.AcceptanceCriterion{{
			ID: "criterion-one", Requirement: "Behavior is implemented.", Status: autonomous.AcceptanceStatusPending, Source: &source,
		}},
		Inputs: []autonomous.EvidenceReference{taskOrigin, decisionArtifact},
		Provenance: PlanningProvenance{
			Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner, WorkerRunID: "worker-run",
			Decision:      reference,
			Dossier:       DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", SHA256: strings.Repeat("2", 64), ByteSize: 100},
			Profile:       ProfileIdentity{Name: autonomous.WorkerProfilePlanner, Path: ".agent/profiles/planner.md", SHA256: strings.Repeat("3", 64), ByteSize: 50},
			RawOutputPath: ".revolvr/runs/worker-run/planner-output.raw.json", SourceRevision: strings.Repeat("4", 64),
		},
	}
	previous := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-1", Lifecycle: autonomous.LifecycleStatePending,
		Attempts: planningZeroAttempts(),
	}
	return output, previous, decision, taskRaw
}

func planningZeroAttempts() autonomous.AttemptState {
	return autonomous.AttemptState{
		RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
		TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
	}
}

func planningEvidence(kind autonomous.EvidenceKind, reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: "Exact durable planning evidence."}
}

func mustPlanningJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneStepsForTest(steps []autonomous.PlanStep) []autonomous.PlanStep {
	raw, _ := json.Marshal(steps)
	var cloned []autonomous.PlanStep
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func cloneAcceptanceForTest(criteria []autonomous.AcceptanceCriterion) []autonomous.AcceptanceCriterion {
	raw, _ := json.Marshal(criteria)
	var cloned []autonomous.AcceptanceCriterion
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}
