# Agent Handoff

Updated: 2026-08-06

## Where We Stopped

- Architecture 017 is complete in the current uncommitted working tree; no
  commit was created. The committed architecture-016/016a broker, runtime
  handler, role registry, and implementer contracts were preserved.
- Reversible migration 00009, named sqlc queries, and regenerated PostgreSQL
  code now store immutable terminal verification-run/check occurrences,
  content-addressed stdout/stderr artifact references, exact original/reuse
  linkage and timestamps, and one atomic result event.
- `internal/verification` now contains the host-owned pinned Tier 0-4 engine,
  canonical material-input fingerprints, exact terminal reuse, forced-fresh
  Tier 4, configured structured parsers, baseline differential classification,
  PostgreSQL persistence, content-addressed artifacts, frozen-workspace
  authority observation, and the fresh sandbox adapter. The legacy verifier
  API remains available and passing.
- The verifier sandbox uses a unique identity, no network, exact image/profile/
  resources/argv/cwd/env, and a read-only frozen candidate mount. Implementer
  and corrector workspace semantics remain read-write. Sandbox artifacts are
  bounded and hash-rechecked before verification ingestion.
- Exact reuse admits only original `passed`/`failed` checks. A new linked
  occurrence is `passed_reused` or `unchanged_failure_reused`; the latter
  remains failed. Cancelled, incomplete, infrastructure-failed, ambiguous, and
  all other typed fail-closed results are not reusable. Completion-purpose
  Tier 4 is always fresh.
- Source/environment/authority changes, missing commands, timeouts,
  cancellation, malformed or truncated structured output, artifact failure,
  forged reuse, stale accepted verification authority, and transaction failure
  all fail closed. The engine cannot select checks, self-certify, waive
  failures, mark criteria complete, or mutate lifecycle authority.
- Migration Up/Down/Up, database-backed verification/sandbox tests, all
  required formatting/sqlc/full-suite/diff checks, and a repeat sqlc hash check
  pass. The temporary PostgreSQL container was removed. `go.mod` and `go.sum`
  are unchanged; no dependency was added.
- Architecture tasks 001-017 and PTC-001 are complete. Architecture 018 is the
  next and only task. Architectures 019-025 and all post-core PTC tasks remain
  gated.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-018-evidence-model-completion-gates.md`.

Read the durable state files and task-018 required ADR/specification sections.
Use the architecture-017 verification records as read-only completion
evidence. Do not rerun architecture 017 or begin architecture 019.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md, REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md, and .agent/tasks/architecture-018-evidence-model-completion-gates.md. Confirm architecture 018 is the sole legally selectable pending task. Complete only architecture-018-evidence-model-completion-gates, preserve architecture-017 verification authority, run its deterministic verification, update durable state, and stop.'
```

Graphiti remains deferred: architecture 025 is a decision-only gate after
successful core-loop dogfooding. Verification-engine evidence is not evidence
that Graphiti or the post-core programmatic workspace is needed.
