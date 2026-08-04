package tasklifecycle

import (
	"errors"
	"testing"
)

func TestTaskTransitionMatrix(t *testing.T) {
	statuses := []TaskStatus{
		TaskDraft, TaskCompiled, TaskAwaitingApproval, TaskPending, TaskAdmitted,
		TaskPlanning, TaskReady, TaskWorking, TaskVerifying, TaskAuditing,
		TaskCorrecting, TaskDocumenting, TaskSimplifying, TaskNeedsInput,
		TaskBlocked, TaskFinalizing, TaskCompleted, TaskCancelled,
		TaskBudgetExhausted, TaskUnsafe, TaskRetrieval, TaskTelemetry,
	}
	legal := make(map[taskEdge][]Authority)
	allow := func(from, to TaskStatus, authorities ...Authority) {
		legal[taskEdge{from: from, to: to}] = authorities
	}

	allow(TaskDraft, TaskCompiled, AuthorityHostValidator)
	allow(TaskCompiled, TaskAwaitingApproval, AuthorityHost)
	allow(TaskAwaitingApproval, TaskPending, AuthorityOperator)
	allow(TaskPending, TaskAdmitted, AuthorityScheduler)
	allow(TaskAdmitted, TaskPlanning, AuthorityPolicy)
	allow(TaskAdmitted, TaskReady, AuthorityPolicy)
	allow(TaskPlanning, TaskReady, AuthorityPlannerHost)
	allow(TaskReady, TaskWorking, AuthoritySupervisorPolicy)
	allow(TaskWorking, TaskVerifying, AuthorityHost)
	allow(TaskVerifying, TaskAuditing, AuthorityHost)
	allow(TaskVerifying, TaskCorrecting, AuthorityHost, AuthorityPolicy)
	allow(TaskAuditing, TaskCorrecting, AuthorityHost, AuthorityPolicy)
	allow(TaskAuditing, TaskDocumenting, AuthorityPolicy)
	allow(TaskAuditing, TaskSimplifying, AuthorityPolicy)
	allow(TaskAuditing, TaskFinalizing, AuthorityPolicy)
	allow(TaskCorrecting, TaskVerifying, AuthorityHost)
	allow(TaskDocumenting, TaskVerifying, AuthorityHost)
	allow(TaskSimplifying, TaskVerifying, AuthorityHost)
	allow(TaskFinalizing, TaskCompleted, AuthorityCompletionFinalizer)

	active := statuses[:16]
	for _, from := range active {
		if from != TaskNeedsInput {
			allow(from, TaskNeedsInput, AuthoritySupervisorPolicy)
		}
		if from != TaskBlocked {
			allow(from, TaskBlocked, AuthoritySupervisorPolicy)
		}
		allow(from, TaskCancelled, AuthorityOperator, AuthorityHost)
		allow(from, TaskBudgetExhausted, AuthorityHost)
		allow(from, TaskUnsafe, AuthorityHost, AuthorityPolicy)
	}
	for _, terminal := range []TaskStatus{TaskCompleted, TaskCancelled, TaskBudgetExhausted, TaskUnsafe} {
		allow(terminal, TaskPending, AuthorityExplicitReopen)
	}

	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				authorities, ok := legal[taskEdge{from: from, to: to}]
				if !ok {
					if err := ValidateTaskTransition(from, to, AuthorityHost); !errors.Is(err, ErrIllegalTransition) {
						t.Fatalf("ValidateTaskTransition() error = %v, want %v", err, ErrIllegalTransition)
					}
					return
				}
				for _, authority := range authorities {
					if err := ValidateTaskTransition(from, to, authority); err != nil {
						t.Errorf("authority %q: %v", authority, err)
					}
				}
				if err := ValidateTaskTransition(from, to, AuthorityModel); !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("model authority error = %v, want %v", err, ErrUnauthorized)
				}
			})
		}
	}
}

func TestCriterionTransitionMatrix(t *testing.T) {
	statuses := []CriterionStatus{
		CriterionPending, CriterionPassed, CriterionFailed,
		CriterionWaived, CriterionNotApplicable, CriterionBlocked,
	}
	type criterionEdge struct {
		from CriterionStatus
		to   CriterionStatus
	}
	legal := map[criterionEdge]CriterionConditions{
		{CriterionPending, CriterionPassed}:        {},
		{CriterionPending, CriterionFailed}:        {},
		{CriterionPending, CriterionWaived}:        {},
		{CriterionPending, CriterionNotApplicable}: {},
		{CriterionPending, CriterionBlocked}:       {},
		{CriterionFailed, CriterionPassed}:         {FreshEvidence: true},
		{CriterionBlocked, CriterionPassed}:        {BlockerResolved: true},
		{CriterionBlocked, CriterionWaived}:        {OperatorAuthorized: true},
	}

	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				conditions, ok := legal[criterionEdge{from: from, to: to}]
				if !ok {
					if err := ValidateCriterionTransition(from, to, CriterionConditions{}); !errors.Is(err, ErrIllegalTransition) {
						t.Fatalf("ValidateCriterionTransition() error = %v, want %v", err, ErrIllegalTransition)
					}
					return
				}
				if err := ValidateCriterionTransition(from, to, conditions); err != nil {
					t.Fatal(err)
				}
				if conditions != (CriterionConditions{}) {
					want := ErrPrecondition
					if conditions.OperatorAuthorized {
						want = ErrUnauthorized
					}
					if err := ValidateCriterionTransition(from, to, CriterionConditions{}); !errors.Is(err, want) {
						t.Fatalf("missing condition error = %v, want %v", err, want)
					}
				}
			})
		}
	}
}
