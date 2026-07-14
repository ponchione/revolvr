package autonomousmigration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/autonomousstate"
	"revolvr/internal/lock"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

const JournalSchemaVersion = "autonomous-migration-v1"
const HistorySchemaVersion = "autonomous-migration-transition-v1"

const migrationsPath = ".revolvr/autonomous/migrations"

type Stage string

const (
	StageAdmitted        Stage = "admitted"
	StageStatesPublished Stage = "states_published"
	StageTasksPublished  Stage = "tasks_published"
	StageCompleted       Stage = "completed"
)

type FailurePoint string

const (
	FailureAfterLock             FailurePoint = "after_lock"
	FailureBeforeMaterial        FailurePoint = "before_material"
	FailureMaterialFileSync      FailurePoint = "material_file_sync"
	FailureMaterialLink          FailurePoint = "material_link"
	FailureMaterialDirectorySync FailurePoint = "material_directory_sync"
	FailureAfterMaterial         FailurePoint = "after_material"
	FailureBeforeHistory         FailurePoint = "before_history"
	FailureHistoryFileSync       FailurePoint = "history_file_sync"
	FailureHistoryLink           FailurePoint = "history_link"
	FailureHistoryDirectorySync  FailurePoint = "history_directory_sync"
	FailureAfterHistory          FailurePoint = "after_history"
	FailureBeforeCheckpoint      FailurePoint = "before_checkpoint"
	FailureCheckpointFileSync    FailurePoint = "checkpoint_file_sync"
	FailureCheckpointRename      FailurePoint = "checkpoint_rename"
	FailureCheckpointDirSync     FailurePoint = "checkpoint_directory_sync"
	FailureAfterCheckpoint       FailurePoint = "after_checkpoint"
	FailureBeforeState           FailurePoint = "before_state"
	FailureStateFileSync         FailurePoint = "state_file_sync"
	FailureStateLink             FailurePoint = "state_link"
	FailureStateDirectorySync    FailurePoint = "state_directory_sync"
	FailureAfterState            FailurePoint = "after_state"
	FailureBeforeTask            FailurePoint = "before_task"
	FailureAfterTask             FailurePoint = "after_task"
)

type FailureInjector func(FailurePoint) error

type Record struct {
	TaskID                string `json:"task_id"`
	SourcePath            string `json:"source_path"`
	SourceSHA256          string `json:"source_sha256"`
	SourceByteSize        int    `json:"source_byte_size"`
	SourceArtifactPath    string `json:"source_artifact_path"`
	ProjectedSHA256       string `json:"projected_sha256"`
	ProjectedByteSize     int    `json:"projected_byte_size"`
	ProjectedArtifactPath string `json:"projected_artifact_path"`
	StatePath             string `json:"state_path"`
	StateSHA256           string `json:"state_sha256"`
	StateByteSize         int    `json:"state_byte_size"`
	StateArtifactPath     string `json:"state_artifact_path"`
}

type Journal struct {
	SchemaVersion  string    `json:"schema_version"`
	OperationID    string    `json:"operation_id"`
	MaterialSHA256 string    `json:"material_sha256"`
	Stage          Stage     `json:"stage"`
	Sequence       int64     `json:"sequence"`
	Records        []Record  `json:"records"`
	CreatedAt      time.Time `json:"created_at"`
}

type HistoryRecord struct {
	SchemaVersion string  `json:"schema_version"`
	Journal       Journal `json:"journal"`
}

type ApplyInput struct {
	RepositoryRoot  string
	Plan            Plan
	CreatedAt       time.Time
	FailureInjector FailureInjector
}

type ApplyResult struct {
	Plan        Plan
	OperationID string
	Stage       Stage
	Replayed    bool
}

func (j Journal) Validate() error {
	if j.SchemaVersion != JournalSchemaVersion || !validOperationID(j.OperationID) || !validSHA256(j.MaterialSHA256) || j.CreatedAt.IsZero() || j.CreatedAt.Location() != time.UTC {
		return errors.New("autonomous migration journal: invalid schema, identity, material, or creation time")
	}
	wantStage, ok := stageForSequence(j.Sequence)
	if !ok || wantStage != j.Stage || len(j.Records) == 0 {
		return errors.New("autonomous migration journal: stage, sequence, or record set is invalid")
	}
	for i, record := range j.Records {
		if err := validateRecord(j.OperationID, record); err != nil {
			return fmt.Errorf("autonomous migration journal: records[%d]: %w", i, err)
		}
		if i > 0 && recordOrder(j.Records[i-1]) >= recordOrder(record) {
			return errors.New("autonomous migration journal: records are not strictly ordered")
		}
	}
	material, err := materialHash(j.Records)
	if err != nil || material != j.MaterialSHA256 || j.OperationID != operationID(material) {
		return errors.Join(err, errors.New("autonomous migration journal: material identity mismatch"))
	}
	return nil
}

func (j Journal) SameAuthority(other Journal) bool {
	other.Stage = j.Stage
	other.Sequence = j.Sequence
	other.CreatedAt = j.CreatedAt
	return reflect.DeepEqual(j, other)
}

func ValidateTransition(previous, next Journal) error {
	if err := previous.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if !previous.SameAuthority(next) || next.Sequence != previous.Sequence+1 {
		return errors.New("autonomous migration journal: authority changed or transition is noncontiguous")
	}
	return nil
}

// Apply publishes one already validated plan. Immutable history is the
// recovery authority; the journal file is only a replaceable checkpoint.
func Apply(ctx context.Context, input ApplyInput) (ApplyResult, error) {
	root, err := runtimepath.CanonicalRoot(strings.TrimSpace(input.RepositoryRoot))
	if err != nil {
		return ApplyResult{}, err
	}
	if input.CreatedAt.IsZero() {
		return ApplyResult{}, errors.New("apply autonomous migration: explicit creation time is required")
	}
	if err := validatePlan(root, input.Plan); err != nil {
		return ApplyResult{}, err
	}
	expected, err := journalForPlan(input.Plan, input.CreatedAt.UTC())
	if err != nil {
		return ApplyResult{}, err
	}
	unlock, err := acquireMigrationLock(ctx, root)
	if err != nil {
		return ApplyResult{}, err
	}
	defer unlock()
	if err := fail(input.FailureInjector, FailureAfterLock); err != nil {
		return ApplyResult{}, err
	}

	journal, found, err := Load(root, expected.OperationID)
	if err != nil {
		return ApplyResult{}, err
	}
	if found {
		if !journal.SameAuthority(expected) {
			return ApplyResult{}, errors.New("apply autonomous migration: operation identity conflicts with durable authority")
		}
		if journal.Stage == StageCompleted {
			if err := validateCompleted(root, input.Plan); err != nil {
				return ApplyResult{}, err
			}
			return ApplyResult{Plan: input.Plan, OperationID: journal.OperationID, Stage: journal.Stage, Replayed: true}, nil
		}
	} else {
		if err := validateBeforeAdmission(root, input.Plan); err != nil {
			return ApplyResult{}, err
		}
		if err := writeMaterials(root, expected, input.Plan, input.FailureInjector); err != nil {
			return ApplyResult{}, err
		}
		journal = expected
		if err := persist(root, Journal{}, journal, input.FailureInjector); err != nil {
			return ApplyResult{}, err
		}
	}

	if journal.Stage == StageAdmitted {
		for i, entry := range input.Plan.Entries {
			if err := fail(input.FailureInjector, FailureBeforeState); err != nil {
				return ApplyResult{}, err
			}
			if err := writeImmutable(root, filepath.Join(root, filepath.FromSlash(entry.AutonomousStatePath)), entry.StateBytes, 0o644, immutableFailurePoints{
				fileSync: FailureStateFileSync, link: FailureStateLink, dirSync: FailureStateDirectorySync,
			}, input.FailureInjector); err != nil {
				return ApplyResult{}, fmt.Errorf("apply autonomous migration: publish state for %q: %w", entry.TaskID, err)
			}
			if err := validateTaskNamespace(root, entry, true); err != nil {
				return ApplyResult{}, err
			}
			if err := fail(input.FailureInjector, FailureAfterState); err != nil {
				return ApplyResult{}, fmt.Errorf("apply autonomous migration: state %d/%d published: %w", i+1, len(input.Plan.Entries), err)
			}
		}
		previous := journal
		journal.Stage, journal.Sequence = StageStatesPublished, journal.Sequence+1
		if err := persist(root, previous, journal, input.FailureInjector); err != nil {
			return ApplyResult{}, err
		}
	}

	if journal.Stage == StageStatesPublished {
		for i, entry := range input.Plan.Entries {
			if err := fail(input.FailureInjector, FailureBeforeTask); err != nil {
				return ApplyResult{}, err
			}
			if _, _, err := taskfile.PublishAutonomousMigration(root, entry.SourceTask, entry.ProjectedTask); err != nil {
				return ApplyResult{}, err
			}
			if err := fail(input.FailureInjector, FailureAfterTask); err != nil {
				return ApplyResult{}, fmt.Errorf("apply autonomous migration: task %d/%d published: %w", i+1, len(input.Plan.Entries), err)
			}
		}
		previous := journal
		journal.Stage, journal.Sequence = StageTasksPublished, journal.Sequence+1
		if err := persist(root, previous, journal, input.FailureInjector); err != nil {
			return ApplyResult{}, err
		}
	}

	if journal.Stage == StageTasksPublished {
		if err := validateInitialPublication(root, input.Plan); err != nil {
			return ApplyResult{}, err
		}
		previous := journal
		journal.Stage, journal.Sequence = StageCompleted, journal.Sequence+1
		if err := persist(root, previous, journal, input.FailureInjector); err != nil {
			return ApplyResult{}, err
		}
	}
	return ApplyResult{Plan: input.Plan, OperationID: journal.OperationID, Stage: journal.Stage}, nil
}

// FindPlan locates a durable migration selected by the same task IDs and
// reconstructs its exact plan from immutable material artifacts. It is used
// before fresh planning so partially published task metadata remains
// recoverable.
func FindPlan(ctx context.Context, repositoryRoot string, request Request) (Plan, Journal, bool, error) {
	root, err := runtimepath.CanonicalRoot(strings.TrimSpace(repositoryRoot))
	if err != nil {
		return Plan{}, Journal{}, false, err
	}
	authorities, err := loadAuthorities(ctx, root)
	if err != nil || len(authorities) == 0 {
		return Plan{}, Journal{}, false, err
	}
	wantIDs, err := normalizedRequestedIDs(request.TaskIDs)
	if err != nil {
		return Plan{}, Journal{}, false, err
	}
	var matches []Journal
	var incomplete []Journal
	for _, authority := range authorities {
		if authority.Stage != StageCompleted {
			incomplete = append(incomplete, authority)
		}
		if request.All || equalStrings(wantIDs, journalTaskIDs(authority)) {
			matches = append(matches, authority)
		}
	}
	if len(incomplete) != 0 {
		var selected []Journal
		for _, candidate := range matches {
			if candidate.Stage != StageCompleted {
				selected = append(selected, candidate)
			}
		}
		if len(selected) != 1 {
			return Plan{}, Journal{}, false, fmt.Errorf("find autonomous migration: incomplete operation %s must be recovered with its exact task set", incomplete[0].OperationID)
		}
		matches = selected
	} else if request.All {
		tasks, loadErr := taskfile.LoadAll(root)
		if loadErr != nil {
			return Plan{}, Journal{}, false, loadErr
		}
		for _, task := range tasks {
			if task.Workflow == taskfile.WorkflowMixedPassV1 {
				return Plan{}, Journal{}, false, nil
			}
		}
		if len(matches) > 1 {
			return Plan{}, Journal{}, false, nil
		}
	}
	if len(matches) == 0 {
		return Plan{}, Journal{}, false, nil
	}
	if len(matches) != 1 {
		return Plan{}, Journal{}, false, errors.New("find autonomous migration: selection matches more than one durable operation")
	}
	plan, err := planFromJournal(root, matches[0])
	if err != nil {
		return Plan{}, Journal{}, false, err
	}
	return plan, matches[0], true, nil
}

func Load(repositoryRoot, operationID string) (Journal, bool, error) {
	root, err := runtimepath.CanonicalRoot(repositoryRoot)
	if err != nil {
		return Journal{}, false, err
	}
	if !validOperationID(operationID) {
		return Journal{}, false, errors.New("load autonomous migration: malformed operation ID")
	}
	base := filepath.Join(root, filepath.FromSlash(migrationsPath))
	if err := runtimepath.CheckDir(root, base, true); err != nil {
		return Journal{}, false, err
	}
	checkpoint, checkpointFound, err := readJournal(root, filepath.Join(base, operationID+".json"), operationID)
	if err != nil {
		return Journal{}, false, err
	}
	history, historyFound, err := readHistory(root, filepath.Join(base, "history"), operationID)
	if err != nil {
		return Journal{}, false, err
	}
	if !historyFound {
		if checkpointFound {
			return Journal{}, false, errors.New("load autonomous migration: checkpoint exists without immutable history")
		}
		return Journal{}, false, nil
	}
	latest := history[len(history)-1]
	if checkpointFound {
		if checkpoint.Sequence > latest.Sequence || !reflect.DeepEqual(checkpoint, history[checkpoint.Sequence-1]) {
			return Journal{}, false, errors.New("load autonomous migration: checkpoint conflicts with immutable history")
		}
	}
	return latest, true, nil
}

func validatePlan(root string, plan Plan) error {
	if plan.SchemaVersion != PlanSchemaVersion || plan.TargetWorkflow != taskfile.WorkflowAutonomousV1 || plan.DryRun || len(plan.Entries) == 0 {
		return errors.New("apply autonomous migration: a non-dry-run current-schema plan is required")
	}
	seen := make(map[string]struct{}, len(plan.Entries))
	for i, entry := range plan.Entries {
		if entry.TaskID == "" || entry.SourceTask.ID != entry.TaskID || entry.ProjectedTask.ID != entry.TaskID || entry.SourcePath != filepath.ToSlash(entry.SourceTask.SourcePath) || entry.ProjectedTask.SourcePath != entry.SourceTask.SourcePath {
			return fmt.Errorf("apply autonomous migration: entries[%d] has inconsistent task identity", i)
		}
		if _, exists := seen[entry.TaskID]; exists {
			return errors.New("apply autonomous migration: duplicate task identity")
		}
		seen[entry.TaskID] = struct{}{}
		if i > 0 && entryOrder(plan.Entries[i-1]) >= entryOrder(entry) {
			return errors.New("apply autonomous migration: entries are not strictly ordered")
		}
		if hashBytes(entry.SourceTask.SourceBytes) != entry.SourceSHA256 || len(entry.SourceTask.SourceBytes) != entry.SourceByteSize || hashBytes(entry.ProjectedTask.SourceBytes) != entry.ProjectedSHA256 || len(entry.ProjectedTask.SourceBytes) != entry.ProjectedByteSize || hashBytes(entry.StateBytes) != entry.StateSHA256 || len(entry.StateBytes) != entry.StateByteSize {
			return fmt.Errorf("apply autonomous migration: entries[%d] byte identity mismatch", i)
		}
		wantProjected, err := taskfile.ProjectAutonomousMigration(root, entry.SourceTask)
		if err != nil || !bytes.Equal(wantProjected.SourceBytes, entry.ProjectedTask.SourceBytes) {
			return errors.Join(err, fmt.Errorf("apply autonomous migration: entries[%d] projection mismatch", i))
		}
		state, err := autonomousstate.DecodeState(entry.StateBytes, entry.TaskID)
		if err != nil || !reflect.DeepEqual(state, initialState(entry.TaskID)) || !reflect.DeepEqual(state, entry.AutonomousState) || entry.AutonomousStatePath != entry.ProjectedTask.AutonomousStatePath {
			return errors.Join(err, fmt.Errorf("apply autonomous migration: entries[%d] state mismatch", i))
		}
	}
	return nil
}

func validateBeforeAdmission(root string, plan Plan) error {
	for _, entry := range plan.Entries {
		current, err := taskfile.Load(root, entry.SourcePath)
		if err != nil {
			return err
		}
		if !bytes.Equal(current.SourceBytes, entry.SourceTask.SourceBytes) {
			return fmt.Errorf("apply autonomous migration: task %q changed after batch planning", entry.TaskID)
		}
		if err := validateTaskNamespace(root, entry, false); err != nil {
			return err
		}
	}
	return nil
}

func validateTaskNamespace(root string, entry Entry, requireState bool) error {
	statePath := filepath.Join(root, filepath.FromSlash(entry.AutonomousStatePath))
	namespace := filepath.Dir(statePath)
	entries, found, err := runtimepath.ReadDir(root, namespace, true)
	if err != nil {
		return err
	}
	if !found {
		if requireState {
			return fmt.Errorf("apply autonomous migration: canonical state for %q is missing", entry.TaskID)
		}
		return nil
	}
	if len(entries) == 0 {
		return fmt.Errorf("apply autonomous migration: autonomous namespace for %q exists without the exact migration state", entry.TaskID)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(statePath) || entries[0].IsDir() {
		return fmt.Errorf("apply autonomous migration: autonomous namespace for %q contains conflicting user evidence", entry.TaskID)
	}
	raw, stateFound, err := runtimepath.ReadFile(root, statePath, false)
	if err != nil || !stateFound {
		return errors.Join(err, fmt.Errorf("apply autonomous migration: canonical state for %q is missing", entry.TaskID))
	}
	if !bytes.Equal(raw, entry.StateBytes) {
		return fmt.Errorf("apply autonomous migration: canonical state for %q conflicts with the exact migration state", entry.TaskID)
	}
	return nil
}

func validateInitialPublication(root string, plan Plan) error {
	for _, entry := range plan.Entries {
		current, err := taskfile.Load(root, entry.SourcePath)
		if err != nil || !bytes.Equal(current.SourceBytes, entry.ProjectedTask.SourceBytes) {
			return errors.Join(err, fmt.Errorf("apply autonomous migration: task %q publication readback mismatch", entry.TaskID))
		}
		if err := validateTaskNamespace(root, entry, true); err != nil {
			return err
		}
	}
	return nil
}

func validateCompleted(root string, plan Plan) error {
	for _, entry := range plan.Entries {
		current, err := taskfile.Load(root, entry.SourcePath)
		if err != nil {
			return err
		}
		if current.ID != entry.TaskID || current.Workflow != taskfile.WorkflowAutonomousV1 || current.AutonomousStatePath != entry.AutonomousStatePath {
			return fmt.Errorf("apply autonomous migration: completed task %q no longer matches migration authority", entry.TaskID)
		}
		if !bytes.Equal(current.SourceBytes, entry.ProjectedTask.SourceBytes) {
			statusOnly, projectErr := taskfile.ProjectMetadataFromSnapshot(root, entry.ProjectedTask, taskfile.MetadataUpdate{Status: current.Status})
			if projectErr != nil || !bytes.Equal(statusOnly.SourceBytes, current.SourceBytes) {
				return errors.Join(projectErr, fmt.Errorf("apply autonomous migration: completed task %q has conflicting metadata", entry.TaskID))
			}
		}
		statePath := filepath.Join(root, filepath.FromSlash(entry.AutonomousStatePath))
		raw, found, err := runtimepath.ReadFile(root, statePath, false)
		if err != nil || !found {
			return errors.Join(err, fmt.Errorf("apply autonomous migration: completed task %q has no canonical state", entry.TaskID))
		}
		if _, err := autonomousstate.DecodeState(raw, entry.TaskID); err != nil {
			return fmt.Errorf("apply autonomous migration: completed task %q state is invalid: %w", entry.TaskID, err)
		}
	}
	return nil
}

func journalForPlan(plan Plan, createdAt time.Time) (Journal, error) {
	records := make([]Record, len(plan.Entries))
	for i, entry := range plan.Entries {
		records[i] = Record{
			TaskID: entry.TaskID, SourcePath: entry.SourcePath, SourceSHA256: entry.SourceSHA256, SourceByteSize: entry.SourceByteSize,
			ProjectedSHA256: entry.ProjectedSHA256, ProjectedByteSize: entry.ProjectedByteSize,
			StatePath: entry.AutonomousStatePath, StateSHA256: entry.StateSHA256, StateByteSize: entry.StateByteSize,
		}
	}
	material, err := materialHash(records)
	if err != nil {
		return Journal{}, err
	}
	op := operationID(material)
	for i := range records {
		base := path.Join(migrationsPath, "materials", op, records[i].TaskID)
		records[i].SourceArtifactPath = base + ".source.md"
		records[i].ProjectedArtifactPath = base + ".projected.md"
		records[i].StateArtifactPath = base + ".state.json"
	}
	journal := Journal{SchemaVersion: JournalSchemaVersion, OperationID: op, MaterialSHA256: material, Stage: StageAdmitted, Sequence: 1, Records: records, CreatedAt: createdAt.UTC()}
	return journal, journal.Validate()
}

func materialHash(records []Record) (string, error) {
	type materialRecord struct {
		TaskID, SourcePath, SourceSHA256, ProjectedSHA256, StatePath, StateSHA256 string
		SourceByteSize, ProjectedByteSize, StateByteSize                          int
	}
	values := make([]materialRecord, len(records))
	for i, record := range records {
		values[i] = materialRecord{record.TaskID, record.SourcePath, record.SourceSHA256, record.ProjectedSHA256, record.StatePath, record.StateSHA256, record.SourceByteSize, record.ProjectedByteSize, record.StateByteSize}
	}
	raw, err := json.Marshal(values)
	return hashBytes(raw), err
}

func writeMaterials(root string, journal Journal, plan Plan, inject FailureInjector) error {
	for i, record := range journal.Records {
		for _, item := range []struct {
			path string
			raw  []byte
		}{
			{record.SourceArtifactPath, plan.Entries[i].SourceTask.SourceBytes},
			{record.ProjectedArtifactPath, plan.Entries[i].ProjectedTask.SourceBytes},
			{record.StateArtifactPath, plan.Entries[i].StateBytes},
		} {
			if err := fail(inject, FailureBeforeMaterial); err != nil {
				return err
			}
			if err := writeImmutable(root, filepath.Join(root, filepath.FromSlash(item.path)), item.raw, 0o600, immutableFailurePoints{fileSync: FailureMaterialFileSync, link: FailureMaterialLink, dirSync: FailureMaterialDirectorySync}, inject); err != nil {
				return err
			}
			if err := fail(inject, FailureAfterMaterial); err != nil {
				return err
			}
		}
	}
	return nil
}

func persist(root string, previous, next Journal, inject FailureInjector) error {
	if previous.Sequence == 0 {
		if err := next.Validate(); err != nil || next.Sequence != 1 {
			return errors.Join(err, errors.New("persist autonomous migration: history must start at admission"))
		}
	} else if err := ValidateTransition(previous, next); err != nil {
		return err
	}
	if err := fail(inject, FailureBeforeHistory); err != nil {
		return err
	}
	historyRaw, err := marshalCanonical(HistoryRecord{SchemaVersion: HistorySchemaVersion, Journal: next})
	if err != nil {
		return err
	}
	historyPath := filepath.Join(root, filepath.FromSlash(path.Join(migrationsPath, "history", historyFilename(next.OperationID, next.Sequence))))
	if err := writeImmutable(root, historyPath, historyRaw, 0o600, immutableFailurePoints{fileSync: FailureHistoryFileSync, link: FailureHistoryLink, dirSync: FailureHistoryDirectorySync}, inject); err != nil {
		return err
	}
	if err := fail(inject, FailureAfterHistory); err != nil {
		return err
	}
	if err := fail(inject, FailureBeforeCheckpoint); err != nil {
		return err
	}
	raw, err := marshalCanonical(next)
	if err != nil {
		return err
	}
	checkpointPath := filepath.Join(root, filepath.FromSlash(path.Join(migrationsPath, next.OperationID+".json")))
	if err := writeMutable(root, checkpointPath, raw, inject); err != nil {
		return err
	}
	return fail(inject, FailureAfterCheckpoint)
}

type immutableFailurePoints struct {
	fileSync FailurePoint
	link     FailurePoint
	dirSync  FailurePoint
}

func writeImmutable(root, target string, raw []byte, mode os.FileMode, points immutableFailurePoints, inject FailureInjector) error {
	dir := filepath.Dir(target)
	if err := runtimepath.EnsureDir(root, dir, 0o700); err != nil {
		return err
	}
	if existing, found, err := runtimepath.ReadFile(root, target, true); err != nil {
		return err
	} else if found {
		if !bytes.Equal(existing, raw) {
			return fmt.Errorf("immutable path %s has different bytes", filepath.ToSlash(target))
		}
		return runtimepath.SyncDir(root, dir)
	}
	file, err := os.CreateTemp(dir, ".migration-immutable-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer removeProtectedTemp(root, name)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, name, file); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := fail(inject, points.fileSync); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := fail(inject, points.link); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, target, true); err != nil {
		return err
	}
	if err := os.Link(name, target); err != nil {
		return err
	}
	if err := os.Remove(name); err != nil {
		return err
	}
	if err := fail(inject, points.dirSync); err != nil {
		return err
	}
	return runtimepath.SyncDir(root, dir)
}

func writeMutable(root, target string, raw []byte, inject FailureInjector) error {
	dir := filepath.Dir(target)
	if err := runtimepath.EnsureDir(root, dir, 0o700); err != nil {
		return err
	}
	if err := runtimepath.CheckFile(root, target, true); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".migration-checkpoint-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer removeProtectedTemp(root, name)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if err := runtimepath.CheckOpenedFile(root, name, file); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := fail(inject, FailureCheckpointFileSync); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := fail(inject, FailureCheckpointRename); err != nil {
		return err
	}
	if err := os.Rename(name, target); err != nil {
		return err
	}
	if err := fail(inject, FailureCheckpointDirSync); err != nil {
		return err
	}
	return runtimepath.SyncDir(root, dir)
}

func readJournal(root, target, operation string) (Journal, bool, error) {
	raw, found, err := runtimepath.ReadFile(root, target, true)
	if err != nil || !found {
		return Journal{}, found, err
	}
	var journal Journal
	if err := decodeCanonical(raw, &journal); err != nil {
		return Journal{}, false, err
	}
	if err := journal.Validate(); err != nil || journal.OperationID != operation {
		return Journal{}, false, errors.Join(err, errors.New("load autonomous migration: checkpoint operation mismatch"))
	}
	return journal, true, nil
}

func readHistory(root, dir, operation string) ([]Journal, bool, error) {
	entries, found, err := runtimepath.ReadDir(root, dir, true)
	if err != nil || !found {
		return nil, found, err
	}
	type item struct {
		name string
		seq  int64
	}
	var matches []item
	for _, entry := range entries {
		sequence, belongs, err := historySequence(entry.Name(), operation)
		if err != nil {
			return nil, false, err
		}
		if belongs {
			if entry.IsDir() {
				return nil, false, errors.New("load autonomous migration: history entry is a directory")
			}
			matches = append(matches, item{entry.Name(), sequence})
		}
	}
	if len(matches) == 0 {
		return nil, false, nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].seq < matches[j].seq })
	result := make([]Journal, len(matches))
	for i, match := range matches {
		sequence := int64(i + 1)
		if match.seq != sequence || match.name != historyFilename(operation, sequence) {
			return nil, false, errors.New("load autonomous migration: immutable history is noncontiguous")
		}
		raw, found, err := runtimepath.ReadFile(root, filepath.Join(dir, match.name), false)
		if err != nil || !found {
			return nil, false, err
		}
		var record HistoryRecord
		if err := decodeCanonical(raw, &record); err != nil {
			return nil, false, err
		}
		if record.SchemaVersion != HistorySchemaVersion || record.Journal.OperationID != operation || record.Journal.Sequence != sequence {
			return nil, false, errors.New("load autonomous migration: invalid immutable history record")
		}
		if err := record.Journal.Validate(); err != nil {
			return nil, false, err
		}
		if i > 0 {
			if err := ValidateTransition(result[i-1], record.Journal); err != nil {
				return nil, false, err
			}
		}
		result[i] = record.Journal
	}
	return result, true, nil
}

func loadAuthorities(ctx context.Context, root string) ([]Journal, error) {
	base := filepath.Join(root, filepath.FromSlash(migrationsPath))
	entries, found, err := runtimepath.ReadDir(root, base, true)
	if err != nil || !found {
		return nil, err
	}
	operations := make(map[string]struct{})
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			op := strings.TrimSuffix(name, ".json")
			if !validOperationID(op) {
				return nil, fmt.Errorf("find autonomous migration: malformed checkpoint name %q", name)
			}
			operations[op] = struct{}{}
		}
	}
	historyDir := filepath.Join(base, "history")
	historyEntries, historyFound, err := runtimepath.ReadDir(root, historyDir, true)
	if err != nil {
		return nil, err
	}
	if historyFound {
		for _, entry := range historyEntries {
			name := entry.Name()
			if len(name) > len("-000001.json") {
				op := strings.TrimSuffix(name, name[len(name)-len("-000001.json"):])
				if validOperationID(op) {
					operations[op] = struct{}{}
				}
			}
		}
	}
	ids := make([]string, 0, len(operations))
	for operation := range operations {
		ids = append(ids, operation)
	}
	sort.Strings(ids)
	result := make([]Journal, 0, len(ids))
	for _, operation := range ids {
		journal, found, err := Load(root, operation)
		if err != nil {
			return nil, err
		}
		if found {
			result = append(result, journal)
		}
	}
	return result, nil
}

func planFromJournal(root string, journal Journal) (Plan, error) {
	entries := make([]Entry, len(journal.Records))
	for i, record := range journal.Records {
		sourceRaw, found, err := runtimepath.ReadFile(root, filepath.Join(root, filepath.FromSlash(record.SourceArtifactPath)), false)
		if err != nil || !found || hashBytes(sourceRaw) != record.SourceSHA256 || len(sourceRaw) != record.SourceByteSize {
			return Plan{}, errors.Join(err, fmt.Errorf("recover autonomous migration: source material for %q is missing or conflicts", record.TaskID))
		}
		projectedRaw, found, err := runtimepath.ReadFile(root, filepath.Join(root, filepath.FromSlash(record.ProjectedArtifactPath)), false)
		if err != nil || !found || hashBytes(projectedRaw) != record.ProjectedSHA256 || len(projectedRaw) != record.ProjectedByteSize {
			return Plan{}, errors.Join(err, fmt.Errorf("recover autonomous migration: projected material for %q is missing or conflicts", record.TaskID))
		}
		stateRaw, found, err := runtimepath.ReadFile(root, filepath.Join(root, filepath.FromSlash(record.StateArtifactPath)), false)
		if err != nil || !found || hashBytes(stateRaw) != record.StateSHA256 || len(stateRaw) != record.StateByteSize {
			return Plan{}, errors.Join(err, fmt.Errorf("recover autonomous migration: state material for %q is missing or conflicts", record.TaskID))
		}
		source, err := taskfile.ParseArchivedTask(root, record.SourcePath, sourceRaw)
		if err != nil {
			return Plan{}, err
		}
		projected, err := taskfile.ParseArchivedTask(root, record.SourcePath, projectedRaw)
		if err != nil {
			return Plan{}, err
		}
		state, err := autonomousstate.DecodeState(stateRaw, record.TaskID)
		if err != nil {
			return Plan{}, err
		}
		entries[i] = Entry{TaskID: record.TaskID, SourcePath: record.SourcePath, SourceSHA256: record.SourceSHA256, SourceByteSize: record.SourceByteSize, SourceTask: source, ProjectedTask: projected, ProjectedSHA256: record.ProjectedSHA256, ProjectedByteSize: record.ProjectedByteSize, AutonomousState: state, AutonomousStatePath: record.StatePath, StateBytes: stateRaw, StateSHA256: record.StateSHA256, StateByteSize: record.StateByteSize}
	}
	plan := Plan{SchemaVersion: PlanSchemaVersion, TargetWorkflow: taskfile.WorkflowAutonomousV1, Entries: entries}
	return plan, validatePlan(root, plan)
}

func validateRecord(operation string, record Record) error {
	if !validStableID(record.TaskID) || record.SourcePath == "" || record.SourcePath != filepath.ToSlash(filepath.Clean(record.SourcePath)) || !strings.HasPrefix(record.SourcePath, taskfile.TasksDir+"/") || !validSHA256(record.SourceSHA256) || !validSHA256(record.ProjectedSHA256) || !validSHA256(record.StateSHA256) || record.SourceByteSize <= 0 || record.ProjectedByteSize <= 0 || record.StateByteSize <= 0 {
		return errors.New("invalid identity, path, hash, or byte size")
	}
	if record.StatePath != path.Join(".revolvr", "autonomous", "tasks", record.TaskID, "state.json") {
		return errors.New("state path is inconsistent")
	}
	base := path.Join(migrationsPath, "materials", operation, record.TaskID)
	if record.SourceArtifactPath != base+".source.md" || record.ProjectedArtifactPath != base+".projected.md" || record.StateArtifactPath != base+".state.json" {
		return errors.New("material path is inconsistent")
	}
	return nil
}

func acquireMigrationLock(ctx context.Context, root string) (func(), error) {
	lease, err := lock.AcquireFlock(ctx, root, lock.FlockConfig{
		RelativePath: ".revolvr/locks/autonomous-migration.lock",
		Mode:         lock.FlockExclusive,
		Wait:         true,
		Create:       true,
	})
	if err != nil {
		return nil, err
	}
	return func() { _ = lease.Close() }, nil
}

func marshalCanonical(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func decodeCanonical(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("autonomous migration authority: trailing JSON")
	}
	canonical, err := marshalCanonical(target)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return errors.New("autonomous migration authority: non-canonical JSON")
	}
	return nil
}

func historyFilename(operation string, sequence int64) string {
	return fmt.Sprintf("%s-%06d.json", operation, sequence)
}

func historySequence(name, operation string) (int64, bool, error) {
	prefix := operation + "-"
	if !strings.HasPrefix(name, prefix) {
		return 0, false, nil
	}
	remainder := strings.TrimPrefix(name, prefix)
	if len(remainder) != len("000001.json") || !strings.HasSuffix(remainder, ".json") {
		return 0, false, errors.New("load autonomous migration: malformed history filename")
	}
	sequence, err := strconv.ParseInt(strings.TrimSuffix(remainder, ".json"), 10, 64)
	if err != nil || sequence < 1 {
		return 0, false, errors.New("load autonomous migration: malformed history sequence")
	}
	return sequence, true, nil
}

func stageForSequence(sequence int64) (Stage, bool) {
	switch sequence {
	case 1:
		return StageAdmitted, true
	case 2:
		return StageStatesPublished, true
	case 3:
		return StageTasksPublished, true
	case 4:
		return StageCompleted, true
	default:
		return "", false
	}
}

func normalizedRequestedIDs(input []string) ([]string, error) {
	result := append([]string(nil), input...)
	for _, id := range result {
		if !validStableID(id) {
			return nil, errors.New("find autonomous migration: malformed task selection")
		}
	}
	sort.Strings(result)
	for i := 1; i < len(result); i++ {
		if result[i-1] == result[i] {
			return nil, errors.New("find autonomous migration: duplicate task selection")
		}
	}
	return result, nil
}

func journalTaskIDs(journal Journal) []string {
	result := make([]string, len(journal.Records))
	for i, record := range journal.Records {
		result[i] = record.TaskID
	}
	sort.Strings(result)
	return result
}

func operationID(material string) string { return "migration-" + material[:24] }
func recordOrder(record Record) string   { return record.SourcePath + "\x00" + record.TaskID }
func entryOrder(entry Entry) string      { return entry.SourcePath + "\x00" + entry.TaskID }
func hashBytes(raw []byte) string        { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func validOperationID(value string) bool {
	return strings.HasPrefix(value, "migration-") && len(value) == len("migration-")+24 && validHex(value[len("migration-"):])
}

func validStableID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	return len(value) == 64 && value == strings.ToLower(value) && validHex(value)
}
func validHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func removeProtectedTemp(root, target string) {
	if err := runtimepath.CheckFile(root, target, false); err == nil {
		_ = os.Remove(target)
	}
}

func fail(inject FailureInjector, point FailurePoint) error {
	if inject == nil || point == "" {
		return nil
	}
	if err := inject(point); err != nil {
		return fmt.Errorf("autonomous migration: injected failure at %s: %w", point, err)
	}
	return nil
}
