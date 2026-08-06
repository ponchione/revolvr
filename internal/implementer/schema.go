package implementer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"revolvr/internal/model"
)

func SummarySchema() (json.RawMessage, error) {
	stringValue := map[string]any{"type": "string"}
	stringArray := map[string]any{"type": "array", "items": stringValue}
	identityProperties := map[string]any{}
	for _, key := range []string{
		"project_id", "task_id", "task_version_id", "run_id", "source_revision", "source_commit", "source_tree",
		"plan_id", "plan_version_id", "step_batch_sha256", "workspace_id", "sandbox_id", "sandbox_sha256", "prompt_version", "prompt_sha256",
		"summary_schema_version", "summary_schema_sha256", "registry_version", "registry_sha256", "host_policy_version",
		"host_policy_sha256", "model_policy_version", "model_policy_sha256",
	} {
		identityProperties[key] = stringValue
	}
	identityProperties["plan_revision"] = map[string]any{"type": "integer", "minimum": 1}
	identityRequired := make([]string, 0, len(identityProperties))
	for key := range identityProperties {
		identityRequired = append(identityRequired, key)
	}
	sortStrings(identityRequired)
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object", "additionalProperties": false,
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "string", "const": SummarySchemaVersion},
			"identity":       map[string]any{"type": "object", "additionalProperties": false, "properties": identityProperties, "required": identityRequired},
			"summary":        stringValue,
			"claimed_files":  stringArray,
			"voluntary_tests": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{"tool_call_id": stringValue, "outcome": stringValue},
				"required":   []string{"tool_call_id", "outcome"},
			}},
			"concerns": stringArray,
			"candidate_plan_progress": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"step_id":           stringValue,
					"status":            map[string]any{"type": "string", "enum": []string{"candidate_completed", "candidate_partial", "unchanged"}},
					"evidence_call_ids": stringArray,
				},
				"required": []string{"step_id", "status", "evidence_call_ids"},
			}},
			"candidate_follow_up_work": stringArray,
			"partial":                  map[string]any{"type": "boolean"},
		},
		"required": []string{"schema_version", "identity", "summary", "claimed_files", "voluntary_tests", "concerns", "candidate_plan_progress", "candidate_follow_up_work", "partial"},
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	if model.SHA256(raw) == "" {
		return nil, errors.New("implementer summary schema identity is empty")
	}
	return raw, nil
}

func validateSummaryJSON(raw []byte) error {
	schemaRaw, err := SummarySchema()
	if err != nil {
		return err
	}
	schemaValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaRaw))
	if err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(summaryRejectingLoader{})
	if err := compiler.AddResource("urn:revolvr:implementer-summary", schemaValue); err != nil {
		return err
	}
	compiled, err := compiler.Compile("urn:revolvr:implementer-summary")
	if err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("decode implementer summary for schema validation: %w", err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("implementer summary violates the closed schema: %w", err)
	}
	return nil
}

type summaryRejectingLoader struct{}

func (summaryRejectingLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external implementer schema resource %q is not admitted", url)
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
