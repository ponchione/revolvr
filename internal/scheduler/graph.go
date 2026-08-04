package scheduler

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Diagnostic struct {
	Code          string
	TaskID        string
	RelatedTaskID string
	Cycle         []string
	Detail        string
}

type WaitingTask struct {
	TaskID string
	Reason string
}

type graphTask struct {
	projectID                  string
	projectStatus              string
	projectSources             []projectSource
	taskID                     string
	externalTaskID             string
	status                     string
	acceptedVersionID          string
	taskVersionID              string
	aggregateVersion           int64
	priority                   int32
	createdAt                  time.Time
	awaitingOperatorCheckpoint bool
	dependencies               []graphEdge
	conflicts                  []graphEdge
}

type graphEdge struct {
	versionID string
	targetID  string
}

type projectSource struct {
	id     string
	commit string
	tree   string
}

type graphResult struct {
	candidate   *Candidate
	waiting     []WaitingTask
	diagnostics []Diagnostic
}

func evaluateGraph(tasks []graphTask) graphResult {
	tasks = append([]graphTask(nil), tasks...)
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].priority != tasks[j].priority {
			return tasks[i].priority < tasks[j].priority
		}
		if !tasks[i].createdAt.Equal(tasks[j].createdAt) {
			return tasks[i].createdAt.Before(tasks[j].createdAt)
		}
		return tasks[i].taskID < tasks[j].taskID
	})

	byID := make(map[string]graphTask, len(tasks))
	byExternalID := make(map[string]string, len(tasks))
	diagnostics := make([]Diagnostic, 0)
	for _, task := range tasks {
		if previous, ok := byID[task.taskID]; ok {
			diagnostics = append(diagnostics, diagnostic("duplicate_task", task.taskID, "", fmt.Sprintf("task identity is duplicated with project %s", previous.projectID)))
		} else {
			byID[task.taskID] = task
		}
		externalKey := task.projectID + "\x00" + task.externalTaskID
		if previous, ok := byExternalID[externalKey]; ok {
			diagnostics = append(diagnostics, diagnostic("ambiguous_task_identity", task.taskID, previous, "project has duplicate external task identity"))
		} else {
			byExternalID[externalKey] = task.taskID
		}
		if !validTaskStatus(task.status) {
			diagnostics = append(diagnostics, diagnostic("invalid_status", task.taskID, "", fmt.Sprintf("task has unsupported status %q", task.status)))
		}
		if activeTaskStatus(task.status) {
			diagnostics = append(diagnostics, diagnostic("active_task_without_run", task.taskID, "", "active task has no authoritative run and global lease"))
		}
		if approvedTaskStatus(task.status) && (task.acceptedVersionID == "" || task.acceptedVersionID != task.taskVersionID) {
			diagnostics = append(diagnostics, diagnostic("invalid_accepted_version", task.taskID, "", "approved task does not have one exact accepted version"))
		}
		if oneOf(task.status, "draft", "compiled", "awaiting_approval") && task.acceptedVersionID != "" {
			diagnostics = append(diagnostics, diagnostic("invalid_accepted_version", task.taskID, "", "unapproved task has an accepted version"))
		}
		if len(task.projectSources) > 1 {
			diagnostics = append(diagnostics, diagnostic("ambiguous_project_source", task.taskID, "", fmt.Sprintf("project %s has %d canonical sources", task.projectID, len(task.projectSources))))
		}
	}

	for _, task := range tasks {
		diagnostics = append(diagnostics, validateEdges(task, task.dependencies, "dependency", byID)...)
		diagnostics = append(diagnostics, validateEdges(task, task.conflicts, "conflict", byID)...)
		dependencies := make(map[string]bool, len(task.dependencies))
		for _, edge := range task.dependencies {
			dependencies[edge.targetID] = true
		}
		for _, edge := range task.conflicts {
			if dependencies[edge.targetID] {
				diagnostics = append(diagnostics, diagnostic("ambiguous_edge", task.taskID, edge.targetID, "task target is both a dependency and a conflict"))
			}
		}
	}
	for _, cycle := range dependencyCycles(byID) {
		diagnostics = append(diagnostics, Diagnostic{Code: "dependency_cycle", TaskID: cycle[0], Cycle: cycle, Detail: "dependency cycle: " + strings.Join(cycle, " -> ")})
	}
	sortDiagnostics(diagnostics)
	if len(diagnostics) != 0 {
		return graphResult{diagnostics: diagnostics}
	}

	result := graphResult{}
	for _, task := range tasks {
		if task.status != "pending" {
			continue
		}
		if reason := waitingReason(task, byID); reason != "" {
			result.waiting = append(result.waiting, WaitingTask{TaskID: task.taskID, Reason: reason})
			continue
		}
		source := task.projectSources[0]
		result.candidate = &Candidate{
			ProjectID: task.projectID, ProjectSourceID: source.id,
			TaskID: task.taskID, TaskVersionID: task.taskVersionID,
			ExternalTaskID: task.externalTaskID, ExpectedAggregateVersion: task.aggregateVersion,
			Priority: task.priority, CreatedAt: task.createdAt,
			SourceCommit: source.commit, SourceTree: source.tree,
		}
		break
	}
	return result
}

func validateEdges(task graphTask, edges []graphEdge, kind string, byID map[string]graphTask) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	seen := make(map[string]bool, len(edges))
	for _, edge := range edges {
		switch {
		case edge.versionID != task.taskVersionID:
			diagnostics = append(diagnostics, diagnostic("stale_"+kind+"_edge", task.taskID, edge.targetID, kind+" does not belong to the accepted task version"))
		case seen[edge.targetID]:
			diagnostics = append(diagnostics, diagnostic("duplicate_"+kind, task.taskID, edge.targetID, kind+" edge is duplicated"))
		case edge.targetID == task.taskID:
			diagnostics = append(diagnostics, diagnostic("self_"+kind, task.taskID, edge.targetID, "task references itself as a "+kind))
		default:
			if _, ok := byID[edge.targetID]; !ok {
				diagnostics = append(diagnostics, diagnostic("missing_"+kind, task.taskID, edge.targetID, kind+" target does not exist"))
			}
		}
		seen[edge.targetID] = true
	}
	return diagnostics
}

func waitingReason(task graphTask, byID map[string]graphTask) string {
	switch {
	case task.projectStatus != "registered":
		return "project_not_healthy"
	case len(task.projectSources) == 0:
		return "project_source_missing"
	case task.awaitingOperatorCheckpoint:
		return "awaiting_operator_checkpoint"
	}
	for _, edge := range task.dependencies {
		dependency := byID[edge.targetID]
		if dependency.status == "completed" {
			continue
		}
		if terminalUnsatisfied(dependency.status) {
			return "terminal_unsatisfied_dependency:" + dependency.taskID
		}
		return "waiting_dependency:" + dependency.taskID
	}
	for _, edge := range task.conflicts {
		conflict := byID[edge.targetID]
		if conflict.status != "pending" && !oneOf(conflict.status, "cancelled", "abandoned", "superseded") {
			return "conflict:" + conflict.taskID
		}
	}
	return ""
}

func terminalUnsatisfied(status string) bool {
	return oneOf(status, "cancelled", "abandoned", "superseded", "budget_exhausted", "unsafe")
}

func validTaskStatus(status string) bool {
	return oneOf(status,
		"draft", "compiled", "awaiting_approval", "pending", "admitted", "planning", "ready",
		"working", "verifying", "auditing", "correcting", "documenting", "simplifying",
		"needs_input", "blocked", "finalizing", "completed", "cancelled", "budget_exhausted",
		"unsafe", "superseded", "abandoned", "retrieval", "telemetry")
}

func approvedTaskStatus(status string) bool {
	return oneOf(status,
		"pending", "admitted", "planning", "ready", "working", "verifying", "auditing",
		"correcting", "documenting", "simplifying", "finalizing", "completed")
}

func dependencyCycles(tasks map[string]graphTask) [][]string {
	state := make(map[string]uint8, len(tasks))
	stack := make([]string, 0, len(tasks))
	cycles := make([][]string, 0)
	seen := make(map[string]bool)
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var visit func(string)
	visit = func(id string) {
		state[id] = 1
		stack = append(stack, id)
		targets := make([]string, 0, len(tasks[id].dependencies))
		for _, edge := range tasks[id].dependencies {
			if _, ok := tasks[edge.targetID]; ok && edge.targetID != id {
				targets = append(targets, edge.targetID)
			}
		}
		sort.Strings(targets)
		for _, target := range targets {
			switch state[target] {
			case 0:
				visit(target)
			case 1:
				start := 0
				for stack[start] != target {
					start++
				}
				cycle := append(append([]string(nil), stack[start:]...), target)
				key := strings.Join(cycle, "\x00")
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
	}
	for _, id := range ids {
		if state[id] == 0 {
			visit(id)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return strings.Join(cycles[i], "\x00") < strings.Join(cycles[j], "\x00") })
	return cycles
}

func diagnostic(code, taskID, relatedTaskID, detail string) Diagnostic {
	return Diagnostic{Code: code, TaskID: taskID, RelatedTaskID: relatedTaskID, Cycle: []string{}, Detail: detail}
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left := strings.Join([]string{diagnostics[i].Code, diagnostics[i].TaskID, diagnostics[i].RelatedTaskID, diagnostics[i].Detail}, "\x00")
		right := strings.Join([]string{diagnostics[j].Code, diagnostics[j].TaskID, diagnostics[j].RelatedTaskID, diagnostics[j].Detail}, "\x00")
		return left < right
	})
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
