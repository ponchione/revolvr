package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"revolvr/internal/ledger"
)

const maxTimelineListItems = 3

// RunTimelineRow is a single app-level run timeline projection row.
type RunTimelineRow struct {
	Timestamp time.Time
	Phase     string
	Status    string
	Detail    string
}

// RunTimeline projects ledger events plus run-row fallback fields into concise
// human-readable rows for later CLI and TUI rendering.
func RunTimeline(history ledger.RunWithEvents) []RunTimelineRow {
	rows := make([]RunTimelineRow, 0, len(history.Events)+2)
	seenRunStarted := false
	seenTerminal := false
	seenVerificationCompleted := false
	seenCommitCreated := false

	for _, event := range history.Events {
		switch event.Type {
		case ledger.EventRunStarted:
			seenRunStarted = true
		case ledger.EventRunCompleted, ledger.EventRunFailed:
			seenTerminal = true
		case ledger.EventVerificationCompleted:
			seenVerificationCompleted = true
		case ledger.EventCommitCreated:
			seenCommitCreated = true
		}
	}

	if !seenRunStarted && !history.Run.StartedAt.IsZero() {
		rows = append(rows, fallbackRunStartedRow(history.Run))
	}

	for _, event := range history.Events {
		row, ok := timelineRowFromEvent(event)
		if ok {
			rows = append(rows, row)
		}
	}

	if !seenTerminal {
		timestamp := runCompletedTimestamp(history.Run)
		if !seenVerificationCompleted && strings.TrimSpace(history.Run.VerificationStatus) != "" {
			rows = append(rows, RunTimelineRow{
				Timestamp: timestamp,
				Phase:     "verification",
				Status:    timelineOneLine(history.Run.VerificationStatus),
				Detail:    "from run record",
			})
		}
		if !seenCommitCreated && strings.TrimSpace(history.Run.CommitSHA) != "" {
			rows = append(rows, RunTimelineRow{
				Timestamp: timestamp,
				Phase:     "commit",
				Status:    "created",
				Detail:    "commit " + timelineOneLine(history.Run.CommitSHA),
			})
		}
		if terminalStatus := terminalStatusFromRun(history.Run); terminalStatus != "" {
			rows = append(rows, RunTimelineRow{
				Timestamp: timestamp,
				Phase:     "run",
				Status:    terminalStatus,
				Detail:    runRecordTerminalDetail(history.Run),
			})
		}
	}

	return rows
}

func timelineRowFromEvent(event ledger.Event) (RunTimelineRow, bool) {
	row := RunTimelineRow{Timestamp: event.CreatedAt}
	switch event.Type {
	case ledger.EventRunStarted:
		row.Phase = "run"
		row.Status = "started"
		row.Detail = runStartedDetail(event)
	case ledger.EventTaskSelected:
		row.Phase = "task"
		row.Status = "selected"
		row.Detail = taskSelectedDetail(event)
	case ledger.EventContextBuilt:
		row.Phase = "context"
		row.Status = "built"
		row.Detail = contextBuiltDetail(event)
	case ledger.EventCodexStarted:
		row.Phase = "codex"
		row.Status = "started"
		row.Detail = codexStartedDetail(event)
	case ledger.EventCodexJSONEvent:
		row.Phase = "codex"
		row.Status, row.Detail = codexProgressStatusAndDetail(event)
	case ledger.EventCodexCompleted:
		row.Phase = "codex"
		row.Status, row.Detail = codexCompletedStatusAndDetail(event)
	case ledger.EventChangedFilesCaptured:
		row.Phase = "changes"
		row.Status, row.Detail = changedFilesStatusAndDetail(event)
	case ledger.EventReceiptParsed:
		row.Phase = "receipt"
		row.Status = "parsed"
		row.Detail = receiptDetail(event, "receipt parsed")
	case ledger.EventReceiptSynthesized:
		row.Phase = "receipt"
		row.Status = "synthesized"
		row.Detail = receiptDetail(event, "receipt synthesized")
	case ledger.EventReceiptWarning:
		row.Phase = "receipt"
		row.Status = "warning"
		row.Detail = receiptWarningDetail(event)
	case ledger.EventVerificationStarted:
		row.Phase = "verification"
		row.Status = "started"
		row.Detail = verificationStartedDetail(event)
	case ledger.EventVerificationCompleted:
		row.Phase = "verification"
		row.Status, row.Detail = verificationCompletedStatusAndDetail(event)
	case ledger.EventCommitStarted:
		row.Phase = "commit"
		row.Status = "started"
		row.Detail = commitStartedDetail(event)
	case ledger.EventCommitCreated:
		row.Phase = "commit"
		row.Status = "created"
		row.Detail = commitCreatedDetail(event)
	case ledger.EventRunCompleted:
		row.Phase = "run"
		row.Status = "completed"
		row.Detail = runFinishedDetail(event, "run completed")
	case ledger.EventRunFailed:
		row.Phase = "run"
		row.Status = "failed"
		row.Detail = runFinishedDetail(event, "run failed")
	default:
		return RunTimelineRow{}, false
	}
	return row, true
}

func fallbackRunStartedRow(run ledger.Run) RunTimelineRow {
	return RunTimelineRow{
		Timestamp: run.StartedAt,
		Phase:     "run",
		Status:    "started",
		Detail:    runIdentityDetail(run.ID, run.TaskID, "run started"),
	}
}

func runStartedDetail(event ledger.Event) string {
	var payload struct {
		RunID  string `json:"run_id"`
		TaskID string `json:"task_id"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "run started"
	}
	return runIdentityDetail(payload.RunID, payload.TaskID, "run started")
}

func taskSelectedDetail(event ledger.Event) string {
	var payload struct {
		TaskID  string `json:"task_id"`
		Summary string `json:"summary"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "task selected"
	}
	taskID := timelineOneLine(payload.TaskID)
	summary := timelineOneLine(payload.Summary)
	switch {
	case taskID != "" && summary != "":
		return "task " + taskID + ": " + summary
	case taskID != "":
		return "task " + taskID
	case summary != "":
		return summary
	default:
		return "task selected"
	}
}

func contextBuiltDetail(event ledger.Event) string {
	var payload struct {
		ContextPayloadPath  string `json:"context_payload_path"`
		ContextManifestPath string `json:"context_manifest_path"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "context built"
	}
	if value := timelineOneLine(payload.ContextPayloadPath); value != "" {
		return value
	}
	if value := timelineOneLine(payload.ContextManifestPath); value != "" {
		return value
	}
	return "context built"
}

func codexStartedDetail(event ledger.Event) string {
	var payload struct {
		Executable string `json:"executable"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "codex started"
	}
	if value := timelineOneLine(payload.Executable); value != "" {
		return value
	}
	return "codex started"
}

func codexProgressStatusAndDetail(event ledger.Event) (string, string) {
	var payload struct {
		Type     string `json:"type"`
		ItemType string `json:"item_type"`
		Message  string `json:"message"`
		Error    string `json:"error"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "progress", "codex progress"
	}
	if value := timelineOneLine(payload.Error); value != "" {
		return "error", "error: " + value
	}
	if value := timelineOneLine(payload.Message); value != "" {
		return "progress", "message: " + value
	}
	eventType := timelineOneLine(payload.Type)
	itemType := timelineOneLine(payload.ItemType)
	switch {
	case eventType != "" && itemType != "":
		return "progress", eventType + " " + itemType
	case eventType != "":
		return "progress", eventType
	case itemType != "":
		return "progress", itemType
	default:
		return "progress", "codex progress"
	}
}

func codexCompletedStatusAndDetail(event ledger.Event) (string, string) {
	var payload struct {
		ExitCode        int      `json:"exit_code"`
		TimedOut        bool     `json:"timed_out"`
		Error           string   `json:"error"`
		JSONEvents      int      `json:"json_events"`
		JSONParseErrors []string `json:"json_parse_errors"`
		ParseError      string   `json:"parse_error"`
		ArtifactError   string   `json:"artifact_error"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "completed", "codex completed"
	}
	status := "completed"
	if payload.ExitCode != 0 || payload.TimedOut || strings.TrimSpace(payload.Error) != "" {
		status = "failed"
	}
	parts := []string{fmt.Sprintf("exit_code=%d", payload.ExitCode)}
	if payload.TimedOut {
		parts = append(parts, "timed_out=true")
	}
	if value := timelineOneLine(payload.Error); value != "" {
		parts = append(parts, "error="+value)
	}
	if payload.JSONEvents > 0 {
		parts = append(parts, fmt.Sprintf("json_events=%d", payload.JSONEvents))
	}
	if len(payload.JSONParseErrors) > 0 {
		parts = append(parts, fmt.Sprintf("json_parse_errors=%d", len(payload.JSONParseErrors)))
	}
	if value := timelineOneLine(payload.ParseError); value != "" {
		parts = append(parts, "parse_error="+value)
	}
	if value := timelineOneLine(payload.ArtifactError); value != "" {
		parts = append(parts, "artifact_error="+value)
	}
	return status, strings.Join(parts, ", ")
}

func changedFilesStatusAndDetail(event ledger.Event) (string, string) {
	var payload struct {
		PreRunDirtyFiles []string `json:"pre_run_dirty_files"`
		ChangedFiles     []string `json:"changed_files"`
		CaptureError     string   `json:"capture_error"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "captured", "changed files captured"
	}
	parts := []string{fileListDetail("changed file", "changed files", payload.ChangedFiles)}
	if dirty := compactTimelineStrings(payload.PreRunDirtyFiles); len(dirty) > 0 {
		parts = append(parts, fileListDetail("pre-run dirty file", "pre-run dirty files", dirty))
	}
	if value := timelineOneLine(payload.CaptureError); value != "" {
		parts = append(parts, "capture_error: "+value)
		return "failed", strings.Join(parts, "; ")
	}
	return "captured", strings.Join(parts, "; ")
}

func receiptDetail(event ledger.Event, fallback string) string {
	var payload struct {
		ReceiptPath string `json:"receipt_path"`
		Verdict     string `json:"verdict"`
		Reason      string `json:"reason"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return fallback
	}
	verdict := timelineOneLine(payload.Verdict)
	path := timelineOneLine(payload.ReceiptPath)
	var detail string
	switch {
	case verdict != "" && path != "":
		detail = verdict + " (" + path + ")"
	case verdict != "":
		detail = verdict
	case path != "":
		detail = path
	default:
		detail = fallback
	}
	if reason := timelineOneLine(payload.Reason); reason != "" {
		detail += "; reason: " + reason
	}
	return detail
}

func receiptWarningDetail(event ledger.Event) string {
	var payload struct {
		WarningType string `json:"warning_type"`
		Message     string `json:"message"`
		ReceiptPath string `json:"receipt_path"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "receipt warning"
	}
	warningType := timelineOneLine(payload.WarningType)
	message := timelineOneLine(payload.Message)
	path := timelineOneLine(payload.ReceiptPath)
	var detail string
	switch {
	case warningType != "" && message != "":
		detail = warningType + ": " + message
	case warningType != "":
		detail = warningType
	case message != "":
		detail = message
	default:
		detail = "receipt warning"
	}
	if path != "" {
		detail += " (" + path + ")"
	}
	return detail
}

func verificationStartedDetail(event ledger.Event) string {
	var payload struct {
		CommandCount int                           `json:"command_count"`
		Commands     []timelineVerificationCommand `json:"commands"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "verification started"
	}
	count := payload.CommandCount
	if count == 0 && len(payload.Commands) > 0 {
		count = len(payload.Commands)
	}
	return commandCountDetail(count, "verification started")
}

type timelineVerificationCommand struct {
	Index    int    `json:"index"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	Passed   bool   `json:"passed"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
	Error    string `json:"error"`
}

func verificationCompletedStatusAndDetail(event ledger.Event) (string, string) {
	var payload struct {
		Status             string                        `json:"status"`
		Passed             bool                          `json:"passed"`
		MissingCommands    bool                          `json:"missing_commands"`
		Message            string                        `json:"message"`
		FailedCommandIndex *int                          `json:"failed_command_index"`
		Commands           []timelineVerificationCommand `json:"commands"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "completed", "verification completed"
	}

	status := timelineOneLine(payload.Status)
	if status == "" {
		switch {
		case payload.MissingCommands:
			status = "missing"
		case payload.Passed:
			status = "passed"
		default:
			status = "completed"
		}
	}

	if payload.MissingCommands {
		if message := timelineOneLine(payload.Message); message != "" {
			return status, message
		}
		return status, "no verification commands configured"
	}
	if command, ok := failedTimelineVerificationCommand(payload.FailedCommandIndex, payload.Commands); ok {
		return status, failedVerificationDetail(command)
	}
	if message := timelineOneLine(payload.Message); message != "" && status != "passed" {
		return status, message
	}
	return status, commandCountDetail(len(payload.Commands), "verification completed")
}

func failedTimelineVerificationCommand(index *int, commands []timelineVerificationCommand) (timelineVerificationCommand, bool) {
	if index != nil && *index >= 0 {
		for _, command := range commands {
			if command.Index == *index && timelineOneLine(command.Command) != "" {
				return command, true
			}
		}
	}
	for _, command := range commands {
		if timelineOneLine(command.Command) == "" {
			continue
		}
		if command.Status == "failed" || (!command.Passed && command.Status != "") {
			return command, true
		}
	}
	return timelineVerificationCommand{}, false
}

func failedVerificationDetail(command timelineVerificationCommand) string {
	detail := fmt.Sprintf("failed command %d: %s", command.Index, timelineOneLine(command.Command))
	parts := []string{}
	if command.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", command.ExitCode))
	}
	if command.TimedOut {
		parts = append(parts, "timed_out=true")
	}
	if value := timelineOneLine(command.Error); value != "" {
		parts = append(parts, "error="+value)
	}
	if len(parts) > 0 {
		detail += " (" + strings.Join(parts, ", ") + ")"
	}
	return detail
}

func commitStartedDetail(event ledger.Event) string {
	var payload struct {
		ChangedFiles []string `json:"changed_files"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "commit started"
	}
	return fileListDetail("changed file", "changed files", payload.ChangedFiles)
}

func commitCreatedDetail(event ledger.Event) string {
	var payload struct {
		CommitSHA    string   `json:"commit_sha"`
		ChangedFiles []string `json:"changed_files"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return "commit created"
	}
	if sha := timelineOneLine(payload.CommitSHA); sha != "" {
		return "commit " + sha
	}
	if len(compactTimelineStrings(payload.ChangedFiles)) > 0 {
		return fileListDetail("changed file", "changed files", payload.ChangedFiles)
	}
	return "commit created"
}

func runFinishedDetail(event ledger.Event, fallback string) string {
	var payload struct {
		Outcome            string `json:"outcome"`
		Message            string `json:"message"`
		CodexExitCode      int    `json:"codex_exit_code"`
		VerificationStatus string `json:"verification_status"`
		CommitSHA          string `json:"commit_sha"`
		CommitStatus       string `json:"commit_status"`
		CommitRefusal      string `json:"commit_refusal"`
		CommitMessage      string `json:"commit_message"`
	}
	if !decodeTimelinePayload(event, &payload) {
		return fallback
	}
	parts := []string{}
	outcome := timelineOneLine(payload.Outcome)
	message := timelineOneLine(payload.Message)
	switch {
	case outcome != "" && message != "":
		parts = append(parts, "outcome="+outcome+": "+message)
	case outcome != "":
		parts = append(parts, "outcome="+outcome)
	case message != "":
		parts = append(parts, message)
	}
	if payload.CodexExitCode != 0 {
		parts = append(parts, fmt.Sprintf("codex_exit_code=%d", payload.CodexExitCode))
	}
	if value := timelineOneLine(payload.VerificationStatus); value != "" {
		parts = append(parts, "verification="+value)
	}
	if value := timelineOneLine(payload.CommitSHA); value != "" {
		parts = append(parts, "commit="+value)
	} else if value := commitStatusDetail(payload.CommitStatus, payload.CommitRefusal, payload.CommitMessage); value != "" {
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, "; ")
}

func commitStatusDetail(status string, refusal string, message string) string {
	status = timelineOneLine(status)
	refusal = timelineOneLine(refusal)
	message = timelineOneLine(message)
	switch {
	case status == "" && refusal == "" && message == "":
		return ""
	case refusal != "" && message != "":
		if status == "" {
			status = "refused"
		}
		return "commit=" + status + " " + refusal + ": " + message
	case refusal != "":
		if status == "" {
			status = "refused"
		}
		return "commit=" + status + " " + refusal
	case status != "" && message != "":
		return "commit=" + status + ": " + message
	case status != "":
		return "commit=" + status
	default:
		return "commit=" + message
	}
}

func runIdentityDetail(runID string, taskID string, fallback string) string {
	parts := []string{}
	if value := timelineOneLine(runID); value != "" {
		parts = append(parts, "run "+value)
	}
	if value := timelineOneLine(taskID); value != "" {
		parts = append(parts, "task "+value)
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, ", ")
}

func runCompletedTimestamp(run ledger.Run) time.Time {
	if run.CompletedAt != nil {
		return *run.CompletedAt
	}
	return time.Time{}
}

func terminalStatusFromRun(run ledger.Run) string {
	switch strings.TrimSpace(run.Status) {
	case ledger.StatusCompleted:
		return "completed"
	case ledger.StatusFailed:
		return "failed"
	default:
		if run.CompletedAt != nil && strings.TrimSpace(run.Status) != "" {
			return timelineOneLine(run.Status)
		}
		return ""
	}
}

func runRecordTerminalDetail(run ledger.Run) string {
	parts := []string{}
	if value := timelineOneLine(run.Summary); value != "" {
		parts = append(parts, value)
	}
	if value := timelineOneLine(run.VerificationStatus); value != "" {
		parts = append(parts, "verification="+value)
	}
	if run.CodexExitCode != nil {
		parts = append(parts, fmt.Sprintf("codex_exit_code=%d", *run.CodexExitCode))
	}
	if value := timelineOneLine(run.CommitSHA); value != "" {
		parts = append(parts, "commit="+value)
	}
	if len(parts) == 0 {
		return "run " + terminalStatusFromRun(run)
	}
	return strings.Join(parts, "; ")
}

func commandCountDetail(count int, fallback string) string {
	switch count {
	case 0:
		return fallback
	case 1:
		return "1 command"
	default:
		return fmt.Sprintf("%d commands", count)
	}
}

func fileListDetail(singular string, plural string, values []string) string {
	files := compactTimelineStrings(values)
	switch len(files) {
	case 0:
		return "no " + plural
	case 1:
		return "1 " + singular + ": " + files[0]
	default:
		return fmt.Sprintf("%d %s: %s", len(files), plural, timelineList(files))
	}
}

func timelineList(values []string) string {
	if len(values) <= maxTimelineListItems {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:maxTimelineListItems], ", ") + fmt.Sprintf(", +%d more", len(values)-maxTimelineListItems)
}

func compactTimelineStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = timelineOneLine(value)
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

func timelineOneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func decodeTimelinePayload(event ledger.Event, target any) bool {
	if len(event.Payload) == 0 {
		return false
	}
	return json.Unmarshal(event.Payload, target) == nil
}
