// Package ledgerexport owns deterministic, immutable exports of the live
// SQLite ledger. It never deletes ledger rows or replaces the live database.
package ledgerexport

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/pathguard"
)

const (
	ManifestSchema = "revolvr-ledger-export-v1"
	RecordSchema   = "revolvr-ledger-export-record-v1"
)

type Artifact struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
}

type Bounds struct {
	AfterEventID   int64 `json:"after_event_id"`
	ThroughEventID int64 `json:"through_event_id"`
}

type Manifest struct {
	SchemaVersion       string                `json:"schema_version"`
	ExportID            string                `json:"export_id"`
	OperationID         string                `json:"operation_id"`
	ExportedAt          time.Time             `json:"exported_at"`
	PolicySHA256        string                `json:"policy_sha256"`
	SourceLedger        Artifact              `json:"source_ledger"`
	Bounds              Bounds                `json:"bounds"`
	HighWaterEventID    int64                 `json:"high_water_event_id"`
	RunCount            int                   `json:"run_count"`
	EventCount          int                   `json:"event_count"`
	LegacyPayloadCount  int                   `json:"legacy_payload_count"`
	Records             Artifact              `json:"records"`
	CompressedArtifacts []CompressedReference `json:"compressed_artifacts,omitempty"`
	PredecessorID       string                `json:"predecessor_id,omitempty"`
}

type CompressedReference struct {
	LogicalPath        string   `json:"logical_path"`
	OriginalSHA256     string   `json:"original_sha256"`
	OriginalByteSize   int64    `json:"original_byte_size"`
	CompressedPath     string   `json:"compressed_path"`
	CompressedSHA256   string   `json:"compressed_sha256"`
	CompressedByteSize int64    `json:"compressed_byte_size"`
	Manifest           Artifact `json:"manifest"`
}
type compressionManifestWire struct {
	SchemaVersion string `json:"schema_version"`
	OriginalPath  string `json:"original_path"`
	Original      struct {
		SHA256   string `json:"sha256"`
		ByteSize int64  `json:"byte_size"`
	} `json:"original"`
	OriginalMTime  time.Time `json:"original_mtime"`
	CompressedPath string    `json:"compressed_path"`
	Compressed     struct {
		SHA256   string `json:"sha256"`
		ByteSize int64  `json:"byte_size"`
	} `json:"compressed"`
}

type Record struct {
	SchemaVersion string       `json:"schema_version"`
	Kind          string       `json:"kind"`
	Run           *RunRecord   `json:"run,omitempty"`
	Event         *EventRecord `json:"event,omitempty"`
}

type RunRecord struct {
	ID                 string     `json:"id"`
	TaskID             string     `json:"task_id"`
	Task               string     `json:"task"`
	Status             string     `json:"status"`
	Summary            string     `json:"summary"`
	StartedAt          time.Time  `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	DurationSeconds    int        `json:"duration_seconds"`
	CodexExitCode      *int       `json:"codex_exit_code,omitempty"`
	VerificationStatus string     `json:"verification_status"`
	CommitSHA          string     `json:"commit_sha"`
}

type EventRecord struct {
	ID            int64     `json:"id"`
	RunID         string    `json:"run_id"`
	Type          string    `json:"type"`
	PayloadSchema string    `json:"payload_schema"`
	PayloadBase64 string    `json:"payload_base64"`
	CreatedAt     time.Time `json:"created_at"`
}

type ExportInput struct {
	RepositoryRoot string
	LedgerPath     string
	OperationID    string
	ExportedAt     time.Time
	PolicySHA256   string
	Bounds         Bounds
	PredecessorID  string
	Secrets        []string
}

type Result struct {
	Manifest     Manifest
	ManifestPath string
	Replayed     bool
}

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type VerifyReport struct {
	ExportID string  `json:"export_id"`
	Passed   bool    `json:"passed"`
	Checks   []Check `json:"checks"`
}

type ReplayReport struct {
	ExportID      string  `json:"export_id"`
	Passed        bool    `json:"passed"`
	RunCount      int     `json:"run_count"`
	EventCount    int     `json:"event_count"`
	TerminalRuns  int     `json:"terminal_runs"`
	ArtifactPaths int     `json:"artifact_paths"`
	Checks        []Check `json:"checks"`
}

func Export(ctx context.Context, in ExportInput) (Result, error) {
	in.RepositoryRoot = strings.TrimSpace(in.RepositoryRoot)
	in.OperationID = strings.TrimSpace(in.OperationID)
	if in.RepositoryRoot == "" || in.OperationID == "" || in.ExportedAt.IsZero() {
		return Result{}, errors.New("ledger export: repository root, operation ID, and frozen export time are required")
	}
	if in.ExportedAt.Location() != time.UTC {
		return Result{}, errors.New("ledger export: exported_at must be UTC")
	}
	if in.Bounds.AfterEventID < 0 || in.Bounds.ThroughEventID < 0 || (in.Bounds.ThroughEventID > 0 && in.Bounds.ThroughEventID <= in.Bounds.AfterEventID) {
		return Result{}, errors.New("ledger export: invalid event bounds")
	}
	root, err := canonicalRoot(in.RepositoryRoot)
	if err != nil {
		return Result{}, err
	}
	ledgerPath := strings.TrimSpace(in.LedgerPath)
	if ledgerPath == "" {
		ledgerPath = filepath.Join(root, ".revolvr", "ledger.sqlite")
	}
	ledgerIdentity, err := fileIdentity(root, ledgerPath)
	if err != nil {
		return Result{}, fmt.Errorf("ledger export: source ledger: %w", err)
	}
	store, err := ledger.OpenLiveReadOnly(ctx, ledgerPath)
	if err != nil {
		return Result{}, err
	}
	snapshot, readErr := store.ReadSnapshot(ctx)
	closeErr := store.Close()
	if readErr != nil {
		return Result{}, readErr
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	afterIdentity, err := fileIdentity(root, ledgerPath)
	if err != nil || afterIdentity != ledgerIdentity {
		return Result{}, errors.Join(err, errors.New("ledger export: source ledger changed during snapshot"))
	}
	through := in.Bounds.ThroughEventID
	if through == 0 || through > snapshot.MaxEventID {
		through = snapshot.MaxEventID
	}
	if in.Bounds.AfterEventID > through {
		return Result{}, errors.New("ledger export: after_event_id exceeds snapshot high-water")
	}
	actualBounds := Bounds{AfterEventID: in.Bounds.AfterEventID, ThroughEventID: through}
	stream, runCount, eventCount, legacyCount, err := encodeSnapshot(ctx, snapshot, actualBounds)
	if err != nil {
		return Result{}, err
	}
	if err := rejectSecrets([][]byte{stream}, in.Secrets); err != nil {
		return Result{}, err
	}
	compressedArtifacts, err := collectCompressedReferences(ctx, root, snapshot, actualBounds, in.Secrets)
	if err != nil {
		return Result{}, err
	}
	recordsHash := hashBytes(stream)
	authority := struct {
		OperationID string                `json:"operation_id"`
		ExportedAt  time.Time             `json:"exported_at"`
		Policy      string                `json:"policy_sha256"`
		Ledger      Artifact              `json:"source_ledger"`
		Bounds      Bounds                `json:"bounds"`
		RecordsHash string                `json:"records_sha256"`
		Predecessor string                `json:"predecessor_id,omitempty"`
		Compressed  []CompressedReference `json:"compressed_artifacts,omitempty"`
	}{in.OperationID, in.ExportedAt, strings.TrimSpace(in.PolicySHA256), ledgerIdentity, actualBounds, recordsHash, strings.TrimSpace(in.PredecessorID), compressedArtifacts}
	authorityRaw, _ := json.Marshal(authority)
	exportID := hashBytes(authorityRaw)
	dirRel := filepath.ToSlash(filepath.Join(".revolvr", "retention", "exports", exportID))
	recordsRel := filepath.ToSlash(filepath.Join(dirRel, "records.jsonl"))
	manifestRel := filepath.ToSlash(filepath.Join(dirRel, "manifest.json"))
	manifest := Manifest{SchemaVersion: ManifestSchema, ExportID: exportID, OperationID: in.OperationID, ExportedAt: in.ExportedAt, PolicySHA256: strings.TrimSpace(in.PolicySHA256), SourceLedger: ledgerIdentity, Bounds: actualBounds, HighWaterEventID: through, RunCount: runCount, EventCount: eventCount, LegacyPayloadCount: legacyCount, Records: Artifact{Path: recordsRel, SHA256: recordsHash, ByteSize: int64(len(stream))}, CompressedArtifacts: compressedArtifacts, PredecessorID: strings.TrimSpace(in.PredecessorID)}
	manifestRaw, err := canonicalJSON(manifest)
	if err != nil {
		return Result{}, err
	}
	if err := rejectSecrets([][]byte{manifestRaw}, in.Secrets); err != nil {
		return Result{}, err
	}
	manifestAbs, err := pathguard.Resolve(root, manifestRel)
	if err != nil {
		return Result{}, err
	}
	recordsAbs, err := pathguard.Resolve(root, recordsRel)
	if err != nil {
		return Result{}, err
	}
	if existing, readErr := os.ReadFile(manifestAbs); readErr == nil {
		if !bytes.Equal(existing, manifestRaw) {
			return Result{}, errors.New("ledger export: conflicting immutable manifest")
		}
		if raw, readErr := os.ReadFile(recordsAbs); readErr != nil || !bytes.Equal(raw, stream) {
			return Result{}, errors.Join(readErr, errors.New("ledger export: conflicting immutable records"))
		}
		return Result{Manifest: manifest, ManifestPath: manifestRel, Replayed: true}, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return Result{}, readErr
	}
	if err := os.MkdirAll(filepath.Dir(manifestAbs), 0o700); err != nil {
		return Result{}, err
	}
	if existing, readErr := os.ReadFile(recordsAbs); readErr == nil {
		if !bytes.Equal(existing, stream) {
			return Result{}, errors.New("ledger export: conflicting orphan records")
		}
	} else if errors.Is(readErr, os.ErrNotExist) {
		if err := writeImmutable(recordsAbs, stream); err != nil {
			return Result{}, err
		}
	} else {
		return Result{}, readErr
	}
	if err := writeImmutable(manifestAbs, manifestRaw); err != nil {
		return Result{}, err
	}
	return Result{Manifest: manifest, ManifestPath: manifestRel}, nil
}

func encodeSnapshot(ctx context.Context, snapshot ledger.Snapshot, bounds Bounds) ([]byte, int, int, int, error) {
	var out bytes.Buffer
	runs, events, legacy := 0, 0, 0
	for _, history := range snapshot.Runs {
		selected := make([]ledger.Event, 0, len(history.Events))
		for _, event := range history.Events {
			if event.ID > bounds.AfterEventID && event.ID <= bounds.ThroughEventID {
				selected = append(selected, event)
			}
		}
		if len(selected) == 0 && len(history.Events) != 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, err
		}
		r := history.Run
		record := Record{SchemaVersion: RecordSchema, Kind: "run", Run: &RunRecord{ID: r.ID, TaskID: r.TaskID, Task: r.Task, Status: r.Status, Summary: r.Summary, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, DurationSeconds: r.DurationSeconds, CodexExitCode: r.CodexExitCode, VerificationStatus: r.VerificationStatus, CommitSHA: r.CommitSHA}}
		if err := appendRecord(&out, record); err != nil {
			return nil, 0, 0, 0, err
		}
		runs++
		for _, event := range selected {
			schema := ledger.EventPayloadSchema(event.Payload)
			if schema == ledger.LegacyEventPayloadSchema {
				legacy++
			}
			record := Record{SchemaVersion: RecordSchema, Kind: "event", Event: &EventRecord{ID: event.ID, RunID: event.RunID, Type: string(event.Type), PayloadSchema: schema, PayloadBase64: base64.StdEncoding.EncodeToString(event.Payload), CreatedAt: event.CreatedAt}}
			if err := appendRecord(&out, record); err != nil {
				return nil, 0, 0, 0, err
			}
			events++
		}
	}
	return out.Bytes(), runs, events, legacy, nil
}

func collectCompressedReferences(ctx context.Context, root string, snapshot ledger.Snapshot, bounds Bounds, secrets []string) ([]CompressedReference, error) {
	wanted := map[string]bool{}
	for _, history := range snapshot.Runs {
		selected := len(history.Events) == 0
		for _, event := range history.Events {
			if event.ID > bounds.AfterEventID && event.ID <= bounds.ThroughEventID {
				selected = true
			}
		}
		if !selected {
			continue
		}
		artifacts, _ := ledger.RunArtifactsFromEvents(history.Events)
		for _, value := range []string{artifacts.CodexStdoutJSONLPath, artifacts.CodexStderrPath} {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			abs := value
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(root, filepath.FromSlash(value))
			}
			abs, err := filepath.Abs(abs)
			if err != nil || !pathguard.WithinRoot(root, abs) {
				return nil, errors.New("ledger export: artifact reference escapes repository")
			}
			rel, _ := filepath.Rel(root, abs)
			wanted[filepath.ToSlash(rel)] = true
		}
	}
	var out []CompressedReference
	for logical := range wanted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		abs := filepath.Join(root, filepath.FromSlash(logical))
		if _, err := os.Lstat(abs); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		manifestPath := abs + ".gz.manifest.json"
		manifestRaw, err := safeRead(manifestPath, 1<<20)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var wire compressionManifestWire
		if err := strictJSON(manifestRaw, &wire); err != nil {
			return nil, err
		}
		canonical, _ := canonicalJSON(wire)
		if !bytes.Equal(manifestRaw, canonical) || wire.SchemaVersion != "revolvr-compressed-artifact-v1" || filepath.ToSlash(filepath.Clean(wire.OriginalPath)) != logical {
			return nil, errors.New("ledger export: invalid compressed artifact manifest")
		}
		if wire.Original.ByteSize < 0 || wire.Compressed.ByteSize < 0 || len(wire.Original.SHA256) != 64 || len(wire.Compressed.SHA256) != 64 || wire.CompressedPath != logical+".gz" {
			return nil, errors.New("ledger export: invalid compressed artifact identities")
		}
		compressedAbs := filepath.Join(root, filepath.FromSlash(wire.CompressedPath))
		if !pathguard.WithinRoot(root, compressedAbs) {
			return nil, errors.New("ledger export: compressed artifact escapes repository")
		}
		compressedRaw, err := safeRead(compressedAbs, wire.Compressed.ByteSize)
		if err != nil {
			return nil, err
		}
		if int64(len(compressedRaw)) != wire.Compressed.ByteSize || hashBytes(compressedRaw) != wire.Compressed.SHA256 {
			return nil, errors.New("ledger export: compressed artifact identity mismatch")
		}
		zr, err := gzip.NewReader(bytes.NewReader(compressedRaw))
		if err != nil {
			return nil, err
		}
		originalRaw, readErr := io.ReadAll(io.LimitReader(zr, wire.Original.ByteSize+1))
		closeErr := zr.Close()
		if readErr != nil || closeErr != nil || int64(len(originalRaw)) != wire.Original.ByteSize || hashBytes(originalRaw) != wire.Original.SHA256 {
			return nil, errors.Join(readErr, closeErr, errors.New("ledger export: original artifact identity mismatch"))
		}
		if err := rejectSecrets([][]byte{originalRaw}, secrets); err != nil {
			return nil, err
		}
		manifestRel, _ := filepath.Rel(root, manifestPath)
		out = append(out, CompressedReference{LogicalPath: logical, OriginalSHA256: wire.Original.SHA256, OriginalByteSize: wire.Original.ByteSize, CompressedPath: filepath.ToSlash(wire.CompressedPath), CompressedSHA256: wire.Compressed.SHA256, CompressedByteSize: wire.Compressed.ByteSize, Manifest: Artifact{Path: filepath.ToSlash(manifestRel), SHA256: hashBytes(manifestRaw), ByteSize: int64(len(manifestRaw))}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LogicalPath < out[j].LogicalPath })
	return out, nil
}

func appendRecord(out *bytes.Buffer, record Record) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	out.Write(raw)
	out.WriteByte('\n')
	return nil
}

func Verify(ctx context.Context, repositoryRoot, exportID string, secrets []string) (VerifyReport, error) {
	manifest, raw, root, err := loadManifest(repositoryRoot, exportID)
	if err != nil {
		return VerifyReport{}, err
	}
	report := VerifyReport{ExportID: manifest.ExportID, Passed: true}
	add := func(name string, err error) {
		check := Check{Name: name, Passed: err == nil, Detail: "ok"}
		if err != nil {
			check.Detail = err.Error()
			report.Passed = false
		}
		report.Checks = append(report.Checks, check)
	}
	add("manifest_schema", validateManifest(manifest, exportID))
	if manifest.PredecessorID != "" {
		predecessor, _, _, predecessorErr := loadManifest(root, manifest.PredecessorID)
		if predecessorErr == nil && predecessor.HighWaterEventID != manifest.Bounds.AfterEventID {
			predecessorErr = errors.New("predecessor coverage does not meet incremental lower bound")
		}
		add("predecessor_export", predecessorErr)
	}
	ledgerAbs, sourcePathErr := pathguard.Resolve(root, manifest.SourceLedger.Path)
	add("source_ledger_path", sourcePathErr)
	if sourcePathErr == nil {
		if _, statErr := os.Stat(ledgerAbs); statErr == nil {
			store, openErr := ledger.OpenLiveReadOnly(ctx, ledgerAbs)
			if openErr == nil {
				snapshot, snapshotErr := store.ReadSnapshot(ctx)
				_ = store.Close()
				openErr = snapshotErr
				if openErr == nil && snapshot.MaxEventID < manifest.HighWaterEventID {
					openErr = errors.New("source ledger no longer covers export high-water")
				}
				if openErr == nil && snapshot.MaxEventID == manifest.HighWaterEventID {
					current, identityErr := fileIdentity(root, ledgerAbs)
					if identityErr != nil || current != manifest.SourceLedger {
						openErr = errors.Join(identityErr, errors.New("source ledger identity changed at same high-water"))
					}
				}
			}
			add("source_ledger_coverage", openErr)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			add("source_ledger_coverage", statErr)
		}
	}
	add("secret_absence_manifest", rejectSecrets([][]byte{raw}, secrets))
	add("compressed_artifact_references", validateCompressedReferences(manifest.CompressedArtifacts))
	recordsAbs, resolveErr := pathguard.Resolve(root, manifest.Records.Path)
	add("records_path", resolveErr)
	var records []byte
	if resolveErr == nil {
		records, err = safeRead(recordsAbs, 1<<30)
	}
	add("records_read", err)
	if err == nil {
		add("records_identity", compareIdentity(records, manifest.Records))
		add("secret_absence_records", rejectSecrets([][]byte{records}, secrets))
		_, runCount, eventCount, legacyCount, parseErr := parseRecords(ctx, records)
		add("records_canonical_order", parseErr)
		if parseErr == nil && (runCount != manifest.RunCount || eventCount != manifest.EventCount || legacyCount != manifest.LegacyPayloadCount) {
			add("record_counts", fmt.Errorf("counts differ: runs=%d events=%d legacy=%d", runCount, eventCount, legacyCount))
		} else if parseErr == nil {
			add("record_counts", nil)
		}
		if parseErr == nil && int64(eventCount) != manifest.Bounds.ThroughEventID-manifest.Bounds.AfterEventID {
			add("event_range_coverage", errors.New("event range contains a gap"))
		} else if parseErr == nil {
			add("event_range_coverage", nil)
		}
	}
	return report, nil
}

func ReplayValidate(ctx context.Context, repositoryRoot, exportID string, secrets []string) (ReplayReport, error) {
	verify, err := Verify(ctx, repositoryRoot, exportID, secrets)
	if err != nil {
		return ReplayReport{}, err
	}
	report := ReplayReport{ExportID: verify.ExportID, Passed: verify.Passed, Checks: append([]Check(nil), verify.Checks...)}
	if !verify.Passed {
		return report, nil
	}
	manifest, _, root, err := loadManifest(repositoryRoot, exportID)
	if err != nil {
		return ReplayReport{}, err
	}
	recordsAbs, _ := pathguard.Resolve(root, manifest.Records.Path)
	raw, err := safeRead(recordsAbs, 1<<30)
	if err != nil {
		return ReplayReport{}, err
	}
	parsed, runs, events, _, err := parseRecords(ctx, raw)
	if err != nil {
		return ReplayReport{}, err
	}
	report.RunCount, report.EventCount = runs, events
	for _, history := range parsed {
		switch history.Run.Status {
		case ledger.StatusRunning:
			if history.Run.CompletedAt != nil {
				report.Passed = false
				report.Checks = append(report.Checks, Check{Name: "terminal_consistency", Detail: "running run has completed_at"})
			}
		case ledger.StatusCompleted, ledger.StatusFailed:
			report.TerminalRuns++
			if history.Run.CompletedAt == nil {
				report.Passed = false
				report.Checks = append(report.Checks, Check{Name: "terminal_consistency", Detail: "terminal run lacks completed_at"})
			}
		default:
			report.Passed = false
			report.Checks = append(report.Checks, Check{Name: "terminal_consistency", Detail: "unknown run status " + history.Run.Status})
		}
		ledgerEvents := make([]ledger.Event, 0, len(history.Events))
		for _, event := range history.Events {
			payload, _ := base64.StdEncoding.DecodeString(event.PayloadBase64)
			ledgerEvents = append(ledgerEvents, ledger.Event{ID: event.ID, RunID: event.RunID, Type: ledger.EventType(event.Type), Payload: payload, CreatedAt: event.CreatedAt})
		}
		artifacts, _ := ledger.RunArtifactsFromEvents(ledgerEvents)
		for _, value := range artifactValues(artifacts) {
			if value != "" {
				report.ArtifactPaths++
			}
		}
	}
	if report.Passed {
		report.Checks = append(report.Checks, Check{Name: "replay_terminal_evidence", Passed: true, Detail: "logical history reconstructed deterministically"})
	}
	return report, nil
}

// ReplaySnapshot verifies an immutable export and reconstructs the same
// logical ledger.Snapshot shape used by live read-only metrics. It performs no
// repair and does not open or mutate the live ledger after verification.
func ReplaySnapshot(ctx context.Context, repositoryRoot, exportID string, secrets []string) (ledger.Snapshot, error) {
	verify, err := Verify(ctx, repositoryRoot, exportID, secrets)
	if err != nil {
		return ledger.Snapshot{}, err
	}
	if !verify.Passed {
		return ledger.Snapshot{}, errors.New("ledger export: verified logical snapshot refused because export checks failed")
	}
	manifest, _, root, err := loadManifest(repositoryRoot, exportID)
	if err != nil {
		return ledger.Snapshot{}, err
	}
	recordsAbs, err := pathguard.Resolve(root, manifest.Records.Path)
	if err != nil {
		return ledger.Snapshot{}, err
	}
	raw, err := safeRead(recordsAbs, 1<<30)
	if err != nil {
		return ledger.Snapshot{}, err
	}
	histories, _, _, _, err := parseRecords(ctx, raw)
	if err != nil {
		return ledger.Snapshot{}, err
	}
	out := ledger.Snapshot{Runs: make([]ledger.RunWithEvents, 0, len(histories))}
	for _, history := range histories {
		run := ledger.Run{ID: history.Run.ID, TaskID: history.Run.TaskID, Task: history.Run.Task, Status: history.Run.Status, Summary: history.Run.Summary, StartedAt: history.Run.StartedAt, CompletedAt: history.Run.CompletedAt, DurationSeconds: history.Run.DurationSeconds, CodexExitCode: history.Run.CodexExitCode, VerificationStatus: history.Run.VerificationStatus, CommitSHA: history.Run.CommitSHA}
		events := make([]ledger.Event, 0, len(history.Events))
		for _, record := range history.Events {
			payload, decodeErr := base64.StdEncoding.DecodeString(record.PayloadBase64)
			if decodeErr != nil {
				return ledger.Snapshot{}, decodeErr
			}
			events = append(events, ledger.Event{ID: record.ID, RunID: record.RunID, Type: ledger.EventType(record.Type), Payload: append([]byte(nil), payload...), CreatedAt: record.CreatedAt})
			if record.ID > out.MaxEventID {
				out.MaxEventID = record.ID
			}
		}
		out.Runs = append(out.Runs, ledger.RunWithEvents{Run: run, Events: events})
	}
	return out, nil
}

type replayHistory struct {
	Run    RunRecord
	Events []EventRecord
}

func parseRecords(ctx context.Context, raw []byte) ([]replayHistory, int, int, int, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var histories []replayHistory
	seenRuns := map[string]bool{}
	lastEventID := map[string]int64{}
	runs, events, legacy := 0, 0, 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, 0, err
		}
		line := append([]byte(nil), scanner.Bytes()...)
		var record Record
		if err := strictJSON(line, &record); err != nil {
			return nil, 0, 0, 0, err
		}
		canonical, _ := json.Marshal(record)
		if !bytes.Equal(line, canonical) {
			return nil, 0, 0, 0, errors.New("non-canonical ledger export record")
		}
		if record.SchemaVersion != RecordSchema {
			return nil, 0, 0, 0, errors.New("unknown ledger export record schema")
		}
		switch record.Kind {
		case "run":
			if record.Run == nil || record.Event != nil || seenRuns[record.Run.ID] || strings.TrimSpace(record.Run.ID) == "" {
				return nil, 0, 0, 0, errors.New("duplicate or malformed run record")
			}
			seenRuns[record.Run.ID] = true
			histories = append(histories, replayHistory{Run: *record.Run})
			runs++
		case "event":
			if record.Event == nil || record.Run != nil || !seenRuns[record.Event.RunID] || record.Event.ID <= lastEventID[record.Event.RunID] {
				return nil, 0, 0, 0, errors.New("orphaned, duplicate, or out-of-order event record")
			}
			payload, err := base64.StdEncoding.DecodeString(record.Event.PayloadBase64)
			if err != nil {
				return nil, 0, 0, 0, err
			}
			if len(payload) > 0 && !json.Valid(payload) {
				return nil, 0, 0, 0, errors.New("event payload is not valid JSON")
			}
			if record.Event.PayloadSchema != ledger.EventPayloadSchema(payload) {
				return nil, 0, 0, 0, errors.New("event payload schema mismatch")
			}
			if !supportedPayloadSchema(record.Event.PayloadSchema) {
				return nil, 0, 0, 0, errors.New("unknown event payload schema")
			}
			if record.Event.PayloadSchema == ledger.LegacyEventPayloadSchema {
				legacy++
			}
			lastEventID[record.Event.RunID] = record.Event.ID
			histories[len(histories)-1].Events = append(histories[len(histories)-1].Events, *record.Event)
			events++
		default:
			return nil, 0, 0, 0, errors.New("unknown ledger export record kind")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, 0, 0, err
	}
	return histories, runs, events, legacy, nil
}

func loadManifest(repositoryRoot, exportID string) (Manifest, []byte, string, error) {
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	exportID = strings.TrimSpace(exportID)
	if !safeID(exportID) {
		return Manifest{}, nil, "", errors.New("ledger export: invalid export ID")
	}
	rel := filepath.ToSlash(filepath.Join(".revolvr", "retention", "exports", exportID, "manifest.json"))
	abs, err := pathguard.Resolve(root, rel)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	raw, err := safeRead(abs, 4<<20)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	var manifest Manifest
	if err := strictJSON(raw, &manifest); err != nil {
		return Manifest{}, nil, "", err
	}
	canonical, _ := canonicalJSON(manifest)
	if !bytes.Equal(raw, canonical) {
		return Manifest{}, nil, "", errors.New("ledger export: non-canonical manifest")
	}
	return manifest, raw, root, nil
}

func validateManifest(m Manifest, id string) error {
	if m.SchemaVersion != ManifestSchema || m.ExportID != id || !safeID(m.ExportID) || strings.TrimSpace(m.OperationID) == "" || m.ExportedAt.IsZero() || m.ExportedAt.Location() != time.UTC {
		return errors.New("invalid ledger export manifest authority")
	}
	if m.RunCount < 0 || m.EventCount < 0 || m.LegacyPayloadCount < 0 || m.HighWaterEventID != m.Bounds.ThroughEventID {
		return errors.New("invalid ledger export counts or high-water mark")
	}
	wantRecords := filepath.ToSlash(filepath.Join(".revolvr", "retention", "exports", id, "records.jsonl"))
	if m.Records.Path != wantRecords || m.Records.ByteSize < 0 || len(m.Records.SHA256) != 64 {
		return errors.New("invalid ledger export records identity")
	}
	if m.SourceLedger.Path != ".revolvr/ledger.sqlite" || m.SourceLedger.ByteSize < 0 || len(m.SourceLedger.SHA256) != 64 {
		return errors.New("invalid source ledger identity")
	}
	return nil
}

func validateCompressedReferences(values []CompressedReference) error {
	previous := ""
	for _, value := range values {
		if value.LogicalPath == "" || value.LogicalPath <= previous || filepath.IsAbs(value.LogicalPath) || strings.HasPrefix(filepath.Clean(value.LogicalPath), "..") || value.OriginalByteSize < 0 || value.CompressedByteSize < 0 || len(value.OriginalSHA256) != 64 || len(value.CompressedSHA256) != 64 || value.CompressedPath != value.LogicalPath+".gz" || value.Manifest.Path != value.LogicalPath+".gz.manifest.json" || value.Manifest.ByteSize < 0 || len(value.Manifest.SHA256) != 64 {
			return errors.New("invalid compressed artifact reference")
		}
		previous = value.LogicalPath
	}
	return nil
}

func fileIdentity(root, path string) (Artifact, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Artifact{}, err
	}
	if !pathguard.WithinRoot(root, abs) {
		return Artifact{}, errors.New("path escapes repository")
	}
	raw, err := safeRead(abs, 1<<30)
	if err != nil {
		return Artifact{}, err
	}
	rel, _ := filepath.Rel(root, abs)
	return Artifact{Path: filepath.ToSlash(rel), SHA256: hashBytes(raw), ByteSize: int64(len(raw))}, nil
}

func safeRead(path string, cap int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("artifact is not a regular non-symlink file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
		return nil, errors.New("artifact has unexpected hard links")
	}
	if info.Size() > cap {
		return nil, errors.New("artifact exceeds read cap")
	}
	return os.ReadFile(path)
}

func writeImmutable(path string, raw []byte) error {
	if _, err := os.Lstat(path); err == nil {
		return errors.New("immutable export path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-export-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func canonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
func hashBytes(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func compareIdentity(raw []byte, a Artifact) error {
	if int64(len(raw)) != a.ByteSize || hashBytes(raw) != a.SHA256 {
		return errors.New("artifact identity mismatch")
	}
	return nil
}
func safeID(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func supportedPayloadSchema(value string) bool {
	if value == ledger.LegacyEventPayloadSchema {
		return true
	}
	if strings.HasSuffix(value, "-v2") {
		switch value {
		case "autonomous-task-run-event-v2", "autonomous-queue-event-v2", "autonomous-queue-event-v3", "autonomous-task-archive-ledger-event-v2", "autonomous-finalization-ledger-event-v2":
			return true
		default:
			return false
		}
	}
	if !strings.HasSuffix(value, "-v1") || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func strictJSON(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}
func rejectSecrets(blobs [][]byte, secrets []string) error {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		for _, blob := range blobs {
			if bytes.Contains(blob, []byte(secret)) {
				return errors.New("ledger export: configured secret detected")
			}
		}
	}
	return nil
}
func artifactValues(a ledger.RunArtifacts) []string {
	return []string{a.ContextPayloadPath, a.ContextManifestPath, a.DossierPath, a.DossierManifestPath, a.CodexStdoutJSONLPath, a.CodexStderrPath, a.LastMessagePath, a.ReceiptPath, a.SupervisorDossierPath, a.SupervisorDossierManifestPath, a.SupervisorPromptPath, a.SupervisorSchemaPath, a.SupervisorOutputPath, a.SupervisorDecisionPath, a.SupervisorProvenancePath, a.SupervisorSourcePath, a.SupervisorDiagnosticsPath, a.VerificationEvidencePath}
}

// List returns deterministic export identities for inspection surfaces.
func List(repositoryRoot string) ([]string, error) {
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, ".revolvr", "retention", "exports")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() && safeID(entry.Name()) {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
