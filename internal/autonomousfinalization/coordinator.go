package autonomousfinalization

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
	"revolvr/internal/redact"
	"revolvr/internal/taskfile"
)

type FailurePoint string

const (
	FailureBeforeFrozenWrite      FailurePoint = "before_frozen_write"
	FailureBeforeCapsuleRename    FailurePoint = "before_capsule_rename"
	FailureAfterCapsuleRename     FailurePoint = "after_capsule_rename"
	FailureBeforeTaskUpdate       FailurePoint = "before_task_update"
	FailureAfterTaskUpdate        FailurePoint = "after_task_update"
	FailureBeforeLedgerCompletion FailurePoint = "before_ledger_completion"
	FailureAfterLedgerCompletion  FailurePoint = "after_ledger_completion"
)

type StateStore interface {
	Load(context.Context, string) (autonomousstate.Snapshot, bool, error)
	CommitFinalization(context.Context, autonomousstate.FinalizationCommitRequest) (autonomousstate.FinalizationCommitResult, error)
}

type Ledger interface {
	CreateRun(context.Context, ledger.RunSpec) (ledger.Run, error)
	GetRunWithEvents(context.Context, string) (ledger.RunWithEvents, bool, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
	CompleteRun(context.Context, string, ledger.RunCompletion) (ledger.Run, bool, error)
}

type Config struct {
	RepositoryRoot  string
	Evidence        FrozenEvidence
	StateStore      StateStore
	Ledger          Ledger
	FailureInjector func(FailurePoint) error
	// RevalidateEvidence performs bounded live readback of the frozen task
	// workspace/source/safety authority. It must not mutate or rerun a gate.
	RevalidateEvidence func(context.Context, FrozenEvidence) error
	Redactor           *redact.Redactor
}

type Result struct {
	State          autonomousstate.Snapshot
	Task           taskfile.Task
	FrozenEvidence autonomous.FinalizationArtifact
	Capsule        autonomous.FinalizationArtifact
	Manifest       autonomous.FinalizationArtifact
	LedgerRun      ledger.Run
	Replayed       bool
}

func Finalize(ctx context.Context, cfg Config) (Result, error) {
	root, err := canonicalRoot(cfg.RepositoryRoot)
	if err != nil {
		return Result{}, err
	}
	if cfg.StateStore == nil || cfg.Ledger == nil || cfg.RevalidateEvidence == nil {
		return Result{}, errors.New("finalize autonomous task: state store, ledger, and live evidence revalidator are required")
	}
	e := cfg.Evidence
	if len(e.SafetyPolicy.Redaction.EnvironmentVariables) > 0 && cfg.Redactor == nil {
		return Result{}, errors.New("finalize autonomous task: configured secret redactor is required")
	}
	if err := e.Validate(); err != nil {
		return Result{}, fmt.Errorf("finalize autonomous task: frozen evidence: %w", err)
	}
	if err := cfg.RevalidateEvidence(ctx, e); err != nil {
		return Result{}, fmt.Errorf("finalize autonomous task: live evidence is stale or unsafe: %w", err)
	}
	policyInput := autonomouspolicy.Input{TaskID: e.Task.TaskID, Decision: e.Decision, Reference: e.DecisionReference, State: e.State, Source: e.Source, Verification: &e.Verification, Audit: &e.Audit}
	route, err := autonomouspolicy.Evaluate(policyInput)
	if err != nil {
		return Result{}, fmt.Errorf("finalize autonomous task: completion revalidation: %w", err)
	}
	if !reflect.DeepEqual(route, e.Route) {
		return Result{}, errors.New("finalize autonomous task: supplied route is not the recomputed complete authorization")
	}
	frozenBytes, err := MarshalFrozen(e)
	if err != nil {
		return Result{}, err
	}
	frozenBytes = redactBytes(cfg.Redactor, frozenBytes)
	base := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", e.Task.TaskID, "completion"))
	frozenPath := filepath.ToSlash(filepath.Join(base, "completion-evidence.json"))
	capsulePath := filepath.ToSlash(filepath.Join(base, "completion.md"))
	manifestPath := filepath.ToSlash(filepath.Join(base, "completion-manifest.json"))
	frozenIdentity := artifact(frozenPath, frozenBytes)

	current, found, err := cfg.StateStore.Load(ctx, e.Task.TaskID)
	if err != nil || !found {
		return Result{}, errors.Join(err, errors.New("finalize autonomous task: canonical state is missing"))
	}
	replayed := current.State.Finalization != nil
	if current.State.Finalization == nil {
		if current.SHA256 != e.StateIdentity.SHA256 || current.ByteSize != e.StateIdentity.ByteSize || !reflect.DeepEqual(current.State, e.State) {
			return Result{}, errors.New("finalize autonomous task: stale canonical state before admission")
		}
		task, err := loadExactTask(root, e.Task, "pending")
		if err != nil {
			return Result{}, err
		}
		projected, err := taskfile.ProjectMetadataFromSnapshot(root, task, taskfile.MetadataUpdate{Status: taskfile.StatusCompleted})
		if err != nil || projected.SourceSHA256() != e.Task.CompletedSHA256 || projected.SourceByteSize() != e.Task.CompletedByteSize {
			return Result{}, errors.Join(err, errors.New("finalize autonomous task: frozen completed-task projection is invalid"))
		}
		if err := fail(cfg, FailureBeforeFrozenWrite); err != nil {
			return Result{}, err
		}
		if err := writeImmutable(root, frozenIdentity, frozenBytes); err != nil {
			return Result{}, err
		}
		next := current.State
		next.Lifecycle = autonomous.LifecycleStateFinalizing
		next.LatestDecision = &e.DecisionReference
		next.Finalization = &autonomous.FinalizationDetail{SchemaVersion: autonomous.FinalizationDetailSchemaVersion, OperationID: e.OperationID, RunID: e.FinalizationRunID, Stage: autonomous.FinalizationStageAdmitted, FrozenEvidence: frozenIdentity, OriginalTaskSHA256: e.Task.SHA256, AdmittedAt: e.AdmittedAt}
		current, err = commitStage(context.WithoutCancel(ctx), cfg, current, next, e.AdmittedAt)
		if err != nil {
			return Result{}, err
		}
		_ = task
	} else {
		d := current.State.Finalization
		if d.OperationID != e.OperationID || d.RunID != e.FinalizationRunID || d.FrozenEvidence != frozenIdentity || d.OriginalTaskSHA256 != e.Task.SHA256 {
			return Result{}, errors.New("finalize autonomous task: material operation reuse conflicts with frozen transaction")
		}
		if err := verifyArtifact(root, frozenIdentity, frozenBytes); err != nil {
			return Result{}, err
		}
	}

	if err := ensureRun(context.WithoutCancel(ctx), cfg.Ledger, e); err != nil {
		return Result{}, err
	}
	if err := ensureEvent(context.WithoutCancel(ctx), cfg.Ledger, e.FinalizationRunID, ledger.EventFinalizationPrepared, stageEvent(e, autonomous.FinalizationStageAdmitted, nil)); err != nil {
		return Result{}, err
	}

	capsuleBytes, err := RenderCapsule(e)
	if err != nil {
		return Result{}, err
	}
	capsuleBytes = redactBytes(cfg.Redactor, capsuleBytes)
	capsuleIdentity := artifact(capsulePath, capsuleBytes)
	manifest, err := BuildManifest(e, frozenIdentity, capsuleIdentity)
	if err != nil {
		return Result{}, err
	}
	manifestBytes, err := MarshalManifest(manifest)
	if err != nil {
		return Result{}, err
	}
	manifestBytes = redactBytes(cfg.Redactor, manifestBytes)
	manifestIdentity := artifact(manifestPath, manifestBytes)
	if stageLess(current.State.Finalization.Stage, autonomous.FinalizationStageMaterialized) {
		if err := fail(cfg, FailureBeforeCapsuleRename); err != nil {
			return Result{}, err
		}
		if err := writeImmutable(root, capsuleIdentity, capsuleBytes); err != nil {
			return Result{}, err
		}
		if err := fail(cfg, FailureAfterCapsuleRename); err != nil {
			return Result{}, err
		}
		if err := writeImmutable(root, manifestIdentity, manifestBytes); err != nil {
			return Result{}, err
		}
		next := current.State
		detail := *next.Finalization
		detail.Stage = autonomous.FinalizationStageMaterialized
		detail.Capsule = &capsuleIdentity
		detail.Manifest = &manifestIdentity
		value := e.AdmittedAt
		detail.MaterializedAt = &value
		next.Finalization = &detail
		current, err = commitStage(context.WithoutCancel(ctx), cfg, current, next, e.AdmittedAt)
		if err != nil {
			return Result{}, err
		}
	}
	if err := verifyArtifact(root, capsuleIdentity, capsuleBytes); err != nil {
		return Result{}, err
	}
	if err := verifyArtifact(root, manifestIdentity, manifestBytes); err != nil {
		return Result{}, err
	}
	if err := ensureEvent(context.WithoutCancel(ctx), cfg.Ledger, e.FinalizationRunID, ledger.EventFinalizationMaterialized, stageEvent(e, autonomous.FinalizationStageMaterialized, &manifestIdentity)); err != nil {
		return Result{}, err
	}

	task, ok, err := taskfile.FindByID(root, e.Task.TaskID)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, errors.New("finalize autonomous task: canonical task is missing")
	}
	if stageLess(current.State.Finalization.Stage, autonomous.FinalizationStageTaskCompleted) {
		if err := cfg.RevalidateEvidence(context.WithoutCancel(ctx), e); err != nil {
			return Result{}, fmt.Errorf("finalize autonomous task: frozen evidence reconciliation stopped: %w", err)
		}
		if err := fail(cfg, FailureBeforeTaskUpdate); err != nil {
			return Result{}, err
		}
		if task.Status == taskfile.StatusPending {
			if task.SourceSHA256() != e.Task.SHA256 || task.SourceByteSize() != e.Task.ByteSize {
				return Result{}, errors.New("finalize autonomous task: task source drifted before terminal status update")
			}
			task, err = taskfile.UpdateMetadataFromSnapshot(root, task, taskfile.MetadataUpdate{Status: taskfile.StatusCompleted})
			if err != nil {
				return Result{}, err
			}
		} else if task.Status != taskfile.StatusCompleted || task.SourceSHA256() != e.Task.CompletedSHA256 || task.SourceByteSize() != e.Task.CompletedByteSize {
			return Result{}, fmt.Errorf("finalize autonomous task: task status is %q", task.Status)
		}
		if err := fail(cfg, FailureAfterTaskUpdate); err != nil {
			return Result{}, err
		}
		next := current.State
		detail := *next.Finalization
		detail.Stage = autonomous.FinalizationStageTaskCompleted
		if task.SourceSHA256() != e.Task.CompletedSHA256 || task.SourceByteSize() != e.Task.CompletedByteSize {
			return Result{}, errors.New("finalize autonomous task: completed task readback identity mismatch")
		}
		detail.CompletedTaskSHA256 = task.SourceSHA256()
		value := e.TerminalAt
		detail.TaskCompletedAt = &value
		next.Finalization = &detail
		current, err = commitStage(context.WithoutCancel(ctx), cfg, current, next, e.TerminalAt)
		if err != nil {
			return Result{}, err
		}
	} else if task.Status != taskfile.StatusCompleted || task.SourceSHA256() != current.State.Finalization.CompletedTaskSHA256 || task.SourceSHA256() != e.Task.CompletedSHA256 || task.SourceByteSize() != e.Task.CompletedByteSize {
		return Result{}, errors.New("finalize autonomous task: completed task source no longer matches frozen transaction")
	}

	if stageLess(current.State.Finalization.Stage, autonomous.FinalizationStageStateCompleted) {
		next := current.State
		detail := *next.Finalization
		detail.Stage = autonomous.FinalizationStageStateCompleted
		value := e.TerminalAt
		detail.StateCompletedAt = &value
		next.Finalization = &detail
		next.Lifecycle = autonomous.LifecycleStateCompleted
		next.Terminal = &autonomous.TerminalDetail{Reason: "Completion gates validated and completion capsule materialized.", Evidence: []autonomous.EvidenceReference{{Kind: autonomous.EvidenceKindFile, Reference: capsuleIdentity.Path, Detail: capsuleIdentity.SHA256}, {Kind: autonomous.EvidenceKindFile, Reference: manifestIdentity.Path, Detail: manifestIdentity.SHA256}, {Kind: autonomous.EvidenceKindLedger, Reference: e.FinalizationRunID, Detail: "terminal finalization run"}}}
		current, err = commitStage(context.WithoutCancel(ctx), cfg, current, next, e.TerminalAt)
		if err != nil {
			return Result{}, err
		}
	}
	if err := ensureEvent(context.WithoutCancel(ctx), cfg.Ledger, e.FinalizationRunID, ledger.EventFinalizationStateTerminal, stageEvent(e, autonomous.FinalizationStageStateCompleted, &manifestIdentity)); err != nil {
		return Result{}, err
	}
	if stageLess(current.State.Finalization.Stage, autonomous.FinalizationStageLedgerCompleted) {
		if err := fail(cfg, FailureBeforeLedgerCompletion); err != nil {
			return Result{}, err
		}
		if err := ensureEvent(context.WithoutCancel(ctx), cfg.Ledger, e.FinalizationRunID, ledger.EventFinalizationCompleted, stageEvent(e, autonomous.FinalizationStageLedgerCompleted, &manifestIdentity)); err != nil {
			return Result{}, err
		}
		run, err := completeRunExact(context.WithoutCancel(ctx), cfg.Ledger, e, manifestIdentity)
		if err != nil {
			return Result{}, err
		}
		if err := fail(cfg, FailureAfterLedgerCompletion); err != nil {
			return Result{}, err
		}
		next := current.State
		detail := *next.Finalization
		detail.Stage = autonomous.FinalizationStageLedgerCompleted
		value := e.TerminalAt
		detail.LedgerCompletedAt = &value
		next.Finalization = &detail
		current, err = commitStage(context.WithoutCancel(ctx), cfg, current, next, e.TerminalAt)
		if err != nil {
			return Result{}, err
		}
		return Result{State: current, Task: task, FrozenEvidence: frozenIdentity, Capsule: capsuleIdentity, Manifest: manifestIdentity, LedgerRun: run, Replayed: replayed}, nil
	}
	runWithEvents, ok, err := cfg.Ledger.GetRunWithEvents(ctx, e.FinalizationRunID)
	if err != nil || !ok {
		return Result{}, errors.Join(err, errors.New("finalize autonomous task: terminal ledger run missing"))
	}
	return Result{State: current, Task: task, FrozenEvidence: frozenIdentity, Capsule: capsuleIdentity, Manifest: manifestIdentity, LedgerRun: runWithEvents.Run, Replayed: true}, nil
}

func commitStage(ctx context.Context, cfg Config, current autonomousstate.Snapshot, next autonomous.ExecutionState, createdAt time.Time) (autonomousstate.Snapshot, error) {
	prevID, err := autonomousstate.StateIdentityFor(current.SourcePath, true, current.State)
	if err != nil {
		return current, err
	}
	nextID, err := autonomousstate.StateIdentityFor(current.SourcePath, true, next)
	if err != nil {
		return current, err
	}
	material, _ := json.Marshal(struct {
		Operation string                        `json:"operation"`
		Stage     autonomous.FinalizationStage  `json:"stage"`
		Detail    autonomous.FinalizationDetail `json:"detail"`
	}{next.Finalization.OperationID, next.Finalization.Stage, *next.Finalization})
	sum := sha256.Sum256(material)
	history := autonomousstate.FinalizationHistoryRecord{SchemaVersion: autonomousstate.FinalizationHistorySchemaVersion, TaskID: next.TaskID, OperationID: next.Finalization.OperationID, ApplicationSHA256: fmt.Sprintf("%x", sum), Stage: next.Finalization.Stage, CreatedAt: createdAt, Finalization: *next.Finalization, PreviousState: prevID, ResultingState: nextID}
	result, err := cfg.StateStore.CommitFinalization(ctx, autonomousstate.FinalizationCommitRequest{TaskID: next.TaskID, Expected: current.Expected(), PreviousState: current.State, NextState: next, History: history})
	if err != nil {
		return current, err
	}
	return result.Current, nil
}

const LedgerEventSchemaVersion = "autonomous-finalization-ledger-event-v2"
const LegacyLedgerEventSchemaVersion = "autonomous-finalization-ledger-event-v1"

type LedgerEvent struct {
	SchemaVersion  string                           `json:"schema_version"`
	TaskID         string                           `json:"task_id"`
	OperationID    string                           `json:"operation_id"`
	Stage          autonomous.FinalizationStage     `json:"stage"`
	SourceRevision string                           `json:"source_revision"`
	PolicySHA256   string                           `json:"policy_sha256"`
	Manifest       *autonomous.FinalizationArtifact `json:"manifest,omitempty"`
	AdmittedAt     time.Time                        `json:"admitted_at"`
	TerminalAt     time.Time                        `json:"terminal_at"`
}

func stageEvent(e FrozenEvidence, stage autonomous.FinalizationStage, m *autonomous.FinalizationArtifact) LedgerEvent {
	return LedgerEvent{LedgerEventSchemaVersion, e.Task.TaskID, e.OperationID, stage, e.Source.Revision, e.SafetyPolicy.PolicySHA256, m, e.AdmittedAt, e.TerminalAt}
}

func DecodeLedgerEvent(raw []byte) (LedgerEvent, error) {
	var event LedgerEvent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return event, errors.New("finalization ledger event contains trailing JSON")
	}
	if event.SchemaVersion != LedgerEventSchemaVersion {
		return event, fmt.Errorf("unknown finalization ledger schema %q", event.SchemaVersion)
	}
	if event.TaskID == "" || event.OperationID == "" || event.AdmittedAt.IsZero() || event.TerminalAt.Before(event.AdmittedAt) {
		return event, errors.New("finalization ledger event authority is malformed")
	}
	return event, nil
}

func ensureRun(ctx context.Context, l Ledger, e FrozenEvidence) error {
	existing, ok, err := l.GetRunWithEvents(ctx, e.FinalizationRunID)
	if err != nil {
		return err
	}
	spec := ledger.RunSpec{ID: e.FinalizationRunID, TaskID: e.Task.TaskID, Task: e.Task.Title, Status: ledger.StatusRunning, Summary: "autonomous terminal finalization", StartedAt: e.AdmittedAt, VerificationStatus: "passed", CommitSHA: e.Workspace.HeadSHA}
	if !ok {
		_, err = l.CreateRun(ctx, spec)
		return err
	}
	r := existing.Run
	if r.ID != spec.ID || r.TaskID != spec.TaskID || r.Task != spec.Task || !r.StartedAt.Equal(spec.StartedAt) || r.CommitSHA != spec.CommitSHA || r.VerificationStatus != "passed" {
		return errors.New("finalize autonomous task: existing finalization run conflicts with exact operation")
	}
	return nil
}
func ensureEvent(ctx context.Context, l Ledger, runID string, kind ledger.EventType, payload any) error {
	raw, _ := json.Marshal(payload)
	history, ok, err := l.GetRunWithEvents(ctx, runID)
	if err != nil || !ok {
		return errors.Join(err, errors.New("finalization ledger run missing"))
	}
	for _, event := range history.Events {
		if event.Type != kind {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(event.Payload), raw) {
			return nil
		}
		return fmt.Errorf("finalization ledger event %q conflicts with existing payload", kind)
	}
	_, err = l.AppendEvent(ctx, runID, kind, payload)
	return err
}
func completeRunExact(ctx context.Context, l Ledger, e FrozenEvidence, manifest autonomous.FinalizationArtifact) (ledger.Run, error) {
	completion := ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: "completed with capsule " + manifest.Path + " " + manifest.SHA256, CompletedAt: e.TerminalAt, VerificationStatus: "passed", CommitSHA: e.Workspace.HeadSHA}
	existing, ok, err := l.GetRunWithEvents(ctx, e.FinalizationRunID)
	if err != nil || !ok {
		return ledger.Run{}, errors.Join(err, errors.New("finalization ledger run missing"))
	}
	if existing.Run.Status != ledger.StatusRunning {
		if runMatchesCompletion(existing.Run, completion) {
			return existing.Run, nil
		}
		return ledger.Run{}, errors.New("finalization ledger run terminal evidence conflicts")
	}
	run, updated, err := l.CompleteRun(ctx, e.FinalizationRunID, completion)
	if err != nil || !updated {
		return ledger.Run{}, errors.Join(err, errors.New("finalization ledger completion was not applied"))
	}
	return run, nil
}
func runMatchesCompletion(r ledger.Run, c ledger.RunCompletion) bool {
	return r.Status == c.Status && r.Summary == c.Summary && r.CompletedAt != nil && r.CompletedAt.Equal(c.CompletedAt) && r.VerificationStatus == c.VerificationStatus && r.CommitSHA == c.CommitSHA
}
func artifact(path string, raw []byte) autonomous.FinalizationArtifact {
	sum := sha256.Sum256(raw)
	return autonomous.FinalizationArtifact{Path: path, SHA256: fmt.Sprintf("%x", sum), ByteSize: len(raw)}
}
func canonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}
func loadExactTask(root string, source TaskSource, status string) (taskfile.Task, error) {
	task, ok, err := taskfile.FindByID(root, source.TaskID)
	if err != nil || !ok {
		return task, errors.Join(err, errors.New("canonical task missing"))
	}
	if task.SourcePath != source.Path || task.SourceSHA256() != source.SHA256 || task.SourceByteSize() != source.ByteSize || task.Title != source.Title || task.Workflow != source.Workflow || task.AutonomousStatePath != source.StatePath || task.Status != status {
		return task, errors.New("canonical task source identity drifted")
	}
	return task, nil
}
func fail(cfg Config, point FailurePoint) error {
	if cfg.FailureInjector == nil {
		return nil
	}
	return cfg.FailureInjector(point)
}
func stageLess(got, want autonomous.FinalizationStage) bool {
	order := map[autonomous.FinalizationStage]int{autonomous.FinalizationStageAdmitted: 1, autonomous.FinalizationStageMaterialized: 2, autonomous.FinalizationStageTaskCompleted: 3, autonomous.FinalizationStageStateCompleted: 4, autonomous.FinalizationStageLedgerCompleted: 5}
	return order[got] < order[want]
}

func writeImmutable(root string, id autonomous.FinalizationArtifact, raw []byte) error {
	abs, err := pathguard.Resolve(root, filepath.FromSlash(id.Path))
	if err != nil {
		return err
	}
	if err := ensureSafeParents(root, filepath.Dir(abs)); err != nil {
		return err
	}
	if existing, err := os.ReadFile(abs); err == nil {
		if bytes.Equal(existing, raw) {
			return verifyArtifact(root, id, raw)
		}
		return fmt.Errorf("completion artifact %q already exists with different bytes", id.Path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(abs), ".completion.tmp-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, abs); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(abs))
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return err
	}
	return verifyArtifact(root, id, raw)
}
func verifyArtifact(root string, id autonomous.FinalizationArtifact, want []byte) error {
	abs, err := pathguard.Resolve(root, filepath.FromSlash(id.Path))
	if err != nil {
		return err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("completion artifact is not a regular non-symlink file")
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	got := artifact(id.Path, raw)
	if got != id || !bytes.Equal(raw, want) {
		return errors.New("completion artifact readback identity mismatch")
	}
	return nil
}
func ensureSafeParents(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("completion artifact path has unsafe parent component")
		}
	}
	return nil
}

func redactBytes(redactor *redact.Redactor, raw []byte) []byte {
	if redactor == nil {
		return raw
	}
	return []byte(redactor.String(string(raw)))
}
