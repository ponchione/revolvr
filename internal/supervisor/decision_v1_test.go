package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/model"
	"revolvr/internal/policy"
	"revolvr/internal/tasklifecycle"
)

func TestDecisionV1AcceptsEveryAdmittedAction(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "unused-secret-sentinel")
	for _, action := range []policy.Action{
		policy.ActionPlan, policy.ActionImplement, policy.ActionCorrect, policy.ActionDocument,
		policy.ActionSimplify, policy.ActionComplete, policy.ActionBlock, policy.ActionNeedsInput,
	} {
		t.Run(string(action), func(t *testing.T) {
			state := v1StateForAction(action)
			cfg, reader, fake, recorder := v1Config(t, state)
			prepared, err := PrepareSupervisorDecision(cfg, state)
			if err != nil {
				t.Fatal(err)
			}
			decision := v1DecisionForAction(action, prepared, state)
			fake.result = v1ModelResult(t, prepared, decision)
			before := v1CloneState(t, state)

			result, err := Decide(context.Background(), cfg)
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if result.Disposition != DecisionAccepted || result.Decision == nil || result.Route == nil || result.Route.Action != action {
				t.Fatalf("unexpected accepted result: %#v", result)
			}
			if action == policy.ActionComplete && result.Route.Kind != policy.RouteCompletionPreflight {
				t.Fatalf("complete route = %q, want completion preflight proposal", result.Route.Kind)
			}
			if action == policy.ActionBlock && result.Route.Kind != policy.RouteBlockAdvisory {
				t.Fatalf("block route = %q", result.Route.Kind)
			}
			if action == policy.ActionNeedsInput && result.Route.Kind != policy.RouteNeedsInputAdvisory {
				t.Fatalf("needs_input route = %q", result.Route.Kind)
			}
			if fake.calls != 1 || reader.reads != 2 || len(recorder.records) != 1 {
				t.Fatalf("calls: model=%d state_reads=%d records=%d", fake.calls, reader.reads, len(recorder.records))
			}
			if recorder.records[0].Disposition != DecisionAccepted || recorder.records[0].Route == nil {
				t.Fatalf("accepted provenance was not persisted: %#v", recorder.records[0])
			}
			if !reflect.DeepEqual(state, before) {
				t.Fatal("supervisor or host policy mutated caller lifecycle state")
			}
			if got := os.Getenv("OPENAI_API_KEY"); got != "unused-secret-sentinel" {
				t.Fatalf("API key environment changed: %q", got)
			}
		})
	}
}

func TestDecisionV1ExactDossierSchemaPromptPolicyAndDecisionIdentity(t *testing.T) {
	state := v1StateForAction(policy.ActionPlan)
	cfg, _, fake, recorder := v1Config(t, state)
	prepared, err := PrepareSupervisorDecision(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Dossier.Version != SupervisorDossierVersion || prepared.Dossier.SHA256 != model.SHA256(prepared.Dossier.Content) || prepared.Dossier.ByteSize != len(prepared.Dossier.Content) {
		t.Fatalf("dossier identity mismatch: %#v", prepared.Dossier)
	}
	var dossier Dossier
	if err := json.Unmarshal(prepared.Dossier.Content, &dossier); err != nil {
		t.Fatal(err)
	}
	wantOmissions := []Omission{
		{Section: "broad_raw_source", Reason: "excluded by supervisor context policy"},
		{Section: "unrelated_code", Reason: "excluded by supervisor context policy"},
		{Section: "conversation_history", Reason: "fresh decision uses canonical durable state only"},
		{Section: "plan", Reason: "no canonical plan exists"},
		{Section: "latest_verification", Reason: "no canonical verification exists"},
		{Section: "latest_audit", Reason: "no canonical audit exists"},
		{Section: "open_findings", Reason: "no canonical findings exist"},
		{Section: "attempt_history", Reason: "no canonical attempts exist"},
		{Section: "strategy_history", Reason: "no canonical strategies exist"},
	}
	if !reflect.DeepEqual(dossier.Omissions, wantOmissions) {
		t.Fatalf("omissions = %#v, want %#v", dossier.Omissions, wantOmissions)
	}
	if prepared.Prompt.Version != SupervisorPromptVersion || prepared.Prompt.SHA256 != model.SHA256([]byte(prepared.Prompt.Content)) {
		t.Fatal("prompt identity is not exact")
	}
	for _, text := range []string{"fresh, stateless", "Use no tools", "no prior conversation", prepared.Dossier.SHA256, cfg.ModelPolicy.SHA256, cfg.HostPolicy.SHA256} {
		if !strings.Contains(prepared.Prompt.Content, text) {
			t.Fatalf("prompt missing %q", text)
		}
	}
	var schema map[string]any
	if err := json.Unmarshal(prepared.ResponseSchema.Content, &schema); err != nil {
		t.Fatal(err)
	}
	actions := schema["properties"].(map[string]any)["action"].(map[string]any)["enum"].([]any)
	if got := strings.Join(v1AnyStrings(actions), ","); got != "plan,implement,correct,document,simplify,complete,block,needs_input" {
		t.Fatalf("schema actions = %s", got)
	}
	if schema["additionalProperties"] != false || prepared.ResponseSchema.SHA256 != prepared.ExpectedOutput.ResponseSchemaSHA256 {
		t.Fatal("response schema is not exact and closed")
	}

	decision := v1DecisionForAction(policy.ActionPlan, prepared, state)
	fake.result = v1ModelResult(t, prepared, decision)
	result, err := Decide(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	record := recorder.records[0]
	if record.Dossier.SHA256 != prepared.Dossier.SHA256 || record.Prompt.SHA256 != prepared.Prompt.SHA256 || record.ResponseSchema.SHA256 != prepared.ResponseSchema.SHA256 || record.ModelPolicy.SHA256 != cfg.ModelPolicy.SHA256 || record.HostPolicy != cfg.HostPolicy || !reflect.DeepEqual(record.ExpectedRequest, prepared.ExpectedRequest) {
		t.Fatalf("persisted provenance is incomplete: %#v", record)
	}
	if result.Decision.Identity.ContentSHA256 == nil || *result.Decision.Identity.ContentSHA256 != model.SHA256(v1DecisionMaterial(t, *result.Decision)) {
		t.Fatal("host-assigned decision content identity is not exact")
	}
}

func TestDecisionV1PersistsRejectedOutputWithFullProvenance(t *testing.T) {
	state := v1StateForAction(policy.ActionPlan)
	cfg, _, fake, recorder := v1Config(t, state)
	prepared, err := PrepareSupervisorDecision(cfg, state)
	if err != nil {
		t.Fatal(err)
	}
	fake.result = v1SuccessfulRawResult(prepared, json.RawMessage(`{"malformed":`))
	result, err := Decide(context.Background(), cfg)
	if !errors.Is(err, ErrDecisionRejected) || result.Disposition != DecisionRejected {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("records=%d", len(recorder.records))
	}
	record := recorder.records[0]
	if record.ReasonCode != "malformed_decision" || record.Dossier.SHA256 == "" || record.Prompt.SHA256 == "" || record.ResponseSchema.SHA256 == "" || record.ExpectedRequest.RequestID == "" || record.ModelResult.Outcome != model.OutcomeSuccess || string(record.RawOutput) != `{"malformed":` {
		t.Fatalf("rejected provenance incomplete: %#v", record)
	}
	if _, err := json.Marshal(record); err != nil {
		t.Fatalf("rejected malformed-output record must remain persistable: %v", err)
	}
	if record.Route != nil {
		t.Fatal("rejected output acquired a host route")
	}
}

func TestDecisionV1RejectsMultipleUnknownMalformedAndRefusal(t *testing.T) {
	tests := []struct {
		name    string
		raw     func(Decision) json.RawMessage
		refusal bool
	}{
		{name: "multiple actions", raw: func(decision Decision) json.RawMessage {
			raw := v1MarshalDecision(t, decision)
			return json.RawMessage(strings.Replace(string(raw), `"action":"plan"`, `"action":"plan","action":"implement"`, 1))
		}},
		{name: "unknown action", raw: func(decision Decision) json.RawMessage {
			decision.Action = policy.Action("audit")
			decision.Identity.ContentSHA256 = nil
			return v1MarshalDecision(t, decision)
		}},
		{name: "unknown field", raw: func(decision Decision) json.RawMessage {
			var object map[string]any
			if err := json.Unmarshal(v1MarshalDecision(t, decision), &object); err != nil {
				t.Fatal(err)
			}
			object["worker_command"] = "forbidden"
			raw, _ := json.Marshal(object)
			return raw
		}},
		{name: "malformed", raw: func(Decision) json.RawMessage { return json.RawMessage(`{"action"`) }},
		{name: "refusal", refusal: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := v1StateForAction(policy.ActionPlan)
			cfg, _, fake, recorder := v1Config(t, state)
			prepared, err := PrepareSupervisorDecision(cfg, state)
			if err != nil {
				t.Fatal(err)
			}
			decision := v1DecisionForAction(policy.ActionPlan, prepared, state)
			if test.refusal {
				fake.result = model.Result{Outcome: model.OutcomeSafetyRefusal, Request: prepared.ExpectedRequest, Refusal: "cannot comply"}
				fake.err = errors.New("OpenAI safety refusal")
			} else {
				fake.result = v1SuccessfulRawResult(prepared, test.raw(decision))
			}
			result, err := Decide(context.Background(), cfg)
			if !errors.Is(err, ErrDecisionRejected) || result.Disposition != DecisionRejected || fake.calls != 1 || len(recorder.records) != 1 || recorder.records[0].Route != nil {
				t.Fatalf("result=%#v err=%v calls=%d records=%d", result, err, fake.calls, len(recorder.records))
			}
		})
	}
}

func TestDecisionV1RejectsStaleTaskSourceDossierAndCanonicalState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Decision)
	}{
		{"task", func(decision *Decision) { decision.RevolvrIdentity.TaskID = "another-task" }},
		{"source", func(decision *Decision) { decision.RevolvrIdentity.SourceRevision = strings.Repeat("f", 64) }},
		{"dossier", func(decision *Decision) { decision.Identity.DossierSHA256 = strings.Repeat("e", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := v1StateForAction(policy.ActionPlan)
			cfg, _, fake, recorder := v1Config(t, state)
			prepared, err := PrepareSupervisorDecision(cfg, state)
			if err != nil {
				t.Fatal(err)
			}
			decision := v1DecisionForAction(policy.ActionPlan, prepared, state)
			test.mutate(&decision)
			decision.Identity.ContentSHA256 = nil
			fake.result = v1ModelResult(t, prepared, decision)
			_, err = Decide(context.Background(), cfg)
			if !errors.Is(err, ErrDecisionRejected) || recorder.records[0].ReasonCode != "stale_decision_identity" {
				t.Fatalf("err=%v record=%#v", err, recorder.records[0])
			}
		})
	}

	t.Run("state changes during call", func(t *testing.T) {
		state := v1StateForAction(policy.ActionPlan)
		cfg, reader, fake, recorder := v1Config(t, state)
		prepared, err := PrepareSupervisorDecision(cfg, state)
		if err != nil {
			t.Fatal(err)
		}
		changed := v1CloneState(t, state)
		changed.Attempts = []AttemptContext{{ID: "attempt-new", Action: policy.ActionPlan, Outcome: "started"}}
		reader.states = []CanonicalState{state, changed}
		fake.result = v1ModelResult(t, prepared, v1DecisionForAction(policy.ActionPlan, prepared, state))
		_, err = Decide(context.Background(), cfg)
		if !errors.Is(err, ErrDecisionRejected) || recorder.records[0].ReasonCode != "stale_canonical_state" || recorder.records[0].ObservedDossier == nil || recorder.records[0].ObservedDossier.SHA256 == prepared.Dossier.SHA256 {
			t.Fatalf("err=%v record=%#v", err, recorder.records[0])
		}
	})
}

func TestDecisionV1RejectsIllegalLifecycleExhaustedBudgetAndScopeBroadening(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CanonicalState, *Decision)
	}{
		{"illegal lifecycle", func(state *CanonicalState, _ *Decision) { state.Lifecycle = tasklifecycle.TaskAuditing }},
		{"exhausted worker budget", func(state *CanonicalState, _ *Decision) { state.Budget.ModelCallsRemaining = 1 }},
		{"scope broadening", func(_ *CanonicalState, decision *Decision) {
			decision.Scope = []string{"outside/task.go"}
			decision.Strategy.Targets = []string{"outside/task.go"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := v1StateForAction(policy.ActionImplement)
			decisionState := v1CloneState(t, state)
			cfg, _, fake, recorder := v1Config(t, state)
			prepared, err := PrepareSupervisorDecision(cfg, state)
			if err != nil {
				t.Fatal(err)
			}
			decision := v1DecisionForAction(policy.ActionImplement, prepared, decisionState)
			test.mutate(&state, &decision)
			cfg.StateReader = &v1StateReader{states: []CanonicalState{state, state}}
			if test.name != "scope broadening" {
				prepared, err = PrepareSupervisorDecision(cfg, state)
				if err != nil {
					t.Fatal(err)
				}
				decision = v1DecisionForAction(policy.ActionImplement, prepared, state)
			}
			decision.Identity.ContentSHA256 = nil
			fake.result = v1ModelResult(t, prepared, decision)
			_, err = Decide(context.Background(), cfg)
			if !errors.Is(err, ErrDecisionRejected) || recorder.records[0].ReasonCode != "host_policy_denied" {
				t.Fatalf("err=%v record=%#v", err, recorder.records[0])
			}
		})
	}
}

func TestDecisionV1CompleteCannotBypassVerificationAuditOrEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CanonicalState)
	}{
		{"missing verification", func(state *CanonicalState) { state.LatestVerification = nil }},
		{"missing audit", func(state *CanonicalState) { state.LatestAudit = nil }},
		{"open finding", func(state *CanonicalState) {
			state.Findings = []FindingContext{{ID: "finding-open", Open: true, Summary: "still open"}}
		}},
		{"pending criterion", func(state *CanonicalState) { state.Criteria[0].Status = "pending" }},
		{"unreconciled workspace", func(state *CanonicalState) { state.WorkspaceReconciled = false }},
		{"incomplete artifact manifest", func(state *CanonicalState) { state.ArtifactManifest.Complete = false }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := v1StateForAction(policy.ActionComplete)
			test.mutate(&state)
			cfg, _, fake, recorder := v1Config(t, state)
			prepared, err := PrepareSupervisorDecision(cfg, state)
			if err != nil {
				t.Fatal(err)
			}
			decision := v1DecisionForAction(policy.ActionComplete, prepared, v1StateForAction(policy.ActionComplete))
			decision.Identity = prepared.ExpectedDecision
			decision.Identity.ContentSHA256 = nil
			fake.result = v1ModelResult(t, prepared, decision)
			_, err = Decide(context.Background(), cfg)
			if !errors.Is(err, ErrDecisionRejected) || recorder.records[0].ReasonCode != "host_policy_denied" {
				t.Fatalf("err=%v record=%#v", err, recorder.records[0])
			}
		})
	}
}

type v1StateReader struct {
	states []CanonicalState
	reads  int
}

func (reader *v1StateReader) ReadSupervisorState(_ context.Context, taskID, runID string) (CanonicalState, error) {
	reader.reads++
	if len(reader.states) == 0 {
		return CanonicalState{}, errors.New("no state")
	}
	index := reader.reads - 1
	if index >= len(reader.states) {
		index = len(reader.states) - 1
	}
	state := v1CloneStateValue(reader.states[index])
	if state.Task.TaskID != taskID || state.Run.RunID != runID {
		return CanonicalState{}, errors.New("selector mismatch")
	}
	return state, nil
}

type v1FakeModel struct {
	result model.Result
	err    error
	calls  int
}

func (fake *v1FakeModel) Invoke(_ context.Context, _ model.PreparedRequest) (model.Result, error) {
	fake.calls++
	return fake.result, fake.err
}

type v1Recorder struct{ records []DecisionRecord }

func (recorder *v1Recorder) PersistSupervisorDecision(_ context.Context, record DecisionRecord) error {
	recorder.records = append(recorder.records, record)
	return nil
}

func v1Config(t *testing.T, state CanonicalState) (DecisionConfig, *v1StateReader, *v1FakeModel, *v1Recorder) {
	t.Helper()
	modelPolicy, err := PinSupervisorModelPolicy(ModelPolicySettings{
		Model: "gpt-5.6-sol", ReasoningEffort: "high", MaxOutputTokens: 2048, Timeout: time.Minute,
		MaxStreamBytes: 1 << 20, MaxDiagnosticBytes: 1 << 14, Retry: model.RetryPolicy{MaxAttempts: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := &v1StateReader{states: []CanonicalState{state, state}}
	fake := &v1FakeModel{}
	recorder := &v1Recorder{}
	return DecisionConfig{
		TaskID: state.Task.TaskID, TaskVersionID: state.Task.TaskVersionID, TaskVersion: state.Task.Version,
		RunID: state.Run.RunID, SourceRevision: state.Source.Revision, SourceCommit: state.Source.Commit, SourceTree: state.Source.Tree,
		DecisionID: "decision-1", ModelPolicy: modelPolicy, HostPolicy: policy.CurrentIdentity(), StateReader: reader, Model: fake, Recorder: recorder,
	}, reader, fake, recorder
}

func v1BaseState() CanonicalState {
	revision := strings.Repeat("a", 64)
	return CanonicalState{
		Task: TaskContext{TaskID: "task-1", TaskVersionID: "version-1", Version: 1, ContractSummary: "Implement only the accepted bounded task.", AllowedPaths: []string{"internal", "docs"}, ExcludedPaths: []string{"internal/generated"}},
		Run:  RunContext{RunID: "run-1"}, Source: SourceContext{Revision: revision, Commit: strings.Repeat("b", 40), Tree: strings.Repeat("c", 40), Safe: true},
		Lifecycle:              tasklifecycle.TaskReady,
		Plan:                   &PlanContext{ID: "plan-1", Steps: []policy.PlanStepGate{{ID: "step-1", Status: "pending"}}},
		Criteria:               []CriterionContext{{ID: "criterion-1", Requirement: "All focused tests pass.", Status: "pending"}},
		Budget:                 policy.Budget{IdentityID: "budget-1", ModelCallsRemaining: 3, WorkerAttemptsRemaining: 2, TokensRemaining: 1000},
		HighAuthorityDecisions: []AuthorityDecision{{ID: "adr-1", Authority: "operator", Decision: "Stay within accepted scope.", SHA256: strings.Repeat("d", 64)}},
		WorkspaceReconciled:    true, ArtifactManifest: ArtifactManifestContext{ID: "manifest-1", Complete: true},
	}
}

func v1StateForAction(action policy.Action) CanonicalState {
	state := v1BaseState()
	switch action {
	case policy.ActionPlan:
		state.Lifecycle = tasklifecycle.TaskAdmitted
		state.Plan = nil
		state.Criteria = nil
	case policy.ActionImplement, policy.ActionBlock, policy.ActionNeedsInput:
		state.Lifecycle = tasklifecycle.TaskReady
	case policy.ActionCorrect:
		state.Lifecycle = tasklifecycle.TaskAuditing
		state.LatestVerification = v1Verification(state.Source.Revision, "passed")
		state.LatestAudit = v1Audit(state.Source.Revision, "changes_required")
		state.Findings = []FindingContext{{ID: "finding-1", Open: true, Summary: "Fix the exact defect."}}
	case policy.ActionDocument, policy.ActionSimplify:
		state.Lifecycle = tasklifecycle.TaskAuditing
		state.LatestVerification = v1Verification(state.Source.Revision, "passed")
		state.LatestAudit = v1Audit(state.Source.Revision, "clean")
	case policy.ActionComplete:
		state.Lifecycle = tasklifecycle.TaskAuditing
		state.Plan.Completed = true
		state.Plan.Steps[0].Status = "completed"
		state.Criteria[0].Status = "passed"
		state.LatestVerification = v1Verification(state.Source.Revision, "passed")
		state.LatestAudit = v1Audit(state.Source.Revision, "clean")
	}
	return state
}

func v1Verification(revision, status string) *VerificationContext {
	return &VerificationContext{ID: "verification-1", Status: status, SourceRevision: revision, Final: true, EvidenceComplete: true, Summary: "Exact final verification evidence."}
}

func v1Audit(revision, status string) *AuditContext {
	return &AuditContext{ID: "audit-1", Status: status, SourceRevision: revision, VerificationID: "verification-1", Independent: true, EvidenceComplete: true, Summary: "Independent audit evidence."}
}

func v1DecisionForAction(action policy.Action, prepared PreparedDecision, state CanonicalState) Decision {
	decision := Decision{
		RevolvrIdentity: prepared.ExpectedOutput, SchemaVersion: SupervisorDecisionVersion, Identity: prepared.ExpectedDecision,
		Action: action, Rationale: "The frozen canonical dossier admits this exact action.", EvidenceRefs: []string{"task:" + state.Task.TaskVersionID}, Scope: []string{},
	}
	switch action {
	case policy.ActionPlan:
		decision.Strategy = &DecisionStrategy{Approach: "Produce the bounded canonical plan.", Techniques: []string{}, Targets: []string{}}
	case policy.ActionImplement, policy.ActionDocument, policy.ActionSimplify:
		decision.Scope = []string{"internal/example.go"}
		decision.Strategy = &DecisionStrategy{Approach: "Change only the exact admitted target.", Techniques: []string{"focused change"}, Targets: append([]string(nil), decision.Scope...)}
	case policy.ActionCorrect:
		decision.Scope = []string{"internal/example.go"}
		decision.Strategy = &DecisionStrategy{Approach: "Correct only the cited finding.", Techniques: []string{"root cause"}, Targets: append([]string(nil), decision.Scope...)}
		decision.Correction = &policy.CorrectionAuthority{Kind: "audit_findings", FindingIDs: []string{"finding-1"}}
	case policy.ActionComplete:
		decision.Completion = &policy.CompletionEvidence{PlanID: "plan-1", CriterionIDs: []string{"criterion-1"}, VerificationID: "verification-1", AuditID: "audit-1", ArtifactManifestID: "manifest-1"}
	case policy.ActionBlock:
		decision.Block = &BlockAdvice{Reason: "A durable blocker remains.", EvidenceRefs: append([]string(nil), decision.EvidenceRefs...), OtherQueueWorkMayRun: true}
	case policy.ActionNeedsInput:
		decision.NeedsInput = &NeedsInputAdvice{QuestionID: "question-1", Question: "Which accepted option should the operator choose?", BlockingReason: "Canonical evidence cannot choose.", Options: []QuestionOption{{ID: "option-a", Meaning: "Use A."}, {ID: "option-b", Meaning: "Use B."}}, EvidenceRefs: append([]string(nil), decision.EvidenceRefs...)}
	}
	return decision
}

func v1ModelResult(t *testing.T, prepared PreparedDecision, decision Decision) model.Result {
	t.Helper()
	return v1SuccessfulRawResult(prepared, v1MarshalDecision(t, decision))
}

func v1SuccessfulRawResult(prepared PreparedDecision, raw json.RawMessage) model.Result {
	return model.Result{Outcome: model.OutcomeSuccess, Request: prepared.ExpectedRequest, StructuredOutput: append(json.RawMessage(nil), raw...), Usage: model.UsageEvidence{Available: true, InputTokens: 5, OutputTokens: 5, TotalTokens: 10}, Service: model.ServiceEvidence{Model: prepared.ExpectedRequest.Model}}
}

func v1MarshalDecision(t *testing.T, decision Decision) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func v1DecisionMaterial(t *testing.T, decision Decision) []byte {
	t.Helper()
	decision.Identity.ContentSHA256 = nil
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func v1CloneState(t *testing.T, state CanonicalState) CanonicalState {
	t.Helper()
	return v1CloneStateValue(state)
}

func v1CloneStateValue(state CanonicalState) CanonicalState {
	raw, _ := json.Marshal(state)
	var cloned CanonicalState
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func v1AnyStrings(values []any) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value.(string)
	}
	return out
}
