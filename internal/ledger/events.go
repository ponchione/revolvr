package ledger

import (
	"encoding/json"
	"strings"
)

type EventType string

const (
	EventRunStarted                EventType = "run_started"
	EventTaskSelected              EventType = "task_selected"
	EventRunArtifacts              EventType = "run_artifacts"
	EventContextBuilt              EventType = "context_built"
	EventCodexStarted              EventType = "codex_started"
	EventCodexJSONEvent            EventType = "codex_json_event"
	EventCodexCompleted            EventType = "codex_completed"
	EventSupervisorPrepared        EventType = "supervisor_prepared"
	EventSupervisorValidated       EventType = "supervisor_decision_validated"
	EventSupervisorRejected        EventType = "supervisor_decision_rejected"
	EventSupervisorMutation        EventType = "supervisor_source_mutation_detected"
	EventChangedFilesCaptured      EventType = "changed_files_captured"
	EventReceiptParsed             EventType = "receipt_parsed"
	EventReceiptSynthesized        EventType = "receipt_synthesized"
	EventReceiptWarning            EventType = "receipt_warning"
	EventVerificationStarted       EventType = "verification_started"
	EventVerificationTierStarted   EventType = "verification_tier_started"
	EventVerificationTierCompleted EventType = "verification_tier_completed"
	EventVerificationRerun         EventType = "verification_rerun_authorized"
	EventVerificationCompleted     EventType = "verification_completed"
	EventCommitStarted             EventType = "commit_started"
	EventCommitCreated             EventType = "commit_created"
	EventOptionalRoleDisposition   EventType = "optional_role_disposition"
	EventFinalizationPrepared      EventType = "finalization_prepared"
	EventFinalizationMaterialized  EventType = "finalization_capsule_materialized"
	EventFinalizationStateTerminal EventType = "finalization_state_terminal"
	EventFinalizationCompleted     EventType = "finalization_terminal_completed"
	EventArchivePrepared           EventType = "archive_prepared"
	EventArchiveFilesPublished     EventType = "archive_files_published"
	EventArchiveActiveRemoved      EventType = "archive_active_task_removed"
	EventArchiveCommitReconciled   EventType = "archive_commit_reconciled"
	EventArchiveCompleted          EventType = "archive_completed"
	EventArchiveReopened           EventType = "archive_reopened"
	EventChildProposalAdmitted     EventType = "child_proposal_admitted"
	EventChildrenPublished         EventType = "children_published"
	EventChildPublicationCompleted EventType = "child_publication_completed"
	EventTaskRunAdmitted           EventType = "autonomous_task_run_admitted"
	EventTaskRunCycleStarted       EventType = "autonomous_task_run_cycle_started"
	EventTaskRunCycleCompleted     EventType = "autonomous_task_run_cycle_completed"
	EventTaskRunRestarted          EventType = "autonomous_task_run_restarted"
	EventTaskRunStopped            EventType = "autonomous_task_run_stopped"
	EventQueueAdmitted             EventType = "autonomous_queue_admitted"
	EventQueueSelection            EventType = "autonomous_queue_selection"
	EventQueueTaskStopped          EventType = "autonomous_queue_task_stopped"
	EventQueueDaemonWake           EventType = "autonomous_queue_daemon_wake"
	EventQueueStopped              EventType = "autonomous_queue_stopped"
	EventRunCompleted              EventType = "run_completed"
	EventRunFailed                 EventType = "run_failed"
)

const LegacyEventPayloadSchema = "legacy-unversioned"

// EventPayloadSchema preserves explicit payload schemas and labels historical
// payloads that predate versioned events without silently upgrading them.
func EventPayloadSchema(payload json.RawMessage) string {
	if len(payload) == 0 {
		return LegacyEventPayloadSchema
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil {
		return LegacyEventPayloadSchema
	}
	for _, key := range []string{"schema_version", "schema"} {
		var value string
		if raw, ok := object[key]; ok && json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return LegacyEventPayloadSchema
}
