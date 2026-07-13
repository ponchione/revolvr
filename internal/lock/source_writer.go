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
	"syscall"
	"time"
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
	path          string
	runID         string
	pid           int
	timeout       time.Duration
	clock         func() time.Time
	workspaceID   string
	executionRoot string
	retentionMu   sync.Mutex
	retentionFile *os.File
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
	cfg, path, err := normalizeConfig(cfg)
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

	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if err := lockFile(ctx, file); err != nil {
		return nil, err
	}
	defer unlockFile(file)

	current, found, err := readMetadata(file)
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
	if err := writeMetadata(file, metadata); err != nil {
		return nil, err
	}

	writer := &SourceWriter{
		path:          path,
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
	path, err := SourceWriterPath(workingDir)
	if err != nil {
		return Metadata{}, false, err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, false, nil
	}
	if err != nil {
		return Metadata{}, false, fmt.Errorf("open source-writer lock: %w", err)
	}
	defer file.Close()

	if err := lockFile(ctx, file); err != nil {
		return Metadata{}, false, err
	}
	defer unlockFile(file)

	return readMetadata(file)
}

func ReadWorkspaceSourceWriter(ctx context.Context, controlRoot, workspaceID string) (Metadata, bool, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, false, err
	}
	path, err := WorkspaceSourceWriterPath(controlRoot, workspaceID)
	if err != nil {
		return Metadata{}, false, err
	}
	return readLockPath(ctx, path)
}

func readLockPath(ctx context.Context, path string) (Metadata, bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return Metadata{}, false, nil
	}
	if err != nil {
		return Metadata{}, false, fmt.Errorf("open source-writer lock: %w", err)
	}
	defer file.Close()
	if err := lockFile(ctx, file); err != nil {
		return Metadata{}, false, err
	}
	defer unlockFile(file)
	return readMetadata(file)
}

func (l *SourceWriter) Heartbeat(ctx context.Context) error {
	if l == nil {
		return errors.New("source-writer lock is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := openLockFile(l.path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := lockFile(ctx, file); err != nil {
		return err
	}
	defer unlockFile(file)

	current, found, err := readMetadata(file)
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
	return writeMetadata(file, current)
}

func (l *SourceWriter) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	defer l.releaseRetentionAdmission()
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := openLockFile(l.path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := lockFile(ctx, file); err != nil {
		return err
	}
	defer unlockFile(file)

	current, found, err := readMetadata(file)
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
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("release source-writer lock: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("release source-writer lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("release source-writer lock: %w", err)
	}
	return nil
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
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Join(abs, SourceWriterRelPath), nil
}

func WorkspaceSourceWriterPath(controlRoot, workspaceID string) (string, error) {
	controlRoot = strings.TrimSpace(controlRoot)
	workspaceID = strings.TrimSpace(workspaceID)
	if controlRoot == "" || workspaceID == "" || strings.ContainsAny(workspaceID, "/\\\r\n") {
		return "", errors.New("workspace source-writer lock: control root and safe workspace ID are required")
	}
	abs, err := filepath.Abs(controlRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, ".revolvr", "locks", "workspaces", workspaceID, "source-writer.lock"), nil
}

func normalizeConfig(cfg Config) (Config, string, error) {
	if strings.TrimSpace(cfg.ControlRoot) != "" || strings.TrimSpace(cfg.ExecutionRoot) != "" || strings.TrimSpace(cfg.WorkspaceID) != "" {
		if strings.TrimSpace(cfg.ControlRoot) == "" || strings.TrimSpace(cfg.ExecutionRoot) == "" || strings.TrimSpace(cfg.WorkspaceID) == "" {
			return Config{}, "", errors.New("workspace source-writer lock requires control root, execution root, and workspace ID together")
		}
		control, err := filepath.Abs(cfg.ControlRoot)
		if err != nil {
			return Config{}, "", err
		}
		execution, err := filepath.Abs(cfg.ExecutionRoot)
		if err != nil {
			return Config{}, "", err
		}
		if filepath.Clean(control) == filepath.Clean(execution) {
			return Config{}, "", errors.New("workspace source-writer lock requires distinct control and execution roots")
		}
		cfg.ControlRoot, cfg.ExecutionRoot = filepath.Clean(control), filepath.Clean(execution)
		path, err := WorkspaceSourceWriterPath(cfg.ControlRoot, cfg.WorkspaceID)
		if err != nil {
			return Config{}, "", err
		}
		cfg.WorkingDir = cfg.ExecutionRoot
		if cfg.PID <= 0 {
			cfg.PID = os.Getpid()
		}
		if cfg.Timeout <= 0 {
			cfg.Timeout = defaultTimeout
		}
		if cfg.Clock == nil {
			cfg.Clock = time.Now
		}
		return cfg, path, nil
	}
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		return Config{}, "", errors.New("source-writer lock: working directory is required")
	}
	cfg.RunID = strings.TrimSpace(cfg.RunID)
	if cfg.RunID == "" {
		return Config{}, "", errors.New("source-writer lock: run id is required")
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
	workingDir, err := filepath.Abs(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", err
	}
	cfg.WorkingDir = filepath.Clean(workingDir)
	path := filepath.Join(cfg.WorkingDir, filepath.FromSlash(SourceWriterRelPath))
	return cfg, path, nil
}

func acquireRetentionAdmission(ctx context.Context, cfg Config) (*os.File, error) {
	// Every source writer holds a shared control-root retention gate for its
	// complete lease. GC takes the same file exclusively before probing any
	// inner coordinator lock, so an autonomous owner may safely wait here: a GC
	// that encounters that owner fails its nonwaiting inner probe and releases.
	root := cfg.ControlRoot
	if root == "" {
		root = cfg.WorkingDir
	}
	path := filepath.Join(root, filepath.FromSlash(ArtifactRetentionRelPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create artifact-retention lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open artifact-retention lock: %w", err)
	}
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("lock artifact-retention admission: %w", err)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func releaseRetentionAdmission(file *os.File) {
	if file == nil {
		return
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
}

func (l *SourceWriter) releaseRetentionAdmission() {
	l.retentionMu.Lock()
	file := l.retentionFile
	l.retentionFile = nil
	l.retentionMu.Unlock()
	releaseRetentionAdmission(file)
}

func openLockFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create source-writer lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open source-writer lock: %w", err)
	}
	return file, nil
}

func lockFile(ctx context.Context, file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return fmt.Errorf("lock source-writer file: %w", err)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func unlockFile(file *os.File) {
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func readMetadata(file *os.File) (Metadata, bool, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Metadata{}, false, fmt.Errorf("read source-writer lock: %w", err)
	}
	content, err := io.ReadAll(file)
	if err != nil {
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

func writeMetadata(file *os.File, metadata Metadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("write source-writer lock: %w", err)
	}
	return nil
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
