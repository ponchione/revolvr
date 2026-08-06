package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/runner"
	"revolvr/internal/runtimepath"
	"revolvr/internal/sandbox"
	"revolvr/internal/workspace"
)

type SandboxGateExecutor struct {
	manager   *sandbox.Manager
	workspace workspace.Workspace
	binding   workspace.SandboxMountBinding
}

func NewSandboxGateExecutor(manager *sandbox.Manager, frozen workspace.Workspace) (*SandboxGateExecutor, error) {
	if manager == nil {
		return nil, errors.New("verification sandbox executor requires a manager")
	}
	if frozen.Status != workspace.StatusFrozen || frozen.CandidateCommit == "" || frozen.CandidateTree == "" {
		return nil, fmt.Errorf("%w: verifier requires a frozen candidate workspace", ErrStaleSource)
	}
	binding, err := frozen.SandboxBinding()
	if err != nil {
		return nil, err
	}
	binding.Mount.Mode = sandbox.MountReadOnly
	return &SandboxGateExecutor{manager: manager, workspace: frozen, binding: binding}, nil
}

func (e *SandboxGateExecutor) Execute(ctx context.Context, execution GateExecution) (ExecutionResult, error) {
	gate := execution.Gate
	if gate.Source.Commit != e.workspace.CandidateCommit || gate.Source.Tree != e.workspace.CandidateTree {
		return ExecutionResult{}, fmt.Errorf("%w: gate source is not the frozen workspace candidate", ErrStaleSource)
	}
	environment := make(map[string]string, len(gate.Environment))
	allowedNames := make([]string, 0, len(gate.Environment))
	for _, variable := range gate.Environment {
		environment[variable.Name] = variable.Value
		allowedNames = append(allowedNames, variable.Name)
	}
	policy := sandbox.Policy{
		ProjectID: execution.Pinned.ProjectID, TaskID: execution.Pinned.TaskID,
		RunID: execution.Pinned.RunID, Role: sandbox.RoleVerifier,
		ApprovedImages: []sandbox.Image{gate.Image}, AllowedProfiles: []sandbox.RuntimeProfile{gate.SandboxProfile},
		AllowedNetworks: []sandbox.NetworkProfile{sandbox.NetworkNone}, AllowedEnvironmentNames: allowedNames,
		ManagedSources: []sandbox.ManagedSource{e.binding.Source}, MaximumResources: gate.Resources,
		ForbiddenHostPaths: e.binding.ForbiddenHostPaths,
	}
	specification, err := sandbox.Validate(sandbox.Request{
		SchemaVersion: sandbox.RequestSchemaVersion, SandboxID: execution.SandboxID,
		ProjectID: execution.Pinned.ProjectID, TaskID: execution.Pinned.TaskID, RunID: execution.Pinned.RunID,
		Role: sandbox.RoleVerifier, Image: gate.Image, RuntimeProfile: gate.SandboxProfile,
		Command: gate.Argv, WorkingDirectory: gate.WorkingDirectory, Mounts: []sandbox.Mount{e.binding.Mount},
		Network: sandbox.NetworkNone, Resources: gate.Resources, Environment: environment,
	}, policy)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("build verifier sandbox: %w", err)
	}
	specificationSHA, err := specification.SHA256()
	if err != nil {
		return ExecutionResult{}, err
	}
	started := time.Now().UTC()
	evidence, runErr := e.manager.Run(ctx, specification)
	completed := time.Now().UTC()
	evidenceRaw, marshalErr := json.Marshal(evidence)
	if marshalErr != nil {
		return ExecutionResult{}, errors.Join(runErr, marshalErr)
	}
	result := ExecutionResult{
		SandboxSpecificationSHA256: specificationSHA, ExitCode: evidence.ExitCode,
		TimedOut: evidence.TimedOut, Cancelled: evidence.Cancelled, MissingCommand: evidence.ExitCode == 127,
		StdoutTruncatedBytes: evidence.Stdout.TruncatedBytes, StderrTruncatedBytes: evidence.Stderr.TruncatedBytes,
		Evidence: evidenceRaw, StartedAt: started, CompletedAt: completed,
	}
	if len(evidence.Transitions) > 0 {
		result.StartedAt = evidence.Transitions[0].At
		result.CompletedAt = evidence.Transitions[len(evidence.Transitions)-1].At
	}
	stdout, stdoutErr := e.manager.ReadArtifact(evidence.Stdout, MaximumCapturedStreamBytes)
	stderr, stderrErr := e.manager.ReadArtifact(evidence.Stderr, MaximumCapturedStreamBytes)
	if stdoutErr != nil || stderrErr != nil {
		return result, fmt.Errorf("%w: read sandbox output: %v", ErrArtifact, errors.Join(stdoutErr, stderrErr))
	}
	result.Stdout = stdout
	result.Stderr = stderr
	return result, runErr
}

type FrozenWorkspaceObserver struct {
	workspace      workspace.Workspace
	root           runtimepath.Boundary
	gitExecutable  string
	environmentSHA string
	run            func(context.Context, runner.Command) runner.Result
}

func NewFrozenWorkspaceObserver(frozen workspace.Workspace, environment ProjectEnvironment, gitExecutable string) (*FrozenWorkspaceObserver, error) {
	if frozen.Status != workspace.StatusFrozen || frozen.CandidateCommit == "" || frozen.CandidateTree == "" {
		return nil, fmt.Errorf("%w: observer requires a frozen candidate workspace", ErrStaleSource)
	}
	boundary, err := runtimepath.Bind(frozen.Path)
	if err != nil {
		return nil, fmt.Errorf("bind verification workspace: %w", err)
	}
	resolved, err := exec.LookPath(gitExecutable)
	if err != nil {
		return nil, fmt.Errorf("resolve verification git executable: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}
	return &FrozenWorkspaceObserver{
		workspace: frozen, root: boundary, gitExecutable: resolved,
		environmentSHA: environment.SHA256, run: runner.Run,
	}, nil
}

func (o *FrozenWorkspaceObserver) Observe(ctx context.Context, gate Gate) (AuthoritySnapshot, error) {
	result := o.run(ctx, runner.Command{
		Name: o.gitExecutable, Args: []string{"-C", o.workspace.Path, "status", "--porcelain=v1", "--untracked-files=all"},
		Env: []string{"LC_ALL=C"}, ReplaceEnv: true, Timeout: 15 * time.Second,
		StdoutLimit: int(MaximumCapturedStreamBytes), StderrLimit: 64 << 10,
	})
	if result.Err != nil || result.ExitCode != 0 || result.TimedOut || strings.TrimSpace(result.Stdout) != "" {
		return AuthoritySnapshot{}, fmt.Errorf("%w: candidate workspace is not clean: %v %s", ErrStaleSource, result.Err, strings.TrimSpace(result.Stderr))
	}
	result = o.run(ctx, runner.Command{
		Name: o.gitExecutable, Args: []string{"-C", o.workspace.Path, "rev-parse", "HEAD", "HEAD^{tree}"},
		Env: []string{"LC_ALL=C"}, ReplaceEnv: true, Timeout: 15 * time.Second,
		StdoutLimit: 1024, StderrLimit: 64 << 10,
	})
	fields := strings.Fields(result.Stdout)
	if result.Err != nil || result.ExitCode != 0 || result.TimedOut || len(fields) != 2 {
		return AuthoritySnapshot{}, fmt.Errorf("%w: observe candidate identity: %v %s", ErrStaleSource, result.Err, strings.TrimSpace(result.Stderr))
	}
	inputs := make([]MaterialInput, 0, len(gate.AuthorityInputs))
	for _, input := range gate.AuthorityInputs {
		filePath := filepath.Join(o.root.Root(), filepath.FromSlash(input.Path))
		raw, found, err := o.root.ReadFileLimit(filePath, false, input.SizeBytes+1)
		if err != nil || !found {
			return AuthoritySnapshot{}, fmt.Errorf("%w: read %s: %v", ErrAuthorityChanged, input.Path, errors.Join(err, errors.New("authority file missing")))
		}
		inputs = append(inputs, MaterialInput{Kind: input.Kind, Path: input.Path, SHA256: hashBytes(raw), SizeBytes: int64(len(raw))})
	}
	return AuthoritySnapshot{
		Source:                   SourceIdentity{Commit: fields[0], Tree: fields[1]},
		ProjectEnvironmentSHA256: o.environmentSHA, AuthorityInputs: inputs,
	}, nil
}
