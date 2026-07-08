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
