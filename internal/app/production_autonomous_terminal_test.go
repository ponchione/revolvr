package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/runonce"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func TestProductionAutonomousTerminalMatrix(t *testing.T) {
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

	tests := []struct {
		name                 string
		kind                 productionTerminalKind
		wantStop             autonomoustaskrun.StopReason
		wantError            bool
		wantInvocations      int
		wantVersionCalls     int
		wantReceipts         []string
		wantVerification     bool
		wantWorkspaceCommits int
		wantWorkspaceStatus  string
		wantControlStatus    string
		wantLifecycle        autonomous.LifecycleState
		wantAttempts         int
		wantInputQuestions   int
		wantBreaker          autonomous.BreakerReason
		wantDetail           string
		wantCycles           int64
		wantReplayed         bool
	}{
		{name: "needs_input", kind: productionTerminalNeedsInput, wantStop: autonomoustaskrun.StopNeedsInput, wantInvocations: 1, wantVersionCalls: 1, wantLifecycle: autonomous.LifecycleStateNeedsInput, wantInputQuestions: 1, wantDetail: "clean", wantCycles: 1},
		{name: "authorized_block", kind: productionTerminalBlock, wantStop: autonomoustaskrun.StopBlocked, wantInvocations: 1, wantVersionCalls: 1, wantControlStatus: "M .agent/tasks/terminal-authorized-block.md", wantLifecycle: autonomous.LifecycleStateBlocked, wantDetail: "The exact external authority required by the task is unavailable.", wantCycles: 1},
		{name: "verification_failure", kind: productionTerminalVerificationFailure, wantStop: autonomoustaskrun.StopUnsafeAmbiguous, wantError: true, wantInvocations: 2, wantVersionCalls: 1, wantReceipts: []string{"verification-failure-05.md"}, wantVerification: true, wantWorkspaceStatus: "M docs/result.md", wantLifecycle: autonomous.LifecycleStateReady, wantAttempts: 2, wantDetail: "verification", wantCycles: 1},
		{name: "no_progress", kind: productionTerminalNoProgress, wantStop: autonomoustaskrun.StopNoProgress, wantInvocations: 3, wantVersionCalls: 1, wantReceipts: []string{"no-progress-05.md"}, wantLifecycle: autonomous.LifecycleStateBlocked, wantAttempts: 2, wantBreaker: autonomous.BreakerIdenticalStrategy, wantDetail: "identical_strategy", wantCycles: 2},
		{name: "trusted_safety_refusal", kind: productionTerminalSafety, wantStop: autonomoustaskrun.StopSafety, wantInvocations: 1, wantVersionCalls: 1, wantWorkspaceStatus: "?? docs/supervisor-mutated.txt", wantLifecycle: autonomous.LifecycleStateReady, wantDetail: "supervisor decision pass changed repository source", wantCycles: 1},
		{name: "caller_cancellation", kind: productionTerminalCancellation, wantStop: autonomoustaskrun.StopOperationCancelled, wantError: true, wantVersionCalls: 1, wantLifecycle: autonomous.LifecycleStateReady, wantDetail: context.Canceled.Error(), wantCycles: 1},
		{name: "restart_exact_durable_authority", kind: productionTerminalRestart, wantStop: autonomoustaskrun.StopNeedsInput, wantInvocations: 1, wantVersionCalls: 2, wantLifecycle: autonomous.LifecycleStateNeedsInput, wantInputQuestions: 1, wantDetail: "clean", wantCycles: 1, wantReplayed: true},
		{name: "maximum_cycle", kind: productionTerminalMaximumCycle, wantStop: autonomoustaskrun.StopMaxCycles, wantInvocations: 2, wantVersionCalls: 1, wantReceipts: []string{"maximum-cycle-05.md"}, wantVerification: true, wantWorkspaceCommits: 1, wantLifecycle: autonomous.LifecycleStateReady, wantAttempts: 2, wantDetail: "caller-owned maximum cycle limit reached", wantCycles: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProductionTerminalFixture(t, executable, releaseManifest, test.kind)
			ctx := context.Background()
			var cancel context.CancelFunc
			progress := autonomoustaskrun.Progress(nil)
			if test.kind == productionTerminalCancellation {
				ctx, cancel = context.WithCancel(ctx)
				progress = func(operation autonomoustaskrun.Operation) {
					if operation.Stage == "cycle_started" {
						cancel()
					}
				}
			}
			input := fixture.input(progress)
			result, runErr := RunTaskUntilTerminal(ctx, Config{WorkDir: fixture.repo}, input)
			if cancel != nil {
				cancel()
			}
			if test.wantError {
				if runErr == nil {
					t.Fatalf("RunTaskUntilTerminal() error = nil; result=%+v", result)
				}
				if test.kind == productionTerminalCancellation && !errors.Is(runErr, context.Canceled) {
					t.Fatalf("cancellation error = %v, want context.Canceled", runErr)
				}
			} else if runErr != nil {
				t.Fatalf("RunTaskUntilTerminal() error = %v; result=%+v; strict=%+v", runErr, result, fixture.fake.loadState(t))
			}

			if test.kind == productionTerminalRestart {
				before := productionTerminalTree(t, filepath.Join(fixture.repo, ".revolvr", "autonomous", "task-runs", fixture.operationID))
				result, runErr = RunTaskUntilTerminal(context.Background(), Config{WorkDir: fixture.repo}, fixture.input(nil))
				if runErr != nil {
					t.Fatalf("restart RunTaskUntilTerminal() error = %v; result=%+v", runErr, result)
				}
				after := productionTerminalTree(t, filepath.Join(fixture.repo, ".revolvr", "autonomous", "task-runs", fixture.operationID))
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("restart changed immutable task-run evidence\nbefore=%v\nafter=%v", before, after)
				}
			}

			if result.StopReason != test.wantStop || result.OperationID != fixture.operationID || result.TaskID != fixture.taskID || result.Statistics.CyclesStarted != test.wantCycles || result.Statistics.CyclesCompleted != test.wantCycles || result.Replayed != test.wantReplayed {
				t.Fatalf("terminal result = %+v, want stop=%s cycles=%d replayed=%v", result, test.wantStop, test.wantCycles, test.wantReplayed)
			}
			if !strings.Contains(result.StopDetail, test.wantDetail) {
				t.Fatalf("stop detail = %q, want material %q", result.StopDetail, test.wantDetail)
			}
			wantFake := strictFakeCodexState{SchemaVersion: strictFakeCodexStateSchema, VersionInvocations: test.wantVersionCalls, NextInvocation: test.wantInvocations, OutputSequence: append([]string(nil), fixture.contract.OutputSequence...)}
			if got := fixture.fake.loadState(t); !reflect.DeepEqual(got, wantFake) {
				t.Fatalf("strict fake state = %+v, want %+v", got, wantFake)
			}
			fixture.assertOutcome(t, result, test)
		})
	}
}

type productionTerminalKind string

const (
	productionTerminalNeedsInput          productionTerminalKind = "needs-input"
	productionTerminalBlock               productionTerminalKind = "authorized-block"
	productionTerminalVerificationFailure productionTerminalKind = "verification-failure"
	productionTerminalNoProgress          productionTerminalKind = "no-progress"
	productionTerminalSafety              productionTerminalKind = "trusted-safety"
	productionTerminalCancellation        productionTerminalKind = "caller-cancellation"
	productionTerminalRestart             productionTerminalKind = "restart-authority"
	productionTerminalMaximumCycle        productionTerminalKind = "maximum-cycle"
)

type productionTerminalFixture struct {
	repo, workspace, taskID, operationID string
	baselineHead, taskBytes              string
	runConfig                            runonce.Config
	releaseManifest                      codexexec.ReleaseManifest
	contract                             strictFakeCodexContract
	fake                                 strictFakeCodexFixture
	now                                  time.Time
	idPrefix                             string
}

func newProductionTerminalFixture(t *testing.T, executable string, manifest codexexec.ReleaseManifest, kind productionTerminalKind) productionTerminalFixture {
	t.Helper()
	name := string(kind)
	taskID := "terminal-" + name
	operationID := taskID + "-operation"
	now := time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC)
	t.Setenv("GIT_AUTHOR_DATE", now.Format(time.RFC3339))
	t.Setenv("GIT_COMMITTER_DATE", now.Format(time.RFC3339))

	repo := t.TempDir()
	initializeSchedulingGitRepository(t, repo)
	for _, profile := range []string{"supervisor", "implementer"} {
		writeProductionHappyFile(t, filepath.Join(repo, ".agent", "profiles", profile+".md"), "Execute only the exact "+profile+" authority.\n")
	}
	writeProductionHappyFile(t, filepath.Join(repo, "docs", "result.md"), "terminal matrix baseline\n")
	task, err := taskfile.ProjectAutonomousTask(repo, taskfile.AutonomousCreateInput{ID: taskID, Title: "Production terminal outcome " + name, Body: "Exercise the exact " + name + " attended-task terminal outcome."})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = taskfile.PublishAutonomousTask(repo, task); err != nil {
		t.Fatal(err)
	}
	seedProductionTerminalState(t, repo, task)
	runSchedulingGit(t, repo, "add", ".agent", "docs")
	runSchedulingGit(t, repo, "commit", "-q", "-m", "Seed terminal matrix "+name)
	baselineHead := runSchedulingGit(t, repo, "rev-parse", "HEAD")
	for _, directory := range []string{"locks", "receipts", "runs"} {
		if err := os.MkdirAll(filepath.Join(repo, ".revolvr", directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	runCfg := DefaultRunOnceConfig(repo)
	runCfg.CodexExecutable = executable
	runCfg.CodexTimeout = 10 * time.Second
	runCfg.VerificationPlan = &autonomousverification.Plan{
		SchemaVersion: autonomousverification.PlanSchemaVersion,
		Tiers: []autonomousverification.Tier{{
			ID: "task-acceptance", Kind: autonomousverification.TierTaskAcceptance,
			RequiredForFinal: true, RunForFinal: true,
			Commands:    []verification.Command{{Name: "sh", Args: []string{"-c", `test "$(cat docs/result.md)" = "verified terminal result"`}}},
			RerunPolicy: autonomousverification.RerunNever,
		}},
	}
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		t.Fatal(err)
	}
	workspace := productionHappyWorkspaceRoot(t, repo, taskID)
	idPrefix := name
	contract := productionTerminalCodexContract(t, repo, workspace, executable, effective, taskID, idPrefix, kind)
	if kind == productionTerminalRestart {
		contract.VersionInvocationCount = 2
	}
	fake := configureStrictFakeCodex(t, executable, repo, contract)
	return productionTerminalFixture{repo: repo, workspace: workspace, taskID: taskID, operationID: operationID, baselineHead: baselineHead, taskBytes: string(task.SourceBytes), runConfig: runCfg, releaseManifest: manifest, contract: contract, fake: fake, now: now, idPrefix: idPrefix}
}

func (fixture productionTerminalFixture) input(progress autonomoustaskrun.Progress) TaskRunInput {
	ids := 0
	clockTicks := int64(-1)
	maxCycles := int64(1)
	if fixture.idPrefix == string(productionTerminalNoProgress) {
		maxCycles = 2
	}
	return TaskRunInput{
		OperationID: fixture.operationID,
		TaskID:      fixture.taskID,
		MaxCycles:   maxCycles,
		Clock: func() time.Time {
			clockTicks++
			return fixture.now.Add(time.Duration(clockTicks) * time.Second)
		},
		Progress:  progress,
		RunConfig: &fixture.runConfig,
		idGenerator: func() string {
			ids++
			return fixture.idPrefix + "-" + leftPadTwo(ids)
		},
		releaseManifest: &fixture.releaseManifest,
	}
}

func seedProductionTerminalState(t *testing.T, repo string, task taskfile.Task) {
	t.Helper()
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        task.ID,
		Lifecycle:     autonomous.LifecycleStateReady,
		Plan: &autonomous.TaskPlan{
			TaskID: task.ID, ID: "terminal-plan", Revision: 1,
			Provenance: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindTask, task.SourcePath, "Exact terminal-matrix task.")},
			Steps:      []autonomous.PlanStep{{ID: "terminal-step", Description: "Exercise the selected terminal boundary.", Status: autonomous.PlanStepStatusPending}},
		},
		AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "terminal-outcome", Requirement: "The selected terminal outcome retains exact evidence.", Status: autonomous.AcceptanceStatusPending}},
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	writeProductionHappyFile(t, filepath.Join(repo, filepath.FromSlash(task.AutonomousStatePath)), string(raw))
}

func productionTerminalCodexContract(t *testing.T, root, workspace, executable string, cfg runonce.Config, taskID, prefix string, kind productionTerminalKind) strictFakeCodexContract {
	t.Helper()
	contract := strictFakeCodexContract{VersionInvocationCount: 1}
	if kind == productionTerminalCancellation {
		return contract
	}
	supervisorSchema, err := supervisor.DecisionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	decision := productionTerminalDecision(t, taskID, kind)
	decisionRaw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	decisionMessage := string(decisionRaw) + "\n"
	events := func(name string) []string {
		return []string{
			`{"type":"thread.started","thread_id":"` + name + `-thread"}`,
			`{"type":"turn.completed","final_message":"` + name + ` completed","usage":{"input_tokens":9,"output_tokens":4,"duration_seconds":1}}`,
		}
	}
	supervisorInvocation := func(name, runID string) strictFakeCodexInvocation {
		base := filepath.ToSlash(filepath.Join(".revolvr", "runs", runID))
		schemaPath := filepath.Join(root, filepath.FromSlash(base), "supervisor-output-schema.json")
		writeProductionHappyFile(t, schemaPath, string(supervisorSchema))
		return strictFakeCodexInvocation{
			Name: name, WorkingDirectory: workspace,
			PromptPath:   filepath.Join(root, filepath.FromSlash(base), "supervisor-prompt.md"),
			OutputSchema: &strictFakeCodexMaterial{Path: schemaPath, Content: string(supervisorSchema)},
			Argv: productionHappyInvocation(t, cfg, executable, workspace, root, codexexec.ArtifactPaths{
				StdoutJSONL: base + "/codex.jsonl", Stderr: base + "/codex.stderr", LastMessage: base + "/supervisor-output.json",
			}, base+"/supervisor-output-schema.json"),
			LastMessage: decisionMessage, StdoutJSONL: events(name), OutputEventTypes: []string{"thread.started", "turn.completed"},
		}
	}
	workerInvocation := func(name, runID string, writes []strictFakeCodexMaterial) strictFakeCodexInvocation {
		base := filepath.ToSlash(filepath.Join(".revolvr", "runs", runID))
		return strictFakeCodexInvocation{
			Name: name, WorkingDirectory: workspace,
			PromptPath: filepath.Join(root, filepath.FromSlash(base), "worker-prompt.md"),
			Argv: productionHappyInvocation(t, cfg, executable, workspace, root, codexexec.ArtifactPaths{
				StdoutJSONL: base + "/codex.jsonl", Stderr: base + "/codex.stderr", LastMessage: base + "/worker-output.txt",
			}, ""),
			LastMessage: "Executed the exact terminal-matrix worker authority.\n", Writes: writes,
			StdoutJSONL: events(name), OutputEventTypes: []string{"thread.started", "turn.completed"},
		}
	}

	firstSupervisor := supervisorInvocation(prefix+"-supervisor", prefix+"-04")
	if kind == productionTerminalSafety {
		firstSupervisor.Writes = []strictFakeCodexMaterial{{Path: filepath.Join("docs", "supervisor-mutated.txt"), Content: "unauthorized supervisor mutation retained for safety evidence\n"}}
	}
	contract.Invocations = append(contract.Invocations, firstSupervisor)
	if kind == productionTerminalVerificationFailure || kind == productionTerminalNoProgress || kind == productionTerminalMaximumCycle {
		var writes []strictFakeCodexMaterial
		if kind == productionTerminalVerificationFailure {
			writes = []strictFakeCodexMaterial{{Path: filepath.Join("docs", "result.md"), Content: "verification must fail\n"}}
		}
		if kind == productionTerminalMaximumCycle {
			writes = []strictFakeCodexMaterial{{Path: filepath.Join("docs", "result.md"), Content: "verified terminal result\n"}}
		}
		contract.Invocations = append(contract.Invocations, workerInvocation(prefix+"-worker", prefix+"-05", writes))
	}
	if kind == productionTerminalNoProgress {
		contract.Invocations = append(contract.Invocations, supervisorInvocation(prefix+"-repeat-supervisor", prefix+"-09"))
	}
	for _, invocation := range contract.Invocations {
		contract.OutputSequence = append(contract.OutputSequence, invocation.Name+":thread.started", invocation.Name+":turn.completed")
	}
	return contract
}

func productionTerminalDecision(t *testing.T, taskID string, kind productionTerminalKind) autonomous.SupervisorDecision {
	t.Helper()
	taskEvidence := productionEvidence(autonomous.EvidenceKindTask, ".agent/tasks/"+taskID+".md", "Exact terminal-matrix task authority.")
	decision := autonomous.SupervisorDecision{TaskID: taskID, Rationale: "Exercise the exact attended terminal boundary.", Inputs: []autonomous.EvidenceReference{taskEvidence}}
	switch kind {
	case productionTerminalNeedsInput, productionTerminalRestart:
		question := autonomous.NeedsInputQuestion{
			TaskID: taskID, QuestionID: "terminal-choice", Revision: 1,
			Question: "Which exact terminal behavior should the operator authorize?", BlockingReason: "The task intentionally leaves two mutually exclusive terminal behaviors unresolved.",
			Options:        []autonomous.NeedsInputOption{{ID: "retain", Meaning: "Retain the current behavior."}, {ID: "replace", Meaning: "Replace the current behavior."}},
			Recommendation: autonomous.NeedsInputRecommendation{OptionID: "retain", Rationale: "Retaining current behavior is the conservative choice."},
			Evidence:       []autonomous.EvidenceReference{taskEvidence},
		}
		question.ContentSHA256, _ = autonomous.QuestionContentSHA256(question)
		decision.Action, decision.NeedsInput = autonomous.ActionNeedsInput, &question
	case productionTerminalBlock:
		decision.Action = autonomous.ActionBlock
		decision.Rationale = "The exact external authority required by the task is unavailable."
	case productionTerminalSafety, productionTerminalVerificationFailure, productionTerminalNoProgress, productionTerminalMaximumCycle:
		target := productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact bounded implementation target.")
		decision.Action, decision.WorkerProfile = autonomous.ActionImplement, autonomous.WorkerProfileImplementer
		decision.Inputs = []autonomous.EvidenceReference{target}
		decision.SuccessCriteria = []string{"docs/result.md contains the exact task-authorized result."}
		decision.Strategy = &autonomous.Strategy{Approach: "Modify only the exact bounded result target.", Techniques: []string{"edit docs/result.md"}, Targets: []autonomous.EvidenceReference{target}}
	case productionTerminalCancellation:
		t.Fatal("caller-cancellation must not request a supervisor decision")
	}
	if err := decision.Validate(); err != nil {
		t.Fatal(err)
	}
	return decision
}

func (fixture productionTerminalFixture) assertOutcome(t *testing.T, result autonomoustaskrun.Result, test struct {
	name                 string
	kind                 productionTerminalKind
	wantStop             autonomoustaskrun.StopReason
	wantError            bool
	wantInvocations      int
	wantVersionCalls     int
	wantReceipts         []string
	wantVerification     bool
	wantWorkspaceCommits int
	wantWorkspaceStatus  string
	wantControlStatus    string
	wantLifecycle        autonomous.LifecycleState
	wantAttempts         int
	wantInputQuestions   int
	wantBreaker          autonomous.BreakerReason
	wantDetail           string
	wantCycles           int64
	wantReplayed         bool
}) {
	t.Helper()
	if got := runSchedulingGit(t, fixture.repo, "rev-parse", "HEAD"); got != fixture.baselineHead {
		t.Fatalf("control HEAD = %s, want unchanged %s", got, fixture.baselineHead)
	}
	if got := runSchedulingGit(t, fixture.repo, "status", "--porcelain=v1", "--untracked-files=all"); got != test.wantControlStatus {
		t.Fatalf("control status = %q, want %q", got, test.wantControlStatus)
	}
	workspaceHead := runSchedulingGit(t, fixture.workspace, "rev-parse", "HEAD")
	if got := runSchedulingGit(t, fixture.workspace, "rev-list", "--count", fixture.baselineHead+".."+workspaceHead); got != decimal(test.wantWorkspaceCommits) {
		t.Fatalf("workspace commit count = %q, want %d", got, test.wantWorkspaceCommits)
	}
	if got := runSchedulingGit(t, fixture.workspace, "status", "--porcelain=v1", "--untracked-files=all"); got != test.wantWorkspaceStatus {
		t.Fatalf("workspace status = %q, want %q", got, test.wantWorkspaceStatus)
	}
	task, found, err := taskfile.FindByID(fixture.repo, fixture.taskID)
	if err != nil || !found {
		t.Fatalf("load task found=%v err=%v", found, err)
	}
	if test.kind == productionTerminalBlock {
		if task.Status != taskfile.StatusBlocked || string(task.SourceBytes) == fixture.taskBytes {
			t.Fatalf("authorized block task = status %q bytes_changed=%v", task.Status, string(task.SourceBytes) != fixture.taskBytes)
		}
	} else if task.Status != taskfile.StatusPending || string(task.SourceBytes) != fixture.taskBytes {
		t.Fatalf("unauthorized task effect: status=%q bytes_changed=%v", task.Status, string(task.SourceBytes) != fixture.taskBytes)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.repo})
	if err != nil {
		t.Fatal(err)
	}
	state, found, err := store.Load(context.Background(), fixture.taskID)
	if err != nil || !found {
		t.Fatalf("load state found=%v err=%v", found, err)
	}
	if state.State.Lifecycle != test.wantLifecycle || len(state.State.Attempts.Events) != test.wantAttempts || len(state.State.Input.Questions) != test.wantInputQuestions || len(state.State.Input.Answers) != 0 || len(state.State.Input.Resumes) != 0 || len(state.State.FindingResolutions) != 0 || len(state.State.OptionalRoles) != 0 || state.State.Finalization != nil || state.State.ReopenedFrom != nil || state.State.ChildOf != nil {
		t.Fatalf("terminal state = lifecycle=%s attempts=%+v input=%+v finalization=%+v", state.State.Lifecycle, state.State.Attempts.Events, state.State.Input, state.State.Finalization)
	}
	if test.wantBreaker == "" {
		if state.State.CircuitBreaker != nil {
			t.Fatalf("unexpected circuit breaker = %+v", state.State.CircuitBreaker)
		}
	} else if state.State.CircuitBreaker == nil || state.State.CircuitBreaker.Reason != test.wantBreaker {
		t.Fatalf("circuit breaker = %+v, want %s", state.State.CircuitBreaker, test.wantBreaker)
	}
	wantWorkspaceHead := fixture.baselineHead
	if test.wantWorkspaceCommits != 0 {
		wantWorkspaceHead = workspaceHead
	}
	if state.State.Workspace == nil || state.State.Workspace.HeadSHA != wantWorkspaceHead || state.State.Workspace.Checkpoint.CommitSHA != wantWorkspaceHead {
		t.Fatalf("workspace state = %+v, want head/checkpoint %s", state.State.Workspace, wantWorkspaceHead)
	}
	wantHistory := map[string]int{"workspace": 1}
	switch test.kind {
	case productionTerminalNeedsInput, productionTerminalRestart:
		wantHistory["input"] = 1
	case productionTerminalBlock:
		wantHistory["block"] = 1
	case productionTerminalVerificationFailure:
		wantHistory["attempts"] = 2
	case productionTerminalNoProgress:
		wantHistory["attempts"] = 3
	case productionTerminalMaximumCycle:
		wantHistory["attempts"] = 2
		wantHistory["workspace"] = 2
	}
	if got := productionTerminalHistoryCounts(t, fixture.repo, fixture.taskID); !reflect.DeepEqual(got, wantHistory) {
		t.Fatalf("state history counts = %v, want only %v", got, wantHistory)
	}
	if _, err := os.Stat(filepath.Join(fixture.repo, ".revolvr", "autonomous", "tasks", fixture.taskID, "completion")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unauthorized completion artifacts: %v", err)
	}
	receipts := productionTerminalNames(t, filepath.Join(fixture.repo, ".revolvr", "receipts"))
	if !slices.Equal(receipts, test.wantReceipts) {
		t.Fatalf("receipt files = %v, want %v", receipts, test.wantReceipts)
	}
	verificationPath := filepath.Join(fixture.repo, ".revolvr", "runs", fixture.idPrefix+"-05", "verification.json")
	_, verificationErr := os.Stat(verificationPath)
	if test.wantVerification && verificationErr != nil {
		t.Fatalf("expected verification evidence: %v", verificationErr)
	}
	if !test.wantVerification && !errors.Is(verificationErr, os.ErrNotExist) {
		t.Fatalf("unauthorized verification evidence: %v", verificationErr)
	}
	operation, found, err := autonomoustaskrun.Inspect(fixture.repo, fixture.operationID)
	if err != nil || !found || operation.Stage != "terminal" || operation.StopReason != result.StopReason || operation.CompletedAt == nil {
		t.Fatalf("terminal task-run operation found=%v operation=%+v err=%v", found, operation, err)
	}
	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(fixture.repo, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	runs, err := ledgerStore.ListRecentRunsForTaskWithEvents(context.Background(), fixture.taskID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != test.wantInvocations+1 {
		t.Fatalf("ledger runs = %d, want task-run plus %d exact model runs: %+v", len(runs), test.wantInvocations, runs)
	}
	finalizationPrefix := "finalization-"
	codexStarts, verificationCompletions, commits := 0, 0, 0
	taskRunID := productionTaskRunLedgerID(fixture.operationID)
	var taskRun *ledger.RunWithEvents
	for _, item := range runs {
		if strings.HasPrefix(item.Run.ID, finalizationPrefix) {
			t.Fatalf("unauthorized terminal ledger effect = %+v", item.Run)
		}
		if item.Run.ID == taskRunID {
			value := item
			taskRun = &value
		}
		for _, event := range item.Events {
			switch event.Type {
			case ledger.EventCodexStarted:
				codexStarts++
			case ledger.EventVerificationCompleted:
				verificationCompletions++
			case ledger.EventCommitCreated:
				commits++
			case ledger.EventFinalizationPrepared, ledger.EventFinalizationMaterialized, ledger.EventFinalizationStateTerminal, ledger.EventFinalizationCompleted:
				t.Fatalf("unauthorized finalization event = %+v", event)
			}
		}
	}
	wantVerificationCompletions := 0
	if test.wantVerification {
		wantVerificationCompletions = 1
	}
	if codexStarts != test.wantInvocations || verificationCompletions != wantVerificationCompletions || commits != test.wantWorkspaceCommits {
		t.Fatalf("ledger effect counts = codex %d verification %d commits %d; want %d/%d/%d", codexStarts, verificationCompletions, commits, test.wantInvocations, wantVerificationCompletions, test.wantWorkspaceCommits)
	}
	if taskRun == nil || taskRun.Run.Status != ledger.StatusCompleted || len(taskRun.Events) != int(2*test.wantCycles+2) {
		t.Fatalf("task-run ledger = %+v, want one immutable admitted/cycle/stopped chain", taskRun)
	}
}

func productionTerminalHistoryCounts(t *testing.T, root, taskID string) map[string]int {
	t.Helper()
	history := filepath.Join(root, ".revolvr", "autonomous", "tasks", taskID, "history")
	counts := map[string]int{}
	err := filepath.WalkDir(history, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(history, path)
		if err != nil {
			return err
		}
		category := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		counts[category]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

func productionTerminalNames(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func productionTerminalTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[filepath.ToSlash(rel)] = info.Mode().String()
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = info.Mode().String() + ":" + productionHash(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
