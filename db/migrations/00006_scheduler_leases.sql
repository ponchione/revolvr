-- +goose Up
ALTER TABLE core.tasks
    DROP CONSTRAINT tasks_status_check,
    DROP CONSTRAINT tasks_approval_consistency,
    ADD CONSTRAINT tasks_status_check CHECK (status IN (
        'draft', 'compiled', 'awaiting_approval', 'pending', 'admitted',
        'planning', 'ready', 'working', 'verifying', 'auditing', 'correcting',
        'documenting', 'simplifying', 'needs_input', 'blocked', 'finalizing',
        'completed', 'cancelled', 'budget_exhausted', 'unsafe', 'superseded',
        'abandoned', 'retrieval', 'telemetry'
    )),
    ADD CONSTRAINT tasks_approval_consistency CHECK (
        (status IN ('draft', 'compiled', 'awaiting_approval') AND accepted_version_id IS NULL)
        OR (status IN (
            'pending', 'admitted', 'planning', 'ready', 'working', 'verifying',
            'auditing', 'correcting', 'documenting', 'simplifying', 'finalizing',
            'completed'
        ) AND accepted_version_id IS NOT NULL)
        OR status IN (
            'needs_input', 'blocked', 'cancelled', 'budget_exhausted', 'unsafe',
            'superseded', 'abandoned', 'retrieval', 'telemetry'
        )
    );

ALTER TABLE core.project_sources
    ADD CONSTRAINT project_sources_id_project_unique UNIQUE (id, project_id);

CREATE TABLE core.runs (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    project_source_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'released')),
    aggregate_version bigint NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
    admitted_task_aggregate_version bigint NOT NULL CHECK (admitted_task_aggregate_version > 0),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    coordinator_identity text NOT NULL CHECK (
        coordinator_identity <> '' AND octet_length(coordinator_identity) <= 1024
    ),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    released_at timestamptz CHECK (released_at IS NULL OR released_at >= created_at),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    FOREIGN KEY (project_source_id, project_id) REFERENCES core.project_sources (id, project_id),
    CHECK (
        (status = 'active' AND released_at IS NULL)
        OR (status = 'released' AND released_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX runs_one_active_global ON core.runs ((status)) WHERE status = 'active';

CREATE TABLE core.execution_leases (
    lease_name text PRIMARY KEY CHECK (lease_name = 'global-source-mutation-v1'),
    run_id uuid UNIQUE REFERENCES core.runs (id),
    coordinator_identity text CHECK (
        coordinator_identity IS NULL OR (
            coordinator_identity <> '' AND octet_length(coordinator_identity) <= 1024
        )
    ),
    acquired_at timestamptz,
    aggregate_version bigint NOT NULL DEFAULT 0 CHECK (aggregate_version >= 0),
    CHECK (
        (run_id IS NULL AND coordinator_identity IS NULL AND acquired_at IS NULL)
        OR (run_id IS NOT NULL AND coordinator_identity IS NOT NULL AND acquired_at IS NOT NULL)
    )
);

INSERT INTO core.execution_leases (lease_name)
VALUES ('global-source-mutation-v1');

ALTER TABLE core.events
    ADD CONSTRAINT events_run_id_fkey FOREIGN KEY (run_id) REFERENCES core.runs (id);

-- +goose Down
ALTER TABLE core.events DROP CONSTRAINT events_run_id_fkey;
DROP TABLE core.execution_leases;
DROP INDEX core.runs_one_active_global;
DROP TABLE core.runs;
ALTER TABLE core.project_sources DROP CONSTRAINT project_sources_id_project_unique;

ALTER TABLE core.tasks
    DROP CONSTRAINT tasks_approval_consistency,
    DROP CONSTRAINT tasks_status_check,
    ADD CONSTRAINT tasks_status_check CHECK (status IN (
        'draft', 'compiled', 'awaiting_approval', 'pending', 'admitted',
        'planning', 'ready', 'working', 'verifying', 'auditing', 'correcting',
        'documenting', 'simplifying', 'needs_input', 'blocked', 'finalizing',
        'completed', 'cancelled', 'budget_exhausted', 'unsafe', 'retrieval',
        'telemetry'
    )),
    ADD CONSTRAINT tasks_approval_consistency CHECK (
        (status IN ('draft', 'compiled', 'awaiting_approval') AND accepted_version_id IS NULL)
        OR (status IN (
            'pending', 'admitted', 'planning', 'ready', 'working', 'verifying',
            'auditing', 'correcting', 'documenting', 'simplifying', 'finalizing',
            'completed'
        ) AND accepted_version_id IS NOT NULL)
        OR status IN (
            'needs_input', 'blocked', 'cancelled', 'budget_exhausted', 'unsafe',
            'retrieval', 'telemetry'
        )
    );
