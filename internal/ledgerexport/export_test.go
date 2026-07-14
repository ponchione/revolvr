package ledgerexport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/runtimepath"
)

func TestSafeReadRejectsBoundAncestorSubstitution(t *testing.T) {
	root := t.TempDir()
	abs := filepath.Join(root, "exports", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("trusted manifest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(root, "exports-held")
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "manifest.json")
	outsideRaw := []byte("attacker manifest\n")
	if err := os.WriteFile(outsidePath, outsideRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Dir(abs), held); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Dir(abs)); err != nil {
		t.Fatal(err)
	}
	if _, err := safeRead(boundary, abs, 1024); !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("safe read error = %v, want unsafe boundary", err)
	}
	got, err := os.ReadFile(outsidePath)
	if err != nil || string(got) != string(outsideRaw) {
		t.Fatalf("outside manifest changed: %q, %v", got, err)
	}
}

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

func TestLogicalSourceIdentityHandlesWALCheckpointAndSameHighWaterDivergence(t *testing.T) {
	root, path, now := exportFixture(t)
	db := openWALWriter(t, path)
	defer db.Close()

	checkpointWAL(t, db)
	checkpointedMain := mainFileHash(t, path)
	if _, err := db.Exec(`
INSERT INTO events(run_id, event_type, event_data, created_at)
VALUES (?, ?, ?, ?)`, "run", string(ledger.EventRunCompleted), `{"state":"wal-only"}`, now.Add(time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if got := mainFileHash(t, path); got != checkpointedMain {
		t.Fatal("WAL commit unexpectedly changed the SQLite main file")
	}

	beforeCheckpoint, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: path, OperationID: "wal-before-checkpoint", ExportedAt: now.Add(2 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if beforeCheckpoint.Manifest.HighWaterEventID != 2 {
		t.Fatalf("uncheckpointed event missing from export: %+v", beforeCheckpoint.Manifest)
	}
	if beforeCheckpoint.Manifest.SourceLedger.IdentitySchema != ledger.SnapshotIdentitySchema {
		t.Fatalf("source identity schema = %q", beforeCheckpoint.Manifest.SourceLedger.IdentitySchema)
	}
	checkpointWAL(t, db)
	if got := mainFileHash(t, path); got == checkpointedMain {
		t.Fatal("checkpoint did not change the SQLite main file; test cannot distinguish physical and logical identity")
	}
	if report, err := Verify(context.Background(), root, beforeCheckpoint.Manifest.ExportID, nil); err != nil || !report.Passed {
		t.Fatalf("logical identity rejected unchanged ledger after checkpoint: report=%+v err=%v", report, err)
	}

	stable, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: path, OperationID: "wal-stable", ExportedAt: now.Add(3 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	stableMain := mainFileHash(t, path)
	if _, err := db.Exec(`UPDATE events SET event_data = ? WHERE id = 2`, `{"state":"diverged"}`); err != nil {
		t.Fatal(err)
	}
	if got := mainFileHash(t, path); got != stableMain {
		t.Fatal("same-high-water WAL update unexpectedly changed the SQLite main file")
	}
	report, err := Verify(context.Background(), root, stable.Manifest.ExportID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || checkPassed(report.Checks, "source_ledger_coverage") {
		t.Fatalf("same-high-water logical divergence passed verification: %+v", report)
	}
}

func TestManifestAuthorityProtectsLogicalIdentitySchemaAndAcceptsLegacyShape(t *testing.T) {
	root, path, now := exportFixture(t)
	result, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: path, OperationID: "identity-schema", ExportedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	tampered := result.Manifest
	tampered.SourceLedger.IdentitySchema = ""
	raw, err := canonicalJSON(tampered)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".revolvr", "retention", "exports", result.Manifest.ExportID, "manifest.json")
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Verify(context.Background(), root, result.Manifest.ExportID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || checkPassed(report.Checks, "manifest_schema") {
		t.Fatalf("stripped logical identity schema passed: %+v", report)
	}

	legacy := result.Manifest
	legacy.SourceLedger.IdentitySchema = ""
	legacy.ExportID = exportAuthorityID(legacy)
	legacy.Records.Path = filepath.ToSlash(filepath.Join(".revolvr", "retention", "exports", legacy.ExportID, "records.jsonl"))
	if err := validateManifest(legacy, legacy.ExportID); err != nil {
		t.Fatalf("legacy source-ledger identity shape rejected: %v", err)
	}
}

func TestExportRemainsCoherentWithConcurrentWALCommit(t *testing.T) {
	for i := 0; i < 5; i++ {
		root, path, now := exportFixture(t)
		db := openWALWriter(t, path)
		checkpointWAL(t, db)

		started := make(chan struct{})
		committed := make(chan error, 1)
		go func() {
			close(started)
			_, err := db.Exec(`
INSERT INTO events(run_id, event_type, event_data, created_at)
VALUES (?, ?, ?, ?)`, "run", string(ledger.EventRunCompleted), `{"concurrent":true}`, now.Add(time.Second).Format(time.RFC3339Nano))
			committed <- err
		}()
		<-started
		result, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: path, OperationID: "concurrent-" + string(rune('a'+i)), ExportedAt: now.Add(2 * time.Second)})
		if err != nil {
			db.Close()
			t.Fatal(err)
		}
		if err := <-committed; err != nil {
			db.Close()
			t.Fatal(err)
		}
		replay, err := ReplayValidate(context.Background(), root, result.Manifest.ExportID, nil)
		if err != nil || !replay.Passed || replay.EventCount != int(result.Manifest.HighWaterEventID) {
			db.Close()
			t.Fatalf("incoherent concurrent export: manifest=%+v replay=%+v err=%v", result.Manifest, replay, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
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

func openWALWriter(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode=WAL`).Scan(&mode); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if strings.ToLower(mode) != "wal" {
		db.Close()
		t.Fatalf("journal mode = %q", mode)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func checkpointWAL(t *testing.T, db *sql.DB) {
	t.Helper()
	var busy, logFrames, checkpointed int
	if err := db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointed); err != nil {
		t.Fatal(err)
	}
	if busy != 0 {
		t.Fatalf("WAL checkpoint remained busy: log=%d checkpointed=%d", logFrames, checkpointed)
	}
}

func mainFileHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(raw)
}

func checkPassed(checks []Check, name string) bool {
	for _, check := range checks {
		if check.Name == name {
			return check.Passed
		}
	}
	return false
}
