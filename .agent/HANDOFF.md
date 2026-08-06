# Agent Handoff

Updated: 2026-08-06

## Where We Stopped

- Architecture 019 is complete in the current working tree. Architecture-017
  verification occurrences remain immutable read-only evidence, and
  architecture-018 retains exclusive completion/finalization authority.
- Reversible migration 00011, named sqlc queries, and regenerated PostgreSQL
  code add immutable audit runs, findings/occurrences/dispositions, failure
  signatures, correction strategies/outcomes, exact ownership, transactional
  event provenance, report-hash validation, and replay-safe record hashes.
- `internal/audit` builds the hashed Section 13.4 dossier with actual bounded
  source and patch bytes; invokes one fresh tool-free independent auditor;
  strictly validates clean/changes-required/blocked output and exact cited
  findings; routes optional specialists deterministically; persists canonical
  evidence; and projects it into architecture-018 completion snapshots.
- `internal/correction` builds only exact failure/finding-scoped context,
  normalizes failure/strategy identities, rejects repeated/no-progress/budget/
  scope failures, enforces the shared corrector sandbox and tool registry, and
  requires fresh final verification plus a distinct clean re-audit. Exact
  multi-finding resolution is atomic.
- Unit, fake-model, sandbox, completion-gate, and PostgreSQL tests cover all
  required fixtures and typed stops, malformed/stale/forged evidence,
  rollback/retry/replay, immutable rows, atomic/concurrent dispositions, and
  successful correction evidence. Migration 00011 Down/Up and the complete
  deterministic verification sequence pass. No dependency was added.
- Architecture tasks 001-019 and PTC-001 are complete. Architecture 020 is the
  next and only dependency-satisfied pending task. Architectures 021-025 and
  every post-core PTC task remain gated.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-020-local-embedding-adapter.md`.

Read the durable state files and task-020 required ADR/specification sections.
Preserve architecture-017 verification, architecture-018 completion, and
architecture-019 audit/correction authority. Do not rerun architecture 019 or
begin architecture 021.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md, and .agent/tasks/architecture-020-local-embedding-adapter.md. Confirm architecture 020 is the sole legally selectable pending task. Complete only architecture-020-local-embedding-adapter, preserve architecture-017 verification, architecture-018 completion, and architecture-019 audit/correction authority, run its deterministic verification, update durable state, and stop.'
```

Graphiti remains deferred: architecture 025 is a decision-only gate after
successful core-loop dogfooding. Audit/correction completion is not evidence
that Graphiti or the post-core programmatic workspace is needed.
