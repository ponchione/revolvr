package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func TestLiveReadOnlySnapshotsStayCoherentAndCurrentAcrossJournalModes(t *testing.T) {
	for _, journalMode := range []string{"delete", "wal"} {
		t.Run(journalMode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "ledger.sqlite")
			now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
			writer, err := OpenWithClock(ctx, path, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			setTestJournalMode(t, writer, journalMode)
			if _, err := writer.CreateRun(ctx, RunSpec{ID: "run-live", TaskID: "task-live", Task: "live snapshot", Summary: "version-0", StartedAt: now}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.AppendEvent(ctx, "run-live", EventContextBuilt, map[string]int{"version": 0}); err != nil {
				t.Fatal(err)
			}

			reader, err := OpenLiveReadOnly(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()

			const finalVersion = 120
			writerDone := make(chan error, 1)
			go func() {
				for version := 1; version <= finalVersion; version++ {
					tx, txErr := writer.db.BeginTx(ctx, nil)
					if txErr != nil {
						writerDone <- txErr
						return
					}
					if _, txErr = tx.ExecContext(ctx, `UPDATE runs SET summary = ? WHERE id = ?`, fmt.Sprintf("version-%d", version), "run-live"); txErr == nil {
						payload, _ := json.Marshal(map[string]int{"version": version})
						_, txErr = tx.ExecContext(ctx, `INSERT INTO events (run_id, event_type, event_data, created_at) VALUES (?, ?, ?, ?)`, "run-live", EventContextBuilt, string(payload), formatTime(now.Add(time.Duration(version)*time.Second)))
					}
					if txErr == nil {
						txErr = tx.Commit()
					} else {
						_ = tx.Rollback()
					}
					if txErr != nil {
						writerDone <- txErr
						return
					}
				}
				writerDone <- nil
			}()

			for {
				history, found, err := reader.GetRunWithEvents(ctx, "run-live")
				if err != nil || !found {
					t.Fatalf("GetRunWithEvents() = found %t, error %v", found, err)
				}
				assertVersionSnapshot(t, history)
				recent, err := reader.ListRecentRunsForTaskWithEvents(ctx, "task-live", 1)
				if err != nil || len(recent) != 1 {
					t.Fatalf("ListRecentRunsForTaskWithEvents() = %#v, %v", recent, err)
				}
				assertVersionSnapshot(t, recent[0])
				snapshot, err := reader.ReadSnapshot(ctx)
				if err != nil || len(snapshot.Runs) != 1 {
					t.Fatalf("ReadSnapshot() = %#v, %v", snapshot, err)
				}
				assertVersionSnapshot(t, snapshot.Runs[0])

				select {
				case err := <-writerDone:
					if err != nil {
						t.Fatalf("writer transaction: %v", err)
					}
					goto writerFinished
				default:
					time.Sleep(100 * time.Microsecond)
				}
			}

		writerFinished:
			final, found, err := reader.GetRunWithEvents(ctx, "run-live")
			if err != nil || !found {
				t.Fatalf("final live read = found %t, error %v", found, err)
			}
			assertVersionSnapshot(t, final)
			if final.Run.Summary != fmt.Sprintf("version-%d", finalVersion) {
				t.Fatalf("live reader remained stale: summary = %q", final.Run.Summary)
			}
		})
	}
}

func TestLiveReadOnlyUseDoesNotCreateOrModifyDurableDatabaseFiles(t *testing.T) {
	for _, journalMode := range []string{"delete", "wal"} {
		t.Run(journalMode, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			path := filepath.Join(root, "ledger.sqlite")
			now := time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC)
			writer, err := OpenWithClock(ctx, path, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			setTestJournalMode(t, writer, journalMode)
			if _, err := writer.CreateRun(ctx, RunSpec{ID: "run-files", TaskID: "task-files", Task: "file identity", StartedAt: now}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.AppendEvent(ctx, "run-files", EventRunStarted, map[string]string{"state": "started"}); err != nil {
				t.Fatal(err)
			}

			before := captureLedgerDirectory(t, root)
			reader, err := OpenLiveReadOnly(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadSnapshot(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.CreateRun(ctx, RunSpec{ID: "refused", TaskID: "refused", Task: "refused"}); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("CreateRun() error = %v, want ErrReadOnly", err)
			}
			if _, err := reader.AppendEvent(ctx, "run-files", EventRunCompleted, nil); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("AppendEvent() error = %v, want ErrReadOnly", err)
			}
			if _, _, err := reader.CompleteRun(ctx, "run-files", RunCompletion{}); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("CompleteRun() error = %v, want ErrReadOnly", err)
			}
			if err := reader.RecordCommitSHA(ctx, "run-files", "abc"); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("RecordCommitSHA() error = %v, want ErrReadOnly", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			after := captureLedgerDirectory(t, root)
			assertDurableLedgerFilesUnchanged(t, before, after)
			if journalMode == "delete" {
				for _, suffix := range []string{"-journal", "-wal", "-shm"} {
					if _, err := os.Lstat(path + suffix); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("read-only access created %s: %v", suffix, err)
					}
				}
			} else {
				for _, suffix := range []string{"-wal", "-shm"} {
					if _, ok := before[filepath.Base(path+suffix)]; !ok {
						t.Fatalf("WAL fixture omitted existing %s sidecar", suffix)
					}
				}
			}
		})
	}
}

func TestLiveReadOnlyCancellationInterruptsBusyRollbackReaderAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	now := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC)
	writer, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	setTestJournalMode(t, writer, "delete")
	if _, err := writer.CreateRun(ctx, RunSpec{ID: "run-busy", TaskID: "task-busy", Task: "busy reader", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenLiveReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	connection, err := writer.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, `BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}
	locked := true
	defer func() {
		if locked {
			_, _ = connection.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	var runCount int
	busyErr := reader.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs`).Scan(&runCount)
	if !isSQLiteBusy(busyErr) {
		t.Fatalf("locked read error = %v, want SQLite busy evidence", busyErr)
	}

	t.Run("later operation deadline", func(t *testing.T) {
		calls := 0
		result, retryErr := retryLiveRead(ctx, true, func() (int, error) {
			calls++
			if calls == 1 {
				return 1, fmt.Errorf("first busy attempt: %w", busyErr)
			}
			return 2, fmt.Errorf("second attempt: %w", context.DeadlineExceeded)
		})
		assertRetainedBusyCancellation(t, result, retryErr, context.DeadlineExceeded)
		if calls != 2 {
			t.Fatalf("operation calls = %d, want 2", calls)
		}
	})

	t.Run("later context cancellation retains latest busy", func(t *testing.T) {
		readCtx, cancel := context.WithCancel(ctx)
		calls := 0
		result, retryErr := retryLiveRead(readCtx, true, func() (int, error) {
			calls++
			switch calls {
			case 1:
				return 1, fmt.Errorf("superseded busy attempt: %w", busyErr)
			case 2:
				return 2, fmt.Errorf("latest busy attempt: %w", busyErr)
			default:
				cancel()
				return 3, errors.New("query interrupted")
			}
		})
		assertRetainedBusyCancellation(t, result, retryErr, context.Canceled)
		if calls != 3 {
			t.Fatalf("operation calls = %d, want 3", calls)
		}
		if !strings.Contains(retryErr.Error(), "latest busy attempt") {
			t.Fatalf("retry error = %v, want latest busy attempt", retryErr)
		}
		if strings.Contains(retryErr.Error(), "superseded busy attempt") {
			t.Fatalf("retry error = %v, retained superseded busy attempt", retryErr)
		}
	})

	t.Run("cancellation without busy evidence", func(t *testing.T) {
		readCtx, cancel := context.WithCancel(ctx)
		result, retryErr := retryLiveRead(readCtx, true, func() (int, error) {
			cancel()
			return 4, errors.New("query interrupted")
		})
		if result != 0 {
			t.Fatalf("retry result = %d, want zero", result)
		}
		if !errors.Is(retryErr, context.Canceled) {
			t.Fatalf("retry error = %v, want context cancellation", retryErr)
		}
		var sqliteErr *sqlite.Error
		if errors.As(retryErr, &sqliteErr) {
			t.Fatalf("retry error = %v, unexpectedly contains SQLite evidence", retryErr)
		}
	})

	if _, err := connection.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	locked = false

	if snapshot, err := reader.ReadSnapshot(ctx); err != nil || len(snapshot.Runs) != 1 {
		t.Fatalf("reader after cancellation = %#v, %v", snapshot, err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	reader = nil
	reopened, err := OpenLiveReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if snapshot, err := reopened.ReadSnapshot(ctx); err != nil || len(snapshot.Runs) != 1 {
		t.Fatalf("reopened reader = %#v, %v", snapshot, err)
	}
}

func assertRetainedBusyCancellation(t *testing.T, result int, err, contextErr error) {
	t.Helper()
	if result != 0 {
		t.Fatalf("retry result = %d, want zero", result)
	}
	if !errors.Is(err, contextErr) {
		t.Fatalf("retry error = %v, want %v", err, contextErr)
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		t.Fatalf("retry error = %v, want retained SQLite error", err)
	}
	if !isSQLiteBusy(err) {
		t.Fatalf("retry error = %v, want retained SQLite busy evidence", err)
	}
}

func assertVersionSnapshot(t *testing.T, history RunWithEvents) {
	t.Helper()
	versionText := strings.TrimPrefix(history.Run.Summary, "version-")
	version, err := strconv.Atoi(versionText)
	if err != nil {
		t.Fatalf("summary = %q, want version-N: %v", history.Run.Summary, err)
	}
	if got, want := len(history.Events), version+1; got != want {
		t.Fatalf("incoherent snapshot: summary version %d has %d events, want %d", version, got, want)
	}
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(history.Events[len(history.Events)-1].Payload, &payload); err != nil {
		t.Fatalf("last event payload: %v", err)
	}
	if payload.Version != version {
		t.Fatalf("incoherent snapshot: summary version %d has last event version %d", version, payload.Version)
	}
}

func setTestJournalMode(t *testing.T, store *Store, mode string) {
	t.Helper()
	var actual string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA journal_mode = `+mode).Scan(&actual); err != nil {
		t.Fatalf("set journal mode %s: %v", mode, err)
	}
	if !strings.EqualFold(actual, mode) {
		t.Fatalf("journal mode = %q, want %q", actual, mode)
	}
	if strings.EqualFold(mode, "wal") {
		if _, err := store.db.ExecContext(context.Background(), `PRAGMA wal_autocheckpoint = 0`); err != nil {
			t.Fatalf("disable WAL autocheckpoint: %v", err)
		}
	}
}

type ledgerFileState struct {
	Mode    os.FileMode
	Size    int64
	ModTime time.Time
	Bytes   string
}

func captureLedgerDirectory(t *testing.T, root string) map[string]ledgerFileState {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]ledgerFileState, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result[entry.Name()] = ledgerFileState{Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime(), Bytes: string(raw)}
	}
	return result
}

func describeLedgerFiles(files map[string]ledgerFileState) string {
	keys := make([]string, 0, len(files))
	for name := range files {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	names := make([]string, 0, len(keys))
	for _, name := range keys {
		state := files[name]
		names = append(names, fmt.Sprintf("%s(mode=%s size=%d mtime=%s)", name, state.Mode, state.Size, state.ModTime.UTC().Format(time.RFC3339Nano)))
	}
	return strings.Join(names, ", ")
}

func assertDurableLedgerFilesUnchanged(t *testing.T, before, after map[string]ledgerFileState) {
	t.Helper()
	if len(after) != len(before) {
		t.Fatalf("live read-only access changed sidecar set\nbefore=%s\nafter=%s", describeLedgerFiles(before), describeLedgerFiles(after))
	}
	for name, beforeState := range before {
		afterState, ok := after[name]
		if !ok {
			t.Fatalf("live read-only access removed %s", name)
		}
		if strings.HasSuffix(name, "-shm") {
			// Safe WAL readers may update transient reader marks when SQLite
			// reuses an in-process writer's shared-memory index. Those marks are
			// required locking coordination, not durable ledger content. The
			// reader must not create, resize, replace, or chmod the sidecar.
			if beforeState.Mode != afterState.Mode || beforeState.Size != afterState.Size {
				t.Fatalf("live read-only access changed WAL shared-memory identity for %s", name)
			}
			continue
		}
		if !reflect.DeepEqual(afterState, beforeState) {
			t.Fatalf("live read-only access modified durable file %s\nbefore=%s\nafter=%s", name, describeLedgerFiles(before), describeLedgerFiles(after))
		}
	}
}
