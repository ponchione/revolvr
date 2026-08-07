package index

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	storage "revolvr/internal/storage/postgres"
)

var (
	ErrBuildConflict = errors.New("code index build authority conflict")
	ErrBuildFailed   = errors.New("code index build failed")
)

type StageResult struct {
	BuildID string
	Replay  bool
}

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	if pool == nil {
		return nil, errors.New("code index PostgreSQL store requires a pool")
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) LoadExisting(ctx context.Context, projectID string) (map[string]ExistingFile, error) {
	project, err := pgUUID(projectID)
	if err != nil {
		return nil, err
	}
	queries := storage.New(s.pool)
	state, err := queries.GetIndexState(ctx, project)
	if errors.Is(err, pgx.ErrNoRows) || !state.ActiveBuildID.Valid {
		return map[string]ExistingFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	spaceSHA := ""
	if state.ActiveEmbeddingSpaceID.Valid {
		space, err := queries.GetEmbeddingSpaceByID(ctx, state.ActiveEmbeddingSpaceID)
		if err != nil {
			return nil, err
		}
		spaceSHA = space.SpaceSha256
	}
	vectors := map[string][]float32{}
	if spaceSHA != "" {
		rows, err := queries.ListActiveChunkEmbeddingIdentities(ctx, project)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			value, err := parseVectorText(row.EmbeddingText)
			if err != nil || len(value) != int(row.Dimensions) {
				return nil, errors.New("code index: active embedding is malformed")
			}
			vectors[uuidString(row.ChunkID)] = value
		}
	}
	documents, err := queries.ListActiveDocuments(ctx, project)
	if err != nil {
		return nil, err
	}
	result := make(map[string]ExistingFile, len(documents))
	for _, document := range documents {
		var provenance StructuralProvenance
		if err := json.Unmarshal(document.StructuralProvenance, &provenance); err != nil {
			return nil, errors.New("code index: active document provenance is malformed")
		}
		parsed := ParsedFile{
			Path: document.FilePath, Language: document.Language,
			ContentSHA256: document.ContentSha256, SizeBytes: int(document.SizeBytes),
			DocumentID: uuidString(document.DocumentID), DocumentVersionID: uuidString(document.DocumentVersionID),
			StructuralProvenance: provenance,
		}
		chunks, err := queries.ListDocumentChunks(ctx, document.DocumentVersionID)
		if err != nil {
			return nil, err
		}
		for _, row := range chunks {
			var chunkProvenance StructuralProvenance
			if err := json.Unmarshal(row.StructuralProvenance, &chunkProvenance); err != nil {
				return nil, err
			}
			parsed.Chunks = append(parsed.Chunks, Chunk{
				ID: uuidString(row.ID), DocumentID: parsed.DocumentID, DocumentVersionID: parsed.DocumentVersionID,
				Ordinal: int(row.ChunkOrdinal), Path: parsed.Path, Language: row.Language, Kind: row.ChunkKind,
				Symbol: row.SymbolName.String, Signature: row.Signature, StartLine: int(row.StartLine), EndLine: int(row.EndLine),
				Body: row.Body, BodySHA256: row.BodySha256, StructuralProvenance: chunkProvenance,
			})
		}
		symbols, err := queries.ListDocumentSymbols(ctx, document.DocumentVersionID)
		if err != nil {
			return nil, err
		}
		for _, row := range symbols {
			parsed.Symbols = append(parsed.Symbols, Symbol{
				ID: uuidString(row.ID), DocumentVersionID: parsed.DocumentVersionID, ChunkID: uuidString(row.ChunkID),
				Name: row.SymbolName, Kind: row.SymbolKind, Signature: row.Signature,
				StartLine: int(row.StartLine), EndLine: int(row.EndLine),
			})
		}
		edges, err := queries.ListDocumentEdges(ctx, document.DocumentVersionID)
		if err != nil {
			return nil, err
		}
		for _, row := range edges {
			var edgeProvenance StructuralProvenance
			if err := json.Unmarshal(row.Provenance, &edgeProvenance); err != nil {
				return nil, err
			}
			parsed.Edges = append(parsed.Edges, Edge{
				ID: uuidString(row.ID), DocumentVersionID: parsed.DocumentVersionID,
				FromSymbolID: uuidString(row.FromSymbolID), Kind: row.EdgeKind, TargetSymbol: row.TargetSymbol,
				TargetPath: row.TargetPath.String, SourceLine: int(row.SourceLine), Provenance: edgeProvenance,
			})
		}
		fileVectors := make(map[string][]float32)
		for _, chunk := range parsed.Chunks {
			if vector, ok := vectors[chunk.ID]; ok {
				fileVectors[chunk.ID] = append([]float32(nil), vector...)
			}
		}
		result[parsed.Path] = ExistingFile{Parsed: parsed, SpaceSHA256: spaceSHA, Vectors: fileVectors}
	}
	return result, nil
}

// MarkDirty records that the registered source advanced while retaining the
// last active index for exact, explicitly stale retrieval.
func (s *PostgresStore) MarkDirty(ctx context.Context, projectID, detail string, at time.Time) error {
	if strings.TrimSpace(detail) == "" || at.IsZero() {
		return errors.New("mark code index dirty: detail and time are required")
	}
	project, err := pgUUID(projectID)
	if err != nil {
		return err
	}
	queries := storage.New(s.pool)
	if _, err := queries.GetIndexState(ctx, project); err != nil {
		return err
	}
	return queries.MarkIndexDirty(ctx, storage.MarkIndexDirtyParams{
		ProjectID: project, Detail: detail, UpdatedAt: timestamp(at.UTC().Truncate(time.Microsecond)),
	})
}

func (s *PostgresStore) Stage(ctx context.Context, build PreparedBuild) (StageResult, error) {
	if err := validatePreparedBuild(build); err != nil {
		return StageResult{}, err
	}
	result := StageResult{BuildID: build.ID}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		projectID, err := pgUUID(build.Snapshot.ProjectID)
		if err != nil {
			return err
		}
		buildID := mustUUID(build.ID)
		if err := queries.InitializeIndexState(ctx, storage.InitializeIndexStateParams{ProjectID: projectID, UpdatedAt: timestamp(build.PreparedAt)}); err != nil {
			return err
		}
		state, err := queries.GetIndexStateForUpdate(ctx, projectID)
		if err != nil {
			return err
		}
		existing, err := queries.GetIndexBuildByOperationID(ctx, build.OperationID)
		if err == nil {
			if uuidString(existing.ID) != build.ID || uuidString(existing.ProjectID) != build.Snapshot.ProjectID || existing.SourceRevision != build.Snapshot.SourceRevision || existing.SourceTree != build.Snapshot.SourceTree || existing.ManifestSha256 != build.Manifest.SHA256 || existing.BuildKind != string(build.Kind) || existing.FileCount != int32(build.Manifest.FileCount) || existing.ChunkCount != int32(build.Manifest.ChunkCount) || existing.SymbolCount != int32(build.Manifest.SymbolCount) || existing.VectorCount != int32(build.Manifest.VectorCount) {
				return ErrBuildConflict
			}
			if existing.Status == "failed" {
				return ErrBuildFailed
			}
			if existing.Status == "clean" {
				result.Replay = true
				return nil
			}
			result.Replay = true
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		} else {
			if state.Status == string(StateBuilding) && state.LastBuildID.Valid {
				return ErrBuildConflict
			}
			spaceID, err := s.ensureEmbeddingSpace(ctx, queries, build)
			if err != nil {
				return err
			}
			if err := queries.InsertIndexBuild(ctx, storage.InsertIndexBuildParams{
				ID: buildID, OperationID: build.OperationID, ProjectID: projectID,
				SourceRevision: build.Snapshot.SourceRevision, SourceTree: build.Snapshot.SourceTree,
				EmbeddingSpaceID: spaceID, PreviousActiveBuildID: state.ActiveBuildID, BuildKind: string(build.Kind),
				ManifestSha256: build.Manifest.SHA256, FileCount: int32(build.Manifest.FileCount),
				ChunkCount: int32(build.Manifest.ChunkCount), SymbolCount: int32(build.Manifest.SymbolCount),
				VectorCount: int32(build.Manifest.VectorCount), CreatedAt: timestamp(build.PreparedAt),
			}); err != nil {
				return err
			}
			if err := queries.SetIndexBuilding(ctx, storage.SetIndexBuildingParams{ProjectID: projectID, LastBuildID: buildID, UpdatedAt: timestamp(build.PreparedAt)}); err != nil {
				return err
			}
		}
		if state.Status == string(StateBuilding) && state.LastBuildID.Valid && uuidString(state.LastBuildID) != build.ID {
			return ErrBuildConflict
		}
		return persistBuildRows(ctx, queries, build, projectID, buildID)
	})
	if err != nil {
		return StageResult{}, fmt.Errorf("stage code index: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) ensureEmbeddingSpace(ctx context.Context, queries *storage.Queries, build PreparedBuild) (pgtype.UUID, error) {
	if build.EmbeddingSpace == nil {
		return pgtype.UUID{}, nil
	}
	evidence := *build.EmbeddingSpace
	id := mustUUID(DeterministicID("embedding-space", evidence.SpaceSHA256))
	model := evidence.Model
	if err := ValidateSelectedEmbeddingModel(model); err != nil {
		return pgtype.UUID{}, err
	}
	if err := queries.InsertEmbeddingSpace(ctx, storage.InsertEmbeddingSpaceParams{
		ID: id, SpaceSha256: evidence.SpaceSHA256, SchemaVersion: embeddingSpaceSchema(),
		ModelName: model.ModelName, ModelRevision: model.Revision, Dimensions: int32(model.Dimensions),
		Pooling: model.Pooling, Normalization: model.Normalization, Quantization: model.Quantization,
		ArtifactSha256: model.ArtifactSHA256, License: evidence.License, SourceUri: evidence.SourceURI,
		ServingImageDigest: evidence.ServingImageDigest, CreatedAt: timestamp(build.PreparedAt),
	}); err != nil {
		return pgtype.UUID{}, err
	}
	row, err := queries.GetEmbeddingSpaceBySHA256(ctx, evidence.SpaceSHA256)
	if err != nil || uuidString(row.ID) != uuidString(id) || row.ModelName != model.ModelName || row.ModelRevision != model.Revision || row.Dimensions != int32(model.Dimensions) || row.Pooling != model.Pooling || row.Normalization != model.Normalization || row.Quantization != model.Quantization || row.ArtifactSha256 != model.ArtifactSHA256 || row.License != evidence.License || row.SourceUri != evidence.SourceURI || row.ServingImageDigest != evidence.ServingImageDigest {
		return pgtype.UUID{}, ErrBuildConflict
	}
	return id, nil
}

func persistBuildRows(ctx context.Context, queries *storage.Queries, build PreparedBuild, projectID, buildID pgtype.UUID) error {
	spaceID := pgtype.UUID{}
	if build.EmbeddingSpace != nil {
		spaceID = mustUUID(DeterministicID("embedding-space", build.EmbeddingSpace.SpaceSHA256))
	}
	for _, file := range build.Files {
		documentID, versionID := mustUUID(file.DocumentID), mustUUID(file.DocumentVersionID)
		provenance, _ := json.Marshal(file.StructuralProvenance)
		if err := queries.InsertRetrievalDocument(ctx, storage.InsertRetrievalDocumentParams{ID: documentID, ProjectID: projectID, FilePath: file.Path, Language: file.Language, CreatedAt: timestamp(build.PreparedAt)}); err != nil {
			return err
		}
		document, err := queries.GetRetrievalDocument(ctx, storage.GetRetrievalDocumentParams{ProjectID: projectID, FilePath: file.Path})
		if err != nil || document.ID != documentID || document.Language != file.Language {
			return ErrBuildConflict
		}
		if err := queries.InsertDocumentVersion(ctx, storage.InsertDocumentVersionParams{ID: versionID, DocumentID: documentID, ContentSha256: file.ContentSHA256, SizeBytes: int64(file.SizeBytes), StructuralProvenance: provenance, CreatedAt: timestamp(build.PreparedAt)}); err != nil {
			return err
		}
		version, err := queries.GetDocumentVersion(ctx, storage.GetDocumentVersionParams{DocumentID: documentID, ContentSha256: file.ContentSHA256})
		if err != nil || version.ID != versionID || version.SizeBytes != int64(file.SizeBytes) || !equalJSON(version.StructuralProvenance, provenance) {
			return ErrBuildConflict
		}
		if err := queries.InsertBuildDocument(ctx, storage.InsertBuildDocumentParams{BuildID: buildID, DocumentVersionID: versionID, FilePath: file.Path, ContentSha256: file.ContentSHA256, Reused: file.Reused}); err != nil {
			return err
		}
		for _, chunk := range file.Chunks {
			chunkID := mustUUID(chunk.ID)
			chunkProvenance, _ := json.Marshal(chunk.StructuralProvenance)
			if err := queries.InsertRetrievalChunk(ctx, storage.InsertRetrievalChunkParams{
				ID: chunkID, DocumentVersionID: versionID, ChunkOrdinal: int32(chunk.Ordinal), ChunkKind: chunk.Kind,
				Language: chunk.Language, SymbolName: text(chunk.Symbol), Signature: chunk.Signature,
				StartLine: int32(chunk.StartLine), EndLine: int32(chunk.EndLine), Body: chunk.Body,
				BodySha256: chunk.BodySHA256, StructuralProvenance: chunkProvenance, CreatedAt: timestamp(build.PreparedAt),
			}); err != nil {
				return err
			}
			if err := queries.InsertBuildChunk(ctx, storage.InsertBuildChunkParams{BuildID: buildID, ChunkID: chunkID}); err != nil {
				return err
			}
			if build.EmbeddingSpace != nil {
				vector, ok := build.Vectors[chunk.ID]
				if !ok {
					return errors.New("code index: missing prepared vector")
				}
				inputSHA := SHA256([]byte(chunk.EmbeddingText()))
				if err := queries.InsertChunkEmbedding(ctx, storage.InsertChunkEmbeddingParams{ChunkID: chunkID, EmbeddingSpaceID: spaceID, Dimensions: int32(len(vector)), EmbeddingInputSha256: inputSHA, CreatedAt: timestamp(build.PreparedAt), Embedding: vectorText(vector)}); err != nil {
					return err
				}
				identity, err := queries.GetChunkEmbeddingIdentity(ctx, storage.GetChunkEmbeddingIdentityParams{ChunkID: chunkID, EmbeddingSpaceID: spaceID})
				if err != nil || identity.Dimensions != int32(len(vector)) || identity.EmbeddingInputSha256 != inputSHA {
					return ErrBuildConflict
				}
			}
		}
		for _, symbol := range file.Symbols {
			symbolID := mustUUID(symbol.ID)
			if err := queries.InsertRetrievalSymbol(ctx, storage.InsertRetrievalSymbolParams{ID: symbolID, DocumentVersionID: versionID, ChunkID: mustUUID(symbol.ChunkID), SymbolName: symbol.Name, SymbolKind: symbol.Kind, Signature: symbol.Signature, StartLine: int32(symbol.StartLine), EndLine: int32(symbol.EndLine), CreatedAt: timestamp(build.PreparedAt)}); err != nil {
				return err
			}
			if err := queries.InsertBuildSymbol(ctx, storage.InsertBuildSymbolParams{BuildID: buildID, SymbolID: symbolID}); err != nil {
				return err
			}
		}
		for _, edge := range file.Edges {
			provenance, _ := json.Marshal(edge.Provenance)
			if err := queries.InsertSymbolEdge(ctx, storage.InsertSymbolEdgeParams{ID: mustUUID(edge.ID), DocumentVersionID: versionID, FromSymbolID: nullableUUID(edge.FromSymbolID), EdgeKind: edge.Kind, TargetSymbol: edge.TargetSymbol, TargetPath: text(edge.TargetPath), SourceLine: int32(edge.SourceLine), Provenance: provenance, CreatedAt: timestamp(build.PreparedAt)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *PostgresStore) Activate(ctx context.Context, operationID string, completedAt time.Time) error {
	if strings.TrimSpace(operationID) == "" || completedAt.IsZero() {
		return errors.New("activate code index: operation and completion time are required")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		build, err := queries.GetIndexBuildByOperationID(ctx, operationID)
		if err != nil {
			return err
		}
		state, err := queries.GetIndexStateForUpdate(ctx, build.ProjectID)
		if err != nil {
			return err
		}
		if build.Status == "clean" && state.ActiveBuildID == build.ID {
			return nil
		}
		if build.Status != "building" || !state.LastBuildID.Valid || state.LastBuildID != build.ID {
			return ErrBuildConflict
		}
		counts, err := queries.CountIndexBuildRows(ctx, build.ID)
		if err != nil {
			return err
		}
		if counts.FileCount != int64(build.FileCount) || counts.ChunkCount != int64(build.ChunkCount) || counts.SymbolCount != int64(build.SymbolCount) || counts.VectorCount != int64(build.VectorCount) || (build.EmbeddingSpaceID.Valid && counts.VectorCount != counts.ChunkCount) || (!build.EmbeddingSpaceID.Valid && counts.VectorCount != 0) {
			return errors.New("activate code index: staged row validation failed")
		}
		completedAt = completedAt.UTC().Truncate(time.Microsecond)
		if err := queries.CompleteIndexBuild(ctx, storage.CompleteIndexBuildParams{ID: build.ID, CompletedAt: timestamp(completedAt)}); err != nil {
			return err
		}
		if err := queries.ActivateIndexBuild(ctx, storage.ActivateIndexBuildParams{ProjectID: build.ProjectID, UpdatedAt: timestamp(completedAt), ID: build.ID}); err != nil {
			return err
		}
		active, err := queries.GetIndexState(ctx, build.ProjectID)
		if err != nil || active.Status != string(StateClean) || active.ActiveBuildID != build.ID || active.ActiveSourceRevision.String != build.SourceRevision || active.ActiveEmbeddingSpaceID != build.EmbeddingSpaceID {
			return errors.New("activate code index: atomic activation did not retain exact build authority")
		}
		return nil
	})
}

func (s *PostgresStore) Fail(ctx context.Context, operationID, code string, failedAt time.Time) error {
	if strings.TrimSpace(operationID) == "" || strings.TrimSpace(code) == "" || failedAt.IsZero() {
		return errors.New("fail code index: exact operation, code, and time are required")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		queries := storage.New(tx)
		build, err := queries.GetIndexBuildByOperationID(ctx, operationID)
		if err != nil {
			return err
		}
		state, err := queries.GetIndexStateForUpdate(ctx, build.ProjectID)
		if err != nil {
			return err
		}
		if build.Status == "failed" {
			if build.ErrorCode.String == code {
				return nil
			}
			return ErrBuildConflict
		}
		if build.Status != "building" || !state.LastBuildID.Valid || state.LastBuildID != build.ID {
			return ErrBuildConflict
		}
		failedAt = failedAt.UTC().Truncate(time.Microsecond)
		if err := queries.FailIndexBuild(ctx, storage.FailIndexBuildParams{ID: build.ID, ErrorCode: text(code), CompletedAt: timestamp(failedAt)}); err != nil {
			return err
		}
		return queries.SetIndexFailed(ctx, storage.SetIndexFailedParams{ProjectID: build.ProjectID, LastBuildID: build.ID, Detail: code, UpdatedAt: timestamp(failedAt)})
	})
}

func validatePreparedBuild(build PreparedBuild) error {
	if build.ID != DeterministicID("index-build", build.Snapshot.ProjectID, build.OperationID) || build.Manifest.SchemaVersion != ManifestSchemaVersion || build.Manifest.ProjectID != build.Snapshot.ProjectID || build.Manifest.SourceRevision != build.Snapshot.SourceRevision || build.Manifest.SourceTree != build.Snapshot.SourceTree || build.Manifest.FileCount != len(build.Files) {
		return errors.New("code index: prepared build identity or manifest is divergent")
	}
	copyBuild := build
	if err := copyBuild.finalizeManifest(); err != nil || copyBuild.Manifest.SHA256 != build.Manifest.SHA256 {
		return errors.New("code index: prepared manifest hash is divergent")
	}
	return nil
}

func parseVectorText(value string) ([]float32, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, errors.New("invalid vector text")
	}
	if value == "[]" {
		return nil, nil
	}
	parts := strings.Split(value[1:len(value)-1], ",")
	result := make([]float32, len(parts))
	for i, part := range parts {
		parsed, err := strconv.ParseFloat(part, 32)
		if err != nil {
			return nil, err
		}
		result[i] = float32(parsed)
	}
	return result, nil
}

func pgUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
func mustUUID(value string) pgtype.UUID {
	parsed, _ := pgUUID(value)
	return parsed
}
func nullableUUID(value string) pgtype.UUID {
	if value == "" {
		return pgtype.UUID{}
	}
	return mustUUID(value)
}
func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
func text(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func embeddingSpaceSchema() string  { return "revolvr-embedding-space-v1" }
func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}
