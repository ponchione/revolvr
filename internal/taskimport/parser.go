// Package taskimport parses Revolvr's small Markdown task import format.
//
// A task import document may start with an optional level-1 title. Each task
// starts with a level-2 task heading:
//
//	## Task: optional short summary
//
// The task body is required and is the text after the task heading, or the text
// in a "### Task" or "### Task Body" subsection. Optional "### Summary",
// "### Acceptance", and "### Verification" subsections are recognized. Summary
// becomes TaskSpec.Summary, while acceptance and verification notes remain in
// TaskSpec.Task so Codex sees the human-readable notes. Any other subsection in
// a task is preserved in TaskSpec.Task.
package taskimport

import (
	"fmt"
	"strings"

	markdownscan "revolvr/internal/markdown"
)

// TaskSpec is a parsed task ready to map to internal/app.AddTaskInput.
type TaskSpec struct {
	Task      string
	Summary   string
	DependsOn []string
	Tags      []string
	Conflicts []string
}

type taskSection struct {
	startLine      int
	headingSummary string
	bodyLines      []string
	subsections    []subsection
}

type subsection struct {
	startLine   int
	headingLine string
	title       string
	lines       []string
}

type heading struct {
	level int
	text  string
}

type sectionKind int

const (
	sectionUnknown sectionKind = iota
	sectionTaskBody
	sectionSummary
	sectionAcceptance
	sectionVerification
	sectionDependsOn
	sectionTags
	sectionConflicts
)

// Parse parses a Markdown task import document.
func Parse(markdown []byte) ([]TaskSpec, error) {
	return ParseString(string(markdown))
}

// ParseString parses a Markdown task import document.
func ParseString(markdown string) ([]TaskSpec, error) {
	lines := splitLines(markdown)
	sections, err := parseTaskSections(lines)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		return nil, parseError(1, "no task sections found; expected a level-2 task heading like %q", "## Task: <summary>")
	}

	specs := make([]TaskSpec, 0, len(sections))
	for _, section := range sections {
		spec, err := buildTaskSpec(section)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

func parseTaskSections(lines []string) ([]taskSection, error) {
	var (
		sections    []taskSection
		current     *taskSection
		activeIndex = -1
		fence       markdownscan.Fence
	)

	for i, line := range lines {
		lineNumber := i + 1
		if fence.Scan(line) == markdownscan.LineOutsideFence {
			if h, ok := parseHeading(line); ok {
				if h.level == 2 && isTaskHeading(h.text) {
					if current != nil {
						sections = append(sections, *current)
					}
					current = &taskSection{
						startLine:      lineNumber,
						headingSummary: taskHeadingSummary(h.text),
					}
					activeIndex = -1
					continue
				}

				if current == nil {
					if h.level == 1 {
						continue
					}
					return nil, parseError(lineNumber, "expected a task section heading like %q, got %q", "## Task: <summary>", strings.TrimSpace(line))
				}

				if h.level == 2 || h.level == 3 {
					current.subsections = append(current.subsections, subsection{
						startLine:   lineNumber,
						headingLine: strings.TrimSpace(line),
						title:       h.text,
					})
					activeIndex = len(current.subsections) - 1
					continue
				}
			}
		}

		if current == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			return nil, parseError(lineNumber, "content before first task section; expected a level-2 task heading like %q", "## Task: <summary>")
		}
		if activeIndex >= 0 {
			current.subsections[activeIndex].lines = append(current.subsections[activeIndex].lines, line)
			continue
		}
		current.bodyLines = append(current.bodyLines, line)
	}

	if current != nil {
		sections = append(sections, *current)
	}
	return sections, nil
}

func buildTaskSpec(section taskSection) (TaskSpec, error) {
	var (
		parts                      []string
		hasTaskBody                bool
		summary                    = oneLine(section.headingSummary)
		seenSummary                bool
		seenAcceptance             bool
		seenVerify                 bool
		dependsOn, tags, conflicts []string
	)

	if body := linesText(section.bodyLines); body != "" {
		parts = append(parts, body)
		hasTaskBody = true
	}

	for _, sub := range section.subsections {
		content := linesText(sub.lines)
		switch sectionKindFor(sub.title) {
		case sectionTaskBody:
			if content == "" {
				return TaskSpec{}, parseError(sub.startLine, "task body section is empty")
			}
			parts = append(parts, content)
			hasTaskBody = true
		case sectionSummary:
			if seenSummary {
				return TaskSpec{}, parseError(sub.startLine, "duplicate Summary section")
			}
			if content == "" {
				return TaskSpec{}, parseError(sub.startLine, "Summary section is empty")
			}
			seenSummary = true
			summary = oneLine(content)
		case sectionAcceptance:
			if seenAcceptance {
				return TaskSpec{}, parseError(sub.startLine, "duplicate Acceptance section")
			}
			if content == "" {
				return TaskSpec{}, parseError(sub.startLine, "Acceptance section is empty")
			}
			seenAcceptance = true
			parts = append(parts, sectionMarkdown(sub, content))
		case sectionVerification:
			if seenVerify {
				return TaskSpec{}, parseError(sub.startLine, "duplicate Verification section")
			}
			if content == "" {
				return TaskSpec{}, parseError(sub.startLine, "Verification section is empty")
			}
			seenVerify = true
			parts = append(parts, sectionMarkdown(sub, content))
		case sectionDependsOn, sectionTags, sectionConflicts:
			if content == "" {
				return TaskSpec{}, parseError(sub.startLine, "%s section is empty", sub.title)
			}
			items, err := metadataList(sub.lines)
			if err != nil {
				return TaskSpec{}, parseError(sub.startLine, "%s: %v", sub.title, err)
			}
			switch sectionKindFor(sub.title) {
			case sectionDependsOn:
				if dependsOn != nil {
					return TaskSpec{}, parseError(sub.startLine, "duplicate Depends On section")
				}
				dependsOn = items
			case sectionTags:
				if tags != nil {
					return TaskSpec{}, parseError(sub.startLine, "duplicate Tags section")
				}
				tags = items
			case sectionConflicts:
				if conflicts != nil {
					return TaskSpec{}, parseError(sub.startLine, "duplicate Conflicts section")
				}
				conflicts = items
			}
			parts = append(parts, sectionMarkdown(sub, content))
		default:
			if unknown := sectionMarkdown(sub, content); unknown != "" {
				parts = append(parts, unknown)
			}
		}
	}

	if !hasTaskBody {
		return TaskSpec{}, parseError(section.startLine, "task section has empty task text")
	}
	return TaskSpec{
		Task:      strings.Join(parts, "\n\n"),
		Summary:   summary,
		DependsOn: dependsOn,
		Tags:      tags,
		Conflicts: conflicts,
	}, nil
}

func metadataList(lines []string) ([]string, error) {
	var result []string
	seen := map[string]struct{}{}
	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, "- ") {
			return nil, fmt.Errorf("expected one '- value' item per line")
		}
		value = strings.TrimSpace(strings.TrimPrefix(value, "- "))
		if value == "" || strings.ContainsAny(value, " ,\t\r\n") {
			return nil, fmt.Errorf("invalid metadata item %q", value)
		}
		if _, ok := seen[value]; ok {
			return nil, fmt.Errorf("duplicate metadata item %q", value)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no metadata items")
	}
	return result, nil
}

func splitLines(markdown string) []string {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}

func parseHeading(line string) (heading, bool) {
	leftTrimmed := strings.TrimLeft(line, " ")
	if len(line)-len(leftTrimmed) > 3 || !strings.HasPrefix(leftTrimmed, "#") {
		return heading{}, false
	}

	level := 0
	for level < len(leftTrimmed) && leftTrimmed[level] == '#' {
		level++
	}
	if level > 6 || level == len(leftTrimmed) {
		return heading{}, false
	}
	if leftTrimmed[level] != ' ' && leftTrimmed[level] != '\t' {
		return heading{}, false
	}

	text := strings.TrimSpace(leftTrimmed[level:])
	return heading{level: level, text: stripClosingHashes(text)}, true
}

func stripClosingHashes(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasSuffix(text, "#") {
		return text
	}

	lastNonHash := len(text) - 1
	for lastNonHash >= 0 && text[lastNonHash] == '#' {
		lastNonHash--
	}
	if lastNonHash < 0 || (text[lastNonHash] != ' ' && text[lastNonHash] != '\t') {
		return text
	}
	return strings.TrimSpace(text[:lastNonHash])
}

func isTaskHeading(text string) bool {
	text = strings.TrimSpace(text)
	if strings.EqualFold(text, "Task") {
		return true
	}
	if len(text) < len("Task:") {
		return false
	}
	return strings.EqualFold(text[:len("Task")], "Task") && strings.HasPrefix(strings.TrimSpace(text[len("Task"):]), ":")
}

func taskHeadingSummary(text string) string {
	text = strings.TrimSpace(text)
	if strings.EqualFold(text, "Task") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text[len("Task"):]), ":"))
}

func sectionKindFor(title string) sectionKind {
	normalized := strings.ToLower(strings.TrimSpace(title))
	normalized = strings.TrimSuffix(normalized, ":")
	normalized = strings.Join(strings.Fields(normalized), " ")

	switch normalized {
	case "task", "task body", "body":
		return sectionTaskBody
	case "summary":
		return sectionSummary
	case "acceptance", "acceptance criteria", "acceptance notes":
		return sectionAcceptance
	case "verification", "verification notes":
		return sectionVerification
	case "depends on", "depends_on":
		return sectionDependsOn
	case "tags":
		return sectionTags
	case "conflicts":
		return sectionConflicts
	default:
		return sectionUnknown
	}
}

func linesText(lines []string) string {
	lines = trimBlankLines(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func trimBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

func sectionMarkdown(sub subsection, content string) string {
	headingLine := strings.TrimSpace(sub.headingLine)
	if headingLine == "" {
		return content
	}
	if content == "" {
		return headingLine
	}
	return headingLine + "\n" + content
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func parseError(line int, format string, args ...any) error {
	return fmt.Errorf("parse task import: line %d: %s", line, fmt.Sprintf(format, args...))
}
