---
id: architecture-024-ui
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-023-sequential-queue
---

# Add the desktop operator UI

## Sequence and status

- Sequence: `024` of `025`.
- Status: pending.
- Prerequisite: `architecture-023-sequential-queue`.
- Phase gate: the CLI-first core loop and bounded queue must be trustworthy and
  fully operable before Phase 9 desktop views are added.

## Primary outcome

Add a Wails/Vue 3/TypeScript desktop operator surface over the same canonical
application services, with no lifecycle or completion truth invented in the
frontend.

## Required reading

- ADR-020 and ADR-021.
- Specification Sections 2.4, 9.24, 22, 24-25,
  29 Phase 9, 37.14, 38, 41, and 58.3.

## Existing foundations to inspect

- `cmd/revolvr`, `internal/app`, and CLI query/command behavior for the
  canonical service boundary.
- Existing `internal/tui` views/tests for useful operator terminology and
  keyboard behavior only; do not duplicate its inferred/local state.
- PostgreSQL task/run/evidence/context/audit/queue projections and event IDs
  implemented by prior tasks.

## Starting assumptions

- CLI operation remains complete and authoritative if the desktop UI is absent.
- The desktop app is local-only, single-user, and reads commands/queries from
  the trusted Go application layer.
- REST commands/queries and resumable SSE are preferred unless Wails bindings
  already provide the same narrow local boundary without unique business logic.

## Implementation requirements

- Create the Wails shell and Vue 3 TypeScript-strict frontend under the
  specification's `web/`/desktop structure with a checked-in lockfile and
  reproducible build commands.
- Expose the documented local project/task/run/queue/artifact/evidence queries
  and mutations through one Go service layer shared with the CLI.
- Implement Dashboard, Projects, task backlog/compiler/review, active run,
  plan/criteria, diff, verification, audit findings, context/evidence, model
  usage, health, and settings views only to the extent backed by canonical API
  data.
- Stream progress with stable event IDs and resume; reconnect must re-query
  canonical state rather than infer missed lifecycle transitions.
- Support explicit task approval/run/cancel, queue start/cancel, and
  needs-input answer flows with confirmation and current-version checks.
- Bind local API to loopback or Unix socket, require the installation secret
  for mutations, reject browser-origin abuse/external binding by default, and
  never expose PostgreSQL/runtime sockets.
- Preserve accessible labels, focus, keyboard navigation, status semantics,
  loading/error/reconnect states, and bounded artifact rendering.

## Scope boundaries and non-goals

- Do not move lifecycle/policy/scheduling/verification logic into TypeScript or
  infer completion from client state.
- Do not add accounts, teams, remote hosting, WebSockets without a proven SSE
  limitation, daemon controls, deployment, or automatic Git export.
- Do not replace or weaken the CLI/TUI.

## Acceptance criteria

- Every displayed lifecycle/evidence value can be traced to one canonical Go
  projection and refresh/reconnect produces the same view.
- Mutations require valid local secret, current identity/version, explicit
  operator action, and return typed stale/denied outcomes.
- SSE resumes without duplicate or missing canonical event identity; malformed
  events and disconnects cannot fabricate progress.
- Browser-origin/external-bind/secret tests fail closed, and secrets never
  appear in frontend bundles/logs.
- Keyboard-only task inspection and needs-input response are usable with
  accessible names and focus behavior.
- Go tests, frontend tests, production frontend build, and diff checks pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go test ./...
npm --prefix web ci
npm --prefix web test -- --run
npm --prefix web run build
git diff --check
```

## Expected completion report

Report Go/frontend files and dependencies, shared service/API surfaces, views
implemented, canonical-state and SSE resume proof, local-security and
accessibility tests, CLI preservation, and build/test results.
