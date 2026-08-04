---
id: architecture-004-migration-sqlc-workflow
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-003-postgresql-pgvector-foundation
---

# Establish the migration and sqlc workflow

## Sequence and status

- Sequence: `004` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-003-postgresql-pgvector-foundation`.
- Phase gate: the PostgreSQL 18/pgvector service is healthy before Phase 1
  schema or generated query code is introduced.

## Primary outcome

Provide the SQL-first schema migration, `sqlc`, `pgx`, and integration-test
workflow that every later PostgreSQL task reuses.

## Required reading

- `AGENTS.md`, `README.md`, `go.mod`, and the repository build baseline.
- ADR-004 through ADR-007 and ADR-019.
- Specification Sections 3.2-3.3, 10.1-10.5, 29 Phase 1, 43, 44, and 58.2.

## Existing foundations to inspect

- `compose/compose.yaml` and `compose/compose.dev.yaml`.
- `db/migrations/00001_foundation.sql`, `db/queries/health.sql`, and
  `db/sqlc.yaml`.
- `internal/storage/postgres/` and the PostgreSQL operator commands in
  `README.md`.

## Starting assumptions

- PostgreSQL is the only canonical database; no SQLite compatibility layer is
  required.
- Goose `v3.23.1` and sqlc `v1.27.0` are pinned workflow tools.
- Generated query code is checked in and must be reproducible.

## Implementation requirements

- Add an ordered Goose migration that enables `vector` and creates the
  `core`, `retrieval`, and `telemetry` schemas, with a valid reverse migration.
- Configure sqlc for PostgreSQL/pgx and generate the health query package under
  `internal/storage/postgres`.
- Provide a bounded pool-opening and health-check path using `pgx`.
- Document exact migrate, rollback, generation, and integration-test commands.
- Test a real database connection, schema/extension presence, and migration
  round trips against the Compose service.

## Scope boundaries and non-goals

- Do not add domain tables beyond the foundation schemas and extension.
- Do not add artifacts, events, projects, tasks, schedulers, or lifecycle
  behavior; later numbered tasks own them.
- Do not add an ORM, a second database backend, an unpinned generator, or
  wrapper scripts around the documented commands.
- Do not place database credentials in source or worker environments.

## Acceptance criteria

- Empty-database migration succeeds and the reverse/forward round trip is
  valid.
- `vector`, `core`, `retrieval`, and `telemetry` exist after migration.
- sqlc generation is repeatable with no unexplained generated diff.
- The pgx health integration test succeeds with `REVOLVR_TEST_DATABASE_URL`.
- The full Go suite passes.

## Deterministic verification

```bash
export REVOLVR_POSTGRES_PASSWORD='revolvr-architecture-004-test-only'
export REVOLVR_DATABASE_URL='postgres://revolvr@127.0.0.1:5432/revolvr?sslmode=disable'
docker compose -f compose/compose.yaml -f compose/compose.dev.yaml up -d --wait postgres
PGPASSWORD="$REVOLVR_POSTGRES_PASSWORD" go run github.com/pressly/goose/v3/cmd/goose@v3.23.1 -dir db/migrations postgres "$REVOLVR_DATABASE_URL" up
PGPASSWORD="$REVOLVR_POSTGRES_PASSWORD" go run github.com/pressly/goose/v3/cmd/goose@v3.23.1 -dir db/migrations postgres "$REVOLVR_DATABASE_URL" down
PGPASSWORD="$REVOLVR_POSTGRES_PASSWORD" go run github.com/pressly/goose/v3/cmd/goose@v3.23.1 -dir db/migrations postgres "$REVOLVR_DATABASE_URL" up
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/storage/postgres
go test ./...
git diff --check
```

## Completed provenance

- Implementation commit:
  `997620b14ad9213503f5818b3fac1f465379d0dd` (`Add PostgreSQL migration and sqlc workflow`).

## Expected completion report

Report the commit, migration and generated-code paths, pinned tool versions,
migration round-trip, sqlc reproducibility, database integration result, and
full-suite result.
