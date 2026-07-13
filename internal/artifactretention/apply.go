package artifactretention

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"revolvr/internal/ledger"
	"revolvr/internal/ledgerexport"
	"revolvr/internal/lock"
	"revolvr/internal/taskfile"
)

const JournalSchema = "revolvr-artifact-gc-journal-v1"

type Journal struct {
	SchemaVersion  string    `json:"schema_version"`
	OperationID    string    `json:"operation_id"`
	Stage          string    `json:"stage"`
	Plan           Plan      `json:"plan"`
	CompletedPaths []string  `json:"completed_paths"`
	ExportID       string    `json:"export_id,omitempty"`
	Cancelled      bool      `json:"cancelled"`
	UpdatedAt      time.Time `json:"updated_at"`
	Sequence       int       `json:"sequence"`
}
type ApplyInput struct {
	RepositoryRoot string
	LedgerPath     string
	Plan           Plan
	Secrets        []string
	Clock          func() time.Time
}
type ApplyResult struct {
	Journal   Journal
	Replayed  bool
	Resumable bool
}

func ApplyGC(ctx context.Context, in ApplyInput) (ApplyResult, error) {
	if err := ValidatePlan(in.Plan); err != nil {
		return ApplyResult{}, err
	}
	if !in.Plan.Policy.MutationEnabled {
		return ApplyResult{}, errors.New("artifact GC apply: retention mutation is disabled")
	}
	root, err := canonicalRoot(in.RepositoryRoot)
	if err != nil {
		return ApplyResult{}, err
	}
	if root != in.Plan.RepositoryRoot {
		return ApplyResult{}, errors.New("artifact GC apply: repository identity differs from plan")
	}
	if in.Clock == nil {
		in.Clock = time.Now
	}
	release, err := acquireMutationLeases(ctx, root, in.Clock)
	if err != nil {
		return ApplyResult{}, err
	}
	defer release()
	journal, found, err := loadJournal(root, in.Plan.OperationID)
	if err != nil {
		return ApplyResult{}, err
	}
	if found {
		if journal.Plan.PlanID != in.Plan.PlanID || journal.Plan.PlanSHA256 != in.Plan.PlanSHA256 {
			return ApplyResult{}, errors.New("artifact GC apply: operation conflicts with admitted plan")
		}
		if journal.Stage == "cleaned" {
			return ApplyResult{Journal: journal, Replayed: true}, nil
		}
	}
	if !found {
		replanned, err := PlanGC(context.WithoutCancel(ctx), PlanInput{RepositoryRoot: root, LedgerPath: in.LedgerPath, OperationID: in.Plan.OperationID, FrozenAt: in.Plan.FrozenAt, Policy: in.Plan.Policy, EffectiveConfigSHA256: in.Plan.EffectiveConfigSHA256})
		if err != nil {
			return ApplyResult{}, err
		}
		if replanned.PlanID != in.Plan.PlanID || replanned.PlanSHA256 != in.Plan.PlanSHA256 {
			return ApplyResult{}, errors.New("artifact GC apply: stale plan")
		}
		journal = Journal{SchemaVersion: JournalSchema, OperationID: in.Plan.OperationID, Stage: "admitted", Plan: in.Plan, UpdatedAt: in.Clock().UTC()}
		if err := persistJournal(root, &journal); err != nil {
			return ApplyResult{}, err
		}
	}
	completed := map[string]bool{}
	for _, path := range journal.CompletedPaths {
		completed[path] = true
	}
	if in.Plan.RequiredExport && journal.ExportID == "" {
		if err := ctx.Err(); err != nil {
			return cancelJournal(root, &journal, in.Clock, err)
		}
		exported, err := ledgerexport.Export(ctx, ledgerexport.ExportInput{RepositoryRoot: root, LedgerPath: in.LedgerPath, OperationID: in.Plan.OperationID, ExportedAt: in.Plan.FrozenAt, PolicySHA256: in.Plan.PolicySHA256, Bounds: ledgerexport.Bounds{ThroughEventID: in.Plan.Ledger.HighWaterEventID}, Secrets: in.Secrets})
		if err != nil {
			return ApplyResult{}, err
		}
		verify, err := ledgerexport.Verify(ctx, root, exported.Manifest.ExportID, in.Secrets)
		if err != nil || !verify.Passed {
			return ApplyResult{}, errors.Join(err, errors.New("artifact GC apply: required ledger export did not verify"))
		}
		replay, err := ledgerexport.ReplayValidate(ctx, root, exported.Manifest.ExportID, in.Secrets)
		if err != nil || !replay.Passed {
			return ApplyResult{}, errors.Join(err, errors.New("artifact GC apply: required ledger export did not replay-validate"))
		}
		journal.ExportID = exported.Manifest.ExportID
		journal.Stage = "export_verified"
		journal.UpdatedAt = in.Clock().UTC()
		if err := persistJournal(root, &journal); err != nil {
			return ApplyResult{}, err
		}
	}
	for _, action := range in.Plan.Actions {
		if action.Kind != ActionCompress && action.Kind != ActionPrune || completed[action.Path] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return cancelJournal(root, &journal, in.Clock, err)
		}
		if err := revalidateActionAuthority(ctx, root, in.LedgerPath, in.Plan, action); err != nil {
			return ApplyResult{}, err
		}
		switch action.Kind {
		case ActionCompress:
			err = applyCompression(ctx, root, action)
		case ActionPrune:
			if in.Plan.RequiredExport && journal.ExportID == "" {
				err = errors.New("artifact GC apply: prune lacks verified export")
			} else {
				err = applyPrune(root, in.Plan.OperationID, action)
			}
		}
		if err != nil {
			return ApplyResult{}, fmt.Errorf("artifact GC apply: %s %s: %w", action.Kind, action.Path, err)
		}
		journal.CompletedPaths = append(journal.CompletedPaths, action.Path)
		completed[action.Path] = true
		journal.Stage = string(action.Kind) + "_applied"
		journal.Cancelled = false
		journal.UpdatedAt = in.Clock().UTC()
		if err := persistJournal(root, &journal); err != nil {
			return ApplyResult{}, err
		}
	}
	journal.Stage = "completed"
	journal.Cancelled = false
	journal.UpdatedAt = in.Clock().UTC()
	if err := persistJournal(root, &journal); err != nil {
		return ApplyResult{}, err
	}
	quarantine := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(in.Plan.OperationID), "quarantine")
	if err := os.RemoveAll(quarantine); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
	}
	journal.Stage = "cleaned"
	journal.UpdatedAt = in.Clock().UTC()
	if err := persistJournal(root, &journal); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Journal: journal}, nil
}

func revalidateActionAuthority(ctx context.Context, root, ledgerPath string, plan Plan, action Action) error {
	if strings.TrimSpace(ledgerPath) == "" {
		ledgerPath = filepath.Join(root, ".revolvr", "ledger.sqlite")
	}
	_, err := statRegular(ledgerPath, 1<<30)
	if err != nil {
		return err
	}
	store, err := ledger.OpenLiveReadOnly(ctx, ledgerPath)
	if err != nil {
		return err
	}
	snapshot, readErr := store.ReadSnapshot(ctx)
	_ = store.Close()
	if readErr != nil {
		return readErr
	}
	if snapshot.MaxEventID != plan.Ledger.HighWaterEventID {
		return errors.New("artifact GC apply: ledger high-water changed")
	}
	identity := ledger.IdentifySnapshot(snapshot)
	if identity.SHA256 != plan.Ledger.SHA256 || identity.ByteSize != plan.Ledger.ByteSize {
		return errors.New("artifact GC apply: ledger identity changed")
	}
	tasks, err := taskfile.List(root)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ID == action.TaskID {
			return errors.New("artifact GC apply: artifact became pinned by an active task")
		}
	}
	for i, count := len(snapshot.Runs)-1, 0; i >= 0 && count < plan.Policy.RecentRunCount; i-- {
		count++
		if snapshot.Runs[i].Run.ID == action.RunID {
			return errors.New("artifact GC apply: artifact became pinned as a recent run")
		}
	}
	refs, err := controlReferences(root, []candidate{{path: action.Path, runID: action.RunID, taskID: action.TaskID}}, false)
	if err != nil {
		return err
	}
	if len(refs[action.Path]) > 0 {
		return errors.New("artifact GC apply: artifact became pinned by control evidence")
	}
	return nil
}

func ResumeGC(ctx context.Context, repositoryRoot, operationID, ledgerPath string, secrets []string) (ApplyResult, error) {
	journal, found, err := loadJournalMustRoot(repositoryRoot, operationID)
	if err != nil {
		return ApplyResult{}, err
	}
	if !found {
		return ApplyResult{}, errors.New("artifact GC resume: operation not found")
	}
	return ApplyGC(ctx, ApplyInput{RepositoryRoot: repositoryRoot, LedgerPath: ledgerPath, Plan: journal.Plan, Secrets: secrets})
}
func InspectGC(repositoryRoot, operationID string) (Journal, bool, error) {
	return loadJournalMustRoot(repositoryRoot, operationID)
}
func loadJournalMustRoot(root, id string) (Journal, bool, error) {
	canonical, err := canonicalRoot(root)
	if err != nil {
		return Journal{}, false, err
	}
	return loadJournal(canonical, id)
}

func applyCompression(ctx context.Context, root string, action Action) error {
	abs := filepath.Join(root, filepath.FromSlash(action.Path))
	manifestAbs := abs + ".gz.manifest.json"
	gzAbs := abs + ".gz"
	if _, err := os.Lstat(abs); errors.Is(err, os.ErrNotExist) {
		_, merr := os.Lstat(manifestAbs)
		if merr == nil {
			_, readErr := Read(ctx, root, action.Path, action.Source, action.Source.ByteSize)
			return readErr
		}
		return err
	}
	if _, err := os.Lstat(manifestAbs); err == nil {
		raw, info, readErr := readRegular(abs, action.Source.ByteSize)
		if readErr != nil || identity(raw) != action.Source || !info.ModTime().UTC().Equal(action.ModifiedAt) {
			return errors.Join(readErr, errors.New("conflicting dual representation"))
		}
		manifestRaw, _, readErr := readRegular(manifestAbs, 1<<20)
		if readErr != nil {
			return readErr
		}
		var manifest CompressionManifest
		if err := strictJSON(manifestRaw, &manifest); err != nil {
			return err
		}
		if manifest.Original != action.Source || manifest.CompressedPath != action.Path+".gz" || action.Compressed == nil || manifest.Compressed != *action.Compressed {
			return errors.New("conflicting dual representation authority")
		}
		gzRaw, _, readErr := readRegular(gzAbs, manifest.Compressed.ByteSize)
		if readErr != nil || identity(gzRaw) != manifest.Compressed {
			return errors.Join(readErr, errors.New("conflicting compressed bytes"))
		}
		if err := os.Remove(abs); err != nil {
			return err
		}
		if err := syncDir(filepath.Dir(abs)); err != nil {
			return err
		}
		_, err = Read(ctx, root, action.Path, action.Source, action.Source.ByteSize)
		return err
	}
	raw, info, err := readRegular(abs, action.Source.ByteSize)
	if err != nil {
		return err
	}
	if identity(raw) != action.Source || !info.ModTime().UTC().Equal(action.ModifiedAt) {
		return errors.New("source identity or mtime changed")
	}
	gz, compressed, err := compressIdentity(ctx, raw)
	if err != nil {
		return err
	}
	if action.Compressed == nil || compressed != *action.Compressed {
		return errors.New("compressed identity differs from plan")
	}
	if existing, _, readErr := readRegular(gzAbs, compressed.ByteSize); readErr == nil {
		if !bytes.Equal(existing, gz) {
			return errors.New("conflicting compressed output")
		}
	} else if errors.Is(readErr, os.ErrNotExist) {
		if err := writeAtomic(gzAbs, gz, 0o600); err != nil {
			return err
		}
	} else {
		return readErr
	}
	manifest := CompressionManifest{SchemaVersion: CompressionManifestSchema, OriginalPath: action.Path, Original: action.Source, OriginalMTime: action.ModifiedAt, CompressedPath: action.Path + ".gz", Compressed: compressed}
	manifestRaw, _ := canonicalJSON(manifest)
	if err := writeAtomic(manifestAbs, manifestRaw, 0o600); err != nil {
		return err
	}
	if _, err := Read(ctx, root, action.Path, action.Source, action.Source.ByteSize); err == nil {
		return errors.New("dual representation unexpectedly readable")
	}
	if err := os.Remove(abs); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(abs)); err != nil {
		return err
	}
	_, err = Read(ctx, root, action.Path, action.Source, action.Source.ByteSize)
	return err
}

func applyPrune(root, operationID string, action Action) error {
	abs := filepath.Join(root, filepath.FromSlash(action.Path))
	base := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(operationID), "quarantine", filepath.FromSlash(action.Path))
	if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
		return err
	}
	if raw, err := Read(context.Background(), root, action.Path, action.Source, action.Source.ByteSize); err == nil {
		if identity(raw) != action.Source {
			return errors.New("prune source identity changed")
		}
		if info, statErr := os.Lstat(abs); statErr == nil {
			if !info.ModTime().UTC().Equal(action.ModifiedAt) {
				return errors.New("prune source mtime changed")
			}
		} else {
			manifestRaw, _, readErr := readRegular(abs+".gz.manifest.json", 1<<20)
			if readErr != nil {
				return readErr
			}
			var manifest CompressionManifest
			if err := strictJSON(manifestRaw, &manifest); err != nil {
				return err
			}
			if !manifest.OriginalMTime.Equal(action.ModifiedAt) {
				return errors.New("prune compressed source mtime changed")
			}
		}
	} else {
		already := false
		for _, suffix := range []string{"", ".gz", ".gz.manifest.json"} {
			if _, statErr := os.Lstat(base + suffix); statErr == nil {
				already = true
			}
		}
		if !already {
			return err
		}
	}
	paths := []struct{ src, dst string }{{abs, base}, {abs + ".gz", base + ".gz"}, {abs + ".gz.manifest.json", base + ".gz.manifest.json"}}
	moved := false
	for _, p := range paths {
		if _, err := os.Lstat(p.src); err == nil {
			if _, err := os.Lstat(p.dst); err == nil {
				return errors.New("quarantine collision")
			}
			if err := os.Rename(p.src, p.dst); err != nil {
				return err
			}
			moved = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !moved {
		for _, p := range paths {
			if _, err := os.Lstat(p.dst); err == nil {
				return nil
			}
		}
		return errors.New("prune source and quarantine are missing")
	}
	return syncDir(filepath.Dir(abs))
}

func cancelJournal(root string, journal *Journal, clock func() time.Time, cause error) (ApplyResult, error) {
	journal.Cancelled = true
	journal.Stage = "cancelled"
	journal.UpdatedAt = clock().UTC()
	persistErr := persistJournal(root, journal)
	return ApplyResult{Journal: *journal, Resumable: true}, errors.Join(cause, persistErr)
}

func journalPath(root, id string) string {
	return filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(id), "journal.json")
}
func safeOperationDir(id string) string { return hash([]byte(strings.TrimSpace(id))) }
func loadJournal(root, id string) (Journal, bool, error) {
	path := journalPath(root, id)
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
	if !bytes.Equal(raw, canonical) || journal.SchemaVersion != JournalSchema || journal.OperationID != strings.TrimSpace(id) {
		return Journal{}, false, errors.New("artifact GC inspect: invalid journal")
	}
	return journal, true, nil
}
func persistJournal(root string, journal *Journal) error {
	journal.Sequence++
	raw, err := canonicalJSON(*journal)
	if err != nil {
		return err
	}
	dir := filepath.Dir(journalPath(root, journal.OperationID))
	historyDir := filepath.Join(dir, "history")
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		return err
	}
	history := filepath.Join(historyDir, fmt.Sprintf("%020d.json", journal.Sequence))
	if err := writeImmutableOrEqual(history, raw); err != nil {
		return err
	}
	return writeAtomic(journalPath(root, journal.OperationID), raw, 0o600)
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

func acquireMutationLeases(ctx context.Context, root string, clock func() time.Time) (func(), error) {
	// The mutation order is retention, autonomous execution, Git administration,
	// then child publication. Retention is the only waiting exclusive acquire;
	// every inner acquire is a probe, but every successful file remains held for
	// the full GC transaction.
	dir := filepath.Join(root, ".revolvr", "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	held := make([]*os.File, 0, 4)
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			releaseFlock(held[i])
		}
	}
	retention, err := openFlock(ctx, filepath.Join(root, filepath.FromSlash(lock.ArtifactRetentionRelPath)), true)
	if err != nil {
		return nil, err
	}
	held = append(held, retention)
	autonomous, err := openFlock(ctx, filepath.Join(dir, "autonomous-execution.lock"), false)
	if err != nil {
		release()
		return nil, errors.New("artifact GC apply: autonomous execution is active")
	}
	held = append(held, autonomous)
	for _, name := range []string{"git-admin.lock", "child-publication.lock"} {
		lease, lockErr := openFlock(ctx, filepath.Join(dir, name), false)
		if lockErr != nil {
			release()
			return nil, fmt.Errorf("artifact GC apply: active %s", name)
		}
		held = append(held, lease)
	}
	if err := rejectActiveSourceWriters(context.WithoutCancel(ctx), root, clock().UTC()); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func rejectActiveSourceWriters(ctx context.Context, root string, now time.Time) error {
	if metadata, found, err := lock.ReadSourceWriter(ctx, root); err != nil {
		return err
	} else if found && metadata.ExpiresAt.After(now) {
		return fmt.Errorf("artifact GC apply: control-root source writer %q is active", metadata.RunID)
	}
	dir := filepath.Join(root, ".revolvr", "locks", "workspaces")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return errors.New("artifact GC apply: malformed workspace source-writer namespace")
		}
		metadata, found, err := lock.ReadWorkspaceSourceWriter(ctx, root, entry.Name())
		if err != nil {
			return err
		}
		if found && metadata.ExpiresAt.After(now) {
			return fmt.Errorf("artifact GC apply: workspace source writer %q is active", metadata.RunID)
		}
	}
	return nil
}

func openFlock(ctx context.Context, path string, wait bool) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, err
		}
		if !wait {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func releaseFlock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
