package autonomousstate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

const stateLockRel = ".revolvr/autonomous/tasks/task-1/state.lock"

func TestAcquireStateLockPreservesExclusiveOSLockAuthority(t *testing.T) {
	store := &Store{root: t.TempDir()}
	owner, err := store.acquireLock(context.Background(), stateLockRel)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	probe := openStateLockProbe(t, filepath.Join(store.root, filepath.FromSlash(stateLockRel)))
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !isWouldBlock(err) {
		t.Fatalf("probe while owned error = %v, want would-block", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("probe after release: %v", err)
	}
	_ = syscall.Flock(int(probe.Fd()), syscall.LOCK_UN)
}

func TestAcquireStateLockAcquiresAfterContentionRelease(t *testing.T) {
	store := &Store{root: t.TempDir()}
	owner, err := store.acquireLock(context.Background(), stateLockRel)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan stateLockResult, 1)
	go func() {
		lease, err := store.acquireLock(ctx, stateLockRel)
		result <- stateLockResult{lease: lease, err: err}
	}()
	select {
	case got := <-result:
		if got.lease != nil {
			_ = got.lease.Close()
		}
		t.Fatalf("waiter returned before release: %v", got.err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatal(got.err)
		}
		_ = got.lease.Close()
	case <-time.After(250 * time.Millisecond):
		t.Fatal("waiter did not acquire promptly after release")
	}
}

func TestAcquireStateLockCancellationIsPromptAndReusable(t *testing.T) {
	store := &Store{root: t.TempDir()}
	owner, err := store.acquireLock(context.Background(), stateLockRel)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		lease, err := store.acquireLock(ctx, stateLockRel)
		if lease != nil {
			_ = lease.Close()
		}
		result <- err
	}()
	time.Sleep(30 * time.Millisecond)
	started := time.Now()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error = %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("canceled waiter did not return promptly")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled waiter return took %s", elapsed)
	}
	_ = owner.Close()
	retry, err := store.acquireLock(context.Background(), stateLockRel)
	if err != nil {
		t.Fatalf("reacquire after cancellation: %v", err)
	}
	_ = retry.Close()
}

func TestAcquireStateLockRejectsPreCanceledContext(t *testing.T) {
	store := &Store{root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lease, err := store.acquireLock(ctx, stateLockRel)
	if lease != nil {
		_ = lease.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled acquire error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(store.root, ".revolvr")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pre-canceled acquire mutated runtime paths: %v", statErr)
	}
}

func TestAcquireStateLockSustainedContentionConsumesBoundedCPU(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)
	store := &Store{root: t.TempDir()}
	owner, err := store.acquireLock(context.Background(), stateLockRel)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	cpuBefore := processCPUTime(t)
	started := time.Now()
	_, err = store.acquireLock(ctx, stateLockRel)
	wallElapsed := time.Since(started)
	cpuElapsed := processCPUTime(t) - cpuBefore
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended waiter error = %v", err)
	}
	if wallElapsed < 300*time.Millisecond || wallElapsed > time.Second {
		t.Fatalf("contended wall wait = %s, want bounded deadline wait", wallElapsed)
	}
	if cpuElapsed > 120*time.Millisecond {
		t.Fatalf("contended CPU time = %s during %s wall wait", cpuElapsed, wallElapsed)
	}
}

func TestAcquireStateLockManyWaitersEventuallyAcquire(t *testing.T) {
	store := &Store{root: t.TempDir()}
	owner, err := store.acquireLock(context.Background(), stateLockRel)
	if err != nil {
		t.Fatal(err)
	}
	const waiterCount = 24
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, waiterCount)
	for range waiterCount {
		go func() {
			<-start
			lease, err := store.acquireLock(ctx, stateLockRel)
			if err == nil {
				time.Sleep(time.Millisecond)
				err = lease.Close()
			}
			results <- err
		}()
	}
	close(start)
	time.Sleep(30 * time.Millisecond)
	_ = owner.Close()
	for i := 0; i < waiterCount; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("waiter %d: %v", i, err)
			}
		case <-ctx.Done():
			t.Fatalf("only %d/%d waiters acquired: %v", i, waiterCount, ctx.Err())
		}
	}
}

func BenchmarkAcquireStateLock(b *testing.B) {
	store := &Store{root: b.TempDir()}
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lease, err := store.acquireLock(ctx, stateLockRel)
		if err != nil {
			b.Fatal(err)
		}
		if err := lease.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

type stateLockResult struct {
	lease interface{ Close() error }
	err   error
}

func openStateLockProbe(tb testing.TB, path string) *os.File {
	tb.Helper()
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = file.Close() })
	return file
}

func isWouldBlock(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func processCPUTime(tb testing.TB) time.Duration {
	tb.Helper()
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		tb.Fatal(err)
	}
	return timevalDuration(usage.Utime) + timevalDuration(usage.Stime)
}

func timevalDuration(value syscall.Timeval) time.Duration {
	return time.Duration(value.Sec)*time.Second + time.Duration(value.Usec)*time.Microsecond
}
