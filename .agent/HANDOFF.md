# Agent Handoff

Updated: 2026-08-06

## Where We Stopped

- Architecture 018 is complete in the current uncommitted working tree; no
  commit was created. Architecture-017 verification rows remain immutable,
  read-only evidence and its host-owned final-verification authority was not
  widened or rewritten.
- Reversible migration 00010, named sqlc queries, and regenerated PostgreSQL
  code add exact artifact provenance, claims/evidence links, completion
  records, attached artifacts, and completion claims. Ownership and SHA-256
  constraints, immutable update triggers, content-compared operation identity,
  and one atomic terminal transaction fail closed.
- `internal/evidence` supplies versioned provenance/claim/trajectory/harness
  contracts, content-addressed immutable bytes, divergence detection, and
  secret-sentinel scanning. Direct tools use explicit inactive trajectory and
  exact empty/inactive harness records; used inputs require complete exact
  version/hash/coverage/artifact authority.
- `internal/completion` supplies a read-only repeatable-read preflight, stable
  Section 18.7 rejection reasons, deterministic JSON/Markdown/manifest
  capsules, post-materialization revalidation, crash-safe byte reuse, and an
  atomic completion transaction. It binds accepted task/version, aggregate
  versions, source/diff/commit/tree, plan/criteria, fresh final verification,
  clean audit evidence, findings, budget, workspace/lease, prompt/model/image/
  profile, artifacts, operator inputs, trajectory, harness assets, and claims.
- The deterministic fixture hashes are preflight `1c708037...a47ceb66`, JSON
  `df699513...2b2c538a`, Markdown `65a8aba5...ed7405d`, and manifest
  `5890c055...e14f0db1`. Forced artifact and transaction interruption, stale
  second reads, rollback, exact retry/replay, divergent bytes/hashes, secret
  sentinels, and architecture-017 immutability are covered.
- Migration Down/Up, pinned sqlc generation, PostgreSQL integration tests,
  formatting, the complete Go suite, and diff validation pass. The temporary
  PostgreSQL container was removed. `go.mod` and `go.sum` are unchanged; no
  dependency was added.
- Architecture tasks 001-018 and PTC-001 are complete. Architecture 019 is the
  next and only task. Architectures 020-025 and all post-core PTC tasks remain
  gated.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-019-auditor-corrector.md`.

Read the durable state files and task-019 required ADR/specification sections.
Use architecture-017 verification and architecture-018 completion preflight as
read-only authority. Do not rerun architecture 018 or begin architecture 020.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md, and .agent/tasks/architecture-019-auditor-corrector.md. Confirm architecture 019 is the sole legally selectable pending task. Complete only architecture-019-auditor-corrector, preserve architecture-017 verification and architecture-018 completion authority, run its deterministic verification, update durable state, and stop.'
```

Graphiti remains deferred: architecture 025 is a decision-only gate after
successful core-loop dogfooding. Completion-gate evidence is not evidence
that Graphiti or the post-core programmatic workspace is needed.
