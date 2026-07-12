package autonomousstate

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
)

func TestCommitInputRecoversBeforeAndAfterStateRename(t *testing.T) {
	for _, tt := range []struct {
		name      string
		point     FailurePoint
		committed bool
	}{{"before", FailureBeforeStateRename, false}, {"after", FailureAfterStateRename, true}} {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := stateTestRepository(t, "task-1")
			request := inputStoreQuestionRequest(t)
			statePath := filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1")))
			writeAuditStoreFile(t, statePath, mustMarshalState(t, request.PreviousState))
			injected := false
			store, err := New(Config{RepositoryRoot: repo, FailureInjector: func(point FailurePoint) error {
				if point == tt.point && !injected {
					injected = true
					return errors.New("crash")
				}
				return nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.CommitInput(context.Background(), request); err == nil {
				t.Fatal("expected injected failure")
			}
			clean, _ := New(Config{RepositoryRoot: repo})
			snapshot, found, err := clean.Load(context.Background(), "task-1")
			if err != nil || !found {
				t.Fatal(err)
			}
			if got := len(snapshot.State.Input.Questions); (got == 1) != tt.committed {
				t.Fatalf("questions=%d committed=%t", got, tt.committed)
			}
			result, err := clean.CommitInput(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if tt.committed && result.Disposition != CommitReplayed {
				t.Fatalf("disposition=%s", result.Disposition)
			}
			if !tt.committed && len(result.Current.State.Input.Questions) != 1 {
				t.Fatal("retry did not commit question")
			}
		})
	}
}

func TestInputOperationConflictAndStaleWrite(t *testing.T) {
	repo, _ := stateTestRepository(t, "task-1")
	request := inputStoreQuestionRequest(t)
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1"))), mustMarshalState(t, request.PreviousState))
	store, _ := New(Config{RepositoryRoot: repo})
	if _, err := store.CommitInput(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	conflict := request
	conflict.History.ApplicationSHA256 = strings.Repeat("f", 64)
	if _, err := store.CommitInput(context.Background(), conflict); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("conflict=%v", err)
	}
	stale := request
	stale.History.OperationID = "different-operation"
	stale.History.ApplicationSHA256 = strings.Repeat("e", 64)
	if _, err := store.CommitInput(context.Background(), stale); !errors.Is(err, ErrStaleWrite) {
		t.Fatalf("stale=%v", err)
	}
}

func TestCommittedAuditReconstructionTraversesInputTransition(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	auditRequest := auditStoreRequest(t, repo, taskRaw)
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1"))), mustMarshalState(t, auditRequest.PreviousState))
	store, _ := New(Config{RepositoryRoot: repo})
	auditResult, err := store.CommitAudit(context.Background(), auditRequest)
	if err != nil {
		t.Fatal(err)
	}
	request := inputStoreQuestionRequest(t)
	request.Expected = auditResult.Current.Expected()
	request.PreviousState = auditResult.Current.State
	next := request.PreviousState
	next.Input = request.NextState.Input
	next.Lifecycle = autonomous.LifecycleStateNeedsInput
	next.NeedsInput = request.NextState.NeedsInput
	next.LatestDecision = request.NextState.LatestDecision
	request.NextState = next
	previousRaw := mustMarshalState(t, request.PreviousState)
	nextRaw := mustMarshalState(t, next)
	request.History.PreviousState = stateIdentity(canonicalStatePath("task-1"), true, previousRaw)
	request.History.ResultingState = stateIdentity(canonicalStatePath("task-1"), true, nextRaw)
	questionResult, err := store.CommitInput(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	identity := request.History.Question.Question.Identity()
	answer := autonomous.InputAnswerRecord{Sequence: 2, AnswerID: "answer-one", TaskID: "task-1", Question: identity, OptionID: "keep", Provenance: autonomous.AnswerProvenance{Kind: autonomous.AnswerProvenanceOperator, Actor: "operator"}, AnsweredAt: time.Date(2026, 7, 12, 12, 2, 0, 0, time.UTC)}
	answerNext := questionResult.Current.State
	answerNext.Input.TransitionSequence = 2
	answerNext.Input.Answers = []autonomous.InputAnswerRecord{answer}
	answerPreviousRaw := mustMarshalState(t, questionResult.Current.State)
	answerNextRaw := mustMarshalState(t, answerNext)
	answerHistory := InputHistoryRecord{SchemaVersion: InputHistorySchemaVersion, TaskID: "task-1", OperationID: "answer-operation", ApplicationSHA256: strings.Repeat("c", 64), Sequence: 2, Kind: InputTransitionAnswer, CreatedAt: answer.AnsweredAt, Answer: &answer, PreviousState: stateIdentity(canonicalStatePath("task-1"), true, answerPreviousRaw), ResultingState: stateIdentity(canonicalStatePath("task-1"), true, answerNextRaw)}
	answerResult, err := store.CommitInput(context.Background(), InputCommitRequest{TaskID: "task-1", Expected: questionResult.Current.Expected(), PreviousState: questionResult.Current.State, NextState: answerNext, History: answerHistory})
	if err != nil {
		t.Fatal(err)
	}
	resume := autonomous.InputResumeRecord{Sequence: 3, ResumeID: "resume-one", TaskID: "task-1", Question: identity, AnswerID: "answer-one", ResumedAt: time.Date(2026, 7, 12, 12, 3, 0, 0, time.UTC)}
	resumeNext := answerResult.Current.State
	resumeNext.Input.TransitionSequence = 3
	resumeNext.Input.Resumes = []autonomous.InputResumeRecord{resume}
	resumeNext.Lifecycle = autonomous.LifecycleStateReady
	resumeNext.NeedsInput = nil
	resumePreviousRaw := mustMarshalState(t, answerResult.Current.State)
	resumeNextRaw := mustMarshalState(t, resumeNext)
	resumeHistory := InputHistoryRecord{SchemaVersion: InputHistorySchemaVersion, TaskID: "task-1", OperationID: "resume-operation", ApplicationSHA256: strings.Repeat("b", 64), Sequence: 3, Kind: InputTransitionResume, CreatedAt: resume.ResumedAt, Resume: &resume, PreviousState: stateIdentity(canonicalStatePath("task-1"), true, resumePreviousRaw), ResultingState: stateIdentity(canonicalStatePath("task-1"), true, resumeNextRaw)}
	if _, err := store.CommitInput(context.Background(), InputCommitRequest{TaskID: "task-1", Expected: answerResult.Current.Expected(), PreviousState: answerResult.Current.State, NextState: resumeNext, History: resumeHistory}); err != nil {
		t.Fatal(err)
	}
	current, found, err := store.LoadCurrentAudit(context.Background(), "task-1")
	if err != nil || !found || current.Report.Disposition != auditRequest.History.Report.Disposition {
		t.Fatalf("current audit found=%t err=%v report=%+v", found, err, current.Report)
	}
}

func inputStoreQuestionRequest(t *testing.T) InputCommitRequest {
	t.Helper()
	previous := stateTestPendingState("task-1")
	previous.Lifecycle = autonomous.LifecycleStateReady
	question := autonomous.NeedsInputQuestion{TaskID: "task-1", QuestionID: "product-mode", Revision: 1, Question: "Which behavior?", BlockingReason: "Ambiguous behavior.", Options: []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep behavior."}, {ID: "change", Meaning: "Change behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "Safer."}, Evidence: []autonomous.EvidenceReference{stateTestEvidence(autonomous.EvidenceKindTask, "task")}}
	hash, _ := autonomous.QuestionContentSHA256(question)
	question.ContentSHA256 = hash
	reference := autonomous.DecisionReference{DecisionID: "decision-input", RunID: "supervisor-input", TaskID: "task-1", Action: autonomous.ActionNeedsInput, Artifact: stateTestEvidence(autonomous.EvidenceKindFile, ".revolvr/runs/input/decision.json"), CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	record := autonomous.InputQuestionRecord{Sequence: 1, TaskID: "task-1", Question: question, Decision: reference, SourceRevision: strings.Repeat("a", 64), ResumeLifecycle: autonomous.LifecycleStateReady, RecordedAt: time.Date(2026, 7, 12, 12, 1, 0, 0, time.UTC)}
	next := previous
	next.Input = autonomous.InputState{TransitionSequence: 1, Questions: []autonomous.InputQuestionRecord{record}}
	identity := question.Identity()
	next.Lifecycle = autonomous.LifecycleStateNeedsInput
	next.NeedsInput = &autonomous.NeedsInputDetail{CurrentQuestion: &identity}
	next.LatestDecision = &reference
	previousRaw := mustMarshalState(t, previous)
	nextRaw := mustMarshalState(t, next)
	history := InputHistoryRecord{SchemaVersion: InputHistorySchemaVersion, TaskID: "task-1", OperationID: "question-operation", ApplicationSHA256: strings.Repeat("d", 64), Sequence: 1, Kind: InputTransitionQuestion, CreatedAt: record.RecordedAt, Question: &record, PreviousState: stateIdentity(canonicalStatePath("task-1"), true, previousRaw), ResultingState: stateIdentity(canonicalStatePath("task-1"), true, nextRaw)}
	return InputCommitRequest{TaskID: "task-1", Expected: ExpectedState{Exists: true, SHA256: hashBytes(previousRaw), ByteSize: len(previousRaw)}, PreviousState: previous, NextState: next, History: history}
}
