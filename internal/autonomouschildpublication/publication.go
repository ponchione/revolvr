// Package autonomouschildpublication owns the shared durable authority for
// one supervised child-publication operation. It performs no publication.
package autonomouschildpublication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

const JournalSchemaVersion = "autonomous-child-publication-v1"
const HistorySchemaVersion = "autonomous-child-publication-transition-v1"

type Stage string

const (
	StageAdmitted        Stage = "admitted"
	StageStatesPublished Stage = "states_published"
	StageTasksPublished  Stage = "tasks_published"
	StageCompleted       Stage = "completed"
)

type ChildRecord struct {
	TaskID      string `json:"task_id"`
	ProposalKey string `json:"proposal_key"`
	TaskPath    string `json:"task_path"`
	TaskSHA256  string `json:"task_sha256"`
	StatePath   string `json:"state_path"`
	StateSHA256 string `json:"state_sha256"`
}

type Journal struct {
	SchemaVersion  string        `json:"schema_version"`
	OperationID    string        `json:"operation_id"`
	ParentTaskID   string        `json:"parent_task_id"`
	DecisionID     string        `json:"decision_id"`
	ProposalID     string        `json:"proposal_id"`
	MaterialSHA256 string        `json:"material_sha256"`
	Stage          Stage         `json:"stage"`
	Sequence       int64         `json:"sequence"`
	Children       []ChildRecord `json:"children"`
	CreatedAt      time.Time     `json:"created_at"`
}

func (j Journal) Validate() error {
	if j.SchemaVersion != JournalSchemaVersion || !validStableID(j.OperationID) || !validStableID(j.ParentTaskID) || !validStableID(j.DecisionID) || !validKebabID(j.ProposalID) || !validSHA256(j.MaterialSHA256) || j.CreatedAt.IsZero() || j.CreatedAt.Location() != time.UTC {
		return errors.New("child publication journal: invalid schema, identity, material, or creation time")
	}
	wantStage, ok := stageForSequence(j.Sequence)
	if !ok || j.Stage != wantStage {
		return errors.New("child publication journal: stage and sequence do not form a legal state")
	}
	if len(j.Children) == 0 {
		return errors.New("child publication journal: child set is empty")
	}
	seenTasks := make(map[string]struct{}, len(j.Children))
	seenKeys := make(map[string]struct{}, len(j.Children))
	for i, child := range j.Children {
		if !validKebabID(child.ProposalKey) || child.TaskID != ChildTaskID(j.ParentTaskID, j.DecisionID, j.ProposalID, child.ProposalKey) || child.TaskPath != path.Join(taskfile.TasksDir, child.TaskID+".md") || child.StatePath != path.Join(".revolvr", "autonomous", "tasks", child.TaskID, "state.json") || !validSHA256(child.TaskSHA256) || !validSHA256(child.StateSHA256) {
			return fmt.Errorf("child publication journal: children[%d] is malformed or inconsistent", i)
		}
		if i > 0 && j.Children[i-1].ProposalKey >= child.ProposalKey {
			return errors.New("child publication journal: children are not strictly ordered by proposal key")
		}
		if _, exists := seenTasks[child.TaskID]; exists {
			return errors.New("child publication journal: duplicate child task identity")
		}
		if _, exists := seenKeys[child.ProposalKey]; exists {
			return errors.New("child publication journal: duplicate proposal key")
		}
		seenTasks[child.TaskID] = struct{}{}
		seenKeys[child.ProposalKey] = struct{}{}
	}
	return nil
}

func (j Journal) SameAuthority(other Journal) bool {
	other.Stage = j.Stage
	other.Sequence = j.Sequence
	return reflect.DeepEqual(j, other)
}

func ValidateTransition(previous, next Journal) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("child publication journal transition: previous: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("child publication journal transition: next: %w", err)
	}
	if !previous.SameAuthority(next) || next.Sequence != previous.Sequence+1 {
		return errors.New("child publication journal transition: authority changed or sequence is noncontiguous")
	}
	return nil
}

type HistoryRecord struct {
	SchemaVersion string  `json:"schema_version"`
	Journal       Journal `json:"journal"`
}

type Projection struct {
	Journal Journal
}

func Load(repositoryRoot, operationID string) (Projection, bool, error) {
	root, err := runtimepath.CanonicalRoot(repositoryRoot)
	if err != nil {
		return Projection{}, false, err
	}
	if !validStableID(operationID) {
		return Projection{}, false, errors.New("child publication authority: operation ID is malformed")
	}
	base := filepath.Join(root, ".revolvr", "autonomous", "child-publications")
	if err := runtimepath.CheckDir(root, base, true); err != nil {
		return Projection{}, false, err
	}
	checkpoint, checkpointFound, err := readJournal(root, filepath.Join(base, operationID+".json"), operationID)
	if err != nil {
		return Projection{}, false, fmt.Errorf("child publication authority: checkpoint: %w", err)
	}
	history, historyFound, err := readHistory(root, filepath.Join(base, "history"), operationID)
	if err != nil {
		return Projection{}, false, err
	}
	if !checkpointFound && !historyFound {
		return Projection{}, false, nil
	}
	if checkpointFound && !historyFound {
		return Projection{}, false, errors.New("child publication authority: checkpoint exists without immutable history")
	}
	latest := history[len(history)-1]
	if checkpointFound {
		if checkpoint.Sequence > latest.Sequence {
			return Projection{}, false, errors.New("child publication authority: checkpoint is ahead of immutable history")
		}
		if !reflect.DeepEqual(checkpoint, history[checkpoint.Sequence-1]) {
			return Projection{}, false, errors.New("child publication authority: checkpoint conflicts with immutable history")
		}
	}
	return Projection{Journal: latest}, true, nil
}

func (p Projection) Child(taskID string) (ChildRecord, bool) {
	for _, child := range p.Journal.Children {
		if child.TaskID == taskID {
			return child, true
		}
	}
	return ChildRecord{}, false
}

// ValidateActiveChild binds an active task and its evolving canonical state to
// the exact completed publication. The task/state hashes in ChildRecord name
// the initially published bytes; lifecycle updates may legitimately replace
// those bytes, while ChildOf is immutable under execution-state transitions.
func (p Projection) ValidateActiveChild(task taskfile.Task, snapshot autonomousstate.Snapshot) error {
	if err := p.Journal.Validate(); err != nil {
		return err
	}
	if p.Journal.Stage != StageCompleted {
		return errors.New("child publication authority: publication is incomplete")
	}
	record, found := p.Child(task.ID)
	if !found {
		return errors.New("child publication authority: active task is absent from the published child set")
	}
	if task.SourcePath != record.TaskPath || task.AutonomousStatePath != record.StatePath || snapshot.SourcePath != record.StatePath || snapshot.State.TaskID != record.TaskID {
		return errors.New("child publication authority: active task or state path differs from publication")
	}
	lineage := snapshot.State.ChildOf
	if lineage == nil || lineage.OperationID != p.Journal.OperationID || lineage.ParentTaskID != p.Journal.ParentTaskID || lineage.DecisionID != p.Journal.DecisionID || lineage.ProposalID != p.Journal.ProposalID || lineage.ProposalKey != record.ProposalKey || !lineage.CreatedAt.Equal(p.Journal.CreatedAt) {
		return errors.New("child publication authority: active child lineage differs from publication")
	}
	if task.ParentTaskID != lineage.ParentTaskID || task.ChildProposalID != lineage.ProposalID || task.ChildDecisionID != lineage.DecisionID || task.ChildRunID != lineage.SupervisorRunID || task.ParentBehavior != string(lineage.ParentBehavior) || !equalStrings(task.ChildEvidence, evidenceTokens(lineage.Evidence)) {
		return errors.New("child publication authority: active task metadata differs from immutable lineage")
	}
	initial := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        record.TaskID,
		Lifecycle:     autonomous.LifecycleStatePending,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
		ChildOf: lineage,
	}
	raw, err := autonomousstate.MarshalState(initial)
	if err != nil || hash(raw) != record.StateSHA256 {
		return errors.Join(err, errors.New("child publication authority: initial state identity differs from child record"))
	}
	if snapshot.SHA256 == record.StateSHA256 && task.SourceSHA256() != record.TaskSHA256 {
		return errors.New("child publication authority: initial task identity differs from child record")
	}
	return nil
}

func MarshalJournal(journal Journal) ([]byte, error) {
	if err := journal.Validate(); err != nil {
		return nil, err
	}
	return canonicalJSON(journal)
}

func MarshalHistory(journal Journal) ([]byte, error) {
	if err := journal.Validate(); err != nil {
		return nil, err
	}
	return canonicalJSON(HistoryRecord{SchemaVersion: HistorySchemaVersion, Journal: journal})
}

func ChildTaskID(parent, decision, proposal, key string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{parent, decision, proposal, key}, "\x00")))
	return "child-" + hex.EncodeToString(sum[:12])
}

func HistoryFilename(operationID string, sequence int64) string {
	return historyName(operationID, sequence)
}

func readJournal(root, filePath, operationID string) (Journal, bool, error) {
	raw, found, err := runtimepath.ReadFile(root, filePath, true)
	if err != nil || !found {
		return Journal{}, found, err
	}
	var journal Journal
	if err := strictCanonicalJSON(raw, &journal); err != nil {
		return Journal{}, false, err
	}
	if err := journal.Validate(); err != nil {
		return Journal{}, false, err
	}
	if journal.OperationID != operationID {
		return Journal{}, false, errors.New("child publication authority: checkpoint operation identity mismatch")
	}
	return journal, true, nil
}

func readHistory(root, dir, operationID string) ([]Journal, bool, error) {
	entries, found, err := runtimepath.ReadDir(root, dir, true)
	if err != nil {
		return nil, false, err
	}
	if !found {
		return nil, false, nil
	}
	type historyEntry struct {
		name     string
		sequence int64
	}
	matching := make([]historyEntry, 0, 4)
	for _, entry := range entries {
		sequence, belongs, parseErr := historySequence(entry.Name(), operationID)
		if parseErr != nil {
			return nil, false, parseErr
		}
		if !belongs {
			continue
		}
		if entry.IsDir() {
			return nil, false, errors.New("child publication authority: history entry is not a file")
		}
		matching = append(matching, historyEntry{name: entry.Name(), sequence: sequence})
	}
	if len(matching) == 0 {
		return nil, false, nil
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].sequence < matching[j].sequence })
	history := make([]Journal, 0, len(matching))
	for i, entry := range matching {
		sequence := int64(i + 1)
		if entry.sequence != sequence || entry.name != historyName(operationID, sequence) {
			return nil, false, errors.New("child publication authority: immutable history is noncontiguous")
		}
		raw, found, err := runtimepath.ReadFile(root, filepath.Join(dir, entry.name), false)
		if err != nil || !found {
			return nil, false, err
		}
		var record HistoryRecord
		if err := strictCanonicalJSON(raw, &record); err != nil {
			return nil, false, err
		}
		if record.SchemaVersion != HistorySchemaVersion || record.Journal.OperationID != operationID || record.Journal.Sequence != sequence {
			return nil, false, errors.New("child publication authority: invalid immutable history record")
		}
		if err := record.Journal.Validate(); err != nil {
			return nil, false, err
		}
		if sequence > 1 {
			if err := ValidateTransition(history[len(history)-1], record.Journal); err != nil {
				return nil, false, err
			}
		}
		history = append(history, record.Journal)
	}
	return history, true, nil
}

func historySequence(name, operationID string) (int64, bool, error) {
	prefix := operationID + "-"
	if !strings.HasPrefix(name, prefix) {
		return 0, false, nil
	}
	remainder := strings.TrimPrefix(name, prefix)
	if len(remainder) != len("000001.json") {
		return 0, false, nil
	}
	if !strings.HasSuffix(remainder, ".json") {
		return 0, false, errors.New("child publication authority: malformed history name")
	}
	sequence, err := strconv.ParseInt(strings.TrimSuffix(remainder, ".json"), 10, 64)
	if err != nil || sequence < 1 {
		return 0, false, errors.New("child publication authority: malformed history sequence")
	}
	return sequence, true, nil
}

func historyName(operationID string, sequence int64) string {
	return fmt.Sprintf("%s-%06d.json", operationID, sequence)
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

func strictCanonicalJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("child publication authority: trailing JSON")
	}
	canonical, err := canonicalJSON(target)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return errors.New("child publication authority: non-canonical JSON")
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
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

func validKebabID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for i, r := range value {
		switch {
		case i == 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && r == '-' && value[i-1] != '-' && i < len(value)-1:
		default:
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func evidenceTokens(evidence []autonomous.EvidenceReference) []string {
	result := make([]string, len(evidence))
	for i, item := range evidence {
		result[i] = string(item.Kind) + ":" + item.Reference
	}
	return result
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
