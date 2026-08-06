# Agent Handoff

Updated: 2026-08-06

## Where We Stopped

- Architecture 020 is complete in the current working tree. Architecture-017
  verification occurrences remain immutable/read-only, architecture-018
  retains exclusive completion/finalization authority, and architecture-019
  retains independent audit/finding/correction authority.
- `internal/embedding` pins exact versioned model metadata and its embedding-
  space SHA-256, implements bounded document/query calls, verifies health and
  metadata before and after vector generation, validates exact count/order/
  dimension/finite values, and returns typed degraded/failed status without
  partial, fabricated, remote, stale, or mixed-space vectors.
- The optional Compose profile grants the dedicated service GPU access, one
  read-only model mount, a hardened read-only container, and only the internal
  control network. The dev override is loopback-only. The service has no
  project, database/OpenAI credential, or runtime-socket mount.
- `cmd/revolvr-embedding-smoke` checks operator-supplied exact metadata and
  vector dimensions without printing vectors. No evaluated image/model was
  supplied, so the live GPU smoke was explicitly omitted and no permanent
  model was selected. Fake endpoint, race, Compose render, focused prior-
  authority, and full deterministic tests pass. No dependency was added.
- Architecture tasks 001-020 and PTC-001 are complete. Architecture 021 is the
  next and only dependency-satisfied pending task. Architectures 022-025 and
  every post-core PTC task remain gated behind their existing chain.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-021-code-indexing-context-assembly.md`.

Read the durable state files and task-021 required ADR/specification sections.
Consume only validated architecture-020 embedding-space identity and degraded
status. Preserve architecture-017 verification, architecture-018 completion,
and architecture-019 audit/correction authority. Do not rerun architecture 020
or begin architecture 022.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md, and .agent/tasks/architecture-021-code-indexing-context-assembly.md. Confirm architecture 021 is the sole legally selectable pending task. Complete only architecture-021-code-indexing-context-assembly, consume only validated architecture-020 embedding-space identity and degraded status, preserve architecture-017 verification, architecture-018 completion, and architecture-019 audit/correction authority, run its deterministic verification, update durable state, and stop.'
```

Graphiti remains deferred: architecture 025 is a decision-only gate after
successful core-loop dogfooding. Audit/correction completion is not evidence
that Graphiti or the post-core programmatic workspace is needed.
