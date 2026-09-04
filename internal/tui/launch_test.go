package tui

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"revolvr/internal/app"
)

func TestRunStatusRendersPendingWhileBootstrapIsBlocked(t *testing.T) {
	bootstrapStarted := make(chan struct{})
	releaseBootstrap := make(chan struct{})
	var bootstrapCalls atomic.Int32
	input, inputWriter := io.Pipe()
	defer input.Close()
	defer inputWriter.Close()
	output := newLockedBuffer()
	done := make(chan error, 1)

	go func() {
		done <- RunStatus(context.Background(), RunOptions{
			Input:  input,
			Output: output,
			BootstrapStatus: func() (app.StatusResult, error) {
				bootstrapCalls.Add(1)
				close(bootstrapStarted)
				<-releaseBootstrap
				return app.StatusResult{Initialized: true, ProjectRoot: "/work/revolvr"}, nil
			},
		})
	}()

	waitForSignal(t, bootstrapStarted, "bootstrap start")
	waitForOutput(t, output, "Loading…")
	if got := output.String(); strings.Contains(got, "Ready") || strings.Contains(got, "At start:") {
		t.Fatalf("blocked bootstrap output contains ready or startup history: %q", got)
	}

	close(releaseBootstrap)
	waitForOutput(t, output, "Ready")
	if _, err := inputWriter.Write([]byte("/quit\r")); err != nil {
		t.Fatalf("write quit: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run status: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("program did not quit")
	}

	if got := bootstrapCalls.Load(); got != 1 {
		t.Fatalf("bootstrap calls = %d, want 1", got)
	}
	rendered := output.String()
	for _, history := range []string{"Project: /work/revolvr", "At start: initialized"} {
		if strings.Contains(rendered, history) {
			t.Fatalf("startup history %q was emitted in %q", history, rendered)
		}
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func newLockedBuffer() *lockedBuffer {
	return &lockedBuffer{}
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func waitForSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitForOutput(t *testing.T, output *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for output %q in %q", want, output.String())
}
