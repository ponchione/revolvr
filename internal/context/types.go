// Package context compiles immutable, role-budgeted dossiers from admitted
// retrieval candidates. It cannot mutate canonical state or grant authority.
package context

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"revolvr/internal/gitoid"
	"revolvr/internal/retrieval"
)

const (
	PackageSchemaVersion = "revolvr-context-package-v1"
	TokenEstimator       = "utf8-bytes-ceil-div-4-v1"
)

type Role string

const (
	RoleSupervisor  Role = "supervisor"
	RolePlanner     Role = "planner"
	RoleImplementer Role = "implementer"
	RoleAuditor     Role = "auditor"
	RoleCorrector   Role = "corrector"
	RoleDocumentor  Role = "documentor"
	RoleSimplifier  Role = "simplifier"
)

type Budget struct {
	Bytes  int `json:"bytes"`
	Tokens int `json:"tokens"`
}

func DefaultBudget(role Role) (Budget, error) {
	switch role {
	case RoleSupervisor:
		return Budget{Bytes: 64 << 10, Tokens: 16 << 10}, nil
	case RolePlanner:
		return Budget{Bytes: 128 << 10, Tokens: 32 << 10}, nil
	case RoleImplementer:
		return Budget{Bytes: 192 << 10, Tokens: 48 << 10}, nil
	case RoleAuditor:
		return Budget{Bytes: 192 << 10, Tokens: 48 << 10}, nil
	case RoleCorrector:
		return Budget{Bytes: 128 << 10, Tokens: 32 << 10}, nil
	case RoleDocumentor, RoleSimplifier:
		return Budget{Bytes: 96 << 10, Tokens: 24 << 10}, nil
	default:
		return Budget{}, errors.New("context package: unsupported role")
	}
}

type StorageForm string

const (
	StorageInline          StorageForm = "inline"
	StorageArtifactRange   StorageForm = "artifact_range"
	StorageTrajectoryRange StorageForm = "trajectory_range"
	StorageOmitted         StorageForm = "omitted"
)

type ArtifactRange struct {
	ArtifactID string `json:"artifact_id"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"size_bytes"`
	Start      int64  `json:"start"`
	End        int64  `json:"end"`
	MediaType  string `json:"media_type"`
	Resolved   bool   `json:"resolved"`
}

type TrajectoryRange struct {
	TrajectoryID string `json:"trajectory_id"`
	SHA256       string `json:"sha256"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	MediaType    string `json:"media_type"`
	Resolved     bool   `json:"resolved"`
}

type Candidate struct {
	Retrieval     retrieval.Candidate `json:"retrieval"`
	ArtifactRange *ArtifactRange      `json:"artifact_range,omitempty"`
	Trajectory    *TrajectoryRange    `json:"trajectory_range,omitempty"`
}

type RetrievalInstruction struct {
	Method    string `json:"method,omitempty"`
	Identity  string `json:"identity,omitempty"`
	Start     int64  `json:"start,omitempty"`
	End       int64  `json:"end,omitempty"`
	MaxBytes  int64  `json:"max_bytes,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

type DossierItem struct {
	CandidateIdentity string                   `json:"candidate_identity"`
	AuthorityClass    retrieval.AuthorityClass `json:"authority_class"`
	SourceKind        string                   `json:"source_kind"`
	SourceIdentity    string                   `json:"source_identity"`
	SourceSHA256      string                   `json:"source_sha256"`
	SourcePath        string                   `json:"source_path,omitempty"`
	Symbol            string                   `json:"symbol,omitempty"`
	StartLine         int                      `json:"start_line,omitempty"`
	EndLine           int                      `json:"end_line,omitempty"`
	StorageForm       StorageForm              `json:"storage_form"`
	InlineContent     string                   `json:"inline_content,omitempty"`
	ArtifactRange     *ArtifactRange           `json:"artifact_range,omitempty"`
	TrajectoryRange   *TrajectoryRange         `json:"trajectory_range,omitempty"`
	Retrieval         RetrievalInstruction     `json:"retrieval_instruction"`
}

type Dossier struct {
	SchemaVersion  string        `json:"schema_version"`
	Role           Role          `json:"role"`
	TaskID         string        `json:"task_id,omitempty"`
	RunID          string        `json:"run_id,omitempty"`
	SourceRevision string        `json:"source_revision"`
	Items          []DossierItem `json:"items"`
}

type ManifestItem struct {
	Ordinal           int                      `json:"ordinal"`
	CandidateIdentity string                   `json:"candidate_identity"`
	AuthorityClass    retrieval.AuthorityClass `json:"authority_class"`
	SourceKind        string                   `json:"source_kind"`
	SourceIdentity    string                   `json:"source_identity"`
	SourceSHA256      string                   `json:"source_sha256"`
	SourcePath        string                   `json:"source_path,omitempty"`
	Symbol            string                   `json:"symbol,omitempty"`
	StartLine         int                      `json:"start_line,omitempty"`
	EndLine           int                      `json:"end_line,omitempty"`
	RankingSignals    retrieval.Signals        `json:"ranking_signals"`
	Score             float64                  `json:"score"`
	Included          bool                     `json:"included"`
	StorageForm       StorageForm              `json:"storage_form"`
	ByteSize          int                      `json:"byte_size"`
	TokenEstimate     int                      `json:"token_estimate"`
	Retrieval         RetrievalInstruction     `json:"retrieval_instruction"`
	OmissionReason    string                   `json:"omission_reason,omitempty"`
}

type Manifest struct {
	SchemaVersion          string           `json:"schema_version"`
	Role                   Role             `json:"role"`
	ProjectID              string           `json:"project_id"`
	TaskID                 string           `json:"task_id,omitempty"`
	RunID                  string           `json:"run_id,omitempty"`
	SourceRevision         string           `json:"source_revision"`
	EmbeddingSpaceSHA256   string           `json:"embedding_space_sha256,omitempty"`
	RetrievalConfiguration retrieval.Report `json:"retrieval_configuration"`
	ByteBudget             int              `json:"byte_budget"`
	TokenBudget            int              `json:"token_budget"`
	FinalBytes             int              `json:"final_bytes"`
	FinalTokens            int              `json:"final_tokens"`
	TokenEstimator         string           `json:"token_estimator"`
	IncludedCount          int              `json:"included_count"`
	ExcludedCount          int              `json:"excluded_count"`
	Items                  []ManifestItem   `json:"items"`
	DossierSHA256          string           `json:"dossier_sha256"`
}

type Package struct {
	ID       string   `json:"id"`
	Dossier  []byte   `json:"dossier"`
	Manifest Manifest `json:"manifest"`
}

type CompileRequest struct {
	ProjectID              string
	TaskID                 string
	RunID                  string
	Role                   Role
	SourceRevision         string
	EmbeddingSpaceSHA256   string
	Budget                 Budget
	RetrievalConfiguration retrieval.Report
	Candidates             []Candidate
}

func (r CompileRequest) validate() error {
	if strings.TrimSpace(r.ProjectID) == "" || !gitoid.Valid(r.SourceRevision) {
		return errors.New("context package: project and source revision are required")
	}
	if _, err := DefaultBudget(r.Role); err != nil {
		return err
	}
	if r.EmbeddingSpaceSHA256 != "" && !validSHA(r.EmbeddingSpaceSHA256) {
		return errors.New("context package: embedding space must be SHA-256")
	}
	if (r.Budget.Bytes == 0) != (r.Budget.Tokens == 0) || r.Budget.Bytes < 0 || r.Budget.Tokens < 0 || r.Budget.Bytes > 4<<20 || r.Budget.Tokens > 1<<20 {
		return errors.New("context package: invalid role budget")
	}
	return nil
}

func estimateTokens(bytes int) int { return (bytes + 3) / 4 }
func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
func validSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}
func referenceError(name string) error {
	return fmt.Errorf("context package: invalid %s reference", name)
}
