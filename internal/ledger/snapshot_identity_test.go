package ledger

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIdentifySnapshotCoversEveryLogicalFieldAndExactPayloadBytes(t *testing.T) {
	started := time.Date(2026, 7, 13, 12, 0, 0, 123, time.UTC)
	completed := started.Add(time.Minute)
	exitCode := 7
	base := Snapshot{
		MaxEventID: 11,
		Runs: []RunWithEvents{{
			Run: Run{
				ID:                 "run-1",
				TaskID:             "task-1",
				Task:               "task text",
				Status:             StatusCompleted,
				Summary:            "summary",
				StartedAt:          started,
				CompletedAt:        &completed,
				DurationSeconds:    60,
				CodexExitCode:      &exitCode,
				VerificationStatus: "passed",
				CommitSHA:          "abc123",
			},
			Events: []Event{{
				ID:        11,
				RunID:     "run-1",
				Type:      EventRunCompleted,
				Payload:   json.RawMessage("{\"a\":1, \"b\":2}"),
				CreatedAt: completed,
			}},
		}},
	}

	want := IdentifySnapshot(base)
	if len(want.SHA256) != 64 || want.ByteSize <= 0 || IdentifySnapshot(cloneIdentitySnapshot(base)) != want {
		t.Fatalf("unstable identity: %+v", want)
	}

	tests := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"max event ID", func(s *Snapshot) { s.MaxEventID++ }},
		{"run ID", func(s *Snapshot) { s.Runs[0].Run.ID += "-changed" }},
		{"task ID", func(s *Snapshot) { s.Runs[0].Run.TaskID += "-changed" }},
		{"task", func(s *Snapshot) { s.Runs[0].Run.Task += " changed" }},
		{"status", func(s *Snapshot) { s.Runs[0].Run.Status = StatusFailed }},
		{"summary", func(s *Snapshot) { s.Runs[0].Run.Summary += " changed" }},
		{"started at", func(s *Snapshot) { s.Runs[0].Run.StartedAt = s.Runs[0].Run.StartedAt.Add(time.Nanosecond) }},
		{"completed at", func(s *Snapshot) { s.Runs[0].Run.CompletedAt = nil }},
		{"duration", func(s *Snapshot) { s.Runs[0].Run.DurationSeconds++ }},
		{"exit code", func(s *Snapshot) { changed := 8; s.Runs[0].Run.CodexExitCode = &changed }},
		{"verification", func(s *Snapshot) { s.Runs[0].Run.VerificationStatus = "failed" }},
		{"commit", func(s *Snapshot) { s.Runs[0].Run.CommitSHA += "0" }},
		{"event count", func(s *Snapshot) { s.Runs[0].Events = nil }},
		{"event ID", func(s *Snapshot) { s.Runs[0].Events[0].ID++ }},
		{"event run ID", func(s *Snapshot) { s.Runs[0].Events[0].RunID += "-changed" }},
		{"event type", func(s *Snapshot) { s.Runs[0].Events[0].Type = EventRunFailed }},
		{"exact payload bytes", func(s *Snapshot) { s.Runs[0].Events[0].Payload = json.RawMessage("{\"a\":1,\"b\":2}") }},
		{"nil payload", func(s *Snapshot) { s.Runs[0].Events[0].Payload = nil }},
		{"event time", func(s *Snapshot) { s.Runs[0].Events[0].CreatedAt = s.Runs[0].Events[0].CreatedAt.Add(time.Nanosecond) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneIdentitySnapshot(base)
			test.mutate(&changed)
			if got := IdentifySnapshot(changed); got == want {
				t.Fatalf("mutation retained identity %+v", got)
			}
		})
	}
}

func TestIdentifySnapshotNormalizesEquivalentTimeLocations(t *testing.T) {
	utc := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	offset := utc.In(time.FixedZone("offset", -4*60*60))
	first := Snapshot{Runs: []RunWithEvents{{Run: Run{StartedAt: utc}}}}
	second := Snapshot{Runs: []RunWithEvents{{Run: Run{StartedAt: offset}}}}
	if IdentifySnapshot(first) != IdentifySnapshot(second) {
		t.Fatal("equivalent instants have different identities")
	}
}

func cloneIdentitySnapshot(source Snapshot) Snapshot {
	out := source
	out.Runs = make([]RunWithEvents, len(source.Runs))
	for i, history := range source.Runs {
		out.Runs[i].Run = history.Run
		if history.Run.CompletedAt != nil {
			value := *history.Run.CompletedAt
			out.Runs[i].Run.CompletedAt = &value
		}
		if history.Run.CodexExitCode != nil {
			value := *history.Run.CodexExitCode
			out.Runs[i].Run.CodexExitCode = &value
		}
		out.Runs[i].Events = make([]Event, len(history.Events))
		copy(out.Runs[i].Events, history.Events)
		for j := range out.Runs[i].Events {
			out.Runs[i].Events[j].Payload = append(json.RawMessage(nil), history.Events[j].Payload...)
		}
	}
	return out
}
