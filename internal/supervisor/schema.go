package supervisor

import (
	"encoding/json"
	"fmt"

	"revolvr/internal/autonomous"
)

const decisionSchemaID = "https://revolvr.local/schemas/supervisor-decision-v1.json"

type actionProfile struct {
	action  autonomous.Action
	profile autonomous.WorkerProfile
}

var workerActionProfiles = []actionProfile{
	{autonomous.ActionPlan, autonomous.WorkerProfilePlanner},
	{autonomous.ActionImplement, autonomous.WorkerProfileImplementer},
	{autonomous.ActionAudit, autonomous.WorkerProfileAuditor},
	{autonomous.ActionCorrect, autonomous.WorkerProfileCorrector},
	{autonomous.ActionDocument, autonomous.WorkerProfileDocumentor},
	{autonomous.ActionSimplify, autonomous.WorkerProfileSimplifier},
}

// DecisionOutputSchema returns the deterministic JSON Schema supplied to
// Codex. SupervisorDecision.Validate remains authoritative after decoding.
func DecisionOutputSchema() ([]byte, error) {
	actions := []string{
		string(autonomous.ActionPlan),
		string(autonomous.ActionImplement),
		string(autonomous.ActionAudit),
		string(autonomous.ActionCorrect),
		string(autonomous.ActionDocument),
		string(autonomous.ActionSimplify),
		string(autonomous.ActionComplete),
		string(autonomous.ActionBlock),
		string(autonomous.ActionNeedsInput),
	}
	profiles := make([]string, 0, len(workerActionProfiles))
	branches := make([]any, 0, len(actions))
	for _, pair := range workerActionProfiles {
		profiles = append(profiles, string(pair.profile))
		branch := map[string]any{
			"properties": map[string]any{
				"action":         map[string]any{"const": string(pair.action)},
				"worker_profile": map[string]any{"const": string(pair.profile)},
				"success_criteria": map[string]any{
					"minItems": 1,
				},
			},
			"required": []string{"worker_profile", "success_criteria"},
		}
		if pair.action == autonomous.ActionCorrect {
			branch["oneOf"] = []any{map[string]any{"required": []string{"finding_ids"}, "not": map[string]any{"required": []string{"verification_failure"}}}, map[string]any{"required": []string{"verification_failure"}, "not": map[string]any{"required": []string{"finding_ids"}}}}
			branch["allOf"] = []any{map[string]any{"not": map[string]any{"required": []string{"needs_input"}}}, map[string]any{"not": map[string]any{"required": []string{"child_tasks"}}}}
		} else {
			branch["allOf"] = []any{map[string]any{"not": map[string]any{"required": []string{"finding_ids"}}}, map[string]any{"not": map[string]any{"required": []string{"verification_failure"}}}, map[string]any{"not": map[string]any{"required": []string{"needs_input"}}}, map[string]any{"not": map[string]any{"required": []string{"child_tasks"}}}}
		}
		branches = append(branches, branch)
	}
	for _, action := range []autonomous.Action{autonomous.ActionComplete, autonomous.ActionBlock} {
		allOf := []any{
			map[string]any{"not": map[string]any{"required": []string{"worker_profile"}}},
			map[string]any{"not": map[string]any{"required": []string{"finding_ids"}}},
			map[string]any{"not": map[string]any{"required": []string{"verification_failure"}}},
			map[string]any{"not": map[string]any{"required": []string{"strategy"}}},
			map[string]any{"not": map[string]any{"required": []string{"needs_input"}}},
		}
		if action == autonomous.ActionComplete {
			allOf = append(allOf, map[string]any{"not": map[string]any{"required": []string{"child_tasks"}}})
		}
		branches = append(branches, map[string]any{
			"properties": map[string]any{"action": map[string]any{"const": string(action)}},
			"allOf":      allOf,
		})
	}
	branches = append(branches, map[string]any{
		"properties": map[string]any{"action": map[string]any{"const": string(autonomous.ActionNeedsInput)}},
		"required":   []string{"needs_input"},
		"allOf": []any{
			map[string]any{"not": map[string]any{"required": []string{"worker_profile"}}},
			map[string]any{"not": map[string]any{"required": []string{"success_criteria"}}},
			map[string]any{"not": map[string]any{"required": []string{"finding_ids"}}},
			map[string]any{"not": map[string]any{"required": []string{"verification_failure"}}},
			map[string]any{"not": map[string]any{"required": []string{"strategy"}}},
		},
	})

	nonblank := `.*\S.*`
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  decisionSchemaID,
		"title":                "Revolvr SupervisorDecision",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"task_id", "action", "rationale", "inputs"},
		"properties": map[string]any{
			"task_id":        map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
			"action":         map[string]any{"type": "string", "enum": actions},
			"worker_profile": map[string]any{"type": "string", "enum": profiles},
			"rationale":      map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
			"success_criteria": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
			},
			"inputs": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    map[string]any{"$ref": "#/$defs/evidence_reference"},
			},
			"finding_ids": map[string]any{
				"type":        "array",
				"minItems":    1,
				"uniqueItems": true,
				"items":       map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
			},
			"verification_failure": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"task_id", "run_id", "occurrence_id", "source_revision", "status", "evidence"},
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "minLength": 1}, "run_id": map[string]any{"type": "string", "minLength": 1}, "occurrence_id": map[string]any{"type": "string", "minLength": 1}, "source_revision": map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}, "status": map[string]any{"const": string(autonomous.VerificationStatusFailed)}, "evidence": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"strategy": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"approach"},
				"properties": map[string]any{
					"approach":   map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"techniques": map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank}},
					"targets":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"needs_input": map[string]any{"$ref": "#/$defs/needs_input_question"},
			"child_tasks": map[string]any{"$ref": "#/$defs/child_task_set"},
		},
		"oneOf": branches,
		"$defs": map[string]any{
			"evidence_reference": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"kind", "reference", "detail"},
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": []string{
						string(autonomous.EvidenceKindTask),
						string(autonomous.EvidenceKindPlan),
						string(autonomous.EvidenceKindLedger),
						string(autonomous.EvidenceKindReceipt),
						string(autonomous.EvidenceKindVerification),
						string(autonomous.EvidenceKindGit),
						string(autonomous.EvidenceKindAudit),
						string(autonomous.EvidenceKindRepository),
						string(autonomous.EvidenceKindFile),
					}},
					"reference": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"detail":    map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
				},
			},
			"needs_input_option": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"id", "meaning"},
				"properties": map[string]any{"id": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}, "meaning": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank}},
			},
			"independent_work": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"id", "action", "worker_profile", "description", "source_effect", "independent_of_option_ids", "inputs"},
				"properties": map[string]any{
					"id":                        map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"action":                    map[string]any{"type": "string", "enum": []string{string(autonomous.ActionPlan), string(autonomous.ActionAudit)}},
					"worker_profile":            map[string]any{"type": "string", "enum": []string{string(autonomous.WorkerProfilePlanner), string(autonomous.WorkerProfileAuditor)}},
					"description":               map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"source_effect":             map[string]any{"const": string(autonomous.InputSourceEffectReadOnly)},
					"independent_of_option_ids": map[string]any{"type": "array", "minItems": 2, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}},
					"inputs":                    map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"needs_input_question": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"task_id", "question_id", "revision", "question", "blocking_reason", "options", "recommendation", "evidence"},
				"properties": map[string]any{
					"task_id":          map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"question_id":      map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"revision":         map[string]any{"type": "integer", "minimum": 1},
					"content_sha256":   map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`},
					"question":         map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"blocking_reason":  map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"options":          map[string]any{"type": "array", "minItems": 2, "uniqueItems": true, "items": map[string]any{"$ref": "#/$defs/needs_input_option"}},
					"recommendation":   map[string]any{"type": "object", "additionalProperties": false, "required": []string{"option_id", "rationale"}, "properties": map[string]any{"option_id": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}, "rationale": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank}}},
					"evidence":         map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
					"independent_work": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"$ref": "#/$defs/independent_work"}},
				},
			},
			"child_task": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"key", "title", "scope", "success_criteria", "parent_behavior", "evidence"},
				"properties": map[string]any{
					"key":              map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"title":            map[string]any{"type": "string", "minLength": 1, "maxLength": 160, "pattern": nonblank},
					"scope":            map[string]any{"type": "string", "minLength": 1, "maxLength": 8192, "pattern": nonblank},
					"success_criteria": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank}},
					"depends_on":       map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_-]+$`}},
					"tags":             map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_-]+$`}},
					"conflicts":        map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_-]+$`}},
					"parent_behavior":  map[string]any{"type": "string", "enum": []string{string(autonomous.ChildDependsOnParent), string(autonomous.ChildIndependent)}},
					"evidence":         map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"child_task_set": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"parent_task_id", "proposal_id", "children"},
				"properties": map[string]any{
					"parent_task_id": map[string]any{"type": "string", "minLength": 1, "pattern": nonblank},
					"proposal_id":    map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"children":       map[string]any{"type": "array", "minItems": 1, "maxItems": autonomous.MaxChildTaskProposals, "items": map[string]any{"$ref": "#/$defs/child_task"}},
				},
			},
		},
	}
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal supervisor decision output schema: %w", err)
	}
	return append(raw, '\n'), nil
}
