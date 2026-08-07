package index

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/embedding"
	"revolvr/internal/storage/postgres"
)

func TestPostgresEmptyIncrementalInterruptionAndAtomicSpaceSwitch(t *testing.T) {
	databaseURL := os.Getenv("REVOLVR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("REVOLVR_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID := uuid.NewString()
	now := time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)
	if _, err := postgres.New(pool).InsertProject(ctx, postgres.InsertProjectParams{ID: mustUUID(projectID), Name: "index-" + projectID, Status: "active", CreatedAt: timestamp(now), UpdatedAt: timestamp(now)}); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	var vectorIndexes, selectedVectorIndexes int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE indexname='retrieval_chunk_embeddings_1024_hnsw_idx') FROM pg_indexes WHERE schemaname='retrieval' AND indexname LIKE 'retrieval_chunk_embeddings_%_hnsw_idx'`).Scan(&vectorIndexes, &selectedVectorIndexes); err != nil {
		t.Fatal(err)
	}
	if vectorIndexes != 1 || selectedVectorIndexes != 1 {
		t.Fatalf("vector indexes = %d selected=%d", vectorIndexes, selectedVectorIndexes)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO retrieval.embedding_spaces (id, space_sha256, schema_version, model_name, model_revision, dimensions, pooling, normalization, quantization, artifact_sha256, license, source_uri, serving_image_digest, created_at) VALUES ($1,$2,'revolvr-embedding-space-v1','unsupported/model','one',1024,'last','l2','Q8_0',$3,'test-only','https://example.invalid/model',$4,$5)`, uuid.NewString(), strings.Repeat("9", 64), strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), now); err == nil {
		t.Fatal("schema admitted an unsupported embedding model")
	}

	empty := Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("1", 40), SourceTree: strings.Repeat("2", 40)}
	emptyBuild := prepareDBBuild(t, PrepareRequest{OperationID: "empty-" + projectID, Kind: BuildFull, Snapshot: empty, Now: now})
	stageActivate(t, ctx, store, emptyBuild, now.Add(time.Second))
	assertActive(t, ctx, pool, projectID, empty.SourceRevision, "", StateClean)

	full := Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("3", 40), SourceTree: strings.Repeat("4", 40), Files: []File{
		{Path: "a.go", Content: []byte("package fixture\nfunc Alpha() { Beta() }\n")},
		{Path: "b.go", Content: []byte("package fixture\nfunc Beta() {}\n")},
	}}
	fullBuild := prepareDBBuild(t, PrepareRequest{OperationID: "full-" + projectID, Kind: BuildRebuild, Snapshot: full, Now: now.Add(2 * time.Second)})
	stageActivate(t, ctx, store, fullBuild, now.Add(3*time.Second))
	if err := store.MarkDirty(ctx, projectID, "registered source advanced", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, pool, projectID, full.SourceRevision, "", StateDirty)

	existing, err := store.LoadExisting(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	incremental := Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("5", 40), SourceTree: strings.Repeat("6", 40), Files: append(append([]File(nil), full.Files...), File{Path: "docs.md", Content: []byte("# Routing\nAlpha calls Beta.\n")})}
	incrementalBuild := prepareDBBuild(t, PrepareRequest{OperationID: "incremental-" + projectID, Kind: BuildIncremental, Snapshot: incremental, Existing: existing, Now: now.Add(5 * time.Second)})
	if !incrementalBuild.Files[0].Reused || !incrementalBuild.Files[1].Reused || incrementalBuild.Files[2].Reused {
		t.Fatalf("incremental reuse = %#v", incrementalBuild.Manifest.Files)
	}
	stageActivate(t, ctx, store, incrementalBuild, now.Add(6*time.Second))
	assertActive(t, ctx, pool, projectID, incremental.SourceRevision, "", StateClean)

	interrupted := Snapshot{ProjectID: projectID, SourceRevision: strings.Repeat("7", 40), SourceTree: strings.Repeat("8", 40), Files: incremental.Files}
	interruptedBuild := prepareDBBuild(t, PrepareRequest{OperationID: "interrupted-" + projectID, Kind: BuildRebuild, Snapshot: interrupted, Now: now.Add(7 * time.Second)})
	if _, err := store.Stage(ctx, interruptedBuild); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, pool, projectID, incremental.SourceRevision, "", StateBuilding)
	if _, err := store.Stage(ctx, interruptedBuild); err != nil {
		t.Fatalf("interrupted exact replay: %v", err)
	}
	if err := store.Fail(ctx, interruptedBuild.OperationID, "evaluation_interrupted", now.Add(8*time.Second)); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, pool, projectID, incremental.SourceRevision, "", StateClean)

	modelA := selectedTestModel("space-a", strings.Repeat("a", 64))
	evidenceA := testEvidence(t, modelA)
	spaceA := prepareDBBuild(t, PrepareRequest{OperationID: "space-a-" + projectID, Kind: BuildSpaceSwitch, Snapshot: interrupted, EmbeddingSpace: &evidenceA, Embedder: &testEmbedder{model: modelA}, Now: now.Add(9 * time.Second)})
	stageActivate(t, ctx, store, spaceA, now.Add(10*time.Second))
	assertActive(t, ctx, pool, projectID, interrupted.SourceRevision, evidenceA.SpaceSHA256, StateClean)

	modelB := selectedTestModel("space-b", strings.Repeat("b", 64))
	evidenceB := testEvidence(t, modelB)
	failedSwitch := prepareDBBuild(t, PrepareRequest{OperationID: "space-b-failed-" + projectID, Kind: BuildSpaceSwitch, Snapshot: interrupted, EmbeddingSpace: &evidenceB, Embedder: &testEmbedder{model: modelB}, Now: now.Add(11 * time.Second)})
	if _, err := store.Stage(ctx, failedSwitch); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, pool, projectID, interrupted.SourceRevision, evidenceA.SpaceSHA256, StateBuilding)
	if _, err := pool.Exec(ctx, `DELETE FROM retrieval.index_build_chunks WHERE build_id=$1 AND chunk_id=(SELECT chunk_id FROM retrieval.index_build_chunks WHERE build_id=$1 ORDER BY chunk_id LIMIT 1)`, failedSwitch.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(ctx, failedSwitch.OperationID, now.Add(12*time.Second)); err == nil {
		t.Fatal("incomplete staged space switch activated")
	}
	assertActive(t, ctx, pool, projectID, interrupted.SourceRevision, evidenceA.SpaceSHA256, StateBuilding)
	if err := store.Fail(ctx, failedSwitch.OperationID, "forced_validation_failure", now.Add(12*time.Second)); err != nil {
		t.Fatal(err)
	}
	assertActive(t, ctx, pool, projectID, interrupted.SourceRevision, evidenceA.SpaceSHA256, StateClean)

	successfulSwitch := prepareDBBuild(t, PrepareRequest{OperationID: "space-b-success-" + projectID, Kind: BuildSpaceSwitch, Snapshot: interrupted, EmbeddingSpace: &evidenceB, Embedder: &testEmbedder{model: modelB}, Now: now.Add(13 * time.Second)})
	stageActivate(t, ctx, store, successfulSwitch, now.Add(14*time.Second))
	assertActive(t, ctx, pool, projectID, interrupted.SourceRevision, evidenceB.SpaceSHA256, StateClean)
	var activeVectors, wrongVectors int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM retrieval.index_states ist JOIN retrieval.index_builds ib ON ib.id=ist.active_build_id JOIN retrieval.index_build_chunks ibc ON ibc.build_id=ib.id JOIN retrieval.chunk_embeddings ce ON ce.chunk_id=ibc.chunk_id AND ce.embedding_space_id=ib.embedding_space_id WHERE ist.project_id=$1`, projectID).Scan(&activeVectors); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM retrieval.index_states ist JOIN retrieval.index_builds ib ON ib.id=ist.active_build_id JOIN retrieval.index_build_chunks ibc ON ibc.build_id=ib.id JOIN retrieval.chunk_embeddings ce ON ce.chunk_id=ibc.chunk_id AND ce.embedding_space_id<>ib.embedding_space_id WHERE ist.project_id=$1`, projectID).Scan(&wrongVectors); err != nil {
		t.Fatal(err)
	}
	if activeVectors != successfulSwitch.Manifest.ChunkCount || wrongVectors == 0 {
		// Historical vectors remain addressable by exact old-space identity; only
		// the active join must select a complete single space.
		t.Fatalf("active/wrong historical vectors = %d/%d, chunks=%d", activeVectors, wrongVectors, successfulSwitch.Manifest.ChunkCount)
	}
}

func selectedTestModel(revision, artifact string) embedding.EmbeddingModelInfo {
	model := SelectedEmbeddingEvidence().Model
	model.Revision = revision
	model.ArtifactSHA256 = artifact
	return model
}

func prepareDBBuild(t *testing.T, request PrepareRequest) PreparedBuild {
	t.Helper()
	build, err := Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return build
}

func stageActivate(t *testing.T, ctx context.Context, store *PostgresStore, build PreparedBuild, completed time.Time) {
	t.Helper()
	if _, err := store.Stage(ctx, build); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(ctx, build.OperationID, completed); err != nil {
		t.Fatal(err)
	}
}

func assertActive(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, revision, space string, state State) {
	t.Helper()
	var gotState, gotRevision string
	var gotSpace *string
	if err := pool.QueryRow(ctx, `SELECT ist.status, coalesce(ist.active_source_revision,''), es.space_sha256 FROM retrieval.index_states ist LEFT JOIN retrieval.embedding_spaces es ON es.id=ist.active_embedding_space_id WHERE ist.project_id=$1`, projectID).Scan(&gotState, &gotRevision, &gotSpace); err != nil {
		t.Fatal(err)
	}
	spaceValue := ""
	if gotSpace != nil {
		spaceValue = *gotSpace
	}
	if gotState != string(state) || gotRevision != revision || spaceValue != space {
		t.Fatalf("active state = %s/%s/%s, want %s/%s/%s", gotState, gotRevision, spaceValue, state, revision, space)
	}
}
