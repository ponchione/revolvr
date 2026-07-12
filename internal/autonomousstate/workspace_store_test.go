package autonomousstate

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
)

func TestCommitWorkspacePersistsReplaysConflictsAndPreservesState(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	store := openStateTestStore(t, repo, nil)
	initial, err := store.CommitPlanning(context.Background(), stateTestRequest(t, repo, taskRaw, "plan-operation", "plan-one"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := testWorkspace(repo, "task-1")
	next := initial.Current.State
	next.Workspace = &workspace
	previousRaw, _ := MarshalState(initial.Current.State)
	nextRaw, _ := MarshalState(next)
	record := WorkspaceHistoryRecord{SchemaVersion: WorkspaceHistorySchemaVersion, TaskID: "task-1", OperationID: "workspace-create", ApplicationSHA256: strings.Repeat("a", 64), Kind: WorkspaceTransitionCreated, CreatedAt: workspace.CreatedAt, ResultingWorkspace: workspace, PreviousState: stateIdentity(initial.Current.SourcePath, true, previousRaw), ResultingState: stateIdentity(initial.Current.SourcePath, true, nextRaw)}
	request := WorkspaceCommitRequest{TaskID: "task-1", Expected: initial.Current.Expected(), PreviousState: initial.Current.State, NextState: next, History: record}

	result, err := store.CommitWorkspace(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitUpdated || !reflect.DeepEqual(result.Current.State.Workspace, &workspace) {
		t.Fatalf("workspace commit = %+v", result)
	}
	replayed, err := store.CommitWorkspace(context.Background(), request)
	if err != nil || replayed.Disposition != CommitReplayed {
		t.Fatalf("replay = %+v, %v", replayed, err)
	}
	conflict := request
	conflict.History.ApplicationSHA256 = strings.Repeat("b", 64)
	if _, err := store.CommitWorkspace(context.Background(), conflict); !errors.Is(err, ErrOperationConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	stale := request
	stale.History.OperationID = "workspace-stale"
	stale.History.ApplicationSHA256 = strings.Repeat("c", 64)
	if _, err := store.CommitWorkspace(context.Background(), stale); !errors.Is(err, ErrStaleWrite) {
		t.Fatalf("stale error = %v", err)
	}
}

func TestCommitWorkspaceRecoversFailureAfterStateRename(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	clean := openStateTestStore(t, repo, nil)
	initial, err := clean.CommitPlanning(context.Background(), stateTestRequest(t, repo, taskRaw, "plan-operation", "plan-one"))
	if err != nil {
		t.Fatal(err)
	}
	request := workspaceRequest(t, repo, initial.Current, "workspace-after-rename")
	injected := openStateTestStore(t, repo, func(point FailurePoint) error {
		if point == FailureAfterStateRename {
			return errors.New("injected after rename")
		}
		return nil
	})
	if _, err := injected.CommitWorkspace(context.Background(), request); err == nil || !strings.Contains(err.Error(), "injected after rename") {
		t.Fatalf("commit error = %v", err)
	}
	reopened := openStateTestStore(t, repo, nil)
	snapshot, found, err := reopened.Load(context.Background(), "task-1")
	if err != nil || !found || snapshot.State.Workspace == nil {
		t.Fatalf("reopen = %+v %t %v", snapshot, found, err)
	}
	replayed, err := reopened.CommitWorkspace(context.Background(), request)
	if err != nil || replayed.Disposition != CommitReplayed {
		t.Fatalf("replay = %+v %v", replayed, err)
	}
}

func TestCommittedAuditReconstructionTraversesWorkspaceTransition(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	auditRequest := auditStoreRequest(t, repo, taskRaw)
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1"))), mustMarshalState(t, auditRequest.PreviousState))
	store := openStateTestStore(t, repo, nil)
	auditResult, err := store.CommitAudit(context.Background(), auditRequest)
	if err != nil {
		t.Fatal(err)
	}
	request := workspaceRequest(t, repo, auditResult.Current, "workspace-after-audit")
	if _, err := store.CommitWorkspace(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	current, found, err := store.LoadCurrentAudit(context.Background(), "task-1")
	if err != nil || !found || current.Report.Disposition != auditRequest.History.Report.Disposition {
		t.Fatalf("current audit found=%t err=%v report=%+v", found, err, current.Report)
	}
}

func workspaceRequest(t *testing.T, repo string, initial Snapshot, operation string) WorkspaceCommitRequest {
	t.Helper()
	workspace := testWorkspace(repo, "task-1")
	next := initial.State
	next.Workspace = &workspace
	previousRaw, _ := MarshalState(initial.State)
	nextRaw, _ := MarshalState(next)
	record := WorkspaceHistoryRecord{SchemaVersion: WorkspaceHistorySchemaVersion, TaskID: "task-1", OperationID: operation, ApplicationSHA256: strings.Repeat("d", 64), Kind: WorkspaceTransitionCreated, CreatedAt: workspace.CreatedAt, ResultingWorkspace: workspace, PreviousState: stateIdentity(initial.SourcePath, true, previousRaw), ResultingState: stateIdentity(initial.SourcePath, true, nextRaw)}
	return WorkspaceCommitRequest{TaskID: "task-1", Expected: initial.Expected(), PreviousState: initial.State, NextState: next, History: record}
}

func testWorkspace(repo, taskID string) autonomous.TaskWorkspace {
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)
	commit := strings.Repeat("1", 40)
	tree := strings.Repeat("2", 40)
	source := strings.Repeat("3", 64)
	root := filepath.Clean(repo)
	return autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: taskID, WorkspaceID: "workspace-one", ControlRoot: root, ExecutionRoot: filepath.Join(root, ".revolvr", "autonomous", "worktrees", "workspace-one"), GitCommonDir: filepath.Join(root, ".git"), BranchRef: "refs/heads/revolvr/tasks/task-1-workspace", OwnerMarker: filepath.Join(root, ".revolvr", "autonomous", "tasks", taskID, "workspace-owner.json"), BaselineSHA: commit, HeadSHA: commit, TreeSHA: tree, SourceRevision: source, Checkpoint: autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: commit, TreeSHA: tree, SourceRevision: source, OperationID: "workspace-create", Provenance: "exact baseline", CreatedAt: now}, Status: autonomous.WorkspaceStatusReady, CreatedAt: now, UpdatedAt: now}
}
