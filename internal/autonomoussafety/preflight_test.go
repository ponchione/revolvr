package autonomoussafety

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/runner"
)

func TestFullyUnattendedRequiresPolicyBoundAcknowledgement(t *testing.T) {
	in := testInput(t)
	in.Declaration.Mode = ModeFullyUnattended
	in.Declaration.ExternalIsolation = ExternalIsolation{Expectation: IsolationContainer, Enforcement: EnforcementExternalAttestation, Attestation: testAttestation()}
	in.Declaration.Network = NetworkPolicy{Access: NetworkDenied, Enforcement: EnforcementExternalAttestation, Attestation: testAttestation()}
	in.Declaration.Hooks.Policy = HooksDisabled
	in.Declaration.Environment.InheritHost = false
	first, err := Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Preflight.Ready || first.Policy.PolicySHA256 == "" {
		t.Fatalf("first = %+v", first.Preflight)
	}
	in.Declaration.Acknowledgement = first.Policy.ExpectedAcknowledgement()
	second, err := Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Preflight.Ready {
		t.Fatalf("checks = %+v", second.Preflight.Checks)
	}
	in.Declaration.Network.Access = NetworkRestricted
	stale, _ := Run(context.Background(), in)
	if stale.Preflight.Ready || stale.Policy.PolicySHA256 == second.Policy.PolicySHA256 {
		t.Fatalf("stale acknowledgement accepted: %+v", stale.Preflight)
	}
}

func TestProtectedPathsRejectModelAuthorityExpansion(t *testing.T) {
	out, err := Run(context.Background(), testInput(t))
	if err != nil || !out.Preflight.Ready {
		t.Fatalf("Run = %+v, %v", out.Preflight, err)
	}
	for _, path := range []string{".agent/tasks/task.md", ".agent/profiles/implementer.md", "AGENTS.md", ".git"} {
		if err := out.Policy.AuthorizeModelChanges([]string{path}); err == nil {
			t.Fatalf("protected %q accepted", path)
		}
	}
	if err := out.Policy.AuthorizeModelChanges([]string{"internal/example.go"}); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightRejectsHookAndCommandDrift(t *testing.T) {
	in := testInput(t)
	hooks := filepath.Join(in.Workspace.GitCommonDir, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	in.Declaration.Hooks.Policy = HooksDisabled
	out, err := Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Preflight.Ready || !hasFailed(out.Preflight, "git_hooks") {
		t.Fatalf("checks = %+v", out.Preflight.Checks)
	}

	in = testInput(t)
	in.LookPath = func(string) (string, error) { return filepath.Join(t.TempDir(), "missing"), nil }
	out, _ = Run(context.Background(), in)
	if out.Preflight.Ready || !hasFailed(out.Preflight, "command_provenance") {
		t.Fatalf("checks = %+v", out.Preflight.Checks)
	}
}

func TestPreflightRejectsSymlinkedWritableRoot(t *testing.T) {
	in := testInput(t)
	runs := filepath.Join(in.Workspace.ControlRoot, ".revolvr", "runs")
	if err := os.Remove(runs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), runs); err != nil {
		t.Fatal(err)
	}
	out, err := Run(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Preflight.Ready || !hasFailed(out.Preflight, "writable_roots_and_protected_paths") {
		t.Fatalf("checks = %+v", out.Preflight.Checks)
	}
}

func TestDeclarationUnknownValuesFailStrictly(t *testing.T) {
	d := DefaultDeclaration()
	d.Mode = "future"
	if err := d.Validate(); err == nil {
		t.Fatal("unknown mode accepted")
	}
	d = DefaultDeclaration()
	d.Network.Access = "future"
	if err := d.Validate(); err == nil {
		t.Fatal("unknown network access accepted")
	}
	d = DefaultDeclaration()
	d.Hooks.Policy = "future"
	if err := d.Validate(); err == nil {
		t.Fatal("unknown hook policy accepted")
	}
}

func testInput(t *testing.T) Input {
	t.Helper()
	root := t.TempDir()
	execution := filepath.Join(root, ".revolvr", "autonomous", "worktrees", "workspace-one")
	common := filepath.Join(root, ".git")
	for _, dir := range []string{execution, common, filepath.Join(root, ".revolvr", "runs"), filepath.Join(root, ".revolvr", "receipts"), filepath.Join(root, ".revolvr", "locks"), filepath.Join(root, ".revolvr", "autonomous", "tasks", "task-1")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(execution, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	revision := strings.Repeat("a", 64)
	workspace := autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: "task-1", WorkspaceID: "workspace-one", ControlRoot: root, ExecutionRoot: execution, GitCommonDir: common, BranchRef: "refs/heads/revolvr/tasks/task-1-workspace", OwnerMarker: filepath.Join(root, ".revolvr", "autonomous", "tasks", "task-1", "workspace-owner.json"), BaselineSHA: strings.Repeat("1", 40), HeadSHA: strings.Repeat("1", 40), TreeSHA: strings.Repeat("2", 40), SourceRevision: revision, Checkpoint: autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: strings.Repeat("1", 40), TreeSHA: strings.Repeat("2", 40), SourceRevision: revision, OperationID: "create", Provenance: "test", CreatedAt: now}, Status: autonomous.WorkspaceStatusReady, CreatedAt: now, UpdatedAt: now}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Input{TaskID: "task-1", Workspace: workspace, SourceRevision: revision, ObservedHead: workspace.HeadSHA, Declaration: DefaultDeclaration(), Codex: CodexPolicy{Sandbox: "workspace-write", ApprovalPolicy: "never", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", Ephemeral: true}, Commands: []CommandSpec{{Kind: "codex", Executable: executable, WorkingDir: execution, Timeout: time.Second, StdoutCap: 1024, StderrCap: 1024}}, ConfigPath: filepath.Join(root, ".revolvr", "config.yaml"), ConfigSHA256: strings.Repeat("b", 64), ObservedAt: now, LookupEnv: func(string) (string, bool) { return "", false }, LookPath: func(name string) (string, error) { return name, nil }, GitExecutable: "git", GitTimeout: time.Second, GitStdoutCap: 1024, GitStderrCap: 1024, CommandRunner: func(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 1} }}
}

func testAttestation() *Attestation {
	return &Attestation{Authority: "operator", Evidence: "container policy record", SHA256: strings.Repeat("c", 64)}
}
func hasFailed(result PreflightResult, name string) bool {
	for _, check := range result.Checks {
		if check.Name == name && check.Status == CheckFail {
			return true
		}
	}
	return false
}
