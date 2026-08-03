-- +goose Up
CREATE TABLE core.projects (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (name <> ''),
    status text NOT NULL CHECK (status <> ''),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at)
);

CREATE TABLE core.project_sources (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    canonical_source_path text NOT NULL UNIQUE CHECK (canonical_source_path <> ''),
    managed_repository_path text NOT NULL UNIQUE CHECK (managed_repository_path <> ''),
    current_commit text NOT NULL CHECK (current_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    current_tree text NOT NULL CHECK (current_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    current_branch text,
    default_branch text,
    dirty_state jsonb NOT NULL CHECK (
        jsonb_typeof(dirty_state) = 'object'
        AND octet_length(dirty_state::text) <= 2097152
    ),
    remotes jsonb NOT NULL CHECK (
        jsonb_typeof(remotes) = 'array'
        AND octet_length(remotes::text) <= 262144
    )
);

-- +goose Down
DROP TABLE core.project_sources;
DROP TABLE core.projects;
