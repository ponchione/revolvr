---
id: architecture-016-tool-broker-implementer
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-015-planner
---

# Implement the tool broker and implementer

## Sequence and status

- Sequence: `016` of `025`.
- Status: completed.
- Prerequisite: `architecture-015-planner`.
- Phase gate: an implementer runs only for an accepted bounded plan step in an
  admitted managed workspace and validated sandbox.

## Primary outcome

Let one fresh implementer inspect and mutate only its admitted workspace
through host-validated tools, while recording exact tool and source-change
evidence.

## Required reading

- ADR-010, ADR-013, ADR-023, and ADR-024.
- Specification Sections 9.11, 12.3, 13.3, 15.3-15.4,
  16, 17, 18.4-18.5, 29 Phase 4, 37.6-37.7,
  44.3, 45, 52-54, and 58.5.

## Existing foundations to inspect

- Model client, supervisor/plan admission, sandbox runtime/validator, and
  managed workspace from tasks 010-015.
- `internal/runner`, `internal/outputcap`, `internal/pathguard`, artifact/event
  storage, and source-snapshot utilities.
- Existing `internal/autonomouscycle`, `internal/codexexec`, and
  `.agent/profiles/implementer.md` for useful iteration/receipt behavior only;
  do not give a worker host or PostgreSQL authority.

## Starting assumptions

- The host supplies one active plan step or explicitly bounded adjacent batch.
- Tool execution occurs only inside the validated sandbox/workspace.
- Implementer claims are advisory and are reconciled against host-observed Git
  state.

## Implementation requirements

- Define a closed, role-scoped tool registry for permitted file reads/search,
  source edits, and direct-argv commands; only mutation roles receive writes.
- Validate tool name/schema, workspace identity, normalized target path,
  working directory, environment names, network profile, timeout/resources,
  protected paths, and output caps before each dispatch.
- Run the fresh model/tool loop with bounded iterations and persist exact tool
  input/result, denial, timing, exit, stdout/stderr artifacts, and cancellation.
- Keep OpenAI/database/runtime credentials and raw container controls outside
  the worker; disable hooks and ambient host configuration by default.
- On final output, validate the structured summary/claims and compare claimed
  files against the host-captured workspace manifest/diff/source identity.
- Classify unexpected/protected changes, dependency/verification-authority
  changes, no-source-change, and partial/cancelled work for later policy;
  never advance criteria or completion directly.

## Scope boundaries and non-goals

- Do not implement verifier, auditor, corrector, completion, unrestricted
  shell/host tools, memory writes, or canonical-state mutation tools.
- Do not allow arbitrary mounts, directories outside the workspace, silent
  network escalation, secret access, or task/plan broadening.
- Do not add a general plugin system or provider abstraction.

## Acceptance criteria

- A deterministic fake model completes a bounded source edit using brokered
  tools and host-observed diff matches the recorded evidence.
- Unknown/malformed calls, traversal/symlink/protected paths, wrong cwd,
  runtime socket/secret access, disallowed network, excessive output/time, and
  stale run/workspace/plan identity are denied before effects.
- Tool/model cancellation kills sandbox work and preserves bounded partial
  evidence; replay does not repeat a proven external effect blindly.
- Claimed/actual file mismatch and test/verification-authority mutation are
  explicit policy signals, not silently accepted.
- Full tests pass without a live OpenAI call.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
env -u OPENAI_API_KEY go test ./internal/tool ./internal/implementer ./internal/sandbox ./internal/workspace
go test ./...
git diff --check
```

## Expected completion report

Report tool schemas and role permissions, sandbox/workspace boundary, evidence
captured, fake implementation result, every denial/cancel/replay category,
claimed/actual reconciliation, changed files, and test results.
