package evaluation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	contextpkg "revolvr/internal/context"
	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
	"revolvr/internal/retrieval"
)

type evaluationRetrievalSource struct {
	status       retrieval.IndexStatus
	exactFiles   []retrieval.Candidate
	exactSymbols []retrieval.Candidate
	structural   []retrieval.Candidate
	fts          []retrieval.Candidate
	vector       []retrieval.Candidate
}

func (s *evaluationRetrievalSource) Status(context.Context, string) (retrieval.IndexStatus, error) {
	return s.status, nil
}
func (s *evaluationRetrievalSource) ExactFiles(context.Context, string, []string, int) ([]retrieval.Candidate, error) {
	return cloneRetrievalCandidates(s.exactFiles), nil
}
func (s *evaluationRetrievalSource) ExactSymbols(context.Context, string, []string, int) ([]retrieval.Candidate, error) {
	return cloneRetrievalCandidates(s.exactSymbols), nil
}
func (s *evaluationRetrievalSource) ExactText(context.Context, string, string, int) ([]retrieval.Candidate, error) {
	return nil, nil
}
func (s *evaluationRetrievalSource) Structural(context.Context, string, []string, int) ([]retrieval.Candidate, error) {
	return cloneRetrievalCandidates(s.structural), nil
}
func (s *evaluationRetrievalSource) FTS(context.Context, string, string, int) ([]retrieval.Candidate, error) {
	return cloneRetrievalCandidates(s.fts), nil
}
func (s *evaluationRetrievalSource) Vector(context.Context, string, []float32, int, int) ([]retrieval.Candidate, error) {
	return cloneRetrievalCandidates(s.vector), nil
}

type deterministicEmbedder struct {
	unavailable bool
}

func (e deterministicEmbedder) ModelInfo(context.Context) (embedding.EmbeddingModelInfo, error) {
	if e.unavailable {
		return embedding.EmbeddingModelInfo{}, errors.New("deterministic embedding service unavailable")
	}
	return codeindex.SelectedEmbeddingEvidence().Model, nil
}

func (e deterministicEmbedder) EmbedDocuments(context.Context, []string) (embedding.EmbeddingBatch, error) {
	return embedding.EmbeddingBatch{}, errors.New("evaluation: document embeddings are not used")
}

func (e deterministicEmbedder) EmbedQuery(context.Context, string) (embedding.Embedding, error) {
	if e.unavailable {
		return embedding.Embedding{}, errors.New("deterministic embedding service unavailable")
	}
	evidence := codeindex.SelectedEmbeddingEvidence()
	value := make([]float32, evidence.Model.Dimensions)
	value[0] = 1
	return embedding.Embedding{
		Status: embedding.ServiceStatus{Mode: embedding.ServiceReady, Model: &evidence.Model, SpaceSHA256: evidence.SpaceSHA256},
		Space:  embedding.EmbeddingSpaceIdentity{SchemaVersion: embedding.SpaceSchemaVersion, Model: evidence.Model, SHA256: evidence.SpaceSHA256},
		Value:  value,
	}, nil
}

type retrievalArtifacts struct {
	fact       RetrievalFact
	dossier    []byte
	manifest   []byte
	candidates []retrieval.Candidate
}

func retrieveAndCompile(ctx context.Context, request ExecutionRequest, source *preparedSource) (retrievalArtifacts, error) {
	mainBytes, err := os.ReadFile(filepath.Join(source.originalCheckout, "main.go"))
	if err != nil {
		return retrievalArtifacts{}, err
	}
	evidence := codeindex.SelectedEmbeddingEvidence()
	activeRevision := source.baselineCommit
	if request.Scenario.RetrievalMode == RetrievalStaleIndex {
		activeRevision = strings.Repeat("b", 40)
	}
	mainCandidate := retrieval.Candidate{
		Identity: "source-main", ChunkID: "source-main", SourceKind: "code_chunk", SourceIdentity: "main.go:Run",
		SourceRevision: source.baselineCommit, SourceSHA256: hashBytes(mainBytes), Path: "main.go", Symbol: "Run", Language: "go", Kind: "function", StartLine: 5, EndLine: 7, Content: string(mainBytes),
	}
	testContent := "func TestRun(t *testing.T) { if Run() != \"ready\" { t.Fatal(\"not ready\") } }\n"
	testCandidate := retrieval.Candidate{
		Identity: "source-test", ChunkID: "source-test", SourceKind: "code_chunk", SourceIdentity: "main_test.go:TestRun",
		SourceRevision: source.baselineCommit, SourceSHA256: hashBytes([]byte(testContent)), Path: "main_test.go", Symbol: "TestRun", Language: "go", Kind: "function", StartLine: 5, EndLine: 7, Content: testContent,
		Signals: retrieval.Signals{LexicalScore: 0.8},
	}
	indexSource := &evaluationRetrievalSource{
		status:     retrieval.IndexStatus{State: "clean", SourceRevision: activeRevision, SpaceSHA256: evidence.SpaceSHA256, Dimensions: evidence.Model.Dimensions},
		exactFiles: []retrieval.Candidate{mainCandidate}, exactSymbols: []retrieval.Candidate{mainCandidate},
		structural: []retrieval.Candidate{testCandidate}, fts: []retrieval.Candidate{testCandidate}, vector: []retrieval.Candidate{testCandidate},
	}
	retriever, err := retrieval.New(indexSource)
	if err != nil {
		return retrievalArtifacts{}, err
	}
	taskContent := request.Authority.Task.Requirement
	canonical := retrieval.Candidate{
		Identity: "canonical-task", Authority: retrieval.AuthorityCanonicalTask, SourceKind: "task", SourceIdentity: request.Authority.Task.TaskVersionID,
		SourceRevision: source.baselineCommit, SourceSHA256: hashBytes([]byte(taskContent)), Content: taskContent,
	}
	result, err := retriever.Retrieve(ctx, retrieval.Request{
		ProjectID: "00000000-0000-5000-8000-000000000022", SourceRevision: source.baselineCommit,
		Canonical: []retrieval.Candidate{canonical}, ExactPaths: []string{"main.go"}, ExactSymbols: []string{"Run"},
		Query: "where is deterministic Run behavior implemented?", Limit: 10,
		ExpectedSpaceSHA256: evidence.SpaceSHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256,
		Embedder: deterministicEmbedder{unavailable: request.Scenario.RetrievalMode == RetrievalMissingEmbeddings},
	})
	if err != nil {
		return retrievalArtifacts{}, err
	}
	candidates := make([]contextpkg.Candidate, 0, len(result.Candidates))
	ids := make([]string, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		ids = append(ids, candidate.Identity)
		candidates = append(candidates, contextpkg.Candidate{Retrieval: candidate})
	}
	contextPackage, err := contextpkg.Compile(contextpkg.CompileRequest{
		ProjectID: "00000000-0000-5000-8000-000000000022", TaskID: request.Authority.Task.TaskID,
		RunID: "run-" + request.Scenario.ID, Role: contextpkg.RoleImplementer, SourceRevision: source.baselineCommit,
		EmbeddingSpaceSHA256: evidence.SpaceSHA256, Budget: contextpkg.Budget{Bytes: 32 << 10, Tokens: 8 << 10},
		RetrievalConfiguration: result.Report, Candidates: candidates,
	})
	if err != nil {
		return retrievalArtifacts{}, err
	}
	manifest, err := Canonical(contextPackage.Manifest)
	if err != nil {
		return retrievalArtifacts{}, err
	}
	laneStates := make([]string, 0, len(result.Report.Lanes))
	degraded := false
	for _, lane := range result.Report.Lanes {
		laneStates = append(laneStates, lane.Lane+":"+string(lane.State))
		if lane.Lane == "vector" && (lane.State == retrieval.LaneDegraded || lane.State == retrieval.LaneStale || lane.State == retrieval.LaneOmitted) {
			degraded = true
		}
	}
	exactFirst := exactAuthoritiesPrecedeDerived(result.Candidates)
	fact := RetrievalFact{
		Status: string(request.Scenario.RetrievalMode), CandidateIDs: ids, LaneStates: laneStates,
		ContextPackageID: contextPackage.ID, ContextManifestSHA256: hashBytes(manifest), DossierSHA256: contextPackage.Manifest.DossierSHA256,
		ExactSourceFirst: exactFirst, EmbeddingModelName: evidence.Model.ModelName, EmbeddingRevision: evidence.Model.Revision,
		EmbeddingSpaceSHA256: evidence.SpaceSHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256,
		DegradedWithoutFallback: degraded,
	}
	return retrievalArtifacts{fact: fact, dossier: contextPackage.Dossier, manifest: manifest, candidates: result.Candidates}, nil
}

func exactAuthoritiesPrecedeDerived(candidates []retrieval.Candidate) bool {
	seenDerived := false
	for _, candidate := range candidates {
		switch candidate.Authority {
		case retrieval.AuthorityCanonicalTask, retrieval.AuthorityExactSource, retrieval.AuthorityHostPolicy, retrieval.AuthorityCanonicalEvidence:
			if seenDerived {
				return false
			}
		default:
			seenDerived = true
		}
	}
	return true
}

func cloneRetrievalCandidates(values []retrieval.Candidate) []retrieval.Candidate {
	return append([]retrieval.Candidate(nil), values...)
}

type qualityFixture struct {
	ID             string `json:"id"`
	Query          string `json:"query"`
	ExactPath      string `json:"exact_path"`
	ExactSymbol    string `json:"exact_symbol"`
	ExpectedPath   string `json:"expected_path"`
	ExpectedSymbol string `json:"expected_symbol"`
}

func measureQuality(repositoryRoot string) (QualityBaseline, error) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, "evals", "retrieval", "queries.json"))
	if err != nil {
		return QualityBaseline{}, err
	}
	var fixtures []qualityFixture
	if err := decodeStrict(raw, &fixtures); err != nil {
		return QualityBaseline{}, err
	}
	if len(fixtures) == 0 {
		return QualityBaseline{}, errors.New("evaluation: retrieval quality fixtures are empty")
	}
	var recall5, recall10, reciprocal, exactPreserved float64
	evidence := codeindex.SelectedEmbeddingEvidence()
	for _, fixture := range fixtures {
		if fixture.ID == "" || fixture.Query == "" || fixture.ExpectedPath == "" {
			return QualityBaseline{}, errors.New("evaluation: retrieval quality fixture is incomplete")
		}
		content := "quality fixture " + fixture.ID + "\n"
		expected := retrieval.Candidate{
			Identity: "expected-" + fixture.ID, ChunkID: "expected-" + fixture.ID, SourceKind: "code_chunk",
			SourceIdentity: fixture.ExpectedPath, SourceRevision: strings.Repeat("a", 40), SourceSHA256: hashBytes([]byte(content)),
			Path: fixture.ExpectedPath, Symbol: fixture.ExpectedSymbol, Content: content,
		}
		noiseContent := "unrelated quality fixture\n"
		noise := retrieval.Candidate{
			Identity: "noise-" + fixture.ID, ChunkID: "noise-" + fixture.ID, SourceKind: "code_chunk",
			SourceIdentity: "noise.txt", SourceRevision: strings.Repeat("a", 40), SourceSHA256: hashBytes([]byte(noiseContent)),
			Path: "noise.txt", Content: noiseContent, Signals: retrieval.Signals{LexicalScore: 1, VectorScore: 1},
		}
		source := &evaluationRetrievalSource{
			status:     retrieval.IndexStatus{State: "clean", SourceRevision: strings.Repeat("a", 40), SpaceSHA256: evidence.SpaceSHA256, Dimensions: evidence.Model.Dimensions},
			exactFiles: []retrieval.Candidate{expected}, fts: []retrieval.Candidate{noise}, vector: []retrieval.Candidate{noise},
		}
		if fixture.ExactSymbol != "" {
			source.exactSymbols = []retrieval.Candidate{expected}
		}
		runner, err := retrieval.New(source)
		if err != nil {
			return QualityBaseline{}, err
		}
		result, err := runner.Retrieve(context.Background(), retrieval.Request{
			ProjectID: "00000000-0000-5000-8000-000000000022", SourceRevision: strings.Repeat("a", 40),
			ExactPaths: []string{fixture.ExactPath}, ExactSymbols: []string{fixture.ExactSymbol}, Query: fixture.Query, Limit: 10,
			ExpectedSpaceSHA256: evidence.SpaceSHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256,
			Embedder: deterministicEmbedder{},
		})
		if err != nil {
			return QualityBaseline{}, err
		}
		first := 0
		for i, candidate := range result.Candidates {
			if candidate.Path == fixture.ExpectedPath && candidate.Symbol == fixture.ExpectedSymbol {
				first = i + 1
				break
			}
		}
		if first > 0 && first <= 5 {
			recall5++
		}
		if first > 0 && first <= 10 {
			recall10++
		}
		if first > 0 {
			reciprocal += 1 / float64(first)
		}
		if fixture.ExactSymbol != "" && fixture.ExactSymbol == fixture.ExpectedSymbol && first > 0 && first <= 10 {
			exactPreserved++
		}
	}
	count := float64(len(fixtures))
	exactCount := 0
	for _, fixture := range fixtures {
		if fixture.ExactSymbol != "" {
			exactCount++
		}
	}
	value := QualityBaseline{
		FixtureCount: len(fixtures), RecallAt5: recall5 / count, RecallAt10: recall10 / count, MRR: reciprocal / count,
		Threshold: nil,
		Omissions: []Omission{
			{Field: "latency", Reason: "nondeterministic_host_timing_excluded_from_byte_stable_quality_baseline"},
			{Field: "quality_threshold", Reason: "not_set_before_measured_baseline"},
		},
	}
	if exactCount > 0 {
		value.ExactSymbolPreservation = exactPreserved / float64(exactCount)
	}
	return value, nil
}
