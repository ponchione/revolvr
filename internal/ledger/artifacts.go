package ledger

import (
	"encoding/json"
	"strings"
)

type RunArtifacts struct {
	ContextPayloadPath   string `json:"context_payload_path"`
	ContextManifestPath  string `json:"context_manifest_path"`
	CodexStdoutJSONLPath string `json:"codex_stdout_jsonl_path"`
	CodexStderrPath      string `json:"codex_stderr_path"`
	LastMessagePath      string `json:"last_message_path"`
	ReceiptPath          string `json:"receipt_path"`
}

func (a RunArtifacts) Empty() bool {
	return strings.TrimSpace(a.ContextPayloadPath) == "" &&
		strings.TrimSpace(a.ContextManifestPath) == "" &&
		strings.TrimSpace(a.CodexStdoutJSONLPath) == "" &&
		strings.TrimSpace(a.CodexStderrPath) == "" &&
		strings.TrimSpace(a.LastMessagePath) == "" &&
		strings.TrimSpace(a.ReceiptPath) == ""
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
}

func artifactsFromEvent(event Event) (RunArtifacts, bool) {
	switch event.Type {
	case EventRunArtifacts:
		paths, _ := decodeRunArtifacts(event.Payload)
		return paths, true
	case EventContextBuilt, EventPromptBuilt:
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
	default:
		return RunArtifacts{}, false
	}
}

func decodeRunArtifacts(payload json.RawMessage) (RunArtifacts, bool) {
	if len(payload) == 0 {
		return RunArtifacts{}, false
	}
	paths := RunArtifacts{
		ContextPayloadPath:   contextPayloadPathField(payload),
		ContextManifestPath:  stringField(payload, "context_manifest_path"),
		CodexStdoutJSONLPath: stringField(payload, "codex_stdout_jsonl_path"),
		CodexStderrPath:      stringField(payload, "codex_stderr_path"),
		LastMessagePath:      stringField(payload, "last_message_path"),
		ReceiptPath:          stringField(payload, "receipt_path"),
	}
	return paths, !paths.Empty()
}

func decodeContextArtifacts(payload json.RawMessage) (RunArtifacts, bool) {
	paths := RunArtifacts{
		ContextPayloadPath:  contextPayloadPathField(payload),
		ContextManifestPath: stringField(payload, "context_manifest_path"),
		ReceiptPath:         stringField(payload, "receipt_path"),
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

func contextPayloadPathField(payload json.RawMessage) string {
	if path := stringField(payload, "context_payload_path"); path != "" {
		return path
	}
	return stringField(payload, "prompt_path")
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
