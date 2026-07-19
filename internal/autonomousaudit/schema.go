package autonomousaudit

import (
	"encoding/json"
	"fmt"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousverification"
)

// AuditOutputSchema returns the deterministic auditor-only JSON Schema. The
// strict decoder and Go validators remain authoritative after execution.
func AuditOutputSchema() ([]byte, error) {
	nonblank := map[string]any{"type": "string", "pattern": `.*\S.*`}
	evidence := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind", "reference", "detail"},
		"properties": map[string]any{
			"kind": map[string]any{"type": "string", "enum": []string{
				"task", "plan", "ledger", "receipt", "verification", "git", "audit", "repository", "file",
			}},
			"reference": nonblank,
			"detail":    nonblank,
		},
	}
	report := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_id", "disposition", "rationale", "inputs", "findings"},
		"properties": map[string]any{
			"task_id":     nonblank,
			"disposition": map[string]any{"type": "string", "enum": []string{string(autonomous.AuditDispositionClean), string(autonomous.AuditDispositionChangesRequired)}},
			"rationale":   nonblank,
			"inputs":      map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence"}},
			"findings":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/finding"}},
		},
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"schema_version", "task_id", "report", "provenance"},
		"properties": map[string]any{
			"schema_version": auditSingletonString(AuditOutputSchemaVersion),
			"task_id":        nonblank,
			"report":         map[string]any{"$ref": "#/$defs/report"},
			"provenance":     map[string]any{"$ref": "#/$defs/provenance"},
		},
		"$defs": map[string]any{
			"evidence": evidence,
			"finding": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"id", "significance", "summary", "evidence", "required_correction"},
				"properties": map[string]any{
					"id":                  stableID(),
					"significance":        map[string]any{"type": "string", "enum": []string{"blocking", "non_blocking"}},
					"summary":             nonblank,
					"evidence":            map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence"}},
					"required_correction": nonblank,
				},
			},
			"report": report,
			"decision": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"decision_id", "run_id", "task_id", "action", "worker_profile", "artifact", "created_at"},
				"properties": map[string]any{
					"decision_id":    stableID(),
					"run_id":         nonblank,
					"task_id":        nonblank,
					"action":         auditSingletonString(string(autonomous.ActionAudit)),
					"worker_profile": auditSingletonString(string(autonomous.WorkerProfileAuditor)),
					"artifact":       map[string]any{"$ref": "#/$defs/evidence"},
					"created_at":     map[string]any{"type": "string", "format": "date-time"},
				},
			},
			"verification_summary": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"task_id", "status", "command", "summary", "run_id", "occurrence_id", "evidence", "tiered"},
				"properties": map[string]any{
					"task_id":       nonblank,
					"status":        auditSingletonString(string(autonomous.VerificationStatusPassed)),
					"command":       map[string]any{"type": []string{"string", "null"}},
					"summary":       nonblank,
					"run_id":        nonblank,
					"occurrence_id": nonblank,
					"evidence":      map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence"}},
					"tiered":        map[string]any{"type": "null"},
				},
			},
			"verification": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"summary", "source_revision", "tiered"},
				"properties": map[string]any{
					"summary":         map[string]any{"$ref": "#/$defs/verification_summary"},
					"source_revision": hashSchema(),
					"tiered":          auditNullableRef("#/$defs/verification_gate"),
				},
			},
			"verification_gate": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"schema_version", "plan", "purpose", "required_final_tiers", "selected_tiers", "executed_tiers", "required_outcomes", "missing_required", "overall_outcome", "final_satisfied"},
				"properties": map[string]any{
					"schema_version":       auditSingletonString(autonomousverification.GateSchemaVersion),
					"plan":                 map[string]any{"$ref": "#/$defs/verification_plan_identity"},
					"purpose":              auditSingletonString(string(autonomousverification.PurposeFinal)),
					"required_final_tiers": tierIDs(),
					"selected_tiers":       tierIDs(),
					"executed_tiers":       tierIDs(),
					"required_outcomes":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/verification_tier_gate"}},
					"missing_required":     tierIDs(),
					"overall_outcome":      auditSingletonString(string(autonomousverification.OutcomePassed)),
					"final_satisfied":      map[string]any{"type": "boolean", "enum": []bool{true}},
				},
			},
			"verification_plan_identity": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"schema_version", "sha256", "byte_size"},
				"properties": map[string]any{
					"schema_version": auditSingletonString(autonomousverification.PlanSchemaVersion),
					"sha256":         hashSchema(),
					"byte_size":      map[string]any{"type": "integer", "minimum": 1},
				},
			},
			"verification_tier_gate": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"tier_id", "outcome"},
				"properties": map[string]any{
					"tier_id": stableID(),
					"outcome": map[string]any{"type": "string", "enum": auditOutcomes()},
				},
			},
			"mutation": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"task_id", "run_id", "decision_id", "action", "resulting_revision"},
				"properties": map[string]any{
					"task_id":            nonblank,
					"run_id":             nonblank,
					"decision_id":        map[string]any{"type": []string{"string", "null"}, "pattern": `.*\S.*`},
					"action":             map[string]any{"type": "string", "enum": []string{"implement", "correct", "document", "simplify"}},
					"resulting_revision": hashSchema(),
				},
			},
			"provenance": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"action", "worker_profile", "worker_run_id", "decision_reference", "dossier", "profile", "raw_output_path", "source_revision", "verification", "latest_source_mutation"},
				"properties": map[string]any{
					"action":                 auditSingletonString(string(autonomous.ActionAudit)),
					"worker_profile":         auditSingletonString(string(autonomous.WorkerProfileAuditor)),
					"worker_run_id":          nonblank,
					"decision_reference":     map[string]any{"$ref": "#/$defs/decision"},
					"dossier":                identityObject(nonblank, false),
					"profile":                identityObject(nonblank, true),
					"raw_output_path":        nonblank,
					"source_revision":        hashSchema(),
					"verification":           map[string]any{"$ref": "#/$defs/verification"},
					"latest_source_mutation": auditNullableRef("#/$defs/mutation"),
				},
			},
		},
	}
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal audit output schema: %w", err)
	}
	return append(raw, '\n'), nil
}

func stableID() map[string]any {
	return map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}
}

func hashSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`}
}

func tierIDs() map[string]any {
	return map[string]any{"type": "array", "items": stableID()}
}

func identityObject(nonblank map[string]any, profile bool) map[string]any {
	properties := map[string]any{
		"schema_version": nonblank,
		"task_id":        nonblank,
		"sha256":         hashSchema(),
		"byte_size":      map[string]any{"type": "integer", "minimum": 1},
	}
	required := []string{"schema_version", "task_id", "sha256", "byte_size"}
	if profile {
		properties = map[string]any{
			"name":      auditSingletonString(string(autonomous.WorkerProfileAuditor)),
			"path":      nonblank,
			"sha256":    hashSchema(),
			"byte_size": map[string]any{"type": "integer", "minimum": 1},
		}
		required = []string{"name", "path", "sha256", "byte_size"}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}

func auditNullableRef(ref string) map[string]any {
	return map[string]any{"anyOf": []any{map[string]any{"$ref": ref}, map[string]any{"type": "null"}}}
}

func auditSingletonString(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}

func auditOutcomes() []string {
	return []string{
		string(autonomousverification.OutcomePassed),
		string(autonomousverification.OutcomeFailed),
		string(autonomousverification.OutcomeFlaky),
		string(autonomousverification.OutcomeMissing),
		string(autonomousverification.OutcomeTimedOut),
		string(autonomousverification.OutcomeCancelled),
		string(autonomousverification.OutcomeRunnerError),
		string(autonomousverification.OutcomeConfigurationError),
		string(autonomousverification.OutcomeLedgerError),
		string(autonomousverification.OutcomeArtifactError),
	}
}
