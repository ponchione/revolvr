package autonomousstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/taskfile"
)

func TestLoadMissingValidMalformedWrongTaskAndUnknownFields(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	store := openStateTestStore(t, repo, nil)
	before := directoryTree(t, repo)
	missing, found, err := store.Load(context.Background(), "task-1")
	if err != nil || found || missing.SourcePath != canonicalStatePath("task-1") {
		t.Fatalf("missing Load() = %+v, %t, %v", missing, found, err)
	}
	if after := directoryTree(t, repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("missing load wrote files: before=%v after=%v", before, after)
	}

	request := stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one")
	committed, err := store.CommitPlanning(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	reopened := openStateTestStore(t, repo, nil)
	snapshot, found, err := reopened.Load(context.Background(), "task-1")
	if err != nil || !found || snapshot.SHA256 != committed.Current.SHA256 || !reflect.DeepEqual(snapshot.State, request.NextState) {
		t.Fatalf("reopened Load() = %+v, %t, %v", snapshot, found, err)
	}

	statePath := filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1")))
	tests := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "malformed", raw: []byte(`{"schema_version":`), want: "decode autonomous state"},
		{name: "unknown field", raw: appendUnknownStateField(t, request.NextState), want: "unknown field"},
		{name: "wrong task", raw: canonicalStateForTask(t, request.NextState, "task-2"), want: "does not match requested"},
		{name: "noncanonical", raw: mustCompactJSON(t, request.NextState), want: "not canonical"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(statePath, tt.raw, 0o644); err != nil {
				t.Fatal(err)
			}
			_, _, err := reopened.Load(context.Background(), "task-1")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsMissingTaskAndSymlinkedNamespace(t *testing.T) {
	repo := t.TempDir()
	store := openStateTestStore(t, repo, nil)
	if _, _, err := store.Load(context.Background(), "missing"); !errors.Is(err, ErrTaskMissing) {
		t.Fatalf("missing task error = %v", err)
	}

	repo, _ = stateTestRepository(t, "task-1")
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".revolvr", "autonomous")); err != nil {
		t.Fatal(err)
	}
	store = openStateTestStore(t, repo, nil)
	if _, _, err := store.Load(context.Background(), "task-1"); err == nil || (!errors.Is(err, ErrUnsafePath) && !strings.Contains(err.Error(), "path escapes root")) {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestCommitPlanningCreatesDeterministicStateHistoryAndReopens(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	store := openStateTestStore(t, repo, nil)
	request := stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one")
	beforePrevious := mustStateJSON(t, request.PreviousState)
	beforeNext := mustStateJSON(t, request.NextState)

	result, err := store.CommitPlanning(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitCreated || result.Current.SourcePath != canonicalStatePath("task-1") || result.History.Record.OperationID != "operation-one" {
		t.Fatalf("commit result = %+v", result)
	}
	wantState, err := MarshalState(request.NextState)
	if err != nil {
		t.Fatal(err)
	}
	gotState, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(result.Current.SourcePath)))
	if err != nil || !reflect.DeepEqual(gotState, wantState) || hashBytes(gotState) != result.Current.SHA256 {
		t.Fatalf("state bytes/hash mismatch error=%v\ngot=%s\nwant=%s", err, gotState, wantState)
	}
	historyRaw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(result.History.SourcePath)))
	if err != nil || hashBytes(historyRaw) != result.History.SHA256 || len(historyRaw) != result.History.ByteSize {
		t.Fatalf("history identity mismatch: error=%v", err)
	}
	if _, err := DecodePlanningHistory(historyRaw); err != nil {
		t.Fatalf("history reopen: %v", err)
	}
	if got := mustStateJSON(t, request.PreviousState); !reflect.DeepEqual(got, beforePrevious) {
		t.Fatal("caller previous state mutated")
	}
	if got := mustStateJSON(t, request.NextState); !reflect.DeepEqual(got, beforeNext) {
		t.Fatal("caller next state mutated")
	}
}

func TestCommitPlanningCASConcurrencyIdempotencyAndConflict(t *testing.T) {
	t.Run("missing and stale expected state", func(t *testing.T) {
		repo, taskRaw := stateTestRepository(t, "task-1")
		store := openStateTestStore(t, repo, nil)
		missing := stateTestRequest(t, repo, taskRaw, "operation-missing", "plan-missing")
		missing.Expected = ExpectedState{Exists: true, SHA256: strings.Repeat("a", 64), ByteSize: 10}
		missing.History.PreviousState.Persisted = true
		if _, err := store.CommitPlanning(context.Background(), missing); !errors.Is(err, ErrStateMissing) {
			t.Fatalf("missing expected error = %v", err)
		}

		initial := stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one")
		if _, err := store.CommitPlanning(context.Background(), initial); err != nil {
			t.Fatal(err)
		}
		stale := stateTestRequest(t, repo, taskRaw, "operation-stale", "plan-stale")
		stale.Expected = ExpectedState{Exists: true, SHA256: strings.Repeat("f", 64), ByteSize: 99}
		stale.History.PreviousState.Persisted = true
		if _, err := store.CommitPlanning(context.Background(), stale); !errors.Is(err, ErrStaleWrite) {
			t.Fatalf("stale expected error = %v", err)
		}
	})

	t.Run("exactly one concurrent writer", func(t *testing.T) {
		repo, taskRaw := stateTestRepository(t, "task-1")
		storeOne := openStateTestStore(t, repo, nil)
		storeTwo := openStateTestStore(t, repo, nil)
		requests := []CommitRequest{
			stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one"),
			stateTestRequest(t, repo, taskRaw, "operation-two", "plan-two"),
		}
		stores := []*Store{storeOne, storeTwo}
		type outcome struct {
			result CommitResult
			err    error
		}
		outcomes := make([]outcome, 2)
		var wg sync.WaitGroup
		for i := range stores {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				outcomes[index].result, outcomes[index].err = stores[index].CommitPlanning(context.Background(), requests[index])
			}(i)
		}
		wg.Wait()
		successes := 0
		stale := 0
		for _, outcome := range outcomes {
			if outcome.err == nil {
				successes++
			} else if errors.Is(outcome.err, ErrStateExists) || errors.Is(outcome.err, ErrStaleWrite) {
				stale++
			}
		}
		if successes != 1 || stale != 1 {
			t.Fatalf("concurrent outcomes = %+v, want one success and one stale", outcomes)
		}
	})

	t.Run("same operation replay and conflict", func(t *testing.T) {
		repo, taskRaw := stateTestRepository(t, "task-1")
		store := openStateTestStore(t, repo, nil)
		request := stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one")
		first, err := store.CommitPlanning(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := store.CommitPlanning(context.Background(), request)
		if err != nil || replay.Disposition != CommitReplayed || replay.History.SourcePath != first.History.SourcePath {
			t.Fatalf("replay = %+v, %v", replay, err)
		}
		historyDir := filepath.Dir(filepath.Join(repo, filepath.FromSlash(first.History.SourcePath)))
		entries, err := os.ReadDir(historyDir)
		if err != nil || len(entries) != 1 {
			t.Fatalf("history entries = %v, error=%v", entries, err)
		}
		conflict := request
		conflict.History.ApplicationSHA256 = strings.Repeat("f", 64)
		if _, err := store.CommitPlanning(context.Background(), conflict); !errors.Is(err, ErrOperationConflict) {
			t.Fatalf("operation conflict error = %v", err)
		}
	})
}

func TestCommitPlanningCrashPointsLeaveRecoverableAuthoritativeState(t *testing.T) {
	points := []FailurePoint{
		FailureBeforeHistoryCreate,
		FailureDuringHistoryWrite,
		FailureHistoryFileSync,
		FailureHistoryDirectorySync,
		FailureAfterHistoryWrite,
		FailureDuringStateWrite,
		FailureStateFileSync,
		FailureBeforeStateRename,
		FailureStateRename,
		FailureAfterStateRename,
		FailureStateDirectorySync,
		FailureStateReadback,
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			repo, taskRaw := stateTestRepository(t, "task-1")
			fired := false
			store := openStateTestStore(t, repo, func(got FailurePoint) error {
				if !fired && got == point {
					fired = true
					return errors.New("crash")
				}
				return nil
			})
			request := stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one")
			if _, err := store.CommitPlanning(context.Background(), request); err == nil || !fired {
				t.Fatalf("failure result error=%v fired=%t", err, fired)
			}
			reopened := openStateTestStore(t, repo, nil)
			snapshot, found, loadErr := reopened.Load(context.Background(), "task-1")
			if loadErr != nil {
				t.Fatalf("load after injected failure: %v", loadErr)
			}
			committedPoint := point == FailureAfterStateRename || point == FailureStateDirectorySync || point == FailureStateReadback
			if found != committedPoint {
				t.Fatalf("state found=%t at %s, want %t (snapshot=%+v)", found, point, committedPoint, snapshot)
			}
			if found {
				history, historyFound, err := reopened.LoadPlanningOperation(context.Background(), "task-1", "operation-one")
				if err != nil || !historyFound || history.Record.ResultingState.SHA256 != snapshot.SHA256 {
					t.Fatalf("committed state lacks history: %+v, %t, %v", history, historyFound, err)
				}
			}
			retry, err := reopened.CommitPlanning(context.Background(), request)
			if err != nil {
				t.Fatalf("retry after %s: %v", point, err)
			}
			if committedPoint && retry.Disposition != CommitReplayed {
				t.Fatalf("retry disposition = %q, want replay", retry.Disposition)
			}
			stateDir := filepath.Join(repo, ".revolvr", "autonomous", "tasks", "task-1")
			entries, err := os.ReadDir(stateDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.Contains(entry.Name(), ".tmp-") {
					t.Fatalf("temporary file survived: %s", entry.Name())
				}
			}
		})
	}
}

func TestReplaceStateFaultBoundariesAndCASRecheck(t *testing.T) {
	points := []FailurePoint{
		FailureDuringStateWrite,
		FailureStateFileSync,
		FailureBeforeStateRename,
		FailureStateRename,
		FailureAfterStateRename,
		FailureStateDirectorySync,
		FailureStateReadback,
	}
	for _, point := range points {
		t.Run(string(point), func(t *testing.T) {
			repo, taskRaw := stateTestRepository(t, "task-1")
			fired := false
			store := openStateTestStore(t, repo, func(got FailurePoint) error {
				if !fired && got == point {
					fired = true
					return errors.New("crash")
				}
				return nil
			})
			task, previous, nextRaw := prepareReplaceStateTest(t, store, repo, taskRaw)
			if _, _, err := store.replaceState(task, previous.Expected(), nextRaw, nil); err == nil || !fired {
				t.Fatalf("replaceState() error=%v fired=%t", err, fired)
			}

			committedPoint := point == FailureAfterStateRename || point == FailureStateDirectorySync || point == FailureStateReadback
			reopened := openStateTestStore(t, repo, nil)
			current, found, err := reopened.readCurrent(task)
			if err != nil || !found {
				t.Fatalf("readCurrent() = %+v, %t, %v", current, found, err)
			}
			if gotCommitted := current.SHA256 == hashBytes(nextRaw); gotCommitted != committedPoint {
				t.Fatalf("committed=%t at %s, want %t", gotCommitted, point, committedPoint)
			}
			assertNoStateTemporaryFiles(t, repo)

			readback, found, err := reopened.replaceState(task, current.Expected(), nextRaw, nil)
			if err != nil || !found || readback.SHA256 != hashBytes(nextRaw) || readback.ByteSize != len(nextRaw) {
				t.Fatalf("retry replaceState() = %+v, %t, %v", readback, found, err)
			}
		})
	}

	t.Run("locked CAS recheck", func(t *testing.T) {
		repo, taskRaw := stateTestRepository(t, "task-1")
		store := openStateTestStore(t, repo, nil)
		task, previous, nextRaw := prepareReplaceStateTest(t, store, repo, taskRaw)
		statePath := filepath.Join(repo, filepath.FromSlash(task.AutonomousStatePath))
		mutated := false
		store.inject = func(point FailurePoint) error {
			if point == FailureStateFileSync && !mutated {
				mutated = true
				if err := os.WriteFile(statePath, nextRaw, 0o644); err != nil {
					return err
				}
			}
			return nil
		}
		if _, _, err := store.replaceState(task, previous.Expected(), nextRaw, nil); !errors.Is(err, ErrStaleWrite) || !mutated {
			t.Fatalf("replaceState() error=%v mutated=%t", err, mutated)
		}
		current, found, err := store.readCurrent(task)
		if err != nil || !found || current.SHA256 != hashBytes(nextRaw) {
			t.Fatalf("readCurrent() = %+v, %t, %v", current, found, err)
		}
		assertNoStateTemporaryFiles(t, repo)
	})
}

func TestReplaceStateRejectsAncestorSubstitutionBeforeOutsidePublication(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	outside := t.TempDir()
	store := openStateTestStore(t, repo, nil)
	task, previous, nextRaw := prepareReplaceStateTest(t, store, repo, taskRaw)
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	lease, err := store.acquireLock(context.Background(), filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()

	stateDir := filepath.Join(repo, filepath.FromSlash(namespace))
	movedDir := stateDir + ".moved"
	mustStateTestWrite(t, filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600)
	var outsideBefore []string
	store.inject = func(point FailurePoint) error {
		if point != FailureBeforeStateRename {
			return nil
		}
		entries, err := os.ReadDir(stateDir)
		if err != nil {
			return err
		}
		tempName := ""
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".state.json.tmp-") {
				tempName = entry.Name()
				break
			}
		}
		if tempName == "" {
			return errors.New("state temporary file was not found")
		}
		if err := os.Rename(stateDir, movedDir); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outside, tempName), []byte("attacker temporary\n"), 0o600); err != nil {
			return err
		}
		if err := os.Symlink(outside, stateDir); err != nil {
			return err
		}
		outsideBefore = stateTestTreeSnapshot(t, outside)
		return nil
	}

	if _, _, err := store.replaceState(task, previous.Expected(), nextRaw, lease); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("replaceState() error = %v, want ErrUnsafePath", err)
	}
	if outsideBefore == nil {
		t.Fatal("substitution hook did not run")
	}
	if after := stateTestTreeSnapshot(t, outside); !reflect.DeepEqual(after, outsideBefore) {
		t.Fatalf("outside tree changed\nbefore: %v\nafter:  %v", outsideBefore, after)
	}
	if _, err := os.Lstat(filepath.Join(outside, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside state.json exists or inspect failed: %v", err)
	}
}

func prepareReplaceStateTest(t *testing.T, store *Store, repo string, taskRaw []byte) (taskfile.Task, Snapshot, []byte) {
	t.Helper()
	task, err := store.canonicalTask("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDirectory(filepath.ToSlash(filepath.Dir(task.AutonomousStatePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	previousRaw, err := MarshalState(stateTestPendingState("task-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(task.AutonomousStatePath)), previousRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	previous, found, err := store.readCurrent(task)
	if err != nil || !found {
		t.Fatalf("readCurrent() = %+v, %t, %v", previous, found, err)
	}
	nextRaw, err := MarshalState(stateTestRequest(t, repo, taskRaw, "operation-one", "plan-one").NextState)
	if err != nil {
		t.Fatal(err)
	}
	return task, previous, nextRaw
}

func assertNoStateTemporaryFiles(t *testing.T, repo string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repo, ".revolvr", "autonomous", "tasks", "task-1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temporary file survived: %s", entry.Name())
		}
	}
}

func stateTestRequest(t *testing.T, repo string, taskRaw []byte, operationID, planID string) CommitRequest {
	t.Helper()
	previous := stateTestPendingState("task-1")
	decisionArtifact := stateTestEvidence(autonomous.EvidenceKindFile, ".revolvr/runs/supervisor-run/supervisor-decision.json")
	decision := autonomous.DecisionReference{
		DecisionID: "decision-one", RunID: "supervisor-run", TaskID: "task-1",
		Action: autonomous.ActionPlan, WorkerProfile: autonomous.WorkerProfilePlanner,
		Artifact: decisionArtifact, CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	}
	taskOrigin := stateTestEvidence(autonomous.EvidenceKindTask, ".agent/tasks/task-1.md#sha256="+hashBytes(taskRaw))
	plan := autonomous.TaskPlan{
		TaskID: "task-1", ID: planID, Revision: 1,
		Provenance: []autonomous.EvidenceReference{taskOrigin, decisionArtifact},
		Steps:      []autonomous.PlanStep{{ID: "step-one", Description: "Implement behavior.", Status: autonomous.PlanStepStatusPending}},
	}
	source := taskOrigin
	criterion := autonomous.AcceptanceCriterion{ID: "criterion-one", Requirement: "Behavior works.", Status: autonomous.AcceptanceStatusPending, Source: &source}
	next := previous
	next.Lifecycle = autonomous.LifecycleStateReady
	next.Plan = &plan
	next.AcceptanceCriteria = []autonomous.AcceptanceCriterion{criterion}
	next.LatestDecision = &decision
	previousRaw, _ := MarshalState(previous)
	nextRaw, _ := MarshalState(next)
	canonical := []byte("{\n  \"fixture\": true\n}\n")
	canonicalPath := filepath.ToSlash(filepath.Join(".revolvr", "runs", "worker-"+operationID, "planner-output.canonical.json"))
	previousStateIdentity := StateIdentity{Path: canonicalStatePath("task-1"), Persisted: false, SHA256: hashBytes(previousRaw), ByteSize: len(previousRaw)}
	resultStateIdentity := StateIdentity{Path: canonicalStatePath("task-1"), Persisted: true, SHA256: hashBytes(nextRaw), ByteSize: len(nextRaw)}
	record := PlanningHistoryRecord{
		SchemaVersion: PlanningHistorySchemaVersion, TaskID: "task-1", OperationID: operationID,
		ApplicationSHA256: hashBytes([]byte(operationID + ":application")), Change: PlanningChangeCreated,
		CreatedAt:          time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
		Decision:           decision,
		SupervisorDecision: ArtifactIdentity{Path: decision.Artifact.Reference, SHA256: strings.Repeat("1", 64), ByteSize: 10},
		WorkerRunID:        "worker-" + operationID,
		Profile:            autonomousplanning.ProfileIdentity{Name: autonomous.WorkerProfilePlanner, Path: ".agent/profiles/planner.md", SHA256: strings.Repeat("2", 64), ByteSize: 10},
		Dossier:            autonomousplanning.DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", SHA256: strings.Repeat("3", 64), ByteSize: 100},
		SourceRevision:     strings.Repeat("4", 64),
		TaskSource:         ArtifactIdentity{Path: ".agent/tasks/task-1.md", SHA256: hashBytes(taskRaw), ByteSize: len(taskRaw)},
		RawOutput:          ArtifactIdentity{Path: filepath.ToSlash(filepath.Join(".revolvr", "runs", "worker-"+operationID, "planner-output.raw.json")), SHA256: strings.Repeat("5", 64), ByteSize: 20},
		CanonicalOutput:    ArtifactIdentity{Path: canonicalPath, SHA256: hashBytes(canonical), ByteSize: len(canonical)},
		PreviousState:      previousStateIdentity, ResultingState: resultStateIdentity,
		ResultingPlan: plan, ResultingAcceptance: []autonomous.AcceptanceCriterion{criterion},
		ResultingPlanIdentity: PlanIdentity{ID: plan.ID, Revision: plan.Revision},
		Acceptance:            CountAcceptance([]autonomous.AcceptanceCriterion{criterion}),
	}
	return CommitRequest{TaskID: "task-1", Expected: ExpectedState{}, PreviousState: previous, NextState: next, History: record, CanonicalOutput: canonical}
}

func stateTestPendingState(taskID string) autonomous.ExecutionState {
	return autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStatePending,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
}

func stateTestRepository(t *testing.T, taskID string) (string, []byte) {
	t.Helper()
	repo := t.TempDir()
	raw := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\n---\n# Task\n\nBehavior works.\n", taskID, taskID))
	path := filepath.Join(repo, ".agent", "tasks", taskID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, raw
}

func openStateTestStore(t *testing.T, repo string, inject FailureInjector) *Store {
	t.Helper()
	store, err := New(Config{RepositoryRoot: repo, FailureInjector: inject})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func stateTestEvidence(kind autonomous.EvidenceKind, reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: "Exact durable test evidence."}
}

func appendUnknownStateField(t *testing.T, state autonomous.ExecutionState) []byte {
	t.Helper()
	raw, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	return bytesReplaceLast(raw, []byte("\n}\n"), []byte(",\n  \"unknown\": true\n}\n"))
}

func canonicalStateForTask(t *testing.T, state autonomous.ExecutionState, taskID string) []byte {
	t.Helper()
	clonedRaw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var cloned autonomous.ExecutionState
	if err := json.Unmarshal(clonedRaw, &cloned); err != nil {
		t.Fatal(err)
	}
	state = cloned
	state.TaskID = taskID
	state.Plan.TaskID = taskID
	state.LatestDecision.TaskID = taskID
	raw, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustCompactJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func mustStateJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func bytesReplaceLast(raw, old, replacement []byte) []byte {
	index := strings.LastIndex(string(raw), string(old))
	if index < 0 {
		return raw
	}
	result := append([]byte(nil), raw[:index]...)
	result = append(result, replacement...)
	result = append(result, raw[index+len(old):]...)
	return result
}

func directoryTree(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		result = append(result, rel)
		return nil
	})
	return result
}

func mustStateTestWrite(t *testing.T, path string, raw []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatal(err)
	}
}

func stateTestTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var result []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		nlink := uint64(0)
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			nlink = uint64(stat.Nlink)
		}
		value := fmt.Sprintf("%s|%s|%04o|%d", filepath.ToSlash(rel), info.Mode().Type(), info.Mode().Perm(), nlink)
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += "|" + string(raw)
		}
		result = append(result, value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
