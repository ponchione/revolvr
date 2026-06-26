package runonce

import (
	"context"
	"sort"
	"strings"

	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
)

const (
	receiptWarningChangedFiles = "changed_files_mismatch"
	receiptWarningVerification = "verification_mismatch"
	receiptWarningVerdict      = "verdict_mismatch"
	changedFilesWarningMessage = "receipt changed files differ from harness captured changed files"
	verificationWarningMessage = "receipt verification claims differ from harness verification results"
	verdictWarningMessage      = "receipt verdict differs from final run verdict"
)

type ReceiptWarning struct {
	WarningType string `json:"warning_type"`
	Message     string `json:"message"`
	ReceiptPath string `json:"receipt_path"`
	Claimed     any    `json:"claimed"`
	Observed    any    `json:"observed"`
}

type verificationFacts struct {
	Status   string                    `json:"status,omitempty"`
	Commands []verificationCommandFact `json:"commands,omitempty"`
}

type verificationCommandFact struct {
	Command  string `json:"command"`
	Status   string `json:"status,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

func recordReceiptWarnings(ctx context.Context, runs *ledger.Store, result *Result, parsed receipt.Receipt, finalVerdict receipt.Verdict, finalVerificationStatus string, observedVerification []receipt.VerificationEntry, observedChangedFiles []string) {
	if !receiptMatches(parsed, result.Run.ID, result.Task.ID) {
		return
	}
	warnings := buildReceiptWarnings(parsed, result.ReceiptRelPath, finalVerdict, finalVerificationStatus, observedVerification, observedChangedFiles)
	for _, warning := range warnings {
		result.ReceiptWarnings = append(result.ReceiptWarnings, warning)
		appendEvent(ctx, result, runs, result.Run.ID, ledger.EventReceiptWarning, warning)
	}
}

func buildReceiptWarnings(parsed receipt.Receipt, receiptPath string, finalVerdict receipt.Verdict, finalVerificationStatus string, observedVerification []receipt.VerificationEntry, observedChangedFiles []string) []ReceiptWarning {
	var warnings []ReceiptWarning

	if warning, ok := changedFilesWarning(parsed, receiptPath, observedChangedFiles); ok {
		warnings = append(warnings, warning)
	}
	if warning, ok := verificationWarning(parsed, receiptPath, finalVerificationStatus, observedVerification); ok {
		warnings = append(warnings, warning)
	}
	if warning, ok := verdictWarning(parsed, receiptPath, finalVerdict); ok {
		warnings = append(warnings, warning)
	}

	return warnings
}

func changedFilesWarning(parsed receipt.Receipt, receiptPath string, observedFiles []string) (ReceiptWarning, bool) {
	claimed := compactSortedStrings(append(append([]string(nil), parsed.ChangedFiles...), parsed.ChangedFileClaims...))
	if len(claimed) == 0 {
		return ReceiptWarning{}, false
	}
	observed := compactSortedStrings(observedFiles)
	if equalStrings(claimed, observed) {
		return ReceiptWarning{}, false
	}
	return ReceiptWarning{
		WarningType: receiptWarningChangedFiles,
		Message:     changedFilesWarningMessage,
		ReceiptPath: receiptPath,
		Claimed:     claimed,
		Observed:    observed,
	}, true
}

func verificationWarning(parsed receipt.Receipt, receiptPath string, finalVerificationStatus string, observedVerification []receipt.VerificationEntry) (ReceiptWarning, bool) {
	claimed := claimedVerificationFacts(parsed)
	if claimed.empty() {
		return ReceiptWarning{}, false
	}
	observed := observedVerificationFacts(finalVerificationStatus, observedVerification)
	if !verificationDisagrees(claimed, observed) {
		return ReceiptWarning{}, false
	}
	return ReceiptWarning{
		WarningType: receiptWarningVerification,
		Message:     verificationWarningMessage,
		ReceiptPath: receiptPath,
		Claimed:     claimed,
		Observed:    observed,
	}, true
}

func verdictWarning(parsed receipt.Receipt, receiptPath string, finalVerdict receipt.Verdict) (ReceiptWarning, bool) {
	if parsed.Verdict == "" || finalVerdict == "" || parsed.Verdict == finalVerdict {
		return ReceiptWarning{}, false
	}
	return ReceiptWarning{
		WarningType: receiptWarningVerdict,
		Message:     verdictWarningMessage,
		ReceiptPath: receiptPath,
		Claimed:     string(parsed.Verdict),
		Observed:    string(finalVerdict),
	}, true
}

func claimedVerificationFacts(parsed receipt.Receipt) verificationFacts {
	facts := verificationFacts{Status: explicitVerificationStatus(parsed.VerificationStatus)}
	for _, entry := range parsed.Verification {
		command := strings.TrimSpace(entry.Command)
		if command == "" {
			continue
		}
		exitCode := entry.ExitCode
		facts.Commands = append(facts.Commands, verificationCommandFact{
			Command:  command,
			Status:   normalizeVerificationStatus(entry.Status),
			ExitCode: &exitCode,
		})
	}
	for _, claim := range parsed.VerificationClaims {
		command := strings.TrimSpace(claim.Command)
		if command == "" {
			continue
		}
		fact := verificationCommandFact{
			Command: command,
			Status:  normalizeVerificationStatus(claim.Status),
		}
		if claim.HasExitCode {
			exitCode := claim.ExitCode
			fact.ExitCode = &exitCode
		}
		facts.Commands = append(facts.Commands, fact)
	}
	facts.Commands = compactVerificationFacts(facts.Commands)
	return facts
}

func observedVerificationFacts(status string, entries []receipt.VerificationEntry) verificationFacts {
	facts := verificationFacts{Status: normalizeVerificationStatus(status)}
	for _, entry := range entries {
		command := strings.TrimSpace(entry.Command)
		if command == "" {
			continue
		}
		exitCode := entry.ExitCode
		facts.Commands = append(facts.Commands, verificationCommandFact{
			Command:  command,
			Status:   normalizeVerificationStatus(entry.Status),
			ExitCode: &exitCode,
		})
	}
	facts.Commands = compactVerificationFacts(facts.Commands)
	return facts
}

func (f verificationFacts) empty() bool {
	return strings.TrimSpace(f.Status) == "" && len(f.Commands) == 0
}

func explicitVerificationStatus(status string) string {
	status = normalizeVerificationStatus(status)
	switch status {
	case "", "not_run", "unknown":
		return ""
	default:
		return status
	}
}

func normalizeVerificationStatus(status string) string {
	status = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(status)), "_"))
	switch {
	case status == "":
		return ""
	case strings.Contains(status, "not_run"):
		return "not_run"
	case strings.Contains(status, "skip"):
		return "skipped"
	case strings.Contains(status, "pass"):
		return "passed"
	case strings.Contains(status, "fail"):
		return "failed"
	case strings.Contains(status, "missing"):
		return "missing"
	default:
		return status
	}
}

func verificationDisagrees(claimed verificationFacts, observed verificationFacts) bool {
	if claimed.Status != "" && claimed.Status != observed.Status {
		return true
	}
	if len(claimed.Commands) == 0 {
		return false
	}

	observedByCommand := map[string]verificationCommandFact{}
	for _, fact := range observed.Commands {
		observedByCommand[fact.Command] = fact
	}
	claimedCommands := map[string]struct{}{}
	for _, claim := range claimed.Commands {
		claimedCommands[claim.Command] = struct{}{}
		observedFact, ok := observedByCommand[claim.Command]
		if !ok {
			return true
		}
		if claim.Status != "" && claim.Status != observedFact.Status {
			return true
		}
		if claim.ExitCode != nil {
			if observedFact.ExitCode == nil || *claim.ExitCode != *observedFact.ExitCode {
				return true
			}
		}
	}
	return len(claimedCommands) != len(observedByCommand)
}

func compactVerificationFacts(values []verificationCommandFact) []verificationCommandFact {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Command != values[j].Command {
			return values[i].Command < values[j].Command
		}
		if values[i].Status != values[j].Status {
			return values[i].Status < values[j].Status
		}
		return exitCodeSortValue(values[i].ExitCode) < exitCodeSortValue(values[j].ExitCode)
	})

	out := make([]verificationCommandFact, 0, len(values))
	var last verificationCommandFact
	for _, value := range values {
		if value.Command == "" {
			continue
		}
		if len(out) > 0 && sameVerificationFact(last, value) {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

func sameVerificationFact(a verificationCommandFact, b verificationCommandFact) bool {
	if a.Command != b.Command || a.Status != b.Status {
		return false
	}
	if a.ExitCode == nil || b.ExitCode == nil {
		return a.ExitCode == nil && b.ExitCode == nil
	}
	return *a.ExitCode == *b.ExitCode
}

func exitCodeSortValue(value *int) int {
	if value == nil {
		return -1 << 30
	}
	return *value
}

func compactSortedStrings(values []string) []string {
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
	sort.Strings(out)
	return out
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
