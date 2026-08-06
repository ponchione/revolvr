---
id: architecture-013-openai-structured-output-client
status: complete
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-012-workspace-lifecycle
---

# Implement the OpenAI structured-output client

## Sequence and status

- Sequence: `013` of `025`.
- Status: complete.
- Prerequisite: `architecture-012-workspace-lifecycle`.
- Phase gate: Phase 4 model work begins only after admission, sandbox, and
  workspace identities can be pinned independently of model output.

## Primary outcome

Provide one trusted-control-plane OpenAI Responses API client with fresh
invocations, streaming diagnostics, versioned structured outputs, usage
evidence, and fail-closed refusal/error handling.

## Required reading

- ADR-008 through ADR-010 and ADR-023.
- Specification Sections 3.1, 6, 9.17, 13.6, 15,
  29 Phase 4, 37.6, 40.4, 42, 45-46, and 58.5.

## Existing foundations to inspect

- `internal/codexexec` and `internal/supervisor` only for existing bounded
  invocation, artifact, and schema-test behavior; Codex CLI sessions are not
  the required direct OpenAI API client.
- `internal/prompt`, `internal/outputcap`, `internal/redact`, artifact/event
  persistence, and scheduler/run identities.
- Existing dependencies in `go.mod` before proposing any new module.

## Starting assumptions

- OpenAI is the only remote provider implemented in v1.
- `OPENAI_API_KEY` remains in trusted process memory and is never serialized,
  logged, stored in a dossier, or passed to a sandbox.
- Each role call is fresh; durable context comes from pinned Revolvr records.

## Implementation requirements

- Implement the current supported Responses API request/stream lifecycle with
  context cancellation, bounded time/output, and no hidden session resume.
- Pin role model settings, reasoning effort, prompt version/hash, response JSON
  schema version/hash, task/run/source identity, and retry policy before the
  request.
- Accumulate streaming diagnostics without treating partial text as canonical;
  persist the exact final response/artifact and token/latency/cache/cost
  metadata available from the API.
- Validate the final structured value strictly and classify success, refusal,
  malformed/schema-invalid output, transport/service retry, timeout,
  cancellation, and exhausted retry separately.
- Retry only admitted transient transport/service failures; never retry a
  semantic refusal or invalid decision as though transport failed.
- Redact configured secrets from every request diagnostic, error, artifact,
  event, and test failure.

## Scope boundaries and non-goals

- Do not implement multiple providers, local reasoning models, role policy,
  supervisor decisions, tools, sandbox execution, or conversation persistence.
- Do not expose API credentials to workers or accept model names as domain
  lifecycle state.
- Do not add a dependency unless its necessity and bounded use are recorded in
  the completion report.

## Acceptance criteria

- A fake OpenAI server proves request identity, streaming accumulation,
  structured-output validation, usage recording, and fresh-call isolation.
- Refusal, malformed JSON, schema mismatch, stale identity, oversized stream,
  timeout, cancellation, retryable service error, nonretryable error, and retry
  exhaustion are distinct typed outcomes.
- Secret sentinel values never occur in recorded requests, logs, artifacts, or
  returned diagnostics.
- Tests require no live API key or network, and the full Go suite passes.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
env -u OPENAI_API_KEY go test ./internal/model
go test ./...
git diff --check
```

## Expected completion report

Report changed packages, API/response schema versions, pinned invocation
identity, retry classification, malformed/refusal/timeout/cancel coverage,
secret-redaction evidence, dependency decision, and test results.
