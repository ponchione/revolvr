package lock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrOwnershipLost = errors.New("source-writer ownership lost")

const heartbeatAttemptTimeout = 5 * time.Second

// SourceLease is the ownership contract monitored by SourceGuard.
type SourceLease interface {
	Heartbeat(context.Context) error
	Release(context.Context) error
}

// SourceGuard propagates asynchronous lease failures into active source work
// and provides synchronous ownership checks at mutation/publication boundaries.
type SourceGuard struct {
	lease SourceLease

	operationCtx context.Context
	cancelWork   context.CancelCauseFunc
	stopMonitor  context.CancelFunc
	done         chan struct{}
	settleOnce   sync.Once
	releaseOnce  sync.Once
	heartbeatMu  sync.Mutex

	mu         sync.Mutex
	failure    error
	releaseErr error
}

func MonitorSourceLease(parent context.Context, lease SourceLease, interval time.Duration) *SourceGuard {
	if interval <= 0 {
		interval = time.Second
	}
	operationCtx, cancelWork := context.WithCancelCause(parent)
	monitorCtx, stopMonitor := context.WithCancel(operationCtx)
	guard := &SourceGuard{
		lease: lease, operationCtx: operationCtx, cancelWork: cancelWork,
		stopMonitor: stopMonitor, done: make(chan struct{}),
	}
	go guard.monitor(monitorCtx, interval)
	return guard
}

func (g *SourceGuard) Context() context.Context {
	if g == nil {
		return context.Background()
	}
	return g.operationCtx
}

// Check proves and refreshes ownership synchronously. A failed check also
// cancels the guarded operation so concurrent work observes the same failure.
func (g *SourceGuard) Check(ctx context.Context) error {
	if g == nil || g.lease == nil {
		return errors.New("source-writer guard has no lease")
	}
	if failure := g.Failure(); failure != nil {
		ctxErr := contextError(ctx)
		if ctxErr == nil || errors.Is(ctxErr, ErrOwnershipLost) {
			return failure
		}
		return errors.Join(ctxErr, failure)
	}
	g.heartbeatMu.Lock()
	if failure := g.Failure(); failure != nil {
		g.heartbeatMu.Unlock()
		return failure
	}
	err := g.lease.Heartbeat(ctx)
	g.heartbeatMu.Unlock()
	if err != nil {
		operationErr := contextError(g.operationCtx)
		if operationErr != nil && !errors.Is(operationErr, ErrOwnershipLost) && err == contextError(ctx) {
			return operationErr
		}
		g.fail(err)
		return errors.Join(contextError(ctx), g.Failure())
	}
	return g.Failure()
}

func (g *SourceGuard) Failure() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.failure
}

// Settle stops and joins the heartbeat monitor without releasing the lease.
// Callers use it before terminal persistence so an in-flight heartbeat has
// published any independent ownership failure before outcome classification.
func (g *SourceGuard) Settle() error {
	if g == nil {
		return nil
	}
	g.settleOnce.Do(func() {
		g.stopMonitor()
		<-g.done
	})
	return g.Failure()
}

// Close joins any heartbeat/check failure with release failure. The release
// context should be independent from the guarded operation's cancellation.
// Monitoring is settled and the lease is released exactly once.
func (g *SourceGuard) Close(ctx context.Context) error {
	if g == nil {
		return nil
	}
	settleErr := g.Settle()
	g.releaseOnce.Do(func() {
		if g.lease == nil {
			return
		}
		if err := g.lease.Release(ctx); err != nil {
			g.mu.Lock()
			g.releaseErr = fmt.Errorf("release source-writer lock: %w", err)
			g.mu.Unlock()
		}
	})
	g.mu.Lock()
	releaseErr := g.releaseErr
	g.mu.Unlock()
	return errors.Join(settleErr, releaseErr)
}

func (g *SourceGuard) monitor(ctx context.Context, interval time.Duration) {
	defer close(g.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeatCtx, cancel := context.WithTimeout(ctx, heartbeatAttemptTimeout)
			g.heartbeatMu.Lock()
			err := g.lease.Heartbeat(heartbeatCtx)
			g.heartbeatMu.Unlock()
			cancel()
			if err == nil {
				continue
			}
			if ctxErr := contextError(ctx); ctxErr != nil && causedOnlyBy(err, ctxErr) {
				return
			}
			g.fail(err)
			return
		}
	}
}

func (g *SourceGuard) fail(err error) {
	if err == nil {
		return
	}
	g.mu.Lock()
	if g.failure == nil {
		g.failure = fmt.Errorf("%w: %w", ErrOwnershipLost, err)
	}
	failure := g.failure
	g.mu.Unlock()
	g.cancelWork(failure)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}

func causedOnlyBy(err, target error) bool {
	if err == nil || target == nil {
		return false
	}
	if err == target {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !causedOnlyBy(cause, target) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return causedOnlyBy(wrapped.Unwrap(), target)
	}
	return false
}
