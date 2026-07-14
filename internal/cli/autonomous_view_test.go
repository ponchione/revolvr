package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousview"
)

func TestTaskWhyActiveByteStable(t *testing.T) {
	root := t.TempDir()
	writeCLIViewFixture(t, root)
	got, err := executeCLI(t, root, "task", "why", "cli-view")
	if err != nil {
		t.Fatalf("task why error = %v", err)
	}
	want := "Why and routing\n" +
		"Latest decision: none\n" +
		"Currently admitted action: none\n" +
		"Scheduler readiness: ready\n" +
		"Next supervisor action: undetermined_requires_supervisor\n" +
		"- scheduler_selected_next: Shared ready ordering selects this task as the next autonomous task.\n" +
		"- plan_incomplete: The current durable plan still has nonterminal steps.\n" +
		"- acceptance_pending: 1 acceptance criterion/criteria remain pending.\n" +
		"- verification_unavailable: Current verification evidence is not available in the bounded snapshot.\n" +
		"- audit_unavailable: Current independent audit evidence is not available in the bounded snapshot.\n" +
		"- next_supervisor_undetermined: Revolvr cannot predict the next supervisor decision from lifecycle or plan position; a fresh supervisor must decide.\n"
	if got != want {
		t.Fatalf("task why output =\n%s\nwant =\n%s", got, want)
	}
	second, err := executeCLI(t, root, "task", "why", "cli-view")
	if err != nil || second != got {
		t.Fatalf("second output differs: err=%v output=%q", err, second)
	}
}

func TestTaskShowHumanAndJSON(t *testing.T) {
	root := t.TempDir()
	writeCLIViewFixture(t, root)
	human, err := executeCLI(t, root, "task", "show", "cli-view")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Autonomous task\n", "Plan\n", "Acceptance\n", "Findings\nnone\n", "Attempts and budgets\n", "Provenance and raw references\n", "Diagnostics\n"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}
	jsonOutput, err := executeCLI(t, root, "task", "show", "cli-view", "--json")
	if err != nil {
		t.Fatal(err)
	}
	view, err := autonomousview.Decode([]byte(jsonOutput))
	if err != nil {
		t.Fatalf("JSON output is not validated canonical view: %v\n%s", err, jsonOutput)
	}
	if view.Identity.TaskID != "cli-view" {
		t.Fatalf("identity = %#v", view.Identity)
	}
}

func TestAutonomousViewRenderingStableAcrossLifecycleLabels(t *testing.T) {
	lifecycles := []string{"pending", "ready", "planning", "working", "verifying", "auditing", "correcting", "needs_input", "blocked", "finalizing", "completed", "cancelled", "superseded", "abandoned"}
	for _, lifecycle := range lifecycles {
		t.Run(lifecycle, func(t *testing.T) {
			view := autonomousview.View{SchemaVersion: autonomousview.SchemaVersion, Identity: autonomousview.Identity{SourceKind: autonomousview.SourceActive, TaskID: "task", TaskPath: ".agent/tasks/task.md", TaskSHA256: strings.Repeat("a", 64), TaskByteSize: 1, Workflow: "autonomous-v1", TaskStatus: "pending", Lifecycle: lifecycle}, Summary: autonomousview.Summary{Phase: lifecycle}, Why: autonomousview.Why{LatestDecision: "none", CurrentlyAdmittedAction: "none", SchedulerReadiness: "not_available", NextSupervisorAction: "undetermined_requires_supervisor", Reasons: []autonomousview.WhyReason{{Code: "fixture", Text: "Stable lifecycle fixture."}}}, Acceptance: []autonomousview.Acceptance{}, Findings: []autonomousview.Finding{}, Attempts: autonomousview.Attempts{PerAction: []autonomousview.ActionAttempts{}, Budgets: []autonomousview.Budget{}, Events: []autonomousview.AttemptReference{}, Stops: []string{}}, Input: autonomousview.OperatorInput{State: "none"}, Verification: autonomousview.Verification{State: "not_available"}, Audit: autonomousview.Audit{State: "not_available"}, Workspace: autonomousview.Workspace{State: "none"}, Terminal: autonomousview.Terminal{State: "active"}, Provenance: autonomousview.Provenance{WorkerRunIDs: []string{}, VerificationRunIDs: []string{}, AuditRunIDs: []string{}, References: []autonomousview.Reference{}}, Diagnostics: []autonomousview.Diagnostic{}}
			var first, second bytes.Buffer
			if err := writeAutonomousTaskView(&first, view); err != nil {
				t.Fatal(err)
			}
			if err := writeAutonomousTaskView(&second, view); err != nil {
				t.Fatal(err)
			}
			if first.String() != second.String() || !strings.Contains(first.String(), "Lifecycle: "+lifecycle+"\n") {
				t.Fatalf("unstable output:\n%s", first.String())
			}
		})
	}
}

func TestTaskHelpListsEvidenceCommands(t *testing.T) {
	output, err := executeCLI(t, t.TempDir(), "task", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"show", "why"} {
		if !strings.Contains(output, command) {
			t.Fatalf("task help missing %q:\n%s", command, output)
		}
	}
}

func writeCLIViewFixture(t *testing.T, root string) {
	t.Helper()
	taskID := "cli-view"
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStateReady, Plan: &autonomous.TaskPlan{TaskID: taskID, ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindTask, Reference: taskID, Detail: "Task."}}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "Do the work.", Status: autonomous.PlanStepStatusPending}}}, AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "It works.", Status: autonomous.AcceptanceStatusPending}}, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnlimited}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnlimited}}}
	taskRaw := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/%s/state.json\n---\n# CLI view\n\nWork.\n", taskID, taskID))
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	for path, raw := range map[string][]byte{filepath.Join(root, ".agent", "tasks", taskID+".md"): taskRaw, filepath.Join(root, ".revolvr", "autonomous", "tasks", taskID, "state.json"): stateRaw} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
