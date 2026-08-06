package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"revolvr/internal/redact"
)

const (
	testAPIKey = "fake-api-key-not-live"
	secret     = "TOP-SECRET-SENTINEL"
)

func TestExactRequestIdentityAndStrictSchemaTransmission(t *testing.T) {
	req := testRequest(t, "request-exact", "run-exact")
	prepared := mustPrepare(t, req)
	var recorded recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded = recordRequest(t, r)
		writeCompleted(t, w, recorded.Body, validOutput(recorded.Body, "accepted"))
	}))
	defer server.Close()

	client := testClient(t, server, nil, nil)
	result, err := client.Invoke(context.Background(), prepared)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Outcome != OutcomeSuccess {
		t.Fatalf("Invoke() outcome = %q", result.Outcome)
	}
	if recorded.Method != http.MethodPost || recorded.Path != "/v1/responses" {
		t.Fatalf("request = %s %s", recorded.Method, recorded.Path)
	}
	if recorded.ContentType != "application/json" || recorded.Accept != "text/event-stream" {
		t.Fatalf("request media types = %q / %q", recorded.ContentType, recorded.Accept)
	}
	if recorded.ClientRequestID != req.RequestID {
		t.Fatalf("X-Client-Request-Id = %q", recorded.ClientRequestID)
	}
	if recorded.Authorization != "Bearer "+testAPIKey {
		t.Fatal("Authorization header did not carry the in-memory fake credential")
	}
	if recorded.Body.Model != req.Model || recorded.Body.Input != req.Prompt || recorded.Body.Reasoning.Effort != req.ReasoningEffort || recorded.Body.MaxOutputTokens != req.MaxOutputTokens {
		t.Fatalf("request model settings = %#v", recorded.Body)
	}
	if !recorded.Body.Stream || recorded.Body.Store {
		t.Fatalf("request stream/store = %v/%v", recorded.Body.Stream, recorded.Body.Store)
	}
	if recorded.HasPreviousResponse || recorded.HasConversation {
		t.Fatal("fresh request transmitted hidden conversation authority")
	}
	format := recorded.Body.Text.Format
	if format.Type != "json_schema" || format.Name != req.ResponseSchemaName || !format.Strict {
		t.Fatalf("text.format = %#v", format)
	}
	var gotSchema, wantSchema any
	if err := json.Unmarshal(format.Schema, &gotSchema); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(req.ResponseSchema, &wantSchema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotSchema, wantSchema) {
		t.Fatal("transmitted response schema differs from prepared schema")
	}
	for key, want := range prepared.metadata {
		if got := recorded.Body.Metadata[key]; got != want {
			t.Fatalf("metadata[%q] = %q, want %q", key, got, want)
		}
	}
	if len(recorded.Body.Metadata) != len(prepared.metadata) {
		t.Fatalf("metadata count = %d, want %d", len(recorded.Body.Metadata), len(prepared.metadata))
	}
}

func TestPrepareRejectsNonTransientRetryStatus(t *testing.T) {
	req := testRequest(t, "request-invalid-retry", "run-invalid-retry")
	req.Retry.RetryableStatusCodes = append(req.Retry.RetryableStatusCodes, http.StatusUnauthorized)
	if _, err := Prepare(req); err == nil || !strings.Contains(err.Error(), "not an admitted transient") {
		t.Fatalf("Prepare() error = %v", err)
	}
}

func TestTypedStreamingCompletedResponseIsAuthoritative(t *testing.T) {
	req := testRequest(t, "request-stream", "run-stream")
	prepared := mustPrepare(t, req)
	var exactCompleted []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := recordRequest(t, r)
		startSSE(w)
		writeSSE(t, w, 0, "response.created", map[string]any{"response": map[string]any{"status": "in_progress"}})
		writeSSE(t, w, 1, "response.output_text.delta", map[string]any{"delta": `{"answer":"partial-and-not-authority`})
		exactCompleted = fakeCompletedResponse(t, recorded.Body, validOutput(recorded.Body, "final-authority"), "completed")
		writeSSE(t, w, 2, "response.completed", map[string]any{"response": json.RawMessage(exactCompleted)})
	}))
	defer server.Close()

	result, err := testClient(t, server, nil, nil).Invoke(context.Background(), prepared)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if string(result.CompletedResponse) != string(exactCompleted) {
		t.Fatal("completed response bytes were not retained exactly")
	}
	var output map[string]any
	if err := json.Unmarshal(result.StructuredOutput, &output); err != nil {
		t.Fatal(err)
	}
	if output["answer"] != "final-authority" {
		t.Fatalf("structured answer = %#v", output["answer"])
	}
	if len(result.Diagnostics) != 3 || !strings.Contains(result.Diagnostics[1].Text, "partial-and-not-authority") {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if !result.Usage.Available || result.Usage.InputTokens != 11 || result.Usage.CachedTokens != 3 || result.Usage.CacheWriteTokens != 2 || result.Usage.OutputTokens != 7 || result.Usage.ReasoningTokens != 4 || result.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if result.Service.ServiceTier != "default" || result.Service.Model != req.Model || result.Service.OpenAIRequestID != "req_fake" {
		t.Fatalf("service = %#v", result.Service)
	}
	if !result.Latency.ServerAvailable || result.Latency.Server != 2*time.Second || result.Latency.Total <= 0 {
		t.Fatalf("latency = %#v", result.Latency)
	}
	if result.Cost.Available || result.Cost.Source != "not_reported_by_responses_api" {
		t.Fatalf("cost = %#v", result.Cost)
	}
}

func TestFreshCallIsolation(t *testing.T) {
	var mu sync.Mutex
	var calls []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := recordRequest(t, r)
		mu.Lock()
		calls = append(calls, recorded)
		mu.Unlock()
		writeCompleted(t, w, recorded.Body, validOutput(recorded.Body, recorded.Body.Input))
	}))
	defer server.Close()
	client := testClient(t, server, nil, nil)

	first := testRequest(t, "request-fresh-a", "run-fresh-a")
	first.Prompt = "first independent prompt"
	first.PromptSHA256 = SHA256([]byte(first.Prompt))
	second := testRequest(t, "request-fresh-b", "run-fresh-b")
	second.Prompt = "second independent prompt"
	second.PromptSHA256 = SHA256([]byte(second.Prompt))
	for _, req := range []Request{first, second} {
		result, err := client.Invoke(context.Background(), mustPrepare(t, req))
		if err != nil || result.Outcome != OutcomeSuccess {
			t.Fatalf("Invoke(%q) = %q, %v", req.RequestID, result.Outcome, err)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	if calls[0].Body.Input == calls[1].Body.Input || calls[0].ClientRequestID == calls[1].ClientRequestID {
		t.Fatal("independent calls reused prompt or request identity")
	}
	for _, call := range calls {
		if call.HasPreviousResponse || call.HasConversation || call.Body.Store {
			t.Fatal("independent call carried hidden session state")
		}
	}
}

func TestSemanticOutcomesNeverRetry(t *testing.T) {
	tests := []struct {
		name    string
		content func(createRequest) any
		want    Outcome
	}{
		{name: "refusal", content: func(createRequest) any { return refusalContent("I cannot help with that.") }, want: OutcomeSafetyRefusal},
		{name: "malformed JSON", content: func(createRequest) any { return outputContent(`{"answer":`) }, want: OutcomeMalformedJSON},
		{name: "schema mismatch", content: func(body createRequest) any {
			value := validOutput(body, "unused")
			value["answer"] = 17
			return outputContent(mustJSON(t, value))
		}, want: OutcomeSchemaInvalid},
		{name: "stale identity", content: func(body createRequest) any {
			value := validOutput(body, "unused")
			identity := value["revolvr_identity"].(OutputIdentity)
			identity.TaskID = "task-stale"
			value["revolvr_identity"] = identity
			return outputContent(mustJSON(t, value))
		}, want: OutcomeStaleIdentity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				recorded := recordRequest(t, r)
				writeCompleted(t, w, recorded.Body, test.content(recorded.Body))
			}))
			defer server.Close()
			req := testRequest(t, "request-semantic", "run-semantic")
			req.Retry.MaxAttempts = 3
			result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
			if err == nil {
				t.Fatal("Invoke() error = nil")
			}
			if result.Outcome != test.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome, test.want)
			}
			if calls.Load() != 1 || len(result.Attempts) != 1 {
				t.Fatalf("semantic failure calls/attempts = %d/%d", calls.Load(), len(result.Attempts))
			}
		})
	}
}

func TestOversizedStreamTimeoutAndCancellation(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = recordRequest(t, r)
			startSSE(w)
			writeSSE(t, w, 0, "response.output_text.delta", map[string]any{"delta": strings.Repeat("x", 1024)})
		}))
		defer server.Close()
		req := testRequest(t, "request-oversized", "run-oversized")
		req.MaxStreamBytes = 128
		result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
		assertOutcomeError(t, result, err, OutcomeOversizedStream)
	})

	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = recordRequest(t, r)
			startSSE(w)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		}))
		defer server.Close()
		req := testRequest(t, "request-timeout", "run-timeout")
		req.Timeout = 30 * time.Millisecond
		result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
		assertOutcomeError(t, result, err, OutcomeTimeout)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = recordRequest(t, r)
			startSSE(w)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(started)
			<-r.Context().Done()
		}))
		defer server.Close()
		req := testRequest(t, "request-cancel", "run-cancel")
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-started
			cancel()
		}()
		result, err := testClient(t, server, nil, nil).Invoke(ctx, mustPrepare(t, req))
		assertOutcomeError(t, result, err, OutcomeCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	})
}

func TestRetryClassification(t *testing.T) {
	t.Run("retryable service then success", func(t *testing.T) {
		var calls atomic.Int32
		var clientRequestIDs []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorded := recordRequest(t, r)
			clientRequestIDs = append(clientRequestIDs, recorded.ClientRequestID)
			if calls.Add(1) == 1 {
				writeAPIError(w, http.StatusInternalServerError, "server_error", "temporary")
				return
			}
			writeCompleted(t, w, recorded.Body, validOutput(recorded.Body, "recovered"))
		}))
		defer server.Close()
		req := testRequest(t, "request-service-retry", "run-service-retry")
		req.Retry.MaxAttempts = 2
		result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
		if err != nil || result.Outcome != OutcomeSuccess {
			t.Fatalf("Invoke() = %q, %v", result.Outcome, err)
		}
		if len(result.Attempts) != 2 || result.Attempts[0].Outcome != OutcomeRetryableService || !result.Attempts[0].Retryable || result.Attempts[1].Outcome != OutcomeSuccess {
			t.Fatalf("attempts = %#v", result.Attempts)
		}
		if len(clientRequestIDs) != 2 || clientRequestIDs[0] != req.RequestID || clientRequestIDs[1] == clientRequestIDs[0] || result.Attempts[1].ClientRequestID != clientRequestIDs[1] {
			t.Fatalf("transport request IDs = %#v", clientRequestIDs)
		}
	})

	t.Run("typed stream service error then success", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorded := recordRequest(t, r)
			if calls.Add(1) == 1 {
				startSSE(w)
				writeSSE(t, w, 0, "error", map[string]any{"code": "temporarily_unavailable", "message": "try later", "param": nil})
				return
			}
			writeCompleted(t, w, recorded.Body, validOutput(recorded.Body, "recovered"))
		}))
		defer server.Close()
		req := testRequest(t, "request-stream-retry", "run-stream-retry")
		req.Retry.MaxAttempts = 2
		req.Retry.RetryableStreamErrCodes = []string{"temporarily_unavailable"}
		result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
		if err != nil || result.Outcome != OutcomeSuccess || result.Attempts[0].Outcome != OutcomeRetryableService {
			t.Fatalf("Invoke() = %#v, %v", result, err)
		}
	})

	t.Run("retryable transport then success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorded := recordRequest(t, r)
			writeCompleted(t, w, recorded.Body, validOutput(recorded.Body, "transport-recovered"))
		}))
		defer server.Close()
		flaky := &flakyHTTPClient{next: server.Client()}
		req := testRequest(t, "request-transport-retry", "run-transport-retry")
		req.Retry.MaxAttempts = 2
		result, err := testClient(t, server, flaky, nil).Invoke(context.Background(), mustPrepare(t, req))
		if err != nil || result.Outcome != OutcomeSuccess || len(result.Attempts) != 2 || result.Attempts[0].Outcome != OutcomeRetryableTransport {
			t.Fatalf("Invoke() = %#v, %v", result, err)
		}
	})

	t.Run("nonretryable and quota failures", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			status    int
			code      string
			errorType string
		}{
			{name: "bad request", status: http.StatusBadRequest, code: "invalid_request_error", errorType: "api_error"},
			{name: "quota code", status: http.StatusTooManyRequests, code: "insufficient_quota", errorType: "api_error"},
			{name: "quota type", status: http.StatusTooManyRequests, errorType: "insufficient_quota"},
		} {
			t.Run(test.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_ = recordRequest(t, r)
					calls.Add(1)
					writeAPIErrorType(w, test.status, test.code, test.errorType, "operator action required")
				}))
				defer server.Close()
				req := testRequest(t, "request-nonretry", "run-nonretry")
				req.Retry.MaxAttempts = 3
				result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
				assertOutcomeError(t, result, err, OutcomeNonRetryableFailure)
				if calls.Load() != 1 {
					t.Fatalf("calls = %d", calls.Load())
				}
			})
		}
	})

	t.Run("exhausted", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = recordRequest(t, r)
			calls.Add(1)
			writeAPIError(w, http.StatusServiceUnavailable, "server_overloaded", "temporary")
		}))
		defer server.Close()
		req := testRequest(t, "request-exhaust", "run-exhaust")
		req.Retry.MaxAttempts = 2
		result, err := testClient(t, server, nil, nil).Invoke(context.Background(), mustPrepare(t, req))
		assertOutcomeError(t, result, err, OutcomeRetriesExhausted)
		if calls.Load() != 2 || len(result.Attempts) != 2 {
			t.Fatalf("calls/attempts = %d/%d", calls.Load(), len(result.Attempts))
		}
		for _, attempt := range result.Attempts {
			if attempt.Outcome != OutcomeRetryableService || !attempt.Retryable {
				t.Fatalf("attempt = %#v", attempt)
			}
		}
	})
}

func TestSecretSentinelAbsentFromReturnedAndRecordedSurfaces(t *testing.T) {
	redactorInstance, _, err := redact.New(redact.Policy{EnvironmentVariables: []string{"MODEL_TEST_SECRET"}}, func(name string) (string, bool) {
		return secret, name == "MODEL_TEST_SECRET"
	})
	if err != nil {
		t.Fatal(err)
	}
	var recorded []recordedRequest
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := recordRequest(t, r)
		recorded = append(recorded, request)
		if calls.Add(1) == 1 {
			startSSE(w)
			writeSSE(t, w, 0, "response.output_text.delta", map[string]any{"delta": "diagnostic " + secret})
			response := fakeCompletedResponse(t, request.Body, validOutput(request.Body, "clean"), "completed")
			writeSSE(t, w, 1, "response.completed", map[string]any{"response": json.RawMessage(response)})
			return
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_request_error", "failure mentions "+secret+" and "+testAPIKey)
	}))
	defer server.Close()
	client := testClient(t, server, nil, redactorInstance)

	first := testRequest(t, "request-secret-success", "run-secret-success")
	result, err := client.Invoke(context.Background(), mustPrepare(t, first))
	if err != nil || result.Outcome != OutcomeSuccess {
		t.Fatalf("first Invoke() = %q, %v", result.Outcome, err)
	}
	assertNoSecrets(t, result)
	preparedRaw, err := json.Marshal(mustPrepare(t, first))
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecretBytes(t, preparedRaw)

	second := testRequest(t, "request-secret-error", "run-secret-error")
	secondResult, secondErr := client.Invoke(context.Background(), mustPrepare(t, second))
	if secondErr == nil {
		t.Fatal("second Invoke() error = nil")
	}
	assertNoSecrets(t, secondResult)
	assertNoSecretBytes(t, []byte(secondErr.Error()))
	assertNoSecretBytes(t, []byte(client.String()))
	clientWithSecretEndpoint, err := NewClient(ClientConfig{
		Endpoint: server.URL + "/" + secret, APIKey: testAPIKey, HTTPClient: server.Client(), Redactor: redactorInstance,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecretBytes(t, []byte(clientWithSecretEndpoint.String()))
	if raw, marshalErr := json.Marshal(client); marshalErr == nil || len(raw) != 0 {
		t.Fatal("client unexpectedly serialized credential-bearing state")
	}
	for _, request := range recorded {
		assertNoSecretBytes(t, request.RawBody)
		if request.ClientRequestID == "" || request.Authorization != "Bearer "+testAPIKey {
			t.Fatal("recorded transport request lost required identity or credential")
		}
	}
}

func TestCompletedResponseContainingSecretIsWithheld(t *testing.T) {
	redactorInstance, _, err := redact.New(redact.Policy{EnvironmentVariables: []string{"MODEL_TEST_SECRET"}}, func(string) (string, bool) { return secret, true })
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := recordRequest(t, r)
		writeCompleted(t, w, recorded.Body, validOutput(recorded.Body, secret))
	}))
	defer server.Close()
	result, invokeErr := testClient(t, server, nil, redactorInstance).Invoke(context.Background(), mustPrepare(t, testRequest(t, "request-secret-withheld", "run-secret-withheld")))
	assertOutcomeError(t, result, invokeErr, OutcomeNonRetryableFailure)
	if len(result.CompletedResponse) != 0 || len(result.StructuredOutput) != 0 {
		t.Fatal("secret-bearing completed response was returned")
	}
	assertNoSecrets(t, result)
	assertNoSecretBytes(t, []byte(invokeErr.Error()))
}

func TestConfiguredSecretInPreparedRequestStopsBeforeTransport(t *testing.T) {
	redactorInstance, _, err := redact.New(redact.Policy{EnvironmentVariables: []string{"MODEL_TEST_SECRET"}}, func(string) (string, bool) { return secret, true })
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, test := range []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "request body", mutate: func(req *Request) {
			req.Prompt += secret
			req.PromptSHA256 = SHA256([]byte(req.Prompt))
		}},
		{name: "request evidence", mutate: func(req *Request) {
			req.Retry.RetryableStreamErrCodes = []string{secret}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := testRequest(t, "request-secret-input", "run-secret-input")
			test.mutate(&req)
			result, invokeErr := testClient(t, server, nil, redactorInstance).Invoke(context.Background(), mustPrepare(t, req))
			assertOutcomeError(t, result, invokeErr, OutcomeNonRetryableFailure)
			if calls.Load() != 0 || len(result.Attempts) != 0 {
				t.Fatalf("secret-bearing request calls/attempts = %d/%d", calls.Load(), len(result.Attempts))
			}
			assertNoSecrets(t, result)
			assertNoSecretBytes(t, []byte(invokeErr.Error()))
		})
	}
}

type recordedRequest struct {
	Method              string
	Path                string
	ContentType         string
	Accept              string
	Authorization       string
	ClientRequestID     string
	RawBody             []byte
	Body                createRequest
	HasPreviousResponse bool
	HasConversation     bool
}

func recordRequest(t *testing.T, request *http.Request) recordedRequest {
	t.Helper()
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read request: %v", err)
	}
	var body createRequest
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Errorf("decode request: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Errorf("decode request fields: %v", err)
	}
	_, previous := fields["previous_response_id"]
	_, conversation := fields["conversation"]
	return recordedRequest{
		Method: request.Method, Path: request.URL.Path, ContentType: request.Header.Get("Content-Type"),
		Accept: request.Header.Get("Accept"), Authorization: request.Header.Get("Authorization"),
		ClientRequestID: request.Header.Get("X-Client-Request-Id"), RawBody: raw, Body: body,
		HasPreviousResponse: previous, HasConversation: conversation,
	}
}

func testRequest(t *testing.T, requestID, runID string) Request {
	t.Helper()
	schema := testSchema(t)
	prompt := "Return the requested structured test value."
	return Request{
		RequestID: requestID, TaskID: "task-model-client", RunID: runID,
		SourceRevision: strings.Repeat("a", 64), Model: "gpt-test-pinned", ReasoningEffort: "high", MaxOutputTokens: 512,
		PromptVersion: "prompt-v1", PromptSHA256: SHA256([]byte(prompt)), Prompt: prompt,
		ResponseSchemaVersion: "response-v1", ResponseSchemaSHA256: SHA256(schema), ResponseSchemaName: "revolvr_test_response", ResponseSchema: schema,
		Timeout: time.Second, Retry: RetryPolicy{
			MaxAttempts: 1, BaseDelay: 0, MaxDelay: 0, RetryTransportErrors: true,
			RetryableStatusCodes: []int{408, 429, 500, 502, 503, 504},
		},
	}
}

func testSchema(t *testing.T) json.RawMessage {
	t.Helper()
	identityProperties := map[string]any{}
	identityRequired := []string{"request_id", "task_id", "run_id", "source_revision", "prompt_version", "prompt_sha256", "response_schema_version", "response_schema_sha256"}
	for _, field := range identityRequired {
		identityProperties[field] = map[string]any{"type": "string"}
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"revolvr_identity", "answer"},
		"properties": map[string]any{
			"revolvr_identity": map[string]any{
				"type": "object", "additionalProperties": false, "required": identityRequired, "properties": identityProperties,
			},
			"answer": map[string]any{"type": "string"},
		},
	}
	return json.RawMessage(mustJSON(t, schema))
}

func mustPrepare(t *testing.T, req Request) PreparedRequest {
	t.Helper()
	prepared, err := Prepare(req)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	return prepared
}

func testClient(t *testing.T, server *httptest.Server, httpClient HTTPClient, redactorInstance *redact.Redactor) *Client {
	t.Helper()
	if httpClient == nil {
		httpClient = server.Client()
	}
	client, err := NewClient(ClientConfig{
		Endpoint: server.URL + "/v1/responses", APIKey: testAPIKey, HTTPClient: httpClient, Redactor: redactorInstance,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func validOutput(body createRequest, answer string) map[string]any {
	return map[string]any{
		"revolvr_identity": outputIdentity(body.Metadata),
		"answer":           answer,
	}
}

func outputIdentity(metadata map[string]string) OutputIdentity {
	return OutputIdentity{
		RequestID: metadata["revolvr_request_id"], TaskID: metadata["revolvr_task_id"], RunID: metadata["revolvr_run_id"],
		SourceRevision: metadata["revolvr_source_revision"], PromptVersion: metadata["revolvr_prompt_version"],
		PromptSHA256: metadata["revolvr_prompt_sha256"], ResponseSchemaVersion: metadata["revolvr_schema_version"],
		ResponseSchemaSHA256: metadata["revolvr_schema_sha256"],
	}
}

func outputContent(raw string) map[string]any {
	return map[string]any{"type": "output_text", "text": raw, "annotations": []any{}}
}

func refusalContent(message string) map[string]any {
	return map[string]any{"type": "refusal", "refusal": message}
}

func fakeCompletedResponse(t *testing.T, body createRequest, content any, status string) []byte {
	t.Helper()
	if value, ok := content.(map[string]any); ok {
		if _, isContent := value["type"]; !isContent {
			content = outputContent(mustJSON(t, value))
		}
	}
	response := map[string]any{
		"id": "resp_fake", "object": "response", "created_at": int64(100), "completed_at": int64(102),
		"status": status, "error": nil, "model": body.Model, "previous_response_id": nil, "store": false,
		"service_tier": "default", "metadata": body.Metadata,
		"output": []any{map[string]any{
			"id": "msg_fake", "type": "message", "status": "completed", "role": "assistant", "content": []any{content},
		}},
		"usage": map[string]any{
			"input_tokens": 11, "input_tokens_details": map[string]any{"cached_tokens": 3, "cache_write_tokens": 2},
			"output_tokens": 7, "output_tokens_details": map[string]any{"reasoning_tokens": 4}, "total_tokens": 18,
		},
	}
	return []byte(mustJSON(t, response))
}

func writeCompleted(t *testing.T, writer http.ResponseWriter, body createRequest, content any) {
	t.Helper()
	startSSE(writer)
	writeSSE(t, writer, 0, "response.completed", map[string]any{"response": json.RawMessage(fakeCompletedResponse(t, body, content, "completed"))})
}

func startSSE(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("x-request-id", "req_fake")
	writer.WriteHeader(http.StatusOK)
}

func writeSSE(t *testing.T, writer http.ResponseWriter, sequence int64, eventType string, fields map[string]any) {
	t.Helper()
	payload := map[string]any{"type": eventType, "sequence_number": sequence}
	for key, value := range fields {
		payload[key] = value
	}
	raw := mustJSON(t, payload)
	if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", eventType, raw); err != nil {
		t.Errorf("write SSE: %v", err)
	}
}

func writeAPIError(writer http.ResponseWriter, status int, code, message string) {
	writeAPIErrorType(writer, status, code, "api_error", message)
}

func writeAPIErrorType(writer http.ResponseWriter, status int, code, errorType, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("x-request-id", "req_fake_error")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]any{"code": code, "message": message, "type": errorType, "param": nil}})
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(raw)
}

func assertOutcomeError(t *testing.T, result Result, err error, want Outcome) {
	t.Helper()
	if err == nil {
		t.Fatal("Invoke() error = nil")
	}
	if result.Outcome != want {
		t.Fatalf("outcome = %q, want %q (error %v)", result.Outcome, want, err)
	}
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) || invocationErr.Outcome != want {
		t.Fatalf("typed error = %#v", err)
	}
}

func assertNoSecrets(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecretBytes(t, raw)
}

func assertNoSecretBytes(t *testing.T, raw []byte) {
	t.Helper()
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), testAPIKey) {
		t.Fatal("secret sentinel appeared on a recorded or returned surface")
	}
}

type flakyHTTPClient struct {
	called atomic.Bool
	next   HTTPClient
}

func (c *flakyHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if !c.called.Swap(true) {
		return nil, &url.Error{Op: "Post", URL: "loopback-fake", Err: &netError{}}
	}
	return c.next.Do(request)
}

type netError struct{}

func (*netError) Error() string   { return "temporary fake transport failure" }
func (*netError) Timeout() bool   { return false }
func (*netError) Temporary() bool { return true }
