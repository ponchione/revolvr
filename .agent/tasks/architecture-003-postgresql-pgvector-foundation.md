---
id: architecture-003-postgresql-pgvector-foundation
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-002-repository-build-baseline
---

# Stand up PostgreSQL with pgvector

## Sequence and status

- Sequence: `003` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-002-repository-build-baseline`.
- Phase gate: the Phase 0 repository baseline is established before the Phase
  1 database service is introduced.

## Objective

Complete architecture sequence item 003 from
`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`: provide a repeatable local
PostgreSQL development service with pgvector installed. This task establishes
only the container foundation and its operator commands. The following task
owns migrations, `sqlc`, and application database connectivity.

## Required reading

Before editing, read:

1. `AGENTS.md`, `README.md`, and
   `docs/architecture/repository-build-baseline.md`.
2. `docs/adr/004-postgresql-is-the-canonical-database.md`,
   `docs/adr/005-go-pgx-sqlc.md`, `docs/adr/006-sql-migrations.md`, and
   `docs/adr/007-pgvector-replaces-lancedb.md`.
3. Sections 3.2-3.3, 6.1-6.2, 7.3, 8, and 10.1; Phase 0 and Phase 1 in
   Section 29; Q5 in Section 32; and Section 33 of
   `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`.
4. The official PostgreSQL versioning policy and pgvector Docker image
   documentation linked below.

Treat the canonical specification and accepted ADRs as architecture authority.
For the specification's deferred major-version choice, this task pins
PostgreSQL 18, the current supported major at task authoring time. Use the
versioned `pgvector/pgvector:0.8.6-pg18-trixie` image; do not substitute a
moving `latest`, `pg18`, or unversioned pgvector tag.

- PostgreSQL versioning: <https://www.postgresql.org/support/versioning/>
- pgvector Docker images: <https://github.com/pgvector/pgvector#docker>

## Existing foundations to inspect

- `compose/compose.yaml` and `compose/compose.dev.yaml` when auditing the
  completed result.
- `README.md` PostgreSQL development-service commands.
- `go.mod` and the current CLI only to confirm this task did not introduce an
  application database layer.

## Starting assumptions

- PostgreSQL is the sole future canonical application database.
- The base service is private; loopback publication is development-only and
  explicit through the overlay.
- Credentials remain operator-supplied and are never committed.

## Scope

- Create `compose/compose.yaml` with one PostgreSQL service using
  `pgvector/pgvector:0.8.6-pg18-trixie`.
- Configure the service with:
  - development database and bootstrap user named `revolvr`,
  - a required `REVOLVR_POSTGRES_PASSWORD` environment input with no committed
    value or fallback,
  - a named persistent data volume,
  - a Compose-local internal network,
  - a `pg_isready` health check for the `revolvr` database and user,
  - no host-published port in the base file.
- Create `compose/compose.dev.yaml` as the explicit development overlay. It may
  publish PostgreSQL only on `127.0.0.1`, using
  `${REVOLVR_POSTGRES_PORT:-5432}` as the host port.
- Add concise README instructions for setting the development password,
  validating Compose configuration, starting and waiting for the service,
  checking health, stopping it without deleting data, and explicitly deleting
  the task-owned development volume when a fresh database is wanted.
- Verify from inside the running container that the server major version is
  18 and pgvector 0.8.6 is available. In a transaction that is rolled back,
  enable the `vector` extension, create a temporary vector column, insert and
  query a value, and confirm the smoke transaction leaves no durable schema.

## Boundaries

- Do not modify `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md` while implementing
  this task.
- Do not add `pgx`, `sqlc`, Goose, another migration tool, or any dependency.
- Do not add migrations, queries, generated code, application schemas,
  extension bootstrap SQL, transaction helpers, event tables, artifact tables,
  backup/restore automation, or a Go database connection layer. Those belong
  to later bounded tasks.
- Do not modify Go source, `go.mod`, `go.sum`, CI workflows, runtime-state
  files, or the existing SQLite, LanceDB, Shunter, provider, and orchestration
  code.
- Do not expose PostgreSQL on `0.0.0.0`, commit credentials, use trust
  authentication, mount the container-runtime socket, or provide database
  credentials to worker sandboxes.
- Do not add a wrapper script or Makefile for the documented Compose commands.
- Do not create any other Section 8 directory or empty future scaffolding.

## Acceptance criteria

- The base Compose file starts one healthy PostgreSQL 18 service with pgvector
  0.8.6 installed, a persistent named volume, an internal network, and no
  published host port.
- The development overlay publishes only a loopback port and permits an
  operator-selected host port without weakening the base file.
- Compose refuses configuration when `REVOLVR_POSTGRES_PASSWORD` is unset; no
  password or usable secret is committed.
- The exact base-plus-development command starts successfully from an empty
  task-specific Compose project and reports the service healthy.
- SQL checks prove PostgreSQL major version 18 and pgvector 0.8.6. The rolled-
  back vector smoke transaction succeeds and leaves no enabled extension,
  table, or other durable test schema.
- README commands match the implemented service, files, environment inputs,
  and data-retention behavior.
- `go test ./...` still passes unchanged.
- The diff contains only the two Compose files, README documentation, and the
  harness-owned metadata transition for this task.

## Verification

Run from the repository root with Docker Compose available:

```bash
if REVOLVR_POSTGRES_PASSWORD= docker compose \
  -f compose/compose.yaml -f compose/compose.dev.yaml config >/dev/null 2>&1
then
  echo "Compose unexpectedly accepted an empty database password" >&2
  exit 1
fi

export REVOLVR_POSTGRES_PASSWORD='revolvr-architecture-003-test-only'
export REVOLVR_POSTGRES_PORT=55432
project='revolvr-architecture-003'

docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml config --quiet
docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml up -d --wait postgres

test "$(docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml exec -T postgres \
  psql -U revolvr -d revolvr -Atc \
  "SELECT current_setting('server_version_num')::int / 10000")" = "18"
test "$(docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml exec -T postgres \
  psql -U revolvr -d revolvr -Atc \
  "SELECT default_version FROM pg_available_extensions WHERE name = 'vector'")" = \
  "0.8.6"

docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml exec -T postgres \
  psql -v ON_ERROR_STOP=1 -U revolvr -d revolvr <<'SQL'
BEGIN;
CREATE EXTENSION vector;
CREATE TEMP TABLE vector_smoke (embedding vector(3));
INSERT INTO vector_smoke VALUES ('[1,2,3]');
SELECT embedding FROM vector_smoke;
ROLLBACK;
SQL

test "$(docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml exec -T postgres \
  psql -U revolvr -d revolvr -Atc \
  "SELECT CASE WHEN NOT EXISTS (SELECT FROM pg_extension WHERE extname = 'vector') AND to_regclass('public.vector_smoke') IS NULL THEN 'rollback-ok' ELSE 'leaked' END")" = \
  "rollback-ok"

test "$(docker inspect -f '{{.State.Health.Status}}' \
  "$(docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml ps -q postgres)")" = \
  "healthy"

docker compose -p "$project" \
  -f compose/compose.yaml -f compose/compose.dev.yaml down --volumes
go test ./...
git diff --check
```

If verification stops before cleanup, rerun the exact `docker compose ... down
--volumes` command for project `revolvr-architecture-003`; do not delete an
unresolved or broader Docker volume target. Manually inspect the rendered base
configuration to confirm it has no published port, and the rendered overlay
configuration to confirm its only published address is `127.0.0.1`.

## Completed provenance

- Implementation commit:
  `67929b83acf92cbfc3019b20e5f79c53ed848ca4` (`Add PostgreSQL pgvector development service`).

## Expected completion report

Report the implementation commit, both Compose files, image and loopback
policy, password rejection, PostgreSQL/pgvector smoke results, rollback and
cleanup results, and unchanged Go tests.
