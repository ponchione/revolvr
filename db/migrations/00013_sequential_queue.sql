-- +goose Up
CREATE TABLE core.queue_operations (
    id uuid PRIMARY KEY,
    schema_version text NOT NULL CHECK (schema_version = 'revolvr-sequential-queue-operation-v1'),
    status text NOT NULL CHECK (status IN ('active', 'terminal')),
    worker_mode text NOT NULL CHECK (worker_mode = 'direct_tools_v1'),
    maximum_workers smallint NOT NULL CHECK (maximum_workers = 1),
    quality_gate_status text NOT NULL CHECK (
        quality_gate_status = 'deterministic_evaluation_only'
    ),
    config_schema text NOT NULL CHECK (config_schema <> '' AND octet_length(config_schema) <= 256),
    config_sha256 text NOT NULL CHECK (config_sha256 ~ '^[0-9a-f]{64}$'),
    configuration jsonb NOT NULL,
    max_tasks bigint NOT NULL CHECK (max_tasks > 0),
    max_cycles_per_task bigint NOT NULL CHECK (max_cycles_per_task > 0),
    max_total_cycles bigint NOT NULL CHECK (max_total_cycles > 0),
    max_remote_tokens bigint NOT NULL CHECK (max_remote_tokens > 0),
    max_cost_microusd bigint NOT NULL CHECK (max_cost_microusd > 0),
    max_duration_milliseconds bigint NOT NULL CHECK (max_duration_milliseconds > 0),
    tasks_started bigint NOT NULL DEFAULT 0 CHECK (tasks_started >= 0),
    cycles_consumed bigint NOT NULL DEFAULT 0 CHECK (cycles_consumed >= 0),
    remote_tokens_consumed bigint NOT NULL DEFAULT 0 CHECK (remote_tokens_consumed >= 0),
    cost_microusd_consumed bigint NOT NULL DEFAULT 0 CHECK (cost_microusd_consumed >= 0),
    peak_source_mutating_workers smallint NOT NULL DEFAULT 0 CHECK (
        peak_source_mutating_workers BETWEEN 0 AND 1
    ),
    next_occurrence_sequence bigint NOT NULL DEFAULT 1 CHECK (next_occurrence_sequence > 0),
    selection_intent_id uuid,
    selection_scheduler_run_id uuid,
    selection_intent_sequence bigint CHECK (selection_intent_sequence IS NULL OR selection_intent_sequence > 0),
    active_occurrence_id uuid,
    cancel_requested_at timestamptz,
    started_at timestamptz NOT NULL,
    deadline_at timestamptz NOT NULL CHECK (deadline_at > started_at),
    updated_at timestamptz NOT NULL CHECK (updated_at >= started_at),
    terminal_at timestamptz,
    stop_reason text CHECK (stop_reason IN (
        'drained', 'waiting_on_dependencies', 'waiting_on_input',
        'all_remaining_blocked', 'budget_exhausted', 'cancelled', 'unsafe',
        'system_failure'
    )),
    stop_detail text CHECK (stop_detail IS NULL OR octet_length(stop_detail) <= 8192),
    terminal_marker_sha256 text CHECK (
        terminal_marker_sha256 IS NULL OR terminal_marker_sha256 ~ '^[0-9a-f]{64}$'
    ),
    aggregate_version bigint NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
    CHECK (
        (status = 'active' AND terminal_at IS NULL AND stop_reason IS NULL
            AND terminal_marker_sha256 IS NULL)
        OR
        (status = 'terminal' AND terminal_at IS NOT NULL AND stop_reason IS NOT NULL
            AND terminal_marker_sha256 IS NOT NULL AND active_occurrence_id IS NULL
            AND selection_intent_id IS NULL)
    ),
    CHECK (
        (selection_intent_id IS NULL AND selection_scheduler_run_id IS NULL AND selection_intent_sequence IS NULL)
        OR
        (selection_intent_id IS NOT NULL AND selection_scheduler_run_id IS NOT NULL AND selection_intent_sequence IS NOT NULL)
    ),
    CHECK (NOT (selection_intent_id IS NOT NULL AND active_occurrence_id IS NOT NULL)),
    CHECK (tasks_started <= max_tasks),
    CHECK (cycles_consumed <= max_total_cycles),
    CHECK (remote_tokens_consumed <= max_remote_tokens),
    CHECK (cost_microusd_consumed <= max_cost_microusd)
);

CREATE UNIQUE INDEX queue_operations_one_active
    ON core.queue_operations ((status)) WHERE status = 'active';

CREATE TABLE core.queue_task_occurrences (
    id uuid PRIMARY KEY,
    queue_operation_id uuid NOT NULL REFERENCES core.queue_operations (id) ON DELETE RESTRICT,
    occurrence_sequence bigint NOT NULL CHECK (occurrence_sequence > 0),
    state text NOT NULL CHECK (state IN (
        'selection_intent', 'selected', 'admitted', 'runner_terminal', 'checkpointed'
    )),
    scheduler_run_id uuid NOT NULL,
    coordinator_identity text NOT NULL CHECK (
        coordinator_identity <> '' AND octet_length(coordinator_identity) <= 1024
    ),
    project_id uuid REFERENCES core.projects (id),
    project_source_id uuid,
    task_id uuid,
    task_version_id uuid,
    external_task_id text,
    expected_task_aggregate_version bigint,
    task_priority integer,
    task_created_at timestamptz,
    source_commit text,
    source_tree text,
    selection jsonb,
    selection_sha256 text CHECK (selection_sha256 IS NULL OR selection_sha256 ~ '^[0-9a-f]{64}$'),
    outcome text CHECK (outcome IN (
        'completed', 'blocked', 'needs_input', 'dependency_waiting',
        'task_budget_exhausted', 'cancelled', 'unsafe', 'system_failure'
    )),
    outcome_detail text CHECK (outcome_detail IS NULL OR octet_length(outcome_detail) <= 8192),
    result jsonb,
    result_sha256 text CHECK (result_sha256 IS NULL OR result_sha256 ~ '^[0-9a-f]{64}$'),
    effect_chain_sha256 text CHECK (effect_chain_sha256 IS NULL OR effect_chain_sha256 ~ '^[0-9a-f]{64}$'),
    cycles_consumed bigint CHECK (cycles_consumed IS NULL OR cycles_consumed >= 0),
    remote_tokens_consumed bigint CHECK (remote_tokens_consumed IS NULL OR remote_tokens_consumed >= 0),
    cost_microusd_consumed bigint CHECK (cost_microusd_consumed IS NULL OR cost_microusd_consumed >= 0),
    workspace_reconciled boolean,
    evidence_reconciled boolean,
    lease_reconciled boolean NOT NULL DEFAULT false,
    selected_at timestamptz,
    admitted_at timestamptz,
    runner_terminal_at timestamptz,
    checkpointed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (queue_operation_id, occurrence_sequence),
    UNIQUE (queue_operation_id, scheduler_run_id),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    FOREIGN KEY (project_source_id, project_id) REFERENCES core.project_sources (id, project_id),
    CHECK (
        state = 'selection_intent'
        OR (
            project_id IS NOT NULL AND project_source_id IS NOT NULL
            AND task_id IS NOT NULL AND task_version_id IS NOT NULL
            AND external_task_id IS NOT NULL AND expected_task_aggregate_version IS NOT NULL
            AND task_priority IS NOT NULL AND task_created_at IS NOT NULL
            AND source_commit IS NOT NULL AND source_tree IS NOT NULL
            AND selection IS NOT NULL AND selection_sha256 IS NOT NULL
            AND selected_at IS NOT NULL
        )
    ),
    CHECK (
        state NOT IN ('runner_terminal', 'checkpointed')
        OR (
            outcome IS NOT NULL AND result IS NOT NULL AND result_sha256 IS NOT NULL
            AND effect_chain_sha256 IS NOT NULL
            AND cycles_consumed IS NOT NULL AND remote_tokens_consumed IS NOT NULL
            AND cost_microusd_consumed IS NOT NULL
            AND workspace_reconciled IS NOT NULL AND evidence_reconciled IS NOT NULL
            AND runner_terminal_at IS NOT NULL
        )
    ),
    CHECK (state <> 'checkpointed' OR (checkpointed_at IS NOT NULL AND lease_reconciled))
);

ALTER TABLE core.queue_operations
    ADD CONSTRAINT queue_operations_active_occurrence_fkey
    FOREIGN KEY (active_occurrence_id) REFERENCES core.queue_task_occurrences (id);

CREATE TABLE core.queue_task_effects (
    id uuid PRIMARY KEY,
    queue_operation_id uuid NOT NULL REFERENCES core.queue_operations (id) ON DELETE RESTRICT,
    task_occurrence_id uuid NOT NULL REFERENCES core.queue_task_occurrences (id) ON DELETE RESTRICT,
    effect_sequence bigint NOT NULL CHECK (effect_sequence > 0),
    effect_kind text NOT NULL CHECK (effect_kind IN (
        'supervisor', 'worker', 'verification', 'audit', 'correction', 'completion'
    )),
    effect_identity text NOT NULL CHECK (effect_identity <> '' AND octet_length(effect_identity) <= 1024),
    material_sha256 text NOT NULL CHECK (material_sha256 ~ '^[0-9a-f]{64}$'),
    status text NOT NULL CHECK (status IN ('intent', 'completed')),
    evidence_sha256 text CHECK (evidence_sha256 IS NULL OR evidence_sha256 ~ '^[0-9a-f]{64}$'),
    intended_at timestamptz NOT NULL,
    completed_at timestamptz,
    UNIQUE (task_occurrence_id, effect_sequence),
    UNIQUE (task_occurrence_id, effect_identity),
    CHECK (
        (status = 'intent' AND evidence_sha256 IS NULL AND completed_at IS NULL)
        OR (status = 'completed' AND evidence_sha256 IS NOT NULL AND completed_at IS NOT NULL)
    )
);

-- +goose Down
DROP TABLE core.queue_task_effects;
ALTER TABLE core.queue_operations DROP CONSTRAINT queue_operations_active_occurrence_fkey;
DROP TABLE core.queue_task_occurrences;
DROP INDEX core.queue_operations_one_active;
DROP TABLE core.queue_operations;
