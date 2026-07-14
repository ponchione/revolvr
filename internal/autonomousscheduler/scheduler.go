// Package autonomousscheduler adapts strict autonomous repository authority
// to the shared workflow-aware taskscheduler. It owns no graph, ordering,
// dependency, cycle, or conflict policy.
package autonomousscheduler

import (
	"fmt"
	"path/filepath"
	"strings"

	"revolvr/internal/taskfile"
	"revolvr/internal/taskscheduler"
)

// ArchiveEvidence is supplied only after an archive owner has inspected the
// exact immutable AW-21 identity. taskscheduler decides whether that evidence
// is valid and whether its disposition satisfies an edge.
type ArchiveEvidence struct {
	TaskID      string
	ArchiveID   string
	Disposition string
	Reason      string
	Verified    bool
	Reconciled  bool
}

type Node struct {
	Task          taskfile.Task
	Lifecycle     string
	StateSHA256   string
	StateByteSize int
	Reason        taskscheduler.Reason
	WaitingOn     []string
	Issues        []taskscheduler.DependencyIssue
	Conflicts     []string
	SelectedNext  bool
}

type Graph struct {
	Nodes    []Node
	active   []ActiveTask
	archives []ArchiveEvidence
}

type Selection struct {
	Task      taskfile.Task
	Found     bool
	Reason    taskscheduler.Reason
	WaitingOn []string
	Issues    []taskscheduler.DependencyIssue
	Conflicts []string
}

type GraphError struct {
	Diagnostics []taskscheduler.Diagnostic
}

func (e GraphError) Error() string {
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, string(diagnostic.Code)+": "+diagnostic.Detail)
	}
	return "scheduler: invalid graph: " + strings.Join(parts, "; ")
}

// BuildResult exposes the complete shared result for autonomous read surfaces,
// including invalid-graph diagnostics. It never hides diagnostics behind a
// first-error graph builder.
func BuildResult(active []ActiveTask, archives []ArchiveEvidence, occupied []string) taskscheduler.Result {
	input := taskscheduler.Input{
		Tasks:             make([]taskscheduler.Task, 0, len(active)),
		Archives:          make([]taskscheduler.Archive, 0, len(archives)),
		Occupied:          append([]string(nil), occupied...),
		SelectionWorkflow: taskscheduler.WorkflowAutonomousV1,
	}
	for _, item := range active {
		state := taskscheduler.State(item.Task.Status)
		if item.Task.Workflow == taskfile.WorkflowAutonomousV1 && item.Lifecycle != "" {
			state = taskscheduler.State(item.Lifecycle)
		}
		input.Tasks = append(input.Tasks, taskscheduler.Task{
			ID:          item.Task.ID,
			Workflow:    taskscheduler.Workflow(item.Task.Workflow),
			State:       state,
			SourcePath:  item.Task.SourcePath,
			Priority:    item.Task.Priority,
			HasPriority: item.Task.HasPriority,
			DependsOn:   append([]string(nil), item.Task.DependsOn...),
			Conflicts:   append([]string(nil), item.Task.Conflicts...),
		})
	}
	for _, archive := range archives {
		input.Archives = append(input.Archives, taskscheduler.Archive{
			TaskID:      archive.TaskID,
			ArchiveID:   archive.ArchiveID,
			Disposition: taskscheduler.State(archive.Disposition),
			Reason:      archive.Reason,
			Verified:    archive.Verified,
			Reconciled:  archive.Reconciled,
		})
	}
	return taskscheduler.Evaluate(input)
}

// BuildSnapshot is the fail-closed autonomous execution/queue admission
// boundary. The pure result remains available through BuildResult for views.
func BuildSnapshot(active []ActiveTask, archives []ArchiveEvidence) (Graph, error) {
	active = cloneActive(active)
	archives = append([]ArchiveEvidence(nil), archives...)
	result := BuildResult(active, archives, nil)
	if !result.Valid() {
		return Graph{}, GraphError{Diagnostics: cloneDiagnostics(result.InvalidGraph)}
	}
	graph := Graph{active: active, archives: archives}
	graph.Nodes = nodesFromResult(active, result)
	return graph, nil
}

// Schedule re-evaluates one admitted immutable graph against queue-local
// occupancy. All policy remains inside taskscheduler.
func Schedule(graph Graph, occupied []string) taskscheduler.Result {
	return BuildResult(graph.active, graph.archives, occupied)
}

func SelectNextReady(graph Graph, occupied []string) Selection {
	result := Schedule(graph, occupied)
	if selected, found := result.SelectedForWorkflow(taskscheduler.WorkflowAutonomousV1); found {
		node, ok := nodeForReadiness(graph.active, selected)
		if ok {
			return Selection{Task: node.Task, Found: true, Reason: selected.Reason}
		}
	}
	for _, node := range nodesFromResult(graph.active, result) {
		if node.Task.Workflow == taskfile.WorkflowAutonomousV1 {
			return Selection{Reason: node.Reason, WaitingOn: append([]string(nil), node.WaitingOn...), Issues: append([]taskscheduler.DependencyIssue(nil), node.Issues...), Conflicts: append([]string(nil), node.Conflicts...)}
		}
	}
	return Selection{}
}

// ClassifyAll returns ready nodes first in the shared ready ordering, followed
// by non-ready nodes in the shared deterministic diagnostic order.
func ClassifyAll(graph Graph, occupied []string) []Node {
	return nodesFromResult(graph.active, Schedule(graph, occupied))
}

func ClassifyTask(graph Graph, taskID string, occupied []string) (Node, error) {
	result := Schedule(graph, occupied)
	for _, readiness := range result.Tasks {
		if readiness.TaskID != taskID {
			continue
		}
		node, ok := nodeForReadiness(graph.active, readiness)
		if !ok {
			break
		}
		if node.Task.Workflow != taskfile.WorkflowAutonomousV1 {
			return Node{}, fmt.Errorf("scheduler: active task %q uses workflow %q, not autonomous-v1", taskID, node.Task.Workflow)
		}
		node.SelectedNext = result.SelectedNext != nil && sameReadiness(*result.SelectedNext, readiness)
		return node, nil
	}
	return Node{}, fmt.Errorf("scheduler: active task %q not found", taskID)
}

func nodesFromResult(active []ActiveTask, result taskscheduler.Result) []Node {
	ordered := make([]taskscheduler.TaskReadiness, 0, len(result.Tasks))
	seen := make(map[string]struct{}, len(result.Ready))
	for _, readiness := range result.Ready {
		ordered = append(ordered, readiness)
		seen[readinessKey(readiness)] = struct{}{}
	}
	for _, readiness := range result.Tasks {
		if _, ok := seen[readinessKey(readiness)]; !ok {
			ordered = append(ordered, readiness)
		}
	}
	nodes := make([]Node, 0, len(ordered))
	for _, readiness := range ordered {
		node, ok := nodeForReadiness(active, readiness)
		if !ok {
			continue
		}
		node.SelectedNext = result.SelectedNext != nil && sameReadiness(*result.SelectedNext, readiness)
		nodes = append(nodes, node)
	}
	return nodes
}

func nodeForReadiness(active []ActiveTask, readiness taskscheduler.TaskReadiness) (Node, bool) {
	for _, item := range active {
		if item.Task.ID != readiness.TaskID || filepath.ToSlash(item.Task.SourcePath) != readiness.SourcePath {
			continue
		}
		return Node{
			Task:          item.Task,
			Lifecycle:     item.Lifecycle,
			StateSHA256:   item.StateSHA256,
			StateByteSize: item.StateByteSize,
			Reason:        readiness.Reason,
			WaitingOn:     append([]string(nil), readiness.UnmetDependencyIDs...),
			Issues:        append([]taskscheduler.DependencyIssue(nil), readiness.DependencyIssues...),
			Conflicts:     append([]string(nil), readiness.ConflictingTaskOrKeys...),
		}, true
	}
	return Node{}, false
}

func readinessKey(readiness taskscheduler.TaskReadiness) string {
	return readiness.TaskID + "\x00" + readiness.SourcePath
}

func sameReadiness(left, right taskscheduler.TaskReadiness) bool {
	return left.TaskID == right.TaskID && left.SourcePath == right.SourcePath
}

func cloneActive(input []ActiveTask) []ActiveTask {
	result := make([]ActiveTask, len(input))
	for i, item := range input {
		item.Task.SourceBytes = append([]byte(nil), item.Task.SourceBytes...)
		item.Task.DependsOn = append([]string(nil), item.Task.DependsOn...)
		item.Task.Tags = append([]string(nil), item.Task.Tags...)
		item.Task.Conflicts = append([]string(nil), item.Task.Conflicts...)
		result[i] = item
	}
	return result
}

func cloneDiagnostics(input []taskscheduler.Diagnostic) []taskscheduler.Diagnostic {
	result := make([]taskscheduler.Diagnostic, len(input))
	for i, diagnostic := range input {
		diagnostic.Cycle = append([]string(nil), diagnostic.Cycle...)
		result[i] = diagnostic
	}
	return result
}
