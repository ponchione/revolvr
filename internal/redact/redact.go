// Package redact provides deterministic configured-secret redaction.
package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	PolicySchemaVersion = "revolvr-secret-redaction-policy-v1"
	Replacement         = "[REDACTED]"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Policy struct {
	SchemaVersion        string   `json:"schema_version"`
	EnvironmentVariables []string `json:"environment_variables,omitempty"`
}

type Facts struct {
	PolicySHA256 string `json:"policy_sha256"`
	SourceCount  int    `json:"source_count"`
	MatchCount   int    `json:"match_count"`
}

type LookupEnv func(string) (string, bool)

type Redactor struct {
	policySHA256 string
	values       []string
}

func (p Policy) Normalize() (Policy, error) {
	if p.SchemaVersion == "" {
		p.SchemaVersion = PolicySchemaVersion
	}
	if p.SchemaVersion != PolicySchemaVersion {
		return Policy{}, fmt.Errorf("redaction policy: unsupported schema_version %q", p.SchemaVersion)
	}
	seen := make(map[string]struct{}, len(p.EnvironmentVariables))
	names := make([]string, 0, len(p.EnvironmentVariables))
	for i, raw := range p.EnvironmentVariables {
		name := strings.TrimSpace(raw)
		if name != raw || !environmentName.MatchString(name) {
			return Policy{}, fmt.Errorf("redaction policy: environment_variables[%d] is malformed", i)
		}
		if _, ok := seen[name]; ok {
			return Policy{}, fmt.Errorf("redaction policy: duplicate environment variable %q", name)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	p.EnvironmentVariables = names
	return p, nil
}

func (p Policy) Identity() (string, error) {
	normalized, err := p.Normalize()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("redaction policy: marshal identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func New(policy Policy, lookup LookupEnv) (*Redactor, Facts, error) {
	normalized, err := policy.Normalize()
	if err != nil {
		return nil, Facts{}, err
	}
	identity, err := normalized.Identity()
	if err != nil {
		return nil, Facts{}, err
	}
	if lookup == nil && len(normalized.EnvironmentVariables) > 0 {
		return nil, Facts{}, errors.New("redaction policy: environment lookup is required")
	}
	values := make([]string, 0, len(normalized.EnvironmentVariables))
	seenValues := map[string]struct{}{}
	for _, name := range normalized.EnvironmentVariables {
		value, ok := lookup(name)
		if !ok || value == "" {
			return nil, Facts{}, fmt.Errorf("redaction policy: configured environment variable %q is missing or empty", name)
		}
		if len(value) < 4 {
			return nil, Facts{}, fmt.Errorf("redaction policy: configured environment variable %q is too short to redact safely", name)
		}
		if _, ok := seenValues[value]; ok {
			continue
		}
		seenValues[value] = struct{}{}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) == len(values[j]) {
			return values[i] < values[j]
		}
		return len(values[i]) > len(values[j])
	})
	return &Redactor{policySHA256: identity, values: values}, Facts{PolicySHA256: identity, SourceCount: len(normalized.EnvironmentVariables)}, nil
}

func (r *Redactor) Redact(value string) (string, Facts) {
	facts := Facts{}
	if r == nil {
		return value, facts
	}
	facts.PolicySHA256 = r.policySHA256
	facts.SourceCount = len(r.values)
	for _, secret := range r.values {
		count := strings.Count(value, secret)
		if count == 0 {
			continue
		}
		facts.MatchCount += count
		value = strings.ReplaceAll(value, secret, Replacement)
	}
	return value, facts
}

func (r *Redactor) String(value string) string {
	redacted, _ := r.Redact(value)
	return redacted
}

func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(r.String(err.Error()))
}
