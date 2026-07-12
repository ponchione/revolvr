package autonomous

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestPlanStepValidateSupportsEveryStatus(t *testing.T) {
	tests := []struct {
		status    PlanStepStatus
		evidence  []EvidenceReference
		rationale string
	}{
		{status: PlanStepStatusPending},
		{status: PlanStepStatusInProgress},
		{status: PlanStepStatusCompleted, evidence: []EvidenceReference{testEvidence(EvidenceKindVerification, "run-1:tests")}},
		{status: PlanStepStatusSkipped, rationale: "The task has no user-facing documentation surface."},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			step := PlanStep{
				ID:          "step-001",
				Description: "Implement the bounded contract slice.",
				Status:      tt.status,
				Evidence:    tt.evidence,
				Rationale:   tt.rationale,
			}
			if err := step.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	step := validPlanStep(PlanStepStatusPending)
	step.Status = "waiting"
	assertErrorContains(t, step.Validate(), `unknown status "waiting"`)
}

func TestTaskPlanValidateAcceptsOrderedSnapshots(t *testing.T) {
	plan := validTaskPlan(false)
	plan.Steps = []PlanStep{
		validPlanStep(PlanStepStatusCompleted),
		{ID: "step-002", Description: "Add focused validation tests.", Status: PlanStepStatusInProgress},
		{ID: "step-003", Description: "Run repository verification.", Status: PlanStepStatusPending},
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	completed := validTaskPlan(true)
	completed.Steps = append(completed.Steps, PlanStep{
		ID:          "step-002",
		Description: "Document an intentionally omitted optional step.",
		Status:      PlanStepStatusSkipped,
		Rationale:   "The optional surface is outside this task's scope.",
	})
	if err := completed.Validate(); err != nil {
		t.Fatalf("completed Validate() error = %v", err)
	}

	revision := completed
	revision.ID = "plan-002"
	revision.Revision = 2
	revision.SupersedesPlanID = completed.ID
	if err := revision.Validate(); err != nil {
		t.Fatalf("revision Validate() error = %v", err)
	}
}

func TestTaskPlanValidateRejectsInvalidSnapshots(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*TaskPlan)
		wantErr string
	}{
		{name: "missing task id", mutate: func(plan *TaskPlan) { plan.TaskID = " " }, wantErr: "task_id is required"},
		{name: "malformed plan id", mutate: func(plan *TaskPlan) { plan.ID = "Plan_1" }, wantErr: `invalid id "Plan_1"`},
		{name: "zero revision", mutate: func(plan *TaskPlan) { plan.Revision = 0 }, wantErr: "revision must be at least 1"},
		{name: "first revision supersedes", mutate: func(plan *TaskPlan) { plan.SupersedesPlanID = "plan-000" }, wantErr: "revision 1 must not set supersedes_plan_id"},
		{name: "later revision missing predecessor", mutate: func(plan *TaskPlan) { plan.Revision = 2 }, wantErr: "invalid supersedes_plan_id"},
		{name: "self superseding revision", mutate: func(plan *TaskPlan) { plan.Revision = 2; plan.SupersedesPlanID = plan.ID }, wantErr: "supersedes_plan_id must differ from id"},
		{name: "missing provenance", mutate: func(plan *TaskPlan) { plan.Provenance = nil }, wantErr: "provenance requires at least one evidence reference"},
		{name: "empty steps", mutate: func(plan *TaskPlan) { plan.Steps = nil }, wantErr: "steps requires at least one plan step"},
		{name: "malformed step id", mutate: func(plan *TaskPlan) { plan.Steps[0].ID = "step_1" }, wantErr: `steps[0]: invalid step id "step_1"`},
		{name: "duplicate step id", mutate: func(plan *TaskPlan) { plan.Steps = append(plan.Steps, plan.Steps[0]) }, wantErr: `duplicate step id "step-001"`},
		{
			name: "multiple in progress",
			mutate: func(plan *TaskPlan) {
				plan.Steps[0] = validPlanStep(PlanStepStatusInProgress)
				plan.Steps = append(plan.Steps, PlanStep{ID: "step-002", Description: "Second active step.", Status: PlanStepStatusInProgress})
			},
			wantErr: "at most one step may be in_progress",
		},
		{name: "completed with unfinished step", mutate: func(plan *TaskPlan) { plan.Completed = true }, wantErr: "completed plan contains unfinished steps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := validTaskPlan(false)
			tt.mutate(&plan)
			assertErrorContains(t, plan.Validate(), tt.wantErr)
		})
	}
}

func TestPlanStepValidateRequiresTerminalEvidenceAndRationale(t *testing.T) {
	tests := []struct {
		name    string
		step    PlanStep
		wantErr string
	}{
		{name: "completed without evidence", step: PlanStep{ID: "step-001", Description: "Complete work.", Status: PlanStepStatusCompleted}, wantErr: "completed step evidence requires at least one evidence reference"},
		{name: "skipped without rationale", step: PlanStep{ID: "step-001", Description: "Skip work.", Status: PlanStepStatusSkipped}, wantErr: "skipped step requires rationale"},
		{name: "pending with evidence", step: PlanStep{ID: "step-001", Description: "Pending work.", Status: PlanStepStatusPending, Evidence: []EvidenceReference{testEvidence(EvidenceKindFile, "file.go")}}, wantErr: "must not include terminal evidence"},
		{name: "in progress with rationale", step: PlanStep{ID: "step-001", Description: "Active work.", Status: PlanStepStatusInProgress, Rationale: "skip"}, wantErr: "must not include skip rationale"},
		{name: "completed with skip rationale", step: PlanStep{ID: "step-001", Description: "Complete work.", Status: PlanStepStatusCompleted, Evidence: []EvidenceReference{testEvidence(EvidenceKindFile, "file.go")}, Rationale: "skip"}, wantErr: "completed step must not include skip rationale"},
		{name: "skipped with invalid evidence", step: PlanStep{ID: "step-001", Description: "Skip work.", Status: PlanStepStatusSkipped, Evidence: []EvidenceReference{{Kind: EvidenceKindFile}}, Rationale: "Not required."}, wantErr: "reference is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.step.Validate(), tt.wantErr)
		})
	}
}

func TestAcceptanceCriterionValidateSupportsEveryStatus(t *testing.T) {
	tests := []AcceptanceCriterion{
		validAcceptanceCriterion("criterion-pending", AcceptanceStatusPending),
		validAcceptanceCriterion("criterion-satisfied", AcceptanceStatusSatisfied),
		validAcceptanceCriterion("criterion-waived", AcceptanceStatusWaived),
		validAcceptanceCriterion("criterion-na", AcceptanceStatusNotApplicable),
	}
	for _, criterion := range tests {
		t.Run(string(criterion.Status), func(t *testing.T) {
			if err := criterion.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	criterion := validAcceptanceCriterion("criterion-001", AcceptanceStatusPending)
	criterion.Status = "partial"
	assertErrorContains(t, criterion.Validate(), `unknown status "partial"`)
}

func TestAcceptanceCriterionValidateRejectsInvalidDisposition(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AcceptanceCriterion)
		wantErr string
	}{
		{name: "malformed id", mutate: func(criterion *AcceptanceCriterion) { criterion.ID = "Criterion_1" }, wantErr: "invalid id"},
		{name: "missing requirement", mutate: func(criterion *AcceptanceCriterion) { criterion.Requirement = " " }, wantErr: "requirement is required"},
		{name: "satisfied without evidence", mutate: func(criterion *AcceptanceCriterion) { criterion.Status = AcceptanceStatusSatisfied }, wantErr: "satisfied evidence requires at least one evidence reference"},
		{name: "waived without rationale", mutate: func(criterion *AcceptanceCriterion) { criterion.Status = AcceptanceStatusWaived }, wantErr: "waived status requires rationale"},
		{name: "not applicable without rationale", mutate: func(criterion *AcceptanceCriterion) { criterion.Status = AcceptanceStatusNotApplicable }, wantErr: "not_applicable status requires rationale"},
		{name: "pending with evidence", mutate: func(criterion *AcceptanceCriterion) {
			criterion.Evidence = []EvidenceReference{testEvidence(EvidenceKindVerification, "run-1:tests")}
		}, wantErr: "pending status must not include disposition evidence"},
		{name: "pending with rationale", mutate: func(criterion *AcceptanceCriterion) { criterion.Rationale = "Already done." }, wantErr: "pending status must not include disposition rationale"},
		{name: "invalid source", mutate: func(criterion *AcceptanceCriterion) { criterion.Source = &EvidenceReference{Kind: EvidenceKindTask} }, wantErr: "source[0]: reference is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			criterion := validAcceptanceCriterion("criterion-001", AcceptanceStatusPending)
			tt.mutate(&criterion)
			assertErrorContains(t, criterion.Validate(), tt.wantErr)
		})
	}
}

func TestExecutionStateValidateRejectsDuplicateCriterionIDs(t *testing.T) {
	state := validExecutionState(LifecycleStateWorking)
	state.AcceptanceCriteria = append(state.AcceptanceCriteria, state.AcceptanceCriteria[0])
	assertErrorContains(t, state.Validate(), `duplicate criterion id "criterion-001"`)
}

func TestFindingResolutionValidateSupportsEveryStatus(t *testing.T) {
	statuses := []FindingResolutionStatus{
		FindingResolutionStatusOpen,
		FindingResolutionStatusResolved,
		FindingResolutionStatusWaived,
		FindingResolutionStatusSuperseded,
		FindingResolutionStatusInvalid,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			resolution := validFindingResolution("finding-001", status)
			if err := resolution.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	resolution := validFindingResolution("finding-001", FindingResolutionStatusOpen)
	resolution.Status = "ignored"
	assertErrorContains(t, resolution.Validate(), `unknown status "ignored"`)
}

func TestFindingResolutionValidateRejectsInvalidDisposition(t *testing.T) {
	tests := []struct {
		name       string
		resolution FindingResolution
		wantErr    string
	}{
		{name: "malformed finding id", resolution: FindingResolution{FindingID: "Finding_1", Status: FindingResolutionStatusOpen}, wantErr: "invalid finding_id"},
		{name: "open with evidence", resolution: FindingResolution{FindingID: "finding-001", Status: FindingResolutionStatusOpen, Evidence: []EvidenceReference{testEvidence(EvidenceKindVerification, "run-1:tests")}}, wantErr: "open status must not include resolution evidence"},
		{name: "resolved without evidence", resolution: FindingResolution{FindingID: "finding-001", Status: FindingResolutionStatusResolved}, wantErr: "resolved evidence requires at least one evidence reference"},
		{name: "waived without rationale", resolution: FindingResolution{FindingID: "finding-001", Status: FindingResolutionStatusWaived}, wantErr: "waived status requires rationale"},
		{name: "invalid without rationale", resolution: FindingResolution{FindingID: "finding-001", Status: FindingResolutionStatusInvalid}, wantErr: "invalid status requires rationale"},
		{name: "superseded malformed replacement", resolution: FindingResolution{FindingID: "finding-001", Status: FindingResolutionStatusSuperseded, SupersedingFindingID: "Finding_2"}, wantErr: "invalid superseding_finding_id"},
		{name: "self superseding", resolution: FindingResolution{FindingID: "finding-001", Status: FindingResolutionStatusSuperseded, SupersedingFindingID: "finding-001"}, wantErr: "finding cannot supersede itself"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.resolution.Validate(), tt.wantErr)
		})
	}
}

func TestExecutionStateValidateRejectsDuplicateFindingResolutionIDs(t *testing.T) {
	state := validExecutionState(LifecycleStateCorrecting)
	state.FindingResolutions = []FindingResolution{
		validFindingResolution("finding-001", FindingResolutionStatusOpen),
		validFindingResolution("finding-001", FindingResolutionStatusResolved),
	}
	assertErrorContains(t, state.Validate(), `duplicate finding_id "finding-001"`)
}

func TestDecisionReferenceValidateSupportsEveryAction(t *testing.T) {
	actions := []Action{
		ActionPlan,
		ActionImplement,
		ActionAudit,
		ActionCorrect,
		ActionDocument,
		ActionSimplify,
		ActionComplete,
		ActionBlock,
		ActionNeedsInput,
	}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			reference := validDecisionReference(action)
			if err := reference.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestDecisionReferenceValidateRejectsInvalidProvenance(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*DecisionReference)
		wantErr string
	}{
		{name: "malformed decision id", mutate: func(reference *DecisionReference) { reference.DecisionID = "Decision_1" }, wantErr: "invalid decision_id"},
		{name: "missing run id", mutate: func(reference *DecisionReference) { reference.RunID = " " }, wantErr: "run_id is required"},
		{name: "missing task id", mutate: func(reference *DecisionReference) { reference.TaskID = "" }, wantErr: "task_id is required"},
		{name: "unknown action", mutate: func(reference *DecisionReference) { reference.Action = "review" }, wantErr: `unknown action "review"`},
		{name: "missing profile", mutate: func(reference *DecisionReference) { reference.WorkerProfile = "" }, wantErr: `requires compatible worker_profile "implementer"`},
		{name: "incompatible profile", mutate: func(reference *DecisionReference) { reference.WorkerProfile = WorkerProfileAuditor }, wantErr: `requires compatible worker_profile "implementer"`},
		{name: "invalid artifact", mutate: func(reference *DecisionReference) { reference.Artifact.Reference = "" }, wantErr: "artifact[0]: reference is required"},
		{name: "missing timestamp", mutate: func(reference *DecisionReference) { reference.CreatedAt = time.Time{} }, wantErr: "created_at is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reference := validDecisionReference(ActionImplement)
			tt.mutate(&reference)
			assertErrorContains(t, reference.Validate(), tt.wantErr)
		})
	}

	terminal := validDecisionReference(ActionComplete)
	terminal.WorkerProfile = WorkerProfileImplementer
	assertErrorContains(t, terminal.Validate(), `terminal action "complete" must not select worker_profile`)
}

func TestBudgetsValidateExplicitModesAndOverLimitUsage(t *testing.T) {
	countBudgets := []CountBudget{
		{Mode: BudgetModeUnset},
		{Mode: BudgetModeLimited, Limit: 0},
		{Mode: BudgetModeLimited, Limit: 1, Consumed: 2},
		{Mode: BudgetModeUnlimited, Consumed: math.MaxInt64},
	}
	for _, budget := range countBudgets {
		t.Run("count_"+string(budget.Mode), func(t *testing.T) {
			if err := budget.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	durationBudgets := []DurationBudget{
		{Mode: BudgetModeUnset},
		{Mode: BudgetModeLimited, Limit: 0},
		{Mode: BudgetModeLimited, Limit: time.Second, Consumed: 2 * time.Second},
		{Mode: BudgetModeUnlimited, Consumed: time.Duration(math.MaxInt64)},
	}
	for _, budget := range durationBudgets {
		t.Run("duration_"+string(budget.Mode), func(t *testing.T) {
			if err := budget.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	assertErrorContains(t, CountBudget{Mode: "bounded"}.Validate(), `unknown mode "bounded"`)
	assertErrorContains(t, DurationBudget{Mode: "bounded"}.Validate(), `unknown mode "bounded"`)
}

func TestBudgetsValidateRejectsNegativeAndContradictoryValues(t *testing.T) {
	countTests := []struct {
		name    string
		budget  CountBudget
		wantErr string
	}{
		{name: "negative limit", budget: CountBudget{Mode: BudgetModeLimited, Limit: -1}, wantErr: "limit cannot be negative"},
		{name: "negative consumed", budget: CountBudget{Mode: BudgetModeUnset, Consumed: -1}, wantErr: "consumed cannot be negative"},
		{name: "unset nonzero limit", budget: CountBudget{Mode: BudgetModeUnset, Limit: 1}, wantErr: "must use a zero limit"},
		{name: "unlimited nonzero limit", budget: CountBudget{Mode: BudgetModeUnlimited, Limit: 1}, wantErr: "must use a zero limit"},
	}
	for _, tt := range countTests {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.budget.Validate(), tt.wantErr)
		})
	}

	durationTests := []struct {
		name    string
		budget  DurationBudget
		wantErr string
	}{
		{name: "negative duration limit", budget: DurationBudget{Mode: BudgetModeLimited, Limit: -time.Second}, wantErr: "limit cannot be negative"},
		{name: "negative duration consumed", budget: DurationBudget{Mode: BudgetModeUnset, Consumed: -time.Second}, wantErr: "consumed cannot be negative"},
		{name: "unlimited duration with limit", budget: DurationBudget{Mode: BudgetModeUnlimited, Limit: time.Second}, wantErr: "must use a zero limit"},
	}
	for _, tt := range durationTests {
		t.Run(tt.name, func(t *testing.T) {
			assertErrorContains(t, tt.budget.Validate(), tt.wantErr)
		})
	}
}

func TestAttemptStateValidateAcceptsZeroAndMaximumCounters(t *testing.T) {
	if err := zeroAttemptState().Validate(); err != nil {
		t.Fatalf("zero Validate() error = %v", err)
	}

	maximum := zeroAttemptState()
	maximum.TotalAttempts = math.MaxInt64
	maximum.ConsecutiveFailures = math.MaxInt64
	maximum.ActionAttempts = []ActionAttempt{{Action: ActionImplement, Attempts: math.MaxInt64}}
	maximum.RetryBudget = CountBudget{Mode: BudgetModeUnlimited, Consumed: math.MaxInt64}
	maximum.TokenBudget = CountBudget{Mode: BudgetModeLimited, Limit: 1, Consumed: math.MaxInt64}
	maximum.ElapsedTimeBudget = DurationBudget{Mode: BudgetModeLimited, Limit: time.Second, Consumed: time.Duration(math.MaxInt64)}
	if err := maximum.Validate(); err != nil {
		t.Fatalf("maximum Validate() error = %v", err)
	}
}

func TestAttemptStateValidateRejectsInvalidCounters(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AttemptState)
		wantErr string
	}{
		{name: "negative total", mutate: func(state *AttemptState) { state.TotalAttempts = -1 }, wantErr: "total_attempts cannot be negative"},
		{name: "negative failures", mutate: func(state *AttemptState) { state.ConsecutiveFailures = -1 }, wantErr: "consecutive_failures cannot be negative"},
		{name: "failures exceed attempts", mutate: func(state *AttemptState) { state.ConsecutiveFailures = 2 }, wantErr: "exceeds total_attempts"},
		{name: "unknown action", mutate: func(state *AttemptState) { state.ActionAttempts = []ActionAttempt{{Action: "review", Attempts: 1}} }, wantErr: `unknown action "review"`},
		{name: "negative action count", mutate: func(state *AttemptState) {
			state.ActionAttempts = []ActionAttempt{{Action: ActionImplement, Attempts: -1}}
		}, wantErr: "attempts cannot be negative"},
		{name: "duplicate action", mutate: func(state *AttemptState) {
			state.ActionAttempts = []ActionAttempt{{Action: ActionImplement, Attempts: 1}, {Action: ActionImplement, Attempts: 0}}
		}, wantErr: `duplicate action "implement"`},
		{name: "action sum exceeds total", mutate: func(state *AttemptState) {
			state.ActionAttempts = []ActionAttempt{{Action: ActionPlan, Attempts: 1}, {Action: ActionImplement, Attempts: 1}}
		}, wantErr: "action attempt total exceeds total_attempts"},
		{name: "retry consumed exceeds total", mutate: func(state *AttemptState) { state.RetryBudget.Consumed = 2 }, wantErr: "retry_budget consumed 2 exceeds total_attempts 1"},
		{name: "negative retry limit", mutate: func(state *AttemptState) { state.RetryBudget.Limit = -1 }, wantErr: "retry_budget: limit cannot be negative"},
		{name: "negative elapsed", mutate: func(state *AttemptState) { state.ElapsedTimeBudget.Consumed = -time.Second }, wantErr: "elapsed_time_budget: consumed cannot be negative"},
		{name: "negative token usage", mutate: func(state *AttemptState) { state.TokenBudget.Consumed = -1 }, wantErr: "token_budget: consumed cannot be negative"},
		{name: "invalid last failure", mutate: func(state *AttemptState) { state.LastFailure = &EvidenceReference{Kind: EvidenceKindLedger} }, wantErr: "last_failure[0]: reference is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := validAttemptState()
			tt.mutate(&state)
			assertErrorContains(t, state.Validate(), tt.wantErr)
		})
	}
}

func TestExecutionStateValidateSupportsEveryLifecycle(t *testing.T) {
	states := []LifecycleState{
		LifecycleStatePending,
		LifecycleStateReady,
		LifecycleStatePlanning,
		LifecycleStateWorking,
		LifecycleStateVerifying,
		LifecycleStateAuditing,
		LifecycleStateCorrecting,
		LifecycleStateNeedsInput,
		LifecycleStateFinalizing,
		LifecycleStateCompleted,
		LifecycleStateBlocked,
		LifecycleStateCancelled,
		LifecycleStateSuperseded,
		LifecycleStateAbandoned,
	}
	for _, lifecycle := range states {
		t.Run(string(lifecycle), func(t *testing.T) {
			state := validExecutionState(lifecycle)
			if err := state.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	state := validExecutionState(LifecycleStateWorking)
	state.Lifecycle = "paused"
	assertErrorContains(t, state.Validate(), `unknown lifecycle "paused"`)
}

func TestExecutionStateValidateRejectsContradictoryDetails(t *testing.T) {
	tests := []struct {
		name    string
		state   ExecutionState
		mutate  func(*ExecutionState)
		wantErr string
	}{
		{name: "unsupported schema", state: validExecutionState(LifecycleStateWorking), mutate: func(state *ExecutionState) { state.SchemaVersion = "v2" }, wantErr: "unsupported schema_version"},
		{name: "missing task id", state: validExecutionState(LifecycleStateWorking), mutate: func(state *ExecutionState) { state.TaskID = " " }, wantErr: "task_id is required"},
		{name: "needs input missing detail", state: validExecutionState(LifecycleStateNeedsInput), mutate: func(state *ExecutionState) { state.NeedsInput = nil }, wantErr: "requires needs_input detail"},
		{name: "needs input missing reason", state: validExecutionState(LifecycleStateNeedsInput), mutate: func(state *ExecutionState) { state.NeedsInput.Reason = " " }, wantErr: "reason is required"},
		{name: "active with needs input", state: validExecutionState(LifecycleStateWorking), mutate: func(state *ExecutionState) { state.NeedsInput = &NeedsInputDetail{Reason: "Need a choice."} }, wantErr: "must not include needs_input detail"},
		{name: "terminal missing detail", state: validExecutionState(LifecycleStateBlocked), mutate: func(state *ExecutionState) { state.Terminal = nil }, wantErr: "requires terminal detail"},
		{name: "terminal blank reason", state: validExecutionState(LifecycleStateCancelled), mutate: func(state *ExecutionState) { state.Terminal.Reason = "" }, wantErr: "reason is required"},
		{name: "active with terminal", state: validExecutionState(LifecycleStateWorking), mutate: func(state *ExecutionState) { state.Terminal = &TerminalDetail{Reason: "Done."} }, wantErr: "nonterminal lifecycle"},
		{name: "completed missing plan", state: validExecutionState(LifecycleStateCompleted), mutate: func(state *ExecutionState) { state.Plan = nil }, wantErr: "completed lifecycle requires a plan"},
		{name: "completed unfinished plan", state: validExecutionState(LifecycleStateCompleted), mutate: func(state *ExecutionState) { state.Plan.Completed = false }, wantErr: "requires a completed plan"},
		{
			name:  "completed pending acceptance",
			state: validExecutionState(LifecycleStateCompleted),
			mutate: func(state *ExecutionState) {
				state.AcceptanceCriteria[0] = validAcceptanceCriterion("criterion-001", AcceptanceStatusPending)
			},
			wantErr: "completed lifecycle has pending acceptance_criteria",
		},
		{
			name:  "completed open finding",
			state: validExecutionState(LifecycleStateCompleted),
			mutate: func(state *ExecutionState) {
				state.FindingResolutions = []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusOpen)}
			},
			wantErr: "completed lifecycle has open finding_resolutions",
		},
		{
			name:  "completed without complete decision",
			state: validExecutionState(LifecycleStateCompleted),
			mutate: func(state *ExecutionState) {
				decision := validDecisionReference(ActionAudit)
				state.LatestDecision = &decision
			},
			wantErr: "requires latest_decision action complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.state
			tt.mutate(&state)
			assertErrorContains(t, state.Validate(), tt.wantErr)
		})
	}
}

func TestExecutionStateValidateRejectsTaskIdentityMismatches(t *testing.T) {
	t.Run("plan", func(t *testing.T) {
		state := validExecutionState(LifecycleStateWorking)
		state.Plan.TaskID = "task-2"
		assertErrorContains(t, state.Validate(), `plan task_id "task-2" does not match state task_id "task-1"`)
	})

	t.Run("latest decision", func(t *testing.T) {
		state := validExecutionState(LifecycleStateWorking)
		state.LatestDecision.TaskID = "task-2"
		assertErrorContains(t, state.Validate(), `latest_decision task_id "task-2" does not match state task_id "task-1"`)
	})

	t.Run("finding resolution decision", func(t *testing.T) {
		state := validExecutionState(LifecycleStateCorrecting)
		resolution := validFindingResolution("finding-001", FindingResolutionStatusResolved)
		resolution.Resolution.TaskID = "task-2"
		state.FindingResolutions = []FindingResolution{resolution}
		assertErrorContains(t, state.Validate(), `resolution task_id "task-2" does not match state task_id "task-1"`)
	})
}

func TestExecutionStateValidateRejectsReusedDecisionIdentity(t *testing.T) {
	state := validExecutionState(LifecycleStateCorrecting)
	resolution := validFindingResolution("finding-001", FindingResolutionStatusResolved)
	resolution.Resolution.DecisionID = state.LatestDecision.DecisionID
	state.FindingResolutions = []FindingResolution{resolution}
	assertErrorContains(t, state.Validate(), `decision_id "decision-001" is reused for a materially different reference`)
}

func TestValidateExecutionStateTransitionAllowsMinimalProgress(t *testing.T) {
	t.Run("plan step progresses", func(t *testing.T) {
		previous := validExecutionState(LifecycleStateWorking)
		next := previous
		next.Plan = copyPlan(previous.Plan)
		next.Plan.Steps = append([]PlanStep(nil), previous.Plan.Steps...)
		next.Plan.Steps[0].Status = PlanStepStatusInProgress
		if err := ValidateExecutionStateTransition(previous, next); err != nil {
			t.Fatalf("ValidateExecutionStateTransition() error = %v", err)
		}
	})

	t.Run("finding resolves", func(t *testing.T) {
		previous := validExecutionState(LifecycleStateCorrecting)
		previous.FindingResolutions = []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusOpen)}
		next := previous
		next.FindingResolutions = []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusResolved)}
		if err := ValidateExecutionStateTransition(previous, next); err != nil {
			t.Fatalf("ValidateExecutionStateTransition() error = %v", err)
		}
	})

	t.Run("linked plan revision", func(t *testing.T) {
		previous := validExecutionState(LifecycleStatePlanning)
		previous.Plan.Steps[0] = validPlanStep(PlanStepStatusCompleted)
		next := previous
		next.Plan = copyPlan(previous.Plan)
		next.Plan.ID = "plan-002"
		next.Plan.Revision = 2
		next.Plan.SupersedesPlanID = "plan-001"
		next.Plan.Steps = []PlanStep{
			validPlanStep(PlanStepStatusCompleted),
			{ID: "step-002", Description: "Perform the revised follow-up.", Status: PlanStepStatusPending},
		}
		if err := ValidateExecutionStateTransition(previous, next); err != nil {
			t.Fatalf("ValidateExecutionStateTransition() error = %v", err)
		}
	})

	t.Run("needs input resumes", func(t *testing.T) {
		previous := validExecutionState(LifecycleStateNeedsInput)
		next := previous
		next.Lifecycle = LifecycleStateReady
		next.NeedsInput = nil
		if err := ValidateExecutionStateTransition(previous, next); err != nil {
			t.Fatalf("ValidateExecutionStateTransition() error = %v", err)
		}
	})
}

func TestValidateExecutionStateTransitionRejectsSilentReversals(t *testing.T) {
	tests := []struct {
		name     string
		previous func() ExecutionState
		next     func(ExecutionState) ExecutionState
		wantErr  string
	}{
		{
			name: "completed step reverts",
			previous: func() ExecutionState {
				state := validExecutionState(LifecycleStateWorking)
				state.Plan.Steps[0] = validPlanStep(PlanStepStatusCompleted)
				return state
			},
			next: func(state ExecutionState) ExecutionState {
				state.Plan = copyPlan(state.Plan)
				state.Plan.Steps = []PlanStep{validPlanStep(PlanStepStatusPending)}
				return state
			},
			wantErr: "terminal step \"step-001\" changed status from \"completed\" to \"pending\"",
		},
		{
			name:     "terminal task returns active",
			previous: func() ExecutionState { return validExecutionState(LifecycleStateCompleted) },
			next: func(state ExecutionState) ExecutionState {
				state.Lifecycle = LifecycleStateWorking
				state.Terminal = nil
				return state
			},
			wantErr: "terminal lifecycle \"completed\" cannot transition to \"working\"",
		},
		{
			name: "resolved finding reopens",
			previous: func() ExecutionState {
				state := validExecutionState(LifecycleStateCorrecting)
				state.FindingResolutions = []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusResolved)}
				return state
			},
			next: func(state ExecutionState) ExecutionState {
				state.FindingResolutions = []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusOpen)}
				return state
			},
			wantErr: "cannot change terminal resolution from \"resolved\" to \"open\"",
		},
		{
			name:     "criterion disappears",
			previous: func() ExecutionState { return validExecutionState(LifecycleStateWorking) },
			next: func(state ExecutionState) ExecutionState {
				state.AcceptanceCriteria = nil
				return state
			},
			wantErr: "criterion \"criterion-001\" must not disappear",
		},
		{
			name:     "criterion id reused",
			previous: func() ExecutionState { return validExecutionState(LifecycleStateWorking) },
			next: func(state ExecutionState) ExecutionState {
				state.AcceptanceCriteria = append([]AcceptanceCriterion(nil), state.AcceptanceCriteria...)
				state.AcceptanceCriteria[0].Requirement = "A materially different requirement."
				return state
			},
			wantErr: "criterion id \"criterion-001\" was reused",
		},
		{
			name:     "decision id reused",
			previous: func() ExecutionState { return validExecutionState(LifecycleStateWorking) },
			next: func(state ExecutionState) ExecutionState {
				decision := *state.LatestDecision
				decision.RunID = "run-2"
				state.LatestDecision = &decision
				return state
			},
			wantErr: "decision_id \"decision-001\" was reused",
		},
		{
			name:     "plan step identity reused",
			previous: func() ExecutionState { return validExecutionState(LifecycleStateWorking) },
			next: func(state ExecutionState) ExecutionState {
				state.Plan = copyPlan(state.Plan)
				state.Plan.Steps = append([]PlanStep(nil), state.Plan.Steps...)
				state.Plan.Steps[0].Description = "A different step."
				return state
			},
			wantErr: "step id \"step-001\" was reused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := tt.previous()
			next := tt.next(previous)
			assertErrorContains(t, ValidateExecutionStateTransition(previous, next), tt.wantErr)
		})
	}
}

func TestExecutionStateJSONRoundTrip(t *testing.T) {
	want := validExecutionState(LifecycleStateCompleted)
	want.Plan.Steps = append(want.Plan.Steps, PlanStep{
		ID:          "step-002",
		Description: "Skip an irrelevant optional surface.",
		Status:      PlanStepStatusSkipped,
		Rationale:   "The optional surface is not present.",
		Evidence:    []EvidenceReference{testEvidence(EvidenceKindRepository, "repository-map-1")},
	})
	want.AcceptanceCriteria = []AcceptanceCriterion{
		validAcceptanceCriterion("criterion-satisfied", AcceptanceStatusSatisfied),
		validAcceptanceCriterion("criterion-waived", AcceptanceStatusWaived),
		validAcceptanceCriterion("criterion-na", AcceptanceStatusNotApplicable),
	}
	want.FindingResolutions = []FindingResolution{
		validFindingResolution("finding-resolved", FindingResolutionStatusResolved),
		validFindingResolution("finding-waived", FindingResolutionStatusWaived),
		validFindingResolution("finding-superseded", FindingResolutionStatusSuperseded),
		validFindingResolution("finding-invalid", FindingResolutionStatusInvalid),
	}
	want.Attempts = validAttemptState()
	want.Terminal.Evidence = []EvidenceReference{testEvidence(EvidenceKindLedger, "run-3:completed")}

	assertJSONRoundTrip(t, want, func(got ExecutionState) error { return got.Validate() })
}

func TestAW01JSONFixturesRemainCompatible(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		validate func([]byte) ([]byte, error)
	}{
		{
			name:    "supervisor decision",
			fixture: `{"task_id":"task-1","action":"correct","worker_profile":"corrector","rationale":"Correct the audit finding.","success_criteria":["The finding is resolved."],"inputs":[{"kind":"audit","reference":"audit-1","detail":"The audit recorded a blocking finding."}],"finding_ids":["finding-001"]}`,
			validate: func(raw []byte) ([]byte, error) {
				var decision SupervisorDecision
				if err := json.Unmarshal(raw, &decision); err != nil {
					return nil, err
				}
				if err := decision.Validate(); err != nil {
					return nil, err
				}
				return json.Marshal(decision)
			},
		},
		{
			name:    "audit report",
			fixture: `{"task_id":"task-1","disposition":"changes_required","rationale":"One correction is required.","inputs":[{"kind":"verification","reference":"run-1:tests","detail":"The focused test failed."}],"findings":[{"id":"finding-001","significance":"blocking","summary":"The required behavior is missing.","evidence":[{"kind":"file","reference":"internal/example.go:10","detail":"The branch returns early."}],"required_correction":"Apply the update before returning."}]}`,
			validate: func(raw []byte) ([]byte, error) {
				var report AuditReport
				if err := json.Unmarshal(raw, &report); err != nil {
					return nil, err
				}
				if err := report.Validate(); err != nil {
					return nil, err
				}
				return json.Marshal(report)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.validate([]byte(tt.fixture))
			if err != nil {
				t.Fatalf("fixture validation error = %v", err)
			}
			if string(got) != tt.fixture {
				t.Fatalf("round-trip JSON = %s, want %s", got, tt.fixture)
			}
		})
	}
}

func validTaskPlan(completed bool) TaskPlan {
	status := PlanStepStatusPending
	if completed {
		status = PlanStepStatusCompleted
	}
	return TaskPlan{
		TaskID:     "task-1",
		ID:         "plan-001",
		Revision:   1,
		Provenance: []EvidenceReference{testEvidence(EvidenceKindTask, ".agent/tasks/task-1.md")},
		Steps:      []PlanStep{validPlanStep(status)},
		Completed:  completed,
	}
}

func validPlanStep(status PlanStepStatus) PlanStep {
	step := PlanStep{
		ID:          "step-001",
		Description: "Implement and verify the requested contract.",
		Status:      status,
	}
	switch status {
	case PlanStepStatusCompleted:
		step.Evidence = []EvidenceReference{testEvidence(EvidenceKindVerification, "run-1:tests")}
	case PlanStepStatusSkipped:
		step.Rationale = "The step is not relevant to this task."
	}
	return step
}

func validAcceptanceCriterion(id string, status AcceptanceStatus) AcceptanceCriterion {
	source := testEvidence(EvidenceKindTask, ".agent/tasks/task-1.md#acceptance")
	criterion := AcceptanceCriterion{
		ID:          id,
		Requirement: "The durable contract is validated by focused tests.",
		Status:      status,
		Source:      &source,
	}
	switch status {
	case AcceptanceStatusSatisfied:
		criterion.Evidence = []EvidenceReference{testEvidence(EvidenceKindVerification, "run-1:tests")}
	case AcceptanceStatusWaived:
		criterion.Rationale = "The operator explicitly waived this optional criterion."
	case AcceptanceStatusNotApplicable:
		criterion.Rationale = "The referenced surface does not exist in this repository."
	}
	return criterion
}

func validFindingResolution(id string, status FindingResolutionStatus) FindingResolution {
	resolution := FindingResolution{FindingID: id, Status: status}
	switch status {
	case FindingResolutionStatusResolved:
		decision := validDecisionReference(ActionCorrect)
		decision.DecisionID = "decision-resolution"
		resolution.Evidence = []EvidenceReference{testEvidence(EvidenceKindVerification, "run-2:tests")}
		resolution.Resolution = &decision
	case FindingResolutionStatusWaived:
		resolution.Rationale = "The operator accepted the documented non-blocking risk."
	case FindingResolutionStatusSuperseded:
		resolution.SupersedingFindingID = "finding-replacement"
	case FindingResolutionStatusInvalid:
		resolution.Rationale = "The cited line is unreachable in the audited revision."
	}
	return resolution
}

func validDecisionReference(action Action) DecisionReference {
	profile, workerAction := workerProfileForAction(action)
	if !workerAction {
		profile = ""
	}
	return DecisionReference{
		DecisionID:    "decision-001",
		RunID:         "019f4b32-0d1a-7000-8000-000000000001",
		TaskID:        "task-1",
		Action:        action,
		WorkerProfile: profile,
		Artifact:      testEvidence(EvidenceKindLedger, "run-1:supervisor-decision"),
		CreatedAt:     time.Date(2026, 7, 10, 14, 30, 0, 0, time.UTC),
	}
}

func zeroAttemptState() AttemptState {
	return AttemptState{
		RetryBudget:       CountBudget{Mode: BudgetModeUnset},
		ElapsedTimeBudget: DurationBudget{Mode: BudgetModeUnset},
		TokenBudget:       CountBudget{Mode: BudgetModeUnset},
	}
}

func validAttemptState() AttemptState {
	lastFailure := testEvidence(EvidenceKindLedger, "run-1:verification-failed")
	return AttemptState{
		TotalAttempts: 1,
		ActionAttempts: []ActionAttempt{
			{Action: ActionImplement, Attempts: 1},
		},
		ConsecutiveFailures: 1,
		RetryBudget:         CountBudget{Mode: BudgetModeLimited, Limit: 3, Consumed: 1},
		ElapsedTimeBudget:   DurationBudget{Mode: BudgetModeLimited, Limit: time.Hour, Consumed: 5 * time.Minute},
		TokenBudget:         CountBudget{Mode: BudgetModeUnlimited, Consumed: 1200},
		LastFailure:         &lastFailure,
	}
}

func validExecutionState(lifecycle LifecycleState) ExecutionState {
	if lifecycle == LifecycleStateCompleted {
		decision := validDecisionReference(ActionComplete)
		return ExecutionState{
			SchemaVersion:      ExecutionStateSchemaVersion,
			TaskID:             "task-1",
			Lifecycle:          lifecycle,
			Plan:               planPointer(validTaskPlan(true)),
			AcceptanceCriteria: []AcceptanceCriterion{validAcceptanceCriterion("criterion-001", AcceptanceStatusSatisfied)},
			FindingResolutions: []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusResolved)},
			LatestDecision:     &decision,
			Attempts:           zeroAttemptState(),
			Terminal: &TerminalDetail{
				Reason: "All durable work is structurally complete.",
			},
		}
	}

	decision := validDecisionReference(ActionImplement)
	state := ExecutionState{
		SchemaVersion:      ExecutionStateSchemaVersion,
		TaskID:             "task-1",
		Lifecycle:          lifecycle,
		Plan:               planPointer(validTaskPlan(false)),
		AcceptanceCriteria: []AcceptanceCriterion{validAcceptanceCriterion("criterion-001", AcceptanceStatusPending)},
		FindingResolutions: []FindingResolution{validFindingResolution("finding-001", FindingResolutionStatusOpen)},
		LatestDecision:     &decision,
		Attempts:           zeroAttemptState(),
	}
	switch lifecycle {
	case LifecycleStateNeedsInput:
		state.NeedsInput = &NeedsInputDetail{Reason: "The task specification leaves two incompatible behaviors unresolved."}
	case LifecycleStateFinalizing:
		state.Finalization = &FinalizationDetail{
			SchemaVersion: FinalizationDetailSchemaVersion,
			OperationID:   "finalize-one",
			RunID:         "finalization-run",
			Stage:         FinalizationStageAdmitted,
			FrozenEvidence: FinalizationArtifact{
				Path: ".revolvr/autonomous/tasks/task-1/completion/completion-evidence.json", SHA256: strings.Repeat("f", 64), ByteSize: 10,
			},
			OriginalTaskSHA256: strings.Repeat("e", 64),
			AdmittedAt:         time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC),
		}
	case LifecycleStateBlocked, LifecycleStateCancelled, LifecycleStateSuperseded, LifecycleStateAbandoned:
		state.Terminal = &TerminalDetail{Reason: "Execution stopped with an explicit durable reason."}
	}
	return state
}

func copyPlan(plan *TaskPlan) *TaskPlan {
	if plan == nil {
		return nil
	}
	copy := *plan
	copy.Provenance = append([]EvidenceReference(nil), plan.Provenance...)
	copy.Steps = append([]PlanStep(nil), plan.Steps...)
	return &copy
}

func planPointer(plan TaskPlan) *TaskPlan {
	return &plan
}

func testEvidence(kind EvidenceKind, reference string) EvidenceReference {
	return EvidenceReference{
		Kind:      kind,
		Reference: reference,
		Detail:    "Durable evidence supports this recorded state.",
	}
}
