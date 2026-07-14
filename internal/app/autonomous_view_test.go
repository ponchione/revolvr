package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousarchive"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousview"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

func TestShowAutonomousTaskActiveIsReadOnlyAndDegradesMalformedOptionalHistory(t *testing.T) {
	root := t.TempDir()
	state := simpleViewState("view-task", autonomous.LifecycleStateReady)
	writeActiveViewFixture(t, root, "view-task", "View task", state)
	historyDir := filepath.Join(root, ".revolvr", "autonomous", "tasks", "view-task", "history", "audit")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, "00000000000000000001-bad.json"), []byte("{bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)

	view, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, "view-task")
	if err != nil {
		t.Fatalf("ShowAutonomousTask() error = %v", err)
	}
	if view.Identity.SourceKind != "active" || view.Identity.Lifecycle != "ready" || view.Summary.Plan.Total != 1 {
		t.Fatalf("view = %#v", view)
	}
	if !hasDiagnostic(view.Diagnostics, "audit_history_unavailable") {
		t.Fatalf("diagnostics = %#v", view.Diagnostics)
	}
	if view.Why.NextSupervisorAction != "undetermined_requires_supervisor" {
		t.Fatalf("why = %#v", view.Why)
	}
	after := snapshotTree(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("read-only view changed repository tree\nbefore=%#v\nafter=%#v", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr", "cache", "dossier")); !os.IsNotExist(err) {
		t.Fatalf("dossier cache was created: %v", err)
	}
}

func TestProjectScheduledReadinessUsesSharedArchiveEvidence(t *testing.T) {
	task := taskfile.Task{ID: "dependent", Workflow: taskfile.WorkflowAutonomousV1, Status: taskfile.StatusPending, SourcePath: ".agent/tasks/dependent.md"}
	taskInput := taskscheduler.Task{ID: task.ID, Workflow: taskscheduler.WorkflowAutonomousV1, State: taskscheduler.StatePending, SourcePath: task.SourcePath, DependsOn: []string{"archived"}}
	tests := []struct {
		name        string
		archive     taskscheduler.Archive
		wantReason  taskscheduler.Reason
		wantWhy     string
		wantInvalid bool
	}{
		{name: "completed", archive: taskscheduler.Archive{TaskID: "archived", ArchiveID: "archive-completed", Disposition: taskscheduler.StateCompleted, Verified: true, Reconciled: true}, wantReason: taskscheduler.ReasonReady, wantWhy: "scheduler_selected_next"},
		{name: "cancelled", archive: taskscheduler.Archive{TaskID: "archived", ArchiveID: "archive-cancelled", Disposition: taskscheduler.StateCancelled, Reason: "operator cancelled", Verified: true, Reconciled: true}, wantReason: taskscheduler.ReasonTerminalUnsatisfiedDependency, wantWhy: string(taskscheduler.ReasonTerminalUnsatisfiedDependency)},
		{name: "abandoned", archive: taskscheduler.Archive{TaskID: "archived", ArchiveID: "archive-abandoned", Disposition: taskscheduler.StateAbandoned, Reason: "operator abandoned", Verified: true, Reconciled: true}, wantReason: taskscheduler.ReasonTerminalUnsatisfiedDependency, wantWhy: string(taskscheduler.ReasonTerminalUnsatisfiedDependency)},
		{name: "superseded", archive: taskscheduler.Archive{TaskID: "archived", ArchiveID: "archive-superseded", Disposition: taskscheduler.StateSuperseded, Reason: "operator superseded", Verified: true, Reconciled: true}, wantReason: taskscheduler.ReasonTerminalUnsatisfiedDependency, wantWhy: string(taskscheduler.ReasonTerminalUnsatisfiedDependency)},
		{name: "unverified", archive: taskscheduler.Archive{TaskID: "archived", ArchiveID: "archive-unverified", Disposition: taskscheduler.StateCompleted}, wantReason: taskscheduler.ReasonInvalidGraph, wantWhy: "scheduler_not_ready", wantInvalid: true},
		{name: "malformed disposition", archive: taskscheduler.Archive{TaskID: "archived", ArchiveID: "archive-malformed", Disposition: "expired", Verified: true, Reconciled: true}, wantReason: taskscheduler.ReasonInvalidGraph, wantWhy: "scheduler_not_ready", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := taskscheduler.Evaluate(taskscheduler.Input{Tasks: []taskscheduler.Task{taskInput}, Archives: []taskscheduler.Archive{tt.archive}, SelectionWorkflow: taskscheduler.WorkflowAutonomousV1})
			reason, why, diagnostics, err := projectScheduledReadiness(task, result, nil)
			if err != nil || reason != string(tt.wantReason) || !hasWhyReason(why, tt.wantWhy) {
				t.Fatalf("reason=%q why=%+v diagnostics=%+v err=%v", reason, why, diagnostics, err)
			}
			if tt.wantInvalid != hasDiagnostic(diagnostics, string(taskscheduler.DiagnosticMalformedArchive)) {
				t.Fatalf("diagnostics=%+v want malformed=%t", diagnostics, tt.wantInvalid)
			}
		})
	}
}

func TestProjectScheduledReadinessReportsExactSharedSelectedIdentity(t *testing.T) {
	viewed := taskfile.Task{ID: "second", Workflow: taskfile.WorkflowAutonomousV1, Status: taskfile.StatusPending, SourcePath: ".agent/tasks/second.md"}
	result := taskscheduler.Evaluate(taskscheduler.Input{
		Tasks: []taskscheduler.Task{
			{ID: "second", Workflow: taskscheduler.WorkflowAutonomousV1, State: taskscheduler.StatePending, SourcePath: viewed.SourcePath, HasPriority: true, Priority: 2},
			{ID: "first", Workflow: taskscheduler.WorkflowAutonomousV1, State: taskscheduler.StatePending, SourcePath: ".agent/tasks/first.md", HasPriority: true, Priority: 1},
		},
		SelectionWorkflow: taskscheduler.WorkflowAutonomousV1,
	})
	reason, why, diagnostics, err := projectScheduledReadiness(viewed, result, nil)
	if err != nil || reason != string(taskscheduler.ReasonReady) || !hasWhyReason(why, "scheduler_ready_not_selected") || len(diagnostics) != 0 {
		t.Fatalf("reason=%q why=%+v diagnostics=%+v err=%v", reason, why, diagnostics, err)
	}
	if !strings.Contains(why[0].Text, "first") {
		t.Fatalf("why=%+v, want selected task identity", why)
	}
}

func TestShowAutonomousTaskMixedPassReturnsExplicitUnavailable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agent", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte("---\nid: mixed-task\nstatus: pending\n---\n# Mixed task\n\nWork.\n")
	if err := os.WriteFile(filepath.Join(root, ".agent", "tasks", "mixed-task.md"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	view, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, "mixed-task")
	if err != nil {
		t.Fatal(err)
	}
	if view.Identity.Workflow != "mixed-pass-v1" || view.Identity.Lifecycle != "" || !hasDiagnostic(view.Diagnostics, "wrong_workflow") {
		t.Fatalf("view = %#v", view)
	}
}

func TestShowAutonomousTaskCancelledArchiveDoesNotClaimVerification(t *testing.T) {
	root := t.TempDir()
	archiveID := writeCancelledArchiveFixture(t, root, "archived-task")
	view, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, archiveID)
	if err != nil {
		t.Fatalf("ShowAutonomousTask() error = %v", err)
	}
	if view.Identity.SourceKind != "archive" || view.Identity.ArchiveDisposition != "cancelled" || view.Terminal.VerifiedNow {
		t.Fatalf("archive view = %#v", view)
	}
	if !hasDiagnostic(view.Diagnostics, "archive_verification_not_run") || !hasDiagnostic(view.Diagnostics, "completion_evidence_not_applicable") {
		t.Fatalf("diagnostics = %#v", view.Diagnostics)
	}
	if view.Why.NextSupervisorAction != "not_applicable_terminal" {
		t.Fatalf("why = %#v", view.Why)
	}
}

func TestShowAutonomousTaskRejectsActiveArchiveAmbiguity(t *testing.T) {
	root := t.TempDir()
	writeActiveViewFixture(t, root, "same-task", "Same", simpleViewState("same-task", autonomous.LifecycleStateReady))
	writeCancelledArchiveFixture(t, root, "same-task")
	_, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, "same-task")
	if err == nil || !strings.Contains(err.Error(), "ambiguous between active task") {
		t.Fatalf("error = %v", err)
	}
}

func TestShowAutonomousTaskRedactsConfiguredSecrets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REVOLVR_VIEW_SECRET", "view-super-secret")
	writeActiveViewFixture(t, root, "secret-task", "Title view-super-secret", simpleViewState("secret-task", autonomous.LifecycleStateReady))
	configPath := filepath.Join(root, ".revolvr", "config.yaml")
	if err := os.WriteFile(configPath, []byte("autonomy:\n  redaction:\n    environment_variables: [REVOLVR_VIEW_SECRET]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	view, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, "secret-task")
	if err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf("%#v", view)
	if strings.Contains(raw, "view-super-secret") || !strings.Contains(raw, "[REDACTED]") {
		t.Fatalf("view was not redacted: %s", raw)
	}
}

func TestShowAutonomousTaskMalformedLatestDecisionMarksRouteUnknown(t *testing.T) {
	root := t.TempDir()
	state := simpleViewState("decision-task", autonomous.LifecycleStateReady)
	state.LatestDecision = &autonomous.DecisionReference{DecisionID: "decision-one", RunID: "supervisor-run", TaskID: "decision-task", Action: autonomous.ActionImplement, WorkerProfile: autonomous.WorkerProfileImplementer, Artifact: autonomous.EvidenceReference{Kind: autonomous.EvidenceKindFile, Reference: ".revolvr/runs/supervisor-run/supervisor-decision.json", Detail: "Accepted decision."}, CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}
	writeActiveViewFixture(t, root, "decision-task", "Decision", state)
	decisionPath := filepath.Join(root, ".revolvr", "runs", "supervisor-run", "supervisor-decision.json")
	if err := os.MkdirAll(filepath.Dir(decisionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decisionPath, []byte("{malformed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	view, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, "decision-task")
	if err != nil {
		t.Fatal(err)
	}
	if view.Why.CurrentlyAdmittedAction != "none" || !hasDiagnostic(view.Diagnostics, "latest_decision_malformed") {
		t.Fatalf("view = %#v", view)
	}
}

func TestShowAutonomousTaskCancellationAndMalformedCanonicalStateFailClosed(t *testing.T) {
	root := t.TempDir()
	writeActiveViewFixture(t, root, "broken-task", "Broken", simpleViewState("broken-task", autonomous.LifecycleStateReady))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ShowAutonomousTask(ctx, Config{WorkDir: root}, "broken-task"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	statePath := filepath.Join(root, ".revolvr", "autonomous", "tasks", "broken-task", "state.json")
	if err := os.WriteFile(statePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ShowAutonomousTask(context.Background(), Config{WorkDir: root}, "broken-task"); err == nil || !strings.Contains(err.Error(), "canonical state") {
		t.Fatalf("malformed state error = %v", err)
	}
}

func simpleViewState(taskID string, lifecycle autonomous.LifecycleState) autonomous.ExecutionState {
	return autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: lifecycle, Plan: &autonomous.TaskPlan{TaskID: taskID, ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: taskID, Detail: "Task."}}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "Do work.", Status: autonomous.PlanStepStatusPending}}}, AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "Works.", Status: autonomous.AcceptanceStatusPending}}, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}}}
}

func writeActiveViewFixture(t *testing.T, root, taskID, title string, state autonomous.ExecutionState) {
	t.Helper()
	taskDir := filepath.Join(root, ".agent", "tasks")
	stateDir := filepath.Join(root, ".revolvr", "autonomous", "tasks", taskID)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	taskRaw := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\n---\n# %s\n\nWork.\n", taskID, taskID, title))
	if err := os.WriteFile(filepath.Join(taskDir, taskID+".md"), taskRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCancelledArchiveFixture(t *testing.T, root, taskID string) string {
	t.Helper()
	terminalAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	archivedAt := terminalAt.Add(time.Minute)
	archiveID := autonomousarchive.ArchiveID(taskID, "archive-operation", autonomousarchive.DispositionCancelled, archivedAt)
	base := filepath.ToSlash(filepath.Join(".agent", "archive", "2026", "07", taskID))
	originalPath := filepath.ToSlash(filepath.Join(".agent", "tasks", taskID+".md"))
	statePath := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json"))
	taskRaw := []byte(fmt.Sprintf("---\nid: %s\nstatus: cancelled\nworkflow: autonomous-v1\nautonomous_state_path: %s\n---\n# Archived task\n\nWork.\n", taskID, statePath))
	state := simpleViewState(taskID, autonomous.LifecycleStateCancelled)
	state.Terminal = &autonomous.TerminalDetail{Reason: "Operator cancelled.", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindLedger, Reference: "terminal-run", Detail: "Cancellation."}}}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	taskArtifact := testArchiveArtifact(filepath.ToSlash(filepath.Join(base, "task.md")), taskRaw)
	manifestPath := filepath.ToSlash(filepath.Join(base, "archive.json"))
	manifest := autonomousarchive.Manifest{SchemaVersion: autonomousarchive.ManifestSchemaVersion, ArchiveID: archiveID, OperationID: "archive-operation", ArchiveRunID: "archive-run", TaskID: taskID, Disposition: autonomousarchive.DispositionCancelled, Reason: "Operator cancelled.", Provenance: "operator", TerminalAt: terminalAt, ArchivedAt: archivedAt, OriginalTask: testArchiveArtifact(originalPath, taskRaw), ArchivedTask: taskArtifact, Workflow: "autonomous-v1", State: testArchiveArtifact(statePath, stateRaw), ExpectedPaths: []string{taskArtifact.Path, manifestPath}, Omissions: []string{"completion evidence is not applicable"}}
	manifestRaw, err := autonomousarchive.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for path, raw := range map[string][]byte{taskArtifact.Path: taskRaw, manifestPath: manifestRaw, statePath: stateRaw} {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return archiveID
}

func testArchiveArtifact(path string, raw []byte) autonomousarchive.Artifact {
	sum := sha256.Sum256(raw)
	return autonomousarchive.Artifact{Path: path, SHA256: fmt.Sprintf("%x", sum), ByteSize: len(raw)}
}

type treeFact struct {
	Mode    os.FileMode
	Size    int64
	ModTime int64
	Content string
}

func snapshotTree(t *testing.T, root string) map[string]treeFact {
	t.Helper()
	result := map[string]treeFact{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		fact := treeFact{Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fact.Content = string(raw)
		}
		result[filepath.ToSlash(rel)] = fact
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func hasDiagnostic(values []autonomousview.Diagnostic, code string) bool {
	for _, item := range values {
		if item.Code == code {
			return true
		}
	}
	return false
}

func hasWhyReason(values []autonomousview.WhyReason, code string) bool {
	for _, item := range values {
		if item.Code == code {
			return true
		}
	}
	return false
}
