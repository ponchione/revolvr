-- +goose Up
CREATE TABLE core.artifacts (
    id uuid PRIMARY KEY,
    sha256 text NOT NULL UNIQUE,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    media_type text NOT NULL,
    logical_kind text NOT NULL,
    storage_path text NOT NULL,
    compression text,
    created_at timestamptz NOT NULL
);

CREATE TABLE core.events (
    id uuid PRIMARY KEY,
    project_id uuid,
    task_id uuid,
    run_id uuid,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (aggregate_type, aggregate_id, aggregate_version)
);

-- +goose Down
DROP TABLE core.events;
DROP TABLE core.artifacts;
