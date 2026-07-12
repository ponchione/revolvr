package autonomousinput

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
)

func TestQuestionAnswerResumeLifecyclePersistsAndReplays(t *testing.T) {
	repo, snapshot := inputRepo(t, "task-1")
	initialAttempts := snapshot.State.Attempts
	basePlan := snapshot.State.Plan
	baseAcceptance := append([]autonomous.AcceptanceCriterion(nil), snapshot.State.AcceptanceCriteria...)
	baseFindings := append([]autonomous.FindingResolution(nil), snapshot.State.FindingResolutions...)
	baseOptional := append([]autonomous.OptionalRoleOccurrence(nil), snapshot.State.OptionalRoles...)
	decision, reference := inputDecision(t, "task-1", "product-mode", 1)
	questionRequest := QuestionRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "question-operation", Expected: snapshot.Expected(), Decision: decision, Reference: reference, SourceRevision: strings.Repeat("a", 64), SourceSafety: autonomouspolicy.SourceSafetySafe, RecordedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	questionResult, err := RecordQuestion(context.Background(), questionRequest)
	if err != nil {
		t.Fatal(err)
	}
	if questionResult.Yield.Disposition != autonomouspolicy.NeedsInputYieldClean || questionResult.State.State.Lifecycle != autonomous.LifecycleStateNeedsInput || questionResult.State.State.NeedsInput.CurrentQuestion == nil {
		t.Fatalf("question result=%+v", questionResult)
	}
	if !reflect.DeepEqual(questionResult.State.State.Attempts, initialAttempts) {
		t.Fatal("recording question changed AW-15 budgets")
	}
	questionReplay, err := RecordQuestion(context.Background(), questionRequest)
	if err != nil || questionReplay.Disposition != autonomousstate.CommitReplayed || questionReplay.Yield.Disposition != autonomouspolicy.NeedsInputYieldClean {
		t.Fatalf("question replay=%+v err=%v", questionReplay, err)
	}
	identity := decision.NeedsInput.Identity()
	bad := AnswerRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "bad-option", Expected: questionResult.State.Expected(), AnswerID: "answer-one", Question: identity, OptionID: "unknown", Provenance: autonomous.AnswerProvenance{Kind: autonomous.AnswerProvenanceOperator, Actor: "operator@example"}, AnsweredAt: time.Date(2026, 7, 12, 12, 1, 0, 0, time.UTC)}
	if _, err := RecordAnswer(context.Background(), bad); !errors.Is(err, ErrUnknownOption) {
		t.Fatalf("unknown option error=%v", err)
	}
	wrongTask := bad
	wrongTask.OperationID = "wrong-task"
	wrongTask.TaskID = "task-2"
	if _, err := RecordAnswer(context.Background(), wrongTask); !errors.Is(err, autonomousstate.ErrTaskMissing) {
		t.Fatalf("wrong-task answer=%v", err)
	}
	changedIdentity := bad
	changedIdentity.OperationID = "changed-identity"
	changedIdentity.Question.ContentSHA256 = strings.Repeat("f", 64)
	if _, err := RecordAnswer(context.Background(), changedIdentity); !errors.Is(err, ErrStaleQuestion) {
		t.Fatalf("changed question identity=%v", err)
	}
	answerRequest := bad
	answerRequest.OperationID = "answer-operation"
	answerRequest.OptionID = "keep-current"
	answerResult, err := RecordAnswer(context.Background(), answerRequest)
	if err != nil {
		t.Fatal(err)
	}
	if answerResult.State.State.Lifecycle != autonomous.LifecycleStateNeedsInput || len(answerResult.State.State.Input.Answers) != 1 {
		t.Fatalf("answer state=%+v", answerResult.State.State)
	}
	replayed, err := RecordAnswer(context.Background(), answerRequest)
	if err != nil || replayed.Disposition != autonomousstate.CommitReplayed {
		t.Fatalf("answer replay=%+v err=%v", replayed, err)
	}
	conflict := answerRequest
	conflict.OptionID = "adopt-new"
	if _, err := RecordAnswer(context.Background(), conflict); !errors.Is(err, autonomousstate.ErrOperationConflict) {
		t.Fatalf("operation conflict=%v", err)
	}
	contradictory := answerRequest
	contradictory.OperationID = "answer-again"
	contradictory.AnswerID = "answer-two"
	contradictory.Expected = answerResult.State.Expected()
	if _, err := RecordAnswer(context.Background(), contradictory); !errors.Is(err, ErrAlreadyAnswered) {
		t.Fatalf("contradictory answer=%v", err)
	}
	stale := contradictory
	stale.OperationID = "stale-answer"
	stale.Question.Revision++
	if _, err := RecordAnswer(context.Background(), stale); !errors.Is(err, ErrStaleQuestion) {
		t.Fatalf("stale answer=%v", err)
	}
	resumeRequest := ResumeRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "resume-operation", Expected: answerResult.State.Expected(), ResumeID: "resume-one", Question: identity, AnswerID: "answer-one", ResumedAt: time.Date(2026, 7, 12, 12, 2, 0, 0, time.UTC)}
	resumeResult, err := Resume(context.Background(), resumeRequest)
	if err != nil {
		t.Fatal(err)
	}
	state := resumeResult.State.State
	if state.Lifecycle != autonomous.LifecycleStateReady || state.NeedsInput != nil || len(state.Input.Questions) != 1 || len(state.Input.Answers) != 1 || len(state.Input.Resumes) != 1 {
		t.Fatalf("resumed state=%+v", state)
	}
	if !reflect.DeepEqual(state.Attempts, initialAttempts) || !reflect.DeepEqual(state.Plan, basePlan) || !reflect.DeepEqual(state.AcceptanceCriteria, baseAcceptance) || !reflect.DeepEqual(state.FindingResolutions, baseFindings) || !reflect.DeepEqual(state.OptionalRoles, baseOptional) {
		t.Fatal("input lifecycle rewrote unrelated evidence or budgets")
	}
	resumeReplay, err := Resume(context.Background(), resumeRequest)
	if err != nil || resumeReplay.Disposition != autonomousstate.CommitReplayed {
		t.Fatalf("resume replay=%+v err=%v", resumeReplay, err)
	}
	double := resumeRequest
	double.OperationID = "resume-again"
	double.ResumeID = "resume-two"
	double.Expected = resumeResult.State.Expected()
	if _, err := Resume(context.Background(), double); !errors.Is(err, ErrAlreadyResumed) {
		t.Fatalf("double resume=%v", err)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	reopened, found, err := store.Load(context.Background(), "task-1")
	if err != nil || !found || !reflect.DeepEqual(reopened.State, state) {
		t.Fatalf("restart state found=%t err=%v state=%+v", found, err, reopened.State)
	}
}

func TestSupersededAndChangedQuestionsRejectStaleAnswers(t *testing.T) {
	repo, snapshot := inputRepo(t, "task-1")
	first, firstRef := inputDecision(t, "task-1", "product-mode", 1)
	one, err := RecordQuestion(context.Background(), QuestionRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "q1", Expected: snapshot.Expected(), Decision: first, Reference: firstRef, SourceRevision: strings.Repeat("a", 64), SourceSafety: autonomouspolicy.SourceSafetySafe, RecordedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	second, secondRef := inputDecision(t, "task-1", "product-mode", 2)
	second.NeedsInput.Options[1].Meaning = "Adopt the revised behavior with migration."
	rehash(t, second.NeedsInput)
	two, err := RecordQuestion(context.Background(), QuestionRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "q2", Expected: one.State.Expected(), Decision: second, Reference: secondRef, SourceRevision: strings.Repeat("a", 64), SourceSafety: autonomouspolicy.SourceSafetySafe, RecordedAt: time.Now().UTC().Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	answer := AnswerRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "stale", Expected: two.State.Expected(), AnswerID: "answer-old", Question: first.NeedsInput.Identity(), OptionID: "keep-current", Provenance: autonomous.AnswerProvenance{Kind: autonomous.AnswerProvenanceOperator, Actor: "operator"}, AnsweredAt: time.Now().UTC()}
	if _, err := RecordAnswer(context.Background(), answer); !errors.Is(err, ErrStaleQuestion) {
		t.Fatalf("superseded answer=%v", err)
	}
}

func TestLegacyNeedsInputFailsClosedForAnswerAuthority(t *testing.T) {
	repo, snapshot := inputRepo(t, "task-1")
	legacy := snapshot.State
	legacy.Lifecycle = autonomous.LifecycleStateNeedsInput
	legacy.NeedsInput = &autonomous.NeedsInputDetail{Reason: "Legacy prose cannot authorize an answer."}
	raw, err := autonomousstate.MarshalState(legacy)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(repo, ".revolvr", "autonomous", "tasks", "task-1", "state.json"), raw)
	store, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	current, _, err := store.Load(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	request := AnswerRequest{RepositoryRoot: repo, TaskID: "task-1", OperationID: "legacy-answer", Expected: current.Expected(), AnswerID: "answer-one", Question: autonomous.QuestionIdentity{QuestionID: "fabricated", Revision: 1, ContentSHA256: strings.Repeat("a", 64)}, OptionID: "fabricated", Provenance: autonomous.AnswerProvenance{Kind: autonomous.AnswerProvenanceOperator, Actor: "operator"}, AnsweredAt: time.Now().UTC()}
	if _, err := RecordAnswer(context.Background(), request); !errors.Is(err, ErrLegacyQuestion) {
		t.Fatalf("legacy answer error=%v", err)
	}
}

func inputRepo(t *testing.T, taskID string) (string, autonomousstate.Snapshot) {
	t.Helper()
	repo := t.TempDir()
	task := []byte("---\nid: " + taskID + "\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/" + taskID + "/state.json\n---\n# Task\n\nExact specification.\n")
	write(t, filepath.Join(repo, ".agent", "tasks", taskID+".md"), task)
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStateReady, Plan: &autonomous.TaskPlan{TaskID: taskID, ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task", Detail: "Exact task."}}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "Work.", Status: autonomous.PlanStepStatusPending}}}, AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "Works.", Status: autonomous.AcceptanceStatusPending}}, Attempts: autonomous.AttemptState{TotalAttempts: 1, RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeLimited, Limit: 4, Consumed: 1}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}}}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, ".revolvr", "autonomous", "tasks", taskID, "state.json")
	write(t, path, raw)
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("load found=%t err=%v", found, err)
	}
	return repo, snapshot
}

func inputDecision(t *testing.T, taskID, questionID string, revision int64) (autonomous.SupervisorDecision, autonomous.DecisionReference) {
	t.Helper()
	q := autonomous.NeedsInputQuestion{TaskID: taskID, QuestionID: questionID, Revision: revision, Question: "Which product behavior should be authoritative?", BlockingReason: "The exact specification permits incompatible public behaviors.", Options: []autonomous.NeedsInputOption{{ID: "keep-current", Meaning: "Preserve current behavior."}, {ID: "adopt-new", Meaning: "Adopt the proposed behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep-current", Rationale: "Minimizes compatibility risk."}, Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: ".agent/tasks/" + taskID + ".md", Detail: "The task is ambiguous."}}}
	rehash(t, &q)
	decision := autonomous.SupervisorDecision{TaskID: taskID, Action: autonomous.ActionNeedsInput, Rationale: "Operator input required.", Inputs: append([]autonomous.EvidenceReference(nil), q.Evidence...), NeedsInput: &q}
	reference := autonomous.DecisionReference{DecisionID: "decision-" + questionID + strings.Repeat("x", int(revision-1)), RunID: "supervisor-" + questionID + strings.Repeat("x", int(revision-1)), TaskID: taskID, Action: autonomous.ActionNeedsInput, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: ".revolvr/runs/supervisor/decision.json", Detail: "Validated decision."}, CreatedAt: time.Date(2026, 7, 12, 11, 0, int(revision), 0, time.UTC)}
	return decision, reference
}
func rehash(t *testing.T, q *autonomous.NeedsInputQuestion) {
	t.Helper()
	q.ContentSHA256 = ""
	value, err := autonomous.QuestionContentSHA256(*q)
	if err != nil {
		t.Fatal(err)
	}
	q.ContentSHA256 = value
}
func write(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
