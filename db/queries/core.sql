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
RETURNING id, project_id, external_task_id, status, accepted_version_id, created_at, updated_at,
    aggregate_version;

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
    t.created_at, t.updated_at, t.aggregate_version,
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

-- name: GetTaskAndVersion :one
SELECT
    t.id, t.project_id, t.external_task_id, t.status, t.accepted_version_id,
    t.created_at, t.updated_at, t.aggregate_version,
    tv.id AS task_version_id,
    tv.version_number AS task_version_number,
    tv.source_artifact_id AS task_version_source_artifact_id,
    tv.title AS task_version_title,
    tv.goal AS task_version_goal,
    tv.risk_class AS task_version_risk_class,
    tv.mutation_class AS task_version_mutation_class,
    tv.network_profile AS task_version_network_profile,
    tv.priority AS task_version_priority,
    tv.read_only_investigation AS task_version_read_only_investigation,
    tv.scope AS task_version_scope,
    tv.excluded_scope AS task_version_excluded_scope,
    tv.verification_plan AS task_version_verification_plan,
    tv.budget AS task_version_budget,
    tv.secret_requirements AS task_version_secret_requirements,
    tv.expected_paths AS task_version_expected_paths,
    tv.operator_checkpoints AS task_version_operator_checkpoints,
    tv.created_at AS task_version_created_at
FROM core.tasks AS t
JOIN core.task_versions AS tv ON tv.task_id = t.id
WHERE t.project_id = sqlc.arg(project_id)
  AND t.id = sqlc.arg(task_id)
  AND tv.id = sqlc.arg(task_version_id);

-- name: CompareAndUpdateTaskState :one
UPDATE core.tasks
SET status = sqlc.arg(new_status),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE project_id = sqlc.arg(project_id)
  AND id = sqlc.arg(task_id)
  AND status = sqlc.arg(expected_status)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING id, project_id, external_task_id, status, accepted_version_id,
    created_at, updated_at, aggregate_version;

-- name: ApproveTaskVersion :one
UPDATE core.tasks AS t
SET accepted_version_id = sqlc.arg(task_version_id),
    status = 'pending',
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE t.project_id = sqlc.arg(project_id)
  AND t.id = sqlc.arg(task_id)
  AND t.status = 'awaiting_approval'
  AND t.aggregate_version = sqlc.arg(expected_aggregate_version)
  AND t.accepted_version_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM core.task_versions AS tv
      WHERE tv.id = sqlc.arg(task_version_id) AND tv.task_id = t.id
  )
RETURNING t.id, t.project_id, t.external_task_id, t.status, t.accepted_version_id,
    t.created_at, t.updated_at, t.aggregate_version;

-- name: GetApprovedTaskWithSelectedVersion :one
SELECT
    t.id, t.project_id, t.external_task_id, t.status, t.accepted_version_id,
    t.created_at, t.updated_at, t.aggregate_version,
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
JOIN core.task_versions AS tv ON tv.id = t.accepted_version_id AND tv.task_id = t.id
WHERE t.project_id = sqlc.arg(project_id) AND t.id = sqlc.arg(task_id);

-- name: ListSchedulerTasks :many
SELECT
    t.id, t.project_id, t.external_task_id, t.status, t.accepted_version_id,
    t.created_at, t.aggregate_version, p.status AS project_status,
    tv.id AS task_version_id, tv.priority AS task_priority,
    EXISTS (
        SELECT 1
        FROM core.task_acceptance_versions AS av
        JOIN core.task_acceptance_criteria AS ac
          ON ac.id = av.criterion_id AND ac.task_id = av.task_id
        WHERE av.task_id = t.id
          AND av.task_version_id = t.accepted_version_id
          AND av.verification_method = 'operator_checkpoint'
          AND ac.status = 'pending'
    ) AS awaiting_operator_checkpoint
FROM core.tasks AS t
JOIN core.projects AS p ON p.id = t.project_id
LEFT JOIN core.task_versions AS tv
  ON tv.id = t.accepted_version_id AND tv.task_id = t.id
ORDER BY t.created_at, t.id;

-- name: ListSchedulerProjectSources :many
SELECT id, project_id, current_commit, current_tree
FROM core.project_sources
ORDER BY project_id, id;

-- name: GetSchedulerProjectSource :one
SELECT id, project_id, current_commit, current_tree
FROM core.project_sources
WHERE id = sqlc.arg(project_source_id) AND project_id = sqlc.arg(project_id);

-- name: ListSchedulerDependencies :many
SELECT d.task_id, d.task_version_id, d.dependency_task_id
FROM core.task_dependencies AS d
JOIN core.tasks AS t
  ON t.id = d.task_id AND t.accepted_version_id = d.task_version_id
ORDER BY d.task_id, d.dependency_task_id;

-- name: ListSchedulerConflicts :many
SELECT c.task_id, c.task_version_id, c.conflicting_task_id
FROM core.task_conflicts AS c
JOIN core.tasks AS t
  ON t.id = c.task_id AND t.accepted_version_id = c.task_version_id
ORDER BY c.task_id, c.conflicting_task_id;

-- name: GetGlobalExecutionLease :one
SELECT lease_name, run_id, coordinator_identity, acquired_at, aggregate_version
FROM core.execution_leases
WHERE lease_name = 'global-source-mutation-v1';

-- name: GetGlobalExecutionLeaseForUpdate :one
SELECT lease_name, run_id, coordinator_identity, acquired_at, aggregate_version
FROM core.execution_leases
WHERE lease_name = 'global-source-mutation-v1'
FOR UPDATE;

-- name: InsertRun :one
INSERT INTO core.runs (
    id, project_id, task_id, task_version_id, project_source_id, status,
    admitted_task_aggregate_version, source_commit, source_tree,
    coordinator_identity, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, 'active', $6, $7, $8, $9, $10, $10
)
RETURNING id, project_id, task_id, task_version_id, project_source_id, status,
    aggregate_version, admitted_task_aggregate_version, source_commit,
    source_tree, coordinator_identity, created_at, updated_at, released_at;

-- name: GetRun :one
SELECT id, project_id, task_id, task_version_id, project_source_id, status,
    aggregate_version, admitted_task_aggregate_version, source_commit,
    source_tree, coordinator_identity, created_at, updated_at, released_at
FROM core.runs
WHERE id = $1;

-- name: ListActiveRuns :many
SELECT id, project_id, task_id, task_version_id, project_source_id, status,
    aggregate_version, admitted_task_aggregate_version, source_commit,
    source_tree, coordinator_identity, created_at, updated_at, released_at
FROM core.runs
WHERE status = 'active'
ORDER BY id;

-- name: CountRunAdmissionEvents :one
SELECT count(*)
FROM core.events
WHERE run_id = sqlc.arg(run_id)
  AND (
      (event_type = 'run.admitted' AND aggregate_type = 'run'
       AND aggregate_id = sqlc.arg(run_id) AND aggregate_version = 1)
      OR
      (event_type = 'task.admitted' AND aggregate_type = 'task'
       AND aggregate_id = sqlc.arg(task_id)
       AND aggregate_version = sqlc.arg(task_aggregate_version))
  );

-- name: AcquireGlobalExecutionLease :one
UPDATE core.execution_leases
SET run_id = sqlc.arg(run_id),
    coordinator_identity = sqlc.arg(coordinator_identity),
    acquired_at = sqlc.arg(acquired_at),
    aggregate_version = aggregate_version + 1
WHERE lease_name = 'global-source-mutation-v1'
  AND run_id IS NULL
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING lease_name, run_id, coordinator_identity, acquired_at, aggregate_version;

-- name: AdmitSchedulerTask :one
UPDATE core.tasks
SET status = 'admitted',
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE project_id = sqlc.arg(project_id)
  AND id = sqlc.arg(task_id)
  AND status = 'pending'
  AND accepted_version_id = sqlc.arg(task_version_id)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING id, project_id, external_task_id, status, accepted_version_id,
    created_at, updated_at, aggregate_version;

-- name: ReleaseRun :one
UPDATE core.runs
SET status = 'released',
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(released_at),
    released_at = sqlc.arg(released_at)
WHERE id = sqlc.arg(run_id)
  AND status = 'active'
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING id, project_id, task_id, task_version_id, project_source_id, status,
    aggregate_version, admitted_task_aggregate_version, source_commit,
    source_tree, coordinator_identity, created_at, updated_at, released_at;

-- name: ReleaseGlobalExecutionLease :one
UPDATE core.execution_leases
SET run_id = NULL,
    coordinator_identity = NULL,
    acquired_at = NULL,
    aggregate_version = aggregate_version + 1
WHERE lease_name = 'global-source-mutation-v1'
  AND run_id = sqlc.arg(run_id)
  AND coordinator_identity = sqlc.arg(coordinator_identity)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING lease_name, run_id, coordinator_identity, acquired_at, aggregate_version;

-- name: GetWorkspaceRunAuthority :one
SELECT
    r.id AS run_id, r.project_id, r.task_id, r.task_version_id,
    r.project_source_id, r.status AS run_status, r.source_commit, r.source_tree,
    r.coordinator_identity, t.status AS task_status,
    ps.canonical_source_path, ps.managed_repository_path,
    ps.current_commit, ps.current_tree
FROM core.runs AS r
JOIN core.tasks AS t ON t.id = r.task_id AND t.project_id = r.project_id
JOIN core.project_sources AS ps
  ON ps.id = r.project_source_id AND ps.project_id = r.project_id
WHERE r.id = $1;

-- name: InsertWorkspace :one
INSERT INTO core.workspaces (
    id, run_id, project_id, project_source_id, task_id,
    creation_operation_id, symbolic_source_id, status,
    original_checkout_path, managed_repository_path, workspace_root, workspace_path, branch_ref,
    source_commit, source_tree, original_identity_before, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'planned', $8, $9, $10, $11, $12,
    $13, $14, $15, $16, $16
)
RETURNING *;

-- name: GetWorkspace :one
SELECT * FROM core.workspaces WHERE id = $1;

-- name: GetWorkspaceForUpdate :one
SELECT * FROM core.workspaces WHERE id = $1 FOR UPDATE;

-- name: GetWorkspaceByRunID :one
SELECT * FROM core.workspaces WHERE run_id = $1;

-- name: AdvanceWorkspaceStatus :one
UPDATE core.workspaces
SET status = sqlc.arg(new_status),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(workspace_id)
  AND status = sqlc.arg(expected_status)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING *;

-- name: MarkWorkspaceReady :one
UPDATE core.workspaces
SET status = 'ready',
    workspace_device = sqlc.arg(workspace_device),
    workspace_inode = sqlc.arg(workspace_inode),
    original_identity_after = sqlc.arg(original_identity_after),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(workspace_id)
  AND status = 'creating'
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING *;

-- name: RecordWorkspaceCapture :one
UPDATE core.workspaces
SET status = 'reconciling',
    git_status = sqlc.arg(git_status),
    changed_manifest = sqlc.arg(changed_manifest),
    diff_artifact_id = sqlc.arg(diff_artifact_id),
    diff_sha256 = sqlc.arg(diff_sha256),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(workspace_id)
  AND status = 'frozen'
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING *;

-- name: RecordWorkspaceCandidate :one
UPDATE core.workspaces
SET status = 'completed',
    terminal_status = 'completed',
    candidate_commit = sqlc.arg(candidate_commit),
    candidate_tree = sqlc.arg(candidate_tree),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(workspace_id)
  AND status = 'reconciling'
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING *;

-- name: MarkWorkspaceTerminal :one
UPDATE core.workspaces
SET status = sqlc.arg(terminal_status),
    terminal_status = sqlc.arg(terminal_status),
    terminal_reason = sqlc.arg(terminal_reason),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(workspace_id)
  AND status = sqlc.arg(expected_status)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
  AND sqlc.arg(terminal_status)::text IN ('cancelled', 'failed')
RETURNING *;

-- name: MarkWorkspaceCleaned :one
UPDATE core.workspaces
SET status = 'cleaned',
    aggregate_version = aggregate_version + 1,
    cleanup_completed_at = sqlc.arg(cleanup_completed_at),
    updated_at = sqlc.arg(cleanup_completed_at)
WHERE id = sqlc.arg(workspace_id)
  AND status = sqlc.arg(expected_terminal_status)
  AND terminal_status = sqlc.arg(expected_terminal_status)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING *;

-- name: InsertWorkspaceOperation :one
INSERT INTO core.workspace_operations (
    operation_id, workspace_id, operation_kind, material_sha256,
    status, created_at
) VALUES ($1, $2, $3, $4, 'planned', $5)
RETURNING *;

-- name: GetWorkspaceOperation :one
SELECT * FROM core.workspace_operations WHERE operation_id = $1;

-- name: CompleteWorkspaceOperation :one
UPDATE core.workspace_operations
SET status = 'applied', effect = sqlc.arg(effect), applied_at = sqlc.arg(applied_at)
WHERE operation_id = sqlc.arg(operation_id)
  AND workspace_id = sqlc.arg(workspace_id)
  AND operation_kind = sqlc.arg(operation_kind)
  AND material_sha256 = sqlc.arg(material_sha256)
  AND status = 'planned'
RETURNING *;

-- name: ListWorkspaceEvents :many
SELECT id, project_id, task_id, run_id, event_type, aggregate_type, aggregate_id,
    aggregate_version, payload, created_at
FROM core.events
WHERE aggregate_type = 'workspace' AND aggregate_id = $1
ORDER BY aggregate_version;
