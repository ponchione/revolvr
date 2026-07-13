package autonomousexec

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLeaseExcludesDirectAndQueueDriversAndCancelsPromptly(t *testing.T) {
	root := t.TempDir()
	release, err := Acquire(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryAcquire(root); !errors.Is(err, ErrActive) {
		t.Fatalf("nonblocking contention err=%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, root); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended acquire err=%v", err)
	}
	release()
	second, err := Acquire(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	second()
	probe, err := TryAcquire(root)
	if err != nil {
		t.Fatal(err)
	}
	probe()
}
