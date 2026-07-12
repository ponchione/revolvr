package ledger

import (
	"encoding/json"
	"strings"
)

type RunArtifacts struct {
	ContextPayloadPath            string `json:"context_payload_path"`
	ContextManifestPath           string `json:"context_manifest_path"`
	CodexStdoutJSONLPath          string `json:"codex_stdout_jsonl_path"`
	CodexStderrPath               string `json:"codex_stderr_path"`
	LastMessagePath               string `json:"last_message_path"`
	ReceiptPath                   string `json:"receipt_path"`
	DossierPath                   string `json:"dossier_path"`
	DossierManifestPath           string `json:"dossier_manifest_path"`
	SupervisorDossierPath         string `json:"supervisor_dossier_path"`
	SupervisorDossierManifestPath string `json:"supervisor_dossier_manifest_path"`
	SupervisorPromptPath          string `json:"supervisor_prompt_path"`
	SupervisorSchemaPath          string `json:"supervisor_schema_path"`
	SupervisorOutputPath          string `json:"supervisor_output_path"`
	SupervisorDecisionPath        string `json:"supervisor_decision_path"`
	SupervisorProvenancePath      string `json:"supervisor_provenance_path"`
	SupervisorSourcePath          string `json:"supervisor_source_path"`
	SupervisorDiagnosticsPath     string `json:"supervisor_diagnostics_path"`
	VerificationEvidencePath      string `json:"verification_evidence_path"`
}

func (a RunArtifacts) Empty() bool {
	return strings.TrimSpace(a.ContextPayloadPath) == "" &&
		strings.TrimSpace(a.ContextManifestPath) == "" &&
		strings.TrimSpace(a.CodexStdoutJSONLPath) == "" &&
		strings.TrimSpace(a.CodexStderrPath) == "" &&
		strings.TrimSpace(a.LastMessagePath) == "" &&
		strings.TrimSpace(a.ReceiptPath) == "" &&
		strings.TrimSpace(a.DossierPath) == "" &&
		strings.TrimSpace(a.DossierManifestPath) == "" &&
		strings.TrimSpace(a.SupervisorDossierPath) == "" &&
		strings.TrimSpace(a.SupervisorDossierManifestPath) == "" &&
		strings.TrimSpace(a.SupervisorPromptPath) == "" &&
		strings.TrimSpace(a.SupervisorSchemaPath) == "" &&
		strings.TrimSpace(a.SupervisorOutputPath) == "" &&
		strings.TrimSpace(a.SupervisorDecisionPath) == "" &&
		strings.TrimSpace(a.SupervisorProvenancePath) == "" &&
		strings.TrimSpace(a.SupervisorSourcePath) == "" &&
		strings.TrimSpace(a.SupervisorDiagnosticsPath) == "" &&
		strings.TrimSpace(a.VerificationEvidencePath) == ""
}

func RunArtifactsFromEvents(events []Event) (RunArtifacts, bool) {
	var out RunArtifacts
	found := false
	for _, event := range events {
		paths, ok := artifactsFromEvent(event)
		if !ok {
			continue
		}
		found = true
		out.mergeMissing(paths)
	}
	return out, found
}

func (a *RunArtifacts) mergeMissing(other RunArtifacts) {
	if strings.TrimSpace(a.ContextPayloadPath) == "" {
		a.ContextPayloadPath = strings.TrimSpace(other.ContextPayloadPath)
	}
	if strings.TrimSpace(a.ContextManifestPath) == "" {
		a.ContextManifestPath = strings.TrimSpace(other.ContextManifestPath)
	}
	if strings.TrimSpace(a.CodexStdoutJSONLPath) == "" {
		a.CodexStdoutJSONLPath = strings.TrimSpace(other.CodexStdoutJSONLPath)
	}
	if strings.TrimSpace(a.CodexStderrPath) == "" {
		a.CodexStderrPath = strings.TrimSpace(other.CodexStderrPath)
	}
	if strings.TrimSpace(a.LastMessagePath) == "" {
		a.LastMessagePath = strings.TrimSpace(other.LastMessagePath)
	}
	if strings.TrimSpace(a.ReceiptPath) == "" {
		a.ReceiptPath = strings.TrimSpace(other.ReceiptPath)
	}
	if strings.TrimSpace(a.DossierPath) == "" {
		a.DossierPath = strings.TrimSpace(other.DossierPath)
	}
	if strings.TrimSpace(a.DossierManifestPath) == "" {
		a.DossierManifestPath = strings.TrimSpace(other.DossierManifestPath)
	}
	if strings.TrimSpace(a.SupervisorDossierPath) == "" {
		a.SupervisorDossierPath = strings.TrimSpace(other.SupervisorDossierPath)
	}
	if strings.TrimSpace(a.SupervisorDossierManifestPath) == "" {
		a.SupervisorDossierManifestPath = strings.TrimSpace(other.SupervisorDossierManifestPath)
	}
	if strings.TrimSpace(a.SupervisorPromptPath) == "" {
		a.SupervisorPromptPath = strings.TrimSpace(other.SupervisorPromptPath)
	}
	if strings.TrimSpace(a.SupervisorSchemaPath) == "" {
		a.SupervisorSchemaPath = strings.TrimSpace(other.SupervisorSchemaPath)
	}
	if strings.TrimSpace(a.SupervisorOutputPath) == "" {
		a.SupervisorOutputPath = strings.TrimSpace(other.SupervisorOutputPath)
	}
	if strings.TrimSpace(a.SupervisorDecisionPath) == "" {
		a.SupervisorDecisionPath = strings.TrimSpace(other.SupervisorDecisionPath)
	}
	if strings.TrimSpace(a.SupervisorProvenancePath) == "" {
		a.SupervisorProvenancePath = strings.TrimSpace(other.SupervisorProvenancePath)
	}
	if strings.TrimSpace(a.SupervisorSourcePath) == "" {
		a.SupervisorSourcePath = strings.TrimSpace(other.SupervisorSourcePath)
	}
	if strings.TrimSpace(a.SupervisorDiagnosticsPath) == "" {
		a.SupervisorDiagnosticsPath = strings.TrimSpace(other.SupervisorDiagnosticsPath)
	}
	if strings.TrimSpace(a.VerificationEvidencePath) == "" {
		a.VerificationEvidencePath = strings.TrimSpace(other.VerificationEvidencePath)
	}
}

func artifactsFromEvent(event Event) (RunArtifacts, bool) {
	switch event.Type {
	case EventRunArtifacts:
		paths, _ := decodeRunArtifacts(event.Payload)
		return paths, true
	case EventContextBuilt:
		paths, ok := decodeContextArtifacts(event.Payload)
		return paths, ok
	case EventCodexStarted, EventCodexCompleted:
		paths, ok := decodeCodexArtifacts(event.Payload)
		return paths, ok
	case EventReceiptParsed, EventReceiptSynthesized:
		receiptPath := stringField(event.Payload, "receipt_path")
		if receiptPath == "" {
			return RunArtifacts{}, false
		}
		return RunArtifacts{ReceiptPath: receiptPath}, true
	case EventSupervisorPrepared, EventSupervisorValidated, EventSupervisorRejected, EventSupervisorMutation:
		paths, ok := decodeSupervisorArtifacts(event.Payload)
		return paths, ok
	case EventVerificationCompleted:
		artifactPayload := objectField(event.Payload, "Artifact")
		if len(artifactPayload) == 0 {
			artifactPayload = objectField(event.Payload, "artifact")
		}
		path := stringField(artifactPayload, "Path")
		if path == "" {
			path = stringField(artifactPayload, "path")
		}
		if path == "" {
			return RunArtifacts{}, false
		}
		return RunArtifacts{VerificationEvidencePath: path}, true
	default:
		return RunArtifacts{}, false
	}
}

func decodeSupervisorArtifacts(payload json.RawMessage) (RunArtifacts, bool) {
	artifactPayload := objectField(payload, "artifacts")
	if len(artifactPayload) == 0 {
		return RunArtifacts{}, false
	}
	paths := RunArtifacts{
		SupervisorPromptPath:          artifactPathField(artifactPayload, "prompt"),
		SupervisorDossierPath:         artifactPathField(artifactPayload, "dossier"),
		SupervisorDossierManifestPath: artifactPathField(artifactPayload, "dossier_manifest"),
		SupervisorSchemaPath:          artifactPathField(artifactPayload, "schema"),
		SupervisorOutputPath:          artifactPathField(artifactPayload, "raw_output"),
		SupervisorDecisionPath:        artifactPathField(artifactPayload, "decision"),
		SupervisorProvenancePath:      artifactPathField(artifactPayload, "provenance"),
		SupervisorSourcePath:          artifactPathField(artifactPayload, "source_evidence"),
		SupervisorDiagnosticsPath:     artifactPathField(artifactPayload, "diagnostics"),
		CodexStdoutJSONLPath:          artifactPathField(artifactPayload, "codex_stdout"),
		CodexStderrPath:               artifactPathField(artifactPayload, "codex_stderr"),
	}
	return paths, !paths.Empty()
}

func artifactPathField(payload json.RawMessage, key string) string {
	value := objectField(payload, key)
	if len(value) == 0 {
		return ""
	}
	if stringField(value, "sha256") == "" {
		return ""
	}
	return stringField(value, "path")
}

func decodeRunArtifacts(payload json.RawMessage) (RunArtifacts, bool) {
	if len(payload) == 0 {
		return RunArtifacts{}, false
	}
	paths := RunArtifacts{
		ContextPayloadPath:   stringField(payload, "context_payload_path"),
		ContextManifestPath:  stringField(payload, "context_manifest_path"),
		CodexStdoutJSONLPath: stringField(payload, "codex_stdout_jsonl_path"),
		CodexStderrPath:      stringField(payload, "codex_stderr_path"),
		LastMessagePath:      stringField(payload, "last_message_path"),
		ReceiptPath:          stringField(payload, "receipt_path"),
		DossierPath:          stringField(payload, "dossier_path"),
		DossierManifestPath:  stringField(payload, "dossier_manifest_path"),
	}
	return paths, !paths.Empty()
}

func decodeContextArtifacts(payload json.RawMessage) (RunArtifacts, bool) {
	paths := RunArtifacts{
		ContextPayloadPath:  stringField(payload, "context_payload_path"),
		ContextManifestPath: stringField(payload, "context_manifest_path"),
		ReceiptPath:         stringField(payload, "receipt_path"),
		DossierPath:         stringField(payload, "dossier_path"),
		DossierManifestPath: stringField(payload, "dossier_manifest_path"),
	}
	return paths, !paths.Empty()
}

func decodeCodexArtifacts(payload json.RawMessage) (RunArtifacts, bool) {
	artifactPayload := objectField(payload, "artifacts")
	if len(artifactPayload) == 0 {
		return RunArtifacts{}, false
	}
	paths := RunArtifacts{
		CodexStdoutJSONLPath: stringField(artifactPayload, "stdout_jsonl"),
		CodexStderrPath:      stringField(artifactPayload, "stderr"),
		LastMessagePath:      stringField(artifactPayload, "last_message"),
	}
	return paths, !paths.Empty()
}

func stringField(payload json.RawMessage, key string) string {
	fields := objectFields(payload)
	if len(fields) == 0 {
		return ""
	}
	raw, ok := fields[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func objectField(payload json.RawMessage, key string) json.RawMessage {
	fields := objectFields(payload)
	if len(fields) == 0 {
		return nil
	}
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil
	}
	return raw
}

func objectFields(payload json.RawMessage) map[string]json.RawMessage {
	if len(payload) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil
	}
	return fields
}
