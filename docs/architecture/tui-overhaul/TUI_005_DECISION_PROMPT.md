# TUI-005 Decision Pass

Execute
[TUI-005](tasks/tui-005-accept-experience-states.md) as one bounded,
documentation-only decision pass.

## Read First

Read `AGENTS.md`, `.agent/HANDOFF.md`, `.agent/TASKS.md`, `.agent/STATE.md`,
`.agent/DECISIONS.md`, and the relevant TUI-overhaul design, reference, epic,
accepted D1-D6, experience-sketch, shell, transcript, composer, live-operation,
overlay, geometry, dashboard-removal, operator-documentation, and closeout
files before editing.

## Resolve Only the Experience States

- Replace the illustrative sketches with accepted initialized-idle,
  uninitialized, running, completed, failed, cancelled, needs-input, overlay,
  and 40-column narrow source snapshots or wireframes.
- Annotate every visible row or fact as committed session/transcript cell,
  replaceable live cell, composer, overlay, or transient footer, using the
  exclusive ownership accepted by D6.
- Use the accepted `session-start` sources and point-in-time initialization
  wording without adding persistent duplicate header chrome or a clear action.
- Settle exact operator-visible wording for safety state, cancellation, current
  work, terminal outcomes, and the next useful action.
- Record normal width, minimum supported width, wrap/truncation rules, and
  behavior below the minimum.
- Reconcile every affected planning, reference, and durable-state document.

## Boundaries

Keep this pass reversible and documentation-only. Preserve accepted D1-D6,
ADR-025, product code, application callbacks, domain state, runtime
dependencies, and existing operator behavior. Publish no canonical/runnable
task and do not start TUI-010. Create no production test fixture, shell
implementation, transcript-cell implementation, footer redesign, clear
command, commit, or push.

## Completion Gate

- Every required state has one exact accepted text snapshot or wireframe with
  no placeholder phrasing.
- The snapshots implement D1-D6 without conflicting routes, duplicated facts,
  or an unsupported action.
- Important state, focus, safety, cancellation, outcome, and next-action text
  remains visible at 40 columns and below-minimum behavior is intentional.
- E0 planning and TUI-005 record the accepted snapshot decision while TUI-010
  remains unpublished and unstarted.
- Durable handoff selects a separate TUI-010 publication pass as the exact next
  fresh pass.
- Delete this prompt after it is consumed, then stop.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul \
  .agent/TASKS.md .agent/DECISIONS.md
rg -n \
  "initialized|uninitialized|running|completed|failed|cancelled|needs-input|overlay|40-column|session-start" \
  docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
```

Also verify relative Markdown links, changed-path scope, accepted-D1-D6
consistency, all required state/owner annotations, the 33-task count, no
published TUI-010, no product-code/runtime-dependency change, prompt deletion,
and the exact next durable selector.
