package index

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"revolvr/internal/embedding"
)

func TestParseFileSupportedLanguagesAndFallback(t *testing.T) {
	tests := []struct {
		path     string
		content  string
		language string
		symbol   string
		mode     string
	}{
		{"main.go", "package fixture\nfunc RouteProvider() { PersistTool() }\n", "go", "RouteProvider", "go-ast"},
		{"router.ts", "export function routeProvider() { persistTool(); }\n", "typescript", "routeProvider", "javascript-declaration"},
		{"worker.js", "export const cleanupWorkspace = () => closeLease();\n", "javascript", "cleanupWorkspace", "javascript-declaration"},
		{"tokens.py", "def expired_token():\n    return verify_token()\n", "python", "expired_token", "python-indent"},
		{"architecture.md", "# Storage backend removal\nLanceDB was removed.\n", "markdown", "Storage backend removal", "markdown-heading"},
		{"schema.sql", "CREATE TABLE tool_executions (id uuid);\n", "sql", "tool_executions", "sql-statement"},
		{"broken.go", "func {\n", "go", "", "fallback"},
		{"notes.txt", "unsupported but exact\n", "text", "", "fallback"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			first, err := ParseFile(uuid.NewString(), File{Path: test.path, Content: []byte(test.content)}, DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			actualMode := first.Chunks[0].StructuralProvenance.Mode
			if test.mode == "fallback" {
				actualMode = first.StructuralProvenance.Mode
			}
			if first.Language != test.language || len(first.Chunks) == 0 || actualMode != test.mode {
				t.Fatalf("parsed file = %#v", first)
			}
			if test.symbol != "" && first.Chunks[0].Symbol != test.symbol {
				t.Fatalf("symbol = %q, want %q", first.Chunks[0].Symbol, test.symbol)
			}
			for _, chunk := range first.Chunks {
				if len(chunk.Body) > DefaultLimits().MaxChunkBytes || chunk.StartLine < 1 || chunk.EndLine < chunk.StartLine || SHA256([]byte(chunk.Body)) != chunk.BodySHA256 {
					t.Fatalf("unbounded or divergent chunk = %#v", chunk)
				}
				if !strings.Contains(chunk.EmbeddingText(), "path: "+test.path) || strings.Contains(chunk.EmbeddingText(), "semantic description") {
					t.Fatalf("embedding input is not exact authoritative content: %q", chunk.EmbeddingText())
				}
			}
		})
	}
}

func TestParseFileDeterministicBoundedMalformedInput(t *testing.T) {
	projectID := uuid.NewString()
	content := []byte("unterminated = '" + strings.Repeat("x", 3000))
	limits := Limits{MaxChunkBytes: 256, MaxChunkLines: 3}
	first, err := ParseFile(projectID, File{Path: "broken.sql", Content: content}, limits)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseFile(projectID, File{Path: "broken.sql", Content: content}, limits)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic parse = %#v, %#v, %v", first, second, err)
	}
	if len(first.Chunks) < 2 || first.StructuralProvenance.Mode != "fallback" {
		t.Fatalf("malformed bounded fallback = %#v", first)
	}
	seen := map[string]struct{}{}
	for _, chunk := range first.Chunks {
		if len(chunk.Body) > 256 {
			t.Fatalf("chunk bytes = %d", len(chunk.Body))
		}
		if _, duplicate := seen[chunk.ID]; duplicate {
			t.Fatalf("duplicate chunk identity %s", chunk.ID)
		}
		seen[chunk.ID] = struct{}{}
	}
}

func TestPrepareIncrementalReuseDeletionRenameOutageAndDrift(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.NewString()
	model := SelectedEmbeddingEvidence().Model
	evidence := testEvidence(t, model)
	embedder := &testEmbedder{model: model}
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	base := Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("1", 40), SourceTree: strings.Repeat("2", 40), Files: []File{
		{Path: "a.go", Content: []byte("package fixture\nfunc Alpha() {}\n")},
		{Path: "deleted.go", Content: []byte("package fixture\nfunc Deleted() {}\n")},
	}}
	full, err := Prepare(ctx, PrepareRequest{OperationID: "full-one", Kind: BuildFull, Snapshot: base, EmbeddingSpace: &evidence, Embedder: embedder, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if full.Manifest.VectorCount != full.Manifest.ChunkCount || embedder.documentInputs.Load() != int32(full.Manifest.ChunkCount) {
		t.Fatalf("full manifest/calls = %#v/%d", full.Manifest, embedder.documentInputs.Load())
	}
	existing := existingFromBuild(full)
	embedder.documentInputs.Store(0)
	next := Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("3", 40), SourceTree: strings.Repeat("4", 40), Files: []File{
		{Path: "a.go", Content: append([]byte(nil), base.Files[0].Content...)},
		{Path: "renamed.go", Content: append([]byte(nil), base.Files[1].Content...)},
	}}
	incremental, err := Prepare(ctx, PrepareRequest{OperationID: "incremental-one", Kind: BuildIncremental, Snapshot: next, Existing: existing, EmbeddingSpace: &evidence, Embedder: embedder, Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if !incremental.Files[0].Reused || incremental.Files[1].Reused || incremental.Files[1].Path != "renamed.go" || embedder.documentInputs.Load() != int32(len(incremental.Files[1].Chunks)) {
		t.Fatalf("incremental reuse/rename = %#v, embedded=%d", incremental.Files, embedder.documentInputs.Load())
	}
	for _, file := range incremental.Files {
		if file.Path == "deleted.go" {
			t.Fatal("deleted file remained in replacement build")
		}
	}

	outage := &testEmbedder{model: model, err: errors.New("offline")}
	if _, err := Prepare(ctx, PrepareRequest{OperationID: "outage", Kind: BuildRebuild, Snapshot: next, EmbeddingSpace: &evidence, Embedder: outage, Now: now}); err == nil {
		t.Fatal("embedding outage returned nil error")
	} else {
		var buildErr *BuildError
		if !errors.As(err, &buildErr) || buildErr.Status.Mode != embedding.ServiceDegraded {
			t.Fatalf("outage error = %#v", err)
		}
	}
	driftModel := model
	driftModel.Revision = "revision-two"
	if _, err := Prepare(ctx, PrepareRequest{OperationID: "drift", Kind: BuildSpaceSwitch, Snapshot: next, EmbeddingSpace: &evidence, Embedder: &testEmbedder{model: driftModel}, Now: now}); err == nil {
		t.Fatal("metadata drift returned nil error")
	}
}

func TestSelectedEmbeddingEvidenceIsExactAndMeasured(t *testing.T) {
	evidence := SelectedEmbeddingEvidence()
	if err := evidence.Validate(); err != nil {
		t.Fatal(err)
	}
	if evidence.Model.ModelName != SelectedEmbeddingModelName || evidence.Model.Dimensions != 1024 || SHA256([]byte(SelectedQueryInstruction)) != SelectedQueryInstructionSHA256 {
		t.Fatalf("selected evidence = %#v", evidence)
	}
}

func TestSelectedEmbeddingModelRejectsPriorAndCandidateRepresentations(t *testing.T) {
	wrongName := SelectedEmbeddingEvidence().Model
	wrongName.ModelName = "unsupported/model"
	wrongDimension := SelectedEmbeddingEvidence().Model
	wrongDimension.Dimensions--
	wrongPooling := SelectedEmbeddingEvidence().Model
	wrongPooling.Pooling = "mean"
	for _, model := range []embedding.EmbeddingModelInfo{wrongName, wrongDimension, wrongPooling} {
		if err := ValidateSelectedEmbeddingModel(model); err == nil {
			t.Fatalf("non-selected model accepted: %#v", model)
		}
	}
}

type testEmbedder struct {
	model          embedding.EmbeddingModelInfo
	err            error
	documentInputs atomic.Int32
}

func (e *testEmbedder) ModelInfo(context.Context) (embedding.EmbeddingModelInfo, error) {
	return e.model, e.err
}

func (e *testEmbedder) EmbedDocuments(_ context.Context, input []string) (embedding.EmbeddingBatch, error) {
	if e.err != nil {
		return embedding.EmbeddingBatch{}, e.err
	}
	e.documentInputs.Add(int32(len(input)))
	space, _ := e.model.SpaceIdentity()
	values := make([][]float32, len(input))
	for i := range input {
		values[i] = make([]float32, e.model.Dimensions)
		values[i][i%e.model.Dimensions] = 1
	}
	return embedding.EmbeddingBatch{Status: embedding.ServiceStatus{Mode: embedding.ServiceReady}, Space: space, Values: values}, nil
}

func (e *testEmbedder) EmbedQuery(context.Context, string) (embedding.Embedding, error) {
	space, _ := e.model.SpaceIdentity()
	return embedding.Embedding{Status: embedding.ServiceStatus{Mode: embedding.ServiceReady}, Space: space, Value: make([]float32, e.model.Dimensions)}, e.err
}

func testEvidence(t *testing.T, model embedding.EmbeddingModelInfo) ModelEvidence {
	t.Helper()
	space, err := model.SpaceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return ModelEvidence{Model: model, SpaceSHA256: space.SHA256, License: "test-only", SourceURI: "https://example.invalid/model", ServingImageDigest: "sha256:" + strings.Repeat("b", 64)}
}

func existingFromBuild(build PreparedBuild) map[string]ExistingFile {
	result := make(map[string]ExistingFile, len(build.Files))
	for _, file := range build.Files {
		vectors := map[string][]float32{}
		for _, chunk := range file.Chunks {
			vectors[chunk.ID] = append([]float32(nil), build.Vectors[chunk.ID]...)
		}
		result[file.Path] = ExistingFile{Parsed: file, SpaceSHA256: build.Manifest.EmbeddingSpace, Vectors: vectors}
	}
	return result
}
