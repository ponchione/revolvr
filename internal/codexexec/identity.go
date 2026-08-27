package codexexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ExecutableLookPath func(string) (string, error)

var ErrExecutableIdentityDrift = errors.New("executable identity drift")

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
	if !validCodexVersion(i.Version) {
		return errors.New("Codex executable identity: a nonempty bounded normalized single-line version without control characters is required")
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
		return fmt.Errorf("%w: inspect current executable: %v", ErrExecutableIdentityDrift, err)
	}
	if !reflect.DeepEqual(current, expected) {
		return fmt.Errorf("%w: admitted resolved=%q sha256=%s current resolved=%q sha256=%s", ErrExecutableIdentityDrift, expected.Resolved, expected.SHA256, current.Resolved, current.SHA256)
	}
	return nil
}

func InspectCodex(ctx context.Context, configured, workDir string, timeoutConfig VersionConfig, lookPath ExecutableLookPath) (CodexExecutableIdentity, error) {
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
	if err := VerifyExecutableIdentity(executable, lookPath); err != nil {
		return CodexExecutableIdentity{}, fmt.Errorf("inspect Codex executable after version discovery: %w", err)
	}
	identity := CodexExecutableIdentity{Version: version, Executable: executable}
	if err := identity.Validate(); err != nil {
		return CodexExecutableIdentity{}, err
	}
	return identity, nil
}

func VerifyCodexIdentity(ctx context.Context, expected CodexExecutableIdentity, workDir string, timeoutConfig VersionConfig, lookPath ExecutableLookPath) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	current, err := InspectCodex(ctx, expected.Executable.Configured, workDir, timeoutConfig, lookPath)
	if err != nil {
		return fmt.Errorf("Codex %w: inspect current executable: %v", ErrExecutableIdentityDrift, err)
	}
	if !reflect.DeepEqual(current, expected) {
		return fmt.Errorf("Codex %w: admitted version=%q resolved=%q sha256=%s current version=%q resolved=%q sha256=%s", ErrExecutableIdentityDrift, expected.Version, expected.Executable.Resolved, expected.Executable.SHA256, current.Version, current.Executable.Resolved, current.Executable.SHA256)
	}
	return nil
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

func validCodexVersion(value string) bool {
	return value != "" &&
		len(value) <= DefaultVersionOutputCap &&
		utf8.ValidString(value) &&
		value == strings.TrimSpace(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func validIdentitySHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
