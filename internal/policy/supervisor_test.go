package policy

import (
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/tasklifecycle"
)

func TestSupervisorPolicyReturnsTrustedRoutesWithoutMutatingInput(t *testing.T) {
	input := policyTestInput(ActionImplement)
	want := clonePolicyInput(input)
	route, err := RouteSupervisor(input)
	if err != nil {
		t.Fatal(err)
	}
	if route.Kind != RouteWorkerRequest || route.WorkerRole != "implementer" || route.ProposedStatus != tasklifecycle.TaskWorking {
		t.Fatalf("route = %#v", route)
	}
	if !reflect.DeepEqual(input, want) {
		t.Fatal("host policy mutated its input")
	}

	complete := policyTestInput(ActionComplete)
	route, err = RouteSupervisor(complete)
	if err != nil {
		t.Fatal(err)
	}
	if route.Kind != RouteCompletionPreflight || route.ProposedStatus != tasklifecycle.TaskFinalizing {
		t.Fatalf("complete route = %#v", route)
	}
}

func TestSupervisorPolicyFailsClosedOnLifecycleBudgetScopeAndCompletion(t *testing.T) {
	tests := []struct {
		name   string
		input  Input
		needle string
	}{
		{name: "lifecycle", input: func() Input {
			in := policyTestInput(ActionImplement)
			in.Lifecycle = tasklifecycle.TaskAuditing
			return in
		}(), needle: "does not admit"},
		{name: "budget", input: func() Input { in := policyTestInput(ActionImplement); in.Budget.ModelCallsRemaining = 1; return in }(), needle: "budget is exhausted"},
		{name: "scope", input: func() Input {
			in := policyTestInput(ActionImplement)
			in.Proposal.Scope = []string{"outside/file.go"}
			return in
		}(), needle: "broadens"},
		{name: "verification", input: func() Input { in := policyTestInput(ActionComplete); in.Verification = nil; return in }(), needle: "verification"},
		{name: "audit", input: func() Input { in := policyTestInput(ActionComplete); in.Audit = nil; return in }(), needle: "audit"},
		{name: "manifest", input: func() Input { in := policyTestInput(ActionComplete); in.ArtifactManifestComplete = false; return in }(), needle: "manifest"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := RouteSupervisor(test.input)
			if err == nil || !strings.Contains(err.Error(), test.needle) {
				t.Fatalf("err = %v, want %q", err, test.needle)
			}
		})
	}
}

func TestSupervisorPolicyIdentityIsExact(t *testing.T) {
	identity := CurrentIdentity()
	if identity.Version != SupervisorPolicyVersion || len(identity.SHA256) != 64 {
		t.Fatalf("identity = %#v", identity)
	}
	if err := ValidateIdentity(identity); err != nil {
		t.Fatal(err)
	}
	identity.SHA256 = strings.Repeat("0", 64)
	if err := ValidateIdentity(identity); err == nil {
		t.Fatal("stale policy identity was accepted")
	}
}

func policyTestInput(action Action) Input {
	revision := strings.Repeat("a", 64)
	input := Input{
		TaskID: "task-1", Lifecycle: tasklifecycle.TaskReady, SourceRevision: revision, SourceSafe: true,
		Budget: Budget{IdentityID: "budget-1", ModelCallsRemaining: 3, WorkerAttemptsRemaining: 2, TokensRemaining: 1000}, SupervisorUsageTokens: 10,
		Scope:               Scope{AllowedPaths: []string{"internal"}, ExcludedPaths: []string{"internal/generated"}},
		Plan:                &PlanGate{ID: "plan-1", Steps: []PlanStepGate{{ID: "step-1", Status: "pending"}}},
		Criteria:            []CriterionGate{{ID: "criterion-1", Status: "pending"}},
		WorkspaceReconciled: true, ArtifactManifestID: "manifest-1", ArtifactManifestComplete: true,
		Proposal: Proposal{DecisionID: "decision-1", Action: action, Scope: []string{"internal/file.go"}},
	}
	switch action {
	case ActionComplete:
		input.Lifecycle = tasklifecycle.TaskAuditing
		input.Plan.Completed = true
		input.Plan.Steps[0].Status = "completed"
		input.Criteria[0].Status = "passed"
		input.Verification = &VerificationGate{ID: "verification-1", Status: "passed", SourceRevision: revision, Final: true, EvidenceComplete: true}
		input.Audit = &AuditGate{ID: "audit-1", Status: "clean", SourceRevision: revision, VerificationID: "verification-1", Independent: true, EvidenceComplete: true}
		input.Proposal.Scope = nil
		input.Proposal.Completion = &CompletionEvidence{PlanID: "plan-1", CriterionIDs: []string{"criterion-1"}, VerificationID: "verification-1", AuditID: "audit-1", ArtifactManifestID: "manifest-1"}
	}
	return input
}

func clonePolicyInput(input Input) Input {
	cloned := input
	cloned.Scope.AllowedPaths = append([]string(nil), input.Scope.AllowedPaths...)
	cloned.Scope.ExcludedPaths = append([]string(nil), input.Scope.ExcludedPaths...)
	cloned.Criteria = append([]CriterionGate(nil), input.Criteria...)
	cloned.Findings = append([]FindingGate(nil), input.Findings...)
	cloned.Proposal.Scope = append([]string(nil), input.Proposal.Scope...)
	if input.Plan != nil {
		plan := *input.Plan
		plan.Steps = append([]PlanStepGate(nil), input.Plan.Steps...)
		cloned.Plan = &plan
	}
	if input.Verification != nil {
		value := *input.Verification
		cloned.Verification = &value
	}
	if input.Audit != nil {
		value := *input.Audit
		cloned.Audit = &value
	}
	if input.Proposal.Completion != nil {
		value := *input.Proposal.Completion
		value.CriterionIDs = append([]string(nil), value.CriterionIDs...)
		cloned.Proposal.Completion = &value
	}
	return cloned
}
