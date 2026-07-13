package autonomouschild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
)

func TestApplyPublishesExactChildrenWithoutMutatingParentAndReplays(t *testing.T) {
	repo, parent, stateSHA := childFixture(t)
	input := childInput(repo, parent, stateSHA)
	ledgerStore, err := ledger.OpenWithClock(context.Background(), filepath.Join(repo, ".revolvr", "ledger.db"), func() time.Time { return input.CreatedAt })
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	if _, err := ledgerStore.CreateRun(context.Background(), ledger.RunSpec{ID: input.Reference.RunID, TaskID: parent.ID, Task: "supervisor", StartedAt: input.CreatedAt}); err != nil {
		t.Fatal(err)
	}
	input.Ledger = ledgerStore
	parentBefore := append([]byte(nil), parent.SourceBytes...)
	first, err := Apply(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Children) != 1 || first.Children[0].ParentTaskID != parent.ID || first.Children[0].ParentBehavior != taskfile.ParentBehaviorIndependent {
		t.Fatalf("result = %#v", first)
	}
	replay, err := Apply(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Children[0].ID != first.Children[0].ID {
		t.Fatalf("replay = %#v", replay)
	}
	current, _, err := taskfile.FindByID(repo, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(current.SourceBytes) != string(parentBefore) {
		t.Fatal("parent task bytes changed")
	}
	history, found, err := ledgerStore.GetRunWithEvents(context.Background(), input.Reference.RunID)
	if err != nil || !found {
		t.Fatalf("ledger: %v %v", found, err)
	}
	counts := map[ledger.EventType]int{}
	for _, event := range history.Events {
		counts[event.Type]++
	}
	for _, kind := range []ledger.EventType{ledger.EventChildProposalAdmitted, ledger.EventChildrenPublished, ledger.EventChildPublicationCompleted} {
		if counts[kind] != 1 {
			t.Fatalf("event %s count=%d", kind, counts[kind])
		}
	}
	store, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	childState, found, err := store.Load(context.Background(), first.Children[0].ID)
	if err != nil || !found || childState.State.ChildOf == nil || childState.State.ChildOf.DecisionID != input.Reference.DecisionID {
		t.Fatalf("child state = %#v found=%v err=%v", childState.State, found, err)
	}
}

func TestApplyRecoversAfterStatePublicationAndRejectsChangedOperation(t *testing.T) {
	repo, parent, stateSHA := childFixture(t)
	input := childInput(repo, parent, stateSHA)
	input.FailureInjector = func(point FailurePoint) error {
		if point == FailureAfterStates {
			return errors.New("crash")
		}
		return nil
	}
	if _, err := Apply(context.Background(), input); err == nil {
		t.Fatal("apply succeeded, want injected crash")
	}
	input.FailureInjector = nil
	if result, err := Apply(context.Background(), input); err != nil || len(result.Children) != 1 {
		t.Fatalf("recovery = %#v, %v", result, err)
	}
	input.Decision.ChildTasks.Children[0].Scope = "Different bounded work."
	if _, err := Apply(context.Background(), input); err == nil {
		t.Fatal("changed operation replay succeeded")
	}
}

func TestApplyRejectsConfiguredSecretBeforePersistentPublication(t *testing.T) {
	repo, parent, stateSHA := childFixture(t)
	input := childInput(repo, parent, stateSHA)
	secret := "top-secret-value"
	input.Decision.ChildTasks.Children[0].Scope = "Use exact evidence " + secret
	input.ForbiddenValues = []string{secret}
	if _, err := Apply(context.Background(), input); err == nil || !strings.Contains(err.Error(), "configured secret") {
		t.Fatalf("error=%v", err)
	}
	entries, err := taskfile.List(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("tasks=%d, want parent only", len(entries))
	}
}

func TestApplyWaitsAtPublicationAdmissionBeforeAnyMutation(t *testing.T) {
	repo, parent, stateSHA := childFixture(t)
	input := childInput(repo, parent, stateSHA)
	release, err := lock(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, err = Apply(ctx, input)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		release()
		t.Fatalf("publication admission error = %v, want lock wait cancellation", err)
	}
	journalPath := filepath.Join(repo, ".revolvr", "autonomous", "child-publications", input.OperationID+".json")
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		release()
		t.Fatalf("blocked publication created journal: %v", err)
	}
	if tasks, err := taskfile.List(repo); err != nil || len(tasks) != 1 || tasks[0].ID != parent.ID {
		release()
		t.Fatalf("blocked publication changed tasks: tasks=%+v err=%v", tasks, err)
	}
	release()
	if result, err := Apply(context.Background(), input); err != nil || len(result.Children) != 1 {
		t.Fatalf("publication after admission release = %+v, %v", result, err)
	}
}

func childFixture(t *testing.T) (string, taskfile.Task, string) {
	t.Helper()
	repo := t.TempDir()
	parent, err := taskfile.ProjectAutonomousTask(repo, taskfile.AutonomousCreateInput{ID: "parent", Title: "Parent", Body: "Parent scope."})
	if err != nil {
		t.Fatal(err)
	}
	parent, err = taskfile.PublishAutonomousTask(repo, parent)
	if err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "parent", Lifecycle: autonomous.LifecycleStatePending, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, filepath.FromSlash(parent.AutonomousStatePath))
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	store, _ := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	snap, found, err := store.Load(context.Background(), "parent")
	if err != nil || !found {
		t.Fatalf("load parent: %v %v", found, err)
	}
	return repo, parent, snap.SHA256
}

func childInput(repo string, parent taskfile.Task, stateSHA string) Input {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	evidence := autonomous.EvidenceReference{Kind: autonomous.EvidenceKindTask, Reference: parent.SourcePath, Detail: "Exact parent scope."}
	decision := autonomous.SupervisorDecision{TaskID: parent.ID, Action: autonomous.ActionBlock, Rationale: "Publish separable bounded work before blocking.", Inputs: []autonomous.EvidenceReference{evidence}, ChildTasks: &autonomous.ChildTaskProposalSet{ParentTaskID: parent.ID, ProposalID: "proposal-one", Children: []autonomous.ChildTaskProposal{{Key: "separable", Title: "Separable child", Scope: "Perform only the cited separable work.", SuccessCriteria: []string{"The bounded work is complete."}, Tags: []string{"small"}, ParentBehavior: autonomous.ChildIndependent, Evidence: []autonomous.EvidenceReference{evidence}}}}}
	reference := autonomous.DecisionReference{DecisionID: "decision-one", RunID: "run-one", TaskID: parent.ID, Action: autonomous.ActionBlock, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: "decision.json", Detail: "Validated decision."}, CreatedAt: now}
	return Input{RepositoryRoot: repo, OperationID: "publish-one", Decision: decision, Reference: reference, ExpectedParentTaskSHA256: parent.SourceSHA256(), ExpectedParentStateSHA256: stateSHA, CreatedAt: now}
}
