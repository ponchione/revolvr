// Package taskscheduler owns pure, workflow-aware task graph validation,
// readiness classification, and deterministic selection. It performs no I/O
// and never runs or mutates a task.
package taskscheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
)

const ResultSchemaVersion = "revolvr-task-schedule-v1"

type Workflow string

const (
	WorkflowMixedPassV1          Workflow = "mixed-pass-v1"
	WorkflowAutonomousV1         Workflow = "autonomous-v1"
	WorkflowOperatorCheckpointV1 Workflow = "operator-checkpoint-v1"
)

type State string

const (
	StatePending          State = "pending"
	StateReady            State = "ready"
	StateRunning          State = "running"
	StatePlanning         State = "planning"
	StateWorking          State = "working"
	StateVerifying        State = "verifying"
	StateAuditing         State = "auditing"
	StateCorrecting       State = "correcting"
	StateNeedsInput       State = "needs_input"
	StateAwaitingOperator State = "awaiting_operator"
	StateFinalizing       State = "finalizing"
	StateCompleted        State = "completed"
	StateBlocked          State = "blocked"
	StateCancelled        State = "cancelled"
	StateAbandoned        State = "abandoned"
	StateSuperseded       State = "superseded"
)

type Reason string

const (
	ReasonReady                         Reason = "ready"
	ReasonWaitingDependency             Reason = "waiting_dependency"
	ReasonBlockedDependency             Reason = "blocked_dependency"
	ReasonNeedsInputDependency          Reason = "needs_input_dependency"
	ReasonAwaitingOperatorDependency    Reason = "awaiting_operator_dependency"
	ReasonTerminalUnsatisfiedDependency Reason = "terminal_unsatisfied_dependency"
	ReasonAwaitingOperator              Reason = "awaiting_operator"
	ReasonNeedsInput                    Reason = "needs_input"
	ReasonConflictBlocked               Reason = "conflict_blocked"
	ReasonRunning                       Reason = "running"
	ReasonCompleted                     Reason = "completed"
	ReasonBlocked                       Reason = "blocked"
	ReasonCancelled                     Reason = "cancelled"
	ReasonAbandoned                     Reason = "abandoned"
	ReasonSuperseded                    Reason = "superseded"
	ReasonInvalidGraph                  Reason = "invalid_graph"
)

type DiagnosticCode string

const (
	DiagnosticInvalidTask             DiagnosticCode = "invalid_task"
	DiagnosticInvalidSelection        DiagnosticCode = "invalid_selection"
	DiagnosticDuplicateTaskID         DiagnosticCode = "duplicate_task_id"
	DiagnosticDuplicateDependencyEdge DiagnosticCode = "duplicate_dependency_edge"
	DiagnosticSelfDependency          DiagnosticCode = "self_dependency"
	DiagnosticMissingDependency       DiagnosticCode = "missing_dependency"
	DiagnosticActiveArchiveAmbiguity  DiagnosticCode = "active_archive_ambiguity"
	DiagnosticMalformedArchive        DiagnosticCode = "malformed_archive_authority"
	DiagnosticDependencyCycle         DiagnosticCode = "dependency_cycle"
)

// Task is the normalized scheduling input supplied by canonical task and
// workflow-state owners. State is the effective state for dependency policy;
// adapters are responsible for deriving it from their already-validated
// canonical evidence.
type Task struct {
	ID          string               `json:"id"`
	Workflow    Workflow             `json:"workflow"`
	State       State                `json:"state"`
	SourcePath  string               `json:"source_path"`
	Priority    int                  `json:"priority"`
	HasPriority bool                 `json:"has_priority"`
	DependsOn   []string             `json:"depends_on"`
	Conflicts   []string             `json:"conflicts"`
	Checkpoint  *CheckpointAuthority `json:"checkpoint,omitempty"`
}

// CheckpointAuthority is validated receipt evidence supplied by the
// canonical repository adapter. A completed checkpoint is dependency
// authority only when its exact bound receipt identity has been verified.
type CheckpointAuthority struct {
	ReceiptPath   string `json:"receipt_path"`
	ReceiptSHA256 string `json:"receipt_sha256"`
	Verified      bool   `json:"verified"`
	Detail        string `json:"detail"`
}

// Archive is exact authority supplied by an archive verifier. All admitted
// archive evidence must be verified and reconciled; only completed archives
// satisfy dependency edges.
type Archive struct {
	TaskID      string `json:"task_id"`
	ArchiveID   string `json:"archive_id"`
	Disposition State  `json:"disposition"`
	Reason      string `json:"reason"`
	Verified    bool   `json:"verified"`
	Reconciled  bool   `json:"reconciled"`
}

type Input struct {
	Tasks             []Task
	Archives          []Archive
	Occupied          []string
	SelectionWorkflow Workflow
}

type Diagnostic struct {
	Code              DiagnosticCode `json:"code"`
	TaskID            string         `json:"task_id"`
	DependencyID      string         `json:"dependency_id"`
	SourcePath        string         `json:"source_path"`
	RelatedSourcePath string         `json:"related_source_path"`
	Cycle             []string       `json:"cycle"`
	Detail            string         `json:"detail"`
}

type DependencyIssue struct {
	DependencyID string   `json:"dependency_id"`
	Workflow     Workflow `json:"workflow"`
	State        State    `json:"state"`
	Reason       Reason   `json:"reason"`
	Archived     bool     `json:"archived"`
	ArchiveID    string   `json:"archive_id"`
	Detail       string   `json:"detail"`
}

type TaskReadiness struct {
	TaskID                string            `json:"task_id"`
	Workflow              Workflow          `json:"workflow"`
	State                 State             `json:"state"`
	SourcePath            string            `json:"source_path"`
	Priority              int               `json:"priority"`
	HasPriority           bool              `json:"has_priority"`
	Reason                Reason            `json:"reason"`
	UnmetDependencyIDs    []string          `json:"unmet_dependency_ids"`
	DependencyIssues      []DependencyIssue `json:"dependency_issues"`
	ConflictingTaskOrKeys []string          `json:"conflicting_task_or_keys"`
}

type WorkflowSelection struct {
	Workflow     Workflow       `json:"workflow"`
	SelectedNext *TaskReadiness `json:"selected_next"`
}

// Result is the complete deterministic scheduling projection. Tasks contains
// every active task; the category slices contain the same value projections
// needed by execution and read surfaces. A nonempty InvalidGraph always means
// SelectedNext is nil and every task is classified invalid_graph.
type Result struct {
	SchemaVersion                string              `json:"schema_version"`
	SelectionWorkflow            Workflow            `json:"selection_workflow"`
	Tasks                        []TaskReadiness     `json:"tasks"`
	Ready                        []TaskReadiness     `json:"ready"`
	Waiting                      []TaskReadiness     `json:"waiting"`
	DependencyBlocked            []TaskReadiness     `json:"dependency_blocked"`
	TerminalUnsatisfied          []TaskReadiness     `json:"terminal_unsatisfied"`
	SelectionTerminalUnsatisfied []TaskReadiness     `json:"selection_terminal_unsatisfied"`
	OperatorInput                []TaskReadiness     `json:"operator_input"`
	ConflictBlocked              []TaskReadiness     `json:"conflict_blocked"`
	Other                        []TaskReadiness     `json:"other"`
	InvalidGraph                 []Diagnostic        `json:"invalid_graph"`
	WorkflowSelections           []WorkflowSelection `json:"workflow_selections"`
	SelectedNext                 *TaskReadiness      `json:"selected_next"`
}

func (r Result) Valid() bool {
	return len(r.InvalidGraph) == 0
}

func (r Result) SelectedForWorkflow(workflow Workflow) (TaskReadiness, bool) {
	for _, selection := range r.WorkflowSelections {
		if selection.Workflow == workflow && selection.SelectedNext != nil {
			return cloneReadiness(*selection.SelectedNext), true
		}
	}
	return TaskReadiness{}, false
}

// Evaluate validates and classifies one immutable input snapshot. Input slice
// order never affects the result.
func Evaluate(input Input) Result {
	result := emptyResult(input.SelectionWorkflow)
	tasks := normalizeTasks(input.Tasks)
	archives := normalizeArchives(input.Archives)
	diagnostics, byID, archiveByID := validate(tasks, archives)
	if input.SelectionWorkflow != "" && input.SelectionWorkflow != WorkflowMixedPassV1 && input.SelectionWorkflow != WorkflowAutonomousV1 {
		diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticInvalidSelection, Cycle: []string{}, Detail: fmt.Sprintf("selection workflow %q is not executable", input.SelectionWorkflow)})
		sortDiagnostics(diagnostics)
	}
	if len(diagnostics) != 0 {
		result.InvalidGraph = diagnostics
		for _, task := range tasks {
			result.Tasks = append(result.Tasks, newReadiness(task, ReasonInvalidGraph))
		}
		return result
	}

	occupied := normalizedUnique(input.Occupied)
	for _, task := range tasks {
		item := classify(task, byID, archiveByID, occupied)
		result.Tasks = append(result.Tasks, item)
		switch item.Reason {
		case ReasonReady:
			result.Ready = append(result.Ready, item)
		case ReasonWaitingDependency:
			result.Waiting = append(result.Waiting, item)
		case ReasonBlockedDependency, ReasonNeedsInputDependency:
			result.DependencyBlocked = append(result.DependencyBlocked, item)
		case ReasonTerminalUnsatisfiedDependency:
			result.TerminalUnsatisfied = append(result.TerminalUnsatisfied, item)
			if input.SelectionWorkflow == "" || item.Workflow == input.SelectionWorkflow {
				result.SelectionTerminalUnsatisfied = append(result.SelectionTerminalUnsatisfied, item)
			}
		case ReasonAwaitingOperator, ReasonNeedsInput:
			result.OperatorInput = append(result.OperatorInput, item)
		case ReasonConflictBlocked:
			result.ConflictBlocked = append(result.ConflictBlocked, item)
		default:
			result.Other = append(result.Other, item)
		}
	}
	sort.Slice(result.Ready, func(i, j int) bool { return readyBefore(result.Ready[i], result.Ready[j]) })
	for i := range result.WorkflowSelections {
		if selectedIndex := selectedReadyIndex(result.Ready, result.WorkflowSelections[i].Workflow); selectedIndex >= 0 {
			selected := cloneReadiness(result.Ready[selectedIndex])
			result.WorkflowSelections[i].SelectedNext = &selected
		}
	}
	if selectedIndex := selectedReadyIndex(result.Ready, input.SelectionWorkflow); selectedIndex >= 0 {
		selected := cloneReadiness(result.Ready[selectedIndex])
		result.SelectedNext = &selected
	}
	return result
}

func selectedReadyIndex(ready []TaskReadiness, workflow Workflow) int {
	for i := range ready {
		if workflow == "" || ready[i].Workflow == workflow {
			return i
		}
	}
	return -1
}

func emptyResult(selectionWorkflow Workflow) Result {
	return Result{
		SchemaVersion:                ResultSchemaVersion,
		SelectionWorkflow:            selectionWorkflow,
		Tasks:                        []TaskReadiness{},
		Ready:                        []TaskReadiness{},
		Waiting:                      []TaskReadiness{},
		DependencyBlocked:            []TaskReadiness{},
		TerminalUnsatisfied:          []TaskReadiness{},
		SelectionTerminalUnsatisfied: []TaskReadiness{},
		OperatorInput:                []TaskReadiness{},
		ConflictBlocked:              []TaskReadiness{},
		Other:                        []TaskReadiness{},
		InvalidGraph:                 []Diagnostic{},
		WorkflowSelections: []WorkflowSelection{
			{Workflow: WorkflowMixedPassV1},
			{Workflow: WorkflowAutonomousV1},
		},
	}
}

func normalizeTasks(input []Task) []Task {
	tasks := make([]Task, len(input))
	for i, task := range input {
		task.SourcePath = normalizeSourcePath(task.SourcePath)
		task.DependsOn = append([]string(nil), task.DependsOn...)
		task.Conflicts = append([]string(nil), task.Conflicts...)
		if task.Checkpoint != nil {
			checkpoint := *task.Checkpoint
			task.Checkpoint = &checkpoint
		}
		tasks[i] = task
	}
	sort.SliceStable(tasks, func(i, j int) bool { return taskIdentityBefore(tasks[i], tasks[j]) })
	return tasks
}

func normalizeArchives(input []Archive) []Archive {
	archives := append([]Archive(nil), input...)
	sort.SliceStable(archives, func(i, j int) bool {
		if archives[i].TaskID != archives[j].TaskID {
			return archives[i].TaskID < archives[j].TaskID
		}
		return archives[i].ArchiveID < archives[j].ArchiveID
	})
	return archives
}

func validate(tasks []Task, archives []Archive) ([]Diagnostic, map[string]Task, map[string]Archive) {
	diagnostics := make([]Diagnostic, 0)
	byID := make(map[string]Task, len(tasks))
	duplicateIDs := make(map[string]bool)
	for _, task := range tasks {
		if detail := validateTask(task); detail != "" {
			diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticInvalidTask, TaskID: task.ID, SourcePath: task.SourcePath, Cycle: []string{}, Detail: detail})
		}
		if previous, ok := byID[task.ID]; ok && task.ID != "" {
			duplicateIDs[task.ID] = true
			diagnostics = append(diagnostics, Diagnostic{
				Code: DiagnosticDuplicateTaskID, TaskID: task.ID, SourcePath: previous.SourcePath,
				RelatedSourcePath: task.SourcePath, Cycle: []string{}, Detail: fmt.Sprintf("task id %q appears in both %q and %q", task.ID, previous.SourcePath, task.SourcePath),
			})
		} else if task.ID != "" {
			byID[task.ID] = task
		}
		seenDependencies := make(map[string]struct{}, len(task.DependsOn))
		for _, dependencyID := range task.DependsOn {
			if _, ok := seenDependencies[dependencyID]; ok {
				diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticDuplicateDependencyEdge, TaskID: task.ID, DependencyID: dependencyID, SourcePath: task.SourcePath, Cycle: []string{}, Detail: fmt.Sprintf("task %q repeats dependency edge %q", task.ID, dependencyID)})
				continue
			}
			seenDependencies[dependencyID] = struct{}{}
			if dependencyID == task.ID {
				diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticSelfDependency, TaskID: task.ID, DependencyID: dependencyID, SourcePath: task.SourcePath, Cycle: []string{}, Detail: fmt.Sprintf("task %q depends on itself", task.ID)})
			}
		}
	}

	archiveByID := make(map[string]Archive, len(archives))
	archiveIDs := make(map[string]string, len(archives))
	for _, archive := range archives {
		if detail := validateArchive(archive); detail != "" {
			diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticMalformedArchive, TaskID: archive.TaskID, Cycle: []string{}, Detail: detail})
		}
		if previous, ok := archiveByID[archive.TaskID]; ok && archive.TaskID != "" {
			diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticDuplicateTaskID, TaskID: archive.TaskID, Cycle: []string{}, Detail: fmt.Sprintf("archived task id %q appears in archives %q and %q", archive.TaskID, previous.ArchiveID, archive.ArchiveID)})
		} else if archive.TaskID != "" {
			archiveByID[archive.TaskID] = archive
		}
		if taskID, ok := archiveIDs[archive.ArchiveID]; ok && archive.ArchiveID != "" && taskID != archive.TaskID {
			diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticMalformedArchive, TaskID: archive.TaskID, Cycle: []string{}, Detail: fmt.Sprintf("archive id %q is shared by tasks %q and %q", archive.ArchiveID, taskID, archive.TaskID)})
		} else if archive.ArchiveID != "" {
			archiveIDs[archive.ArchiveID] = archive.TaskID
		}
		if task, ok := byID[archive.TaskID]; ok && archive.TaskID != "" {
			diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticActiveArchiveAmbiguity, TaskID: archive.TaskID, SourcePath: task.SourcePath, Cycle: []string{}, Detail: fmt.Sprintf("task id %q is both active and archived", archive.TaskID)})
		}
	}

	for _, task := range tasks {
		seen := make(map[string]struct{}, len(task.DependsOn))
		for _, dependencyID := range task.DependsOn {
			if dependencyID == task.ID {
				continue
			}
			if _, duplicate := seen[dependencyID]; duplicate {
				continue
			}
			seen[dependencyID] = struct{}{}
			_, active := byID[dependencyID]
			_, archived := archiveByID[dependencyID]
			if !active && !archived {
				diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticMissingDependency, TaskID: task.ID, DependencyID: dependencyID, SourcePath: task.SourcePath, Cycle: []string{}, Detail: fmt.Sprintf("task %q has missing dependency %q", task.ID, dependencyID)})
			}
		}
	}

	cycleByID := make(map[string]Task, len(byID)-len(duplicateIDs))
	for id, task := range byID {
		if !duplicateIDs[id] {
			cycleByID[id] = task
		}
	}
	for _, cycle := range findCycles(cycleByID) {
		diagnostics = append(diagnostics, Diagnostic{Code: DiagnosticDependencyCycle, TaskID: cycle[0], Cycle: cycle, Detail: "dependency cycle: " + strings.Join(cycle, " -> ")})
	}
	sortDiagnostics(diagnostics)
	return diagnostics, byID, archiveByID
}

func validateTask(task Task) string {
	if task.ID == "" || strings.TrimSpace(task.ID) != task.ID {
		return "task id must be nonempty without surrounding whitespace"
	}
	if task.SourcePath == "" || task.SourcePath == "." {
		return fmt.Sprintf("task %q has no canonical source path", task.ID)
	}
	switch task.Workflow {
	case WorkflowMixedPassV1:
		if task.Checkpoint != nil {
			return fmt.Sprintf("mixed-pass task %q has operator checkpoint authority", task.ID)
		}
		if !oneOfState(task.State, StatePending, StateRunning, StateCompleted, StateBlocked, StateCancelled, StateAbandoned, StateSuperseded) {
			return fmt.Sprintf("mixed-pass task %q has unsupported state %q", task.ID, task.State)
		}
	case WorkflowAutonomousV1:
		if task.Checkpoint != nil {
			return fmt.Sprintf("autonomous task %q has operator checkpoint authority", task.ID)
		}
		if !oneOfState(task.State, StatePending, StateReady, StateRunning, StatePlanning, StateWorking, StateVerifying, StateAuditing, StateCorrecting, StateNeedsInput, StateFinalizing, StateCompleted, StateBlocked, StateCancelled, StateAbandoned, StateSuperseded) {
			return fmt.Sprintf("autonomous task %q has unsupported state %q", task.ID, task.State)
		}
	case WorkflowOperatorCheckpointV1:
		if !oneOfState(task.State, StatePending, StateAwaitingOperator, StateCompleted) {
			return fmt.Sprintf("operator checkpoint %q has unsupported state %q", task.ID, task.State)
		}
		if detail := validateCheckpointAuthority(task); detail != "" {
			return detail
		}
	default:
		return fmt.Sprintf("task %q has unsupported workflow %q", task.ID, task.Workflow)
	}
	return ""
}

func validateCheckpointAuthority(task Task) string {
	if task.Checkpoint == nil {
		return fmt.Sprintf("operator checkpoint %q has no receipt authority", task.ID)
	}
	authority := task.Checkpoint
	expectedPath := path.Join(".agent/checkpoints", task.ID, "receipt.json")
	if authority.ReceiptPath != expectedPath {
		return fmt.Sprintf("operator checkpoint %q receipt path %q is not canonical (want %q)", task.ID, authority.ReceiptPath, expectedPath)
	}
	switch task.State {
	case StatePending, StateAwaitingOperator:
		if authority.ReceiptSHA256 != "" || authority.Verified || authority.Detail != "" {
			return fmt.Sprintf("awaiting operator checkpoint %q must not claim bound receipt authority", task.ID)
		}
	case StateCompleted:
		if !validSHA256(authority.ReceiptSHA256) {
			return fmt.Sprintf("completed operator checkpoint %q has malformed receipt identity", task.ID)
		}
		if !authority.Verified {
			detail := strings.TrimSpace(authority.Detail)
			if detail == "" {
				detail = "receipt authority was not verified"
			}
			return fmt.Sprintf("completed operator checkpoint %q is invalid: %s", task.ID, detail)
		}
		if authority.Detail != "" {
			return fmt.Sprintf("completed operator checkpoint %q has contradictory receipt authority detail", task.ID)
		}
	}
	return ""
}

func validateArchive(archive Archive) string {
	if archive.TaskID == "" || strings.TrimSpace(archive.TaskID) != archive.TaskID {
		return "archive task id must be nonempty without surrounding whitespace"
	}
	if archive.ArchiveID == "" || strings.TrimSpace(archive.ArchiveID) != archive.ArchiveID {
		return fmt.Sprintf("archive authority for %q has invalid archive id", archive.TaskID)
	}
	if !archive.Verified || !archive.Reconciled {
		return fmt.Sprintf("archive authority for %q is not verified and reconciled", archive.TaskID)
	}
	if !oneOfState(archive.Disposition, StateCompleted, StateCancelled, StateAbandoned, StateSuperseded) {
		return fmt.Sprintf("archive authority for %q has unsupported disposition %q", archive.TaskID, archive.Disposition)
	}
	if archive.Disposition != StateCompleted && strings.TrimSpace(archive.Reason) == "" {
		return fmt.Sprintf("non-completed archive authority for %q has no terminal reason", archive.TaskID)
	}
	return ""
}

func classify(task Task, byID map[string]Task, archives map[string]Archive, occupied []string) TaskReadiness {
	switch task.Workflow {
	case WorkflowOperatorCheckpointV1:
		if task.State == StatePending || task.State == StateAwaitingOperator {
			return newReadiness(task, ReasonAwaitingOperator)
		}
	}
	switch task.State {
	case StateNeedsInput:
		return newReadiness(task, ReasonNeedsInput)
	case StateRunning, StatePlanning, StateWorking, StateVerifying, StateAuditing, StateCorrecting, StateFinalizing:
		return newReadiness(task, ReasonRunning)
	case StateCompleted:
		return newReadiness(task, ReasonCompleted)
	case StateBlocked:
		return newReadiness(task, ReasonBlocked)
	case StateCancelled:
		return newReadiness(task, ReasonCancelled)
	case StateAbandoned:
		return newReadiness(task, ReasonAbandoned)
	case StateSuperseded:
		return newReadiness(task, ReasonSuperseded)
	}

	item := newReadiness(task, ReasonReady)
	for _, dependencyID := range task.DependsOn {
		if archive, ok := archives[dependencyID]; ok {
			if archive.Disposition != StateCompleted {
				item.DependencyIssues = append(item.DependencyIssues, DependencyIssue{
					DependencyID: dependencyID, State: archive.Disposition, Reason: ReasonTerminalUnsatisfiedDependency,
					Archived: true, ArchiveID: archive.ArchiveID, Detail: archive.Reason,
				})
			}
			continue
		}
		dependency := byID[dependencyID]
		if dependency.State == StateCompleted {
			continue
		}
		issue := DependencyIssue{DependencyID: dependencyID, Workflow: dependency.Workflow, State: dependency.State}
		switch {
		case dependency.Workflow == WorkflowOperatorCheckpointV1 && (dependency.State == StatePending || dependency.State == StateAwaitingOperator):
			issue.Reason = ReasonAwaitingOperatorDependency
		case dependency.State == StateBlocked:
			issue.Reason = ReasonBlockedDependency
		case dependency.State == StateNeedsInput:
			issue.Reason = ReasonNeedsInputDependency
		case oneOfState(dependency.State, StateCancelled, StateAbandoned, StateSuperseded):
			issue.Reason = ReasonTerminalUnsatisfiedDependency
		default:
			issue.Reason = ReasonWaitingDependency
		}
		item.DependencyIssues = append(item.DependencyIssues, issue)
	}
	sort.Slice(item.DependencyIssues, func(i, j int) bool {
		return item.DependencyIssues[i].DependencyID < item.DependencyIssues[j].DependencyID
	})
	for _, issue := range item.DependencyIssues {
		item.UnmetDependencyIDs = append(item.UnmetDependencyIDs, issue.DependencyID)
	}
	if len(item.DependencyIssues) != 0 {
		item.Reason = dependencyPrecedence(item.DependencyIssues)
		return item
	}

	for _, occupiedID := range occupied {
		if conflicts(task, occupiedID, byID) {
			item.ConflictingTaskOrKeys = append(item.ConflictingTaskOrKeys, occupiedID)
		}
	}
	if len(item.ConflictingTaskOrKeys) != 0 {
		item.Reason = ReasonConflictBlocked
	}
	return item
}

func dependencyPrecedence(issues []DependencyIssue) Reason {
	result := ReasonWaitingDependency
	for _, issue := range issues {
		switch issue.Reason {
		case ReasonTerminalUnsatisfiedDependency:
			return ReasonTerminalUnsatisfiedDependency
		case ReasonNeedsInputDependency:
			result = ReasonNeedsInputDependency
		case ReasonBlockedDependency:
			if result != ReasonNeedsInputDependency {
				result = ReasonBlockedDependency
			}
		}
	}
	return result
}

func conflicts(task Task, occupiedID string, byID map[string]Task) bool {
	if task.ID == occupiedID || contains(task.Conflicts, occupiedID) {
		return true
	}
	other, ok := byID[occupiedID]
	if !ok {
		return false
	}
	if contains(other.Conflicts, task.ID) {
		return true
	}
	for _, left := range task.Conflicts {
		if contains(other.Conflicts, left) {
			return true
		}
	}
	return false
}

func newReadiness(task Task, reason Reason) TaskReadiness {
	return TaskReadiness{
		TaskID: task.ID, Workflow: task.Workflow, State: task.State, SourcePath: task.SourcePath,
		Priority: task.Priority, HasPriority: task.HasPriority, Reason: reason,
		UnmetDependencyIDs: []string{}, DependencyIssues: []DependencyIssue{}, ConflictingTaskOrKeys: []string{},
	}
}

func cloneReadiness(input TaskReadiness) TaskReadiness {
	input.UnmetDependencyIDs = append([]string{}, input.UnmetDependencyIDs...)
	input.DependencyIssues = append([]DependencyIssue{}, input.DependencyIssues...)
	input.ConflictingTaskOrKeys = append([]string{}, input.ConflictingTaskOrKeys...)
	return input
}

func taskIdentityBefore(left, right Task) bool {
	if left.SourcePath != right.SourcePath {
		return left.SourcePath < right.SourcePath
	}
	return left.ID < right.ID
}

func readyBefore(left, right TaskReadiness) bool {
	if left.HasPriority != right.HasPriority {
		return left.HasPriority
	}
	if left.HasPriority && left.Priority != right.Priority {
		return left.Priority < right.Priority
	}
	if left.SourcePath != right.SourcePath {
		return left.SourcePath < right.SourcePath
	}
	return left.TaskID < right.TaskID
}

func normalizeSourcePath(sourcePath string) string {
	sourcePath = strings.ReplaceAll(sourcePath, "\\", "/")
	if sourcePath == "" {
		return ""
	}
	return path.Clean(sourcePath)
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func normalizedUnique(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		leftKey := strings.Join([]string{string(left.Code), left.TaskID, left.DependencyID, left.SourcePath, left.RelatedSourcePath, strings.Join(left.Cycle, "\x00"), left.Detail}, "\x00")
		rightKey := strings.Join([]string{string(right.Code), right.TaskID, right.DependencyID, right.SourcePath, right.RelatedSourcePath, strings.Join(right.Cycle, "\x00"), right.Detail}, "\x00")
		return leftKey < rightKey
	})
}

func findCycles(byID map[string]Task) [][]string {
	index := 0
	indices := make(map[string]int, len(byID))
	lowlinks := make(map[string]int, len(byID))
	onStack := make(map[string]bool, len(byID))
	stack := make([]string, 0, len(byID))
	components := make([][]string, 0)

	var visit func(string)
	visit = func(id string) {
		indices[id] = index
		lowlinks[id] = index
		index++
		stack = append(stack, id)
		onStack[id] = true

		dependencies := activeDependencies(byID[id], byID)
		for _, dependencyID := range dependencies {
			if dependencyID == id {
				continue
			}
			if _, seen := indices[dependencyID]; !seen {
				visit(dependencyID)
				if lowlinks[dependencyID] < lowlinks[id] {
					lowlinks[id] = lowlinks[dependencyID]
				}
			} else if onStack[dependencyID] && indices[dependencyID] < lowlinks[id] {
				lowlinks[id] = indices[dependencyID]
			}
		}

		if lowlinks[id] != indices[id] {
			return
		}
		component := make([]string, 0)
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == id {
				break
			}
		}
		if len(component) > 1 {
			sort.Strings(component)
			components = append(components, component)
		}
	}

	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, seen := indices[id]; !seen {
			visit(id)
		}
	}

	cycles := make([][]string, 0, len(components))
	for _, component := range components {
		cycles = append(cycles, cyclePath(component, byID))
	}
	sort.Slice(cycles, func(i, j int) bool { return strings.Join(cycles[i], "\x00") < strings.Join(cycles[j], "\x00") })
	return cycles
}

func cyclePath(component []string, byID map[string]Task) []string {
	allowed := make(map[string]bool, len(component))
	for _, id := range component {
		allowed[id] = true
	}
	start := component[0]
	pathIDs := []string{start}
	inPath := map[string]bool{start: true}
	var search func(string) bool
	search = func(id string) bool {
		for _, dependencyID := range activeDependencies(byID[id], byID) {
			if !allowed[dependencyID] {
				continue
			}
			if dependencyID == start {
				pathIDs = append(pathIDs, start)
				return true
			}
			if inPath[dependencyID] {
				continue
			}
			inPath[dependencyID] = true
			pathIDs = append(pathIDs, dependencyID)
			if search(dependencyID) {
				return true
			}
			pathIDs = pathIDs[:len(pathIDs)-1]
			delete(inPath, dependencyID)
		}
		return false
	}
	if search(start) {
		return pathIDs
	}
	return append(append([]string(nil), component...), start)
}

func activeDependencies(task Task, byID map[string]Task) []string {
	set := make(map[string]struct{}, len(task.DependsOn))
	for _, dependencyID := range task.DependsOn {
		if _, ok := byID[dependencyID]; ok {
			set[dependencyID] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for dependencyID := range set {
		result = append(result, dependencyID)
	}
	sort.Strings(result)
	return result
}

func oneOfState(value State, allowed ...State) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
