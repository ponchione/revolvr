package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"revolvr/internal/outputcap"
)

const (
	defaultTimeout              = 30 * time.Minute
	defaultTerminateGracePeriod = 10 * time.Second
	defaultKillSettlementPeriod = 5 * time.Second
	maxLineEmitterPendingBytes  = 64 * 1024
	lineTruncationMarker        = " [line truncated]"
)

var (
	ErrProcessTreeUnsupported   = errors.New("process-tree lifecycle boundary is unsupported on this platform")
	ErrProcessTreeUnsettled     = errors.New("command exited while descendants remained running")
	errProcessTreeIdentityReuse = errors.New("process-group identity was reused before settlement")
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
	// KillSettlementPeriod separately bounds proof that the process group is
	// gone after a force signal has been delivered.
	KillSettlementPeriod time.Duration
	StdoutLimit          int
	StderrLimit          int
	// StdoutWriter receives the authoritative stdout byte stream independently
	// of the capped capture buffer and bounded line preview.
	StdoutWriter io.Writer
	OnStdoutLine func(string)
	OnStderrLine func(string)
	OnStart      func(pid int)
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
	if in.KillSettlementPeriod <= 0 {
		in.KillSettlementPeriod = defaultKillSettlementPeriod
	}

	runCtx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	stdoutBuf := outputcap.NewBuffer(in.StdoutLimit)
	stderrBuf := outputcap.NewBuffer(in.StderrLimit)
	stdout, flushStdout := composeOutputWriter(stdoutBuf, in.OnStdoutLine, in.StdoutWriter)
	stderr, flushStderr := composeOutputWriter(stderrBuf, in.OnStderrLine)
	defer flushStdout()
	defer flushStderr()

	cmd := exec.Command(in.Name, in.Args...)
	cmd.Stdin = in.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = in.Dir
	if in.ReplaceEnv {
		cmd.Env = append([]string(nil), in.Env...)
	} else if len(in.Env) > 0 {
		cmd.Env = append(os.Environ(), in.Env...)
	}
	if err := prepareProcessTree(cmd); err != nil {
		return resultFromBuffers(stdoutBuf, stderrBuf, -1, fmt.Errorf("prepare command: %w", err), false)
	}
	cmd.WaitDelay = in.TerminateGracePeriod

	if err := runCtx.Err(); err != nil {
		return resultFromBuffers(stdoutBuf, stderrBuf, -1, err, errors.Is(err, context.DeadlineExceeded))
	}
	if err := cmd.Start(); err != nil {
		return resultFromBuffers(stdoutBuf, stderrBuf, -1, fmt.Errorf("start command: %w", err), false)
	}
	if in.OnStart != nil && cmd.Process != nil {
		in.OnStart(cmd.Process.Pid)
	}

	commandDone := make(chan struct{})
	terminationDone := watchProcessTree(runCtx, cmd.Process.Pid, in.TerminateGracePeriod, in.KillSettlementPeriod, commandDone)
	err := cmd.Wait()
	close(commandDone)
	termination := <-terminationDone
	if termination.cancelled {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return resultFromBuffers(
			stdoutBuf,
			stderrBuf,
			exitCode,
			errors.Join(termination.cause, termination.err),
			errors.Is(termination.cause, context.DeadlineExceeded),
		)
	}
	if termination.descendantsRemained {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return resultFromBuffers(
			stdoutBuf,
			stderrBuf,
			exitCode,
			errors.Join(ErrProcessTreeUnsettled, termination.err),
			false,
		)
	}
	if err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if _, ok := err.(*exec.ExitError); ok {
			return resultFromBuffers(stdoutBuf, stderrBuf, exitCode, nil, false)
		}
		return resultFromBuffers(stdoutBuf, stderrBuf, exitCode, fmt.Errorf("run command: %w", err), false)
	}

	return resultFromBuffers(stdoutBuf, stderrBuf, 0, nil, false)
}

type processTreeTermination struct {
	cancelled           bool
	descendantsRemained bool
	cause               error
	err                 error
}

func watchProcessTree(ctx context.Context, pid int, grace, killSettlement time.Duration, commandDone <-chan struct{}) <-chan processTreeTermination {
	done := make(chan processTreeTermination, 1)
	go func() {
		select {
		case <-ctx.Done():
			done <- processTreeTermination{cancelled: true, cause: ctx.Err(), err: terminateProcessTree(pid, grace, killSettlement, commandDone)}
		case <-commandDone:
			remained, err := settleExitedProcessTree(pid, grace, killSettlement)
			cause := ctx.Err()
			done <- processTreeTermination{
				cancelled:           cause != nil,
				descendantsRemained: remained,
				cause:               cause,
				err:                 err,
			}
		}
	}()
	return done
}

func settleExitedProcessTree(pid int, grace, killSettlement time.Duration) (bool, error) {
	lifecycle := newProcessTreeLifecycle(pid, func() bool { return true }, nativeProcessTreePlatform())
	running, err := lifecycle.running()
	if err != nil {
		return true, errors.Join(ErrProcessTreeUnsettled, fmt.Errorf("inspect process tree after command exit: %w", err))
	}
	if !running {
		return false, nil
	}
	return true, terminateProcessTreeWithSignal(grace, killSettlement, lifecycle)
}

type processTreePlatform struct {
	signal         func(pid int, force bool) error
	running        func(pid int) (bool, error)
	identityReused func(pid int) (bool, error)
}

func nativeProcessTreePlatform() processTreePlatform {
	return processTreePlatform{
		signal:         signalProcessTree,
		running:        processTreeRunning,
		identityReused: processTreeIdentityReused,
	}
}

type processTreeLifecycle struct {
	signal  func(force bool) error
	running func() (bool, error)
}

func newProcessTreeLifecycle(pid int, leaderReaped func() bool, platform processTreePlatform) processTreeLifecycle {
	verifyIdentity := func() error {
		if leaderReaped == nil || !leaderReaped() {
			return nil
		}
		reused, err := platform.identityReused(pid)
		if err != nil {
			return fmt.Errorf("verify exited process-tree identity: %w", err)
		}
		if reused {
			return errProcessTreeIdentityReuse
		}
		return nil
	}
	return processTreeLifecycle{
		signal: func(force bool) error {
			if err := verifyIdentity(); err != nil {
				return err
			}
			return platform.signal(pid, force)
		},
		running: func() (bool, error) {
			if err := verifyIdentity(); err != nil {
				return false, err
			}
			return platform.running(pid)
		},
	}
}

func terminateProcessTree(pid int, grace, killSettlement time.Duration, commandDone <-chan struct{}) error {
	leaderReaped := func() bool {
		select {
		case <-commandDone:
			return true
		default:
			return false
		}
	}
	return terminateProcessTreeWithSignal(grace, killSettlement, newProcessTreeLifecycle(pid, leaderReaped, nativeProcessTreePlatform()))
}

func terminateProcessTreeWithSignal(grace, killSettlement time.Duration, lifecycle processTreeLifecycle) error {
	var result error
	if err := lifecycle.signal(false); err != nil && !errors.Is(err, os.ErrProcessDone) {
		result = errors.Join(result, fmt.Errorf("gracefully terminate process tree: %w", err))
		if errors.Is(err, errProcessTreeIdentityReuse) {
			return errors.Join(ErrProcessTreeUnsettled, result)
		}
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()
	ticker := time.NewTicker(processTreePollInterval(grace))
	defer ticker.Stop()
	var inspectionErr error

	for {
		running, err := lifecycle.running()
		if err != nil {
			if inspectionErr == nil {
				inspectionErr = fmt.Errorf("inspect process tree: %w", err)
			}
			if errors.Is(err, errProcessTreeIdentityReuse) {
				return errors.Join(ErrProcessTreeUnsettled, result, inspectionErr)
			}
		} else if !running {
			return errors.Join(result, inspectionErr)
		}

		select {
		case <-timer.C:
			result = errors.Join(result, inspectionErr)
			if err := lifecycle.signal(true); err != nil && !errors.Is(err, os.ErrProcessDone) {
				result = errors.Join(result, fmt.Errorf("force terminate process tree: %w", err))
				if errors.Is(err, errProcessTreeIdentityReuse) {
					return errors.Join(ErrProcessTreeUnsettled, result)
				}
			}
			return settleKilledProcessTree(killSettlement, lifecycle, result)
		case <-ticker.C:
		}
	}
}

func settleKilledProcessTree(period time.Duration, lifecycle processTreeLifecycle, result error) error {
	timer := time.NewTimer(period)
	defer timer.Stop()
	ticker := time.NewTicker(processTreePollInterval(period))
	defer ticker.Stop()
	var inspectionErr error

	for {
		running, err := lifecycle.running()
		if err != nil {
			if inspectionErr == nil {
				inspectionErr = fmt.Errorf("inspect force-killed process tree: %w", err)
			}
			if errors.Is(err, errProcessTreeIdentityReuse) {
				return errors.Join(ErrProcessTreeUnsettled, result, inspectionErr)
			}
		} else if !running {
			return errors.Join(result, inspectionErr)
		}

		select {
		case <-timer.C:
			return errors.Join(ErrProcessTreeUnsettled, result, inspectionErr, fmt.Errorf("force-killed process tree did not settle within %s", period))
		case <-ticker.C:
		}
	}
}

func processTreePollInterval(grace time.Duration) time.Duration {
	interval := grace / 20
	if interval < time.Millisecond {
		return time.Millisecond
	}
	if interval > 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return interval
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

func composeOutputWriter(base io.Writer, onLine func(string), additional ...io.Writer) (io.Writer, func()) {
	writers := make([]io.Writer, 0, 2+len(additional))
	if base != nil {
		writers = append(writers, base)
	}
	flush := func() {}
	if onLine != nil {
		lineWriter := &lineEmitter{onLine: onLine}
		writers = append(writers, lineWriter)
		flush = lineWriter.Flush
	}
	for _, writer := range additional {
		if writer != nil {
			writers = append(writers, writer)
		}
	}
	if len(writers) == 1 {
		return writers[0], flush
	}
	return io.MultiWriter(writers...), flush
}

type lineEmitter struct {
	pending    []byte
	discarding bool
	onLine     func(string)
}

func (w *lineEmitter) Write(p []byte) (int, error) {
	remaining := p
	for len(remaining) > 0 {
		if w.discarding {
			newline := bytes.IndexByte(remaining, '\n')
			if newline < 0 {
				return len(p), nil
			}
			w.discarding = false
			remaining = remaining[newline+1:]
			continue
		}

		newline := bytes.IndexByte(remaining, '\n')
		segment := remaining
		if newline >= 0 {
			segment = remaining[:newline]
		}
		available := maxLineEmitterPendingBytes - len(w.pending)
		if len(segment) > available {
			w.pending = append(w.pending, segment[:available]...)
			w.emitTruncated()
			if newline < 0 {
				return len(p), nil
			}
			w.discarding = false
			remaining = remaining[newline+1:]
			continue
		}

		w.pending = append(w.pending, segment...)
		if newline < 0 {
			return len(p), nil
		}
		w.emitPending()
		remaining = remaining[newline+1:]
	}
	return len(p), nil
}

func (w *lineEmitter) Flush() {
	if len(w.pending) == 0 || w.discarding {
		return
	}
	w.emitPending()
}

func (w *lineEmitter) emitPending() {
	if w.onLine != nil {
		w.onLine(string(w.pending))
	}
	w.pending = w.pending[:0]
}

func (w *lineEmitter) emitTruncated() {
	prefixBytes := maxLineEmitterPendingBytes - len(lineTruncationMarker)
	prefixBytes = completeUTF8Prefix(w.pending, prefixBytes)
	if w.onLine != nil {
		w.onLine(string(w.pending[:prefixBytes]) + lineTruncationMarker)
	}
	w.pending = w.pending[:0]
	w.discarding = true
}

func completeUTF8Prefix(value []byte, limit int) int {
	if limit >= len(value) {
		return len(value)
	}
	for limit > 0 && value[limit]&0xc0 == 0x80 {
		limit--
	}
	return limit
}
