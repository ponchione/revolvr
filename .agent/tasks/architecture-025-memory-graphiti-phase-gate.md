---
id: architecture-025-memory-graphiti-phase-gate
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-024-ui
---

# Decide whether the existing brain has an evidenced gap

## Sequence and status

- Sequence: `025` of `025`.
- Status: completed at `simplify` with decision `defer`.
- Prerequisite: completed `architecture-024-ui`.
- This is a documentation-only decision task, not an implementation or
  prototype task.

## Primary outcome

Review real TUI/core-loop usage and decide whether Revolvr's existing brain—
durable project knowledge, typed relationships, exact/lexical/vector
retrieval, prior evidence, and provenance-bearing context assembly—is
insufficient in one concrete, repeated way.

## Required evidence

- Substantial real task/run history using the existing brain.
- Baseline retrieval/context metrics from the existing lanes.
- Repeated source-linked failures involving entity aliases, temporal
  supersession, or cross-document multi-hop retrieval.
- Evidence that a smaller fix to existing relational or retrieval behavior is
  insufficient.

## Decision rule

- If any required evidence is absent, record `defer` and name the missing
  evidence and future re-evaluation trigger.
- If the evidence exists, record the exact gap and authorize at most a separate
  small comparison prototype. Do not plan or implement Graphiti in this task.
- Graphiti remains optional, derived, removable, and subordinate to canonical
  Go/PostgreSQL ledger and artifact authority.

## Scope boundaries and non-goals

- Do not add Graphiti, Neo4j, FalkorDB, Python services, dependencies,
  containers, schemas, adapters, or runtime configuration.
- Do not infer a graph need from architectural completeness, the retired PTC
  chain, or the existence of compatibility seams.
- Do not let a brain or graph projection mutate tasks, satisfy criteria,
  resolve findings, authorize actions, or complete work.

## Acceptance criteria

- Every claimed gap resolves to exact usage, query, source, and outcome
  evidence.
- Missing or inconclusive evidence deterministically produces `defer`.
- Any authorized comparison is a separate bounded task and keeps the existing
  brain as baseline and canonical authority unchanged.
- The diff contains decision/evidence documentation only.

## Completed result

- Decision: **defer** Graphiti; no comparison prototype is authorized.
- Gate results: real current-brain usage history absent; baseline retrieval and
  context metrics supported; repeated source-linked alias, temporal-
  supersession, or cross-document multi-hop failures absent; smaller-fix
  insufficiency evidence absent.
- Evidence and the exact re-evaluation trigger are recorded in
  `docs/architecture/memory-graphiti-phase-gate.md`.
- The simplify phase consolidated duplicate gate narration without changing
  the evidence, trigger, decision, or authority boundaries.
- Canonical Go/PostgreSQL ledger and artifact authority is unchanged. No graph
  or Python implementation, dependency, service, schema, adapter, container,
  or runtime configuration was added.

## Deterministic verification

```bash
go test ./...
git diff --check
git diff --name-only
```

## Expected completion report

Report the evidence reviewed, each gate result, the defer or bounded-prototype
decision, the re-evaluation trigger, and confirmation that no graph/Python
implementation or dependency was added.
