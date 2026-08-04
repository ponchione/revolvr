---
id: architecture-005-core-ids-artifacts-events-transactions
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-004-migration-sqlc-workflow
---

# Add core IDs, artifact metadata, events, and transactions

## Sequence and status

- Sequence: `005` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-004-migration-sqlc-workflow`.
- Phase gate: the Phase 1 migration/sqlc workflow is proven before canonical
  persistence primitives are added.

## Primary outcome

Establish UUIDv7 identities and the smallest PostgreSQL primitives for
content-addressed artifact metadata, append-only events, and atomic writes.

## Required reading

- ADR-005, ADR-016, and ADR-019.
- Specification Sections 3.2-3.3, 9.17, 10.2-10.6, 18.1, 29 Phase 1,
  43.3-43.4, 44, and NFR-002/NFR-006/NFR-011.

## Existing foundations to inspect

- `internal/id/id.go` and its tests.
- `db/migrations/00001_foundation.sql`, the sqlc configuration, and
  `internal/storage/postgres`.
- The legacy `internal/ledger` and `internal/receipt` packages only for useful
  behavior; SQLite/runtime-state authority is not ported into PostgreSQL.

## Starting assumptions

- Major canonical entities use UUIDv7 generated in Go.
- Artifact bytes remain filesystem-owned; this slice stores metadata only.
- Current state plus an append-only event table is sufficient; full event
  sourcing is explicitly excluded.

## Implementation requirements

- Provide a UUIDv7 generator used by new PostgreSQL entities.
- Add reversible `core.artifacts` and `core.events` schema with the specified
  identity, hash, size, provenance, payload, time, and aggregate-version
  constraints.
- Add named sqlc insert/read queries and regenerate checked-in code.
- Demonstrate a single pgx transaction that commits artifact metadata and its
  event together.
- Prove callback failure rolls both writes back and duplicate aggregate
  versions are rejected by PostgreSQL.

## Scope boundaries and non-goals

- Do not implement the filesystem artifact store, retention, claims, project
  registration, task state, or completion capsules.
- Do not introduce an application-wide repository abstraction or full event
  replay model.
- Do not use local time, random UUIDv4 for new major entities, or JSONB for
  query-critical identity/status columns.

## Acceptance criteria

- Generated IDs are UUIDv7.
- Artifact size and unique SHA-256 constraints and event aggregate-version
  uniqueness are database-enforced.
- Commit, rollback, and duplicate-event integration cases pass against
  PostgreSQL.
- sqlc regeneration and the complete Go suite are clean.

## Deterministic verification

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/storage/postgres -run TestArtifactAndEventTransaction
go test ./internal/id
go test ./...
git diff --check
```

## Completed provenance

- Implementation commit:
  `c9fd1e3b8f192be6a67cd1254cbbd96321da9069` (`Add core artifact and event persistence`).

## Expected completion report

Report the commit, migration/query/generated-code files, UUID version check,
transaction commit/rollback/uniqueness results, and full-suite result.
