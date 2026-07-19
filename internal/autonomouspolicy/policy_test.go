package autonomouspolicy

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousverification"
)

const taskID = "task-1"

var (
	currentRevision = strings.Repeat("a", 64)
	staleRevision   = strings.Repeat("b", 64)
	fixedTime       = time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
)

func TestEvaluateCommonValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  autonomous.Action
		mutate  func(*Input)
		wantErr string
	}{
		{name: "valid identity", action: autonomous.ActionPlan},
		{name: "decision task mismatch", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Decision.TaskID = "task-2" }, wantErr: "decision task_id"},
		{name: "reference task mismatch", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Reference.TaskID = "task-2" }, wantErr: "decision reference task_id"},
		{name: "state task mismatch", action: autonomous.ActionPlan, mutate: func(in *Input) { in.State.TaskID = "task-2" }, wantErr: "execution state task_id"},
		{name: "verification task mismatch", action: autonomous.ActionAudit, mutate: func(in *Input) { in.Verification.Summary.TaskID = "task-2" }, wantErr: "verification task_id"},
		{name: "audit task mismatch", action: autonomous.ActionDocument, mutate: func(in *Input) { in.Audit.Report.TaskID = "task-2" }, wantErr: "audit task_id"},
		{name: "mutation task mismatch", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Source.LatestMutation.TaskID = "task-2" }, wantErr: "latest source mutation task_id"},
		{name: "decision reference action mismatch", action: autonomous.ActionPlan, mutate: func(in *Input) {
			in.Reference.Action = autonomous.ActionImplement
			in.Reference.WorkerProfile = autonomous.WorkerProfileImplementer
		}, wantErr: "reference action"},
		{name: "decision reference profile mismatch", action: autonomous.ActionImplement, mutate: func(in *Input) { in.Reference.WorkerProfile = autonomous.WorkerProfileAuditor }, wantErr: "decision reference gate"},
		{name: "invalid decision", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Decision.Rationale = "" }, wantErr: "supervisor decision gate"},
		{name: "unknown decision action", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Decision.Action = "review" }, wantErr: "unknown action"},
		{name: "unknown decision profile", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Decision.WorkerProfile = "reviewer" }, wantErr: "worker_profile"},
		{name: "invalid decision reference", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Reference.RunID = "" }, wantErr: "decision reference gate"},
		{name: "invalid execution state", action: autonomous.ActionPlan, mutate: func(in *Input) { in.State.SchemaVersion = "future" }, wantErr: "execution state gate"},
		{name: "unknown execution lifecycle", action: autonomous.ActionPlan, mutate: func(in *Input) { in.State.Lifecycle = "paused" }, wantErr: "unknown lifecycle"},
		{name: "invalid verification evidence", action: autonomous.ActionAudit, mutate: func(in *Input) { in.Verification.Summary.Evidence = nil }, wantErr: "verification evidence gate"},
		{name: "unknown verification status", action: autonomous.ActionAudit, mutate: func(in *Input) { in.Verification.Summary.Status = "partial" }, wantErr: "unknown status"},
		{name: "unknown verification evidence kind", action: autonomous.ActionAudit, mutate: func(in *Input) { in.Verification.Summary.Evidence[0].Kind = "build" }, wantErr: "unknown kind"},
		{name: "invalid audit evidence", action: autonomous.ActionDocument, mutate: func(in *Input) { in.Audit.AuditorProfile = "reviewer" }, wantErr: "audit has unknown profile"},
		{name: "missing audit run identity", action: autonomous.ActionDocument, mutate: func(in *Input) { in.Audit.RunID = "" }, wantErr: "audit run_id"},
		{name: "empty source revision", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Source.Revision = "" }, wantErr: "current source revision"},
		{name: "malformed source revision", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Source.Revision = strings.Repeat("A", 64) }, wantErr: "current source revision"},
		{name: "unknown source safety", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Source.Safety = "maybe" }, wantErr: "unknown source safety status"},
		{name: "invalid latest mutation action", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Source.LatestMutation.Action = autonomous.ActionAudit }, wantErr: "is not source-changing"},
		{name: "materially reused decision identity", action: autonomous.ActionPlan, mutate: func(in *Input) {
			prior := in.Reference
			prior.RunID = "run-prior"
			in.State.FindingResolutions = []autonomous.FindingResolution{resolvedFinding("finding-prior", &prior)}
		}, wantErr: "materially different"},
		{name: "latest decision replay", action: autonomous.ActionPlan, mutate: func(in *Input) {
			latest := in.Reference
			in.State.LatestDecision = &latest
		}, wantErr: "replays the execution state's latest decision"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput(tt.action)
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			route, err := Evaluate(in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Evaluate() error = %v", err)
				}
				assertRoute(t, route, in)
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Evaluate() error = %v, want %q", err, tt.wantErr)
			}
			if route != (Route{}) {
				t.Fatalf("rejected route = %+v, want zero", route)
			}
		})
	}
}

func TestEvaluateIsDeterministicAndDoesNotMutateInput(t *testing.T) {
	for _, action := range allActions() {
		t.Run(string(action), func(t *testing.T) {
			in := validInput(action)
			before := mustJSON(t, in)
			first, firstErr := Evaluate(in)
			second, secondErr := Evaluate(in)
			if firstErr != nil || secondErr != nil || first != second || errorText(firstErr) != errorText(secondErr) {
				t.Fatalf("repeated Evaluate() = (%+v, %v), (%+v, %v)", first, firstErr, second, secondErr)
			}
			if after := mustJSON(t, in); string(after) != string(before) {
				t.Fatalf("input changed\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}

	in := validInput(autonomous.ActionComplete)
	in.Verification.SourceRevision = staleRevision
	before := mustJSON(t, in)
	_, firstErr := Evaluate(in)
	_, secondErr := Evaluate(in)
	if firstErr == nil || errorText(firstErr) != errorText(secondErr) {
		t.Fatalf("repeated errors = %v / %v", firstErr, secondErr)
	}
	if after := mustJSON(t, in); string(after) != string(before) {
		t.Fatal("rejected evaluation mutated input")
	}
}

func TestEvaluateLifecycleMatrix(t *testing.T) {
	lifecycles := []autonomous.LifecycleState{
		autonomous.LifecycleStatePending,
		autonomous.LifecycleStateReady,
		autonomous.LifecycleStatePlanning,
		autonomous.LifecycleStateWorking,
		autonomous.LifecycleStateVerifying,
		autonomous.LifecycleStateAuditing,
		autonomous.LifecycleStateCorrecting,
		autonomous.LifecycleStateNeedsInput,
		autonomous.LifecycleStateFinalizing,
		autonomous.LifecycleStateCompleted,
		autonomous.LifecycleStateBlocked,
		autonomous.LifecycleStateCancelled,
		autonomous.LifecycleStateSuperseded,
		autonomous.LifecycleStateAbandoned,
	}
	for _, action := range allActions() {
		for _, lifecycle := range lifecycles {
			t.Run(string(action)+"/"+string(lifecycle), func(t *testing.T) {
				in := validInput(action)
				setLifecycle(&in.State, lifecycle)
				_, err := Evaluate(in)
				authority, authorityErr := RoutingAuthorityForLifecycle(lifecycle)
				wantAllowed := authorityErr == nil && authority.Admits(action)
				if wantAllowed && err != nil {
					t.Fatalf("Evaluate() error = %v", err)
				}
				if !wantAllowed && (err == nil || !strings.Contains(err.Error(), "lifecycle gate")) {
					t.Fatalf("Evaluate() error = %v, want lifecycle rejection", err)
				}
			})
		}
	}
}

func TestRoutingAuthorityForLifecycleIsExactAndDeterministic(t *testing.T) {
	all := allActions()
	tests := []struct {
		lifecycle autonomous.LifecycleState
		want      []autonomous.Action
		wantErr   string
	}{
		{lifecycle: autonomous.LifecycleStatePending, want: []autonomous.Action{autonomous.ActionPlan, autonomous.ActionBlock, autonomous.ActionNeedsInput}},
		{lifecycle: autonomous.LifecycleStateReady, want: all},
		{lifecycle: autonomous.LifecycleStatePlanning, wantErr: "operation in flight"},
		{lifecycle: autonomous.LifecycleStateWorking, wantErr: "operation in flight"},
		{lifecycle: autonomous.LifecycleStateVerifying, wantErr: "operation in flight"},
		{lifecycle: autonomous.LifecycleStateAuditing, wantErr: "operation in flight"},
		{lifecycle: autonomous.LifecycleStateCorrecting, wantErr: "operation in flight"},
		{lifecycle: autonomous.LifecycleStateNeedsInput, wantErr: "exact durable answer"},
		{lifecycle: autonomous.LifecycleStateFinalizing, wantErr: "admits no new routing"},
		{lifecycle: autonomous.LifecycleStateCompleted, wantErr: "terminal lifecycle"},
		{lifecycle: autonomous.LifecycleStateBlocked, wantErr: "terminal lifecycle"},
		{lifecycle: autonomous.LifecycleStateCancelled, wantErr: "terminal lifecycle"},
		{lifecycle: autonomous.LifecycleStateSuperseded, wantErr: "terminal lifecycle"},
		{lifecycle: autonomous.LifecycleStateAbandoned, wantErr: "terminal lifecycle"},
		{lifecycle: autonomous.LifecycleState("unknown"), wantErr: "unknown lifecycle"},
	}
	for _, tt := range tests {
		t.Run(string(tt.lifecycle), func(t *testing.T) {
			first, firstErr := RoutingAuthorityForLifecycle(tt.lifecycle)
			second, secondErr := RoutingAuthorityForLifecycle(tt.lifecycle)
			if errorText(firstErr) != errorText(secondErr) || !reflect.DeepEqual(first, second) {
				t.Fatalf("repeated authority = %+v/%v and %+v/%v", first, firstErr, second, secondErr)
			}
			if tt.wantErr != "" {
				if firstErr == nil || !strings.Contains(firstErr.Error(), tt.wantErr) || len(first.AdmittedActions) != 0 {
					t.Fatalf("authority = %+v, error = %v; want closed %q", first, firstErr, tt.wantErr)
				}
				return
			}
			if firstErr != nil || first.SchemaVersion != LifecycleRoutingAuthoritySchemaVersion || first.Lifecycle != tt.lifecycle || !reflect.DeepEqual(first.AdmittedActions, tt.want) {
				t.Fatalf("authority = %+v, error = %v; want actions %v", first, firstErr, tt.want)
			}
			if err := first.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			for _, action := range all {
				if first.Admits(action) != slices.Contains(tt.want, action) {
					t.Fatalf("Admits(%q) = %t, want %t", action, first.Admits(action), slices.Contains(tt.want, action))
				}
			}
		})
	}

	pending, err := RoutingAuthorityForLifecycle(autonomous.LifecycleStatePending)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Admits(autonomous.ActionImplement) {
		t.Fatal("pending lifecycle authority admitted implement")
	}
	pending.AdmittedActions[0] = autonomous.ActionImplement
	if pending.Validate() == nil || pending.Admits(autonomous.ActionImplement) {
		t.Fatal("mutated lifecycle authority did not fail closed")
	}
}

func TestEvaluateWorkerRoutes(t *testing.T) {
	tests := []struct {
		action  autonomous.Action
		profile autonomous.WorkerProfile
	}{
		{autonomous.ActionPlan, autonomous.WorkerProfilePlanner},
		{autonomous.ActionImplement, autonomous.WorkerProfileImplementer},
		{autonomous.ActionAudit, autonomous.WorkerProfileAuditor},
		{autonomous.ActionCorrect, autonomous.WorkerProfileCorrector},
		{autonomous.ActionDocument, autonomous.WorkerProfileDocumentor},
		{autonomous.ActionSimplify, autonomous.WorkerProfileSimplifier},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			in := validInput(tt.action)
			route, err := Evaluate(in)
			if err != nil || route.Kind != RouteKindWorker || route.WorkerProfile != tt.profile {
				t.Fatalf("Evaluate() = %+v, %v; want worker/%q", route, err, tt.profile)
			}

			wrong := validInput(tt.action)
			wrongProfile := autonomous.WorkerProfilePlanner
			if wrongProfile == tt.profile {
				wrongProfile = autonomous.WorkerProfileAuditor
			}
			wrong.Decision.WorkerProfile = wrongProfile
			wrong.Reference.WorkerProfile = wrongProfile
			if _, err := Evaluate(wrong); err == nil || !strings.Contains(err.Error(), "worker_profile") {
				t.Fatalf("wrong-profile error = %v", err)
			}

			for _, safety := range []SourceSafety{SourceSafetyUnsafe, SourceSafetyUnknown} {
				unsafe := validInput(tt.action)
				unsafe.Source.Safety = safety
				if _, err := Evaluate(unsafe); err == nil || !strings.Contains(err.Error(), "source safety gate") {
					t.Fatalf("safety %q error = %v", safety, err)
				}
			}

			active := validInput(tt.action)
			setLifecycle(&active.State, autonomous.LifecycleStateWorking)
			if _, err := Evaluate(active); err == nil || !strings.Contains(err.Error(), "lifecycle gate") {
				t.Fatalf("active lifecycle error = %v", err)
			}
		})
	}
}

func TestEvaluatePlanAndImplementation(t *testing.T) {
	tests := []struct {
		name    string
		action  autonomous.Action
		mutate  func(*Input)
		wantErr string
	}{
		{name: "initial planning", action: autonomous.ActionPlan},
		{name: "planning later revision remains possible", action: autonomous.ActionPlan, mutate: func(in *Input) { in.State.Plan = completedPlan() }},
		{name: "plan requires no verification or audit", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Verification, in.Audit = nil, nil }},
		{name: "plan rejects unresolved blocking finding", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Audit = changesAudit(blockingFinding("finding-one")) }, wantErr: "current blocking audit finding"},
		{name: "plan allows unresolved nonblocking finding", action: autonomous.ActionPlan, mutate: func(in *Input) { in.Audit = changesAudit(nonBlockingFinding("finding-one")) }},
		{name: "implement pending step", action: autonomous.ActionImplement},
		{name: "implement in-progress step", action: autonomous.ActionImplement, mutate: func(in *Input) { in.State.Plan.Steps[0].Status = autonomous.PlanStepStatusInProgress }},
		{name: "implement without plan", action: autonomous.ActionImplement, mutate: func(in *Input) { in.State.Plan = nil }, wantErr: "current plan is required"},
		{name: "implement completed plan", action: autonomous.ActionImplement, mutate: func(in *Input) { in.State.Plan = completedPlan() }, wantErr: "plan \"plan-one\" is completed"},
		{name: "implement only terminal steps", action: autonomous.ActionImplement, mutate: func(in *Input) {
			in.State.Plan = completedPlan()
			in.State.Plan.Completed = false
		}, wantErr: "has no pending or in_progress step"},
		{name: "implement rejects unresolved audit finding", action: autonomous.ActionImplement, mutate: func(in *Input) { in.Audit = changesAudit(nonBlockingFinding("finding-one")) }, wantErr: "requires correction"},
		{name: "implement allows terminally resolved audit finding", action: autonomous.ActionImplement, mutate: func(in *Input) {
			in.Audit = changesAudit(blockingFinding("finding-one"))
			in.State.FindingResolutions = []autonomous.FindingResolution{resolvedFinding("finding-one", nil)}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput(tt.action)
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			_, err := Evaluate(in)
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestEvaluateVerificationAndAuditFreshness(t *testing.T) {
	verificationTests := []struct {
		name    string
		mutate  func(*Input)
		wantErr string
	}{
		{name: "passed current verification"},
		{name: "failed verification", mutate: func(in *Input) { in.Verification.Summary.Status = autonomous.VerificationStatusFailed }, wantErr: "want passed"},
		{name: "missing verification", mutate: func(in *Input) { in.Verification = nil }, wantErr: "verification is missing"},
		{name: "stale verification", mutate: func(in *Input) { in.Verification.SourceRevision = staleRevision }, wantErr: "is stale"},
		{name: "verification wrong task", mutate: func(in *Input) { in.Verification.Summary.TaskID = "task-2" }, wantErr: "verification task_id"},
		{name: "verification missing run", mutate: func(in *Input) { in.Verification.Summary.RunID = "" }, wantErr: "verification run_id"},
		{name: "verification missing occurrence", mutate: func(in *Input) { in.Verification.Summary.OccurrenceID = "" }, wantErr: "verification occurrence_id"},
	}
	for _, tt := range verificationTests {
		t.Run("audit action/"+tt.name, func(t *testing.T) {
			in := validInput(autonomous.ActionAudit)
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			_, err := Evaluate(in)
			assertError(t, err, tt.wantErr)
		})
	}

	auditTests := []struct {
		name    string
		mutate  func(*Input)
		wantErr string
	}{
		{name: "current verification and independent clean audit"},
		{name: "audit wrong task", mutate: func(in *Input) { in.Audit.Report.TaskID = "task-2" }, wantErr: "audit task_id"},
		{name: "audit wrong profile", mutate: func(in *Input) { in.Audit.AuditorProfile = autonomous.WorkerProfileImplementer }, wantErr: "used profile"},
		{name: "audit stale source", mutate: func(in *Input) { in.Audit.SourceRevision = staleRevision }, wantErr: "audit gate: run \"run-audit\" is stale"},
		{name: "audit different verification run", mutate: func(in *Input) { in.Audit.VerificationRunID = "run-old-verification" }, wantErr: "consumed verification"},
		{name: "audit different verification occurrence", mutate: func(in *Input) { in.Audit.VerificationOccurrenceID = "verification-old" }, wantErr: "consumed verification"},
		{name: "audit run equals latest source mutation", mutate: func(in *Input) { in.Audit.RunID = in.Source.LatestMutation.RunID }, wantErr: "audit independence gate"},
		{name: "missing audit", mutate: func(in *Input) { in.Audit = nil }, wantErr: "current audit is missing"},
		{name: "changes-required audit", mutate: func(in *Input) { in.Audit = changesAudit(blockingFinding("finding-one")) }, wantErr: "want clean"},
		{name: "failed verification before audit", mutate: func(in *Input) { in.Verification.Summary.Status = autonomous.VerificationStatusFailed }, wantErr: "want passed"},
		{name: "missing verification before audit", mutate: func(in *Input) { in.Verification = nil }, wantErr: "verification is missing"},
		{name: "stale verification before audit", mutate: func(in *Input) { in.Verification.SourceRevision = staleRevision }, wantErr: "verification gate"},
	}
	for _, tt := range auditTests {
		t.Run("document action/"+tt.name, func(t *testing.T) {
			in := validInput(autonomous.ActionDocument)
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			_, err := Evaluate(in)
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestEvaluateTieredFinalGate(t *testing.T) {
	tests := []struct {
		name string
		gate autonomousverification.GateEvidence
		want string
	}{
		{"current final", testGate(autonomousverification.PurposeFinal, autonomousverification.OutcomePassed, true), ""},
		{"fast only", testGate(autonomousverification.PurposeFast, autonomousverification.OutcomePassed, false), "fast-only"},
		{"failed", testGate(autonomousverification.PurposeFinal, autonomousverification.OutcomeFailed, false), "cannot project as passed"},
		{"flaky", testGate(autonomousverification.PurposeFinal, autonomousverification.OutcomeFlaky, false), "cannot project as passed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput(autonomous.ActionAudit)
			in.Verification.Tiered = &tt.gate
			_, err := Evaluate(in)
			assertError(t, err, tt.want)
		})
	}
}

func testGate(purpose autonomousverification.Purpose, outcome autonomousverification.Outcome, satisfied bool) autonomousverification.GateEvidence {
	gate := autonomousverification.GateEvidence{SchemaVersion: autonomousverification.GateSchemaVersion, Plan: autonomousverification.PlanIdentity{SchemaVersion: autonomousverification.PlanSchemaVersion, SHA256: strings.Repeat("a", 64), ByteSize: 10}, Purpose: purpose, RequiredFinalTiers: []string{"full-suite"}, OverallOutcome: outcome, FinalSatisfied: satisfied}
	if purpose == autonomousverification.PurposeFinal {
		gate.SelectedTiers = []string{"full-suite"}
		gate.ExecutedTiers = []string{"full-suite"}
		gate.RequiredOutcomes = []autonomousverification.TierGate{{TierID: "full-suite", Outcome: outcome}}
	} else {
		gate.MissingRequired = []string{"full-suite"}
	}
	return gate
}

func TestEvaluateCorrection(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Input)
		wantErr string
	}{
		{name: "valid unresolved finding"},
		{name: "valid subset of unresolved findings", mutate: func(in *Input) {
			in.Audit = changesAudit(blockingFinding("finding-one"), nonBlockingFinding("finding-two"))
			in.Decision.FindingIDs = []string{"finding-two"}
		}},
		{name: "unknown finding", mutate: func(in *Input) { in.Decision.FindingIDs = []string{"finding-unknown"} }, wantErr: "does not reference an audit finding"},
		{name: "duplicate finding", mutate: func(in *Input) { in.Decision.FindingIDs = []string{"finding-one", "finding-one"} }, wantErr: "duplicate finding id"},
		{name: "missing audit", mutate: func(in *Input) { in.Audit = nil }, wantErr: "current audit is missing"},
		{name: "clean audit", mutate: func(in *Input) { in.Audit = cleanAudit() }, wantErr: "want \"changes_required\""},
		{name: "stale audit", mutate: func(in *Input) { in.Audit.SourceRevision = staleRevision }, wantErr: "is stale"},
		{name: "wrong-task audit", mutate: func(in *Input) { in.Audit.Report.TaskID = "task-2" }, wantErr: "audit task_id"},
		{name: "already resolved finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{resolvedFinding("finding-one", nil)}
		}, wantErr: "already terminally dispositioned as \"resolved\""},
		{name: "waived finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{terminalFinding("finding-one", autonomous.FindingResolutionStatusWaived)}
		}, wantErr: "as \"waived\""},
		{name: "superseded finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{terminalFinding("finding-one", autonomous.FindingResolutionStatusSuperseded)}
		}, wantErr: "as \"superseded\""},
		{name: "invalid finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{terminalFinding("finding-one", autonomous.FindingResolutionStatusInvalid)}
		}, wantErr: "as \"invalid\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput(autonomous.ActionCorrect)
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			before := mustJSON(t, in)
			_, err := Evaluate(in)
			assertError(t, err, tt.wantErr)
			if got := mustJSON(t, in); string(got) != string(before) {
				t.Fatal("correction evaluation changed report, decision, or resolution state")
			}
		})
	}
}

func TestEvaluateVerificationFailureCorrectionUsesExactTypedTarget(t *testing.T) {
	in := validInput(autonomous.ActionCorrect)
	in.Audit = nil
	in.State.FindingResolutions = nil
	in.Decision.FindingIDs = nil
	in.Verification.Summary.Status = autonomous.VerificationStatusFailed
	target := autonomous.VerificationFailureTarget{TaskID: in.TaskID, RunID: in.Verification.Summary.RunID, OccurrenceID: in.Verification.Summary.OccurrenceID, SourceRevision: in.Verification.SourceRevision, Status: autonomous.VerificationStatusFailed, Evidence: append([]autonomous.EvidenceReference(nil), in.Verification.Summary.Evidence...)}
	in.Decision.VerificationFailure = &target
	in.CorrectionFailure = &target
	if route, err := Evaluate(in); err != nil || route.Action != autonomous.ActionCorrect {
		t.Fatalf("route=%+v err=%v", route, err)
	}
	wrong := target
	wrong.OccurrenceID = "other-occurrence"
	in.CorrectionFailure = &wrong
	if _, err := Evaluate(in); err == nil || !strings.Contains(err.Error(), "does not exactly match") {
		t.Fatalf("wrong authority error=%v", err)
	}
}

func TestEvaluateOptionalDocumentationAndSimplification(t *testing.T) {
	for _, action := range []autonomous.Action{autonomous.ActionDocument, autonomous.ActionSimplify} {
		t.Run(string(action), func(t *testing.T) {
			tests := []struct {
				name    string
				mutate  func(*Input)
				wantErr string
			}{
				{name: "allowed after fresh clean gates"},
				{name: "missing verification", mutate: func(in *Input) { in.Verification = nil }, wantErr: "verification is missing"},
				{name: "failed verification", mutate: func(in *Input) { in.Verification.Summary.Status = autonomous.VerificationStatusFailed }, wantErr: "want passed"},
				{name: "stale verification", mutate: func(in *Input) { in.Verification.SourceRevision = staleRevision }, wantErr: "verification gate"},
				{name: "missing audit", mutate: func(in *Input) { in.Audit = nil }, wantErr: "current audit is missing"},
				{name: "stale audit", mutate: func(in *Input) { in.Audit.SourceRevision = staleRevision }, wantErr: "audit gate"},
				{name: "non-independent audit", mutate: func(in *Input) { in.Audit.RunID = in.Source.LatestMutation.RunID }, wantErr: "audit independence"},
				{name: "changes-required audit", mutate: func(in *Input) { in.Audit = changesAudit(blockingFinding("finding-one")) }, wantErr: "want clean"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					in := validInput(action)
					if tt.mutate != nil {
						tt.mutate(&in)
					}
					_, err := Evaluate(in)
					assertError(t, err, tt.wantErr)
				})
			}
		})
	}

	complete := validInput(autonomous.ActionComplete)
	if len(complete.State.Attempts.ActionAttempts) != 0 {
		t.Fatal("completion fixture unexpectedly records optional role attempts")
	}
	if _, err := Evaluate(complete); err != nil {
		t.Fatalf("completion without document or simplify attempts: %v", err)
	}
	if _, err := Evaluate(validInput(autonomous.ActionSimplify)); err != nil {
		t.Fatalf("simplify without prior document route: %v", err)
	}
}

func TestEvaluateCompletionMatrix(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Input)
		wantErr string
	}{
		{name: "fully valid completion"},
		{name: "missing plan", mutate: func(in *Input) { in.State.Plan = nil }, wantErr: "current plan is required"},
		{name: "incomplete plan", mutate: func(in *Input) { in.State.Plan.Completed = false }, wantErr: "not marked completed"},
		{name: "nonterminal plan step", mutate: func(in *Input) {
			in.State.Plan.Completed = false
			in.State.Plan.Steps[0].Status = autonomous.PlanStepStatusPending
			in.State.Plan.Steps[0].Evidence = nil
		}, wantErr: "step \"step-one\" is nonterminal"},
		{name: "missing acceptance criteria", mutate: func(in *Input) { in.State.AcceptanceCriteria = nil }, wantErr: "at least one acceptance criterion"},
		{name: "pending acceptance", mutate: func(in *Input) {
			in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{acceptance(autonomous.AcceptanceStatusPending)}
		}, wantErr: "is pending"},
		{name: "satisfied acceptance without evidence", mutate: func(in *Input) { in.State.AcceptanceCriteria[0].Evidence = nil }, wantErr: "satisfied evidence requires"},
		{name: "waived acceptance without rationale", mutate: func(in *Input) {
			criterion := acceptance(autonomous.AcceptanceStatusWaived)
			criterion.Rationale = ""
			in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{criterion}
		}, wantErr: "waived status requires rationale"},
		{name: "not-applicable acceptance without rationale", mutate: func(in *Input) {
			criterion := acceptance(autonomous.AcceptanceStatusNotApplicable)
			criterion.Rationale = ""
			in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{criterion}
		}, wantErr: "not_applicable status requires rationale"},
		{name: "valid satisfied acceptance", mutate: func(in *Input) {
			in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{acceptance(autonomous.AcceptanceStatusSatisfied)}
		}},
		{name: "valid waived acceptance", mutate: func(in *Input) {
			in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{acceptance(autonomous.AcceptanceStatusWaived)}
		}},
		{name: "valid not-applicable acceptance", mutate: func(in *Input) {
			in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{acceptance(autonomous.AcceptanceStatusNotApplicable)}
		}},
		{name: "missing verification", mutate: func(in *Input) { in.Verification = nil }, wantErr: "verification is missing"},
		{name: "failed verification", mutate: func(in *Input) { in.Verification.Summary.Status = autonomous.VerificationStatusFailed }, wantErr: "want passed"},
		{name: "stale verification", mutate: func(in *Input) { in.Verification.SourceRevision = staleRevision }, wantErr: "verification gate"},
		{name: "missing audit", mutate: func(in *Input) { in.Audit = nil }, wantErr: "current audit is missing"},
		{name: "stale audit", mutate: func(in *Input) { in.Audit.SourceRevision = staleRevision }, wantErr: "audit gate"},
		{name: "wrong verification occurrence", mutate: func(in *Input) { in.Audit.VerificationOccurrenceID = "verification-old" }, wantErr: "consumed verification"},
		{name: "non-auditor provenance", mutate: func(in *Input) { in.Audit.AuditorProfile = autonomous.WorkerProfileImplementer }, wantErr: "used profile"},
		{name: "non-independent audit", mutate: func(in *Input) { in.Audit.RunID = in.Source.LatestMutation.RunID }, wantErr: "audit independence"},
		{name: "changes-required audit", mutate: func(in *Input) { in.Audit = changesAudit(blockingFinding("finding-one")) }, wantErr: "want clean"},
		{name: "open finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}}
		}, wantErr: "remains open"},
		{name: "resolved finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{resolvedFinding("finding-one", nil)}
		}},
		{name: "waived finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{terminalFinding("finding-one", autonomous.FindingResolutionStatusWaived)}
		}},
		{name: "superseded finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{terminalFinding("finding-one", autonomous.FindingResolutionStatusSuperseded)}
		}},
		{name: "invalid finding", mutate: func(in *Input) {
			in.State.FindingResolutions = []autonomous.FindingResolution{terminalFinding("finding-one", autonomous.FindingResolutionStatusInvalid)}
		}},
		{name: "unsafe source", mutate: func(in *Input) { in.Source.Safety = SourceSafetyUnsafe }, wantErr: "source safety gate"},
		{name: "unknown source", mutate: func(in *Input) { in.Source.Safety = SourceSafetyUnknown }, wantErr: "source safety gate"},
		{name: "task mismatch", mutate: func(in *Input) { in.Decision.TaskID = "task-2" }, wantErr: "decision task_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput(autonomous.ActionComplete)
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			before := mustJSON(t, in)
			route, err := Evaluate(in)
			assertError(t, err, tt.wantErr)
			if tt.wantErr == "" && route.Kind != RouteKindComplete {
				t.Fatalf("route kind = %q, want complete", route.Kind)
			}
			if got := mustJSON(t, in); string(got) != string(before) {
				t.Fatal("completion evaluation mutated task or execution state")
			}
		})
	}
}

func TestEvaluateBlock(t *testing.T) {
	for _, lifecycle := range []autonomous.LifecycleState{autonomous.LifecycleStatePending, autonomous.LifecycleStateReady} {
		for _, safety := range []SourceSafety{SourceSafetySafe, SourceSafetyUnsafe, SourceSafetyUnknown} {
			t.Run(string(lifecycle)+"/"+string(safety), func(t *testing.T) {
				in := validInput(autonomous.ActionBlock)
				setLifecycle(&in.State, lifecycle)
				in.Source.Safety = safety
				before := mustJSON(t, in)
				route, err := Evaluate(in)
				if err != nil || route.Kind != RouteKindBlock || route.WorkerProfile != "" {
					t.Fatalf("Evaluate() = %+v, %v", route, err)
				}
				if got := mustJSON(t, in); string(got) != string(before) {
					t.Fatal("block evaluation mutated input")
				}
			})
		}
	}

	illegalProfile := validInput(autonomous.ActionBlock)
	illegalProfile.Decision.WorkerProfile = autonomous.WorkerProfilePlanner
	illegalProfile.Reference.WorkerProfile = autonomous.WorkerProfilePlanner
	if _, err := Evaluate(illegalProfile); err == nil || !strings.Contains(err.Error(), "must not select worker_profile") {
		t.Fatalf("illegal block profile error = %v", err)
	}
	missingInput := validInput(autonomous.ActionBlock)
	missingInput.Decision.Inputs = nil
	if _, err := Evaluate(missingInput); err == nil || !strings.Contains(err.Error(), "inputs requires at least one evidence reference") {
		t.Fatalf("missing block input evidence error = %v", err)
	}

	for _, lifecycle := range []autonomous.LifecycleState{
		autonomous.LifecycleStatePlanning,
		autonomous.LifecycleStateWorking,
		autonomous.LifecycleStateVerifying,
		autonomous.LifecycleStateAuditing,
		autonomous.LifecycleStateCorrecting,
		autonomous.LifecycleStateNeedsInput,
		autonomous.LifecycleStateFinalizing,
		autonomous.LifecycleStateCompleted,
		autonomous.LifecycleStateBlocked,
		autonomous.LifecycleStateCancelled,
	} {
		t.Run("reject/"+string(lifecycle), func(t *testing.T) {
			in := validInput(autonomous.ActionBlock)
			setLifecycle(&in.State, lifecycle)
			if _, err := Evaluate(in); err == nil || !strings.Contains(err.Error(), "lifecycle gate") {
				t.Fatalf("Evaluate() error = %v", err)
			}
		})
	}
}

func TestValidateEvidenceChecksTransientIdentityWithoutAuthorizingAction(t *testing.T) {
	in := validInput(autonomous.ActionDocument)
	if err := ValidateEvidence(in.TaskID, in.Source, in.Verification, in.Audit); err != nil {
		t.Fatalf("ValidateEvidence() error = %v", err)
	}
	in.Verification.Summary.TaskID = "other-task"
	if err := ValidateEvidence(in.TaskID, in.Source, in.Verification, in.Audit); err == nil || !strings.Contains(err.Error(), "verification task_id") {
		t.Fatalf("ValidateEvidence() error = %v, want identity rejection", err)
	}
}

func validInput(action autonomous.Action) Input {
	profile := profileForAction(action)
	decision := autonomous.SupervisorDecision{
		TaskID:          taskID,
		Action:          action,
		WorkerProfile:   profile,
		Rationale:       "Current durable evidence supports this action.",
		SuccessCriteria: []string{"The authorized action records concrete evidence."},
		Inputs:          []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, ".agent/tasks/task-1.md")},
	}
	if action == autonomous.ActionComplete || action == autonomous.ActionBlock || action == autonomous.ActionNeedsInput {
		decision.SuccessCriteria = nil
	}
	if action == autonomous.ActionNeedsInput {
		question := autonomous.NeedsInputQuestion{TaskID: taskID, QuestionID: "product-mode", Revision: 1, Question: "Which behavior?", BlockingReason: "The task is ambiguous.", Options: []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep behavior."}, {ID: "change", Meaning: "Change behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "Safer."}, Evidence: append([]autonomous.EvidenceReference(nil), decision.Inputs...)}
		hash, _ := autonomous.QuestionContentSHA256(question)
		question.ContentSHA256 = hash
		decision.NeedsInput = &question
	}
	if action == autonomous.ActionCorrect {
		decision.FindingIDs = []string{"finding-one"}
	}
	reference := decisionReference("decision-current", "run-supervisor", action, profile)
	in := Input{
		TaskID:    taskID,
		Decision:  decision,
		Reference: reference,
		State: autonomous.ExecutionState{
			SchemaVersion: autonomous.ExecutionStateSchemaVersion,
			TaskID:        taskID,
			Lifecycle:     autonomous.LifecycleStateReady,
			Attempts:      zeroAttempts(),
		},
		Source: SourceEvidence{
			Revision: currentRevision,
			Safety:   SourceSafetySafe,
			LatestMutation: &SourceMutation{
				TaskID:            taskID,
				RunID:             "run-worker",
				DecisionID:        "decision-worker",
				Action:            autonomous.ActionImplement,
				ResultingRevision: currentRevision,
			},
		},
	}
	switch action {
	case autonomous.ActionImplement:
		in.State.Plan = actionablePlan()
	case autonomous.ActionAudit:
		in.Verification = passedVerification()
	case autonomous.ActionCorrect:
		in.Verification = passedVerification()
		in.Audit = changesAudit(blockingFinding("finding-one"))
	case autonomous.ActionDocument, autonomous.ActionSimplify:
		in.Verification = passedVerification()
		in.Audit = cleanAudit()
	case autonomous.ActionComplete:
		in.State.Plan = completedPlan()
		in.State.AcceptanceCriteria = []autonomous.AcceptanceCriterion{acceptance(autonomous.AcceptanceStatusSatisfied)}
		in.Verification = passedVerification()
		in.Audit = cleanAudit()
	}
	return in
}

func profileForAction(action autonomous.Action) autonomous.WorkerProfile {
	switch action {
	case autonomous.ActionPlan:
		return autonomous.WorkerProfilePlanner
	case autonomous.ActionImplement:
		return autonomous.WorkerProfileImplementer
	case autonomous.ActionAudit:
		return autonomous.WorkerProfileAuditor
	case autonomous.ActionCorrect:
		return autonomous.WorkerProfileCorrector
	case autonomous.ActionDocument:
		return autonomous.WorkerProfileDocumentor
	case autonomous.ActionSimplify:
		return autonomous.WorkerProfileSimplifier
	default:
		return ""
	}
}

func allActions() []autonomous.Action {
	return []autonomous.Action{
		autonomous.ActionPlan,
		autonomous.ActionImplement,
		autonomous.ActionAudit,
		autonomous.ActionCorrect,
		autonomous.ActionDocument,
		autonomous.ActionSimplify,
		autonomous.ActionComplete,
		autonomous.ActionBlock,
		autonomous.ActionNeedsInput,
	}
}

func setLifecycle(state *autonomous.ExecutionState, lifecycle autonomous.LifecycleState) {
	state.Lifecycle = lifecycle
	state.NeedsInput = nil
	state.Terminal = nil
	state.LatestDecision = nil
	state.Finalization = nil
	switch lifecycle {
	case autonomous.LifecycleStateNeedsInput:
		state.NeedsInput = &autonomous.NeedsInputDetail{Reason: "A product choice is required."}
	case autonomous.LifecycleStateFinalizing:
		state.Finalization = &autonomous.FinalizationDetail{
			SchemaVersion: autonomous.FinalizationDetailSchemaVersion, OperationID: "finalize-one", RunID: "finalization-run", Stage: autonomous.FinalizationStageAdmitted,
			FrozenEvidence:     autonomous.FinalizationArtifact{Path: ".revolvr/autonomous/tasks/task-1/completion/completion-evidence.json", SHA256: strings.Repeat("f", 64), ByteSize: 10},
			OriginalTaskSHA256: strings.Repeat("e", 64), AdmittedAt: time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC),
		}
	case autonomous.LifecycleStateCompleted:
		state.Plan = completedPlan()
		state.AcceptanceCriteria = []autonomous.AcceptanceCriterion{acceptance(autonomous.AcceptanceStatusSatisfied)}
		latest := decisionReference("decision-old", "run-old-supervisor", autonomous.ActionComplete, "")
		state.LatestDecision = &latest
		state.Terminal = &autonomous.TerminalDetail{Reason: "The task completed earlier."}
	case autonomous.LifecycleStateBlocked, autonomous.LifecycleStateCancelled, autonomous.LifecycleStateSuperseded, autonomous.LifecycleStateAbandoned:
		state.Terminal = &autonomous.TerminalDetail{Reason: "The task is terminal."}
	}
}

func actionablePlan() *autonomous.TaskPlan {
	return &autonomous.TaskPlan{
		TaskID:     taskID,
		ID:         "plan-one",
		Revision:   1,
		Provenance: []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, ".agent/tasks/task-1.md")},
		Steps: []autonomous.PlanStep{{
			ID:          "step-one",
			Description: "Implement the requested behavior.",
			Status:      autonomous.PlanStepStatusPending,
		}},
	}
}

func completedPlan() *autonomous.TaskPlan {
	plan := actionablePlan()
	plan.Completed = true
	plan.Steps[0].Status = autonomous.PlanStepStatusCompleted
	plan.Steps[0].Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindFile, "internal/example.go")}
	return plan
}

func acceptance(status autonomous.AcceptanceStatus) autonomous.AcceptanceCriterion {
	criterion := autonomous.AcceptanceCriterion{
		ID:          "criterion-one",
		Requirement: "The requested behavior is verified.",
		Status:      status,
	}
	switch status {
	case autonomous.AcceptanceStatusSatisfied:
		criterion.Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindVerification, "verification-current")}
	case autonomous.AcceptanceStatusWaived:
		criterion.Rationale = "The operator explicitly waived this criterion."
		criterion.Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, "waiver-record")}
	case autonomous.AcceptanceStatusNotApplicable:
		criterion.Rationale = "The criterion does not apply to this task."
		criterion.Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, "scope-record")}
	}
	return criterion
}

func passedVerification() *VerificationEvidence {
	return &VerificationEvidence{
		Summary: autonomous.VerificationSummary{
			TaskID:       taskID,
			Status:       autonomous.VerificationStatusPassed,
			Summary:      "The current verification suite passed.",
			RunID:        "run-verification",
			OccurrenceID: "verification-current",
			Evidence:     []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindVerification, "run-verification:verification-current")},
		},
		SourceRevision: currentRevision,
	}
}

func cleanAudit() *AuditEvidence {
	return &AuditEvidence{
		Report: autonomous.AuditReport{
			TaskID:      taskID,
			Disposition: autonomous.AuditDispositionClean,
			Rationale:   "Independent review found no remaining findings.",
			Inputs:      []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindVerification, "verification-current")},
		},
		RunID:                    "run-audit",
		AuditorProfile:           autonomous.WorkerProfileAuditor,
		SourceRevision:           currentRevision,
		VerificationRunID:        "run-verification",
		VerificationOccurrenceID: "verification-current",
	}
}

func changesAudit(findings ...autonomous.AuditFinding) *AuditEvidence {
	audit := cleanAudit()
	audit.Report.Disposition = autonomous.AuditDispositionChangesRequired
	audit.Report.Rationale = "Independent review found changes that require correction."
	audit.Report.Findings = append([]autonomous.AuditFinding(nil), findings...)
	return audit
}

func blockingFinding(id string) autonomous.AuditFinding {
	return autonomous.AuditFinding{
		ID:                 id,
		Significance:       autonomous.FindingSignificanceBlocking,
		Summary:            "A blocking defect remains.",
		Evidence:           []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindFile, "internal/example.go")},
		RequiredCorrection: "Correct the defect and verify the result.",
	}
}

func nonBlockingFinding(id string) autonomous.AuditFinding {
	finding := blockingFinding(id)
	finding.Significance = autonomous.FindingSignificanceNonBlocking
	finding.Summary = "A non-blocking defect remains."
	return finding
}

func resolvedFinding(id string, reference *autonomous.DecisionReference) autonomous.FindingResolution {
	return autonomous.FindingResolution{
		FindingID:  id,
		Status:     autonomous.FindingResolutionStatusResolved,
		Evidence:   []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindVerification, "resolution-verification")},
		Resolution: reference,
	}
}

func terminalFinding(id string, status autonomous.FindingResolutionStatus) autonomous.FindingResolution {
	resolution := autonomous.FindingResolution{FindingID: id, Status: status}
	switch status {
	case autonomous.FindingResolutionStatusWaived:
		resolution.Rationale = "The documented risk was explicitly waived."
		resolution.Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, "waiver-record")}
	case autonomous.FindingResolutionStatusSuperseded:
		resolution.SupersedingFindingID = "finding-new"
		resolution.Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindAudit, "audit-new")}
	case autonomous.FindingResolutionStatusInvalid:
		resolution.Rationale = "Independent evidence proves the finding invalid."
		resolution.Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindAudit, "audit-invalid")}
	}
	return resolution
}

func decisionReference(id, runID string, action autonomous.Action, profile autonomous.WorkerProfile) autonomous.DecisionReference {
	return autonomous.DecisionReference{
		DecisionID:    id,
		RunID:         runID,
		TaskID:        taskID,
		Action:        action,
		WorkerProfile: profile,
		Artifact:      evidence(autonomous.EvidenceKindFile, ".revolvr/runs/"+runID+"/supervisor-decision.json"),
		CreatedAt:     fixedTime,
	}
}

func zeroAttempts() autonomous.AttemptState {
	return autonomous.AttemptState{
		RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
		TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
	}
}

func evidence(kind autonomous.EvidenceKind, reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: "Concrete durable evidence for policy evaluation."}
}

func assertRoute(t *testing.T, route Route, in Input) {
	t.Helper()
	wantKind := RouteKindWorker
	if in.Decision.Action == autonomous.ActionComplete {
		wantKind = RouteKindComplete
	} else if in.Decision.Action == autonomous.ActionBlock {
		wantKind = RouteKindBlock
	}
	want := Route{
		Kind:           wantKind,
		TaskID:         in.TaskID,
		DecisionID:     in.Reference.DecisionID,
		Action:         in.Decision.Action,
		WorkerProfile:  in.Decision.WorkerProfile,
		SourceRevision: in.Source.Revision,
	}
	if !reflect.DeepEqual(route, want) {
		t.Fatalf("route = %+v, want %+v", route, want)
	}
}

func assertError(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("Evaluate() error = %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Evaluate() error = %v, want substring %q", err, want)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
