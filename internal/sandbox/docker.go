package sandbox

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/runner"
)

const (
	dockerManagedLabel = "dev.revolvr.managed"
	dockerOwnerLabel   = "dev.revolvr.owner"
	dockerSpecLabel    = "dev.revolvr.spec-sha256"
)

type DockerRuntime struct {
	Executable        string
	Host              string
	Owner             string
	DependencyNetwork string
	OpenNetwork       string
	run               func(context.Context, runner.Command) runner.Result
}

func NewDockerRuntime(executable, host, owner string) (*DockerRuntime, error) {
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve Docker executable: %v", ErrRuntimeUnavailable, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve Docker executable path: %v", ErrRuntimeUnavailable, err)
	}
	if !strings.HasPrefix(host, "unix://") || !filepath.IsAbs(strings.TrimPrefix(host, "unix://")) {
		return nil, fmt.Errorf("%w: Docker host must be an absolute Unix socket", ErrRuntimeUnavailable)
	}
	if !validDigest("sha256:" + owner) {
		return nil, fmt.Errorf("%w: Docker owner must be a lowercase SHA-256", ErrRuntimeUnavailable)
	}
	return &DockerRuntime{Executable: resolved, Host: host, Owner: owner, run: runner.Run}, nil
}

func (r *DockerRuntime) Create(ctx context.Context, specification Specification) (SandboxHandle, error) {
	if err := CheckSpecification(specification); err != nil {
		return SandboxHandle{}, err
	}
	if err := r.checkAvailability(ctx, specification.RuntimeProfile); err != nil {
		return SandboxHandle{}, err
	}
	if specification.Network == NetworkDependencies && r.DependencyNetwork == "" {
		return SandboxHandle{}, fmt.Errorf("%w: dependency network is not configured", ErrRuntimeUnavailable)
	}
	if specification.Network == NetworkOpen && r.OpenNetwork == "" {
		return SandboxHandle{}, fmt.Errorf("%w: open network is not configured", ErrRuntimeUnavailable)
	}
	identity, err := specification.SHA256()
	if err != nil {
		return SandboxHandle{}, err
	}
	name := "revolvr-" + identity[:32]
	if existing, found, err := r.existing(ctx, name, identity); err != nil {
		return SandboxHandle{}, err
	} else if found {
		return SandboxHandle{ID: existing, Name: name, Command: append([]string(nil), specification.Command...)}, nil
	}
	result := r.command(ctx, 30*time.Second, r.createArguments(specification, name, identity)...)
	if err := dockerCommandError("create", result); err != nil {
		return SandboxHandle{}, errors.Join(err, r.cleanupFailedCreate(name, identity))
	}
	id := strings.TrimSpace(result.Stdout)
	if !validContainerID(id) {
		return SandboxHandle{}, errors.Join(
			fmt.Errorf("create Docker sandbox: malformed container identity %q", id),
			r.cleanupFailedCreate(name, identity),
		)
	}
	return SandboxHandle{ID: id, Name: name, Command: append([]string(nil), specification.Command...)}, nil
}

func (r *DockerRuntime) cleanupFailedCreate(name, identity string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	id, found, err := r.existing(ctx, name, identity)
	if err != nil || !found {
		return err
	}
	handle := SandboxHandle{ID: id}
	return errors.Join(r.Stop(ctx, handle), r.Remove(ctx, handle))
}

func (r *DockerRuntime) Exec(ctx context.Context, handle SandboxHandle, command CommandSpec) (CommandResult, error) {
	if !validContainerID(handle.ID) || !slices.Equal(handle.Command, command.Arguments) {
		return CommandResult{}, errors.New("exec Docker sandbox: handle or command does not match created authority")
	}
	result := r.command(ctx, command.Timeout, "start", "--attach", handle.ID)
	commandResult := CommandResult{
		ExitCode: result.ExitCode, Error: result.Err, TimedOut: result.TimedOut,
		Cancelled: errors.Is(result.Err, context.Canceled), Stdout: result.Stdout, Stderr: result.Stderr,
		StdoutTruncatedBytes: result.StdoutTruncatedBytes, StderrTruncatedBytes: result.StderrTruncatedBytes,
	}
	if result.Err != nil || result.TimedOut || commandResult.Cancelled {
		return commandResult, nil
	}
	status, err := r.Inspect(ctx, handle)
	if err != nil {
		return commandResult, err
	}
	commandResult.ExitCode = status.ExitCode
	return commandResult, nil
}

func (r *DockerRuntime) Stop(ctx context.Context, handle SandboxHandle) error {
	if !validContainerID(handle.ID) {
		return errors.New("stop Docker sandbox: invalid container identity")
	}
	result := r.command(ctx, 10*time.Second, "stop", "--time", "1", handle.ID)
	if dockerNotFound(result) {
		return nil
	}
	return dockerCommandError("stop", result)
}

func (r *DockerRuntime) Inspect(ctx context.Context, handle SandboxHandle) (SandboxStatus, error) {
	if !validContainerID(handle.ID) {
		return SandboxStatus{}, errors.New("inspect Docker sandbox: invalid container identity")
	}
	result := r.command(ctx, 10*time.Second, "inspect", "--format", "{{json .State}}", handle.ID)
	if dockerNotFound(result) {
		return SandboxStatus{}, ErrSandboxNotFound
	}
	if err := dockerCommandError("inspect", result); err != nil {
		return SandboxStatus{}, err
	}
	var state struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &state); err != nil {
		return SandboxStatus{}, fmt.Errorf("inspect Docker sandbox: decode state: %w", err)
	}
	return SandboxStatus{
		State: state.Status, Running: state.Running, ExitCode: state.ExitCode,
		StartedAt: parseDockerTime(state.StartedAt), FinishedAt: parseDockerTime(state.FinishedAt),
	}, nil
}

func (r *DockerRuntime) Remove(ctx context.Context, handle SandboxHandle) error {
	if !validContainerID(handle.ID) {
		return errors.New("remove Docker sandbox: invalid container identity")
	}
	result := r.command(ctx, 10*time.Second, "rm", "--force", handle.ID)
	if dockerNotFound(result) {
		return nil
	}
	return dockerCommandError("remove", result)
}

// Reconcile removes only containers carrying this state directory's exact
// owner label. It never uses a name prefix or an unfiltered engine listing.
func (r *DockerRuntime) Reconcile(ctx context.Context) ([]string, error) {
	if err := r.checkAvailability(ctx, ProfileCompatible); err != nil {
		return nil, err
	}
	result := r.command(ctx, 15*time.Second, "ps", "--all", "--quiet", "--no-trunc",
		"--filter", "label="+dockerManagedLabel+"=true",
		"--filter", "label="+dockerOwnerLabel+"="+r.Owner)
	if err := dockerCommandError("list managed containers", result); err != nil {
		return nil, err
	}
	var removed []string
	for _, id := range strings.Fields(result.Stdout) {
		if !validContainerID(id) {
			return nil, fmt.Errorf("reconcile Docker sandboxes: malformed container identity %q", id)
		}
		handle := SandboxHandle{ID: id}
		if err := r.Stop(ctx, handle); err != nil {
			return removed, err
		}
		if err := r.Remove(ctx, handle); err != nil {
			return removed, err
		}
		removed = append(removed, id)
	}
	return removed, nil
}

func (r *DockerRuntime) checkAvailability(ctx context.Context, profile RuntimeProfile) error {
	result := r.command(ctx, 10*time.Second, "info", "--format", "{{json .SecurityOptions}}\n{{json .Runtimes}}")
	if err := dockerCommandError("inspect runtime", result); err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeUnavailable, err)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("%w: Docker returned malformed runtime facts", ErrRuntimeUnavailable)
	}
	var security []string
	var runtimes map[string]json.RawMessage
	if json.Unmarshal([]byte(lines[0]), &security) != nil || json.Unmarshal([]byte(lines[1]), &runtimes) != nil {
		return fmt.Errorf("%w: Docker returned malformed runtime facts", ErrRuntimeUnavailable)
	}
	rootless := false
	for _, option := range security {
		if option == "name=rootless" || option == "rootless" {
			rootless = true
			break
		}
	}
	if !rootless {
		return fmt.Errorf("%w: Docker daemon is not rootless", ErrRuntimeUnavailable)
	}
	if profile == ProfileStrict {
		if _, ok := runtimes["runsc"]; !ok {
			return fmt.Errorf("%w: strict requires Docker runtime runsc", ErrProfileUnavailable)
		}
	}
	return nil
}

func (r *DockerRuntime) createArguments(specification Specification, name, identity string) []string {
	arguments := []string{
		"create", "--pull=never", "--name", name,
		"--label", dockerManagedLabel + "=true", "--label", dockerOwnerLabel + "=" + r.Owner,
		"--label", dockerSpecLabel + "=" + identity,
		"--user", "65532:65532", "--cap-drop=ALL", "--security-opt=no-new-privileges=true",
		"--read-only", "--pids-limit=" + strconv.FormatInt(specification.Resources.PIDs, 10),
		"--cpus=" + strconv.FormatInt(specification.Resources.CPUs, 10),
		"--memory=" + strconv.FormatInt(specification.Resources.MemoryBytes, 10),
		"--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=" + strconv.FormatInt(specification.Resources.TmpfsBytes, 10),
		"--workdir", "/workspace", "--stop-timeout=1",
	}
	if specification.RuntimeProfile == ProfileStrict {
		arguments = append(arguments, "--runtime=runsc")
	}
	switch specification.Network {
	case NetworkNone:
		arguments = append(arguments, "--network=none")
	case NetworkDependencies:
		arguments = append(arguments, "--network="+r.DependencyNetwork)
	case NetworkOpen:
		arguments = append(arguments, "--network="+r.OpenNetwork)
	}
	for _, variable := range specification.Environment {
		arguments = append(arguments, "--env", variable.Name+"="+variable.Value)
	}
	for _, mount := range specification.Mounts {
		arguments = append(arguments, "--mount", dockerMount(mount))
	}
	arguments = append(arguments, "--", specification.Image.Reference+"@"+specification.Image.Digest)
	return append(arguments, specification.Command...)
}

func (r *DockerRuntime) existing(ctx context.Context, name, identity string) (string, bool, error) {
	result := r.command(ctx, 10*time.Second, "inspect", "--format", "{{.Id}} {{index .Config.Labels \""+dockerOwnerLabel+"\"}} {{index .Config.Labels \""+dockerSpecLabel+"\"}}", name)
	if dockerNotFound(result) {
		return "", false, nil
	}
	if err := dockerCommandError("inspect existing container", result); err != nil {
		return "", false, err
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) != 3 || !validContainerID(fields[0]) || fields[1] != r.Owner || fields[2] != identity {
		return "", false, fmt.Errorf("create Docker sandbox: stable name %q is occupied by foreign authority", name)
	}
	return fields[0], true, nil
}

func (r *DockerRuntime) command(ctx context.Context, timeout time.Duration, arguments ...string) runner.Result {
	return r.run(ctx, runner.Command{
		Name: r.Executable, Args: arguments, Env: []string{"DOCKER_HOST=" + r.Host}, ReplaceEnv: true,
		Timeout: timeout, TerminateGracePeriod: time.Second, KillSettlementPeriod: time.Second,
		StdoutLimit: 1 << 20, StderrLimit: 1 << 20,
	})
}

func dockerMount(mount ResolvedMount) string {
	fields := []string{"type=bind", "src=" + mount.SourcePath, "dst=" + mount.Target}
	if mount.Mode == MountReadOnly {
		fields = append(fields, "readonly")
	}
	var value strings.Builder
	writer := csv.NewWriter(&value)
	_ = writer.Write(fields)
	writer.Flush()
	return strings.TrimSuffix(value.String(), "\n")
}

func dockerCommandError(operation string, result runner.Result) error {
	if result.Err != nil {
		return fmt.Errorf("%s Docker sandbox: %w", operation, result.Err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s Docker sandbox: exit %d: %s", operation, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func dockerNotFound(result runner.Result) bool {
	stderr := strings.ToLower(result.Stderr)
	return result.ExitCode != 0 && (strings.Contains(stderr, "no such container") || strings.Contains(stderr, "no such object"))
}

func validContainerID(value string) bool {
	if len(value) < 12 || len(value) > 64 {
		return false
	}
	_, err := strconv.ParseUint(value[:12], 16, 64)
	if err != nil {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func parseDockerTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
