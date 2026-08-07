-- +goose Up
CREATE TABLE retrieval.embedding_spaces (
    id uuid PRIMARY KEY,
    space_sha256 text NOT NULL UNIQUE CHECK (space_sha256 ~ '^[0-9a-f]{64}$'),
    schema_version text NOT NULL CHECK (schema_version = 'revolvr-embedding-space-v1'),
    model_name text NOT NULL CHECK (model_name = 'Qwen/Qwen3-Embedding-0.6B-GGUF'),
    model_revision text NOT NULL CHECK (model_revision <> ''),
    dimensions integer NOT NULL CHECK (dimensions = 1024),
    pooling text NOT NULL CHECK (pooling = 'last'),
    normalization text NOT NULL CHECK (normalization = 'l2'),
    quantization text NOT NULL CHECK (quantization = 'Q8_0'),
    artifact_sha256 text NOT NULL CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
    license text NOT NULL CHECK (license <> ''),
    source_uri text NOT NULL CHECK (source_uri <> ''),
    serving_image_digest text NOT NULL CHECK (serving_image_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL
);

CREATE TABLE retrieval.index_builds (
    id uuid PRIMARY KEY,
    operation_id text NOT NULL UNIQUE CHECK (operation_id <> ''),
    project_id uuid NOT NULL REFERENCES core.projects (id),
    source_revision text NOT NULL CHECK (source_revision ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    embedding_space_id uuid REFERENCES retrieval.embedding_spaces (id),
    previous_active_build_id uuid REFERENCES retrieval.index_builds (id),
    build_kind text NOT NULL CHECK (build_kind IN ('full', 'incremental', 'rebuild', 'space_switch')),
    status text NOT NULL CHECK (status IN ('building', 'clean', 'failed')),
    manifest_sha256 text NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    file_count integer NOT NULL CHECK (file_count >= 0),
    chunk_count integer NOT NULL CHECK (chunk_count >= 0),
    symbol_count integer NOT NULL CHECK (symbol_count >= 0),
    vector_count integer NOT NULL CHECK (vector_count >= 0),
    error_code text,
    created_at timestamptz NOT NULL,
    completed_at timestamptz,
    CHECK ((status = 'building') = (completed_at IS NULL)),
    CHECK ((status = 'failed') = (error_code IS NOT NULL)),
    CHECK (vector_count = 0 OR embedding_space_id IS NOT NULL)
);

CREATE TABLE retrieval.index_states (
    project_id uuid PRIMARY KEY REFERENCES core.projects (id),
    status text NOT NULL CHECK (status IN ('never_indexed', 'clean', 'dirty', 'building', 'failed')),
    active_build_id uuid REFERENCES retrieval.index_builds (id),
    active_source_revision text CHECK (active_source_revision ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    active_embedding_space_id uuid REFERENCES retrieval.embedding_spaces (id),
    last_build_id uuid REFERENCES retrieval.index_builds (id),
    detail text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL,
    CHECK (
        (active_build_id IS NULL AND active_source_revision IS NULL AND active_embedding_space_id IS NULL)
        OR (active_build_id IS NOT NULL AND active_source_revision IS NOT NULL)
    )
);

CREATE TABLE retrieval.documents (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    file_path text NOT NULL CHECK (file_path <> '' AND file_path !~ '(^|/)\.\.(/|$)' AND file_path !~ '^/'),
    language text NOT NULL CHECK (language <> ''),
    created_at timestamptz NOT NULL,
    UNIQUE (project_id, file_path)
);

CREATE TABLE retrieval.document_versions (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES retrieval.documents (id),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    structural_provenance jsonb NOT NULL CHECK (jsonb_typeof(structural_provenance) = 'object'),
    created_at timestamptz NOT NULL,
    UNIQUE (document_id, content_sha256)
);

CREATE TABLE retrieval.chunks (
    id uuid PRIMARY KEY,
    document_version_id uuid NOT NULL REFERENCES retrieval.document_versions (id),
    chunk_ordinal integer NOT NULL CHECK (chunk_ordinal > 0),
    chunk_kind text NOT NULL CHECK (chunk_kind <> ''),
    language text NOT NULL CHECK (language <> ''),
    symbol_name text,
    signature text NOT NULL,
    start_line integer NOT NULL CHECK (start_line > 0),
    end_line integer NOT NULL CHECK (end_line >= start_line),
    body text NOT NULL CHECK (body <> '' AND octet_length(body) <= 131072),
    body_sha256 text NOT NULL CHECK (body_sha256 ~ '^[0-9a-f]{64}$'),
    structural_provenance jsonb NOT NULL CHECK (jsonb_typeof(structural_provenance) = 'object'),
    search_vector tsvector GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(symbol_name, '') || ' ' || signature || ' ' || body)
    ) STORED,
    created_at timestamptz NOT NULL,
    UNIQUE (document_version_id, chunk_ordinal),
    UNIQUE (document_version_id, body_sha256, start_line, end_line)
);

CREATE INDEX retrieval_chunks_fts_idx ON retrieval.chunks USING gin (search_vector);
CREATE INDEX retrieval_chunks_symbol_idx ON retrieval.chunks (lower(symbol_name)) WHERE symbol_name IS NOT NULL;

CREATE TABLE retrieval.symbols (
    id uuid PRIMARY KEY,
    document_version_id uuid NOT NULL REFERENCES retrieval.document_versions (id),
    chunk_id uuid NOT NULL REFERENCES retrieval.chunks (id),
    symbol_name text NOT NULL CHECK (symbol_name <> ''),
    symbol_kind text NOT NULL CHECK (symbol_kind <> ''),
    signature text NOT NULL,
    start_line integer NOT NULL CHECK (start_line > 0),
    end_line integer NOT NULL CHECK (end_line >= start_line),
    created_at timestamptz NOT NULL,
    UNIQUE (document_version_id, symbol_name, start_line)
);

CREATE INDEX retrieval_symbols_name_idx ON retrieval.symbols (lower(symbol_name));

CREATE TABLE retrieval.symbol_edges (
    id uuid PRIMARY KEY,
    document_version_id uuid NOT NULL REFERENCES retrieval.document_versions (id),
    from_symbol_id uuid REFERENCES retrieval.symbols (id),
    edge_kind text NOT NULL CHECK (edge_kind IN ('imports', 'calls', 'references', 'implements', 'contains')),
    target_symbol text NOT NULL CHECK (target_symbol <> ''),
    target_path text,
    source_line integer NOT NULL CHECK (source_line > 0),
    provenance jsonb NOT NULL CHECK (jsonb_typeof(provenance) = 'object'),
    created_at timestamptz NOT NULL,
    UNIQUE (document_version_id, from_symbol_id, edge_kind, target_symbol, source_line)
);

CREATE INDEX retrieval_symbol_edges_target_idx ON retrieval.symbol_edges (lower(target_symbol), edge_kind);

CREATE TABLE retrieval.chunk_embeddings (
    chunk_id uuid NOT NULL REFERENCES retrieval.chunks (id),
    embedding_space_id uuid NOT NULL REFERENCES retrieval.embedding_spaces (id),
    dimensions integer NOT NULL CHECK (dimensions = 1024),
    embedding vector NOT NULL,
    embedding_input_sha256 text NOT NULL CHECK (embedding_input_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (chunk_id, embedding_space_id)
);

-- +goose StatementBegin
CREATE FUNCTION retrieval.validate_chunk_embedding() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE expected_dimensions integer;
BEGIN
    SELECT dimensions INTO STRICT expected_dimensions
    FROM retrieval.embedding_spaces
    WHERE id = NEW.embedding_space_id;
    IF NEW.dimensions <> expected_dimensions OR vector_dims(NEW.embedding) <> expected_dimensions THEN
        RAISE EXCEPTION 'chunk embedding dimensions do not match embedding space';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER retrieval_chunk_embedding_validate
BEFORE INSERT OR UPDATE ON retrieval.chunk_embeddings
FOR EACH ROW EXECUTE FUNCTION retrieval.validate_chunk_embedding();

CREATE INDEX retrieval_chunk_embeddings_1024_hnsw_idx
ON retrieval.chunk_embeddings USING hnsw ((embedding::halfvec(1024)) halfvec_cosine_ops)
WHERE dimensions = 1024;

CREATE TABLE retrieval.index_build_documents (
    build_id uuid NOT NULL REFERENCES retrieval.index_builds (id) ON DELETE CASCADE,
    document_version_id uuid NOT NULL REFERENCES retrieval.document_versions (id),
    file_path text NOT NULL,
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    reused boolean NOT NULL,
    PRIMARY KEY (build_id, document_version_id),
    UNIQUE (build_id, file_path)
);

CREATE TABLE retrieval.index_build_chunks (
    build_id uuid NOT NULL REFERENCES retrieval.index_builds (id) ON DELETE CASCADE,
    chunk_id uuid NOT NULL REFERENCES retrieval.chunks (id),
    PRIMARY KEY (build_id, chunk_id)
);

CREATE TABLE retrieval.index_build_symbols (
    build_id uuid NOT NULL REFERENCES retrieval.index_builds (id) ON DELETE CASCADE,
    symbol_id uuid NOT NULL REFERENCES retrieval.symbols (id),
    PRIMARY KEY (build_id, symbol_id)
);

CREATE TABLE retrieval.relations (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    build_id uuid NOT NULL REFERENCES retrieval.index_builds (id) ON DELETE CASCADE,
    subject_kind text NOT NULL CHECK (subject_kind <> ''),
    subject_identity text NOT NULL CHECK (subject_identity <> ''),
    predicate text NOT NULL CHECK (predicate <> ''),
    object_kind text NOT NULL CHECK (object_kind <> ''),
    object_identity text NOT NULL CHECK (object_identity <> ''),
    authority_class text NOT NULL CHECK (authority_class <> ''),
    created_at timestamptz NOT NULL,
    UNIQUE (build_id, subject_kind, subject_identity, predicate, object_kind, object_identity)
);

CREATE TABLE retrieval.relation_sources (
    relation_id uuid NOT NULL REFERENCES retrieval.relations (id) ON DELETE CASCADE,
    source_kind text NOT NULL CHECK (source_kind <> ''),
    source_identity text NOT NULL CHECK (source_identity <> ''),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    artifact_id uuid REFERENCES core.artifacts (id),
    provenance jsonb NOT NULL CHECK (jsonb_typeof(provenance) = 'object'),
    PRIMARY KEY (relation_id, source_kind, source_identity)
);

CREATE TABLE telemetry.context_packages (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    task_id uuid REFERENCES core.tasks (id),
    run_id uuid REFERENCES core.runs (id),
    schema_version text NOT NULL CHECK (schema_version = 'revolvr-context-package-v1'),
    role text NOT NULL CHECK (role IN ('supervisor', 'planner', 'implementer', 'auditor', 'corrector', 'documentor', 'simplifier')),
    source_revision text NOT NULL CHECK (source_revision ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    embedding_space_id uuid REFERENCES retrieval.embedding_spaces (id),
    byte_budget integer NOT NULL CHECK (byte_budget > 0),
    token_budget integer NOT NULL CHECK (token_budget > 0),
    final_bytes integer NOT NULL CHECK (final_bytes >= 0 AND final_bytes <= byte_budget),
    final_tokens integer NOT NULL CHECK (final_tokens >= 0 AND final_tokens <= token_budget),
    token_estimator text NOT NULL CHECK (token_estimator = 'utf8-bytes-ceil-div-4-v1'),
    retrieval_configuration jsonb NOT NULL CHECK (jsonb_typeof(retrieval_configuration) = 'object'),
    manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
    dossier bytea NOT NULL CHECK (octet_length(dossier) <= 4194304),
    dossier_sha256 text NOT NULL CHECK (dossier_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL
);

CREATE INDEX telemetry_context_packages_dossier_sha256_idx
ON telemetry.context_packages (dossier_sha256);

CREATE TABLE telemetry.context_items (
    context_package_id uuid NOT NULL REFERENCES telemetry.context_packages (id),
    ordinal integer NOT NULL CHECK (ordinal > 0),
    candidate_identity text NOT NULL CHECK (candidate_identity <> ''),
    authority_class text NOT NULL CHECK (authority_class <> ''),
    source_kind text NOT NULL CHECK (source_kind <> ''),
    source_identity text NOT NULL CHECK (source_identity <> ''),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    source_path text,
    symbol_name text,
    start_line integer,
    end_line integer,
    ranking_signals jsonb NOT NULL CHECK (jsonb_typeof(ranking_signals) = 'object'),
    included boolean NOT NULL,
    storage_form text NOT NULL CHECK (storage_form IN ('inline', 'artifact_range', 'trajectory_range', 'omitted')),
    inline_content text,
    artifact_id uuid REFERENCES core.artifacts (id),
    artifact_sha256 text CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
    range_start bigint,
    range_end bigint,
    trajectory_id text,
    trajectory_start bigint,
    trajectory_end bigint,
    media_type text,
    retrieval_instructions jsonb NOT NULL CHECK (jsonb_typeof(retrieval_instructions) = 'object'),
    omission_reason text,
    PRIMARY KEY (context_package_id, ordinal),
    UNIQUE (context_package_id, candidate_identity),
    CHECK ((start_line IS NULL AND end_line IS NULL) OR (start_line > 0 AND end_line >= start_line)),
    CHECK (
        (storage_form = 'inline' AND included AND inline_content IS NOT NULL AND artifact_id IS NULL AND trajectory_id IS NULL)
        OR (storage_form = 'artifact_range' AND included AND inline_content IS NULL AND artifact_id IS NOT NULL AND artifact_sha256 IS NOT NULL AND range_start >= 0 AND range_end > range_start AND trajectory_id IS NULL)
        OR (storage_form = 'trajectory_range' AND included AND inline_content IS NULL AND artifact_id IS NULL AND trajectory_id IS NOT NULL AND trajectory_start >= 0 AND trajectory_end > trajectory_start)
        OR (storage_form = 'omitted' AND NOT included AND inline_content IS NULL AND artifact_id IS NULL AND trajectory_id IS NULL AND omission_reason IS NOT NULL)
    )
);

-- +goose StatementBegin
CREATE FUNCTION retrieval.reject_immutable_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'immutable retrieval/context row cannot be changed';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER retrieval_embedding_spaces_immutable BEFORE UPDATE OR DELETE ON retrieval.embedding_spaces FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_documents_immutable BEFORE UPDATE OR DELETE ON retrieval.documents FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_document_versions_immutable BEFORE UPDATE OR DELETE ON retrieval.document_versions FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_chunks_immutable BEFORE UPDATE OR DELETE ON retrieval.chunks FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_symbols_immutable BEFORE UPDATE OR DELETE ON retrieval.symbols FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_symbol_edges_immutable BEFORE UPDATE OR DELETE ON retrieval.symbol_edges FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_chunk_embeddings_immutable BEFORE UPDATE OR DELETE ON retrieval.chunk_embeddings FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_relations_immutable BEFORE UPDATE OR DELETE ON retrieval.relations FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER retrieval_relation_sources_immutable BEFORE UPDATE OR DELETE ON retrieval.relation_sources FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER telemetry_context_packages_immutable BEFORE UPDATE OR DELETE ON telemetry.context_packages FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();
CREATE TRIGGER telemetry_context_items_immutable BEFORE UPDATE OR DELETE ON telemetry.context_items FOR EACH ROW EXECUTE FUNCTION retrieval.reject_immutable_change();

-- +goose Down
DROP TRIGGER IF EXISTS telemetry_context_items_immutable ON telemetry.context_items;
DROP TRIGGER IF EXISTS telemetry_context_packages_immutable ON telemetry.context_packages;
DROP TRIGGER IF EXISTS retrieval_relation_sources_immutable ON retrieval.relation_sources;
DROP TRIGGER IF EXISTS retrieval_relations_immutable ON retrieval.relations;
DROP TRIGGER IF EXISTS retrieval_chunk_embeddings_immutable ON retrieval.chunk_embeddings;
DROP TRIGGER IF EXISTS retrieval_symbol_edges_immutable ON retrieval.symbol_edges;
DROP TRIGGER IF EXISTS retrieval_symbols_immutable ON retrieval.symbols;
DROP TRIGGER IF EXISTS retrieval_chunks_immutable ON retrieval.chunks;
DROP TRIGGER IF EXISTS retrieval_document_versions_immutable ON retrieval.document_versions;
DROP TRIGGER IF EXISTS retrieval_documents_immutable ON retrieval.documents;
DROP TRIGGER IF EXISTS retrieval_embedding_spaces_immutable ON retrieval.embedding_spaces;
DROP FUNCTION retrieval.reject_immutable_change();
DROP TABLE telemetry.context_items;
DROP TABLE telemetry.context_packages;
DROP TABLE retrieval.relation_sources;
DROP TABLE retrieval.relations;
DROP TABLE retrieval.index_build_symbols;
DROP TABLE retrieval.index_build_chunks;
DROP TABLE retrieval.index_build_documents;
DROP INDEX retrieval.retrieval_chunk_embeddings_1024_hnsw_idx;
DROP TRIGGER retrieval_chunk_embedding_validate ON retrieval.chunk_embeddings;
DROP FUNCTION retrieval.validate_chunk_embedding();
DROP TABLE retrieval.chunk_embeddings;
DROP TABLE retrieval.symbol_edges;
DROP TABLE retrieval.symbols;
DROP TABLE retrieval.chunks;
DROP TABLE retrieval.document_versions;
DROP TABLE retrieval.documents;
DROP TABLE retrieval.index_states;
DROP TABLE retrieval.index_builds;
DROP TABLE retrieval.embedding_spaces;
