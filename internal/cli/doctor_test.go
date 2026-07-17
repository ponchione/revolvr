package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"revolvr/internal/app"
	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/codexexec"
	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

func TestDoctorStatusAdmissionAgreeOnUnsafeAgent(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	if err := os.Chmod(filepath.Join(workDir, ".agent"), 0o775); err != nil {
		t.Fatal(err)
	}
	before := snapshotCLITree(t, workDir)

	doctorOut, doctorErr := executeCLI(t, workDir, "doctor")
	if doctorErr == nil || !strings.Contains(doctorOut, "FAIL state:") || !strings.Contains(doctorOut, `".agent" has unsafe directory mode 0775`) || !strings.Contains(doctorOut, "Ready: false\n") {
		t.Fatalf("doctor output/error = %q / %v, want unsafe .agent refusal", doctorOut, doctorErr)
	}
	if _, err := executeCLI(t, workDir, "status"); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("status error = %v, want unsafe .agent refusal", err)
	}
	if _, err := taskfile.LoadAll(workDir); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("canonical task load error = %v, want unsafe .agent refusal", err)
	}
	runnerCalled := false
	if _, err := app.RunTaskUntilTerminal(context.Background(), app.Config{WorkDir: workDir}, app.TaskRunInput{
		OperationID: "unsafe-agent-probe",
		MaxCycles:   1,
		Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			runnerCalled = true
			return autonomoustaskrun.StepResult{}, nil
		},
	}); !errors.Is(err, runtimepath.ErrUnsafe) || runnerCalled {
		t.Fatalf("admission error = %v runner_called=%t, want no-model unsafe .agent refusal", err, runnerCalled)
	}

	if after := snapshotCLITree(t, workDir); !reflect.DeepEqual(after, before) {
		t.Fatalf("unsafe path checks mutated repository\nbefore=%v\nafter=%v", before, after)
	}
}

func TestDoctorForModesAndTaskSelector(t *testing.T) {
	workDir := t.TempDir()
	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), "verification:\n  commands: [{name: go}]\n")
	projected, err := taskfile.ProjectAutonomousTask(workDir, taskfile.AutonomousCreateInput{ID: "doctor-task", Title: "Doctor Task", Body: "Validate the exact attended task."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := taskfile.PublishAutonomousTask(workDir, projected); err != nil {
		t.Fatal(err)
	}
	state := autonomous.ExecutionState{
		SchemaVersion: autonomous.ExecutionStateSchemaVersion,
		TaskID:        "doctor-task",
		Lifecycle:     autonomous.LifecycleStatePending,
		Attempts: autonomous.AttemptState{
			RetryBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
			ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset},
			TokenBudget:       autonomous.CountBudget{Mode: autonomous.BudgetModeUnset},
		},
	}
	rawState, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "autonomous", "tasks", "doctor-task", "state.json"), string(rawState))

	run := func(args ...string) (string, error, int) {
		var out bytes.Buffer
		commandCalls := 0
		root := NewRootCommand(Options{
			Version: "test",
			Out:     &out,
			WorkDir: workDir,
			DoctorCommandRunner: func(_ context.Context, command runner.Command) runner.Result {
				commandCalls++
				if reflect.DeepEqual(command.Args, []string{"--version"}) {
					return runner.Result{ExitCode: 0, Stdout: "codex-test 1.2.3\n"}
				}
				switch strings.Join(command.Args, "\x00") {
				case "rev-parse\x00--is-bare-repository":
					return runner.Result{ExitCode: 0, Stdout: "false\n"}
				case "rev-parse\x00--show-toplevel":
					return runner.Result{ExitCode: 0, Stdout: command.Dir + "\n"}
				case "submodule\x00status\x00--recursive":
					return runner.Result{ExitCode: 0}
				case "config\x00--get\x00user.name":
					return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
				case "config\x00--get\x00user.email":
					return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
				case "status\x00--porcelain=v1\x00-z\x00--untracked-files=all", "check-ignore\x00--quiet\x00.revolvr/":
					return runner.Result{ExitCode: 0}
				default:
					t.Fatalf("unexpected doctor command: %s %v", command.Name, command.Args)
					return runner.Result{ExitCode: 1}
				}
			},
			ExecutableLookPath:     func(name string) (string, error) { return "/fake/bin/" + name, nil },
			ExecutableInspector:    cliTestExecutableInspector,
			CodexIdentityInspector: cliTestCodexIdentityInspector,
		})
		root.SetArgs(append([]string{"doctor"}, args...))
		executeErr := root.Execute()
		return out.String(), executeErr, commandCalls
	}

	bare, err, calls := run()
	if err != nil || calls == 0 {
		t.Fatalf("bare doctor output=%q calls=%d err=%v", bare, calls, err)
	}
	explicit, err, _ := run("--for", "attended-task")
	if err != nil || explicit != bare {
		t.Fatalf("explicit attended output/error = %q / %v, want bare bytes %q", explicit, err, bare)
	}
	for _, mode := range []string{"queue", "daemon"} {
		out, err, _ := run("--for", mode)
		if err != nil || !strings.Contains(out, "OK task graph: mode="+mode+" canonical_tasks=1 autonomous_tasks=1") {
			t.Fatalf("doctor --for %s output/error = %q / %v", mode, out, err)
		}
	}
	selected, err, _ := run("--for", "attended-task", "--task", "doctor-task")
	if err != nil || !strings.Contains(selected, "task=doctor-task readiness=ready") {
		t.Fatalf("selected doctor output/error = %q / %v", selected, err)
	}

	before := snapshotCLITree(t, workDir)
	for _, args := range [][]string{
		{"--for", "attended"},
		{"--for", "queue", "--task", "doctor-task"},
		{"--for", "daemon", "--task", "doctor-task"},
		{"--for", ""},
		{"--task", ""},
	} {
		out, err, calls := run(args...)
		if err == nil || calls != 0 {
			t.Fatalf("doctor %v output=%q calls=%d err=%v, want pre-command refusal", args, out, calls, err)
		}
	}
	if after := snapshotCLITree(t, workDir); !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid doctor requests mutated repository\nbefore=%v\nafter=%v", before, after)
	}
}

func TestDoctorReportsReadyForDogfood(t *testing.T) {
	workDir := newDoctorGitRepo(t)
	fakeCodex := writeDoctorFakeExecutable(t, "codex-test")
	releaseVersion := currentReleaseCodexVersion(t)

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

	var output bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &output, WorkDir: workDir, DoctorCommandRunner: runner.Run, ExecutableLookPath: exec.LookPath, ExecutableInspector: cliTestExecutableInspector, CodexIdentityInspector: cliTestCodexIdentityInspector})
	root.SetArgs([]string{"doctor"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute doctor: %v\n%s", err, output.String())
	}
	out := output.String()

	for _, want := range []string{
		"Dogfood preflight:\n",
		"OK state: initialized at " + filepath.Join(workDir, ".revolvr"),
		"OK config: loaded " + filepath.Join(workDir, ".revolvr", "config.yaml"),
		"OK codex executable: configured=" + strconv.Quote(fakeCodex),
		"OK codex model: gpt-doctor",
		"OK codex reasoning effort: high",
		"OK codex session: ephemeral (ephemeral=true)",
		"OK codex version: " + releaseVersion + " (release-authorized exact identity)",
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
			case "rev-parse\x00--is-bare-repository":
				return runner.Result{ExitCode: 0, Stdout: "false\n"}
			case "rev-parse\x00--show-toplevel":
				return runner.Result{ExitCode: 0, Stdout: command.Dir + "\n"}
			case "submodule\x00status\x00--recursive":
				return runner.Result{ExitCode: 0}
			case "config\x00--get\x00user.name":
				return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
			case "config\x00--get\x00user.email":
				return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
			case "status\x00--porcelain=v1\x00-z\x00--untracked-files=all":
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
		ExecutableInspector:    cliTestExecutableInspector,
		CodexIdentityInspector: cliTestCodexIdentityInspector,
	})
	root.SetArgs([]string{"doctor"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute doctor: %v\n%s", err, out.String())
	}
	manifest, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	want := "Dogfood preflight:\n" +
		"OK state: initialized at " + filepath.Join(workDir, ".revolvr") + "\n" +
		"OK config: loaded " + filepath.Join(workDir, ".revolvr", "config.yaml") + "\n" +
		"OK task graph: mode=attended-task canonical_tasks=0 autonomous_tasks=0\n" +
		"OK platform: mode=attended-task os=linux\n" +
		"OK operational bounds: task_attempts=16 action_attempts=[audit=4,correct=4,document=4,implement=4,plan=4,simplify=4] elapsed=4h0m0s model_tokens=1000000 cycles_per_task=50 process_duration=30m0s output_bytes_per_stream=262144 retained_disk_bytes=1073741824 notification_attempts=0\n" +
		"OK git executable: configured=\"git-test\" resolved=\"/fake/bin/git-test\" sha256=" + strings.Repeat("b", 64) + "\n" +
		"OK repository shape: operator-controlled non-bare Git worktree at " + workDir + "\n" +
		"OK active submodules: none\n" +
		"OK worktree clean: no changes\n" +
		"OK verification commands: 1 command configured\n" +
		"OK autonomy safety: mode=operator_attended; operator remains responsible for host, network, hooks, and credentials; worktree isolation is Git/source isolation only\n" +
		"OK autonomous queue: schema=autonomous-queue-policy-v1 maximum_workers=1\n" +
		"OK artifact retention: schema=revolvr-artifact-retention-policy-v1 mutation_enabled=false recent_runs=20\n" +
		"OK notification hooks: disabled; no executable lookup, environment load, outbox write, or process start\n" +
		"OK codex executable: configured=\"codex-test\" resolved=\"/fake/bin/codex-test\" sha256=" + manifest.Codex[0].SHA256 + "\n" +
		"OK codex model: gpt-5.6-sol\n" +
		"OK codex reasoning effort: xhigh\n" +
		"OK codex session: ephemeral (ephemeral=true)\n" +
		"OK codex version: " + manifest.Codex[0].Version + " (release-authorized exact identity)\n" +
		"OK git identity: Revolvr Doctor <doctor@example.invalid>\n" +
		"OK runtime state ignored: .revolvr/ ignored by Git\n" +
		"Ready: true\n"
	if got := out.String(); got != want {
		t.Fatalf("doctor output = %q, want %q", got, want)
	}
}

func TestDoctorFailsWhenRequiredChecksAreNotReady(t *testing.T) {
	workDir := newDoctorGitRepo(t)
	missingCodex := filepath.Join(workDir, "missing-codex")
	writeCLIFile(t, filepath.Join(workDir, "go.mod"), "module example.com/doctor-empty-verification\n")
	runDoctorGitTestCommand(t, workDir, "add", "go.mod")
	runDoctorGitTestCommand(t, workDir, "commit", "-q", "-m", "Add Go module")

	if _, err := executeCLI(t, workDir, "init"); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	commitDoctorProfiles(t, workDir)
	writeCLIFile(t, filepath.Join(workDir, ".revolvr", "config.yaml"), `
codex:
  executable: `+strconv.Quote(missingCodex)+`
verification:
  missing_policy: fail
  commands: []
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
			case "rev-parse\x00--is-bare-repository":
				return runner.Result{ExitCode: 0, Stdout: "false\n"}
			case "rev-parse\x00--show-toplevel":
				return runner.Result{ExitCode: 0, Stdout: command.Dir + "\n"}
			case "submodule\x00status\x00--recursive":
				return runner.Result{ExitCode: 0}
			case "config\x00--get\x00user.name":
				return runner.Result{ExitCode: 0, Stdout: "Revolvr Doctor\n"}
			case "config\x00--get\x00user.email":
				return runner.Result{ExitCode: 0, Stdout: "doctor@example.invalid\n"}
			case "status\x00--porcelain=v1\x00-z\x00--untracked-files=all", "check-ignore\x00--quiet\x00.revolvr/":
				return runner.Result{ExitCode: 0}
			default:
				t.Fatalf("unexpected doctor command: %s %v", command.Name, command.Args)
				return runner.Result{ExitCode: 1}
			}
		},
		ExecutableLookPath:     func(name string) (string, error) { return "/fake/bin/" + name, nil },
		ExecutableInspector:    cliTestExecutableInspector,
		CodexIdentityInspector: cliTestCodexIdentityInspector,
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

func cliTestExecutableInspector(configured string, lookPath codexexec.ExecutableLookPath) (codexexec.ExecutableIdentity, error) {
	path, err := lookPath(configured)
	if err != nil {
		return codexexec.ExecutableIdentity{}, err
	}
	return codexexec.ExecutableIdentity{Configured: configured, Resolved: path, SHA256: strings.Repeat("b", 64)}, nil
}

func cliTestCodexIdentityInspector(ctx context.Context, configured, workDir string, cfg codexexec.VersionConfig, lookPath codexexec.ExecutableLookPath) (codexexec.CodexExecutableIdentity, error) {
	path, err := lookPath(configured)
	if err != nil {
		return codexexec.CodexExecutableIdentity{}, err
	}
	cfg.Executable = configured
	cfg.WorkingDir = workDir
	if _, err := codexexec.DiscoverVersion(ctx, cfg); err != nil {
		return codexexec.CodexExecutableIdentity{}, err
	}
	manifest, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		return codexexec.CodexExecutableIdentity{}, err
	}
	return codexexec.CodexExecutableIdentity{Version: manifest.Codex[0].Version, Executable: codexexec.ExecutableIdentity{Configured: configured, Resolved: path, SHA256: manifest.Codex[0].SHA256}}, nil
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
	hardenCLIGitMetadata(t, filepath.Join(workDir, ".git"))
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
	content := "#!/usr/bin/env sh\nif [ \"${1:-}\" = \"--version\" ]; then\n  printf '%s\\n' " + strconv.Quote(currentReleaseCodexVersion(t)) + "\n  exit 0\nfi\nexit 64\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return path
}

func currentReleaseCodexVersion(t *testing.T) string {
	t.Helper()
	manifest, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Codex[0].Version
}
