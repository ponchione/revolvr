package autonomouscycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"revolvr/internal/artifactretention"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
	"revolvr/internal/receipt"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func finalizeWorkerReceipt(ctx context.Context, n normalizedConfig, task taskfile.Task, route autonomouspolicy.Route, result *Result, verdict receipt.Verdict, verificationStatus, commitSHA string, timestamp time.Time, cause error) {
	relPath := result.Worker.Artifacts.Receipt.Path
	result.Worker.Receipt.Path = relPath
	absPath, err := pathguard.Resolve(n.root, relPath)
	if err != nil {
		result.Worker.Receipt.ParseError = err.Error()
		return
	}

	var parsed receipt.Receipt
	var original receipt.Receipt
	validOriginal := false
	content, readErr := os.ReadFile(absPath)
	if readErr == nil {
		if stdoutPath := result.Worker.Artifacts.CodexStdout.Path; stdoutPath != "" {
			if _, resolveErr := pathguard.Resolve(n.root, stdoutPath); resolveErr == nil {
				if jsonl, _, jsonlErr := artifactretention.ReadLogical(ctx, n.root, stdoutPath, int64(n.CodexStdoutCap)); jsonlErr == nil {
					if updated, reparsed, changed, rewriteErr := receipt.RewriteMetricsFromCodexJSONL(content, jsonl); rewriteErr == nil {
						content = updated
						parsed = reparsed
						if changed {
							_ = os.WriteFile(absPath, updated, 0o644)
						}
					}
				}
			}
		}
		if parsed.RunID == "" {
			parsed, err = receipt.Parse(content)
		}
		if err == nil && workerReceiptMatches(parsed, result.Worker.RunID, n.TaskID) {
			original = parsed
			validOriginal = true
		} else {
			if err == nil {
				err = errors.New("receipt identifiers do not match the worker run and task")
			}
			result.Worker.Receipt.ParseError = err.Error()
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		result.Worker.Receipt.ParseError = readErr.Error()
	} else {
		result.Worker.Receipt.ParseError = "worker receipt is missing"
	}

	entries := receiptVerificationEntries(result.Worker.Verification.Result)
	metrics := parsed.Metrics
	if result.Worker.Codex.UsageFound {
		metrics = result.Worker.Codex.Usage
	}
	if validOriginal {
		updated, reparsed, _, rewriteErr := receipt.RewriteHarnessFields(content, receipt.HarnessFields{
			Timestamp:          timestamp,
			Verdict:            verdict,
			CodexExitCode:      result.Worker.Codex.ExitCode,
			VerificationStatus: verificationStatus,
			CommitSHA:          commitSHA,
			ChangedFiles:       result.Source.ChangedFiles,
			Verification:       entries,
			Metrics:            metrics,
		})
		if rewriteErr == nil && workerReceiptMatches(reparsed, result.Worker.RunID, n.TaskID) {
			if writeErr := os.WriteFile(absPath, updated, 0o644); writeErr == nil {
				parsed = reparsed
				result.Worker.Receipt.Warnings = receiptWarnings(original, verdict, verificationStatus, entries, result.Source.ChangedFiles)
				appendWorkerEvent(ctx, n, result, ledger.EventReceiptParsed, map[string]any{"receipt_path": relPath, "verdict": parsed.Verdict})
			} else {
				result.Worker.Receipt.ParseError = writeErr.Error()
				validOriginal = false
			}
		} else {
			if rewriteErr != nil {
				result.Worker.Receipt.ParseError = rewriteErr.Error()
			}
			validOriginal = false
		}
	}
	if !validOriginal {
		finalText := strings.TrimSpace(result.Worker.Codex.FinalMessage)
		if cause != nil {
			finalText = cause.Error()
		}
		fallback, fallbackReceipt := receipt.FormatFallbackReceipt(receipt.FallbackInput{
			RunID:              result.Worker.RunID,
			PassID:             result.Worker.RunID,
			TaskID:             n.TaskID,
			Task:               task.ContextBody,
			Verdict:            verdict,
			Timestamp:          timestamp,
			CodexExitCode:      result.Worker.Codex.ExitCode,
			VerificationStatus: verificationStatus,
			CommitSHA:          commitSHA,
			ChangedFiles:       result.Source.ChangedFiles,
			Verification:       entries,
			Metrics:            result.Worker.Codex.Usage,
			FinalText:          finalText,
		})
		writeErr := os.MkdirAll(filepath.Dir(absPath), 0o755)
		if writeErr == nil {
			writeErr = os.WriteFile(absPath, []byte(fallback), 0o644)
		}
		if writeErr != nil {
			result.Worker.Receipt.ParseError = strings.TrimSpace(strings.Join([]string{result.Worker.Receipt.ParseError, writeErr.Error()}, "; "))
			return
		}
		parsed = fallbackReceipt
		result.Worker.Receipt.Synthesized = true
		appendWorkerEvent(ctx, n, result, ledger.EventReceiptSynthesized, map[string]any{
			"receipt_path": relPath,
			"verdict":      parsed.Verdict,
			"reason":       result.Worker.Receipt.ParseError,
		})
	}
	result.Worker.Receipt.Receipt = parsed
	if artifact, refErr := referenceArtifact(n.root, relPath); refErr == nil {
		result.Worker.Artifacts.Receipt = artifact
	}
	for _, warning := range result.Worker.Receipt.Warnings {
		appendWorkerEvent(ctx, n, result, ledger.EventReceiptWarning, warning)
	}
}

func workerReceiptMatches(value receipt.Receipt, runID, taskID string) bool {
	return value.RunID == runID && value.PassID == runID && value.TaskID == taskID
}

func receiptVerificationEntries(result verification.Result) []receipt.VerificationEntry {
	entries := make([]receipt.VerificationEntry, 0, len(result.Commands))
	for _, command := range result.Commands {
		entries = append(entries, receipt.VerificationEntry{
			Command:  command.Command,
			ExitCode: command.ExitCode,
			Status:   string(command.Status),
		})
	}
	return entries
}

func receiptWarnings(parsed receipt.Receipt, verdict receipt.Verdict, verificationStatus string, verificationEntries []receipt.VerificationEntry, changedFiles []string) []ReceiptWarning {
	warnings := make([]ReceiptWarning, 0, 3)
	claimedFiles := append(append([]string(nil), parsed.ChangedFiles...), parsed.ChangedFileClaims...)
	claimedFiles = compactSorted(claimedFiles)
	observedFiles := compactSorted(changedFiles)
	if len(claimedFiles) > 0 && !equalStrings(claimedFiles, observedFiles) {
		warnings = append(warnings, ReceiptWarning{
			Kind:     "changed_files_mismatch",
			Message:  "receipt changed-file claims differ from harness-observed source changes",
			Claimed:  claimedFiles,
			Observed: observedFiles,
		})
	}
	claimedVerification := claimedVerificationFacts(parsed)
	observedVerification := observedVerificationFacts(verificationStatus, verificationEntries)
	if len(claimedVerification) > 0 && !stringsSubset(claimedVerification, observedVerification) {
		warnings = append(warnings, ReceiptWarning{
			Kind:     "verification_mismatch",
			Message:  "receipt verification claims differ from harness verification",
			Claimed:  claimedVerification,
			Observed: observedVerification,
		})
	}
	if parsed.Verdict != "" && parsed.Verdict != verdict {
		warnings = append(warnings, ReceiptWarning{
			Kind:     "verdict_mismatch",
			Message:  "receipt verdict differs from the harness outcome",
			Claimed:  []string{string(parsed.Verdict)},
			Observed: []string{string(verdict)},
		})
	}
	return warnings
}

func claimedVerificationFacts(parsed receipt.Receipt) []string {
	values := make([]string, 0, len(parsed.Verification)+len(parsed.VerificationClaims)+1)
	if status := normalizeVerificationStatus(parsed.VerificationStatus); status != "" && status != "unknown" && status != "not_run" {
		values = append(values, "status="+status)
	}
	for _, entry := range parsed.Verification {
		values = append(values, verificationFact(entry.Command, entry.Status, entry.ExitCode, true))
	}
	for _, claim := range parsed.VerificationClaims {
		values = append(values, verificationFact(claim.Command, claim.Status, claim.ExitCode, claim.HasExitCode))
	}
	return compactSorted(values)
}

func observedVerificationFacts(status string, entries []receipt.VerificationEntry) []string {
	values := make([]string, 0, len(entries)+1)
	if normalized := normalizeVerificationStatus(status); normalized != "" {
		values = append(values, "status="+normalized)
	}
	for _, entry := range entries {
		values = append(values, verificationFact(entry.Command, entry.Status, entry.ExitCode, true))
	}
	return compactSorted(values)
}

func verificationFact(command, status string, exitCode int, hasExitCode bool) string {
	parts := []string{"command=" + strings.TrimSpace(command)}
	if normalized := normalizeVerificationStatus(status); normalized != "" {
		parts = append(parts, "status="+normalized)
	}
	if hasExitCode {
		parts = append(parts, "exit="+strconv.Itoa(exitCode))
	}
	return strings.Join(parts, "|")
}

func normalizeVerificationStatus(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), "_"))
	switch {
	case strings.Contains(value, "not_run"):
		return "not_run"
	case strings.Contains(value, "missing"):
		return "missing"
	case strings.Contains(value, "pass"):
		return "passed"
	case strings.Contains(value, "fail"):
		return "failed"
	default:
		return value
	}
}

func compactSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func stringsSubset(subset, set []string) bool {
	known := make(map[string]struct{}, len(set))
	for _, value := range set {
		known[value] = struct{}{}
	}
	for _, value := range subset {
		if _, ok := known[value]; !ok {
			return false
		}
	}
	return true
}
