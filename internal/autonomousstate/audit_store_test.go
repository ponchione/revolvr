package autonomousstate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomouspolicy"
)

func TestCommitAuditReopenReplayCASAndOrphanRecovery(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	request := auditStoreRequest(t, repo, taskRaw)
	statePath := filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1")))
	writeAuditStoreFile(t, statePath, mustMarshalState(t, request.PreviousState))
	store := openStateTestStore(t, repo, nil)
	result, err := store.CommitAudit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitUpdated || result.History.Record.AuditRevision != 1 {
		t.Fatalf("result=%+v", result)
	}
	reopened := openStateTestStore(t, repo, nil)
	audit, found, err := reopened.LoadCurrentAudit(context.Background(), "task-1")
	if err != nil || !found || audit.Revision != 1 || !reflect.DeepEqual(audit.Report, request.History.Report) || !reflect.DeepEqual(audit.PolicyEvidence, request.History.PolicyEvidence) {
		t.Fatalf("audit=%+v found=%t err=%v", audit, found, err)
	}
	replay, err := reopened.CommitAudit(context.Background(), request)
	if err != nil || replay.Disposition != CommitReplayed {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	stale := request
	stale.History.OperationID = "audit-operation-stale"
	stale.History.Sequence = 2
	stale.History.AuditRevision = 2
	stale.History.ApplicationSHA256 = strings.Repeat("9", 64)
	stale.Expected = ExpectedState{Exists: true, SHA256: strings.Repeat("8", 64), ByteSize: 10}
	if _, err := reopened.CommitAudit(context.Background(), stale); !errors.Is(err, ErrStaleWrite) {
		t.Fatalf("stale error=%v", err)
	}

	t.Run("orphan is not current and identical retry reconciles", func(t *testing.T) {
		repo, taskRaw := stateTestRepository(t, "task-1")
		request := auditStoreRequest(t, repo, taskRaw)
		writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1"))), mustMarshalState(t, request.PreviousState))
		fired := false
		failing := openStateTestStore(t, repo, func(point FailurePoint) error {
			if !fired && point == FailureAfterAuditHistory {
				fired = true
				return errors.New("crash")
			}
			return nil
		})
		if _, err := failing.CommitAudit(context.Background(), request); err == nil || !fired {
			t.Fatalf("error=%v fired=%t", err, fired)
		}
		reopened := openStateTestStore(t, repo, nil)
		if _, found, err := reopened.LoadCurrentAudit(context.Background(), "task-1"); err != nil || found {
			t.Fatalf("orphan became current found=%t err=%v", found, err)
		}
		result, err := reopened.CommitAudit(context.Background(), request)
		if err != nil || result.Disposition != CommitUpdated {
			t.Fatalf("retry=%+v err=%v", result, err)
		}
	})
}

func TestCommitAuditFailureBeforeRenamePreservesPreviousState(t *testing.T) {
	for _, point := range []FailurePoint{FailureBeforeAuditOutput, FailureDuringAuditOutput, FailureBeforeAuditHistory, FailureDuringAuditHistory, FailureAfterAuditHistory, FailureDuringStateWrite, FailureBeforeStateRename, FailureStateRename} {
		t.Run(string(point), func(t *testing.T) {
			repo, taskRaw := stateTestRepository(t, "task-1")
			request := auditStoreRequest(t, repo, taskRaw)
			previousRaw := mustMarshalState(t, request.PreviousState)
			statePath := filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1")))
			writeAuditStoreFile(t, statePath, previousRaw)
			fired := false
			store := openStateTestStore(t, repo, func(got FailurePoint) error {
				if !fired && got == point {
					fired = true
					return errors.New("crash")
				}
				return nil
			})
			if _, err := store.CommitAudit(context.Background(), request); err == nil || !fired {
				t.Fatalf("error=%v", err)
			}
			got, _ := os.ReadFile(statePath)
			if !reflect.DeepEqual(got, previousRaw) {
				t.Fatalf("previous state changed at %s", point)
			}
		})
	}
}

func TestLoadCurrentAuditTraversesAttemptAndOptionalRoleEvidence(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	auditRequest := auditStoreRequest(t, repo, taskRaw)
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1"))), mustMarshalState(t, auditRequest.PreviousState))
	store := openStateTestStore(t, repo, nil)
	auditResult, err := store.CommitAudit(context.Background(), auditRequest)
	if err != nil {
		t.Fatal(err)
	}

	decision := autonomous.DecisionReference{DecisionID: "decision-document", RunID: "supervisor-document", TaskID: "task-1", Action: autonomous.ActionDocument, WorkerProfile: autonomous.WorkerProfileDocumentor, Artifact: stateTestEvidence(autonomous.EvidenceKindLedger, "decision-document"), CreatedAt: time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)}
	admitted := autonomous.AttemptEvent{Sequence: 1, Kind: autonomous.AttemptEventAdmitted, AttemptID: "attempt-document", Action: autonomous.ActionDocument, Decision: decision, StrategySHA256: strings.Repeat("a", 64), SourceBefore: strings.Repeat("4", 64), Evidence: []autonomous.EvidenceReference{decision.Artifact}, CreatedAt: decision.CreatedAt}
	completed := admitted
	completed.Sequence, completed.Kind, completed.RunID, completed.OccurrenceID = 2, autonomous.AttemptEventCompleted, "worker-document", "verification-document"
	completed.SourceAfter, completed.SourceAfterKnown, completed.Outcome, completed.CreatedAt = strings.Repeat("5", 64), true, autonomous.AttemptOutcomeSucceeded, decision.CreatedAt.Add(time.Minute)
	attemptState := auditResult.Current.State
	attemptState.Attempts = autonomous.AttemptState{TotalAttempts: 1, ActionAttempts: []autonomous.ActionAttempt{{Action: autonomous.ActionDocument, Attempts: 1}}, ActionBudgets: []autonomous.ActionBudget{{Action: autonomous.ActionDocument, Budget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited, Consumed: 1}}}, RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited, Consumed: 1}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}, RepeatedSignatureLimit: 3, TransitionSequence: 2, Events: []autonomous.AttemptEvent{admitted, completed}}
	previousIdentity, _ := StateIdentityFor(auditResult.Current.SourcePath, true, auditResult.Current.State)
	attemptIdentity, _ := StateIdentityFor(auditResult.Current.SourcePath, true, attemptState)
	attemptResult, err := store.CommitAttempt(context.Background(), AttemptCommitRequest{TaskID: "task-1", Expected: auditResult.Current.Expected(), PreviousState: auditResult.Current.State, NextState: attemptState, History: AttemptHistoryRecord{SchemaVersion: AttemptHistorySchemaVersion, TaskID: "task-1", OperationID: "complete-document", ApplicationSHA256: strings.Repeat("b", 64), Sequence: 2, Kind: AttemptTransitionCompleted, CreatedAt: completed.CreatedAt, Event: &completed, PreviousState: previousIdentity, ResultingState: attemptIdentity}})
	if err != nil {
		t.Fatal(err)
	}

	occurrence := autonomous.OptionalRoleOccurrence{SchemaVersion: autonomous.OptionalRoleOccurrenceSchemaVersion, Sequence: 1, TaskID: "task-1", Role: autonomous.WorkerProfileDocumentor, Outcome: autonomous.OptionalRoleOutcomeSourceChanged, Decision: decision, AssessmentSHA256: strings.Repeat("c", 64), SourceBefore: strings.Repeat("4", 64), SourceAfter: strings.Repeat("5", 64), Gate: autonomous.OptionalRoleGate{SourceRevision: strings.Repeat("5", 64), VerificationRunID: "worker-document", VerificationOccurrenceID: "verification-document", AuditSupervisorRunID: "audit-supervisor-new", AuditWorkerRunID: "audit-worker-new", AuditRevision: 2}, Worker: &autonomous.OptionalRoleWorkerEvidence{AttemptID: "attempt-document", RunID: "worker-document", DossierSHA256: strings.Repeat("d", 64), DossierByteSize: 10, ProfilePath: ".agent/profiles/documentor.md", ProfileSHA256: strings.Repeat("e", 64), ProfileByteSize: 10, Receipt: stateTestEvidence(autonomous.EvidenceKindReceipt, "receipt-document"), Ledger: stateTestEvidence(autonomous.EvidenceKindLedger, "ledger-document")}, ChangedPaths: []string{"README.md"}, CommitSHA: "commit-document", Evidence: []autonomous.EvidenceReference{stateTestEvidence(autonomous.EvidenceKindLedger, "optional-role")}, Rationale: "Documented exact user-facing behavior.", CreatedAt: completed.CreatedAt.Add(time.Minute)}
	optionalState := attemptResult.Current.State
	optionalState.OptionalRoles = append(optionalState.OptionalRoles, occurrence)
	attemptStateIdentity, _ := StateIdentityFor(attemptResult.Current.SourcePath, true, attemptResult.Current.State)
	optionalIdentity, _ := StateIdentityFor(attemptResult.Current.SourcePath, true, optionalState)
	_, err = store.CommitOptionalRole(context.Background(), OptionalRoleCommitRequest{TaskID: "task-1", Expected: attemptResult.Current.Expected(), PreviousState: attemptResult.Current.State, NextState: optionalState, History: OptionalRoleHistoryRecord{SchemaVersion: OptionalRoleHistorySchemaVersion, TaskID: "task-1", OperationID: "record-document", ApplicationSHA256: strings.Repeat("f", 64), Sequence: 1, CreatedAt: occurrence.CreatedAt, Occurrence: occurrence, PreviousState: attemptStateIdentity, ResultingState: optionalIdentity}})
	if err != nil {
		t.Fatal(err)
	}

	reopened, found, err := store.LoadCurrentAudit(context.Background(), "task-1")
	if err != nil || !found || reopened.Revision != auditResult.History.Record.AuditRevision || reopened.PolicyEvidence.RunID != auditResult.History.Record.PolicyEvidence.RunID || len(reopened.State.State.OptionalRoles) != 1 {
		t.Fatalf("reopened=%+v found=%t err=%v", reopened, found, err)
	}
}

func TestCommitAuditSharedStateLockAllowsExactlyOneWriter(t *testing.T) {
	repo, taskRaw := stateTestRepository(t, "task-1")
	first := auditStoreRequest(t, repo, taskRaw)
	second := first
	second.History.OperationID = "audit-operation-two"
	second.History.ApplicationSHA256 = strings.Repeat("7", 64)
	statePath := filepath.Join(repo, filepath.FromSlash(canonicalStatePath("task-1")))
	writeAuditStoreFile(t, statePath, mustMarshalState(t, first.PreviousState))
	stores := []*Store{openStateTestStore(t, repo, nil), openStateTestStore(t, repo, nil)}
	requests := []AuditCommitRequest{first, second}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, errs[index] = stores[index].CommitAudit(context.Background(), requests[index])
		}(i)
	}
	wg.Wait()
	successes, stale := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrStaleWrite) {
			stale++
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent errors=%v, want one success and one stale", errs)
	}
}

func auditStoreRequest(t *testing.T, repo string, taskRaw []byte) AuditCommitRequest {
	t.Helper()
	previous := stateTestPendingState("task-1")
	previous.Lifecycle = autonomous.LifecycleStateReady
	decisionArtifactPath := ".revolvr/runs/supervisor-audit/supervisor-decision.json"
	decisionArtifactRaw := []byte("{\"decision\":true}\n")
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(decisionArtifactPath)), decisionArtifactRaw)
	decision := autonomous.DecisionReference{DecisionID: "decision-audit", RunID: "supervisor-audit", TaskID: "task-1", Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, Artifact: stateTestEvidence(autonomous.EvidenceKindFile, decisionArtifactPath), CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
	next := previous
	next.LatestDecision = &decision
	next.FindingResolutions = []autonomous.FindingResolution{{FindingID: "finding-one", Status: autonomous.FindingResolutionStatusOpen}}
	verification := autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: "task-1", Status: autonomous.VerificationStatusPassed, Summary: "passed", RunID: "verification-run", OccurrenceID: "occurrence-one", Evidence: []autonomous.EvidenceReference{stateTestEvidence(autonomous.EvidenceKindVerification, "verification-run:occurrence-one")}}, SourceRevision: strings.Repeat("4", 64)}
	report := autonomous.AuditReport{TaskID: "task-1", Disposition: autonomous.AuditDispositionChangesRequired, Rationale: "one finding", Inputs: verification.Summary.Evidence, Findings: []autonomous.AuditFinding{{ID: "finding-one", Significance: autonomous.FindingSignificanceBlocking, Summary: "defect", Evidence: []autonomous.EvidenceReference{stateTestEvidence(autonomous.EvidenceKindFile, "example.go")}, RequiredCorrection: "fix defect"}}}
	workerRun := "auditor-run"
	rawPath := ".revolvr/runs/auditor-run/auditor-output.raw.json"
	canonicalPath := ".revolvr/runs/auditor-run/auditor-output.canonical.json"
	profilePath := ".agent/profiles/auditor.md"
	profileRaw := []byte("auditor profile\n")
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(profilePath)), profileRaw)
	dossier := autonomousaudit.DossierIdentity{SchemaVersion: autonomous.DossierManifestSchemaVersion, TaskID: "task-1", SHA256: strings.Repeat("6", 64), ByteSize: 100}
	profile := autonomousaudit.ProfileIdentity{Name: autonomous.WorkerProfileAuditor, Path: profilePath, SHA256: hashBytes(profileRaw), ByteSize: len(profileRaw)}
	mutation := &autonomousaudit.SourceMutationIdentity{TaskID: "task-1", RunID: "worker-run", Action: autonomous.ActionImplement, ResultingRevision: verification.SourceRevision}
	output := autonomousaudit.AuditOutput{SchemaVersion: autonomousaudit.AuditOutputSchemaVersion, TaskID: "task-1", Report: report, Provenance: autonomousaudit.AuditProvenance{Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor, WorkerRunID: workerRun, Decision: decision, Dossier: dossier, Profile: profile, RawOutputPath: rawPath, SourceRevision: verification.SourceRevision, Verification: verification, LatestSourceMutation: mutation}}
	canonical, err := autonomousaudit.MarshalAuditOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	writeAuditStoreFile(t, filepath.Join(repo, filepath.FromSlash(rawPath)), raw)
	previousRaw := mustMarshalState(t, previous)
	nextRaw := mustMarshalState(t, next)
	record := AuditHistoryRecord{SchemaVersion: AuditHistorySchemaVersion, TaskID: "task-1", Sequence: 1, AuditRevision: 1, OperationID: "audit-operation-one", ApplicationSHA256: strings.Repeat("5", 64), Kind: AuditTransitionRecorded, CreatedAt: time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC), Decision: decision, SupervisorDecision: artifactIdentity(decisionArtifactPath, decisionArtifactRaw), WorkerRunID: workerRun, Profile: profile, Dossier: dossier, SourceRevision: verification.SourceRevision, Verification: verification, LatestSourceMutation: mutation, TaskSource: artifactIdentity(".agent/tasks/task-1.md", taskRaw), RawOutput: artifactIdentity(rawPath, raw), CanonicalOutput: artifactIdentity(canonicalPath, canonical), Report: report, PolicyEvidence: autonomouspolicy.AuditEvidence{Report: report, RunID: workerRun, AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: verification.SourceRevision, VerificationRunID: verification.Summary.RunID, VerificationOccurrenceID: verification.Summary.OccurrenceID}, PreviousState: stateIdentity(canonicalStatePath("task-1"), true, previousRaw), ResultingState: stateIdentity(canonicalStatePath("task-1"), true, nextRaw), ResultingResolutions: cloneAuditStoreResolutions(next.FindingResolutions), NewFindingIDs: []string{"finding-one"}}
	return AuditCommitRequest{TaskID: "task-1", Expected: ExpectedState{Exists: true, SHA256: hashBytes(previousRaw), ByteSize: len(previousRaw)}, PreviousState: previous, NextState: next, History: record, CanonicalOutput: canonical}
}
func mustMarshalState(t *testing.T, state autonomous.ExecutionState) []byte {
	t.Helper()
	raw, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func cloneAuditStoreResolutions(v []autonomous.FindingResolution) []autonomous.FindingResolution {
	return append([]autonomous.FindingResolution(nil), v...)
}
func writeAuditStoreFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
