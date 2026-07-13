package artifactretention

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/ledgerexport"
)

func TestJournalRecoveryUsesContiguousHistoryAndTreatsCheckpointAsCache(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*testing.T, string, Journal)
		wantError string
	}{
		{
			name: "missing checkpoint",
			mutate: func(t *testing.T, root string, journal Journal) {
				if err := os.Remove(journalPath(root, journal.OperationID)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "stale backed checkpoint",
			mutate: func(t *testing.T, root string, journal Journal) {
				copyJournalFile(t, journalHistoryPath(root, journal.OperationID, 1), journalPath(root, journal.OperationID))
			},
		},
		{
			name: "abandoned history temp",
			mutate: func(t *testing.T, root string, journal Journal) {
				temp := filepath.Join(filepath.Dir(journalHistoryPath(root, journal.OperationID, 1)), ".tmp-retention-abandoned")
				if err := os.WriteFile(temp, []byte("uncommitted"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "ahead checkpoint",
			mutate: func(t *testing.T, root string, journal Journal) {
				journal.Sequence++
				writeJournalFile(t, journalPath(root, journal.OperationID), journal)
			},
			wantError: "ahead",
		},
		{
			name: "same-sequence checkpoint conflict",
			mutate: func(t *testing.T, root string, journal Journal) {
				journal.UpdatedAt = journal.UpdatedAt.Add(time.Second)
				writeJournalFile(t, journalPath(root, journal.OperationID), journal)
			},
			wantError: "conflicts",
		},
		{
			name: "stale checkpoint conflict",
			mutate: func(t *testing.T, root string, journal Journal) {
				raw, err := os.ReadFile(journalHistoryPath(root, journal.OperationID, 1))
				if err != nil {
					t.Fatal(err)
				}
				var stale Journal
				if err := strictJSON(raw, &stale); err != nil {
					t.Fatal(err)
				}
				stale.UpdatedAt = stale.UpdatedAt.Add(time.Second)
				writeJournalFile(t, journalPath(root, journal.OperationID), stale)
			},
			wantError: "conflicts",
		},
		{
			name: "checkpoint without history",
			mutate: func(t *testing.T, root string, journal Journal) {
				if err := os.RemoveAll(filepath.Dir(journalHistoryPath(root, journal.OperationID, 1))); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "without immutable history",
		},
		{
			name: "history gap",
			mutate: func(t *testing.T, root string, journal Journal) {
				if err := os.Remove(journalHistoryPath(root, journal.OperationID, 2)); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "noncontiguous",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, ledgerPath, _, plan, latest := completedCompressionJournal(t, "checkpoint-"+strings.ReplaceAll(test.name, " ", "-"))
			test.mutate(t, root, latest)
			journal, found, err := InspectGC(root, plan.OperationID)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("InspectGC error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil || !found || !reflect.DeepEqual(journal, latest) {
				t.Fatalf("history recovery = %+v found=%t err=%v, want %+v", journal, found, err, latest)
			}
			replay, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan})
			if err != nil || !replay.Replayed || !reflect.DeepEqual(replay.Journal, latest) {
				t.Fatalf("history-backed replay = %+v err=%v", replay, err)
			}
		})
	}
}

func TestJournalHistoryRejectsReorderedAndDuplicateCompletedPaths(t *testing.T) {
	for _, test := range []struct {
		name  string
		paths func([]Action) []string
	}{
		{"reordered", func(actions []Action) []string { return []string{actions[1].Path} }},
		{"duplicate", func(actions []Action) []string { return []string{actions[0].Path, actions[0].Path} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, plan := twoActionCompressionPlan(t, "invalid-prefix-"+test.name)
			journal := Journal{SchemaVersion: JournalSchema, OperationID: plan.OperationID, Stage: stageAdmitted, Plan: plan, UpdatedAt: plan.FrozenAt}
			if err := persistJournal(root, Journal{}, &journal); err != nil {
				t.Fatal(err)
			}
			actions := mutatingActions(plan)
			corrupt := cloneJournal(journal)
			corrupt.Sequence = 2
			corrupt.Stage = stageCompress
			corrupt.CompletedPaths = test.paths(actions)
			corrupt.UpdatedAt = corrupt.UpdatedAt.Add(time.Second)
			writeJournalFile(t, journalHistoryPath(root, plan.OperationID, 2), corrupt)
			if _, _, err := InspectGC(root, plan.OperationID); err == nil || !strings.Contains(err.Error(), "completed paths") {
				t.Fatalf("corrupt history error = %v", err)
			}
		})
	}
}

func TestJournalHistoryRejectsInvalidStateTransitions(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Journal, Journal, Action)
		wantError string
	}{
		{
			name: "cancelled stage without cancellation",
			mutate: func(next *Journal, _ Journal, _ Action) {
				next.Stage = stageCancelled
			},
			wantError: "cancelled state",
		},
		{
			name: "timestamp moves backward",
			mutate: func(next *Journal, prior Journal, _ Action) {
				next.Stage = stageCancelled
				next.Cancelled = true
				next.UpdatedAt = prior.UpdatedAt.Add(-time.Second)
			},
			wantError: "timestamp",
		},
		{
			name: "completion skips action transition",
			mutate: func(next *Journal, _ Journal, action Action) {
				next.Stage = stageCompleted
				next.CompletedPaths = []string{action.Path}
			},
			wantError: "completion transition",
		},
		{
			name: "unexpected export identity",
			mutate: func(next *Journal, _ Journal, _ Action) {
				next.Stage = stageCancelled
				next.Cancelled = true
				next.ExportID = strings.Repeat("a", 64)
			},
			wantError: "unexpected export identity",
		},
		{
			name: "unknown stage",
			mutate: func(next *Journal, _ Journal, _ Action) {
				next.Stage = "invented"
			},
			wantError: "unknown stage",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, _, _, plan := compressionPlan(t, "invalid-transition-"+strings.ReplaceAll(test.name, " ", "-"))
			prior := Journal{SchemaVersion: JournalSchema, OperationID: plan.OperationID, Stage: stageAdmitted, Plan: plan, UpdatedAt: plan.FrozenAt}
			if err := persistJournal(root, Journal{}, &prior); err != nil {
				t.Fatal(err)
			}
			next := cloneJournal(prior)
			next.Sequence++
			next.UpdatedAt = next.UpdatedAt.Add(time.Second)
			test.mutate(&next, prior, mutatingActions(plan)[0])
			writeJournalFile(t, journalHistoryPath(root, plan.OperationID, next.Sequence), next)
			if _, _, err := InspectGC(root, plan.OperationID); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("invalid transition error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestCleanedHistoryClaimRequiresEveryObservedEffect(t *testing.T) {
	root, ledgerPath, logical, _, old := retentionFixture(t)
	policy := DefaultPolicy()
	policy.MutationEnabled = true
	policy.RecentRunCount = 0
	policy.MinimumCompressBytes = 0
	policy.CompressAfter = time.Hour
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "fake-cleaned", FrozenAt: old.Add(48 * time.Hour), Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	action := mutatingActions(plan)[0]
	journal := Journal{SchemaVersion: JournalSchema, OperationID: plan.OperationID, Stage: stageAdmitted, Plan: plan, UpdatedAt: plan.FrozenAt}
	if err := persistJournal(root, Journal{}, &journal); err != nil {
		t.Fatal(err)
	}
	prior := cloneJournal(journal)
	journal.Stage = stageCompress
	journal.CompletedPaths = []string{action.Path}
	journal.UpdatedAt = journal.UpdatedAt.Add(time.Second)
	if err := persistJournal(root, prior, &journal); err != nil {
		t.Fatal(err)
	}
	prior = cloneJournal(journal)
	journal.Stage = stageCompleted
	journal.UpdatedAt = journal.UpdatedAt.Add(time.Second)
	if err := persistJournal(root, prior, &journal); err != nil {
		t.Fatal(err)
	}
	prior = cloneJournal(journal)
	journal.Stage = stageCleaned
	journal.UpdatedAt = journal.UpdatedAt.Add(time.Second)
	if err := persistJournal(root, prior, &journal); err != nil {
		t.Fatal(err)
	}

	if _, _, err := InspectGC(root, plan.OperationID); err == nil || !strings.Contains(err.Error(), "completed compress effect") {
		t.Fatalf("false cleaned inspection error = %v", err)
	}
	result, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan})
	if err == nil || result.Replayed || !strings.Contains(err.Error(), "completed compress effect") {
		t.Fatalf("false cleaned replay = %+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(logical))); err != nil {
		t.Fatalf("recovery mutated unapplied source: %v", err)
	}
}

func TestRecordedPruneExportIsReverifiedBeforeResumeOrTerminalReplay(t *testing.T) {
	t.Run("fake export ID", func(t *testing.T) {
		root, ledgerPath, logical, plan := prunePlan(t, "fake-export")
		journal := Journal{SchemaVersion: JournalSchema, OperationID: plan.OperationID, Stage: stageAdmitted, Plan: plan, UpdatedAt: plan.FrozenAt}
		if err := persistJournal(root, Journal{}, &journal); err != nil {
			t.Fatal(err)
		}
		prior := cloneJournal(journal)
		journal.Stage = stageExportVerified
		journal.ExportID = strings.Repeat("f", 64)
		journal.UpdatedAt = journal.UpdatedAt.Add(time.Second)
		if err := persistJournal(root, prior, &journal); err != nil {
			t.Fatal(err)
		}
		if _, _, err := InspectGC(root, plan.OperationID); err == nil || !strings.Contains(err.Error(), "recorded ledger export") {
			t.Fatalf("fake export inspection error = %v", err)
		}
		if _, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan}); err == nil || !strings.Contains(err.Error(), "recorded ledger export") {
			t.Fatalf("fake export resume error = %v", err)
		}
		if _, _, err := ReadLogical(context.Background(), root, logical, 1<<20); err != nil {
			t.Fatalf("fake export allowed prune mutation: %v", err)
		}
	})

	t.Run("valid export with different authority", func(t *testing.T) {
		root, ledgerPath, logical, plan := prunePlan(t, "wrong-export-authority")
		exported, err := ledgerexport.Export(context.Background(), ledgerexport.ExportInput{
			RepositoryRoot: root,
			LedgerPath:     ledgerPath,
			OperationID:    plan.OperationID + "-other",
			ExportedAt:     plan.FrozenAt,
			PolicySHA256:   plan.PolicySHA256,
			Bounds:         ledgerexport.Bounds{ThroughEventID: plan.Ledger.HighWaterEventID},
		})
		if err != nil {
			t.Fatal(err)
		}
		journal := Journal{SchemaVersion: JournalSchema, OperationID: plan.OperationID, Stage: stageAdmitted, Plan: plan, UpdatedAt: plan.FrozenAt}
		if err := persistJournal(root, Journal{}, &journal); err != nil {
			t.Fatal(err)
		}
		prior := cloneJournal(journal)
		journal.Stage = stageExportVerified
		journal.ExportID = exported.Manifest.ExportID
		journal.UpdatedAt = journal.UpdatedAt.Add(time.Second)
		if err := persistJournal(root, prior, &journal); err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan}); err == nil || !strings.Contains(err.Error(), "authority differs from plan") {
			t.Fatalf("wrong export authority error = %v", err)
		}
		if _, _, err := ReadLogical(context.Background(), root, logical, 1<<20); err != nil {
			t.Fatalf("wrong export authority allowed prune mutation: %v", err)
		}
	})

	t.Run("tampered terminal export", func(t *testing.T) {
		root, ledgerPath, _, plan := prunePlan(t, "tampered-export")
		result, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan, Clock: func() time.Time { return plan.FrozenAt }})
		if err != nil || result.Journal.Stage != stageCleaned || result.Journal.ExportID == "" {
			t.Fatalf("initial prune = %+v err=%v", result, err)
		}
		replay, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan})
		if err != nil || !replay.Replayed {
			t.Fatalf("verified terminal replay = %+v err=%v", replay, err)
		}
		recordsPath := filepath.Join(root, ".revolvr", "retention", "exports", result.Journal.ExportID, "records.jsonl")
		if err := os.WriteFile(recordsPath, []byte("tampered\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if replay, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan}); err == nil || replay.Replayed || !strings.Contains(err.Error(), "recorded ledger export") {
			t.Fatalf("tampered terminal replay = %+v err=%v", replay, err)
		}
	})
}

func completedCompressionJournal(t *testing.T, operationID string) (string, string, string, Plan, Journal) {
	t.Helper()
	root, ledgerPath, logical, plan := compressionPlan(t, operationID)
	result, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan, Clock: func() time.Time { return plan.FrozenAt }})
	if err != nil || result.Journal.Stage != stageCleaned {
		t.Fatalf("complete compression = %+v err=%v", result, err)
	}
	return root, ledgerPath, logical, plan, result.Journal
}

func compressionPlan(t *testing.T, operationID string) (string, string, string, Plan) {
	t.Helper()
	root, ledgerPath, logical, _, old := retentionFixture(t)
	policy := DefaultPolicy()
	policy.MutationEnabled = true
	policy.RecentRunCount = 0
	policy.MinimumCompressBytes = 0
	policy.CompressAfter = time.Hour
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: operationID, FrozenAt: old.Add(48 * time.Hour), Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	return root, ledgerPath, logical, plan
}

func twoActionCompressionPlan(t *testing.T, operationID string) (string, Plan) {
	t.Helper()
	root, ledgerPath, _, _, old := retentionFixture(t)
	logical := filepath.ToSlash(filepath.Join(".revolvr", "runs", "run-second", "codex.jsonl"))
	abs := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("{\"type\":\"second\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(abs, old, old); err != nil {
		t.Fatal(err)
	}
	store, err := ledger.OpenWithClock(context.Background(), ledgerPath, func() time.Time { return old })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRun(context.Background(), ledger.RunSpec{ID: "run-second", TaskID: "task-second", Task: "second task", StartedAt: old}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "run-second", ledger.EventRunArtifacts, map[string]string{"codex_stdout_jsonl_path": logical}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, _, err := store.CompleteRun(context.Background(), "run-second", ledger.RunCompletion{Status: ledger.StatusCompleted, CompletedAt: old.Add(time.Minute)}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	policy := DefaultPolicy()
	policy.MutationEnabled = true
	policy.RecentRunCount = 0
	policy.MinimumCompressBytes = 0
	policy.CompressAfter = time.Hour
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: operationID, FrozenAt: old.Add(48 * time.Hour), Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	if len(mutatingActions(plan)) != 2 {
		t.Fatalf("two-action plan = %+v", plan.Actions)
	}
	return root, plan
}

func prunePlan(t *testing.T, operationID string) (string, string, string, Plan) {
	t.Helper()
	root, ledgerPath, logical, _, old := retentionFixture(t)
	policy := DefaultPolicy()
	policy.MutationEnabled = true
	policy.RecentRunCount = 0
	policy.MinimumCompressBytes = 0
	policy.CompressAfter = time.Hour
	policy.PruneAfter = 2 * time.Hour
	policy.PruneCompressedStreams = true
	compression, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: operationID + "-compress", FrozenAt: old.Add(3 * time.Hour), Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: compression, Clock: func() time.Time { return compression.FrozenAt }}); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: operationID, FrozenAt: old.Add(3 * time.Hour), Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Totals.Prune != 1 || !plan.RequiredExport {
		t.Fatalf("prune plan = %+v", plan)
	}
	return root, ledgerPath, logical, plan
}

func journalHistoryPath(root, operationID string, sequence int) string {
	return filepath.Join(filepath.Dir(journalPath(root, operationID)), "history", formatJournalSequence(sequence))
}

func formatJournalSequence(sequence int) string {
	return fmt.Sprintf("%020d.json", sequence)
}

func copyJournalFile(t *testing.T, source, destination string) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeJournalFile(t *testing.T, path string, journal Journal) {
	t.Helper()
	raw, err := canonicalJSON(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
