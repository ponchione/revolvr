# Agent Decisions

- The repo itself is the durable state for autonomous loop runs.
- Each loop pass starts as a fresh `codex exec` session.
- Do not use resumed Codex sessions for this workflow.
- Work is limited to one small, verifiable task per pass.
- Existing project patterns should win over new abstractions unless a task explicitly asks for a change.
- Tests, build output, and typechecks are the source of truth.
- `.revolvr/` is local runtime state for the Revolvr CLI and is ignored by Git.
- CLI-initiated harness runs default to Codex dangerous bypass/yolo mode for unattended autonomy; repo config can disable it with `codex.dangerously_bypass_approvals_and_sandbox: false` or `codex.yolo: false`.
- `revolvr init` keeps runtime state local by adding `/.revolvr/` to `.git/info/exclude` when initialized from a Git worktree, avoiding tracked `.gitignore` changes.
- Dogfood readiness is exposed as a top-level `revolvr doctor` command so preflight checks stay separate from config inspection.
- `revolvr run` streams summarized Codex progress events to stdout while keeping raw Codex JSONL and stderr as run artifacts; console output should stay human-readable and artifact-backed.
- `revolvr receipt validate <run-id>` treats the ledger run row plus run events as the source of truth for finalized receipt validation, and recorded artifact paths must exist on disk.
- `revolvr task retry <task-id>` is the preferred blocked-task recovery command; it reuses the same blocked-to-pending task update as `task unblock` so task identity and prior run history remain intact.
- `revolvr run --max-passes` must print one concise final loop summary and stop before a failed dirty pass or blocked outcome can cascade into blocking unrelated tasks; only clean failed outcomes may repeat, and those stop after two consecutive failures.
- The real-Codex live dogfood check is an opt-in script (`scripts/dogfood-live.sh`) because it removes local `.revolvr/` runtime state and creates a Git commit; it must require a clean source worktree before resetting state.
- Read-only CLI inspection should move orchestration into `internal/app` while keeping exact command rendering in `internal/cli`; `status` and `show` are the first commands following this boundary.
- Receipt validation orchestration now follows the same boundary: `internal/app` resolves state, loads run history, and invokes receipt validation, while `internal/cli` keeps exact command rendering and failed-check exit formatting.
- Task queue command orchestration follows the same boundary: `internal/app` owns task add/list/retry/unblock state resolution, task store access, and blocked-to-pending transitions, while `internal/cli` keeps the exact command output formatting.
