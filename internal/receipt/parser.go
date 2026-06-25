package receipt

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrMissingFrontmatter = errors.New("receipt: missing or malformed YAML frontmatter")
	ErrMissingField       = errors.New("receipt: missing required field")
	ErrInvalidField       = errors.New("receipt: invalid field")
	ErrInvalidVerdict     = errors.New("receipt: invalid verdict")
	ErrMissingSection     = errors.New("receipt: missing required section")
)

var requiredFields = []string{
	"schema_version",
	"run_id",
	"pass_id",
	"task_id",
	"task",
	"verdict",
	"timestamp",
	"codex_exit_code",
	"verification_status",
	"commit_sha",
	"changed_files",
	"verification",
	"metrics",
}

var requiredMetricFields = []string{
	"input_tokens",
	"output_tokens",
	"duration_seconds",
}

var RequiredSections = []string{
	"Summary",
	"Changed Files",
	"Verification",
	"Concerns",
	"Next Steps",
}

func Parse(content []byte) (Receipt, error) {
	frontmatter, body, ok := splitFrontmatter(content)
	if !ok {
		return Receipt{}, ErrMissingFrontmatter
	}

	var root yaml.Node
	if err := yaml.Unmarshal(frontmatter, &root); err != nil {
		return Receipt{}, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	mapping, err := frontmatterMapping(&root)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateRequiredFrontmatter(mapping); err != nil {
		return Receipt{}, err
	}

	var parsed Receipt
	if err := yaml.Unmarshal(frontmatter, &parsed); err != nil {
		return Receipt{}, fmt.Errorf("receipt: decode yaml: %w", err)
	}
	parsed.normalize()
	parsed.RawBody = string(body)
	parsed.ChangedFileClaims = ParseChangedFiles(parsed.RawBody)
	parsed.VerificationClaims = ParseVerificationClaims(parsed.RawBody)
	if err := parsed.validate(); err != nil {
		return Receipt{}, err
	}
	if err := ValidateRequiredSections(parsed.RawBody, RequiredSections); err != nil {
		return Receipt{}, err
	}
	return parsed, nil
}

func (r *Receipt) normalize() {
	r.SchemaVersion = strings.TrimSpace(r.SchemaVersion)
	r.RunID = strings.TrimSpace(r.RunID)
	r.PassID = strings.TrimSpace(r.PassID)
	r.TaskID = strings.TrimSpace(r.TaskID)
	r.Task = strings.TrimSpace(r.Task)
	r.Verdict = Verdict(strings.TrimSpace(string(r.Verdict)))
	r.VerificationStatus = strings.TrimSpace(r.VerificationStatus)
	r.CommitSHA = strings.TrimSpace(r.CommitSHA)
	r.ChangedFiles = compactStrings(r.ChangedFiles)
	for i := range r.Verification {
		r.Verification[i].Command = strings.TrimSpace(r.Verification[i].Command)
		r.Verification[i].Status = strings.TrimSpace(r.Verification[i].Status)
	}
}

func (r Receipt) validate() error {
	if r.SchemaVersion == "" {
		return fmt.Errorf("%w: schema_version", ErrMissingField)
	}
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema_version (got %q, want %q)", ErrInvalidField, r.SchemaVersion, SchemaVersion)
	}
	if r.RunID == "" {
		return fmt.Errorf("%w: run_id", ErrMissingField)
	}
	if r.PassID == "" {
		return fmt.Errorf("%w: pass_id", ErrMissingField)
	}
	if r.TaskID == "" {
		return fmt.Errorf("%w: task_id", ErrMissingField)
	}
	if r.Task == "" {
		return fmt.Errorf("%w: task", ErrMissingField)
	}
	if r.Verdict == "" {
		return fmt.Errorf("%w: verdict", ErrMissingField)
	}
	if !validVerdict(r.Verdict) {
		return fmt.Errorf("%w: %q", ErrInvalidVerdict, r.Verdict)
	}
	if r.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp", ErrMissingField)
	}
	if r.VerificationStatus == "" {
		return fmt.Errorf("%w: verification_status", ErrMissingField)
	}
	if r.Metrics.InputTokens < 0 {
		return fmt.Errorf("%w: metrics.input_tokens (must be >= 0, got %d)", ErrInvalidField, r.Metrics.InputTokens)
	}
	if r.Metrics.OutputTokens < 0 {
		return fmt.Errorf("%w: metrics.output_tokens (must be >= 0, got %d)", ErrInvalidField, r.Metrics.OutputTokens)
	}
	if r.Metrics.DurationSeconds < 0 {
		return fmt.Errorf("%w: metrics.duration_seconds (must be >= 0, got %d)", ErrInvalidField, r.Metrics.DurationSeconds)
	}
	for i, entry := range r.Verification {
		if entry.Command == "" {
			return fmt.Errorf("%w: verification[%d].command", ErrMissingField, i)
		}
	}
	return nil
}

func validVerdict(verdict Verdict) bool {
	switch verdict {
	case VerdictCompleted, VerdictCompletedWithConcerns, VerdictBlocked, VerdictVerificationFailed, VerdictCodexFailed, VerdictSafetyLimit, VerdictNoChanges:
		return true
	default:
		return false
	}
}

func validateRequiredFrontmatter(mapping *yaml.Node) error {
	for _, key := range requiredFields {
		if yamlMappingValue(mapping, key) == nil {
			return fmt.Errorf("%w: %s", ErrMissingField, key)
		}
	}
	metrics := yamlMappingValue(mapping, "metrics")
	if metrics == nil || metrics.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: metrics", ErrMissingField)
	}
	for _, key := range requiredMetricFields {
		if yamlMappingValue(metrics, key) == nil {
			return fmt.Errorf("%w: metrics.%s", ErrMissingField, key)
		}
	}
	return nil
}

func frontmatterMapping(root *yaml.Node) (*yaml.Node, error) {
	if root == nil {
		return nil, ErrMissingFrontmatter
	}
	node := root
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, ErrMissingFrontmatter
		}
		node = root.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil, ErrMissingFrontmatter
	}
	return node, nil
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func ValidateRequiredSections(body string, required []string) error {
	present := receiptSections(body)
	missing := make([]string, 0)
	for _, section := range required {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if _, ok := present[normalizeSection(section)]; !ok {
			missing = append(missing, section)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingSection, strings.Join(missing, ", "))
	}
	return nil
}

func HasSection(body string, section string) bool {
	_, ok := receiptSections(body)[normalizeSection(section)]
	return ok
}

func receiptSections(body string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		if name != "" {
			out[normalizeSection(name)] = struct{}{}
		}
	}
	return out
}

func normalizeSection(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
	if bytes.HasPrefix(content, []byte("---\n")) {
		rest := content[len("---\n"):]
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true
		}
		return nil, nil, false
	}
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		rest := content[len("---\r\n"):]
		if idx := bytes.Index(rest, []byte("\r\n---\r\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\r\n---\r\n"):], true
		}
		if idx := bytes.Index(rest, []byte("\n---\n")); idx >= 0 {
			return rest[:idx], rest[idx+len("\n---\n"):], true
		}
	}
	return nil, nil, false
}
