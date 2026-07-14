package lock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"revolvr/internal/runtimepath"
)

const (
	SourceWriterRelPath      = ".revolvr/locks/source-writer.lock"
	ArtifactRetentionRelPath = ".revolvr/locks/artifact-retention.lock"
)

const defaultTimeout = 5 * time.Minute

var (
	ErrHeld         = errors.New("source-writer lock is held")
	ErrNotOwner     = errors.New("source-writer lock is held by another owner")
	ErrLeaseExpired = errors.New("source-writer lease expired")
)

type Config struct {
	WorkingDir    string
	ControlRoot   string
	ExecutionRoot string
	WorkspaceID   string
	RunID         string
	PID           int
	Timeout       time.Duration
	Clock         func() time.Time
}

type Metadata struct {
	RunID         string    `json:"run_id"`
	PID           int       `json:"pid"`
	AcquiredAt    time.Time `json:"acquired_at"`
	HeartbeatAt   time.Time `json:"heartbeat_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	ExecutionRoot string    `json:"execution_root,omitempty"`
}

type SourceWriter struct {
	root          string
	path          string
	fileIdentity  os.FileInfo
	runID         string
	pid           int
	timeout       time.Duration
	clock         func() time.Time
	workspaceID   string
	executionRoot string
	retentionMu   sync.Mutex
	retentionFile *Flock
}

type HeldError struct {
	Metadata Metadata
}

type ExpiredError struct {
	Metadata   Metadata
	ObservedAt time.Time
}

func (e *ExpiredError) Error() string {
	return fmt.Sprintf("source-writer lease for run %s expired at %s before ownership check at %s", e.Metadata.RunID, e.Metadata.ExpiresAt.Format(time.RFC3339Nano), e.ObservedAt.Format(time.RFC3339Nano))
}

func (e *ExpiredError) Unwrap() error { return ErrLeaseExpired }

func (e *HeldError) Error() string {
	return fmt.Sprintf(
		"source-writer lock is held by run %s pid %d until %s",
		e.Metadata.RunID,
		e.Metadata.PID,
		e.Metadata.ExpiresAt.Format(time.RFC3339Nano),
	)
}

func (e *HeldError) Unwrap() error {
	return ErrHeld
}

func AcquireSourceWriter(ctx context.Context, cfg Config) (*SourceWriter, error) {
	cfg, root, path, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	retentionFile, err := acquireRetentionAdmission(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("acquire source-writer retention admission: %w", err)
	}
	keepRetention := false
	defer func() {
		if !keepRetention {
			releaseRetentionAdmission(retentionFile)
		}
	}()

	fileLock, err := openSourceWriterLock(ctx, root, path, true)
	if err != nil {
		return nil, err
	}
	defer fileLock.Close()

	current, found, err := readMetadata(fileLock)
	if err != nil {
		return nil, err
	}

	now := cfg.Clock().UTC()
	if found && current.ExpiresAt.After(now) {
		return nil, &HeldError{Metadata: current}
	}

	metadata := Metadata{
		RunID:         cfg.RunID,
		PID:           cfg.PID,
		AcquiredAt:    now,
		HeartbeatAt:   now,
		ExpiresAt:     now.Add(cfg.Timeout),
		WorkspaceID:   cfg.WorkspaceID,
		ExecutionRoot: cfg.ExecutionRoot,
	}
	if err := retentionFile.Check(); err != nil {
		return nil, fmt.Errorf("validate artifact-retention admission before source metadata write: %w", err)
	}
	if err := writeMetadata(fileLock, metadata); err != nil {
		return nil, err
	}
	fileIdentity, err := fileLock.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("capture source-writer lock identity: %w", err)
	}
	if err := fileLock.Check(); err != nil {
		return nil, fmt.Errorf("validate source-writer lock after identity capture: %w", err)
	}

	writer := &SourceWriter{
		root:          root,
		path:          path,
		fileIdentity:  fileIdentity,
		runID:         cfg.RunID,
		pid:           cfg.PID,
		timeout:       cfg.Timeout,
		clock:         cfg.Clock,
		workspaceID:   cfg.WorkspaceID,
		executionRoot: cfg.ExecutionRoot,
		retentionFile: retentionFile,
	}
	keepRetention = true
	return writer, nil
}

func ReadSourceWriter(ctx context.Context, workingDir string) (Metadata, bool, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, false, err
	}
	root, err := runtimepath.CanonicalRoot(workingDir)
	if err != nil {
		return Metadata{}, false, err
	}
	path := filepath.Join(root, filepath.FromSlash(SourceWriterRelPath))
	return readLockPath(ctx, root, path)
}

func ReadWorkspaceSourceWriter(ctx context.Context, controlRoot, workspaceID string) (Metadata, bool, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, false, err
	}
	root, err := runtimepath.CanonicalRoot(controlRoot)
	if err != nil {
		return Metadata{}, false, err
	}
	path, err := WorkspaceSourceWriterPath(root, workspaceID)
	if err != nil {
		return Metadata{}, false, err
	}
	return readLockPath(ctx, root, path)
}

func readLockPath(ctx context.Context, root, path string) (Metadata, bool, error) {
	fileLock, err := openSourceWriterLock(ctx, root, path, false)
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, false, nil
	}
	if err != nil {
		return Metadata{}, false, fmt.Errorf("open source-writer lock: %w", err)
	}
	defer fileLock.Close()
	return readMetadata(fileLock)
}

func (l *SourceWriter) Heartbeat(ctx context.Context) error {
	if l == nil {
		return errors.New("source-writer lock is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	retentionFile, temporaryRetention, err := l.retentionForOperation(ctx)
	if err != nil {
		return err
	}
	if temporaryRetention {
		defer retentionFile.Close()
	}
	if err := retentionFile.Check(); err != nil {
		return fmt.Errorf("validate artifact-retention admission before heartbeat: %w", err)
	}
	fileLock, err := openSourceWriterLock(ctx, l.root, l.path, true)
	if err != nil {
		return err
	}
	defer fileLock.Close()
	if err := l.checkSourceFileIdentity(fileLock); err != nil {
		return err
	}

	current, found, err := readMetadata(fileLock)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotOwner
	}
	if !l.owns(current) {
		return &HeldError{Metadata: current}
	}

	now := l.clock().UTC()
	if !current.ExpiresAt.After(now) {
		return &ExpiredError{Metadata: current, ObservedAt: now}
	}
	current.HeartbeatAt = now
	current.ExpiresAt = now.Add(l.timeout)
	if err := retentionFile.Check(); err != nil {
		return fmt.Errorf("validate artifact-retention admission before heartbeat write: %w", err)
	}
	if err := l.checkSourceFileIdentity(fileLock); err != nil {
		return err
	}
	return writeMetadata(fileLock, current)
}

func (l *SourceWriter) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	defer l.releaseRetentionAdmission()
	if err := ctx.Err(); err != nil {
		return err
	}
	retentionFile, temporaryRetention, err := l.retentionForOperation(ctx)
	if err != nil {
		return err
	}
	if temporaryRetention {
		defer retentionFile.Close()
	}
	if err := retentionFile.Check(); err != nil {
		return fmt.Errorf("validate artifact-retention admission before release: %w", err)
	}
	fileLock, err := openSourceWriterLock(ctx, l.root, l.path, true)
	if err != nil {
		return err
	}
	defer fileLock.Close()
	if err := l.checkSourceFileIdentity(fileLock); err != nil {
		return err
	}

	current, found, err := readMetadata(fileLock)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if !l.owns(current) {
		return &HeldError{Metadata: current}
	}
	now := l.clock().UTC()
	if !current.ExpiresAt.After(now) {
		return &ExpiredError{Metadata: current, ObservedAt: now}
	}
	if err := retentionFile.Check(); err != nil {
		return fmt.Errorf("validate artifact-retention admission before release write: %w", err)
	}
	if err := fileLock.Check(); err != nil {
		return fmt.Errorf("validate source-writer lock before release write: %w", err)
	}
	if err := l.checkSourceFileIdentity(fileLock); err != nil {
		return err
	}
	if err := fileLock.file.Truncate(0); err != nil {
		return fmt.Errorf("release source-writer lock: %w", err)
	}
	if _, err := fileLock.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("release source-writer lock: %w", err)
	}
	if err := fileLock.file.Sync(); err != nil {
		return fmt.Errorf("release source-writer lock: %w", err)
	}
	return fileLock.Check()
}

func (l *SourceWriter) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func SourceWriterPath(workingDir string) (string, error) {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return "", errors.New("source-writer lock: working directory is required")
	}
	root, err := runtimepath.CanonicalRoot(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Join(root, filepath.FromSlash(SourceWriterRelPath)), nil
}

func WorkspaceSourceWriterPath(controlRoot, workspaceID string) (string, error) {
	controlRoot = strings.TrimSpace(controlRoot)
	workspaceID = strings.TrimSpace(workspaceID)
	if controlRoot == "" || workspaceID == "" || strings.ContainsAny(workspaceID, "/\\\r\n") {
		return "", errors.New("workspace source-writer lock: control root and safe workspace ID are required")
	}
	root, err := runtimepath.CanonicalRoot(controlRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".revolvr", "locks", "workspaces", workspaceID, "source-writer.lock"), nil
}

func normalizeConfig(cfg Config) (Config, string, string, error) {
	cfg.RunID = strings.TrimSpace(cfg.RunID)
	if cfg.RunID == "" {
		return Config{}, "", "", errors.New("source-writer lock: run id is required")
	}
	if cfg.PID <= 0 {
		cfg.PID = os.Getpid()
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if strings.TrimSpace(cfg.ControlRoot) != "" || strings.TrimSpace(cfg.ExecutionRoot) != "" || strings.TrimSpace(cfg.WorkspaceID) != "" {
		if strings.TrimSpace(cfg.ControlRoot) == "" || strings.TrimSpace(cfg.ExecutionRoot) == "" || strings.TrimSpace(cfg.WorkspaceID) == "" {
			return Config{}, "", "", errors.New("workspace source-writer lock requires control root, execution root, and workspace ID together")
		}
		control, err := runtimepath.CanonicalRoot(cfg.ControlRoot)
		if err != nil {
			return Config{}, "", "", err
		}
		execution, err := runtimepath.CanonicalRoot(cfg.ExecutionRoot)
		if err != nil {
			return Config{}, "", "", err
		}
		if filepath.Clean(control) == filepath.Clean(execution) {
			return Config{}, "", "", errors.New("workspace source-writer lock requires distinct control and execution roots")
		}
		cfg.ControlRoot, cfg.ExecutionRoot = filepath.Clean(control), filepath.Clean(execution)
		cfg.WorkspaceID = strings.TrimSpace(cfg.WorkspaceID)
		if strings.ContainsAny(cfg.WorkspaceID, "/\\\r\n") {
			return Config{}, "", "", errors.New("workspace source-writer lock requires a safe workspace ID")
		}
		path := filepath.Join(control, ".revolvr", "locks", "workspaces", cfg.WorkspaceID, "source-writer.lock")
		cfg.WorkingDir = cfg.ExecutionRoot
		return cfg, control, path, nil
	}
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		return Config{}, "", "", errors.New("source-writer lock: working directory is required")
	}
	workingDir, err := runtimepath.CanonicalRoot(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", "", err
	}
	cfg.WorkingDir = filepath.Clean(workingDir)
	path := filepath.Join(cfg.WorkingDir, filepath.FromSlash(SourceWriterRelPath))
	return cfg, cfg.WorkingDir, path, nil
}

func acquireRetentionAdmission(ctx context.Context, cfg Config) (*Flock, error) {
	// Every source writer holds a shared control-root retention gate for its
	// complete lease. GC takes the same file exclusively before probing any
	// inner coordinator lock, so an autonomous owner may safely wait here: a GC
	// that encounters that owner fails its nonwaiting inner probe and releases.
	root := cfg.ControlRoot
	if root == "" {
		root = cfg.WorkingDir
	}
	return acquireRetentionAdmissionAtRoot(ctx, root)
}

func acquireRetentionAdmissionAtRoot(ctx context.Context, root string) (*Flock, error) {
	file, err := AcquireFlock(ctx, root, FlockConfig{
		RelativePath: ArtifactRetentionRelPath,
		Mode:         FlockShared,
		Wait:         true,
		Create:       true,
	})
	if err != nil {
		return nil, fmt.Errorf("open artifact-retention lock: %w", err)
	}
	return file, nil
}

func releaseRetentionAdmission(file *Flock) {
	if file == nil {
		return
	}
	_ = file.Close()
}

func (l *SourceWriter) releaseRetentionAdmission() {
	l.retentionMu.Lock()
	file := l.retentionFile
	l.retentionFile = nil
	l.retentionMu.Unlock()
	releaseRetentionAdmission(file)
}

func (l *SourceWriter) retentionForOperation(ctx context.Context) (*Flock, bool, error) {
	l.retentionMu.Lock()
	file := l.retentionFile
	l.retentionMu.Unlock()
	if file != nil {
		return file, false, nil
	}
	file, err := acquireRetentionAdmissionAtRoot(ctx, l.root)
	if err != nil {
		return nil, false, fmt.Errorf("reacquire source-writer retention admission: %w", err)
	}
	return file, true, nil
}

func (l *SourceWriter) checkSourceFileIdentity(fileLock *Flock) error {
	if l == nil || l.fileIdentity == nil || fileLock == nil || fileLock.file == nil {
		return fmt.Errorf("%w: source-writer lock identity is missing", runtimepath.ErrUnsafe)
	}
	current, err := fileLock.file.Stat()
	if err != nil {
		return fmt.Errorf("stat source-writer lock identity: %w", err)
	}
	if !os.SameFile(l.fileIdentity, current) {
		return fmt.Errorf("%w: source-writer lock inode changed during lease", runtimepath.ErrUnsafe)
	}
	return nil
}

func openSourceWriterLock(ctx context.Context, root, path string, create bool) (*Flock, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, fmt.Errorf("resolve source-writer lock path: %w", err)
	}
	file, err := AcquireFlock(ctx, root, FlockConfig{
		RelativePath: rel,
		Mode:         FlockExclusive,
		Wait:         true,
		Create:       create,
	})
	if err != nil {
		return nil, fmt.Errorf("open source-writer lock: %w", err)
	}
	return file, nil
}

func readMetadata(fileLock *Flock) (Metadata, bool, error) {
	if err := fileLock.Check(); err != nil {
		return Metadata{}, false, fmt.Errorf("read source-writer lock: %w", err)
	}
	if _, err := fileLock.file.Seek(0, io.SeekStart); err != nil {
		return Metadata{}, false, fmt.Errorf("read source-writer lock: %w", err)
	}
	content, err := io.ReadAll(fileLock.file)
	if err != nil {
		return Metadata{}, false, fmt.Errorf("read source-writer lock: %w", err)
	}
	if err := fileLock.Check(); err != nil {
		return Metadata{}, false, fmt.Errorf("read source-writer lock: %w", err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return Metadata{}, false, nil
	}
	var metadata Metadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return Metadata{}, false, fmt.Errorf("parse source-writer lock: %w", err)
	}
	if err := validateMetadata(metadata); err != nil {
		return Metadata{}, false, err
	}
	return metadata, true, nil
}

func writeMetadata(fileLock *Flock, metadata Metadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	if err := fileLock.Check(); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	if err := fileLock.file.Truncate(0); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	if _, err := fileLock.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	encoder := json.NewEncoder(fileLock.file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	if err := fileLock.file.Sync(); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	return fileLock.Check()
}

func validateMetadata(metadata Metadata) error {
	if strings.TrimSpace(metadata.RunID) == "" {
		return errors.New("source-writer lock metadata missing run_id")
	}
	if metadata.PID <= 0 {
		return errors.New("source-writer lock metadata missing pid")
	}
	if metadata.AcquiredAt.IsZero() {
		return errors.New("source-writer lock metadata missing acquired_at")
	}
	if metadata.HeartbeatAt.IsZero() {
		return errors.New("source-writer lock metadata missing heartbeat_at")
	}
	if metadata.ExpiresAt.IsZero() {
		return errors.New("source-writer lock metadata missing expires_at")
	}
	return nil
}

func (l *SourceWriter) owns(metadata Metadata) bool {
	return strings.TrimSpace(metadata.RunID) == l.runID && metadata.PID == l.pid && metadata.WorkspaceID == l.workspaceID && metadata.ExecutionRoot == l.executionRoot
}
