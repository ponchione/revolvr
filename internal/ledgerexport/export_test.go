package ledgerexport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
)

func TestExportVerifyReplayDeterministicAndExact(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".revolvr"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".revolvr", "ledger.sqlite")
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(context.Background(), path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRun(context.Background(), ledger.RunSpec{ID: "run-1", TaskID: "task-1", Task: "exact task", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"context_payload_path": ".revolvr/runs/run-1/context.md", "message": "exact"}
	if _, err := store.AppendEvent(context.Background(), "run-1", ledger.EventRunArtifacts, payload); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CompleteRun(context.Background(), "run-1", ledger.RunCompletion{Status: ledger.StatusCompleted, Summary: "done", CompletedAt: now.Add(time.Second), VerificationStatus: "passed", CommitSHA: strings.Repeat("a", 40)}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	in := ExportInput{RepositoryRoot: root, LedgerPath: path, OperationID: "export-one", ExportedAt: now.Add(2 * time.Second), PolicySHA256: strings.Repeat("b", 64)}
	first, err := Export(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Export(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.ExportID != second.Manifest.ExportID || !second.Replayed {
		t.Fatalf("exports differ: first=%+v second=%+v", first, second)
	}
	if first.Manifest.LegacyPayloadCount != 1 {
		t.Fatalf("legacy payload count=%d", first.Manifest.LegacyPayloadCount)
	}
	verify, err := Verify(context.Background(), root, first.Manifest.ExportID, nil)
	if err != nil || !verify.Passed {
		t.Fatalf("verify=%+v err=%v", verify, err)
	}
	replay, err := ReplayValidate(context.Background(), root, first.Manifest.ExportID, nil)
	if err != nil || !replay.Passed || replay.TerminalRuns != 1 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	records, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.Manifest.Records.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(records), `"payload_schema":"legacy-unversioned"`) {
		t.Fatalf("records omit legacy schema: %s", records)
	}
}

func TestVerifyRejectsTamperedRecordsAndSecrets(t *testing.T) {
	root, path, now := exportFixture(t)
	result, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: path, OperationID: "export", ExportedAt: now, Secrets: []string{"not-present"}})
	if err != nil {
		t.Fatal(err)
	}
	recordsPath := filepath.Join(root, filepath.FromSlash(result.Manifest.Records.Path))
	if err := os.WriteFile(recordsPath, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), root, result.Manifest.ExportID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed {
		t.Fatalf("tampered export passed: %+v", report)
	}
	root2, path2, now2 := exportFixture(t)
	if _, err := Export(context.Background(), ExportInput{RepositoryRoot: root2, LedgerPath: path2, OperationID: "secret", ExportedAt: now2, Secrets: []string{"secret-value"}}); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret export error=%v", err)
	}
}

func exportFixture(t *testing.T) (string, string, time.Time) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".revolvr"), 0o700)
	path := filepath.Join(root, ".revolvr", "ledger.sqlite")
	now := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(context.Background(), path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	store.CreateRun(context.Background(), ledger.RunSpec{ID: "run", TaskID: "task", Task: "secret-value", StartedAt: now})
	store.AppendEvent(context.Background(), "run", ledger.EventRunStarted, map[string]string{"value": "secret-value"})
	store.Close()
	return root, path, now
}
