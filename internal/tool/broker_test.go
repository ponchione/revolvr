package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"revolvr/internal/sandbox"
)

type fakeSandboxExecutor struct {
	root          string
	executions    atomic.Int64
	cancels       atomic.Int64
	block         bool
	commandStdout []byte
	specification sandbox.Specification
}

type scriptedSequencer struct {
	grants []func(SequenceRequest) SequenceGrant
	next   int
}

func (s *scriptedSequencer) Next(_ context.Context, request SequenceRequest) (SequenceGrant, error) {
	if s.next >= len(s.grants) {
		return SequenceGrant{}, errors.New("unexpected sequence request")
	}
	grant := s.grants[s.next](request)
	s.next++
	return grant, nil
}

func trustedSequence(sequence int64) func(SequenceRequest) SequenceGrant {
	return func(request SequenceRequest) SequenceGrant {
		return SequenceGrant{
			Sequence: sequence, RuntimeKind: request.RuntimeKind, RunID: request.RunID,
			RequestSHA256: request.RequestSHA256, Trusted: true,
		}
	}
}

type capturingRuntimeHandler struct {
	kind     RuntimeKind
	result   ExecutionResult
	request  RuntimeExecutionRequest
	executes atomic.Int64
	cancels  atomic.Int64
}

func (h *capturingRuntimeHandler) Kind() RuntimeKind { return h.kind }

func (h *capturingRuntimeHandler) Execute(_ context.Context, request RuntimeExecutionRequest) (ExecutionResult, error) {
	h.executes.Add(1)
	h.request = request
	return h.result, nil
}

func (h *capturingRuntimeHandler) Cancel(context.Context, string) error {
	h.cancels.Add(1)
	return nil
}

func (f *fakeSandboxExecutor) Execute(ctx context.Context, specification sandbox.Specification, operation Operation) (ExecutionResult, error) {
	f.executions.Add(1)
	f.specification = specification
	if f.block {
		<-ctx.Done()
		return ExecutionResult{ExitCode: -1, Cancelled: true, Stdout: []byte("partial")}, ctx.Err()
	}
	switch operation.Tool {
	case ToolSourceEdit:
		argument := operation.SourceEdit
		path := filepath.Join(f.root, filepath.FromSlash(argument.Path))
		before, err := os.ReadFile(path)
		beforeIdentity := ""
		if errors.Is(err, os.ErrNotExist) && argument.ExpectedSHA256 == "absent" {
			before = nil
			beforeIdentity = "absent"
		} else if err != nil {
			return ExecutionResult{ExitCode: 1}, err
		} else if digest(before) != argument.ExpectedSHA256 {
			return ExecutionResult{ExitCode: 1}, errors.New("stale edit input")
		} else {
			beforeIdentity = digest(before)
		}
		if err := os.WriteFile(path, []byte(argument.Content), 0o644); err != nil {
			return ExecutionResult{ExitCode: 1}, err
		}
		after := digest([]byte(argument.Content))
		return ExecutionResult{
			ExitCode: 0, SourceChanges: []SourceChange{{Path: argument.Path, BeforeSHA256: beforeIdentity, AfterSHA256: after}},
			Effect: EffectProof{Proven: true, Kind: "source_edit", Identity: argument.Path, BeforeSHA256: beforeIdentity, AfterSHA256: after},
		}, nil
	case ToolFileRead:
		raw, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(operation.FileRead.Path)))
		if err != nil {
			return ExecutionResult{ExitCode: 1}, err
		}
		start := operation.FileRead.Offset
		if start > int64(len(raw)) {
			start = int64(len(raw))
		}
		end := min(int64(len(raw)), start+operation.FileRead.MaxBytes)
		return ExecutionResult{ExitCode: 0, Stdout: append([]byte(nil), raw[start:end]...)}, nil
	case ToolTextSearch:
		return ExecutionResult{ExitCode: 0, Stdout: []byte("src/value.txt:1:match\n")}, nil
	case ToolCommand:
		stdout := f.commandStdout
		if stdout == nil {
			stdout = []byte("ok\n")
		}
		return ExecutionResult{ExitCode: 0, Stdout: append([]byte(nil), stdout...)}, nil
	default:
		return ExecutionResult{ExitCode: 1}, errors.New("unexpected operation")
	}
}

func (f *fakeSandboxExecutor) Cancel(context.Context, string) error {
	f.cancels.Add(1)
	return nil
}

type brokerFixture struct {
	broker    *Broker
	policy    Policy
	executor  *fakeSandboxExecutor
	workspace string
	store     *FileStore
}

func newBrokerFixture(t *testing.T) brokerFixture {
	t.Helper()
	managed := t.TempDir()
	if err := os.Chmod(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(managed, "workspace-1")
	if err := os.Mkdir(workspace, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"src", "tests"} {
		if err := os.Mkdir(filepath.Join(workspace, directory), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "src", "value.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := sandbox.Request{
		SchemaVersion: sandbox.RequestSchemaVersion, SandboxID: "sandbox-1", ProjectID: "project-1",
		TaskID: "task-1", RunID: "run-1", Role: sandbox.RoleImplementer,
		Image:          sandbox.Image{Reference: "worker:1", Digest: "sha256:" + strings.Repeat("a", 64)},
		RuntimeProfile: sandbox.ProfileStrict, Command: []string{"/usr/local/bin/revolvr-worker"},
		Mounts:      []sandbox.Mount{{SourceID: "workspace-source", Target: "/workspace", Mode: sandbox.MountReadWrite}},
		Network:     sandbox.NetworkNone,
		Resources:   sandbox.Resources{CPUs: 2, MemoryBytes: 1 << 30, PIDs: 64, TimeoutSeconds: 60, TmpfsBytes: 64 << 20},
		Environment: map[string]string{"TASK_ID": "task-1", "RUN_ID": "run-1", "ROLE": "implementer", "SAFE_FLAG": "yes"},
	}
	specification, err := sandbox.Validate(request, sandbox.Policy{
		ProjectID: "project-1", TaskID: "task-1", RunID: "run-1", Role: sandbox.RoleImplementer,
		ApprovedImages: []sandbox.Image{request.Image}, AllowedProfiles: []sandbox.RuntimeProfile{sandbox.ProfileStrict},
		AllowedNetworks:         []sandbox.NetworkProfile{sandbox.NetworkNone},
		AllowedEnvironmentNames: []string{"TASK_ID", "RUN_ID", "ROLE", "SAFE_FLAG"},
		ManagedSources:          []sandbox.ManagedSource{{ID: "workspace-source", Root: managed, RelativePath: "workspace-1", Kind: sandbox.SourceWorkspace, Type: sandbox.SourceDirectory, Target: "/workspace"}},
		MaximumResources:        request.Resources,
	})
	if err != nil {
		t.Fatal(err)
	}
	mount, ok := workspaceMount(specification)
	if !ok {
		t.Fatal("workspace mount missing")
	}
	authority := Authority{
		ProjectID: "project-1", TaskID: "task-1", TaskVersionID: "task-version-1", RunID: "run-1",
		SourceRevision: strings.Repeat("b", 64), SourceCommit: strings.Repeat("c", 40), SourceTree: strings.Repeat("d", 40),
		PlanID: "plan-1", PlanVersionID: "plan-version-1", PlanRevision: 1,
		StepBatchSHA256: strings.Repeat("f", 64), StepIDs: []string{"step-1"},
		WorkspaceID: "workspace-1", SandboxID: "sandbox-1",
	}
	policy, err := PinPolicy(PolicySettings{
		Authority: authority, Role: sandbox.RoleImplementer, WorkspaceRoot: workspace,
		WorkspaceDevice: mount.SourceDevice, WorkspaceInode: mount.SourceInode, Sandbox: specification,
		ExpectedPaths: []string{"src"}, AdjacentPaths: []string{"tests"}, ProtectedPaths: []string{"src/protected.txt"},
		DependencyPaths: []string{"go.mod", "go.sum"}, VerificationAuthorityPaths: []string{"tests"},
		AllowedCommands: [][]string{{"go", "test", "./..."}}, AllowedEnvironmentNames: []string{"SAFE_FLAG"},
		Network: sandbox.NetworkNone, MaximumTimeout: 30 * time.Second, MaximumCPUs: 2,
		MaximumMemoryBytes: 1 << 30, MaximumPIDs: 64, MaximumTmpfsBytes: 64 << 20,
		MaximumStdoutBytes: 1024, MaximumStderrBytes: 1024, MaximumReadBytes: 1024,
		MaximumEditBytes: 4096, MaximumSearchResults: 20, HooksDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceRoot := t.TempDir()
	if err := os.Chmod(evidenceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(evidenceRoot)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeSandboxExecutor{root: workspace}
	broker, err := NewBroker(policy, executor, store)
	if err != nil {
		t.Fatal(err)
	}
	return brokerFixture{broker: broker, policy: policy, executor: executor, workspace: workspace, store: store}
}

func TestRegistryIsClosedVersionedAndRoleScoped(t *testing.T) {
	implementer, err := RegistryForRole(sandbox.RoleImplementer)
	if err != nil {
		t.Fatal(err)
	}
	if implementer.Version != RegistryVersion || len(implementer.Definitions) != 4 || implementer.SHA256 == "" {
		t.Fatalf("implementer registry = %+v", implementer)
	}
	for _, definition := range implementer.Definitions {
		if !strings.Contains(string(definition.InputSchema), `"additionalProperties":false`) {
			t.Fatalf("schema for %s is not closed: %s", definition.Name, definition.InputSchema)
		}
	}
	verifier, err := RegistryForRole(sandbox.RoleVerifier)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range verifier.Definitions {
		if definition.MutatesSource || definition.Capability == CapabilityWrite || definition.Name == ToolSourceEdit {
			t.Fatalf("verifier received mutation capability: %+v", definition)
		}
	}
	if _, err := RegistryForRole(sandbox.Role("auditor")); err == nil {
		t.Fatal("unknown executable role received a registry")
	}
}

func TestRuntimeKindIsClosedBeforeDispatchOrReplay(t *testing.T) {
	fixture := newBrokerFixture(t)
	call := makeCall(t, fixture.broker, "runtime-1", ToolFileRead, FileReadArguments{Path: "src/value.txt", MaxBytes: 10})
	if _, err := fixture.broker.DispatchRuntime(context.Background(), RuntimeKind("reserved_runtime_v1"), call); err == nil {
		t.Fatal("reserved runtime kind was admitted")
	}
	if fixture.executor.executions.Load() != 0 {
		t.Fatal("reserved runtime kind reached the executor")
	}
	if _, err := fixture.broker.Dispatch(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.broker.DispatchRuntime(context.Background(), RuntimeKind("reserved_runtime_v1"), call); err == nil {
		t.Fatal("reserved runtime kind accepted a direct-tools replay")
	}
	if fixture.executor.executions.Load() != 1 {
		t.Fatal("reserved runtime replay repeated or altered the effect count")
	}
	unknown := &capturingRuntimeHandler{kind: RuntimeKind("reserved_runtime_v1")}
	if _, err := NewBrokerWithRuntime(fixture.policy, unknown, fixture.store, &localSequencer{}); err == nil {
		t.Fatal("reserved runtime handler was installed")
	}
}

func TestResultRepresentationIsExclusiveBoundedAndStorageIndependent(t *testing.T) {
	raw := []byte(`{"status":"completed"}` + "\n")
	artifact := artifactWithMediaType("content-addressed/result", raw, resultMediaType)
	inline := representResult(raw, artifact, len(raw), 0)
	reference := representResult(raw, artifact, 0, 0)
	if inline.Kind != ResultRepresentationInline || reference.Kind != ResultRepresentationArtifact || inline.SHA256 != reference.SHA256 || inline.SHA256 != digest(raw) {
		t.Fatalf("storage-form-independent identities differ: inline=%+v artifact=%+v", inline, reference)
	}
	if err := validateResultRepresentation(inline, digest(raw)); err != nil {
		t.Fatal(err)
	}
	if err := validateResultRepresentation(reference, digest(raw)); err != nil {
		t.Fatal(err)
	}
	mismatch := inline
	mismatch.Inline = &InlineResult{MediaType: inline.Inline.MediaType, Content: inline.Inline.Content, SHA256: strings.Repeat("0", 64), SizeBytes: inline.Inline.SizeBytes}
	if err := validateResultRepresentation(mismatch, mismatch.SHA256); err == nil {
		t.Fatal("inline content/hash mismatch was accepted")
	}
	mutable := reference
	mutable.Artifacts = append([]ArtifactReference(nil), reference.Artifacts...)
	mutable.Artifacts[0].Immutable = false
	if err := validateResultRepresentation(mutable, mutable.SHA256); err == nil {
		t.Fatal("mutable artifact result was accepted")
	}
	truncated := representResult(raw, artifact, len(raw), 7)
	if !truncated.Truncated || truncated.TruncatedBytes != 7 || truncated.Resolution != "bounded_inline_truncated" {
		t.Fatalf("truncation resolution = %+v", truncated)
	}
}

func TestTrustedHostTrajectoryOrderingRejectsMissingDuplicateStaleAndUntrusted(t *testing.T) {
	tests := []struct {
		name   string
		grants []func(SequenceRequest) SequenceGrant
		calls  int
	}{
		{"missing", []func(SequenceRequest) SequenceGrant{trustedSequence(0)}, 1},
		{"untrusted", []func(SequenceRequest) SequenceGrant{func(request SequenceRequest) SequenceGrant {
			grant := trustedSequence(1)(request)
			grant.Trusted = false
			return grant
		}}, 1},
		{"wrong authority", []func(SequenceRequest) SequenceGrant{func(request SequenceRequest) SequenceGrant {
			grant := trustedSequence(1)(request)
			grant.RunID = "foreign-run"
			return grant
		}}, 1},
		{"duplicate", []func(SequenceRequest) SequenceGrant{trustedSequence(1), trustedSequence(1)}, 2},
		{"stale", []func(SequenceRequest) SequenceGrant{trustedSequence(2), trustedSequence(1)}, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBrokerFixture(t)
			broker, err := NewBrokerWithRuntime(fixture.policy, directRuntimeHandler{executor: fixture.executor}, fixture.store, &scriptedSequencer{grants: test.grants})
			if err != nil {
				t.Fatal(err)
			}
			for index := 0; index < test.calls-1; index++ {
				call := makeCall(t, broker, "ordered-first", ToolFileRead, FileReadArguments{Path: "src/value.txt", MaxBytes: 10})
				if _, err := broker.Dispatch(context.Background(), call); err != nil {
					t.Fatalf("first ordered call: %v", err)
				}
			}
			call := makeCall(t, broker, "ordered-refused", ToolFileRead, FileReadArguments{Path: "src/value.txt", MaxBytes: 10})
			if _, err := broker.Dispatch(context.Background(), call); err == nil {
				t.Fatal("invalid trajectory sequence was admitted")
			}
			wantExecutions := int64(test.calls - 1)
			if fixture.executor.executions.Load() != wantExecutions {
				t.Fatalf("executor calls = %d, want %d", fixture.executor.executions.Load(), wantExecutions)
			}
		})
	}
}

func TestInternalRuntimeHandlerUsesTheSameNormalizedEvidenceBoundary(t *testing.T) {
	fixture := newBrokerFixture(t)
	changes := make([]SourceChange, 600)
	for index := range changes {
		changes[index] = SourceChange{Path: "src/generated-" + strings.Repeat("x", 24), BeforeSHA256: "absent", AfterSHA256: strings.Repeat("a", 64)}
	}
	handler := &capturingRuntimeHandler{
		kind:   RuntimeDirectToolsV1,
		result: ExecutionResult{ExitCode: 0, Stdout: []byte("handler output\n"), SourceChanges: changes, Effect: EffectProof{Proven: true, Kind: "command", Identity: "handler-effect"}},
	}
	broker, err := NewBrokerWithRuntime(fixture.policy, handler, fixture.store, &scriptedSequencer{grants: []func(SequenceRequest) SequenceGrant{trustedSequence(41)}})
	if err != nil {
		t.Fatal(err)
	}
	call := makeCall(t, broker, "handler-1", ToolCommand, CommandArguments{
		Argv: []string{"go", "test", "./..."}, WorkingDirectory: "/workspace", EnvironmentNames: []string{"SAFE_FLAG"},
		Network: sandbox.NetworkNone, TimeoutMilliseconds: 1000, CPUs: 1, MemoryBytes: 1 << 20, PIDs: 2,
		TmpfsBytes: 1 << 20, StdoutCapBytes: 100, StderrCapBytes: 100,
	})
	outcome, err := broker.Dispatch(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if handler.executes.Load() != 1 || handler.request.RuntimeKind != RuntimeDirectToolsV1 || handler.request.TrajectorySequence != 41 || handler.request.RequestSHA256 != digest(call) || handler.request.Call.Authority.RunID != fixture.policy.Authority.RunID {
		t.Fatalf("handler request lost broker authority: %+v", handler.request)
	}
	if handler.request.Sandbox.Image != fixture.policy.sandbox.Image || handler.request.Sandbox.RuntimeProfile != fixture.policy.sandbox.RuntimeProfile || handler.request.Sandbox.Resources != fixture.policy.sandbox.Resources {
		t.Fatalf("handler request lost image/profile/resource evidence: %+v", handler.request.Sandbox)
	}
	if outcome.Evidence.RuntimeKind != RuntimeDirectToolsV1 || outcome.Evidence.TrajectorySequence != 41 || outcome.Evidence.RequestSHA256 != digest(call) || outcome.Evidence.ResultSHA256 == "" || outcome.Evidence.ResultRepresentation.Kind != ResultRepresentationArtifact || outcome.Evidence.ResultRepresentation.Artifacts[0].Artifact.SHA256 != outcome.Evidence.ResultSHA256 {
		t.Fatalf("normalized handler evidence = %+v", outcome.Evidence)
	}
	if outcome.Evidence.Runtime.Image != fixture.policy.sandbox.Image || outcome.Evidence.Runtime.Profile != fixture.policy.sandbox.RuntimeProfile || outcome.Evidence.Runtime.Resources != fixture.policy.sandbox.Resources {
		t.Fatalf("normalized runtime evidence = %+v", outcome.Evidence.Runtime)
	}
	if err := validateResultRepresentation(outcome.Evidence.ResultRepresentation, outcome.Evidence.ResultSHA256); err != nil {
		t.Fatal(err)
	}
}

func TestBrokeredEditPersistsExactEvidenceAndReplayDoesNotRepeatEffect(t *testing.T) {
	fixture := newBrokerFixture(t)
	before := []byte("before\n")
	call := makeCall(t, fixture.broker, "edit-1", ToolSourceEdit, SourceEditArguments{
		Path: "src/value.txt", ExpectedSHA256: digest(before), Content: "after\n",
	})
	outcome, err := fixture.broker.Dispatch(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Evidence.Disposition != "completed" || !outcome.Evidence.Effect.Proven || fixture.executor.executions.Load() != 1 {
		t.Fatalf("first outcome = %+v executions=%d", outcome, fixture.executor.executions.Load())
	}
	if outcome.Evidence.RuntimeKind != RuntimeDirectToolsV1 || outcome.Evidence.TrajectorySequence != 1 || outcome.TrajectorySequence != 1 || outcome.Evidence.RequestSHA256 != digest(call) || outcome.Evidence.ResultSHA256 == "" {
		t.Fatalf("missing runtime, ordering, or request/result hashes: %+v", outcome)
	}
	if err := validateResultRepresentation(outcome.Evidence.ResultRepresentation, outcome.Evidence.ResultSHA256); err != nil {
		t.Fatal(err)
	}
	sandboxSHA, sandboxErr := fixture.executor.specification.SHA256()
	workspace, mounted := workspaceMount(fixture.executor.specification)
	if sandboxErr != nil || sandboxSHA != fixture.policy.SandboxSHA256 || !mounted || workspace.SourcePath != fixture.workspace {
		t.Fatalf("executor sandbox boundary = %+v sha=%s err=%v", fixture.executor.specification, sandboxSHA, sandboxErr)
	}
	if raw, err := os.ReadFile(outcome.Evidence.Input.Path); err != nil || !reflect.DeepEqual(raw, call) {
		t.Fatalf("exact input artifact mismatch: %v", err)
	}
	if raw, err := os.ReadFile(outcome.Evidence.Result.Path); err != nil || !strings.Contains(string(raw), `"after_sha256"`) {
		t.Fatalf("result artifact missing effect evidence: %v %s", err, raw)
	} else if digest(raw) != outcome.Evidence.ResultSHA256 {
		t.Fatalf("explicit result hash %s does not match stored bytes %s", outcome.Evidence.ResultSHA256, digest(raw))
	}
	replayed, err := fixture.broker.Dispatch(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Evidence.Replayed || replayed.TrajectorySequence != 2 || replayed.ReplayedFromSequence != 1 || replayed.Evidence.TrajectorySequence != 1 || replayed.Evidence.ResultSHA256 != outcome.Evidence.ResultSHA256 || fixture.executor.executions.Load() != 1 {
		t.Fatalf("replay repeated effect: %+v executions=%d", replayed, fixture.executor.executions.Load())
	}
	if raw, _ := os.ReadFile(filepath.Join(fixture.workspace, "src", "value.txt")); string(raw) != "after\n" {
		t.Fatalf("edited bytes = %q", raw)
	}
}

func TestBrokerDeniesInvalidCallsBeforeExecutorEffects(t *testing.T) {
	fixture := newBrokerFixture(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.workspace, "src", "link")); err != nil {
		t.Fatal(err)
	}
	baseCommand := CommandArguments{Argv: []string{"go", "test", "./..."}, WorkingDirectory: "/workspace", EnvironmentNames: []string{"SAFE_FLAG"}, Network: sandbox.NetworkNone, TimeoutMilliseconds: 1000, CPUs: 1, MemoryBytes: 1 << 20, PIDs: 2, TmpfsBytes: 1 << 20, StdoutCapBytes: 100, StderrCapBytes: 100}
	tests := []struct {
		name string
		raw  func(string) []byte
		code string
	}{
		{"unknown tool", func(id string) []byte { return makeCall(t, fixture.broker, id, "shell", map[string]any{}) }, "unknown_tool"},
		{"malformed unknown field", func(id string) []byte {
			raw := makeCall(t, fixture.broker, id, ToolFileRead, FileReadArguments{Path: "src/value.txt", MaxBytes: 10})
			return []byte(strings.Replace(string(raw), `"max_bytes":10`, `"max_bytes":10,"extra":true`, 1))
		}, "malformed_arguments"},
		{"traversal", func(id string) []byte {
			return makeCall(t, fixture.broker, id, ToolFileRead, FileReadArguments{Path: "../outside", MaxBytes: 10})
		}, "unsafe_path"},
		{"symlink", func(id string) []byte {
			return makeCall(t, fixture.broker, id, ToolFileRead, FileReadArguments{Path: "src/link", MaxBytes: 10})
		}, "symlink_denied"},
		{"protected", func(id string) []byte {
			return makeCall(t, fixture.broker, id, ToolSourceEdit, SourceEditArguments{Path: "src/protected.txt", ExpectedSHA256: "absent", Content: "x"})
		}, "protected_path"},
		{"secret path", func(id string) []byte {
			return makeCall(t, fixture.broker, id, ToolFileRead, FileReadArguments{Path: ".env", MaxBytes: 10})
		}, "secret_or_control_path"},
		{"runtime socket", func(id string) []byte {
			return makeCall(t, fixture.broker, id, ToolFileRead, FileReadArguments{Path: "src/docker.sock", MaxBytes: 10})
		}, "secret_or_control_path"},
		{"wrong cwd", func(id string) []byte {
			value := baseCommand
			value.WorkingDirectory = "/tmp"
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "working_directory_denied"},
		{"network", func(id string) []byte {
			value := baseCommand
			value.Network = sandbox.NetworkOpen
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "network_denied"},
		{"timeout", func(id string) []byte {
			value := baseCommand
			value.TimeoutMilliseconds = 30001
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "resource_limit"},
		{"output", func(id string) []byte {
			value := baseCommand
			value.StdoutCapBytes = 1025
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "resource_limit"},
		{"cpu", func(id string) []byte {
			value := baseCommand
			value.CPUs = 3
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "resource_limit"},
		{"secret environment", func(id string) []byte {
			value := baseCommand
			value.EnvironmentNames = []string{"OPENAI_API_KEY"}
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "environment_denied"},
		{"raw shell", func(id string) []byte {
			value := baseCommand
			value.Argv = []string{"sh", "-c", "cat /etc/passwd"}
			return makeCall(t, fixture.broker, id, ToolCommand, value)
		}, "command_denied"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, err := fixture.broker.Dispatch(context.Background(), test.raw("deny-"+string(rune('a'+index))))
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Evidence.Disposition != "denied" || outcome.Evidence.DenialCode != test.code {
				t.Fatalf("outcome = %+v, want denial %s", outcome, test.code)
			}
			if outcome.Evidence.RequestSHA256 == "" || outcome.Evidence.ResultSHA256 == "" {
				t.Fatalf("denial omitted explicit request/result hashes: %+v", outcome.Evidence)
			}
			if err := validateResultRepresentation(outcome.Evidence.ResultRepresentation, outcome.Evidence.ResultSHA256); err != nil {
				t.Fatalf("denial result representation: %v", err)
			}
			if _, err := os.Stat(outcome.Evidence.Result.Path); err != nil {
				t.Fatalf("denial was not persisted: %v", err)
			}
		})
	}
	for index, mutate := range []func(*Authority){
		func(a *Authority) { a.RunID = "stale-run" }, func(a *Authority) { a.WorkspaceID = "stale-workspace" },
		func(a *Authority) { a.PlanVersionID = "stale-plan" }, func(a *Authority) { a.SourceRevision = strings.Repeat("e", 64) },
		func(a *Authority) { a.StepBatchSHA256 = strings.Repeat("e", 64) },
		func(a *Authority) { a.SandboxID = "stale-sandbox" }, func(a *Authority) { a.HostPolicySHA256 = strings.Repeat("e", 64) },
	} {
		call := Call{SchemaVersion: CallSchemaVersion, CallID: "stale-" + string(rune('a'+index)), Tool: ToolFileRead, Authority: fixture.broker.Authority()}
		mutate(&call.Authority)
		call.Arguments, _ = json.Marshal(FileReadArguments{Path: "src/value.txt", MaxBytes: 10})
		raw, _ := json.Marshal(call)
		outcome, err := fixture.broker.Dispatch(context.Background(), raw)
		if err != nil || outcome.Evidence.DenialCode != "stale_authority" {
			t.Fatalf("stale authority outcome = %+v err=%v", outcome, err)
		}
	}
	if fixture.executor.executions.Load() != 0 {
		t.Fatalf("invalid calls reached executor %d times", fixture.executor.executions.Load())
	}
}

func TestReplacedWorkspaceIdentityIsDeniedBeforeEffect(t *testing.T) {
	fixture := newBrokerFixture(t)
	displaced := fixture.workspace + "-displaced"
	if err := os.Rename(fixture.workspace, displaced); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(fixture.workspace, 0o750); err != nil {
		t.Fatal(err)
	}
	call := makeCall(t, fixture.broker, "replaced-workspace", ToolFileRead, FileReadArguments{Path: "src/value.txt", MaxBytes: 10})
	outcome, err := fixture.broker.Dispatch(context.Background(), call)
	if err != nil || outcome.Evidence.DenialCode != "stale_host_policy" || fixture.executor.executions.Load() != 0 {
		t.Fatalf("replaced workspace outcome = %+v err=%v executions=%d", outcome, err, fixture.executor.executions.Load())
	}
}

func TestCancellationStopsSandboxAndRetainsPartialEvidence(t *testing.T) {
	fixture := newBrokerFixture(t)
	fixture.executor.block = true
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	call := makeCall(t, fixture.broker, "cancel-1", ToolCommand, CommandArguments{
		Argv: []string{"go", "test", "./..."}, WorkingDirectory: "/workspace", EnvironmentNames: []string{"SAFE_FLAG"},
		Network: sandbox.NetworkNone, TimeoutMilliseconds: 1000, CPUs: 1, MemoryBytes: 1 << 20, PIDs: 2,
		TmpfsBytes: 1 << 20, StdoutCapBytes: 100, StderrCapBytes: 100,
	})
	outcome, err := fixture.broker.Dispatch(ctx, call)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dispatch error = %v", err)
	}
	if outcome.Evidence.Disposition != "cancelled" || !outcome.Evidence.Cancellation.StopSucceeded || fixture.executor.cancels.Load() != 1 {
		t.Fatalf("cancellation evidence = %+v cancels=%d", outcome.Evidence, fixture.executor.cancels.Load())
	}
	if outcome.Evidence.ResultSHA256 == "" || outcome.Evidence.ResultRepresentation.Resolution == "" {
		t.Fatalf("cancellation result evidence is incomplete: %+v", outcome.Evidence)
	}
	if raw, readErr := os.ReadFile(outcome.Evidence.Stdout.Path); readErr != nil || string(raw) != "partial" {
		t.Fatalf("partial output = %q err=%v", raw, readErr)
	}
}

func TestToolTimeoutStopsSandboxAndExcessOutputEvidenceStaysBounded(t *testing.T) {
	fixture := newBrokerFixture(t)
	fixture.executor.block = true
	call := makeCall(t, fixture.broker, "timeout-1", ToolCommand, CommandArguments{
		Argv: []string{"go", "test", "./..."}, WorkingDirectory: "/workspace", EnvironmentNames: []string{"SAFE_FLAG"},
		Network: sandbox.NetworkNone, TimeoutMilliseconds: 10, CPUs: 1, MemoryBytes: 1 << 20, PIDs: 2,
		TmpfsBytes: 1 << 20, StdoutCapBytes: 100, StderrCapBytes: 100,
	})
	outcome, err := fixture.broker.Dispatch(context.Background(), call)
	if !errors.Is(err, context.DeadlineExceeded) || outcome.Evidence.Disposition != "timed_out" || !outcome.Evidence.Cancellation.StopSucceeded {
		t.Fatalf("timeout outcome = %+v err=%v", outcome, err)
	}

	fixture = newBrokerFixture(t)
	fixture.executor.commandStdout = []byte(strings.Repeat("x", 150))
	call = makeCall(t, fixture.broker, "output-1", ToolCommand, CommandArguments{
		Argv: []string{"go", "test", "./..."}, WorkingDirectory: "/workspace", EnvironmentNames: []string{"SAFE_FLAG"},
		Network: sandbox.NetworkNone, TimeoutMilliseconds: 1000, CPUs: 1, MemoryBytes: 1 << 20, PIDs: 2,
		TmpfsBytes: 1 << 20, StdoutCapBytes: 100, StderrCapBytes: 100,
	})
	outcome, err = fixture.broker.Dispatch(context.Background(), call)
	if err != nil || outcome.Evidence.DenialCode != "output_cap_exceeded" || outcome.Evidence.Stdout.SizeBytes != 100 || outcome.Evidence.StdoutTruncatedBytes != 50 {
		t.Fatalf("bounded output outcome = %+v err=%v", outcome, err)
	}
	if !outcome.Evidence.ResultRepresentation.Truncated || outcome.Evidence.ResultRepresentation.TruncatedBytes != 50 {
		t.Fatalf("truncation resolution evidence = %+v", outcome.Evidence.ResultRepresentation)
	}
}

func TestIndeterminateIntentIsNotBlindlyDispatched(t *testing.T) {
	fixture := newBrokerFixture(t)
	call := makeCall(t, fixture.broker, "indeterminate-1", ToolSourceEdit, SourceEditArguments{Path: "src/value.txt", ExpectedSHA256: digest([]byte("before\n")), Content: "after\n"})
	if begin, err := fixture.store.Begin(context.Background(), "indeterminate-1", call); err != nil || begin.disposition != beginNew {
		t.Fatalf("begin = %+v err=%v", begin, err)
	}
	outcome, err := fixture.broker.Dispatch(context.Background(), call)
	if err != nil || outcome.Evidence.DenialCode != "indeterminate_prior_effect" || fixture.executor.executions.Load() != 0 {
		t.Fatalf("outcome = %+v err=%v executions=%d", outcome, err, fixture.executor.executions.Load())
	}
}

func TestPolicyRejectsCredentialsHooksAmbientConfigurationAndRawAuthority(t *testing.T) {
	fixture := newBrokerFixture(t)
	bad := fixture.policy
	bad.HooksDisabled = false
	if _, err := NewBroker(bad, fixture.executor, fixture.store); err == nil {
		t.Fatal("enabled hooks were admitted")
	}
	bad = fixture.policy
	bad.AmbientHostConfiguration = true
	if _, err := NewBroker(bad, fixture.executor, fixture.store); err == nil {
		t.Fatal("ambient host configuration was admitted")
	}
	for _, forbidden := range []string{"OPENAI_API_KEY", "REVOLVR_DATABASE_URL", "PGPASSWORD", "DOCKER_HOST", "SSH_AUTH_SOCK", "HOME"} {
		if !forbiddenEnvironmentName(forbidden) {
			t.Fatalf("credential/runtime authority %q is not forbidden", forbidden)
		}
	}
	bad = fixture.policy
	bad.AllowedCommands = [][]string{{"docker", "run", "worker:latest"}}
	if err := validatePolicy(bad, false); err == nil {
		t.Fatal("raw container control was admitted")
	}
}

func makeCall(t *testing.T, broker *Broker, id, name string, arguments any) []byte {
	t.Helper()
	rawArguments, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Call{SchemaVersion: CallSchemaVersion, CallID: id, Tool: name, Authority: broker.Authority(), Arguments: rawArguments})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
