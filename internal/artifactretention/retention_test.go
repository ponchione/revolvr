package artifactretention

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"revolvr/internal/autonomousexec"
	"revolvr/internal/ledger"
	"revolvr/internal/ledgerexport"
	revolvrlock "revolvr/internal/lock"
)

func TestPolicyDefaultsAndValidation(t *testing.T) {
	p := DefaultPolicy()
	if p.MutationEnabled || !p.RequireVerifiedExport || p.RecentRunCount != 20 {
		t.Fatalf("unsafe defaults: %+v", p)
	}
	first, raw, err := p.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	second, _, _ := p.Fingerprint()
	if first != second || len(raw) == 0 {
		t.Fatal("policy fingerprint is not deterministic")
	}
	bad := p
	bad.PruneCompressedStreams = true
	bad.RequireVerifiedExport = false
	if bad.Validate() == nil {
		t.Fatal("destructive policy without export accepted")
	}
	bad = p
	bad.CompressAfter = bad.PruneAfter + time.Second
	if bad.Validate() == nil {
		t.Fatal("contradictory ages accepted")
	}
}

func TestMutationLeasesHoldEveryCompetingAdmissionAfterAcquisitionBarrier(t *testing.T) {
	root := t.TempDir()
	release, err := acquireMutationLeases(context.Background(), root, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()

	if unlock, err := autonomousexec.TryAcquire(root); !errors.Is(err, autonomousexec.ErrActive) {
		if err == nil {
			unlock()
		}
		t.Fatalf("archive execution admission error = %v, want active exclusion", err)
	}

	lockFiles := make([]*os.File, 0, 2)
	for _, name := range []string{"git-admin.lock", "child-publication.lock"} {
		path := filepath.Join(root, ".revolvr", "locks", name)
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("open held %s: %v", name, err)
		}
		lockFiles = append(lockFiles, file)
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			t.Fatalf("competing %s admission lock error = %v, want contention", name, err)
		}
	}
	defer func() {
		for _, file := range lockFiles {
			_ = file.Close()
		}
	}()

	writerCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if writer, err := revolvrlock.AcquireSourceWriter(writerCtx, revolvrlock.Config{WorkingDir: root, RunID: "blocked-writer", PID: 123, Timeout: time.Minute}); !errors.Is(err, context.DeadlineExceeded) {
		if err == nil {
			_ = writer.Release(context.Background())
		}
		t.Fatalf("source-writer admission error = %v, want retention contention", err)
	}
	if _, found, err := revolvrlock.ReadSourceWriter(context.Background(), root); err != nil || found {
		t.Fatalf("blocked source writer published metadata: found=%t err=%v", found, err)
	}

	release()
	released = true
	if unlock, err := autonomousexec.TryAcquire(root); err != nil {
		t.Fatalf("archive admission remained blocked after release: %v", err)
	} else {
		unlock()
	}
	for _, file := range lockFiles {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatalf("competing admission remained blocked after release: %v", err)
		}
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}
	writer, err := revolvrlock.AcquireSourceWriter(context.Background(), revolvrlock.Config{WorkingDir: root, RunID: "admitted-writer", PID: 456, Timeout: time.Minute})
	if err != nil {
		t.Fatalf("source writer remained blocked after release: %v", err)
	}
	if err := writer.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestControlAndWorkspaceSourceWritersHoldSharedRetentionAdmission(t *testing.T) {
	for _, workspace := range []bool{false, true} {
		name := "control"
		if workspace {
			name = "workspace"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			cfg := revolvrlock.Config{WorkingDir: root, RunID: name + "-writer", PID: 123, Timeout: time.Minute}
			if workspace {
				execution := filepath.Join(root, ".revolvr", "autonomous", "worktrees", "workspace-one")
				if err := os.MkdirAll(execution, 0o755); err != nil {
					t.Fatal(err)
				}
				cfg = revolvrlock.Config{ControlRoot: root, ExecutionRoot: execution, WorkspaceID: "workspace-one", RunID: name + "-writer", PID: 123, Timeout: time.Minute}
			}
			writer, err := revolvrlock.AcquireSourceWriter(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}

			gcCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if release, err := acquireMutationLeases(gcCtx, root, time.Now); !errors.Is(err, context.DeadlineExceeded) {
				if err == nil {
					release()
				}
				cancel()
				_ = writer.Release(context.Background())
				t.Fatalf("GC admission error = %v, want active source-writer contention", err)
			}
			cancel()
			if err := writer.Release(context.Background()); err != nil {
				t.Fatal(err)
			}
			release, err := acquireMutationLeases(context.Background(), root, time.Now)
			if err != nil {
				t.Fatalf("GC remained blocked after source-writer release: %v", err)
			}
			release()
		})
	}
}

func TestMutationLeasesRejectLiveWriterMetadataAfterGateReleaseFailure(t *testing.T) {
	for _, workspace := range []bool{false, true} {
		name := "control"
		if workspace {
			name = "workspace"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			cfg := revolvrlock.Config{WorkingDir: root, RunID: name + "-writer", PID: 123, Timeout: time.Minute}
			if workspace {
				execution := filepath.Join(root, ".revolvr", "autonomous", "worktrees", "workspace-one")
				if err := os.MkdirAll(execution, 0o755); err != nil {
					t.Fatal(err)
				}
				cfg = revolvrlock.Config{ControlRoot: root, ExecutionRoot: execution, WorkspaceID: "workspace-one", RunID: name + "-writer", PID: 123, Timeout: time.Minute}
			}
			writer, err := revolvrlock.AcquireSourceWriter(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			if err := writer.Release(cancelled); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled writer release error = %v", err)
			}
			if release, err := acquireMutationLeases(context.Background(), root, time.Now); err == nil || !strings.Contains(err.Error(), "source writer") {
				if err == nil {
					release()
				}
				t.Fatalf("GC live-metadata admission error = %v", err)
			}
			if err := writer.Release(context.Background()); err != nil {
				t.Fatalf("clear writer metadata: %v", err)
			}
		})
	}
}

func TestPlanCompressApplyReadAndReplay(t *testing.T) {
	root, ledgerPath, logical, raw, old := retentionFixture(t)
	p := DefaultPolicy()
	p.MutationEnabled = true
	p.RecentRunCount = 0
	p.MinimumCompressBytes = 0
	p.CompressAfter = time.Hour
	p.PruneAfter = 30 * 24 * time.Hour
	in := PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "gc-one", FrozenAt: old.Add(48 * time.Hour), Policy: p, EffectiveConfigSHA256: strings.Repeat("c", 64)}
	first, err := PlanGC(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanGC(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if first.PlanID != second.PlanID || first.Totals.Compress != 1 {
		t.Fatalf("plans differ or no compression: %+v %+v", first, second)
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr", "retention")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run mutated runtime: %v", err)
	}
	result, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: first, Clock: func() time.Time { return in.FrozenAt }})
	if err != nil {
		t.Fatal(err)
	}
	if result.Journal.Stage != "cleaned" {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(logical))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original still exists: %v", err)
	}
	read, id, err := ReadLogical(context.Background(), root, logical, int64(len(raw)))
	if err != nil || id != identity(raw) || !bytes.Equal(read, raw) {
		t.Fatalf("compressed read id=%+v err=%v", id, err)
	}
	replay, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: first})
	if err != nil || !replay.Replayed {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}

func TestPruneRequiresExportAndCancelledOperationResumes(t *testing.T) {
	root, ledgerPath, logical, _, old := retentionFixture(t)
	p := DefaultPolicy()
	p.MutationEnabled = true
	p.RecentRunCount = 0
	p.MinimumCompressBytes = 0
	p.CompressAfter = time.Hour
	p.PruneAfter = 2 * time.Hour
	p.PruneCompressedStreams = true
	compressPlan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "gc-compress-first", FrozenAt: old.Add(3 * time.Hour), Policy: p, EffectiveConfigSHA256: "cfg"})
	if err != nil {
		t.Fatal(err)
	}
	if compressPlan.Totals.Compress != 1 {
		t.Fatalf("initial plan=%+v", compressPlan)
	}
	if _, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: compressPlan}); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "gc-prune", FrozenAt: old.Add(3 * time.Hour), Policy: p, EffectiveConfigSHA256: "cfg"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Totals.Prune != 1 || !plan.RequiredExport {
		t.Fatalf("plan=%+v", plan)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := ApplyGC(cancelled, ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan})
	if err == nil || !result.Resumable || !result.Journal.Cancelled {
		t.Fatalf("cancel result=%+v err=%v", result, err)
	}
	resumed, err := ResumeGC(context.Background(), root, "gc-prune", ledgerPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Journal.ExportID == "" || resumed.Journal.Stage != "cleaned" {
		t.Fatalf("resumed=%+v", resumed)
	}
	manifestRaw, readErr := os.ReadFile(filepath.Join(root, ".revolvr", "retention", "exports", resumed.Journal.ExportID, "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var manifest ledgerexport.Manifest
	if json.Unmarshal(manifestRaw, &manifest) != nil || len(manifest.CompressedArtifacts) != 1 || manifest.CompressedArtifacts[0].LogicalPath != logical {
		t.Fatalf("export compressed references=%+v", manifest.CompressedArtifacts)
	}
	if _, _, err := ReadLogical(context.Background(), root, logical, 1<<20); err == nil {
		t.Fatal("pruned artifact remains readable")
	}
}

func TestPlanPinsActiveAndRecentAndRejectsUnknownOrUnsafe(t *testing.T) {
	root, ledgerPath, _, _, old := retentionFixture(t)
	os.MkdirAll(filepath.Join(root, ".agent", "tasks"), 0o755)
	os.WriteFile(filepath.Join(root, ".agent", "tasks", "active.md"), []byte("---\nid: task-old\nstatus: blocked\n---\n# Active\n"), 0o644)
	p := DefaultPolicy()
	p.MutationEnabled = true
	p.RecentRunCount = 1
	p.MinimumCompressBytes = 0
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "pins", FrozenAt: old.Add(10 * 24 * time.Hour), Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Totals.Pinned != 1 || plan.Actions[0].Kind != ActionRetain {
		t.Fatalf("pin plan=%+v", plan)
	}
	unknown := filepath.Join(root, ".revolvr", "runs", "foreign", "unknown.jsonl")
	os.MkdirAll(filepath.Dir(unknown), 0o700)
	os.WriteFile(unknown, []byte("{}\n"), 0o600)
	if _, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "unknown", FrozenAt: old.Add(10 * 24 * time.Hour), Policy: p}); err == nil || !strings.Contains(err.Error(), "unknown stream") {
		t.Fatalf("unknown error=%v", err)
	}
}

func TestDeterministicGzipAndCorruption(t *testing.T) {
	raw := bytes.Repeat([]byte("line\n"), 1000)
	one, err := deterministicGzip(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	two, _ := deterministicGzip(context.Background(), raw)
	if !bytes.Equal(one, two) {
		t.Fatal("gzip bytes differ")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := deterministicGzip(ctx, raw); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

func TestApplyReconcilesInterruptedCompressionPublication(t *testing.T) {
	root, ledgerPath, logical, raw, old := retentionFixture(t)
	p := DefaultPolicy()
	p.MutationEnabled = true
	p.RecentRunCount = 0
	p.MinimumCompressBytes = 0
	p.CompressAfter = time.Hour
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "interrupted", FrozenAt: old.Add(48 * time.Hour), Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	journal := Journal{SchemaVersion: JournalSchema, OperationID: plan.OperationID, Stage: "admitted", Plan: plan, UpdatedAt: plan.FrozenAt}
	if err := persistJournal(root, Journal{}, &journal); err != nil {
		t.Fatal(err)
	}
	action := plan.Actions[0]
	gz, compressed, err := compressIdentity(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(root, filepath.FromSlash(logical))
	if err := writeAtomic(abs+".gz", gz, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestRaw, _ := canonicalJSON(CompressionManifest{SchemaVersion: CompressionManifestSchema, OriginalPath: logical, Original: action.Source, OriginalMTime: action.ModifiedAt, CompressedPath: logical + ".gz", Compressed: compressed})
	if err := writeAtomic(abs+".gz.manifest.json", manifestRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ApplyGC(context.Background(), ApplyInput{RepositoryRoot: root, LedgerPath: ledgerPath, Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if result.Journal.Stage != "cleaned" {
		t.Fatalf("result=%+v", result)
	}
	if read, _, err := ReadLogical(context.Background(), root, logical, int64(len(raw))); err != nil || !bytes.Equal(read, raw) {
		t.Fatalf("read after reconcile err=%v", err)
	}
}

func TestActionRevalidationUsesLogicalLedgerIdentityInWALMode(t *testing.T) {
	root, ledgerPath, _, _, old := retentionFixture(t)
	db := openRetentionWALWriter(t, ledgerPath)
	defer db.Close()
	checkpointRetentionWAL(t, db)
	checkpointedMain := retentionMainFileHash(t, ledgerPath)

	if _, err := db.Exec(`UPDATE runs SET summary = ? WHERE id = ?`, "committed in WAL", "run-old"); err != nil {
		t.Fatal(err)
	}
	if got := retentionMainFileHash(t, ledgerPath); got != checkpointedMain {
		t.Fatal("WAL commit unexpectedly changed the SQLite main file")
	}
	p := DefaultPolicy()
	p.MutationEnabled = true
	p.RecentRunCount = 0
	p.MinimumCompressBytes = 0
	plan, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "wal-checkpoint", FrozenAt: old.Add(48 * time.Hour), Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) == 0 {
		t.Fatal("plan has no action to revalidate")
	}
	checkpointRetentionWAL(t, db)
	if got := retentionMainFileHash(t, ledgerPath); got == checkpointedMain {
		t.Fatal("checkpoint did not change the SQLite main file; test cannot distinguish physical and logical identity")
	}
	if err := revalidateActionAuthority(context.Background(), root, ledgerPath, plan, plan.Actions[0]); err != nil {
		t.Fatalf("unchanged logical ledger rejected after checkpoint: %v", err)
	}

	stable, err := PlanGC(context.Background(), PlanInput{RepositoryRoot: root, LedgerPath: ledgerPath, OperationID: "wal-divergence", FrozenAt: old.Add(48 * time.Hour), Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	stableMain := retentionMainFileHash(t, ledgerPath)
	if _, err := db.Exec(`UPDATE runs SET task = ? WHERE id = ?`, "same-high-water divergence", "run-old"); err != nil {
		t.Fatal(err)
	}
	if got := retentionMainFileHash(t, ledgerPath); got != stableMain {
		t.Fatal("same-high-water WAL update unexpectedly changed the SQLite main file")
	}
	err = revalidateActionAuthority(context.Background(), root, ledgerPath, stable, stable.Actions[0])
	if err == nil || !strings.Contains(err.Error(), "ledger identity changed") {
		t.Fatalf("same-high-water logical divergence error = %v", err)
	}
}

func retentionFixture(t *testing.T) (string, string, string, []byte, time.Time) {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".revolvr", "runs", "run-old"), 0o700)
	os.MkdirAll(filepath.Join(root, ".agent", "tasks"), 0o755)
	ledgerPath := filepath.Join(root, ".revolvr", "ledger.sqlite")
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	logical := filepath.ToSlash(filepath.Join(".revolvr", "runs", "run-old", "codex.jsonl"))
	raw := bytes.Repeat([]byte("{\"type\":\"event\"}\n"), 100)
	abs := filepath.Join(root, filepath.FromSlash(logical))
	os.WriteFile(abs, raw, 0o600)
	os.Chtimes(abs, old, old)
	store, err := ledger.OpenWithClock(context.Background(), ledgerPath, func() time.Time { return old })
	if err != nil {
		t.Fatal(err)
	}
	store.CreateRun(context.Background(), ledger.RunSpec{ID: "run-old", TaskID: "task-old", Task: "old task", StartedAt: old})
	store.AppendEvent(context.Background(), "run-old", ledger.EventRunArtifacts, map[string]string{"codex_stdout_jsonl_path": logical})
	store.CompleteRun(context.Background(), "run-old", ledger.RunCompletion{Status: ledger.StatusCompleted, CompletedAt: old.Add(time.Minute)})
	store.Close()
	return root, ledgerPath, logical, raw, old
}

func openRetentionWALWriter(t *testing.T, path string) *sql.DB {
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

func checkpointRetentionWAL(t *testing.T, db *sql.DB) {
	t.Helper()
	var busy, logFrames, checkpointed int
	if err := db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointed); err != nil {
		t.Fatal(err)
	}
	if busy != 0 {
		t.Fatalf("WAL checkpoint remained busy: log=%d checkpointed=%d", logFrames, checkpointed)
	}
}

func retentionMainFileHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(raw)
}
