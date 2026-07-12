// Package autonomouschild owns validation and restartable publication of one
// exact bounded supervisor child proposal set. It starts no worker and mutates
// no parent task or state.
package autonomouschild

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousscheduler"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
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

type FailurePoint string

const (
	FailureAfterAdmission FailurePoint = "after_admission"
	FailureAfterStates    FailurePoint = "after_states"
	FailureAfterTasks     FailurePoint = "after_tasks"
)

type Input struct {
	RepositoryRoot            string
	OperationID               string
	Decision                  autonomous.SupervisorDecision
	Reference                 autonomous.DecisionReference
	ExpectedParentTaskSHA256  string
	ExpectedParentStateSHA256 string
	ArchiveEvidence           []autonomousscheduler.ArchiveEvidence
	ForbiddenValues           []string
	Ledger                    Ledger
	CreatedAt                 time.Time
	FailureInjector           func(FailurePoint) error
}

type Ledger interface {
	GetRunWithEvents(context.Context, string) (ledger.RunWithEvents, bool, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
}

type ledgerEvent struct {
	SchemaVersion string        `json:"schema_version"`
	OperationID   string        `json:"operation_id"`
	ParentTaskID  string        `json:"parent_task_id"`
	ProposalID    string        `json:"proposal_id"`
	Stage         Stage         `json:"stage"`
	Children      []ChildRecord `json:"children"`
}

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

type HistoryRecord struct {
	SchemaVersion string  `json:"schema_version"`
	Journal       Journal `json:"journal"`
}

type Result struct {
	Children []taskfile.Task
	Replayed bool
}

func Apply(ctx context.Context, input Input) (Result, error) {
	root, err := filepath.Abs(strings.TrimSpace(input.RepositoryRoot))
	if err != nil {
		return Result{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Result{}, err
	}
	if !safeID(input.OperationID) || input.CreatedAt.IsZero() {
		return Result{}, errors.New("child publication: safe operation ID and explicit creation time are required")
	}
	if err := input.Decision.Validate(); err != nil {
		return Result{}, err
	}
	if input.Decision.ChildTasks == nil {
		return Result{}, errors.New("child publication: decision has no child proposal set")
	}
	if err := input.Reference.Validate(); err != nil {
		return Result{}, err
	}
	if input.Reference.TaskID != input.Decision.TaskID || input.Reference.DecisionID == "" || input.Reference.Action != input.Decision.Action {
		return Result{}, errors.New("child publication: decision reference authority mismatch")
	}
	unlock, err := lock(ctx, root)
	if err != nil {
		return Result{}, err
	}
	defer unlock()

	parent, found, err := taskfile.FindByID(root, input.Decision.TaskID)
	if err != nil || !found {
		return Result{}, errors.Join(err, errors.New("child publication: parent task missing"))
	}
	if parent.SourceSHA256() != input.ExpectedParentTaskSHA256 {
		return Result{}, errors.New("child publication: stale parent task identity")
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		return Result{}, err
	}
	parentState, found, err := store.Load(ctx, parent.ID)
	if err != nil || !found {
		return Result{}, errors.Join(err, errors.New("child publication: parent state missing"))
	}
	if parentState.SHA256 != input.ExpectedParentStateSHA256 {
		return Result{}, errors.New("child publication: stale parent state identity")
	}

	projected, states, records, err := project(root, input)
	if err != nil {
		return Result{}, err
	}
	for i := range projected {
		stateRaw := mustState(states[i])
		for _, secret := range input.ForbiddenValues {
			if secret != "" && (bytes.Contains(projected[i].SourceBytes, []byte(secret)) || bytes.Contains(stateRaw, []byte(secret))) {
				return Result{}, errors.New("child publication: configured secret value is present in persistent child evidence")
			}
		}
	}
	material, err := materialHash(input, records)
	if err != nil {
		return Result{}, err
	}
	journalPath := filepath.Join(root, ".revolvr", "autonomous", "child-publications", input.OperationID+".json")
	journal, exists, err := readJournal(journalPath)
	if err != nil {
		return Result{}, err
	}
	if exists {
		if journal.MaterialSHA256 != material {
			return Result{}, errors.New("child publication: operation ID content conflict")
		}
		if journal.Stage == StageCompleted {
			tasks, err := loadChildren(root, journal.Children)
			return Result{Children: tasks, Replayed: true}, err
		}
	} else {
		active, loadErr := autonomousscheduler.LoadActiveStrict(ctx, root)
		if loadErr != nil {
			return Result{}, loadErr
		}
		for i := range projected {
			for _, item := range active {
				if item.Task.ID == projected[i].ID {
					return Result{}, fmt.Errorf("child publication: task id %q collides with an existing task", projected[i].ID)
				}
			}
			active = append(active, autonomousscheduler.ActiveTask{Task: projected[i], Lifecycle: string(states[i].Lifecycle)})
		}
		if _, buildErr := autonomousscheduler.BuildSnapshot(active, input.ArchiveEvidence); buildErr != nil {
			return Result{}, fmt.Errorf("child publication: proposed graph: %w", buildErr)
		}
		journal = Journal{SchemaVersion: JournalSchemaVersion, OperationID: input.OperationID, ParentTaskID: parent.ID, DecisionID: input.Reference.DecisionID, ProposalID: input.Decision.ChildTasks.ProposalID, MaterialSHA256: material, Stage: StageAdmitted, Sequence: 1, Children: records, CreatedAt: input.CreatedAt.UTC()}
		if err := persist(root, journal); err != nil {
			return Result{}, err
		}
		if input.FailureInjector != nil {
			if err := input.FailureInjector(FailureAfterAdmission); err != nil {
				return Result{}, err
			}
		}
	}
	if journal.Stage == StageAdmitted {
		for i := range records {
			if err := writeImmutable(root, records[i].StatePath, mustState(states[i]), records[i].StateSHA256); err != nil {
				return Result{}, err
			}
		}
		journal.Stage, journal.Sequence = StageStatesPublished, journal.Sequence+1
		if err := persist(root, journal); err != nil {
			return Result{}, err
		}
		if input.FailureInjector != nil {
			if err := input.FailureInjector(FailureAfterStates); err != nil {
				return Result{}, err
			}
		}
	}
	if journal.Stage == StageStatesPublished {
		for i := range projected {
			if _, err := taskfile.PublishAutonomousTask(root, projected[i]); err != nil {
				return Result{}, err
			}
		}
		journal.Stage, journal.Sequence = StageTasksPublished, journal.Sequence+1
		if err := persist(root, journal); err != nil {
			return Result{}, err
		}
		if input.FailureInjector != nil {
			if err := input.FailureInjector(FailureAfterTasks); err != nil {
				return Result{}, err
			}
		}
	}
	if journal.Stage == StageTasksPublished {
		if input.Ledger != nil {
			for _, eventType := range []ledger.EventType{ledger.EventChildProposalAdmitted, ledger.EventChildrenPublished, ledger.EventChildPublicationCompleted} {
				if err := ensureLedgerEvent(context.WithoutCancel(ctx), input.Ledger, input.Reference.RunID, eventType, journal); err != nil {
					return Result{}, err
				}
			}
		}
		journal.Stage, journal.Sequence = StageCompleted, journal.Sequence+1
		if err := persist(root, journal); err != nil {
			return Result{}, err
		}
	}
	tasks, err := loadChildren(root, records)
	return Result{Children: tasks}, err
}

func ensureLedgerEvent(ctx context.Context, store Ledger, runID string, eventType ledger.EventType, journal Journal) error {
	payload := ledgerEvent{SchemaVersion: "autonomous-child-ledger-event-v1", OperationID: journal.OperationID, ParentTaskID: journal.ParentTaskID, ProposalID: journal.ProposalID, Stage: journal.Stage, Children: journal.Children}
	want, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	history, found, err := store.GetRunWithEvents(ctx, runID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("child publication: supervisor ledger run %q is missing", runID)
	}
	for _, event := range history.Events {
		if event.Type != eventType {
			continue
		}
		var existing ledgerEvent
		if json.Unmarshal(event.Payload, &existing) == nil && existing.OperationID == journal.OperationID {
			got, _ := json.Marshal(existing)
			if bytes.Equal(got, want) {
				return nil
			}
			return errors.New("child publication: ledger operation evidence conflict")
		}
	}
	_, err = store.AppendEvent(ctx, runID, eventType, payload)
	return err
}

func project(root string, input Input) ([]taskfile.Task, []autonomous.ExecutionState, []ChildRecord, error) {
	proposal := input.Decision.ChildTasks
	children := append([]autonomous.ChildTaskProposal(nil), proposal.Children...)
	sort.Slice(children, func(i, j int) bool { return children[i].Key < children[j].Key })
	tasks := make([]taskfile.Task, 0, len(children))
	states := make([]autonomous.ExecutionState, 0, len(children))
	records := make([]ChildRecord, 0, len(children))
	for _, child := range children {
		id := childID(input.Decision.TaskID, input.Reference.DecisionID, proposal.ProposalID, child.Key)
		evidenceTokens := make([]string, len(child.Evidence))
		for i, e := range child.Evidence {
			evidenceTokens[i] = string(e.Kind) + ":" + e.Reference
		}
		body := child.Scope + "\n\n## Success criteria\n"
		for _, criterion := range child.SuccessCriteria {
			body += "- " + criterion + "\n"
		}
		task, err := taskfile.ProjectAutonomousTask(root, taskfile.AutonomousCreateInput{ID: id, Title: child.Title, Body: body, DependsOn: child.DependsOn, Tags: child.Tags, Conflicts: child.Conflicts, ParentTaskID: input.Decision.TaskID, ChildProposalID: proposal.ProposalID, ChildDecisionID: input.Reference.DecisionID, ChildRunID: input.Reference.RunID, ChildEvidence: evidenceTokens, ParentBehavior: string(child.ParentBehavior)})
		if err != nil {
			return nil, nil, nil, err
		}
		lineage := autonomous.ChildLineage{SchemaVersion: autonomous.ChildLineageSchemaVersion, OperationID: input.OperationID, ParentTaskID: input.Decision.TaskID, ProposalID: proposal.ProposalID, ProposalKey: child.Key, DecisionID: input.Reference.DecisionID, SupervisorRunID: input.Reference.RunID, ParentBehavior: child.ParentBehavior, Evidence: child.Evidence, CreatedAt: input.CreatedAt.UTC()}
		state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: id, Lifecycle: autonomous.LifecycleStatePending, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}, ChildOf: &lineage}
		stateRaw, err := autonomousstate.MarshalState(state)
		if err != nil {
			return nil, nil, nil, err
		}
		records = append(records, ChildRecord{TaskID: id, ProposalKey: child.Key, TaskPath: task.SourcePath, TaskSHA256: task.SourceSHA256(), StatePath: task.AutonomousStatePath, StateSHA256: hash(stateRaw)})
		tasks, states = append(tasks, task), append(states, state)
	}
	return tasks, states, records, nil
}

func childID(parent, decision, proposal, key string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{parent, decision, proposal, key}, "\x00")))
	return "child-" + hex.EncodeToString(sum[:12])
}
func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func mustState(state autonomous.ExecutionState) []byte {
	raw, err := autonomousstate.MarshalState(state)
	if err != nil {
		panic(err)
	}
	return raw
}
func materialHash(input Input, children []ChildRecord) (string, error) {
	raw, err := json.Marshal(struct {
		Operation               string
		Decision                autonomous.SupervisorDecision
		Reference               autonomous.DecisionReference
		ParentTask, ParentState string
		Children                []ChildRecord
	}{input.OperationID, input.Decision, input.Reference, input.ExpectedParentTaskSHA256, input.ExpectedParentStateSHA256, children})
	return hash(raw), err
}

func persist(root string, journal Journal) error {
	raw, _ := json.MarshalIndent(journal, "", "  ")
	raw = append(raw, '\n')
	history := HistoryRecord{SchemaVersion: HistorySchemaVersion, Journal: journal}
	hraw, _ := json.MarshalIndent(history, "", "  ")
	hraw = append(hraw, '\n')
	base := filepath.Join(root, ".revolvr", "autonomous", "child-publications")
	if err := os.MkdirAll(filepath.Join(base, "history"), 0o755); err != nil {
		return err
	}
	if err := writeImmutable(root, filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "child-publications", "history", fmt.Sprintf("%s-%06d.json", journal.OperationID, journal.Sequence))), hraw, hash(hraw)); err != nil {
		return err
	}
	return writeMutable(filepath.Join(base, journal.OperationID+".json"), raw)
}
func readJournal(path string) (Journal, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Journal{}, false, nil
	}
	if err != nil {
		return Journal{}, false, err
	}
	var j Journal
	if err := json.Unmarshal(raw, &j); err != nil {
		return Journal{}, false, err
	}
	if j.SchemaVersion != JournalSchemaVersion {
		return Journal{}, false, errors.New("child publication: invalid journal schema")
	}
	return j, true, nil
}
func writeMutable(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".child-journal-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o644); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	return err
}
func writeImmutable(root, rel string, raw []byte, want string) error {
	if hash(raw) != want {
		return errors.New("child publication: immutable identity mismatch")
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return errors.New("child publication: unsafe path")
	}
	if existing, err := os.ReadFile(abs); err == nil {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("child publication: path %s has different bytes", rel)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(abs), ".child-immutable-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o644); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(name, abs)
	}
	return err
}
func loadChildren(root string, records []ChildRecord) ([]taskfile.Task, error) {
	result := make([]taskfile.Task, 0, len(records))
	for _, record := range records {
		task, err := taskfile.Load(root, record.TaskPath)
		if err != nil {
			return nil, err
		}
		if task.SourceSHA256() != record.TaskSHA256 {
			return nil, errors.New("child publication: task readback identity mismatch")
		}
		result = append(result, task)
	}
	return result, nil
}
func lock(ctx context.Context, root string) (func(), error) {
	path := filepath.Join(root, ".revolvr", "locks", "child-publication.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
func safeID(v string) bool {
	if v == "" || v != strings.TrimSpace(v) {
		return false
	}
	for _, r := range v {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
