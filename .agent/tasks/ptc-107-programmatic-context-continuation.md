---
id: ptc-107-programmatic-context-continuation
status: superseded
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-104-durable-scratch-catalog
---

# Add externalized context querying, bounded continuation, and compaction provenance

## Supersession

- Superseded on 2026-08-27 by ADR-025. This task is terminal and not runnable.
- Existing provenance-bearing context assembly and direct tools remain the
  supported path; no custom programmatic continuation runtime is planned.

Everything below is retained as historical proposal context, not current
instruction.

## Sequence and status

- Sequence: PTC-107 after PTC-104 and the PTC-101 trajectory foundation.
- Status: blocked and deferred.
- Phase gate: trajectory queries, programmatic execution, and durable scratch
  must each be complete and retain exact authority before continuation is
  admitted.

## Primary outcome

Let implementer/corrector query externalized context, artifacts, trajectory
ranges, and scratch through a read-only sandbox client and continue across
bounded model turns without reinjecting full history.

## Required behavior

- Extend architecture-021's host query interface through the existing broker;
  preserve frozen context-package identities, exact external item/range hashes,
  omissions, bounds, and retrieval instructions.
- Continue model requests with immutable task/plan/policy/source authority,
  bounded recent turns, scratch-catalog summary, prior context reference, and
  the exact externally available trajectory range.
- Allow host-decided bounded compaction. Every compacted artifact binds its
  exact covered sequence range and entry hashes, summarizer model/prompt/schema
  identities when model-assisted, omissions/unresolved references, and output
  hash.
- A worker may request compaction but cannot choose canonical history,
  authority precedence, deletion, coverage, activation, or lifecycle effects.

## Authority and scope boundaries

- Exact task state, source files/symbols, host policy, verification, audit, and
  canonical evidence outrank scratch, summaries, compaction, and refinement.
- Full append-only trajectory remains recoverable. No continuation or
  compaction may rewrite/delete entries or become canonical source authority.
- Execution remains sequential; add no skills, refinement activation, runtime
  package installation, credentials, network, worker role, or Graphiti.

## Acceptance criteria

- Deterministic fixtures resolve inline/artifact/trajectory/scratch references
  under exact bounds and reject stale context/source/range/hash/role identity.
- Multi-turn fixtures retain immutable authority, bounded recent material, and
  explicit token/context omissions without relying on provider conversation
  state.
- Compaction fixtures prove exact coverage/provenance, full-history recovery,
  model-assisted identity binding, and refusal of gaps, overlap ambiguity,
  stale summaries, deletion, or authority inversion.
- Focused context/trajectory/programmatic/model and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
env -u OPENAI_API_KEY go test ./internal/context ./internal/trajectory ./internal/programmatic ./internal/model
go test ./...
git diff --check
```

## Expected completion report

Report query/client and continuation contracts, exact request context,
externalized material, compaction provenance/coverage, ranking/refusal cases,
bounded metrics, unchanged authority, and tests.
