package autonomouschild

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouschildpublication"
	"revolvr/internal/autonomousscheduler"
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
	projection, found, err := autonomouschildpublication.Load(repo, input.OperationID)
	if err != nil || !found || projection.Journal.Stage != StageStatesPublished {
		t.Fatalf("load interrupted publication: projection=%+v found=%t err=%v", projection, found, err)
	}
	stale := projection.Journal
	stale.Stage, stale.Sequence = StageAdmitted, 1
	raw, err := autonomouschildpublication.MarshalJournal(stale)
	if err != nil {
		t.Fatal(err)
	}
	writeChildTestFile(t, childCheckpointPath(repo, input.OperationID), raw)
	input.FailureInjector = nil
	if result, err := Apply(context.Background(), input); err != nil || len(result.Children) != 1 {
		t.Fatalf("recovery = %#v, %v", result, err)
	}
	input.Decision.ChildTasks.Children[0].Scope = "Different bounded work."
	if _, err := Apply(context.Background(), input); err == nil {
		t.Fatal("changed operation replay succeeded")
	}
}

func TestApplyRejectsCorruptCompletedCheckpointAndSubstitutedHistory(t *testing.T) {
	t.Run("mutable completed checkpoint with empty children", func(t *testing.T) {
		repo, parent, stateSHA := childFixture(t)
		input := childInput(repo, parent, stateSHA)
		input.FailureInjector = func(point FailurePoint) error {
			if point == FailureAfterAdmission {
				return errors.New("crash")
			}
			return nil
		}
		if _, err := Apply(context.Background(), input); err == nil {
			t.Fatal("admission crash did not fire")
		}
		projection, found, err := autonomouschildpublication.Load(repo, input.OperationID)
		if err != nil || !found {
			t.Fatalf("load admission: found=%t err=%v", found, err)
		}
		corrupt := projection.Journal
		corrupt.Stage, corrupt.Sequence, corrupt.Children = StageCompleted, 4, nil
		raw, err := json.MarshalIndent(corrupt, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeChildTestFile(t, childCheckpointPath(repo, input.OperationID), append(raw, '\n'))
		input.FailureInjector = nil
		if result, err := Apply(context.Background(), input); err == nil || result.Replayed || !strings.Contains(err.Error(), "child set is empty") {
			t.Fatalf("corrupt completed replay = %+v err=%v", result, err)
		}
		if tasks, err := taskfile.List(repo); err != nil || len(tasks) != 1 || tasks[0].ID != parent.ID {
			t.Fatalf("corrupt replay published children: tasks=%+v err=%v", tasks, err)
		}
	})

	t.Run("structurally valid substituted immutable child set", func(t *testing.T) {
		repo, parent, stateSHA := childFixture(t)
		input := childInput(repo, parent, stateSHA)
		input.FailureInjector = func(point FailurePoint) error {
			if point == FailureAfterAdmission {
				return errors.New("crash")
			}
			return nil
		}
		if _, err := Apply(context.Background(), input); err == nil {
			t.Fatal("admission crash did not fire")
		}
		projection, found, err := autonomouschildpublication.Load(repo, input.OperationID)
		if err != nil || !found {
			t.Fatalf("load admission: found=%t err=%v", found, err)
		}
		wrong := projection.Journal
		wrong.Children = []ChildRecord{substitutedChildRecord(wrong, "other-child")}
		writeChildHistoryChain(t, repo, wrong, 4)
		input.FailureInjector = nil
		if result, err := Apply(context.Background(), input); err == nil || result.Replayed || !strings.Contains(err.Error(), "content conflict") {
			t.Fatalf("substituted history replay = %+v err=%v", result, err)
		}
	})
}

func TestSchedulerUsesSharedValidatedPublicationProjection(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func(*testing.T, string, Input, Journal)
		wantError string
	}{
		{
			name: "missing checkpoint recovers from history",
			mutate: func(t *testing.T, repo string, input Input, _ Journal) {
				if err := os.Remove(childCheckpointPath(repo, input.OperationID)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "behind checkpoint recovers from history",
			mutate: func(t *testing.T, repo string, input Input, journal Journal) {
				journal.Stage, journal.Sequence = StageStatesPublished, 2
				raw, err := autonomouschildpublication.MarshalJournal(journal)
				if err != nil {
					t.Fatal(err)
				}
				writeChildTestFile(t, childCheckpointPath(repo, input.OperationID), raw)
			},
		},
		{
			name: "history gap is rejected",
			mutate: func(t *testing.T, repo string, input Input, _ Journal) {
				if err := os.Remove(childHistoryPath(repo, input.OperationID, 2)); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "noncontiguous",
		},
		{
			name: "substituted child membership is rejected",
			mutate: func(t *testing.T, repo string, _ Input, journal Journal) {
				journal.Children = []ChildRecord{substitutedChildRecord(journal, "other-child")}
				writeChildHistoryChain(t, repo, journal, 4)
			},
			wantError: "absent from the published child set",
		},
		{
			name: "removed immutable state lineage is rejected",
			mutate: func(t *testing.T, repo string, _ Input, journal Journal) {
				store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
				if err != nil {
					t.Fatal(err)
				}
				snapshot, found, err := store.Load(context.Background(), journal.Children[0].TaskID)
				if err != nil || !found {
					t.Fatalf("load child state: found=%t err=%v", found, err)
				}
				snapshot.State.ChildOf = nil
				raw, err := autonomousstate.MarshalState(snapshot.State)
				if err != nil {
					t.Fatal(err)
				}
				writeChildTestFile(t, filepath.Join(repo, filepath.FromSlash(journal.Children[0].StatePath)), raw)
			},
			wantError: "no immutable child lineage",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, parent, stateSHA := childFixture(t)
			input := childInput(repo, parent, stateSHA)
			if _, err := Apply(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			projection, found, err := autonomouschildpublication.Load(repo, input.OperationID)
			if err != nil || !found {
				t.Fatalf("load completed publication: found=%t err=%v", found, err)
			}
			test.mutate(t, repo, input, projection.Journal)
			active, err := autonomousscheduler.LoadActiveStrict(context.Background(), repo)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("scheduler error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || len(active) != 2 {
				t.Fatalf("scheduler active = %+v err=%v", active, err)
			}
		})
	}
}

func TestIncompletePublicationCannotBecomeSchedulingAuthority(t *testing.T) {
	repo, parent, stateSHA := childFixture(t)
	input := childInput(repo, parent, stateSHA)
	input.FailureInjector = func(point FailurePoint) error {
		if point == FailureAfterTasks {
			return errors.New("crash")
		}
		return nil
	}
	if _, err := Apply(context.Background(), input); err == nil {
		t.Fatal("task-publication crash did not fire")
	}
	if _, err := autonomousscheduler.LoadActiveStrict(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "publication is incomplete") {
		t.Fatalf("incomplete scheduler authority error = %v", err)
	}
	projection, found, err := autonomouschildpublication.Load(repo, input.OperationID)
	if err != nil || !found || projection.Journal.Stage != StageTasksPublished {
		t.Fatalf("incomplete projection = %+v found=%t err=%v", projection, found, err)
	}
	if err := os.Remove(filepath.Join(repo, filepath.FromSlash(projection.Journal.Children[0].StatePath))); err != nil {
		t.Fatal(err)
	}
	if _, err := autonomousscheduler.LoadActive(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "no canonical state") {
		t.Fatalf("missing child state scheduler error = %v", err)
	}
	input.FailureInjector = nil
	if result, err := Apply(context.Background(), input); err == nil || result.Replayed {
		t.Fatalf("missing state completed publication = %+v err=%v", result, err)
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

func substitutedChildRecord(journal Journal, proposalKey string) ChildRecord {
	taskID := autonomouschildpublication.ChildTaskID(journal.ParentTaskID, journal.DecisionID, journal.ProposalID, proposalKey)
	return ChildRecord{
		TaskID:      taskID,
		ProposalKey: proposalKey,
		TaskPath:    filepath.ToSlash(filepath.Join(taskfile.TasksDir, taskID+".md")),
		TaskSHA256:  strings.Repeat("a", 64),
		StatePath:   filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json")),
		StateSHA256: strings.Repeat("b", 64),
	}
}

func writeChildHistoryChain(t *testing.T, repo string, authority Journal, through int64) {
	t.Helper()
	var latest Journal
	for sequence := int64(1); sequence <= through; sequence++ {
		journal := authority
		journal.Sequence = sequence
		switch sequence {
		case 1:
			journal.Stage = StageAdmitted
		case 2:
			journal.Stage = StageStatesPublished
		case 3:
			journal.Stage = StageTasksPublished
		case 4:
			journal.Stage = StageCompleted
		}
		raw, err := autonomouschildpublication.MarshalHistory(journal)
		if err != nil {
			t.Fatal(err)
		}
		writeChildTestFile(t, childHistoryPath(repo, journal.OperationID, sequence), raw)
		latest = journal
	}
	raw, err := autonomouschildpublication.MarshalJournal(latest)
	if err != nil {
		t.Fatal(err)
	}
	writeChildTestFile(t, childCheckpointPath(repo, latest.OperationID), raw)
}

func childCheckpointPath(repo, operationID string) string {
	return filepath.Join(repo, ".revolvr", "autonomous", "child-publications", operationID+".json")
}

func childHistoryPath(repo, operationID string, sequence int64) string {
	return filepath.Join(repo, ".revolvr", "autonomous", "child-publications", "history", autonomouschildpublication.HistoryFilename(operationID, sequence))
}

func writeChildTestFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
