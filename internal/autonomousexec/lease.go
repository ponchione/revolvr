// Package autonomousexec owns the outer one-active autonomous execution
// boundary shared by direct AW-22 runs and AW-24 queue sweeps. It is never
// acquired from task/state/Git/source-writer owners.
package autonomousexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"revolvr/internal/runtimepath"
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
	root, err := runtimepath.CanonicalRoot(repositoryRoot)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, ".revolvr", "locks")
	if err := runtimepath.EnsureDir(root, dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "autonomous-execution.lock")
	if err := runtimepath.CheckFile(root, path, true); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := runtimepath.CheckOpenedFile(root, path, f); err != nil {
		_ = f.Close()
		return nil, err
	}
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			if err := runtimepath.CheckOpenedFile(root, path, f); err != nil {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
				return nil, err
			}
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if !wait {
			_ = f.Close()
			return nil, ErrActive
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
