# Agent Handoff

Updated: 2026-08-06

## Where We Stopped

- Architecture 016a is complete in the current uncommitted working tree; no
  commit was created. The prior architecture-016 broker/implementer
  implementation and PTC-001 task-sequence changes remain uncommitted and were
  preserved.
- `internal/tool` now records the sole admitted runtime kind
  `direct_tools_v1`, trusted host-assigned occurrence sequences, explicit
  request/result hashes, and exclusive bounded-inline or immutable artifact-
  backed result evidence with media type, size, truncation, and resolution.
- The host-injected sequence seam refuses missing, duplicate, stale, foreign,
  and untrusted grants. Exact terminal replay is non-effecting and links a new
  occurrence sequence to the original execution; intent-only ambiguous effects
  remain non-repeatable.
- The narrow `RuntimeHandler` receives only already-validated broker authority,
  sandbox, operation, sequence, request hash, and policy identity. The existing
  direct sandbox executor is the only installed handler. Broker validation,
  journal/replay, output caps, cancellation, effect proof, and evidence
  normalization remain host-owned.
- Existing run, task/version, source, accepted plan/step batch, workspace,
  sandbox, image/profile/network/resources, policy, denial, truncation, replay,
  and cancellation evidence remains present. Model-visible history strips host
  artifact paths.
- The closed four-tool role registry and implementer final-summary schema are
  byte-compatible. No role, tool, capability, lifecycle authority, canonical
  worker contract, dependency, migration, Python/runtime, scratch, skill,
  refinement, network capability, or Graphiti work was added.
- Required formatting, focused owner tests, the full Go suite, and diff checks
  pass. `go.mod`, `go.sum`, and migrations `00001` through `00008` are
  unchanged.
- Architecture tasks 001-016a and PTC-001 are complete. Architecture 017 is
  the next and only task; it has not begun. Architectures 018-025 and the
  post-core PTC sequence remain gated.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-017-verification-engine.md`.

Read the durable state files, the task-017 required ADR/specification sections,
and the completed source/evidence foundations. Do not rerun architecture 016a
or begin architecture 018.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md, and .agent/tasks/architecture-017-verification-engine.md. Complete only architecture-017-verification-engine, run its verification, update durable state, and stop.'
```

Graphiti remains deferred: architecture 025 is a decision-only gate after
successful core-loop dogfooding. Programmatic-workspace or continual-harness
evidence is not evidence that Graphiti is needed.
