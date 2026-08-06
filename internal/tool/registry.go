package tool

import (
	"encoding/json"
	"errors"

	"revolvr/internal/model"
	"revolvr/internal/sandbox"
)

func RegistryForRole(role sandbox.Role) (Registry, error) {
	definitions := allDefinitions()
	switch role {
	case sandbox.RoleImplementer, sandbox.RoleCorrector:
		// These are the only v1 source-mutation roles.
	case sandbox.RoleVerifier:
		definitions = definitions[:2+1]
		definitions[2] = allDefinitions()[3]
	default:
		return Registry{}, errors.New("tool registry: unsupported role")
	}
	registry := Registry{Version: RegistryVersion, Role: role, Definitions: definitions}
	raw, _ := json.Marshal(registryMaterial(registry))
	registry.SHA256 = model.SHA256(raw)
	return registry, nil
}

func allDefinitions() []Definition {
	return []Definition{
		{Name: ToolFileRead, SchemaVersion: "revolvr-tool-file-read-v1", Capability: CapabilityRead, InputSchema: objectSchema(map[string]any{
			"path": stringSchema(), "offset": integerSchema(0), "max_bytes": integerSchema(1),
		}, []string{"path", "offset", "max_bytes"})},
		{Name: ToolTextSearch, SchemaVersion: "revolvr-tool-text-search-v1", Capability: CapabilitySearch, InputSchema: objectSchema(map[string]any{
			"query": stringSchema(), "paths": arraySchema(stringSchema()), "maximum_results": integerSchema(1), "output_cap_bytes": integerSchema(1),
		}, []string{"query", "paths", "maximum_results", "output_cap_bytes"})},
		{Name: ToolSourceEdit, SchemaVersion: "revolvr-tool-source-edit-v1", Capability: CapabilityWrite, MutatesSource: true, InputSchema: objectSchema(map[string]any{
			"path": stringSchema(), "expected_sha256": stringSchema(), "content": stringSchema(),
		}, []string{"path", "expected_sha256", "content"})},
		{Name: ToolCommand, SchemaVersion: "revolvr-tool-command-v1", Capability: CapabilityCommand, InputSchema: objectSchema(map[string]any{
			"argv": arraySchema(stringSchema()), "working_directory": stringSchema(), "environment_names": arraySchema(stringSchema()),
			"network":              map[string]any{"type": "string", "enum": []string{"none", "dependencies", "open"}},
			"timeout_milliseconds": integerSchema(1), "cpus": integerSchema(1), "memory_bytes": integerSchema(1),
			"pids": integerSchema(1), "tmpfs_bytes": integerSchema(1), "stdout_cap_bytes": integerSchema(1), "stderr_cap_bytes": integerSchema(1),
		}, []string{"argv", "working_directory", "environment_names", "network", "timeout_milliseconds", "cpus", "memory_bytes", "pids", "tmpfs_bytes", "stdout_cap_bytes", "stderr_cap_bytes"})},
	}
}

func registryMaterial(registry Registry) Registry {
	registry.SHA256 = ""
	registry.Definitions = cloneDefinitions(registry.Definitions)
	return registry
}

func cloneDefinitions(values []Definition) []Definition {
	result := make([]Definition, len(values))
	for i, value := range values {
		result[i] = value
		result[i].InputSchema = append(json.RawMessage(nil), value.InputSchema...)
	}
	return result
}

func objectSchema(properties map[string]any, required []string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": properties, "required": required,
	})
	return raw
}

func stringSchema() map[string]any { return map[string]any{"type": "string"} }
func integerSchema(minimum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": minimum}
}
func arraySchema(items any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}
