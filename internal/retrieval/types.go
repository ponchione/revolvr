// Package retrieval combines exact, structural, lexical, and optional vector
// lanes without granting derived results canonical authority.
package retrieval

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"

	"revolvr/internal/embedding"
	codeindex "revolvr/internal/index"
)

const ConfigurationVersion = "revolvr-hybrid-retrieval-v2"

type AuthorityClass string

const (
	AuthorityCanonicalTask     AuthorityClass = "canonical_task"
	AuthorityExactSource       AuthorityClass = "exact_source"
	AuthorityHostPolicy        AuthorityClass = "host_policy"
	AuthorityCanonicalEvidence AuthorityClass = "canonical_evidence"
	AuthorityStructural        AuthorityClass = "structural_source"
	AuthorityLexical           AuthorityClass = "lexical_source"
	AuthorityVector            AuthorityClass = "vector_source"
	AuthorityAdvisory          AuthorityClass = "model_advisory"
)

var authorityOrder = map[AuthorityClass]int{
	AuthorityCanonicalTask: 0, AuthorityExactSource: 1, AuthorityHostPolicy: 2,
	AuthorityCanonicalEvidence: 3, AuthorityStructural: 4, AuthorityLexical: 5,
	AuthorityVector: 6, AuthorityAdvisory: 7,
}

func AuthorityPriority(class AuthorityClass) (int, bool) {
	priority, ok := authorityOrder[class]
	return priority, ok
}

type Signals struct {
	DirectTaskReference  bool    `json:"direct_task_reference"`
	ExactPath            bool    `json:"exact_path"`
	ExactSymbol          bool    `json:"exact_symbol"`
	ExactText            bool    `json:"exact_text"`
	Structural           bool    `json:"structural"`
	LexicalScore         float64 `json:"lexical_score"`
	VectorScore          float64 `json:"vector_score"`
	AcceptedArchitecture bool    `json:"accepted_architecture"`
	RecentPriorUse       bool    `json:"recent_prior_use"`
	Stale                bool    `json:"stale"`
	LowAuthority         bool    `json:"low_authority"`
}

func (s Signals) Score() float64 {
	score := clamp(s.LexicalScore)*30 + clamp(s.VectorScore)*30
	if s.ExactPath {
		score += 100
	}
	if s.ExactSymbol {
		score += 80
	}
	if s.DirectTaskReference {
		score += 70
	}
	if s.ExactText {
		score += 35
	}
	if s.Structural {
		score += 15
	}
	if s.AcceptedArchitecture {
		score += 10
	}
	if s.RecentPriorUse {
		score += 5
	}
	if s.Stale {
		score -= 20
	}
	if s.LowAuthority {
		score -= 15
	}
	return score
}

type Candidate struct {
	Identity       string         `json:"identity"`
	ChunkID        string         `json:"chunk_id,omitempty"`
	Authority      AuthorityClass `json:"authority"`
	SourceKind     string         `json:"source_kind"`
	SourceIdentity string         `json:"source_identity"`
	SourceRevision string         `json:"source_revision"`
	SourceSHA256   string         `json:"source_sha256"`
	Path           string         `json:"path,omitempty"`
	Symbol         string         `json:"symbol,omitempty"`
	Language       string         `json:"language,omitempty"`
	Kind           string         `json:"kind,omitempty"`
	Signature      string         `json:"signature,omitempty"`
	StartLine      int            `json:"start_line,omitempty"`
	EndLine        int            `json:"end_line,omitempty"`
	Content        string         `json:"content,omitempty"`
	Signals        Signals        `json:"signals"`
	Score          float64        `json:"score"`
	MatchedLanes   []string       `json:"matched_lanes"`
}

type LaneState string

const (
	LaneUsed     LaneState = "used"
	LaneEmpty    LaneState = "empty"
	LaneOmitted  LaneState = "omitted"
	LaneStale    LaneState = "stale"
	LaneDegraded LaneState = "degraded"
)

type LaneReport struct {
	Lane   string    `json:"lane"`
	State  LaneState `json:"state"`
	Count  int       `json:"count"`
	Reason string    `json:"reason,omitempty"`
}

type Report struct {
	ConfigurationVersion   string       `json:"configuration_version"`
	SourceRevision         string       `json:"source_revision"`
	ActiveSourceRevision   string       `json:"active_source_revision,omitempty"`
	EmbeddingSpace         string       `json:"embedding_space,omitempty"`
	QueryInstructionSHA256 string       `json:"query_instruction_sha256,omitempty"`
	Lanes                  []LaneReport `json:"lanes"`
}

type Request struct {
	ProjectID              string
	SourceRevision         string
	Canonical              []Candidate
	ExactPaths             []string
	ExactSymbols           []string
	ExactText              string
	Query                  string
	Limit                  int
	LaneLimit              int
	ExpectedSpaceSHA256    string
	QueryInstructionSHA256 string
	Embedder               embedding.Embedder
}

type Result struct {
	Candidates []Candidate `json:"candidates"`
	Report     Report      `json:"report"`
}

type IndexStatus struct {
	State          string
	SourceRevision string
	SpaceSHA256    string
	Dimensions     int
}

type Source interface {
	Status(context.Context, string) (IndexStatus, error)
	ExactFiles(context.Context, string, []string, int) ([]Candidate, error)
	ExactSymbols(context.Context, string, []string, int) ([]Candidate, error)
	ExactText(context.Context, string, string, int) ([]Candidate, error)
	Structural(context.Context, string, []string, int) ([]Candidate, error)
	FTS(context.Context, string, string, int) ([]Candidate, error)
	Vector(context.Context, string, []float32, int, int) ([]Candidate, error)
}

var ErrInvalidRequest = errors.New("invalid retrieval request")

func validateRequest(request Request) error {
	if strings.TrimSpace(request.ProjectID) == "" || strings.TrimSpace(request.SourceRevision) == "" {
		return ErrInvalidRequest
	}
	if request.Limit == 0 {
		request.Limit = 30
	}
	if request.Limit < 1 || request.Limit > 500 || request.LaneLimit < 0 || request.LaneLimit > 500 {
		return ErrInvalidRequest
	}
	for _, candidate := range request.Canonical {
		if candidate.Identity == "" || candidate.SourceSHA256 == "" {
			return ErrInvalidRequest
		}
		if _, ok := authorityOrder[candidate.Authority]; !ok {
			return ErrInvalidRequest
		}
	}
	if request.Embedder != nil && strings.TrimSpace(request.Query) != "" {
		if !validSHA256(request.ExpectedSpaceSHA256) || request.QueryInstructionSHA256 != codeindex.SelectedQueryInstructionSHA256 {
			return ErrInvalidRequest
		}
	}
	return nil
}

func rank(candidates []Candidate) []Candidate {
	for i := range candidates {
		candidates[i].Score = candidates[i].Signals.Score()
		sort.Strings(candidates[i].MatchedLanes)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := authorityOrder[candidates[i].Authority], authorityOrder[candidates[j].Authority]
		if left != right {
			return left < right
		}
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		if candidates[i].StartLine != candidates[j].StartLine {
			return candidates[i].StartLine < candidates[j].StartLine
		}
		return candidates[i].Identity < candidates[j].Identity
	})
	return candidates
}

func clamp(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}
