-- +goose Up
CREATE TABLE core.workspaces (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL UNIQUE REFERENCES core.runs (id),
    project_id uuid NOT NULL REFERENCES core.projects (id),
    project_source_id uuid NOT NULL,
    task_id uuid NOT NULL,
    creation_operation_id text NOT NULL UNIQUE CHECK (
        creation_operation_id <> '' AND octet_length(creation_operation_id) <= 512
    ),
    symbolic_source_id text NOT NULL UNIQUE CHECK (
        symbolic_source_id ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$'
    ),
    status text NOT NULL CHECK (status IN (
        'planned', 'creating', 'ready', 'active', 'frozen', 'reconciling',
        'completed', 'cancelled', 'failed', 'cleaned'
    )),
    terminal_status text CHECK (terminal_status IN ('completed', 'cancelled', 'failed')),
    aggregate_version bigint NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
    original_checkout_path text NOT NULL CHECK (original_checkout_path <> ''),
    managed_repository_path text NOT NULL CHECK (managed_repository_path <> ''),
    workspace_root text NOT NULL CHECK (workspace_root <> ''),
    workspace_path text NOT NULL UNIQUE CHECK (workspace_path <> ''),
    branch_ref text NOT NULL UNIQUE CHECK (branch_ref ~ '^refs/heads/revolvr/workspaces/[A-Za-z0-9._-]+$'),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    workspace_device bigint,
    workspace_inode bigint,
    original_identity_before jsonb NOT NULL CHECK (
        jsonb_typeof(original_identity_before) = 'object'
        AND octet_length(original_identity_before::text) <= 4194304
    ),
    original_identity_after jsonb CHECK (
        original_identity_after IS NULL OR (
            jsonb_typeof(original_identity_after) = 'object'
            AND octet_length(original_identity_after::text) <= 4194304
        )
    ),
    git_status bytea,
    changed_manifest jsonb CHECK (
        changed_manifest IS NULL OR (
            jsonb_typeof(changed_manifest) = 'array'
            AND octet_length(changed_manifest::text) <= 4194304
        )
    ),
    diff_artifact_id uuid REFERENCES core.artifacts (id),
    diff_sha256 text CHECK (diff_sha256 IS NULL OR diff_sha256 ~ '^[0-9a-f]{64}$'),
    candidate_commit text CHECK (candidate_commit IS NULL OR candidate_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    candidate_tree text CHECK (candidate_tree IS NULL OR candidate_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    terminal_reason text CHECK (
        terminal_reason IS NULL OR (terminal_reason <> '' AND octet_length(terminal_reason) <= 4096)
    ),
    cleanup_completed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    FOREIGN KEY (project_source_id, project_id) REFERENCES core.project_sources (id, project_id),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    CHECK ((workspace_device IS NULL) = (workspace_inode IS NULL)),
    CHECK ((original_identity_after IS NULL) = (workspace_device IS NULL)),
    CHECK ((diff_artifact_id IS NULL) = (diff_sha256 IS NULL)),
    CHECK ((candidate_commit IS NULL) = (candidate_tree IS NULL)),
    CHECK (
        (status = 'cleaned' AND terminal_status IS NOT NULL AND cleanup_completed_at IS NOT NULL)
        OR (status IN ('completed', 'cancelled', 'failed') AND terminal_status = status AND cleanup_completed_at IS NULL)
        OR (status NOT IN ('completed', 'cancelled', 'failed', 'cleaned')
            AND terminal_status IS NULL AND cleanup_completed_at IS NULL)
    )
);

CREATE TABLE core.workspace_operations (
    operation_id text PRIMARY KEY CHECK (
        operation_id <> '' AND octet_length(operation_id) <= 512
    ),
    workspace_id uuid NOT NULL REFERENCES core.workspaces (id),
    operation_kind text NOT NULL CHECK (operation_kind IN (
        'branch_create', 'worktree_create', 'capture', 'commit', 'worktree_cleanup'
    )),
    material_sha256 text NOT NULL CHECK (material_sha256 ~ '^[0-9a-f]{64}$'),
    status text NOT NULL CHECK (status IN ('planned', 'applied')),
    effect jsonb CHECK (
        effect IS NULL OR (
            jsonb_typeof(effect) = 'object' AND octet_length(effect::text) <= 4194304
        )
    ),
    created_at timestamptz NOT NULL,
    applied_at timestamptz CHECK (applied_at IS NULL OR applied_at >= created_at),
    CHECK (
        (status = 'planned' AND effect IS NULL AND applied_at IS NULL)
        OR (status = 'applied' AND effect IS NOT NULL AND applied_at IS NOT NULL)
    )
);

-- +goose Down
DROP TABLE core.workspace_operations;
DROP TABLE core.workspaces;
