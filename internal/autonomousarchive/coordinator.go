package autonomousarchive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/ledger"
	revolvrlock "revolvr/internal/lock"
	"revolvr/internal/runner"
	"revolvr/internal/taskfile"
)

type FailurePoint string

const (
	FailureBeforeHistory         FailurePoint = "before_history"
	FailureAfterHistory          FailurePoint = "after_history"
	FailureBeforeJournalRename   FailurePoint = "before_journal_rename"
	FailureAfterJournalRename    FailurePoint = "after_journal_rename"
	FailureBeforeTaskPublish     FailurePoint = "before_task_publish"
	FailureBeforeCapsulePublish  FailurePoint = "before_capsule_publish"
	FailureBeforeManifestPublish FailurePoint = "before_manifest_publish"
	FailureBeforeActiveRemoval   FailurePoint = "before_active_task_removal"
	FailureAfterActiveRemoval    FailurePoint = "after_active_task_removal"
	FailureBeforeStage           FailurePoint = "before_git_stage"
	FailureBeforeCommit          FailurePoint = "before_git_commit"
	FailureAfterCommit           FailurePoint = "after_git_commit"
	FailureBeforeLedgerEffect    FailurePoint = "before_ledger_effect"
	FailureAfterLedgerEffect     FailurePoint = "after_ledger_effect"
	FailureAfterReturnEvidence   FailurePoint = "after_return_evidence"
)

type Ledger interface {
	CreateRun(context.Context, ledger.RunSpec) (ledger.Run, error)
	GetRunWithEvents(context.Context, string) (ledger.RunWithEvents, bool, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
	CompleteRun(context.Context, string, ledger.RunCompletion) (ledger.Run, bool, error)
}

type Config struct {
	RepositoryRoot  string
	Ledger          Ledger
	GitExecutable   string
	GitTimeout      time.Duration
	CommandRunner   CommandRunner
	FailureInjector func(FailurePoint) error
	ForbiddenValues []string
}

type ArchiveRequest struct {
	TaskID       string
	OperationID  string
	ArchiveRunID string
	Authority    TerminalAuthority
	ArchivedAt   time.Time
}

type ArchiveResult struct {
	Entry     Entry
	Journal   Journal
	CommitSHA string
	Replayed  bool
}

func Archive(ctx context.Context, cfg Config, request ArchiveRequest) (ArchiveResult, error) {
	root, git, err := normalizeConfig(cfg)
	if err != nil {
		return ArchiveResult{}, err
	}
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.ArchiveRunID = strings.TrimSpace(request.ArchiveRunID)
	if !validIdentity(request.TaskID) || !validIdentity(request.OperationID) || request.ArchivedAt.IsZero() || request.ArchivedAt.Location() != time.UTC {
		return ArchiveResult{}, errors.New("archive task: exact task, operation, and UTC archive time are required")
	}
	if err := request.Authority.Validate(); err != nil {
		return ArchiveResult{}, fmt.Errorf("archive task: %w", err)
	}
	archiveID := ArchiveID(request.TaskID, request.OperationID, request.Authority.Disposition, request.ArchivedAt)
	if request.ArchiveRunID == "" {
		request.ArchiveRunID = archiveID
	}
	if !validIdentity(request.ArchiveRunID) {
		return ArchiveResult{}, errors.New("archive task: archive run id is malformed")
	}

	// Git administration is always acquired before the per-task state lock.
	releaseAdmin, err := acquireFileLock(ctx, root, ".revolvr/locks/git-admin.lock")
	if err != nil {
		return ArchiveResult{}, err
	}
	defer releaseAdmin()
	releaseState, err := acquireFileLock(ctx, root, filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", request.TaskID, "state.lock")))
	if err != nil {
		return ArchiveResult{}, err
	}
	defer releaseState()

	journalPath := archiveJournalPath(request.TaskID, request.OperationID)
	journal, journalFound, err := loadJournal(root, journalPath)
	if err != nil {
		return ArchiveResult{}, err
	}
	var manifest Manifest
	var manifestBytes []byte
	replayed := journalFound
	if journalFound {
		if journal.ArchiveID != archiveID || journal.OperationID != request.OperationID || journal.TaskID != request.TaskID {
			return ArchiveResult{}, errors.New("archive task: existing journal conflicts with operation identity")
		}
		manifest, manifestBytes, err = loadManifest(root, journal.Manifest.Path)
		if err != nil {
			return ArchiveResult{}, err
		}
		if artifact(journal.Manifest.Path, manifestBytes) != journal.Manifest {
			return ArchiveResult{}, errors.New("archive task: journal manifest identity mismatch")
		}
	} else {
		manifest, manifestBytes, err = prepareManifest(ctx, root, cfg.Ledger, request, archiveID)
		if err != nil {
			return ArchiveResult{}, err
		}
		if err := rejectActiveWriters(ctx, root); err != nil {
			return ArchiveResult{}, err
		}
		gitEntries, err := git.status(ctx)
		if err != nil {
			return ArchiveResult{}, err
		}
		if err := validateOperationStatus(gitEntries, []string{manifest.OriginalTask.Path}, false); err != nil {
			return ArchiveResult{}, fmt.Errorf("archive task: pre-admission Git state: %w", err)
		}
		if err := rejectForbiddenPersistent(root, manifest, manifestBytes, cfg.ForbiddenValues); err != nil {
			return ArchiveResult{}, err
		}
		existingEntries, err := List(root)
		if err != nil {
			return ArchiveResult{}, err
		}
		for _, entry := range existingEntries {
			if entry.Manifest.ArchiveID == manifest.ArchiveID || entry.Manifest.TaskID == manifest.TaskID {
				return ArchiveResult{}, fmt.Errorf("archive task: archive or task identity already exists at %s", entry.ManifestPath)
			}
		}
		journal = Journal{SchemaVersion: JournalSchemaVersion, ArchiveID: archiveID, OperationID: request.OperationID, TaskID: request.TaskID, Stage: StageAdmitted, Manifest: artifact(archiveManifestPath(request.ArchivedAt, request.TaskID), manifestBytes), UpdatedAt: request.ArchivedAt}
		if err := persistStage(root, cfg, journal, 1); err != nil {
			return ArchiveResult{}, err
		}
	}

	persistCtx := context.WithoutCancel(ctx)
	if err := ensureArchiveRun(persistCtx, cfg.Ledger, manifest); err != nil {
		return ArchiveResult{}, err
	}
	if err := ensureArchiveEvent(persistCtx, cfg.Ledger, manifest.ArchiveRunID, ledger.EventArchivePrepared, archiveEvent(manifest, StageAdmitted, "")); err != nil {
		return ArchiveResult{}, err
	}

	taskBytes, err := readArtifactBytes(root, manifest.OriginalTask)
	if err != nil {
		if stageOrder(journal.Stage) < stageOrder(StageFilesPublished) {
			return ArchiveResult{}, err
		}
		taskBytes, err = readArtifactBytes(root, manifest.ArchivedTask)
		if err != nil || artifact(manifest.OriginalTask.Path, taskBytes).SHA256 != manifest.OriginalTask.SHA256 || len(taskBytes) != manifest.OriginalTask.ByteSize {
			return ArchiveResult{}, errors.Join(err, errors.New("archive task: neither exact active nor archived task bytes are recoverable"))
		}
	}
	if stageOrder(journal.Stage) < stageOrder(StageFilesPublished) {
		if err := fail(cfg, FailureBeforeTaskPublish); err != nil {
			return ArchiveResult{}, err
		}
		if len(taskBytes) == 0 {
			return ArchiveResult{}, errors.New("archive task: active task bytes are unavailable before publication")
		}
		if err := writeImmutable(root, manifest.ArchivedTask, taskBytes); err != nil {
			return ArchiveResult{}, err
		}
		if manifest.CompletionCapsule != nil {
			if err := fail(cfg, FailureBeforeCapsulePublish); err != nil {
				return ArchiveResult{}, err
			}
			activeCapsule := Artifact{Path: manifest.FinalizationCapsuleSource(), SHA256: manifest.CompletionCapsule.SHA256, ByteSize: manifest.CompletionCapsule.ByteSize}
			capsuleBytes, err := readArtifactBytes(root, activeCapsule)
			if err != nil {
				return ArchiveResult{}, err
			}
			if err := writeImmutable(root, *manifest.CompletionCapsule, capsuleBytes); err != nil {
				return ArchiveResult{}, err
			}
		}
		if err := fail(cfg, FailureBeforeManifestPublish); err != nil {
			return ArchiveResult{}, err
		}
		if err := writeImmutable(root, journal.Manifest, manifestBytes); err != nil {
			return ArchiveResult{}, err
		}
		journal.Stage = StageFilesPublished
		journal.UpdatedAt = request.ArchivedAt
		if err := persistStage(root, cfg, journal, 2); err != nil {
			return ArchiveResult{}, err
		}
		if err := ensureArchiveEvent(persistCtx, cfg.Ledger, manifest.ArchiveRunID, ledger.EventArchiveFilesPublished, archiveEvent(manifest, journal.Stage, "")); err != nil {
			return ArchiveResult{}, err
		}
	}

	if stageOrder(journal.Stage) < stageOrder(StageActiveRemoved) {
		if err := fail(cfg, FailureBeforeActiveRemoval); err != nil {
			return ArchiveResult{}, err
		}
		if err := removeExact(root, manifest.OriginalTask); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return ArchiveResult{}, err
			}
		}
		if err := fail(cfg, FailureAfterActiveRemoval); err != nil {
			return ArchiveResult{}, err
		}
		journal.Stage = StageActiveRemoved
		if err := persistStage(root, cfg, journal, 3); err != nil {
			return ArchiveResult{}, err
		}
		if err := ensureArchiveEvent(persistCtx, cfg.Ledger, manifest.ArchiveRunID, ledger.EventArchiveActiveRemoved, archiveEvent(manifest, journal.Stage, "")); err != nil {
			return ArchiveResult{}, err
		}
	}

	if stageOrder(journal.Stage) < stageOrder(StageCommitted) {
		commitSHA, err := commitArchive(persistCtx, cfg, git, manifest, manifestBytes)
		if err != nil {
			return ArchiveResult{}, err
		}
		journal.Stage = StageCommitted
		journal.CommitSHA = commitSHA
		if err := persistStage(root, cfg, journal, 4); err != nil {
			return ArchiveResult{}, err
		}
		if err := ensureArchiveEvent(persistCtx, cfg.Ledger, manifest.ArchiveRunID, ledger.EventArchiveCommitReconciled, archiveEvent(manifest, journal.Stage, commitSHA)); err != nil {
			return ArchiveResult{}, err
		}
	}

	if stageOrder(journal.Stage) < stageOrder(StageLedgerComplete) {
		if err := fail(cfg, FailureBeforeLedgerEffect); err != nil {
			return ArchiveResult{}, err
		}
		if err := ensureArchiveEvent(persistCtx, cfg.Ledger, manifest.ArchiveRunID, ledger.EventArchiveCompleted, archiveEvent(manifest, StageLedgerComplete, journal.CommitSHA)); err != nil {
			return ArchiveResult{}, err
		}
		if err := completeArchiveRun(persistCtx, cfg.Ledger, manifest, journal.CommitSHA); err != nil {
			return ArchiveResult{}, err
		}
		if err := fail(cfg, FailureAfterLedgerEffect); err != nil {
			return ArchiveResult{}, err
		}
		journal.Stage = StageLedgerComplete
		if err := persistStage(root, cfg, journal, 5); err != nil {
			return ArchiveResult{}, err
		}
	}
	if err := fail(cfg, FailureAfterReturnEvidence); err != nil {
		return ArchiveResult{}, err
	}
	return ArchiveResult{Entry: Entry{Manifest: manifest, ManifestBytes: manifestBytes, ManifestPath: journal.Manifest.Path, CommitSHA: journal.CommitSHA}, Journal: journal, CommitSHA: journal.CommitSHA, Replayed: replayed}, nil
}

func prepareManifest(ctx context.Context, root string, archiveLedger Ledger, request ArchiveRequest, archiveID string) (Manifest, []byte, error) {
	task, found, err := taskfile.FindByID(root, request.TaskID)
	if err != nil || !found {
		return Manifest{}, nil, errors.Join(err, errors.New("archive task: canonical active task is missing"))
	}
	if task.Workflow != taskfile.WorkflowAutonomousV1 || task.Status != string(request.Authority.Disposition) {
		return Manifest{}, nil, fmt.Errorf("archive task: task workflow/status %s/%s does not match autonomous terminal disposition %s", task.Workflow, task.Status, request.Authority.Disposition)
	}
	state, stateBytes, err := readState(root, task.AutonomousStatePath, task.ID)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("archive task: terminal state: %w", err)
	}
	if stateLifecycleDisposition(state.Lifecycle) != request.Authority.Disposition || state.Terminal == nil || state.Terminal.Reason != request.Authority.Reason {
		return Manifest{}, nil, errors.New("archive task: terminal state authority does not match disposition and reason")
	}
	if attemptInFlight(state) || state.Lifecycle == autonomous.LifecycleStateBlocked || state.Lifecycle == autonomous.LifecycleStateNeedsInput || state.Lifecycle == autonomous.LifecycleStateFinalizing {
		return Manifest{}, nil, errors.New("archive task: terminal authority is blocked, needs input, finalizing, or has an in-flight attempt")
	}
	base := filepath.ToSlash(filepath.Join(ArchiveRoot, request.ArchivedAt.Format("2006"), request.ArchivedAt.Format("01"), task.ID))
	original := artifact(task.SourcePath, task.SourceBytes)
	manifest := Manifest{SchemaVersion: ManifestSchemaVersion, ArchiveID: archiveID, OperationID: request.OperationID, ArchiveRunID: request.ArchiveRunID, TaskID: task.ID, Disposition: request.Authority.Disposition, Reason: request.Authority.Reason, Provenance: request.Authority.Provenance, TerminalAt: request.Authority.TerminalAt, ArchivedAt: request.ArchivedAt, OriginalTask: original, ArchivedTask: artifact(filepath.ToSlash(filepath.Join(base, "task.md")), task.SourceBytes), Workflow: task.Workflow, State: artifact(task.AutonomousStatePath, stateBytes)}
	manifest.ExpectedPaths = []string{manifest.ArchivedTask.Path, filepath.ToSlash(filepath.Join(base, "archive.json"))}
	if request.Authority.Disposition == DispositionCompleted {
		if state.Finalization == nil || state.Finalization.Stage != autonomous.FinalizationStageLedgerCompleted || state.Finalization.Capsule == nil || state.Finalization.Manifest == nil || state.Finalization.LedgerCompletedAt == nil {
			return Manifest{}, nil, errors.New("archive task: completed state lacks ledger-completed AW-20 finalization")
		}
		frozenID := Artifact{Path: state.Finalization.FrozenEvidence.Path, SHA256: state.Finalization.FrozenEvidence.SHA256, ByteSize: state.Finalization.FrozenEvidence.ByteSize}
		frozen, _, err := readFrozen(root, frozenID)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("archive task: frozen evidence: %w", err)
		}
		if frozen.Task.CompletedSHA256 != original.SHA256 || frozen.Task.CompletedByteSize != original.ByteSize || frozen.OperationID != state.Finalization.OperationID || frozen.FinalizationRunID != state.Finalization.RunID || !frozen.TerminalAt.Equal(request.Authority.TerminalAt) {
			return Manifest{}, nil, errors.New("archive task: frozen task/finalization identity does not match terminal task and state")
		}
		capsuleSource := Artifact{Path: state.Finalization.Capsule.Path, SHA256: state.Finalization.Capsule.SHA256, ByteSize: state.Finalization.Capsule.ByteSize}
		capsuleBytes, err := readArtifactBytes(root, capsuleSource)
		if err != nil {
			return Manifest{}, nil, err
		}
		completionManifest := Artifact{Path: state.Finalization.Manifest.Path, SHA256: state.Finalization.Manifest.SHA256, ByteSize: state.Finalization.Manifest.ByteSize}
		completionBytes, err := readArtifactBytes(root, completionManifest)
		if err != nil {
			return Manifest{}, nil, err
		}
		var aw20 autonomousfinalization.Manifest
		if err := decodeCanonical(completionBytes, &aw20); err != nil || aw20.Validate() != nil || aw20.TaskID != task.ID || aw20.OperationID != frozen.OperationID || aw20.FrozenEvidence.Path != frozenID.Path || aw20.Capsule.Path != capsuleSource.Path {
			return Manifest{}, nil, errors.Join(err, errors.New("archive task: AW-20 completion manifest is malformed or mismatched"))
		}
		terminalLedger, err := terminalLedgerIdentity(ctx, archiveLedger, frozen, completionManifest)
		if err != nil {
			return Manifest{}, nil, err
		}
		archivedCapsule := artifact(filepath.ToSlash(filepath.Join(base, "completion.md")), capsuleBytes)
		manifest.FrozenEvidence = &frozenID
		manifest.CompletionCapsule = &archivedCapsule
		manifest.CompletionManifest = &completionManifest
		manifest.Finalization = &FinalizationIdentity{OperationID: frozen.OperationID, RunID: frozen.FinalizationRunID, Stage: state.Finalization.Stage, SourceRevision: frozen.Source.Revision, WorkspaceID: frozen.Workspace.WorkspaceID, CheckpointCommit: frozen.Workspace.Checkpoint.CommitSHA, VerificationRunID: frozen.Verification.Summary.RunID, AuditRunID: frozen.Audit.RunID, SafetyPolicySHA: frozen.SafetyPolicy.PolicySHA256}
		manifest.TerminalLedger = &terminalLedger
		manifest.ExpectedPaths = append(manifest.ExpectedPaths, archivedCapsule.Path)
		manifest.Omissions = []string{"administrative commit SHA is recorded non-recursively in immutable runtime history and archive ledger evidence"}
	} else {
		manifest.Omissions = []string{"completion capsule omitted because disposition is not completed", "AW-20 frozen evidence and completion manifest are not claimed for this disposition", "administrative commit SHA is recorded non-recursively in immutable runtime history and archive ledger evidence"}
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, nil, err
	}
	raw, err := Marshal(manifest)
	return manifest, raw, err
}

func (m Manifest) FinalizationCapsuleSource() string {
	if m.Finalization == nil {
		return ""
	}
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", m.TaskID, "completion", "completion.md"))
}

func terminalLedgerIdentity(ctx context.Context, store Ledger, frozen autonomousfinalization.FrozenEvidence, completionManifest Artifact) (LedgerIdentity, error) {
	if store == nil {
		return LedgerIdentity{}, errors.New("archive task: ledger is required")
	}
	history, found, err := store.GetRunWithEvents(ctx, frozen.FinalizationRunID)
	if err != nil || !found {
		return LedgerIdentity{}, errors.Join(err, errors.New("archive task: finalization ledger run is missing"))
	}
	if history.Run.Status != ledger.StatusCompleted || history.Run.TaskID != frozen.Task.TaskID || history.Run.VerificationStatus != "passed" || history.Run.CompletedAt == nil || !history.Run.CompletedAt.Equal(frozen.TerminalAt) {
		return LedgerIdentity{}, errors.New("archive task: finalization ledger run terminal identity is mismatched")
	}
	var foundEvent *ledger.Event
	for i := range history.Events {
		if history.Events[i].Type != ledger.EventFinalizationCompleted {
			continue
		}
		if foundEvent != nil {
			return LedgerIdentity{}, errors.New("archive task: duplicate terminal finalization ledger events")
		}
		foundEvent = &history.Events[i]
	}
	if foundEvent == nil {
		return LedgerIdentity{}, errors.New("archive task: terminal finalization ledger event is missing")
	}
	var payload struct {
		SchemaVersion string                           `json:"schema_version"`
		TaskID        string                           `json:"task_id"`
		OperationID   string                           `json:"operation_id"`
		Stage         autonomous.FinalizationStage     `json:"stage"`
		Manifest      *autonomous.FinalizationArtifact `json:"manifest"`
	}
	if err := json.Unmarshal(foundEvent.Payload, &payload); err != nil || payload.SchemaVersion != "autonomous-finalization-ledger-event-v1" || payload.TaskID != frozen.Task.TaskID || payload.OperationID != frozen.OperationID || payload.Stage != autonomous.FinalizationStageLedgerCompleted || payload.Manifest == nil || payload.Manifest.Path != completionManifest.Path || payload.Manifest.SHA256 != completionManifest.SHA256 || payload.Manifest.ByteSize != completionManifest.ByteSize {
		return LedgerIdentity{}, errors.Join(err, errors.New("archive task: terminal finalization ledger payload is mismatched"))
	}
	return LedgerIdentity{RunID: frozen.FinalizationRunID, TerminalEventID: foundEvent.ID, TerminalEventType: string(foundEvent.Type)}, nil
}

func commitArchive(ctx context.Context, cfg Config, git gitConfig, manifest Manifest, manifestBytes []byte) (string, error) {
	paths := append([]string(nil), manifest.ExpectedPaths...)
	paths = append(paths, manifest.OriginalTask.Path)
	sort.Strings(paths)
	entries, err := git.status(ctx)
	if err != nil {
		return "", err
	}
	if err := validateOperationStatus(entries, paths, true); err != nil {
		return "", err
	}
	expectedFiles := map[string][]byte{}
	for _, identity := range []Artifact{manifest.ArchivedTask, {Path: archiveManifestPath(manifest.ArchivedAt, manifest.TaskID), SHA256: artifact(archiveManifestPath(manifest.ArchivedAt, manifest.TaskID), manifestBytes).SHA256, ByteSize: len(manifestBytes)}} {
		bytes, err := readArtifactBytes(git.root, identity)
		if err != nil {
			return "", err
		}
		expectedFiles[identity.Path] = bytes
	}
	if manifest.CompletionCapsule != nil {
		bytes, err := readArtifactBytes(git.root, *manifest.CompletionCapsule)
		if err != nil {
			return "", err
		}
		expectedFiles[manifest.CompletionCapsule.Path] = bytes
	}
	trailers := archiveTrailers(manifest)
	if len(entries) == 0 {
		if head, exists, err := git.head(ctx); err == nil && exists {
			for _, candidate := range [][]string{manifest.ExpectedPaths, paths} {
				if verifyCommit(ctx, git, head, candidate, expectedFiles, trailers) == nil {
					return head, nil
				}
			}
		}
		return "", errors.New("archive git: no operation-owned changes and HEAD does not prove the administrative commit")
	}
	if err := fail(cfg, FailureBeforeStage); err != nil {
		return "", err
	}
	before, beforeExists, err := git.head(ctx)
	if err != nil {
		return "", err
	}
	if err := git.stage(ctx, paths); err != nil {
		return "", err
	}
	staged, err := git.stagedPaths(ctx)
	if err != nil {
		return "", err
	}
	for _, required := range manifest.ExpectedPaths {
		if !stringSet(staged)[required] {
			return "", fmt.Errorf("archive git: required staged archive path %q is missing", required)
		}
	}
	if !subset(staged, paths) {
		return "", fmt.Errorf("archive git: staged paths %v exceed operation authority %v", staged, paths)
	}
	if err := fail(cfg, FailureBeforeCommit); err != nil {
		return "", err
	}
	command := git.commit(ctx, "Archive task "+manifest.TaskID+" ("+string(manifest.Disposition)+")", strings.Join(trailers, "\n"))
	sha, _, err := reconcileCommit(ctx, git, before, beforeExists, command, staged, expectedFiles, trailers)
	if err != nil {
		return "", err
	}
	if err := fail(cfg, FailureAfterCommit); err != nil {
		return "", err
	}
	return sha, nil
}

func archiveTrailers(m Manifest) []string {
	terminal := "disposition:" + string(m.Disposition)
	if m.Finalization != nil {
		terminal = "finalization:" + m.Finalization.OperationID
	}
	return []string{"Archive-Operation: " + m.OperationID, "Archive-ID: " + m.ArchiveID, "Task-ID: " + m.TaskID, "Disposition: " + string(m.Disposition), "Terminal-Identity: " + terminal}
}

type ledgerArchiveEvent struct {
	SchemaVersion string      `json:"schema_version"`
	ArchiveID     string      `json:"archive_id"`
	OperationID   string      `json:"operation_id"`
	TaskID        string      `json:"task_id"`
	Disposition   Disposition `json:"disposition"`
	Stage         Stage       `json:"stage"`
	Manifest      Artifact    `json:"manifest"`
	CommitSHA     string      `json:"commit_sha,omitempty"`
}

func archiveEvent(m Manifest, stage Stage, commit string) ledgerArchiveEvent {
	manifestPath := archiveManifestPath(m.ArchivedAt, m.TaskID)
	return ledgerArchiveEvent{LedgerEventSchemaVersion, m.ArchiveID, m.OperationID, m.TaskID, m.Disposition, stage, artifact(manifestPath, mustMarshal(m)), commit}
}

func ensureArchiveRun(ctx context.Context, store Ledger, m Manifest) error {
	if store == nil {
		return errors.New("archive task: ledger is required")
	}
	history, found, err := store.GetRunWithEvents(ctx, m.ArchiveRunID)
	if err != nil {
		return err
	}
	spec := ledger.RunSpec{ID: m.ArchiveRunID, TaskID: m.TaskID, Task: "archive " + m.TaskID, Status: ledger.StatusRunning, Summary: "tracked terminal task archive", StartedAt: m.ArchivedAt, VerificationStatus: "not_run"}
	if !found {
		_, err = store.CreateRun(ctx, spec)
		return err
	}
	if history.Run.ID != spec.ID || history.Run.TaskID != spec.TaskID || history.Run.Task != spec.Task || !history.Run.StartedAt.Equal(spec.StartedAt) {
		return errors.New("archive task: existing archive ledger run conflicts")
	}
	return nil
}

func ensureArchiveEvent(ctx context.Context, store Ledger, runID string, kind ledger.EventType, payload any) error {
	raw, _ := json.Marshal(payload)
	history, found, err := store.GetRunWithEvents(ctx, runID)
	if err != nil || !found {
		return errors.Join(err, errors.New("archive task: archive ledger run is missing"))
	}
	for _, event := range history.Events {
		if event.Type != kind {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(event.Payload), raw) {
			return nil
		}
		return fmt.Errorf("archive task: ledger event %s conflicts", kind)
	}
	_, err = store.AppendEvent(ctx, runID, kind, payload)
	return err
}

func completeArchiveRun(ctx context.Context, store Ledger, m Manifest, commitSHA string) error {
	history, found, err := store.GetRunWithEvents(ctx, m.ArchiveRunID)
	if err != nil || !found {
		return errors.Join(err, errors.New("archive task: archive ledger run is missing"))
	}
	completion := ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: "archived " + m.ArchiveID, CompletedAt: m.ArchivedAt, VerificationStatus: "passed", CommitSHA: commitSHA}
	if history.Run.Status == ledger.StatusCompleted {
		if history.Run.Summary == completion.Summary && history.Run.CommitSHA == commitSHA && history.Run.CompletedAt != nil && history.Run.CompletedAt.Equal(m.ArchivedAt) {
			return nil
		}
		return errors.New("archive task: terminal archive ledger run conflicts")
	}
	_, updated, err := store.CompleteRun(ctx, m.ArchiveRunID, completion)
	if err != nil || !updated {
		return errors.Join(err, errors.New("archive task: ledger completion was not applied"))
	}
	return nil
}

func persistStage(root string, cfg Config, journal Journal, sequence int64) error {
	if err := fail(cfg, FailureBeforeHistory); err != nil {
		return err
	}
	record := HistoryRecord{SchemaVersion: HistorySchemaVersion, ArchiveID: journal.ArchiveID, OperationID: journal.OperationID, TaskID: journal.TaskID, Sequence: sequence, Stage: journal.Stage, Manifest: journal.Manifest, CommitSHA: journal.CommitSHA, CreatedAt: journal.UpdatedAt}
	raw, err := Marshal(record)
	if err != nil {
		return err
	}
	historyPath := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", journal.TaskID, "archive", "history", fmt.Sprintf("%020d-%s.json", sequence, operationHash(journal.OperationID))))
	if err := writeImmutable(root, artifact(historyPath, raw), raw); err != nil {
		return err
	}
	if err := fail(cfg, FailureAfterHistory); err != nil {
		return err
	}
	journalRaw, err := Marshal(journal)
	if err != nil {
		return err
	}
	if err := fail(cfg, FailureBeforeJournalRename); err != nil {
		return err
	}
	if err := writeMutable(root, archiveJournalPath(journal.TaskID, journal.OperationID), journalRaw); err != nil {
		return err
	}
	return fail(cfg, FailureAfterJournalRename)
}

func loadJournal(root, rel string) (Journal, bool, error) {
	abs, err := safePath(root, rel)
	if err != nil {
		return Journal{}, false, err
	}
	raw, err := readRegular(abs)
	if errors.Is(err, os.ErrNotExist) {
		return Journal{}, false, nil
	}
	if err != nil {
		return Journal{}, false, err
	}
	var journal Journal
	if err := decodeCanonical(raw, &journal); err != nil {
		return Journal{}, false, err
	}
	if journal.SchemaVersion != JournalSchemaVersion || !validArchiveID(journal.ArchiveID) || !validIdentity(journal.OperationID) || !validIdentity(journal.TaskID) || stageOrder(journal.Stage) == 0 || journal.Manifest.Validate() != nil {
		return Journal{}, false, errors.New("archive task: journal is malformed")
	}
	return journal, true, nil
}

func normalizeConfig(cfg Config) (string, gitConfig, error) {
	root, err := canonicalRoot(cfg.RepositoryRoot)
	if err != nil {
		return "", gitConfig{}, err
	}
	executable := strings.TrimSpace(cfg.GitExecutable)
	if executable == "" {
		executable = "git"
	}
	timeout := cfg.GitTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	commandRunner := cfg.CommandRunner
	if commandRunner == nil {
		commandRunner = runner.Run
	}
	return root, gitConfig{root: root, executable: executable, timeout: timeout, runner: commandRunner}, nil
}

func archiveManifestPath(at time.Time, taskID string) string {
	return filepath.ToSlash(filepath.Join(ArchiveRoot, at.Format("2006"), at.Format("01"), taskID, "archive.json"))
}

func archiveJournalPath(taskID, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "archive", "journal-"+operationHash(operationID)+".json"))
}

func stageOrder(stage Stage) int {
	switch stage {
	case StageAdmitted:
		return 1
	case StageFilesPublished:
		return 2
	case StageActiveRemoved:
		return 3
	case StageCommitted:
		return 4
	case StageLedgerComplete:
		return 5
	default:
		return 0
	}
}

func stateLifecycleDisposition(state autonomous.LifecycleState) Disposition {
	switch state {
	case autonomous.LifecycleStateCompleted:
		return DispositionCompleted
	case autonomous.LifecycleStateCancelled:
		return DispositionCancelled
	case autonomous.LifecycleStateSuperseded:
		return DispositionSuperseded
	case autonomous.LifecycleStateAbandoned:
		return DispositionAbandoned
	default:
		return ""
	}
}

func attemptInFlight(state autonomous.ExecutionState) bool {
	open := map[string]bool{}
	for _, event := range state.Attempts.Events {
		if event.Kind == autonomous.AttemptEventAdmitted {
			open[event.AttemptID] = true
		} else if event.Kind == autonomous.AttemptEventCompleted {
			delete(open, event.AttemptID)
		}
	}
	return len(open) != 0
}

func readArtifactBytes(root string, identity Artifact) ([]byte, error) {
	abs, err := safePath(root, identity.Path)
	if err != nil {
		return nil, err
	}
	raw, err := readRegular(abs)
	if err != nil {
		return nil, err
	}
	if artifact(identity.Path, raw) != identity {
		return nil, fmt.Errorf("archive task: artifact %s identity mismatch", identity.Path)
	}
	return raw, nil
}

func fail(cfg Config, point FailurePoint) error {
	if cfg.FailureInjector == nil {
		return nil
	}
	if err := cfg.FailureInjector(point); err != nil {
		return fmt.Errorf("archive task: injected failure at %s: %w", point, err)
	}
	return nil
}

func mustMarshal(value any) []byte {
	raw, _ := Marshal(value)
	return raw
}

func subset(values, allowed []string) bool {
	set := stringSet(allowed)
	for _, value := range values {
		if !set[value] {
			return false
		}
	}
	return true
}

func rejectActiveWriters(ctx context.Context, root string) error {
	if metadata, found, err := revolvrlock.ReadSourceWriter(ctx, root); err != nil {
		return err
	} else if found {
		return fmt.Errorf("archive task: control-root source writer %q is active", metadata.RunID)
	}
	dir := filepath.Join(root, ".revolvr", "locks", "workspaces")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return errors.New("archive task: malformed workspace source-writer namespace")
		}
		metadata, found, err := revolvrlock.ReadWorkspaceSourceWriter(ctx, root, entry.Name())
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("archive task: workspace source writer %q is active", metadata.RunID)
		}
	}
	return nil
}

func rejectForbiddenPersistent(root string, manifest Manifest, manifestBytes []byte, forbidden []string) error {
	materials := [][]byte{manifestBytes}
	taskBytes, err := readArtifactBytes(root, manifest.OriginalTask)
	if err != nil {
		return err
	}
	materials = append(materials, taskBytes)
	if manifest.CompletionCapsule != nil {
		source := Artifact{Path: manifest.FinalizationCapsuleSource(), SHA256: manifest.CompletionCapsule.SHA256, ByteSize: manifest.CompletionCapsule.ByteSize}
		capsule, err := readArtifactBytes(root, source)
		if err != nil {
			return err
		}
		materials = append(materials, capsule)
	}
	for _, value := range forbidden {
		if value == "" {
			continue
		}
		for _, raw := range materials {
			if bytes.Contains(raw, []byte(value)) {
				return errors.New("archive task: configured secret value would cross a persistent archive boundary")
			}
		}
	}
	return nil
}
