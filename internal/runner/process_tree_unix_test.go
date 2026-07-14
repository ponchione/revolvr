//go:build unix

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	processTreeHelperEnvironment = "RUNNER_PROCESS_TREE_HELPER"
	processTreeHelperRole        = "RUNNER_PROCESS_TREE_ROLE"
	processTreeHelperRoot        = "RUNNER_PROCESS_TREE_ROOT"
	processTreeHelperIgnoreTerm  = "RUNNER_PROCESS_TREE_IGNORE_TERM"
	processTreeSentinelDelay     = 600 * time.Millisecond
)

func TestRunSettlesDescendantsAfterSuccessfulLeaderExit(t *testing.T) {
	root := t.TempDir()
	command := redirectedBackgroundWriterCommand(root, false, false)
	command.TerminateGracePeriod = 120 * time.Millisecond

	result := Run(context.Background(), command)
	if !errors.Is(result.Err, ErrProcessTreeUnsettled) {
		t.Fatalf("result = %+v, want unsettled process-tree error", result)
	}
	if result.ExitCode != 0 || result.TimedOut {
		t.Fatalf("result = %+v, want successful leader exit without timeout", result)
	}

	assertRedirectedWriterStoppedWithoutMutation(t, root)
}

func TestRunPreservesDeadlineDuringNaturalExitSettlement(t *testing.T) {
	root := t.TempDir()
	command := redirectedBackgroundWriterCommand(root, false, true)
	command.Timeout = 150 * time.Millisecond
	command.TerminateGracePeriod = 400 * time.Millisecond

	result := Run(context.Background(), command)
	if !errors.Is(result.Err, context.DeadlineExceeded) || !result.TimedOut {
		t.Fatalf("result = %+v, want deadline during natural-exit settlement", result)
	}

	assertRedirectedWriterStoppedWithoutMutation(t, root)
}

func TestRunCancellationSettlesRedirectedBackgroundWriter(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultDone := make(chan Result, 1)
	command := redirectedBackgroundWriterCommand(root, true, false)
	command.TerminateGracePeriod = 120 * time.Millisecond
	go func() {
		resultDone <- Run(ctx, command)
	}()
	waitForHelperFiles(t, root, "ready-redirected-writer")

	cancel()
	result := waitForRunnerResult(t, resultDone)
	if !errors.Is(result.Err, context.Canceled) || result.TimedOut {
		t.Fatalf("result = %+v, want caller cancellation", result)
	}

	assertRedirectedWriterStoppedWithoutMutation(t, root)
}

func redirectedBackgroundWriterCommand(root string, keepLeaderRunning, ignoreTerm bool) Command {
	writer := `printf ready > "$RUNNER_WRITER_READY"; sleep 0.6; printf late > "$RUNNER_WRITER_SENTINEL"`
	if ignoreTerm {
		writer = `trap '' TERM; ` + writer
	}
	script := `(` + writer + `) </dev/null >/dev/null 2>&1 & ` +
		`while [ ! -f "$RUNNER_WRITER_READY" ]; do sleep 0.01; done`
	if keepLeaderRunning {
		script += `; sleep 5`
	}
	return Command{
		Name: "/bin/sh",
		Args: []string{"-c", script},
		Env: []string{
			"RUNNER_WRITER_READY=" + filepath.Join(root, "ready-redirected-writer"),
			"RUNNER_WRITER_SENTINEL=" + filepath.Join(root, "sentinel-redirected-writer"),
		},
		Timeout: 5 * time.Second,
	}
}

func assertRedirectedWriterStoppedWithoutMutation(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, "ready-redirected-writer")); err != nil {
		t.Fatalf("redirected background writer did not start: %v", err)
	}
	time.Sleep(processTreeSentinelDelay + 100*time.Millisecond)
	if _, err := os.Stat(filepath.Join(root, "sentinel-redirected-writer")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("redirected background writer mutated after return: stat error %v", err)
	}
}

func TestRunCancellationTerminatesChildAndGrandchild(t *testing.T) {
	root := t.TempDir()
	unrelated, unrelatedDone := startUnrelatedHelper(t, root)
	unrelatedFinished := false
	defer func() {
		if unrelatedFinished {
			return
		}
		_ = unrelated.Process.Kill()
		<-unrelatedDone
	}()
	waitForHelperFiles(t, root, "ready-unrelated")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultDone := make(chan Result, 1)
	command := processTreeHelperCommand(root, false)
	command.TerminateGracePeriod = 120 * time.Millisecond
	go func() {
		resultDone <- Run(ctx, command)
	}()
	waitForHelperFiles(t, root, "ready-root", "ready-child", "ready-grandchild")

	for i := 0; i < 20; i++ {
		cancel()
	}
	result := waitForRunnerResult(t, resultDone)
	if !errors.Is(result.Err, context.Canceled) || result.TimedOut {
		t.Fatalf("result = %+v, want caller cancellation", result)
	}

	select {
	case err := <-unrelatedDone:
		unrelatedFinished = true
		if err != nil {
			t.Fatalf("unrelated helper: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("unrelated helper did not finish")
	}
	if _, err := os.Stat(filepath.Join(root, "sentinel-unrelated")); err != nil {
		t.Fatalf("unrelated helper was affected by cancellation: %v", err)
	}

	assertTreeStoppedWithoutMutation(t, root)
}

func TestRunForceKillsSignalIgnoringDescendantsAfterGrace(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultDone := make(chan Result, 1)
	command := processTreeHelperCommand(root, true)
	grace := 120 * time.Millisecond
	command.TerminateGracePeriod = grace
	go func() {
		resultDone <- Run(ctx, command)
	}()
	waitForHelperFiles(t, root, "ready-root", "ready-child", "ready-grandchild")

	started := time.Now()
	cancel()
	result := waitForRunnerResult(t, resultDone)
	elapsed := time.Since(started)
	if !errors.Is(result.Err, context.Canceled) || result.TimedOut {
		t.Fatalf("result = %+v, want caller cancellation", result)
	}
	if elapsed < grace-25*time.Millisecond {
		t.Fatalf("runner returned after %s, before grace period %s", elapsed, grace)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("runner returned after %s, want bounded force termination", elapsed)
	}

	assertTreeStoppedWithoutMutation(t, root)
}

func TestRunPreservesCancellationWhenCommandExitsDuringGrace(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultDone := make(chan Result, 1)
	command := processTreeHelperCommandForRole(root, "graceful", false)
	command.TerminateGracePeriod = 500 * time.Millisecond
	go func() {
		resultDone <- Run(ctx, command)
	}()
	waitForHelperFiles(t, root, "ready-graceful")

	started := time.Now()
	cancel()
	result := waitForRunnerResult(t, resultDone)
	if !errors.Is(result.Err, context.Canceled) || result.TimedOut {
		t.Fatalf("result = %+v, want caller cancellation", result)
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want preserved cancellation exit code -1", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "graceful exit\n") {
		t.Fatalf("stdout = %q, want graceful output", result.Stdout)
	}
	if elapsed := time.Since(started); elapsed >= command.TerminateGracePeriod {
		t.Fatalf("graceful exit took %s, want less than %s", elapsed, command.TerminateGracePeriod)
	}
}

func processTreeHelperCommand(root string, ignoreTerm bool) Command {
	return processTreeHelperCommandForRole(root, "root", ignoreTerm)
}

func processTreeHelperCommandForRole(root, role string, ignoreTerm bool) Command {
	return Command{
		Name:        os.Args[0],
		Args:        []string{"-test.run=^TestProcessTreeHelperProcess$", "--"},
		Env:         helperEnvironment(role, root, ignoreTerm),
		ReplaceEnv:  true,
		Timeout:     5 * time.Second,
		StdoutLimit: 1024,
		StderrLimit: 1024,
	}
}

func startUnrelatedHelper(t *testing.T, root string) (*exec.Cmd, <-chan error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessTreeHelperProcess$", "--")
	cmd.Env = helperEnvironment("unrelated", root, false)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start unrelated helper: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	return cmd, done
}

func waitForRunnerResult(t *testing.T, done <-chan Result) Result {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not return")
		return Result{}
	}
}

func waitForHelperFiles(t *testing.T, root string, names ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		allExist := true
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(root, name)); err != nil {
				allExist = false
				break
			}
		}
		if allExist {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper files did not appear: %v", names)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func assertTreeStoppedWithoutMutation(t *testing.T, root string) {
	t.Helper()
	for _, role := range []string{"root", "child", "grandchild"} {
		pidBytes, err := os.ReadFile(filepath.Join(root, "pid-"+role))
		if err != nil {
			t.Fatalf("read %s pid: %v", role, err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		if err != nil {
			t.Fatalf("parse %s pid: %v", role, err)
		}
		waitForProcessStop(t, pid)
	}

	time.Sleep(processTreeSentinelDelay + 100*time.Millisecond)
	for _, role := range []string{"child", "grandchild"} {
		path := filepath.Join(root, "sentinel-"+role)
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cancelled %s mutated filesystem after return: stat error %v", role, err)
		}
	}
}

func waitForProcessStop(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for processExecuting(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d remains executable after runner return", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func processExecuting(pid int) bool {
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if runtime.GOOS != "linux" {
		return true
	}
	status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			return len(fields) < 2 || (fields[1] != "Z" && fields[1] != "X")
		}
	}
	return true
}

func TestProcessTreeHelperProcess(t *testing.T) {
	if os.Getenv(processTreeHelperEnvironment) != "1" {
		return
	}

	role := os.Getenv(processTreeHelperRole)
	root := os.Getenv(processTreeHelperRoot)
	ignoreTerm := os.Getenv(processTreeHelperIgnoreTerm) == "true"
	if ignoreTerm {
		signal.Ignore(syscall.SIGTERM)
	}
	writeHelperFile(root, "pid-"+role, strconv.Itoa(os.Getpid()))

	switch role {
	case "root":
		child := helperSubprocess("child", root, ignoreTerm)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "start child: %v\n", err)
			os.Exit(3)
		}
		writeHelperFile(root, "ready-root", "ready")
		if err := child.Wait(); err != nil {
			os.Exit(4)
		}
	case "child":
		grandchild := helperSubprocess("grandchild", root, ignoreTerm)
		grandchild.Stdout = os.Stdout
		grandchild.Stderr = os.Stderr
		if err := grandchild.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "start grandchild: %v\n", err)
			os.Exit(5)
		}
		writeHelperFile(root, "ready-child", "ready")
		go delayedHelperMutation(root, role)
		if err := grandchild.Wait(); err != nil {
			os.Exit(6)
		}
	case "grandchild":
		writeHelperFile(root, "ready-grandchild", "ready")
		delayedHelperMutation(root, role)
		time.Sleep(5 * time.Second)
	case "unrelated":
		writeHelperFile(root, "ready-unrelated", "ready")
		time.Sleep(200 * time.Millisecond)
		writeHelperFile(root, "sentinel-unrelated", "unrelated")
	case "graceful":
		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGTERM)
		writeHelperFile(root, "ready-graceful", "ready")
		<-term
		fmt.Fprintln(os.Stdout, "graceful exit")
	default:
		fmt.Fprintf(os.Stderr, "unknown process-tree helper role %q\n", role)
		os.Exit(2)
	}
}

func helperSubprocess(role, root string, ignoreTerm bool) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessTreeHelperProcess$", "--")
	cmd.Env = helperEnvironment(role, root, ignoreTerm)
	return cmd
}

func helperEnvironment(role, root string, ignoreTerm bool) []string {
	filtered := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, processTreeHelperEnvironment+"=") ||
			strings.HasPrefix(value, processTreeHelperRole+"=") ||
			strings.HasPrefix(value, processTreeHelperRoot+"=") ||
			strings.HasPrefix(value, processTreeHelperIgnoreTerm+"=") ||
			strings.HasPrefix(value, "GORACE=") {
			continue
		}
		filtered = append(filtered, value)
	}
	raceOptions := make([]string, 0, len(strings.Fields(os.Getenv("GORACE")))+1)
	for _, option := range strings.Fields(os.Getenv("GORACE")) {
		if !strings.HasPrefix(option, "atexit_sleep_ms=") {
			raceOptions = append(raceOptions, option)
		}
	}
	raceOptions = append(raceOptions, "atexit_sleep_ms=0")
	return append(filtered,
		processTreeHelperEnvironment+"=1",
		processTreeHelperRole+"="+role,
		processTreeHelperRoot+"="+root,
		processTreeHelperIgnoreTerm+"="+strconv.FormatBool(ignoreTerm),
		"GORACE="+strings.Join(raceOptions, " "),
	)
}

func delayedHelperMutation(root, role string) {
	time.Sleep(processTreeSentinelDelay)
	writeHelperFile(root, "sentinel-"+role, "survived")
}

func writeHelperFile(root, name, content string) {
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write helper file %s: %v\n", name, err)
		os.Exit(7)
	}
}
