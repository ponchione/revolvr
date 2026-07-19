package autonomousplanning

import (
	"encoding/json"
	"fmt"

	"revolvr/internal/autonomous"
)

// PlanningOutputSchema returns the deterministic planner-only JSON Schema.
// ParsePlanningOutput and the Go validators remain authoritative after Codex.
func PlanningOutputSchema() ([]byte, error) {
	nonblank := `.*\S.*`
	evidenceKinds := []string{
		string(autonomous.EvidenceKindTask), string(autonomous.EvidenceKindPlan),
		string(autonomous.EvidenceKindLedger), string(autonomous.EvidenceKindReceipt),
		string(autonomous.EvidenceKindVerification), string(autonomous.EvidenceKindGit),
		string(autonomous.EvidenceKindAudit), string(autonomous.EvidenceKindRepository),
		string(autonomous.EvidenceKindFile),
	}
	acceptanceStatuses := []string{
		string(autonomous.AcceptanceStatusPending), string(autonomous.AcceptanceStatusSatisfied),
		string(autonomous.AcceptanceStatusWaived), string(autonomous.AcceptanceStatusNotApplicable),
	}
	stepStatuses := []string{
		string(autonomous.PlanStepStatusPending), string(autonomous.PlanStepStatusInProgress),
		string(autonomous.PlanStepStatusCompleted), string(autonomous.PlanStepStatusSkipped),
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"schema_version", "task_id", "plan", "acceptance_criteria", "inputs", "provenance"},
		"properties": map[string]any{
			"schema_version": singletonString(PlanningOutputSchemaVersion),
			"task_id":        nonblankString(nonblank),
			"plan":           map[string]any{"$ref": "#/$defs/task_plan"},
			"acceptance_criteria": map[string]any{
				"type": "array", "minItems": 1,
				"items": map[string]any{"$ref": "#/$defs/acceptance_criterion"},
			},
			"inputs": map[string]any{
				"type": "array", "minItems": 1,
				"items": map[string]any{"$ref": "#/$defs/evidence_reference"},
			},
			"provenance": map[string]any{"$ref": "#/$defs/planning_provenance"},
		},
		"$defs": map[string]any{
			"evidence_reference": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"kind", "reference", "detail"},
				"properties": map[string]any{
					"kind":      map[string]any{"type": "string", "enum": evidenceKinds},
					"reference": nonblankString(nonblank),
					"detail":    nonblankString(nonblank),
				},
			},
			"plan_step": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"id", "description", "status", "evidence", "rationale"},
				"properties": map[string]any{
					"id":          stableIDSchema(),
					"description": nonblankString(nonblank),
					"status":      map[string]any{"type": "string", "enum": stepStatuses},
					"evidence": map[string]any{
						"type": "array", "items": map[string]any{"$ref": "#/$defs/evidence_reference"},
					},
					"rationale": nullablePatternString(nonblank),
				},
			},
			"task_plan": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"task_id", "id", "revision", "supersedes_plan_id", "provenance", "steps", "completed"},
				"properties": map[string]any{
					"task_id":            nonblankString(nonblank),
					"id":                 stableIDSchema(),
					"revision":           map[string]any{"type": "integer", "minimum": 1},
					"supersedes_plan_id": nullablePatternString(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`),
					"provenance": map[string]any{
						"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"},
					},
					"steps": map[string]any{
						"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/plan_step"},
					},
					"completed": map[string]any{"type": "boolean"},
				},
			},
			"acceptance_criterion": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"id", "requirement", "status", "evidence", "rationale", "source"},
				"properties": map[string]any{
					"id":          stableIDSchema(),
					"requirement": nonblankString(nonblank),
					"status":      map[string]any{"type": "string", "enum": acceptanceStatuses},
					"evidence": map[string]any{
						"type": "array", "items": map[string]any{"$ref": "#/$defs/evidence_reference"},
					},
					"rationale": nullablePatternString(nonblank),
					"source":    map[string]any{"$ref": "#/$defs/evidence_reference"},
				},
			},
			"decision_reference": decisionReferenceSchema(nonblank),
			"dossier_identity": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"schema_version", "task_id", "sha256", "byte_size"},
				"properties": map[string]any{
					"schema_version": nonblankString(nonblank), "task_id": nonblankString(nonblank),
					"sha256": hashSchema(), "byte_size": map[string]any{"type": "integer", "minimum": 1},
				},
			},
			"profile_identity": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"name", "path", "sha256", "byte_size"},
				"properties": map[string]any{
					"name": singletonString(string(autonomous.WorkerProfilePlanner)),
					"path": nonblankString(nonblank), "sha256": hashSchema(),
					"byte_size": map[string]any{"type": "integer", "minimum": 1},
				},
			},
			"planning_provenance": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"action", "worker_profile", "worker_run_id", "decision_reference", "dossier", "profile", "raw_output_path", "source_revision"},
				"properties": map[string]any{
					"action":             singletonString(string(autonomous.ActionPlan)),
					"worker_profile":     singletonString(string(autonomous.WorkerProfilePlanner)),
					"worker_run_id":      nonblankString(nonblank),
					"decision_reference": map[string]any{"$ref": "#/$defs/decision_reference"},
					"dossier":            map[string]any{"$ref": "#/$defs/dossier_identity"},
					"profile":            map[string]any{"$ref": "#/$defs/profile_identity"},
					"raw_output_path":    nonblankString(nonblank),
					"source_revision":    hashSchema(),
				},
			},
		},
	}
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal planning output schema: %w", err)
	}
	return append(raw, '\n'), nil
}

func nonblankString(pattern string) map[string]any {
	return map[string]any{"type": "string", "pattern": pattern}
}

func nullablePatternString(pattern string) map[string]any {
	return map[string]any{"type": []string{"string", "null"}, "pattern": pattern}
}

func stableIDSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}
}

func hashSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`}
}

func decisionReferenceSchema(nonblank string) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"decision_id", "run_id", "task_id", "action", "worker_profile", "artifact", "created_at"},
		"properties": map[string]any{
			"decision_id": stableIDSchema(), "run_id": nonblankString(nonblank), "task_id": nonblankString(nonblank),
			"action":         singletonString(string(autonomous.ActionPlan)),
			"worker_profile": singletonString(string(autonomous.WorkerProfilePlanner)),
			"artifact":       map[string]any{"$ref": "#/$defs/evidence_reference"},
			"created_at":     map[string]any{"type": "string", "format": "date-time"},
		},
	}
}

func singletonString(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}
