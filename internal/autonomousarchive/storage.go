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
	"syscall"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/pathguard"
	"revolvr/internal/taskfile"
)

func canonicalRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("archive: resolve repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("archive: resolve repository root symlinks: %w", err)
	}
	return resolved, nil
}

func safePath(root, rel string) (string, error) {
	abs, err := pathguard.Resolve(root, filepath.FromSlash(rel))
	if err != nil {
		return "", fmt.Errorf("archive: unsafe path %q: %w", rel, err)
	}
	current := root
	for _, part := range strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("archive: path component %q is a symbolic link", part)
		}
	}
	return abs, nil
}

func ensureDirectories(root, rel string) error {
	clean := filepath.Clean(filepath.FromSlash(rel))
	current := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive: path component %q is not a non-symlink directory", part)
		}
	}
	return nil
}

func writeImmutable(root string, identity Artifact, raw []byte) error {
	if got := artifact(identity.Path, raw); got != identity {
		return errors.New("archive: immutable write identity does not match bytes")
	}
	abs, err := safePath(root, identity.Path)
	if err != nil {
		return err
	}
	if existing, err := readRegular(abs); err == nil {
		if bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("archive: immutable path %q already exists with different bytes", identity.Path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ensureDirectories(root, filepath.ToSlash(filepath.Dir(identity.Path))); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(abs), ".archive.tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// A same-directory hard link is an atomic no-overwrite publication. The
	// temporary name is immediately removed and target link count is checked.
	if err := os.Link(tempPath, abs); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, readErr := readRegular(abs)
			if readErr == nil && bytes.Equal(existing, raw) {
				return nil
			}
		}
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(abs)); err != nil {
		return err
	}
	return verifyArtifact(root, identity)
}

func writeMutable(root, rel string, raw []byte) error {
	abs, err := safePath(root, rel)
	if err != nil {
		return err
	}
	if err := ensureDirectories(root, filepath.ToSlash(filepath.Dir(rel))); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(abs), ".journal.tmp-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, abs); err != nil {
		return err
	}
	return syncDir(filepath.Dir(abs))
}

func verifyArtifact(root string, identity Artifact) error {
	abs, err := safePath(root, identity.Path)
	if err != nil {
		return err
	}
	raw, err := readRegular(abs)
	if err != nil {
		return err
	}
	if artifact(identity.Path, raw) != identity {
		return fmt.Errorf("archive: artifact %q identity mismatch", identity.Path)
	}
	return nil
}

func readRegular(abs string) ([]byte, error) {
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("archive: expected a non-symlink, non-group/world-writable regular file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink != 1 {
		return nil, errors.New("archive: regular file has an unsafe hard-link count")
	}
	return os.ReadFile(abs)
}

func removeExact(root string, identity Artifact) error {
	if err := verifyArtifact(root, identity); err != nil {
		return err
	}
	abs, _ := safePath(root, identity.Path)
	if err := os.Remove(abs); err != nil {
		return err
	}
	return syncDir(filepath.Dir(abs))
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
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

func loadManifest(root, rel string) (Manifest, []byte, error) {
	abs, err := safePath(root, rel)
	if err != nil {
		return Manifest{}, nil, err
	}
	raw, err := readRegular(abs)
	if err != nil {
		return Manifest{}, nil, err
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

func List(root string) ([]Entry, error) {
	canonical, err := canonicalRoot(root)
	if err != nil {
		return nil, err
	}
	base, err := safePath(canonical, ArchiveRoot)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(base); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var manifests []string
	err = filepath.WalkDir(base, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive: symbolic link in archive hierarchy: %s", current)
		}
		if entry.IsDir() {
			rel, _ := filepath.Rel(base, current)
			if rel == "." {
				return nil
			}
			parts := strings.Split(filepath.ToSlash(rel), "/")
			switch len(parts) {
			case 1:
				if len(parts[0]) != 4 || strings.Trim(parts[0], "0123456789") != "" {
					return fmt.Errorf("archive: malformed UTC year directory %q", rel)
				}
			case 2:
				if parts[1] < "01" || parts[1] > "12" || len(parts[1]) != 2 {
					return fmt.Errorf("archive: malformed UTC month directory %q", rel)
				}
			case 3:
				if !validIdentity(parts[2]) {
					return fmt.Errorf("archive: malformed task directory %q", rel)
				}
			default:
				return fmt.Errorf("archive: unexpected directory depth %q", rel)
			}
			return nil
		}
		rel, _ := filepath.Rel(canonical, current)
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		if len(parts) != 6 || parts[0] != ".agent" || parts[1] != "archive" || parts[5] != "archive.json" {
			if len(parts) == 6 && (parts[5] == "task.md" || parts[5] == "completion.md") {
				return nil
			}
			return fmt.Errorf("archive: foreign file in canonical hierarchy: %s", rel)
		}
		manifests = append(manifests, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(manifests)
	entries := make([]Entry, 0, len(manifests))
	archiveIDs := map[string]string{}
	taskIDs := map[string]string{}
	for _, rel := range manifests {
		manifest, raw, err := loadManifest(canonical, rel)
		if err != nil {
			return nil, err
		}
		if previous := archiveIDs[manifest.ArchiveID]; previous != "" {
			return nil, fmt.Errorf("archive: duplicate archive id %q in %s and %s", manifest.ArchiveID, previous, rel)
		}
		if previous := taskIDs[manifest.TaskID]; previous != "" {
			return nil, fmt.Errorf("archive: duplicate archived task id %q in %s and %s", manifest.TaskID, previous, rel)
		}
		archiveIDs[manifest.ArchiveID] = rel
		taskIDs[manifest.TaskID] = rel
		entries = append(entries, Entry{Manifest: manifest, ManifestBytes: raw, ManifestPath: rel})
	}
	return entries, nil
}

func Show(root, selector string) (Entry, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Entry{}, errors.New("archive show: archive or task id is required")
	}
	entries, err := List(root)
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
	canonical, err := canonicalRoot(root)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	entry, err := Show(canonical, selector)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	taskAbs, err := safePath(canonical, entry.Manifest.ArchivedTask.Path)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	taskRaw, err := readRegular(taskAbs)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("archive evidence: read archived task: %w", err)
	}
	if artifact(entry.Manifest.ArchivedTask.Path, taskRaw) != entry.Manifest.ArchivedTask {
		return EvidenceSnapshot{}, errors.New("archive evidence: archived task identity mismatch")
	}
	task, err := taskfile.ParseArchivedTask(canonical, entry.Manifest.OriginalTask.Path, taskRaw)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("archive evidence: parse archived task: %w", err)
	}
	if task.ID != entry.Manifest.TaskID || task.Workflow != taskfile.WorkflowAutonomousV1 {
		return EvidenceSnapshot{}, errors.New("archive evidence: archived task identity or workflow mismatch")
	}
	state, stateRaw, err := readState(canonical, entry.Manifest.State.Path, entry.Manifest.TaskID)
	if err != nil {
		return EvidenceSnapshot{}, fmt.Errorf("archive evidence: load terminal state: %w", err)
	}
	if artifact(entry.Manifest.State.Path, stateRaw) != entry.Manifest.State {
		return EvidenceSnapshot{}, errors.New("archive evidence: terminal state identity mismatch")
	}
	result := EvidenceSnapshot{Entry: entry, Task: task, State: state, StateBytes: append([]byte(nil), stateRaw...)}
	if entry.Manifest.FrozenEvidence != nil {
		frozen, _, err := readFrozen(canonical, *entry.Manifest.FrozenEvidence)
		if err != nil {
			return EvidenceSnapshot{}, fmt.Errorf("archive evidence: load frozen completion evidence: %w", err)
		}
		result.Frozen = &frozen
	}
	return result, nil
}

func readState(root, rel, taskID string) (autonomous.ExecutionState, []byte, error) {
	abs, err := safePath(root, rel)
	if err != nil {
		return autonomous.ExecutionState{}, nil, err
	}
	raw, err := readRegular(abs)
	if err != nil {
		return autonomous.ExecutionState{}, nil, err
	}
	state, err := autonomousstate.DecodeState(raw, taskID)
	return state, raw, err
}

func readFrozen(root string, identity Artifact) (autonomousfinalization.FrozenEvidence, []byte, error) {
	abs, err := safePath(root, identity.Path)
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, nil, err
	}
	raw, err := readRegular(abs)
	if err != nil {
		return autonomousfinalization.FrozenEvidence{}, nil, err
	}
	if artifact(identity.Path, raw) != identity {
		return autonomousfinalization.FrozenEvidence{}, nil, errors.New("archive: frozen evidence identity mismatch")
	}
	frozen, err := autonomousfinalization.DecodeFrozen(raw)
	if err != nil {
		return frozen, nil, err
	}
	return frozen, raw, nil
}

func acquireFileLock(ctx context.Context, root, rel string) (func(), error) {
	if err := ensureDirectories(root, filepath.ToSlash(filepath.Dir(rel))); err != nil {
		return nil, err
	}
	abs, err := safePath(root, rel)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func operationHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
