package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/codexexec"
	"revolvr/internal/lock"
	"revolvr/internal/repositorypath"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

type externalPathFixture struct {
	repository string
	outside    string
	options    repositorypath.InspectOptions
	before     map[string]string
}

func TestExternalExecutableIdentityAdmission(t *testing.T) {
	writeCodex := func(t *testing.T, path, version, suffix string) {
		t.Helper()
		content := "#!/bin/sh\nif [ \"${1:-}\" = \"--version\" ]; then\n  printf '%s\\n' " + fmt.Sprintf("%q", version) + "\n  exit 0\nfi\nexit 64\n" + suffix
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	newFixture := func(t *testing.T) (string, string, codexexec.ReleaseManifest, codexexec.CodexExecutableIdentity) {
		t.Helper()
		repository := t.TempDir()
		createSchedulingTask(t, repository, "identity-task", nil)
		createAppPreflightState(t, repository)
		codexPath := filepath.Join(t.TempDir(), "codex")
		writeCodex(t, codexPath, "codex-cli 1.2.3", "")
		executable, err := codexexec.InspectExecutable(codexPath, exec.LookPath)
		if err != nil {
			t.Fatal(err)
		}
		manifest := codexexec.ReleaseManifest{SchemaVersion: codexexec.ReleaseManifestSchema, Codex: []codexexec.ReleaseCodexBuild{{Version: "codex-cli 1.2.3", SHA256: executable.SHA256}}}
		identity := codexexec.CodexExecutableIdentity{Version: manifest.Codex[0].Version, Executable: executable}
		mustWriteExternalPath(t, filepath.Join(repository, ".revolvr", "config.yaml"), "codex:\n  executable: "+fmt.Sprintf("%q", codexPath)+"\nverification:\n  commands: [{name: go}]\n", 0o644)
		return repository, codexPath, manifest, identity
	}
	preflight := func(t *testing.T, repository string, manifest codexexec.ReleaseManifest) PreflightResult {
		t.Helper()
		result, err := Preflight(context.Background(), Config{WorkDir: repository}, PreflightInput{
			CommandRunner: runner.Run,
			LookPath:      exec.LookPath,
			CodexIdentityInspector: func(ctx context.Context, configured, workDir string, cfg codexexec.VersionConfig, lookPath codexexec.ExecutableLookPath) (codexexec.CodexExecutableIdentity, error) {
				return codexexec.InspectCodexWithManifest(ctx, configured, workDir, cfg, lookPath, manifest)
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	repository, _, manifest, identity := newFixture(t)
	ready := preflight(t, repository, manifest)
	if !ready.Ready {
		t.Fatalf("authorized preflight = %+v", ready.Checks)
	}
	var codexDetail, gitDetail string
	for _, check := range ready.Checks {
		switch check.Name {
		case "codex executable":
			codexDetail = check.Detail
		case "git executable":
			gitDetail = check.Detail
		}
	}
	if !strings.Contains(codexDetail, identity.Executable.Resolved) || !strings.Contains(codexDetail, identity.Executable.SHA256) || !strings.Contains(gitDetail, "sha256=") {
		t.Fatalf("identity details: codex=%q git=%q", codexDetail, gitDetail)
	}
	checked, err := CheckRunConfig(repository)
	if err != nil {
		t.Fatal(err)
	}
	if checked.Effective.CodexIdentity != identity || checked.Effective.GitIdentity == (codexexec.ExecutableIdentity{}) {
		t.Fatalf("config-check identities: codex=%+v git=%+v errors=%q/%q", checked.Effective.CodexIdentity, checked.Effective.GitIdentity, checked.CodexIdentityError, checked.GitIdentityError)
	}
	fingerprint := mustFingerprintEffectiveConfig(t, checked.Effective)
	for _, value := range []string{identity.Version, identity.Executable.Resolved, identity.Executable.SHA256, checked.Effective.GitIdentity.Resolved, checked.Effective.GitIdentity.SHA256} {
		if !strings.Contains(string(fingerprint.JSON), value) {
			t.Fatalf("effective fingerprint omitted identity %q: %s", value, fingerprint.JSON)
		}
	}
	invocation, _, err := codexexec.PrepareInvocation(codexexec.InvocationConfig{Executable: identity.Executable.Configured, WorkingDir: repository, Model: codexexec.DefaultModel, ReasoningEffort: codexexec.DefaultReasoningEffort, Ephemeral: true, Artifacts: codexexec.ArtifactPaths{StdoutJSONL: filepath.Join(".revolvr", "runs", "identity.jsonl")}, CodexVersion: identity.Version, EffectiveConfigSchema: fingerprint.Schema, EffectiveConfigSHA256: fingerprint.SHA256, CodexIdentity: identity, GitIdentity: checked.Effective.GitIdentity})
	if err != nil || invocation.CodexIdentity == nil || invocation.GitIdentity == nil || *invocation.CodexIdentity != identity || *invocation.GitIdentity != checked.Effective.GitIdentity {
		t.Fatalf("run provenance identities = %+v err=%v", invocation, err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{name: "unlisted version", mutate: func(t *testing.T, path string) { writeCodex(t, path, "codex-cli 1.2.4", "") }, want: "not release-authorized"},
		{name: "listed version different bytes", mutate: func(t *testing.T, path string) { writeCodex(t, path, "codex-cli 1.2.3", "# different bytes\n") }, want: "not release-authorized"},
		{name: "unresolved executable", mutate: func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}, want: "resolve executable"},
	} {
		t.Run("preflight "+test.name, func(t *testing.T) {
			repository, codexPath, manifest, _ := newFixture(t)
			test.mutate(t, codexPath)
			beforeRuntime := snapshotExternalTree(t, filepath.Join(repository, ".revolvr"))
			beforeTasks := snapshotExternalTree(t, filepath.Join(repository, ".agent"))
			result := preflight(t, repository, manifest)
			if result.Ready {
				t.Fatalf("preflight unexpectedly ready: %+v", result.Checks)
			}
			found := false
			for _, check := range result.Checks {
				if check.Name == "codex executable" && check.Status == PreflightFail && strings.Contains(check.Detail, test.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("preflight checks = %+v, want %q", result.Checks, test.want)
			}
			if after := snapshotExternalTree(t, filepath.Join(repository, ".revolvr")); !reflect.DeepEqual(after, beforeRuntime) {
				t.Fatalf("identity refusal mutated runtime authority\nbefore=%v\nafter=%v", beforeRuntime, after)
			}
			if after := snapshotExternalTree(t, filepath.Join(repository, ".agent")); !reflect.DeepEqual(after, beforeTasks) {
				t.Fatalf("identity refusal mutated task authority\nbefore=%v\nafter=%v", beforeTasks, after)
			}
		})
	}

	gitIdentity, err := codexexec.InspectExecutable("git", exec.LookPath)
	if err != nil {
		t.Fatal(err)
	}
	release, err := codexexec.CurrentReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		version string
		mutate  bool
		remove  bool
		want    string
	}{
		{name: "execution unlisted version", version: "codex-cli 1.2.3", want: "not release-authorized"},
		{name: "execution listed version different bytes", version: release.Codex[0].Version, want: "not release-authorized"},
		{name: "execution unresolved", version: release.Codex[0].Version, remove: true, want: "resolve executable"},
		{name: "execution identity drift", version: release.Codex[0].Version, mutate: true, want: "identity drift"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "codex")
			writeCodex(t, path, "codex-cli 1.2.3", "")
			executable, inspectErr := codexexec.InspectExecutable(path, exec.LookPath)
			if inspectErr != nil {
				t.Fatal(inspectErr)
			}
			if test.remove {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if test.mutate {
				writeCodex(t, path, "codex-cli 1.2.3", "# drift\n")
			}
			cfg := runonce.Config{GitIdentity: gitIdentity, CodexIdentity: codexexec.CodexExecutableIdentity{Version: test.version, Executable: executable}}
			err := recheckExternalExecutableIdentities(cfg, true)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("execution recheck error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestExternalRepositoryShapeAndPlatformMatrix(t *testing.T) {
	newRepository := func(t *testing.T, taskID string) string {
		t.Helper()
		repository := t.TempDir()
		createSchedulingTask(t, repository, taskID, nil)
		createAppPreflightState(t, repository)
		return repository
	}
	hybridRunner := func(ctx context.Context, command runner.Command) runner.Result {
		if reflect.DeepEqual(command.Args, []string{"--version"}) {
			manifest, err := codexexec.CurrentReleaseManifest()
			if err != nil {
				t.Fatal(err)
			}
			return runner.Result{ExitCode: 0, Stdout: manifest.Codex[0].Version + "\n"}
		}
		return runner.Run(ctx, command)
	}
	lookPath := func(name string) (string, error) {
		if name == "codex" {
			return "/fake/bin/codex", nil
		}
		return exec.LookPath(name)
	}
	findCheck := func(t *testing.T, result PreflightResult, name string) PreflightCheck {
		t.Helper()
		for _, check := range result.Checks {
			if check.Name == name {
				return check
			}
		}
		t.Fatalf("check %q missing from %+v", name, result.Checks)
		return PreflightCheck{}
	}

	repository := newRepository(t, "platform-task")
	for _, test := range []struct {
		mode     PreflightMode
		platform string
		want     PreflightCheckStatus
	}{
		{PreflightModeAttendedTask, "linux", PreflightOK},
		{PreflightModeAttendedTask, "darwin", PreflightOK},
		{PreflightModeAttendedTask, "freebsd", PreflightOK},
		{PreflightModeAttendedTask, "plan9", PreflightFail},
		{PreflightModeQueue, "linux", PreflightOK},
		{PreflightModeQueue, "darwin", PreflightFail},
		{PreflightModeQueue, "freebsd", PreflightFail},
		{PreflightModeDaemon, "linux", PreflightOK},
		{PreflightModeDaemon, "darwin", PreflightFail},
	} {
		t.Run(string(test.mode)+"/"+test.platform, func(t *testing.T) {
			result, err := Preflight(context.Background(), Config{WorkDir: repository}, PreflightInput{Mode: test.mode, Platform: test.platform, CommandRunner: hybridRunner, LookPath: lookPath, ExecutableInspector: testPreflightExecutableInspector, CodexIdentityInspector: testPreflightCodexIdentityInspector})
			if err != nil {
				t.Fatal(err)
			}
			if got := findCheck(t, result, "platform"); got.Status != test.want {
				t.Fatalf("platform check = %+v, want %s", got, test.want)
			}
		})
	}

	t.Run("safe non-bare worktree", func(t *testing.T) {
		calls := 0
		result, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repository}, TaskRunInput{OperationID: "repository-safe", TaskID: "platform-task", MaxCycles: 1, Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			calls++
			return autonomoustaskrun.StepResult{StopReason: autonomoustaskrun.StopSafety, StopDetail: "bounded fixture"}, nil
		}})
		if err != nil || calls != 1 || result.StopReason != autonomoustaskrun.StopSafety {
			t.Fatalf("safe admission result=%+v calls=%d err=%v", result, calls, err)
		}
	})

	assertRefused := func(t *testing.T, repository, want string) {
		t.Helper()
		beforeRuntime := snapshotExternalTree(t, filepath.Join(repository, ".revolvr"))
		taskRoot := filepath.Join(repository, ".agent")
		beforeTasks := map[string]string(nil)
		tasksPresent := true
		if _, err := os.Lstat(taskRoot); errors.Is(err, os.ErrNotExist) {
			tasksPresent = false
		} else if err != nil {
			t.Fatal(err)
		} else {
			beforeTasks = snapshotExternalTree(t, taskRoot)
		}
		calls := 0
		_, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repository}, TaskRunInput{OperationID: "scope-refusal", MaxCycles: 1, Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			calls++
			return autonomoustaskrun.StepResult{}, nil
		}})
		if err == nil || !strings.Contains(err.Error(), want) || calls != 0 {
			t.Fatalf("refusal error=%v calls=%d, want %q", err, calls, want)
		}
		if after := snapshotExternalTree(t, filepath.Join(repository, ".revolvr")); !reflect.DeepEqual(after, beforeRuntime) {
			t.Fatalf("refused admission mutated runtime authority\nbefore=%v\nafter=%v", beforeRuntime, after)
		}
		if tasksPresent {
			if after := snapshotExternalTree(t, taskRoot); !reflect.DeepEqual(after, beforeTasks) {
				t.Fatalf("refused admission mutated task authority\nbefore=%v\nafter=%v", beforeTasks, after)
			}
		} else if _, err := os.Lstat(taskRoot); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("refused admission created task authority: %v", err)
		}
	}

	t.Run("bare repository", func(t *testing.T) {
		repository := t.TempDir()
		runSchedulingGit(t, repository, "init", "--bare", "-q")
		mustWriteExternalPath(t, filepath.Join(repository, ".revolvr", "config.yaml"), "verification:\n  commands: [{name: go}]\n", 0o644)
		assertRefused(t, repository, "bare Git repositories")
	})

	t.Run("active submodule", func(t *testing.T) {
		repository := newRepository(t, "submodule-task")
		child := t.TempDir()
		runSchedulingGit(t, child, "init", "-q")
		runSchedulingGit(t, child, "config", "user.name", "Revolvr Test")
		runSchedulingGit(t, child, "config", "user.email", "revolvr@example.invalid")
		mustWriteExternalPath(t, filepath.Join(child, "child.txt"), "child\n", 0o644)
		runSchedulingGit(t, child, "add", "child.txt")
		runSchedulingGit(t, child, "commit", "-q", "-m", "Child")
		runSchedulingGit(t, repository, "-c", "protocol.file.allow=always", "submodule", "add", "-q", child, "vendor/child")
		runSchedulingGit(t, repository, "commit", "-q", "-am", "Add submodule")
		assertRefused(t, repository, "active submodules")
	})

	t.Run("missing verification", func(t *testing.T) {
		repository := newRepository(t, "verification-task")
		mustWriteExternalPath(t, filepath.Join(repository, ".revolvr", "config.yaml"), "verification:\n  commands: []\ncommit:\n  allow_missing_verification: true\n", 0o644)
		assertRefused(t, repository, "verification commands")
	})

	t.Run("dirty worktree", func(t *testing.T) {
		repository := newRepository(t, "dirty-task")
		path := filepath.Join(repository, ".agent", "tasks", "dirty-task.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, []byte("dirty\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		assertRefused(t, repository, "worktree clean")
	})

	t.Run("unresolved git", func(t *testing.T) {
		repository := newRepository(t, "git-task")
		mustWriteExternalPath(t, filepath.Join(repository, ".revolvr", "config.yaml"), "git:\n  executable: definitely-missing-revolvr-git\nverification:\n  commands: [{name: go}]\n", 0o644)
		assertRefused(t, repository, "git executable")
	})
}

func TestAttendedEffectiveBoundsVisibleAndRecorded(t *testing.T) {
	repository := t.TempDir()
	createSchedulingTask(t, repository, "bounded-task", nil)
	createAppPreflightState(t, repository)
	mustWriteExternalPath(t, filepath.Join(repository, ".revolvr", "config.yaml"), `notifications:
  enabled: true
  events: [task_completed]
  executable: "true"
  directory: repository_root
  timeout_seconds: 2
  stdout_cap_bytes: 128
  stderr_cap_bytes: 128
  maximum_attempts: 2
  retry_delay_seconds: 0
verification:
  commands: [{name: go}]
`, 0o644)

	checked, err := CheckRunConfig(repository)
	if err != nil {
		t.Fatal(err)
	}
	bounds := checked.Effective.OperationalBounds
	if bounds.SchemaVersion != runonce.OperationalBoundsSchema || bounds.TaskAttempts != 16 || bounds.Elapsed != 4*60*60*1e9 || bounds.ModelTokens != 1_000_000 || bounds.CyclesPerTask != 50 || bounds.ProcessDuration != 30*60*1e9 || bounds.OutputBytesPerStream != 262144 || bounds.RetainedDiskBytes != 1<<30 || bounds.NotificationAttempts != 2 || len(bounds.ActionAttempts) != 6 {
		t.Fatalf("effective attended bounds = %+v", bounds)
	}
	if !strings.Contains(string(mustFingerprintEffectiveConfig(t, checked.Effective).JSON), `"operational_bounds"`) {
		t.Fatal("effective configuration fingerprint omitted operational bounds")
	}

	hybridRunner := func(ctx context.Context, command runner.Command) runner.Result {
		if reflect.DeepEqual(command.Args, []string{"--version"}) {
			manifest, err := codexexec.CurrentReleaseManifest()
			if err != nil {
				t.Fatal(err)
			}
			return runner.Result{ExitCode: 0, Stdout: manifest.Codex[0].Version + "\n"}
		}
		return runner.Run(ctx, command)
	}
	preflight, err := Preflight(context.Background(), Config{WorkDir: repository}, PreflightInput{CommandRunner: hybridRunner, ExecutableInspector: testPreflightExecutableInspector, CodexIdentityInspector: testPreflightCodexIdentityInspector, LookPath: func(name string) (string, error) {
		if name == "codex" {
			return "/fake/bin/codex", nil
		}
		return exec.LookPath(name)
	}})
	if err != nil || !preflight.Ready {
		t.Fatalf("preflight=%+v err=%v", preflight, err)
	}
	visible := false
	for _, check := range preflight.Checks {
		if check.Name == "operational bounds" && strings.Contains(check.Detail, "model_tokens=1000000") && strings.Contains(check.Detail, "notification_attempts=2") {
			visible = true
		}
	}
	if !visible {
		t.Fatalf("operational bounds not visible in doctor projection: %+v", preflight.Checks)
	}

	calls := 0
	result, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repository}, TaskRunInput{OperationID: "bounded-evidence", TaskID: "bounded-task", MaxCycles: 3, Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
		calls++
		return autonomoustaskrun.StepResult{StopReason: autonomoustaskrun.StopSafety, StopDetail: "bounded fixture"}, nil
	}})
	if err != nil || calls != 1 || result.StopReason != autonomoustaskrun.StopSafety {
		t.Fatalf("run result=%+v calls=%d err=%v", result, calls, err)
	}
	operation, found, err := autonomoustaskrun.Inspect(repository, "bounded-evidence")
	if err != nil || !found || operation.EffectiveBounds == nil {
		t.Fatalf("durable operation=%+v found=%t err=%v", operation, found, err)
	}
	recorded := operation.EffectiveBounds
	if recorded.TaskAttempts != 16 || recorded.ModelTokens != 1_000_000 || recorded.CyclesPerTask != 3 || recorded.ProcessNanoseconds != int64(30*60*1e9) || recorded.OutputBytesPerStream != 262144 || recorded.RetainedDiskBytes != 1<<30 || recorded.NotificationAttempts != 2 || len(recorded.ActionAttempts) != 6 || operation.MaxCycles.Limit != 3 {
		t.Fatalf("recorded bounds = %+v operation=%+v", recorded, operation)
	}
}

func TestExternalSourceWriterLockWindowAdmission(t *testing.T) {
	repository := t.TempDir()
	createSchedulingTask(t, repository, "lock-window-task", nil)
	createAppPreflightState(t, repository)
	mustWriteExternalPath(t, filepath.Join(repository, ".revolvr", "config.yaml"), `codex:
  timeout_seconds: 1800
git:
  timeout_seconds: 30
verification:
  commands: [{name: go}]
`, 0o644)

	checked, err := CheckRunConfig(repository)
	if err != nil {
		t.Fatal(err)
	}
	required, err := lock.RequiredSourceWriterTimeout(checked.Effective.CodexTimeout, checked.Effective.GitTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if required != 32*time.Minute || checked.Effective.SourceWriterLockTimeout != required || checked.Effective.SourceWriterLockHeartbeatInterval != 10*time.Minute+40*time.Second {
		t.Fatalf("effective source-writer authority = timeout %s heartbeat %s required %s", checked.Effective.SourceWriterLockTimeout, checked.Effective.SourceWriterLockHeartbeatInterval, required)
	}
	if checked.EffectiveConfigSchema != "revolvr-effective-run-config-v8" {
		t.Fatalf("effective config schema = %q", checked.EffectiveConfigSchema)
	}
	fingerprint := mustFingerprintEffectiveConfig(t, checked.Effective)
	if fingerprint.Schema != runonce.EffectiveConfigSchema || fingerprint.Projection.SourceWriterLock.Timeout != required || fingerprint.Projection.SourceWriterLock.HeartbeatInterval != checked.Effective.SourceWriterLockHeartbeatInterval {
		t.Fatalf("fingerprinted source-writer authority = %+v", fingerprint.Projection.SourceWriterLock)
	}

	modelAttempts := 0
	readyRunner := readyPreflightCommandRunner(t)
	preflight, err := Preflight(context.Background(), Config{WorkDir: repository}, PreflightInput{
		Mode:   PreflightModeAttendedTask,
		TaskID: "lock-window-task",
		CommandRunner: func(ctx context.Context, command runner.Command) runner.Result {
			if len(command.Args) > 0 && command.Args[0] == "exec" {
				modelAttempts++
				return runner.Result{ExitCode: 64, Err: errors.New("model invocation is forbidden in preflight")}
			}
			return readyRunner(ctx, command)
		},
		LookPath:               preflightLookPath(map[string]string{"codex": "/fake/bin/codex", "git": "/fake/bin/git"}),
		ExecutableInspector:    testPreflightExecutableInspector,
		CodexIdentityInspector: testPreflightCodexIdentityInspector,
	})
	if err != nil || !preflight.Ready || modelAttempts != 0 {
		t.Fatalf("preflight=%+v model_attempts=%d err=%v", preflight, modelAttempts, err)
	}
	lockReady := false
	for _, check := range preflight.Checks {
		if check.Name == "source-writer lock" && check.Status == PreflightOK && check.Detail == "timeout=32m0s heartbeat_interval=10m40s required=32m0s" {
			lockReady = true
		}
	}
	if !lockReady {
		t.Fatalf("source-writer lock authority not visible in ready preflight: %+v", preflight.Checks)
	}

	invalid := checked.Effective
	invalid.SourceWriterLockTimeout = required - time.Second
	if _, err := loadEffectiveExternalConfig(repository, 0, &invalid); err == nil || !strings.Contains(err.Error(), "shorter than required supervisor window") {
		t.Fatalf("invalid external source-writer authority error = %v", err)
	}
}

func mustFingerprintEffectiveConfig(t *testing.T, cfg runonce.Config) runonce.EffectiveConfigFingerprint {
	t.Helper()
	fingerprint, err := runonce.FingerprintEffectiveConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func TestModeAwarePreflight(t *testing.T) {
	newReadyRepository := func(t *testing.T) string {
		t.Helper()
		repository := t.TempDir()
		createAppPreflightState(t, repository)
		createSchedulingTask(t, repository, "ready-task", nil)
		mustWriteExternalPath(t, filepath.Join(repository, filepath.FromSlash(repositorypath.ConfigFile)), "verification:\n  commands: [{name: go}]\n", 0o644)
		return repository
	}
	preflightInput := func(mode PreflightMode, taskID string, called *bool) PreflightInput {
		return PreflightInput{
			Mode:   mode,
			TaskID: taskID,
			CommandRunner: func(ctx context.Context, command runner.Command) runner.Result {
				*called = true
				return readyPreflightCommandRunner(t)(ctx, command)
			},
			LookPath:               preflightLookPath(map[string]string{"codex": "/fake/bin/codex", "git": "/fake/bin/git"}),
			ExecutableInspector:    testPreflightExecutableInspector,
			CodexIdentityInspector: testPreflightCodexIdentityInspector,
		}
	}
	graphCheck := func(t *testing.T, result PreflightResult) PreflightCheck {
		t.Helper()
		for _, check := range result.Checks {
			if check.Name == "task graph" {
				return check
			}
		}
		t.Fatalf("task graph check missing from %+v", result.Checks)
		return PreflightCheck{}
	}

	repository := newReadyRepository(t)
	before := snapshotExternalTree(t, repository)
	var bare PreflightResult
	for _, test := range []struct {
		name, taskID string
		mode         PreflightMode
	}{
		{name: "bare"},
		{name: "attended", mode: PreflightModeAttendedTask},
		{name: "selected attended", mode: PreflightModeAttendedTask, taskID: "ready-task"},
		{name: "queue", mode: PreflightModeQueue},
		{name: "daemon", mode: PreflightModeDaemon},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			result, err := Preflight(context.Background(), Config{WorkDir: repository}, preflightInput(test.mode, test.taskID, &called))
			if err != nil || !result.Ready || !called {
				t.Fatalf("preflight result=%+v called=%t err=%v, want ready mode", result, called, err)
			}
			wantMode := test.mode
			if wantMode == "" {
				wantMode = PreflightModeAttendedTask
			}
			if result.Mode != wantMode || result.TaskID != test.taskID {
				t.Fatalf("preflight authority = mode %q task %q, want %q/%q", result.Mode, result.TaskID, wantMode, test.taskID)
			}
			check := graphCheck(t, result)
			if check.Status != PreflightOK || !strings.Contains(check.Detail, "mode="+string(wantMode)+" canonical_tasks=1 autonomous_tasks=1") {
				t.Fatalf("task graph check = %+v", check)
			}
			if test.taskID != "" && !strings.Contains(check.Detail, "task=ready-task readiness=ready") {
				t.Fatalf("selected task graph check = %+v", check)
			}
			if test.name == "bare" {
				bare = result
			}
			if test.name == "attended" && !reflect.DeepEqual(result, bare) {
				t.Fatalf("bare and explicit attended preflight differ\nbare=%+v\nexplicit=%+v", bare, result)
			}
		})
	}
	if after := snapshotExternalTree(t, repository); !reflect.DeepEqual(after, before) {
		t.Fatalf("mode-aware preflight mutated repository\nbefore=%v\nafter=%v", before, after)
	}

	for _, test := range []struct {
		name, taskID string
		mode         PreflightMode
	}{
		{name: "unknown mode", mode: "attended"},
		{name: "queue selector", mode: PreflightModeQueue, taskID: "ready-task"},
		{name: "daemon selector", mode: PreflightModeDaemon, taskID: "ready-task"},
		{name: "non-exact selector", mode: PreflightModeAttendedTask, taskID: " ready-task "},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			invalidBefore := snapshotExternalTree(t, repository)
			result, err := Preflight(context.Background(), Config{WorkDir: repository}, preflightInput(test.mode, test.taskID, &called))
			if err == nil || called || !reflect.DeepEqual(result, PreflightResult{}) {
				t.Fatalf("invalid preflight result=%+v called=%t err=%v", result, called, err)
			}
			if after := snapshotExternalTree(t, repository); !reflect.DeepEqual(after, invalidBefore) {
				t.Fatalf("invalid request mutated repository\nbefore=%v\nafter=%v", invalidBefore, after)
			}
		})
	}

	for _, mode := range []PreflightMode{PreflightModeAttendedTask, PreflightModeQueue, PreflightModeDaemon} {
		t.Run("unsafe state "+string(mode), func(t *testing.T) {
			unsafeRepository := newReadyRepository(t)
			statePath := filepath.Join(unsafeRepository, ".revolvr", "autonomous", "tasks", "ready-task", "state.json")
			if err := os.Chmod(statePath, 0o666); err != nil {
				t.Fatal(err)
			}
			called := false
			result, err := Preflight(context.Background(), Config{WorkDir: unsafeRepository}, preflightInput(mode, "", &called))
			check := graphCheck(t, result)
			if err != nil || result.Ready || called || check.Status != PreflightFail || !strings.Contains(check.Detail, "unsafe") {
				t.Fatalf("unsafe state preflight result=%+v called=%t err=%v", result, called, err)
			}
		})
	}
	for _, mode := range []PreflightMode{PreflightModeAttendedTask, PreflightModeQueue, PreflightModeDaemon} {
		t.Run("invalid graph "+string(mode), func(t *testing.T) {
			invalidRepository := t.TempDir()
			createAppPreflightState(t, invalidRepository)
			createSchedulingTask(t, invalidRepository, "invalid-task", []string{"missing-task"})
			mustWriteExternalPath(t, filepath.Join(invalidRepository, filepath.FromSlash(repositorypath.ConfigFile)), "verification:\n  commands: [{name: go}]\n", 0o644)
			called := false
			result, err := Preflight(context.Background(), Config{WorkDir: invalidRepository}, preflightInput(mode, "", &called))
			check := graphCheck(t, result)
			if err != nil || result.Ready || called || check.Status != PreflightFail || !strings.Contains(check.Detail, "missing_dependency") {
				t.Fatalf("invalid graph preflight result=%+v called=%t err=%v", result, called, err)
			}
		})
	}

	passed, err := Preflight(context.Background(), Config{WorkDir: repository}, preflightInput(PreflightModeAttendedTask, "ready-task", new(bool)))
	if err != nil || !passed.Ready {
		t.Fatalf("pre-mutation preflight = %+v err=%v", passed, err)
	}
	statePath := filepath.Join(repository, ".revolvr", "autonomous", "tasks", "ready-task", "state.json")
	if err := os.Chmod(statePath, 0o666); err != nil {
		t.Fatal(err)
	}
	runnerCalled := false
	_, err = RunTaskUntilTerminal(context.Background(), Config{WorkDir: repository}, TaskRunInput{
		OperationID: "mode-aware-recheck",
		TaskID:      "ready-task",
		MaxCycles:   1,
		Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			runnerCalled = true
			return autonomoustaskrun.StepResult{}, nil
		},
	})
	if !errors.Is(err, runtimepath.ErrUnsafe) || runnerCalled {
		t.Fatalf("execution recheck error=%v runner_called=%t, want unsafe no-model refusal", err, runnerCalled)
	}
}

func TestExternalPreflightSharedPathMatrix(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(*testing.T, *externalPathFixture)
		wantSafe        bool
		wantInitialized bool
	}{
		{name: "safe", wantSafe: true, wantInitialized: true},
		{name: "missing agent", setup: func(t *testing.T, f *externalPathFixture) {
			mustRemoveExternalPath(t, filepath.Join(f.repository, repositorypath.AgentDir))
		}, wantSafe: true, wantInitialized: true},
		{name: "missing runtime", setup: func(t *testing.T, f *externalPathFixture) {
			mustRemoveExternalPath(t, filepath.Join(f.repository, repositorypath.RuntimeDir))
		}, wantSafe: true, wantInitialized: false},
		{name: "malformed agent", setup: func(t *testing.T, f *externalPathFixture) {
			mustRemoveExternalPath(t, filepath.Join(f.repository, repositorypath.AgentDir))
			mustWriteExternalPath(t, filepath.Join(f.repository, repositorypath.AgentDir), "not a directory\n", 0o644)
		}},
		{name: "malformed runtime", setup: func(t *testing.T, f *externalPathFixture) {
			mustRemoveExternalPath(t, filepath.Join(f.repository, repositorypath.RuntimeDir))
			mustWriteExternalPath(t, filepath.Join(f.repository, repositorypath.RuntimeDir), "not a directory\n", 0o644)
		}},
		{name: "agent final symlink", setup: func(t *testing.T, f *externalPathFixture) {
			path := filepath.Join(f.repository, taskfile.TasksDir, "task.md")
			mustRemoveExternalPath(t, path)
			mustSymlinkExternalPath(t, filepath.Join(f.outside, "sentinel"), path)
		}},
		{name: "runtime final symlink", setup: func(t *testing.T, f *externalPathFixture) {
			path := filepath.Join(f.repository, filepath.FromSlash(repositorypath.LedgerFile))
			mustRemoveExternalPath(t, path)
			mustSymlinkExternalPath(t, filepath.Join(f.outside, "sentinel"), path)
		}},
		{name: "agent ancestor symlink", setup: func(t *testing.T, f *externalPathFixture) {
			path := filepath.Join(f.repository, repositorypath.AgentDir)
			mustRemoveExternalPath(t, path)
			mustSymlinkExternalPath(t, f.outside, path)
		}},
		{name: "runtime ancestor symlink", setup: func(t *testing.T, f *externalPathFixture) {
			path := filepath.Join(f.repository, repositorypath.RuntimeDir)
			mustRemoveExternalPath(t, path)
			mustSymlinkExternalPath(t, f.outside, path)
		}},
		{name: "agent hard link", setup: func(t *testing.T, f *externalPathFixture) {
			path := filepath.Join(f.repository, taskfile.TasksDir, "task.md")
			mustRemoveExternalPath(t, path)
			if err := os.Link(filepath.Join(f.outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "runtime hard link", setup: func(t *testing.T, f *externalPathFixture) {
			path := filepath.Join(f.repository, filepath.FromSlash(repositorypath.LedgerFile))
			mustRemoveExternalPath(t, path)
			if err := os.Link(filepath.Join(f.outside, "sentinel"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "agent group writable", setup: func(t *testing.T, f *externalPathFixture) {
			if err := os.Chmod(filepath.Join(f.repository, repositorypath.AgentDir), 0o775); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "runtime group writable", setup: func(t *testing.T, f *externalPathFixture) {
			if err := os.Chmod(filepath.Join(f.repository, repositorypath.RuntimeDir), 0o775); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "agent identity substituted", setup: func(t *testing.T, f *externalPathFixture) {
			f.options.AfterOpen = substituteExternalPath(t, f, repositorypath.AgentDir)
		}},
		{name: "runtime identity substituted", setup: func(t *testing.T, f *externalPathFixture) {
			f.options.AfterOpen = substituteExternalPath(t, f, repositorypath.RuntimeDir)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := externalPathFixture{repository: t.TempDir(), outside: t.TempDir()}
			createSafeExternalPaths(t, f.repository)
			mustWriteExternalPath(t, filepath.Join(f.outside, "sentinel"), "outside authority\n", 0o600)
			if test.setup != nil {
				test.setup(t, &f)
			}
			if f.options.AfterOpen == nil {
				f.before = snapshotExternalTree(t, f.repository)
			}
			outsideBefore := snapshotExternalTree(t, f.outside)

			authority, err := repositorypath.Inspect(f.repository, f.options)
			if test.wantSafe {
				if err != nil {
					t.Fatalf("shared inspection: %v", err)
				}
				if authority.Initialized() != test.wantInitialized {
					t.Fatalf("initialized = %t, want %t", authority.Initialized(), test.wantInitialized)
				}
			} else {
				if !errors.Is(err, runtimepath.ErrUnsafe) {
					t.Fatalf("shared inspection error = %v, want unsafe refusal", err)
				}
				assertExternalPathConsumersRefuse(t, f.repository)
			}
			if f.before == nil {
				t.Fatal("identity-substitution fixture did not capture its post-substitution baseline")
			}
			if after := snapshotExternalTree(t, f.repository); !reflect.DeepEqual(after, f.before) {
				t.Fatalf("repository changed after inspection/refusal\nbefore=%v\nafter=%v", f.before, after)
			}
			if after := snapshotExternalTree(t, f.outside); !reflect.DeepEqual(after, outsideBefore) {
				t.Fatalf("outside tree changed after inspection/refusal\nbefore=%v\nafter=%v", outsideBefore, after)
			}
		})
	}
}

func assertExternalPathConsumersRefuse(t *testing.T, repository string) {
	t.Helper()
	commandCalled := false
	preflight, err := Preflight(context.Background(), Config{WorkDir: repository}, PreflightInput{
		CommandRunner: func(context.Context, runner.Command) runner.Result {
			commandCalled = true
			return runner.Result{}
		},
		LookPath: func(name string) (string, error) { return "/unused/" + name, nil },
	})
	if err != nil || preflight.Ready || commandCalled || len(preflight.Checks) != 1 || preflight.Checks[0].Status != PreflightFail {
		t.Fatalf("preflight = %+v err=%v command_called=%t, want one read-only path refusal", preflight, err, commandCalled)
	}
	if _, err := Status(context.Background(), Config{WorkDir: repository}); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("status error = %v, want unsafe refusal", err)
	}
	if _, err := taskfile.LoadAll(repository); err == nil {
		t.Fatalf("canonical task load error = %v, want unsafe refusal", err)
	}
	runnerCalled := false
	if _, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repository}, TaskRunInput{
		OperationID: "external-path-probe",
		MaxCycles:   1,
		Runner: func(context.Context, autonomoustaskrun.StepInput) (autonomoustaskrun.StepResult, error) {
			runnerCalled = true
			return autonomoustaskrun.StepResult{}, nil
		},
	}); !errors.Is(err, runtimepath.ErrUnsafe) || runnerCalled {
		t.Fatalf("autonomous admission error = %v runner_called=%t, want no-model unsafe refusal", err, runnerCalled)
	}
}

func createSafeExternalPaths(t *testing.T, repository string) {
	t.Helper()
	mustWriteExternalPath(t, filepath.Join(repository, taskfile.TasksDir, "task.md"), "---\nid: task\nstatus: pending\n---\n# Task\n", 0o644)
	mustWriteExternalPath(t, filepath.Join(repository, filepath.FromSlash(repositorypath.ConfigFile)), "verification:\n  commands: [{name: go}]\n", 0o644)
	mustWriteExternalPath(t, filepath.Join(repository, filepath.FromSlash(repositorypath.LedgerFile)), "ledger authority\n", 0o644)
}

func substituteExternalPath(t *testing.T, f *externalPathFixture, relativePath string) func(string) {
	t.Helper()
	used := false
	return func(opened string) {
		if used || opened != relativePath {
			return
		}
		used = true
		path := filepath.Join(f.repository, filepath.FromSlash(relativePath))
		if err := os.Rename(path, path+".displaced"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(f.outside, path); err != nil {
			t.Fatal(err)
		}
		f.before = snapshotExternalTree(t, f.repository)
	}
}

func mustWriteExternalPath(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func mustRemoveExternalPath(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

func mustSymlinkExternalPath(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func snapshotExternalTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("stat identity unavailable for %s", path)
		}
		identity := fmt.Sprintf("mode=%s perm=%04o size=%d mtime=%d links=%d", info.Mode().Type(), info.Mode().Perm(), info.Size(), info.ModTime().UnixNano(), uint64(stat.Nlink))
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			identity += " target=" + target
		case info.Mode().IsRegular():
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			identity += fmt.Sprintf(" sha256=%x", sha256.Sum256(raw))
		}
		result[filepath.ToSlash(rel)] = identity
		return nil
	}); err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return result
}
