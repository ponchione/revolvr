# TUI-010 Publication Pass

Publish
[TUI-010](tasks/tui-010-prove-shell-composition.md) as the sole canonical
pending TUI implementation task. This is one bounded documentation and durable-
state publication pass, not the implementation pass.

## Read First

Read `AGENTS.md`, `.agent/HANDOFF.md`, `.agent/TASKS.md`, `.agent/STATE.md`,
`.agent/DECISIONS.md`, `.agent/LOOP_PROMPT.md`, the accepted TUI-overhaul design
and E0 record, the TUI-010 draft, and the repository's current canonical-task
format before editing.

## Publish Only TUI-010

- Create one canonical pending task under `.agent/tasks/` from the accepted
  TUI-010 draft, preserving its proof-only scope, dependencies, acceptance,
  verification, exclusions, and links to the literal TUI-005 snapshots.
- Make that task the first dependency-satisfied pending item in the active
  durable selector and mark this publication task complete.
- Record that E0 is accepted, TUI-010 is published but unstarted, and its
  implementation is the exact next fresh pass.
- Reconcile only affected planning and durable-state status references.

## Boundaries

Do not implement or partially prototype TUI-010. Change no Go source, Go test,
production fixture, application callback, domain state, runtime dependency,
terminal behavior, accepted D1-D6/TUI-005 decision, or unrelated backlog item.
Do not publish any later TUI task, commit, push, or start a nested Codex run.

## Completion Gate

- Exactly one new canonical pending task exists and it is TUI-010.
- Its text is sufficient for a fresh implementation pass without product
  inference and matches the accepted draft and source snapshots.
- No product/test/dependency file changed and TUI-010 has not started.
- Durable handoff selects TUI-010 implementation as the exact next fresh pass.
- Delete this prompt after it is consumed, then stop.

## Verification

```bash
git diff --check -- .agent docs/architecture/tui-overhaul
rg -n "TUI-010|pending|session-start|80|40|tea.Println" \
  .agent/tasks .agent/TASKS.md .agent/HANDOFF.md \
  docs/architecture/tui-overhaul/tasks/tui-010-prove-shell-composition.md
```

Also verify relative Markdown links, changed-path scope, exactly one newly
published task, no later published TUI task, no product/test/dependency change,
prompt deletion, and the exact next durable selector.
