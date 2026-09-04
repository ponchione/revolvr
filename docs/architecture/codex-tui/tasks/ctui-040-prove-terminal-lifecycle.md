# CTUI-040 — Prove Core Terminal Lifecycle

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-030

## Outcome

Prove the core terminal lifecycle and regress the TTY/redirected-I/O contract
already locked by CTUI-001.

## Scope

- Exercise the locked TTY and redirected-input/output routing, output, and exit
  behavior, ordinary inline launch, normal quit, Ctrl-C, and startup-error
  cleanup.
- Inspect every terminal mode enabled by the installed stack and prove it is
  paired on exit.
- Prove cursor visibility, canonical input, and echo after each exit path.
- Prefer existing Bubble Tea lifecycle facilities; change behavior only where
  a focused failure demonstrates a gap.

## Acceptance

- Redirected-I/O cases match CTUI-001's deterministic routing, output, and
  status and emit no partial TUI controls.
- Ordinary launch remains inline and preserves prior scrollback.
- Normal quit, Ctrl-C, and startup error restore enabled modes, cursor,
  canonical input, and echo.
- Automated PTY evidence makes the proof repeatable within one fresh pass.

## Verification

- Run focused PTY cases for every scope item and inspect raw mode sequences.
- Perform an input/echo probe after each exit and run `go test ./...`.

## Not Included

- Panic restoration, resize, repeated-redraw evidence, renderer or adapter
  work, interactive-child handoff, transcript behavior, or focused surfaces.
- Requirements from the retired TUI plan.
- New launch, TTY, or redirected-I/O policy decisions.
