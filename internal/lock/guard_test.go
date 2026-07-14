package lock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSourceGuardCancelsWorkAndPreservesHeartbeatFailure(t *testing.T) {
	persistenceErr := errors.New("injected heartbeat persistence failure")
	lease := &testSourceLease{heartbeat: func(context.Context) error { return persistenceErr }}
	guard := MonitorSourceLease(context.Background(), lease, time.Millisecond)

	select {
	case <-guard.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("guard did not cancel active work")
	}
	failure := guard.Failure()
	if !errors.Is(failure, ErrOwnershipLost) || !errors.Is(failure, persistenceErr) {
		t.Fatalf("failure = %v, want ownership and persistence errors", failure)
	}
	if cause := context.Cause(guard.Context()); !errors.Is(cause, ErrOwnershipLost) || !errors.Is(cause, persistenceErr) {
		t.Fatalf("context cause = %v", cause)
	}
	if err := guard.Close(context.Background()); !errors.Is(err, persistenceErr) {
		t.Fatalf("Close error = %v, want persistence failure", err)
	}
}

func TestSourceGuardCheckFailsClosedAndJoinsReleaseFailure(t *testing.T) {
	ownerErr := ErrNotOwner
	releaseErr := errors.New("injected release persistence failure")
	lease := &testSourceLease{
		heartbeat:  func(context.Context) error { return ownerErr },
		releaseErr: releaseErr,
	}
	guard := MonitorSourceLease(context.Background(), lease, time.Hour)
	checkErr := guard.Check(context.Background())
	if !errors.Is(checkErr, ErrOwnershipLost) || !errors.Is(checkErr, ownerErr) {
		t.Fatalf("Check error = %v", checkErr)
	}
	closeErr := guard.Close(context.Background())
	for _, want := range []error{ErrOwnershipLost, ownerErr, releaseErr} {
		if !errors.Is(closeErr, want) {
			t.Fatalf("Close error = %v, missing %v", closeErr, want)
		}
	}
}

func TestSourceGuardPreservesSimultaneousCancellationAndHeartbeatFailure(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	persistenceErr := errors.New("heartbeat failed during cancellation")
	lease := &testSourceLease{heartbeat: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return errors.Join(ctx.Err(), persistenceErr)
	}}
	guard := MonitorSourceLease(parent, lease, time.Millisecond)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not start")
	}
	cancel()
	if err := guard.Close(context.Background()); !errors.Is(err, context.Canceled) || !errors.Is(err, persistenceErr) || !errors.Is(err, ErrOwnershipLost) {
		t.Fatalf("Close error = %v, want cancellation plus ownership persistence failure", err)
	}
}

func TestSourceGuardSettleWaitsForInFlightFailureWithoutReleasing(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	allowReturn := make(chan struct{})
	persistenceErr := errors.New("heartbeat failed while settling")
	lease := &testSourceLease{heartbeat: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		<-allowReturn
		return errors.Join(ctx.Err(), persistenceErr)
	}}
	guard := MonitorSourceLease(parent, lease, time.Millisecond)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not start")
	}
	cancel()

	settled := make(chan error, 1)
	go func() { settled <- guard.Settle() }()
	select {
	case err := <-settled:
		t.Fatalf("Settle returned before heartbeat completed: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	if lease.releaseCount() != 0 {
		t.Fatal("Settle released the source lease")
	}

	close(allowReturn)
	if err := <-settled; !errors.Is(err, ErrOwnershipLost) || !errors.Is(err, persistenceErr) {
		t.Fatalf("Settle error = %v, want ownership and persistence failures", err)
	}
	if lease.releaseCount() != 0 {
		t.Fatal("Settle released the source lease")
	}
	if err := guard.Close(context.Background()); !errors.Is(err, persistenceErr) {
		t.Fatalf("Close error = %v, want persistence failure", err)
	}
	if err := guard.Close(context.Background()); !errors.Is(err, persistenceErr) {
		t.Fatalf("second Close error = %v, want retained persistence failure", err)
	}
	if lease.releaseCount() != 1 {
		t.Fatalf("release count = %d, want 1", lease.releaseCount())
	}
}

func TestSourceGuardSettleIgnoresWrappedCancellationOnly(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	lease := &testSourceLease{heartbeat: func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return fmt.Errorf("heartbeat stopped: %w", ctx.Err())
	}}
	guard := MonitorSourceLease(parent, lease, time.Millisecond)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not start")
	}
	cancel()
	if err := guard.Settle(); err != nil {
		t.Fatalf("Settle error = %v, want ordinary cancellation", err)
	}
	if err := guard.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSourceGuardHealthyStopDoesNotLeakOrAddNoise(t *testing.T) {
	lease := &testSourceLease{}
	guard := MonitorSourceLease(context.Background(), lease, time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for lease.heartbeatCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := guard.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := guard.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if guard.Failure() != nil || lease.releaseCount() != 1 {
		t.Fatalf("failure=%v releases=%d", guard.Failure(), lease.releaseCount())
	}
}

type testSourceLease struct {
	mu         sync.Mutex
	heartbeat  func(context.Context) error
	releaseErr error
	heartbeats int
	releases   int
}

func (l *testSourceLease) Heartbeat(ctx context.Context) error {
	l.mu.Lock()
	l.heartbeats++
	fn := l.heartbeat
	l.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil
}

func (l *testSourceLease) Release(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releases++
	return l.releaseErr
}

func (l *testSourceLease) heartbeatCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.heartbeats
}

func (l *testSourceLease) releaseCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.releases
}
