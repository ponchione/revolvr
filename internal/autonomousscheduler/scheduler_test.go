package autonomousscheduler

import (
	"strings"
	"testing"

	"revolvr/internal/taskfile"
)

func task(id string, priority int, deps ...string) ActiveTask {
	return ActiveTask{Task: taskfile.Task{ID: id, Title: id, Status: taskfile.StatusPending, Workflow: taskfile.WorkflowAutonomousV1, HasPriority: true, Priority: priority, DependsOn: deps, SourcePath: ".agent/tasks/" + id + ".md"}, Lifecycle: "pending"}
}

func TestDependencyDAGAndDeterministicPrioritySelection(t *testing.T) {
	a := task("a", 5)
	a.Task.Status = taskfile.StatusCompleted
	b := task("b", 2, "a")
	c := task("c", 1, "a")
	d := task("d", 0, "b", "c")
	g, err := BuildSnapshot([]ActiveTask{d, b, a, c}, nil)
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectNextReady(g, nil)
	if !selected.Found || selected.Task.ID != "c" {
		t.Fatalf("selection = %#v, want c", selected)
	}
	node, err := ClassifyTask(g, "d", nil)
	if err != nil || node.Reason != ReasonWaitingDependency || strings.Join(node.WaitingOn, ",") != "b,c" {
		t.Fatalf("d = %#v, %v", node, err)
	}
}

func TestCompletedArchiveUnlocksButIsNeverSelected(t *testing.T) {
	g, err := BuildSnapshot([]ActiveTask{task("dependent", 1, "archived")}, []ArchiveEvidence{{TaskID: "archived", ArchiveID: "archive-1", Disposition: "completed", Verified: true, Reconciled: true}})
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectNextReady(g, nil)
	if !selected.Found || selected.Task.ID != "dependent" {
		t.Fatalf("selection = %#v", selected)
	}
}

func TestGraphRejectsMissingDuplicateAmbiguousAndCycles(t *testing.T) {
	tests := []struct {
		name     string
		active   []ActiveTask
		archives []ArchiveEvidence
		want     string
	}{
		{"missing", []ActiveTask{task("a", 1, "missing")}, nil, `missing dependency "missing"`},
		{"duplicate", []ActiveTask{task("a", 1), task("a", 2)}, nil, `duplicate active task id "a"`},
		{"ambiguous", []ActiveTask{task("a", 1)}, []ArchiveEvidence{{TaskID: "a", ArchiveID: "archive-a", Disposition: "completed", Verified: true, Reconciled: true}}, `ambiguous between active and archived`},
		{"cycle", []ActiveTask{task("a", 1, "b"), task("b", 1, "c"), task("c", 1, "a")}, nil, `a -> b -> c -> a`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildSnapshot(tt.active, tt.archives)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPriorityTieAndConflictsAreStable(t *testing.T) {
	b := task("b", 1)
	a := task("a", 1)
	b.Task.SourcePath = ".agent/tasks/001.md"
	a.Task.SourcePath = ".agent/tasks/001.md"
	b.Task.Conflicts = []string{"resource"}
	a.Task.Conflicts = []string{"resource"}
	g, err := BuildSnapshot([]ActiveTask{b, a}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := SelectNextReady(g, nil).Task.ID; got != "a" {
		t.Fatalf("tie selection = %q, want a", got)
	}
	if node, _ := ClassifyTask(g, "b", []string{"a"}); node.Reason != ReasonConflict {
		t.Fatalf("conflict node = %#v", node)
	}
}

func TestDependencyStateReasons(t *testing.T) {
	for _, tt := range []struct {
		name, lifecycle, status string
		want                    Reason
	}{
		{"pending", "pending", taskfile.StatusPending, ReasonWaitingDependency},
		{"blocked", "blocked", taskfile.StatusBlocked, ReasonBlockedDependency},
		{"input", "needs_input", taskfile.StatusPending, ReasonNeedsInput},
		{"cancelled", "cancelled", taskfile.StatusCancelled, ReasonWaitingDependency},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dep := task("dep", 1)
			dep.Lifecycle, dep.Task.Status = tt.lifecycle, tt.status
			g, err := BuildSnapshot([]ActiveTask{dep, task("child", 1, "dep")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			node, _ := ClassifyTask(g, "child", nil)
			if node.Reason != tt.want {
				t.Fatalf("reason = %s, want %s", node.Reason, tt.want)
			}
		})
	}
}
