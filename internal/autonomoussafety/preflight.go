package autonomoussafety

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/redact"
	"revolvr/internal/runner"
)

type CommandSpec struct {
	Kind        string
	Executable  string
	Args        []string
	WorkingDir  string
	Environment []string
	Timeout     time.Duration
	StdoutCap   int
	StderrCap   int
}

type Input struct {
	TaskID         string
	Workspace      autonomous.TaskWorkspace
	SourceRevision string
	ObservedHead   string
	Declaration    Declaration
	Codex          CodexPolicy
	Commands       []CommandSpec
	ConfigPath     string
	ConfigSHA256   string
	ObservedAt     time.Time
	LookupEnv      redact.LookupEnv
	LookPath       func(string) (string, error)
	CommandRunner  func(context.Context, runner.Command) runner.Result
	GitExecutable  string
	GitTimeout     time.Duration
	GitStdoutCap   int
	GitStderrCap   int
}

type Output struct {
	Policy    Policy
	Preflight PreflightResult
	Redactor  *redact.Redactor
	Redaction redact.Facts
}

func Run(ctx context.Context, in Input) (Output, error) {
	result := PreflightResult{SchemaVersion: PreflightSchemaVersion, TaskID: in.TaskID, WorkspaceID: in.Workspace.WorkspaceID, SourceRevision: in.SourceRevision, ConfigSHA256: in.ConfigSHA256, ObservedAt: in.ObservedAt, Ready: true}
	add := func(name string, err error, detail string) {
		status := CheckOK
		if err != nil {
			status = CheckFail
			result.Ready = false
			detail = err.Error()
		}
		result.Checks = append(result.Checks, Check{Name: name, Status: status, Detail: strings.TrimSpace(detail)})
	}

	declarationErr := in.Declaration.Validate()
	add("autonomy_mode", declarationErr, fmt.Sprintf("mode=%s", in.Declaration.Mode))
	if declarationErr != nil {
		return Output{Preflight: result}, nil
	}
	if in.ObservedAt.IsZero() {
		add("policy_schema", errors.New("preflight observation time is required"), "")
		return Output{Preflight: result}, nil
	}
	if !validSHA256(in.ConfigSHA256) {
		add("policy_schema", errors.New("effective config SHA-256 is invalid"), "")
		return Output{Preflight: result}, nil
	}
	if in.Workspace.TaskID != in.TaskID || in.SourceRevision != in.Workspace.SourceRevision || in.ObservedHead != in.Workspace.HeadSHA {
		add("workspace_identity", errors.New("task/workspace/source identity mismatch"), "")
		return Output{Preflight: result}, nil
	}
	if err := in.Workspace.Validate(); err != nil {
		add("workspace_identity", err, "")
		return Output{Preflight: result}, nil
	}
	if in.Workspace.Status != autonomous.WorkspaceStatusReady && in.Workspace.Status != autonomous.WorkspaceStatusRestored {
		add("workspace_identity", fmt.Errorf("workspace status %q is not admitted", in.Workspace.Status), "")
		return Output{Preflight: result}, nil
	}
	add("workspace_identity", nil, fmt.Sprintf("workspace=%s control=%s execution=%s; Git worktree isolation is not a security sandbox", in.Workspace.WorkspaceID, in.Workspace.ControlRoot, in.Workspace.ExecutionRoot))

	roots, protected, rootsErr := derivePaths(in.Workspace)
	add("writable_roots_and_protected_paths", rootsErr, fmt.Sprintf("writable_roots=%d protected_paths=%d", len(roots), len(protected)))

	redactor, redactionFacts, redactionErr := redact.New(in.Declaration.Redaction, in.LookupEnv)
	add("secret_redaction", redactionErr, fmt.Sprintf("policy=%s sources=%d", redactionFacts.PolicySHA256, redactionFacts.SourceCount))
	redactionHash, _ := in.Declaration.Redaction.Identity()
	add("environment_policy", validateEnvironment(in.Declaration.Environment, in.LookupEnv), fmt.Sprintf("inherit_host=%t allowed_names=%d", in.Declaration.Environment.InheritHost, len(in.Declaration.Environment.Allow)))

	commands, commandErr := resolveCommands(in.Commands, in.LookPath)
	add("command_provenance", commandErr, fmt.Sprintf("commands=%d", len(commands)))

	hooks, hooksErr := inspectHooks(ctx, in)
	add("git_hooks", hooksErr, hooks)

	policy := Policy{
		SchemaVersion: PolicySchemaVersion, TaskID: in.TaskID, Workspace: in.Workspace,
		Mode: in.Declaration.Mode, Codex: in.Codex, ExternalIsolation: in.Declaration.ExternalIsolation,
		Network: in.Declaration.Network, Hooks: in.Declaration.Hooks, Environment: in.Declaration.Environment,
		Redaction: in.Declaration.Redaction, RedactionPolicyHash: redactionHash, WritableRoots: roots,
		ProtectedPaths: protected, Commands: commands, ConfigPath: in.ConfigPath, ConfigSHA256: in.ConfigSHA256,
		Acknowledgement: in.Declaration.Acknowledgement,
		WorktreeNotice:  "Git worktree isolation is source/Git isolation, not a security sandbox.",
	}
	finalized, identityErr := FinalizePolicy(policy)
	policy = finalized
	identity := policy.PolicySHA256
	result.PolicySHA256 = identity
	add("policy_schema", identityErr, fmt.Sprintf("schema=%s sha256=%s", PolicySchemaVersion, identity))

	ackErr := validateModeAuthority(policy)
	add("acknowledgement", ackErr, acknowledgementDetail(policy))
	add("codex_permissions", validateCodexPolicy(policy), fmt.Sprintf("sandbox=%s approval=%s dangerous_bypass=%t", policy.Codex.Sandbox, policy.Codex.ApprovalPolicy, policy.Codex.DangerousBypass))
	add("external_isolation", validateExternalForMode(policy), fmt.Sprintf("expectation=%s enforcement=%s", policy.ExternalIsolation.Expectation, policy.ExternalIsolation.Enforcement))
	add("network_policy", validateNetworkForMode(policy), fmt.Sprintf("access=%s enforcement=%s", policy.Network.Access, policy.Network.Enforcement))

	if rootsErr != nil || redactionErr != nil || commandErr != nil || hooksErr != nil || identityErr != nil {
		result.Ready = false
	}
	if result.Ready {
		validationCopy := policy
		validationCopy.Acknowledgement = policy.Acknowledgement
		// Validate hashes against the policy identity material that deliberately
		// excludes the acknowledgement token itself.
		if policy.SchemaVersion == "" {
			result.Ready = false
		}
	}
	return Output{Policy: policy, Preflight: result, Redactor: redactor, Redaction: redactionFacts}, nil
}

func validateEnvironment(policy EnvironmentPolicy, lookup redact.LookupEnv) error {
	if policy.InheritHost {
		return nil
	}
	if lookup == nil {
		lookup = os.LookupEnv
	}
	for _, name := range policy.Allow {
		if _, ok := lookup(name); !ok {
			return fmt.Errorf("allowed environment variable %q is unavailable", name)
		}
	}
	return nil
}

func validateModeAuthority(policy Policy) error {
	if policy.Mode == ModeOperatorAttended {
		return nil
	}
	if policy.Environment.InheritHost {
		return errors.New("fully unattended mode must disable ambient host environment inheritance")
	}
	want := policy.ExpectedAcknowledgement()
	if policy.Acknowledgement != want {
		return fmt.Errorf("fully unattended mode requires exact acknowledgement %q", want)
	}
	return nil
}

func acknowledgementDetail(policy Policy) string {
	if policy.Mode == ModeOperatorAttended {
		return "not required for operator-attended execution"
	}
	return "exact policy-bound acknowledgement accepted"
}

func validateCodexPolicy(policy Policy) error {
	if strings.TrimSpace(policy.Codex.Model) == "" || strings.TrimSpace(policy.Codex.ReasoningEffort) == "" || !policy.Codex.Ephemeral {
		return errors.New("explicit model, reasoning effort, and ephemeral session are required")
	}
	if strings.TrimSpace(policy.Codex.Sandbox) == "" || strings.TrimSpace(policy.Codex.ApprovalPolicy) == "" {
		return errors.New("explicit Codex sandbox and approval policy are required")
	}
	if policy.Codex.DangerousBypass && policy.Mode == ModeFullyUnattended && policy.ExternalIsolation.Enforcement != EnforcementExternalAttestation {
		return errors.New("dangerous bypass in fully unattended mode requires externally attested isolation")
	}
	return nil
}

func validateExternalForMode(policy Policy) error {
	if policy.Mode == ModeOperatorAttended {
		return nil
	}
	if policy.ExternalIsolation.Expectation == IsolationNone || policy.ExternalIsolation.Enforcement != EnforcementExternalAttestation || policy.ExternalIsolation.Attestation == nil {
		return errors.New("fully unattended mode requires an externally attested container or OS sandbox")
	}
	return nil
}

func validateNetworkForMode(policy Policy) error {
	if policy.Mode == ModeOperatorAttended {
		return nil
	}
	if policy.Network.Access == NetworkUnknown || policy.Network.Enforcement != EnforcementExternalAttestation || policy.Network.Attestation == nil {
		return errors.New("fully unattended mode requires an explicit externally attested network policy")
	}
	return nil
}

func derivePaths(workspace autonomous.TaskWorkspace) ([]WritableRoot, []ProtectedPath, error) {
	control := workspace.ControlRoot
	taskRuntime := filepath.Join(control, ".revolvr", "autonomous", "tasks", workspace.TaskID)
	roots := []WritableRoot{
		{Path: filepath.Join(control, ".revolvr", "runs"), Purpose: "run_artifacts", Actor: "harness"},
		{Path: filepath.Join(control, ".revolvr", "receipts"), Purpose: "receipts", Actor: "harness"},
		{Path: filepath.Join(control, ".revolvr", "locks"), Purpose: "locks", Actor: "harness"},
		{Path: taskRuntime, Purpose: "autonomous_state_and_history", Actor: "harness"},
		{Path: workspace.ExecutionRoot, Purpose: "authorized_task_source", Actor: "model"},
	}
	for _, root := range roots {
		if err := canonicalExistingDirectory(root.Path); err != nil {
			return nil, nil, fmt.Errorf("writable root %s: %w", root.Purpose, err)
		}
	}
	for i := range roots {
		for j := i + 1; j < len(roots); j++ {
			if roots[i].Path == roots[j].Path || sameOrWithin(roots[i].Path, roots[j].Path) || sameOrWithin(roots[j].Path, roots[i].Path) {
				return nil, nil, fmt.Errorf("writable roots %q and %q overlap", roots[i].Path, roots[j].Path)
			}
		}
	}
	protected := []ProtectedPath{
		{Path: workspace.GitCommonDir, Class: "git_common_directory"},
		{Path: filepath.Join(workspace.ExecutionRoot, ".git"), Class: "worktree_git_administration"},
		{Path: workspace.OwnerMarker, Class: "workspace_ownership"},
		{Path: filepath.Join(control, ".revolvr", "locks"), Class: "harness_locks"},
		{Path: filepath.Join(taskRuntime, "state.json"), Class: "autonomous_state"},
		{Path: filepath.Join(taskRuntime, "history"), Class: "autonomous_history"},
		{Path: filepath.Join(control, ".revolvr", "ledger.sqlite"), Class: "ledger"},
		{Path: filepath.Join(workspace.ExecutionRoot, ".agent", "tasks"), Class: "task_specifications"},
		{Path: filepath.Join(workspace.ExecutionRoot, ".agent", "profiles"), Class: "role_profiles"},
		{Path: filepath.Join(workspace.ExecutionRoot, "AGENTS.md"), Class: "repository_guidance"},
		{Path: filepath.Join(control, ".revolvr", "config.yaml"), Class: "safety_configuration"},
	}
	sort.Slice(protected, func(i, j int) bool {
		if protected[i].Class == protected[j].Class {
			return protected[i].Path < protected[j].Path
		}
		return protected[i].Class < protected[j].Class
	})
	return roots, protected, nil
}

func canonicalExistingDirectory(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("path is not canonical and absolute")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if resolved != path {
		return errors.New("path contains a symlink or alternate spelling")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}

func resolveCommands(specs []CommandSpec, lookPath func(string) (string, error)) ([]CommandProvenance, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if len(specs) == 0 {
		return nil, errors.New("at least one configured command is required")
	}
	result := make([]CommandProvenance, 0, len(specs))
	for i, spec := range specs {
		if strings.TrimSpace(spec.Kind) == "" || strings.TrimSpace(spec.Executable) == "" || strings.ContainsAny(spec.Kind+spec.Executable, "\x00\r\n") {
			return nil, fmt.Errorf("command[%d] kind/executable is malformed", i)
		}
		if err := validateOrderedStrings("command environment", spec.Environment); err != nil {
			return nil, err
		}
		for _, arg := range spec.Args {
			if strings.ContainsAny(arg, "\x00\r\n") {
				return nil, fmt.Errorf("command[%d] argv contains control ambiguity", i)
			}
		}
		resolved, err := lookPath(spec.Executable)
		if err != nil {
			return nil, fmt.Errorf("resolve command[%d] %q: %w", i, spec.Executable, err)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return nil, err
		}
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return nil, fmt.Errorf("resolved command %q is not an executable regular file", resolved)
		}
		hash, err := hashFile(resolved)
		if err != nil {
			return nil, err
		}
		workDir := filepath.Clean(spec.WorkingDir)
		if !filepath.IsAbs(workDir) {
			return nil, fmt.Errorf("command[%d] working directory is not absolute", i)
		}
		if spec.Timeout <= 0 || spec.StdoutCap <= 0 || spec.StderrCap <= 0 {
			return nil, fmt.Errorf("command[%d] requires positive timeout and output caps", i)
		}
		result = append(result, CommandProvenance{Kind: spec.Kind, Configured: spec.Executable, Resolved: resolved, ExecutableSHA256: hash, Argv: append([]string(nil), spec.Args...), WorkingDir: workDir, Environment: append([]string(nil), spec.Environment...), Timeout: spec.Timeout, StdoutCap: spec.StdoutCap, StderrCap: spec.StderrCap})
	}
	return result, nil
}

func inspectHooks(ctx context.Context, in Input) (string, error) {
	if in.CommandRunner == nil {
		in.CommandRunner = runner.Run
	}
	res := in.CommandRunner(ctx, runner.Command{Name: in.GitExecutable, Args: []string{"config", "--path", "--get", "core.hooksPath"}, Dir: in.Workspace.ExecutionRoot, Timeout: in.GitTimeout, StdoutLimit: in.GitStdoutCap, StderrLimit: in.GitStderrCap})
	if res.StdoutTruncatedBytes > 0 || res.StderrTruncatedBytes > 0 {
		return "", errors.New("Git hook path evidence was truncated")
	}
	hooksPath := ""
	if res.Err == nil && !res.TimedOut && res.ExitCode == 0 {
		hooksPath = strings.TrimSpace(res.Stdout)
	} else if res.Err == nil && !res.TimedOut && res.ExitCode == 1 {
		hooksPath = filepath.Join(in.Workspace.GitCommonDir, "hooks")
	} else {
		return "", fmt.Errorf("resolve core.hooksPath: %s", commandFailure(res))
	}
	if hooksPath == "" {
		return "", errors.New("core.hooksPath returned an empty path")
	}
	if !filepath.IsAbs(hooksPath) {
		hooksPath = filepath.Join(in.Workspace.ExecutionRoot, hooksPath)
	}
	hooksPath = filepath.Clean(hooksPath)
	if in.Declaration.Mode == ModeFullyUnattended && !sameOrWithin(in.Workspace.GitCommonDir, hooksPath) && !sameOrWithin(in.Workspace.ExecutionRoot, hooksPath) {
		return "", fmt.Errorf("fully unattended hooks path %s is outside admitted Git/workspace roots", hooksPath)
	}
	if info, statErr := os.Lstat(hooksPath); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("hooks path is symlinked")
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return "", statErr
	}
	entries, err := os.ReadDir(hooksPath)
	if errors.Is(err, fs.ErrNotExist) {
		entries = nil
		err = nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect hooks path %s: %w", hooksPath, err)
	}
	if len(entries) > 0 {
		resolved, resolveErr := filepath.EvalSymlinks(hooksPath)
		if resolveErr != nil || resolved != hooksPath {
			return "", errors.New("hooks path is symlinked or noncanonical")
		}
	}
	executable := make([]TrustedHook, 0)
	for _, entry := range entries {
		path := filepath.Join(hooksPath, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("hook %s is a symlink", path)
		}
		if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			continue
		}
		hash, err := hashFile(path)
		if err != nil {
			return "", err
		}
		executable = append(executable, TrustedHook{Path: path, SHA256: hash})
	}
	sort.Slice(executable, func(i, j int) bool { return executable[i].Path < executable[j].Path })
	switch in.Declaration.Hooks.Policy {
	case HooksDisabled:
		if len(executable) > 0 {
			return "", fmt.Errorf("hooks policy disabled but %d executable hook(s) exist at %s", len(executable), hooksPath)
		}
	case HooksTrusted:
		if len(executable) != len(in.Declaration.Hooks.Trusted) {
			return "", errors.New("observed executable hooks do not match trusted hook identities")
		}
		for i := range executable {
			if executable[i] != in.Declaration.Hooks.Trusted[i] {
				return "", fmt.Errorf("hook identity drift at %s", executable[i].Path)
			}
		}
	case HooksOperatorAttended:
		if in.Declaration.Mode == ModeFullyUnattended {
			return "", errors.New("fully unattended mode cannot use operator-attended hook trust")
		}
	}
	return fmt.Sprintf("path=%s executable_hooks=%d policy=%s", hooksPath, len(executable), in.Declaration.Hooks.Policy), nil
}

func hashFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func commandFailure(result runner.Result) string {
	if result.TimedOut {
		return "timed out"
	}
	if result.Err != nil {
		return result.Err.Error()
	}
	return fmt.Sprintf("exit %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
}
