-- name: InsertEmbeddingSpace :exec
INSERT INTO retrieval.embedding_spaces (
    id, space_sha256, schema_version, model_name, model_revision, dimensions,
    pooling, normalization, quantization, artifact_sha256, license, source_uri,
    serving_image_digest, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
) ON CONFLICT (space_sha256) DO NOTHING;

-- name: GetEmbeddingSpaceBySHA256 :one
SELECT id, space_sha256, schema_version, model_name, model_revision, dimensions,
    pooling, normalization, quantization, artifact_sha256, license, source_uri,
    serving_image_digest, created_at
FROM retrieval.embedding_spaces
WHERE space_sha256 = $1;

-- name: GetEmbeddingSpaceByID :one
SELECT id, space_sha256, schema_version, model_name, model_revision, dimensions,
    pooling, normalization, quantization, artifact_sha256, license, source_uri,
    serving_image_digest, created_at
FROM retrieval.embedding_spaces
WHERE id = $1;

-- name: InitializeIndexState :exec
INSERT INTO retrieval.index_states (
    project_id, status, detail, updated_at
) VALUES ($1, 'never_indexed', '', $2)
ON CONFLICT (project_id) DO NOTHING;

-- name: GetIndexState :one
SELECT project_id, status, active_build_id, active_source_revision,
    active_embedding_space_id, last_build_id, detail, updated_at
FROM retrieval.index_states
WHERE project_id = $1;

-- name: GetIndexStateForUpdate :one
SELECT project_id, status, active_build_id, active_source_revision,
    active_embedding_space_id, last_build_id, detail, updated_at
FROM retrieval.index_states
WHERE project_id = $1
FOR UPDATE;

-- name: MarkIndexDirty :exec
UPDATE retrieval.index_states
SET status = 'dirty', detail = $2, updated_at = $3
WHERE project_id = $1;

-- name: InsertIndexBuild :exec
INSERT INTO retrieval.index_builds (
    id, operation_id, project_id, source_revision, source_tree,
    embedding_space_id, previous_active_build_id, build_kind, status,
    manifest_sha256, file_count, chunk_count, symbol_count, vector_count,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, 'building', $9, $10, $11, $12, $13, $14
);

-- name: GetIndexBuildByOperationID :one
SELECT id, operation_id, project_id, source_revision, source_tree,
    embedding_space_id, previous_active_build_id, build_kind, status,
    manifest_sha256, file_count, chunk_count, symbol_count, vector_count,
    error_code, created_at, completed_at
FROM retrieval.index_builds
WHERE operation_id = $1;

-- name: SetIndexBuilding :exec
UPDATE retrieval.index_states
SET status = 'building', last_build_id = $2, detail = '', updated_at = $3
WHERE project_id = $1;

-- name: FailIndexBuild :exec
UPDATE retrieval.index_builds
SET status = 'failed', error_code = $2, completed_at = $3
WHERE id = $1 AND status = 'building';

-- name: SetIndexFailed :exec
UPDATE retrieval.index_states
SET status = CASE WHEN active_build_id IS NULL THEN 'failed' ELSE 'clean' END,
    last_build_id = $2, detail = $3, updated_at = $4
WHERE project_id = $1;

-- name: InsertRetrievalDocument :exec
INSERT INTO retrieval.documents (id, project_id, file_path, language, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (project_id, file_path) DO NOTHING;

-- name: GetRetrievalDocument :one
SELECT id, project_id, file_path, language, created_at
FROM retrieval.documents
WHERE project_id = $1 AND file_path = $2;

-- name: InsertDocumentVersion :exec
INSERT INTO retrieval.document_versions (
    id, document_id, content_sha256, size_bytes, structural_provenance, created_at
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (document_id, content_sha256) DO NOTHING;

-- name: GetDocumentVersion :one
SELECT id, document_id, content_sha256, size_bytes, structural_provenance, created_at
FROM retrieval.document_versions
WHERE document_id = $1 AND content_sha256 = $2;

-- name: InsertRetrievalChunk :exec
INSERT INTO retrieval.chunks (
    id, document_version_id, chunk_ordinal, chunk_kind, language, symbol_name,
    signature, start_line, end_line, body, body_sha256,
    structural_provenance, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (id) DO NOTHING;

-- name: InsertRetrievalSymbol :exec
INSERT INTO retrieval.symbols (
    id, document_version_id, chunk_id, symbol_name, symbol_kind, signature,
    start_line, end_line, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO NOTHING;

-- name: InsertSymbolEdge :exec
INSERT INTO retrieval.symbol_edges (
    id, document_version_id, from_symbol_id, edge_kind, target_symbol,
    target_path, source_line, provenance, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO NOTHING;

-- name: InsertBuildDocument :exec
INSERT INTO retrieval.index_build_documents (
    build_id, document_version_id, file_path, content_sha256, reused
) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (build_id, document_version_id) DO NOTHING;

-- name: InsertBuildChunk :exec
INSERT INTO retrieval.index_build_chunks (build_id, chunk_id)
VALUES ($1, $2)
ON CONFLICT (build_id, chunk_id) DO NOTHING;

-- name: InsertBuildSymbol :exec
INSERT INTO retrieval.index_build_symbols (build_id, symbol_id)
VALUES ($1, $2)
ON CONFLICT (build_id, symbol_id) DO NOTHING;

-- name: InsertChunkEmbedding :exec
INSERT INTO retrieval.chunk_embeddings (
    chunk_id, embedding_space_id, dimensions, embedding,
    embedding_input_sha256, created_at
) VALUES (
    $1, $2, $3, sqlc.arg(embedding)::text::vector, $4, $5
) ON CONFLICT (chunk_id, embedding_space_id) DO NOTHING;

-- name: GetChunkEmbeddingIdentity :one
SELECT chunk_id, embedding_space_id, dimensions, embedding_input_sha256,
    embedding::text AS embedding_text, created_at
FROM retrieval.chunk_embeddings
WHERE chunk_id = $1 AND embedding_space_id = $2;

-- name: ListActiveDocuments :many
SELECT d.id AS document_id, dv.id AS document_version_id, d.file_path,
    d.language, dv.content_sha256, dv.size_bytes, dv.structural_provenance
FROM retrieval.index_states AS ist
JOIN retrieval.index_build_documents AS ibd ON ibd.build_id = ist.active_build_id
JOIN retrieval.document_versions AS dv ON dv.id = ibd.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = $1
ORDER BY d.file_path;

-- name: ListDocumentChunks :many
SELECT id, document_version_id, chunk_ordinal, chunk_kind, language,
    symbol_name, signature, start_line, end_line, body, body_sha256,
    structural_provenance, created_at
FROM retrieval.chunks
WHERE document_version_id = $1
ORDER BY chunk_ordinal;

-- name: ListDocumentSymbols :many
SELECT id, document_version_id, chunk_id, symbol_name, symbol_kind, signature,
    start_line, end_line, created_at
FROM retrieval.symbols
WHERE document_version_id = $1
ORDER BY start_line, symbol_name;

-- name: ListDocumentEdges :many
SELECT id, document_version_id, from_symbol_id, edge_kind, target_symbol,
    target_path, source_line, provenance, created_at
FROM retrieval.symbol_edges
WHERE document_version_id = $1
ORDER BY source_line, edge_kind, target_symbol, id;

-- name: ListActiveChunkEmbeddingIdentities :many
SELECT ce.chunk_id, ce.embedding_space_id, ce.dimensions,
    ce.embedding_input_sha256, ce.embedding::text AS embedding_text
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_chunks AS ibc ON ibc.build_id = ib.id
JOIN retrieval.chunk_embeddings AS ce ON ce.chunk_id = ibc.chunk_id
    AND ce.embedding_space_id = ib.embedding_space_id
WHERE ist.project_id = $1
ORDER BY ce.chunk_id;

-- name: CountIndexBuildRows :one
SELECT
    (SELECT count(*) FROM retrieval.index_build_documents AS ibd WHERE ibd.build_id = sqlc.arg(target_build_id)) AS file_count,
    (SELECT count(*) FROM retrieval.index_build_chunks AS ibc_count WHERE ibc_count.build_id = sqlc.arg(target_build_id)) AS chunk_count,
    (SELECT count(*) FROM retrieval.index_build_symbols AS ibs WHERE ibs.build_id = sqlc.arg(target_build_id)) AS symbol_count,
    (SELECT count(*) FROM retrieval.index_build_chunks AS ibc
        JOIN retrieval.chunk_embeddings AS ce ON ce.chunk_id = ibc.chunk_id
        JOIN retrieval.index_builds AS ib ON ib.id = ibc.build_id
        WHERE ibc.build_id = sqlc.arg(target_build_id)
          AND ce.embedding_space_id = ib.embedding_space_id) AS vector_count;

-- name: CompleteIndexBuild :exec
UPDATE retrieval.index_builds
SET status = 'clean', completed_at = $2
WHERE id = $1 AND status = 'building';

-- name: ActivateIndexBuild :exec
UPDATE retrieval.index_states AS ist
SET status = 'clean', active_build_id = ib.id,
    active_source_revision = ib.source_revision,
    active_embedding_space_id = ib.embedding_space_id,
    last_build_id = ib.id, detail = '', updated_at = $2
FROM retrieval.index_builds AS ib
WHERE ist.project_id = $1 AND ib.id = $3 AND ib.project_id = ist.project_id
  AND ib.status = 'clean';

-- name: ExactFileChunks :many
SELECT c.id AS chunk_id, d.file_path, c.language, c.symbol_name, c.signature,
    c.chunk_kind, c.start_line, c.end_line, c.body, c.body_sha256,
    ib.source_revision
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_chunks AS ibc ON ibc.build_id = ib.id
JOIN retrieval.chunks AS c ON c.id = ibc.chunk_id
JOIN retrieval.document_versions AS dv ON dv.id = c.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = sqlc.arg(project_id)
  AND d.file_path = ANY(sqlc.arg(file_paths)::text[])
ORDER BY array_position(sqlc.arg(file_paths)::text[], d.file_path), c.start_line, c.id
LIMIT sqlc.arg(result_limit);

-- name: ExactSymbolChunks :many
SELECT c.id AS chunk_id, d.file_path, c.language, c.symbol_name, c.signature,
    c.chunk_kind, c.start_line, c.end_line, c.body, c.body_sha256,
    ib.source_revision
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_symbols AS ibs ON ibs.build_id = ib.id
JOIN retrieval.symbols AS s ON s.id = ibs.symbol_id
JOIN retrieval.chunks AS c ON c.id = s.chunk_id
JOIN retrieval.document_versions AS dv ON dv.id = c.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = sqlc.arg(project_id)
  AND lower(s.symbol_name) = ANY(sqlc.arg(symbol_names)::text[])
ORDER BY array_position(sqlc.arg(symbol_names)::text[], lower(s.symbol_name)), d.file_path, c.start_line, c.id
LIMIT sqlc.arg(result_limit);

-- name: ExactTextChunks :many
SELECT c.id AS chunk_id, d.file_path, c.language, c.symbol_name, c.signature,
    c.chunk_kind, c.start_line, c.end_line, c.body, c.body_sha256,
    ib.source_revision
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_chunks AS ibc ON ibc.build_id = ib.id
JOIN retrieval.chunks AS c ON c.id = ibc.chunk_id
JOIN retrieval.document_versions AS dv ON dv.id = c.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = sqlc.arg(project_id)
  AND position(lower(sqlc.arg(query)) in lower(c.body)) > 0
ORDER BY d.file_path, c.start_line, c.id
LIMIT sqlc.arg(result_limit);

-- name: StructuralChunks :many
SELECT DISTINCT c.id AS chunk_id, d.file_path, c.language, c.symbol_name,
    c.signature, c.chunk_kind, c.start_line, c.end_line, c.body,
    c.body_sha256, ib.source_revision
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_symbols AS ibs ON ibs.build_id = ib.id
JOIN retrieval.symbols AS s ON s.id = ibs.symbol_id
JOIN retrieval.chunks AS c ON c.id = s.chunk_id
JOIN retrieval.document_versions AS dv ON dv.id = c.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = sqlc.arg(project_id)
  AND (
    lower(s.symbol_name) = ANY(sqlc.arg(symbol_names)::text[])
    OR EXISTS (
      SELECT 1
      FROM retrieval.index_build_symbols AS seed_ibs
      JOIN retrieval.symbols AS seed ON seed.id = seed_ibs.symbol_id
      JOIN retrieval.symbol_edges AS se ON se.document_version_id IN (s.document_version_id, seed.document_version_id)
      WHERE seed_ibs.build_id = ib.id
        AND lower(seed.symbol_name) = ANY(sqlc.arg(symbol_names)::text[])
        AND (
          (se.from_symbol_id = seed.id AND lower(se.target_symbol) = lower(s.symbol_name))
          OR (se.from_symbol_id = s.id AND lower(se.target_symbol) = lower(seed.symbol_name))
        )
    )
  )
ORDER BY d.file_path, c.start_line, c.id
LIMIT sqlc.arg(result_limit);

-- name: FTSChunks :many
SELECT c.id AS chunk_id, d.file_path, c.language, c.symbol_name, c.signature,
    c.chunk_kind, c.start_line, c.end_line, c.body, c.body_sha256,
    ib.source_revision,
    ts_rank_cd(c.search_vector, to_tsquery('simple', sqlc.arg(query)), 32)::double precision AS lexical_score
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_chunks AS ibc ON ibc.build_id = ib.id
JOIN retrieval.chunks AS c ON c.id = ibc.chunk_id
JOIN retrieval.document_versions AS dv ON dv.id = c.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = sqlc.arg(project_id)
  AND c.search_vector @@ to_tsquery('simple', sqlc.arg(query))
ORDER BY lexical_score DESC, d.file_path, c.start_line, c.id
LIMIT sqlc.arg(result_limit);

-- name: VectorChunks1024 :many
SELECT c.id AS chunk_id, d.file_path, c.language, c.symbol_name, c.signature,
    c.chunk_kind, c.start_line, c.end_line, c.body, c.body_sha256,
    ib.source_revision,
    (1 - (ce.embedding::halfvec(1024) <=> sqlc.arg(query_vector)::text::halfvec(1024)))::double precision AS vector_score
FROM retrieval.index_states AS ist
JOIN retrieval.index_builds AS ib ON ib.id = ist.active_build_id
JOIN retrieval.index_build_chunks AS ibc ON ibc.build_id = ib.id
JOIN retrieval.chunks AS c ON c.id = ibc.chunk_id
JOIN retrieval.chunk_embeddings AS ce ON ce.chunk_id = c.id AND ce.embedding_space_id = ib.embedding_space_id
JOIN retrieval.document_versions AS dv ON dv.id = c.document_version_id
JOIN retrieval.documents AS d ON d.id = dv.document_id
WHERE ist.project_id = sqlc.arg(project_id) AND ce.dimensions = 1024
ORDER BY ce.embedding::halfvec(1024) <=> sqlc.arg(query_vector)::text::halfvec(1024), d.file_path, c.start_line, c.id
LIMIT sqlc.arg(result_limit);

-- name: InsertRetrievalRelation :exec
INSERT INTO retrieval.relations (
    id, project_id, build_id, subject_kind, subject_identity, predicate,
    object_kind, object_identity, authority_class, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (id) DO NOTHING;

-- name: InsertRetrievalRelationSource :exec
INSERT INTO retrieval.relation_sources (
    relation_id, source_kind, source_identity, source_sha256, artifact_id, provenance
) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (relation_id, source_kind, source_identity) DO NOTHING;

-- name: ListActiveRetrievalRelations :many
SELECT r.id, r.project_id, r.build_id, r.subject_kind, r.subject_identity,
    r.predicate, r.object_kind, r.object_identity, r.authority_class,
    r.created_at
FROM retrieval.index_states AS ist
JOIN retrieval.relations AS r ON r.build_id = ist.active_build_id
WHERE ist.project_id = $1
ORDER BY r.subject_kind, r.subject_identity, r.predicate,
    r.object_kind, r.object_identity, r.id
LIMIT $2;

-- name: ListRetrievalRelationSources :many
SELECT relation_id, source_kind, source_identity, source_sha256, artifact_id,
    provenance
FROM retrieval.relation_sources
WHERE relation_id = $1
ORDER BY source_kind, source_identity;

-- name: InsertContextPackage :exec
INSERT INTO telemetry.context_packages (
    id, project_id, task_id, run_id, schema_version, role, source_revision,
    embedding_space_id, byte_budget, token_budget, final_bytes, final_tokens,
    token_estimator, retrieval_configuration, manifest, dossier, dossier_sha256, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
);

-- name: InsertContextItem :exec
INSERT INTO telemetry.context_items (
    context_package_id, ordinal, candidate_identity, authority_class,
    source_kind, source_identity, source_sha256, source_path, symbol_name,
    start_line, end_line, ranking_signals, included, storage_form,
    inline_content, artifact_id, artifact_sha256, range_start, range_end,
    trajectory_id, trajectory_start, trajectory_end, media_type,
    retrieval_instructions, omission_reason
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
);

-- name: GetContextPackage :one
SELECT id, project_id, task_id, run_id, schema_version, role, source_revision,
    embedding_space_id, byte_budget, token_budget, final_bytes, final_tokens,
    token_estimator, retrieval_configuration, manifest, dossier, dossier_sha256, created_at
FROM telemetry.context_packages
WHERE id = $1;

-- name: ListContextItems :many
SELECT context_package_id, ordinal, candidate_identity, authority_class,
    source_kind, source_identity, source_sha256, source_path, symbol_name,
    start_line, end_line, ranking_signals, included, storage_form,
    inline_content, artifact_id, artifact_sha256, range_start, range_end,
    trajectory_id, trajectory_start, trajectory_end, media_type,
    retrieval_instructions, omission_reason
FROM telemetry.context_items
WHERE context_package_id = $1 AND included
ORDER BY ordinal
LIMIT $2;

-- name: GetContextItemByCandidate :one
SELECT context_package_id, ordinal, candidate_identity, authority_class,
    source_kind, source_identity, source_sha256, source_path, symbol_name,
    start_line, end_line, ranking_signals, included, storage_form,
    inline_content, artifact_id, artifact_sha256, range_start, range_end,
    trajectory_id, trajectory_start, trajectory_end, media_type,
    retrieval_instructions, omission_reason
FROM telemetry.context_items
WHERE context_package_id = $1 AND candidate_identity = $2;
