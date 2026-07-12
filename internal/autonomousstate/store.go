// Package autonomousstate owns canonical AW-08 state loading and atomic,
// compare-and-swap persistence for planning, audit, attempt, and optional-role
// transitions.
package autonomousstate

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
	"runtime"
	"sort"
	"strings"
	"syscall"

	"revolvr/internal/autonomous"
	"revolvr/internal/pathguard"
	"revolvr/internal/taskfile"
)

var (
	ErrTaskMissing       = errors.New("canonical autonomous task is missing")
	ErrStateMissing      = errors.New("canonical autonomous state is missing")
	ErrStateExists       = errors.New("canonical autonomous state already exists")
	ErrStaleWrite        = errors.New("stale autonomous state compare-and-swap")
	ErrOperationConflict = errors.New("autonomous operation ID content conflict")
	ErrUnsafePath        = errors.New("unsafe autonomous state or history path")
)

type FailurePoint string

const (
	FailureBeforeHistoryCreate  FailurePoint = "before_history_create"
	FailureDuringHistoryWrite   FailurePoint = "during_history_write"
	FailureHistoryFileSync      FailurePoint = "history_file_sync"
	FailureHistoryDirectorySync FailurePoint = "history_directory_sync"
	FailureAfterHistoryWrite    FailurePoint = "after_history_write"
	FailureDuringStateWrite     FailurePoint = "during_state_temporary_write"
	FailureStateFileSync        FailurePoint = "state_temporary_file_sync"
	FailureBeforeStateRename    FailurePoint = "before_state_rename"
	FailureStateRename          FailurePoint = "state_rename"
	FailureAfterStateRename     FailurePoint = "after_state_rename"
	FailureStateDirectorySync   FailurePoint = "state_directory_sync"
	FailureStateReadback        FailurePoint = "state_readback"
	FailureBeforeAuditOutput    FailurePoint = "before_canonical_audit_output"
	FailureDuringAuditOutput    FailurePoint = "during_canonical_audit_output_write"
	FailureBeforeAuditHistory   FailurePoint = "before_audit_history_create"
	FailureDuringAuditHistory   FailurePoint = "during_audit_history_write"
	FailureAfterAuditHistory    FailurePoint = "after_audit_history_write"
)

type FailureInjector func(FailurePoint) error

type Config struct {
	RepositoryRoot  string
	FailureInjector FailureInjector
}

type Store struct {
	root   string
	inject FailureInjector
}

type ExpectedState struct {
	Exists   bool
	SHA256   string
	ByteSize int
}

type Snapshot struct {
	State      autonomous.ExecutionState
	SHA256     string
	ByteSize   int
	SourcePath string
}

func (s Snapshot) Expected() ExpectedState {
	return ExpectedState{Exists: true, SHA256: s.SHA256, ByteSize: s.ByteSize}
}

type HistorySnapshot struct {
	Record     PlanningHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}

type CommitRequest struct {
	TaskID          string
	Expected        ExpectedState
	PreviousState   autonomous.ExecutionState
	NextState       autonomous.ExecutionState
	History         PlanningHistoryRecord
	CanonicalOutput []byte
}

type CommitDisposition string

const (
	CommitCreated  CommitDisposition = "created"
	CommitUpdated  CommitDisposition = "updated"
	CommitReplayed CommitDisposition = "replayed"
)

type CommitResult struct {
	Disposition CommitDisposition
	Previous    StateIdentity
	Current     Snapshot
	History     HistorySnapshot
}

func New(cfg Config) (*Store, error) {
	root := strings.TrimSpace(cfg.RepositoryRoot)
	if root == "" {
		return nil, errors.New("open autonomous state store: repository root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("open autonomous state store: resolve repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("open autonomous state store: resolve repository root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("open autonomous state store: repository root is not a directory: %w", err)
	}
	return &Store{root: resolved, inject: cfg.FailureInjector}, nil
}

// Load performs no writes and accepts only canonical deterministic state JSON.
func (s *Store) Load(ctx context.Context, taskID string) (Snapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, false, err
	}
	return s.readCurrent(task)
}

func (s *Store) LoadPlanningOperation(ctx context.Context, taskID, operationID string) (HistorySnapshot, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return HistorySnapshot{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return HistorySnapshot{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return HistorySnapshot{}, false, err
	}
	return s.readOperation(task, operationID)
}

// ReplayPlanning confirms an already committed operation under the same
// compare-and-swap lock and re-syncs its immutable evidence and state
// directories. It returns found=false for a missing or orphaned operation so
// the caller can continue normal recovery with the original proposal.
func (s *Store) ReplayPlanning(ctx context.Context, taskID, operationID, applicationSHA256 string) (CommitResult, bool, error) {
	task, err := s.canonicalTask(taskID)
	if err != nil {
		return CommitResult{}, false, err
	}
	if err := validateIdentity("operation_id", operationID); err != nil {
		return CommitResult{}, false, err
	}
	if !validSHA256(applicationSHA256) {
		return CommitResult{}, false, errors.New("replay planning transition: application SHA-256 is invalid")
	}
	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	lockFile, err := s.openLock(filepath.ToSlash(filepath.Join(namespace, "state.lock")))
	if err != nil {
		return CommitResult{}, false, err
	}
	defer lockFile.Close()
	if err := flockContext(ctx, lockFile); err != nil {
		return CommitResult{}, false, err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	history, found, err := s.readOperation(task, operationID)
	if err != nil || !found {
		return CommitResult{}, false, err
	}
	if history.Record.ApplicationSHA256 != applicationSHA256 {
		return CommitResult{}, false, fmt.Errorf("%w: operation %q has different application evidence", ErrOperationConflict, operationID)
	}
	current, stateFound, err := s.readCurrent(task)
	if err != nil {
		return CommitResult{}, false, err
	}
	if !stateFound || current.SHA256 != history.Record.ResultingState.SHA256 || current.ByteSize != history.Record.ResultingState.ByteSize {
		return CommitResult{}, false, nil
	}
	if err := s.verifyArtifact(history.Record.CanonicalOutput); err != nil {
		return CommitResult{}, false, err
	}
	paths := []string{history.Record.CanonicalOutput.Path, history.SourcePath, task.AutonomousStatePath}
	for _, rel := range paths {
		abs, err := s.safePath(rel)
		if err != nil {
			return CommitResult{}, false, err
		}
		if err := syncDirectory(filepath.Dir(abs)); err != nil {
			return CommitResult{}, false, fmt.Errorf("replay planning transition: sync directory for %s: %w", rel, err)
		}
	}
	return CommitResult{Disposition: CommitReplayed, Previous: history.Record.PreviousState, Current: current, History: history}, true, nil
}

func (s *Store) CommitPlanning(ctx context.Context, request CommitRequest) (CommitResult, error) {
	task, err := s.canonicalTask(request.TaskID)
	if err != nil {
		return CommitResult{}, err
	}
	if err := request.Expected.Validate(); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: expected state: %w", err)
	}
	if err := request.PreviousState.Validate(); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: previous state: %w", err)
	}
	if err := request.NextState.Validate(); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: next state: %w", err)
	}
	if err := autonomous.ValidateExecutionStateTransition(request.PreviousState, request.NextState); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: %w", err)
	}
	if request.PreviousState.TaskID != request.TaskID || request.NextState.TaskID != request.TaskID {
		return CommitResult{}, errors.New("commit planning transition: state task identity mismatch")
	}
	if err := request.History.Validate(); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: history: %w", err)
	}
	if request.History.TaskID != request.TaskID {
		return CommitResult{}, errors.New("commit planning transition: history task identity mismatch")
	}
	previousBytes, err := MarshalState(request.PreviousState)
	if err != nil {
		return CommitResult{}, err
	}
	nextBytes, err := MarshalState(request.NextState)
	if err != nil {
		return CommitResult{}, err
	}
	previousIdentity := stateIdentity(task.AutonomousStatePath, request.Expected.Exists, previousBytes)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextBytes)
	if request.History.PreviousState != previousIdentity || request.History.ResultingState != resultingIdentity {
		return CommitResult{}, errors.New("commit planning transition: history state identities do not match exact canonical state bytes")
	}
	canonicalIdentity := artifactIdentity(request.History.CanonicalOutput.Path, request.CanonicalOutput)
	if request.History.CanonicalOutput != canonicalIdentity {
		return CommitResult{}, errors.New("commit planning transition: canonical output identity does not match exact bytes")
	}

	namespace := filepath.ToSlash(filepath.Dir(task.AutonomousStatePath))
	if err := s.ensureDirectory(namespace, 0o755); err != nil {
		return CommitResult{}, err
	}
	lockPath := filepath.ToSlash(filepath.Join(namespace, "state.lock"))
	lockFile, err := s.openLock(lockPath)
	if err != nil {
		return CommitResult{}, err
	}
	defer lockFile.Close()
	if err := flockContext(ctx, lockFile); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: acquire compare-and-swap lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	existingHistory, historyFound, err := s.readOperation(task, request.History.OperationID)
	if err != nil {
		return CommitResult{}, err
	}
	current, currentFound, err := s.readCurrent(task)
	if err != nil {
		return CommitResult{}, err
	}
	if historyFound {
		if err := sameOperation(existingHistory.Record, request.History); err != nil {
			return CommitResult{}, err
		}
		if currentFound && current.SHA256 == request.History.ResultingState.SHA256 && current.ByteSize == request.History.ResultingState.ByteSize {
			if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(task.AutonomousStatePath)))); err != nil {
				return CommitResult{}, fmt.Errorf("commit planning transition: recover state directory sync: %w", err)
			}
			return CommitResult{Disposition: CommitReplayed, Previous: request.History.PreviousState, Current: current, History: existingHistory}, nil
		}
	}
	if err := compareExpected(request.Expected, current, currentFound); err != nil {
		return CommitResult{}, err
	}
	if historyFound && currentFound && current.SHA256 != request.History.PreviousState.SHA256 {
		return CommitResult{}, fmt.Errorf("%w: orphaned operation history previous hash %q does not match current hash %q", ErrStaleWrite, request.History.PreviousState.SHA256, current.SHA256)
	}

	canonicalCreated, err := s.writeImmutable(request.History.CanonicalOutput.Path, request.CanonicalOutput, "canonical planner output", "")
	if err != nil {
		return CommitResult{}, err
	}
	if canonicalCreated {
		canonicalAbs, err := s.safePath(request.History.CanonicalOutput.Path)
		if err != nil {
			return CommitResult{}, err
		}
		if err := syncDirectory(filepath.Dir(canonicalAbs)); err != nil {
			return CommitResult{}, fmt.Errorf("commit planning transition: sync canonical output directory: %w", err)
		}
	}
	history := existingHistory
	if !historyFound {
		if err := s.fail(FailureBeforeHistoryCreate); err != nil {
			return CommitResult{}, err
		}
		historyPath := planningHistoryPath(request.TaskID, request.History.ResultingPlan.Revision, request.History.OperationID)
		historyBytes, err := MarshalPlanningHistory(request.History)
		if err != nil {
			return CommitResult{}, err
		}
		created, err := s.writeImmutable(historyPath, historyBytes, "planning history", FailureDuringHistoryWrite)
		if err != nil {
			return CommitResult{}, err
		}
		if !created {
			return CommitResult{}, fmt.Errorf("%w: history appeared concurrently", ErrOperationConflict)
		}
		if err := s.fail(FailureHistoryFileSync); err != nil {
			return CommitResult{}, err
		}
		if err := s.fail(FailureHistoryDirectorySync); err != nil {
			return CommitResult{}, err
		}
		if err := syncDirectory(filepath.Dir(filepath.Join(s.root, filepath.FromSlash(historyPath)))); err != nil {
			return CommitResult{}, fmt.Errorf("commit planning transition: sync history directory: %w", err)
		}
		history = HistorySnapshot{Record: request.History, SourcePath: historyPath, SHA256: hashBytes(historyBytes), ByteSize: len(historyBytes)}
	}
	if err := s.fail(FailureAfterHistoryWrite); err != nil {
		return CommitResult{}, err
	}

	statePath, err := s.safePath(task.AutonomousStatePath)
	if err != nil {
		return CommitResult{}, err
	}
	temp, err := os.CreateTemp(filepath.Dir(statePath), ".state.json.tmp-*")
	if err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: create state temporary file: %w", err)
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o644); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: chmod state temporary file: %w", err)
	}
	if err := s.fail(FailureDuringStateWrite); err != nil {
		return CommitResult{}, err
	}
	if _, err := temp.Write(nextBytes); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: write state temporary file: %w", err)
	}
	if err := s.fail(FailureStateFileSync); err != nil {
		return CommitResult{}, err
	}
	if err := temp.Sync(); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: sync state temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		closed = true
		return CommitResult{}, fmt.Errorf("commit planning transition: close state temporary file: %w", err)
	}
	closed = true

	latest, latestFound, err := s.readCurrent(task)
	if err != nil {
		return CommitResult{}, err
	}
	if err := compareExpected(request.Expected, latest, latestFound); err != nil {
		return CommitResult{}, err
	}
	if err := s.fail(FailureBeforeStateRename); err != nil {
		return CommitResult{}, err
	}
	if err := s.fail(FailureStateRename); err != nil {
		return CommitResult{}, err
	}
	if err := os.Rename(tempPath, statePath); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: atomically replace state: %w", err)
	}
	if err := s.fail(FailureAfterStateRename); err != nil {
		return CommitResult{}, err
	}
	if err := s.fail(FailureStateDirectorySync); err != nil {
		return CommitResult{}, err
	}
	if err := syncDirectory(filepath.Dir(statePath)); err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: sync state directory: %w", err)
	}
	if err := s.fail(FailureStateReadback); err != nil {
		return CommitResult{}, err
	}
	readback, found, err := s.readCurrent(task)
	if err != nil {
		return CommitResult{}, fmt.Errorf("commit planning transition: reopen committed state: %w", err)
	}
	if !found || readback.SHA256 != resultingIdentity.SHA256 || readback.ByteSize != resultingIdentity.ByteSize || !reflectStateEqual(readback.State, request.NextState) {
		return CommitResult{}, errors.New("commit planning transition: reopened state does not match committed state")
	}
	disposition := CommitUpdated
	if !request.Expected.Exists {
		disposition = CommitCreated
	}
	return CommitResult{Disposition: disposition, Previous: previousIdentity, Current: readback, History: history}, nil
}

func MarshalState(state autonomous.ExecutionState) ([]byte, error) {
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("marshal autonomous state: %w", err)
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal autonomous state: %w", err)
	}
	return append(raw, '\n'), nil
}

// StateIdentityFor returns the exact canonical identity used by immutable
// transition history without exposing store path internals.
func StateIdentityFor(path string, persisted bool, state autonomous.ExecutionState) (StateIdentity, error) {
	raw, err := MarshalState(state)
	if err != nil {
		return StateIdentity{}, err
	}
	return stateIdentity(path, persisted, raw), nil
}

func DecodeState(raw []byte, taskID string) (autonomous.ExecutionState, error) {
	var state autonomous.ExecutionState
	if err := decodeStrict(raw, &state); err != nil {
		return autonomous.ExecutionState{}, fmt.Errorf("decode autonomous state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return autonomous.ExecutionState{}, err
	}
	if state.TaskID != taskID {
		return autonomous.ExecutionState{}, fmt.Errorf("decode autonomous state: state task_id %q does not match requested task_id %q", state.TaskID, taskID)
	}
	canonical, err := MarshalState(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return autonomous.ExecutionState{}, errors.New("decode autonomous state: bytes are not canonical deterministic JSON")
	}
	return state, nil
}

func (e ExpectedState) Validate() error {
	if !e.Exists {
		if e.SHA256 != "" || e.ByteSize != 0 {
			return errors.New("absent expectation must not include SHA-256 or byte size")
		}
		return nil
	}
	if !validSHA256(e.SHA256) || e.ByteSize <= 0 {
		return errors.New("present expectation requires a valid SHA-256 and positive byte size")
	}
	return nil
}

func (s *Store) canonicalTask(taskID string) (taskfile.Task, error) {
	if err := validateIdentity("task_id", taskID); err != nil {
		return taskfile.Task{}, err
	}
	task, found, err := taskfile.FindByID(s.root, taskID)
	if err != nil {
		return taskfile.Task{}, fmt.Errorf("load canonical autonomous task %q: %w", taskID, err)
	}
	if !found {
		return taskfile.Task{}, fmt.Errorf("%w: %q", ErrTaskMissing, taskID)
	}
	if task.Workflow != taskfile.WorkflowAutonomousV1 || task.AutonomousStatePath != canonicalStatePath(taskID) {
		return taskfile.Task{}, fmt.Errorf("load canonical autonomous task %q: workflow/state path is not canonical autonomous-v1 metadata", taskID)
	}
	if _, err := s.safePath(task.AutonomousStatePath); err != nil {
		return taskfile.Task{}, err
	}
	return task, nil
}

func (s *Store) readCurrent(task taskfile.Task) (Snapshot, bool, error) {
	abs, err := s.safePath(task.AutonomousStatePath)
	if err != nil {
		return Snapshot{}, false, err
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{SourcePath: task.AutonomousStatePath}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("load autonomous state: inspect %s: %w", task.AutonomousStatePath, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Snapshot{}, false, fmt.Errorf("%w: state path %q is not a regular non-symlink file", ErrUnsafePath, task.AutonomousStatePath)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("load autonomous state %s: %w", task.AutonomousStatePath, err)
	}
	state, err := DecodeState(raw, task.ID)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("load autonomous state %s: %w", task.AutonomousStatePath, err)
	}
	return Snapshot{State: state, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: task.AutonomousStatePath}, true, nil
}

func (s *Store) readOperation(task taskfile.Task, operationID string) (HistorySnapshot, bool, error) {
	dirRel := filepath.ToSlash(filepath.Join(filepath.Dir(task.AutonomousStatePath), "history", "planning"))
	dir, err := s.safePath(dirRel)
	if err != nil {
		return HistorySnapshot{}, false, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return HistorySnapshot{}, false, nil
	}
	if err != nil {
		return HistorySnapshot{}, false, fmt.Errorf("load planning history: %w", err)
	}
	suffix := "-" + operationHash(operationID) + ".json"
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return HistorySnapshot{}, false, nil
	}
	if len(names) != 1 {
		return HistorySnapshot{}, false, fmt.Errorf("%w: operation %q has multiple immutable history records", ErrOperationConflict, operationID)
	}
	rel := filepath.ToSlash(filepath.Join(dirRel, names[0]))
	abs, err := s.safePath(rel)
	if err != nil {
		return HistorySnapshot{}, false, err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return HistorySnapshot{}, false, fmt.Errorf("load planning history %s: %w", rel, err)
	}
	record, err := DecodePlanningHistory(raw)
	if err != nil {
		return HistorySnapshot{}, false, fmt.Errorf("load planning history %s: %w", rel, err)
	}
	if record.TaskID != task.ID || record.OperationID != operationID {
		return HistorySnapshot{}, false, fmt.Errorf("load planning history %s: task or operation identity mismatch", rel)
	}
	return HistorySnapshot{Record: record, SHA256: hashBytes(raw), ByteSize: len(raw), SourcePath: rel}, true, nil
}

func (s *Store) writeImmutable(rel string, content []byte, label string, point FailurePoint) (bool, error) {
	abs, err := s.safePath(rel)
	if err != nil {
		return false, err
	}
	if err := s.ensureDirectory(filepath.ToSlash(filepath.Dir(rel)), 0o755); err != nil {
		return false, err
	}
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(abs)
		if readErr != nil {
			return false, fmt.Errorf("read existing immutable %s: %w", label, readErr)
		}
		if !bytes.Equal(existing, content) {
			return false, fmt.Errorf("%w: existing immutable %s %q has different content", ErrOperationConflict, label, rel)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create immutable %s %q: %w", label, rel, err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(abs)
		}
	}()
	if point != "" {
		if err := s.fail(point); err != nil {
			return false, err
		}
	}
	if _, err := file.Write(content); err != nil {
		return false, fmt.Errorf("write immutable %s %q: %w", label, rel, err)
	}
	if err := file.Sync(); err != nil {
		return false, fmt.Errorf("sync immutable %s %q: %w", label, rel, err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close immutable %s %q: %w", label, rel, err)
	}
	ok = true
	return true, nil
}

func (s *Store) verifyArtifact(identity ArtifactIdentity) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	abs, err := s.safePath(identity.Path)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("read immutable artifact %s: %w", identity.Path, err)
	}
	if hashBytes(raw) != identity.SHA256 || len(raw) != identity.ByteSize {
		return fmt.Errorf("%w: immutable artifact %s no longer matches its identity", ErrOperationConflict, identity.Path)
	}
	return nil
}

func (s *Store) ensureDirectory(rel string, perm os.FileMode) error {
	clean := filepath.Clean(filepath.FromSlash(rel))
	current := s.root
	for _, component := range strings.Split(clean, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, perm); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create autonomous state directory %s: %w", rel, err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return fmt.Errorf("inspect autonomous state directory %s: %w", rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: path component %q is not a non-symlink directory", ErrUnsafePath, component)
		}
	}
	return nil
}

func (s *Store) safePath(rel string) (string, error) {
	abs, err := pathguard.Resolve(s.root, filepath.FromSlash(rel))
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %v", ErrUnsafePath, rel, err)
	}
	current := s.root
	for _, component := range strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", fmt.Errorf("%w: inspect path component %q: %v", ErrUnsafePath, component, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: path component %q is a symbolic link", ErrUnsafePath, component)
		}
	}
	return abs, nil
}

func (s *Store) openLock(rel string) (*os.File, error) {
	abs, err := s.safePath(rel)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open autonomous state lock: %w", err)
	}
	return file, nil
}

func (s *Store) fail(point FailurePoint) error {
	if s.inject == nil {
		return nil
	}
	if err := s.inject(point); err != nil {
		return fmt.Errorf("autonomous state transition: injected failure at %s: %w", point, err)
	}
	return nil
}

func compareExpected(expected ExpectedState, current Snapshot, found bool) error {
	if expected.Exists && !found {
		return fmt.Errorf("%w: caller expected existing state", ErrStateMissing)
	}
	if !expected.Exists && found {
		return fmt.Errorf("%w: caller expected state path to be absent", ErrStateExists)
	}
	if expected.Exists && (expected.SHA256 != current.SHA256 || expected.ByteSize != current.ByteSize) {
		return fmt.Errorf("%w: expected %s/%d, observed %s/%d", ErrStaleWrite, expected.SHA256, expected.ByteSize, current.SHA256, current.ByteSize)
	}
	return nil
}

func sameOperation(existing, requested PlanningHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: operation %q was reused for materially different input", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	left, _ := MarshalPlanningHistory(existing)
	right, err := MarshalPlanningHistory(normalized)
	if err != nil || !bytes.Equal(left, right) {
		return fmt.Errorf("%w: operation %q history content differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	return nil
}

func flockContext(ctx context.Context, file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			runtime.Gosched()
		}
	}
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func canonicalStatePath(taskID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json"))
}

func planningHistoryPath(taskID string, revision int64, operationID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "history", "planning", fmt.Sprintf("%020d-%s.json", revision, operationHash(operationID))))
}

func operationHash(operationID string) string {
	sum := sha256.Sum256([]byte(operationID))
	return fmt.Sprintf("%x", sum)
}

func stateIdentity(path string, persisted bool, raw []byte) StateIdentity {
	return StateIdentity{Path: path, Persisted: persisted, SHA256: hashBytes(raw), ByteSize: len(raw)}
}

func artifactIdentity(path string, raw []byte) ArtifactIdentity {
	return ArtifactIdentity{Path: path, SHA256: hashBytes(raw), ByteSize: len(raw)}
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func reflectStateEqual(left, right autonomous.ExecutionState) bool {
	leftRaw, _ := json.Marshal(left)
	rightRaw, _ := json.Marshal(right)
	return bytes.Equal(leftRaw, rightRaw)
}
