package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
	"revolvr/internal/retrieval"
	storage "revolvr/internal/storage/postgres"
)

const reportSchema = "revolvr-retrieval-model-evaluation-v1"

type dataset struct {
	SchemaVersion string              `json:"schema_version"`
	Projects      []projectDefinition `json:"projects"`
	Fixtures      []retrieval.Fixture `json:"fixtures"`
}

type projectDefinition struct {
	Name       string   `json:"name"`
	Repository string   `json:"repository"`
	SourceURI  string   `json:"source_uri"`
	Revision   string   `json:"revision"`
	Tree       string   `json:"tree"`
	Include    []string `json:"include"`
}

type projectReport struct {
	Name           string `json:"name"`
	SourceURI      string `json:"source_uri"`
	Revision       string `json:"revision"`
	Tree           string `json:"tree"`
	FileCount      int    `json:"file_count"`
	ChunkCount     int    `json:"chunk_count"`
	SymbolCount    int    `json:"symbol_count"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type report struct {
	SchemaVersion            string                       `json:"schema_version"`
	MeasuredAt               time.Time                    `json:"measured_at"`
	DatasetSHA256            string                       `json:"dataset_sha256"`
	Model                    embedding.EmbeddingModelInfo `json:"model"`
	EmbeddingSpaceSHA256     string                       `json:"embedding_space_sha256"`
	License                  string                       `json:"license"`
	SourceURI                string                       `json:"source_uri"`
	ServingImageDigest       string                       `json:"serving_image_digest"`
	QueryInstructionSHA256   string                       `json:"query_instruction_sha256"`
	Projects                 []projectReport              `json:"projects"`
	FileCount                int                          `json:"file_count"`
	ChunkCount               int                          `json:"chunk_count"`
	SymbolCount              int                          `json:"symbol_count"`
	IndexBuildNanoseconds    int64                        `json:"index_build_nanoseconds"`
	IndexChunksPerSecond     float64                      `json:"index_chunks_per_second"`
	PostgresIndexGrowthBytes int64                        `json:"postgres_index_growth_bytes"`
	GPU                      string                       `json:"gpu"`
	VRAMMiB                  int                          `json:"vram_mib"`
	VectorOnly               retrieval.QualityMetrics     `json:"vector_only"`
	LexicalOnly              retrieval.QualityMetrics     `json:"lexical_only"`
	Hybrid                   retrieval.QualityMetrics     `json:"hybrid"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("revolvr-retrieval-eval", flag.ContinueOnError)
	fixturesPath := flags.String("fixtures", "internal/retrieval/testdata/architecture-021-real-projects.json", "fixture manifest")
	sourceRoot := flags.String("source-root", "/home/gernsback/source", "parent containing exact project repositories")
	databaseURL := flags.String("database-url", os.Getenv("REVOLVR_TEST_DATABASE_URL"), "migrated PostgreSQL URL")
	endpoint := flags.String("embedding-endpoint", os.Getenv("REVOLVR_EMBEDDING_ENDPOINT"), "Architecture 020 endpoint")
	license := flags.String("license", os.Getenv("REVOLVR_EMBEDDING_LICENSE"), "model license")
	sourceURI := flags.String("model-source-uri", os.Getenv("REVOLVR_EMBEDDING_SOURCE_URI"), "exact model source")
	imageDigest := flags.String("serving-image-digest", os.Getenv("REVOLVR_EMBEDDING_SERVING_IMAGE_DIGEST"), "pinned serving image digest")
	queryInstructionSHA := flags.String("query-instruction-sha256", os.Getenv("REVOLVR_EVAL_QUERY_INSTRUCTION_SHA256"), "exact adapter query instruction hash")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *databaseURL == "" {
		return errors.New("retrieval evaluation: migrated database URL is required and positional arguments are unsupported")
	}
	model, err := modelFromEnvironment()
	if err != nil {
		return err
	}
	space, _ := model.SpaceIdentity()
	evidence := codeindex.ModelEvidence{Model: model, SpaceSHA256: space.SHA256, License: strings.TrimSpace(*license), SourceURI: strings.TrimSpace(*sourceURI), ServingImageDigest: strings.TrimSpace(*imageDigest)}
	if err := evidence.Validate(); err != nil {
		return err
	}
	if *queryInstructionSHA != codeindex.SelectedQueryInstructionSHA256 {
		return errors.New("retrieval evaluation: selected Qwen query-instruction SHA-256 is required")
	}
	raw, err := os.ReadFile(*fixturesPath)
	if err != nil {
		return err
	}
	data, err := decodeDataset(raw)
	if err != nil {
		return err
	}
	datasetSHA := hash(raw)
	client, err := embedding.NewClient(embedding.Config{Endpoint: strings.TrimSpace(*endpoint), ExpectedModel: model, Timeout: 2 * time.Minute, MaxBatchInputs: 32, MaxInputBytes: 256 << 10, MaxBatchBytes: 1 << 20})
	if err != nil {
		return err
	}
	if status, err := client.Health(ctx); err != nil || status.Mode != embedding.ServiceReady {
		return fmt.Errorf("retrieval evaluation: embedding service is not exactly ready: %w", err)
	}
	pool, err := storage.Open(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	before, err := relationSize(ctx, pool)
	if err != nil {
		return err
	}
	result := report{SchemaVersion: reportSchema, MeasuredAt: time.Now().UTC().Truncate(time.Second), DatasetSHA256: datasetSHA, Model: model, EmbeddingSpaceSHA256: space.SHA256, License: evidence.License, SourceURI: evidence.SourceURI, ServingImageDigest: evidence.ServingImageDigest, QueryInstructionSHA256: *queryInstructionSHA}
	sources := map[string]*retrieval.PostgresSource{}
	retrievers := map[string]*retrieval.Retriever{}
	revisions := map[string]string{}
	buildStarted := time.Now()
	for _, project := range data.Projects {
		projectID := codeindex.DeterministicID("retrieval-evaluation-project", datasetSHA, space.SHA256, project.Name)
		now := time.Now().UTC().Truncate(time.Microsecond)
		if _, err := pool.Exec(ctx, `INSERT INTO core.projects (id,name,status,created_at,updated_at) VALUES ($1,$2,'active',$3,$3) ON CONFLICT (id) DO NOTHING`, projectID, "retrieval-evaluation-"+project.Name+"-"+space.SHA256[:12], now); err != nil {
			return err
		}
		snapshot, err := codeindex.ReadGitSnapshot(ctx, projectID, filepath.Join(*sourceRoot, project.Repository), project.Revision, project.Tree, codeindex.AdmissionRules{Include: project.Include}, codeindex.DefaultLimits())
		if err != nil {
			return fmt.Errorf("retrieval evaluation snapshot %s: %w", project.Name, err)
		}
		build, err := codeindex.Prepare(ctx, codeindex.PrepareRequest{OperationID: "retrieval-evaluation-index-" + projectID, Kind: codeindex.BuildFull, Snapshot: snapshot, EmbeddingSpace: &evidence, Embedder: client, Now: now})
		if err != nil {
			return fmt.Errorf("retrieval evaluation build %s: %w", project.Name, err)
		}
		store, _ := codeindex.NewPostgresStore(pool)
		if _, err := store.Stage(ctx, build); err != nil {
			return err
		}
		if err := store.Activate(ctx, build.OperationID, time.Now().UTC()); err != nil {
			return err
		}
		projectSource, _ := retrieval.NewPostgresSource(pool)
		projectRetriever, _ := retrieval.New(projectSource)
		sources[project.Name], retrievers[project.Name], revisions[project.Name] = projectSource, projectRetriever, project.Revision
		projectResult := projectReport{Name: project.Name, SourceURI: project.SourceURI, Revision: project.Revision, Tree: project.Tree, FileCount: build.Manifest.FileCount, ChunkCount: build.Manifest.ChunkCount, SymbolCount: build.Manifest.SymbolCount, ManifestSHA256: build.Manifest.SHA256}
		result.Projects = append(result.Projects, projectResult)
		result.FileCount += projectResult.FileCount
		result.ChunkCount += projectResult.ChunkCount
		result.SymbolCount += projectResult.SymbolCount
	}
	result.IndexBuildNanoseconds = time.Since(buildStarted).Nanoseconds()
	if result.IndexBuildNanoseconds > 0 {
		result.IndexChunksPerSecond = float64(result.ChunkCount) / (float64(result.IndexBuildNanoseconds) / float64(time.Second))
	}
	after, err := relationSize(ctx, pool)
	if err != nil {
		return err
	}
	result.PostgresIndexGrowthBytes = after - before
	result.GPU, result.VRAMMiB = gpuMeasurement()
	result.VectorOnly, err = retrieval.Evaluate(ctx, data.Fixtures, func(ctx context.Context, fixture retrieval.Fixture) (retrieval.Result, error) {
		source := sources[fixture.Project]
		if source == nil {
			return retrieval.Result{}, errors.New("unknown fixture project")
		}
		query, err := client.EmbedQuery(ctx, fixture.Query)
		if err != nil {
			return retrieval.Result{}, err
		}
		candidates, err := source.Vector(ctx, projectIDFor(datasetSHA, space.SHA256, fixture.Project), query.Value, model.Dimensions, 30)
		return retrieval.Result{Candidates: candidates}, err
	})
	if err != nil {
		return err
	}
	result.LexicalOnly, err = retrieval.Evaluate(ctx, data.Fixtures, func(ctx context.Context, fixture retrieval.Fixture) (retrieval.Result, error) {
		candidates, err := sources[fixture.Project].FTS(ctx, projectIDFor(datasetSHA, space.SHA256, fixture.Project), fixture.Query, 30)
		return retrieval.Result{Candidates: candidates}, err
	})
	if err != nil {
		return err
	}
	result.Hybrid, err = retrieval.Evaluate(ctx, data.Fixtures, func(ctx context.Context, fixture retrieval.Fixture) (retrieval.Result, error) {
		return retrievers[fixture.Project].Retrieve(ctx, retrieval.Request{ProjectID: projectIDFor(datasetSHA, space.SHA256, fixture.Project), SourceRevision: revisions[fixture.Project], Query: fixture.Query, ExactPaths: fixture.ExactPaths, ExactSymbols: fixture.ExactSymbols, Limit: 30, ExpectedSpaceSHA256: space.SHA256, QueryInstructionSHA256: *queryInstructionSHA, Embedder: client})
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func decodeDataset(raw []byte) (dataset, error) {
	var result dataset
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || decoder.Decode(&struct{}{}) != io.EOF || result.SchemaVersion != "revolvr-retrieval-fixtures-v1" || len(result.Projects) == 0 || len(result.Fixtures) == 0 {
		return dataset{}, errors.New("retrieval evaluation: fixture manifest is invalid")
	}
	projects := map[string]bool{}
	for _, project := range result.Projects {
		if project.Name == "" || projects[project.Name] || project.Repository == "" || project.SourceURI == "" || !validGitID(project.Revision) || !validGitID(project.Tree) || len(project.Include) == 0 {
			return dataset{}, errors.New("retrieval evaluation: project fixture is invalid")
		}
		projects[project.Name] = true
	}
	for _, fixture := range result.Fixtures {
		if !projects[fixture.Project] || fixture.ID == "" || fixture.Query == "" || len(fixture.Expected) == 0 {
			return dataset{}, errors.New("retrieval evaluation: query fixture is invalid")
		}
	}
	return result, nil
}

func modelFromEnvironment() (embedding.EmbeddingModelInfo, error) {
	dimensions, err := strconv.Atoi(os.Getenv("REVOLVR_EMBEDDING_DIMENSIONS"))
	model := embedding.EmbeddingModelInfo{SchemaVersion: embedding.ModelInfoSchemaVersion, ModelName: os.Getenv("REVOLVR_EMBEDDING_MODEL_NAME"), Revision: os.Getenv("REVOLVR_EMBEDDING_MODEL_REVISION"), Dimensions: dimensions, Pooling: os.Getenv("REVOLVR_EMBEDDING_POOLING"), Normalization: os.Getenv("REVOLVR_EMBEDDING_NORMALIZATION"), Quantization: os.Getenv("REVOLVR_EMBEDDING_QUANTIZATION"), ArtifactSHA256: os.Getenv("REVOLVR_EMBEDDING_ARTIFACT_SHA256")}
	if err != nil || codeindex.ValidateSelectedEmbeddingModel(model) != nil {
		return embedding.EmbeddingModelInfo{}, errors.New("retrieval evaluation: selected Qwen model environment is invalid")
	}
	return model, nil
}

func relationSize(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var size int64
	err := pool.QueryRow(ctx, `SELECT coalesce(sum(pg_total_relation_size(format('%I.%I', schemaname, tablename)::regclass)),0)::bigint
FROM pg_tables
WHERE schemaname='retrieval'
   OR (schemaname='telemetry' AND tablename IN ('context_packages','context_items'))`).Scan(&size)
	return size, err
}

func projectIDFor(datasetSHA, spaceSHA, name string) string {
	return codeindex.DeterministicID("retrieval-evaluation-project", datasetSHA, spaceSHA, name)
}

func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validGitID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded)*2 == len(value) && strings.ToLower(value) == value
}

func gpuMeasurement() (string, int) {
	nameRaw, _ := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output()
	memoryRaw, _ := exec.Command("nvidia-smi", "--query-compute-apps=used_gpu_memory", "--format=csv,noheader,nounits").Output()
	total := 0
	for _, line := range strings.Fields(string(memoryRaw)) {
		value, _ := strconv.Atoi(line)
		total += value
	}
	return strings.TrimSpace(string(nameRaw)), total
}
