# TUI-070 — Remove Obsolete Dashboard Presentation

- Status: Draft; not canonical or runnable
- Epic: [E7 — Remove the old dashboard shell](../epics/e7-remove-dashboard.md)
- Depends on: E1-E6 exit gates

## Outcome

Delete migration-only dashboard/page presentation so the accepted transcript
application has one rendering and navigation model.

## Scope

- Delete dashboard-only activity compaction, persistent header/footer chrome,
  inactive composer state, and page-only navigation made obsolete by parity
  migrations.
- Delete obsolete tests and replace only those still needed to assert accepted
  behavior.
- Retain app callbacks, projections, focused renderers, guards, and key routes
  still used by transcript, commands, or overlays.
- Remove migration aliases only when their E5 parity evidence allows it.

## Acceptance

- Launch output contains no Dashboard label or dashboard-only content path.
- No fact is rendered through duplicate transcript/live/page representations.
- Every accepted action remains reachable through documented command/key paths.
- The diff removes more presentation code than it adds and introduces no new
  abstraction.
- Full tests show no behavior change outside the accepted TUI replacement.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go \
  internal/tui/architecture_024_test.go internal/tui/checkpoint_test.go
go test ./internal/tui
go test ./...
rg -n "Dashboard|inactive composer|dashboard" internal/tui
git diff --check
```

Any remaining match must be an intentional historical/test fixture and be
accounted for in the completion evidence.

## Not Included

- No operator-documentation rewrite, final manual matrix, app/domain cleanup,
  or unrelated TUI refactor.
