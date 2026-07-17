package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func TestProductionAutonomousHappyPath(t *testing.T) {
	runProductionAutonomousHappyPath(t, false)
}

func TestProductionAutonomyForbidsRepositoryIntegrationOps(t *testing.T) {
	runProductionAutonomousHappyPath(t, true)
}

func runProductionAutonomousHappyPath(t *testing.T, proveRepositoryContainment bool) {
	t.Helper()
	const (
		taskID      = "production-happy"
		operationID = "production-happy-operation"
	)
	now := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	t.Setenv("GIT_AUTHOR_DATE", now.Format(time.RFC3339))
	t.Setenv("GIT_COMMITTER_DATE", now.Format(time.RFC3339))

	repo := t.TempDir()
	initializeSchedulingGitRepository(t, repo)
	for name, content := range map[string]string{
		"supervisor": "Select exactly one evidence-grounded autonomous action.",
		"documentor": "Update only the exact documentation target.",
		"auditor":    "Independently audit exact source and verification evidence.",
	} {
		writeProductionHappyFile(t, filepath.Join(repo, ".agent", "profiles", name+".md"), content+"\n")
	}
	writeProductionHappyFile(t, filepath.Join(repo, "docs", "source.md"), "production fixture source\n")
	task, err := taskfile.ProjectAutonomousTask(repo, taskfile.AutonomousCreateInput{
		ID: taskID, Title: "Production composition happy path",
		Body: "Create `docs/result.md` containing the verified production happy-path result, independently audit it, and complete the task.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = taskfile.PublishAutonomousTask(repo, task); err != nil {
		t.Fatal(err)
	}
	runSchedulingGit(t, repo, "add", ".agent", "docs")
	runSchedulingGit(t, repo, "commit", "-q", "-m", "Seed production happy path")
	baselineHead := runSchedulingGit(t, repo, "rev-parse", "HEAD")
	var unrelated productionLinkedWorktreeSnapshot
	if proveRepositoryContainment {
		unrelated = prepareProductionUnrelatedWorktree(t, repo, baselineHead)
	}
	baselineRevision := productionHappySourceRevision(t, repo)
	seedProductionHappyAudit(t, repo, task, baselineRevision, now)
	for _, directory := range []string{"locks", "receipts", "runs"} {
		if err := os.MkdirAll(filepath.Join(repo, ".revolvr", directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	executable := buildStrictFakeCodex(t)
	executableIdentity, err := codexexec.InspectExecutable(executable, nil)
	if err != nil {
		t.Fatal(err)
	}
	releaseManifest := codexexec.ReleaseManifest{
		SchemaVersion: codexexec.ReleaseManifestSchema,
		Codex:         []codexexec.ReleaseCodexBuild{{Version: strictFakeCodexVersion, SHA256: executableIdentity.SHA256}},
	}
	if err := releaseManifest.Validate(); err != nil {
		t.Fatal(err)
	}
	runCfg := DefaultRunOnceConfig(repo)
	runCfg.CodexExecutable = executable
	runCfg.CodexTimeout = 10 * time.Second
	runCfg.VerificationPlan = &autonomousverification.Plan{
		SchemaVersion: autonomousverification.PlanSchemaVersion,
		Tiers: []autonomousverification.Tier{{
			ID: "task-acceptance", Kind: autonomousverification.TierTaskAcceptance,
			RequiredForFinal: true, RunForFinal: true,
			Commands:    []verification.Command{{Name: "sh", Args: []string{"-c", `test "$(cat docs/result.md)" = "production happy path"`}}},
			RerunPolicy: autonomousverification.RerunNever,
		}},
	}
	var commandSpy *productionGitCommandSpy
	if proveRepositoryContainment {
		commandSpy = &productionGitCommandSpy{}
		runCfg.CommandRunner = commandSpy.Run
	}
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		t.Fatal(err)
	}
	workspace := productionHappyWorkspaceRoot(t, repo, taskID)
	contract := productionHappyCodexContract(t, repo, workspace, executable, effective, taskID)
	fixture := configureStrictFakeCodex(t, executable, repo, contract)

	ids := 0
	clockTicks := int64(-1)
	result, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repo}, TaskRunInput{
		OperationID: operationID,
		TaskID:      taskID,
		MaxCycles:   2,
		Clock: func() time.Time {
			clockTicks++
			return now.Add(time.Duration(clockTicks) * time.Second)
		},
		RunConfig: &runCfg,
		idGenerator: func() string {
			ids++
			return "happy-" + leftPadTwo(ids)
		},
		releaseManifest: &releaseManifest,
	})
	if err != nil {
		stderr, _ := os.ReadFile(filepath.Join(repo, ".revolvr", "runs", "happy-04", "codex.stderr"))
		t.Fatalf("RunTaskUntilTerminal() error = %v; result=%+v; strict stderr=%s", err, result, stderr)
	}
	wantStatistics := autonomoustaskrun.Statistics{
		SupervisorStarted: 2, SupervisorCompleted: 2, CyclesStarted: 2, CyclesCompleted: 2,
		AttemptsAdmitted: 1, AttemptsCompleted: 1, VerificationRuns: 1, Audits: 1,
		OptionalRoles: 1, SourceCommits: 1, CheckpointAdvances: 1,
		Actions: []autonomoustaskrun.ActionCount{{Action: string(autonomous.ActionDocument), Count: 1}},
	}
	if result.StopReason != autonomoustaskrun.StopCompleted || result.OperationID != operationID || result.TaskID != taskID || result.LastAction != string(autonomous.ActionComplete) || result.LastRunID != "happy-16" || !reflect.DeepEqual(result.Statistics, wantStatistics) {
		t.Fatalf("terminal result = %+v, want completed exact statistics %+v", result, wantStatistics)
	}
	if ids != 17 {
		t.Fatalf("production ID calls = %d, want 17", ids)
	}
	wantFakeState := strictFakeCodexState{SchemaVersion: strictFakeCodexStateSchema, VersionInvocations: 1, NextInvocation: 5, OutputSequence: append([]string(nil), contract.OutputSequence...)}
	if got := fixture.loadState(t); !reflect.DeepEqual(got, wantFakeState) {
		t.Fatalf("strict fake state = %+v, want %+v", got, wantFakeState)
	}
	if proveRepositoryContainment {
		commandSpy.assertAllowed(t)
		assertProductionLinkedWorktreeUnchanged(t, unrelated)
	}

	assertStrictFakeArtifact(t, filepath.Join(workspace, "docs", "result.md"), "production happy path\n")
	if _, err := os.Stat(filepath.Join(repo, "docs", "result.md")); !os.IsNotExist(err) {
		t.Fatalf("control-root source mutation exists or is unreadable: %v", err)
	}
	workspaceHead := runSchedulingGit(t, workspace, "rev-parse", "HEAD")
	if workspaceHead == baselineHead || runSchedulingGit(t, repo, "rev-parse", "HEAD") != baselineHead {
		t.Fatalf("workspace/control HEADs = %s/%s, baseline %s", workspaceHead, runSchedulingGit(t, repo, "rev-parse", "HEAD"), baselineHead)
	}
	if got := runSchedulingGit(t, workspace, "diff-tree", "--no-commit-id", "--name-status", "-r", workspaceHead); got != "A\tdocs/result.md" {
		t.Fatalf("run-owned commit diff = %q", got)
	}

	stateStore, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	state, found, err := stateStore.Load(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("load terminal state found=%v err=%v", found, err)
	}
	if state.State.Lifecycle != autonomous.LifecycleStateCompleted || state.State.Finalization == nil || state.State.Finalization.Stage != autonomous.FinalizationStageLedgerCompleted || state.State.Workspace == nil || state.State.Workspace.HeadSHA != workspaceHead || state.State.Workspace.BaselineSHA != baselineHead || len(state.State.Attempts.Events) != 2 || len(state.State.OptionalRoles) != 1 {
		t.Fatalf("terminal state = %+v", state.State)
	}
	markerRaw, err := os.ReadFile(state.State.Workspace.OwnerMarker)
	if err != nil {
		t.Fatal(err)
	}
	var marker productionWorkspaceOwner
	if err := json.Unmarshal(markerRaw, &marker); err != nil {
		t.Fatal(err)
	}
	markerMaterial := marker
	markerMaterial.MaterialSHA256 = ""
	materialRaw, _ := json.Marshal(markerMaterial)
	if marker.SchemaVersion != "revolvr-workspace-owner-v1" || marker.TaskID != taskID || marker.WorkspaceID != state.State.Workspace.WorkspaceID || marker.ControlRoot != repo || marker.ExecutionRoot != workspace || marker.BaselineSHA != baselineHead || marker.CreationOperationID != operationID+"-workspace" || marker.MaterialSHA256 != productionHash(materialRaw) {
		t.Fatalf("workspace owner marker = %+v", marker)
	}
	markerCanonical, _ := json.MarshalIndent(marker, "", "  ")
	markerCanonical = append(markerCanonical, '\n')
	if !reflect.DeepEqual(markerRaw, markerCanonical) {
		t.Fatal("workspace owner marker bytes are not canonical")
	}
	stateRaw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(task.AutonomousStatePath)))
	if err != nil {
		t.Fatal(err)
	}
	stateCanonical, err := autonomousstate.MarshalState(state.State)
	if err != nil || !reflect.DeepEqual(stateRaw, stateCanonical) {
		t.Fatalf("canonical state bytes mismatch: err=%v", err)
	}
	completedTask, found, err := taskfile.FindByID(repo, taskID)
	if err != nil || !found || completedTask.Status != taskfile.StatusCompleted {
		t.Fatalf("completed task found=%v task=%+v err=%v", found, completedTask, err)
	}
	projectedCompleted, err := taskfile.ProjectMetadataFromSnapshot(repo, task, taskfile.MetadataUpdate{Status: taskfile.StatusCompleted})
	if err != nil || !reflect.DeepEqual(completedTask.SourceBytes, projectedCompleted.SourceBytes) {
		t.Fatalf("canonical completed task bytes mismatch: err=%v", err)
	}

	receiptPath := filepath.Join(repo, ".revolvr", "receipts", "happy-05.md")
	receiptRaw, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	parsedReceipt, err := receipt.Parse(receiptRaw)
	if err != nil {
		t.Fatal(err)
	}
	if parsedReceipt.RunID != "happy-05" || parsedReceipt.TaskID != taskID || parsedReceipt.Verdict != receipt.VerdictCompleted || parsedReceipt.VerificationStatus != "passed" || parsedReceipt.CommitSHA != workspaceHead || !reflect.DeepEqual(parsedReceipt.ChangedFiles, []string{"docs/result.md"}) || parsedReceipt.Metrics.InputTokens != 11 || parsedReceipt.Metrics.OutputTokens != 6 {
		t.Fatalf("worker receipt = %+v", parsedReceipt)
	}
	wantReceipt, _ := receipt.FormatFallbackReceipt(receipt.FallbackInput{RunID: parsedReceipt.RunID, PassID: parsedReceipt.PassID, TaskID: parsedReceipt.TaskID, Task: parsedReceipt.Task, Verdict: parsedReceipt.Verdict, Timestamp: parsedReceipt.Timestamp, CodexExitCode: parsedReceipt.CodexExitCode, VerificationStatus: parsedReceipt.VerificationStatus, CommitSHA: parsedReceipt.CommitSHA, ChangedFiles: parsedReceipt.ChangedFiles, Verification: parsedReceipt.Verification, Metrics: parsedReceipt.Metrics, FinalText: "Documented the exact production happy-path result."})
	if string(receiptRaw) != wantReceipt {
		t.Fatalf("receipt bytes differ from exact harness fallback\ngot:\n%s\nwant:\n%s", receiptRaw, wantReceipt)
	}

	completionBase := filepath.Join(repo, ".revolvr", "autonomous", "tasks", taskID, "completion")
	frozenRaw, err := os.ReadFile(filepath.Join(completionBase, "completion-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := autonomousfinalization.DecodeFrozen(frozenRaw)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.OperationID != operationID+"-finalization" || frozen.FinalizationRunID != "finalization-happy-17" || frozen.Workspace.HeadSHA != workspaceHead || frozen.Source.Revision != state.State.Workspace.SourceRevision || frozen.Verification.Summary.RunID != "happy-05" || frozen.Audit.RunID != "happy-11" || frozen.Decision.Action != autonomous.ActionComplete {
		t.Fatalf("frozen evidence identities = %+v", frozen)
	}
	frozenStateIdentity, err := autonomousstate.StateIdentityFor(task.AutonomousStatePath, true, frozen.State)
	if err != nil || frozenStateIdentity != frozen.StateIdentity {
		t.Fatalf("frozen state identity = %+v, computed %+v, err=%v", frozen.StateIdentity, frozenStateIdentity, err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(completionBase, "completion-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest autonomousfinalization.Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifestCanonical, err := autonomousfinalization.MarshalManifest(manifest)
	if err != nil || !reflect.DeepEqual(manifestRaw, manifestCanonical) {
		t.Fatalf("canonical manifest bytes mismatch: err=%v", err)
	}
	assertProductionArtifactIdentity(t, repo, manifest.FrozenEvidence)
	assertProductionArtifactIdentity(t, repo, manifest.Capsule)
	assertProductionArtifactIdentity(t, repo, *state.State.Finalization.Manifest)

	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(repo, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	runs, err := ledgerStore.ListRecentRunsForTaskWithEvents(context.Background(), taskID, 100)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	events := map[string][]ledger.EventType{}
	for _, item := range runs {
		statuses[item.Run.ID] = item.Run.Status
		for _, event := range item.Events {
			events[item.Run.ID] = append(events[item.Run.ID], event.Type)
		}
	}
	if len(runs) != 7 {
		t.Fatalf("ledger run count = %d, want 7", len(runs))
	}
	for _, runID := range []string{productionTaskRunLedgerID(operationID), "happy-04", "happy-05", "happy-10", "happy-11", "happy-16", "finalization-happy-17"} {
		if statuses[runID] != ledger.StatusCompleted {
			t.Fatalf("ledger run %s status = %q; all=%v", runID, statuses[runID], statuses)
		}
	}
	wantFinalizationEvents := []ledger.EventType{ledger.EventFinalizationPrepared, ledger.EventFinalizationMaterialized, ledger.EventFinalizationStateTerminal, ledger.EventFinalizationCompleted}
	if !reflect.DeepEqual(events["finalization-happy-17"], wantFinalizationEvents) {
		t.Fatalf("finalization ledger events = %v, want %v", events["finalization-happy-17"], wantFinalizationEvents)
	}
	operation, found, err := autonomoustaskrun.Inspect(repo, operationID)
	if err != nil || !found || operation.StopReason != autonomoustaskrun.StopCompleted || operation.Stage != "terminal" || operation.WorkspaceID != state.State.Workspace.WorkspaceID || operation.CheckpointSHA != workspaceHead {
		t.Fatalf("terminal operation found=%v operation=%+v err=%v", found, operation, err)
	}
	operationRaw, err := os.ReadFile(filepath.Join(repo, ".revolvr", "autonomous", "task-runs", operationID, "operation.json"))
	if err != nil {
		t.Fatal(err)
	}
	operationCanonical, err := json.MarshalIndent(operation, "", "  ")
	operationCanonical = append(operationCanonical, '\n')
	if err != nil || !reflect.DeepEqual(operationRaw, operationCanonical) {
		t.Fatalf("canonical task-run operation bytes mismatch: err=%v", err)
	}
}

func productionHappyCodexContract(t *testing.T, root, workspace, executable string, cfg runonce.Config, taskID string) strictFakeCodexContract {
	t.Helper()
	supervisorSchema, err := supervisor.DecisionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	auditSchema, err := autonomousaudit.AuditOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	document := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionDocument, WorkerProfile: autonomous.WorkerProfileDocumentor,
		Rationale:       "The task explicitly requires the bounded result document.",
		SuccessCriteria: []string{"docs/result.md contains the verified production happy-path result."},
		Inputs:          []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact task-required documentation target.")},
		Strategy:        &autonomous.Strategy{Approach: "Write the exact bounded result document.", Techniques: []string{"edit only docs/result.md"}, Targets: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact documentation target.")}},
	}
	auditDecision := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor,
		Rationale:       "The verified documentation commit requires a fresh independent audit.",
		SuccessCriteria: []string{"Return an exact clean audit tied to current verification."},
		Inputs:          []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact committed documentation target.")},
		Strategy:        &autonomous.Strategy{Approach: "Audit the exact verified documentation change.", Techniques: []string{"inspect current source and verification"}, Targets: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact audit target.")}},
	}
	complete := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionComplete,
		Rationale: "The completed plan, satisfied acceptance, passed verification, clean independent audit, and workspace checkpoint authorize completion.",
		Inputs:    []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindTask, ".agent/tasks/"+taskID+".md", "Exact canonical task.")},
	}
	for _, decision := range []*autonomous.SupervisorDecision{&document, &auditDecision, &complete} {
		if err := decision.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	decisionJSON := func(value autonomous.SupervisorDecision) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw) + "\n"
	}
	events := func(name string, input, output int) []string {
		return []string{
			`{"type":"thread.started","thread_id":"` + name + `-thread"}`,
			`{"type":"turn.completed","final_message":"` + name + ` completed","usage":{"input_tokens":` + decimal(input) + `,"output_tokens":` + decimal(output) + `,"duration_seconds":1}}`,
		}
	}
	type invocationSpec struct {
		name, runID string
		action      autonomous.Action
		supervisor  bool
		message     string
	}
	specs := []invocationSpec{
		{name: "document-supervisor", runID: "happy-04", supervisor: true, message: decisionJSON(document)},
		{name: "document-worker", runID: "happy-05", action: autonomous.ActionDocument, message: "Documented the exact production happy-path result.\n"},
		{name: "audit-supervisor", runID: "happy-10", supervisor: true, message: decisionJSON(auditDecision)},
		{name: "audit-worker", runID: "happy-11", action: autonomous.ActionAudit, message: `{"schema_version":"autonomous-audit-output-v1","task_id":"` + taskID + `","report":{"task_id":"` + taskID + `","disposition":"clean","rationale":"The exact verified documentation change is correct and complete.","inputs":@@INPUTS@@},"provenance":@@PROVENANCE@@}` + "\n"},
		{name: "complete-supervisor", runID: "happy-16", supervisor: true, message: decisionJSON(complete)},
	}
	contract := strictFakeCodexContract{VersionInvocationCount: 1}
	for index, spec := range specs {
		invocation := strictFakeCodexInvocation{Name: spec.name, WorkingDirectory: workspace, LastMessage: spec.message}
		if spec.supervisor {
			base := filepath.ToSlash(filepath.Join(".revolvr", "runs", spec.runID))
			invocation.PromptPath = filepath.Join(root, filepath.FromSlash(base), "supervisor-prompt.md")
			invocation.OutputSchema = &strictFakeCodexMaterial{Path: filepath.Join(root, filepath.FromSlash(base), "supervisor-output-schema.json"), Content: string(supervisorSchema)}
			writeProductionHappyFile(t, invocation.OutputSchema.Path, invocation.OutputSchema.Content)
			invocation.Argv = productionHappyInvocation(t, cfg, executable, workspace, root, codexexec.ArtifactPaths{StdoutJSONL: base + "/codex.jsonl", Stderr: base + "/codex.stderr", LastMessage: base + "/supervisor-output.json"}, base+"/supervisor-output-schema.json")
		} else {
			base := filepath.ToSlash(filepath.Join(".revolvr", "runs", spec.runID))
			output, schema := base+"/worker-output.txt", ""
			if spec.action == autonomous.ActionAudit {
				output, schema = base+"/auditor-output.raw.json", base+"/auditor-output-schema.json"
				invocation.OutputSchema = &strictFakeCodexMaterial{Path: filepath.Join(root, filepath.FromSlash(schema)), Content: string(auditSchema)}
				writeProductionHappyFile(t, invocation.OutputSchema.Path, invocation.OutputSchema.Content)
				invocation.Substitutions = []strictFakeSubstitution{{Token: "@@INPUTS@@", Heading: "Exact Audit Output Provenance", JSONPointer: "/verification/summary/evidence"}, {Token: "@@PROVENANCE@@", Heading: "Exact Audit Output Provenance"}}
			} else {
				invocation.Writes = []strictFakeCodexMaterial{{Path: filepath.Join("docs", "result.md"), Content: "production happy path\n"}}
			}
			invocation.PromptPath = filepath.Join(root, filepath.FromSlash(base), "worker-prompt.md")
			invocation.Argv = productionHappyInvocation(t, cfg, executable, workspace, root, codexexec.ArtifactPaths{StdoutJSONL: base + "/codex.jsonl", Stderr: base + "/codex.stderr", LastMessage: output}, schema)
		}
		invocation.StdoutJSONL = events(spec.name, 10+index, 5+index)
		invocation.OutputEventTypes = []string{"thread.started", "turn.completed"}
		contract.Invocations = append(contract.Invocations, invocation)
		contract.OutputSequence = append(contract.OutputSequence, spec.name+":thread.started", spec.name+":turn.completed")
	}
	return contract
}

func productionHappyInvocation(t *testing.T, cfg runonce.Config, executable, workspace, root string, artifacts codexexec.ArtifactPaths, outputSchema string) []string {
	t.Helper()
	prepared, _, err := codexexec.PrepareInvocation(codexexec.InvocationConfig{
		Executable: executable, WorkingDir: workspace, ArtifactRoot: root,
		Model: cfg.CodexModel, ReasoningEffort: cfg.CodexReasoningEffort, Ephemeral: cfg.CodexEphemeral,
		Sandbox: cfg.CodexSandbox, ApprovalPolicy: cfg.CodexApprovalPolicy, BypassApprovalsSandbox: cfg.CodexBypassApprovalsAndSandbox,
		Artifacts: artifacts, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared.Argv
}

func seedProductionHappyAudit(t *testing.T, repo string, task taskfile.Task, revision string, now time.Time) {
	t.Helper()
	evidence := []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindVerification, ".revolvr/runs/baseline-verification/verification.json", "Exact passed baseline verification.")}
	previous := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: task.ID, Lifecycle: autonomous.LifecycleStateReady,
		Plan:               &autonomous.TaskPlan{TaskID: task.ID, ID: "production-plan", Revision: 1, Provenance: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindTask, task.SourcePath, "Exact canonical task source.")}, Steps: []autonomous.PlanStep{{ID: "implementation-complete", Description: "Establish the bounded implementation baseline.", Status: autonomous.PlanStepStatusCompleted, Evidence: evidence}}, Completed: true},
		AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "production-result", Requirement: "The production-composition happy path is verified.", Status: autonomous.AcceptanceStatusSatisfied, Evidence: evidence}},
		Attempts:           autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}},
	}
	stateRaw, err := autonomousstate.MarshalState(previous)
	if err != nil {
		t.Fatal(err)
	}
	writeProductionHappyFile(t, filepath.Join(repo, filepath.FromSlash(task.AutonomousStatePath)), string(stateRaw))

	decisionPath := ".revolvr/runs/baseline-audit-supervisor/supervisor-decision.json"
	decision := autonomous.DecisionReference{DecisionID: "decision-baseline-audit", RunID: "baseline-audit-supervisor", TaskID: task.ID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: productionEvidence(autonomous.EvidenceKindFile, decisionPath, "Exact baseline audit decision."), CreatedAt: now}
	decisionRaw := []byte("{\"baseline_audit_decision\":true}\n")
	writeProductionHappyFile(t, filepath.Join(repo, filepath.FromSlash(decisionPath)), string(decisionRaw))
	next := previous
	next.LatestDecision = &decision
	nextRaw, err := autonomousstate.MarshalState(next)
	if err != nil {
		t.Fatal(err)
	}
	verificationEvidence := autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: task.ID, Status: autonomous.VerificationStatusPassed, Summary: "Baseline verification passed.", RunID: "baseline-verification", OccurrenceID: "baseline-occurrence", Evidence: evidence}, SourceRevision: revision}
	report := autonomous.AuditReport{TaskID: task.ID, Disposition: autonomous.AuditDispositionClean, Rationale: "The exact baseline is clean.", Inputs: evidence}
	workerRunID := "baseline-auditor"
	rawPath := ".revolvr/runs/" + workerRunID + "/auditor-output.raw.json"
	canonicalPath := ".revolvr/runs/" + workerRunID + "/auditor-output.canonical.json"
	profilePath := ".agent/profiles/auditor.md"
	profileRaw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(profilePath)))
	if err != nil {
		t.Fatal(err)
	}
	dossier := autonomousaudit.DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: task.ID, SHA256: strings.Repeat("6", 64), ByteSize: 100}
	profile := autonomousaudit.ProfileIdentity{Name: autonomous.WorkerProfileAuditor, Path: profilePath, SHA256: productionHash(profileRaw), ByteSize: len(profileRaw)}
	output := autonomousaudit.AuditOutput{SchemaVersion: autonomousaudit.AuditOutputSchemaVersion, TaskID: task.ID, Report: report, Provenance: autonomousaudit.AuditProvenance{Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, WorkerRunID: workerRunID, Decision: decision, Dossier: dossier, Profile: profile, RawOutputPath: rawPath, SourceRevision: revision, Verification: verificationEvidence}}
	canonical, err := autonomousaudit.MarshalAuditOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	writeProductionHappyFile(t, filepath.Join(repo, filepath.FromSlash(rawPath)), string(raw))
	policyAudit := autonomouspolicy.AuditEvidence{Report: report, RunID: workerRunID, AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: revision, VerificationRunID: verificationEvidence.Summary.RunID, VerificationOccurrenceID: verificationEvidence.Summary.OccurrenceID}
	history := autonomousstate.AuditHistoryRecord{
		SchemaVersion: autonomousstate.AuditHistorySchemaVersion, TaskID: task.ID, Sequence: 1, AuditRevision: 1, OperationID: "baseline-audit-operation", ApplicationSHA256: strings.Repeat("5", 64), Kind: autonomousstate.AuditTransitionRecorded, CreatedAt: now,
		Decision: decision, SupervisorDecision: productionArtifact(decisionPath, decisionRaw), WorkerRunID: workerRunID, Profile: profile, Dossier: dossier, SourceRevision: revision, Verification: verificationEvidence,
		TaskSource: productionArtifact(task.SourcePath, task.SourceBytes), RawOutput: productionArtifact(rawPath, raw), CanonicalOutput: productionArtifact(canonicalPath, canonical), Report: report, PolicyEvidence: policyAudit,
		PreviousState: productionStateIdentity(task.AutonomousStatePath, stateRaw), ResultingState: productionStateIdentity(task.AutonomousStatePath, nextRaw),
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitAudit(context.Background(), autonomousstate.AuditCommitRequest{TaskID: task.ID, Expected: autonomousstate.ExpectedState{Exists: true, SHA256: productionHash(stateRaw), ByteSize: len(stateRaw)}, PreviousState: previous, NextState: next, History: history, CanonicalOutput: canonical}); err != nil {
		t.Fatal(err)
	}
}

func productionHappySourceRevision(t *testing.T, repo string) string {
	t.Helper()
	snapshot, err := gitstate.CaptureSourceSnapshot(context.Background(), gitstate.SourceSnapshotConfig{WorkingDir: repo, GitExecutable: "git", Timeout: 10 * time.Second, StdoutCap: 1 << 20, StderrCap: 1 << 20, AllowHarnessRuntime: true})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := gitstate.PolicySourceRevision(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func productionHappyWorkspaceRoot(t *testing.T, repo, taskID string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	common, err := filepath.EvalSymlinks(filepath.Join(root, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(root + "\x00" + common + "\x00" + taskID))
	return filepath.Join(root, ".revolvr", "autonomous", "worktrees", hex.EncodeToString(digest[:16]))
}

func productionEvidence(kind autonomous.EvidenceKind, reference, detail string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: detail}
}

type productionWorkspaceOwner struct {
	SchemaVersion       string    `json:"schema_version"`
	TaskID              string    `json:"task_id"`
	WorkspaceID         string    `json:"workspace_id"`
	ControlRoot         string    `json:"control_root"`
	ExecutionRoot       string    `json:"execution_root"`
	GitCommonDir        string    `json:"git_common_dir"`
	BranchRef           string    `json:"branch_ref"`
	BaselineSHA         string    `json:"baseline_sha"`
	CreationOperationID string    `json:"creation_operation_id"`
	MaterialSHA256      string    `json:"material_sha256"`
	CreatedAt           time.Time `json:"created_at"`
}

func productionHash(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func productionTaskRunLedgerID(operationID string) string {
	digest := sha256.Sum256([]byte("autonomous-task-run-ledger-v1\x00" + operationID))
	return "task-run-" + hex.EncodeToString(digest[:12])
}

func productionArtifact(path string, raw []byte) autonomousstate.ArtifactIdentity {
	return autonomousstate.ArtifactIdentity{Path: path, SHA256: productionHash(raw), ByteSize: len(raw)}
}

func productionStateIdentity(path string, raw []byte) autonomousstate.StateIdentity {
	return autonomousstate.StateIdentity{Path: path, Persisted: true, SHA256: productionHash(raw), ByteSize: len(raw)}
}

func writeProductionHappyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertProductionArtifactIdentity(t *testing.T, root string, artifact autonomous.FinalizationArtifact) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.SHA256 != productionHash(raw) || artifact.ByteSize != len(raw) {
		t.Fatalf("artifact identity = %+v, actual sha=%s bytes=%d", artifact, productionHash(raw), len(raw))
	}
}

func leftPadTwo(value int) string {
	if value < 10 {
		return "0" + decimal(value)
	}
	return decimal(value)
}

func decimal(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}

var productionProhibitedGitVerbs = map[string]struct{}{
	"push": {}, "merge": {}, "rebase": {}, "reset": {}, "clean": {}, "stash": {},
}

type productionGitCommandSpy struct {
	mu         sync.Mutex
	commands   []runner.Command
	prohibited []string
}

func (s *productionGitCommandSpy) Run(ctx context.Context, command runner.Command) runner.Result {
	verb := productionGitVerb(command)
	_, prohibited := productionProhibitedGitVerbs[verb]
	s.mu.Lock()
	s.commands = append(s.commands, command)
	if prohibited {
		s.prohibited = append(s.prohibited, verb)
	}
	s.mu.Unlock()
	if prohibited {
		return runner.Result{ExitCode: 125, Err: fmt.Errorf("production autonomy invoked prohibited git verb %q", verb)}
	}
	return runner.Run(ctx, command)
}

func (s *productionGitCommandSpy) assertAllowed(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.prohibited) != 0 {
		t.Fatalf("prohibited production Git verbs = %v", s.prohibited)
	}
	seenWorktree, seenCommit := false, false
	for _, command := range s.commands {
		switch productionGitVerb(command) {
		case "worktree":
			seenWorktree = true
		case "commit":
			seenCommit = true
		}
	}
	if !seenWorktree || !seenCommit {
		t.Fatalf("command spy did not observe the production workspace and commit boundaries: worktree=%v commit=%v", seenWorktree, seenCommit)
	}
}

func productionGitVerb(command runner.Command) string {
	if filepath.Base(command.Name) != "git" {
		return ""
	}
	for _, arg := range command.Args {
		if arg == "--literal-pathspecs" {
			continue
		}
		return arg
	}
	return ""
}

type productionLinkedWorktreeSnapshot struct {
	root, head, branch, status string
	tracked, sentinel          []byte
	sentinelMode               os.FileMode
}

func prepareProductionUnrelatedWorktree(t *testing.T, repo, baselineHead string) productionLinkedWorktreeSnapshot {
	t.Helper()
	root := filepath.Join(t.TempDir(), "unrelated-worktree")
	runSchedulingGit(t, repo, "worktree", "add", "-q", "-b", "ext10-unrelated-proof", root, baselineHead)
	sentinelPath := filepath.Join(root, "unrelated-sentinel.txt")
	if err := os.WriteFile(sentinelPath, []byte("unrelated worktree sentinel\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	tracked, err := os.ReadFile(filepath.Join(root, "docs", "source.md"))
	if err != nil {
		t.Fatal(err)
	}
	sentinel, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	return productionLinkedWorktreeSnapshot{
		root: root, head: runSchedulingGit(t, root, "rev-parse", "HEAD"), branch: runSchedulingGit(t, root, "symbolic-ref", "HEAD"), status: runSchedulingGit(t, root, "status", "--porcelain=v1", "--untracked-files=all"),
		tracked: tracked, sentinel: sentinel, sentinelMode: info.Mode(),
	}
}

func assertProductionLinkedWorktreeUnchanged(t *testing.T, before productionLinkedWorktreeSnapshot) {
	t.Helper()
	if got := runSchedulingGit(t, before.root, "rev-parse", "HEAD"); got != before.head {
		t.Fatalf("unrelated worktree HEAD = %q, want %q", got, before.head)
	}
	if got := runSchedulingGit(t, before.root, "symbolic-ref", "HEAD"); got != before.branch {
		t.Fatalf("unrelated worktree branch = %q, want %q", got, before.branch)
	}
	if got := runSchedulingGit(t, before.root, "status", "--porcelain=v1", "--untracked-files=all"); got != before.status {
		t.Fatalf("unrelated worktree status = %q, want %q", got, before.status)
	}
	tracked, err := os.ReadFile(filepath.Join(before.root, "docs", "source.md"))
	if err != nil || !reflect.DeepEqual(tracked, before.tracked) {
		t.Fatalf("unrelated tracked sentinel changed: err=%v bytes=%q want=%q", err, tracked, before.tracked)
	}
	sentinelPath := filepath.Join(before.root, "unrelated-sentinel.txt")
	sentinel, err := os.ReadFile(sentinelPath)
	if err != nil || !reflect.DeepEqual(sentinel, before.sentinel) {
		t.Fatalf("unrelated untracked sentinel changed: err=%v bytes=%q want=%q", err, sentinel, before.sentinel)
	}
	info, err := os.Lstat(sentinelPath)
	if err != nil || info.Mode() != before.sentinelMode {
		t.Fatalf("unrelated sentinel mode changed: err=%v mode=%v want=%v", err, infoMode(info), before.sentinelMode)
	}
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode()
}
