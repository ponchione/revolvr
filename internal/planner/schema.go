package planner

import "encoding/json"

func OutputSchema() ([]byte, error) {
	stable := map[string]any{"type": "string", "pattern": `^[^\s]+$`}
	nonblank := map[string]any{"type": "string", "pattern": `.*\S.*`}
	sha := map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`}
	stringArray := func(min int) map[string]any { return map[string]any{"type": "array", "minItems": min, "items": stable} }
	object := func(required []string, properties map[string]any) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
	}
	nullable := func(ref string) map[string]any {
		return map[string]any{"anyOf": []any{map[string]any{"$ref": ref}, map[string]any{"type": "null"}}}
	}
	nullableString := map[string]any{"type": []string{"string", "null"}, "pattern": `^[^\s]+$`}
	nullableSHA := map[string]any{"type": []string{"string", "null"}, "pattern": `^[a-f0-9]{64}$`}

	root := object([]string{"revolvr_identity", "schema_version", "revision_identity", "change_explanation", "task_dependency_ids", "steps", "risks", "assumptions", "evidence_refs"}, map[string]any{
		"revolvr_identity":   map[string]any{"$ref": "#/$defs/revolvr_identity"},
		"schema_version":     map[string]any{"type": "string", "enum": []string{OutputSchemaVersion}},
		"revision_identity":  map[string]any{"$ref": "#/$defs/revision_identity"},
		"change_explanation": nonblank, "task_dependency_ids": stringArray(0),
		"steps": map[string]any{"type": "array", "minItems": 1, "maxItems": MaximumSteps, "items": map[string]any{"$ref": "#/$defs/step"}},
		"risks": stringArray(0), "assumptions": stringArray(0), "evidence_refs": stringArray(1),
	})
	root["$defs"] = map[string]any{
		"revolvr_identity": object([]string{"request_id", "task_id", "run_id", "source_revision", "prompt_version", "prompt_sha256", "response_schema_version", "response_schema_sha256"}, map[string]any{
			"request_id": stable, "task_id": stable, "run_id": stable, "source_revision": sha, "prompt_version": stable, "prompt_sha256": sha, "response_schema_version": stable, "response_schema_sha256": sha,
		}),
		"revision_identity": object([]string{"plan_id", "plan_version_id", "revision_number", "supersedes_plan_version_id", "task_id", "task_version_id", "task_version_number", "run_id", "project_source_id", "source_revision", "supervisor_decision_id", "supervisor_decision_sha256", "dossier_version", "dossier_sha256", "prompt_version", "prompt_sha256", "response_schema_version", "response_schema_sha256", "model_policy_version", "model_policy_sha256", "host_policy_version", "host_policy_sha256", "content_sha256"}, map[string]any{
			"plan_id": stable, "plan_version_id": stable, "revision_number": map[string]any{"type": "integer", "minimum": 1}, "supersedes_plan_version_id": nullableString,
			"task_id": stable, "task_version_id": stable, "task_version_number": map[string]any{"type": "integer", "minimum": 1}, "run_id": stable, "project_source_id": stable, "source_revision": sha,
			"supervisor_decision_id": stable, "supervisor_decision_sha256": sha, "dossier_version": stable, "dossier_sha256": sha, "prompt_version": stable, "prompt_sha256": sha,
			"response_schema_version": stable, "response_schema_sha256": sha, "model_policy_version": stable, "model_policy_sha256": sha, "host_policy_version": stable, "host_policy_sha256": sha, "content_sha256": nullableSHA,
		}),
		"test_strategy": object([]string{"criterion_id", "method", "reference"}, map[string]any{"criterion_id": stable, "method": map[string]any{"type": "string", "enum": []string{"command", "operator_checkpoint"}}, "reference": nonblank}),
		"lineage":       object([]string{"prior_plan_version_id", "prior_step_id", "prior_status", "transition_evidence"}, map[string]any{"prior_plan_version_id": stable, "prior_step_id": stable, "prior_status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "skipped"}}, "transition_evidence": map[string]any{"type": "string"}}),
		"step": object([]string{"id", "ordinal", "status", "description", "criterion_ids", "depends_on_step_ids", "expected_paths", "components", "test_strategy", "risks", "assumptions", "evidence_refs", "lineage"}, map[string]any{
			"id": stable, "ordinal": map[string]any{"type": "integer", "minimum": 1, "maximum": MaximumSteps}, "status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "skipped"}}, "description": nonblank,
			"criterion_ids": stringArray(1), "depends_on_step_ids": stringArray(0), "expected_paths": stringArray(1), "components": stringArray(1),
			"test_strategy": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/test_strategy"}}, "risks": stringArray(0), "assumptions": stringArray(0), "evidence_refs": stringArray(1), "lineage": nullable("#/$defs/lineage"),
		}),
	}
	return json.Marshal(root)
}
