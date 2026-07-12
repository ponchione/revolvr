// Package autonomousscheduler owns pure dependency graph validation,
// readiness classification, and deterministic selection. It never runs a
// task and never turns the pinned task runner into a queue.
package autonomousscheduler

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"revolvr/internal/taskfile"
)

type Reason string

const (
	ReasonReady             Reason = "ready"
	ReasonNotPending        Reason = "not_pending"
	ReasonWrongWorkflow     Reason = "wrong_workflow"
	ReasonWaitingDependency Reason = "waiting_dependency"
	ReasonBlockedDependency Reason = "blocked_dependency"
	ReasonNeedsInput        Reason = "needs_input_dependency"
	ReasonConflict          Reason = "conflict"
)

type ActiveTask struct {
	Task          taskfile.Task
	Lifecycle     string
	StateSHA256   string
	StateByteSize int
}

// ArchiveEvidence is supplied only after an archive owner has verified and
// reconciled the exact immutable AW-21 identity. The pure graph does not read
// live archive or Git state.
type ArchiveEvidence struct {
	TaskID, ArchiveID string
	Disposition       string
	Verified          bool
	Reconciled        bool
}

type Node struct {
	Task          taskfile.Task
	Lifecycle     string
	StateSHA256   string
	StateByteSize int
	Reason        Reason
	WaitingOn     []string
	Conflicts     []string
}

type Graph struct {
	Nodes    []Node
	archives map[string]ArchiveEvidence
	byID     map[string]int
}

type Selection struct {
	Task      taskfile.Task
	Found     bool
	Reason    Reason
	WaitingOn []string
	Conflicts []string
}

func BuildSnapshot(active []ActiveTask, archives []ArchiveEvidence) (Graph, error) {
	items := append([]ActiveTask(nil), active...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Task.SourcePath != items[j].Task.SourcePath {
			return items[i].Task.SourcePath < items[j].Task.SourcePath
		}
		return items[i].Task.ID < items[j].Task.ID
	})
	g := Graph{Nodes: make([]Node, len(items)), byID: make(map[string]int, len(items)), archives: make(map[string]ArchiveEvidence, len(archives))}
	for i, item := range items {
		if item.Task.ID == "" {
			return Graph{}, errors.New("scheduler: active task has empty identity")
		}
		if previous, ok := g.byID[item.Task.ID]; ok {
			return Graph{}, fmt.Errorf("scheduler: duplicate active task id %q at %s and %s", item.Task.ID, items[previous].Task.SourcePath, item.Task.SourcePath)
		}
		g.byID[item.Task.ID] = i
		g.Nodes[i] = Node{Task: item.Task, Lifecycle: item.Lifecycle, StateSHA256: item.StateSHA256, StateByteSize: item.StateByteSize}
	}
	orderedArchives := append([]ArchiveEvidence(nil), archives...)
	sort.Slice(orderedArchives, func(i, j int) bool { return orderedArchives[i].TaskID < orderedArchives[j].TaskID })
	for _, archive := range orderedArchives {
		if archive.TaskID == "" || archive.ArchiveID == "" || !archive.Verified || !archive.Reconciled {
			return Graph{}, fmt.Errorf("scheduler: archive evidence for %q is not exact verified and reconciled authority", archive.TaskID)
		}
		if _, ok := g.archives[archive.TaskID]; ok {
			return Graph{}, fmt.Errorf("scheduler: duplicate archived task id %q", archive.TaskID)
		}
		if _, ok := g.byID[archive.TaskID]; ok {
			return Graph{}, fmt.Errorf("scheduler: task id %q is ambiguous between active and archived identities", archive.TaskID)
		}
		g.archives[archive.TaskID] = archive
	}
	for i := range g.Nodes {
		for _, dependency := range g.Nodes[i].Task.DependsOn {
			if _, activeOK := g.byID[dependency]; activeOK {
				continue
			}
			if _, archiveOK := g.archives[dependency]; archiveOK {
				continue
			}
			return Graph{}, fmt.Errorf("scheduler: task %q has missing dependency %q", g.Nodes[i].Task.ID, dependency)
		}
	}
	if cycle := findCycle(g); len(cycle) != 0 {
		return Graph{}, fmt.Errorf("scheduler: dependency cycle: %s", strings.Join(cycle, " -> "))
	}
	return g, nil
}

func SelectNextReady(graph Graph, occupied []string) Selection {
	occupiedSet := make(map[string]struct{}, len(occupied))
	for _, id := range occupied {
		occupiedSet[id] = struct{}{}
	}
	nodes := make([]Node, len(graph.Nodes))
	for i := range graph.Nodes {
		nodes[i] = classify(graph, i, occupiedSet)
	}
	sort.Slice(nodes, func(i, j int) bool { return taskBefore(nodes[i].Task, nodes[j].Task) })
	for _, node := range nodes {
		if node.Reason == ReasonReady {
			return Selection{Task: node.Task, Found: true, Reason: node.Reason}
		}
	}
	for _, reason := range []Reason{ReasonNeedsInput, ReasonBlockedDependency, ReasonWaitingDependency, ReasonConflict, ReasonNotPending, ReasonWrongWorkflow} {
		for _, node := range nodes {
			if node.Reason == reason {
				return Selection{Reason: reason, WaitingOn: append([]string(nil), node.WaitingOn...), Conflicts: append([]string(nil), node.Conflicts...)}
			}
		}
	}
	return Selection{}
}

// ClassifyAll returns every node in the same deterministic order used by
// selection. Queue-local exclusion and fairness policy can consume this pure
// projection without moving that policy into the scheduler.
func ClassifyAll(graph Graph, occupied []string) []Node {
	occupiedSet := make(map[string]struct{}, len(occupied))
	for _, id := range occupied {
		occupiedSet[id] = struct{}{}
	}
	nodes := make([]Node, len(graph.Nodes))
	for i := range graph.Nodes {
		nodes[i] = classify(graph, i, occupiedSet)
	}
	sort.Slice(nodes, func(i, j int) bool { return taskBefore(nodes[i].Task, nodes[j].Task) })
	return nodes
}

func ClassifyTask(graph Graph, taskID string, occupied []string) (Node, error) {
	i, ok := graph.byID[taskID]
	if !ok {
		return Node{}, fmt.Errorf("scheduler: active task %q not found", taskID)
	}
	set := make(map[string]struct{}, len(occupied))
	for _, id := range occupied {
		set[id] = struct{}{}
	}
	return classify(graph, i, set), nil
}

func classify(g Graph, index int, occupied map[string]struct{}) Node {
	node := g.Nodes[index]
	if node.Task.Workflow != taskfile.WorkflowAutonomousV1 {
		node.Reason = ReasonWrongWorkflow
		return node
	}
	if node.Task.Status != taskfile.StatusPending {
		node.Reason = ReasonNotPending
		return node
	}
	if node.Lifecycle == "needs_input" {
		node.Reason = ReasonNeedsInput
		return node
	}
	if node.Lifecycle == taskfile.StatusBlocked {
		node.Reason = ReasonBlockedDependency
		return node
	}
	for _, dependency := range node.Task.DependsOn {
		if archive, ok := g.archives[dependency]; ok {
			if archive.Disposition != taskfile.StatusCompleted {
				node.WaitingOn = append(node.WaitingOn, dependency)
			}
			continue
		}
		dep := g.Nodes[g.byID[dependency]]
		if dep.Task.Status == taskfile.StatusCompleted || dep.Lifecycle == taskfile.StatusCompleted {
			continue
		}
		node.WaitingOn = append(node.WaitingOn, dependency)
		switch {
		case dep.Lifecycle == "needs_input":
			node.Reason = ReasonNeedsInput
		case dep.Task.Status == taskfile.StatusBlocked || dep.Lifecycle == taskfile.StatusBlocked:
			if node.Reason != ReasonNeedsInput {
				node.Reason = ReasonBlockedDependency
			}
		default:
			if node.Reason == "" {
				node.Reason = ReasonWaitingDependency
			}
		}
	}
	if len(node.WaitingOn) != 0 {
		sort.Strings(node.WaitingOn)
		return node
	}
	for occupiedID := range occupied {
		if conflicts(g, node.Task, occupiedID) {
			node.Conflicts = append(node.Conflicts, occupiedID)
		}
	}
	if len(node.Conflicts) != 0 {
		sort.Strings(node.Conflicts)
		node.Reason = ReasonConflict
		return node
	}
	node.Reason = ReasonReady
	return node
}

func conflicts(g Graph, task taskfile.Task, occupiedID string) bool {
	if occupiedID == task.ID {
		return true
	}
	for _, token := range task.Conflicts {
		if token == occupiedID {
			return true
		}
	}
	i, ok := g.byID[occupiedID]
	if !ok {
		return false
	}
	other := g.Nodes[i].Task
	for _, token := range other.Conflicts {
		if token == task.ID {
			return true
		}
	}
	for _, left := range task.Conflicts {
		for _, right := range other.Conflicts {
			if left == right {
				return true
			}
		}
	}
	return false
}

func taskBefore(left, right taskfile.Task) bool {
	if left.HasPriority && right.HasPriority && left.Priority != right.Priority {
		return left.Priority < right.Priority
	}
	if left.HasPriority != right.HasPriority {
		return left.HasPriority
	}
	if left.SourcePath != right.SourcePath {
		return filepath.ToSlash(left.SourcePath) < filepath.ToSlash(right.SourcePath)
	}
	return left.ID < right.ID
}

func findCycle(g Graph) []string {
	state := make([]uint8, len(g.Nodes))
	stack := make([]int, 0, len(g.Nodes))
	var visit func(int) []string
	visit = func(i int) []string {
		state[i] = 1
		stack = append(stack, i)
		deps := append([]string(nil), g.Nodes[i].Task.DependsOn...)
		sort.Strings(deps)
		for _, id := range deps {
			j, active := g.byID[id]
			if !active {
				continue
			}
			if state[j] == 0 {
				if cycle := visit(j); len(cycle) != 0 {
					return cycle
				}
			}
			if state[j] == 1 {
				start := 0
				for stack[start] != j {
					start++
				}
				cycle := make([]string, 0, len(stack)-start+1)
				for _, n := range stack[start:] {
					cycle = append(cycle, g.Nodes[n].Task.ID)
				}
				return append(cycle, g.Nodes[j].Task.ID)
			}
		}
		stack = stack[:len(stack)-1]
		state[i] = 2
		return nil
	}
	for i := range g.Nodes {
		if state[i] == 0 {
			if c := visit(i); len(c) != 0 {
				return c
			}
		}
	}
	return nil
}
