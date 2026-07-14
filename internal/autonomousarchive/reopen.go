package autonomousarchive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
)

type ReopenRequest struct {
	Selector    string
	OperationID string
	NewTaskID   string
	Authority   string
	Reason      string
	ReopenedAt  time.Time
}

type ReopenResult struct {
	Record   ReopenRecord
	Task     taskfile.Task
	State    autonomousstate.Snapshot
	Replayed bool
}

func Reopen(ctx context.Context, cfg Config, request ReopenRequest) (ReopenResult, error) {
	boundary, git, err := normalizeConfig(cfg)
	if err != nil {
		return ReopenResult{}, err
	}
	root := boundary.Root()
	reader := newArchiveStorage(boundary, nil)
	request.Selector = strings.TrimSpace(request.Selector)
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.NewTaskID = strings.TrimSpace(request.NewTaskID)
	request.Authority = strings.TrimSpace(request.Authority)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Selector == "" || !validIdentity(request.OperationID) || !validIdentity(request.NewTaskID) || request.ReopenedAt.IsZero() || request.ReopenedAt.Location() != time.UTC {
		return ReopenResult{}, errors.New("reopen archive: selector, operation, new task id, and explicit UTC time are required")
	}
	if err := validateText("reopen authority", request.Authority); err != nil {
		return ReopenResult{}, err
	}
	if err := validateText("reopen reason", request.Reason); err != nil {
		return ReopenResult{}, err
	}
	verifyConfig := VerifyConfig{RepositoryRoot: root, Ledger: cfg.Ledger, GitExecutable: cfg.GitExecutable, GitTimeout: cfg.GitTimeout, CommandRunner: cfg.CommandRunner}
	report, err := verifyWithStorage(ctx, verifyConfig, git, reader, request.Selector)
	if err != nil {
		return ReopenResult{}, err
	}
	if !report.Passed {
		return ReopenResult{}, fmt.Errorf("reopen archive: archive verification failed: %s", failedChecks(report))
	}
	entry, err := reader.show(request.Selector)
	if err != nil {
		return ReopenResult{}, err
	}
	m := entry.Manifest
	journal, found, err := reader.loadJournal(archiveJournalPath(m.TaskID, m.OperationID))
	if err != nil || !found || journal.Stage != StageLedgerComplete {
		return ReopenResult{}, errors.Join(err, errors.New("reopen archive: completed archive journal is missing"))
	}
	recordPath := reopenRecordPath(m.ArchiveID, request.OperationID)
	if existing, ok, err := reader.loadReopenRecord(recordPath); err != nil {
		return ReopenResult{}, err
	} else if ok {
		if existing.ArchiveID != m.ArchiveID || existing.NewTaskID != request.NewTaskID || existing.Lineage.Authority != request.Authority || existing.Lineage.Reason != request.Reason || !existing.CreatedAt.Equal(request.ReopenedAt) {
			return ReopenResult{}, errors.New("reopen archive: operation id was reused for materially different authority")
		}
		task, found, err := taskfile.FindByID(root, request.NewTaskID)
		if err != nil || !found {
			return ReopenResult{}, errors.Join(err, errors.New("reopen archive: replayed active task is missing"))
		}
		state, raw, err := reader.readState(task.AutonomousStatePath, task.ID)
		if err != nil {
			return ReopenResult{}, err
		}
		return ReopenResult{Record: existing, Task: task, State: autonomousstate.Snapshot{State: state, SHA256: artifact(task.AutonomousStatePath, raw).SHA256, ByteSize: len(raw), SourcePath: task.AutonomousStatePath}, Replayed: true}, nil
	}
	expectedArchiveID, expectedTaskID := m.ArchiveID, m.TaskID

	releaseExecution, err := autonomousexec.TryAcquire(root)
	if err != nil {
		return ReopenResult{}, fmt.Errorf("reopen archive: %w", err)
	}
	defer releaseExecution()
	adminLease, err := acquireFileLock(ctx, boundary, ".revolvr/locks/git-admin.lock")
	if err != nil {
		return ReopenResult{}, err
	}
	defer adminLease.Close()
	stateLease, err := acquireFileLock(ctx, boundary, filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", m.TaskID, "state.lock")))
	if err != nil {
		return ReopenResult{}, err
	}
	defer stateLease.Close()
	storage := newArchiveStorage(boundary, func(point FailurePoint) error { return fail(cfg, point) }, adminLease, stateLease)

	report, err = verifyWithStorage(ctx, verifyConfig, git, storage, request.Selector)
	if err != nil || !report.Passed || report.ArchiveID != expectedArchiveID || report.TaskID != expectedTaskID {
		return ReopenResult{}, errors.Join(err, fmt.Errorf("reopen archive: locked revalidation failed: %s", failedChecks(report)))
	}
	entry, err = storage.show(request.Selector)
	if err != nil || entry.Manifest.ArchiveID != expectedArchiveID || entry.Manifest.TaskID != expectedTaskID {
		return ReopenResult{}, errors.Join(err, errors.New("reopen archive: archive identity changed during lock admission"))
	}
	m = entry.Manifest
	journal, found, err = storage.loadJournal(archiveJournalPath(m.TaskID, m.OperationID))
	if err != nil || !found || journal.Stage != StageLedgerComplete {
		return ReopenResult{}, errors.Join(err, errors.New("reopen archive: locked completed archive journal is missing"))
	}
	archivedBytes, err := storage.readArtifact(m.ArchivedTask)
	if err != nil {
		return ReopenResult{}, err
	}
	projected, err := taskfile.ProjectReopenedTask(root, taskfile.ReopenInput{OriginalSourcePath: m.OriginalTask.Path, ArchivedSourceBytes: archivedBytes, NewTaskID: request.NewTaskID})
	if err != nil {
		return ReopenResult{}, err
	}
	lineage := autonomous.ReopenLineage{SchemaVersion: autonomous.ReopenLineageSchemaVersion, OperationID: request.OperationID, ArchiveID: m.ArchiveID, ArchivedTaskID: m.TaskID, ArchivedTaskSHA256: m.ArchivedTask.SHA256, ArchivedTaskSize: m.ArchivedTask.ByteSize, Disposition: string(m.Disposition), ArchiveCommitSHA: journal.CommitSHA, Authority: request.Authority, Reason: request.Reason, ReopenedAt: request.ReopenedAt}
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: request.NewTaskID, Lifecycle: autonomous.LifecycleStatePending, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}, ReopenedFrom: &lineage}
	stateBytes, err := autonomousstate.MarshalState(state)
	if err != nil {
		return ReopenResult{}, err
	}
	stateIdentity := artifact(projected.AutonomousStatePath, stateBytes)
	recovering, err := validateExistingReopen(storage, m.ArchiveID, projected, state, stateIdentity)
	if err != nil {
		return ReopenResult{}, err
	}
	entries, err := git.status(ctx)
	if err != nil {
		return ReopenResult{}, err
	}
	if !recovering && len(entries) != 0 {
		return ReopenResult{}, fmt.Errorf("reopen archive: repository has unrelated dirty paths before admission")
	}
	if recovering {
		if err := validateOperationStatus(entries, []string{projected.SourcePath}, true); err != nil {
			return ReopenResult{}, err
		}
	}
	if err := storage.writeImmutable(stateIdentity, stateBytes); err != nil {
		return ReopenResult{}, err
	}
	published, err := taskfile.PublishReopenedTask(root, projected)
	if err != nil {
		return ReopenResult{}, err
	}
	commitSHA, err := commitReopen(context.WithoutCancel(ctx), git, m, published, request)
	if err != nil {
		return ReopenResult{}, err
	}
	record := ReopenRecord{SchemaVersion: ReopenRecordSchemaVersion, OperationID: request.OperationID, ArchiveID: m.ArchiveID, ArchivedTaskID: m.TaskID, NewTaskID: request.NewTaskID, Task: artifact(published.SourcePath, published.SourceBytes), State: stateIdentity, Lineage: lineage, CommitSHA: commitSHA, CreatedAt: request.ReopenedAt}
	recordBytes, err := Marshal(record)
	if err != nil {
		return ReopenResult{}, err
	}
	if err := storage.writeImmutable(artifact(recordPath, recordBytes), recordBytes); err != nil {
		return ReopenResult{}, err
	}
	if err := ensureArchiveEvent(context.WithoutCancel(ctx), cfg.Ledger, m.ArchiveRunID, ledger.EventArchiveReopened, reopenEvent(record)); err != nil {
		return ReopenResult{}, err
	}
	return ReopenResult{Record: record, Task: published, State: autonomousstate.Snapshot{State: state, SHA256: stateIdentity.SHA256, ByteSize: stateIdentity.ByteSize, SourcePath: stateIdentity.Path}}, nil
}

func commitReopen(ctx context.Context, git gitConfig, m Manifest, task taskfile.Task, request ReopenRequest) (string, error) {
	entries, err := git.status(ctx)
	if err != nil {
		return "", err
	}
	if err := validateOperationStatus(entries, []string{task.SourcePath}, true); err != nil {
		return "", err
	}
	trailers := []string{"Reopen-Operation: " + request.OperationID, "Reopened-From: " + m.ArchiveID, "Archived-Task-ID: " + m.TaskID, "Task-ID: " + request.NewTaskID}
	expectedFiles := map[string][]byte{task.SourcePath: task.SourceBytes}
	if len(entries) == 0 {
		head, exists, err := git.head(ctx)
		if err == nil && exists && verifyCommit(ctx, git, head, []string{task.SourcePath}, expectedFiles, trailers) == nil {
			return head, nil
		}
		return "", errors.New("reopen archive: no task addition and HEAD does not prove the reopen commit")
	}
	before, beforeExists, err := git.head(ctx)
	if err != nil {
		return "", err
	}
	if err := git.stage(ctx, []string{task.SourcePath}); err != nil {
		return "", err
	}
	staged, err := git.stagedPaths(ctx)
	if err != nil || !equalStrings(staged, []string{task.SourcePath}) {
		return "", errors.Join(err, errors.New("reopen archive: staged paths do not equal the new active task"))
	}
	command := git.commit(ctx, "Reopen archived task "+m.TaskID+" as "+request.NewTaskID, strings.Join(trailers, "\n"))
	sha, _, err := reconcileCommit(ctx, git, before, beforeExists, command, staged, expectedFiles, trailers)
	return sha, err
}

func validateExistingReopen(storage *archiveStorage, archiveID string, projected taskfile.Task, expectedState autonomous.ExecutionState, stateIdentity Artifact) (bool, error) {
	root := storage.root()
	tasks, err := taskfile.List(root)
	if err != nil {
		return false, err
	}
	recovering := false
	for _, task := range tasks {
		if task.Workflow != taskfile.WorkflowAutonomousV1 {
			continue
		}
		state, _, err := storage.readState(task.AutonomousStatePath, task.ID)
		if err != nil {
			return false, err
		}
		if state.ReopenedFrom != nil && state.ReopenedFrom.ArchiveID == archiveID {
			if task.ID != projected.ID || !bytes.Equal(task.SourceBytes, projected.SourceBytes) || !reflect.DeepEqual(state, expectedState) {
				return false, fmt.Errorf("reopen archive: archive already reopened as a different active lifecycle %q", task.ID)
			}
			recovering = true
		}
	}
	if !recovering {
		if task, found, err := taskfile.FindByID(root, projected.ID); err != nil {
			return false, err
		} else if found {
			return false, fmt.Errorf("reopen archive: new task id %q already exists at %s", projected.ID, task.SourcePath)
		}
		if _, raw, err := storage.readState(stateIdentity.Path, projected.ID); err == nil && artifact(stateIdentity.Path, raw) == stateIdentity {
			return true, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			// readState wraps decode errors but preserves a missing-path error.
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) || !errors.Is(pathErr.Err, os.ErrNotExist) {
				return false, err
			}
		}
	}
	return recovering, nil
}

func reopenRecordPath(archiveID, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "archives", archiveID, "reopen", operationHash(operationID)+".json"))
}

func (s *archiveStorage) loadReopenRecord(rel string) (ReopenRecord, bool, error) {
	raw, found, err := s.readRegular(rel, true)
	if errors.Is(err, os.ErrNotExist) {
		return ReopenRecord{}, false, nil
	}
	if err != nil {
		return ReopenRecord{}, false, err
	}
	if !found {
		return ReopenRecord{}, false, nil
	}
	var record ReopenRecord
	if err := decodeCanonical(raw, &record); err != nil {
		return ReopenRecord{}, false, err
	}
	if record.SchemaVersion != ReopenRecordSchemaVersion || !validIdentity(record.OperationID) || !validArchiveID(record.ArchiveID) || !validIdentity(record.ArchivedTaskID) || !validIdentity(record.NewTaskID) || record.Task.Validate() != nil || record.State.Validate() != nil || record.Lineage.Validate() != nil || !validOID(record.CommitSHA) || record.CreatedAt.IsZero() {
		return ReopenRecord{}, false, errors.New("reopen archive: persisted reopen record is malformed")
	}
	return record, true, nil
}

func reopenEvent(record ReopenRecord) any {
	return struct {
		SchemaVersion  string `json:"schema_version"`
		OperationID    string `json:"operation_id"`
		ArchiveID      string `json:"archive_id"`
		ArchivedTaskID string `json:"archived_task_id"`
		NewTaskID      string `json:"new_task_id"`
		CommitSHA      string `json:"commit_sha"`
	}{ReopenRecordSchemaVersion, record.OperationID, record.ArchiveID, record.ArchivedTaskID, record.NewTaskID, record.CommitSHA}
}

func failedChecks(report VerificationReport) string {
	values := []string{}
	for _, check := range report.Checks {
		if !check.Passed {
			values = append(values, check.Name+": "+check.Detail)
		}
	}
	return strings.Join(values, "; ")
}
