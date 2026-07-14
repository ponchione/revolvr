package gitstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"revolvr/internal/runner"
)

const (
	SourceSnapshotSchemaVersion       = "revolvr-source-snapshot-v1"
	PolicySourceRevisionSchemaVersion = "revolvr-policy-source-revision-v1"
)

var ErrPolicyRelevantIgnored = errors.New("policy-relevant ignored source or verification inputs are present")

type SourceSnapshotConfig struct {
	WorkingDir          string
	GitExecutable       string
	Timeout             time.Duration
	StdoutCap           int
	StderrCap           int
	AllowHarnessRuntime bool
	CommandRunner       CommandRunner
}

type ignoredSourceEntry struct {
	Path           string
	FileType       string
	Classification string
}

type SourceEntry struct {
	Path        string `json:"path"`
	IndexRecord string `json:"index_record,omitempty"`
	FileType    string `json:"file_type"`
	Mode        uint32 `json:"mode,omitempty"`
	ByteSize    int64  `json:"byte_size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type SourceSnapshot struct {
	SchemaVersion  string        `json:"schema_version"`
	Head           string        `json:"head"`
	IndexSHA256    string        `json:"index_sha256"`
	WorktreeSHA256 string        `json:"worktree_sha256"`
	SnapshotSHA256 string        `json:"snapshot_sha256"`
	Entries        []SourceEntry `json:"entries"`
}

type SourcePathChange struct {
	Path   string       `json:"path"`
	Before *SourceEntry `json:"before,omitempty"`
	After  *SourceEntry `json:"after,omitempty"`
}

type SourceDifference struct {
	Changed         bool               `json:"changed"`
	HeadChanged     bool               `json:"head_changed"`
	IndexChanged    bool               `json:"index_changed"`
	WorktreeChanged bool               `json:"worktree_changed"`
	BeforeSHA256    string             `json:"before_sha256"`
	AfterSHA256     string             `json:"after_sha256"`
	PathChanges     []SourcePathChange `json:"path_changes"`
}

type policySourceEntry struct {
	Path     string `json:"path"`
	FileType string `json:"file_type"`
	Mode     uint32 `json:"mode,omitempty"`
	ByteSize int64  `json:"byte_size,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
}

// PolicySourceRevision returns the content-tree identity used by autonomous
// routing and verification freshness. Full SourceSnapshot equality remains
// authoritative for race and mutation detection. This projection deliberately
// excludes HEAD and index placement so staging or committing the same verified
// worktree bytes does not make verification stale. Missing tracked entries are
// also omitted: their absence changes the revision when compared with a
// snapshot where the path existed, while remaining stable after the deletion
// is staged and committed.
func PolicySourceRevision(snapshot SourceSnapshot) (string, error) {
	if err := snapshot.Validate(); err != nil {
		return "", fmt.Errorf("policy source revision: %w", err)
	}
	entries := make([]policySourceEntry, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.FileType == "missing" {
			continue
		}
		entries = append(entries, policySourceEntry{
			Path: entry.Path, FileType: entry.FileType, Mode: policySourceMode(entry),
			ByteSize: entry.ByteSize, SHA256: entry.SHA256,
		})
	}
	raw, err := json.Marshal(struct {
		SchemaVersion string              `json:"schema_version"`
		Entries       []policySourceEntry `json:"entries"`
	}{PolicySourceRevisionSchemaVersion, entries})
	if err != nil {
		return "", fmt.Errorf("policy source revision: marshal projection: %w", err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

func policySourceMode(entry SourceEntry) uint32 {
	if entry.FileType != "regular" {
		return 0
	}
	return entry.Mode & 0o111
}

// CaptureSourceSnapshot captures content-sensitive repository evidence. It
// includes every tracked path plus every non-ignored untracked path. Ignored
// paths fail closed unless the caller explicitly identifies ignored .revolvr
// state in the control repository as harness-owned runtime state.
func CaptureSourceSnapshot(ctx context.Context, cfg SourceSnapshotConfig) (SourceSnapshot, error) {
	cfg, workDir, err := normalizeSourceSnapshotConfig(cfg)
	if err != nil {
		return SourceSnapshot{}, err
	}

	headRaw, err := runSnapshotGit(ctx, cfg, workDir, []string{"rev-parse", "--verify", "HEAD"})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("capture source snapshot HEAD: %w", err)
	}
	head := strings.TrimSpace(headRaw)
	if head == "" || strings.ContainsAny(head, "\r\n") {
		return SourceSnapshot{}, errors.New("capture source snapshot HEAD: output is empty or malformed")
	}

	indexRaw, err := runSnapshotGit(ctx, cfg, workDir, []string{"ls-files", "--stage", "-z"})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("capture source snapshot index: %w", err)
	}
	index, err := parseIndexRecords(indexRaw)
	if err != nil {
		return SourceSnapshot{}, err
	}

	pathsRaw, err := runSnapshotGit(ctx, cfg, workDir, []string{"ls-files", "--cached", "--others", "--exclude-standard", "-z"})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("capture source snapshot paths: %w", err)
	}
	paths, err := parseSnapshotPaths(pathsRaw)
	if err != nil {
		return SourceSnapshot{}, err
	}
	ignoredRaw, err := runSnapshotGit(ctx, cfg, workDir, []string{"ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z", "--"})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("capture source snapshot ignored paths: %w", err)
	}
	ignored, err := inventoryIgnoredSource(workDir, ignoredRaw, cfg.AllowHarnessRuntime)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if err := rejectPolicyRelevantIgnored(ignored); err != nil {
		return SourceSnapshot{}, err
	}
	for path := range index {
		paths[path] = struct{}{}
	}

	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	entries := make([]SourceEntry, 0, len(ordered))
	for _, path := range ordered {
		entry, err := captureSourceEntry(workDir, path, index[path])
		if err != nil {
			return SourceSnapshot{}, fmt.Errorf("capture source snapshot path %q: %w", path, err)
		}
		entries = append(entries, entry)
	}

	indexHash := sha256.New()
	for _, entry := range entries {
		if entry.IndexRecord == "" {
			continue
		}
		_, _ = io.WriteString(indexHash, entry.Path)
		_, _ = indexHash.Write([]byte{0})
		_, _ = io.WriteString(indexHash, entry.IndexRecord)
		_, _ = indexHash.Write([]byte{0})
	}
	worktreeRaw, err := json.Marshal(entries)
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("capture source snapshot: marshal worktree entries: %w", err)
	}
	worktreeHash := sha256.Sum256(worktreeRaw)

	snapshot := SourceSnapshot{
		SchemaVersion:  SourceSnapshotSchemaVersion,
		Head:           head,
		IndexSHA256:    fmt.Sprintf("%x", indexHash.Sum(nil)),
		WorktreeSHA256: fmt.Sprintf("%x", worktreeHash),
		Entries:        entries,
	}
	snapshotRaw, err := json.Marshal(struct {
		SchemaVersion  string        `json:"schema_version"`
		Head           string        `json:"head"`
		IndexSHA256    string        `json:"index_sha256"`
		WorktreeSHA256 string        `json:"worktree_sha256"`
		Entries        []SourceEntry `json:"entries"`
	}{snapshot.SchemaVersion, snapshot.Head, snapshot.IndexSHA256, snapshot.WorktreeSHA256, snapshot.Entries})
	if err != nil {
		return SourceSnapshot{}, fmt.Errorf("capture source snapshot: marshal snapshot: %w", err)
	}
	snapshotHash := sha256.Sum256(snapshotRaw)
	snapshot.SnapshotSHA256 = fmt.Sprintf("%x", snapshotHash)
	return snapshot, nil
}

func CompareSourceSnapshots(before, after SourceSnapshot) SourceDifference {
	difference := SourceDifference{
		HeadChanged:     before.Head != after.Head,
		IndexChanged:    before.IndexSHA256 != after.IndexSHA256,
		WorktreeChanged: before.WorktreeSHA256 != after.WorktreeSHA256,
		BeforeSHA256:    before.SnapshotSHA256,
		AfterSHA256:     after.SnapshotSHA256,
	}
	beforeEntries := make(map[string]SourceEntry, len(before.Entries))
	afterEntries := make(map[string]SourceEntry, len(after.Entries))
	paths := make(map[string]struct{}, len(before.Entries)+len(after.Entries))
	for _, entry := range before.Entries {
		beforeEntries[entry.Path] = entry
		paths[entry.Path] = struct{}{}
	}
	for _, entry := range after.Entries {
		afterEntries[entry.Path] = entry
		paths[entry.Path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		beforeEntry, beforeOK := beforeEntries[path]
		afterEntry, afterOK := afterEntries[path]
		if beforeOK && afterOK && beforeEntry == afterEntry {
			continue
		}
		change := SourcePathChange{Path: path}
		if beforeOK {
			entry := beforeEntry
			change.Before = &entry
		}
		if afterOK {
			entry := afterEntry
			change.After = &entry
		}
		difference.PathChanges = append(difference.PathChanges, change)
	}
	difference.Changed = difference.HeadChanged || difference.IndexChanged || difference.WorktreeChanged || len(difference.PathChanges) > 0
	return difference
}

func (snapshot SourceSnapshot) Validate() error {
	if snapshot.SchemaVersion != SourceSnapshotSchemaVersion {
		return fmt.Errorf("validate source snapshot: unsupported schema %q", snapshot.SchemaVersion)
	}
	if strings.TrimSpace(snapshot.Head) == "" || strings.ContainsAny(snapshot.Head, "\r\n") {
		return errors.New("validate source snapshot: HEAD is missing or malformed")
	}
	hashes := []struct {
		name  string
		value string
	}{
		{name: "index", value: snapshot.IndexSHA256},
		{name: "worktree", value: snapshot.WorktreeSHA256},
		{name: "snapshot", value: snapshot.SnapshotSHA256},
	}
	for _, hash := range hashes {
		if !validSnapshotHash(hash.value) {
			return fmt.Errorf("validate source snapshot: %s SHA-256 is invalid", hash.name)
		}
	}
	previousPath := ""
	for i, entry := range snapshot.Entries {
		if entry.Path == "" || filepath.IsAbs(entry.Path) || strings.HasPrefix(entry.Path, "../") {
			return fmt.Errorf("validate source snapshot: entries[%d] has unsafe path %q", i, entry.Path)
		}
		if previousPath != "" && entry.Path <= previousPath {
			return fmt.Errorf("validate source snapshot: entries are not uniquely sorted at %q", entry.Path)
		}
		previousPath = entry.Path
		switch entry.FileType {
		case "regular", "symlink":
			if !validSnapshotHash(entry.SHA256) {
				return fmt.Errorf("validate source snapshot: entries[%d] content SHA-256 is invalid", i)
			}
		case "missing", "directory":
			if entry.SHA256 != "" {
				return fmt.Errorf("validate source snapshot: entries[%d] file type %q must not have a content SHA-256", i, entry.FileType)
			}
		default:
			return fmt.Errorf("validate source snapshot: entries[%d] has unknown file type %q", i, entry.FileType)
		}
	}

	indexHash := sha256.New()
	for _, entry := range snapshot.Entries {
		if entry.IndexRecord == "" {
			continue
		}
		_, _ = io.WriteString(indexHash, entry.Path)
		_, _ = indexHash.Write([]byte{0})
		_, _ = io.WriteString(indexHash, entry.IndexRecord)
		_, _ = indexHash.Write([]byte{0})
	}
	if got := fmt.Sprintf("%x", indexHash.Sum(nil)); got != snapshot.IndexSHA256 {
		return errors.New("validate source snapshot: index SHA-256 does not match entries")
	}
	worktreeRaw, err := json.Marshal(snapshot.Entries)
	if err != nil {
		return fmt.Errorf("validate source snapshot: marshal entries: %w", err)
	}
	worktreeHash := sha256.Sum256(worktreeRaw)
	if got := fmt.Sprintf("%x", worktreeHash); got != snapshot.WorktreeSHA256 {
		return errors.New("validate source snapshot: worktree SHA-256 does not match entries")
	}
	snapshotRaw, err := json.Marshal(struct {
		SchemaVersion  string        `json:"schema_version"`
		Head           string        `json:"head"`
		IndexSHA256    string        `json:"index_sha256"`
		WorktreeSHA256 string        `json:"worktree_sha256"`
		Entries        []SourceEntry `json:"entries"`
	}{snapshot.SchemaVersion, snapshot.Head, snapshot.IndexSHA256, snapshot.WorktreeSHA256, snapshot.Entries})
	if err != nil {
		return fmt.Errorf("validate source snapshot: marshal snapshot: %w", err)
	}
	snapshotHash := sha256.Sum256(snapshotRaw)
	if got := fmt.Sprintf("%x", snapshotHash); got != snapshot.SnapshotSHA256 {
		return errors.New("validate source snapshot: snapshot SHA-256 does not match content")
	}
	return nil
}

func normalizeSourceSnapshotConfig(cfg SourceSnapshotConfig) (SourceSnapshotConfig, string, error) {
	workDir := strings.TrimSpace(cfg.WorkingDir)
	if workDir == "" {
		return SourceSnapshotConfig{}, "", errors.New("capture source snapshot: working directory is required")
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return SourceSnapshotConfig{}, "", fmt.Errorf("capture source snapshot: resolve working directory: %w", err)
	}
	cfg.GitExecutable = strings.TrimSpace(cfg.GitExecutable)
	if cfg.GitExecutable == "" {
		cfg.GitExecutable = defaultGitExecutable
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.StdoutCap <= 0 {
		cfg.StdoutCap = 8 * 1024 * 1024
	}
	if cfg.StderrCap <= 0 {
		cfg.StderrCap = 256 * 1024
	}
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = runner.Run
	}
	return cfg, abs, nil
}

func runSnapshotGit(ctx context.Context, cfg SourceSnapshotConfig, workDir string, args []string) (string, error) {
	result := cfg.CommandRunner(ctx, runner.Command{
		Name:        cfg.GitExecutable,
		Args:        append([]string(nil), args...),
		Dir:         workDir,
		Timeout:     cfg.Timeout,
		StdoutLimit: cfg.StdoutCap,
		StderrLimit: cfg.StderrCap,
	})
	if result.TimedOut {
		return "", fmt.Errorf("git %s timed out after %s", strings.Join(args, " "), cfg.Timeout)
	}
	if result.Err != nil {
		return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), result.Err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("git %s exited with code %d: %s", strings.Join(args, " "), result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	if result.StdoutTruncatedBytes > 0 || result.StderrTruncatedBytes > 0 {
		return "", fmt.Errorf("git %s output was truncated (stdout=%d bytes, stderr=%d bytes)", strings.Join(args, " "), result.StdoutTruncatedBytes, result.StderrTruncatedBytes)
	}
	return result.Stdout, nil
}

func parseIndexRecords(raw string) (map[string]string, error) {
	result := make(map[string]string)
	for _, record := range strings.Split(raw, "\x00") {
		if record == "" {
			continue
		}
		tab := strings.IndexByte(record, '\t')
		if tab < 1 || tab == len(record)-1 {
			return nil, errors.New("capture source snapshot index: malformed git ls-files record")
		}
		path := record[tab+1:]
		if _, exists := result[path]; exists {
			return nil, fmt.Errorf("capture source snapshot index: duplicate path %q", path)
		}
		result[path] = record[:tab]
	}
	return result, nil
}

func parseSnapshotPaths(raw string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	for _, path := range strings.Split(raw, "\x00") {
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") {
			return nil, fmt.Errorf("capture source snapshot paths: unsafe path %q", path)
		}
		result[path] = struct{}{}
	}
	return result, nil
}

func inventoryIgnoredSource(workDir, raw string, allowHarnessRuntime bool) ([]ignoredSourceEntry, error) {
	seen := make(map[string]struct{})
	entries := make([]ignoredSourceEntry, 0)
	for _, rawPath := range strings.Split(raw, "\x00") {
		if rawPath == "" {
			continue
		}
		path := filepath.ToSlash(strings.TrimSuffix(rawPath, "/"))
		if path == "" || path == "." || filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") {
			return nil, fmt.Errorf("capture source snapshot ignored paths: unsafe path %q", rawPath)
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Lstat(filepath.Join(workDir, filepath.FromSlash(path)))
		if err != nil {
			return nil, fmt.Errorf("capture source snapshot ignored path %q: %w", path, err)
		}
		fileType := sourceFileType(info.Mode())
		classification := "policy_relevant"
		if allowHarnessRuntime && isRuntimeArtifactPath(path) && fileType != "symlink" && fileType != "other" {
			classification = "harness_runtime"
		}
		entries = append(entries, ignoredSourceEntry{Path: path, FileType: fileType, Classification: classification})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func rejectPolicyRelevantIgnored(entries []ignoredSourceEntry) error {
	var descriptions []string
	for _, entry := range entries {
		if entry.Classification == "harness_runtime" {
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("%q (%s, classification=%s)", entry.Path, entry.FileType, entry.Classification))
	}
	if len(descriptions) == 0 {
		return nil
	}
	return fmt.Errorf("capture source snapshot: %w: %s", ErrPolicyRelevantIgnored, strings.Join(descriptions, ", "))
}

func sourceFileType(mode os.FileMode) string {
	switch {
	case mode.IsRegular():
		return "regular"
	case mode&os.ModeSymlink != 0:
		return "symlink"
	case mode.IsDir():
		return "directory"
	default:
		return "other"
	}
}

func captureSourceEntry(workDir, path, indexRecord string) (SourceEntry, error) {
	entry := SourceEntry{Path: path, IndexRecord: indexRecord}
	absPath := filepath.Join(workDir, filepath.FromSlash(path))
	before, err := os.Lstat(absPath)
	if errors.Is(err, os.ErrNotExist) {
		entry.FileType = "missing"
		return entry, nil
	}
	if err != nil {
		return SourceEntry{}, err
	}
	entry.Mode = uint32(before.Mode())
	entry.ByteSize = before.Size()

	switch {
	case before.Mode().IsRegular():
		entry.FileType = "regular"
		file, err := os.Open(absPath)
		if err != nil {
			return SourceEntry{}, err
		}
		hash := sha256.New()
		readSize, readErr := io.Copy(hash, file)
		closeErr := file.Close()
		if readErr != nil {
			return SourceEntry{}, readErr
		}
		if closeErr != nil {
			return SourceEntry{}, closeErr
		}
		after, err := os.Lstat(absPath)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() || before.Mode() != after.Mode() {
			return SourceEntry{}, errors.New("file changed while it was being captured")
		}
		entry.ByteSize = readSize
		entry.SHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	case before.Mode()&os.ModeSymlink != 0:
		entry.FileType = "symlink"
		target, err := os.Readlink(absPath)
		if err != nil {
			return SourceEntry{}, err
		}
		hash := sha256.Sum256([]byte(target))
		entry.ByteSize = int64(len(target))
		entry.SHA256 = fmt.Sprintf("%x", hash)
	case before.IsDir():
		entry.FileType = "directory"
	default:
		return SourceEntry{}, fmt.Errorf("unsupported file type %s", before.Mode().Type())
	}
	return entry, nil
}

func isRuntimeArtifactPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == ".revolvr" || strings.HasPrefix(clean, ".revolvr/")
}

func validSnapshotHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}
