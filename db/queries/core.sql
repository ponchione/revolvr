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

-- name: GetArtifactByID :one
SELECT id, sha256, size_bytes, media_type, logical_kind, storage_path, compression, created_at
FROM core.artifacts
WHERE id = $1;

-- name: InsertVerificationRun :one
INSERT INTO core.verification_runs (
    id, project_id, task_id, task_version_id, run_id, workspace_id, purpose,
    status, plan_schema_version, plan_version, plan_sha256, pinned_plan,
    candidate_commit, candidate_tree, project_environment_sha256,
    project_environment, differential, started_at, completed_at, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20
)
RETURNING id, project_id, task_id, task_version_id, run_id, workspace_id,
    purpose, status, plan_schema_version, plan_version, plan_sha256,
    pinned_plan, candidate_commit, candidate_tree, project_environment_sha256,
    project_environment, differential, started_at, completed_at, created_at;

-- name: InsertVerificationCheck :one
INSERT INTO core.verification_checks (
    id, verification_run_id, run_id, ordinal, gate_id, tier, outcome,
    execution_fingerprint, verifier_protocol_version,
    verifier_implementation_version, parser_kind, parser_version,
    source_commit, source_tree, command_argv, working_directory, environment,
    image_reference, image_digest, sandbox_profile, sandbox_profile_sha256,
    sandbox_specification_sha256, authority_inputs, output_policy, exit_code,
    timed_out, cancelled, stdout_artifact_id, stderr_artifact_id, parsed_result,
    sandbox_evidence, failure_signatures, reused_from_check_id,
    original_executed_at, occurred_at, started_at, completed_at, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27,
    $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38
)
RETURNING id, verification_run_id, run_id, ordinal, gate_id, tier, outcome,
    execution_fingerprint, verifier_protocol_version,
    verifier_implementation_version, parser_kind, parser_version,
    source_commit, source_tree, command_argv, working_directory, environment,
    image_reference, image_digest, sandbox_profile, sandbox_profile_sha256,
    sandbox_specification_sha256, authority_inputs, output_policy, exit_code,
    timed_out, cancelled, stdout_artifact_id, stderr_artifact_id, parsed_result,
    sandbox_evidence, failure_signatures, reused_from_check_id,
    original_executed_at, occurred_at, started_at, completed_at, created_at;

-- name: GetVerificationRun :one
SELECT id, project_id, task_id, task_version_id, run_id, workspace_id,
    purpose, status, plan_schema_version, plan_version, plan_sha256,
    pinned_plan, candidate_commit, candidate_tree, project_environment_sha256,
    project_environment, differential, started_at, completed_at, created_at
FROM core.verification_runs
WHERE id = $1;

-- name: ListVerificationChecks :many
SELECT id, verification_run_id, run_id, ordinal, gate_id, tier, outcome,
    execution_fingerprint, verifier_protocol_version,
    verifier_implementation_version, parser_kind, parser_version,
    source_commit, source_tree, command_argv, working_directory, environment,
    image_reference, image_digest, sandbox_profile, sandbox_profile_sha256,
    sandbox_specification_sha256, authority_inputs, output_policy, exit_code,
    timed_out, cancelled, stdout_artifact_id, stderr_artifact_id, parsed_result,
    sandbox_evidence, failure_signatures, reused_from_check_id,
    original_executed_at, occurred_at, started_at, completed_at, created_at
FROM core.verification_checks
WHERE verification_run_id = $1
ORDER BY ordinal;

-- name: FindReusableVerificationCheck :one
SELECT id, verification_run_id, run_id, ordinal, gate_id, tier, outcome,
    execution_fingerprint, verifier_protocol_version,
    verifier_implementation_version, parser_kind, parser_version,
    source_commit, source_tree, command_argv, working_directory, environment,
    image_reference, image_digest, sandbox_profile, sandbox_profile_sha256,
    sandbox_specification_sha256, authority_inputs, output_policy, exit_code,
    timed_out, cancelled, stdout_artifact_id, stderr_artifact_id, parsed_result,
    sandbox_evidence, failure_signatures, reused_from_check_id,
    original_executed_at, occurred_at, started_at, completed_at, created_at
FROM core.verification_checks
WHERE execution_fingerprint = $1
  AND reused_from_check_id IS NULL
  AND outcome IN ('passed', 'failed')
ORDER BY completed_at DESC, id DESC
LIMIT 1;

-- name: GetVerificationPersistenceAuthority :one
SELECT
    r.project_id, r.task_id, r.task_version_id, r.status AS run_status,
    t.accepted_version_id, t.status AS task_status,
    tv.verification_plan AS accepted_verification_plan,
    w.id AS workspace_id, w.run_id AS workspace_run_id,
    w.project_id AS workspace_project_id, w.task_id AS workspace_task_id,
    w.status AS workspace_status, w.candidate_commit, w.candidate_tree
FROM core.runs AS r
JOIN core.tasks AS t ON t.id = r.task_id AND t.project_id = r.project_id
JOIN core.task_versions AS tv ON tv.id = r.task_version_id AND tv.task_id = r.task_id
JOIN core.workspaces AS w ON w.run_id = r.id
WHERE r.id = sqlc.arg(run_id) AND w.id = sqlc.arg(workspace_id)
FOR SHARE OF r, t, w;

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

-- name: GetPlannerRunAuthority :one
SELECT
    r.id AS run_id, r.project_id, r.task_id, r.task_version_id,
    r.project_source_id, r.status AS run_status, r.source_commit, r.source_tree,
    t.status AS task_status, t.accepted_version_id, t.aggregate_version AS task_aggregate_version,
    ps.current_commit, ps.current_tree
FROM core.runs AS r
JOIN core.tasks AS t ON t.id = r.task_id AND t.project_id = r.project_id
JOIN core.project_sources AS ps
  ON ps.id = r.project_source_id AND ps.project_id = r.project_id
WHERE r.id = $1
FOR UPDATE OF r, t, ps;

-- name: InsertPlan :one
INSERT INTO core.plans (
    id, project_id, task_id, task_version_id, run_id, project_source_id,
    source_revision, source_commit, source_tree, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
RETURNING *;

-- name: GetPlan :one
SELECT * FROM core.plans WHERE id = $1;

-- name: GetPlanForUpdate :one
SELECT * FROM core.plans WHERE id = $1 FOR UPDATE;

-- name: GetPlanByRunID :one
SELECT * FROM core.plans WHERE run_id = $1;

-- name: AdvancePlanCandidate :one
UPDATE core.plans
SET aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(plan_id)
  AND aggregate_version = sqlc.arg(expected_aggregate_version)
RETURNING *;

-- name: InsertPlanVersion :one
INSERT INTO core.plan_versions (
    id, plan_id, task_id, task_version_id, run_id, project_source_id,
    revision_number, supersedes_version_id, candidate_sha256, content_sha256,
    change_explanation, source_revision,
    supervisor_decision_id, supervisor_decision_sha256,
    dossier_version, dossier_sha256, dossier_content,
    prompt_version, prompt_sha256, prompt_content,
    response_schema_version, response_schema_sha256, response_schema,
    model_policy_version, model_policy_sha256, model_policy,
    host_policy_version, host_policy_sha256, host_policy,
    expected_request, model_result, raw_output, canonical_output, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26,
    $27, $28, $29, $30, $31, $32, $33, $34
)
RETURNING *;

-- name: GetPlanVersion :one
SELECT * FROM core.plan_versions WHERE id = $1;

-- name: GetPlanVersionByRevision :one
SELECT * FROM core.plan_versions
WHERE plan_id = sqlc.arg(plan_id) AND revision_number = sqlc.arg(revision_number);

-- name: InsertPlanStep :one
INSERT INTO core.plan_steps (
    plan_version_id, plan_id, step_id, ordinal, status, description,
    criterion_ids, depends_on_step_ids, expected_paths, components,
    test_strategy, risks, assumptions, evidence_refs, lineage
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: ListPlanSteps :many
SELECT * FROM core.plan_steps WHERE plan_version_id = $1 ORDER BY ordinal;

-- name: AcceptPlanVersion :one
UPDATE core.plans AS p
SET accepted_version_id = sqlc.arg(plan_version_id),
    accepted_operation_id = sqlc.arg(operation_id),
    accepted_by = sqlc.arg(accepted_by),
    accepted_at = sqlc.arg(accepted_at),
    aggregate_version = aggregate_version + 1,
    updated_at = sqlc.arg(accepted_at)
WHERE p.id = sqlc.arg(plan_id)
  AND p.aggregate_version = sqlc.arg(expected_aggregate_version)
  AND (p.accepted_version_id IS NULL
       OR p.accepted_version_id <> sqlc.arg(plan_version_id))
  AND EXISTS (
      SELECT 1 FROM core.plan_versions AS pv
      WHERE pv.id = sqlc.arg(plan_version_id) AND pv.plan_id = p.id
  )
RETURNING p.*;

-- name: ListPlanEvents :many
SELECT id, project_id, task_id, run_id, event_type, aggregate_type, aggregate_id,
    aggregate_version, payload, created_at
FROM core.events
WHERE aggregate_type = 'plan' AND aggregate_id = $1
ORDER BY aggregate_version;

-- name: GetCompletionPersistenceAuthority :one
SELECT
    t.project_id,
    t.id AS task_id,
    t.accepted_version_id,
    t.status AS task_status,
    t.aggregate_version AS task_aggregate_version,
    r.id AS run_id,
    r.task_version_id,
    r.status AS run_status,
    r.aggregate_version AS run_aggregate_version,
    w.id AS workspace_id,
    w.status AS workspace_status,
    w.aggregate_version AS workspace_aggregate_version,
    w.candidate_commit,
    w.candidate_tree,
    w.diff_artifact_id,
    w.diff_sha256,
    p.id AS plan_id,
    p.accepted_version_id AS accepted_plan_version_id,
    pv.content_sha256 AS accepted_plan_content_sha256,
    p.aggregate_version AS plan_aggregate_version,
    l.lease_name,
    l.run_id AS lease_run_id,
    l.aggregate_version AS lease_aggregate_version
FROM core.tasks t
JOIN core.runs r ON r.task_id = t.id AND r.project_id = t.project_id
JOIN core.workspaces w ON w.run_id = r.id
JOIN core.plans p ON p.run_id = r.id
JOIN core.plan_versions pv ON pv.id = p.accepted_version_id AND pv.plan_id = p.id
JOIN core.execution_leases l ON l.lease_name = 'global-source-mutation-v1'
WHERE t.id = $1 AND r.id = $2 AND w.id = $3
FOR UPDATE OF t, r, w, p, l;

-- name: GetCompletionReadAuthority :one
SELECT
    t.project_id,
    t.id AS task_id,
    t.accepted_version_id,
    t.status AS task_status,
    t.aggregate_version AS task_aggregate_version,
    r.id AS run_id,
    r.task_version_id,
    r.status AS run_status,
    r.aggregate_version AS run_aggregate_version,
    r.source_commit AS before_commit,
    r.source_tree AS before_tree,
    w.id AS workspace_id,
    w.status AS workspace_status,
    w.aggregate_version AS workspace_aggregate_version,
    w.candidate_commit,
    w.candidate_tree,
    w.diff_artifact_id,
    w.diff_sha256,
    w.updated_at AS workspace_updated_at,
    p.id AS plan_id,
    p.accepted_version_id AS accepted_plan_version_id,
    pv.content_sha256 AS accepted_plan_content_sha256,
    p.aggregate_version AS plan_aggregate_version,
    l.lease_name,
    l.run_id AS lease_run_id,
    l.aggregate_version AS lease_aggregate_version
FROM core.tasks t
JOIN core.runs r ON r.task_id = t.id AND r.project_id = t.project_id
JOIN core.workspaces w ON w.run_id = r.id
JOIN core.plans p ON p.run_id = r.id
JOIN core.plan_versions pv ON pv.id = p.accepted_version_id AND pv.plan_id = p.id
JOIN core.execution_leases l ON l.lease_name = 'global-source-mutation-v1'
WHERE t.id = $1 AND r.id = $2 AND w.id = $3;

-- name: GetCompletionVerificationAuthority :one
SELECT
    v.id,
    v.project_id,
    v.task_id,
    v.task_version_id,
    v.run_id,
    v.workspace_id,
    v.purpose,
    v.status,
    v.candidate_commit,
    v.candidate_tree,
    v.completed_at,
    count(c.id)::bigint AS check_count,
    count(c.id) FILTER (
        WHERE c.tier = 4
          AND c.outcome = 'passed'
          AND c.reused_from_check_id IS NULL
    )::bigint AS fresh_final_check_count,
    count(c.id) FILTER (
        WHERE c.outcome NOT IN ('passed', 'passed_reused')
           OR (c.tier = 4 AND (c.outcome <> 'passed' OR c.reused_from_check_id IS NOT NULL))
    )::bigint AS nonfresh_or_nonpassing_check_count
FROM core.verification_runs v
JOIN core.verification_checks c ON c.verification_run_id = v.id
WHERE v.id = $1
GROUP BY v.id;

-- name: CountCompletionNonterminalPlanSteps :one
SELECT count(*)
FROM core.plan_steps
WHERE plan_version_id = $1 AND status NOT IN ('completed', 'skipped');

-- name: CountCompletionUnsatisfiedCriteria :one
SELECT count(*)
FROM core.task_acceptance_criteria
WHERE task_id = $1 AND status NOT IN ('passed', 'waived', 'not_applicable');

-- name: ListCompletionCriteria :many
SELECT id, external_criterion_id, status
FROM core.task_acceptance_criteria
WHERE task_id = $1
ORDER BY external_criterion_id, id;

-- name: InsertArtifactProvenance :one
INSERT INTO core.artifact_provenance (
    id, artifact_id, project_id, task_id, task_version_id, run_id, workspace_id,
    producer_role, producing_operation_id, source_commit, source_tree, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: InsertClaim :one
INSERT INTO core.claims (
    id, project_id, task_id, task_version_id, run_id, criterion_id, claim_key,
    statement, statement_sha256, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING *;

-- name: InsertClaimEvidence :one
INSERT INTO core.claim_evidence (
    claim_id, project_id, task_id, task_version_id, run_id, ordinal,
    evidence_kind, artifact_id, verification_check_id, evidence_sha256, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING *;

-- name: InsertCompletion :one
INSERT INTO core.completions (
    id, operation_id, project_id, task_id, task_version_id, run_id, workspace_id,
    verification_run_id, preflight_sha256, evidence_artifact_id, evidence_sha256,
    markdown_artifact_id, markdown_sha256, manifest_artifact_id, manifest_sha256,
    trajectory_envelope, trajectory_sha256, harness_asset_set_manifest,
    harness_asset_set_sha256, completed_at, created_at
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21
)
RETURNING *;

-- name: GetCompletionByOperationID :one
SELECT * FROM core.completions WHERE operation_id = $1;

-- name: InsertCompletionArtifact :one
INSERT INTO core.completion_artifacts (
    completion_id, ordinal, artifact_id, artifact_sha256, artifact_role
) VALUES ($1,$2,$3,$4,$5)
RETURNING *;

-- name: InsertCompletionClaim :exec
INSERT INTO core.completion_claims (
    completion_id, project_id, task_id, task_version_id, run_id, claim_id
) VALUES ($1,$2,$3,$4,$5,$6);

-- name: CompleteTask :one
UPDATE core.tasks
SET status = 'completed', aggregate_version = aggregate_version + 1, updated_at = $3
WHERE id = $1 AND status = 'finalizing' AND aggregate_version = $2
RETURNING *;

-- name: CompleteRun :one
UPDATE core.runs
SET status = 'released', aggregate_version = aggregate_version + 1,
    released_at = $3, updated_at = $3
WHERE id = $1 AND status = 'active' AND aggregate_version = $2
RETURNING *;

-- name: CompleteWorkspace :one
UPDATE core.workspaces
SET status = 'completed', terminal_status = 'completed', terminal_reason = $3,
    aggregate_version = aggregate_version + 1, updated_at = $4
WHERE id = $1 AND status = 'frozen' AND aggregate_version = $2
RETURNING *;

-- name: ReleaseCompletionLease :one
UPDATE core.execution_leases
SET run_id = NULL, coordinator_identity = NULL, acquired_at = NULL,
    aggregate_version = aggregate_version + 1
WHERE lease_name = 'global-source-mutation-v1'
  AND run_id = $1 AND aggregate_version = $2
RETURNING *;
