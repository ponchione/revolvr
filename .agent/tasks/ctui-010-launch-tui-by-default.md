---
id: ctui-010-launch-tui-by-default
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 3
depends_on: ctui-001-lock-launch-contract
---

# CTUI-010 — Establish Early TUI Ownership

- Status: Pending
- Accepted plan: [Codex TUI task plan](../../docs/architecture/codex-tui/README.md)
- Accepted draft: [CTUI-010](../../docs/architecture/codex-tui/tasks/ctui-010-launch-tui-by-default.md)
- Authoritative contract: [Ordinary initialized launch contract](../../docs/architecture/codex-tui/launch-contract.md)
- Depends on: CTUI-001 (completed)
- Completion handoff: publish only CTUI-020; do not execute it in the same pass

## Outcome

Route ordinary initialized launch through one dispatcher and establish Bubble
Tea ownership before asynchronous bootstrap begins, without implementing the
accepted loading or ready presentation.

## Scope

- Make no-argument bare `revolvr` and explicit `revolvr tui` call the same TUI
  dispatcher and construct one terminal-owning Bubble Tea program.
- Keep help, `--version`, Cobra parse errors, and every existing explicit
  non-TUI command ahead of the TUI route. Replace only the retired TUI help
  description with the exact accepted help text.
- Apply the accepted stdin-first/stdout-second TTY gate to both TUI entries
  before any bootstrap/status/config read or terminal control output. Use the
  command's effective streams; do not reopen a controlling terminal.
- Let Bubble Tea render a nonempty minimal pending frame before starting the
  asynchronous bootstrap, then deliver one completion result to the same
  running model.
- Remove startup session/history emission through `tea.Println` or equivalent
  terminal-owned scrollback output.
- Use only the contract's disposable unstyled `Loading…` tracer if a pending
  marker is needed. Do not implement or snapshot CTUI-020's accepted fixtures.

## Acceptance

- Bare and explicit interactive entries share the same dispatcher, stream
  gate, model, callbacks, and single Bubble Tea program.
- An injected blocked bootstrap cannot delay terminal ownership or the first
  nonempty pending render. Releasing it updates that same program rather than
  starting a second program or replaying startup history.
- Every TTY/redirected combination produces the exact stdout, stderr, exit
  status, check order, and pre-bootstrap behavior in the accepted matrix.
- Root help, TUI help, `--version`, unknown inputs, and all existing non-TUI
  subcommands retain the accepted route and text. Bare launch no longer falls
  back to help when both streams are TTYs.
- The implementation makes no lasting visual, styling, editable-draft,
  action-gating, uninitialized, or startup-error presentation decision.
- No dependency or Codex source is added, copied, ported, vendored, or embedded.

## Verification

- Add focused dispatch, help, parse, version, TTY-gate, and delayed-bootstrap
  tests. Prove no bootstrap callback runs before the pending render is allowed.
- Capture bare and explicit entries in a fixed 80x24 PTY; inspect that each
  reaches the same inline pending program without startup scrollback.
- Exercise all accepted redirected-stdin/stdout combinations and compare exact
  output and status with the launch matrix.
- Run `gofmt` on changed Go files, `go test ./...`, focused CLI commands, and
  `git diff --check`.
- Rerun the read-only selector before completion.

## Not Included

- CTUI-020's loading/ready card, composer, footer, styles, editable draft,
  gating, focus transfer, or snapshot fixtures.
- CTUI-025's uninitialized or startup-error fixtures and interactions.
- Terminal lifecycle hardening, result append behavior, command discovery,
  overlays, viewport policy, or any requirement from the retired TUI plan.
