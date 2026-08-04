package tasklifecycle

import "errors"

type TaskStatus string

const (
	TaskDraft            TaskStatus = "draft"
	TaskCompiled         TaskStatus = "compiled"
	TaskAwaitingApproval TaskStatus = "awaiting_approval"
	TaskPending          TaskStatus = "pending"
	TaskAdmitted         TaskStatus = "admitted"
	TaskPlanning         TaskStatus = "planning"
	TaskReady            TaskStatus = "ready"
	TaskWorking          TaskStatus = "working"
	TaskVerifying        TaskStatus = "verifying"
	TaskAuditing         TaskStatus = "auditing"
	TaskCorrecting       TaskStatus = "correcting"
	TaskDocumenting      TaskStatus = "documenting"
	TaskSimplifying      TaskStatus = "simplifying"
	TaskNeedsInput       TaskStatus = "needs_input"
	TaskBlocked          TaskStatus = "blocked"
	TaskFinalizing       TaskStatus = "finalizing"
	TaskCompleted        TaskStatus = "completed"
	TaskCancelled        TaskStatus = "cancelled"
	TaskBudgetExhausted  TaskStatus = "budget_exhausted"
	TaskUnsafe           TaskStatus = "unsafe"
	TaskSuperseded       TaskStatus = "superseded"
	TaskAbandoned        TaskStatus = "abandoned"
	TaskRetrieval        TaskStatus = "retrieval"
	TaskTelemetry        TaskStatus = "telemetry"
)

type Authority string

const (
	AuthorityHostValidator       Authority = "host_validator"
	AuthorityHost                Authority = "host"
	AuthorityOperator            Authority = "operator"
	AuthorityScheduler           Authority = "scheduler"
	AuthorityPolicy              Authority = "policy"
	AuthorityPlannerHost         Authority = "planner_host"
	AuthoritySupervisorPolicy    Authority = "supervisor_policy"
	AuthorityCompletionFinalizer Authority = "completion_finalizer"
	AuthorityExplicitReopen      Authority = "explicit_reopen"
	AuthorityModel               Authority = "model"
)

var (
	ErrIllegalTransition = errors.New("illegal lifecycle transition")
	ErrUnauthorized      = errors.New("lifecycle authority is not permitted")
	ErrPrecondition      = errors.New("lifecycle transition precondition is not satisfied")
)

type taskEdge struct {
	from TaskStatus
	to   TaskStatus
}

var taskTransitions = buildTaskTransitions()

func buildTaskTransitions() map[taskEdge]map[Authority]struct{} {
	transitions := make(map[taskEdge]map[Authority]struct{})
	allow := func(from, to TaskStatus, authorities ...Authority) {
		key := taskEdge{from: from, to: to}
		if transitions[key] == nil {
			transitions[key] = make(map[Authority]struct{})
		}
		for _, authority := range authorities {
			transitions[key][authority] = struct{}{}
		}
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

	active := []TaskStatus{
		TaskDraft, TaskCompiled, TaskAwaitingApproval, TaskPending, TaskAdmitted,
		TaskPlanning, TaskReady, TaskWorking, TaskVerifying, TaskAuditing,
		TaskCorrecting, TaskDocumenting, TaskSimplifying, TaskNeedsInput,
		TaskBlocked, TaskFinalizing,
	}
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
	return transitions
}

func ValidateTaskTransition(from, to TaskStatus, authority Authority) error {
	authorities, ok := taskTransitions[taskEdge{from: from, to: to}]
	if !ok {
		return ErrIllegalTransition
	}
	if _, ok := authorities[authority]; !ok {
		return ErrUnauthorized
	}
	return nil
}

type CriterionStatus string

const (
	CriterionPending       CriterionStatus = "pending"
	CriterionPassed        CriterionStatus = "passed"
	CriterionFailed        CriterionStatus = "failed"
	CriterionWaived        CriterionStatus = "waived"
	CriterionNotApplicable CriterionStatus = "not_applicable"
	CriterionBlocked       CriterionStatus = "blocked"
)

type CriterionConditions struct {
	FreshEvidence      bool
	BlockerResolved    bool
	OperatorAuthorized bool
}

func ValidateCriterionTransition(from, to CriterionStatus, conditions CriterionConditions) error {
	switch {
	case from == CriterionPending && (to == CriterionPassed || to == CriterionFailed ||
		to == CriterionWaived || to == CriterionNotApplicable || to == CriterionBlocked):
		return nil
	case from == CriterionFailed && to == CriterionPassed:
		if conditions.FreshEvidence {
			return nil
		}
		return ErrPrecondition
	case from == CriterionBlocked && to == CriterionPassed:
		if conditions.BlockerResolved {
			return nil
		}
		return ErrPrecondition
	case from == CriterionBlocked && to == CriterionWaived:
		if conditions.OperatorAuthorized {
			return nil
		}
		return ErrUnauthorized
	default:
		return ErrIllegalTransition
	}
}
