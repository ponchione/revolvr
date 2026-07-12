package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
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
	case "environment":
		fmt.Fprintf(os.Stdout, "allowed=%s ambient=%s\n", os.Getenv("ONLY_ALLOWED"), os.Getenv("HOME"))
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", os.Getenv("RUNNER_HELPER_MODE"))
		os.Exit(2)
	}
}
