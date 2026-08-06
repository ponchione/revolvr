package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/id"
	"revolvr/internal/project"
	"revolvr/internal/runner"
	"revolvr/internal/sandbox"
	"revolvr/internal/storage/postgres"
)

func TestManagedWorkspaceHappyPathPreservesOriginalAndDisablesHooks(t *testing.T) {
	fixture := newWorkspaceFixture(t)
	before := checkoutBytesIdentity(t, fixture.source)
	hookSentinel := filepath.Join(t.TempDir(), "hook-ran")
	installAttemptedHooks(t, fixture.registration.ManagedRepositoryPath, hookSentinel)
	home := t.TempDir()
	globalHook := filepath.Join(home, "global-hooks")
	if err := os.Mkdir(globalHook, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(globalHook, "post-checkout"), []byte("#!/bin/sh\nprintf global >"+shellQuote(hookSentinel)+"\n"), 0o755)
	writeTestFile(t, filepath.Join(home, ".gitconfig"), []byte("[core]\n\thooksPath = "+globalHook+"\n"), 0o600)
	t.Setenv("HOME", home)

	manager := fixture.manager(nil, nil)
	created, err := manager.Create(fixture.ctx, fixture.createRequest("happy-create"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusReady || created.SourceCommit != fixture.registration.CurrentCommit || created.SourceTree != fixture.registration.CurrentTree {
		t.Fatalf("created workspace = %#v", created)
	}
	if created.OriginalAfter == nil || created.OriginalBefore != *created.OriginalAfter {
		t.Fatalf("original checkout source/filesystem identity differs: before=%#v after=%#v", created.OriginalBefore, created.OriginalAfter)
	}
	binding, err := created.SandboxBinding()
	if err != nil {
		t.Fatal(err)
	}
	specification, err := sandbox.Validate(sandbox.Request{
		SchemaVersion: sandbox.RequestSchemaVersion, SandboxID: "sandbox-happy",
		ProjectID: created.ProjectID, TaskID: created.TaskID, RunID: created.RunID,
		Role:           sandbox.RoleImplementer,
		Image:          sandbox.Image{Reference: "example.invalid/worker:1", Digest: "sha256:" + strings.Repeat("a", 64)},
		RuntimeProfile: sandbox.ProfileCompatible, Command: []string{"true"},
		Mounts: []sandbox.Mount{binding.Mount}, Network: sandbox.NetworkNone,
		Resources: sandbox.Resources{CPUs: 1, MemoryBytes: 1 << 20, PIDs: 8, TimeoutSeconds: 30, TmpfsBytes: 1 << 20},
	}, sandbox.Policy{
		ProjectID: created.ProjectID, TaskID: created.TaskID, RunID: created.RunID, Role: sandbox.RoleImplementer,
		ApprovedImages:  []sandbox.Image{{Reference: "example.invalid/worker:1", Digest: "sha256:" + strings.Repeat("a", 64)}},
		AllowedProfiles: []sandbox.RuntimeProfile{sandbox.ProfileCompatible}, AllowedNetworks: []sandbox.NetworkProfile{sandbox.NetworkNone},
		ManagedSources: []sandbox.ManagedSource{binding.Source}, ForbiddenHostPaths: binding.ForbiddenHostPaths,
		MaximumResources: sandbox.Resources{CPUs: 1, MemoryBytes: 1 << 20, PIDs: 8, TimeoutSeconds: 30, TmpfsBytes: 1 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(specification.Mounts) != 1 || specification.Mounts[0].SourcePath != created.Path || specification.Mounts[0].Target != "/workspace" {
		t.Fatalf("sandbox mount = %#v, want exact workspace", specification.Mounts)
	}

	active, err := manager.Activate(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "happy-active"})
	if err != nil || active.Status != StatusActive {
		t.Fatalf("Activate() = %#v, %v", active, err)
	}
	writeTestFile(t, filepath.Join(active.Path, "changed.txt"), []byte("managed workspace bytes\n"), 0o644)
	frozen, err := manager.Freeze(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "happy-frozen"})
	if err != nil || frozen.Status != StatusFrozen {
		t.Fatalf("Freeze() = %#v, %v", frozen, err)
	}
	evidence, err := manager.Commit(fixture.ctx, CommitRequest{WorkspaceID: created.ID, OperationID: "happy-commit", Summary: "Add managed workspace change"})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Workspace.Status != StatusCleaned || evidence.Workspace.TerminalStatus != StatusCompleted {
		t.Fatalf("terminal workspace = %#v", evidence.Workspace)
	}
	if len(evidence.ChangedManifest) != 1 || evidence.ChangedManifest[0].Path != "changed.txt" || evidence.ChangedManifest[0].Kind != "untracked" {
		t.Fatalf("changed manifest = %#v", evidence.ChangedManifest)
	}
	diffBytes, err := os.ReadFile(evidence.DiffArtifactPath)
	if err != nil || hexString(diffBytes) != evidence.DiffSHA256 || !bytes.Contains(diffBytes, []byte("managed workspace bytes")) {
		t.Fatalf("diff artifact hash/content mismatch: err=%v hash=%s", err, hexString(diffBytes))
	}
	if got := runTestGit(t, fixture.registration.ManagedRepositoryPath, "show", evidence.CandidateCommit+":changed.txt"); got != "managed workspace bytes\n" {
		t.Fatalf("candidate bytes = %q", got)
	}
	if got := strings.TrimSpace(runTestGit(t, fixture.registration.ManagedRepositoryPath, "rev-parse", evidence.CandidateCommit+"^{tree}")); got != evidence.CandidateTree {
		t.Fatalf("candidate tree = %q, want %q", got, evidence.CandidateTree)
	}
	if _, err := os.Lstat(created.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleaned worktree still exists: %v", err)
	}
	if _, err := os.Stat(hookSentinel); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Git hook executed or sentinel check failed: %v", err)
	}
	if after := checkoutBytesIdentity(t, fixture.source); after != before {
		t.Fatalf("original checkout identity changed\nbefore=%s\nafter=%s", before, after)
	}

	events, err := postgres.New(fixture.pool).ListWorkspaceEvents(fixture.ctx, mustPGUUID(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	var eventTypes []string
	for _, event := range events {
		eventTypes = append(eventTypes, event.EventType)
	}
	wantEvents := []string{"workspace.planned", "workspace.creating", "workspace.ready", "workspace.active", "workspace.frozen", "workspace.reconciling", "workspace.completed", "workspace.cleaned"}
	if !slices.Equal(eventTypes, wantEvents) {
		t.Fatalf("events = %v, want %v", eventTypes, wantEvents)
	}
	assertAppliedOperations(t, fixture, created.ID, 5)
}

func TestManagedWorkspaceRejectsCollisionsWrongRevisionIdentityAndSymlink(t *testing.T) {
	t.Run("path collision", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		request := fixture.createRequest("path-collision")
		path := filepath.Join(fixture.workspaceRoot, request.WorkspaceID)
		writeTestFile(t, filepath.Join(path, "sentinel"), []byte("foreign\n"), 0o644)
		_, err := fixture.manager(nil, nil).Create(fixture.ctx, request)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("Create() error = %v, want conflict", err)
		}
		if got := readTestFile(t, filepath.Join(path, "sentinel")); got != "foreign\n" {
			t.Fatalf("collision sentinel changed: %q", got)
		}
	})

	t.Run("branch collision", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		request := fixture.createRequest("branch-collision")
		branch := "refs/heads/revolvr/workspaces/" + request.WorkspaceID
		runTestGit(t, fixture.registration.ManagedRepositoryPath, "update-ref", branch, fixture.registration.CurrentCommit)
		_, err := fixture.manager(nil, nil).Create(fixture.ctx, request)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("Create() error = %v, want conflict", err)
		}
		if got := strings.TrimSpace(runTestGit(t, fixture.registration.ManagedRepositoryPath, "rev-parse", branch)); got != fixture.registration.CurrentCommit {
			t.Fatalf("foreign branch changed: %q", got)
		}
	})

	t.Run("wrong scheduler source", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		if _, err := fixture.pool.Exec(fixture.ctx, "UPDATE core.project_sources SET current_tree = $1 WHERE id = $2", strings.Repeat("0", 40), fixture.registration.ProjectSourceID); err != nil {
			t.Fatal(err)
		}
		request := fixture.createRequest("wrong-source")
		_, err := fixture.manager(nil, nil).Create(fixture.ctx, request)
		if !errors.Is(err, ErrWrongSourceRevision) {
			t.Fatalf("Create() error = %v, want wrong source revision", err)
		}
		if _, err := os.Lstat(filepath.Join(fixture.workspaceRoot, request.WorkspaceID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("wrong-source worktree exists: %v", err)
		}
	})

	t.Run("operation identity mismatch", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		request := fixture.createRequest("identity-one")
		created, err := fixture.manager(nil, nil).Create(fixture.ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		changed := request
		changed.OperationID = "identity-two"
		if _, err := fixture.manager(nil, nil).Create(fixture.ctx, changed); !errors.Is(err, ErrConflict) {
			t.Fatalf("changed operation Create() error = %v, want conflict", err)
		}
		_, _ = fixture.manager(nil, nil).Cancel(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "identity-cancel", Reason: "test cleanup"})
	})

	t.Run("symlink substitution", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		manager := fixture.manager(nil, nil)
		created, err := manager.Create(fixture.ctx, fixture.createRequest("symlink-create"))
		if err != nil {
			t.Fatal(err)
		}
		displaced := created.Path + ".displaced"
		if err := os.Rename(created.Path, displaced); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), created.Path); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Activate(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "symlink-active"}); !errors.Is(err, ErrConflict) && !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Activate() error = %v, want safe refusal", err)
		}
		if err := os.Remove(created.Path); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(displaced, created.Path); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Cancel(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "symlink-cancel", Reason: "test cleanup"}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestManagedWorkspaceCancellationTimeoutAndCleanupFailure(t *testing.T) {
	t.Run("cancellation retains diff and cleans", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		manager := fixture.manager(nil, nil)
		created, err := manager.Create(fixture.ctx, fixture.createRequest("cancel-create"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Activate(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "cancel-active"}); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(created.Path, "cancelled.txt"), []byte("retained cancellation evidence\n"), 0o644)
		cancelled, err := manager.Cancel(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "cancel-run", Reason: "operator cancellation"})
		if err != nil {
			t.Fatal(err)
		}
		if cancelled.Status != StatusCleaned || cancelled.TerminalStatus != StatusCancelled || cancelled.DiffSHA256 == "" || len(cancelled.ChangedManifest) != 1 {
			t.Fatalf("cancelled workspace = %#v", cancelled)
		}
		if raw := readTestBytes(t, cancelled.DiffArtifactPath); !bytes.Contains(raw, []byte("retained cancellation evidence")) {
			t.Fatalf("cancellation diff = %q", raw)
		}
	})

	t.Run("timeout reconciles terminal cleanup", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		runner := func(ctx context.Context, command runner.Command) runner.Result {
			if commandHasGitSequence(command.Args, "worktree", "add") {
				return runner.Result{ExitCode: -1, TimedOut: true, Err: context.DeadlineExceeded}
			}
			return runner.Run(ctx, command)
		}
		request := fixture.createRequest("timeout-create")
		_, err := fixture.manager(runner, nil).Create(fixture.ctx, request)
		if err == nil {
			t.Fatal("Create() succeeded despite injected timeout")
		}
		workspace, getErr := fixture.manager(nil, nil).Get(fixture.ctx, request.WorkspaceID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if workspace.Status != StatusCleaned || workspace.TerminalStatus != StatusFailed {
			t.Fatalf("timeout workspace = %#v", workspace)
		}
		if _, err := os.Lstat(workspace.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("timeout worktree remains: %v", err)
		}
	})

	t.Run("cleanup failure is durable and retryable", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		manager := fixture.manager(nil, nil)
		created, err := manager.Create(fixture.ctx, fixture.createRequest("cleanup-failure-create"))
		if err != nil {
			t.Fatal(err)
		}
		failingRunner := func(ctx context.Context, command runner.Command) runner.Result {
			if commandHasGitSequence(command.Args, "worktree", "remove") {
				return runner.Result{ExitCode: 2, Stderr: "injected cleanup failure"}
			}
			return runner.Run(ctx, command)
		}
		failing := fixture.manager(failingRunner, nil)
		_, err = failing.Cancel(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "cleanup-failure", Reason: "operator cancellation"})
		if !errors.Is(err, ErrCleanupFailed) {
			t.Fatalf("Cancel() error = %v, want cleanup failure", err)
		}
		terminal, err := manager.Get(fixture.ctx, created.ID)
		if err != nil || terminal.Status != StatusCancelled {
			t.Fatalf("terminal after cleanup failure = %#v, %v", terminal, err)
		}
		cleaned, err := manager.Cleanup(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "cleanup-failure:cleanup"})
		if err != nil || cleaned.Status != StatusCleaned {
			t.Fatalf("Cleanup() = %#v, %v", cleaned, err)
		}
		replayed, err := manager.Cleanup(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "cleanup-failure:cleanup"})
		if err != nil || replayed.Status != StatusCleaned {
			t.Fatalf("replayed Cleanup() = %#v, %v", replayed, err)
		}
		if _, err := manager.Cleanup(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "different-cleanup"}); !errors.Is(err, ErrConflict) {
			t.Fatalf("divergent Cleanup() error = %v, want conflict", err)
		}
	})
}

func TestManagedWorkspaceCrashRecoveryAfterBranchWorktreeAndCommit(t *testing.T) {
	for _, point := range []FailurePoint{FailureAfterBranch, FailureAfterWorktree} {
		t.Run(string(point), func(t *testing.T) {
			fixture := newWorkspaceFixture(t)
			injected := false
			injector := func(actual FailurePoint) error {
				if actual == point && !injected {
					injected = true
					return errors.New("simulated process crash")
				}
				return nil
			}
			request := fixture.createRequest("recover-" + string(point))
			if _, err := fixture.manager(nil, injector).Create(fixture.ctx, request); !errors.Is(err, ErrInjectedCrash) {
				t.Fatalf("first Create() error = %v, want injected crash", err)
			}
			created, err := fixture.manager(nil, nil).Create(fixture.ctx, request)
			if err != nil || created.Status != StatusReady || !created.Replayed {
				t.Fatalf("recovered Create() = %#v, %v", created, err)
			}
			if _, err := fixture.manager(nil, nil).Cancel(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "recover-cleanup-" + string(point), Reason: "test cleanup"}); err != nil {
				t.Fatal(err)
			}
		})
	}

	t.Run("after candidate commit", func(t *testing.T) {
		fixture := newWorkspaceFixture(t)
		base := fixture.manager(nil, nil)
		created, err := base.Create(fixture.ctx, fixture.createRequest("recover-commit-create"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := base.Activate(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "recover-commit-active"}); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(created.Path, "recovered.txt"), []byte("crash recovery\n"), 0o644)
		if _, err := base.Freeze(fixture.ctx, TransitionRequest{WorkspaceID: created.ID, OperationID: "recover-commit-frozen"}); err != nil {
			t.Fatal(err)
		}
		injected := false
		crashing := fixture.manager(nil, func(point FailurePoint) error {
			if point == FailureAfterCommit && !injected {
				injected = true
				return errors.New("simulated commit crash")
			}
			return nil
		})
		request := CommitRequest{WorkspaceID: created.ID, OperationID: "recover-candidate", Summary: "Recover exact candidate"}
		if _, err := crashing.Commit(fixture.ctx, request); !errors.Is(err, ErrInjectedCrash) {
			t.Fatalf("first Commit() error = %v, want injected crash", err)
		}
		advanced := strings.TrimSpace(runTestGit(t, fixture.registration.ManagedRepositoryPath, "rev-parse", "refs/heads/revolvr/workspaces/"+created.ID))
		if advanced == fixture.registration.CurrentCommit {
			t.Fatal("candidate branch did not advance before injected crash")
		}
		evidence, err := base.Commit(fixture.ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if evidence.CandidateCommit != advanced || evidence.Workspace.Status != StatusCleaned || !evidence.Workspace.Replayed {
			t.Fatalf("recovered evidence = %#v, want commit %s", evidence, advanced)
		}
		if got := strings.TrimSpace(runTestGit(t, fixture.registration.ManagedRepositoryPath, "rev-list", "--count", fixture.registration.CurrentCommit+".."+advanced)); got != "1" {
			t.Fatalf("candidate commit count = %q, want exactly one", got)
		}
	})
}

type workspaceFixture struct {
	t             *testing.T
	ctx           context.Context
	pool          *pgxpool.Pool
	source        string
	registration  project.Registration
	workspaceRoot string
	artifactRoot  string
	taskID        pgtype.UUID
	taskVersionID pgtype.UUID
	runID         pgtype.UUID
	artifactID    pgtype.UUID
}

func newWorkspaceFixture(t *testing.T) *workspaceFixture {
	t.Helper()
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	root := t.TempDir()
	source := filepath.Join(root, "operator-checkout")
	runTestGit(t, "", "init", "-q", "-b", "main", source)
	runTestGit(t, source, "config", "user.name", "Workspace Test")
	runTestGit(t, source, "config", "user.email", "workspace@example.invalid")
	writeTestFile(t, filepath.Join(source, "tracked.txt"), []byte("original bytes\n"), 0o644)
	runTestGit(t, source, "add", "tracked.txt")
	runTestGit(t, source, "commit", "-q", "-m", "initial")
	registration, err := project.Register(ctx, pool, source, filepath.Join(root, "managed"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &workspaceFixture{
		t: t, ctx: ctx, pool: pool, source: source, registration: registration,
		workspaceRoot: filepath.Join(root, "workspaces"), artifactRoot: filepath.Join(root, "artifacts"),
		taskID: newTestUUID(), taskVersionID: newTestUUID(), runID: newTestUUID(), artifactID: newTestUUID(),
	}
	if err := os.Mkdir(fixture.workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.artifactRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.insertRunAuthority()
	t.Cleanup(fixture.cleanupDatabase)
	return fixture
}

func (f *workspaceFixture) insertRunAuthority() {
	f.t.Helper()
	queries := postgres.New(f.pool)
	created := timestamp(time.Now().UTC().Truncate(time.Microsecond))
	contentHash := sha256.Sum256([]byte(uuid.NewString()))
	if _, err := queries.InsertArtifact(f.ctx, postgres.InsertArtifactParams{
		ID: f.artifactID, Sha256: hex.EncodeToString(contentHash[:]), SizeBytes: 1,
		MediaType: "text/markdown", LogicalKind: "workspace_test_task", StoragePath: "test/" + uuid.NewString(), CreatedAt: created,
	}); err != nil {
		f.t.Fatal(err)
	}
	projectID := mustPGUUID(f.registration.ProjectID)
	if _, err := queries.InsertTask(f.ctx, postgres.InsertTaskParams{
		ID: f.taskID, ProjectID: projectID, ExternalTaskID: "workspace-" + uuid.NewString(),
		Status: "draft", CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		f.t.Fatal(err)
	}
	emptyArray, object := []byte(`[]`), []byte(`{}`)
	if _, err := queries.InsertTaskVersion(f.ctx, postgres.InsertTaskVersionParams{
		ID: f.taskVersionID, TaskID: f.taskID, VersionNumber: 1, SourceArtifactID: f.artifactID,
		Title: "Workspace lifecycle", Goal: "Exercise managed workspace", RiskClass: "medium",
		MutationClass: "bounded_source", NetworkProfile: "none", Priority: 1,
		Scope: emptyArray, ExcludedScope: emptyArray, VerificationPlan: emptyArray, Budget: object,
		SecretRequirements: emptyArray, ExpectedPaths: emptyArray, OperatorCheckpoints: emptyArray, CreatedAt: created,
	}); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE core.tasks
		SET accepted_version_id = $1, status = 'admitted', aggregate_version = 2, updated_at = $2
		WHERE id = $3`, f.taskVersionID, created, f.taskID); err != nil {
		f.t.Fatal(err)
	}
	if _, err := queries.InsertRun(f.ctx, postgres.InsertRunParams{
		ID: f.runID, ProjectID: projectID, TaskID: f.taskID, TaskVersionID: f.taskVersionID,
		ProjectSourceID: mustPGUUID(f.registration.ProjectSourceID), AdmittedTaskAggregateVersion: 2,
		SourceCommit: f.registration.CurrentCommit, SourceTree: f.registration.CurrentTree,
		CoordinatorIdentity: "workspace-test", CreatedAt: created,
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *workspaceFixture) cleanupDatabase() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statements := []struct {
		query string
		args  []any
	}{
		{"DELETE FROM core.workspace_operations WHERE workspace_id IN (SELECT id FROM core.workspaces WHERE run_id = $1)", []any{f.runID}},
		{"DELETE FROM core.workspaces WHERE run_id = $1", []any{f.runID}},
		{"DELETE FROM core.events WHERE project_id = $1", []any{f.registration.ProjectID}},
		{"DELETE FROM core.runs WHERE id = $1", []any{f.runID}},
		{"UPDATE core.tasks SET accepted_version_id = NULL, status = 'draft' WHERE id = $1", []any{f.taskID}},
		{"DELETE FROM core.task_versions WHERE task_id = $1", []any{f.taskID}},
		{"DELETE FROM core.tasks WHERE id = $1", []any{f.taskID}},
		{"DELETE FROM core.artifacts WHERE logical_kind = 'workspace_diff' AND storage_path LIKE $1", []any{f.artifactRoot + "/%"}},
		{"DELETE FROM core.artifacts WHERE id = $1", []any{f.artifactID}},
		{"DELETE FROM core.project_sources WHERE id = $1", []any{f.registration.ProjectSourceID}},
		{"DELETE FROM core.projects WHERE id = $1", []any{f.registration.ProjectID}},
	}
	for _, statement := range statements {
		if _, err := f.pool.Exec(ctx, statement.query, statement.args...); err != nil {
			f.t.Errorf("cleanup %q: %v", statement.query, err)
		}
	}
}

func (f *workspaceFixture) manager(commandRunner CommandRunner, injector FailureInjector) *Manager {
	f.t.Helper()
	manager, err := New(Config{
		Pool: f.pool, WorkspaceRoot: f.workspaceRoot, ArtifactRoot: f.artifactRoot,
		Timeout: 10 * time.Second, CommandRunner: commandRunner, FailureInjector: injector,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return manager
}

func (f *workspaceFixture) createRequest(operation string) CreateRequest {
	return CreateRequest{WorkspaceID: id.New(), RunID: uuidString(f.runID), OperationID: operation}
}

func assertAppliedOperations(t *testing.T, fixture *workspaceFixture, workspaceID string, count int) {
	t.Helper()
	var got int
	if err := fixture.pool.QueryRow(fixture.ctx, "SELECT count(*) FROM core.workspace_operations WHERE workspace_id = $1 AND status = 'applied'", workspaceID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != count {
		t.Fatalf("applied operations = %d, want %d", got, count)
	}
}

func installAttemptedHooks(t *testing.T, repository, sentinel string) {
	t.Helper()
	for _, name := range []string{"post-checkout", "pre-commit"} {
		writeTestFile(t, filepath.Join(repository, "hooks", name), []byte("#!/bin/sh\nprintf hook >"+shellQuote(sentinel)+"\n"), 0o755)
	}
}

func checkoutBytesIdentity(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(hash, "root:%d:%s\n", info.Mode(), info.ModTime().UTC().Format(time.RFC3339Nano))
	for _, args := range [][]string{{"rev-parse", "HEAD^{commit}"}, {"rev-parse", "HEAD^{tree}"}, {"status", "--porcelain=v1", "-z", "--untracked-files=all"}} {
		fmt.Fprintf(hash, "git:%q:%s\x00", args, runTestGit(t, root, args...))
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		if rel == ".git" {
			return filepath.SkipDir
		}
		if rel == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s:%s:%d:", filepath.ToSlash(rel), info.Mode(), info.Size())
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash.Write(raw)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hash.Write([]byte(target))
		}
		hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func commandHasGitSequence(args []string, left, right string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == left && args[index+1] == right {
			return true
		}
	}
	return false
}

func runTestGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	commandArgs := args
	if directory != "" {
		commandArgs = append([]string{"-C", directory}, args...)
	}
	command := exec.Command("git", commandArgs...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", commandArgs, err, output)
	}
	return string(output)
}

func writeTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	return string(readTestBytes(t, path))
}

func readTestBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newTestUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(id.New()), Valid: true}
}

func mustPGUUID(value string) pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.MustParse(value), Valid: true}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
