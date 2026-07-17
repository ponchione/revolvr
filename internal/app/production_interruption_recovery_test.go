package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomousnotification"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func TestProductionTaskInterruptionRecoveryMatrix(t *testing.T) {
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

	points := []struct {
		name  string
		point taskInterruptionPoint
	}{
		{"before_supervisor", taskFailureBeforeSupervisor},
		{"after_supervisor", taskFailureAfterSupervisor},
		{"before_worker", taskFailureBeforeWorker},
		{"after_worker", taskFailureAfterWorker},
		{"before_verification", taskFailureBeforeVerification},
		{"after_verification", taskFailureAfterVerification},
		{"before_commit", taskFailureBeforeCommit},
		{"after_commit", taskFailureAfterCommit},
		{"before_checkpoint", taskFailureBeforeCheckpoint},
		{"after_checkpoint", taskFailureAfterCheckpoint},
		{"before_audit", taskFailureBeforeAudit},
		{"after_audit", taskFailureAfterAudit},
		{"before_finalization", taskFailureBeforeFinalization},
		{"after_finalization", taskFailureAfterFinalization},
	}
	for _, test := range points {
		t.Run("task_"+test.name, func(t *testing.T) {
			fixture := newProductionInterruptionFixture(t, executable, releaseManifest)
			input := fixture.input
			input.failureInjector = func(point taskInterruptionPoint) error {
				if point == test.point {
					panic(productionInterruption{point: string(point)})
				}
				return nil
			}
			requireProductionInterruption(t, string(test.point), func() {
				_, _ = RunTaskUntilTerminal(context.Background(), Config{WorkDir: fixture.root}, input)
			})

			interrupted, found, err := autonomoustaskrun.Inspect(fixture.root, fixture.operationID)
			if err != nil || !found || !interrupted.InFlight || interrupted.Stage != "cycle_started" || interrupted.StopReason != "" {
				t.Fatalf("interrupted operation found=%t operation=%+v err=%v", found, interrupted, err)
			}
			before := productionInterruptionEffects(t, fixture)
			fakeBefore := fixture.fake.loadState(t)

			input.failureInjector = nil
			restarted, err := runTaskUntilTerminal(context.Background(), Config{WorkDir: fixture.root}, input)
			if err != nil || restarted.StopReason != autonomoustaskrun.StopUnsafeAmbiguous || restarted.OperationID != fixture.operationID {
				t.Fatalf("restart result=%+v err=%v", restarted, err)
			}
			after := productionInterruptionEffects(t, fixture)
			if !reflect.DeepEqual(after.domain, before.domain) {
				t.Fatalf("domain effects changed during unsafe restart\nbefore=%+v\nafter=%+v", before.domain, after.domain)
			}
			if after.taskRunStopped != 1 {
				t.Fatalf("task-run terminal events = %d, want exactly one", after.taskRunStopped)
			}
			if got := fixture.fake.loadState(t); !reflect.DeepEqual(got, fakeBefore) {
				t.Fatalf("restart invoked Codex again\nbefore=%+v\nafter=%+v", fakeBefore, got)
			}
			assertNoDuplicateProductionEffects(t, after.domain)

			operationBeforeReplay := productionTerminalTree(t, filepath.Join(fixture.root, ".revolvr", "autonomous", "task-runs", fixture.operationID))
			replayed, err := runTaskUntilTerminal(context.Background(), Config{WorkDir: fixture.root}, input)
			if err != nil || !replayed.Replayed || replayed.StopReason != autonomoustaskrun.StopUnsafeAmbiguous {
				t.Fatalf("terminal replay=%+v err=%v", replayed, err)
			}
			operationAfterReplay := productionTerminalTree(t, filepath.Join(fixture.root, ".revolvr", "autonomous", "task-runs", fixture.operationID))
			if !reflect.DeepEqual(operationAfterReplay, operationBeforeReplay) {
				t.Fatal("terminal replay changed immutable task-run evidence")
			}
		})
	}

	for _, point := range []notificationInterruptionPoint{notificationFailureBeforeDelivery, notificationFailureAfterDelivery} {
		t.Run("notification_"+string(point), func(t *testing.T) {
			testProductionNotificationInterruption(t, point)
		})
	}

	for _, point := range []autonomousarchive.FailurePoint{autonomousarchive.FailureBeforeManifestPublish, autonomousarchive.FailureAfterManifestPublish} {
		t.Run("archive_"+string(point), func(t *testing.T) {
			testProductionArchiveInterruption(t, executable, releaseManifest, point)
		})
	}
}

type productionInterruption struct{ point string }

func requireProductionInterruption(t *testing.T, point string, run func()) {
	t.Helper()
	deferred := false
	func() {
		defer func() {
			value := recover()
			if interruption, ok := value.(productionInterruption); ok && interruption.point == point {
				deferred = true
				return
			}
			if value != nil {
				panic(value)
			}
		}()
		run()
	}()
	if !deferred {
		t.Fatalf("production interruption point %q was not reached", point)
	}
}

type productionInterruptionFixture struct {
	root, workspace, taskID, operationID, baseline string
	now                                            time.Time
	input                                          TaskRunInput
	fake                                           strictFakeCodexFixture
}

func newProductionInterruptionFixture(t *testing.T, executable string, releaseManifest codexexec.ReleaseManifest) productionInterruptionFixture {
	t.Helper()
	const taskID = "production-interruption"
	const operationID = "production-interruption-operation"
	now := time.Date(2026, 7, 16, 23, 0, 0, 0, time.UTC)
	t.Setenv("GIT_AUTHOR_DATE", now.Format(time.RFC3339))
	t.Setenv("GIT_COMMITTER_DATE", now.Format(time.RFC3339))

	root := t.TempDir()
	initializeSchedulingGitRepository(t, root)
	for name, content := range map[string]string{
		"supervisor": "Select exactly one evidence-grounded autonomous action.",
		"documentor": "Update only the exact documentation target.",
		"auditor":    "Independently audit exact source and verification evidence.",
	} {
		writeProductionHappyFile(t, filepath.Join(root, ".agent", "profiles", name+".md"), content+"\n")
	}
	writeProductionHappyFile(t, filepath.Join(root, "docs", "source.md"), "production fixture source\n")
	task, err := taskfile.ProjectAutonomousTask(root, taskfile.AutonomousCreateInput{
		ID: taskID, Title: "Production interruption recovery",
		Body: "Create `docs/result.md` containing the verified production happy-path result, independently audit it, and complete the task.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = taskfile.PublishAutonomousTask(root, task); err != nil {
		t.Fatal(err)
	}
	runSchedulingGit(t, root, "add", ".agent", "docs")
	runSchedulingGit(t, root, "commit", "-q", "-m", "Seed production interruption recovery")
	baseline := runSchedulingGit(t, root, "rev-parse", "HEAD")
	seedProductionHappyAudit(t, root, task, productionHappySourceRevision(t, root), now)
	for _, directory := range []string{"locks", "receipts", "runs"} {
		if err := os.MkdirAll(filepath.Join(root, ".revolvr", directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	runCfg := DefaultRunOnceConfig(root)
	runCfg.CodexExecutable = executable
	runCfg.GitExecutable = DefaultGitExecutable
	runCfg.OperationalBounds.CyclesPerTask = 2
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
	executableIdentity, err := codexexec.InspectExecutable(executable, nil)
	if err != nil {
		t.Fatal(err)
	}
	gitIdentity, err := codexexec.InspectExecutable(runCfg.GitExecutable, nil)
	if err != nil {
		t.Fatal(err)
	}
	runCfg.CodexIdentity = codexexec.CodexExecutableIdentity{Executable: executableIdentity, Version: strictFakeCodexVersion}
	runCfg.GitIdentity = gitIdentity
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		t.Fatal(err)
	}
	workspace := productionHappyWorkspaceRoot(t, root, taskID)
	contract := productionHappyCodexContract(t, root, workspace, executable, effective, taskID)
	contract.VersionInvocationCount = 1
	fake := configureStrictFakeCodex(t, executable, root, contract)
	ids := 0
	clockTicks := int64(-1)
	input := TaskRunInput{
		OperationID: operationID, TaskID: taskID, MaxCycles: 2, RunConfig: &runCfg,
		Clock: func() time.Time {
			clockTicks++
			return now.Add(time.Duration(clockTicks) * time.Second)
		},
		idGenerator: func() string {
			ids++
			return "happy-" + leftPadTwo(ids)
		},
		releaseManifest: &releaseManifest,
	}
	return productionInterruptionFixture{root: root, workspace: workspace, taskID: taskID, operationID: operationID, baseline: baseline, now: now, input: input, fake: fake}
}

type productionDomainEffects struct {
	workspaceCommits, attemptsAdmitted, attemptsCompleted int
	completionArtifacts, finalizationTerminal             int
	notificationSuccess, archives                         int
	receiptArtifacts                                      map[string]string
	taskCompleted                                         bool
}

type productionEffects struct {
	domain         productionDomainEffects
	taskRunStopped int
}

func productionInterruptionEffects(t *testing.T, fixture productionInterruptionFixture) productionEffects {
	t.Helper()
	effects := productionEffects{}
	if _, err := os.Stat(fixture.workspace); err == nil {
		var commits int
		if _, err := fmt.Sscan(runSchedulingGit(t, fixture.workspace, "rev-list", "--count", fixture.baseline+"..HEAD"), &commits); err != nil {
			t.Fatal(err)
		}
		effects.domain.workspaceCommits = commits
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.root})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), fixture.taskID)
	if err != nil || !found {
		t.Fatalf("load interruption state found=%t err=%v", found, err)
	}
	for _, event := range snapshot.State.Attempts.Events {
		switch event.Kind {
		case autonomous.AttemptEventAdmitted:
			effects.domain.attemptsAdmitted++
		case autonomous.AttemptEventCompleted:
			effects.domain.attemptsCompleted++
		}
	}
	task, found, err := taskfile.FindByID(fixture.root, fixture.taskID)
	if err != nil {
		t.Fatal(err)
	}
	effects.domain.taskCompleted = found && task.Status == taskfile.StatusCompleted
	effects.domain.receiptArtifacts = productionTerminalTree(t, filepath.Join(fixture.root, ".revolvr", "receipts"))
	effects.domain.completionArtifacts = countRegularFiles(t, filepath.Join(fixture.root, ".revolvr", "autonomous", "tasks", fixture.taskID, "completion"))
	if summaries, err := autonomousnotification.List(fixture.root); err == nil {
		for _, summary := range summaries {
			if summary.Stage == autonomousnotification.StageSucceeded {
				effects.domain.notificationSuccess++
			}
		}
	} else {
		t.Fatal(err)
	}
	if entries, err := autonomousarchive.List(fixture.root); err == nil {
		effects.domain.archives = len(entries)
	} else {
		t.Fatal(err)
	}
	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(fixture.root, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	history, err := ledgerStore.ListRecentRunsForTaskWithEvents(context.Background(), fixture.taskID, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range history {
		for _, event := range item.Events {
			switch event.Type {
			case ledger.EventFinalizationCompleted:
				effects.domain.finalizationTerminal++
			case ledger.EventTaskRunStopped:
				effects.taskRunStopped++
			}
		}
	}
	return effects
}

func assertNoDuplicateProductionEffects(t *testing.T, effects productionDomainEffects) {
	t.Helper()
	for name, count := range map[string]int{
		"workspace commits":            effects.workspaceCommits,
		"attempt admissions":           effects.attemptsAdmitted,
		"attempt completions":          effects.attemptsCompleted,
		"finalization terminal events": effects.finalizationTerminal,
		"notification success claims":  effects.notificationSuccess,
		"archives":                     effects.archives,
	} {
		if count > 1 {
			t.Fatalf("duplicate %s: %d", name, count)
		}
	}
	if effects.completionArtifacts != 0 && effects.completionArtifacts != 3 {
		t.Fatalf("completion artifact count = %d, want zero or one exact three-file set", effects.completionArtifacts)
	}
}

func countRegularFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			count++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return count
}

func testProductionNotificationInterruption(t *testing.T, point notificationInterruptionPoint) {
	t.Helper()
	root := t.TempDir()
	initializeSchedulingGitRepository(t, root)
	task, err := taskfile.ProjectAutonomousTask(root, taskfile.AutonomousCreateInput{ID: "notification-interruption", Title: "Notification interruption", Body: "Prove stable notification recovery."})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = taskfile.PublishAutonomousTask(root, task); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        task.ID,
		Lifecycle:     autonomous.LifecycleStateReady,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	writeProductionHappyFile(t, filepath.Join(root, filepath.FromSlash(task.AutonomousStatePath)), string(stateRaw))
	hook := filepath.Join(root, "notification-hook")
	if err := os.WriteFile(hook, []byte("fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeProductionHappyFile(t, filepath.Join(root, ".revolvr", "config.yaml"), `
notifications:
  enabled: true
  events: [safety_stop]
  executable: notification-hook
  directory: repository_root
  timeout_seconds: 1
  stdout_cap_bytes: 128
  stderr_cap_bytes: 128
  maximum_attempts: 1
  retry_delay_seconds: 0
verification:
  commands: [{name: go}]
`)
	runSchedulingGit(t, root, "add", ".agent", "notification-hook")
	runSchedulingGit(t, root, "commit", "-q", "-m", "Seed notification interruption")

	now := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	clockTick := int64(-1)
	hookCalls := 0
	var receiverDeliveryIDs []string
	runtime := NotificationRuntime{
		Clock: func() time.Time {
			clockTick++
			return now.Add(time.Duration(clockTick) * time.Second)
		},
		LookPath: func(string) (string, error) { return hook, nil },
		Runner: func(_ context.Context, command runner.Command) runner.Result {
			hookCalls++
			raw, readErr := io.ReadAll(command.Stdin)
			if readErr != nil {
				t.Fatal(readErr)
			}
			payload, decodeErr := autonomousnotification.DecodePayload(raw)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			receiverDeliveryIDs = append(receiverDeliveryIDs, payload.DeliveryID)
			return runner.Result{ExitCode: 0}
		},
		failureInjector: func(got notificationInterruptionPoint) error {
			if got == point {
				panic(productionInterruption{point: string(got)})
			}
			return nil
		},
	}
	steps := 0
	input := TaskRunInput{
		OperationID: "notification-interruption-operation", TaskID: task.ID, MaxCycles: 1,
		Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			steps++
			return autonomoustaskrun.StepResult{StopReason: autonomoustaskrun.StopSafety, StopDetail: "typed safety interruption"}, nil
		},
		Clock:               runtime.Clock,
		NotificationRuntime: runtime,
	}
	requireProductionInterruption(t, string(point), func() {
		_, _ = RunTaskUntilTerminal(context.Background(), Config{WorkDir: root}, input)
	})
	if steps != 1 {
		t.Fatalf("task steps after notification interruption = %d, want one", steps)
	}
	input.NotificationRuntime.failureInjector = nil
	replayed, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: root}, input)
	if err != nil || !replayed.Replayed || replayed.StopReason != autonomoustaskrun.StopSafety {
		t.Fatalf("notification source replay=%+v err=%v", replayed, err)
	}
	if steps != 1 || hookCalls != 1 || len(receiverDeliveryIDs) != 1 {
		t.Fatalf("steps/hooks/delivery IDs = %d/%d/%v, want 1/1/one", steps, hookCalls, receiverDeliveryIDs)
	}
	summaries, err := autonomousnotification.List(root)
	if err != nil || len(summaries) != 1 || summaries[0].Stage != autonomousnotification.StageSucceeded || summaries[0].Attempts != 1 || summaries[0].DeliveryID != receiverDeliveryIDs[0] {
		t.Fatalf("notification summaries=%+v err=%v", summaries, err)
	}
	if _, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: root}, input); err != nil {
		t.Fatal(err)
	}
	if steps != 1 || hookCalls != 1 {
		t.Fatalf("terminal notification replay duplicated effects: steps=%d hooks=%d", steps, hookCalls)
	}
	intent, payload, journal, found, err := autonomousnotification.Inspect(root, summaries[0].DeliveryID)
	if err != nil || !found || intent.DeliveryID != payload.DeliveryID || journal.DeliveryID != payload.DeliveryID || journal.Stage != autonomousnotification.StageSucceeded || len(journal.Attempts) != 1 {
		t.Fatalf("notification evidence found=%t intent=%+v payload=%+v journal=%+v err=%v", found, intent, payload, journal, err)
	}
}

func testProductionArchiveInterruption(t *testing.T, executable string, releaseManifest codexexec.ReleaseManifest, point autonomousarchive.FailurePoint) {
	t.Helper()
	fixture := newProductionInterruptionFixture(t, executable, releaseManifest)
	completed, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: fixture.root}, fixture.input)
	if err != nil || completed.StopReason != autonomoustaskrun.StopCompleted {
		t.Fatalf("prepare completed task result=%+v err=%v", completed, err)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: fixture.root})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), fixture.taskID)
	if err != nil || !found || snapshot.State.Finalization == nil || snapshot.State.Terminal == nil {
		t.Fatalf("completed archive authority found=%t state=%+v err=%v", found, snapshot.State, err)
	}
	frozenRaw, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(snapshot.State.Finalization.FrozenEvidence.Path)))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := autonomousfinalization.DecodeFrozen(frozenRaw)
	if err != nil {
		t.Fatal(err)
	}
	archiveInput := ArchiveTaskInput{
		TaskID: fixture.taskID, OperationID: "production-interruption-archive", ArchiveRunID: "production-interruption-archive-run",
		Disposition: autonomousarchive.DispositionCompleted, Reason: snapshot.State.Terminal.Reason,
		Provenance: "ext14:explicit-administration", TerminalAt: frozen.TerminalAt, ArchivedAt: fixture.now.Add(2 * time.Hour),
		FailureInjector: func(got autonomousarchive.FailurePoint) error {
			if got == point {
				panic(productionInterruption{point: string(got)})
			}
			return nil
		},
	}
	requireProductionInterruption(t, string(point), func() {
		_, _ = ArchiveTask(context.Background(), Config{WorkDir: fixture.root}, archiveInput)
	})
	archiveInput.FailureInjector = nil
	preReplayHead := runSchedulingGit(t, fixture.root, "rev-parse", "HEAD")
	result, err := ArchiveTask(context.Background(), Config{WorkDir: fixture.root}, archiveInput)
	if err != nil || !result.Replayed || result.Journal.Stage != autonomousarchive.StageLedgerComplete {
		t.Fatalf("archive restart result=%+v err=%v", result, err)
	}
	postReplayHead := runSchedulingGit(t, fixture.root, "rev-parse", "HEAD")
	var commitCount int
	if _, err := fmt.Sscan(runSchedulingGit(t, fixture.root, "rev-list", "--count", preReplayHead+".."+postReplayHead), &commitCount); err != nil || commitCount != 1 {
		t.Fatalf("archive restart commit count=%d err=%v", commitCount, err)
	}
	entries, err := autonomousarchive.List(fixture.root)
	if err != nil || len(entries) != 1 || entries[0].Manifest.ArchiveID != result.Entry.Manifest.ArchiveID {
		t.Fatalf("archive entries=%+v err=%v", entries, err)
	}
	if _, found, err := taskfile.FindByID(fixture.root, fixture.taskID); err != nil || found {
		t.Fatalf("active archived task found=%t err=%v", found, err)
	}
	archiveTree := productionTerminalTree(t, filepath.Join(fixture.root, ".agent", "archive"))
	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(fixture.root, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	history, found, err := ledgerStore.GetRunWithEvents(context.Background(), archiveInput.ArchiveRunID)
	if closeErr := ledgerStore.Close(); err == nil {
		err = closeErr
	}
	if err != nil || !found {
		t.Fatalf("archive ledger found=%t err=%v", found, err)
	}
	completedEvents := 0
	for _, event := range history.Events {
		if event.Type == ledger.EventArchiveCompleted {
			completedEvents++
		}
	}
	if completedEvents != 1 {
		t.Fatalf("archive completed events=%d, want one", completedEvents)
	}
	replayed, err := ArchiveTask(context.Background(), Config{WorkDir: fixture.root}, archiveInput)
	if err != nil || !replayed.Replayed || replayed.CommitSHA != result.CommitSHA {
		t.Fatalf("completed archive replay=%+v err=%v", replayed, err)
	}
	if got := runSchedulingGit(t, fixture.root, "rev-parse", "HEAD"); got != postReplayHead {
		t.Fatalf("completed archive replay changed HEAD from %s to %s", postReplayHead, got)
	}
	if got := productionTerminalTree(t, filepath.Join(fixture.root, ".agent", "archive")); !reflect.DeepEqual(got, archiveTree) {
		t.Fatal("completed archive replay changed immutable archive evidence")
	}
}
