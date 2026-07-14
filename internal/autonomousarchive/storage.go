package autonomousarchive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/lock"
	"revolvr/internal/runtimepath"
	"revolvr/internal/taskfile"
)

type archiveStorage struct {
	boundary runtimepath.Boundary
	leases   []*lock.Flock
	inject   func(FailurePoint) error
}

func bindArchiveStorage(root string) (*archiveStorage, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	boundary, err := runtimepath.Bind(root)
	if err != nil {
		return nil, fmt.Errorf("archive: bind repository root: %w", err)
	}
	return &archiveStorage{boundary: boundary}, nil
}

func newArchiveStorage(boundary runtimepath.Boundary, inject func(FailurePoint) error, leases ...*lock.Flock) *archiveStorage {
	filtered := make([]*lock.Flock, 0, len(leases))
	for _, lease := range leases {
		if lease != nil {
			filtered = append(filtered, lease)
		}
	}
	return &archiveStorage{boundary: boundary, leases: filtered, inject: inject}
}

func (s *archiveStorage) root() string { return s.boundary.Root() }

func (s *archiveStorage) resolve(rel string) (string, error) {
	rel = filepath.Clean(filepath.FromSlash(strings.TrimSpace(rel)))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive: unsafe path %q", filepath.ToSlash(rel))
	}
	return filepath.Join(s.root(), rel), nil
}

func (s *archiveStorage) checkLeases() error {
	for _, lease := range s.leases {
		if err := lease.Check(); err != nil {
			return fmt.Errorf("archive: validate held lease: %w", err)
		}
	}
	return nil
}

func (s *archiveStorage) checkDirectory(directory *runtimepath.Directory) error {
	if err := directory.Check(); err != nil {
		return err
	}
	return s.checkLeases()
}

func (s *archiveStorage) fail(point FailurePoint) error {
	if s.inject == nil {
		return nil
	}
	return s.inject(point)
}

func (s *archiveStorage) openParent(rel string, create bool) (*runtimepath.Directory, string, error) {
	abs, err := s.resolve(rel)
	if err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(abs)
	if create {
		if err := s.checkLeases(); err != nil {
			return nil, "", err
		}
		if err := s.boundary.EnsureDir(parent, 0o755); err != nil {
			return nil, "", err
		}
		if err := s.checkLeases(); err != nil {
			return nil, "", err
		}
	}
	directory, found, err := s.boundary.OpenDir(parent, false)
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "", os.ErrNotExist
	}
	if err := s.checkDirectory(directory); err != nil {
		_ = directory.Close()
		return nil, "", err
	}
	return directory, filepath.Base(abs), nil
}

func (s *archiveStorage) readRegular(rel string, missingOK bool) ([]byte, bool, error) {
	directory, name, err := s.openParent(rel, false)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer directory.Close()
	if err := s.fail(FailureAfterStorageDirectoryOpen); err != nil {
		return nil, false, err
	}
	return s.readDirectoryFile(directory, name, missingOK)
}

func (s *archiveStorage) readDirectoryFile(directory *runtimepath.Directory, name string, missingOK bool) ([]byte, bool, error) {
	file, err := directory.OpenFile(name, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	if err := s.fail(FailureAfterStorageReadOpen); err != nil {
		return nil, false, err
	}
	if err := file.Check(); err != nil {
		return nil, false, err
	}
	if err := s.checkDirectory(directory); err != nil {
		return nil, false, err
	}
	raw, err := file.ReadAll()
	if err != nil {
		return nil, false, err
	}
	if err := s.checkDirectory(directory); err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s *archiveStorage) readArtifact(identity Artifact) ([]byte, error) {
	raw, found, err := s.readRegular(identity.Path, false)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	if artifact(identity.Path, raw) != identity {
		return nil, fmt.Errorf("archive task: artifact %s identity mismatch", identity.Path)
	}
	return raw, nil
}

func (s *archiveStorage) writeImmutable(identity Artifact, raw []byte) (err error) {
	if got := artifact(identity.Path, raw); got != identity {
		return errors.New("archive: immutable write identity does not match bytes")
	}
	if existing, found, readErr := s.readRegular(identity.Path, true); readErr != nil {
		return readErr
	} else if found {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("archive: immutable path %q already exists with different bytes", identity.Path)
	}
	directory, name, err := s.openParent(identity.Path, true)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := s.fail(FailureAfterStorageDirectoryOpen); err != nil {
		return err
	}
	if existing, found, readErr := s.readDirectoryFile(directory, name, true); readErr != nil {
		return readErr
	} else if found {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("archive: immutable path %q already exists with different bytes", identity.Path)
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	temp, err := directory.CreateTemp(".archive.tmp-", 0o644)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(directory, temp))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := s.fail(FailureAfterStorageOpen); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageFileSync); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStoragePublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	linkErr := directory.Link(temp, name)
	published = temp.IsNamed(name)
	if linkErr != nil {
		if errors.Is(linkErr, os.ErrExist) {
			existing, found, readErr := s.readDirectoryFile(directory, name, false)
			if readErr == nil && found && bytes.Equal(existing, raw) {
				return nil
			}
		}
		return linkErr
	}
	if err := s.fail(FailureAfterStoragePublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageDirectorySync); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageReadback); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	observed, found, err := s.readDirectoryFile(directory, name, false)
	if err != nil {
		return err
	}
	if !found || artifact(identity.Path, observed) != identity {
		return fmt.Errorf("archive: artifact %q identity mismatch", identity.Path)
	}
	return s.checkDirectory(directory)
}

func (s *archiveStorage) writeMutable(rel string, raw []byte) (err error) {
	directory, name, err := s.openParent(rel, true)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := s.fail(FailureAfterStorageDirectoryOpen); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	temp, err := directory.CreateTemp(".journal.tmp-", 0o644)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, s.cleanupTemp(directory, temp))
		}
		err = errors.Join(err, temp.Close())
	}()
	if err := s.fail(FailureAfterStorageOpen); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageFileSync); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStoragePublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	replaceErr := directory.Replace(temp, name)
	published = temp.IsNamed(name)
	if replaceErr != nil {
		return replaceErr
	}
	if err := s.fail(FailureAfterStoragePublish); err != nil {
		return err
	}
	if err := temp.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageDirectorySync); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageReadback); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	observed, found, err := s.readDirectoryFile(directory, name, false)
	if err != nil {
		return err
	}
	if !found || !bytes.Equal(observed, raw) {
		return errors.New("archive: mutable file strict readback failed")
	}
	return s.checkDirectory(directory)
}

func (s *archiveStorage) removeExact(identity Artifact) error {
	directory, name, err := s.openParent(identity.Path, false)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := s.fail(FailureAfterStorageDirectoryOpen); err != nil {
		return err
	}
	file, err := directory.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := s.fail(FailureAfterStorageReadOpen); err != nil {
		return err
	}
	raw, err := file.ReadAll()
	if err != nil {
		return err
	}
	if artifact(identity.Path, raw) != identity {
		return fmt.Errorf("archive: artifact %q identity mismatch", identity.Path)
	}
	if err := s.fail(FailureBeforeStorageRemove); err != nil {
		return err
	}
	if err := file.Check(); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := directory.Remove(file); err != nil {
		return err
	}
	if err := s.fail(FailureAfterStorageRemove); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageDirectorySync); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	return directory.Sync()
}

func (s *archiveStorage) cleanupTemp(directory *runtimepath.Directory, temp *runtimepath.File) error {
	faultErr := s.fail(FailureBeforeStorageCleanup)
	if err := temp.Check(); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := s.checkDirectory(directory); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := directory.Remove(temp); err != nil {
		return errors.Join(faultErr, err)
	}
	if err := s.checkDirectory(directory); err != nil {
		return errors.Join(faultErr, err)
	}
	return errors.Join(faultErr, directory.Sync())
}

// writeImmutable remains a package test/setup entry point. Production archive
// and reopen operations use one retained archiveStorage with their held leases.
func writeImmutable(root string, identity Artifact, raw []byte) error {
	storage, err := bindArchiveStorage(root)
	if err != nil {
		return err
	}
	return storage.writeImmutable(identity, raw)
}

func decodeCanonical(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	canonical, err := Marshal(target)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, canonical) {
		return errors.New("JSON is not canonical deterministic encoding")
	}
	return nil
}

func (s *archiveStorage) loadManifest(rel string) (Manifest, []byte, error) {
	raw, found, err := s.readRegular(rel, false)
	if err != nil {
		return Manifest{}, nil, err
	}
	if !found {
		return Manifest{}, nil, os.ErrNotExist
	}
	var manifest Manifest
	if err := decodeCanonical(raw, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("archive: decode manifest %s: %w", rel, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, nil, fmt.Errorf("archive: validate manifest %s: %w", rel, err)
	}
	return manifest, raw, nil
}

type archiveManifestCandidate struct {
	path string
	raw  []byte
}

func List(root string) ([]Entry, error) {
	storage, err := bindArchiveStorage(root)
	if err != nil {
		return nil, err
	}
	return storage.list()
}

func (s *archiveStorage) list() ([]Entry, error) {
	basePath, err := s.resolve(ArchiveRoot)
	if err != nil {
		return nil, err
	}
	if err := s.checkLeases(); err != nil {
		return nil, err
	}
	base, found, err := s.boundary.OpenDir(basePath, true)
	if err != nil || !found {
		return nil, err
	}
	defer base.Close()
	if err := s.fail(FailureAfterStorageDirectoryOpen); err != nil {
		return nil, err
	}
	if err := s.checkDirectory(base); err != nil {
		return nil, err
	}
	var candidates []archiveManifestCandidate
	if err := s.walkArchive(base, nil, &candidates); err != nil {
		return nil, err
	}
	if err := s.checkDirectory(base); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })
	entries := make([]Entry, 0, len(candidates))
	archiveIDs := map[string]string{}
	taskIDs := map[string]string{}
	for _, candidate := range candidates {
		var manifest Manifest
		if err := decodeCanonical(candidate.raw, &manifest); err != nil {
			return nil, fmt.Errorf("archive: decode manifest %s: %w", candidate.path, err)
		}
		if err := manifest.Validate(); err != nil {
			return nil, fmt.Errorf("archive: validate manifest %s: %w", candidate.path, err)
		}
		if previous := archiveIDs[manifest.ArchiveID]; previous != "" {
			return nil, fmt.Errorf("archive: duplicate archive id %q in %s and %s", manifest.ArchiveID, previous, candidate.path)
		}
		if previous := taskIDs[manifest.TaskID]; previous != "" {
			return nil, fmt.Errorf("archive: duplicate archived task id %q in %s and %s", manifest.TaskID, previous, candidate.path)
		}
		archiveIDs[manifest.ArchiveID] = candidate.path
		taskIDs[manifest.TaskID] = candidate.path
		entries = append(entries, Entry{Manifest: manifest, ManifestBytes: candidate.raw, ManifestPath: candidate.path})
	}
	return entries, nil
}

func (s *archiveStorage) walkArchive(directory *runtimepath.Directory, parts []string, candidates *[]archiveManifestCandidate) error {
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	if err := s.fail(FailureBeforeStorageEnumeration); err != nil {
		return err
	}
	if err := s.checkDirectory(directory); err != nil {
		return err
	}
	entries, err := directory.ReadDir()
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	if len(parts) < 3 {
		for _, entry := range entries {
			name := entry.Name()
			switch len(parts) {
			case 0:
				if len(name) != 4 || strings.Trim(name, "0123456789") != "" {
					return fmt.Errorf("archive: malformed UTC year directory %q", filepath.ToSlash(filepath.Join(append(parts, name)...)))
				}
			case 1:
				if name < "01" || name > "12" || len(name) != 2 {
					return fmt.Errorf("archive: malformed UTC month directory %q", filepath.ToSlash(filepath.Join(append(parts, name)...)))
				}
			case 2:
				if !validIdentity(name) {
					return fmt.Errorf("archive: malformed task directory %q", filepath.ToSlash(filepath.Join(append(parts, name)...)))
				}
			}
			child, found, err := directory.OpenDir(name, false)
			if err != nil || !found {
				return errors.Join(err, fmt.Errorf("archive: expected directory %q", name))
			}
			if err := s.fail(FailureAfterStorageDirectoryOpen); err != nil {
				_ = child.Close()
				return err
			}
			err = s.walkArchive(child, append(append([]string(nil), parts...), name), candidates)
			closeErr := child.Close()
			if err != nil || closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		return s.checkDirectory(directory)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name != "archive.json" && name != "task.md" && name != "completion.md" {
			return fmt.Errorf("archive: foreign file in canonical hierarchy: %s", filepath.ToSlash(filepath.Join(append(parts, name)...)))
		}
		raw, found, err := s.readDirectoryFile(directory, name, false)
		if err != nil || !found {
			return errors.Join(err, fmt.Errorf("archive: protected archive file %q is missing", name))
		}
		if name == "archive.json" {
			rel := filepath.ToSlash(filepath.Join(append([]string{ArchiveRoot}, append(parts, name)...)...))
			*candidates = append(*candidates, archiveManifestCandidate{path: rel, raw: raw})
		}
	}
	return s.checkDirectory(directory)
}

func Show(root, selector string) (Entry, error) {
	storage, err := bindArchiveStorage(root)
	if err != nil {
		return Entry{}, err
	}
	return storage.show(selector)
}

func (s *archiveStorage) show(selector string) (Entry, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Entry{}, errors.New("archive show: archive or task id is required")
	}
	entries, err := s.list()
	if err != nil {
		return Entry{}, err
	}
	var found *Entry
	for i := range entries {
		if entries[i].Manifest.ArchiveID != selector && entries[i].Manifest.TaskID != selector {
			continue
		}
		if found != nil {
			return Entry{}, fmt.Errorf("archive show: selector %q is ambiguous", selector)
		}
		copyValue := entries[i]
		found = &copyValue
	}
	if found == nil {
		return Entry{}, fmt.Errorf("archive show: %q not found", selector)
	}
	return *found, nil
}

// LoadEvidence performs strict Show loading plus exact archived task and
// terminal-state reads. It does not perform the separate full Verify operation.
func LoadEvidence(root, selector string) (EvidenceSnapshot, error) {
	storage, err := bindArchiveStorage(root)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	return storage.loadEvidence(selector)
}

func (s *archiveStorage) loadEvidence(selector string) (EvidenceSnapshot, error) {
	entry, err := s.show(selector)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	taskRaw, err := s.readArtifact(entry.Manifest.ArchivedTask)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("archive evidence: read archived task: %w", err)
	}
	task, err := taskfile.ParseArchivedTask(s.root(), entry.Manifest.OriginalTask.Path, taskRaw)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("archive evidence: parse archived task: %w", err)
	}
	if task.ID != entry.Manifest.TaskID || task.Workflow != taskfile.WorkflowAutonomousV1 {
		return EvidenceSnapshot{}, errors.New("archive evidence: archived task identity or workflow mismatch")
	}
	state, stateRaw, err := s.readState(entry.Manifest.State.Path, entry.Manifest.TaskID)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("archive evidence: load terminal state: %w", err)
	}
	if artifact(entry.Manifest.State.Path, stateRaw) != entry.Manifest.State {
		return EvidenceSnapshot{}, errors.New("archive evidence: terminal state identity mismatch")
	}
	result := EvidenceSnapshot{Entry: entry, Task: task, State: state, StateBytes: append([]byte(nil), stateRaw...)}
	if entry.Manifest.FrozenEvidence != nil {
		frozen, _, err := s.readFrozen(*entry.Manifest.FrozenEvidence)
		if err != nil {
			return EvidenceSnapshot{}, fmt.Errorf("archive evidence: load frozen completion evidence: %w", err)
		}
		result.Frozen = &frozen
	}
	return result, nil
}

func (s *archiveStorage) readState(rel, taskID string) (autonomous.ExecutionState, []byte, error) {
	raw, found, err := s.readRegular(rel, false)
	if err != nil {
		return autonomous.ExecutionState{}, nil, err
	}
	if !found {
		return autonomous.ExecutionState{}, nil, os.ErrNotExist
	}
	state, err := autonomousstate.DecodeState(raw, taskID)
	return state, raw, err
}

func readState(root, rel, taskID string) (autonomous.ExecutionState, []byte, error) {
	storage, err := bindArchiveStorage(root)
	if err != nil {
		return autonomous.ExecutionState{}, nil, err
	}
	return storage.readState(rel, taskID)
}

func (s *archiveStorage) readFrozen(identity Artifact) (autonomousfinalization.FrozenEvidence, []byte, error) {
	raw, err := s.readArtifact(identity)
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, nil, err
	}
	frozen, err := autonomousfinalization.DecodeFrozen(raw)
	if err != nil {
		return frozen, nil, err
	}
	return frozen, raw, nil
}

func readFrozen(root string, identity Artifact) (autonomousfinalization.FrozenEvidence, []byte, error) {
	storage, err := bindArchiveStorage(root)
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, nil, err
	}
	return storage.readFrozen(identity)
}

func acquireFileLock(ctx context.Context, boundary runtimepath.Boundary, rel string) (*lock.Flock, error) {
	if err := boundary.CheckDir(boundary.Root(), false); err != nil {
		return nil, err
	}
	lease, err := lock.AcquireFlock(ctx, boundary.Root(), lock.FlockConfig{
		RelativePath: rel,
		Mode:         lock.FlockExclusive,
		Wait:         true,
		Create:       true,
	})
	if err != nil {
		return nil, err
	}
	if err := lease.Check(); err != nil {
		_ = lease.Close()
		return nil, err
	}
	if err := boundary.CheckDir(boundary.Root(), false); err != nil {
		_ = lease.Close()
		return nil, err
	}
	return lease, nil
}

func operationHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
