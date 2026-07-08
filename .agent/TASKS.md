# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, write the blocker under `Blocked` and stop.
- Add new tasks only when they are directly discovered while working on the current task.
- New tasks must be specific, small, and verifiable.
- Do not invent broad roadmap items.

## Backlog

- [x] Replace bare parent command placeholder output for `revolvr task` and `revolvr config` with normal help output, and update focused CLI tests.
- [x] Add Codex yolo/dangerous bypass support for autonomous harness runs, fix fresh-session wrapper flags, and update focused tests.
- [x] Add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- [x] Expand `revolvr config check` output to show the effective verification command details, not only the command count.
- [x] Add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- [x] Add a no-real-Codex integration smoke test for `revolvr run --once` success path using a strict fake Codex executable.
- [x] Add a no-real-Codex integration smoke test for `revolvr run --once` verification failure path using a strict fake Codex executable.
- [x] Make `revolvr init` locally ignore `.revolvr/` in Git worktrees so fresh dogfood repos do not start dirty.
- [x] Add a `revolvr doctor` dogfood preflight for Codex, Git identity, clean worktree, runtime ignore state, and verification readiness.
- [x] Resolve dogfood run diagnostics by finalizing receipt body facts, preserving dotfile changed-file claims, recovering Codex usage metrics after malformed JSONL fragments, and streaming summarized `run` progress.
- [x] Overwrite finalized receipt timestamps with the harness completion time instead of preserving agent-authored stale timestamps.
- [x] Expand `revolvr status` to show latest run summary, verification status, commit SHA, and artifact path hints when a run exists.
- [x] Add a first-class receipt validation command that checks a run receipt against ledger completion time, commit SHA, changed files, verification results, and artifact existence.
- [x] Add focused failure-recovery CLI support for blocked tasks, starting with one command to retry or unblock a blocked task safely.
- [x] Add safer `run --max-passes` loop guardrails for repeated failures or blocked tasks, and show a concise final loop summary.
- [x] Add a live dogfood verification script or README checklist that resets runtime state, queues a tiny task, runs once, and verifies receipt, ledger, commit, and clean-worktree consistency.
- [x] Introduce `internal/app` with read-only `Status` and `ShowRun` operations, update CLI `status` and `show` to use it without changing output, and add focused tests.
- [x] Move receipt validation orchestration behind `internal/app`, update CLI `receipt validate` to use it without changing output, and add focused tests.
- [x] Move task add/list/retry orchestration behind `internal/app`, update CLI task commands to use it without changing output, and add focused tests.
- [x] Move run once and run loop orchestration behind `internal/app`, preserving CLI output and `run --max-passes` guardrails.
- [x] Add stable Charm dependencies for Bubble Tea, Bubbles, and Lip Gloss, and create a minimal `internal/tui` model that renders a static app status snapshot in tests.
- [x] Add `revolvr tui` showing task counts, latest run summary, and recent runs from `internal/app`.
- [x] Add basic TUI actions for refresh, opening selected run details, and quit, without starting real Codex runs yet.

## Blocked

None.
