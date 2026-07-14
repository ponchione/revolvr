package receipt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"revolvr/internal/jsonl"
)

var (
	ErrMalformedCodexJSONL = errors.New("receipt: malformed Codex JSONL")
	ErrCodexUsageOverflow  = errors.New("receipt: Codex usage metrics overflow")
	ErrCodexUsageAmbiguity = errors.New("receipt: ambiguous Codex usage metrics")
	ErrCodexJSONLSource    = errors.New("receipt: Codex JSONL source failure")
)

type MalformedCodexJSONLError struct {
	FirstRecord int
	Count       int
	Cause       error
}

func (e *MalformedCodexJSONLError) Error() string {
	return fmt.Sprintf("receipt: parse Codex JSONL record %d (%d malformed record(s)): %v", e.FirstRecord, e.Count, e.Cause)
}

func (e *MalformedCodexJSONLError) Unwrap() []error {
	return []error{ErrMalformedCodexJSONL, e.Cause}
}

type CodexUsageOverflowError struct {
	Record int
	Field  string
}

func (e *CodexUsageOverflowError) Error() string {
	return fmt.Sprintf("receipt: Codex usage metrics overflow at record %d field %s", e.Record, e.Field)
}

func (e *CodexUsageOverflowError) Unwrap() error {
	return ErrCodexUsageOverflow
}

type CodexUsageAmbiguityError struct {
	Record         int
	CandidatePaths []string
}

func (e *CodexUsageAmbiguityError) Error() string {
	return fmt.Sprintf("receipt: ambiguous Codex usage metrics at record %d: candidates %s", e.Record, strings.Join(e.CandidatePaths, ", "))
}

func (e *CodexUsageAmbiguityError) Unwrap() error {
	return ErrCodexUsageAmbiguity
}

type CodexJSONLSourceError struct {
	Operation string
	Path      string
	Cause     error
}

func (e *CodexJSONLSourceError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("receipt: %s Codex JSONL metrics source: %v", e.Operation, e.Cause)
	}
	return fmt.Sprintf("receipt: %s Codex JSONL metrics source %s: %v", e.Operation, e.Path, e.Cause)
}

func (e *CodexJSONLSourceError) Unwrap() []error {
	return []error{ErrCodexJSONLSource, e.Cause}
}

func RewriteMetricsFromCodexJSONL(content []byte, jsonl []byte) ([]byte, Receipt, bool, error) {
	return RewriteMetricsFromCodexJSONLReader(context.Background(), content, bytes.NewReader(jsonl))
}

func RewriteMetricsFromCodexJSONLReader(ctx context.Context, content []byte, reader io.Reader) ([]byte, Receipt, bool, error) {
	metrics, found, err := ParseCodexUsageMetricsReader(ctx, reader)
	return rewriteMetricsFromCodexUsage(content, metrics, found, err)
}

func RewriteMetricsFromCodexJSONLFile(ctx context.Context, content []byte, path string) ([]byte, Receipt, bool, error) {
	metrics, found, err := ParseCodexUsageMetricsFile(ctx, path)
	return rewriteMetricsFromCodexUsage(content, metrics, found, err)
}

func rewriteMetricsFromCodexUsage(content []byte, metrics Metrics, found bool, metricsErr error) ([]byte, Receipt, bool, error) {
	if metricsErr != nil {
		parsed, parseErr := Parse(content)
		if parseErr != nil {
			return nil, Receipt{}, false, errors.Join(metricsErr, parseErr)
		}
		return append([]byte(nil), content...), parsed, false, metricsErr
	}
	if !found {
		parsed, parseErr := Parse(content)
		if parseErr != nil {
			return nil, Receipt{}, false, parseErr
		}
		return append([]byte(nil), content...), parsed, false, nil
	}
	return RewriteMetrics(content, metrics)
}

func ParseCodexUsageMetrics(jsonl []byte) (Metrics, bool, error) {
	return ParseCodexUsageMetricsReader(context.Background(), bytes.NewReader(jsonl))
}

func ParseCodexUsageMetricsFile(ctx context.Context, path string) (Metrics, bool, error) {
	if ctx == nil {
		return Metrics{}, false, errors.New("receipt: parse Codex usage metrics: context is required")
	}
	if err := ctx.Err(); err != nil {
		return Metrics{}, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Metrics{}, false, &CodexJSONLSourceError{Operation: "open", Path: path, Cause: err}
	}
	metrics, found, parseErr := parseCodexUsageMetricsReadCloser(ctx, file)
	if parseErr == nil || isCodexMetricsDiagnostic(parseErr) {
		return metrics, found, parseErr
	}
	return Metrics{}, false, &CodexJSONLSourceError{Operation: "read", Path: path, Cause: parseErr}
}

func RewriteMetrics(content []byte, metrics Metrics) ([]byte, Receipt, bool, error) {
	if metrics.InputTokens < 0 {
		return nil, Receipt{}, false, fmt.Errorf("%w: metrics.input_tokens (must be >= 0, got %d)", ErrInvalidField, metrics.InputTokens)
	}
	if metrics.OutputTokens < 0 {
		return nil, Receipt{}, false, fmt.Errorf("%w: metrics.output_tokens (must be >= 0, got %d)", ErrInvalidField, metrics.OutputTokens)
	}
	if metrics.DurationSeconds < 0 {
		return nil, Receipt{}, false, fmt.Errorf("%w: metrics.duration_seconds (must be >= 0, got %d)", ErrInvalidField, metrics.DurationSeconds)
	}

	frontmatter, body, ok := splitFrontmatter(content)
	if !ok {
		return nil, Receipt{}, false, ErrMissingFrontmatter
	}
	parsed, err := Parse(content)
	if err != nil {
		return nil, Receipt{}, false, err
	}
	if parsed.Metrics == metrics {
		return append([]byte(nil), content...), parsed, false, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(frontmatter, &root); err != nil {
		return nil, Receipt{}, false, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	mapping, err := frontmatterMapping(&root)
	if err != nil {
		return nil, Receipt{}, false, err
	}
	metricsNode := yamlMappingValue(mapping, "metrics")
	if metricsNode == nil || metricsNode.Kind != yaml.MappingNode {
		return nil, Receipt{}, false, fmt.Errorf("%w: metrics", ErrMissingField)
	}
	setYAMLInt(metricsNode, "input_tokens", metrics.InputTokens)
	setYAMLInt(metricsNode, "output_tokens", metrics.OutputTokens)
	setYAMLInt(metricsNode, "duration_seconds", metrics.DurationSeconds)

	var updatedFrontmatter bytes.Buffer
	encoder := yaml.NewEncoder(&updatedFrontmatter)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		_ = encoder.Close()
		return nil, Receipt{}, false, fmt.Errorf("receipt: encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, Receipt{}, false, fmt.Errorf("receipt: encode yaml: %w", err)
	}

	var updated bytes.Buffer
	updated.WriteString("---\n")
	updated.Write(updatedFrontmatter.Bytes())
	updated.WriteString("---\n")
	updated.Write(body)
	reparsed, err := Parse(updated.Bytes())
	if err != nil {
		return nil, Receipt{}, false, err
	}
	return updated.Bytes(), reparsed, true, nil
}

func ParseCodexUsageMetricsReader(ctx context.Context, reader io.Reader) (Metrics, bool, error) {
	total := Metrics{}
	found := false
	malformedCount := 0
	firstMalformedRecord := 0
	var firstMalformedCause error
	err := jsonl.ReadRecords(ctx, reader, func(recordNumber int, record []byte) error {
		record = bytes.TrimSpace(record)
		if len(record) == 0 {
			return nil
		}
		var event map[string]any
		decoder := json.NewDecoder(bytes.NewReader(record))
		decoder.UseNumber()
		if err := decoder.Decode(&event); err != nil {
			malformedCount++
			if firstMalformedCause == nil {
				firstMalformedRecord = recordNumber
				firstMalformedCause = err
			}
			return nil
		}
		if err := requireJSONDecoderEOF(decoder); err != nil {
			malformedCount++
			if firstMalformedCause == nil {
				firstMalformedRecord = recordNumber
				firstMalformedCause = err
			}
			return nil
		}
		usage, ok, err := usageMap(event)
		if err != nil {
			return codexUsageAmbiguity(recordNumber, err)
		}
		if !ok {
			return nil
		}
		metrics, metricsFound, err := metricsFromMap(usage)
		if err != nil {
			return codexUsageOverflow(recordNumber, err)
		}
		if !metricsFound {
			return nil
		}
		if metrics.DurationSeconds == 0 {
			duration, ok, err := durationSeconds(event)
			if err != nil {
				return codexUsageOverflow(recordNumber, err)
			}
			if ok {
				metrics.DurationSeconds = duration
			}
		}
		if total.InputTokens, ok = addMetricValue(total.InputTokens, metrics.InputTokens); !ok {
			return &CodexUsageOverflowError{Record: recordNumber, Field: "input_tokens"}
		}
		if total.OutputTokens, ok = addMetricValue(total.OutputTokens, metrics.OutputTokens); !ok {
			return &CodexUsageOverflowError{Record: recordNumber, Field: "output_tokens"}
		}
		if total.DurationSeconds, ok = addMetricValue(total.DurationSeconds, metrics.DurationSeconds); !ok {
			return &CodexUsageOverflowError{Record: recordNumber, Field: "duration_seconds"}
		}
		found = true
		return nil
	})
	if err != nil {
		return Metrics{}, false, fmt.Errorf("receipt: parse Codex JSONL metrics: %w", err)
	}
	if malformedCount > 0 {
		return total, found, &MalformedCodexJSONLError{FirstRecord: firstMalformedRecord, Count: malformedCount, Cause: firstMalformedCause}
	}
	return total, found, nil
}

func parseCodexUsageMetricsReadCloser(ctx context.Context, reader io.ReadCloser) (Metrics, bool, error) {
	var closeOnce sync.Once
	var closeErr error
	closeReader := func() {
		closeOnce.Do(func() {
			closeErr = reader.Close()
		})
	}

	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	if ctx.Done() == nil {
		close(watcherDone)
	} else {
		go func() {
			defer close(watcherDone)
			select {
			case <-ctx.Done():
				closeReader()
			case <-stopWatcher:
			}
		}()
	}

	metrics, found, parseErr := ParseCodexUsageMetricsReader(ctx, reader)
	close(stopWatcher)
	closeReader()
	<-watcherDone
	if parseErr != nil {
		return metrics, found, parseErr
	}
	if closeErr != nil {
		return Metrics{}, false, closeErr
	}
	return metrics, found, nil
}

func isCodexMetricsDiagnostic(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, ErrMalformedCodexJSONL) ||
		errors.Is(err, ErrCodexUsageOverflow) ||
		errors.Is(err, ErrCodexUsageAmbiguity) ||
		errors.Is(err, jsonl.ErrRecordTooLarge)
}

func requireJSONDecoderEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values in one record")
	}
	return err
}

func setYAMLInt(mapping *yaml.Node, key string, value int) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1].Kind = yaml.ScalarNode
			mapping.Content[i+1].Tag = "!!int"
			mapping.Content[i+1].Value = strconv.Itoa(value)
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)},
	)
}

func usageMap(event map[string]any) (map[string]any, bool, error) {
	// Supported schemas are ordered from most specific to least specific:
	// direct usage objects, usage objects in known event envelopes, and bare
	// metric fields. The order of both key lists is part of the precedence
	// contract.
	for _, key := range usageKeys {
		if usage, ok := asMap(event[key]); ok {
			return usage, true, nil
		}
	}
	for _, key := range usageParentKeys {
		if parent, ok := asMap(event[key]); ok {
			for _, usageKey := range usageKeys {
				if usage, ok := asMap(parent[usageKey]); ok {
					return usage, true, nil
				}
			}
		}
	}
	if hasUsageFields(event) {
		return event, true, nil
	}

	// Preserve the legacy nested shape only when it has one unambiguous usage
	// authority. Traversal order is used solely to produce stable diagnostics;
	// it never chooses among multiple candidates.
	candidates := nestedUsageCandidates(event)
	switch len(candidates) {
	case 0:
		return nil, false, nil
	case 1:
		return candidates[0].values, true, nil
	default:
		paths := make([]string, len(candidates))
		for i, candidate := range candidates {
			paths[i] = candidate.path
		}
		return nil, false, &usageAmbiguityError{candidates: paths}
	}
}

type usageCandidate struct {
	path   string
	values map[string]any
}

type usageAmbiguityError struct {
	candidates []string
}

func (e *usageAmbiguityError) Error() string {
	return "ambiguous nested usage candidates: " + strings.Join(e.candidates, ", ")
}

func nestedUsageCandidates(value any) []usageCandidate {
	var candidates []usageCandidate
	collectNestedUsageCandidates(value, "", &candidates)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].path < candidates[j].path
	})
	return candidates
}

func collectNestedUsageCandidates(value any, path string, candidates *[]usageCandidate) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := typed[key]
			childPath := jsonPointerChild(path, key)
			if isUsageKey(key) {
				if usage, ok := asMap(child); ok {
					_, found, err := metricsFromMap(usage)
					if found || err != nil {
						*candidates = append(*candidates, usageCandidate{path: childPath, values: usage})
					}
				}
			}
			collectNestedUsageCandidates(child, childPath, candidates)
		}
	case []any:
		for i, child := range typed {
			collectNestedUsageCandidates(child, path+"/"+strconv.Itoa(i), candidates)
		}
	}
}

func jsonPointerChild(path, key string) string {
	key = strings.ReplaceAll(key, "~", "~0")
	key = strings.ReplaceAll(key, "/", "~1")
	return path + "/" + key
}

var (
	usageKeys       = []string{"usage", "total_usage", "token_usage", "total_token_usage"}
	usageParentKeys = []string{"response", "result", "message", "event"}
)

func isUsageKey(key string) bool {
	for _, usageKey := range usageKeys {
		if key == usageKey {
			return true
		}
	}
	return false
}

func metricsFromMap(values map[string]any) (Metrics, bool, error) {
	input, inputOK, err := firstInt(values, "input_tokens", "prompt_tokens", "total_input_tokens")
	if err != nil {
		return Metrics{}, false, err
	}
	output, outputOK, err := firstInt(values, "output_tokens", "completion_tokens", "total_output_tokens")
	if err != nil {
		return Metrics{}, false, err
	}
	duration, durationOK, err := durationSeconds(values)
	if err != nil {
		return Metrics{}, false, err
	}
	return Metrics{
		InputTokens:     input,
		OutputTokens:    output,
		DurationSeconds: duration,
	}, inputOK || outputOK || durationOK, nil
}

func hasUsageFields(values map[string]any) bool {
	for _, key := range []string{
		"input_tokens", "prompt_tokens", "total_input_tokens",
		"output_tokens", "completion_tokens", "total_output_tokens",
		"duration_seconds", "duration_secs", "elapsed_seconds",
		"duration_ms", "elapsed_ms",
	} {
		if _, ok := values[key]; ok {
			return true
		}
	}
	return false
}

func firstInt(values map[string]any, keys ...string) (int, bool, error) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			parsed, found, err := numberAsInt(value)
			if err != nil {
				return 0, false, &usageValueOverflowError{field: key}
			}
			if found {
				return parsed, true, nil
			}
		}
	}
	return 0, false, nil
}

func durationSeconds(values map[string]any) (int, bool, error) {
	for _, key := range []string{"duration_seconds", "duration_secs", "elapsed_seconds"} {
		if value, ok := values[key]; ok {
			seconds, found, err := numberAsRoundedInt(value, 1)
			if err != nil {
				return 0, false, &usageValueOverflowError{field: key}
			}
			if found {
				return seconds, true, nil
			}
		}
	}
	for _, key := range []string{"duration_ms", "elapsed_ms"} {
		if value, ok := values[key]; ok {
			seconds, found, err := numberAsRoundedInt(value, 1000)
			if err != nil {
				return 0, false, &usageValueOverflowError{field: key}
			}
			if found {
				return seconds, true, nil
			}
		}
	}
	return 0, false, nil
}

type usageValueOverflowError struct {
	field string
}

func (e *usageValueOverflowError) Error() string {
	return "usage field " + e.field + " exceeds integer range"
}

func codexUsageOverflow(recordNumber int, err error) error {
	var valueErr *usageValueOverflowError
	if errors.As(err, &valueErr) {
		return &CodexUsageOverflowError{Record: recordNumber, Field: valueErr.field}
	}
	return &CodexUsageOverflowError{Record: recordNumber, Field: "unknown"}
}

func codexUsageAmbiguity(recordNumber int, err error) error {
	var ambiguityErr *usageAmbiguityError
	if errors.As(err, &ambiguityErr) {
		return &CodexUsageAmbiguityError{
			Record:         recordNumber,
			CandidatePaths: append([]string(nil), ambiguityErr.candidates...),
		}
	}
	return err
}

func numberAsInt(value any) (int, bool, error) {
	switch typed := value.(type) {
	case float64:
		return truncateFloatToInt(typed)
	case float32:
		return truncateFloatToInt(float64(typed))
	case int:
		return typed, true, nil
	case int64:
		return int64ToInt(typed)
	case json.Number:
		return parseIntCompatibleNumber(string(typed))
	case string:
		return parseIntCompatibleNumber(strings.TrimSpace(typed))
	default:
		return 0, false, nil
	}
}

func numberAsRoundedInt(value any, divisor float64) (int, bool, error) {
	if divisor == 1 {
		if text, ok := numericText(value); ok && integerSyntax(text) {
			return parseIntCompatibleNumber(text)
		}
	}
	number, found, err := numberAsFloat(value)
	if err != nil || !found {
		return 0, found, err
	}
	return roundedFloatToInt(number / divisor)
}

func numberAsFloat(value any) (float64, bool, error) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false, ErrCodexUsageOverflow
		}
		return typed, true, nil
	case float32:
		return float64(typed), true, nil
	case int:
		return float64(typed), true, nil
	case int64:
		return float64(typed), true, nil
	case json.Number:
		return parseFloatNumber(string(typed))
	case string:
		return parseFloatNumber(strings.TrimSpace(typed))
	default:
		return 0, false, nil
	}
}

func parseIntCompatibleNumber(value string) (int, bool, error) {
	if value == "" {
		return 0, false, nil
	}
	if integerSyntax(value) {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return 0, false, ErrCodexUsageOverflow
			}
			return 0, false, nil
		}
		return int64ToInt(parsed)
	}
	parsed, found, err := parseFloatNumber(value)
	if err != nil || !found {
		return 0, found, err
	}
	return truncateFloatToInt(parsed)
}

func parseFloatNumber(value string) (float64, bool, error) {
	if value == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, false, ErrCodexUsageOverflow
		}
		return 0, false, nil
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false, ErrCodexUsageOverflow
	}
	return parsed, true, nil
}

func truncateFloatToInt(value float64) (int, bool, error) {
	return checkedFloatToInt(math.Trunc(value))
}

func roundedFloatToInt(value float64) (int, bool, error) {
	return checkedFloatToInt(math.Round(value))
}

func checkedFloatToInt(value float64) (int, bool, error) {
	limit := math.Ldexp(1, strconv.IntSize-1)
	if math.IsNaN(value) || math.IsInf(value, 0) || value >= limit || value < -limit {
		return 0, false, ErrCodexUsageOverflow
	}
	return int(value), true, nil
}

func int64ToInt(value int64) (int, bool, error) {
	if strconv.IntSize == 32 && (value > int64(maxIntValue()) || value < int64(minIntValue())) {
		return 0, false, ErrCodexUsageOverflow
	}
	return int(value), true, nil
}

func numericText(value any) (string, bool) {
	switch typed := value.(type) {
	case json.Number:
		return string(typed), true
	case string:
		return strings.TrimSpace(typed), true
	default:
		return "", false
	}
}

func integerSyntax(value string) bool {
	if value == "" {
		return false
	}
	start := 0
	if value[0] == '+' || value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return false
	}
	for i := start; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func addMetricValue(current, delta int) (int, bool) {
	if delta > 0 && current > maxIntValue()-delta {
		return 0, false
	}
	if delta < 0 && current < minIntValue()-delta {
		return 0, false
	}
	return current + delta, true
}

func maxIntValue() int {
	return int(^uint(0) >> 1)
}

func minIntValue() int {
	return -maxIntValue() - 1
}

func asMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}
