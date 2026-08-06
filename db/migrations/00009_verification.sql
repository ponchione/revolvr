-- +goose Up
CREATE TABLE core.verification_runs (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL REFERENCES core.runs (id),
    workspace_id uuid NOT NULL REFERENCES core.workspaces (id),
    purpose text NOT NULL CHECK (purpose IN ('baseline', 'candidate', 'final')),
    status text NOT NULL CHECK (status IN (
        'passed', 'failed', 'cancelled', 'incomplete', 'infrastructure_failed', 'ambiguous'
    )),
    plan_schema_version text NOT NULL CHECK (
        plan_schema_version <> '' AND octet_length(plan_schema_version) <= 256
    ),
    plan_version text NOT NULL CHECK (plan_version <> '' AND octet_length(plan_version) <= 256),
    plan_sha256 text NOT NULL CHECK (plan_sha256 ~ '^[0-9a-f]{64}$'),
    pinned_plan jsonb NOT NULL CHECK (
        jsonb_typeof(pinned_plan) = 'object' AND octet_length(pinned_plan::text) <= 4194304
    ),
    candidate_commit text NOT NULL CHECK (candidate_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    candidate_tree text NOT NULL CHECK (candidate_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    project_environment_sha256 text NOT NULL CHECK (project_environment_sha256 ~ '^[0-9a-f]{64}$'),
    project_environment jsonb NOT NULL CHECK (
        jsonb_typeof(project_environment) = 'object'
        AND octet_length(project_environment::text) <= 1048576
    ),
    differential jsonb NOT NULL CHECK (
        jsonb_typeof(differential) = 'object' AND octet_length(differential::text) <= 4194304
    ),
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL CHECK (completed_at >= started_at),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (task_id, project_id) REFERENCES core.tasks (id, project_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES core.task_versions (id, task_id),
    UNIQUE (id, run_id)
);

CREATE TABLE core.verification_checks (
    id uuid PRIMARY KEY,
    verification_run_id uuid NOT NULL REFERENCES core.verification_runs (id),
    run_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 256),
    gate_id text NOT NULL CHECK (gate_id ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'),
    tier smallint NOT NULL CHECK (tier >= 0 AND tier <= 4),
    outcome text NOT NULL CHECK (outcome IN (
        'passed', 'failed', 'passed_reused', 'unchanged_failure_reused',
        'timed_out', 'cancelled', 'incomplete', 'infrastructure_failed',
        'ambiguous', 'malformed_output', 'artifact_failed', 'stale_source',
        'stale_environment', 'missing_command', 'authority_tampered'
    )),
    execution_fingerprint text NOT NULL CHECK (execution_fingerprint ~ '^[0-9a-f]{64}$'),
    verifier_protocol_version text NOT NULL CHECK (
        verifier_protocol_version <> '' AND octet_length(verifier_protocol_version) <= 256
    ),
    verifier_implementation_version text NOT NULL CHECK (
        verifier_implementation_version <> '' AND octet_length(verifier_implementation_version) <= 256
    ),
    parser_kind text NOT NULL CHECK (parser_kind IN ('none', 'go_test_json', 'json', 'junit_xml')),
    parser_version text NOT NULL CHECK (parser_version <> '' AND octet_length(parser_version) <= 256),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    command_argv jsonb NOT NULL CHECK (
        jsonb_typeof(command_argv) = 'array' AND jsonb_array_length(command_argv) > 0
        AND octet_length(command_argv::text) <= 262144
    ),
    working_directory text NOT NULL CHECK (
        working_directory ~ '^/workspace(/[^/]+)*$' AND octet_length(working_directory) <= 4096
    ),
    environment jsonb NOT NULL CHECK (
        jsonb_typeof(environment) = 'array' AND octet_length(environment::text) <= 1048576
    ),
    image_reference text NOT NULL CHECK (image_reference <> '' AND octet_length(image_reference) <= 512),
    image_digest text NOT NULL CHECK (image_digest ~ '^sha256:[0-9a-f]{64}$'),
    sandbox_profile text NOT NULL CHECK (sandbox_profile IN ('strict', 'compatible')),
    sandbox_profile_sha256 text NOT NULL CHECK (sandbox_profile_sha256 ~ '^[0-9a-f]{64}$'),
    sandbox_specification_sha256 text NOT NULL CHECK (sandbox_specification_sha256 ~ '^[0-9a-f]{64}$'),
    authority_inputs jsonb NOT NULL CHECK (
        jsonb_typeof(authority_inputs) = 'array' AND octet_length(authority_inputs::text) <= 4194304
    ),
    output_policy jsonb NOT NULL CHECK (
        jsonb_typeof(output_policy) = 'object' AND octet_length(output_policy::text) <= 262144
    ),
    exit_code integer,
    timed_out boolean NOT NULL,
    cancelled boolean NOT NULL,
    stdout_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    stderr_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    parsed_result jsonb NOT NULL CHECK (
        jsonb_typeof(parsed_result) IN ('object', 'array')
        AND octet_length(parsed_result::text) <= 4194304
    ),
    sandbox_evidence jsonb NOT NULL CHECK (
        jsonb_typeof(sandbox_evidence) = 'object'
        AND octet_length(sandbox_evidence::text) <= 4194304
    ),
    failure_signatures jsonb NOT NULL CHECK (
        jsonb_typeof(failure_signatures) = 'array'
        AND octet_length(failure_signatures::text) <= 1048576
    ),
    reused_from_check_id uuid REFERENCES core.verification_checks (id),
    original_executed_at timestamptz NOT NULL,
    occurred_at timestamptz NOT NULL,
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL CHECK (completed_at >= started_at),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (verification_run_id, run_id) REFERENCES core.verification_runs (id, run_id),
    UNIQUE (verification_run_id, ordinal),
    UNIQUE (verification_run_id, gate_id),
    CHECK (
        (outcome IN ('passed_reused', 'unchanged_failure_reused')
            AND reused_from_check_id IS NOT NULL
            AND original_executed_at <= occurred_at)
        OR (outcome NOT IN ('passed_reused', 'unchanged_failure_reused')
            AND reused_from_check_id IS NULL
            AND original_executed_at = occurred_at)
    ),
    CHECK ((outcome = 'timed_out' AND timed_out) OR (outcome <> 'timed_out' AND NOT timed_out)),
    CHECK ((outcome = 'cancelled' AND cancelled) OR (outcome <> 'cancelled' AND NOT cancelled)),
    CHECK (outcome NOT IN ('passed', 'failed', 'passed_reused', 'unchanged_failure_reused') OR exit_code IS NOT NULL)
);

CREATE INDEX verification_checks_exact_reuse
    ON core.verification_checks (execution_fingerprint, completed_at DESC, id DESC)
    WHERE reused_from_check_id IS NULL AND outcome IN ('passed', 'failed');

-- +goose StatementBegin
CREATE FUNCTION core.validate_verification_check_reuse() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    original core.verification_checks%ROWTYPE;
BEGIN
    IF NEW.reused_from_check_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT * INTO original
    FROM core.verification_checks
    WHERE id = NEW.reused_from_check_id
      AND reused_from_check_id IS NULL
      AND outcome IN ('passed', 'failed');
    IF NOT FOUND THEN
        RAISE EXCEPTION 'verification reuse must reference a reusable original execution';
    END IF;
    IF NEW.execution_fingerprint IS DISTINCT FROM original.execution_fingerprint
       OR NEW.original_executed_at IS DISTINCT FROM original.original_executed_at
       OR NEW.stdout_artifact_id IS DISTINCT FROM original.stdout_artifact_id
       OR NEW.stderr_artifact_id IS DISTINCT FROM original.stderr_artifact_id
       OR NEW.exit_code IS DISTINCT FROM original.exit_code
       OR NEW.timed_out IS DISTINCT FROM original.timed_out
       OR NEW.cancelled IS DISTINCT FROM original.cancelled
       OR NEW.parsed_result IS DISTINCT FROM original.parsed_result
       OR NEW.sandbox_evidence IS DISTINCT FROM original.sandbox_evidence
       OR NEW.failure_signatures IS DISTINCT FROM original.failure_signatures
       OR (original.outcome = 'passed' AND NEW.outcome <> 'passed_reused')
       OR (original.outcome = 'failed' AND NEW.outcome <> 'unchanged_failure_reused') THEN
        RAISE EXCEPTION 'verification reuse does not exactly preserve its original execution';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER verification_checks_validate_reuse
BEFORE INSERT ON core.verification_checks
FOR EACH ROW EXECUTE FUNCTION core.validate_verification_check_reuse();

-- +goose StatementBegin
CREATE FUNCTION core.reject_verification_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'verification occurrences are immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER verification_runs_no_update
BEFORE UPDATE ON core.verification_runs
FOR EACH ROW EXECUTE FUNCTION core.reject_verification_update();

CREATE TRIGGER verification_checks_no_update
BEFORE UPDATE ON core.verification_checks
FOR EACH ROW EXECUTE FUNCTION core.reject_verification_update();

-- +goose Down
DROP FUNCTION IF EXISTS core.reject_verification_update() CASCADE;
DROP FUNCTION IF EXISTS core.validate_verification_check_reuse() CASCADE;
DROP INDEX core.verification_checks_exact_reuse;
DROP TABLE core.verification_checks;
DROP TABLE core.verification_runs;
