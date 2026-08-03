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
