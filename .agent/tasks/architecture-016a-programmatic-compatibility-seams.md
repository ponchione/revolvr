---
id: architecture-016a-programmatic-compatibility-seams
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-016-tool-broker-implementer,ptc-001-amend-task-specifications
---

# Add programmatic-runtime compatibility seams to the direct-tool broker

## Sequence and status

- Sequence: `016a`, immediately after completed architecture 016 and before
  architecture 017.
- Status: completed.
- Prerequisites: `architecture-016-tool-broker-implementer` and
  `ptc-001-amend-task-specifications`.
- Phase gate: preserve the completed direct-tool implementation while making
  its contracts compatible with a future programmatic runtime. Do not
  implement or simulate that runtime.

## Primary outcome

Add narrow, closed compatibility seams so a later sandboxed `python_exec`
implementation can reuse the existing broker and worker lifecycle without
changing canonical lifecycle, implementer-summary, or worker contracts.

## Required reading

- ADR-010, ADR-011, ADR-013, ADR-016, ADR-019, ADR-023, and ADR-024.
- The completed architecture-016 task, implementation, tests, state, and
  handoff evidence.
- `REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md` Sections 2,
  7, 8.4-8.6, 10, 17, 19, 20, and 24 only as subordinate future-compatibility
  guidance.

## Existing foundations to inspect

- `internal/tool` contracts, registry, policy, broker, journal, replay, and
  focused tests.
- `internal/implementer` runtime, model-visible tool history, closed final
  summary, host observer, and reconciliation tests.
- Existing content-addressed artifact contracts and host-assigned event/order
  identities; do not create a second artifact or event authority.

## Implementation requirements

- Add a closed generic runtime-kind discriminator to execution evidence. Its
  only admitted initial value is `direct_tools_v1`; unknown or reserved future
  values fail closed and cannot be used to claim a completed execution.
- Represent each execution result as either bounded exact inline content or
  immutable content-addressed artifact references, with explicit media type,
  size, content hash, truncation, and resolution evidence as applicable.
- Retain explicit request and result hashes independent of whether result
  bytes are inline or artifact-backed.
- Add a trusted-host ordering/trajectory-sequence compatibility field or
  injection seam. The host owns ordering; model, sandbox, and container clocks
  do not. Do not implement the post-core trajectory manifest/query service.
- Preserve exact run, task/version, source commit/tree/revision, accepted
  plan/version/step batch, workspace, sandbox, image/profile, timeout,
  resource-policy, denial, truncation, replay, and cancellation evidence.
- Introduce one narrow internal runtime/tool-handler extension boundary so a
  later role-admitted `python_exec` handler can use broker validation,
  journaling, replay, output, and cancellation behavior without changing
  lifecycle, the implementer final-summary schema, or canonical worker
  contracts.
- Preserve the closed role registry exactly. Add no worker role, capability,
  tool name, permission, or runtime admission in this task.
- Keep exact terminal replay non-effecting and intent-only/ambiguous external
  effects non-repeatable across the new evidence forms.

## Scope boundaries and non-goals

- Do not add Python, `python_exec`, a REPL/kernel/service, process persistence,
  durable scratch, skills, refinement, a package, a database migration, or a
  network capability.
- Do not add provider/plugin abstractions, canonical trajectory storage, a new
  event store, model-authored sequence authority, or worker canonical-state
  authority.
- Do not alter plan, lifecycle, verification, completion, audit, or role
  semantics established by architectures 014-016.

## Acceptance criteria

- Closed-enum tests accept only the existing direct-tool runtime and reject
  unknown/reserved runtime kinds before dispatch or accepted replay.
- Deterministic tests cover bounded inline results, immutable artifact-backed
  results, exact request/result hashes, artifact/hash mismatch, truncation,
  denial, cancellation, and terminal/intent-only replay.
- Host-order injection tests prove deterministic ordering evidence and refusal
  of missing, duplicate, stale, or untrusted sequence authority without a
  trajectory service.
- A fake internal handler proves the extension boundary can produce the same
  normalized evidence while the public tool registry and implementer final
  summary remain byte/schema compatible and expose no new capability.
- Existing architecture-016 broker/implementer tests and the full Go suite
  pass without a live model, Python runtime, network, PostgreSQL migration, or
  new dependency.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
env -u OPENAI_API_KEY go test ./internal/tool ./internal/implementer
go test ./...
git diff --check
```

## Expected completion report

Report the runtime-kind contract, inline/artifact result forms, hashes,
ordering seam, preserved evidence fields, extension boundary, unchanged role
registry and worker contracts, denial/replay/cancellation coverage, files,
dependencies/migrations, and all test results.
