package ledger

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRunArtifactsFromEventsUsesExplicitArtifactEvent(t *testing.T) {
	events := []Event{
		{
			Type: EventRunArtifacts,
			Payload: mustRawMessage(t, RunArtifacts{
				ContextPayloadPath:   ".revolvr/runs/run-1/context.md",
				ContextManifestPath:  ".revolvr/runs/run-1/context.json",
				CodexStdoutJSONLPath: ".revolvr/runs/run-1/codex.jsonl",
				CodexStderrPath:      ".revolvr/runs/run-1/codex.stderr",
				LastMessagePath:      ".revolvr/runs/run-1/last-message.txt",
				ReceiptPath:          ".revolvr/receipts/run-1.md",
			}),
		},
		{
			Type:    EventCodexCompleted,
			Payload: json.RawMessage(`{"artifacts":{"stdout_jsonl":"/repo/.revolvr/runs/run-1/codex.jsonl"}}`),
		},
	}

	got, found := RunArtifactsFromEvents(events)
	if !found {
		t.Fatal("found = false, want true")
	}
	want := RunArtifacts{
		ContextPayloadPath:   ".revolvr/runs/run-1/context.md",
		ContextManifestPath:  ".revolvr/runs/run-1/context.json",
		CodexStdoutJSONLPath: ".revolvr/runs/run-1/codex.jsonl",
		CodexStderrPath:      ".revolvr/runs/run-1/codex.stderr",
		LastMessagePath:      ".revolvr/runs/run-1/last-message.txt",
		ReceiptPath:          ".revolvr/receipts/run-1.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifacts = %#v, want %#v", got, want)
	}
}

func TestRunArtifactsFromEventsReadsContextCodexAndReceiptPayloads(t *testing.T) {
	events := []Event{
		{
			Type:    EventContextBuilt,
			Payload: json.RawMessage(`{"context_payload_path":".revolvr/runs/run-2/context.md","context_manifest_path":".revolvr/runs/run-2/context.json","receipt_path":".revolvr/receipts/run-2.md"}`),
		},
		{
			Type: EventCodexStarted,
			Payload: json.RawMessage(`{
				"artifacts": {
					"stdout_jsonl": "/repo/.revolvr/runs/run-2/codex.jsonl",
					"stderr": "/repo/.revolvr/runs/run-2/codex.stderr",
					"last_message": "/repo/.revolvr/runs/run-2/last-message.txt"
				}
			}`),
		},
		{
			Type:    EventReceiptParsed,
			Payload: json.RawMessage(`{"receipt_path":".revolvr/receipts/run-2-final.md"}`),
		},
	}

	got, found := RunArtifactsFromEvents(events)
	if !found {
		t.Fatal("found = false, want true")
	}
	want := RunArtifacts{
		ContextPayloadPath:   ".revolvr/runs/run-2/context.md",
		ContextManifestPath:  ".revolvr/runs/run-2/context.json",
		CodexStdoutJSONLPath: "/repo/.revolvr/runs/run-2/codex.jsonl",
		CodexStderrPath:      "/repo/.revolvr/runs/run-2/codex.stderr",
		LastMessagePath:      "/repo/.revolvr/runs/run-2/last-message.txt",
		ReceiptPath:          ".revolvr/receipts/run-2.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifacts = %#v, want %#v", got, want)
	}
}

func TestRunArtifactsFromEventsReadsLegacyPromptPayloadPath(t *testing.T) {
	events := []Event{
		{
			Type:    EventPromptBuilt,
			Payload: json.RawMessage(`{"prompt_path":".revolvr/runs/run-legacy/prompt.md","receipt_path":".revolvr/receipts/run-legacy.md"}`),
		},
	}

	got, found := RunArtifactsFromEvents(events)
	if !found {
		t.Fatal("found = false, want true")
	}
	want := RunArtifacts{
		ContextPayloadPath: ".revolvr/runs/run-legacy/prompt.md",
		ReceiptPath:        ".revolvr/receipts/run-legacy.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifacts = %#v, want %#v", got, want)
	}
}

func TestRunArtifactsFromEventsNoArtifactEvents(t *testing.T) {
	artifacts, found := RunArtifactsFromEvents([]Event{
		{Type: EventRunStarted, Payload: json.RawMessage(`{"run_id":"run-3"}`)},
	})
	if found {
		t.Fatalf("found = true, want false with artifacts %#v", artifacts)
	}
	if !artifacts.Empty() {
		t.Fatalf("artifacts empty = false: %#v", artifacts)
	}
}

func mustRawMessage(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw message: %v", err)
	}
	return raw
}
