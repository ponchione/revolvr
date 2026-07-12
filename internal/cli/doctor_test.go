package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"revolvr/internal/runner"
)

func TestDoctorReportsReadyForDogfood(t *testing.T) {
	workDir := newDoctorGitRepo(t)
	fakeCodex := writeDoctorFakeExecutable(t, "codex-test")

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	commitDoctorProfiles(t, workDir)
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: `+strconv.Quote(fakeCodex)+`
  model: gpt-doctor
  reasoning_effort: high
  ephemeral: true
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
		"OK codex model: gpt-doctor",
		"OK codex reasoning effort: high",
		"OK codex session: ephemeral (ephemeral=true)",
		"OK codex version: codex-test 1.2.3",
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

func TestDoctorOutputPreservedWithStructuredPreflight(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: codex-test
git:
  executable: git-test
verification:
  commands:
    - name: go
`)

	var out bytes.Buffer
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		DoctorCommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			if command.Name == "codex-test" && reflect.DeepEqual(command.Args, []string{"--version"}) {
				return runner.Result{ExitCode: 0, Stdout: "codex-test 1.2.3\n"}
			}
			switch strings.Join(command.Args, "\x00") {
			case "config\x00--get\x00user.name":
				return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
			case "config\x00--get\x00user.email":
				return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
			case "status\x00--short\x00--untracked-files=all":
				return runner.Result{ExitCode: 0}
			case "check-ignore\x00--quiet\x00.revolvr/":
				return runner.Result{ExitCode: 0}
			default:
				t.Fatalf("unexpected doctor command: %s %v", command.Name, command.Args)
				return runner.Result{ExitCode: 1}
			}
		},
		ExecutableLookPath: func(name string) (string, error) {
			switch name {
			case "codex-test":
				return "/fake/bin/codex-test", nil
			case "git-test":
				return "/fake/bin/git-test", nil
			default:
				return "", fmt.Errorf("executable %s not found", name)
			}
		},
	})
	root.SetArgs([]string{"doctor"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute doctor: %v\n%s", err, out.String())
	}
	want := "Dogfood preflight:\n" +
		"OK state: initialized at " + filepath.Join(workDir, ".revolvr") + "\n" +
		"OK config: loaded " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"OK codex executable: /fake/bin/codex-test\n" +
		"OK codex model: gpt-5.6-sol\n" +
		"OK codex reasoning effort: xhigh\n" +
		"OK codex session: ephemeral (ephemeral=true)\n" +
		"OK codex version: codex-test 1.2.3\n" +
		"OK git executable: /fake/bin/git-test\n" +
		"OK git identity: Revolvr Doctor <doctor@example.invalid>\n" +
		"OK worktree clean: no changes\n" +
		"OK runtime state ignored: .revolvr/ ignored by Git\n" +
		"OK verification commands: 1 command configured\n" +
		"Ready: true\n"
	if got := out.String(); got != want {
		t.Fatalf("doctor output = %q, want %q", got, want)
	}
}

func TestDoctorFailsWhenRequiredChecksAreNotReady(t *testing.T) {
	workDir := newDoctorGitRepo(t)
	missingCodex := filepath.Join(workDir, "missing-codex")

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	commitDoctorProfiles(t, workDir)
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

func TestDoctorFailsWhenCodexVersionDiscoveryFails(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), "codex:\n  executable: codex-test\ngit:\n  executable: git-test\nverification:\n  commands:\n    - name: go\n")
	var out bytes.Buffer
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: workDir,
		DoctorCommandRunner: func(_ context.Context, command runner.Command) runner.Result {
			if command.Name == "codex-test" && reflect.DeepEqual(command.Args, []string{"--version"}) {
				return runner.Result{TimedOut: true, Err: context.DeadlineExceeded}
			}
			switch strings.Join(command.Args, "\x00") {
			case "config\x00--get\x00user.name":
				return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
			case "config\x00--get\x00user.email":
				return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
			case "status\x00--short\x00--untracked-files=all", "check-ignore\x00--quiet\x00.revolvr/":
				return runner.Result{ExitCode: 0}
			default:
				t.Fatalf("unexpected doctor command: %s %v", command.Name, command.Args)
				return runner.Result{ExitCode: 1}
			}
		},
		ExecutableLookPath: func(name string) (string, error) { return "/fake/bin/" + name, nil },
	})
	root.SetArgs([]string{"doctor"})
	err := root.Execute()
	if err == nil || err.Error() != "doctor: preflight failed" {
		t.Fatalf("doctor error = %v", err)
	}
	if !strings.Contains(out.String(), "FAIL codex version:") || !strings.Contains(out.String(), "timed out") || !strings.Contains(out.String(), "Ready: false") {
		t.Fatalf("doctor output:\n%s", out.String())
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

func commitDoctorProfiles(t *testing.T, workDir string) {
	t.Helper()
	runDoctorGitTestCommand(t, workDir, "add", ".agent/profiles")
	runDoctorGitTestCommand(t, workDir, "commit", "-q", "-m", "Add revolvr profiles")
}

func writeDoctorFakeExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\nif [ \"${1:-}\" = \"--version\" ]; then\n  printf 'codex-test 1.2.3\\n'\n  exit 0\nfi\nexit 64\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}
