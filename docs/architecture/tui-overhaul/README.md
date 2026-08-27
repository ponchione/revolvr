# TUI Overhaul — Transcript-First Operator Application

- Status: Draft for operator review
- Baseline date: 2026-08-27
- Reference implementation: local `openai/codex` checkout at
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`
- Implementation status: blocked until the required decisions below are
  accepted
- Epic and task index: [Implementation plan](#implementation-plan)

## Purpose

Replace Revolvr's dashboard-and-pages presentation with a terminal application
that feels like the Codex CLI: a durable transcript, one live operation area,
an always-available composer, and temporary overlays for focused actions.

This is a TUI architecture and interaction change, not a replacement runtime.
Revolvr's Go application services, safety policy, scheduler, ledger, receipts,
artifacts, and task lifecycle remain authoritative.

This document and the linked epic/task files are deliberately drafts. They
capture the current system, the proposed target, open decisions, and a bounded
implementation sequence so the team can edit the design before changing more
product code.

## Plan Use and Task Contract

The files under `epics/` and `tasks/` are the only draft implementation plan.
They are not canonical tasks and must not be copied wholesale into
`.agent/tasks/` or `.agent/TASKS.md`.

Each task owns one decision, proof, component change, view migration, or
verification pass. Its tests and documentation of that same outcome are part
of its definition of done, not separate work. If implementation exposes a
missing app/domain capability, add one bounded prerequisite task rather than
absorbing that capability into a TUI task.

After the design is accepted, promote only the first dependency-satisfied task.
Each fresh pass completes that one task, verifies it, updates durable state,
and stops. Combine adjacent draft tasks only when the code proves they are one
smaller change; do not combine them for scheduling convenience.

## Implementation Plan

| Epic | Outcome | Depends on | Exit gate |
| --- | --- | --- | --- |
| [E0 — Settle the product contract](epics/e0-product-contract.md) | accepted source, input, shell, overlay, and experience decisions | none | D1-D6 resolved and experience snapshots accepted |
| [E1 — Prove the terminal shell](epics/e1-terminal-shell.md) | the chosen transcript/scrollback technique works in Bubble Tea | E0 | composition, resize/reflow, IO, and settlement proofs pass |
| [E2 — Build semantic transcript cells](epics/e2-semantic-transcript.md) | historical and live evidence share one presentation vocabulary | E1 | refresh/restart and live completion reconcile |
| [E3 — Make the composer primary](epics/e3-primary-composer.md) | always-focused input and command discovery have explicit semantics | E1, D2 | slash commands and reviewed idle task drafts work safely |
| [E4 — Surface runs and loops live](epics/e4-live-operations.md) | one live cell presents progress, passes, cancellation, and terminal state | E2, E3 | active operation never becomes a log wall |
| [E5 — Move focused work to overlays](epics/e5-overlays.md) | existing focused views remain available without replacing the transcript | E3 | each focused-view parity task passes |
| [E6 — Harden terminal behavior](epics/e6-terminal-hardening.md) | geometry, scrollback, lifecycle, and text styling work in supported terminals | E1-E5 | automated geometry and manual terminal checks pass |
| [E7 — Remove the old dashboard shell](epics/e7-remove-dashboard.md) | only the accepted transcript application remains | E1-E6 | obsolete presentation is gone and final acceptance passes |

## Why the Current Console Still Feels Like a Dashboard

The compact dashboard maintenance pass removed the worst duplication, but the
underlying interaction model did not change:

- the whole screen is still a persistent header, a Bubbles viewport, and a
  footer;
- the dashboard reconstructs the latest run into a fixed page on every draw;
- the composer is an inactive footer affordance until `/` is pressed;
- Tasks, Runs, Run Detail, Preflight, Workflow, Help, Change Summary, Evidence,
  and Approval are page-like views selected in the main model;
- live progress is prepended to the selected page rather than being one live
  transcript cell;
- terminal scrollback is not the primary history/navigation surface.

The result is more readable than the earlier event wall, but it still feels
like a status screen with a command entry mode. Codex feels like an application
because its transcript, live state, composer, and overlays form one continuous
interaction model.

## Current Revolvr Baseline

This section describes the current working tree, including the uncommitted TUI
presentation and task-publication changes present on 2026-08-27.

### Program and shell

- `internal/tui.RunStatus` starts the existing Bubble Tea program in inline
  mode. It does not request the alternate screen.
- `StatusModel.View` renders the complete visible frame as:
  one-line header, blank row, viewport, blank row, footer.
- Bubbles `viewport.Model` owns the central content area's dimensions and
  scrolling. The viewport height is derived from the current header/footer
  row count.
- Lip Gloss is applied after plain-text wrapping. Primary text uses the
  terminal default, headings are bold, secondary text is faint, selection is
  ANSI cyan, success is ANSI green, and failure is ANSI red.

### Dashboard

- The header is `Revolvr  <view>  <initialized state>`.
- An initialized dashboard projects the latest run's compact operator status
  and `app.RunTimeline` history.
- Dashboard history filters run/task duplication and low-level Codex lifecycle
  noise, keeps the latest eight meaningful transitions, shortens long detail,
  and points to Run Detail for complete history.
- With no run history, the dashboard shows `Idle`, no-run state, and the next
  runnable task when one exists.
- An uninitialized repository shows one actionable `revolvr init` instruction.
- The dashboard no longer duplicates task counts, Latest Run, Recent Runs, or
  raw ledger Events.

### Composer and commands

- The inactive dashboard footer always shows `› / for commands`.
- `/` activates a hand-built command composer with the same `› ` prompt.
- Enter executes an existing slash command and Escape closes the composer.
- Plain text is not currently a first-class operator message.
- Full discovery remains in Help and the slash-command list.

### Live operations

- Existing app callbacks start single passes, bounded loops, autonomous task
  runs, and queues.
- One TUI-started operation may be active. Its context owns cancellation.
- `c`, Escape, `q`, or Ctrl-C preserve the established cancellation and
  operation-settlement rules for the relevant state.
- Live progress is retained in model state and rendered above the current view.
- Quitting during an active operation waits for the domain result to settle;
  the TUI does not pretend cancellation completed early.

### Focused views

The current model already exposes the information the new shell needs:

- Tasks and task creation/retry;
- Runs and Run Detail;
- Preflight;
- autonomous Workflow and typed needs-input answers;
- Change Summary, Evidence, and Approval;
- receipt validation and Help.

These views call existing `StatusActions` callbacks. They do not load or mutate
canonical stores directly, and that boundary must survive the overhaul.

### Existing test authority

`internal/tui/model_test.go`, `architecture_024_test.go`, and
`checkpoint_test.go` currently cover rendering at wide/narrow widths,
navigation, command execution, focused views, refresh, scrolling, cancellation,
typed responses, and the active-operation quit settlement contract. The new
work should replace obsolete presentation assertions while retaining these
behavioral guarantees.

## Proposed Target Experience

The target has four persistent concepts instead of a dashboard and many pages:

1. **Committed transcript** — completed operator and Revolvr entries form the
   readable history. The transcript is traceable to canonical app projections.
2. **Live cell** — the current run/loop state changes in place and becomes a
   committed transcript entry only when its meaning is stable.
3. **Composer** — the prompt is always visible and focused when no modal input
   owns focus. `/` discovers commands; initialized idle plain text opens the
   existing reviewed Add Task flow and is unavailable in every other state.
4. **Overlay** — Tasks, Runs, Run Detail, Preflight, Workflow, Help, changes,
   evidence, approvals, and typed questions temporarily cover the transcript
   without replacing its identity.

The target is behavioral fidelity to the useful Codex interaction model under
Revolvr branding. It is not a Rust port or a second runtime.

### Idle sketch

```text
Revolvr
  project  /path/to/repository
  state    initialized · ready

• Ready
  Next task  Compact durable agent state
  Workflow   mixed-pass-v1 · audit

› /run
  ? for shortcuts                         ready
```

### Running sketch

```text
• Started task Compact durable agent state

• Codex
  I am inspecting the durable state and task conventions.

• Working (18s · pass 1/3 · esc to interrupt)
  └ Running go test ./...

› / for commands
  ? for shortcuts                         run active
```

### Completed sketch

```text
• Completed task Compact durable agent state
  Verification passed · commit ff50d9b5cd07
  2 receipt warnings · /run to inspect

› /run
  ? for shortcuts                         ready
```

The text is illustrative, not accepted event phrasing. Exact cells and command
semantics are settled by the decision and snapshot tasks before implementation.

## Presentation Architecture

```text
canonical task/state + ledger + artifacts
                  |
          existing app services
                  |
        semantic operator projections
                  |
    +-------------+-------------+
    |             |             |
transcript     live cell      overlays
    +-------------+-------------+
                  |
               composer
```

The app-service boundary is the deep module. The TUI should depend on its small
typed interface and hide terminal layout, focus, reflow, and command discovery
inside `internal/tui`.

Do not introduce a Go interface merely to name this diagram. `StatusActions`
and existing app result types are already the seam. Add a new application
projection only when the TUI otherwise has to infer business meaning from raw
events, and put that projection beside the existing app services.

### Projection rules

- Canonical state is never reconstructed from rendered strings.
- `app.RunTimeline` remains the current semantic run-history projection.
- Raw ledger events remain audit/debug evidence in Run Detail, not the main
  transcript.
- A displayed action must route to an existing app service or to an explicitly
  accepted new app service. The TUI does not mutate task, state, ledger, or
  artifact files.
- Stored and live forms of the same transition must reconcile by stable domain
  identity, not timestamp/prose heuristics.
- Refresh must reproduce committed transcript meaning from canonical evidence.

## Codex Behavior Worth Reusing

Use the completed [Codex TUI behavioral reference set](reference/README.md)
before implementation work.

The ignored local checkout is a design reference, not runtime authority. The
following files at the pinned commit are the relevant evidence:

| Behavior | Local reference |
| --- | --- |
| default/bold/dim and ANSI color rules | `.reference/codex/codex-rs/tui/styles.md` |
| retained semantic transcript cells | `.reference/codex/codex-rs/tui/src/app.rs` |
| finalized history in terminal scrollback | `.reference/codex/codex-rs/tui/src/insert_history.rs` |
| scrollback-safe inline drawing | `.reference/codex/codex-rs/tui/src/tui/scrollback.rs` |
| source-backed resize/reflow | `.reference/codex/codex-rs/tui/src/app/resize_reflow.rs` |
| transcript insertion lifecycle | `.reference/codex/codex-rs/tui/src/app/history_ui.rs` |
| empty always-visible composer | `.reference/codex/codex-rs/tui/src/bottom_pane/snapshots/codex_tui__bottom_pane__chat_composer__tests__empty.snap` |
| live working indicator | `.reference/codex/codex-rs/tui/src/snapshots/codex_tui__status_indicator_widget__tests__renders_with_working_header.snap` |
| transcript/live/composer composition | `.reference/codex/codex-rs/tui/src/chatwidget/snapshots/codex_tui__chatwidget__tests__chatwidget_tall.snap` |
| one-time session header | `.reference/codex/codex-rs/tui/src/history_cell/snapshots/codex_tui__history_cell__tests__session_header_indicates_yolo_mode.snap` |

The reusable idea is the behavioral split between source-backed committed
history, replaceable live state, a bottom composer, and overlays. Revolvr does
not need Codex's thread, model, token, attachment, plugin, multi-agent, or
provider machinery.

Codex source and snapshots are behavioral acceptance evidence only. Revolvr
reimplements the accepted interaction behavior in its existing Go/Bubble Tea
TUI and does not copy, port, vendor, depend on, or distribute Codex
implementation source.

## Invariants

Every epic must preserve these properties:

- Existing application and domain services remain authoritative.
- All current keyboard actions remain reachable until an accepted replacement
  is tested and documented.
- Safety state, cancellation, refresh, command execution, typed responses,
  scrolling, and active-operation settlement remain explicit.
- No model-authored text becomes lifecycle, verification, approval, commit, or
  completion authority.
- No new dependency is added without a demonstrated gap in Bubble Tea,
  Bubbles, Lip Gloss, or the standard library.
- Wide and narrow output remains within terminal width.
- Important state is textual and never color-only.
- Default terminal foreground remains the primary color; cyan, green, and red
  retain their current semantic roles.
- Transcript history is bounded by canonical source availability and terminal
  capability, not an unbounded in-memory log.
- No desktop, web server, REST/SSE surface, or Codex runtime dependency is
  introduced.

## Required Decisions Before Implementation

### D1 — Codex fidelity and ADR-025

**Status: Accepted 2026-08-27.** Reproduce the accepted Codex interaction
behavior in the existing Go/Bubble Tea code, use the pinned local source and
snapshots only as behavioral acceptance evidence, retain Revolvr branding and
domain semantics, and copy no Rust implementation. This retains ADR-025
unchanged and creates no source-attribution, NOTICE, Go/Rust-boundary, or
upgrade-ownership follow-up.

### D2 — Plain-text composer meaning

**Status: Accepted 2026-08-27.** Nonblank plain text has exactly one meaning:
when the repository is initialized and no operation or modal input is active,
Enter opens the existing Add Task review with the text prefilled as the task
body. This transition is an ephemeral draft, not a task, instruction, command,
or transcript commitment. The operator may edit the task and summary, press
Enter in the review to call `app.AddTaskAndCommit`, or press Escape to cancel
without a durable effect. Whitespace-only input does nothing.

Plain-text Enter is rejected everywhere else. Rejection preserves the composer
buffer, reports why the current state cannot accept it, and calls no application
service. The text is never interpreted as a run instruction, current-pass
steer, later-pass input, autonomous queue item, approval, or typed answer.

The current typed needs-input path remains exclusive: the question overlay
owns focus, requires an offered option and explicit two-step confirmation, and
submits task ID, question ID, revision, content SHA-256, and option ID through
`app.AnswerAutonomousInput`. No existing domain contract supports a free-form
answer.

The accepted composer contract is:

| State | Composer focus | Slash commands | Plain-text Enter | Confirmation or rejection | App/domain authority | Durable effect | Recovery |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Uninitialized | Focused when no overlay owns focus | Discovery, Help, navigation, refresh, and quit remain available; domain actions retain their initialization guards | Reject; the text is neither a task nor an instruction | Preserve the buffer and direct the operator to `revolvr init` | Existing command guards and `app.Status` | None | Initialize, refresh, then press Enter again; restart discards the local buffer |
| Initialized and idle | Focused when no overlay owns focus | All current commands remain discoverable and use their existing view, preflight, safety, and scheduling guards | Move nonblank text into the existing Add Task review as the task body | Review permits editing; review Enter publishes, while Escape cancels with no write | TUI `taskEntryState`, then `app.AddTaskAndCommit` on confirmation | No effect for the draft; confirmation creates one canonical task and commits only its task file | A pre-confirmation restart loses the ephemeral draft; publication errors stay in review for correction, cancellation, or canonical refresh |
| Active one-pass run | Focused; the live cell remains the operation owner | Read-only navigation/help plus `/cancel` and settlement-safe `/quit`; conflicting actions remain visible but guarded | Reject; never steer the running Codex process | Preserve the buffer and report that active steering is unsupported | `app.RunOnce`, `runonce`, and the existing cancellation context | None from text | Wait or cancel and settle; only a new Enter after the model is idle can open task review |
| Active bounded loop, autonomous task run, or autonomous task queue | Focused; the live cell remains the operation owner | Read-only navigation/help plus `/cancel` and settlement-safe `/quit`; new run, refresh, validation, answer, and task-publication actions remain guarded | Reject; never target a later fresh pass and never enter the autonomous task queue | Preserve the buffer and report that queued/deferred input is unsupported | `app.RunLoop`, `app.RunTaskUntilTerminal`, and `app.RunQueue`; the autonomous queue orders tasks only | None from text | Wait or cancel and settle; no text is replayed, consumed, or submitted automatically after a pass, task, queue, or restart |
| Typed needs-input | The typed question overlay owns focus; the normal composer is unavailable | Unavailable until the question overlay closes | Not a composer action; Enter selects/confirms only the offered typed option | Select an option, press Enter to confirm, then Enter again to persist; Escape closes without answering | `app.AnswerAutonomousInput` and `autonomousinput` using exact task/question/revision/content/option identity | Durable answer and separate durable resume transition | Stale identity fails closed and reloads current authority; a persisted answer whose resume failed is retried through the same exact typed path |
| Required callback unavailable, guard refusal, or app error | Focus stays with the current non-modal owner | Unaffected commands remain available; the affected command is disabled or returns its existing readable error | Reject before dispatch when task review cannot be supported; otherwise publication errors remain in review | Preserve the composer or review buffer and show the authoritative error; never render success | The existing command guard or called app service | None unless the app explicitly reports that its durable step already succeeded | Correct the condition and retry explicitly; refresh canonical state before retry when an outcome is indeterminate |

This reuses the current reviewed task-publication path and creates no new
application or domain prerequisite.

### D3 — Transcript and scrollback ownership

**Status: Open.** The desired behavior is copy-friendly continuous history.
Candidate implementations are:

- terminal-native scrollback with retained source cells and resize reflow;
- the existing viewport with a transcript overlay/pager;
- a small hybrid that commits finalized lines to scrollback and uses a viewport
  only for overlays.

The decision requires a bounded proof for resize, tmux, narrow widths, test IO,
and active-cell replacement. Do not assume that one `tea.Println` call solves
reflow and replay.

### D4 — Overlay migration

**Status: Proposed.** Keep every existing focused view and key route while the
new shell lands, then move views behind overlays in small groups. Remove the
page navigation only after parity tests exist. This makes the migration
reversible and keeps the domain seam unchanged.

### D5 — Loop and queued-input semantics

**Status: Accepted 2026-08-27.** Active plain text is unavailable for a
one-pass run, bounded loop, autonomous task run, and autonomous task queue.
Revolvr has no current-process steering channel or operator-message queue, and
fresh passes derive their authority from canonical tasks rather than carried
composer prose.

Rejected text creates no item, so it has no queue identity, order,
persistence, restart, cancellation, editing, consumption, or stale-run
lifecycle. A preserved composer buffer is local editable UI state only: it is
discarded on restart, may be cleared by the operator, is never auto-submitted
after settlement, and is reclassified only when the operator presses Enter
again in the then-current state. `autonomousqueue.Operation` and its ordered
selections remain solely an autonomous task scheduler contract; they are not
reused for messages.

The Codex study proves that steering and queued messages can form a coherent
chat interaction, but Revolvr has neither the domain authority nor an evidenced
need for that backend. TUI-041 is removed, and no prerequisite is created.

### D6 — Session header lifecycle

**Status: Proposed.** Replace the persistent `Revolvr Dashboard initialized`
row with one compact session transcript cell shown at startup and after an
explicit clear. The live footer should carry only context needed now.

## Whole-Overhaul Acceptance

The overhaul is complete only when:

- launching `revolvr tui` presents a transcript/composer application rather
  than a dashboard page;
- a completed run is readable as semantic transcript cells and a running
  operation occupies one replaceable live region;
- the composer is always visible when no typed question or overlay owns focus;
- slash commands discover every retained action;
- accepted plain-text behavior has a documented app-level contract;
- Tasks, Runs, Run Detail, Preflight, Workflow, Help, changes, evidence,
  approval, receipt validation, and typed responses remain reachable;
- cancellation and quit wait for active operation settlement;
- refresh/restart reproduces committed meaning from canonical evidence;
- normal and narrow-width snapshots stay within terminal bounds;
- transcript copy/scroll behavior is manually verified in the supported
  terminal matrix;
- obsolete dashboard/page chrome and duplicate presentation code are removed;
- focused and full Go tests pass with no new business logic in `internal/tui`.

## Non-Goals

- Rewriting Revolvr in Rust or embedding the Codex TUI.
- Mirroring Codex branding, models, token counters, plugins, attachments,
  multi-agent UI, or provider-specific features.
- Changing ledger event schemas or `app.RunTimeline` as a presentation shortcut.
- Adding a generalized chat backend or operator-message queue.
- Redesigning domain workflows, verification, evidence, approvals, or task
  lifecycle inside the TUI project.
- Publishing all draft tasks as one autonomous runnable chain.

## Document Review Gate

Before the first implementation task is created:

1. Resolve D1, D2, and D3; accept or replace D4 through D6.
2. Complete [TUI-005](tasks/tui-005-accept-experience-states.md) so the idle,
   running, completed, failure, cancellation, needs-input, and narrow-terminal
   snapshots describe the intended product.
3. Review the [epic index](#implementation-plan) and delete any task that does
   not independently improve or prove the operator experience.
4. Promote only [TUI-010](tasks/tui-010-prove-shell-composition.md) into
   `.agent/tasks/` and the active selector after E0 exits.
