package retrieval

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

const EvaluationSchemaVersion = "revolvr-retrieval-evaluation-v1"

type ExpectedHit struct {
	Path     string `json:"path"`
	Symbol   string `json:"symbol,omitempty"`
	Language string `json:"language"`
}

type Fixture struct {
	ID           string        `json:"id"`
	Project      string        `json:"project,omitempty"`
	Query        string        `json:"query"`
	ExactPaths   []string      `json:"exact_paths,omitempty"`
	ExactSymbols []string      `json:"exact_symbols,omitempty"`
	Expected     []ExpectedHit `json:"expected"`
}

type QualityMetrics struct {
	QueryCount              int                        `json:"query_count"`
	RecallAt5               float64                    `json:"recall_at_5"`
	RecallAt10              float64                    `json:"recall_at_10"`
	MRR                     float64                    `json:"mrr"`
	ExactSymbolPreservation float64                    `json:"exact_symbol_preservation"`
	MeanLatencyNanoseconds  int64                      `json:"mean_latency_nanoseconds"`
	P95LatencyNanoseconds   int64                      `json:"p95_latency_nanoseconds"`
	QueriesPerSecond        float64                    `json:"queries_per_second"`
	LanguageBreakdown       map[string]LanguageMetrics `json:"language_breakdown"`
}

type LanguageMetrics struct {
	FixtureCount int     `json:"fixture_count"`
	RecallAt5    float64 `json:"recall_at_5"`
	RecallAt10   float64 `json:"recall_at_10"`
	MRR          float64 `json:"mrr"`
}

type EvaluationRunner func(context.Context, Fixture) (Result, error)

func Evaluate(ctx context.Context, fixtures []Fixture, run EvaluationRunner) (QualityMetrics, error) {
	if len(fixtures) == 0 || run == nil {
		return QualityMetrics{}, errors.New("retrieval evaluation requires fixtures and a runner")
	}
	metrics := QualityMetrics{QueryCount: len(fixtures), LanguageBreakdown: map[string]LanguageMetrics{}}
	latencies := make([]int64, 0, len(fixtures))
	started := time.Now()
	exactSymbolFixtures, preservedSymbols := 0, 0
	for _, fixture := range fixtures {
		if fixture.ID == "" || fixture.Query == "" || len(fixture.Expected) == 0 {
			return QualityMetrics{}, errors.New("retrieval evaluation fixture is incomplete")
		}
		queryStarted := time.Now()
		result, err := run(ctx, fixture)
		latency := time.Since(queryStarted)
		if err != nil {
			return QualityMetrics{}, err
		}
		latencies = append(latencies, latency.Nanoseconds())
		r5, r10, reciprocal := scoreFixture(fixture, result.Candidates)
		metrics.RecallAt5 += r5
		metrics.RecallAt10 += r10
		metrics.MRR += reciprocal
		seenLanguages := map[string]struct{}{}
		for _, expected := range fixture.Expected {
			if exactSymbolRequested(fixture, expected.Symbol) {
				exactSymbolFixtures++
				if symbolInTop(expected, result.Candidates, 10) {
					preservedSymbols++
				}
			}
			if _, seen := seenLanguages[expected.Language]; seen {
				continue
			}
			seenLanguages[expected.Language] = struct{}{}
			language := metrics.LanguageBreakdown[expected.Language]
			language.FixtureCount++
			language.RecallAt5 += r5
			language.RecallAt10 += r10
			language.MRR += reciprocal
			metrics.LanguageBreakdown[expected.Language] = language
		}
	}
	count := float64(len(fixtures))
	metrics.RecallAt5 /= count
	metrics.RecallAt10 /= count
	metrics.MRR /= count
	if exactSymbolFixtures > 0 {
		metrics.ExactSymbolPreservation = float64(preservedSymbols) / float64(exactSymbolFixtures)
	}
	for language, value := range metrics.LanguageBreakdown {
		count := float64(value.FixtureCount)
		value.RecallAt5 /= count
		value.RecallAt10 /= count
		value.MRR /= count
		metrics.LanguageBreakdown[language] = value
	}
	var total int64
	for _, value := range latencies {
		total += value
	}
	metrics.MeanLatencyNanoseconds = total / int64(len(latencies))
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	metrics.P95LatencyNanoseconds = latencies[(len(latencies)*95+99)/100-1]
	elapsed := time.Since(started).Seconds()
	if elapsed > 0 {
		metrics.QueriesPerSecond = float64(len(fixtures)) / elapsed
	}
	return metrics, nil
}

func exactSymbolRequested(fixture Fixture, symbol string) bool {
	for _, requested := range fixture.ExactSymbols {
		if strings.EqualFold(strings.TrimSpace(requested), strings.TrimSpace(symbol)) && strings.TrimSpace(symbol) != "" {
			return true
		}
	}
	return false
}

func scoreFixture(fixture Fixture, candidates []Candidate) (float64, float64, float64) {
	hits5, hits10 := map[int]struct{}{}, map[int]struct{}{}
	first := 0
	for rank, candidate := range candidates {
		for expectedIndex, expected := range fixture.Expected {
			if candidate.Path != expected.Path || (expected.Symbol != "" && candidate.Symbol != expected.Symbol) {
				continue
			}
			if first == 0 {
				first = rank + 1
			}
			if rank < 5 {
				hits5[expectedIndex] = struct{}{}
			}
			if rank < 10 {
				hits10[expectedIndex] = struct{}{}
			}
		}
	}
	total := float64(len(fixture.Expected))
	reciprocal := 0.0
	if first > 0 {
		reciprocal = 1 / float64(first)
	}
	return float64(len(hits5)) / total, float64(len(hits10)) / total, reciprocal
}

func symbolInTop(expected ExpectedHit, candidates []Candidate, limit int) bool {
	for index, candidate := range candidates {
		if index >= limit {
			break
		}
		if candidate.Path == expected.Path && candidate.Symbol == expected.Symbol {
			return true
		}
	}
	return false
}
