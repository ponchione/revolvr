package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDoctorReportsReadyForDogfood(t *testing.T) {
	workDir := newDoctorGitRepo(t)
	fakeCodex := writeDoctorFakeExecutable(t, "codex-test")

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: `+strconv.Quote(fakeCodex)+`
verification:
  missing_policy: fail
  commands:
    - name: sh
      args: ["-c", "true"]
`)

	out, err := executeCLI(t, workDir, "doctor")
	if err != nil {
		t.Fatalf("execute doctor: %v\n%s", err, out)
	}

	for _, want := range []string{
		"Dogfood preflight:\n",
		"OK state: initialized at " + filepath.Join(workDir, ".revolvr"),
		"OK config: loaded " + filepath.Join(workDir, ".revolvr", "config.yaml"),
		"OK codex executable: " + fakeCodex,
		"OK git executable:",
		"OK git identity: Revolvr Doctor <doctor@example.invalid>",
		"OK worktree clean: no changes",
		"OK runtime state ignored: .revolvr/ ignored by Git",
		"OK verification commands: 1 command configured",
		"Ready: true\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorFailsWhenRequiredChecksAreNotReady(t *testing.T) {
	workDir := newDoctorGitRepo(t)
	missingCodex := filepath.Join(workDir, "missing-codex")

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: `+strconv.Quote(missingCodex)+`
verification:
  missing_policy: fail
`)

	out, err := executeCLI(t, workDir, "doctor")
	if err == nil {
		t.Fatalf("execute doctor succeeded, want failure\n%s", out)
	}
	if got, want := err.Error(), "doctor: preflight failed"; got != want {
		t.Fatalf("doctor error = %q, want %q", got, want)
	}

	for _, want := range []string{
		"FAIL codex executable:",
		"OK git identity: Revolvr Doctor <doctor@example.invalid>",
		"OK worktree clean: no changes",
		"OK runtime state ignored: .revolvr/ ignored by Git",
		"FAIL verification commands: no verification commands configured",
		"Ready: false\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func newDoctorGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	workDir := t.TempDir()
	runDoctorGitTestCommand(t, workDir, "init", "-q")
	runDoctorGitTestCommand(t, workDir, "config", "user.name", "Revolvr Doctor")
	runDoctorGitTestCommand(t, workDir, "config", "user.email", "doctor@example.invalid")
	return workDir
}

func runDoctorGitTestCommand(t *testing.T, workDir string, args ...string) {
	t.Helper()
	allArgs := append([]string{"-C", workDir}, args...)
	cmd := exec.Command("git", allArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(allArgs, " "), err, output)
	}
}

func writeDoctorFakeExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}
