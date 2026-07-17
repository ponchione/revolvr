package app

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"revolvr/internal/codexexec"
	"revolvr/internal/repositorypath"
	"revolvr/internal/runner"
	"revolvr/internal/runonce"
)

type externalScopeInput struct {
	Mode                   PreflightMode
	Platform               string
	WorkDir                string
	RunConfig              runonce.Config
	CommandRunner          PreflightCommandRunner
	LookPath               ExecutableLookPath
	ExecutableInspector    ExecutableInspector
	CodexIdentityInspector CodexIdentityInspector
	SkipCodexIdentity      bool
}

type externalScopeResult struct {
	Checks []PreflightCheck
	Config runonce.Config
}

func admitExternalMode(ctx context.Context, root string, mode PreflightMode, cycles int64, override *runonce.Config, requireCodexIdentity bool) (runonce.Config, error) {
	return admitExternalModeWithManifest(ctx, root, mode, cycles, override, requireCodexIdentity, nil)
}

func admitExternalModeWithManifest(ctx context.Context, root string, mode PreflightMode, cycles int64, override *runonce.Config, requireCodexIdentity bool, releaseManifest *codexexec.ReleaseManifest) (runonce.Config, error) {
	if _, err := repositorypath.Inspect(root, repositorypath.InspectOptions{}); err != nil {
		return runonce.Config{}, err
	}
	cfg, err := loadEffectiveExternalConfig(root, cycles, override)
	if err != nil {
		return runonce.Config{}, err
	}
	input := externalScopeInput{
		Mode:              mode,
		Platform:          runtime.GOOS,
		WorkDir:           cfg.WorkingDir,
		RunConfig:         cfg,
		CommandRunner:     PreflightCommandRunner(commandRunner(cfg)),
		LookPath:          exec.LookPath,
		SkipCodexIdentity: !requireCodexIdentity,
	}
	if releaseManifest != nil {
		manifest := *releaseManifest
		input.CodexIdentityInspector = func(ctx context.Context, configured, workDir string, cfg codexexec.VersionConfig, lookPath codexexec.ExecutableLookPath) (codexexec.CodexExecutableIdentity, error) {
			return codexexec.InspectCodexWithManifest(ctx, configured, workDir, cfg, lookPath, manifest)
		}
	}
	scope := inspectExternalScope(ctx, input)
	for _, check := range scope.Checks {
		if check.Status == PreflightFail {
			return runonce.Config{}, fmt.Errorf("external %s admission: %s: %s", mode, check.Name, check.Detail)
		}
	}
	return scope.Config, nil
}

func loadEffectiveExternalConfig(root string, cycles int64, override *runonce.Config) (runonce.Config, error) {
	cfg, err := LoadRunOnceConfig(root, DefaultRunOnceConfig(root))
	if override != nil {
		cfg = *override
		cfg.WorkingDir = root
		err = nil
	}
	if err != nil {
		return runonce.Config{}, err
	}
	if cycles > 0 {
		cfg.OperationalBounds.CyclesPerTask = cycles
	}
	return runonce.EffectiveConfig(cfg)
}

func recheckExternalExecutableIdentities(cfg runonce.Config, requireCodex bool) error {
	return recheckExternalExecutableIdentitiesWithManifest(cfg, requireCodex, nil)
}

func recheckExternalExecutableIdentitiesWithManifest(cfg runonce.Config, requireCodex bool, releaseManifest *codexexec.ReleaseManifest) error {
	if err := codexexec.VerifyExecutableIdentity(cfg.GitIdentity, nil); err != nil {
		return fmt.Errorf("external executable admission: Git: %w", err)
	}
	if !requireCodex {
		return nil
	}
	if err := codexexec.VerifyExecutableIdentity(cfg.CodexIdentity.Executable, nil); err != nil {
		return fmt.Errorf("external executable admission: Codex: %w", err)
	}
	manifest := codexexec.ReleaseManifest{}
	var err error
	if releaseManifest != nil {
		manifest = *releaseManifest
	} else {
		manifest, err = codexexec.CurrentReleaseManifest()
		if err != nil {
			return err
		}
	}
	if err := manifest.Authorize(cfg.CodexIdentity); err != nil {
		return fmt.Errorf("external executable admission: %w", err)
	}
	return nil
}

func externalScopeChecks(ctx context.Context, input externalScopeInput) []PreflightCheck {
	return inspectExternalScope(ctx, input).Checks
}

func inspectExternalScope(ctx context.Context, input externalScopeInput) externalScopeResult {
	commandRunner := input.CommandRunner
	if commandRunner == nil {
		commandRunner = runner.Run
	}
	lookPath := input.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	executableInspector := input.ExecutableInspector
	if executableInspector == nil {
		executableInspector = codexexec.InspectExecutable
	}
	codexIdentityInspector := input.CodexIdentityInspector
	if codexIdentityInspector == nil {
		codexIdentityInspector = codexexec.InspectReleaseCodex
	}
	platform := input.Platform
	if platform == "" {
		platform = runtime.GOOS
	}
	var checks []PreflightCheck
	add := func(status PreflightCheckStatus, name, detail string) {
		checks = append(checks, PreflightCheck{Status: status, Name: name, Detail: strings.TrimSpace(detail)})
	}
	cfg := input.RunConfig
	codexExecutable := preflightEffectiveString(cfg.CodexExecutable, DefaultCodexExecutable)
	if input.SkipCodexIdentity {
		add(PreflightOK, "codex executable", "not required for an injected non-production step runner")
		add(PreflightOK, "codex version", "not required for an injected non-production step runner")
	} else {
		identity, err := codexIdentityInspector(ctx, codexExecutable, input.WorkDir, codexexec.VersionConfig{Timeout: cfg.CodexTimeout, StdoutCap: cfg.CodexStdoutCap, StderrCap: cfg.CodexStderrCap, CommandRunner: codexexec.CommandRunner(commandRunner)}, codexexec.ExecutableLookPath(lookPath))
		if err != nil {
			add(PreflightFail, "codex executable", err.Error())
			add(PreflightFail, "codex version", err.Error())
		} else {
			cfg.CodexIdentity = identity
			add(PreflightOK, "codex executable", codexexec.FormatExecutableIdentity(identity.Executable))
			add(PreflightOK, "codex version", identity.Version+" (release-authorized exact identity)")
		}
	}

	if externalPlatformSupported(input.Mode, platform) {
		add(PreflightOK, "platform", fmt.Sprintf("mode=%s os=%s", input.Mode, platform))
	} else {
		add(PreflightFail, "platform", fmt.Sprintf("mode=%s is not supported on %s", input.Mode, platform))
	}
	if err := input.RunConfig.OperationalBounds.Validate(input.RunConfig.NotificationPolicy.Enabled); err != nil {
		add(PreflightFail, "operational bounds", err.Error())
	} else {
		add(PreflightOK, "operational bounds", FormatOperationalBounds(input.RunConfig.OperationalBounds))
	}

	gitExecutable := preflightEffectiveString(cfg.GitExecutable, DefaultGitExecutable)
	gitIdentity, err := executableInspector(gitExecutable, codexexec.ExecutableLookPath(lookPath))
	if err != nil {
		add(PreflightFail, "git executable", fmt.Sprintf("%q not found: %v", gitExecutable, err))
		add(PreflightFail, "repository shape", "not checked because the configured Git executable is unavailable")
		add(PreflightFail, "active submodules", "not checked because the configured Git executable is unavailable")
		add(PreflightFail, "worktree clean", "not checked because the configured Git executable is unavailable")
	} else {
		cfg.GitIdentity = gitIdentity
		add(PreflightOK, "git executable", codexexec.FormatExecutableIdentity(gitIdentity))
		shapeOK := addRepositoryShapeCheck(ctx, add, commandRunner, input.WorkDir, gitExecutable, cfg)
		if shapeOK {
			addActiveSubmoduleCheck(ctx, add, commandRunner, input.WorkDir, gitExecutable, cfg)
			addWorktreeCheck(ctx, add, commandRunner, input.WorkDir, gitExecutable, cfg.GitTimeout, cfg.GitStdoutCap, cfg.GitStderrCap)
		} else {
			add(PreflightFail, "active submodules", "not checked because repository shape is invalid")
			add(PreflightFail, "worktree clean", "not checked because repository shape is invalid")
		}
	}
	addVerificationCheck(add, verificationCommandCount(cfg), false)
	return externalScopeResult{Checks: checks, Config: cfg}
}

func externalPlatformSupported(mode PreflightMode, platform string) bool {
	switch mode {
	case PreflightModeAttendedTask:
		return platform == "linux" || platform == "darwin" || platform == "freebsd"
	case PreflightModeQueue, PreflightModeDaemon:
		return platform == "linux"
	default:
		return false
	}
}

func addRepositoryShapeCheck(ctx context.Context, add func(PreflightCheckStatus, string, string), commandRunner PreflightCommandRunner, workDir, gitExecutable string, cfg runonce.Config) bool {
	bare := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"rev-parse", "--is-bare-repository"}, cfg.GitTimeout, cfg.GitStdoutCap, cfg.GitStderrCap)
	if !preflightCommandPassed(bare) {
		add(PreflightFail, "repository shape", "cannot establish Git repository authority: "+preflightCommandFailure(bare))
		return false
	}
	switch strings.TrimSpace(bare.Stdout) {
	case "true":
		add(PreflightFail, "repository shape", "bare Git repositories are not supported")
		return false
	case "false":
	default:
		add(PreflightFail, "repository shape", "git rev-parse returned malformed bare-repository authority")
		return false
	}
	top := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"rev-parse", "--show-toplevel"}, cfg.GitTimeout, cfg.GitStdoutCap, cfg.GitStderrCap)
	if !preflightCommandPassed(top) || top.StdoutTruncatedBytes != 0 {
		detail := preflightCommandFailure(top)
		if top.StdoutTruncatedBytes != 0 {
			detail = "command output was truncated"
		}
		add(PreflightFail, "repository shape", "cannot establish worktree root: "+detail)
		return false
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(top.Stdout))
	if err != nil {
		add(PreflightFail, "repository shape", "resolve Git worktree root: "+err.Error())
		return false
	}
	want, err := filepath.EvalSymlinks(workDir)
	if err != nil || root != want {
		add(PreflightFail, "repository shape", fmt.Sprintf("Git worktree root %q does not match repository root %q", root, want))
		return false
	}
	add(PreflightOK, "repository shape", "operator-controlled non-bare Git worktree at "+want)
	return true
}

func addActiveSubmoduleCheck(ctx context.Context, add func(PreflightCheckStatus, string, string), commandRunner PreflightCommandRunner, workDir, gitExecutable string, cfg runonce.Config) {
	result := runPreflightGit(ctx, commandRunner, workDir, gitExecutable, []string{"submodule", "status", "--recursive"}, cfg.GitTimeout, cfg.GitStdoutCap, cfg.GitStderrCap)
	if !preflightCommandPassed(result) || result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
		detail := preflightCommandFailure(result)
		if result.StdoutTruncatedBytes != 0 || result.StderrTruncatedBytes != 0 {
			detail = "command output was truncated"
		}
		add(PreflightFail, "active submodules", detail)
		return
	}
	var active []string
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if line == "" || line[0] == '-' {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			add(PreflightFail, "active submodules", "git submodule status returned malformed authority")
			return
		}
		active = append(active, fields[1])
	}
	if len(active) != 0 {
		add(PreflightFail, "active submodules", "active paths: "+strings.Join(active, ", "))
		return
	}
	add(PreflightOK, "active submodules", "none")
}

func verificationCommandCount(cfg runonce.Config) int {
	count := len(cfg.VerificationCommands)
	if cfg.VerificationPlan != nil {
		for _, tier := range cfg.VerificationPlan.Tiers {
			count += len(tier.Commands)
		}
	}
	return count
}

func FormatOperationalBounds(bounds runonce.OperationalBounds) string {
	actions := make([]string, len(bounds.ActionAttempts))
	for i, bound := range bounds.ActionAttempts {
		actions[i] = fmt.Sprintf("%s=%d", bound.Action, bound.Attempts)
	}
	return fmt.Sprintf("task_attempts=%d action_attempts=[%s] elapsed=%s model_tokens=%d cycles_per_task=%d process_duration=%s output_bytes_per_stream=%d retained_disk_bytes=%d notification_attempts=%d", bounds.TaskAttempts, strings.Join(actions, ","), bounds.Elapsed, bounds.ModelTokens, bounds.CyclesPerTask, bounds.ProcessDuration, bounds.OutputBytesPerStream, bounds.RetainedDiskBytes, bounds.NotificationAttempts)
}
