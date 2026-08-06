package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

const (
	healthPath     = "/health"
	metadataPath   = "/metadata"
	embeddingsPath = "/embeddings"
)

type Client struct {
	config Config
	space  EmbeddingSpaceIdentity
}

var _ Embedder = (*Client)(nil)

func NewClient(config Config) (*Client, error) {
	normalized, err := config.normalize()
	if err != nil {
		return nil, err
	}
	space, err := normalized.ExpectedModel.SpaceIdentity()
	if err != nil {
		return nil, err
	}
	return &Client{config: normalized, space: space}, nil
}

func (c *Client) Config() Config {
	if c == nil {
		return Config{}
	}
	return c.config
}

func (c *Client) Health(ctx context.Context) (ServiceStatus, error) {
	if c == nil {
		err := newAdapterError(ErrorMalformedResponse, "health", 0, "client is nil", nil)
		return err.Status(), err
	}
	operationContext, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	if err := c.checkHealth(operationContext); err != nil {
		return errStatus(err), err
	}
	if _, err := c.fetchModelInfo(operationContext); err != nil {
		return errStatus(err), err
	}
	return readyStatus(c.config.ExpectedModel, c.space.SHA256), nil
}

func (c *Client) ModelInfo(ctx context.Context) (EmbeddingModelInfo, error) {
	status, err := c.Health(ctx)
	if err != nil {
		return EmbeddingModelInfo{}, err
	}
	return *status.Model, nil
}

// Metadata preserves the Section 14.4 naming while ModelInfo implements the
// canonical service boundary from Section 40.12.
func (c *Client) Metadata(ctx context.Context) (EmbeddingModelInfo, error) {
	return c.ModelInfo(ctx)
}

func (c *Client) EmbedDocuments(ctx context.Context, input []string) (EmbeddingBatch, error) {
	values, status, err := c.embed(ctx, "documents", input)
	if err != nil {
		return EmbeddingBatch{}, err
	}
	return EmbeddingBatch{Status: status, Space: c.space, Values: values}, nil
}

func (c *Client) EmbedQuery(ctx context.Context, input string) (Embedding, error) {
	values, status, err := c.embed(ctx, "query", []string{input})
	if err != nil {
		return Embedding{}, err
	}
	return Embedding{Status: status, Space: c.space, Value: values[0]}, nil
}

type healthResponse struct {
	Status string `json:"status"`
}

func (c *Client) checkHealth(ctx context.Context) error {
	var response healthResponse
	if err := c.doJSON(ctx, http.MethodGet, healthPath, nil, "health", &response); err != nil {
		return err
	}
	if response.Status != "ok" {
		return newAdapterError(ErrorUnhealthy, "health", 0, fmt.Sprintf("service status is %q", response.Status), nil)
	}
	return nil
}

func (c *Client) fetchModelInfo(ctx context.Context) (EmbeddingModelInfo, error) {
	var info EmbeddingModelInfo
	if err := c.doJSON(ctx, http.MethodGet, metadataPath, nil, "metadata", &info); err != nil {
		return EmbeddingModelInfo{}, err
	}
	if err := info.Validate(); err != nil {
		return EmbeddingModelInfo{}, newAdapterError(ErrorMalformedResponse, "metadata", 0, err.Error(), err)
	}
	if info != c.config.ExpectedModel {
		return EmbeddingModelInfo{}, newAdapterError(ErrorModelMetadataDrift, "metadata", 0, "service metadata does not match the pinned embedding space", nil)
	}
	return info, nil
}

type embeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	InputType      string   `json:"input_type"`
	EncodingFormat string   `json:"encoding_format"`
}

type embeddingResponse struct {
	Object    string             `json:"object"`
	Data      []embeddingData    `json:"data"`
	Model     string             `json:"model"`
	ModelInfo EmbeddingModelInfo `json:"model_info"`
	Usage     json.RawMessage    `json:"usage,omitempty"`
}

type embeddingData struct {
	Object    string            `json:"object"`
	Index     int               `json:"index"`
	Embedding []json.RawMessage `json:"embedding"`
}

func (c *Client) embed(ctx context.Context, inputType string, input []string) ([][]float32, ServiceStatus, error) {
	if c == nil {
		err := newAdapterError(ErrorMalformedResponse, inputType, 0, "client is nil", nil)
		return nil, err.Status(), err
	}
	if err := c.validateInput(inputType, input); err != nil {
		return nil, errStatus(err), err
	}
	operationContext, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	if err := c.checkHealth(operationContext); err != nil {
		return nil, errStatus(err), err
	}
	if _, err := c.fetchModelInfo(operationContext); err != nil {
		return nil, errStatus(err), err
	}

	request := embeddingRequest{
		Model:          c.config.ExpectedModel.ModelName,
		Input:          append([]string(nil), input...),
		InputType:      inputType,
		EncodingFormat: "float",
	}
	var response embeddingResponse
	if err := c.doJSON(operationContext, http.MethodPost, embeddingsPath, request, inputType, &response); err != nil {
		return nil, errStatus(err), err
	}
	if response.Object != "list" || response.Model == "" {
		err := newAdapterError(ErrorMalformedResponse, inputType, 0, "response is not an embedding list with a model", nil)
		return nil, err.Status(), err
	}
	if err := response.ModelInfo.Validate(); err != nil {
		adapterErr := newAdapterError(ErrorMalformedResponse, inputType, 0, "response model_info is invalid", err)
		return nil, adapterErr.Status(), adapterErr
	}
	if response.Model != c.config.ExpectedModel.ModelName || response.ModelInfo != c.config.ExpectedModel {
		err := newAdapterError(ErrorModelMetadataDrift, inputType, 0, "embedding response does not match the pinned embedding space", nil)
		return nil, err.Status(), err
	}
	values, err := c.validateVectors(inputType, len(input), response.Data)
	if err != nil {
		return nil, errStatus(err), err
	}
	// Metadata is checked again after vector production. Callers receive no
	// vectors unless the service retained the exact space for the full request.
	if _, err := c.fetchModelInfo(operationContext); err != nil {
		return nil, errStatus(err), err
	}
	return values, readyStatus(c.config.ExpectedModel, c.space.SHA256), nil
}

func (c *Client) validateInput(operation string, input []string) error {
	if len(input) == 0 {
		return newAdapterError(ErrorInvalidInput, operation, 0, "at least one input is required", nil)
	}
	if len(input) > c.config.MaxBatchInputs {
		return newAdapterError(ErrorInvalidInput, operation, 0, fmt.Sprintf("input count %d exceeds limit %d", len(input), c.config.MaxBatchInputs), nil)
	}
	total := 0
	for i, value := range input {
		if value == "" {
			return newAdapterError(ErrorInvalidInput, operation, 0, fmt.Sprintf("input %d is empty", i), nil)
		}
		if len(value) > c.config.MaxInputBytes {
			return newAdapterError(ErrorInvalidInput, operation, 0, fmt.Sprintf("input %d has %d bytes and exceeds limit %d", i, len(value), c.config.MaxInputBytes), nil)
		}
		total += len(value)
		if total > c.config.MaxBatchBytes {
			return newAdapterError(ErrorInvalidInput, operation, 0, fmt.Sprintf("input batch has more than %d bytes", c.config.MaxBatchBytes), nil)
		}
	}
	return nil
}

func (c *Client) validateVectors(operation string, count int, data []embeddingData) ([][]float32, error) {
	if len(data) != count {
		return nil, newAdapterError(ErrorWrongCount, operation, 0, fmt.Sprintf("received %d vectors for %d inputs", len(data), count), nil)
	}
	values := make([][]float32, count)
	seen := make([]bool, count)
	for _, item := range data {
		if item.Object != "embedding" || item.Index < 0 || item.Index >= count || seen[item.Index] {
			return nil, newAdapterError(ErrorWrongCount, operation, 0, "embedding indexes are not an exact unique input sequence", nil)
		}
		seen[item.Index] = true
		if len(item.Embedding) != c.config.ExpectedModel.Dimensions {
			return nil, newAdapterError(ErrorWrongDimension, operation, 0, fmt.Sprintf("vector %d has dimension %d, want %d", item.Index, len(item.Embedding), c.config.ExpectedModel.Dimensions), nil)
		}
		vector := make([]float32, len(item.Embedding))
		for dimension, raw := range item.Embedding {
			value, kind, err := parseVectorValue(raw)
			if err != nil {
				return nil, newAdapterError(kind, operation, 0, fmt.Sprintf("vector %d dimension %d: %v", item.Index, dimension, err), err)
			}
			vector[dimension] = value
		}
		values[item.Index] = vector
	}
	return values, nil
}

func parseVectorValue(raw json.RawMessage) (float32, ErrorKind, error) {
	text := string(bytes.TrimSpace(raw))
	if strings.HasPrefix(text, `"`) {
		var label string
		if err := json.Unmarshal(raw, &label); err != nil {
			return 0, ErrorMalformedResponse, errors.New("value is not a JSON number")
		}
		parsed, err := strconv.ParseFloat(label, 64)
		if err == nil && (math.IsNaN(parsed) || math.IsInf(parsed, 0)) {
			return 0, ErrorNonFiniteVector, errors.New("value is non-finite")
		}
		return 0, ErrorMalformedResponse, errors.New("value is not a JSON number")
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		if math.IsInf(parsed, 0) {
			return 0, ErrorNonFiniteVector, errors.New("value is outside the finite range")
		}
		return 0, ErrorMalformedResponse, errors.New("value is not a JSON number")
	}
	converted := float32(parsed)
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.IsNaN(float64(converted)) || math.IsInf(float64(converted), 0) {
		return 0, ErrorNonFiniteVector, errors.New("value is outside the finite float32 range")
	}
	return converted, "", nil
}

func (c *Client) doJSON(ctx context.Context, method, suffix string, requestBody any, operation string, target any) error {
	var body io.Reader
	if requestBody != nil {
		raw, err := json.Marshal(requestBody)
		if err != nil {
			return newAdapterError(ErrorInvalidInput, operation, 0, "request could not be encoded", err)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.config.Endpoint+suffix, body)
	if err != nil {
		return newAdapterError(ErrorInvalidInput, operation, 0, "request could not be constructed", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.config.HTTPClient.Do(request)
	if err != nil {
		return classifyTransportError(ctx, operation, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		kind := ErrorUnavailable
		if operation == "health" {
			kind = ErrorUnhealthy
		}
		return newAdapterError(kind, operation, response.StatusCode, fmt.Sprintf("service returned HTTP %d", response.StatusCode), nil)
	}
	limited := io.LimitReader(response.Body, c.config.MaxResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return classifyTransportError(ctx, operation, err)
	}
	if int64(len(raw)) > c.config.MaxResponseBytes {
		return newAdapterError(ErrorMalformedResponse, operation, response.StatusCode, fmt.Sprintf("response exceeds %d bytes", c.config.MaxResponseBytes), nil)
	}
	if err := validateJSONShape(raw); err != nil {
		return newAdapterError(ErrorMalformedResponse, operation, response.StatusCode, "response contains ambiguous JSON", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return newAdapterError(ErrorMalformedResponse, operation, response.StatusCode, "response is not valid contract JSON", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return newAdapterError(ErrorMalformedResponse, operation, response.StatusCode, "response contains trailing JSON", nil)
	}
	return nil
}

func validateJSONShape(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple top-level values")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("array is not closed")
		}
	default:
		return errors.New("unexpected closing delimiter")
	}
	return nil
}

func classifyTransportError(ctx context.Context, operation string, err error) error {
	contextErr := ctx.Err()
	if errors.Is(contextErr, context.Canceled) || errors.Is(err, context.Canceled) {
		return newAdapterError(ErrorCancelled, operation, 0, "request was cancelled", context.Canceled)
	}
	if errors.Is(contextErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return newAdapterError(ErrorTimeout, operation, 0, "request exceeded its deadline", context.DeadlineExceeded)
	}
	return newAdapterError(ErrorUnavailable, operation, 0, "local service is unavailable", err)
}

func newAdapterError(kind ErrorKind, operation string, statusCode int, detail string, cause error) *AdapterError {
	mode := ServiceDegraded
	if kind == ErrorInvalidInput || kind == ErrorCancelled {
		mode = ServiceFailed
	}
	return &AdapterError{Kind: kind, Operation: operation, StatusCode: statusCode, Detail: detail, Mode: mode, cause: cause}
}

func errStatus(err error) ServiceStatus {
	var adapterErr *AdapterError
	if errors.As(err, &adapterErr) {
		return adapterErr.Status()
	}
	return ServiceStatus{Mode: ServiceFailed, Kind: ErrorMalformedResponse, Detail: err.Error()}
}
