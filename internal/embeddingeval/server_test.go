package embeddingeval

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
)

func TestProxyPreservesPinnedContractAndPrefixesQuery(t *testing.T) {
	var backendInput []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/v1/embeddings":
			var request struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			backendInput = request.Input
			_ = json.NewEncoder(w).Encode(map[string]any{
				"object": "list", "model": "backend",
				"data": []any{map[string]any{"object": "embedding", "index": 0, "embedding": unitVector()}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	model := selectedModel("one", strings.Repeat("a", 64))
	proxy, err := New(Config{BackendEndpoint: backend.URL, BackendModel: "backend", Model: model, QueryPrefix: codeindex.SelectedQueryInstruction})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.Handler())
	defer server.Close()
	client, err := embedding.NewClient(embedding.Config{Endpoint: server.URL + "/v1", ExpectedModel: model})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.EmbedQuery(t.Context(), "find routing")
	if err != nil || len(result.Value) != codeindex.SelectedEmbeddingDimensions || len(backendInput) != 1 || backendInput[0] != codeindex.SelectedQueryInstruction+"find routing" {
		t.Fatalf("query = %#v, input=%#v, err=%v", result, backendInput, err)
	}
}

func TestProxyRejectsNonUnitOrDivergentVectors(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "model": "backend", "data": []any{map[string]any{"object": "embedding", "index": 0, "embedding": []float64{2, 0}}}})
	}))
	defer backend.Close()
	model := selectedModel("two", strings.Repeat("b", 64))
	proxy, _ := New(Config{BackendEndpoint: backend.URL, BackendModel: "backend", Model: model, QueryPrefix: codeindex.SelectedQueryInstruction})
	server := httptest.NewServer(proxy.Handler())
	defer server.Close()
	client, _ := embedding.NewClient(embedding.Config{Endpoint: server.URL + "/v1", ExpectedModel: model})
	if _, err := client.EmbedQuery(t.Context(), "query"); !embedding.IsKind(err, embedding.ErrorUnavailable) {
		t.Fatalf("non-unit vector error = %v", err)
	}
}

func selectedModel(revision, artifact string) embedding.EmbeddingModelInfo {
	model := codeindex.SelectedEmbeddingEvidence().Model
	model.Revision = revision
	model.ArtifactSHA256 = artifact
	return model
}

func unitVector() []float64 {
	result := make([]float64, codeindex.SelectedEmbeddingDimensions)
	result[0] = 1
	return result
}
