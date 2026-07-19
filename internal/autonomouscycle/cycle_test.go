package autonomouscycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousassembly"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/prompt"
	"revolvr/internal/receipt"
	"revolvr/internal/runner"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func assertCycleArtifact(t *testing.T, root string, artifact Artifact) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.Path)))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := fmt.Sprintf("%x", sum); got != artifact.SHA256 || len(raw) != artifact.ByteSize {
		t.Fatalf("artifact %s identity=%s/%d want=%s/%d", artifact.Path, got, len(raw), artifact.SHA256, artifact.ByteSize)
	}
}

func TestRunEveryWorkerActionUsesExactProfileAndOneFreshWorker(t *testing.T) {
	tests := []struct {
		action           autonomous.Action
		profile          autonomous.WorkerProfile
		changes          bool
		wantOutcome      Outcome
		wantVerification int
		wantCommit       int
	}{
		{autonomous.ActionPlan, autonomous.WorkerProfilePlanner, false, OutcomeReadOnlyCompleted, 0, 0},
		{autonomous.ActionImplement, autonomous.WorkerProfileImplementer, true, OutcomeVerifiedChangesCommitted, 1, 1},
		{autonomous.ActionAudit, autonomous.WorkerProfileAuditor, false, OutcomeReadOnlyCompleted, 0, 0},
		{autonomous.ActionCorrect, autonomous.WorkerProfileCorrector, true, OutcomeVerifiedChangesCommitted, 1, 1},
		{autonomous.ActionDocument, autonomous.WorkerProfileDocumentor, false, OutcomeWorkerNoChanges, 0, 0},
		{autonomous.ActionSimplify, autonomous.WorkerProfileSimplifier, false, OutcomeWorkerNoChanges, 0, 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			fixture := newCycleFixture(t, tt.action)
			if tt.changes {
				fixture.withChangedWorker()
			}
			stateBefore := mustJSON(t, fixture.cfg.State)
			taskBefore := append([]byte(nil), fixture.task.SourceBytes...)

			result, err := Run(context.Background(), fixture.cfg)
			if err != nil {
				t.Fatalf("Run() error = %v\nresult = %+v", err, result)
			}
			if result.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome, tt.wantOutcome)
			}
			if fixture.supervisorCalls != 1 || fixture.policyCalls != 1 || fixture.codexCalls != 1 {
				t.Fatalf("calls supervisor/policy/codex = %d/%d/%d, want 1/1/1", fixture.supervisorCalls, fixture.policyCalls, fixture.codexCalls)
			}
			if fixture.verificationCalls != tt.wantVerification || fixture.commitCalls != tt.wantCommit {
				t.Fatalf("verification/commit calls = %d/%d, want %d/%d", fixture.verificationCalls, fixture.commitCalls, tt.wantVerification, tt.wantCommit)
			}
			if result.Worker.Profile.Name != string(tt.profile) || result.Worker.Action != tt.action {
				t.Fatalf("worker profile/action = %q/%q, want %q/%q", result.Worker.Profile.Name, result.Worker.Action, tt.profile, tt.action)
			}
			if result.Worker.RunID == result.Supervisor.RunID || result.Worker.RunID == "" {
				t.Fatalf("worker run ID %q is not distinct from supervisor %q", result.Worker.RunID, result.Supervisor.RunID)
			}
			if result.Worker.Artifacts.Prompt.Path == result.Supervisor.Artifacts.Prompt.Path || !strings.Contains(result.Worker.Artifacts.Prompt.Path, result.Worker.RunID) {
				t.Fatalf("worker/supervisor prompt paths are not isolated: worker=%q supervisor=%q", result.Worker.Artifacts.Prompt.Path, result.Supervisor.Artifacts.Prompt.Path)
			}
			assertCycleArtifact(t, fixture.root, result.Worker.Artifacts.Dossier)
			assertCycleArtifact(t, fixture.root, result.Worker.Artifacts.DossierManifest)
			dossierRaw, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(result.Worker.Artifacts.Dossier.Path)))
			if err != nil || !strings.Contains(fixture.workerPrompt, string(dossierRaw)) {
				t.Fatalf("exact worker dossier is not retained in sent prompt: %v", err)
			}
			manifestRaw, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(result.Worker.Artifacts.DossierManifest.Path)))
			var manifest autonomous.TaskDossierManifest
			if err != nil || json.Unmarshal(manifestRaw, &manifest) != nil || manifest.Projection == nil || manifest.Projection.Role != autonomous.DossierRole(tt.profile) || manifest.DossierSHA256 != result.Worker.Artifacts.Dossier.SHA256 {
				t.Fatalf("worker dossier manifest mismatch: err=%v manifest=%+v", err, manifest)
			}
			if !result.Worker.Invocation.Ephemeral || containsArg(result.Worker.Invocation.Argv, "resume") || countArg(result.Worker.Invocation.Argv, "exec") != 1 {
				t.Fatalf("worker invocation is not one fresh ephemeral exec: %+v", result.Worker.Invocation)
			}
			if tt.action == autonomous.ActionPlan || tt.action == autonomous.ActionAudit || tt.action == autonomous.ActionCorrect {
				if result.Worker.Artifacts.OutputSchema == nil || result.Worker.Artifacts.OutputSchema.SHA256 == "" || fixture.workerOutputSchema == "" || !containsArg(result.Worker.Invocation.Argv, "--output-schema") {
					t.Fatalf("structured output schema evidence is incomplete: artifact=%+v schema=%q invocation=%+v", result.Worker.Artifacts.OutputSchema, fixture.workerOutputSchema, result.Worker.Invocation)
				}
				wantSuffix := "planner-output.raw.json"
				if tt.action == autonomous.ActionAudit {
					wantSuffix = "auditor-output.raw.json"
				} else if tt.action == autonomous.ActionCorrect {
					wantSuffix = "corrector-output.raw.json"
				}
				if !strings.HasSuffix(result.Worker.Artifacts.Output.Path, wantSuffix) {
					t.Fatalf("structured raw output path = %q, want suffix %q", result.Worker.Artifacts.Output.Path, wantSuffix)
				}
				gotSchema, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(result.Worker.Artifacts.OutputSchema.Path)))
				if err != nil {
					t.Fatal(err)
				}
				wantSchema, err := autonomousplanning.PlanningOutputSchema()
				if tt.action == autonomous.ActionAudit {
					wantSchema, err = autonomousaudit.AuditOutputSchema()
				} else if tt.action == autonomous.ActionCorrect {
					wantSchema, err = autonomous.CorrectionOutputSchema()
				}
				if err != nil || !reflect.DeepEqual(gotSchema, wantSchema) {
					t.Fatalf("structured schema bytes mismatch: %v", err)
				}
			} else if result.Worker.Artifacts.OutputSchema != nil || fixture.workerOutputSchema != "" || containsArg(result.Worker.Invocation.Argv, "--output-schema") {
				t.Fatalf("schema-free action received structured schema evidence: artifact=%+v schema=%q invocation=%+v", result.Worker.Artifacts.OutputSchema, fixture.workerOutputSchema, result.Worker.Invocation)
			}
			for _, want := range []string{string(tt.action), string(tt.profile), fixture.decisionID, result.Source.AdmissionRevision, "must not route or start another worker", string(fixture.task.SourceBytes)} {
				if !strings.Contains(fixture.workerPrompt, want) {
					t.Fatalf("worker prompt missing %q:\n%s", want, fixture.workerPrompt)
				}
			}
			if tt.action == autonomous.ActionCorrect {
				if !strings.Contains(fixture.workerPrompt, "finding-one") || !strings.Contains(fixture.workerPrompt, "Exclusive Correction Authority") {
					t.Fatalf("corrector prompt lacks cited finding scope:\n%s", fixture.workerPrompt)
				}
			}
			if !reflect.DeepEqual(mustJSON(t, fixture.cfg.State), stateBefore) {
				t.Fatal("input execution state mutated")
			}
			if !reflect.DeepEqual(fixture.task.SourceBytes, taskBefore) {
				t.Fatal("input task bytes mutated")
			}
			if !fixture.lock.released {
				t.Fatal("worker source lock was not released")
			}
		})
	}
}

func TestWorkerAdmissionRunsAfterPolicyBeforeWorker(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.withChangedWorker()
	want := errors.New("attempt budget exhausted")
	called := false
	fixture.cfg.BeforeWorker = func(_ context.Context, input WorkerAdmissionInput) error {
		called = true
		if input.TaskID != "task-1" || input.Decision.Action != autonomous.ActionImplement || input.Route.Kind != autonomouspolicy.RouteKindWorker || input.SourceRevision == "" {
			t.Fatalf("admission input = %+v", input)
		}
		return want
	}
	result, err := Run(context.Background(), fixture.cfg)
	if !errors.Is(err, want) || !called {
		t.Fatalf("Run err=%v called=%t", err, called)
	}
	if result.Worker.Started || result.Outcome != OutcomePolicyRejected || result.Failure == nil || result.Failure.Stage != "attempt_admission" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunHeartbeatFailureCancelsWorkerAndPreventsVerification(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	heartbeatErr := errors.New("injected autonomous heartbeat failure")
	fixture.lock.heartbeatErrAt = 3
	fixture.lock.heartbeatErr = heartbeatErr
	fixture.cfg.SourceWriterLockHeartbeatInterval = 100 * time.Millisecond
	fixture.cfg.CodexRunner = func(ctx context.Context, _ codexexec.Config) (codexexec.Result, error) {
		fixture.codexCalls++
		<-ctx.Done()
		return codexexec.Result{ExitCode: -1, Err: context.Cause(ctx)}, context.Cause(ctx)
	}

	result, err := Run(context.Background(), fixture.cfg)
	if !errors.Is(err, lock.ErrOwnershipLost) || !errors.Is(err, heartbeatErr) {
		t.Fatalf("Run error = %v, want ownership and heartbeat failures", err)
	}
	if result.Outcome != OutcomeWorkerFailed || result.Failure == nil || fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
		t.Fatalf("result=%+v verification=%d commit=%d", result, fixture.verificationCalls, fixture.commitCalls)
	}
	if !fixture.lock.released {
		t.Fatal("failed lease was not released")
	}
}

func TestRunSettlesCancellationRacingWithHeartbeatFailureBeforeTerminalEvidence(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	taskPath := filepath.Join(fixture.root, filepath.FromSlash(fixture.task.SourcePath))
	writeFixtureFile(t, taskPath, fixture.task.SourceBytes)
	taskBefore, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	armHeartbeat := make(chan struct{})
	heartbeatStarted := make(chan struct{})
	heartbeatCanceled := make(chan struct{})
	allowHeartbeatReturn := make(chan struct{})
	codexReturned := make(chan struct{})
	var startedOnce sync.Once
	var canceledOnce sync.Once
	persistenceErr := errors.New("autonomous heartbeat persistence failed during cancellation")
	fixture.lock.heartbeatFunc = func(ctx context.Context) error {
		select {
		case <-armHeartbeat:
		default:
			return nil
		}
		startedOnce.Do(func() { close(heartbeatStarted) })
		<-ctx.Done()
		canceledOnce.Do(func() { close(heartbeatCanceled) })
		<-allowHeartbeatReturn
		return errors.Join(ctx.Err(), persistenceErr)
	}
	fixture.cfg.SourceWriterLockHeartbeatInterval = time.Millisecond
	fixture.cfg.CodexRunner = func(ctx context.Context, _ codexexec.Config) (codexexec.Result, error) {
		fixture.codexCalls++
		close(armHeartbeat)
		select {
		case <-heartbeatStarted:
		case <-time.After(time.Second):
			return codexexec.Result{}, errors.New("heartbeat did not start")
		}
		cancel()
		<-heartbeatCanceled
		close(codexReturned)
		return codexexec.Result{ExitCode: -1, Err: ctx.Err()}, ctx.Err()
	}
	type cycleReturn struct {
		result Result
		err    error
	}
	returned := make(chan cycleReturn, 1)
	go func() {
		result, runErr := Run(parent, fixture.cfg)
		returned <- cycleReturn{result: result, err: runErr}
	}()
	select {
	case <-codexReturned:
	case <-time.After(time.Second):
		t.Fatal("Codex runner did not return")
	}
	select {
	case premature := <-returned:
		t.Fatalf("Run returned before the heartbeat published its failure: %+v %v", premature.result, premature.err)
	case <-time.After(20 * time.Millisecond):
	}
	if taskDuringSettlement, err := os.ReadFile(taskPath); err != nil || string(taskDuringSettlement) != string(taskBefore) {
		t.Fatalf("canonical task changed before heartbeat settlement: err=%v\nbefore=%q\nafter=%q", err, taskBefore, taskDuringSettlement)
	}
	close(allowHeartbeatReturn)
	completed := <-returned
	result, err := completed.result, completed.err
	for _, want := range []error{context.Canceled, lock.ErrOwnershipLost, persistenceErr} {
		if !errors.Is(err, want) {
			t.Fatalf("Run error = %v, missing %v", err, want)
		}
	}
	if result.Outcome != OutcomeWorkerFailed || result.Failure == nil || result.Failure.Stage != "source_lock_after_worker" {
		t.Fatalf("result = %+v", result)
	}
	if result.Worker.Run.Status != ledger.StatusFailed || result.Worker.Receipt.Receipt.Verdict != receipt.VerdictSafetyLimit {
		t.Fatalf("worker terminal evidence run=%+v receipt=%+v", result.Worker.Run, result.Worker.Receipt)
	}
	if fixture.verificationCalls != 0 || fixture.commitCalls != 0 || fixture.lock.releaseCount() != 1 {
		t.Fatalf("verification=%d commit=%d releases=%d, want 0/0/1", fixture.verificationCalls, fixture.commitCalls, fixture.lock.releaseCount())
	}
	if taskAfter, err := os.ReadFile(taskPath); err != nil || string(taskAfter) != string(taskBefore) {
		t.Fatalf("canonical task bytes changed after ownership failure: err=%v\nbefore=%q\nafter=%q", err, taskBefore, taskAfter)
	}
	events := fixture.ledger.events[result.Worker.RunID]
	if len(events) == 0 || events[len(events)-1].Type != ledger.EventRunFailed || !strings.Contains(string(events[len(events)-1].Payload), string(OutcomeWorkerFailed)) || !strings.Contains(string(events[len(events)-1].Payload), persistenceErr.Error()) {
		t.Fatalf("terminal worker ledger events = %+v", events)
	}
}

func TestRunOwnershipReplacementBeforeCommitFailsClosed(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.withChangedWorker()
	fixture.cfg.SourceWriterLockHeartbeatInterval = time.Hour
	fixture.lock.heartbeatErrAt = 6
	fixture.lock.heartbeatErr = lock.ErrHeld

	result, err := Run(context.Background(), fixture.cfg)
	if !errors.Is(err, lock.ErrOwnershipLost) || !errors.Is(err, lock.ErrHeld) {
		t.Fatalf("Run error = %v, want replacement-owner failure", err)
	}
	if result.Outcome != OutcomeCommitFailed || result.Failure == nil || result.Failure.Stage != "source_lock_before_commit" {
		t.Fatalf("result = %+v", result)
	}
	if fixture.verificationCalls != 1 || fixture.commitCalls != 0 {
		t.Fatalf("verification/commit calls = %d/%d, want 1/0", fixture.verificationCalls, fixture.commitCalls)
	}
}

func TestRunReleaseFailurePreventsSuccessfulTerminalReturn(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionPlan)
	releaseErr := errors.New("injected autonomous release failure")
	fixture.lock.releaseErr = releaseErr
	result, err := Run(context.Background(), fixture.cfg)
	if !errors.Is(err, releaseErr) {
		t.Fatalf("Run error = %v, want release failure", err)
	}
	if result.Outcome != OutcomeSourceChanged || result.Failure == nil || result.Failure.Stage != "worker_lock" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRunRejectsIgnoredStateAtEveryExecutionBoundary(t *testing.T) {
	tests := []struct {
		name             string
		prepare          func(*cycleFixture)
		wantOutcome      Outcome
		wantStage        string
		wantCodex        int
		wantVerification int
	}{
		{
			name: "pre-existing input",
			prepare: func(f *cycleFixture) {
				writeFixtureFile(f.t, filepath.Join(f.cfg.Workspace.ExecutionRoot, "preexisting.env"), []byte("admission secret\n"))
			},
			wantOutcome: OutcomeDossierFailed, wantStage: "dossier_source_before",
		},
		{
			name: "worker-created input",
			prepare: func(f *cycleFixture) {
				base := f.cfg.CodexRunner
				f.cfg.CodexRunner = func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
					result, err := base(ctx, cfg)
					writeFixtureFile(f.t, filepath.Join(cfg.WorkingDir, "worker.env"), []byte("worker secret\n"))
					return result, err
				}
			},
			wantOutcome: OutcomeWorkerFailed, wantStage: "worker_source_after", wantCodex: 1,
		},
		{
			name: "verification-created input",
			prepare: func(f *cycleFixture) {
				f.receiptChangedFiles = []string{"tracked.txt"}
				f.receiptVerification = []receipt.VerificationEntry{{Command: "fake-verify", ExitCode: 0, Status: "passed"}}
				baseCodex := f.cfg.CodexRunner
				f.cfg.CodexRunner = func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
					result, err := baseCodex(ctx, cfg)
					writeFixtureFile(f.t, filepath.Join(cfg.WorkingDir, "tracked.txt"), []byte("worker change\n"))
					return result, err
				}
				baseVerification := f.cfg.VerificationRunner
				f.cfg.VerificationRunner = func(ctx context.Context, cfg verification.Config) (verification.Result, error) {
					result, err := baseVerification(ctx, cfg)
					writeFixtureFile(f.t, filepath.Join(cfg.WorkingDir, "verification.env"), []byte("verification secret\n"))
					return result, err
				}
			},
			wantOutcome: OutcomeVerificationFailed, wantStage: "verification_source_after", wantCodex: 1, wantVerification: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionImplement)
			fixture.useRealSourceRepository()
			tt.prepare(fixture)

			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != tt.wantOutcome || result.Failure == nil || result.Failure.Stage != tt.wantStage {
				t.Fatalf("result=%+v error=%v, want %q/%q", result, err, tt.wantOutcome, tt.wantStage)
			}
			if !strings.Contains(err.Error(), "classification=policy_relevant") || strings.Contains(err.Error(), " secret") {
				t.Fatalf("ignored-state diagnostic is unsafe or incomplete: %v", err)
			}
			if fixture.codexCalls != tt.wantCodex || fixture.verificationCalls != tt.wantVerification || fixture.commitCalls != 0 {
				t.Fatalf("calls codex/verification/commit=%d/%d/%d, want %d/%d/0", fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls, tt.wantCodex, tt.wantVerification)
			}
		})
	}
}

func TestRunTieredVerificationPurposeArtifactAndFlakyAdmission(t *testing.T) {
	for _, action := range []autonomous.Action{autonomous.ActionImplement, autonomous.ActionCorrect, autonomous.ActionDocument, autonomous.ActionSimplify} {
		t.Run(string(action), func(t *testing.T) {
			fixture := newCycleFixture(t, action)
			fixture.withChangedWorker()
			plan := autonomousverification.Plan{SchemaVersion: autonomousverification.PlanSchemaVersion, Tiers: []autonomousverification.Tier{{ID: "structural", Kind: autonomousverification.TierStructural, RequiredForFinal: true, RunForFast: true, RunForFinal: true, Commands: []verification.Command{{Name: "fake-verify"}}, RerunPolicy: autonomousverification.RerunNever}}}
			fixture.cfg.VerificationCommands = nil
			fixture.cfg.VerificationPlan = &plan
			fixture.ids = append(fixture.ids, "attempt-one")
			result, err := Run(context.Background(), fixture.cfg)
			if err != nil {
				t.Fatalf("Run: %v\n%+v", err, result)
			}
			want := autonomousverification.PurposeFinal
			if action == autonomous.ActionCorrect {
				want = autonomousverification.PurposeFast
			}
			if result.Worker.Verification.Tiered == nil || result.Worker.Verification.Tiered.Purpose != want || result.Worker.Artifacts.Verification == nil {
				t.Fatalf("tiered evidence=%+v artifacts=%+v", result.Worker.Verification, result.Worker.Artifacts)
			}
			if want == autonomousverification.PurposeFinal && !result.Worker.Verification.Tiered.Gate.FinalSatisfied {
				t.Fatal("final route did not satisfy final gate")
			}
			if want == autonomousverification.PurposeFast && result.Worker.Verification.Tiered.Gate.FinalSatisfied {
				t.Fatal("fast route claimed final gate")
			}
			if fixture.verificationCalls != 0 || fixture.commitCalls != 1 {
				t.Fatalf("legacy verification/tier commit calls=%d/%d", fixture.verificationCalls, fixture.commitCalls)
			}
			if _, err := os.Stat(filepath.Join(fixture.root, filepath.FromSlash(result.Worker.Artifacts.Verification.Path))); err != nil {
				t.Fatal(err)
			}
		})
	}

	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.withChangedWorker()
	plan := autonomousverification.Plan{SchemaVersion: autonomousverification.PlanSchemaVersion, Tiers: []autonomousverification.Tier{{ID: "structural", Kind: autonomousverification.TierStructural, RequiredForFinal: true, RunForFinal: true, Commands: []verification.Command{{Name: "fake-verify"}}, RerunPolicy: autonomousverification.RerunOnceToClassifyFlaky}}}
	fixture.cfg.VerificationCommands = nil
	fixture.cfg.VerificationPlan = &plan
	fixture.ids = append(fixture.ids, "attempt-one", "attempt-two")
	calls := 0
	fixture.cfg.CommandRunner = func(context.Context, runner.Command) runner.Result {
		calls++
		if calls == 1 {
			return runner.Result{ExitCode: 1, Stderr: "first failure"}
		}
		return runner.Result{ExitCode: 0}
	}
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeVerificationFailed || fixture.commitCalls != 0 || calls != 2 {
		t.Fatalf("flaky result=%+v err=%v commit=%d calls=%d", result, err, fixture.commitCalls, calls)
	}
	if result.Worker.Verification.Tiered == nil || result.Worker.Verification.Tiered.Outcome != autonomousverification.OutcomeFlaky || result.Worker.Receipt.Receipt.VerificationStatus != "failed" || len(result.Worker.Receipt.Receipt.Verification) != 2 {
		t.Fatalf("flaky evidence=%+v receipt=%+v", result.Worker.Verification, result.Worker.Receipt.Receipt)
	}
}

func TestRunVerificationFailureCorrectionPromptRetainsExactAuthority(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionCorrect)
	fixture.withChangedWorker()
	revision, _ := gitstate.PolicySourceRevision(fixture.baseline)
	failed := currentVerificationEvidence("task-1", revision)
	failed.Summary.Status = autonomous.VerificationStatusFailed
	target := autonomous.VerificationFailureTarget{TaskID: "task-1", RunID: failed.Summary.RunID, OccurrenceID: failed.Summary.OccurrenceID, SourceRevision: revision, Status: autonomous.VerificationStatusFailed, Evidence: append([]autonomous.EvidenceReference(nil), failed.Summary.Evidence...)}
	fixture.cfg.Verification, fixture.cfg.Audit, fixture.cfg.CorrectionFailure = &failed, nil, &target
	fixture.decision.FindingIDs = nil
	fixture.decision.VerificationFailure = &target
	fixture.state.FindingResolutions = nil
	fixture.cfg.State = fixture.state
	result, err := Run(context.Background(), fixture.cfg)
	if err != nil || result.Outcome != OutcomeVerifiedChangesCommitted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, want := range []string{target.RunID, target.OccurrenceID, target.SourceRevision, target.Evidence[0].Reference} {
		if !strings.Contains(fixture.workerPrompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestRunRejectsContradictoryFlatAndTieredVerification(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	plan := autonomousverification.AdaptLegacy([]verification.Command{{Name: "other"}})
	fixture.cfg.VerificationPlan = &plan
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeInvalidConfiguration || fixture.supervisorCalls != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestRunTerminalAuthorizationsHaveNoWorkerVerificationCommitOrPersistence(t *testing.T) {
	for _, action := range []autonomous.Action{autonomous.ActionComplete, autonomous.ActionBlock, autonomous.ActionNeedsInput} {
		t.Run(string(action), func(t *testing.T) {
			fixture := newCycleFixture(t, action)
			stateBefore := mustJSON(t, fixture.cfg.State)
			taskBefore := append([]byte(nil), fixture.task.SourceBytes...)
			result, err := Run(context.Background(), fixture.cfg)
			if err != nil {
				t.Fatal(err)
			}
			want := OutcomeCompleteAuthorized
			if action == autonomous.ActionBlock {
				want = OutcomeBlockAuthorized
			} else if action == autonomous.ActionNeedsInput {
				want = OutcomeNeedsInputAuthorized
			}
			if result.Outcome != want || result.Route == nil || result.Route.Action != action {
				t.Fatalf("terminal result = %+v, want outcome %q", result, want)
			}
			if result.Worker.Started || result.Worker.RunID != "" || fixture.codexCalls != 0 || fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
				t.Fatalf("terminal route started work: worker=%+v calls=%d/%d/%d", result.Worker, fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls)
			}
			if fixture.ledger.runCount() != 1 {
				t.Fatalf("ledger run count = %d, want only supervisor", fixture.ledger.runCount())
			}
			if !reflect.DeepEqual(stateBefore, mustJSON(t, fixture.cfg.State)) || !reflect.DeepEqual(taskBefore, fixture.task.SourceBytes) {
				t.Fatal("terminal authorization mutated task or state")
			}
			if fixture.cfg.State.LatestDecision != nil {
				t.Fatal("terminal authorization persisted LatestDecision")
			}
		})
	}
}

func TestRunFailsClosedBeforeSupervisorWhenLifecycleAdmitsNoRouting(t *testing.T) {
	f := newCycleFixture(t, autonomous.ActionImplement)
	f.state.Lifecycle = autonomous.LifecycleStateWorking
	f.cfg.State = f.state

	result, err := Run(context.Background(), f.cfg)
	if err == nil || result.Outcome != OutcomePolicyRejected || result.Failure == nil || result.Failure.Stage != "lifecycle_authority" || !strings.Contains(err.Error(), "operation in flight") {
		t.Fatalf("Run() result=%+v error=%v", result, err)
	}
	if f.supervisorCalls != 0 || f.codexCalls != 0 || f.verificationCalls != 0 || f.commitCalls != 0 {
		t.Fatalf("closed lifecycle calls supervisor/codex/verification/commit = %d/%d/%d/%d", f.supervisorCalls, f.codexCalls, f.verificationCalls, f.commitCalls)
	}
}

func TestRunRejectsPreparationAndRaceFailuresBeforeWorker(t *testing.T) {
	tests := []struct {
		name        string
		prepare     func(*cycleFixture)
		wantOutcome Outcome
		wantStage   string
		wantPolicy  int
	}{
		{
			name: "mixed pass task",
			prepare: func(f *cycleFixture) {
				f.task.Workflow = taskfile.WorkflowMixedPassV1
				f.task.Phase = taskfile.PhaseImplement
				f.task.AutonomousStatePath = ""
			},
			wantOutcome: OutcomeInvalidConfiguration, wantStage: "task",
		},
		{
			name:        "dossier assembly",
			prepare:     func(f *cycleFixture) { f.assembleErr = errors.New("injected assembly failure") },
			wantOutcome: OutcomeDossierFailed, wantStage: "dossier_assembly",
		},
		{
			name:        "source changes during assembly",
			prepare:     func(f *cycleFixture) { f.snapshots = []gitstate.SourceSnapshot{f.baseline, f.changed} },
			wantOutcome: OutcomeSourceChangedDuringDossier, wantStage: "dossier_source_window",
		},
		{
			name:        "supervisor invocation",
			prepare:     func(f *cycleFixture) { f.supervisorErr = errors.New("injected supervisor failure") },
			wantOutcome: OutcomeSupervisorFailed, wantStage: "supervisor",
		},
		{
			name:        "supervisor source mismatch",
			prepare:     func(f *cycleFixture) { f.supervisorSource = f.changed },
			wantOutcome: OutcomeSupervisorFailed, wantStage: "supervisor_evidence",
		},
		{
			name:        "source changes before admission",
			prepare:     func(f *cycleFixture) { f.snapshots = []gitstate.SourceSnapshot{f.baseline, f.baseline, f.changed} },
			wantOutcome: OutcomeSourceChanged, wantStage: "worker_admission_source",
		},
		{
			name:        "policy rejection",
			prepare:     func(f *cycleFixture) { f.policyErr = errors.New("injected policy rejection") },
			wantOutcome: OutcomePolicyRejected, wantStage: "policy", wantPolicy: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionPlan)
			tt.prepare(fixture)
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil {
				t.Fatalf("Run() succeeded: %+v", result)
			}
			if result.Outcome != tt.wantOutcome || result.Failure == nil || result.Failure.Stage != tt.wantStage {
				t.Fatalf("result = %+v, want outcome/stage %q/%q", result, tt.wantOutcome, tt.wantStage)
			}
			if fixture.codexCalls != 0 || fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
				t.Fatalf("failure started worker path: codex/verify/commit=%d/%d/%d", fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls)
			}
			if fixture.policyCalls != tt.wantPolicy {
				t.Fatalf("policy calls = %d, want %d", fixture.policyCalls, tt.wantPolicy)
			}
		})
	}
}

func TestRunMissingExplicitAutonomousTaskReturnsNoTaskStateOutcome(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionPlan)
	fixture.cfg.TaskLoader = func(string, string) (taskfile.Task, bool, error) { return taskfile.Task{}, false, nil }
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeNoTaskState || fixture.supervisorCalls != 0 {
		t.Fatalf("Run() result=%+v error=%v supervisor=%d", result, err, fixture.supervisorCalls)
	}
}

func TestRunRejectsWrongTaskStateAndEvidenceBeforeSupervisor(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "missing state", mutate: func(cfg *Config) { cfg.State = autonomous.ExecutionState{} }, want: "execution state"},
		{name: "wrong state task", mutate: func(cfg *Config) { cfg.State.TaskID = "other" }, want: "execution state"},
		{name: "wrong verification task", mutate: func(cfg *Config) { cfg.Verification.Summary.TaskID = "other" }, want: "verification task_id"},
		{name: "wrong audit task", mutate: func(cfg *Config) { cfg.Audit.Report.TaskID = "other" }, want: "audit task_id"},
		{name: "unknown source safety", mutate: func(cfg *Config) { cfg.SourceSafety = "maybe" }, want: "unknown source safety"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionDocument)
			tt.mutate(&fixture.cfg)
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) || result.Outcome != OutcomeInvalidConfiguration {
				t.Fatalf("Run() result=%+v error=%v, want %q", result, err, tt.want)
			}
			if fixture.supervisorCalls != 0 || fixture.codexCalls != 0 {
				t.Fatalf("invalid evidence started supervisor/worker: %d/%d", fixture.supervisorCalls, fixture.codexCalls)
			}
		})
	}
}

func TestRunRejectsMissingDependenciesFreshSessionConfigAndMalformedPathsBeforeSupervisor(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "ledger", mutate: func(cfg *Config) { cfg.Ledger = nil }, want: "writable ledger"},
		{name: "id generator", mutate: func(cfg *Config) { cfg.IDGenerator = nil }, want: "ID generator"},
		{name: "clock", mutate: func(cfg *Config) { cfg.Clock = nil }, want: "clock"},
		{name: "non ephemeral", mutate: func(cfg *Config) { cfg.CodexEphemeral = false }, want: "fresh ephemeral"},
		{name: "effective config", mutate: func(cfg *Config) { cfg.EffectiveConfigSHA256 = "short" }, want: "effective-config"},
		{name: "verification directory", mutate: func(cfg *Config) { cfg.VerificationCommands[0].Dir = "../outside" }, want: "directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionPlan)
			tt.mutate(&fixture.cfg)
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) || result.Outcome != OutcomeInvalidConfiguration {
				t.Fatalf("Run() result=%+v error=%v, want %q", result, err, tt.want)
			}
			if fixture.supervisorCalls != 0 || fixture.codexCalls != 0 {
				t.Fatalf("invalid configuration started supervisor/worker: %d/%d", fixture.supervisorCalls, fixture.codexCalls)
			}
		})
	}
}

func TestRunInWorkspaceUsesExecutionRootAndKeepsControlEvidenceAtControlRoot(t *testing.T) {
	f := newCycleFixture(t, autonomous.ActionImplement)
	f.withChangedWorker()
	execution := filepath.Join(f.root, ".revolvr", "autonomous", "worktrees", "workspace-one")
	if err := os.MkdirAll(execution, 0o755); err != nil {
		t.Fatal(err)
	}
	source, err := gitstate.PolicySourceRevision(f.baseline)
	if err != nil {
		t.Fatal(err)
	}
	now := f.now
	workspace := autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: f.cfg.TaskID, WorkspaceID: "workspace-one", ControlRoot: f.root, ExecutionRoot: execution, GitCommonDir: filepath.Join(f.root, ".git"), BranchRef: "refs/heads/revolvr/tasks/task-1-workspace", OwnerMarker: filepath.Join(f.root, ".revolvr", "autonomous", "tasks", f.cfg.TaskID, "workspace-owner.json"), BaselineSHA: strings.Repeat("1", 40), HeadSHA: strings.Repeat("1", 40), TreeSHA: strings.Repeat("2", 40), SourceRevision: source, Checkpoint: autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: strings.Repeat("1", 40), TreeSHA: strings.Repeat("2", 40), SourceRevision: source, OperationID: "create-workspace", Provenance: "exact baseline", CreatedAt: now}, Status: autonomous.WorkspaceStatusReady, CreatedAt: now, UpdatedAt: now}
	f.state.Workspace = &workspace
	f.cfg.State = f.state
	f.cfg.Workspace = &workspace

	originalAssembler := f.cfg.DossierAssembler
	f.cfg.DossierAssembler = func(ctx context.Context, in autonomousassembly.Input) (autonomous.TaskDossier, error) {
		if in.RepositoryRoot != f.root || in.ExecutionRoot != execution {
			t.Fatalf("dossier roots = %q / %q", in.RepositoryRoot, in.ExecutionRoot)
		}
		return originalAssembler(ctx, in)
	}
	originalSupervisor := f.cfg.SupervisorRunner
	f.cfg.SupervisorRunner = func(ctx context.Context, cfg supervisor.Config) (supervisor.Result, error) {
		if cfg.RepositoryRoot != f.root || cfg.ExecutionRoot != execution || cfg.WorkspaceID != workspace.WorkspaceID {
			t.Fatalf("supervisor roots = %+v", cfg)
		}
		return originalSupervisor(ctx, cfg)
	}
	originalCodex := f.cfg.CodexRunner
	f.cfg.CodexRunner = func(ctx context.Context, cfg codexexec.Config) (codexexec.Result, error) {
		if cfg.WorkingDir != execution {
			t.Fatalf("Codex working dir = %q", cfg.WorkingDir)
		}
		return originalCodex(ctx, cfg)
	}
	originalVerification := f.cfg.VerificationRunner
	f.cfg.VerificationRunner = func(ctx context.Context, cfg verification.Config) (verification.Result, error) {
		if cfg.WorkingDir != execution {
			t.Fatalf("verification working dir = %q", cfg.WorkingDir)
		}
		return originalVerification(ctx, cfg)
	}
	originalCommit := f.cfg.CommitRunner
	f.cfg.CommitRunner = func(ctx context.Context, cfg commit.Config) (commit.Result, error) {
		if cfg.WorkingDir != execution {
			t.Fatalf("commit working dir = %q", cfg.WorkingDir)
		}
		return originalCommit(ctx, cfg)
	}
	originalLock := f.cfg.LockAcquirer
	f.cfg.LockAcquirer = func(ctx context.Context, cfg lock.Config) (SourceLock, error) {
		if cfg.ControlRoot != f.root || cfg.ExecutionRoot != execution || cfg.WorkspaceID != workspace.WorkspaceID {
			t.Fatalf("source lock authority = %+v", cfg)
		}
		return originalLock(ctx, cfg)
	}
	result, err := RunInWorkspace(context.Background(), f.cfg)
	if err != nil || result.Outcome != OutcomeVerifiedChangesCommitted {
		t.Fatalf("RunInWorkspace = %q, %v", result.Outcome, err)
	}
	if _, err := os.Stat(filepath.Join(f.root, ".revolvr", "runs", result.Worker.RunID, "worker-prompt.md")); err != nil {
		t.Fatalf("control-root artifact missing: %v", err)
	}
	if joined := strings.Join(result.Worker.Invocation.Argv, "\n"); !strings.Contains(joined, filepath.Join(f.root, ".revolvr", "runs")) {
		t.Fatalf("Codex artifact argv is not control-rooted: %v", result.Worker.Invocation.Argv)
	}
}

func TestRunInWorkspaceFailsClosedWithoutWorkspace(t *testing.T) {
	result, err := RunInWorkspace(context.Background(), Config{TaskID: "task-1"})
	if err == nil || result.Outcome != OutcomeInvalidConfiguration || result.Failure == nil || result.Failure.Stage != "workspace" {
		t.Fatalf("RunInWorkspace = %+v, %v", result, err)
	}
}

func TestRunSafetyPreflightFailsBeforeSupervisorOrWorker(t *testing.T) {
	f := newCycleFixture(t, autonomous.ActionImplement)
	f.cfg.SafetyPreflightRunner = func(_ context.Context, input autonomoussafety.Input) (autonomoussafety.Output, error) {
		output, err := testSafetyOutput(t, input)
		output.Preflight.Ready = false
		output.Preflight.Checks = []autonomoussafety.Check{{Name: "network_policy", Status: autonomoussafety.CheckFail, Detail: "proof unavailable"}}
		return output, err
	}
	result, err := Run(context.Background(), f.cfg)
	if err == nil || result.Outcome != OutcomeSafetyPreflightFailed || result.Failure == nil || result.Failure.Stage != "safety_preflight" {
		t.Fatalf("Run = %+v, %v", result, err)
	}
	if f.supervisorCalls != 0 || f.codexCalls != 0 || f.verificationCalls != 0 || f.commitCalls != 0 {
		t.Fatalf("unsafe preflight started work: supervisor/codex/verify/commit=%d/%d/%d/%d", f.supervisorCalls, f.codexCalls, f.verificationCalls, f.commitCalls)
	}
}

func TestRunBindsSameSafetyPolicyToSupervisorAndWorkerProvenance(t *testing.T) {
	f := newCycleFixture(t, autonomous.ActionImplement)
	result, err := Run(context.Background(), f.cfg)
	if err != nil || !result.Worker.Started {
		t.Fatalf("Run = %+v, %v", result, err)
	}
	if result.SafetyPolicy == nil || result.SafetyPreflight == nil || result.SafetyPolicy.PolicySHA256 == "" || result.Supervisor.Invocation.SafetyPolicySHA256 != result.SafetyPolicy.PolicySHA256 || result.Worker.Invocation.SafetyPolicySHA256 != result.SafetyPolicy.PolicySHA256 {
		t.Fatalf("safety provenance = policy=%+v preflight=%+v supervisor=%+v worker=%+v", result.SafetyPolicy, result.SafetyPreflight, result.Supervisor.Invocation, result.Worker.Invocation)
	}
}

func TestRunRejectsModelChangeToProtectedTaskAuthority(t *testing.T) {
	f := newCycleFixture(t, autonomous.ActionImplement)
	protected := testSourceSnapshotAtPath("head-1", "changed\n", "100644 changed 0", ".agent/profiles/implementer.md")
	f.snapshots = []gitstate.SourceSnapshot{f.baseline, f.baseline, f.baseline, protected}
	result, err := Run(context.Background(), f.cfg)
	if err == nil || result.Failure == nil || result.Failure.Stage != "protected_paths" || f.verificationCalls != 0 || f.commitCalls != 0 {
		t.Fatalf("Run = %+v, %v; calls=%d/%d", result, err, f.verificationCalls, f.commitCalls)
	}
}

func TestRunReadOnlyMutationAndWorkerFailurePreserveEvidenceAndSkipFollowups(t *testing.T) {
	for _, test := range []struct {
		name        string
		action      autonomous.Action
		codexFailed bool
		want        Outcome
	}{
		{name: "planner mutation", action: autonomous.ActionPlan, want: OutcomeReadOnlyMutation},
		{name: "auditor mutation", action: autonomous.ActionAudit, want: OutcomeReadOnlyMutation},
		{name: "worker failure", action: autonomous.ActionImplement, codexFailed: true, want: OutcomeWorkerFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCycleFixture(t, test.action)
			fixture.withChangedWorker()
			if test.codexFailed {
				fixture.codexErr = errors.New("worker exploded")
			}
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != test.want {
				t.Fatalf("Run() result=%+v error=%v, want outcome %q", result, err, test.want)
			}
			if fixture.codexCalls != 1 || fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
				t.Fatalf("calls codex/verification/commit=%d/%d/%d", fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls)
			}
			if result.Source.WorkerAfter == nil || len(result.Source.ChangedFiles) != 1 || result.Source.ChangedFiles[0] != "tracked.txt" {
				t.Fatalf("source failure evidence = %+v", result.Source)
			}
			if result.Worker.Receipt.Path == "" || result.Worker.Artifacts.SourceEvidence.SHA256 == "" {
				t.Fatalf("failure artifacts missing: receipt=%+v source=%+v", result.Worker.Receipt, result.Worker.Artifacts.SourceEvidence)
			}
		})
	}
}

func TestRunVerificationFailuresNeverCommitOrStartCorrector(t *testing.T) {
	tests := []struct {
		name   string
		policy verification.MissingCommandsPolicy
		result verification.Result
		runErr error
	}{
		{name: "failed", policy: verification.MissingCommandsFail, result: failedVerification()},
		{name: "timeout", policy: verification.MissingCommandsFail, result: timedOutVerification()},
		{name: "missing fail policy", policy: verification.MissingCommandsFail, result: verification.Result{Status: verification.StatusFailed, MissingCommands: true, Message: "no commands"}},
		{name: "missing pass policy", policy: verification.MissingCommandsPass, result: verification.Result{Status: verification.StatusPassed, Passed: true, MissingCommands: true, Message: "no commands"}},
		{name: "runner error", policy: verification.MissingCommandsFail, runErr: errors.New("verification harness failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionImplement)
			fixture.withChangedWorker()
			fixture.cfg.MissingVerificationPolicy = tt.policy
			fixture.verificationResult = tt.result
			fixture.verificationErr = tt.runErr
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != OutcomeVerificationFailed {
				t.Fatalf("Run() result=%+v error=%v", result, err)
			}
			if fixture.verificationCalls != 1 || fixture.commitCalls != 0 || fixture.codexCalls != 1 {
				t.Fatalf("calls codex/verification/commit=%d/%d/%d", fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls)
			}
			if result.Worker.Verification.OccurrenceID == "" || result.Worker.Verification.SourceRevision != result.Source.WorkerRevision || result.Worker.Verification.Policy == nil {
				t.Fatalf("verification evidence incomplete: %+v", result.Worker.Verification)
			}
			if result.Worker.Verification.Policy.Summary.Status != autonomous.VerificationStatusFailed {
				t.Fatalf("policy verification status = %q", result.Worker.Verification.Policy.Summary.Status)
			}
		})
	}
}

func TestRunCommitOutcomesAndFreshness(t *testing.T) {
	tests := []struct {
		name      string
		commit    commit.Result
		commitErr error
		final     *gitstate.SourceSnapshot
		want      Outcome
		wantError bool
	}{
		{name: "committed", commit: committedResult(), want: OutcomeVerifiedChangesCommitted},
		{name: "refused", commit: commit.Result{Status: commit.StatusRefused, RefusalReason: commit.ReasonPreExistingDirty, Message: "refused"}, want: OutcomeCommitFailed, wantError: true},
		{name: "command failure", commit: commit.Result{Status: commit.StatusFailed, Message: "git commit failed", PreCommitSHA: "head-1", PostCommitSHA: "head-1"}, want: OutcomeCommitFailed, wantError: true},
		{name: "indeterminate", commit: commit.Result{Status: commit.StatusIndeterminate, Message: "post HEAD unavailable", PreCommitSHA: "head-1"}, want: OutcomeCommitFailed, wantError: true},
		{name: "runner error", commitErr: errors.New("commit runner failed"), want: OutcomeCommitFailed, wantError: true},
		{name: "freshness mismatch", commit: committedResult(), final: pointerSnapshot(testSourceSnapshot("head-2", "different final", "index-new")), want: OutcomeCommitFailed, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionImplement)
			fixture.withChangedWorker()
			fixture.commitResult = tt.commit
			fixture.commitErr = tt.commitErr
			if tt.final != nil {
				fixture.snapshots[len(fixture.snapshots)-1] = *tt.final
			}
			result, err := Run(context.Background(), fixture.cfg)
			if (err != nil) != tt.wantError || result.Outcome != tt.want {
				t.Fatalf("Run() result=%+v error=%v, want outcome/error %q/%t", result, err, tt.want, tt.wantError)
			}
			if fixture.commitCalls != 1 || result.Worker.Verification.SourceRevision == "" {
				t.Fatalf("commit/verification evidence = calls %d evidence %+v", fixture.commitCalls, result.Worker.Verification)
			}
			if tt.want == OutcomeVerifiedChangesCommitted {
				if result.Worker.Commit.CommitSHA != "commit-sha" || result.Source.FinalRevision != result.Worker.Verification.SourceRevision {
					t.Fatalf("successful commit freshness evidence = commit %+v source %+v verification %+v", result.Worker.Commit, result.Source, result.Worker.Verification)
				}
			}
		})
	}
}

func TestRunReceiptFallbackAndHarnessFactsAreAuthoritative(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.withChangedWorker()
	fixture.writeReceipt = true
	fixture.receiptChangedFiles = []string{"claimed.txt"}
	fixture.receiptVerification = []receipt.VerificationEntry{{Command: "fake verify", ExitCode: 9, Status: "failed"}}
	result, err := Run(context.Background(), fixture.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Worker.Receipt.Synthesized {
		t.Fatal("valid worker receipt unexpectedly synthesized")
	}
	if got := result.Worker.Receipt.Receipt.ChangedFiles; !reflect.DeepEqual(got, []string{"tracked.txt"}) {
		t.Fatalf("final receipt changed files = %v, want harness observation", got)
	}
	if result.Worker.Receipt.Receipt.VerificationStatus != "passed" || result.Worker.Receipt.Receipt.CommitSHA != "commit-sha" {
		t.Fatalf("final receipt harness fields = %+v", result.Worker.Receipt.Receipt)
	}
	kinds := warningKinds(result.Worker.Receipt.Warnings)
	for _, want := range []string{"changed_files_mismatch", "verification_mismatch", "verdict_mismatch"} {
		if !containsArg(kinds, want) {
			t.Fatalf("receipt warning kinds = %v, want %q", kinds, want)
		}
	}

	missing := newCycleFixture(t, autonomous.ActionDocument)
	missing.writeReceipt = false
	missingResult, err := Run(context.Background(), missing.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !missingResult.Worker.Receipt.Synthesized || !strings.Contains(missingResult.Worker.Receipt.ParseError, "missing") {
		t.Fatalf("missing receipt evidence = %+v", missingResult.Worker.Receipt)
	}

	malformed := newCycleFixture(t, autonomous.ActionDocument)
	malformed.writeReceipt = true
	malformed.malformedReceipt = true
	malformedResult, err := Run(context.Background(), malformed.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !malformedResult.Worker.Receipt.Synthesized || malformedResult.Worker.Receipt.ParseError == "" {
		t.Fatalf("malformed receipt evidence = %+v", malformedResult.Worker.Receipt)
	}
}

func TestRunMissingOrWrongProfileBlocksBeforeWorkerCodex(t *testing.T) {
	for _, test := range []struct {
		name   string
		loader ProfileLoader
	}{
		{name: "missing", loader: func(string, string) (prompt.RunProfile, error) {
			return prompt.RunProfile{}, errors.New("missing profile")
		}},
		{name: "wrong", loader: func(_ string, _ string) (prompt.RunProfile, error) {
			return prompt.RunProfile{Name: "auditor", SourcePath: ".agent/profiles/auditor.md", Description: "wrong"}, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionPlan)
			fixture.cfg.ProfileLoader = test.loader
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != OutcomeWorkerFailed || fixture.codexCalls != 0 || result.Worker.RunID != "" {
				t.Fatalf("Run() result=%+v error=%v codex=%d", result, err, fixture.codexCalls)
			}
			if fixture.ledger.runCount() != 1 {
				t.Fatalf("worker ledger run exists before valid profile: %d runs", fixture.ledger.runCount())
			}
		})
	}
}

func TestRunChangedCaptureFailureBlocksVerificationAndCommit(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.withChangedWorker()
	fixture.changedCaptureErr = errors.New("git status unavailable")
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeChangedCaptureFailed {
		t.Fatalf("Run() result=%+v error=%v", result, err)
	}
	if fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
		t.Fatalf("changed capture failure ran verification/commit: %d/%d", fixture.verificationCalls, fixture.commitCalls)
	}
}

func TestRunWorkerCannotMutateDurableExecutionState(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.workerMutatesState = true
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeWorkerFailed || result.Failure == nil || result.Failure.Stage != "immutable_task_state" {
		t.Fatalf("Run() result=%+v error=%v", result, err)
	}
	if fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
		t.Fatalf("durable state mutation reached verification/commit: %d/%d", fixture.verificationCalls, fixture.commitCalls)
	}
}

func TestRunPreservesWorkerLedgerErrors(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionPlan)
	fixture.codexLedgerErr = errors.New("injected Codex ledger failure")
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Worker.LedgerError == nil || !strings.Contains(err.Error(), "injected Codex ledger failure") {
		t.Fatalf("Run() result=%+v error=%v", result, err)
	}
	if fixture.codexCalls != 1 || fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
		t.Fatalf("ledger failure calls=%d/%d/%d", fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls)
	}
}

func TestRunSupervisorDecisionAndMutationFailuresNeverStartWorker(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*cycleFixture)
	}{
		{name: "missing decision", prepare: func(f *cycleFixture) { f.supervisorMissingDecision = true }},
		{name: "invalid decision", prepare: func(f *cycleFixture) { f.decision.Rationale = "" }},
		{name: "source mutation", prepare: func(f *cycleFixture) { f.supervisorAfter = f.changed }},
		{name: "durable state mutation", prepare: func(f *cycleFixture) { f.supervisorMutatesState = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionPlan)
			tt.prepare(fixture)
			result, err := Run(context.Background(), fixture.cfg)
			if err == nil || result.Outcome != OutcomeSupervisorFailed {
				t.Fatalf("Run() result=%+v error=%v", result, err)
			}
			if fixture.policyCalls != 0 || fixture.codexCalls != 0 || fixture.verificationCalls != 0 || fixture.commitCalls != 0 {
				t.Fatalf("supervisor failure reached route/worker: %d/%d/%d/%d", fixture.policyCalls, fixture.codexCalls, fixture.verificationCalls, fixture.commitCalls)
			}
			if fixture.ledger.runCount() != 1 {
				t.Fatalf("ledger contains worker run after supervisor failure: %d", fixture.ledger.runCount())
			}
		})
	}
}

func TestRunUnsafeAndUnknownBlockRemainSafeStops(t *testing.T) {
	for _, safety := range []autonomouspolicy.SourceSafety{autonomouspolicy.SourceSafetyUnsafe, autonomouspolicy.SourceSafetyUnknown} {
		t.Run(string(safety), func(t *testing.T) {
			fixture := newCycleFixture(t, autonomous.ActionBlock)
			fixture.cfg.SourceSafety = safety
			result, err := Run(context.Background(), fixture.cfg)
			if err != nil || result.Outcome != OutcomeBlockAuthorized || result.Worker.Started {
				t.Fatalf("Run() result=%+v error=%v", result, err)
			}
		})
	}
}

func TestRunPreExistingWorkIsPreservedAndNeverSentToCommit(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionImplement)
	fixture.dirtyPaths = []string{"user-work.txt"}
	commands := make([]runner.Command, 0)
	fixture.cfg.CommandRunner = func(_ context.Context, command runner.Command) runner.Result {
		commands = append(commands, command)
		return runner.Result{ExitCode: 0}
	}
	result, err := Run(context.Background(), fixture.cfg)
	if err == nil || result.Outcome != OutcomeWorkerFailed || fixture.codexCalls != 0 || fixture.commitCalls != 0 {
		t.Fatalf("Run() result=%+v error=%v calls=%d/%d", result, err, fixture.codexCalls, fixture.commitCalls)
	}
	for _, command := range commands {
		for _, forbidden := range []string{"reset", "clean", "restore", "checkout"} {
			if containsArg(command.Args, forbidden) {
				t.Fatalf("forbidden destructive command issued: %+v", command)
			}
		}
	}
	if len(commands) != 0 {
		t.Fatalf("pre-existing dirty refusal should use typed capture, commands=%+v", commands)
	}
}

func TestRunWorkerArtifactsRemainReadableAfterLedgerReopen(t *testing.T) {
	fixture := newCycleFixture(t, autonomous.ActionPlan)
	ledgerPath := filepath.Join(fixture.root, ".revolvr", "ledger.sqlite")
	store, err := ledger.OpenWithClock(context.Background(), ledgerPath, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	fixture.cfg.Ledger = store
	result, err := Run(context.Background(), fixture.cfg)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := ledger.OpenLiveReadOnly(context.Background(), ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	history, found, err := reopened.GetRunWithEvents(context.Background(), result.Worker.RunID)
	if err != nil || !found {
		t.Fatalf("reopen worker history found=%t error=%v", found, err)
	}
	artifacts, found := ledger.RunArtifactsFromEvents(history.Events)
	if !found {
		t.Fatal("worker artifacts not projected from reopened ledger")
	}
	for _, path := range []string{
		artifacts.ContextPayloadPath,
		artifacts.ContextManifestPath,
		artifacts.CodexStdoutJSONLPath,
		artifacts.CodexStderrPath,
		artifacts.LastMessagePath,
		artifacts.ReceiptPath,
	} {
		if strings.TrimSpace(path) == "" {
			t.Fatalf("reopened artifact path is empty: %+v", artifacts)
		}
		if _, err := os.ReadFile(filepath.Join(fixture.root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("read reopened artifact %q: %v", path, err)
		}
	}
}

func TestRunDeterministicWithFixedInputs(t *testing.T) {
	first := newCycleFixture(t, autonomous.ActionBlock)
	second := newCycleFixture(t, autonomous.ActionBlock)
	second.cfg.RepositoryRoot = first.cfg.RepositoryRoot
	workspace := *first.cfg.Workspace
	second.cfg.Workspace = &workspace
	second.cfg.State.Workspace = &workspace
	firstResult, firstErr := Run(context.Background(), first.cfg)
	secondResult, secondErr := Run(context.Background(), second.cfg)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("fixed runs failed: %v / %v", firstErr, secondErr)
	}
	firstJSON := mustJSON(t, deterministicProjection(firstResult))
	secondJSON := mustJSON(t, deterministicProjection(secondResult))
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("fixed result projections differ:\n%s\n%s", firstJSON, secondJSON)
	}
}

type deterministicResult struct {
	Outcome        Outcome
	TaskID         string
	Dossier        string
	SupervisorRun  string
	DecisionID     string
	Route          autonomouspolicy.Route
	SourceRevision string
}

func deterministicProjection(result Result) deterministicResult {
	projection := deterministicResult{Outcome: result.Outcome, TaskID: result.TaskID, Dossier: result.DossierManifest.DossierSHA256, SupervisorRun: result.Supervisor.RunID, SourceRevision: result.Source.FinalRevision}
	if result.Supervisor.DecisionReference != nil {
		projection.DecisionID = result.Supervisor.DecisionReference.DecisionID
	}
	if result.Route != nil {
		projection.Route = *result.Route
	}
	return projection
}

type cycleFixture struct {
	t                         *testing.T
	root                      string
	now                       time.Time
	task                      taskfile.Task
	state                     autonomous.ExecutionState
	baseline                  gitstate.SourceSnapshot
	changed                   gitstate.SourceSnapshot
	committed                 gitstate.SourceSnapshot
	snapshots                 []gitstate.SourceSnapshot
	snapshotIndex             int
	ledger                    *memoryLedger
	lock                      *fakeSourceLock
	cfg                       Config
	decision                  autonomous.SupervisorDecision
	decisionID                string
	assembleErr               error
	supervisorErr             error
	policyErr                 error
	supervisorSource          gitstate.SourceSnapshot
	supervisorAfter           gitstate.SourceSnapshot
	supervisorMissingDecision bool
	supervisorMutatesState    bool
	verificationResult        verification.Result
	verificationErr           error
	commitResult              commit.Result
	commitErr                 error
	codexErr                  error
	codexLedgerErr            error
	changedCaptureErr         error
	dirtyPaths                []string
	workerMutatesState        bool
	writeReceipt              bool
	malformedReceipt          bool
	receiptChangedFiles       []string
	receiptVerification       []receipt.VerificationEntry
	workerPrompt              string
	workerOutputSchema        string
	supervisorCalls           int
	policyCalls               int
	codexCalls                int
	verificationCalls         int
	commitCalls               int
	ids                       []string
	idIndex                   int
}

func newCycleFixture(t *testing.T, action autonomous.Action) *cycleFixture {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	taskRaw := []byte("---\nid: task-1\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/task-1/state.json\n---\n# Exact task\n\nImplement exact behavior.\n")
	task := taskfile.Task{
		ID: "task-1", Title: "Exact task", Status: taskfile.StatusPending,
		Workflow:            taskfile.WorkflowAutonomousV1,
		AutonomousStatePath: ".revolvr/autonomous/tasks/task-1/state.json",
		ContextBody:         string(taskRaw), SourcePath: ".agent/tasks/task-1.md", SourceBytes: taskRaw,
	}
	baseline := testSourceSnapshot("head-1", "baseline\n", "100644 baseline 0")
	changed := testSourceSnapshot("head-1", "changed\n", "100644 baseline 0")
	committed := testSourceSnapshot("head-2", "changed\n", "100644 changed 0")
	fixture := &cycleFixture{
		t: t, root: root, now: now, task: task,
		baseline: baseline, changed: changed, committed: committed,
		snapshots: []gitstate.SourceSnapshot{baseline, baseline, baseline, baseline},
		ledger:    newMemoryLedger(now), lock: &fakeSourceLock{},
		verificationResult: passedVerification(), commitResult: committedResult(),
		writeReceipt:        true,
		receiptChangedFiles: nil,
		receiptVerification: nil,
		ids:                 []string{"supervisor-run", "worker-run", "verify-occurrence"},
	}
	fixture.configureAction(action)
	executionRoot := filepath.Join(root, ".revolvr", "autonomous", "worktrees", "fixture-workspace")
	if err := os.MkdirAll(executionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureSourceRevision, err := gitstate.PolicySourceRevision(baseline)
	if err != nil {
		t.Fatal(err)
	}
	fixtureWorkspace := autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: "task-1", WorkspaceID: "fixture-workspace", ControlRoot: root, ExecutionRoot: executionRoot, GitCommonDir: filepath.Join(root, ".git"), BranchRef: "refs/heads/revolvr/tasks/task-1-fixture", OwnerMarker: filepath.Join(root, ".revolvr", "autonomous", "tasks", "task-1", "workspace-owner.json"), BaselineSHA: strings.Repeat("1", 40), HeadSHA: strings.Repeat("1", 40), TreeSHA: strings.Repeat("2", 40), SourceRevision: fixtureSourceRevision, Checkpoint: autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: strings.Repeat("1", 40), TreeSHA: strings.Repeat("2", 40), SourceRevision: fixtureSourceRevision, OperationID: "fixture-workspace-create", Provenance: "test baseline", CreatedAt: now}, Status: autonomous.WorkspaceStatusReady, CreatedAt: now, UpdatedAt: now}
	fixture.state.Workspace = &fixtureWorkspace
	fixture.supervisorSource = baseline
	fixture.cfg = Config{
		RepositoryRoot:                    root,
		Workspace:                         &fixtureWorkspace,
		TaskID:                            "task-1",
		State:                             fixture.state,
		SafetyDeclaration:                 autonomoussafety.DefaultDeclaration(),
		SourceSafety:                      autonomouspolicy.SourceSafetySafe,
		HistoryPolicy:                     autonomousassembly.HistoryPolicy{},
		Ledger:                            fixture.ledger,
		CodexExecutable:                   "fake-codex",
		CodexModel:                        codexexec.DefaultModel,
		CodexReasoningEffort:              codexexec.DefaultReasoningEffort,
		CodexEphemeral:                    true,
		CodexSandbox:                      "workspace-write",
		CodexApprovalPolicy:               "never",
		CodexVersion:                      "codex-cli test",
		EffectiveConfigSchema:             "revolvr-effective-run-config-v1",
		EffectiveConfigSHA256:             strings.Repeat("a", 64),
		CodexTimeout:                      2 * time.Second,
		CodexStdoutCap:                    1024 * 1024,
		CodexStderrCap:                    1024 * 1024,
		GitExecutable:                     "fake-git",
		GitTimeout:                        time.Second,
		GitStdoutCap:                      1024 * 1024,
		GitStderrCap:                      1024 * 1024,
		VerificationCommands:              []verification.Command{{Name: "fake-verify"}},
		MissingVerificationPolicy:         verification.MissingCommandsFail,
		VerificationTimeout:               time.Second,
		VerificationStdoutCap:             1024 * 1024,
		VerificationStderrCap:             1024 * 1024,
		CommitTimeout:                     time.Second,
		CommitStdoutCap:                   1024 * 1024,
		CommitStderrCap:                   1024 * 1024,
		SourceWriterLockTimeout:           2*time.Second + 2*time.Second + time.Minute,
		SourceWriterLockHeartbeatInterval: time.Hour,
		SourceWriterLockPID:               123,
		IDGenerator:                       fixture.nextID,
		Clock:                             func() time.Time { return now },
		TaskLoader:                        fixture.loadTask,
		DossierAssembler:                  fixture.assemble,
		SupervisorRunner:                  fixture.runSupervisor,
		PolicyEvaluator:                   fixture.evaluatePolicy,
		ProfileLoader:                     fixture.loadProfile,
		CodexRunner:                       fixture.runCodex,
		SourceSnapshotter:                 fixture.captureSnapshot,
		DirtyCapture:                      fixture.captureDirty,
		ChangedCapture:                    fixture.captureChanged,
		VerificationRunner:                fixture.runVerification,
		CommitRunner:                      fixture.runCommit,
		LockAcquirer:                      fixture.acquireLock,
		CommandRunner:                     func(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 0} },
		SafetyPreflightRunner: func(_ context.Context, input autonomoussafety.Input) (autonomoussafety.Output, error) {
			return testSafetyOutput(t, input)
		},
	}
	fixture.applyActionEvidence()
	return fixture
}

func testSafetyOutput(t *testing.T, input autonomoussafety.Input) (autonomoussafety.Output, error) {
	t.Helper()
	policy, err := autonomoussafety.FinalizePolicy(autonomoussafety.Policy{
		SchemaVersion: autonomoussafety.PolicySchemaVersion, TaskID: input.TaskID, Workspace: input.Workspace,
		Mode: autonomoussafety.ModeOperatorAttended, Codex: input.Codex,
		ExternalIsolation: autonomoussafety.ExternalIsolation{Expectation: autonomoussafety.IsolationNone, Enforcement: autonomoussafety.EnforcementNone},
		Network:           autonomoussafety.NetworkPolicy{Access: autonomoussafety.NetworkUnknown, Enforcement: autonomoussafety.EnforcementNone}, Hooks: autonomoussafety.HookTrust{Policy: autonomoussafety.HooksOperatorAttended},
		Environment: autonomoussafety.EnvironmentPolicy{InheritHost: true}, RedactionPolicyHash: strings.Repeat("b", 64),
		ProtectedPaths: []autonomoussafety.ProtectedPath{
			{Path: filepath.Join(input.Workspace.ExecutionRoot, ".git"), Class: "worktree_git_administration"},
			{Path: filepath.Join(input.Workspace.ExecutionRoot, ".agent", "tasks"), Class: "task_specifications"},
			{Path: filepath.Join(input.Workspace.ExecutionRoot, ".agent", "profiles"), Class: "role_profiles"},
			{Path: filepath.Join(input.Workspace.ExecutionRoot, "AGENTS.md"), Class: "repository_guidance"},
		},
		ConfigPath: input.ConfigPath, ConfigSHA256: input.ConfigSHA256,
		WorktreeNotice: "Git worktree isolation is source/Git isolation, not a security sandbox.",
	})
	if err != nil {
		return autonomoussafety.Output{}, err
	}
	preflight := autonomoussafety.PreflightResult{SchemaVersion: autonomoussafety.PreflightSchemaVersion, TaskID: input.TaskID, WorkspaceID: input.Workspace.WorkspaceID, SourceRevision: input.SourceRevision, PolicySHA256: policy.PolicySHA256, ConfigSHA256: input.ConfigSHA256, ObservedAt: input.ObservedAt, Ready: true, Checks: []autonomoussafety.Check{{Name: "fixture", Status: autonomoussafety.CheckOK, Detail: "validated"}}}
	return autonomoussafety.Output{Policy: policy, Preflight: preflight}, nil
}

func (f *cycleFixture) configureAction(action autonomous.Action) {
	profile := profileForAction(action)
	f.decision = autonomous.SupervisorDecision{
		TaskID: "task-1", Action: action, WorkerProfile: profile,
		Rationale: "Current durable evidence supports exactly this route.",
		Inputs:    []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: ".agent/tasks/task-1.md", Detail: "Exact canonical task evidence."}},
	}
	if profile != "" {
		f.decision.SuccessCriteria = []string{"Return exact evidence for this one action."}
	}
	f.state = baseState("task-1", autonomous.LifecycleStateReady)
	switch action {
	case autonomous.ActionPlan:
		f.state.Lifecycle = autonomous.LifecycleStatePending
	case autonomous.ActionImplement:
		f.state.Plan = pendingPlan("task-1")
	case autonomous.ActionAudit:
	case autonomous.ActionCorrect:
		f.state.Plan = pendingPlan("task-1")
		f.state.FindingResolutions = []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}}
		f.decision.FindingIDs = []string{"finding-one"}
	case autonomous.ActionDocument, autonomous.ActionSimplify:
		f.state.Plan = completedPlan("task-1")
	case autonomous.ActionComplete:
		f.state.Plan = completedPlan("task-1")
		f.state.AcceptanceCriteria = []autonomous.AcceptanceCriterion{{
			ID: "criterion-one", Requirement: "Behavior works", Status: autonomous.AcceptanceStatusSatisfied,
			Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "verify-1", Detail: "Current verification passed."}},
		}}
	case autonomous.ActionBlock:
		f.state.Lifecycle = autonomous.LifecycleStatePending
	case autonomous.ActionNeedsInput:
		question := autonomous.NeedsInputQuestion{TaskID: "task-1", QuestionID: "product-mode", Revision: 1, Question: "Which behavior?", BlockingReason: "The task permits incompatible behaviors.", Options: []autonomous.NeedsInputOption{{ID: "keep", Meaning: "Keep behavior."}, {ID: "change", Meaning: "Change behavior."}}, Recommendation: autonomous.NeedsInputRecommendation{OptionID: "keep", Rationale: "Safer."}, Evidence: append([]autonomous.EvidenceReference(nil), f.decision.Inputs...)}
		hash, _ := autonomous.QuestionContentSHA256(question)
		question.ContentSHA256 = hash
		f.decision.NeedsInput = &question
	}
}

func (f *cycleFixture) applyActionEvidence() {
	revision, err := gitstate.PolicySourceRevision(f.baseline)
	if err != nil {
		f.t.Fatal(err)
	}
	verificationEvidence := currentVerificationEvidence("task-1", revision)
	cleanAudit := currentAuditEvidence("task-1", revision, autonomous.AuditDispositionClean)
	switch f.decision.Action {
	case autonomous.ActionAudit:
		f.cfg.Verification = &verificationEvidence
	case autonomous.ActionCorrect:
		changesAudit := currentAuditEvidence("task-1", revision, autonomous.AuditDispositionChangesRequired)
		f.cfg.Verification = &verificationEvidence
		f.cfg.Audit = &changesAudit
	case autonomous.ActionDocument, autonomous.ActionSimplify, autonomous.ActionComplete:
		f.cfg.Verification = &verificationEvidence
		f.cfg.Audit = &cleanAudit
	}
}

func (f *cycleFixture) withChangedWorker() {
	f.snapshots = []gitstate.SourceSnapshot{f.baseline, f.baseline, f.baseline, f.changed, f.changed, f.committed}
	f.receiptChangedFiles = []string{"tracked.txt"}
	f.receiptVerification = []receipt.VerificationEntry{{Command: "fake-verify", ExitCode: 0, Status: "passed"}}
}

func (f *cycleFixture) useRealSourceRepository() {
	f.t.Helper()
	executionRoot := f.cfg.Workspace.ExecutionRoot
	runCycleGit(f.t, executionRoot, "init", "-q")
	runCycleGit(f.t, executionRoot, "config", "user.email", "cycle@example.test")
	runCycleGit(f.t, executionRoot, "config", "user.name", "Cycle Test")
	writeFixtureFile(f.t, filepath.Join(executionRoot, ".gitignore"), []byte("*.env\ncache/\n.revolvr/\n"))
	writeFixtureFile(f.t, filepath.Join(executionRoot, "tracked.txt"), []byte("baseline\n"))
	runCycleGit(f.t, executionRoot, "add", ".gitignore", "tracked.txt")
	runCycleGit(f.t, executionRoot, "commit", "-qm", "baseline")

	snapshot, err := gitstate.CaptureSourceSnapshot(context.Background(), gitstate.SourceSnapshotConfig{WorkingDir: executionRoot})
	if err != nil {
		f.t.Fatal(err)
	}
	revision, err := gitstate.PolicySourceRevision(snapshot)
	if err != nil {
		f.t.Fatal(err)
	}
	workspace := *f.cfg.Workspace
	workspace.GitCommonDir = filepath.Join(executionRoot, ".git")
	workspace.BaselineSHA = snapshot.Head
	workspace.HeadSHA = snapshot.Head
	workspace.TreeSHA = runCycleGit(f.t, executionRoot, "rev-parse", "HEAD^{tree}")
	workspace.SourceRevision = revision
	workspace.Checkpoint.CommitSHA = workspace.HeadSHA
	workspace.Checkpoint.TreeSHA = workspace.TreeSHA
	workspace.Checkpoint.SourceRevision = revision
	f.state.Workspace = &workspace
	f.cfg.State = f.state
	f.cfg.Workspace = &workspace
	f.baseline = snapshot
	f.supervisorSource = snapshot
	f.cfg.GitExecutable = "git"
	f.cfg.SourceSnapshotter = gitstate.CaptureSourceSnapshot
	f.cfg.DirtyCapture = gitstate.CaptureDirtyWorktree
	f.cfg.ChangedCapture = gitstate.CaptureChangedFiles
	f.cfg.CommandRunner = runner.Run
	f.applyActionEvidence()
}

func runCycleGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *cycleFixture) nextID() string {
	if f.idIndex >= len(f.ids) {
		f.t.Fatalf("unexpected ID request %d", f.idIndex)
	}
	value := f.ids[f.idIndex]
	f.idIndex++
	return value
}

func (f *cycleFixture) loadTask(string, string) (taskfile.Task, bool, error) {
	task := f.task
	task.SourceBytes = append([]byte(nil), f.task.SourceBytes...)
	return task, true, nil
}

func (f *cycleFixture) assemble(_ context.Context, in autonomousassembly.Input) (autonomous.TaskDossier, error) {
	if f.assembleErr != nil {
		return autonomous.TaskDossier{}, f.assembleErr
	}
	return autonomous.BuildTaskDossier(autonomous.TaskDossierInput{
		TaskID:         in.TaskID,
		TaskSpec:       autonomous.TaskSpecSource{ID: "task-spec:" + in.TaskID, Path: f.task.SourcePath, Label: f.task.Title, Content: append([]byte(nil), f.task.SourceBytes...)},
		State:          in.State,
		Audit:          in.Audit,
		RecentRunLimit: 0,
		Git:            &autonomous.GitSnapshot{Head: f.baseline.Head, WorktreeStatus: "stable fake snapshot", DiffSummary: "none"},
	})
}

func (f *cycleFixture) runSupervisor(ctx context.Context, cfg supervisor.Config) (supervisor.Result, error) {
	f.supervisorCalls++
	if f.supervisorErr != nil {
		return supervisor.Result{RunID: cfg.RunID}, f.supervisorErr
	}
	if f.supervisorMutatesState {
		writeFixtureFile(f.t, filepath.Join(f.root, f.task.AutonomousStatePath), []byte("mutated state"))
	}
	if _, err := cfg.Ledger.CreateRun(ctx, ledger.RunSpec{ID: cfg.RunID, TaskID: cfg.TaskID, Task: "supervisor", StartedAt: f.now}); err != nil {
		return supervisor.Result{}, err
	}
	_, _ = cfg.Ledger.AppendEvent(ctx, cfg.RunID, ledger.EventSupervisorPrepared, map[string]any{"artifacts": map[string]any{}})
	_, _, _ = cfg.Ledger.CompleteRun(ctx, cfg.RunID, ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: "decision accepted", CompletedAt: f.now, VerificationStatus: "not_run"})
	f.decisionID = cfg.DecisionID
	decision := f.decision
	reference := autonomous.DecisionReference{
		DecisionID: cfg.DecisionID, RunID: cfg.RunID, TaskID: cfg.TaskID,
		Action: decision.Action, WorkerProfile: decision.WorkerProfile,
		Artifact:  autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: filepath.Join(".revolvr", "runs", cfg.RunID, "supervisor-decision.json"), Detail: "Exact validated supervisor decision."},
		CreatedAt: f.now,
	}
	source := f.supervisorSource
	after := source
	if f.supervisorAfter.SchemaVersion != "" {
		after = f.supervisorAfter
	}
	var decisionPointer *autonomous.SupervisorDecision
	var referencePointer *autonomous.DecisionReference
	if !f.supervisorMissingDecision {
		decisionPointer = &decision
		referencePointer = &reference
	}
	routingAuthority, err := autonomouspolicy.RoutingAuthorityForLifecycle(cfg.Lifecycle)
	if err != nil {
		return supervisor.Result{}, err
	}
	return supervisor.Result{
		RunID:             cfg.RunID,
		Decision:          decisionPointer,
		DecisionReference: referencePointer,
		Artifacts:         supervisor.Artifacts{Prompt: supervisor.Artifact{Path: filepath.Join(".revolvr", "runs", cfg.RunID, "supervisor-prompt.md"), SHA256: strings.Repeat("1", 64), ByteSize: 1}},
		Dossier:           supervisor.DossierProvenance{SchemaVersion: cfg.Dossier.Manifest.SchemaVersion, TaskID: cfg.TaskID, SHA256: cfg.Dossier.Manifest.DossierSHA256, ByteSize: cfg.Dossier.Manifest.DossierByteSize},
		Profile:           supervisor.ProfileProvenance{Name: supervisor.SupervisorProfileName, Path: ".agent/profiles/supervisor.md", SHA256: strings.Repeat("2", 64), ByteSize: 10},
		RoutingAuthority:  routingAuthority,
		Invocation: testInvocationWithSafety(func() string {
			if cfg.ExecutionRoot != "" {
				return cfg.ExecutionRoot
			}
			return f.root
		}(), cfg.SafetyPolicySHA256),
		SourceBefore:     &source,
		SourceAfter:      &after,
		SourceDifference: gitstate.CompareSourceSnapshots(source, after),
	}, nil
}

func (f *cycleFixture) evaluatePolicy(in autonomouspolicy.Input) (autonomouspolicy.Route, error) {
	f.policyCalls++
	if !f.lock.acquired || f.lock.released {
		f.t.Fatal("policy evaluation did not run while worker source lock was held")
	}
	if f.policyErr != nil {
		return autonomouspolicy.Route{}, f.policyErr
	}
	return autonomouspolicy.Evaluate(in)
}

func (f *cycleFixture) loadProfile(_ string, name string) (prompt.RunProfile, error) {
	return prompt.RunProfile{Name: name, SourcePath: filepath.Join(".agent", "profiles", name+".md"), Description: "Exact repo-authored " + name + " profile."}, nil
}

func (f *cycleFixture) runCodex(_ context.Context, cfg codexexec.Config) (codexexec.Result, error) {
	f.codexCalls++
	if !f.lock.acquired || f.lock.released {
		f.t.Fatal("worker Codex did not run while source lock was held")
	}
	f.workerPrompt = cfg.Prompt
	f.workerOutputSchema = cfg.OutputSchema
	artifactRoot := cfg.ArtifactRoot
	if artifactRoot == "" {
		artifactRoot = cfg.WorkingDir
	}
	writeFixtureFile(f.t, filepath.Join(artifactRoot, cfg.Artifacts.StdoutJSONL), []byte("{\"type\":\"turn.completed\"}\n"))
	writeFixtureFile(f.t, filepath.Join(artifactRoot, cfg.Artifacts.Stderr), nil)
	writeFixtureFile(f.t, filepath.Join(artifactRoot, cfg.Artifacts.LastMessage), []byte("exact worker final output"))
	if f.workerMutatesState {
		writeFixtureFile(f.t, filepath.Join(f.root, f.task.AutonomousStatePath), []byte("worker-mutated state"))
	}
	if f.writeReceipt {
		receiptPath := filepath.Join(artifactRoot, ".revolvr", "receipts", cfg.RunID+".md")
		if f.malformedReceipt {
			writeFixtureFile(f.t, receiptPath, []byte("not a receipt"))
		} else {
			content, _ := receipt.FormatFallbackReceipt(receipt.FallbackInput{
				RunID: cfg.RunID, PassID: cfg.RunID, TaskID: "task-1", Task: "Exact task",
				Verdict: receipt.VerdictCompletedWithConcerns, Timestamp: f.now,
				CodexExitCode: 0, VerificationStatus: "not_run",
				ChangedFiles: append([]string(nil), f.receiptChangedFiles...),
				Verification: append([]receipt.VerificationEntry(nil), f.receiptVerification...),
				FinalText:    "worker-authored evidence",
			})
			writeFixtureFile(f.t, receiptPath, []byte(content))
		}
	}
	result := codexexec.Result{
		ExitCode:     0,
		FinalMessage: "exact worker final output",
		Artifacts:    cfg.Artifacts,
		Usage:        receipt.Metrics{InputTokens: 10, OutputTokens: 5, DurationSeconds: 1},
		UsageFound:   true,
		LedgerError:  f.codexLedgerErr,
	}
	if f.codexErr != nil {
		result.ExitCode = 1
		result.Err = f.codexErr
	}
	return result, f.codexErr
}

func (f *cycleFixture) captureSnapshot(context.Context, gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error) {
	if f.snapshotIndex >= len(f.snapshots) {
		f.t.Fatalf("unexpected source snapshot %d (have %d)", f.snapshotIndex, len(f.snapshots))
	}
	value := f.snapshots[f.snapshotIndex]
	f.snapshotIndex++
	return value, nil
}

func (f *cycleFixture) captureDirty(context.Context, gitstate.Config) (gitstate.Capture, error) {
	return gitstate.Capture{Kind: gitstate.CaptureKindDirty, Paths: append([]string(nil), f.dirtyPaths...), DirtyFiles: append([]string(nil), f.dirtyPaths...)}, nil
}

func (f *cycleFixture) captureChanged(context.Context, gitstate.Config) (gitstate.Capture, error) {
	if f.changedCaptureErr != nil {
		return gitstate.Capture{}, f.changedCaptureErr
	}
	if reflect.DeepEqual(f.snapshots, []gitstate.SourceSnapshot{f.baseline, f.baseline, f.baseline, f.baseline}) {
		return gitstate.Capture{Kind: gitstate.CaptureKindChanged}, nil
	}
	return gitstate.Capture{Kind: gitstate.CaptureKindChanged, Paths: []string{"tracked.txt"}, ChangedFiles: []string{"tracked.txt"}}, nil
}

func (f *cycleFixture) runVerification(context.Context, verification.Config) (verification.Result, error) {
	f.verificationCalls++
	if !f.lock.acquired || f.lock.released {
		f.t.Fatal("verification did not run while source lock was held")
	}
	return f.verificationResult, f.verificationErr
}

func (f *cycleFixture) runCommit(context.Context, commit.Config) (commit.Result, error) {
	f.commitCalls++
	if !f.lock.acquired || f.lock.released {
		f.t.Fatal("commit did not run while source lock was held")
	}
	return f.commitResult, f.commitErr
}

func (f *cycleFixture) acquireLock(context.Context, lock.Config) (SourceLock, error) {
	f.lock.acquired = true
	return f.lock, nil
}

type fakeSourceLock struct {
	mu             sync.Mutex
	acquired       bool
	released       bool
	releases       int
	heartbeats     int
	heartbeatErrAt int
	heartbeatErr   error
	heartbeatFunc  func(context.Context) error
	releaseErr     error
}

func (l *fakeSourceLock) Heartbeat(ctx context.Context) error {
	l.mu.Lock()
	l.heartbeats++
	if l.heartbeatErrAt > 0 && l.heartbeats >= l.heartbeatErrAt {
		err := l.heartbeatErr
		l.mu.Unlock()
		return err
	}
	fn := l.heartbeatFunc
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil
}
func (l *fakeSourceLock) Release(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released = true
	l.releases++
	return l.releaseErr
}

func (l *fakeSourceLock) releaseCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.releases
}

type memoryLedger struct {
	now       time.Time
	runs      map[string]ledger.Run
	events    map[string][]ledger.Event
	nextEvent int64
}

func newMemoryLedger(now time.Time) *memoryLedger {
	return &memoryLedger{now: now, runs: make(map[string]ledger.Run), events: make(map[string][]ledger.Event)}
}

func (m *memoryLedger) CreateRun(_ context.Context, spec ledger.RunSpec) (ledger.Run, error) {
	if _, exists := m.runs[spec.ID]; exists {
		return ledger.Run{}, errors.New("duplicate run")
	}
	started := spec.StartedAt
	if started.IsZero() {
		started = m.now
	}
	run := ledger.Run{ID: spec.ID, TaskID: spec.TaskID, Task: spec.Task, Status: ledger.StatusRunning, StartedAt: started}
	m.runs[run.ID] = run
	return run, nil
}

func (m *memoryLedger) AppendEvent(_ context.Context, runID string, eventType ledger.EventType, payload any) (ledger.Event, error) {
	if _, ok := m.runs[runID]; !ok {
		return ledger.Event{}, errors.New("run not found")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ledger.Event{}, err
	}
	m.nextEvent++
	event := ledger.Event{ID: m.nextEvent, RunID: runID, Type: eventType, Payload: raw, CreatedAt: m.now}
	m.events[runID] = append(m.events[runID], event)
	return event, nil
}

func (m *memoryLedger) CompleteRun(_ context.Context, runID string, completion ledger.RunCompletion) (ledger.Run, bool, error) {
	run, ok := m.runs[runID]
	if !ok {
		return ledger.Run{}, false, nil
	}
	run.Status = completion.Status
	run.Summary = completion.Summary
	completed := completion.CompletedAt
	run.CompletedAt = &completed
	run.CodexExitCode = completion.CodexExitCode
	run.VerificationStatus = completion.VerificationStatus
	run.CommitSHA = completion.CommitSHA
	m.runs[runID] = run
	return run, true, nil
}

func (m *memoryLedger) RecordCommitSHA(_ context.Context, runID, sha string) error {
	run, ok := m.runs[runID]
	if !ok {
		return errors.New("run not found")
	}
	run.CommitSHA = sha
	m.runs[runID] = run
	return nil
}

func (m *memoryLedger) runCount() int { return len(m.runs) }

func baseState(taskID string, lifecycle autonomous.LifecycleState) autonomous.ExecutionState {
	return autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        taskID,
		Lifecycle:     lifecycle,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
}

func pendingPlan(taskID string) *autonomous.TaskPlan {
	return &autonomous.TaskPlan{
		TaskID: taskID, ID: "plan-one", Revision: 1,
		Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task", Detail: "Task requires work."}},
		Steps:      []autonomous.PlanStep{{ID: "step-one", Description: "Implement behavior", Status: autonomous.PlanStepStatusPending}},
	}
}

func completedPlan(taskID string) *autonomous.TaskPlan {
	return &autonomous.TaskPlan{
		TaskID: taskID, ID: "plan-one", Revision: 1, Completed: true,
		Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: "task", Detail: "Task requires work."}},
		Steps: []autonomous.PlanStep{{
			ID: "step-one", Description: "Implement behavior", Status: autonomous.PlanStepStatusCompleted,
			Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "verify-1", Detail: "Step verified."}},
		}},
	}
}

func currentVerificationEvidence(taskID, revision string) autonomouspolicy.VerificationEvidence {
	return autonomouspolicy.VerificationEvidence{
		Summary: autonomous.VerificationSummary{
			TaskID: taskID, Status: autonomous.VerificationStatusPassed, Command: "fake-verify", Summary: "passed", RunID: "verify-run", OccurrenceID: "verify-occurrence-current",
			Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindVerification, Reference: "verify-current", Detail: "Current verification passed."}},
		},
		SourceRevision: revision,
	}
}

func currentAuditEvidence(taskID, revision string, disposition autonomous.AuditDisposition) autonomouspolicy.AuditEvidence {
	report := autonomous.AuditReport{
		TaskID: taskID, Disposition: disposition, Rationale: "Current independent audit evidence.",
		Inputs: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindAudit, Reference: "audit-current", Detail: "Independent audit."}},
	}
	if disposition == autonomous.AuditDispositionChangesRequired {
		report.Findings = []autonomous.AuditFinding{{
			ID: "finding-one", Significance: autonomous.FindingSignificanceBlocking, Summary: "Defect remains",
			Evidence:           []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: "tracked.txt", Detail: "Exact defect evidence."}},
			RequiredCorrection: "Correct the cited defect.",
		}}
	}
	return autonomouspolicy.AuditEvidence{
		Report: report, RunID: "audit-run", AuditorProfile: autonomous.WorkerProfileAuditor,
		SourceRevision: revision, VerificationRunID: "verify-run", VerificationOccurrenceID: "verify-occurrence-current",
	}
}

func profileForAction(action autonomous.Action) autonomous.WorkerProfile {
	switch action {
	case autonomous.ActionPlan:
		return autonomous.WorkerProfilePlanner
	case autonomous.ActionImplement:
		return autonomous.WorkerProfileImplementer
	case autonomous.ActionAudit:
		return autonomous.WorkerProfileAuditor
	case autonomous.ActionCorrect:
		return autonomous.WorkerProfileCorrector
	case autonomous.ActionDocument:
		return autonomous.WorkerProfileDocumentor
	case autonomous.ActionSimplify:
		return autonomous.WorkerProfileSimplifier
	default:
		return ""
	}
}

func testInvocation(root string) codexexec.InvocationProvenance {
	return codexexec.InvocationProvenance{
		Executable: "fake-codex", Version: "codex-cli test", Model: codexexec.DefaultModel, ReasoningEffort: codexexec.DefaultReasoningEffort,
		Ephemeral: true, SessionMode: codexexec.SessionModeEphemeral,
		EffectiveConfigSchema: "revolvr-effective-run-config-v1", EffectiveConfigSHA256: strings.Repeat("a", 64),
		Argv: []string{"exec", "--json", "--ephemeral", "-"}, WorkingDir: root,
	}
}

func testInvocationWithSafety(root, safetyPolicySHA256 string) codexexec.InvocationProvenance {
	invocation := testInvocation(root)
	invocation.SafetyPolicySHA256 = safetyPolicySHA256
	return invocation
}

func testSourceSnapshot(head, content, indexRecord string) gitstate.SourceSnapshot {
	return testSourceSnapshotAtPath(head, content, indexRecord, "tracked.txt")
}

func testSourceSnapshotAtPath(head, content, indexRecord, path string) gitstate.SourceSnapshot {
	contentSum := sha256.Sum256([]byte(content))
	entries := []gitstate.SourceEntry{{Path: path, IndexRecord: indexRecord, FileType: "regular", Mode: 0o644, ByteSize: int64(len(content)), SHA256: fmt.Sprintf("%x", contentSum)}}
	indexHash := sha256.New()
	_, _ = io.WriteString(indexHash, path)
	_, _ = indexHash.Write([]byte{0})
	_, _ = io.WriteString(indexHash, indexRecord)
	_, _ = indexHash.Write([]byte{0})
	worktreeRaw, _ := json.Marshal(entries)
	worktreeSum := sha256.Sum256(worktreeRaw)
	snapshot := gitstate.SourceSnapshot{SchemaVersion: gitstate.SourceSnapshotSchemaVersion, Head: head, IndexSHA256: fmt.Sprintf("%x", indexHash.Sum(nil)), WorktreeSHA256: fmt.Sprintf("%x", worktreeSum), Entries: entries}
	snapshotRaw, _ := json.Marshal(struct {
		SchemaVersion  string                 `json:"schema_version"`
		Head           string                 `json:"head"`
		IndexSHA256    string                 `json:"index_sha256"`
		WorktreeSHA256 string                 `json:"worktree_sha256"`
		Entries        []gitstate.SourceEntry `json:"entries"`
	}{snapshot.SchemaVersion, snapshot.Head, snapshot.IndexSHA256, snapshot.WorktreeSHA256, snapshot.Entries})
	snapshotSum := sha256.Sum256(snapshotRaw)
	snapshot.SnapshotSHA256 = fmt.Sprintf("%x", snapshotSum)
	return snapshot
}

func passedVerification() verification.Result {
	return verification.Result{Status: verification.StatusPassed, Passed: true, FailedCommandIndex: -1, Commands: []verification.CommandResult{{Index: 0, Command: "fake-verify", Name: "fake-verify", Status: verification.StatusPassed, Passed: true, ExitCode: 0}}}
}

func failedVerification() verification.Result {
	return verification.Result{Status: verification.StatusFailed, Passed: false, FailedCommandIndex: 0, Message: "verification failed", Commands: []verification.CommandResult{{Index: 0, Command: "fake-verify", Name: "fake-verify", Status: verification.StatusFailed, ExitCode: 1}}}
}

func timedOutVerification() verification.Result {
	return verification.Result{Status: verification.StatusFailed, Passed: false, FailedCommandIndex: 0, Message: "verification timed out", Commands: []verification.CommandResult{{Index: 0, Command: "fake-verify", Name: "fake-verify", Status: verification.StatusFailed, ExitCode: -1, TimedOut: true}}}
}

func committedResult() commit.Result {
	return commit.Result{Status: commit.StatusCommitted, CommitSHA: "commit-sha", PreCommitSHA: "head-1", PostCommitSHA: "head-2", Message: "commit created", ChangedFiles: []string{"tracked.txt"}}
}
func pointerSnapshot(value gitstate.SourceSnapshot) *gitstate.SourceSnapshot { return &value }

func writeFixtureFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func containsArg(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func countArg(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func warningKinds(warnings []ReceiptWarning) []string {
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, warning.Kind)
	}
	return result
}
