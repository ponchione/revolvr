# Agent Handoff

Updated: 2026-09-04

## Pause Point

- CTUI-010 is terminally complete. Its implementation consumes the accepted
  ordinary initialized launch contract at
  `docs/architecture/codex-tui/launch-contract.md`.
- `ctui-020-match-initial-frame` is the only canonical pending task. It was
  published as CTUI-010's accepted successor and has not been executed.
- Local `main`, `HEAD`, and `origin/main` remain at
  `e0b6372f54f8721aa59a767376b0266d29f97876`. Preserve that baseline; do not
  reset, rebase, or reconstruct it.
- CTUI-010's product code, focused tests, provenance-recorded local Bubble Tea
  v1.3.4 source replacement, documentation, and durable metadata are
  uncommitted and ready for review. The module identity/version set is
  unchanged and no Codex source was copied.

## Read-Only Selector

Run:

```bash
go run ./cmd/revolvr status
```

The expected selector is `ctui-020-match-initial-frame`. Historical run
records may still appear; they are local runtime history, not selectable tasks.

## Resume Rule

- Start a fresh pass and execute only the canonical CTUI-020 task.
- Read the accepted launch contract before implementation. Replace the
  disposable `Loading…` tracer with its exact initialized loading and ready
  fixtures and implement lossless editable-draft transfer without reopening
  the shared route, TTY gate, or startup-branch presentation.
- Leave CTUI-025 and every later draft unpublished until the current canonical
  task terminally completes under the repository fresh-pass rule.
- Do not copy Codex source, add a dependency, or revive the retired TUI plan.

## CTUI-010 Completion Evidence

- Focused dispatch, exact-help, parsing, version, all-row gate, process-init,
  and blocked-bootstrap tests passed. `go test ./...` passed.
- The six redirected 80x24 PTY rows had empty stdout, exact stdin-first errors,
  and exit 1. Bare and explicit interactive 80x24 captures had empty stderr,
  exited 0 after Ctrl-C, and were byte-identical at 241 bytes with SHA-256
  `548b67057efeaa5cc5cca1a2a458f457a67f7543647bcc44073bb17a85f2f310`.
  Both live ANSI replays were inspected and had no startup history or
  CTUI-020 presentation.
- The existing Bubble Tea v1.3.4 identity is locally supplied with only its
  unconditional pre-main terminal probe removed. Provenance is recorded in
  `third_party/bubbletea/REVOLVR_PATCH.md`; all 77 baseline module identities
  and versions remain unchanged.
- Focused CLI checks, `gofmt`, `git diff --check`, and explicit untracked-file
  checks passed. The final read-only selector reported exactly:

```text
Total tasks: 3
Pending tasks: 1
Blocked tasks: 0
Completed tasks: 2
Next task: ctui-020-match-initial-frame - CTUI-020 — Match Initialized Loading and Ready Frames
Next pass: workflow=mixed-pass-v1 phase=implement profile=implementer next=audit
```

- `go run ./cmd/revolvr task list` reported CTUI-001 and CTUI-010 `completed`
  and CTUI-020 `pending`, `ready`, and selected with CTUI-010 as its only
  dependency.
