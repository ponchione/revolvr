---
id: architecture-007-task-schema-import
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-006-project-registration-managed-clone
---

# Add the canonical task schema and import transaction

## Sequence and status

- Sequence: `007` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-006-project-registration-managed-clone`.
- Phase gate: task intake binds to an existing registered project and the
  established artifact/event transaction primitives.

## Primary outcome

Import immutable task-source bytes and, when they already conform to the
canonical task contract, persist a validated draft task graph atomically.

## Required reading

- Specification Sections 9.3-9.5, 10.6, 11, 29 Phase 2, 33,
  37.2, 43.1-43.2, 44.1, 49, and NFR-006/NFR-011.
- ADR-016 and ADR-019.

## Existing foundations to inspect

- `internal/taskintake/contract.go`, `import.go`, and their tests.
- `internal/markdown` for fenced Markdown handling.
- `core.artifacts`, `core.events`, `core.projects`, and generated PostgreSQL
  queries.
- Legacy `internal/taskfile`, `internal/taskimport`, and `internal/taskmodel`
  only for useful parsing behavior; their local runtime state is not canonical
  for this architecture.

## Starting assumptions

- Imported source may be canonical `revolvr-task-v1` Markdown or prose that
  still needs compilation.
- Original bytes and SHA-256 are immutable authority.
- Operator approval and model-assisted compilation are later lifecycle/model
  operations, not import side effects.

## Implementation requirements

- Define and strictly validate the bounded task contract fields, mutation/risk
  classes, network profile, budgets, scope/exclusions, graph edges, and
  acceptance criteria from Sections 11 and 37.2.
- Store source bytes through a symlink-safe content-addressed artifact path.
- Persist import, draft task/version, graph edges, criteria/versions, artifact
  metadata, and event references in one transaction when the source is
  canonical.
- Persist noncanonical input as `needs_compilation` without fabricating a task.
- Reject malformed canonical input, unknown/duplicate/self/missing graph
  references, invalid acceptance, unsafe artifact paths, and changed replay
  authority without partial writes.

## Scope boundaries and non-goals

- Do not call a model, compile prose, approve tasks, schedule work, or implement
  lifecycle transitions.
- Do not make Markdown files the PostgreSQL runtime authority.
- Do not add arbitrary task-schema extensions or unsupported external
  requirements.

## Acceptance criteria

- Canonical imports produce immutable source, draft task/version, criteria,
  graph rows, and one consistent event set.
- Prose imports stop at `needs_compilation`.
- Identical replay is idempotent; conflicting replay fails closed.
- Malformed input, path traversal/symlink substitution, missing graph targets,
  and forced transaction failure leave no partial canonical state.
- PostgreSQL integration and full Go tests pass.

## Deterministic verification

```bash
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/taskintake
go test ./...
git diff --check
```

## Completed provenance

- Implementation commit:
  `bb8429ea72a9ae0df09064b23ecf9d93ea515eb0` (`Add PostgreSQL task intake`).

## Expected completion report

Report the commit, schema/query/package paths, canonical and prose import
outcomes, malformed/graph/path/replay/rollback tests, artifact hash identity,
and full-suite result.
