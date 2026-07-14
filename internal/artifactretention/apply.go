package artifactretention

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	RepositoryRoot  string
	LedgerPath      string
	Plan            Plan
	Secrets         []string
	Clock           func() time.Time
	FailureInjector ApplyFailureInjector
}
type ApplyResult struct {
	Journal   Journal
	Replayed  bool
	Resumable bool
}

type ApplyFailurePoint string

const (
	FailureAfterPruneRename          ApplyFailurePoint = "after_prune_rename"
	FailureAfterPruneSourceSync      ApplyFailurePoint = "after_prune_source_sync"
	FailureAfterPruneDestinationSync ApplyFailurePoint = "after_prune_destination_sync"
	FailureAfterActionJournal        ApplyFailurePoint = "after_action_journal"
	FailureAfterCompletedJournal     ApplyFailurePoint = "after_completed_journal"
	FailureAfterQuarantineRemoval    ApplyFailurePoint = "after_quarantine_removal"
	FailureAfterQuarantineParentSync ApplyFailurePoint = "after_quarantine_parent_sync"
	FailureAfterCleanedJournal       ApplyFailurePoint = "after_cleaned_journal"
)

type ApplyFailureInjector func(ApplyFailurePoint, string) error

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
	leases, err := acquireMutationLeases(ctx, root, in.Clock)
	if err != nil {
		return ApplyResult{}, err
	}
	defer leases.Close()
	journal, found, err := loadJournal(root, in.Plan.OperationID)
	if err != nil {
		return ApplyResult{}, err
	}
	exportVerified := false
	if found {
		if journal.Plan.PlanID != in.Plan.PlanID || journal.Plan.PlanSHA256 != in.Plan.PlanSHA256 {
			return ApplyResult{}, errors.New("artifact GC apply: operation conflicts with admitted plan")
		}
		if in.Plan.RequiredExport && journal.ExportID != "" {
			if err := verifyJournalExport(ctx, root, journal, in.Secrets); err != nil {
				return ApplyResult{}, err
			}
			exportVerified = true
		}
		if err := reconcileCompletedEffects(ctx, root, journal); err != nil {
			return ApplyResult{}, err
		}
		if journal.Stage == stageCleaned {
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
		journal = Journal{SchemaVersion: JournalSchema, OperationID: in.Plan.OperationID, Stage: stageAdmitted, Plan: in.Plan, UpdatedAt: in.Clock().UTC()}
		if err := leases.Check(); err != nil {
			return ApplyResult{}, err
		}
		if err := persistJournal(root, Journal{}, &journal); err != nil {
			return ApplyResult{}, err
		}
	}
	completed := map[string]bool{}
	for _, path := range journal.CompletedPaths {
		completed[path] = true
	}
	if in.Plan.RequiredExport && journal.ExportID == "" {
		if err := ctx.Err(); err != nil {
			if leaseErr := leases.Check(); leaseErr != nil {
				return ApplyResult{}, errors.Join(err, leaseErr)
			}
			return cancelJournal(root, &journal, in.Clock, err)
		}
		if err := leases.Check(); err != nil {
			return ApplyResult{}, err
		}
		exported, err := ledgerexport.Export(ctx, ledgerexport.ExportInput{RepositoryRoot: root, LedgerPath: in.LedgerPath, OperationID: in.Plan.OperationID, ExportedAt: in.Plan.FrozenAt, PolicySHA256: in.Plan.PolicySHA256, Bounds: ledgerexport.Bounds{ThroughEventID: in.Plan.Ledger.HighWaterEventID}, Secrets: in.Secrets})
		if err != nil {
			return ApplyResult{}, err
		}
		prior := cloneJournal(journal)
		journal.ExportID = exported.Manifest.ExportID
		journal.Stage = stageExportVerified
		journal.Cancelled = false
		journal.UpdatedAt = in.Clock().UTC()
		if err := verifyJournalExport(ctx, root, journal, in.Secrets); err != nil {
			return ApplyResult{}, err
		}
		exportVerified = true
		if err := leases.Check(); err != nil {
			return ApplyResult{}, err
		}
		if err := persistJournal(root, prior, &journal); err != nil {
			return ApplyResult{}, err
		}
	}
	for _, action := range in.Plan.Actions {
		if action.Kind != ActionCompress && action.Kind != ActionPrune || completed[action.Path] {
			continue
		}
		if err := ctx.Err(); err != nil {
			if leaseErr := leases.Check(); leaseErr != nil {
				return ApplyResult{}, errors.Join(err, leaseErr)
			}
			return cancelJournal(root, &journal, in.Clock, err)
		}
		if err := revalidateActionAuthority(ctx, root, in.LedgerPath, in.Plan, action); err != nil {
			return ApplyResult{}, err
		}
		if err := leases.Check(); err != nil {
			return ApplyResult{}, err
		}
		switch action.Kind {
		case ActionCompress:
			err = applyCompression(ctx, root, action)
		case ActionPrune:
			if in.Plan.RequiredExport && !exportVerified {
				err = errors.New("artifact GC apply: prune lacks verified export")
			} else {
				err = applyPrune(root, in.Plan.OperationID, action, in.FailureInjector)
			}
		}
		if err != nil {
			return ApplyResult{}, fmt.Errorf("artifact GC apply: %s %s: %w", action.Kind, action.Path, err)
		}
		prior := cloneJournal(journal)
		journal.CompletedPaths = append(journal.CompletedPaths, action.Path)
		completed[action.Path] = true
		journal.Stage = string(action.Kind) + "_applied"
		journal.Cancelled = false
		journal.UpdatedAt = in.Clock().UTC()
		if err := leases.Check(); err != nil {
			return ApplyResult{}, err
		}
		if err := persistJournal(root, prior, &journal); err != nil {
			return ApplyResult{}, err
		}
		if err := injectApplyFailure(in.FailureInjector, FailureAfterActionJournal, action.Path); err != nil {
			return ApplyResult{Journal: journal, Resumable: true}, err
		}
	}
	if journal.Stage != stageCompleted {
		prior := cloneJournal(journal)
		journal.Stage = stageCompleted
		journal.Cancelled = false
		journal.UpdatedAt = in.Clock().UTC()
		if err := leases.Check(); err != nil {
			return ApplyResult{}, err
		}
		if err := persistJournal(root, prior, &journal); err != nil {
			return ApplyResult{}, err
		}
		if err := injectApplyFailure(in.FailureInjector, FailureAfterCompletedJournal, journal.OperationID); err != nil {
			return ApplyResult{Journal: journal, Resumable: true}, err
		}
	}
	operationDir := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(in.Plan.OperationID))
	quarantine := filepath.Join(operationDir, "quarantine")
	if err := leases.Check(); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
	}
	if err := injectApplyFailure(in.FailureInjector, FailureAfterQuarantineRemoval, quarantine); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
	}
	if err := syncDir(operationDir); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
	}
	if err := injectApplyFailure(in.FailureInjector, FailureAfterQuarantineParentSync, operationDir); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
	}
	prior := cloneJournal(journal)
	journal.Stage = stageCleaned
	journal.UpdatedAt = in.Clock().UTC()
	if err := leases.Check(); err != nil {
		return ApplyResult{}, err
	}
	if err := persistJournal(root, prior, &journal); err != nil {
		return ApplyResult{}, err
	}
	if err := injectApplyFailure(in.FailureInjector, FailureAfterCleanedJournal, journal.OperationID); err != nil {
		return ApplyResult{Journal: journal, Resumable: true}, err
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
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return Journal{}, false, err
	}
	journal, found, err := loadJournal(root, operationID)
	if err != nil || !found {
		return journal, found, err
	}
	if journal.Plan.RequiredExport && journal.ExportID != "" {
		if err := verifyJournalExport(context.Background(), root, journal, nil); err != nil {
			return Journal{}, false, err
		}
	}
	if err := reconcileCompletedEffects(context.Background(), root, journal); err != nil {
		return Journal{}, false, err
	}
	return journal, true, nil
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

func applyPrune(root, operationID string, action Action, inject ApplyFailureInjector) error {
	abs := filepath.Join(root, filepath.FromSlash(action.Path))
	operationDir := filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(operationID))
	base := filepath.Join(operationDir, "quarantine", filepath.FromSlash(action.Path))
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
	quarantined := false
	for _, p := range paths {
		if _, err := os.Lstat(p.src); err == nil {
			if _, err := os.Lstat(p.dst); err == nil {
				return errors.New("quarantine collision")
			}
			if err := os.Rename(p.src, p.dst); err != nil {
				return err
			}
			quarantined = true
			if err := injectApplyFailure(inject, FailureAfterPruneRename, p.dst); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else if _, err := os.Lstat(p.dst); err == nil {
			quarantined = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !quarantined {
		return errors.New("prune source and quarantine are missing")
	}
	if err := syncDir(filepath.Dir(abs)); err != nil {
		return err
	}
	if err := injectApplyFailure(inject, FailureAfterPruneSourceSync, filepath.Dir(abs)); err != nil {
		return err
	}
	return syncDestinationChain(filepath.Dir(base), operationDir, inject)
}

func syncDestinationChain(leaf, stop string, inject ApplyFailureInjector) error {
	relative, err := filepath.Rel(stop, leaf)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.Join(err, errors.New("prune destination escapes its operation directory"))
	}
	for dir := filepath.Clean(leaf); ; dir = filepath.Dir(dir) {
		if err := syncDir(dir); err != nil {
			return err
		}
		if err := injectApplyFailure(inject, FailureAfterPruneDestinationSync, dir); err != nil {
			return err
		}
		if dir == stop {
			return nil
		}
	}
}

func injectApplyFailure(inject ApplyFailureInjector, point ApplyFailurePoint, path string) error {
	if inject == nil {
		return nil
	}
	return inject(point, path)
}

func cancelJournal(root string, journal *Journal, clock func() time.Time, cause error) (ApplyResult, error) {
	prior := cloneJournal(*journal)
	journal.Cancelled = true
	journal.Stage = stageCancelled
	journal.UpdatedAt = clock().UTC()
	persistErr := persistJournal(root, prior, journal)
	return ApplyResult{Journal: *journal, Resumable: true}, errors.Join(cause, persistErr)
}

func journalPath(root, id string) string {
	return filepath.Join(root, ".revolvr", "retention", "gc", safeOperationDir(id), "journal.json")
}
func safeOperationDir(id string) string { return hash([]byte(strings.TrimSpace(id))) }

type mutationLeases struct {
	retention *lock.Flock
	inner     []*lock.Flock
}

func (l *mutationLeases) Check() error {
	if l == nil || l.retention == nil {
		return errors.New("artifact GC apply: retention lease is missing")
	}
	if err := l.retention.Check(); err != nil {
		return fmt.Errorf("artifact GC apply: retention lease identity changed: %w", err)
	}
	for _, lease := range l.inner {
		if err := lease.Check(); err != nil {
			return fmt.Errorf("artifact GC apply: coordination lease identity changed: %w", err)
		}
	}
	return nil
}

func (l *mutationLeases) Close() {
	if l == nil {
		return
	}
	for i := len(l.inner) - 1; i >= 0; i-- {
		_ = l.inner[i].Close()
	}
	l.inner = nil
	if l.retention != nil {
		_ = l.retention.Close()
		l.retention = nil
	}
}

func acquireMutationLeases(ctx context.Context, root string, clock func() time.Time) (*mutationLeases, error) {
	// The mutation order is retention, autonomous execution, Git administration,
	// then child publication. Retention is the only waiting exclusive acquire;
	// every inner acquire is a probe, but every successful file remains held for
	// the full GC transaction.
	retention, err := lock.AcquireFlock(ctx, root, lock.FlockConfig{
		RelativePath: lock.ArtifactRetentionRelPath,
		Mode:         lock.FlockExclusive,
		Wait:         true,
		Create:       true,
	})
	if err != nil {
		return nil, err
	}
	leases := &mutationLeases{retention: retention, inner: make([]*lock.Flock, 0, 3)}
	autonomous, err := lock.AcquireFlock(ctx, root, lock.FlockConfig{
		RelativePath: ".revolvr/locks/autonomous-execution.lock",
		Mode:         lock.FlockExclusive,
		Wait:         false,
		Create:       true,
	})
	if err != nil {
		leases.Close()
		if errors.Is(err, lock.ErrFlockContended) {
			return nil, errors.New("artifact GC apply: autonomous execution is active")
		}
		return nil, err
	}
	leases.inner = append(leases.inner, autonomous)
	for _, name := range []string{"git-admin.lock", "child-publication.lock"} {
		lease, lockErr := lock.AcquireFlock(ctx, root, lock.FlockConfig{
			RelativePath: filepath.ToSlash(filepath.Join(".revolvr", "locks", name)),
			Mode:         lock.FlockExclusive,
			Wait:         false,
			Create:       true,
		})
		if lockErr != nil {
			leases.Close()
			if errors.Is(lockErr, lock.ErrFlockContended) {
				return nil, fmt.Errorf("artifact GC apply: active %s", name)
			}
			return nil, lockErr
		}
		leases.inner = append(leases.inner, lease)
	}
	if err := leases.Check(); err != nil {
		leases.Close()
		return nil, err
	}
	if err := rejectActiveSourceWriters(context.WithoutCancel(ctx), root, clock().UTC()); err != nil {
		leases.Close()
		return nil, err
	}
	if err := leases.Check(); err != nil {
		leases.Close()
		return nil, err
	}
	return leases, nil
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
