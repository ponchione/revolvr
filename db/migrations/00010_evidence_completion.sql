-- +goose Up
ALTER TABLE core.runs
    ADD CONSTRAINT runs_completion_ownership_unique
    UNIQUE (id, project_id, task_id, task_version_id);

ALTER TABLE core.workspaces
    ADD CONSTRAINT workspaces_completion_ownership_unique
    UNIQUE (id, run_id, project_id, task_id);

ALTER TABLE core.verification_runs
    ADD CONSTRAINT verification_runs_completion_ownership_unique
    UNIQUE (id, run_id, project_id, task_id, task_version_id, workspace_id);

CREATE TABLE core.artifact_provenance (
    id uuid PRIMARY KEY,
    artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    workspace_id uuid NOT NULL,
    producer_role text NOT NULL CHECK (
        producer_role IN ('host', 'supervisor', 'planner', 'implementer', 'verifier', 'auditor', 'corrector', 'operator')
    ),
    producing_operation_id text NOT NULL CHECK (
        producing_operation_id <> '' AND octet_length(producing_operation_id) <= 512
    ),
    source_commit text NOT NULL CHECK (source_commit ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    source_tree text NOT NULL CHECK (source_tree ~ '^[0-9a-f]{40}([0-9a-f]{24})?$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (run_id, project_id, task_id, task_version_id)
        REFERENCES core.runs (id, project_id, task_id, task_version_id),
    FOREIGN KEY (workspace_id, run_id, project_id, task_id)
        REFERENCES core.workspaces (id, run_id, project_id, task_id),
    UNIQUE (artifact_id, run_id, producing_operation_id, producer_role)
);

CREATE TABLE core.claims (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    criterion_id uuid,
    claim_key text NOT NULL CHECK (claim_key ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'),
    statement text NOT NULL CHECK (statement <> '' AND octet_length(statement) <= 65536),
    statement_sha256 text NOT NULL CHECK (statement_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (run_id, project_id, task_id, task_version_id)
        REFERENCES core.runs (id, project_id, task_id, task_version_id),
    FOREIGN KEY (criterion_id, task_id)
        REFERENCES core.task_acceptance_criteria (id, task_id),
    UNIQUE (run_id, claim_key),
    UNIQUE (id, project_id, task_id, task_version_id, run_id)
);

CREATE TABLE core.claim_evidence (
    claim_id uuid NOT NULL,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 256),
    evidence_kind text NOT NULL CHECK (evidence_kind IN ('artifact', 'verification_check')),
    artifact_id uuid REFERENCES core.artifacts (id),
    verification_check_id uuid REFERENCES core.verification_checks (id),
    evidence_sha256 text NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (claim_id, ordinal),
    FOREIGN KEY (claim_id, project_id, task_id, task_version_id, run_id)
        REFERENCES core.claims (id, project_id, task_id, task_version_id, run_id),
    CHECK (
        (evidence_kind = 'artifact' AND artifact_id IS NOT NULL AND verification_check_id IS NULL)
        OR (evidence_kind = 'verification_check' AND artifact_id IS NULL AND verification_check_id IS NOT NULL)
    )
);

-- +goose StatementBegin
CREATE FUNCTION core.validate_claim_evidence() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    found_hash text;
BEGIN
    IF NEW.evidence_kind = 'artifact' THEN
        SELECT a.sha256 INTO found_hash
        FROM core.artifacts a
        WHERE a.id = NEW.artifact_id
          AND (
              EXISTS (
                  SELECT 1 FROM core.artifact_provenance p
                  WHERE p.artifact_id = a.id
                    AND p.project_id = NEW.project_id
                    AND p.task_id = NEW.task_id
                    AND p.task_version_id = NEW.task_version_id
                    AND p.run_id = NEW.run_id
              )
              OR EXISTS (
                  SELECT 1
                  FROM core.verification_checks c
                  JOIN core.verification_runs v ON v.id = c.verification_run_id
                  WHERE (c.stdout_artifact_id = a.id OR c.stderr_artifact_id = a.id)
                    AND v.project_id = NEW.project_id
                    AND v.task_id = NEW.task_id
                    AND v.task_version_id = NEW.task_version_id
                    AND v.run_id = NEW.run_id
              )
          );
    ELSE
        SELECT c.execution_fingerprint INTO found_hash
        FROM core.verification_checks c
        JOIN core.verification_runs v ON v.id = c.verification_run_id
        WHERE c.id = NEW.verification_check_id
          AND v.project_id = NEW.project_id
          AND v.task_id = NEW.task_id
          AND v.task_version_id = NEW.task_version_id
          AND v.run_id = NEW.run_id;
    END IF;
    IF found_hash IS NULL OR found_hash IS DISTINCT FROM NEW.evidence_sha256 THEN
        RAISE EXCEPTION 'claim evidence ownership or hash does not match its claim';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER claim_evidence_validate
BEFORE INSERT ON core.claim_evidence
FOR EACH ROW EXECUTE FUNCTION core.validate_claim_evidence();

CREATE TABLE core.completions (
    id uuid PRIMARY KEY,
    operation_id text NOT NULL UNIQUE CHECK (
        operation_id <> '' AND octet_length(operation_id) <= 512
    ),
    project_id uuid NOT NULL,
    task_id uuid NOT NULL UNIQUE,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL UNIQUE,
    workspace_id uuid NOT NULL UNIQUE,
    verification_run_id uuid NOT NULL,
    preflight_sha256 text NOT NULL CHECK (preflight_sha256 ~ '^[0-9a-f]{64}$'),
    evidence_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    evidence_sha256 text NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
    markdown_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    markdown_sha256 text NOT NULL CHECK (markdown_sha256 ~ '^[0-9a-f]{64}$'),
    manifest_artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    manifest_sha256 text NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
    trajectory_envelope jsonb NOT NULL CHECK (
        jsonb_typeof(trajectory_envelope) = 'object'
        AND octet_length(trajectory_envelope::text) <= 4194304
    ),
    trajectory_sha256 text NOT NULL CHECK (trajectory_sha256 ~ '^[0-9a-f]{64}$'),
    harness_asset_set_manifest jsonb NOT NULL CHECK (
        jsonb_typeof(harness_asset_set_manifest) = 'object'
        AND octet_length(harness_asset_set_manifest::text) <= 4194304
    ),
    harness_asset_set_sha256 text NOT NULL CHECK (harness_asset_set_sha256 ~ '^[0-9a-f]{64}$'),
    completed_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    FOREIGN KEY (run_id, project_id, task_id, task_version_id)
        REFERENCES core.runs (id, project_id, task_id, task_version_id),
    FOREIGN KEY (workspace_id, run_id, project_id, task_id)
        REFERENCES core.workspaces (id, run_id, project_id, task_id),
    FOREIGN KEY (verification_run_id, run_id, project_id, task_id, task_version_id, workspace_id)
        REFERENCES core.verification_runs (id, run_id, project_id, task_id, task_version_id, workspace_id),
    UNIQUE (id, project_id, task_id, task_version_id, run_id)
);

CREATE TABLE core.completion_artifacts (
    completion_id uuid NOT NULL REFERENCES core.completions (id),
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 1024),
    artifact_id uuid NOT NULL REFERENCES core.artifacts (id),
    artifact_sha256 text NOT NULL CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
    artifact_role text NOT NULL CHECK (artifact_role IN (
        'evidence_json', 'human_markdown', 'manifest', 'supporting'
    )),
    PRIMARY KEY (completion_id, ordinal),
    UNIQUE (completion_id, artifact_role, artifact_id)
);

CREATE TABLE core.completion_claims (
    completion_id uuid NOT NULL,
    project_id uuid NOT NULL,
    task_id uuid NOT NULL,
    task_version_id uuid NOT NULL,
    run_id uuid NOT NULL,
    claim_id uuid NOT NULL,
    PRIMARY KEY (completion_id, claim_id),
    FOREIGN KEY (completion_id, project_id, task_id, task_version_id, run_id)
        REFERENCES core.completions (id, project_id, task_id, task_version_id, run_id),
    FOREIGN KEY (claim_id, project_id, task_id, task_version_id, run_id)
        REFERENCES core.claims (id, project_id, task_id, task_version_id, run_id)
);

-- +goose StatementBegin
CREATE FUNCTION core.validate_completion_artifact_hashes() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    found_hash text;
BEGIN
    IF TG_TABLE_NAME = 'completions' THEN
        SELECT sha256 INTO found_hash FROM core.artifacts WHERE id = NEW.evidence_artifact_id;
        IF found_hash IS DISTINCT FROM NEW.evidence_sha256 THEN
            RAISE EXCEPTION 'completion evidence artifact hash does not match';
        END IF;
        SELECT sha256 INTO found_hash FROM core.artifacts WHERE id = NEW.markdown_artifact_id;
        IF found_hash IS DISTINCT FROM NEW.markdown_sha256 THEN
            RAISE EXCEPTION 'completion markdown artifact hash does not match';
        END IF;
        SELECT sha256 INTO found_hash FROM core.artifacts WHERE id = NEW.manifest_artifact_id;
        IF found_hash IS DISTINCT FROM NEW.manifest_sha256 THEN
            RAISE EXCEPTION 'completion manifest artifact hash does not match';
        END IF;
    ELSE
        SELECT sha256 INTO found_hash FROM core.artifacts WHERE id = NEW.artifact_id;
        IF found_hash IS DISTINCT FROM NEW.artifact_sha256 THEN
            RAISE EXCEPTION 'attached completion artifact hash does not match';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER completions_validate_artifact_hashes
BEFORE INSERT ON core.completions
FOR EACH ROW EXECUTE FUNCTION core.validate_completion_artifact_hashes();

CREATE TRIGGER completion_artifacts_validate_hash
BEFORE INSERT ON core.completion_artifacts
FOR EACH ROW EXECUTE FUNCTION core.validate_completion_artifact_hashes();

-- +goose StatementBegin
CREATE FUNCTION core.reject_evidence_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'completion evidence records are immutable';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER artifact_provenance_no_update
BEFORE UPDATE ON core.artifact_provenance
FOR EACH ROW EXECUTE FUNCTION core.reject_evidence_update();

CREATE TRIGGER claims_no_update
BEFORE UPDATE ON core.claims
FOR EACH ROW EXECUTE FUNCTION core.reject_evidence_update();

CREATE TRIGGER claim_evidence_no_update
BEFORE UPDATE ON core.claim_evidence
FOR EACH ROW EXECUTE FUNCTION core.reject_evidence_update();

CREATE TRIGGER completions_no_update
BEFORE UPDATE ON core.completions
FOR EACH ROW EXECUTE FUNCTION core.reject_evidence_update();

CREATE TRIGGER completion_artifacts_no_update
BEFORE UPDATE ON core.completion_artifacts
FOR EACH ROW EXECUTE FUNCTION core.reject_evidence_update();

CREATE TRIGGER completion_claims_no_update
BEFORE UPDATE ON core.completion_claims
FOR EACH ROW EXECUTE FUNCTION core.reject_evidence_update();

-- +goose Down
DROP FUNCTION IF EXISTS core.reject_evidence_update() CASCADE;
DROP FUNCTION IF EXISTS core.validate_completion_artifact_hashes() CASCADE;
DROP TABLE core.completion_claims;
DROP TABLE core.completion_artifacts;
DROP TABLE core.completions;
DROP FUNCTION IF EXISTS core.validate_claim_evidence() CASCADE;
DROP TABLE core.claim_evidence;
DROP TABLE core.claims;
DROP TABLE core.artifact_provenance;
ALTER TABLE core.verification_runs DROP CONSTRAINT verification_runs_completion_ownership_unique;
ALTER TABLE core.workspaces DROP CONSTRAINT workspaces_completion_ownership_unique;
ALTER TABLE core.runs DROP CONSTRAINT runs_completion_ownership_unique;
