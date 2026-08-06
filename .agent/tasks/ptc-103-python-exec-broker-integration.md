---
id: ptc-103-python-exec-broker-integration
status: blocked
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-102-programmatic-workspace-runtime
---

# Integrate role-scoped python_exec through the broker

## Sequence and status

- Sequence: PTC-103 after PTC-102.
- Status: blocked and deferred.
- Phase gate: the pinned PTC-102 runtime must pass its isolation, identity,
  cancellation, and recovery gates before any worker receives `python_exec`.

## Primary outcome

Add `python_exec` to the existing closed tool broker for implementer and
corrector only, reusing architecture-016a lifecycle, evidence, replay, and
extension contracts.

## Required behavior

- Add one closed versioned request/result schema and role permission; all
  other roles and unimplemented modes reject it before effects.
- Bind each call to exact runtime generation, role, run/task/source/plan/
  workspace/sandbox/image/profile/context/trajectory identities and limits.
- Retain exact code/request/result hashes, trusted-host trajectory sequence,
  bounded inline content or immutable artifact references, created durable
  references, duration/resources, denial, truncation, timeout, cancellation,
  and runtime-loss evidence.
- Reconcile every claimed workspace effect against host-observed Git state
  through the same architecture-016/016a worker boundary.
- Preserve exact replay; never repeat an uncertain external effect or treat
  missing artifact evidence as successful execution.

## Authority and scope boundaries

- The Python process cannot call OpenAI, PostgreSQL, the container runtime,
  canonical-state APIs, or arbitrary network endpoints and receives no such
  credentials.
- Add no worker role, lifecycle/summary authority, package installation,
  skills, scratch catalog, refinement activation, or Graphiti work.
- Execution remains sequential and direct tools remain supported.

## Acceptance criteria

- Implementer/corrector fixtures execute bounded calls; supervisor, planner,
  auditor, verifier, and unknown-role calls are denied before runtime dispatch.
- Stale generation/identity, malformed code request, output limit, Python
  error, runtime loss, timeout, cancellation, artifact failure, and replay
  fixtures retain deterministic evidence and no false success.
- Host-observed source reconciliation catches unclaimed, protected,
  verification-authority, and mismatched effects exactly as direct tools do.
- Focused broker/programmatic/implementer/corrector and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
env -u OPENAI_API_KEY go test ./internal/tool ./internal/programmatic ./internal/implementer ./internal/correction
go test ./...
git diff --check
```

## Expected completion report

Report the closed schema and role matrix, identity/evidence fields, runtime
dispatch, replay/cancellation/artifact behavior, host source reconciliation,
unchanged canonical authority, and tests.
