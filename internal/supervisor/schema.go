package supervisor

import (
	"encoding/json"
	"fmt"

	"revolvr/internal/autonomous"
)

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
	profiles := make([]any, 0, len(workerActionProfiles)+1)
	for _, pair := range workerActionProfiles {
		profiles = append(profiles, string(pair.profile))
	}
	profiles = append(profiles, nil)

	nonblank := `.*\S.*`
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"task_id", "action", "worker_profile", "rationale", "success_criteria", "inputs",
			"finding_ids", "verification_failure", "strategy", "needs_input", "child_tasks",
		},
		"properties": map[string]any{
			"task_id":        map[string]any{"type": "string", "pattern": nonblank},
			"action":         map[string]any{"type": "string", "enum": actions},
			"worker_profile": map[string]any{"type": []string{"string", "null"}, "enum": profiles},
			"rationale":      map[string]any{"type": "string", "pattern": nonblank},
			"success_criteria": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "pattern": nonblank},
			},
			"inputs": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    map[string]any{"$ref": "#/$defs/evidence_reference"},
			},
			"finding_ids": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
			},
			"verification_failure": nullableRef("#/$defs/verification_failure"),
			"strategy":             nullableRef("#/$defs/strategy"),
			"needs_input":          nullableRef("#/$defs/needs_input_question"),
			"child_tasks":          nullableRef("#/$defs/child_task_set"),
		},
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
					"reference": map[string]any{"type": "string", "pattern": nonblank},
					"detail":    map[string]any{"type": "string", "pattern": nonblank},
				},
			},
			"verification_failure": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"task_id", "run_id", "occurrence_id", "source_revision", "status", "evidence"},
				"properties": map[string]any{
					"task_id":         map[string]any{"type": "string", "pattern": nonblank},
					"run_id":          map[string]any{"type": "string", "pattern": nonblank},
					"occurrence_id":   map[string]any{"type": "string", "pattern": nonblank},
					"source_revision": map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"},
					"status":          singletonString(string(autonomous.VerificationStatusFailed)),
					"evidence":        map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"strategy": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"approach", "techniques", "targets"},
				"properties": map[string]any{
					"approach":   map[string]any{"type": "string", "pattern": nonblank},
					"techniques": map[string]any{"type": "array", "items": map[string]any{"type": "string", "pattern": nonblank}},
					"targets":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"needs_input_option": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"id", "meaning"},
				"properties": map[string]any{
					"id":      map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"meaning": map[string]any{"type": "string", "pattern": nonblank},
				},
			},
			"independent_work": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"id", "action", "worker_profile", "description", "source_effect", "independent_of_option_ids", "inputs"},
				"properties": map[string]any{
					"id":                        map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"action":                    map[string]any{"type": "string", "enum": []string{string(autonomous.ActionPlan), string(autonomous.ActionAudit)}},
					"worker_profile":            map[string]any{"type": "string", "enum": []string{string(autonomous.WorkerProfilePlanner), string(autonomous.WorkerProfileAuditor)}},
					"description":               map[string]any{"type": "string", "pattern": nonblank},
					"source_effect":             singletonString(string(autonomous.InputSourceEffectReadOnly)),
					"independent_of_option_ids": map[string]any{"type": "array", "minItems": 2, "items": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}},
					"inputs":                    map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"needs_input_question": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required": []string{
					"task_id", "question_id", "revision", "content_sha256", "question", "blocking_reason",
					"options", "recommendation", "evidence", "independent_work",
				},
				"properties": map[string]any{
					"task_id":          map[string]any{"type": "string", "pattern": nonblank},
					"question_id":      map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"revision":         map[string]any{"type": "integer", "minimum": 1},
					"content_sha256":   map[string]any{"type": []string{"string", "null"}, "pattern": `^[a-f0-9]{64}$`},
					"question":         map[string]any{"type": "string", "pattern": nonblank},
					"blocking_reason":  map[string]any{"type": "string", "pattern": nonblank},
					"options":          map[string]any{"type": "array", "minItems": 2, "items": map[string]any{"$ref": "#/$defs/needs_input_option"}},
					"recommendation":   map[string]any{"type": "object", "additionalProperties": false, "required": []string{"option_id", "rationale"}, "properties": map[string]any{"option_id": map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}, "rationale": map[string]any{"type": "string", "pattern": nonblank}}},
					"evidence":         map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
					"independent_work": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/independent_work"}},
				},
			},
			"child_task": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"key", "title", "scope", "success_criteria", "depends_on", "tags", "conflicts", "parent_behavior", "evidence"},
				"properties": map[string]any{
					"key":              map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`},
					"title":            map[string]any{"type": "string", "pattern": nonblank},
					"scope":            map[string]any{"type": "string", "pattern": nonblank},
					"success_criteria": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "pattern": nonblank}},
					"depends_on":       map[string]any{"type": "array", "items": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_-]+$`}},
					"tags":             map[string]any{"type": "array", "items": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_-]+$`}},
					"conflicts":        map[string]any{"type": "array", "items": map[string]any{"type": "string", "pattern": `^[A-Za-z0-9_-]+$`}},
					"parent_behavior":  map[string]any{"type": "string", "enum": []string{string(autonomous.ChildDependsOnParent), string(autonomous.ChildIndependent)}},
					"evidence":         map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"$ref": "#/$defs/evidence_reference"}},
				},
			},
			"child_task_set": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"parent_task_id", "proposal_id", "children"},
				"properties": map[string]any{
					"parent_task_id": map[string]any{"type": "string", "pattern": nonblank},
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

func nullableRef(ref string) map[string]any {
	return map[string]any{"anyOf": []any{map[string]any{"$ref": ref}, map[string]any{"type": "null"}}}
}

func singletonString(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}
