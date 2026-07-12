package autonomousarchive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/ledger"
	"revolvr/internal/taskfile"
)

type VerifyConfig struct {
	RepositoryRoot  string
	Ledger          Ledger
	GitExecutable   string
	GitTimeout      time.Duration
	CommandRunner   CommandRunner
	ForbiddenValues []string
}

func Verify(ctx context.Context, cfg VerifyConfig, selector string) (VerificationReport, error) {
	root, git, err := normalizeConfig(Config{RepositoryRoot: cfg.RepositoryRoot, Ledger: cfg.Ledger, GitExecutable: cfg.GitExecutable, GitTimeout: cfg.GitTimeout, CommandRunner: cfg.CommandRunner})
	if err != nil {
		return VerificationReport{}, err
	}
	report := VerificationReport{Passed: true}
	add := func(name string, passed bool, detail string) {
		report.Checks = append(report.Checks, Check{Name: name, Passed: passed, Detail: strings.Join(strings.Fields(detail), " ")})
		if !passed {
			report.Passed = false
		}
	}
	entry, err := Show(root, selector)
	if err != nil {
		add("manifest", false, err.Error())
		return report, nil
	}
	m := entry.Manifest
	report.ArchiveID, report.TaskID = m.ArchiveID, m.TaskID
	add("manifest", true, "canonical archive manifest decoded and validated")
	wantManifestPath := archiveManifestPath(m.ArchivedAt, m.TaskID)
	add("archive_path", entry.ManifestPath == wantManifestPath, fmt.Sprintf("manifest path %s; expected %s", entry.ManifestPath, wantManifestPath))

	taskBytes, taskErr := readArtifactBytes(root, m.ArchivedTask)
	add("task_bytes", taskErr == nil, detailOr(taskErr, "archived task hash and byte size match"))
	if taskErr == nil {
		task, parseErr := taskfile.ParseArchivedTask(root, m.OriginalTask.Path, taskBytes)
		passed := parseErr == nil && task.ID == m.TaskID && task.Workflow == m.Workflow && task.Status == string(m.Disposition) && bytes.Equal(task.SourceBytes, taskBytes)
		add("task_metadata", passed, detailOr(parseErr, "task identity, terminal status, workflow, and exact source are preserved"))
	}

	state, stateBytes, stateErr := readState(root, m.State.Path, m.TaskID)
	statePassed := stateErr == nil && artifact(m.State.Path, stateBytes) == m.State && stateLifecycleDisposition(state.Lifecycle) == m.Disposition && state.Terminal != nil && state.Terminal.Reason == m.Reason && !attemptInFlight(state)
	add("terminal_state", statePassed, detailOr(stateErr, "canonical terminal state, disposition, reason, and closed attempts match"))

	if m.Disposition == DispositionCompleted {
		verifyCompleted(ctx, root, cfg.Ledger, m, state, add)
	} else {
		add("completion_omission", m.CompletionCapsule == nil && m.FrozenEvidence == nil && m.CompletionManifest == nil && m.Finalization == nil && m.TerminalLedger == nil, "non-completed disposition explicitly omits AW-20 completion authority")
	}

	journal, found, journalErr := loadJournal(root, archiveJournalPath(m.TaskID, m.OperationID))
	journalPassed := journalErr == nil && found && journal.Stage == StageLedgerComplete && journal.ArchiveID == m.ArchiveID && validOID(journal.CommitSHA) && journal.Manifest.Path == entry.ManifestPath && journal.Manifest.SHA256 == artifact(entry.ManifestPath, entry.ManifestBytes).SHA256
	add("archive_history", journalPassed, detailOr(journalErr, "immutable runtime journal reached ledger completion and names a reconciled commit"))
	if journalPassed {
		historyErr := verifyHistory(root, journal)
		add("archive_history_chain", historyErr == nil, detailOr(historyErr, "all five immutable monotonic archive history records match the journal"))
		files := map[string][]byte{m.ArchivedTask.Path: taskBytes, entry.ManifestPath: entry.ManifestBytes}
		if m.CompletionCapsule != nil {
			capsule, err := readArtifactBytes(root, *m.CompletionCapsule)
			if err == nil {
				files[m.CompletionCapsule.Path] = capsule
			}
		}
		commitPaths, pathErr := git.commitPaths(ctx, journal.CommitSHA)
		expected := append([]string(nil), m.ExpectedPaths...)
		if pathErr == nil && stringSet(commitPaths)[m.OriginalTask.Path] {
			expected = append(expected, m.OriginalTask.Path)
		}
		commitErr := verifyCommit(ctx, git, journal.CommitSHA, expected, files, archiveTrailers(m))
		add("administrative_commit", pathErr == nil && commitErr == nil, detailOr(errors.Join(pathErr, commitErr), "commit object, identity lines, exact paths, and archived bytes match"))
	}

	ledgerErr := verifyArchiveLedger(ctx, cfg.Ledger, m, journal)
	add("archive_ledger", ledgerErr == nil, detailOr(ledgerErr, "archive ledger run and exact terminal event match runtime history"))
	lineageErr := verifyActiveRelationship(root, m, journal.CommitSHA)
	add("active_relationship", lineageErr == nil, detailOr(lineageErr, "archived task is excluded from active discovery and reopen lineage is unambiguous"))
	secretErr := verifyForbiddenValues(root, m, entry.ManifestBytes, cfg.ForbiddenValues)
	add("persistent_secret_absence", secretErr == nil, detailOr(secretErr, "configured secret values are absent from tracked archive files"))
	return report, nil
}

func verifyCompleted(ctx context.Context, root string, store Ledger, m Manifest, state autonomous.ExecutionState, add func(string, bool, string)) {
	if state.Finalization == nil || m.Finalization == nil || m.FrozenEvidence == nil || m.CompletionManifest == nil || m.CompletionCapsule == nil || m.TerminalLedger == nil {
		add("aw20_finalization", false, "completed archive is missing required AW-20 identities")
		return
	}
	frozen, _, frozenErr := readFrozen(root, *m.FrozenEvidence)
	frozenPassed := frozenErr == nil && frozen.Task.TaskID == m.TaskID && frozen.Task.CompletedSHA256 == m.ArchivedTask.SHA256 && frozen.Task.CompletedByteSize == m.ArchivedTask.ByteSize && frozen.OperationID == m.Finalization.OperationID && frozen.FinalizationRunID == m.Finalization.RunID && frozen.Source.Revision == m.Finalization.SourceRevision && frozen.Workspace.WorkspaceID == m.Finalization.WorkspaceID && frozen.Workspace.Checkpoint.CommitSHA == m.Finalization.CheckpointCommit && frozen.Verification.Summary.RunID == m.Finalization.VerificationRunID && frozen.Audit.RunID == m.Finalization.AuditRunID && frozen.SafetyPolicy.PolicySHA256 == m.Finalization.SafetyPolicySHA
	add("aw20_frozen_evidence", frozenPassed, detailOr(frozenErr, "frozen task/state/source/workspace/checkpoint/verification/audit/safety identities match"))
	completionBytes, completionErr := readArtifactBytes(root, *m.CompletionManifest)
	var completion autonomousfinalization.Manifest
	completionPassed := false
	if completionErr == nil {
		decodeErr := decodeCanonical(completionBytes, &completion)
		completionErr = errors.Join(completionErr, decodeErr)
		completionPassed = decodeErr == nil && completion.Validate() == nil && completion.TaskID == m.TaskID && completion.OperationID == m.Finalization.OperationID && completion.FrozenEvidence.Path == m.FrozenEvidence.Path
	}
	add("aw20_completion_manifest", completionPassed, detailOr(completionErr, "AW-20 completion manifest and frozen evidence identity match"))
	archivedCapsule, archiveCapsuleErr := readArtifactBytes(root, *m.CompletionCapsule)
	activeCapsule := Artifact{Path: state.Finalization.Capsule.Path, SHA256: state.Finalization.Capsule.SHA256, ByteSize: state.Finalization.Capsule.ByteSize}
	activeCapsuleBytes, activeCapsuleErr := readArtifactBytes(root, activeCapsule)
	add("completion_capsule_copy", archiveCapsuleErr == nil && activeCapsuleErr == nil && bytes.Equal(archivedCapsule, activeCapsuleBytes), detailOr(errors.Join(archiveCapsuleErr, activeCapsuleErr), "archived completion.md is byte-identical to verified AW-20 capsule"))
	if frozenErr == nil {
		ledgerIdentity, ledgerErr := terminalLedgerIdentity(ctx, store, frozen, *m.CompletionManifest)
		add("aw20_terminal_ledger", ledgerErr == nil && ledgerIdentity == *m.TerminalLedger, detailOr(ledgerErr, "terminal finalization run/event identity matches"))
	}
}

func verifyHistory(root string, journal Journal) error {
	for sequence := int64(1); sequence <= 5; sequence++ {
		rel := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", journal.TaskID, "archive", "history", fmt.Sprintf("%020d-%s.json", sequence, operationHash(journal.OperationID))))
		abs, err := safePath(root, rel)
		if err != nil {
			return err
		}
		raw, err := readRegular(abs)
		if err != nil {
			return err
		}
		var record HistoryRecord
		if err := decodeCanonical(raw, &record); err != nil {
			return err
		}
		if record.SchemaVersion != HistorySchemaVersion || record.Sequence != sequence || record.ArchiveID != journal.ArchiveID || record.OperationID != journal.OperationID || record.TaskID != journal.TaskID || stageOrder(record.Stage) != int(sequence) || record.Manifest != journal.Manifest || (sequence >= 4 && record.CommitSHA != journal.CommitSHA) {
			return fmt.Errorf("archive history sequence %d conflicts with journal", sequence)
		}
	}
	return nil
}

func verifyArchiveLedger(ctx context.Context, store Ledger, m Manifest, journal Journal) error {
	if store == nil {
		return errors.New("archive ledger is unavailable")
	}
	history, found, err := store.GetRunWithEvents(ctx, m.ArchiveRunID)
	if err != nil || !found {
		return errors.Join(err, errors.New("archive ledger run is missing"))
	}
	if history.Run.Status != ledger.StatusCompleted || history.Run.TaskID != m.TaskID || history.Run.CommitSHA != journal.CommitSHA || history.Run.CompletedAt == nil || !history.Run.CompletedAt.Equal(m.ArchivedAt) {
		return errors.New("archive ledger terminal run identity mismatch")
	}
	wantTypes := []ledger.EventType{ledger.EventArchivePrepared, ledger.EventArchiveFilesPublished, ledger.EventArchiveActiveRemoved, ledger.EventArchiveCommitReconciled, ledger.EventArchiveCompleted}
	for _, kind := range wantTypes {
		count := 0
		for _, event := range history.Events {
			if event.Type != kind {
				continue
			}
			count++
			var payload ledgerArchiveEvent
			if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.SchemaVersion != LedgerEventSchemaVersion || payload.ArchiveID != m.ArchiveID || payload.OperationID != m.OperationID || payload.TaskID != m.TaskID {
				return fmt.Errorf("archive ledger event %s payload mismatch", kind)
			}
		}
		if count != 1 {
			return fmt.Errorf("archive ledger event %s count is %d", kind, count)
		}
	}
	return nil
}

func verifyActiveRelationship(root string, m Manifest, commitSHA string) error {
	active, found, err := taskfile.FindByID(root, m.TaskID)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("archived task id remains active at %s", active.SourcePath)
	}
	tasks, err := taskfile.List(root)
	if err != nil {
		return err
	}
	matches := 0
	for _, task := range tasks {
		if task.Workflow != taskfile.WorkflowAutonomousV1 {
			continue
		}
		state, _, err := readState(root, task.AutonomousStatePath, task.ID)
		if err != nil {
			return err
		}
		if state.ReopenedFrom == nil || state.ReopenedFrom.ArchiveID != m.ArchiveID {
			continue
		}
		matches++
		lineage := state.ReopenedFrom
		if lineage.ArchivedTaskID != m.TaskID || lineage.ArchivedTaskSHA256 != m.ArchivedTask.SHA256 || lineage.ArchivedTaskSize != m.ArchivedTask.ByteSize || lineage.Disposition != string(m.Disposition) || lineage.ArchiveCommitSHA != commitSHA {
			return errors.New("active reopen lineage conflicts with immutable archive")
		}
	}
	if matches > 1 {
		return errors.New("multiple active lifecycles claim the same archive")
	}
	return nil
}

func verifyForbiddenValues(root string, m Manifest, manifestBytes []byte, forbidden []string) error {
	values := make([][]byte, 0, len(forbidden))
	for _, value := range forbidden {
		if value != "" {
			values = append(values, []byte(value))
		}
	}
	files := [][]byte{manifestBytes}
	for _, identity := range []Artifact{m.ArchivedTask} {
		raw, err := readArtifactBytes(root, identity)
		if err != nil {
			return err
		}
		files = append(files, raw)
	}
	if m.CompletionCapsule != nil {
		raw, err := readArtifactBytes(root, *m.CompletionCapsule)
		if err != nil {
			return err
		}
		files = append(files, raw)
	}
	for _, raw := range files {
		for _, value := range values {
			if bytes.Contains(raw, value) {
				return errors.New("configured secret value is present in tracked archive bytes")
			}
		}
	}
	return nil
}

func detailOr(err error, success string) string {
	if err != nil {
		return err.Error()
	}
	return success
}
