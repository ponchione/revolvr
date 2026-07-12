package autonomouspolicy

import (
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
)

func TestNeedsInputRouteAndSuspensionRefusal(t *testing.T) {
	in := validInput(autonomous.ActionBlock)
	decision, reference, record := policyQuestion(t)
	in.Decision = decision
	in.Reference = reference
	route, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if route.Kind != RouteKindNeedsInput || route.WorkerProfile != "" {
		t.Fatalf("route=%+v", route)
	}
	suspended := in
	suspended.State = typedSuspendedState(record)
	suspended.Decision = validInput(autonomous.ActionImplement).Decision
	suspended.Reference = validInput(autonomous.ActionImplement).Reference
	if _, err := Evaluate(suspended); err == nil || !strings.Contains(err.Error(), "exact durable answer") {
		t.Fatalf("routing while unanswered error=%v", err)
	}
}

func TestNeedsInputYieldSafetyAndIndependentWork(t *testing.T) {
	_, _, record := policyQuestion(t)
	state := typedSuspendedState(record)
	base := NeedsInputYieldInput{TaskID: "task-1", State: state, Source: SourceEvidence{Revision: record.SourceRevision, Safety: SourceSafetySafe}}
	if got := EvaluateNeedsInputYield(base); got.Disposition != NeedsInputYieldClean || got.Question == nil {
		t.Fatalf("clean yield=%+v", got)
	}
	if got := EvaluateIndependentWork(base, "inspect-contract"); !got.Allowed || got.Work == nil || got.Work.SourceEffect != autonomous.InputSourceEffectReadOnly {
		t.Fatalf("independent work=%+v", got)
	}
	tests := []struct {
		name   string
		mutate func(*NeedsInputYieldInput)
		reason NeedsInputYieldReason
	}{
		{"wrong task", func(v *NeedsInputYieldInput) { v.TaskID = "task-2" }, NeedsInputYieldReasonTaskMismatch},
		{"dirty", func(v *NeedsInputYieldInput) { v.Source.Safety = SourceSafetyUnsafe }, NeedsInputYieldReasonUnsafeSource},
		{"unknown", func(v *NeedsInputYieldInput) { v.Source.Safety = SourceSafetyUnknown }, NeedsInputYieldReasonUnknownSource},
		{"in flight", func(v *NeedsInputYieldInput) { v.SourceOperationInFlight = true }, NeedsInputYieldReasonOperationInFlight},
		{"stale source", func(v *NeedsInputYieldInput) { v.Source.Revision = strings.Repeat("b", 64) }, NeedsInputYieldReasonStaleSource},
		{"legacy", func(v *NeedsInputYieldInput) {
			v.State.Input = autonomous.InputState{}
			v.State.NeedsInput = &autonomous.NeedsInputDetail{Reason: "legacy"}
		}, NeedsInputYieldReasonNotSuspended},
		{"malformed question", func(v *NeedsInputYieldInput) {
			v.State.Input.Questions[0].Question.Question = "drifted question"
		}, NeedsInputYieldReasonMalformedState},
		{"missing provenance", func(v *NeedsInputYieldInput) {
			v.State.Input.Questions[0].Decision.Artifact.Reference = ""
		}, NeedsInputYieldReasonMissingProvenance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := base
			tt.mutate(&value)
			got := EvaluateNeedsInputYield(value)
			if got.Disposition != NeedsInputYieldUnsafe || got.Reason != tt.reason {
				t.Fatalf("got=%+v want reason=%s", got, tt.reason)
			}
		})
	}
}

func policyQuestion(t *testing.T) (autonomous.SupervisorDecision, autonomous.DecisionReference, autonomous.InputQuestionRecord) {
	t.Helper()
	q := autonomous.NeedsInputQuestion{TaskID: "task-1", QuestionID: "product-mode", Revision: 1, Question: "Which behavior?", BlockingReason: "Two incompatible behaviors remain.", Options: []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep behavior."}, {ID: "change", Meaning: "Change behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "Safer."}, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task", Detail: "Ambiguous task."}}, IndependentWork: []autonomous.IndependentWorkDeclaration{{ID: "inspect-contract", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Description: "Inspect current contract.", SourceEffect: autonomous.InputSourceEffectReadOnly, IndependentOfOptionIDs: []string{"keep", "change"}, Inputs: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindRepository, Reference: "README.md", Detail: "Current contract."}}}}}
	hash, err := autonomous.QuestionContentSHA256(q)
	if err != nil {
		t.Fatal(err)
	}
	q.ContentSHA256 = hash
	decision := autonomous.SupervisorDecision{TaskID: "task-1", Action: autonomous.ActionNeedsInput, Rationale: "Input required.", Inputs: q.Evidence, NeedsInput: &q}
	reference := decisionReference("decision-input", "run-input", autonomous.ActionNeedsInput, "")
	record := autonomous.InputQuestionRecord{Sequence: 1, TaskID: "task-1", Question: q, Decision: reference, SourceRevision: strings.Repeat("a", 64), ResumeLifecycle: autonomous.LifecycleStateReady, RecordedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	return decision, reference, record
}
func typedSuspendedState(record autonomous.InputQuestionRecord) autonomous.ExecutionState {
	state := validInput(autonomous.ActionBlock).State
	state.Lifecycle = autonomous.LifecycleStateNeedsInput
	state.Input = autonomous.InputState{TransitionSequence: 1, Questions: []autonomous.InputQuestionRecord{record}}
	identity := record.Question.Identity()
	state.NeedsInput = &autonomous.NeedsInputDetail{CurrentQuestion: &identity}
	state.LatestDecision = &record.Decision
	return state
}
