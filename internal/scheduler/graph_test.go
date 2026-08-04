package scheduler

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestGraphSelectsDeterministicallyAndClassifiesWaiting(t *testing.T) {
	created := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	completed := graphFixture("completed", "completed", 50, created)
	first := graphFixture("first", "pending", 10, created)
	first.dependencies = []graphEdge{{versionID: first.taskVersionID, targetID: completed.taskID}}
	second := graphFixture("second", "pending", 10, created)
	second.createdAt = created.Add(time.Second)
	waiting := graphFixture("waiting", "pending", 1, created)
	waiting.dependencies = []graphEdge{{versionID: waiting.taskVersionID, targetID: second.taskID}}
	checkpoint := graphFixture("checkpoint", "pending", 2, created)
	checkpoint.awaitingOperatorCheckpoint = true
	terminal := graphFixture("terminal", "cancelled", 50, created)
	terminalWait := graphFixture("terminal-wait", "pending", 3, created)
	terminalWait.dependencies = []graphEdge{{versionID: terminalWait.taskVersionID, targetID: terminal.taskID}}
	conflictWait := graphFixture("conflict-wait", "pending", 4, created)
	conflictWait.conflicts = []graphEdge{{versionID: conflictWait.taskVersionID, targetID: completed.taskID}}

	result := evaluateGraph([]graphTask{second, checkpoint, terminalWait, completed, conflictWait, waiting, terminal, first})
	if len(result.diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.diagnostics)
	}
	if result.candidate == nil || result.candidate.TaskID != "first" {
		t.Fatalf("candidate = %#v, want first", result.candidate)
	}
	wantWaiting := []WaitingTask{
		{TaskID: "waiting", Reason: "waiting_dependency:second"},
		{TaskID: "checkpoint", Reason: "awaiting_operator_checkpoint"},
		{TaskID: "terminal-wait", Reason: "terminal_unsatisfied_dependency:terminal"},
		{TaskID: "conflict-wait", Reason: "conflict:completed"},
	}
	if !slices.Equal(result.waiting, wantWaiting) {
		t.Fatalf("waiting = %#v, want %#v", result.waiting, wantWaiting)
	}

	reversed := evaluateGraph([]graphTask{first, terminal, waiting, conflictWait, completed, terminalWait, checkpoint, second})
	if reversed.candidate == nil || !sameCandidate(*result.candidate, *reversed.candidate) || !slices.Equal(reversed.waiting, wantWaiting) {
		t.Fatalf("reordered result = %#v, want identical selection", reversed)
	}
}

func TestGraphReturnsTypedEmptyOutcomes(t *testing.T) {
	if _, err := selectProjection(graphResult{}); !errors.Is(err, ErrNoReady) {
		t.Fatalf("empty selection error = %v, want %v", err, ErrNoReady)
	}
	waitingTasks := []WaitingTask{{TaskID: "task", Reason: "waiting_dependency:dependency"}}
	if _, err := selectProjection(graphResult{waiting: waitingTasks}); !errors.Is(err, ErrWaiting) {
		t.Fatalf("waiting selection error = %v, want %v", err, ErrWaiting)
	} else {
		var waiting *WaitingError
		if !errors.As(err, &waiting) || !slices.Equal(waiting.Tasks, waitingTasks) {
			t.Fatalf("waiting detail = %#v", waiting)
		}
	}
	diagnostics := []Diagnostic{diagnostic("missing_dependency", "task", "dependency", "missing")}
	if _, err := selectProjection(graphResult{diagnostics: diagnostics}); !errors.Is(err, ErrUnsafeGraph) {
		t.Fatalf("unsafe selection error = %v, want %v", err, ErrUnsafeGraph)
	}
}

func TestGraphRejectsEveryInvalidShape(t *testing.T) {
	base := graphFixture("task", "pending", 1, time.Now().UTC())
	target := graphFixture("target", "completed", 1, base.createdAt)

	tests := []struct {
		name string
		code string
		make func() []graphTask
	}{
		{"duplicate task", "duplicate_task", func() []graphTask { return []graphTask{base, base} }},
		{"invalid accepted version", "invalid_accepted_version", func() []graphTask {
			bad := base
			bad.acceptedVersionID = "other-version"
			return []graphTask{bad}
		}},
		{"invalid status", "invalid_status", func() []graphTask {
			bad := base
			bad.status = "invented"
			return []graphTask{bad}
		}},
		{"active task without run", "active_task_without_run", func() []graphTask {
			bad := base
			bad.status = "admitted"
			return []graphTask{bad}
		}},
		{"ambiguous task identity", "ambiguous_task_identity", func() []graphTask {
			bad := target
			bad.externalTaskID = base.externalTaskID
			return []graphTask{base, bad}
		}},
		{"ambiguous project source", "ambiguous_project_source", func() []graphTask {
			bad := base
			bad.projectSources = append(bad.projectSources, projectSource{id: "source-2"})
			return []graphTask{bad}
		}},
		{"duplicate dependency", "duplicate_dependency", func() []graphTask {
			bad := base
			bad.dependencies = []graphEdge{{bad.taskVersionID, target.taskID}, {bad.taskVersionID, target.taskID}}
			return []graphTask{bad, target}
		}},
		{"self dependency", "self_dependency", func() []graphTask {
			bad := base
			bad.dependencies = []graphEdge{{bad.taskVersionID, bad.taskID}}
			return []graphTask{bad}
		}},
		{"missing dependency", "missing_dependency", func() []graphTask {
			bad := base
			bad.dependencies = []graphEdge{{bad.taskVersionID, "missing"}}
			return []graphTask{bad}
		}},
		{"stale dependency", "stale_dependency_edge", func() []graphTask {
			bad := base
			bad.dependencies = []graphEdge{{"old-version", target.taskID}}
			return []graphTask{bad, target}
		}},
		{"dependency cycle", "dependency_cycle", func() []graphTask {
			left, right := base, target
			left.dependencies = []graphEdge{{left.taskVersionID, right.taskID}}
			right.dependencies = []graphEdge{{right.taskVersionID, left.taskID}}
			return []graphTask{left, right}
		}},
		{"duplicate conflict", "duplicate_conflict", func() []graphTask {
			bad := base
			bad.conflicts = []graphEdge{{bad.taskVersionID, target.taskID}, {bad.taskVersionID, target.taskID}}
			return []graphTask{bad, target}
		}},
		{"self conflict", "self_conflict", func() []graphTask {
			bad := base
			bad.conflicts = []graphEdge{{bad.taskVersionID, bad.taskID}}
			return []graphTask{bad}
		}},
		{"missing conflict", "missing_conflict", func() []graphTask {
			bad := base
			bad.conflicts = []graphEdge{{bad.taskVersionID, "missing"}}
			return []graphTask{bad}
		}},
		{"stale conflict", "stale_conflict_edge", func() []graphTask {
			bad := base
			bad.conflicts = []graphEdge{{"old-version", target.taskID}}
			return []graphTask{bad, target}
		}},
		{"ambiguous edge", "ambiguous_edge", func() []graphTask {
			bad := base
			bad.dependencies = []graphEdge{{bad.taskVersionID, target.taskID}}
			bad.conflicts = []graphEdge{{bad.taskVersionID, target.taskID}}
			return []graphTask{bad, target}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateGraph(tt.make())
			if result.candidate != nil {
				t.Fatalf("invalid graph selected %#v", result.candidate)
			}
			if !hasDiagnostic(result.diagnostics, tt.code) {
				t.Fatalf("diagnostics = %#v, want code %q", result.diagnostics, tt.code)
			}
		})
	}
}

func graphFixture(taskID, status string, priority int32, createdAt time.Time) graphTask {
	return graphTask{
		projectID: "project", projectStatus: "registered",
		projectSources: []projectSource{{id: "source", commit: "commit", tree: "tree"}},
		taskID:         taskID, externalTaskID: taskID, status: status,
		acceptedVersionID: taskID + "-version", taskVersionID: taskID + "-version",
		aggregateVersion: 4, priority: priority, createdAt: createdAt,
	}
}

func hasDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
