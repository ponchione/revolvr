package ledger

type EventType string

const (
	EventRunStarted            EventType = "run_started"
	EventTaskSelected          EventType = "task_selected"
	EventRunArtifacts          EventType = "run_artifacts"
	EventContextBuilt          EventType = "context_built"
	EventPromptBuilt           EventType = "prompt_built"
	EventCodexStarted          EventType = "codex_started"
	EventCodexJSONEvent        EventType = "codex_json_event"
	EventCodexCompleted        EventType = "codex_completed"
	EventChangedFilesCaptured  EventType = "changed_files_captured"
	EventReceiptParsed         EventType = "receipt_parsed"
	EventReceiptSynthesized    EventType = "receipt_synthesized"
	EventReceiptWarning        EventType = "receipt_warning"
	EventVerificationStarted   EventType = "verification_started"
	EventVerificationCompleted EventType = "verification_completed"
	EventCommitStarted         EventType = "commit_started"
	EventCommitCreated         EventType = "commit_created"
	EventRunCompleted          EventType = "run_completed"
	EventRunFailed             EventType = "run_failed"
)
