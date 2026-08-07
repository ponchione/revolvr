package retrieval

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
	storage "revolvr/internal/storage/postgres"
)

func TestPostgresExactFTSStructuralAndPgvectorRetrieval(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := storage.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID := uuid.NewString()
	now := time.Date(2026, 8, 7, 15, 0, 0, 0, time.UTC)
	projectUUID := pgtype.UUID{Bytes: uuid.MustParse(projectID), Valid: true}
	stamp := pgtype.Timestamptz{Time: now, Valid: true}
	if _, err := storage.New(pool).InsertProject(ctx, storage.InsertProjectParams{ID: projectUUID, Name: "retrieval-" + projectID, Status: "active", CreatedAt: stamp, UpdatedAt: stamp}); err != nil {
		t.Fatal(err)
	}
	model := codeindex.SelectedEmbeddingEvidence().Model
	model.Revision = "retrieval-fixture"
	model.ArtifactSHA256 = strings.Repeat("a", 64)
	space, _ := model.SpaceIdentity()
	evidence := codeindex.ModelEvidence{Model: model, SpaceSHA256: space.SHA256, License: "test-only", SourceURI: "https://example.invalid/retrieval", ServingImageDigest: "sha256:" + strings.Repeat("b", 64)}
	embedder := &semanticFixtureEmbedder{model: model}
	snapshot := codeindex.Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("c", 40), SourceTree: strings.Repeat("d", 40), Files: []codeindex.File{
		{Path: "internal/provider/router.go", Content: []byte("package provider\n// RouteProvider selects provider routing configuration.\nfunc RouteProvider() { PersistToolExecution() }\n")},
		{Path: "internal/provider/storage.go", Content: []byte("package provider\nfunc PersistToolExecution() {}\n")},
		{Path: "internal/workspace/cleanup.go", Content: []byte("package workspace\nfunc CleanupWorkspace() {}\n")},
	}}
	build, err := codeindex.Prepare(ctx, codeindex.PrepareRequest{OperationID: "retrieval-" + projectID, Kind: codeindex.BuildFull, Snapshot: snapshot, EmbeddingSpace: &evidence, Embedder: embedder, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := codeindex.NewPostgresStore(pool)
	if _, err := store.Stage(ctx, build); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(ctx, build.OperationID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	source, _ := NewPostgresSource(pool)
	retriever, _ := New(source)
	result, err := retriever.Retrieve(ctx, Request{
		ProjectID: projectID, SourceRevision: snapshot.SourceRevision,
		ExactPaths: []string{"internal/provider/router.go"}, ExactSymbols: []string{"RouteProvider"},
		ExactText: "provider routing configuration", Query: "provider routing",
		ExpectedSpaceSHA256: space.SHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256, Embedder: embedder, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) < 2 || result.Candidates[0].Path != "internal/provider/router.go" || !result.Candidates[0].Signals.ExactPath || laneState(result.Report, "fts") != LaneUsed || laneState(result.Report, "vector") != LaneUsed {
		t.Fatalf("hybrid result = %#v", result)
	}
	structural, err := retriever.Retrieve(ctx, Request{ProjectID: projectID, SourceRevision: snapshot.SourceRevision, ExactSymbols: []string{"RouteProvider"}, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	foundNeighbor := false
	for _, candidate := range structural.Candidates {
		if candidate.Symbol == "PersistToolExecution" && candidate.Signals.Structural {
			foundNeighbor = true
		}
	}
	if !foundNeighbor {
		t.Fatalf("structural one-hop result = %#v", structural.Candidates)
	}
}

type semanticFixtureEmbedder struct{ model embedding.EmbeddingModelInfo }

func (e *semanticFixtureEmbedder) ModelInfo(context.Context) (embedding.EmbeddingModelInfo, error) {
	return e.model, nil
}
func (e *semanticFixtureEmbedder) EmbedDocuments(_ context.Context, input []string) (embedding.EmbeddingBatch, error) {
	space, _ := e.model.SpaceIdentity()
	values := make([][]float32, len(input))
	for index, text := range input {
		values[index] = semanticVector(e.model.Dimensions, text)
	}
	return embedding.EmbeddingBatch{Status: embedding.ServiceStatus{Mode: embedding.ServiceReady}, Space: space, Values: values}, nil
}
func (e *semanticFixtureEmbedder) EmbedQuery(_ context.Context, input string) (embedding.Embedding, error) {
	space, _ := e.model.SpaceIdentity()
	return embedding.Embedding{Status: embedding.ServiceStatus{Mode: embedding.ServiceReady}, Space: space, Value: semanticVector(e.model.Dimensions, input)}, nil
}
func semanticVector(dimensions int, text string) []float32 {
	value := make([]float32, dimensions)
	if strings.Contains(strings.ToLower(text), "provider") || strings.Contains(strings.ToLower(text), "routing") {
		value[0] = 1
	} else {
		value[1] = 1
	}
	return value
}
