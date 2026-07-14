package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRunCapturesSuccessfulCommandOutput(t *testing.T) {
	result := Run(context.Background(), helperCommand("success"))

	if result.Err != nil {
		t.Fatalf("run error: %v", result.Err)
	}
	if result.TimedOut {
		t.Fatal("timed out, want false")
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if got, want := result.Stdout, "stdout one\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := result.Stderr, "stderr one\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunCanReplaceAmbientEnvironment(t *testing.T) {
	command := helperCommand("environment")
	command.ReplaceEnv = true
	command.Env = append(command.Env, "ONLY_ALLOWED=present")
	result := Run(context.Background(), command)
	if result.Err != nil || result.ExitCode != 0 {
		t.Fatalf("result = %+v", result)
	}
	if got, want := strings.TrimSpace(result.Stdout), "allowed=present ambient="; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunReportsNonZeroExitCode(t *testing.T) {
	result := Run(context.Background(), helperCommand("nonzero"))

	if result.Err != nil {
		t.Fatalf("run error = %v, want nil for process exit", result.Err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
	if got, want := result.Stderr, "failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunTerminatesCommandOnTimeout(t *testing.T) {
	command := helperCommand("timeout")
	command.Timeout = 50 * time.Millisecond
	command.TerminateGracePeriod = 50 * time.Millisecond

	start := time.Now()
	result := Run(context.Background(), command)
	elapsed := time.Since(start)

	if !result.TimedOut {
		t.Fatal("timed out = false, want true")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", result.Err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took %s, want under 2s", elapsed)
	}
	if result.ExitCode == 0 {
		t.Fatal("exit code = 0, want non-zero for timed out process")
	}
}

func TestRunEmitsStdoutAndStderrLines(t *testing.T) {
	var stdoutLines []string
	var stderrLines []string
	command := helperCommand("lines")
	command.OnStdoutLine = func(line string) {
		stdoutLines = append(stdoutLines, line)
	}
	command.OnStderrLine = func(line string) {
		stderrLines = append(stderrLines, line)
	}

	result := Run(context.Background(), command)

	if result.Err != nil {
		t.Fatalf("run error: %v", result.Err)
	}
	if !reflect.DeepEqual(stdoutLines, []string{"one", "two", "partial"}) {
		t.Fatalf("stdout lines = %#v", stdoutLines)
	}
	if !reflect.DeepEqual(stderrLines, []string{"err"}) {
		t.Fatalf("stderr lines = %#v", stderrLines)
	}
}

func TestRunCapsCapturedOutput(t *testing.T) {
	command := helperCommand("truncate")
	command.StdoutLimit = 4
	command.StderrLimit = 3

	result := Run(context.Background(), command)

	if result.Err != nil {
		t.Fatalf("run error: %v", result.Err)
	}
	if got, want := result.Stdout, "xxxx"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := result.Stderr, "yyy"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got, want := result.StdoutTruncatedBytes, int64(6); got != want {
		t.Fatalf("stdout truncated bytes = %d, want %d", got, want)
	}
	if got, want := result.StderrTruncatedBytes, int64(6); got != want {
		t.Fatalf("stderr truncated bytes = %d, want %d", got, want)
	}
}

func TestRunKeepsAuthoritativeStdoutIndependentOfBoundedPreview(t *testing.T) {
	var authoritative bytes.Buffer
	var previews []string
	command := helperCommand("large-line")
	command.StdoutLimit = 16
	command.StdoutWriter = &authoritative
	command.OnStdoutLine = func(line string) {
		previews = append(previews, line)
	}

	result := Run(context.Background(), command)

	if result.Err != nil {
		t.Fatalf("run error: %v", result.Err)
	}
	want := strings.Repeat("x", maxLineEmitterPendingBytes+100) + "\nnext\n"
	if authoritative.String() != want {
		t.Fatalf("authoritative stdout size = %d, want %d", authoritative.Len(), len(want))
	}
	if len(result.Stdout) != 16 || result.StdoutTruncatedBytes != int64(len(want)-16) {
		t.Fatalf("capped stdout = %d bytes, truncated = %d", len(result.Stdout), result.StdoutTruncatedBytes)
	}
	if len(previews) != 2 {
		t.Fatalf("previews = %#v, want one truncated line and next line", previews)
	}
	if len(previews[0]) != maxLineEmitterPendingBytes || !strings.HasSuffix(previews[0], lineTruncationMarker) {
		t.Fatalf("first preview size/suffix = %d/%q", len(previews[0]), previews[0][len(previews[0])-len(lineTruncationMarker):])
	}
	if previews[1] != "next" {
		t.Fatalf("second preview = %q, want next", previews[1])
	}
}

func TestRunPreservesAuthoritativeStdoutWriterErrorIdentity(t *testing.T) {
	wantErr := errors.New("authoritative stdout rejected")
	command := helperCommand("success")
	command.StdoutWriter = errorWriter{err: wantErr}

	result := Run(context.Background(), command)

	if !errors.Is(result.Err, wantErr) {
		t.Fatalf("run error = %v, want authoritative writer error", result.Err)
	}
}

func TestTerminateProcessTreeWaitsForForceKillSettlement(t *testing.T) {
	forceSent := make(chan struct{})
	release := make(chan struct{})
	lifecycle := processTreeLifecycle{
		signal: func(force bool) error {
			if force {
				close(forceSent)
			}
			return nil
		},
		running: func() (bool, error) {
			select {
			case <-forceSent:
				select {
				case <-release:
					return false, nil
				default:
					return true, nil
				}
			default:
				return true, nil
			}
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- terminateProcessTreeWithSignal(5*time.Millisecond, 250*time.Millisecond, lifecycle)
	}()

	select {
	case <-forceSent:
	case <-time.After(time.Second):
		t.Fatal("force signal was not sent")
	}
	select {
	case err := <-done:
		t.Fatalf("termination returned before force-killed tree settled: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("termination error after settlement: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("termination did not return after settlement")
	}
}

func TestTerminateProcessTreeReportsBoundedUnsettledAndPreservesErrors(t *testing.T) {
	gracefulErr := errors.New("graceful signal failed")
	forceErr := errors.New("force signal failed")
	inspectionErr := errors.New("inspection failed")
	forceSent := false
	lifecycle := processTreeLifecycle{
		signal: func(force bool) error {
			if force {
				forceSent = true
				return forceErr
			}
			return gracefulErr
		},
		running: func() (bool, error) { return false, inspectionErr },
	}
	started := time.Now()
	err := terminateProcessTreeWithSignal(5*time.Millisecond, 20*time.Millisecond, lifecycle)
	if !forceSent || !errors.Is(err, ErrProcessTreeUnsettled) || !errors.Is(err, gracefulErr) || !errors.Is(err, forceErr) || !errors.Is(err, inspectionErr) {
		t.Fatalf("termination error = %v, force sent = %t", err, forceSent)
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond {
		t.Fatalf("termination returned after %s, before kill-settlement deadline", elapsed)
	}
}

func TestProcessTreeLifecycleChecksReapedIdentityBeforeSignalAndPoll(t *testing.T) {
	reaped := false
	reused := false
	identityCalls, signalCalls, runningCalls := 0, 0, 0
	lifecycle := newProcessTreeLifecycle(42, func() bool { return reaped }, processTreePlatform{
		signal: func(pid int, force bool) error {
			if pid != 42 || force {
				t.Fatalf("signal = (%d, %t)", pid, force)
			}
			signalCalls++
			return nil
		},
		running: func(pid int) (bool, error) {
			if pid != 42 {
				t.Fatalf("running pid = %d", pid)
			}
			runningCalls++
			return true, nil
		},
		identityReused: func(pid int) (bool, error) {
			if pid != 42 {
				t.Fatalf("identity pid = %d", pid)
			}
			identityCalls++
			return reused, nil
		},
	})
	if err := lifecycle.signal(false); err != nil {
		t.Fatal(err)
	}
	if running, err := lifecycle.running(); err != nil || !running {
		t.Fatalf("pre-reap running = %t, %v", running, err)
	}
	if identityCalls != 0 || signalCalls != 1 || runningCalls != 1 {
		t.Fatalf("pre-reap calls = identity %d, signal %d, running %d", identityCalls, signalCalls, runningCalls)
	}

	reaped, reused = true, true
	if _, err := lifecycle.running(); !errors.Is(err, errProcessTreeIdentityReuse) {
		t.Fatalf("reused poll error = %v", err)
	}
	if err := lifecycle.signal(false); !errors.Is(err, errProcessTreeIdentityReuse) {
		t.Fatalf("reused signal error = %v", err)
	}
	if identityCalls != 2 || signalCalls != 1 || runningCalls != 1 {
		t.Fatalf("post-reap calls = identity %d, signal %d, running %d", identityCalls, signalCalls, runningCalls)
	}
}

func TestTerminationRefusesReusedIdentityOnNaturalExitAndCancellationRace(t *testing.T) {
	for _, tc := range []struct {
		name          string
		initialReaped bool
		reapOnSignal  bool
		wantSignals   int
	}{
		{name: "natural exit", initialReaped: true, wantSignals: 0},
		{name: "cancellation race", reapOnSignal: true, wantSignals: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reaped := tc.initialReaped
			signalCalls, runningCalls := 0, 0
			platform := processTreePlatform{
				signal: func(int, bool) error {
					signalCalls++
					if tc.reapOnSignal {
						reaped = true
					}
					return nil
				},
				running: func(int) (bool, error) {
					runningCalls++
					return true, nil
				},
				identityReused: func(int) (bool, error) { return true, nil },
			}
			lifecycle := newProcessTreeLifecycle(42, func() bool { return reaped }, platform)
			err := terminateProcessTreeWithSignal(100*time.Millisecond, 100*time.Millisecond, lifecycle)
			if !errors.Is(err, ErrProcessTreeUnsettled) || !errors.Is(err, errProcessTreeIdentityReuse) {
				t.Fatalf("termination error = %v", err)
			}
			if signalCalls != tc.wantSignals || runningCalls != 0 {
				t.Fatalf("calls = signal %d, running %d; want signal %d, running 0", signalCalls, runningCalls, tc.wantSignals)
			}
		})
	}
}

func TestLineEmitterTruncatesOnceAndResynchronizesAcrossChunks(t *testing.T) {
	var lines []string
	writer := &lineEmitter{onLine: func(line string) { lines = append(lines, line) }}
	if _, err := writer.Write([]byte(strings.Repeat("a", maxLineEmitterPendingBytes))); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("tail")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(" discarded\nnext\n")); err != nil {
		t.Fatal(err)
	}
	writer.Flush()

	if len(lines) != 2 {
		t.Fatalf("lines = %#v, want truncated line and next line", lines)
	}
	if len(lines[0]) != maxLineEmitterPendingBytes || !strings.HasSuffix(lines[0], lineTruncationMarker) {
		t.Fatalf("truncated preview size/suffix = %d/%q", len(lines[0]), lines[0][len(lines[0])-len(lineTruncationMarker):])
	}
	if lines[1] != "next" {
		t.Fatalf("resynchronized line = %q, want next", lines[1])
	}
}

func TestLineEmitterDoesNotSplitUTF8InTruncatedPreview(t *testing.T) {
	var preview string
	writer := &lineEmitter{onLine: func(line string) { preview = line }}
	prefixBytes := maxLineEmitterPendingBytes - len(lineTruncationMarker)
	line := strings.Repeat("a", prefixBytes-1) + "é" + strings.Repeat("b", len(lineTruncationMarker)+10) + "\n"
	if _, err := writer.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(preview) || !strings.HasSuffix(preview, lineTruncationMarker) {
		t.Fatalf("preview is not valid visibly truncated UTF-8: %q", preview[len(preview)-len(lineTruncationMarker):])
	}
	if len(preview) > maxLineEmitterPendingBytes {
		t.Fatalf("preview size = %d, want at most %d", len(preview), maxLineEmitterPendingBytes)
	}
}

func helperCommand(mode string) Command {
	return Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestHelperProcess", "--"},
		Env: []string{
			"RUNNER_HELPER_PROCESS=1",
			"RUNNER_HELPER_MODE=" + mode,
		},
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("RUNNER_HELPER_PROCESS") != "1" {
		return
	}

	switch os.Getenv("RUNNER_HELPER_MODE") {
	case "success":
		fmt.Fprintln(os.Stdout, "stdout one")
		fmt.Fprintln(os.Stderr, "stderr one")
		os.Exit(0)
	case "nonzero":
		fmt.Fprintln(os.Stderr, "failed")
		os.Exit(7)
	case "timeout":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "lines":
		fmt.Fprint(os.Stdout, "one\ntwo\npartial")
		fmt.Fprint(os.Stderr, "err\n")
		os.Exit(0)
	case "truncate":
		fmt.Fprint(os.Stdout, strings.Repeat("x", 10))
		fmt.Fprint(os.Stderr, strings.Repeat("y", 9))
		os.Exit(0)
	case "large-line":
		fmt.Fprint(os.Stdout, strings.Repeat("x", maxLineEmitterPendingBytes+100)+"\nnext\n")
		os.Exit(0)
	case "environment":
		fmt.Fprintf(os.Stdout, "allowed=%s ambient=%s\n", os.Getenv("ONLY_ALLOWED"), os.Getenv("HOME"))
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", os.Getenv("RUNNER_HELPER_MODE"))
		os.Exit(2)
	}
}
