-- +goose Up
ALTER TABLE core.tasks
    DROP CONSTRAINT tasks_status_check,
    DROP CONSTRAINT tasks_imported_drafts_are_unaccepted,
    ADD COLUMN aggregate_version bigint NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
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

-- +goose Down
ALTER TABLE core.tasks
    DROP CONSTRAINT tasks_approval_consistency,
    DROP CONSTRAINT tasks_status_check,
    DROP COLUMN aggregate_version,
    ADD CONSTRAINT tasks_status_check CHECK (status = 'draft'),
    ADD CONSTRAINT tasks_imported_drafts_are_unaccepted CHECK (accepted_version_id IS NULL);
