// Package autonomousinput applies one durable structured question, answer, or
// resume transition. It starts no model, worker, verification, audit, or commit.
package autonomousinput

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
)

var (
	ErrStaleQuestion   = errors.New("stale needs-input question")
	ErrUnknownOption   = errors.New("unknown needs-input option")
	ErrAlreadyAnswered = errors.New("needs-input question already answered")
	ErrAlreadyResumed  = errors.New("needs-input question already resumed")
	ErrLegacyQuestion  = errors.New("legacy needs-input detail has no answer or resume authority")
)

type QuestionRequest struct {
	RepositoryRoot          string
	TaskID                  string
	OperationID             string
	Expected                autonomousstate.ExpectedState
	Decision                autonomous.SupervisorDecision
	Reference               autonomous.DecisionReference
	SourceRevision          string
	SourceSafety            autonomouspolicy.SourceSafety
	SourceOperationInFlight bool
	RecordedAt              time.Time
}

type AnswerRequest struct {
	RepositoryRoot string
	TaskID         string
	OperationID    string
	Expected       autonomousstate.ExpectedState
	AnswerID       string
	Question       autonomous.QuestionIdentity
	OptionID       string
	Provenance     autonomous.AnswerProvenance
	AnsweredAt     time.Time
}

type ResumeRequest struct {
	RepositoryRoot string
	TaskID         string
	OperationID    string
	Expected       autonomousstate.ExpectedState
	ResumeID       string
	Question       autonomous.QuestionIdentity
	AnswerID       string
	ResumedAt      time.Time
}

type Result struct {
	Disposition autonomousstate.CommitDisposition
	State       autonomousstate.Snapshot
	History     autonomousstate.InputHistorySnapshot
	Yield       autonomouspolicy.NeedsInputYieldResult
}

func RecordQuestion(ctx context.Context, request QuestionRequest) (Result, error) {
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: request.RepositoryRoot})
	if err != nil {
		return Result{}, err
	}
	application, err := applicationSHA(request)
	if err != nil {
		return Result{}, err
	}
	if replay, ok, err := replay(ctx, store, request.TaskID, request.OperationID, application); err != nil || ok {
		return replay, err
	}
	previous, found, err := store.Load(ctx, request.TaskID)
	if err != nil || !found {
		return Result{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if err := exactExpected(request.Expected, previous); err != nil {
		return Result{}, err
	}
	if request.SourceSafety != autonomouspolicy.SourceSafetySafe || request.SourceOperationInFlight {
		return Result{}, errors.New("record needs-input question: source is not clean and idle")
	}
	if err := autonomouspolicy.ValidateEvidence(request.TaskID, autonomouspolicy.SourceEvidence{Revision: request.SourceRevision, Safety: request.SourceSafety}, nil, nil); err != nil {
		return Result{}, err
	}
	if err := request.Decision.Validate(); err != nil {
		return Result{}, err
	}
	if request.Decision.Action != autonomous.ActionNeedsInput || request.Decision.NeedsInput == nil || request.Decision.TaskID != request.TaskID {
		return Result{}, errors.New("record needs-input question: exact structured needs_input decision is required")
	}
	if err := request.Reference.Validate(); err != nil {
		return Result{}, err
	}
	if request.Reference.TaskID != request.TaskID || request.Reference.Action != autonomous.ActionNeedsInput || request.Reference.WorkerProfile != "" {
		return Result{}, errors.New("record needs-input question: decision reference mismatch")
	}
	if previous.State.Lifecycle != autonomous.LifecycleStatePending && previous.State.Lifecycle != autonomous.LifecycleStateReady && previous.State.Lifecycle != autonomous.LifecycleStateNeedsInput {
		return Result{}, fmt.Errorf("record needs-input question: lifecycle %q cannot suspend", previous.State.Lifecycle)
	}
	next := previous.State
	sequence := next.Input.TransitionSequence + 1
	resumeLifecycle := previous.State.Lifecycle
	var supersedes *autonomous.QuestionIdentity
	if len(previous.State.Input.Questions) > 0 {
		prior := previous.State.Input.Questions[len(previous.State.Input.Questions)-1].Question.Identity()
		supersedes = &prior
	}
	if previous.State.Lifecycle == autonomous.LifecycleStateNeedsInput {
		current, currentRecord, err := currentQuestion(previous.State)
		if err != nil {
			return Result{}, err
		}
		if answered(previous.State, current) {
			return Result{}, errors.New("record needs-input question: answered question must be explicitly resumed before another question")
		}
		question := request.Decision.NeedsInput
		if question.QuestionID != current.QuestionID || question.Revision != current.Revision+1 {
			return Result{}, fmt.Errorf("%w: superseding question must retain question_id and increment revision by one", ErrStaleQuestion)
		}
		resumeLifecycle = currentRecord.ResumeLifecycle
	}
	record := autonomous.InputQuestionRecord{Sequence: sequence, TaskID: request.TaskID, Question: *request.Decision.NeedsInput, Decision: request.Reference, SourceRevision: request.SourceRevision, ResumeLifecycle: resumeLifecycle, Supersedes: supersedes, RecordedAt: request.RecordedAt.UTC()}
	next.Input.TransitionSequence = sequence
	next.Input.Questions = append(next.Input.Questions, record)
	identity := record.Question.Identity()
	next.Lifecycle = autonomous.LifecycleStateNeedsInput
	next.NeedsInput = &autonomous.NeedsInputDetail{CurrentQuestion: &identity}
	next.LatestDecision = &request.Reference
	next.Terminal = nil
	return commit(ctx, store, previous, next, request.OperationID, application, request.RecordedAt, autonomousstate.InputTransitionQuestion, &record, nil, nil, request.SourceRevision, false)
}

func RecordAnswer(ctx context.Context, request AnswerRequest) (Result, error) {
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: request.RepositoryRoot})
	if err != nil {
		return Result{}, err
	}
	application, err := applicationSHA(request)
	if err != nil {
		return Result{}, err
	}
	if replay, ok, err := replay(ctx, store, request.TaskID, request.OperationID, application); err != nil || ok {
		return replay, err
	}
	previous, found, err := store.Load(ctx, request.TaskID)
	if err != nil || !found {
		return Result{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if err := exactExpected(request.Expected, previous); err != nil {
		return Result{}, err
	}
	current, questionRecord, err := currentQuestion(previous.State)
	if err != nil {
		return Result{}, err
	}
	if request.Question != current {
		return Result{}, fmt.Errorf("%w: current=%+v supplied=%+v", ErrStaleQuestion, current, request.Question)
	}
	if answered(previous.State, current) {
		return Result{}, ErrAlreadyAnswered
	}
	foundOption := false
	for _, option := range questionRecord.Question.Options {
		if option.ID == request.OptionID {
			foundOption = true
		}
	}
	if !foundOption {
		return Result{}, ErrUnknownOption
	}
	sequence := previous.State.Input.TransitionSequence + 1
	record := autonomous.InputAnswerRecord{Sequence: sequence, AnswerID: request.AnswerID, TaskID: request.TaskID, Question: request.Question, OptionID: request.OptionID, Provenance: request.Provenance, AnsweredAt: request.AnsweredAt.UTC()}
	if err := record.Validate(request.TaskID); err != nil {
		return Result{}, err
	}
	next := previous.State
	next.Input.TransitionSequence = sequence
	next.Input.Answers = append(next.Input.Answers, record)
	return commit(ctx, store, previous, next, request.OperationID, application, request.AnsweredAt, autonomousstate.InputTransitionAnswer, nil, &record, nil, questionRecord.SourceRevision, false)
}

func Resume(ctx context.Context, request ResumeRequest) (Result, error) {
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: request.RepositoryRoot})
	if err != nil {
		return Result{}, err
	}
	application, err := applicationSHA(request)
	if err != nil {
		return Result{}, err
	}
	if replay, ok, err := replay(ctx, store, request.TaskID, request.OperationID, application); err != nil || ok {
		return replay, err
	}
	previous, found, err := store.Load(ctx, request.TaskID)
	if err != nil || !found {
		return Result{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if err := exactExpected(request.Expected, previous); err != nil {
		return Result{}, err
	}
	for _, prior := range previous.State.Input.Resumes {
		if prior.Question == request.Question {
			return Result{}, ErrAlreadyResumed
		}
	}
	current, questionRecord, err := currentQuestion(previous.State)
	if err != nil {
		return Result{}, err
	}
	if request.Question != current {
		return Result{}, ErrStaleQuestion
	}
	for _, resume := range previous.State.Input.Resumes {
		if resume.Question == current {
			return Result{}, ErrAlreadyResumed
		}
	}
	var answer *autonomous.InputAnswerRecord
	for i := range previous.State.Input.Answers {
		if previous.State.Input.Answers[i].Question == current {
			answer = &previous.State.Input.Answers[i]
		}
	}
	if answer == nil || answer.AnswerID != request.AnswerID {
		return Result{}, fmt.Errorf("%w: exact durable answer is required", ErrStaleQuestion)
	}
	sequence := previous.State.Input.TransitionSequence + 1
	record := autonomous.InputResumeRecord{Sequence: sequence, ResumeID: request.ResumeID, TaskID: request.TaskID, Question: current, AnswerID: request.AnswerID, ResumedAt: request.ResumedAt.UTC()}
	if err := record.Validate(request.TaskID); err != nil {
		return Result{}, err
	}
	next := previous.State
	next.Input.TransitionSequence = sequence
	next.Input.Resumes = append(next.Input.Resumes, record)
	next.Lifecycle = questionRecord.ResumeLifecycle
	next.NeedsInput = nil
	return commit(ctx, store, previous, next, request.OperationID, application, request.ResumedAt, autonomousstate.InputTransitionResume, nil, nil, &record, questionRecord.SourceRevision, true)
}

func currentQuestion(state autonomous.ExecutionState) (autonomous.QuestionIdentity, autonomous.InputQuestionRecord, error) {
	if state.Lifecycle != autonomous.LifecycleStateNeedsInput || state.NeedsInput == nil {
		return autonomous.QuestionIdentity{}, autonomous.InputQuestionRecord{}, ErrStaleQuestion
	}
	if state.NeedsInput.CurrentQuestion == nil {
		return autonomous.QuestionIdentity{}, autonomous.InputQuestionRecord{}, ErrLegacyQuestion
	}
	current := *state.NeedsInput.CurrentQuestion
	for _, record := range state.Input.Questions {
		if record.Question.Identity() == current {
			return current, record, nil
		}
	}
	return autonomous.QuestionIdentity{}, autonomous.InputQuestionRecord{}, ErrStaleQuestion
}

func answered(state autonomous.ExecutionState, question autonomous.QuestionIdentity) bool {
	for _, answer := range state.Input.Answers {
		if answer.Question == question {
			return true
		}
	}
	return false
}
func exactExpected(expected autonomousstate.ExpectedState, snapshot autonomousstate.Snapshot) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	if !expected.Exists || expected.SHA256 != snapshot.SHA256 || expected.ByteSize != snapshot.ByteSize {
		return autonomousstate.ErrStaleWrite
	}
	return nil
}
func applicationSHA(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

func replay(ctx context.Context, store *autonomousstate.Store, taskID, operationID, application string) (Result, bool, error) {
	if strings.TrimSpace(operationID) == "" {
		return Result{}, false, errors.New("input operation_id is required")
	}
	history, found, err := store.LoadInputOperation(ctx, taskID, operationID)
	if err != nil || !found {
		return Result{}, false, err
	}
	if history.Record.ApplicationSHA256 != application {
		return Result{}, true, fmt.Errorf("%w: input operation material changed", autonomousstate.ErrOperationConflict)
	}
	snapshot, stateFound, err := store.Load(ctx, taskID)
	if err != nil || !stateFound {
		return Result{}, true, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	applied := false
	switch history.Record.Kind {
	case autonomousstate.InputTransitionQuestion:
		for _, v := range snapshot.State.Input.Questions {
			if reflect.DeepEqual(v, *history.Record.Question) {
				applied = true
			}
		}
	case autonomousstate.InputTransitionAnswer:
		for _, v := range snapshot.State.Input.Answers {
			if reflect.DeepEqual(v, *history.Record.Answer) {
				applied = true
			}
		}
	case autonomousstate.InputTransitionResume:
		for _, v := range snapshot.State.Input.Resumes {
			if reflect.DeepEqual(v, *history.Record.Resume) {
				applied = true
			}
		}
	}
	if !applied {
		return Result{}, false, nil
	}
	result := Result{Disposition: autonomousstate.CommitReplayed, State: snapshot, History: history}
	if snapshot.State.Lifecycle == autonomous.LifecycleStateNeedsInput && snapshot.State.NeedsInput != nil && snapshot.State.NeedsInput.CurrentQuestion != nil {
		current := *snapshot.State.NeedsInput.CurrentQuestion
		for _, q := range snapshot.State.Input.Questions {
			if q.Question.Identity() == current {
				result.Yield = autonomouspolicy.EvaluateNeedsInputYield(autonomouspolicy.NeedsInputYieldInput{TaskID: taskID, State: snapshot.State, Source: autonomouspolicy.SourceEvidence{Revision: q.SourceRevision, Safety: autonomouspolicy.SourceSafetySafe}})
			}
		}
	}
	return result, true, nil
}

func commit(ctx context.Context, store *autonomousstate.Store, previous autonomousstate.Snapshot, next autonomous.ExecutionState, operationID, application string, created time.Time, kind autonomousstate.InputTransitionKind, question *autonomous.InputQuestionRecord, answer *autonomous.InputAnswerRecord, resume *autonomous.InputResumeRecord, sourceRevision string, resumed bool) (Result, error) {
	previousIdentity, err := autonomousstate.StateIdentityFor(previous.SourcePath, true, previous.State)
	if err != nil {
		return Result{}, err
	}
	nextIdentity, err := autonomousstate.StateIdentityFor(previous.SourcePath, true, next)
	if err != nil {
		return Result{}, err
	}
	record := autonomousstate.InputHistoryRecord{SchemaVersion: autonomousstate.InputHistorySchemaVersion, TaskID: previous.State.TaskID, OperationID: operationID, ApplicationSHA256: application, Sequence: next.Input.TransitionSequence, Kind: kind, CreatedAt: created.UTC(), Question: question, Answer: answer, Resume: resume, PreviousState: previousIdentity, ResultingState: nextIdentity}
	result, err := store.CommitInput(ctx, autonomousstate.InputCommitRequest{TaskID: previous.State.TaskID, Expected: previous.Expected(), PreviousState: previous.State, NextState: next, History: record})
	if err != nil {
		return Result{}, err
	}
	out := Result{Disposition: result.Disposition, State: result.Current, History: result.History}
	if !resumed {
		out.Yield = autonomouspolicy.EvaluateNeedsInputYield(autonomouspolicy.NeedsInputYieldInput{TaskID: next.TaskID, State: result.Current.State, Source: autonomouspolicy.SourceEvidence{Revision: sourceRevision, Safety: autonomouspolicy.SourceSafetySafe}})
	}
	return out, nil
}
