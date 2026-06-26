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
	"syscall"
	"time"
)

const SourceWriterRelPath = ".revolvr/locks/source-writer.lock"

const defaultTimeout = 5 * time.Minute

var (
	ErrHeld     = errors.New("source-writer lock is held")
	ErrNotOwner = errors.New("source-writer lock is held by another owner")
)

type Config struct {
	WorkingDir string
	RunID      string
	PID        int
	Timeout    time.Duration
	Clock      func() time.Time
}

type Metadata struct {
	RunID       string    `json:"run_id"`
	PID         int       `json:"pid"`
	AcquiredAt  time.Time `json:"acquired_at"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type SourceWriter struct {
	path    string
	runID   string
	pid     int
	timeout time.Duration
	clock   func() time.Time
}

type HeldError struct {
	Metadata Metadata
}

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
		RunID:       cfg.RunID,
		PID:         cfg.PID,
		AcquiredAt:  now,
		HeartbeatAt: now,
		ExpiresAt:   now.Add(cfg.Timeout),
	}
	if err := writeMetadata(file, metadata); err != nil {
		return nil, err
	}

	return &SourceWriter{
		path:    path,
		runID:   cfg.RunID,
		pid:     cfg.PID,
		timeout: cfg.Timeout,
		clock:   cfg.Clock,
	}, nil
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
	current.HeartbeatAt = now
	current.ExpiresAt = now.Add(l.timeout)
	return writeMetadata(file, current)
}

func (l *SourceWriter) Release(ctx context.Context) error {
	if l == nil {
		return nil
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
		return nil
	}
	if !l.owns(current) {
		return &HeldError{Metadata: current}
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

func normalizeConfig(cfg Config) (Config, string, error) {
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
	path, err := SourceWriterPath(cfg.WorkingDir)
	if err != nil {
		return Config{}, "", err
	}
	return cfg, path, nil
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
	return strings.TrimSpace(metadata.RunID) == l.runID && metadata.PID == l.pid
}
