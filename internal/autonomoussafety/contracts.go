// Package autonomoussafety defines the host-permission boundary for one
// autonomous task workspace. A Git worktree is treated as source isolation,
// never as an operating-system security sandbox.
package autonomoussafety

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/redact"
)

const (
	DeclarationSchemaVersion = "revolvr-autonomous-safety-declaration-v1"
	PolicySchemaVersion      = "revolvr-autonomous-safety-policy-v1"
	PreflightSchemaVersion   = "revolvr-autonomous-safety-preflight-v1"
	AcknowledgementPrefix    = "revolvr-fully-unattended-v1:"
)

type Mode string

const (
	ModeOperatorAttended Mode = "operator_attended"
	ModeFullyUnattended  Mode = "fully_unattended"
)

type IsolationExpectation string

const (
	IsolationNone      IsolationExpectation = "none"
	IsolationContainer IsolationExpectation = "container"
	IsolationOSSandbox IsolationExpectation = "os_sandbox"
)

type NetworkAccess string

const (
	NetworkUnknown      NetworkAccess = "unknown"
	NetworkDenied       NetworkAccess = "denied"
	NetworkRestricted   NetworkAccess = "restricted"
	NetworkUnrestricted NetworkAccess = "unrestricted"
)

type Enforcement string

const (
	EnforcementNone                Enforcement = "none"
	EnforcementExternalAttestation Enforcement = "external_attestation"
)

type HookPolicy string

const (
	HooksOperatorAttended HookPolicy = "operator_attended"
	HooksDisabled         HookPolicy = "disabled"
	HooksTrusted          HookPolicy = "trusted"
)

type Attestation struct {
	Authority string `json:"authority"`
	Evidence  string `json:"evidence"`
	SHA256    string `json:"sha256"`
}

type ExternalIsolation struct {
	Expectation IsolationExpectation `json:"expectation"`
	Enforcement Enforcement          `json:"enforcement"`
	Attestation *Attestation         `json:"attestation,omitempty"`
}

type NetworkPolicy struct {
	Access      NetworkAccess `json:"access"`
	Enforcement Enforcement   `json:"enforcement"`
	Attestation *Attestation  `json:"attestation,omitempty"`
}

type TrustedHook struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type HookTrust struct {
	Policy  HookPolicy    `json:"policy"`
	Trusted []TrustedHook `json:"trusted,omitempty"`
}

type EnvironmentPolicy struct {
	InheritHost bool     `json:"inherit_host"`
	Allow       []string `json:"allow,omitempty"`
}

type Declaration struct {
	SchemaVersion     string            `json:"schema_version"`
	Mode              Mode              `json:"mode"`
	ExternalIsolation ExternalIsolation `json:"external_isolation"`
	Network           NetworkPolicy     `json:"network"`
	Hooks             HookTrust         `json:"hooks"`
	Environment       EnvironmentPolicy `json:"environment"`
	Redaction         redact.Policy     `json:"redaction"`
	Acknowledgement   string            `json:"acknowledgement,omitempty"`
}

type CodexPolicy struct {
	Sandbox         string `json:"sandbox"`
	ApprovalPolicy  string `json:"approval_policy"`
	DangerousBypass bool   `json:"dangerous_bypass"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Ephemeral       bool   `json:"ephemeral"`
}

type WritableRoot struct {
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
	Actor   string `json:"actor"`
}

type ProtectedPath struct {
	Path  string `json:"path"`
	Class string `json:"class"`
}

type CommandProvenance struct {
	Kind             string        `json:"kind"`
	Configured       string        `json:"configured"`
	Resolved         string        `json:"resolved"`
	ExecutableSHA256 string        `json:"executable_sha256"`
	Argv             []string      `json:"argv"`
	WorkingDir       string        `json:"working_dir"`
	Environment      []string      `json:"environment,omitempty"`
	Timeout          time.Duration `json:"timeout"`
	StdoutCap        int           `json:"stdout_cap"`
	StderrCap        int           `json:"stderr_cap"`
}

type Policy struct {
	SchemaVersion       string                   `json:"schema_version"`
	TaskID              string                   `json:"task_id"`
	Workspace           autonomous.TaskWorkspace `json:"workspace"`
	Mode                Mode                     `json:"mode"`
	Codex               CodexPolicy              `json:"codex"`
	ExternalIsolation   ExternalIsolation        `json:"external_isolation"`
	Network             NetworkPolicy            `json:"network"`
	Hooks               HookTrust                `json:"hooks"`
	Environment         EnvironmentPolicy        `json:"environment"`
	Redaction           redact.Policy            `json:"redaction"`
	RedactionPolicyHash string                   `json:"redaction_policy_sha256"`
	WritableRoots       []WritableRoot           `json:"writable_roots"`
	ProtectedPaths      []ProtectedPath          `json:"protected_paths"`
	Commands            []CommandProvenance      `json:"commands"`
	ConfigPath          string                   `json:"config_path"`
	ConfigSHA256        string                   `json:"config_sha256"`
	Acknowledgement     string                   `json:"acknowledgement,omitempty"`
	PolicySHA256        string                   `json:"policy_sha256"`
	WorktreeNotice      string                   `json:"worktree_notice"`
}

type CheckStatus string

const (
	CheckOK   CheckStatus = "ok"
	CheckFail CheckStatus = "fail"
)

type Check struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail"`
}

type PreflightResult struct {
	SchemaVersion  string    `json:"schema_version"`
	TaskID         string    `json:"task_id"`
	WorkspaceID    string    `json:"workspace_id"`
	SourceRevision string    `json:"source_revision"`
	PolicySHA256   string    `json:"policy_sha256"`
	ConfigSHA256   string    `json:"config_sha256"`
	ObservedAt     time.Time `json:"observed_at"`
	Ready          bool      `json:"ready"`
	Checks         []Check   `json:"checks"`
}

func DefaultDeclaration() Declaration {
	return Declaration{
		SchemaVersion:     DeclarationSchemaVersion,
		Mode:              ModeOperatorAttended,
		ExternalIsolation: ExternalIsolation{Expectation: IsolationNone, Enforcement: EnforcementNone},
		Network:           NetworkPolicy{Access: NetworkUnknown, Enforcement: EnforcementNone},
		Hooks:             HookTrust{Policy: HooksOperatorAttended},
		Environment:       EnvironmentPolicy{InheritHost: true},
		Redaction:         redact.Policy{SchemaVersion: redact.PolicySchemaVersion},
	}
}

func (d Declaration) Validate() error {
	if d.SchemaVersion != DeclarationSchemaVersion {
		return fmt.Errorf("safety declaration: unsupported schema_version %q", d.SchemaVersion)
	}
	switch d.Mode {
	case ModeOperatorAttended, ModeFullyUnattended:
	default:
		return fmt.Errorf("safety declaration: unknown mode %q", d.Mode)
	}
	if err := d.ExternalIsolation.Validate(); err != nil {
		return fmt.Errorf("safety declaration: external isolation: %w", err)
	}
	if err := d.Network.Validate(); err != nil {
		return fmt.Errorf("safety declaration: network: %w", err)
	}
	if err := d.Hooks.Validate(); err != nil {
		return fmt.Errorf("safety declaration: hooks: %w", err)
	}
	if d.Environment.InheritHost && len(d.Environment.Allow) > 0 {
		return errors.New("safety declaration: environment allow list cannot be combined with inherit_host")
	}
	if err := validateOrderedStrings("environment allow", d.Environment.Allow); err != nil {
		return err
	}
	if _, err := d.Redaction.Normalize(); err != nil {
		return err
	}
	return nil
}

func (p ExternalIsolation) Validate() error {
	switch p.Expectation {
	case IsolationNone, IsolationContainer, IsolationOSSandbox:
	default:
		return fmt.Errorf("unknown expectation %q", p.Expectation)
	}
	if err := validateEnforcement(p.Enforcement); err != nil {
		return err
	}
	if p.Expectation == IsolationNone && (p.Enforcement != EnforcementNone || p.Attestation != nil) {
		return errors.New("none expectation cannot claim enforcement or attestation")
	}
	if p.Enforcement == EnforcementExternalAttestation {
		if p.Attestation == nil {
			return errors.New("external attestation evidence is required")
		}
		return p.Attestation.Validate()
	}
	if p.Attestation != nil {
		return errors.New("attestation is present without external_attestation enforcement")
	}
	return nil
}

func (p NetworkPolicy) Validate() error {
	switch p.Access {
	case NetworkUnknown, NetworkDenied, NetworkRestricted, NetworkUnrestricted:
	default:
		return fmt.Errorf("unknown access %q", p.Access)
	}
	if err := validateEnforcement(p.Enforcement); err != nil {
		return err
	}
	if p.Enforcement == EnforcementExternalAttestation {
		if p.Attestation == nil {
			return errors.New("external attestation evidence is required")
		}
		return p.Attestation.Validate()
	}
	if p.Attestation != nil {
		return errors.New("attestation is present without external_attestation enforcement")
	}
	return nil
}

func (p HookTrust) Validate() error {
	switch p.Policy {
	case HooksOperatorAttended, HooksDisabled, HooksTrusted:
	default:
		return fmt.Errorf("unknown policy %q", p.Policy)
	}
	if p.Policy != HooksTrusted && len(p.Trusted) > 0 {
		return errors.New("trusted hook identities require hooks policy trusted")
	}
	seen := map[string]struct{}{}
	for i, hook := range p.Trusted {
		if hook.Path == "" || !filepath.IsAbs(hook.Path) || filepath.Clean(hook.Path) != hook.Path || !validSHA256(hook.SHA256) {
			return fmt.Errorf("trusted[%d] requires a canonical absolute path and SHA-256", i)
		}
		if _, ok := seen[hook.Path]; ok {
			return fmt.Errorf("duplicate trusted hook %q", hook.Path)
		}
		seen[hook.Path] = struct{}{}
		if i > 0 && p.Trusted[i-1].Path > hook.Path {
			return errors.New("trusted hook identities must be ordered by path")
		}
	}
	return nil
}

func (a Attestation) Validate() error {
	if strings.TrimSpace(a.Authority) == "" || strings.TrimSpace(a.Evidence) == "" || strings.ContainsAny(a.Authority+a.Evidence, "\r\n") || !validSHA256(a.SHA256) {
		return errors.New("authority, single-line evidence, and SHA-256 are required")
	}
	return nil
}

func (p Policy) Validate() error {
	if p.SchemaVersion != PolicySchemaVersion || p.TaskID != p.Workspace.TaskID {
		return errors.New("safety policy: schema/task/workspace identity mismatch")
	}
	if err := p.Workspace.Validate(); err != nil {
		return fmt.Errorf("safety policy: %w", err)
	}
	if p.Workspace.Status != autonomous.WorkspaceStatusReady && p.Workspace.Status != autonomous.WorkspaceStatusRestored {
		return errors.New("safety policy: workspace is not ready or restored")
	}
	if p.WorktreeNotice != "Git worktree isolation is source/Git isolation, not a security sandbox." {
		return errors.New("safety policy: worktree security notice is required")
	}
	if !validSHA256(p.ConfigSHA256) || !validSHA256(p.RedactionPolicyHash) || !validSHA256(p.PolicySHA256) {
		return errors.New("safety policy: config, redaction, and policy SHA-256 identities are required")
	}
	copyPolicy := p
	copyPolicy.PolicySHA256 = ""
	copyPolicy.Acknowledgement = ""
	copyPolicy.ConfigSHA256 = ""
	identity, err := policyIdentity(copyPolicy)
	if err != nil {
		return err
	}
	if identity != p.PolicySHA256 {
		return errors.New("safety policy: policy SHA-256 does not match material")
	}
	return nil
}

func (p Policy) ExpectedAcknowledgement() string { return AcknowledgementPrefix + p.PolicySHA256 }

// FinalizePolicy assigns the deterministic identity over material policy
// fields. The acknowledgement is intentionally excluded because it authorizes
// the resulting identity rather than participating in a recursive hash.
func FinalizePolicy(policy Policy) (Policy, error) {
	policy.PolicySHA256 = ""
	material := policy
	material.Acknowledgement = ""
	material.ConfigSHA256 = ""
	identity, err := policyIdentity(material)
	if err != nil {
		return Policy{}, err
	}
	policy.PolicySHA256 = identity
	return policy, nil
}

func (p Policy) AuthorizeModelChanges(paths []string) error {
	for _, changed := range paths {
		clean := filepath.Clean(changed)
		if changed != clean || filepath.IsAbs(changed) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.ContainsAny(changed, "\x00\r\n") {
			return fmt.Errorf("model change path %q is malformed", changed)
		}
		absolute := filepath.Join(p.Workspace.ExecutionRoot, clean)
		for _, protected := range p.ProtectedPaths {
			if sameOrWithin(protected.Path, absolute) {
				return fmt.Errorf("model change %q targets protected %s path", changed, protected.Class)
			}
		}
	}
	return nil
}

func validateEnforcement(value Enforcement) error {
	if value != EnforcementNone && value != EnforcementExternalAttestation {
		return fmt.Errorf("unknown enforcement %q", value)
	}
	return nil
}
func validateOrderedStrings(label string, values []string) error {
	seen := map[string]struct{}{}
	for i, v := range values {
		if strings.TrimSpace(v) != v || v == "" || strings.ContainsAny(v, "\x00\r\n=") {
			return fmt.Errorf("safety declaration: %s[%d] is malformed", label, i)
		}
		if _, ok := seen[v]; ok {
			return fmt.Errorf("safety declaration: duplicate %s %q", label, v)
		}
		seen[v] = struct{}{}
	}
	return nil
}
func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
func policyIdentity(policy Policy) (string, error) {
	raw, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func sameOrWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)))
}
