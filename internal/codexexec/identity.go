package codexexec

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

const ReleaseManifestSchema = "revolvr-release-executable-manifest-v1"

//go:embed release_manifest.json
var releaseManifestJSON []byte

type ExecutableLookPath func(string) (string, error)

// ExecutableIdentity binds a configured command name to the canonical file
// whose bytes may be executed.
type ExecutableIdentity struct {
	Configured string `json:"configured"`
	Resolved   string `json:"resolved"`
	SHA256     string `json:"sha256"`
}

type CodexExecutableIdentity struct {
	Version    string             `json:"version"`
	Executable ExecutableIdentity `json:"executable"`
}

type ReleaseCodexBuild struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type ReleaseManifest struct {
	SchemaVersion string              `json:"schema_version"`
	Codex         []ReleaseCodexBuild `json:"codex"`
}

func CurrentReleaseManifest() (ReleaseManifest, error) {
	var manifest ReleaseManifest
	decoder := json.NewDecoder(strings.NewReader(string(releaseManifestJSON)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release executable manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return ReleaseManifest{}, fmt.Errorf("decode release executable manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return ReleaseManifest{}, err
	}
	manifest.Codex = append([]ReleaseCodexBuild(nil), manifest.Codex...)
	return manifest, nil
}

func (m ReleaseManifest) Validate() error {
	if m.SchemaVersion != ReleaseManifestSchema {
		return fmt.Errorf("release executable manifest: unsupported schema_version %q", m.SchemaVersion)
	}
	if len(m.Codex) != 1 {
		return fmt.Errorf("release executable manifest: exactly one Codex build is required, got %d", len(m.Codex))
	}
	seen := make(map[string]struct{}, len(m.Codex))
	for i, build := range m.Codex {
		if !exactCodexVersion(build.Version) {
			return fmt.Errorf("release executable manifest: codex[%d] version must be one exact Codex CLI version string", i)
		}
		if !validIdentitySHA256(build.SHA256) {
			return fmt.Errorf("release executable manifest: codex[%d] SHA-256 must be 64 lowercase hexadecimal characters", i)
		}
		key := build.Version + "\x00" + build.SHA256
		if _, ok := seen[key]; ok {
			return fmt.Errorf("release executable manifest: duplicate Codex build %q", build.Version)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (m ReleaseManifest) Authorize(identity CodexExecutableIdentity) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	for _, build := range m.Codex {
		if identity.Version == build.Version && identity.Executable.SHA256 == build.SHA256 {
			return nil
		}
	}
	return fmt.Errorf("Codex executable identity is not release-authorized: version=%q sha256=%s", identity.Version, identity.Executable.SHA256)
}

func (i ExecutableIdentity) Validate() error {
	if strings.TrimSpace(i.Configured) == "" || i.Configured != strings.TrimSpace(i.Configured) || strings.ContainsAny(i.Configured, "\x00\r\n") {
		return errors.New("executable identity: configured executable is required and must be canonical")
	}
	if strings.TrimSpace(i.Resolved) == "" || !filepath.IsAbs(i.Resolved) || filepath.Clean(i.Resolved) != i.Resolved {
		return errors.New("executable identity: canonical absolute resolved path is required")
	}
	if !validIdentitySHA256(i.SHA256) {
		return errors.New("executable identity: SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}

func (i CodexExecutableIdentity) Validate() error {
	if !exactCodexVersion(i.Version) {
		return errors.New("Codex executable identity: exact version string is required")
	}
	return i.Executable.Validate()
}

func InspectExecutable(configured string, lookPath ExecutableLookPath) (ExecutableIdentity, error) {
	configured = strings.TrimSpace(configured)
	if configured == "" || strings.ContainsAny(configured, "\x00\r\n") {
		return ExecutableIdentity{}, errors.New("inspect executable identity: configured executable is required")
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(configured)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("resolve executable %q: %w", configured, err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("resolve executable %q: %w", configured, err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("resolve executable %q: %w", configured, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("resolve executable %q: %w", configured, err)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("open resolved executable %q: %w", resolved, err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("inspect resolved executable %q: %w", resolved, err)
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0o111 == 0 {
		return ExecutableIdentity{}, fmt.Errorf("inspect resolved executable %q: executable regular file is required", resolved)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ExecutableIdentity{}, fmt.Errorf("hash resolved executable %q: %w", resolved, err)
	}
	after, err := file.Stat()
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("reinspect resolved executable %q: %w", resolved, err)
	}
	named, err := os.Stat(resolved)
	if err != nil {
		return ExecutableIdentity{}, fmt.Errorf("reinspect resolved executable path %q: %w", resolved, err)
	}
	if !os.SameFile(before, after) || !os.SameFile(before, named) || before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return ExecutableIdentity{}, fmt.Errorf("inspect resolved executable %q: identity changed while hashing", resolved)
	}
	return ExecutableIdentity{Configured: configured, Resolved: filepath.Clean(resolved), SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func VerifyExecutableIdentity(expected ExecutableIdentity, lookPath ExecutableLookPath) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	current, err := InspectExecutable(expected.Configured, lookPath)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return fmt.Errorf("executable identity drift: admitted resolved=%q sha256=%s current resolved=%q sha256=%s", expected.Resolved, expected.SHA256, current.Resolved, current.SHA256)
	}
	return nil
}

func InspectReleaseCodex(ctx context.Context, configured, workDir string, timeoutConfig VersionConfig, lookPath ExecutableLookPath) (CodexExecutableIdentity, error) {
	manifest, err := CurrentReleaseManifest()
	if err != nil {
		return CodexExecutableIdentity{}, err
	}
	return InspectCodexWithManifest(ctx, configured, workDir, timeoutConfig, lookPath, manifest)
}

func InspectCodexWithManifest(ctx context.Context, configured, workDir string, timeoutConfig VersionConfig, lookPath ExecutableLookPath, manifest ReleaseManifest) (CodexExecutableIdentity, error) {
	executable, err := InspectExecutable(configured, lookPath)
	if err != nil {
		return CodexExecutableIdentity{}, err
	}
	timeoutConfig.Executable = executable.Resolved
	timeoutConfig.WorkingDir = workDir
	version, err := DiscoverVersion(ctx, timeoutConfig)
	if err != nil {
		return CodexExecutableIdentity{}, err
	}
	identity := CodexExecutableIdentity{Version: version, Executable: executable}
	if err := manifest.Authorize(identity); err != nil {
		return CodexExecutableIdentity{}, err
	}
	return identity, nil
}

func FormatExecutableIdentity(identity ExecutableIdentity) string {
	if err := identity.Validate(); err != nil {
		return "unresolved"
	}
	return fmt.Sprintf("configured=%q resolved=%q sha256=%s", identity.Configured, identity.Resolved, identity.SHA256)
}

func FormatCodexExecutableIdentity(identity CodexExecutableIdentity) string {
	if err := identity.Validate(); err != nil {
		return "unresolved"
	}
	return fmt.Sprintf("version=%q %s", identity.Version, FormatExecutableIdentity(identity.Executable))
}

func exactCodexVersion(value string) bool {
	if value != strings.TrimSpace(value) || !strings.HasPrefix(value, "codex-cli ") || strings.ContainsAny(value, "\x00\r\n<>=*^~|") {
		return false
	}
	version := strings.TrimPrefix(value, "codex-cli ")
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func validIdentitySHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
