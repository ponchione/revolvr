---
id: architecture-025-memory-graphiti-phase-gate
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-024-ui
---

# Decide whether a memory/Graphiti experiment is justified

## Sequence and status

- Sequence: `025` of `025`.
- Status: pending and phase-gated.
- Prerequisite: `architecture-024-ui` and all preceding core-loop tasks.
- Phase gate: do not make an adoption decision until substantial successful
  core-loop dogfooding, baseline retrieval metrics, and concrete multi-hop or
  temporal retrieval failures are present. If they are absent, the correct
  result is a documented deferral.

## Primary outcome

Produce an evidence-backed repository decision to defer, reject, or authorize
only a removable Graphiti dogfood experiment; this task does not implement or
adopt Graphiti.

## Required reading

- ADR-017 and ADR-018.
- Specification Sections 3.8, 9.22-9.23, 14, 18.1,
  27, 29 Phase 8 and Phase 11, 31, 32 Q10, 37.12,
  50-51, and NFR-008/NFR-014.

## Existing foundations to inspect

- Canonical PostgreSQL documents/entities/relations/provenance and the
  FTS/pgvector/structural retrieval lanes from task 021.
- Task 022 deterministic retrieval baselines plus real dogfood run, context,
  reactive-search, intervention, failure, and completion evidence.
- Accepted decisions, completion histories, aliases/supersession needs, and
  artifact storage growth; do not rely on model summaries without source
  provenance.

## Starting assumptions

- Relational relationships and hybrid retrieval remain the default sufficient
  solution.
- Graphiti/graph-database output can only ever be an optional rebuildable
  retrieval projection.
- Python is allowed behind a versioned interface only after measurable value is
  demonstrated; it never owns canonical state.

## Implementation requirements

- Define and evaluate the Phase 11 entry checklist with exact evidence links:
  substantial real run history, baseline retrieval metrics, and repeated clear
  entity-alias, temporal-supersession, or cross-document multi-hop failures.
- If any entry criterion is missing, write a durable deferral naming the missing
  evidence and the future re-evaluation trigger; do not speculate or prototype.
- If all gates pass, define a bounded A/B experiment comparing the existing
  retrieval lanes against a source-grounded graph projection on fixed judged
  queries, measuring quality, latency, resource/storage/operational cost, stale
  behavior, and provenance resolution.
- Record a decision of `defer`, `reject`, or `authorize_experiment` with exact
  evidence, thresholds, authority boundaries, failure/degraded behavior,
  removal path, and the separate follow-up task required before any code or
  dependency is added.
- Reaffirm that the system works without Graphiti and graph facts cannot mutate
  tasks, satisfy criteria, resolve findings, authorize actions, or complete
  work.

## Scope boundaries and non-goals

- Do not add Graphiti, Neo4j, FalkorDB, Python services, dependencies,
  containers, schema, adapters, projections, or runtime configuration here.
- Do not call lack of evidence a positive adoption result.
- Do not move canonical document/relation/task/run authority out of PostgreSQL
  or accept graph facts without resolvable source provenance.

## Acceptance criteria

- The decision record links every dogfood run/metric/failure used and explicitly
  says whether each entry criterion passed.
- An unmet gate deterministically produces `defer`; adoption is never presumed.
- If an experiment is authorized, it is separately bounded, removable,
  source-grounded, A/B measured, and still requires a new approved task before
  implementation.
- The diff contains decision/evidence documentation only and introduces no
  graph/Python dependency or runtime feature.
- Existing tests and `git diff --check` pass.

## Deterministic verification

```bash
go test ./...
git diff --check
git diff --name-only --diff-filter=ACMRTUXB | sort
git diff -- go.mod go.sum compose db internal cmd web
```

Manually resolve every cited dogfood artifact/hash and confirm the decision
record contains no uncited adoption claim. The final `git diff` command must be
empty for this task.

## Expected completion report

Report gate evidence and pass/fail for each entry criterion, the
defer/reject/authorize-experiment decision, measured failures/metrics,
authority/removal boundaries, changed documentation, verification results, and
confirmation that no Graphiti implementation or dependency was added.
