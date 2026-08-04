-- +goose Up
CREATE TABLE core.tasks (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    external_task_id text NOT NULL CHECK (
        external_task_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'
    ),
    status text NOT NULL CHECK (status = 'draft'),
    accepted_version_id uuid,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (project_id, external_task_id),
    UNIQUE (id, project_id),
    CONSTRAINT tasks_imported_drafts_are_unaccepted CHECK (accepted_version_id IS NULL)
);

CREATE TABLE core.task_versions (
    id uuid PRIMARY KEY,
    task_id uuid NOT NULL REFERENCES core.tasks (id),
    version_number integer NOT NULL CHECK (version_number > 0),
    source_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    title text NOT NULL CHECK (title <> '' AND octet_length(title) <= 1024),
    goal text NOT NULL CHECK (goal <> '' AND octet_length(goal) <= 65536),
    risk_class text NOT NULL CHECK (risk_class IN ('low', 'medium', 'high', 'critical')),
    mutation_class text NOT NULL CHECK (mutation_class IN (
        'read_only', 'documentation', 'test_only', 'bounded_source',
        'database_migration', 'dependency_change', 'architecture_change',
        'security_sensitive', 'release_or_deployment'
    )),
    network_profile text NOT NULL CHECK (network_profile IN ('none', 'dependencies', 'open')),
    priority integer NOT NULL CHECK (priority >= 0),
    read_only_investigation boolean NOT NULL,
    scope jsonb NOT NULL CHECK (
        jsonb_typeof(scope) = 'array' AND octet_length(scope::text) <= 262144
    ),
    excluded_scope jsonb NOT NULL CHECK (
        jsonb_typeof(excluded_scope) = 'array' AND octet_length(excluded_scope::text) <= 262144
    ),
    verification_plan jsonb NOT NULL CHECK (
        jsonb_typeof(verification_plan) = 'array' AND octet_length(verification_plan::text) <= 262144
    ),
    budget jsonb NOT NULL CHECK (
        jsonb_typeof(budget) = 'object' AND octet_length(budget::text) <= 16384
    ),
    secret_requirements jsonb NOT NULL CHECK (
        jsonb_typeof(secret_requirements) = 'array' AND octet_length(secret_requirements::text) <= 65536
    ),
    expected_paths jsonb NOT NULL CHECK (
        jsonb_typeof(expected_paths) = 'array' AND octet_length(expected_paths::text) <= 65536
    ),
    operator_checkpoints jsonb NOT NULL CHECK (
        jsonb_typeof(operator_checkpoints) = 'array' AND octet_length(operator_checkpoints::text) <= 65536
    ),
    created_at timestamptz NOT NULL,
    UNIQUE (task_id, version_number),
    UNIQUE (id, task_id)
);

ALTER TABLE core.tasks
    ADD CONSTRAINT tasks_accepted_version_ownership
    FOREIGN KEY (accepted_version_id, id)
    REFERENCES core.task_versions (id, task_id);

CREATE TABLE core.task_imports (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects (id),
    source_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    task_id uuid,
    source_name text NOT NULL CHECK (source_name <> '' AND octet_length(source_name) <= 1024),
    source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    media_type text NOT NULL CHECK (media_type <> '' AND octet_length(media_type) <= 512),
    status text NOT NULL CHECK (status IN ('needs_compilation', 'draft')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (project_id, source_name),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    CHECK (
        (status = 'needs_compilation' AND task_id IS NULL)
        OR (status = 'draft' AND task_id IS NOT NULL)
    )
);

CREATE TABLE core.task_dependencies (
    task_version_id uuid NOT NULL,
    task_id uuid NOT NULL,
    project_id uuid NOT NULL,
    dependency_task_id uuid NOT NULL,
    dependency_type text NOT NULL CHECK (dependency_type = 'requires'),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (task_version_id, dependency_task_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    FOREIGN KEY (dependency_task_id, project_id) REFERENCES core.tasks (id, project_id),
    CHECK (task_id <> dependency_task_id)
);

CREATE TABLE core.task_conflicts (
    task_version_id uuid NOT NULL,
    task_id uuid NOT NULL,
    project_id uuid NOT NULL,
    conflicting_task_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (task_version_id, conflicting_task_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    FOREIGN KEY (conflicting_task_id, project_id) REFERENCES core.tasks (id, project_id),
    CHECK (task_id <> conflicting_task_id)
);

CREATE TABLE core.task_acceptance_criteria (
    id uuid PRIMARY KEY,
    task_id uuid NOT NULL REFERENCES core.tasks (id),
    external_criterion_id text NOT NULL CHECK (
        external_criterion_id ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'
    ),
    status text NOT NULL CHECK (status IN (
        'pending', 'passed', 'failed', 'waived', 'not_applicable', 'blocked'
    )),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (task_id, external_criterion_id),
    UNIQUE (id, task_id)
);

CREATE TABLE core.task_acceptance_versions (
    id uuid PRIMARY KEY,
    criterion_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    version_number integer NOT NULL CHECK (version_number > 0),
    requirement text NOT NULL CHECK (requirement <> '' AND octet_length(requirement) <= 65536),
    verification_method text NOT NULL CHECK (verification_method IN ('command', 'operator_checkpoint')),
    verification_reference text,
    operator_checkpoint jsonb,
    created_at timestamptz NOT NULL,
    UNIQUE (criterion_id, version_number),
    UNIQUE (task_version_id, criterion_id),
    FOREIGN KEY (criterion_id, task_id) REFERENCES core.task_acceptance_criteria (id, task_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    CHECK (
        (verification_method = 'command'
            AND verification_reference IS NOT NULL
            AND verification_reference <> ''
            AND octet_length(verification_reference) <= 65536
            AND operator_checkpoint IS NULL)
        OR (verification_method = 'operator_checkpoint'
            AND verification_reference IS NULL
            AND jsonb_typeof(operator_checkpoint) = 'object'
            AND octet_length(operator_checkpoint::text) <= 65536)
    )
);

-- +goose Down
DROP TABLE core.task_acceptance_versions;
DROP TABLE core.task_acceptance_criteria;
DROP TABLE core.task_conflicts;
DROP TABLE core.task_dependencies;
DROP TABLE core.task_imports;
ALTER TABLE core.tasks DROP CONSTRAINT tasks_accepted_version_ownership;
DROP TABLE core.task_versions;
DROP TABLE core.tasks;
