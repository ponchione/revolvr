//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProcessInitializationDoesNotProbeTerminal(t *testing.T) {
	const helperEnv = "REVOLVR_TEST_PROCESS_INITIALIZATION"
	if os.Getenv(helperEnv) == "1" {
		fmt.Printf("main reached TERM=%s\n", os.Getenv("TERM"))
		return
	}

	script, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script command is unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	command := fmt.Sprintf(
		"stty rows 24 cols 80; %s=1 TERM=xterm-256color COLORTERM=truecolor exec %q -test.run=^TestProcessInitializationDoesNotProbeTerminal$ </dev/null",
		helperEnv,
		executable,
	)
	cmd := exec.Command(script, "-qefc", command, "/dev/null")
	cmd.Stdin = strings.NewReader("")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("run helper in PTY: %v; output = %q", err, output.String())
	}

	const wantPrefix = "main reached TERM=xterm-256color\r\n"
	if got := output.String(); !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("process emitted terminal bytes before main or did not restore TERM: %q", got)
	}
}
