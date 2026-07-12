package autonomousblock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/taskfile"
)

func TestApplyPersistsExactBlockAndReplays(t *testing.T) {
	repo, store, snapshot := blockFixture(t)
	cfg := blockConfig(repo, store, snapshot)
	first, err := Apply(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Disposition != autonomousstate.CommitUpdated || first.Current.State.Lifecycle != autonomous.LifecycleStateBlocked || first.Current.State.Terminal == nil || first.Current.State.Terminal.Reason != cfg.Decision.Rationale || first.Current.State.LatestDecision == nil || *first.Current.State.LatestDecision != cfg.Reference {
		t.Fatalf("first=%+v", first)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(first.History.SourcePath))); err != nil {
		t.Fatal(err)
	}
	blockedTask, found, err := taskfile.FindByID(repo, "task-block")
	if err != nil || !found || blockedTask.Status != taskfile.StatusBlocked {
		t.Fatalf("blocked task=%+v found=%v err=%v", blockedTask, found, err)
	}
	replayed, err := Apply(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Disposition != autonomousstate.CommitReplayed || replayed.History.SHA256 != first.History.SHA256 {
		t.Fatalf("replayed=%+v", replayed)
	}
}

func TestApplyRejectsStaleAndConflictingAuthority(t *testing.T) {
	repo, store, snapshot := blockFixture(t)
	cfg := blockConfig(repo, store, snapshot)
	stale := cfg
	stale.Expected.SHA256 = fmt.Sprintf("%064x", 7)
	if _, err := Apply(context.Background(), stale); !errors.Is(err, autonomousstate.ErrStaleWrite) {
		t.Fatalf("stale err=%v", err)
	}
	if _, err := Apply(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	conflict := cfg
	conflict.Decision.Rationale = "A different material reason."
	if _, err := Apply(context.Background(), conflict); err == nil {
		t.Fatal("conflicting replay succeeded")
	}
}

func blockFixture(t *testing.T) (string, *autonomousstate.Store, autonomousstate.Snapshot) {
	t.Helper()
	repo := t.TempDir()
	task := []byte("---\nid: task-block\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/task-block/state.json\n---\n# Block task\n")
	taskPath := filepath.Join(repo, ".agent", "tasks", "task-block.md")
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath, task, 0o644); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: "task-block", Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(repo, ".revolvr", "autonomous", "tasks", "task-block", "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), "task-block")
	if err != nil || !found {
		t.Fatalf("load found=%v err=%v", found, err)
	}
	return repo, store, snapshot
}

func blockConfig(repo string, store *autonomousstate.Store, snapshot autonomousstate.Snapshot) Config {
	now := time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC)
	evidence := autonomous.EvidenceReference{Kind: autonomous.EvidenceKindTask, Reference: ".agent/tasks/task-block.md", Detail: "Exact task evidence."}
	decision := autonomous.SupervisorDecision{TaskID: "task-block", Action: autonomous.ActionBlock, Rationale: "A required external authority is unavailable.", Inputs: []autonomous.EvidenceReference{evidence}}
	reference := autonomous.DecisionReference{DecisionID: "decision-block", RunID: "supervisor-block", TaskID: "task-block", Action: autonomous.ActionBlock, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: ".revolvr/runs/supervisor-block/supervisor-decision.json", Detail: "Exact validated decision artifact."}, CreatedAt: now}
	return Config{RepositoryRoot: repo, TaskID: "task-block", OperationID: "block-operation", Expected: snapshot.Expected(), Decision: decision, Reference: reference, Source: autonomouspolicy.SourceEvidence{Revision: fmt.Sprintf("%064x", 1), Safety: autonomouspolicy.SourceSafetySafe}, CreatedAt: now, Store: store}
}
