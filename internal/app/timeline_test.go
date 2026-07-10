package app

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"revolvr/internal/ledger"
)

func TestRunTimelineCompletedRun(t *testing.T) {
	base := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	completedAt := base.Add(12 * time.Second)
	exitCode := 0
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:                 "run-complete",
			TaskID:             "task-complete",
			Task:               "Implement timeline",
			Status:             ledger.StatusCompleted,
			Summary:            "committed abc123",
			StartedAt:          base,
			CompletedAt:        &completedAt,
			CodexExitCode:      &exitCode,
			VerificationStatus: "passed",
			CommitSHA:          "abc123",
		},
		Events: []ledger.Event{
			timelineEvent(t, 1, "run-complete", ledger.EventRunStarted, base, map[string]any{"run_id": "run-complete", "task_id": "task-complete"}),
			timelineEvent(t, 2, "run-complete", ledger.EventTaskSelected, base.Add(time.Second), map[string]any{
				"task_id":      "task-complete",
				"summary":      "Implement timeline",
				"workflow":     "mixed-pass-v1",
				"phase":        "audit",
				"profile_name": "auditor",
			}),
			timelineEvent(t, 3, "run-complete", ledger.EventContextBuilt, base.Add(2*time.Second), map[string]any{"context_payload_path": ".revolvr/runs/run-complete/context.md", "context_manifest_path": ".revolvr/runs/run-complete/context.json"}),
			timelineEvent(t, 4, "run-complete", ledger.EventCodexStarted, base.Add(3*time.Second), map[string]any{"executable": "codex"}),
			timelineEvent(t, 5, "run-complete", ledger.EventCodexJSONEvent, base.Add(4*time.Second), map[string]any{"type": "turn.started"}),
			timelineEvent(t, 6, "run-complete", ledger.EventCodexCompleted, base.Add(5*time.Second), map[string]any{"exit_code": 0, "json_events": 1}),
			timelineEvent(t, 7, "run-complete", ledger.EventChangedFilesCaptured, base.Add(6*time.Second), map[string]any{"changed_files": []string{"internal/app/timeline.go", "internal/app/timeline_test.go"}}),
			timelineEvent(t, 8, "run-complete", ledger.EventReceiptParsed, base.Add(7*time.Second), map[string]any{"receipt_path": ".revolvr/receipts/run-complete.md", "verdict": "completed"}),
			timelineEvent(t, 9, "run-complete", ledger.EventVerificationStarted, base.Add(8*time.Second), map[string]any{"command_count": 1}),
			timelineEvent(t, 10, "run-complete", ledger.EventVerificationCompleted, base.Add(9*time.Second), map[string]any{
				"status":   "passed",
				"passed":   true,
				"commands": []map[string]any{{"index": 0, "command": "go test ./...", "status": "passed", "passed": true, "exit_code": 0}},
			}),
			timelineEvent(t, 11, "run-complete", ledger.EventCommitStarted, base.Add(10*time.Second), map[string]any{"changed_files": []string{"internal/app/timeline.go", "internal/app/timeline_test.go"}}),
			timelineEvent(t, 12, "run-complete", ledger.EventCommitCreated, base.Add(11*time.Second), map[string]any{"commit_sha": "abc123", "changed_files": []string{"internal/app/timeline.go", "internal/app/timeline_test.go"}}),
			timelineEvent(t, 13, "run-complete", ledger.EventRunCompleted, completedAt, map[string]any{"outcome": "committed", "message": "committed abc123", "verification_status": "passed", "commit_sha": "abc123"}),
		},
	}

	want := []RunTimelineRow{
		{Timestamp: base, Phase: "run", Status: "started", Detail: "run run-complete, task task-complete"},
		{Timestamp: base.Add(time.Second), Phase: "task", Status: "selected", Detail: "task task-complete: Implement timeline; workflow=mixed-pass-v1; phase=audit; profile=auditor"},
		{Timestamp: base.Add(2 * time.Second), Phase: "context", Status: "built", Detail: ".revolvr/runs/run-complete/context.md"},
		{Timestamp: base.Add(3 * time.Second), Phase: "codex", Status: "started", Detail: "codex"},
		{Timestamp: base.Add(4 * time.Second), Phase: "codex", Status: "progress", Detail: "turn.started"},
		{Timestamp: base.Add(5 * time.Second), Phase: "codex", Status: "completed", Detail: "exit_code=0, json_events=1"},
		{Timestamp: base.Add(6 * time.Second), Phase: "changes", Status: "captured", Detail: "2 changed files: internal/app/timeline.go, internal/app/timeline_test.go"},
		{Timestamp: base.Add(7 * time.Second), Phase: "receipt", Status: "parsed", Detail: "completed (.revolvr/receipts/run-complete.md)"},
		{Timestamp: base.Add(8 * time.Second), Phase: "verification", Status: "started", Detail: "1 command"},
		{Timestamp: base.Add(9 * time.Second), Phase: "verification", Status: "passed", Detail: "1 command"},
		{Timestamp: base.Add(10 * time.Second), Phase: "commit", Status: "started", Detail: "2 changed files: internal/app/timeline.go, internal/app/timeline_test.go"},
		{Timestamp: base.Add(11 * time.Second), Phase: "commit", Status: "created", Detail: "commit abc123"},
		{Timestamp: completedAt, Phase: "run", Status: "completed", Detail: "outcome=committed: committed abc123; verification=passed; commit=abc123"},
	}
	if got := RunTimeline(history); !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestRunTimelineVerificationFailedRun(t *testing.T) {
	base := time.Date(2026, 7, 9, 11, 0, 0, 0, time.UTC)
	completedAt := base.Add(8 * time.Second)
	exitCode := 0
	failedIndex := 0
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:                 "run-verification-failed",
			TaskID:             "task-verification-failed",
			Task:               "Break verification",
			Status:             ledger.StatusFailed,
			Summary:            "verification command 0 failed",
			StartedAt:          base,
			CompletedAt:        &completedAt,
			CodexExitCode:      &exitCode,
			VerificationStatus: "failed",
		},
		Events: []ledger.Event{
			timelineEvent(t, 1, "run-verification-failed", ledger.EventRunStarted, base, map[string]any{"run_id": "run-verification-failed", "task_id": "task-verification-failed"}),
			timelineEvent(t, 2, "run-verification-failed", ledger.EventTaskSelected, base.Add(time.Second), map[string]any{"task_id": "task-verification-failed", "summary": "Break verification"}),
			timelineEvent(t, 3, "run-verification-failed", ledger.EventCodexCompleted, base.Add(2*time.Second), map[string]any{"exit_code": 0}),
			timelineEvent(t, 4, "run-verification-failed", ledger.EventReceiptSynthesized, base.Add(3*time.Second), map[string]any{"receipt_path": ".revolvr/receipts/run-verification-failed.md", "verdict": "completed_with_concerns"}),
			timelineEvent(t, 5, "run-verification-failed", ledger.EventVerificationStarted, base.Add(4*time.Second), map[string]any{"command_count": 1}),
			timelineEvent(t, 6, "run-verification-failed", ledger.EventVerificationCompleted, base.Add(5*time.Second), map[string]any{
				"status":               "failed",
				"passed":               false,
				"failed_command_index": failedIndex,
				"commands":             []map[string]any{{"index": 0, "command": "go test ./...", "status": "failed", "passed": false, "exit_code": 1}},
			}),
			timelineEvent(t, 7, "run-verification-failed", ledger.EventReceiptWarning, base.Add(6*time.Second), map[string]any{"warning_type": "verification_mismatch", "message": "receipt verification claims differ from harness verification results", "receipt_path": ".revolvr/receipts/run-verification-failed.md"}),
			timelineEvent(t, 8, "run-verification-failed", ledger.EventRunFailed, completedAt, map[string]any{"outcome": "verification_failed", "message": "verification command 0 failed", "verification_status": "failed"}),
		},
	}

	want := []RunTimelineRow{
		{Timestamp: base, Phase: "run", Status: "started", Detail: "run run-verification-failed, task task-verification-failed"},
		{Timestamp: base.Add(time.Second), Phase: "task", Status: "selected", Detail: "task task-verification-failed: Break verification"},
		{Timestamp: base.Add(2 * time.Second), Phase: "codex", Status: "completed", Detail: "exit_code=0"},
		{Timestamp: base.Add(3 * time.Second), Phase: "receipt", Status: "synthesized", Detail: "completed_with_concerns (.revolvr/receipts/run-verification-failed.md)"},
		{Timestamp: base.Add(4 * time.Second), Phase: "verification", Status: "started", Detail: "1 command"},
		{Timestamp: base.Add(5 * time.Second), Phase: "verification", Status: "failed", Detail: "failed command 0: go test ./... (exit_code=1)"},
		{Timestamp: base.Add(6 * time.Second), Phase: "receipt", Status: "warning", Detail: "verification_mismatch: receipt verification claims differ from harness verification results (.revolvr/receipts/run-verification-failed.md)"},
		{Timestamp: completedAt, Phase: "run", Status: "failed", Detail: "outcome=verification_failed: verification command 0 failed; verification=failed"},
	}
	if got := RunTimeline(history); !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestRunTimelineCodexFailedRun(t *testing.T) {
	base := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	completedAt := base.Add(6 * time.Second)
	exitCode := 1
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:            "run-codex-failed",
			TaskID:        "task-codex-failed",
			Task:          "Break Codex",
			Status:        ledger.StatusFailed,
			Summary:       "Codex exited with code 1",
			StartedAt:     base,
			CompletedAt:   &completedAt,
			CodexExitCode: &exitCode,
		},
		Events: []ledger.Event{
			timelineEvent(t, 1, "run-codex-failed", ledger.EventRunStarted, base, map[string]any{"run_id": "run-codex-failed", "task_id": "task-codex-failed"}),
			timelineEvent(t, 2, "run-codex-failed", ledger.EventCodexStarted, base.Add(time.Second), map[string]any{"executable": "codex"}),
			timelineEvent(t, 3, "run-codex-failed", ledger.EventCodexJSONEvent, base.Add(2*time.Second), map[string]any{"type": "turn.failed", "error": "tool failed"}),
			timelineEvent(t, 4, "run-codex-failed", ledger.EventCodexCompleted, base.Add(3*time.Second), map[string]any{"exit_code": 1, "error": "exit status 1"}),
			timelineEvent(t, 5, "run-codex-failed", ledger.EventChangedFilesCaptured, base.Add(4*time.Second), map[string]any{"changed_files": []string{"internal/broken.go"}}),
			timelineEvent(t, 6, "run-codex-failed", ledger.EventReceiptSynthesized, base.Add(5*time.Second), map[string]any{"receipt_path": ".revolvr/receipts/run-codex-failed.md", "verdict": "codex_failed"}),
			timelineEvent(t, 7, "run-codex-failed", ledger.EventRunFailed, completedAt, map[string]any{"outcome": "codex_failed", "message": "Codex exited with code 1", "codex_exit_code": 1, "verification_status": "not_run"}),
		},
	}

	want := []RunTimelineRow{
		{Timestamp: base, Phase: "run", Status: "started", Detail: "run run-codex-failed, task task-codex-failed"},
		{Timestamp: base.Add(time.Second), Phase: "codex", Status: "started", Detail: "codex"},
		{Timestamp: base.Add(2 * time.Second), Phase: "codex", Status: "error", Detail: "error: tool failed"},
		{Timestamp: base.Add(3 * time.Second), Phase: "codex", Status: "failed", Detail: "exit_code=1, error=exit status 1"},
		{Timestamp: base.Add(4 * time.Second), Phase: "changes", Status: "captured", Detail: "1 changed file: internal/broken.go"},
		{Timestamp: base.Add(5 * time.Second), Phase: "receipt", Status: "synthesized", Detail: "codex_failed (.revolvr/receipts/run-codex-failed.md)"},
		{Timestamp: completedAt, Phase: "run", Status: "failed", Detail: "outcome=codex_failed: Codex exited with code 1; codex_exit_code=1; verification=not_run"},
	}
	if got := RunTimeline(history); !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestRunTimelineBlockedPreRunFailure(t *testing.T) {
	base := time.Date(2026, 7, 9, 13, 0, 0, 0, time.UTC)
	completedAt := base.Add(4 * time.Second)
	exitCode := 0
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:                 "run-blocked",
			TaskID:             "task-blocked",
			Task:               "Blocked before Codex",
			Status:             ledger.StatusFailed,
			Summary:            "pre-existing dirty files are present",
			StartedAt:          base,
			CompletedAt:        &completedAt,
			CodexExitCode:      &exitCode,
			VerificationStatus: "not_run",
		},
		Events: []ledger.Event{
			timelineEvent(t, 1, "run-blocked", ledger.EventRunStarted, base, map[string]any{"run_id": "run-blocked", "task_id": "task-blocked"}),
			timelineEvent(t, 2, "run-blocked", ledger.EventTaskSelected, base.Add(time.Second), map[string]any{"task_id": "task-blocked", "summary": "Blocked before Codex"}),
			timelineEvent(t, 3, "run-blocked", ledger.EventChangedFilesCaptured, base.Add(2*time.Second), map[string]any{"pre_run_dirty_files": []string{"README.md"}, "changed_files": []string{}}),
			timelineEvent(t, 4, "run-blocked", ledger.EventReceiptSynthesized, base.Add(3*time.Second), map[string]any{"receipt_path": ".revolvr/receipts/run-blocked.md", "verdict": "blocked", "reason": "pre-existing dirty"}),
			timelineEvent(t, 5, "run-blocked", ledger.EventRunFailed, completedAt, map[string]any{"outcome": "blocked", "message": "pre-existing dirty files are present", "verification_status": "not_run"}),
		},
	}

	want := []RunTimelineRow{
		{Timestamp: base, Phase: "run", Status: "started", Detail: "run run-blocked, task task-blocked"},
		{Timestamp: base.Add(time.Second), Phase: "task", Status: "selected", Detail: "task task-blocked: Blocked before Codex"},
		{Timestamp: base.Add(2 * time.Second), Phase: "changes", Status: "captured", Detail: "no changed files; 1 pre-run dirty file: README.md"},
		{Timestamp: base.Add(3 * time.Second), Phase: "receipt", Status: "synthesized", Detail: "blocked (.revolvr/receipts/run-blocked.md); reason: pre-existing dirty"},
		{Timestamp: completedAt, Phase: "run", Status: "failed", Detail: "outcome=blocked: pre-existing dirty files are present; verification=not_run"},
	}
	if got := RunTimeline(history); !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestRunTimelineSparseHistoryUsesRunFallback(t *testing.T) {
	startedAt := time.Date(2026, 7, 9, 14, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(time.Minute)
	exitCode := 0
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:                 "run-sparse",
			TaskID:             "task-sparse",
			Task:               "Sparse run",
			Status:             ledger.StatusCompleted,
			Summary:            "committed sparse result",
			StartedAt:          startedAt,
			CompletedAt:        &completedAt,
			CodexExitCode:      &exitCode,
			VerificationStatus: "passed",
			CommitSHA:          "def456",
		},
	}

	want := []RunTimelineRow{
		{Timestamp: startedAt, Phase: "run", Status: "started", Detail: "run run-sparse, task task-sparse"},
		{Timestamp: completedAt, Phase: "verification", Status: "passed", Detail: "from run record"},
		{Timestamp: completedAt, Phase: "commit", Status: "created", Detail: "commit def456"},
		{Timestamp: completedAt, Phase: "run", Status: "completed", Detail: "committed sparse result; verification=passed; codex_exit_code=0; commit=def456"},
	}
	if got := RunTimeline(history); !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func TestRunTimelineMalformedPayloadsUseGenericRows(t *testing.T) {
	base := time.Date(2026, 7, 9, 15, 0, 0, 0, time.UTC)
	completedAt := base.Add(9 * time.Second)
	history := ledger.RunWithEvents{
		Run: ledger.Run{
			ID:          "run-malformed",
			TaskID:      "task-malformed",
			Task:        "Malformed payloads",
			Status:      ledger.StatusFailed,
			Summary:     "failed",
			StartedAt:   base,
			CompletedAt: &completedAt,
		},
		Events: []ledger.Event{
			{ID: 1, RunID: "run-malformed", Type: ledger.EventRunStarted, CreatedAt: base, Payload: json.RawMessage(`{"run_id":123}`)},
			{ID: 2, RunID: "run-malformed", Type: ledger.EventTaskSelected, CreatedAt: base.Add(time.Second), Payload: json.RawMessage(`{"task_id":123}`)},
			{ID: 3, RunID: "run-malformed", Type: ledger.EventContextBuilt, CreatedAt: base.Add(2 * time.Second), Payload: json.RawMessage(`{"context_payload_path":123}`)},
			{ID: 4, RunID: "run-malformed", Type: ledger.EventCodexJSONEvent, CreatedAt: base.Add(3 * time.Second), Payload: json.RawMessage(`{"message":12}`)},
			{ID: 5, RunID: "run-malformed", Type: ledger.EventCodexCompleted, CreatedAt: base.Add(4 * time.Second), Payload: json.RawMessage(`{bad`)},
			{ID: 6, RunID: "run-malformed", Type: ledger.EventChangedFilesCaptured, CreatedAt: base.Add(5 * time.Second), Payload: json.RawMessage(`{"changed_files":"oops"}`)},
			{ID: 7, RunID: "run-malformed", Type: ledger.EventReceiptWarning, CreatedAt: base.Add(6 * time.Second), Payload: json.RawMessage(`{"message":"  Something bad\nwith newline  "}`)},
			{ID: 8, RunID: "run-malformed", Type: ledger.EventVerificationCompleted, CreatedAt: base.Add(7 * time.Second), Payload: json.RawMessage(`{"commands":"oops"}`)},
			{ID: 9, RunID: "run-malformed", Type: ledger.EventCommitCreated, CreatedAt: base.Add(8 * time.Second), Payload: json.RawMessage(`{"commit_sha":42}`)},
			{ID: 10, RunID: "run-malformed", Type: ledger.EventRunFailed, CreatedAt: completedAt, Payload: json.RawMessage(`{bad`)},
		},
	}

	want := []RunTimelineRow{
		{Timestamp: base, Phase: "run", Status: "started", Detail: "run started"},
		{Timestamp: base.Add(time.Second), Phase: "task", Status: "selected", Detail: "task selected"},
		{Timestamp: base.Add(2 * time.Second), Phase: "context", Status: "built", Detail: "context built"},
		{Timestamp: base.Add(3 * time.Second), Phase: "codex", Status: "progress", Detail: "codex progress"},
		{Timestamp: base.Add(4 * time.Second), Phase: "codex", Status: "completed", Detail: "codex completed"},
		{Timestamp: base.Add(5 * time.Second), Phase: "changes", Status: "captured", Detail: "changed files captured"},
		{Timestamp: base.Add(6 * time.Second), Phase: "receipt", Status: "warning", Detail: "Something bad with newline"},
		{Timestamp: base.Add(7 * time.Second), Phase: "verification", Status: "completed", Detail: "verification completed"},
		{Timestamp: base.Add(8 * time.Second), Phase: "commit", Status: "created", Detail: "commit created"},
		{Timestamp: completedAt, Phase: "run", Status: "failed", Detail: "run failed"},
	}
	if got := RunTimeline(history); !reflect.DeepEqual(got, want) {
		t.Fatalf("timeline = %#v, want %#v", got, want)
	}
}

func timelineEvent(t *testing.T, id int64, runID string, eventType ledger.EventType, createdAt time.Time, payload any) ledger.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal timeline payload: %v", err)
	}
	return ledger.Event{
		ID:        id,
		RunID:     runID,
		Type:      eventType,
		Payload:   raw,
		CreatedAt: createdAt,
	}
}
