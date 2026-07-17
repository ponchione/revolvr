package autonomousstate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/taskfile"
)

type AuditCommitRequest struct {
	TaskID          string
	Expected        ExpectedState
	PreviousState   autonomous.ExecutionState
	NextState       autonomous.ExecutionState
	History         AuditHistoryRecord
	CanonicalOutput []byte
}

type AuditCommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     AuditHistorySnapshot
}

func (s *Store) LoadAuditOperation(ctx context.Context, taskID, operationID string) (AuditHistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return AuditHistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return AuditHistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return AuditHistorySnapshot{}, false, err
	}
	return s.readAuditOperation(task, operationID)
}

// LoadAuditHistory returns strict immutable evidence in deterministic sequence
// order. Callers must use LoadCurrentAudit for authority; an orphan is history
// evidence only and is never current.
func (s *Store) LoadAuditHistory(ctx context.Context, taskID string) ([]AuditHistorySnapshot, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.readAuditHistory(task)
}

// LoadCommittedAuditHistory walks the immutable state-transition chain
// backwards from canonical state across planning, audit, attempt, optional-role,
// input-lifecycle, and workspace records. This excludes orphan audit files while retaining
// reports that predate later evidence-only transitions.
func (s *Store) LoadCommittedAuditHistory(ctx context.Context, taskID string) ([]AuditHistorySnapshot, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, found, err := s.readCurrent(task)
	if err != nil || !found {
		return nil, err
	}
	audits, err := s.readAuditHistory(task)
	if err != nil {
		return nil, err
	}
	plans, err := s.readAllPlanningHistory(task)
	if err != nil {
		return nil, err
	}
	attempts, err := s.readAllAttemptHistory(task)
	if err != nil {
		return nil, err
	}
	optionalRoles, err := s.readAllOptionalRoleHistory(task)
	if err != nil {
		return nil, err
	}
	inputs, err := s.readAllInputHistory(task)
	if err != nil {
		return nil, err
	}
	workspaces, err := s.readAllWorkspaceHistory(task)
	if err != nil {
		return nil, err
	}
	finalizations, err := s.readAllFinalizationHistory(task)
	if err != nil {
		return nil, err
	}
	type edge struct {
		previous StateIdentity
		audit    *AuditHistorySnapshot
	}
	edges := make(map[string]edge, len(audits)+len(plans))
	add := func(result StateIdentity, value edge) error {
		key := stateKey(result)
		if _, exists := edges[key]; exists {
			return fmt.Errorf("%w: multiple transitions produce state %s", ErrOperationConflict, key)
		}
		edges[key] = value
		return nil
	}
	for i := range audits {
		if err := add(audits[i].Record.ResultingState, edge{previous: audits[i].Record.PreviousState, audit: &audits[i]}); err != nil {
			return nil, err
		}
	}
	for i := range plans {
		if err := add(plans[i].Record.ResultingState, edge{previous: plans[i].Record.PreviousState}); err != nil {
			return nil, err
		}
	}
	for i := range attempts {
		if err := add(attempts[i].Record.ResultingState, edge{previous: attempts[i].Record.PreviousState}); err != nil {
			return nil, err
		}
	}
	for i := range optionalRoles {
		if err := add(optionalRoles[i].Record.ResultingState, edge{previous: optionalRoles[i].Record.PreviousState}); err != nil {
			return nil, err
		}
	}
	for i := range inputs {
		if err := add(inputs[i].Record.ResultingState, edge{previous: inputs[i].Record.PreviousState}); err != nil {
			return nil, err
		}
	}
	for i := range workspaces {
		if err := add(workspaces[i].Record.ResultingState, edge{previous: workspaces[i].Record.PreviousState}); err != nil {
			return nil, err
		}
	}
	for i := range finalizations {
		if err := add(finalizations[i].Record.ResultingState, edge{previous: finalizations[i].Record.PreviousState}); err != nil {
			return nil, err
		}
	}
	key := fmt.Sprintf("%s/%d", current.SHA256, current.ByteSize)
	seen := map[string]bool{}
	var committed []AuditHistorySnapshot
	for key != "" && !seen[key] {
		seen[key] = true
		transition, ok := edges[key]
		if !ok {
			break
		}
		if transition.audit != nil {
			committed = append(committed, *transition.audit)
		}
		if !transition.previous.Persisted {
			break
		}
		key = stateKey(transition.previous)
	}
	for i, j := 0, len(committed)-1; i < j; i, j = i+1, j-1 {
		committed[i], committed[j] = committed[j], committed[i]
	}
	return committed, nil
}

// LoadCurrentAudit performs no writes. Current authority is the newest audit
// on the immutable transition chain ending at canonical state; later planning,
// attempt-accounting, optional-role, or input-lifecycle evidence does not hide
// an otherwise current source-bound audit.
func (s *Store) LoadCurrentAudit(ctx context.Context, taskID string) (AuditSnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return AuditSnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return AuditSnapshot{}, false, err
	}
	current, found, err := s.readCurrent(task)
	if err != nil || !found {
		return AuditSnapshot{}, false, err
	}
	records, err := s.LoadCommittedAuditHistory(ctx, taskID)
	if err != nil {
		return AuditSnapshot{}, false, err
	}
	var selected *AuditHistorySnapshot
	for i := range records {
		record := &records[i]
		if selected == nil || record.Record.Sequence > selected.Record.Sequence {
			selected = record
		}
	}
	if selected == nil {
		return AuditSnapshot{}, false, nil
	}
	artifacts := []ArtifactIdentity{
		selected.Record.SupervisorDecision,
		selected.Record.TaskSource,
		selected.Record.RawOutput,
		selected.Record.CanonicalOutput,
	}
	for _, artifact := range artifacts {
		if err := s.verifyArtifact(artifact); err != nil {
			return AuditSnapshot{}, false, err
		}
	}
	if err := s.verifyProfileArtifact(selected.Record.Profile); err != nil {
		return AuditSnapshot{}, false, err
	}
	rawOutput, err := s.readArtifactBytes(selected.Record.RawOutput)
	if err != nil {
		return AuditSnapshot{}, false, err
	}
	canonicalOutput, err := s.readArtifactBytes(selected.Record.CanonicalOutput)
	if err != nil {
		return AuditSnapshot{}, false, err
	}
	rawEnvelope, err := autonomousaudit.ParseAuditOutput(rawOutput)
	if err != nil {
		return AuditSnapshot{}, false, fmt.Errorf("reopen raw audit output: %w", err)
	}
	canonicalEnvelope, err := autonomousaudit.ParseAuditOutput(canonicalOutput)
	if err != nil {
		return AuditSnapshot{}, false, fmt.Errorf("reopen canonical audit output: %w", err)
	}
	canonicalBytes, err := autonomousaudit.MarshalAuditOutput(canonicalEnvelope)
	if err != nil || !bytes.Equal(canonicalBytes, canonicalOutput) {
		return AuditSnapshot{}, false, errors.New("reopen canonical audit output: bytes are not canonical")
	}
	if !reflect.DeepEqual(rawEnvelope, canonicalEnvelope) || !reflect.DeepEqual(canonicalEnvelope.Report, selected.Record.Report) || canonicalEnvelope.Provenance.WorkerRunID != selected.Record.WorkerRunID || canonicalEnvelope.Provenance.Decision != selected.Record.Decision {
		return AuditSnapshot{}, false, errors.New("reopen audit output: raw, canonical, and history evidence disagree")
	}
	return AuditSnapshot{State: current, Revision: selected.Record.AuditRevision, Report: selected.Record.Report, PolicyEvidence: selected.Record.PolicyEvidence, CanonicalOutput: selected.Record.CanonicalOutput, History: *selected}, true, nil
}

func (s *Store) ReplayAudit(ctx context.Context, taskID, operationID, applicationSHA string) (AuditCommitResult, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return AuditCommitResult{}, false, err
	}
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	lockLease, err := s.acquireLock(ctx, filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return AuditCommitResult{}, false, err
	}
	defer lockLease.Close()
	history, found, err := s.readAuditOperation(task, operationID)
	if err != nil || !found {
		return AuditCommitResult{}, false, err
	}
	if history.Record.ApplicationSHA256 != applicationSHA {
		return AuditCommitResult{}, false, fmt.Errorf("%w: operation %q has different application evidence", ErrOperationConflict, operationID)
	}
	current, stateFound, err := s.readCurrent(task)
	if err != nil {
		return AuditCommitResult{}, false, err
	}
	if !stateFound || current.SHA256 != history.Record.ResultingState.SHA256 || current.ByteSize != history.Record.ResultingState.ByteSize {
		return AuditCommitResult{}, false, nil
	}
	if err := s.verifyArtifact(history.Record.CanonicalOutput); err != nil {
		return AuditCommitResult{}, false, err
	}
	for _, rel := range []string{history.Record.CanonicalOutput.Path, history.SourcePath, task.AutonomousStatePath} {
		abs, err := s.safePath(rel)
		if err != nil {
			return AuditCommitResult{}, false, err
		}
		if err := s.syncDirectory(filepath.Dir(abs)); err != nil {
			return AuditCommitResult{}, false, err
		}
	}
	return AuditCommitResult{Disposition: CommitReplayed, Previous: history.Record.PreviousState, Current: current, History: history}, true, nil
}

func (s *Store) CommitAudit(ctx context.Context, request AuditCommitRequest) (AuditCommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return AuditCommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil || !request.Expected.Exists {
		return AuditCommitResult{}, errors.New("commit audit transition: an exact existing state expectation is required")
	}
	if err := request.PreviousState.Validate(); err != nil {
		return AuditCommitResult{}, err
	}
	if err := request.NextState.Validate(); err != nil {
		return AuditCommitResult{}, err
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return AuditCommitResult{}, err
	}
	if err := request.History.Validate(); err != nil {
		return AuditCommitResult{}, err
	}
	previousBytes, _ := MarshalState(request.PreviousState)
	nextBytes, _ := MarshalState(request.NextState)
	previousIdentity := stateIdentity(task.AutonomousStatePath, true, previousBytes)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextBytes)
	if request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return AuditCommitResult{}, errors.New("commit audit transition: history state identities do not match canonical state bytes")
	}
	if request.History.CanonicalOutput != artifactIdentity(request.History.CanonicalOutput.Path, request.CanonicalOutput) {
		return AuditCommitResult{}, errors.New("commit audit transition: canonical output identity mismatch")
	}

	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return AuditCommitResult{}, err
	}
	lockLease, err := s.acquireLock(ctx, filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return AuditCommitResult{}, err
	}
	defer lockLease.Close()
	existing, historyFound, err := s.readAuditOperation(task, request.History.OperationID)
	if err != nil {
		return AuditCommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return AuditCommitResult{}, err
	}
	if historyFound {
		if err := sameAuditOperation(existing.Record, request.History); err != nil {
			return AuditCommitResult{}, err
		}
		if currentFound && current.SHA256 == request.History.ResultingState.SHA256 && current.ByteSize == request.History.ResultingState.ByteSize {
			return AuditCommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existing}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return AuditCommitResult{}, err
	}
	records, err := s.readAuditHistory(task)
	if err != nil {
		return AuditCommitResult{}, err
	}
	maxSequence, maxAuditRevision := int64(0), int64(0)
	for _, record := range records {
		if record.Record.Sequence > maxSequence {
			maxSequence = record.Record.Sequence
		}
		if record.Record.AuditRevision > maxAuditRevision {
			maxAuditRevision = record.Record.AuditRevision
		}
	}
	if !historyFound {
		if request.History.Sequence != maxSequence+1 {
			return AuditCommitResult{}, fmt.Errorf("commit audit transition: sequence %d is stale (want %d)", request.History.Sequence, maxSequence+1)
		}
		wantRevision := maxAuditRevision
		if request.History.Kind == AuditTransitionRecorded {
			wantRevision++
		}
		if request.History.AuditRevision != wantRevision {
			return AuditCommitResult{}, fmt.Errorf("commit audit transition: audit revision %d is stale (want %d)", request.History.AuditRevision, wantRevision)
		}
	}
	if err := s.failAudit(FailureBeforeAuditOutput); err != nil {
		return AuditCommitResult{}, err
	}
	created, err := s.writeImmutable(request.History.CanonicalOutput.Path, request.CanonicalOutput, "canonical audit output", FailureDuringAuditOutput, lockLease)
	if err != nil {
		return AuditCommitResult{}, err
	}
	if created {
		abs, _ := s.safePath(request.History.CanonicalOutput.Path)
		if err := s.syncDirectory(filepath.Dir(abs)); err != nil {
			return AuditCommitResult{}, err
		}
	}
	history := existing
	if !historyFound {
		if err := s.failAudit(FailureBeforeAuditHistory); err != nil {
			return AuditCommitResult{}, err
		}
		historyPath := auditHistoryPath(request.TaskID, request.History.Sequence, request.History.Kind, request.History.OperationID)
		historyBytes, err := MarshalAuditHistory(request.History)
		if err != nil {
			return AuditCommitResult{}, err
		}
		created, err := s.writeImmutable(historyPath, historyBytes, "audit history", FailureDuringAuditHistory, lockLease)
		if err != nil {
			return AuditCommitResult{}, err
		}
		if !created {
			return AuditCommitResult{}, fmt.Errorf("%w: audit history appeared concurrently", ErrOperationConflict)
		}
		if err := s.syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(historyPath)))); err != nil {
			return AuditCommitResult{}, err
		}
		history = AuditHistorySnapshot{Record: request.History, SourcePath: historyPath, SHA256: hashBytes(historyBytes), ByteSize: len(historyBytes)}
	}
	if err := s.failAudit(FailureAfterAuditHistory); err != nil {
		return AuditCommitResult{}, err
	}

	readback, found, err := s.replaceState(task, request.Expected, nextBytes, lockLease)
	if err != nil {
		return AuditCommitResult{}, err
	}
	if !found {
		return AuditCommitResult{}, errors.New("commit audit transition: state readback missing")
	}
	if readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize {
		return AuditCommitResult{}, errors.New("commit audit transition: state readback mismatch")
	}
	return AuditCommitResult{Disposition: CommitUpdated, Previous: previousIdentity, Current: readback, History: history}, nil
}

func (s *Store) readAuditHistory(task taskfile.Task) ([]AuditHistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "audit"))
	dir, found, err := s.openDir(dirRel, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	defer dir.Close()
	entries, err := dir.ReadDir()
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	result := make([]AuditHistorySnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(dirRel, entry.Name()))
		raw, found, err := dir.ReadFile(entry.Name(), false)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, os.ErrNotExist
		}
		record, err := DecodeAuditHistory(raw)
		if err != nil {
			return nil, fmt.Errorf("load audit history %s: %w", rel, err)
		}
		if record.TaskID != task.ID {
			return nil, errors.New("load audit history: wrong task association")
		}
		result = append(result, AuditHistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Record.Sequence < result[j].Record.Sequence })
	return result, nil
}

func (s *Store) readAllPlanningHistory(task taskfile.Task) ([]HistorySnapshot, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "planning"))
	dir, found, err := s.openDir(dirRel, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	defer dir.Close()
	entries, err := dir.ReadDir()
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var result []HistorySnapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join(dirRel, entry.Name()))
		raw, found, err := dir.ReadFile(entry.Name(), false)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, os.ErrNotExist
		}
		record, err := DecodePlanningHistory(raw)
		if err != nil {
			return nil, err
		}
		if record.TaskID != task.ID {
			return nil, errors.New("load planning history: wrong task association")
		}
		result = append(result, HistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel})
	}
	return result, nil
}
func stateKey(value StateIdentity) string { return fmt.Sprintf("%s/%d", value.SHA256, value.ByteSize) }

func (s *Store) readAuditOperation(task taskfile.Task, operationID string) (AuditHistorySnapshot, bool, error) {
	records, err := s.readAuditHistory(task)
	if err != nil {
		return AuditHistorySnapshot{}, false, err
	}
	var matches []AuditHistorySnapshot
	for _, record := range records {
		if record.Record.OperationID == operationID {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return AuditHistorySnapshot{}, false, nil
	}
	if len(matches) != 1 {
		return AuditHistorySnapshot{}, false, fmt.Errorf("%w: duplicate audit operation %q", ErrOperationConflict, operationID)
	}
	return matches[0], true, nil
}
func sameAuditOperation(existing, requested AuditHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: operation %q material differs", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	left, _ := MarshalAuditHistory(existing)
	right, err := MarshalAuditHistory(normalized)
	if err != nil || !bytes.Equal(left, right) {
		return fmt.Errorf("%w: operation %q history differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}
func auditHistoryPath(taskID string, sequence int64, kind AuditTransitionKind, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "audit", fmt.Sprintf("%020d-%s-%s.json", sequence, kind, operationHash(operationID))))
}
func (s *Store) failAudit(point FailurePoint) error {
	if s.inject == nil {
		return nil
	}
	if err := s.inject(point); err != nil {
		return fmt.Errorf("commit audit transition: injected failure at %s: %w", point, err)
	}
	return nil
}

func (s *Store) readArtifactBytes(identity ArtifactIdentity) ([]byte, error) {
	raw, found, err := s.readFile(identity.Path, false)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	if hashBytes(raw) != identity.SHA256 || len(raw) != identity.ByteSize {
		return nil, fmt.Errorf("%w: immutable artifact %s no longer matches its identity", ErrOperationConflict, identity.Path)
	}
	return raw, nil
}

func (s *Store) verifyProfileArtifact(identity autonomousaudit.ProfileIdentity) error {
	raw, found, err := s.readFile(identity.Path, false)
	if err != nil {
		return err
	}
	if !found {
		return os.ErrNotExist
	}
	normalized := []byte(strings.TrimSpace(string(raw)))
	exact := hashBytes(raw) == identity.SHA256 && len(raw) == identity.ByteSize
	trimmed := hashBytes(normalized) == identity.SHA256 && len(normalized) == identity.ByteSize
	if !exact && !trimmed {
		return fmt.Errorf("%w: immutable artifact %s no longer matches its identity", ErrOperationConflict, identity.Path)
	}
	return nil
}
