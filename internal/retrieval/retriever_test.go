package retrieval

import (
	"context"
	"errors"
	"strings"
	"testing"

	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
)

func TestRetrievePreservesAuthorityAndExactPriority(t *testing.T) {
	model := fixtureModel()
	space, _ := model.SpaceIdentity()
	source := &fixtureSource{status: IndexStatus{State: "clean", SourceRevision: strings.Repeat("a", 40), SpaceSHA256: space.SHA256, Dimensions: model.Dimensions}}
	source.exactFiles = []Candidate{candidate("exact-file", "internal/provider/router.go", "Route", 0.1)}
	source.exactSymbols = []Candidate{candidate("exact-symbol", "internal/provider/router.go", "RouteProvider", 0.1)}
	source.structural = []Candidate{candidate("structural", "internal/provider/config.go", "ProviderConfig", 0.9)}
	source.fts = []Candidate{candidate("fts", "docs/provider.md", "Provider routing", 1)}
	source.vector = []Candidate{candidate("vector", "unrelated/high-score.go", "Unrelated", 1)}
	embedder := &fixtureEmbedder{model: model}
	retriever, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := retriever.Retrieve(context.Background(), Request{
		ProjectID: "project", SourceRevision: source.status.SourceRevision,
		Canonical:  []Candidate{{Identity: "task", Authority: AuthorityCanonicalTask, SourceKind: "task", SourceIdentity: "task:one", SourceSHA256: strings.Repeat("c", 64), Content: "canonical"}},
		ExactPaths: []string{"internal/provider/router.go"}, ExactSymbols: []string{"RouteProvider"},
		ExactText: "provider routing", Query: "where is provider routing configured?", Limit: 20,
		ExpectedSpaceSHA256: source.status.SpaceSHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256, Embedder: embedder,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"task", "exact-file", "exact-symbol"}
	for index, identity := range want {
		if result.Candidates[index].Identity != identity {
			t.Fatalf("candidate %d = %s, want %s; all=%#v", index, result.Candidates[index].Identity, identity, result.Candidates)
		}
	}
	for index, item := range result.Candidates {
		if item.Identity == "vector" && index < 3 {
			t.Fatal("vector result outranked exact authority")
		}
	}
	if laneState(result.Report, "vector") != LaneUsed || laneState(result.Report, "relationship_graph") != LaneOmitted {
		t.Fatalf("lane report = %#v", result.Report.Lanes)
	}
}

func TestRetrieveKeepsExactAndFTSWhenVectorIsStaleOrUnavailable(t *testing.T) {
	revision := strings.Repeat("d", 40)
	source := &fixtureSource{
		status:     IndexStatus{State: "clean", SourceRevision: strings.Repeat("e", 40), SpaceSHA256: strings.Repeat("f", 64), Dimensions: codeindex.SelectedEmbeddingDimensions},
		exactFiles: []Candidate{candidate("exact", "worker.go", "Cleanup", 0)},
		fts:        []Candidate{candidate("fts", "worker.go", "Cleanup", 0.8)},
	}
	retriever, _ := New(source)
	stale, err := retriever.Retrieve(context.Background(), Request{ProjectID: "project", SourceRevision: revision, ExactPaths: []string{"worker.go"}, Query: "cleanup workspace", Limit: 10, ExpectedSpaceSHA256: source.status.SpaceSHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256, Embedder: &fixtureEmbedder{model: fixtureModel()}})
	if err != nil || len(stale.Candidates) == 0 || laneState(stale.Report, "vector") != LaneStale || !stale.Candidates[0].Signals.Stale {
		t.Fatalf("stale retrieval = %#v, %v", stale, err)
	}

	source.status.SourceRevision = revision
	outage := &fixtureEmbedder{model: fixtureModel(), err: errors.New("offline")}
	degraded, err := retriever.Retrieve(context.Background(), Request{ProjectID: "project", SourceRevision: revision, ExactPaths: []string{"worker.go"}, Query: "cleanup workspace", Limit: 10, ExpectedSpaceSHA256: source.status.SpaceSHA256, QueryInstructionSHA256: codeindex.SelectedQueryInstructionSHA256, Embedder: outage})
	if err != nil || len(degraded.Candidates) == 0 || laneState(degraded.Report, "vector") != LaneDegraded || laneState(degraded.Report, "fts") != LaneUsed {
		t.Fatalf("degraded retrieval = %#v, %v", degraded, err)
	}
}

func TestRetrieveRejectsNonSelectedQueryInstruction(t *testing.T) {
	model := fixtureModel()
	space, _ := model.SpaceIdentity()
	retriever, _ := New(&fixtureSource{status: IndexStatus{State: "clean", SourceRevision: strings.Repeat("a", 40), SpaceSHA256: space.SHA256, Dimensions: model.Dimensions}})
	_, err := retriever.Retrieve(context.Background(), Request{ProjectID: "project", SourceRevision: strings.Repeat("a", 40), Query: "routing", ExpectedSpaceSHA256: space.SHA256, QueryInstructionSHA256: strings.Repeat("b", 64), Embedder: &fixtureEmbedder{model: model}})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("non-selected instruction error = %v", err)
	}
}

func TestEvaluateRecordsRecallMRRAndLanguageBreakdown(t *testing.T) {
	fixtures := []Fixture{
		{ID: "go-symbol", Query: "provider routing", ExactSymbols: []string{"RouteProvider"}, Expected: []ExpectedHit{{Path: "router.go", Symbol: "RouteProvider", Language: "go"}}},
		{ID: "markdown", Query: "removed storage backend", Expected: []ExpectedHit{{Path: "ADR.md", Symbol: "Storage", Language: "markdown"}}},
	}
	metrics, err := Evaluate(context.Background(), fixtures, func(_ context.Context, fixture Fixture) (Result, error) {
		if fixture.ID == "go-symbol" {
			return Result{Candidates: []Candidate{{Path: "router.go", Symbol: "RouteProvider"}}}, nil
		}
		return Result{Candidates: []Candidate{{Path: "noise"}, {Path: "ADR.md", Symbol: "Storage"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RecallAt5 != 1 || metrics.RecallAt10 != 1 || metrics.MRR != 0.75 || metrics.ExactSymbolPreservation != 1 || metrics.LanguageBreakdown["go"].RecallAt5 != 1 || metrics.LanguageBreakdown["markdown"].MRR != 0.5 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestCompileFTSQueryIsBoundedDeterministicAndNaturalLanguageFriendly(t *testing.T) {
	want := "'persist':* | 'verific':* | 'executi':* | 'evidenc':*"
	if got := compileFTSQuery("What persists verification execution evidence, and verification?"); got != want {
		t.Fatalf("compiled FTS query = %q, want %q", got, want)
	}
	if got := compileFTSQuery("what is the and or"); got != "" {
		t.Fatalf("stop-word-only query = %q", got)
	}
}

type fixtureSource struct {
	status       IndexStatus
	exactFiles   []Candidate
	exactSymbols []Candidate
	exactText    []Candidate
	structural   []Candidate
	fts          []Candidate
	vector       []Candidate
}

func (s *fixtureSource) Status(context.Context, string) (IndexStatus, error) { return s.status, nil }
func (s *fixtureSource) ExactFiles(context.Context, string, []string, int) ([]Candidate, error) {
	return cloneCandidates(s.exactFiles), nil
}
func (s *fixtureSource) ExactSymbols(context.Context, string, []string, int) ([]Candidate, error) {
	return cloneCandidates(s.exactSymbols), nil
}
func (s *fixtureSource) ExactText(context.Context, string, string, int) ([]Candidate, error) {
	return cloneCandidates(s.exactText), nil
}
func (s *fixtureSource) Structural(context.Context, string, []string, int) ([]Candidate, error) {
	return cloneCandidates(s.structural), nil
}
func (s *fixtureSource) FTS(context.Context, string, string, int) ([]Candidate, error) {
	return cloneCandidates(s.fts), nil
}
func (s *fixtureSource) Vector(context.Context, string, []float32, int, int) ([]Candidate, error) {
	return cloneCandidates(s.vector), nil
}

type fixtureEmbedder struct {
	model embedding.EmbeddingModelInfo
	err   error
}

func (e *fixtureEmbedder) ModelInfo(context.Context) (embedding.EmbeddingModelInfo, error) {
	return e.model, e.err
}
func (e *fixtureEmbedder) EmbedDocuments(context.Context, []string) (embedding.EmbeddingBatch, error) {
	return embedding.EmbeddingBatch{}, errors.New("not used")
}
func (e *fixtureEmbedder) EmbedQuery(context.Context, string) (embedding.Embedding, error) {
	if e.err != nil {
		return embedding.Embedding{}, e.err
	}
	space, _ := e.model.SpaceIdentity()
	vector := make([]float32, e.model.Dimensions)
	vector[0] = 1
	return embedding.Embedding{Status: embedding.ServiceStatus{Mode: embedding.ServiceReady}, Space: space, Value: vector}, nil
}

func fixtureModel() embedding.EmbeddingModelInfo {
	return codeindex.SelectedEmbeddingEvidence().Model
}
func candidate(id, path, symbol string, score float64) Candidate {
	return Candidate{Identity: id, ChunkID: id, SourceKind: "code_chunk", SourceIdentity: path, SourceSHA256: strings.Repeat("a", 64), Path: path, Symbol: symbol, Signals: Signals{LexicalScore: score, VectorScore: score}}
}
func cloneCandidates(values []Candidate) []Candidate { return append([]Candidate(nil), values...) }
func laneState(report Report, name string) LaneState {
	for _, lane := range report.Lanes {
		if lane.Lane == name {
			return lane.State
		}
	}
	return ""
}
