# Agent Decisions

- The repo itself is the durable state for autonomous loop runs.
- Each loop pass starts as a fresh `codex exec` session.
- Do not use resumed Codex sessions for this workflow.
- Work is limited to one small, verifiable task per pass.
- Existing project patterns should win over new abstractions unless a task explicitly asks for a change.
- Tests, build output, and typechecks are the source of truth.
- `.revolvr/` is local runtime state for the Revolvr CLI and is ignored by Git.
