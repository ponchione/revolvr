package autonomousaudit

import (
	"encoding/json"
	"fmt"

	"revolvr/internal/autonomous"
)

const auditSchemaID = "https://revolvr.local/schemas/autonomous-audit-output-v1.json"

// AuditOutputSchema returns the deterministic auditor-only JSON Schema. The
// strict decoder and Go validators remain authoritative after execution.
func AuditOutputSchema() ([]byte, error) {
	nonblank := map[string]any{"type": "string", "minLength": 1, "pattern": `.*\S.*`}
	evidence := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"kind", "reference", "detail"},
		"properties": map[string]any{
			"kind":      map[string]any{"type": "string", "enum": []string{"task", "plan", "ledger", "receipt", "verification", "git", "audit", "repository", "file"}},
			"reference": nonblank, "detail": nonblank,
		},
	}
	report := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"task_id", "disposition", "rationale", "inputs"},
		"properties": map[string]any{
			"task_id":     nonblank,
			"disposition": map[string]any{"type": "string", "enum": []string{string(autonomous.AuditDispositionClean), string(autonomous.AuditDispositionChangesRequired)}},
			"rationale":   nonblank,
			"inputs":      map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence"}},
			"findings":    map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"$ref": "#/$defs/finding"}},
		},
	}
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": auditSchemaID,
		"title": "Revolvr AuditOutput", "type": "object", "additionalProperties": false,
		"required": []string{"schema_version", "task_id", "report", "provenance"},
		"properties": map[string]any{
			"schema_version": map[string]any{"const": AuditOutputSchemaVersion}, "task_id": nonblank,
			"report": map[string]any{"$ref": "#/$defs/report"}, "provenance": map[string]any{"$ref": "#/$defs/provenance"},
		},
		"$defs": map[string]any{
			"evidence": evidence,
			"finding": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"id", "significance", "summary", "evidence", "required_correction"},
				"properties": map[string]any{
					"id": stableID(), "significance": map[string]any{"type": "string", "enum": []string{"blocking", "non_blocking"}},
					"summary": nonblank, "evidence": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence"}}, "required_correction": nonblank,
				},
			},
			"report": report,
			"decision": map[string]any{
				"type": "object", "additionalProperties": false,
				"required":   []string{"decision_id", "run_id", "task_id", "action", "worker_profile", "artifact", "created_at"},
				"properties": map[string]any{"decision_id": stableID(), "run_id": nonblank, "task_id": nonblank, "action": map[string]any{"const": "audit"}, "worker_profile": map[string]any{"const": "auditor"}, "artifact": map[string]any{"$ref": "#/$defs/evidence"}, "created_at": map[string]any{"type": "string", "format": "date-time"}},
			},
			"verification_summary": map[string]any{
				"type": "object", "additionalProperties": false,
				"required":   []string{"task_id", "status", "summary", "run_id", "occurrence_id", "evidence"},
				"properties": map[string]any{"task_id": nonblank, "status": map[string]any{"const": "passed"}, "command": map[string]any{"type": "string"}, "summary": nonblank, "run_id": nonblank, "occurrence_id": nonblank, "evidence": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence"}}, "tiered": map[string]any{"type": "object"}},
			},
			"verification": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"summary", "source_revision"},
				"properties": map[string]any{"summary": map[string]any{"$ref": "#/$defs/verification_summary"}, "source_revision": hashSchema(), "tiered": map[string]any{"$ref": "#/$defs/verification_gate"}},
			},
			"verification_gate": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"schema_version", "plan", "purpose", "required_final_tiers", "selected_tiers", "executed_tiers", "required_outcomes", "missing_required", "overall_outcome", "final_satisfied"},
				"properties": map[string]any{
					"schema_version":       map[string]any{"const": "autonomous-verification-gate-v1"},
					"plan":                 map[string]any{"$ref": "#/$defs/verification_plan_identity"},
					"purpose":              map[string]any{"const": "final"},
					"required_final_tiers": tierIDs(), "selected_tiers": tierIDs(), "executed_tiers": tierIDs(), "missing_required": tierIDs(),
					"required_outcomes": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/verification_tier_gate"}},
					"overall_outcome":   map[string]any{"const": "passed"}, "final_satisfied": map[string]any{"const": true},
				},
			},
			"verification_plan_identity": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"schema_version", "sha256", "byte_size"},
				"properties": map[string]any{"schema_version": map[string]any{"const": "autonomous-verification-plan-v1"}, "sha256": hashSchema(), "byte_size": map[string]any{"type": "integer", "minimum": 1}},
			},
			"verification_tier_gate": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"tier_id", "outcome"},
				"properties": map[string]any{"tier_id": stableID(), "outcome": map[string]any{"type": "string", "enum": []string{"passed", "failed", "flaky", "missing", "timed_out", "cancelled", "runner_error", "configuration_error", "ledger_error", "artifact_error"}}},
			},
			"mutation": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"task_id", "run_id", "action", "resulting_revision"},
				"properties": map[string]any{"task_id": nonblank, "run_id": nonblank, "decision_id": map[string]any{"type": "string"}, "action": map[string]any{"type": "string", "enum": []string{"implement", "correct", "document", "simplify"}}, "resulting_revision": hashSchema()},
			},
			"provenance": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"action", "worker_profile", "worker_run_id", "decision_reference", "dossier", "profile", "raw_output_path", "source_revision", "verification"},
				"properties": map[string]any{
					"action": map[string]any{"const": "audit"}, "worker_profile": map[string]any{"const": "auditor"}, "worker_run_id": nonblank,
					"decision_reference": map[string]any{"$ref": "#/$defs/decision"},
					"dossier":            identityObject(nonblank, false), "profile": identityObject(nonblank, true),
					"raw_output_path": nonblank, "source_revision": hashSchema(), "verification": map[string]any{"$ref": "#/$defs/verification"}, "latest_source_mutation": map[string]any{"$ref": "#/$defs/mutation"},
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
	return map[string]any{"type": "array", "uniqueItems": true, "items": stableID()}
}
func identityObject(nonblank map[string]any, profile bool) map[string]any {
	properties := map[string]any{"schema_version": nonblank, "task_id": nonblank, "sha256": hashSchema(), "byte_size": map[string]any{"type": "integer", "minimum": 1}}
	required := []string{"schema_version", "task_id", "sha256", "byte_size"}
	if profile {
		properties = map[string]any{"name": map[string]any{"const": "auditor"}, "path": nonblank, "sha256": hashSchema(), "byte_size": map[string]any{"type": "integer", "minimum": 1}}
		required = []string{"name", "path", "sha256", "byte_size"}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}
