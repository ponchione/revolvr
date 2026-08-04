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

-- name: GetProjectByID :one
SELECT id, name, status, created_at, updated_at
FROM core.projects
WHERE id = $1;

-- name: InsertTaskImport :one
INSERT INTO core.task_imports (
    id, project_id, source_artifact_id, task_id, source_name, source_sha256,
    media_type, status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id, project_id, source_artifact_id, task_id, source_name, source_sha256,
    media_type, status, created_at, updated_at;

-- name: GetTaskImport :one
SELECT id, project_id, source_artifact_id, task_id, source_name, source_sha256,
    media_type, status, created_at, updated_at
FROM core.task_imports
WHERE id = $1;

-- name: GetTaskImportBySourceIdentity :one
SELECT
    ti.id, ti.project_id, ti.source_artifact_id, ti.task_id, ti.source_name,
    ti.source_sha256, ti.media_type, ti.status, ti.created_at, ti.updated_at,
    a.size_bytes AS artifact_size_bytes,
    a.storage_path AS artifact_storage_path
FROM core.task_imports AS ti
JOIN core.artifacts AS a ON a.id = ti.source_artifact_id
WHERE ti.project_id = $1 AND ti.source_name = $2;

-- name: InsertTask :one
INSERT INTO core.tasks (
    id, project_id, external_task_id, status, accepted_version_id, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, project_id, external_task_id, status, accepted_version_id, created_at, updated_at;

-- name: InsertTaskVersion :one
INSERT INTO core.task_versions (
    id, task_id, version_number, source_artifact_id, title, goal, risk_class,
    mutation_class, network_profile, priority, read_only_investigation, scope,
    excluded_scope, verification_plan, budget, secret_requirements,
    expected_paths, operator_checkpoints, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19
)
RETURNING id, task_id, version_number, source_artifact_id, title, goal, risk_class,
    mutation_class, network_profile, priority, read_only_investigation, scope,
    excluded_scope, verification_plan, budget, secret_requirements,
    expected_paths, operator_checkpoints, created_at;

-- name: InsertTaskDependency :one
INSERT INTO core.task_dependencies (
    task_version_id, task_id, project_id, dependency_task_id, dependency_type, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING task_version_id, task_id, project_id, dependency_task_id, dependency_type, created_at;

-- name: InsertTaskConflict :one
INSERT INTO core.task_conflicts (
    task_version_id, task_id, project_id, conflicting_task_id, created_at
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING task_version_id, task_id, project_id, conflicting_task_id, created_at;

-- name: InsertTaskAcceptanceCriterion :one
INSERT INTO core.task_acceptance_criteria (
    id, task_id, external_criterion_id, status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id, task_id, external_criterion_id, status, created_at, updated_at;

-- name: InsertTaskAcceptanceVersion :one
INSERT INTO core.task_acceptance_versions (
    id, criterion_id, task_id, task_version_id, version_number, requirement,
    verification_method, verification_reference, operator_checkpoint, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id, criterion_id, task_id, task_version_id, version_number, requirement,
    verification_method, verification_reference, operator_checkpoint, created_at;

-- name: GetTaskWithSelectedVersionByExternalID :one
SELECT
    t.id, t.project_id, t.external_task_id, t.status, t.accepted_version_id,
    t.created_at, t.updated_at,
    tv.id AS selected_version_id,
    tv.version_number AS selected_version_number,
    tv.source_artifact_id AS selected_source_artifact_id,
    tv.title AS selected_title,
    tv.goal AS selected_goal,
    tv.risk_class AS selected_risk_class,
    tv.mutation_class AS selected_mutation_class,
    tv.network_profile AS selected_network_profile,
    tv.priority AS selected_priority,
    tv.read_only_investigation AS selected_read_only_investigation,
    tv.scope AS selected_scope,
    tv.excluded_scope AS selected_excluded_scope,
    tv.verification_plan AS selected_verification_plan,
    tv.budget AS selected_budget,
    tv.secret_requirements AS selected_secret_requirements,
    tv.expected_paths AS selected_expected_paths,
    tv.operator_checkpoints AS selected_operator_checkpoints,
    tv.created_at AS selected_created_at
FROM core.tasks AS t
LEFT JOIN core.task_versions AS tv ON tv.id = t.accepted_version_id
WHERE t.project_id = $1 AND t.external_task_id = $2;
