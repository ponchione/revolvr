-- name: InsertArtifact :one
INSERT INTO core.artifacts (
    id, sha256, size_bytes, media_type, logical_kind, storage_path, compression, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, sha256, size_bytes, media_type, logical_kind, storage_path, compression, created_at;

-- name: GetArtifactBySHA256 :one
SELECT id, sha256, size_bytes, media_type, logical_kind, storage_path, compression, created_at
FROM core.artifacts
WHERE sha256 = $1;

-- name: AppendEvent :one
INSERT INTO core.events (
    id, project_id, task_id, run_id, event_type, aggregate_type, aggregate_id,
    aggregate_version, payload, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id, project_id, task_id, run_id, event_type, aggregate_type, aggregate_id,
    aggregate_version, payload, created_at;

-- name: GetEvent :one
SELECT id, project_id, task_id, run_id, event_type, aggregate_type, aggregate_id,
    aggregate_version, payload, created_at
FROM core.events
WHERE id = $1;

-- name: InsertProject :one
INSERT INTO core.projects (
    id, name, status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id, name, status, created_at, updated_at;

-- name: InsertProjectSource :one
INSERT INTO core.project_sources (
    id, project_id, canonical_source_path, managed_repository_path,
    current_commit, current_tree, current_branch, default_branch,
    dirty_state, remotes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id, project_id, canonical_source_path, managed_repository_path,
    current_commit, current_tree, current_branch, default_branch,
    dirty_state, remotes;

-- name: GetProjectRegistrationByCanonicalSourcePath :one
SELECT
    p.id AS project_id,
    p.name,
    p.status,
    p.created_at,
    p.updated_at,
    ps.id AS project_source_id,
    ps.canonical_source_path,
    ps.managed_repository_path,
    ps.current_commit,
    ps.current_tree,
    ps.current_branch,
    ps.default_branch,
    ps.dirty_state,
    ps.remotes
FROM core.projects AS p
JOIN core.project_sources AS ps ON ps.project_id = p.id
WHERE ps.canonical_source_path = $1;
