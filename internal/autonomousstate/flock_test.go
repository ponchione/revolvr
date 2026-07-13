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

func TestFlockContextPreservesExclusiveOSLockAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	owner := openFlockTestFile(t, path)
	probe := openFlockTestFile(t, path)
	if err := flockContext(context.Background(), owner); err != nil {
		t.Fatalf("acquire owner lock: %v", err)
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !isWouldBlock(err) {
		t.Fatalf("probe while owned error = %v, want would-block", err)
	}
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("release owner lock: %v", err)
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("probe after release: %v", err)
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatalf("release probe lock: %v", err)
	}
}

func TestFlockContextAcquiresAfterContendedLockRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	owner := openFlockTestFile(t, path)
	waiter := openFlockTestFile(t, path)
	probe := openFlockTestFile(t, path)
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- flockContext(ctx, waiter) }()
	select {
	case err := <-result:
		t.Fatalf("waiter returned before release: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("waiter after release: %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("waiter did not acquire promptly after release")
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !isWouldBlock(err) {
		t.Fatalf("probe after waiter acquisition error = %v, want would-block", err)
	}
	if err := syscall.Flock(int(waiter.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
}

func TestFlockContextCancellationIsPromptAndLeavesFileReusable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	owner := openFlockTestFile(t, path)
	waiter := openFlockTestFile(t, path)
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- flockContext(ctx, waiter) }()
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
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	retryCtx, retryCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer retryCancel()
	if err := flockContext(retryCtx, waiter); err != nil {
		t.Fatalf("reacquire with canceled waiter's file: %v", err)
	}
	if err := syscall.Flock(int(waiter.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
}

func TestFlockContextSustainedContentionConsumesBoundedCPU(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousProcs)
	path := filepath.Join(t.TempDir(), "state.lock")
	owner := openFlockTestFile(t, path)
	waiter := openFlockTestFile(t, path)
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(owner.Fd()), syscall.LOCK_UN)

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	cpuBefore := processCPUTime(t)
	started := time.Now()
	err := flockContext(ctx, waiter)
	wallElapsed := time.Since(started)
	cpuElapsed := processCPUTime(t) - cpuBefore
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended waiter error = %v", err)
	}
	if wallElapsed < 300*time.Millisecond || wallElapsed > time.Second {
		t.Fatalf("contended wall wait = %s, want bounded deadline wait", wallElapsed)
	}
	if cpuElapsed > 120*time.Millisecond {
		t.Fatalf("contended CPU time = %s during %s wall wait, want timer-backed wait", cpuElapsed, wallElapsed)
	}
}

func TestFlockContextManyWaitersEventuallyAcquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	owner := openFlockTestFile(t, path)
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	const waiterCount = 24
	waiters := make([]*os.File, waiterCount)
	for i := range waiters {
		waiters[i] = openFlockTestFile(t, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := make(chan struct{})
	type waiterResult struct {
		index int
		err   error
	}
	results := make(chan waiterResult, waiterCount)
	for i, waiter := range waiters {
		go func(index int, file *os.File) {
			<-start
			err := flockContext(ctx, file)
			if err == nil {
				time.Sleep(time.Millisecond)
				err = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
			}
			results <- waiterResult{index: index, err: err}
		}(i, waiter)
	}
	close(start)
	time.Sleep(30 * time.Millisecond)
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}

	seen := make(map[int]bool, waiterCount)
	for range waiterCount {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("waiter %d: %v", result.index, result.err)
			}
			if seen[result.index] {
				t.Fatalf("duplicate waiter result %d", result.index)
			}
			seen[result.index] = true
		case <-ctx.Done():
			t.Fatalf("only %d/%d waiters acquired before deadline: %v", len(seen), waiterCount, ctx.Err())
		}
	}
}

func TestFlockContextReturnsLockErrorsUnchanged(t *testing.T) {
	file := openFlockTestFile(t, filepath.Join(t.TempDir(), "state.lock"))
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	err := flockContext(context.Background(), file)
	if !errors.Is(err, syscall.EBADF) {
		t.Fatalf("closed-file lock error = %v, want EBADF", err)
	}
}

func BenchmarkFlockContext(b *testing.B) {
	path := filepath.Join(b.TempDir(), "state.lock")
	b.Run("uncontended", func(b *testing.B) {
		file := openFlockTestFile(b, path)
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := flockContext(ctx, file); err != nil {
				b.Fatal(err)
			}
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("contended_deadline", func(b *testing.B) {
		owner := openFlockTestFile(b, path)
		waiter := openFlockTestFile(b, path)
		if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			b.Fatal(err)
		}
		defer syscall.Flock(int(owner.Fd()), syscall.LOCK_UN)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
			err := flockContext(ctx, waiter)
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				b.Fatalf("contended error = %v", err)
			}
		}
	})
}

func openFlockTestFile(tb testing.TB, path string) *os.File {
	tb.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
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
