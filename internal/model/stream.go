package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var errStreamTooLarge = errors.New("Responses API stream exceeded its pinned byte limit")

type DiagnosticEvent struct {
	SequenceNumber int64  `json:"sequence_number"`
	Type           string `json:"type"`
	Text           string `json:"text,omitempty"`
	TruncatedBytes int64  `json:"truncated_bytes,omitempty"`
}

type UsageEvidence struct {
	Available        bool  `json:"available"`
	InputTokens      int64 `json:"input_tokens,omitempty"`
	CachedTokens     int64 `json:"cached_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`
	TotalTokens      int64 `json:"total_tokens,omitempty"`
}

type LatencyEvidence struct {
	Total           time.Duration `json:"total_ns"`
	Server          time.Duration `json:"server_ns,omitempty"`
	ServerAvailable bool          `json:"server_available"`
}

type ServiceEvidence struct {
	OpenAIRequestID string `json:"openai_request_id,omitempty"`
	ResponseID      string `json:"response_id,omitempty"`
	Model           string `json:"model,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
}

type CostEvidence struct {
	Available bool   `json:"available"`
	Currency  string `json:"currency,omitempty"`
	Source    string `json:"source"`
}

type streamEvent struct {
	Type           string          `json:"type"`
	SequenceNumber *int64          `json:"sequence_number"`
	Delta          string          `json:"delta"`
	Text           string          `json:"text"`
	Refusal        string          `json:"refusal"`
	Code           string          `json:"code"`
	Message        string          `json:"message"`
	Param          string          `json:"param"`
	Response       json.RawMessage `json:"response"`
}

type streamResult struct {
	completed   json.RawMessage
	diagnostics []DiagnosticEvent
	outcome     Outcome
	code        string
	message     string
	param       string
	retryable   bool
}

type diagnosticCollector struct {
	remaining int
	events    []DiagnosticEvent
	sanitize  func(string) string
}

func (c *diagnosticCollector) add(sequence int64, eventType, text string) {
	event := DiagnosticEvent{SequenceNumber: sequence, Type: eventType}
	if text != "" {
		text = c.sanitize(text)
		raw := []byte(text)
		if len(raw) <= c.remaining {
			event.Text = text
			c.remaining -= len(raw)
		} else {
			if c.remaining > 0 {
				event.Text = string(raw[:c.remaining])
			}
			event.TruncatedBytes = int64(len(raw) - c.remaining)
			c.remaining = 0
		}
	}
	c.events = append(c.events, event)
}

func consumeStream(ctx context.Context, reader io.Reader, maxBytes int64, maxDiagnostic int, sanitize func(string) string, retryCodes []string) streamResult {
	limited := &sseReader{reader: bufio.NewReaderSize(reader, 32<<10), limit: maxBytes}
	collector := diagnosticCollector{remaining: maxDiagnostic, sanitize: sanitize}
	var eventName string
	var data bytes.Buffer
	var lastSequence int64 = -1

	dispatch := func() *streamResult {
		if data.Len() == 0 {
			eventName = ""
			return nil
		}
		raw := append([]byte(nil), data.Bytes()...)
		data.Reset()
		var event streamEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			result := streamResult{outcome: OutcomeNonRetryableFailure, message: fmt.Sprintf("decode typed Responses API event: %v", err)}
			return &result
		}
		if event.Type == "" || event.SequenceNumber == nil {
			result := streamResult{outcome: OutcomeNonRetryableFailure, message: "typed Responses API event is missing type or sequence_number"}
			return &result
		}
		if eventName != "" && eventName != event.Type {
			result := streamResult{outcome: OutcomeNonRetryableFailure, message: "Responses API SSE event name does not match its typed payload"}
			return &result
		}
		if *event.SequenceNumber <= lastSequence {
			result := streamResult{outcome: OutcomeNonRetryableFailure, message: "Responses API event sequence is not strictly increasing"}
			return &result
		}
		lastSequence = *event.SequenceNumber
		eventName = ""
		text := event.Delta
		if text == "" {
			text = event.Text
		}
		if text == "" {
			text = event.Refusal
		}
		collector.add(lastSequence, event.Type, text)

		switch event.Type {
		case "response.completed":
			if len(event.Response) == 0 {
				result := streamResult{outcome: OutcomeNonRetryableFailure, message: "response.completed is missing the completed response"}
				return &result
			}
			result := streamResult{completed: append(json.RawMessage(nil), event.Response...), diagnostics: collector.events}
			return &result
		case "response.failed", "response.incomplete":
			failure := responseFailure(event.Response)
			failure.diagnostics = collector.events
			if event.Type == "response.incomplete" {
				failure.outcome = OutcomeNonRetryableFailure
				failure.retryable = false
				if failure.message == "" {
					failure.message = "Responses API response was incomplete"
				}
			} else if containsString(retryCodes, failure.code) {
				failure.outcome = OutcomeRetryableService
				failure.retryable = true
			}
			return &failure
		case "error":
			outcome := OutcomeNonRetryableFailure
			retryable := containsString(retryCodes, event.Code)
			if retryable {
				outcome = OutcomeRetryableService
			}
			result := streamResult{diagnostics: collector.events, outcome: outcome, code: event.Code, message: event.Message, param: event.Param, retryable: retryable}
			return &result
		default:
			return nil
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			return streamResult{diagnostics: collector.events, message: err.Error()}
		}
		line, err := limited.readLine()
		if errors.Is(err, errStreamTooLarge) {
			return streamResult{diagnostics: collector.events, outcome: OutcomeOversizedStream, message: err.Error()}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return streamResult{diagnostics: collector.events, outcome: OutcomeRetryableTransport, message: fmt.Sprintf("read Responses API stream: %v", err), retryable: true}
		}
		if len(line) != 0 {
			line = bytes.TrimSuffix(line, []byte{'\n'})
			line = bytes.TrimSuffix(line, []byte{'\r'})
			switch {
			case len(line) == 0:
				if result := dispatch(); result != nil {
					return *result
				}
			case line[0] == ':':
			case bytes.HasPrefix(line, []byte("event:")):
				eventName = strings.TrimSpace(string(line[len("event:"):]))
			case bytes.HasPrefix(line, []byte("data:")):
				value := line[len("data:"):]
				if len(value) > 0 && value[0] == ' ' {
					value = value[1:]
				}
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				_, _ = data.Write(value)
			}
		}
		if errors.Is(err, io.EOF) {
			if result := dispatch(); result != nil {
				return *result
			}
			return streamResult{diagnostics: collector.events, outcome: OutcomeRetryableTransport, message: "Responses API stream ended before a terminal event", retryable: true}
		}
	}
}

type sseReader struct {
	reader *bufio.Reader
	limit  int64
	read   int64
}

func (r *sseReader) readLine() ([]byte, error) {
	var line []byte
	for {
		fragment, err := r.reader.ReadSlice('\n')
		r.read += int64(len(fragment))
		if r.read > r.limit {
			return nil, errStreamTooLarge
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

type completedResponse struct {
	ID                 string            `json:"id"`
	Object             string            `json:"object"`
	CreatedAt          int64             `json:"created_at"`
	CompletedAt        *int64            `json:"completed_at"`
	Status             string            `json:"status"`
	Error              *responseError    `json:"error"`
	Model              string            `json:"model"`
	Output             []outputItem      `json:"output"`
	PreviousResponseID json.RawMessage   `json:"previous_response_id"`
	Store              *bool             `json:"store"`
	ServiceTier        string            `json:"service_tier"`
	Usage              *responseUsage    `json:"usage"`
	Metadata           map[string]string `json:"metadata"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param"`
}

type outputItem struct {
	Type    string        `json:"type"`
	Role    string        `json:"role"`
	Status  string        `json:"status"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type responseUsage struct {
	InputTokens        int64              `json:"input_tokens"`
	InputTokensDetails inputTokenDetails  `json:"input_tokens_details"`
	OutputTokens       int64              `json:"output_tokens"`
	OutputTokenDetails outputTokenDetails `json:"output_tokens_details"`
	TotalTokens        int64              `json:"total_tokens"`
}

type inputTokenDetails struct {
	CachedTokens     int64 `json:"cached_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

type outputTokenDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type validatedResponse struct {
	outcome         Outcome
	structured      json.RawMessage
	refusal         string
	usage           UsageEvidence
	serverTime      time.Duration
	serverAvailable bool
	service         ServiceEvidence
	message         string
}

func validateCompleted(raw json.RawMessage, prepared PreparedRequest) validatedResponse {
	var response completedResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return validatedResponse{outcome: OutcomeNonRetryableFailure, message: fmt.Sprintf("decode response.completed response: %v", err)}
	}
	if response.Object != "response" || response.Status != "completed" || response.ID == "" {
		return validatedResponse{outcome: OutcomeNonRetryableFailure, message: "response.completed does not contain one completed response object"}
	}
	if response.Error != nil {
		return validatedResponse{outcome: OutcomeNonRetryableFailure, message: response.Error.Message}
	}
	if response.Store == nil || *response.Store || !bytes.Equal(bytes.TrimSpace(response.PreviousResponseID), []byte("null")) {
		return validatedResponse{outcome: OutcomeStaleIdentity, message: "completed response does not retain fresh stateless request authority"}
	}
	for key, want := range prepared.metadata {
		if response.Metadata[key] != want {
			return validatedResponse{outcome: OutcomeStaleIdentity, message: fmt.Sprintf("completed response metadata %q does not match pinned request identity", key)}
		}
	}

	var texts []string
	var refusals []string
	for _, item := range response.Output {
		switch item.Type {
		case "reasoning":
			continue
		case "message":
			if item.Role != "assistant" || item.Status != "completed" {
				return validatedResponse{outcome: OutcomeNonRetryableFailure, message: "completed response contains a non-completed assistant message"}
			}
			for _, part := range item.Content {
				switch part.Type {
				case "output_text":
					texts = append(texts, part.Text)
				case "refusal":
					refusals = append(refusals, part.Refusal)
				default:
					return validatedResponse{outcome: OutcomeNonRetryableFailure, message: fmt.Sprintf("completed response contains unsupported message content %q", part.Type)}
				}
			}
		default:
			return validatedResponse{outcome: OutcomeNonRetryableFailure, message: fmt.Sprintf("completed response contains unexpected output item %q", item.Type)}
		}
	}
	usage, usageErr := validateUsage(response.Usage)
	if usageErr != nil {
		return validatedResponse{outcome: OutcomeNonRetryableFailure, message: usageErr.Error()}
	}
	serverTime := time.Duration(0)
	if response.CompletedAt != nil && *response.CompletedAt >= response.CreatedAt {
		serverTime = time.Duration(*response.CompletedAt-response.CreatedAt) * time.Second
	}
	base := validatedResponse{
		usage:           usage,
		serverTime:      serverTime,
		serverAvailable: response.CompletedAt != nil && *response.CompletedAt >= response.CreatedAt,
		service:         ServiceEvidence{ResponseID: response.ID, Model: response.Model, ServiceTier: response.ServiceTier},
	}
	if len(refusals) != 0 {
		if len(refusals) != 1 || len(texts) != 0 || strings.TrimSpace(refusals[0]) == "" {
			base.outcome = OutcomeNonRetryableFailure
			base.message = "completed response contains ambiguous refusal content"
			return base
		}
		base.outcome = OutcomeSafetyRefusal
		base.refusal = refusals[0]
		return base
	}
	if len(texts) != 1 {
		base.outcome = OutcomeNonRetryableFailure
		base.message = "completed response must contain exactly one structured output_text part"
		return base
	}
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(texts[0]))
	if err != nil {
		base.outcome = OutcomeMalformedJSON
		base.message = fmt.Sprintf("decode final structured output: %v", err)
		return base
	}
	if err := prepared.validator.Validate(value); err != nil {
		base.outcome = OutcomeSchemaInvalid
		base.message = fmt.Sprintf("validate final structured output: %v", err)
		return base
	}
	if err := validateOutputIdentity(value, prepared.outputIdentity); err != nil {
		base.outcome = OutcomeStaleIdentity
		base.message = err.Error()
		return base
	}
	base.outcome = OutcomeSuccess
	base.structured = json.RawMessage(texts[0])
	return base
}

func validateOutputIdentity(value any, expected OutputIdentity) error {
	object, ok := value.(map[string]any)
	if !ok {
		return errors.New("final structured output is not an identity-bearing object")
	}
	rawIdentity, ok := object["revolvr_identity"]
	if !ok {
		return errors.New("final structured output is missing revolvr_identity")
	}
	raw, err := json.Marshal(rawIdentity)
	if err != nil {
		return fmt.Errorf("encode final structured output identity: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var actual OutputIdentity
	if err := decoder.Decode(&actual); err != nil {
		return fmt.Errorf("decode final structured output identity: %w", err)
	}
	if actual != expected {
		return errors.New("final structured output identity does not match pinned request identity")
	}
	return nil
}

func validateUsage(usage *responseUsage) (UsageEvidence, error) {
	if usage == nil {
		return UsageEvidence{}, errors.New("completed response is missing usage metadata")
	}
	values := []int64{
		usage.InputTokens, usage.InputTokensDetails.CachedTokens, usage.InputTokensDetails.CacheWriteTokens,
		usage.OutputTokens, usage.OutputTokenDetails.ReasoningTokens, usage.TotalTokens,
	}
	for _, value := range values {
		if value < 0 {
			return UsageEvidence{}, errors.New("completed response contains negative usage metadata")
		}
	}
	if usage.InputTokensDetails.CachedTokens > usage.InputTokens || usage.OutputTokenDetails.ReasoningTokens > usage.OutputTokens || usage.TotalTokens != usage.InputTokens+usage.OutputTokens {
		return UsageEvidence{}, errors.New("completed response contains inconsistent usage metadata")
	}
	return UsageEvidence{
		Available: true, InputTokens: usage.InputTokens, CachedTokens: usage.InputTokensDetails.CachedTokens,
		CacheWriteTokens: usage.InputTokensDetails.CacheWriteTokens, OutputTokens: usage.OutputTokens,
		ReasoningTokens: usage.OutputTokenDetails.ReasoningTokens, TotalTokens: usage.TotalTokens,
	}, nil
}

func responseFailure(raw json.RawMessage) streamResult {
	var response completedResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return streamResult{outcome: OutcomeNonRetryableFailure, message: fmt.Sprintf("decode failed Responses API response: %v", err)}
	}
	if response.Error == nil {
		return streamResult{outcome: OutcomeNonRetryableFailure, message: "Responses API emitted a failed response without error details"}
	}
	return streamResult{outcome: OutcomeNonRetryableFailure, code: response.Error.Code, message: response.Error.Message, param: response.Error.Param}
}
