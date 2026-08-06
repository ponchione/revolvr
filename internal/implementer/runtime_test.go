package implementer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"revolvr/internal/model"
	"revolvr/internal/planner"
	"revolvr/internal/sandbox"
	"revolvr/internal/tool"
)

type implementationExecutor struct {
	root       string
	executions atomic.Int64
	cancels    atomic.Int64
}

func (e *implementationExecutor) Execute(_ context.Context, _ sandbox.Specification, operation tool.Operation) (tool.ExecutionResult, error) {
	e.executions.Add(1)
	if operation.Tool != tool.ToolSourceEdit || operation.SourceEdit == nil {
		return tool.ExecutionResult{ExitCode: 1}, errors.New("fake sandbox received an unexpected operation")
	}
	argument := operation.SourceEdit
	path := filepath.Join(e.root, filepath.FromSlash(argument.Path))
	before, err := os.ReadFile(path)
	if err != nil || digestBytes(before) != argument.ExpectedSHA256 {
		return tool.ExecutionResult{ExitCode: 1}, errors.Join(err, errors.New("fake sandbox edit source is stale"))
	}
	if err := os.WriteFile(path, []byte(argument.Content), 0o644); err != nil {
		return tool.ExecutionResult{ExitCode: 1}, err
	}
	after := digestBytes([]byte(argument.Content))
	return tool.ExecutionResult{
		ExitCode:      0,
		SourceChanges: []tool.SourceChange{{Path: argument.Path, BeforeSHA256: digestBytes(before), AfterSHA256: after}},
		Effect:        tool.EffectProof{Proven: true, Kind: "source_edit", Identity: argument.Path, BeforeSHA256: digestBytes(before), AfterSHA256: after},
	}, nil
}

func (e *implementationExecutor) Cancel(context.Context, string) error {
	e.cancels.Add(1)
	return nil
}

type deterministicModel struct {
	admission Admission
	policy    ModelPolicy
	calls     atomic.Int64
	claim     string
}

func (m *deterministicModel) Next(_ context.Context, request ModelRequest) (ModelTurn, error) {
	call := int(m.calls.Add(1))
	usage := model.UsageEvidence{Available: true, InputTokens: 10, OutputTokens: 5, ReasoningTokens: 1, TotalTokens: 15}
	if !request.FreshSession || request.SchemaVersion != InvocationVersion || request.InvocationID != m.admission.RunID+".implementer" {
		return ModelTurn{}, errors.New("fake model received stale invocation identity")
	}
	requestRaw, _ := json.Marshal(request)
	if bytes.Contains(requestRaw, []byte(m.admission.WorkspaceRoot)) {
		return ModelTurn{}, errors.New("fake model received the host workspace path")
	}
	switch call {
	case 1:
		if request.Iteration != 1 || len(request.History) != 0 {
			return ModelTurn{}, errors.New("first fake-model request carried hidden history")
		}
		arguments, _ := json.Marshal(tool.SourceEditArguments{Path: "src/value.txt", ExpectedSHA256: digestBytes([]byte("before\n")), Content: "after\n"})
		raw, _ := json.Marshal(tool.Call{SchemaVersion: tool.CallSchemaVersion, CallID: "edit-1", Tool: tool.ToolSourceEdit, Authority: request.Authority, Arguments: arguments})
		return ModelTurn{ToolCalls: []json.RawMessage{raw}, Usage: usage}, nil
	case 2:
		if request.Iteration != 2 || len(request.History) != 1 || request.History[0].ToolOutcome == nil || request.History[0].ToolOutcome.Evidence.CallID != "edit-1" {
			return ModelTurn{}, errors.New("second fake-model request lacks the exact invocation-local tool history")
		}
		toolEvidence := request.History[0].ToolOutcome.Evidence
		if toolEvidence.Input.Path != "" || toolEvidence.Result.Path != "" || toolEvidence.Stdout.Path != "" || toolEvidence.Stderr.Path != "" {
			return ModelTurn{}, errors.New("fake model received host artifact paths")
		}
		schemaSHA := digestBytes(request.SummarySchema)
		identity := expectedSummaryIdentity(m.admission, m.policy, request.PromptSHA256, schemaSHA)
		claimed := "src/value.txt"
		if m.claim != "" {
			claimed = m.claim
		}
		summary := Summary{
			SchemaVersion: SummarySchemaVersion, Identity: identity, Summary: "Updated the exact admitted file.",
			ClaimedFiles: []string{claimed}, Concerns: []string{}, CandidateFollowUpWork: []string{}, VoluntaryTests: []VoluntaryTest{},
			CandidatePlanProgress: []PlanProgress{{StepID: "step-1", Status: "candidate_completed", EvidenceCallIDs: []string{"edit-1"}}},
		}
		raw, _ := json.Marshal(summary)
		return ModelTurn{FinalOutput: raw, Usage: usage}, nil
	default:
		return ModelTurn{}, errors.New("fake model was invoked beyond its bounded deterministic script")
	}
}

type cancelledModel struct {
	calls   atomic.Int64
	started chan struct{}
}

func (m *cancelledModel) Next(ctx context.Context, request ModelRequest) (ModelTurn, error) {
	m.calls.Add(1)
	if !request.FreshSession || len(request.History) != 0 {
		return ModelTurn{}, errors.New("cancelled model received hidden state")
	}
	close(m.started)
	<-ctx.Done()
	return ModelTurn{}, ctx.Err()
}

type implementerFixture struct {
	config   Config
	model    *deterministicModel
	executor *implementationExecutor
	root     string
}

func newImplementerFixture(t *testing.T) implementerFixture {
	t.Helper()
	managed := t.TempDir()
	if err := os.Chmod(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(managed, "workspace-1")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--initial-branch=main")
	runGit(t, root, "config", "user.name", "Fixture")
	runGit(t, root, "config", "user.email", "fixture@example.invalid")
	if err := os.Mkdir(filepath.Join(root, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "value.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "src/value.txt")
	runGit(t, root, "commit", "--no-verify", "-m", "fixture")

	observer := HostObserver{}
	initial, err := observer.Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	request := sandbox.Request{
		SchemaVersion: sandbox.RequestSchemaVersion, SandboxID: "sandbox-1", ProjectID: "project-1", TaskID: "task-1", RunID: "run-1",
		Role: sandbox.RoleImplementer, Image: sandbox.Image{Reference: "worker:1", Digest: "sha256:" + strings.Repeat("a", 64)},
		RuntimeProfile: sandbox.ProfileStrict, Command: []string{"/usr/local/bin/revolvr-worker"},
		Mounts: []sandbox.Mount{{SourceID: "workspace-source", Target: "/workspace", Mode: sandbox.MountReadWrite}}, Network: sandbox.NetworkNone,
		Resources:   sandbox.Resources{CPUs: 2, MemoryBytes: 1 << 30, PIDs: 64, TimeoutSeconds: 60, TmpfsBytes: 64 << 20},
		Environment: map[string]string{"TASK_ID": "task-1", "RUN_ID": "run-1", "ROLE": "implementer"},
	}
	specification, err := sandbox.Validate(request, sandbox.Policy{
		ProjectID: request.ProjectID, TaskID: request.TaskID, RunID: request.RunID, Role: request.Role,
		ApprovedImages: []sandbox.Image{request.Image}, AllowedProfiles: []sandbox.RuntimeProfile{sandbox.ProfileStrict},
		AllowedNetworks: []sandbox.NetworkProfile{sandbox.NetworkNone}, AllowedEnvironmentNames: []string{"TASK_ID", "RUN_ID", "ROLE"},
		ManagedSources:   []sandbox.ManagedSource{{ID: "workspace-source", Root: managed, RelativePath: "workspace-1", Kind: sandbox.SourceWorkspace, Type: sandbox.SourceDirectory, Target: "/workspace"}},
		MaximumResources: request.Resources,
	})
	if err != nil {
		t.Fatal(err)
	}
	mount := specification.Mounts[0]
	activeSteps := []planner.Step{{ID: "step-1", Ordinal: 1, Status: "pending", Description: "Update exact value", CriterionIDs: []string{"criterion-1"}, ExpectedPaths: []string{"src/value.txt"}}}
	stepRaw, err := json.Marshal(activeSteps)
	if err != nil {
		t.Fatal(err)
	}
	stepBatchSHA := digestBytes(stepRaw)
	authority := tool.Authority{
		ProjectID: "project-1", TaskID: "task-1", TaskVersionID: "task-version-1", RunID: "run-1",
		SourceRevision: initial.SourceRevision, SourceCommit: initial.HeadCommit, SourceTree: initial.HeadTree,
		PlanID: "plan-1", PlanVersionID: "plan-version-1", PlanRevision: 1,
		StepBatchSHA256: stepBatchSHA, StepIDs: []string{"step-1"},
		WorkspaceID: "workspace-1", SandboxID: "sandbox-1",
	}
	policy, err := tool.PinPolicy(tool.PolicySettings{
		Authority: authority, Role: sandbox.RoleImplementer, WorkspaceRoot: root, WorkspaceDevice: mount.SourceDevice, WorkspaceInode: mount.SourceInode,
		Sandbox: specification, ExpectedPaths: []string{"src/value.txt"}, AdjacentPaths: []string{"tests"},
		ProtectedPaths: []string{"src/protected.txt"}, DependencyPaths: []string{"go.mod", "go.sum"}, VerificationAuthorityPaths: []string{"tests"},
		AllowedCommands: [][]string{{"go", "test", "./..."}}, Network: sandbox.NetworkNone, MaximumTimeout: 30 * time.Second,
		MaximumCPUs: 2, MaximumMemoryBytes: 1 << 30, MaximumPIDs: 64, MaximumTmpfsBytes: 64 << 20,
		MaximumStdoutBytes: 1 << 20, MaximumStderrBytes: 1 << 20, MaximumReadBytes: 1 << 20, MaximumEditBytes: 1 << 20,
		MaximumSearchResults: 100, HooksDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	toolEvidence := t.TempDir()
	if err := os.Chmod(toolEvidence, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := tool.NewFileStore(toolEvidence)
	if err != nil {
		t.Fatal(err)
	}
	executor := &implementationExecutor{root: root}
	broker, err := tool.NewBroker(policy, executor, store)
	if err != nil {
		t.Fatal(err)
	}
	modelPolicy, err := PinModelPolicy("deterministic-fake", 3, 4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	registry := broker.Registry()
	policyVersion, policySHA := broker.PolicyIdentity()
	admission := Admission{
		SchemaVersion: AdmissionSchemaVersion, Accepted: true, AcceptanceID: "admission-1", ProjectID: "project-1", TaskID: "task-1",
		TaskVersionID: "task-version-1", RunID: "run-1", ProjectSourceID: "project-source-1",
		SourceRevision: initial.SourceRevision, SourceCommit: initial.HeadCommit, SourceTree: initial.HeadTree,
		PlanID: "plan-1", PlanVersionID: "plan-version-1", PlanRevision: 1, StepBatchSHA256: stepBatchSHA,
		PlanAccepted: true, PlanAcceptanceID: "plan-acceptance-1", ActiveSteps: activeSteps,
		WorkspaceID: "workspace-1", WorkspaceRoot: root, WorkspaceDevice: mount.SourceDevice, WorkspaceInode: mount.SourceInode, WorkspaceStatus: "active",
		SandboxID: "sandbox-1", SandboxSHA256: authorityWithPolicy(broker).SandboxSHA256,
		HostPolicyVersion: policyVersion, HostPolicySHA256: policySHA, RegistryVersion: registry.Version, RegistrySHA256: registry.SHA256,
		ModelPolicyVersion: modelPolicy.Version, ModelPolicySHA256: modelPolicy.SHA256,
		ExpectedPaths: []string{"src/value.txt"}, AdjacentPaths: []string{"tests"}, ProtectedPaths: []string{"src/protected.txt", ".agent", ".revolvr", ".git"},
		DependencyPaths: []string{"go.mod", "go.sum"}, VerificationPaths: []string{"tests"},
	}
	evidenceRoot := t.TempDir()
	if err := os.Chmod(evidenceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeModel := &deterministicModel{admission: admission, policy: modelPolicy}
	return implementerFixture{
		config: Config{Admission: admission, ModelPolicy: modelPolicy, Model: fakeModel, Broker: broker, Observer: observer, EvidenceRoot: evidenceRoot},
		model:  fakeModel, executor: executor, root: root,
	}
}

func TestDeterministicFakeModelCompletesBrokeredEditWithExactHostEvidence(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "must-not-enter-worker-or-evidence")
	fixture := newImplementerFixture(t)
	result, err := Run(context.Background(), fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != "completed" || result.Summary == nil || fixture.model.calls.Load() != 2 || fixture.executor.executions.Load() != 1 {
		t.Fatalf("result = %+v model=%d executor=%d", result, fixture.model.calls.Load(), fixture.executor.executions.Load())
	}
	if len(result.After.ChangedManifest) != 1 || result.After.ChangedManifest[0].Path != "src/value.txt" || result.Source.DiffSHA256 != result.Source.Diff.SHA256 {
		t.Fatalf("host source evidence = %+v", result.Source)
	}
	diff, err := os.ReadFile(result.Source.Diff.Path)
	if err != nil || !bytes.Contains(diff, []byte("-before")) || !bytes.Contains(diff, []byte("+after")) || digestBytes(diff) != result.After.DiffSHA256 {
		t.Fatalf("diff evidence mismatch: %v\n%s", err, diff)
	}
	if len(result.ToolExecutions) != 1 || len(result.ToolExecutions[0].SourceChanges) != 1 || result.ToolExecutions[0].SourceChanges[0].Path != result.After.ChangedManifest[0].Path || result.ToolExecutions[0].Effect.AfterSHA256 != digestBytes([]byte("after\n")) {
		t.Fatalf("tool/source evidence mismatch: %+v", result.ToolExecutions)
	}
	if len(result.Signals) != 0 {
		t.Fatalf("unexpected policy signals: %+v", result.Signals)
	}
	for _, iteration := range result.ModelIterations {
		if !iteration.Usage.Available || iteration.Usage.TotalTokens != 15 {
			t.Fatalf("model usage was not retained: %+v", iteration.Usage)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(fixture.root, "src", "value.txt")); string(raw) != "after\n" {
		t.Fatalf("workspace bytes = %q", raw)
	}
	if found := findSentinel(t, fixture.config.EvidenceRoot, "must-not-enter-worker-or-evidence"); found != "" {
		t.Fatalf("API key sentinel leaked to evidence %s", found)
	}
	replayed, err := Run(context.Background(), fixture.config)
	if err != nil || !replayed.Replayed || fixture.model.calls.Load() != 2 || fixture.executor.executions.Load() != 1 {
		t.Fatalf("whole invocation replay = %+v err=%v model=%d executor=%d", replayed, err, fixture.model.calls.Load(), fixture.executor.executions.Load())
	}
	if err := os.WriteFile(filepath.Join(fixture.root, "src", "value.txt"), []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), fixture.config); !errors.Is(err, ErrReplaySourceDrift) || fixture.model.calls.Load() != 2 || fixture.executor.executions.Load() != 1 {
		t.Fatalf("drifted replay err=%v model=%d executor=%d", err, fixture.model.calls.Load(), fixture.executor.executions.Load())
	}
}

func TestModelCancellationStopsSandboxAndPersistsBoundedPartialEvidence(t *testing.T) {
	fixture := newImplementerFixture(t)
	model := &cancelledModel{started: make(chan struct{})}
	fixture.config.Model = model
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-model.started
		cancel()
	}()
	result, err := Run(ctx, fixture.config)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v", err)
	}
	if result.Disposition != "cancelled" || !result.Cancellation.StopSucceeded || fixture.executor.cancels.Load() != 1 || len(result.ModelIterations) != 1 {
		t.Fatalf("cancellation result = %+v cancels=%d", result, fixture.executor.cancels.Load())
	}
	for _, kind := range []SignalKind{SignalCancellation, SignalPartialWork, SignalNoSourceChange} {
		if !hasSignal(result.Signals, kind) {
			t.Fatalf("missing signal %s in %+v", kind, result.Signals)
		}
	}
	if _, statErr := os.Stat(filepath.Join(fixture.config.EvidenceRoot, "result.json")); statErr != nil {
		t.Fatalf("partial result was not persisted: %v", statErr)
	}
}

func TestClaimedActualAndSensitiveChangeClassesAreExplicitSignals(t *testing.T) {
	admission := Admission{
		ExpectedPaths: []string{"src"}, AdjacentPaths: []string{"docs"}, ProtectedPaths: []string{".agent", "deploy"},
		DependencyPaths: []string{"go.mod", "go.sum"}, VerificationPaths: []string{"tests", ".github/workflows"},
	}
	summary := &Summary{ClaimedFiles: []string{"src/claimed.go"}}
	after := WorkspaceObservation{ChangedManifest: []Change{
		{Path: "src/actual.go"}, {Path: "other/unexpected.go"}, {Path: ".agent/state.json"},
		{Path: "go.mod"}, {Path: "tests/verify.sh"}, {Path: "docs/note.md"},
	}}
	executions := []tool.Evidence{{CallID: "protected-attempt", Tool: tool.ToolSourceEdit, DenialCode: "protected_path"}}
	signals := reconcile(admission, summary, executions, WorkspaceObservation{HeadCommit: strings.Repeat("a", 40)}, after, "partial")
	for _, kind := range []SignalKind{
		SignalAdjacentChange, SignalUnexpectedChange, SignalProtectedChange, SignalDependencyChange,
		SignalVerificationAuthorityMutation, SignalClaimedActualMismatch, SignalPartialWork,
	} {
		if !hasSignal(signals, kind) {
			t.Fatalf("missing signal %s in %+v", kind, signals)
		}
	}
	noChange := reconcile(admission, &Summary{}, nil, WorkspaceObservation{}, WorkspaceObservation{}, "completed")
	if !hasSignal(noChange, SignalNoSourceChange) {
		t.Fatalf("no-source-change signal missing: %+v", noChange)
	}
}

func TestStaleAdmissionAndSummaryCannotBroadenOrAdvanceAuthority(t *testing.T) {
	fixture := newImplementerFixture(t)
	for _, mutate := range []func(*Admission){
		func(a *Admission) { a.Accepted = false },
		func(a *Admission) { a.RunID = "stale-run" },
		func(a *Admission) { a.PlanVersionID = "stale-plan" },
		func(a *Admission) { a.SourceRevision = strings.Repeat("e", 64) },
		func(a *Admission) { a.WorkspaceID = "stale-workspace" },
		func(a *Admission) { a.StepBatchSHA256 = strings.Repeat("e", 64) },
		func(a *Admission) { a.ExpectedPaths = append(a.ExpectedPaths, "broadened") },
	} {
		bad := fixture.config
		bad.Admission = cloneAdmission(fixture.config.Admission)
		mutate(&bad.Admission)
		bad.EvidenceRoot = t.TempDir()
		_ = os.Chmod(bad.EvidenceRoot, 0o700)
		if _, err := Run(context.Background(), bad); err == nil {
			t.Fatal("stale implementer admission was accepted")
		}
	}
	if fixture.model.calls.Load() != 0 {
		t.Fatalf("stale admission reached model %d times", fixture.model.calls.Load())
	}

	prepared, err := prepare(fixture.config)
	if err != nil {
		t.Fatal(err)
	}
	summary := Summary{SchemaVersion: SummarySchemaVersion, Identity: prepared.summaryIdentity, Summary: "Advisory only.", ClaimedFiles: []string{}, VoluntaryTests: []VoluntaryTest{}, Concerns: []string{}, CandidatePlanProgress: []PlanProgress{}, CandidateFollowUpWork: []string{}}
	raw, _ := json.Marshal(summary)
	raw = []byte(strings.TrimSuffix(string(raw), "}") + `,"completed":true}`)
	if _, err := parseSummary(raw, prepared.summaryIdentity, fixture.config.Admission, nil); err == nil {
		t.Fatal("summary with completion authority was accepted")
	}
	for _, forbidden := range []string{"criterion_status", "verification_status", "task_status", "lifecycle", "completion"} {
		schema, _ := SummarySchema()
		if strings.Contains(string(schema), `"`+forbidden+`"`) {
			t.Fatalf("summary schema exposes forbidden authority %q", forbidden)
		}
	}
}

func TestImplementerUsesNoCredentialDatabaseLiveNetworkOrHiddenSessionSurface(t *testing.T) {
	for _, file := range []string{"runtime.go", "contracts.go", "observer.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"OPENAI_API_KEY", "internal/storage/postgres", "pgxpool", "http.Client", "previous_response_id", "conversation_id", "tasklifecycle", "internal/verification"} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("%s contains forbidden worker authority %q", file, forbidden)
			}
		}
	}
}

func TestPublicToolRegistryAndImplementerSummarySchemaRemainByteCompatible(t *testing.T) {
	registry, err := tool.RegistryForRole(sandbox.RoleImplementer)
	if err != nil {
		t.Fatal(err)
	}
	if registry.SHA256 != "31133527fcfa704bfe4cdfa8272ab45b3e111f829a98a65fd88f1995f10a73aa" {
		t.Fatalf("implementer registry bytes changed: %s", registry.SHA256)
	}
	schema, err := SummarySchema()
	if err != nil {
		t.Fatal(err)
	}
	if got := digestBytes(schema); got != "f9dab2c9ae3719880db7e593313e5de7cc9126f5fce5e0369679d88c20cfe6bb" {
		t.Fatalf("implementer summary schema bytes changed: %s", got)
	}
	if SummarySchemaVersion != "revolvr-implementer-summary-v1" || SummarySchemaName != "revolvr_implementer_summary_v1" || len(registry.Definitions) != 4 {
		t.Fatal("public implementer contract names or closed tool count changed")
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null"}, arguments...)...)
	command.Dir = directory
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/nonexistent", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LANG=C", "LC_ALL=C"}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func authorityWithPolicy(broker *tool.Broker) tool.Authority { return broker.Authority() }

func hasSignal(signals []PolicySignal, kind SignalKind) bool {
	return slices.ContainsFunc(signals, func(signal PolicySignal) bool { return signal.Kind == kind })
}

func findSentinel(t *testing.T, root, sentinel string) string {
	t.Helper()
	found := ""
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(raw, []byte(sentinel)) {
			found = path
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return found
}
