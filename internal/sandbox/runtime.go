package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"revolvr/internal/pathguard"
	"revolvr/internal/runtimepath"
)

const EvidenceSchemaVersion = "revolvr-sandbox-evidence-v1"

var (
	ErrRuntimeUnavailable = errors.New("sandbox runtime unavailable")
	ErrProfileUnavailable = errors.New("sandbox runtime profile unavailable")
	ErrSandboxNotFound    = errors.New("sandbox not found")
)

type SandboxHandle struct {
	ID      string
	Name    string
	Command []string
}

type CommandSpec struct {
	Arguments []string
	Timeout   time.Duration
}

type CommandResult struct {
	ExitCode             int
	Error                error
	TimedOut             bool
	Cancelled            bool
	Stdout               string
	Stderr               string
	StdoutTruncatedBytes int64
	StderrTruncatedBytes int64
}

type SandboxStatus struct {
	State      string
	Running    bool
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}

// SandboxRuntime is the only container-engine authority used by sandboxd.
type SandboxRuntime interface {
	Create(context.Context, Specification) (SandboxHandle, error)
	Exec(context.Context, SandboxHandle, CommandSpec) (CommandResult, error)
	Stop(context.Context, SandboxHandle) error
	Inspect(context.Context, SandboxHandle) (SandboxStatus, error)
	Remove(context.Context, SandboxHandle) error
}

type LifecycleTransition struct {
	State string    `json:"state"`
	At    time.Time `json:"at"`
}

type ArtifactReference struct {
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	SizeBytes      int64  `json:"size_bytes"`
	TruncatedBytes int64  `json:"truncated_bytes"`
}

type Evidence struct {
	SchemaVersion     string                `json:"schema_version"`
	SpecificationHash string                `json:"specification_sha256"`
	SandboxID         string                `json:"sandbox_id"`
	ProjectID         string                `json:"project_id"`
	TaskID            string                `json:"task_id"`
	RunID             string                `json:"run_id"`
	Role              Role                  `json:"role"`
	Runtime           string                `json:"runtime"`
	RuntimeProfile    RuntimeProfile        `json:"runtime_profile"`
	Image             Image                 `json:"image"`
	Command           []string              `json:"command"`
	WorkingDirectory  string                `json:"working_directory"`
	Network           NetworkProfile        `json:"network"`
	Resources         Resources             `json:"resources"`
	Transitions       []LifecycleTransition `json:"transitions"`
	ExitCode          int                   `json:"exit_code"`
	TimedOut          bool                  `json:"timed_out"`
	Cancelled         bool                  `json:"cancelled"`
	Error             string                `json:"error,omitempty"`
	Stdout            ArtifactReference     `json:"stdout"`
	Stderr            ArtifactReference     `json:"stderr"`
	EvidencePath      string                `json:"evidence_path"`
}

type Manager struct {
	runtime SandboxRuntime
	store   runtimepath.Boundary
	now     func() time.Time
}

func NewManager(runtime SandboxRuntime, stateDirectory string) (*Manager, error) {
	if runtime == nil {
		return nil, errors.New("sandbox manager: runtime is required")
	}
	store, err := runtimepath.Bind(stateDirectory)
	if err != nil {
		return nil, fmt.Errorf("sandbox manager: bind state directory: %w", err)
	}
	return &Manager{runtime: runtime, store: store, now: time.Now}, nil
}

// ReadArtifact returns exactly the bytes named by one manager-produced
// reference after rechecking the bound state root, size, and content hash.
func (m *Manager) ReadArtifact(reference ArtifactReference, maximumBytes int64) ([]byte, error) {
	if maximumBytes < 0 || reference.SizeBytes < 0 || reference.SizeBytes > maximumBytes {
		return nil, errors.New("read sandbox artifact: size exceeds policy")
	}
	raw, found, err := m.store.ReadFileLimit(reference.Path, false, maximumBytes+1)
	if err != nil || !found {
		return nil, fmt.Errorf("read sandbox artifact: %w", errors.Join(err, os.ErrNotExist))
	}
	if int64(len(raw)) != reference.SizeBytes {
		return nil, errors.New("read sandbox artifact: recorded size does not match bytes")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != reference.SHA256 {
		return nil, errors.New("read sandbox artifact: recorded hash does not match bytes")
	}
	return raw, nil
}

func (m *Manager) Run(ctx context.Context, specification Specification) (Evidence, error) {
	if err := CheckSpecification(specification); err != nil {
		return Evidence{}, err
	}
	identity, err := specification.SHA256()
	if err != nil {
		return Evidence{}, err
	}
	evidence := Evidence{
		SchemaVersion: EvidenceSchemaVersion, SpecificationHash: identity,
		SandboxID: specification.SandboxID, ProjectID: specification.ProjectID,
		TaskID: specification.TaskID, RunID: specification.RunID, Role: specification.Role,
		Runtime: "docker-rootless", RuntimeProfile: specification.RuntimeProfile,
		Image: specification.Image, Command: append([]string(nil), specification.Command...),
		WorkingDirectory: specification.WorkingDirectory,
		Network:          specification.Network, Resources: specification.Resources, ExitCode: -1,
	}
	transition := func(state string) {
		evidence.Transitions = append(evidence.Transitions, LifecycleTransition{State: state, At: m.now().UTC()})
	}
	transition("requested")
	transition("validated")
	transition("creating")
	handle, runErr := m.runtime.Create(ctx, specification)
	if runErr != nil {
		transition("failed")
		evidence.Error = boundedError(runErr)
		persistErr := m.persist(&evidence, nil, nil)
		return evidence, errors.Join(runErr, persistErr)
	}

	transition("running")
	result, execErr := m.runtime.Exec(ctx, handle, CommandSpec{
		Arguments: specification.Command,
		Timeout:   time.Duration(specification.Resources.TimeoutSeconds) * time.Second,
	})
	evidence.ExitCode = result.ExitCode
	evidence.TimedOut = result.TimedOut
	evidence.Cancelled = result.Cancelled || errors.Is(ctx.Err(), context.Canceled)
	if execErr != nil {
		runErr = errors.Join(runErr, execErr)
	}
	if result.Error != nil {
		runErr = errors.Join(runErr, result.Error)
	}
	evidence.Stdout.TruncatedBytes = result.StdoutTruncatedBytes
	evidence.Stderr.TruncatedBytes = result.StderrTruncatedBytes

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if evidence.TimedOut || evidence.Cancelled || runErr != nil {
		transition("stopping")
		if stopErr := m.runtime.Stop(cleanupCtx, handle); stopErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("stop sandbox: %w", stopErr))
		}
	}
	if evidence.TimedOut {
		transition("timed_out")
	} else if evidence.Cancelled {
		transition("cancelled")
	} else if runErr != nil {
		transition("failed")
	} else {
		transition("exited")
	}
	if _, inspectErr := m.runtime.Inspect(cleanupCtx, handle); inspectErr != nil && !errors.Is(inspectErr, ErrSandboxNotFound) {
		runErr = errors.Join(runErr, fmt.Errorf("inspect sandbox: %w", inspectErr))
	}
	if removeErr := m.runtime.Remove(cleanupCtx, handle); removeErr != nil {
		runErr = errors.Join(runErr, fmt.Errorf("remove sandbox: %w", removeErr))
	} else {
		transition("removed")
	}
	if runErr != nil {
		evidence.Error = boundedError(runErr)
	}
	persistErr := m.persist(&evidence, []byte(result.Stdout), []byte(result.Stderr))
	return evidence, errors.Join(runErr, persistErr)
}

func (m *Manager) persist(evidence *Evidence, stdout, stderr []byte) error {
	evidenceDirectory := directoryPath(m.store, evidence.SpecificationHash)
	if err := m.store.EnsureDir(evidenceDirectory, 0o700); err != nil {
		return fmt.Errorf("record sandbox evidence: %w", err)
	}
	directory, found, err := m.store.OpenDir(evidenceDirectory, false)
	if err != nil || !found {
		return fmt.Errorf("record sandbox evidence: %w", errors.Join(err, os.ErrNotExist))
	}
	defer directory.Close()
	evidence.Stdout = artifactReference(filepath.Join(evidenceDirectory, "stdout"), stdout, evidence.Stdout.TruncatedBytes)
	evidence.Stderr = artifactReference(filepath.Join(evidenceDirectory, "stderr"), stderr, evidence.Stderr.TruncatedBytes)
	evidence.EvidencePath = filepath.Join(evidenceDirectory, "evidence.json")
	if err := publish(directory, "stdout", stdout); err != nil {
		return err
	}
	if err := publish(directory, "stderr", stderr); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return publish(directory, "evidence.json", raw)
}

func directoryPath(boundary runtimepath.Boundary, name string) string {
	return filepath.Join(boundary.Root(), name)
}

func artifactReference(path string, raw []byte, truncated int64) ArtifactReference {
	sum := sha256.Sum256(raw)
	return ArtifactReference{Path: path, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(raw)), TruncatedBytes: truncated}
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	const limit = 4096
	value := err.Error()
	if len(value) <= limit {
		return value
	}
	return value[:limit] + " [truncated]"
}

func publish(directory *runtimepath.Directory, name string, raw []byte) error {
	temporary, err := directory.CreateTemp(".sandbox-", 0o600)
	if err != nil {
		return err
	}
	defer temporary.Close()
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := directory.Replace(temporary, name); err != nil {
		return err
	}
	return directory.Sync()
}

// CheckSpecification rechecks normalized structure and every mount inode at
// the last trusted boundary before container creation.
func CheckSpecification(specification Specification) error {
	if specification.SchemaVersion != RequestSchemaVersion {
		return invalidf("unsupported normalized schema_version %q", specification.SchemaVersion)
	}
	for _, value := range []struct{ name, value string }{
		{"sandbox_id", specification.SandboxID}, {"project_id", specification.ProjectID},
		{"task_id", specification.TaskID}, {"run_id", specification.RunID},
	} {
		if err := validateText(value.name, value.value, maxIdentityBytes); err != nil {
			return err
		}
	}
	if !validRole(specification.Role) || !validProfile(specification.RuntimeProfile) || !validNetwork(specification.Network) {
		return invalidf("normalized role, runtime_profile, or network is invalid")
	}
	if specification.Role == RoleVerifier && specification.Network != NetworkNone {
		return invalidf("verifier network must be none")
	}
	if err := validateImage(specification.Image); err != nil {
		return invalidf("normalized image: %v", err)
	}
	if command, err := validateCommand(specification.Command); err != nil {
		return err
	} else if len(command) != len(specification.Command) {
		return invalidf("normalized command is invalid")
	}
	if workingDirectory, err := validateWorkingDirectory(specification.WorkingDirectory); err != nil {
		return err
	} else if workingDirectory != specification.WorkingDirectory {
		return invalidf("normalized working_directory is invalid")
	}
	if err := validatePositiveResources("resources", specification.Resources); err != nil {
		return err
	}
	if err := checkNormalizedEnvironment(specification); err != nil {
		return err
	}
	return checkNormalizedMounts(specification.Mounts, specification.Role)
}

func checkNormalizedEnvironment(specification Specification) error {
	if len(specification.Environment) > maxEnvironment {
		return invalidf("environment exceeds %d variables", maxEnvironment)
	}
	total := 0
	previous := ""
	for i, variable := range specification.Environment {
		if variable.Name <= previous || !environmentName.MatchString(variable.Name) || forbiddenEnvironment(variable.Name) {
			return invalidf("normalized environment[%d] is unsafe or unsorted", i)
		}
		if variable.Value == "" || !utf8.ValidString(variable.Value) || strings.ContainsRune(variable.Value, 0) || len(variable.Value) > maxEnvironmentValue {
			return invalidf("normalized environment[%d] value is invalid", i)
		}
		total += len(variable.Name) + len(variable.Value)
		if total > maxEnvironmentBytes {
			return invalidf("environment exceeds %d bytes", maxEnvironmentBytes)
		}
		switch variable.Name {
		case "TASK_ID":
			if variable.Value != specification.TaskID {
				return invalidf("environment TASK_ID does not match task_id")
			}
		case "RUN_ID":
			if variable.Value != specification.RunID {
				return invalidf("environment RUN_ID does not match run_id")
			}
		case "ROLE":
			if variable.Value != string(specification.Role) {
				return invalidf("environment ROLE does not match role")
			}
		}
		previous = variable.Name
	}
	return nil
}

func checkNormalizedMounts(mounts []ResolvedMount, role Role) error {
	if len(mounts) == 0 || len(mounts) > maxMounts {
		return invalidf("normalized mount count must be between 1 and %d", maxMounts)
	}
	forbidden, err := forbiddenPaths(nil)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	workspaceCount := 0
	for i, mount := range mounts {
		if err := validateText(fmt.Sprintf("normalized mounts[%d].source_id", i), mount.SourceID, maxIdentityBytes); err != nil {
			return err
		}
		if seen[mount.SourceID] {
			return invalidf("normalized mount source_id %q is duplicated", mount.SourceID)
		}
		seen[mount.SourceID] = true
		if !path.IsAbs(mount.Target) || path.Clean(mount.Target) != mount.Target || strings.Contains(mount.Target, "\\") {
			return invalidf("normalized mount target %q is invalid", mount.Target)
		}
		if i > 0 && mounts[i-1].Target+"\x00"+mounts[i-1].SourceID >= mount.Target+"\x00"+mount.SourceID {
			return invalidf("normalized mounts are not ordered")
		}
		switch {
		case mount.Target == "/workspace":
			workspaceCount++
			expectedMode := MountReadWrite
			if role == RoleVerifier {
				expectedMode = MountReadOnly
			}
			if mount.Mode != expectedMode || mount.SourceType != SourceDirectory {
				return invalidf("normalized workspace mount is invalid")
			}
		case mount.Target == "/context" || strings.HasPrefix(mount.Target, "/context/"):
			if mount.Mode != MountReadOnly {
				return invalidf("normalized context mount is writable")
			}
		case strings.HasPrefix(mount.Target, "/cache/"):
			if mount.Mode != MountReadOnly || mount.SourceType != SourceDirectory {
				return invalidf("normalized cache mount is invalid")
			}
		default:
			return invalidf("normalized mount target %q is invalid", mount.Target)
		}
		if err := recheckMount(mount, forbidden); err != nil {
			return invalidf("normalized mounts[%d]: %v", i, err)
		}
	}
	if workspaceCount != 1 {
		return invalidf("exactly one workspace mount is required")
	}
	for i := range mounts {
		for j := i + 1; j < len(mounts); j++ {
			if slashPathsOverlap(mounts[i].Target, mounts[j].Target) || filesystemPathsOverlap(mounts[i].SourcePath, mounts[j].SourcePath) {
				return invalidf("normalized mounts overlap")
			}
		}
	}
	return nil
}

func recheckMount(mount ResolvedMount, forbidden []forbiddenPath) error {
	if !filepath.IsAbs(mount.ManagedRoot) || filepath.Clean(mount.ManagedRoot) != mount.ManagedRoot ||
		!filepath.IsAbs(mount.SourcePath) || filepath.Clean(mount.SourcePath) != mount.SourcePath ||
		mount.SourcePath == mount.ManagedRoot || !pathguard.WithinRoot(mount.ManagedRoot, mount.SourcePath) {
		return errors.New("source path is outside its managed root")
	}
	for _, blocked := range forbidden {
		if pathguard.WithinRoot(mount.SourcePath, blocked.path) || (blocked.descendant && pathguard.WithinRoot(blocked.path, mount.SourcePath)) {
			return fmt.Errorf("source exposes forbidden host path %q", blocked.path)
		}
	}
	boundary, err := runtimepath.Bind(mount.ManagedRoot)
	if err != nil {
		return err
	}
	var device, inode uint64
	switch mount.SourceType {
	case SourceDirectory:
		directory, found, openErr := boundary.OpenDir(mount.SourcePath, false)
		if openErr != nil || !found {
			return errors.Join(openErr, os.ErrNotExist)
		}
		defer directory.Close()
		device, inode, err = directory.Identity()
	case SourceFile:
		parent, found, openErr := boundary.OpenDir(filepath.Dir(mount.SourcePath), false)
		if openErr != nil || !found {
			return errors.Join(openErr, os.ErrNotExist)
		}
		defer parent.Close()
		file, openErr := parent.OpenFile(filepath.Base(mount.SourcePath), os.O_RDONLY, 0)
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		device, inode, err = file.Identity()
	default:
		return errors.New("source type is invalid")
	}
	if err != nil {
		return err
	}
	if device != mount.SourceDevice || inode != mount.SourceInode {
		return errors.New("source filesystem identity changed after validation")
	}
	return nil
}
