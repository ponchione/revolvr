package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	storage "revolvr/internal/storage/postgres"
)

type PostgresSource struct{ pool *pgxpool.Pool }

func NewPostgresSource(pool *pgxpool.Pool) (*PostgresSource, error) {
	if pool == nil {
		return nil, errors.New("retrieval PostgreSQL source requires a pool")
	}
	return &PostgresSource{pool: pool}, nil
}

func (s *PostgresSource) Status(ctx context.Context, projectID string) (IndexStatus, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return IndexStatus{}, err
	}
	queries := storage.New(s.pool)
	row, err := queries.GetIndexState(ctx, project)
	if errors.Is(err, pgx.ErrNoRows) {
		return IndexStatus{State: "never_indexed"}, nil
	}
	if err != nil {
		return IndexStatus{}, err
	}
	result := IndexStatus{State: row.Status, SourceRevision: row.ActiveSourceRevision.String}
	if row.ActiveEmbeddingSpaceID.Valid {
		space, err := queries.GetEmbeddingSpaceByID(ctx, row.ActiveEmbeddingSpaceID)
		if err != nil {
			return IndexStatus{}, err
		}
		result.SpaceSHA256, result.Dimensions = space.SpaceSha256, int(space.Dimensions)
	}
	return result, nil
}

func (s *PostgresSource) ExactFiles(ctx context.Context, projectID string, paths []string, limit int) ([]Candidate, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := storage.New(s.pool).ExactFileChunks(ctx, storage.ExactFileChunksParams{ProjectID: project, FilePaths: paths, ResultLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, chunkCandidate(uuidText(row.ChunkID), row.FilePath, row.Language, row.SymbolName.String, row.Signature, row.ChunkKind, int(row.StartLine), int(row.EndLine), row.Body, row.BodySha256, row.SourceRevision))
	}
	return result, nil
}

func (s *PostgresSource) ExactSymbols(ctx context.Context, projectID string, symbols []string, limit int) ([]Candidate, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := storage.New(s.pool).ExactSymbolChunks(ctx, storage.ExactSymbolChunksParams{ProjectID: project, SymbolNames: symbols, ResultLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, chunkCandidate(uuidText(row.ChunkID), row.FilePath, row.Language, row.SymbolName.String, row.Signature, row.ChunkKind, int(row.StartLine), int(row.EndLine), row.Body, row.BodySha256, row.SourceRevision))
	}
	return result, nil
}

func (s *PostgresSource) ExactText(ctx context.Context, projectID, query string, limit int) ([]Candidate, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := storage.New(s.pool).ExactTextChunks(ctx, storage.ExactTextChunksParams{ProjectID: project, Query: query, ResultLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, chunkCandidate(uuidText(row.ChunkID), row.FilePath, row.Language, row.SymbolName.String, row.Signature, row.ChunkKind, int(row.StartLine), int(row.EndLine), row.Body, row.BodySha256, row.SourceRevision))
	}
	return result, nil
}

func (s *PostgresSource) Structural(ctx context.Context, projectID string, symbols []string, limit int) ([]Candidate, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return nil, err
	}
	rows, err := storage.New(s.pool).StructuralChunks(ctx, storage.StructuralChunksParams{ProjectID: project, SymbolNames: symbols, ResultLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, chunkCandidate(uuidText(row.ChunkID), row.FilePath, row.Language, row.SymbolName.String, row.Signature, row.ChunkKind, int(row.StartLine), int(row.EndLine), row.Body, row.BodySha256, row.SourceRevision))
	}
	return result, nil
}

func (s *PostgresSource) FTS(ctx context.Context, projectID, query string, limit int) ([]Candidate, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return nil, err
	}
	compiled := compileFTSQuery(query)
	if compiled == "" {
		return nil, nil
	}
	rows, err := storage.New(s.pool).FTSChunks(ctx, storage.FTSChunksParams{ProjectID: project, Query: compiled, ResultLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		candidate := chunkCandidate(uuidText(row.ChunkID), row.FilePath, row.Language, row.SymbolName.String, row.Signature, row.ChunkKind, int(row.StartLine), int(row.EndLine), row.Body, row.BodySha256, row.SourceRevision)
		candidate.Signals.LexicalScore = row.LexicalScore
		result = append(result, candidate)
	}
	return result, nil
}

func (s *PostgresSource) Vector(ctx context.Context, projectID string, vector []float32, dimensions, limit int) ([]Candidate, error) {
	project, err := retrievalUUID(projectID)
	if err != nil {
		return nil, err
	}
	if len(vector) != dimensions {
		return nil, errors.New("retrieval query vector has wrong dimensions")
	}
	queryVector := retrievalVectorText(vector)
	queries := storage.New(s.pool)
	if dimensions != 1024 {
		return nil, fmt.Errorf("selected Qwen embedding space requires 1024 dimensions, got %d", dimensions)
	}
	rows, err := queries.VectorChunks1024(ctx, storage.VectorChunks1024Params{ProjectID: project, QueryVector: queryVector, ResultLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	result := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		candidate := chunkCandidate(uuidText(row.ChunkID), row.FilePath, row.Language, row.SymbolName.String, row.Signature, row.ChunkKind, int(row.StartLine), int(row.EndLine), row.Body, row.BodySha256, row.SourceRevision)
		candidate.Signals.VectorScore = row.VectorScore
		result = append(result, candidate)
	}
	return result, nil
}

func chunkCandidate(id, path, language, symbol, signature, kind string, startLine, endLine int, body, bodySHA, revision string) Candidate {
	return Candidate{
		Identity: id, ChunkID: id, SourceKind: "code_chunk", SourceIdentity: path + "#L" + strconv.Itoa(startLine) + "-L" + strconv.Itoa(endLine),
		SourceRevision: revision, SourceSHA256: bodySHA, Path: path, Symbol: symbol, Language: language,
		Kind: kind, Signature: signature, StartLine: startLine, EndLine: endLine, Content: body,
	}
}

func retrievalUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}
func uuidText(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}
func retrievalVectorText(value []float32) string {
	parts := make([]string, len(value))
	for i, number := range value {
		parts[i] = strconv.FormatFloat(float64(number), 'g', 9, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

var ftsStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "how": true, "in": true,
	"into": true, "is": true, "it": true, "of": true, "on": true, "or": true,
	"that": true, "the": true, "this": true, "to": true, "what": true, "where": true,
	"which": true, "who": true, "why": true, "with": true,
}

// compileFTSQuery turns a natural-language question into a bounded transparent
// OR query. Seven-rune prefixes bridge common prose variants such as persists
// and persistence while retaining PostgreSQL's simple tokenizer for code.
func compileFTSQuery(value string) string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})
	seen := map[string]bool{}
	terms := make([]string, 0, min(len(fields), 32))
	for _, field := range fields {
		if len(field) < 2 || ftsStopWords[field] || seen[field] {
			continue
		}
		seen[field] = true
		prefix := field
		if len(prefix) > 7 {
			prefix = prefix[:7]
		}
		terms = append(terms, "'"+strings.ReplaceAll(prefix, "'", "''")+"':*")
		if len(terms) == 32 {
			break
		}
	}
	return strings.Join(terms, " | ")
}
