package autonomousoptional

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
)

func TestApplyOptionalRoleOccurrenceIsAppendOnlyReplayableAndTaskIsolated(t *testing.T) {
	repo := t.TempDir()
	storeA, snapshotA := optionalStore(t, repo, "task-a")
	_, snapshotB := optionalStore(t, repo, "task-b")
	assessment := testAssessment("task-a", autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionNotApplicable, snapshotA.SHA256, testHash("source-a"), "decision-a")
	occurrence := testSkippedOccurrence(t, assessment, 1)
	cfg := ApplyConfig{TaskID: "task-a", OperationID: "skip-document-a", Expected: snapshotA.Expected(), Assessment: assessment, Occurrence: occurrence, CreatedAt: occurrence.CreatedAt, Store: storeA}

	first, err := Apply(context.Background(), cfg)
	if err != nil || first.Disposition != autonomousstate.CommitUpdated || len(first.Current.State.OptionalRoles) != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	replay, err := Apply(context.Background(), cfg)
	if err != nil || replay.Disposition != autonomousstate.CommitReplayed || !reflect.DeepEqual(replay.Current.State.OptionalRoles, first.Current.State.OptionalRoles) {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	history, found, err := storeA.LoadOptionalRoleOperation(context.Background(), "task-a", cfg.OperationID)
	if err != nil || !found || history.Record.Occurrence.Outcome != autonomous.OptionalRoleOutcomeNotApplicable {
		t.Fatalf("history=%+v found=%t err=%v", history, found, err)
	}
	loadedB, found, err := storeA.Load(context.Background(), "task-b")
	if err != nil || !found || loadedB.SHA256 != snapshotB.SHA256 || len(loadedB.State.OptionalRoles) != 0 {
		t.Fatalf("task-b=%+v found=%t err=%v", loadedB, found, err)
	}

	conflict := cfg
	conflict.Occurrence.Rationale = "materially different"
	if _, err := Apply(context.Background(), conflict); !errors.Is(err, autonomousstate.ErrOperationConflict) {
		t.Fatalf("conflict error=%v", err)
	}
}

func TestApplyOptionalRoleOccurrenceRejectsStaleWriterAndPreservesOlderSourceEvidence(t *testing.T) {
	repo := t.TempDir()
	store, first := optionalStore(t, repo, "task-stale")
	oldAssessment := testAssessment("task-stale", autonomous.WorkerProfileSimplifier, autonomous.OptionalRoleDispositionNotApplicable, first.SHA256, testHash("source-old"), "decision-old")
	oldOccurrence := testSkippedOccurrence(t, oldAssessment, 1)
	old, err := Apply(context.Background(), ApplyConfig{TaskID: "task-stale", OperationID: "skip-old", Expected: first.Expected(), Assessment: oldAssessment, Occurrence: oldOccurrence, CreatedAt: oldOccurrence.CreatedAt, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	newAssessment := testAssessment("task-stale", autonomous.WorkerProfileDocumentor, autonomous.OptionalRoleDispositionNotApplicable, old.Current.SHA256, testHash("source-new"), "decision-new")
	newOccurrence := testSkippedOccurrence(t, newAssessment, 2)
	if _, err := Apply(context.Background(), ApplyConfig{TaskID: "task-stale", OperationID: "skip-new", Expected: first.Expected(), Assessment: newAssessment, Occurrence: newOccurrence, CreatedAt: newOccurrence.CreatedAt, Store: store}); !errors.Is(err, autonomousstate.ErrStaleWrite) {
		t.Fatalf("stale error=%v", err)
	}
	newResult, err := Apply(context.Background(), ApplyConfig{TaskID: "task-stale", OperationID: "skip-new", Expected: old.Current.Expected(), Assessment: newAssessment, Occurrence: newOccurrence, CreatedAt: newOccurrence.CreatedAt, Store: store})
	if err != nil || len(newResult.Current.State.OptionalRoles) != 2 || newResult.Current.State.OptionalRoles[0].SourceAfter != testHash("source-old") || newResult.Current.State.OptionalRoles[1].SourceAfter != testHash("source-new") {
		t.Fatalf("new=%+v err=%v", newResult, err)
	}
}

func optionalStore(t *testing.T, repo, taskID string) (*autonomousstate.Store, autonomousstate.Snapshot) {
	t.Helper()
	taskPath := filepath.Join(repo, ".agent", "tasks", taskID+".md")
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		t.Fatal(err)
	}
	task := fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\n---\n# Task\n\nTest.\n", taskID, taskID)
	if err := os.WriteFile(taskPath, []byte(task), 0o644); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStateReady, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(repo, ".revolvr", "autonomous", "tasks", taskID, "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, found, err := store.Load(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("load found=%t err=%v", found, err)
	}
	return store, snapshot
}

func testAssessment(taskID string, role autonomous.WorkerProfile, disposition autonomous.OptionalRoleDisposition, stateSHA, source, decisionID string) autonomous.OptionalRoleAssessment {
	action := autonomous.ActionDocument
	if role == autonomous.WorkerProfileSimplifier {
		action = autonomous.ActionSimplify
	}
	evidence := testEvidence("scope-" + decisionID)
	decision := autonomous.SupervisorDecision{TaskID: taskID, Action: action, WorkerProfile: role, Rationale: "Exact evidence supports the disposition.", SuccessCriteria: []string{"Record the conditional role outcome."}, Inputs: []autonomous.EvidenceReference{evidence}, Strategy: &autonomous.Strategy{Approach: "use exact selected evidence", Targets: []autonomous.EvidenceReference{evidence}}}
	return autonomous.OptionalRoleAssessment{SchemaVersion: autonomous.OptionalRoleAssessmentSchemaVersion, TaskID: taskID, Role: role, Disposition: disposition, Decision: decision, DecisionReference: autonomous.DecisionReference{DecisionID: decisionID, RunID: "supervisor-" + decisionID, TaskID: taskID, Action: action, WorkerProfile: role, Artifact: testEvidence("artifact-" + decisionID), CreatedAt: testTime}, TaskSource: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindTask, Reference: ".agent/tasks/" + taskID + ".md", Detail: "Exact task source."}, StateSHA256: stateSHA, SourceRevision: source, VerificationRunID: "verification-" + decisionID, VerificationID: "occurrence-" + decisionID, AuditRunID: "audit-" + decisionID, AuditSourceRevision: source, Evidence: []autonomous.OptionalRoleEvidence{{ID: "scope-one", Role: role, Kind: autonomous.OptionalRoleEvidenceNoRelevantWork, Reference: evidence, SourceRevision: source}}, SelectedEvidenceIDs: []string{"scope-one"}, Rationale: "No structured obligation or target exists for this role."}
}

func testSkippedOccurrence(t *testing.T, assessment autonomous.OptionalRoleAssessment, sequence int64) autonomous.OptionalRoleOccurrence {
	t.Helper()
	sha, err := assessment.Identity()
	if err != nil {
		t.Fatal(err)
	}
	return autonomous.OptionalRoleOccurrence{SchemaVersion: autonomous.OptionalRoleOccurrenceSchemaVersion, Sequence: sequence, TaskID: assessment.TaskID, Role: assessment.Role, Outcome: autonomous.OptionalRoleOutcomeNotApplicable, Decision: assessment.DecisionReference, AssessmentSHA256: sha, SourceBefore: assessment.SourceRevision, SourceAfter: assessment.SourceRevision, Gate: autonomous.OptionalRoleGate{SourceRevision: assessment.SourceRevision, VerificationRunID: assessment.VerificationRunID, VerificationOccurrenceID: assessment.VerificationID, AuditSupervisorRunID: "audit-supervisor-" + assessment.DecisionReference.DecisionID, AuditWorkerRunID: assessment.AuditRunID, AuditRevision: sequence}, Evidence: []autonomous.EvidenceReference{assessment.TaskSource, assessment.Evidence[0].Reference}, Rationale: assessment.Rationale, CreatedAt: testTime.Add(time.Duration(sequence) * time.Minute)}
}

var testTime = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

func testEvidence(reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: autonomous.EvidenceKindLedger, Reference: reference, Detail: "Exact test evidence."}
}

func testHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}
