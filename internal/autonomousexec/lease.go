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
)

func Acquire(ctx context.Context, repositoryRoot string) (func(), error) {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, ".revolvr", "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "autonomous-execution.lock")
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("autonomous execution: symlinked lease path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
