package ledger

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
	EventRunCompleted              EventType = "run_completed"
	EventRunFailed                 EventType = "run_failed"
)
