# Agent Handoff

Updated: 2026-09-04

## Pause Point

- CTUI-001 is terminally complete. Its accepted ordinary initialized launch
  contract is `docs/architecture/codex-tui/launch-contract.md`.
- `ctui-010-launch-tui-by-default` is the only canonical pending task. It was
  published as CTUI-001's accepted successor and has not been executed.
- Local `main` remains at
  `ac61907f7469f8a5836e9ee57a59066c854f2b4d`, exactly one commit ahead of
  `origin/main`. Preserve that baseline; do not reset, rebase, or reconstruct
  it from the remote.
- CTUI-001 changed documentation and durable task metadata only. Its changes
  are uncommitted and ready for review; no dependency or product code changed.

## Read-Only Selector

Run:

```bash
go run ./cmd/revolvr status
```

The expected selector is `ctui-010-launch-tui-by-default`. Historical run
records may still appear; they are local runtime history, not selectable tasks.

## Resume Rule

- Start a fresh pass and execute only the canonical CTUI-010 task.
- Read the accepted launch contract before implementation. Consume its shared
  route, TTY gate, exact help, pending-frame boundary, and ownership order
  without reopening the locked fixtures or startup-branch presentation.
- Leave CTUI-020 and every later draft unpublished until the current canonical
  task terminally completes under the repository fresh-pass rule.
- Do not copy Codex source, add a dependency, or revive the retired TUI plan.

## CTUI-001 Completion Evidence

- Fresh 80x24 and 60x20 PTY captures, current redirected-I/O probes, literal
  fixtures, SHA-256 identities, and primary-source citations are recorded in
  the accepted contract.
- An acceptance-coverage check passed all 16 checks. A local Markdown validator
  resolved all 36 local links and source line anchors in the 8 changed files;
  fixture validation confirmed all 48 card rows at their locked widths.
- `git diff --check` and explicit no-index checks for both new files passed.
  The final read-only selector reported exactly:

```text
Total tasks: 2
Pending tasks: 1
Blocked tasks: 0
Completed tasks: 1
Next task: ctui-010-launch-tui-by-default - CTUI-010 — Establish Early TUI Ownership
Next pass: workflow=mixed-pass-v1 phase=implement profile=implementer next=audit
```

- `go run ./cmd/revolvr task list` reported CTUI-001 `completed` and CTUI-010
  `pending`, `ready`, and selected with CTUI-001 as its only dependency.
