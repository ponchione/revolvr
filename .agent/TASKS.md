# Agent Tasks

Updated: 2026-09-04

## Active Codex TUI Sequence

- The accepted task graph and publication order are in
  `docs/architecture/codex-tui/README.md`.
- `ctui-001-lock-launch-contract` is completed.
- `ctui-010-launch-tui-by-default` is the only canonical pending task.
- CTUI-020 through CTUI-070 remain accepted unpublished drafts. Publish them
  one at a time in numbered order only after the current task completes and
  its direct dependencies are satisfied.
- The former architecture, PTC, EXT, and transcript-first TUI task inventories
  remain retired. Git history is their archive; do not recover or republish
  them.

## Rules

- Execute exactly one canonical task per fresh pass.
- Mark it complete only after its acceptance and verification pass.
- A completion handoff may publish exactly one accepted, dependency-satisfied
  successor; it must not execute that successor in the same pass.
- Do not copy, port, vendor, embed, or depend on Codex source.

## Current Work

- [x] CTUI-001 — accepted the ordinary initialized launch contract from fresh
  `standard-80x24` and `narrow-60x20` evidence without changing product code or
  dependencies.
- [ ] CTUI-010 — implement shared bare/explicit launch and early terminal
  ownership from the accepted contract in a separate fresh pass.
- Do not execute CTUI-010 in the CTUI-001 completion pass.
