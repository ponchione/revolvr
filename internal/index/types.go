// Package index builds reproducible, revision-bound derived code indexes.
// It has no task lifecycle, verification, completion, audit, or correction
// authority.
package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"revolvr/internal/embedding"
	"revolvr/internal/gitoid"
)

const (
	ManifestSchemaVersion = "revolvr-code-index-manifest-v1"
	ParserVersion         = "revolvr-structural-chunker-v1"
	EmbeddingInputVersion = "revolvr-code-embedding-input-v1"
)

type State string

const (
	StateNeverIndexed State = "never_indexed"
	StateClean        State = "clean"
	StateDirty        State = "dirty"
	StateBuilding     State = "building"
	StateFailed       State = "failed"
)

type BuildKind string

const (
	BuildFull        BuildKind = "full"
	BuildIncremental BuildKind = "incremental"
	BuildRebuild     BuildKind = "rebuild"
	BuildSpaceSwitch BuildKind = "space_switch"
)

type Limits struct {
	MaxFiles      int
	MaxFileBytes  int
	MaxChunkBytes int
	MaxChunkLines int
}

func DefaultLimits() Limits {
	return Limits{MaxFiles: 100_000, MaxFileBytes: 4 << 20, MaxChunkBytes: 32 << 10, MaxChunkLines: 240}
}

func (l Limits) normalized() (Limits, error) {
	defaults := DefaultLimits()
	if l.MaxFiles == 0 {
		l.MaxFiles = defaults.MaxFiles
	}
	if l.MaxFileBytes == 0 {
		l.MaxFileBytes = defaults.MaxFileBytes
	}
	if l.MaxChunkBytes == 0 {
		l.MaxChunkBytes = defaults.MaxChunkBytes
	}
	if l.MaxChunkLines == 0 {
		l.MaxChunkLines = defaults.MaxChunkLines
	}
	if l.MaxFiles < 1 || l.MaxFiles > 1_000_000 || l.MaxFileBytes < 1 || l.MaxFileBytes > 64<<20 || l.MaxChunkBytes < 256 || l.MaxChunkBytes > 128<<10 || l.MaxChunkLines < 1 || l.MaxChunkLines > 2000 {
		return Limits{}, errors.New("code index: invalid resource limits")
	}
	return l, nil
}

type File struct {
	Path    string
	Content []byte
}

type Snapshot struct {
	ProjectID      string
	SourceRevision string
	SourceTree     string
	Files          []File
}

func (s Snapshot) Validate(limits Limits) error {
	if _, err := uuid.Parse(s.ProjectID); err != nil {
		return errors.New("code index: project ID must be a UUID")
	}
	if !gitoid.Valid(s.SourceRevision) || !gitoid.Valid(s.SourceTree) {
		return errors.New("code index: source revision and tree must be lowercase Git object IDs")
	}
	if len(s.Files) > limits.MaxFiles {
		return fmt.Errorf("code index: file count %d exceeds limit %d", len(s.Files), limits.MaxFiles)
	}
	previous := ""
	for _, file := range s.Files {
		path := filepath.ToSlash(file.Path)
		if path == "" || path != file.Path || filepath.IsAbs(path) || filepath.Clean(path) != path || path == ".." || strings.HasPrefix(path, "../") || strings.ContainsRune(path, '\x00') {
			return fmt.Errorf("code index: invalid admitted path %q", file.Path)
		}
		if previous != "" && path <= previous {
			return errors.New("code index: admitted files must be strictly path ordered")
		}
		if len(file.Content) > limits.MaxFileBytes {
			return fmt.Errorf("code index: %s exceeds the per-file byte limit", path)
		}
		if !utf8.Valid(file.Content) || strings.IndexByte(string(file.Content), 0) >= 0 {
			return fmt.Errorf("code index: %s is not bounded UTF-8 text", path)
		}
		previous = path
	}
	return nil
}

type StructuralProvenance struct {
	ParserVersion string `json:"parser_version"`
	Parser        string `json:"parser"`
	Mode          string `json:"mode"`
	Reason        string `json:"reason,omitempty"`
}

type Chunk struct {
	ID                   string               `json:"id"`
	DocumentID           string               `json:"document_id"`
	DocumentVersionID    string               `json:"document_version_id"`
	Ordinal              int                  `json:"ordinal"`
	Path                 string               `json:"path"`
	Language             string               `json:"language"`
	Kind                 string               `json:"kind"`
	Symbol               string               `json:"symbol,omitempty"`
	Signature            string               `json:"signature"`
	StartLine            int                  `json:"start_line"`
	EndLine              int                  `json:"end_line"`
	Body                 string               `json:"body"`
	BodySHA256           string               `json:"body_sha256"`
	StructuralProvenance StructuralProvenance `json:"structural_provenance"`
}

// EmbeddingText contains only authoritative indexed fields. The build's exact
// Git revision remains mandatory provenance around the unit, but is not fed as
// semantically meaningless text; the content hash is the unit-level immutable
// source revision and permits unchanged chunks to reuse exact embeddings.
func (c Chunk) EmbeddingText() string {
	return fmt.Sprintf("schema: %s\npath: %s\nsymbol: %s\nsignature: %s\nlanguage: %s\nlines: %d-%d\nsource-content-sha256: %s\nstructural-parser: %s\nstructural-mode: %s\ncontent:\n%s",
		EmbeddingInputVersion, c.Path, c.Symbol, c.Signature, c.Language, c.StartLine, c.EndLine,
		c.BodySHA256, c.StructuralProvenance.Parser, c.StructuralProvenance.Mode, c.Body)
}

type Symbol struct {
	ID                string `json:"id"`
	DocumentVersionID string `json:"document_version_id"`
	ChunkID           string `json:"chunk_id"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	Signature         string `json:"signature"`
	StartLine         int    `json:"start_line"`
	EndLine           int    `json:"end_line"`
}

type Edge struct {
	ID                string               `json:"id"`
	DocumentVersionID string               `json:"document_version_id"`
	FromSymbolID      string               `json:"from_symbol_id,omitempty"`
	Kind              string               `json:"kind"`
	TargetSymbol      string               `json:"target_symbol"`
	TargetPath        string               `json:"target_path,omitempty"`
	SourceLine        int                  `json:"source_line"`
	Provenance        StructuralProvenance `json:"provenance"`
}

type ParsedFile struct {
	Path                 string               `json:"path"`
	Language             string               `json:"language"`
	ContentSHA256        string               `json:"content_sha256"`
	SizeBytes            int                  `json:"size_bytes"`
	DocumentID           string               `json:"document_id"`
	DocumentVersionID    string               `json:"document_version_id"`
	StructuralProvenance StructuralProvenance `json:"structural_provenance"`
	Chunks               []Chunk              `json:"chunks"`
	Symbols              []Symbol             `json:"symbols"`
	Edges                []Edge               `json:"edges"`
	Reused               bool                 `json:"reused"`
}

type ModelEvidence struct {
	Model              embedding.EmbeddingModelInfo `json:"model"`
	SpaceSHA256        string                       `json:"space_sha256"`
	License            string                       `json:"license"`
	SourceURI          string                       `json:"source_uri"`
	ServingImageDigest string                       `json:"serving_image_digest"`
}

func (m ModelEvidence) Validate() error {
	if err := ValidateSelectedEmbeddingModel(m.Model); err != nil {
		return err
	}
	space, err := m.Model.SpaceIdentity()
	if err != nil {
		return err
	}
	if m.SpaceSHA256 != space.SHA256 || strings.TrimSpace(m.License) == "" || strings.TrimSpace(m.SourceURI) == "" || !strings.HasPrefix(m.ServingImageDigest, "sha256:") || len(m.ServingImageDigest) != 71 {
		return errors.New("code index: incomplete or divergent embedding-space evidence")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(m.ServingImageDigest, "sha256:")); err != nil {
		return errors.New("code index: serving image digest must be sha256")
	}
	return nil
}

type Manifest struct {
	SchemaVersion  string         `json:"schema_version"`
	ProjectID      string         `json:"project_id"`
	SourceRevision string         `json:"source_revision"`
	SourceTree     string         `json:"source_tree"`
	ParserVersion  string         `json:"parser_version"`
	EmbeddingSpace string         `json:"embedding_space,omitempty"`
	Files          []ManifestFile `json:"files"`
	FileCount      int            `json:"file_count"`
	ChunkCount     int            `json:"chunk_count"`
	SymbolCount    int            `json:"symbol_count"`
	VectorCount    int            `json:"vector_count"`
	SHA256         string         `json:"sha256"`
}

type ManifestFile struct {
	Path          string `json:"path"`
	Language      string `json:"language"`
	ContentSHA256 string `json:"content_sha256"`
	SizeBytes     int    `json:"size_bytes"`
	ChunkCount    int    `json:"chunk_count"`
	SymbolCount   int    `json:"symbol_count"`
	Reused        bool   `json:"reused"`
}

type PreparedBuild struct {
	ID             string
	OperationID    string
	Kind           BuildKind
	Snapshot       Snapshot
	EmbeddingSpace *ModelEvidence
	Files          []ParsedFile
	Vectors        map[string][]float32
	Manifest       Manifest
	PreparedAt     time.Time
}

func (b *PreparedBuild) finalizeManifest() error {
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion, ProjectID: b.Snapshot.ProjectID,
		SourceRevision: b.Snapshot.SourceRevision, SourceTree: b.Snapshot.SourceTree,
		ParserVersion: ParserVersion, FileCount: len(b.Files),
	}
	if b.EmbeddingSpace != nil {
		manifest.EmbeddingSpace = b.EmbeddingSpace.SpaceSHA256
	}
	for _, file := range b.Files {
		manifest.Files = append(manifest.Files, ManifestFile{
			Path: file.Path, Language: file.Language, ContentSHA256: file.ContentSHA256,
			SizeBytes: file.SizeBytes, ChunkCount: len(file.Chunks), SymbolCount: len(file.Symbols), Reused: file.Reused,
		})
		manifest.ChunkCount += len(file.Chunks)
		manifest.SymbolCount += len(file.Symbols)
	}
	manifest.VectorCount = len(b.Vectors)
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifest.SHA256 = SHA256(raw)
	b.Manifest = manifest
	return nil
}

func SHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func DeterministicID(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(fmt.Sprintf("%d:", len(part))))
		_, _ = h.Write([]byte(part))
	}
	raw := h.Sum(nil)[:16]
	raw[6] = (raw[6] & 0x0f) | 0x50
	raw[8] = (raw[8] & 0x3f) | 0x80
	id, _ := uuid.FromBytes(raw)
	return id.String()
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".md", ".markdown":
		return "markdown"
	case ".sql":
		return "sql"
	default:
		return "text"
	}
}

func sortedFiles(files []ParsedFile) []ParsedFile {
	copyFiles := append([]ParsedFile(nil), files...)
	sort.Slice(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path })
	return copyFiles
}
