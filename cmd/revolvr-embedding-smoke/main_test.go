package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"revolvr/internal/embedding"
)

func TestRunReturnsExactMetadataAndVectorDimensions(t *testing.T) {
	model := embedding.EmbeddingModelInfo{
		SchemaVersion:  embedding.ModelInfoSchemaVersion,
		ModelName:      "fixture/smoke-model",
		Revision:       "revision-7",
		Dimensions:     2,
		Pooling:        "mean",
		Normalization:  "l2",
		Quantization:   "fp16",
		ArtifactSHA256: strings.Repeat("b", 64),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case "/v1/metadata":
			_ = json.NewEncoder(w).Encode(model)
		case "/v1/embeddings":
			var request struct {
				InputType string `json:"input_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.InputType != "query" {
				t.Errorf("embedding request = %+v, %v", request, err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object":     "list",
				"model":      model.ModelName,
				"model_info": model,
				"data": []any{map[string]any{
					"object":    "embedding",
					"index":     0,
					"embedding": []float64{0.25, 0.75},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	environment := map[string]string{
		"REVOLVR_EMBEDDING_ENDPOINT":        server.URL + "/v1",
		"REVOLVR_EMBEDDING_MODEL_NAME":      model.ModelName,
		"REVOLVR_EMBEDDING_MODEL_REVISION":  model.Revision,
		"REVOLVR_EMBEDDING_DIMENSIONS":      "2",
		"REVOLVR_EMBEDDING_POOLING":         model.Pooling,
		"REVOLVR_EMBEDDING_NORMALIZATION":   model.Normalization,
		"REVOLVR_EMBEDDING_QUANTIZATION":    model.Quantization,
		"REVOLVR_EMBEDDING_ARTIFACT_SHA256": model.ArtifactSHA256,
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{"-text", "smoke query"}, func(name string) string { return environment[name] }, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var report smokeReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.SchemaVersion != smokeSchemaVersion || report.Status.Mode != embedding.ServiceReady || report.Model != model || report.Space.Model != model || report.VectorDimensions != model.Dimensions {
		t.Fatalf("smoke report = %+v", report)
	}
	if report.Space.SHA256 == "" {
		t.Fatal("smoke report omitted embedding-space identity")
	}
}
