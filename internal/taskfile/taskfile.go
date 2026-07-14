package taskfile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"revolvr/internal/id"
	"revolvr/internal/operatorcheckpoint"
	"revolvr/internal/pathguard"
	"revolvr/internal/runtimepath"
)

const TasksDir = ".agent/tasks"

const (
	StatusPending    = "pending"
	StatusRunning    = "running"
	StatusCompleted  = "completed"
	StatusBlocked    = "blocked"
	StatusCancelled  = "cancelled"
	StatusSuperseded = "superseded"
	StatusAbandoned  = "abandoned"
)

const (
	WorkflowMixedPassV1          = "mixed-pass-v1"
	WorkflowAutonomousV1         = "autonomous-v1"
	WorkflowOperatorCheckpointV1 = "operator-checkpoint-v1"
	DefaultWorkflow              = WorkflowMixedPassV1
)

const (
	PhaseImplement = "implement"
	PhaseAudit     = "audit"
	PhaseDocument  = "document"
	PhaseSimplify  = "simplify"
	DefaultPhase   = PhaseImplement
)

type Task struct {
	ID                      string
	Title                   string
	Profile                 string
	Status                  string
	Workflow                string
	Phase                   string
	AutonomousStatePath     string
	CheckpointReceiptPath   string
	CheckpointReceiptSHA256 string
	Priority                int
	HasPriority             bool
	DependsOn               []string
	Tags                    []string
	Conflicts               []string
	ParentTaskID            string
	ChildProposalID         string
	ChildDecisionID         string
	ChildRunID              string
	ChildEvidence           []string
	ParentBehavior          string
	ContextBody             string
	SourcePath              string
	SourceBytes             []byte
}

const (
	ParentBehaviorDependent   = "depends_on_parent"
	ParentBehaviorIndependent = "independent"
)

// AutonomousCreateInput is the typed, canonical taskfile boundary used by
// supervised child publication. Ordinary task add/import continues to use
// CreateInput and therefore cannot manufacture autonomous lineage.
type AutonomousCreateInput struct {
	ID, Title, Body               string
	Priority                      int
	HasPriority                   bool
	DependsOn, Tags, Conflicts    []string
	ParentTaskID, ChildProposalID string
	ChildDecisionID, ChildRunID   string
	ChildEvidence                 []string
	ParentBehavior                string
}

type CreateInput struct {
	ID        string
	Title     string
	Body      string
	DependsOn []string
	Tags      []string
	Conflicts []string
}

type MetadataUpdate struct {
	Status string
	Phase  string
}

var writeCheckpointFileAtomically = writeFileAtomically
var writeMigrationFileAtomically = writeFileAtomically

type ReopenInput struct {
	OriginalSourcePath  string
	ArchivedSourceBytes []byte
	NewTaskID           string
}

// ParseArchivedTask validates exact preserved task bytes against their former
// canonical active path without requiring that active file to still exist.
func ParseArchivedTask(repositoryRoot, originalSourcePath string, raw []byte) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, _, err := resolveTaskPath(root, originalSourcePath)
	if err != nil {
		return Task{}, err
	}
	task, err := parse(raw, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("parse archived task source: %w", err)
	}
	return task, nil
}

// ProjectReopenedTask creates the exact pending autonomous task bytes for a
// new lifecycle without mutating the archived terminal source. Only the
// harness-owned id, status, and autonomous state reference are changed.
func ProjectReopenedTask(repositoryRoot string, input ReopenInput) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	newID := strings.TrimSpace(input.NewTaskID)
	if !validTaskID(newID) {
		return Task{}, fmt.Errorf("project reopened task: invalid new task id %q", input.NewTaskID)
	}
	original, err := parse(input.ArchivedSourceBytes, input.OriginalSourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("project reopened task: archived source: %w", err)
	}
	if original.Workflow != WorkflowAutonomousV1 || !terminalArchiveStatus(original.Status) {
		return Task{}, errors.New("project reopened task: archived source is not a terminal autonomous task")
	}
	updated, err := rewriteReopenMetadata(input.ArchivedSourceBytes, newID)
	if err != nil {
		return Task{}, fmt.Errorf("project reopened task: %w", err)
	}
	target := filepath.ToSlash(filepath.Join(TasksDir, newID+".md"))
	projected, err := parse(updated, target, root)
	if err != nil {
		return Task{}, fmt.Errorf("project reopened task: projected source: %w", err)
	}
	return projected, nil
}

// PublishReopenedTask atomically publishes a previously projected task with
// no-overwrite semantics and strict byte-for-byte readback.
func PublishReopenedTask(repositoryRoot string, projected Task) (Task, error) {
	return publishProjectedTask(repositoryRoot, projected, "publish reopened task")
}

// ProjectAutonomousMigration returns the exact canonical task bytes produced
// by migrating one pending mixed-pass implementation task. It preserves every
// unrelated frontmatter and body byte, removes mixed-pass-only routing fields,
// and performs no filesystem mutation.
func ProjectAutonomousMigration(repositoryRoot string, snapshot Task) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(snapshot.SourcePath) == "" || len(snapshot.SourceBytes) == 0 {
		return Task{}, errors.New("project autonomous migration: exact task snapshot is required")
	}
	sourcePath, _, err := resolveTaskPath(root, snapshot.SourcePath)
	if err != nil {
		return Task{}, err
	}
	current, err := parse(snapshot.SourceBytes, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("project autonomous migration %s: validate snapshot: %w", sourcePath, err)
	}
	if current.ID != snapshot.ID {
		return Task{}, fmt.Errorf("project autonomous migration %s: task identity changed from %q to %q", sourcePath, snapshot.ID, current.ID)
	}
	if current.Workflow != WorkflowMixedPassV1 || current.Status != StatusPending || current.Phase != PhaseImplement {
		return Task{}, fmt.Errorf("project autonomous migration %s: task %q is not a pending mixed-pass implement task", sourcePath, current.ID)
	}
	if current.ParentTaskID != "" || current.ChildProposalID != "" || current.ChildDecisionID != "" || current.ChildRunID != "" || len(current.ChildEvidence) != 0 || current.ParentBehavior != "" {
		return Task{}, fmt.Errorf("project autonomous migration %s: task %q has child lineage", sourcePath, current.ID)
	}

	updated, err := rewriteAutonomousMigrationMetadata(snapshot.SourceBytes, current.ID)
	if err != nil {
		return Task{}, fmt.Errorf("project autonomous migration %s: %w", sourcePath, err)
	}
	projected, err := parse(updated, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("project autonomous migration %s: validate projected task: %w", sourcePath, err)
	}
	return projected, nil
}

// PublishAutonomousMigration atomically replaces one exact mixed-pass task
// snapshot with its exact autonomous migration projection. An already
// projected file is an idempotent replay; every other current byte sequence is
// a conflict and is never overwritten.
func PublishAutonomousMigration(repositoryRoot string, snapshot, projected Task) (Task, bool, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, false, err
	}
	if strings.TrimSpace(snapshot.SourcePath) == "" || len(snapshot.SourceBytes) == 0 {
		return Task{}, false, errors.New("publish autonomous migration: exact mixed-pass task snapshot is required")
	}
	want, err := ProjectAutonomousMigration(root, snapshot)
	if err != nil {
		return Task{}, false, err
	}
	if projected.ID != want.ID || projected.SourcePath != want.SourcePath || !bytes.Equal(projected.SourceBytes, want.SourceBytes) {
		return Task{}, false, errors.New("publish autonomous migration: projected task differs from the deterministic migration projection")
	}
	sourcePath, absPath, err := resolveTaskPath(root, snapshot.SourcePath)
	if err != nil {
		return Task{}, false, err
	}
	currentRaw, err := os.ReadFile(absPath)
	if err != nil {
		return Task{}, false, fmt.Errorf("publish autonomous migration %s: %w", sourcePath, err)
	}
	if bytes.Equal(currentRaw, projected.SourceBytes) {
		current, loadErr := Load(root, sourcePath)
		return current, false, loadErr
	}
	if !bytes.Equal(currentRaw, snapshot.SourceBytes) {
		return Task{}, false, fmt.Errorf("publish autonomous migration %s: task bytes changed since planning", sourcePath)
	}
	if err := writeMigrationFileAtomically(absPath, projected.SourceBytes, 0o644); err != nil {
		return Task{}, false, fmt.Errorf("publish autonomous migration %s: %w", sourcePath, err)
	}
	if err := syncTaskDirectory(filepath.Dir(absPath)); err != nil {
		return Task{}, false, fmt.Errorf("publish autonomous migration %s: sync task directory: %w", sourcePath, err)
	}
	current, err := Load(root, sourcePath)
	if err != nil {
		return Task{}, false, err
	}
	if !bytes.Equal(current.SourceBytes, projected.SourceBytes) {
		return Task{}, false, fmt.Errorf("publish autonomous migration %s: readback identity mismatch", sourcePath)
	}
	return current, true, nil
}

// ProjectAutonomousTask returns deterministic LF task bytes for a new pending
// autonomous task. It performs the same strict parse used for canonical files.
func ProjectAutonomousTask(repositoryRoot string, input AutonomousCreateInput) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	if !validTaskID(input.ID) {
		return Task{}, fmt.Errorf("project autonomous task: invalid task id %q", input.ID)
	}
	title := taskTitle(input.Title)
	body := strings.TrimSpace(normalizeLineEndings(input.Body))
	if title == "" || body == "" {
		return Task{}, errors.New("project autonomous task: title and body are required")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "---\nid: %s\nstatus: %s\nworkflow: %s\nautonomous_state_path: %s\n", input.ID, StatusPending, WorkflowAutonomousV1, path.Join(".revolvr", "autonomous", "tasks", input.ID, "state.json"))
	if input.HasPriority {
		fmt.Fprintf(&out, "priority: %d\n", input.Priority)
	}
	writeListMetadata := func(key string, values []string) {
		if len(values) != 0 {
			fmt.Fprintf(&out, "%s: %s\n", key, strings.Join(values, ","))
		}
	}
	writeListMetadata("depends_on", input.DependsOn)
	writeListMetadata("tags", input.Tags)
	writeListMetadata("conflicts", input.Conflicts)
	if input.ParentTaskID != "" {
		fmt.Fprintf(&out, "parent_task_id: %s\nchild_proposal_id: %s\nchild_decision_id: %s\nchild_run_id: %s\nparent_behavior: %s\n", input.ParentTaskID, input.ChildProposalID, input.ChildDecisionID, input.ChildRunID, input.ParentBehavior)
		writeListMetadata("child_evidence", input.ChildEvidence)
	}
	fmt.Fprintf(&out, "---\n# %s\n\n%s\n", title, body)
	sourcePath := filepath.ToSlash(filepath.Join(TasksDir, input.ID+".md"))
	task, err := parse([]byte(out.String()), sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("project autonomous task: %w", err)
	}
	return task, nil
}

// PublishAutonomousTask atomically publishes exact projected bytes without
// overwriting or adopting different user-owned content.
func PublishAutonomousTask(repositoryRoot string, projected Task) (Task, error) {
	return publishProjectedTask(repositoryRoot, projected, "publish autonomous task")
}

func publishProjectedTask(repositoryRoot string, projected Task, operation string) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	if projected.Status != StatusPending || projected.Workflow != WorkflowAutonomousV1 || len(projected.SourceBytes) == 0 {
		return Task{}, fmt.Errorf("%s: projected pending autonomous task is required", operation)
	}
	sourcePath, absPath, err := resolveTaskPath(root, projected.SourcePath)
	if err != nil {
		return Task{}, err
	}
	if sourcePath != filepath.ToSlash(filepath.Join(TasksDir, projected.ID+".md")) && sourcePath != filepath.Join(TasksDir, projected.ID+".md") {
		return Task{}, fmt.Errorf("%s: source path is not canonical for the new task id", operation)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return Task{}, err
	}
	if _, err := os.Lstat(absPath); err == nil {
		existing, loadErr := Load(root, sourcePath)
		if loadErr == nil && bytes.Equal(existing.SourceBytes, projected.SourceBytes) {
			return existing, nil
		}
		return Task{}, fmt.Errorf("%s: target %s already exists with different bytes", operation, sourcePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Task{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(absPath), ".reopen-task.tmp-*")
	if err != nil {
		return Task{}, err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return Task{}, err
	}
	if _, err := file.Write(projected.SourceBytes); err != nil {
		_ = file.Close()
		return Task{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return Task{}, err
	}
	if err := file.Close(); err != nil {
		return Task{}, err
	}
	if err := os.Link(tempPath, absPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, loadErr := Load(root, sourcePath)
			if loadErr == nil && bytes.Equal(existing.SourceBytes, projected.SourceBytes) {
				return existing, nil
			}
			return Task{}, fmt.Errorf("%s: target %s appeared with different bytes", operation, sourcePath)
		}
		return Task{}, err
	}
	if err := os.Remove(tempPath); err != nil {
		return Task{}, err
	}
	if err := syncTaskDirectory(filepath.Dir(absPath)); err != nil {
		return Task{}, err
	}
	readback, err := Load(root, sourcePath)
	if err != nil || !bytes.Equal(readback.SourceBytes, projected.SourceBytes) {
		return Task{}, errors.Join(err, fmt.Errorf("%s: readback mismatch", operation))
	}
	return readback, nil
}

func Create(repositoryRoot string, input CreateInput) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}

	title := taskTitle(input.Title)
	if title == "" {
		return Task{}, errors.New("create task file: title is required")
	}
	body := strings.TrimSpace(normalizeLineEndings(input.Body))
	if body == "" {
		return Task{}, errors.New("create task file: body is required")
	}

	taskID := strings.TrimSpace(input.ID)
	generated := taskID == ""
	for attempts := 0; attempts < 8; attempts++ {
		if taskID == "" {
			taskID = id.New()
		}
		if !validTaskID(taskID) {
			return Task{}, fmt.Errorf("create task file: invalid task id %q", taskID)
		}
		if err := validateCreateScheduling(taskID, input.DependsOn, input.Tags, input.Conflicts); err != nil {
			return Task{}, fmt.Errorf("create task file: %w", err)
		}

		if existing, ok, err := FindByID(root, taskID); err != nil {
			return Task{}, fmt.Errorf("create task file: %w", err)
		} else if ok {
			if generated {
				taskID = ""
				continue
			}
			return Task{}, fmt.Errorf("create task file: task id %q already exists at %s", taskID, existing.SourcePath)
		}

		task, err := writeNewTaskFile(root, taskID, title, body, input.DependsOn, input.Tags, input.Conflicts, generated)
		if err != nil {
			if generated && errors.Is(err, os.ErrExist) {
				taskID = ""
				continue
			}
			return Task{}, err
		}
		return task, nil
	}
	return Task{}, errors.New("create task file: generated task id collided repeatedly")
}

func validateCreateScheduling(taskID string, dependsOn, tags, conflicts []string) error {
	for _, value := range []struct {
		key   string
		items []string
		valid func(string) bool
	}{{"depends_on", dependsOn, validTaskID}, {"tags", tags, validSchedulingToken}, {"conflicts", conflicts, validSchedulingToken}} {
		parsed, err := parseIdentityList(value.key, strings.Join(value.items, ","), value.valid)
		if err != nil {
			return err
		}
		if value.key == "depends_on" && containsString(parsed, taskID) {
			return fmt.Errorf("depends_on contains self dependency %q", taskID)
		}
	}
	return nil
}

// ValidateSchedulingMetadata validates list syntax and duplicates without a
// task identity. Create and autonomous projection additionally reject self
// dependencies once the canonical ID is known.
func ValidateSchedulingMetadata(dependsOn, tags, conflicts []string) error {
	return validateCreateScheduling("__validation_identity__", dependsOn, tags, conflicts)
}

func Load(repositoryRoot string, path string) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, absPath, err := resolveTaskPath(root, path)
	if err != nil {
		return Task{}, err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return Task{}, fmt.Errorf("load task file %s: %w", sourcePath, err)
	}
	task, err := parse(raw, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("load task file %s: %w", sourcePath, err)
	}
	return task, nil
}

func List(repositoryRoot string) ([]Task, error) {
	tasks, err := LoadAll(repositoryRoot)
	if err != nil {
		return nil, err
	}
	tasksByID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		if previous, exists := tasksByID[task.ID]; exists {
			return nil, fmt.Errorf("task id %q is duplicated in %s and %s", task.ID, previous.SourcePath, task.SourcePath)
		}
		tasksByID[task.ID] = task
	}
	return tasks, nil
}

// LoadAll returns every locally valid canonical task in source-path order
// without applying cross-task graph validation such as duplicate identity
// checks. Shared scheduling adapters use this boundary so taskscheduler owns
// complete-graph diagnostics; ordinary List callers retain fail-fast
// compatibility until their projections migrate.
func LoadAll(repositoryRoot string) ([]Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, TasksDir)
	if err := validateResolvedTaskDirectory(root, dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list task files: read %s: %w", TasksDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isTaskDocumentName(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	tasks := make([]Task, 0, len(names))
	for _, name := range names {
		task, err := Load(root, filepath.Join(TasksDir, name))
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func isTaskDocumentName(name string) bool {
	return name != "AGENTS.md" && filepath.Ext(name) == ".md"
}

func FindByID(repositoryRoot string, taskID string) (Task, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, false, errors.New("find task file: task id is required")
	}

	tasks, err := List(repositoryRoot)
	if err != nil {
		return Task{}, false, err
	}
	var found Task
	for _, task := range tasks {
		if task.ID != taskID {
			continue
		}
		if found.ID != "" {
			return Task{}, false, fmt.Errorf("task id %q is duplicated in %s and %s", taskID, found.SourcePath, task.SourcePath)
		}
		found = task
	}
	if found.ID == "" {
		return Task{}, false, nil
	}
	return found, true, nil
}

func UpdateBlockedToPending(repositoryRoot string, taskID string) (Task, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, false, errors.New("update blocked task file: task id is required")
	}

	task, ok, err := FindByID(repositoryRoot, taskID)
	if err != nil {
		return Task{}, false, err
	}
	if !ok {
		return Task{}, false, nil
	}
	if task.Status != StatusBlocked {
		return task, false, nil
	}
	updated, err := UpdateStatus(repositoryRoot, task.SourcePath, StatusPending)
	if err != nil {
		return Task{}, false, err
	}
	return updated, true, nil
}

func UpdateStatus(repositoryRoot string, path string, status string) (Task, error) {
	return UpdateMetadata(repositoryRoot, path, MetadataUpdate{Status: status})
}

// FulfillOperatorCheckpoint atomically changes one exact pending checkpoint
// snapshot to completed and binds it to the supplied receipt identity. An
// already completed checkpoint with the same identity is an idempotent replay;
// every other completed state is a conflict.
func FulfillOperatorCheckpoint(repositoryRoot string, snapshot Task, receiptSHA256 string) (Task, bool, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, false, err
	}
	if strings.TrimSpace(snapshot.SourcePath) == "" || len(snapshot.SourceBytes) == 0 {
		return Task{}, false, errors.New("fulfill operator checkpoint: exact task snapshot is required")
	}
	sourcePath, absPath, err := resolveTaskPath(root, snapshot.SourcePath)
	if err != nil {
		return Task{}, false, err
	}
	projected, changed, err := projectOperatorCheckpointFulfillment(root, sourcePath, snapshot, receiptSHA256)
	if err != nil {
		return Task{}, false, err
	}

	currentRaw, err := os.ReadFile(absPath)
	if err != nil {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: %w", sourcePath, err)
	}
	if !bytes.Equal(currentRaw, snapshot.SourceBytes) {
		current, parseErr := parse(currentRaw, sourcePath, root)
		if parseErr == nil && current.ID == snapshot.ID && current.Workflow == WorkflowOperatorCheckpointV1 && current.Status == StatusCompleted && current.CheckpointReceiptSHA256 == receiptSHA256 {
			return current, false, nil
		}
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: task bytes changed since validation", sourcePath)
	}
	if !changed {
		return projected, false, nil
	}
	if err := writeCheckpointFileAtomically(absPath, projected.SourceBytes, 0o644); err != nil {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: %w", sourcePath, err)
	}
	return projected, true, nil
}

func projectOperatorCheckpointFulfillment(root, sourcePath string, snapshot Task, receiptSHA256 string) (Task, bool, error) {
	parsed, err := parse(snapshot.SourceBytes, sourcePath, root)
	if err != nil {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: validate snapshot: %w", sourcePath, err)
	}
	if parsed.ID != snapshot.ID {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: task identity changed from %q to %q", sourcePath, snapshot.ID, parsed.ID)
	}
	if parsed.Workflow != WorkflowOperatorCheckpointV1 {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: task %q uses workflow %q", sourcePath, parsed.ID, parsed.Workflow)
	}
	if !operatorcheckpoint.ValidSHA256(receiptSHA256) {
		return Task{}, false, errors.New("fulfill operator checkpoint: receipt identity must be a lowercase SHA-256")
	}
	if parsed.Status == StatusCompleted {
		if parsed.CheckpointReceiptSHA256 != receiptSHA256 {
			return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: conflicting replay binds %s, not %s", sourcePath, parsed.CheckpointReceiptSHA256, receiptSHA256)
		}
		return parsed, false, nil
	}
	if parsed.Status != StatusPending {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: checkpoint status is %q", sourcePath, parsed.Status)
	}
	updated, err := fulfillOperatorCheckpointBytes(snapshot.SourceBytes, receiptSHA256)
	if err != nil {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: %w", sourcePath, err)
	}
	projected, err := parse(updated, sourcePath, root)
	if err != nil {
		return Task{}, false, fmt.Errorf("fulfill operator checkpoint %s: validate projected task: %w", sourcePath, err)
	}
	return projected, true, nil
}

func UpdateMetadata(repositoryRoot string, path string, update MetadataUpdate) (Task, error) {
	update, err := validateMetadataUpdate(update)
	if err != nil {
		return Task{}, err
	}

	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, absPath, err := resolveTaskPath(root, path)
	if err != nil {
		return Task{}, err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	return updateMetadataFromBytes(root, sourcePath, absPath, raw, update)
}

func UpdateMetadataFromSnapshot(repositoryRoot string, snapshot Task, update MetadataUpdate) (Task, error) {
	update, err := validateMetadataUpdate(update)
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(snapshot.SourcePath) == "" {
		return Task{}, errors.New("update task metadata from snapshot: source path is required")
	}
	if len(snapshot.SourceBytes) == 0 {
		return Task{}, errors.New("update task metadata from snapshot: source bytes are required")
	}

	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, absPath, err := resolveTaskPath(root, snapshot.SourcePath)
	if err != nil {
		return Task{}, err
	}
	parsed, err := parse(snapshot.SourceBytes, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata from snapshot %s: %w", sourcePath, err)
	}
	if parsed.ID != snapshot.ID {
		return Task{}, fmt.Errorf("update task metadata from snapshot %s: task id changed from %q to %q", sourcePath, snapshot.ID, parsed.ID)
	}
	return updateMetadataFromBytes(root, sourcePath, absPath, snapshot.SourceBytes, update)
}

// ProjectMetadataFromSnapshot returns the exact task bytes that a snapshot
// metadata update would write without reading or mutating the current file.
func ProjectMetadataFromSnapshot(repositoryRoot string, snapshot Task, update MetadataUpdate) (Task, error) {
	update, err := validateMetadataUpdate(update)
	if err != nil {
		return Task{}, err
	}
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, _, err := resolveTaskPath(root, snapshot.SourcePath)
	if err != nil {
		return Task{}, err
	}
	parsed, err := parse(snapshot.SourceBytes, sourcePath, root)
	if err != nil || parsed.ID != snapshot.ID {
		return Task{}, errors.Join(err, errors.New("project task metadata: snapshot identity mismatch"))
	}
	updated, err := updateMetadataBytes(snapshot.SourceBytes, update)
	if err != nil {
		return Task{}, err
	}
	return parse(updated, sourcePath, root)
}

func validateMetadataUpdate(update MetadataUpdate) (MetadataUpdate, error) {
	update.Status = strings.TrimSpace(update.Status)
	update.Phase = strings.TrimSpace(update.Phase)
	if update.Status == "" && update.Phase == "" {
		return MetadataUpdate{}, errors.New("update task metadata: no metadata update requested")
	}
	if update.Status != "" && !validStatus(update.Status) {
		return MetadataUpdate{}, fmt.Errorf("invalid status %q", update.Status)
	}
	if update.Phase != "" && !validPhase(update.Phase) {
		return MetadataUpdate{}, fmt.Errorf("invalid phase %q", update.Phase)
	}
	return update, nil
}

func updateMetadataFromBytes(root string, sourcePath string, absPath string, raw []byte, update MetadataUpdate) (Task, error) {
	if _, err := parse(raw, sourcePath, root); err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}

	updated, err := updateMetadataBytes(raw, update)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	task, err := parse(updated, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	if err := writeFileAtomically(absPath, updated, 0o644); err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	return task, nil
}

func writeNewTaskFile(root string, taskID string, title string, body string, dependsOn, tags, conflicts []string, generated bool) (Task, error) {
	canonicalRoot, err := runtimepath.CanonicalRoot(root)
	if err != nil {
		return Task{}, fmt.Errorf("create task file: resolve repository root: %w", err)
	}
	root = canonicalRoot
	dir := filepath.Join(root, TasksDir)
	if err := runtimepath.EnsureDir(root, dir, 0o755); err != nil {
		return Task{}, fmt.Errorf("create task file: create %s: %w", TasksDir, err)
	}

	sourcePath, absPath, err := resolveTaskPath(root, filepath.Join(TasksDir, taskID+".md"))
	if err != nil {
		return Task{}, fmt.Errorf("create task file: %w", err)
	}
	content := createTaskMarkdown(taskID, title, body, dependsOn, tags, conflicts)
	file, err := runtimepath.OpenFile(root, absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) && generated {
			return Task{}, err
		}
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, err)
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	identityErr := runtimepath.CheckOpenedFile(root, absPath, file)
	closeErr := file.Close()
	if writeErr != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, writeErr)
	}
	if syncErr != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, syncErr)
	}
	if identityErr != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, identityErr)
	}
	if closeErr != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, closeErr)
	}

	task, err := parse(content, sourcePath, root)
	if err != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, err)
	}
	return task, nil
}

func createTaskMarkdown(taskID string, title string, body string, dependsOn, tags, conflicts []string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "---\nid: %s\nstatus: %s\n", taskID, StatusPending)
	for _, value := range []struct {
		key   string
		items []string
	}{{"depends_on", dependsOn}, {"tags", tags}, {"conflicts", conflicts}} {
		if len(value.items) != 0 {
			fmt.Fprintf(&out, "%s: %s\n", value.key, strings.Join(value.items, ","))
		}
	}
	fmt.Fprintf(&out, "---\n# %s\n\n%s\n", title, body)
	return []byte(out.String())
}

func taskTitle(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func (t Task) SourceSHA256() string {
	sum := sha256.Sum256(t.SourceBytes)
	return fmt.Sprintf("%x", sum)
}

func (t Task) SourceByteSize() int {
	return len(t.SourceBytes)
}

func parse(raw []byte, sourcePath string, repositoryRoot string) (Task, error) {
	lines := splitLines(string(raw))
	meta, bodyStart, err := parseFrontmatter(lines, sourcePath)
	if err != nil {
		return Task{}, err
	}
	title, err := findH1Title(lines[bodyStart:])
	if err != nil {
		return Task{}, err
	}

	status, statusSet := meta["status"]
	status = strings.TrimSpace(status)
	if !statusSet {
		status = StatusPending
	}
	if !validStatus(status) {
		return Task{}, fmt.Errorf("invalid status %q", status)
	}

	workflow, workflowSet := meta["workflow"]
	workflow = strings.TrimSpace(workflow)
	if !workflowSet {
		workflow = DefaultWorkflow
	}
	if !validWorkflow(workflow) {
		return Task{}, fmt.Errorf("invalid workflow %q", workflow)
	}

	var priority int
	hasPriority := false
	if rawPriority, prioritySet := meta["priority"]; prioritySet {
		rawPriority = strings.TrimSpace(rawPriority)
		parsed, err := strconv.Atoi(rawPriority)
		if err != nil {
			return Task{}, fmt.Errorf("invalid priority %q", rawPriority)
		}
		priority = parsed
		hasPriority = true
	}

	taskID := strings.TrimSpace(meta["id"])
	if taskID == "" {
		base := filepath.Base(sourcePath)
		taskID = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if !validTaskID(taskID) {
		return Task{}, fmt.Errorf("invalid task id %q", taskID)
	}

	dependsOn, err := parseIdentityList("depends_on", meta["depends_on"], validTaskID)
	if err != nil {
		return Task{}, err
	}
	for _, dependency := range dependsOn {
		if dependency == taskID {
			return Task{}, fmt.Errorf("depends_on contains self dependency %q", taskID)
		}
	}
	tags, err := parseIdentityList("tags", meta["tags"], validSchedulingToken)
	if err != nil {
		return Task{}, err
	}
	conflicts, err := parseIdentityList("conflicts", meta["conflicts"], validSchedulingToken)
	if err != nil {
		return Task{}, err
	}
	childEvidence, err := parseIdentityList("child_evidence", meta["child_evidence"], validEvidenceToken)
	if err != nil {
		return Task{}, err
	}
	parentTaskID := meta["parent_task_id"]
	childProposalID := meta["child_proposal_id"]
	childDecisionID := meta["child_decision_id"]
	childRunID := meta["child_run_id"]
	parentBehavior := meta["parent_behavior"]
	lineageValues := []string{parentTaskID, childProposalID, childDecisionID, childRunID, parentBehavior}
	lineageSet := false
	for _, value := range lineageValues {
		lineageSet = lineageSet || value != ""
	}
	lineageSet = lineageSet || len(childEvidence) != 0
	if lineageSet {
		if !validTaskID(parentTaskID) || parentTaskID == taskID || !validTaskID(childProposalID) || !validTaskID(childDecisionID) || !validTaskID(childRunID) || len(childEvidence) == 0 {
			return Task{}, errors.New("child lineage requires distinct valid parent_task_id, child_proposal_id, child_decision_id, child_run_id, and child_evidence")
		}
		if parentBehavior != ParentBehaviorDependent && parentBehavior != ParentBehaviorIndependent {
			return Task{}, fmt.Errorf("invalid parent_behavior %q", parentBehavior)
		}
		if parentBehavior == ParentBehaviorDependent && !containsString(dependsOn, parentTaskID) {
			return Task{}, errors.New("depends_on_parent child must name parent_task_id in depends_on")
		}
		if parentBehavior == ParentBehaviorIndependent && containsString(dependsOn, parentTaskID) {
			return Task{}, errors.New("independent child must not depend on parent_task_id")
		}
	}

	profile, profileSet := meta["profile"]
	profile = strings.TrimSpace(profile)
	statePath, statePathSet := meta["autonomous_state_path"]
	statePath = strings.TrimSpace(statePath)
	receiptPath, receiptPathSet := meta["checkpoint_receipt_path"]
	receiptPath = strings.TrimSpace(receiptPath)
	receiptSHA256, receiptSHA256Set := meta["checkpoint_receipt_sha256"]
	receiptSHA256 = strings.TrimSpace(receiptSHA256)
	phase, phaseSet := meta["phase"]
	phase = strings.TrimSpace(phase)
	switch workflow {
	case WorkflowMixedPassV1:
		if profile != "" && !validProfileName(profile) {
			return Task{}, fmt.Errorf("invalid profile name %q", profile)
		}
		if !phaseSet {
			phase = DefaultPhase
		}
		if !validPhase(phase) {
			return Task{}, fmt.Errorf("invalid phase %q", phase)
		}
		if statePathSet {
			return Task{}, fmt.Errorf("frontmatter key %q is not allowed for workflow %q", "autonomous_state_path", workflow)
		}
		if receiptPathSet || receiptSHA256Set {
			return Task{}, fmt.Errorf("checkpoint receipt metadata is not allowed for workflow %q", workflow)
		}
	case WorkflowAutonomousV1:
		if phaseSet {
			return Task{}, fmt.Errorf("frontmatter key %q is not allowed for workflow %q", "phase", workflow)
		}
		if profile != "" {
			return Task{}, fmt.Errorf("frontmatter key %q is not allowed for workflow %q", "profile", workflow)
		}
		if !statePathSet || statePath == "" {
			return Task{}, fmt.Errorf("frontmatter key %q is required for workflow %q", "autonomous_state_path", workflow)
		}
		if err := validateAutonomousStatePath(repositoryRoot, taskID, statePath); err != nil {
			return Task{}, err
		}
		if receiptPathSet || receiptSHA256Set {
			return Task{}, fmt.Errorf("checkpoint receipt metadata is not allowed for workflow %q", workflow)
		}
	case WorkflowOperatorCheckpointV1:
		if phaseSet || profileSet || statePathSet {
			return Task{}, fmt.Errorf("phase, profile, and autonomous_state_path are not allowed for workflow %q", workflow)
		}
		if lineageSet {
			return Task{}, fmt.Errorf("autonomous child lineage is not allowed for workflow %q", workflow)
		}
		if status != StatusPending && status != StatusCompleted {
			return Task{}, fmt.Errorf("invalid status %q for workflow %q", status, workflow)
		}
		if !receiptPathSet || receiptPath == "" {
			return Task{}, fmt.Errorf("frontmatter key %q is required for workflow %q", "checkpoint_receipt_path", workflow)
		}
		if _, err := operatorcheckpoint.ValidateCanonicalReceiptPath(repositoryRoot, taskID, receiptPath); err != nil {
			return Task{}, err
		}
		if status == StatusPending && receiptSHA256Set {
			return Task{}, fmt.Errorf("frontmatter key %q is not allowed for pending workflow %q", "checkpoint_receipt_sha256", workflow)
		}
		if status == StatusCompleted && (!receiptSHA256Set || !operatorcheckpoint.ValidSHA256(receiptSHA256)) {
			return Task{}, fmt.Errorf("frontmatter key %q must be a lowercase SHA-256 for completed workflow %q", "checkpoint_receipt_sha256", workflow)
		}
	}

	return Task{
		ID:                      taskID,
		Title:                   title,
		Profile:                 profile,
		Status:                  status,
		Workflow:                workflow,
		Phase:                   phase,
		AutonomousStatePath:     statePath,
		CheckpointReceiptPath:   receiptPath,
		CheckpointReceiptSHA256: receiptSHA256,
		Priority:                priority,
		HasPriority:             hasPriority,
		DependsOn:               dependsOn,
		Tags:                    tags,
		Conflicts:               conflicts,
		ParentTaskID:            parentTaskID,
		ChildProposalID:         childProposalID,
		ChildDecisionID:         childDecisionID,
		ChildRunID:              childRunID,
		ChildEvidence:           childEvidence,
		ParentBehavior:          parentBehavior,
		ContextBody:             string(raw),
		SourcePath:              sourcePath,
		SourceBytes:             append([]byte(nil), raw...),
	}, nil
}

func parseFrontmatter(lines []string, sourcePath string) (map[string]string, int, error) {
	meta := map[string]string{}
	seen := map[string]struct{}{}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, 0, nil
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			return meta, i + 1, nil
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rawKey, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, 0, fmt.Errorf("invalid frontmatter line %d: expected key: value", i+1)
		}
		rawKey = strings.TrimSpace(rawKey)
		key := strings.ToLower(rawKey)
		value = trimScalar(strings.TrimSpace(value))
		recognized := recognizedFrontmatterKey(key)
		if recognized || validExtensionKey(key) {
			if _, exists := seen[key]; exists {
				return nil, 0, fmt.Errorf("duplicate frontmatter key %q", key)
			}
			seen[key] = struct{}{}
			if recognized {
				meta[key] = value
			}
			continue
		}
		return nil, 0, fmt.Errorf("unsupported frontmatter key %q at %s:%d", rawKey, filepath.ToSlash(filepath.Clean(sourcePath)), i+1)
	}
	return nil, 0, errors.New("unterminated frontmatter")
}

func recognizedFrontmatterKey(key string) bool {
	switch key {
	case "id", "profile", "status", "priority", "workflow", "phase", "autonomous_state_path", "checkpoint_receipt_path", "checkpoint_receipt_sha256", "depends_on", "tags", "conflicts", "parent_task_id", "child_proposal_id", "child_decision_id", "child_run_id", "child_evidence", "parent_behavior":
		return true
	default:
		return false
	}
}

func validExtensionKey(key string) bool {
	return strings.HasPrefix(key, "x-") && len(key) > len("x-")
}

func parseIdentityList(key, raw string, valid func(string) bool) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	values := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(values))
	for i, value := range values {
		if value == "" || value != strings.TrimSpace(value) || !valid(value) {
			return nil, fmt.Errorf("invalid %s item %q at index %d", key, value, i)
		}
		if _, ok := seen[value]; ok {
			return nil, fmt.Errorf("duplicate %s item %q", key, value)
		}
		seen[value] = struct{}{}
	}
	return values, nil
}

func validSchedulingToken(value string) bool {
	return validTaskID(value)
}

func validEvidenceToken(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e || r == ',' || r == '\\' {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func updateMetadataBytes(raw []byte, update MetadataUpdate) ([]byte, error) {
	lines := splitRawLines(raw)
	if len(lines) > 0 && strings.TrimSpace(string(lines[0].content)) == "---" {
		return updateMetadataInFrontmatter(lines, update)
	}

	eol := preferredLineEnding(lines)
	var out bytes.Buffer
	out.WriteString("---")
	out.Write(eol)
	writeMetadataUpdate(&out, update, eol)
	out.WriteString("---")
	out.Write(eol)
	out.Write(eol)
	out.Write(raw)
	return out.Bytes(), nil
}

type rawLine struct {
	content []byte
	ending  []byte
}

func updateMetadataInFrontmatter(lines []rawLine, update MetadataUpdate) ([]byte, error) {
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i].content)) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, errors.New("unterminated frontmatter")
	}

	var out bytes.Buffer
	writeRawLine(&out, lines[0])
	replacedStatus := update.Status == ""
	replacedPhase := update.Phase == ""
	for i := 1; i < end; i++ {
		switch frontmatterKey(string(lines[i].content)) {
		case "status":
			if update.Status != "" {
				out.WriteString("status: " + update.Status)
				out.Write(lines[i].ending)
				replacedStatus = true
				continue
			}
		case "phase":
			if update.Phase != "" {
				out.WriteString("phase: " + update.Phase)
				out.Write(lines[i].ending)
				replacedPhase = true
				continue
			}
		}
		writeRawLine(&out, lines[i])
	}
	eol := preferredLineEnding(lines)
	if !replacedStatus {
		out.WriteString("status: " + update.Status)
		out.Write(eol)
	}
	if !replacedPhase {
		out.WriteString("phase: " + update.Phase)
		out.Write(eol)
	}
	for i := end; i < len(lines); i++ {
		writeRawLine(&out, lines[i])
	}
	return out.Bytes(), nil
}

func fulfillOperatorCheckpointBytes(raw []byte, receiptSHA256 string) ([]byte, error) {
	lines := splitRawLines(raw)
	if len(lines) == 0 || strings.TrimSpace(string(lines[0].content)) != "---" {
		return nil, errors.New("checkpoint task has no frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i].content)) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, errors.New("unterminated frontmatter")
	}

	var out bytes.Buffer
	writeRawLine(&out, lines[0])
	replacedStatus := false
	insertedReceiptIdentity := false
	eol := preferredLineEnding(lines)
	for i := 1; i < end; i++ {
		switch frontmatterKey(string(lines[i].content)) {
		case "status":
			out.WriteString("status: " + StatusCompleted)
			out.Write(lines[i].ending)
			replacedStatus = true
			continue
		case "checkpoint_receipt_path":
			writeRawLine(&out, lines[i])
			out.WriteString("checkpoint_receipt_sha256: " + receiptSHA256)
			out.Write(eol)
			insertedReceiptIdentity = true
			continue
		}
		writeRawLine(&out, lines[i])
	}
	if !replacedStatus {
		return nil, errors.New("checkpoint task has no status metadata")
	}
	if !insertedReceiptIdentity {
		return nil, errors.New("checkpoint task has no receipt path metadata")
	}
	for i := end; i < len(lines); i++ {
		writeRawLine(&out, lines[i])
	}
	return out.Bytes(), nil
}

func writeMetadataUpdate(out *bytes.Buffer, update MetadataUpdate, eol []byte) {
	if update.Status != "" {
		fmt.Fprintf(out, "status: %s", update.Status)
		out.Write(eol)
	}
	if update.Phase != "" {
		fmt.Fprintf(out, "phase: %s", update.Phase)
		out.Write(eol)
	}
}

func splitRawLines(raw []byte) []rawLine {
	lines := make([]rawLine, 0, bytes.Count(raw, []byte{'\n'})+1)
	start := 0
	for i := 0; i < len(raw); i++ {
		endingSize := 0
		switch raw[i] {
		case '\n':
			endingSize = 1
		case '\r':
			endingSize = 1
			if i+1 < len(raw) && raw[i+1] == '\n' {
				endingSize = 2
			}
		}
		if endingSize == 0 {
			continue
		}
		lines = append(lines, rawLine{
			content: raw[start:i],
			ending:  raw[i : i+endingSize],
		})
		i += endingSize - 1
		start = i + 1
	}
	if start < len(raw) || len(raw) == 0 {
		lines = append(lines, rawLine{content: raw[start:]})
	}
	return lines
}

func preferredLineEnding(lines []rawLine) []byte {
	for _, line := range lines {
		if len(line.ending) > 0 {
			return line.ending
		}
	}
	return []byte{'\n'}
}

func writeRawLine(out *bytes.Buffer, line rawLine) {
	out.Write(line.content)
	out.Write(line.ending)
}

func frontmatterKey(line string) string {
	key, _, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(key))
}

func normalizeLineEndings(markdown string) string {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	return strings.ReplaceAll(normalized, "\r", "\n")
}

func trimScalar(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func findH1Title(lines []string) (string, error) {
	for _, line := range lines {
		heading, ok := parseHeading(line)
		if !ok || heading.level != 1 {
			continue
		}
		if heading.text == "" {
			return "", errors.New("task file has empty H1 title")
		}
		return heading.text, nil
	}
	return "", errors.New("task file has no H1 title")
}

func splitLines(markdown string) []string {
	return strings.Split(normalizeLineEndings(markdown), "\n")
}

type heading struct {
	level int
	text  string
}

func parseHeading(line string) (heading, bool) {
	leftTrimmed := strings.TrimLeft(line, " ")
	if len(line)-len(leftTrimmed) > 3 || !strings.HasPrefix(leftTrimmed, "#") {
		return heading{}, false
	}

	level := 0
	for level < len(leftTrimmed) && leftTrimmed[level] == '#' {
		level++
	}
	if level > 6 || level == len(leftTrimmed) {
		return heading{}, false
	}
	if leftTrimmed[level] != ' ' && leftTrimmed[level] != '\t' {
		return heading{}, false
	}

	text := strings.TrimSpace(leftTrimmed[level:])
	return heading{level: level, text: stripClosingHashes(text)}, true
}

func stripClosingHashes(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasSuffix(text, "#") {
		return text
	}

	lastNonHash := len(text) - 1
	for lastNonHash >= 0 && text[lastNonHash] == '#' {
		lastNonHash--
	}
	if lastNonHash < 0 || (text[lastNonHash] != ' ' && text[lastNonHash] != '\t') {
		return text
	}
	return strings.TrimSpace(text[:lastNonHash])
}

func validStatus(status string) bool {
	switch status {
	case StatusPending, StatusRunning, StatusCompleted, StatusBlocked, StatusCancelled, StatusSuperseded, StatusAbandoned:
		return true
	default:
		return false
	}
}

func terminalArchiveStatus(status string) bool {
	return status == StatusCompleted || status == StatusCancelled || status == StatusSuperseded || status == StatusAbandoned
}

func rewriteReopenMetadata(raw []byte, newTaskID string) ([]byte, error) {
	lines := splitRawLines(raw)
	if len(lines) == 0 || strings.TrimSpace(string(lines[0].content)) != "---" {
		return nil, errors.New("terminal autonomous task must have frontmatter")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i].content)) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, errors.New("unterminated frontmatter")
	}
	statePath := path.Join(".revolvr", "autonomous", "tasks", newTaskID, "state.json")
	eol := preferredLineEnding(lines)
	replaced := map[string]bool{"id": false, "status": false, "autonomous_state_path": false}
	var out bytes.Buffer
	writeRawLine(&out, lines[0])
	for i := 1; i < end; i++ {
		key := frontmatterKey(string(lines[i].content))
		value := ""
		switch key {
		case "id":
			value = newTaskID
		case "status":
			value = StatusPending
		case "autonomous_state_path":
			value = statePath
		}
		if value != "" {
			out.WriteString(key + ": " + value)
			out.Write(lines[i].ending)
			replaced[key] = true
			continue
		}
		writeRawLine(&out, lines[i])
	}
	for _, key := range []string{"id", "status", "autonomous_state_path"} {
		if replaced[key] {
			continue
		}
		value := newTaskID
		if key == "status" {
			value = StatusPending
		} else if key == "autonomous_state_path" {
			value = statePath
		}
		out.WriteString(key + ": " + value)
		out.Write(eol)
	}
	for i := end; i < len(lines); i++ {
		writeRawLine(&out, lines[i])
	}
	return out.Bytes(), nil
}

func rewriteAutonomousMigrationMetadata(raw []byte, taskID string) ([]byte, error) {
	lines := splitRawLines(raw)
	statePath := path.Join(".revolvr", "autonomous", "tasks", taskID, "state.json")
	eol := preferredLineEnding(lines)
	if len(lines) == 0 || strings.TrimSpace(string(lines[0].content)) != "---" {
		var out bytes.Buffer
		out.WriteString("---")
		out.Write(eol)
		out.WriteString("workflow: " + WorkflowAutonomousV1)
		out.Write(eol)
		out.WriteString("autonomous_state_path: " + statePath)
		out.Write(eol)
		out.WriteString("---")
		out.Write(eol)
		out.Write(raw)
		return out.Bytes(), nil
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i].content)) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, errors.New("unterminated frontmatter")
	}

	var out bytes.Buffer
	writeRawLine(&out, lines[0])
	wroteWorkflow := false
	for i := 1; i < end; i++ {
		switch frontmatterKey(string(lines[i].content)) {
		case "workflow":
			out.WriteString("workflow: " + WorkflowAutonomousV1)
			out.Write(lines[i].ending)
			wroteWorkflow = true
		case "phase", "profile":
			// These fields route mixed-pass phases and have no autonomous-v1
			// representation. All other authored bytes remain untouched.
		default:
			writeRawLine(&out, lines[i])
		}
	}
	if !wroteWorkflow {
		out.WriteString("workflow: " + WorkflowAutonomousV1)
		out.Write(eol)
	}
	out.WriteString("autonomous_state_path: " + statePath)
	out.Write(eol)
	for i := end; i < len(lines); i++ {
		writeRawLine(&out, lines[i])
	}
	return out.Bytes(), nil
}

func syncTaskDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func validWorkflow(workflow string) bool {
	return workflow == WorkflowMixedPassV1 || workflow == WorkflowAutonomousV1 || workflow == WorkflowOperatorCheckpointV1
}

func validateAutonomousStatePath(repositoryRoot string, taskID string, statePath string) error {
	expected := path.Join(".revolvr", "autonomous", "tasks", taskID, "state.json")
	if statePath != expected {
		return fmt.Errorf("invalid autonomous_state_path %q for task %q: must be %q", statePath, taskID, expected)
	}
	if _, err := pathguard.Resolve(repositoryRoot, filepath.FromSlash(statePath)); err != nil {
		return fmt.Errorf("invalid autonomous_state_path %q for task %q: %w", statePath, taskID, err)
	}
	current := repositoryRoot
	for _, component := range strings.Split(filepath.FromSlash(statePath), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid autonomous_state_path %q for task %q: inspect path component: %w", statePath, taskID, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid autonomous_state_path %q for task %q: path component %s is a symbolic link", statePath, taskID, component)
		}
	}
	return nil
}

func validPhase(phase string) bool {
	switch phase {
	case PhaseImplement, PhaseAudit, PhaseDocument, PhaseSimplify:
		return true
	default:
		return false
	}
}

func validTaskID(taskID string) bool {
	if taskID == "" || taskID == "." || taskID == ".." {
		return false
	}
	for _, r := range taskID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func validProfileName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func repositoryRootAbs(repositoryRoot string) (string, error) {
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		repositoryRoot = "."
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	return root, nil
}

func writeFileAtomically(path string, content []byte, defaultPerm os.FileMode) error {
	perm := defaultPerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(perm); err != nil {
		return err
	}
	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	return os.Rename(tempPath, path)
}

func resolveTaskPath(root string, path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", errors.New("task file path is required")
	}
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(root, absPath)
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve task file path: %w", err)
	}

	taskDir := filepath.Join(root, TasksDir)
	rel, err := filepath.Rel(taskDir, absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve task file path relative to %s: %w", TasksDir, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("task file path %s is outside %s", path, TasksDir)
	}
	if err := validateResolvedTaskPath(root, taskDir, absPath, path); err != nil {
		return "", "", err
	}
	return filepath.Join(TasksDir, rel), absPath, nil
}

func validateResolvedTaskPath(root string, taskDir string, absPath string, displayPath string) error {
	resolvedTaskDir, err := validatedResolvedTaskDirectory(root, taskDir)
	if err != nil {
		return err
	}
	if resolvedTaskDir == "" {
		return nil
	}

	if info, err := os.Lstat(absPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("task file path %s is a symbolic link", displayPath)
		}
		resolvedPath, resolveErr := filepath.EvalSymlinks(absPath)
		if resolveErr != nil {
			return fmt.Errorf("resolve task file path %s symlinks: %w", displayPath, resolveErr)
		}
		if !pathWithin(resolvedTaskDir, resolvedPath) {
			return fmt.Errorf("task file path %s resolves outside %s", displayPath, TasksDir)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect task file path %s: %w", displayPath, err)
	}

	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("resolve task file parent for %s: %w", displayPath, err)
	}
	if !pathWithin(resolvedTaskDir, resolvedParent) {
		return fmt.Errorf("task file path %s resolves outside %s", displayPath, TasksDir)
	}
	return nil
}

func validateResolvedTaskDirectory(root string, taskDir string) error {
	_, err := validatedResolvedTaskDirectory(root, taskDir)
	return err
}

func validatedResolvedTaskDirectory(root string, taskDir string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolvedTaskDir, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve %s symlinks: %w", TasksDir, err)
		}
		resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(taskDir))
		if parentErr != nil {
			if errors.Is(parentErr, os.ErrNotExist) {
				return "", nil
			}
			return "", fmt.Errorf("resolve %s parent symlinks: %w", TasksDir, parentErr)
		}
		resolvedTaskDir = filepath.Join(resolvedParent, filepath.Base(taskDir))
	}
	if !pathWithin(resolvedRoot, resolvedTaskDir) {
		return "", fmt.Errorf("task directory %s resolves outside repository root", TasksDir)
	}
	return resolvedTaskDir, nil
}

func pathWithin(base string, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
