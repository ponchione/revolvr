package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"revolvr/internal/ledger"
)

type runDiagnostics struct {
	Outcome                    string
	Message                    string
	CodexSeen                  bool
	CodexExitCode              int
	CodexTimedOut              bool
	CodexError                 string
	VerificationStatus         string
	FailedVerificationCommand  string
	FailedVerificationExitCode *int
	CommitSHA                  string
	CommitStatus               string
	CommitRefusal              string
	CommitMessage              string
	ReceiptVerdict             string
	ReceiptPath                string
	ChangedFiles               []string
	Warnings                   []diagnosticWarning
	commitSHAFromRun           string
}

type diagnosticWarning struct {
	WarningType string
	Message     string
	ReceiptPath string
}

func diagnosticsFromHistory(history ledger.RunWithEvents) runDiagnostics {
	diagnostics := runDiagnostics{commitSHAFromRun: strings.TrimSpace(history.Run.CommitSHA)}
	for _, event := range history.Events {
		switch event.Type {
		case ledger.EventRunCompleted, ledger.EventRunFailed:
			diagnostics.applyRunFinished(event)
		case ledger.EventCodexCompleted:
			diagnostics.applyCodexCompleted(event)
		case ledger.EventVerificationCompleted:
			diagnostics.applyVerificationCompleted(event)
		case ledger.EventCommitCreated:
			diagnostics.applyCommitCreated(event)
		case ledger.EventReceiptParsed, ledger.EventReceiptSynthesized:
			diagnostics.applyReceipt(event)
		case ledger.EventReceiptWarning:
			diagnostics.applyReceiptWarning(event)
		case ledger.EventChangedFilesCaptured:
			diagnostics.applyChangedFiles(event)
		}
	}
	if diagnostics.CommitSHA == "" && diagnostics.usefulWithoutRunCommit() {
		diagnostics.CommitSHA = diagnostics.commitSHAFromRun
	}
	return diagnostics
}

func (d runDiagnostics) Empty() bool {
	return !d.usefulWithoutRunCommit() && strings.TrimSpace(d.CommitSHA) == ""
}

func (d runDiagnostics) usefulWithoutRunCommit() bool {
	return strings.TrimSpace(d.Outcome) != "" ||
		strings.TrimSpace(d.Message) != "" ||
		d.CodexSeen ||
		strings.TrimSpace(d.VerificationStatus) != "" ||
		strings.TrimSpace(d.FailedVerificationCommand) != "" ||
		strings.TrimSpace(d.CommitStatus) != "" ||
		strings.TrimSpace(d.CommitRefusal) != "" ||
		strings.TrimSpace(d.CommitMessage) != "" ||
		strings.TrimSpace(d.ReceiptVerdict) != "" ||
		strings.TrimSpace(d.ReceiptPath) != "" ||
		len(d.ChangedFiles) > 0 ||
		len(d.Warnings) > 0
}

func (d *runDiagnostics) applyRunFinished(event ledger.Event) {
	var payload struct {
		Outcome       string `json:"outcome"`
		Message       string `json:"message"`
		CommitSHA     string `json:"commit_sha"`
		CommitStatus  string `json:"commit_status"`
		CommitRefusal string `json:"commit_refusal"`
		CommitMessage string `json:"commit_message"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	if value := strings.TrimSpace(payload.Outcome); value != "" {
		d.Outcome = value
	}
	if value := strings.TrimSpace(payload.Message); value != "" {
		d.Message = value
	}
	if value := strings.TrimSpace(payload.CommitSHA); value != "" {
		d.CommitSHA = value
	}
	if value := strings.TrimSpace(payload.CommitStatus); value != "" {
		d.CommitStatus = value
	}
	if value := strings.TrimSpace(payload.CommitRefusal); value != "" {
		d.CommitRefusal = value
	}
	if value := strings.TrimSpace(payload.CommitMessage); value != "" {
		d.CommitMessage = value
	}
}

func (d *runDiagnostics) applyCodexCompleted(event ledger.Event) {
	var payload struct {
		ExitCode int    `json:"exit_code"`
		TimedOut bool   `json:"timed_out"`
		Error    string `json:"error"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	d.CodexSeen = true
	d.CodexExitCode = payload.ExitCode
	d.CodexTimedOut = payload.TimedOut
	d.CodexError = strings.TrimSpace(payload.Error)
}

func (d *runDiagnostics) applyVerificationCompleted(event ledger.Event) {
	var payload struct {
		Status             string                       `json:"status"`
		FailedCommandIndex *int                         `json:"failed_command_index"`
		Commands           []verificationCommandPayload `json:"commands"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	if value := strings.TrimSpace(payload.Status); value != "" {
		d.VerificationStatus = value
	}
	if command, ok := failedVerificationCommand(payload); ok {
		d.FailedVerificationCommand = command.Command
		exitCode := command.ExitCode
		d.FailedVerificationExitCode = &exitCode
	}
}

type verificationCommandPayload struct {
	Index    int    `json:"index"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code"`
}

func failedVerificationCommand(payload struct {
	Status             string                       `json:"status"`
	FailedCommandIndex *int                         `json:"failed_command_index"`
	Commands           []verificationCommandPayload `json:"commands"`
}) (verificationCommandPayload, bool) {
	if payload.FailedCommandIndex != nil {
		for _, command := range payload.Commands {
			if command.Index == *payload.FailedCommandIndex && strings.TrimSpace(command.Command) != "" {
				return command, true
			}
		}
	}
	for _, command := range payload.Commands {
		status := strings.TrimSpace(command.Status)
		if strings.TrimSpace(command.Command) == "" {
			continue
		}
		if status == "failed" || (status != "" && !command.Passed) {
			return command, true
		}
	}
	return verificationCommandPayload{}, false
}

func (d *runDiagnostics) applyCommitCreated(event ledger.Event) {
	var payload struct {
		CommitSHA string `json:"commit_sha"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	if value := strings.TrimSpace(payload.CommitSHA); value != "" {
		d.CommitSHA = value
	}
}

func (d *runDiagnostics) applyReceipt(event ledger.Event) {
	var payload struct {
		ReceiptPath string `json:"receipt_path"`
		Verdict     string `json:"verdict"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	if value := strings.TrimSpace(payload.ReceiptPath); value != "" {
		d.ReceiptPath = value
	}
	if value := strings.TrimSpace(payload.Verdict); value != "" {
		d.ReceiptVerdict = value
	}
}

func (d *runDiagnostics) applyReceiptWarning(event ledger.Event) {
	var payload struct {
		WarningType string `json:"warning_type"`
		Message     string `json:"message"`
		ReceiptPath string `json:"receipt_path"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	warning := diagnosticWarning{
		WarningType: oneLine(payload.WarningType),
		Message:     oneLine(payload.Message),
		ReceiptPath: oneLine(payload.ReceiptPath),
	}
	if warning.WarningType == "" && warning.Message == "" {
		return
	}
	d.Warnings = append(d.Warnings, warning)
}

func (d *runDiagnostics) applyChangedFiles(event ledger.Event) {
	var payload struct {
		ChangedFiles []string `json:"changed_files"`
	}
	if !decodePayload(event, &payload) {
		return
	}
	if files := compactDiagnosticStrings(payload.ChangedFiles); len(files) > 0 {
		d.ChangedFiles = files
	}
}

func writeDiagnostics(out io.Writer, diagnostics runDiagnostics) error {
	if diagnostics.Empty() {
		return nil
	}
	if _, err := fmt.Fprint(out, "Diagnostics:\n"); err != nil {
		return err
	}
	for _, line := range diagnosticLines(diagnostics) {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func diagnosticLines(d runDiagnostics) []string {
	lines := []string{}
	if value := oneLine(d.Outcome); value != "" {
		lines = append(lines, "outcome: "+value)
	}
	if value := oneLine(d.Message); value != "" {
		lines = append(lines, "message: "+value)
	}
	if d.CodexSeen {
		parts := []string{
			fmt.Sprintf("exit_code=%d", d.CodexExitCode),
			fmt.Sprintf("timed_out=%t", d.CodexTimedOut),
		}
		if value := oneLine(d.CodexError); value != "" {
			parts = append(parts, "error="+value)
		}
		lines = append(lines, "codex: "+strings.Join(parts, ", "))
	}
	if value := oneLine(d.VerificationStatus); value != "" {
		lines = append(lines, "verification: "+value)
	}
	if value := oneLine(d.FailedVerificationCommand); value != "" {
		line := "failed verification: " + value
		if d.FailedVerificationExitCode != nil {
			line += fmt.Sprintf(" (exit_code=%d)", *d.FailedVerificationExitCode)
		}
		lines = append(lines, line)
	}
	if line := commitDiagnosticLine(d); line != "" {
		lines = append(lines, line)
	}
	if line := receiptDiagnosticLine(d); line != "" {
		lines = append(lines, line)
	}
	for _, warning := range d.Warnings {
		if line := warningDiagnosticLine(warning); line != "" {
			lines = append(lines, line)
		}
	}
	if len(d.ChangedFiles) > 0 {
		lines = append(lines, "changed files: "+strings.Join(d.ChangedFiles, ", "))
	}
	return lines
}

func commitDiagnosticLine(d runDiagnostics) string {
	if value := oneLine(d.CommitSHA); value != "" {
		return "commit: " + value
	}
	if value := oneLine(d.CommitRefusal); value != "" {
		line := "commit: refused " + value
		if message := oneLine(d.CommitMessage); message != "" {
			line += ": " + message
		}
		return line
	}
	status := oneLine(d.CommitStatus)
	message := oneLine(d.CommitMessage)
	if status == "" {
		if message == "" {
			return ""
		}
		return "commit: " + message
	}
	if status == "committed" {
		return ""
	}
	line := "commit: " + status
	if message != "" {
		line += ": " + message
	}
	return line
}

func receiptDiagnosticLine(d runDiagnostics) string {
	verdict := oneLine(d.ReceiptVerdict)
	path := oneLine(d.ReceiptPath)
	if verdict == "" && path == "" {
		return ""
	}
	if verdict == "" {
		verdict = "recorded"
	}
	if path == "" {
		return "receipt: " + verdict
	}
	return fmt.Sprintf("receipt: %s (%s)", verdict, path)
}

func warningDiagnosticLine(warning diagnosticWarning) string {
	warningType := oneLine(warning.WarningType)
	message := oneLine(warning.Message)
	if warningType == "" {
		if message == "" {
			return ""
		}
		return "warning: " + message
	}
	line := "warning: " + warningType
	if message != "" {
		line += ": " + message
	}
	if path := oneLine(warning.ReceiptPath); path != "" {
		line += " (" + path + ")"
	}
	return line
}

func decodePayload(event ledger.Event, target any) bool {
	if len(event.Payload) == 0 {
		return false
	}
	return json.Unmarshal(event.Payload, target) == nil
}

func compactDiagnosticStrings(values []string) []string {
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
