# TUI-072 — Close the Overhaul Acceptance Record

- Status: Draft; not canonical or runnable
- Epic: [E7 — Remove the old dashboard shell](../epics/e7-remove-dashboard.md)
- Depends on: [TUI-071](tui-071-update-operator-docs.md)

## Outcome

Run and record the complete automated/manual acceptance evidence, then close
the draft plan only if every criterion passes.

## Scope

- Evaluate every whole-overhaul criterion in the design authority.
- Run the full Go suite and focused CLI help checks.
- Repeat the accepted E6 terminal scrollback, lifecycle, and accessibility
  matrices against the final build.
- Record exact commands/results and reconcile `.agent/HANDOFF.md`,
  `.agent/TASKS.md`, `.agent/STATE.md`, and only affected decision references.
- Mark the design/epics/tasks complete or historical only after all evidence is
  present.

## Acceptance

- Every whole-overhaul criterion points to passing automated or manual evidence.
- No failing criterion is waived inside this closeout task; it becomes one
  bounded corrective task and this task remains incomplete.
- Existing CLI behavior outside `revolvr tui` remains unchanged.
- Durable state selects no completed/superseded draft item.
- No draft is promoted twice and no unaccepted task is published.

## Verification

```bash
go test ./internal/tui
go test ./...
go run ./cmd/revolvr tui --help
go run ./cmd/revolvr --help
git diff --check
git diff --name-only
```

Repeat and record the exact manual commands accepted by TUI-061 through
TUI-063.

## Not Included

- No bundled repair, new feature, unrelated cleanup, or commit without explicit
  operator authorization.
