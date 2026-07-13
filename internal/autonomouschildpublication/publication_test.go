package autonomouschildpublication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/taskfile"
)

func TestJournalValidateCoversEveryAuthorityField(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Journal)
	}{
		{"schema", func(j *Journal) { j.SchemaVersion = "other" }},
		{"operation", func(j *Journal) { j.OperationID = "bad/id" }},
		{"parent", func(j *Journal) { j.ParentTaskID = "" }},
		{"decision", func(j *Journal) { j.DecisionID = "bad decision" }},
		{"proposal", func(j *Journal) { j.ProposalID = "Bad" }},
		{"material", func(j *Journal) { j.MaterialSHA256 = "bad" }},
		{"stage", func(j *Journal) { j.Stage = StageCompleted }},
		{"sequence", func(j *Journal) { j.Sequence = 0 }},
		{"children", func(j *Journal) { j.Children = nil }},
		{"created_at", func(j *Journal) { j.CreatedAt = time.Time{} }},
		{"child task", func(j *Journal) { j.Children[0].TaskID = "other" }},
		{"child key", func(j *Journal) { j.Children[0].ProposalKey = "Bad" }},
		{"child task path", func(j *Journal) { j.Children[0].TaskPath = ".agent/tasks/other.md" }},
		{"child task hash", func(j *Journal) { j.Children[0].TaskSHA256 = "bad" }},
		{"child state path", func(j *Journal) { j.Children[0].StatePath = ".revolvr/autonomous/tasks/other/state.json" }},
		{"child state hash", func(j *Journal) { j.Children[0].StateSHA256 = "bad" }},
	}
	if err := sampleJournal().Validate(); err != nil {
		t.Fatalf("valid journal: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			journal := sampleJournal()
			test.mutate(&journal)
			if err := journal.Validate(); err == nil {
				t.Fatalf("mutated journal validated: %+v", journal)
			}
		})
	}
}

func TestJournalValidateRejectsReorderedDuplicateAndDivergentTransitions(t *testing.T) {
	journal := sampleJournal()
	second := journal.Children[0]
	second.ProposalKey = "another"
	second.TaskID = ChildTaskID(journal.ParentTaskID, journal.DecisionID, journal.ProposalID, second.ProposalKey)
	second.TaskPath = filepath.ToSlash(filepath.Join(taskfile.TasksDir, second.TaskID+".md"))
	second.StatePath = filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", second.TaskID, "state.json"))

	reordered := journal
	reordered.Children = []ChildRecord{journal.Children[0], second}
	if err := reordered.Validate(); err == nil || !strings.Contains(err.Error(), "ordered") {
		t.Fatalf("reordered children error = %v", err)
	}
	duplicate := journal
	duplicate.Children = []ChildRecord{journal.Children[0], journal.Children[0]}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate children validated")
	}

	next := journal
	next.Stage, next.Sequence = StageStatesPublished, 2
	if err := ValidateTransition(journal, next); err != nil {
		t.Fatalf("valid transition: %v", err)
	}
	next.MaterialSHA256 = strings.Repeat("d", 64)
	if err := ValidateTransition(journal, next); err == nil || !strings.Contains(err.Error(), "authority changed") {
		t.Fatalf("divergent transition error = %v", err)
	}
}

func TestJournalSameAuthorityCoversEveryImmutableField(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Journal)
	}{
		{"schema", func(j *Journal) { j.SchemaVersion = "other" }},
		{"operation", func(j *Journal) { j.OperationID = "publish-two" }},
		{"parent", func(j *Journal) { j.ParentTaskID = "parent-two" }},
		{"decision", func(j *Journal) { j.DecisionID = "decision-two" }},
		{"proposal", func(j *Journal) { j.ProposalID = "proposal-two" }},
		{"material", func(j *Journal) { j.MaterialSHA256 = strings.Repeat("d", 64) }},
		{"children", func(j *Journal) { j.Children[0].TaskSHA256 = strings.Repeat("d", 64) }},
		{"created_at", func(j *Journal) { j.CreatedAt = j.CreatedAt.Add(time.Second) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, right := sampleJournal(), sampleJournal()
			right.Children = append([]ChildRecord(nil), right.Children...)
			test.mutate(&right)
			if left.SameAuthority(right) {
				t.Fatalf("authority mutation %q compared equal", test.name)
			}
		})
	}
	left, right := sampleJournal(), sampleJournal()
	right.Stage, right.Sequence = StageCompleted, 4
	if !left.SameAuthority(right) {
		t.Fatal("stage and sequence should not alter immutable authority")
	}
}

func TestLoadReconstructsContiguousHistoryAndTreatsCheckpointAsCache(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T, string, Journal)
		wantError string
	}{
		{
			name: "missing checkpoint",
			setup: func(t *testing.T, root string, journal Journal) {
				writeHistoryChain(t, root, journal, 4)
			},
		},
		{
			name: "behind checkpoint",
			setup: func(t *testing.T, root string, journal Journal) {
				history := writeHistoryChain(t, root, journal, 4)
				writeCheckpoint(t, root, history[1])
			},
		},
		{
			name: "ahead checkpoint",
			setup: func(t *testing.T, root string, journal Journal) {
				writeHistoryChain(t, root, journal, 3)
				journal.Stage, journal.Sequence = StageCompleted, 4
				writeCheckpoint(t, root, journal)
			},
			wantError: "ahead",
		},
		{
			name: "conflicting checkpoint",
			setup: func(t *testing.T, root string, journal Journal) {
				history := writeHistoryChain(t, root, journal, 4)
				conflict := history[3]
				conflict.MaterialSHA256 = strings.Repeat("d", 64)
				writeCheckpoint(t, root, conflict)
			},
			wantError: "conflicts",
		},
		{
			name: "checkpoint without history",
			setup: func(t *testing.T, root string, journal Journal) {
				writeCheckpoint(t, root, journal)
			},
			wantError: "without immutable history",
		},
		{
			name: "history gap",
			setup: func(t *testing.T, root string, journal Journal) {
				writeHistoryChain(t, root, journal, 4)
				if err := os.Remove(filepath.Join(historyDir(root), historyName(journal.OperationID, 2))); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "noncontiguous",
		},
		{
			name: "divergent history authority",
			setup: func(t *testing.T, root string, journal Journal) {
				history := writeHistoryChain(t, root, journal, 2)
				divergent := history[1]
				divergent.MaterialSHA256 = strings.Repeat("d", 64)
				writeHistory(t, root, divergent)
			},
			wantError: "authority changed",
		},
		{
			name: "noncanonical checkpoint",
			setup: func(t *testing.T, root string, journal Journal) {
				writeHistoryChain(t, root, journal, 1)
				raw, err := MarshalJournal(journal)
				if err != nil {
					t.Fatal(err)
				}
				writeRaw(t, checkpointPath(root, journal.OperationID), append(raw, ' '))
			},
			wantError: "non-canonical",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			initial := sampleJournal()
			test.setup(t, root, initial)
			projection, found, err := Load(root, initial.OperationID)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Load error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || !found || projection.Journal.Stage != StageCompleted || projection.Journal.Sequence != 4 {
				t.Fatalf("Load = %+v found=%t err=%v", projection, found, err)
			}
		})
	}
}

func sampleJournal() Journal {
	journal := Journal{
		SchemaVersion:  JournalSchemaVersion,
		OperationID:    "publish-one",
		ParentTaskID:   "parent",
		DecisionID:     "decision-one",
		ProposalID:     "proposal-one",
		MaterialSHA256: strings.Repeat("a", 64),
		Stage:          StageAdmitted,
		Sequence:       1,
		CreatedAt:      time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	}
	taskID := ChildTaskID(journal.ParentTaskID, journal.DecisionID, journal.ProposalID, "child-one")
	journal.Children = []ChildRecord{{
		TaskID:      taskID,
		ProposalKey: "child-one",
		TaskPath:    filepath.ToSlash(filepath.Join(taskfile.TasksDir, taskID+".md")),
		TaskSHA256:  strings.Repeat("b", 64),
		StatePath:   filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json")),
		StateSHA256: strings.Repeat("c", 64),
	}}
	return journal
}

func writeHistoryChain(t *testing.T, root string, initial Journal, through int64) []Journal {
	t.Helper()
	history := make([]Journal, 0, through)
	for sequence := int64(1); sequence <= through; sequence++ {
		journal := initial
		journal.Sequence = sequence
		journal.Stage, _ = stageForSequence(sequence)
		writeHistory(t, root, journal)
		history = append(history, journal)
	}
	return history
}

func writeHistory(t *testing.T, root string, journal Journal) {
	t.Helper()
	raw, err := MarshalHistory(journal)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, filepath.Join(historyDir(root), historyName(journal.OperationID, journal.Sequence)), raw)
}

func writeCheckpoint(t *testing.T, root string, journal Journal) {
	t.Helper()
	raw, err := MarshalJournal(journal)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, checkpointPath(root, journal.OperationID), raw)
}

func writeRaw(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func historyDir(root string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "child-publications", "history")
}

func checkpointPath(root, operationID string) string {
	return filepath.Join(root, ".revolvr", "autonomous", "child-publications", operationID+".json")
}
