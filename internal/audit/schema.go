package audit

import (
	"encoding/json"
	"fmt"

	"revolvr/internal/model"
)

func OutputSchema() ([]byte, error) {
	nonblank := map[string]any{"type": "string", "pattern": `.*\S.*`}
	hash := map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`}
	stable := map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`}
	identity := object([]string{"request_id", "task_id", "run_id", "source_revision", "prompt_version", "prompt_sha256", "response_schema_version", "response_schema_sha256"}, map[string]any{
		"request_id": nonblank, "task_id": nonblank, "run_id": nonblank,
		"source_revision": hash, "prompt_version": nonblank, "prompt_sha256": hash,
		"response_schema_version": singleton(OutputSchemaVersion), "response_schema_sha256": hash,
	})
	authority := object([]string{"audit_id", "audit_kind", "task_id", "task_version_id", "run_id", "source_revision", "source_commit", "source_tree", "diff_sha256", "verification_run_id", "dossier_schema_version", "dossier_sha256"}, map[string]any{
		"audit_id":   nonblank,
		"audit_kind": map[string]any{"type": "string", "enum": []string{string(KindBase), string(KindSecurity), string(KindPerformance), string(KindIntegration), string(KindMigration), string(KindDocumentation), string(KindAPICompatibility)}},
		"task_id":    nonblank, "task_version_id": nonblank, "run_id": nonblank,
		"source_revision": hash,
		"source_commit":   map[string]any{"type": "string", "pattern": `^[a-f0-9]{40}([a-f0-9]{24})?$`},
		"source_tree":     map[string]any{"type": "string", "pattern": `^[a-f0-9]{40}([a-f0-9]{24})?$`},
		"diff_sha256":     hash, "verification_run_id": nonblank,
		"dossier_schema_version": singleton(DossierSchemaVersion), "dossier_sha256": hash,
	})
	citation := object([]string{"artifact_id", "path", "sha256", "start_line", "end_line"}, map[string]any{
		"artifact_id": nonblank, "path": nonblank, "sha256": hash,
		"start_line": map[string]any{"type": "integer", "minimum": 1},
		"end_line":   map[string]any{"type": "integer", "minimum": 1},
	})
	impact := object([]string{"criterion_id", "impact", "detail"}, map[string]any{
		"criterion_id": nonblank,
		"impact":       map[string]any{"type": "string", "enum": []string{string(ImpactViolated), string(ImpactAtRisk), string(ImpactUnverified)}},
		"detail":       nonblank,
	})
	finding := object([]string{"id", "significance", "summary", "required_correction", "source_evidence", "affected_files", "affected_symbols", "criterion_impact"}, map[string]any{
		"id":           stable,
		"significance": map[string]any{"type": "string", "enum": []string{string(SignificanceBlocking), string(SignificanceNonBlocking)}},
		"summary":      nonblank, "required_correction": nonblank,
		"source_evidence":  map[string]any{"type": "array", "minItems": 1, "items": citation},
		"affected_files":   map[string]any{"type": "array", "minItems": 1, "items": nonblank},
		"affected_symbols": map[string]any{"type": "array", "items": nonblank},
		"criterion_impact": map[string]any{"type": "array", "minItems": 1, "items": impact},
	})
	schema := object([]string{"revolvr_identity", "schema_version", "authority", "disposition", "rationale", "blocked_reason", "findings"}, map[string]any{
		"revolvr_identity": identity,
		"schema_version":   singleton(OutputSchemaVersion),
		"authority":        authority,
		"disposition":      map[string]any{"type": "string", "enum": []string{string(DispositionClean), string(DispositionChangesRequired), string(DispositionBlocked)}},
		"rationale":        nonblank,
		"blocked_reason":   map[string]any{"type": "string"},
		"findings":         map[string]any{"type": "array", "maxItems": MaximumFindings, "items": finding},
	})
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal audit output schema: %w", err)
	}
	return raw, nil
}

func object(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}

func singleton(value string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{value}}
}

func schemaIdentity(raw []byte) string { return model.SHA256(raw) }
