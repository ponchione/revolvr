package verification

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/google/uuid"

	"revolvr/internal/sandbox"
	"revolvr/internal/workspace"
)

type captureSandboxRuntime struct {
	specifications []sandbox.Specification
}

func (r *captureSandboxRuntime) Create(_ context.Context, specification sandbox.Specification) (sandbox.SandboxHandle, error) {
	r.specifications = append(r.specifications, specification)
	return sandbox.SandboxHandle{ID: strings.Repeat("a", 64), Name: "verification-fixture", Command: append([]string(nil), specification.Command...)}, nil
}

func (r *captureSandboxRuntime) Exec(_ context.Context, _ sandbox.SandboxHandle, _ sandbox.CommandSpec) (sandbox.CommandResult, error) {
	return sandbox.CommandResult{ExitCode: 0, Stdout: "ok\n"}, nil
}

func (r *captureSandboxRuntime) Stop(context.Context, sandbox.SandboxHandle) error { return nil }
func (r *captureSandboxRuntime) Inspect(context.Context, sandbox.SandboxHandle) (sandbox.SandboxStatus, error) {
	return sandbox.SandboxStatus{State: "exited", ExitCode: 0}, nil
}
func (r *captureSandboxRuntime) Remove(context.Context, sandbox.SandboxHandle) error { return nil }

func TestSandboxGateExecutorUsesReadOnlyFrozenCandidateAuthority(t *testing.T) {
	pinned := fixturePinned(t, TierFocused)
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(root, "candidate")
	if err := os.Mkdir(workspacePath, 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(workspacePath)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	frozen := workspace.Workspace{
		ID: pinned.WorkspaceID, RunID: pinned.RunID, ProjectID: pinned.ProjectID, TaskID: pinned.TaskID,
		SymbolicSourceID: "verification-candidate", Status: workspace.StatusFrozen,
		OriginalCheckoutPath: filepath.Join(root, "operator"), ManagedRepositoryPath: filepath.Join(root, "managed"),
		WorkspaceRoot: root, Path: workspacePath, Device: uint64(stat.Dev), Inode: stat.Ino,
		CandidateCommit: pinned.Candidate.Commit, CandidateTree: pinned.Candidate.Tree,
	}
	runtime := &captureSandboxRuntime{}
	manager, err := sandbox.NewManager(runtime, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewSandboxGateExecutor(manager, frozen)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), GateExecution{SandboxID: uuid.NewString(), Pinned: pinned, Gate: pinned.Plan.Gates[0]})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || string(result.Stdout) != "ok\n" || len(runtime.specifications) != 1 {
		t.Fatalf("execution = %#v, specifications %d", result, len(runtime.specifications))
	}
	specification := runtime.specifications[0]
	if specification.Role != sandbox.RoleVerifier || specification.Network != sandbox.NetworkNone || specification.WorkingDirectory != pinned.Plan.Gates[0].WorkingDirectory || len(specification.Mounts) != 1 || specification.Mounts[0].Mode != sandbox.MountReadOnly || specification.Mounts[0].Target != "/workspace" {
		t.Fatalf("verifier sandbox authority = %#v", specification)
	}
}

func TestFrozenWorkspaceObserverDetectsPostFreezeMutation(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "-q", "-b", "main")
	runGit(t, repository, "config", "user.name", "Verification Test")
	runGit(t, repository, "config", "user.email", "verification@example.invalid")
	contract := []byte("module fixture\n\ngo 1.26.5\n")
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), contract, 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "go.mod")
	runGit(t, repository, "commit", "-q", "-m", "fixture")
	commit := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD^{tree}"))
	info, err := os.Stat(repository)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	environment := ProjectEnvironment{SHA256: strings.Repeat("9", 64)}
	observer, err := NewFrozenWorkspaceObserver(workspace.Workspace{
		Status: workspace.StatusFrozen, Path: repository, WorkspaceRoot: filepath.Dir(repository),
		Device: uint64(stat.Dev), Inode: stat.Ino, SymbolicSourceID: "observer-fixture",
		CandidateCommit: commit, CandidateTree: tree,
	}, environment, "git")
	if err != nil {
		t.Fatal(err)
	}
	gate := Gate{Source: SourceIdentity{Commit: commit, Tree: tree}, AuthorityInputs: []MaterialInput{{Kind: "project-contract", Path: "go.mod", SHA256: hashBytes(contract), SizeBytes: int64(len(contract))}}}
	snapshot, err := observer.Observe(context.Background(), gate)
	if err != nil || snapshot.Source != gate.Source || snapshot.ProjectEnvironmentSHA256 != environment.SHA256 || len(snapshot.AuthorityInputs) != 1 || snapshot.AuthorityInputs[0] != gate.AuthorityInputs[0] {
		t.Fatalf("initial authority = %#v, %v", snapshot, err)
	}
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), append(contract, []byte("// changed\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := observer.Observe(context.Background(), gate); !errors.Is(err, ErrStaleSource) {
		t.Fatalf("post-freeze mutation error = %v", err)
	}
}

func runGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	raw, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, raw)
	}
	return string(raw)
}
