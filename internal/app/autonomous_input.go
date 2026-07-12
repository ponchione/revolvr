package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousinput"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousview"
	"revolvr/internal/id"
	"revolvr/internal/redact"
	"revolvr/internal/taskfile"
)

// AutonomousTaskSelector is the bounded identity summary used by the TUI to
// select active tasks and tracked archives before loading the rich AW-27 view.
type AutonomousTaskSelector struct {
	Selector    string
	TaskID      string
	Label       string
	SourceKind  autonomousview.SourceKind
	Title       string
	Status      string
	ArchiveID   string
	Disposition string
}

// ListAutonomousTaskSelectors performs only strict task/archive discovery. It
// does not load state, verify archives, or create runtime evidence.
func ListAutonomousTaskSelectors(ctx context.Context, cfg Config) ([]AutonomousTaskSelector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return nil, err
	}
	redactor, _, err := redact.New(runCfg.SafetyDeclaration.Redaction, os.LookupEnv)
	if err != nil {
		return nil, err
	}
	tasks, err := taskfile.List(paths.WorkDir)
	if err != nil {
		return nil, redactor.Error(fmt.Errorf("list task evidence selectors: %w", err))
	}
	archives, err := autonomousarchive.List(paths.WorkDir)
	if err != nil {
		return nil, redactor.Error(fmt.Errorf("list task evidence selectors: %w", err))
	}
	selectors := make([]AutonomousTaskSelector, 0, len(tasks)+len(archives))
	for _, task := range tasks {
		selectors = append(selectors, AutonomousTaskSelector{
			Selector:   task.ID,
			TaskID:     task.ID,
			Label:      redactor.String(strings.TrimSpace(task.ID + " - " + task.Title)),
			SourceKind: autonomousview.SourceActive,
			Title:      task.Title,
			Status:     string(task.Status),
		})
	}
	for _, entry := range archives {
		manifest := entry.Manifest
		selectors = append(selectors, AutonomousTaskSelector{
			Selector:    manifest.ArchiveID,
			TaskID:      manifest.TaskID,
			Label:       redactor.String(manifest.TaskID + " / " + manifest.ArchiveID),
			SourceKind:  autonomousview.SourceArchive,
			Status:      string(manifest.Disposition),
			ArchiveID:   manifest.ArchiveID,
			Disposition: string(manifest.Disposition),
		})
	}
	return selectors, nil
}

type AnswerAutonomousInputRequest struct {
	TaskID       string
	QuestionID   string
	Revision     int64
	ContentSHA   string
	OptionID     string
	Operator     string
	OperationID  string
	AnswerID     string
	ResumeID     string
	Clock        func() time.Time
	BeforeResume func() error
}

type AnswerAutonomousInputResult struct {
	TaskID            string
	QuestionID        string
	Revision          int64
	OptionID          string
	AnswerID          string
	AnswerDisposition autonomousstate.CommitDisposition
	ResumeDisposition autonomousstate.CommitDisposition
	AnswerPersisted   bool
	Resumed           bool
}

// AnswerAutonomousInput reloads exact current authority, records one explicit
// operator answer, and then performs the separately durable resume transition.
// A nonzero result with AnswerPersisted=true remains authoritative if resume
// fails and is deliberately returned with the error.
func AnswerAutonomousInput(ctx context.Context, cfg Config, request AnswerAutonomousInputRequest) (AnswerAutonomousInputResult, error) {
	result := AnswerAutonomousInputResult{
		TaskID:     strings.TrimSpace(request.TaskID),
		QuestionID: strings.TrimSpace(request.QuestionID),
		Revision:   request.Revision,
		OptionID:   strings.TrimSpace(request.OptionID),
	}
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return result, err
	}
	runCfg, err := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
	if err != nil {
		return result, err
	}
	redactor, _, err := redact.New(runCfg.SafetyDeclaration.Redaction, os.LookupEnv)
	if err != nil {
		return result, err
	}
	fail := func(err error) (AnswerAutonomousInputResult, error) {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, redactor.Error(err)
	}
	if result.TaskID == "" || result.QuestionID == "" || request.Revision <= 0 || strings.TrimSpace(request.ContentSHA) == "" || result.OptionID == "" || strings.TrimSpace(request.Operator) == "" || request.Operator != strings.TrimSpace(request.Operator) {
		return fail(errors.New("answer autonomous input: exact question, option, and normalized operator are required"))
	}

	view, err := loadAutonomousTaskView(ctx, paths.WorkDir, result.TaskID)
	if err != nil {
		return fail(err)
	}
	if view.Identity.SourceKind != autonomousview.SourceActive || view.Identity.Workflow != taskfile.WorkflowAutonomousV1 {
		return fail(errors.New("answer autonomous input: an active autonomous-v1 task is required"))
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: paths.WorkDir})
	if err != nil {
		return fail(err)
	}
	snapshot, found, err := store.Load(ctx, result.TaskID)
	if err != nil || !found {
		return fail(errors.Join(err, autonomousstate.ErrStateMissing))
	}
	question := autonomous.QuestionIdentity{QuestionID: result.QuestionID, Revision: result.Revision, ContentSHA256: request.ContentSHA}
	if snapshot.State.Lifecycle != autonomous.LifecycleStateNeedsInput || snapshot.State.NeedsInput == nil || snapshot.State.NeedsInput.CurrentQuestion == nil {
		return fail(errors.New("answer autonomous input: current typed question is not waiting for an answer"))
	}
	if *snapshot.State.NeedsInput.CurrentQuestion != question {
		return fail(autonomousinput.ErrStaleQuestion)
	}
	optionOffered := false
	for _, record := range snapshot.State.Input.Questions {
		if record.Question.Identity() != question {
			continue
		}
		for _, option := range record.Question.Options {
			if option.ID == result.OptionID {
				optionOffered = true
			}
		}
	}
	if !optionOffered {
		return fail(autonomousinput.ErrUnknownOption)
	}
	now := time.Now().UTC()
	if request.Clock != nil {
		now = request.Clock().UTC()
	}
	operationID := strings.TrimSpace(request.OperationID)
	if operationID == "" {
		operationID = "input-" + id.New()
	}
	answerID := strings.TrimSpace(request.AnswerID)
	if answerID == "" {
		answerID = "answer-" + id.New()
	}
	resumeID := strings.TrimSpace(request.ResumeID)
	if resumeID == "" {
		resumeID = "resume-" + id.New()
	}
	result.AnswerID = answerID

	answerSnapshot := snapshot
	existingAnswer := false
	for _, answer := range snapshot.State.Input.Answers {
		if answer.Question != question {
			continue
		}
		if answer.OptionID != result.OptionID {
			return fail(fmt.Errorf("%w: current durable answer selected option %s", autonomousinput.ErrAlreadyAnswered, answer.OptionID))
		}
		answerID = answer.AnswerID
		result.AnswerID = answer.AnswerID
		result.AnswerDisposition = autonomousstate.CommitReplayed
		result.AnswerPersisted = true
		existingAnswer = true
		break
	}
	if !existingAnswer {
		answer, err := autonomousinput.RecordAnswer(ctx, autonomousinput.AnswerRequest{
			RepositoryRoot: paths.WorkDir,
			TaskID:         result.TaskID,
			OperationID:    operationID + "-answer",
			Expected:       snapshot.Expected(),
			AnswerID:       answerID,
			Question:       question,
			OptionID:       result.OptionID,
			Provenance:     autonomous.AnswerProvenance{Kind: autonomous.AnswerProvenanceOperator, Actor: request.Operator},
			AnsweredAt:     now,
		})
		if err != nil {
			return fail(err)
		}
		answerSnapshot = answer.State
		result.AnswerDisposition = answer.Disposition
		result.AnswerPersisted = true
	}
	if request.BeforeResume != nil {
		if err := request.BeforeResume(); err != nil {
			return fail(fmt.Errorf("answer autonomous input: answer persisted as %s but resume failed: %w", answerID, err))
		}
	}

	resume, err := autonomousinput.Resume(ctx, autonomousinput.ResumeRequest{
		RepositoryRoot: paths.WorkDir,
		TaskID:         result.TaskID,
		OperationID:    operationID + "-resume",
		Expected:       answerSnapshot.Expected(),
		ResumeID:       resumeID,
		Question:       question,
		AnswerID:       answerID,
		ResumedAt:      now,
	})
	if err != nil {
		return fail(fmt.Errorf("answer autonomous input: answer persisted as %s but resume failed: %w", answerID, err))
	}
	result.ResumeDisposition = resume.Disposition
	result.Resumed = true
	return result, nil
}
