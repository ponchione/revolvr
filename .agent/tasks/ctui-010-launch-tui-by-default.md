---
id: ctui-010-launch-tui-by-default
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 3
depends_on: ctui-001-lock-launch-contract
---

# CTUI-010 — Establish Early TUI Ownership

- Status: Completed 2026-09-04
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

- [x] Bare and explicit interactive entries share the same dispatcher, stream
  gate, model, callbacks, and single Bubble Tea program.
- [x] An injected blocked bootstrap cannot delay terminal ownership or the first
  nonempty pending render. Releasing it updates that same program rather than
  starting a second program or replaying startup history.
- [x] Every TTY/redirected combination produces the exact stdout, stderr, exit
  status, check order, and pre-bootstrap behavior in the accepted matrix.
- [x] Root help, TUI help, `--version`, unknown inputs, and all existing non-TUI
  subcommands retain the accepted route and text. Bare launch no longer falls
  back to help when both streams are TTYs.
- [x] The implementation makes no lasting visual, styling, editable-draft,
  action-gating, uninitialized, or startup-error presentation decision.
- [x] No dependency or Codex source is added, copied, ported, vendored, or embedded.

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

## Completion Evidence

- Baseline guard: before edits, local `main`, `HEAD`, and `origin/main` were all
  `e0b6372f54f8721aa59a767376b0266d29f97876`; both the worktree and index were
  clean. The initial `go run ./cmd/revolvr status` selector reported 2 total,
  1 pending, 0 blocked, and 1 completed task, with only CTUI-010 selected.
- Bare `revolvr` and explicit `revolvr tui` now call one `runTUI` dispatcher.
  Cobra still owns help, version, parse failures, and explicit non-TUI
  commands. Focused tests cover the exact root/TUI help, dispatch, parse and
  version bypasses, every effective-stream gate outcome, stdin-first order,
  and zero pre-bootstrap calls on refusals.
- One inline Bubble Tea program now paints the disposable unstyled `Loading…`
  model before its initial command can run bootstrap. The blocked-bootstrap
  test observed that pending render while bootstrap remained blocked, then
  observed ready output from the same program after one completion. Startup
  committed cells are marked emitted without a startup `tea.Println` append.
- Bubble Tea v1.3.4's unconditional package initializer emitted OSC 11 and DSR
  bytes before `main`, ahead of any possible application gate. The existing
  dependency identity is therefore supplied by `third_party/bubbletea` with
  only that initializer removed; provenance and the one-file delta are
  recorded in `REVOLVR_PATCH.md`. Normalized `go list -m all` output retained
  the same 77 module identities and versions; no dependency or Codex source
  was added.
- The six redirected 80x24 PTY rows all exited 1 with zero stdout and exact
  stderr: 24-byte `stdin is not a terminal\n` for redirected stdin or both,
  and 25-byte `stdout is not a terminal\n` for redirected stdout. The
  executable-level PTY regression also proved no package-init bytes precede
  `main` and that `TERM=xterm-256color` is unchanged when `main` begins.
- Fresh bare and explicit fixed 80x24 PTY captures both exited 0 after Ctrl-C,
  had empty stderr, and were byte-identical: 241 bytes with SHA-256
  `548b67057efeaa5cc5cca1a2a458f457a67f7543647bcc44073bb17a85f2f310`.
  Inspection of both live ANSI replays showed only the retained inline
  composer marker and shortcut footer, with no startup session/history, card,
  clipping, corruption, or alternate-screen transition.
- Focused CLI checks passed for root/TUI help, `--version`, `config check`,
  `status`, unknown commands, positional input, and malformed flags. Focused
  launch tests and `go test ./...` passed. `gofmt` covered every changed Go
  file; `git diff --check` and explicit checks of all untracked files returned
  no whitespace errors.
