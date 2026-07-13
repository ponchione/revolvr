package ledgerexport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/ledger"
)

func TestRecordSizeContractAcceptsExactLimitAndRejectsLimitPlusOne(t *testing.T) {
	exact := sizedRunRecord(t, MaximumRecordBytes)
	var stream bytes.Buffer
	if err := appendRecord(&stream, exact); err != nil {
		t.Fatal(err)
	}
	if stream.Len() != MaximumRecordBytes+1 {
		t.Fatalf("encoded stream size = %d, want %d including newline", stream.Len(), MaximumRecordBytes+1)
	}
	if _, runs, events, _, err := parseRecords(context.Background(), stream.Bytes()); err != nil || runs != 1 || events != 0 {
		t.Fatalf("parse exact-limit record: runs=%d events=%d err=%v", runs, events, err)
	}

	tooLarge := sizedRunRecord(t, MaximumRecordBytes+1)
	stream.Reset()
	if err := appendRecord(&stream, tooLarge); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("append limit-plus-one error = %v", err)
	}
	if stream.Len() != 0 {
		t.Fatalf("oversized append wrote %d bytes", stream.Len())
	}
	raw, err := json.Marshal(tooLarge)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if _, _, _, _, err := parseRecords(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("parse limit-plus-one error = %v", err)
	}
}

func TestMaximumSizedTaskExportVerifiesAndLimitPlusOneDoesNotPublish(t *testing.T) {
	root, ledgerPath, now, run, task := sizedTaskLedger(t)
	result, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "exact-limit", ExportedAt: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	records, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Manifest.Records.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != MaximumRecordBytes+1 || records[len(records)-1] != '\n' {
		t.Fatalf("records size = %d, want exact-limit record plus newline", len(records))
	}
	verify, err := Verify(context.Background(), root, result.Manifest.ExportID, nil)
	if err != nil || !verify.Passed {
		t.Fatalf("Verify() = %+v, %v", verify, err)
	}
	replay, err := ReplayValidate(context.Background(), root, result.Manifest.ExportID, nil)
	if err != nil || !replay.Passed || replay.RunCount != 1 || replay.EventCount != 0 {
		t.Fatalf("ReplayValidate() = %+v, %v", replay, err)
	}

	db, err := sql.Open("sqlite", ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runs SET task = ? WHERE id = ?`, task+"x", run.ID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	exportsDir := filepath.Join(root, ".revolvr", "retention", "exports")
	before, err := os.ReadDir(exportsDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "limit-plus-one", ExportedAt: now.Add(2 * time.Second)}); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("limit-plus-one export error = %v", err)
	}
	after, err := os.ReadDir(exportsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("oversized task published an export: before=%d after=%d", len(before), len(after))
	}
}

func TestOversizedEventExportDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".revolvr"), 0o700); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(root, ".revolvr", "ledger.sqlite")
	now := time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(context.Background(), ledgerPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRun(context.Background(), ledger.RunSpec{ID: "large-event-run", TaskID: "task", Task: "task", StartedAt: now}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), "large-event-run", ledger.EventRunStarted, map[string]string{"value": strings.Repeat("x", MaximumRecordBytes)}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(context.Background(), ExportInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "large-event", ExportedAt: now.Add(time.Second)}); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("oversized event export error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr", "retention")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized event published retention state: %v", err)
	}
}

func sizedRunRecord(t *testing.T, size int) Record {
	t.Helper()
	record := Record{SchemaVersion: RecordSchema, Kind: "run", Run: &RunRecord{ID: "run", TaskID: "task", Task: "", Status: ledger.StatusRunning, StartedAt: time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)}}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if size < len(raw) {
		t.Fatalf("requested record size %d is below fixed record size %d", size, len(raw))
	}
	record.Run.Task = strings.Repeat("x", size-len(raw))
	raw, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != size {
		t.Fatalf("sized record = %d bytes, want %d", len(raw), size)
	}
	return record
}

func sizedTaskLedger(t *testing.T) (string, string, time.Time, ledger.Run, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".revolvr"), 0o700); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(root, ".revolvr", "ledger.sqlite")
	now := time.Date(2026, 7, 13, 14, 30, 0, 0, time.UTC)
	store, err := ledger.OpenWithClock(context.Background(), ledgerPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.CreateRun(context.Background(), ledger.RunSpec{ID: "large-task-run", TaskID: "task", Task: "x", StartedAt: now})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(runExportRecord(run))
	if err != nil {
		t.Fatal(err)
	}
	taskSize := len(run.Task) + MaximumRecordBytes - len(raw)
	task := strings.Repeat("x", taskSize)
	run.Task = task
	raw, err = json.Marshal(runExportRecord(run))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != MaximumRecordBytes {
		t.Fatalf("task record = %d bytes, want %d", len(raw), MaximumRecordBytes)
	}
	db, err := sql.Open("sqlite", ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE runs SET task = ? WHERE id = ?`, task, run.ID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return root, ledgerPath, now, run, task
}
