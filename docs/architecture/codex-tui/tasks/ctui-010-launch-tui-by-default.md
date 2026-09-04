# CTUI-010 — Establish Early TUI Ownership

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-001

## Outcome

Route ordinary initialized launch through one dispatcher and establish Bubble
Tea ownership before asynchronous bootstrap begins.

## Scope

- Share dispatch between bare and explicit TUI launch while preserving help
  and existing subcommands, and implement the redirected-I/O routing locked by
  CTUI-001.
- Start Bubble Tea before bootstrap, paint a minimal pending frame immediately,
  and deliver bootstrap completion asynchronously to the running program.
- Remove startup `tea.Println` history emission.

## Acceptance

- Both interactive entries reach the same dispatcher and terminal-owning
  program.
- A delayed bootstrap cannot delay ownership or the minimal first frame.
- Startup history is not printed before or during startup; non-TUI routes are
  unchanged, and redirected invocations produce the locked output and status.
- The slice has focused dispatch, delayed-bootstrap, and PTY evidence and fits
  one fresh pass.

## Verification

- Run focused dispatch and delayed-bootstrap tests.
- Capture both entries in a fixed-size PTY and inspect ordering and scrollback.
- Exercise the CTUI-001 help, subcommand, and redirected-I/O routing matrix.
- Run `go test ./...`.

## Not Included

- Editable drafts, action gating, loading/ready fixture presentation, startup
  branch presentation, exact fixture styling, redirected-I/O policy decisions,
  or terminal hardening.
- New dependencies or requirements from the retired TUI plan.
