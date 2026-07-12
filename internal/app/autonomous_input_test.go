package app

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
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousview"
)

func TestListAutonomousTaskSelectorsIncludesActiveAndArchiveWithoutVerification(t *testing.T) {
	root := t.TempDir()
	writeActiveViewFixture(t, root, "active-task", "Active title", simpleViewState("active-task", autonomous.LifecycleStateReady))
	archiveID := writeCancelledArchiveFixture(t, root, "archive-task")
	before := snapshotTree(t, root)

	selectors, err := ListAutonomousTaskSelectors(context.Background(), Config{WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(selectors) != 2 {
		t.Fatalf("selectors = %#v", selectors)
	}
	if selectors[0].Selector != "active-task" || selectors[0].SourceKind != autonomousview.SourceActive || selectors[0].Title != "Active title" {
		t.Fatalf("active selector = %#v", selectors[0])
	}
	if selectors[1].Selector != archiveID || selectors[1].TaskID != "archive-task" || selectors[1].SourceKind != autonomousview.SourceArchive || selectors[1].Disposition != "cancelled" {
		t.Fatalf("archive selector = %#v", selectors[1])
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("selector read mutated repository\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestListAutonomousTaskSelectorsRedactsDisplayLabel(t *testing.T) {
	root := t.TempDir()
	secret := "selector-super-secret"
	t.Setenv("REVOLVR_SELECTOR_SECRET", secret)
	writeActiveViewFixture(t, root, "secret-task", "Title "+secret, simpleViewState("secret-task", autonomous.LifecycleStateReady))
	if err := os.WriteFile(filepath.Join(root, ".revolvr", "config.yaml"), []byte("autonomy:\n  redaction:\n    environment_variables: [REVOLVR_SELECTOR_SECRET]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selectors, err := ListAutonomousTaskSelectors(context.Background(), Config{WorkDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(selectors) != 1 || strings.Contains(selectors[0].Label, secret) || !strings.Contains(selectors[0].Label, "[REDACTED]") {
		t.Fatalf("selectors = %#v", selectors)
	}
}

func TestAnswerAutonomousInputPersistsAnswerThenResumesExactQuestion(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC)
	question, reference := inputQuestionFixture(t, "input-task", now)
	state := simpleViewState("input-task", autonomous.LifecycleStateNeedsInput)
	record := autonomous.InputQuestionRecord{Sequence: 1, TaskID: "input-task", Question: question, Decision: reference, SourceRevision: hash64("a"), ResumeLifecycle: autonomous.LifecycleStateReady, RecordedAt: now}
	identity := question.Identity()
	state.Input = autonomous.InputState{TransitionSequence: 1, Questions: []autonomous.InputQuestionRecord{record}}
	state.NeedsInput = &autonomous.NeedsInputDetail{CurrentQuestion: &identity}
	state.LatestDecision = &reference
	writeActiveViewFixture(t, root, "input-task", "Input", state)
	if stale, staleErr := AnswerAutonomousInput(context.Background(), Config{WorkDir: root}, AnswerAutonomousInputRequest{TaskID: "input-task", QuestionID: identity.QuestionID, Revision: identity.Revision + 1, ContentSHA: identity.ContentSHA256, OptionID: "keep", Operator: "operator"}); staleErr == nil || stale.AnswerPersisted {
		t.Fatalf("stale result=%#v err=%v", stale, staleErr)
	}

	result, err := AnswerAutonomousInput(context.Background(), Config{WorkDir: root}, AnswerAutonomousInputRequest{
		TaskID:      "input-task",
		QuestionID:  identity.QuestionID,
		Revision:    identity.Revision,
		ContentSHA:  identity.ContentSHA256,
		OptionID:    "keep",
		Operator:    "tui-operator",
		OperationID: "answer-operation",
		AnswerID:    "answer-one",
		ResumeID:    "resume-one",
		Clock:       func() time.Time { return now.Add(time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AnswerPersisted || !result.Resumed || result.OptionID != "keep" {
		t.Fatalf("result = %#v", result)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), "input-task")
	if err != nil || !found {
		t.Fatalf("load state: found=%t err=%v", found, err)
	}
	if snapshot.State.Lifecycle != autonomous.LifecycleStateReady || len(snapshot.State.Input.Answers) != 1 || len(snapshot.State.Input.Resumes) != 1 || snapshot.State.Input.Answers[0].OptionID != "keep" {
		t.Fatalf("state = %#v", snapshot.State)
	}
}

func TestAnswerAutonomousInputReturnsPersistedEvidenceWhenResumeFails(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC)
	question, reference := inputQuestionFixture(t, "resume-failure", now)
	state := simpleViewState("resume-failure", autonomous.LifecycleStateNeedsInput)
	record := autonomous.InputQuestionRecord{Sequence: 1, TaskID: "resume-failure", Question: question, Decision: reference, SourceRevision: hash64("b"), ResumeLifecycle: autonomous.LifecycleStateReady, RecordedAt: now}
	identity := question.Identity()
	state.Input = autonomous.InputState{TransitionSequence: 1, Questions: []autonomous.InputQuestionRecord{record}}
	state.NeedsInput = &autonomous.NeedsInputDetail{CurrentQuestion: &identity}
	state.LatestDecision = &reference
	writeActiveViewFixture(t, root, "resume-failure", "Resume failure", state)

	result, err := AnswerAutonomousInput(context.Background(), Config{WorkDir: root}, AnswerAutonomousInputRequest{
		TaskID: "resume-failure", QuestionID: identity.QuestionID, Revision: identity.Revision, ContentSHA: identity.ContentSHA256,
		OptionID: "keep", Operator: "operator", OperationID: "resume-failure-operation", AnswerID: "answer-one", ResumeID: "resume-one",
		Clock: func() time.Time { return now.Add(time.Minute) }, BeforeResume: func() error { return errors.New("injected resume failure") },
	})
	if err == nil || !result.AnswerPersisted || result.Resumed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	store, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	snapshot, found, loadErr := store.Load(context.Background(), "resume-failure")
	if loadErr != nil || !found || len(snapshot.State.Input.Answers) != 1 || len(snapshot.State.Input.Resumes) != 0 || snapshot.State.Lifecycle != autonomous.LifecycleStateNeedsInput {
		t.Fatalf("persisted state=%#v found=%t err=%v", snapshot.State, found, loadErr)
	}
	recovered, recoverErr := AnswerAutonomousInput(context.Background(), Config{WorkDir: root}, AnswerAutonomousInputRequest{
		TaskID: "resume-failure", QuestionID: identity.QuestionID, Revision: identity.Revision, ContentSHA: identity.ContentSHA256,
		OptionID: "keep", Operator: "operator", OperationID: "resume-failure-operation", ResumeID: "resume-one",
		Clock: func() time.Time { return now.Add(2 * time.Minute) },
	})
	if recoverErr != nil || !recovered.AnswerPersisted || !recovered.Resumed || recovered.AnswerID != "answer-one" {
		t.Fatalf("recovery=%#v err=%v", recovered, recoverErr)
	}
}

func inputQuestionFixture(t *testing.T, taskID string, now time.Time) (autonomous.NeedsInputQuestion, autonomous.DecisionReference) {
	t.Helper()
	question := autonomous.NeedsInputQuestion{
		TaskID: taskID, QuestionID: "deployment-mode", Revision: 1,
		Question: "Which deployment mode should be used?", BlockingReason: "The task does not choose one.",
		Options:        []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep the current behavior."}, {ID: "change", Meaning: "Change the public behavior."}},
		Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "It preserves compatibility."},
		Evidence:       []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: ".agent/tasks/" + taskID + ".md", Detail: "Canonical task."}},
	}
	hash, err := autonomous.QuestionContentSHA256(question)
	if err != nil {
		t.Fatal(err)
	}
	question.ContentSHA256 = hash
	reference := autonomous.DecisionReference{DecisionID: "decision-input", RunID: "supervisor-input", TaskID: taskID, Action: autonomous.ActionNeedsInput, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: ".revolvr/runs/supervisor-input/supervisor-decision.json", Detail: "Accepted decision."}, CreatedAt: now}
	return question, reference
}

func hash64(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
