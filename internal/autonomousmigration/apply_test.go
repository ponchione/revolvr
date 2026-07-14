package autonomousmigration

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomousstate"
	"revolvr/internal/taskfile"
)

func TestApplyPublishesStatesBeforeTasksAndReplaysExactOperation(t *testing.T) {
	root := t.TempDir()
	writeMigrationTask(t, root, "b.md", []byte(mixedMigrationTask("beta", "pending", "implement", "")))
	writeMigrationTask(t, root, "a.md", []byte(mixedMigrationTask("alpha", "pending", "implement", "")))
	plan := buildApplyPlan(t, root, []string{"beta", "alpha"})

	result, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || result.Stage != StageCompleted || result.OperationID == "" {
		t.Fatalf("apply result = %+v", result)
	}
	assertInitialMigrationPublication(t, root, plan)

	history, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(migrationsPath), "history"))
	if err != nil || len(history) != 4 {
		t.Fatalf("history entries = %d, %v", len(history), err)
	}
	journal, found, err := Load(root, result.OperationID)
	if err != nil || !found || journal.Stage != StageCompleted || journal.Sequence != 4 {
		t.Fatalf("load completed journal = %+v, found=%v err=%v", journal, found, err)
	}

	replay, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.OperationID != result.OperationID || replay.Stage != StageCompleted {
		t.Fatalf("replay result = %+v", replay)
	}
	assertInitialMigrationPublication(t, root, plan)
}

func TestApplyRecoversAfterEveryDurablePublicationBoundary(t *testing.T) {
	type failure struct {
		point      FailurePoint
		occurrence int
	}
	points := []failure{
		{FailureAfterLock, 1},
		{FailureBeforeMaterial, 1}, {FailureMaterialFileSync, 1}, {FailureMaterialLink, 1}, {FailureMaterialDirectorySync, 1}, {FailureAfterMaterial, 1},
		{FailureBeforeHistory, 1}, {FailureHistoryFileSync, 1}, {FailureHistoryLink, 1}, {FailureHistoryDirectorySync, 1}, {FailureAfterHistory, 1},
		{FailureBeforeCheckpoint, 1}, {FailureCheckpointFileSync, 1}, {FailureCheckpointRename, 1}, {FailureCheckpointDirSync, 1}, {FailureAfterCheckpoint, 1},
		{FailureBeforeState, 1}, {FailureStateFileSync, 1}, {FailureStateLink, 1}, {FailureStateDirectorySync, 1}, {FailureAfterState, 1},
		{FailureBeforeState, 2}, {FailureAfterState, 2},
		{FailureBeforeTask, 1}, {FailureAfterTask, 1}, {FailureBeforeTask, 2}, {FailureAfterTask, 2},
	}
	for _, item := range points {
		name := string(item.point)
		if item.occurrence > 1 {
			name += "_second_item"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeMigrationTask(t, root, "a.md", []byte(mixedMigrationTask("alpha", "pending", "implement", "")))
			writeMigrationTask(t, root, "b.md", []byte(mixedMigrationTask("beta", "pending", "implement", "")))
			plan := buildApplyPlan(t, root, []string{"alpha", "beta"})
			injected := errors.New("crash")
			fired, observedCount := false, 0
			_, err := Apply(context.Background(), ApplyInput{
				RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime(),
				FailureInjector: func(observed FailurePoint) error {
					if observed == item.point {
						observedCount++
					}
					if observed == item.point && observedCount == item.occurrence && !fired {
						fired = true
						return injected
					}
					return nil
				},
			})
			if !fired || !errors.Is(err, injected) {
				t.Fatalf("failure at %s occurrence %d = %v, fired=%v", item.point, item.occurrence, err, fired)
			}
			assertNoAutonomousTaskWithoutState(t, root)

			recovered, authority, found, findErr := FindPlan(context.Background(), root, Request{TaskIDs: []string{"beta", "alpha"}})
			if findErr != nil {
				t.Fatal(findErr)
			}
			createdAt := migrationTime()
			if found {
				plan = recovered
				createdAt = authority.CreatedAt
			}
			result, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: createdAt})
			if err != nil {
				t.Fatalf("recover after %s occurrence %d: %v", item.point, item.occurrence, err)
			}
			if result.Stage != StageCompleted {
				t.Fatalf("recovered stage = %s", result.Stage)
			}
			assertInitialMigrationPublication(t, root, plan)
		})
	}
}

func TestApplyAdoptsExactOrphanStateAndRejectsConflictingEvidence(t *testing.T) {
	t.Run("adopts exact orphan", func(t *testing.T) {
		root := t.TempDir()
		writeMigrationTask(t, root, "candidate.md", []byte(mixedMigrationTask("candidate", "pending", "implement", "")))
		stateRaw, err := autonomousstate.MarshalState(initialState("candidate"))
		if err != nil {
			t.Fatal(err)
		}
		statePath := filepath.Join(root, ".revolvr", "autonomous", "tasks", "candidate", "state.json")
		writeMigrationFile(t, statePath, stateRaw)
		plan, err := Build(root, loadMigrationSchedule(t, root), Request{TaskIDs: []string{"candidate"}, AllowExactOrphanState: true})
		if err != nil {
			t.Fatal(err)
		}
		result, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime()})
		if err != nil || result.Stage != StageCompleted {
			t.Fatalf("adopt orphan = %+v, %v", result, err)
		}
		if got := mustReadMigrationFile(t, statePath); !bytes.Equal(got, stateRaw) {
			t.Fatal("orphan state bytes changed")
		}
	})

	t.Run("rejects extra namespace evidence during planning", func(t *testing.T) {
		root := t.TempDir()
		writeMigrationTask(t, root, "candidate.md", []byte(mixedMigrationTask("candidate", "pending", "implement", "")))
		stateRaw, _ := autonomousstate.MarshalState(initialState("candidate"))
		namespace := filepath.Join(root, ".revolvr", "autonomous", "tasks", "candidate")
		writeMigrationFile(t, filepath.Join(namespace, "state.json"), stateRaw)
		writeMigrationFile(t, filepath.Join(namespace, "user-evidence.txt"), []byte("preserve me\n"))
		_, err := Build(root, loadMigrationSchedule(t, root), Request{TaskIDs: []string{"candidate"}, AllowExactOrphanState: true})
		if err == nil || !strings.Contains(err.Error(), "autonomous_namespace_exists") {
			t.Fatalf("Build error = %v", err)
		}
		if string(mustReadMigrationFile(t, filepath.Join(namespace, "user-evidence.txt"))) != "preserve me\n" {
			t.Fatal("user evidence changed")
		}
	})

	t.Run("never overwrites conflicting recovered state", func(t *testing.T) {
		root := t.TempDir()
		writeMigrationTask(t, root, "candidate.md", []byte(mixedMigrationTask("candidate", "pending", "implement", "")))
		plan := buildApplyPlan(t, root, []string{"candidate"})
		injected := errors.New("stop after state")
		_, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime(), FailureInjector: func(point FailurePoint) error {
			if point == FailureAfterState {
				return injected
			}
			return nil
		}})
		if !errors.Is(err, injected) {
			t.Fatalf("Apply error = %v", err)
		}
		statePath := filepath.Join(root, filepath.FromSlash(plan.Entries[0].AutonomousStatePath))
		conflict := []byte("user-owned conflicting evidence\n")
		if err := os.WriteFile(statePath, conflict, 0o644); err != nil {
			t.Fatal(err)
		}
		recovered, authority, found, err := FindPlan(context.Background(), root, Request{TaskIDs: []string{"candidate"}})
		if err != nil || !found {
			t.Fatalf("FindPlan = found %v, err %v", found, err)
		}
		_, err = Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: recovered, CreatedAt: authority.CreatedAt})
		if err == nil || !strings.Contains(err.Error(), "different bytes") {
			t.Fatalf("conflicting recovery error = %v", err)
		}
		if got := mustReadMigrationFile(t, statePath); !bytes.Equal(got, conflict) {
			t.Fatal("conflicting user state was overwritten")
		}
	})
}

func TestApplyValidatesCompleteBatchBeforeMutation(t *testing.T) {
	root := t.TempDir()
	alphaPath := writeMigrationTask(t, root, "a.md", []byte(mixedMigrationTask("alpha", "pending", "implement", "")))
	betaPath := writeMigrationTask(t, root, "b.md", []byte(mixedMigrationTask("beta", "pending", "implement", "")))
	plan := buildApplyPlan(t, root, []string{"alpha", "beta"})
	changed := append(mustReadMigrationFile(t, betaPath), []byte("changed after plan\n")...)
	if err := os.WriteFile(betaPath, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime()})
	if err == nil || !strings.Contains(err.Error(), "changed after batch planning") {
		t.Fatalf("Apply error = %v", err)
	}
	if got := mustReadMigrationFile(t, alphaPath); !bytes.Equal(got, plan.Entries[0].SourceTask.SourceBytes) {
		t.Fatal("valid task changed after rejected batch")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(migrationsPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected batch created migration authority: %v", err)
	}
}

func TestApplyMigrationLockIsCancellable(t *testing.T) {
	root := t.TempDir()
	writeMigrationTask(t, root, "candidate.md", []byte(mixedMigrationTask("candidate", "pending", "implement", "")))
	plan := buildApplyPlan(t, root, []string{"candidate"})
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := Apply(context.Background(), ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime(), FailureInjector: func(point FailurePoint) error {
			if point == FailureAfterLock {
				close(locked)
				<-release
				return errors.New("release first")
			}
			return nil
		}})
		firstDone <- err
	}()
	<-locked
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := Apply(ctx, ApplyInput{RepositoryRoot: root, Plan: plan, CreatedAt: migrationTime()}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended Apply error = %v", err)
	}
	close(release)
	if err := <-firstDone; err == nil || !strings.Contains(err.Error(), "release first") {
		t.Fatalf("first Apply error = %v", err)
	}
}

func buildApplyPlan(t *testing.T, root string, ids []string) Plan {
	t.Helper()
	plan, err := Build(root, loadMigrationSchedule(t, root), Request{TaskIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func assertInitialMigrationPublication(t *testing.T, root string, plan Plan) {
	t.Helper()
	for _, entry := range plan.Entries {
		task, err := taskfile.Load(root, entry.SourcePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(task.SourceBytes, entry.ProjectedTask.SourceBytes) || task.Workflow != taskfile.WorkflowAutonomousV1 {
			t.Fatalf("task %q publication mismatch", entry.TaskID)
		}
		statePath := filepath.Join(root, filepath.FromSlash(entry.AutonomousStatePath))
		if got := mustReadMigrationFile(t, statePath); !bytes.Equal(got, entry.StateBytes) {
			t.Fatalf("state %q publication mismatch", entry.TaskID)
		}
	}
}

func assertNoAutonomousTaskWithoutState(t *testing.T, root string) {
	t.Helper()
	tasks, err := taskfile.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Workflow != taskfile.WorkflowAutonomousV1 {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(task.AutonomousStatePath))); err != nil {
			t.Fatalf("autonomous task %q points at missing state: %v", task.ID, err)
		}
	}
}

func migrationTime() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) }
