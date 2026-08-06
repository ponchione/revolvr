package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"revolvr/internal/gitoid"
	"revolvr/internal/model"
	"revolvr/internal/pathguard"
	"revolvr/internal/sandbox"
)

const (
	maximumIdentityBytes = 512
	maximumPathBytes     = 4096
	maximumCallBytes     = 2 << 20
)

type PolicySettings struct {
	Authority                  Authority
	Role                       sandbox.Role
	WorkspaceRoot              string
	WorkspaceDevice            uint64
	WorkspaceInode             uint64
	Sandbox                    sandbox.Specification
	ExpectedPaths              []string
	AdjacentPaths              []string
	ProtectedPaths             []string
	DeniedReadPaths            []string
	DependencyPaths            []string
	VerificationAuthorityPaths []string
	AllowedCommands            [][]string
	AllowedWorkingDirectories  []string
	AllowedEnvironmentNames    []string
	Network                    sandbox.NetworkProfile
	MaximumTimeout             time.Duration
	MaximumCPUs                int64
	MaximumMemoryBytes         int64
	MaximumPIDs                int64
	MaximumTmpfsBytes          int64
	MaximumStdoutBytes         int64
	MaximumStderrBytes         int64
	MaximumReadBytes           int64
	MaximumEditBytes           int64
	MaximumSearchResults       int
	HooksDisabled              bool
	AmbientHostConfiguration   bool
}

type Policy struct {
	Version                    string                 `json:"version"`
	SHA256                     string                 `json:"sha256"`
	Authority                  Authority              `json:"authority"`
	Role                       sandbox.Role           `json:"role"`
	WorkspaceRoot              string                 `json:"workspace_root"`
	WorkspaceDevice            uint64                 `json:"workspace_device"`
	WorkspaceInode             uint64                 `json:"workspace_inode"`
	SandboxSHA256              string                 `json:"sandbox_sha256"`
	ExpectedPaths              []string               `json:"expected_paths"`
	AdjacentPaths              []string               `json:"adjacent_paths"`
	ProtectedPaths             []string               `json:"protected_paths"`
	DeniedReadPaths            []string               `json:"denied_read_paths"`
	DependencyPaths            []string               `json:"dependency_paths"`
	VerificationAuthorityPaths []string               `json:"verification_authority_paths"`
	AllowedCommands            [][]string             `json:"allowed_commands"`
	AllowedWorkingDirectories  []string               `json:"allowed_working_directories"`
	AllowedEnvironmentNames    []string               `json:"allowed_environment_names"`
	Network                    sandbox.NetworkProfile `json:"network"`
	MaximumTimeoutMilliseconds int64                  `json:"maximum_timeout_milliseconds"`
	MaximumCPUs                int64                  `json:"maximum_cpus"`
	MaximumMemoryBytes         int64                  `json:"maximum_memory_bytes"`
	MaximumPIDs                int64                  `json:"maximum_pids"`
	MaximumTmpfsBytes          int64                  `json:"maximum_tmpfs_bytes"`
	MaximumStdoutBytes         int64                  `json:"maximum_stdout_bytes"`
	MaximumStderrBytes         int64                  `json:"maximum_stderr_bytes"`
	MaximumReadBytes           int64                  `json:"maximum_read_bytes"`
	MaximumEditBytes           int64                  `json:"maximum_edit_bytes"`
	MaximumSearchResults       int                    `json:"maximum_search_results"`
	HooksDisabled              bool                   `json:"hooks_disabled"`
	AmbientHostConfiguration   bool                   `json:"ambient_host_configuration"`

	sandbox  sandbox.Specification
	registry Registry
}

type PolicyScope struct {
	Role                       sandbox.Role
	ExpectedPaths              []string
	AdjacentPaths              []string
	ProtectedPaths             []string
	DependencyPaths            []string
	VerificationAuthorityPaths []string
}

func PinPolicy(settings PolicySettings) (Policy, error) {
	registry, err := RegistryForRole(settings.Role)
	if err != nil {
		return Policy{}, err
	}
	if err := sandbox.CheckSpecification(settings.Sandbox); err != nil {
		return Policy{}, fmt.Errorf("tool policy: sandbox specification is not valid: %w", err)
	}
	sandboxSHA, err := settings.Sandbox.SHA256()
	if err != nil {
		return Policy{}, err
	}
	settings.Authority.RegistryVersion = registry.Version
	settings.Authority.RegistrySHA256 = registry.SHA256
	settings.Authority.SandboxSHA256 = sandboxSHA
	settings.Authority.HostPolicyVersion = HostPolicyVersion
	settings.Authority.HostPolicySHA256 = ""
	policy := Policy{
		Version: HostPolicyVersion, Authority: settings.Authority, Role: settings.Role,
		WorkspaceRoot: settings.WorkspaceRoot, WorkspaceDevice: settings.WorkspaceDevice, WorkspaceInode: settings.WorkspaceInode,
		SandboxSHA256: sandboxSHA, ExpectedPaths: cleanList(settings.ExpectedPaths), AdjacentPaths: cleanList(settings.AdjacentPaths),
		ProtectedPaths:  cleanList(append([]string{".git", ".revolvr", ".agent"}, settings.ProtectedPaths...)),
		DeniedReadPaths: cleanList(append([]string{".git", ".revolvr", ".agent", ".env", ".pgpass"}, settings.DeniedReadPaths...)),
		DependencyPaths: cleanList(settings.DependencyPaths), VerificationAuthorityPaths: cleanList(settings.VerificationAuthorityPaths),
		AllowedCommands: cloneCommands(settings.AllowedCommands), AllowedWorkingDirectories: cleanList(settings.AllowedWorkingDirectories),
		AllowedEnvironmentNames: cleanList(settings.AllowedEnvironmentNames), Network: settings.Network,
		MaximumTimeoutMilliseconds: settings.MaximumTimeout.Milliseconds(), MaximumCPUs: settings.MaximumCPUs,
		MaximumMemoryBytes: settings.MaximumMemoryBytes, MaximumPIDs: settings.MaximumPIDs, MaximumTmpfsBytes: settings.MaximumTmpfsBytes,
		MaximumStdoutBytes: settings.MaximumStdoutBytes, MaximumStderrBytes: settings.MaximumStderrBytes,
		MaximumReadBytes: settings.MaximumReadBytes, MaximumEditBytes: settings.MaximumEditBytes,
		MaximumSearchResults: settings.MaximumSearchResults, HooksDisabled: settings.HooksDisabled,
		AmbientHostConfiguration: settings.AmbientHostConfiguration, sandbox: cloneSandbox(settings.Sandbox), registry: registry,
	}
	if len(policy.AllowedWorkingDirectories) == 0 {
		policy.AllowedWorkingDirectories = []string{"/workspace"}
	}
	if err := validatePolicy(policy, false); err != nil {
		return Policy{}, err
	}
	raw, _ := json.Marshal(policyMaterial(policy))
	policy.SHA256 = model.SHA256(raw)
	policy.Authority.HostPolicySHA256 = policy.SHA256
	return policy, nil
}

func validatePolicy(policy Policy, requireHash bool) error {
	if policy.Version != HostPolicyVersion || policy.Role != policy.sandbox.Role || policy.Role != policy.registry.Role {
		return errors.New("tool policy: version or role is stale")
	}
	if !policy.HooksDisabled || policy.AmbientHostConfiguration {
		return errors.New("tool policy: hooks must be disabled and ambient host configuration forbidden")
	}
	if err := validateAuthority(policy.Authority); err != nil {
		return fmt.Errorf("tool policy: %w", err)
	}
	if policy.Authority.RegistryVersion != policy.registry.Version || policy.Authority.RegistrySHA256 != policy.registry.SHA256 || policy.Authority.SandboxSHA256 != policy.SandboxSHA256 {
		return errors.New("tool policy: registry or sandbox authority is stale")
	}
	if err := sandbox.CheckSpecification(policy.sandbox); err != nil {
		return fmt.Errorf("tool policy: sandbox changed after admission: %w", err)
	}
	actualSandboxSHA, err := policy.sandbox.SHA256()
	if err != nil || actualSandboxSHA != policy.SandboxSHA256 {
		return errors.Join(err, errors.New("tool policy: sandbox identity is stale"))
	}
	if policy.sandbox.SandboxID != policy.Authority.SandboxID || policy.sandbox.ProjectID != policy.Authority.ProjectID || policy.sandbox.TaskID != policy.Authority.TaskID || policy.sandbox.RunID != policy.Authority.RunID {
		return errors.New("tool policy: sandbox scheduler identities are stale")
	}
	workspace, ok := workspaceMount(policy.sandbox)
	if !ok || workspace.SourcePath != policy.WorkspaceRoot || workspace.SourceDevice != policy.WorkspaceDevice || workspace.SourceInode != policy.WorkspaceInode {
		return errors.New("tool policy: workspace mount identity is stale")
	}
	if policy.Network == "" || policy.Network != policy.sandbox.Network {
		return errors.New("tool policy: network differs from the admitted sandbox")
	}
	for _, value := range []struct {
		name string
		got  int64
		max  int64
	}{
		{"timeout", policy.MaximumTimeoutMilliseconds, policy.sandbox.Resources.TimeoutSeconds * 1000},
		{"cpus", policy.MaximumCPUs, policy.sandbox.Resources.CPUs}, {"memory", policy.MaximumMemoryBytes, policy.sandbox.Resources.MemoryBytes},
		{"pids", policy.MaximumPIDs, policy.sandbox.Resources.PIDs}, {"tmpfs", policy.MaximumTmpfsBytes, policy.sandbox.Resources.TmpfsBytes},
	} {
		if value.got <= 0 || value.got > value.max {
			return fmt.Errorf("tool policy: %s limit is invalid", value.name)
		}
	}
	if policy.MaximumStdoutBytes <= 0 || policy.MaximumStdoutBytes > 64<<20 ||
		policy.MaximumStderrBytes <= 0 || policy.MaximumStderrBytes > 64<<20 ||
		policy.MaximumReadBytes <= 0 || policy.MaximumReadBytes > 64<<20 ||
		policy.MaximumEditBytes <= 0 || policy.MaximumEditBytes > maximumCallBytes ||
		policy.MaximumSearchResults <= 0 || policy.MaximumSearchResults > 10_000 {
		return errors.New("tool policy: output, read, edit, and search limits must be positive")
	}
	for _, set := range [][]string{policy.ExpectedPaths, policy.AdjacentPaths, policy.ProtectedPaths, policy.DeniedReadPaths, policy.DependencyPaths, policy.VerificationAuthorityPaths} {
		for _, candidate := range set {
			if !cleanRelativePath(candidate) {
				return fmt.Errorf("tool policy: path %q is not canonical", candidate)
			}
		}
	}
	if len(policy.ExpectedPaths) == 0 {
		return errors.New("tool policy: at least one expected source path is required")
	}
	for _, directory := range policy.AllowedWorkingDirectories {
		if directory != "/workspace" && !strings.HasPrefix(directory, "/workspace/") || path.Clean(directory) != directory {
			return fmt.Errorf("tool policy: working directory %q is unsafe", directory)
		}
	}
	for _, name := range policy.AllowedEnvironmentNames {
		if !validEnvironmentName(name) || forbiddenEnvironmentName(name) || !sandboxHasEnvironment(policy.sandbox, name) {
			return fmt.Errorf("tool policy: environment name %q is not in the safe sandbox environment", name)
		}
	}
	for _, variable := range policy.sandbox.Environment {
		if forbiddenEnvironmentName(variable.Name) {
			return fmt.Errorf("tool policy: sandbox environment exposes forbidden authority %q", variable.Name)
		}
	}
	for i, command := range policy.AllowedCommands {
		if len(command) == 0 || len(command) > 256 || i > 0 && slices.Equal(command, policy.AllowedCommands[i-1]) || forbiddenContainerControl(command[0]) {
			return errors.New("tool policy: admitted direct command is empty, duplicated, oversized, or a raw container control")
		}
		for _, argument := range command {
			if argument == "" || len(argument) > 4096 || !utf8.ValidString(argument) || strings.ContainsRune(argument, 0) {
				return errors.New("tool policy: admitted direct command contains malformed argv")
			}
		}
	}
	if requireHash {
		raw, _ := json.Marshal(policyMaterial(policy))
		if policy.SHA256 != model.SHA256(raw) || policy.Authority.HostPolicyVersion != policy.Version || policy.Authority.HostPolicySHA256 != policy.SHA256 {
			return errors.New("tool policy: host policy hash is stale")
		}
	}
	return nil
}

func policyMaterial(policy Policy) Policy {
	policy.SHA256 = ""
	policy.Authority.HostPolicySHA256 = ""
	policy.sandbox = sandbox.Specification{}
	policy.registry = Registry{}
	return policy
}

func validateAuthority(value Authority) error {
	for label, candidate := range map[string]string{
		"project_id": value.ProjectID, "task_id": value.TaskID, "task_version_id": value.TaskVersionID, "run_id": value.RunID,
		"plan_id": value.PlanID, "plan_version_id": value.PlanVersionID, "workspace_id": value.WorkspaceID, "sandbox_id": value.SandboxID,
	} {
		if !identityToken(candidate) {
			return fmt.Errorf("%s is malformed", label)
		}
	}
	if !validSHA(value.SourceRevision) || !gitoid.Valid(value.SourceCommit) || !gitoid.Valid(value.SourceTree) || !validSHA(value.StepBatchSHA256) || value.PlanRevision <= 0 {
		return errors.New("source or plan revision identity is malformed")
	}
	if len(value.StepIDs) == 0 || len(value.StepIDs) > 4 {
		return errors.New("bounded step identity is missing or oversized")
	}
	for i, stepID := range value.StepIDs {
		if !identityToken(stepID) || slices.Contains(value.StepIDs[:i], stepID) {
			return errors.New("bounded step identity is malformed or duplicated")
		}
	}
	return nil
}

func workspaceMount(spec sandbox.Specification) (sandbox.ResolvedMount, bool) {
	for _, mount := range spec.Mounts {
		if mount.Target == "/workspace" && mount.Mode == sandbox.MountReadWrite {
			return mount, true
		}
	}
	return sandbox.ResolvedMount{}, false
}

func (p Policy) AuthorityCopy() Authority { return cloneAuthority(p.Authority) }
func (p Policy) ScopeCopy() PolicyScope {
	return PolicyScope{
		Role: p.Role, ExpectedPaths: append([]string(nil), p.ExpectedPaths...),
		AdjacentPaths: append([]string(nil), p.AdjacentPaths...), ProtectedPaths: append([]string(nil), p.ProtectedPaths...),
		DependencyPaths:            append([]string(nil), p.DependencyPaths...),
		VerificationAuthorityPaths: append([]string(nil), p.VerificationAuthorityPaths...),
	}
}
func (p Policy) RegistryCopy() Registry {
	result := p.registry
	result.Definitions = cloneDefinitions(result.Definitions)
	return result
}
func (p Policy) SandboxCopy() sandbox.Specification { return cloneSandbox(p.sandbox) }

func cloneAuthority(value Authority) Authority {
	value.StepIDs = append([]string(nil), value.StepIDs...)
	return value
}

func cloneSandbox(value sandbox.Specification) sandbox.Specification {
	value.Command = append([]string(nil), value.Command...)
	value.Mounts = append([]sandbox.ResolvedMount(nil), value.Mounts...)
	value.Environment = append([]sandbox.EnvironmentVariable(nil), value.Environment...)
	return value
}

func cleanList(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return slices.Compact(result)
}

func cloneCommands(values [][]string) [][]string {
	result := make([][]string, len(values))
	for i := range values {
		result[i] = append([]string(nil), values[i]...)
	}
	sort.Slice(result, func(i, j int) bool { return strings.Join(result[i], "\x00") < strings.Join(result[j], "\x00") })
	return result
}

func cleanRelativePath(value string) bool {
	return value != "" && len(value) <= maximumPathBytes && utf8.ValidString(value) && !strings.ContainsAny(value, "\\\x00\r\n") &&
		!path.IsAbs(value) && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func identityToken(value string) bool {
	return value != "" && len(value) <= maximumIdentityBytes && value == strings.TrimSpace(value) && !strings.ContainsAny(value, " \t\r\n\x00")
}

func validSHA(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && value == strings.ToLower(value)
}

func pathCovered(candidate string, roots []string) bool {
	for _, root := range roots {
		if candidate == root || strings.HasPrefix(candidate, root+"/") {
			return true
		}
	}
	return false
}

func resolveWorkspacePath(root, relative string, missingFinalOK bool) (string, error) {
	if !cleanRelativePath(relative) {
		return "", errors.New("path is not a normalized workspace-relative path")
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	current := root
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) && missingFinalOK && index == len(parts)-1 {
			return target, nil
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("path contains a symbolic link")
		}
	}
	resolved, err := pathguard.Resolve(root, filepath.FromSlash(relative))
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func validEnvironmentName(name string) bool {
	if name == "" || (name[0] != '_' && (name[0] < 'A' || name[0] > 'Z') && (name[0] < 'a' || name[0] > 'z')) {
		return false
	}
	for _, r := range name[1:] {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func forbiddenEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	for _, fragment := range []string{"OPENAI", "DATABASE", "POSTGRES", "PGPASS", "DOCKER", "CONTAINER", "SSH_AUTH", "GITHUB_TOKEN", "GH_TOKEN", "AWS_", "AZURE_", "GOOGLE_APPLICATION", "XDG_RUNTIME", "HOME"} {
		if strings.Contains(upper, fragment) {
			return true
		}
	}
	return false
}

func secretPath(candidate string) bool {
	for _, component := range strings.Split(candidate, "/") {
		lower := strings.ToLower(component)
		if strings.HasPrefix(lower, ".env") || strings.HasSuffix(lower, ".sock") || slices.Contains([]string{
			".pgpass", ".netrc", ".npmrc", ".pypirc", ".ssh", ".aws", ".azure", ".kube", ".docker", ".gnupg",
			"credentials", "id_rsa", "id_ed25519", "docker.sock", "containerd.sock", "podman.sock",
		}, lower) {
			return true
		}
	}
	return false
}

func forbiddenContainerControl(executable string) bool {
	switch strings.ToLower(path.Base(strings.ReplaceAll(executable, "\\", "/"))) {
	case "docker", "podman", "nerdctl", "ctr", "crictl", "runc", "containerd", "nsenter", "unshare", "mount", "umount", "chroot":
		return true
	default:
		return false
	}
}

func sandboxHasEnvironment(spec sandbox.Specification, name string) bool {
	for _, variable := range spec.Environment {
		if variable.Name == name {
			return true
		}
	}
	return false
}
