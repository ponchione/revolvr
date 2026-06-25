package receipt

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type FallbackInput struct {
	RunID              string
	PassID             string
	TaskID             string
	Task               string
	Verdict            Verdict
	Timestamp          time.Time
	CodexExitCode      int
	VerificationStatus string
	CommitSHA          string
	ChangedFiles       []string
	Verification       []VerificationEntry
	Metrics            Metrics
	FinalText          string
}

func FormatFallbackReceipt(input FallbackInput) (string, Receipt) {
	receipt := fallbackReceipt(input)
	body := fallbackBody(input, receipt)
	frontmatter, err := yaml.Marshal(receipt)
	if err != nil {
		panic(fmt.Sprintf("receipt: marshal fallback: %v", err))
	}
	content := fmt.Sprintf("---\n%s---\n%s", frontmatter, body)
	parsed, err := Parse([]byte(content))
	if err == nil {
		return content, parsed
	}
	receipt.RawBody = body
	receipt.ChangedFileClaims = ParseChangedFiles(body)
	receipt.VerificationClaims = ParseVerificationClaims(body)
	return content, receipt
}

func fallbackReceipt(input FallbackInput) Receipt {
	timestamp := input.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	} else {
		timestamp = timestamp.UTC()
	}
	verdict := input.Verdict
	if !validVerdict(verdict) {
		if input.CodexExitCode != 0 {
			verdict = VerdictCodexFailed
		} else {
			verdict = VerdictCompletedWithConcerns
		}
	}
	verificationStatus := strings.TrimSpace(input.VerificationStatus)
	if verificationStatus == "" {
		if input.CodexExitCode != 0 {
			verificationStatus = "not_run"
		} else {
			verificationStatus = "unknown"
		}
	}
	return Receipt{
		SchemaVersion:      SchemaVersion,
		RunID:              fallbackString(input.RunID, "unknown-run"),
		PassID:             fallbackString(input.PassID, "unknown-pass"),
		TaskID:             fallbackString(input.TaskID, "unknown-task"),
		Task:               fallbackString(input.Task, "Unknown task"),
		Verdict:            verdict,
		Timestamp:          timestamp,
		CodexExitCode:      input.CodexExitCode,
		VerificationStatus: verificationStatus,
		CommitSHA:          strings.TrimSpace(input.CommitSHA),
		ChangedFiles:       compactStrings(input.ChangedFiles),
		Verification:       compactVerification(input.Verification),
		Metrics:            input.Metrics,
	}
}

func fallbackBody(input FallbackInput, r Receipt) string {
	var out strings.Builder
	out.WriteString("## Summary\n")
	summary := strings.TrimSpace(input.FinalText)
	if summary == "" {
		summary = "No agent-authored receipt was produced."
	}
	out.WriteString(summary)
	out.WriteString("\n\n## Changed Files\n")
	if len(r.ChangedFiles) == 0 {
		out.WriteString("None.\n")
	} else {
		for _, path := range r.ChangedFiles {
			out.WriteString("- ")
			out.WriteString(path)
			out.WriteByte('\n')
		}
	}
	out.WriteString("\n## Verification\n")
	if len(r.Verification) == 0 {
		out.WriteString("- The harness did not receive agent-authored verification claims.\n")
	} else {
		for _, entry := range r.Verification {
			out.WriteString("- `")
			out.WriteString(entry.Command)
			out.WriteString("`")
			details := verificationDetails(entry)
			if details != "" {
				out.WriteByte(' ')
				out.WriteString(details)
			}
			out.WriteByte('\n')
		}
	}
	out.WriteString("\n## Concerns\n")
	out.WriteString("- Codex did not produce a receipt; this fallback was synthesized by the harness.\n")
	out.WriteString("\n## Next Steps\n")
	out.WriteString("- Inspect the pass output and decide whether follow-up work is needed.\n")
	return out.String()
}

func fallbackString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func compactVerification(entries []VerificationEntry) []VerificationEntry {
	out := make([]VerificationEntry, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		entry.Command = strings.TrimSpace(entry.Command)
		entry.Status = strings.TrimSpace(entry.Status)
		if entry.Command == "" {
			continue
		}
		if _, ok := seen[entry.Command]; ok {
			continue
		}
		seen[entry.Command] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func verificationDetails(entry VerificationEntry) string {
	parts := []string{}
	if entry.Status != "" {
		parts = append(parts, entry.Status)
	}
	parts = append(parts, fmt.Sprintf("exit %d", entry.ExitCode))
	return "(" + strings.Join(parts, ", ") + ")"
}
