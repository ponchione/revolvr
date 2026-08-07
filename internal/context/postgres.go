package context

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	storage "revolvr/internal/storage/postgres"
)

type PersistResult struct {
	PackageID string
	Replay    bool
}

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("context package PostgreSQL store requires a pool")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Persist(ctx stdctx.Context, value Package, createdAt time.Time) (PersistResult, error) {
	if createdAt.IsZero() || value.ID == "" || value.Manifest.DossierSHA256 != hash(value.Dossier) || value.Manifest.FinalBytes != len(value.Dossier) || value.Manifest.FinalTokens != estimateTokens(len(value.Dossier)) {
		return PersistResult{}, errors.New("persist context package: package bytes or identity are divergent")
	}
	manifestRaw, err := marshalCanonical(value.Manifest)
	if err != nil {
		return PersistResult{}, err
	}
	if value.ID != deterministicID("context-package", hash(manifestRaw)) {
		return PersistResult{}, errors.New("persist context package: package identity differs from manifest")
	}
	retrievalRaw, err := marshalCanonical(value.Manifest.RetrievalConfiguration)
	if err != nil {
		return PersistResult{}, err
	}
	var dossier Dossier
	if err := json.Unmarshal(value.Dossier, &dossier); err != nil || dossier.SchemaVersion != PackageSchemaVersion || dossier.Role != value.Manifest.Role || dossier.SourceRevision != value.Manifest.SourceRevision {
		return PersistResult{}, errors.New("persist context package: dossier is not the declared package")
	}
	dossierItems := make(map[string]DossierItem, len(dossier.Items))
	for _, item := range dossier.Items {
		dossierItems[item.CandidateIdentity] = item
	}
	result := PersistResult{PackageID: value.ID}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		packageID, err := contextUUID(value.ID)
		if err != nil {
			return err
		}
		existing, err := queries.GetContextPackage(ctx, packageID)
		if err == nil {
			if !bytes.Equal(existing.Dossier, value.Dossier) || !equalJSON(existing.Manifest, manifestRaw) {
				return errors.New("persist context package: immutable identity collision or divergent replay")
			}
			result.Replay = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		projectID, err := contextUUID(value.Manifest.ProjectID)
		if err != nil {
			return err
		}
		spaceID := pgtype.UUID{}
		if value.Manifest.EmbeddingSpaceSHA256 != "" {
			space, err := queries.GetEmbeddingSpaceBySHA256(ctx, value.Manifest.EmbeddingSpaceSHA256)
			if err != nil {
				return errors.New("persist context package: unresolved embedding space")
			}
			spaceID = space.ID
		}
		if err := queries.InsertContextPackage(ctx, storage.InsertContextPackageParams{
			ID: packageID, ProjectID: projectID, TaskID: nullableContextUUID(value.Manifest.TaskID),
			RunID: nullableContextUUID(value.Manifest.RunID), SchemaVersion: PackageSchemaVersion,
			Role: string(value.Manifest.Role), SourceRevision: value.Manifest.SourceRevision,
			EmbeddingSpaceID: spaceID, ByteBudget: int32(value.Manifest.ByteBudget), TokenBudget: int32(value.Manifest.TokenBudget),
			FinalBytes: int32(value.Manifest.FinalBytes), FinalTokens: int32(value.Manifest.FinalTokens), TokenEstimator: TokenEstimator,
			RetrievalConfiguration: retrievalRaw, Manifest: manifestRaw, Dossier: append([]byte(nil), value.Dossier...),
			DossierSha256: value.Manifest.DossierSHA256, CreatedAt: contextTimestamp(createdAt),
		}); err != nil {
			return err
		}
		for _, manifestItem := range value.Manifest.Items {
			dossierItem, included := dossierItems[manifestItem.CandidateIdentity]
			params, err := itemParams(packageID, manifestItem, dossierItem, included)
			if err != nil {
				return err
			}
			if err := queries.InsertContextItem(ctx, params); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PersistResult{}, fmt.Errorf("persist context package: %w", err)
	}
	return result, nil
}

func itemParams(packageID pgtype.UUID, manifest ManifestItem, dossier DossierItem, included bool) (storage.InsertContextItemParams, error) {
	ranking, _ := json.Marshal(manifest.RankingSignals)
	instructions, _ := json.Marshal(manifest.Retrieval)
	params := storage.InsertContextItemParams{
		ContextPackageID: packageID, Ordinal: int32(manifest.Ordinal), CandidateIdentity: manifest.CandidateIdentity,
		AuthorityClass: string(manifest.AuthorityClass), SourceKind: manifest.SourceKind,
		SourceIdentity: manifest.SourceIdentity, SourceSha256: manifest.SourceSHA256,
		SourcePath: contextText(manifest.SourcePath), SymbolName: contextText(manifest.Symbol),
		StartLine: contextInt4(manifest.StartLine), EndLine: contextInt4(manifest.EndLine),
		RankingSignals: ranking, Included: included, StorageForm: string(manifest.StorageForm),
		RetrievalInstructions: instructions, OmissionReason: contextText(manifest.OmissionReason),
	}
	if included != manifest.Included || (included && dossier.StorageForm != manifest.StorageForm) {
		return params, errors.New("persist context package: manifest inclusion differs from dossier")
	}
	if !included {
		return params, nil
	}
	switch dossier.StorageForm {
	case StorageInline:
		params.InlineContent = contextText(dossier.InlineContent)
	case StorageArtifactRange:
		if dossier.ArtifactRange == nil {
			return params, errors.New("persist context package: artifact reference is missing")
		}
		params.ArtifactID = nullableContextUUID(dossier.ArtifactRange.ArtifactID)
		params.ArtifactSha256 = contextText(dossier.ArtifactRange.SHA256)
		params.RangeStart, params.RangeEnd = contextInt8(dossier.ArtifactRange.Start, true), contextInt8(dossier.ArtifactRange.End, true)
		params.MediaType = contextText(dossier.ArtifactRange.MediaType)
	case StorageTrajectoryRange:
		if dossier.TrajectoryRange == nil {
			return params, errors.New("persist context package: trajectory reference is missing")
		}
		params.TrajectoryID = contextText(dossier.TrajectoryRange.TrajectoryID)
		params.TrajectoryStart, params.TrajectoryEnd = contextInt8(dossier.TrajectoryRange.Start, true), contextInt8(dossier.TrajectoryRange.End, true)
		params.MediaType = contextText(dossier.TrajectoryRange.MediaType)
	default:
		return params, errors.New("persist context package: invalid included storage form")
	}
	return params, nil
}

func contextUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
func nullableContextUUID(value string) pgtype.UUID {
	if value == "" {
		return pgtype.UUID{}
	}
	parsed, _ := contextUUID(value)
	return parsed
}
func ctxUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
func contextText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func contextInt4(value int) pgtype.Int4    { return pgtype.Int4{Int32: int32(value), Valid: value > 0} }
func contextInt8(value int64, valid bool) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: valid}
}
func contextTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC().Truncate(time.Microsecond), Valid: true}
}
func equalJSON(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
