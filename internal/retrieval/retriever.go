package retrieval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"revolvr/internal/embedding"
)

type Retriever struct{ source Source }

func New(source Source) (*Retriever, error) {
	if source == nil {
		return nil, errors.New("retrieval source is required")
	}
	return &Retriever{source: source}, nil
}

func (r *Retriever) Retrieve(ctx context.Context, request Request) (Result, error) {
	if r == nil || r.source == nil || validateRequest(request) != nil {
		return Result{}, ErrInvalidRequest
	}
	if request.Limit == 0 {
		request.Limit = 30
	}
	if request.LaneLimit == 0 {
		request.LaneLimit = min(100, max(request.Limit*3, 30))
	}
	request.ExactPaths = normalized(request.ExactPaths, false)
	request.ExactSymbols = normalized(request.ExactSymbols, true)
	status, err := r.source.Status(ctx, request.ProjectID)
	if err != nil {
		return Result{}, fmt.Errorf("retrieve index status: %w", err)
	}
	report := Report{
		ConfigurationVersion: ConfigurationVersion, SourceRevision: request.SourceRevision,
		ActiveSourceRevision: status.SourceRevision, EmbeddingSpace: status.SpaceSHA256,
		QueryInstructionSHA256: request.QueryInstructionSHA256,
	}
	stale := status.SourceRevision != "" && status.SourceRevision != request.SourceRevision
	all := make([]Candidate, 0, len(request.Canonical)+request.LaneLimit*5)
	canonical := append([]Candidate(nil), request.Canonical...)
	for i := range canonical {
		canonical[i].MatchedLanes = append(canonical[i].MatchedLanes, "canonical")
		canonical[i].Signals.DirectTaskReference = canonical[i].Authority == AuthorityCanonicalTask || canonical[i].Signals.DirectTaskReference
	}
	all = append(all, canonical...)
	report.Lanes = append(report.Lanes, lane("canonical", len(canonical), false, ""))

	if len(request.ExactPaths) > 0 {
		items, laneErr := r.source.ExactFiles(ctx, request.ProjectID, request.ExactPaths, request.LaneLimit)
		if laneErr != nil {
			return Result{}, fmt.Errorf("retrieve exact files: %w", laneErr)
		}
		mark(items, "exact_file", AuthorityExactSource, stale, func(signals *Signals) { signals.ExactPath = true })
		all = append(all, items...)
		report.Lanes = append(report.Lanes, lane("exact_file", len(items), stale, staleReason(stale)))
	} else {
		report.Lanes = append(report.Lanes, LaneReport{Lane: "exact_file", State: LaneOmitted, Reason: "no exact path reference"})
	}

	if len(request.ExactSymbols) > 0 {
		items, laneErr := r.source.ExactSymbols(ctx, request.ProjectID, request.ExactSymbols, request.LaneLimit)
		if laneErr != nil {
			return Result{}, fmt.Errorf("retrieve exact symbols: %w", laneErr)
		}
		mark(items, "exact_symbol", AuthorityExactSource, stale, func(signals *Signals) { signals.ExactSymbol = true })
		all = append(all, items...)
		report.Lanes = append(report.Lanes, lane("exact_symbol", len(items), stale, staleReason(stale)))
	} else {
		report.Lanes = append(report.Lanes, LaneReport{Lane: "exact_symbol", State: LaneOmitted, Reason: "no exact symbol reference"})
	}

	if strings.TrimSpace(request.ExactText) != "" {
		items, laneErr := r.source.ExactText(ctx, request.ProjectID, request.ExactText, request.LaneLimit)
		if laneErr != nil {
			return Result{}, fmt.Errorf("retrieve exact text: %w", laneErr)
		}
		mark(items, "exact_text", AuthorityExactSource, stale, func(signals *Signals) { signals.ExactText = true })
		all = append(all, items...)
		report.Lanes = append(report.Lanes, lane("exact_text", len(items), stale, staleReason(stale)))
	} else {
		report.Lanes = append(report.Lanes, LaneReport{Lane: "exact_text", State: LaneOmitted, Reason: "no exact text reference"})
	}

	if len(request.ExactSymbols) > 0 {
		items, laneErr := r.source.Structural(ctx, request.ProjectID, request.ExactSymbols, request.LaneLimit)
		if laneErr != nil {
			return Result{}, fmt.Errorf("retrieve structural graph: %w", laneErr)
		}
		mark(items, "structural", AuthorityStructural, stale, func(signals *Signals) { signals.Structural = true })
		all = append(all, items...)
		report.Lanes = append(report.Lanes, lane("structural", len(items), stale, staleReason(stale)))
	} else {
		report.Lanes = append(report.Lanes, LaneReport{Lane: "structural", State: LaneOmitted, Reason: "no structural seed symbol"})
	}

	if strings.TrimSpace(request.Query) != "" {
		items, laneErr := r.source.FTS(ctx, request.ProjectID, request.Query, request.LaneLimit)
		if laneErr != nil {
			return Result{}, fmt.Errorf("retrieve PostgreSQL FTS: %w", laneErr)
		}
		mark(items, "fts", AuthorityLexical, stale, nil)
		all = append(all, items...)
		report.Lanes = append(report.Lanes, lane("fts", len(items), stale, staleReason(stale)))
	} else {
		report.Lanes = append(report.Lanes, LaneReport{Lane: "fts", State: LaneOmitted, Reason: "no search query"})
	}

	vectorItems, vectorReport := r.vectorLane(ctx, request, status, stale)
	all = append(all, vectorItems...)
	report.Lanes = append(report.Lanes, vectorReport)
	report.Lanes = append(report.Lanes,
		LaneReport{Lane: "relationship_graph", State: LaneOmitted, Reason: "no admitted typed relationship seed"},
		LaneReport{Lane: "reranker", State: LaneOmitted, Reason: "no measured reranker is configured"},
	)

	merged := deduplicate(all)
	merged = rank(merged)
	if len(merged) > request.Limit {
		merged = merged[:request.Limit]
	}
	return Result{Candidates: merged, Report: report}, nil
}

func (r *Retriever) vectorLane(ctx context.Context, request Request, status IndexStatus, stale bool) ([]Candidate, LaneReport) {
	if strings.TrimSpace(request.Query) == "" {
		return nil, LaneReport{Lane: "vector", State: LaneOmitted, Reason: "no search query"}
	}
	if request.Embedder == nil {
		return nil, LaneReport{Lane: "vector", State: LaneOmitted, Reason: "embedding service not configured"}
	}
	if status.SpaceSHA256 == "" || status.Dimensions == 0 {
		return nil, LaneReport{Lane: "vector", State: LaneOmitted, Reason: "active index has no embedding space"}
	}
	if stale {
		return nil, LaneReport{Lane: "vector", State: LaneStale, Reason: "active vector source revision differs from requested revision"}
	}
	if status.State != "clean" {
		return nil, LaneReport{Lane: "vector", State: LaneOmitted, Reason: "active index is not clean"}
	}
	if request.ExpectedSpaceSHA256 == "" || request.ExpectedSpaceSHA256 != status.SpaceSHA256 {
		return nil, LaneReport{Lane: "vector", State: LaneOmitted, Reason: "requested and active embedding spaces differ"}
	}
	query, err := request.Embedder.EmbedQuery(ctx, request.Query)
	if err != nil {
		return nil, LaneReport{Lane: "vector", State: LaneDegraded, Reason: vectorFailureReason(err)}
	}
	if query.Status.Mode != embedding.ServiceReady || query.Space.SHA256 != status.SpaceSHA256 || len(query.Value) != status.Dimensions {
		return nil, LaneReport{Lane: "vector", State: LaneDegraded, Reason: "embedding response drifted from active index space"}
	}
	items, err := r.source.Vector(ctx, request.ProjectID, query.Value, status.Dimensions, request.LaneLimit)
	if err != nil {
		return nil, LaneReport{Lane: "vector", State: LaneDegraded, Reason: bounded(err.Error())}
	}
	mark(items, "vector", AuthorityVector, false, nil)
	return items, lane("vector", len(items), false, "")
}

func mark(items []Candidate, laneName string, authority AuthorityClass, stale bool, signal func(*Signals)) {
	for i := range items {
		items[i].Authority = authority
		items[i].MatchedLanes = append(items[i].MatchedLanes, laneName)
		items[i].Signals.Stale = stale
		if signal != nil {
			signal(&items[i].Signals)
		}
	}
}

func lane(name string, count int, stale bool, reason string) LaneReport {
	state := LaneUsed
	if count == 0 {
		state = LaneEmpty
	}
	if stale {
		state = LaneStale
	}
	return LaneReport{Lane: name, State: state, Count: count, Reason: reason}
}

func deduplicate(candidates []Candidate) []Candidate {
	merged := make(map[string]Candidate, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		key := candidate.ChunkID
		if key == "" {
			key = candidate.Identity
		}
		existing, ok := merged[key]
		if !ok {
			candidate.MatchedLanes = unique(candidate.MatchedLanes)
			merged[key] = candidate
			order = append(order, key)
			continue
		}
		existing.Signals = mergeSignals(existing.Signals, candidate.Signals)
		existing.MatchedLanes = unique(append(existing.MatchedLanes, candidate.MatchedLanes...))
		if authorityOrder[candidate.Authority] < authorityOrder[existing.Authority] {
			existing.Authority = candidate.Authority
		}
		merged[key] = existing
	}
	result := make([]Candidate, 0, len(order))
	for _, key := range order {
		result = append(result, merged[key])
	}
	return result
}

func mergeSignals(left, right Signals) Signals {
	left.DirectTaskReference = left.DirectTaskReference || right.DirectTaskReference
	left.ExactPath = left.ExactPath || right.ExactPath
	left.ExactSymbol = left.ExactSymbol || right.ExactSymbol
	left.ExactText = left.ExactText || right.ExactText
	left.Structural = left.Structural || right.Structural
	left.LexicalScore = max(left.LexicalScore, right.LexicalScore)
	left.VectorScore = max(left.VectorScore, right.VectorScore)
	left.AcceptedArchitecture = left.AcceptedArchitecture || right.AcceptedArchitecture
	left.RecentPriorUse = left.RecentPriorUse || right.RecentPriorUse
	left.Stale = left.Stale || right.Stale
	left.LowAuthority = left.LowAuthority || right.LowAuthority
	return left
}

func normalized(values []string, lower bool) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if lower {
			value = strings.ToLower(value)
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func unique(values []string) []string {
	values = normalized(values, false)
	sort.Strings(values)
	return values
}
func staleReason(stale bool) string {
	if stale {
		return "active index source revision differs from requested revision"
	}
	return ""
}
func vectorFailureReason(err error) string {
	var adapter *embedding.AdapterError
	if errors.As(err, &adapter) {
		return string(adapter.Kind) + ": " + bounded(adapter.Detail)
	}
	return bounded(err.Error())
}
func bounded(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 256 {
		return value[:256]
	}
	return value
}
