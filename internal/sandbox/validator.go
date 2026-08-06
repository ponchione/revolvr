// Package sandbox validates typed requests before an OCI runtime sees them.
package sandbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"revolvr/internal/pathguard"
	"revolvr/internal/runtimepath"
)

const (
	RequestSchemaVersion = "revolvr-sandbox-request-v1"
	maxRequestBytes      = 1 << 20
	maxIdentityBytes     = 256
	maxImageBytes        = 512
	maxMounts            = 64
	maxCommandArguments  = 256
	maxArgumentBytes     = 4096
	maxCommandBytes      = 64 << 10
	maxEnvironment       = 128
	maxEnvironmentValue  = 16 << 10
	maxEnvironmentBytes  = 64 << 10
)

var (
	ErrInvalidRequest = errors.New("invalid sandbox request")
	environmentName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type Role string

const (
	RoleImplementer Role = "implementer"
	RoleCorrector   Role = "corrector"
	RoleVerifier    Role = "verifier"
)

type RuntimeProfile string

const (
	ProfileStrict     RuntimeProfile = "strict"
	ProfileCompatible RuntimeProfile = "compatible"
	ProfileDiagnostic RuntimeProfile = "diagnostic"
)

type NetworkProfile string

const (
	NetworkNone         NetworkProfile = "none"
	NetworkDependencies NetworkProfile = "dependencies"
	NetworkOpen         NetworkProfile = "open"
)

type MountMode string

const (
	MountReadOnly  MountMode = "ro"
	MountReadWrite MountMode = "rw"
)

type SourceKind string

const (
	SourceWorkspace SourceKind = "workspace"
	SourceContext   SourceKind = "context"
	SourceCache     SourceKind = "cache"
)

type SourceType string

const (
	SourceDirectory SourceType = "directory"
	SourceFile      SourceType = "file"
)

type Image struct {
	Reference string `json:"reference"`
	Digest    string `json:"digest"`
}

type Mount struct {
	SourceID string    `json:"source_id"`
	Target   string    `json:"target"`
	Mode     MountMode `json:"mode"`
}

type Resources struct {
	CPUs           int64 `json:"cpus"`
	MemoryBytes    int64 `json:"memory_bytes"`
	PIDs           int64 `json:"pids"`
	TimeoutSeconds int64 `json:"timeout_seconds"`
	TmpfsBytes     int64 `json:"tmpfs_bytes"`
}

type Request struct {
	SchemaVersion    string            `json:"schema_version"`
	SandboxID        string            `json:"sandbox_id"`
	ProjectID        string            `json:"project_id"`
	TaskID           string            `json:"task_id"`
	RunID            string            `json:"run_id"`
	Role             Role              `json:"role"`
	Image            Image             `json:"image"`
	RuntimeProfile   RuntimeProfile    `json:"runtime_profile"`
	Command          []string          `json:"command"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Mounts           []Mount           `json:"mounts"`
	Network          NetworkProfile    `json:"network"`
	Resources        Resources         `json:"resources"`
	Environment      map[string]string `json:"environment"`
}

// ManagedSource is trusted host policy. Requests name SourceID only; they
// never provide Root, RelativePath, or another host path.
type ManagedSource struct {
	ID           string
	Root         string
	RelativePath string
	Kind         SourceKind
	Type         SourceType
	Target       string
}

// Policy pins a sandbox request to one admitted scheduler run and the finite
// host authority available to it.
type Policy struct {
	ProjectID               string
	TaskID                  string
	RunID                   string
	Role                    Role
	Attended                bool
	ApprovedImages          []Image
	AllowedProfiles         []RuntimeProfile
	AllowedNetworks         []NetworkProfile
	AllowedEnvironmentNames []string
	ManagedSources          []ManagedSource
	MaximumResources        Resources
	ForbiddenHostPaths      []string
}

type EnvironmentVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ResolvedMount struct {
	SourceID     string     `json:"source_id"`
	ManagedRoot  string     `json:"managed_root"`
	SourcePath   string     `json:"source_path"`
	SourceType   SourceType `json:"source_type"`
	SourceDevice uint64     `json:"source_device"`
	SourceInode  uint64     `json:"source_inode"`
	Target       string     `json:"target"`
	Mode         MountMode  `json:"mode"`
}

// Specification contains only normalized, runtime-effective values. Slices
// and maps from the request and policy are not retained.
type Specification struct {
	SchemaVersion    string                `json:"schema_version"`
	SandboxID        string                `json:"sandbox_id"`
	ProjectID        string                `json:"project_id"`
	TaskID           string                `json:"task_id"`
	RunID            string                `json:"run_id"`
	Role             Role                  `json:"role"`
	Image            Image                 `json:"image"`
	RuntimeProfile   RuntimeProfile        `json:"runtime_profile"`
	Command          []string              `json:"command"`
	WorkingDirectory string                `json:"working_directory"`
	Mounts           []ResolvedMount       `json:"mounts"`
	Network          NetworkProfile        `json:"network"`
	Resources        Resources             `json:"resources"`
	Environment      []EnvironmentVariable `json:"environment"`
}

// SHA256 hashes the compact canonical JSON form used as later runtime
// identity and evidence input.
func (s Specification) SHA256() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("sandbox specification identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Decode rejects duplicate and unknown fields before validating the request.
func Decode(raw []byte, policy Policy) (Specification, error) {
	if len(raw) == 0 {
		return Specification{}, invalidf("input is empty")
	}
	if len(raw) > maxRequestBytes {
		return Specification{}, invalidf("input exceeds %d bytes", maxRequestBytes)
	}
	if err := rejectDuplicateFields(raw); err != nil {
		return Specification{}, invalidf("decode JSON: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Specification{}, invalidf("decode JSON: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Specification{}, invalidf("multiple JSON values are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return Specification{}, invalidf("trailing JSON: %v", err)
	}
	return Validate(request, policy)
}

// Validate resolves one typed request without starting a process or changing
// host state.
func Validate(request Request, policy Policy) (Specification, error) {
	normalizedPolicy, err := normalizePolicy(policy)
	if err != nil {
		return Specification{}, err
	}
	if request.SchemaVersion != RequestSchemaVersion {
		return Specification{}, invalidf("unsupported schema_version %q", request.SchemaVersion)
	}
	for _, value := range []struct {
		name string
		got  string
		want string
	}{
		{"project_id", request.ProjectID, normalizedPolicy.projectID},
		{"task_id", request.TaskID, normalizedPolicy.taskID},
		{"run_id", request.RunID, normalizedPolicy.runID},
	} {
		if err := validateText(value.name, value.got, maxIdentityBytes); err != nil {
			return Specification{}, err
		}
		if value.got != value.want {
			return Specification{}, invalidf("%s does not match admitted authority", value.name)
		}
	}
	if err := validateText("sandbox_id", request.SandboxID, maxIdentityBytes); err != nil {
		return Specification{}, err
	}
	if request.Role != normalizedPolicy.role {
		return Specification{}, invalidf("role %q does not match admitted authority", request.Role)
	}
	if !normalizedPolicy.images[imageKey(request.Image)] {
		return Specification{}, invalidf("image %q with digest %q is not approved", request.Image.Reference, request.Image.Digest)
	}
	if !normalizedPolicy.profiles[request.RuntimeProfile] {
		return Specification{}, invalidf("runtime_profile %q is not approved", request.RuntimeProfile)
	}
	if request.RuntimeProfile == ProfileDiagnostic && !policy.Attended {
		return Specification{}, invalidf("diagnostic runtime_profile requires attended execution")
	}
	network := request.Network
	if network == "" {
		network = NetworkNone
	}
	if !normalizedPolicy.networks[network] {
		return Specification{}, invalidf("network profile %q is not approved", network)
	}
	if network == NetworkOpen && (!policy.Attended || request.RuntimeProfile != ProfileDiagnostic) {
		return Specification{}, invalidf("open network requires attended diagnostic execution")
	}
	if request.Role == RoleVerifier && network != NetworkNone {
		return Specification{}, invalidf("verifier network must be none")
	}
	command, err := validateCommand(request.Command)
	if err != nil {
		return Specification{}, err
	}
	workingDirectory, err := validateWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return Specification{}, err
	}
	if err := validateResources(request.Resources, normalizedPolicy.maximumResources); err != nil {
		return Specification{}, err
	}
	environment, err := validateEnvironment(request.Environment, normalizedPolicy.environment)
	if err != nil {
		return Specification{}, err
	}
	for _, variable := range environment {
		switch variable.Name {
		case "TASK_ID":
			if variable.Value != request.TaskID {
				return Specification{}, invalidf("environment TASK_ID does not match task_id")
			}
		case "RUN_ID":
			if variable.Value != request.RunID {
				return Specification{}, invalidf("environment RUN_ID does not match run_id")
			}
		case "ROLE":
			if variable.Value != string(request.Role) {
				return Specification{}, invalidf("environment ROLE does not match role")
			}
		}
	}
	mounts, err := validateMounts(request.Mounts, normalizedPolicy.sources, policy.ForbiddenHostPaths, request.Role)
	if err != nil {
		return Specification{}, err
	}
	return Specification{
		SchemaVersion: RequestSchemaVersion, SandboxID: request.SandboxID,
		ProjectID: request.ProjectID, TaskID: request.TaskID, RunID: request.RunID,
		Role: request.Role, Image: request.Image, RuntimeProfile: request.RuntimeProfile,
		Command: command, WorkingDirectory: workingDirectory,
		Mounts: mounts, Network: network, Resources: request.Resources,
		Environment: environment,
	}, nil
}

func validateWorkingDirectory(value string) (string, error) {
	if value == "" {
		value = "/workspace"
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > 4096 {
		return "", invalidf("working_directory is invalid")
	}
	cleaned := path.Clean(value)
	if cleaned != value || (cleaned != "/workspace" && !strings.HasPrefix(cleaned, "/workspace/")) {
		return "", invalidf("working_directory must be /workspace or a canonical descendant")
	}
	return cleaned, nil
}

type normalizedPolicy struct {
	projectID        string
	taskID           string
	runID            string
	role             Role
	images           map[string]bool
	profiles         map[RuntimeProfile]bool
	networks         map[NetworkProfile]bool
	environment      map[string]bool
	sources          map[string]ManagedSource
	maximumResources Resources
}

func normalizePolicy(policy Policy) (normalizedPolicy, error) {
	for _, identity := range []struct {
		name  string
		value string
	}{{"project_id", policy.ProjectID}, {"task_id", policy.TaskID}, {"run_id", policy.RunID}} {
		if err := validatePolicyText(identity.name, identity.value, maxIdentityBytes); err != nil {
			return normalizedPolicy{}, err
		}
	}
	if !validRole(policy.Role) {
		return normalizedPolicy{}, invalidf("policy role %q is unsafe", policy.Role)
	}
	result := normalizedPolicy{
		projectID: policy.ProjectID, taskID: policy.TaskID, runID: policy.RunID, role: policy.Role,
		images: map[string]bool{}, profiles: map[RuntimeProfile]bool{},
		networks: map[NetworkProfile]bool{NetworkNone: true}, environment: map[string]bool{},
		sources: map[string]ManagedSource{}, maximumResources: policy.MaximumResources,
	}
	if err := validatePositiveResources("policy maximum", policy.MaximumResources); err != nil {
		return normalizedPolicy{}, err
	}
	for i, image := range policy.ApprovedImages {
		if err := validateImage(image); err != nil {
			return normalizedPolicy{}, invalidf("policy approved_images[%d]: %v", i, err)
		}
		key := imageKey(image)
		if result.images[key] {
			return normalizedPolicy{}, invalidf("policy approved_images[%d] is duplicated", i)
		}
		result.images[key] = true
	}
	if len(result.images) == 0 {
		return normalizedPolicy{}, invalidf("policy requires at least one approved image")
	}
	profiles := policy.AllowedProfiles
	if len(profiles) == 0 {
		profiles = []RuntimeProfile{ProfileStrict}
	}
	for i, profile := range profiles {
		if !validProfile(profile) || result.profiles[profile] {
			return normalizedPolicy{}, invalidf("policy allowed_profiles[%d] is invalid or duplicated", i)
		}
		result.profiles[profile] = true
	}
	seenNetworks := map[NetworkProfile]bool{}
	for i, network := range policy.AllowedNetworks {
		if !validNetwork(network) || seenNetworks[network] {
			return normalizedPolicy{}, invalidf("policy allowed_networks[%d] is invalid or duplicated", i)
		}
		seenNetworks[network] = true
		result.networks[network] = true
	}
	for i, name := range policy.AllowedEnvironmentNames {
		if !environmentName.MatchString(name) || forbiddenEnvironment(name) || result.environment[name] {
			return normalizedPolicy{}, invalidf("policy allowed_environment_names[%d] is unsafe or duplicated", i)
		}
		result.environment[name] = true
	}
	for i, source := range policy.ManagedSources {
		if err := validatePolicyText(fmt.Sprintf("managed_sources[%d].id", i), source.ID, maxIdentityBytes); err != nil {
			return normalizedPolicy{}, err
		}
		if _, exists := result.sources[source.ID]; exists {
			return normalizedPolicy{}, invalidf("policy managed source %q is duplicated", source.ID)
		}
		if !validSourceKind(source.Kind) || !validSourceType(source.Type) {
			return normalizedPolicy{}, invalidf("policy managed source %q has unsafe kind or type", source.ID)
		}
		if source.Kind != SourceContext && source.Type != SourceDirectory {
			return normalizedPolicy{}, invalidf("policy managed source %q must be a directory", source.ID)
		}
		if err := validateManagedRelativePath(source.RelativePath); err != nil {
			return normalizedPolicy{}, invalidf("policy managed source %q: %v", source.ID, err)
		}
		if !filepath.IsAbs(source.Root) || filepath.Clean(source.Root) != source.Root {
			return normalizedPolicy{}, invalidf("policy managed source %q root must be an absolute clean path", source.ID)
		}
		if err := validateTarget(source.Kind, source.Target); err != nil {
			return normalizedPolicy{}, invalidf("policy managed source %q: %v", source.ID, err)
		}
		result.sources[source.ID] = source
	}
	return result, nil
}

func validateMounts(requests []Mount, sources map[string]ManagedSource, configuredForbidden []string, role Role) ([]ResolvedMount, error) {
	if len(requests) == 0 || len(requests) > maxMounts {
		return nil, invalidf("mount count must be between 1 and %d", maxMounts)
	}
	forbidden, err := forbiddenPaths(configuredForbidden)
	if err != nil {
		return nil, err
	}
	resolved := make([]ResolvedMount, 0, len(requests))
	seenSources := make(map[string]bool, len(requests))
	workspaceCount := 0
	for i, mount := range requests {
		source, ok := sources[mount.SourceID]
		if !ok {
			return nil, invalidf("mounts[%d] source_id %q is not managed", i, mount.SourceID)
		}
		if seenSources[mount.SourceID] {
			return nil, invalidf("mounts[%d] duplicates source_id %q", i, mount.SourceID)
		}
		seenSources[mount.SourceID] = true
		if mount.Target != source.Target {
			return nil, invalidf("mounts[%d] target does not match managed source %q", i, mount.SourceID)
		}
		switch source.Kind {
		case SourceWorkspace:
			workspaceCount++
			if role == RoleVerifier && mount.Mode != MountReadOnly {
				return nil, invalidf("verifier workspace mount must be read-only")
			}
			if role != RoleVerifier && mount.Mode != MountReadWrite {
				return nil, invalidf("implementer or corrector workspace mount must be read-write")
			}
		case SourceContext, SourceCache:
			if mount.Mode != MountReadOnly {
				return nil, invalidf("%s mount must be read-only", source.Kind)
			}
		}
		item, err := resolveMount(source, mount, forbidden)
		if err != nil {
			return nil, invalidf("mounts[%d]: %v", i, err)
		}
		resolved = append(resolved, item)
	}
	if workspaceCount != 1 {
		return nil, invalidf("exactly one workspace mount is required")
	}
	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].Target+"\x00"+resolved[i].SourceID < resolved[j].Target+"\x00"+resolved[j].SourceID
	})
	for i := range resolved {
		for j := i + 1; j < len(resolved); j++ {
			if slashPathsOverlap(resolved[i].Target, resolved[j].Target) {
				return nil, invalidf("mount targets %q and %q overlap", resolved[i].Target, resolved[j].Target)
			}
			if filesystemPathsOverlap(resolved[i].SourcePath, resolved[j].SourcePath) {
				return nil, invalidf("mount sources %q and %q overlap", resolved[i].SourceID, resolved[j].SourceID)
			}
		}
	}
	return resolved, nil
}

type forbiddenPath struct {
	path       string
	descendant bool
}

func resolveMount(source ManagedSource, mount Mount, forbidden []forbiddenPath) (ResolvedMount, error) {
	boundary, err := runtimepath.Bind(source.Root)
	if err != nil {
		return ResolvedMount{}, fmt.Errorf("bind managed root: %w", err)
	}
	rootInfo, err := os.Lstat(boundary.Root())
	if err != nil {
		return ResolvedMount{}, fmt.Errorf("inspect managed root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return ResolvedMount{}, errors.New("managed root is not a directory")
	}
	if rootInfo.Mode().Perm()&0o022 != 0 {
		return ResolvedMount{}, fmt.Errorf("managed root has unsafe directory mode %04o", rootInfo.Mode().Perm())
	}
	sourcePath, err := pathguard.Resolve(boundary.Root(), source.RelativePath)
	if err != nil {
		return ResolvedMount{}, fmt.Errorf("resolve managed source: %w", err)
	}
	if sourcePath == boundary.Root() {
		return ResolvedMount{}, errors.New("managed source must be below its managed root")
	}
	for _, blocked := range forbidden {
		if pathguard.WithinRoot(sourcePath, blocked.path) || (blocked.descendant && pathguard.WithinRoot(blocked.path, sourcePath)) {
			return ResolvedMount{}, fmt.Errorf("managed source exposes forbidden host path %q", blocked.path)
		}
	}
	var device, inode uint64
	switch source.Type {
	case SourceDirectory:
		directory, found, err := boundary.OpenDir(sourcePath, false)
		if err != nil || !found {
			if err == nil {
				err = os.ErrNotExist
			}
			return ResolvedMount{}, fmt.Errorf("open managed directory: %w", err)
		}
		defer directory.Close()
		device, inode, err = directory.Identity()
	case SourceFile:
		parent, found, openErr := boundary.OpenDir(filepath.Dir(sourcePath), false)
		if openErr != nil || !found {
			if openErr == nil {
				openErr = os.ErrNotExist
			}
			return ResolvedMount{}, fmt.Errorf("open managed file parent: %w", openErr)
		}
		defer parent.Close()
		file, openErr := parent.OpenFile(filepath.Base(sourcePath), os.O_RDONLY, 0)
		if openErr != nil {
			return ResolvedMount{}, fmt.Errorf("open managed file: %w", openErr)
		}
		defer file.Close()
		device, inode, err = file.Identity()
	}
	if err != nil {
		return ResolvedMount{}, fmt.Errorf("identify managed source: %w", err)
	}
	return ResolvedMount{
		SourceID: mount.SourceID, ManagedRoot: boundary.Root(), SourcePath: sourcePath,
		SourceType: source.Type, SourceDevice: device, SourceInode: inode,
		Target: mount.Target, Mode: mount.Mode,
	}, nil
}

func validateEnvironment(values map[string]string, allowed map[string]bool) ([]EnvironmentVariable, error) {
	if len(values) > maxEnvironment {
		return nil, invalidf("environment exceeds %d variables", maxEnvironment)
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]EnvironmentVariable, 0, len(names))
	total := 0
	for _, name := range names {
		value := values[name]
		if !environmentName.MatchString(name) || forbiddenEnvironment(name) || !allowed[name] {
			return nil, invalidf("environment name %q is not allowed", name)
		}
		if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > maxEnvironmentValue {
			return nil, invalidf("environment value for %q is empty, malformed, or oversized", name)
		}
		total += len(name) + len(value)
		if total > maxEnvironmentBytes {
			return nil, invalidf("environment exceeds %d bytes", maxEnvironmentBytes)
		}
		result = append(result, EnvironmentVariable{Name: name, Value: value})
	}
	return result, nil
}

func validateCommand(command []string) ([]string, error) {
	if len(command) == 0 || len(command) > maxCommandArguments {
		return nil, invalidf("command must contain between 1 and %d direct argv values", maxCommandArguments)
	}
	total := 0
	result := make([]string, len(command))
	for i, argument := range command {
		if argument == "" || !utf8.ValidString(argument) || strings.ContainsRune(argument, 0) || len(argument) > maxArgumentBytes {
			return nil, invalidf("command[%d] is empty, malformed, or oversized", i)
		}
		total += len(argument)
		if total > maxCommandBytes {
			return nil, invalidf("command exceeds %d bytes", maxCommandBytes)
		}
		result[i] = argument
	}
	return result, nil
}

func validateResources(resources, maximum Resources) error {
	if err := validatePositiveResources("resources", resources); err != nil {
		return err
	}
	for _, value := range []struct {
		name string
		got  int64
		max  int64
	}{
		{"cpus", resources.CPUs, maximum.CPUs},
		{"memory_bytes", resources.MemoryBytes, maximum.MemoryBytes},
		{"pids", resources.PIDs, maximum.PIDs},
		{"timeout_seconds", resources.TimeoutSeconds, maximum.TimeoutSeconds},
		{"tmpfs_bytes", resources.TmpfsBytes, maximum.TmpfsBytes},
	} {
		if value.got > value.max {
			return invalidf("resources.%s exceeds policy maximum", value.name)
		}
	}
	return nil
}

func validatePositiveResources(label string, resources Resources) error {
	for _, value := range []struct {
		name string
		got  int64
	}{
		{"cpus", resources.CPUs}, {"memory_bytes", resources.MemoryBytes},
		{"pids", resources.PIDs}, {"timeout_seconds", resources.TimeoutSeconds},
		{"tmpfs_bytes", resources.TmpfsBytes},
	} {
		if value.got <= 0 {
			return invalidf("%s.%s must be positive", label, value.name)
		}
	}
	return nil
}

func forbiddenPaths(configured []string) ([]forbiddenPath, error) {
	result := make([]forbiddenPath, 0, len(configured)+16)
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, invalidf("resolve host home: %v", err)
	}
	home, err = comparisonPath(home)
	if err != nil {
		return nil, invalidf("resolve host home: %v", err)
	}
	result = append(result, forbiddenPath{path: home})
	for _, relative := range []string{".ssh", ".docker", ".config/containers", ".config/openai", ".pgpass"} {
		result = append(result, forbiddenPath{path: filepath.Join(home, filepath.FromSlash(relative)), descendant: true})
	}
	sockets := []string{
		"/run/docker.sock", "/var/run/docker.sock", "/run/containerd/containerd.sock",
		"/var/run/crio/crio.sock", "/run/podman/podman.sock",
		filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "docker.sock"),
		filepath.Join("/run/user", strconv.Itoa(os.Getuid()), "podman", "podman.sock"),
	}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtimeDir) {
		sockets = append(sockets, filepath.Join(runtimeDir, "docker.sock"), filepath.Join(runtimeDir, "podman", "podman.sock"))
	}
	for _, socket := range sockets {
		canonical, pathErr := comparisonPath(socket)
		if pathErr != nil {
			return nil, invalidf("resolve runtime socket path: %v", pathErr)
		}
		result = append(result, forbiddenPath{path: canonical})
	}
	for i, configuredPath := range configured {
		canonical, pathErr := comparisonPath(configuredPath)
		if pathErr != nil {
			return nil, invalidf("policy forbidden_host_paths[%d]: %v", i, pathErr)
		}
		result = append(result, forbiddenPath{path: canonical, descendant: true})
	}
	return result, nil
}

func comparisonPath(value string) (string, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", errors.New("path must be absolute and clean")
	}
	current := value
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			relative, err := filepath.Rel(current, value)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("no existing path ancestor")
		}
		current = parent
	}
}

func validateImage(image Image) error {
	if err := validateRawText("image.reference", image.Reference, maxImageBytes); err != nil {
		return err
	}
	if strings.Contains(image.Reference, "@") || strings.ContainsAny(image.Reference, " \t") {
		return errors.New("image.reference must not contain a digest or whitespace")
	}
	if !validDigest(image.Digest) {
		return errors.New("image.digest must be a lowercase sha256 digest")
	}
	return nil
}

func validDigest(value string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	raw := strings.TrimPrefix(value, prefix)
	decoded, err := hex.DecodeString(raw)
	return err == nil && len(decoded) == sha256.Size && raw == strings.ToLower(raw)
}

func imageKey(image Image) string { return image.Reference + "\x00" + image.Digest }

func validateManagedRelativePath(value string) error {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") || filepath.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
		return errors.New("relative_path must be a nonempty canonical relative path without traversal")
	}
	return nil
}

func validateTarget(kind SourceKind, target string) error {
	if target == "" || !path.IsAbs(target) || strings.Contains(target, "\\") || path.Clean(target) != target {
		return errors.New("target must be a canonical absolute container path")
	}
	switch kind {
	case SourceWorkspace:
		if target != "/workspace" {
			return errors.New("workspace target must be /workspace")
		}
	case SourceContext:
		if target != "/context" && !strings.HasPrefix(target, "/context/") {
			return errors.New("context target must be /context or below it")
		}
	case SourceCache:
		if !strings.HasPrefix(target, "/cache/") {
			return errors.New("cache target must be below /cache")
		}
	}
	return nil
}

func slashPathsOverlap(left, right string) bool {
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func filesystemPathsOverlap(left, right string) bool {
	return pathguard.WithinRoot(left, right) || pathguard.WithinRoot(right, left)
}

func validRole(role Role) bool {
	return role == RoleImplementer || role == RoleCorrector || role == RoleVerifier
}

func validProfile(profile RuntimeProfile) bool {
	return profile == ProfileStrict || profile == ProfileCompatible || profile == ProfileDiagnostic
}

func validNetwork(network NetworkProfile) bool {
	return network == NetworkNone || network == NetworkDependencies || network == NetworkOpen
}

func validSourceKind(kind SourceKind) bool {
	return kind == SourceWorkspace || kind == SourceContext || kind == SourceCache
}

func validSourceType(sourceType SourceType) bool {
	return sourceType == SourceDirectory || sourceType == SourceFile
}

func forbiddenEnvironment(name string) bool {
	switch strings.ToUpper(name) {
	case "HOME", "USERPROFILE", "SSH_AUTH_SOCK", "GIT_SSH_COMMAND",
		"DOCKER_HOST", "CONTAINER_HOST", "DOCKER_CONFIG",
		"OPENAI_API_KEY", "OPENAI_ORG_ID", "OPENAI_PROJECT_ID",
		"DATABASE_URL", "REVOLVR_DATABASE_URL", "PGPASSWORD", "PGPASSFILE", "PGSERVICEFILE",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"GITHUB_TOKEN", "GH_TOKEN", "GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CLIENT_SECRET":
		return true
	default:
		return false
	}
}

func validateText(name, value string, limit int) error {
	if err := validateRawText(name, value, limit); err != nil {
		return invalidf("%v", err)
	}
	return nil
}

func validatePolicyText(name, value string, limit int) error {
	if err := validateRawText("policy "+name, value, limit); err != nil {
		return invalidf("%v", err)
	}
	return nil
}

func validateRawText(name, value string, limit int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") || len(value) > limit {
		return fmt.Errorf("%s must be nonempty single-line UTF-8 text of at most %d bytes", name, limit)
	}
	return nil
}

func rejectDuplicateFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var inspect func() error
	inspect = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate field %q", key)
				}
				seen[key] = true
				if err := inspect(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := inspect(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
	}
	return inspect()
}

func invalidf(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, arguments...))
}
