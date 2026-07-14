// Package autonomousexec owns the outer one-active autonomous execution
// boundary shared by direct AW-22 runs and AW-24 queue sweeps. It is never
// acquired from task/state/Git/source-writer owners.
package autonomousexec

import (
	"context"
	"errors"

	"revolvr/internal/lock"
)

var ErrActive = errors.New("autonomous execution: another coordinator is active")

func Acquire(ctx context.Context, repositoryRoot string) (func(), error) {
	return acquire(ctx, repositoryRoot, true)
}

// TryAcquire takes the outer coordinator lease without waiting. Administrative
// operations use it to fail closed while a direct run or queue is active.
func TryAcquire(repositoryRoot string) (func(), error) {
	return acquire(context.Background(), repositoryRoot, false)
}

func acquire(ctx context.Context, repositoryRoot string, wait bool) (func(), error) {
	lease, err := lock.AcquireFlock(ctx, repositoryRoot, lock.FlockConfig{
		RelativePath: ".revolvr/locks/autonomous-execution.lock",
		Mode:         lock.FlockExclusive,
		Wait:         wait,
		Create:       true,
	})
	if errors.Is(err, lock.ErrFlockContended) {
		return nil, ErrActive
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = lease.Close() }, nil
}
