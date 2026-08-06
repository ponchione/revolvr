-- +goose Up
CREATE UNIQUE INDEX verification_checks_failure_ownership
    ON core.verification_checks (id, verification_run_id);

CREATE UNIQUE INDEX verification_runs_finding_ownership
    ON core.verification_runs (id, task_id);

CREATE TABLE core.audit_runs (
    id uuid PRIMARY KEY,
    operation_id text NOT NULL UNIQUE CHECK (
        operation_id <> '' AND octet_length(operation_id) <= 512
    ),
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    verification_run_id uuid NOT NULL,
    audit_kind text NOT NULL CHECK (audit_kind IN (
        'base', 'security', 'performance', 'integration', 'migration',
        'documentation', 'api_compatibility'
    )),
    disposition text NOT NULL CHECK (disposition IN ('clean', 'changes_required', 'blocked')),
    independent boolean NOT NULL CHECK (independent),
    auditor_invocation_id text NOT NULL CHECK (
        auditor_invocation_id <> '' AND octet_length(auditor_invocation_id) <= 512
    ),
    source_mutating_invocation_ids jsonb NOT NULL CHECK (
        jsonb_typeof(source_mutating_invocation_ids) = 'array'
        AND jsonb_array_length(source_mutating_invocation_ids) > 0
        AND octet_length(source_mutating_invocation_ids::text) <= 1048576
    ),
    dossier_schema_version text NOT NULL CHECK (
        dossier_schema_version <> '' AND octet_length(dossier_schema_version) <= 256
    ),
    dossier_sha256 text NOT NULL CHECK (dossier_sha256 ~ '^[0-9a-f]{64}$'),
    dossier jsonb NOT NULL CHECK (
        jsonb_typeof(dossier) = 'object' AND octet_length(dossier::text) <= 4194304
    ),
    prompt_version text NOT NULL CHECK (
        prompt_version <> '' AND octet_length(prompt_version) <= 256
    ),
    prompt_sha256 text NOT NULL CHECK (prompt_sha256 ~ '^[0-9a-f]{64}$'),
    prompt text NOT NULL CHECK (prompt <> '' AND octet_length(prompt) <= 4194304),
    response_schema_version text NOT NULL CHECK (
        response_schema_version <> '' AND octet_length(response_schema_version) <= 256
    ),
    response_schema_sha256 text NOT NULL CHECK (response_schema_sha256 ~ '^[0-9a-f]{64}$'),
    response_schema jsonb NOT NULL CHECK (
        jsonb_typeof(response_schema) = 'object'
        AND octet_length(response_schema::text) <= 1048576
    ),
    model text NOT NULL CHECK (model <> '' AND octet_length(model) <= 256),
    model_request jsonb NOT NULL CHECK (
        jsonb_typeof(model_request) = 'object' AND octet_length(model_request::text) <= 4194304
    ),
    model_result jsonb NOT NULL CHECK (
        jsonb_typeof(model_result) = 'object' AND octet_length(model_result::text) <= 4194304
    ),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    diff_sha256 text NOT NULL CHECK (diff_sha256 ~ '^[0-9a-f]{64}$'),
    report_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    report_sha256 text NOT NULL CHECK (report_sha256 ~ '^[0-9a-f]{64}$'),
    record_sha256 text NOT NULL CHECK (record_sha256 ~ '^[0-9a-f]{64}$'),
    started_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL CHECK (completed_at >= started_at),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (run_id, project_id, task_id, task_version_id)
        REFERENCES core.runs (id, project_id, task_id, task_version_id),
    FOREIGN KEY (workspace_id, run_id, project_id, task_id)
        REFERENCES core.workspaces (id, run_id, project_id, task_id),
    FOREIGN KEY (
        verification_run_id, run_id, project_id, task_id, task_version_id, workspace_id
    ) REFERENCES core.verification_runs (
        id, run_id, project_id, task_id, task_version_id, workspace_id
    ),
    UNIQUE (id, project_id, task_id, task_version_id, run_id, workspace_id),
    UNIQUE (id, task_id)
);

CREATE INDEX audit_runs_current_source
    ON core.audit_runs (task_id, source_commit, source_tree, completed_at DESC, id DESC);

CREATE TABLE core.audit_findings (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    introduced_audit_run_id uuid NOT NULL,
    finding_key text NOT NULL CHECK (finding_key ~ '^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$'),
    significance text NOT NULL CHECK (significance IN ('blocking', 'non_blocking')),
    summary text NOT NULL CHECK (summary <> '' AND octet_length(summary) <= 65536),
    required_correction text NOT NULL CHECK (
        required_correction <> '' AND octet_length(required_correction) <= 65536
    ),
    source_evidence jsonb NOT NULL CHECK (
        jsonb_typeof(source_evidence) = 'array' AND jsonb_array_length(source_evidence) > 0
        AND octet_length(source_evidence::text) <= 4194304
    ),
    affected_files jsonb NOT NULL CHECK (
        jsonb_typeof(affected_files) = 'array' AND jsonb_array_length(affected_files) > 0
        AND octet_length(affected_files::text) <= 1048576
    ),
    affected_symbols jsonb NOT NULL CHECK (
        jsonb_typeof(affected_symbols) = 'array'
        AND octet_length(affected_symbols::text) <= 1048576
    ),
    criterion_impact jsonb NOT NULL CHECK (
        jsonb_typeof(criterion_impact) = 'array' AND jsonb_array_length(criterion_impact) > 0
        AND octet_length(criterion_impact::text) <= 1048576
    ),
    definition_sha256 text NOT NULL CHECK (definition_sha256 ~ '^[0-9a-f]{64}$'),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (
        introduced_audit_run_id, project_id, task_id, task_version_id, run_id, workspace_id
    ) REFERENCES core.audit_runs (id, project_id, task_id, task_version_id, run_id, workspace_id),
    UNIQUE (task_id, finding_key),
    UNIQUE (id, task_id)
);

CREATE TABLE core.audit_finding_occurrences (
    id uuid PRIMARY KEY,
    audit_run_id uuid NOT NULL,
    task_id uuid NOT NULL,
    finding_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 256),
    occurrence_sha256 text NOT NULL CHECK (occurrence_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (audit_run_id, task_id) REFERENCES core.audit_runs (id, task_id),
    FOREIGN KEY (finding_id, task_id) REFERENCES core.audit_findings (id, task_id),
    UNIQUE (audit_run_id, ordinal),
    UNIQUE (audit_run_id, finding_id)
);

CREATE TABLE core.finding_dispositions (
    id uuid PRIMARY KEY,
    operation_id text NOT NULL UNIQUE CHECK (
        operation_id <> '' AND octet_length(operation_id) <= 512
    ),
    finding_id uuid NOT NULL,
    task_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('resolved', 'waived', 'rejected', 'superseded', 'stale')),
    authority_role text NOT NULL CHECK (authority_role IN ('host', 'operator', 'auditor')),
    authority_id text NOT NULL CHECK (authority_id <> '' AND octet_length(authority_id) <= 512),
    resolution_verification_run_id uuid REFERENCES core.verification_runs (id),
    resolution_audit_run_id uuid REFERENCES core.audit_runs (id),
    superseding_finding_id uuid,
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    evidence jsonb NOT NULL CHECK (
        jsonb_typeof(evidence) = 'array' AND jsonb_array_length(evidence) > 0
        AND octet_length(evidence::text) <= 4194304
    ),
    rationale text NOT NULL CHECK (octet_length(rationale) <= 65536),
    record_sha256 text NOT NULL CHECK (record_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (finding_id, task_id) REFERENCES core.audit_findings (id, task_id),
    FOREIGN KEY (superseding_finding_id, task_id) REFERENCES core.audit_findings (id, task_id),
    FOREIGN KEY (resolution_audit_run_id, task_id)
        REFERENCES core.audit_runs (id, task_id),
    FOREIGN KEY (resolution_verification_run_id, task_id)
        REFERENCES core.verification_runs (id, task_id),
    UNIQUE (finding_id),
    CHECK ((status = 'superseded') = (superseding_finding_id IS NOT NULL)),
    CHECK (superseding_finding_id IS NULL OR superseding_finding_id <> finding_id),
    CHECK (status NOT IN ('waived', 'rejected') OR authority_role = 'operator'),
    CHECK (status <> 'resolved' OR (
        resolution_verification_run_id IS NOT NULL AND resolution_audit_run_id IS NOT NULL
    )),
    CHECK (status <> 'stale' OR authority_role = 'host')
);

CREATE TABLE core.failure_signatures (
    id uuid PRIMARY KEY,
    operation_id text NOT NULL UNIQUE CHECK (
        operation_id <> '' AND octet_length(operation_id) <= 512
    ),
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    authority_kind text NOT NULL CHECK (authority_kind IN ('verification_failure', 'audit_findings')),
    verification_run_id uuid REFERENCES core.verification_runs (id),
    verification_check_id uuid REFERENCES core.verification_checks (id),
    audit_run_id uuid REFERENCES core.audit_runs (id),
    finding_keys jsonb NOT NULL CHECK (
        jsonb_typeof(finding_keys) = 'array' AND octet_length(finding_keys::text) <= 1048576
    ),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    normalized_material jsonb NOT NULL CHECK (
        jsonb_typeof(normalized_material) = 'object'
        AND octet_length(normalized_material::text) <= 4194304
    ),
    signature_sha256 text NOT NULL CHECK (signature_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (run_id, project_id, task_id, task_version_id)
        REFERENCES core.runs (id, project_id, task_id, task_version_id),
    FOREIGN KEY (workspace_id, run_id, project_id, task_id)
        REFERENCES core.workspaces (id, run_id, project_id, task_id),
    FOREIGN KEY (
        verification_run_id, run_id, project_id, task_id, task_version_id, workspace_id
    ) REFERENCES core.verification_runs (
        id, run_id, project_id, task_id, task_version_id, workspace_id
    ),
    FOREIGN KEY (verification_check_id, verification_run_id)
        REFERENCES core.verification_checks (id, verification_run_id),
    FOREIGN KEY (
        audit_run_id, project_id, task_id, task_version_id, run_id, workspace_id
    ) REFERENCES core.audit_runs (
        id, project_id, task_id, task_version_id, run_id, workspace_id
    ),
    CHECK (
        (authority_kind = 'verification_failure' AND verification_run_id IS NOT NULL
            AND verification_check_id IS NOT NULL AND audit_run_id IS NULL
            AND jsonb_array_length(finding_keys) = 0)
        OR
        (authority_kind = 'audit_findings' AND verification_run_id IS NULL
            AND verification_check_id IS NULL AND audit_run_id IS NOT NULL
            AND jsonb_array_length(finding_keys) > 0)
    ),
    UNIQUE (id, project_id, task_id, task_version_id, run_id, workspace_id)
);

CREATE INDEX failure_signatures_exact
    ON core.failure_signatures (task_id, signature_sha256, created_at DESC, id DESC);

CREATE TABLE core.strategies (
    id uuid PRIMARY KEY,
    operation_id text NOT NULL UNIQUE CHECK (
        operation_id <> '' AND octet_length(operation_id) <= 512
    ),
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    failure_signature_id uuid NOT NULL REFERENCES core.failure_signatures (id),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    dossier_sha256 text NOT NULL CHECK (dossier_sha256 ~ '^[0-9a-f]{64}$'),
    strategy_fingerprint text NOT NULL CHECK (strategy_fingerprint ~ '^[0-9a-f]{64}$'),
    normalized_strategy jsonb NOT NULL CHECK (
        jsonb_typeof(normalized_strategy) = 'object'
        AND octet_length(normalized_strategy::text) <= 4194304
    ),
    corrector_invocation_id text NOT NULL CHECK (
        corrector_invocation_id <> '' AND octet_length(corrector_invocation_id) <= 512
    ),
    sandbox_specification_sha256 text NOT NULL CHECK (
        sandbox_specification_sha256 ~ '^[0-9a-f]{64}$'
    ),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (run_id, project_id, task_id, task_version_id)
        REFERENCES core.runs (id, project_id, task_id, task_version_id),
    FOREIGN KEY (workspace_id, run_id, project_id, task_id)
        REFERENCES core.workspaces (id, run_id, project_id, task_id),
    FOREIGN KEY (
        failure_signature_id, project_id, task_id, task_version_id, run_id, workspace_id
    ) REFERENCES core.failure_signatures (
        id, project_id, task_id, task_version_id, run_id, workspace_id
    ),
    UNIQUE (id, task_id)
);

CREATE INDEX strategies_exact_fingerprint
    ON core.strategies (
        task_id, failure_signature_id, strategy_fingerprint, created_at DESC, id DESC
    );

CREATE TABLE core.strategy_outcomes (
    id uuid PRIMARY KEY,
    strategy_id uuid NOT NULL UNIQUE,
    task_id uuid NOT NULL,
    outcome text NOT NULL CHECK (outcome IN (
        'succeeded', 'failed', 'no_progress', 'cancelled', 'budget_exhausted', 'blocked'
    )),
    resulting_source_commit text CHECK (
        resulting_source_commit IS NULL OR resulting_source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'
    ),
    resulting_source_tree text CHECK (
        resulting_source_tree IS NULL OR resulting_source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'
    ),
    diff_sha256 text CHECK (diff_sha256 IS NULL OR diff_sha256 ~ '^[0-9a-f]{64}$'),
    verification_run_id uuid REFERENCES core.verification_runs (id),
    audit_run_id uuid REFERENCES core.audit_runs (id),
    evidence jsonb NOT NULL CHECK (
        jsonb_typeof(evidence) = 'array' AND octet_length(evidence::text) <= 4194304
    ),
    record_sha256 text NOT NULL CHECK (record_sha256 ~ '^[0-9a-f]{64}$'),
    completed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (strategy_id, task_id) REFERENCES core.strategies (id, task_id),
    FOREIGN KEY (verification_run_id, task_id)
        REFERENCES core.verification_runs (id, task_id),
    FOREIGN KEY (audit_run_id, task_id)
        REFERENCES core.audit_runs (id, task_id)
);

-- +goose StatementBegin
CREATE FUNCTION core.validate_audit_artifact_hash() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    found_hash text;
BEGIN
    SELECT sha256 INTO found_hash FROM core.artifacts WHERE id = NEW.report_artifact_id;
    IF found_hash IS NULL OR found_hash IS DISTINCT FROM NEW.report_sha256 THEN
        RAISE EXCEPTION 'audit report artifact hash does not match';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_runs_validate_report_hash
BEFORE INSERT ON core.audit_runs
FOR EACH ROW EXECUTE FUNCTION core.validate_audit_artifact_hash();

-- +goose StatementBegin
CREATE FUNCTION core.reject_audit_correction_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit and correction occurrence evidence is immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER audit_runs_no_update
BEFORE UPDATE ON core.audit_runs
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();
CREATE TRIGGER audit_findings_no_update
BEFORE UPDATE ON core.audit_findings
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();
CREATE TRIGGER audit_finding_occurrences_no_update
BEFORE UPDATE ON core.audit_finding_occurrences
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();
CREATE TRIGGER finding_dispositions_no_update
BEFORE UPDATE ON core.finding_dispositions
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();
CREATE TRIGGER failure_signatures_no_update
BEFORE UPDATE ON core.failure_signatures
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();
CREATE TRIGGER strategies_no_update
BEFORE UPDATE ON core.strategies
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();
CREATE TRIGGER strategy_outcomes_no_update
BEFORE UPDATE ON core.strategy_outcomes
FOR EACH ROW EXECUTE FUNCTION core.reject_audit_correction_update();

-- +goose Down
DROP FUNCTION IF EXISTS core.reject_audit_correction_update() CASCADE;
DROP FUNCTION IF EXISTS core.validate_audit_artifact_hash() CASCADE;
DROP TABLE core.strategy_outcomes;
DROP TABLE core.strategies;
DROP INDEX core.failure_signatures_exact;
DROP TABLE core.failure_signatures;
DROP TABLE core.finding_dispositions;
DROP TABLE core.audit_finding_occurrences;
DROP TABLE core.audit_findings;
DROP INDEX core.audit_runs_current_source;
DROP TABLE core.audit_runs;
DROP INDEX core.verification_runs_finding_ownership;
DROP INDEX core.verification_checks_failure_ownership;
