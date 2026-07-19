package autonomouscycle_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/supervisor"
)

var supportedStructuredOutputsKeywords = map[string]struct{}{
	"type":                 {},
	"properties":           {},
	"required":             {},
	"additionalProperties": {},
	"items":                {},
	"enum":                 {},
	"anyOf":                {},
	"$defs":                {},
	"$ref":                 {},
	"description":          {},
	"pattern":              {},
	"format":               {},
	"multipleOf":           {},
	"maximum":              {},
	"exclusiveMaximum":     {},
	"minimum":              {},
	"exclusiveMinimum":     {},
	"minItems":             {},
	"maxItems":             {},
}

func TestProductionModelOutputSchemasUseSupportedStructuredOutputsSubset(t *testing.T) {
	builders := []struct {
		name  string
		build func() ([]byte, error)
	}{
		{name: "supervisor", build: supervisor.DecisionOutputSchema},
		{name: "planning", build: autonomousplanning.PlanningOutputSchema},
		{name: "audit", build: autonomousaudit.AuditOutputSchema},
		{name: "correction", build: autonomous.CorrectionOutputSchema},
	}
	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			raw, err := builder.build()
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatalf("decode production output schema: %v", err)
			}
			if err := validateSupportedStructuredOutputsSchema(schema); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSupportedStructuredOutputsValidatorRejectsUnsupportedKeywords(t *testing.T) {
	for _, keyword := range []string{
		"contains", "uniqueItems", "allOf", "oneOf", "not", "dependentRequired",
		"dependentSchemas", "if", "then", "else",
	} {
		t.Run(keyword, func(t *testing.T) {
			schema := closedObjectSchema()
			schema[keyword] = map[string]any{"type": "string"}
			assertSchemaValidationError(t, schema, "$."+keyword)
		})
	}
}

func TestSupportedStructuredOutputsValidatorRejectsInvalidStructure(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]any
		path   string
	}{
		{
			name:   "missing additionalProperties",
			schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []any{"value"}},
			path:   "$.additionalProperties",
		},
		{
			name:   "additionalProperties true",
			schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []any{"value"}, "additionalProperties": true},
			path:   "$.additionalProperties",
		},
		{
			name:   "missing property from required",
			schema: map[string]any{"type": "object", "properties": map[string]any{"first": map[string]any{"type": "string"}, "second": map[string]any{"type": "string"}}, "required": []any{"first"}, "additionalProperties": false},
			path:   "$.required",
		},
		{
			name:   "duplicate required entry",
			schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []any{"value", "value"}, "additionalProperties": false},
			path:   "$.required[1]",
		},
		{
			name:   "required entry absent from properties",
			schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []any{"missing"}, "additionalProperties": false},
			path:   "$.required[0]",
		},
		{
			name:   "unconstrained object",
			schema: map[string]any{"type": "object"},
			path:   "$.properties",
		},
		{
			name:   "array without items",
			schema: objectWithValueSchema(map[string]any{"type": "array"}),
			path:   "$.properties.value.items",
		},
		{
			name: "nullable object missing closure",
			schema: objectWithValueSchema(map[string]any{
				"type":       []any{"object", "null"},
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
				"required":   []any{"value"},
			}),
			path: "$.properties.value.additionalProperties",
		},
		{
			name:   "unresolved local ref",
			schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"$ref": "#/$defs/missing"}}, "required": []any{"value"}, "additionalProperties": false, "$defs": map[string]any{}},
			path:   "$.properties.value.$ref",
		},
		{
			name:   "nullable enum missing null",
			schema: objectWithValueSchema(map[string]any{"type": []any{"string", "null"}, "enum": []any{"value"}}),
			path:   "$.properties.value.enum",
		},
		{
			name: "invalid definition",
			schema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"value": map[string]any{"$ref": "#/$defs/value"}},
				"required":             []any{"value"},
				"additionalProperties": false,
				"$defs":                map[string]any{"value": map[string]any{"type": "array"}},
			},
			path: "$.$defs.value.items",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSchemaValidationError(t, tt.schema, tt.path)
		})
	}
}

func TestSupportedStructuredOutputsValidatorDistinguishesUserNames(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contains": map[string]any{"$ref": "#/$defs/oneOf"},
		},
		"required":             []any{"contains"},
		"additionalProperties": false,
		"$defs": map[string]any{
			"oneOf": map[string]any{"type": "string"},
		},
	}
	if err := validateSupportedStructuredOutputsSchema(schema); err != nil {
		t.Fatal(err)
	}
}

func validateSupportedStructuredOutputsSchema(root map[string]any) error {
	if err := validateSchemaNode(root, root, "$", true); err != nil {
		return err
	}
	return nil
}

func validateSchemaNode(root map[string]any, schema map[string]any, path string, isRoot bool) error {
	if len(schema) == 0 {
		return fmt.Errorf("%s: empty schemas are not in the supported Structured Outputs subset", path)
	}
	keys := make([]string, 0, len(schema))
	for key := range schema {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := supportedStructuredOutputsKeywords[key]; !ok {
			return fmt.Errorf("%s: unsupported Structured Outputs keyword %q", schemaPath(path, key), key)
		}
	}

	types, err := schemaTypes(schema["type"], schemaPath(path, "type"))
	if err != nil {
		return err
	}
	if isRoot && (len(types) != 1 || types[0] != "object") {
		return fmt.Errorf("%s: root Structured Outputs schema must have type object", schemaPath(path, "type"))
	}

	_, hasProperties := schema["properties"]
	_, hasRequired := schema["required"]
	_, hasAdditionalProperties := schema["additionalProperties"]
	isObject := containsString(types, "object") || hasProperties || hasRequired || hasAdditionalProperties
	if isObject {
		if !containsString(types, "object") {
			return fmt.Errorf("%s: object schema must declare type object", schemaPath(path, "type"))
		}
		if err := validateObjectSchema(root, schema, path); err != nil {
			return err
		}
	}

	_, hasItems := schema["items"]
	_, hasMinItems := schema["minItems"]
	_, hasMaxItems := schema["maxItems"]
	isArray := containsString(types, "array") || hasItems || hasMinItems || hasMaxItems
	if isArray {
		if !containsString(types, "array") {
			return fmt.Errorf("%s: array schema must declare type array", schemaPath(path, "type"))
		}
		item, ok := schema["items"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: arrays require a concrete items schema", schemaPath(path, "items"))
		}
		if err := validateSchemaNode(root, item, schemaPath(path, "items"), false); err != nil {
			return err
		}
	}

	if raw, ok := schema["anyOf"]; ok {
		branches, ok := raw.([]any)
		if !ok || len(branches) == 0 {
			return fmt.Errorf("%s: anyOf must be a non-empty schema array", schemaPath(path, "anyOf"))
		}
		for i, branch := range branches {
			branchSchema, ok := branch.(map[string]any)
			branchPath := fmt.Sprintf("%s[%d]", schemaPath(path, "anyOf"), i)
			if !ok {
				return fmt.Errorf("%s: anyOf branch must be a concrete schema", branchPath)
			}
			if err := validateSchemaNode(root, branchSchema, branchPath, false); err != nil {
				return err
			}
		}
	}

	if raw, ok := schema["$defs"]; ok {
		defs, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: $defs must be an object", schemaPath(path, "$defs"))
		}
		defNames := sortedKeys(defs)
		for _, name := range defNames {
			definition, ok := defs[name].(map[string]any)
			definitionPath := schemaPath(schemaPath(path, "$defs"), name)
			if !ok {
				return fmt.Errorf("%s: definition must be a concrete schema", definitionPath)
			}
			if err := validateSchemaNode(root, definition, definitionPath, false); err != nil {
				return err
			}
		}
	}

	if raw, ok := schema["$ref"]; ok {
		ref, ok := raw.(string)
		refPath := schemaPath(path, "$ref")
		if !ok || ref == "" {
			return fmt.Errorf("%s: $ref must be a non-empty local reference", refPath)
		}
		if _, err := resolveLocalRef(root, ref); err != nil {
			return fmt.Errorf("%s: %w", refPath, err)
		}
	}

	if raw, ok := schema["enum"]; ok {
		values, ok := raw.([]any)
		if !ok || len(values) == 0 {
			return fmt.Errorf("%s: enum must be a non-empty array", schemaPath(path, "enum"))
		}
		if containsString(types, "null") {
			nullable := false
			for _, value := range values {
				if value == nil {
					nullable = true
					break
				}
			}
			if !nullable {
				return fmt.Errorf("%s: nullable enum must include null", schemaPath(path, "enum"))
			}
		}
	}
	for _, key := range []string{"description", "pattern", "format"} {
		if raw, ok := schema[key]; ok {
			if _, ok := raw.(string); !ok {
				return fmt.Errorf("%s: %s must be a string", schemaPath(path, key), key)
			}
		}
	}
	for _, key := range []string{"multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum", "minItems", "maxItems"} {
		if raw, ok := schema[key]; ok {
			if _, ok := raw.(float64); !ok {
				return fmt.Errorf("%s: %s must be a number", schemaPath(path, key), key)
			}
		}
	}
	return nil
}

func validateObjectSchema(root map[string]any, schema map[string]any, path string) error {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s: objects require a concrete properties object", schemaPath(path, "properties"))
	}
	if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
		return fmt.Errorf("%s: objects require additionalProperties to be exactly false", schemaPath(path, "additionalProperties"))
	}
	required, ok := schema["required"].([]any)
	if !ok {
		return fmt.Errorf("%s: objects require a required array containing every property", schemaPath(path, "required"))
	}
	seen := make(map[string]struct{}, len(required))
	for i, raw := range required {
		requiredPath := fmt.Sprintf("%s[%d]", schemaPath(path, "required"), i)
		name, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s: required entry must be a property name", requiredPath)
		}
		if _, exists := properties[name]; !exists {
			return fmt.Errorf("%s: required entry %q is absent from properties", requiredPath, name)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("%s: duplicate required entry %q", requiredPath, name)
		}
		seen[name] = struct{}{}
	}
	propertyNames := sortedKeys(properties)
	for _, name := range propertyNames {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("%s: property %q is missing from required", schemaPath(path, "required"), name)
		}
		propertySchema, ok := properties[name].(map[string]any)
		propertyPath := schemaPath(schemaPath(path, "properties"), name)
		if !ok {
			return fmt.Errorf("%s: property must have a concrete schema", propertyPath)
		}
		if err := validateSchemaNode(root, propertySchema, propertyPath, false); err != nil {
			return err
		}
	}
	return nil
}

func schemaTypes(raw any, path string) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	if value, ok := raw.(string); ok {
		if !supportedSchemaType(value) {
			return nil, fmt.Errorf("%s: unsupported schema type %q", path, value)
		}
		return []string{value}, nil
	}
	values, ok := raw.([]any)
	if !ok || len(values) != 2 {
		return nil, fmt.Errorf("%s: type arrays must contain one concrete type and null", path)
	}
	types := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for i, rawType := range values {
		value, ok := rawType.(string)
		if !ok || !supportedSchemaType(value) {
			return nil, fmt.Errorf("%s[%d]: unsupported schema type", path, i)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("%s[%d]: duplicate schema type %q", path, i, value)
		}
		seen[value] = struct{}{}
		types = append(types, value)
	}
	if _, nullable := seen["null"]; !nullable {
		return nil, fmt.Errorf("%s: type arrays must include null", path)
	}
	return types, nil
}

func resolveLocalRef(root map[string]any, ref string) (any, error) {
	if ref == "#" {
		return root, nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("$ref %q is not a local JSON pointer", ref)
	}
	var current any = root
	for _, token := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("$ref %q traverses a non-object", ref)
		}
		next, ok := object[token]
		if !ok {
			return nil, fmt.Errorf("$ref %q does not resolve", ref)
		}
		current = next
	}
	if _, ok := current.(map[string]any); !ok {
		return nil, fmt.Errorf("$ref %q does not target a concrete schema", ref)
	}
	return current, nil
}

func schemaPath(path, key string) string {
	if key != "" && strings.IndexFunc(key, func(r rune) bool {
		return !(r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) == -1 {
		return path + "." + key
	}
	return path + "[" + strconv.Quote(key) + "]"
}

func supportedSchemaType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array", "null":
		return true
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func closedObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []any{},
		"additionalProperties": false,
	}
}

func objectWithValueSchema(value map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"value": value},
		"required":             []any{"value"},
		"additionalProperties": false,
	}
}

func assertSchemaValidationError(t *testing.T, schema map[string]any, paths ...string) {
	t.Helper()
	err := validateSupportedStructuredOutputsSchema(schema)
	if err == nil {
		t.Fatal("schema validation succeeded, want failure")
	}
	for _, path := range paths {
		if strings.Contains(err.Error(), path) {
			return
		}
	}
	t.Fatalf("schema validation error = %v, want exact JSON path in %q", err, paths)
}
