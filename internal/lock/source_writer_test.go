package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAcquireSourceWriterWritesAndReleasesLock(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}

	handle, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "run-1",
		PID:        1234,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("acquire source writer: %v", err)
	}

	metadata, found, err := ReadSourceWriter(ctx, workDir)
	if err != nil {
		t.Fatalf("read source writer: %v", err)
	}
	if !found {
		t.Fatal("lock metadata not found")
	}
	if metadata.RunID != "run-1" || metadata.PID != 1234 {
		t.Fatalf("metadata owner = run %q pid %d", metadata.RunID, metadata.PID)
	}
	if !metadata.AcquiredAt.Equal(clock.value) || !metadata.HeartbeatAt.Equal(clock.value) {
		t.Fatalf("metadata timestamps = acquired %s heartbeat %s", metadata.AcquiredAt, metadata.HeartbeatAt)
	}
	if want := clock.value.Add(time.Minute); !metadata.ExpiresAt.Equal(want) {
		t.Fatalf("expires at = %s, want %s", metadata.ExpiresAt, want)
	}

	if err := handle.Release(ctx); err != nil {
		t.Fatalf("release source writer: %v", err)
	}
	if _, found, err := ReadSourceWriter(ctx, workDir); err != nil || found {
		t.Fatalf("read after release found=%v err=%v, want no lock", found, err)
	}
	if _, err := os.Stat(filepath.Join(workDir, SourceWriterRelPath)); err != nil {
		t.Fatalf("lock file missing after release: %v", err)
	}
}

func TestAcquireSourceWriterRefusesLiveLock(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}

	first, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "run-1",
		PID:        111,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.Release(ctx)

	_, err = AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "run-2",
		PID:        222,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("second acquire error = %v, want ErrHeld", err)
	}
	var held *HeldError
	if !errors.As(err, &held) || held.Metadata.RunID != "run-1" || held.Metadata.PID != 111 {
		t.Fatalf("held error = %#v, want run-1 pid 111", err)
	}
}

func TestAcquireSourceWriterReplacesStaleLock(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}

	stale, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "stale-run",
		PID:        111,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer stale.Release(ctx)

	clock.value = clock.value.Add(2 * time.Minute)
	fresh, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "fresh-run",
		PID:        222,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("fresh acquire replacing stale lock: %v", err)
	}
	defer fresh.Release(ctx)

	metadata, found, err := ReadSourceWriter(ctx, workDir)
	if err != nil {
		t.Fatalf("read source writer: %v", err)
	}
	if !found || metadata.RunID != "fresh-run" || metadata.PID != 222 {
		t.Fatalf("metadata after stale replacement = %+v found=%v", metadata, found)
	}
}

func TestHeartbeatRefreshesTimestampAndExpiry(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}

	handle, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "run-heartbeat",
		PID:        123,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("acquire source writer: %v", err)
	}
	defer handle.Release(ctx)

	clock.value = clock.value.Add(20 * time.Second)
	if err := handle.Heartbeat(ctx); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	metadata, found, err := ReadSourceWriter(ctx, workDir)
	if err != nil {
		t.Fatalf("read source writer: %v", err)
	}
	if !found {
		t.Fatal("lock metadata not found after heartbeat")
	}
	if !metadata.HeartbeatAt.Equal(clock.value) {
		t.Fatalf("heartbeat at = %s, want %s", metadata.HeartbeatAt, clock.value)
	}
	if want := clock.value.Add(time.Minute); !metadata.ExpiresAt.Equal(want) {
		t.Fatalf("expires at = %s, want %s", metadata.ExpiresAt, want)
	}
}

func TestHeartbeatCannotReviveExpiredLease(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	handle, err := AcquireSourceWriter(ctx, Config{WorkingDir: workDir, RunID: "expiring-run", PID: 123, Timeout: time.Minute, Clock: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	clock.value = clock.value.Add(time.Minute)
	if err := handle.Heartbeat(ctx); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("Heartbeat error = %v, want ErrLeaseExpired", err)
	}
	if err := handle.Release(ctx); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("Release error = %v, want ErrLeaseExpired", err)
	}
	fresh, err := AcquireSourceWriter(ctx, Config{WorkingDir: workDir, RunID: "replacement-run", PID: 456, Timeout: time.Minute, Clock: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release(ctx)
}

func TestHeartbeatReportsLockPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	handle, err := AcquireSourceWriter(ctx, Config{WorkingDir: workDir, RunID: "io-failure-run", PID: 123, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.releaseRetentionAdmission()
	path := handle.Path()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workDir, filepath.FromSlash(ArtifactRetentionRelPath))); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Dir(path), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := handle.Heartbeat(ctx); err == nil || !strings.Contains(err.Error(), "source-writer lock directory") {
		t.Fatalf("Heartbeat error = %v, want persistence failure", err)
	}
}

func TestHeartbeatRefusesReplacementOwnerToken(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	stale, err := AcquireSourceWriter(ctx, Config{WorkingDir: workDir, RunID: "stale-run", PID: 111, Timeout: time.Minute, Clock: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Release(ctx)
	clock.value = clock.value.Add(2 * time.Minute)
	fresh, err := AcquireSourceWriter(ctx, Config{WorkingDir: workDir, RunID: "fresh-run", PID: 222, Timeout: time.Minute, Clock: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release(ctx)
	if err := stale.Heartbeat(ctx); !errors.Is(err, ErrHeld) {
		t.Fatalf("stale Heartbeat error = %v, want replacement-owner evidence", err)
	}
}

func TestReleaseDoesNotClearAnotherOwner(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	clock := &fixedClock{value: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}

	stale, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "stale-run",
		PID:        111,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	clock.value = clock.value.Add(2 * time.Minute)
	fresh, err := AcquireSourceWriter(ctx, Config{
		WorkingDir: workDir,
		RunID:      "fresh-run",
		PID:        222,
		Timeout:    time.Minute,
		Clock:      clock.now,
	})
	if err != nil {
		t.Fatalf("fresh acquire replacing stale lock: %v", err)
	}
	defer fresh.Release(ctx)

	if err := stale.Release(ctx); !errors.Is(err, ErrHeld) {
		t.Fatalf("stale release error = %v, want ErrHeld", err)
	}
	metadata, found, err := ReadSourceWriter(ctx, workDir)
	if err != nil {
		t.Fatalf("read source writer: %v", err)
	}
	if !found || metadata.RunID != "fresh-run" {
		t.Fatalf("metadata after stale release = %+v found=%v", metadata, found)
	}
}

func TestWorkspaceSourceWriterUsesControlRootAndBindsExecutionIdentity(t *testing.T) {
	control := t.TempDir()
	execution := filepath.Join(control, ".revolvr", "autonomous", "worktrees", "workspace-one")
	if err := os.MkdirAll(execution, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	writer, err := AcquireSourceWriter(context.Background(), Config{ControlRoot: control, ExecutionRoot: execution, WorkspaceID: "workspace-one", RunID: "run-one", PID: 101, Timeout: time.Minute, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	want, err := WorkspaceSourceWriterPath(control, "workspace-one")
	if err != nil {
		t.Fatal(err)
	}
	if writer.Path() != want || strings.HasPrefix(writer.Path(), execution+string(filepath.Separator)) {
		t.Fatalf("workspace lock path = %q, want control-root %q", writer.Path(), want)
	}
	raw, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"workspace_id": "workspace-one"`) || !strings.Contains(string(raw), execution) {
		t.Fatalf("workspace lock metadata = %s", raw)
	}
	if err := writer.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentWorkspaceSourceWritersShareRetentionAdmission(t *testing.T) {
	control := t.TempDir()
	acquire := func(workspaceID string, pid int) *SourceWriter {
		execution := filepath.Join(control, ".revolvr", "autonomous", "worktrees", workspaceID)
		if err := os.MkdirAll(execution, 0o755); err != nil {
			t.Fatal(err)
		}
		writer, err := AcquireSourceWriter(context.Background(), Config{ControlRoot: control, ExecutionRoot: execution, WorkspaceID: workspaceID, RunID: "run-" + workspaceID, PID: pid, Timeout: time.Minute})
		if err != nil {
			t.Fatalf("acquire %s: %v", workspaceID, err)
		}
		return writer
	}
	first := acquire("workspace-one", 101)
	defer first.Release(context.Background())
	second := acquire("workspace-two", 202)
	defer second.Release(context.Background())
}

func TestReleaseDropsRetentionAdmissionEvenWhenMetadataReleaseFails(t *testing.T) {
	root := t.TempDir()
	writer, err := AcquireSourceWriter(context.Background(), Config{WorkingDir: root, RunID: "cancelled-release", PID: 123, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := writer.Release(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled release error = %v", err)
	}

	path := filepath.Join(root, filepath.FromSlash(ArtifactRetentionRelPath))
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		t.Fatalf("retention admission remained held after failed release: %v", err)
	}
	_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	_ = file.Close()
	if err := writer.Release(context.Background()); err != nil {
		t.Fatalf("clear metadata after cancelled release: %v", err)
	}
}

type fixedClock struct {
	value time.Time
}

func (c *fixedClock) now() time.Time {
	return c.value
}
