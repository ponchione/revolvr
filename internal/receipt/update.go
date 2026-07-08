package receipt

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type HarnessFields struct {
	Timestamp          time.Time
	Verdict            Verdict
	CodexExitCode      int
	VerificationStatus string
	CommitSHA          string
	ChangedFiles       []string
	Verification       []VerificationEntry
	Metrics            Metrics
}

func RewriteHarnessFields(content []byte, fields HarnessFields) ([]byte, Receipt, bool, error) {
	parsed, err := Parse(content)
	if err != nil {
		return nil, Receipt{}, false, err
	}

	updatedReceipt := parsed
	if !fields.Timestamp.IsZero() {
		updatedReceipt.Timestamp = fields.Timestamp.UTC()
	}
	updatedReceipt.Verdict = fields.Verdict
	updatedReceipt.CodexExitCode = fields.CodexExitCode
	updatedReceipt.VerificationStatus = fields.VerificationStatus
	updatedReceipt.CommitSHA = fields.CommitSHA
	updatedReceipt.ChangedFiles = compactStrings(fields.ChangedFiles)
	updatedReceipt.Verification = compactVerification(fields.Verification)
	updatedReceipt.Metrics = fields.Metrics

	frontmatter, err := yaml.Marshal(updatedReceipt)
	if err != nil {
		return nil, Receipt{}, false, fmt.Errorf("receipt: encode yaml: %w", err)
	}

	var updated bytes.Buffer
	updated.WriteString("---\n")
	updated.Write(frontmatter)
	updated.WriteString("---\n")
	updated.WriteString(rewriteHarnessBody(parsed.RawBody, updatedReceipt))

	reparsed, err := Parse(updated.Bytes())
	if err != nil {
		return nil, Receipt{}, false, err
	}
	return updated.Bytes(), reparsed, !bytes.Equal(content, updated.Bytes()), nil
}

func rewriteHarnessBody(body string, r Receipt) string {
	body = replaceSectionLines(body, "Changed Files", changedFilesSectionLines(r.ChangedFiles))
	body = replaceSectionLines(body, "Verification", verificationSectionLines(r.VerificationStatus, r.Verification))
	return body
}

func replaceSectionLines(body string, section string, replacement []string) string {
	lines := strings.Split(body, "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		if !isSecondLevelHeading(line) {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "## "))
		if start >= 0 {
			end = i
			break
		}
		if normalizeSection(name) == normalizeSection(section) {
			start = i + 1
		}
	}
	if start < 0 {
		return body
	}
	updated := make([]string, 0, len(lines)-end+start+len(replacement))
	updated = append(updated, lines[:start]...)
	updated = append(updated, replacement...)
	updated = append(updated, lines[end:]...)
	return strings.Join(updated, "\n")
}

func isSecondLevelHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ")
}

func changedFilesSectionLines(paths []string) []string {
	paths = compactStrings(paths)
	if len(paths) == 0 {
		return []string{"None.", ""}
	}
	lines := make([]string, 0, len(paths)+1)
	for _, path := range paths {
		lines = append(lines, "- `"+path+"`")
	}
	lines = append(lines, "")
	return lines
}

func verificationSectionLines(status string, entries []VerificationEntry) []string {
	entries = compactVerification(entries)
	if len(entries) == 0 {
		return []string{verificationStatusLine(status), ""}
	}
	lines := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		lines = append(lines, "- `"+entry.Command+"` "+verificationDetails(entry))
	}
	lines = append(lines, "")
	return lines
}

func verificationStatusLine(status string) string {
	switch strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(status)), "_")) {
	case "", "unknown":
		return "- Harness verification status: unknown."
	case "not_run":
		return "- Not run."
	case "passed":
		return "- Harness verification passed."
	case "failed":
		return "- Harness verification failed."
	default:
		return "- Harness verification status: " + strings.TrimSpace(status) + "."
	}
}
