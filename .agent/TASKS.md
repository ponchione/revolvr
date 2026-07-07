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

## Blocked

None.
