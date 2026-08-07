package context

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/google/uuid"

	"revolvr/internal/retrieval"
)

func Compile(request CompileRequest) (Package, error) {
	if err := request.validate(); err != nil {
		return Package{}, err
	}
	budget := request.Budget
	if budget.Bytes == 0 {
		var err error
		budget, err = DefaultBudget(request.Role)
		if err != nil {
			return Package{}, err
		}
	}
	if configurationRevision := request.RetrievalConfiguration.SourceRevision; configurationRevision != "" && configurationRevision != request.SourceRevision {
		return Package{}, errors.New("context package: retrieval configuration source revision is stale")
	}
	candidates := append([]Candidate(nil), request.Candidates...)
	sort.SliceStable(candidates, func(i, j int) bool {
		left, _ := retrieval.AuthorityPriority(candidates[i].Retrieval.Authority)
		right, _ := retrieval.AuthorityPriority(candidates[j].Retrieval.Authority)
		if left != right {
			return left < right
		}
		if candidates[i].Retrieval.Score != candidates[j].Retrieval.Score {
			return candidates[i].Retrieval.Score > candidates[j].Retrieval.Score
		}
		if candidates[i].Retrieval.Path != candidates[j].Retrieval.Path {
			return candidates[i].Retrieval.Path < candidates[j].Retrieval.Path
		}
		if candidates[i].Retrieval.StartLine != candidates[j].Retrieval.StartLine {
			return candidates[i].Retrieval.StartLine < candidates[j].Retrieval.StartLine
		}
		return candidates[i].Retrieval.Identity < candidates[j].Retrieval.Identity
	})
	dossier := Dossier{
		SchemaVersion: PackageSchemaVersion, Role: request.Role, TaskID: request.TaskID,
		RunID: request.RunID, SourceRevision: request.SourceRevision,
	}
	base, err := marshalCanonical(dossier)
	if err != nil || len(base) > budget.Bytes || estimateTokens(len(base)) > budget.Tokens {
		return Package{}, errors.New("context package: budget cannot contain required dossier identity")
	}
	manifest := Manifest{
		SchemaVersion: PackageSchemaVersion, Role: request.Role, ProjectID: request.ProjectID,
		TaskID: request.TaskID, RunID: request.RunID, SourceRevision: request.SourceRevision,
		EmbeddingSpaceSHA256:   request.EmbeddingSpaceSHA256,
		RetrievalConfiguration: request.RetrievalConfiguration,
		ByteBudget:             budget.Bytes, TokenBudget: budget.Tokens, TokenEstimator: TokenEstimator,
	}
	seen := map[string]struct{}{}
	for index, candidate := range candidates {
		item, alternatives, omission, err := dossierAlternatives(candidate)
		if err != nil {
			return Package{}, err
		}
		if _, duplicate := seen[candidate.Retrieval.Identity]; duplicate {
			return Package{}, errors.New("context package: duplicate candidate identity")
		}
		seen[candidate.Retrieval.Identity] = struct{}{}
		manifestItem := newManifestItem(index+1, candidate)
		included := false
		for _, alternative := range alternatives {
			trial := dossier
			trial.Items = append(append([]DossierItem(nil), dossier.Items...), alternative)
			raw, marshalErr := marshalCanonical(trial)
			if marshalErr != nil {
				return Package{}, marshalErr
			}
			if len(raw) <= budget.Bytes && estimateTokens(len(raw)) <= budget.Tokens {
				dossier = trial
				item = alternative
				included = true
				break
			}
		}
		manifestItem.Included = included
		if included {
			manifestItem.StorageForm = item.StorageForm
			manifestItem.Retrieval = item.Retrieval
			manifestItem.ByteSize = materialBytes(item)
			manifestItem.TokenEstimate = estimateTokens(manifestItem.ByteSize)
			manifest.IncludedCount++
		} else {
			manifestItem.StorageForm = StorageOmitted
			manifestItem.OmissionReason = firstNonempty(omission, "role_budget_exceeded")
			manifestItem.Retrieval = item.Retrieval
			for _, alternative := range alternatives {
				if alternative.StorageForm == StorageArtifactRange || alternative.StorageForm == StorageTrajectoryRange {
					manifestItem.Retrieval = alternative.Retrieval
				}
			}
			manifest.ExcludedCount++
		}
		manifest.Items = append(manifest.Items, manifestItem)
	}
	raw, err := marshalCanonical(dossier)
	if err != nil {
		return Package{}, err
	}
	manifest.FinalBytes = len(raw)
	manifest.FinalTokens = estimateTokens(len(raw))
	manifest.DossierSHA256 = hash(raw)
	manifestRaw, err := marshalCanonical(manifest)
	if err != nil {
		return Package{}, err
	}
	id := deterministicID("context-package", hash(manifestRaw))
	return Package{ID: id, Dossier: raw, Manifest: manifest}, nil
}

func dossierAlternatives(candidate Candidate) (DossierItem, []DossierItem, string, error) {
	value := candidate.Retrieval
	if value.Identity == "" || value.SourceKind == "" || value.SourceIdentity == "" || !validSHA(value.SourceSHA256) {
		return DossierItem{}, nil, "", errors.New("context package: candidate provenance is incomplete")
	}
	if _, ok := retrieval.AuthorityPriority(value.Authority); !ok {
		return DossierItem{}, nil, "", errors.New("context package: candidate authority is unknown")
	}
	if candidate.ArtifactRange != nil && candidate.Trajectory != nil {
		return DossierItem{}, nil, "", errors.New("context package: candidate has multiple reference forms")
	}
	base := DossierItem{
		CandidateIdentity: value.Identity, AuthorityClass: value.Authority,
		SourceKind: value.SourceKind, SourceIdentity: value.SourceIdentity, SourceSHA256: value.SourceSHA256,
		SourcePath: value.Path, Symbol: value.Symbol, StartLine: value.StartLine, EndLine: value.EndLine,
	}
	manifestFallback := base
	omission := ""
	var alternatives []DossierItem
	if value.Content != "" {
		if hash([]byte(value.Content)) != value.SourceSHA256 {
			return DossierItem{}, nil, "", errors.New("context package: inline content differs from source hash")
		}
		inline := base
		inline.StorageForm = StorageInline
		inline.InlineContent = value.Content
		alternatives = append(alternatives, inline)
	}
	if reference := candidate.ArtifactRange; reference != nil {
		if reference.ArtifactID == "" || !validSHA(reference.SHA256) || reference.SizeBytes < 0 || reference.Start < 0 || reference.End <= reference.Start || reference.End > reference.SizeBytes || reference.End-reference.Start > MaximumQueryRangeBytes || reference.MediaType == "" {
			return DossierItem{}, nil, "", referenceError("artifact range")
		}
		item := base
		item.StorageForm, item.ArtifactRange = StorageArtifactRange, cloneArtifact(reference)
		item.Retrieval = RetrievalInstruction{Method: "host_query.artifact_range", Identity: reference.ArtifactID, Start: reference.Start, End: reference.End, MaxBytes: reference.End - reference.Start, MediaType: reference.MediaType, SHA256: reference.SHA256}
		manifestFallback = item
		if reference.Resolved {
			alternatives = append(alternatives, item)
		} else {
			omission = "unresolved_artifact_reference"
		}
	}
	if reference := candidate.Trajectory; reference != nil {
		if reference.TrajectoryID == "" || !validSHA(reference.SHA256) || reference.Start < 0 || reference.End <= reference.Start || reference.End-reference.Start > MaximumQueryRangeBytes || reference.MediaType == "" {
			return DossierItem{}, nil, "", referenceError("trajectory range")
		}
		item := base
		item.StorageForm, item.TrajectoryRange = StorageTrajectoryRange, cloneTrajectory(reference)
		item.Retrieval = RetrievalInstruction{Method: "host_query.trajectory_range", Identity: reference.TrajectoryID, Start: reference.Start, End: reference.End, MaxBytes: reference.End - reference.Start, MediaType: reference.MediaType, SHA256: reference.SHA256}
		manifestFallback = item
		if reference.Resolved {
			alternatives = append(alternatives, item)
		} else {
			omission = "unresolved_trajectory_reference"
		}
	}
	if len(alternatives) == 0 {
		return manifestFallback, nil, firstNonempty(omission, "content_or_reference_missing"), nil
	}
	return manifestFallback, alternatives, omission, nil
}

func newManifestItem(ordinal int, candidate Candidate) ManifestItem {
	value := candidate.Retrieval
	return ManifestItem{
		Ordinal: ordinal, CandidateIdentity: value.Identity, AuthorityClass: value.Authority,
		SourceKind: value.SourceKind, SourceIdentity: value.SourceIdentity, SourceSHA256: value.SourceSHA256,
		SourcePath: value.Path, Symbol: value.Symbol, StartLine: value.StartLine, EndLine: value.EndLine,
		RankingSignals: value.Signals, Score: value.Score,
	}
}

func materialBytes(item DossierItem) int {
	switch item.StorageForm {
	case StorageInline:
		return len(item.InlineContent)
	case StorageArtifactRange:
		return int(item.ArtifactRange.End - item.ArtifactRange.Start)
	case StorageTrajectoryRange:
		return int(item.TrajectoryRange.End - item.TrajectoryRange.Start)
	default:
		return 0
	}
}

func marshalCanonical(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) || bytes.Contains(raw, []byte("\n")) {
		return nil, errors.New("context package: canonical JSON encoding failed")
	}
	return raw, nil
}

func deterministicID(parts ...string) string {
	material := ""
	for _, part := range parts {
		material += strconv.Itoa(len(part)) + ":" + part
	}
	raw, _ := hexDecode(hash([]byte(material)))
	raw = raw[:16]
	raw[6] = (raw[6] & 0x0f) | 0x50
	raw[8] = (raw[8] & 0x3f) | 0x80
	id, _ := uuid.FromBytes(raw)
	return id.String()
}

func hexDecode(value string) ([]byte, error) {
	result := make([]byte, len(value)/2)
	for i := range result {
		parsed, err := strconv.ParseUint(value[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		result[i] = byte(parsed)
	}
	return result, nil
}

func cloneArtifact(value *ArtifactRange) *ArtifactRange {
	copyValue := *value
	return &copyValue
}
func cloneTrajectory(value *TrajectoryRange) *TrajectoryRange {
	copyValue := *value
	return &copyValue
}
func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
