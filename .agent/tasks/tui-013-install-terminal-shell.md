---
id: tui-013-install-terminal-shell
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-011-prove-resize-reflow,tui-012-prove-active-settlement
---

# TUI-013 — Install the Proven Terminal Shell

- Status: Completed 2026-08-28
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-013 draft](../../docs/architecture/tui-overhaul/tasks/tui-013-install-terminal-shell.md)
- Epic:
  [E1 — Prove the terminal shell](../../docs/architecture/tui-overhaul/epics/e1-terminal-shell.md)
- Depends on:
  [completed TUI-011](tui-011-prove-resize-reflow.md) and
  [completed TUI-012](tui-012-prove-active-settlement.md)
- Design authority:
  [D3 transcript ownership](../../docs/architecture/tui-overhaul/README.md#d3--transcript-and-scrollback-ownership)
  and
  [D6 session lifecycle](../../docs/architecture/tui-overhaul/README.md#d6--session-header-lifecycle)

## Outcome

Make the proven shell the TUI container while preserving all current content,
routes, callbacks, and guards behind it.

## Scope

- Replace the persistent header/viewport/footer frame with the proven D3
  hybrid: appended committed history plus a managed live/composer/overlay
  frame.
- Add `ProjectRoot` to the existing `app.StatusResult` projection from
  `repositorypath.Inspect(...).Root()` and render it with the initial
  `Initialized` value in the accepted one-time `session-start` cell.
- Keep current dashboard content in an explicitly migration-only managed panel
  until E2 replaces it; never append dashboard strings as committed history.
- Keep current page keys, slash commands, actions, refresh, scrolling, and
  active-operation behavior reachable during migration.
- Remove only shell code made dead by this installation.

## Acceptance

- Launch no longer depends on a persistent dashboard header row.
- Startup emits exactly one `session-start` before migration-only content;
  refresh and overlay navigation do not emit another.
- Every pre-existing navigation route and command regression still passes.
- Current dashboard content remains available without callback or domain-
  authority changes.
- Shell proof, resize, and settlement checks remain green in the installed path.
- Installation adds no alternate-screen mode, terminal escape layer, or new
  dependency.
- Reverting this task would require no app/domain rollback.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go \
  internal/tui/architecture_024_test.go internal/tui/checkpoint_test.go
go test ./internal/tui
go test ./...
go run ./cmd/revolvr tui --help
git diff --check
```

## Not Included

- No semantic cell projection, primary-composer behavior, overlay migration, or
  dashboard-content deletion; no clear command, callback, or domain-authority
  change.

## Completion Evidence

- `app.StatusResult.ProjectRoot` carries the inspected absolute repository root
  for initialized and uninitialized repositories.
- `StatusModel.Init` appends one source-backed `session-start` through
  `tea.Println`; refresh, navigation, resize, and a second append cannot replay
  its process-local identity.
- `StatusModel.View` now owns only the migration panel plus footer. The former
  persistent header and its dead styling/layout code were removed while every
  current page, command, callback, guard, and dashboard projection remains.
- `TestStatusModelInstallsTranscriptShell` runs the installed Bubble Tea model
  through test IO and proves session ordering, exact-once output, retained
  migration content, and absence of the old header.
- Required formatting, focused shell/resize/settlement tests, TUI package tests,
  `go test ./...`, `go run ./cmd/revolvr tui --help`, and `git diff --check` —
  PASS.
