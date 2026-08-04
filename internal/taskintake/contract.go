package taskintake

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	markdownscan "revolvr/internal/markdown"
)

const (
	// MaximumSourceBytes is the documented task-intake source limit (4 MiB).
	MaximumSourceBytes = 4 << 20
	maximumListItems   = 256
	maximumCriteria    = 128
	maximumItemBytes   = 4096
)

var (
	taskIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	criterionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	canonicalSchema    = regexp.MustCompile(`(?m)^\s*schema\s*:\s*["']?revolvr-task-v1["']?\s*$`)
)

type Budget struct {
	MaximumCycles      int32  `yaml:"max_cycles" json:"max_cycles"`
	MaximumModelTokens int64  `yaml:"max_model_tokens" json:"max_model_tokens"`
	MaximumWallTime    string `yaml:"max_wall_time" json:"max_wall_time"`
}

type Criterion struct {
	ID                     string
	Requirement            string
	VerificationMethod     string
	VerificationReference  string
	OperatorCheckpointText string
}

type verificationPlanEntry struct {
	CriterionID        string `json:"criterion_id"`
	Method             string `json:"method"`
	Reference          string `json:"reference,omitempty"`
	OperatorCheckpoint string `json:"operator_checkpoint,omitempty"`
}

type Contract struct {
	ExternalTaskID        string
	Title                 string
	Goal                  string
	Scope                 []string
	ExcludedScope         []string
	RiskClass             string
	MutationClass         string
	NetworkProfile        string
	Priority              int32
	ReadOnlyInvestigation bool
	Dependencies          []string
	Conflicts             []string
	ExpectedPaths         []string
	Budget                Budget
	SecretRequirements    []string
	Criteria              []Criterion
}

type canonicalFrontmatter struct {
	Schema                string   `yaml:"schema"`
	ID                    string   `yaml:"id"`
	Priority              int32    `yaml:"priority"`
	MutationClass         string   `yaml:"mutation_class"`
	Risk                  string   `yaml:"risk"`
	Network               string   `yaml:"network"`
	DependsOn             []string `yaml:"depends_on"`
	Conflicts             []string `yaml:"conflicts"`
	ExpectedPaths         []string `yaml:"expected_paths"`
	Budget                Budget   `yaml:"budget"`
	SecretRequirements    []string `yaml:"secret_requirements"`
	ReadOnlyInvestigation bool     `yaml:"read_only_investigation"`
}

func parseSource(source []byte) (*Contract, error) {
	frontmatter, body, ok := splitFrontmatter(source)
	if !ok {
		return nil, nil
	}

	var document yaml.Node
	if err := yaml.Unmarshal(frontmatter, &document); err != nil {
		if canonicalSchema.Match(frontmatter) {
			return nil, fmt.Errorf("parse revolvr-task-v1 frontmatter: %w", err)
		}
		return nil, nil
	}
	mapping, err := yamlMapping(&document)
	if err != nil {
		if canonicalSchema.Match(frontmatter) {
			return nil, err
		}
		return nil, nil
	}
	if value := mappingValue(mapping, "schema"); value == nil || value.Value != "revolvr-task-v1" {
		return nil, nil
	}

	allowed := map[string]bool{
		"schema": true, "id": true, "priority": true, "mutation_class": true,
		"risk": true, "network": true, "depends_on": true, "conflicts": true,
		"expected_paths": true, "budget": true, "secret_requirements": true,
		"read_only_investigation": true,
	}
	required := []string{
		"schema", "id", "priority", "mutation_class", "risk", "network",
		"depends_on", "conflicts", "expected_paths", "budget",
	}
	if err := validateMapping(mapping, allowed, required, "frontmatter"); err != nil {
		return nil, err
	}
	budget := mappingValue(mapping, "budget")
	if budget == nil || budget.Kind != yaml.MappingNode {
		return nil, errors.New("frontmatter budget must be a mapping")
	}
	if err := validateMapping(budget, map[string]bool{
		"max_cycles": true, "max_model_tokens": true, "max_wall_time": true,
	}, []string{"max_cycles", "max_model_tokens", "max_wall_time"}, "frontmatter budget"); err != nil {
		return nil, err
	}

	var metadata canonicalFrontmatter
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	decoder.KnownFields(true)
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode revolvr-task-v1 frontmatter: %w", err)
	}
	contract, err := validateMetadata(metadata)
	if err != nil {
		return nil, err
	}
	title, sections, err := parseBody(body, metadata.ReadOnlyInvestigation)
	if err != nil {
		return nil, err
	}
	contract.Title = title
	contract.Goal, err = boundedText("goal", sections["Goal"], 65536)
	if err != nil {
		return nil, err
	}
	if contract.Scope, err = markdownList("scope", sections["Scope"], true); err != nil {
		return nil, err
	}
	if contract.ExcludedScope, err = markdownList("excluded scope", sections["Excluded Scope"], true); err != nil {
		return nil, err
	}
	if acceptance, exists := sections["Acceptance"]; exists {
		contract.Criteria, err = parseCriteria(acceptance)
		if err != nil {
			return nil, err
		}
	}
	if len(contract.Criteria) == 0 && !(contract.MutationClass == "read_only" && contract.ReadOnlyInvestigation) {
		return nil, errors.New("canonical task requires at least one acceptance criterion")
	}
	return &contract, nil
}

func validateMetadata(metadata canonicalFrontmatter) (Contract, error) {
	metadata.Schema = strings.TrimSpace(metadata.Schema)
	metadata.ID = strings.TrimSpace(metadata.ID)
	metadata.MutationClass = strings.TrimSpace(metadata.MutationClass)
	metadata.Risk = strings.TrimSpace(metadata.Risk)
	metadata.Network = strings.TrimSpace(metadata.Network)
	if metadata.Schema != "revolvr-task-v1" {
		return Contract{}, fmt.Errorf("unsupported task schema %q", metadata.Schema)
	}
	if !taskIDPattern.MatchString(metadata.ID) {
		return Contract{}, fmt.Errorf("invalid task id %q", metadata.ID)
	}
	if metadata.Priority < 0 {
		return Contract{}, errors.New("priority must be non-negative")
	}
	if !oneOf(metadata.MutationClass,
		"read_only", "documentation", "test_only", "bounded_source", "database_migration",
		"dependency_change", "architecture_change", "security_sensitive", "release_or_deployment") {
		return Contract{}, fmt.Errorf("invalid mutation class %q", metadata.MutationClass)
	}
	if !oneOf(metadata.Risk, "low", "medium", "high", "critical") {
		return Contract{}, fmt.Errorf("invalid risk class %q", metadata.Risk)
	}
	if !oneOf(metadata.Network, "none", "dependencies", "open") {
		return Contract{}, fmt.Errorf("invalid network profile %q", metadata.Network)
	}
	if metadata.ReadOnlyInvestigation && metadata.MutationClass != "read_only" {
		return Contract{}, errors.New("read_only_investigation requires mutation_class read_only")
	}

	dependencies, err := identityList("depends_on", metadata.DependsOn, metadata.ID)
	if err != nil {
		return Contract{}, err
	}
	conflicts, err := identityList("conflicts", metadata.Conflicts, metadata.ID)
	if err != nil {
		return Contract{}, err
	}
	dependencySet := make(map[string]bool, len(dependencies))
	for _, dependency := range dependencies {
		dependencySet[dependency] = true
	}
	for _, conflict := range conflicts {
		if dependencySet[conflict] {
			return Contract{}, fmt.Errorf("task %q cannot be both a dependency and a conflict", conflict)
		}
	}
	expectedPaths, err := expectedPathList(metadata.ExpectedPaths)
	if err != nil {
		return Contract{}, err
	}
	secrets, err := boundedUniqueList("secret_requirements", metadata.SecretRequirements, false)
	if err != nil {
		return Contract{}, err
	}
	duration, err := time.ParseDuration(strings.TrimSpace(metadata.Budget.MaximumWallTime))
	if err != nil || duration <= 0 || duration > 30*24*time.Hour {
		return Contract{}, fmt.Errorf("invalid budget max_wall_time %q", metadata.Budget.MaximumWallTime)
	}
	if metadata.Budget.MaximumCycles <= 0 || metadata.Budget.MaximumCycles > 1000 {
		return Contract{}, errors.New("budget max_cycles must be between 1 and 1000")
	}
	if metadata.Budget.MaximumModelTokens <= 0 || metadata.Budget.MaximumModelTokens > 1_000_000_000 {
		return Contract{}, errors.New("budget max_model_tokens must be between 1 and 1000000000")
	}
	metadata.Budget.MaximumWallTime = duration.String()

	return Contract{
		ExternalTaskID: metadata.ID, RiskClass: metadata.Risk,
		MutationClass: metadata.MutationClass, NetworkProfile: metadata.Network,
		Priority: metadata.Priority, ReadOnlyInvestigation: metadata.ReadOnlyInvestigation,
		Dependencies: dependencies, Conflicts: conflicts, ExpectedPaths: expectedPaths,
		Budget: metadata.Budget, SecretRequirements: secrets,
	}, nil
}

func splitFrontmatter(source []byte) ([]byte, string, bool) {
	if !utf8.Valid(source) {
		return nil, "", false
	}
	normalized := strings.ReplaceAll(string(source), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return []byte(strings.Join(lines[1:i], "\n")), strings.Join(lines[i+1:], "\n"), true
		}
	}
	if canonicalSchema.Match(source) {
		return source, "", true
	}
	return nil, "", false
}

func yamlMapping(document *yaml.Node) (*yaml.Node, error) {
	if document == nil || document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("revolvr-task-v1 frontmatter must be one YAML mapping")
	}
	return document.Content[0], nil
}

func validateMapping(mapping *yaml.Node, allowed map[string]bool, required []string, label string) error {
	seen := make(map[string]bool, len(mapping.Content)/2)
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value
		if seen[key] {
			return fmt.Errorf("duplicate %s key %q", label, key)
		}
		seen[key] = true
		if !allowed[key] {
			return fmt.Errorf("unsupported %s key %q", label, key)
		}
	}
	for _, key := range required {
		if !seen[key] {
			return fmt.Errorf("%s key %q is required", label, key)
		}
	}
	return nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func parseBody(body string, readOnlyInvestigation bool) (string, map[string]string, error) {
	lines := strings.Split(body, "\n")
	sections := map[string]string{}
	var title string
	var current string
	var content []string
	var fence markdownscan.Fence
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(strings.Join(content, "\n"))
		}
		content = nil
	}
	order := []string{"Goal", "Scope", "Excluded Scope", "Acceptance"}
	next := 0
	for lineNumber, line := range lines {
		outside := fence.Scan(line) == markdownscan.LineOutsideFence
		level, text, heading := markdownHeading(line)
		if outside && heading {
			switch level {
			case 1:
				if title != "" || current != "" || hasNonblank(content) {
					return "", nil, fmt.Errorf("line %d: task title must be the first heading", lineNumber+1)
				}
				title = strings.Join(strings.Fields(text), " ")
				if title == "" || len(title) > 1024 {
					return "", nil, errors.New("task title must contain at most 1024 bytes")
				}
				continue
			case 2:
				if title == "" {
					return "", nil, fmt.Errorf("line %d: task section appears before title", lineNumber+1)
				}
				if next >= len(order) || text != order[next] {
					if readOnlyInvestigation && next == 3 {
						return "", nil, fmt.Errorf("line %d: unsupported task section %q", lineNumber+1, text)
					}
					want := "no more sections"
					if next < len(order) {
						want = order[next]
					}
					return "", nil, fmt.Errorf("line %d: got task section %q, want %q", lineNumber+1, text, want)
				}
				flush()
				current = text
				next++
				continue
			case 3:
				if current != "Acceptance" {
					return "", nil, fmt.Errorf("line %d: criterion heading outside Acceptance", lineNumber+1)
				}
			}
		}
		if title == "" && strings.TrimSpace(line) != "" {
			return "", nil, fmt.Errorf("line %d: content appears before task title", lineNumber+1)
		}
		if current == "" {
			if title != "" && strings.TrimSpace(line) != "" {
				return "", nil, fmt.Errorf("line %d: content appears before Goal", lineNumber+1)
			}
			continue
		}
		content = append(content, line)
	}
	flush()
	if title == "" {
		return "", nil, errors.New("canonical task requires one level-1 title")
	}
	for _, required := range order[:3] {
		if strings.TrimSpace(sections[required]) == "" {
			return "", nil, fmt.Errorf("canonical task requires non-empty %s section", required)
		}
	}
	if _, ok := sections["Acceptance"]; !ok && !readOnlyInvestigation {
		return "", nil, errors.New("canonical task requires Acceptance section")
	}
	return title, sections, nil
}

func parseCriteria(raw string) ([]Criterion, error) {
	lines := strings.Split(raw, "\n")
	type criterionSection struct {
		id    string
		lines []string
	}
	var sections []criterionSection
	var current *criterionSection
	var fence markdownscan.Fence
	for lineNumber, line := range lines {
		outside := fence.Scan(line) == markdownscan.LineOutsideFence
		level, text, heading := markdownHeading(line)
		if outside && heading && level == 3 {
			sections = append(sections, criterionSection{id: strings.TrimSpace(text)})
			current = &sections[len(sections)-1]
			continue
		}
		if current == nil {
			if strings.TrimSpace(line) != "" {
				return nil, fmt.Errorf("acceptance line %d: content appears before criterion heading", lineNumber+1)
			}
			continue
		}
		current.lines = append(current.lines, line)
	}
	if len(sections) > maximumCriteria {
		return nil, fmt.Errorf("acceptance has %d criteria; maximum is %d", len(sections), maximumCriteria)
	}
	criteria := make([]Criterion, 0, len(sections))
	seen := map[string]bool{}
	for _, section := range sections {
		if !criterionIDPattern.MatchString(section.id) {
			return nil, fmt.Errorf("invalid criterion id %q", section.id)
		}
		if seen[section.id] {
			return nil, fmt.Errorf("duplicate criterion id %q", section.id)
		}
		seen[section.id] = true
		criterion, err := parseCriterion(section.id, section.lines)
		if err != nil {
			return nil, err
		}
		criteria = append(criteria, criterion)
	}
	return criteria, nil
}

func parseCriterion(id string, lines []string) (Criterion, error) {
	marker := -1
	method := ""
	var fence markdownscan.Fence
	for i, line := range lines {
		if fence.Scan(line) != markdownscan.LineOutsideFence {
			continue
		}
		switch strings.TrimSpace(line) {
		case "Verification:":
			if marker >= 0 {
				return Criterion{}, fmt.Errorf("criterion %q has multiple verification methods", id)
			}
			marker, method = i, "command"
		case "Operator Checkpoint:":
			if marker >= 0 {
				return Criterion{}, fmt.Errorf("criterion %q has multiple verification methods", id)
			}
			marker, method = i, "operator_checkpoint"
		}
	}
	if marker < 0 {
		return Criterion{}, fmt.Errorf("criterion %q requires Verification or Operator Checkpoint", id)
	}
	requirement, err := boundedText("criterion "+id+" requirement", strings.Join(lines[:marker], "\n"), 65536)
	if err != nil {
		return Criterion{}, err
	}
	referenceText, err := stripCodeFence(lines[marker+1:])
	if err != nil {
		return Criterion{}, fmt.Errorf("criterion %q: %w", id, err)
	}
	reference, err := boundedText("criterion "+id+" verification", referenceText, 65536)
	if err != nil {
		return Criterion{}, err
	}
	criterion := Criterion{ID: id, Requirement: requirement, VerificationMethod: method}
	if method == "command" {
		criterion.VerificationReference = reference
	} else {
		criterion.OperatorCheckpointText = reference
	}
	return criterion, nil
}

func stripCodeFence(lines []string) (string, error) {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	lines = lines[start:end]
	if len(lines) < 2 {
		return strings.Join(lines, "\n"), nil
	}
	opening := strings.TrimSpace(lines[0])
	if len(opening) < 3 || (opening[0] != '`' && opening[0] != '~') {
		return strings.Join(lines, "\n"), nil
	}
	marker := opening[0]
	count := 0
	for count < len(opening) && opening[count] == marker {
		count++
	}
	closing := strings.TrimSpace(lines[len(lines)-1])
	if count < 3 || len(closing) < count || strings.Trim(closing, string(marker)) != "" {
		return "", errors.New("verification code fence is not closed")
	}
	return strings.Join(lines[1:len(lines)-1], "\n"), nil
}

func markdownList(label, raw string, required bool) ([]string, error) {
	var items []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") {
			return nil, fmt.Errorf("%s must contain one '- value' item per line", label)
		}
		items = append(items, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}
	return boundedUniqueList(label, items, required)
}

func identityList(label string, values []string, self string) ([]string, error) {
	values, err := boundedUniqueList(label, values, false)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if !taskIDPattern.MatchString(value) {
			return nil, fmt.Errorf("invalid %s task id %q", label, value)
		}
		if value == self {
			return nil, fmt.Errorf("%s contains self reference %q", label, self)
		}
	}
	return values, nil
}

func expectedPathList(values []string) ([]string, error) {
	values, err := boundedUniqueList("expected_paths", values, true)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		clean := path.Clean(value)
		if strings.HasPrefix(value, "/") || clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(value, "\\") {
			return nil, fmt.Errorf("unsafe expected path %q", value)
		}
	}
	return values, nil
}

func boundedUniqueList(label string, values []string, required bool) ([]string, error) {
	if len(values) > maximumListItems {
		return nil, fmt.Errorf("%s has %d items; maximum is %d", label, len(values), maximumListItems)
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > maximumItemBytes || strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("%s contains an empty or oversized item", label)
		}
		if seen[value] {
			return nil, fmt.Errorf("%s contains duplicate item %q", label, value)
		}
		seen[value] = true
		out = append(out, value)
	}
	if required && len(out) == 0 {
		return nil, fmt.Errorf("%s requires at least one item", label)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

func boundedText(label, value string, maximum int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum || strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("%s must contain at most %d bytes", label, maximum)
	}
	return value, nil
}

func markdownHeading(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 6 || level == len(trimmed) || (trimmed[level] != ' ' && trimmed[level] != '\t') {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed[level:]), true
}

func hasNonblank(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

func oneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

func (contract Contract) verificationPlan() []verificationPlanEntry {
	plan := make([]verificationPlanEntry, 0, len(contract.Criteria))
	for _, criterion := range contract.Criteria {
		plan = append(plan, verificationPlanEntry{
			CriterionID: criterion.ID, Method: criterion.VerificationMethod,
			Reference:          criterion.VerificationReference,
			OperatorCheckpoint: criterion.OperatorCheckpointText,
		})
	}
	return plan
}
