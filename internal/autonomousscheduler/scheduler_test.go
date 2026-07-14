package autonomousscheduler

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

func task(id string, priority int, dependencies ...string) ActiveTask {
	return ActiveTask{Task: taskfile.Task{ID: id, Title: id, Status: taskfile.StatusPending, Workflow: taskfile.WorkflowAutonomousV1, HasPriority: true, Priority: priority, DependsOn: dependencies, SourcePath: ".agent/tasks/" + id + ".md"}, Lifecycle: "pending"}
}

func TestAdapterUsesSharedDependencyStateSemantics(t *testing.T) {
	tests := []struct {
		name, lifecycle, status string
		want                    taskscheduler.Reason
	}{
		{name: "pending", lifecycle: "pending", status: taskfile.StatusPending, want: taskscheduler.ReasonWaitingDependency},
		{name: "completed", lifecycle: "completed", status: taskfile.StatusCompleted, want: taskscheduler.ReasonReady},
		{name: "running", lifecycle: "working", status: taskfile.StatusPending, want: taskscheduler.ReasonWaitingDependency},
		{name: "blocked", lifecycle: "blocked", status: taskfile.StatusBlocked, want: taskscheduler.ReasonBlockedDependency},
		{name: "needs input", lifecycle: "needs_input", status: taskfile.StatusPending, want: taskscheduler.ReasonNeedsInputDependency},
		{name: "cancelled", lifecycle: "cancelled", status: taskfile.StatusCancelled, want: taskscheduler.ReasonTerminalUnsatisfiedDependency},
		{name: "abandoned", lifecycle: "abandoned", status: taskfile.StatusAbandoned, want: taskscheduler.ReasonTerminalUnsatisfiedDependency},
		{name: "superseded", lifecycle: "superseded", status: taskfile.StatusSuperseded, want: taskscheduler.ReasonTerminalUnsatisfiedDependency},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependency := task("dependency", 50)
			dependency.Lifecycle, dependency.Task.Status = tt.lifecycle, tt.status
			graph, err := BuildSnapshot([]ActiveTask{task("dependent", 1, "dependency"), dependency}, nil)
			if err != nil {
				t.Fatal(err)
			}
			node, err := ClassifyTask(graph, "dependent", nil)
			if err != nil || node.Reason != tt.want {
				t.Fatalf("node=%+v err=%v, want reason %s", node, err, tt.want)
			}
			if tt.want == taskscheduler.ReasonReady {
				if len(node.WaitingOn) != 0 {
					t.Fatalf("completed dependency remained waiting: %+v", node)
				}
			} else if !reflect.DeepEqual(node.WaitingOn, []string{"dependency"}) {
				t.Fatalf("waiting=%v, want dependency", node.WaitingOn)
			}
		})
	}
}

func TestAdapterUsesSharedReadyOrderingSelectionAndConflicts(t *testing.T) {
	mixed := task("mixed", -100)
	mixed.Task.Workflow = taskfile.WorkflowMixedPassV1
	mixed.Lifecycle = ""
	a := task("a", 1)
	b := task("b", 1)
	a.Task.SourcePath = ".agent/tasks/same.md"
	b.Task.SourcePath = ".agent/tasks/same.md"
	a.Task.Conflicts = []string{"resource"}
	b.Task.Conflicts = []string{"resource"}
	graph, err := BuildSnapshot([]ActiveTask{b, mixed, a}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectNextReady(graph, nil)
	if !selected.Found || selected.Task.ID != "a" {
		t.Fatalf("selection=%+v, want autonomous a", selected)
	}
	nodes := ClassifyAll(graph, nil)
	var ready []string
	for _, node := range nodes {
		if node.Reason == taskscheduler.ReasonReady && node.Task.Workflow == taskfile.WorkflowAutonomousV1 {
			ready = append(ready, node.Task.ID)
		}
	}
	if !reflect.DeepEqual(ready, []string{"a", "b"}) {
		t.Fatalf("ready order=%v", ready)
	}
	conflict, err := ClassifyTask(graph, "b", []string{"a"})
	if err != nil || conflict.Reason != taskscheduler.ReasonConflictBlocked || !reflect.DeepEqual(conflict.Conflicts, []string{"a"}) {
		t.Fatalf("conflict=%+v err=%v", conflict, err)
	}
}

func TestAdapterUsesSharedCrossWorkflowDependencyEvidence(t *testing.T) {
	mixed := task("mixed-dependency", 10)
	mixed.Task.Workflow = taskfile.WorkflowMixedPassV1
	mixed.Lifecycle = ""
	dependent := task("autonomous-dependent", 1, mixed.Task.ID)

	graph, err := BuildSnapshot([]ActiveTask{dependent, mixed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := ClassifyTask(graph, dependent.Task.ID, nil)
	if err != nil || waiting.Reason != taskscheduler.ReasonWaitingDependency || len(waiting.Issues) != 1 || waiting.Issues[0].Workflow != taskscheduler.WorkflowMixedPassV1 {
		t.Fatalf("waiting=%+v err=%v", waiting, err)
	}

	mixed.Task.Status = taskfile.StatusCompleted
	graph, err = BuildSnapshot([]ActiveTask{dependent, mixed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectNextReady(graph, nil)
	if !selected.Found || selected.Task.ID != dependent.Task.ID {
		t.Fatalf("selection=%+v, want unlocked autonomous dependent", selected)
	}
}

func TestAdapterPreservesEveryArchiveDispositionReason(t *testing.T) {
	for _, disposition := range []string{taskfile.StatusCompleted, taskfile.StatusCancelled, taskfile.StatusAbandoned, taskfile.StatusSuperseded} {
		t.Run(disposition, func(t *testing.T) {
			archive := ArchiveEvidence{TaskID: "archived", ArchiveID: "archive-one", Disposition: disposition, Verified: true, Reconciled: true}
			if disposition != taskfile.StatusCompleted {
				archive.Reason = "operator recorded " + disposition
			}
			graph, err := BuildSnapshot([]ActiveTask{task("dependent", 1, "archived")}, []ArchiveEvidence{archive})
			if err != nil {
				t.Fatal(err)
			}
			node, err := ClassifyTask(graph, "dependent", nil)
			if err != nil {
				t.Fatal(err)
			}
			if disposition == taskfile.StatusCompleted {
				if node.Reason != taskscheduler.ReasonReady {
					t.Fatalf("completed archive node=%+v", node)
				}
				return
			}
			if node.Reason != taskscheduler.ReasonTerminalUnsatisfiedDependency || len(node.Issues) != 1 || node.Issues[0].Detail != archive.Reason || node.Issues[0].ArchiveID != archive.ArchiveID {
				t.Fatalf("terminal archive node=%+v", node)
			}
		})
	}
}

func TestAdapterReturnsSharedInvalidGraphDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		active   []ActiveTask
		archives []ArchiveEvidence
		code     taskscheduler.DiagnosticCode
	}{
		{name: "missing", active: []ActiveTask{task("a", 1, "missing")}, code: taskscheduler.DiagnosticMissingDependency},
		{name: "duplicate", active: []ActiveTask{task("a", 1), task("a", 2)}, code: taskscheduler.DiagnosticDuplicateTaskID},
		{name: "ambiguous", active: []ActiveTask{task("a", 1)}, archives: []ArchiveEvidence{{TaskID: "a", ArchiveID: "archive-a", Disposition: "completed", Verified: true, Reconciled: true}}, code: taskscheduler.DiagnosticActiveArchiveAmbiguity},
		{name: "cycle", active: []ActiveTask{task("a", 1, "b"), task("b", 1, "c"), task("c", 1, "a")}, code: taskscheduler.DiagnosticDependencyCycle},
		{name: "unverified archive", active: []ActiveTask{task("a", 1, "archived")}, archives: []ArchiveEvidence{{TaskID: "archived", ArchiveID: "archive-a", Disposition: "completed"}}, code: taskscheduler.DiagnosticMalformedArchive},
		{name: "noncompleted archive without reason", active: []ActiveTask{task("a", 1, "archived")}, archives: []ArchiveEvidence{{TaskID: "archived", ArchiveID: "archive-a", Disposition: "cancelled", Verified: true, Reconciled: true}}, code: taskscheduler.DiagnosticMalformedArchive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildResult(tt.active, tt.archives, nil)
			if result.Valid() || result.SelectedNext != nil || !hasDiagnostic(result.InvalidGraph, tt.code) {
				t.Fatalf("result=%+v", result)
			}
			_, err := BuildSnapshot(tt.active, tt.archives)
			var graphErr GraphError
			if !errors.As(err, &graphErr) || !hasDiagnostic(graphErr.Diagnostics, tt.code) || !strings.Contains(err.Error(), string(tt.code)) {
				t.Fatalf("error=%v diagnostics=%+v", err, graphErr.Diagnostics)
			}
		})
	}
}

func hasDiagnostic(diagnostics []taskscheduler.Diagnostic, code taskscheduler.DiagnosticCode) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
