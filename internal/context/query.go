package context

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"revolvr/internal/evidence"
	"revolvr/internal/runtimepath"
	storage "revolvr/internal/storage/postgres"
)

const (
	MaximumQueryItems        = 256
	MaximumQueryRangeBytes   = int64(256 << 10)
	MaximumArtifactReadBytes = int64(64 << 20)
)

var (
	ErrQueryBoundExceeded    = errors.New("context host query bound exceeded")
	ErrReferenceNotAdmitted  = errors.New("context reference is not admitted")
	ErrTrajectoryUnavailable = errors.New("trajectory-range service is reserved and unavailable")
)

type AdmittedItem struct {
	Ordinal           int                  `json:"ordinal"`
	CandidateIdentity string               `json:"candidate_identity"`
	AuthorityClass    string               `json:"authority_class"`
	SourceKind        string               `json:"source_kind"`
	SourceIdentity    string               `json:"source_identity"`
	SourceSHA256      string               `json:"source_sha256"`
	StorageForm       StorageForm          `json:"storage_form"`
	InlineContent     string               `json:"inline_content,omitempty"`
	Retrieval         RetrievalInstruction `json:"retrieval_instruction"`
}

type RangeResult struct {
	Identity  string `json:"identity"`
	SHA256    string `json:"sha256"`
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	MediaType string `json:"media_type"`
	Content   []byte `json:"content"`
}

type ArtifactResolver interface {
	ResolveArtifactRange(stdctx.Context, string, string, int64, int64, int64) (RangeResult, error)
}

type TrajectoryResolver interface {
	ResolveTrajectoryRange(stdctx.Context, string, string, int64, int64, int64) (RangeResult, error)
}

// HostQuery is deliberately read-only. It exposes no database handle and no
// storage, lifecycle, policy, verification, completion, audit, or correction
// operation.
type HostQuery struct {
	queries    *storage.Queries
	artifacts  ArtifactResolver
	trajectory TrajectoryResolver
}

func NewHostQuery(pool *pgxpool.Pool, artifacts ArtifactResolver, trajectory TrajectoryResolver) (*HostQuery, error) {
	if pool == nil {
		return nil, errors.New("context host query requires a PostgreSQL pool")
	}
	return &HostQuery{queries: storage.New(pool), artifacts: artifacts, trajectory: trajectory}, nil
}

func (q *HostQuery) Manifest(ctx stdctx.Context, packageID string) (Manifest, error) {
	id, err := contextUUID(packageID)
	if err != nil {
		return Manifest{}, err
	}
	row, err := q.queries.GetContextPackage(ctx, id)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := strictJSON(row.Manifest, &manifest); err != nil || manifest.DossierSHA256 != row.DossierSha256 || hash(row.Dossier) != row.DossierSha256 || manifest.FinalBytes != len(row.Dossier) {
		return Manifest{}, errors.New("context host query: immutable package validation failed")
	}
	return manifest, nil
}

func (q *HostQuery) AdmittedItems(ctx stdctx.Context, packageID string, maximum int) ([]AdmittedItem, error) {
	if maximum <= 0 || maximum > MaximumQueryItems {
		return nil, ErrQueryBoundExceeded
	}
	id, err := contextUUID(packageID)
	if err != nil {
		return nil, err
	}
	rows, err := q.queries.ListContextItems(ctx, storage.ListContextItemsParams{ContextPackageID: id, Limit: int32(maximum + 1)})
	if err != nil {
		return nil, err
	}
	result := make([]AdmittedItem, 0, min(len(rows), maximum))
	for _, row := range rows {
		if !row.Included {
			continue
		}
		if len(result) == maximum {
			return nil, ErrQueryBoundExceeded
		}
		var instruction RetrievalInstruction
		if err := strictJSON(row.RetrievalInstructions, &instruction); err != nil {
			return nil, err
		}
		result = append(result, AdmittedItem{
			Ordinal: int(row.Ordinal), CandidateIdentity: row.CandidateIdentity,
			AuthorityClass: row.AuthorityClass, SourceKind: row.SourceKind,
			SourceIdentity: row.SourceIdentity, SourceSHA256: row.SourceSha256,
			StorageForm: StorageForm(row.StorageForm), InlineContent: row.InlineContent.String,
			Retrieval: instruction,
		})
	}
	return result, nil
}

func (q *HostQuery) ArtifactRange(ctx stdctx.Context, packageID, candidateIdentity string, start, end int64) (RangeResult, error) {
	row, err := q.referenceItem(ctx, packageID, candidateIdentity)
	if err != nil {
		return RangeResult{}, err
	}
	if !row.Included || row.StorageForm != string(StorageArtifactRange) || !row.ArtifactID.Valid || !row.ArtifactSha256.Valid || !row.RangeStart.Valid || !row.RangeEnd.Valid || start != row.RangeStart.Int64 || end != row.RangeEnd.Int64 || end-start > MaximumQueryRangeBytes {
		return RangeResult{}, ErrReferenceNotAdmitted
	}
	if q.artifacts == nil {
		return RangeResult{}, errors.New("context host query: artifact resolver is unavailable")
	}
	return q.artifacts.ResolveArtifactRange(ctx, ctxUUIDString(row.ArtifactID), row.ArtifactSha256.String, start, end, MaximumQueryRangeBytes)
}

func (q *HostQuery) TrajectoryRange(ctx stdctx.Context, packageID, candidateIdentity string, start, end int64) (RangeResult, error) {
	row, err := q.referenceItem(ctx, packageID, candidateIdentity)
	if err != nil {
		return RangeResult{}, err
	}
	if !row.Included || row.StorageForm != string(StorageTrajectoryRange) || !row.TrajectoryID.Valid || !row.TrajectoryStart.Valid || !row.TrajectoryEnd.Valid || start != row.TrajectoryStart.Int64 || end != row.TrajectoryEnd.Int64 || end-start > MaximumQueryRangeBytes {
		return RangeResult{}, ErrReferenceNotAdmitted
	}
	var instruction RetrievalInstruction
	if strictJSON(row.RetrievalInstructions, &instruction) != nil || instruction.Method != "host_query.trajectory_range" || instruction.Identity != row.TrajectoryID.String || instruction.Start != start || instruction.End != end || instruction.MaxBytes != end-start || instruction.MediaType != row.MediaType.String || !validSHA(instruction.SHA256) {
		return RangeResult{}, ErrReferenceNotAdmitted
	}
	if q.trajectory == nil {
		return RangeResult{}, ErrTrajectoryUnavailable
	}
	return q.trajectory.ResolveTrajectoryRange(ctx, row.TrajectoryID.String, instruction.SHA256, start, end, MaximumQueryRangeBytes)
}

func (q *HostQuery) referenceItem(ctx stdctx.Context, packageID, candidateIdentity string) (storage.TelemetryContextItem, error) {
	id, err := contextUUID(packageID)
	if err != nil || candidateIdentity == "" {
		return storage.TelemetryContextItem{}, ErrReferenceNotAdmitted
	}
	return q.queries.GetContextItemByCandidate(ctx, storage.GetContextItemByCandidateParams{ContextPackageID: id, CandidateIdentity: candidateIdentity})
}

type FilesystemArtifactResolver struct {
	queries  *storage.Queries
	boundary runtimepath.Boundary
}

func NewFilesystemArtifactResolver(pool *pgxpool.Pool, artifactRoot string) (*FilesystemArtifactResolver, error) {
	if pool == nil {
		return nil, errors.New("artifact range resolver requires a PostgreSQL pool")
	}
	boundary, err := runtimepath.Bind(artifactRoot)
	if err != nil {
		return nil, err
	}
	return &FilesystemArtifactResolver{queries: storage.New(pool), boundary: boundary}, nil
}

func (r *FilesystemArtifactResolver) ResolveArtifactRange(ctx stdctx.Context, artifactID, expectedSHA string, start, end, maximum int64) (RangeResult, error) {
	if maximum <= 0 || maximum > MaximumQueryRangeBytes || start < 0 || end <= start || end-start > maximum {
		return RangeResult{}, ErrQueryBoundExceeded
	}
	id, err := contextUUID(artifactID)
	if err != nil {
		return RangeResult{}, err
	}
	row, err := r.queries.GetArtifactByID(ctx, id)
	if err != nil {
		return RangeResult{}, err
	}
	if row.Sha256 != expectedSHA || row.SizeBytes < end || row.SizeBytes > MaximumArtifactReadBytes {
		return RangeResult{}, ErrReferenceNotAdmitted
	}
	path := filepath.Clean(row.StoragePath)
	content, found, err := r.boundary.ReadFileLimit(path, false, MaximumArtifactReadBytes)
	if err != nil || !found {
		return RangeResult{}, errors.Join(err, ErrReferenceNotAdmitted)
	}
	if int64(len(content)) != row.SizeBytes || evidence.HashBytes(content) != row.Sha256 {
		return RangeResult{}, errors.New("context host query: artifact bytes differ from immutable identity")
	}
	return RangeResult{Identity: artifactID, SHA256: row.Sha256, Start: start, End: end, MediaType: row.MediaType, Content: append([]byte(nil), content[start:end]...)}, nil
}

func strictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("context host query: trailing JSON")
	}
	return nil
}
