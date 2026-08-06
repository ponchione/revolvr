package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestModelInfoIdentityAndHealth(t *testing.T) {
	model := fixtureModel()
	server := newFixtureServer(t, model, nil)
	defer server.Close()
	client := fixtureClient(t, server.URL+"/v1", model, Config{})

	status, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if status.Mode != ServiceReady || status.Model == nil || *status.Model != model {
		t.Fatalf("Health() status = %+v", status)
	}
	const wantSpace = "4512c844ad325360777aecbfe38a23d22638cd2a5344a5418597ead7c3242707"
	if status.SpaceSHA256 != wantSpace {
		t.Fatalf("space SHA-256 = %q, want %q", status.SpaceSHA256, wantSpace)
	}
	info, err := client.ModelInfo(context.Background())
	if err != nil || info != model {
		t.Fatalf("ModelInfo() = %+v, %v", info, err)
	}
}

func TestEmbedDocumentsAndQueryUseExactBatchContract(t *testing.T) {
	model := fixtureModel()
	var requests []embeddingRequest
	server := newFixtureServer(t, model, func(w http.ResponseWriter, r *http.Request) {
		var request embeddingRequest
		decodeRequest(t, r, &request)
		requests = append(requests, request)
		data := make([]embeddingData, len(request.Input))
		for i := range request.Input {
			// Return reverse index order to prove the adapter restores input order.
			index := len(request.Input) - i - 1
			data[i] = vectorData(index, float64(index+1))
		}
		writeJSON(t, w, embeddingResponse{Object: "list", Data: data, Model: model.ModelName, ModelInfo: model})
	})
	defer server.Close()
	client := fixtureClient(t, server.URL+"/v1", model, Config{})

	documents, err := client.EmbedDocuments(context.Background(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("EmbedDocuments() error = %v", err)
	}
	wantDocuments := [][]float32{{1, 1.25, 1.5}, {2, 2.25, 2.5}}
	if !reflect.DeepEqual(documents.Values, wantDocuments) || documents.Status.Mode != ServiceReady || documents.Space.SHA256 == "" {
		t.Fatalf("EmbedDocuments() = %+v, want values %v", documents, wantDocuments)
	}
	query, err := client.EmbedQuery(context.Background(), "where is the scheduler?")
	if err != nil {
		t.Fatalf("EmbedQuery() error = %v", err)
	}
	if !reflect.DeepEqual(query.Value, []float32{1, 1.25, 1.5}) || query.Space != documents.Space {
		t.Fatalf("EmbedQuery() = %+v", query)
	}
	if len(requests) != 2 {
		t.Fatalf("embedding requests = %d, want 2", len(requests))
	}
	if got := requests[0]; got.Model != model.ModelName || got.InputType != "documents" || got.EncodingFormat != "float" || !reflect.DeepEqual(got.Input, []string{"alpha", "beta"}) {
		t.Fatalf("document request = %+v", got)
	}
	if got := requests[1]; got.InputType != "query" || !reflect.DeepEqual(got.Input, []string{"where is the scheduler?"}) {
		t.Fatalf("query request = %+v", got)
	}
}

func TestInputBoundsFailBeforeNetwork(t *testing.T) {
	model := fixtureModel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	client := fixtureClient(t, server.URL+"/v1", model, Config{MaxBatchInputs: 2, MaxInputBytes: 4, MaxBatchBytes: 6})

	tests := []struct {
		name  string
		input []string
	}{
		{name: "empty batch"},
		{name: "too many", input: []string{"a", "b", "c"}},
		{name: "empty item", input: []string{"a", ""}},
		{name: "item bytes", input: []string{"12345"}},
		{name: "batch bytes", input: []string{"1234", "5678"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := client.EmbedDocuments(context.Background(), test.input)
			if !IsKind(err, ErrorInvalidInput) || !reflect.DeepEqual(result, EmbeddingBatch{}) {
				t.Fatalf("EmbedDocuments() = %+v, %v", result, err)
			}
			var adapterErr *AdapterError
			if !errors.As(err, &adapterErr) || adapterErr.Status().Mode != ServiceFailed {
				t.Fatalf("invalid-input status = %+v", adapterErr)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("network calls = %d, want 0", calls.Load())
	}
}

func TestTypedDegradedResponseFailures(t *testing.T) {
	model := fixtureModel()
	drift := model
	drift.Revision = "fixture-revision-2"
	tests := []struct {
		name      string
		want      ErrorKind
		health    func(http.ResponseWriter)
		metadata  func(http.ResponseWriter)
		embedding func(http.ResponseWriter)
		maxBytes  int64
	}{
		{name: "unhealthy", want: ErrorUnhealthy, health: func(w http.ResponseWriter) { writeJSON(t, w, healthResponse{Status: "starting"}) }},
		{name: "unavailable", want: ErrorUnavailable, metadata: func(w http.ResponseWriter) { http.Error(w, "unavailable", http.StatusServiceUnavailable) }},
		{name: "malformed", want: ErrorMalformedResponse, embedding: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"object":`)) }},
		{name: "duplicate field", want: ErrorMalformedResponse, embedding: func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"object":"list","object":"list"}`)) }},
		{name: "wrong count", want: ErrorWrongCount, embedding: func(w http.ResponseWriter) {
			writeJSON(t, w, embeddingResponse{Object: "list", Model: model.ModelName, ModelInfo: model})
		}},
		{name: "wrong dimension", want: ErrorWrongDimension, embedding: func(w http.ResponseWriter) {
			writeJSON(t, w, embeddingResponse{Object: "list", Data: []embeddingData{{Object: "embedding", Index: 0, Embedding: rawValues(1, 2)}}, Model: model.ModelName, ModelInfo: model})
		}},
		{name: "non finite", want: ErrorNonFiniteVector, embedding: func(w http.ResponseWriter) {
			writeJSON(t, w, embeddingResponse{Object: "list", Data: []embeddingData{{Object: "embedding", Index: 0, Embedding: []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"NaN"`), json.RawMessage(`3`)}}}, Model: model.ModelName, ModelInfo: model})
		}},
		{name: "response drift", want: ErrorModelMetadataDrift, embedding: func(w http.ResponseWriter) {
			writeJSON(t, w, embeddingResponse{Object: "list", Data: []embeddingData{vectorData(0, 1)}, Model: drift.ModelName, ModelInfo: drift})
		}},
		{name: "oversized", want: ErrorMalformedResponse, embedding: func(w http.ResponseWriter) { _, _ = w.Write([]byte(strings.Repeat(" ", 129))) }, maxBytes: 128},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/health":
					if test.health != nil {
						test.health(w)
						return
					}
					writeJSON(t, w, healthResponse{Status: "ok"})
				case "/v1/metadata":
					if test.metadata != nil {
						test.metadata(w)
						return
					}
					writeJSON(t, w, model)
				case "/v1/embeddings":
					test.embedding(w)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := fixtureClient(t, server.URL+"/v1", model, Config{MaxResponseBytes: test.maxBytes})
			result, err := client.EmbedQuery(context.Background(), "query")
			if !IsKind(err, test.want) || !reflect.DeepEqual(result, Embedding{}) {
				t.Fatalf("EmbedQuery() = %+v, %v; want %s", result, err, test.want)
			}
			var adapterErr *AdapterError
			if !errors.As(err, &adapterErr) || adapterErr.Status().Mode != ServiceDegraded {
				t.Fatalf("degraded status = %+v", adapterErr)
			}
		})
	}
}

func TestTransportUnavailableReturnsDegradedStatus(t *testing.T) {
	model := fixtureModel()
	server := newFixtureServer(t, model, nil)
	client := fixtureClient(t, server.URL+"/v1", model, Config{})
	server.Close()

	status, err := client.Health(context.Background())
	if !IsKind(err, ErrorUnavailable) || status.Mode != ServiceDegraded || status.Kind != ErrorUnavailable {
		t.Fatalf("Health() = %+v, %v", status, err)
	}
}

func TestMetadataDriftAfterEmbeddingDiscardsWholeResult(t *testing.T) {
	model := fixtureModel()
	drift := model
	drift.Quantization = "int8"
	var metadataCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			writeJSON(t, w, healthResponse{Status: "ok"})
		case "/v1/metadata":
			if metadataCalls.Add(1) == 1 {
				writeJSON(t, w, model)
			} else {
				writeJSON(t, w, drift)
			}
		case "/v1/embeddings":
			writeJSON(t, w, embeddingResponse{Object: "list", Data: []embeddingData{vectorData(0, 1)}, Model: model.ModelName, ModelInfo: model})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := fixtureClient(t, server.URL+"/v1", model, Config{})

	result, err := client.EmbedQuery(context.Background(), "query")
	if !IsKind(err, ErrorModelMetadataDrift) || !reflect.DeepEqual(result, Embedding{}) {
		t.Fatalf("EmbedQuery() = %+v, %v", result, err)
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	model := fixtureModel()
	t.Run("timeout", func(t *testing.T) {
		server := blockingEmbeddingServer(t, model, nil)
		defer closeBlockingServer(server)
		client := fixtureClient(t, server.URL+"/v1", model, Config{Timeout: 25 * time.Millisecond})
		result, err := client.EmbedQuery(context.Background(), "query")
		if !IsKind(err, ErrorTimeout) || !errors.Is(err, context.DeadlineExceeded) || !reflect.DeepEqual(result, Embedding{}) {
			t.Fatalf("EmbedQuery() = %+v, %v", result, err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		server := blockingEmbeddingServer(t, model, started)
		defer closeBlockingServer(server)
		client := fixtureClient(t, server.URL+"/v1", model, Config{Timeout: time.Second})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := client.EmbedQuery(ctx, "query")
			done <- err
		}()
		<-started
		cancel()
		err := <-done
		if !IsKind(err, ErrorCancelled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("EmbedQuery() error = %v", err)
		}
		var adapterErr *AdapterError
		if !errors.As(err, &adapterErr) || adapterErr.Status().Mode != ServiceFailed {
			t.Fatalf("cancelled status = %+v", adapterErr)
		}
	})
}

func TestClientRejectsRemoteAndInvalidConfiguration(t *testing.T) {
	model := fixtureModel()
	tests := []Config{
		{Endpoint: "https://api.example.com/v1", ExpectedModel: model},
		{Endpoint: "file:///tmp/embedding", ExpectedModel: model},
		{Endpoint: "http://embedding-service/v1", ExpectedModel: EmbeddingModelInfo{}},
		{Endpoint: "http://embedding-service/v1", ExpectedModel: model, MaxInputBytes: 2, MaxBatchBytes: 1},
	}
	for _, config := range tests {
		if _, err := NewClient(config); err == nil {
			t.Fatalf("NewClient(%+v) error = nil", config)
		}
	}
	if _, err := NewClient(Config{Endpoint: "http://embedding-service:8080/v1", ExpectedModel: model}); err != nil {
		t.Fatalf("NewClient(local Compose endpoint) error = %v", err)
	}
}

func fixtureModel() EmbeddingModelInfo {
	return EmbeddingModelInfo{
		SchemaVersion:  ModelInfoSchemaVersion,
		ModelName:      "fixture/code-embed",
		Revision:       "fixture-revision-1",
		Dimensions:     3,
		Pooling:        "mean",
		Normalization:  "l2",
		Quantization:   "fp16",
		ArtifactSHA256: strings.Repeat("a", 64),
	}
}

func fixtureClient(t *testing.T, endpoint string, model EmbeddingModelInfo, overrides Config) *Client {
	t.Helper()
	overrides.Endpoint = endpoint
	overrides.ExpectedModel = model
	client, err := NewClient(overrides)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func newFixtureServer(t *testing.T, model EmbeddingModelInfo, embeddingHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			if r.Method != http.MethodGet {
				t.Errorf("health method = %s", r.Method)
			}
			writeJSON(t, w, healthResponse{Status: "ok"})
		case "/v1/metadata":
			if r.Method != http.MethodGet {
				t.Errorf("metadata method = %s", r.Method)
			}
			writeJSON(t, w, model)
		case "/v1/embeddings":
			if r.Method != http.MethodPost {
				t.Errorf("embeddings method = %s", r.Method)
			}
			if embeddingHandler == nil {
				writeJSON(t, w, embeddingResponse{Object: "list", Data: []embeddingData{vectorData(0, 1)}, Model: model.ModelName, ModelInfo: model})
				return
			}
			embeddingHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

func blockingEmbeddingServer(t *testing.T, model EmbeddingModelInfo, started chan<- struct{}) *httptest.Server {
	t.Helper()
	return newFixtureServer(t, model, func(w http.ResponseWriter, r *http.Request) {
		if started != nil {
			close(started)
		}
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	})
}

func closeBlockingServer(server *httptest.Server) {
	server.CloseClientConnections()
	server.Close()
}

func vectorData(index int, start float64) embeddingData {
	return embeddingData{Object: "embedding", Index: index, Embedding: rawValues(start, start+0.25, start+0.5)}
}

func rawValues(values ...float64) []json.RawMessage {
	result := make([]json.RawMessage, len(values))
	for i, value := range values {
		result[i] = json.RawMessage(strconvFormat(value))
	}
	return result
}

func strconvFormat(value float64) string {
	return fmt.Sprintf("%g", value)
}

func decodeRequest(t *testing.T, r *http.Request, target any) {
	t.Helper()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Errorf("decode request: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
