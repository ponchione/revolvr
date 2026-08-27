# TUI-062 — Verify Terminal and Process Lifecycle

- Status: Draft; not canonical or runnable
- Epic: [E6 — Harden terminal behavior](../epics/e6-terminal-hardening.md)
- Depends on: [TUI-060](tui-060-lock-geometry-snapshots.md)

## Outcome

Prove that normal exit, cancellation, errors, and supported process-control
events restore a usable terminal.

## Scope

- Exercise normal quit from idle and overlay states.
- Exercise cancellation and quit during an active operation through settlement.
- Exercise a startup/runtime error path that enters Bubble Tea and exits.
- Confirm appended committed rows remain ordinary terminal history after exit
  while no managed live frame or terminal mode remains active.
- Exercise Ctrl-C and suspend/continue only where the current program and
  supported terminal environment define them.
- Record exact commands, environment, terminal state afterward, and any
  supported limitation; make only focused restoration fixes.

## Acceptance

- Normal quit, cancellation, and error leave echo, cursor, line mode, and prompt
  usable without a manual terminal reset.
- Cancellation emits one settled final row before quit and leaves no duplicate
  live state.
- Active quit still waits for domain settlement.
- Supported suspend/continue behavior is explicit and reproducible.
- Automated settlement regressions remain green.
- Unsupported signal behavior is documented rather than silently claimed.

## Verification

```bash
go test ./internal/tui -run 'Test.*(Quit|Cancel|Settlement|Error)'
go test ./internal/tui
go run ./cmd/revolvr tui
git diff --check -- docs internal/tui
```

## Not Included

- No transcript navigation/copy matrix, daemonization, or replacement terminal
  runtime.
