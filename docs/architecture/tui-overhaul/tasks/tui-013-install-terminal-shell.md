# TUI-013 — Install the Proven Terminal Shell

- Status: Draft; not canonical or runnable
- Epic: [E1 — Prove the terminal shell](../epics/e1-terminal-shell.md)
- Depends on: [TUI-011](tui-011-prove-resize-reflow.md) and
  [TUI-012](tui-012-prove-active-settlement.md)

## Outcome

Make the proven shell the TUI container while preserving all current content,
routes, callbacks, and guards behind it.

## Scope

- Replace the persistent header/viewport/footer frame with the proven D3
  hybrid: appended committed history plus a managed live/composer/overlay
  frame.
- Render the accepted one-time session cell and bottom region.
- Keep current dashboard content in an explicitly migration-only managed panel
  until E2 replaces it; never append dashboard strings as committed history.
- Keep current page keys, slash commands, actions, refresh, scrolling, and
  active-operation behavior reachable during migration.
- Remove only shell code made dead by this installation.

## Acceptance

- Launch no longer depends on a persistent dashboard header row.
- Every pre-existing navigation route and command regression still passes.
- Current dashboard content remains available without app/domain changes.
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
  dashboard-content deletion.
