// Package model provides the trusted control-plane OpenAI Responses API
// boundary. It does not expose credentials to model workers or retain hidden
// conversation state.
package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	DefaultEndpoint           = "https://api.openai.com/v1/responses"
	DefaultMaxStreamBytes     = 1 << 20
	DefaultMaxDiagnosticBytes = 64 << 10
	DefaultMaxErrorBytes      = 64 << 10
	MaxStreamBytes            = 64 << 20
	MaxDiagnosticBytes        = 1 << 20
)

type Outcome string

const (
	OutcomeSuccess             Outcome = "success"
	OutcomeSafetyRefusal       Outcome = "safety_refusal"
	OutcomeMalformedJSON       Outcome = "malformed_json"
	OutcomeSchemaInvalid       Outcome = "schema_invalid"
	OutcomeStaleIdentity       Outcome = "stale_identity"
	OutcomeOversizedStream     Outcome = "oversized_stream"
	OutcomeTimeout             Outcome = "timeout"
	OutcomeCancelled           Outcome = "cancelled"
	OutcomeRetryableTransport  Outcome = "retryable_transport_failure"
	OutcomeRetryableService    Outcome = "retryable_service_failure"
	OutcomeNonRetryableFailure Outcome = "nonretryable_failure"
	OutcomeRetriesExhausted    Outcome = "retries_exhausted"
)

type RetryPolicy struct {
	MaxAttempts             int           `json:"max_attempts"`
	BaseDelay               time.Duration `json:"base_delay_ns"`
	MaxDelay                time.Duration `json:"max_delay_ns"`
	RetryTransportErrors    bool          `json:"retry_transport_errors"`
	RetryableStatusCodes    []int         `json:"retryable_status_codes"`
	RetryableStreamErrCodes []string      `json:"retryable_stream_error_codes"`
}

func (p RetryPolicy) normalize() (RetryPolicy, error) {
	if p.MaxAttempts <= 0 {
		return RetryPolicy{}, errors.New("model retry policy: max_attempts must be positive")
	}
	if p.BaseDelay < 0 || p.MaxDelay < 0 {
		return RetryPolicy{}, errors.New("model retry policy: delays must be nonnegative")
	}
	if p.MaxDelay > 0 && p.BaseDelay > p.MaxDelay {
		return RetryPolicy{}, errors.New("model retry policy: base delay exceeds max delay")
	}
	statusSeen := make(map[int]struct{}, len(p.RetryableStatusCodes))
	statuses := append([]int(nil), p.RetryableStatusCodes...)
	sort.Ints(statuses)
	for _, status := range statuses {
		if !admittedTransientStatus(status) {
			return RetryPolicy{}, fmt.Errorf("model retry policy: status %d is not an admitted transient Responses API failure", status)
		}
		if _, ok := statusSeen[status]; ok {
			return RetryPolicy{}, fmt.Errorf("model retry policy: duplicate retryable status %d", status)
		}
		statusSeen[status] = struct{}{}
	}
	codeSeen := make(map[string]struct{}, len(p.RetryableStreamErrCodes))
	codes := append([]string(nil), p.RetryableStreamErrCodes...)
	sort.Strings(codes)
	for _, code := range codes {
		if err := normalizedToken("retryable stream error code", code); err != nil {
			return RetryPolicy{}, fmt.Errorf("model retry policy: %w", err)
		}
		if _, ok := codeSeen[code]; ok {
			return RetryPolicy{}, fmt.Errorf("model retry policy: duplicate retryable stream error code %q", code)
		}
		codeSeen[code] = struct{}{}
	}
	p.RetryableStatusCodes = statuses
	p.RetryableStreamErrCodes = codes
	return p, nil
}

func (p RetryPolicy) identity() (string, error) {
	normalized, err := p.normalize()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("model retry policy: marshal identity: %w", err)
	}
	return SHA256(raw), nil
}

type Request struct {
	RequestID string
	TaskID    string
	RunID     string

	SourceRevision  string
	Model           string
	ReasoningEffort string
	MaxOutputTokens int

	PromptVersion string
	PromptSHA256  string
	Prompt        string

	ResponseSchemaVersion string
	ResponseSchemaSHA256  string
	ResponseSchemaName    string
	ResponseSchema        json.RawMessage

	Timeout            time.Duration
	MaxStreamBytes     int64
	MaxDiagnosticBytes int
	Retry              RetryPolicy
}

type OutputIdentity struct {
	RequestID             string `json:"request_id"`
	TaskID                string `json:"task_id"`
	RunID                 string `json:"run_id"`
	SourceRevision        string `json:"source_revision"`
	PromptVersion         string `json:"prompt_version"`
	PromptSHA256          string `json:"prompt_sha256"`
	ResponseSchemaVersion string `json:"response_schema_version"`
	ResponseSchemaSHA256  string `json:"response_schema_sha256"`
}

type RequestEvidence struct {
	RequestID             string         `json:"request_id"`
	TaskID                string         `json:"task_id"`
	RunID                 string         `json:"run_id"`
	SourceRevision        string         `json:"source_revision"`
	Model                 string         `json:"model"`
	ReasoningEffort       string         `json:"reasoning_effort"`
	MaxOutputTokens       int            `json:"max_output_tokens"`
	PromptVersion         string         `json:"prompt_version"`
	PromptSHA256          string         `json:"prompt_sha256"`
	ResponseSchemaVersion string         `json:"response_schema_version"`
	ResponseSchemaSHA256  string         `json:"response_schema_sha256"`
	ResponseSchemaName    string         `json:"response_schema_name"`
	Timeout               time.Duration  `json:"timeout_ns"`
	MaxStreamBytes        int64          `json:"max_stream_bytes"`
	MaxDiagnosticBytes    int            `json:"max_diagnostic_bytes"`
	RetryPolicySHA256     string         `json:"retry_policy_sha256"`
	Retry                 RetryPolicy    `json:"retry_policy"`
	OutputIdentity        OutputIdentity `json:"output_identity"`
}

type PreparedRequest struct {
	evidence       RequestEvidence
	body           []byte
	validator      *jsonschema.Schema
	metadata       map[string]string
	outputIdentity OutputIdentity
	timeout        time.Duration
	maxStream      int64
	maxDiagnostic  int
	retry          RetryPolicy
}

type createRequest struct {
	Model           string            `json:"model"`
	Input           string            `json:"input"`
	Reasoning       reasoningRequest  `json:"reasoning"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	Text            textRequest       `json:"text"`
	Metadata        map[string]string `json:"metadata"`
	Store           bool              `json:"store"`
	Stream          bool              `json:"stream"`
}

type reasoningRequest struct {
	Effort string `json:"effort"`
}

type textRequest struct {
	Format formatRequest `json:"format"`
}

type formatRequest struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

func Prepare(req Request) (PreparedRequest, error) {
	if err := validateRequest(req); err != nil {
		return PreparedRequest{}, err
	}
	retry, err := req.Retry.normalize()
	if err != nil {
		return PreparedRequest{}, err
	}
	retrySHA256, err := retry.identity()
	if err != nil {
		return PreparedRequest{}, err
	}
	schemaRaw, schemaValue, err := normalizeSchema(req.ResponseSchema)
	if err != nil {
		return PreparedRequest{}, err
	}
	if got := SHA256(schemaRaw); got != req.ResponseSchemaSHA256 {
		return PreparedRequest{}, fmt.Errorf("prepare model request: response schema SHA-256 %q does not match canonical schema %q", req.ResponseSchemaSHA256, got)
	}
	validator, err := compileSchema(schemaValue)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("prepare model request: compile response schema: %w", err)
	}

	maxStream := req.MaxStreamBytes
	if maxStream == 0 {
		maxStream = DefaultMaxStreamBytes
	}
	maxDiagnostic := req.MaxDiagnosticBytes
	if maxDiagnostic == 0 {
		maxDiagnostic = DefaultMaxDiagnosticBytes
	}
	identity := OutputIdentity{
		RequestID:             req.RequestID,
		TaskID:                req.TaskID,
		RunID:                 req.RunID,
		SourceRevision:        req.SourceRevision,
		PromptVersion:         req.PromptVersion,
		PromptSHA256:          req.PromptSHA256,
		ResponseSchemaVersion: req.ResponseSchemaVersion,
		ResponseSchemaSHA256:  req.ResponseSchemaSHA256,
	}
	metadata := map[string]string{
		"revolvr_request_id":          req.RequestID,
		"revolvr_task_id":             req.TaskID,
		"revolvr_run_id":              req.RunID,
		"revolvr_source_revision":     req.SourceRevision,
		"revolvr_prompt_version":      req.PromptVersion,
		"revolvr_prompt_sha256":       req.PromptSHA256,
		"revolvr_schema_version":      req.ResponseSchemaVersion,
		"revolvr_schema_sha256":       req.ResponseSchemaSHA256,
		"revolvr_retry_policy_sha256": retrySHA256,
	}
	for key, value := range metadata {
		if len(key) > 64 || len(value) > 512 {
			return PreparedRequest{}, fmt.Errorf("prepare model request: metadata %q exceeds the Responses API limit", key)
		}
	}
	body, err := json.Marshal(createRequest{
		Model:           req.Model,
		Input:           req.Prompt,
		Reasoning:       reasoningRequest{Effort: req.ReasoningEffort},
		MaxOutputTokens: req.MaxOutputTokens,
		Text: textRequest{Format: formatRequest{
			Type: "json_schema", Name: req.ResponseSchemaName, Schema: schemaRaw, Strict: true,
		}},
		Metadata: metadata,
		Store:    false,
		Stream:   true,
	})
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("prepare model request: marshal Responses API body: %w", err)
	}
	evidence := RequestEvidence{
		RequestID:             req.RequestID,
		TaskID:                req.TaskID,
		RunID:                 req.RunID,
		SourceRevision:        req.SourceRevision,
		Model:                 req.Model,
		ReasoningEffort:       req.ReasoningEffort,
		MaxOutputTokens:       req.MaxOutputTokens,
		PromptVersion:         req.PromptVersion,
		PromptSHA256:          req.PromptSHA256,
		ResponseSchemaVersion: req.ResponseSchemaVersion,
		ResponseSchemaSHA256:  req.ResponseSchemaSHA256,
		ResponseSchemaName:    req.ResponseSchemaName,
		Timeout:               req.Timeout,
		MaxStreamBytes:        maxStream,
		MaxDiagnosticBytes:    maxDiagnostic,
		RetryPolicySHA256:     retrySHA256,
		Retry:                 retry,
		OutputIdentity:        identity,
	}
	return PreparedRequest{
		evidence:       cloneRequestEvidence(evidence),
		body:           append([]byte(nil), body...),
		validator:      validator,
		metadata:       cloneStringMap(metadata),
		outputIdentity: identity,
		timeout:        req.Timeout,
		maxStream:      maxStream,
		maxDiagnostic:  maxDiagnostic,
		retry:          retry,
	}, nil
}

func validateRequest(req Request) error {
	fields := []struct{ label, value string }{
		{"request ID", req.RequestID}, {"task ID", req.TaskID}, {"run ID", req.RunID},
		{"source revision", req.SourceRevision}, {"model", req.Model},
		{"reasoning effort", req.ReasoningEffort}, {"prompt version", req.PromptVersion},
		{"response schema version", req.ResponseSchemaVersion}, {"response schema name", req.ResponseSchemaName},
	}
	for _, field := range fields {
		if err := normalizedToken(field.label, field.value); err != nil {
			return fmt.Errorf("prepare model request: %w", err)
		}
	}
	if len(req.RequestID) > 512 || !isASCII(req.RequestID) {
		return errors.New("prepare model request: request ID must be ASCII and at most 512 bytes")
	}
	if len(req.ResponseSchemaName) > 64 || !validSchemaName(req.ResponseSchemaName) {
		return errors.New("prepare model request: response schema name must contain at most 64 letters, digits, underscores, or hyphens")
	}
	if req.MaxOutputTokens <= 0 {
		return errors.New("prepare model request: max output tokens must be positive")
	}
	if req.Timeout <= 0 {
		return errors.New("prepare model request: timeout must be positive")
	}
	if req.MaxStreamBytes < 0 || req.MaxStreamBytes > MaxStreamBytes || req.MaxDiagnosticBytes < 0 || req.MaxDiagnosticBytes > MaxDiagnosticBytes {
		return errors.New("prepare model request: stream or diagnostic limit is outside its bounded range")
	}
	if req.Prompt == "" {
		return errors.New("prepare model request: prompt is required")
	}
	if !validSHA256(req.PromptSHA256) || req.PromptSHA256 != SHA256([]byte(req.Prompt)) {
		return errors.New("prepare model request: prompt SHA-256 does not match exact prompt bytes")
	}
	if !validSHA256(req.ResponseSchemaSHA256) {
		return errors.New("prepare model request: response schema SHA-256 must be 64 lowercase hexadecimal characters")
	}
	if len(req.ResponseSchema) == 0 {
		return errors.New("prepare model request: response schema is required")
	}
	return nil
}

func normalizeSchema(raw []byte) ([]byte, any, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, nil, fmt.Errorf("prepare model request: response schema is malformed JSON: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(compact.Bytes()))
	if err != nil {
		return nil, nil, fmt.Errorf("prepare model request: decode response schema: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, nil, errors.New("prepare model request: response schema root must be an object")
	}
	if object["type"] != "object" || object["additionalProperties"] != false {
		return nil, nil, errors.New("prepare model request: strict response schema root must be a closed object")
	}
	if err := validateStrictObjects(value, "$"); err != nil {
		return nil, nil, fmt.Errorf("prepare model request: %w", err)
	}
	return append([]byte(nil), compact.Bytes()...), value, nil
}

func validateStrictObjects(value any, path string) error {
	switch node := value.(type) {
	case map[string]any:
		if node["type"] == "object" {
			properties, ok := node["properties"].(map[string]any)
			if !ok || node["additionalProperties"] != false {
				return fmt.Errorf("strict response schema object %s must declare properties and additionalProperties false", path)
			}
			requiredValues, ok := node["required"].([]any)
			if !ok || len(requiredValues) != len(properties) {
				return fmt.Errorf("strict response schema object %s must require every declared property exactly once", path)
			}
			required := make(map[string]struct{}, len(requiredValues))
			for _, value := range requiredValues {
				name, ok := value.(string)
				if !ok {
					return fmt.Errorf("strict response schema object %s has a non-string required property", path)
				}
				if _, duplicate := required[name]; duplicate {
					return fmt.Errorf("strict response schema object %s requires property %q more than once", path, name)
				}
				required[name] = struct{}{}
			}
			for name := range properties {
				if _, ok := required[name]; !ok {
					return fmt.Errorf("strict response schema object %s does not require property %q", path, name)
				}
			}
		}
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validateStrictObjects(node[key], path+"/"+key); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range node {
			if err := validateStrictObjects(item, fmt.Sprintf("%s/%d", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

type rejectingLoader struct{}

func (rejectingLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external JSON Schema resource %q is not admitted", url)
}

func compileSchema(value any) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(rejectingLoader{})
	if err := compiler.AddResource("urn:revolvr:response-schema", value); err != nil {
		return nil, err
	}
	return compiler.Compile("urn:revolvr:response-schema")
}

func normalizedToken(label, value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be nonblank and normalized", label)
	}
	if strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return fmt.Errorf("%s contains whitespace or a control character", label)
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func isASCII(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func validSchemaName(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func SHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneRetryPolicy(policy RetryPolicy) RetryPolicy {
	policy.RetryableStatusCodes = append([]int(nil), policy.RetryableStatusCodes...)
	policy.RetryableStreamErrCodes = append([]string(nil), policy.RetryableStreamErrCodes...)
	return policy
}

func cloneRequestEvidence(evidence RequestEvidence) RequestEvidence {
	evidence.Retry = cloneRetryPolicy(evidence.Retry)
	return evidence
}

func containsInt(values []int, value int) bool {
	i := sort.SearchInts(values, value)
	return i < len(values) && values[i] == value
}

func containsString(values []string, value string) bool {
	i := sort.SearchStrings(values, value)
	return i < len(values) && values[i] == value
}

func admittedTransientStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryAfter(response *http.Response) time.Duration {
	value := strings.TrimSpace(response.Header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
		return 0
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
