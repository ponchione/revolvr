// Package autonomousmigration builds deterministic plans and applies them
// through restartable state-before-task publication transactions.
package autonomousmigration

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskschedule"
	"revolvr/internal/taskscheduler"
)

const PlanSchemaVersion = "autonomous-migration-plan-v1"

type Request struct {
	TaskIDs               []string
	All                   bool
	DryRun                bool
	AllowExactOrphanState bool
}

type Plan struct {
	SchemaVersion  string
	TargetWorkflow string
	DryRun         bool
	Entries        []Entry
}

type Entry struct {
	TaskID              string
	SourcePath          string
	SourceSHA256        string
	SourceByteSize      int
	SourceTask          taskfile.Task
	ProjectedTask       taskfile.Task
	ProjectedSHA256     string
	ProjectedByteSize   int
	AutonomousState     autonomous.ExecutionState
	AutonomousStatePath string
	StateBytes          []byte
	StateSHA256         string
	StateByteSize       int
}

type Rejection struct {
	TaskID     string
	SourcePath string
	Code       string
	Detail     string
}

type BatchError struct {
	Rejections []Rejection
}

func (e *BatchError) Error() string {
	if e == nil || len(e.Rejections) == 0 {
		return "plan autonomous migration: batch is ineligible"
	}
	var out strings.Builder
	out.WriteString("plan autonomous migration: batch is ineligible")
	for _, rejection := range e.Rejections {
		identity := rejection.TaskID
		if identity == "" {
			identity = "batch"
		}
		if rejection.SourcePath != "" {
			identity += " at " + rejection.SourcePath
		}
		fmt.Fprintf(&out, "\n- %s: %s: %s", identity, rejection.Code, rejection.Detail)
	}
	return out.String()
}

// Build validates one complete requested batch against the exact shared
// scheduling snapshot and returns projected task/state bytes without writing.
func Build(repositoryRoot string, snapshot taskschedule.Snapshot, request Request) (Plan, error) {
	root, err := resolveRoot(repositoryRoot)
	if err != nil {
		return Plan{}, fmt.Errorf("plan autonomous migration: %w", err)
	}
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return Plan{}, fmt.Errorf("plan autonomous migration: bind repository boundary: %w", err)
	}
	root = boundary.Root()
	if request.All && len(request.TaskIDs) != 0 {
		return Plan{}, errors.New("plan autonomous migration: --all cannot be combined with task IDs")
	}
	if !request.All && len(request.TaskIDs) == 0 {
		return Plan{}, errors.New("plan autonomous migration: provide at least one task ID or --all")
	}
	if !snapshot.Result.Valid() {
		rejections := make([]Rejection, 0, len(snapshot.Result.InvalidGraph))
		for _, diagnostic := range snapshot.Result.InvalidGraph {
			rejections = append(rejections, Rejection{
				TaskID: diagnostic.TaskID, SourcePath: diagnostic.SourcePath,
				Code: "invalid_graph", Detail: formatDiagnostic(diagnostic),
			})
		}
		return Plan{}, newBatchError(rejections)
	}

	selected, selectionRejections := selectTasks(snapshot.Tasks, request)
	if len(selectionRejections) != 0 {
		return Plan{}, newBatchError(selectionRejections)
	}

	entries := make([]Entry, 0, len(selected))
	rejections := make([]Rejection, 0)
	for _, task := range selected {
		taskRejections := validateEligibility(boundary, task, request.AllowExactOrphanState)
		if len(taskRejections) != 0 {
			rejections = append(rejections, taskRejections...)
			continue
		}
		projected, projectErr := taskfile.ProjectAutonomousMigration(root, task)
		if projectErr != nil {
			rejections = append(rejections, taskRejection(task, "projection_failed", projectErr.Error()))
			continue
		}
		state := initialState(task.ID)
		stateBytes, stateErr := autonomousstate.MarshalState(state)
		if stateErr != nil {
			return Plan{}, fmt.Errorf("plan autonomous migration: marshal state for %q: %w", task.ID, stateErr)
		}
		if request.AllowExactOrphanState {
			if code, detail := inspectExactOrphanState(boundary, projected.AutonomousStatePath, stateBytes); code != "" {
				rejections = append(rejections, taskRejection(task, code, detail))
				continue
			}
		}
		entries = append(entries, Entry{
			TaskID: task.ID, SourcePath: filepath.ToSlash(task.SourcePath),
			SourceSHA256: task.SourceSHA256(), SourceByteSize: task.SourceByteSize(), SourceTask: task,
			ProjectedTask: projected, ProjectedSHA256: projected.SourceSHA256(), ProjectedByteSize: projected.SourceByteSize(),
			AutonomousState: state, AutonomousStatePath: projected.AutonomousStatePath,
			StateBytes: append([]byte(nil), stateBytes...), StateSHA256: hash(stateBytes), StateByteSize: len(stateBytes),
		})
	}
	if len(rejections) != 0 {
		return Plan{}, newBatchError(rejections)
	}
	return Plan{
		SchemaVersion: PlanSchemaVersion, TargetWorkflow: taskfile.WorkflowAutonomousV1,
		DryRun: request.DryRun, Entries: entries,
	}, nil
}

func selectTasks(tasks []taskfile.Task, request Request) ([]taskfile.Task, []Rejection) {
	byID := make(map[string][]taskfile.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = append(byID[task.ID], task)
	}

	selected := make([]taskfile.Task, 0)
	rejections := make([]Rejection, 0)
	if request.All {
		for _, task := range tasks {
			if task.Workflow == taskfile.WorkflowMixedPassV1 {
				selected = append(selected, task)
			}
		}
		if len(selected) == 0 {
			rejections = append(rejections, Rejection{Code: "no_mixed_pass_tasks", Detail: "no active mixed-pass-v1 tasks are available to migrate"})
		}
	} else {
		seen := make(map[string]struct{}, len(request.TaskIDs))
		ids := append([]string(nil), request.TaskIDs...)
		sort.Strings(ids)
		for _, rawID := range ids {
			id := strings.TrimSpace(rawID)
			switch {
			case id == "" || id != rawID:
				rejections = append(rejections, Rejection{TaskID: rawID, Code: "invalid_task_id", Detail: "task IDs must be nonempty and have no surrounding whitespace"})
			case hasKey(seen, id):
				rejections = append(rejections, Rejection{TaskID: id, Code: "duplicate_request", Detail: "task ID is requested more than once"})
			default:
				seen[id] = struct{}{}
				matches := byID[id]
				if len(matches) == 0 {
					rejections = append(rejections, Rejection{TaskID: id, Code: "task_not_found", Detail: "no active canonical task has this ID"})
					continue
				}
				if len(matches) != 1 {
					rejections = append(rejections, Rejection{TaskID: id, Code: "ambiguous_task_id", Detail: "more than one active canonical task has this ID"})
					continue
				}
				selected = append(selected, matches[0])
			}
		}
	}
	sortTasks(selected)
	return selected, rejections
}

func validateEligibility(boundary runtimepath.Boundary, task taskfile.Task, allowExactOrphanState bool) []Rejection {
	rejections := make([]Rejection, 0)
	if task.Workflow != taskfile.WorkflowMixedPassV1 {
		rejections = append(rejections, taskRejection(task, "workflow_not_mixed_pass", fmt.Sprintf("workflow is %q; only %q can be migrated", task.Workflow, taskfile.WorkflowMixedPassV1)))
	}
	if task.Status != taskfile.StatusPending {
		rejections = append(rejections, taskRejection(task, "status_not_pending", fmt.Sprintf("status is %q; only pending tasks can be migrated", task.Status)))
	}
	if task.Phase != taskfile.PhaseImplement {
		rejections = append(rejections, taskRejection(task, "phase_not_implement", fmt.Sprintf("phase is %q; audit, document, simplify, and other prior-phase evidence cannot be represented", task.Phase)))
	}
	if task.ParentTaskID != "" || task.ChildProposalID != "" || task.ChildDecisionID != "" || task.ChildRunID != "" || len(task.ChildEvidence) != 0 || task.ParentBehavior != "" {
		rejections = append(rejections, taskRejection(task, "child_lineage_present", "supervised child lineage cannot be replaced by a migration-created lifecycle"))
	}
	if !allowExactOrphanState {
		statePath := path.Join(".revolvr", "autonomous", "tasks", task.ID, "state.json")
		if code, detail := inspectStateNamespace(boundary, statePath); code != "" {
			rejections = append(rejections, taskRejection(task, code, detail))
		}
	}
	return rejections
}

func inspectExactOrphanState(boundary runtimepath.Boundary, statePath string, expected []byte) (string, string) {
	absState := filepath.Join(boundary.Root(), filepath.FromSlash(statePath))
	namespace := filepath.Dir(absState)
	dir, found, err := boundary.OpenDir(namespace, true)
	if err != nil {
		return "autonomous_state_path_unsafe", fmt.Sprintf("inspect %s: %v", filepath.ToSlash(namespace), err)
	}
	if !found {
		return "", ""
	}
	defer dir.Close()
	entries, err := dir.ReadDir()
	if err != nil {
		return "autonomous_state_path_unsafe", fmt.Sprintf("read autonomous task namespace %q: %v", filepath.ToSlash(filepath.Dir(statePath)), err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(absState) {
		return "autonomous_namespace_exists", fmt.Sprintf("autonomous task namespace %q contains evidence other than the exact orphan state", filepath.ToSlash(filepath.Dir(statePath)))
	}
	raw, _, err := dir.ReadFileLimit(filepath.Base(absState), false, int64(len(expected)))
	if errors.Is(err, runtimepath.ErrReadLimit) {
		return "autonomous_state_conflict", fmt.Sprintf("autonomous state %q contains different bytes", statePath)
	}
	if err != nil {
		return "autonomous_state_path_unsafe", fmt.Sprintf("read autonomous state %q: %v", statePath, err)
	}
	if !bytes.Equal(raw, expected) {
		return "autonomous_state_conflict", fmt.Sprintf("autonomous state %q contains different bytes", statePath)
	}
	return "", ""
}

func inspectStateNamespace(boundary runtimepath.Boundary, statePath string) (string, string) {
	absState := filepath.Join(boundary.Root(), filepath.FromSlash(statePath))
	dir, found, err := boundary.OpenDir(filepath.Dir(absState), true)
	if err != nil {
		return "autonomous_state_path_unsafe", fmt.Sprintf("inspect %s: %v", statePath, err)
	}
	if !found {
		return "", ""
	}
	defer dir.Close()
	_, stateFound, err := dir.ReadFile(filepath.Base(absState), true)
	if err != nil {
		return "autonomous_state_path_unsafe", fmt.Sprintf("inspect autonomous state %q: %v", statePath, err)
	}
	if stateFound {
		return "autonomous_state_exists", fmt.Sprintf("autonomous state %q already exists", statePath)
	}
	return "autonomous_namespace_exists", fmt.Sprintf("autonomous task namespace %q already exists", filepath.ToSlash(filepath.Dir(statePath)))
}

func initialState(taskID string) autonomous.ExecutionState {
	return autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        taskID, Lifecycle: autonomous.LifecycleStatePending,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
}

func resolveRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("repository root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.Join(err, errors.New("repository root is not a directory"))
	}
	return resolved, nil
}

func taskRejection(task taskfile.Task, code, detail string) Rejection {
	return Rejection{TaskID: task.ID, SourcePath: filepath.ToSlash(task.SourcePath), Code: code, Detail: detail}
}

func newBatchError(rejections []Rejection) *BatchError {
	result := append([]Rejection(nil), rejections...)
	sort.Slice(result, func(i, j int) bool {
		left := strings.Join([]string{result[i].SourcePath, result[i].TaskID, result[i].Code, result[i].Detail}, "\x00")
		right := strings.Join([]string{result[j].SourcePath, result[j].TaskID, result[j].Code, result[j].Detail}, "\x00")
		return left < right
	})
	return &BatchError{Rejections: result}
}

func sortTasks(tasks []taskfile.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		left, right := filepath.ToSlash(tasks[i].SourcePath), filepath.ToSlash(tasks[j].SourcePath)
		if left != right {
			return left < right
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func formatDiagnostic(diagnostic taskscheduler.Diagnostic) string {
	parts := []string{string(diagnostic.Code)}
	if diagnostic.DependencyID != "" {
		parts = append(parts, "dependency="+diagnostic.DependencyID)
	}
	if len(diagnostic.Cycle) != 0 {
		parts = append(parts, "cycle="+strings.Join(diagnostic.Cycle, "->"))
	}
	if diagnostic.Detail != "" {
		parts = append(parts, strings.Join(strings.Fields(diagnostic.Detail), " "))
	}
	return strings.Join(parts, ": ")
}

func hasKey(values map[string]struct{}, key string) bool {
	_, ok := values[key]
	return ok
}

func hash(raw []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
