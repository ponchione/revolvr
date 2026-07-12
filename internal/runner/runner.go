package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"revolvr/internal/outputcap"
)

const (
	defaultTimeout              = 30 * time.Minute
	defaultTerminateGracePeriod = 10 * time.Second
	maxLineEmitterPendingBytes  = 64 * 1024
)

type Command struct {
	Name                 string
	Args                 []string
	Stdin                io.Reader
	Dir                  string
	Env                  []string
	ReplaceEnv           bool
	Timeout              time.Duration
	TerminateGracePeriod time.Duration
	StdoutLimit          int
	StderrLimit          int
	OnStdoutLine         func(string)
	OnStderrLine         func(string)
	OnStart              func(pid int)
}

type Result struct {
	ExitCode             int
	Err                  error
	TimedOut             bool
	Stdout               string
	Stderr               string
	StdoutTruncatedBytes int64
	StderrTruncatedBytes int64
}

func Run(ctx context.Context, in Command) Result {
	if in.Timeout <= 0 {
		in.Timeout = defaultTimeout
	}
	if in.TerminateGracePeriod <= 0 {
		in.TerminateGracePeriod = defaultTerminateGracePeriod
	}

	runCtx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	stdoutBuf := outputcap.NewBuffer(in.StdoutLimit)
	stderrBuf := outputcap.NewBuffer(in.StderrLimit)
	stdout, flushStdout := composeOutputWriter(stdoutBuf, in.OnStdoutLine)
	stderr, flushStderr := composeOutputWriter(stderrBuf, in.OnStderrLine)
	defer flushStdout()
	defer flushStderr()

	cmd := exec.CommandContext(runCtx, in.Name, in.Args...)
	cmd.Stdin = in.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = in.Dir
	if in.ReplaceEnv {
		cmd.Env = append([]string(nil), in.Env...)
	} else if len(in.Env) > 0 {
		cmd.Env = append(os.Environ(), in.Env...)
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = in.TerminateGracePeriod

	if err := cmd.Start(); err != nil {
		return resultFromBuffers(stdoutBuf, stderrBuf, -1, fmt.Errorf("start command: %w", err), false)
	}
	if in.OnStart != nil && cmd.Process != nil {
		in.OnStart(cmd.Process.Pid)
	}

	err := cmd.Wait()
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if timedOut {
			return resultFromBuffers(stdoutBuf, stderrBuf, exitCode, context.DeadlineExceeded, true)
		}
		if runCtx.Err() != nil {
			return resultFromBuffers(stdoutBuf, stderrBuf, exitCode, runCtx.Err(), false)
		}
		if _, ok := err.(*exec.ExitError); ok {
			return resultFromBuffers(stdoutBuf, stderrBuf, exitCode, nil, false)
		}
		return resultFromBuffers(stdoutBuf, stderrBuf, exitCode, fmt.Errorf("run command: %w", err), false)
	}

	return resultFromBuffers(stdoutBuf, stderrBuf, 0, nil, false)
}

func resultFromBuffers(stdoutBuf, stderrBuf *outputcap.Buffer, exitCode int, err error, timedOut bool) Result {
	return Result{
		ExitCode:             exitCode,
		Err:                  err,
		TimedOut:             timedOut,
		Stdout:               stdoutBuf.String(),
		Stderr:               stderrBuf.String(),
		StdoutTruncatedBytes: stdoutBuf.TruncatedBytes(),
		StderrTruncatedBytes: stderrBuf.TruncatedBytes(),
	}
}

func composeOutputWriter(base io.Writer, onLine func(string)) (io.Writer, func()) {
	if onLine == nil {
		return base, func() {}
	}
	lineWriter := &lineEmitter{onLine: onLine}
	if base == nil {
		return lineWriter, lineWriter.Flush
	}
	return io.MultiWriter(base, lineWriter), lineWriter.Flush
}

type lineEmitter struct {
	pending []byte
	onLine  func(string)
}

func (w *lineEmitter) Write(p []byte) (int, error) {
	remaining := append(w.pending, p...)
	for {
		idx := -1
		for i, b := range remaining {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		if w.onLine != nil {
			w.onLine(string(remaining[:idx]))
		}
		remaining = remaining[idx+1:]
	}
	if len(remaining) > maxLineEmitterPendingBytes {
		if w.onLine != nil {
			w.onLine(string(remaining[:maxLineEmitterPendingBytes]) + " [line truncated]")
		}
		remaining = remaining[:0]
	}
	w.pending = append(w.pending[:0], remaining...)
	return len(p), nil
}

func (w *lineEmitter) Flush() {
	if len(w.pending) == 0 {
		return
	}
	if w.onLine != nil {
		w.onLine(string(w.pending))
	}
	w.pending = w.pending[:0]
}
