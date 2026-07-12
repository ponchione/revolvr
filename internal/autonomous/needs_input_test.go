package autonomous

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNeedsInputDecisionValidationAndRoundTrip(t *testing.T) {
	decision := needsInputDecisionFixture(t)
	if err := decision.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SupervisorDecision
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decision, decoded) {
		t.Fatalf("round trip differs\nwant=%+v\n got=%+v", decision, decoded)
	}
}

func TestDossierRendersTypedAndLegacyNeedsInputLifecycle(t *testing.T) {
	decision := needsInputDecisionFixture(t)
	reference := DecisionReference{DecisionID: "decision-input", RunID: "run-input", TaskID: "task-1", Action: ActionNeedsInput, Artifact: EvidenceReference{Kind: EvidenceKindFile, Reference: "decision.json", Detail: "Validated decision."}, CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	record := InputQuestionRecord{Sequence: 1, TaskID: "task-1", Question: *decision.NeedsInput, Decision: reference, SourceRevision: strings.Repeat("a", 64), ResumeLifecycle: LifecycleStateReady, RecordedAt: reference.CreatedAt}
	identity := record.Question.Identity()
	state := validExecutionState(LifecycleStateReady)
	state.Lifecycle = LifecycleStateNeedsInput
	state.Input = InputState{TransitionSequence: 1, Questions: []InputQuestionRecord{record}}
	state.NeedsInput = &NeedsInputDetail{CurrentQuestion: &identity}
	state.LatestDecision = &reference
	assertProjection := func(label string, state ExecutionState, wants ...string) {
		t.Helper()
		var out bytes.Buffer
		writeExecutionState(&out, state)
		text := out.String()
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q:\n%s", label, want, text)
			}
		}
	}
	assertProjection("unanswered", state, "Exact question: Should the feature", "status=unanswered", "recommendation not selected automatically", "audit-current-contract")
	answer := InputAnswerRecord{Sequence: 2, AnswerID: "answer-one", TaskID: "task-1", Question: identity, OptionID: "disable", Provenance: AnswerProvenance{Kind: AnswerProvenanceOperator, Actor: "operator"}, AnsweredAt: reference.CreatedAt.Add(time.Minute)}
	state.Input.TransitionSequence = 2
	state.Input.Answers = []InputAnswerRecord{answer}
	assertProjection("answered", state, "status=answered", "option=disable", "operator")
	resume := InputResumeRecord{Sequence: 3, ResumeID: "resume-one", TaskID: "task-1", Question: identity, AnswerID: "answer-one", ResumedAt: reference.CreatedAt.Add(2 * time.Minute)}
	state.Input.TransitionSequence = 3
	state.Input.Resumes = []InputResumeRecord{resume}
	state.Lifecycle = LifecycleStateReady
	state.NeedsInput = nil
	assertProjection("resumed", state, "status=resumed", "Resume: resume-one")
	legacy := validExecutionState(LifecycleStateNeedsInput)
	var out bytes.Buffer
	writeExecutionState(&out, legacy)
	if !strings.Contains(out.String(), "legacy, non-answerable") {
		t.Fatalf("legacy projection:\n%s", out.String())
	}
}

func TestInputTransitionRejectsAnswerDisappearanceAndRewrite(t *testing.T) {
	decision := needsInputDecisionFixture(t)
	reference := DecisionReference{DecisionID: "decision-input", RunID: "run-input", TaskID: "task-1", Action: ActionNeedsInput, Artifact: EvidenceReference{Kind: EvidenceKindFile, Reference: "decision.json", Detail: "Validated decision."}, CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	question := InputQuestionRecord{Sequence: 1, TaskID: "task-1", Question: *decision.NeedsInput, Decision: reference, SourceRevision: strings.Repeat("a", 64), ResumeLifecycle: LifecycleStateReady, RecordedAt: reference.CreatedAt}
	identity := question.Question.Identity()
	answer := InputAnswerRecord{Sequence: 2, AnswerID: "answer-one", TaskID: "task-1", Question: identity, OptionID: "disable", Provenance: AnswerProvenance{Kind: AnswerProvenanceOperator, Actor: "operator"}, AnsweredAt: reference.CreatedAt.Add(time.Minute)}
	previous := validExecutionState(LifecycleStateReady)
	previous.Lifecycle = LifecycleStateNeedsInput
	previous.Input = InputState{TransitionSequence: 2, Questions: []InputQuestionRecord{question}, Answers: []InputAnswerRecord{answer}}
	previous.NeedsInput = &NeedsInputDetail{CurrentQuestion: &identity}
	previous.LatestDecision = &reference
	next := previous
	next.Input.Answers = nil
	next.Input.TransitionSequence = 1
	if err := ValidateExecutionStateTransition(previous, next); err == nil || !strings.Contains(err.Error(), "must not disappear") {
		t.Fatalf("disappearance error=%v", err)
	}
	next = previous
	next.Input.Answers = append([]InputAnswerRecord(nil), previous.Input.Answers...)
	next.Input.Answers[0].Provenance.Actor = "someone-else"
	if err := ValidateExecutionStateTransition(previous, next); err == nil || !strings.Contains(err.Error(), "was rewritten") {
		t.Fatalf("rewrite error=%v", err)
	}
}

func TestNeedsInputDecisionRejectsMalformedCompositions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SupervisorDecision)
		want   string
	}{
		{"missing outcome", func(d *SupervisorDecision) { d.NeedsInput = nil }, "requires structured"},
		{"worker", func(d *SupervisorDecision) { d.WorkerProfile = WorkerProfilePlanner }, "must not select"},
		{"strategy", func(d *SupervisorDecision) { d.Strategy = &Strategy{Approach: "guess"} }, "must not include strategy"},
		{"success criteria", func(d *SupervisorDecision) { d.SuccessCriteria = []string{"unrelated"} }, "must not include success_criteria"},
		{"correction", func(d *SupervisorDecision) { d.FindingIDs = []string{"finding-one"} }, "only valid for action"},
		{"wrong task", func(d *SupervisorDecision) { d.NeedsInput.TaskID = "task-2"; rehashQuestion(t, d.NeedsInput) }, "task identity mismatch"},
		{"bad question id", func(d *SupervisorDecision) { d.NeedsInput.QuestionID = "Bad"; rehashQuestion(t, d.NeedsInput) }, "invalid question_id"},
		{"zero revision", func(d *SupervisorDecision) { d.NeedsInput.Revision = 0; rehashQuestion(t, d.NeedsInput) }, "revision must be positive"},
		{"one option", func(d *SupervisorDecision) {
			d.NeedsInput.Options = d.NeedsInput.Options[:1]
			rehashQuestion(t, d.NeedsInput)
		}, "at least two"},
		{"duplicate option id", func(d *SupervisorDecision) {
			d.NeedsInput.Options[1].ID = d.NeedsInput.Options[0].ID
			rehashQuestion(t, d.NeedsInput)
		}, "duplicate option id"},
		{"ambiguous meanings", func(d *SupervisorDecision) {
			d.NeedsInput.Options[1].Meaning = "  ENABLE THE FEATURE BY DEFAULT. "
			rehashQuestion(t, d.NeedsInput)
		}, "ambiguous"},
		{"outside recommendation", func(d *SupervisorDecision) {
			d.NeedsInput.Recommendation.OptionID = "third"
			rehashQuestion(t, d.NeedsInput)
		}, "offered option"},
		{"missing recommendation rationale", func(d *SupervisorDecision) {
			d.NeedsInput.Recommendation.Rationale = " "
			rehashQuestion(t, d.NeedsInput)
		}, "recommendation rationale"},
		{"changed content identity", func(d *SupervisorDecision) { d.NeedsInput.Question = "Changed?" }, "does not match deterministic"},
		{"dependent work options", func(d *SupervisorDecision) {
			d.NeedsInput.IndependentWork[0].IndependentOfOptionIDs = []string{"enable"}
			rehashQuestion(t, d.NeedsInput)
		}, "name every offered option"},
		{"source mutating independent work", func(d *SupervisorDecision) {
			w := &d.NeedsInput.IndependentWork[0]
			w.Action = ActionImplement
			w.WorkerProfile = WorkerProfileImplementer
			rehashQuestion(t, d.NeedsInput)
		}, "only compatible plan/planner or audit/auditor"},
		{"unsafe independent effect", func(d *SupervisorDecision) {
			d.NeedsInput.IndependentWork[0].SourceEffect = "source_mutating"
			rehashQuestion(t, d.NeedsInput)
		}, "read_only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := needsInputDecisionFixture(t)
			tt.mutate(&d)
			if err := d.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v want %q", err, tt.want)
			}
		})
	}
}

func needsInputDecisionFixture(t *testing.T) SupervisorDecision {
	t.Helper()
	q := NeedsInputQuestion{
		TaskID: "task-1", QuestionID: "feature-mode", Revision: 1,
		Question: "Should the feature be enabled by default?", BlockingReason: "The specification does not choose a default and either behavior changes the public contract.",
		Options:         []NeedsInputOption{{ID: "enable", Meaning: "Enable the feature by default."}, {ID: "disable", Meaning: "Keep the feature disabled by default."}},
		Recommendation:  NeedsInputRecommendation{OptionID: "disable", Rationale: "Preserves current behavior."},
		Evidence:        []EvidenceReference{{Kind: EvidenceKindTask, Reference: ".agent/tasks/task-1.md", Detail: "The exact task omits the default."}},
		IndependentWork: []IndependentWorkDeclaration{{ID: "audit-current-contract", Action: ActionAudit, WorkerProfile: WorkerProfileAuditor, Description: "Audit the current documented behavior without changing source.", SourceEffect: InputSourceEffectReadOnly, IndependentOfOptionIDs: []string{"enable", "disable"}, Inputs: []EvidenceReference{{Kind: EvidenceKindRepository, Reference: "README.md", Detail: "Current contract evidence."}}}},
	}
	rehashQuestion(t, &q)
	return SupervisorDecision{TaskID: "task-1", Action: ActionNeedsInput, Rationale: "An operator product decision is required.", Inputs: append([]EvidenceReference(nil), q.Evidence...), NeedsInput: &q}
}

func rehashQuestion(t *testing.T, q *NeedsInputQuestion) {
	t.Helper()
	q.ContentSHA256 = ""
	hash, err := QuestionContentSHA256(*q)
	if err != nil {
		t.Fatal(err)
	}
	q.ContentSHA256 = hash
}
