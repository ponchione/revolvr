package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/redact"
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type ClientConfig struct {
	Endpoint   string
	APIKey     string
	HTTPClient HTTPClient
	Redactor   *redact.Redactor
}

type Client struct {
	endpoint   string
	apiKey     string
	httpClient HTTPClient
	redactor   *redact.Redactor
}

func NewClient(config ClientConfig) (*Client, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("model client: endpoint must be an absolute URL without query or fragment")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil, errors.New("model client: plaintext HTTP is permitted only for a loopback fake server")
	}
	if strings.TrimSpace(config.APIKey) != config.APIKey || len(config.APIKey) < 4 || strings.IndexAny(config.APIKey, "\r\n") >= 0 {
		return nil, errors.New("model client: an in-memory normalized API credential is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{endpoint: endpoint, apiKey: config.APIKey, httpClient: httpClient, redactor: config.Redactor}, nil
}

type AttemptEvidence struct {
	Attempt         int           `json:"attempt"`
	ClientRequestID string        `json:"client_request_id"`
	Outcome         Outcome       `json:"outcome"`
	StatusCode      int           `json:"status_code,omitempty"`
	OpenAIRequestID string        `json:"openai_request_id,omitempty"`
	Latency         time.Duration `json:"latency_ns"`
	RetryAfter      time.Duration `json:"retry_after_ns,omitempty"`
	ErrorCode       string        `json:"error_code,omitempty"`
	ErrorType       string        `json:"error_type,omitempty"`
	ErrorParam      string        `json:"error_param,omitempty"`
	Detail          string        `json:"detail,omitempty"`
	Retryable       bool          `json:"retryable"`
}

type Result struct {
	Outcome           Outcome           `json:"outcome"`
	Request           RequestEvidence   `json:"request"`
	Attempts          []AttemptEvidence `json:"attempts"`
	Diagnostics       []DiagnosticEvent `json:"diagnostics,omitempty"`
	CompletedResponse json.RawMessage   `json:"completed_response,omitempty"`
	StructuredOutput  json.RawMessage   `json:"structured_output,omitempty"`
	Refusal           string            `json:"refusal,omitempty"`
	Usage             UsageEvidence     `json:"usage"`
	Latency           LatencyEvidence   `json:"latency"`
	Service           ServiceEvidence   `json:"service"`
	Cost              CostEvidence      `json:"cost"`
}

type InvocationError struct {
	Outcome    Outcome
	Attempt    int
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
	cause      error
}

func (e *InvocationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *InvocationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

type attemptResult struct {
	evidence    AttemptEvidence
	diagnostics []DiagnosticEvent
	completed   json.RawMessage
	retryable   bool
	cause       error
}

func (c *Client) Invoke(ctx context.Context, prepared PreparedRequest) (Result, error) {
	if c == nil {
		return Result{}, errors.New("model client: client is nil")
	}
	if prepared.validator == nil || len(prepared.body) == 0 {
		return Result{}, errors.New("model client: request was not prepared")
	}
	started := time.Now()
	invocationContext, cancel := context.WithTimeout(ctx, prepared.timeout)
	defer cancel()
	result := Result{
		Cost: CostEvidence{Available: false, Source: "not_reported_by_responses_api"},
	}
	evidenceRaw, err := json.Marshal(prepared.evidence)
	if err != nil {
		result.Outcome = OutcomeNonRetryableFailure
		return result, c.invocationError(result.Outcome, 0, AttemptEvidence{}, "prepared request evidence could not be encoded", nil)
	}
	if c.containsSecret(prepared.body) || c.containsSecret(evidenceRaw) {
		result.Outcome = OutcomeNonRetryableFailure
		return result, c.invocationError(result.Outcome, 0, AttemptEvidence{}, "prepared request contains a configured secret and was not sent", nil)
	}
	result.Request = cloneRequestEvidence(prepared.evidence)

	for attempt := 1; attempt <= prepared.retry.MaxAttempts; attempt++ {
		current := c.invokeAttempt(invocationContext, prepared, attempt)
		result.Attempts = append(result.Attempts, current.evidence)
		result.Diagnostics = append(result.Diagnostics, current.diagnostics...)
		if current.evidence.Outcome == OutcomeSuccess && len(current.completed) != 0 {
			if c.containsSecret(current.completed) {
				result.Outcome = OutcomeNonRetryableFailure
				result.Attempts[len(result.Attempts)-1].Outcome = result.Outcome
				result.Attempts[len(result.Attempts)-1].Detail = "completed response contained a configured secret and was withheld"
				result.Latency.Total = time.Since(started)
				return result, c.invocationError(result.Outcome, attempt, current.evidence, "completed response contained a configured secret and was withheld", nil)
			}
			validated := validateCompleted(current.completed, prepared)
			result.Attempts[len(result.Attempts)-1].Outcome = validated.outcome
			result.Attempts[len(result.Attempts)-1].Detail = c.sanitize(validated.message)
			result.Outcome = validated.outcome
			result.CompletedResponse = append(json.RawMessage(nil), current.completed...)
			result.StructuredOutput = validated.structured
			result.Refusal = c.sanitize(validated.refusal)
			result.Usage = validated.usage
			result.Service = validated.service
			result.Service.OpenAIRequestID = current.evidence.OpenAIRequestID
			result.Latency.Server = validated.serverTime
			result.Latency.ServerAvailable = validated.serverAvailable
			result.Latency.Total = time.Since(started)
			if validated.outcome == OutcomeSuccess {
				return result, nil
			}
			message := validated.message
			if validated.outcome == OutcomeSafetyRefusal {
				message = "OpenAI safety refusal"
			}
			return result, c.invocationError(validated.outcome, attempt, current.evidence, message, nil)
		}
		if !current.retryable {
			result.Outcome = current.evidence.Outcome
			result.Latency.Total = time.Since(started)
			return result, c.invocationError(result.Outcome, attempt, current.evidence, current.evidence.Detail, current.cause)
		}
		if attempt == prepared.retry.MaxAttempts {
			result.Outcome = OutcomeRetriesExhausted
			result.Latency.Total = time.Since(started)
			message := fmt.Sprintf("Responses API retry policy exhausted after %d attempt(s)", attempt)
			return result, c.invocationError(result.Outcome, attempt, current.evidence, message, current.cause)
		}
		delay := retryDelay(prepared.retry, attempt, current.evidence.RetryAfter)
		if err := waitRetry(invocationContext, delay); err != nil {
			outcome := OutcomeCancelled
			if errors.Is(err, context.DeadlineExceeded) {
				outcome = OutcomeTimeout
			}
			result.Outcome = outcome
			result.Latency.Total = time.Since(started)
			return result, c.invocationError(outcome, attempt, current.evidence, err.Error(), err)
		}
	}
	panic("unreachable model retry loop")
}

func (c *Client) invokeAttempt(ctx context.Context, prepared PreparedRequest, attempt int) attemptResult {
	started := time.Now()
	clientRequestID := transportRequestID(prepared.evidence.RequestID, attempt)
	evidence := AttemptEvidence{Attempt: attempt, ClientRequestID: clientRequestID}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(prepared.body))
	if err != nil {
		evidence.Outcome = OutcomeNonRetryableFailure
		evidence.Detail = c.sanitize(fmt.Sprintf("construct Responses API request: %v", err))
		evidence.Latency = time.Since(started)
		return attemptResult{evidence: evidence}
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("X-Client-Request-Id", clientRequestID)

	response, err := c.httpClient.Do(request)
	if err != nil {
		evidence.Latency = time.Since(started)
		if ctxErr := ctx.Err(); ctxErr != nil {
			evidence.Outcome = OutcomeCancelled
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				evidence.Outcome = OutcomeTimeout
			}
			evidence.Detail = c.sanitize(ctxErr.Error())
			return attemptResult{evidence: evidence, cause: ctxErr}
		}
		evidence.Outcome = OutcomeNonRetryableFailure
		evidence.Detail = c.sanitize(fmt.Sprintf("Responses API transport: %v", err))
		if prepared.retry.RetryTransportErrors && isTransportError(err) {
			evidence.Outcome = OutcomeRetryableTransport
			evidence.Retryable = true
		}
		return attemptResult{evidence: evidence, retryable: evidence.Retryable, cause: errors.New(evidence.Detail)}
	}
	defer response.Body.Close()
	evidence.StatusCode = response.StatusCode
	evidence.OpenAIRequestID = c.sanitize(strings.TrimSpace(response.Header.Get("x-request-id")))
	evidence.RetryAfter = retryAfter(response)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		apiErr, bodyErr := readAPIError(response.Body)
		evidence.ErrorCode = c.sanitize(apiErr.Code)
		evidence.ErrorType = c.sanitize(apiErr.Type)
		evidence.ErrorParam = c.sanitize(apiErr.Param)
		evidence.Detail = c.sanitize(apiErr.Message)
		if evidence.Detail == "" {
			evidence.Detail = http.StatusText(response.StatusCode)
		}
		if bodyErr != nil {
			evidence.Detail = c.sanitize(fmt.Sprintf("read Responses API error: %v", bodyErr))
		}
		evidence.Outcome = OutcomeNonRetryableFailure
		if retryableHTTP(response.StatusCode, apiErr.Code, apiErr.Type, prepared.retry) {
			evidence.Outcome = OutcomeRetryableService
			evidence.Retryable = true
		}
		evidence.Latency = time.Since(started)
		return attemptResult{evidence: evidence, retryable: evidence.Retryable}
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.HasPrefix(mediaType, "text/event-stream") {
		evidence.Outcome = OutcomeNonRetryableFailure
		evidence.Detail = "Responses API returned a non-streaming content type"
		evidence.Latency = time.Since(started)
		return attemptResult{evidence: evidence}
	}

	stream := consumeStream(ctx, response.Body, prepared.maxStream, prepared.maxDiagnostic, c.sanitize, prepared.retry.RetryableStreamErrCodes)
	evidence.Latency = time.Since(started)
	if len(stream.completed) != 0 {
		evidence.Outcome = OutcomeSuccess
		return attemptResult{evidence: evidence, diagnostics: stream.diagnostics, completed: stream.completed}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		evidence.Outcome = OutcomeCancelled
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			evidence.Outcome = OutcomeTimeout
		}
		evidence.Detail = c.sanitize(ctxErr.Error())
		return attemptResult{evidence: evidence, diagnostics: stream.diagnostics, cause: ctxErr}
	}
	evidence.Outcome = stream.outcome
	evidence.ErrorCode = c.sanitize(stream.code)
	evidence.ErrorParam = c.sanitize(stream.param)
	evidence.Detail = c.sanitize(stream.message)
	evidence.Retryable = stream.retryable && (stream.outcome != OutcomeRetryableTransport || prepared.retry.RetryTransportErrors)
	return attemptResult{evidence: evidence, diagnostics: stream.diagnostics, retryable: evidence.Retryable}
}

type apiErrorEnvelope struct {
	Error responseError `json:"error"`
}

func readAPIError(reader io.Reader) (responseError, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, DefaultMaxErrorBytes+1))
	if err != nil {
		return responseError{}, err
	}
	if len(raw) > DefaultMaxErrorBytes {
		return responseError{}, errors.New("error response exceeded its byte limit")
	}
	var envelope apiErrorEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return responseError{}, fmt.Errorf("decode error response: %w", err)
	}
	return envelope.Error, nil
}

func retryableHTTP(status int, code, errorType string, policy RetryPolicy) bool {
	if !containsInt(policy.RetryableStatusCodes, status) {
		return false
	}
	if status != http.StatusTooManyRequests {
		return true
	}
	return !nonRetryableRateLimit(code) && !nonRetryableRateLimit(errorType)
}

func nonRetryableRateLimit(value string) bool {
	switch value {
	case "credit_balance_exhausted", "organization_spend_limit_exceeded", "project_spend_limit_exceeded", "organization_usage_limit_exceeded", "insufficient_quota", "billing_hard_limit_reached":
		return true
	default:
		return false
	}
}

func retryDelay(policy RetryPolicy, failedAttempt int, retryAfter time.Duration) time.Duration {
	delay := policy.BaseDelay
	for i := 1; i < failedAttempt && delay > 0; i++ {
		if policy.MaxDelay > 0 && delay > policy.MaxDelay/2 {
			delay = policy.MaxDelay
			break
		}
		delay *= 2
	}
	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}
	if retryAfter > delay {
		delay = retryAfter
	}
	return delay
}

func transportRequestID(requestID string, attempt int) string {
	if attempt <= 1 {
		return requestID
	}
	suffix := fmt.Sprintf("/retry-%d", attempt)
	if len(requestID)+len(suffix) > 512 {
		requestID = requestID[:512-len(suffix)]
	}
	return requestID + suffix
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTransportError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) invocationError(outcome Outcome, attempt int, evidence AttemptEvidence, message string, cause error) error {
	message = c.sanitize(message)
	if message == "" {
		message = "Responses API invocation failed"
	}
	return &InvocationError{
		Outcome: outcome, Attempt: attempt, StatusCode: evidence.StatusCode,
		Code: c.sanitize(evidence.ErrorCode), Message: message, Retryable: evidence.Retryable, cause: cause,
	}
}

func (c *Client) sanitize(value string) string {
	if c == nil {
		return value
	}
	if c.apiKey != "" {
		value = strings.ReplaceAll(value, c.apiKey, redact.Replacement)
	}
	if c.redactor != nil {
		value = c.redactor.String(value)
	}
	return value
}

func (c *Client) containsSecret(raw []byte) bool {
	value := string(raw)
	if c.apiKey != "" && strings.Contains(value, c.apiKey) {
		return true
	}
	if c.redactor != nil {
		_, facts := c.redactor.Redact(value)
		return facts.MatchCount != 0
	}
	return false
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return nil, errors.New("model client credentials and transport state are not serializable")
}

func (c *Client) String() string {
	if c == nil {
		return "model.Client<nil>"
	}
	return "model.Client{endpoint:" + strconv.Quote(c.sanitize(c.endpoint)) + ", credential:[REDACTED]}"
}
