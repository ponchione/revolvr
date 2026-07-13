package artifactretention

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/ledgerexport"
)

const (
	stageAdmitted       = "admitted"
	stageExportVerified = "export_verified"
	stageCompress       = "compress_applied"
	stagePrune          = "prune_applied"
	stageCancelled      = "cancelled"
	stageCompleted      = "completed"
	stageCleaned        = "cleaned"
)

func loadJournal(root, operationID string) (Journal, bool, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return Journal{}, false, errors.New("artifact GC inspect: operation ID is required")
	}
	checkpoint, checkpointFound, checkpointErr := readJournalFile(journalPath(root, operationID), operationID)
	if checkpointErr != nil {
		return Journal{}, false, fmt.Errorf("artifact GC inspect: checkpoint: %w", checkpointErr)
	}
	history, historyFound, err := journalFromHistory(root, operationID)
	if err != nil {
		return Journal{}, false, err
	}
	if !checkpointFound && !historyFound {
		return Journal{}, false, nil
	}
	if checkpointFound && !historyFound {
		return Journal{}, false, errors.New("artifact GC inspect: checkpoint exists without immutable history")
	}
	if checkpointFound {
		if checkpoint.Sequence > history[len(history)-1].Sequence {
			return Journal{}, false, errors.New("artifact GC inspect: checkpoint is ahead of immutable history")
		}
		backing := history[checkpoint.Sequence-1]
		if !reflect.DeepEqual(checkpoint, backing) {
			return Journal{}, false, errors.New("artifact GC inspect: checkpoint conflicts with immutable history")
		}
	}
	return history[len(history)-1], true, nil
}

func journalFromHistory(root, operationID string) ([]Journal, bool, error) {
	dir := filepath.Join(filepath.Dir(journalPath(root, operationID)), "history")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	committed := entries[:0]
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-retention-") && entry.Type().IsRegular() {
			continue
		}
		committed = append(committed, entry)
	}
	entries = committed
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	history := make([]Journal, 0, len(entries))
	for i, entry := range entries {
		sequence := i + 1
		if entry.IsDir() || entry.Name() != fmt.Sprintf("%020d.json", sequence) {
			return nil, false, errors.New("artifact GC inspect: invalid or noncontiguous immutable history")
		}
		journal, found, err := readJournalFile(filepath.Join(dir, entry.Name()), operationID)
		if err != nil || !found || journal.Sequence != sequence {
			return nil, false, errors.Join(err, errors.New("artifact GC inspect: invalid immutable history entry"))
		}
		if sequence == 1 {
			if err := validateInitialJournal(journal); err != nil {
				return nil, false, err
			}
		} else if err := validateJournalTransition(history[len(history)-1], journal); err != nil {
			return nil, false, err
		}
		history = append(history, journal)
	}
	return history, len(history) > 0, nil
}

func readJournalFile(path, operationID string) (Journal, bool, error) {
	raw, _, err := readRegular(path, 32<<20)
	if errors.Is(err, os.ErrNotExist) {
		return Journal{}, false, nil
	}
	if err != nil {
		return Journal{}, false, err
	}
	var journal Journal
	if err := strictJSON(raw, &journal); err != nil {
		return Journal{}, false, err
	}
	canonical, _ := canonicalJSON(journal)
	if !bytes.Equal(raw, canonical) {
		return Journal{}, false, errors.New("non-canonical journal")
	}
	if journal.OperationID != operationID {
		return Journal{}, false, errors.New("journal operation identity mismatch")
	}
	if err := validateJournalState(journal); err != nil {
		return Journal{}, false, err
	}
	return journal, true, nil
}

func validateJournalState(journal Journal) error {
	if journal.SchemaVersion != JournalSchema || strings.TrimSpace(journal.OperationID) == "" || journal.Sequence <= 0 || journal.UpdatedAt.IsZero() || journal.UpdatedAt.Location() != time.UTC {
		return errors.New("artifact GC journal: invalid identity, sequence, or timestamp")
	}
	if err := ValidatePlan(journal.Plan); err != nil {
		return fmt.Errorf("artifact GC journal: invalid plan: %w", err)
	}
	if journal.Plan.OperationID != journal.OperationID {
		return errors.New("artifact GC journal: plan operation identity mismatch")
	}
	actions := mutatingActions(journal.Plan)
	if len(journal.CompletedPaths) > len(actions) {
		return errors.New("artifact GC journal: completed actions exceed plan")
	}
	for i, path := range journal.CompletedPaths {
		if path != actions[i].Path {
			return errors.New("artifact GC journal: completed paths are not the exact action prefix")
		}
	}
	if journal.Plan.RequiredExport {
		if journal.ExportID != "" && !validExportID(journal.ExportID) {
			return errors.New("artifact GC journal: invalid export identity")
		}
	} else if journal.ExportID != "" {
		return errors.New("artifact GC journal: unexpected export identity")
	}

	switch journal.Stage {
	case stageAdmitted:
		if journal.Cancelled || len(journal.CompletedPaths) != 0 || journal.ExportID != "" {
			return errors.New("artifact GC journal: invalid admitted state")
		}
	case stageExportVerified:
		if journal.Cancelled || !journal.Plan.RequiredExport || journal.ExportID == "" || len(journal.CompletedPaths) != 0 {
			return errors.New("artifact GC journal: invalid export-verified state")
		}
	case stageCompress, stagePrune:
		if journal.Cancelled || len(journal.CompletedPaths) == 0 {
			return errors.New("artifact GC journal: invalid applied-action state")
		}
		last := actions[len(journal.CompletedPaths)-1]
		if journal.Stage != string(last.Kind)+"_applied" || journal.Plan.RequiredExport && journal.ExportID == "" {
			return errors.New("artifact GC journal: applied stage does not match completed action")
		}
	case stageCancelled:
		if !journal.Cancelled || len(journal.CompletedPaths) > 0 && journal.Plan.RequiredExport && journal.ExportID == "" {
			return errors.New("artifact GC journal: invalid cancelled state")
		}
	case stageCompleted, stageCleaned:
		if journal.Cancelled || len(journal.CompletedPaths) != len(actions) || journal.Plan.RequiredExport && journal.ExportID == "" {
			return errors.New("artifact GC journal: invalid terminal state")
		}
	default:
		return errors.New("artifact GC journal: unknown stage")
	}
	return nil
}

func validateInitialJournal(journal Journal) error {
	if journal.Sequence != 1 || journal.Stage != stageAdmitted {
		return errors.New("artifact GC journal: history must start with admission at sequence 1")
	}
	return nil
}

func validateJournalTransition(prior, next Journal) error {
	if next.Sequence != prior.Sequence+1 || next.OperationID != prior.OperationID || !reflect.DeepEqual(next.Plan, prior.Plan) || next.UpdatedAt.Before(prior.UpdatedAt) {
		return errors.New("artifact GC journal: divergent identity, plan, sequence, or timestamp")
	}
	if prior.Stage == stageCleaned {
		return errors.New("artifact GC journal: transition follows terminal cleaned state")
	}
	sameProgress := reflect.DeepEqual(next.CompletedPaths, prior.CompletedPaths) && next.ExportID == prior.ExportID
	switch next.Stage {
	case stageCancelled:
		if prior.Stage == stageCompleted || !sameProgress {
			return errors.New("artifact GC journal: invalid cancellation transition")
		}
	case stageExportVerified:
		if prior.Stage != stageAdmitted && prior.Stage != stageCancelled || prior.ExportID != "" || !reflect.DeepEqual(next.CompletedPaths, prior.CompletedPaths) {
			return errors.New("artifact GC journal: invalid export transition")
		}
	case stageCompress, stagePrune:
		if prior.Stage == stageCompleted || len(next.CompletedPaths) != len(prior.CompletedPaths)+1 || next.ExportID != prior.ExportID || !equalStrings(next.CompletedPaths[:len(prior.CompletedPaths)], prior.CompletedPaths) {
			return errors.New("artifact GC journal: invalid action transition")
		}
	case stageCompleted:
		if !sameProgress || !canCompleteFrom(prior) {
			return errors.New("artifact GC journal: invalid completion transition")
		}
	case stageCleaned:
		if prior.Stage != stageCompleted || !sameProgress {
			return errors.New("artifact GC journal: invalid cleanup transition")
		}
	default:
		return errors.New("artifact GC journal: invalid stage transition")
	}
	return nil
}

func canCompleteFrom(journal Journal) bool {
	actions := mutatingActions(journal.Plan)
	if len(journal.CompletedPaths) != len(actions) || journal.Cancelled {
		return false
	}
	if len(actions) == 0 {
		if journal.Plan.RequiredExport {
			return journal.Stage == stageExportVerified
		}
		return journal.Stage == stageAdmitted
	}
	return journal.Stage == string(actions[len(actions)-1].Kind)+"_applied"
}

func mutatingActions(plan Plan) []Action {
	actions := make([]Action, 0, plan.Totals.Compress+plan.Totals.Prune)
	for _, action := range plan.Actions {
		if action.Kind == ActionCompress || action.Kind == ActionPrune {
			actions = append(actions, action)
		}
	}
	return actions
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validExportID(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func persistJournal(root string, prior Journal, journal *Journal) error {
	next := cloneJournal(*journal)
	next.Sequence = prior.Sequence + 1
	if err := validateJournalState(next); err != nil {
		return err
	}
	if prior.Sequence == 0 {
		if err := validateInitialJournal(next); err != nil {
			return err
		}
	} else {
		if err := validateJournalState(prior); err != nil {
			return err
		}
		if err := validateJournalTransition(prior, next); err != nil {
			return err
		}
	}
	raw, err := canonicalJSON(next)
	if err != nil {
		return err
	}
	dir := filepath.Dir(journalPath(root, next.OperationID))
	historyDir := filepath.Join(dir, "history")
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		return err
	}
	historyPath := filepath.Join(historyDir, fmt.Sprintf("%020d.json", next.Sequence))
	if err := writeImmutableOrEqual(historyPath, raw); err != nil {
		return err
	}
	*journal = next
	return writeAtomic(journalPath(root, next.OperationID), raw, 0o600)
}

func cloneJournal(journal Journal) Journal {
	journal.CompletedPaths = append([]string(nil), journal.CompletedPaths...)
	return journal
}

func writeImmutableOrEqual(path string, raw []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return errors.New("artifact GC journal: conflicting immutable history")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAtomic(path, raw, 0o600)
}

func verifyJournalExport(ctx context.Context, root string, journal Journal, secrets []string) error {
	if !journal.Plan.RequiredExport || journal.ExportID == "" {
		return errors.New("artifact GC apply: required journal export is missing")
	}
	verify, err := ledgerexport.Verify(ctx, root, journal.ExportID, secrets)
	if err != nil || !verify.Passed {
		return errors.Join(err, errors.New("artifact GC apply: recorded ledger export did not verify"))
	}
	replay, err := ledgerexport.ReplayValidate(ctx, root, journal.ExportID, secrets)
	if err != nil || !replay.Passed {
		return errors.Join(err, errors.New("artifact GC apply: recorded ledger export did not replay-validate"))
	}
	manifestPath := filepath.Join(root, ".revolvr", "retention", "exports", journal.ExportID, "manifest.json")
	raw, _, err := readRegular(manifestPath, 4<<20)
	if err != nil {
		return err
	}
	var manifest ledgerexport.Manifest
	if err := strictJSON(raw, &manifest); err != nil {
		return err
	}
	canonical, _ := canonicalJSON(manifest)
	plan := journal.Plan
	if !bytes.Equal(raw, canonical) || manifest.ExportID != journal.ExportID || manifest.OperationID != plan.OperationID || !manifest.ExportedAt.Equal(plan.FrozenAt) || manifest.PolicySHA256 != plan.PolicySHA256 || manifest.Bounds.AfterEventID != 0 || manifest.Bounds.ThroughEventID != plan.Ledger.HighWaterEventID || manifest.HighWaterEventID != plan.Ledger.HighWaterEventID || manifest.PredecessorID != "" || manifest.SourceLedger.Path != plan.Ledger.Path || manifest.SourceLedger.SHA256 != plan.Ledger.SHA256 || manifest.SourceLedger.ByteSize != plan.Ledger.ByteSize || manifest.SourceLedger.IdentitySchema != plan.Ledger.IdentitySchema {
		return errors.New("artifact GC apply: recorded ledger export authority differs from plan")
	}
	return nil
}

func reconcileCompletedEffects(ctx context.Context, root string, journal Journal) error {
	actions := mutatingActions(journal.Plan)
	for i := range journal.CompletedPaths {
		action := actions[i]
		var err error
		switch action.Kind {
		case ActionCompress:
			err = reconcileCompressionEffect(ctx, root, action)
		case ActionPrune:
			err = reconcilePruneEffect(ctx, root, journal, action)
		}
		if err != nil {
			return fmt.Errorf("artifact GC recovery: completed %s effect %s: %w", action.Kind, action.Path, err)
		}
	}
	if journal.Stage == stageCleaned {
		quarantine := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(journal.OperationID), "quarantine")
		if _, err := os.Lstat(quarantine); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, errors.New("artifact GC recovery: cleaned journal retains quarantine"))
		}
	}
	return nil
}

func reconcileCompressionEffect(ctx context.Context, root string, action Action) error {
	abs := filepath.Join(root, filepath.FromSlash(action.Path))
	if err := requireAbsent(abs); err != nil {
		return errors.Join(err, errors.New("compressed action still has original representation"))
	}
	return validateCompressedEffect(ctx, abs+".gz", abs+".gz.manifest.json", action, true)
}

func reconcilePruneEffect(ctx context.Context, root string, journal Journal, action Action) error {
	abs := filepath.Join(root, filepath.FromSlash(action.Path))
	for _, path := range []string{abs, abs + ".gz", abs + ".gz.manifest.json"} {
		if err := requireAbsent(path); err != nil {
			return errors.Join(err, errors.New("pruned action still has a source representation"))
		}
	}
	base := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(journal.OperationID), "quarantine", filepath.FromSlash(action.Path))
	original, err := pathExists(base)
	if err != nil {
		return err
	}
	gz, err := pathExists(base + ".gz")
	if err != nil {
		return err
	}
	manifest, err := pathExists(base + ".gz.manifest.json")
	if err != nil {
		return err
	}
	if !original && !gz && !manifest {
		if journal.Stage == stageCompleted || journal.Stage == stageCleaned {
			return nil
		}
		return errors.New("quarantined representation is missing before cleanup")
	}
	if original {
		if action.Representation != "original" {
			return errors.New("quarantined representation kind differs from plan")
		}
		if gz || manifest {
			return errors.New("quarantine contains conflicting representations")
		}
		raw, info, err := readRegular(base, action.Source.ByteSize)
		if err != nil {
			return err
		}
		if identity(raw) != action.Source || !info.ModTime().UTC().Equal(action.ModifiedAt) {
			return errors.New("quarantined original identity or mtime differs from plan")
		}
		return nil
	}
	if !gz || !manifest {
		return errors.New("quarantined compressed representation is incomplete")
	}
	if action.Representation != "compressed" {
		return errors.New("quarantined representation kind differs from plan")
	}
	return validateCompressedEffect(ctx, base+".gz", base+".gz.manifest.json", action, false)
}

func validateCompressedEffect(ctx context.Context, gzPath, manifestPath string, action Action, comparePlannedCompressed bool) error {
	manifestRaw, _, err := readRegular(manifestPath, 1<<20)
	if err != nil {
		return err
	}
	var manifest CompressionManifest
	if err := strictJSON(manifestRaw, &manifest); err != nil {
		return err
	}
	canonical, _ := canonicalJSON(manifest)
	if !bytes.Equal(manifestRaw, canonical) || manifest.SchemaVersion != CompressionManifestSchema || manifest.OriginalPath != action.Path || manifest.Original != action.Source || !manifest.OriginalMTime.Equal(action.ModifiedAt) || manifest.CompressedPath != action.Path+".gz" || manifest.Compressed.ByteSize < 0 || !validSHA256(manifest.Compressed.SHA256) {
		return errors.New("compressed effect manifest differs from plan")
	}
	if comparePlannedCompressed && (action.Compressed == nil || manifest.Compressed != *action.Compressed) {
		return errors.New("compressed effect identity differs from planned output")
	}
	gzRaw, _, err := readRegular(gzPath, manifest.Compressed.ByteSize)
	if err != nil {
		return err
	}
	if identity(gzRaw) != manifest.Compressed {
		return errors.New("compressed effect bytes differ from manifest")
	}
	zr, err := gzip.NewReader(bytes.NewReader(gzRaw))
	if err != nil {
		return err
	}
	if action.Source.ByteSize == math.MaxInt64 {
		_ = zr.Close()
		return errors.New("compressed effect source size cannot be bounded")
	}
	raw, readErr := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, reader: zr}, action.Source.ByteSize+1))
	closeErr := zr.Close()
	if readErr != nil || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if int64(len(raw)) != action.Source.ByteSize || identity(raw) != action.Source {
		return errors.New("compressed effect does not reproduce planned source")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func requireAbsent(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		return errors.New("path exists")
	}
	return err
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
