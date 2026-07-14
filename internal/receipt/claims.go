package receipt

import (
	"regexp"
	"strconv"
	"strings"

	markdownscan "revolvr/internal/markdown"
)

var exitCodePattern = regexp.MustCompile(`(?i)\b(?:exit(?:_code)?|code)\s*[:= ]\s*(-?\d+)\b`)

func ParseChangedFiles(body string) []string {
	lines := sectionContent(body, "Changed Files")
	paths := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(value string) {
		value = normalizeChangedFilePath(value)
		if ignoreClaim(value) {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		paths = append(paths, value)
	}

	var fence markdownscan.Fence
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineKind := fence.Scan(line)
		if lineKind == markdownscan.LineFenceBoundary {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			addChangedFileClaim(add, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		if lineKind == markdownscan.LineInsideFence {
			addChangedFileClaim(add, trimmed)
		}
	}
	return paths
}

func ParseVerificationCommands(body string) []string {
	claims := ParseVerificationClaims(body)
	commands := make([]string, 0, len(claims))
	for _, claim := range claims {
		commands = append(commands, claim.Command)
	}
	return commands
}

func ParseVerificationClaims(body string) []VerificationClaim {
	lines := sectionContent(body, "Verification")
	claims := make([]VerificationClaim, 0)
	seen := map[string]struct{}{}
	add := func(raw string) {
		claim := parseVerificationClaim(raw)
		if claim.Command == "" {
			return
		}
		if _, ok := seen[claim.Command]; ok {
			return
		}
		seen[claim.Command] = struct{}{}
		claims = append(claims, claim)
	}

	var fence markdownscan.Fence
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineKind := fence.Scan(line)
		if lineKind == markdownscan.LineFenceBoundary {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			add(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		if lineKind == markdownscan.LineInsideFence || looksLikeVerificationCommand(trimmed) {
			add(trimmed)
		}
	}
	return claims
}

func addChangedFileClaim(add func(string), raw string) {
	raw = stripCheckbox(strings.TrimSpace(raw))
	if raw == "" {
		return
	}
	spans := codeSpans(raw)
	if len(spans) > 0 {
		for _, span := range spans {
			add(span)
		}
		return
	}
	for _, sep := range []string{" -> ", "=>"} {
		if before, after, ok := strings.Cut(raw, sep); ok {
			add(before)
			add(after)
			return
		}
	}
	add(raw)
}

func normalizeChangedFilePath(value string) string {
	value = strings.TrimSpace(value)
	value = stripCheckbox(value)
	for _, prefix := range []string{
		"modified:", "updated:", "added:", "created:", "deleted:", "removed:", "renamed:",
		"M ", "A ", "D ", "R ", "?? ",
	} {
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix)) {
			value = strings.TrimSpace(value[len(prefix):])
			break
		}
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		value = strings.TrimSpace(unquoted)
	}
	value = strings.Trim(value, "`'\" ")
	if before, after, ok := strings.Cut(value, " - "); ok && claimSuffix(after) {
		value = before
	}
	if before, after, ok := strings.Cut(value, ": "); ok && strings.ContainsAny(before, "/.") && claimSuffix(after) {
		value = before
	}
	fields := strings.Fields(value)
	if len(fields) > 1 && strings.ContainsAny(fields[0], "/.") {
		value = fields[0]
	}
	return strings.TrimRight(strings.TrimSpace(value), ".,;")
}

func parseVerificationClaim(raw string) VerificationClaim {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "$ "))
	raw = stripCheckbox(raw)
	if raw == "" || ignoreClaim(raw) {
		return VerificationClaim{}
	}
	status := claimStatus(raw)
	exitCode, hasExitCode := claimExitCode(raw)

	command := ""
	if spans := codeSpans(raw); len(spans) > 0 {
		command = spans[0]
	} else {
		command = raw
		if before, after, ok := strings.Cut(command, " - "); ok && claimSuffix(after) {
			command = before
		}
		if before, after, ok := strings.Cut(command, " ("); ok && claimSuffix(strings.TrimSuffix(after, ")")) {
			command = before
		}
		if key, value, ok := strings.Cut(command, ":"); ok {
			switch normalizeSection(key) {
			case "command", "cmd":
				command = strings.TrimSpace(value)
			case "passed", "pass", "failed", "fail", "skipped", "not run":
				command = strings.TrimSpace(value)
			}
		}
	}
	command = strings.Trim(strings.TrimSpace(strings.TrimPrefix(command, "$ ")), "`")
	if ignoreClaim(command) {
		return VerificationClaim{}
	}
	return VerificationClaim{
		Command:     command,
		ExitCode:    exitCode,
		HasExitCode: hasExitCode,
		Status:      status,
	}
}

func sectionContent(body string, section string) []string {
	section = normalizeSection(section)
	var lines []string
	inSection := false
	var fence markdownscan.Fence
	for _, line := range strings.Split(body, "\n") {
		if fence.Scan(line) == markdownscan.LineOutsideFence {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
				inSection = normalizeSection(name) == section
				continue
			}
		}
		if inSection {
			lines = append(lines, line)
		}
	}
	return lines
}

func codeSpans(value string) []string {
	spans := []string{}
	for {
		_, after, ok := strings.Cut(value, "`")
		if !ok {
			return spans
		}
		span, rest, ok := strings.Cut(after, "`")
		if !ok {
			return spans
		}
		span = strings.TrimSpace(span)
		if span != "" {
			spans = append(spans, span)
		}
		value = rest
	}
}

func stripCheckbox(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[ ] ") || strings.HasPrefix(strings.ToLower(value), "[x] ") {
		return strings.TrimSpace(value[4:])
	}
	return value
}

func ignoreClaim(value string) bool {
	value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".,;")
	switch value {
	case "", "none", "no changes", "no changed files", "n/a", "not applicable", "not run", "not run yet":
		return true
	default:
		return false
	}
}

func claimSuffix(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" ||
		strings.Contains(value, "pass") ||
		strings.Contains(value, "fail") ||
		strings.Contains(value, "skip") ||
		strings.Contains(value, "not run") ||
		strings.Contains(value, "exit") ||
		strings.Contains(value, "added") ||
		strings.Contains(value, "updated") ||
		strings.Contains(value, "modified") ||
		strings.Contains(value, "deleted") ||
		strings.Contains(value, "removed") ||
		strings.Contains(value, "created") ||
		strings.Contains(value, "renamed")
}

func claimStatus(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "not run"):
		return "not_run"
	case strings.Contains(lower, "skip"):
		return "skipped"
	case strings.Contains(lower, "fail"):
		return "failed"
	case strings.Contains(lower, "pass"):
		return "passed"
	default:
		return ""
	}
}

func claimExitCode(value string) (int, bool) {
	match := exitCodePattern.FindStringSubmatch(value)
	if len(match) != 2 {
		return 0, false
	}
	exitCode, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return exitCode, true
}

func looksLikeVerificationCommand(value string) bool {
	value = strings.TrimSpace(strings.TrimPrefix(value, "$ "))
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{
		"go test",
		"go run",
		"go build",
		"make ",
		"just ",
		"npm ",
		"pnpm ",
		"yarn ",
		"bun ",
		"deno ",
		"pytest",
		"python ",
		"cargo ",
		"git ",
		"docker ",
		"revolvr ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}
