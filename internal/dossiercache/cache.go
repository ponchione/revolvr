package dossiercache

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
	"reflect"
	"strings"
	"syscall"

	"revolvr/internal/gitoid"
)

const (
	SchemaVersion     = "revolvr-dossier-cache-v1"
	ProducerAlgorithm = "git-tree-path-map-v1"
	MaxEntryBytes     = 4 * 1024 * 1024
)

type ResultClass string

const (
	ResultHit         ResultClass = "hit"
	ResultMiss        ResultClass = "miss"
	ResultCorrupt     ResultClass = "corrupt"
	ResultUnsupported ResultClass = "unsupported"
	ResultRecomputed  ResultClass = "recomputed"
)

type GuidanceIdentity struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type Source struct {
	SchemaVersion   string             `json:"schema_version"`
	Algorithm       string             `json:"algorithm"`
	ControlRootID   string             `json:"control_root_id"`
	ExecutionRootID string             `json:"execution_root_id"`
	CommitSHA       string             `json:"commit_sha"`
	TreeSHA         string             `json:"tree_sha"`
	MaxPaths        int                `json:"max_paths"`
	MaxBytes        int                `json:"max_bytes"`
	Guidance        []GuidanceIdentity `json:"guidance"`
}

type Manifest struct {
	SchemaVersion  string `json:"schema_version"`
	Key            string `json:"key"`
	Source         Source `json:"source"`
	OutputSHA256   string `json:"output_sha256"`
	OutputByteSize int    `json:"output_byte_size"`
	TotalItems     int    `json:"total_items"`
	IncludedItems  int    `json:"included_items"`
	OmittedItems   int    `json:"omitted_items"`
	Truncated      bool   `json:"truncated"`
	TokenEstimator string `json:"token_estimator"`
	TokenEstimate  int    `json:"token_estimate"`
}

type Entry struct {
	Manifest Manifest
	Content  []byte
}

type LookupResult struct {
	Class      ResultClass
	Key        string
	Entry      Entry
	Diagnostic string
}

type Store struct {
	RepositoryRoot string
}

func RootIdentity(root string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(resolved)))
	return hex.EncodeToString(sum[:]), nil
}

func DeriveKey(source Source) (string, error) {
	if err := source.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s Source) Validate() error {
	if s.SchemaVersion != SchemaVersion || s.Algorithm != ProducerAlgorithm {
		return errors.New("dossier cache: unsupported source schema or algorithm")
	}
	rootIDs := []struct {
		name  string
		value string
	}{
		{name: "control_root_id", value: s.ControlRootID},
		{name: "execution_root_id", value: s.ExecutionRootID},
	}
	for _, rootID := range rootIDs {
		if !validSHA256(rootID.value) {
			return fmt.Errorf("dossier cache: %s must be SHA-256", rootID.name)
		}
	}
	if !validGitOID(s.CommitSHA) || !validGitOID(s.TreeSHA) {
		return errors.New("dossier cache: commit_sha and tree_sha must be 40- or 64-character lowercase Git object IDs")
	}
	if s.MaxPaths <= 0 || s.MaxPaths > 1_000_000 || s.MaxBytes <= 0 || s.MaxBytes > MaxEntryBytes {
		return errors.New("dossier cache: invalid repository map bounds")
	}
	previous := ""
	for _, item := range s.Guidance {
		if item.Path == "" || item.Path <= previous || filepath.IsAbs(item.Path) || filepath.Clean(item.Path) != item.Path || escapesRepositoryRoot(item.Path) || !validSHA256(item.SHA256) || item.ByteSize < 0 {
			return errors.New("dossier cache: guidance identities must be ordered, canonical, and exact")
		}
		previous = item.Path
	}
	return nil
}

func (m Manifest) Validate(content []byte) error {
	if m.SchemaVersion != SchemaVersion {
		return errors.New("unsupported_schema")
	}
	key, err := DeriveKey(m.Source)
	if err != nil || key != m.Key {
		return errors.New("key_source_mismatch")
	}
	if m.OutputByteSize != len(content) || m.OutputSHA256 != hash(content) {
		return errors.New("output_identity_mismatch")
	}
	if m.TotalItems < 0 || m.IncludedItems < 0 || m.OmittedItems < 0 || m.IncludedItems+m.OmittedItems != m.TotalItems || m.Truncated != (m.OmittedItems > 0) {
		return errors.New("item_count_mismatch")
	}
	if m.TokenEstimator != "utf8-bytes-ceil-div-4-v1" || m.TokenEstimate != estimateTokens(len(content)) {
		return errors.New("token_estimate_mismatch")
	}
	return nil
}

func NewEntry(source Source, content []byte, total, included int) (Entry, error) {
	key, err := DeriveKey(source)
	if err != nil {
		return Entry{}, err
	}
	if len(content) == 0 || len(content) > source.MaxBytes || included < 0 || total < included {
		return Entry{}, errors.New("dossier cache: invalid derived content or counts")
	}
	copyContent := append([]byte(nil), content...)
	entry := Entry{Content: copyContent, Manifest: Manifest{
		SchemaVersion: SchemaVersion, Key: key, Source: source,
		OutputSHA256: hash(copyContent), OutputByteSize: len(copyContent),
		TotalItems: total, IncludedItems: included, OmittedItems: total - included, Truncated: included < total,
		TokenEstimator: "utf8-bytes-ceil-div-4-v1", TokenEstimate: estimateTokens(len(copyContent)),
	}}
	return entry, entry.Manifest.Validate(entry.Content)
}

func MarshalManifest(manifest Manifest) ([]byte, error) { return marshalCanonical(manifest) }

func (s Store) Lookup(ctx context.Context, source Source) (LookupResult, error) {
	if err := ctx.Err(); err != nil {
		return LookupResult{}, err
	}
	key, err := DeriveKey(source)
	if err != nil {
		return LookupResult{}, err
	}
	dir, err := s.entryDir(key)
	if err != nil {
		return LookupResult{}, err
	}
	if code := validateParents(s.RepositoryRoot, filepath.Dir(dir)); code != "" {
		if code == "missing" {
			return LookupResult{Class: ResultMiss, Key: key}, nil
		}
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: "cache_parent_" + code}, nil
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return LookupResult{Class: ResultMiss, Key: key}, nil
	} else if err != nil {
		return LookupResult{}, err
	}
	manifestRaw, code := readStrict(filepath.Join(dir, "manifest.json"), MaxEntryBytes)
	if code != "" {
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: "manifest_" + code}, nil
	}
	content, code := readStrict(filepath.Join(dir, "repository-map.md"), MaxEntryBytes)
	if code != "" {
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: "content_" + code}, nil
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: "manifest_invalid_json"}, nil
	}
	canonical, err := marshalCanonical(manifest)
	if err != nil || !bytes.Equal(canonical, manifestRaw) {
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: "manifest_noncanonical"}, nil
	}
	if manifest.SchemaVersion != SchemaVersion {
		return LookupResult{Class: ResultUnsupported, Key: key, Diagnostic: "unsupported_schema"}, nil
	}
	if manifest.Key != key || !reflect.DeepEqual(manifest.Source, source) {
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: "source_mismatch"}, nil
	}
	if err := manifest.Validate(content); err != nil {
		return LookupResult{Class: ResultCorrupt, Key: key, Diagnostic: err.Error()}, nil
	}
	return LookupResult{Class: ResultHit, Key: key, Entry: Entry{Manifest: manifest, Content: append([]byte(nil), content...)}}, nil
}

func (s Store) Publish(ctx context.Context, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := entry.Manifest.Validate(entry.Content); err != nil {
		return fmt.Errorf("dossier cache publish: %w", err)
	}
	dir, err := s.entryDir(entry.Manifest.Key)
	if err != nil {
		return err
	}
	parent := filepath.Dir(dir)
	if err := secureMkdirAll(parent); err != nil {
		return err
	}
	if existing, err := s.Lookup(ctx, entry.Manifest.Source); err == nil && existing.Class == ResultHit {
		if bytes.Equal(existing.Entry.Content, entry.Content) && reflect.DeepEqual(existing.Entry.Manifest, entry.Manifest) {
			return nil
		}
		return errors.New("dossier cache publish: conflicting immutable entry")
	} else if err != nil {
		return err
	} else if existing.Class != ResultMiss {
		return fmt.Errorf("dossier cache publish: existing entry is %s (%s)", existing.Class, existing.Diagnostic)
	}
	tmp, err := os.MkdirTemp(parent, ".tmp-dossier-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	manifestRaw, err := marshalCanonical(entry.Manifest)
	if err != nil {
		return err
	}
	if err := syncFile(filepath.Join(tmp, "repository-map.md"), entry.Content); err != nil {
		return err
	}
	if err := syncFile(filepath.Join(tmp, "manifest.json"), manifestRaw); err != nil {
		return err
	}
	if err := syncDir(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if existing, lookupErr := s.Lookup(ctx, entry.Manifest.Source); lookupErr == nil && existing.Class == ResultHit && bytes.Equal(existing.Entry.Content, entry.Content) && reflect.DeepEqual(existing.Entry.Manifest, entry.Manifest) {
			return nil
		}
		return fmt.Errorf("dossier cache publish immutable entry: %w", err)
	}
	return syncDir(parent)
}

func (s Store) entryDir(key string) (string, error) {
	if !validSHA256(key) {
		return "", errors.New("dossier cache: invalid key")
	}
	root, err := filepath.Abs(strings.TrimSpace(s.RepositoryRoot))
	if err != nil || root == "" {
		return "", errors.New("dossier cache: repository root is required")
	}
	return filepath.Join(root, ".revolvr", "cache", "dossier", "v1", key), nil
}

func readStrict(path string, cap int64) ([]byte, string) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "missing"
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, "not_regular"
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, "unsafe_mode"
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		return nil, "hard_link"
	}
	if info.Size() < 0 || info.Size() > cap {
		return nil, "oversized"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "unreadable"
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, cap+1))
	if err != nil || int64(len(raw)) > cap {
		return nil, "unreadable"
	}
	return raw, ""
}

func secureMkdirAll(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	for current := path; current != filepath.Dir(current); current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("dossier cache: unsafe cache directory")
		}
		if filepath.Base(current) == ".revolvr" {
			break
		}
	}
	return nil
}

func validateParents(repositoryRoot, path string) string {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "invalid"
	}
	stop := filepath.Join(root, ".revolvr")
	current := path
	for {
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return "missing"
		}
		if err != nil {
			return "unreadable"
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "not_directory"
		}
		if info.Mode().Perm()&0o022 != 0 {
			return "unsafe_mode"
		}
		if current == stop {
			return ""
		}
		parent := filepath.Dir(current)
		if parent == current || !strings.HasPrefix(current, stop+string(filepath.Separator)) {
			return "escape"
		}
		current = parent
	}
}

func syncFile(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(content)
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func marshalCanonical(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validGitOID(value string) bool {
	return gitoid.Valid(value)
}

func hash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func estimateTokens(size int) int { return (size + 3) / 4 }
