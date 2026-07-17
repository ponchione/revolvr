package autonomousworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/lock"
	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
)

var fixedTime = time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)

func TestPrepareUsesExactCommitAndIgnoresDirtyPrimaryWorktree(t *testing.T) {
	repo, baseline := testRepository(t)
	writeFile(t, filepath.Join(repo, "tracked.txt"), "dirty primary\n")
	runGit(t, repo, "add", "tracked.txt")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "untracked.txt"), "operator bytes\n")

	result, err := Prepare(context.Background(), testConfig(repo, "task-one", "create-one", baseline))
	if err != nil {
		t.Fatal(err)
	}
	if result.Workspace.HeadSHA != baseline || result.Workspace.Checkpoint.CommitSHA != baseline {
		t.Fatalf("workspace identity = %+v, want baseline %s", result.Workspace, baseline)
	}
	if got := readFile(t, filepath.Join(result.Workspace.ExecutionRoot, "tracked.txt")); got != "baseline\n" {
		t.Fatalf("execution tracked bytes = %q", got)
	}
	if got := readFile(t, filepath.Join(result.Workspace.ExecutionRoot, "deleted.txt")); got != "baseline delete guard\n" {
		t.Fatalf("deleted primary bytes entered workspace: %q", got)
	}
	if _, err := os.Stat(filepath.Join(result.Workspace.ExecutionRoot, "untracked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("primary untracked file entered workspace: %v", err)
	}
	if len(result.PrimaryDirty) != 3 {
		t.Fatalf("primary dirty evidence = %v", result.PrimaryDirty)
	}
	if result.Workspace.ControlRoot == result.Workspace.ExecutionRoot || result.Workspace.GitCommonDir == "" {
		t.Fatalf("control/execution identity is ambiguous: %+v", result.Workspace)
	}
	firstJSON, err := DeterministicJSON(result.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := DeterministicJSON(result.Workspace)
	if err != nil || string(firstJSON) != string(secondJSON) {
		t.Fatalf("workspace JSON is nondeterministic: %v", err)
	}
	var decoded autonomous.TaskWorkspace
	if err := json.Unmarshal(firstJSON, &decoded); err != nil || decoded.WorkspaceID != result.Workspace.WorkspaceID {
		t.Fatalf("workspace JSON round trip = %+v, %v", decoded, err)
	}
}

func TestExternalTaskWorkspaceAuthority(t *testing.T) {
	t.Run("records exact isolated baseline authority", func(t *testing.T) {
		repo, baseline := testRepository(t)
		ambientBranch := runGit(t, repo, "symbolic-ref", "HEAD")
		writeFile(t, filepath.Join(repo, "operator-only.txt"), "ambient operator branch\n")
		runGit(t, repo, "add", "operator-only.txt")
		runGit(t, repo, "commit", "-q", "-m", "operator branch advance")
		ambientHead := runGit(t, repo, "rev-parse", "HEAD")

		cfg := testConfig(repo, "external-authority", "prepare-external-authority", baseline)
		prepared, err := Prepare(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		n, err := normalize(context.Background(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		workspace := prepared.Workspace
		if workspace.ControlRoot != n.root || workspace.ExecutionRoot != n.execution || workspace.GitCommonDir != n.common || workspace.BranchRef != n.branchRef || workspace.OwnerMarker != n.markerPath {
			t.Fatalf("recorded workspace authority = %+v, want normalized authority %+v", workspace, n)
		}
		if workspace.BaselineSHA != baseline || workspace.HeadSHA != baseline || workspace.Checkpoint.CommitSHA != baseline {
			t.Fatalf("baseline authority = baseline %s head %s checkpoint %s, want %s", workspace.BaselineSHA, workspace.HeadSHA, workspace.Checkpoint.CommitSHA, baseline)
		}
		if got := runGit(t, workspace.ExecutionRoot, "symbolic-ref", "HEAD"); got != workspace.BranchRef {
			t.Fatalf("execution branch = %q, want %q", got, workspace.BranchRef)
		}
		registry := runGit(t, repo, "worktree", "list", "--porcelain", "-z")
		if !registered(registry, workspace.ExecutionRoot, workspace.BranchRef) {
			t.Fatalf("workspace registration does not contain exact path/ref: %q", registry)
		}
		marker, found, err := readMarker(n.root, workspace.OwnerMarker)
		if err != nil || !found {
			t.Fatalf("read ownership marker: found=%t err=%v", found, err)
		}
		if err := validateMarker(marker, n); err != nil {
			t.Fatalf("ownership marker does not bind exact authority: %v", err)
		}
		if got := runGit(t, repo, "symbolic-ref", "HEAD"); got != ambientBranch {
			t.Fatalf("ambient branch changed to %q, want %q", got, ambientBranch)
		}
		if got := runGit(t, repo, "rev-parse", "HEAD"); got != ambientHead {
			t.Fatalf("ambient HEAD changed to %q, want %q", got, ambientHead)
		}
		if _, err := os.Stat(filepath.Join(workspace.ExecutionRoot, "operator-only.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ambient branch bytes entered task workspace: %v", err)
		}
		raw, err := DeterministicJSON(workspace)
		if err != nil {
			t.Fatal(err)
		}
		var durable autonomous.TaskWorkspace
		if err := json.Unmarshal(raw, &durable); err != nil {
			t.Fatal(err)
		}
		if durable.ControlRoot != workspace.ControlRoot || durable.GitCommonDir != workspace.GitCommonDir || durable.BranchRef != workspace.BranchRef || durable.BaselineSHA != workspace.BaselineSHA || durable.HeadSHA != workspace.HeadSHA || durable.OwnerMarker != workspace.OwnerMarker {
			t.Fatalf("durable workspace evidence lost authority: %+v", durable)
		}
	})

	t.Run("refuses drifted durable identity before source mutation", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*autonomous.TaskWorkspace)
		}{
			{name: "workspace path", mutate: func(w *autonomous.TaskWorkspace) {
				w.ExecutionRoot = filepath.Join(w.ControlRoot, ".revolvr", "autonomous", "worktrees", "foreign")
			}},
			{name: "Git common directory", mutate: func(w *autonomous.TaskWorkspace) { w.GitCommonDir = filepath.Join(w.ControlRoot, ".git", "objects") }},
			{name: "branch ref", mutate: func(w *autonomous.TaskWorkspace) { w.BranchRef = "refs/heads/revolvr/tasks/foreign" }},
			{name: "ownership marker", mutate: func(w *autonomous.TaskWorkspace) {
				w.OwnerMarker = filepath.Join(w.ControlRoot, ".revolvr", "autonomous", "tasks", "foreign", "workspace-owner.json")
			}},
			{name: "baseline", mutate: func(w *autonomous.TaskWorkspace) { w.BaselineSHA = strings.Repeat("1", len(w.BaselineSHA)) }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo, baseline := testRepository(t)
				cfg := testConfig(repo, "durable-drift", "prepare-durable-drift", baseline)
				prepared, err := Prepare(context.Background(), cfg)
				if err != nil {
					t.Fatal(err)
				}
				before := captureWorkspaceSource(t, prepared.Workspace.ExecutionRoot)
				expected := prepared.Workspace
				tt.mutate(&expected)
				_, err = Reopen(context.Background(), testConfig(repo, "durable-drift", "reopen-durable-drift", baseline), expected)
				if !errors.Is(err, ErrUnsafeOwnership) {
					t.Fatalf("Reopen error = %v, want ErrUnsafeOwnership", err)
				}
				assertWorkspaceSource(t, prepared.Workspace.ExecutionRoot, before)
			})
		}
	})

	t.Run("refuses foreign live authority before source mutation", func(t *testing.T) {
		t.Run("marker symlink", func(t *testing.T) {
			repo, baseline := testRepository(t)
			prepared, err := Prepare(context.Background(), testConfig(repo, "marker-link", "prepare-marker-link", baseline))
			if err != nil {
				t.Fatal(err)
			}
			markerBytes, err := os.ReadFile(prepared.Workspace.OwnerMarker)
			if err != nil {
				t.Fatal(err)
			}
			outsideMarker := filepath.Join(t.TempDir(), "workspace-owner.json")
			if err := os.WriteFile(outsideMarker, markerBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(prepared.Workspace.OwnerMarker); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outsideMarker, prepared.Workspace.OwnerMarker); err != nil {
				t.Fatal(err)
			}
			before := captureWorkspaceSource(t, prepared.Workspace.ExecutionRoot)
			_, err = Reopen(context.Background(), testConfig(repo, "marker-link", "reopen-marker-link", baseline), prepared.Workspace)
			if !errors.Is(err, ErrUnsafeOwnership) {
				t.Fatalf("Reopen error = %v, want ErrUnsafeOwnership", err)
			}
			assertWorkspaceSource(t, prepared.Workspace.ExecutionRoot, before)
			if got := readFile(t, outsideMarker); got != string(markerBytes) {
				t.Fatalf("outside marker changed: %q", got)
			}
		})

		t.Run("linked Git file symlink", func(t *testing.T) {
			repo, baseline := testRepository(t)
			prepared, err := Prepare(context.Background(), testConfig(repo, "git-link", "prepare-git-link", baseline))
			if err != nil {
				t.Fatal(err)
			}
			gitLink := filepath.Join(prepared.Workspace.ExecutionRoot, ".git")
			gitLinkBytes, err := os.ReadFile(gitLink)
			if err != nil {
				t.Fatal(err)
			}
			outsideLink := filepath.Join(t.TempDir(), "git-link")
			if err := os.WriteFile(outsideLink, gitLinkBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(gitLink); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outsideLink, gitLink); err != nil {
				t.Fatal(err)
			}
			before := captureWorkspaceSource(t, prepared.Workspace.ExecutionRoot)
			_, err = Reopen(context.Background(), testConfig(repo, "git-link", "reopen-git-link", baseline), prepared.Workspace)
			if !errors.Is(err, ErrUnsafeOwnership) {
				t.Fatalf("Reopen error = %v, want ErrUnsafeOwnership", err)
			}
			assertWorkspaceSource(t, prepared.Workspace.ExecutionRoot, before)
			if got := readFile(t, outsideLink); got != string(gitLinkBytes) {
				t.Fatalf("outside Git link changed: %q", got)
			}
		})

		t.Run("worktree registration path", func(t *testing.T) {
			repo, baseline := testRepository(t)
			prepared, err := Prepare(context.Background(), testConfig(repo, "registration", "prepare-registration", baseline))
			if err != nil {
				t.Fatal(err)
			}
			cfg := testConfig(repo, "registration", "reopen-registration", baseline)
			cfg.CommandRunner = func(ctx context.Context, command runner.Command) runner.Result {
				result := runner.Run(ctx, command)
				if command.Dir == repo && equalStrings(command.Args, []string{"worktree", "list", "--porcelain", "-z"}) {
					result.Stdout = strings.ReplaceAll(result.Stdout, prepared.Workspace.ExecutionRoot, prepared.Workspace.ExecutionRoot+"-foreign")
				}
				return result
			}
			before := captureWorkspaceSource(t, prepared.Workspace.ExecutionRoot)
			_, err = Reopen(context.Background(), cfg, prepared.Workspace)
			if !errors.Is(err, ErrUnsafeOwnership) {
				t.Fatalf("Reopen error = %v, want ErrUnsafeOwnership", err)
			}
			assertWorkspaceSource(t, prepared.Workspace.ExecutionRoot, before)
		})

		t.Run("Git common directory relationship", func(t *testing.T) {
			repo, baseline := testRepository(t)
			prepared, err := Prepare(context.Background(), testConfig(repo, "common-dir", "prepare-common-dir", baseline))
			if err != nil {
				t.Fatal(err)
			}
			foreignCommon := t.TempDir()
			cfg := testConfig(repo, "common-dir", "reopen-common-dir", baseline)
			cfg.CommandRunner = func(ctx context.Context, command runner.Command) runner.Result {
				result := runner.Run(ctx, command)
				if command.Dir == prepared.Workspace.ExecutionRoot && equalStrings(command.Args, []string{"rev-parse", "--git-common-dir"}) {
					result.Stdout = foreignCommon + "\n"
				}
				return result
			}
			before := captureWorkspaceSource(t, prepared.Workspace.ExecutionRoot)
			_, err = Reopen(context.Background(), cfg, prepared.Workspace)
			if !errors.Is(err, ErrUnsafeOwnership) {
				t.Fatalf("Reopen error = %v, want ErrUnsafeOwnership", err)
			}
			assertWorkspaceSource(t, prepared.Workspace.ExecutionRoot, before)
		})

		t.Run("changed control root", func(t *testing.T) {
			repo, baseline := testRepository(t)
			prepared, err := Prepare(context.Background(), testConfig(repo, "control-root", "prepare-control-root", baseline))
			if err != nil {
				t.Fatal(err)
			}
			before := captureWorkspaceSource(t, prepared.Workspace.ExecutionRoot)
			cfg := testConfig(prepared.Workspace.ExecutionRoot, "control-root", "reopen-control-root", baseline)
			_, err = Reopen(context.Background(), cfg, prepared.Workspace)
			if !errors.Is(err, ErrUnsafeOwnership) {
				t.Fatalf("Reopen error = %v, want ErrUnsafeOwnership", err)
			}
			assertWorkspaceSource(t, prepared.Workspace.ExecutionRoot, before)
			if _, err := os.Lstat(filepath.Join(prepared.Workspace.ExecutionRoot, ".revolvr")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("changed control root created task-source runtime state: %v", err)
			}
		})
	})
}

func TestConcurrentPrimaryEditDoesNotEnterWorkspace(t *testing.T) {
	repo, baseline := testRepository(t)
	cfg := testConfig(repo, "task-concurrent", "create-concurrent", baseline)
	mutated := false
	cfg.CommandRunner = func(ctx context.Context, command runner.Command) runner.Result {
		result := runner.Run(ctx, command)
		if !mutated && len(command.Args) > 0 && command.Args[0] == "update-ref" {
			mutated = true
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("concurrent primary\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	prepared, err := Prepare(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !mutated {
		t.Fatal("concurrent mutation hook was not reached")
	}
	if got := readFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "tracked.txt")); got != "baseline\n" {
		t.Fatalf("concurrent bytes entered workspace: %q", got)
	}
	if got := readFile(t, filepath.Join(repo, "tracked.txt")); got != "concurrent primary\n" {
		t.Fatalf("primary bytes were altered: %q", got)
	}
}

func TestPrepareReopensCrashAfterOwnedRefCreation(t *testing.T) {
	repo, baseline := testRepository(t)
	cfg := testConfig(repo, "task-reopen", "create-reopen", baseline)
	first, err := Prepare(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Prepare(context.Background(), testConfig(repo, "task-reopen", "reopen-operation", baseline))
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Reopened || reopened.Workspace.WorkspaceID != first.Workspace.WorkspaceID || reopened.Workspace.HeadSHA != baseline {
		t.Fatalf("reopen = %+v", reopened)
	}
}

func TestPrepareRecoversOwnedCreationWindows(t *testing.T) {
	for _, removeRef := range []bool{false, true} {
		t.Run(map[bool]string{false: "ref exists without registration", true: "marker exists before ref"}[removeRef], func(t *testing.T) {
			repo, baseline := testRepository(t)
			cfg := testConfig(repo, "task-window", "create-window", baseline)
			prepared, err := Prepare(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			runGit(t, repo, "worktree", "remove", prepared.Workspace.ExecutionRoot)
			if removeRef {
				runGit(t, repo, "update-ref", "-d", prepared.Workspace.BranchRef, baseline)
			}
			recovered, err := Prepare(context.Background(), testConfig(repo, "task-window", "recover-window", baseline))
			if err != nil {
				t.Fatal(err)
			}
			if !recovered.Reopened || recovered.Workspace.HeadSHA != baseline {
				t.Fatalf("recovered = %+v", recovered)
			}
		})
	}
}

func TestPrepareRefusesUserOwnedBranchConflict(t *testing.T) {
	repo, baseline := testRepository(t)
	cfg := testConfig(repo, "task-conflict", "prepare-conflict", baseline)
	n, err := normalize(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "branch", strings.TrimPrefix(n.branchRef, "refs/heads/"), baseline)
	_, err = Prepare(context.Background(), cfg)
	if !errors.Is(err, ErrGitConflict) {
		t.Fatalf("Prepare error = %v, want Git conflict", err)
	}
	if got := runGit(t, repo, "rev-parse", "--verify", n.branchRef); got != baseline {
		t.Fatalf("user branch changed to %s", got)
	}
}

func TestPrepareRefusesSymlinkedRuntimeNamespace(t *testing.T) {
	repo, baseline := testRepository(t)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".revolvr", "autonomous")); err != nil {
		t.Fatal(err)
	}
	_, err := Prepare(context.Background(), testConfig(repo, "task-symlink", "prepare-symlink", baseline))
	if !errors.Is(err, ErrUnsafeOwnership) {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestAdminLockRejectsUnsafePathsAndOpenSubstitution(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, root, outside, sentinel, lockPath string)
		afterOpen func(root, path string) error
	}{
		{
			name: "final symlink",
			setup: func(t *testing.T, _, _, sentinel, lockPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(sentinel, lockPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hard link alias",
			setup: func(t *testing.T, _, _, sentinel, lockPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(sentinel, lockPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked ancestor",
			setup: func(t *testing.T, root, outside, _, _ string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(root, ".revolvr"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, ".revolvr", "locks")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "substitution after open",
			afterOpen: func(_, path string) error {
				if err := os.Rename(path, path+".opened"); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("replacement\n"), 0o600)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			sentinel := filepath.Join(outside, "sentinel.txt")
			const sentinelBytes = "outside sentinel\n"
			if err := os.WriteFile(sentinel, []byte(sentinelBytes), 0o600); err != nil {
				t.Fatal(err)
			}
			lockPath := filepath.Join(root, ".revolvr", "locks", "git-admin.lock")
			if tt.setup != nil {
				tt.setup(t, root, outside, sentinel, lockPath)
			}
			unlock, err := acquireAdminLock(context.Background(), root, tt.afterOpen)
			if err == nil {
				unlock()
				t.Fatal("unsafe Git-admin lock path was acquired")
			}
			if !errors.Is(err, runtimepath.ErrUnsafe) {
				t.Fatalf("acquire error = %v, want runtimepath.ErrUnsafe", err)
			}
			raw, readErr := os.ReadFile(sentinel)
			if readErr != nil || string(raw) != sentinelBytes {
				t.Fatalf("outside sentinel changed: err=%v bytes=%q", readErr, raw)
			}
			entries, readErr := os.ReadDir(outside)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != "sentinel.txt" {
				t.Fatalf("outside directory changed: err=%v entries=%v", readErr, entries)
			}
			if tt.afterOpen != nil {
				if raw, readErr := os.ReadFile(lockPath); readErr != nil || string(raw) != "replacement\n" {
					t.Fatalf("replacement changed: err=%v bytes=%q", readErr, raw)
				}
			}
		})
	}
}

func TestPrepareRefusesRegisteredUserWorktreeAtExpectedPath(t *testing.T) {
	repo, baseline := testRepository(t)
	cfg := testConfig(repo, "task-registered", "prepare-registered", baseline)
	n, err := normalize(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "branch", "user-owned", baseline)
	if err := os.MkdirAll(filepath.Dir(n.execution), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "worktree", "add", n.execution, "user-owned")
	_, err = Prepare(context.Background(), cfg)
	if !errors.Is(err, ErrGitConflict) {
		t.Fatalf("registered conflict = %v", err)
	}
	if got := runGit(t, n.execution, "rev-parse", "--abbrev-ref", "HEAD"); got != "user-owned" {
		t.Fatalf("user worktree changed: %s", got)
	}
}

func TestReconcileCommitAndAdvanceCheckpoint(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-checkpoint", "prepare-checkpoint", baseline))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "tracked.txt"), "verified change\n")
	runGit(t, prepared.Workspace.ExecutionRoot, "add", "tracked.txt")
	runGit(t, prepared.Workspace.ExecutionRoot, "commit", "-q", "-m", "verified")
	commit := runGit(t, prepared.Workspace.ExecutionRoot, "rev-parse", "HEAD")

	reconciled, err := ReconcileCommit(context.Background(), testConfig(repo, "task-checkpoint", "reconcile-checkpoint", baseline), prepared.Workspace, commit)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Workspace.Status != autonomous.WorkspaceStatusReady || reconciled.Workspace.HeadSHA != commit || reconciled.Workspace.Checkpoint.CommitSHA != baseline {
		t.Fatalf("reconciled = %+v", reconciled.Workspace)
	}
	advanced, err := AdvanceCheckpoint(context.Background(), testConfig(repo, "task-checkpoint", "checkpoint-two", baseline), reconciled.Workspace, "final verification passed")
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Workspace.Checkpoint.Sequence != 2 || advanced.Workspace.Checkpoint.CommitSHA != commit || advanced.Workspace.Checkpoint.Provenance != "final verification passed" {
		t.Fatalf("advanced checkpoint = %+v", advanced.Workspace.Checkpoint)
	}
}

func TestAdvanceCheckpointRefusesIgnoredVerificationInput(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-ignored-checkpoint", "prepare-checkpoint", baseline))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "tracked.txt"), "verified change\n")
	runGit(t, prepared.Workspace.ExecutionRoot, "add", "tracked.txt")
	runGit(t, prepared.Workspace.ExecutionRoot, "commit", "-q", "-m", "verified")
	commit := runGit(t, prepared.Workspace.ExecutionRoot, "rev-parse", "HEAD")
	reconciled, err := ReconcileCommit(context.Background(), testConfig(repo, "task-ignored-checkpoint", "reconcile-checkpoint", baseline), prepared.Workspace, commit)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "ignored.tmp"), "verification-only input\n")

	advanced, err := AdvanceCheckpoint(context.Background(), testConfig(repo, "task-ignored-checkpoint", "advance-checkpoint", baseline), reconciled.Workspace, "final verification passed")
	if err == nil || !strings.Contains(err.Error(), "ignored.tmp") || !strings.Contains(err.Error(), "classification=policy_relevant") {
		t.Fatalf("AdvanceCheckpoint = %+v, %v, want ignored-input refusal", advanced, err)
	}
	if advanced.Workspace.Checkpoint.Sequence > reconciled.Workspace.Checkpoint.Sequence {
		t.Fatalf("checkpoint advanced despite ignored input: %+v", advanced.Workspace.Checkpoint)
	}
}

func TestReopenRetainsUnreconciledAdvancedHead(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-ambiguous", "prepare-ambiguous", baseline))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "tracked.txt"), "uncertain\n")
	runGit(t, prepared.Workspace.ExecutionRoot, "add", "tracked.txt")
	runGit(t, prepared.Workspace.ExecutionRoot, "commit", "-q", "-m", "uncertain")
	reopened, err := Reopen(context.Background(), testConfig(repo, "task-ambiguous", "reopen-ambiguous", baseline), prepared.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Workspace.Status != autonomous.WorkspaceStatusAmbiguous || len(reopened.Workspace.RetainedRefs) != 1 {
		t.Fatalf("ambiguous reopen = %+v", reopened.Workspace)
	}
	if got := runGit(t, repo, "rev-parse", reopened.Workspace.RetainedRefs[0].Ref); got != reopened.Workspace.HeadSHA {
		t.Fatalf("retained ref = %s, want %s", got, reopened.Workspace.HeadSHA)
	}
}

func TestCleanupRefusesDirtyAndRemovesOnlyExactCleanOwnedWorkspace(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-clean", "prepare-clean", baseline))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "scratch.txt"), "keep\n")
	_, err = Cleanup(context.Background(), testConfig(repo, "task-clean", "cleanup-dirty", baseline), prepared.Workspace)
	if !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("dirty cleanup error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(prepared.Workspace.ExecutionRoot, "scratch.txt")); err != nil {
		t.Fatalf("dirty user file lost: %v", err)
	}
	os.Remove(filepath.Join(prepared.Workspace.ExecutionRoot, "scratch.txt"))
	cleaned, err := Cleanup(context.Background(), testConfig(repo, "task-clean", "cleanup-clean", baseline), prepared.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	if cleaned.Workspace.Status != autonomous.WorkspaceStatusCleaned {
		t.Fatalf("cleanup status = %q", cleaned.Workspace.Status)
	}
	if _, err := os.Stat(prepared.Workspace.ExecutionRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("execution path remains: %v", err)
	}
	if out, err := exec.Command("git", "-C", repo, "show-ref", "--verify", prepared.Workspace.BranchRef).CombinedOutput(); err == nil {
		t.Fatalf("owned branch remains: %s", out)
	}
}

func TestCleanupRefusesActiveWorkspaceWriter(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-busy", "prepare-busy", baseline))
	if err != nil {
		t.Fatal(err)
	}
	writer, err := lock.AcquireSourceWriter(context.Background(), lock.Config{ControlRoot: repo, ExecutionRoot: prepared.Workspace.ExecutionRoot, WorkspaceID: prepared.Workspace.WorkspaceID, RunID: "active-run", PID: 123, Timeout: time.Minute, Clock: func() time.Time { return fixedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Release(context.Background())
	_, err = Cleanup(context.Background(), testConfig(repo, "task-busy", "cleanup-busy", baseline), prepared.Workspace)
	if !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("busy cleanup error = %v", err)
	}
	if _, err := os.Stat(prepared.Workspace.ExecutionRoot); err != nil {
		t.Fatalf("busy workspace removed: %v", err)
	}
}

func TestCleanupRefusesGitLockedWorktree(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-locked", "prepare-locked", baseline))
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "worktree", "lock", prepared.Workspace.ExecutionRoot)
	_, err = Cleanup(context.Background(), testConfig(repo, "task-locked", "cleanup-locked", baseline), prepared.Workspace)
	if !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("locked cleanup error = %v", err)
	}
	if _, err := os.Stat(prepared.Workspace.ExecutionRoot); err != nil {
		t.Fatalf("locked workspace removed: %v", err)
	}
}

func TestCleanupRefusesIgnoredUserFile(t *testing.T) {
	repo, baseline := testRepository(t)
	prepared, err := Prepare(context.Background(), testConfig(repo, "task-ignored", "prepare-ignored", baseline))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "ignored.tmp"), "user data\n")
	_, err = Cleanup(context.Background(), testConfig(repo, "task-ignored", "cleanup-ignored", baseline), prepared.Workspace)
	if !errors.Is(err, ErrCleanupRefused) {
		t.Fatalf("ignored cleanup error = %v", err)
	}
	if got := readFile(t, filepath.Join(prepared.Workspace.ExecutionRoot, "ignored.tmp")); got != "user data\n" {
		t.Fatalf("ignored user data changed: %q", got)
	}
}

func TestTwoTaskWorkspacesAreIsolated(t *testing.T) {
	repo, baseline := testRepository(t)
	one, err := Prepare(context.Background(), testConfig(repo, "task-a", "create-a", baseline))
	if err != nil {
		t.Fatal(err)
	}
	two, err := Prepare(context.Background(), testConfig(repo, "task-b", "create-b", baseline))
	if err != nil {
		t.Fatal(err)
	}
	if one.Workspace.WorkspaceID == two.Workspace.WorkspaceID || one.Workspace.ExecutionRoot == two.Workspace.ExecutionRoot || one.Workspace.BranchRef == two.Workspace.BranchRef {
		t.Fatalf("workspaces collided: %+v %+v", one.Workspace, two.Workspace)
	}
	writeFile(t, filepath.Join(one.Workspace.ExecutionRoot, "tracked.txt"), "task a\n")
	if got := readFile(t, filepath.Join(two.Workspace.ExecutionRoot, "tracked.txt")); got != "baseline\n" {
		t.Fatalf("task A contaminated task B: %q", got)
	}
}

type workspaceSourceSnapshot struct {
	Head        string
	Branch      string
	Status      string
	Tracked     string
	DeleteGuard string
}

func captureWorkspaceSource(t *testing.T, root string) workspaceSourceSnapshot {
	t.Helper()
	return workspaceSourceSnapshot{
		Head:        runGit(t, root, "rev-parse", "HEAD"),
		Branch:      runGit(t, root, "symbolic-ref", "HEAD"),
		Status:      runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"),
		Tracked:     readFile(t, filepath.Join(root, "tracked.txt")),
		DeleteGuard: readFile(t, filepath.Join(root, "deleted.txt")),
	}
}

func assertWorkspaceSource(t *testing.T, root string, before workspaceSourceSnapshot) {
	t.Helper()
	if after := captureWorkspaceSource(t, root); after != before {
		t.Fatalf("workspace source changed on refused authority:\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func testConfig(repo, taskID, operationID, baseline string) Config {
	return Config{ControlRoot: repo, TaskID: taskID, OperationID: operationID, BaselineSHA: baseline, Timeout: 10 * time.Second, StdoutCap: 1 << 20, StderrCap: 1 << 20, Clock: func() time.Time { return fixedTime }}
}

func testRepository(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.name", "Revolvr Test")
	runGit(t, repo, "config", "user.email", "revolvr@example.test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "baseline\n")
	writeFile(t, filepath.Join(repo, "deleted.txt"), "baseline delete guard\n")
	writeFile(t, filepath.Join(repo, ".gitignore"), "ignored.tmp\n")
	runGit(t, repo, "add", "tracked.txt", "deleted.txt", ".gitignore")
	runGit(t, repo, "commit", "-q", "-m", "baseline")
	return repo, runGit(t, repo, "rev-parse", "HEAD")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
