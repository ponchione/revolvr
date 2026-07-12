package redact

import (
	"strings"
	"testing"
)

func TestRedactorOverlappingRepeatedValues(t *testing.T) {
	policy := Policy{SchemaVersion: PolicySchemaVersion, EnvironmentVariables: []string{"TOKEN", "TOKEN_PREFIX"}}
	values := map[string]string{"TOKEN": "abcd", "TOKEN_PREFIX": "abcdef"}
	r, facts, err := New(policy, func(name string) (string, bool) { value, ok := values[name]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	got, redaction := r.Redact(`{"one":"abcdef","two":"abcd-abcd"}`)
	if strings.Contains(got, "abcd") || got != `{"one":"[REDACTED]","two":"[REDACTED]-[REDACTED]"}` {
		t.Fatalf("redacted = %q", got)
	}
	if facts.PolicySHA256 == "" || redaction.MatchCount != 3 || redaction.SourceCount != 2 {
		t.Fatalf("facts = %+v / %+v", facts, redaction)
	}
}

func TestPolicyRejectsUnsafeSourcesWithoutPrintingValues(t *testing.T) {
	tests := []Policy{
		{SchemaVersion: "unknown"},
		{EnvironmentVariables: []string{"BAD-NAME"}},
		{EnvironmentVariables: []string{"TOKEN", "TOKEN"}},
	}
	for _, policy := range tests {
		if _, _, err := New(policy, func(string) (string, bool) { return "super-secret", true }); err == nil || strings.Contains(err.Error(), "super-secret") {
			t.Fatalf("New(%+v) error = %v", policy, err)
		}
	}
}
