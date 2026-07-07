# Agent Decisions

- The repo itself is the durable state for autonomous loop runs.
- Each loop pass starts as a fresh `codex exec` session.
- Do not use resumed Codex sessions for this workflow.
- Work is limited to one small, verifiable task per pass.
- Existing project patterns should win over new abstractions unless a task explicitly asks for a change.
- Tests, build output, and typechecks are the source of truth.
- `.revolvr/` is local runtime state for the Revolvr CLI and is ignored by Git.
- CLI-initiated harness runs default to Codex dangerous bypass/yolo mode for unattended autonomy; repo config can disable it with `codex.dangerously_bypass_approvals_and_sandbox: false` or `codex.yolo: false`.
