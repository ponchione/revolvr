-- +goose Up
CREATE TABLE core.plans (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL UNIQUE,
    project_source_id uuid NOT NULL,
    source_revision text NOT NULL CHECK (source_revision ~ '^[0-9a-f]{64}$'),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    accepted_version_id uuid,
    accepted_operation_id text UNIQUE CHECK (
        accepted_operation_id IS NULL OR (
            accepted_operation_id <> '' AND octet_length(accepted_operation_id) <= 512
        )
    ),
    accepted_by text CHECK (
        accepted_by IS NULL OR (accepted_by <> '' AND octet_length(accepted_by) <= 1024)
    ),
    accepted_at timestamptz,
    aggregate_version bigint NOT NULL DEFAULT 0 CHECK (aggregate_version >= 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    FOREIGN KEY (run_id) REFERENCES core.runs (id),
    FOREIGN KEY (project_source_id, project_id) REFERENCES core.project_sources (id, project_id),
    UNIQUE (id, task_id),
    CHECK (
        (accepted_version_id IS NULL AND accepted_operation_id IS NULL
            AND accepted_by IS NULL AND accepted_at IS NULL)
        OR (accepted_version_id IS NOT NULL AND accepted_operation_id IS NOT NULL
            AND accepted_by IS NOT NULL AND accepted_at IS NOT NULL)
    )
);

CREATE TABLE core.plan_versions (
    id uuid PRIMARY KEY,
    plan_id uuid NOT NULL REFERENCES core.plans (id),
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    project_source_id uuid NOT NULL,
    revision_number integer NOT NULL CHECK (revision_number > 0),
    supersedes_version_id uuid,
    candidate_sha256 text NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    change_explanation text NOT NULL CHECK (
        change_explanation <> '' AND octet_length(change_explanation) <= 65536
    ),
    source_revision text NOT NULL CHECK (source_revision ~ '^[0-9a-f]{64}$'),
    supervisor_decision_id text NOT NULL CHECK (
        supervisor_decision_id <> '' AND octet_length(supervisor_decision_id) <= 512
    ),
    supervisor_decision_sha256 text NOT NULL CHECK (supervisor_decision_sha256 ~ '^[0-9a-f]{64}$'),
    dossier_version text NOT NULL CHECK (dossier_version <> '' AND octet_length(dossier_version) <= 256),
    dossier_sha256 text NOT NULL CHECK (dossier_sha256 ~ '^[0-9a-f]{64}$'),
    dossier_content jsonb NOT NULL CHECK (
        jsonb_typeof(dossier_content) = 'object' AND octet_length(dossier_content::text) <= 4194304
    ),
    prompt_version text NOT NULL CHECK (prompt_version <> '' AND octet_length(prompt_version) <= 256),
    prompt_sha256 text NOT NULL CHECK (prompt_sha256 ~ '^[0-9a-f]{64}$'),
    prompt_content bytea NOT NULL CHECK (octet_length(prompt_content) > 0 AND octet_length(prompt_content) <= 4194304),
    response_schema_version text NOT NULL CHECK (
        response_schema_version <> '' AND octet_length(response_schema_version) <= 256
    ),
    response_schema_sha256 text NOT NULL CHECK (response_schema_sha256 ~ '^[0-9a-f]{64}$'),
    response_schema jsonb NOT NULL CHECK (
        jsonb_typeof(response_schema) = 'object' AND octet_length(response_schema::text) <= 4194304
    ),
    model_policy_version text NOT NULL CHECK (
        model_policy_version <> '' AND octet_length(model_policy_version) <= 256
    ),
    model_policy_sha256 text NOT NULL CHECK (model_policy_sha256 ~ '^[0-9a-f]{64}$'),
    model_policy jsonb NOT NULL CHECK (
        jsonb_typeof(model_policy) = 'object' AND octet_length(model_policy::text) <= 262144
    ),
    host_policy_version text NOT NULL CHECK (
        host_policy_version <> '' AND octet_length(host_policy_version) <= 256
    ),
    host_policy_sha256 text NOT NULL CHECK (host_policy_sha256 ~ '^[0-9a-f]{64}$'),
    host_policy jsonb NOT NULL CHECK (
        jsonb_typeof(host_policy) = 'object' AND octet_length(host_policy::text) <= 262144
    ),
    expected_request jsonb NOT NULL CHECK (
        jsonb_typeof(expected_request) = 'object' AND octet_length(expected_request::text) <= 4194304
    ),
    model_result jsonb NOT NULL CHECK (
        jsonb_typeof(model_result) = 'object' AND octet_length(model_result::text) <= 16777216
    ),
    raw_output bytea NOT NULL CHECK (octet_length(raw_output) > 0 AND octet_length(raw_output) <= 4194304),
    canonical_output jsonb NOT NULL CHECK (
        jsonb_typeof(canonical_output) = 'object' AND octet_length(canonical_output::text) <= 4194304
    ),
    created_at timestamptz NOT NULL,
    UNIQUE (plan_id, revision_number),
    UNIQUE (plan_id, candidate_sha256),
    UNIQUE (id, plan_id),
    FOREIGN KEY (plan_id, task_id) REFERENCES core.plans (id, task_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    FOREIGN KEY (run_id) REFERENCES core.runs (id),
    FOREIGN KEY (project_source_id) REFERENCES core.project_sources (id),
    FOREIGN KEY (supersedes_version_id, plan_id) REFERENCES core.plan_versions (id, plan_id),
    CHECK (
        (revision_number = 1 AND supersedes_version_id IS NULL)
        OR (revision_number > 1 AND supersedes_version_id IS NOT NULL)
    )
);

ALTER TABLE core.plans
    ADD CONSTRAINT plans_accepted_version_ownership
    FOREIGN KEY (accepted_version_id, id)
    REFERENCES core.plan_versions (id, plan_id);

CREATE TABLE core.plan_steps (
    plan_version_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    step_id text NOT NULL CHECK (step_id ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'),
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 64),
    status text NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed', 'skipped')),
    description text NOT NULL CHECK (description <> '' AND octet_length(description) <= 16384),
    criterion_ids jsonb NOT NULL CHECK (
        jsonb_typeof(criterion_ids) = 'array' AND jsonb_array_length(criterion_ids) > 0
        AND octet_length(criterion_ids::text) <= 65536
    ),
    depends_on_step_ids jsonb NOT NULL CHECK (
        jsonb_typeof(depends_on_step_ids) = 'array' AND octet_length(depends_on_step_ids::text) <= 65536
    ),
    expected_paths jsonb NOT NULL CHECK (
        jsonb_typeof(expected_paths) = 'array' AND jsonb_array_length(expected_paths) > 0
        AND octet_length(expected_paths::text) <= 65536
    ),
    components jsonb NOT NULL CHECK (
        jsonb_typeof(components) = 'array' AND jsonb_array_length(components) > 0
        AND octet_length(components::text) <= 65536
    ),
    test_strategy jsonb NOT NULL CHECK (
        jsonb_typeof(test_strategy) = 'array' AND jsonb_array_length(test_strategy) > 0
        AND octet_length(test_strategy::text) <= 262144
    ),
    risks jsonb NOT NULL CHECK (jsonb_typeof(risks) = 'array' AND octet_length(risks::text) <= 65536),
    assumptions jsonb NOT NULL CHECK (
        jsonb_typeof(assumptions) = 'array' AND octet_length(assumptions::text) <= 65536
    ),
    evidence_refs jsonb NOT NULL CHECK (
        jsonb_typeof(evidence_refs) = 'array' AND jsonb_array_length(evidence_refs) > 0
        AND octet_length(evidence_refs::text) <= 65536
    ),
    lineage jsonb,
    PRIMARY KEY (plan_version_id, step_id),
    UNIQUE (plan_version_id, ordinal),
    FOREIGN KEY (plan_version_id, plan_id) REFERENCES core.plan_versions (id, plan_id),
    CHECK (lineage IS NULL OR (jsonb_typeof(lineage) = 'object' AND octet_length(lineage::text) <= 65536))
);

-- +goose StatementBegin
CREATE FUNCTION core.reject_plan_version_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'plan versions are immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER plan_versions_no_update
BEFORE UPDATE ON core.plan_versions
FOR EACH ROW EXECUTE FUNCTION core.reject_plan_version_update();

-- +goose StatementBegin
CREATE FUNCTION core.validate_plan_step_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.plan_version_id IS DISTINCT FROM OLD.plan_version_id
       OR NEW.plan_id IS DISTINCT FROM OLD.plan_id
       OR NEW.step_id IS DISTINCT FROM OLD.step_id
       OR NEW.ordinal IS DISTINCT FROM OLD.ordinal
       OR NEW.description IS DISTINCT FROM OLD.description
       OR NEW.criterion_ids IS DISTINCT FROM OLD.criterion_ids
       OR NEW.depends_on_step_ids IS DISTINCT FROM OLD.depends_on_step_ids
       OR NEW.expected_paths IS DISTINCT FROM OLD.expected_paths
       OR NEW.components IS DISTINCT FROM OLD.components
       OR NEW.test_strategy IS DISTINCT FROM OLD.test_strategy
       OR NEW.risks IS DISTINCT FROM OLD.risks
       OR NEW.assumptions IS DISTINCT FROM OLD.assumptions
       OR NEW.evidence_refs IS DISTINCT FROM OLD.evidence_refs
       OR NEW.lineage IS DISTINCT FROM OLD.lineage THEN
        RAISE EXCEPTION 'plan step revision content is immutable';
    END IF;
    IF OLD.status IN ('completed', 'skipped') AND NEW.status <> OLD.status THEN
        RAISE EXCEPTION 'terminal plan steps cannot regress';
    END IF;
    IF OLD.status = 'in_progress' AND NEW.status = 'pending' THEN
        RAISE EXCEPTION 'in-progress plan steps cannot regress';
    END IF;
    IF OLD.status = 'pending' AND NEW.status NOT IN ('pending', 'in_progress', 'completed', 'skipped')
       OR OLD.status = 'in_progress' AND NEW.status NOT IN ('in_progress', 'completed', 'skipped') THEN
        RAISE EXCEPTION 'illegal plan step transition';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER plan_steps_monotonic_update
BEFORE UPDATE ON core.plan_steps
FOR EACH ROW EXECUTE FUNCTION core.validate_plan_step_update();

-- +goose Down
DROP FUNCTION IF EXISTS core.validate_plan_step_update() CASCADE;
DROP FUNCTION IF EXISTS core.reject_plan_version_update() CASCADE;
DROP TABLE core.plan_steps;
ALTER TABLE core.plans DROP CONSTRAINT plans_accepted_version_ownership;
DROP TABLE core.plan_versions;
DROP TABLE core.plans;
