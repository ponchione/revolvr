package receipt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func RewriteMetricsFromCodexJSONL(content []byte, jsonl []byte) ([]byte, Receipt, bool, error) {
	metrics, found, err := ParseCodexUsageMetrics(jsonl)
	if err != nil {
		return nil, Receipt{}, false, err
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

func ParseCodexUsageMetrics(jsonl []byte) (Metrics, bool, error) {
	scanner := bufio.NewScanner(bytes.NewReader(jsonl))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	total := Metrics{}
	found := false
	var firstParseErr error
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			if firstParseErr == nil {
				firstParseErr = fmt.Errorf("receipt: parse codex jsonl line %d: %w", lineNumber, err)
			}
			continue
		}
		usage, ok := usageMap(event)
		if !ok {
			continue
		}
		metrics, metricsFound := metricsFromMap(usage)
		if !metricsFound {
			continue
		}
		if metrics.DurationSeconds == 0 {
			if duration, ok := durationSeconds(event); ok {
				metrics.DurationSeconds = duration
			}
		}
		total.InputTokens += metrics.InputTokens
		total.OutputTokens += metrics.OutputTokens
		total.DurationSeconds += metrics.DurationSeconds
		found = true
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return Metrics{}, false, fmt.Errorf("receipt: read codex jsonl: %w", err)
	}
	if !found && firstParseErr != nil {
		return Metrics{}, false, firstParseErr
	}
	return total, found, nil
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

func usageMap(event map[string]any) (map[string]any, bool) {
	for _, key := range usageKeys {
		if usage, ok := asMap(event[key]); ok {
			return usage, true
		}
	}
	for _, key := range []string{"response", "result", "message", "event"} {
		if parent, ok := asMap(event[key]); ok {
			for _, usageKey := range usageKeys {
				if usage, ok := asMap(parent[usageKey]); ok {
					return usage, true
				}
			}
		}
	}
	if hasUsageFields(event) {
		return event, true
	}
	return nestedUsageMap(event)
}

func nestedUsageMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isUsageKey(key) {
				if usage, ok := asMap(child); ok {
					return usage, true
				}
			}
			if usage, ok := nestedUsageMap(child); ok {
				return usage, true
			}
		}
	case []any:
		for _, child := range typed {
			if usage, ok := nestedUsageMap(child); ok {
				return usage, true
			}
		}
	}
	return nil, false
}

var usageKeys = []string{"usage", "total_usage", "token_usage", "total_token_usage"}

func isUsageKey(key string) bool {
	for _, usageKey := range usageKeys {
		if key == usageKey {
			return true
		}
	}
	return false
}

func metricsFromMap(values map[string]any) (Metrics, bool) {
	input, inputOK := firstInt(values, "input_tokens", "prompt_tokens", "total_input_tokens")
	output, outputOK := firstInt(values, "output_tokens", "completion_tokens", "total_output_tokens")
	duration, durationOK := durationSeconds(values)
	return Metrics{
		InputTokens:     input,
		OutputTokens:    output,
		DurationSeconds: duration,
	}, inputOK || outputOK || durationOK
}

func hasUsageFields(values map[string]any) bool {
	_, inputOK := firstInt(values, "input_tokens", "prompt_tokens", "total_input_tokens")
	_, outputOK := firstInt(values, "output_tokens", "completion_tokens", "total_output_tokens")
	_, durationOK := durationSeconds(values)
	return inputOK || outputOK || durationOK
}

func firstInt(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			parsed, ok := numberAsFloat(value)
			if ok {
				return int(parsed), true
			}
		}
	}
	return 0, false
}

func durationSeconds(values map[string]any) (int, bool) {
	for _, key := range []string{"duration_seconds", "duration_secs", "elapsed_seconds"} {
		if value, ok := values[key]; ok {
			seconds, ok := numberAsFloat(value)
			if ok {
				return int(math.Round(seconds)), true
			}
		}
	}
	for _, key := range []string{"duration_ms", "elapsed_ms"} {
		if value, ok := values[key]; ok {
			milliseconds, ok := numberAsFloat(value)
			if ok {
				return int(math.Round(milliseconds / 1000)), true
			}
		}
	}
	return 0, false
}

func numberAsFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func asMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	return typed, ok
}
